//go:build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

const utf8CodePage = 65001

const (
	enableVirtualTerminalProcessing = 0x0004
	enableProcessedOutput           = 0x0001
	enableWrapAtEolOutput           = 0x0002
)

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleCP       = kernel32.NewProc("SetConsoleCP")
	procSetConsoleOutputCP = kernel32.NewProc("SetConsoleOutputCP")
	procGetConsoleMode     = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode     = kernel32.NewProc("SetConsoleMode")
)

func configureConsoleUTF8Platform() {
	r, _, err := procSetConsoleCP.Call(uintptr(utf8CodePage))
	if r == 0 {
		printWarn("SetConsoleCP failed: %v (console may not support full UTF-8)", err)
	}

	r, _, err = procSetConsoleOutputCP.Call(uintptr(utf8CodePage))
	if r == 0 {
		printWarn("SetConsoleOutputCP failed: %v (console may not support full UTF-8)", err)
	}

	enableANSIColors(os.Stdout.Fd())
	enableANSIColors(os.Stderr.Fd())
}

func enableANSIColors(fd uintptr) {
	var mode uint32
	r, _, _ := procGetConsoleMode.Call(fd, uintptr(unsafe.Pointer(&mode)))
	if r == 0 {
		return
	}
	newMode := mode | enableVirtualTerminalProcessing | enableWrapAtEolOutput
	if newMode != mode {
		_, _, _ = procSetConsoleMode.Call(fd, uintptr(newMode))
	}
}
