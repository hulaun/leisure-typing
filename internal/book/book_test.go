package book

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hulaun/leisure-typing/internal/progress"
)

// The fixture is a few paragraphs of our own prose: enough to have chapter
// headings, paragraph breaks and a couple of hundred words.
const fixture = "CHAPTER I\n\nThe morning came in slowly over the roofs.\n" +
	"A cart went past and did not stop.\n\nHe counted the panes twice.\n\n" +
	"CHAPTER II\n\nLater there would be work, and the relief of it.\n"

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func writeFixture(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// M1's done-when: a plain-text book round-trips and lists at 0%.
func TestImportRoundTrip(t *testing.T) {
	s := newStore(t)
	m, err := s.Import(writeFixture(t, "the-morning.txt", fixture))
	if err != nil {
		t.Fatal(err)
	}

	if m.ID != "the-morning" {
		t.Errorf("ID = %q, want %q", m.ID, "the-morning")
	}
	if m.Title != "The Morning" {
		t.Errorf("Title = %q", m.Title)
	}
	if m.Format != "txt" {
		t.Errorf("Format = %q", m.Format)
	}

	// Everything the layout promises is on disk.
	for _, name := range []string{"text.txt", "meta.json", "progress.json", "source.txt"} {
		if _, err := os.Stat(filepath.Join(s.Dir(m.ID), name)); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}

	text, err := s.Text(m)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), "The morning came in slowly") {
		t.Error("the text did not survive the import")
	}
	if len(text) != m.Runes {
		t.Errorf("meta says %d runes, text is %d", m.Runes, len(text))
	}

	// Importing makes the book current, and it starts at zero.
	cur, err := s.Current()
	if err != nil || cur.ID != m.ID {
		t.Errorf("Current() = %+v, %v", cur, err)
	}
	p, err := progress.Load(s.ProgressPath(m.ID), m.SHA256)
	if err != nil || p.Offset != 0 {
		t.Errorf("a fresh book is at %+v, %v", p, err)
	}
}

// The hash is there so that a changed text.txt is detected rather than
// silently resumed against.
func TestTamperedTextIsRefused(t *testing.T) {
	s := newStore(t)
	m, err := s.Import(writeFixture(t, "book.txt", fixture))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.TextPath(m.ID), []byte("something else entirely\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = s.Text(m)
	if err == nil {
		t.Fatal("a text.txt that does not match its hash was accepted")
	}
	if !strings.Contains(err.Error(), "re-import") {
		t.Errorf("the error should offer a re-import, got: %v", err)
	}
}

// Two books with the same name must not overwrite each other.
func TestIDCollision(t *testing.T) {
	s := newStore(t)
	first, err := s.Import(writeFixture(t, "book.txt", fixture))
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Import(writeFixture(t, "book.txt", fixture+"\nA second book.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatalf("both books got the id %q", first.ID)
	}
	if second.ID != "book-2" {
		t.Errorf("second id = %q, want book-2", second.ID)
	}
	if text, err := s.Text(first); err != nil || strings.Contains(string(text), "A second book") {
		t.Error("the first book was overwritten")
	}
}

func TestChaptersAreFound(t *testing.T) {
	s := newStore(t)
	m, err := s.Import(writeFixture(t, "book.txt", fixture))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Chapters) != 2 {
		t.Fatalf("found %d chapters: %+v", len(m.Chapters), m.Chapters)
	}
	if got := m.ChapterAt(0); got != "CHAPTER I" {
		t.Errorf("ChapterAt(0) = %q", got)
	}
	if got := m.ChapterAt(m.Chapters[1].Offset + 5); got != "CHAPTER II" {
		t.Errorf("ChapterAt(into chapter two) = %q", got)
	}
}

func TestListAndFind(t *testing.T) {
	s := newStore(t)
	if books, err := s.List(); err != nil || len(books) != 0 {
		t.Errorf("an empty store lists %v, %v", books, err)
	}
	if _, err := s.Current(); err != ErrNoBooks {
		t.Errorf("Current() on an empty store = %v, want ErrNoBooks", err)
	}

	if _, err := s.Import(writeFixture(t, "the-morning.txt", fixture)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Import(writeFixture(t, "another-one.txt", fixture)); err != nil {
		t.Fatal(err)
	}

	books, err := s.List()
	if err != nil || len(books) != 2 {
		t.Fatalf("List() = %d books, %v", len(books), err)
	}

	// A substring of the id or the title is enough.
	if m, err := s.Find("morning"); err != nil || m.ID != "the-morning" {
		t.Errorf("Find(\"morning\") = %+v, %v", m, err)
	}
	// An ambiguous name names the candidates rather than guessing.
	if _, err := s.Find("the"); err == nil {
		t.Error("an ambiguous name was resolved silently")
	}
	if _, err := s.Find("nothing-like-this"); err == nil {
		t.Error("a name matching nothing was accepted")
	}
}

func TestUseSwitchesCurrent(t *testing.T) {
	s := newStore(t)
	first, _ := s.Import(writeFixture(t, "one.txt", fixture))
	second, _ := s.Import(writeFixture(t, "two.txt", fixture))
	if cur, _ := s.Current(); cur.ID != second.ID {
		t.Error("importing did not make the new book current")
	}
	if err := s.SetCurrent(first.ID); err != nil {
		t.Fatal(err)
	}
	if cur, _ := s.Current(); cur.ID != first.ID {
		t.Error("SetCurrent did not take")
	}
}

func TestUnknownFormatIsRefused(t *testing.T) {
	s := newStore(t)
	_, err := s.Import(writeFixture(t, "book.xyz", fixture))
	if err == nil {
		t.Fatal("an unknown extension was imported")
	}
	if !strings.Contains(err.Error(), ".epub") {
		t.Errorf("the error should point at epub, got: %v", err)
	}
}

func TestEmptyBookIsRefused(t *testing.T) {
	s := newStore(t)
	if _, err := s.Import(writeFixture(t, "empty.txt", "   \n\n  \n")); err == nil {
		t.Error("a book with no text was imported")
	}
}

func TestSlug(t *testing.T) {
	cases := []struct{ in, want string }{
		{"The Morning", "the-morning"},
		{"  Spaced  Out  ", "spaced-out"},
		// Accents are dropped rather than transliterated: the id is only a
		// directory name, and meta.json keeps the real title.
		{"Ünïcödé", "n-c-d"},
		{"war & peace", "war-peace"},
		{"", ""},
	}
	for _, c := range cases {
		if got := slug(c.in); got != c.want {
			t.Errorf("slug(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
