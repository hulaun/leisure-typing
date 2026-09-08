// Package epub extracts a book's text from an .epub file.
//
// An EPUB is a ZIP archive of XHTML documents plus the metadata that says
// which order to read them in. Four steps, each of which can fail and each of
// which fails loudly: the container, the package document, the spine, and the
// reduction of each document to text.
//
// See SPEC.md for the rules; testdata/minimal.epub exercises all of them.
package epub

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"unicode"
)

// Book is what Extract recovers from the archive.
type Book struct {
	Title  string // from the OPF <dc:title>, "" if absent
	Author string // from the OPF <dc:creator>, "" if absent

	// Text is the whole book in reading order, as text. Paragraphs are
	// separated by a blank line. It is not yet normalized — it is still
	// Unicode, and norm.Normalize is what makes it typeable. Keeping the two
	// apart means the character table can change without re-reading any EPUB.
	Text string
}

// Extract reads the EPUB at name and returns its text in reading order.
//
// It returns an error for anything that is not a readable EPUB: a file that is
// not a ZIP, a missing or malformed container, an OPF that cannot be parsed,
// a spine that resolves to no documents. A book whose text comes out empty is
// an error too — silently importing nothing is the worst outcome.
func Extract(name string) (*Book, error) {
	zr, err := zip.OpenReader(name)
	if err != nil {
		return nil, fmt.Errorf("%s could not be opened as an EPUB "+
			"(it has to be a ZIP archive): %w", filepath.Base(name), err)
	}
	defer zr.Close()

	files := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		files[f.Name] = f
	}

	if err := checkMimetype(files); err != nil {
		return nil, err
	}
	opfPath, err := rootfile(files)
	if err != nil {
		return nil, err
	}
	pkg, err := readOPF(files, opfPath)
	if err != nil {
		return nil, err
	}

	book := &Book{
		Title:  firstNonEmpty(pkg.Metadata.Title),
		Author: firstNonEmpty(pkg.Metadata.Creator),
	}

	// hrefs are relative to the OPF's own directory, not to the ZIP root.
	// This is the single most common way a first implementation goes wrong.
	base := dirOf(opfPath)

	// The manifest is a lookup, not an order. The spine is the order.
	href := make(map[string]string, len(pkg.Manifest.Items))
	for _, it := range pkg.Manifest.Items {
		href[it.ID] = it.Href
	}

	var docs []string
	for _, ref := range pkg.Spine.Refs {
		if strings.EqualFold(ref.Linear, "no") {
			continue // material outside the main reading flow
		}
		h, ok := href[ref.IDRef]
		if !ok {
			continue // a spine entry naming nothing: skip it rather than fail
		}
		entry, err := resolve(base, h)
		if err != nil {
			continue
		}
		f, ok := files[entry]
		if !ok {
			continue
		}
		text, err := document(f)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", entry, err)
		}
		if strings.TrimSpace(text) != "" {
			docs = append(docs, text)
		}
	}

	if len(docs) == 0 {
		return nil, fmt.Errorf("%s has no readable text in its spine", filepath.Base(name))
	}

	// A chapter boundary is a paragraph boundary.
	book.Text = tidy(strings.Join(docs, "\n\n"))
	if book.Text == "" {
		return nil, fmt.Errorf("%s came out empty", filepath.Base(name))
	}
	return book, nil
}

// --- the container walk -----------------------------------------------------

// checkMimetype verifies the archive says what it is.
//
// The entry is meant to be first and stored uncompressed; neither is required
// here, because plenty of real books in the wild get that wrong and their
// content is still fine. A *wrong* mimetype is worth refusing; a missing one
// is not, since container.xml is the real test of whether this is an EPUB.
func checkMimetype(files map[string]*zip.File) error {
	f, ok := files["mimetype"]
	if !ok {
		return nil
	}
	b, err := read(f)
	if err != nil {
		return err
	}
	if got := strings.TrimSpace(string(b)); got != "application/epub+zip" {
		return fmt.Errorf("this is a ZIP archive but not an EPUB: its mimetype is %q", got)
	}
	return nil
}

type containerXML struct {
	Rootfiles []struct {
		FullPath string `xml:"full-path,attr"`
	} `xml:"rootfiles>rootfile"`
}

// rootfile finds the package document the container points at.
func rootfile(files map[string]*zip.File) (string, error) {
	f, ok := files["META-INF/container.xml"]
	if !ok {
		return "", fmt.Errorf("no META-INF/container.xml: this is not an EPUB")
	}
	b, err := read(f)
	if err != nil {
		return "", err
	}
	var c containerXML
	if err := xml.Unmarshal(b, &c); err != nil {
		return "", fmt.Errorf("META-INF/container.xml is malformed: %w", err)
	}
	for _, rf := range c.Rootfiles {
		if rf.FullPath != "" {
			return path.Clean(rf.FullPath), nil
		}
	}
	return "", fmt.Errorf("META-INF/container.xml names no package document")
}

// opfXML is the package document: what to read, and in what order.
//
// The struct tags carry local names only, so they match whichever namespace
// prefix the book happens to use — dc:title and title alike.
type opfXML struct {
	Metadata struct {
		Title   []string `xml:"title"`
		Creator []string `xml:"creator"`
	} `xml:"metadata"`
	Manifest struct {
		Items []struct {
			ID   string `xml:"id,attr"`
			Href string `xml:"href,attr"`
		} `xml:"item"`
	} `xml:"manifest"`
	Spine struct {
		Refs []struct {
			IDRef  string `xml:"idref,attr"`
			Linear string `xml:"linear,attr"`
		} `xml:"itemref"`
	} `xml:"spine"`
}

func readOPF(files map[string]*zip.File, name string) (*opfXML, error) {
	f, ok := files[name]
	if !ok {
		return nil, fmt.Errorf("the package document %s is not in the archive", name)
	}
	b, err := read(f)
	if err != nil {
		return nil, err
	}
	var pkg opfXML
	if err := xml.Unmarshal(b, &pkg); err != nil {
		return nil, fmt.Errorf("%s is malformed: %w", name, err)
	}
	if len(pkg.Spine.Refs) == 0 {
		return nil, fmt.Errorf("%s has an empty spine: there is no reading order", name)
	}
	return &pkg, nil
}

// resolve turns an href from the OPF into a ZIP entry name: relative to the
// OPF's directory, percent-decoded, without its fragment.
func resolve(base, href string) (string, error) {
	if i := strings.IndexByte(href, '#'); i >= 0 {
		href = href[:i]
	}
	decoded, err := url.PathUnescape(href)
	if err != nil {
		decoded = href // a stray % is not a reason to lose the chapter
	}
	if decoded == "" {
		return "", fmt.Errorf("empty href")
	}
	return path.Join(base, decoded), nil
}

func dirOf(p string) string {
	if d := path.Dir(p); d != "." {
		return d
	}
	return ""
}

// --- XHTML to text ----------------------------------------------------------

// Dropped entirely, along with their character data. Tables in novels are
// almost always layout artefacts, and typing one is miserable.
var dropped = map[string]bool{
	"script": true, "style": true, "head": true, "title": true,
	"meta": true, "link": true, "svg": true, "img": true,
	"figure": true, "figcaption": true, "table": true,
}

// A paragraph break before and after. Everything not named here and not
// dropped is inline: it adds no break and no space.
var block = map[string]bool{
	"p": true, "div": true, "blockquote": true, "li": true, "pre": true,
	"section": true, "article": true, "hr": true, "nav": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
}

// document reduces one XHTML file to text.
func document(f *zip.File) (string, error) {
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	d := xml.NewDecoder(rc)
	// Real books contain undeclared HTML entities. Without these three lines
	// the decoder stops at the first &nbsp; in the book.
	d.Strict = false
	d.AutoClose = xml.HTMLAutoClose
	d.Entity = xml.HTMLEntity

	var b strings.Builder
	depth := 0
	skipFrom := -1 // the depth of the dropped element we are inside, or -1

	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			if skipFrom >= 0 {
				continue
			}
			name := strings.ToLower(t.Name.Local)
			if dropped[name] || isNoteref(name, t.Attr) {
				skipFrom = depth
				continue
			}
			if name == "br" {
				b.WriteByte('\n') // a line break, not a paragraph break
				continue
			}
			if block[name] {
				b.WriteString("\n\n")
			}

		case xml.EndElement:
			if skipFrom >= 0 {
				if depth == skipFrom {
					skipFrom = -1
				}
				depth--
				continue
			}
			if block[strings.ToLower(t.Name.Local)] {
				b.WriteString("\n\n")
			}
			depth--

		case xml.CharData:
			if skipFrom >= 0 {
				continue
			}
			b.WriteString(squeeze(string(t)))
		}
	}
	return b.String(), nil
}

// isNoteref reports whether an <a> is a footnote marker rather than prose.
// The marker is not part of the sentence and nobody wants to type it.
func isNoteref(name string, attrs []xml.Attr) bool {
	if name != "a" {
		return false
	}
	for _, a := range attrs {
		switch strings.ToLower(a.Name.Local) {
		case "type": // epub:type="noteref"
			if strings.Contains(strings.ToLower(a.Value), "noteref") {
				return true
			}
		case "class":
			v := strings.ToLower(a.Value)
			if strings.Contains(v, "footnote") || strings.Contains(v, "noteref") {
				return true
			}
		}
	}
	return false
}

// squeeze collapses a run of text's internal whitespace to single spaces and
// keeps its edges: the space between "The " and an <em> belongs to the
// sentence, and dropping it would weld the words together.
func squeeze(s string) string {
	var b strings.Builder
	space := false
	for _, r := range s {
		if unicode.IsSpace(r) {
			space = true
			continue
		}
		if space {
			b.WriteByte(' ')
			space = false
		}
		b.WriteRune(r)
	}
	if space {
		b.WriteByte(' ')
	}
	return b.String()
}

// tidy gives the joined documents the shape the contract promises: no line
// with trailing whitespace, no run of blank lines, no blank line at either
// end, and exactly one newline to finish.
func tidy(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := false
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			blank = true
			continue
		}
		if blank && len(out) > 0 {
			out = append(out, "")
		}
		blank = false
		out = append(out, l)
	}
	if len(out) == 0 {
		return ""
	}
	return strings.Join(out, "\n") + "\n"
}

// --- small helpers ----------------------------------------------------------

func read(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

func firstNonEmpty(ss []string) string {
	for _, s := range ss {
		if t := strings.TrimSpace(s); t != "" {
			return t
		}
	}
	return ""
}
