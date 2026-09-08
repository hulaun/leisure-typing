package convert

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandles(t *testing.T) {
	for _, ext := range []string{".pdf", ".mobi", ".azw3", ".fb2", ".PDF"} {
		if !Handles(ext) {
			t.Errorf("Handles(%q) = false", ext)
		}
	}
	// The native formats must not be routed through a converter.
	for _, ext := range []string{".txt", ".epub", ".jpg", ""} {
		if Handles(ext) {
			t.Errorf("Handles(%q) = true", ext)
		}
	}
}

// Without a converter the message names both tools and recommends EPUB,
// rather than failing vaguely or extracting badly and quietly.
func TestErrNoConverterIsSpecific(t *testing.T) {
	msg := ErrNoConverter(".pdf").Error()
	for _, want := range []string{"pdftotext", "poppler", "ebook-convert", "Calibre", ".epub"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the message does not mention %q:\n%s", want, msg)
		}
	}
}

func TestUnknownExtension(t *testing.T) {
	if _, err := ToText("book.xyz"); err == nil {
		t.Error("ToText accepted an extension no converter handles")
	}
}

// The environment override exists so a Calibre installed outside PATH still
// works. It must point at something real to count.
func TestEnvOverride(t *testing.T) {
	fake := filepath.Join(t.TempDir(), "ebook-convert.exe")
	if err := os.WriteFile(fake, []byte("not really an executable"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EbookConvert.Env, fake)
	if got := EbookConvert.Path(); got != fake {
		t.Errorf("Path() = %q, want the override %q", got, fake)
	}

	t.Setenv(EbookConvert.Env, filepath.Join(t.TempDir(), "does-not-exist.exe"))
	if got := EbookConvert.Path(); got == filepath.Join(t.TempDir(), "does-not-exist.exe") {
		t.Error("an override pointing at nothing was used anyway")
	}
}

// A converter that is not installed must be reported as missing, not run.
func TestMissingToolIsNotRun(t *testing.T) {
	tool := Tool{Name: "definitely-not-a-real-program-xyz", What: "nothing", Env: "LEISURE_NOPE"}
	if tool.Available() {
		t.Skip("a program by that name exists on this machine")
	}
	if p := tool.Path(); p != "" {
		t.Errorf("Path() = %q for a tool that is not installed", p)
	}
}
