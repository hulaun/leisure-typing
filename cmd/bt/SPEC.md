# cmd/bt — moving the reading position

This is the rule set for every way the position moves other than typing. It is
written down because the reading position is the one piece of state the app
cannot afford to get wrong, and because §2 is the question a future reader will
ask first.

Change the spec and the tests together when the behaviour should change; do not
let them drift apart.

---

## 1. What the position is

A rune offset into `text.txt`, and nothing else. `text.txt` is immutable and
its sha256 is recorded in `meta.json`, so the offset is valid for the life of
the import and means the same thing on any machine holding the same bytes.

Every move below produces an offset, and every offset is saved through
`progress.Save` — a temp file and a rename, on a debounce and on every exit
path.

## 2. Why a jump is allowed when Left and Right are not

`navigate.go` argues that a caret loose *inside* a line lets the saved position
drift away from what has actually been read, which is why Left, Right, Home and
the Page keys do nothing and always will. Up and Down are the exception,
because a line is a unit of the page.

A jump does not contradict that, for three reasons:

1. **It lands on the head of a word** (§4). The position after a jump is the
   head of something unread, which is the same guarantee Up and Down keep.
2. **It is explicit.** It is a number typed into a field and confirmed against
   a preview, not a caret sliding sideways under a key you were holding down.
   Drift is the accumulation of small unnoticed moves; this is one large
   deliberate one.
3. **It is the only way the position can leave one machine and arrive at
   another.** That is the whole purpose; see §3.

What a jump still is not: it does not measure, block or score anything, and
there is no end screen. The table at the top of `CLAUDE.md` is unchanged.

## 3. Place is a sync, done by hand

The reason the screen exists at all. The position is one integer, so reading it
off one device and typing it into another is a complete transfer of where you
are — with no cable, no protocol, no clock and no conflict resolution.

For that to be true, three things must be on the screen, and they are not
decoration:

- **The offset**, grouped (`184,223`), because that is the number you copy.
- **Six characters of the text hash**, because an offset only means anything
  against one exact `text.txt`. If two devices show different hashes the number
  is not transferable, and nothing else would tell you — the position would
  simply resume in the wrong chapter. This is the same check `store.Text` and
  `progress.Load` make against the full hash.
- **A preview** of where the typed number lands: the offset after snapping, the
  chapter, the percentage, and the line of text there.

The preview is the answer to typing a six-digit number by hand, and it is
deliberately preferred to a check digit or an encoded "place code". A dropped
digit sends you to 4% instead of 23% and you see that before pressing Enter; a
number copied from a different book shows a sentence you do not recognise. A
checksum catches the first and not the second.

## 4. Snapping

An arbitrary number lands mid-word most of the time, and beginning from the
fourth letter of something is neither readable nor typable. So:

- Inside a word, the offset moves **back** to the head of that word.
- In the whitespace between words — including a paragraph break, which is
  several runes — it moves **forward** to the start of the next word.
- Below zero it clamps to `0`; past the end it clamps to `len(text)`.

Either way the result is the head of something not yet read.

Snapping applies to numbers that came from outside the reading. It must **not**
apply to a position that was reached by reading, which is why undo restores
through `moveTo` rather than `jumpTo`: a position reached by typing may
legitimately sit on a space, and re-snapping it would land the undo somewhere
other than where the jump started.

## 5. Marking

A jump marks everything before the new position `correct` and everything after
it `untyped` — exactly what `read()` does when resuming a saved position.

Nothing was got wrong on either side of a jump. Marking it any other way paints
a screenful of red on the first backspace across the seam, for characters that
were never typed.

## 6. Undo

Jumping is the only operation in the app that can lose your place, so it is the
only one with an undo. Ctrl+Z on the Place screen returns to where the last
jump started.

`moveTo` records its origin whichever direction it runs, so a second Ctrl+Z
returns to the jump. That is a redo, and it is a consequence of the rule rather
than a feature designed separately.

## 7. The screen owns every key while it is open

Nothing typed into the field may also reach the book. A digit that leaked
through would be typed into the text and counted wrong, because the book almost
certainly does not say `4` at that point.

Esc closes the screen rather than quitting the app. **Ctrl+C still quits from
the Place screen**, as it does from everywhere: raw mode means the app owns
Ctrl+C, and there must be no state the app can be in with no way out
(`CLAUDE.md`, gotcha 3).

Left, Right, Home and the Page keys stay inert here too.

## 8. The frame

The Place screen replaces the page rather than overlaying it — the page is the
whole terminal, and a box drawn on top would have to solve what is underneath
at every width.

It obeys the same two frame rules as the page, for the same reasons:

- Exactly as many rows as the terminal has, with the **last row written
  unterminated**. One row too many scrolls the terminal, which tears through
  ConPTY in a way that looks exactly like a rendering bug.
- No row wider than the terminal.

The key hint owns the bottom row, in the position the status line occupies on
the page.

## 9. `bt pos`

The same thing without the reader: `bt pos` prints the position, the chapter,
the percentage and the hash; `bt pos <offset>` snaps and saves one. It exists so
the number can be read off or put in without opening the book, and it goes
through the same `snapToWord` as the screen.

It prints to stdout, which is allowed only because the alternate buffer is not
up: nothing may print to the terminal while it is (`CLAUDE.md`, conventions).
