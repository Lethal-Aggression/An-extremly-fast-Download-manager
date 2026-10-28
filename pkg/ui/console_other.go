//go:build !windows

package ui

// EnableVirtualTerminalProcessing is a no-op on Unix-like systems where ANSI is native.
func EnableVirtualTerminalProcessing() {
	// Native ANSI terminal support
}
