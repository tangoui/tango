package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// TestSplitterLazyAndDragEvents: a lazy drag keeps the panes until the
// release commits the clamped ratio; DragStarted / DragEnded fire once.
func TestSplitterLazyAndDragEvents(t *testing.T) {
	var s *SplitterW
	var started, ended, changed int
	var lastStart, lastEnd int
	lazy := true
	d := navDriver(func(c *Context) {
		s = Splitter(c).Style(layout.W(606), layout.H(100))
		if lazy {
			s.Lazy()
		}
		s.Body(func() {
			SplitterPanel(c).Body(func() {})
			SplitterPanel(c).Min(100).Body(func() {})
		})
		if b, ok := s.DragStarted(); ok {
			started++
			lastStart = b
		}
		if b, ok := s.DragEnded(); ok {
			ended++
			lastEnd = b
		}
		if s.Changed() {
			changed++
		}
	})
	d.Frame()
	d.Frame()
	bar := mustFind(t, d, "splitter-bar-1")
	from := bar.Center()
	d.Inject(input.Pointer{Kind: input.Press, Pos: from, Button: input.ButtonLeft})
	d.Frame()
	if started != 1 || lastStart != 1 || !s.Dragging() {
		t.Fatalf("drag start: started=%d bar=%d dragging=%v", started, lastStart, s.Dragging())
	}
	d.Move(geom.Pt(from.X+100, from.Y))
	if f := s.Sizes(); !near(f[0]*600, 300) || changed != 0 {
		t.Fatalf("lazy drag moved the panes: %v changed=%d", f, changed)
	}
	if !s.st.previewOn || !near(s.st.preview*600, 400) {
		t.Fatalf("preview = %v on=%v", s.st.preview*600, s.st.previewOn)
	}
	// Past the second pane's minimum the preview stops at the limit.
	d.Move(geom.Pt(from.X+400, from.Y))
	if !near(s.st.preview*600, 500) {
		t.Fatalf("preview not clamped: %v", s.st.preview*600)
	}
	d.Inject(input.Pointer{Kind: input.Release, Pos: geom.Pt(from.X+400, from.Y), Button: input.ButtonLeft})
	d.Frame()
	d.Frame()
	if f := s.Sizes(); !near(f[0]*600, 500) || !near(f[1]*600, 100) {
		t.Fatalf("release did not commit: %v", f)
	}
	if ended != 1 || lastEnd != 1 || changed != 1 || started != 1 || s.Dragging() {
		t.Fatalf("events: started=%d ended=%d changed=%d dragging=%v", started, ended, changed, s.Dragging())
	}

	// A cancelled lazy drag commits nothing but still ends.
	bar = mustFind(t, d, "splitter-bar-1")
	from = bar.Center()
	d.Inject(input.Pointer{Kind: input.Press, Pos: from, Button: input.ButtonLeft})
	d.Frame()
	d.Move(geom.Pt(from.X-200, from.Y))
	d.Inject(input.Pointer{Kind: input.Cancel, Pos: geom.Pt(from.X-200, from.Y)})
	d.Frame()
	d.Frame()
	if f := s.Sizes(); !near(f[0]*600, 500) {
		t.Fatalf("cancelled drag committed: %v", f)
	}
	if started != 2 || ended != 2 {
		t.Fatalf("cancel events: started=%d ended=%d", started, ended)
	}

	// Without Lazy the ratio follows the pointer and the events still fire.
	lazy = false
	d.Frame()
	bar = mustFind(t, d, "splitter-bar-1")
	navDrag(d, bar.Center(), geom.Pt(bar.Center().X-200, bar.Center().Y))
	d.Frame()
	if f := s.Sizes(); !near(f[0]*600, 300) {
		t.Fatalf("live drag: %v", f)
	}
	if started != 3 || ended != 3 {
		t.Fatalf("live events: started=%d ended=%d", started, ended)
	}
}

// TestSplitterCollapseToggle: a CollapseToggle pane gets one arrow on its
// bar (like an IDE side bar); the arrow shows a tip on hover, collapses
// the pane and, once collapsed, restores it with the other tip.
func TestSplitterCollapseToggle(t *testing.T) {
	var s *SplitterW
	d := navDriver(func(c *Context) {
		s = Splitter(c).Style(layout.W(606), layout.H(200))
		s.Body(func() {
			SplitterPanel(c).DefaultSize(200).CollapseToggle("折叠工程侧栏", "展开工程侧栏").Body(func() {})
			SplitterPanel(c).Body(func() {})
		})
	})
	d.Frame()
	d.Frame()
	if _, ok := d.Find("collapse-start"); ok {
		t.Fatal("toggle mode still shows the two-arrow pair")
	}
	arrow := mustFind(t, d, "collapse-toggle")
	if _, ok := d.FindText("折叠工程侧栏"); ok {
		t.Fatal("tip shown before hover")
	}
	d.Move(arrow.Center())
	d.Frame()
	if _, ok := d.FindText("折叠工程侧栏"); !ok {
		t.Fatal("no collapse tip on hover")
	}
	before := s.Sizes()[0]
	d.Click(arrow.Center())
	if f := s.Sizes(); f[0] != 0 {
		t.Fatalf("not collapsed: %v", f)
	}
	arrow = mustFind(t, d, "collapse-toggle")
	d.Move(arrow.Center())
	d.Frame()
	if _, ok := d.FindText("展开工程侧栏"); !ok {
		t.Fatal("no expand tip once collapsed")
	}
	d.Click(arrow.Center())
	if f := s.Sizes(); f[0] != before {
		t.Fatalf("not restored to %v: %v", before, f)
	}
}

// TestSplitterToggleIcon: the arrow points at the pane it collapses and
// flips once that pane is collapsed.
func TestSplitterToggleIcon(t *testing.T) {
	cases := []struct {
		vertical, start, collapsed bool
		want                       *icon.Icon
	}{
		{false, true, false, icon.CaretLeft},
		{false, true, true, icon.CaretRight},
		{false, false, false, icon.CaretRight},
		{false, false, true, icon.CaretLeft},
		{true, true, false, icon.CaretUp},
		{true, true, true, icon.CaretDown},
		{true, false, false, icon.CaretDown},
		{true, false, true, icon.CaretUp},
	}
	for _, tc := range cases {
		if got := splitterToggleIcon(tc.vertical, tc.start, tc.collapsed); got != tc.want {
			t.Errorf("vertical=%v start=%v collapsed=%v: wrong icon", tc.vertical, tc.start, tc.collapsed)
		}
	}
}

// TestA11ySplitterCollapseToggle: the toggle is one button named by the
// tips; Enter on the bar toggles the toggle pane.
func TestA11ySplitterCollapseToggle(t *testing.T) {
	var sp *SplitterW
	d := NewDriver(func(c *Context) {
		sp = Splitter(c).Style(layout.W(606), layout.H(200))
		sp.Body(func() {
			SplitterPanel(c).Body(func() { Text(c, "L") })
			SplitterPanel(c).DefaultPercent(0.3).CollapseToggle("折叠大纲", "展开大纲").Body(func() { Text(c, "R") })
		})
	}, DriverOptions{Width: 700, Height: 300})
	d.EnableAccessibility()
	d.Settle()
	d.Settle()
	mustNode(t, d, a11y.RoleButton, "折叠大纲")
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyEnter, 0)
	if f := sp.Sizes(); f[1] > 0.001 {
		t.Fatalf("Enter did not collapse the toggle pane: %v", f)
	}
	mustNode(t, d, a11y.RoleButton, "展开大纲")
	d.Key(input.KeyEnter, 0)
	if f := sp.Sizes(); !near(f[1]*600, 180) {
		t.Fatalf("Enter did not restore: %v", f)
	}
}

// TestSplitterDefaultSizeUsesOwnWidth: DefaultSize is measured against the
// splitter's own length, not the window's (the first frame has no rect yet).
func TestSplitterDefaultSizeUsesOwnWidth(t *testing.T) {
	var s *SplitterW
	var pane *Box
	d := navDriver(func(c *Context) { // window 960 wide, splitter 606
		s = Splitter(c).Style(layout.W(606), layout.H(100))
		s.Body(func() {
			SplitterPanel(c).DefaultSize(200).Body(func() {
				pane = Column(c, func() {}, layout.Grow(1))
			})
			SplitterPanel(c).Body(func() {})
		})
	})
	d.Settle()
	if f := s.Sizes(); !near(f[0]*600, 200) {
		t.Fatalf("DefaultSize(200) gave fraction %v (%vpx of 600)", f[0], f[0]*600)
	}
	if w := pane.Rect().W; !near(w, 200) {
		t.Fatalf("pane width %v, want 200", w)
	}
	if d.NeedsFrame() {
		t.Fatal("splitter keeps asking for frames once laid out")
	}
}

// TestSplitterToggleArrowInsideWhenCollapsed: once its pane is collapsed
// the bar sits on the splitter's edge; the toggle arrow must stay fully
// inside a clipping splitter (start / end pane, both axes, RTL).
func TestSplitterToggleArrowInsideWhenCollapsed(t *testing.T) {
	for _, tc := range []struct {
		name            string
		vertical, start bool
		rtl             bool
	}{
		{"h-start", false, true, false},
		{"h-end", false, false, false},
		{"v-start", true, true, false},
		{"v-end", true, false, false},
		{"h-start-rtl", false, true, true},
		{"h-end-rtl", false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var s *SplitterW
			d := navDriver(func(c *Context) {
				build := func() {
					s = Splitter(c).Style(layout.W(606), layout.H(200))
					if tc.vertical {
						s.Vertical()
					}
					s.Body(func() {
						p0 := SplitterPanel(c)
						p1 := SplitterPanel(c)
						if tc.start {
							p0.CollapseToggle("", "")
						} else {
							p1.CollapseToggle("", "")
						}
						p0.Body(func() {})
						p1.Body(func() {})
					}).Clip()
				}
				if tc.rtl {
					ConfigProvider(c).Direction(RTL).Body(build)
					return
				}
				build()
			})
			d.Settle()
			d.Click(mustFind(t, d, "collapse-toggle").Center())
			d.Settle()
			if f := s.Sizes(); (tc.start && f[0] != 0) || (!tc.start && f[1] != 0) {
				t.Fatalf("not collapsed: %v", f)
			}
			a := mustFind(t, d, "collapse-toggle") // visible (clipped) hit rect
			full := float32(16)
			got := a.W
			if tc.vertical {
				got = a.H
			}
			if !near(got, full) {
				t.Fatalf("arrow clipped: visible %v of %v (%v)", got, full, a)
			}
			// It still restores the pane.
			d.Click(a.Center())
			d.Settle()
			if f := s.Sizes(); f[0] == 0 || f[1] == 0 {
				t.Fatalf("not restored: %v", f)
			}
		})
	}
}
