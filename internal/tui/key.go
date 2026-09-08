package tui

import "unicode/utf8"

// KeyType is what a decoded keystroke is.
type KeyType int

const (
	KeyNone KeyType = iota // nothing decodable yet: wait for more bytes
	KeyRune                // a printable character; see Key.Rune
	KeyEnter
	KeyBackspace
	KeyTab
	KeyEsc
	KeyCtrlC
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
	KeyDelete
	KeyUnknown // a recognised-but-unused sequence, consumed and ignored
)

// A Key is one decoded keystroke.
type Key struct {
	Type KeyType
	Rune rune
}

// Decode reads one keystroke from the front of b.
//
// It returns the key and how many bytes it consumed. A return of (KeyNone, 0)
// means b holds the start of a sequence but not all of it — read more and call
// again. This is a pure function precisely so that the escape-sequence
// handling can be tested without a terminal, which is where the bugs are.
//
// With ENABLE_VIRTUAL_TERMINAL_INPUT set, keys arrive on stdin as these VT
// sequences under both ConPTY and conhost. The console input API is the one
// that differs between them (CLAUDE.md, gotcha 2).
func Decode(b []byte) (Key, int) {
	if len(b) == 0 {
		return Key{Type: KeyNone}, 0
	}

	switch b[0] {
	case 0x03:
		return Key{Type: KeyCtrlC}, 1
	case '\r', '\n':
		return Key{Type: KeyEnter}, 1
	case '\t':
		return Key{Type: KeyTab}, 1
	case 0x08, 0x7f: // Ctrl+H and DEL both arrive as Backspace
		return Key{Type: KeyBackspace}, 1
	case 0x1b:
		return decodeEscape(b)
	}

	if b[0] < 0x20 {
		return Key{Type: KeyUnknown}, 1 // some other control byte
	}

	r, size := utf8.DecodeRune(b)
	if r == utf8.RuneError && size <= 1 {
		if !utf8.FullRune(b) {
			return Key{Type: KeyNone}, 0 // a partial multi-byte rune
		}
		return Key{Type: KeyUnknown}, 1
	}
	return Key{Type: KeyRune, Rune: r}, size
}

// decodeEscape handles ESC and everything introduced by it.
//
// A bare Esc and the start of an arrow key look identical for one byte, which
// is the classic terminal ambiguity. Here it is resolved structurally: Esc is
// only reported when the byte after it cannot continue a sequence. The reader
// calls Decode on whatever a single read returned, and a real Esc keypress
// arrives alone in its own read, while an arrow key arrives as three bytes at
// once.
func decodeEscape(b []byte) (Key, int) {
	if len(b) == 1 {
		return Key{Type: KeyEsc}, 1
	}

	switch b[1] {
	case '[':
		return decodeCSI(b)
	case 'O': // SS3: the arrows in application cursor mode
		if len(b) < 3 {
			return Key{Type: KeyNone}, 0
		}
		if k, ok := cursorKey(b[2]); ok {
			return k, 3
		}
		return Key{Type: KeyUnknown}, 3
	case 0x1b:
		return Key{Type: KeyEsc}, 1 // Esc pressed twice
	}
	// Alt+key, or something we do not use. Consume the escape only, so the
	// character behind it is still typed rather than swallowed.
	return Key{Type: KeyEsc}, 1
}

// decodeCSI reads ESC [ ... final-byte.
func decodeCSI(b []byte) (Key, int) {
	// Parameters are digits and semicolons; the sequence ends at the first
	// byte in [0x40, 0x7E].
	for i := 2; i < len(b); i++ {
		c := b[i]
		if c >= 0x40 && c <= 0x7e {
			return csiKey(b[2:i], c), i + 1
		}
		if !(c >= '0' && c <= '9') && c != ';' && c != '?' {
			return Key{Type: KeyUnknown}, i + 1 // malformed: drop it
		}
	}
	return Key{Type: KeyNone}, 0 // still incomplete
}

func csiKey(params []byte, final byte) Key {
	if k, ok := cursorKey(final); ok {
		return k
	}
	switch final {
	case 'H':
		return Key{Type: KeyHome}
	case 'F':
		return Key{Type: KeyEnd}
	case '~':
		switch string(params) {
		case "1", "7":
			return Key{Type: KeyHome}
		case "3":
			return Key{Type: KeyDelete}
		case "4", "8":
			return Key{Type: KeyEnd}
		case "5":
			return Key{Type: KeyPageUp}
		case "6":
			return Key{Type: KeyPageDown}
		}
	}
	return Key{Type: KeyUnknown}
}

func cursorKey(final byte) (Key, bool) {
	switch final {
	case 'A':
		return Key{Type: KeyUp}, true
	case 'B':
		return Key{Type: KeyDown}, true
	case 'C':
		return Key{Type: KeyRight}, true
	case 'D':
		return Key{Type: KeyLeft}, true
	}
	return Key{}, false
}
