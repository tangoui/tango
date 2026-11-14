package diffview

import (
	"math"
	"sort"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor/diff"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
)

// The revert arrows and the merged overview ruler
// (docs/specs/diff-view.md, 3.9, 3.10).

const (
	revertW   = 16 // width of the revert arrows' column
	overviewW = 30 // the merged overview ruler (Monaco's 30px)
)

// changeTop returns the top of change i on the modified side, in
// ScrollTop coordinates: its first line, or the padding zone (side by
// side) or the deleted lines (inline) where its modified range is empty or
// preceded by them.
func (d *State) changeTop(i int) float64 {
	st := d.Modified
	c := d.res.Changes[i]
	n := st.Snapshot().LineCount()
	lh := float64(st.Viewport.LineHeight)
	if d.frameMode == Inline {
		if c.Modified.Start >= n {
			top, t, z := st.LineBlock(n - 1)
			return top + float64(t+z)*lh - float64(c.Original.Len())*lh
		}
		top := st.LineTop(c.Modified.Start)
		if !c.Original.Empty() {
			top -= float64(c.Original.Len()) * lh
		}
		return top
	}
	al := d.lay.al
	bi := d.lay.changeBlock(i)
	if al == nil || bi < 0 {
		return st.LineTop(c.Modified.Start)
	}
	return al.blockTop(diff.Modified, bi, d.geom(diff.Modified))
}

// changeBlock returns the alignment block of change i, -1 for none.
func (ly *layoutState) changeBlock(i int) int {
	al := ly.al
	if al == nil {
		return -1
	}
	if ly.blockOf == nil || ly.blockOfAl != al {
		ly.blockOf = ly.blockOf[:0]
		for bi, b := range al.blocks {
			if b.change >= 0 {
				for len(ly.blockOf) <= b.change {
					ly.blockOf = append(ly.blockOf, -1)
				}
				ly.blockOf[b.change] = bi
			}
		}
		ly.blockOfAl = al
	}
	if i < 0 || i >= len(ly.blockOf) {
		return -1
	}
	return ly.blockOf[i]
}

// visibleChanges calls fn with the changes whose modified lines may show
// in the modified side's viewport and their tops relative to the
// viewport's top edge.
func (d *State) visibleChanges(fn func(i int, y float32)) {
	st := d.Modified
	lh := st.Viewport.LineHeight
	if !d.hasRes || lh <= 0 {
		return
	}
	cs := d.res.Changes
	top := st.ScrollTop()
	first, _ := st.LineAtTop()
	rows := int(st.Viewport.Height/lh) + 2
	i := sort.Search(len(cs), func(i int) bool { return cs[i].Modified.End >= first })
	for ; i < len(cs); i++ {
		y := float32(d.changeTop(i) - top)
		if y > st.Viewport.Height {
			break
		}
		if cs[i].Modified.Start > first+rows*4 && y > st.Viewport.Height {
			break
		}
		fn(i, y)
	}
}

// revertColumn builds the column of revert arrows left of the modified
// editor: an arrow at the top of every change, which reverts it when
// clicked (only while the modified side is editable and no HunkActions
// are set).
func (w *ViewW) revertColumn() {
	d := w.d
	if !d.Options.RenderMarginRevertIcon || d.actions != nil || d.Modified.Options.ReadOnly {
		return
	}
	c := w.c
	var box *ui.Box
	box = ui.Custom(c, func(_, maxH float32) geom.Size {
		if math.IsInf(float64(maxH), 0) {
			maxH = 0
		}
		return geom.Size{W: revertW, H: maxH}
	}, func(l *paint.List, r geom.Rect) {
		cols := &d.lay.cols
		lh := d.Modified.Viewport.LineHeight
		hover := d.lay.revertHover
		l.Clip(r, geom.Radii{})
		d.visibleChanges(func(i int, y float32) {
			col := cols.revert
			if i == hover {
				col = cols.revertHover
			}
			cy := r.Y + y + lh/2
			cx := r.X + r.W/2
			const a = 4
			l.Line(geom.Pt(cx-a, cy), geom.Pt(cx+a, cy), 1.5, col)
			l.Line(geom.Pt(cx+a-3, cy-3), geom.Pt(cx+a, cy), 1.5, col)
			l.Line(geom.Pt(cx+a-3, cy+3), geom.Pt(cx+a, cy), 1.5, col)
		})
		l.Pop()
	}, layout.W(revertW))
	box.Semantics(0, d.lay.texts.revert).A11yHidden()
	ia := box.Interact(input.CursorDefault)
	at := -1
	if p, in := c.Pointer(); in && ia.Hovered {
		at = d.revertAt(p.Y - box.Rect().Y)
	}
	if at != d.lay.revertHover {
		d.lay.revertHover = at
	}
	if ia.Clicked && at >= 0 {
		_ = d.Revert(at)
	}
	w.revertID = box.ID()
}

// revertAt returns the change whose arrow is at y (relative to the
// column's top), -1 for none.
func (d *State) revertAt(y float32) int {
	lh := d.Modified.Viewport.LineHeight
	at := -1
	d.visibleChanges(func(i int, top float32) {
		if y >= top && y < top+lh {
			at = i
		}
	})
	return at
}

// overviewRuler builds the merged overview ruler: the original's changes
// in the left lane, the modified's in the right, at the modified side's
// scale (the sides are aligned), and the viewport's slider; a press or a
// drag scrolls there.
func (w *ViewW) overviewRuler() {
	d := w.d
	c := w.c
	var box *ui.Box
	box = ui.Custom(c, func(_, maxH float32) geom.Size {
		if math.IsInf(float64(maxH), 0) {
			maxH = 0
		}
		return geom.Size{W: overviewW, H: maxH}
	}, func(l *paint.List, r geom.Rect) {
		d.paintOverview(l, r, c.Dragging(box.ID()) || box.Hovered())
	}, layout.W(overviewW))
	box.A11yHidden()
	ia := box.Interact(input.CursorDefault)
	if p, _ := c.Pointer(); (ia.PressedNow || c.Dragging(box.ID())) && d.hasRes {
		r := box.Rect()
		st := d.Modified
		if r.H > 0 {
			sh := st.ScrollHeight()
			y := float64(p.Y-r.Y) / float64(r.H) * sh
			st.SetScrollTop(y - float64(st.Viewport.Height)/2)
			d.lead = diff.Modified
			d.syncFrom(diff.Modified)
		}
	}
}

// overviewMarks caches the ruler's marks.
type overviewMarks struct {
	al        *alignment
	gen       uint64
	region    uint64
	h, sh     float64
	mode      Mode
	rects     [2][]geom.Rect // left, right lane; y and height only
	lineH     float32
	resultGen uint64
}

func (d *State) paintOverview(l *paint.List, r geom.Rect, hover bool) {
	cols := &d.lay.cols
	l.Fill(geom.Rect{X: r.X, Y: r.Y, W: 1, H: r.H}, geom.Radii{}, cols.border)
	if !d.hasRes {
		return
	}
	st := d.Modified
	sh := st.ScrollHeight()
	if sh <= 0 || r.H <= 0 {
		return
	}
	m := &d.lay.ov
	if m.al != d.lay.al || m.gen != d.gen || m.region != d.regionGen || m.h != float64(r.H) || m.sh != sh || m.lineH != st.Viewport.LineHeight {
		*m = overviewMarks{al: d.lay.al, gen: d.gen, region: d.regionGen, h: float64(r.H), sh: sh, lineH: st.Viewport.LineHeight}
		scale := float64(r.H) / sh
		for i, ch := range d.res.Changes {
			y0 := d.changeTop(i) * scale
			var y1 float64
			if bi := d.lay.changeBlock(i); bi >= 0 && d.lay.al != nil {
				y1 = d.lay.al.blockTop(diff.Modified, bi+1, d.geom(diff.Modified)) * scale
			} else {
				y1 = y0 + float64(max(ch.Modified.Len(), 1))*float64(st.Viewport.LineHeight)*scale
			}
			y1 = max(y1, y0+2)
			for lane, empty := range [2]bool{ch.Original.Empty(), ch.Modified.Empty()} {
				if empty {
					continue
				}
				rs := m.rects[lane]
				a, b := float32(math.Floor(y0)), float32(math.Ceil(y1))
				if k := len(rs); k > 0 && rs[k-1].Bottom() >= a {
					rs[k-1].H = max(rs[k-1].H, b-rs[k-1].Y)
				} else {
					rs = append(rs, geom.Rect{Y: a, H: b - a})
				}
				m.rects[lane] = rs
			}
		}
	}
	laneW := (r.W - 2) / 2
	for lane, rs := range m.rects {
		x := r.X + 1 + float32(lane)*laneW
		col := cols.overviewRemoved
		if lane == 1 {
			col = cols.overviewInserted
		}
		for _, q := range rs {
			l.Fill(geom.Rect{X: x + 1, Y: r.Y + q.Y, W: laneW - 1, H: q.H}, geom.Radii{}, col)
		}
	}
	// The viewport's slider.
	scale := float64(r.H) / sh
	y := float32(st.ScrollTop() * scale)
	h := max(float32(float64(st.Viewport.Height)*scale), 4)
	col := cols.slider
	if hover {
		col = cols.sliderHover
	}
	l.Fill(geom.Rect{X: r.X + 1, Y: r.Y + y, W: r.W - 1, H: h}, geom.Radii{}, col)
}
