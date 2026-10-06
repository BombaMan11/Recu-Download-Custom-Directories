### This was built using v1.12.4

Use `.\recu-custom-dir-windows-amd64.exe --help` for more information

Run with `.\recu-custom-dir-windows-amd64.exe`

First run will generate `config.json`, fill in urls to download and Cookie and User-Agent with header info using the network DevTools in Chrome
```Json
{
	"urls": [
		""
	],
	"header": {
		"Cookie": "",
		"User-Agent": ""
	},
	"options": {
		"Finished Directory": "",
		"Maximum Resolution (Height)": "",
		"Playlist Directory": "",
		"Unfinished Directory": ""
	}
}
```

Usage: `recurbate <json location> playlist/series <playlist.m3u8>`

`<json location>` is the location of the json

specifying `playlist` will cause the program to download only the .m3u8 file

specifying `series` will cause the program to download the videos serially instead of in parallel

specifying `playlist <playlist.m3u8>` will read the playlist from the location specified from `<playlist.m3u8>` and download that video
### Advanced Usage for the Custom Directory in v1.0.0
To specify a specific part of a video to download

example:
```JSON
{
	"urls": [
		["https://recu.me/video/xxxxxxx/play","55:00","1:10:00","1:30:00"]
	],
	"header": {
		"Cookie": "",
		"User-Agent": ""
	},
	"options": {
		"Finished Directory": "C:/Videos/Finished",
		"Maximum Resolution (Height)": "",
		"Playlist Directory": "C:/Videos/m3u8",
		"Unfinished Directory": "C:/Videos/Unfinished"
	}
}
```
Where you specify the start, end and total length of the video and set the directory of the specific parts of the file
