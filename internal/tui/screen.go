// Package tui owns the terminal: console modes, the alternate screen buffer,
// the frame buffer, and input decoding.
//
// Nothing outside this package writes to stdout. A stray fmt.Println in raw
// mode with the alternate buffer active corrupts the display in a way that
// looks like a rendering bug (CLAUDE.md, conventions).
package tui

import (
	"bytes"
	"fmt"
	"os"
	"strings"
)

// The ANSI vocabulary, in one place. Nothing else in the app hardcodes a
// colour (CLAUDE.md, conventions).
//
// This is the entire palette, and it is the only correctness signal in the
// app: dim for context, normal for typed-correct, red for typed-wrong, bright
// for untyped on the active line, and a caret.
var (
	Reset  = "\x1b[0m"
	Dim    = "\x1b[2;37m" // context lines above and below
	Normal = "\x1b[0;37m" // typed correctly
	Wrong  = "\x1b[1;31m" // typed wrongly — marked, never blocked on
	Bright = "\x1b[1;97m" // the untyped remainder of the active line
	Caret  = "\x1b[7m"    // reverse video: the cursor cell
	Rule   = "\x1b[2;90m" // the status rule
	Status = "\x1b[0;36m" // the status line
)

// Control sequences.
const (
	altEnter  = "\x1b[?1049h"
	altLeave  = "\x1b[?1049l"
	hideCur   = "\x1b[?25l"
	showCur   = "\x1b[?25h"
	clear     = "\x1b[2J"
	home      = "\x1b[H"
	clearLine = "\x1b[K"
)

// A Console is the terminal for as long as the app owns it.
type Console struct {
	In  *os.File
	Out *os.File

	restore func()
	stopped bool

	buf  bytes.Buffer // the frame under construction
	read []byte       // the stdin read buffer
	held []byte       // bytes read but not yet decoded into a whole key
}

// Start takes the terminal: VT on both handles, raw mode, the alternate
// screen buffer, cursor hidden.
//
// The alternate buffer is a feature of the product, not just hygiene — the app
// owns the whole terminal while it runs and leaves nothing in the scrollback
// afterwards.
//
// The caller must defer Stop, and must also recover from panics, or a crash
// leaves the user with a shell that has no echo and a frozen half-drawn page.
func Start() (*Console, error) {
	c := &Console{In: os.Stdin, Out: os.Stdout, read: make([]byte, 256)}

	restore, err := enterRaw(c.In, c.Out)
	if err != nil {
		return nil, err
	}
	c.restore = restore

	if _, err := c.Out.WriteString(altEnter + hideCur + clear + home); err != nil {
		restore()
		return nil, err
	}
	return c, nil
}

// Stop gives the terminal back. It is idempotent, because it is called from a
// defer and may also be called explicitly on the quit path.
//
// The alternate buffer and the console mode are restored together, in one
// place, so that no exit path can restore one and forget the other.
func (c *Console) Stop() {
	if c == nil || c.stopped {
		return
	}
	c.stopped = true
	c.Out.WriteString(showCur + altLeave + Reset)
	if c.restore != nil {
		c.restore()
	}
}

// Size returns the terminal's columns and rows, falling back to a usable
// default if the console cannot be measured.
func (c *Console) Size() (cols, rows int) {
	cols, rows, err := size(c.Out)
	if err != nil || cols <= 0 || rows <= 0 {
		return 80, 25
	}
	return cols, rows
}

// --- the frame buffer -------------------------------------------------------
//
// The whole frame is built here and emitted in a single write (CLAUDE.md,
// decision 3). VS Code's terminal goes through ConPTY, which is slow enough
// that a frame emitted as many small writes tears visibly. At 30x100 a full
// repaint is about 3 KB, so there is no dirty-region tracking: repaint
// everything, every keystroke, in one call.

// Begin starts a frame, clearing the buffer and homing the cursor.
func (c *Console) Begin() {
	c.buf.Reset()
	c.buf.WriteString(home)
}

// Write adds raw text to the frame.
func (c *Console) Write(s string) { c.buf.WriteString(s) }

// Writef adds formatted text to the frame.
func (c *Console) Writef(format string, a ...any) { fmt.Fprintf(&c.buf, format, a...) }

// Line adds a row and clears the rest of it, so the previous frame's longer
// line cannot leave a tail behind.
func (c *Console) Line(s string) {
	c.buf.WriteString(s)
	c.buf.WriteString(clearLine)
	c.buf.WriteString("\r\n")
}

// Blank adds an empty row.
func (c *Console) Blank() { c.Line("") }

// End emits the frame: one buffer, one write.
func (c *Console) End() error {
	c.buf.WriteString("\x1b[J") // clear anything below the last row drawn
	_, err := c.Out.Write(c.buf.Bytes())
	return err
}

// Pad returns s padded or truncated to exactly n display columns. Because
// normalization guarantees ASCII, one rune is one cell and this is just
// counting (CLAUDE.md, decision 2).
func Pad(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s + strings.Repeat(" ", n-len(r))
}

// Indent is the left margin that centres a column of the given width in the
// terminal.
func Indent(cols, width int) string {
	n := (cols - width) / 2
	if n < 0 {
		n = 0
	}
	return strings.Repeat(" ", n)
}

// --- input ------------------------------------------------------------------

// ReadKey blocks until a whole keystroke is available and returns it.
//
// Bytes that arrive as part of an incomplete escape sequence are held and
// prepended to the next read, so a sequence split across two reads decodes
// correctly rather than as a bare Esc — which would quit the app on an arrow
// key.
func (c *Console) ReadKey() (Key, error) {
	for {
		if k, n := Decode(c.held); n > 0 {
			c.held = c.held[n:]
			return k, nil
		}
		n, err := c.In.Read(c.read)
		if n > 0 {
			c.held = append(c.held, c.read[:n]...)
			continue
		}
		if err != nil {
			return Key{}, err
		}
	}
}

// Pending reports whether a decoded key is already waiting, without reading.
// The run loop uses it to coalesce a burst of fast typing into one repaint.
func (c *Console) Pending() bool {
	_, n := Decode(c.held)
	return n > 0
}
