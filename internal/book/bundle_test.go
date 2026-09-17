package book

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hulaun/leisure-typing/internal/progress"
)

// A book long enough that an offset in the middle of it is worth checking.
const bundleFixture = "CHAPTER I\n\n" +
	"The morning came in slowly over the roofs of the town, and a cart went " +
	"past below the window and did not stop at any door.\n\n" +
	"He counted the panes of glass twice over, and then a third time, because " +
	"the counting was easier than the waiting had been.\n\n" +
	"CHAPTER II\n\n" +
	"Later there would be work, and the relief of it, and the ordinary noise " +
	"of other people going about their own mornings.\n"

func exported(t *testing.T) (*Store, Meta, string) {
	t.Helper()
	src := newStore(t)
	m, err := src.Import(writeFixture(t, "the-morning.txt", bundleFixture))
	if err != nil {
		t.Fatal(err)
	}
	path, err := src.Export(m, filepath.Join(t.TempDir(), "out"+BundleExt))
	if err != nil {
		t.Fatal(err)
	}
	return src, m, path
}

// The promise the bundle exists to keep: after the book has crossed to another
// device, a rune offset means the same thing on both. That is what makes the
// Place screen a sync — read the number off one, type it into the other.
func TestBundleRoundTripKeepsOffsetsMeaningful(t *testing.T) {
	src, m, path := exported(t)

	dst := newStore(t)
	got, err := dst.Import(path)
	if err != nil {
		t.Fatal(err)
	}

	if got.SHA256 != m.SHA256 {
		t.Fatalf("hash changed in transit:\n  before %s\n  after  %s", m.SHA256, got.SHA256)
	}

	before, err := src.Text(m)
	if err != nil {
		t.Fatal(err)
	}
	after, err := dst.Text(got)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("the text is not byte-identical after a round trip")
	}

	// Stated as the user would state it: the same number lands on the same
	// words. This is the assertion that would fail if importing a bundle ever
	// re-derived the text instead of copying it.
	for _, off := range []int{0, 17, 120, 240, len(before) - 5} {
		if off < 0 || off+10 > len(before) {
			continue
		}
		if a, b := string(before[off:off+10]), string(after[off:off+10]); a != b {
			t.Errorf("offset %d is %q on one device and %q on the other", off, a, b)
		}
	}
}

// Everything derived stays as the exporter wrote it, so the two devices agree
// on the chapter and the percentage as well as the offset.
func TestBundleCarriesTheDerivedMetadata(t *testing.T) {
	_, m, path := exported(t)

	dst := newStore(t)
	got, err := dst.Import(path)
	if err != nil {
		t.Fatal(err)
	}

	if got.Title != m.Title {
		t.Errorf("Title = %q, want %q", got.Title, m.Title)
	}
	if got.Runes != m.Runes {
		t.Errorf("Runes = %d, want %d", got.Runes, m.Runes)
	}
	if got.Words != m.Words {
		t.Errorf("Words = %d, want %d", got.Words, m.Words)
	}
	if len(got.Chapters) != len(m.Chapters) {
		t.Fatalf("got %d chapters, want %d", len(got.Chapters), len(m.Chapters))
	}
	for i := range m.Chapters {
		if got.Chapters[i] != m.Chapters[i] {
			t.Errorf("chapter %d = %+v, want %+v", i, got.Chapters[i], m.Chapters[i])
		}
	}
}

// Importing a bundle must not re-derive anything. A bundle whose counts are
// deliberately odd keeps them: if these were recomputed, the far device could
// disagree with the near one about the percentage at the same offset.
func TestBundleImportDoesNotRecompute(t *testing.T) {
	text := []byte(bundleFixture)
	meta := Meta{
		ID:       "odd-book",
		Title:    "Odd Book",
		SHA256:   Hash(text),
		Runes:    len([]rune(bundleFixture)),
		Words:    999999,
		Chapters: []Chapter{{Title: "ONLY ONE", Offset: 3}},
	}
	path := filepath.Join(t.TempDir(), "odd"+BundleExt)
	writeBundle(t, path, bundleInfo{Version: bundleVersion}, meta, text, nil)

	got, err := newStore(t).Import(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Words != 999999 {
		t.Errorf("Words = %d: the word count was recomputed on import", got.Words)
	}
	if len(got.Chapters) != 1 || got.Chapters[0].Title != "ONLY ONE" {
		t.Errorf("Chapters = %+v: the chapters were re-derived on import", got.Chapters)
	}
}

// The position travels with the book, so the first transfer lands you where
// you already were rather than at the start.
func TestBundleCarriesThePosition(t *testing.T) {
	src := newStore(t)
	m, err := src.Import(writeFixture(t, "book.txt", bundleFixture))
	if err != nil {
		t.Fatal(err)
	}
	if err := progress.Save(src.ProgressPath(m.ID), progress.Progress{
		Offset: 137,
		SHA256: m.SHA256,
	}); err != nil {
		t.Fatal(err)
	}

	path, err := src.Export(m, filepath.Join(t.TempDir(), "b"+BundleExt))
	if err != nil {
		t.Fatal(err)
	}

	dst := newStore(t)
	got, err := dst.Import(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := progress.Load(dst.ProgressPath(got.ID), got.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	if p.Offset != 137 {
		t.Errorf("offset after transfer = %d, want 137", p.Offset)
	}
}

// A position that belongs to a different text is dropped rather than applied.
func TestBundleIgnoresAPositionFromAnotherText(t *testing.T) {
	text := []byte(bundleFixture)
	meta := Meta{ID: "b", Title: "B", SHA256: Hash(text), Runes: len([]rune(bundleFixture))}
	path := filepath.Join(t.TempDir(), "b"+BundleExt)
	writeBundle(t, path, bundleInfo{Version: bundleVersion}, meta, text,
		&progress.Progress{Offset: 120, SHA256: "a-hash-from-some-other-book"})

	s := newStore(t)
	got, err := s.Import(path)
	if err != nil {
		t.Fatal(err)
	}
	p, err := progress.Load(s.ProgressPath(got.ID), got.SHA256)
	if err != nil {
		t.Fatal(err)
	}
	if p.Offset != 0 {
		t.Errorf("offset = %d, want 0 — a position from another text was applied", p.Offset)
	}
}

// The check the whole transfer rests on. A bundle whose text does not match
// its recorded hash must not import at all: the alternative is a book that
// opens fine and resumes in the wrong chapter.
func TestBundleRefusesDamagedText(t *testing.T) {
	text := []byte(bundleFixture)
	meta := Meta{ID: "b", Title: "B", SHA256: Hash(text), Runes: len([]rune(bundleFixture))}
	path := filepath.Join(t.TempDir(), "b"+BundleExt)
	writeBundle(t, path, bundleInfo{Version: bundleVersion}, meta,
		[]byte("something else entirely\n"), nil)

	_, err := newStore(t).Import(path)
	if err == nil {
		t.Fatal("a bundle whose text does not match its hash was imported")
	}
	if !strings.Contains(err.Error(), "export it again") {
		t.Errorf("the error should say what to do, got: %v", err)
	}
}

// A file format that crosses between two machines which update separately has
// to say so when the far end is older.
func TestBundleRefusesANewerVersion(t *testing.T) {
	text := []byte(bundleFixture)
	meta := Meta{ID: "b", Title: "B", SHA256: Hash(text)}
	path := filepath.Join(t.TempDir(), "b"+BundleExt)
	writeBundle(t, path, bundleInfo{Version: bundleVersion + 1}, meta, text, nil)

	_, err := newStore(t).Import(path)
	if err == nil {
		t.Fatal("a bundle from a newer version was imported")
	}
	if !strings.Contains(err.Error(), "update bt") {
		t.Errorf("the error should say to update, got: %v", err)
	}
}

func TestBundleRefusesRubbish(t *testing.T) {
	dir := t.TempDir()

	notZip := filepath.Join(dir, "junk"+BundleExt)
	if err := os.WriteFile(notZip, []byte("this is not a zip file at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := newStore(t).Import(notZip); err == nil {
		t.Error("a file that is not a zip was imported as a bundle")
	}

	// A zip, but not one of ours.
	empty := filepath.Join(dir, "empty"+BundleExt)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("readme.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(empty, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := newStore(t).Import(empty); err == nil {
		t.Error("a zip with no book in it was imported as a bundle")
	}
}

// The laptop is the archive: the phone gets the derived text, not the source.
func TestBundleDoesNotCarryTheSource(t *testing.T) {
	_, _, path := exported(t)

	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()

	want := map[string]bool{
		"bundle.json": true, "meta.json": true,
		"text.txt": true, "progress.json": true,
	}
	for _, f := range zr.File {
		if !want[f.Name] {
			t.Errorf("the bundle carries %q, which it should not", f.Name)
		}
		delete(want, f.Name)
	}
	for name := range want {
		t.Errorf("the bundle is missing %q", name)
	}
}

// Importing the same bundle twice must not overwrite the first copy, the same
// as importing the same book twice.
func TestBundleImportedTwiceGetsANewID(t *testing.T) {
	_, _, path := exported(t)

	dst := newStore(t)
	first, err := dst.Import(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := dst.Import(path)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatalf("both imports got the id %q", first.ID)
	}
	if _, err := dst.Text(first); err != nil {
		t.Errorf("the first copy was damaged by the second import: %v", err)
	}
}

func TestExportDestinations(t *testing.T) {
	src := newStore(t)
	m, err := src.Import(writeFixture(t, "the-morning.txt", bundleFixture))
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	cases := []struct {
		name string
		dst  string
		want string
	}{
		// No destination must not mean "the current directory": an export
		// lands in bt-books/ beside the store wherever bt was run from.
		{"no destination", "", filepath.Join(src.BundleDir(), m.ID+BundleExt)},
		{"into a directory", dir, filepath.Join(dir, m.ID+BundleExt)},
		{"an explicit name", filepath.Join(dir, "carry.btbook"), filepath.Join(dir, "carry.btbook")},
		{"extension added", filepath.Join(dir, "carry2"), filepath.Join(dir, "carry2"+BundleExt)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := src.Export(m, c.dst)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Errorf("wrote %q, want %q", got, c.want)
			}
			if _, err := os.Stat(got); err != nil {
				t.Errorf("nothing at %q: %v", got, err)
			}
		})
	}
}

// Exporting a book whose text.txt has been tampered with would hand the other
// device a position that silently resumes in the wrong place.
func TestExportRefusesATamperedBook(t *testing.T) {
	src := newStore(t)
	m, err := src.Import(writeFixture(t, "book.txt", bundleFixture))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src.TextPath(m.ID), []byte("something else\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Export(m, filepath.Join(t.TempDir(), "x"+BundleExt)); err == nil {
		t.Fatal("a book whose text does not match its hash was exported")
	}
}

// writeBundle builds a bundle by hand, so that the damaged and malformed cases
// can be tested without a second copy of the writer.
func writeBundle(t *testing.T, path string, info bundleInfo, m Meta, text []byte,
	p *progress.Progress) {
	t.Helper()
	if info.Exported.IsZero() {
		info.Exported = time.Now()
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name string, v any) {
		t.Helper()
		body, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	add("bundle.json", info)
	add("meta.json", m)

	w, err := zw.Create("text.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(text); err != nil {
		t.Fatal(err)
	}
	if p != nil {
		add("progress.json", *p)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}
