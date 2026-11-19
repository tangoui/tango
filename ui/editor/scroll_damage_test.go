package editor

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

func TestFractionalWheelReusesEditorPixels(t *testing.T) {
	goText(t)
	for _, scale := range []float32{1, 1.25, 1.5, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			st := NewState("func outer() {\n"+strings.Repeat("  work()\n", 70)+"}\n", "a.go", theme.Base("vs-dark"))
			t.Cleanup(st.Close)
			st.Options.Minimap.Enabled = false
			h := newHintHost(t, st, geom.Size{W: 360, H: 210}, scale)
			h.frame()
			// Twelve rows start on an integer device pixel at all scales.
			st.Viewport.TopLine = 12
			h.frame()
			h.frame()
			if len(h.vw.vs.stickyLines) == 0 {
				t.Fatal("expected a sticky header over the scrolling body")
			}
			box := h.vw.vs.text
			p := geom.Pt(box.X+32, box.Y+5*st.Viewport.LineHeight)
			h.r.HandleEvent(input.Pointer{Kind: input.Move, Pos: p})
			h.frame()
			wheel := func(dy float32) {
				h.r.HandleEvent(input.Scroll{Pos: p, DY: dy})
			}
			start := st.Viewport.scrollY()
			for i := range 3 {
				wheel(0.3 / scale)
				label := fmt.Sprintf("subpixel step %d", i+1)
				if i == 1 {
					result := h.requirePartialStep(label)
					if result.frames[0].copies != 1 {
						t.Fatalf("copies = %d, want 1", result.frames[0].copies)
					}
				} else {
					h.ren.SetVerify(false)
					result := h.tracedStep(label)
					h.ren.SetVerify(true)
					if _, shifts := h.lineHints(); len(shifts) != 0 {
						t.Fatalf("subpixel residual emitted a scroll shift: %v", shifts)
					}
					if result.calls != 0 {
						damage, shifts := h.lineHints()
						t.Logf("damage %v shifts %v", damage, shifts)
						t.Fatalf("subpixel residual submitted %d frames, want 0", result.calls)
					}
				}
			}
			logical := (st.Viewport.scrollY() - start) * float64(scale)
			if logical < 0.8999 || logical > 0.9001 {
				t.Fatalf("wheel accumulated %.6f device pixels, want 0.9", logical)
			}
			// The point is 0.95 device pixels above the unscrolled row
			// boundary. It is on the next row after the one-pixel move.
			p.Y -= 0.95 / scale
			if got := h.vw.pointRow(p); got != 17 {
				t.Fatalf("hit row %d, want 17", got)
			}

			for _, dy := range []float32{42.25, -42.25} {
				wheel(dy)
				result := h.requirePartialStep(fmt.Sprintf("scroll %v logical pixels", dy))
				if result.frames[0].copies != 1 {
					t.Fatalf("copies = %d, want 1", result.frames[0].copies)
				}
			}
		})
	}
}
