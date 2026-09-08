//go:build !windows

package tui

import (
	"os"

	"golang.org/x/term"
)

// The POSIX path exists so the package builds and its tests run anywhere.
// Everything above this line in the package is portable; only the console
// mode handling is not, and on a terminal that is already VT there is nothing
// to enable.
func enterRaw(in, out *os.File) (restore func(), err error) {
	state, err := term.MakeRaw(int(in.Fd()))
	if err != nil {
		return nil, err
	}
	return func() { term.Restore(int(in.Fd()), state) }, nil
}

func size(out *os.File) (cols, rows int, err error) {
	return term.GetSize(int(out.Fd()))
}
