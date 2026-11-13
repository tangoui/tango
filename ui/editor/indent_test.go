package editor

import (
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/input"
)

func useTabs(st *State) { st.Model.SetInsertSpaces(false) }

func TestTabAndOutdent(t *testing.T) {
	runCases(t, []opCase{
		// No selection: spaces (or a tab) to the next tab stop.
		{"tab caret", "tab", "ab|", "ab  |", "", nil},
		{"tab caret with tabs", "tab", "ab|", "ab\t|", "", useTabs},
		{"tab caret in indentation", "tab", "  |x", "    |x", "", nil},
		// A selection inside one line is replaced by the spaces.
		{"tab part of a line", "tab", "a[bc]d", "a   |d", "", nil},
		// A whole line, or several, is indented; a selection starting in
		// the indentation keeps its start column.
		{"tab whole line", "tab", "[abc]\nd", "[    abc]\nd", "", nil},
		{"tab lines", "tab", "a[b\nc]d\ne", "    a[b\n    c]d\ne", "", nil},
		{"tab lines skip empty", "tab", "[a\n\nb]", "[    a\n\n    b]", "", nil},
		{"tab lines with tabs", "tab", "[a\nb]", "[\ta\n\tb]", "", useTabs},
		{"tab lines to the next stop", "tab", "[  a\n      b]", "[    a\n        b]", "", nil},
		{"tab selection ending at column 0", "tab", "[a\n]b", "[    a\n]b", "", nil},
		{"tab mixed caret and lines", "tab", "x|\n[a\nb]", "x   |\n[    a\n    b]", "", nil},

		// Shift+Tab outdents the lines of every cursor, a caret too.
		{"outdent caret", "outdent", "    a|b", "a|b", "", nil},
		{"outdent to the previous stop", "outdent", "      a|b", "    a|b", "", nil},
		{"outdent mixed indentation to spaces", "outdent", "\t  a|", "    a|", "", nil},
		{"outdent mixed indentation to tabs", "outdent", "\t  a|", "\ta|", "", useTabs},
		{"outdent spaces then tab", "outdent", "  \ta|", "a|", "", nil},
		{"outdent tabs", "outdent", "\t\ta|", "\ta|", "", useTabs},
		{"outdent short indentation", "outdent", "  a|", "a|", "", nil},
		{"outdent no indentation", "outdent", "a|b", "a|b", "", nil},
		{"outdent empty line", "outdent", "|\nx", "|\nx", "", nil},
		{"outdent blank line", "outdent", "      |", "    |", "", nil},
		{"outdent caret inside the indentation", "outdent", "  |  a", "|a", "", nil},
		{"outdent lines", "outdent", "[    a\n\tb\nc]", "[a\nb\nc]", "", nil},
		{"outdent selection inside the text", "outdent", "    a[b]c", "a[b]c", "", nil},

		// indentLines / outdentLines (Cmd+] / Cmd+[) shift a caret's line.
		{"indent caret", "editor.action.indentLines", "a|b", "    a|b", "", nil},
		{"indent empty line", "editor.action.indentLines", "|\nx", "    |\nx", "", nil},
		{"indent blank line", "editor.action.indentLines", "  |\nx", "    |\nx", "", nil},
		{"outdent lines command", "editor.action.outdentLines", "  a|", "a|", "", nil},
		// Cursors on one line shift it once.
		{"indent two carets on one line", "editor.action.indentLines", "  a|b|", "    a|b|", "", nil},
		{"outdent caret and selection on one line", "outdent", "        a|b[c]", "    a|b[c]", "", nil},
	})
}

func TestReindentLines(t *testing.T) {
	re := "editor.action.reindentlines"
	runCases(t, []opCase{
		{"ruby", re, "def a\nb|\nif c\nd\nend\nend", "def a\n    b|\n    if c\n        d\n    end\nend", "a.rb", nil},
		{"ruby already indented", re, "def a|\n    b\nend", "def a|\n    b\nend", "a.rb", nil},
		{"ruby with tabs", re, "def a\nb|\nend", "def a\n\tb|\nend", "a.rb", useTabs},
		{"ruby caret in an unchanged indentation", re, "def a\n  |  b\nend", "def a\n  |  b\nend", "a.rb", nil},
		{"ruby caret in the indentation", re, "def a\n        |b\nend", "def a\n    |b\nend", "a.rb", nil},
		// Brackets inside comments (and strings) do not count: the "{" in
		// the comment would otherwise indent the next line.
		{"proto comment bracket", re, "message A {\n/* { */\nx|\n}", "message A {\n    /* { */\n    x|\n}", "a.proto", nil},
		// A language without indentation rules does nothing.
		{"go", re, "func a() {\nb|\n}", "func a() {\nb|\n}", "a.go", nil},
	})
}

func TestIndentationConvert(t *testing.T) {
	spaces, tabs := "editor.action.indentationToSpaces", "editor.action.indentationToTabs"
	runCases(t, []opCase{
		{"to spaces", spaces, "\ta\n  \tb|\nc", "    a\n      b|\nc", "", useTabs},
		{"to tabs", tabs, "        a\n  b\n    c|\n      ", "\t\ta\n  b\n\tc|\n\t  ", "", nil},
	})
}

// TestIndentationOptionsShared: the tab size and insertSpaces are the
// Model's (Monaco's model options): converting the indentation in one view
// of a split changes how the other view's Tab indents.
func TestIndentationOptionsShared(t *testing.T) {
	m := NewModel("a.txt", "\tx\ny", nil)
	m.SetInsertSpaces(false)
	a, b := NewView(m, nil), NewView(m, nil)
	defer a.Close()
	defer b.Close()
	if !a.Execute("editor.action.indentationToSpaces", nil) || m.Snapshot().String() != "    x\ny" {
		t.Fatalf("toSpaces: %q", m.Snapshot().String())
	}
	if !m.InsertSpaces() {
		t.Fatal("toSpaces did not set the Model's InsertSpaces")
	}
	b.SetSelections(carets(m.Snapshot().Len())) // after "y"
	b.Execute("tab", nil)
	if got := m.Snapshot().String(); got != "    x\ny   " {
		t.Errorf("view B's Tab after toSpaces: %q", got)
	}
	a.Execute("editor.action.indentationToTabs", nil)
	if m.InsertSpaces() {
		t.Error("toTabs did not clear the Model's InsertSpaces")
	}
	b.SetSelections(carets(0))
	b.Execute("tab", nil)
	if got := m.Snapshot().String(); !strings.HasPrefix(got, "\t\tx") {
		t.Errorf("view B's Tab after toTabs: %q", got)
	}
	// The tab size is shared too; a View's TabSize modifier is display only.
	m.SetTabSize(2)
	if a.tabSize() != 2 || b.indentSize() != 2 {
		t.Errorf("tab size: %d %d", a.tabSize(), b.indentSize())
	}
	b.frameTab = 8
	if b.tabSize() != 8 || b.indentSize() != 2 {
		t.Errorf("frame override: display %d, indent %d", b.tabSize(), b.indentSize())
	}
	m.SetTabSize(0)
	if m.TabSize() != DefaultTabSize {
		t.Errorf("SetTabSize(0): %d", m.TabSize())
	}
}

// TestLineOpsKeys runs the new commands through the default macOS keys in
// a focused view.
func TestLineOpsKeys(t *testing.T) {
	goText(t)
	st := NewState("a\n  b\nc", "a.go", theme.Base("vs"))
	ed := newEditDriver(st, 400, 200, pinMac)
	ed.focus(t)
	text := func() string { return st.Snapshot().String() }
	at := func(off int) { st.SetSelections(carets(off)) }
	alt, shift, meta, ctrl := input.ModAlt, input.ModShift, input.ModMeta, input.ModCtrl
	steps := []struct {
		name  string
		caret int
		key   input.Key
		mods  input.Modifiers
		want  string
	}{
		{"Alt+Down", 0, input.KeyDown, alt, "  b\na\nc"},
		{"Alt+Up", 4, input.KeyUp, alt, "a\n  b\nc"},
		{"Shift+Alt+Down", 0, input.KeyDown, shift | alt, "a\na\n  b\nc"},
		{"Shift+Cmd+K", 0, input.KeyK, shift | meta, "a\n  b\nc"},
		{"Shift+Tab", 4, input.KeyTab, shift, "a\nb\nc"},
		{"Cmd+]", 2, input.KeyBracketRight, meta, "a\n    b\nc"},
		{"Cmd+[", 2, input.KeyBracketLeft, meta, "a\nb\nc"},
		{"Cmd+/", 0, input.KeySlash, meta, "// a\nb\nc"},
		{"Cmd+/ again", 0, input.KeySlash, meta, "a\nb\nc"},
		{"Ctrl+J", 0, input.KeyJ, ctrl, "a b\nc"},
		{"Cmd+Enter", 0, input.KeyEnter, meta, "a b\n\nc"},
		{"Shift+Alt+A", 0, input.KeyA, shift | alt, "/*  */a b\n\nc"},
	}
	for _, s := range steps {
		at(s.caret)
		ed.Key(s.key, s.mods)
		if got := text(); got != s.want {
			t.Errorf("%s: %q, want %q", s.name, got, s.want)
		}
	}
	// A chord: Cmd+K Cmd+C adds a line comment, Cmd+K Cmd+U removes it.
	at(0)
	ed.Key(input.KeyK, meta)
	ed.Key(input.KeyC, meta)
	if got := text(); got != "// /*  */a b\n\nc" {
		t.Errorf("Cmd+K Cmd+C: %q", got)
	}
	ed.Key(input.KeyK, meta)
	ed.Key(input.KeyU, meta)
	if got := text(); got != "/*  */a b\n\nc" {
		t.Errorf("Cmd+K Cmd+U: %q", got)
	}
}
