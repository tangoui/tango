package editor

import (
	"math"

	"github.com/tangoui/tango/internal/nativeglyph"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

type spanX struct{ left, right float32 }

func (x spanX) empty() bool { return x.right <= x.left }

func (x *spanX) add(left, right float32) {
	if right <= left {
		return
	}
	if x.empty() {
		*x = spanX{left, right}
	} else {
		x.left, x.right = min(x.left, left), max(x.right, right)
	}
}

// rowInk records the bounded row layers from the display list. The text
// commands borrow immutable glyph slices; their indices are relative to rowOps.
type rowInk struct {
	start, end int
	sel        spanX
	number     int // zero when the number is hidden
	borderW    float32
	bands      uint8
	fallback   bool
}

const (
	inkLineHighlight uint8 = 1 << iota
	inkGutterHighlight
	inkNumberLeft
	inkNumberActive
)

// transformedPaint reports whether the current device-space glyph placement
// depends on an enclosing transform. Only open scopes affect this paint.
func transformedPaint(l *paint.List) bool {
	closed := 0
	for i := len(l.Ops) - 1; i >= 0; i-- {
		switch l.Ops[i].(type) {
		case paint.Pop:
			closed++
		case paint.PushTransform:
			if closed == 0 {
				return true
			}
			closed--
		case paint.PushClip, paint.PushOpacity:
			closed = max(0, closed-1)
		}
	}
	return false
}

func (r *damageRec) recordInk(f *paintFrame, spare *damageRec) {
	clear(spare.rowOps)
	r.rowOps, r.ink = spare.rowOps[:0], spare.ink[:0]
	if !f.boundRows || !(r.scale > 0 && r.scale < 1<<16) {
		return
	}
	r.rowOps = append(r.rowOps, f.rowOps...)
	for k, lv := range f.lines {
		ink := lv.damageInk
		if f.cur != nil && f.cur[k] {
			ink.bands |= inkLineHighlight
			ink.borderW = f.lineBorderW
		}
		if f.curG != nil && f.curG[k] {
			ink.bands |= inkGutterHighlight
		}
		if n, show, left := f.g.opts.lineNumber(lv.i, f.caretLine, r.lineCount); show {
			ink.number = n
			if left {
				ink.bands |= inkNumberLeft
			}
			if lv.i == f.caretLine {
				ink.bands |= inkNumberActive
			}
		}
		ink.fallback = ink.fallback || lv.long != nil || len(lv.inj) != 0 ||
			f.fold[k] != 0 || (k < len(f.guides) && f.guides[k].level > 0) ||
			(k < len(f.bguides) && len(f.bguides[k].g) > 0) ||
			(k < len(f.decos) && f.decos[k] != 0) ||
			(k < len(f.udecos) && len(f.udecos[k].ds) > 0)
		r.ink = append(r.ink, ink)
	}
	for _, m := range f.match {
		r.ink[m.k].fallback = true
	}
}

type glyphInkKey struct {
	face       *text.Face
	id         uint32
	size, skew float32
	sub        uint8
}

type rowInkMeasure struct {
	cache *map[glyphInkKey]spanX
	scale float32
	cpu   *paint.CPU
}

// changedRows returns the union of both paints, including glyphs that
// disappeared when a line shrank or became empty.
func (m *rowInkMeasure) changedRows(prev, cur *damageRec, k int) (spanX, bool) {
	a, b := prev.ink[k], cur.ink[k]
	if a.fallback || b.fallback || a.bands != b.bands || a.number != b.number || a.borderW != b.borderW {
		return spanX{}, false
	}
	x, ok := m.measure(a, prev.rowOps[a.start:a.end])
	if !ok {
		return spanX{}, false
	}
	y, ok := m.measure(b, cur.rowOps[b.start:b.end])
	x.add(y.left, y.right)
	if !x.empty() {
		// Leave a device pixel for antialiasing at shape edges. The Runner
		// rounds the resulting rectangle outwards to device pixels.
		pad := 1 / m.scale
		x.left = max(cur.g.text.X, x.left-pad)
		x.right = min(cur.g.text.Right(), x.right+pad)
	}
	if x.empty() && a.number != 0 {
		return spanX{}, false
	}
	if !x.empty() && a.number != 0 {
		// A changed row may have new digit ink even when its number and
		// advance are unchanged (for example, after a font replacement).
		x.left = cur.g.gutter.X
	}
	return x, ok
}

func (m *rowInkMeasure) measure(row rowInk, ops []paint.Op) (spanX, bool) {
	x := row.sel
	for _, op := range ops {
		switch o := op.(type) {
		case paint.FillRect: // token underlines and strikethroughs
			x.add(o.Rect.X, o.Rect.Right())
		case paint.Text:
			for _, g := range o.Glyphs {
				if g.Src == nil {
					continue
				}
				face, ok := g.Src.(*text.Face)
				if !ok || !(g.Size*m.scale > 0 && g.Size*m.scale <= 4096) {
					return spanX{}, false
				}
				px, _, sub := paint.GlyphPlacement(o.Origin, g, m.scale)
				ink := m.glyph(g, glyphInkKey{face, g.ID, g.Size * m.scale, g.Skew, sub})
				if !ink.empty() {
					x.add((float32(px)+ink.left)/m.scale, (float32(px)+ink.right)/m.scale)
				}
			}
		default:
			return spanX{}, false
		}
	}
	return x, true
}

func (m *rowInkMeasure) glyph(g paint.Glyph, key glyphInkKey) spanX {
	cache := *m.cache
	if x, ok := cache[key]; ok {
		return x
	}
	if m.cpu == nil {
		m.cpu = paint.NewCPU()
	}
	var x spanX
	// CPU and the GPU atlas share mask placement and coverage, including
	// rasterizer-specific overhang, padding and synthetic italic.
	if mask, off := m.cpu.GlyphMask(g, key.size, key.sub); mask != nil {
		x.add(float32(off.X), float32(off.X+mask.Rect.Dx()))
	}
	// Quartz can also draw a glyph directly instead of using its mask.
	if p := nativeglyph.CoreGraphics; p != nil && key.id <= math.MaxUint16 && p.Available(key.face) {
		run := nativeglyph.Run{
			Source: key.face, Size: key.size, Skew: key.skew, IDs: []uint16{uint16(key.id)},
			Positions: []geom.Point{{X: float32(key.sub) / 4}},
		}
		if b, ok := p.Bounds(run); ok {
			x.add(float32(b.Min.X), float32(b.Max.X))
		}
	}
	// Keep the metrics small, without retaining another atlas of masks.
	if cache == nil {
		cache = make(map[glyphInkKey]spanX)
		*m.cache = cache
	} else if len(cache) >= 2048 {
		clear(cache)
		m.cpu = nil
	}
	cache[key] = x
	return x
}
