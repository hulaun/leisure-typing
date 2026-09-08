package epub

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func todo(t *testing.T) {
	t.Helper()
	if os.Getenv("LEISURE_TODO") == "" {
		t.Skip("epub.Extract is RESERVED for the user; set LEISURE_TODO=1 to run its spec (internal/epub/SPEC.md)")
	}
}

func extract(t *testing.T) *Book {
	t.Helper()
	b, err := Extract(filepath.Join("testdata", "minimal.epub"))
	if err != nil {
		t.Fatalf("Extract(minimal.epub): %v", err)
	}
	if b == nil {
		t.Fatal("Extract returned a nil book and a nil error")
	}
	return b
}

func TestMetadata(t *testing.T) {
	todo(t)
	b := extract(t)
	if b.Title != "A Minimal Book" {
		t.Errorf("Title = %q, want %q", b.Title, "A Minimal Book")
	}
	if b.Author != "Nobody At All" {
		t.Errorf("Author = %q, want %q", b.Author, "Nobody At All")
	}
}

// The spine, not the manifest, is the reading order — and the manifest lists
// chapter two first precisely to catch an implementation that walks it.
func TestSpineOrder(t *testing.T) {
	todo(t)
	text := extract(t).Text
	one, two := strings.Index(text, "Chapter One"), strings.Index(text, "Chapter Two")
	if one < 0 || two < 0 {
		t.Fatalf("a chapter heading is missing from the text:\n%s", text)
	}
	if one > two {
		t.Error("chapters came out in manifest order, not spine order")
	}
}

// href is relative to the OPF's directory, and may be percent-encoded.
func TestHrefResolution(t *testing.T) {
	todo(t)
	if text := extract(t).Text; !strings.Contains(text, "The second chapter begins here.") {
		t.Errorf("OEBPS/text/ch 2.xhtml (href=\"text/ch%%202.xhtml\") was not resolved:\n%s", text)
	}
}

func TestLinearNoIsSkipped(t *testing.T) {
	todo(t)
	if text := extract(t).Text; strings.Contains(text, "Contents") {
		t.Errorf("the linear=\"no\" nav document was included:\n%s", text)
	}
}

func TestDroppedElements(t *testing.T) {
	todo(t)
	text := extract(t).Text
	for _, s := range []string{"var dropped", "margin: 0", "<", ">"} {
		if strings.Contains(text, s) {
			t.Errorf("output contains %q, which should have been dropped:\n%s", s, text)
		}
	}
}

func TestInlineElementsDoNotSplitWords(t *testing.T) {
	todo(t)
	if text := extract(t).Text; !strings.Contains(text, "The first paragraph") {
		t.Errorf("inline <em> introduced a break:\n%s", text)
	}
}

func TestNoterefIsDropped(t *testing.T) {
	todo(t)
	if text := extract(t).Text; !strings.Contains(text, "A second paragraph ending") {
		t.Errorf("the noteref marker survived, or its text was mangled:\n%s", text)
	}
}

func TestParagraphSeparation(t *testing.T) {
	todo(t)
	text := extract(t).Text
	if !strings.Contains(text, "\n\n") {
		t.Errorf("no paragraph breaks at all in the output:\n%s", text)
	}
	if strings.Contains(text, "\n\n\n") {
		t.Errorf("runs of blank lines were not collapsed:\n%s", text)
	}
	if strings.HasPrefix(text, "\n") {
		t.Error("output has a leading blank line")
	}
	if !strings.HasSuffix(text, "\n") || strings.HasSuffix(text, "\n\n") {
		t.Error("output should end with exactly one newline")
	}
	// <br/> is a single newline, not a paragraph break.
	if !strings.Contains(text, "A line\nand its continuation.") {
		t.Errorf("<br/> was not rendered as a single newline:\n%s", text)
	}
}

func TestNotAnEpub(t *testing.T) {
	todo(t)
	path := filepath.Join(t.TempDir(), "not.epub")
	if err := os.WriteFile(path, []byte("this is not a zip archive"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Extract(path); err == nil {
		t.Error("Extract accepted a file that is not a ZIP")
	}
}

func TestMissingFile(t *testing.T) {
	todo(t)
	if _, err := Extract(filepath.Join("testdata", "does-not-exist.epub")); err == nil {
		t.Error("Extract accepted a path that does not exist")
	}
}
