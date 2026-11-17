package ui

import (
	"math"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// pathBounds returns the bounding box of every FillPath in l.
func arrowPathBounds(t *testing.T, l *paint.List) geom.Rect {
	t.Helper()
	minX, minY := float32(math.Inf(1)), float32(math.Inf(1))
	maxX, maxY := float32(math.Inf(-1)), float32(math.Inf(-1))
	n := 0
	for _, op := range l.Ops {
		f, ok := op.(paint.FillPath)
		if !ok {
			continue
		}
		b := f.Path.Bounds()
		minX, minY = min(minX, b.X), min(minY, b.Y)
		maxX, maxY = max(maxX, b.Right()), max(maxY, b.Bottom())
		n++
	}
	if n == 0 {
		t.Fatal("nothing painted")
	}
	return geom.Rect{X: minX, Y: minY, W: maxX - minX, H: maxY - minY}
}

// TestAntdCSSChevronGeometry pins the shape antd draws with CSS borders: an
// L of two strokes `size` long and `width` thick rotated to point a way
// (date-picker/style/panel.js prev/next icons: 7px, 1.5px; carousel
// slick-prev/next: 16/√2, 2px). The painted box is the rotated L, so a
// left-pointing chevron is taller than wide and its tip is at the left.
func TestAntdCSSChevronGeometry(t *testing.T) {
	col := theme.Hex("#000000")
	center := geom.Pt(50, 50)
	for _, tc := range []struct {
		name        string
		dir         chevronDir
		size, width float32
	}{
		{"picker prev", chevronLeft, 7, 1.5},
		{"picker next", chevronRight, 7, 1.5},
		{"carousel up", chevronUp, 16 / math.Sqrt2, 2},
		{"carousel down", chevronDown, 16 / math.Sqrt2, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var l paint.List
			antdCSSChevron(&l, center, tc.size, tc.width, tc.dir, col)
			b := arrowPathBounds(t, &l)
			// The borders lie inside the border-box, so the rotated L spans
			// the box diagonal (size·√2) across the pointing axis and
			// (size+width)/√2 along it.
			across := tc.size * math.Sqrt2
			along := (tc.size + tc.width) / math.Sqrt2
			w, h := b.W, b.H
			if tc.dir == chevronLeft || tc.dir == chevronRight {
				w, h = h, w
			}
			if math.Abs(float64(w-across)) > 0.6 || math.Abs(float64(h-along)) > 0.6 {
				t.Fatalf("box %vx%v, want about %vx%v (across x along)", b.W, b.H, across, along)
			}
			// The tip points the right way.
			mid := b.X + b.W/2
			midY := b.Y + b.H/2
			switch tc.dir {
			case chevronLeft:
				if mid > center.X+0.6 {
					t.Fatalf("left chevron box centered right of the anchor: %v", b)
				}
			case chevronRight:
				if mid < center.X-0.6 {
					t.Fatalf("right chevron box centered left of the anchor: %v", b)
				}
			case chevronUp:
				if midY > center.Y+0.6 {
					t.Fatalf("up chevron box below the anchor: %v", b)
				}
			case chevronDown:
				if midY < center.Y-0.6 {
					t.Fatalf("down chevron box above the anchor: %v", b)
				}
			}
		})
	}
}

// TestAntdSubmenuArrowBars pins antd's inline submenu arrow (menu/style
// index.js genSubMenuArrowStyle): size = fontSize·5/7, bars 0.6·size long
// and 0.15·size thick, the arrow box inset token.margin from the end edge.
func TestAntdSubmenuArrowBars(t *testing.T) {
	th := theme.AntDesign()
	row := geom.Rect{X: 0, Y: 0, W: 240, H: 40}
	size := th.FontSize * 5 / 7
	for _, rtl := range []bool{false, true} {
		for _, open := range []float32{0, 1} {
			var l paint.List
			antdSubmenuArrow(&l, inlineSpan{r: row, rtl: rtl}, row, th, theme.Hex("#000000"), open, chevronDown)
			b := arrowPathBounds(t, &l)
			// The chevron lies inside its size-wide box at the end edge.
			boxStart := row.W - th.Margin - size
			if rtl {
				boxStart = th.Margin
			}
			if b.X < boxStart-0.6 || b.Right() > boxStart+size+0.6 {
				t.Fatalf("rtl=%v open=%v: arrow %v outside its box [%v, %v]", rtl, open, b, boxStart, boxStart+size)
			}
			// Bars 0.15·size thick: the chevron is visibly bolder than a
			// 1px hairline, as antd's is.
			if thick := size * 0.15; thick < 1.4 {
				t.Fatalf("bar thickness %v, want antd's 0.15·size (1.5px at 14px)", thick)
			}
			if b.H < size*0.3 || b.H > size*0.7 {
				t.Fatalf("rtl=%v open=%v: arrow height %v, want a chevron about half its size %v tall", rtl, open, b.H, size)
			}
		}
	}
}
