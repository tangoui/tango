package editor

import (
	"slices"
	"sort"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/paint"
)

// View zones (docs/specs/diff-view.md, 2.2; Monaco's IViewZone): blocks of
// whole rows a view inserts after model lines, drawn by their owner (the
// diff view's alignment padding and its "N hidden lines" bar). Their rows
// are view lines like the rows of a wrapped line: the view lines' tree
// counts them after their line's text rows (viewLines.zr, lead), so
// scrolling, the scroll bar, the overview ruler, the minimap and the
// mapping between model and view lines need nothing else. A zone follows
// its line through edits (zones_store.go: an anchor at the line's end,
// Left biased: Enter at the end of the line leaves the zone after it); a
// zone after a hidden line moves to the shown line before it when it
// ShowInHidden, or is not shown.
//
// The layout rows of a zone (lineView.zone) have no text: every pass over
// the laid out rows that reads text (selections, guides, decorations,
// whitespace, ...) skips them; the carets never go there (Up and Down move
// over the lines' text rows only).

// ViewZone is a block of whole rows after a model line, drawn by its
// owner. The editor copies it: change a zone by setting the owner's zones
// again.
//
// Experimental: part of the view zones API (docs/specs/diff-view.md).
type ViewZone struct {
	// ID identifies the zone within its owner (the redraw hints).
	ID uint64
	// AfterLine is the model line (zero-based) the zone follows; -1 puts
	// it before the first line.
	AfterLine int
	// Rows is the height in rows (line heights); a zone of no rows is not
	// shown.
	Rows int
	// MinWidth is the width the zone's content needs, in logical pixels
	// (the horizontal scroll range; Monaco's minWidthInPx).
	MinWidth float32
	// ShowInHidden keeps the zone shown when AfterLine is hidden (folded
	// or in a hidden area): it moves to the shown line before it.
	ShowInHidden bool

	// Version is the zone's drawing version: whatever Paint and
	// PaintGutter draw depends only on it, the ZonePaint geometry (size and
	// OriginX, relative to the zone's rectangle) and the theme, and the
	// owner increments it when that changes. 0 lets the editor fingerprint
	// the drawing commands the callbacks emit every frame instead.
	Version uint64
	// Fixed keeps the text area part from scrolling horizontally
	// (OriginX is the text area's left edge): the zone is redrawn on
	// every horizontal scroll.
	Fixed bool

	// Paint draws the text area part (nil: the editor background);
	// PaintGutter the gutter part (nil: the gutter background). They run
	// in the View's paint, clipped to the part's visible rectangle.
	Paint       func(l *paint.List, z ZonePaint)
	PaintGutter func(l *paint.List, z ZonePaint)

	// SuppressMouseDown leaves the caret where it is when the zone is
	// pressed (without it, as Monaco, the caret goes to the end of the
	// line above for a press in the zone's upper half, to the start of
	// the line below for the lower half).
	SuppressMouseDown bool
	// OnPointer, when set, sees the presses, releases and moves over the
	// zone (a release goes to the zone pressed).
	OnPointer func(e ZonePointer)
	// Cursor is the pointer shape over the zone (the default arrow when
	// zero).
	Cursor input.Cursor
	// Label names the zone for screen readers.
	Label string
}

// ZonePaint is where a zone part is drawn.
//
// Experimental: part of the view zones API (docs/specs/diff-view.md).
type ZonePaint struct {
	// Rect is the part's whole rectangle in window coordinates (the text
	// area's or the gutter's width, Rows line heights); rows outside the
	// view are clipped away.
	Rect geom.Rect
	// OriginX is the window x of content x 0: the text area's left edge
	// minus the horizontal scroll (the left edge for a Fixed zone and for
	// the gutter).
	OriginX float32
	// RowH is the row height.
	RowH float32
	// FirstRow is the first row of the zone in the view (rows before it
	// are scrolled above the top).
	FirstRow int
	Theme    *theme.Theme

	view *ViewW // the View painting the zone (ModelLines)
}

// ZoneArea is the part of a zone a pointer event is in.
//
// Experimental: part of the view zones API (docs/specs/diff-view.md).
type ZoneArea uint8

const (
	ZoneText ZoneArea = iota
	ZoneGutter
)

// ZonePointer is a pointer event over a zone.
//
// Experimental: part of the view zones API (docs/specs/diff-view.md).
type ZonePointer struct {
	Kind input.PointerKind // Press, Release or Move
	// Pos is relative to the top-left corner of the part's rectangle
	// (ZonePaint.Rect).
	Pos    geom.Point
	Area   ZoneArea
	Clicks int
}

// SetViewZones replaces owner's zones in this view; no zones remove the
// owner. AfterLine is clamped to the current text. The view keeps its top
// model line in place.
//
// Experimental: part of the view zones API (docs/specs/diff-view.md).
func (s *State) SetViewZones(owner string, zs []ViewZone) {
	if s.zones == nil {
		if len(zs) == 0 {
			return
		}
		s.zones = newZoneStore[ViewZone]()
	}
	specs := make([]zoneSpec[ViewZone], len(zs))
	for i := range zs {
		z := zs[i]
		specs[i] = zoneSpec[ViewZone]{id: z.ID, afterLine: z.AfterLine, rows: z.Rows, showInHidden: z.ShowInHidden, z: &z}
	}
	old := s.zones.get(owner)
	s.zones.set(owner, specs, s.Snapshot())
	if !sameZoneLayout(old, s.zones.get(owner)) {
		s.zgen++
	}
	s.track()
	vl := s.lines()
	line, row := vl.find(s.Viewport.TopLine)
	// A view scrolled to the very top stays there, showing the zones
	// added before the first line (as Monaco keeps scrollTop 0).
	atTop := s.Viewport.TopLine == 0 && s.Viewport.OffsetY <= 0
	s.syncZones()
	s.anchorTop(line, row)
	if atTop {
		s.Viewport.TopLine = 0
	}
	s.clampViewport()
}

// sameZoneLayout reports whether two zone lists take the same rows in the
// same places (the redraw hints: only their drawings may differ).
func sameZoneLayout(a, b []zoneSpec[ViewZone]) bool {
	return slices.EqualFunc(a, b, func(x, y zoneSpec[ViewZone]) bool {
		return x.id == y.id && x.afterLine == y.afterLine && x.rows == y.rows && x.showInHidden == y.showInHidden &&
			x.z.MinWidth == y.z.MinWidth && x.z.Fixed == y.z.Fixed
	})
}

// ViewZones returns owner's zones in this view, AfterLine moved with the
// edits since they were set, in the order set.
//
// Experimental: part of the view zones API (docs/specs/diff-view.md).
func (s *State) ViewZones(owner string) []ViewZone {
	if s.zones == nil {
		return nil
	}
	specs := s.zones.get(owner)
	if specs == nil {
		return nil
	}
	out := make([]ViewZone, len(specs))
	for i, sp := range specs {
		out[i] = *sp.z
		out[i].AfterLine = sp.afterLine
	}
	return out
}

// hasZones reports whether the view has zones.
func (s *State) hasZones() bool { return s.zones != nil && len(s.zones.owners) > 0 }

// zonePlaced is a zone where the view shows it.
type zonePlaced struct {
	line  int // the shown model line it follows; -1 before the first line
	rows  int
	owner string
	z     *ViewZone
}

// zoneRow is a laid out row of a zone: the zone and the row in it.
type zoneRow struct {
	p *zonePlaced
	k int
}

// syncZones places the zones in the view lines (synced to the current
// text, their hidden lines set): after their lines, or the shown line
// before a hidden one, in the order of their lines, then of their owners'
// names, then of the owners' lists.
func (s *State) syncZones() {
	vl := &s.vl
	s.zplaced = s.zplaced[:0]
	if !s.hasZones() {
		vl.setZones(0, nil)
		return
	}
	owners := make([]string, 0, len(s.zones.owners))
	for o := range s.zones.owners {
		owners = append(owners, o)
	}
	sort.Strings(owners)
	for _, o := range owners {
		for _, it := range s.zones.owners[o] {
			l := min(it.spec.afterLine, vl.n-1)
			if l >= 0 {
				if i := vl.hiddenAt(l); i >= 0 {
					if !it.spec.showInHidden {
						continue
					}
					l = vl.hid[i].a - 1 // the shown line above (-1: before the first)
				}
			}
			if it.spec.rows <= 0 {
				continue
			}
			s.zplaced = append(s.zplaced, zonePlaced{line: l, rows: it.spec.rows, owner: o, z: it.spec.z})
		}
	}
	slices.SortStableFunc(s.zplaced, func(a, b zonePlaced) int { return a.line - b.line })
	lead := 0
	var zr []zoneLineRows
	for _, p := range s.zplaced {
		switch k := len(zr); {
		case p.line < 0:
			lead += p.rows
		case k > 0 && zr[k-1].line == p.line:
			zr[k-1].rows += p.rows
		default:
			zr = append(zr, zoneLineRows{p.line, p.rows})
		}
	}
	vl.setZones(lead, zr)
}

// zoneAt returns the zone row at row of model line line (find's terms:
// negative before the first line, past the text rows after it), false for
// a text row.
func (s *State) zoneAt(line, row int) (zoneRow, bool) {
	vl := &s.vl
	after := line
	k := row
	if row < 0 {
		after, k = -1, row+vl.lead
	} else {
		text := vl.rowsOf(line)
		if row < text {
			return zoneRow{}, false
		}
		k = row - text
	}
	ps := s.zplaced
	i := sort.Search(len(ps), func(i int) bool { return ps[i].line >= after })
	for ; i < len(ps) && ps[i].line == after; i++ {
		if k < ps[i].rows {
			return zoneRow{&ps[i], k}, true
		}
		k -= ps[i].rows
	}
	return zoneRow{}, false
}

// zoneView returns the laid out row v, row row of model line i, a zone
// row.
func (w *ViewW) zoneView(i, row, v int) (lineView, bool) {
	zr, ok := w.st.zoneAt(i, row)
	if !ok {
		return lineView{}, false
	}
	return lineView{i: i, row: row, rows: w.st.vl.rowsOf(i), vi: v, zone: &zr}, true
}

// zoneMinWidth returns the widest MinWidth of the zones laid out.
func zoneMinWidth(lines []lineView) float32 {
	w := float32(0)
	for k := range lines {
		if z := lines[k].zone; z != nil {
			w = max(w, z.p.z.MinWidth)
		}
	}
	return w
}

// zoneGroups calls fn for each zone with rows among lines: the zone, the
// laid out index of its first row there and that row's index in the zone.
func zoneGroups(lines []lineView, fn func(p *zonePlaced, k0, first int)) {
	for k := 0; k < len(lines); k++ {
		z := lines[k].zone
		if z == nil {
			continue
		}
		fn(z.p, k, z.k)
		for k+1 < len(lines) && lines[k+1].zone != nil && lines[k+1].zone.p == z.p {
			k++
		}
	}
}

// paintZones draws the zones of the laid out rows in the text area (gutter
// false) or in the gutter, and records the fingerprints of the drawings
// of zones without a Version (vs.zfp, by zone).
func (w *ViewW) paintZones(l *paint.List, lines []lineView, lineY func(int) float32, gutter bool) {
	vs, st := w.vs, w.st
	lh := st.Viewport.LineHeight
	th := themeOf(st)
	zoneGroups(lines, func(p *zonePlaced, k0, first int) {
		z := p.z
		fn := z.Paint
		area, origin := vs.text, vs.text.X-st.Viewport.ScrollX
		if z.Fixed {
			origin = vs.text.X
		}
		if gutter {
			fn, area, origin = z.PaintGutter, vs.gutter, vs.gutter.X
		}
		if fn == nil {
			return
		}
		top := lineY(lines[k0].vi) - float32(first)*lh
		r := geom.Rect{X: area.X, Y: top, W: area.W, H: float32(p.rows) * lh}
		clip := r.Intersect(area)
		if clip.Empty() {
			return
		}
		l.Clip(clip, geom.Radii{})
		at := len(l.Ops)
		fn(l, ZonePaint{Rect: r, OriginX: origin, RowH: lh, FirstRow: first, Theme: th, view: w})
		if z.Version == 0 {
			// Relative to content x 0 and the zone's top: a drawing that
			// moves with the text is unchanged by scrolling (the scroll
			// copies it).
			fp, ok := opsFingerprint(l.Ops[at:], geom.Pt(origin, r.Y))
			if !ok {
				vs.zoneOpaque++ // undescribed drawing: redraw everything
			}
			if vs.zfp == nil {
				vs.zfp = map[*ViewZone]uint64{}
			}
			vs.zfp[z] = vs.zfp[z]*1099511628211 ^ fp
		}
		l.Pop()
	})
}

// zoneHash returns the fingerprint of a zone row (damage.go): the zone,
// its version (or the fingerprint of its drawing) and the row in it.
func (w *ViewW) zoneHash(z *zoneRow, scrollX float32, num func(int), str func(string)) {
	num(12)
	str(z.p.owner)
	num(int(z.p.z.ID))
	num(int(z.p.z.Version))
	num(z.p.rows)
	num(z.k)
	if z.p.z.Version == 0 {
		num(int(w.vs.zfp[z.p.z]))
	}
	if z.p.z.Fixed {
		num(int(int32(scrollX * 64)))
	}
}

// --- input ---

// zoneHit is a point over a zone.
type zoneHit struct {
	z    zoneRow
	area ZoneArea
	pos  geom.Point // relative to the part's rectangle
	r    geom.Rect  // the part's rectangle
}

// zoneUnder returns the zone part at window point p (the last painted
// geometry).
func (w *ViewW) zoneUnder(p geom.Point) (zoneHit, bool) {
	st, vs := w.st, w.vs
	if !st.hasZones() || !w.hasGeom() || !vs.rect.Contains(p) {
		return zoneHit{}, false
	}
	r := w.pointRow(p)
	if r < 0 || r >= st.viewCount() {
		return zoneHit{}, false
	}
	line, row := st.lines().find(r)
	z, ok := st.zoneAt(line, row)
	if !ok {
		return zoneHit{}, false
	}
	v := st.Viewport
	top := vs.text.Y - v.OffsetY + float32(r-z.k-v.TopLine)*v.LineHeight
	h := zoneHit{z: z, area: ZoneText}
	part := vs.text
	if vs.gutter.Contains(p) {
		h.area, part = ZoneGutter, vs.gutter
	} else if !vs.text.Contains(p) {
		return zoneHit{}, false
	}
	h.r = geom.Rect{X: part.X, Y: top, W: part.W, H: float32(z.p.rows) * v.LineHeight}
	h.pos = geom.Pt(p.X-h.r.X, p.Y-h.r.Y)
	return h, true
}

// zonePress handles a press over a zone: its OnPointer sees it; it
// reports whether the caret stays (SuppressMouseDown).
func (w *ViewW) zonePress(p pending) (handled bool) {
	h, ok := w.zoneUnder(p.pressPos)
	if !ok {
		return false
	}
	w.vs.zonePress = h.z.p.z
	if fn := h.z.p.z.OnPointer; fn != nil {
		fn(ZonePointer{Kind: input.Press, Pos: h.pos, Area: h.area, Clicks: p.clicks})
	}
	return h.z.p.z.SuppressMouseDown
}

// zonePointer delivers the release and the moves of the frame to the
// zones.
func (w *ViewW) zonePointer(p pending) {
	vs := w.vs
	if p.release && vs.zonePress != nil {
		z := vs.zonePress
		vs.zonePress = nil
		if h, ok := w.zoneUnder(p.relPos); ok && h.z.p.z == z && z.OnPointer != nil {
			z.OnPointer(ZonePointer{Kind: input.Release, Pos: h.pos, Area: h.area})
		} else if z.OnPointer != nil && w.st.zoneLive(z) {
			z.OnPointer(ZonePointer{Kind: input.Release, Pos: geom.Pt(-1, -1)})
		}
	}
	if p.ptr != vs.zonePtr {
		vs.zonePtr = p.ptr
		if h, ok := w.zoneUnder(p.ptr); ok && h.z.p.z.OnPointer != nil {
			h.z.p.z.OnPointer(ZonePointer{Kind: input.Move, Pos: h.pos, Area: h.area})
		}
	}
}

// zoneLive reports whether z is one of the view's zones.
func (s *State) zoneLive(z *ViewZone) bool {
	for i := range s.zplaced {
		if s.zplaced[i].z == z {
			return true
		}
	}
	return false
}

// zoneCaret returns where a press over zone row z at window y puts the
// caret (Monaco: the end of the line above for the zone's upper half, the
// start of the line below for the lower half).
func (w *ViewW) zoneCaret(snap buffer.Snapshot, h zoneHit) int {
	vl := &w.st.vl
	upper := h.pos.Y < h.r.H/2
	line := h.z.p.line
	if line < 0 {
		return snap.LineStart(vl.firstVisible())
	}
	if upper {
		return lineEnd(snap, line)
	}
	if nx := vl.nextVisible(line); nx < snap.LineCount() {
		return snap.LineStart(nx)
	}
	return snap.Len()
}

// zoneCursor returns the pointer shape over a zone at p, false when p is
// over no zone or the zone sets none.
func (w *ViewW) zoneCursor(p geom.Point) (input.Cursor, bool) {
	h, ok := w.zoneUnder(p)
	if !ok {
		return 0, false
	}
	return h.z.p.z.Cursor, true
}
