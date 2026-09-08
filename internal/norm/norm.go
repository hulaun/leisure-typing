// Package norm reduces the Unicode text of a book to plain, typeable ASCII.
//
// This is load-bearing twice over. Typing stalls dead on a character that
// cannot be produced from the keyboard, and the invisible ones — soft hyphen,
// non-breaking space, zero-width joiner — are maddening because the screen
// shows nothing wrong. Second, ASCII means one rune is one cell, always, so
// the renderer never meets the East-Asian-width or combining-character problem
// that makes general terminal text layout hard.
//
// See SPEC.md for the character table Normalize is expected to cover.
package norm

// Normalize converts arbitrary Unicode book text into ASCII that can be typed
// on a US keyboard, and tidies the structure of the text for reading.
//
// The contract:
//
//   - The result contains only bytes in [0x20, 0x7E] plus '\n'. No tabs, no
//     carriage returns, no C0 or C1 controls, no rune above U+007F.
//   - Paragraphs are separated by exactly one blank line. No other run of
//     blank lines survives, and there is no leading or trailing blank line.
//   - Lines carry no trailing whitespace, and no run of two or more spaces
//     survives inside a line.
//   - Hard line breaks inside a paragraph — the fixed-width wrapping of a
//     plain-text book — are joined back into one long line, because wrapping
//     is a view computed at render time and never stored.
//   - Words hyphenated across a line break are rejoined.
//   - Footnote markers are removed; front matter is trimmed.
//   - The result ends with a single '\n' unless it is empty.
//
// Normalize is total: it never returns an error, and any input at all
// produces some valid output.
//
// RESERVED: this is the user's to write. See CLAUDE.md, "Division of work".
func Normalize(s string) string {
	// TODO(user): the character table. See SPEC.md.
	return s
}
