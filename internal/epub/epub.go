// Package epub extracts a book's text from an .epub file.
//
// An EPUB is a ZIP archive of XHTML documents plus the metadata that says
// which order to read them in. The mechanical shape of the walk is
// archive/zip and encoding/xml, both stdlib; the judgement is in block-level
// element handling, what to drop, and where paragraph breaks belong.
//
// See SPEC.md for the container layout and the reduction rules.
package epub

import "errors"

// ErrNotImplemented is returned by Extract until the user writes it.
var ErrNotImplemented = errors.New("epub: Extract is not implemented (RESERVED: see internal/epub/SPEC.md)")

// Book is what Extract recovers from the archive.
type Book struct {
	Title  string // from the OPF <dc:title>, "" if absent
	Author string // from the OPF <dc:creator>, "" if absent

	// Text is the whole book in reading order, as text. Paragraphs are
	// separated by a blank line. It is not yet normalized — it is still
	// Unicode, and norm.Normalize is what makes it typeable.
	Text string
}

// Extract reads the EPUB at path and returns its text in reading order.
//
// It returns an error for anything that is not a readable EPUB: a file that is
// not a ZIP, a missing or malformed container, an OPF that cannot be parsed,
// a spine that resolves to no documents. A book whose text comes out empty is
// an error too — silently importing nothing is the worst outcome.
//
// RESERVED: this is the user's to write. See CLAUDE.md, "Division of work".
func Extract(path string) (*Book, error) {
	// TODO(user): the container/OPF/spine walk and the XHTML reduction.
	return nil, ErrNotImplemented
}
