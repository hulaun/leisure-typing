package com.hulaun.leisuretyping

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * The typing rule and the position moves, ported from cmd/bt/read_test.go and
 * place_test.go.
 *
 * These are the parts most likely to be "improved" into a typing test later, on
 * a phone more than anywhere: a soft keyboard makes a WPM counter look free.
 * It is not, and the table at the top of CLAUDE.md is why.
 */
class ReaderTest {

    private fun reader(text: String, words: Int = 100, offset: Int = 0): Reader {
        val m = Meta(
            id = "a-book", title = "A Book", sha256 = "abc123",
            runes = text.length, words = words,
        )
        return Reader(m, text, offset).apply { cols = 41 } // pageWidth() == 40
    }

    private fun Reader.type(s: String) = s.forEach { typeChar(it) }

    // --- typing --------------------------------------------------------------

    @Test
    fun `typing advances and marks correct`() {
        val r = reader("abc def\n")
        r.type("abc")
        assertEquals(3, r.offset)
        for (i in 0 until 3) assertEquals(CORRECT, r.status[i])
    }

    /**
     * The rule the whole app turns on: a wrong character is marked and the caret
     * moves on. It is not blocked on, and the wrong character is not inserted.
     */
    @Test
    fun `a wrong character does not block`() {
        val r = reader("abcdef\n")
        r.type("axc")
        assertEquals("a wrong character blocked the caret", 3, r.offset)
        assertEquals(WRONG, r.status[1])
        assertEquals("typing continued out of alignment", CORRECT, r.status[2])
        assertEquals("the text was modified", "abcdef\n", r.doc.text)
    }

    @Test
    fun `newlines are stepped over`() {
        val r = reader("ab\n\ncd\n")
        r.type("ab")
        assertEquals("the paragraph gap was not skipped", 4, r.offset)
    }

    @Test
    fun `leading newlines are skipped`() {
        assertEquals(2, reader("\n\nab\n").offset)
    }

    @Test
    fun `backspace forgets what was typed`() {
        val r = reader("abc\n")
        r.type("ab")
        r.back()
        assertEquals(1, r.offset)
        assertEquals(UNTYPED, r.status[1])
    }

    @Test
    fun `backspace across a paragraph lands on real text`() {
        val r = reader("ab\n\ncd\n")
        r.type("ab")
        r.back()
        assertEquals("backspace landed in the gap, not on a character", 1, r.offset)
    }

    @Test
    fun `backspace at the start is harmless`() {
        val r = reader("abc\n")
        r.back()
        assertEquals(0, r.offset)
    }

    @Test
    fun `typing past the end is harmless`() {
        val r = reader("ab\n")
        r.type("abcdef")
        assertEquals(r.length, r.offset)
    }

    // --- moving a line at a time ---------------------------------------------

    @Test
    fun `lineDown lands on the head of the next line`() {
        val text = "alpha bravo charlie delta echo foxtrot golf hotel india juliet " +
            "kilo lima mike november oscar papa quebec romeo sierra tango\n"
        val r = reader(text)
        val before = allLines(r.doc, r.pageWidth())
        r.lineDown()
        assertEquals("Down did not land on the second line", before[1].start, r.offset)
    }

    /**
     * Skipped text is marked correct rather than left untyped: nothing was got
     * wrong there, and backspacing back over it should not paint a screenful of
     * red that was never typed.
     */
    @Test
    fun `lineDown marks the skipped line read`() {
        val text = "alpha bravo charlie delta echo foxtrot golf hotel india juliet " +
            "kilo lima mike november oscar papa quebec romeo sierra tango\n"
        val r = reader(text)
        r.lineDown()
        for (i in 0 until r.offset) {
            assertEquals("status[$i] was left untyped", CORRECT, r.status[i])
        }
        assertTrue("Down did not mark the position for saving", r.dirty)
    }

    @Test
    fun `lineUp undoes lineDown exactly`() {
        val text = "alpha bravo charlie delta echo foxtrot golf hotel india juliet " +
            "kilo lima mike november oscar papa quebec romeo sierra tango\n"
        val r = reader(text)
        r.lineDown()
        r.lineDown()
        val was = r.offset
        r.lineUp()
        assertTrue("Up did not move back", r.offset < was)
        r.lineDown()
        assertEquals("Up then Down did not return to the same place", was, r.offset)
    }

    @Test
    fun `lineUp at the start is harmless`() {
        val r = reader("alpha bravo\n")
        r.lineUp()
        assertEquals(0, r.offset)
    }

    // --- the word snap -------------------------------------------------------

    @Test
    fun `snapToWord goes back to the head of a word`() {
        val text = "alpha bravo charlie"
        for (off in listOf(6, 7, 8, 9, 10)) {
            assertEquals("snapToWord($off)", 6, snapToWord(text, off))
        }
    }

    @Test
    fun `snapToWord goes forward out of whitespace`() {
        assertEquals("the space should go forward", 6, snapToWord("alpha bravo", 5))
        assertEquals("the paragraph gap should be crossed", 5, snapToWord("one\n\ntwo", 3))
    }

    @Test
    fun `snapToWord clamps`() {
        val text = "alpha bravo"
        assertEquals(0, snapToWord(text, -50))
        assertEquals(text.length, snapToWord(text, 9999))
    }

    /** Whatever number goes in, the result is never inside a word. */
    @Test
    fun `snapToWord never lands mid-word`() {
        val text = "The morning came in slowly over the roofs.\n\n" +
            "A cart went past and did not stop.\n"
        for (off in 0..text.length + 5) {
            val got = snapToWord(text, off)
            if (got in 1 until text.length) {
                val prev = text[got - 1]
                assertTrue(
                    "snapToWord($off) = $got is inside a word",
                    prev == ' ' || prev == '\n'
                )
            }
        }
    }

    // --- jumping -------------------------------------------------------------

    @Test
    fun `a jump marks either side`() {
        val r = reader("alpha bravo charlie delta echo\n")
        r.type("alpha bravo")
        r.jumpTo(20) // the head of "delta"
        assertEquals(20, r.offset)
        for (i in 0 until r.offset) assertEquals(CORRECT, r.status[i])
        for (i in r.offset until r.length) assertEquals(UNTYPED, r.status[i])
    }

    @Test
    fun `a jump past the end clamps`() {
        val r = reader("alpha bravo\n")
        r.jumpTo(99999)
        assertEquals(r.length, r.offset)
    }

    /**
     * Undo restores exactly where the jump started, even when that position
     * sits on a space.
     *
     * This is the bug the Go version had first: undo went through the snap, and
     * a position reached by typing may legitimately sit on a space, so the undo
     * landed one char away from where it began.
     */
    @Test
    fun `undo restores the exact position a jump started from`() {
        val r = reader("alpha bravo charlie delta echo\n")
        r.type("alpha") // offset 5, sitting on the space
        val was = r.offset
        assertEquals(5, was)

        r.jumpTo(20)
        assertEquals(20, r.offset)

        r.undoJump()
        assertEquals("undo went through the word snap", was, r.offset)
    }

    @Test
    fun `there is no undo before any jump`() {
        val r = reader("alpha bravo\n")
        assertFalse(r.hasPrev)
        r.undoJump()
        assertEquals(0, r.offset)
    }

    // --- the status line -----------------------------------------------------

    /**
     * Progress runs cleanly from nothing to the whole book, and never goes
     * backwards. This is the one number the app shows, and the whole reward
     * loop rests on it: nobody types 40,000 words without seeing it move.
     */
    @Test
    fun `progress runs from zero to a hundred percent and never backwards`() {
        val text = "alpha bravo charlie delta ".repeat(8)
        val r = reader(text, words = 400)

        assertEquals(0.0, r.progressAt(0).second, 0.001)
        assertEquals(100.0, r.progressAt(r.length).second, 0.001)
        assertEquals(400, r.progressAt(r.length).third)

        var last = -1.0
        for (off in 0..r.length) {
            val pct = r.progressAt(off).second
            assertTrue("the percentage went backwards at " + off, pct >= last)
            assertTrue("the percentage left 0..100 at " + off, pct >= 0.0 && pct <= 100.0)
            last = pct
        }
    }

    @Test
    fun `the chapter in force is the last one at or before the offset`() {
        val m = Meta(
            id = "b", title = "B", sha256 = "x", runes = 100, words = 10,
            chapters = listOf(Chapter("CHAPTER I", 0), Chapter("CHAPTER II", 50)),
        )
        assertEquals("CHAPTER I", m.chapterAt(0))
        assertEquals("CHAPTER I", m.chapterAt(49))
        assertEquals("CHAPTER II", m.chapterAt(50))
        assertEquals("CHAPTER II", m.chapterAt(99))
    }
}
