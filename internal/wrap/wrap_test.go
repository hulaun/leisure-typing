package wrap

import (
	"strings"
	"testing"
)

// Original filler prose, so the fixtures carry nobody's copyright but ours.
const sample = "The morning came in slowly over the roofs and the whole street " +
	"waited for it, the way a room waits for someone who has said they will " +
	"arrive.\n\nHe counted the panes in the window twice and got a different " +
	"answer each time, which told him something about the morning and nothing " +
	"at all about the window.\n\nLater there would be work.\n"

func TestNoLineExceedsWidth(t *testing.T) {
	text := []rune(sample)
	for _, w := range []int{10, 20, 40, 68, 200} {
		for _, l := range Lines(text, w) {
			if n := len([]rune(l.Text)); n > w {
				t.Errorf("width %d: line %q is %d runes", w, l.Text, n)
			}
		}
	}
}

// Every rune of the text belongs to exactly one line, and the lines are laid
// end to end. This is what makes a stored offset findable on screen.
func TestRangesAreContiguousAndComplete(t *testing.T) {
	text := []rune(sample)
	lines := Lines(text, 30)
	if lines[0].Start != 0 {
		t.Errorf("first line starts at %d, want 0", lines[0].Start)
	}
	for i := 1; i < len(lines); i++ {
		if lines[i].Start != lines[i-1].End {
			t.Fatalf("gap between line %d (ends %d) and %d (starts %d)",
				i-1, lines[i-1].End, i, lines[i].Start)
		}
	}
	if last := lines[len(lines)-1]; last.End != len(text) {
		t.Errorf("last line ends at %d, text is %d runes", last.End, len(text))
	}
}

// No line may start on a space: the break consumes the whitespace, so the
// left margin stays straight.
func TestLinesDoNotStartOrEndWithSpace(t *testing.T) {
	text := []rune(sample)
	for _, l := range Lines(text, 25) {
		if l.Blank {
			continue
		}
		if strings.HasPrefix(l.Text, " ") || strings.HasSuffix(l.Text, " ") {
			t.Errorf("line %q has edge whitespace", l.Text)
		}
	}
}

func TestParagraphsBecomeBlankLines(t *testing.T) {
	lines := Lines([]rune(sample), 40)
	blanks := 0
	for _, l := range lines {
		if l.Blank {
			blanks++
		}
	}
	if blanks != 2 { // three paragraphs, two gaps
		t.Errorf("got %d blank lines, want 2", blanks)
	}
}

// A word longer than the line is hard-broken rather than allowed to overflow.
func TestOverlongWordIsHardBroken(t *testing.T) {
	text := []rune("short " + strings.Repeat("x", 25) + " end")
	for _, l := range Lines(text, 10) {
		if n := len([]rune(l.Text)); n > 10 {
			t.Fatalf("line %q is %d runes, width was 10", l.Text, n)
		}
	}
}

func TestColumnClampsEatenWhitespace(t *testing.T) {
	l := Line{Start: 0, End: 12, Text: "hello world"} // one space eaten
	if got := l.Column(0); got != 0 {
		t.Errorf("Column(0) = %d, want 0", got)
	}
	if got := l.Column(11); got != 11 {
		t.Errorf("Column(11) = %d, want 11", got)
	}
	// The eaten space draws in the single cell past the text, not off-screen.
	if got := l.Column(12); got != 11 {
		t.Errorf("Column(12) = %d, want 11", got)
	}
}

// The point of paragraph-anchored wrapping: a window is exact, not an
// approximation of the full layout.
func TestWindowMatchesFullLayout(t *testing.T) {
	text := []rune(sample)
	full := Lines(text, 30)
	for offset := 0; offset < len(text); offset += 7 {
		win, active := Window(text, offset, 30, 3, 3)
		got := win[active]
		if got.Blank {
			t.Fatalf("offset %d: active line is a blank line", offset)
		}
		// Find the same line in the full layout and compare.
		found := false
		for _, l := range full {
			if l.Start == got.Start {
				if l != got {
					t.Errorf("offset %d: window line %+v != full line %+v", offset, got, l)
				}
				found = true
				break
			}
		}
		if !found {
			t.Errorf("offset %d: window line %+v is in no full layout", offset, got)
		}
	}
}

func TestWindowGivesContextWhereThereIsSome(t *testing.T) {
	text := []rune(sample)
	// An offset in the middle paragraph has room above and below.
	mid := strings.Index(sample, "He counted") + 20
	win, active := Window(text, mid, 30, 3, 3)
	if active < 3 {
		t.Errorf("only %d lines of context above, want 3", active)
	}
	if below := len(win) - 1 - active; below < 3 {
		t.Errorf("only %d lines of context below, want 3", below)
	}
}

// At the very start there is nothing above, and Window must return rather
// than spin looking for context that does not exist.
func TestWindowAtTheEdges(t *testing.T) {
	text := []rune(sample)
	if win, active := Window(text, 0, 30, 3, 3); active != 0 || len(win) < 4 {
		t.Errorf("at offset 0: active=%d, %d lines", active, len(win))
	}
	if win, active := Window(text, len(text)-1, 30, 3, 3); active != len(win)-1 {
		t.Errorf("at the end: active=%d of %d lines", active, len(win))
	}
}

func TestEmptyText(t *testing.T) {
	if lines := Lines(nil, 30); len(lines) != 0 {
		t.Errorf("Lines(nil) = %v", lines)
	}
	if win, active := Window(nil, 0, 30, 3, 3); len(win) != 1 || active != 0 {
		t.Errorf("Window(nil) = %v, %d", win, active)
	}
}

// The bug this pins down: a paragraph after the first began one rune early,
// sitting on the second newline of its break, and carried that newline into
// its first display line. The renderer wrote it out, the row split in two, and
// the line appeared twice on screen.
//
// No display line may contain a newline, ever. This is the invariant the
// renderer depends on and the one the original tests missed — they checked for
// a leading *space*, which a leading newline is not.
func TestNoLineContainsANewline(t *testing.T) {
	texts := []string{
		sample,
		"One.\n\nTwo.\n\nThree.\n",
		"A\n\nB",
		"para one\n\n\n\npara two after several blank lines\n",
		// Not normalized: a book imported before norm.Normalize existed still
		// has to render as one row per line.
		"a hard-wrapped paragraph\nthat still has its own\nline breaks in it\n\nand another\n",
	}
	for _, text := range texts {
		for _, w := range []int{10, 30, 68} {
			for _, l := range Lines([]rune(text), w) {
				if strings.Contains(l.Text, "\n") {
					t.Errorf("width %d, text %.20q: line %q contains a newline", w, text, l.Text)
				}
			}
		}
	}
}

// A paragraph starts on text, never on the whitespace that ended the one
// before, and the gap between them covers that whitespace exactly.
func TestParagraphBoundaries(t *testing.T) {
	texts := []string{
		sample,
		"One.\n\nTwo.\n\nThree.\n",
		"  leading space\n\n\n\ntrailing too   \n\n",
		"no trailing newline\n\nat all",
	}
	for _, s := range texts {
		text := []rune(s)
		paras := Paras(text)
		for i, p := range paras {
			if p.Start >= p.End {
				t.Errorf("%.20q: paragraph %d is empty: %+v", s, i, p)
				continue
			}
			if isSpace(text[p.Start]) {
				t.Errorf("%.20q: paragraph %d starts on whitespace %q", s, i, text[p.Start])
			}
			// A paragraph may end on trailing spaces, and deliberately does:
			// they are runes the reader still has to type, so they belong to
			// the last real line's range, where Column clamps the caret to
			// the cell past the text. Pushing them into the blank separator
			// line instead would leave the caret invisible. It must not end
			// on a newline, though — that is the break, not the paragraph.
			if text[p.End-1] == '\n' {
				t.Errorf("%.20q: paragraph %d ends on a newline", s, i)
			}
			if p.Gap < p.End {
				t.Errorf("%.20q: paragraph %d has gap %d before end %d", s, i, p.Gap, p.End)
			}
			if i+1 < len(paras) && paras[i+1].Start != p.Gap {
				t.Errorf("%.20q: paragraph %d ends its gap at %d, but %d starts at %d",
					s, i, p.Gap, i+1, paras[i+1].Start)
			}
		}
	}
}

// A single newline is inside a paragraph; two or more separate paragraphs.
func TestParagraphCount(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"one\ntwo\nthree", 1},
		{"one\n\ntwo", 2},
		{"one\n\n\n\ntwo", 2},
		{"one\n \ntwo", 2}, // a blank line with a space on it is still blank
		{"one\n\ntwo\n\nthree\n", 3},
		{"\n\n\nonly one\n\n\n", 1},
		{"", 0},
		{"   \n\n  ", 0},
	}
	for _, c := range cases {
		if got := len(Paras([]rune(c.text))); got != c.want {
			t.Errorf("Paras(%q) found %d paragraphs, want %d", c.text, got, c.want)
		}
	}
}
