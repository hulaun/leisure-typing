package com.hulaun.leisuretyping

import android.app.Activity
import android.app.AlertDialog
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.text.Editable
import android.text.InputType
import android.text.TextWatcher
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.view.WindowInsets
import android.view.WindowInsetsController
import android.view.inputmethod.InputMethodManager
import android.widget.AdapterView
import android.widget.ArrayAdapter
import android.widget.Button
import android.widget.EditText
import android.widget.FrameLayout
import android.widget.LinearLayout
import android.widget.ListView
import android.widget.TextView
import android.widget.Toast

/**
 * bt on the phone: a reader whose page-turn is the keyboard.
 *
 * Books arrive as .btbook files exported from the laptop — extraction and
 * normalization happen there, exactly once. What crosses afterwards is the
 * reading position, which is one integer: read it off one device, type it into
 * the other. That is the whole of the sync, and it is why there is no cable
 * protocol here.
 */
class MainActivity : Activity() {

    private lateinit var store: Store
    private lateinit var root: FrameLayout
    private var page: PageView? = null
    private var reader: Reader? = null
    private var keyboardShown = false
    private var palette = Palette.DARK

    /** The reader's own views, recoloured in place when the theme changes. */
    private var readerCol: LinearLayout? = null
    private var bar: LinearLayout? = null
    private val barButtons = ArrayList<Button>()

    private val saveTick = Handler(Looper.getMainLooper())
    private val saver = object : Runnable {
        override fun run() {
            saveIfDirty()
            saveTick.postDelayed(this, SAVE_DEBOUNCE_MS)
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        store = Store(filesDir)
        root = FrameLayout(this)
        setContentView(root)
        palette = if (prefs().getBoolean(KEY_LIGHT, false)) Palette.LIGHT else Palette.DARK
        applySystemBars()
        applySafeArea(root)

        val current = store.currentId()
        if (current != null) openBook(current) else showLibrary()
    }

    override fun onResume() {
        super.onResume()
        saveTick.postDelayed(saver, SAVE_DEBOUNCE_MS)
    }

    override fun onPause() {
        super.onPause()
        saveTick.removeCallbacks(saver)
        // The position is saved unconditionally on the way out. Losing ten
        // minutes of a five-week book is the worst bug this app can have.
        saveIfDirty(force = true)
    }

    private fun saveIfDirty(force: Boolean = false) {
        val r = reader ?: return
        if (!r.dirty && !force) return
        try {
            store.saveProgress(r.meta.id, Progress(r.offset, r.meta.sha256))
            r.dirty = false
        } catch (_: Exception) {
            // Nothing useful to say mid-page; the next tick tries again.
        }
    }

    // --- the reader ----------------------------------------------------------

    private fun openBook(id: String) {
        try {
            val m = store.meta(id)
            val text = store.text(m)
            val p = store.loadProgress(id, m.sha256)
            store.setCurrent(id)

            val r = Reader(m, text, p.offset)
            reader = r

            val v = PageView(this)
            v.palette = palette
            v.reader = r
            v.setTextSizePx(prefs().getFloat(KEY_TEXT_SIZE, 42f))
            v.onMoved = { }
            page = v

            root.removeAllViews()
            root.addView(readerLayout(v), ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.MATCH_PARENT)
            v.requestFocus()
            showKeyboard(v)
        } catch (e: Exception) {
            say(e.message ?: "That book could not be opened.")
            showLibrary()
        }
    }

    private fun readerLayout(v: PageView): View {
        val col = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setBackgroundColor(palette.background)
        }
        readerCol = col
        col.addView(v, LinearLayout.LayoutParams(
            ViewGroup.LayoutParams.MATCH_PARENT, 0, 1f))

        val bar = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            setBackgroundColor(palette.bar)
        }
        this.bar = bar
        barButtons.clear()
        // Down is a first-class control here, not only a key. Typing is what
        // holds the attention, but the hands tire long before the reading stops
        // being wanted, and on a phone that is most of the time. Down counts
        // the line as read and keeps progress moving; Up goes back so a line
        // can be typed again.
        fun barButton(label: String, onClick: () -> Unit) {
            val b = Button(this).apply {
                text = label
                setTextColor(palette.barText)
                setBackgroundColor(palette.bar)
                setOnClickListener { onClick() }
            }
            barButtons.add(b)
            bar.addView(b, LinearLayout.LayoutParams(0, ViewGroup.LayoutParams.WRAP_CONTENT, 1f))
        }
        barButton("↑") { reader?.lineUp(); v.invalidate() }
        barButton("↓") { reader?.lineDown(); v.invalidate() }
        barButton("Keys") { toggleKeyboard(v) }
        barButton("Place") { showPlace() }
        barButton("◐") { toggleTheme() }
        barButton("≡") { showMenu() }

        col.addView(bar, LinearLayout.LayoutParams(
            ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT))
        return col
    }

    // --- the theme -----------------------------------------------------------

    /**
     * Swaps dark and light, and remembers the choice. The views are recoloured
     * in place rather than rebuilt, so what has been typed on the active line
     * (and the red in it) survives the switch.
     */
    private fun toggleTheme() {
        palette = if (palette === Palette.DARK) Palette.LIGHT else Palette.DARK
        prefs().edit().putBoolean(KEY_LIGHT, palette === Palette.LIGHT).apply()

        page?.palette = palette
        readerCol?.setBackgroundColor(palette.background)
        bar?.setBackgroundColor(palette.bar)
        for (b in barButtons) {
            b.setTextColor(palette.barText)
            b.setBackgroundColor(palette.bar)
        }
        applySystemBars()
    }

    /**
     * Colours what shows around the page: the window behind the safe-area
     * padding, and the status and navigation bars, whose icons have to turn
     * dark on the light page or they vanish into it.
     */
    @Suppress("DEPRECATION")
    private fun applySystemBars() {
        root.setBackgroundColor(palette.background)
        window.statusBarColor = palette.background
        window.navigationBarColor = palette.bar
        val light = palette === Palette.LIGHT
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            val mask = WindowInsetsController.APPEARANCE_LIGHT_STATUS_BARS or
                WindowInsetsController.APPEARANCE_LIGHT_NAVIGATION_BARS
            window.insetsController?.setSystemBarsAppearance(if (light) mask else 0, mask)
        } else {
            val flags = View.SYSTEM_UI_FLAG_LIGHT_STATUS_BAR or View.SYSTEM_UI_FLAG_LIGHT_NAVIGATION_BAR
            val d = window.decorView
            d.systemUiVisibility = if (light) d.systemUiVisibility or flags
            else d.systemUiVisibility and flags.inv()
        }
    }

    // --- Place ---------------------------------------------------------------

    /**
     * Where you are, and a field to type a new offset into.
     *
     * The preview is the whole reason this is a screen and not a prompt: a
     * dropped digit sends you to 4% instead of 23%, and you see that before
     * pressing Go. It also catches a number copied from a different book,
     * which a check digit could not. The six characters of the text hash are
     * what you compare against the laptop: an offset only means anything
     * against one exact text.txt.
     */
    private fun showPlace() {
        val r = reader ?: return
        val v = page ?: return

        val pad = (16 * resources.displayMetrics.density).toInt()
        val box = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(pad, pad, pad, pad)
        }

        val (chapter, pct, words) = r.progressAt(r.offset)
        val here = TextView(this).apply {
            text = "%s\n%.0f%%   %s words\n\nnow at %s        text %s".format(
                chapter.ifEmpty { r.meta.title }, pct, comma(words),
                comma(r.offset), shortHash(r.meta.sha256))
            setTextIsSelectable(true) // so the number can be copied off
        }
        box.addView(here)

        val field = EditText(this).apply {
            inputType = InputType.TYPE_CLASS_NUMBER
            hint = "go to offset"
        }
        box.addView(field)

        val preview = TextView(this).apply {
            setPadding(0, pad / 2, 0, 0)
            text = "Type the offset shown on the laptop."
        }
        box.addView(preview)

        fun target(): Int? {
            val digits = field.text.toString().filter { it.isDigit() }
            if (digits.isEmpty()) return null
            return digits.toIntOrNull()
        }

        field.addTextChangedListener(object : TextWatcher {
            override fun afterTextChanged(s: Editable?) {
                val n = target()
                if (n == null) {
                    preview.text = "Type the offset shown on the laptop."
                    return
                }
                val at = snapToWord(r.doc.text, n)
                val (ch, p, _) = r.progressAt(at)
                val w = window(r.doc, at, r.pageWidth(), 0, 0)
                val line = w.lines[w.active].text.trim()
                val head = StringBuilder()
                if (n > r.length) head.append("past the end; ")
                head.append("lands at ").append(comma(at))
                if (ch.isNotEmpty()) head.append(", in ").append(ch)
                head.append(", %.0f%%".format(p))
                preview.text = "$head\n\n$line"
            }

            override fun beforeTextChanged(s: CharSequence?, a: Int, b: Int, c: Int) {}
            override fun onTextChanged(s: CharSequence?, a: Int, b: Int, c: Int) {}
        })

        val d = AlertDialog.Builder(this)
            .setTitle("Place")
            .setView(box)
            .setPositiveButton("Go") { _, _ ->
                target()?.let { r.jumpTo(it) }
                saveIfDirty(force = true)
                v.invalidate()
            }
            .setNegativeButton("Cancel", null)

        // Jumping is the only thing here that can lose your place, so it is the
        // only thing with an undo.
        if (r.hasPrev) {
            d.setNeutralButton("Undo (${comma(r.prevOffset)})") { _, _ ->
                r.undoJump()
                saveIfDirty(force = true)
                v.invalidate()
            }
        }
        d.show()
    }

    // --- the library ---------------------------------------------------------

    private fun showLibrary() {
        reader = null
        page = null
        readerCol = null
        bar = null
        barButtons.clear()
        val books = store.list()

        val col = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setBackgroundColor(palette.background)
        }
        val pad = (16 * resources.displayMetrics.density).toInt()

        col.addView(TextView(this).apply {
            text = if (books.isEmpty()) "No books yet" else "Books"
            setTextColor(palette.heading)
            textSize = 22f
            setPadding(pad, pad, pad, pad / 2)
        })

        if (books.isEmpty()) {
            col.addView(TextView(this).apply {
                text = "On the laptop:\n\n    bt export <book>\n\n" +
                    "Copy the .btbook file here over the cable, then import it below."
                setTextColor(palette.muted)
                setPadding(pad, 0, pad, pad)
            })
        } else {
            val labels = books.map { m ->
                val p = store.loadProgress(m.id, m.sha256)
                val pct = if (m.runes > 0) 100.0 * p.offset / m.runes else 0.0
                "%s\n%.0f%%   offset %s   text %s".format(
                    m.title, pct, comma(p.offset), shortHash(m.sha256))
            }
            val list = ListView(this)
            list.adapter = object : ArrayAdapter<String>(
                this, android.R.layout.simple_list_item_1, labels
            ) {
                override fun getView(pos: Int, cv: View?, parent: ViewGroup): View {
                    val tv = super.getView(pos, cv, parent) as TextView
                    tv.setTextColor(palette.normal)
                    return tv
                }
            }
            list.setOnItemClickListener { _, _, pos, _ -> openBook(books[pos].id) }
            list.onItemLongClickListener = AdapterView.OnItemLongClickListener { _, _, pos, _ ->
                confirmDelete(books[pos])
                true
            }
            col.addView(list, LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, 0, 1f))
        }

        col.addView(Button(this).apply {
            text = "Import a .btbook"
            setOnClickListener { pickBundle() }
        }, LinearLayout.LayoutParams(
            ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.WRAP_CONTENT))

        root.removeAllViews()
        root.addView(col, ViewGroup.LayoutParams.MATCH_PARENT, ViewGroup.LayoutParams.MATCH_PARENT)
    }

    private fun confirmDelete(m: Meta) {
        AlertDialog.Builder(this)
            .setTitle("Remove ${m.title}?")
            .setMessage("The book and your place in it are deleted from this " +
                "device. The laptop copy is untouched.")
            .setPositiveButton("Remove") { _, _ ->
                store.delete(m.id)
                showLibrary()
            }
            .setNegativeButton("Keep", null)
            .show()
    }

    // --- importing -----------------------------------------------------------

    private fun pickBundle() {
        val i = Intent(Intent.ACTION_OPEN_DOCUMENT).apply {
            addCategory(Intent.CATEGORY_OPENABLE)
            // .btbook has no registered MIME type, so anything openable is
            // offered and the file itself is validated after it is read.
            type = "*/*"
        }
        try {
            startActivityForResult(i, REQ_IMPORT)
        } catch (_: Exception) {
            say("No file picker on this device.")
        }
    }

    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        super.onActivityResult(requestCode, resultCode, data)
        if (requestCode != REQ_IMPORT || resultCode != RESULT_OK) return
        val uri = data?.data ?: return

        val name = uri.lastPathSegment?.substringAfterLast('/') ?: "the file"
        try {
            val m = contentResolver.openInputStream(uri).use { input ->
                if (input == null) throw BookError("$name could not be opened.")
                store.importBundle(input, name)
            }
            say("Imported ${m.title}")
            openBook(m.id)
        } catch (e: Exception) {
            AlertDialog.Builder(this)
                .setTitle("Could not import")
                .setMessage(e.message ?: "$name is not a .btbook.")
                .setPositiveButton("OK", null)
                .show()
        }
    }

    // --- the menu ------------------------------------------------------------

    private fun showMenu() {
        val items = arrayOf("Books", "Import a .btbook", "Bigger text", "Smaller text")
        AlertDialog.Builder(this)
            .setItems(items) { _, which ->
                when (which) {
                    0 -> { saveIfDirty(force = true); showLibrary() }
                    1 -> pickBundle()
                    2 -> resize(+4f)
                    3 -> resize(-4f)
                }
            }
            .show()
    }

    private fun resize(by: Float) {
        val v = page ?: return
        val size = v.textSizePx() + by
        v.setTextSizePx(size)
        prefs().edit().putFloat(KEY_TEXT_SIZE, v.textSizePx()).apply()
    }

    // --- odds and ends -------------------------------------------------------

    @Deprecated("Superseded by OnBackInvokedDispatcher, which needs AndroidX")
    @Suppress("DEPRECATION")
    override fun onBackPressed() {
        if (reader != null) {
            saveIfDirty(force = true)
            showLibrary()
        } else {
            super.onBackPressed()
        }
    }

    /**
     * Keeps the page and the button bar out from under the system bars.
     *
     * An app targeting SDK 35 is edge-to-edge whether it asks to be or not, so
     * the window extends behind the status bar at the top and the gesture pill
     * or the three-button nav at the bottom. Without this the bottom row of
     * buttons sits underneath the navigation bar and cannot be tapped.
     *
     * The bottom inset takes whichever is larger, the navigation bar or the
     * keyboard. Edge-to-edge also means the window no longer resizes itself
     * when the IME opens — adjustResize stops doing the work — so the keyboard
     * has to be padded for here, or it covers the buttons instead.
     *
     * Padding the one root view covers every screen: the reader, the library
     * and the empty state all sit inside it.
     */
    private fun applySafeArea(target: View) {
        target.setOnApplyWindowInsetsListener { v, insets ->
            val e = safeEdges(insets)
            v.setPadding(e[0], e[1], e[2], e[3])
            insets
        }
        target.requestApplyInsets()
    }

    /** left, top, right, bottom — the edges it is not safe to draw in. */
    @Suppress("DEPRECATION")
    private fun safeEdges(insets: WindowInsets): IntArray {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            // displayCutout as well as systemBars, for a notch or a punch-hole
            // in landscape, where it eats into the side rather than the top.
            val bars = insets.getInsets(
                WindowInsets.Type.systemBars() or WindowInsets.Type.displayCutout()
            )
            val ime = insets.getInsets(WindowInsets.Type.ime())
            return intArrayOf(
                maxOf(bars.left, ime.left),
                bars.top,
                maxOf(bars.right, ime.right),
                maxOf(bars.bottom, ime.bottom),
            )
        }
        // Below API 30 the window is not edge-to-edge and adjustResize still
        // handles the keyboard, so the system-window insets are the whole of it.
        return intArrayOf(
            insets.systemWindowInsetLeft,
            insets.systemWindowInsetTop,
            insets.systemWindowInsetRight,
            insets.systemWindowInsetBottom,
        )
    }

    private fun prefs() = getSharedPreferences("bt", Context.MODE_PRIVATE)

    private fun imm() = getSystemService(Context.INPUT_METHOD_SERVICE) as InputMethodManager

    private fun showKeyboard(v: View) {
        v.requestFocus()
        imm().showSoftInput(v, InputMethodManager.SHOW_IMPLICIT)
        keyboardShown = true
    }

    /**
     * Shows or hides the keyboard, tracking which it did.
     *
     * toggleSoftInput would be one call, and is deprecated; asking the window
     * whether the IME is visible needs API 30 or AndroidX. Remembering it is
     * the cheapest of the three, and the worst case is one wasted tap.
     */
    private fun toggleKeyboard(v: View) {
        v.requestFocus()
        keyboardShown = if (keyboardShown) {
            imm().hideSoftInputFromWindow(v.windowToken, 0)
            false
        } else {
            imm().showSoftInput(v, InputMethodManager.SHOW_IMPLICIT)
            true
        }
    }

    private fun say(s: String) {
        Toast.makeText(this, s, Toast.LENGTH_SHORT).apply {
            setGravity(Gravity.CENTER, 0, 0)
        }.show()
    }

    companion object {
        private const val REQ_IMPORT = 1
        private const val SAVE_DEBOUNCE_MS = 2000L
        private const val KEY_TEXT_SIZE = "textSizePx"
        private const val KEY_LIGHT = "lightTheme"
    }
}
