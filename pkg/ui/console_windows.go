//go:build windows

package ui

import (
	"os"
	"golang.org/x/sys/windows"
)

// EnableVirtualTerminalProcessing enables ANSI escape sequences on Windows 10/11 console.
func EnableVirtualTerminalProcessing() {
	handle := windows.Handle(os.Stdout.Fd())
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err == nil {
		mode |= windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
		_ = windows.SetConsoleMode(handle, mode)
	}

	errHandle := windows.Handle(os.Stderr.Fd())
	var errMode uint32
	if err := windows.GetConsoleMode(errHandle, &errMode); err == nil {
		errMode |= windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING
		_ = windows.SetConsoleMode(errHandle, errMode)
	}
}
