package ui

import (
	"image"
	"math"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// WatermarkW tiles rotated text (or an image) over its content. The marks
// are painted above the children but never take pointer input.
//
// The tiling follows Ant Design 6.7 (es/watermark/useClips.js): each mark
// is rotated about its centre, the rotated bounding box plus the gap makes
// a cell, and every other column is shifted down by half a cell.
type WatermarkW struct {
	_ [0]func()

	c        *Context
	b        *Box
	lines    []string
	rotate   float64
	gapX     float32
	gapY     float32
	offX     float32
	offY     float32
	offSet   bool
	size     float32
	weight   text.Weight
	italic   bool
	col      theme.Color
	colSet   bool
	img      image.Image
	imgW     float32
	imgH     float32
	markW    float32 // MarkSize (0: from the content)
	markH    float32
	stagger  bool
	imgAlpha float32
}

// Watermark starts a watermark container; each argument is one text line.
// Defaults follow Ant Design / Element Plus: -22°, gap 100×100, 16px,
// rgba(0,0,0,.15) (theme-aware), every other column shifted by half a
// cell.
func Watermark(c *Context, lines ...string) *WatermarkW {
	w := &WatermarkW{c: c, lines: lines, rotate: -22, gapX: 100, gapY: 100, size: 16, weight: text.Regular, stagger: true, imgAlpha: 1}
	w.b = c.box("watermark", layout.Column, nil)
	return w
}

// Rotate sets the rotation in degrees (negative = counter-clockwise).
func (w *WatermarkW) Rotate(deg float64) *WatermarkW { w.rotate = deg; return w }

// Gap sets the spacing between marks.
func (w *WatermarkW) Gap(x, y float32) *WatermarkW { w.gapX, w.gapY = x, y; return w }

// Offset sets where the first mark's box starts, measured to the middle
// of its gap (Ant offset; default gap/2, the box at the top-left corner).
func (w *WatermarkW) Offset(x, y float32) *WatermarkW {
	w.offX, w.offY, w.offSet = x, y, true
	return w
}

// FontSize sets the text size.
func (w *WatermarkW) FontSize(px float32) *WatermarkW { w.size = px; return w }

// Weight sets the text weight.
func (w *WatermarkW) Weight(wt text.Weight) *WatermarkW { w.weight = wt; return w }

// FontStyle sets "normal" (default) or "italic" ("oblique" draws italic;
// Ant font.fontStyle).
func (w *WatermarkW) FontStyle(s string) *WatermarkW {
	w.italic = s == "italic" || s == "oblique"
	return w
}

// Color sets the mark color (use a translucent color).
func (w *WatermarkW) Color(col theme.Color) *WatermarkW { w.col, w.colSet = col, true; return w }

// MarkSize sets the size of one mark before rotation (Ant width / height;
// default: the text's size, or 120×64 for an image without a size).
func (w *WatermarkW) MarkSize(wpx, hpx float32) *WatermarkW { w.markW, w.markH = wpx, hpx; return w }

// Image uses img (drawn at w×h and rotated like text) instead of text.
func (w *WatermarkW) Image(img image.Image, wpx, hpx float32) *WatermarkW {
	w.img, w.imgW, w.imgH = img, wpx, hpx
	return w
}

// ImageOpacity sets the opacity of image marks (default 1: Ant draws the
// image as it is, so use a faint image or a lower opacity).
func (w *WatermarkW) ImageOpacity(a float32) *WatermarkW { w.imgAlpha = a; return w }

// Grid disables the shifted columns (marks on a plain grid).
func (w *WatermarkW) Grid() *WatermarkW { w.stagger = false; return w }

// Layout applies layout options to the container.
func (w *WatermarkW) Layout(o ...layout.Option) *WatermarkW {
	flexbox.Apply(&w.b.n.ln.Style, o...)
	return w
}

// Body builds the watermarked content.
func (w *WatermarkW) Body(fn func()) *WatermarkW {
	w.b.Body(fn)
	w.b.n.over = w.paint
	return w
}

// wmFontGap is the space between two text lines (antd FontGap).
const wmFontGap = 3

// shape lays out the text lines and returns them with the size of the
// whole (unrotated) mark: the widest line, and the line heights plus the
// gaps between lines (antd getMarkSize).
func (w *WatermarkW) shape() ([]*text.Layout, float32, float32) {
	st := text.Style{Size: w.size, Weight: w.weight, Italic: w.italic, FontFamily: w.c.Theme.FontFamily}
	var mw, mh float32
	lays := make([]*text.Layout, len(w.lines))
	for i, s := range w.lines {
		lays[i] = text.Shape(s, st, text.Options{NoWrap: true})
		mw = max(mw, lays[i].Width)
		mh += lays[i].Height
	}
	if n := len(lays); n > 1 {
		mh += float32(n-1) * wmFontGap
	}
	return lays, float32(math.Ceil(float64(mw))), float32(math.Ceil(float64(mh)))
}

// markSize returns the unrotated mark size.
func (w *WatermarkW) markSize() (lays []*text.Layout, mw, mh float32) {
	if w.img != nil {
		mw, mh = w.imgW, w.imgH
		if mw <= 0 || mh <= 0 {
			mw, mh = 120, 64
		}
	} else {
		lays, mw, mh = w.shape()
	}
	if w.markW > 0 {
		mw = w.markW
	}
	if w.markH > 0 {
		mh = w.markH
	}
	return lays, mw, mh
}

func (w *WatermarkW) paint(l *paint.List, r geom.Rect) {
	col := w.col
	if !w.colSet {
		col = theme.Fade(w.c.Theme.Text, 0.17)
	}
	lays, mw, mh := w.markSize()
	if mw <= 0 || mh <= 0 {
		return
	}
	ang := w.rotate * math.Pi / 180
	sn, cs := math.Sincos(ang)
	// Rotated bounding box: one cell is the box plus the gap.
	bw := float32(math.Abs(float64(mw)*cs) + math.Abs(float64(mh)*sn))
	bh := float32(math.Abs(float64(mw)*sn) + math.Abs(float64(mh)*cs))
	// A negative gap may not overlap the marks into nothing: at least a
	// pixel per cell, so the tiling loops end.
	cellW, cellH := max(1, bw+w.gapX), max(1, bh+w.gapY)
	// The pattern starts at offset - gap/2; a positive start also moves
	// the edge of the marked area (antd markStyle left / top).
	ox, oy := w.offX, w.offY
	if !w.offSet {
		ox, oy = w.gapX/2, w.gapY/2
	}
	px, py := ox-w.gapX/2, oy-w.gapY/2
	area := r
	if px > 0 {
		area.X, area.W = r.X+px, max(0, r.W-px)
	}
	if py > 0 {
		area.Y, area.H = r.Y+py, max(0, r.H-py)
	}
	l.Clip(area, geom.Radii{})
	if w.img != nil && w.imgAlpha < 1 {
		l.Opacity(w.imgAlpha)
	}
	x0, y0 := r.X+px, r.Y+py
	// First column and row that can reach the area.
	j0 := int(math.Floor(float64((area.X - x0) / cellW)))
	for j := j0 - 1; x0+float32(j)*cellW < area.Right(); j++ {
		shift := float32(0)
		if w.stagger && j%2 != 0 {
			shift = cellH / 2
		}
		i0 := int(math.Floor(float64((area.Y - y0 - shift) / cellH)))
		for i := i0 - 1; y0+shift+float32(i)*cellH < area.Bottom(); i++ {
			cx := x0 + float32(j)*cellW + bw/2
			cy := y0 + shift + float32(i)*cellH + bh/2
			if ang != 0 {
				l.Transform(geom.RotateAround(ang, geom.Pt(cx, cy)))
			}
			if w.img != nil {
				l.DrawImage(w.img, geom.Rect{X: cx - mw/2, Y: cy - mh/2, W: mw, H: mh}, geom.Radii{})
			} else {
				// Lines from the top of the mark, centred, fontSize + 3 apart
				// (canvas textBaseline top).
				ly := cy - mh/2
				for _, lay := range lays {
					l.Glyphs(geom.Pt(cx-lay.Width/2, ly), lay.Glyphs, col)
					ly += w.size + wmFontGap
				}
			}
			if ang != 0 {
				l.Pop()
			}
		}
	}
	if w.img != nil && w.imgAlpha < 1 {
		l.Pop()
	}
	l.Pop()
}
