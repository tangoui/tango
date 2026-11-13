package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// navAbsRect computes the absolute rectangle of n from the solved layout
// (valid after layout even when n was not painted because it scrolled
// out of view).
func navAbsRect(n *node) geom.Rect {
	r := n.ln.Rect
	for p := n.parent; p != nil; p = p.parent {
		if s := p.scroll; s != nil {
			ox, oy := s.shift(p.ln.Rect.Size(), p.ln.Content)
			r.X -= ox
			r.Y -= oy
		}
		r.X += p.ln.Rect.X
		r.Y += p.ln.Rect.Y
	}
	return r
}

// ---------------------------------------------------------------------------
// Affix

type affixState struct {
	affixed bool
}

// AffixW pins its content to the viewport (or to a scroll container) once
// it would scroll past OffsetTop / OffsetBottom.
type AffixW struct {
	c         *Context
	top       float32
	bottom    float32
	hasTop    bool
	hasBottom bool
	target    ID
	st        *affixState
	changed   bool
}

// Affix starts a pinned block (default: offset top 0 in the window). Give
// it Target(scrollBox.ID()) when the page scrolls inside a container.
//
// ConfigProvider.Defaults("Affix", ...) keys: "offsetTop" (number).
func Affix(c *Context) *AffixW {
	a := &AffixW{c: c}
	if v, ok := p2bNum(c, "Affix", "offsetTop"); ok {
		a.OffsetTop(float32(v))
	}
	return a
}

// OffsetTop pins the content this far below the top of the viewport.
func (a *AffixW) OffsetTop(px float32) *AffixW { a.top, a.hasTop = px, true; return a }

// OffsetBottom pins the content this far above the bottom of the viewport.
func (a *AffixW) OffsetBottom(px float32) *AffixW { a.bottom, a.hasBottom = px, true; return a }

// Target uses a scroll container (ScrollBox.ID()) as viewport.
func (a *AffixW) Target(id ID) *AffixW { a.target = id; return a }

// Affixed reports whether the content is currently pinned.
func (a *AffixW) Affixed() bool { return a.st != nil && a.st.affixed }

// Changed reports a pin state change detected in the previous frame.
func (a *AffixW) Changed() bool { return a.changed }

func (a *AffixW) view() geom.Rect {
	c := a.c
	if a.target != 0 {
		if r, ok := c.nextRects[a.target]; ok {
			return r
		}
		if r, ok := c.rects[a.target]; ok {
			return r
		}
	}
	return geom.Rect{W: c.size.W, H: c.size.H}
}

// want reports whether content laid out at r should be pinned, and where.
func (a *AffixW) want(r geom.Rect) (bool, float32) {
	v := a.view()
	if !a.hasTop && !a.hasBottom {
		a.hasTop = true
	}
	if a.hasTop && r.Y < v.Y+a.top {
		return true, v.Y + a.top
	}
	if a.hasBottom && r.Bottom() > v.Bottom()-a.bottom {
		return true, v.Bottom() - a.bottom - r.H
	}
	return false, r.Y
}

// Body builds the pinned content.
func (a *AffixW) Body(fn func()) *AffixW {
	c := a.c
	ph := c.add("affix", "")
	ph.ln.Dir = layout.Column
	a.st = State[affixState](c, ph.id, nil)
	prev := a.st.affixed
	if !a.st.affixed {
		// In flow: content scrolls with the page until it reaches the edge.
		navBodyScoped(c, ph, ph.id, fn)
		ov := Overlay(c, OverlayOptions{Place: ScreenFill}, func() {})
		ov.n.hit, ov.n.blocks = false, false
		ov.n.paint = func(l *paint.List, _ geom.Rect) {
			if on, _ := a.want(navAbsRect(ph)); on {
				a.st.affixed = true
				c.WakeAt(c.now)
			}
		}
		return a
	}
	// Pinned: a placeholder keeps the space; the content lives in a layer.
	var clip, inner *node
	ov := Overlay(c, OverlayOptions{Place: ScreenFill}, func() {
		clip = c.add("affix-clip", "")
		clip.ln.Absolute, clip.ln.Pos = true, layout.Pos{Left: 0, Top: 0, Right: layout.Unset, Bottom: layout.Unset}
		clip.ln.Width, clip.ln.Height = 1, 1
		clip.clip = true
		c.body(clip, func() {
			inner = c.add("affix-inner", "")
			inner.ln.Absolute, inner.ln.Pos = true, layout.Pos{Left: 0, Top: 0, Right: layout.Unset, Bottom: layout.Unset}
			navBodyScoped(c, inner, ph.id, fn)
		})
	})
	ov.n.hit, ov.n.blocks = false, false
	ph.ln.Measure = func(maxW, maxH float32) geom.Size {
		link(inner)
		fw := maxW < geom.Inf
		sz := layout.SolveAt(&inner.ln, maxW, geom.Inf, fw, false)
		return sz
	}
	ov.n.paint = func(l *paint.List, _ geom.Rect) {
		r := navAbsRect(ph)
		on, y := a.want(r)
		if !on {
			a.st.affixed = false
			c.WakeAt(c.now)
		}
		v := a.view()
		if a.target == 0 {
			v = geom.Rect{W: c.size.W, H: c.size.H}
		}
		clip.ln.Rect = v
		layout.Solve(&inner.ln, r.W, r.H)
		inner.ln.Rect.X, inner.ln.Rect.Y = r.X-v.X, y-v.Y
	}
	a.changed = prev != a.st.affixed
	return a
}

// ---------------------------------------------------------------------------
// Anchor

// AnchorItem is one link of an Anchor; Key matches an AnchorSection.
type AnchorItem struct {
	Key, Title string
	Children   []AnchorItem
}

type anchorRegistry struct {
	ids map[string]ID
}

var anchorRegKey = hashID("anchor-registry")

func (c *Context) anchorReg() *anchorRegistry {
	r := State[anchorRegistry](c, anchorRegKey, func() anchorRegistry { return anchorRegistry{ids: map[string]ID{}} })
	return r
}

// AnchorSection wraps a block of page content that an Anchor link with the
// same key scrolls to and highlights.
func AnchorSection(c *Context, key string, fn func(), opts ...layout.Option) *Box {
	b := c.box("anchor-section", layout.Column, opts)
	c.anchorReg().ids[key] = b.n.id
	return b.Body(fn)
}

type anchorState struct {
	active  string
	clicked string // pinned while the smooth scroll runs
}

// AnchorW is an in-page table of contents that follows scrolling.
type AnchorW struct {
	c          *Context
	items      []AnchorItem
	target     ID
	offset     float32
	bounds     float32
	horizontal bool
	noInk      bool
	st         *anchorState
	clicked    string
	changed    bool
}

// Anchor starts a table of contents; call Target with the scroll container
// holding the AnchorSections, then Show.
func Anchor(c *Context, items []AnchorItem) *AnchorW {
	a := &AnchorW{c: c, items: items, bounds: 5}
	// ConfigProvider.Defaults("Anchor", ...) keys: "targetOffset" /
	// "offsetTop", "bounds" (numbers).
	if v, ok := p2bNum(c, "Anchor", "targetOffset"); ok {
		a.offset = float32(v)
	} else if v, ok := p2bNum(c, "Anchor", "offsetTop"); ok {
		a.offset = float32(v)
	}
	if v, ok := p2bNum(c, "Anchor", "bounds"); ok {
		a.bounds = float32(v)
	}
	return a
}

// Target sets the scroll container (ScrollBox.ID()).
func (a *AnchorW) Target(id ID) *AnchorW { a.target = id; return a }

// TargetOffset scrolls sections this far below the container top (fixed
// headers) and uses it for the active test.
func (a *AnchorW) TargetOffset(px float32) *AnchorW { a.offset = px; return a }

// Horizontal lays links out in a row with a bottom ink bar.
func (a *AnchorW) Horizontal() *AnchorW { a.horizontal = true; return a }

// NoInk hides the ink rail (Element "underline" off).
func (a *AnchorW) NoInk() *AnchorW { a.noInk = true; return a }

// Active returns the highlighted key.
func (a *AnchorW) Active() string { return a.st.active }

// Clicked returns the key clicked this frame.
func (a *AnchorW) Clicked() string { return a.clicked }

// Changed reports that the active link changed this frame.
func (a *AnchorW) Changed() bool { return a.changed }

func flattenAnchor(items []AnchorItem, level int, out *[]anchorFlat) {
	for _, it := range items {
		*out = append(*out, anchorFlat{it, level})
		flattenAnchor(it.Children, level+1, out)
	}
}

type anchorFlat struct {
	it    AnchorItem
	level int
}

// computeActive picks the last section whose top passed the offset line.
func (a *AnchorW) computeActive(flat []anchorFlat) string {
	c := a.c
	view, ok := c.rects[a.target]
	if !ok {
		view = geom.Rect{W: c.size.W, H: c.size.H}
	}
	reg := c.anchorReg()
	line := view.Y + a.offset + a.bounds
	best, bestY := "", float32(-1e9)
	for _, f := range flat {
		id, ok := reg.ids[f.it.Key]
		if !ok {
			continue
		}
		r, ok := c.rects[id]
		if !ok {
			continue
		}
		if r.Y <= line && r.Y >= bestY {
			best, bestY = f.it.Key, r.Y
		}
	}
	if best == "" && len(flat) > 0 {
		if s := navScrollState(c, a.target); s != nil && s.off.Y <= 0 {
			return flat[0].it.Key
		}
	}
	return best
}

// Show builds the anchor.
func (a *AnchorW) Show() *AnchorW {
	c := a.c
	th := c.ThemeFor(theme.CompAnchor)
	tk := &th.Components.Anchor
	el := navEl(c)
	var flat []anchorFlat
	flattenAnchor(a.items, 0, &flat)
	b := c.box("anchor", layout.Column, nil)
	b.n.setRole(a11y.RoleNavigation).setName(c.Locale().A11y.Navigation)
	a.st = State[anchorState](c, b.n.id, nil)
	if a.target != 0 {
		navScrollStep(c, a.target)
		tw := State[navScrollTween](c, hashID(a.target, "scroll-tween"), nil)
		if !tw.active {
			a.st.clicked = ""
		}
	}
	prev := a.st.active
	if a.st.clicked != "" {
		a.st.active = a.st.clicked
	} else if act := a.computeActive(flat); act != "" {
		a.st.active = act
	}
	var nodes []*node
	var active *node
	link := func(f anchorFlat) {
		it := f.it
		n := c.add("anchor-link", it.Title)
		nodes = append(nodes, n)
		st := c.style(tk.FontSize, text.Regular)
		indent := tk.LinkPaddingInlineStart * float32(1+f.level)
		if a.horizontal {
			indent = 0
		}
		lh := 22 + 2*tk.LinkPaddingBlock
		n.ln.Measure = func(maxW, _ float32) geom.Size {
			if a.horizontal {
				return geom.Size{W: text.Width(it.Title, st), H: lh}
			}
			return geom.Size{W: indent + text.Width(it.Title, st), H: lh}
		}
		n.ln.NoStretch = a.horizontal
		ia := c.interact(n, true, input.CursorPointer)
		isActive := it.Key == a.st.active
		if isActive {
			active = n
		}
		n.setRole(a11y.RoleLink).stateIf(isActive, a11y.Current)
		if f.level > 0 && c.a11yOn {
			lv := f.level + 1
			n.fill(semFunc(func(s *a11y.Node) { s.Level = lv }))
		}
		if ia.Clicked {
			a.clicked = it.Key
			a.st.clicked = it.Key
			if id, ok := c.anchorReg().ids[it.Key]; ok && a.target != 0 {
				if s := navScrollState(c, a.target); s != nil {
					sr, ok1 := c.rects[id]
					vr, ok2 := c.rects[a.target]
					if ok1 && ok2 {
						navScrollTo(c, a.target, float32(s.off.Y)+sr.Y-vr.Y-a.offset, th.DurationSlow+200e6)
					}
				}
			}
			c.invalid = true
		}
		n.paint = func(l *paint.List, r geom.Rect) {
			col := tk.LinkColor
			if ia.Hovered {
				col = tk.LinkHoverColor
			}
			if isActive {
				col = tk.LinkActiveColor
			}
			lay := text.Shape(it.Title, st, c.textOpts(text.Options{NoWrap: true, Ellipsis: true, MaxWidth: r.W - indent}))
			l.Glyphs(geom.Pt(c.inline(r).X(indent, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
			if ia.FocusVisible {
				l.Stroke(r, geom.Rad(th.RadiusSM), 2, th.FocusRing)
			}
		}
	}
	if a.horizontal {
		b.n.ln.Dir = layout.Row
		b.n.ln.Gap = tk.HorizontalGap
	} else {
		b.n.ln.Padding = geom.Insets{Top: tk.PaddingBlock, Bottom: tk.PaddingBlock}
	}
	b.Body(func() {
		for _, f := range flat {
			link(f)
		}
	})
	id := b.n.id
	b.PaintOver(func(l *paint.List, r geom.Rect) {
		if a.noInk && !a.horizontal {
			return
		}
		if a.horizontal {
			navHLine(l, r.X, r.Bottom()-1, r.W, tk.RailColor)
			if active != nil {
				// Logical offset of the active link from the start edge.
				s := c.inline(r)
				off := s.Off(active.abs.X)
				if s.rtl {
					off = s.Off(active.abs.Right())
				}
				x := c.Anim(id, "ink-x", off, th.DurationSlow)
				w := c.Anim(id, "ink-w", active.abs.W, th.DurationSlow)
				l.Fill(s.Rect(x, r.Bottom()-tk.RailWidth, w, tk.RailWidth), geom.Radii{}, tk.InkBarColor)
			}
			return
		}
		rail := tk.RailWidth
		// The rail runs along the start edge.
		rx := c.inline(r).X(0, rail)
		l.Fill(geom.Rect{X: rx, Y: r.Y, W: rail, H: r.H}, geom.Radii{}, tk.RailColor)
		if active != nil {
			y := c.Anim(id, "ink-y", active.abs.Y-r.Y, th.DurationSlow)
			if el {
				// Element: a short marker centered on the link.
				l.Fill(geom.Rect{X: rx, Y: r.Y + y + 7, W: rail, H: 16}, geom.Rad(1), tk.InkBarColor)
			} else {
				l.Fill(geom.Rect{X: rx, Y: r.Y + y + 4, W: rail, H: active.abs.H - 8}, geom.Radii{}, tk.InkBarColor)
			}
		}
	})
	a.changed = prev != "" && prev != a.st.active
	return a
}
