package com.hulaun.leisuretyping

import android.content.Context
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import android.graphics.Typeface
import android.text.InputType
import android.view.KeyEvent
import android.view.MotionEvent
import android.view.View
import android.view.inputmethod.BaseInputConnection
import android.view.inputmethod.EditorInfo
import android.view.inputmethod.InputConnection
import kotlin.math.floor

/**
 * The page: context above and below the active line, so it reads as a book
 * rather than as a test.
 *
 * The whole screen is the page, in both directions, and the palette is the
 * terminal's: dim for context, normal for typed-correct, red for typed-wrong,
 * bright for untyped on the active line, and a caret. That is the entire
 * palette and it is the only correctness signal in the app.
 *
 * This is a plain View drawing on a Canvas. The page is a monospace grid, a
 * caret and a status line; a UI framework would be most of the APK and would
 * buy nothing — the same argument CLAUDE.md makes for rejecting bubbletea on
 * the laptop.
 */
class PageView(context: Context) : View(context) {

    // The palette, in one place. Nothing else hardcodes a colour.
    private val colBackground = Color.parseColor("#FF000000")
    private val colDim = Color.parseColor("#FF6E6E6E") // context lines
    private val colNormal = Color.parseColor("#FFC8C8C8") // typed correctly
    private val colWrong = Color.parseColor("#FFFF5F5F") // typed wrongly
    private val colBright = Color.parseColor("#FFFFFFFF") // untyped, active line
    private val colRule = Color.parseColor("#FF3A3A3A")
    private val colStatus = Color.parseColor("#FF6FB8C8")
    private val colCaret = Color.parseColor("#FFFFFFFF")

    private val paint = Paint(Paint.ANTI_ALIAS_FLAG).apply {
        typeface = Typeface.MONOSPACE
        textSize = 42f
    }
    private val fill = Paint()

    var reader: Reader? = null
        set(value) {
            field = value
            measureGrid()
            invalidate()
        }

    /** Called after anything that moved the position. */
    var onMoved: (() -> Unit)? = null

    private var charW = 1f
    private var lineH = 1f
    private var cols = 1
    private var rows = 1
    private val padX get() = charW / 2f
    private val padY = 8f

    init {
        isFocusable = true
        isFocusableInTouchMode = true
        setBackgroundColor(colBackground)
    }

    /** Sets the type size in pixels and re-measures the grid. */
    fun setTextSizePx(px: Float) {
        paint.textSize = px.coerceIn(18f, 120f)
        measureGrid()
        invalidate()
    }

    fun textSizePx(): Float = paint.textSize

    private fun measureGrid() {
        charW = paint.measureText("M")
        if (charW <= 0f) charW = 1f
        lineH = paint.fontSpacing
        if (lineH <= 0f) lineH = 1f
        cols = maxOf(Reader.MIN_WIDTH, floor((width - 2 * padX) / charW).toInt())
        rows = maxOf(3, floor((height - 2 * padY) / lineH).toInt())
        reader?.cols = cols
    }

    override fun onSizeChanged(w: Int, h: Int, ow: Int, oh: Int) {
        super.onSizeChanged(w, h, ow, oh)
        measureGrid()
    }

    // --- drawing -------------------------------------------------------------

    override fun onDraw(canvas: Canvas) {
        val r = reader ?: return
        if (width == 0 || height == 0) return

        val width = r.pageWidth()
        // The page has to fit above the rule and the status line.
        val avail = rows - 2
        if (avail < 1 || width < Reader.MIN_WIDTH) return

        // Ask for a screenful of context each way; it is trimmed to what fits
        // just below. That is what fills the page in both directions.
        val context = avail
        val w = window(r.doc, r.offset, width, context, context)
        var lo = maxOf(0, w.active - context)
        val hi = minOf(w.lines.size - 1, w.active + context)
        var visible = w.lines.subList(lo, hi + 1)

        // On a short screen, drop context rather than draw more rows than
        // there are.
        if (visible.size > avail) {
            var start = (w.active - lo) - avail / 2 // keep the active line centred
            if (start < 0) start = 0
            if (start + avail > visible.size) start = visible.size - avail
            lo += start
            visible = visible.subList(start, start + avail)
        }

        // Centre the page vertically, leaving the last two rows for the rule
        // and the status line.
        val top = maxOf(0, (avail - visible.size) / 2)
        var y = padY + paint.fontMetrics.let { -it.top } + top * lineH

        for (i in visible.indices) {
            val line = visible[i]
            if (!line.blank) {
                if (lo + i == w.active) drawActiveLine(canvas, r, line, y)
                else {
                    paint.color = colDim
                    canvas.drawText(line.text, padX, y, paint)
                }
            }
            y += lineH
        }

        // The rule and the status line, on the last two rows.
        val ruleY = padY + (rows - 2) * lineH + lineH * 0.6f
        fill.color = colRule
        canvas.drawRect(padX, ruleY, padX + width * charW, ruleY + 2f, fill)

        drawStatusLine(canvas, r, width, padY + (rows - 1) * lineH + -paint.fontMetrics.top)
    }

    /**
     * The one bright row: what has been typed, in normal weight or red where
     * it was wrong, a caret, and the rest of the line ahead of it.
     */
    private fun drawActiveLine(canvas: Canvas, r: Reader, line: Line, y: Float) {
        val runes = line.text
        // An offset past the end of the drawn text is the whitespace the break
        // consumed. It still has to be typed, so it gets one cell at the end
        // of the line for the caret to sit in.
        val cells = if (line.start + runes.length < line.end) runes + " " else runes

        for (i in cells.indices) {
            val off = line.start + i
            val next = if (i == cells.length - 1) line.end else off + 1
            val x = padX + i * charW

            if (r.offset in off until next) {
                // The caret cell, drawn as the terminal draws it: reverse video.
                fill.color = colCaret
                canvas.drawRect(x, y + paint.fontMetrics.top, x + charW, y + paint.fontMetrics.bottom, fill)
                paint.color = colBackground
            } else if (off < r.offset) {
                paint.color = if (r.status[off] == WRONG) colWrong else colNormal
            } else {
                paint.color = colBright
            }
            canvas.drawText(cells, i, i + 1, x, y, paint)
        }
    }

    /**
     * The status line carries the one number that matters: progress through
     * the book. That is the whole reward loop — nobody types 40,000 words
     * without being able to see it moving.
     */
    private fun drawStatusLine(canvas: Canvas, r: Reader, width: Int, y: Float) {
        val (chapter0, pct, words) = r.progressAt(r.offset)
        val left = chapter0.ifEmpty { r.meta.title }
        val right = "%.0f%%   %s words".format(pct, comma(words))

        paint.color = colStatus
        canvas.drawText(right, padX + (width - right.length) * charW, y, paint)

        val room = width - right.length - 2
        if (room > 0) {
            val trimmed = if (left.length > room) left.substring(0, maxOf(1, room - 1)) + "…" else left
            canvas.drawText(trimmed, padX, y, paint)
        }
    }

    // --- input ---------------------------------------------------------------

    override fun onCheckIsTextEditor(): Boolean = true

    override fun onCreateInputConnection(outAttrs: EditorInfo): InputConnection {
        // Autocorrect, prediction and the composing region all have to be off:
        // this types one character at a time into a book, and an IME that
        // "helps" would insert words that are not in the text. Visible-password
        // is the variation mainstream keyboards actually honour; NO_SUGGESTIONS
        // alone is advisory and widely ignored.
        outAttrs.inputType = InputType.TYPE_CLASS_TEXT or
            InputType.TYPE_TEXT_VARIATION_VISIBLE_PASSWORD or
            InputType.TYPE_TEXT_FLAG_NO_SUGGESTIONS
        outAttrs.imeOptions = EditorInfo.IME_ACTION_NONE or
            EditorInfo.IME_FLAG_NO_FULLSCREEN or
            EditorInfo.IME_FLAG_NO_EXTRACT_UI
        outAttrs.initialSelStart = -1
        outAttrs.initialSelEnd = -1
        return BtInputConnection(this)
    }

    private fun typeOne(c: Char) {
        val r = reader ?: return
        // Newlines and tabs are structure, not text to type. They are ignored
        // rather than counted wrong: it is not a mistake in the text.
        if (c == '\n' || c == '\r' || c == '\t') return
        r.typeChar(c)
    }

    private fun afterInput() {
        invalidate()
        onMoved?.invoke()
    }

    /**
     * Feeds IME text into the reader.
     *
     * Both commit and compose are handled, because an IME that decides to
     * compose anyway would otherwise type nothing at all, and one that revises
     * what it composed would otherwise type it twice. Only the difference from
     * the previous composing text is applied.
     */
    private inner class BtInputConnection(v: View) : BaseInputConnection(v, false) {

        private var composing = ""

        private fun apply(next: String) {
            val common = commonPrefixLength(composing, next)
            repeat(composing.length - common) { reader?.back() }
            for (i in common until next.length) typeOne(next[i])
            afterInput()
        }

        override fun setComposingText(text: CharSequence?, newCursorPosition: Int): Boolean {
            val s = text?.toString() ?: ""
            apply(s)
            composing = s
            return true
        }

        override fun finishComposingText(): Boolean {
            composing = ""
            return true
        }

        override fun commitText(text: CharSequence?, newCursorPosition: Int): Boolean {
            apply(text?.toString() ?: "")
            composing = ""
            return true
        }

        override fun deleteSurroundingText(beforeLength: Int, afterLength: Int): Boolean {
            if (composing.isNotEmpty()) {
                apply("")
                composing = ""
                return true
            }
            repeat(beforeLength) { reader?.back() }
            afterInput()
            return true
        }

        override fun sendKeyEvent(event: KeyEvent): Boolean {
            if (event.action == KeyEvent.ACTION_DOWN) return onKeyDown(event.keyCode, event)
            return true
        }
    }

    override fun onKeyDown(keyCode: Int, event: KeyEvent): Boolean {
        val r = reader ?: return super.onKeyDown(keyCode, event)
        when (keyCode) {
            KeyEvent.KEYCODE_DEL -> {
                r.back(); afterInput(); return true
            }
            KeyEvent.KEYCODE_DPAD_DOWN -> {
                r.lineDown(); afterInput(); return true
            }
            KeyEvent.KEYCODE_DPAD_UP -> {
                r.lineUp(); afterInput(); return true
            }
            // Left, Right, Home and the page keys stay inert, as on the laptop:
            // a caret loose inside a line lets the saved position drift away
            // from what has actually been read.
            KeyEvent.KEYCODE_DPAD_LEFT, KeyEvent.KEYCODE_DPAD_RIGHT,
            KeyEvent.KEYCODE_MOVE_HOME, KeyEvent.KEYCODE_MOVE_END,
            KeyEvent.KEYCODE_PAGE_UP, KeyEvent.KEYCODE_PAGE_DOWN,
            KeyEvent.KEYCODE_ENTER, KeyEvent.KEYCODE_TAB -> return true
        }
        val ch = event.unicodeChar
        if (ch != 0) {
            typeOne(ch.toChar())
            afterInput()
            return true
        }
        return super.onKeyDown(keyCode, event)
    }

    override fun onTouchEvent(event: MotionEvent): Boolean {
        if (event.action == MotionEvent.ACTION_UP) {
            performClick()
        }
        return true
    }

    override fun performClick(): Boolean {
        super.performClick()
        requestFocus()
        return true
    }
}

private fun commonPrefixLength(a: String, b: String): Int {
    var i = 0
    while (i < a.length && i < b.length && a[i] == b[i]) i++
    return i
}
