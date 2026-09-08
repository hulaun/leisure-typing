package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/hulaun/leisure-typing/internal/book"
	"github.com/hulaun/leisure-typing/internal/progress"
	"github.com/hulaun/leisure-typing/internal/tui"
	"github.com/hulaun/leisure-typing/internal/wrap"
)

// Geometry, in one place. Nothing else hardcodes a width (CLAUDE.md,
// conventions).
const (
	wrapWidth    = 68 // the reading column, centred in the terminal
	contextLines = 3  // dimmed lines above and below the active one
	minWidth     = 20 // below this the terminal is unusable

	saveDebounce = 2 * time.Second
	resizePoll   = 100 * time.Millisecond // there is no SIGWINCH on Windows
)

// The state of each character the reader has passed.
type state uint8

const (
	untyped state = iota
	correct
	wrong
)

type reader struct {
	store *book.Store
	meta  book.Meta

	text   []rune
	status []state
	offset int

	con  *tui.Console
	cols int
	rows int

	dirty     bool // position changed since the last save
	lastSaved time.Time
}

// read opens the current book and runs until the user quits.
func read(store *book.Store) (err error) {
	m, err := store.Current()
	if err != nil {
		return err
	}
	text, err := store.Text(m)
	if err != nil {
		return err
	}

	p, err := progress.Load(store.ProgressPath(m.ID), m.SHA256)
	if err != nil {
		return err
	}
	if p.Offset > len(text) {
		p.Offset = len(text)
	}

	r := &reader{
		store:  store,
		meta:   m,
		text:   text,
		status: make([]state, len(text)),
		offset: p.Offset,
	}
	// Everything before the saved position was typed in an earlier session.
	// It renders as context either way, but marking it keeps backspacing
	// across the resume point from showing a screenful of false errors.
	for i := 0; i < r.offset; i++ {
		r.status[i] = correct
	}
	r.skipWhitespace()

	con, err := tui.Start()
	if err != nil {
		return err
	}

	// Every exit path restores the console mode and leaves the alternate
	// buffer, including a panic — a shell with no echo looks like the shell
	// is broken rather than like this app crashed, and it would happen at
	// work (CLAUDE.md, gotchas 4 and 7).
	defer func() {
		if p := recover(); p != nil {
			con.Stop()
			r.save()
			logf(store, "panic: %v\n%s", p, debug.Stack())
			err = fmt.Errorf("bt crashed; the details are in %s", logPath(store))
		}
	}()
	defer func() {
		con.Stop()
		// The position is saved unconditionally on the way out. Losing ten
		// minutes of a five-week book is the worst bug this app can have.
		if serr := r.save(); serr != nil && err == nil {
			err = serr
		}
	}()

	r.con = con
	return r.run()
}

func (r *reader) run() error {
	r.cols, r.rows = r.con.Size()
	if err := r.render(); err != nil {
		return err
	}

	// Keys are read on their own goroutine so that the resize poll and the
	// save debounce have somewhere to happen.
	keys := make(chan tui.Key, 64)
	errs := make(chan error, 1)
	go func() {
		for {
			k, err := r.con.ReadKey()
			if err != nil {
				errs <- err
				return
			}
			keys <- k
		}
	}()

	resize := time.NewTicker(resizePoll)
	defer resize.Stop()
	save := time.NewTicker(saveDebounce)
	defer save.Stop()

	for {
		select {
		case k := <-keys:
			if quit := r.key(k); quit {
				return nil
			}
			// Coalesce a burst of fast typing into one repaint: if more keys
			// are already queued, handle them before drawing.
			for drained := false; !drained; {
				select {
				case k := <-keys:
					if quit := r.key(k); quit {
						return nil
					}
				default:
					drained = true
				}
			}
			if err := r.render(); err != nil {
				return err
			}

		case <-resize.C:
			// Polling, because there is no SIGWINCH on Windows. Cheap,
			// because wrapping is a view and a resize invalidates nothing.
			if cols, rows := r.con.Size(); cols != r.cols || rows != r.rows {
				r.cols, r.rows = cols, rows
				if err := r.render(); err != nil {
					return err
				}
			}

		case <-save.C:
			if r.dirty {
				if err := r.save(); err != nil {
					return err
				}
			}

		case err := <-errs:
			return err
		}
	}
}

// key applies one keystroke and reports whether to quit.
func (r *reader) key(k tui.Key) bool {
	switch k.Type {
	case tui.KeyEsc, tui.KeyCtrlC:
		return true

	case tui.KeyBackspace:
		r.back()

	case tui.KeyRune:
		r.typeRune(k.Rune)

	case tui.KeyTab, tui.KeyEnter:
		// The text has no tabs, and newlines are stepped over rather than
		// typed, so neither key has anything to do. Ignored rather than
		// counted wrong: it is not a mistake in the text.

	default:
		// Arrows, Home, the Page keys. Moving the caret freely would let the
		// position drift away from what has actually been read, so they do
		// nothing. Reading forwards is what advances the book.
	}
	return false
}

// typeRune is the whole of the typing rule.
//
// A wrong character is marked red and the caret advances anyway. It is not
// blocked on, and the wrong character is never inserted into the text: the
// line stays aligned and the wrapping stays stable, so the eye is not yanked
// out of the sentence to fix a character nobody will read again.
func (r *reader) typeRune(got rune) {
	if r.offset >= len(r.text) {
		return
	}
	if got == r.text[r.offset] {
		r.status[r.offset] = correct
	} else {
		r.status[r.offset] = wrong
	}
	r.offset++
	r.skipWhitespace()
	r.dirty = true
}

// back steps one character and forgets what was typed there.
func (r *reader) back() {
	if r.offset == 0 {
		return
	}
	r.offset--
	// Step back over the newlines that were auto-advanced, so backspace at
	// the head of a paragraph lands on the last real character of the one
	// before it rather than in the gap.
	for r.offset > 0 && r.text[r.offset] == '\n' {
		r.offset--
	}
	r.status[r.offset] = untyped
	r.dirty = true
}

// skipWhitespace steps over line and paragraph breaks. They are structure,
// not text to be typed — nobody wants to press Enter twice between paragraphs.
func (r *reader) skipWhitespace() {
	for r.offset < len(r.text) && r.text[r.offset] == '\n' {
		r.offset++
	}
}

func (r *reader) save() error {
	if !r.dirty && !r.lastSaved.IsZero() {
		return nil
	}
	err := progress.Save(r.store.ProgressPath(r.meta.ID), progress.Progress{
		Offset: r.offset,
		SHA256: r.meta.SHA256,
	})
	if err != nil {
		return err
	}
	r.dirty = false
	r.lastSaved = time.Now()
	return nil
}

// --- rendering --------------------------------------------------------------

func (r *reader) render() error {
	width := wrapWidth
	if width > r.cols-4 {
		width = r.cols - 4
	}
	if width < minWidth {
		return r.renderTooSmall()
	}

	lines, active := wrap.Window(r.text, r.offset, width, contextLines, contextLines)
	lo := active - contextLines
	if lo < 0 {
		lo = 0
	}
	hi := active + contextLines
	if hi > len(lines)-1 {
		hi = len(lines) - 1
	}
	visible := lines[lo : hi+1]

	// The page has to fit above the rule and the status line. On a short
	// terminal, drop context rather than write more rows than the screen has:
	// the terminal would scroll, and a scrolled frame tears in a way that
	// looks exactly like a rendering bug.
	avail := r.rows - 2
	if avail < 1 {
		return r.renderTooSmall()
	}
	if len(visible) > avail {
		start := (active - lo) - avail/2 // keep the active line centred
		if start < 0 {
			start = 0
		}
		if start+avail > len(visible) {
			start = len(visible) - avail
		}
		lo += start
		visible = visible[start : start+avail]
	}

	indent := tui.Indent(r.cols, width)
	// Centre the page vertically, leaving the last two rows for the rule and
	// the status line.
	top := (r.rows - 2 - len(visible)) / 2
	if top < 0 {
		top = 0
	}

	c := r.con
	c.Begin()
	for i := 0; i < top; i++ {
		c.Blank()
	}
	for i, l := range visible {
		switch {
		case l.Blank:
			c.Blank()
		case lo+i == active:
			c.Line(indent + r.activeLine(l))
		default:
			c.Line(indent + tui.Dim + l.Text + tui.Reset)
		}
	}
	for i := top + len(visible); i < r.rows-2; i++ {
		c.Blank()
	}
	c.Line(indent + tui.Rule + strings.Repeat("-", width) + tui.Reset)
	c.Write(indent + r.statusLine(width))
	return c.End()
}

// activeLine draws the one bright row: what has been typed, in normal weight
// or red where it was wrong, a caret, and the rest of the line ahead of it.
func (r *reader) activeLine(l wrap.Line) string {
	runes := []rune(l.Text)

	// Offsets past the end of the drawn text are the whitespace the break
	// consumed. It still has to be typed, so it gets one cell at the end of
	// the line for the caret to sit in.
	cells := make([]rune, len(runes))
	copy(cells, runes)
	if l.Start+len(runes) < l.End {
		cells = append(cells, ' ')
	}

	var b strings.Builder
	for i, ch := range cells {
		off := l.Start + i
		next := off + 1
		if i == len(cells)-1 {
			next = l.End // the last cell absorbs any remaining whitespace
		}

		switch {
		case r.offset >= off && r.offset < next:
			b.WriteString(tui.Caret)
			b.WriteRune(ch)
			b.WriteString(tui.Reset)
		case off < r.offset:
			if r.status[off] == wrong {
				b.WriteString(tui.Wrong)
			} else {
				b.WriteString(tui.Normal)
			}
			b.WriteRune(ch)
			b.WriteString(tui.Reset)
		default:
			b.WriteString(tui.Bright)
			b.WriteRune(ch)
			b.WriteString(tui.Reset)
		}
	}
	return b.String()
}

// statusLine carries the one number that matters: progress through the book.
// That is the whole reward loop — nobody types 40,000 words without being able
// to see it moving.
func (r *reader) statusLine(width int) string {
	pct := 0.0
	words := 0
	if len(r.text) > 0 {
		through := float64(r.offset) / float64(len(r.text))
		pct = through * 100
		// Estimated from the position rather than counted every frame.
		words = int(float64(r.meta.Words) * through)
	}

	right := fmt.Sprintf("%.0f%%   %s words", pct, comma(words))
	left := r.meta.ChapterAt(r.offset)
	if left == "" {
		left = r.meta.Title
	}

	gap := width - len([]rune(right)) - len([]rune(left))
	if gap < 1 {
		left = trim(left, maxInt(1, width-len([]rune(right))-2))
		gap = width - len([]rune(right)) - len([]rune(left))
		if gap < 1 {
			gap = 1
		}
	}
	return tui.Status + left + strings.Repeat(" ", gap) + right + tui.Reset
}

func (r *reader) renderTooSmall() error {
	c := r.con
	c.Begin()
	for i := 0; i < r.rows/2; i++ {
		c.Blank()
	}
	c.Write(tui.Dim + " the terminal is too narrow to read in " + tui.Reset)
	return c.End()
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// --- the error log ----------------------------------------------------------
//
// Nothing may print to the terminal while the alternate buffer is up, so
// errors go to a file (CLAUDE.md, conventions).

func logPath(store *book.Store) string { return filepath.Join(store.Root, "bt.log") }

func logf(store *book.Store, format string, a ...any) {
	f, err := os.OpenFile(logPath(store), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s  ", time.Now().Format(time.RFC3339))
	fmt.Fprintf(f, format, a...)
	fmt.Fprintln(f)
}
