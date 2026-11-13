package ui

import (
	"fmt"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

func TestScrollHorizontalBarDragAndHover(t *testing.T) {
	var sb *ScrollBox
	d := NewDriver(func(c *Context) {
		sb = ScrollH(c, func() {
			Row(c, func() {}, layout.W(2000), layout.H(40))
		}, layout.W(400))
	}, DriverOptions{Width: 600, Height: 400})
	d.Frame()
	d.Frame()
	r := d.r.c.rects[sb.ID()]
	// Hovering the bar strip widens the thumb (animation off: at once).
	d.Move(geom.Pt(r.X+10, r.Bottom()-4))
	if id, ok := d.r.c.scrollbarAt(d.r.c.pointer); !ok || id != sb.ID()^scrollTag {
		t.Fatal("bar strip not found under the pointer")
	}
	d.Frame()
	wide := false
	for _, op := range d.r.List().Ops {
		if f, ok := op.(paint.FillRect); ok && f.Rect.H > 7.5 && f.Rect.H < 8.5 && f.Rect.W > 50 {
			wide = true
		}
	}
	if !wide {
		t.Fatal("thumb not widened on hover")
	}
	// The thumb is 80px of a 400px track: 320px of travel for 1600px.
	d.Drag(geom.Pt(r.X+10, r.Bottom()-4), geom.Pt(r.X+10+160, r.Bottom()-4))
	if x, _ := sb.Offset(); x < 790 || x > 810 {
		t.Fatalf("drag offset %v want ~800", x)
	}
}

func TestScrollAlwaysShowsBars(t *testing.T) {
	d := driver(func(c *Context) {
		Scroll(c, func() { Column(c, func() {}, layout.H(800)) }, layout.H(100), layout.W(200)).Always()
	})
	d.Frame()
	d.Frame()
	d.Move(geom.Pt(500, 350)) // pointer away from the area
	found := false
	for _, op := range d.r.List().Ops {
		if f, ok := op.(paint.FillRect); ok && f.Rect.W >= 5 && f.Rect.W <= 8 && f.Rect.X > 180 && f.Rect.X < 200 {
			found = true
		}
	}
	if !found {
		t.Fatal("Always: no thumb painted without hover")
	}
}

func TestScrollIntoViewAndReveal(t *testing.T) {
	var sb *ScrollBox
	var ids [40]ID
	target := -1
	d := driver(func(c *Context) {
		sb = Scroll(c, func() {
			for i := range ids {
				ids[i] = Button(c, fmt.Sprintf("b%d", i)).n.id
			}
		}, layout.H(200))
		if target >= 0 {
			c.ScrollIntoView(sb.ID(), ids[target])
		}
	})
	d.Frame()
	d.Frame()
	view := d.r.c.rects[sb.ID()]
	target = 30
	d.Settle()
	d.Frame()
	r := d.r.c.rects[ids[30]]
	if r.Y < view.Y-0.5 || r.Bottom() > view.Bottom()+0.5 || !near(r.Bottom(), view.Bottom()) {
		t.Fatalf("row 30 at %v, view %v", r, view)
	}
	target = 2
	d.Settle()
	d.Frame()
	if r := d.r.c.rects[ids[2]]; !near(r.Y, view.Y) {
		t.Fatalf("row 2 at %v, view %v", r, view)
	}
	target = -1
	sb.Reveal(1000, 20)
	d.Frame()
	if _, y := sb.Offset(); y < 1000+20-200-0.5 {
		t.Fatalf("reveal offset %v", y)
	}
}

func TestVirtualListMillionRowsStayExact(t *testing.T) {
	const n = 1_000_000
	var w *VirtualListW
	jump := -1
	d := driver(func(c *Context) {
		w = VirtualList(c, n, 33.5, func(i int) { Button(c, fmt.Sprintf("r%d", i)) }).Height(335)
		if jump >= 0 {
			w.ScrollToIndex(jump)
			jump = -1
		}
	})
	d.Frame()
	jump = n - 20
	d.Settle()
	d.Frame()
	view := d.r.c.rects[w.ID()]
	// Wheel by single pixels deep in the list: every step must move rows.
	for step := 1; step <= 5; step++ {
		d.Scroll(view.Center(), 1)
		d.Frame()
		off := w.Offset()
		want := float64(n-20)*33.5 + float64(step)
		if off != want {
			t.Fatalf("offset %v want %v", off, want)
		}
		r := mustFind(t, d, fmt.Sprintf("r%d", n-19))
		// Row n-19 sits 33.5px below the top row, minus the step.
		if !near(r.Y, view.Y+33.5-float32(step)) {
			t.Fatalf("step %d: row at %v view %v", step, r.Y, view.Y)
		}
	}
}

func TestWheelForAndInnerScroll(t *testing.T) {
	var zoom float32
	var box *Box
	var inner *ScrollBox
	d := driver(func(c *Context) {
		Scroll(c, func() {
			box = Div(c, layout.H(150))
			box.Body(func() {
				inner = Scroll(c, func() { Column(c, func() {}, layout.H(400)) }, layout.H(60))
			})
			box.Hovered()
			if _, dy, ok := c.WheelFor(box.ID()); ok {
				zoom += dy
			}
			Column(c, func() {}, layout.H(1000))
		}, layout.H(300))
	})
	d.Frame()
	d.Frame()
	br := d.r.c.rects[box.ID()]
	// Over the box (outside the inner scroll): the listener gets the wheel,
	// the page does not scroll.
	d.Scroll(geom.Pt(br.X+10, br.Bottom()-10), 30)
	if zoom != 30 {
		t.Fatalf("wheel for box = %v", zoom)
	}
	// Over the inner scroll: it consumes the wheel first.
	ir := d.r.c.rects[inner.ID()]
	d.Scroll(ir.Center(), 40)
	if _, y := inner.Offset(); y != 40 || zoom != 30 {
		t.Fatalf("inner offset %v zoom %v", y, zoom)
	}
	// Once the inner scroll is at its end the wheel goes to the listener.
	d.Scroll(ir.Center(), 1000)
	d.Scroll(ir.Center(), 5)
	if zoom != 35 {
		t.Fatalf("after inner end zoom %v", zoom)
	}
}

func TestAffixDoesNotSwallowWheel(t *testing.T) {
	var sb *ScrollBox
	var aff *AffixW
	var target ID
	d := navDriver(func(c *Context) {
		sb = Scroll(c, func() {
			Gap(c, 50)
			aff = Affix(c).Target(target).Body(func() { Button(c, "固定按钮") })
			Column(c, func() {}, layout.H(2000))
		}, layout.H(300))
		target = sb.ID()
	})
	d.Frame()
	d.Settle()
	d.Scroll(geom.Pt(300, 150), 200)
	d.Frame()
	d.Frame()
	if !aff.Affixed() {
		t.Fatal("not affixed")
	}
	_, before := sb.Offset()
	r := mustFind(t, d, "固定按钮")
	d.Scroll(r.Center(), 100)
	if _, y := sb.Offset(); y != before+100 {
		t.Fatalf("wheel over pinned content: offset %v -> %v", before, y)
	}
}

func TestSelectLongListKeyboardFollows(t *testing.T) {
	v := ""
	opts := make([]Option, 40)
	for i := range opts {
		opts[i] = Option{Value: fmt.Sprint(i), Label: fmt.Sprintf("选项 %d", i)}
	}
	var w *SelectW
	d := driver(func(c *Context) { w = Select(c, &v, opts) })
	d.Frame()
	d.Click(d.r.c.rects[w.n.id].Center())
	d.Settle()
	panel := d.r.c.rects[w.st.panel]
	if panel.H > 256+8+1 {
		t.Fatalf("dropdown not limited: %v", panel.H)
	}
	for range 15 {
		d.Key(input.KeyDown, 0)
	}
	d.Frame()
	r := mustFind(t, d, "选项 15")
	if r.Bottom() > panel.Bottom()+0.5 || r.Y < panel.Y {
		t.Fatalf("active row %v outside panel %v", r, panel)
	}
	d.Key(input.KeyEnter, 0)
	if v != "15" {
		t.Fatalf("picked %q", v)
	}
	// Reopening reveals the selected value.
	d.Click(d.r.c.rects[w.n.id].Center())
	d.Settle()
	d.Frame()
	if r := mustFind(t, d, "选项 15"); r.Bottom() > d.r.c.rects[w.st.panel].Bottom()+0.5 {
		t.Fatalf("selected row hidden on reopen: %v", r)
	}
}

func TestTextAreaClickBelowTextPutsCaretAtEnd(t *testing.T) {
	v := "ab"
	var w *TextAreaW
	d := driver(func(c *Context) { w = TextArea(c, &v).Rows(4) })
	d.Frame()
	r := d.r.c.rects[w.box.id]
	d.Click(geom.Pt(r.X+40, r.Bottom()-8))
	if d.r.c.focus != w.ID() {
		t.Fatal("not focused")
	}
	d.Type("c")
	if v != "abc" {
		t.Fatalf("typed into %q", v)
	}
}

func TestLabelAlignCenterPerLine(t *testing.T) {
	var lb *Label
	d := driver(func(c *Context) {
		lb = Text(c, "short\nmuch longer second line").Align(AlignCenter)
		lb.n.ln.Width = 400
	})
	d.Frame()
	lay := text.Shape(lb.s, lb.st, text.Options{MaxWidth: 400})
	if len(lay.Lines) != 2 {
		t.Fatalf("lines %d", len(lay.Lines))
	}
	var xs []float32
	for _, op := range d.r.List().Ops {
		if g, ok := op.(paint.Text); ok {
			xs = append(xs, g.Origin.X)
		}
	}
	if len(xs) < 2 {
		t.Fatalf("expected one glyph run per line, got %d", len(xs))
	}
	r := d.r.c.rects[lb.n.id]
	want0 := r.X + (r.W-lay.Lines[0].Width)/2
	want1 := r.X + (r.W-lay.Lines[1].Width)/2
	if !near(xs[0], want0) || !near(xs[1], want1) {
		t.Fatalf("line x %v want %v, %v", xs, want0, want1)
	}
}
