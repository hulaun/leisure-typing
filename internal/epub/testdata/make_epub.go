//go:build ignore

// make_epub.go builds testdata/minimal.epub, the hand-built fixture the
// Extract tests run against. It is checked in as a generator rather than only
// as a binary blob so that the fixture can be read and changed.
//
//	go run make_epub.go
package main

import (
	"archive/zip"
	"log"
	"os"
)

// The fixture exercises, in order: the mimetype entry, container.xml pointing
// at an OPF in a subdirectory, hrefs relative to that subdirectory (including
// one a level deeper and one percent-encoded), spine order that differs from
// manifest order, a linear="no" document that must be skipped, dropped
// elements, inline elements that must not introduce spaces, <br/>, and a
// noteref footnote marker that must not survive.
var entries = []struct {
	name, body string
	store      bool
}{
	{name: "mimetype", body: "application/epub+zip", store: true},

	{name: "META-INF/container.xml", body: `<?xml version="1.0"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
  <rootfiles>
    <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
  </rootfiles>
</container>
`},

	{name: "OEBPS/content.opf", body: `<?xml version="1.0" encoding="utf-8"?>
<package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="id">
  <metadata xmlns:dc="http://purl.org/dc/elements/1.1/">
    <dc:title>A Minimal Book</dc:title>
    <dc:creator>Nobody At All</dc:creator>
    <dc:identifier id="id">urn:uuid:0000</dc:identifier>
    <dc:language>en</dc:language>
  </metadata>
  <manifest>
    <item id="nav"   href="nav.xhtml"        media-type="application/xhtml+xml" properties="nav"/>
    <item id="two"   href="text/ch%202.xhtml" media-type="application/xhtml+xml"/>
    <item id="one"   href="text/ch1.xhtml"   media-type="application/xhtml+xml"/>
    <item id="css"   href="style.css"        media-type="text/css"/>
  </manifest>
  <spine>
    <itemref idref="nav" linear="no"/>
    <itemref idref="one"/>
    <itemref idref="two"/>
  </spine>
</package>
`},

	// linear="no": must not appear in the output at all.
	{name: "OEBPS/nav.xhtml", body: `<?xml version="1.0" encoding="utf-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>Contents</title></head>
<body><nav epub:type="toc"><ol><li>Chapter One</li><li>Chapter Two</li></ol></nav></body>
</html>
`},

	{name: "OEBPS/text/ch1.xhtml", body: `<?xml version="1.0" encoding="utf-8"?>
<html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops">
<head><title>One</title><style>p { margin: 0 }</style></head>
<body>
  <h1>Chapter One</h1>
  <p>The <em>first</em> paragraph, with an inline word and&nbsp;a
     hard-wrapped line inside the source.</p>
  <p>A second paragraph<a epub:type="noteref" href="#n1" id="r1">1</a> ending
     after a marker.</p>
  <p>A line<br/>and its continuation.</p>
  <script>var dropped = true;</script>
</body>
</html>
`},

	{name: "OEBPS/text/ch 2.xhtml", body: `<?xml version="1.0" encoding="utf-8"?>
<html xmlns="http://www.w3.org/1999/xhtml">
<head><title>Two</title></head>
<body>
  <h1>Chapter Two</h1>
  <div><p>The second chapter begins here.</p></div>
  <blockquote><p>A quoted line.</p></blockquote>
</body>
</html>
`},

	{name: "OEBPS/style.css", body: "p { margin: 0 }\n"},
}

func main() {
	f, err := os.Create("minimal.epub")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	z := zip.NewWriter(f)
	for _, e := range entries {
		method := zip.Deflate
		if e.store {
			method = zip.Store // the mimetype entry must not be compressed
		}
		w, err := z.CreateHeader(&zip.FileHeader{Name: e.name, Method: method})
		if err != nil {
			log.Fatal(err)
		}
		if _, err := w.Write([]byte(e.body)); err != nil {
			log.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		log.Fatal(err)
	}
}
