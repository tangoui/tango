package editor

import (
	"math"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor/keymap"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// short is the platform shortcut modifier.
var short = func() input.Modifiers {
	if runtime.GOOS == "darwin" {
		return input.ModMeta
	}
	return input.ModCtrl
}()

// editDriver is viewDriver that also counts the frames whose View reported
// Changed.
type editDriver struct {
	*ui.Driver
	vw      **ViewW
	changes int
}

// macKeymap is the macOS default keymap; pinMac makes a view use it, so
// milestone 1's key tables (written for macOS keys) pass on every OS.
// TestM1KeysStillWork checks the Windows and Linux bindings.
var macKeymap = keymap.New(keymap.Mac, keymap.DefaultRules(keymap.Mac))

func pinMac(w *ViewW) { w.Keymap(macKeymap) }

func newEditDriver(st *State, w, h float32, mods func(*ViewW)) *editDriver {
	ed := &editDriver{}
	var vw *ViewW
	ed.vw = &vw
	ed.Driver = ui.NewDriver(func(c *ui.Context) {
		vw = View(c, st)
		if mods != nil {
			mods(vw)
		}
		if vw.Changed() {
			ed.changes++
		}
	}, ui.DriverOptions{Width: w, Height: h})
	ed.Frame()
	ed.Frame()
	return ed
}

// at returns the window point of the caret before byte column col of line
// (vertically centred in the line), from the View's own layout.
func (ed *editDriver) at(t *testing.T, st *State, line, col int) geom.Point {
	t.Helper()
	w := *ed.vw
	x, ok := w.contentX(st.Snapshot(), st.Snapshot().LineStart(line)+col)
	if !ok {
		t.Fatalf("no caret x for line %d col %d", line, col)
	}
	g := w.vs
	v := st.Viewport
	return geom.Pt(g.text.X-v.ScrollX+x+0.5, g.text.Y-v.OffsetY+float32(line-v.TopLine)*v.LineHeight+v.LineHeight/2)
}

func (ed *editDriver) focus(t *testing.T) {
	t.Helper()
	g := (*ed.vw).vs
	ed.Click(geom.Pt(g.text.X+g.text.W-2, g.text.Y+g.text.H-2))
	ed.Advance(time.Second) // no double click with the next press
	if !(*ed.vw).Focused() {
		t.Fatal("view not focused")
	}
}

func sel(a, h int) Selection { return Selection{Anchor: a, Head: h} }

func TestTypingAndUndo(t *testing.T) {
	goText(t)
	short := input.ModMeta // pinned to the macOS keymap (pinMac) on every OS
	st := NewState("", "a.go", theme.Base("vs"))
	ed := newEditDriver(st, 500, 300, pinMac)
	ed.focus(t)
	text := func() string { return st.Snapshot().String() }

	ed.Type("h")
	ed.Type("i")
	if text() != "hi" || st.Primary() != sel(2, 2) {
		t.Fatalf("typed %q sel %+v", text(), st.Primary())
	}
	if ed.changes != 2 {
		t.Errorf("Changed reported %d times, want 2", ed.changes)
	}
	// Enter keeps the indentation of the line (normalised to the Model's
	// indentation style, as Monaco's Enter: tabs here).
	st.Model.SetInsertSpaces(false)
	st.Insert("\n\tfoo")
	ed.Key(input.KeyEnter, 0)
	if text() != "hi\n\tfoo\n\t" {
		t.Fatalf("after Enter: %q", text())
	}
	st.Model.SetInsertSpaces(true)
	ed.Type("x")
	// Backspace, Delete.
	ed.Key(input.KeyBackspace, 0)
	if text() != "hi\n\tfoo\n\t" {
		t.Fatalf("after Backspace: %q", text())
	}
	ed.Key(input.KeyLeft, 0)
	ed.Key(input.KeyDelete, 0)
	if text() != "hi\n\tfoo\n" {
		t.Fatalf("after Delete: %q", text())
	}
	// Word deletes.
	st.MoveCaret(0, false)
	st.Insert("alpha beta ")
	ed.Key(input.KeyBackspace, input.ModAlt)
	if !strings.HasPrefix(text(), "alpha hi") {
		t.Fatalf("after Alt+Backspace: %q", text())
	}
	ed.Key(input.KeyDelete, input.ModAlt)
	if !strings.HasPrefix(text(), "alpha \n") {
		t.Fatalf("after Alt+Delete: %q", text())
	}
	// Undo / redo.
	before := text()
	ed.Key(input.KeyZ, short)
	if text() == before {
		t.Fatalf("undo did nothing")
	}
	ed.Key(input.KeyZ, short|input.ModShift)
	if text() != before {
		t.Fatalf("redo: %q, want %q", text(), before)
	}
	for st.Undo() {
	}
	if text() != "" {
		t.Fatalf("undo all: %q", text())
	}
	// Typing groups into one undo step.
	ed.Type("a")
	ed.Type("b")
	ed.Type("c")
	ed.Key(input.KeyZ, short)
	if text() != "" {
		t.Errorf("undo after typing abc: %q", text())
	}
	ed.Key(input.KeyZ, short|input.ModShift)

	// Clipboard.
	ed.Key(input.KeyA, short)
	if st.Primary() != sel(0, 3) {
		t.Fatalf("select all: %+v", st.Primary())
	}
	ed.Key(input.KeyC, short)
	if ed.Clipboard() != "abc" {
		t.Errorf("copy: clipboard %q", ed.Clipboard())
	}
	ed.Key(input.KeyX, short)
	if text() != "" || ed.Clipboard() != "abc" {
		t.Errorf("cut: text %q clipboard %q", text(), ed.Clipboard())
	}
	ed.SetClipboard("one\r\ntwo")
	ed.Key(input.KeyV, short)
	if text() != "one\ntwo" || st.Primary() != sel(7, 7) {
		t.Errorf("paste: %q sel %+v", text(), st.Primary())
	}
	ed.Key(input.KeyZ, short)
	if text() != "" {
		t.Errorf("undo paste: %q", text())
	}

	// Tab (captured from the Runner while focused): spaces to the next
	// tab stop, or a tab character. Shift+Tab outdents, and the line has
	// no indentation to remove.
	tab := func() { ed.Key(input.KeyTab, 0) }
	st.Insert("ab")
	tab()
	if text() != "ab  " {
		t.Errorf("Tab with spaces: %q", text())
	}
	st.Model.SetInsertSpaces(false)
	tab()
	if text() != "ab  \t" {
		t.Errorf("Tab character: %q", text())
	}
	ed.Key(input.KeyTab, input.ModShift)
	if text() != "ab  \t" || !(*ed.vw).Focused() {
		t.Errorf("Shift+Tab: text %q focused %v", text(), (*ed.vw).Focused())
	}
}

func TestKeyboardNavigation(t *testing.T) {
	goText(t)
	short := input.ModMeta // pinned to the macOS keymap (pinMac) on every OS
	doc := "hello world foo\n  indented line\nx\nab你好cd\n" + manyLines(60)
	type tc struct {
		name  string
		start Selection
		key   input.Key
		mods  input.Modifiers
		want  Selection
	}
	snap0 := NewState(doc, "a.txt", nil).Snapshot()
	l1, l2, l3 := snap0.LineStart(1), snap0.LineStart(2), snap0.LineStart(3)
	cases := []tc{
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
		{"shortcut right line end", sel(3, 3), input.KeyRight, short, sel(15, 15)},
		{"shortcut left first non-blank", sel(l1+8, l1+8), input.KeyLeft, short, sel(l1+2, l1+2)},
		{"home first non-blank", sel(l1+8, l1+8), input.KeyHome, 0, sel(l1+2, l1+2)},
		{"home again column 0", sel(l1+2, l1+2), input.KeyHome, 0, sel(l1, l1)},
		{"end", sel(l1, l1), input.KeyEnd, 0, sel(l1+15, l1+15)},
		{"shift end", sel(l1, l1), input.KeyEnd, input.ModShift, sel(l1, l1+15)},
		{"down", sel(3, 3), input.KeyDown, 0, sel(l1+3, l1+3)},
		{"up", sel(l1+3, l1+3), input.KeyUp, 0, sel(3, 3)},
		{"up on first line", sel(3, 3), input.KeyUp, 0, sel(0, 0)},
		{"down short line clamps", sel(l1+10, l1+10), input.KeyDown, 0, sel(l2+1, l2+1)},
		{"shift down", sel(0, 0), input.KeyDown, input.ModShift, sel(0, l1)},
		{"select all", sel(4, 4), input.KeyA, short, sel(0, snap0.Len())},
		{"shortcut down doc end", sel(4, 4), input.KeyDown, short, sel(snap0.Len(), snap0.Len())},
		{"shortcut up doc start", sel(l3, l3), input.KeyUp, short, sel(0, 0)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := NewState(doc, "a.txt", theme.Base("vs"))
			ed := newEditDriver(st, 500, 300, pinMac)
			ed.focus(t)
			st.MoveCaret(c.start.Anchor, false)
			st.MoveCaret(c.start.Head, true)
			ed.Frame()
			ed.Key(c.key, c.mods)
			if st.Primary() != c.want {
				t.Errorf("Sel = %+v, want %+v", st.Primary(), c.want)
			}
		})
	}

	t.Run("vertical by layout x", func(t *testing.T) {
		// From line 0 col 5 down onto "ab你好cd" (through the short line
		// "x", which must not lose the goal): the caret lands on the
		// boundary whose layout caret is nearest the start x, not on the
		// one the monospace column model picks (CJK glyphs are not exactly
		// two columns wide), and Up returns to the start.
		st := NewState(doc, "a.txt", theme.Base("vs"))
		ed := newEditDriver(st, 500, 300, pinMac)
		ed.focus(t)
		st.MoveCaret(5, false)
		ed.Frame()
		w := *ed.vw
		x0, _ := w.contentX(st.Snapshot(), 5)
		for range 3 {
			ed.Key(input.KeyDown, 0)
		}
		head := st.Primary().Head
		if st.Snapshot().LineOf(head) != 3 {
			t.Fatalf("down: head %d not on line 3", head)
		}
		best, bd := -1, float32(1e9)
		line := st.Snapshot().Line(3)
		for col := 0; ; col = nextBoundary(line, col) {
			x, _ := w.contentX(st.Snapshot(), l3+col)
			if d := abs32(x - x0); d < bd {
				best, bd = l3+col, d
			}
			if col == len(line) {
				break
			}
		}
		if model := l3 + st.offsetForColumn(line, 5); model == best {
			t.Fatalf("fixture does not tell layout x (%d) from the column model (%d)", best, model)
		}
		if head != best {
			t.Errorf("down: head %d, want %d (nearest layout caret)", head, best)
		}
		for range 3 {
			ed.Key(input.KeyUp, 0)
		}
		if st.Primary().Head != 5 {
			t.Errorf("down x3, up x3: head %d, want 5", st.Primary().Head)
		}
		// From after "ab你" the goal x is past the "x" line.
		st.MoveCaret(l3+5, false)
		ed.Frame()
		ed.Key(input.KeyUp, 0)
		ed.Key(input.KeyDown, 0)
		if st.Primary().Head != l3+5 {
			t.Errorf("up, down from after 你: head %d, want %d", st.Primary().Head, l3+5)
		}
	})

	t.Run("page down scrolls and reveals", func(t *testing.T) {
		st := NewState(doc, "a.txt", theme.Base("vs"))
		ed := newEditDriver(st, 500, 300, pinMac)
		ed.focus(t)
		st.MoveCaret(0, false)
		ed.Frame()
		ed.Key(input.KeyPageDown, 0)
		n := st.pageLines()
		if l := st.Snapshot().LineOf(st.Primary().Head); l != n {
			t.Errorf("page down: line %d, want %d", l, n)
		}
		if st.Viewport.TopLine == 0 {
			t.Errorf("page down did not scroll")
		}
		ed.Key(input.KeyPageUp, 0)
		if l := st.Snapshot().LineOf(st.Primary().Head); l != 0 {
			t.Errorf("page up: line %d", l)
		}
		// Moving the caret off screen with the keyboard reveals it.
		st.MoveCaret(st.Snapshot().Len(), false)
		ed.Key(input.KeyLeft, 0)
		first, last := st.VisibleLines()
		if l := st.Snapshot().LineOf(st.Primary().Head); l < first || l > last {
			t.Errorf("caret line %d not revealed (visible %d..%d)", l, first, last)
		}
	})
}

func nextBoundary(s string, i int) int {
	if i >= len(s) {
		return len(s)
	}
	return nextGrapheme(s, i)
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func TestMouseSelection(t *testing.T) {
	goText(t)
	doc := "hello world foo\n  indented line\nx = alpha_beta + 1\n"
	st := NewState(doc, "a.txt", theme.Base("vs"))
	ed := newEditDriver(st, 500, 300, nil)
	snap := st.Snapshot()
	l1, l2 := snap.LineStart(1), snap.LineStart(2)

	// Single click: caret at the nearest boundary, focus.
	ed.Click(ed.at(t, st, 1, 4))
	if !(*ed.vw).Focused() {
		t.Fatal("click did not focus")
	}
	if st.Primary() != sel(l1+4, l1+4) {
		t.Fatalf("click: %+v, want %d", st.Primary(), l1+4)
	}
	ed.Advance(time.Second)
	// Clicking right of the line end goes to the line end, below the last
	// line to the document end, in the gutter to the line start.
	g := (*ed.vw).vs
	ed.Click(geom.Pt(g.text.X+g.text.W-5, ed.at(t, st, 0, 0).Y))
	ed.Advance(time.Second)
	if st.Primary() != sel(15, 15) {
		t.Errorf("click past line end: %+v", st.Primary())
	}
	ed.Click(geom.Pt(g.text.X+20, g.text.Y+g.text.H-3))
	ed.Advance(time.Second)
	if st.Primary() != sel(snap.Len(), snap.Len()) {
		t.Errorf("click below the last line: %+v", st.Primary())
	}
	ed.Click(geom.Pt(g.gutter.X+3, ed.at(t, st, 1, 0).Y))
	ed.Advance(time.Second)
	if st.Primary() != sel(l1, l1) {
		t.Errorf("click in the gutter: %+v", st.Primary())
	}

	// Drag: anchor at the press, head follows the pointer.
	ed.Drag(ed.at(t, st, 0, 2), ed.at(t, st, 2, 5))
	ed.Advance(time.Second)
	if st.Primary() != sel(2, l2+5) {
		t.Errorf("drag: %+v, want {2 %d}", st.Primary(), l2+5)
	}
	// Drag backwards (from outside the selection: a press on it would
	// drag the text, dnd.go).
	ed.Click(ed.at(t, st, 0, 0))
	ed.Advance(time.Second)
	ed.Drag(ed.at(t, st, 2, 3), ed.at(t, st, 1, 1))
	ed.Advance(time.Second)
	if st.Primary() != sel(l2+3, l1+1) {
		t.Errorf("drag back: %+v", st.Primary())
	}

	// Double click: the word. Triple click: the line with its break.
	p := ed.at(t, st, 2, 7) // inside alpha_beta
	ed.Click(p)
	ed.Click(p)
	if st.Primary() != sel(l2+4, l2+14) {
		t.Errorf("double click: %+v, want {%d %d}", st.Primary(), l2+4, l2+14)
	}
	ed.Click(p)
	if st.Primary() != sel(l2, snap.Len()) {
		t.Errorf("triple click: %+v, want {%d %d}", st.Primary(), l2, snap.Len())
	}
	ed.Advance(time.Second)
	ed.Click(ed.at(t, st, 1, 6))
	ed.Click(ed.at(t, st, 1, 6))
	if st.Primary() != sel(l1+2, l1+10) {
		t.Errorf("double click on line 1: %+v", st.Primary())
	}
	ed.Click(ed.at(t, st, 1, 6))
	if st.Primary() != sel(l1, l2) {
		t.Errorf("triple click on line 1: %+v, want {%d %d}", st.Primary(), l1, l2)
	}
	ed.Advance(time.Second)

	// Double click and drag extends by words.
	ed.Click(ed.at(t, st, 0, 1))
	ed.Inject(input.Pointer{Kind: input.Press, Pos: ed.at(t, st, 0, 1)})
	ed.Frame()
	ed.Inject(input.Pointer{Kind: input.Move, Pos: ed.at(t, st, 0, 8)})
	ed.Frame()
	ed.Inject(input.Pointer{Kind: input.Release, Pos: ed.at(t, st, 0, 8)})
	ed.Settle()
	if st.Primary() != sel(0, 11) {
		t.Errorf("double click drag: %+v, want {0 11}", st.Primary())
	}
	if ed.changes != 0 {
		t.Errorf("mouse selection reported %d changes", ed.changes)
	}

	// The host's click count (system double-click interval) is used.
	ed.Advance(time.Second)
	p = ed.at(t, st, 2, 7)
	ed.Inject(input.Pointer{Kind: input.Move, Pos: p})
	ed.Inject(input.Pointer{Kind: input.Press, Pos: p, Clicks: 2})
	ed.Inject(input.Pointer{Kind: input.Release, Pos: p, Clicks: 2})
	ed.Settle()
	if st.Primary() != sel(l2+4, l2+14) {
		t.Errorf("host double click: %+v", st.Primary())
	}
}

func TestCaretAndHitTestMixedScripts(t *testing.T) {
	goText(t)
	// Tab, CJK, an emoji, a ZWJ emoji sequence, combining marks.
	line := "a\tb你好😀x👩‍💻é̂z"
	st := NewState(line+"\nnext\n", "a.txt", theme.Base("vs"))
	ed := newEditDriver(st, 600, 200, nil)
	ed.focus(t)
	cur := defaultColor(st.Theme, "editorCursor.foreground")
	var bounds []int
	for i := 0; ; i = nextGrapheme(line, i) {
		bounds = append(bounds, i)
		if i >= len(line) {
			break
		}
	}
	prevX := float32(-1)
	for _, off := range bounds {
		st.MoveCaret(off, false)
		img := ed.Frame()
		on, caret := ed.TextInputActive()
		if !on {
			t.Fatal("text input not active while focused")
		}
		w := *ed.vw
		x, ok := w.contentX(st.Snapshot(), off)
		if !ok {
			t.Fatalf("offset %d: no caret x", off)
		}
		g := w.vs
		if want := g.text.X + x; abs32(caret.X-want) > 1 {
			t.Errorf("offset %d: IME caret x %v, layout caret %v", off, caret.X, want)
		}
		if x <= prevX {
			t.Errorf("offset %d: caret x %v not right of previous %v", off, x, prevX)
		}
		prevX = x
		// The caret is drawn there.
		cr := geom.Rect{X: caret.X - 1, Y: caret.Y, W: 4, H: caret.H}
		if n := countColor(img, cr, cur); n < int(caret.H/2) {
			t.Errorf("offset %d: %d caret pixels at x %v", off, n, caret.X)
		}
		// Clicking at the caret x puts the caret back on off.
		st.MoveCaret(0, false)
		ed.Frame()
		ed.Click(geom.Pt(caret.X+0.5, caret.Y+caret.H/2))
		ed.Advance(time.Second)
		if st.Primary().Head != off {
			t.Errorf("click at caret x of %d: head %d", off, st.Primary().Head)
		}
	}
	// Clicks anywhere along the line land on grapheme boundaries.
	w := *ed.vw
	g := w.vs
	isBound := map[int]bool{}
	for _, b := range bounds {
		isBound[b] = true
	}
	endX, _ := w.contentX(st.Snapshot(), len(line))
	for x := float32(0); x < endX+10; x += 1.5 {
		ed.Click(geom.Pt(g.text.X+x, g.text.Y+st.Viewport.LineHeight/2))
		ed.Advance(time.Second)
		if !isBound[st.Primary().Head] {
			t.Fatalf("click at x %v: head %d is not a grapheme boundary", x, st.Primary().Head)
		}
	}
	// Selection rectangles cover the selected glyphs: selecting 你好
	// colors the area between their carets.
	a := strings.Index(line, "你")
	b := a + len("你好")
	st.MoveCaret(a, false)
	st.MoveCaret(b, true)
	img := ed.Frame()
	xa, _ := w.contentX(st.Snapshot(), a)
	xb, _ := w.contentX(st.Snapshot(), b)
	selBg := defaultColor(st.Theme, "editor.selectionBackground")
	mid := geom.Rect{X: g.text.X + (xa+xb)/2 - 1, Y: g.text.Y + 1, W: 2, H: 3}
	if n := countColor(img, mid, selBg); n == 0 {
		t.Errorf("no selection color between the carets of 你好")
	}
	if utf8.RuneCountInString(line) < 10 {
		t.Fatal("bad fixture")
	}
}

func TestIMECompositionLifecycle(t *testing.T) {
	goText(t)
	st := NewState("hello\n"+manyLines(200), "a.txt", theme.Base("vs"))
	var vw *ViewW
	d := ui.NewDriver(func(c *ui.Context) {
		ui.Column(c, func() {
			other := ui.Div(c, layout.H(20), layout.W(100))
			other.Focusable(input.CursorDefault)
			vw = View(c, st)
		}, layout.Grow(1))
	}, ui.DriverOptions{Width: 500, Height: 300})
	d.Frame()
	d.Frame()
	ed := &editDriver{Driver: d, vw: &vw}
	text := func() string { return st.Snapshot().Line(0) }

	ed.Click(ed.at(t, st, 0, 5))
	ed.Advance(time.Second)
	if st.Primary().Head != 5 {
		t.Fatalf("caret %d", st.Primary().Head)
	}
	ed.Type("x")
	_, caret0 := ed.TextInputActive()
	ver := st.Model.Buffer().Snapshot().Version()

	// Composing: shown, not in the buffer; the IME caret follows the
	// composition cursor.
	ed.Compose("ni", 2)
	if text() != "hellox" || st.Model.Buffer().Snapshot().Version() != ver {
		t.Fatalf("composition reached the buffer: %q", text())
	}
	on, caret := ed.TextInputActive()
	if !on || caret.X <= caret0.X+1 {
		t.Errorf("IME caret %v not after the composition (before %v)", caret, caret0)
	}
	if vw.vs.comp != "ni" {
		t.Errorf("composition not shown: %q", vw.vs.comp)
	}
	// Underline under the composition.
	fg := defaultColor(st.Theme, "editor.foreground")
	img := ed.Image()
	ul := geom.Rect{X: caret0.X + 1, Y: caret0.Y + caret0.H - 5, W: caret.X - caret0.X - 2, H: 4}
	if n := countTinted(img, ul, defaultColor(st.Theme, "editor.background"), fg); n < 3 {
		t.Errorf("composition underline: %d pixels", n)
	}

	// Scroll while composing: still composing, buffer untouched, the IME
	// caret stays inside the view.
	ed.Scroll(geom.Pt(250, 150), 30*st.Viewport.LineHeight)
	if st.Viewport.TopLine == 0 {
		t.Fatal("did not scroll")
	}
	ed.Compose("nih", 3)
	on, caret = ed.TextInputActive()
	g := vw.vs
	if !on || caret.Y < g.text.Y || caret.Y+caret.H > g.text.Y+g.text.H+0.5 {
		t.Errorf("IME caret %v outside the text area %v after scrolling", caret, g.text)
	}
	if text() != "hellox" {
		t.Errorf("buffer changed while composing: %q", text())
	}
	ed.Scroll(geom.Pt(250, 150), -1000)
	ed.Compose("你", 1)
	// Commit: one Insert, one undo step of its own (even a single
	// character right after typed text).
	ed.Type("你")
	if text() != "hellox你" || st.Primary().Head != len("hellox你") {
		t.Fatalf("commit: %q sel %+v", text(), st.Primary())
	}
	if vw.vs.comp != "" {
		t.Errorf("composition still shown after commit: %q", vw.vs.comp)
	}
	ed.Key(input.KeyZ, short)
	if text() != "hellox" {
		t.Errorf("undo of the commit: %q, want %q", text(), "hellox")
	}
	ed.Key(input.KeyZ, short)
	if text() != "hello" {
		t.Errorf("second undo: %q", text())
	}

	// Hosts that end the composition (empty pre-edit) before the commit
	// arrives in a later frame: still an IME commit, its own undo step.
	ed.Type("y")
	ed.Compose("ni", 2)
	ed.Compose("", 0)
	ed.Type("你")
	ed.Key(input.KeyZ, short)
	if text() != "helloy" {
		t.Errorf("undo of a commit after an ended composition: %q, want %q", text(), "helloy")
	}
	ed.Key(input.KeyZ, short)

	// Composition, then focus moves away: cancelled, nothing written, the
	// text input is turned off; coming back does not resurrect it.
	ed.Compose("abc", 3)
	ed.Click(geom.Pt(20, 10)) // the other focusable box
	ed.Advance(time.Second)
	if vw.Focused() {
		t.Fatal("view still focused")
	}
	if on, _ := ed.TextInputActive(); on {
		t.Errorf("text input still on after losing focus")
	}
	if text() != "hello" || vw.vs.comp != "" {
		t.Errorf("after blur: text %q composition %q", text(), vw.vs.comp)
	}
	ed.Click(ed.at(t, st, 0, 5))
	ed.Advance(time.Second)
	if vw.vs.comp != "" {
		t.Errorf("stale composition %q shown after refocus", vw.vs.comp)
	}
	ed.Compose("q", 1)
	if vw.vs.comp != "q" {
		t.Errorf("new composition after refocus: %q", vw.vs.comp)
	}
	ed.Type("q")
	if text() != "helloq" {
		t.Errorf("commit after refocus: %q", text())
	}
}

func TestReadOnlyRejectsEdits(t *testing.T) {
	goText(t)
	st := NewState("hello world\nline two\n", "a.txt", theme.Base("vs"))
	ed := newEditDriver(st, 500, 300, func(w *ViewW) { w.ReadOnly(true) })
	ed.focus(t)
	want := st.Snapshot().String()
	ver := st.Snapshot().Version()
	st.MoveCaret(5, false)
	ed.Frame()
	ed.Type("x")
	ed.Key(input.KeyBackspace, 0)
	ed.Key(input.KeyDelete, 0)
	ed.Key(input.KeyEnter, 0)
	ed.SetClipboard("pasted")
	ed.Key(input.KeyV, short)
	ed.Compose("ni", 2)
	ed.Type("你")
	(*ed.vw).vs.pend.events = append((*ed.vw).vs.pend.events, input.KeyEvent{Key: input.KeyTab, Down: true})
	ed.Frame()
	ed.Key(input.KeyA, short)
	ed.Key(input.KeyX, short)
	ed.Key(input.KeyZ, short)
	if got := st.Snapshot().String(); got != want || st.Snapshot().Version() != ver {
		t.Errorf("read-only text changed: %q", got)
	}
	if ed.changes != 0 {
		t.Errorf("Changed reported %d times", ed.changes)
	}
	// Navigation, selection and copy still work.
	if st.Primary() != sel(0, len(want)) {
		t.Errorf("select all in read-only: %+v", st.Primary())
	}
	if ed.Clipboard() != want {
		t.Errorf("cut in read-only should copy: clipboard %q", ed.Clipboard())
	}
	ed.Key(input.KeyRight, 0)
	if st.Primary().Head != len(want) {
		t.Errorf("navigation in read-only: %+v", st.Primary())
	}
}

func TestCaretBlink(t *testing.T) {
	goText(t)
	st := NewState("abc\n", "a.txt", theme.Base("vs"))
	ed := newEditDriver(st, 300, 100, nil)
	if !ed.Runner().NextWake().IsZero() {
		t.Errorf("unfocused view asks for a wake-up")
	}
	ed.focus(t)
	cur := defaultColor(st.Theme, "editorCursor.foreground")
	caretPx := func() int {
		img := ed.Frame()
		_, r := ed.TextInputActive()
		return countColor(img, geom.Rect{X: r.X - 1, Y: r.Y, W: r.W + 2, H: r.H}, cur)
	}
	ed.Key(input.KeyRight, 0) // input: caret visible, blink restarts
	if caretPx() == 0 {
		t.Fatal("caret not visible after input")
	}
	wake := ed.Runner().NextWake()
	if dt := wake.Sub(ed.Now()); dt <= 0 || dt > 530*time.Millisecond {
		t.Errorf("next wake in %v, want within 530ms", dt)
	}
	ed.Advance(600 * time.Millisecond)
	if caretPx() != 0 {
		t.Errorf("caret visible in the off phase")
	}
	ed.Advance(500 * time.Millisecond)
	if caretPx() == 0 {
		t.Errorf("caret hidden in the next on phase")
	}
	ed.Advance(600 * time.Millisecond)
	if caretPx() != 0 {
		t.Fatalf("caret visible in the off phase")
	}
	ed.Type("z") // any input shows the caret again
	if caretPx() == 0 {
		t.Errorf("caret hidden right after typing")
	}
	// No timer frames while the caret line is scrolled out of view.
	st.Insert(strings.Repeat("\n", 30))
	st.MoveCaret(0, false)
	ed.Frame()
	ed.Scroll(geom.Pt(150, 50), 20*st.Viewport.LineHeight)
	ed.Frame()
	if st.Viewport.TopLine == 0 {
		t.Fatal("did not scroll")
	}
	if w := ed.Runner().NextWake(); !w.IsZero() {
		t.Errorf("caret off screen: wake-up requested at %v", w)
	}
}

func TestTabFocusMode(t *testing.T) {
	goText(t)
	st := NewState("ab", "a.txt", theme.Base("vs"))
	var vw *ViewW
	readOnly := false
	d := ui.NewDriver(func(c *ui.Context) {
		ui.Column(c, func() {
			other := ui.Div(c, layout.H(20), layout.W(100))
			other.Focusable(input.CursorDefault)
			vw = View(c, st).ReadOnly(readOnly)
		}, layout.Grow(1))
	}, ui.DriverOptions{Width: 400, Height: 200})
	d.Frame()
	d.Frame()
	ed := &editDriver{Driver: d, vw: &vw}
	ed.focus(t)
	text := func() string { return st.Snapshot().String() }

	// Default: Tab indents and keeps focus.
	ed.Key(input.KeyTab, 0)
	if text() != "ab  " || !vw.Focused() {
		t.Fatalf("Tab: text %q focused %v", text(), vw.Focused())
	}
	// Ctrl+Tab still moves focus (no keyboard trap), and so does Esc, Tab.
	ed.Key(input.KeyTab, input.ModCtrl)
	if vw.Focused() || text() != "ab  " {
		t.Errorf("Ctrl+Tab: text %q focused %v", text(), vw.Focused())
	}
	ed.focus(t)
	ed.Key(input.KeyEscape, 0)
	ed.Key(input.KeyTab, 0)
	if vw.Focused() || text() != "ab  " {
		t.Errorf("Esc, Tab: text %q focused %v", text(), vw.Focused())
	}

	// Ctrl+M: Tab moves focus.
	ed.focus(t)
	ed.Key(input.KeyM, input.ModCtrl)
	if text() != "ab  " {
		t.Fatalf("Ctrl+M edited: %q", text())
	}
	ed.Key(input.KeyTab, 0)
	if vw.Focused() || text() != "ab  " {
		t.Errorf("Tab in focus mode: text %q focused %v", text(), vw.Focused())
	}
	// Ctrl+M again: Tab indents again.
	ed.focus(t)
	ed.Key(input.KeyM, input.ModCtrl)
	ed.Key(input.KeyTab, 0)
	if !vw.Focused() || text() != "ab      " {
		t.Errorf("Tab after toggling back: text %q focused %v", text(), vw.Focused())
	}

	// A read-only view does not capture Tab.
	readOnly = true
	ed.Frame()
	ed.Key(input.KeyTab, 0)
	if vw.Focused() {
		t.Errorf("read-only view kept focus on Tab")
	}
}

func TestLongLineWindowStartState(t *testing.T) {
	goText(t)
	th := theme.Base("vs")
	long := strings.Repeat("abc def ", 400) // 3200 runes, inside /* */
	st := NewState("/* open\n"+long+"\n*/\n", "a.go", th)
	ed := newEditDriver(st, 600, 200, nil)
	img := ed.Frame()
	g := (*ed.vw).vs
	lh := st.Viewport.LineHeight
	cm := th.Style("comment.go").Foreground
	bg := defaultColor(th, "editor.background")
	if cm.A == 0 {
		t.Fatal("no comment color")
	}
	r := geom.Rect{X: g.text.X, Y: g.text.Y + lh, W: 20 * g.colW, H: lh}
	if n := countTinted(img, r, bg, cm); n < 10 {
		t.Errorf("long line inside a block comment: %d comment-colored pixels, want >= 10", n)
	}
}

// TestSameFrameInputOrder: keys and committed text of one frame are applied
// in arrival order (Context.EventsFor), through the real Context.
func TestSameFrameInputOrder(t *testing.T) {
	goText(t)
	st := NewState("xy", "a.txt", theme.Base("vs"))
	ed := newEditDriver(st, 300, 100, nil)
	ed.focus(t)
	st.MoveCaret(2, false)
	ed.Frame()
	// xy| Backspace then "a" → xa.
	ed.Inject(input.KeyEvent{Key: input.KeyBackspace, Down: true})
	ed.Inject(input.KeyEvent{Key: input.KeyBackspace})
	ed.Inject(input.TextInput{Text: "a"})
	ed.Settle()
	if got := st.Snapshot().String(); got != "xa" {
		t.Fatalf("Backspace then a: %q, want %q", got, "xa")
	}
	// "b" then Backspace → xa again.
	ed.Inject(input.TextInput{Text: "b"})
	ed.Inject(input.KeyEvent{Key: input.KeyBackspace, Down: true})
	ed.Inject(input.KeyEvent{Key: input.KeyBackspace})
	ed.Settle()
	if got := st.Snapshot().String(); got != "xa" {
		t.Fatalf("b then Backspace: %q, want %q", got, "xa")
	}
}

// TestApplyEventsOrdered feeds an interleaved sequence straight to the
// ordered consumer (bypassing the Context).
func TestApplyEventsOrdered(t *testing.T) {
	goText(t)
	st := NewState("xy", "a.txt", theme.Base("vs"))
	ed := newEditDriver(st, 300, 100, nil)
	st.MoveCaret(2, false)
	w := *ed.vw
	w.applyEvents([]input.Event{
		input.KeyEvent{Key: input.KeyBackspace, Down: true},
		input.KeyEvent{Key: input.KeyBackspace},
		input.TextInput{Text: "a"},
		input.KeyEvent{Key: input.KeyEscape, Down: true}, // ignored
		input.Composition{Text: "ni"},                    // shown, never applied
	})
	if got := st.Snapshot().String(); got != "xa" {
		t.Fatalf("got %q, want %q", got, "xa")
	}
}

// TestShiftClickExtendsWithoutKeyEvent: Shift+click extends the selection
// from the modifiers of the press itself (Context.PressMods), even when no
// key event reported Shift before it.
func TestShiftClickExtendsWithoutKeyEvent(t *testing.T) {
	goText(t)
	st := NewState("hello world\n", "a.txt", theme.Base("vs"))
	ed := newEditDriver(st, 400, 100, nil)
	ed.focus(t)
	st.MoveCaret(1, false)
	ed.Frame()
	p := ed.at(t, st, 0, 7)
	ed.Inject(input.Pointer{Kind: input.Move, Pos: p})
	ed.Inject(input.Pointer{Kind: input.Press, Pos: p, Button: input.ButtonLeft, Mods: input.ModShift})
	ed.Inject(input.Pointer{Kind: input.Release, Pos: p, Button: input.ButtonLeft, Mods: input.ModShift})
	ed.Settle()
	if st.Primary() != sel(1, 7) {
		t.Fatalf("Shift+click: sel %+v, want %+v", st.Primary(), sel(1, 7))
	}
}

// TestNoBlinkWhenWindowUnfocused: no caret blink timer frames while the
// window has no key focus; the blink restarts when it comes back.
func TestNoBlinkWhenWindowUnfocused(t *testing.T) {
	goText(t)
	st := NewState("abc\n", "a.txt", theme.Base("vs"))
	ed := newEditDriver(st, 300, 100, nil)
	ed.focus(t)
	ed.Frame()
	if ed.Runner().NextWake().IsZero() {
		t.Fatal("focused view: no blink wake-up")
	}
	ed.Inject(input.WindowFocus{Focused: false})
	ed.Settle()
	ed.Advance(700 * time.Millisecond)
	ed.Frame()
	if w := ed.Runner().NextWake(); !w.IsZero() {
		t.Errorf("window unfocused: wake-up requested at %v", w)
	}
	ed.Inject(input.WindowFocus{Focused: true})
	ed.Settle()
	if ed.Runner().NextWake().IsZero() {
		t.Error("window refocused: no blink wake-up")
	}
	cur := defaultColor(st.Theme, "editorCursor.foreground")
	img := ed.Frame()
	_, r := ed.TextInputActive()
	if countColor(img, geom.Rect{X: r.X - 1, Y: r.Y, W: r.W + 2, H: r.H}, cur) == 0 {
		t.Error("caret not visible right after the window is refocused")
	}
}

// TestRTLMixedLines: as in Monaco, every line has a left-to-right base
// direction and starts at the left edge; right-to-left runs are reordered
// within it. Caret x, hit testing and selection rectangles follow the
// visual order.
func TestRTLMixedLines(t *testing.T) {
	goText(t)
	lines := []string{
		"// שלום x",          // Hebrew comment: "//" stays on the left
		"مرحبا abc",          // Arabic first: still left-aligned, LTR base
		"s := \"שלום עולם\"", // Hebrew string in code
		"x = 'مرحبا' + 1",    // Arabic string in code
	}
	st := NewState(strings.Join(lines, "\n")+"\n", "a.txt", theme.Base("vs"))
	ed := newEditDriver(st, 600, 200, nil)
	ed.focus(t)
	w := *ed.vw
	g := w.vs
	snap := st.Snapshot()
	for i, line := range lines {
		ls := snap.LineStart(i)
		lv := w.rowAt(snap, i, 0)
		if len(lv.segs) != 1 || lv.segs[0].x0 != 0 {
			t.Fatalf("line %d: not at the left edge (segs %d, x0 %v)", i, len(lv.segs), lv.segs[0].x0)
		}
		if ln := lv.segs[0].lay.Lines; len(ln) == 0 || ln[0].RTL {
			t.Fatalf("line %d: right-to-left base direction", i)
		}
		// Hit testing: clicking at each boundary's caret x lands on an
		// offset with that caret x (at a direction boundary two offsets
		// share it), and on the offset itself when its x is unique.
		xs := map[float32]int{}
		minX := float32(math.MaxFloat32)
		var bounds []int
		for c := 0; ; c = nextGrapheme(line, c) {
			bounds = append(bounds, c)
			x, _ := w.contentX(snap, ls+c)
			xs[x]++
			minX = min(minX, x)
			if c >= len(line) {
				break
			}
		}
		// The leftmost caret is at the left edge (for "مرحبا abc" that
		// is the end of the Arabic run, drawn first from the left).
		if minX != 0 {
			t.Errorf("line %d: leftmost caret x %v, want 0", i, minX)
		}
		for _, c := range bounds {
			x, _ := w.contentX(snap, ls+c)
			got := w.offsetAt(snap, geom.Pt(g.text.X+x, g.text.Y+(float32(i)+0.5)*st.Viewport.LineHeight)) - ls
			gx, _ := w.contentX(snap, ls+got)
			if abs32(gx-x) > 0.5 {
				t.Errorf("line %d col %d: hit at x %v gives col %d with caret x %v", i, c, x, got, gx)
			} else if xs[x] == 1 && got != c {
				t.Errorf("line %d col %d: hit at x %v gives col %d", i, c, x, got)
			}
		}
	}
	// Line 0 visual order: "//" left of the Hebrew word, "x" right of it.
	ls := snap.LineStart(0)
	heb := strings.Index(lines[0], "ש")
	hebEnd := heb + len("שלום")
	xSlash, _ := w.contentX(snap, ls+2)
	xHebA, _ := w.contentX(snap, ls+heb+len("ש")) // inside the run
	xX, _ := w.contentX(snap, ls+len(lines[0]))
	if !(xSlash < xHebA && xHebA < xX) {
		t.Errorf("visual order: // %v, inside שלום %v, end %v", xSlash, xHebA, xX)
	}
	// Selection of the Hebrew word covers its glyphs, between "// " and
	// " x", and nothing over "//".
	st.MoveCaret(ls+heb, false)
	st.MoveCaret(ls+hebEnd, true)
	img := ed.Frame()
	selBg := defaultColor(st.Theme, "editor.selectionBackground")
	lh := st.Viewport.LineHeight
	xSp, _ := w.contentX(snap, ls+heb)       // caret after "// "
	xSp2, _ := w.contentX(snap, ls+hebEnd+1) // caret after " " following the word
	mid := geom.Rect{X: g.text.X + (xSp+xSp2)/2 - 1, Y: g.text.Y + 1, W: 2, H: lh - 2}
	if countColor(img, mid, selBg) == 0 {
		t.Errorf("no selection over the Hebrew word")
	}
	left := geom.Rect{X: g.text.X, Y: g.text.Y + 1, W: xSlash, H: lh - 2}
	if n := countColor(img, left, selBg); n != 0 {
		t.Errorf("%d selection pixels over \"//\"", n)
	}
	// The composition path lays the caret line out the same way.
	st.MoveCaret(ls+len(lines[0]), false)
	ed.Compose("ab", 2)
	_, caret := ed.TextInputActive()
	if caret.X <= g.text.X+xX {
		t.Errorf("composition caret %v not right of the line end %v", caret.X, g.text.X+xX)
	}
}
