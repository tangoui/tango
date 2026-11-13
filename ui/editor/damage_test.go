package editor

import (
	"bytes"
	"fmt"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/gpu"
	"github.com/tangoui/tango/ui/gpu/cpuexec"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/paint"
)

// hintWindow is a ui.Window without a screen for the redraw hint tests.
type hintWindow struct {
	size  geom.Size
	scale float32
	clip  string
}

func (w *hintWindow) Size() geom.Size              { return w.size }
func (w *hintWindow) Scale() float32               { return w.scale }
func (w *hintWindow) Invalidate()                  {}
func (w *hintWindow) InvalidateAt(time.Time)       {}
func (w *hintWindow) SetCursor(input.Cursor)       {}
func (w *hintWindow) ReadClipboard() string        { return w.clip }
func (w *hintWindow) WriteClipboard(s string)      { w.clip = s }
func (w *hintWindow) SetTextInput(bool, geom.Rect) {}
func (w *hintWindow) Present(*image.RGBA) error    { return nil }

// hintHost runs a page showing st through a gpu.Renderer on a
// cpuexec.Device in strict verify mode: every partial frame is compared
// with a full redraw, and a difference fails the frame.
type hintHost struct {
	t   *testing.T
	st  *State
	w   *hintWindow
	r   *ui.Runner
	ren *gpu.Renderer
	dev *cpuexec.Device
	vw  *ViewW
	now time.Time

	frames, partial int // counted frames and the partial ones among them
}

func newHintHost(t *testing.T, st *State, size geom.Size, scale float32) *hintHost {
	t.Helper()
	h := &hintHost{t: t, st: st, w: &hintWindow{size: size, scale: scale}, dev: cpuexec.NewDevice(),
		now: time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)}
	h.dev.SampledOnly = true // same pixels, much faster (see cpuexec.Device)
	h.ren = gpu.NewRenderer(h.dev)
	h.ren.SetVerify(true)
	h.ren.SetVerifyStrict(true)
	h.r = ui.NewRunner(func(c *ui.Context) {
		h.vw = View(c, st)
	}, ui.RunnerOptions{Renderer: h.ren, NoAnimations: true, Clock: func() time.Time { return h.now }})
	h.r.Attach(h.w)
	return h
}

// frame runs one frame and reports whether the renderer drew it partially.
func (h *hintHost) frame() bool {
	h.t.Helper()
	before := h.dev.Stats().Partial
	if err := h.r.Frame(); err != nil {
		h.t.Fatalf("frame %d: %v", h.frames, err)
	}
	return h.dev.Stats().Partial > before
}

// step runs a counted frame.
func (h *hintHost) step(what string) {
	h.t.Helper()
	h.frames++
	if h.frame() {
		h.partial++
	} else if testing.Verbose() {
		h.t.Logf("full frame: %s", what)
	}
}

func (h *hintHost) key(k input.Key, mods input.Modifiers) {
	h.r.HandleEvent(input.KeyEvent{Key: k, Down: true, Mods: mods})
	h.r.HandleEvent(input.KeyEvent{Key: k, Mods: mods})
}

func (h *hintHost) typeText(s string) {
	h.r.HandleEvent(input.KeyEvent{Key: input.KeyX, Down: true})
	h.r.HandleEvent(input.TextInput{Text: s})
	h.r.HandleEvent(input.KeyEvent{Key: input.KeyX})
}

// lineHints returns the Damage and ScrollRegion hints of the last frame.
func (h *hintHost) lineHints() (dam []geom.Rect, sc []paint.ScrollRegion) {
	for _, op := range h.r.List().Ops {
		switch o := op.(type) {
		case paint.Damage:
			dam = append(dam, o.Rect)
		case paint.ScrollRegion:
			sc = append(sc, o)
		}
	}
	return
}

// focusAt clicks at line (about the middle of the text area's width).
func (h *hintHost) focusAt(line int) {
	h.t.Helper()
	h.frame()
	g := h.vw.vs
	v := h.st.Viewport
	p := geom.Pt(g.text.X+40, g.text.Y+(float32(line-v.TopLine)+0.5)*v.LineHeight)
	h.r.HandleEvent(input.Pointer{Kind: input.Move, Pos: p})
	h.r.HandleEvent(input.Pointer{Kind: input.Press, Pos: p})
	h.r.HandleEvent(input.Pointer{Kind: input.Release, Pos: p})
	h.frame()
	h.frame()
	if !h.vw.Focused() {
		h.t.Fatal("view not focused")
	}
}

// hintDoc is a Go document with a few token kinds on every line.
func hintDoc(n int) string {
	s := "package main\n\n"
	for i := range n {
		s += fmt.Sprintf("func f%d(x int) string { return \"v%d\" + fmt.Sprint(x*%d) } // note %d\n", i, i, i, i)
	}
	return s
}

// TestViewDamageVerified runs an editing script through a GPU renderer on
// cpuexec in strict verify mode (spec 6.2, 6.8): every partial frame must
// equal a full redraw, and most frames must be partial.
func TestViewDamageVerified(t *testing.T) {
	goText(t)
	scales := []float32{1, 1.25, 1.5}
	if raceEnabled {
		scales = []float32{1.25} // every frame rasterizes on the CPU twice, 10–20× slower under -race
	}
	for _, scale := range scales {
		t.Run(fmt.Sprintf("scale%v", scale), func(t *testing.T) {
			th := theme.Base("vs-dark")
			if scale == 1.25 {
				th = theme.Base("vs") // the current line gets a border, not a fill
			}
			st := NewState(hintDoc(120), "main.go", th)
			h := newHintHost(t, st, geom.Size{W: 480, H: 360}, scale)
			h.focusAt(10)

			// Typing, backspace, Enter.
			for i := range 20 {
				h.typeText(string(rune('a' + i)))
				h.step("type")
			}
			h.key(input.KeyBackspace, 0)
			h.step("backspace")
			h.key(input.KeyEnter, 0)
			h.step("enter")

			// The caret blinks: the host wakes the runner at NextWake.
			for range 3 {
				next := h.r.NextWake()
				if !next.After(h.now) {
					t.Fatalf("no blink timer (NextWake %v, now %v)", next, h.now)
				}
				h.now = next
				h.step("blink")
				if dam, sc := h.lineHints(); len(sc) != 0 || len(dam) == 0 || dam[0].W > 4 {
					t.Fatalf("blink frame hints: damage %v, scroll %v (want only the caret)", dam, sc)
				}
			}

			// Wheel scrolling, with fractional offsets.
			p := geom.Pt(240, 180)
			for _, dy := range []float32{8, 16, 4, 2.4, 12, -8, -4, 21, -2.4, 6} {
				h.r.HandleEvent(input.Scroll{Pos: p, DY: dy})
				h.step(fmt.Sprintf("scroll %v", dy))
			}

			// Horizontal scrolling (the current line's highlight does not
			// scroll with the text).
			for _, dx := range []float32{8, 16, -8} {
				h.r.HandleEvent(input.Scroll{Pos: p, DX: dx})
				h.step(fmt.Sprintf("scroll x %v", dx))
			}
			if st.Viewport.ScrollX == 0 {
				t.Fatal("did not scroll horizontally")
			}

			// Shift selection.
			for range 3 {
				h.key(input.KeyRight, input.ModShift)
				h.step("shift+right")
			}
			h.key(input.KeyDown, input.ModShift)
			h.step("shift+down")

			// Several cursors, typing at each.
			snap := st.Snapshot()
			st.SetSelections([]Selection{
				{Anchor: snap.LineStart(8) + 4, Head: snap.LineStart(8) + 4},
				{Anchor: snap.LineStart(9) + 4, Head: snap.LineStart(9) + 4},
				{Anchor: snap.LineStart(11) + 4, Head: snap.LineStart(11) + 4},
			})
			h.step("cursors")
			for _, s := range []string{"x", "y", "z"} {
				h.typeText(s)
				h.step("multi-cursor type")
			}
			st.SetSelections(st.Sels[:1])
			h.step("single cursor")

			// IME composition and commit.
			for i, s := range []string{"n", "ni", "nih"} {
				h.r.HandleEvent(input.Composition{Text: s, Cursor: i + 1})
				h.step("compose")
			}
			h.r.HandleEvent(input.Composition{})
			h.r.HandleEvent(input.TextInput{Text: "你好"})
			h.step("commit")

			h.verified()
		})
	}
	// Word wrap: rows are view lines (see damageWrapScript).
	for _, scale := range scales {
		t.Run(fmt.Sprintf("wrap/scale%v", scale), func(t *testing.T) { damageWrapScript(t, scale) })
	}
	t.Run("staleTop", damageStaleTop)
	t.Run("recolour", damageRecolour)
	t.Run("otherView", damageOtherView)
}

// verified fails the test on a verify failure or when fewer than 70% of
// the counted frames were partial.
func (h *hintHost) verified() {
	h.t.Helper()
	if n, last := h.ren.VerifyFailures(); n != 0 {
		h.t.Fatalf("%d verify failures, last %v", n, last)
	}
	if 10*h.partial < 7*h.frames {
		h.t.Fatalf("%d of %d frames partial, want at least 70%%", h.partial, h.frames)
	}
	h.t.Logf("%d of %d frames partial", h.partial, h.frames)
}

// damageWrapScript is TestViewDamageVerified's script with word wrap on:
// typing that changes a line's row count, moving by rows, Home/End on
// rows, selections across rows, scrolling (rows above the top becoming
// known), an edit through another view above the top, a composition on a
// wrapped line, and the changes that redraw everything (a resize, which
// moves the wrapping column, and toggling word wrap).
func damageWrapScript(t *testing.T, scale float32) {
	th := theme.Base("vs-dark")
	if scale == 1.25 {
		th = theme.Base("vs")
	}
	st := NewState(hintDoc(160), "main.go", th)
	st.Options.WordWrap = WrapOn
	h := newHintHost(t, st, geom.Size{W: 480, H: 360}, scale)
	h.focusAt(10)
	if !st.wrapping() || st.breaks(snapLineOfHead(st)).rows() < 2 {
		t.Fatal("lines do not wrap")
	}
	for i := range 24 {
		h.typeText(string(rune('a' + i%26)))
		h.step("type")
	}
	h.key(input.KeyBackspace, 0)
	h.step("backspace")
	h.key(input.KeyEnter, 0)
	h.step("enter")
	for _, k := range []input.Key{input.KeyDown, input.KeyDown, input.KeyDown, input.KeyUp, input.KeyEnd, input.KeyEnd, input.KeyHome, input.KeyHome, input.KeyDown} {
		h.key(k, 0)
		h.step(fmt.Sprintf("key %v", k))
	}
	for range 3 {
		h.key(input.KeyDown, input.ModShift)
		h.step("shift+down")
	}
	h.key(input.KeyRight, 0)
	h.step("collapse")
	p := geom.Pt(240, 180)
	for _, dy := range []float32{8, 16, 4, 2.4, 12, 40, -8, -4, 21, -2.4, 6, -60, -30} {
		h.r.HandleEvent(input.Scroll{Pos: p, DY: dy})
		h.step(fmt.Sprintf("scroll %v", dy))
	}
	// Scrolled into the middle: upward scrolling discovers the rows of the
	// lines above the top.
	st.Viewport.TopLine, st.Viewport.OffsetY = st.vl.viewOf(120), 0
	h.frame()
	for range 8 {
		h.r.HandleEvent(input.Scroll{Pos: p, DY: -st.Viewport.LineHeight * 1.5})
		h.step("scroll up")
	}
	// Another view inserts lines above the top.
	other := NewView(st.Model, th)
	defer other.Close()
	other.MoveCaret(other.Snapshot().LineStart(3), false)
	other.Insert("// one\n// two that is long enough to wrap onto a second row of the view\n")
	h.step("other view insert above")
	snap := st.Snapshot()
	st.MoveCaret(snap.LineStart(st.topModelLine()+2)+5, false)
	h.step("caret")
	for i, s := range []string{"n", "ni", "nih"} {
		h.r.HandleEvent(input.Composition{Text: s, Cursor: i + 1})
		h.step("compose")
	}
	h.r.HandleEvent(input.Composition{})
	h.r.HandleEvent(input.TextInput{Text: "你好"})
	h.step("commit")
	// Whole-view changes, then partial frames again.
	h.w.size = geom.Size{W: 420, H: 360}
	h.frame()
	h.typeText("q")
	h.step("type after resize")
	st.ToggleWordWrap()
	h.frame()
	h.typeText("r")
	h.step("type unwrapped")
	st.ToggleWordWrap()
	h.frame()
	h.typeText("s")
	h.step("type wrapped again")
	h.verified()
}

func snapLineOfHead(st *State) int { return st.Snapshot().LineOf(st.Primary().Head) }

// longGoDoc is hintDoc for n lines, built in linear time.
func longGoDoc(n int) string {
	var b strings.Builder
	b.WriteString("package main\n\n")
	for i := range n {
		fmt.Fprintf(&b, "func f%d(x int) string { return \"v%d\" + fmt.Sprint(x*%d) } // note %d\n", i, i, i, i)
	}
	return b.String()
}

// damageRecolour: rows nobody edited change colors when highlighting
// catches up (the Highlighter's background work, here run directly): a
// "/*" far above the view turns the visible rows down to a "*/" in the
// middle of the view into a comment once the lines in between are
// tokenized, and back when it is deleted.
func damageRecolour(t *testing.T) {
	th := theme.Base("vs")
	n := 20000
	if raceEnabled {
		n = 8000
	}
	end := n - 197 // the line holding "*/", near the top of the view
	doc := strings.Split(longGoDoc(n), "\n")
	doc[end] = "var x = 1 */ + 2"
	st := NewState(strings.Join(doc, "\n"), "main.go", th)
	h := newHintHost(t, st, geom.Size{W: 480, H: 360}, 1.25)
	h.focusAt(3)
	hl := st.Highlighter()
	hl.Highlight(st.Snapshot(), n, 30*time.Second)
	st.Viewport.TopLine = n - 200
	h.frame()
	h.focusAt(n - 190)
	// Open a comment far above the view (Insert: no auto-closing), the
	// caret staying in the view.
	caret := st.Primary().Head
	st.MoveCaret(st.Snapshot().LineStart(4), false)
	st.Insert("/*")
	st.MoveCaret(caret+2, false)
	h.frame()
	hl.Highlight(st.Snapshot(), n, 30*time.Second)
	h.step("recolour")
	dam, sc := h.lineHints()
	if len(dam) == 0 || len(sc) != 0 {
		t.Fatalf("recolouring declared damage %v, scroll %v", dam, sc)
	}
	for i := range 4 {
		h.typeText(string(rune('a' + i)))
		h.step("type")
	}
	caret = st.Primary().Head
	st.SetSelections([]Selection{{Anchor: st.Snapshot().LineStart(4), Head: st.Snapshot().LineStart(4) + 2}})
	st.Insert("")
	st.MoveCaret(caret-2, false)
	h.frame()
	hl.Highlight(st.Snapshot(), n, 30*time.Second)
	h.step("recolour back")
	h.verified()
}

// damageOtherView: edits through another view of the same Model above,
// in and below the visible lines. Edits that change the line count above
// or in the view move every line number below them (whole-view damage, not
// counted); the others must be partial.
func damageOtherView(t *testing.T) {
	th := theme.Base("vs-dark")
	st := NewState(hintDoc(200), "main.go", th)
	h := newHintHost(t, st, geom.Size{W: 480, H: 360}, 1)
	h.focusAt(5)
	st.Viewport.TopLine = 40
	h.frame()
	h.focusAt(46)
	other := NewView(st.Model, th)
	defer other.Close()
	edit := func(what string, counted bool, line, col int, text string, del int) {
		s := other.Snapshot()
		a := s.LineStart(line) + col
		other.SetSelections([]Selection{{Anchor: a, Head: a + del}})
		other.Insert(text)
		if counted {
			h.step(what)
		} else {
			h.frame()
		}
	}
	edit("insert above", false, 10, 0, "// x\n// y\n", 0)
	edit("delete above", false, 12, 0, "", len(other.Snapshot().Line(12))+1)
	edit("type in view", true, 45, 3, "zz", 0)
	edit("type in view", true, 48, 0, "q", 0)
	edit("replace in view", true, 50, 2, "longer replacement", 4)
	edit("split in view", false, 47, 10, "\n", 0)
	edit("join in view", false, 50, 0, "", 0)
	edit("delete in view", false, 44, 0, "", len(other.Snapshot().Line(44))+1)
	edit("insert below", false, 150, 0, "// below\n", 0)
	edit("type below", false, 150, 0, "x", 0)
	for i := range 6 {
		h.typeText(string(rune('a' + i)))
		h.step("type")
	}
	h.verified()
}

// TestViewDamageGutterDigits: the line numbers widen from 99999 to 100000
// lines; in a view too narrow for the gutter its rectangle does not
// change, so the numbers' width itself redraws the whole view.
func TestViewDamageGutterDigits(t *testing.T) {
	goText(t)
	st := NewState(strings.Repeat("x\n", 99998), "a.txt", theme.Base("vs"))
	h := newHintHost(t, st, geom.Size{W: 60, H: 200}, 1)
	st.Viewport.TopLine = 99990
	h.frame()
	h.frame()
	g := h.vw.vs.gutter
	snap := st.Snapshot()
	st.MoveCaret(snap.Len(), false)
	st.Insert("\n")
	if st.Snapshot().LineCount() != 100000 {
		t.Fatalf("%d lines", st.Snapshot().LineCount())
	}
	if h.frame() {
		t.Fatal("drawn partially")
	}
	if h.vw.vs.gutter != g {
		t.Fatalf("gutter %v -> %v: not the clipped case", g, h.vw.vs.gutter)
	}
	whole := false
	dam, _ := h.lineHints()
	for _, d := range dam {
		if d.Intersect(h.vw.vs.rect) == h.vw.vs.rect {
			whole = true
		}
	}
	if !whole {
		t.Fatalf("damage %v does not cover the view %v", dam, h.vw.vs.rect)
	}
	if n, last := h.ren.VerifyFailures(); n != 0 {
		t.Fatalf("%d verify failures, last %v", n, last)
	}
}

// damageStaleTop: with the caret on a wrapped line above the view whose
// rows are not known (a resize forgot them, or another view edited the
// line), painting must not measure it after laying out the rows: that
// moved TopLine under the drawn rows, and the next frame (none scheduled:
// the caret is off screen) showed the text shifted. The frame after the
// change must equal the next one, pixel for pixel, and the strict renderer
// must accept the partial frames around it.
func damageStaleTop(t *testing.T) {
	goText(t)
	for _, c := range []string{"resize", "other view edit"} {
		m := NewModel("a.txt", bigWrapDoc(400), DefaultRegistry())
		st := NewView(m, theme.Base("vs"))
		st.Options.WordWrap = WrapOn
		other := NewView(m, theme.Base("vs"))
		ed := newEditDriver(st, 500, 300, pinMac)
		st.MoveCaret(st.Snapshot().LineStart(100)+3, false)
		ed.Frame()
		if st.breaks(100).rows() < 2 {
			t.Fatalf("%s: line 100 does not wrap", c)
		}
		st.Viewport.TopLine = st.vl.viewOf(250)
		ed.Frame()
		if c == "resize" {
			ed.Resize(420, 300)
		} else {
			other.MoveCaret(other.Snapshot().LineStart(100)+5, false)
			other.Insert("x")
		}
		img1 := append([]byte(nil), ed.Frame().Pix...)
		top := st.Viewport.TopLine
		img2 := append([]byte(nil), ed.Frame().Pix...)
		if !bytes.Equal(img1, img2) || st.Viewport.TopLine != top {
			t.Errorf("%s: the frame after differs from the next one (TopLine %d -> %d)", c, top, st.Viewport.TopLine)
		}
		if l, r := st.topAnchor(); l != 250 || r != 0 {
			t.Errorf("%s: top %d/%d, want 250/0", c, l, r)
		}
		other.Close()
		st.Close()
	}
	// The same through the strict renderer.
	m := NewModel("a.txt", bigWrapDoc(400), DefaultRegistry())
	st := NewView(m, theme.Base("vs-dark"))
	defer st.Close()
	st.Options.WordWrap = WrapOn
	other := NewView(m, theme.Base("vs-dark"))
	defer other.Close()
	h := newHintHost(t, st, geom.Size{W: 480, H: 360}, 1)
	h.focusAt(3)
	st.MoveCaret(st.Snapshot().LineStart(100)+3, false)
	h.frame()
	st.Viewport.TopLine = st.vl.viewOf(250)
	h.frame()
	other.MoveCaret(other.Snapshot().LineStart(100)+5, false)
	other.Insert("x")
	h.frame()
	h.w.size = geom.Size{W: 420, H: 360}
	h.frame()
	p := geom.Pt(240, 180)
	for _, dy := range []float32{4, 9, -6, 3} {
		h.r.HandleEvent(input.Scroll{Pos: p, DY: dy})
		h.step("scroll")
	}
	// An edit above the view changes nothing on screen.
	other.MoveCaret(other.Snapshot().LineStart(100)+5, false)
	other.Insert("y")
	h.frame()
	if dam, sc := h.lineHints(); len(dam) != 0 || len(sc) != 0 {
		t.Errorf("edit above the view: damage %v, scroll %v", dam, sc)
	}
	for _, dy := range []float32{-5, 7} {
		h.r.HandleEvent(input.Scroll{Pos: p, DY: dy})
		h.step("scroll")
	}
	h.verified()
}
