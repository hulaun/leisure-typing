// Command bt is a reader for typing books out.
//
//	bt                    open the current book where it was left
//	bt import <file>      import a book and make it current
//	bt list               list imported books, with progress
//	bt use <name>         switch the current book
//	bt pos [offset]       show the reading position, or go to one
//	bt export <name>      write one .btbook file into bt-books/, to carry elsewhere
//
// It is not a typing test. There is no timer, no words per minute, no score
// and no results screen, and a wrong character is marked rather than blocked
// on. See CLAUDE.md for why.
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/hulaun/leisure-typing/internal/book"
	"github.com/hulaun/leisure-typing/internal/convert"
	"github.com/hulaun/leisure-typing/internal/progress"
)

const usage = `bt — read a book by typing it out

  bt                 open the current book where you left it
  bt import <file>   import a book (.epub is the format to prefer)
  bt list            list imported books, with progress
  bt use <name>      switch the current book
  bt pos [offset]    show where you are, or go to an offset
  bt export <name>   write the book as one .btbook file into bt-books/

While reading: type. Down and Up move a line without typing it, for when
the hands are tired but the reading is not. Ctrl+G opens Place, which
shows the offset you are at and takes one you type — that is how the
position is carried between two machines. Esc or Ctrl+C quits, and your
place is saved.`

func main() {
	// Everything from here to the end of run() is inside the recover in
	// read(); this level only handles the non-terminal commands.
	if err := dispatch(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "bt:", err)
		os.Exit(1)
	}
}

func dispatch(args []string) error {
	root, err := storageRoot()
	if err != nil {
		return err
	}
	store, err := book.Open(root)
	if err != nil {
		return err
	}

	if len(args) == 0 {
		return read(store)
	}

	switch args[0] {
	case "import", "add":
		if len(args) != 2 {
			return errors.New("usage: bt import <file>")
		}
		return importBook(store, args[1])

	case "list", "ls":
		return list(store)

	case "use", "switch":
		if len(args) != 2 {
			return errors.New("usage: bt use <name>")
		}
		m, err := store.Find(args[1])
		if err != nil {
			return err
		}
		if err := store.SetCurrent(m.ID); err != nil {
			return err
		}
		fmt.Printf("current book is now %s\n", m.Title)
		return nil

	case "export":
		if len(args) < 2 || len(args) > 3 {
			return fmt.Errorf("usage: bt export <name> [destination]\n\n"+
				"with no destination it goes to %s", store.BundleDir())
		}
		dst := ""
		if len(args) == 3 {
			dst = args[2]
		}
		return export(store, args[1], dst)

	case "pos", "place":
		if len(args) > 2 {
			return errors.New("usage: bt pos [offset]")
		}
		return pos(store, args[1:])

	case "where":
		fmt.Println(store.Root)
		return nil

	case "help", "-h", "--help":
		fmt.Println(usage)
		return nil

	default:
		if strings.HasPrefix(args[0], "-") {
			return fmt.Errorf("unknown flag %s\n\n%s", args[0], usage)
		}
		// `bt sometitle` is a reasonable thing to type; treat it as `bt use`
		// followed by opening the book.
		m, err := store.Find(args[0])
		if err != nil {
			return fmt.Errorf("%w\n\n%s", err, usage)
		}
		if err := store.SetCurrent(m.ID); err != nil {
			return err
		}
		return read(store)
	}
}

func importBook(store *book.Store, path string) error {
	m, err := store.Import(path)
	if err != nil {
		return err
	}
	fmt.Printf("imported %s", m.Title)
	if m.Author != "" {
		fmt.Printf(" by %s", m.Author)
	}
	fmt.Printf("\n  %s words", comma(m.Words))
	if n := len(m.Chapters); n > 0 {
		fmt.Printf(", %d chapters", n)
	}
	fmt.Printf("\n  %s\n", store.Dir(m.ID))
	fmt.Println("\nit is now the current book — run bt to start")
	return nil
}

func list(store *book.Store) error {
	books, err := store.List()
	if err != nil {
		return err
	}
	if len(books) == 0 {
		fmt.Println("no books yet — bt import <file>")
		if !convert.EbookConvert.Available() && !convert.PDFToText.Available() {
			fmt.Println("(.epub and .txt work out of the box; .pdf and .mobi need a converter)")
		}
		return nil
	}

	current, _ := store.Current()
	for _, m := range books {
		mark := " "
		if m.ID == current.ID {
			mark = "*"
		}
		pct := 0.0
		if p, err := progress.Load(store.ProgressPath(m.ID), m.SHA256); err == nil && m.Runes > 0 {
			pct = float64(p.Offset) / float64(m.Runes) * 100
		}
		fmt.Printf("%s %-24s %5.1f%%  %9s words  %s\n",
			mark, trim(m.ID, 24), pct, comma(m.Words), trim(m.Title, 40))
	}
	return nil
}

// storageRoot decides where storage/ lives.
//
// In a checkout it is the repo's own storage/ directory, which is gitignored
// as one unit — books are the user's files and none of them belongs in the
// repo. Once the binary is installed somewhere else, there is no checkout to
// find, and it falls back to the user's local app data.
// export writes a book out as one file, to be carried to another device.
//
// Getting a book onto the phone happens once per book, so it is a file you
// drag across the cable rather than a sync protocol. What crosses afterwards
// is the position, which is one integer (bt pos, and the Place screen).
//
// The hash prefix is printed because it is what the two devices compare: an
// offset only means anything against one exact text.txt, and if the far side
// shows different characters the number is not transferable.
func export(store *book.Store, name, dst string) error {
	m, err := store.Find(name)
	if err != nil {
		return err
	}
	path, err := store.Export(m, dst)
	if err != nil {
		return err
	}
	abs, aerr := filepath.Abs(path)
	if aerr != nil {
		abs = path
	}
	info, serr := os.Stat(path)

	fmt.Println("exported " + m.Title)
	fmt.Println("  " + abs)
	if serr == nil {
		fmt.Println("  " + comma(int((info.Size()+1023)/1024)) +
			" KB   text " + shortHash(m.SHA256))
	}
	fmt.Println()
	fmt.Println("Copy it to the other device and import it there. Both ends will")
	fmt.Println("show the same text hash, which is what makes an offset mean the")
	fmt.Println("same thing on both.")
	return nil
}

// pos prints the reading position, or moves it.
//
// This is the Place screen without the reader: the point of both is that the
// position is one integer, so reading it off one machine and typing it into
// another is a complete sync with no cable and no protocol. The hash is
// printed alongside because an offset only means anything against one exact
// text.txt — if the two machines show different hashes, the number is not
// transferable and nothing else will tell you.
func pos(store *book.Store, args []string) error {
	m, err := store.Current()
	if err != nil {
		return err
	}
	text, err := store.Text(m)
	if err != nil {
		return err
	}
	p, err := progress.Load(store.ProgressPath(m.ID), m.SHA256)
	if err != nil {
		return err
	}
	if p.Offset > len(text) {
		p.Offset = len(text)
	}

	if len(args) == 1 {
		// Grouped input is accepted, because the number is printed grouped
		// and retyping it with the commas is the natural thing to do.
		digits := strings.NewReplacer(",", "", "_", "", ".", "", " ", "").Replace(args[0])
		n, cerr := strconv.Atoi(digits)
		if cerr != nil || n < 0 {
			return fmt.Errorf("bt pos: %q is not an offset", args[0])
		}
		was := p.Offset
		p.Offset = snapToWord(text, n)
		if err := progress.Save(store.ProgressPath(m.ID), progress.Progress{
			Offset: p.Offset,
			SHA256: m.SHA256,
		}); err != nil {
			return err
		}
		fmt.Println("moved from " + comma(was) + " to " + comma(p.Offset))
		fmt.Println()
	}

	chapter, pct, words := progressAt(m, len(text), p.Offset)
	if chapter == "" {
		chapter = m.Title
	}
	fmt.Println(m.Title)
	fmt.Println("  " + spread(chapter,
		fmt.Sprintf("%.0f%%   %s words", pct, comma(words)), posWidth))
	fmt.Println("  " + spread("offset "+comma(p.Offset),
		"text "+shortHash(m.SHA256), posWidth))
	return nil
}

// posWidth is the column `bt pos` prints into. The reader has the terminal to
// measure; this does not, and a fixed width keeps the two ends lined up.
const posWidth = 58

func storageRoot() (string, error) {
	if env := os.Getenv("LEISURE_HOME"); env != "" {
		return env, nil
	}

	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		for i := 0; i < 4; i++ { // bin/bt.exe is one level down; allow a few
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				return filepath.Join(dir, "storage"), nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("cannot find anywhere to keep books: %w", err)
	}
	return filepath.Join(base, "leisure-typing", "storage"), nil
}

func trim(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

// comma groups a count for the status line: 9240 becomes 9,240.
func comma(n int) string {
	s := fmt.Sprint(n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}
