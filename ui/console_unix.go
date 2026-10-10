//go:build !windows

package ui

import (
	"os"
	"syscall"
	"unsafe"
)

type winsize struct {
	rows, cols, xpixel, ypixel uint16
}

// Linux / macOS terminals already understand the codes, this only checks
// that the output really is a terminal (not a file or a pipe).
func initConsole() (restore func(), ok bool) {
	if _, _, ok := consoleSize(); !ok {
		return nil, false
	}
	return func() {}, true
}

// size of the terminal in characters
func consoleSize() (width, height int, ok bool) {
	var ws winsize
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, os.Stdout.Fd(),
		uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
	if errno != 0 || ws.cols == 0 || ws.rows == 0 {
		return 0, 0, false
	}
	return int(ws.cols), int(ws.rows), true
}
