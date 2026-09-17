package com.hulaun.leisuretyping

/**
 * Greedy word wrap into display lines — a port of internal/wrap.
 *
 * Wrapping is a view and is never stored (CLAUDE.md, decision 5). The reading
 * position is a rune offset into text.txt; line breaks are computed here, at
 * render time, from the current screen width. A rotation therefore re-renders
 * and invalidates nothing.
 *
 * This must produce the same lines as the Go package for the same offsets and
 * the same width, because the two ends have to agree about what an offset
 * means. The one intentional difference is [Doc], which caches the paragraph
 * split: Go re-splits on every call and can afford to, a phone redrawing on
 * every keystroke would rather not.
 *
 * Offsets are indices into a Kotlin String. That is the same as a Go rune
 * offset only because normalization guarantees plain ASCII, so one char is one
 * rune is one cell (CLAUDE.md, decision 2). [Store] refuses a text that is not
 * ASCII rather than let that assumption fail quietly.
 */

/**
 * One row of the page, covering the range [start, end) of the text.
 *
 * [text] is what is drawn: the range with any whitespace the break consumed
 * trimmed off, so it is never longer than the wrap width and never ends in a
 * space. The chars between start + text.length and end are the whitespace the
 * break ate — they still have to be typed, and the reader draws them as one
 * trailing cell.
 *
 * A [blank] line is the gap between paragraphs, drawn as an empty row.
 */
class Line(
    val start: Int,
    val end: Int,
    val text: String = "",
    val blank: Boolean = false,
) {
    /** The column at which the char at [offset] is drawn. */
    fun column(offset: Int): Int {
        var col = offset - start
        if (col > text.length) col = text.length
        if (col < 0) col = 0
        return col
    }
}

/**
 * One paragraph: the content range [start, end), and [gap], one past the
 * whitespace that follows it.
 *
 * Paragraphs are the anchor that makes windowed wrapping exact. Each wraps
 * independently of every other, so wrapping a slice of them produces identical
 * lines to wrapping the whole book — which is what lets a screenful be laid
 * out without touching the other 230 KB.
 */
class Para(val start: Int, val end: Int, val gap: Int)

/** A text with its paragraph split computed once. */
class Doc(val text: String) {
    val paras: List<Para> = splitParas(text)
    val length: Int get() = text.length
}

private fun isSpace(c: Char): Boolean = c == ' ' || c == '\n'

private fun skipSpace(text: String, from: Int): Int {
    var i = from
    while (i < text.length && isSpace(text[i])) i++
    return i
}

private fun newlinesIn(text: String, from: Int, to: Int): Int {
    var n = 0
    for (i in from until to) if (text[i] == '\n') n++
    return n
}

/**
 * Splits the text into paragraphs.
 *
 * A run of whitespace holding two or more newlines separates them. A single
 * newline is *inside* a paragraph: normalization has already joined
 * hard-wrapped lines.
 *
 * The whole whitespace run is measured in one go. Testing it a char at a time
 * is what went wrong in the Go version first time round: a predicate that only
 * looked forward from a newline never counted the second newline of a pair as
 * part of the break, so every paragraph after the first began one char early,
 * sitting on that newline, and carried it into its first display line — where
 * it split the row in two and the line appeared to be drawn twice.
 */
fun splitParas(text: String): List<Para> {
    val out = ArrayList<Para>()
    val n = text.length
    var i = skipSpace(text, 0)
    while (i < n) {
        val start = i
        var end = n
        var gap = n
        var j = start
        while (j < n) {
            if (text[j] != '\n') {
                j++
                continue
            }
            val after = skipSpace(text, j)
            if (after >= n || newlinesIn(text, j, after) >= 2) {
                end = j
                gap = after
                break
            }
            j = after // a lone newline: the paragraph carries on
        }
        out.add(Para(start, end, gap))
        i = gap
    }
    return out
}

private fun mkLine(text: String, start: Int, textEnd: Int, end: Int): Line {
    // A display line is one row by construction. A newline left inside it
    // would be drawn into the row, splitting it in two. Replacing rather than
    // trimming keeps one char to one column, which is what column() depends
    // on.
    val s = text.substring(start, textEnd).trimEnd(' ', '\n').replace('\n', ' ')
    return Line(start, end, s)
}

/** Lays out one paragraph greedily into lines no wider than [widthIn]. */
fun wrapPara(text: String, p: Para, widthIn: Int): List<Line> {
    val width = if (widthIn < 1) 1 else widthIn
    val out = ArrayList<Line>()
    var i = p.start
    while (i < p.end) {
        val lineStart = i
        val end = i + width
        if (end >= p.end) {
            out.add(mkLine(text, lineStart, p.end, p.end))
            return out
        }
        // Walk back to the last space at or before the width limit.
        var brk = -1
        for (j in end downTo lineStart + 1) {
            if (isSpace(text[j])) {
                brk = j
                break
            }
        }
        if (brk < 0) {
            // One word longer than the whole line: hard-break it. Nothing else
            // keeps the guarantee that no line exceeds the width.
            out.add(mkLine(text, lineStart, end, end))
            i = end
            continue
        }
        // Consume the run of spaces at the break; they are typed, but they are
        // not drawn at the start of the next line.
        var next = brk
        while (next < p.end && isSpace(text[next])) next++
        out.add(mkLine(text, lineStart, brk, next))
        i = next
    }
    if (out.isEmpty()) out.add(Line(p.start, p.end))
    return out
}

/**
 * Hands the whitespace after the book's final paragraph to the last line, so
 * the lines cover every char of the text with no gap at the end.
 */
private fun closeTail(out: MutableList<Line>, paras: List<Para>, hi: Int) {
    if (out.isNotEmpty() && hi >= 0 && hi == paras.size - 1) {
        val last = out[out.size - 1]
        out[out.size - 1] = Line(last.start, paras[hi].gap, last.text, last.blank)
    }
}

/**
 * Wraps the whole text. This is what the conformance golden measures against;
 * the reader uses [window] instead, which produces identical lines without
 * touching the rest of the book.
 */
fun allLines(doc: Doc, width: Int): List<Line> {
    val out = ArrayList<Line>()
    for (i in doc.paras.indices) {
        out.addAll(wrapPara(doc.text, doc.paras[i], width))
        if (i < doc.paras.size - 1) {
            out.add(Line(doc.paras[i].end, doc.paras[i].gap, "", true))
        }
    }
    closeTail(out, doc.paras, doc.paras.size - 1)
    return out
}

/** The result of [window]: the lines, and which of them holds the offset. */
class Windowed(val lines: List<Line>, val active: Int)

/** Wraps paragraphs [lo, hi] and locates the line holding [offset]. */
private fun layout(doc: Doc, lo: Int, hi: Int, offset: Int, width: Int): Windowed {
    val out = ArrayList<Line>()
    for (i in lo..hi) {
        out.addAll(wrapPara(doc.text, doc.paras[i], width))
        if (i < hi) out.add(Line(doc.paras[i].end, doc.paras[i].gap, "", true))
    }
    closeTail(out, doc.paras, hi)

    var active = 0
    for (i in out.indices) {
        if (out[i].blank) continue
        active = i
        if (offset < out[i].end) break
    }
    return Windowed(out, active)
}

/**
 * Lays out just enough of the book to draw a page: the line holding [offset],
 * with at least [before] lines above it and [after] below.
 *
 * Because paragraphs wrap independently, these are exactly the lines a full
 * layout would have produced for the same offsets.
 */
fun window(doc: Doc, offsetIn: Int, width: Int, before: Int, after: Int): Windowed {
    if (doc.paras.isEmpty()) return Windowed(listOf(Line(0, 0)), 0)
    val offset = if (offsetIn < 0) 0 else offsetIn

    // The paragraph holding the offset — or, if the offset landed in the gap
    // between two, the one that follows it.
    var pi = 0
    for (i in doc.paras.indices) {
        val p = doc.paras[i]
        if (offset < p.end) {
            pi = i
            break
        }
        pi = i
        if (offset < p.gap) {
            if (i + 1 < doc.paras.size) pi = i + 1
            break
        }
    }

    var lo = pi
    var hi = pi
    while (true) {
        val w = layout(doc, lo, hi, offset, width)
        val enough = (w.active >= before || lo == 0) &&
            (w.lines.size - 1 - w.active >= after || hi == doc.paras.size - 1)
        if (enough) return w
        if (lo > 0) lo--
        if (hi < doc.paras.size - 1) hi++
    }
}
