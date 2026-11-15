package editor

import (
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"image/color"
	"strings"
	"testing"
	"time"
)

func TestMinimapDoesNotReplaceBlinkWake(t *testing.T) {
	goText(t)
	st := NewState(strings.Repeat("var x = 1 // token work\n", 20000), "a.go", theme.Base("vs"))
	defer st.Close()
	st.Options.OccurrencesHighlight = OccurrencesOff
	st.Options.MatchBrackets = MatchBracketsNever
	st.Options.Folding = false
	st.Options.StickyScroll.Enabled = false
	st.Viewport.TopLine = 10000
	h := newHintHost(t, st, geom.Size{W: 640, H: 300}, 1.25)
	h.focusAt(10003)
	next := h.r.NextWake()
	if next.Sub(h.now) != blinkPeriod/2 {
		t.Fatalf("minimap replaced blink wake with %v", next.Sub(h.now))
	}
}

func TestOverviewOwnerOrderDamage(t *testing.T) {
	goText(t)
	st := NewState(manyLines(100), "a.txt", theme.Base("vs"))
	defer st.Close()
	st.Options.OccurrencesHighlight = OccurrencesOff
	st.Options.SelectionHighlight = false
	h := newHintHost(t, st, geom.Size{W: 640, H: 300}, 1.25)
	h.focusAt(3)
	red := []OverviewMark{{Line: 50, Color: color.NRGBA{255, 0, 0, 255}, Lane: 7}}
	blue := []OverviewMark{{Line: 50, Color: color.NRGBA{0, 0, 255, 255}, Lane: 7}}
	st.SetOverviewMarks("a", red)
	st.SetOverviewMarks("b", blue)
	h.frame()
	st.SetOverviewMarks("a", nil)
	st.SetOverviewMarks("z", red)
	h.now = h.now.Add(600 * time.Millisecond)
	h.step("overlapping owners reordered with caret blink")
	if h.partial == 0 {
		t.Fatal("owner-order frame unexpectedly full")
	}
}

func TestViewDamageVerifiedTask14(t *testing.T) {
	goText(t)
	for _, scale := range []float32{1, 1.25, 1.5, 2} {
		st := NewState("func outer() {\n  func inner() {\n"+strings.Repeat("    return 1 // body\n", 100)+"  }\n}\n", "a.go", theme.Base("vs-dark"))
		st.Options.Minimap.ShowSlider = MinimapSliderAlways
		h := newHintHost(t, st, geom.Size{W: 640, H: 300}, scale)
		h.focusAt(4)
		for range 12 {
			st.ScrollBy(16, 0)
			h.step("sticky and minimap scroll")
		}
		if len(h.vw.vs.stickyLines) == 0 {
			t.Fatal("no sticky headers")
		}
		st.SetOverviewMarks("find", []OverviewMark{{Line: 40, Color: color.NRGBA{255, 0, 0, 255}, Lane: 5}})
		h.step("owner marks")
		st.SetOverviewMarks("find", []OverviewMark{{Line: 70, Color: color.NRGBA{0, 0, 255, 255}, Lane: 2}})
		h.step("owner replacement")
		st.SetOverviewMarks("find", nil)
		h.step("owner removal")
		r := h.vw.vs.minimapRect
		h.r.HandleEvent(input.Pointer{Kind: input.Move, Pos: geom.Pt(r.X+2, r.Y+50)})
		h.step("minimap hover")
		st.Options.Minimap.Side = MinimapLeft
		h.step("left minimap")
		st.Options.StickyScroll.Enabled = false
		h.step("sticky disabled")
		st.Options.Minimap.Enabled = false
		h.step("minimap disabled")
		if h.partial*100 < h.frames*70 {
			t.Fatalf("partial %d/%d", h.partial, h.frames)
		}
		st.Close()
	}
}
