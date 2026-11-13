package editor

import (
	"testing"

	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/input"
)

const (
	cmdCopy  = "editor.action.clipboardCopyAction"
	cmdCut   = "editor.action.clipboardCutAction"
	cmdPaste = "editor.action.clipboardPasteAction"
)

func TestCopyCutEmptySelectionLine(t *testing.T) {
	goText(t)
	st := NewState("one\ntwo\nthree", "a.txt", theme.Base("vs"))
	ed := newEditDriver(st, 500, 300, pinMac)
	ed.focus(t)
	text := func() string { return st.Snapshot().String() }
	run := func(id string) {
		t.Helper()
		ed.Frame()
		if !st.Execute(id, nil) {
			t.Fatalf("%s did nothing", id)
		}
		ed.Frame()
	}

	// Copy with a caret copies its line with the line break.
	st.SetSelections(carets(5)) // t|wo
	run(cmdCopy)
	if ed.Clipboard() != "two\n" {
		t.Fatalf("copy line: %q", ed.Clipboard())
	}
	// Pasting it with a caret inserts it above the caret's line; the caret
	// stays where it was in the text.
	st.SetSelections(carets(10)) // th|ree
	run(cmdPaste)
	if got := text(); got != "one\ntwo\ntwo\nthree" {
		t.Fatalf("paste line: %q", got)
	}
	wantSels(t, st, "paste line", sel(14, 14))
	st.Undo()
	if got := text(); got != "one\ntwo\nthree" {
		t.Fatalf("undo paste line: %q", got)
	}
	// On an empty line too.
	setDoc(st, "a\n\nb")
	st.SetSelections(carets(2))
	run(cmdPaste)
	if got := text(); got != "a\ntwo\n\nb" {
		t.Fatalf("paste line on an empty line: %q", got)
	}
	wantSels(t, st, "paste line on an empty line", sel(6, 6))
	setDoc(st, "one\ntwo\nthree")
	// Over a selection it replaces the selection.
	st.SetSelections([]Selection{sel(0, 3)})
	run(cmdPaste)
	if got := text(); got != "two\n\ntwo\nthree" {
		t.Fatalf("paste line over a selection: %q", got)
	}
	st.Undo()
	// Other text (not what the editor copied) pastes at the caret.
	ed.SetClipboard("zz\n")
	st.SetSelections(carets(5))
	run(cmdPaste)
	if got := text(); got != "one\ntzz\nwo\nthree" {
		t.Fatalf("paste of foreign text: %q", got)
	}
	st.Undo()
	// ... and the remembered copy is forgotten.
	ed.SetClipboard("two\n")
	run(cmdPaste)
	if got := text(); got != "one\nttwo\nwo\nthree" {
		t.Fatalf("paste after the metadata was forgotten: %q", got)
	}
	st.Undo()

	// Cut with a caret cuts its line with the line break; the caret goes
	// to the start of the next line.
	st.SetSelections(carets(1))
	run(cmdCut)
	if text() != "two\nthree" || ed.Clipboard() != "one\n" {
		t.Fatalf("cut line: %q clipboard %q", text(), ed.Clipboard())
	}
	wantSels(t, st, "cut line", sel(0, 0))
	// On the last line, the line break before it.
	st.SetSelections(carets(6)) // th|ree
	run(cmdCut)
	if text() != "two" || ed.Clipboard() != "three\n" {
		t.Fatalf("cut last line: %q clipboard %q", text(), ed.Clipboard())
	}
	wantSels(t, st, "cut last line", sel(3, 3))
	// The only line: its text.
	run(cmdCut)
	if text() != "" || ed.Clipboard() != "two\n" {
		t.Fatalf("cut the only line: %q clipboard %q", text(), ed.Clipboard())
	}
	// Paste it back above the (empty) line.
	run(cmdPaste)
	if text() != "two\n" {
		t.Fatalf("paste cut line: %q", text())
	}
	st.Undo()
	st.Undo()
	st.Undo()
	st.Undo()
	if got := text(); got != "one\ntwo\nthree" {
		t.Fatalf("undo cuts: %q", got)
	}

	// Several carets: one line each, once per line, in document order,
	// joined with line breaks (Monaco's getPlainTextToCopy).
	st.SetSelections(carets(10, 1, 2))
	run(cmdCopy)
	if ed.Clipboard() != "one\n\nthree\n" {
		t.Fatalf("copy lines of several carets: %q", ed.Clipboard())
	}
	// Pasted into as many cursors, each gets its piece.
	st.SetSelections(carets(4, 0))
	run(cmdPaste)
	if got := text(); got != "one\none\nthree\ntwo\nthree" {
		t.Fatalf("paste pieces: %q", got)
	}
	st.Undo()
	// Cut with several carets on the last lines.
	st.SetSelections(carets(10, 5))
	run(cmdCut)
	if got := text(); got != "one\n" {
		t.Fatalf("cut two last lines: %q", got)
	}
	st.Undo()

	// The document's line ending is used.
	setDoc(st, "ab\ncd")
	st.Model.SetEOL("\r\n")
	st.SetSelections(carets(1))
	run(cmdCopy)
	if ed.Clipboard() != "ab\r\n" {
		t.Fatalf("copy line with CRLF: %q", ed.Clipboard())
	}
	st.SetSelections(carets(4))
	run(cmdPaste)
	if got := text(); got != "ab\nab\ncd" {
		t.Fatalf("paste line with CRLF: %q", got)
	}

	// A whole line copied from a CRLF document pastes above the caret's
	// line of an LF document.
	st.SetSelections(carets(0))
	run(cmdCopy)
	if ed.Clipboard() != "ab\r\n" {
		t.Fatalf("copy from CRLF: %q", ed.Clipboard())
	}
	setDoc(st, "x\ny")
	st.Model.SetEOL("\n")
	st.SetSelections(carets(3))
	run(cmdPaste)
	if got := text(); got != "x\nab\ny" {
		t.Fatalf("CRLF line into LF document: %q", got)
	}
	wantSels(t, st, "CRLF line into LF document", sel(6, 6))

	// emptySelectionClipboard off: a caret copies and cuts nothing.
	st.Options.EmptySelectionClipboard = false
	ed.SetClipboard("keep")
	st.SetSelections(carets(1))
	ed.Frame()
	if st.Execute(cmdCopy, nil) || st.Execute(cmdCut, nil) || ed.Clipboard() != "keep" {
		t.Errorf("caret copy/cut with emptySelectionClipboard off: clipboard %q", ed.Clipboard())
	}
	// ... nor does an empty primary with a selection elsewhere (Monaco).
	setDoc(st, "one\ntwo")
	st.SetSelections([]Selection{sel(1, 1), sel(4, 7)})
	ed.Frame()
	if st.Execute(cmdCopy, nil) || st.Execute(cmdCut, nil) || ed.Clipboard() != "keep" || text() != "one\ntwo" {
		t.Errorf("empty primary with emptySelectionClipboard off: clipboard %q text %q", ed.Clipboard(), text())
	}
	st.Options.EmptySelectionClipboard = true

	// Through the keys.
	setDoc(st, "one\ntwo\nthree")
	st.SetSelections(carets(0))
	ed.Frame()
	ed.Key(input.KeyX, input.ModMeta)
	ed.Key(input.KeyDown, 0)
	ed.Key(input.KeyV, input.ModMeta)
	if got := text(); got != "two\none\nthree" {
		t.Errorf("Cmd+X, Down, Cmd+V: %q", got)
	}
}

// setDoc replaces the whole text (an undo step).
func setDoc(st *State, s string) {
	st.SetSelections([]Selection{sel(0, st.Snapshot().Len())})
	st.Insert(s)
	st.SetSelections(carets(0))
}

func TestMultiCursorPasteSpread(t *testing.T) {
	goText(t)
	st := NewState("a\nb\nc", "a.txt", theme.Base("vs"))
	ed := newEditDriver(st, 500, 300, pinMac)
	ed.focus(t)
	text := func() string { return st.Snapshot().String() }
	run := func(id string) {
		t.Helper()
		ed.Frame()
		if !st.Execute(id, nil) {
			t.Fatalf("%s did nothing", id)
		}
		ed.Frame()
	}

	// As many lines as cursors: one line each, in document order whatever
	// the order of the cursors (the primary last in the text here).
	ed.SetClipboard("1\r\n2\r\n3\r\n")
	st.SetSelections(carets(5, 1, 3))
	run(cmdPaste)
	if got := text(); got != "a1\nb2\nc3" {
		t.Fatalf("spread: %q", got)
	}
	wantSels(t, st, "spread", sel(8, 8), sel(2, 2), sel(5, 5))
	st.Undo()
	// Another line count: the whole text at every cursor.
	ed.SetClipboard("x\ny")
	st.SetSelections(carets(5, 1, 3))
	run(cmdPaste)
	if got := text(); got != "ax\ny\nbx\ny\ncx\ny" {
		t.Fatalf("no spread: %q", got)
	}
	st.Undo()
	// multiCursorPaste "full": no spread.
	st.Options.MultiCursorPaste = MultiCursorPasteFull
	ed.SetClipboard("1\n2\n3")
	st.SetSelections(carets(1, 3, 5))
	run(cmdPaste)
	if got := text(); got != "a1\n2\n3\nb1\n2\n3\nc1\n2\n3" {
		t.Fatalf("full: %q", got)
	}
	st.Undo()
	st.Options.MultiCursorPaste = MultiCursorPasteSpread

	// A copy of several selections pastes piece by piece into as many
	// cursors, even when a piece has several lines.
	setDoc(st, "ab\ncd\nef")
	st.SetSelections([]Selection{sel(7, 8), sel(1, 4)}) // "f", "b\nc"
	run(cmdCopy)
	if ed.Clipboard() != "b\nc\nf" {
		t.Fatalf("copy of two selections: %q", ed.Clipboard())
	}
	st.SetSelections(carets(8, 0))
	run(cmdPaste)
	if got := text(); got != "b\ncab\ncd\neff" {
		t.Fatalf("paste of the pieces: %q", got)
	}
	st.Undo()
	// Into another number of cursors: lines spread when they match...
	st.SetSelections(carets(0, 3, 6))
	run(cmdPaste)
	if got := text(); got != "bab\nccd\nfef" {
		t.Fatalf("pieces into three cursors: %q", got)
	}
	st.Undo()
	// ... and a single cursor gets the whole text.
	st.SetSelections(carets(0))
	run(cmdPaste)
	if got := text(); got != "b\nc\nfab\ncd\nef" {
		t.Fatalf("pieces into one cursor: %q", got)
	}
	st.Undo()

	// A whole line copied from a caret goes above each caret's line, not
	// spread.
	st.SetSelections(carets(0))
	run(cmdCopy)
	st.SetSelections(carets(3, 6))
	run(cmdPaste)
	if got := text(); got != "ab\nab\ncd\nab\nef" {
		t.Fatalf("whole line into two carets: %q", got)
	}
	wantSels(t, st, "whole line into two carets", sel(6, 6), sel(12, 12))
}

func TestReadOnlyRejectsAllMutations(t *testing.T) {
	goText(t)
	doc := "func f() {\n\treturn 1 // x\n}\n\nfoo bar foo\n"
	st := NewState(doc, "a.go", theme.Base("vs"))
	ed := newEditDriver(st, 500, 300, func(w *ViewW) { pinMac(w); w.ReadOnly(true) })
	ed.focus(t)
	if v, _ := st.contextKeys().Value(CtxEditorReadonly); v != true {
		t.Fatalf("editorReadonly = %v in a read-only view", v)
	}
	setups := map[string][]Selection{
		"caret":     carets(14),
		"selection": {sel(12, 20)},
		"multi":     {sel(4, 6), sel(30, 33), sel(38, 41)},
	}
	args := []any{nil, "x", TypeArgs{Text: "\n"}, map[string]any{"text": "("}}
	ver := st.Snapshot().Version()
	mutating := 0
	for name, sels := range setups {
		for _, c := range Commands() {
			if c.Mutates {
				mutating++
			}
			for _, a := range args {
				ed.SetClipboard("pasted\n")
				st.SetSelections(sels)
				ed.Frame()
				ran := st.Execute(c.ID, a)
				if c.Mutates && ran {
					t.Errorf("%s: Mutates command %s ran in a read-only view", name, c.ID)
				}
				if got := st.Snapshot().String(); got != doc || st.Snapshot().Version() != ver {
					t.Fatalf("%s: %s (args %v) changed read-only text: %q", name, c.ID, a, got)
				}
			}
		}
	}
	if mutating == 0 {
		t.Fatal("no Mutates commands")
	}
	// Copy and cut still copy; navigation works.
	st.SetSelections([]Selection{sel(0, 4)})
	ed.Frame()
	ed.SetClipboard("")
	ed.Key(input.KeyX, input.ModMeta)
	if ed.Clipboard() != "func" || st.Snapshot().String() != doc {
		t.Errorf("cut in read-only: clipboard %q text %q", ed.Clipboard(), st.Snapshot().String())
	}
	st.SetSelections(carets(0))
	ed.Frame()
	ed.Key(input.KeyC, input.ModMeta)
	if ed.Clipboard() != "func f() {\n" {
		t.Errorf("caret copy in read-only: %q", ed.Clipboard())
	}
	ed.Key(input.KeyDown, 0)
	if st.Primary().Head != 11 {
		t.Errorf("navigation in read-only: %+v", st.Primary())
	}
	// Without a view, Options.ReadOnly refuses them too.
	st2 := NewState(doc, "a.go", theme.Base("vs"))
	st2.Options.ReadOnly = true
	for _, c := range Commands() {
		if c.Mutates && st2.Execute(c.ID, "x") {
			t.Errorf("Mutates command %s ran with Options.ReadOnly", c.ID)
		}
	}
	if st2.Snapshot().String() != doc {
		t.Errorf("Options.ReadOnly text changed: %q", st2.Snapshot().String())
	}
}
