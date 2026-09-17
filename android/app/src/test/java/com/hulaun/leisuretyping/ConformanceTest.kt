package com.hulaun.leisuretyping

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test

/**
 * The port agrees with the Go wrapper, exactly.
 *
 * There are two implementations of the wrapper — internal/wrap and this one —
 * and they have to agree about what an offset means. If they disagree, a
 * position carried from the laptop to the phone lands somewhere else, and the
 * reader has no way to tell that is what happened: the page just looks like
 * somewhere they have not read.
 *
 * Both sides verify against the same golden in testdata/conformance; neither is
 * the reference for the other. Regenerate it with
 *
 *     LEISURE_GOLDEN=1 go test ./internal/wrap/
 *
 * and this test will fail until the port is brought back into line.
 */
class ConformanceTest {

    private val widths = listOf(20, 33, 40, 68)
    private val offsets = listOf(0, 1, 10, 50, 120, 200, 260, 300, 400, 500, 600)

    private fun resource(name: String): String {
        val s = javaClass.classLoader!!.getResourceAsStream(name)
            ?: error(
                "$name is not on the test classpath. It comes from " +
                    "testdata/conformance, wired in by app/build.gradle.kts."
            )
        return s.use { String(it.readBytes(), Charsets.UTF_8) }
    }

    @Test
    fun `wrapping matches the Go golden`() {
        val fixture = resource("fixture.txt")
        // The two ports index the same bytes, so a checkout that translated
        // line endings must fail loudly rather than quietly disagree.
        assertFalse(
            "fixture.txt has CRLF line endings; it must be LF",
            fixture.contains('\r')
        )

        val golden = resource("wrap-golden.txt").replace("\r\n", "\n")
        val built = buildGolden(Doc(fixture))

        if (golden != built) {
            // Report the first differing record rather than two 7 KB blobs.
            val g = golden.trimEnd('\n').split('\n')
            val b = built.trimEnd('\n').split('\n')
            for (i in 0 until maxOf(g.size, b.size)) {
                val lg = g.getOrNull(i)
                val lb = b.getOrNull(i)
                if (lg != lb) {
                    throw AssertionError(
                        "the Kotlin wrapper has drifted from the Go one at " +
                            "record ${i + 1}:\n  golden $lg\n  kotlin $lb"
                    )
                }
            }
        }
        assertEquals(golden, built)
    }

    /** The same format internal/wrap/conformance_test.go writes. */
    private fun buildGolden(doc: Doc): String {
        val b = StringBuilder()
        b.append("# leisure-typing wrap conformance golden\n")
        b.append("# L <width> <index> <start> <end> <blank> <text>\n")
        b.append("# W <width> <offset> <activeStart> <activeEnd> <lines> <text>\n")

        for (width in widths) {
            allLines(doc, width).forEachIndexed { i, l ->
                val blank = if (l.blank) 1 else 0
                b.append("L $width $i ${l.start} ${l.end} $blank ${l.text}\n")
            }
        }

        // Window is what the line movement uses, so it is checked separately
        // from allLines: an agreeing full layout and a disagreeing window would
        // still send Down to the wrong place.
        for (width in widths) {
            for (off in offsets) {
                if (off > doc.length) continue
                val w = window(doc, off, width, 2, 2)
                val a = w.lines[w.active]
                b.append("W $width $off ${a.start} ${a.end} ${w.lines.size} ${a.text}\n")
            }
        }
        return b.toString()
    }

    /** No line may exceed the width, at any width. */
    @Test
    fun `no line exceeds the wrap width`() {
        val doc = Doc(resource("fixture.txt"))
        for (width in 8..80) {
            for (l in allLines(doc, width)) {
                if (l.text.length > width) {
                    throw AssertionError(
                        "at width $width a line is ${l.text.length} columns: '${l.text}'"
                    )
                }
            }
        }
    }

    /** The lines cover every char of the text, with no gap and no overlap. */
    @Test
    fun `lines cover the whole text`() {
        val doc = Doc(resource("fixture.txt"))
        for (width in listOf(20, 33, 40, 68)) {
            val lines = allLines(doc, width)
            // The first paragraph may start past leading whitespace.
            var at = lines.first().start
            for (l in lines) {
                assertEquals("width $width: a gap or overlap at $at", at, l.start)
                at = l.end
            }
            assertEquals("width $width: the lines stop short of the end", doc.length, at)
        }
    }
}
