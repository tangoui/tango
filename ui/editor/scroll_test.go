package editor

import (
	"math"
	"testing"

	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// TestScrollPixels: ScrollTop, LineTop, LineBlock and LineAtTop agree with
// each other and with the view lines, through zones, hidden lines,
// wrapping and the top padding; SetLineAtTop inverts LineAtTop.
func TestScrollPixels(t *testing.T) {
	goText(t)
	var lines []string
	for i := range 200 {
		s := "line"
		if i%7 == 3 {
			s = "a long line that wraps: x x x x x x x x x x x x x x x x x x x x x x x x x x x x x x x x x x x x x"
		}
		lines = append(lines, s)
	}
	doc := ""
	for _, l := range lines {
		doc += l + "\n"
	}
	st := NewState(doc, "a.txt", theme.Base("vs"))
	st.Options.WordWrap = WrapOn
	st.Options.Padding.Top = 6
	d, _ := viewDriver(st, 400, 300, nil)
	d.Frame()
	st.SetViewZones("z", []ViewZone{{ID: 1, AfterLine: -1, Rows: 2}, {ID: 2, AfterLine: 10, Rows: 3}, {ID: 3, AfterLine: 40, Rows: 1}})
	st.SetHiddenAreas("h", []LineRange{{20, 30}})
	d.Frame()
	lh := float64(st.Viewport.LineHeight)
	pad := 6.0
	if top, text, zones := st.LineBlock(10); top != st.LineTop(10) || text != st.RowsOf(10) || zones != 3 {
		t.Fatalf("LineBlock(10) = %v, %d, %d", top, text, zones)
	}
	if got, want := st.LineTop(0), 2*lh+pad; got != want {
		t.Fatalf("LineTop(0) = %v, want %v (the 2 zone rows before it)", got, want)
	}
	if got, want := st.LineTop(11), st.LineTop(10)+float64(st.RowsOf(10)+3)*lh; got != want {
		t.Fatalf("LineTop(11) = %v, want %v", got, want)
	}
	if got := st.LineTop(25); got != st.LineTop(30) {
		t.Fatalf("hidden line's top %v, want the next shown line's %v", got, st.LineTop(30))
	}
	if _, text, zones := st.LineBlock(25); text != 0 || zones != 0 {
		t.Fatal("a hidden line has rows")
	}
	for _, c := range []struct {
		line int
		off  float32
	}{{0, 0}, {0, -20}, {0, -48}, {10, 30}, {10, 70}, {50, 5}, {150, 0}} {
		st.SetLineAtTop(c.line, c.off)
		line, off := st.LineAtTop()
		if line != c.line || off != c.off {
			t.Fatalf("SetLineAtTop(%d, %v): LineAtTop %d, %v", c.line, c.off, line, off)
		}
		if got, want := st.ScrollTop(), st.LineTop(line)+float64(off); math.Abs(got-want) > 1e-3 {
			t.Fatalf("ScrollTop %v, want LineTop+offset %v", got, want)
		}
	}
	st.SetScrollTop(0)
	if st.ScrollTop() != 0 || st.Viewport.TopLine != 0 || st.Viewport.OffsetY != -6 {
		t.Fatalf("SetScrollTop(0): %v, top %d+%v", st.ScrollTop(), st.Viewport.TopLine, st.Viewport.OffsetY)
	}
	st.SetScrollTop(1e12)
	if want := st.ScrollHeight() - float64(st.Viewport.Height); math.Abs(st.ScrollTop()-want) > 1e-3 {
		t.Fatalf("scrolled to the end: %v, want ScrollHeight-Height %v", st.ScrollTop(), want)
	}
	st.SetScrollLeft(-5)
	if st.ScrollLeft() != 0 {
		t.Fatal("negative ScrollLeft")
	}
}

// TestRevealLine: the four places a line is revealed at.
func TestRevealLine(t *testing.T) {
	goText(t)
	st := NewState(manyLines(500), "a.txt", theme.Base("vs"))
	d, _ := viewDriver(st, 400, 300, nil)
	d.Frame()
	lh := float64(st.Viewport.LineHeight)
	h := float64(st.Viewport.Height)
	st.RevealLine(100, RevealTop)
	if st.ScrollTop() != st.LineTop(100) {
		t.Fatalf("RevealTop: %v, want %v", st.ScrollTop(), st.LineTop(100))
	}
	st.RevealLine(200, RevealCenter)
	if got, want := st.ScrollTop(), st.LineTop(200)+lh/2-h/2; math.Abs(got-want) > 1e-3 {
		t.Fatalf("RevealCenter: %v, want %v", got, want)
	}
	before := st.ScrollTop()
	st.RevealLine(201, RevealCenterIfOutside)
	if st.ScrollTop() != before {
		t.Fatal("RevealCenterIfOutside scrolled to a shown line")
	}
	st.RevealLine(400, RevealCenterIfOutside)
	if got, want := st.ScrollTop(), st.LineTop(400)+lh/2-h/2; math.Abs(got-want) > 1e-3 {
		t.Fatalf("RevealCenterIfOutside: %v, want %v", got, want)
	}
	st.RevealLine(450, RevealNearest)
	if got, want := st.ScrollTop(), st.LineTop(450)+lh-h; math.Abs(got-want) > 1e-3 {
		t.Fatalf("RevealNearest below: %v, want %v (the line at the bottom)", got, want)
	}
	snap := st.Snapshot()
	st.RevealRange(snap.LineStart(10), snap.LineStart(12), RevealNearest)
	if got := st.ScrollTop(); got != st.LineTop(10) {
		t.Fatalf("RevealRange above: %v, want %v", got, st.LineTop(10))
	}
}

// TestOnDidScroll: the wheel, a key that reveals the caret, a drag on the
// overview ruler and a direct SetScrollTop are reported (once, with the
// new position); rows above the top becoming known while wrapping are
// not; a callback can scroll another view.
func TestOnDidScroll(t *testing.T) {
	goText(t)
	doc := ""
	for i := range 400 {
		if i%3 == 0 {
			doc += "a long line that wraps x x x x x x x x x x x x x x x x x x x x x x x x x x x x x x x x x\n"
		} else {
			doc += "short\n"
		}
	}
	st := NewState(doc, "a.txt", theme.Base("vs"))
	st.Options.WordWrap = WrapOn
	other := NewView(st.Model, theme.Base("vs"))
	var evs []ScrollEvent
	cancel := st.OnDidScroll(func(e ScrollEvent) {
		evs = append(evs, e)
		other.Viewport.LineHeight, other.Viewport.Height = st.Viewport.LineHeight, st.Viewport.Height
		other.SetScrollTop(e.ScrollTop)
	})
	h := newHintHost(t, st, geom.Size{W: 400, H: 300}, 1)
	h.frame()
	h.frame()
	if len(evs) != 0 {
		t.Fatalf("events without scrolling: %+v", evs)
	}
	h.r.HandleEvent(input.Scroll{Pos: geom.Pt(200, 150), DY: 30})
	h.frame()
	if len(evs) != 1 || !evs[0].TopChanged || evs[0].LeftChanged || evs[0].ScrollTop != st.ScrollTop() || other.ScrollTop() != st.ScrollTop() {
		t.Fatalf("wheel: %+v (ScrollTop %v, other %v)", evs, st.ScrollTop(), other.ScrollTop())
	}
	if l, off := st.LineAtTop(); evs[0].Line != l || evs[0].Offset != off {
		t.Fatalf("event at %d+%v, LineAtTop %d+%v", evs[0].Line, evs[0].Offset, l, off)
	}
	// A key moving the caret below the viewport reveals it.
	h.focusAt(2)
	evs = nil
	for range 20 {
		h.key(input.KeyDown, 0)
		h.frame()
	}
	if len(evs) == 0 {
		t.Fatal("the caret's reveal was not reported")
	}
	// The overview ruler.
	evs = nil
	ov := h.vw.vs.overviewRect
	if ov.Empty() {
		t.Fatal("no overview ruler")
	}
	p := geom.Pt(ov.X+ov.W/2, ov.Y+ov.H*0.7)
	h.r.HandleEvent(input.Pointer{Kind: input.Move, Pos: p})
	h.r.HandleEvent(input.Pointer{Kind: input.Press, Pos: p})
	h.frame()
	h.r.HandleEvent(input.Pointer{Kind: input.Release, Pos: p})
	h.frame()
	if len(evs) == 0 {
		t.Fatal("the overview ruler's scroll was not reported")
	}
	// Measuring the rows above the top moves TopLine, not the picture.
	st.SetLineAtTop(300, 0)
	h.frame()
	evs = nil
	vl := st.lines()
	clear(vl.rows)
	vl.rebuild()
	st.anchorTop(300, 0)
	top := st.Viewport.TopLine
	st.measure(0, 299)
	if st.Viewport.TopLine == top {
		t.Fatal("measuring did not move TopLine")
	}
	h.frame()
	if len(evs) != 0 {
		t.Fatalf("measuring was reported as a scroll: %+v", evs)
	}
	// Directly, between frames; horizontally.
	st.SetScrollTop(100)
	h.frame()
	if len(evs) != 1 {
		t.Fatalf("SetScrollTop: %d events", len(evs))
	}
	cancel()
	st.SetScrollTop(300)
	h.frame()
	if len(evs) != 1 {
		t.Fatal("event after cancel")
	}
}
