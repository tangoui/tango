package gpu

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// Partial redraw (spec 6.2, 6.8).
//
// The Renderer redraws only part of a frame when the List carries redraw
// hints (paint.Damage, paint.ScrollRegion) and the device's target still
// holds the previous frame: the Device is a Retainer reporting it retained
// at this size, the previous Render executed successfully, and the scale
// and List.Background did not change. Otherwise, and for a frame without
// any hint, the whole frame is drawn.
//
// planPartial pre-scans the ops for hints with their composed transform
// (also those inside subtrees drawn as patches): a Damage rect becomes the
// device bounding box of its transformed rect. A ScrollRegion under a
// translate/uniform-scale transform whose device delta is whole pixels
// becomes a CopyRegion (the part of its rect that stays inside it after the
// shift) plus dirty rectangles for the rest of the rect (the exposed strip
// and partial pixels at its edges); the Renderer trusts that everything
// else in the rect moved with the content. A ScrollRegion is unsafe, and
// its whole rect dirty instead, when it is recorded under another
// transform, an unknown Push… op or a translucent opacity group, when a
// clip's effective rectangle partly overlaps it (an edge inside the region
// that would be copied along) or a rounded clip containing it has a corner
// over it, when a translucent opacity group's bounds partly overlap or
// contain it, or when it overlaps another ScrollRegion (the copies would
// depend on each other's order). A Damage under an unknown Push… op makes
// the frame full.
//
// Backdrop blurs (paint.BackdropBlur) read the target as drawn before
// them, so a partial frame must redraw a blur's whole source region (its
// plan's Src) whenever it redraws any of it: blurDirty grows the dirty
// rectangles accordingly, and makes the destination of a copy that would
// carry last frame's blurred pixels dirty too. Frames without blurs (this
// one and the last) are planned exactly as before.
//
// The dirty rectangles are clipped to the frame, made disjoint (overlapping
// ones merged) and merged down to maxDirty, the pair whose union adds the
// least area first; if they then cover more than half the frame, the frame
// is drawn in full. A partial frame is built like a full one with its root
// scissor the dirty rectangles' bounding box (what lies outside is culled,
// patches are rasterized only inside it), and instances drawing into the
// frame's target that touch no dirty rectangle are dropped; the device
// draws each such batch once per dirty rectangle (Frame.Dirty).

// maxDirty is the most dirty rectangles a partial frame carries.
const maxDirty = 8

// resident is what the device's target holds after the last Render.
type resident struct {
	ok    bool // the last Render executed its frame successfully
	size  image.Point
	scale float32
	bg    color.NRGBA
}

// scanState is the hint pre-scan's state at an op.
type scanState struct {
	m       geom.Affine     // composed logical transform
	native  bool            // m only translates and scales uniformly (or is the identity)
	sc      image.Rectangle // effective clip rectangle (device)
	alpha   bool            // inside a translucent opacity group
	unknown bool            // inside an unknown Push… op
}

// scanClip is a clip seen by the pre-scan: its effective rectangle and,
// when rounded, its device rectangle and largest radius.
type scanClip struct {
	sc      image.Rectangle
	rounded bool
	rect    image.Rectangle
	rad     int
}

// scanGroup is a translucent opacity group seen by the pre-scan: the
// index of its PushOpacity and the transform it is under.
type scanGroup struct {
	i      int
	m      geom.Affine
	native bool
}

// scanBlur is a BackdropBlur seen by the pre-scan: the pixels it changes
// and the pixels it reads (paint.BlurPlan's Out and Src).
type scanBlur struct {
	out, src image.Rectangle
}

// blurDirty adds to the dirty rectangles what the frame's backdrop blurs
// (and the last frame's, prev: their Out rectangles) require, so that a
// partial frame redraws each blur from a source that equals a full
// frame's at that point:
//
//   - a copy carries the pixels of its source along: where that source
//     held last frame's blurred pixels, its destination is redrawn (a
//     blur's look must not move with scrolled content);
//   - a copy whose destination reaches a blur's source changed what the
//     blur reads: the blur's whole source is redrawn;
//   - a dirty rectangle reaching a blur's source likewise makes its whole
//     source dirty, until no blur's source is reached that is not dirty
//     (merging the rectangles as mergeDirty does at each step, since
//     merging grows them).
//
// The result is merged (mergeDirty) to at most maxDirty rectangles.
func blurDirty(dirty []image.Rectangle, copies []CopyRegion, blurs []scanBlur, prev []image.Rectangle) []image.Rectangle {
	added := make([]bool, len(blurs))
	for _, c := range copies {
		d := c.Dst.Min.Sub(c.Src.Min)
		for _, p := range prev {
			if g := p.Intersect(c.Src); !g.Empty() {
				dirty = append(dirty, g.Add(d).Intersect(c.Dst))
			}
		}
		for i, b := range blurs {
			if !added[i] && c.Dst.Overlaps(b.src) {
				dirty = append(dirty, b.src)
				added[i] = true
			}
		}
	}
	for {
		dirty = mergeDirty(dirty, maxDirty)
		grew := false
		for i, b := range blurs {
			if !added[i] && overlapsAny(b.src, dirty) {
				dirty = append(dirty, b.src)
				added[i], grew = true, true
			}
		}
		if !grew {
			return dirty
		}
	}
}

// scanScroll is a ScrollRegion seen by the pre-scan.
type scanScroll struct {
	o  paint.ScrollRegion
	st scanState
}

// SetVerify turns verify mode on or off. In verify mode every partial
// redraw is checked: after it executes, the whole frame is drawn too and
// the two targets (read back with PixelReader; a device without it is not
// checked) are compared pixel for pixel (tolerance: see verifyPartial); a
// difference, which usually means the List's hints missed a change, is a
// *VerifyError that Render returns or logs (SetVerifyStrict). Verify mode is on by default except in builds with the
// tango_release tag; it costs two read-backs and a full frame per partial
// frame.
func (r *Renderer) SetVerify(on bool) { r.verify = on }

// planPartial decides whether this frame (ops at size and scale, cleared
// to bg) is drawn partially, filling r.dirty and r.copies (see above). It
// needs r.scale set.
func (r *Renderer) planPartial(ops []paint.Op, size image.Point, scale float32, bg color.NRGBA) bool {
	r.dirty, r.copies = r.dirty[:0], r.copies[:0]
	rt, ok := r.dev.(Retainer)
	if !ok {
		r.prevClips = r.prevClips[:0]
		r.prevBlurs = r.prevBlurs[:0]
		return false
	}
	// The scan runs for every frame on a Retainer: the next frame checks
	// its scroll regions against this frame's clips.
	full := image.Rectangle{Max: size}
	hints, forceFull := false, false
	clips, groups, scrolls := r.scanClips[:0], r.scanGroups[:0], r.scanScrolls[:0]
	blurs := r.scanBlurs[:0]
	prev := r.prevClips
	prevBlurs := r.prevBlurs
	defer func() {
		clear(scrolls) // drop references to the list
		// This frame's clips and blurs become the previous frame's.
		r.prevClips, r.scanClips = clips, prev[:0]
		r.scanGroups, r.scanScrolls = groups[:0], scrolls[:0]
		r.prevBlurs = r.prevBlurs[:0]
		for _, b := range blurs {
			r.prevBlurs = append(r.prevBlurs, b.out)
		}
		r.scanBlurs = blurs[:0]
	}()
	st := r.scanStack[:0]
	cur := scanState{m: geom.Identity(), native: true, sc: full}
	for i, op := range ops {
		switch o := op.(type) {
		case paint.PushClip:
			st = append(st, cur)
			var b image.Rectangle
			rounded := !o.Radii.IsZero()
			var rr image.Rectangle
			rad := 0
			if cur.native {
				g := cur.m.Bounds(o.Rect)
				b = r.devRect(g)
				if rounded {
					rr = b
					k := r.scale * cur.m.A
					rad = clampInt(math.Ceil(float64(max(o.Radii.TL, o.Radii.TR, o.Radii.BR, o.Radii.BL) * k)))
				}
			} else {
				b = r.outBounds(cur.m.Bounds(o.Rect), 1)
				if rounded {
					rr, rad = b, max(b.Dx(), b.Dy()) // treat it as all corner
				}
			}
			cur.sc = cur.sc.Intersect(b)
			clips = append(clips, scanClip{sc: cur.sc, rounded: rounded, rect: rr, rad: rad})
		case paint.PushOpacity:
			st = append(st, cur)
			if o.Alpha > 0 && o.Alpha*255+0.5 < 255 { // quantizes below 255 (0 draws nothing, but stays conservative)
				cur.alpha = true
				groups = append(groups, scanGroup{i: i, m: cur.m, native: cur.native})
			}
		case paint.PushTransform:
			st = append(st, cur)
			cur.m = cur.m.Mul(o.M)
			cur.native = cur.m.IsIdentity() || nativeXform(cur.m)
		case paint.Pop:
			if n := len(st); n > 0 {
				cur = st[n-1]
				st = st[:n-1]
			}
		case paint.Damage:
			hints = true
			if cur.unknown {
				forceFull = true
			}
			r.dirty = append(r.dirty, r.hintBounds(o.Rect, cur).Intersect(full))
		case paint.ScrollRegion:
			hints = true
			scrolls = append(scrolls, scanScroll{o, cur})
		case paint.BackdropBlur:
			// What backdropBlur plans for it (drawn only under a native
			// transform; c.full is the scan's sc).
			if cur.native {
				k := float32(math.Abs(float64(cur.m.A)))
				g := cur.m.Bounds(o.Rect)
				s := r.scale
				dr := geom.Rect{X: g.X * s, Y: g.Y * s, W: g.W * s, H: g.H * s}
				if p, ok := paint.PlanBackdropBlur(dr, paint.BlurSigma(o.Radius*k, s), cur.sc, full); ok {
					blurs = append(blurs, scanBlur{out: p.Out, src: p.Src})
				}
			}
		default:
			if op != nil && isPush(op) {
				st = append(st, cur)
				cur.unknown = true
			}
		}
	}
	r.scanStack = st[:0]
	res := r.res
	if !hints || forceFull || !res.ok || res.size != size || res.scale != scale || res.bg != bg || !rt.Retained(size) {
		r.dirty = r.dirty[:0]
		return false
	}

	// Opacity group bounds, needed to check scroll regions against them.
	var gb []image.Rectangle
	if len(scrolls) > 0 && len(groups) > 0 {
		gb = make([]image.Rectangle, len(groups))
		for k, g := range groups {
			end := matchingPop(ops, g.i)
			body := ops[g.i+1 : end]
			if g.native {
				m, xf := g.m, !g.m.IsIdentity()
				gb[k] = r.boundsIn(body, m, xf)
			} else {
				gb[k] = r.transformBounds(g.m, r.boundsIn(body, geom.Affine{}, false))
			}
		}
	}
	type scrollGeom struct {
		outer, inner image.Rectangle
		d            image.Point
		ok           bool
	}
	sg := make([]scrollGeom, len(scrolls))
	for k, s := range scrolls {
		g := &sg[k]
		g.outer = r.hintBounds(s.o.Rect, s.st).Intersect(full)
		if s.st.unknown {
			r.dirty = r.dirty[:0]
			return false
		}
		if s.st.alpha || !s.st.native || g.outer.Empty() {
			continue
		}
		ks := float64(r.scale * s.st.m.A)
		dx, dy := float64(s.o.Delta.X)*ks, float64(s.o.Delta.Y)*ks
		rx, ry := math.Round(dx), math.Round(dy)
		if !(math.Abs(dx-rx) < 1e-3 && math.Abs(dy-ry) < 1e-3) {
			continue // not whole device pixels
		}
		g.d = image.Pt(clampInt(rx), clampInt(ry))
		lg := s.st.m.Bounds(s.o.Rect)
		sc := float64(r.scale)
		g.inner = image.Rect(clampInt(math.Ceil(float64(lg.X)*sc-1e-3)), clampInt(math.Ceil(float64(lg.Y)*sc-1e-3)),
			clampInt(math.Floor(float64(lg.Right())*sc+1e-3)), clampInt(math.Floor(float64(lg.Bottom())*sc+1e-3))).Intersect(full)
		g.ok = !g.inner.Empty()
		if g.ok {
			dst := g.inner.Add(g.d).Intersect(g.inner)
			g.ok = scrollSafe(g.outer, dst, g.d, clips, prev, gb)
		}
	}
	for a := range sg {
		for b := a + 1; b < len(sg); b++ {
			if sg[a].outer.Overlaps(sg[b].outer) {
				sg[a].ok, sg[b].ok = false, false
			}
		}
	}
	for _, g := range sg {
		if !g.ok {
			r.dirty = append(r.dirty, g.outer)
			continue
		}
		dst := g.inner.Add(g.d).Intersect(g.inner)
		if dst.Empty() || g.d == (image.Point{}) {
			// Nothing left to copy, or a shift that rounds to no device
			// pixels (a sub-pixel tail of an inertial scroll): the content
			// moved by a fraction of a pixel, so redraw the region.
			r.dirty = append(r.dirty, g.outer)
			continue
		}
		r.copies = append(r.copies, CopyRegion{Src: dst.Sub(g.d), Dst: dst})
		r.dirty = appendDiff(r.dirty, g.outer, dst)
	}

	if len(blurs) > 0 || len(prevBlurs) > 0 {
		r.dirty = blurDirty(r.dirty, r.copies, blurs, prevBlurs)
	}
	r.dirty = mergeDirty(r.dirty, maxDirty)
	if len(r.dirty) == 0 {
		// Every copy leaves a dirty strip, so there is none either; a
		// frame without Dirty is never partial (Frame.Dirty).
		r.copies = r.copies[:0]
	}
	area := 0
	for _, d := range r.dirty {
		area += d.Dx() * d.Dy()
	}
	if 2*area > size.X*size.Y {
		r.dirty, r.copies = r.dirty[:0], r.copies[:0]
		return false
	}
	return true
}

// hintBounds returns the device bounding box of the hint rect g recorded
// in state st: g through st's transform, rounded out (and grown by a pixel
// under a rotating or skewing transform, whose patch is antialiased along
// the rotated edges).
func (r *Renderer) hintBounds(g geom.Rect, st scanState) image.Rectangle {
	if g.Empty() {
		return image.Rectangle{}
	}
	pad := 0
	if !st.m.IsIdentity() {
		g = st.m.Bounds(g)
		if !st.native {
			pad = 1
		}
	}
	return r.outBounds(g, pad)
}

// scrollSafe reports whether copying the pixels of a scroll region (device
// rectangle outer, shifted by d into dst) is safe with respect to the
// frame's clips, the previous frame's clips prev and the frame's
// translucent opacity groups (bounds gb):
//
//   - a clip edge strictly inside the region must have moved with the
//     content: an edge of this frame's clips over dst needs a matching edge
//     (same side, at its position minus d, overlapping span) among prev,
//     and an edge of prev's clips over the copy's source (dst minus d) one
//     among this frame's clips at its position plus d. So the viewport's
//     own clip (containing the region) and the clips of rows scrolling
//     with the content pass, and a fixed clip edge crossing the region,
//     whose clipped look the copy would carry along, does not. Edges only
//     in the exposed strip are redrawn anyway.
//   - a rounded clip around the region has no corner over it;
//   - no group partly overlaps or contains it (one inside moves along).
func scrollSafe(outer, dst image.Rectangle, d image.Point, clips, prev []scanClip, gb []image.Rectangle) bool {
	src := dst.Sub(d)
	for _, c := range clips {
		if !c.sc.Overlaps(outer) {
			continue
		}
		if !edgesMatch(c.sc, outer, dst, d, prev) {
			return false
		}
		if c.rounded && c.rad > 0 && outer.In(c.sc) {
			rr, k := c.rect, c.rad
			for _, corner := range [4]image.Rectangle{
				image.Rect(rr.Min.X, rr.Min.Y, rr.Min.X+k, rr.Min.Y+k),
				image.Rect(rr.Max.X-k, rr.Min.Y, rr.Max.X, rr.Min.Y+k),
				image.Rect(rr.Min.X, rr.Max.Y-k, rr.Min.X+k, rr.Max.Y),
				image.Rect(rr.Max.X-k, rr.Max.Y-k, rr.Max.X, rr.Max.Y),
			} {
				if corner.Overlaps(outer) {
					return false
				}
			}
		}
	}
	for _, p := range prev {
		if p.sc.Overlaps(outer) && !edgesMatch(p.sc, outer, src, image.Pt(-d.X, -d.Y), clips) {
			return false
		}
	}
	for _, b := range gb {
		if b.Overlaps(outer) && !b.In(outer) {
			return false
		}
	}
	return true
}

// edgesMatch reports whether every edge of the clip rectangle c that lies
// strictly inside outer and overlaps area has a counterpart in others: an
// edge of the same side at its position minus d whose span overlaps its
// span minus d.
func edgesMatch(c, outer, area image.Rectangle, d image.Point, others []scanClip) bool {
	// Horizontal edges (top, bottom) span [c.Min.X, c.Max.X); vertical
	// ones (left, right) [c.Min.Y, c.Max.Y).
	for side := range 4 {
		var at, lo, hi int
		var inside, over bool
		switch side {
		case 0, 1: // top, bottom
			at, lo, hi = c.Min.Y, c.Min.X, c.Max.X
			if side == 1 {
				at = c.Max.Y
			}
			inside = at > outer.Min.Y && at < outer.Max.Y
			// Strictly inside area: an edge on its border cuts no copied
			// pixel (and its counterpart may be a row just scrolled out of
			// the viewport, whose clip is empty there).
			over = at > area.Min.Y && at < area.Max.Y && lo < area.Max.X && hi > area.Min.X
			at, lo, hi = at-d.Y, lo-d.X, hi-d.X
		default: // left, right
			at, lo, hi = c.Min.X, c.Min.Y, c.Max.Y
			if side == 3 {
				at = c.Max.X
			}
			inside = at > outer.Min.X && at < outer.Max.X
			over = at > area.Min.X && at < area.Max.X && lo < area.Max.Y && hi > area.Min.Y
			at, lo, hi = at-d.X, lo-d.Y, hi-d.Y
		}
		if !inside || !over {
			continue
		}
		found := false
		for _, o := range others {
			r := o.sc
			if r.Empty() {
				continue
			}
			var oat, olo, ohi int
			switch side {
			case 0:
				oat, olo, ohi = r.Min.Y, r.Min.X, r.Max.X
			case 1:
				oat, olo, ohi = r.Max.Y, r.Min.X, r.Max.X
			case 2:
				oat, olo, ohi = r.Min.X, r.Min.Y, r.Max.Y
			default:
				oat, olo, ohi = r.Max.X, r.Min.Y, r.Max.Y
			}
			if oat == at && olo < hi && ohi > lo {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// appendDiff appends the parts of a not covered by b (at most 4
// rectangles) to rs.
func appendDiff(rs []image.Rectangle, a, b image.Rectangle) []image.Rectangle {
	b = b.Intersect(a)
	if b.Empty() {
		return append(rs, a)
	}
	if b.Min.Y > a.Min.Y {
		rs = append(rs, image.Rect(a.Min.X, a.Min.Y, a.Max.X, b.Min.Y))
	}
	if b.Max.Y < a.Max.Y {
		rs = append(rs, image.Rect(a.Min.X, b.Max.Y, a.Max.X, a.Max.Y))
	}
	if b.Min.X > a.Min.X {
		rs = append(rs, image.Rect(a.Min.X, b.Min.Y, b.Min.X, b.Max.Y))
	}
	if b.Max.X < a.Max.X {
		rs = append(rs, image.Rect(b.Max.X, b.Min.Y, a.Max.X, b.Max.Y))
	}
	return rs
}

// mergeDirty returns rs, in place, without empty rectangles, with
// overlapping rectangles replaced by their bounding box until none
// overlap, and merged down to at most n, each time the pair whose bounding
// box adds the least area. Rectangles that only touch are kept apart.
func mergeDirty(rs []image.Rectangle, n int) []image.Rectangle {
	out := rs[:0]
	for _, d := range rs {
		if !d.Empty() {
			out = append(out, d)
		}
	}
	rs = out
	const many = 64 // beyond this, pairwise merging is not worth it
	if len(rs) > many {
		u := rs[0]
		for _, d := range rs[1:] {
			u = u.Union(d)
		}
		return append(rs[:0], u)
	}
	area := func(d image.Rectangle) int { return d.Dx() * d.Dy() }
	for {
		// Make them disjoint.
		for merged := true; merged; {
			merged = false
			for a := 0; a < len(rs); a++ {
				for b := a + 1; b < len(rs); b++ {
					if rs[a].Overlaps(rs[b]) {
						rs[a] = rs[a].Union(rs[b])
						rs = append(rs[:b], rs[b+1:]...)
						merged = true
						b--
					}
				}
			}
		}
		if len(rs) <= n {
			return rs
		}
		ba, bb, best := 0, 1, math.MaxInt
		for a := 0; a < len(rs); a++ {
			for b := a + 1; b < len(rs); b++ {
				if c := area(rs[a].Union(rs[b])) - area(rs[a]) - area(rs[b]); c < best {
					ba, bb, best = a, b, c
				}
			}
		}
		rs[ba] = rs[ba].Union(rs[bb])
		rs = append(rs[:bb], rs[bb+1:]...)
	}
}

// overlapsAny reports whether b overlaps one of rs.
func overlapsAny(b image.Rectangle, rs []image.Rectangle) bool {
	for _, d := range rs {
		if b.Overlaps(d) {
			return true
		}
	}
	return false
}

// VerifyError is what verify mode (SetVerify) reports when a partial
// redraw differs from the full frame: usually a Damage or ScrollRegion
// hint missed a change. When it is reported the device's target already
// holds the correct full frame.
type VerifyError struct {
	Region image.Rectangle // bounding box of the differing pixels
	Pixels int             // differing pixels (beyond the tolerance)
	Max    int             // largest channel difference
	Dirty  []image.Rectangle
	Copies []CopyRegion
}

func (e *VerifyError) Error() string {
	return fmt.Sprintf("gpu: verify: partial redraw differs from the full frame in %v (%d pixels, up to %d per channel); dirty %v, copies %v: the frame's Damage/ScrollRegion hints missed a change",
		e.Region, e.Pixels, e.Max, e.Dirty, e.Copies)
}

// verifyPartial draws the frame in full after its partial redraw and
// compares the two targets (verify mode; see SetVerify). A difference is
// returned as a *VerifyError; other errors are the device's.
//
// Tolerance: 0, except inside the destination of a copy (Frame.Copies),
// where a channel may differ by verifyCopyTolerance. A whole-pixel copy
// reproduces a fresh render only up to rounding: content at fractional
// positions is rasterized from (y-off)·scale in float32, which rounds
// differently once shifted, so an antialiased edge may land one level
// apart; and the gradient dither (paint.Dither4x4) is anchored to
// absolute device pixels, so a shift that is not a multiple of 4 changes
// the dither phase, again by at most one level. Both stay within one
// level and do not accumulate beyond it per copy. Dirty rectangles are
// drawn from the same instances as the full frame and must match exactly.
func (r *Renderer) verifyPartial(bg color.NRGBA, ops []paint.Op, size image.Point, scale float32) error {
	pr, ok := r.dev.(PixelReader)
	if !ok {
		return nil
	}
	got, err := pr.ReadPixels()
	if err != nil {
		return err
	}
	dirty := append([]image.Rectangle(nil), r.dirty...)
	copies := append([]CopyRegion(nil), r.copies...)
	r.dirty, r.copies = r.dirty[:0], r.copies[:0]
	if err := r.frame1(bg, ops, size, scale, false); err != nil {
		return err
	}
	want, err := pr.ReadPixels()
	if err != nil {
		return err
	}
	if got.Rect != want.Rect {
		return fmt.Errorf("gpu: verify: partial redraw read back %v, full redraw %v", got.Rect, want.Rect)
	}
	var diff image.Rectangle
	n, worst := 0, 0
	for y := got.Rect.Min.Y; y < got.Rect.Max.Y; y++ {
		a := got.Pix[got.PixOffset(got.Rect.Min.X, y):][:4*got.Rect.Dx()]
		b := want.Pix[want.PixOffset(want.Rect.Min.X, y):][:4*want.Rect.Dx()]
		for i := 0; i < len(a); i += 4 {
			px := 0
			for c := range 4 {
				px = max(px, absInt(int(a[i+c])-int(b[i+c])))
			}
			if px == 0 {
				continue
			}
			x := got.Rect.Min.X + i/4
			if px <= verifyCopyTolerance && inCopy(x, y, dirty, copies) {
				continue
			}
			diff = diff.Union(image.Rect(x, y, x+1, y+1))
			n++
			worst = max(worst, px)
		}
	}
	if n > 0 {
		return &VerifyError{Region: diff, Pixels: n, Max: worst, Dirty: dirty, Copies: copies}
	}
	return nil
}

// verifyCopyTolerance is the largest channel difference verify mode
// accepts inside a copy's destination (see verifyPartial); elsewhere the
// partial frame must equal the full frame exactly.
const verifyCopyTolerance = 1

// inCopy reports whether pixel (x, y) was produced by a copy: inside a
// copy's Dst and not redrawn (outside every dirty rectangle).
func inCopy(x, y int, dirty []image.Rectangle, copies []CopyRegion) bool {
	p := image.Pt(x, y)
	for _, d := range dirty {
		if p.In(d) {
			return false
		}
	}
	for _, c := range copies {
		if p.In(c.Dst) {
			return true
		}
	}
	return false
}

// verifyLogEvery is how many Render calls apart verify failures are
// logged at most when they do not fail the frame (SetVerifyStrict).
const verifyLogEvery = 120

// SetVerifyStrict sets what a verify mode difference does: strict, Render
// returns the *VerifyError (and the next frame is drawn in full); not
// strict, Render logs it (at most once every verifyLogEvery frames, with
// the count of the others) and returns nil, so the frame — the device's
// target holds the correct full redraw by then — is presented. The
// default is strict in test binaries (testing.Testing) and not strict
// otherwise: ui.Runner does not present a frame whose Render failed, so a
// strict development build would drop correctly drawn frames.
func (r *Renderer) SetVerifyStrict(on bool) { r.verifyStrict = on }

// VerifyFailures returns how many partial frames verify mode found to
// differ from the full frame, and the last difference (nil if none).
func (r *Renderer) VerifyFailures() (int, *VerifyError) { return r.verifyFails, r.verifyLast }

// verifyFailed records a verify difference and returns what Render
// returns for it.
func (r *Renderer) verifyFailed(e *VerifyError) error {
	r.verifyFails++
	r.verifyLast = e
	if r.verifyStrict {
		return e
	}
	if r.verifyLogged == 0 || r.renders-r.verifyLogged >= verifyLogEvery {
		if r.verifySilent > 0 {
			r.logf("%v (and %d more since the last report)", e, r.verifySilent)
		} else {
			r.logf("%v", e)
		}
		r.verifyLogged, r.verifySilent = r.renders, 0
	} else {
		r.verifySilent++
	}
	return nil
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
