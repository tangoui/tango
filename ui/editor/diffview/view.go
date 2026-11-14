package diffview

import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/diff"
	"github.com/tangoui/tango/ui/layout"
)

// ViewW is the diff view of a State: in SideBySide mode the original's
// editor and the modified's side by side in a splitter (the panes are not
// mirrored right to left, like VS Code's), the revert arrows between
// them and the merged overview ruler at the right; in Inline mode the
// modified's editor alone with the deleted lines between its lines. Both
// editors scroll together, block by block.
type ViewW struct {
	c        *ui.Context
	d        *State
	box      *ui.Box
	ed       [2]*editor.ViewW
	mode     Mode
	changed  bool
	revertID ui.ID
}

// View shows d. The application holds d; call View every frame.
func View(c *ui.Context, d *State) *ViewW {
	w := &ViewW{c: c, d: d}
	inv := c.Invalidator()
	d.wakeFn.Store(&inv)
	d.inBuild = true
	defer func() { d.inBuild = false }()

	d.take()
	ly := &d.lay
	ly.rtl = c.IsRTL()
	ly.texts = textsOf(c.Locale())
	ly.cols = resolveColors(d.Modified.Theme, tokensOf(c))
	mode := d.modeFor(c)
	w.mode, d.frameMode = mode, mode
	d.applyOptions(mode)
	d.apply(mode)

	w.box = ui.Row(c, func() {
		if mode == SideBySide {
			ui.Splitter(c).Style(layout.Grow(1), layout.Physical()).Body(func() {
				ui.SplitterPanel(c).Min(80).Body(func() {
					w.ed[diff.Original] = w.editor(diff.Original)
				})
				ui.SplitterPanel(c).Min(80).Body(func() {
					ui.Row(c, func() {
						w.revertColumn()
						w.ed[diff.Modified] = w.editor(diff.Modified)
					}, layout.Grow(1), layout.Physical())
				})
			})
			if d.Options.RenderOverviewRuler {
				w.overviewRuler()
			}
			return
		}
		w.revertColumn()
		w.ed[diff.Modified] = w.editor(diff.Modified)
	}, layout.Grow(1), layout.Physical())

	// Apply both editors' input now, in the build, so that what it
	// scrolls is synchronised before either side paints.
	for _, e := range w.ed {
		if e != nil && e.Changed() {
			w.changed = true
		}
	}
	for s, e := range w.ed {
		if e != nil && e.Focused() {
			d.focus = diff.Side(s)
		}
	}
	if f := d.focusReq; f != nil {
		d.focusReq = nil
		if e := w.ed[*f]; e != nil {
			c.SetFocus(e.Box().ID())
			d.focus = *f
		}
	}
	d.revealSelections(mode)
	d.apply(mode)
	if mode == Inline {
		d.refreshDeleted()
	}
	if d.needSync && d.syncFrom(d.lead) {
		d.needSync = false
	}
	d.announce(c, mode)
	d.width = w.box.Rect().W
	return w
}

// editor builds side s's editor view.
func (w *ViewW) editor(s diff.Side) *editor.ViewW {
	d := w.d
	e := editor.View(w.c, d.side(s))
	e.Keymap(d.keymap(w.c)).Context(contextKeys)
	e.Box().Semantics(a11y.RoleTextbox, d.label(s))
	return e
}

// label returns side s's accessible name.
func (d *State) label(s diff.Side) string {
	if d.labels[s] != "" {
		return d.labels[s]
	}
	tx := &d.lay.texts
	name := d.models[s].URI()
	if name == "" {
		name = tx.unnamed
	}
	switch {
	case s == diff.Modified:
		return sprintf(tx.modified, name)
	case d.Options.OriginalEditable:
		return sprintf(tx.originalEditable, name)
	}
	return sprintf(tx.original, name)
}

// contextKeys adds the diff view's context keys to the editors'
// keybinding when clauses (Monaco's isInDiffEditor).
func contextKeys(key string) (any, bool) {
	if key == "isInDiffEditor" {
		return true, true
	}
	return nil, false
}

// modeFor returns the mode of this frame: Mode's override, else
// Options.Mode, switched to Inline while the view is narrower than
// InlineBreakpoint (UseInlineWhenSpaceIsLimited).
func (d *State) modeFor(c *ui.Context) Mode {
	if d.modeOver != nil {
		return *d.modeOver
	}
	m := d.Options.Mode
	if m == SideBySide && d.Options.UseInlineWhenSpaceIsLimited && d.width > 0 && d.width < d.Options.InlineBreakpoint {
		m = Inline
	}
	return m
}

// applyOptions sets the editor options the diff view owns (Monaco's
// diffEditorEditors overrides).
func (d *State) applyOptions(mode Mode) {
	m := &d.Modified.Options
	for _, st := range d.sides() {
		o := &st.Options
		o.Folding = false
		o.Minimap.Enabled = false
		if d.Options.HideUnchangedRegions.Enabled {
			o.StickyScroll.Enabled = false
		}
		if d.Options.DiffWordWrap != "" {
			o.WordWrap = d.Options.DiffWordWrap
		}
	}
	o := &d.Original.Options
	o.ReadOnly = !d.Options.OriginalEditable
	o.FontSize, o.Padding = m.FontSize, m.Padding
	lanes := 0
	if mode == Inline {
		lanes = d.lanes
	}
	o.OverviewRulerLanes, m.OverviewRulerLanes = 0, lanes
}

// Mode overrides the mode this frame (the frame is built again when it
// differs from the mode the view was built for).
func (w *ViewW) Mode(m Mode) *ViewW {
	w.d.modeOver = &m
	if m != w.mode {
		w.c.Invalidate()
	}
	return w
}

// Layout applies layout options to the view's box (it grows by default).
func (w *ViewW) Layout(opts ...layout.Option) *ViewW { w.box.Style(opts...); return w }

// Box returns the view's box.
func (w *ViewW) Box() *ui.Box { return w.box }

// Labels sets the accessible names of the two editors (default: "原文（只
// 读）— <URI>" and "修改后 — <URI>" from the locale).
func (w *ViewW) Labels(original, modified string) *ViewW {
	d := w.d
	d.labels = [2]string{original, modified}
	for s, e := range w.ed {
		if e != nil {
			e.Box().Semantics(a11y.RoleTextbox, d.label(diff.Side(s)))
		}
	}
	return w
}

// HunkActions lets the host offer actions on every change, drawn in a row
// above it (replacing the revert arrows); fn returns the actions of
// change i (none: no row). The rows are rebuilt when the diff changes;
// the frame is built again when HunkActions starts or stops being set.
func (w *ViewW) HunkActions(fn func(i int, ch diff.Change) []HunkAction) *ViewW {
	d := w.d
	on := fn != nil
	d.actions = fn
	if on != d.lay.actionOn {
		d.lay.actionOn = on
		w.c.Invalidate()
	}
	return w
}

// Changed reports whether either side's text was edited through the view
// this frame.
func (w *ViewW) Changed() bool { return w.changed }

// --- scrolling ---

// geom returns side s's vertical geometry for the alignment.
func (d *State) geom(s diff.Side) sideGeom {
	st := d.side(s)
	n := st.Snapshot().LineCount()
	lh := float64(st.Viewport.LineHeight)
	top, t, z := st.LineBlock(n - 1)
	return sideGeom{lineTop: st.LineTop, rowH: lh, lines: n, end: top + float64(t+z)*lh}
}

// scrolled is side s's OnDidScroll: the other side follows.
func (d *State) scrolled(s diff.Side, ev editor.ScrollEvent) {
	if e := d.expect[s]; e.ok && e.line == ev.Line && e.off == ev.Offset && e.left == float32(ev.ScrollLeft) {
		d.expect[s].ok = false // the position syncFrom set
		return
	}
	d.lead = s
	if !d.syncFrom(s) {
		d.needSync = true
	}
}

// syncFrom scrolls the other side to follow side s (alignment.syncTop:
// the same aligned block at the same offset from its top) and copies the
// horizontal position. It reports false when the sides cannot be
// synchronised yet (no diff or not painted).
func (d *State) syncFrom(s diff.Side) bool {
	al := d.lay.al
	if d.frameMode != SideBySide {
		return true
	}
	if al == nil || !d.hasRes {
		return false
	}
	lead, follow := d.side(s), d.side(1-s)
	if lead.Viewport.LineHeight <= 0 || follow.Viewport.LineHeight <= 0 {
		return false
	}
	top := al.syncTop(s, lead.ScrollTop(), d.geom(s), d.geom(1-s))
	follow.SetScrollTop(top)
	follow.SetScrollLeft(lead.ScrollLeft())
	line, off := follow.LineAtTop()
	d.expect[1-s] = scrollPos{line: line, off: off, left: follow.ScrollLeft(), ok: true}
	if !d.inBuild {
		d.wake() // in a paint: the other side may be drawn already
	}
	return true
}
