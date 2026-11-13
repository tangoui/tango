package editor

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor/keymap"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// m1Commands are the commands behind milestone 1's fixed key map.
var m1Commands = []string{
	"cursorLeft", "cursorLeftSelect", "cursorRight", "cursorRightSelect",
	"cursorUp", "cursorUpSelect", "cursorDown", "cursorDownSelect",
	"cursorPageUp", "cursorPageUpSelect", "cursorPageDown", "cursorPageDownSelect",
	"cursorHome", "cursorHomeSelect", "cursorEnd", "cursorEndSelect",
	"cursorTop", "cursorTopSelect", "cursorBottom", "cursorBottomSelect",
	"cursorWordLeft", "cursorWordLeftSelect", "cursorWordEndRight", "cursorWordEndRightSelect",
	"editor.action.selectAll",
	"deleteLeft", "deleteRight", "deleteWordLeft", "deleteWordRight", "deleteAllLeft", "deleteAllRight",
	"type", "tab", "editor.action.toggleTabFocusMode", "undo", "redo",
	"editor.action.clipboardCopyAction", "editor.action.clipboardCutAction", "editor.action.clipboardPasteAction",
}

// recorder registers a test command counting its runs.
func recorder(id string) *int {
	n := new(int)
	RegisterCommand(Command{ID: id, Label: "Test " + id, Run: func(*State, any) bool { *n++; return true }})
	return n
}

func TestCommandsListed(t *testing.T) {
	cs := Commands()
	ids := map[string]bool{}
	for i, c := range cs {
		if c.Label == "" {
			t.Errorf("command %s has no Label", c.ID)
		}
		if ids[c.ID] {
			t.Errorf("duplicate ID %s", c.ID)
		}
		ids[c.ID] = true
		if i > 0 && cs[i-1].ID >= c.ID {
			t.Errorf("not sorted: %s before %s", cs[i-1].ID, c.ID)
		}
	}
	for _, id := range m1Commands {
		if !ids[id] {
			t.Errorf("milestone 1 command %s not registered", id)
		}
	}
	// Every default binding names a registered command.
	for _, p := range []keymap.Platform{keymap.Mac, keymap.Windows, keymap.Linux} {
		for _, r := range keymap.DefaultRules(p) {
			if !ids[r.Command] {
				t.Errorf("%v default %q → %s: not a registered command", p, r.Key, r.Command)
			}
		}
	}
	if _, ok := LookupCommand("cursorLeft"); !ok {
		t.Error("LookupCommand(cursorLeft) failed")
	}
	if _, ok := LookupCommand("no.such.command"); ok {
		t.Error("LookupCommand found an unknown command")
	}
	for _, bad := range []Command{{Label: "x", Run: func(*State, any) bool { return true }}, {ID: "x"}, {ID: "x", Run: func(*State, any) bool { return true }, Precondition: "a &&"}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("RegisterCommand(%+v) did not panic", bad)
				}
			}()
			RegisterCommand(bad)
		}()
	}
}

// pcKey translates a milestone 1 key (macOS layout of the table in
// TestKeyboardNavigation, Cmd as the shortcut) to Monaco's Windows/Linux
// binding of the same command.
func pcKey(k input.Key, m input.Modifiers) (input.Key, input.Modifiers) {
	shift := m & input.ModShift
	switch {
	case m&input.ModAlt != 0 && (k == input.KeyLeft || k == input.KeyRight):
		return k, input.ModCtrl | shift // word
	case m&input.ModMeta != 0 && k == input.KeyLeft:
		return input.KeyHome, shift
	case m&input.ModMeta != 0 && k == input.KeyRight:
		return input.KeyEnd, shift
	case m&input.ModMeta != 0 && k == input.KeyUp:
		return input.KeyHome, input.ModCtrl | shift
	case m&input.ModMeta != 0 && k == input.KeyDown:
		return input.KeyEnd, input.ModCtrl | shift
	case m&input.ModMeta != 0:
		return k, m&^input.ModMeta | input.ModCtrl
	}
	return k, m
}

// TestM1KeysStillWork runs every entry of milestone 1's
// TestKeyboardNavigation table through the keymap on each platform: as is
// with the macOS keymap, and translated to Monaco's Windows/Linux keys
// (pcKey) with theirs.
func TestM1KeysStillWork(t *testing.T) {
	goText(t)
	doc := "hello world foo\n  indented line\nx\nab你好cd\n" + manyLines(60)
	snap0 := NewState(doc, "a.txt", nil).Snapshot()
	l1, l2, l3 := snap0.LineStart(1), snap0.LineStart(2), snap0.LineStart(3)
	meta := input.ModMeta
	cases := []struct {
		name  string
		start Selection
		key   input.Key
		mods  input.Modifiers
		want  Selection
	}{
		{"right", sel(0, 0), input.KeyRight, 0, sel(1, 1)},
		{"left", sel(3, 3), input.KeyLeft, 0, sel(2, 2)},
		{"left at start", sel(0, 0), input.KeyLeft, 0, sel(0, 0)},
		{"left crosses line", sel(l1, l1), input.KeyLeft, 0, sel(l1-1, l1-1)},
		{"shift right", sel(0, 0), input.KeyRight, input.ModShift, sel(0, 1)},
		{"right collapses", sel(2, 5), input.KeyRight, 0, sel(5, 5)},
		{"left collapses", sel(5, 2), input.KeyLeft, 0, sel(2, 2)},
		{"alt right word", sel(0, 0), input.KeyRight, input.ModAlt, sel(5, 5)},
		{"alt right next word", sel(5, 5), input.KeyRight, input.ModAlt, sel(11, 11)},
		{"alt left word", sel(11, 11), input.KeyLeft, input.ModAlt, sel(6, 6)},
		{"shift alt left", sel(11, 11), input.KeyLeft, input.ModAlt | input.ModShift, sel(11, 6)},
		{"shortcut right line end", sel(3, 3), input.KeyRight, meta, sel(15, 15)},
		{"shortcut left first non-blank", sel(l1+8, l1+8), input.KeyLeft, meta, sel(l1+2, l1+2)},
		{"home first non-blank", sel(l1+8, l1+8), input.KeyHome, 0, sel(l1+2, l1+2)},
		{"home again column 0", sel(l1+2, l1+2), input.KeyHome, 0, sel(l1, l1)},
		{"end", sel(l1, l1), input.KeyEnd, 0, sel(l1+15, l1+15)},
		{"shift end", sel(l1, l1), input.KeyEnd, input.ModShift, sel(l1, l1+15)},
		{"down", sel(3, 3), input.KeyDown, 0, sel(l1+3, l1+3)},
		{"up", sel(l1+3, l1+3), input.KeyUp, 0, sel(3, 3)},
		{"up on first line", sel(3, 3), input.KeyUp, 0, sel(0, 0)},
		{"down short line clamps", sel(l1+10, l1+10), input.KeyDown, 0, sel(l2+1, l2+1)},
		{"shift down", sel(0, 0), input.KeyDown, input.ModShift, sel(0, l1)},
		{"select all", sel(4, 4), input.KeyA, meta, sel(0, snap0.Len())},
		{"shortcut down doc end", sel(4, 4), input.KeyDown, meta, sel(snap0.Len(), snap0.Len())},
		{"shortcut up doc start", sel(l3, l3), input.KeyUp, meta, sel(0, 0)},
		{"page down", sel(0, 0), input.KeyPageDown, 0, sel(-1, -1)}, // checked below
	}
	for _, p := range []keymap.Platform{keymap.Mac, keymap.Windows, keymap.Linux} {
		km := keymap.New(p, keymap.DefaultRules(p))
		for _, c := range cases {
			t.Run(p.String()+"/"+c.name, func(t *testing.T) {
				st := NewState(doc, "a.txt", theme.Base("vs"))
				ed := newEditDriver(st, 500, 300, func(w *ViewW) { w.Keymap(km) })
				ed.focus(t)
				st.MoveCaret(c.start.Anchor, false)
				st.MoveCaret(c.start.Head, true)
				ed.Frame()
				k, m := c.key, c.mods
				if p != keymap.Mac {
					k, m = pcKey(k, m)
				}
				ed.Key(k, m)
				want := c.want
				if c.key == input.KeyPageDown {
					n := st.pageLines()
					want = sel(snap0.LineStart(n), snap0.LineStart(n))
				}
				if st.Primary() != want {
					t.Errorf("%v %v: Sel = %+v, want %+v", k, m, st.Primary(), want)
				}
			})
		}
	}

	// Editing keys of TestTypingAndUndo / TestTabFocusMode per platform.
	for _, p := range []keymap.Platform{keymap.Mac, keymap.Windows, keymap.Linux} {
		t.Run(p.String()+"/editing", func(t *testing.T) {
			km := keymap.New(p, keymap.DefaultRules(p))
			sc, word := input.ModMeta, input.ModAlt
			if p != keymap.Mac {
				sc, word = input.ModCtrl, input.ModCtrl
			}
			st := NewState("", "a.go", theme.Base("vs"))
			ed := newEditDriver(st, 500, 300, func(w *ViewW) { w.Keymap(km) })
			ed.focus(t)
			text := func() string { return st.Snapshot().String() }
			st.Model.SetInsertSpaces(false) // Enter normalises the indentation to tabs
			st.Insert("\tfoo")
			ed.Key(input.KeyEnter, 0)
			if text() != "\tfoo\n\t" {
				t.Fatalf("Enter: %q", text())
			}
			st.Model.SetInsertSpaces(true)
			ed.Key(input.KeyBackspace, 0)
			st.Insert("alpha beta ")
			ed.Key(input.KeyBackspace, word)
			if text() != "\tfoo\nalpha " {
				t.Fatalf("word delete left: %q", text())
			}
			ed.Key(input.KeyZ, sc)
			if text() != "\tfoo\nalpha beta " {
				t.Fatalf("undo: %q", text())
			}
			if p == keymap.Mac {
				ed.Key(input.KeyZ, sc|input.ModShift)
			} else {
				ed.Key(input.KeyY, input.ModCtrl)
			}
			if text() != "\tfoo\nalpha " {
				t.Fatalf("redo: %q", text())
			}
			ed.Key(input.KeyA, sc)
			ed.Key(input.KeyC, sc)
			if ed.Clipboard() != "\tfoo\nalpha " {
				t.Errorf("copy: %q", ed.Clipboard())
			}
			ed.Key(input.KeyX, sc)
			if text() != "" {
				t.Errorf("cut: %q", text())
			}
			ed.Key(input.KeyV, sc)
			if text() != "\tfoo\nalpha " {
				t.Errorf("paste: %q", text())
			}
			ed.Key(input.KeyTab, 0)
			if text() != "\tfoo\nalpha   " {
				t.Errorf("tab: %q", text())
			}
			tm := input.ModCtrl
			if p == keymap.Mac {
				tm |= input.ModShift // Monaco's macOS binding
			}
			ed.Key(input.KeyM, tm)
			if !st.tabFocus {
				t.Errorf("toggleTabFocusMode via %v did not toggle", tm)
			}
		})
	}
}

func TestIMEComposingSkipsKeymap(t *testing.T) {
	goText(t)
	n := recorder("test.imeRecord")
	km := keymap.New(keymap.Mac, keymap.DefaultRules(keymap.Mac), []keymap.Rule{
		{Key: "cmd+d", Command: "test.imeRecord"},
		{Key: "cmd+k cmd+i", Command: "test.imeRecord"},
	})
	st := NewState("hello", "a.txt", theme.Base("vs"))
	ed := newEditDriver(st, 500, 300, func(w *ViewW) { w.Keymap(km) })
	ed.focus(t)
	st.MoveCaret(5, false)
	ed.Frame()

	ed.Compose("ni", 2)
	ed.Key(input.KeyD, input.ModMeta) // conflicts with a shortcut
	ed.Key(input.KeyBackspace, 0)     // the IME edits its composition
	ed.Key(input.KeyLeft, 0)          // and moves in it
	ed.Key(input.KeyK, input.ModMeta) // no chord starts
	if *n != 0 {
		t.Fatalf("a command ran while composing (%d)", *n)
	}
	if km.Pending() {
		t.Fatal("a chord started while composing")
	}
	if got := (*ed.vw).vs.comp; got != "ni" {
		t.Fatalf("composition interrupted: %q", got)
	}
	if st.Snapshot().String() != "hello" || st.Primary() != sel(5, 5) {
		t.Fatalf("keys reached the text while composing: %q sel %+v", st.Snapshot().String(), st.Primary())
	}
	// Committing ends the composition; shortcuts work again.
	ed.Type("你")
	if st.Snapshot().String() != "hello你" {
		t.Fatalf("commit: %q", st.Snapshot().String())
	}
	ed.Key(input.KeyD, input.ModMeta)
	if *n != 1 {
		t.Errorf("shortcut after the commit ran %d times", *n)
	}
	// A composition cancelled (empty) also ends it.
	ed.Compose("a", 1)
	ed.Key(input.KeyD, input.ModMeta)
	ed.Compose("", 0)
	ed.Key(input.KeyD, input.ModMeta)
	if *n != 2 {
		t.Errorf("after a cancelled composition: %d runs, want 2", *n)
	}
	// Composition and a key in the same frame, in order: the key before
	// the composition starts is a shortcut, the one after is not.
	ed.Inject(input.KeyEvent{Key: input.KeyD, Down: true, Mods: input.ModMeta})
	ed.Inject(input.Composition{Text: "x", Cursor: 1})
	ed.Inject(input.KeyEvent{Key: input.KeyD, Down: true, Mods: input.ModMeta})
	ed.Settle()
	if *n != 3 {
		t.Errorf("same-frame order: %d runs, want 3", *n)
	}
}

func TestChordStateClearedOnBlur(t *testing.T) {
	goText(t)
	n := recorder("test.chordRecord")
	km := keymap.New(keymap.Mac, keymap.DefaultRules(keymap.Mac), []keymap.Rule{
		{Key: "cmd+k cmd+d", Command: "test.chordRecord"},
		{Key: "cmd+k c", Command: "test.chordRecord"},
	})
	st := NewState("ab", "a.txt", theme.Base("vs"))
	var vw *ViewW
	var other *ui.Box
	d := ui.NewDriver(func(c *ui.Context) {
		ui.Column(c, func() {
			other = ui.Div(c, layout.H(20), layout.W(100))
			other.Focusable(input.CursorDefault)
			vw = View(c, st).Keymap(km)
		}, layout.Grow(1))
	}, ui.DriverOptions{Width: 400, Height: 200})
	d.Frame()
	d.Frame()
	ed := &editDriver{Driver: d, vw: &vw}
	ed.focus(t)

	// The chord works, with no timeout.
	ed.Key(input.KeyK, input.ModMeta)
	if !km.Pending() {
		t.Fatal("cmd+k: no pending chord")
	}
	ed.Advance(10 * time.Second)
	ed.Key(input.KeyD, input.ModMeta)
	if *n != 1 || km.Pending() {
		t.Fatalf("cmd+k cmd+d: runs %d pending %v", *n, km.Pending())
	}
	// A plain key ending a chord, Windows-style events (WM_KEYDOWN, then
	// WM_CHAR): the key resolves the chord and its text is dropped. The
	// macOS / Linux sequence (text only) is TestChordPlatformEvents.
	ed.Key(input.KeyK, input.ModMeta)
	ed.Inject(input.KeyEvent{Key: input.KeyC, Down: true})
	ed.Inject(input.TextInput{Text: "c"})
	ed.Inject(input.KeyEvent{Key: input.KeyC})
	ed.Settle()
	if *n != 2 || st.Snapshot().String() != "ab" {
		t.Fatalf("cmd+k c: runs %d text %q", *n, st.Snapshot().String())
	}
	// A key cancelling a chord is swallowed with its text, too
	// (Windows-style).
	ed.Key(input.KeyK, input.ModMeta)
	ed.Inject(input.KeyEvent{Key: input.KeyX, Down: true})
	ed.Inject(input.TextInput{Text: "x"})
	ed.Settle()
	if st.Snapshot().String() != "ab" || km.Pending() {
		t.Fatalf("cancelling key: text %q pending %v", st.Snapshot().String(), km.Pending())
	}

	// Focus moves to another widget after the first chord.
	ed.Key(input.KeyK, input.ModMeta)
	if !km.Pending() {
		t.Fatal("no pending chord")
	}
	g := other.Rect()
	ed.Click(geom.Pt(g.X+2, g.Y+2))
	if vw.Focused() {
		t.Fatal("view still focused")
	}
	if km.Pending() {
		t.Fatal("chord state survived losing focus")
	}
	ed.focus(t)
	ed.Key(input.KeyD, input.ModMeta)
	if *n != 2 {
		t.Errorf("second chord after refocus ran the command (%d)", *n)
	}

	// The window deactivates after the first chord.
	ed.Key(input.KeyK, input.ModMeta)
	ed.Inject(input.WindowFocus{Focused: false})
	ed.Settle()
	if km.Pending() {
		t.Fatal("chord state survived window deactivation")
	}
	ed.Inject(input.WindowFocus{Focused: true})
	ed.Settle()
	ed.Key(input.KeyD, input.ModMeta)
	if *n != 2 {
		t.Errorf("second chord after reactivation ran the command (%d)", *n)
	}
}

func TestExecuteAndContextKeys(t *testing.T) {
	goText(t)
	st := NewState("hello world\n", "a.go", theme.Base("vs"))
	readOnly := false
	var host map[string]any
	km := keymap.New(keymap.Linux, keymap.DefaultRules(keymap.Linux), []keymap.Rule{
		{Key: "ctrl+q", Command: "test.ctxRecord", When: "myPanel && editorLangId == go && isLinux"},
	})
	n := recorder("test.ctxRecord")
	ed := newEditDriver(st, 500, 300, func(w *ViewW) {
		w.Keymap(km).ReadOnly(readOnly).Context(func(k string) (any, bool) {
			v, ok := host[k]
			return v, ok
		})
	})
	ed.focus(t)

	ed.Key(input.KeyQ, input.ModCtrl)
	if *n != 0 {
		t.Fatal("rule ran without the host key")
	}
	host = map[string]any{"myPanel": true}
	ed.Key(input.KeyQ, input.ModCtrl)
	if *n != 1 {
		t.Fatalf("rule with the host key: %d runs", *n)
	}

	ck := st.contextKeys()
	check := func(key string, want any) {
		t.Helper()
		if v, ok := ck.Value(key); !ok || v != want {
			t.Errorf("%s = %v (%v), want %v", key, v, ok, want)
		}
	}
	check(CtxEditorFocus, true)
	check(CtxEditorTextFocus, true)
	check(CtxTextInputFocus, true)
	check(CtxEditorReadonly, false)
	check(CtxEditorHasSelection, false)
	check(CtxEditorHasMultipleSelections, false)
	check(CtxEditorLangID, "go")
	check(CtxEditorTabMovesFocus, false)
	check(CtxIsLinux, true)
	check(CtxIsMac, false)
	check(CtxIsWindows, false)
	st.MoveCaret(3, true)
	check(CtxEditorHasSelection, true)
	if _, ok := ck.Value("unknown"); ok {
		t.Error("unknown key defined")
	}
	// The host function may override a built-in key.
	host[CtxEditorLangID] = "rust"
	check(CtxEditorLangID, "rust")
	delete(host, CtxEditorLangID)

	// Execute: unknown commands, Mutates and preconditions in a read-only
	// view; cut still copies.
	if st.Execute("no.such.command", nil) {
		t.Error("unknown command ran")
	}
	readOnly = true
	ed.Frame()
	check(CtxEditorReadonly, true)
	st.MoveCaret(0, false)
	st.MoveCaret(5, true)
	before := st.Snapshot().String()
	for _, id := range []string{"deleteLeft", "type", "tab", "undo", "editor.action.clipboardPasteAction"} {
		if st.Execute(id, "x") {
			t.Errorf("%s ran in a read-only view", id)
		}
	}
	if !st.Execute("editor.action.clipboardCutAction", nil) || ed.Clipboard() != "hello" {
		t.Errorf("cut in read-only: clipboard %q", ed.Clipboard())
	}
	if st.Snapshot().String() != before {
		t.Errorf("read-only text changed: %q", st.Snapshot().String())
	}
	if !st.Execute("cursorEnd", nil) || st.Primary() != sel(11, 11) {
		t.Errorf("navigation in read-only: %+v", st.Primary())
	}
	readOnly = false
	ed.Frame()
	if !st.Execute("type", TypeArgs{Text: "!"}) || st.Snapshot().Line(0) != "hello world!" {
		t.Errorf("type: %q", st.Snapshot().Line(0))
	}
	if !st.Execute("type", map[string]any{"text": "?"}) || st.Snapshot().Line(0) != "hello world!?" {
		t.Errorf("type map args: %q", st.Snapshot().Line(0))
	}
	if st.Execute("type", nil) {
		t.Error("type without text ran")
	}

	// A State never shown falls back to its Options and the column model.
	bare := NewState("a\nb\n", "a.txt", nil)
	if !bare.Execute("cursorDown", nil) || bare.Primary() != sel(2, 2) {
		t.Errorf("bare cursorDown: %+v", bare.Primary())
	}
	bare.Options.ReadOnly = true
	if bare.Execute("deleteRight", nil) {
		t.Error("bare read-only deleteRight ran")
	}
	if bare.Execute("editor.action.clipboardCopyAction", nil) {
		t.Error("copy without a view reported success")
	}
	if v, _ := bare.contextKeys().Value(CtxEditorFocus); v != false {
		t.Errorf("bare editorFocus = %v", v)
	}
}

// TestDefaultKeymapPerView: views without a Keymap get their own clone of
// the default keymap (a chord in one does not leak into another).
func TestDefaultKeymapPerView(t *testing.T) {
	goText(t)
	a := NewState("a", "a.txt", nil)
	b := NewState("b", "b.txt", nil)
	var wa, wb *ViewW
	d := ui.NewDriver(func(c *ui.Context) {
		ui.Column(c, func() {
			wa = View(c, a)
			wb = View(c, b)
		}, layout.Grow(1))
	}, ui.DriverOptions{Width: 400, Height: 400})
	d.Frame()
	ka, kb := wa.keymap(), wb.keymap()
	if ka == kb || ka == defaultKeymap() {
		t.Fatal("views share the default keymap")
	}
	if wa.keymap() != ka {
		t.Error("default keymap not retained")
	}
	if ka.Platform() != keymap.Current() || len(ka.Errors()) != 0 {
		t.Errorf("default keymap: platform %v errors %v", ka.Platform(), ka.Errors())
	}
}

// TestChordPlatformEvents: the event sequences macOS and Linux deliver. A
// key that produces text arrives as TextInput only (no KeyEvent): macOS
// sends it through interpretKeyEvents, GTK through its input-method
// filter; Cmd/Ctrl chords and navigation keys still arrive as KeyEvents
// (macOS: Code is never filled, so the Runner copies Key).
func TestChordPlatformEvents(t *testing.T) {
	goText(t)
	n := recorder("test.platformRecord")
	for _, p := range []keymap.Platform{keymap.Mac, keymap.Linux} {
		t.Run(p.String(), func(t *testing.T) {
			*n = 0
			km := keymap.New(p, keymap.DefaultRules(p), []keymap.Rule{
				{Key: "mod+k c", Command: "test.platformRecord"},
				{Key: "mod+k shift+/", Command: "test.platformRecord", Args: "help"},
				{Key: "shift shift", Command: "test.platformRecord"},
			})
			mod := input.ModMeta
			if p != keymap.Mac {
				mod = input.ModCtrl
			}
			st := NewState("ab", "a.txt", theme.Base("vs"))
			ed := newEditDriver(st, 400, 200, func(w *ViewW) { w.Keymap(km) })
			ed.focus(t)
			st.MoveCaret(2, false)
			ed.Frame()
			text := func() string { return st.Snapshot().String() }
			keyOnly := func(k input.Key, m input.Modifiers) { // no Code: as macOS
				ed.Inject(input.KeyEvent{Key: k, Down: true, Mods: m})
				ed.Inject(input.KeyEvent{Key: k, Mods: m})
			}

			// mod+k, then "c" as text only: the chord completes, nothing is
			// typed, and the next arrow key is not swallowed.
			keyOnly(input.KeyK, mod)
			ed.Settle()
			if !km.Pending() {
				t.Fatal("mod+k: not pending")
			}
			ed.Inject(input.TextInput{Text: "c"})
			ed.Settle()
			if *n != 1 || text() != "ab" || km.Pending() {
				t.Fatalf("text-only c: runs %d text %q pending %v", *n, text(), km.Pending())
			}
			keyOnly(input.KeyLeft, 0)
			ed.Settle()
			if st.Primary() != sel(1, 1) {
				t.Errorf("arrow after the chord: %+v", st.Primary())
			}
			// Shifted character: "?" is shift+/.
			keyOnly(input.KeyK, mod)
			ed.Inject(input.TextInput{Text: "?"})
			ed.Settle()
			if *n != 2 || text() != "ab" {
				t.Errorf("text-only ?: runs %d text %q", *n, text())
			}
			// A character that maps to no key cancels the chord and is dropped.
			keyOnly(input.KeyK, mod)
			ed.Inject(input.TextInput{Text: "é"})
			ed.Settle()
			if km.Pending() || text() != "ab" || *n != 2 {
				t.Errorf("text-only é: pending %v text %q runs %d", km.Pending(), text(), *n)
			}
			// A mismatching character cancels too; typing works afterwards.
			keyOnly(input.KeyK, mod)
			ed.Inject(input.TextInput{Text: "z"})
			ed.Inject(input.TextInput{Text: "y"})
			ed.Settle()
			if km.Pending() || text() != "ayb" {
				t.Errorf("mismatch then typing: pending %v text %q", km.Pending(), text())
			}
			// Shift held for a capital letter (flagsChanged press/release
			// around text-only "A"), twice: not a Shift double tap.
			shiftTap := func(typed string) {
				ed.Inject(input.KeyEvent{Key: input.KeyShiftLeft, Down: true, Mods: input.ModShift})
				if typed != "" {
					ed.Inject(input.TextInput{Text: typed})
				}
				ed.Inject(input.KeyEvent{Key: input.KeyShiftLeft})
				ed.Settle()
			}
			before := *n
			shiftTap("A")
			shiftTap("B")
			if *n != before {
				t.Errorf("capital letters fired the Shift double tap")
			}
			shiftTap("")
			shiftTap("")
			if *n != before+1 {
				t.Errorf("Shift double tap: %d runs, want %d", *n, before+1)
			}
		})
	}
}

// TestPreconditionInKeybinding: a key whose command is unavailable (a
// Mutates command in a read-only view) is not consumed and falls through
// to a lower-priority binding, as Monaco folds preconditions into the
// keybinding's when.
func TestPreconditionInKeybinding(t *testing.T) {
	goText(t)
	n := recorder("test.lowPriority")
	km := keymap.New(keymap.Mac, []keymap.Rule{
		{Key: "cmd+z", Command: "test.lowPriority"},
		{Key: "cmd+z", Command: "undo"},
		{Key: "backspace", Command: "test.lowPriority"},
		{Key: "backspace", Command: "deleteLeft"},
	})
	readOnly := true
	st := NewState("ab", "a.txt", theme.Base("vs"))
	ed := newEditDriver(st, 400, 200, func(w *ViewW) { w.Keymap(km).ReadOnly(readOnly) })
	ed.focus(t)
	st.MoveCaret(2, false)
	ed.Frame()
	ed.Key(input.KeyZ, input.ModMeta)
	ed.Key(input.KeyBackspace, 0)
	if *n != 2 || st.Snapshot().String() != "ab" {
		t.Errorf("read-only: lower-priority runs %d text %q", *n, st.Snapshot().String())
	}
	readOnly = false
	ed.Frame()
	ed.Key(input.KeyBackspace, 0)
	if *n != 2 || st.Snapshot().String() != "a" {
		t.Errorf("writable: lower-priority runs %d text %q", *n, st.Snapshot().String())
	}
}
