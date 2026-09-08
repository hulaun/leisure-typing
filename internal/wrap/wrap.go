// Package wrap turns the book's text into display lines.
//
// Wrapping is a view and is never stored (CLAUDE.md, decision 5). The reading
// position is a rune offset into text.txt; line breaks are computed here, at
// render time, from the current terminal width. A resize therefore
// re-renders and invalidates nothing.
//
// Every line the package produces carries the offset range it covers, which is
// what lets the reader map a stored offset back onto the screen.
package wrap

import "strings"

// A Line is one row of the page.
//
// It covers the rune range [Start, End) of the text. Text is what is drawn:
// the range with any whitespace that the break consumed trimmed off, so
// Text is never longer than the wrap width and never ends in a space. The
// runes between Start+len(Text) and End are the whitespace the break ate —
// they still have to be typed, and the reader draws them as one trailing cell.
//
// A Blank line is the gap between paragraphs. It covers the paragraph break
// itself and is drawn as an empty row.
type Line struct {
	Start int
	End   int
	Text  string
	Blank bool
}

// Runes returns the number of runes on the line, including any whitespace the
// break consumed.
func (l Line) Runes() int { return l.End - l.Start }

// Contains reports whether the rune at offset belongs to this line.
func (l Line) Contains(offset int) bool { return offset >= l.Start && offset < l.End }

// Column returns the screen column at which the rune at offset is drawn.
// Whitespace eaten by the break collapses onto the single cell after the text.
func (l Line) Column(offset int) int {
	col := offset - l.Start
	if n := len([]rune(l.Text)); col > n {
		col = n
	}
	if col < 0 {
		col = 0
	}
	return col
}

// A Para is one paragraph of the text: the rune range [Start, End) of its
// content, and Gap, the offset one past the whitespace that follows it.
//
// Paragraphs are the anchor that makes windowed wrapping exact. Because each
// one wraps independently of every other, wrapping a slice of paragraphs
// produces byte-identical lines to wrapping the whole book — which is why the
// reader can lay out a screenful without touching the other 230 KB.
type Para struct {
	Start int
	End   int
	Gap   int
}

// Paras splits the text into paragraphs.
//
// A run of whitespace holding two or more newlines separates them. A single
// newline is *inside* a paragraph: normalization has already joined
// hard-wrapped lines (norm/SPEC.md, §4), and a book imported before that pass
// existed still has to render.
//
// The whole whitespace run is the gap, measured in one go. Testing it a rune
// at a time is what went wrong first time round: a predicate that only looked
// forward from a newline never counted the *second* newline of a "\n\n" pair
// as part of the break, so every paragraph after the first began one rune
// early — sitting on that newline — and carried it into its first display
// line, where the renderer wrote it out and split the row in two.
func Paras(text []rune) []Para {
	var out []Para
	n := len(text)
	i := skipSpace(text, 0)
	for i < n {
		start := i
		end, gap := n, n
		for j := i; j < n; j++ {
			if text[j] != '\n' {
				continue
			}
			after := skipSpace(text, j)
			if after >= n || newlinesIn(text[j:after]) >= 2 {
				end, gap = j, after
				break
			}
			j = after - 1 // a lone newline: the paragraph carries on
		}
		out = append(out, Para{Start: start, End: end, Gap: gap})
		i = gap
	}
	return out
}

// skipSpace returns the first index at or after i that is not whitespace.
func skipSpace(text []rune, i int) int {
	for i < len(text) && isSpace(text[i]) {
		i++
	}
	return i
}

func newlinesIn(rs []rune) int {
	n := 0
	for _, r := range rs {
		if r == '\n' {
			n++
		}
	}
	return n
}

// Wrap lays out one paragraph greedily into lines no wider than width.
func Wrap(text []rune, p Para, width int) []Line {
	if width < 1 {
		width = 1
	}
	var out []Line
	i := p.Start
	for i < p.End {
		// Skip the whitespace that the previous break consumed; it belongs
		// to that line's range, not to this one.
		lineStart := i
		// How far can this line reach?
		end := i + width
		if end >= p.End {
			out = append(out, line(text, lineStart, p.End, p.End))
			return out
		}
		// Walk back to the last space at or before the width limit.
		brk := -1
		for j := end; j > lineStart; j-- {
			if isSpace(text[j]) {
				brk = j
				break
			}
		}
		if brk < 0 {
			// One word longer than the whole line: hard-break it. Nothing
			// else keeps the guarantee that no line exceeds the width.
			out = append(out, line(text, lineStart, end, end))
			i = end
			continue
		}
		// Consume the run of spaces at the break; they are typed, but they
		// are not drawn at the start of the next line.
		next := brk
		for next < p.End && isSpace(text[next]) {
			next++
		}
		out = append(out, line(text, lineStart, brk, next))
		i = next
	}
	if len(out) == 0 {
		out = append(out, Line{Start: p.Start, End: p.End})
	}
	return out
}

func line(text []rune, start, textEnd, end int) Line {
	s := strings.TrimRight(string(text[start:textEnd]), " \n")
	// A display line is one row by construction. A newline left inside it
	// would be written straight into the frame, splitting the row in two and
	// pushing everything below it down — which reads on screen as the line
	// appearing twice. Replacing rather than trimming keeps one rune to one
	// column, which is what Column depends on.
	s = strings.ReplaceAll(s, "\n", " ")
	return Line{Start: start, End: end, Text: s}
}

func isSpace(r rune) bool { return r == ' ' || r == '\n' }

// Lines wraps the whole text. It is what the tests measure against and what a
// short text can use directly; the reader uses Window instead.
func Lines(text []rune, width int) []Line {
	var out []Line
	paras := Paras(text)
	for i, p := range paras {
		out = append(out, Wrap(text, p, width)...)
		if i < len(paras)-1 {
			out = append(out, Line{Start: p.End, End: p.Gap, Blank: true})
		}
	}
	return closeTail(out, paras, len(paras)-1)
}

// closeTail hands the whitespace after the book's final paragraph — the
// trailing newline — to the last line, so that the lines cover every rune of
// the text with no gap at the end.
func closeTail(out []Line, paras []Para, hi int) []Line {
	if len(out) > 0 && hi == len(paras)-1 && hi >= 0 {
		out[len(out)-1].End = paras[hi].Gap
	}
	return out
}

// Window lays out just enough of the book to draw a page: the line holding
// offset, at least before lines above it and after lines below.
//
// It returns the lines and the index of the active line within them. Because
// paragraphs wrap independently, these lines are exactly the lines Lines would
// have produced for the same offsets.
func Window(text []rune, offset, width, before, after int) (lines []Line, active int) {
	paras := Paras(text)
	if len(paras) == 0 {
		return []Line{{}}, 0
	}
	if offset < 0 {
		offset = 0
	}

	// The paragraph holding the offset — or, if the offset has landed in the
	// gap between two, the one that follows it.
	pi := 0
	for i, p := range paras {
		if offset < p.End {
			pi = i
			break
		}
		pi = i
		if offset < p.Gap {
			// In the gap: the reader is between paragraphs.
			if i+1 < len(paras) {
				pi = i + 1
			}
			break
		}
	}

	lo, hi := pi, pi
	for {
		lines, active = layout(text, paras, lo, hi, offset, width)
		enough := (active >= before || lo == 0) &&
			(len(lines)-1-active >= after || hi == len(paras)-1)
		if enough {
			return lines, active
		}
		if lo > 0 {
			lo--
		}
		if hi < len(paras)-1 {
			hi++
		}
	}
}

// layout wraps paragraphs [lo, hi] and locates the line holding offset.
func layout(text []rune, paras []Para, lo, hi, offset, width int) ([]Line, int) {
	var out []Line
	for i := lo; i <= hi; i++ {
		out = append(out, Wrap(text, paras[i], width)...)
		if i < hi {
			out = append(out, Line{Start: paras[i].End, End: paras[i].Gap, Blank: true})
		}
	}
	out = closeTail(out, paras, hi)
	active := 0
	for i, l := range out {
		if l.Blank {
			continue
		}
		active = i
		if offset < l.End {
			break
		}
	}
	return out, active
}
