# `norm.Normalize` — the spec

Unicode book text in, typeable ASCII out. The contract is on `Normalize`'s doc
comment; this file is the character table behind it.

The tests in `norm_test.go` are the executable form of this document, and
`table.go` is §1-6 in code. Both run in the default suite:

```bash
go test ./internal/norm/ -v
```

---

## 1. Quotation marks and apostrophes

| In | Out | Note |
|---|---|---|
| `U+2018 U+2019` `‘ ’` | `'` | single quotes, and the apostrophe in *don’t* |
| `U+201A U+201B` `‚ ‛` | `'` | low and reversed single |
| `U+201C U+201D` `“ ”` | `"` | double quotes |
| `U+201E U+201F` `„ ‟` | `"` | low and reversed double |
| `U+2039 U+203A` `‹ ›` | `'` | single angle quotes |
| `U+00AB U+00BB` `« »` | `"` | double angle quotes |
| `U+02BC` `ʼ` | `'` | modifier letter apostrophe |
| `U+0060 U+00B4` `` ` `` `´` | `'` | grave and acute used as quotes |

## 2. Dashes and hyphens

| In | Out | Note |
|---|---|---|
| `U+2010 U+2011` `‐ ‑` | `-` | hyphen, non-breaking hyphen |
| `U+2012 U+2013` `‒ –` | `-` | figure dash, en dash |
| `U+2014 U+2015` `— ―` | ` - ` | em dash, horizontal bar — spaced |
| `U+2212` `−` | `-` | minus sign |
| `U+00AD` `­` | *(removed)* | **soft hyphen**: delete, joining the word |

The em dash becomes a spaced hyphen rather than a bare one so that
`word—word` does not become the single unwrappable token `word-word`. Collapse
the result: `word — word`, never a double space.

## 3. Spaces

| In | Out |
|---|---|
| `U+00A0` no-break space | ` ` |
| `U+2000`–`U+200A` en/em/thin/hair quad spaces | ` ` |
| `U+202F` narrow no-break space | ` ` |
| `U+205F` medium mathematical space | ` ` |
| `U+3000` ideographic space | ` ` |
| `U+0009` tab | ` ` |
| `U+200B U+200C U+200D U+2060 U+FEFF` zero-width, ZWNJ, ZWJ, word joiner, BOM | *(removed)* |

Runs of two or more spaces collapse to one. Trailing spaces on a line are cut.

## 4. Line and paragraph structure

- `\r\n` and lone `\r` become `\n`.
- `U+2028` line separator and `U+2029` paragraph separator become `\n`.
- A run of two or more `\n` is a paragraph break: exactly one blank line.
- A single `\n` inside a paragraph is a hard-wrap artefact: it becomes a space
  and the lines join. A book stored at 72 columns must come out as one long
  line per paragraph, because wrapping is a view (CLAUDE.md, decision 5).
- A line ending in `-` followed by a lowercase letter on the next line is a
  word broken across the break: join with no space and drop the hyphen.
  `disap-\npointed` → `disappointed`. Do not join when the next line starts
  with a capital — `Anglo-\nSaxon` keeps its hyphen.
- No leading or trailing blank lines. The result ends in exactly one `\n`.

## 5. Ligatures and letters

| In | Out |
|---|---|
| `ﬀ ﬁ ﬂ ﬃ ﬄ` `U+FB00`–`U+FB04` | `ff fi fl ffi ffl` |
| `Æ æ Œ œ` | `AE ae OE oe` |
| `ß` | `ss` |
| `Ø ø` | `O o` |
| `Đ đ Ð ð Þ þ` | `D d D d Th th` |
| `Ł ł` | `L l` |

Every other accented Latin letter loses its accent: decompose to NFD and drop
the combining marks, so `é`→`e`, `ñ`→`n`, `ü`→`u`, `Å`→`A`. `golang.org/x/text`
is deliberately *not* a dependency; a table for Latin-1 and Latin Extended-A
covers essentially every novel in English.

## 6. Punctuation and symbols

| In | Out |
|---|---|
| `U+2026` `…` | `...` |
| `U+2022 U+00B7 U+2023 U+25E6` bullets | `*` |
| `U+00A9` `©` | `(c)` |
| `U+00AE` `®` | `(R)` |
| `U+2122` `™` | `(TM)` |
| `U+00B0` `°` | ` degrees` |
| `U+00BD U+00BC U+00BE` `½ ¼ ¾` | `1/2 1/4 3/4` |
| `U+2032 U+2033` `′ ″` | `' "` |
| `U+00D7 U+00F7` `× ÷` | `x /` |
| `U+2190`–`U+21FF` arrows | `->` and `<-` as they fit |
| `£ € ¥ ¢` | `GBP EUR JPY c` |

Anything still non-ASCII after all of the above is **dropped**, not replaced
with `?`. A stray glyph that survives is better silently absent than sitting in
the text as an untypeable character.

## 7. Footnote markers

Books carry markers that are not part of the sentence and that nobody wants to
type.

- Superscript digits `U+00B9 U+00B2 U+00B3` and `U+2070`–`U+209F`: removed.
- A bracketed number immediately after a word, with no space before it —
  `word[1]`, `word[12]` — is a marker: removed.
- `word{1}` likewise.
- A bare `*` or `†` or `‡` attached to the end of a word: removed.
- A bracketed number *with* a space before it is left alone. `see [3] below`
  is prose about a reference; `word[3]` is a marker.

## 8. Front matter

The extracted text of a book opens with a title page, a copyright page, and
often a table of contents. Nobody wants to type an ISBN.

The rule is deliberately conservative, because the two failures are not
symmetrical: a false trim silently loses real text, while a missed one costs a
minute of typing.

- Scan a window at the start of the text: the first 10%, capped at 20 000
  characters, but never less than 2 000 — the floor is what keeps the rule
  meaningful on a short text, where a tenth is a line and a half.
- Within it, find the lines that match a chapter opening: `CHAPTER`,
  `Chapter`, `PART`, `BOOK`, `PROLOGUE` or `EPILOGUE` with an optional number
  or title after it, or a bare roman numeral, alone on the line.
- Take the **first such heading with at least 400 characters behind it** —
  behind it meaning before the next heading, or before the end of the text for
  the last one.
- If no heading has that much room, take the **first** heading.
- If that heading is not already at offset zero, drop everything before it.
- If there are no headings at all, change nothing.

The 400-character gap is what separates a real heading from a table of
contents. TOC entries sit a line apart, so a run of them is walked straight
past; the heading that has actual prose behind it is where the book starts.

Both halves of that rule were got wrong first time, and both failures were the
same failure — cutting too much:

- The first draft said "the last heading in the window". That reads a contents
  page correctly and then eats the opening chapters of any book whose first
  tenth holds more than one of them.
- The gap test then had no answer for the *last* heading, having nothing to
  measure it against, so a book whose body begins at the final front-matter
  heading was never found. Measuring that one against the end of the text
  fixes it.
- And the fallback, when nothing has room behind it, must be the **first**
  heading rather than the last. Two real chapters a few lines apart look
  exactly like two contents entries; there is no telling them apart, so keep
  the text. `TestFrontMatterDoesNotEatCloseChapters` is that case.

A book with no chapter headings keeps all of its text, which is the safe way
to fail. Every choice above leans the same way: when the rule cannot tell, it
keeps text and costs a minute of typing, rather than trimming and losing a
chapter silently.
