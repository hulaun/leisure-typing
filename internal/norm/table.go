package norm

import "strings"

// The character table. It is grouped by what a run of characters becomes, so
// that it reads as the table in SPEC.md rather than as a list of pairs, and so
// that a missing character is easy to spot and add.
//
// Anything not named here and not already ASCII is dropped. A stray glyph that
// survives every rule above is better silently absent than sitting in the text
// as something that cannot be typed.
var groups = map[string]string{
	// Quotation marks and apostrophes (SPEC §1).
	"‘’‚‛‹›ʼ´`": "'",
	"“”„‟«»":    `"`,

	// Dashes (SPEC §2). The em dash becomes a *spaced* hyphen so that
	// word—word does not turn into the single unwrappable token word-word.
	"‐‑‒–−": "-",
	"—―":    " - ",

	// Spaces (SPEC §3).
	"              　\t": " ",

	// Ligatures (SPEC §5).
	"ﬀ": "ff", "ﬁ": "fi", "ﬂ": "fl", "ﬃ": "ffi", "ﬄ": "ffl",
	"Ĳ": "IJ", "ĳ": "ij",

	// Letters that are not one letter (SPEC §5).
	"Æ": "AE", "æ": "ae", "Œ": "OE", "œ": "oe",
	"ß": "ss", "Þ": "Th", "þ": "th",

	// Accented Latin, folded to the bare letter. This is the Latin-1 and
	// Latin Extended-A coverage the spec calls for, which is essentially
	// every novel in English; golang.org/x/text is deliberately not a
	// dependency for it.
	"ÀÁÂÃÄÅĀĂĄ":  "A",
	"àáâãäåāăą":  "a",
	"ÇĆĈĊČ":      "C",
	"çćĉċč":      "c",
	"ÐĎĐ":        "D",
	"ðďđ":        "d",
	"ÈÉÊËĒĔĖĘĚ":  "E",
	"èéêëēĕėęě":  "e",
	"ĜĞĠĢ":       "G",
	"ĝğġģ":       "g",
	"ĤĦ":         "H",
	"ĥħ":         "h",
	"ÌÍÎÏĨĪĬĮİ":  "I",
	"ìíîïĩīĭįı":  "i",
	"Ĵ":          "J",
	"ĵ":          "j",
	"Ķ":          "K",
	"ķĸ":         "k",
	"ĹĻĽĿŁ":      "L",
	"ĺļľŀł":      "l",
	"ÑŃŅŇŊ":      "N",
	"ñńņňŉŋ":     "n",
	"ÒÓÔÕÖØŌŎŐ":  "O",
	"òóôõöøōŏő":  "o",
	"ŔŖŘ":        "R",
	"ŕŗř":        "r",
	"ŚŜŞŠ":       "S",
	"śŝşšſ":      "s",
	"ŢŤŦ":        "T",
	"ţťŧ":        "t",
	"ÙÚÛÜŨŪŬŮŰŲ": "U",
	"ùúûüũūŭůűų": "u",
	"Ŵ":          "W",
	"ŵ":          "w",
	"ÝŶŸ":        "Y",
	"ýÿŷ":        "y",
	"ŹŻŽ":        "Z",
	"źżž":        "z",

	// Punctuation and symbols (SPEC §6).
	"…":     "...",
	"•·‣◦⁃": "*",
	"©":     "(c)",
	"®":     "(R)",
	"™":     "(TM)",
	"°":     " degrees",
	"½":     "1/2",
	"¼":     "1/4",
	"¾":     "3/4",
	"⅓":     "1/3",
	"⅔":     "2/3",
	"⅛":     "1/8",
	"′":     "'",
	"″":     `"`,
	"×":     "x",
	"÷":     "/",
	"¡":     "!",
	"¿":     "?",
	"←⇐":    "<-",
	"→⇒":    "->",
	"£":     "GBP",
	"€":     "EUR",
	"¥":     "JPY",
	"¢":     "c",
	"§":     "section ",
	"¶":     "",
}

// removed are the characters that must vanish rather than become anything.
//
// The invisible ones are the reason this matters: typing stalls dead on a
// character that cannot be produced, and a soft hyphen or a zero-width joiner
// is maddening precisely because the screen shows nothing wrong.
var removed = map[rune]bool{
	'\u00ad': true, // soft hyphen — inside a word, joining it back up
	'\u200b': true, // zero-width space
	'\u200c': true, // zero-width non-joiner
	'\u200d': true, // zero-width joiner
	'\u2060': true, // word joiner
	'\ufeff': true, // byte order mark
	'¹':      true, // superscript one, two, three: footnote markers (SPEC §7)
	'²':      true,
	'³':      true,
	'†':      true, // dagger and double dagger, likewise
	'‡':      true,
}

// table is groups, flattened to one rune at a time.
var table = func() map[rune]string {
	m := make(map[rune]string, 512)
	for from, to := range groups {
		for _, r := range from {
			m[r] = to
		}
	}
	return m
}()

// isRemoved reports whether a rune is dropped outright.
func isRemoved(r rune) bool {
	if removed[r] {
		return true
	}
	// Superscripts and subscripts: footnote markers, never prose (SPEC §7).
	return r >= 0x2070 && r <= 0x209f
}

// toASCII applies the table, dropping anything it does not cover.
//
// This is the pass that buys the renderer its one-rune-one-cell property, and
// with it the freedom never to meet the East-Asian-width or combining-character
// problem that makes general terminal text layout hard.
func toASCII(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n':
			b.WriteByte('\n')
		case r == ' ' || r == ' ': // line and paragraph separators
			b.WriteByte('\n')
		case isRemoved(r):
			// dropped
		case r >= 0x20 && r <= 0x7e:
			b.WriteRune(r)
		case r < 0x20:
			// Tabs are in the table above; every other C0 control goes.
			if rep, ok := table[r]; ok {
				b.WriteString(rep)
			}
		default:
			if rep, ok := table[r]; ok {
				b.WriteString(rep)
			}
			// Anything else is dropped rather than replaced with '?'.
		}
	}
	return b.String()
}
