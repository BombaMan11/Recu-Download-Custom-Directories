//go:build windows

package ui

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode             = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode             = kernel32.NewProc("SetConsoleMode")
	procGetConsoleOutputCP         = kernel32.NewProc("GetConsoleOutputCP")
	procSetConsoleOutputCP         = kernel32.NewProc("SetConsoleOutputCP")
	procGetConsoleScreenBufferInfo = kernel32.NewProc("GetConsoleScreenBufferInfo")
)

const (
	enableProcessedOutput           = 0x0001
	enableVirtualTerminalProcessing = 0x0004
	codePageUTF8                    = 65001
)

type coord struct{ x, y int16 }

type smallRect struct{ left, top, right, bottom int16 }

type consoleScreenBufferInfo struct {
	size              coord
	cursorPosition    coord
	attributes        uint16
	window            smallRect
	maximumWindowSize coord
}

// Makes the console understand color / cursor codes and UTF-8 text.
// Returns false if the output is not a console (redirected to a file, ...)
// or it refuses, restore puts the console back the way it was.
func initConsole() (restore func(), ok bool) {
	handle := os.Stdout.Fd()
	var mode uint32
	r, _, _ := procGetConsoleMode.Call(handle, uintptr(unsafe.Pointer(&mode)))
	if r == 0 {
		return nil, false
	}
	r, _, _ = procSetConsoleMode.Call(handle, uintptr(mode|enableProcessedOutput|enableVirtualTerminalProcessing))
	if r == 0 {
		return nil, false
	}
	oldCP, _, _ := procGetConsoleOutputCP.Call()
	procSetConsoleOutputCP.Call(codePageUTF8)
	return func() {
		procSetConsoleMode.Call(handle, uintptr(mode))
		if oldCP != 0 {
			procSetConsoleOutputCP.Call(oldCP)
		}
	}, true
}

// size of the visible console window in characters
func consoleSize() (width, height int, ok bool) {
	var info consoleScreenBufferInfo
	r, _, _ := procGetConsoleScreenBufferInfo.Call(os.Stdout.Fd(), uintptr(unsafe.Pointer(&info)))
	if r == 0 {
		return 0, 0, false
	}
	width = int(info.window.right-info.window.left) + 1
	height = int(info.window.bottom-info.window.top) + 1
	return width, height, width > 0 && height > 0
}
