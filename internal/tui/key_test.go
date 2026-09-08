package tui

import "testing"

func TestDecodeSimpleKeys(t *testing.T) {
	cases := []struct {
		in   string
		typ  KeyType
		r    rune
		size int
	}{
		{"a", KeyRune, 'a', 1},
		{" ", KeyRune, ' ', 1},
		{"~", KeyRune, '~', 1},
		{"\r", KeyEnter, 0, 1},
		{"\n", KeyEnter, 0, 1},
		{"\t", KeyTab, 0, 1},
		{"\x08", KeyBackspace, 0, 1},
		{"\x7f", KeyBackspace, 0, 1},
		{"\x03", KeyCtrlC, 0, 1},
		{"\x1b", KeyEsc, 0, 1},
	}
	for _, c := range cases {
		k, n := Decode([]byte(c.in))
		if k.Type != c.typ || n != c.size || (c.typ == KeyRune && k.Rune != c.r) {
			t.Errorf("Decode(%q) = %+v, %d; want type %v rune %q size %d",
				c.in, k, n, c.typ, c.r, c.size)
		}
	}
}

func TestDecodeEscapeSequences(t *testing.T) {
	cases := []struct {
		in   string
		typ  KeyType
		size int
	}{
		{"\x1b[A", KeyUp, 3},
		{"\x1b[B", KeyDown, 3},
		{"\x1b[C", KeyRight, 3},
		{"\x1b[D", KeyLeft, 3},
		{"\x1bOA", KeyUp, 3}, // application cursor mode
		{"\x1b[H", KeyHome, 3},
		{"\x1b[F", KeyEnd, 3},
		{"\x1b[3~", KeyDelete, 4},
		{"\x1b[5~", KeyPageUp, 4},
		{"\x1b[6~", KeyPageDown, 4},
		{"\x1b[1;2A", KeyUp, 6}, // modified arrow: still an arrow
	}
	for _, c := range cases {
		k, n := Decode([]byte(c.in))
		if k.Type != c.typ || n != c.size {
			t.Errorf("Decode(%q) = %+v, %d; want type %v size %d", c.in, k, n, c.typ, c.size)
		}
	}
}

// A sequence split across two reads must not be decoded as a bare Esc — that
// is the bug that makes an arrow key quit the app.
func TestDecodeIncompleteSequence(t *testing.T) {
	for _, in := range []string{"\x1b[", "\x1b[1", "\x1b[1;", "\x1bO"} {
		if k, n := Decode([]byte(in)); k.Type != KeyNone || n != 0 {
			t.Errorf("Decode(%q) = %+v, %d; want KeyNone, 0 (incomplete)", in, k, n)
		}
	}
}

func TestDecodeEmpty(t *testing.T) {
	if k, n := Decode(nil); k.Type != KeyNone || n != 0 {
		t.Errorf("Decode(nil) = %+v, %d", k, n)
	}
}

// Several keystrokes can arrive in one read; decoding must walk them.
func TestDecodeStream(t *testing.T) {
	b := []byte("ab\x1b[Ac\x7f")
	var got []KeyType
	for len(b) > 0 {
		k, n := Decode(b)
		if n == 0 {
			t.Fatalf("stalled with %q remaining", b)
		}
		got = append(got, k.Type)
		b = b[n:]
	}
	want := []KeyType{KeyRune, KeyRune, KeyUp, KeyRune, KeyBackspace}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// Normalization guarantees ASCII, but a stray multi-byte rune on stdin (an
// IME, a paste) must not desynchronise the stream.
func TestDecodeMultiByteRune(t *testing.T) {
	k, n := Decode([]byte("é"))
	if k.Type != KeyRune || n != 2 || k.Rune != 'é' {
		t.Errorf("Decode(\"é\") = %+v, %d", k, n)
	}
	if k, n := Decode([]byte("\xc3")); k.Type != KeyNone || n != 0 {
		t.Errorf("partial rune: Decode = %+v, %d; want KeyNone, 0", k, n)
	}
}
