package book

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hulaun/leisure-typing/internal/progress"
)

// A bundle is one book as a single file: the thing you drag across the cable.
//
// Getting a book onto the phone happens once per book, and Windows already
// knows how to copy a file to a phone — so this is a file rather than a sync
// protocol. What crosses afterwards is the reading position, which is one
// integer typed into the Place screen (cmd/bt/SPEC.md §3).
//
// The archive holds the *derived* book, not the source:
//
//	bundle.json     { version, exported }
//	meta.json       title, author, sha256, words, chapter offsets
//	text.txt        the normalized text, byte for byte
//	progress.json   where you were when you exported, so the first
//	                transfer carries your place as well as the book
//
// source.epub is deliberately absent. The laptop is the archive; the phone
// gets the derived text only, and extraction still happens exactly once
// (CLAUDE.md, decision 1).
//
// The invariant the whole thing rests on: text.txt must arrive byte-identical,
// because a reading position is a rune offset into it and its sha256 is
// checked on every load. That is why importing a bundle does not re-run the
// normalizer — it copies the bytes and verifies them against the hash the
// exporter recorded. Re-deriving the text on the far side is exactly what
// would turn "resume" back into a search problem.
const (
	// BundleExt is the extension, recognised by Import.
	BundleExt = ".btbook"

	// bundleVersion is the format version. It exists so that a future change
	// is detected rather than misread by an older copy of the app on the
	// other device, which is the whole hazard of a file format that crosses
	// between two machines that update separately.
	bundleVersion = 1

	// The largest text a bundle may carry once decompressed. A 200-page novel
	// is around 230 KB; this is a guard on a file that arrived from another
	// device, not a meaningful limit on a book.
	bundleTextLimit = 64 << 20
	bundleMetaLimit = 4 << 20
)

// bundleInfo is bundle.json.
type bundleInfo struct {
	Version  int       `json:"version"`
	Exported time.Time `json:"exported"`
}

// ErrNotABundle is returned when a file with the right extension is not a
// bundle at all.
var ErrNotABundle = errors.New("not a .btbook bundle")

// Export writes a book to dst as a single .btbook file.
//
// If dst is a directory, or ends in a separator, the file is named after the
// book's id inside it. The text is verified against its hash on the way out:
// exporting a book whose text.txt has been tampered with would hand the other
// device a position that silently resumes in the wrong place.
func (s *Store) Export(m Meta, dst string) (string, error) {
	text, err := s.Text(m) // this is the hash check
	if err != nil {
		return "", err
	}
	body := []byte(string(text))

	dst, err = bundlePath(dst, s.BundleDir(), m.ID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}

	// The position travels with the book, so the first transfer lands you
	// where you already were rather than at the start.
	p, err := progress.Load(s.ProgressPath(m.ID), m.SHA256)
	if err != nil {
		return "", err
	}

	info, err := marshalIndent(bundleInfo{Version: bundleVersion, Exported: time.Now()})
	if err != nil {
		return "", err
	}
	meta, err := marshalIndent(m)
	if err != nil {
		return "", err
	}
	prog, err := marshalIndent(p)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range []struct {
		name string
		body []byte
	}{
		{"bundle.json", info},
		{"meta.json", meta},
		{"text.txt", body},
		{"progress.json", prog},
	} {
		w, err := zw.CreateHeader(&zip.FileHeader{Name: f.name, Method: zip.Deflate})
		if err != nil {
			return "", err
		}
		if _, err := w.Write(f.body); err != nil {
			return "", err
		}
	}
	if err := zw.Close(); err != nil {
		return "", err
	}

	// Through a temp file and a rename, as every other write in the store is:
	// a crash mid-write must not leave a half-built bundle that looks
	// importable.
	if err := writeAtomic(dst, buf.Bytes()); err != nil {
		return "", err
	}
	return dst, nil
}

// bundlePath resolves the destination a user gave into a file path.
//
// With no destination the bundle lands in dir — bt-books/ beside the store —
// rather than in whatever directory bt happened to be run from. Exports are
// rare and are always looked for again later, so one known place beats a
// trail of .btbook files scattered across the disk.
func bundlePath(dst, dir, id string) (string, error) {
	if dst == "" {
		return filepath.Join(dir, id+BundleExt), nil
	}
	if strings.HasSuffix(dst, "/") || strings.HasSuffix(dst, string(os.PathSeparator)) {
		return filepath.Join(dst, id+BundleExt), nil
	}
	if info, err := os.Stat(dst); err == nil && info.IsDir() {
		return filepath.Join(dst, id+BundleExt), nil
	}
	if !strings.EqualFold(filepath.Ext(dst), BundleExt) {
		dst += BundleExt
	}
	return dst, nil
}

// importBundle reads a .btbook into the store.
//
// It does not normalize, does not re-derive chapters and does not recount
// words: everything but the identity of the book in this store is carried
// across as the exporter wrote it, so that a rune offset means the same thing
// on both devices. The one thing it does check is the hash.
func (s *Store) importBundle(path string) (Meta, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return Meta{}, fmt.Errorf("%s: %w (%v)", filepath.Base(path), ErrNotABundle, err)
	}
	defer zr.Close()

	files := map[string]*zip.File{}
	for _, f := range zr.File {
		// Only the four names below are read, and they are matched exactly.
		// Nothing from the archive is ever used as a path, so a crafted entry
		// name cannot escape the store.
		files[f.Name] = f
	}

	infoRaw, err := readBundleFile(files, "bundle.json", bundleMetaLimit)
	if err != nil {
		return Meta{}, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	var info bundleInfo
	if err := json.Unmarshal(infoRaw, &info); err != nil {
		return Meta{}, fmt.Errorf("%s: bundle.json is corrupt: %w", filepath.Base(path), err)
	}
	if info.Version > bundleVersion {
		return Meta{}, fmt.Errorf("%s was written by a newer version of bt "+
			"(bundle version %d, this understands %d) — update bt on this device",
			filepath.Base(path), info.Version, bundleVersion)
	}

	metaRaw, err := readBundleFile(files, "meta.json", bundleMetaLimit)
	if err != nil {
		return Meta{}, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	var m Meta
	if err := json.Unmarshal(metaRaw, &m); err != nil {
		return Meta{}, fmt.Errorf("%s: meta.json is corrupt: %w", filepath.Base(path), err)
	}

	body, err := readBundleFile(files, "text.txt", bundleTextLimit)
	if err != nil {
		return Meta{}, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}

	// The check the transfer rests on. A reading position is a rune offset
	// into these exact bytes, so a bundle whose text does not match its
	// recorded hash cannot be imported at all: the alternative is a book that
	// opens fine and resumes in the wrong chapter.
	if got := Hash(body); got != m.SHA256 {
		return Meta{}, fmt.Errorf("%s is damaged: its text does not match the hash "+
			"recorded in it (export it again)", filepath.Base(path))
	}

	// The position, if the bundle carried one that belongs to this text.
	offset := 0
	if raw, err := readBundleFile(files, "progress.json", bundleMetaLimit); err == nil {
		var p progress.Progress
		if json.Unmarshal(raw, &p) == nil && p.SHA256 == m.SHA256 {
			offset = p.Offset
		}
	}
	if offset < 0 {
		offset = 0
	}
	if n := len([]rune(string(body))); offset > n {
		offset = n
	}

	// Identity in *this* store: a fresh id, and the bundle as the source.
	// Everything else is the exporter's.
	base := m.ID
	if base == "" {
		base = slug(m.Title)
	}
	id, err := s.freeID(base)
	if err != nil {
		return Meta{}, err
	}
	m.ID = id
	m.Source = filepath.Base(path)
	m.Imported = time.Now()
	if m.Format == "" {
		m.Format = strings.TrimPrefix(BundleExt, ".")
	}

	if err := os.MkdirAll(s.Dir(id), 0o755); err != nil {
		return Meta{}, err
	}
	if err := writeAtomic(s.TextPath(id), body); err != nil {
		return Meta{}, err
	}
	if err := s.writeMeta(m); err != nil {
		return Meta{}, err
	}
	if err := progress.Save(s.ProgressPath(id), progress.Progress{
		Offset: offset,
		SHA256: m.SHA256,
	}); err != nil {
		return Meta{}, err
	}
	// The bundle is kept as the source, for the same reason every other
	// import keeps its original: re-importing should not mean hunting for the
	// file again.
	if err := copyFile(path, filepath.Join(s.Dir(id), "source"+BundleExt)); err != nil {
		return Meta{}, err
	}
	if err := s.SetCurrent(id); err != nil {
		return Meta{}, err
	}
	return m, nil
}

// readBundleFile reads one named entry, refusing anything implausibly large.
// The limit is a guard on a file that arrived from another device.
func readBundleFile(files map[string]*zip.File, name string, limit int64) ([]byte, error) {
	f, ok := files[name]
	if !ok {
		return nil, fmt.Errorf("%w: it has no %s", ErrNotABundle, name)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("%s is unreadable: %w", name, err)
	}
	defer rc.Close()

	b, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%s is unreadable: %w", name, err)
	}
	if int64(len(b)) > limit {
		return nil, fmt.Errorf("%s is larger than %d bytes", name, limit)
	}
	return b, nil
}
