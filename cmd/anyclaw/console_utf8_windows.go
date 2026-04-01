//go:build windows

package main

import "syscall"

const utf8CodePage = 65001

var (
	kernel32               = syscall.NewLazyDLL("kernel32.dll")
	procSetConsoleCP       = kernel32.NewProc("SetConsoleCP")
	procSetConsoleOutputCP = kernel32.NewProc("SetConsoleOutputCP")
)

func configureConsoleUTF8Platform() {
	_, _, _ = procSetConsoleCP.Call(uintptr(utf8CodePage))
	_, _, _ = procSetConsoleOutputCP.Call(uintptr(utf8CodePage))
}
