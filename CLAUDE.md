# leisure-typing

A terminal application for reading books by typing them out.

Run `bt`, the terminal fills with a page of the book you are part-way through,
and you start typing. It remembers where you stopped. Quit and the terminal is
exactly as it was, with nothing left in the scrollback.

---

## Why this exists

The user wants to read a full book, and has leisure time at a desk where
sitting with a novel would be conspicuous but sitting in a terminal is not.
Typing the text is what keeps the attention on it — reading passively at a
desk drifts, typing does not.

**This is not a typing test.** It is a reader whose page-turn happens to be the
keyboard. Every decision follows from that, and the two are constantly confused,
so the distinction is written down here rather than rediscovered:

| A typing test | This |
|---|---|
| Measures WPM and accuracy | **Measures nothing.** No timer, no score, no stats |
| Blocks on a wrong character until it is fixed | Marks it red and moves on |
| Random words or a short fixed passage | One long book, resumed across weeks |
| Session ends, result screen | There is no end; you stop when you stop |

**Do not add WPM, a timer, a results screen, or error-blocking.** They were
considered and rejected on 2026-09-08. Blocking on a typo is the one that looks
most like an improvement and is the most damaging: it yanks the eye out of the
sentence to fix a character nobody will read again, which is the exact opposite
of the point.

The one number that matters is **progress through the book**, because that is
the whole reward loop. A 200-page novel is around 40,000 words, which is
something like 17 hours of typing — five weeks at half an hour a day. Nobody
finishes that without being able to see it moving.

---

## The central design decisions

**1. Extraction happens once, at import.** The book file is never read while
typing. Import produces a normalized, immutable `text.txt`, and the reading
position is a rune offset into that file. Opening a book is one `os.ReadFile`
and a slice — the 230 KB of a novel costs under a millisecond, and the source
format stops mattering the moment import is done. Do not re-derive text from
the source at runtime; that is what turns "resume" from an integer into a
search problem.

**2. Normalization is to plain ASCII, and it is load-bearing twice over.**
A book is full of characters that are not on the keyboard — curly quotes, em
dashes, ellipsis characters, non-breaking and thin spaces, soft hyphens inside
words, `fi`/`fl` ligatures. Typing stalls dead on a character that cannot be
produced, and the invisible ones (soft hyphen, NBSP, zero-width joiner) are
maddening because the screen shows nothing wrong.

The second payoff is in the terminal: ASCII means one rune is one cell, always,
so the renderer never meets the East-Asian-width or combining-character problem
that makes general TUI text layout genuinely hard. **Do not relax the
normalization to "keep the nice typography".** It would cost a rewrite of the
wrapper.

**3. One `Write` per frame.** The whole frame is built into a buffer and
emitted in a single write to stdout. VS Code's terminal goes through ConPTY,
which is slow enough that a frame emitted as many small writes tears visibly.
At 30x100 a full repaint is about 3 KB, so there is no need for dirty-region
tracking — repaint everything, every keystroke, in one call.

**4. The alternate screen buffer.** `\x1b[?1049h` on start and `\x1b[?1049l`
on quit. The app owns the whole terminal while it runs and leaves no trace in
the scrollback afterwards. This is a feature of the product, not just hygiene.

**5. Wrapping is a view, never stored.** The stored position is an offset into
`text.txt`. Line breaks are computed at render time from the current terminal
width. A resize therefore re-renders and invalidates nothing, and the wrap
width can be changed later without breaking every saved position.

---

## Division of work — IMPORTANT

**Nothing is reserved any more.** Both of the packages this section held back
for the user were handed over on request on 2026-09-08, after the rest was
built, and both are written:

1. **`internal/epub.Extract`** — the container/OPF/spine walk and the XHTML to
   text reduction. `SPEC.md` in that directory is the rule set;
   `testdata/minimal.epub` and its generator are the fixture, built so the
   manifest order disagrees with the spine and one href is percent-encoded.

2. **`internal/norm.Normalize`** — the character table. `table.go` is the table
   itself, grouped by what a run of characters becomes; `norm.go` is the passes
   over it.

Both packages' tests now run in the default suite. `LEISURE_TODO` no longer
gates anything, and `go test ./...` covers everything there is.

The two `SPEC.md` files stay, and stay authoritative. They are the statement of
what these packages must do, written before the code and revised where the
code proved them wrong — see norm §8 on the table-of-contents case, and the
note under epub §2 on `<br/>` and verse. Change the spec and the tests when the
behaviour should change; do not quietly let them drift apart.

The instinct that put them here in the first place still generalises, as it
does in quick-tools: when a task has a genuinely interesting algorithmic core —
parser, scheduler, diff, solver — ask before implementing it. The user asked
for these two explicitly; that is what changed, not the default.

## Format support

| Format | How | State |
|---|---|---|
| `.txt` | read it | native |
| `.epub` | ZIP of XHTML, walk the spine | native |
| `.pdf` | shell out to `pdftotext -layout` (poppler) | external |
| `.mobi`, `.azw3`, `.fb2` | shell out to `ebook-convert` (Calibre) | external |

**EPUB is the format to prefer and to tell the user to prefer.**

**PDF is not to be parsed in-process.** A PDF has no text, only glyphs at
coordinates, and extracting from it means solving reading order across columns,
repeated headers and footers, hyphens broken across lines, ligatures, and fonts
with absent or broken `ToUnicode` maps. It is weeks of work and still wrong
sometimes. Detect the converters on PATH the way quick-tools' Places tab detects
openers; if neither is present, say so and tell the user to convert to EPUB
rather than doing a bad job quietly.

---

## On-disk layout

```
storage/books/<id>/
  source.epub      the original, kept
  text.txt         normalized, immutable, the only thing read at runtime
  meta.json        title, author, sha256 of text.txt, chapter offsets
  progress.json    { offset, updated }
```

The `sha256` exists so that changing the extractor is detected rather than
silently resuming at an offset that now points somewhere else. On mismatch,
say so and offer to re-import; never resume against a hash that does not match.

`storage/` is gitignored as one unit, as in quick-tools. Books are the user's
own files and none of them belongs in the repo.

---

## Layout on screen

Context above and below the active line, so it reads as a book rather than as a
test. **The page is the whole terminal**, in both directions: text wraps to the
full width, and there are as many context lines as there are rows for.

```
lines already typed, dimmed, filling the screen upwards, so the eye has
somewhere to return to when it loses its place on the page.

the active line, typed portion in normal weight, a caret, and then the rest of
it bright with wrong characters in red

lines still to come, dimmed the same as the ones above so that the active line
is the only bright thing on the screen, running down as far as the rule.

--------------------------------------------------------------------------
Chapter 4                                                 23%   9,240 words
```

This was **changed on request on 2026-09-08**. It was built the other way, and
the argument for the old way is worth keeping, because it is the reason to go
back if the full width turns out to read badly:

> Roughly 68 columns, centred in whatever width the terminal is. Past about 70
> characters the eye starts losing its place coming back to the start of the
> next line, which is why books, newspapers and this document are all set to a
> measure rather than to the width of the paper.

Both are one constant each in `cmd/bt/read.go`: `wrapWidth = 68` restores the
measure, `contextLines = 3` restores the band of context. Zero means "as much
as the terminal has".

One column of the width is reserved and always will be. The active line draws
a cell *past* its text, for the caret to sit in when the next thing to type is
the whitespace a line break consumed; without that column a full-width line
runs one past the edge, wraps, and pushes the whole frame down a row.

Colours: dim for context, normal for typed-correct, red for typed-wrong, bright
for untyped-on-the-active-line, and a caret. That is the entire palette and it
is the only correctness signal in the app.

---

## Gotchas (Windows console)

These are known before the first line is written — they come from the design
discussion on 2026-09-08 and from quick-tools' Win32 experience. Add to this
list as things bite.

**1. VT must be enabled explicitly, on both handles.** The output handle needs
`ENABLE_VIRTUAL_TERMINAL_PROCESSING` or every escape sequence prints as literal
garbage. The input handle needs `ENABLE_VIRTUAL_TERMINAL_INPUT`, which makes
keys arrive on stdin as VT byte sequences.

**2. Read keys from stdin as VT, not with `ReadConsoleInputW`.** The VT path
behaves identically under ConPTY (VS Code's terminal) and under conhost. The
console-input API is the one that differs between them, and running in both is
a requirement, not a nice-to-have.

**3. Raw mode means the app owns Ctrl+C.** Clearing `ENABLE_PROCESSED_INPUT`
stops Windows generating an interrupt, so `0x03` simply arrives as a byte. Quit
is Esc, with Ctrl+C handled as a backup — miss this and there is no way out.

**4. Restore the console mode on every exit path, including panic.**
`defer term.Restore(...)` and a `recover` at the top of main. Leaving the
console in raw mode gives the user a shell with no echo, which looks like the
shell is broken rather than like this app crashed — and it will happen at work.

**5. There is no `SIGWINCH` on Windows.** Poll `GetConsoleScreenBufferInfo` on
a ~100 ms ticker and re-render when the width changes. Cheap, because wrapping
is a view (see design decision 5) and nothing is invalidated by a resize.

**6. Many small writes tear through ConPTY.** See design decision 3. One
buffer, one write, every frame.

**7. The alternate screen buffer must be left on every exit path too**, and in
the same `defer` as the mode restore. An app that dies holding the alternate
buffer leaves the terminal showing a frozen half-drawn page.

---

## Build and test

Go lives at `C:\Program Files\Go`. In Git Bash:

```bash
export PATH="$PATH:/c/Program Files/Go/bin"

go test ./...                  # default suite, stays green
go vet ./...
go build -o bin/bt.exe ./cmd/bt
```

The binary is `bt`. A shim in `%LOCALAPPDATA%\Microsoft\WindowsApps` (on the
Windows PATH by default) makes `bt` work from cmd, PowerShell, Git Bash and the
VS Code terminal, from any directory — the same arrangement as quick-tools'
`qt.cmd`.

```
bt                    open the current book where it was left
bt import <file>      import a book and make it current
bt list               list imported books, with progress
bt use <name>         switch the current book
```

---

## Layout

```
cmd/bt/            the binary; flags, subcommands, the run loop
internal/epub/     EPUB -> text          container, spine, XHTML
internal/norm/     Unicode -> ASCII      the character table
internal/book/     import pipeline, meta.json, the on-disk store
internal/progress/ progress.json: read, write, hash check
internal/tui/      console mode, alt screen, frame buffer, input decode
internal/wrap/     greedy word wrap into display lines
internal/convert/  finding and running pdftotext / ebook-convert
storage/books/     imported books (gitignored)
testdata/          fixture books, including a hand-built minimal .epub
```

### Dependencies

| Need | Package | Why |
|---|---|---|
| Raw console mode | `golang.org/x/term` | gets the Windows console modes right; small and maintained |

Nothing else. Deliberately avoided: **bubbletea** and every other TUI framework
— this screen is a static full-screen layout driven by one keystroke at a time,
and the framework would be most of the binary. **Any Go PDF library**, for the
reason under Format support.

---

## Conventions

- **Colour and layout live in one place each**, as in quick-tools: a constants
  block for geometry and a vars block for the ANSI colours. Nothing else
  hardcodes a width or a colour.
- **Nothing writes to stdout except the frame renderer.** A stray `fmt.Println`
  in raw mode with the alternate buffer active corrupts the display in a way
  that looks like a rendering bug. Errors go to a log file, not to the terminal.
- **The reading position is saved on a debounce, and on every exit path.**
  Losing ten minutes of a five-week book is the worst bug this app can have.
  Write to a temp file and rename, so a crash mid-write cannot leave a truncated
  `progress.json`.
- Windows-only files carry `//go:build windows`, though nothing here needs to be
  Windows-only by nature; the console setup is the only part that is.
