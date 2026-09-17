package com.hulaun.leisuretyping

import org.json.JSONArray
import org.json.JSONObject
import java.io.File
import java.io.InputStream
import java.security.MessageDigest
import java.time.Instant
import java.util.zip.ZipInputStream

/**
 * The on-disk store, and the .btbook importer.
 *
 * The layout mirrors the laptop's, one directory per book:
 *
 *     files/books/<id>/
 *       text.txt        the normalized text, immutable, the only thing read
 *       meta.json       title, author, sha256, chapter offsets
 *       progress.json   { offset, updated, sha256 }
 *     files/current     the id of the book that opens by default
 *
 * This device never imports a book from source and never normalizes anything.
 * It takes a .btbook — already extracted, already normalized, with a sha256
 * over the text — copies the bytes and verifies them. Extraction happens
 * exactly once, on the laptop (CLAUDE.md, decision 1), which is also why this
 * app needs no EPUB parser and no character table.
 */

class Chapter(val title: String, val offset: Int)

class Meta(
    val id: String,
    val title: String,
    val author: String = "",
    val format: String = "",
    val sha256: String,
    val runes: Int,
    val words: Int,
    val chapters: List<Chapter> = emptyList(),
) {
    /** The heading in force at an offset. */
    fun chapterAt(offset: Int): String {
        var title = ""
        for (c in chapters) {
            if (c.offset > offset) break
            title = c.title
        }
        return title
    }

    fun toJson(): JSONObject {
        val o = JSONObject()
        o.put("id", id)
        o.put("title", title)
        if (author.isNotEmpty()) o.put("author", author)
        o.put("format", format)
        o.put("sha256", sha256)
        o.put("runes", runes)
        o.put("words", words)
        val arr = JSONArray()
        for (c in chapters) {
            arr.put(JSONObject().put("title", c.title).put("offset", c.offset))
        }
        if (arr.length() > 0) o.put("chapters", arr)
        return o
    }

    companion object {
        fun fromJson(o: JSONObject, idOverride: String? = null): Meta {
            val chapters = ArrayList<Chapter>()
            val arr = o.optJSONArray("chapters")
            if (arr != null) {
                for (i in 0 until arr.length()) {
                    val c = arr.getJSONObject(i)
                    chapters.add(Chapter(c.optString("title"), c.optInt("offset")))
                }
            }
            return Meta(
                id = idOverride ?: o.optString("id"),
                title = o.optString("title").ifEmpty { "Book" },
                author = o.optString("author"),
                format = o.optString("format"),
                sha256 = o.optString("sha256"),
                runes = o.optInt("runes"),
                words = o.optInt("words"),
                chapters = chapters,
            )
        }
    }
}

class Progress(val offset: Int, val sha256: String)

/** Thrown for every refusal that has something useful to tell the user. */
class BookError(message: String) : Exception(message)

class Store(root: File) {

    private val booksDir = File(root, "books")
    private val currentFile = File(root, "current")

    init {
        booksDir.mkdirs()
    }

    fun dir(id: String) = File(booksDir, id)
    fun textPath(id: String) = File(dir(id), "text.txt")
    fun metaPath(id: String) = File(dir(id), "meta.json")
    fun progressPath(id: String) = File(dir(id), "progress.json")

    fun list(): List<Meta> {
        val out = ArrayList<Meta>()
        val entries = booksDir.listFiles() ?: return out
        for (d in entries.sortedBy { it.name }) {
            if (!d.isDirectory) continue
            try {
                out.add(meta(d.name))
            } catch (_: Exception) {
                // A half-written import is skipped, not fatal to the list.
            }
        }
        return out
    }

    fun meta(id: String): Meta =
        Meta.fromJson(JSONObject(metaPath(id).readText()), id)

    /**
     * Reads a book's text and verifies it against the hash in meta.json.
     *
     * A mismatch is an error offering a re-import, never a silent resume: the
     * reading position is an offset into these exact bytes, and resuming
     * against different ones drops the reader into the wrong chapter without
     * anything looking wrong.
     */
    fun text(m: Meta): String {
        val body = textPath(m.id).readBytes()
        val got = sha256Hex(body)
        if (got != m.sha256) {
            throw BookError(
                "${m.title}: the text does not match its recorded hash. " +
                    "Import it again from the laptop."
            )
        }
        return String(body, Charsets.UTF_8)
    }

    fun currentId(): String? {
        if (!currentFile.exists()) return null
        val id = currentFile.readText().trim()
        return if (id.isNotEmpty() && metaPath(id).exists()) id else null
    }

    fun setCurrent(id: String) = writeAtomic(currentFile, (id + "\n").toByteArray())

    fun loadProgress(id: String, sha256: String): Progress {
        val f = progressPath(id)
        if (!f.exists()) return Progress(0, sha256)
        return try {
            val o = JSONObject(f.readText())
            val saved = o.optString("sha256")
            // A position measured against a different text is not this book's
            // position. Start over rather than resume somewhere arbitrary.
            if (saved.isNotEmpty() && saved != sha256) Progress(0, sha256)
            else Progress(maxOf(0, o.optInt("offset")), sha256)
        } catch (_: Exception) {
            Progress(0, sha256)
        }
    }

    /**
     * Saves the position, through a temp file and a rename.
     *
     * Losing ten minutes of a five-week book is the worst bug this app can
     * have, so a crash mid-write must not be able to leave a truncated
     * progress.json behind.
     */
    fun saveProgress(id: String, p: Progress) {
        val o = JSONObject()
        o.put("offset", maxOf(0, p.offset))
        o.put("updated", Instant.now().toString())
        o.put("sha256", p.sha256)
        writeAtomic(progressPath(id), (o.toString(2) + "\n").toByteArray())
    }

    fun delete(id: String) {
        dir(id).deleteRecursively()
        if (currentId() == id) currentFile.delete()
    }

    // --- importing a bundle --------------------------------------------------

    /**
     * Imports a .btbook.
     *
     * Nothing is re-derived: the text is copied byte for byte and checked
     * against the hash the exporter recorded, and the word count and chapter
     * offsets are taken as written. Re-deriving on this side is exactly what
     * would make the same number mean two different places on the two devices.
     */
    fun importBundle(input: InputStream, sourceName: String): Meta {
        var infoRaw: ByteArray? = null
        var metaRaw: ByteArray? = null
        var textRaw: ByteArray? = null
        var progRaw: ByteArray? = null

        ZipInputStream(input).use { zin ->
            while (true) {
                val e = zin.nextEntry ?: break
                // Only these four names are read, matched exactly. Nothing
                // from the archive is used as a path, so a crafted entry name
                // cannot escape the store.
                when (e.name) {
                    "bundle.json" -> infoRaw = readCapped(zin, META_LIMIT, e.name)
                    "meta.json" -> metaRaw = readCapped(zin, META_LIMIT, e.name)
                    "text.txt" -> textRaw = readCapped(zin, TEXT_LIMIT, e.name)
                    "progress.json" -> progRaw = readCapped(zin, META_LIMIT, e.name)
                }
                zin.closeEntry()
            }
        }

        val info = infoRaw ?: throw BookError(
            "$sourceName is not a .btbook — it has no bundle.json."
        )
        val version = try {
            JSONObject(String(info, Charsets.UTF_8)).optInt("version", 0)
        } catch (_: Exception) {
            throw BookError("$sourceName is damaged: bundle.json is corrupt.")
        }
        if (version > BUNDLE_VERSION) {
            throw BookError(
                "$sourceName was written by a newer version of bt " +
                    "(bundle version $version, this understands $BUNDLE_VERSION). " +
                    "Update this app."
            )
        }

        val metaBytes = metaRaw ?: throw BookError("$sourceName has no meta.json.")
        val body = textRaw ?: throw BookError("$sourceName has no text.txt.")

        val m0 = try {
            Meta.fromJson(JSONObject(String(metaBytes, Charsets.UTF_8)))
        } catch (_: Exception) {
            throw BookError("$sourceName is damaged: meta.json is corrupt.")
        }

        // The check the whole transfer rests on.
        if (sha256Hex(body) != m0.sha256) {
            throw BookError(
                "$sourceName is damaged: its text does not match the hash " +
                    "recorded in it. Export it again on the laptop."
            )
        }

        // One char must be one offset, or every position in the book is wrong
        // by an amount that grows as you read. Normalization guarantees ASCII
        // (CLAUDE.md, decision 2); this is where that guarantee is checked
        // rather than assumed.
        val text = String(body, Charsets.UTF_8)
        for (c in text) {
            if (c.code > 0x7F) {
                throw BookError(
                    "$sourceName contains characters that are not plain ASCII, " +
                        "so offsets would not line up with the laptop. " +
                        "Re-import the book there and export it again."
                )
            }
        }

        val id = freeId(if (m0.id.isNotEmpty()) m0.id else slug(m0.title))
        val m = Meta(
            id = id,
            title = m0.title,
            author = m0.author,
            format = m0.format.ifEmpty { "btbook" },
            sha256 = m0.sha256,
            runes = if (m0.runes > 0) m0.runes else text.length,
            words = m0.words,
            chapters = m0.chapters,
        )

        // The position rides along, so the first transfer lands you where you
        // already were rather than at the start.
        var offset = 0
        val pr = progRaw
        if (pr != null) {
            try {
                val o = JSONObject(String(pr, Charsets.UTF_8))
                if (o.optString("sha256") == m.sha256) offset = o.optInt("offset")
            } catch (_: Exception) {
                // A corrupt position is not a reason to refuse the book.
            }
        }
        offset = offset.coerceIn(0, text.length)

        dir(id).mkdirs()
        writeAtomic(textPath(id), body)
        writeAtomic(metaPath(id), (m.toJson().toString(2) + "\n").toByteArray())
        saveProgress(id, Progress(offset, m.sha256))
        setCurrent(id)
        return m
    }

    private fun freeId(base0: String): String {
        val base = base0.ifEmpty { "book" }
        if (!dir(base).exists()) return base
        for (n in 2..999) {
            val id = "$base-$n"
            if (!dir(id).exists()) return id
        }
        throw BookError("Too many books named $base.")
    }

    companion object {
        const val BUNDLE_VERSION = 1
        const val BUNDLE_EXT = ".btbook"
        private const val TEXT_LIMIT = 64L shl 20
        private const val META_LIMIT = 4L shl 20
    }
}

// --- helpers ----------------------------------------------------------------

fun sha256Hex(b: ByteArray): String {
    val d = MessageDigest.getInstance("SHA-256").digest(b)
    val sb = StringBuilder(d.size * 2)
    for (x in d) sb.append("%02x".format(x))
    return sb.toString()
}

/** The prefix shown next to the offset, and compared against the laptop's. */
fun shortHash(sum: String): String =
    if (sum.isEmpty()) "------" else if (sum.length > 6) sum.substring(0, 6) else sum

fun slug(s: String): String {
    val sb = StringBuilder()
    var lastHyphen = true
    for (r in s.lowercase()) {
        if (r in 'a'..'z' || r in '0'..'9') {
            sb.append(r)
            lastHyphen = false
        } else if (!lastHyphen) {
            sb.append('-')
            lastHyphen = true
        }
    }
    return sb.toString().trim('-')
}

/** Groups a count the way the status line does: 9240 becomes 9,240. */
fun comma(n: Int): String {
    val s = n.toString()
    if (s.length <= 3) return s
    val sb = StringBuilder()
    for (i in s.indices) {
        if (i > 0 && (s.length - i) % 3 == 0) sb.append(',')
        sb.append(s[i])
    }
    return sb.toString()
}

private fun readCapped(input: InputStream, limit: Long, name: String): ByteArray {
    val out = java.io.ByteArrayOutputStream()
    val buf = ByteArray(32 * 1024)
    var total = 0L
    while (true) {
        val n = input.read(buf)
        if (n <= 0) break
        total += n
        if (total > limit) throw BookError("$name is implausibly large.")
        out.write(buf, 0, n)
    }
    return out.toByteArray()
}

private fun writeAtomic(target: File, body: ByteArray) {
    target.parentFile?.mkdirs()
    val tmp = File.createTempFile("tmp-", null, target.parentFile)
    try {
        tmp.outputStream().use { it.write(body); it.fd.sync() }
        if (!tmp.renameTo(target)) {
            // renameTo will not replace on some filesystems; fall back.
            target.delete()
            if (!tmp.renameTo(target)) throw BookError("Could not write ${target.name}.")
        }
    } finally {
        tmp.delete()
    }
}
