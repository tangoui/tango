package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

func moreCountNodes(n *node) int {
	k := 1
	for _, c := range n.kids {
		k += moreCountNodes(c)
	}
	return k
}

func TestMoreVirtualListRangeScrollJump(t *testing.T) {
	const total = 100000
	clicked := -1
	jump, into := -1, -1
	var w *VirtualListW
	d := driver(func(c *Context) {
		Column(c, func() {
			w = VirtualList(c, total, 32, func(i int) {
				if Button(c, fmt.Sprintf("第 %d 项", i)).Clicked() {
					clicked = i
				}
			}).Height(320)
			if jump >= 0 {
				w.ScrollToIndex(jump)
				jump = -1
			}
			if into >= 0 {
				w.ScrollIntoView(into)
				into = -1
			}
		}, layout.Pad(20))
	})
	d.Frame()
	first, end := w.Range()
	if first != 0 || end != 13 {
		t.Fatalf("initial range %d..%d", first, end)
	}
	if n := moreCountNodes(d.r.c.root); n > 60 {
		t.Fatalf("built %d nodes for %d items", n, total)
	}
	view := d.r.c.rects[w.ID()]
	if view.H != 320 {
		t.Fatalf("viewport %v", view)
	}
	d.Scroll(geom.Pt(100, 100), 3200)
	if first, _ := w.Range(); first != 97 {
		t.Fatalf("after scroll first=%d", first)
	}
	d.ClickLabel("第 101 项")
	if clicked != 101 {
		t.Fatalf("click row: %d", clicked)
	}
	jump = 50000
	d.Settle()
	d.Frame()
	r := mustFind(t, d, "第 50000 项")
	if r.Y < view.Y-1 || r.Y > view.Y+1 {
		t.Fatalf("jump: row at %v, view %v", r, view)
	}
	// Past the end clamps to the last page.
	jump = total + 10
	d.Settle()
	d.Frame()
	last := mustFind(t, d, fmt.Sprintf("第 %d 项", total-1))
	if last.Bottom() > view.Bottom()+1 || last.Bottom() < view.Bottom()-1 {
		t.Fatalf("last row %v view %v", last, view)
	}
	if w.Offset() != total*32-320 {
		t.Fatalf("clamped offset %v", w.Offset())
	}
	// ScrollIntoView moves only as far as needed.
	into = total - 20
	d.Settle()
	d.Frame()
	if w.Offset() != float64(total-20)*32 {
		t.Fatalf("into view (above) offset %v", w.Offset())
	}
	into = total - 20 + 12
	d.Settle()
	d.Frame()
	if w.Offset() != float64(total-20+13)*32-320 {
		t.Fatalf("into view (below) offset %v", w.Offset())
	}
}

func TestMoreVirtualListFastAndThemes(t *testing.T) {
	items := make([]string, 100000)
	for i := range items {
		items[i] = fmt.Sprintf("记录 %06d", i)
	}
	d := NewDriver(func(c *Context) {
		VirtualList(c, len(items), 32, func(i int) { Text(c, items[i]) }, layout.H(600))
	}, DriverOptions{Width: 600, Height: 600})
	d.Frame()
	start := time.Now()
	const frames = 60
	for i := 0; i < frames; i++ {
		d.Scroll(geom.Pt(100, 100), 1777)
	}
	per := time.Since(start) / frames
	t.Logf("%v per scrolled frame (100k items)", per)
	if per > 100*time.Millisecond {
		t.Fatalf("too slow: %v per frame", per)
	}
	d.SetTheme(theme.ElementPlus())
	d.Settle()
	if n := moreCountNodes(d.r.c.root); n > 80 {
		t.Fatalf("element build %d nodes", n)
	}
	// Empty list builds nothing and does not panic.
	d2 := driver(func(c *Context) { VirtualList(c, 0, 32, func(int) { t.Fatal("built row") }).Height(100) })
	d2.Frame()
	d2.Scroll(geom.Pt(10, 10), 100)
}

func TestMoreVirtualListScrollbarDrag(t *testing.T) {
	var w *VirtualListW
	d := driver(func(c *Context) {
		w = VirtualList(c, 100000, 32, func(i int) { Text(c, fmt.Sprintf("%d", i)) }).Height(320)
	})
	d.Frame()
	d.Frame()
	view := d.r.c.rects[w.ID()]
	x := view.Right() - 4
	// Drag the thumb (at the top) to the bottom of the track.
	d.Inject(input.Pointer{Kind: input.Move, Pos: geom.Pt(x, view.Y+5)})
	d.Inject(input.Pointer{Kind: input.Press, Pos: geom.Pt(x, view.Y+5)})
	d.Frame()
	d.Inject(input.Pointer{Kind: input.Move, Pos: geom.Pt(x, view.Y+view.H/2)})
	d.Frame()
	mid := w.Offset()
	total := float64(100000*32) - 320
	if mid < total*0.4 || mid > total*0.6 {
		t.Fatalf("half-way drag offset %v of %v", mid, total)
	}
	if first, _ := w.Range(); first < 40000 {
		t.Fatalf("rows not rebuilt after drag: first=%d", first)
	}
	d.Inject(input.Pointer{Kind: input.Move, Pos: geom.Pt(x, view.Bottom()+50)})
	d.Inject(input.Pointer{Kind: input.Release, Pos: geom.Pt(x, view.Bottom()+50)})
	d.Frame()
	if w.Offset() != total {
		t.Fatalf("drag past end offset %v want %v", w.Offset(), total)
	}
	// Track click jumps the thumb to the pointer.
	d.Click(geom.Pt(x, view.Y+view.H/4))
	if off := w.Offset(); off <= 0 || off >= total*0.5 {
		t.Fatalf("track click offset %v", off)
	}
}
