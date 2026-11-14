package ui

import (
	"strconv"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// Splitter

// SplitterPanelW is one pane of a Splitter.
type SplitterPanelW struct {
	_ [0]func()

	c           *Context
	size        float32 // default size in px (0 = share)
	percent     float32 // default size as fraction 0..1
	min, max    float32 // px limits
	collapsible bool
	toggle      bool   // CollapseToggle: one arrow that folds this pane
	collapseTip string // CollapseToggle tips (empty: locale Collapse / Expand)
	expandTip   string
	fixed       bool
	fn          func()
}

type splitterCtx struct{ panels []*SplitterPanelW }

var splitterKey = hashID("splitter-ctx")

// SplitterPanel adds a pane; call it inside Splitter.Body.
func SplitterPanel(c *Context) *SplitterPanelW {
	p := &SplitterPanelW{c: c}
	p.configDefaults() // Defaults("SplitterPanel", ...): collapsible, resizable
	if st := State[[]*splitterCtx](c, splitterKey, nil); len(*st) > 0 {
		s := (*st)[len(*st)-1]
		s.panels = append(s.panels, p)
	}
	return p
}

// DefaultSize sets the initial size in pixels.
func (p *SplitterPanelW) DefaultSize(px float32) *SplitterPanelW { p.size = px; return p }

// DefaultPercent sets the initial size as a fraction (0.3 = 30%).
func (p *SplitterPanelW) DefaultPercent(f float32) *SplitterPanelW { p.percent = f; return p }

// Min sets the minimum size in pixels.
func (p *SplitterPanelW) Min(px float32) *SplitterPanelW { p.min = px; return p }

// Max sets the maximum size in pixels.
func (p *SplitterPanelW) Max(px float32) *SplitterPanelW { p.max = px; return p }

// Collapsible shows collapse arrows on the adjacent bars.
func (p *SplitterPanelW) Collapsible() *SplitterPanelW { p.collapsible = true; return p }

// CollapseToggle shows a single arrow on the adjacent bar, like an IDE
// side bar: it points at this pane and collapses it, then flips and
// restores the saved size. Hovering the arrow shows collapseTip (expandTip
// while collapsed); the tips also name the button for screen readers and
// default to the locale's Collapse / Expand. Enter on the bar toggles it.
func (p *SplitterPanelW) CollapseToggle(collapseTip, expandTip string) *SplitterPanelW {
	p.toggle, p.collapseTip, p.expandTip = true, collapseTip, expandTip
	return p
}

// splitterToggleIcon is the CollapseToggle caret: towards the pane it
// collapses (start: left / up), flipped once that pane is collapsed.
func splitterToggleIcon(vertical, start, collapsed bool) *icon.Icon {
	towardsStart := start != collapsed
	switch {
	case vertical && towardsStart:
		return icon.CaretUp
	case vertical:
		return icon.CaretDown
	case towardsStart:
		return icon.CaretLeft
	}
	return icon.CaretRight
}

// Resizable(false) locks the bars next to this pane.
func (p *SplitterPanelW) Resizable(on bool) *SplitterPanelW { p.fixed = !on; return p }

// Body sets the pane content.
func (p *SplitterPanelW) Body(fn func()) *SplitterPanelW { p.fn = fn; return p }

type splitterState struct {
	frac      []float32 // current fractions (sum 1)
	saved     []float32 // sizes before collapsing
	init      bool
	guessed   bool // fractions came from the window size (no rect yet)
	dragStart []float32
	dragFrom  geom.Point
	dragBar   int
	dragging  bool    // a bar holds the pointer (for DragEnded)
	preview   float32 // Lazy: the fraction pane dragBar-1 gets on release
	previewOn bool
}

// SplitterW lays panes side by side with draggable bars.
type SplitterW struct {
	_ [0]func()

	c        *Context
	vertical bool
	opts     []layout.Option
	st       *splitterState
	changed  bool
	lazy     bool
	started  int                  // bar index whose drag started this frame (0: none)
	ended    int                  // bar index whose drag ended this frame (0: none)
	th       *theme.Theme         // c.ThemeFor(theme.CompSplitter), set by Body
	tk       *theme.SplitterToken // &th.Components.Splitter
}

// Splitter starts a split view; chain options, then Body with SplitterPanel calls.
//
// ConfigProvider.Defaults("Splitter", ...) keys: "layout" ("vertical").
func Splitter(c *Context) *SplitterW {
	return &SplitterW{c: c, vertical: Prop(c, "Splitter", "layout", "") == "vertical"}
}

// Vertical stacks panes top to bottom.
func (s *SplitterW) Vertical() *SplitterW { s.vertical = true; return s }

// Style applies layout options (give the splitter a size, e.g. layout.H(200)).
func (s *SplitterW) Style(opts ...layout.Option) *SplitterW { s.opts = opts; return s }

// Lazy makes a drag show only a preview line; the panes keep their sizes
// until the pointer is released, then the new ratio is committed at once
// (Changed reports it on that frame). Keyboard resizing is unaffected.
func (s *SplitterW) Lazy() *SplitterW { s.lazy = true; return s }

// DragStarted reports the bar (1-based: bar i sits before pane i) whose
// pointer drag began this frame.
func (s *SplitterW) DragStarted() (bar int, ok bool) { return s.started, s.started > 0 }

// DragEnded reports the bar whose pointer drag ended this frame (released
// or cancelled). In Lazy mode the sizes are already committed.
func (s *SplitterW) DragEnded() (bar int, ok bool) { return s.ended, s.ended > 0 }

// Dragging reports whether a bar is being dragged with the pointer.
func (s *SplitterW) Dragging() bool { return s.st != nil && s.st.dragging }

// Sizes returns the pane sizes as fractions.
func (s *SplitterW) Sizes() []float32 { return append([]float32(nil), s.st.frac...) }

// Changed reports a resize or collapse this frame.
func (s *SplitterW) Changed() bool { return s.changed }

// total is the length shared by the panes (the bars take
// SplitTriggerSize each; their visible line is 1px).
func (s *SplitterW) total(r geom.Rect, n int) float32 {
	t := r.W
	if s.vertical {
		t = r.H
	}
	return max(1, t-float32(n-1)*s.tk.SplitTriggerSize)
}

// Body builds the panes.
func (s *SplitterW) Body(fn func()) *Box {
	c := s.c
	s.th = c.ThemeFor(theme.CompSplitter)
	s.tk = &s.th.Components.Splitter
	st := State[[]*splitterCtx](c, splitterKey, nil)
	ctx := &splitterCtx{}
	*st = append(*st, ctx)
	fn()
	st = State[[]*splitterCtx](c, splitterKey, nil)
	*st = (*st)[:len(*st)-1]
	panels := ctx.panels
	dir := layout.Row
	if s.vertical {
		dir = layout.Column
	}
	b := c.box("splitter", dir, s.opts)
	s.st = State[splitterState](c, b.n.id, nil)
	n := len(panels)
	prevRect, known := c.rects[b.n.id]
	if !known {
		prevRect = geom.Rect{W: c.size.W, H: c.size.H}
	}
	total := s.total(prevRect, n)
	if !s.st.init || len(s.st.frac) != n {
		s.st.frac = make([]float32, n)
		s.st.saved = make([]float32, n)
		var used float32
		free := 0
		for i, p := range panels {
			switch {
			case p.percent > 0:
				s.st.frac[i] = p.percent
			case p.size > 0:
				s.st.frac[i] = p.size / total
			default:
				s.st.frac[i] = -1
				free++
			}
			if s.st.frac[i] > 0 {
				used += s.st.frac[i]
			}
		}
		for i := range s.st.frac {
			if s.st.frac[i] < 0 {
				s.st.frac[i] = max(0, 1-used) / float32(max(1, free))
			}
		}
		// Without a rect yet the fractions are a window-based guess: keep
		// them for this frame only and rebuild once the splitter is laid
		// out, so DefaultSize is measured against its own length. A
		// splitter still unpainted after that keeps the guess rather than
		// asking for frames forever.
		s.st.init = known || s.st.guessed
		if !s.st.init {
			s.st.guessed = true
			c.animating = true // no geometry yet: ask for one more frame
		}
	}
	limits := func(i int) (lo, hi float32) {
		lo, hi = panels[i].min/total, 1
		if panels[i].max > 0 {
			hi = panels[i].max / total
		}
		return
	}
	if s.lazy {
		st, tk, vertical := s.st, s.tk, s.vertical
		b.PaintOver(func(l *paint.List, r geom.Rect) {
			if !st.previewOn || st.dragBar < 1 || st.dragBar >= len(st.frac) {
				return
			}
			tot := s.total(r, len(st.frac))
			var off float32
			for k := 0; k < st.dragBar-1; k++ {
				off += st.frac[k]
			}
			off = (off+st.preview)*tot + float32(st.dragBar-1)*tk.SplitTriggerSize + tk.SplitTriggerSize/2
			w := tk.PreviewSize
			if vertical {
				l.Fill(geom.Rect{X: r.X, Y: r.Y + off - w/2, W: r.W, H: w}, geom.Rad(0), tk.PreviewColor)
			} else {
				l.Fill(geom.Rect{X: c.inline(r).X(off-w/2, w), Y: r.Y, W: w, H: r.H}, geom.Rad(0), tk.PreviewColor)
			}
		})
	}
	b.Body(func() {
		for i, p := range panels {
			i, p := i, p
			if i > 0 {
				s.bar(i, panels, total, limits)
			}
			pane := c.add("splitter-panel", "")
			pane.ln.Dir = layout.Column
			pane.ln.Grow = max(s.st.frac[i]*1000, 0.0001)
			pane.ln.HasBasis = true
			pane.ln.MinW, pane.ln.MinH = 0, 0
			pane.clip = true
			if s.st.frac[i] <= 0.0005 {
				pane.ln.Grow = 0
				pane.ln.Basis = 0
			}
			Key(c, i, func() { c.body(pane, p.fn) })
		}
	})
	return b
}

func (s *SplitterW) bar(i int, panels []*SplitterPanelW, total float32, limits func(int) (float32, float32)) {
	c, th, tk := s.c, s.th, s.tk
	barW := tk.SplitTriggerSize
	n := c.add("splitter-bar", "splitter-bar-"+strconv.Itoa(i))
	resizable := !panels[i-1].fixed && !panels[i].fixed
	if s.vertical {
		n.ln.Height = barW
	} else {
		n.ln.Width = barW
	}
	cur := input.CursorResizeEW
	if s.vertical {
		cur = input.CursorResizeNS
	}
	if !resizable {
		cur = input.CursorDefault
	}
	// The bar is a focusable separator whose value is the size of the
	// pane before it (percent); arrow keys move it, Home / End go to the
	// limits and Enter collapses / restores the start pane.
	n.ownKeys = true
	ia := c.interact(n, resizable, cur)
	n.setRole(a11y.RoleSeparator).setName(c.Locale().A11y.Splitter)
	if s.vertical {
		n.setState(a11y.Horizontal)
	} else {
		n.setState(a11y.Vertical)
	}
	st := s.st
	a, bIdx := i-1, i
	if resizable && c.a11yOn {
		lo, hi := limits(a)
		n.fill(splitterSem{st: st, a: a, lo: lo, hi: hi})
	}
	// Collapse arrows: two small buttons centered on the bar, or a single
	// toggle when a pane next to it uses CollapseToggle (tgt: that pane).
	tgt := -1
	if panels[a].toggle {
		tgt = a
	} else if panels[bIdx].toggle {
		tgt = bIdx
	}
	other := a + bIdx - tgt
	folded := func(k int) bool { return st.frac[k] <= 0.0005 && st.saved[k] > 0 }
	collapsible := panels[a].collapsible || panels[bIdx].collapsible || tgt >= 0
	nArrows := 2
	if tgt >= 0 {
		nArrows = 1
	}
	var arrows [2]Interaction
	var arrowNodes [2]*node
	if collapsible {
		c.body(n, func() {
			for k := 0; k < nArrows; k++ {
				label := [2]string{"collapse-start", "collapse-end"}[k]
				if tgt >= 0 {
					label = "collapse-toggle"
				}
				an := c.add("splitter-arrow", label)
				an.ln.Absolute = true
				w, h := float32(16), float32(20)
				if s.vertical {
					w, h = 20, 16
				}
				an.ln.Width, an.ln.Height = w, h
				if s.vertical {
					an.ln.Pos = flexbox.Pos{Left: 0, Top: barW/2 - h/2, Right: layout.Unset, Bottom: layout.Unset}
				} else {
					an.ln.Pos = flexbox.Pos{Left: barW/2 - w/2, Top: layout.Unset, Right: layout.Unset, Bottom: layout.Unset}
				}
				arrows[k] = c.interact(an, false, input.CursorPointer)
				arrowNodes[k] = an
				// Buttons for screen readers; keyboard users press Enter
				// on the bar.
				an.aKbd = true
				to := bIdx
				if k == 1 {
					to = a
				}
				name := c.Locale().Collapse
				if folded(to) {
					name = c.Locale().Expand
				}
				if tgt >= 0 {
					name = s.toggleTip(panels[tgt], folded(tgt))
				}
				an.setRole(a11y.RoleButton).setName(name)
			}
		})
	}
	collapse := func(toEnd bool) {
		from, to := a, bIdx // collapse "from" into "to"
		if toEnd {
			from, to = bIdx, a
		}
		if st.frac[to] <= 0.0005 && st.saved[to] > 0 {
			give := st.saved[to]
			st.frac[from] -= give
			st.frac[to] = give
			st.saved[to] = 0
		} else if st.frac[from] > 0.0005 {
			st.saved[from] = st.frac[from]
			st.frac[to] += st.frac[from]
			st.frac[from] = 0
		}
		s.changed = true
		c.invalid = true
	}
	// toggle folds the CollapseToggle pane into its neighbour or restores it.
	toggle := func() {
		if folded(tgt) {
			give := st.saved[tgt]
			st.frac[other] -= give
			st.frac[tgt] = give
			st.saved[tgt] = 0
		} else if st.frac[tgt] > 0.0005 {
			st.saved[tgt] = st.frac[tgt]
			st.frac[other] += st.frac[tgt]
			st.frac[tgt] = 0
		} else {
			return
		}
		s.changed = true
		c.invalid = true
	}
	switch {
	case tgt >= 0 && arrows[0].Clicked:
		toggle()
	case tgt < 0 && collapsible && (arrows[0].Clicked || arrows[1].Clicked):
		collapse(arrows[1].Clicked)
	}
	if tgt >= 0 && arrows[0].Hovered && !c.Dragging(n.id) {
		place := Bottom
		if s.vertical {
			place = InlineEnd
		}
		navTooltipBubble(c, arrowNodes[0].id, s.toggleTip(panels[tgt], folded(tgt)), place)
	}
	// clampA returns the fractions of panes a and b when the bar moves so
	// that pane a gets na, honoring both panes' limits.
	clampA := func(na float32, start []float32) (float32, float32) {
		loA, hiA := limits(a)
		loB, hiB := limits(bIdx)
		sum := start[a] + start[bIdx]
		na = max(loA, min(na, hiA))
		nb := sum - na
		if nb < loB {
			nb = loB
			na = sum - nb
		}
		if nb > hiB {
			nb = hiB
			na = sum - nb
		}
		return na, nb
	}
	// setA moves the bar so that pane a gets fraction na (clamped).
	setA := func(na float32, start []float32) {
		na, nb := clampA(na, start)
		if na != st.frac[a] {
			st.frac[a], st.frac[bIdx] = max(0, na), max(0, nb)
			st.saved[a], st.saved[bIdx] = 0, 0
			s.changed = true
			c.invalid = true
		}
	}
	if resizable && ia.Focused {
		step := 10 / total // 10px per arrow press
		cur := st.frac[a]
		target, moved := cur, false
		if act, _, ok := c.A11yAction(n.id); ok && (act == a11y.Increment || act == a11y.Decrement) {
			c.KeysFor(n.id) // the synthetic arrow key of the action
			target, moved = cur+step, true
			if act == a11y.Decrement {
				target = cur - step
			}
		} else if v, ok := c.takeSetNumber(n.id); ok {
			target, moved = float32(v/100), true
		}
		for _, k := range c.KeysFor(n.id) {
			if !k.Down {
				continue
			}
			// The bar follows the arrow visually: towards the start pane
			// (Left in LTR, Right in RTL, Up) shrinks it.
			prev, next := c.inlineKey(k.Key)
			switch {
			case prev || k.Key == input.KeyUp:
				target, moved = target-step, true
			case next || k.Key == input.KeyDown:
				target, moved = target+step, true
			}
			switch k.Key {
			case input.KeyHome:
				target, moved = 0, true
			case input.KeyEnd:
				target, moved = 1, true
			case input.KeyEnter, input.KeySpace:
				switch {
				case tgt >= 0:
					toggle()
				case collapsible:
					collapse(false)
				}
			}
		}
		if moved {
			setA(target, st.frac)
		}
	}
	// PressedNow holds for every build of the press frame; start once.
	if resizable && ia.PressedNow && !(st.dragging && st.dragBar == i) {
		st.dragStart = append(st.dragStart[:0], st.frac...)
		st.dragFrom = c.pointer
		st.dragBar = i
		st.dragging = true
		st.previewOn = false
		s.started = i
	}
	if st.dragging && st.dragBar == i && !c.Dragging(n.id) {
		// Released (or cancelled): a lazy drag commits its preview now,
		// unless the system cancelled the gesture.
		if s.lazy && st.previewOn && !c.PointerCancelled(n.id) && len(st.dragStart) == len(st.frac) {
			setA(st.preview, st.dragStart)
		}
		st.dragging, st.previewOn = false, false
		s.ended = i
		c.invalid = true
	}
	if resizable && c.Dragging(n.id) && st.dragBar == i && len(st.dragStart) == len(st.frac) {
		c.KeepPointer(n.id)
		// Logical delta: in RTL the start pane is on the right, so dragging
		// left grows it.
		d := c.pointer.X - st.dragFrom.X
		if c.IsRTL() {
			d = -d
		}
		if s.vertical {
			d = c.pointer.Y - st.dragFrom.Y
		}
		if s.lazy {
			na, _ := clampA(st.dragStart[a]+d/total, st.dragStart)
			st.preview, st.previewOn = na, true
		} else {
			setA(st.dragStart[a]+d/total, st.dragStart)
		}
	}
	id := n.id
	vertical := s.vertical
	n.paint = func(l *paint.List, r geom.Rect) {
		hov := c.Anim(id, "hover", b2f(ia.Hovered || c.Dragging(id)), th.DurationMid)
		line := tk.BarColor
		if vertical {
			navHLine(l, r.X, r.Y+r.H/2-0.5, r.W, line)
		} else {
			navVLine(l, r.X+r.W/2-0.5, r.Y, r.H, line)
		}
		if ia.FocusVisible {
			drawFocusRing(l, th, r, geom.Rad(th.RadiusSM))
		}
		if resizable {
			col := theme.Lerp(tk.HandleHoverColor, tk.HandleActiveColor, b2f(c.Dragging(id)))
			if hov > 0 || c.Dragging(id) {
				col = theme.Fade(col, float64(max(hov, b2f(c.Dragging(id)))))
				bs, bl := tk.SplitBarSize, tk.SplitBarDraggableSize
				if vertical {
					l.Fill(geom.Rect{X: r.X + r.W/2 - bl/2, Y: r.Y + r.H/2 - bs/2, W: bl, H: bs}, geom.Rad(bs/2), col)
				} else {
					l.Fill(geom.Rect{X: r.X + r.W/2 - bs/2, Y: r.Y + r.H/2 - bl/2, W: bs, H: bl}, geom.Rad(bs/2), col)
				}
			}
		}
		if collapsible {
			ah := max(hov, c.Anim(id, "arrow-hover", b2f(arrows[0].Hovered || arrows[1].Hovered), th.DurationMid))
			for k, an := range arrowNodes[:nArrows] {
				// Position the arrow boxes around the bar center for this
				// frame (a single toggle sits on the center).
				off := -26 + float32(k)*32
				if tgt >= 0 {
					off = -10
				}
				var ar geom.Rect
				if vertical {
					ar = geom.Rect{X: r.X + r.W/2 + off, Y: r.Y + r.H/2 - 8, W: 20, H: 16}
				} else {
					ar = geom.Rect{X: r.X + r.W/2 - 8, Y: r.Y + r.H/2 + off, W: 16, H: 20}
				}
				if tgt >= 0 && folded(tgt) {
					// The bar now lies on the splitter's edge (the folded
					// pane's side): grow the arrow away from that edge so a
					// clipping splitter does not cut it in half.
					atStart := tgt == a // physical left / top, unless RTL
					switch {
					case vertical && atStart:
						ar.Y = r.Y
					case vertical:
						ar.Y = r.Bottom() - ar.H
					case atStart != c.IsRTL():
						ar.X = r.X
					default:
						ar.X = r.Right() - ar.W
					}
				}
				an.ln.Rect = geom.Rect{X: ar.X - r.X, Y: ar.Y - r.Y, W: ar.W, H: ar.H}
				if ah <= 0.01 {
					continue
				}
				bg := theme.Fade(tk.CollapseBg, float64(ah))
				if arrows[k].Hovered {
					bg = theme.Fade(tk.CollapseHoverBg, float64(ah))
				}
				l.Fill(ar, geom.Rad(tk.CollapseRadius), bg)
				g := [2]*icon.Icon{icon.CaretLeft, icon.CaretRight}[k]
				if vertical {
					g = [2]*icon.Icon{icon.CaretUp, icon.CaretDown}[k]
				}
				if tgt >= 0 {
					g = splitterToggleIcon(vertical, tgt == a, folded(tgt))
				}
				// Horizontal carets point to the start / end pane (mirrored
				// in RTL).
				c.paintIcon(l, g, ar.Expand(-1), theme.Fade(tk.CollapseColor, float64(ah)), 0)
			}
		}
	}
	_ = layout.Row
}

// toggleTip names a CollapseToggle arrow: the pane's tip for its state,
// or the locale's Collapse / Expand.
func (s *SplitterW) toggleTip(p *SplitterPanelW, collapsed bool) string {
	if collapsed {
		if p.expandTip != "" {
			return p.expandTip
		}
		return s.c.Locale().Expand
	}
	if p.collapseTip != "" {
		return p.collapseTip
	}
	return s.c.Locale().Collapse
}

// splitterSem exposes a bar's position: the size of the pane before it in
// percent of the splitter, with its limits.
type splitterSem struct {
	st     *splitterState
	a      int
	lo, hi float32
}

func (m splitterSem) semantics(s *a11y.Node) {
	if m.a >= len(m.st.frac) {
		return
	}
	pct := func(f float32) float64 { return float64(int(f*1000+0.5)) / 10 }
	rangeSem(s, pct(m.st.frac[m.a]), pct(m.lo), pct(min(m.hi, 1)), 1)
	s.Value = strconv.FormatFloat(s.ValueNow, 'f', -1, 64) + "%"
	if s.States.Has(a11y.Focusable) {
		s.Actions |= a11y.Increment | a11y.Decrement | a11y.SetValue
	}
}
