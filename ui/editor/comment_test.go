package editor

import (
	"testing"

	"github.com/tangoui/tango/ui/editor/theme"
)

func TestLineComment(t *testing.T) {
	toggle, add, remove := "editor.action.commentLine", "editor.action.addCommentLine", "editor.action.removeCommentLine"
	noIgnore := func(st *State) { st.Options.Comments.IgnoreEmptyLines = false }
	noSpace := func(st *State) { st.Options.Comments.InsertSpace = false }
	runCases(t, []opCase{
		{"comment a line", toggle, "some |text\nmore", "// some |text\nmore", "a.go", nil},
		{"uncomment a line", toggle, "// some |text", "some |text", "a.go", nil},
		{"uncomment without the space", toggle, "//some |text", "some |text", "a.go", nil},
		{"uncomment keeps a second space", toggle, "//  a|", " a|", "a.go", nil},
		{"indented line", toggle, "\tsome text|", "\t// some text|", "a.go", nil},
		{"no space", toggle, "a|", "//a|", "a.go", noSpace},
		{"uncomment no space", toggle, "// a|", " a|", "a.go", noSpace},
		// Several lines: the tokens line up at the smallest indentation,
		// rounded down to a tab stop.
		{"lines aligned", toggle, "[    a\n        b]", "[    // a\n    //     b]", "a.go", nil},
		{"lines aligned to a tab stop", toggle, "[  a\n      b]", "// [  a\n//       b]", "a.go", nil},
		{"lines with tabs", toggle, "[\ta\n\t\tb]", "[\t// a\n\t// \tb]", "a.go", nil},
		{"uncomment lines", toggle, "[// a\n  // b]", "[a\n  b]", "a.go", nil},
		{"mixed lines get commented", toggle, "[// a\nb]", "// [// a\n// b]", "a.go", nil},
		// Empty and whitespace-only lines are left alone among others...
		{"empty line among others", toggle, "[a\n\nb]", "// [a\n\n// b]", "a.go", nil},
		{"blank line among others", toggle, "[a\n   \nb]", "// [a\n   \n// b]", "a.go", nil},
		{"uncomment around a blank line", toggle, "[// a\n   \n// b]", "[a\n   \nb]", "a.go", nil},
		{"empty line not ignored", toggle, "[a\n\nb]", "// [a\n// \n// b]", "a.go", noIgnore},
		// ...but a caret on one gets a comment, after its blanks.
		{"empty line alone", toggle, "|", "// |", "a.go", nil},
		{"blank line alone", toggle, "    |", "    // |", "a.go", nil},
		// The insertion point is rounded down to a tab stop (4).
		{"short blank line alone", toggle, "  |", "//   |", "a.go", nil},
		{"blank lines alone", toggle, "[    \n      ]", "[    // \n    //   ]", "a.go", nil},
		{"selection ending at column 0", toggle, "[a\n]b", "// [a\n]b", "a.go", nil},
		{"case-insensitive token", toggle, "rem echo|", "echo|", "a.bat", nil},
		{"add on a comment", add, "// a|", "// // a|", "a.go", nil},
		{"add on mixed lines", add, "[// a\nb]", "// [// a\n// b]", "a.go", nil},
		{"remove on mixed lines", remove, "[// a\nb]", "[a\nb]", "a.go", nil},
		{"remove without comments", remove, "a|", "a|", "a.go", nil},
		// Cursors on one line comment it once; a selection starting on the
		// line another ends on leaves that line to it.
		{"two carets on one line", toggle, "a|b|\nc", "// a|b|\nc", "a.go", nil},
		{"carets on two lines", toggle, "a|\nb|", "// a|\n// b|", "a.go", nil},
		{"selections sharing a line", toggle, "[a\nb]c[d\ne]", "// [a\n// b]c[d\n// e]", "a.go", nil},
		{"chained selections", toggle, "[a\nb][b\nc\nd][d\ne]", "// [a\n// b][b\n// c\n// d][d\n// e]", "a.go", nil},
		{"plain text has no comments", toggle, "a|", "a|", "a.txt", nil},
	})
}

// TestLineCommentLaterCaretGivesLineUp exercises the one-line-owner
// exception of the line dedupe: a caret added later (index 1) on line 0
// sorts before an earlier-added selection (index 0) that starts later on
// line 0 and runs into line 1. The caret, a one-line selection added later
// than the one after it, gives line 0 up, so the selection toggles both
// lines as one ("// a" and "b" are mixed: both get commented). Without the
// exception the caret would uncomment line 0 alone ("a\n// b"). The
// selections cannot be written as marked text (markers list selections in
// document order), so they are set directly.
func TestLineCommentLaterCaretGivesLineUp(t *testing.T) {
	st := NewState("// a\nb", "a.go", theme.Base("vs"))
	st.SetSelections([]Selection{{Anchor: 3, Head: 6}, {Anchor: 1, Head: 1}})
	if !st.Execute("editor.action.commentLine", nil) {
		t.Fatal("commentLine did nothing")
	}
	if got := st.Snapshot().String(); got != "// // a\n// b" {
		t.Errorf("got %q, want %q", got, "// // a\n// b")
	}
}

func TestBlockComment(t *testing.T) {
	block := "editor.action.blockComment"
	runCases(t, []opCase{
		{"wrap selection", block, "a[bc]d", "a/* [bc] */d", "a.go", nil},
		{"unwrap selection", block, "a/* [bc] */d", "a[bc]d", "a.go", nil},
		{"unwrap selection with the tokens", block, "a[/* bc */]d", "a[bc]d", "a.go", nil},
		{"caret", block, "a|b", "a/* | */b", "a.go", nil},
		{"caret in an empty comment", block, "a/* | */b", "a|b", "a.go", nil},
		{"lines", block, "[a\nb]", "/* [a\nb] */", "a.go", nil},
		{"unwrap lines", block, "/* [a\nb] */", "[a\nb]", "a.go", nil},
		{"no space", block, "[ab]", "/*[ab]*/", "a.go", func(st *State) { st.Options.Comments.InsertSpace = false }},
		{"end token inside", block, "/* [a */ b]", "/* /* [a */ b] */", "a.go", nil},
		{"two selections", block, "[a] [b]", "/* [a] */ /* [b] */", "a.go", nil},
		{"html", block, "<p>[x]</p>", "<p><!-- [x] --></p>", "a.html", nil},
		{"plain text", block, "[a]", "[a]", "a.txt", nil},
	})
}

// TestCommentToggleLanguages toggles comments in three comment styles:
// Go's //, Python's # and HTML's block comment (HTML has no line comment,
// so the line command wraps the line in <!-- -->).
func TestCommentToggleLanguages(t *testing.T) {
	toggle := "editor.action.commentLine"
	// A selection starting where the token goes does not grow over it
	// (Monaco tracks selections with NeverGrowsWhenTypingAtEdges).
	runCases(t, []opCase{
		{"go", toggle, "func a() {\n\t[x := 1\n\ty := 2]\n}", "func a() {\n\t// [x := 1\n\t// y := 2]\n}", "a.go", nil},
		{"go uncomment", toggle, "\t[// x := 1\n\t// y := 2]", "\t[x := 1\n\ty := 2]", "a.go", nil},
		{"python", toggle, "def f():\n    [x = 1\n    y = 2]", "def f():\n    # [x = 1\n    # y = 2]", "a.py", nil},
		{"python uncomment", toggle, "#  x|", " x|", "a.py", nil},
		{"html caret", toggle, "  <p>|</p>", "  <!-- <p>|</p> -->", "a.html", nil},
		{"html uncomment caret", toggle, "  <!-- <p>|</p> -->", "  <p>|</p>", "a.html", nil},
		{"html empty line", toggle, "|", "<!-- | -->", "a.html", nil},
		{"html lines", toggle, "[<a>\n</a>]", "<!-- [<a>\n</a>] -->", "a.html", nil},
		{"html uncomment lines", toggle, "<!-- [<a>\n</a>] -->", "[<a>\n</a>]", "a.html", nil},
		{"css", toggle, "a { b|: c }", "/* a { b|: c } */", "a.css", nil},
	})
}
