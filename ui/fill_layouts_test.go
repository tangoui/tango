package ui

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

func TestFillMasonryPlace(t *testing.T) {
	col, top, colH := fillMasonryPlace([]float32{100, 50, 70, 30, 40}, 3, 10, nil)
	wantCol := []int{0, 1, 2, 1, 2}
	wantTop := []float32{0, 0, 0, 60, 80}
	for i := range col {
		if col[i] != wantCol[i] || top[i] != wantTop[i] {
			t.Fatalf("item %d: col %d top %v", i, col[i], top[i])
		}
	}
	if colH[0] != 110 || colH[1] != 100 || colH[2] != 130 {
		t.Fatalf("colH = %v", colH)
	}
	// Pinned items keep their column.
	col, _, _ = fillMasonryPlace([]float32{10, 10, 10}, 3, 0, func(i int) int {
		if i == 2 {
			return 0
		}
		return -1
	})
	if col[2] != 0 {
		t.Fatalf("pin ignored: %v", col)
	}
}

func TestMasonryLayoutAndVirtual(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		hs := []float32{100, 50, 70, 30, 40, 60}
		var m, v *MasonryW
		d := fillDriver(th, func(c *Context) {
			m = Masonry(c, len(hs), func(i int) { Text(c, fmt.Sprintf("tile %d", i)) }).
				Columns(3).Gutter(10).ItemHeight(func(i int) float32 { return hs[i] }).Show()
			v = Masonry(c, 100000, func(i int) {
				Column(c, func() { Text(c, fmt.Sprintf("v%d", i)) })
			}).Columns(4).Gutter(8).Virtual(300).Show()
		})
		a := m.Assignment()
		if a[3] != 1 || a[4] != 2 || a[5] != 1 {
			t.Fatalf("assignment = %v", a)
		}
		r0, _ := d.FindText("tile 0")
		r1, _ := d.FindText("tile 1")
		r3, _ := d.FindText("tile 3")
		if r1.X <= r0.X || math.Abs(float64(r3.X-r1.X)) > 1 || r3.Y < r1.Y+59 {
			t.Fatalf("positions %v %v %v", r0, r1, r3)
		}
		if v.Built() > 200 {
			t.Fatalf("virtual built %d items", v.Built())
		}
		if _, ok := d.FindText("v0"); !ok {
			t.Fatal("first virtual item missing")
		}
		// Scroll far: later items appear, the first one goes away.
		vr, _ := d.FindText("v0")
		for i := 0; i < 40; i++ {
			d.Scroll(vr.Center(), 400)
		}
		if _, ok := d.FindText("v0"); ok {
			t.Fatal("scrolled-out item still built")
		}
	})
}

func TestListyGroupsStickyScrollLoadMore(t *testing.T) {
	fillThemes(t, func(t *testing.T, th func() *theme.Theme) {
		names := []string{"apple", "banana", "avocado", "blueberry", "cherry"}
		n := 3000
		jump := ""
		hasMore, loading := true, false
		reached := 0
		var l *ListyW
		d := fillDriver(th, func(c *Context) {
			l = Listy(c, n, func(i int) { Text(c, fmt.Sprintf("%s %d", names[i%len(names)], i)) }).
				Group(func(i int) string { return names[i%len(names)][:1] }, nil).
				Sticky().Virtual().Height(300).LoadMore(hasMore, loading)
			if jump != "" {
				l.ScrollToGroup(jump, AlignTop, 0)
				jump = ""
			}
			l.Show()
			if l.EndReached() {
				reached++
			}
		})
		// Grouped by first letter in order of first appearance: a, b, c.
		if l.groups[0] != "a" || l.groups[1] != "b" || l.groups[2] != "c" || l.counts[0] != 1200 {
			t.Fatalf("groups %v counts %v", l.groups, l.counts)
		}
		if f, e := l.Range(); e-f > 20 {
			t.Fatalf("built %d rows", e-f)
		}
		jump = "b"
		d.Settle()
		d.Settle()
		r, ok := d.FindText("banana 1")
		if !ok {
			t.Fatal("group b not scrolled to")
		}
		// Scroll a little inside group b: the sticky "b" header stays on top.
		d.Scroll(r.Center(), 200)
		hr, ok := d.FindText("b")
		vp := d.r.c.rects[l.ID()]
		if !ok || hr.Y < vp.Y || hr.Bottom() > vp.Y+l.headerH() {
			t.Fatalf("sticky header at %v, viewport %v", hr, vp)
		}
		// Scroll to the end: EndReached fires.
		for i := 0; i < 400 && reached == 0; i++ {
			d.Scroll(r.Center(), 2000)
		}
		if reached == 0 {
			t.Fatal("EndReached never fired")
		}
		loading = true
		d.Settle()
		if _, ok := d.FindText(ZhCN.Loading); !ok {
			t.Fatal("loading footer missing")
		}
	})
}

func TestListyMeasuredHeightsAndClick(t *testing.T) {
	clicked := -1
	d := fillDriver(func() *theme.Theme { return theme.AntDesign() }, func(c *Context) {
		l := Listy(c, 50, func(i int) {
			Text(c, fmt.Sprintf("row %d", i))
			if i%2 == 0 {
				Text(c, "second line")
			}
		}).RowKey(func(i int) any { return fmt.Sprint("k", i) }).Height(400).Virtual().Show()
		if i, ok := l.Clicked(); ok {
			clicked = i
		}
	})
	d.Settle()
	r0, _ := d.FindText("row 0")
	r1, _ := d.FindText("row 1")
	r2, _ := d.FindText("row 2")
	if (r1.Y - r0.Y) <= (r2.Y - r1.Y) {
		t.Fatalf("variable heights not honoured: %v %v %v", r0, r1, r2)
	}
	d.Click(r1.Center())
	if clicked != 1 {
		t.Fatalf("clicked %d", clicked)
	}
}

func TestFillPerimPoint(t *testing.T) {
	r := geom.Rect{X: 0, Y: 0, W: 100, H: 50}
	L := fillPerimeter(r, 10)
	want := 2*(100+50) - 80 + 2*math.Pi*10
	if math.Abs(L-want) > 1e-3 {
		t.Fatalf("perimeter %v want %v", L, want)
	}
	for _, tc := range []struct {
		s    float64
		x, y float32
	}{{0, 10, 0}, {80, 90, 0}, {80 + math.Pi*5, 100, 10}, {L, 10, 0}} {
		p := fillPerimPoint(r, 10, tc.s)
		if math.Abs(float64(p.X-tc.x)) > 0.01 || math.Abs(float64(p.Y-tc.y)) > 0.01 {
			t.Fatalf("s=%v: %v want (%v,%v)", tc.s, p, tc.x, tc.y)
		}
	}
	c := fillBeamColor([]BeamStop{{theme.Hex("#ff0000"), 0}, {theme.Hex("#0000ff"), 100}}, 1)
	if c.B != 255 || c.A != 255 {
		t.Fatalf("head color %v", c)
	}
	if fillBeamColor([]BeamStop{{theme.Hex("#ff0000"), 0}}, 0).A != 0 {
		t.Fatal("tail must be transparent")
	}
}

func TestBorderBeamMotion(t *testing.T) {
	page := func(c *Context) {
		BorderBeam(c).Body(func() { Card(c, "beam").Body(func() { Text(c, "x") }) })
	}
	// NoAnimations (Driver default): drawn still, no frames requested.
	d := NewDriver(page, DriverOptions{Width: 400, Height: 300})
	d.Settle()
	if d.NeedsFrame() {
		t.Fatal("still beam must not request frames")
	}
	still := d.Frame()
	// Animations on: frames keep coming and the beam moves.
	a := NewDriver(page, DriverOptions{Width: 400, Height: 300, Animations: true})
	a.Frame()
	if !a.NeedsFrame() {
		t.Fatal("moving beam should request frames")
	}
	img1 := append([]byte(nil), a.Frame().Pix...)
	a.Advance(500 * time.Millisecond)
	img2 := a.Frame().Pix
	if string(img1) == string(img2) {
		t.Fatal("beam did not move")
	}
	// ReduceMotion: hidden, no frames.
	rm := NewDriver(page, DriverOptions{Width: 400, Height: 300, Animations: true, ReduceMotion: true})
	rm.Settle()
	if rm.NeedsFrame() {
		t.Fatal("reduced motion must stand still")
	}
	if string(rm.Frame().Pix) == string(still.Pix) {
		t.Fatal("reduced motion should hide the beam (image equals the still beam)")
	}
}

// BenchmarkListy100k measures a frame of a grouped, sticky 100k-row
// Listy with measured heights while scrolling.
func BenchmarkListy100k(b *testing.B) {
	d := NewDriver(func(c *Context) {
		Listy(c, 100000, func(i int) { Text(c, fmt.Sprintf("row %d", i)) }).
			Group(func(i int) string { return string(rune('A' + i%26)) }, nil).Sticky().Virtual().Height(500).Show()
	}, DriverOptions{Width: 900, Height: 700})
	d.Settle()
	r, ok := d.FindText("row 0")
	if !ok {
		b.Fatal("no row")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dy := float32(60)
		if i%200 >= 100 {
			dy = -60
		}
		d.Inject(input.Scroll{Pos: r.Center(), DY: dy})
		d.Frame()
	}
}

// BenchmarkMasonry100k measures a frame of a virtual 100k-item masonry
// while scrolling.
func BenchmarkMasonry100k(b *testing.B) {
	d := NewDriver(func(c *Context) {
		Masonry(c, 100000, func(i int) { Text(c, fmt.Sprintf("m%d", i)) }).
			ItemHeight(func(i int) float32 { return float32(40 + i%5*20) }).Columns(5).Gutter(8).Virtual(500).Show()
	}, DriverOptions{Width: 900, Height: 700})
	d.Settle()
	r, ok := d.FindText("m0")
	if !ok {
		b.Fatal("no item")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dy := float32(60)
		if i%200 >= 100 {
			dy = -60
		}
		d.Inject(input.Scroll{Pos: r.Center(), DY: dy})
		d.Frame()
	}
}
