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

### Advanced Usage for the Custom Directory in v1.0.0
To specify a specific part of a video to download and location of video parts

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
