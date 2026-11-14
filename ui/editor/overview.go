package editor

import (
	"image/color"
	"slices"
	"sort"

	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// OverviewMark is a model-line marker (zero based). Lane is a bit mask:
// 1 left, 2 middle, 4 right. A combination paints each selected lane.
type OverviewMark struct {
	Line  int
	Color color.NRGBA
	Lane  int
}

// SetOverviewMarks replaces owner's marks in this view, not in the shared
// Model. The input is copied; nil or empty removes the owner. The host
// requests a frame, as with changes to Options or selections.
func (s *State) SetOverviewMarks(owner string, marks []OverviewMark) {
	if len(marks) == 0 {
		delete(s.overview, owner)
		return
	}
	if s.overview == nil {
		s.overview = map[string][]OverviewMark{}
	}
	s.overview[owner] = slices.Clone(marks)
}

func (w *ViewW) paintOverview(l *paint.List, th *theme.Theme) {
	r := w.vs.overviewRect
	if r.Empty() {
		return
	}
	l.Clip(r, geom.Radii{})
	w.navigationFill(l, r, defaultColor(th, "editor.background"))
	if w.opts.OverviewRulerBorder {
		c := defaultColor(th, "editorOverviewRuler.border")
		w.navigationFill(l, geom.Rect{X: r.X, Y: r.Y, W: 1, H: r.H}, c)
	}
	n := w.st.Snapshot().LineCount()
	lanes := max(1, min(3, w.opts.OverviewRulerLanes))
	draw := func(line int, c color.NRGBA, lane int) {
		if line < 0 || line >= n || c.A == 0 {
			return
		}
		y := r.Y + (float32(w.navigationRow(line))*w.st.Viewport.LineHeight+w.st.Viewport.PadTop)/w.navigationHeight()*r.H
		for k := 0; k < 3; k++ {
			if lane&(1<<k) != 0 {
				// With fewer lanes, combine the logical left/middle/right lanes.
				col := min(lanes-1, k*lanes/3)
				width := (r.W - 1) / float32(lanes)
				w.navigationFill(l, geom.Rect{X: r.X + 1 + float32(col)*width, Y: y, W: width, H: 3}, c)
			}
		}
	}
	// Stable owner order makes overlapping markers deterministic.
	owners := make([]string, 0, len(w.st.overview))
	for owner := range w.st.overview {
		owners = append(owners, owner)
	}
	sort.Strings(owners)
	for _, owner := range owners {
		for _, m := range w.st.overview[owner] {
			draw(m.Line, m.Color, m.Lane)
		}
	}
	for idx, sel := range w.st.sels() {
		a, b := ordered(sel)
		if a != b {
			first, last := w.navigationOffset(a, false), w.navigationOffset(b, true)
			y := r.Y + (float32(first)*w.st.Viewport.LineHeight+w.st.Viewport.PadTop)/w.navigationHeight()*r.H
			height := max(3, float32(last-first+1)*w.st.Viewport.LineHeight/w.navigationHeight()*r.H)
			c := defaultColor(th, "editorOverviewRuler.selectionHighlightForeground")
			w.navigationFill(l, geom.Rect{X: r.X + 1, Y: y, W: r.W - 1, H: height}, c)
		}
		drawRow := w.navigationOffset(sel.Head, w.st.headUp(idx))
		y := r.Y + (float32(drawRow)*w.st.Viewport.LineHeight+w.st.Viewport.PadTop)/w.navigationHeight()*r.H
		w.navigationFill(l, geom.Rect{X: r.X + 1, Y: y, W: r.W - 1, H: 3}, defaultColor(th, "editorCursor.foreground"))
	}
	l.Pop()
}

// railInput owns clicks and captured drags on fixed navigation surfaces.
// It runs against the last painted geometry, before this frame is measured.
func (w *ViewW) railInput(p pending) bool {
	vs := w.vs
	handled := false
	if p.press {
		vs.railDrag = 0
		switch {
		case !vs.overviewRect.Empty() && vs.overviewRect.Contains(p.pressPos):
			vs.railDrag = 2
			handled = true
			w.navigateOverview(p.pressPos)
		case !vs.minimapRect.Empty() && vs.minimapRect.Contains(p.pressPos):
			vs.railDrag = 1
			handled = true
			vs.railThumbHeight = vs.minimapSlider.H
			vs.railGrab = vs.minimapSlider.H / 2
			if vs.minimapSlider.Contains(p.pressPos) {
				vs.railGrab = p.pressPos.Y - vs.minimapSlider.Y
			} else {
				w.navigateMinimapClick(p.pressPos)
			}
		case !vs.stickyRect.Empty() && vs.stickyRect.Contains(p.pressPos):
			handled = true
			vs.railDrag = 3
			for k, clip := range vs.stickyClips {
				if clip.Contains(p.pressPos) {
					w.jumpLine(vs.stickyLines[k])
					break
				}
			}
		}
	}
	if vs.railDrag != 0 {
		handled = true
		if p.drag {
			if vs.railDrag == 1 {
				w.navigateMinimap(p.dragPos)
			}
			if vs.railDrag == 2 {
				w.navigateOverview(p.dragPos)
			}
		}
		if p.release || p.cancel {
			vs.railDrag = 0
		}
	}
	return handled
}

func (w *ViewW) jumpLine(line int) {
	st := w.st
	snap := st.Snapshot()
	line = max(0, min(line, snap.LineCount()-1))
	st.MoveCaret(snap.LineStart(line), false)
	st.lines()
	st.Viewport.TopLine = st.vl.viewOf(line)
	st.Viewport.OffsetY = 0
	st.clampViewport()
}

func (w *ViewW) navigateOverview(p geom.Point) {
	r := w.vs.overviewRect
	if r.H > 0 {
		v := w.st.Viewport
		row := int(((p.Y-r.Y)/r.H*w.navigationHeight() - v.PadTop) / max(1, v.LineHeight))
		w.jumpViewRow(row)
	}
}
