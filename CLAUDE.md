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
| `.btbook` | a book already extracted by bt, for carrying between devices | native |

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

**Down and Up move one display line, without typing it.** Added on request on
2026-09-09, after ten pages in one sitting: typing is what holds the attention,
but the hands tire long before the reading stops being wanted, and the
alternative to a way down the page is closing the book. Down counts the line
as read and keeps progress moving; Up goes back to the head of the current
line, or to the line above when already there, forgetting what was typed so
the line can be typed again.

This is not the first step towards free cursor movement, and Left, Right, Home
and the Page keys stay inert. A caret loose *inside* a line lets the saved
position drift away from what has actually been read; a line is a unit of the
page, and the position after the move is still the head of something unread.
It measures, blocks and scores nothing — see the table above, which is
unchanged. `cmd/bt/navigate.go` holds the movement and `pageWidth`, which the
renderer and the movement must agree on exactly or Down lands somewhere other
than the line the reader can see below the caret.

**Ctrl+G opens Place, which is how the position moves between machines.**
Added on request on 2026-09-16. The position is a rune offset into an immutable
file, so reading the number off one device and typing it into another *is* a
complete sync — no cable, no protocol, no clock, no conflict resolution. Place
shows where you are and takes a number; `bt pos [offset]` is the same thing
from the command line, for when the book is not open.

Typing a six-digit number by hand is the risk, and the screen answers it with a
**preview** rather than a check digit: as digits arrive it draws the offset
after snapping, the chapter, the percentage and the line of text there. A
dropped digit shows 4% instead of 23% before you press Enter, and a number
copied from a different book shows a sentence you do not recognise — a checksum
catches the first and not the second. Six characters of the text sha256 sit on
the screen next to the offset for the same reason: an offset only means
anything against one exact `text.txt`, and two devices showing different hashes
is the one failure nothing else would report.

A jump lands on the head of a word, never inside one, so the position after it
is still the head of something unread — which is why it does not reopen the
argument against free cursor movement above. It is also the only operation that
can lose your place, so it is the only one with an undo (Ctrl+Z).
`cmd/bt/place.go` holds the screen and `cmd/bt/SPEC.md` is the rule set, which
stays authoritative the way the epub and norm specs do.

---

## Moving a book to another device

Two things have to cross, and they are deliberately not the same mechanism,
because they happen at wildly different rates.

**The book crosses once, as a file.** `bt export <name>` writes one
`.btbook` — a zip of `bundle.json`, `meta.json`, `text.txt` and
`progress.json` — and you drag it over the cable in Explorer. `bt import` takes
it back. Once per book is rare enough that Windows copying a file is the whole
transport; there is no protocol, no adb and no pairing, and none was built.

With no destination the file lands in **`bt-books/`**, beside `storage/` and
not inside it, whatever directory `bt` was run from — an export is looked for
again later, and one known place beats a trail of `.btbook` files across the
disk. It is outside `storage/` because the point of a bundle is to be found in
Explorer and dragged across; `bt export <name> <path>` still overrides it.

`source.epub` is **not** in the bundle. The laptop is the archive; the far
device gets the derived text only, so extraction still happens exactly once
(decision 1) and the far device never needs an extractor at all.

Importing a bundle does **not** re-run the normalizer and does not re-derive
chapters or word counts. It copies `text.txt` byte for byte and verifies it
against the sha256 the exporter recorded, refusing the import outright on a
mismatch. This is the invariant everything else hangs off: a reading position
is a rune offset into those exact bytes, so re-deriving the text on the far
side is what would turn "resume" back into a search problem — and worse, would
do it silently.

**The position crosses every time, as a number.** That is the Place screen
above. Both devices print six characters of the text hash next to the offset,
and if those disagree the number is not transferable.

An automated sync over the cable (adb port-forward, a handful of JSON routes,
a base-offset conflict rule) was designed and then **not built**, because
typing one number in is not annoying enough to justify it. If it ever is, the
`.btbook` format is unaffected — it would automate the number, not the book.

---

## The phone app (`android/`)

Built on request on 2026-09-16. Kotlin, `minSdk 26`, and **no AndroidX and no
Compose**: a plain `Activity` and a custom `View` drawing on a `Canvas`. The
page is a monospace grid, a caret and a status line, so a UI framework would be
most of the APK and would buy nothing — the same argument this file already
makes for rejecting bubbletea. The release APK is about 640 KB and the only
dependency is the Kotlin stdlib.

**The phone never imports from source and never normalizes anything.** It takes
a `.btbook`, copies the text and verifies the hash. That is what keeps this app
free of an EPUB parser and a character table, and it is why extraction still
happens exactly once (decision 1).

It also **refuses a text that is not plain ASCII**, which the Go side does not
need to: offsets there are rune offsets, while a Kotlin `String` is indexed by
`Char`. The two are the same number only because normalization guarantees ASCII,
so that guarantee is checked on import rather than assumed. Without the check, a
single non-ASCII character would silently shift every offset after it, and the
error would grow the further you read.

### The conformance golden — IMPORTANT

There are now **two implementations of the wrapper**, and they have to agree
about what an offset means, or a position carried between the devices lands
somewhere else and the reader has no way to tell that is what happened.

`testdata/conformance/` holds a fixture and a golden of the exact lines the
wrapper produces at four widths, plus what `Window` returns at eleven offsets —
`Window` separately, because an agreeing full layout and a disagreeing window
would still send Down to the wrong place. **Both sides verify against that
golden and neither is the reference for the other**, so a drift in either is
caught rather than propagated.

After a deliberate change to the wrapping:

```bash
LEISURE_GOLDEN=1 go test ./internal/wrap/     # regenerate
./restart-android.sh                          # fails until the port agrees
```

What is on screen there: the same page, the same palette, and the same Place
screen with the same preview and the same six characters of hash. `Down` is a
button as well as a key, because on a phone the hands-tired case is most of the
time. `Left`, `Right`, `Home` and the page keys are inert, as on the laptop.

The IME is set to `TYPE_TEXT_VARIATION_VISIBLE_PASSWORD` plus
`TYPE_TEXT_FLAG_NO_SUGGESTIONS`, and the input connection handles both commit
and compose, applying only the difference from the previous composing text.
Autocorrect would otherwise type words that are not in the book, and
`NO_SUGGESTIONS` alone is advisory and widely ignored.

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
`qt.cmd`. `./restart.sh` writes the shim if it is missing.

**`go test` passing is not evidence that the change is in the binary.** Rebuild
with `./restart.sh` (`restart` from cmd), which is the inner loop:

```bash
./restart.sh              rebuild and run
./restart.sh --test       vet and test first
./restart.sh --no-launch  rebuild only
./restart.sh --force      kill a running copy instead of refusing
```

It exists because `go build -o bin/bt.exe` does not fail while a copy of bt is
running — Windows locks the exe, so Go renames the old one to `bin/bt.exe~`,
writes the new one, and the copy on screen goes on running the old code. A
change with no effect and a green suite is what that looks like from the
outside, and it has already happened once.

Unlike quick-tools' script, this one **refuses** rather than killing a running
copy, and the difference is the point: Esc quits bt, saves the position,
restores the console mode and drops the alternate buffer. `taskkill` skips all
four, and leaves the terminal it was reading in with no echo (gotchas 4 and 7).
`--force` is for a copy whose terminal is already lost.

The phone app builds with `./restart-android.sh`, which finds the JDK, the SDK
and a cached Gradle distribution itself:

```bash
./restart-android.sh              test, build release, copy to bin/bt-<version>.apk
./restart-android.sh --no-test    skip the unit tests
./restart-android.sh --debug      the debug APK
./restart-android.sh --install    also adb install it, if a device is attached
```

There is no Gradle wrapper jar in the repo, deliberately — a checked-in binary
blob is worth avoiding when a cached distribution is already on the machine.

```
bt                    open the current book where it was left
bt import <file>      import a book and make it current
bt list               list imported books, with progress
bt use <name>         switch the current book
bt pos [offset]       show the reading position, or go to one
bt export <name>      write one .btbook file into bt-books/, to carry elsewhere
```

---

## Layout

```
cmd/bt/            the binary; flags, subcommands, the run loop, Up/Down, Place
internal/epub/     EPUB -> text          container, spine, XHTML
internal/norm/     Unicode -> ASCII      the character table
internal/book/     import pipeline, meta.json, the on-disk store
internal/progress/ progress.json: read, write, hash check
internal/tui/      console mode, alt screen, frame buffer, input decode
internal/wrap/     greedy word wrap into display lines
internal/convert/  finding and running pdftotext / ebook-convert
internal/book/     ... including bundle.go, the .btbook export/import
storage/books/     imported books (gitignored)
bt-books/          exported .btbook bundles, where bt export writes (gitignored)
bin/               bt.exe and the APK restart-android.sh copies out (gitignored)
testdata/          fixture books, including a hand-built minimal .epub
testdata/conformance/  the fixture and golden both wrappers verify against
android/           the phone app: Kotlin, no AndroidX, no Compose
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
