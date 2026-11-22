package main

import (
	"os"
	"syscall"
	"unsafe"
)

// attachConsole connects a GUI-subsystem binary to the parent console for CLI commands.
func attachConsole() {
	if r, _, _ := syscall.NewLazyDLL("kernel32.dll").NewProc("AttachConsole").Call(^uintptr(0)); r == 0 { // ATTACH_PARENT_PROCESS
		return
	}
	if f, err := os.OpenFile("CONOUT$", os.O_WRONLY, 0); err == nil {
		os.Stdout, os.Stderr = f, f
	}
}

func showError(msg string) {
	t, _ := syscall.UTF16PtrFromString("vidpreset")
	m, _ := syscall.UTF16PtrFromString(msg)
	user32 := syscall.NewLazyDLL("user32.dll")
	user32.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x10)
}

func fatal(msg string) {
	showError(msg)
	if _, err := os.Stderr.Stat(); err == nil {
		os.Stderr.WriteString(msg + "\n")
	}
	os.Exit(1)
}
