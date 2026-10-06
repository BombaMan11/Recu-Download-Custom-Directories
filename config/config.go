package config

import (
	"recurbate/recu"
	"recurbate/playlist"
	"recurbate/tools"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// mutex
var (
	mtx sync.Mutex
)

// Names of the keys inside the "options" part of the json
const (
	optMaxRes     = "Maximum Resolution (Height)"
	optUnfinished = "Unfinished Directory"
	optFinished   = "Finished Directory"
	optPlaylist   = "Playlist Directory"
)

// Defines the JSON used
type Config struct {
	Urls    []any             `json:"urls"`
	Header  map[string]string `json:"header"`
	Options map[string]string `json:"options"`
}

// ---------------------------------------------------------------------
// NEW: directory options
// ---------------------------------------------------------------------

// reads a folder option from the json, blank if it is missing
func (config Config) dirOption(key string) string {
	return strings.TrimSpace(config.Options[key])
}

// Folder where downloads are written while they are in progress.
// Blank means the working directory.
func (config Config) UnfinishedDir() string {
	return config.dirOption(optUnfinished)
}

// Folder where a download is moved once it has fully downloaded.
// Blank means the working directory.
func (config Config) FinishedDir() string {
	return config.dirOption(optFinished)
}

// Folder where .m3u8 playlist files are saved.
// Blank means the same folder as the unfinished downloads.
func (config Config) PlaylistDir() string {
	dir := config.dirOption(optPlaylist)
	if dir != "" {
		return dir
	}
	return config.UnfinishedDir()
}

// Creates every folder set in the json so they don't have to exist beforehand
func (config Config) EnsureDirs() error {
	for _, dir := range []string{config.UnfinishedDir(), config.FinishedDir(), config.PlaylistDir()} {
		if dir == "" {
			continue
		}
		err := os.MkdirAll(dir, 0755)
		if err != nil {
			return fmt.Errorf("can not create folder %s: %v", dir, err)
		}
	}
	return nil
}

// true if both paths point at the same existing folder ("" = working directory)
func sameDir(a, b string) bool {
	if a == "" {
		a = "."
	}
	if b == "" {
		b = "."
	}
	infoA, err := os.Stat(a)
	if err != nil {
		return false
	}
	infoB, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(infoA, infoB)
}

// Moves a fully downloaded file from the unfinished folder to the finished folder
func (config Config) moveToFinished(unfinishedDir, fileName string) {
	finishedDir := config.FinishedDir()
	if sameDir(unfinishedDir, finishedDir) {
		return
	}
	src := filepath.Join(unfinishedDir, fileName)
	dst, err := tools.MoveFile(src, finishedDir)
	if err != nil {
		// The download itself succeeded, so it is still marked COMPLETE
		fmt.Fprintf(os.Stderr, "Download complete, but moving the file failed: %v\nFile is at: %s\n", err, src)
		return
	}
	fmt.Printf("Moved to: %s\n", dst)
}

// ---------------------------------------------------------------------

// Gets Playlist
func (config Config) GetPlaylist(urlAny any, jsonLoc int) (playList playlist.Playlist) {
	url, _, _, err, complete := parseUrl(urlAny)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetPlaylist: urls are in wrong format, error: %v\n", err)
	}
	if complete {
		return
	}
	playList, status, err := recu.Parse(url, config.Header, jsonLoc, config.parseMaxRes())
	switch status {
	case "cloudflare":
		fmt.Fprintf(os.Stderr, "%s\nCloudflare Blocked: Failed on url: %v\n", err.Error(), url)
	case "cookie":
		fmt.Fprintf(os.Stderr, "Please Log in: Failed on url: %v\n", url)
	case "wait":
		fmt.Fprintf(os.Stderr, "Daily View Used: Failed on url: %v\n", url)
	case "panic":
		fmt.Fprintf(os.Stderr, "Error: %s\nFailed on url: %v\n", err.Error(), url)
	}
	return
}

// parse maximum resolution from json to an integer
func (config Config) parseMaxRes() int {
	maxResString := config.Options[optMaxRes]
	i, err := strconv.Atoi(maxResString)
	if err != nil {
		i = 6969
	}
	return i
}

// Saves video to the unfinished folder, then moves it to the finished folder once complete
func (config *Config) GetVideo(playList playlist.Playlist) error {
	url, duration, startIndex, err, _ := parseUrl(config.Urls[playList.JsonLoc])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return err
	}
	unfinishedDir := config.UnfinishedDir()
	// download and mux playlist
	lastIndex, finalName, err := recu.Mux(playList, tools.FormatedHeader(config.Header, "", 0), startIndex, duration, unfinishedDir)
	println()
	if err == nil {
		modifyUrl(&config.Urls[playList.JsonLoc], "COMPLETE")
		fmt.Printf("Completed: %v:%v\n", finalName, url)
		// only a fully downloaded file ever leaves the unfinished folder
		config.moveToFinished(unfinishedDir, finalName+".ts")
	} else {
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintf(os.Stderr, "Download Failed at line: %v\n", lastIndex)
		modifyUrl(&config.Urls[playList.JsonLoc], lastIndex)
	}
	// save state to json
	err2 := config.Save()
	if err2 != nil {
		fmt.Println(err2)
	}
	return err
}

// Modify URL object from json
func modifyUrl(url *any, lastIndex any) {
	switch t := (*url).(type) {
	case string:
		*url = []any{t, lastIndex}
	case []any:
		switch len(t) {
		case 1:
			t = append(t, lastIndex)
			*url = t
		case 2:
			t[1] = lastIndex
			*url = t
		case 4:
			t = append(t, lastIndex)
			*url = t
		case 5:
			t[4] = lastIndex
			*url = t
		}
	}
}

// Parse URL object from json
func parseUrl(url any) (urlString string, duration []float64, startIndex int, err error, complete bool) {
	defer func() {
		r := recover()
		if r != nil {
			err = fmt.Errorf("GetVideo: urls are in wrong format, error: %v", r)
		}
	}()
	switch t := url.(type) {
	case string:
		urlString = t
	case []any:
		switch len(t) {
		case 1:
			urlString = t[0].(string)
		case 2:
			urlString = t[0].(string)
			str, ok := t[1].(string)
			if ok {
				if str == "COMPLETE" {
					complete = true
				}
			} else {
				startIndex = int(t[1].(float64))
			}
		case 4:
			urlString = t[0].(string)
			duration = tools.PercentPrase(t[1:])
		case 5:
			urlString = t[0].(string)
			duration = tools.PercentPrase(t[1:4])
			str, ok := t[4].(string)
			if ok {
				if str == "COMPLETE" {
					complete = true
				}
			} else {
				startIndex = int(t[4].(float64))
			}

		default:
			err = fmt.Errorf("incorrect length of url array")
		}
	default:
		err = fmt.Errorf("url is incorrect type")
	}
	if duration == nil {
		duration = []float64{0, 100}
	}
	return
}

// Returns default templet
func Default() Config {
	var jsonTemplet Config
	jsonTemplet.Header = map[string]string{
		"Cookie":     "",
		"User-Agent": "",
	}
	jsonTemplet.Urls = []any{""}
	jsonTemplet.Options = map[string]string{
		optMaxRes:     "",
		optUnfinished: "",
		optFinished:   "",
		optPlaylist:   "",
	}
	return jsonTemplet
}

// Saves Json
func (config *Config) Save() (err error) {
	// defer so the lock is always released, even on an error return
	mtx.Lock()
	defer mtx.Unlock()
	var jsonData []byte
	jsonData, err = json.MarshalIndent(config, "", "\t")
	if err != nil {
		return fmt.Errorf("error: Parsing Json%v", err)
	}
	jsonLocation := "config.json"
	if tools.Argparser(1) != "" {
		jsonLocation = tools.Argparser(1)
	}
	err = os.WriteFile(jsonLocation, jsonData, 0666)
	if err != nil {
		err = fmt.Errorf("error: Saving Json:%v", err)
		return
	}
	return
}

func (config *Config) Empty() bool {
	return (len(config.Urls) < 1 || config.Urls[0] == "" || config.Header["Cookie"] == "" || config.Header["User-Agent"] == "")

}

// Parse Urls from HTML
func (config Config) ParseHtml(url string) (err error) {
	fmt.Println("Downloading HTML")
	resp, code, err := tools.Request(url, 10, tools.FormatedHeader(config.Header, "", 1), nil, "GET")
	if code != 200 || err != nil {
		if err == nil {
			err = fmt.Errorf("response: %s, status code: %d, cloudflare blocked", tools.ANSIColor(string(resp), 2), code)
		}
		return
	}
	fmt.Println("Searching for Links")
	urlSplit := strings.Split(url, "/")
	name := urlSplit[4]
	prefix := strings.Join(urlSplit[:3], "/")
	urls := config.Urls
	lines := strings.Split(string(resp), "\n")
	for _, v := range lines {
		code, err := tools.SearchString(v, fmt.Sprintf(`href="/%s/video/`, name), `/play"`)
		if err != nil {
			continue
		}
		urls = append(urls, fmt.Sprintf("%s/%s/video/%s/play", prefix, name, code))
	}
	config.Urls = urls
	err = config.Save()
	return
}
