package main

import (
	"recurbate/config"
	"recurbate/playlist"
	"recurbate/tools"
	"recurbate/ui"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

var tag string

// NEW: writes the .m3u8 playlist into the "Playlist Directory"
// (falls back to the unfinished folder, then the working directory)
func savePlaylist(cfg config.Config, playList playlist.Playlist) error {
	return os.WriteFile(filepath.Join(cfg.PlaylistDir(), playList.Filename+".m3u8"), playList.M3u8, 0666)
}

// NEW: writes the playlist and shows a failure in the error box
func savePlaylistReport(cfg config.Config, playList playlist.Playlist) {
	err := savePlaylist(cfg, playList)
	if err != nil {
		ui.Error(fmt.Sprintf("Failed to write playlist data: %v", err))
	}
}

// NEW: gets the playlist of every url. Each one found is added to the
// waiting list on the dashboard as soon as its name is known.
func fetchPlaylists(cfg config.Config) []playlist.Playlist {
	playlists := make([]playlist.Playlist, len(cfg.Urls))
	for i, link := range cfg.Urls {
		ui.Phase(fmt.Sprintf("Fetching playlists %d/%d", i+1, len(cfg.Urls)))
		playlists[i] = cfg.GetPlaylist(link, i)
		if !playlists[i].IsNil() {
			ui.Queue(i, playlists[i].Filename)
		}
	}
	ui.Phase("")
	return playlists
}

// NEW: runs a service with the dashboard on screen (when the output is a
// terminal), the dashboard is always closed again afterwards
func runWithUI(cfg config.Config, run func()) {
	if ui.Open(cfg.ASCIIMode()) {
		defer ui.Close()
	}
	run()
}

func parallelService(cfg config.Config) {
	playlists := fetchPlaylists(cfg)
	var wg sync.WaitGroup
	for _, playList := range playlists {
		if playList.IsNil() {
			continue
		}
		wg.Add(1)
		go func(playList playlist.Playlist) {
			defer wg.Done()
			if cfg.GetVideo(playList) == nil {
				return
			}
			savePlaylistReport(cfg, playList)
		}(playList)
		time.Sleep(time.Second)
	}
	wg.Wait()
}

func hybridService(cfg config.Config) {
	playlists := fetchPlaylists(cfg)
	cfg.SeedAll(playlists) // new urls get their file first
	servers := make(map[string][]playlist.Playlist)
	// organize playlist by server
	for _, playList := range playlists {
		if playList.IsNil() {
			continue // already finished, or its error was already shown
		}
		server, err := playList.PlaylistOrigin()
		if err != nil {
			ui.Error(err.Error())
			continue
		}
		if servers[server] == nil {
			servers[server] = make([]playlist.Playlist, 0)
		}
		servers[server] = append(servers[server], playList)
	}
	var wg sync.WaitGroup
	for _, playlists := range servers {
		wg.Add(1)
		go func(playlists []playlist.Playlist) {
			defer wg.Done()
			for _, playList := range playlists {
				if playList.IsNil() {
					continue
				}
				if cfg.GetVideo(playList) == nil {
					continue
				}
				savePlaylistReport(cfg, playList)
			}
		}(playlists)
	}
	wg.Wait()
}

// NEW: Reads the optional number after "series" = how many files download at once.
// Missing or invalid means 1 (one file at a time, same as before).
func parseSeriesCount() int {
	arg := tools.Argparser(3)
	if arg == "" {
		return 1
	}
	n, err := strconv.Atoi(arg)
	if err != nil || n < 1 {
		fmt.Fprintf(os.Stderr, "Invalid number of simultaneous downloads %q, using 1\n", arg)
		return 1
	}
	return n
}

// NEW: Downloads the list with up to `workers` files at the same time
func serialService(cfg config.Config, workers int) {
	playlists := fetchPlaylists(cfg)
	cfg.SeedAll(playlists) // new urls get their file first
	if workers > len(playlists) {
		workers = len(playlists)
	}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if tools.Abort {
					continue // drain the queue without starting anything new
				}
				playList := playlists[i]
				if cfg.GetVideo(playList) == nil {
					continue
				}
				savePlaylistReport(cfg, playList)
			}
		}()
	}
	for i, playList := range playlists {
		if playList.IsNil() {
			continue
		}
		if tools.Abort {
			break
		}
		jobs <- i
	}
	close(jobs)
	wg.Wait()
}

func downloadPlaylist(cfg config.Config) {
	for i, v := range cfg.Urls {
		ui.Phase(fmt.Sprintf("Fetching playlist %d/%d", i+1, len(cfg.Urls)))
		playList := cfg.GetPlaylist(v, i)
		if playList.IsNil() {
			continue
		}
		err := savePlaylist(cfg, playList)
		if err != nil {
			ui.Error(fmt.Sprintf("Failed to write playlist data: %v", err))
			continue
		}
		ui.Logf("Completed: %v:%v", playList.Filename, v)
	}
}
func downloadConent(cfg config.Config) {
	playlistPath := tools.Argparser(3)
	data, err := os.ReadFile(playlistPath)
	if err != nil {
		ui.Error(fmt.Sprintf("Failed to read playlist: %v", err))
		return
	}
	filename := playlistPath
	if strings.Contains(filename, string(os.PathSeparator)) {
		tempSplit := strings.Split(filename, string(os.PathSeparator))
		filename = tempSplit[len(tempSplit)-1]
	}
	filename = strings.ReplaceAll(filename, ".m3u8", "")
	playList := playlist.NewFromFilename(data, filename, 0)
	cfg.GetVideo(playList)
}
func readme() string {
	path := tools.Argparser(0)
	if strings.Contains(path, string(os.PathSeparator)) {
		split := strings.Split(path, string(os.PathSeparator))
		path = split[len(split)-1]
	}
	string1 := `recurbate:
If ran for the first time, json configuration will be generated
	in the working directory
Fill in the json's URL, Cookie and User-Agent to allow the
	program to run

Usage: `
	string2 := ` <json location> playlist|series|hybrid [N | playlist.m3u8]

if "playlist" is used, only the .m3u8 playlist file will be
	downloaded, specifiying the playlist location will
	download the contents of the playlist
if "series" is used, the program will download all the videos
	in the list. An optional number sets how many download at the
	same time (e.g. "series 4"), default is 1
if "hybrid is used, the program will download sequentially from
	each server but in parallel from different servers
"series" and "hybrid" first give every new url (last value missing or
	0) a short download of its first fragments, all at once, so its
	.ts file exists and shows how many files there are. The value in
	the json becomes the line it reached (0 becomes 2). The full
	downloads then continue from there.

Dashboard: while downloading, the screen shows the active downloads,
	the waiting ones, the finished ones and the errors. Set
	"ASCII Interface" to "true" in the "options" part of the json if the
	console shows the boxes or the bar as "?"

Folders (set in the "options" part of the json, all optional):
"Unfinished Directory" - downloads are written here while in progress
	and stay here if the download is stopped or fails
"Finished Directory"   - a download is moved here once it has fully
	downloaded
"Playlist Directory"   - .m3u8 files are saved here (defaults to the
	unfinished folder)
Blank means the working directory. Use / or \\ in paths,
	e.g. "D:/Videos/Done"`
	return string1 + path + string2
}
func init() {
	go func() {
		inter := make(chan os.Signal, 1)
		signal.Notify(inter, os.Interrupt, syscall.SIGTERM)
		<-inter
		tools.Abort = true
		force := make(chan os.Signal, 1)
		signal.Notify(force, os.Interrupt, syscall.SIGTERM)
		<-force
		ui.Close() // give the normal screen back
		os.Exit(0)
	}()
}
func main() {
	fmt.Printf("recurbate Custom Downloader %v\n", tag)
	tools.CheckUpdate(tag)
	if tools.Argparser(1) == "--help" {
		fmt.Println(readme())
		return
	}
	json_location := "config.json"
	if tools.Argparser(1) != "" {
		json_location = tools.Argparser(1)
	}
	_, err := os.Stat(json_location)
	if err != nil {
		defaultConfig := config.Default()
		defaultConfig.Save()
		fmt.Printf("%v created in working directory\nPlease fill in the %v with the \n\tURLs to Download\n\tCookies\n\tUser-Agent\n", json_location, json_location)
		return
	}
	jsonData, err := os.ReadFile(json_location)
	if err != nil {
		fmt.Println(err)
		os.Exit(4)
	}
	var cfg config.Config
	err = json.Unmarshal(jsonData, &cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Reading Json: %v", err)
		os.Exit(4)
	}
	// NEW: create the unfinished / finished / playlist folders if they don't exist
	err = cfg.EnsureDirs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(4)
	}
	if cfg.Empty() {
		fmt.Println("please modify config.json")
		if tools.Argparser(2) != "parse" {
			return
		}
	}
	switch tools.Argparser(2) {
	case "playlist":
		if tools.Argparser(3) != "" {
			_, err := os.Stat(tools.Argparser(3))
			if err != nil {
				fmt.Println(err)
				os.Exit(4)
			}
			runWithUI(cfg, func() { downloadConent(cfg) })
		} else {
			downloadPlaylist(cfg)
		}
	case "series":
		workers := parseSeriesCount() // read before the dashboard starts, it can print a warning
		runWithUI(cfg, func() { serialService(cfg, workers) })
	case "hybrid":
		runWithUI(cfg, func() { hybridService(cfg) })
	case "parse":
		err := cfg.ParseHtml(tools.Argparser(3))
		if err != nil {
			fmt.Println(err)
		} else {
			fmt.Println("Parsed HTML Successfully")
		}
	default:
		runWithUI(cfg, func() { parallelService(cfg) })
	}
}
