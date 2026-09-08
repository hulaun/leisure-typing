package epub

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These are SPEC.md in executable form, run against the hand-built
// testdata/minimal.epub. They ran red behind LEISURE_TODO=1 while Extract was
// a stub; the package is written now, so they belong in the default suite.

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
	if text := extract(t).Text; !strings.Contains(text, "The second chapter begins here.") {
		t.Errorf("OEBPS/text/ch 2.xhtml (href=\"text/ch%%202.xhtml\") was not resolved:\n%s", text)
	}
}

func TestLinearNoIsSkipped(t *testing.T) {
	if text := extract(t).Text; strings.Contains(text, "Contents") {
		t.Errorf("the linear=\"no\" nav document was included:\n%s", text)
	}
}

func TestDroppedElements(t *testing.T) {
	text := extract(t).Text
	for _, s := range []string{"var dropped", "margin: 0", "<", ">"} {
		if strings.Contains(text, s) {
			t.Errorf("output contains %q, which should have been dropped:\n%s", s, text)
		}
	}
}

func TestInlineElementsDoNotSplitWords(t *testing.T) {
	if text := extract(t).Text; !strings.Contains(text, "The first paragraph") {
		t.Errorf("inline <em> introduced a break:\n%s", text)
	}
}

func TestNoterefIsDropped(t *testing.T) {
	if text := extract(t).Text; !strings.Contains(text, "A second paragraph ending") {
		t.Errorf("the noteref marker survived, or its text was mangled:\n%s", text)
	}
}

func TestParagraphSeparation(t *testing.T) {
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
	path := filepath.Join(t.TempDir(), "not.epub")
	if err := os.WriteFile(path, []byte("this is not a zip archive"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Extract(path); err == nil {
		t.Error("Extract accepted a file that is not a ZIP")
	}
}

func TestMissingFile(t *testing.T) {
	if _, err := Extract(filepath.Join("testdata", "does-not-exist.epub")); err == nil {
		t.Error("Extract accepted a path that does not exist")
	}
}

// buildEPUB writes a ZIP of the given entries to a temp file and returns its
// path, so the failure paths can be exercised without another fixture on disk.
func buildEPUB(t *testing.T, entries map[string]string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "built.epub")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	z := zip.NewWriter(f)
	for name, body := range entries {
		w, err := z.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

const containerDoc = `<?xml version="1.0"?>
<container xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles><rootfile full-path="content.opf"/></rootfiles>
</container>`

// opf builds a package document with one spine document.
func opf(extra string) string {
	return `<?xml version="1.0"?>
<package xmlns="http://www.idpf.org/2007/opf">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title>Built</dc:title></metadata>
  <manifest><item id="a" href="a.xhtml" media-type="application/xhtml+xml"/></manifest>
  <spine>` + extra + `<itemref idref="a"/></spine>
</package>`
}

func page(body string) string {
	return `<?xml version="1.0"?><html xmlns="http://www.w3.org/1999/xhtml">` +
		`<head><title>t</title></head><body>` + body + `</body></html>`
}

// A ZIP with no container is not an EPUB, and the message should say which
// piece is missing rather than failing vaguely.
func TestMissingContainer(t *testing.T) {
	path := buildEPUB(t, map[string]string{"mimetype": "application/epub+zip"})
	_, err := Extract(path)
	if err == nil {
		t.Fatal("a ZIP with no container.xml was accepted")
	}
	if !strings.Contains(err.Error(), "container.xml") {
		t.Errorf("the error should name container.xml, got: %v", err)
	}
}

// A ZIP that says it is something else entirely.
func TestWrongMimetype(t *testing.T) {
	path := buildEPUB(t, map[string]string{
		"mimetype":               "application/zip",
		"META-INF/container.xml": containerDoc,
	})
	if _, err := Extract(path); err == nil {
		t.Error("an archive declaring the wrong mimetype was accepted")
	}
}

// A missing mimetype entry is tolerated: plenty of real books get it wrong,
// and container.xml is the real test of whether this is an EPUB.
func TestMissingMimetypeIsTolerated(t *testing.T) {
	path := buildEPUB(t, map[string]string{
		"META-INF/container.xml": containerDoc,
		"content.opf":            opf(""),
		"a.xhtml":                page("<p>Some prose in a book with no mimetype entry.</p>"),
	})
	b, err := Extract(path)
	if err != nil {
		t.Fatalf("a book with no mimetype entry was refused: %v", err)
	}
	if !strings.Contains(b.Text, "Some prose") {
		t.Errorf("text = %q", b.Text)
	}
}

func TestEmptySpine(t *testing.T) {
	path := buildEPUB(t, map[string]string{
		"mimetype":               "application/epub+zip",
		"META-INF/container.xml": containerDoc,
		"content.opf": `<?xml version="1.0"?><package xmlns="http://www.idpf.org/2007/opf">
			<manifest/><spine/></package>`,
	})
	_, err := Extract(path)
	if err == nil {
		t.Fatal("a book with no spine was accepted")
	}
	if !strings.Contains(err.Error(), "spine") {
		t.Errorf("the error should name the spine, got: %v", err)
	}
}

// A spine that resolves to nothing readable must fail rather than import an
// empty book: silently importing nothing is the worst outcome.
func TestSpineResolvingToNothing(t *testing.T) {
	path := buildEPUB(t, map[string]string{
		"mimetype":               "application/epub+zip",
		"META-INF/container.xml": containerDoc,
		"content.opf":            opf(""), // names a.xhtml, which is not in the archive
	})
	if _, err := Extract(path); err == nil {
		t.Error("a spine resolving to no documents was accepted")
	}
}

// Tables in novels are layout artefacts, and typing one is miserable.
func TestTablesAreDropped(t *testing.T) {
	path := buildEPUB(t, map[string]string{
		"mimetype":               "application/epub+zip",
		"META-INF/container.xml": containerDoc,
		"content.opf":            opf(""),
		"a.xhtml": page(`<p>Before the table.</p>
			<table><tr><td>cell one</td><td>cell two</td></tr></table>
			<p>After the table.</p>`),
	})
	b, err := Extract(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.Text, "cell one") {
		t.Errorf("a table survived:\n%s", b.Text)
	}
	for _, want := range []string{"Before the table.", "After the table."} {
		if !strings.Contains(b.Text, want) {
			t.Errorf("%q was lost with the table:\n%s", want, b.Text)
		}
	}
}

// Undeclared HTML entities are everywhere in real books, and stop a strict
// decoder at the first one.
func TestUndeclaredEntities(t *testing.T) {
	path := buildEPUB(t, map[string]string{
		"mimetype":               "application/epub+zip",
		"META-INF/container.xml": containerDoc,
		"content.opf":            opf(""),
		"a.xhtml":                page("<p>one&nbsp;two&mdash;three&hellip;four</p>"),
	})
	b, err := Extract(path)
	if err != nil {
		t.Fatalf("an undeclared entity stopped the decoder: %v", err)
	}
	if !strings.Contains(b.Text, "three") || !strings.Contains(b.Text, "four") {
		t.Errorf("text was lost after an entity:\n%q", b.Text)
	}
}

// Extract returns Unicode; making it typeable is norm.Normalize's job, and
// keeping them apart means the character table can change without re-reading
// any EPUB.
func TestExtractDoesNotNormalize(t *testing.T) {
	path := buildEPUB(t, map[string]string{
		"mimetype":               "application/epub+zip",
		"META-INF/container.xml": containerDoc,
		"content.opf":            opf(""),
		"a.xhtml":                page("<p>“Curly” and an em—dash</p>"),
	})
	b, err := Extract(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.Text, "“Curly”") {
		t.Errorf("Extract flattened characters that are norm's to handle:\n%q", b.Text)
	}
}
