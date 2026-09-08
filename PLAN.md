# leisure-typing — plan

Agreed 2026-09-08. `CLAUDE.md` holds the standing rules; this file holds the
order of work and what "done" means for each step.

Estimate: **about two days** of work for M0–M4, plus whatever the two reserved
packages take the user.

---

## Status — 2026-09-08

M1–M4 are built and the whole app runs end to end, on `.txt` and on `.epub`.
`go test ./...` is green and now covers everything: `LEISURE_TODO` gates
nothing any more, because there is no longer a red loop to gate.

**Both reserved packages were handed over on request**, after the rest was
built, and both are written. `internal/norm/table.go` is the character table
and `norm.go` the passes over it; `internal/epub/epub.go` is the container,
OPF, spine and XHTML walk. Their two `SPEC.md` files stay authoritative.

| | State |
|---|---|
| M0 console spike | **folded into `internal/tui`** — see below |
| M1 import and storage | done |
| M2 the reader | done, pending the twenty-minute sit-down |
| M3 EPUB | done — `epub` and `norm` both written, tests in the default suite |
| M4 converters and the shim | done; `bt.cmd` is installed |
| M5 polish | chapter offsets and the error log landed early, being cheap |

**M0 was never written as `cmd/spike`.** The ground it was meant to prove —
VT on both handles, raw mode, the alternate buffer, one write per frame,
decoded keystrokes, restore on every exit path including panic, and the resize
poll — is all in `internal/tui`, which M2 exercises directly. Nothing was
skipped; there was just no throwaway to delete afterwards.

What that leaves is the one thing a test cannot check: **the three-terminal
run**. Windows Terminal, the old conhost, and the VS Code integrated terminal
all have to look identical, and killing `bt` mid-run has to leave a working
shell behind. That needs hands on a keyboard.

Two things also want feeling out rather than testing: whether three lines of
context is the right amount, and whether a wrong character advancing the caret
(open question 2) reads well over a long sitting.

### What was decided by building it

- **Open question 1 — context.** Three lines each way to begin with, then
  **changed on request on 2026-09-08 to fill the terminal**: as many context
  lines as there are rows for, and the text wrapped to the full width rather
  than to a 68-column measure centred in the screen.

  Both are still one constant each in `cmd/bt/read.go`, and zero now means "as
  much as the terminal has": `contextLines = 3` and `wrapWidth = 68` put the
  old layout back. CLAUDE.md's "Layout on screen" keeps the argument for the
  measure — past about 70 characters the eye loses its place returning to the
  start of the next line — so that going back is a decision with its reasons
  written down rather than a guess.

  One column of the width is reserved and always will be: the active line
  draws a cell past its text for the caret to sit in, and without it a
  full-width line wraps and pushes the frame down a row.
- **Open question 2 — the caret on a wrong character.** It advances, and the
  cell shows the character that *should* have been typed, in red. The wrongly
  typed one is never inserted, so the line stays aligned and the wrapping
  stays stable.
- **Open question 3 — blank lines.** Newlines are stepped over, never typed.
  Enter and Tab do nothing rather than counting as wrong: they are not
  mistakes in the text. Whether a chapter-number line should be skipped too is
  still open, and wants real use.
- **Open question 4 — the source file.** Kept, as `source.<ext>`.
- **Open question 5 — front matter.** Now answered in code, as §8 of
  `internal/norm/SPEC.md`: trim to the first chapter heading that has 400
  characters of prose behind it, which walks past a contents page without
  eating real chapters; no trim at all when there is no heading to find. It is
  part of `Normalize` rather than a separate step, because it needs the
  paragraph structure that `Normalize` has just built.

  Both halves of that rule were wrong first time, in the same direction —
  cutting too much. A fixture in `internal/book` caught it: two real chapters a
  few lines apart look exactly like two contents entries, and the original
  "last heading in the window" rule threw chapter one away. The spec records
  what changed and why; `TestFrontMatterDoesNotEatCloseChapters` holds it
  down.

### Not yet true

- **No PDF or MOBI has been imported end to end**, because neither converter
  is installed on this machine. The dispatch, the error paths and the messages
  are built and tested; the conversions themselves are unrun. An EPUB now
  imports for real — `internal/epub/testdata/minimal.epub` goes in and comes
  out as text with its spine order, its percent-encoded href and its
  `linear="no"` document all handled.
- **`<br/>` does not survive the import.** `epub` emits it as a line break, as
  its spec says verse needs; `norm` then joins single newlines into spaces, as
  its spec says a hard-wrapped `.txt` needs. Both rules are right alone and
  they disagree here. Prose is unaffected and verse comes out as one long
  line. The note under epub/SPEC.md §2 says where the fix belongs if a book of
  poetry ever makes the case; it is deliberately not fixed yet. The dispatch, the error paths and the messages are built and
  tested; the conversions themselves are unrun.
- `bt` has not been sat in for twenty minutes. M2's done-when is unmet until
  it has been.

---

## M0 — the console spike

The one genuinely risky part, so it goes first and gets proved in isolation
before anything is built on it. A throwaway `cmd/spike` that:

- enables VT on both handles, enters raw mode, enters the alternate buffer
- draws a frame of coloured text in **one** `Write`
- echoes decoded keystrokes, including Backspace, Esc, Ctrl+C and arrows
- restores everything on exit and on a forced panic
- polls the console size and redraws when the terminal is resized

**Done when** it behaves identically in all three: Windows Terminal, the old
conhost, and the VS Code integrated terminal — and when killing it mid-run
leaves a working shell behind.

Delete it once M2 has exercised the same ground, exactly as quick-tools deleted
`cmd/m0`. Note the deletion here when it happens.

---

## M1 — import and storage, `.txt` only

The whole pipeline end to end with the interesting parts stubbed, so that the
shape is proved before either reserved package exists.

- `bt import <file.txt>` → `storage/books/<id>/` with `source`, `text.txt`,
  `meta.json`, `progress.json`
- `bt list`, `bt use <name>`
- id derived from the filename, slugified; collisions get a suffix
- `text.txt` written through a temp file and renamed
- `meta.json` carries the sha256 of `text.txt`; a mismatch on load is an error
  offering re-import, never a silent resume
- `internal/norm.Normalize` is called here, and at this point it is the stub —
  a `.txt` book that is already ASCII imports correctly regardless

**Done when** a plain-text book round-trips and `bt list` shows `0%`.

---

## M2 — the reader

The part the user actually sits in.

- `internal/wrap` — greedy word wrap to a target column count, returning
  display lines that each carry their start offset in `text.txt`. Pure, and
  tested without a terminal.
- `internal/tui` — the frame buffer, the colour vocabulary, the input decode
- the run loop: render, read a key, advance or mark wrong, render again
- Backspace steps back one character and clears its state
- **no blocking on a wrong character** — mark it red, advance
- Esc quits; Ctrl+C quits
- progress saved on a ~2 s debounce and unconditionally on exit
- resize re-renders from the stored offset

**Done when** a `.txt` novel can be typed through for twenty minutes, quit,
reopened, and it resumes on the same character.

---

## M3 — EPUB

- `internal/epub.Extract` ships as a **stub** with the contract, the spec and
  fixtures, behind `LEISURE_TODO=1`. **Reserved for the user.**
- `internal/norm.Normalize` likewise. **Reserved for the user.**
- everything around them — dispatch on extension, the import path, the
  fixtures, a hand-built minimal `.epub` in `testdata/` — is finished
- `scripts/` or a doc note recording the character table the normalizer is
  expected to cover, as the spec rather than as an implementation

**Done when** `LEISURE_TODO=1 go test ./internal/epub/ ./internal/norm/ -v` is
red for the right reasons and `go test ./...` is green.

---

## M4 — external converters, and the shim

- `internal/convert` finds `pdftotext` and `ebook-convert` on PATH, overridable
  in config
- `bt import book.pdf` runs one of them into a temp file, then feeds the normal
  `.txt` path
- neither present → a clear message naming both and recommending EPUB, not a
  half-working extraction
- `bt.cmd` shim in `%LOCALAPPDATA%\Microsoft\WindowsApps`

**Done when** a PDF and a MOBI both import on a machine with Calibre, and fail
informatively on one without.

---

## M5 — polish

Only after the above is being used daily, and only what the use actually asks
for:

- chapter offsets in `meta.json`, and a way to jump to one
- a `bt` with no current book saying something useful
- an error log file, since nothing may print to the terminal (see conventions)
- config for wrap width and the number of context lines

---

## Decisions already made — do not re-litigate

- **No WPM, timer, score or results screen.** See CLAUDE.md; this is a reader,
  not a typing test.
- **No blocking on a wrong character.** Red and move on.
- **Terminal, not a GUI.** It must run in the VS Code terminal, and the
  alternate screen buffer leaving no scrollback is a feature of the product.
- **Not part of quick-tools.** That is a resident process opened for three
  seconds at a time; this is sat in for half an hour and would drag zip, XML and
  converter code into a 7.5 MB always-on binary. Separate repo, separate binary.
- **No TUI framework.** Static full-screen layout, one keystroke at a time.
- **No in-process PDF parsing.** Shell out or refuse.
- **Normalization goes all the way to ASCII**, and the one-rune-one-cell
  property that buys is why.

---

## Open questions

1. **How much context above and below?** Three lines each way is the starting
   guess. It is a config value, not a decision to agonise over now.
2. **What does a wrong character do to the caret?** Two schools: advance
   regardless (the text stays aligned, the red character sits where the right
   one should be), or insert the wrong character and let the line drift. The
   first is chosen for M2 because it keeps wrapping stable, but it is worth
   feeling out in use.
3. **Punctuation-only lines and blank lines between paragraphs** — typed, or
   skipped? Skipping blank lines is almost certainly right; skipping a line
   that is only a chapter number probably is too. Decide from real use.
4. **Whether to keep `source.epub` after import.** Costs a few hundred KB per
   book and makes re-import possible without hunting for the file again. Kept
   for now.
5. **Front matter.** Title pages, copyright pages and tables of contents are
   part of the extracted text and nobody wants to type an ISBN. Whether this is
   the normalizer's job or a separate trim step is the user's call, since it
   falls inside the reserved work.
