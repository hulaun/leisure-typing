package main

import "github.com/hulaun/leisure-typing/internal/wrap"

// Moving by a whole display line, on Up and Down.
//
// This is the one place the caret moves without the text being typed, and it
// is not a step towards free cursor movement — Left, Right and Home still do
// nothing, because a caret loose inside a line lets the saved position drift
// away from what has actually been read. A line is different. It is a unit of
// the page: skipping one means "I have read this line without typing it", and
// the position after the move is still the head of something not yet read.
//
// It exists because typing is what holds the attention, but an hour of it
// tires the hands long before the reading stops being wanted, and the
// alternative to a way down the page is closing the book. Down keeps the one
// number that matters moving; without it the reward loop stops the moment the
// hands do.
//
// Note what this is not: it does not measure, block, or score anything, and
// there is still no end screen. Skipped text is marked correct rather than
// left untyped, for the same reason the runes before a resumed position are —
// nothing was got wrong there, and backspacing back over it should not paint
// a screenful of red that was never typed.

// pageWidth is the wrap width for the current terminal.
//
// The active line draws one cell past its text: the caret has to have
// somewhere to sit when the next thing to type is the whitespace a break
// consumed. Leave a column for it, or a full-width line runs one past the
// edge, wraps, and pushes the whole frame down a row. It also keeps the last
// row from filling its final cell, which would leave the cursor in the
// terminal's pending-wrap state.
//
// The renderer and the line movement have to agree on this exactly, or Down
// lands somewhere other than the line the reader can see below the caret.
func (r *reader) pageWidth() int {
	width := r.cols
	if wrapWidth > 0 && wrapWidth < width {
		width = wrapWidth
	}
	if width >= r.cols {
		width = r.cols - 1
	}
	return width
}

// lineDown moves to the head of the next display line, counting everything
// stepped over as read.
func (r *reader) lineDown() {
	width := r.pageWidth()
	if width < minWidth || r.offset >= len(r.text) {
		return
	}

	// One line of context each way is all this needs; the window is exact,
	// so these are the same lines the renderer draws.
	lines, active := wrap.Window(r.text, r.offset, width, 0, 1)

	// The end of the book if there is no next line: Down on the last line
	// finishes it rather than doing nothing.
	target := len(r.text)
	for _, l := range lines[active+1:] {
		if l.Blank {
			continue // the gap between paragraphs is not a line to land on
		}
		target = l.Start
		break
	}
	if target <= r.offset {
		return
	}
	for i := r.offset; i < target; i++ {
		r.status[i] = correct
	}
	r.offset = target
	r.skipWhitespace()
	r.dirty = true
}

// lineUp moves back a line, so a line can be re-read or retyped.
//
// From the middle of a line it goes to the head of that line, which is what
// makes Up undo a Down exactly. What it steps back over is forgotten, the
// same as backspace: the line can be typed again from the start.
func (r *reader) lineUp() {
	width := r.pageWidth()
	if width < minWidth || r.offset == 0 {
		return
	}

	lines, active := wrap.Window(r.text, r.offset, width, 1, 0)
	target := lines[active].Start
	if target >= r.offset {
		// Already at the head of the line: take the one above it. Zero if
		// there is none, which is the first line of the book.
		target = 0
		for i := active - 1; i >= 0; i-- {
			if lines[i].Blank {
				continue
			}
			target = lines[i].Start
			break
		}
	}
	if target >= r.offset {
		return
	}
	for i := target; i < r.offset; i++ {
		r.status[i] = untyped
	}
	r.offset = target
	r.skipWhitespace()
	r.dirty = true
}
