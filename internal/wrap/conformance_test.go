package wrap_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hulaun/leisure-typing/internal/wrap"
)

// The conformance golden: the exact lines this package produces for a fixed
// fixture at a fixed set of widths.
//
// It exists because there are now two implementations of the wrapper — this one
// and the Kotlin port in android/ — and they have to agree about what an offset
// means. If they disagree, a position carried from one device to the other
// lands somewhere else, which is the one failure the whole design is arranged
// to prevent and the one the user would have no way to diagnose.
//
// Both sides verify against this file; neither is the reference for the other.
// The Kotlin test reads the same two files out of testdata/conformance, wired
// into its test resources by android/app/build.gradle.kts.
//
// Regenerate after a deliberate change to the wrapping:
//
//	LEISURE_GOLDEN=1 go test ./internal/wrap/
//
// and then run the Kotlin test, which will fail if the port has drifted.

var (
	goldenWidths  = []int{20, 33, 40, 68}
	goldenOffsets = []int{0, 1, 10, 50, 120, 200, 260, 300, 400, 500, 600}
)

func conformanceDir() string {
	return filepath.Join("..", "..", "testdata", "conformance")
}

func TestWrapConformanceGolden(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(conformanceDir(), "fixture.txt"))
	if err != nil {
		t.Fatal(err)
	}
	// The fixture must be byte-identical on both sides, so a checkout that
	// translated its line endings has to fail loudly rather than quietly
	// produce a different golden.
	if strings.Contains(string(raw), "\r") {
		t.Fatal("fixture.txt has CRLF line endings; it must be LF " +
			"(the two ports index the same bytes)")
	}
	text := []rune(string(raw))

	got := buildGolden(text)
	path := filepath.Join(conformanceDir(), "wrap-golden.txt")

	if os.Getenv("LEISURE_GOLDEN") != "" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s (%d bytes) — now run the Kotlin conformance test", path, len(got))
		return
	}

	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v\n\nregenerate with: LEISURE_GOLDEN=1 go test ./internal/wrap/", err)
	}
	if strings.ReplaceAll(string(want), "\r\n", "\n") != got {
		t.Error("the wrapping no longer matches the conformance golden.\n" +
			"If the change was deliberate: regenerate with " +
			"LEISURE_GOLDEN=1 go test ./internal/wrap/ and then run the " +
			"Kotlin conformance test, which will fail until the port is updated.")
		writeDiff(t, string(want), got)
	}
}

// buildGolden is the format both ports produce. Text is last on the line
// because it contains spaces; it can never contain a newline, which is what
// makes one record one line.
func buildGolden(text []rune) string {
	var b strings.Builder
	b.WriteString("# leisure-typing wrap conformance golden\n")
	b.WriteString("# L <width> <index> <start> <end> <blank> <text>\n")
	b.WriteString("# W <width> <offset> <activeStart> <activeEnd> <lines> <text>\n")

	for _, width := range goldenWidths {
		for i, l := range wrap.Lines(text, width) {
			blank := 0
			if l.Blank {
				blank = 1
			}
			fmt.Fprintf(&b, "L %d %d %d %d %d %s\n", width, i, l.Start, l.End, blank, l.Text)
		}
	}

	// Window is what the line movement uses, so it is checked separately from
	// Lines: an agreeing Lines and a disagreeing Window would still send Down
	// to the wrong place.
	for _, width := range goldenWidths {
		for _, off := range goldenOffsets {
			if off > len(text) {
				continue
			}
			lines, active := wrap.Window(text, off, width, 2, 2)
			a := lines[active]
			fmt.Fprintf(&b, "W %d %d %d %d %d %s\n",
				width, off, a.Start, a.End, len(lines), a.Text)
		}
	}
	return b.String()
}

// writeDiff reports the first few differing records, which is enough to see
// what moved without printing the whole file.
func writeDiff(t *testing.T, want, got string) {
	t.Helper()
	w := strings.Split(strings.TrimRight(want, "\n"), "\n")
	g := strings.Split(strings.TrimRight(got, "\n"), "\n")
	shown := 0
	for i := 0; i < len(w) || i < len(g); i++ {
		var lw, lg string
		if i < len(w) {
			lw = w[i]
		}
		if i < len(g) {
			lg = g[i]
		}
		if lw != lg {
			t.Errorf("line %d:\n  golden %q\n  now    %q", i+1, lw, lg)
			if shown++; shown >= 5 {
				t.Errorf("(and possibly more)")
				return
			}
		}
	}
}
