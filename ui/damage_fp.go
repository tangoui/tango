package ui

import (
	"image/color"
	"math"
	"unsafe"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// Paint fingerprints: the safety net under the redraw hints.
//
// Damage and ScrollShift declarations are a performance hint, and a
// partial frame is only correct when every pixel that changed is covered.
// Widgets that do not declare (most of them) would leave stale pixels the
// moment another widget declares something. So, in every frame that ends
// up partial, the Runner compares what each node painted with what it
// painted the frame before and damages the nodes whose output changed:
//
//   - paintNode records, per node, the ranges of the frame's paint.List its
//     own paint and over functions filled (not its children's, which have
//     their own records), its screen rect, the clip it was painted under
//     and the transform it was painted through (fpNode). This is all a
//     frame pays when it is not partial.
//   - A partial frame hashes every record (fpState.hash): the ops, with
//     their coordinates relative to the node's origin, the node's size and
//     the opacity and transform it inherits. The previous frame's records
//     and ops are kept (one frame, by swapping slices), so when the last
//     frame was not partial its records are hashed then, lazily, instead
//     of hashing every frame "just in case". This relies on display list
//     data not being modified once recorded (already the rule for paths
//     and images, see paint.FillPath and paint.Image).
//   - Records are matched by node ID. A node whose hash, rect or clip
//     changed is damaged where it painted last frame (and where a
//     ScrollShift copy carried those pixels) and where it paints now; new
//     nodes are damaged where they paint, vanished ones where they painted.
//     A node inside a ScrollShift's content that moved by exactly the
//     shift's delta and painted the same thing is left to the copy.
//   - A node's fingerprint covers its own ops only. A paint.BackdropBlur
//     also shows what was painted under it, so the Runner damages every
//     blur a change (or a scroll copy) reaches after the fingerprints
//     (addBlurDamage in damage.go).
//   - A node that declared its own damage while painting (Context.Damage,
//     Box.ScrollShift or paint.List hint ops from its paint or over
//     function: a code editor) is trusted: its declarations are finer than
//     a fingerprint, which only knows that something in the node changed.

// fpPad pads the painted bounds of what a node draws through a rotating or
// skewing transform: its anti-aliased edges reach about a pixel beyond the
// transformed bounding box. Other bounds need no pad: they are already
// conservative (strokes, paths, shadows, glyphs) and dirty rects are
// rounded out to whole device pixels, so a node that changes inside the
// rect a widget declared for it adds nothing.
const fpPad = 1

// fpNode is what paintNode records about one painted node.
type fpNode struct {
	id     ID
	n      *node // this frame's node (shift classification)
	parent int32 // index of the parent's record, -1 for roots
	// [a0, a1): the node's opacity group, paint function and clip push;
	// [b0, b1): its clip pop, over function and opacity pop.
	a0, a1, b0, b1 int32
	sh, shKids     int16 // c.shifts index the node / its kids move with, -1 none
	self           bool  // declared its own damage while painting
	xfOn           bool
	opacity        float32
	org            geom.Point // paint-space origin (the r passed to paint)
	rect           geom.Rect  // screen rect
	clip           geom.Rect  // screen-space clip it was painted under
	xf             geom.Affine

	// Filled in by fpState.hash.
	inh uint64    // inherited opacity and transform
	sum uint64    // fingerprint of the node's own drawing
	vis geom.Rect // screen bounds of that drawing, padded and clipped
}

// fpState is the Runner's fingerprint bookkeeping.
type fpState struct {
	cur, prev  []fpNode   // this frame's records, the last frame's
	prevOps    []paint.Op // the last frame's display list (records index it)
	prevHashed bool       // prev's sums are computed
	parent     int32      // record of the node being painted (-1: none)
	idx        map[ID]int32
	used       []bool
	stack      []geom.Affine
	unknown    uint64 // makes unhashable ops differ every time
}

// fpBegin starts the records of a frame and returns the ops slice the frame
// should record into: the one of the frame before last (the last frame's
// stays in prevOps).
func (s *fpState) fpBegin(last []paint.Op) []paint.Op {
	s.cur = s.cur[:0]
	s.parent = -1
	ops := s.prevOps[:0]
	s.prevOps = last
	return ops
}

// fpEnd ends a frame: its records become the baseline of the next one.
func (s *fpState) fpEnd(hashed bool) {
	s.prev, s.cur = s.cur, s.prev[:0]
	s.prevHashed = hashed
}

// fpOpen appends the record of n, painted at r (paint space) under clip
// (paint space), and returns its index.
func (c *Context) fpOpen(n *node, r, clip geom.Rect, ops int) int32 {
	s := &c.fp
	s.cur = append(s.cur, fpNode{
		id: n.id, n: n, parent: s.parent, a0: int32(ops), sh: -1, shKids: -1,
		xfOn: c.paintXfOn, xf: c.paintXf, opacity: n.opacity,
		org: r.Min(), rect: n.scr, clip: c.screenRect(clip),
	})
	return int32(len(s.cur) - 1)
}

// hash computes inh, sum and vis of recs from ops.
func (s *fpState) hash(recs []fpNode, ops []paint.Op) {
	for i := range recs {
		rec := &recs[i]
		var inh uint64
		if p := rec.parent; p >= 0 {
			inh = fpMix(recs[p].inh, uint64(math.Float32bits(recs[p].opacity)))
		}
		m := geom.Identity()
		if rec.xfOn {
			m = rec.xf
			inh = fpMix(fpMix(fpMix(inh, fpPair(m.A, m.B)), fpPair(m.C, m.D)), fpPair(m.E, m.F))
		}
		rec.inh = inh
		h := fpMix(fpMix(fpSeed, inh), fpPair(rec.rect.W, rec.rect.H))
		s.stack = append(s.stack[:0], m)
		var b geom.Rect
		h, b = s.scan(rec, ops[rec.a0:rec.a1], h, b)
		h, b = s.scan(rec, ops[rec.b0:rec.b1], h, b)
		rec.sum = h
		rec.vis = rec.rect.Union(b).Intersect(rec.clip)
	}
}

// scan hashes ops (coordinates relative to rec.org) into h and adds their
// screen bounds to b.
func (s *fpState) scan(rec *fpNode, ops []paint.Op, h uint64, b geom.Rect) (uint64, geom.Rect) {
	o := rec.org
	add := func(r geom.Rect) {
		if m := &s.stack[len(s.stack)-1]; *m != (geom.Affine{A: 1, D: 1}) {
			r = m.Bounds(r)
			if m.B != 0 || m.C != 0 {
				r = r.Expand(fpPad)
			}
		}
		b = b.Union(r)
	}
	rel := func(h uint64, r geom.Rect) uint64 {
		return fpMix(fpMix(h, fpPair(r.X-o.X, r.Y-o.Y)), fpPair(r.W, r.H))
	}
	for _, op := range ops {
		switch v := op.(type) {
		case paint.FillRect:
			h = fpRadii(fpColor(rel(fpMix(h, 1), v.Rect), v.Color), v.Radii)
			add(v.Rect)
		case paint.StrokeRect:
			h = fpRadii(fpColor(rel(fpMix(h, 2), v.Rect), v.Color), v.Radii)
			h = fpMix(h, uint64(math.Float32bits(v.Width)))
			for _, d := range v.Dash {
				h = fpMix(h, uint64(math.Float32bits(d)))
			}
			add(v.Rect.Expand(v.Width))
		case paint.ShadowRect:
			sh := v.Shadow
			h = fpRadii(fpColor(rel(fpMix(h, 3), v.Rect), sh.Color), v.Radii)
			h = fpMix(fpMix(h, fpPair(sh.X, sh.Y)), fpPair(sh.Blur, sh.Spread))
			add(v.Rect.Offset(sh.X, sh.Y).Expand(abs32(sh.Spread) + 1.5*abs32(sh.Blur) + 1))
		case paint.FillPath:
			var pb geom.Rect
			h, pb = fpPath(fpColor(fpMix(h, 4), v.Color), v.Path, o)
			add(pb)
		case paint.LinearGradient:
			h = fpRadii(rel(fpMix(h, 5), v.Rect), v.Radii)
			h = fpMix(fpMix(h, fpPair(v.P0.X-o.X, v.P0.Y-o.Y)), fpPair(v.P1.X-o.X, v.P1.Y-o.Y))
			h = fpColor(fpColor(h, v.From), v.To)
			add(v.Rect)
		case paint.Gradient:
			h = fpRadii(rel(fpMix(h, 6+uint64(v.Kind)<<8), v.Rect), v.Radii)
			h = fpMix(fpMix(h, fpPair(v.Start.X-o.X, v.Start.Y-o.Y)), fpPair(v.End.X-o.X, v.End.Y-o.Y))
			for _, st := range v.Stops {
				h = fpColor(fpMix(h, uint64(math.Float32bits(st.Offset))), st.Color)
			}
			if v.Path != nil {
				var pb geom.Rect
				h, pb = fpPath(h, v.Path, o)
				add(pb)
			} else {
				add(v.Rect)
			}
		case paint.Checker:
			h = fpRadii(rel(fpMix(h, 7), v.Rect), v.Radii)
			h = fpColor(fpColor(fpMix(h, uint64(math.Float32bits(v.Cell))), v.A), v.B)
			add(v.Rect)
		case paint.Text:
			h = fpColor(fpMix(fpMix(h, 8), fpPair(v.Origin.X-o.X, v.Origin.Y-o.Y)), v.Color)
			var src uint64
			// A glyph's outline is unknown here: assume it stays within an
			// em above, two ems right and half an em around its origin
			// (wide glyphs, synthetic italics, accents).
			x0, y0 := float32(math.Inf(1)), float32(math.Inf(1))
			x1, y1 := float32(math.Inf(-1)), float32(math.Inf(-1))
			for i := range v.Glyphs {
				g := &v.Glyphs[i]
				if id := fpIdentity(g.Src); i == 0 || id != src {
					src = id
					h = fpMix(h, id)
				}
				h = fpMix(fpMix(h, uint64(g.ID)<<32|uint64(math.Float32bits(g.Size))), fpPair(g.X, g.Y))
				if g.Skew != 0 {
					h = fpMix(h, uint64(math.Float32bits(g.Skew)))
				}
				sz := abs32(g.Size)
				x0, y0 = min(x0, g.X-sz/2), min(y0, g.Y-1.5*sz)
				x1, y1 = max(x1, g.X+2*sz), max(y1, g.Y+sz/2)
			}
			if x1 > x0 {
				add(geom.Rect{X: v.Origin.X + x0, Y: v.Origin.Y + y0, W: x1 - x0, H: y1 - y0})
			}
		case paint.Image:
			// Renderers cache images by value (pointer); in-place pixel
			// changes need InvalidateImage and a declaration.
			h = fpRadii(rel(fpMix(fpMix(h, 9), fpIdentity(v.Img)), v.Rect), v.Radii)
			add(v.Rect)
		case paint.BackdropBlur:
			// What it draws depends on what lies under it too: the Runner
			// damages it whenever a change reaches it (addBlurDamage).
			h = fpRadii(rel(fpMix(h, 14), v.Rect), v.Radii)
			h = fpMix(h, uint64(math.Float32bits(v.Radius)))
			add(v.Rect)
		case paint.PushClip:
			h = fpRadii(rel(fpMix(h, 10), v.Rect), v.Radii)
			s.stack = append(s.stack, s.stack[len(s.stack)-1])
		case paint.PushOpacity:
			h = fpMix(fpMix(h, 11), uint64(math.Float32bits(v.Alpha)))
			s.stack = append(s.stack, s.stack[len(s.stack)-1])
		case paint.PushTransform:
			// Conjugate by the node's origin, so a node that moves
			// rigidly hashes the same: M'(q) = M(q+o) - o.
			m := v.M
			e := m.A*o.X + m.C*o.Y + m.E - o.X
			f := m.B*o.X + m.D*o.Y + m.F - o.Y
			h = fpMix(fpMix(fpMix(fpMix(h, 12), fpPair(m.A, m.B)), fpPair(m.C, m.D)), fpPair(e, f))
			s.stack = append(s.stack, s.stack[len(s.stack)-1].Mul(m))
		case paint.Pop:
			h = fpMix(h, 13)
			if len(s.stack) > 1 {
				s.stack = s.stack[:len(s.stack)-1]
			}
		case paint.Damage, paint.ScrollRegion:
			rec.self = true // hints recorded by the node's own paint
		default:
			s.unknown++
			h = fpMix(h, s.unknown|1<<63)
		}
	}
	return h, b
}

// fingerprintDamage appends to dam the damage of the nodes whose painting
// changed without a declaration (see the top of this file). ops is this
// frame's display list.
func (c *Context) fingerprintDamage(dam []geom.Rect, ops []paint.Op) []geom.Rect {
	s := &c.fp
	if !s.prevHashed {
		s.hash(s.prev, s.prevOps)
		s.prevHashed = true
	}
	s.hash(s.cur, ops)
	cur, prev := s.cur, s.prev

	// The ScrollShift each node moves with, as layoutShiftDamage decides
	// it: the content of a Box.ScrollShift moves with its kids, the kids
	// of a scroll container move.
	if len(c.shifts) > 0 {
		for i := range cur {
			rec := &cur[i]
			in := int16(-1)
			if rec.parent >= 0 {
				in = cur[rec.parent].shKids
			}
			own := int16(-1)
			for k := range c.shifts {
				if c.shifts[k].content == rec.n {
					own = int16(k)
					break
				}
			}
			switch {
			case own < 0:
				rec.sh, rec.shKids = in, in
			case rec.n.scroll == nil:
				rec.sh, rec.shKids = own, own
			default:
				rec.sh, rec.shKids = in, own
			}
		}
	}

	if cap(s.used) < len(prev) {
		s.used = make([]bool, len(prev))
	}
	used := s.used[:len(prev)]
	clear(used)
	byIndex := true // records still line up with last frame's
	for i := range cur {
		cr := &cur[i]
		j := int32(-1)
		if byIndex && i < len(prev) && prev[i].id == cr.id {
			j = int32(i)
		} else {
			if byIndex {
				byIndex = false
				if s.idx == nil {
					s.idx = make(map[ID]int32, len(prev))
				}
				clear(s.idx)
				for k := range prev {
					s.idx[prev[k].id] = int32(k)
				}
			}
			if k, ok := s.idx[cr.id]; ok && !used[k] {
				j = k
			}
		}
		if j < 0 {
			if !cr.self {
				dam = append(dam, cr.vis) // appeared
			}
			continue
		}
		used[j] = true
		pr := &prev[j]
		if cr.self {
			continue // declares its own damage
		}
		if k := cr.sh; k >= 0 {
			sh := &c.shifts[k]
			if pr.sum == cr.sum && pr.rect.Offset(sh.d.X, sh.d.Y) == cr.rect &&
				(pr.clip == cr.clip || pr.clip.Offset(sh.d.X, sh.d.Y) == cr.clip) {
				// Moved with the content: the copy redraws it, except
				// where it lies outside the copied region.
				dam = appendOutside(dam, pr.vis, sh.r)
				dam = appendOutside(dam, cr.vis, sh.r)
				continue
			}
		}
		if pr.sum == cr.sum && pr.rect == cr.rect && pr.clip == cr.clip {
			continue
		}
		dam = append(dam, pr.vis)
		dam, _ = c.appendShiftGhosts(dam, pr.vis, geom.Rect{})
		dam = append(dam, cr.vis)
	}
	if byIndex && len(prev) == len(cur) {
		return dam
	}
	for j := range prev {
		if !used[j] {
			pr := &prev[j] // vanished
			dam = append(dam, pr.vis)
			dam, _ = c.appendShiftGhosts(dam, pr.vis, geom.Rect{})
		}
	}
	return dam
}

// Inline 64-bit FNV-style hashing over words: every step is a bijection of
// h, so two sequences that differ in one word never hash alike.
const (
	fpSeed  = 14695981039346656037
	fpPrime = 1099511628211
)

func fpMix(h, x uint64) uint64 { return (h ^ x) * fpPrime }

func fpPair(a, b float32) uint64 {
	return uint64(math.Float32bits(a))<<32 | uint64(math.Float32bits(b))
}

func fpColor(h uint64, c color.NRGBA) uint64 {
	return fpMix(h, uint64(c.R)<<24|uint64(c.G)<<16|uint64(c.B)<<8|uint64(c.A))
}

func fpRadii(h uint64, r geom.Radii) uint64 {
	if r == (geom.Radii{}) {
		return fpMix(h, 0)
	}
	return fpMix(fpMix(h, fpPair(r.TL, r.TR)), fpPair(r.BR, r.BL))
}

// fpPath hashes p's segments relative to o and returns their bounds (the
// control points bound the curves).
func fpPath(h uint64, p *paint.Path, o geom.Point) (uint64, geom.Rect) {
	if p == nil {
		return fpMix(h, 0), geom.Rect{}
	}
	x0, y0 := float32(math.Inf(1)), float32(math.Inf(1))
	x1, y1 := float32(math.Inf(-1)), float32(math.Inf(-1))
	for _, sg := range p.Segs {
		h = fpMix(h, uint64(sg.Op))
		n := 3
		switch sg.Op {
		case paint.SegMove, paint.SegLine:
			n = 1
		case paint.SegQuad:
			n = 2
		case paint.SegClose:
			n = 0
		}
		for _, pt := range sg.P[:n] {
			h = fpMix(h, fpPair(pt.X-o.X, pt.Y-o.Y))
			x0, y0, x1, y1 = min(x0, pt.X), min(y0, pt.Y), max(x1, pt.X), max(y1, pt.Y)
		}
	}
	if x1 < x0 {
		return h, geom.Rect{}
	}
	// Zero-width bounds (a straight line) still cover a pixel.
	return h, geom.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}.Expand(1)
}

// fpIdentity identifies a retained object (a font, an image) by the data
// word of its interface value: the pointer for pointer types. A value
// type is boxed anew whenever it is stored in an interface, so it rarely
// matches, which only costs a redraw.
func fpIdentity(v any) uint64 {
	if v == nil {
		return 0
	}
	return uint64((*[2]uintptr)(unsafe.Pointer(&v))[1])
}
