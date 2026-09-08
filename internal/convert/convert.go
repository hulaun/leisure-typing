// Package convert finds and runs the external tools that turn a format we do
// not parse into plain text.
//
// A PDF has no text, only glyphs at coordinates. Extracting from it means
// solving reading order across columns, repeated headers and footers, hyphens
// broken across lines, ligatures, and fonts with absent or broken ToUnicode
// maps. It is weeks of work and still wrong sometimes, so it is not done in
// process — see CLAUDE.md, "Format support". If no converter is on PATH we
// say so and recommend EPUB rather than doing a bad job quietly.
package convert

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// A Tool is an external converter.
type Tool struct {
	Name string // the executable, as it is looked up on PATH
	What string // what it comes from, for the error message
	Env  string // an environment variable that overrides the PATH lookup
}

var (
	// PDFToText comes with poppler-utils.
	PDFToText = Tool{Name: "pdftotext", What: "poppler-utils", Env: "LEISURE_PDFTOTEXT"}
	// EbookConvert comes with Calibre.
	EbookConvert = Tool{Name: "ebook-convert", What: "Calibre", Env: "LEISURE_EBOOK_CONVERT"}
)

// Path returns the tool's executable path, or "" if it cannot be found. The
// environment override wins, so a Calibre installed outside PATH still works.
func (t Tool) Path() string {
	if p := os.Getenv(t.Env); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath(t.Name); err == nil {
		return p
	}
	return ""
}

// Available reports whether the tool can be run.
func (t Tool) Available() bool { return t.Path() != "" }

// ErrNoConverter names both tools and recommends EPUB. It is the message a
// user without either one sees, and it is deliberately specific: "conversion
// failed" sends nobody anywhere.
func ErrNoConverter(ext string) error {
	return fmt.Errorf(`%s needs an external converter, and neither is installed.

  pdftotext      from poppler-utils   (winget install oschwartz10612.Poppler)
  ebook-convert  from Calibre         (winget install calibre.calibre)

Either will do. Better still, find the book as .epub — it is the format this
reads natively and the one that comes out cleanest`, ext)
}

// Timeout bounds a conversion. A novel takes a second or two; a minute means
// something has gone wrong and hanging forever is worse than failing.
var Timeout = 2 * time.Minute

// ToText converts the book at src into plain text, choosing a converter by
// extension. The returned text is still Unicode — norm.Normalize is the next
// step, exactly as it is for a native .txt.
func ToText(src string) (string, error) {
	ext := strings.ToLower(filepath.Ext(src))
	switch ext {
	case ".pdf":
		// -layout keeps the reading order of a single-column book roughly
		// intact, which is the best a PDF affords.
		if PDFToText.Available() {
			return run(PDFToText, src, ".txt", func(in, out string) []string {
				return []string{"-layout", "-enc", "UTF-8", in, out}
			})
		}
		if EbookConvert.Available() {
			return run(EbookConvert, src, ".txt", func(in, out string) []string {
				return []string{in, out}
			})
		}
		return "", ErrNoConverter(ext)

	case ".mobi", ".azw", ".azw3", ".fb2", ".lit", ".pdb", ".rtf", ".doc", ".docx":
		if EbookConvert.Available() {
			return run(EbookConvert, src, ".txt", func(in, out string) []string {
				return []string{in, out}
			})
		}
		return "", ErrNoConverter(ext)
	}
	return "", fmt.Errorf("no converter knows how to read %s", ext)
}

// Handles reports whether ToText has a converter path for this extension —
// whether or not the tool is actually installed.
func Handles(ext string) bool {
	switch strings.ToLower(ext) {
	case ".pdf", ".mobi", ".azw", ".azw3", ".fb2", ".lit", ".pdb", ".rtf", ".doc", ".docx":
		return true
	}
	return false
}

// run executes the tool into a temp file and reads the result back.
func run(t Tool, src, outExt string, args func(in, out string) []string) (string, error) {
	dir, err := os.MkdirTemp("", "leisure-convert-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	abs, err := filepath.Abs(src)
	if err != nil {
		return "", err
	}
	out := filepath.Join(dir, "out"+outExt)

	ctx, cancel := context.WithTimeout(context.Background(), Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, t.Path(), args(abs, out)...)
	stderr := &strings.Builder{}
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("%s took longer than %s and was stopped", t.Name, Timeout)
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s failed: %s", t.Name, msg)
	}

	b, err := os.ReadFile(out)
	if err != nil {
		return "", fmt.Errorf("%s produced no output for %s", t.Name, filepath.Base(src))
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return "", fmt.Errorf("%s extracted no text from %s — if it is a scan rather than "+
			"a text PDF, no converter will help; find the book as .epub",
			t.Name, filepath.Base(src))
	}
	return string(b), nil
}
