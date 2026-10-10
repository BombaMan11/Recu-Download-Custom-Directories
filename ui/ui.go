// Package ui draws the full screen download dashboard.
//
// The rest of the program never prints while a download is running, it
// reports what is happening to this package instead:
//
//	Queue    - a file is known and waiting
//	Begin    - a file started downloading
//	Progress - how far a download is
//	Note     - a short temporary message for one download (retrying...)
//	Finish   - a download completed
//	Fail     - a download stopped
//	Error    - anything else that went wrong
//	Phase    - what the program is doing before the downloads start
//
// If the output is not a terminal (redirected to a file, or the terminal can
// not be set up) nothing is drawn and these same calls print plain lines
// instead, so the program still works.
package ui

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

type download struct {
	id      int
	name    string
	percent float64
	eta     string
	speed   string
	note    string
}

type queued struct {
	id   int
	name string
}

type errorEntry struct {
	when  time.Time
	text  string
	count int
}

const (
	// longer messages are cut so one huge response body can't flood the screen
	maxErrorLen = 400
	// the oldest messages are dropped after this many
	maxErrors = 500
)

var (
	mu       sync.Mutex
	active   = map[int]*download{}
	lastStep = map[int]int{} // plain mode: last 10% step printed per download
	waiting  []queued
	finished []string
	errs     []errorEntry
	phase    string

	enabled bool // true while the dashboard is on screen
	ascii   bool // plain ASCII characters instead of box drawing / block characters

	restoreConsole func()
	stopCh         chan struct{}
	doneCh         chan struct{}
	closeOnce      sync.Once
)

// ---------------------------------------------------------------------
// text cleanup
// ---------------------------------------------------------------------

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)

// Turns any message into one clean line: color codes removed, line breaks
// and runs of spaces collapsed, and cut to limit characters.
func clean(s string, limit int) string {
	s = ansiRe.ReplaceAllString(s, "")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && !unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if limit > 3 && len(r) > limit {
		s = string(r[:limit-3]) + "..."
	}
	return s
}

// ---------------------------------------------------------------------
// reporting API (safe to call from any goroutine)
// ---------------------------------------------------------------------

// Remembers a file that is waiting to be downloaded
func Queue(id int, name string) {
	mu.Lock()
	defer mu.Unlock()
	for i := range waiting {
		if waiting[i].id == id {
			waiting[i].name = name
			return
		}
	}
	waiting = append(waiting, queued{id: id, name: name})
}

func removeQueued(id int) {
	for i := range waiting {
		if waiting[i].id == id {
			waiting = append(waiting[:i], waiting[i+1:]...)
			return
		}
	}
}

// A file started downloading (moves it from waiting to downloading)
func Begin(id int, name string) {
	mu.Lock()
	defer mu.Unlock()
	removeQueued(id)
	active[id] = &download{id: id, name: name}
	if !enabled {
		fmt.Printf("Downloading: %s\n", name)
	}
}

// Updates a download. eta and speed are already formatted text.
func Progress(id int, percent float64, eta, speed string) {
	mu.Lock()
	defer mu.Unlock()
	d, ok := active[id]
	if !ok {
		return
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	d.percent, d.eta, d.speed, d.note = percent, eta, speed, ""
	if !enabled {
		// no live display, so print a line every 10%
		step := int(percent) / 10
		if step > lastStep[id] {
			lastStep[id] = step
			fmt.Printf("%s: %d%%\n", d.name, step*10)
		}
	}
}

// Shows a short temporary message under one download, it disappears on the
// next progress update.
func Note(id int, text string) {
	mu.Lock()
	defer mu.Unlock()
	text = clean(text, 120)
	name := ""
	if d, ok := active[id]; ok {
		d.note = text
		name = d.name
	}
	if !enabled {
		fmt.Fprintf(os.Stderr, "%s: %s\n", name, text)
	}
}

// A download completed. info is only used for the plain text output.
func Finish(id int, name, info string) {
	mu.Lock()
	defer mu.Unlock()
	delete(active, id)
	delete(lastStep, id)
	removeQueued(id)
	finished = append(finished, name)
	if !enabled {
		if info != "" {
			fmt.Printf("Completed: %v:%v\n", name, info)
		} else {
			fmt.Printf("Completed: %v\n", name)
		}
	}
}

// A download stopped, the reason goes into the error box
func Fail(id int, name, reason string) {
	mu.Lock()
	defer mu.Unlock()
	delete(active, id)
	delete(lastStep, id)
	removeQueued(id)
	addError(fmt.Sprintf("%s: %s", name, reason))
}

// Anything else that went wrong
func Error(msg string) {
	mu.Lock()
	defer mu.Unlock()
	addError(msg)
}

// the caller must hold mu
func addError(msg string) {
	msg = clean(msg, maxErrorLen)
	if msg == "" {
		return
	}
	if n := len(errs); n > 0 && errs[n-1].text == msg {
		// the same message again: count it instead of repeating it
		errs[n-1].count++
		errs[n-1].when = time.Now()
		return
	}
	errs = append(errs, errorEntry{when: time.Now(), text: msg, count: 1})
	if len(errs) > maxErrors {
		errs = errs[len(errs)-maxErrors:]
	}
	if !enabled {
		fmt.Fprintln(os.Stderr, msg)
	}
}

// What the program is doing before the downloads start ("" clears it)
func Phase(text string) {
	mu.Lock()
	defer mu.Unlock()
	phase = clean(text, 200)
	if !enabled && phase != "" {
		fmt.Println(phase)
	}
}

// A normal message. Printed as a line when there is no dashboard, and left
// out when there is one (the dashboard already shows the state).
func Logf(format string, args ...any) {
	mu.Lock()
	on := enabled
	mu.Unlock()
	if !on {
		fmt.Printf(format+"\n", args...)
	}
}

// ---------------------------------------------------------------------
// dashboard on/off
// ---------------------------------------------------------------------

// Turns the dashboard on. Returns false (and changes nothing) when the output
// is not a terminal that can show it, the plain text output is used then.
// asciiMode uses only plain ASCII characters.
func Open(asciiMode bool) bool {
	mu.Lock()
	if enabled {
		mu.Unlock()
		return true
	}
	mu.Unlock()
	restore, ok := initConsole()
	if !ok {
		return false
	}
	mu.Lock()
	enabled = true
	ascii = asciiMode
	restoreConsole = restore
	stopCh = make(chan struct{})
	doneCh = make(chan struct{})
	mu.Unlock()
	// alternate screen (the old screen comes back afterwards), hidden cursor
	os.Stdout.WriteString("\x1b[?1049h\x1b[?25l\x1b[2J")
	go loop(stopCh, doneCh)
	return true
}

// Turns the dashboard off, gives the normal screen back and prints a summary.
// Safe to call more than once and from any goroutine.
func Close() {
	closeOnce.Do(realClose)
}

func realClose() {
	mu.Lock()
	on := enabled
	mu.Unlock()
	if !on {
		return
	}
	close(stopCh)
	<-doneCh // the loop draws one last frame before it stops
	os.Stdout.WriteString("\x1b[?25h\x1b[?1049l")
	if restoreConsole != nil {
		restoreConsole()
	}
	mu.Lock()
	enabled = false
	mu.Unlock()
	printSummary()
}

// What was on the dashboard is gone after it closes, so print what matters
func printSummary() {
	mu.Lock()
	defer mu.Unlock()
	fmt.Printf("Finished downloads: %d\n", len(finished))
	if len(finished) > 0 && len(finished) <= 30 {
		for _, name := range finished {
			fmt.Printf("  %s\n", name)
		}
	}
	if len(waiting) > 0 {
		fmt.Printf("Not started: %d\n", len(waiting))
	}
	if len(errs) > 0 {
		fmt.Println("Errors:")
		for _, e := range errs {
			count := ""
			if e.count > 1 {
				count = fmt.Sprintf(" (x%d)", e.count)
			}
			fmt.Printf("  [%s] %s%s\n", e.when.Format("15:04:05"), e.text, count)
		}
	}
}

// redraws the screen until stopped
func loop(stop, done chan struct{}) {
	defer close(done)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var lastW, lastH int
	var last string
	draw := func() {
		w, h, ok := consoleSize()
		if !ok {
			w, h = 80, 24
		}
		if w != lastW || h != lastH {
			// window was resized, wipe whatever was left from the old size
			os.Stdout.WriteString("\x1b[2J")
			lastW, lastH = w, h
			last = ""
		}
		frame := render(w, h)
		if frame != last {
			last = frame
			os.Stdout.WriteString("\x1b[H" + frame)
		}
	}
	for {
		select {
		case <-stop:
			draw()
			return
		case <-ticker.C:
			draw()
		}
	}
}

// ---------------------------------------------------------------------
// a copy of the state to draw from, so drawing never holds the lock
// ---------------------------------------------------------------------

type snapshot struct {
	active   []download
	waiting  []string
	finished []string
	errors   []errorEntry
	phase    string
	ascii    bool
}

func takeSnapshot() snapshot {
	mu.Lock()
	defer mu.Unlock()
	s := snapshot{phase: phase, ascii: ascii}
	for _, d := range active {
		s.active = append(s.active, *d)
	}
	// nearest to finished on top, same percentage keeps a stable order by name
	sort.Slice(s.active, func(i, j int) bool {
		if s.active[i].percent != s.active[j].percent {
			return s.active[i].percent > s.active[j].percent
		}
		return s.active[i].name < s.active[j].name
	})
	for _, q := range waiting {
		s.waiting = append(s.waiting, q.name)
	}
	s.finished = append(s.finished, finished...)
	s.errors = append(s.errors, errs...)
	return s
}
