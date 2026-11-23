package editor

import (
	"encoding/binary"
	"hash/maphash"
	"image/color"
	"math"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// Redraw hints (spec 6.2).
//
// After painting, the View compares what it drew with what it drew in its
// last paint and declares the difference to the Runner (ui.Context.Damage,
// ui.Box.ScrollShift), so a GPU renderer redraws only that:
//
//   - Each drawn row (a view line lo..hi: a document line, or one row of a
//     wrapped line) has a fingerprint of everything painted on it: its
//     model line and row, its text (the shaped chunks of a long line, the
//     row of a wrapped one) with its x, its token colors and styles, the
//     selection ranges on it and whether its line break is selected, the
//     current-line highlight in the text area and in the gutter, the
//     composition shown on it and its line number as shown (its value in
//     the relative and interval modes, its alignment, its color), its
//     folding decorations (the folding control shown, the placeholder and
//     background of a folded line), its indentation guides (their count
//     and the active one), its bracket pair guides (their kinds, places,
//     levels and whether they are in the active color; the active pair
//     follows the primary caret), the matched brackets' boxes on it and its
//     display decorations (the occurrences of the word at the caret, the
//     selection's matches, suspicious characters, links, color swatches:
//     their kinds, places and swatch colors; paint.go), and the Model's
//     decorations touching its line (their options' fingerprints and
//     places: the backgrounds, inline styles, gutter marks, margin
//     backgrounds and line number colors; decorations.go). Colored
//     brackets and control characters are run colors; whitespace marks
//     follow from the text, the options and the selections.
//     A view zone's row (zones.go) has instead the zone's owner, ID,
//     Version, size and the row in it (with the horizontal scroll for a
//     Fixed zone); a zone without a Version has the fingerprint of the
//     drawing commands its callbacks emitted this frame (zone_fp.go), and a
//     drawing it cannot describe redraws the whole view.
//     A row whose fingerprint changed, or that is drawn in only one of the
//     two paints, is damaged at its place in this paint, padded by rowPad for
//     glyphs and strokes reaching past the line box. With an unchanged
//     viewport and row mapping, damage can be limited to the old and new
//     ink when neither paint has unbounded decorations and the full-line
//     and gutter layers are unchanged. An edit damages the changed lines and,
//     when the line count changed, every row below them (their line index now
//     holds other text); a selection or cursor change damages the rows of
//     the old and the new selections.
//   - Marks drawn over rows that change on their own (the carets, which
//     blink, and the drag-and-drop insertion bar) are compared as
//     rectangles: each one in only one of the two paints is damaged. A
//     blink frame damages only the carets.
//   - A vertical scroll is a ScrollShift of the whole view (gutter and text
//     move together), a horizontal one a ScrollShift of the text area; the
//     Runner damages the strip the copy uncovers. The rows and marks of the
//     last paint are compared at the place the copy moved them to. A frame
//     that scrolls both ways, or farther than the region, damages the
//     whole view. While wrapping, view line numbers move when the rows of
//     lines above the top become known (viewlines.go), so the last paint's
//     top is located again through its anchor (its top model line and row)
//     in this paint's mapping; an anchor that no longer exists damages the
//     whole view.
//   - Anything else that changes the picture as a whole damages the whole
//     view, at its old and new rectangle, and declares nothing finer:
//     the view's rectangle or layout (gutter and text areas), the theme
//     (a different *theme.Theme), the options in effect (font size, line
//     numbers, tab size, read-only, word wrap...), the wrapping in effect
//     (its column follows the width; editor.action.toggleWordWrap), the
//     width of the line numbers (their digit count; a narrow view clips the
//     gutter, so its rectangle may not change), focus (selection colors),
//     the first paint after a frame that did not paint the view, and a
//     change of the folded regions (folding or unfolding hides or shows
//     lines: the mapping changes everywhere below them; folding.go), of an
//     owner's hidden areas (hidareas.go) or of the view zones' layout (a
//     zone added, removed, moved or resized by SetViewZones; zones.go).
//     The hidden lines moving with an edit are row changes like any other;
//     with hidden lines, as while wrapping, the last paint's top is
//     located through its anchor.
//
// A frame in which nothing changed declares nothing. Display elements added
// to the View must be covered by these rules: fingerprinted with their row,
// recorded as marks, or added to damageGlobal. Row layers must also be
// included in the ink bounds or make the affected rows fall back.

// damageGlobal is what makes the View's whole picture change when it
// differs between two paints.
type damageGlobal struct {
	rect, gutter, text geom.Rect
	colW, lh           float32
	theme              *theme.Theme
	opts               Options
	tab                int
	focused            bool
	wrap               wrapConfig
	numW               float32 // width of the line numbers
	foldGen            uint64  // foldState.gen: changes of the folded regions
	hidGen             uint64  // State.hidGen: changes of the hidden areas (hidareas.go)
	zoneGen            uint64  // State.zgen: zone layout changes (zones.go)
	zoneOpaque         uint64  // frames with zone drawings the fingerprint cannot describe
	lnFn               bool    // a LineNumbersFunc labels the lines (gutter.go)
	lnGen              uint64  // its generation
}

// damageRec is what the View drew in its last paint.
type damageRec struct {
	ok      bool
	g       damageGlobal
	topLine int
	// aLine, aRow: the model line and row at the top (topLine's anchor).
	aLine, aRow     int
	offsetY         float32
	scrollX         float32
	scrollRemainder bool     // the input position differs from the drawn position
	lo              int      // first row, -1 for none
	rows            []uint64 // fingerprints of rows lo, lo+1, ...
	lineCount       int
	scale           float32
	ink             []rowInk
	rowOps          []paint.Op
	marks           []geom.Rect
	sticky          geom.Rect // fixed header in this paint, never carried by ScrollShift
	fixed           []fixedPaint
}

// paintFrame is what paint drew this frame, for declareDamage.
type paintFrame struct {
	g         damageGlobal
	snap      buffer.Snapshot
	lines     []lineView
	vis       []visSel
	cur       []bool // rows with the current-line highlight (nil: none)
	curG      []bool // rows with the gutter's current-line highlight (nil: none)
	cmp       compView
	caretLine int
	offsetY   float32
	scrollX   float32
	marks     []geom.Rect
	// Width of the highlighted line box when painted with a border.
	lineBorderW float32
	// aLine, aRow: the model line and row at the top.
	aLine, aRow int
	fold        []uint8 // folding decorations per row (foldCtlExpanded ...)
	guides      []rowGuides
	bguides     []bgRow // bracket pair guides (nil: off)
	match       []matchRow
	decos       []uint64 // fingerprints of the rows' decorations (nil: none)
	udecos      []udRow  // the Model's decorations of the rows (nil: none; decorations.go)
	boundRows   bool
	rowOps      []paint.Op
}

// rowPad is how far, in logical pixels, a row's damage reaches past its
// line box: the current-line border straddles the box edge and some
// glyphs (accents, descenders of tall fonts) overhang it.
func rowPad(lh float32) float32 { return max(2, float32(math.Ceil(float64(lh)/4))) }

var rowSeed = maphash.MakeSeed()

// rowHashes returns the fingerprint of every row of pf (see above).
func (w *ViewW) rowHashes(pf *paintFrame, dst []uint64) []uint64 {
	dst = dst[:0]
	snap := pf.snap
	n := snap.LineCount()
	var h maphash.Hash
	h.SetSeed(rowSeed)
	var b [8]byte
	num := func(v int) {
		binary.LittleEndian.PutUint64(b[:], uint64(v))
		h.Write(b[:])
	}
	f32 := func(v float32) { num(int(math.Float32bits(v))) }
	for k := range pf.lines {
		lv := &pf.lines[k]
		h.Reset()
		num(lv.i)
		num(lv.row)
		if lv.zone != nil {
			// A view zone's row: the zone and its drawing (zones.go).
			w.zoneHash(lv.zone, pf.scrollX, num, func(s string) { h.WriteString(s) })
			dst = append(dst, h.Sum64())
			continue
		}
		if lv.i == pf.caretLine {
			num(1) // the active line number color
		}
		if w.lnFn != nil {
			if lv.row == 0 && pf.g.opts.lineNumbers() {
				if label, ok := w.lnFn(lv.i); ok {
					h.WriteString(label)
				}
			}
		} else if v, show, left := pf.g.opts.lineNumber(lv.i, pf.caretLine, n); show && lv.row == 0 {
			num(v)
			if left {
				num(1)
			}
		}
		num(len(lv.segs))
		for _, sg := range lv.segs {
			h.WriteString(lv.text[sg.b0:sg.b1])
			f32(sg.x0)
			num(sg.r0)
			num(sg.r1)
			for _, p := range sg.paints {
				c := p.fg
				num(int(c.R) | int(c.G)<<8 | int(c.B)<<16 | int(c.A)<<24)
				flags := 0
				for i, on := range [...]bool{p.underline, p.strike, p.bold, p.italic} {
					if on {
						flags |= 1 << i
					}
				}
				num(flags)
				num(p.r0)
				num(p.r1)
			}
		}
		for _, in := range lv.inj {
			if q := in.hint; q != nil {
				num(in.col)
				f32(in.w)
				h.WriteString(q.hint.Text)
				for _, c := range [...]color.NRGBA{q.hint.Foreground, q.hint.Background} {
					num(int(c.R) | int(c.G)<<8 | int(c.B)<<16 | int(c.A)<<24)
				}
			}
		}
		// Repaint current-line borders when a scroll or a change in the
		// widest line moves a border edge into or out of the text clip.
		if pf.cur != nil && pf.cur[k] {
			num(2)
			f32(pf.scrollX)
			f32(pf.lineBorderW)
		}
		// Selections on the row (as paint draws them).
		ls := snap.LineStart(lv.i)
		le := ls + len(lv.text)
		for _, vsl := range pf.vis {
			a, bb := vsl.a, vsl.b
			if a == bb || (vsl.primary && pf.cmp.on) || bb < ls || a > le {
				continue
			}
			if s0, s1 := lv.runeAt(max(a, ls)-ls), lv.runeAt(min(bb, le)-ls); s0 < s1 {
				num(3)
				num(s0)
				num(s1)
				if vsl.right {
					num(5) // the head drawn right of a gap (injected.go)
				}
			}
			if lv.breakSelected(bb, le, n) {
				num(4)
			}
		}
		if pf.curG != nil && pf.curG[k] {
			num(9)
		}
		if k < len(pf.guides) && pf.guides[k].level > 0 {
			num(8)
			num(pf.guides[k].level)
			num(pf.guides[k].active)
		}
		if k < len(pf.bguides) {
			for _, g := range pf.bguides[k].g {
				num(11)
				num(g.kind)
				f32(g.x0)
				f32(g.x1)
				num(g.level)
				if g.hl {
					num(1)
				}
			}
		}
		for _, m := range pf.match {
			if m.k == k {
				num(7)
				num(m.r0)
				num(m.r1)
			}
		}
		if k < len(pf.decos) && pf.decos[k] != 0 {
			num(10)
			h.Write(binary.LittleEndian.AppendUint64(b[:0], pf.decos[k]))
		}
		if k < len(pf.udecos) && pf.udecos[k].sig != 0 {
			// The Model's decorations of the row's line: their styles and
			// places (the text, gutter and margin layers alike).
			num(13)
			h.Write(binary.LittleEndian.AppendUint64(b[:0], pf.udecos[k].sig))
		}
		if k < len(pf.fold) && pf.fold[k] != 0 {
			num(6)
			num(int(pf.fold[k]))
			if pf.fold[k]&foldCtlExpanded != 0 {
				f32(w.foldOpacity)
			}
		}
		// The composition (inline: its text is in lv.text).
		if c := pf.cmp; c.on && lv.i == pf.caretLine {
			num(5)
			num(c.r0)
			num(c.r1)
			if c.overlay != nil {
				h.WriteString(w.vs.comp)
				f32(c.x0)
			}
		}
		dst = append(dst, h.Sum64())
	}
	return dst
}

// declareDamage declares what changed since the View's last paint (see
// above) and records this paint.
func (w *ViewW) declareDamage(l *paint.List, pf *paintFrame) {
	vs, c := w.vs, w.c
	prev := &vs.dmg
	v := w.st.Viewport
	// cur reuses the buffers of the paint before last.
	cur := damageRec{ok: true, g: pf.g, topLine: v.TopLine, aLine: pf.aLine, aRow: pf.aRow, offsetY: pf.offsetY, scrollX: pf.scrollX, lo: -1,
		scrollRemainder: pf.offsetY != v.OffsetY || pf.scrollX != v.ScrollX,
		rows:            w.rowHashes(pf, vs.dmgSpare.rows),
		lineCount:       pf.snap.LineCount(),
		scale:           c.PixelScale(),
		sticky:          vs.stickyRect,
		marks:           append(vs.dmgSpare.marks[:0], pf.marks...),
		fixed:           append(vs.dmgSpare.fixed[:0], vs.navPaint...)}
	cur.recordInk(pf, &vs.dmgSpare)
	// Fixed decorations are compared in screen space, independently of
	// scrolling rows. Blink-only frames must still damage only the caret.
	var fixedDirty [3]geom.Rect
	dirtyFixed := func(r geom.Rect) {
		k := 2
		if !r.Intersect(vs.minimapRect).Empty() {
			k = 0
		} else if !r.Intersect(vs.overviewRect).Empty() {
			k = 1
		}
		if fixedDirty[k].Empty() {
			fixedDirty[k] = r
		} else {
			fixedDirty[k] = fixedDirty[k].Union(r)
		}
	}
	oldFixed := make(map[fixedPaint]bool, len(prev.fixed))
	newFixed := make(map[fixedPaint]bool, len(cur.fixed))
	for _, p := range prev.fixed {
		oldFixed[p] = true
	}
	for _, p := range cur.fixed {
		newFixed[p] = true
		if !oldFixed[p] {
			dirtyFixed(p.rect)
		}
	}
	for _, p := range prev.fixed {
		if !newFixed[p] {
			dirtyFixed(p.rect)
		}
	}
	for _, r := range fixedDirty {
		c.Damage(r)
	}
	if len(pf.lines) > 0 {
		cur.lo = pf.lines[0].vi
	}
	defer func() { vs.dmg, vs.dmgSpare = cur, vs.dmg }()

	_, painted := c.RectOf(w.box.ID()) // the view was painted last frame
	if !prev.ok || !painted || prev.g != pf.g {
		if prev.ok {
			c.Damage(prev.g.rect)
		}
		c.Damage(pf.g.rect)
		return
	}
	g := pf.g
	lh := g.lh
	// The last paint's top and first row in this paint's view lines.
	ptop, plo := prev.topLine, prev.lo
	if vl := &w.st.vl; g.wrap.on() || vl.mapped() {
		if prev.aLine >= vl.n || prev.aRow < vl.minRow(prev.aLine) || prev.aRow >= vl.blockRows(prev.aLine) {
			c.Damage(g.rect)
			return
		}
		t := vl.viewOf(prev.aLine) + prev.aRow
		if plo >= 0 {
			plo += t - ptop
		}
		ptop = t
	}
	dy := float32(float64(cur.topLine-ptop)*float64(lh) + float64(cur.offsetY) - float64(prev.offsetY))
	dx := cur.scrollX - prev.scrollX
	var shift geom.Point
	switch {
	case dy != 0 && dx != 0, dy != 0 && absf(dy) >= g.rect.H, dx != 0 && absf(dx) >= g.text.W:
		c.Damage(g.rect)
		return
	case dy != 0:
		shift = geom.Pt(0, -dy)
		body := geom.Rect{X: g.gutter.X, Y: g.rect.Y, W: g.gutter.W + g.text.W, H: g.rect.H}
		top := max(body.Y, prev.sticky.Bottom(), cur.sticky.Bottom())
		body.H = max(0, body.Bottom()-top)
		body.Y = top
		w.box.ScrollShift(body, shift)
	case dx != 0:
		shift = geom.Pt(-dx, 0)
		body := g.text
		top := max(body.Y, prev.sticky.Bottom(), cur.sticky.Bottom())
		body.H = max(0, body.Bottom()-top)
		body.Y = top
		w.box.ScrollShift(body, shift)
	case cur.scrollRemainder || prev.scrollRemainder:
		// The input can move within one device pixel without moving the
		// drawn rows. An empty damage hint lets a retaining renderer skip
		// the frame if the Runner and other paint changes add no damage.
		l.Ops = append(l.Ops, paint.Damage{})
	}

	if !prev.sticky.Empty() && shift != (geom.Point{}) {
		c.Damage(prev.sticky.Offset(shift.X, shift.Y).Intersect(g.rect))
		c.Damage(cur.sticky)
		c.Damage(prev.sticky)
	}

	// Rows, at their place in this paint; runs of adjacent rows are
	// declared as one rectangle.
	ty := g.text.Y - cur.offsetY
	pad := rowPad(lh)
	run0, run1 := 0, -1 // the pending run of rows [run0, run1]
	var run spanX
	full := spanX{g.gutter.X, g.gutter.X + g.gutter.W + g.text.W}
	narrow := shift == (geom.Point{}) && cur.topLine == prev.topLine &&
		cur.aLine == prev.aLine && cur.aRow == prev.aRow && cur.lo == prev.lo &&
		cur.lineCount == prev.lineCount && len(cur.rows) == len(prev.rows) &&
		len(cur.ink) == len(cur.rows) && len(prev.ink) == len(prev.rows) &&
		cur.scale == prev.scale
	checkedTransform := false
	ink := rowInkMeasure{cache: &vs.glyphInk, scale: cur.scale}
	var strip geom.Rect // the strip a horizontal shift uncovers
	switch t := g.text; {
	case shift.X < 0:
		strip = geom.Rect{X: t.Right() + shift.X, Y: t.Y, W: -shift.X, H: t.H}
	case shift.X > 0:
		strip = geom.Rect{X: t.X, Y: t.Y, W: shift.X, H: t.H}
	}
	flush := func() {
		if run1 < run0 {
			return
		}
		y0 := ty + float32(run0-v.TopLine)*lh - pad
		y1 := ty + float32(run1+1-v.TopLine)*lh + pad
		r := geom.Rect{X: run.left, Y: y0, W: run.right - run.left, H: y1 - y0}.Intersect(g.rect)
		if r.Empty() {
			run0, run1 = 0, -1
			return
		}
		if strip.Empty() {
			c.Damage(r)
		} else {
			// Leave out the strip a horizontal shift uncovers (the Runner
			// damages it): a renderer merges overlapping dirty rectangles
			// into their bounding box, which here would be the whole
			// text area.
			if l := (geom.Rect{X: r.X, Y: r.Y, W: strip.X - r.X, H: r.H}); !l.Empty() {
				c.Damage(l)
			}
			if rr := (geom.Rect{X: strip.Right(), Y: r.Y, W: r.Right() - strip.Right(), H: r.H}); !rr.Empty() {
				c.Damage(rr)
			}
		}
		run0, run1 = 0, -1
	}
	row := func(i int, x spanX) {
		if x.empty() {
			return
		}
		if run1 >= run0 && i == run1+1 {
			run1 = i
			run.add(x.left, x.right)
			return
		}
		flush()
		run0, run1 = i, i
		run = x
	}
	old := func(i int) (uint64, bool) {
		k := i - plo
		if plo < 0 || k < 0 || k >= len(prev.rows) {
			return 0, false
		}
		return prev.rows[k], true
	}
	drawn := func(i int) bool { return cur.lo >= 0 && i >= cur.lo && i < cur.lo+len(cur.rows) }
	// Rows of the last paint before this paint's first and after its last
	// are no longer drawn: the copy (or nothing) left their pixels there.
	for k := range prev.rows {
		if i := plo + k; !drawn(i) && (cur.lo < 0 || i < cur.lo) {
			row(i, full)
		}
	}
	for k, hv := range cur.rows {
		i := cur.lo + k
		if ov, ok := old(i); !ok || ov != hv {
			x := full
			if narrow && !checkedTransform {
				checkedTransform = true
				narrow = !transformedPaint(l)
			}
			if narrow {
				if bounds, ok := ink.changedRows(prev, &cur, k); ok {
					x = bounds
				}
			}
			row(i, x)
		}
	}
	for k := range prev.rows {
		if i := plo + k; !drawn(i) && cur.lo >= 0 && i >= cur.lo {
			row(i, full)
		}
	}
	flush()

	// Marks: the last paint's moved with the copy.
	pm := markSet(prev.marks, shift)
	cm := markSet(cur.marks, geom.Point{})
	for _, m := range prev.marks {
		if m = m.Offset(shift.X, shift.Y); !cm.has(m) {
			c.Damage(m.Expand(1))
		}
	}
	for _, m := range cur.marks {
		if !pm.has(m) {
			c.Damage(m.Expand(1))
		}
	}
}

// marks is a set of mark rectangles; small sets are scanned (one caret,
// a few cursors), large ones (thousands of cursors) hashed.
type marks struct {
	rs  []geom.Rect
	off geom.Point
	m   map[geom.Rect]struct{}
}

// markSet returns the set of rs moved by off.
func markSet(rs []geom.Rect, off geom.Point) marks {
	s := marks{rs: rs, off: off}
	if len(rs) > 16 {
		s.m = make(map[geom.Rect]struct{}, len(rs))
		for _, r := range rs {
			s.m[r.Offset(off.X, off.Y)] = struct{}{}
		}
	}
	return s
}

func (s marks) has(r geom.Rect) bool {
	if s.m != nil {
		_, ok := s.m[r]
		return ok
	}
	for _, x := range s.rs {
		if x.Offset(s.off.X, s.off.Y) == r {
			return true
		}
	}
	return false
}

func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
