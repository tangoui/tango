package flexbox

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	. "github.com/tangoui/tango/ui/layout"
)

func box(w, h float32) *Node { return &Node{Style: Style{Width: w, Height: h}} }

func eq(t *testing.T, name string, got, want geom.Rect) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %+v want %+v", name, got, want)
	}
}

func TestRowFixedAndGrow(t *testing.T) {
	a, b, c := box(50, 20), &Node{Style: Style{Grow: 1}}, box(30, 10)
	root := &Node{Style: Style{Dir: Row, Gap: 10, Padding: geom.All(5), Align: AlignStart}, Children: []*Node{a, b, c}}
	Solve(root, 300, 100)
	eq(t, "a", a.Rect, geom.R(5, 5, 50, 20))
	eq(t, "b", b.Rect, geom.R(65, 5, 290-50-30-20, 0))
	eq(t, "c", c.Rect, geom.R(300-5-30, 5, 30, 10))
}

func TestColumnGrowUsesZeroBasis(t *testing.T) {
	// One grow child has big content; equal Grow must still split equally.
	big := &Node{Style: Style{Grow: 1}, Measure: func(w, h float32) geom.Size { return geom.Size{W: 10, H: 150} }}
	small := &Node{Style: Style{Grow: 1}}
	root := &Node{Children: []*Node{big, small}}
	Solve(root, 100, 400)
	if big.Rect.H != 200 || small.Rect.H != 200 {
		t.Fatalf("grow split %v %v", big.Rect, small.Rect)
	}
	if big.Rect.W != 100 {
		t.Fatalf("stretch width %v", big.Rect)
	}
}

func TestJustifyAlignCenter(t *testing.T) {
	a := box(20, 20)
	root := &Node{Style: Style{Dir: Row, Justify: JustifyCenter, Align: AlignCenter}, Children: []*Node{a}}
	Solve(root, 100, 60)
	eq(t, "a", a.Rect, geom.R(40, 20, 20, 20))
}

func TestSpaceBetween(t *testing.T) {
	a, b, c := box(10, 10), box(10, 10), box(10, 10)
	root := &Node{Style: Style{Dir: Row, Justify: JustifySpaceBetween}, Children: []*Node{a, b, c}}
	Solve(root, 110, 10)
	if a.Rect.X != 0 || b.Rect.X != 50 || c.Rect.X != 100 {
		t.Fatal(a.Rect, b.Rect, c.Rect)
	}
}

func TestWrap(t *testing.T) {
	var kids []*Node
	for i := 0; i < 5; i++ {
		kids = append(kids, box(40, 10))
	}
	root := &Node{Style: Style{Dir: Row, Wrap: true, Gap: 10, LineGap: 5, Align: AlignStart}, Children: kids}
	sz := SolveAt(root, 100, geom.Inf, true, false)
	if sz.H != 10*3+5*2 {
		t.Fatalf("wrapped height %v", sz)
	}
	eq(t, "k2", kids[2].Rect, geom.R(0, 15, 40, 10))
	eq(t, "k4", kids[4].Rect, geom.R(0, 30, 40, 10))
}

func TestTextLikeHeightDependsOnWidth(t *testing.T) {
	// A paragraph of 300px worth of text, 20px per line.
	para := &Node{Measure: func(w, h float32) geom.Size {
		if geom.IsInf(w) || w >= 300 {
			return geom.Size{W: 300, H: 20}
		}
		lines := int((300 + w - 1) / w)
		return geom.Size{W: w, H: float32(lines) * 20}
	}}
	tail := box(0, 10)
	root := &Node{Style: Style{Padding: geom.All(10)}, Children: []*Node{para, tail}}
	Solve(root, 120, 500)
	if para.Rect.W != 100 || para.Rect.H != 60 {
		t.Fatalf("paragraph %v", para.Rect)
	}
	if tail.Rect.Y != 70 {
		t.Fatalf("tail at %v", tail.Rect)
	}
}

func TestAbsoluteAndStack(t *testing.T) {
	abs := &Node{Style: Style{Absolute: true, Width: 10, Height: 10, Pos: Pos{Left: Unset, Top: 5, Right: 5, Bottom: Unset}}}
	fill := &Node{}
	centered := &Node{Style: Style{Width: 20, Height: 20, AlignSelf: AlignCenter}}
	root := &Node{Style: Style{Dir: Stack}, Children: []*Node{fill, centered, abs}}
	Solve(root, 100, 50)
	eq(t, "fill", fill.Rect, geom.R(0, 0, 100, 50))
	eq(t, "centered", centered.Rect, geom.R(40, 15, 20, 20))
	eq(t, "abs", abs.Rect, geom.R(85, 5, 10, 10))
}

func TestScrollContent(t *testing.T) {
	var kids []*Node
	for i := 0; i < 10; i++ {
		kids = append(kids, box(0, 30))
	}
	sc := &Node{Style: Style{Grow: 1, ScrollY: true}, Children: kids}
	root := &Node{Children: []*Node{box(0, 50), sc}}
	Solve(root, 200, 250)
	if sc.Rect.H != 200 || sc.Content.H != 300 {
		t.Fatalf("scroll %v content %v", sc.Rect, sc.Content)
	}
	if kids[9].Rect.Y != 270 || kids[9].Rect.W != 200 {
		t.Fatalf("last kid %v", kids[9].Rect)
	}
}

func TestShrinkAndMinMax(t *testing.T) {
	a := &Node{Style: Style{Width: 100, Shrink: 1}}
	b := &Node{Style: Style{Width: 100, Shrink: 1, MinW: 80}}
	root := &Node{Style: Style{Dir: Row}, Children: []*Node{a, b}}
	Solve(root, 150, 10)
	if a.Rect.W != 75 || b.Rect.W != 80 {
		t.Fatalf("shrink %v %v", a.Rect, b.Rect)
	}
	g := &Node{Style: Style{Grow: 1, MaxW: 50}}
	h := &Node{Style: Style{Grow: 1}}
	root2 := &Node{Style: Style{Dir: Row}, Children: []*Node{g, h}}
	Solve(root2, 200, 10)
	if g.Rect.W != 50 || h.Rect.W != 150 {
		t.Fatalf("grow with max %v %v", g.Rect, h.Rect)
	}
}

func TestAutoSizeColumnStretchesToWidest(t *testing.T) {
	a, b := &Node{Style: Style{Height: 10}, Measure: func(w, h float32) geom.Size { return geom.Size{W: 40, H: 10} }}, &Node{Measure: func(w, h float32) geom.Size { return geom.Size{W: 80, H: 10} }}
	root := &Node{Children: []*Node{a, b}}
	sz := SolveAt(root, geom.Inf, geom.Inf, false, false)
	if sz.W != 80 || a.Rect.W != 80 {
		t.Fatalf("size %v a %v", sz, a.Rect)
	}
}
