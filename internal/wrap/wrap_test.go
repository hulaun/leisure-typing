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
