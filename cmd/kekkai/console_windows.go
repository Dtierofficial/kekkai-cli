//go:build windows

package main

import (
	"os"

	"golang.org/x/sys/windows"
)

// SetupConsole enables virtual terminal processing on the output buffer so
// colors and the alt screen work in legacy conhost. Input is deliberately
// left untouched: Bubble Tea's own Windows input setup (coninput reader on
// stdin) is the path that was verified to survive focus loss, and any extra
// mode changes on the input handle (QuickEdit, VT input, a dedicated CONIN$
// handle) caused keys to die after Alt+Tab. The returned closure restores
// the original output mode.
func SetupConsole() (func(), error) {
	hOut := windows.Handle(os.Stdout.Fd())
	var origOut uint32
	if err := windows.GetConsoleMode(hOut, &origOut); err != nil {
		// Windows Terminal/ConPTY may expose stdout as a non-console
		// handle; nothing to configure in that case.
		return func() {}, nil
	}
	modeOut := uint32(windows.ENABLE_PROCESSED_OUTPUT | windows.ENABLE_WRAP_AT_EOL_OUTPUT | windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	_ = windows.SetConsoleMode(hOut, modeOut)
	return func() {
		_ = windows.SetConsoleMode(hOut, origOut)
	}, nil
}
