# `epub.Extract` — the spec

`.epub` in, the book's text in reading order out. The contract is on
`Extract`'s doc comment; this file is how to get there.

```bash
go test ./internal/epub/ -v
```

The fixture `testdata/minimal.epub` is hand-built by `testdata/make_epub.go`
and exercises every rule below. It is small enough to read in full.

---

## 1. The container walk

An EPUB is a ZIP. Four steps, each of which can fail and must fail loudly:

1. **`mimetype`** — the first entry, stored uncompressed, containing exactly
   `application/epub+zip`. Neither the ordering nor the compression is checked:
   plenty of real books in the wild get them wrong and their content is still
   fine. A *wrong* mimetype is refused; a **missing** one is tolerated, because
   `container.xml` is the real test of whether this is an EPUB and refusing on
   the weaker signal only loses readable books.

2. **`META-INF/container.xml`** — points at the package document:

   ```xml
   <container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
     <rootfiles>
       <rootfile full-path="OEBPS/content.opf" media-type="application/oebps-package+xml"/>
     </rootfiles>
   </container>
   ```

   Take the first `<rootfile>`. Its `full-path` is relative to the ZIP root.

3. **The OPF** — the package document. Three parts matter:

   - `<metadata>`: `<dc:title>` and `<dc:creator>` for `Book.Title` and
     `Book.Author`. Both optional.
   - `<manifest>`: `<item id="..." href="..." media-type="..."/>` — the id to
     href map. **`href` is relative to the OPF's own directory**, not to the
     ZIP root. This is the single most common way a first implementation goes
     wrong: `OEBPS/content.opf` with `href="text/ch1.xhtml"` means the ZIP
     entry `OEBPS/text/ch1.xhtml`.
   - `<spine>`: `<itemref idref="..."/>` in reading order. This is the order,
     not the manifest order and not the ZIP order.

4. **The spine documents** — resolve each `idref` through the manifest, read
   the ZIP entry, reduce it to text (below), and join with a blank line.

`linear="no"` on an `<itemref>` marks material outside the main reading flow.
Skip those.

Note on ZIP paths: entry names are stored with `/` separators and may be
percent-encoded in the OPF (`href="text/ch%201.xhtml"`). Decode before looking
the entry up, and match case-sensitively — `archive/zip` does.

## 2. XHTML to text

The input is XHTML, so `encoding/xml` parses it — but real books contain
undeclared HTML entities (`&nbsp;`, `&mdash;`). Set `d.Strict = false` and
`d.AutoClose = xml.HTMLAutoClose`, and give `d.Entity` the HTML entity table
you need, or the decoder stops on the first `&nbsp;` in the book.

Walking the token stream:

**Drop entirely, including their character data:** `script`, `style`, `head`,
`title`, `meta`, `link`, `svg`, `img`, `figure`, `figcaption`, `table`. Tables
in novels are almost always layout artefacts and typing one is miserable.

**Block elements — a paragraph break before and after:** `p`, `div`, `h1`–`h6`,
`blockquote`, `li`, `pre`, `section`, `article`, `hr`.

**Inline — no break, no space added:** `em`, `i`, `strong`, `b`, `span`, `a`,
`small`, `u`, `q`, `cite`, `abbr`, `code`.

**`br`** — a single newline, not a paragraph break. Verse depends on it.

> **Open, and it needs a decision from real use.** `norm.Normalize` then treats
> a single newline inside a paragraph as the hard wrapping of a plain-text book
> and joins it into a space (norm/SPEC.md §4) — so the line break a `<br/>`
> preserves here does not survive the import. Prose is unaffected; verse comes
> out as one long line.
>
> The two rules are each right alone and they disagree on this one case. There
> is no fixing it in `norm`: in a `.txt` there is no telling a verse break from
> a wrap artefact, and joining is what makes a stored book re-wrappable at all.
> If it matters, the fix belongs here — emit a paragraph break for `<br/>`
> inside a verse context, since this is the layer that actually knows a `<br/>`
> was written. Left alone until a book of poetry makes the case.

**Footnote links** — an `<a>` whose `epub:type` is `noteref`, or whose class
contains `footnote` or `noteref`, is dropped with its text. The marker is not
part of the sentence.

Character data outside a dropped element is kept as-is. Collapse runs of
whitespace within a run of text to a single space; leading and trailing
whitespace on a paragraph goes.

## 3. Joining

Between spine documents, one blank line — a chapter boundary is a paragraph
boundary. Runs of blank lines collapse to one. The result has no leading or
trailing blank line and ends with a single `\n`.

Extract does **not** normalize. It returns Unicode; `norm.Normalize` is a
separate pass, run by the import pipeline, and keeping them apart means the
character table can be changed without re-reading any EPUB.
