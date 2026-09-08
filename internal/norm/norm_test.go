package norm

import (
	"os"
	"strings"
	"testing"
)

// The reserved packages keep their own red loop, out of the default suite so
// that `go test ./...` stays green while the user has not written them yet.
func todo(t *testing.T) {
	t.Helper()
	if os.Getenv("LEISURE_TODO") == "" {
		t.Skip("norm.Normalize is RESERVED for the user; set LEISURE_TODO=1 to run its spec (internal/norm/SPEC.md)")
	}
}

func TestQuotesAndDashes(t *testing.T) {
	todo(t)
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
	todo(t)
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
	todo(t)
	// A hard-wrapped paragraph joins into one line; a blank line is a break.
	in := "the first line runs on\nand continues here\n\n\n\nthe second para\n"
	want := "the first line runs on and continues here\n\nthe second para\n"
	if got := Normalize(in); got != want {
		t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
	}
}

func TestHyphenRejoin(t *testing.T) {
	todo(t)
	if got := Normalize("disap-\npointed"); got != "disappointed\n" {
		t.Errorf("broken word not rejoined: %q", got)
	}
	if got := Normalize("Anglo-\nSaxon"); got != "Anglo-Saxon\n" {
		t.Errorf("real hyphen lost: %q", got)
	}
}

func TestLigaturesAndAccents(t *testing.T) {
	todo(t)
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
	todo(t)
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
	todo(t)
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
	todo(t)
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
	todo(t)
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
