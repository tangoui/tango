package graph

import (
	"fmt"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// GitColumns selects the text columns of a GitGraph besides the graph and
// the subject.
type GitColumns uint8

const (
	ColAuthor GitColumns = 1 << iota
	ColDate
	ColHash
	ColAll = ColAuthor | ColDate | ColHash
)

// GitGraphW builds a commit graph list; configure it with the modifiers
// and finish with Show.
type GitGraphW struct {
	c       *ui.Context
	commits []Commit
	height  float32
	rowH    float32
	laneW   float32
	maxLane int
	cols    GitColumns
	sel     *string
	opts    []layout.Option
	tweaks  []func(*Style)
	name    string
	key     any
}

// GitGraph starts a commit graph of commits (newest first, every commit
// above its parents, as git log prints them).
func GitGraph(c *ui.Context, commits []Commit) *GitGraphW {
	return &GitGraphW{c: c, commits: commits, height: 420, rowH: 30, laneW: 16, maxLane: 12, cols: ColAll}
}

// Height sets the list height (default 420).
func (w *GitGraphW) Height(h float32) *GitGraphW { w.height = h; return w }

// RowHeight sets the row height (default 30).
func (w *GitGraphW) RowHeight(h float32) *GitGraphW {
	if h > 0 {
		w.rowH = h
	}
	return w
}

// LaneWidth sets the lane spacing (default 16) and how many lanes are
// shown before the graph column is clipped (default 12).
func (w *GitGraphW) LaneWidth(px float32, maxLanes int) *GitGraphW {
	if px > 0 {
		w.laneW = px
	}
	if maxLanes > 0 {
		w.maxLane = maxLanes
	}
	return w
}

// Columns chooses the text columns (default ColAll).
func (w *GitGraphW) Columns(cols GitColumns) *GitGraphW { w.cols = cols; return w }

// Selected binds the selected commit hash.
func (w *GitGraphW) Selected(hash *string) *GitGraphW { w.sel = hash; return w }

// Layout applies layout options to the list box.
func (w *GitGraphW) Layout(o ...layout.Option) *GitGraphW { w.opts = append(w.opts, o...); return w }

// Style tweaks the resolved style for this list only.
func (w *GitGraphW) Style(fn func(*Style)) *GitGraphW { w.tweaks = append(w.tweaks, fn); return w }

// A11yLabel sets the accessible name (default Locale.Graph.GitGraph).
func (w *GitGraphW) A11yLabel(s string) *GitGraphW { w.name = s; return w }

// Key gives the list a stable identity when widgets around it change.
func (w *GitGraphW) Key(k any) *GitGraphW { w.key = k; return w }

// gitState is retained per commit graph.
type gitState struct {
	lay      *GitLayout
	sig      [4]any
	sel      int // selected row (when not bound)
	hover    int
	scrollTo int
}

// GitGraphView is a shown commit graph and what happened this frame.
type GitGraphView struct {
	*ui.Box
	commits []Commit
	st      *gitState
	clicked int
	dbl     int
}

// Clicked returns the commit clicked this frame.
func (v *GitGraphView) Clicked() (Commit, bool) { return v.at(v.clicked) }

// DoubleClicked returns the commit double-clicked this frame.
func (v *GitGraphView) DoubleClicked() (Commit, bool) { return v.at(v.dbl) }

// Hovered returns the commit under the pointer.
func (v *GitGraphView) Hovered() (Commit, bool) { return v.at(v.st.hover) }

// Layout returns the lane layout of the shown commits.
func (v *GitGraphView) Layout() *GitLayout { return v.st.lay }

// ScrollTo scrolls commit hash into view on the next frame.
func (v *GitGraphView) ScrollTo(hash string) {
	for i := range v.commits {
		if v.commits[i].Hash == hash {
			v.st.scrollTo = i
			return
		}
	}
}

func (v *GitGraphView) at(i int) (Commit, bool) {
	if i < 0 || i >= len(v.commits) {
		return Commit{}, false
	}
	return v.commits[i], true
}

// Show builds the list and handles this frame's input.
func (w *GitGraphW) Show() *GitGraphView {
	var v *GitGraphView
	if w.key != nil {
		ui.Key(w.c, w.key, func() { v = w.show() })
	} else {
		v = w.show()
	}
	return v
}

func (w *GitGraphW) layoutFor(st *gitState) *GitLayout {
	cs := w.commits
	sig := [4]any{len(cs), nil, "", ""}
	if len(cs) > 0 {
		sig[1], sig[2], sig[3] = &cs[0], cs[0].Hash, cs[len(cs)-1].Hash
	}
	if st.lay == nil || st.sig != sig {
		st.lay, st.sig = LayoutCommits(cs), sig
	}
	return st.lay
}

func (w *GitGraphW) show() *GitGraphView {
	c := w.c
	th := c.Theme
	sty := StyleOf(th)
	for _, f := range w.tweaks {
		f(sty)
	}
	loc := c.Locale()
	opts := append([]layout.Option{layout.H(w.height)}, w.opts...)
	outer := ui.Div(c, opts...)
	st := ui.State[gitState](c, outer.ID(), func() gitState { return gitState{sel: -1, hover: -1, scrollTo: -1} })
	lay := w.layoutFor(st)
	v := &GitGraphView{Box: outer, commits: w.commits, st: st, clicked: -1, dbl: -1}
	n := len(w.commits)

	selIdx := st.sel
	if w.sel != nil {
		selIdx = -1
		if *w.sel != "" {
			if selIdx = st.sel; selIdx < 0 || selIdx >= n || w.commits[selIdx].Hash != *w.sel {
				selIdx = -1
				for i := range w.commits {
					if w.commits[i].Hash == *w.sel {
						selIdx = i
						break
					}
				}
			}
		}
	}
	setSel := func(i int) {
		selIdx = i
		st.sel = i
		if w.sel != nil {
			*w.sel = ""
			if i >= 0 {
				*w.sel = w.commits[i].Hash
			}
		}
	}

	name := w.name
	if name == "" {
		name = loc.Graph.GitGraph
	}
	ia := outer.Focusable(input.CursorDefault)
	outer.Semantics(a11y.RoleListbox, name).A11y(func(s *a11y.Node) {
		s.Description = fmt.Sprintf(loc.Graph.CommitsFmt, n)
		s.RowCount = n
		if selIdx >= 0 && selIdx < n {
			s.Value = w.commits[selIdx].Subject
		}
	})
	if ia.Focused && n > 0 {
		page := max(1, int(w.height/w.rowH)-1)
		for _, k := range c.KeysFor(outer.ID()) {
			if !k.Down {
				continue
			}
			i := selIdx
			switch k.Key {
			case input.KeyDown:
				i++
			case input.KeyUp:
				i--
				if selIdx < 0 {
					i = 0
				}
			case input.KeyPageDown:
				i += page
			case input.KeyPageUp:
				i -= page
			case input.KeyHome:
				i = 0
			case input.KeyEnd:
				i = n - 1
			case input.KeyEnter:
				if selIdx >= 0 {
					v.dbl = selIdx
				}
				continue
			default:
				continue
			}
			i = max(0, min(n-1, i))
			if i != selIdx {
				setSel(i)
				v.clicked = i
				st.scrollTo = i
			}
		}
	}

	st.hover = -1
	now := c.Now()
	graphW := float32(min(max(lay.Lanes, 1), w.maxLane))*w.laneW + 8
	outer.Body(func() {
		vl := ui.VirtualList(c, n, w.rowH, func(i int) {
			row := ui.Div(c, layout.Grow(1))
			ria := row.Interact(input.CursorPointer)
			if ria.Hovered {
				st.hover = i
			}
			if ria.PressedNow {
				c.SetFocus(outer.ID())
			}
			if ria.Clicked {
				setSel(i)
				v.clicked = i
				if ria.Clicks >= 2 {
					v.dbl = i
				}
			}
			cm := &w.commits[i]
			row.Semantics(a11y.RoleOption, cm.Subject).A11yKeyboard()
			if i == selIdx {
				row.A11yState(a11y.Selected)
			}
			if c.A11yEnabled() {
				desc := cm.Author + " · " + fmtWhen(cm.Time, now, loc) + " · " + shortHash(cm.Hash)
				row.A11y(func(s *a11y.Node) { s.Description = desc })
			}
			gr := &lay.Rows[i]
			sel, hov := i == selIdx, ria.Hovered
			row.Paint(func(l *paint.List, r geom.Rect) {
				w.paintRow(l, r, cm, gr, sty, th, loc, now, graphW, sel, hov)
			})
		}, layout.Grow(1))
		vl.A11yLabel(name)
		if st.scrollTo >= 0 {
			vl.ScrollIntoView(min(st.scrollTo, n-1))
			st.scrollTo = -1
		}
		vl.Range() // build the rows now, so row clicks are reported by Show

	})
	if ia.FocusVisible {
		outer.PaintOver(func(l *paint.List, r geom.Rect) {
			l.Stroke(r.Expand(-1), geom.Rad(th.Radius), max(th.FocusRingWidth, 2), th.FocusOutline)
		})
	}
	return v
}

func shortHash(h string) string {
	if len(h) > 7 {
		return h[:7]
	}
	return h
}

// fmtWhen formats t relative to now within a month ("3 小时前"), else as a
// date.
func fmtWhen(t, now time.Time, loc *ui.Locale) string {
	if t.IsZero() {
		return ""
	}
	pf := loc.ProField
	d := now.Sub(t)
	switch {
	case d < 0:
		return t.Format("2006-01-02")
	case d < time.Minute:
		return pf.JustNow
	case d < time.Hour:
		return fmt.Sprintf(pf.AgoFmt, fmt.Sprintf(pf.MinuteFmt, int(d/time.Minute)))
	case d < 24*time.Hour:
		return fmt.Sprintf(pf.AgoFmt, fmt.Sprintf(pf.HourFmt, int(d/time.Hour)))
	case d < 30*24*time.Hour:
		return fmt.Sprintf(pf.AgoFmt, fmt.Sprintf(pf.DayFmt, int(d/(24*time.Hour))))
	}
	return t.Format("2006-01-02")
}

// Column widths of the commit list.
const (
	gitAuthorW = 120
	gitDateW   = 96
	gitHashW   = 72
	gitDot     = 4
)

func (w *GitGraphW) paintRow(l *paint.List, r geom.Rect, cm *Commit, gr *GitRow, sty *Style, th *theme.Theme, loc *ui.Locale, now time.Time, graphW float32, sel, hov bool) {
	c := w.c
	s := c.Inline(r)
	switch {
	case sel:
		l.Fill(r, geom.Rad(th.RadiusSM), sty.RowSel)
	case hov:
		l.Fill(r, geom.Rad(th.RadiusSM), sty.RowHover)
	}
	// Graph cell.
	gx := s.Rect(0, r.Y, graphW, r.H)
	l.Clip(gx, geom.Radii{})
	laneX := func(j int) float32 { return s.X(4+float32(j)*w.laneW+w.laneW/2, 0) }
	top, mid, bot := r.Y, r.Y+r.H/2, r.Y+r.H
	for _, ln := range gr.Lines {
		col := sty.lane(ln.Color)
		x0, x1 := laneX(ln.From), laneX(ln.To)
		var p *paint.Path
		switch ln.Part {
		case LineThrough:
			p = paint.NewPath().MoveTo(x0, top).LineTo(x0, bot)
		case LineIn:
			p = gitCurve(x0, top, x1, mid)
		default:
			p = gitCurve(x0, mid, x1, bot)
		}
		l.StrokePath(p, paint.StrokeStyle{Width: 1.5}, col)
	}
	dc := sty.lane(gr.Color)
	cx := laneX(gr.Lane)
	if gr.Merge {
		l.Path(paint.NewPath().Circle(cx, mid, gitDot), dc)
		l.Path(paint.NewPath().Circle(cx, mid, gitDot-1.75), sty.Canvas)
	} else {
		l.Path(paint.NewPath().Circle(cx, mid, gitDot), dc)
	}
	l.Pop()

	// Text columns from the end edge back.
	small := text.Style{Size: th.FontSizeSM}
	off := graphW + 4
	end := r.W - 8
	col := func(width float32, str string, st text.Style, tc theme.Color) {
		end -= width
		if str == "" {
			return
		}
		// Commit data keeps its own direction (DirAuto), like user input.
		lay := text.Shape(str, st, text.Options{NoWrap: true, MaxWidth: width - 12, Ellipsis: true})
		x := s.X(end, lay.Width)
		l.Glyphs(geom.Pt(x, r.Y+(r.H-lay.Height)/2), lay.Glyphs, tc)
	}
	if w.cols&ColHash != 0 {
		col(gitHashW, shortHash(cm.Hash), text.Style{Size: th.FontSizeSM, Mono: true}, sty.Hash)
	}
	if w.cols&ColDate != 0 {
		col(gitDateW, fmtWhen(cm.Time, now, loc), small, sty.Meta)
	}
	if w.cols&ColAuthor != 0 {
		col(gitAuthorW, cm.Author, small, sty.Meta)
	}
	// Ref badges, then the subject.
	for _, ref := range cm.Refs {
		if off > end-40 {
			break
		}
		off += w.paintRef(l, s, r, off, end-off, ref, th) + 6
	}
	subj := sty.Subject
	if sel && sty.RowSelText.A > 0 {
		subj = sty.RowSelText
	}
	if avail := end - off - 8; avail > 16 {
		lay := text.Shape(cm.Subject, text.Style{Size: th.FontSize}, text.Options{NoWrap: true, MaxWidth: avail, Ellipsis: true})
		l.Glyphs(geom.Pt(s.X(off, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, subj)
	}
}

// gitCurve joins (x0,y0) and (x1,y1) vertically: a line when the lanes
// match, else an S-curve.
func gitCurve(x0, y0, x1, y1 float32) *paint.Path {
	p := paint.NewPath().MoveTo(x0, y0)
	if x0 == x1 {
		return p.LineTo(x1, y1)
	}
	my := (y0 + y1) / 2
	return p.CubeTo(x0, my, x1, my, x1, y1)
}

// paintRef draws a ref badge at logical offset off and returns its width.
func (w *GitGraphW) paintRef(l *paint.List, s ui.InlineSpan, r geom.Rect, off, avail float32, ref Ref, th *theme.Theme) float32 {
	pal := th.Primary
	ic := icon.Branch
	switch ref.Kind {
	case RefRemote:
		pal = th.Success
	case RefTag:
		pal = th.Warning
		ic = icon.Tag
	}
	fs := text.Style{Size: th.FontSizeSM}
	lay := text.Shape(ref.Name, fs, text.Options{NoWrap: true, MaxWidth: max(avail-30, 20), Ellipsis: true})
	h := min(r.H-8, 20)
	bw := lay.Width + 12 + 16
	br := s.Rect(off, r.Y+(r.H-h)/2, bw, h)
	bg, fg, bd := pal.Bg, pal.Base, pal.Border
	if ref.Kind == RefHead {
		bg, fg, bd = pal.Base, th.TextOnPrimary, pal.Base
	}
	l.Fill(br, geom.Rad(th.RadiusSM), bg)
	l.Stroke(br, geom.Rad(th.RadiusSM), 1, bd)
	is := float32(12)
	ir := w.c.Inline(br).Rect(5, br.Y+(h-is)/2, is, is)
	w.c.PaintIcon(l, ic, ir, fg, 0)
	l.Glyphs(geom.Pt(w.c.Inline(br).X(5+is+4, lay.Width), br.Y+(h-lay.Height)/2), lay.Glyphs, fg)
	return bw
}
