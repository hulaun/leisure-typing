package com.hulaun.leisuretyping

/**
 * The typing state — a port of cmd/bt/read.go, navigate.go and place.go.
 *
 * This is not a typing test. There is no timer, no words per minute, no score
 * and no results screen, and a wrong character is marked rather than blocked
 * on. The one number that matters is progress through the book. See CLAUDE.md
 * for why, and do not add the others here just because a phone makes them easy.
 */

const val UNTYPED: Byte = 0
const val CORRECT: Byte = 1
const val WRONG: Byte = 2

class Reader(val meta: Meta, text: String, startOffset: Int) {

    val doc = Doc(text)
    val status = ByteArray(text.length)

    var offset: Int = startOffset.coerceIn(0, text.length)
        private set

    /** Set by the view once it knows how many columns fit. */
    var cols: Int = 40

    /** True when the position has moved since the last save. */
    var dirty: Boolean = false

    /** Where the last jump came from, for undo. */
    var prevOffset: Int = 0
        private set
    var hasPrev: Boolean = false
        private set

    init {
        // Everything before the resumed position was typed in an earlier
        // session. It renders as context either way, but marking it keeps
        // backspacing across the resume point from showing a screenful of
        // false errors.
        for (i in 0 until offset) status[i] = CORRECT
        skipWhitespace()
    }

    val length: Int get() = doc.length

    /**
     * The wrap width. One column is reserved: the active line draws a cell
     * past its text, for the caret to sit in when the next thing to type is
     * the whitespace a line break consumed. The renderer and the line movement
     * must agree on this exactly, or Down lands somewhere other than the line
     * the reader can see below the caret.
     */
    fun pageWidth(): Int = maxOf(MIN_WIDTH, cols - 1)

    // --- typing --------------------------------------------------------------

    /**
     * The whole of the typing rule.
     *
     * A wrong character is marked and the caret advances anyway. It is not
     * blocked on, and the wrong character is never inserted into the text: the
     * line stays aligned and the wrapping stays stable, so the eye is not
     * yanked out of the sentence to fix a character nobody will read again.
     */
    fun typeChar(got: Char) {
        if (offset >= doc.length) return
        status[offset] = if (got == doc.text[offset]) CORRECT else WRONG
        offset++
        skipWhitespace()
        dirty = true
    }

    /** Steps back one character and forgets what was typed there. */
    fun back() {
        if (offset == 0) return
        offset--
        // Step back over the newlines that were auto-advanced, so backspace at
        // the head of a paragraph lands on the last real character of the one
        // before it rather than in the gap.
        while (offset > 0 && doc.text[offset] == '\n') offset--
        status[offset] = UNTYPED
        dirty = true
    }

    /**
     * Steps over line and paragraph breaks. They are structure, not text to be
     * typed — nobody wants to press Enter twice between paragraphs.
     */
    private fun skipWhitespace() {
        while (offset < doc.length && doc.text[offset] == '\n') offset++
    }

    // --- moving a line at a time ---------------------------------------------

    /**
     * Moves to the head of the next display line, counting everything stepped
     * over as read.
     *
     * Typing is what holds the attention, but an hour of it tires the hands
     * long before the reading stops being wanted, and the alternative to a way
     * down the page is closing the book. On a phone that is most of the time,
     * which is why this is a button and not only a key.
     *
     * Skipped text is marked correct rather than left untyped, for the same
     * reason the chars before a resumed position are: nothing was got wrong
     * there, and backspacing back over it should not paint a screenful of red
     * that was never typed.
     */
    fun lineDown() {
        val width = pageWidth()
        if (offset >= doc.length) return

        val w = window(doc, offset, width, 0, 1)
        var target = doc.length
        for (i in w.active + 1 until w.lines.size) {
            if (w.lines[i].blank) continue // the gap between paragraphs is not a line to land on
            target = w.lines[i].start
            break
        }
        if (target <= offset) return
        for (i in offset until target) status[i] = CORRECT
        offset = target
        skipWhitespace()
        dirty = true
    }

    /**
     * Moves back a line, so a line can be re-read or retyped.
     *
     * From the middle of a line it goes to the head of that line, which is what
     * makes Up undo a Down exactly. What it steps back over is forgotten, the
     * same as backspace: the line can be typed again from the start.
     */
    fun lineUp() {
        val width = pageWidth()
        if (offset == 0) return

        val w = window(doc, offset, width, 1, 0)
        var target = w.lines[w.active].start
        if (target >= offset) {
            // Already at the head of the line: take the one above it. Zero if
            // there is none, which is the first line of the book.
            target = 0
            for (i in w.active - 1 downTo 0) {
                if (w.lines[i].blank) continue
                target = w.lines[i].start
                break
            }
        }
        if (target >= offset) return
        for (i in target until offset) status[i] = UNTYPED
        offset = target
        skipWhitespace()
        dirty = true
    }

    /**
     * Moves [n] display lines at once — where a drag of the page lets go.
     *
     * The view counts rows as they are drawn, gaps between paragraphs
     * included, so [n] counts them too: the line that ends up in the active
     * row is the one [n] rows away from the current one. A gap cannot be
     * landed on, so a count that ends on one carries on in the same direction
     * to the next real line.
     *
     * It is Up and Down repeated, not a jump: forwards marks what was passed
     * over as read, backwards forgets it, and there is no undo, because the
     * position after it is still the head of a line on the page the reader
     * was looking at.
     */
    fun scrollLines(n: Int) {
        if (n == 0) return
        val reach = kotlin.math.abs(n) + 1
        val w = window(doc, offset, pageWidth(), reach, reach)
        val step = if (n > 0) 1 else -1

        var i = (w.active + n).coerceIn(0, w.lines.size - 1)
        while (i in w.lines.indices && w.lines[i].blank) i += step
        if (i !in w.lines.indices) {
            // Ran off the text looking for a real line. Backwards that is the
            // first line of the book; forwards, the last line on offer.
            i = if (step < 0) 0 else w.lines.indexOfLast { !it.blank }
            if (i < 0) return
        }

        val target = w.lines[i].start
        // Never the opposite way to the drag: at the last line, "forwards"
        // would otherwise be the head of the line the caret is already on.
        if (if (step > 0) target <= offset else target >= offset) return
        if (target > offset) {
            for (k in offset until target) status[k] = CORRECT
        } else {
            for (k in target until offset) status[k] = UNTYPED
        }
        offset = target
        skipWhitespace()
        dirty = true
    }

    // --- jumping -------------------------------------------------------------

    /**
     * Moves to an offset that came from outside the reading — a number typed
     * into Place, read off the laptop.
     *
     * Such a number is arbitrary, so it is clamped and snapped to a word
     * boundary first. [moveTo] is the same move for a position already known
     * to be good.
     */
    fun jumpTo(target: Int) = moveTo(snapToWord(doc.text, target))

    /**
     * The jump itself, for a target that is already a valid position.
     *
     * Everything before the new position is marked read and everything after
     * it untyped, which is exactly what resuming does: nothing was got wrong
     * on either side of a jump.
     */
    fun moveTo(targetIn: Int) {
        val target = targetIn.coerceIn(0, doc.length)
        if (target == offset) return

        prevOffset = offset
        hasPrev = true
        offset = target
        for (i in status.indices) status[i] = if (i < offset) CORRECT else UNTYPED
        skipWhitespace()
        dirty = true
    }

    /**
     * Undoes the last jump.
     *
     * moveTo, not jumpTo: the position being restored was reached by reading,
     * so it is already valid and may legitimately sit on a space. Snapping it
     * would land the undo somewhere other than where the jump started, which
     * is the one thing undo must not do.
     */
    fun undoJump() {
        if (hasPrev) moveTo(prevOffset)
    }

    // --- the status line -----------------------------------------------------

    /** The chapter, percentage and word count at an offset. */
    fun progressAt(at: Int): Triple<String, Double, Int> {
        var pct = 0.0
        var words = 0
        if (doc.length > 0) {
            val through = (at.toDouble() / doc.length).coerceIn(0.0, 1.0)
            pct = through * 100
            words = (meta.words * through).toInt()
        }
        return Triple(meta.chapterAt(at), pct, words)
    }

    companion object {
        const val MIN_WIDTH = 16
    }
}

/**
 * Clamps an offset into the text and moves it to a word boundary.
 *
 * An arbitrary number lands mid-word most of the time, and beginning from the
 * fourth letter of something is both unreadable and untypable. Inside a word it
 * goes back to the head of that word; in the whitespace between words it goes
 * forward to the start of the next one. Either way the position after the move
 * is the head of something not yet read, which is the rule the line moves keep
 * as well.
 */
fun snapToWord(text: String, offIn: Int): Int {
    var off = offIn
    if (off <= 0) return 0
    if (off >= text.length) return text.length
    if (isWordSpace(text[off])) {
        while (off < text.length && isWordSpace(text[off])) off++
        return off
    }
    while (off > 0 && !isWordSpace(text[off - 1])) off--
    return off
}

private fun isWordSpace(c: Char): Boolean = c == ' ' || c == '\n'
