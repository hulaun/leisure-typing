package progress

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "progress.json")
	if err := Save(path, Progress{Offset: 4210, SHA256: "abc"}); err != nil {
		t.Fatal(err)
	}
	p, err := Load(path, "abc")
	if err != nil {
		t.Fatal(err)
	}
	if p.Offset != 4210 {
		t.Errorf("Offset = %d, want 4210", p.Offset)
	}
	if p.Updated.IsZero() {
		t.Error("Updated was not stamped")
	}
}

// A book that has never been opened is at offset zero, not an error.
func TestLoadMissingIsZero(t *testing.T) {
	p, err := Load(filepath.Join(t.TempDir(), "nothing.json"), "abc")
	if err != nil {
		t.Fatalf("Load of a missing file: %v", err)
	}
	if p.Offset != 0 || p.SHA256 != "abc" {
		t.Errorf("got %+v", p)
	}
}

// The whole point of the hash: a changed text.txt must not resume silently at
// an offset that now points somewhere else.
func TestStaleHashIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "progress.json")
	if err := Save(path, Progress{Offset: 900, SHA256: "old"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, "new"); !errors.Is(err, ErrStale) {
		t.Errorf("Load with a changed hash: err = %v, want ErrStale", err)
	}
}

func TestCorruptFileIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "progress.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path, "abc"); err == nil {
		t.Error("a corrupt progress.json loaded without complaint")
	}
}

// Saving repeatedly must leave exactly one file behind: the temp files are
// renamed, not accumulated.
func TestSaveLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "progress.json")
	for i := 0; i < 5; i++ {
		if err := Save(path, Progress{Offset: i, SHA256: "abc"}); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("directory holds %v, want just progress.json", names)
	}
	p, err := Load(path, "abc")
	if err != nil || p.Offset != 4 {
		t.Errorf("after five saves: %+v, %v", p, err)
	}
}

func TestNegativeOffsetIsClamped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "progress.json")
	if err := Save(path, Progress{Offset: -5, SHA256: "abc"}); err != nil {
		t.Fatal(err)
	}
	if p, _ := Load(path, "abc"); p.Offset != 0 {
		t.Errorf("Offset = %d, want 0", p.Offset)
	}
}
