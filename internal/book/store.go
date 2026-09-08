// Package book is the import pipeline and the on-disk store.
//
// The layout, one directory per book:
//
//	storage/books/<id>/
//	  source.<ext>    the original, kept, so re-import needs no hunting
//	  text.txt        normalized, immutable, the only thing read at runtime
//	  meta.json       title, author, sha256 of text.txt, chapter offsets
//	  progress.json   { offset, updated }
//	storage/current   the id of the book bt opens with no arguments
package book

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Meta is meta.json.
type Meta struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Author   string    `json:"author,omitempty"`
	Source   string    `json:"source"` // the original file's name
	Format   string    `json:"format"` // txt, epub, pdf, mobi, ...
	SHA256   string    `json:"sha256"` // of text.txt
	Runes    int       `json:"runes"`  // length of text.txt in runes
	Words    int       `json:"words"`  // for the status line
	Imported time.Time `json:"imported"`

	// Chapters are offsets into text.txt, in order, for the status line and
	// for jumping. Empty when the extractor found none.
	Chapters []Chapter `json:"chapters,omitempty"`
}

// A Chapter is a heading and where it starts.
type Chapter struct {
	Title  string `json:"title"`
	Offset int    `json:"offset"`
}

// A Store is a storage root: everything under storage/.
type Store struct{ Root string }

// ErrNoBooks is returned when the store holds nothing yet.
var ErrNoBooks = errors.New("no books imported yet — try: bt import <file>")

// ErrNoCurrent is returned when no book has been made current.
var ErrNoCurrent = errors.New("no current book — try: bt list, then bt use <name>")

// Open returns the store at root, creating the directories it needs.
func Open(root string) (*Store, error) {
	s := &Store{Root: root}
	if err := os.MkdirAll(s.booksDir(), 0o755); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) booksDir() string              { return filepath.Join(s.Root, "books") }
func (s *Store) Dir(id string) string          { return filepath.Join(s.booksDir(), id) }
func (s *Store) currentFile() string           { return filepath.Join(s.Root, "current") }
func (s *Store) TextPath(id string) string     { return filepath.Join(s.Dir(id), "text.txt") }
func (s *Store) MetaPath(id string) string     { return filepath.Join(s.Dir(id), "meta.json") }
func (s *Store) ProgressPath(id string) string { return filepath.Join(s.Dir(id), "progress.json") }

// List returns the imported books, oldest import first.
func (s *Store) List() ([]Meta, error) {
	entries, err := os.ReadDir(s.booksDir())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []Meta
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := s.Meta(e.Name())
		if err != nil {
			continue // a half-written import is skipped, not fatal to `bt list`
		}
		out = append(out, m)
	}
	return out, nil
}

// Meta reads one book's meta.json.
func (s *Store) Meta(id string) (Meta, error) {
	b, err := os.ReadFile(s.MetaPath(id))
	if err != nil {
		return Meta{}, err
	}
	var m Meta
	if err := json.Unmarshal(b, &m); err != nil {
		return Meta{}, fmt.Errorf("book %s: meta.json is corrupt: %w", id, err)
	}
	return m, nil
}

// Text reads a book's text.txt and verifies it against the hash in meta.json.
// A mismatch is an error offering re-import, never a silent resume.
func (s *Store) Text(m Meta) ([]rune, error) {
	b, err := os.ReadFile(s.TextPath(m.ID))
	if err != nil {
		return nil, err
	}
	if got := Hash(b); got != m.SHA256 {
		return nil, fmt.Errorf("book %s: text.txt does not match its recorded hash "+
			"(re-import it: bt import <file>)", m.ID)
	}
	return []rune(string(b)), nil
}

// Current returns the current book's meta.
func (s *Store) Current() (Meta, error) {
	b, err := os.ReadFile(s.currentFile())
	if errors.Is(err, os.ErrNotExist) {
		books, lerr := s.List()
		if lerr == nil && len(books) == 0 {
			return Meta{}, ErrNoBooks
		}
		return Meta{}, ErrNoCurrent
	}
	if err != nil {
		return Meta{}, err
	}
	id := strings.TrimSpace(string(b))
	m, err := s.Meta(id)
	if err != nil {
		return Meta{}, fmt.Errorf("current book %q is not readable: %w", id, err)
	}
	return m, nil
}

// SetCurrent points the store at a book.
func (s *Store) SetCurrent(id string) error {
	if _, err := s.Meta(id); err != nil {
		return err
	}
	return writeAtomic(s.currentFile(), []byte(id+"\n"))
}

// Find resolves a user-typed name to a book: an exact id, else a unique
// case-insensitive substring of an id or a title.
func (s *Store) Find(name string) (Meta, error) {
	books, err := s.List()
	if err != nil {
		return Meta{}, err
	}
	if len(books) == 0 {
		return Meta{}, ErrNoBooks
	}
	for _, m := range books {
		if m.ID == name {
			return m, nil
		}
	}
	needle := strings.ToLower(name)
	var hits []Meta
	for _, m := range books {
		if strings.Contains(strings.ToLower(m.ID), needle) ||
			strings.Contains(strings.ToLower(m.Title), needle) {
			hits = append(hits, m)
		}
	}
	switch len(hits) {
	case 0:
		return Meta{}, fmt.Errorf("no book matches %q (bt list shows them all)", name)
	case 1:
		return hits[0], nil
	default:
		var ids []string
		for _, m := range hits {
			ids = append(ids, m.ID)
		}
		return Meta{}, fmt.Errorf("%q matches %s", name, strings.Join(ids, ", "))
	}
}

// Hash is the content hash recorded in meta.json and checked on every load.
func Hash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// writeAtomic writes through a temp file and a rename, as progress does.
func writeAtomic(path string, b []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)

	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// marshalIndent is the one place meta.json's formatting is decided.
func marshalIndent(v any) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
