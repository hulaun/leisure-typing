package main

import (
	"strings"
	"testing"

	"github.com/hulaun/leisure-typing/internal/tui"
)

func press(r *reader, keys ...tui.Key) bool {
	for _, k := range keys {
		if quit := r.key(k); quit {
			return true
		}
	}
	return false
}

func digits(s string) []tui.Key {
	var ks []tui.Key
	for _, ch := range s {
		ks = append(ks, tui.Key{Type: tui.KeyRune, Rune: ch})
	}
	return ks
}

var (
	ctrlG = tui.Key{Type: tui.KeyCtrlG}
	ctrlZ = tui.Key{Type: tui.KeyCtrlZ}
	enter = tui.Key{Type: tui.KeyEnter}
	esc   = tui.Key{Type: tui.KeyEsc}
)

// --- the word snap ----------------------------------------------------------

// A number typed by hand lands mid-word most of the time, and starting from
// the fourth letter of something is neither readable nor typable.
func TestSnapToWordGoesBackToTheHeadOfAWord(t *testing.T) {
	text := []rune("alpha bravo charlie")
	// 6 is the 'b' of bravo; 7, 8, 9, 10 are inside it.
	for _, off := range []int{6, 7, 8, 9, 10} {
		if got := snapToWord(text, off); got != 6 {
			t.Errorf("snapToWord(%d) = %d, want 6 (the head of \"bravo\")", off, got)
		}
	}
}

// Landing in the gap between words goes forward, not back: the position after
// any move is the head of something not yet read.
func TestSnapToWordForwardOutOfWhitespace(t *testing.T) {
	text := []rune("alpha bravo")
	if got := snapToWord(text, 5); got != 6 {
		t.Errorf("snapToWord(5) = %d, want 6 — the space should go forward", got)
	}

	// Across a paragraph break, which is several whitespace runes at once.
	para := []rune("one\n\ntwo")
	if got := snapToWord(para, 3); got != 5 {
		t.Errorf("snapToWord(3) = %d, want 5 — the paragraph gap should be crossed", got)
	}
}

func TestSnapToWordClamps(t *testing.T) {
	text := []rune("alpha bravo")
	if got := snapToWord(text, -50); got != 0 {
		t.Errorf("a negative offset = %d, want 0", got)
	}
	if got := snapToWord(text, 9999); got != len(text) {
		t.Errorf("an offset past the end = %d, want %d", got, len(text))
	}
}

// Whatever number goes in, the result is never inside a word.
func TestSnapToWordNeverLandsMidWord(t *testing.T) {
	text := []rune("The morning came in slowly over the roofs.\n\n" +
		"A cart went past and did not stop.\n")
	for off := 0; off <= len(text)+5; off++ {
		got := snapToWord(text, off)
		if got > 0 && got < len(text) && !isPlaceSpace(text[got-1]) {
			t.Fatalf("snapToWord(%d) = %d, which is inside the word ending %q",
				off, got, string(text[maxInt(0, got-8):got+1]))
		}
	}
}

// --- jumping ----------------------------------------------------------------

// A jump marks either side the way a resume does: read behind, untyped ahead.
// Nothing was got wrong on either side of it, so backspacing back over a jump
// must not paint a screenful of red that was never typed.
func TestJumpToMarksEitherSide(t *testing.T) {
	r := newReader("alpha bravo charlie delta echo\n")
	r.typeString("alpha bravo") // leaves some status set
	r.jumpTo(20)                // the head of "delta"
	if r.offset != 20 {
		t.Fatalf("offset = %d, want 20", r.offset)
	}
	for i := 0; i < r.offset; i++ {
		if r.status[i] != correct {
			t.Fatalf("status[%d] = %v behind the jump, want correct", i, r.status[i])
		}
	}
	for i := r.offset; i < len(r.status); i++ {
		if r.status[i] != untyped {
			t.Fatalf("status[%d] = %v ahead of the jump, want untyped", i, r.status[i])
		}
	}
	if !r.dirty {
		t.Error("a jump did not mark the position for saving")
	}
}

// Jumping backwards forgets what was typed ahead of the new position, so the
// text can be read again.
func TestJumpBackwardsForgetsWhatWasTyped(t *testing.T) {
	r := newReader("alpha bravo charlie\n")
	r.typeString("alpha bravo char")
	r.jumpTo(6) // back to "bravo"
	if r.offset != 6 {
		t.Fatalf("offset = %d, want 6", r.offset)
	}
	if r.status[10] != untyped {
		t.Errorf("status[10] = %v, want untyped — the text ahead was not forgotten",
			r.status[10])
	}
}

func TestJumpPastTheEndClamps(t *testing.T) {
	r := newReader("alpha bravo\n")
	r.jumpTo(99999)
	if r.offset != len(r.text) {
		t.Errorf("offset = %d, want %d", r.offset, len(r.text))
	}
}

// Jumping is the only thing in the app that can lose your place, so it is the
// only thing with an undo.
func TestCtrlZUndoesTheLastJump(t *testing.T) {
	r := newReader("alpha bravo charlie delta echo\n")
	r.typeString("alpha")
	was := r.offset

	press(r, ctrlG)
	press(r, digits("20")...)
	press(r, enter)
	if r.offset != 20 {
		t.Fatalf("offset = %d after the jump, want 20", r.offset)
	}

	press(r, ctrlG, ctrlZ)
	if r.offset != was {
		t.Errorf("offset = %d after undo, want %d", r.offset, was)
	}
	if r.place != nil {
		t.Error("undo left the Place screen open")
	}
}

func TestUndoIsNotOfferedBeforeAnyJump(t *testing.T) {
	r := newReader("alpha bravo charlie\n")
	press(r, ctrlG)
	if r.hasPrev {
		t.Fatal("there is an undo before anything has been jumped")
	}
	press(r, ctrlZ)
	if r.offset != 0 || r.place == nil {
		t.Error("Ctrl+Z with nothing to undo should do nothing at all")
	}
	if got := strings.Join(r.placeRows(placeWidth), "\n"); strings.Contains(got, "undo") {
		t.Error("the hint offers an undo that does not exist")
	}
}

// --- the screen -------------------------------------------------------------

func TestCtrlGTogglesPlace(t *testing.T) {
	r := newReader("alpha bravo\n")
	press(r, ctrlG)
	if r.place == nil {
		t.Fatal("Ctrl+G did not open Place")
	}
	press(r, ctrlG)
	if r.place != nil {
		t.Error("Ctrl+G did not close Place again")
	}
}

// The screen has to consume every key while it is open. A digit that reaches
// the book as well as the field would be typed into the text — and it would
// be counted wrong, since the book almost certainly does not say "4" there.
func TestPlaceConsumesKeysSoTheyDoNotReachTheBook(t *testing.T) {
	r := newReader("alpha bravo charlie\n")
	press(r, ctrlG)
	press(r, digits("12")...)

	if r.offset != 0 {
		t.Errorf("offset = %d: a key typed into the field also reached the book", r.offset)
	}
	for i, s := range r.status {
		if s != untyped {
			t.Fatalf("status[%d] = %v: the field wrote into the book", i, s)
		}
	}
	if r.place.input != "12" {
		t.Errorf("input = %q, want \"12\"", r.place.input)
	}
}

// The number is displayed grouped, so retyping it with the comma is the
// natural thing to do and must work.
func TestPlaceAcceptsGroupedNumbers(t *testing.T) {
	r := newReader(strings.Repeat("alpha bravo charlie delta ", 200))
	press(r, ctrlG)
	press(r, digits("1,234")...)
	if got, _ := r.place.offset(); got != 1234 {
		t.Errorf("offset = %d, want 1234 — the comma was not dropped", got)
	}
	// Spaces, dots and underscores too, and nothing else gets in.
	press(r, digits(" 5_6.7")...)
	if got, _ := r.place.offset(); got != 1234567 {
		t.Errorf("offset = %d, want 1234567", got)
	}
	press(r, digits("abc")...)
	if got, _ := r.place.offset(); got != 1234567 {
		t.Errorf("offset = %d: a letter got into the field", got)
	}
}

func TestPlaceBackspaceEditsTheField(t *testing.T) {
	r := newReader("alpha bravo\n")
	press(r, ctrlG)
	press(r, digits("123")...)
	press(r, tui.Key{Type: tui.KeyBackspace})
	if r.place.input != "12" {
		t.Errorf("input = %q, want \"12\"", r.place.input)
	}
	// Backspace on an empty field is harmless rather than a panic.
	press(r, tui.Key{Type: tui.KeyBackspace}, tui.Key{Type: tui.KeyBackspace},
		tui.Key{Type: tui.KeyBackspace})
	if r.place.input != "" {
		t.Errorf("input = %q, want empty", r.place.input)
	}
}

func TestPlaceEscCancelsWithoutMoving(t *testing.T) {
	r := newReader("alpha bravo charlie delta\n")
	press(r, ctrlG)
	press(r, digits("18")...)
	press(r, esc)

	if r.place != nil {
		t.Error("Esc did not close the screen")
	}
	if r.offset != 0 {
		t.Errorf("offset = %d: Esc moved the position", r.offset)
	}
}

// Esc closes the screen rather than quitting the app, but Ctrl+C still quits
// from anywhere — miss that and there is no way out (CLAUDE.md, gotcha 3).
func TestPlaceEscDoesNotQuitButCtrlCDoes(t *testing.T) {
	r := newReader("alpha bravo\n")
	press(r, ctrlG)
	if quit := r.key(esc); quit {
		t.Error("Esc on the Place screen quit the app")
	}
	press(r, ctrlG)
	if quit := r.key(tui.Key{Type: tui.KeyCtrlC}); !quit {
		t.Error("Ctrl+C on the Place screen did not quit")
	}
}

func TestPlaceEnterOnAnEmptyFieldDoesNotMove(t *testing.T) {
	r := newReader("alpha bravo charlie\n")
	r.typeString("alpha")
	was := r.offset
	press(r, ctrlG, enter)
	if r.offset != was {
		t.Errorf("offset = %d, want %d — Enter on an empty field moved", r.offset, was)
	}
	if r.hasPrev {
		t.Error("Enter on an empty field recorded an undo")
	}
}

// --- the preview ------------------------------------------------------------

// The preview is the whole reason this is a screen and not a prompt: a dropped
// digit sends you to 4% instead of 23%, and you see that before pressing
// Enter. It is also what catches a number that came from a different book,
// which a check digit could not.
func TestPreviewShowsWhereTheNumberLands(t *testing.T) {
	text := strings.Repeat("alpha bravo charlie delta echo foxtrot ", 40) +
		"the recognisable sentence at the end of it all.\n"
	r := newReader(text)
	r.meta.Words = 400

	press(r, ctrlG)
	press(r, digits("1520")...)

	// The number is snapped to a word boundary, so the preview shows where it
	// actually lands rather than what was typed — which is the point of it.
	want := comma(snapToWord(r.text, 1520))
	rows := stripANSI(strings.Join(r.placeRows(placeWidth), "\n"))
	if !strings.Contains(rows, "lands at "+want) {
		t.Errorf("the preview does not say it lands at %s:\n%s", want, rows)
	}
	if !strings.Contains(rows, "recognisable") {
		t.Errorf("the preview does not show the text at the offset:\n%s", rows)
	}
	if !strings.Contains(rows, "%") {
		t.Errorf("the preview does not show a percentage:\n%s", rows)
	}
}

func TestPreviewSaysWhenTheNumberIsPastTheEnd(t *testing.T) {
	r := newReader("alpha bravo charlie\n")
	press(r, ctrlG)
	press(r, digits("99999")...)
	rows := stripANSI(strings.Join(r.placeRows(placeWidth), "\n"))
	if !strings.Contains(rows, "past the end") {
		t.Errorf("a number past the end is not flagged:\n%s", rows)
	}
}

// An offset is only meaningful against one exact text.txt. Six characters of
// the hash on both screens is the whole safeguard against a number from
// another copy of the book resuming silently in the wrong chapter.
func TestPlaceShowsTheTextHash(t *testing.T) {
	r := newReader("alpha bravo charlie\n")
	r.meta.SHA256 = "a3f19c4b5d6e7f8091a2b3c4d5e6f70819a2b3c4d5e6f70819a2b3c4d5e6f708"
	press(r, ctrlG)

	rows := stripANSI(strings.Join(r.placeRows(placeWidth), "\n"))
	if !strings.Contains(rows, "text a3f19c") {
		t.Errorf("the hash prefix is not on the screen:\n%s", rows)
	}
	if strings.Contains(rows, "a3f19c4b") {
		t.Error("the whole hash is shown; six characters is the point")
	}
}

// The offset is the number you copy to the other device, so it has to be on
// the screen, grouped the way it is printed everywhere else.
func TestPlaceShowsTheCurrentOffset(t *testing.T) {
	r := newReader(strings.Repeat("alpha bravo charlie delta ", 200))
	r.jumpTo(4321)
	press(r, ctrlG)

	rows := stripANSI(strings.Join(r.placeRows(placeWidth), "\n"))
	if !strings.Contains(rows, "now at 4,3") {
		t.Errorf("the current offset is not on the screen:\n%s", rows)
	}
}

// --- the frame --------------------------------------------------------------

// The same rule as the page: a frame is exactly as tall as the terminal, and
// the last row is not terminated. One row too many and the terminal scrolls,
// which tears in a way that looks like a rendering bug.
func TestPlaceFrameIsExactlyTerminalHeight(t *testing.T) {
	r := newReader(strings.Repeat("A paragraph of a reasonable length.\n\n", 20))
	press(r, ctrlG)
	press(r, digits("120")...)

	for _, size := range [][2]int{{100, 30}, {80, 24}, {70, 16}, {60, 12}, {40, 10}} {
		frame := renderFrame(t, r, size[0], size[1])
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

// No row of the Place frame may be wider than the terminal, or it wraps and
// pushes the whole frame down.
func TestPlaceFrameFitsTheWidth(t *testing.T) {
	r := newReader(strings.Repeat("alpha bravo charlie delta echo ", 60))
	r.meta.Title = "A Title That Is Considerably Longer Than Any Narrow Terminal Is Wide"
	r.meta.SHA256 = strings.Repeat("ab", 32)
	press(r, ctrlG)
	press(r, digits("400")...)

	for _, size := range [][2]int{{100, 30}, {80, 24}, {40, 14}, {24, 12}} {
		frame := stripANSI(renderFrame(t, r, size[0], size[1]))
		for i, row := range strings.Split(frame, "\r\n") {
			if len([]rune(row)) > size[0] {
				t.Errorf("%dx%d: row %d is %d columns wide: %q",
					size[0], size[1], i, len([]rune(row)), row)
			}
		}
	}
}

// The screen has no stray newlines in it, for the same reason the page has
// none: a bare "\n" is a row the renderer did not intend.
func TestPlaceFrameHasNoStrayNewlines(t *testing.T) {
	r := newReader(strings.Repeat("alpha bravo charlie delta echo ", 60))
	press(r, ctrlG)
	press(r, digits("300")...)

	frame := renderFrame(t, r, 90, 26)
	if got, want := strings.Count(frame, "\n"), strings.Count(frame, "\r\n"); got != want {
		t.Errorf("frame has %d newlines but %d row terminators — %d stray",
			got, want, got-want)
	}
}

// --- what the Place screen is not -------------------------------------------

// Left, Right, Home and the Page keys stay inert while Place is open, the same
// as they are while reading. This is a way to carry a position between two
// machines, not the first step towards a free cursor.
func TestPlaceDoesNotAddCursorMovement(t *testing.T) {
	r := newReader("alpha bravo charlie delta\n")
	r.typeString("alpha")
	was := r.offset

	press(r, ctrlG)
	for _, k := range []tui.KeyType{tui.KeyLeft, tui.KeyRight, tui.KeyHome,
		tui.KeyEnd, tui.KeyPageUp, tui.KeyPageDown, tui.KeyUp, tui.KeyDown} {
		press(r, tui.Key{Type: k})
	}
	if r.offset != was {
		t.Errorf("offset = %d, want %d — a cursor key moved the position", r.offset, was)
	}
	if r.place == nil {
		t.Error("a cursor key closed the screen")
	}
}

// The status line and the Place screen have to agree exactly, or the number
// you copy off one screen does not match what you check on the other.
func TestStatusLineAndPlaceAgree(t *testing.T) {
	r := newReader(strings.Repeat("alpha bravo charlie delta ", 100))
	r.meta.Words = 400
	r.jumpTo(1200)

	status := stripANSI(r.statusLine(placeWidth))
	press(r, ctrlG)
	rows := stripANSI(strings.Join(r.placeRows(placeWidth), "\n"))

	pct := strings.TrimSpace(strings.Fields(status)[len(strings.Fields(status))-3])
	if !strings.Contains(rows, pct) {
		t.Errorf("the status line says %s but Place does not:\n%s", pct, rows)
	}
}
