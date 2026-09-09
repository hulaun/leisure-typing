package main

import (
	"os"
	"strings"
	"testing"

	"github.com/hulaun/leisure-typing/internal/book"
	"github.com/hulaun/leisure-typing/internal/tui"
	"github.com/hulaun/leisure-typing/internal/wrap"
)

// A reader over a text, with no terminal attached. The typing rule and the
// line rendering are both pure enough to test this way, which matters: they
// are the parts most likely to be "improved" into a typing test later.
func newReader(text string) *reader {
	r := &reader{
		text:   []rune(text),
		status: make([]state, len([]rune(text))),
		meta:   book.Meta{Title: "A Book", Words: 100},
	}
	r.skipWhitespace()
	return r
}

func (r *reader) typeString(s string) {
	for _, ch := range s {
		r.typeRune(ch)
	}
}

func TestTypingAdvances(t *testing.T) {
	r := newReader("abc def\n")
	r.typeString("abc")
	if r.offset != 3 {
		t.Errorf("offset = %d, want 3", r.offset)
	}
	for i := 0; i < 3; i++ {
		if r.status[i] != correct {
			t.Errorf("status[%d] = %v, want correct", i, r.status[i])
		}
	}
}

// The rule the whole app turns on: a wrong character is marked and the caret
// moves on. It is not blocked on, and the wrong character is not inserted.
func TestWrongCharacterDoesNotBlock(t *testing.T) {
	r := newReader("abcdef\n")
	r.typeString("axc")
	if r.offset != 3 {
		t.Fatalf("offset = %d, want 3 — a wrong character blocked the caret", r.offset)
	}
	if r.status[1] != wrong {
		t.Errorf("status[1] = %v, want wrong", r.status[1])
	}
	if r.status[2] != correct {
		t.Errorf("status[2] = %v: typing continued out of alignment", r.status[2])
	}
	// The text itself is untouched: the line stays aligned and the wrapping
	// stays stable.
	if string(r.text) != "abcdef\n" {
		t.Errorf("the text was modified: %q", string(r.text))
	}
}

// Newlines are structure, not text to type: nobody presses Enter twice
// between paragraphs.
func TestNewlinesAreSteppedOver(t *testing.T) {
	r := newReader("ab\n\ncd\n")
	r.typeString("ab")
	if r.offset != 4 {
		t.Errorf("offset = %d, want 4 (the paragraph break was skipped)", r.offset)
	}
	if got := r.text[r.offset]; got != 'c' {
		t.Errorf("next character is %q, want 'c'", got)
	}
}

// Leading whitespace at the very start is skipped too, or the reader opens on
// a caret sitting in a blank line.
func TestLeadingNewlinesSkipped(t *testing.T) {
	if r := newReader("\n\nabc\n"); r.offset != 2 {
		t.Errorf("offset = %d, want 2", r.offset)
	}
}

func TestBackspace(t *testing.T) {
	r := newReader("abc\n")
	r.typeString("axc")
	r.back()
	if r.offset != 2 {
		t.Errorf("offset = %d, want 2", r.offset)
	}
	if r.status[2] != untyped {
		t.Errorf("status[2] = %v, want untyped", r.status[2])
	}
	// Back over the wrong character, which clears the mark.
	r.back()
	if r.offset != 1 || r.status[1] != untyped {
		t.Errorf("offset = %d, status[1] = %v", r.offset, r.status[1])
	}
}

// Backspacing at the head of a paragraph lands on the last real character of
// the one before, not in the gap.
func TestBackspaceAcrossParagraph(t *testing.T) {
	r := newReader("ab\n\ncd\n")
	r.typeString("ab")
	r.back()
	if got := r.text[r.offset]; got != 'b' {
		t.Errorf("landed on %q, want 'b'", got)
	}
	if r.status[r.offset] != untyped {
		t.Error("the character stepped back onto is still marked")
	}
}

func TestBackspaceAtStartIsHarmless(t *testing.T) {
	r := newReader("abc\n")
	r.back()
	if r.offset != 0 {
		t.Errorf("offset = %d, want 0", r.offset)
	}
}

func TestTypingPastTheEndIsHarmless(t *testing.T) {
	r := newReader("ab\n")
	r.typeString("abcdef")
	if r.offset != len(r.text) {
		t.Errorf("offset = %d, text is %d runes", r.offset, len(r.text))
	}
}

func TestQuitKeys(t *testing.T) {
	r := newReader("abc\n")
	for _, k := range []tui.KeyType{tui.KeyEsc, tui.KeyCtrlC} {
		if !r.key(tui.Key{Type: k}) {
			t.Errorf("key %v did not quit", k)
		}
	}
	// Enter and the arrows do nothing at all, and in particular do not count
	// as a wrong character.
	for _, k := range []tui.KeyType{tui.KeyEnter, tui.KeyTab, tui.KeyUp, tui.KeyLeft, tui.KeyHome} {
		if r.key(tui.Key{Type: k}) {
			t.Errorf("key %v quit the reader", k)
		}
		if r.offset != 0 {
			t.Fatalf("key %v moved the caret to %d", k, r.offset)
		}
	}
}

// The active line carries the whole palette, and a wrong character must come
// out red rather than merely uncoloured.
func TestActiveLineColours(t *testing.T) {
	r := newReader("abcdef\n")
	r.typeString("axc")
	lines, active := wrap.Window(r.text, r.offset, 20, 3, 3)
	out := r.activeLine(lines[active])

	// The cell shows the character that *should* have been typed, in red —
	// the wrong one is never inserted, so the line stays aligned.
	if !strings.Contains(out, tui.Wrong+"b") {
		t.Errorf("the wrong character is not red:\n%q", out)
	}
	if strings.Contains(stripANSI(out), "x") {
		t.Errorf("the wrongly typed character was inserted into the line:\n%q", out)
	}
	if !strings.Contains(out, tui.Normal+"a") {
		t.Errorf("a correct character is not in normal weight:\n%q", out)
	}
	if !strings.Contains(out, tui.Caret+"d") {
		t.Errorf("the caret is not on the next character:\n%q", out)
	}
	if !strings.Contains(out, tui.Bright+"e") {
		t.Errorf("the untyped remainder is not bright:\n%q", out)
	}
	// Every character of the line still appears, in order.
	// The trailing cell is the whitespace the break consumed; it is where
	// the caret sits when the last character of the line has been typed.
	if got := stripANSI(out); got != "abcdef " {
		t.Errorf("the line reads %q, want %q", got, "abcdef ")
	}
}

// The status line is the reward loop, and it must fit the column exactly.
func TestStatusLineFitsAndShowsProgress(t *testing.T) {
	r := newReader(strings.Repeat("word ", 200) + "\n")
	r.offset = len(r.text) / 4

	line := stripANSI(r.statusLine(68))
	if n := len([]rune(line)); n != 68 {
		t.Errorf("status line is %d columns, want 68: %q", n, line)
	}
	if !strings.Contains(line, "25%") {
		t.Errorf("no percentage in %q", line)
	}
	// No timer, no words per minute, no score. This is a reader.
	for _, forbidden := range []string{"wpm", "WPM", "accuracy", "elapsed"} {
		if strings.Contains(line, forbidden) {
			t.Errorf("the status line reports %q — this is not a typing test", forbidden)
		}
	}
}

// A very long chapter title must not push the status line over the column.
func TestStatusLineTruncatesLongTitles(t *testing.T) {
	r := newReader("abc\n")
	r.meta.Title = strings.Repeat("a very long chapter title ", 5)
	if n := len([]rune(stripANSI(r.statusLine(68)))); n > 68 {
		t.Errorf("status line is %d columns, want at most 68", n)
	}
}

// stripANSI removes escape sequences so a rendered line can be measured and
// read as text.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b {
			for i < len(s) && !(s[i] >= 0x40 && s[i] <= 0x7e && i > 0 && s[i-1] != 0x1b) {
				i++
			}
			i++
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// renderFrame draws one frame into a file and returns the bytes written. The
// Console's fields are enough to build one without a terminal, which is what
// lets the frame itself be asserted on.
func renderFrame(t *testing.T, r *reader, cols, rows int) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "frame-*")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	r.con = &tui.Console{Out: f}
	r.cols, r.rows = cols, rows
	if err := r.render(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The reported bug, at the level it was seen: the first line of a paragraph
// appeared twice on screen.
//
// The cause was a newline inside a display line. Every row of a frame is
// terminated by the renderer with "\r\n"; a bare "\n" anywhere else is a row
// the renderer did not intend, splitting one line across two and pushing
// everything below it down.
func TestFrameHasNoStrayNewlines(t *testing.T) {
	text := "First paragraph, which is long enough to wrap once or twice " +
		"at the widths a terminal actually has.\n\n" +
		"Second paragraph, the one whose first line came out twice.\n\n" +
		"Third paragraph, so there is context below as well.\n"

	// Type into the second paragraph, so it is the active one.
	r := newReader(text)
	for i := 0; i < 130; i++ {
		r.typeRune(r.text[r.offset])
	}

	for _, size := range [][2]int{{100, 30}, {80, 24}, {72, 20}} {
		frame := renderFrame(t, r, size[0], size[1])
		if got, want := strings.Count(frame, "\n"), strings.Count(frame, "\r\n"); got != want {
			t.Errorf("%dx%d: frame has %d newlines but only %d row terminators — "+
				"%d stray newline(s) split a row", size[0], size[1], got, want, got-want)
		}
	}
}

// The same bug stated as what the user saw: no line of the book is drawn on
// two rows of the frame.
func TestNoLineIsDrawnTwice(t *testing.T) {
	text := "First paragraph here, with enough words in it to be recognisable.\n\n" +
		"Second paragraph starts now and carries on for a little while.\n\n" +
		"Third paragraph, for the context below.\n"

	r := newReader(text)
	for i := 0; i < 70; i++ {
		r.typeRune(r.text[r.offset])
	}

	frame := stripANSI(renderFrame(t, r, 100, 30))
	rows := strings.Split(frame, "\r\n")

	seen := map[string]int{}
	for _, row := range rows {
		if row = strings.TrimSpace(row); row != "" {
			seen[row]++
		}
	}
	for row, n := range seen {
		if n > 1 {
			t.Errorf("the row %q is drawn %d times in one frame", row, n)
		}
	}
}

// A frame is exactly as tall as the terminal. One row too many and the
// terminal scrolls, which tears the display and looks like a rendering bug.
func TestFrameIsExactlyTerminalHeight(t *testing.T) {
	r := newReader(strings.Repeat("A paragraph of a reasonable length.\n\n", 20))
	for i := 0; i < 200; i++ {
		r.typeRune(r.text[r.offset])
	}

	for _, size := range [][2]int{{100, 30}, {80, 24}, {90, 12}, {80, 9}, {80, 8}, {80, 6}} {
		frame := renderFrame(t, r, size[0], size[1])
		// Every row but the last is terminated; the last deliberately is not,
		// or writing it would scroll the terminal by one line.
		if got, want := strings.Count(frame, "\r\n"), size[1]-1; got != want {
			t.Errorf("%dx%d: frame has %d terminated rows, want %d",
				size[0], size[1], got, want)
		}
		if strings.HasSuffix(stripANSI(frame), "\r\n") {
			t.Errorf("%dx%d: the last row is terminated, which scrolls the terminal",
				size[0], size[1])
		}
	}
}

// The page uses the whole terminal, in both directions. This replaced a
// 68-column measure centred in the terminal, on request; see the note on the
// geometry constants.
func TestPageFillsTheTerminal(t *testing.T) {
	// One long paragraph, so there are no blank separator rows to muddy the
	// count of how much of the screen is page.
	r := newReader(strings.Repeat(
		"Prose that runs on for a good while and wraps at any sensible width. ", 300) + "\n")
	for i := 0; i < 900; i++ {
		r.typeRune(r.text[r.offset])
	}

	for _, size := range [][2]int{{100, 30}, {120, 40}, {80, 24}} {
		cols, rowCount := size[0], size[1]
		frame := stripANSI(renderFrame(t, r, cols, rowCount))
		rows := strings.Split(frame, "\r\n")

		if len(rows) != rowCount {
			t.Fatalf("%dx%d: frame has %d rows, want %d", cols, rowCount, len(rows), rowCount)
		}

		// Every row above the rule is page, with nothing left blank.
		for i := 0; i < rowCount-2; i++ {
			if strings.TrimSpace(rows[i]) == "" {
				t.Errorf("%dx%d: row %d is blank; the page does not fill the height",
					cols, rowCount, i+1)
			}
		}

		// The text uses the full width: wider than the old 68-column measure,
		// and never wider than the terminal.
		widest := 0
		for i := 0; i < rowCount-2; i++ {
			if n := len([]rune(rows[i])); n > widest {
				widest = n
			}
		}
		if widest <= 68 {
			t.Errorf("%dx%d: the widest row is %d columns; the page is not using the width",
				cols, rowCount, widest)
		}
		if widest > cols {
			t.Errorf("%dx%d: a row is %d columns, wider than the terminal",
				cols, rowCount, widest)
		}

		// The last row must not fill its final cell: that leaves the cursor
		// in the terminal's pending-wrap state, and the next write scrolls.
		if n := len([]rune(rows[rowCount-1])); n >= cols {
			t.Errorf("%dx%d: the status row is %d columns and fills the last cell",
				cols, rowCount, n)
		}
	}
}

// At the very start of a book there is nothing above to show, so the page
// fills downwards instead of sitting half empty in the middle.
func TestPageFillsFromTheTopAtTheStart(t *testing.T) {
	r := newReader(strings.Repeat("A sentence of perfectly ordinary length goes here. ", 200) + "\n")

	frame := stripANSI(renderFrame(t, r, 100, 24))
	rows := strings.Split(frame, "\r\n")
	if strings.TrimSpace(rows[0]) == "" {
		t.Errorf("the first row is blank at the start of a book:\n%s", frame)
	}
	for i := 0; i < 22; i++ {
		if strings.TrimSpace(rows[i]) == "" {
			t.Errorf("row %d is blank; the page did not fill downwards", i+1)
		}
	}
}

// --- moving by a line -------------------------------------------------------
//
// Up and Down are the one way the caret moves without the text being typed.
// The tests hold them to the shape that makes that safe: a move lands on the
// head of a display line, never inside one, and Up undoes a Down exactly.

// nav returns a reader at a known width, with the lines it will be moving
// over. The width is the reader's own, so the test and the app agree on where
// the breaks fall.
func nav(t *testing.T, text string, cols int) (*reader, []wrap.Line) {
	t.Helper()
	r := newReader(text)
	r.cols = cols
	return r, wrap.Lines(r.text, r.pageWidth())
}

const navText = "alpha bravo charlie delta echo foxtrot golf hotel india\n\n" +
	"juliett kilo lima mike november oscar papa quebec romeo\n"

func TestLineDownLandsOnTheNextLine(t *testing.T) {
	r, lines := nav(t, navText, 21)
	if len(lines) < 3 {
		t.Fatalf("the fixture wraps to %d lines; the test needs at least 3", len(lines))
	}
	r.lineDown()
	if r.offset != lines[1].Start {
		t.Errorf("offset = %d, want %d (the head of the second line)", r.offset, lines[1].Start)
	}
	r.lineDown()
	if r.offset != lines[2].Start {
		t.Errorf("offset = %d, want %d (the head of the third line)", r.offset, lines[2].Start)
	}
}

// Skipped text counts as read, not as typed wrong: backspacing into it must
// not paint a screenful of red that was never typed.
func TestLineDownMarksSkippedTextRead(t *testing.T) {
	r, lines := nav(t, navText, 21)
	r.lineDown()
	for i := 0; i < lines[1].Start; i++ {
		if r.status[i] == wrong {
			t.Fatalf("status[%d] = wrong: skipped text was counted as an error", i)
		}
	}
	if !r.dirty {
		t.Error("moving a line left the position unsaved")
	}
}

// The gap between paragraphs is not a line to land on.
func TestLineDownStepsOverTheParagraphGap(t *testing.T) {
	r, _ := nav(t, navText, 21)
	for i := 0; i < 20 && r.offset < len(r.text); i++ {
		r.lineDown()
		if r.offset < len(r.text) && r.text[r.offset] == '\n' {
			t.Fatalf("landed on a newline at %d", r.offset)
		}
	}
	// Every landing was the head of some drawn line, or the end of the book.
	if r.offset != len(r.text) {
		t.Errorf("offset = %d after running off the end, want %d", r.offset, len(r.text))
	}
}

func TestLineUpUndoesLineDown(t *testing.T) {
	r, _ := nav(t, navText, 21)
	r.lineDown()
	r.lineDown()
	was := r.offset
	r.lineUp()
	if r.offset >= was {
		t.Fatalf("offset = %d after Up, want less than %d", r.offset, was)
	}
	r.lineUp()
	if r.offset != 0 {
		t.Errorf("offset = %d, want 0 — Up did not undo Down", r.offset)
	}
}

// From inside a line, Up goes to the head of that line and forgets what was
// typed there, so the line can be typed again from the start.
func TestLineUpFromMidLineForgetsTheLine(t *testing.T) {
	r, lines := nav(t, navText, 21)
	r.typeString("alp")
	r.lineUp()
	if r.offset != lines[0].Start {
		t.Fatalf("offset = %d, want %d (the head of the line)", r.offset, lines[0].Start)
	}
	for i := 0; i < 3; i++ {
		if r.status[i] != untyped {
			t.Errorf("status[%d] = %v, want untyped", i, r.status[i])
		}
	}
}

func TestLineUpAtTheStartIsHarmless(t *testing.T) {
	r, _ := nav(t, navText, 21)
	r.lineUp()
	if r.offset != 0 {
		t.Errorf("offset = %d, want 0", r.offset)
	}
}

func TestArrowKeysMoveTheLine(t *testing.T) {
	r, lines := nav(t, navText, 21)
	if quit := r.key(tui.Key{Type: tui.KeyDown}); quit {
		t.Fatal("Down quit")
	}
	if r.offset != lines[1].Start {
		t.Errorf("offset = %d, want %d", r.offset, lines[1].Start)
	}
	r.key(tui.Key{Type: tui.KeyUp})
	if r.offset != 0 {
		t.Errorf("offset = %d, want 0", r.offset)
	}
}

// Left, Right and Home stay inert: a caret loose inside a line is what lets
// the saved position drift away from what has been read.
func TestOtherArrowsStillDoNothing(t *testing.T) {
	r, _ := nav(t, navText, 21)
	r.typeString("alpha")
	was := r.offset
	for _, k := range []tui.KeyType{tui.KeyLeft, tui.KeyRight, tui.KeyHome, tui.KeyEnd, tui.KeyPageUp, tui.KeyPageDown} {
		r.key(tui.Key{Type: k})
		if r.offset != was {
			t.Fatalf("key %v moved the offset to %d, want %d", k, r.offset, was)
		}
	}
}
