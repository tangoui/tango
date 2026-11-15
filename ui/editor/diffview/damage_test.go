package diffview

import (
	"fmt"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/diff"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// TestDiffViewDamageVerified runs a diff view script through a GPU
// renderer on cpuexec in strict verify mode, so that both editors (and the
// revert arrows and the merged overview ruler) are checked at once: every
// partial frame must equal a full redraw (docs/specs/diff-view.md, 4 C1).
// It types into the modified side, lets the debounced result arrive,
// scrolls both sides (the other one follows) and navigates between
// changes.
func TestDiffViewDamageVerified(t *testing.T) {
	useTestText(t)
	for _, scale := range []float32{1, 1.25, 2} {
		t.Run(fmt.Sprintf("scale%v", scale), func(t *testing.T) {
			orig := docLines(160)
			d := newTestDiff(t, orig, modify(orig, 12))
			clk := &fakeClock{}
			d.run.clk = clk
			d.Options.UseInlineWhenSpaceIsLimited = false
			h := newHost(t, d, geom.Size{W: 900, H: 420}, scale, true)
			h.settle()
			step := func(what string) {
				t.Helper()
				if !h.frame() && testing.Verbose() {
					t.Logf("full frame: %s", what)
				}
			}
			h.clickLine(diff.Modified, 5)
			h.frame()
			h.frame()

			// Typing: the result is mapped through the edits.
			for i := range 8 {
				h.typeText(string(rune('a' + i)))
				step("type")
			}
			h.key(input.KeyEnter, 0)
			step("enter")
			h.typeText("new line")
			step("type")
			if _, ok := d.Result(); ok {
				t.Fatal("result up to date while the recomputation is due")
			}
			// The debounced result arrives.
			clk.Advance(time.Second)
			d.run.wait()
			step("result")
			if _, ok := d.Result(); !ok {
				t.Fatal("result not up to date")
			}
			checkAligned(t, d, "after result")

			// Scrolling either side; the other follows. (Moving the
			// pointer from one editor to the other redraws both: the
			// hovered widget changes.)
			for i, dy := range []float32{8, 16, 4, 2.4, 40, -8, 21, -2.4, 6, 120, 33, 12} {
				s := diff.Modified
				if i >= 6 {
					s = diff.Original
				}
				h.wheel(s, dy)
				step(fmt.Sprintf("scroll %v", dy))
				checkAligned(t, d, "scroll")
			}

			// Navigation.
			for range 4 {
				if !d.Next() {
					t.Fatal("Next")
				}
				step("next")
				checkAligned(t, d, "next")
			}
			for range 2 {
				h.key(input.KeyF5, input.ModAlt|input.ModShift)
				step("prev key")
			}
			// At 1.25 the splitter puts the editors at fractional device
			// pixels, where scrolled pixels cannot be copied: only the
			// verification counts there.
			if scale != 1.25 && h.parts*2 < h.frames {
				t.Fatalf("%d of %d frames partial", h.parts, h.frames)
			}
			t.Logf("%d of %d frames partial", h.parts, h.frames)
		})
	}
}
