package ui

import (
	"fmt"
	"strconv"
	"strings"
)

// Layout (one screen):
//
//	+------------------------+--------------+
//	| Downloading            | Finished     |
//	|                        |              |
//	|                        +--------------+
//	+------------------------+ Errors       |
//	| Waiting                |              |
//	+------------------------+--------------+

const (
	// smaller windows get a "too small" message instead of the dashboard
	minWidth  = 60
	minHeight = 18
)

// characters used to draw
type charset struct {
	tl, tr, bl, br string // box corners
	h, v           string // box lines
	teeDown, teeUp string // where a divider meets the top / bottom line
	filled, blank  string // progress bar
	ellipsis       string
	bullet         string
}

var unicodeSet = charset{"┌", "┐", "└", "┘", "─", "│", "┬", "┴", "▇", "-", "…", "-"}
var asciiSet = charset{"+", "+", "+", "+", "-", "|", "+", "+", "#", "-", "~", "-"}

// color of the waiting box, also used for its middle divider
const waitingColor = "93"

// ---------------------------------------------------------------------
// text helpers
// ---------------------------------------------------------------------

const resetStyle = "\x1b[0m"

// wraps text in a color / style (an ANSI "SGR" code like "91" or "1;93")
func style(code, s string) string {
	if s == "" {
		return ""
	}
	return "\x1b[" + code + "m" + s + resetStyle
}

func runeLen(s string) int {
	return len([]rune(s))
}

// number of characters that show on screen (color codes take no space)
func visLen(s string) int {
	n := 0
	inEsc := false
	for _, r := range s {
		switch {
		case inEsc:
			if r == 'm' {
				inEsc = false
			}
		case r == '\x1b':
			inEsc = true
		default:
			n++
		}
	}
	return n
}

func padRight(s string, w int) string {
	if n := visLen(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

// cuts the end off, for text where the start matters
func cutEnd(s string, n int, cs charset) string {
	r := []rune(s)
	if n <= 0 {
		return ""
	}
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + cs.ellipsis
}

// cuts the middle out, for file names: the end holds the date and time
// that tell the files apart, so it is kept
func cutName(s string, n int, cs charset) string {
	r := []rune(s)
	if n <= 0 {
		return ""
	}
	if len(r) <= n {
		return s
	}
	if n == 1 {
		return cs.ellipsis
	}
	head := (n - 1) / 3
	tail := n - 1 - head
	return string(r[:head]) + cs.ellipsis + string(r[len(r)-tail:])
}

// splits text into lines of at most n characters
func wrap(s string, n int) []string {
	if n < 1 {
		n = 1
	}
	var lines []string
	cur := ""
	for _, word := range strings.Fields(s) {
		for runeLen(word) > n {
			if cur != "" {
				lines = append(lines, cur)
				cur = ""
			}
			r := []rune(word)
			lines = append(lines, string(r[:n]))
			word = string(r[n:])
		}
		switch {
		case cur == "":
			cur = word
		case runeLen(cur)+1+runeLen(word) <= n:
			cur += " " + word
		default:
			lines = append(lines, cur)
			cur = word
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// ---------------------------------------------------------------------
// boxes
// ---------------------------------------------------------------------

// Draws a box of exactly w x h characters. Content lines must already fit
// inside (w-4 characters), extra lines are cut off.
// split > 0 joins a divider to the top and bottom line, split characters
// from the left edge of the content (the content draws the divider itself).
func box(cs charset, title, color string, w, h, split int, content []string) []string {
	iw := w - 4
	ih := h - 2
	lines := make([]string, 0, h)

	t := " " + cutEnd(title, w-7, cs) + " "
	fill := w - 3 - runeLen(t)
	if fill < 0 {
		fill = 0
	}
	divCol := 2 + split // column of the divider, counted from the left edge
	fillStr := strings.Repeat(cs.h, fill)
	if tStart := 2 + runeLen(t); split > 0 && divCol >= tStart && divCol < tStart+fill {
		idx := divCol - tStart
		fillStr = strings.Repeat(cs.h, idx) + cs.teeDown + strings.Repeat(cs.h, fill-idx-1)
	}
	lines = append(lines,
		style(color, cs.tl+cs.h)+style("1;"+color, t)+style(color, fillStr+cs.tr))

	side := style(color, cs.v)
	for i := 0; i < ih; i++ {
		line := ""
		if i < len(content) {
			line = content[i]
		}
		lines = append(lines, side+" "+padRight(line, iw)+" "+side)
	}

	bottom := strings.Repeat(cs.h, w-2)
	if split > 0 && divCol-1 < w-2 {
		bottom = strings.Repeat(cs.h, divCol-1) + cs.teeUp + strings.Repeat(cs.h, w-2-(divCol-1)-1)
	}
	lines = append(lines, style(color, cs.bl+bottom+cs.br))
	return lines
}

// ---------------------------------------------------------------------
// box contents
// ---------------------------------------------------------------------

func progressBar(percent float64, bw int, cs charset) string {
	f := int(percent / 100 * float64(bw))
	if f > bw {
		f = bw
	}
	if f < 0 {
		f = 0
	}
	return "[" + style("97", strings.Repeat(cs.filled, f)) +
		style("90", strings.Repeat(cs.blank, bw-f)) + "]"
}

// "[▇▇▇▇▇-----] 54.3%  ETA 2.1 mins  3.2 MB/s", the ETA and speed are left
// out when the box is too narrow for them
func barLine(d download, cs charset, iw int) string {
	pct := fmt.Sprintf("%5.1f%%", d.percent)
	etaPart := ""
	if d.eta != "" {
		etaPart = "  ETA " + d.eta
	}
	speedPart := ""
	if d.speed != "" {
		speedPart = "  " + d.speed
	}
	tails := []string{etaPart + speedPart, etaPart, ""}
	for i, rest := range tails {
		bw := iw - 3 - runeLen(pct) - runeLen(rest)
		if bw >= 10 || i == len(tails)-1 {
			if bw < 1 {
				return style("93", pct) + rest
			}
			return progressBar(d.percent, bw, cs) + " " + style("93", pct) + rest
		}
	}
	return ""
}

// the file name, followed by the temporary note if there is one
func nameLine(d download, cs charset, iw int) string {
	if d.note == "" {
		return style("1", cutName(d.name, iw, cs))
	}
	note := cutEnd(d.note, iw/2, cs)
	nameW := iw - runeLen(note) - 2
	return style("1", cutName(d.name, nameW, cs)) + "  " + style("33", note)
}

func downloadingContent(snap snapshot, cs charset, iw, ih int) []string {
	var lines []string
	if snap.phase != "" {
		lines = append(lines, style("93", cutEnd(snap.phase, iw, cs)))
		if len(snap.active) > 0 {
			lines = append(lines, "")
		}
	}
	total := len(snap.active)
	if total == 0 {
		if snap.phase == "" {
			lines = append(lines, style("90", "No active downloads"))
		}
		return lines
	}
	// every download is 2 lines: name and bar. A temporary note (retrying...)
	// is shown on the name line, after the name.
	room := ih - len(lines)
	fit := room / 2
	more := 0
	if total > fit {
		fit = (room - 1) / 2 // leave a line for "+N more"
		if fit < 1 {
			fit = 1
		}
		more = total - fit
	}
	for i := 0; i < fit && i < total; i++ {
		d := snap.active[i]
		lines = append(lines, nameLine(d, cs, iw), barLine(d, cs, iw))
	}
	if more > 0 {
		lines = append(lines, style("90", fmt.Sprintf("+%d more", more)))
	}
	return lines
}

// width of the left column of the waiting box (the right one is the rest,
// minus 1 character for the divider)
func waitingSplit(iw int) int {
	return (iw - 1) / 2
}

// Two columns with a divider: the list fills the left column from top to
// bottom and then continues in the right column.
func waitingContent(snap snapshot, cs charset, iw, ih int) []string {
	lc := waitingSplit(iw)
	rc := iw - 1 - lc
	capacity := 2 * ih
	names := snap.waiting
	moreLine := ""
	if len(names) > capacity {
		names = names[:capacity-1]
		moreLine = style("90", fmt.Sprintf("+%d more", len(snap.waiting)-len(names)))
	}
	// text of the k-th slot (0..capacity-1), width is that column's width
	cell := func(k, width int) string {
		switch {
		case k < len(names):
			return cutName(names[k], width, cs)
		case k == capacity-1 && moreLine != "":
			return moreLine
		}
		return ""
	}
	divider := style(waitingColor, cs.v)
	lines := make([]string, ih)
	for i := 0; i < ih; i++ {
		lines[i] = padRight(cell(i, lc), lc) + divider + cell(ih+i, rc)
	}
	return lines
}

func finishedContent(snap snapshot, cs charset, iw, ih int) []string {
	// newest at the bottom, the oldest scroll off the top
	names := snap.finished
	if len(names) > ih {
		names = names[len(names)-ih:]
	}
	var lines []string
	for _, name := range names {
		lines = append(lines, cutName(name, iw, cs))
	}
	return lines
}

func errorContent(snap snapshot, cs charset, iw, ih int) []string {
	const maxLinesPerMessage = 4
	var all []string
	for _, e := range snap.errors {
		text := e.text
		if e.count > 1 {
			text += fmt.Sprintf(" (x%d)", e.count)
		}
		wrapped := wrap(text, iw-2)
		if len(wrapped) > maxLinesPerMessage {
			// too long: keep the first lines and end the last one with "..."
			wrapped = wrapped[:maxLinesPerMessage]
			last := []rune(wrapped[maxLinesPerMessage-1])
			if len(last) > iw-3 && iw-3 > 0 {
				last = last[:iw-3]
			}
			wrapped[maxLinesPerMessage-1] = string(last) + cs.ellipsis
		}
		for i, l := range wrapped {
			prefix := "  "
			if i == 0 {
				prefix = cs.bullet + " "
			}
			all = append(all, style("91", prefix+l))
		}
	}
	// newest at the bottom
	if len(all) > ih {
		all = all[len(all)-ih:]
	}
	return all
}

// ---------------------------------------------------------------------
// the whole screen
// ---------------------------------------------------------------------

// Builds one full frame for a terminal of termW x termH characters.
// Lines are joined with \r\n and every line ends by clearing the rest of it.
func render(termW, termH int) string {
	snap := takeSnapshot()
	cs := unicodeSet
	if snap.ascii {
		cs = asciiSet
	}
	// the last column is left empty, writing into it can make some
	// consoles jump to the next line
	w := termW - 1
	h := termH
	if w < minWidth || h < minHeight {
		return fmt.Sprintf("Window too small, make it at least %dx%d (now %dx%d)\x1b[K",
			minWidth+1, minHeight, termW, termH)
	}

	rw := w * 35 / 100 // right column
	if rw < 28 {
		rw = 28
	}
	if rw > 50 {
		rw = 50
	}
	lw := w - rw // left column
	gh := h * 22 / 100
	if gh < 5 {
		gh = 5
	}
	gh += 4      // waiting box is 4 lines taller than the base size
	rh := h - gh // downloading box
	bh := gh     // errors box: same height as the waiting box
	oh := h - bh // finished box gets the rest of the right column

	downloadBox := box(cs, "Downloading ("+strconv.Itoa(len(snap.active))+")", "92", lw, rh, 0,
		downloadingContent(snap, cs, lw-4, rh-2))
	waitingBox := box(cs, "Waiting ("+strconv.Itoa(len(snap.waiting))+")", waitingColor, lw, gh, waitingSplit(lw-4),
		waitingContent(snap, cs, lw-4, gh-2))
	finishedBox := box(cs, "Finished ("+strconv.Itoa(len(snap.finished))+")", "97", rw, oh, 0,
		finishedContent(snap, cs, rw-4, oh-2))
	errorBox := box(cs, "Errors ("+strconv.Itoa(len(snap.errors))+")", "91", rw, bh, 0,
		errorContent(snap, cs, rw-4, bh-2))

	var sb strings.Builder
	for r := 0; r < h; r++ {
		var left, right string
		if r < rh {
			left = downloadBox[r]
		} else {
			left = waitingBox[r-rh]
		}
		if r < oh {
			right = finishedBox[r]
		} else {
			right = errorBox[r-oh]
		}
		sb.WriteString(left)
		sb.WriteString(right)
		sb.WriteString("\x1b[K")
		if r < h-1 {
			sb.WriteString("\r\n")
		}
	}
	return sb.String()
}
