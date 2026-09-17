package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hulaun/leisure-typing/internal/book"
	"github.com/hulaun/leisure-typing/internal/tui"
	"github.com/hulaun/leisure-typing/internal/wrap"
)

// The Place screen: where you are, and a field to type a new offset into.
//
// It exists so that the position can be carried between machines by hand. The
// reading position is a rune offset into text.txt and nothing else, so reading
// the number off one device and typing it into another is a complete sync,
// with no cable, no protocol and no clock. SPEC.md in this directory has why a
// jump is allowed when free cursor movement is not — the short version is that
// it lands on the head of a word, which is still the head of something unread,
// and that it is explicit rather than a caret drifting sideways.
//
// Typing a six-digit number by hand is the risk, and three things in the
// layout below are the answer to it rather than decoration:
//
//   - The preview. As digits arrive, the chapter, the percentage and the line
//     at that offset are drawn underneath. A dropped digit sends you to 4%
//     instead of 23% and you see it before pressing Enter. This is why there
//     is no check digit: a preview catches a typo *and* catches the number
//     having come from a different book, which a checksum cannot.
//   - The text hash. An offset is only meaningful against one exact text.txt.
//     Six characters of the sha256 on both screens is the whole safeguard;
//     without it a number from another copy resumes silently in the wrong
//     chapter.
//   - Undo. Jumping is the only thing in the app that can lose your place, so
//     it is the only thing with an undo.
type place struct {
	input string // the digits typed so far
}

const (
	// The Place screen is a form, not a page of prose: it is capped rather
	// than run to the full width of the terminal.
	placeWidth  = 64
	placeDigits = 12 // a book of 10^12 runes is not the case to support
)

// openPlace opens the screen, or closes it if it is already open.
func (r *reader) openPlace() {
	if r.place != nil {
		r.place = nil
		return
	}
	r.place = &place{}
}

// placeKey applies one keystroke to the Place screen and reports whether to
// quit the app. It is only called while the screen is open, and it consumes
// every key: nothing typed here reaches the book.
func (r *reader) placeKey(k tui.Key) bool {
	p := r.place

	switch k.Type {
	case tui.KeyCtrlC:
		return true

	case tui.KeyEsc, tui.KeyCtrlG:
		r.place = nil

	case tui.KeyEnter:
		if n, ok := p.offset(); ok {
			r.jumpTo(n)
		}
		r.place = nil

	case tui.KeyBackspace:
		if len(p.input) > 0 {
			p.input = p.input[:len(p.input)-1]
		}

	case tui.KeyCtrlZ:
		if r.hasPrev {
			// moveTo, not jumpTo: the position being restored was reached by
			// reading, so it is already valid and may legitimately sit on a
			// space. Snapping it would land the undo somewhere other than
			// where the jump started, which is the one thing undo must not
			// do. A second Ctrl+Z returns to the jump, since moveTo records
			// where it came from either way.
			r.moveTo(r.prevOffset)
			r.place = nil
		}

	case tui.KeyRune:
		// Commas, spaces, dots and underscores are accepted and dropped,
		// because the number is displayed grouped — 184,223 — and retyping
		// it with the comma is the natural thing to do.
		switch {
		case k.Rune >= '0' && k.Rune <= '9':
			if len(p.input) < placeDigits {
				p.input += string(k.Rune)
			}
		case k.Rune == ',' || k.Rune == ' ' || k.Rune == '_' || k.Rune == '.':
		}
	}
	return false
}

// offset is the number typed so far, and whether there is one.
func (p *place) offset() (int, bool) {
	if p.input == "" {
		return 0, false
	}
	n, err := strconv.Atoi(p.input)
	if err != nil {
		return 0, false
	}
	return n, true
}

// jumpTo moves the reading position to an offset that came from outside the
// reading — a number typed on the Place screen, or `bt pos <n>`.
//
// Such a number is arbitrary, so it is clamped and snapped to a word boundary
// before anything else happens. moveTo is the same move for a position that is
// already known good.
func (r *reader) jumpTo(target int) { r.moveTo(snapToWord(r.text, target)) }

// moveTo is the jump itself, for a target that is already a valid position.
//
// It records where it came from so the move can be undone. Everything before
// the new position is marked read and everything after it untyped, which is
// exactly what read() does when resuming: nothing was got wrong on either side
// of a jump, and backspacing back over it should not paint a screenful of red
// that was never typed.
func (r *reader) moveTo(target int) {
	if target < 0 {
		target = 0
	}
	if target > len(r.text) {
		target = len(r.text)
	}
	if target == r.offset {
		return
	}

	r.prevOffset, r.hasPrev = r.offset, true
	r.offset = target
	for i := range r.status {
		if i < r.offset {
			r.status[i] = correct
		} else {
			r.status[i] = untyped
		}
	}
	r.skipWhitespace()
	r.dirty = true
}

// snapToWord clamps an offset into the text and moves it to a word boundary.
//
// An arbitrary number lands mid-word most of the time, and beginning from the
// fourth letter of something is both unreadable and untypable. Inside a word
// it goes back to the head of that word; in the whitespace between words it
// goes forward to the start of the next one. Either way the position after the
// move is the head of something not yet read, which is the rule Up and Down
// keep as well.
func snapToWord(text []rune, off int) int {
	if off <= 0 {
		return 0
	}
	if off >= len(text) {
		return len(text)
	}
	if isPlaceSpace(text[off]) {
		for off < len(text) && isPlaceSpace(text[off]) {
			off++
		}
		return off
	}
	for off > 0 && !isPlaceSpace(text[off-1]) {
		off--
	}
	return off
}

func isPlaceSpace(r rune) bool { return r == ' ' || r == '\n' }

// --- rendering --------------------------------------------------------------

// renderPlace draws the screen. It replaces the page entirely rather than
// overlaying it: the page is the whole terminal, and a box drawn on top would
// have to solve what is underneath at every width.
func (r *reader) renderPlace() error {
	width := r.pageWidth()
	if width < minWidth || r.rows < 10 {
		return r.renderTooSmall()
	}
	if width > placeWidth {
		width = placeWidth
	}

	// The hint is the last row of placeRows and it owns the last row of the
	// frame, exactly as the status line does on the page. That row is written
	// unterminated: terminating it would scroll the terminal by one line and
	// leave the cursor in the pending-wrap state.
	rows := r.placeRows(width)
	body, hint := rows[:len(rows)-1], rows[len(rows)-1]

	avail := r.rows - 1
	if len(body) > avail {
		body = body[:avail]
	}
	top := (avail - len(body)) / 2
	if top < 0 {
		top = 0
	}
	indent := tui.Indent(r.cols, width)

	c := r.con
	c.Begin()
	for i := 0; i < top; i++ {
		c.Blank()
	}
	for _, row := range body {
		c.Line(indent + row)
	}
	for i := top + len(body); i < avail; i++ {
		c.Blank()
	}
	c.Write(indent + hint)
	return c.End()
}

// placeRows builds the screen as text, so the layout can be tested without a
// terminal. An empty string is a blank row, and the last row is the key hint,
// which the renderer puts on the bottom row of the frame.
func (r *reader) placeRows(width int) []string {
	rows := []string{
		tui.Bright + "Place" + tui.Reset,
		"",
	}

	// Where you are now: the same three facts the status line carries, plus
	// the raw offset and the hash, which are the two things you copy across.
	chapter, pct, words := progressAt(r.meta, len(r.text), r.offset)
	if chapter == "" {
		chapter = r.meta.Title
	}
	rows = append(rows,
		tui.Status+spread(chapter,
			fmt.Sprintf("%.0f%%   %s words", pct, comma(words)), width)+tui.Reset,
		tui.Normal+spread("now at "+comma(r.offset),
			"text "+shortHash(r.meta.SHA256), width)+tui.Reset,
		"",
	)

	// The field.
	typed := ""
	if n, ok := r.place.offset(); ok {
		typed = comma(n)
	}
	rows = append(rows,
		tui.Bright+"go to "+tui.Reset+tui.Normal+typed+tui.Reset+tui.Caret+" "+tui.Reset,
		"",
	)

	// The preview, which is the whole reason this is a screen and not a
	// prompt.
	rows = append(rows, r.previewRows(width)...)

	hint := "Enter go   Esc cancel"
	if r.hasPrev {
		hint += "   Ctrl+Z undo (" + comma(r.prevOffset) + ")"
	}
	rows = append(rows, "", tui.Dim+trim(hint, width)+tui.Reset)
	return rows
}

// previewRows are the two lines under the field: where the typed number lands,
// and the text there.
func (r *reader) previewRows(width int) []string {
	n, ok := r.place.offset()
	if !ok {
		return []string{
			tui.Dim + trim("type the offset shown on the other device", width) + tui.Reset,
			"",
		}
	}

	target := snapToWord(r.text, n)
	chapter, pct, _ := progressAt(r.meta, len(r.text), target)

	head := "lands at " + comma(target)
	if n > len(r.text) {
		head = "past the end; lands at " + comma(target)
	}
	if chapter != "" {
		head += ", in " + chapter
	}
	head += fmt.Sprintf(", %.0f%%", pct)

	// The line at that offset, drawn as the reader would draw it. This is the
	// sanity check that beats a checksum: you recognise the sentence, or you
	// do not.
	text := ""
	if len(r.text) > 0 && target < len(r.text) {
		lines, active := wrap.Window(r.text, target, width-2, 0, 0)
		text = strings.TrimSpace(lines[active].Text)
	}
	if text == "" {
		text = "(the end of the book)"
	}

	return []string{
		tui.Status + trim(head, width) + tui.Reset,
		tui.Bright + "  " + trim(text, width-2) + tui.Reset,
	}
}

// --- shared with the status line --------------------------------------------

// progressAt is the chapter, the percentage and the word count at an offset.
// The status line and the Place screen have to agree on these exactly, or the
// number you copy off one screen does not match what you check on the other.
func progressAt(m book.Meta, textLen, offset int) (chapter string, pct float64, words int) {
	if textLen > 0 {
		through := float64(offset) / float64(textLen)
		if through > 1 {
			through = 1
		}
		if through < 0 {
			through = 0
		}
		pct = through * 100
		// Estimated from the position rather than counted every frame.
		words = int(float64(m.Words) * through)
	}
	return m.ChapterAt(offset), pct, words
}

// shortHash is the prefix of the text hash shown on both devices. An offset is
// only meaningful against one exact text.txt, and six characters is enough to
// notice that two copies are not the same one.
func shortHash(sum string) string {
	if sum == "" {
		return "------"
	}
	if len(sum) > 6 {
		return sum[:6]
	}
	return sum
}

// spread puts left and right at the two ends of a row of the given width,
// truncating the left-hand side if they do not both fit.
func spread(left, right string, width int) string {
	gap := width - len([]rune(left)) - len([]rune(right))
	if gap < 1 {
		left = trim(left, maxInt(1, width-len([]rune(right))-2))
		gap = width - len([]rune(left)) - len([]rune(right))
		if gap < 1 {
			gap = 1
		}
	}
	return left + strings.Repeat(" ", gap) + right
}
