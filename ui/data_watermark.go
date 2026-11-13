package ui

import (
	"image"
	"math"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// WatermarkW tiles rotated text (or an image) over its content. The marks
// are painted above the children but never take pointer input.
type WatermarkW struct {
	c        *Context
	b        *Box
	lines    []string
	rotate   float64
	gapX     float32
	gapY     float32
	offX     float32
	offY     float32
	size     float32
	weight   text.Weight
	col      theme.Color
	colSet   bool
	img      image.Image
	imgW     float32
	imgH     float32
	stagger  bool
	imgAlpha float32
}

// Watermark starts a watermark container; each argument is one text line.
// Defaults follow Ant Design / Element Plus: -22°, gap 100×100, 16px,
// rgba(0,0,0,.15) (theme-aware), staggered rows.
func Watermark(c *Context, lines ...string) *WatermarkW {
	w := &WatermarkW{c: c, lines: lines, rotate: -22, gapX: 100, gapY: 100, size: 16, weight: text.Regular, stagger: true}
	w.b = c.box("watermark", layout.Column, nil)
	return w
}

// Rotate sets the rotation in degrees (negative = counter-clockwise).
func (w *WatermarkW) Rotate(deg float64) *WatermarkW { w.rotate = deg; return w }

// Gap sets the spacing between marks.
func (w *WatermarkW) Gap(x, y float32) *WatermarkW { w.gapX, w.gapY = x, y; return w }

// Offset shifts the first mark from the top-left corner (default gap/2).
func (w *WatermarkW) Offset(x, y float32) *WatermarkW { w.offX, w.offY = x, y; return w }

// FontSize sets the text size.
func (w *WatermarkW) FontSize(px float32) *WatermarkW { w.size = px; return w }

// Weight sets the text weight.
func (w *WatermarkW) Weight(wt text.Weight) *WatermarkW { w.weight = wt; return w }

// Color sets the mark color (use a translucent color).
func (w *WatermarkW) Color(col theme.Color) *WatermarkW { w.col, w.colSet = col, true; return w }

// Image uses img (drawn at w×h, not rotated, 20% opacity) instead of text.
func (w *WatermarkW) Image(img image.Image, wpx, hpx float32) *WatermarkW {
	w.img, w.imgW, w.imgH = img, wpx, hpx
	if w.imgAlpha == 0 {
		w.imgAlpha = 0.2
	}
	return w
}

// ImageOpacity sets the opacity of image marks (default 0.2).
func (w *WatermarkW) ImageOpacity(a float32) *WatermarkW { w.imgAlpha = a; return w }

// Grid disables the staggered layout (marks on a plain grid).
func (w *WatermarkW) Grid() *WatermarkW { w.stagger = false; return w }

// Layout applies layout options to the container.
func (w *WatermarkW) Layout(o ...layout.Option) *WatermarkW { w.b.n.ln.Style.Apply(o...); return w }

// Body builds the watermarked content.
func (w *WatermarkW) Body(fn func()) *WatermarkW {
	w.b.Body(fn)
	w.b.n.over = w.paint
	return w
}

// shape lays out the text lines and returns them with the size of the
// whole (unrotated) mark.
func (w *WatermarkW) shape() ([]*text.Layout, float32, float32) {
	st := w.c.style(w.size, w.weight)
	var mw, mh float32
	lays := make([]*text.Layout, len(w.lines))
	for i, s := range w.lines {
		lays[i] = text.Shape(s, st, text.Options{NoWrap: true})
		mw = max(mw, lays[i].Width)
		mh += lays[i].Height
	}
	return lays, mw, mh
}

func (w *WatermarkW) paint(l *paint.List, r geom.Rect) {
	col := w.col
	if !w.colSet {
		col = theme.Fade(w.c.Theme.Text, 0.17)
	}
	var lays []*text.Layout
	var mw, mh float32
	if w.img != nil {
		mw, mh = w.imgW, w.imgH
	} else {
		lays, mw, mh = w.shape()
	}
	// Tile = mark box + gap; the mark sits at the tile center.
	tw, thh := max(mw, 120)+w.gapX, max(mh, 64)+w.gapY
	ox, oy := w.offX, w.offY
	if ox == 0 && oy == 0 {
		ox, oy = w.gapX/2, w.gapY/2
	}
	ang := w.rotate * math.Pi / 180
	l.Clip(r, geom.Radii{})
	if w.img != nil {
		l.Opacity(w.imgAlpha)
	}
	row := 0
	for y := r.Y + oy - thh; y < r.Bottom()+thh; y += thh {
		shift := float32(0)
		if w.stagger && row%2 == 1 {
			shift = tw / 2
		}
		for x := r.X + ox - tw + shift; x < r.Right()+tw; x += tw {
			cx, cy := x+(tw-w.gapX)/2, y+(thh-w.gapY)/2
			if w.img != nil {
				l.DrawImage(w.img, geom.Rect{X: cx - mw/2, Y: cy - mh/2, W: mw, H: mh}, geom.Radii{})
				continue
			}
			if ang != 0 {
				l.Transform(geom.RotateAround(ang, geom.Pt(cx, cy)))
			}
			ly := cy - mh/2
			for _, lay := range lays {
				l.Glyphs(geom.Pt(cx-lay.Width/2, ly), lay.Glyphs, col)
				ly += lay.Height
			}
			if ang != 0 {
				l.Pop()
			}
		}
		row++
	}
	if w.img != nil {
		l.Pop()
	}
	l.Pop()
}
