// Package norm reduces the Unicode text of a book to plain, typeable ASCII.
//
// This is load-bearing twice over. Typing stalls dead on a character that
// cannot be produced, and the invisible ones — soft hyphen, non-breaking
// space, zero-width joiner — are maddening because the screen shows nothing
// wrong. Second, ASCII means one rune is one cell, always, so the renderer
// never meets the East-Asian-width or combining-character problem that makes
// general terminal text layout hard.
//
// SPEC.md is the character table; table.go is that table in code.
package norm

import (
	"regexp"
	"strings"
)

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
func Normalize(s string) string {
	// Line endings first, so every later rule sees one kind of break.
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")

	s = toASCII(s)         // the character table (SPEC §1-6)
	s = dropMarkers(s)     // footnote markers (SPEC §7)
	s = rejoinHyphens(s)   // words broken across a line break (SPEC §4)
	s = paragraphs(s)      // hard wrapping undone, blank runs collapsed
	s = collapseSpaces(s)  // runs of spaces, trailing whitespace
	s = trimFrontMatter(s) // title and copyright pages (SPEC §8)

	s = strings.Trim(s, "\n")
	if s == "" {
		return ""
	}
	return s + "\n"
}

// --- footnote markers (SPEC §7) ---------------------------------------------

var (
	// word[1] and word{1}: a bracketed number welded to a word with no space
	// is a marker. With a space — "see [3] below" — it is prose about a
	// reference, and it stays.
	markerBracket = regexp.MustCompile(`(\S)\[\d{1,3}\]`)
	markerBrace   = regexp.MustCompile(`(\S)\{\d{1,3}\}`)

	// A bare asterisk welded to the end of a word. A standalone "* * *"
	// section break has spaces around it and is left alone.
	markerStar = regexp.MustCompile(`([A-Za-z])\*+`)
)

// dropMarkers removes the markers that are not part of the sentence. The
// superscript digits and the daggers are already gone: they are dropped by the
// character table.
func dropMarkers(s string) string {
	s = markerBracket.ReplaceAllString(s, "${1}")
	s = markerBrace.ReplaceAllString(s, "${1}")
	return markerStar.ReplaceAllString(s, "${1}")
}

// --- hyphens across a line break (SPEC §4) ----------------------------------

var (
	// disap-\npointed: the hyphen is the typesetter's, not the word's.
	brokenWord = regexp.MustCompile(`([a-z])-\n([a-z])`)
	// Anglo-\nSaxon: a capital after the break means the hyphen is real and
	// belongs to the word. Only the newline goes.
	keptHyphen = regexp.MustCompile(`-\n([A-Za-z])`)
)

func rejoinHyphens(s string) string {
	s = brokenWord.ReplaceAllString(s, "${1}${2}")
	return keptHyphen.ReplaceAllString(s, "-${1}")
}

// --- paragraph structure (SPEC §4) ------------------------------------------

// blankRun is two or more newlines, whatever whitespace sits between them.
var blankRun = regexp.MustCompile(`\n[ \t]*(?:\n[ \t]*)+`)

// paragraphs undoes the fixed-width wrapping of a plain-text book.
//
// A blank line is a paragraph break and survives as exactly one. A single
// newline inside a paragraph is an artefact of how the file was stored, and
// becomes a space: a paragraph must come out as one long line, because
// wrapping is a view computed at render time from the terminal's width.
func paragraphs(s string) string {
	// A sentinel no surviving character can be: the table has already
	// dropped every C0 control except the newline.
	const mark = "\x00"
	s = blankRun.ReplaceAllString(s, mark)
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.ReplaceAll(s, mark, "\n\n")
}

var spaceRun = regexp.MustCompile(`  +`)

func collapseSpaces(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSpace(spaceRun.ReplaceAllString(l, " "))
	}
	return strings.Join(lines, "\n")
}

// --- front matter (SPEC §8) -------------------------------------------------

// The extracted text of a book opens with a title page, a copyright page and
// often a table of contents. Nobody wants to type an ISBN.
//
// The rule is deliberately conservative, because the two failures are not
// symmetrical: a false trim silently loses real text, while a missed one costs
// a minute of typing.
const (
	frontWindowFrac = 10    // look only at the first tenth of the book...
	frontWindowMax  = 20000 // ...and never at more than this...
	frontWindowMin  = 2000  // ...but always at least this, for short texts
	bodyGap         = 400   // prose after a heading, rather than another entry
)

// headingLine matches a chapter opening standing alone on its line.
var headingLine = regexp.MustCompile(
	`^(?:(?:CHAPTER|Chapter|PART|Part|BOOK|Book|PROLOGUE|Prologue|EPILOGUE|Epilogue)\b.{0,60}` +
		`|[IVXLCDM]{1,7}\.?)$`)

// trimFrontMatter drops everything before the first chapter opening that
// actually starts a body of prose.
//
// "Actually starts a body of prose" is what separates the real heading from a
// table of contents: TOC entries sit a few characters apart, so the first
// heading with a real gap after it is the one the book begins at. A book with
// no headings at all keeps every word, which is the safe way to fail.
func trimFrontMatter(s string) string {
	window := len(s) / frontWindowFrac
	if window > frontWindowMax {
		window = frontWindowMax
	}
	if window < frontWindowMin {
		window = frontWindowMin
	}
	if window > len(s) {
		window = len(s)
	}

	var hits []int
	off := 0
	for _, line := range strings.SplitAfter(s, "\n") {
		if off >= window {
			break
		}
		if headingLine.MatchString(strings.TrimSpace(line)) {
			hits = append(hits, off)
		}
		off += len(line)
	}
	if len(hits) == 0 {
		return s
	}

	// The first heading with real prose behind it. What follows the last
	// heading is measured against the end of the text, or a book whose body
	// starts at its final front-matter heading is never found at all.
	//
	// The fallback is the *first* heading, not the last. When no heading has
	// room behind it — a short text, or a dense list — there is no way to
	// tell a contents page from chapters that simply run close together, and
	// cutting to the last one would eat every chapter before it. Keeping too
	// much is the failure to prefer.
	start := hits[0]
	for i, h := range hits {
		next := len(s)
		if i+1 < len(hits) {
			next = hits[i+1]
		}
		if next-h >= bodyGap {
			start = h
			break
		}
	}
	if start == 0 {
		return s // the book already begins at its first heading
	}
	return s[start:]
}
