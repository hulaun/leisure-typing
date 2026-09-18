package com.hulaun.leisuretyping

import android.graphics.Color

/**
 * The colours, in one place. Nothing else hardcodes a colour.
 *
 * The page palette is the terminal's — dim for context, normal for
 * typed-correct, red for typed-wrong, bright for untyped on the active line,
 * and a caret — and it is the only correctness signal in the app. A theme
 * changes the values, never the roles: light is the same five signals on paper.
 *
 * Light was added on request on 2026-09-18, after two days of reading white on
 * black on the phone. It is a warm off-white rather than pure white, and the
 * dark theme's context grey is kept well off the background, for the same
 * reason: the page is looked at for half an hour at a time.
 */
class Palette(
    val background: Int,
    val dim: Int, // context lines
    val normal: Int, // typed correctly
    val wrong: Int, // typed wrongly
    val bright: Int, // untyped, active line
    val rule: Int,
    val status: Int,
    val caret: Int,
    val bar: Int, // the button bar under the page
    val barText: Int,
    val heading: Int, // the library
    val muted: Int,
) {
    companion object {
        val DARK = Palette(
            background = Color.parseColor("#FF000000"),
            dim = Color.parseColor("#FF6E6E6E"),
            normal = Color.parseColor("#FFC8C8C8"),
            wrong = Color.parseColor("#FFFF5F5F"),
            bright = Color.parseColor("#FFFFFFFF"),
            rule = Color.parseColor("#FF3A3A3A"),
            status = Color.parseColor("#FF6FB8C8"),
            caret = Color.parseColor("#FFFFFFFF"),
            bar = Color.parseColor("#FF101010"),
            barText = Color.parseColor("#FFC8C8C8"),
            heading = Color.parseColor("#FFFFFFFF"),
            muted = Color.parseColor("#FF9A9A9A"),
        )

        val LIGHT = Palette(
            background = Color.parseColor("#FFF6F1E7"),
            dim = Color.parseColor("#FF9C9488"),
            normal = Color.parseColor("#FF4A4640"),
            wrong = Color.parseColor("#FFC62828"),
            bright = Color.parseColor("#FF1A1A1A"),
            rule = Color.parseColor("#FFD9D0C0"),
            status = Color.parseColor("#FF2E6E7E"),
            caret = Color.parseColor("#FF1A1A1A"),
            bar = Color.parseColor("#FFEAE3D5"),
            barText = Color.parseColor("#FF3A3632"),
            heading = Color.parseColor("#FF1A1A1A"),
            muted = Color.parseColor("#FF6E675E"),
        )
    }
}
