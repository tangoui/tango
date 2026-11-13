package editor

import (
	"slices"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/theme"
)

// Test cases for the line, indentation and comment commands. Monaco 0.56's
// npm package ships no unit tests, so the cases follow the behaviour of
// its sources (linesOperations, copyLinesCommand, moveLinesCommand,
// sortLinesCommand, shiftCommand, cursorTypeEditOperations, indentation,
// lineCommentCommand, blockCommentCommand) and the cases of Monaco's own
// test suites for those commands as far as they carry over.
//
// Texts are written with markers: "|" is a caret, "[" a selection's
// anchor and "]" its head. In inputs the selections are listed in the
// order of their first marker (the first is the primary); outputs show
// every selection in document order.

// parseMarked splits marked text into the text and its selections.
func parseMarked(t *testing.T, s string) (string, []Selection) {
	t.Helper()
	var b strings.Builder
	var sels []Selection
	open := -1 // index in sels of the open selection
	openAnchor := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		off := b.Len()
		switch c {
		case '|':
			sels = append(sels, Selection{Anchor: off, Head: off})
		case '[', ']':
			isAnchor := c == '['
			if open < 0 {
				sels = append(sels, Selection{Anchor: off, Head: off})
				open, openAnchor = len(sels)-1, isAnchor
				continue
			}
			if isAnchor == openAnchor {
				t.Fatalf("marked text %q: %c twice", s, c)
			}
			if isAnchor {
				sels[open].Anchor = off
			} else {
				sels[open].Head = off
			}
			open = -1
		default:
			b.WriteByte(c)
		}
	}
	if open >= 0 {
		t.Fatalf("marked text %q: unclosed selection", s)
	}
	if len(sels) == 0 {
		t.Fatalf("marked text %q: no selection", s)
	}
	return b.String(), sels
}

// marked renders the State's text with its selections.
func marked(st *State) string {
	text := st.Snapshot().String()
	type mark struct {
		off int
		s   string
	}
	var ms []mark
	for _, x := range st.Sels {
		if x.Anchor == x.Head {
			ms = append(ms, mark{x.Head, "|"})
			continue
		}
		ms = append(ms, mark{x.Anchor, "["}, mark{x.Head, "]"})
	}
	// Insert from the end; marks at one offset are inserted last first, so
	// they read in order (one selection's head before the next one's anchor).
	slices.Reverse(ms)
	slices.SortStableFunc(ms, func(a, b mark) int { return b.off - a.off })
	for _, m := range ms {
		text = text[:m.off] + m.s + text[m.off:]
	}
	return text
}

// opCase is one table case: the command run on in (marked) gives want.
type opCase struct {
	name string
	cmd  string
	in   string
	want string
	file string       // the file name (language); default a.txt
	opts func(*State) // adjusts the options before the command
}

func runCases(t *testing.T, cases []opCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			file := c.file
			if file == "" {
				file = "a.txt"
			}
			text, sels := parseMarked(t, c.in)
			st := NewState(text, file, theme.Base("vs"))
			if c.opts != nil {
				c.opts(st)
			}
			st.SetSelections(sels)
			before := st.Snapshot().String()
			ran := st.Execute(c.cmd, nil)
			if got := marked(st); got != c.want {
				t.Errorf("%s:\n got %q\nwant %q", c.cmd, got, c.want)
			}
			// A command reports true when it changed the text (or made
			// edits that cancel out, as moving an empty line onto one).
			changed := st.Snapshot().String() != before
			if changed && !ran {
				t.Errorf("Execute reported false, the text changed")
			}
			if changed {
				// One undo step restores the text and the selections.
				if !st.Undo() || st.Snapshot().String() != before {
					t.Errorf("undo: %q, want %q", st.Snapshot().String(), before)
				}
				if got := marked(st); got != canonical(t, c.in) {
					t.Errorf("after undo: %q, want %q", got, canonical(t, c.in))
				}
			}
		})
	}
}

// canonical renders marked text the way marked does (selections in
// document order, merged as the State merges them).
func canonical(t *testing.T, s string) string {
	text, sels := parseMarked(t, s)
	st := NewState(text, "a.txt", nil)
	st.SetSelections(sels)
	return marked(st)
}

func TestCopyLines(t *testing.T) {
	up, down := "editor.action.copyLinesUpAction", "editor.action.copyLinesDownAction"
	runCases(t, []opCase{
		{"down first line", down, "fir|st\nsecond", "first\nfir|st\nsecond", "", nil},
		{"up first line", up, "fir|st\nsecond", "fir|st\nfirst\nsecond", "", nil},
		{"down last line", down, "first\nsec|ond", "first\nsecond\nsec|ond", "", nil},
		{"up last line", up, "first\nsec|ond", "first\nsec|ond\nsecond", "", nil},
		{"down empty line", down, "a\n|\nb", "a\n\n|\nb", "", nil},
		{"up empty line", up, "a\n|\nb", "a\n|\n\nb", "", nil},
		{"down selection over two lines", down, "[ab\nc]d\ne", "ab\ncd\n[ab\nc]d\ne", "", nil},
		{"up selection over two lines", up, "[ab\nc]d\ne", "[ab\nc]d\nab\ncd\ne", "", nil},
		// A selection ending at the start of a line leaves that line out
		// and keeps ending at the start of the line after the copy.
		{"down selection ending at column 0", down, "[ab\n]cd", "ab\n[ab\n]cd", "", nil},
		{"up selection ending at column 0", up, "[ab\n]cd", "[ab\n]ab\ncd", "", nil},
		{"down reversed selection", down, "a]b\nc[d", "ab\ncd\na]b\nc[d", "", nil},
		// Several cursors on one line copy it once.
		{"down two carets on one line", down, "a|b|c\nd", "abc\na|b|c\nd", "", nil},
		{"up two carets on one line", up, "a|b|c\nd", "a|b|c\nabc\nd", "", nil},
		{"down carets on two lines", down, "a|\nb|\nc", "a\na|\nb\nb|\nc", "", nil},
		{"down selection and caret sharing a line", down, "[a\nb]b|\nc", "a\nbb\n[a\nb]b|\nc", "", nil},
	})
}

// TestCopyLinesCRLF: a CRLF document copies its lines with CRLF (the
// buffer stores LF and keeps the EOL).
func TestCopyLinesCRLF(t *testing.T) {
	st := NewState("one\r\ntwo\r\nthree\r\n", "a.txt", nil)
	st.SetSelections(carets(st.Snapshot().LineStart(1) + 1))
	if !st.Execute("editor.action.copyLinesDownAction", nil) {
		t.Fatal("copyLinesDown did nothing")
	}
	if got := st.Snapshot().TextWithEOL(); got != "one\r\ntwo\r\ntwo\r\nthree\r\n" {
		t.Errorf("copy down: %q", got)
	}
	if got := marked(st); got != "one\ntwo\nt|wo\nthree\n" {
		t.Errorf("caret: %q", got)
	}
	st.Execute("editor.action.copyLinesUpAction", nil)
	if got := st.Snapshot().TextWithEOL(); got != "one\r\ntwo\r\ntwo\r\ntwo\r\nthree\r\n" {
		t.Errorf("copy up: %q", got)
	}
	if strings.Contains(st.Snapshot().String(), "\r") {
		t.Error("the stored text has a CR")
	}
}

func TestMoveLines(t *testing.T) {
	up, down := "editor.action.moveLinesUpAction", "editor.action.moveLinesDownAction"
	runCases(t, []opCase{
		// The edges: the first line does not move up, the last not down.
		{"up first line", up, "fi|rst\nsecond", "fi|rst\nsecond", "", nil},
		{"down last line", down, "first\nsec|ond", "first\nsec|ond", "", nil},
		{"down last line before final newline", down, "a\nb|\n", "a\n\nb|", "", nil},
		{"up second line", up, "first\nsec|ond\nthird", "sec|ond\nfirst\nthird", "", nil},
		{"down first line", down, "fi|rst\nsecond\nthird", "second\nfi|rst\nthird", "", nil},
		{"up last line", up, "a\nb\nc|", "a\nc|\nb", "", nil},
		{"down to the last line", down, "a\nb|\nc", "a\nc\nb|", "", nil},
		{"up empty line", up, "a\n|", "|\na", "", nil},
		{"down empty line onto empty line", down, "|\n\nx", "\n|\nx", "", nil},
		{"up selection", up, "a\n[b\nc]\nd", "[b\nc]\na\nd", "", nil},
		{"down selection ending at column 0", down, "[a\n]b\nc", "b\n[a\n]c", "", nil},
		{"up selection at the top", up, "[a\nb]\nc", "[a\nb]\nc", "", nil},
		// Cursors on one line or neighbouring lines move together.
		{"up two carets on one line", up, "a\nb|c|\nd", "b|c|\na\nd", "", nil},
		{"up carets on neighbouring lines", up, "a\nb|\nc|\nd", "b|\nc|\na\nd", "", nil},
		{"down carets on separate lines", down, "a|\nb\nc|\nd", "b\na|\nd\nc|", "", nil},
		// Neighbouring lines are one block: with the first line in it,
		// nothing moves up (Monaco would move line 2 above line 1).
		{"up carets on the first two lines", up, "a|\nb|\nc", "a|\nb|\nc", "", nil},
		{"up one block at the top, one not", up, "a|\nb\nc|", "a|\nc|\nb", "", nil},
	})
}

func TestDeleteLines(t *testing.T) {
	del := "editor.action.deleteLines"
	runCases(t, []opCase{
		{"first line", del, "ab|c\ndef\nghi", "de|f\nghi", "", nil},
		{"middle line", del, "abc\nd|ef\nghi", "abc\ng|hi", "", nil},
		{"last line", del, "abc\ndef\ngh|i", "abc\nde|f", "", nil},
		{"only line", del, "ab|c", "|", "", nil},
		{"column clamped", del, "abcdef|\nx\ny", "x|\ny", "", nil},
		{"selection", del, "a\n[b\nc]\nd", "a\nd|", "", nil},
		{"selection ending at column 0", del, "[a\n]b\nc", "|b\nc", "", nil},
		{"selection to the end", del, "a\n[b\nc]", "a|", "", nil},
		{"two carets on one line", del, "a|b|c\nd", "d|", "", nil},
		{"carets on neighbouring lines", del, "a|\nb|\nc\nd", "c|\nd", "", nil},
		{"carets on separate lines", del, "a|\nb\nc|\nd", "b|\nd|", "", nil},
		{"empty document", del, "|", "|", "", nil},
	})
}

func TestJoinLines(t *testing.T) {
	join := "editor.action.joinLines"
	runCases(t, []opCase{
		{"caret", join, "hel|lo\n   world\nend", "hello| world\nend", "", nil},
		{"trailing blanks collapse", join, "a  |\n  b", "a |b", "", nil},
		{"empty first line", join, "|\n  b", "|b", "", nil},
		{"blank next line", join, "a|\n   \nb", "a|\nb", "", nil},
		{"last line", join, "a\nb|", "a\nb|", "", nil},
		{"selection over three lines", join, "[a\n b\n c]\nd", "[a b c]\nd", "", nil},
		{"selection on one line", join, "a[b]c\nd", "a[b]c d", "", nil},
		{"selection end keeps its distance", join, "[ab\ncd]ef\ng", "[ab cd]ef\ng", "", nil},
		{"two carets on one line", join, "a|b|\nc", "ab| c", "", nil},
		// Monaco merges carets on neighbouring lines into one selection.
		{"carets on neighbouring lines", join, "a|\nb|\nc\nd", "a[ b]\nc\nd", "", nil},
		{"carets on separate lines", join, "a|\nb\nc|\nd", "a| b\nc| d", "", nil},
	})
}

func TestInsertLine(t *testing.T) {
	before, after := "editor.action.insertLineBefore", "editor.action.insertLineAfter"
	runCases(t, []opCase{
		{"after", after, "a|bc\ndef", "abc\n|\ndef", "", nil},
		{"after keeps indentation", after, "  a|b\nc", "  ab\n  |\nc", "", nil},
		{"after last line", after, "ab|", "ab\n|", "", nil},
		{"before", before, "abc\nd|ef", "abc\n|\ndef", "", nil},
		{"before first line", before, "a|bc", "|\nabc", "", nil},
		{"before indents like the line above", before, "  x\ny|", "  x\n  |\ny", "", nil},
		{"after selection's head line", after, "[a\nb]\nc", "a\nb\n|\nc", "", nil},
		{"after two carets on one line", after, "a|b|\nc", "ab\n|\nc", "", nil},
		{"after carets on two lines", after, "a|\nb|", "a\n|\nb\n|", "", nil},
		{"before two carets on the first line", before, "a|b|", "|\nab", "", nil},
	})
}

func TestDuplicateSelection(t *testing.T) {
	dup := "editor.action.duplicateSelection"
	runCases(t, []opCase{
		{"selection", dup, "a[bc]d", "abc[bc]d", "", nil},
		{"reversed selection", dup, "a]bc[d", "abc[bc]d", "", nil},
		{"multi-line selection", dup, "a[b\nc]d", "ab\nc[b\nc]d", "", nil},
		{"caret copies the line", dup, "a|b\nc", "ab\na|b\nc", "", nil},
		{"two carets on one line", dup, "a|b|\nc", "ab\na|b|\nc", "", nil},
		{"two selections", dup, "[a]-[b]", "a[a]-b[b]", "", nil},
	})
}

func TestTranspose(t *testing.T) {
	tr := "editor.action.transpose"
	runCases(t, []opCase{
		{"middle", tr, "ab|cd", "acb|d", "", nil},
		{"line start", tr, "|abc", "a|bc", "", nil},
		{"line end", tr, "ab|\ncd", "a\nb|cd", "", nil},
		{"end of document", tr, "ab|", "ab|", "", nil},
		{"empty line", tr, "|\nx", "\n|x", "", nil},
		{"selection left alone", tr, "[ab]cd", "[ab]cd", "", nil},
		{"wide characters", tr, "a你|好b", "a好你|b", "", nil},
		{"two carets", tr, "ab|cd ef|gh", "acb|d egf|h", "", nil},
	})
}

func TestSortLines(t *testing.T) {
	asc, desc := "editor.action.sortLinesAscending", "editor.action.sortLinesDescending"
	runCases(t, []opCase{
		{"whole document", asc, "c|\na\nb", "a|\nb\nc", "", nil},
		{"two selections", asc, "[b\na]\nx\n[d\nc]", "[a\nb]\nx\n[c\nd]", "", nil},
		{"one selection sorted already", asc, "[b\na]\nx\n[c\nd]", "[b\na]\nx\n[c\nd]", "", nil},
		{"whole document sorts", asc, "c\nb|\na", "a\nb|\nc", "", nil},
		{"selection", asc, "z\n[c\nb\na]\ny", "z\n[a\nb\nc]\ny", "", nil},
		{"descending", desc, "[a\nc\nb]", "[c\nb\na]", "", nil},
		{"case", asc, "[b\nB\na\nA]", "[a\nA\nb\nB]", "", nil},
		{"already sorted", asc, "[a\nb]", "[a\nb]", "", nil},
		{"selection ending at column 0", asc, "[b\na\n]c", "[a\nb\n]c", "", nil},
		{"one line selected", asc, "[ab]\nc", "[ab]\nc", "", nil},
	})
}

func TestTrimTrailingWhitespace(t *testing.T) {
	trim := "editor.action.trimTrailingWhitespace"
	runCases(t, []opCase{
		{"lines", trim, "a  \n\tb\t \n   \nc|", "a\n\tb\n\nc|", "", nil},
		{"cursor in the trimmed blanks", trim, "ab  | \nc", "ab|\nc", "", nil},
		{"nothing to trim", trim, "a\nb|", "a\nb|", "", nil},
		{"carets on several lines", trim, "a |\nb|  ", "a|\nb|", "", nil},
		// Blanks that start inside a string or regex token are part of it
		// and stay (standalone Monaco: trimTrailingWhitespaceInRegexAndStrings
		// undefined); the lines around are trimmed.
		{"go raw string", trim, "s := `a  \nb`  |", "s := `a  \nb`|", "a.go", nil},
		{"js template string", trim, "x = `a  \nb`;  \ny|", "x = `a  \nb`;\ny|", "a.js", nil},
		{"python triple-quoted string", trim, "s = '''a  \nb'''  |", "s = '''a  \nb'''|", "a.py", nil},
		// A JavaScript regex cannot end a line inside it; CoffeeScript's
		// block regexes can.
		{"coffeescript block regex", trim, "r = ///a  \nb///  |", "r = ///a  \nb///|", "a.coffee", nil},
		{"after a js regex", trim, "x = /a /.test(s)  |", "x = /a /.test(s)|", "a.js", nil},
		{"plain text has no strings", trim, "`a  \nb|", "`a\nb|", "a.txt", nil},
	})
}

// TestLineOpsMultiCursorOneUndo: every command runs on every cursor as one
// undo step that restores the text and every cursor.
func TestLineOpsMultiCursorOneUndo(t *testing.T) {
	cmds := []string{
		"editor.action.copyLinesUpAction", "editor.action.copyLinesDownAction",
		"editor.action.moveLinesUpAction", "editor.action.moveLinesDownAction",
		"editor.action.deleteLines", "editor.action.joinLines",
		"editor.action.insertLineBefore", "editor.action.insertLineAfter",
		"editor.action.duplicateSelection", "editor.action.transpose",
		"editor.action.trimTrailingWhitespace",
		"editor.action.indentLines", "editor.action.outdentLines", "tab", "outdent",
		"editor.action.commentLine", "editor.action.addCommentLine", "editor.action.blockComment",
		"editor.action.indentationToTabs",
	}
	const doc = "package a  \n\n    func b() {\n// x\n\tc := 1\n  }\nz\n"
	for _, id := range cmds {
		t.Run(id, func(t *testing.T) {
			st := NewState(doc, "a.go", nil)
			snap := st.Snapshot()
			// Two cursors on line 2, one on line 4, a selection over 5-6.
			sels := []Selection{
				{Anchor: snap.LineStart(2) + 5, Head: snap.LineStart(2) + 5},
				{Anchor: snap.LineStart(2) + 9, Head: snap.LineStart(2) + 9},
				{Anchor: snap.LineStart(4) + 2, Head: snap.LineStart(4) + 2},
				{Anchor: snap.LineStart(5) + 1, Head: snap.LineStart(6) + 1},
			}
			st.SetSelections(sels)
			want := slices.Clone(st.Sels)
			if !st.Execute(id, nil) {
				t.Fatalf("%s did nothing", id)
			}
			if st.Snapshot().String() == doc {
				t.Fatalf("%s did not change the text", id)
			}
			if !st.Undo() {
				t.Fatal("undo failed")
			}
			if got := st.Snapshot().String(); got != doc {
				t.Errorf("after one undo: %q", got)
			}
			wantSels(t, st, "after undo", want...)
			if st.Undo() {
				t.Errorf("%s took more than one undo step", id)
			}
			st.Redo()
			if st.Snapshot().String() == doc {
				t.Error("redo did nothing")
			}
		})
	}
}

// TestLineOpsReadOnly: the editing commands are refused while read-only.
func TestLineOpsReadOnly(t *testing.T) {
	st := NewState("a\nb", "a.txt", nil)
	st.Options.ReadOnly = true
	for _, id := range []string{"editor.action.copyLinesDownAction", "editor.action.deleteLines", "editor.action.commentLine", "tab", "outdent"} {
		if st.Execute(id, nil) {
			t.Errorf("%s ran in a read-only editor", id)
		}
	}
	if st.Snapshot().String() != "a\nb" {
		t.Errorf("read-only text changed: %q", st.Snapshot().String())
	}
}
