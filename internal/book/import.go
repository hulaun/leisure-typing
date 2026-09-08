package book

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/hulaun/leisure-typing/internal/convert"
	"github.com/hulaun/leisure-typing/internal/epub"
	"github.com/hulaun/leisure-typing/internal/norm"
	"github.com/hulaun/leisure-typing/internal/progress"
)

// Import reads a book file, extracts and normalizes its text, and writes the
// book into the store.
//
// Extraction happens once, here (CLAUDE.md, decision 1). The source file is
// never read again while typing: the runtime cost of opening a book is one
// os.ReadFile of an immutable text.txt and a slice into it.
func (s *Store) Import(path string) (Meta, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Meta{}, err
	}
	if info.IsDir() {
		return Meta{}, fmt.Errorf("%s is a directory", path)
	}

	ext := strings.ToLower(filepath.Ext(path))
	var raw, title, author string

	switch {
	case ext == ".txt", ext == ".text", ext == "":
		b, err := os.ReadFile(path)
		if err != nil {
			return Meta{}, err
		}
		raw = string(b)

	case ext == ".epub":
		b, err := epub.Extract(path)
		if err != nil {
			return Meta{}, fmt.Errorf("reading %s: %w", filepath.Base(path), err)
		}
		raw, title, author = b.Text, b.Title, b.Author

	case convert.Handles(ext):
		raw, err = convert.ToText(path)
		if err != nil {
			return Meta{}, err
		}

	default:
		return Meta{}, fmt.Errorf("%s is not a format this reads (.epub is the one to prefer; "+
			".txt, .pdf, .mobi, .azw3 and .fb2 also work)", ext)
	}

	// The one normalization pass. Everything downstream — the wrapper, the
	// renderer, the offsets — depends on the ASCII it guarantees.
	text := norm.Normalize(raw)
	if strings.TrimSpace(text) == "" {
		return Meta{}, fmt.Errorf("%s came out empty after extraction", filepath.Base(path))
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}

	if title == "" {
		title = titleFromFilename(path)
	}
	id, err := s.freeID(slug(title))
	if err != nil {
		return Meta{}, err
	}

	dir := s.Dir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Meta{}, err
	}

	body := []byte(text)
	m := Meta{
		ID:       id,
		Title:    title,
		Author:   author,
		Source:   filepath.Base(path),
		Format:   strings.TrimPrefix(ext, "."),
		SHA256:   Hash(body),
		Runes:    len([]rune(text)),
		Words:    countWords(text),
		Imported: time.Now(),
		Chapters: findChapters(text),
	}
	if m.Format == "" {
		m.Format = "txt"
	}

	if err := writeAtomic(s.TextPath(id), body); err != nil {
		return Meta{}, err
	}
	if err := s.writeMeta(m); err != nil {
		return Meta{}, err
	}
	if err := progress.Save(s.ProgressPath(id), progress.Progress{SHA256: m.SHA256}); err != nil {
		return Meta{}, err
	}
	// The original is kept, so re-import after a change to the extractor does
	// not mean hunting for the file again.
	if err := copyFile(path, filepath.Join(dir, "source"+ext)); err != nil {
		return Meta{}, err
	}
	if err := s.SetCurrent(id); err != nil {
		return Meta{}, err
	}
	return m, nil
}

func (s *Store) writeMeta(m Meta) error {
	b, err := marshalIndent(m)
	if err != nil {
		return err
	}
	return writeAtomic(s.MetaPath(m.ID), b)
}

// freeID gives the book a directory name, suffixing on a collision so that
// importing two books with the same title never overwrites the first.
func (s *Store) freeID(base string) (string, error) {
	if base == "" {
		base = "book"
	}
	id := base
	for n := 2; ; n++ {
		if _, err := os.Stat(s.Dir(id)); os.IsNotExist(err) {
			return id, nil
		} else if err != nil {
			return "", err
		}
		id = fmt.Sprintf("%s-%d", base, n)
		if n > 999 {
			return "", fmt.Errorf("too many books named %q", base)
		}
	}
}

// slug turns a title into a directory name: lowercase, ASCII, hyphenated.
func slug(s string) string {
	var b strings.Builder
	lastHyphen := true // suppresses a leading hyphen
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastHyphen = false
		default:
			if !lastHyphen {
				b.WriteByte('-')
				lastHyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

// titleFromFilename is the fallback when the format carries no metadata:
// "the-great-book_v2.txt" becomes "The Great Book V2".
func titleFromFilename(path string) string {
	name := filepath.Base(path)
	name = strings.TrimSuffix(name, filepath.Ext(name))
	name = strings.NewReplacer("_", " ", "-", " ", ".", " ").Replace(name)
	fields := strings.Fields(name)
	for i, f := range fields {
		r := []rune(f)
		r[0] = unicode.ToUpper(r[0])
		fields[i] = string(r)
	}
	if len(fields) == 0 {
		return "Book"
	}
	return strings.Join(fields, " ")
}

func countWords(s string) int { return len(strings.Fields(s)) }

// chapterLine matches a heading standing alone on its line. It is the same
// shape the normalizer uses to find the end of the front matter, and it is
// deliberately conservative: a missed chapter costs a status line, a false one
// would put the reader in the wrong place.
var chapterLine = regexp.MustCompile(
	`^\s*((?:CHAPTER|Chapter|PART|Part|BOOK|Book|PROLOGUE|Prologue|EPILOGUE|Epilogue)\b.{0,60}|` +
		`[IVXLC]{1,7}\.?|\d{1,3}\.?)\s*$`)

// findChapters records where each heading starts, as a rune offset into the
// text, for the status line.
func findChapters(text string) []Chapter {
	var out []Chapter
	offset := 0
	for _, line := range strings.SplitAfter(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && chapterLine.MatchString(trimmed) {
			out = append(out, Chapter{Title: trimmed, Offset: offset})
		}
		offset += len([]rune(line))
	}
	if len(out) > 500 {
		// Something matched far too eagerly — a book of numbered lines, say.
		// No chapters at all is better than five hundred wrong ones.
		return nil
	}
	return out
}

// ChapterAt returns the heading in force at a rune offset.
func (m Meta) ChapterAt(offset int) string {
	title := ""
	for _, c := range m.Chapters {
		if c.Offset > offset {
			break
		}
		title = c.Title
	}
	return title
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return writeAtomic(dst, b)
}
