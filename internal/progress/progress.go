// Package progress reads and writes the reading position.
//
// Losing ten minutes of a five-week book is the worst bug this app can have,
// so every write goes through a temp file and a rename: a crash mid-write
// cannot leave a truncated progress.json behind.
package progress

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Progress is the whole of progress.json.
type Progress struct {
	// Offset is a rune offset into text.txt — never a byte offset, and never
	// a line or page number. text.txt is immutable, so this stays valid for
	// the life of the import.
	Offset int `json:"offset"`

	// Updated is when the position was last written.
	Updated time.Time `json:"updated"`

	// SHA256 is the hash of the text.txt this offset was measured against.
	// It is checked on load: if the extractor changes and the book is
	// re-imported, an old offset points somewhere else entirely, and
	// resuming against it silently drops the reader into the wrong chapter.
	SHA256 string `json:"sha256"`
}

// ErrStale is returned by Load when the saved position was measured against a
// different text.txt than the one on disk now.
var ErrStale = errors.New("progress: saved position does not match this text (re-import the book)")

// Load reads the progress file at path and checks it against the hash of the
// text it is meant to index. A missing file is not an error: a book that has
// never been opened is at offset zero.
func Load(path, sha256 string) (Progress, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Progress{SHA256: sha256}, nil
	}
	if err != nil {
		return Progress{}, err
	}

	var p Progress
	if err := json.Unmarshal(b, &p); err != nil {
		return Progress{}, fmt.Errorf("progress: %s is corrupt: %w", path, err)
	}
	if p.SHA256 != "" && sha256 != "" && p.SHA256 != sha256 {
		return Progress{}, ErrStale
	}
	if p.Offset < 0 {
		p.Offset = 0
	}
	p.SHA256 = sha256
	return p, nil
}

// Save writes the position atomically: a temp file in the same directory,
// flushed, then renamed over the target. The rename is what makes it atomic,
// and the same directory is what makes the rename possible.
func Save(path string, p Progress) error {
	if p.Offset < 0 {
		p.Offset = 0
	}
	p.Updated = time.Now()

	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".progress-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name) // a no-op once the rename has succeeded

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
	// Windows will not rename over an existing file with os.Rename in every
	// case, but Go's implementation uses MoveFileEx with REPLACE_EXISTING,
	// which does.
	return os.Rename(name, path)
}
