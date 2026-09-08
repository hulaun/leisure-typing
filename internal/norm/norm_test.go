package norm

import (
	"strings"
	"testing"
)

// These are SPEC.md in executable form. They ran red behind LEISURE_TODO=1
// while Normalize was a stub; the package is written now, so they belong in
// the default suite.

func TestQuotesAndDashes(t *testing.T) {
	cases := []struct{ in, want string }{
		{"‘a’", "'a'"},
		{"“a”", `"a"`},
		{"«a»", `"a"`},
		{"don’t", "don't"},
		{"a–b", "a-b"},
		{"a—b", "a - b"},
		{"co­operate", "cooperate"}, // soft hyphen
		{"a−b", "a-b"},
	}
	for _, c := range cases {
		if got := strings.TrimRight(Normalize(c.in), "\n"); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSpaces(t *testing.T) {
	cases := []struct{ in, want string }{
		{"a b", "a b"}, // no-break space
		{"a b", "a b"},
		{"a​b", "ab"},        // zero-width space
		{"\ufeffabc", "abc"}, // byte order mark
		{"a\tb", "a b"},
		{"a    b", "a b"},
		{"a b   ", "a b"},
	}
	for _, c := range cases {
		if got := strings.TrimRight(Normalize(c.in), "\n"); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParagraphStructure(t *testing.T) {
	// A hard-wrapped paragraph joins into one line; a blank line is a break.
	in := "the first line runs on\nand continues here\n\n\n\nthe second para\n"
	want := "the first line runs on and continues here\n\nthe second para\n"
	if got := Normalize(in); got != want {
		t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
	}
}

func TestHyphenRejoin(t *testing.T) {
	if got := Normalize("disap-\npointed"); got != "disappointed\n" {
		t.Errorf("broken word not rejoined: %q", got)
	}
	if got := Normalize("Anglo-\nSaxon"); got != "Anglo-Saxon\n" {
		t.Errorf("real hyphen lost: %q", got)
	}
}

func TestLigaturesAndAccents(t *testing.T) {
	cases := []struct{ in, want string }{
		{"ﬁne", "fine"},
		{"ﬂow", "flow"},
		{"café", "cafe"},
		{"naïve", "naive"},
		{"straße", "strasse"},
		{"œuvre", "oeuvre"},
		{"Ångstrom", "Angstrom"},
	}
	for _, c := range cases {
		if got := strings.TrimRight(Normalize(c.in), "\n"); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSymbols(t *testing.T) {
	cases := []struct{ in, want string }{
		{"wait…", "wait..."},
		{"© 2026", "(c) 2026"},
		{"90°", "90 degrees"},
		{"½ a loaf", "1/2 a loaf"},
	}
	for _, c := range cases {
		if got := strings.TrimRight(Normalize(c.in), "\n"); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFootnoteMarkers(t *testing.T) {
	cases := []struct{ in, want string }{
		{"the word[1] after", "the word after"},
		{"the word{12} after", "the word after"},
		{"the word² after", "the word after"},
		{"see [3] below", "see [3] below"}, // spaced: prose, not a marker
	}
	for _, c := range cases {
		if got := strings.TrimRight(Normalize(c.in), "\n"); got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFrontMatterTrim(t *testing.T) {
	in := "The Title\n\nby Somebody\n\nCopyright 2026\n\nCHAPTER I\n\nThe body of the book begins.\n"
	got := Normalize(in)
	if !strings.HasPrefix(got, "CHAPTER I") {
		t.Errorf("front matter not trimmed: %q", got)
	}
	// A book with no chapter headings must keep everything.
	plain := "just some prose\n\nand more of it\n"
	if got := Normalize(plain); got != plain {
		t.Errorf("text without headings was trimmed: %q", got)
	}
}

// The property that the renderer depends on: one rune, one cell.
func TestOutputIsASCII(t *testing.T) {
	in := "“Café—naïve’s ﬁn­s”… 中文\r\n\tend"
	for i, r := range Normalize(in) {
		if r == '\n' {
			continue
		}
		if r < 0x20 || r > 0x7e {
			t.Fatalf("non-ASCII rune %q (U+%04X) at byte %d of output", r, r, i)
		}
	}
}

// A table of contents lists chapter headings a few characters apart. The trim
// has to walk past all of them to the heading that actually starts prose,
// rather than stopping at the last TOC entry.
func TestFrontMatterSkipsTableOfContents(t *testing.T) {
	body := "The body of the book begins here, and runs on for a good while " +
		"so that there is plainly prose after the heading rather than another " +
		"line of a list. " + strings.Repeat("More words follow. ", 30)

	in := "A Book Of Some Kind\n\nby Nobody\n\nCopyright 2026, nobody at all\n\n" +
		"CONTENTS\n\nCHAPTER I\n\nCHAPTER II\n\nCHAPTER III\n\n" +
		"CHAPTER I\n\n" + body

	got := Normalize(in)
	if !strings.HasPrefix(got, "CHAPTER I\n\nThe body of the book") {
		t.Errorf("the trim stopped on a contents entry:\n%.120q", got)
	}
	if strings.Contains(got, "Copyright") {
		t.Errorf("the copyright page survived:\n%.120q", got)
	}
	// Exactly one heading survives: the three TOC entries went with it.
	if n := strings.Count(got, "CHAPTER"); n != 1 {
		t.Errorf("%d chapter headings survived, want 1:\n%.200q", n, got)
	}
}

// A book that already opens on its first heading must not lose it.
func TestFrontMatterKeepsAnOpeningHeading(t *testing.T) {
	in := "CHAPTER I\n\nThe body begins at once, with no title page before it.\n"
	if got := Normalize(in); !strings.HasPrefix(got, "CHAPTER I") {
		t.Errorf("the opening heading was trimmed away:\n%q", got)
	}
}

// The safe failure: no heading anywhere means no trim, however much the
// opening looks like front matter.
func TestFrontMatterWithoutHeadingsKeepsEverything(t *testing.T) {
	in := "A Title\n\nby Somebody\n\nCopyright 2026\n\nAnd then the prose.\n"
	if got := Normalize(in); got != in {
		t.Errorf("text with no chapter heading was trimmed:\n got %q\nwant %q", got, in)
	}
}

// The whole point of the pass, stated once as a property: whatever goes in,
// what comes out can be typed on a US keyboard.
func TestEveryOutputCharacterIsTypeable(t *testing.T) {
	inputs := []string{
		"", "   ", "\n\n\n", "\r\n\r\n",
		"“Curly” ‘quotes’ — and dashes…",
		"中文 and ελληνικά and العربية",
		"\x00\x01\x07 control bytes \x1b[31m",
		strings.Repeat("word[1] ", 100),
	}
	for _, in := range inputs {
		out := Normalize(in)
		for _, r := range out {
			if r == '\n' {
				continue
			}
			if r < 0x20 || r > 0x7e {
				t.Errorf("Normalize(%.20q) produced %q (U+%04X)", in, r, r)
			}
		}
		if out != "" && !strings.HasSuffix(out, "\n") {
			t.Errorf("Normalize(%.20q) = %q: no trailing newline", in, out)
		}
		if strings.Contains(out, "\n\n\n") {
			t.Errorf("Normalize(%.20q) left a run of blank lines", in)
		}
	}
}

// Normalizing twice must change nothing the second time: the import pipeline
// depends on text.txt being a fixed point, since its hash is what progress is
// checked against.
func TestNormalizeIsIdempotent(t *testing.T) {
	inputs := []string{
		"“Café—naïve’s ﬁne”… \r\n\tend",
		"CHAPTER I\n\nA paragraph that\nwas hard wrapped.\n\nAnd another.\n",
		"disap-\npointed and Anglo-\nSaxon",
		"the word[1] after",
	}
	for _, in := range inputs {
		once := Normalize(in)
		if twice := Normalize(once); twice != once {
			t.Errorf("Normalize is not a fixed point for %.30q:\n once %q\ntwice %q", in, once, twice)
		}
	}
}

// Two real chapters close together must both survive. This is the case that
// caught the first version of the rule: no heading had 400 characters behind
// it, the fallback took the *last* heading, and the whole of chapter one went
// with the front matter it was mistaken for.
func TestFrontMatterDoesNotEatCloseChapters(t *testing.T) {
	in := "CHAPTER I\n\nThe morning came in slowly over the roofs.\n\n" +
		"CHAPTER II\n\nLater there would be work, and the relief of it.\n"

	got := Normalize(in)
	if !strings.Contains(got, "The morning came in slowly") {
		t.Errorf("chapter one was trimmed away as front matter:\n%q", got)
	}
	if n := strings.Count(got, "CHAPTER"); n != 2 {
		t.Errorf("%d headings survived, want 2:\n%q", n, got)
	}
}

// The same shape, but with a title page in front of it: the trim should still
// happen, and should still stop at the first heading.
func TestFrontMatterTrimsToTheFirstOfCloseChapters(t *testing.T) {
	in := "A Title\n\nby Somebody\n\nCopyright 2026\n\n" +
		"CHAPTER I\n\nThe morning came in slowly.\n\n" +
		"CHAPTER II\n\nLater there would be work.\n"

	got := Normalize(in)
	if strings.Contains(got, "Copyright") {
		t.Errorf("the copyright page survived:\n%q", got)
	}
	if !strings.HasPrefix(got, "CHAPTER I") {
		t.Errorf("the trim did not stop at the first heading:\n%q", got)
	}
	if !strings.Contains(got, "The morning came in slowly") {
		t.Errorf("chapter one's text was lost:\n%q", got)
	}
}
