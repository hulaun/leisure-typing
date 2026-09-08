//go:build windows

package tui

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// Windows console setup. Nothing in this app is Windows-only by nature; the
// console modes are the one part that is (CLAUDE.md, conventions).
//
// Two handles, two different sets of flags, and both must be set or the app
// misbehaves in a way that looks like a rendering bug:
//
//   - The output handle needs ENABLE_VIRTUAL_TERMINAL_PROCESSING, or every
//     escape sequence prints as literal garbage.
//   - The input handle needs ENABLE_VIRTUAL_TERMINAL_INPUT, which makes keys
//     arrive on stdin as the VT byte sequences Decode understands — and which
//     behave identically under ConPTY and conhost, unlike ReadConsoleInputW.
func enterRaw(in, out *os.File) (restore func(), err error) {
	hIn := windows.Handle(in.Fd())
	hOut := windows.Handle(out.Fd())

	var inMode, outMode uint32
	if err := windows.GetConsoleMode(hIn, &inMode); err != nil {
		return nil, fmt.Errorf("stdin is not a console (bt has to be run in a terminal, "+
			"not with its input redirected): %w", err)
	}
	if err := windows.GetConsoleMode(hOut, &outMode); err != nil {
		return nil, fmt.Errorf("stdout is not a console (bt has to be run in a terminal, "+
			"not piped): %w", err)
	}

	newIn := inMode &^ uint32(windows.ENABLE_ECHO_INPUT|
		windows.ENABLE_LINE_INPUT|
		windows.ENABLE_MOUSE_INPUT|
		// Clearing ENABLE_PROCESSED_INPUT stops Windows generating an
		// interrupt, so Ctrl+C simply arrives as the byte 0x03 and the app
		// owns quitting. Miss this and there is no way out.
		windows.ENABLE_PROCESSED_INPUT)
	newIn |= uint32(windows.ENABLE_VIRTUAL_TERMINAL_INPUT)

	newOut := outMode | uint32(windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING|
		windows.ENABLE_PROCESSED_OUTPUT)
	// Stop the console scrolling when a character lands in the bottom-right
	// cell, which would otherwise shift the whole frame up by a line.
	newOut |= uint32(windows.DISABLE_NEWLINE_AUTO_RETURN)

	if err := windows.SetConsoleMode(hIn, newIn); err != nil {
		return nil, fmt.Errorf("enabling VT input: %w", err)
	}
	if err := windows.SetConsoleMode(hOut, newOut); err != nil {
		windows.SetConsoleMode(hIn, inMode)
		return nil, fmt.Errorf("enabling VT output: %w", err)
	}

	return func() {
		windows.SetConsoleMode(hIn, inMode)
		windows.SetConsoleMode(hOut, outMode)
	}, nil
}

// size reads the console window — not the buffer, which on conhost is often
// far taller than the visible window.
//
// There is no SIGWINCH on Windows, so the reader polls this on a ticker
// (CLAUDE.md, gotcha 5). It is cheap, and because wrapping is a view, a resize
// invalidates nothing.
func size(out *os.File) (cols, rows int, err error) {
	var info windows.ConsoleScreenBufferInfo
	if err := windows.GetConsoleScreenBufferInfo(windows.Handle(out.Fd()), &info); err != nil {
		return 0, 0, err
	}
	cols = int(info.Window.Right-info.Window.Left) + 1
	rows = int(info.Window.Bottom-info.Window.Top) + 1
	return cols, rows, nil
}
