package flexbox

import (
	"fmt"
	"math"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	. "github.com/tangoui/tango/ui/layout"
)

// rtlCase builds a fresh tree for one configuration; rtl only toggles the
// root's RTL flag so the logical solve is identical.
type rtlCase struct {
	dir     Direction
	justify Justify
	align   Align
	wrap    bool
	pad     geom.Insets
	margin  geom.Insets
	abs     bool
	scrollX bool
	noMir   bool // second child is Physical
}

func (c rtlCase) String() string {
	return fmt.Sprintf("dir=%d j=%d a=%d wrap=%v pad=%v m=%v abs=%v sx=%v nomir=%v",
		c.dir, c.justify, c.align, c.wrap, c.pad, c.margin, c.abs, c.scrollX, c.noMir)
}

func (c rtlCase) build(rtl bool) *Node {
	kids := []*Node{
		{Style: Style{Width: 40, Height: 20, Margin: c.margin}},
		{Style: Style{Width: 70, Height: 30, NoMirror: c.noMir}},
		{Style: Style{Grow: 1, MinW: 10, MinH: 10, Margin: c.margin}},
		{Style: Style{Width: 90, Height: 15}},
		// nested box: its own children must not move relative to it
		{Style: Style{Dir: Row, Gap: 3}, Children: []*Node{box(11, 5), box(13, 7)}},
	}
	if c.abs {
		kids = append(kids,
			&Node{Style: Style{Absolute: true, Width: 25, Height: 12, Pos: Pos{Left: 8, Top: 4, Right: Unset, Bottom: Unset}}},
			&Node{Style: Style{Absolute: true, Width: 25, Height: 12, Pos: Pos{Left: Unset, Top: Unset, Right: 6, Bottom: 9}}},
			&Node{Style: Style{Absolute: true, Height: 12, Pos: Pos{Left: 10, Top: Unset, Right: 30, Bottom: 2}}},
		)
	}
	return &Node{Style: Style{
		Dir: c.dir, Justify: c.justify, Align: c.align, Wrap: c.wrap,
		Padding: c.pad, Gap: 5, LineGap: 4, ScrollX: c.scrollX, RTL: rtl,
	}, Children: kids}
}

func TestRTLMirror(t *testing.T) {
	const W, H = 300, 200
	pads := []geom.Insets{{}, {Top: 3, Right: 17, Bottom: 5, Left: 7}}
	margins := []geom.Insets{{}, {Top: 2, Right: 9, Bottom: 1, Left: 4}}
	n := 0
	for _, dir := range []Direction{Row, Column, Stack} {
		for j := JustifyStart; j <= JustifySpaceEvenly; j++ {
			for a := AlignAuto; a <= AlignEnd; a++ {
				for _, wrap := range []bool{false, true} {
					for _, pad := range pads {
						for _, m := range margins {
							for _, abs := range []bool{false, true} {
								for _, sx := range []bool{false, true} {
									for _, nm := range []bool{false, true} {
										c := rtlCase{dir, j, a, wrap, pad, m, abs, sx, nm}
										checkMirror(t, c, W, H)
										n++
									}
								}
							}
						}
					}
				}
			}
		}
	}
	if n == 0 {
		t.Fatal("no cases")
	}
}

func checkMirror(t *testing.T, c rtlCase, w, h float32) {
	t.Helper()
	ltr, rtl := c.build(false), c.build(true)
	Solve(ltr, w, h)
	Solve(rtl, w, h)
	if ltr.Content != rtl.Content {
		t.Fatalf("%v: content %v vs %v", c, ltr.Content, rtl.Content)
	}
	span := float32(w)
	if c.scrollX {
		span = max(w, ltr.Content.W)
	}
	for i := range ltr.Children {
		l, r := ltr.Children[i].Rect, rtl.Children[i].Rect
		want := span - l.X - l.W
		if rtl.Children[i].NoMirror {
			want = l.X
		}
		if math.Abs(float64(r.X-want)) > 1e-3 || r.Y != l.Y || r.W != l.W || r.H != l.H {
			t.Fatalf("%v: child %d ltr=%+v rtl=%+v wantX=%v", c, i, l, r, want)
		}
		// Grandchildren keep their relative layout (child itself is LTR).
		for k := range ltr.Children[i].Children {
			if g, gr := ltr.Children[i].Children[k].Rect, rtl.Children[i].Children[k].Rect; g != gr {
				t.Fatalf("%v: grandchild %d/%d %+v vs %+v", c, i, k, g, gr)
			}
		}
	}
}

func TestRTLExamples(t *testing.T) {
	// Row: first child at the right edge, padding-left acts as start.
	a, b := box(50, 10), box(30, 10)
	root := &Node{Style: Style{Dir: Row, Gap: 10, Padding: geom.Insets{Left: 5, Right: 2}, Align: AlignStart, RTL: true}, Children: []*Node{a, b}}
	Solve(root, 200, 50)
	eq(t, "a", a.Rect, geom.R(200-5-50, 0, 50, 10))
	eq(t, "b", b.Rect, geom.R(200-5-50-10-30, 0, 30, 10))

	// Abs(left=8) becomes 8 from the right; Physical keeps it on the left.
	p := &Node{Style: Style{Absolute: true, Width: 20, Height: 20, Pos: Pos{Left: 8, Top: 0, Right: Unset, Bottom: Unset}}}
	q := &Node{Style: Style{Absolute: true, Width: 20, Height: 20, Pos: Pos{Left: 8, Top: 0, Right: Unset, Bottom: Unset}, NoMirror: true}}
	root = &Node{Style: Style{RTL: true}, Children: []*Node{p, q}}
	Solve(root, 100, 100)
	eq(t, "abs", p.Rect, geom.R(72, 0, 20, 20))
	eq(t, "physical", q.Rect, geom.R(8, 0, 20, 20))

	// ScrollX: logical start is the right end of the content.
	x, y := box(80, 10), box(80, 10)
	root = &Node{Style: Style{Dir: Row, ScrollX: true, RTL: true}, Children: []*Node{x, y}}
	Solve(root, 100, 10)
	eq(t, "sx0", x.Rect, geom.R(80, 0, 80, 10))
	eq(t, "sx1", y.Rect, geom.R(0, 0, 80, 10))

	// Nested RTL: inner node mirrors its own children in its own box.
	i1, i2 := box(10, 10), box(20, 10)
	inner := &Node{Style: Style{Dir: Row, Width: 60, RTL: true, Align: AlignStart}, Children: []*Node{i1, i2}}
	root = &Node{Style: Style{Dir: Row, RTL: true, Align: AlignStart}, Children: []*Node{inner}}
	Solve(root, 100, 10)
	eq(t, "inner", inner.Rect, geom.R(40, 0, 60, 10))
	eq(t, "i1", i1.Rect, geom.R(50, 0, 10, 10))
	eq(t, "i2", i2.Rect, geom.R(30, 0, 20, 10))
}

func TestPhysicalOption(t *testing.T) {
	var s Style
	Apply(&s, Physical(), PadSE(1, 2, 3))
	if !s.NoMirror || s.Padding != (geom.Insets{Top: 1, Right: 3, Bottom: 1, Left: 2}) {
		t.Fatalf("%+v", s)
	}
}

func BenchmarkSolveLTR(b *testing.B) {
	c := rtlCase{dir: Row, wrap: true, abs: true}
	root := c.build(false)
	for i := 0; i < b.N; i++ {
		Solve(root, 300, 200)
	}
}
