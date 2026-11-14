package ui

import (
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// Typography additions on Label: Copyable, Editable and Expandable
// ellipsis. They extend the label's measure/paint in place, so they chain
// after any other Label modifier:
//
//	ui.Text(c, s).Copyable()
//	ui.Paragraph(c, long).Lines(2).Expandable(&open)
//	ui.Text(c, title).Editable(&title)

const typoIcon = 14 // suffix icon size
const typoGap = 4

type typoSuffix struct {
	kind string // copy | edit
	r    geom.Rect
}

type typoState struct {
	pass     uint64
	suffixes []typoSuffix
	rects    []geom.Rect // icon rects of the last paint
	copiedAt time.Time
	editing  bool
	draft    string
	linkR    geom.Rect
	started  bool
	edited   bool
	caretFix int
	origin   geom.Rect // label paint rect of the last frame (icon / link rects are inside it)
}

// typoButton adds an out-of-flow focusable button over a suffix icon or
// the expand link r of the label (painted by the label itself), so the
// action has an accessible name and keyboard access. It sits in the
// label's parent at the geometry of the last frame.
func (t *Label) typoButton(st *typoState, key, name string, r geom.Rect) (*node, Interaction) {
	c, th := t.c, t.c.Theme
	n := c.add("typo-"+key, "")
	n.setRole(a11y.RoleButton).setName(name)
	n.ln.NoStretch = true
	lr, ok1 := c.rects[t.n.id]
	var pr geom.Rect
	ok2 := false
	if t.n.parent != nil {
		pr, ok2 = c.rects[t.n.parent.id]
	}
	if ok1 && ok2 && !r.Empty() {
		x := lr.X - pr.X + r.X - st.origin.X
		if t.n.parent.ln.RTL {
			x = pr.W - x - r.W // Abs left is the parent's inline start (its right edge)
		}
		y := lr.Y - pr.Y + r.Y - st.origin.Y
		flexbox.Apply(&n.ln.Style, layout.Abs(x, y, layout.Unset, layout.Unset))
		n.ln.Width, n.ln.Height = r.W, r.H
	} else {
		flexbox.Apply(&n.ln.Style, layout.Abs(0, 0, layout.Unset, layout.Unset))
		n.ln.Width, n.ln.Height = 1, 1
		c.animating = true // geometry known after the first paint
	}
	ia := c.interact(n, true, input.CursorPointer)
	n.paint = func(l *paint.List, rr geom.Rect) {
		if ia.FocusVisible {
			drawFocusRing(l, th, rr, geom.Rad(th.RadiusSM))
		}
	}
	return n, ia
}

// suffixRect returns the icon rect of the suffix kind (last frame).
func (st *typoState) suffixRect(kind string) geom.Rect {
	for i, s := range st.suffixes {
		if s.kind == kind && i < len(st.rects) {
			return st.rects[i]
		}
	}
	return geom.Rect{}
}

func (t *Label) typo() *typoState {
	st := State[typoState](t.c, t.n.id^0x7790, nil)
	if st.pass != t.c.pass {
		st.pass = t.c.pass
		st.suffixes = st.suffixes[:0]
	}
	return st
}

func (t *Label) suffixWidth(st *typoState) float32 {
	return float32(len(st.suffixes)) * (typoGap + typoIcon)
}

// addSuffix registers a trailing icon and, on the first one, wraps the
// label's measure and paint to make room for the icons after the text.
func (t *Label) addSuffix(kind string) (*typoState, Interaction) {
	c := t.c
	st := t.typo()
	first := len(st.suffixes) == 0
	st.suffixes = append(st.suffixes, typoSuffix{kind: kind})
	if !first {
		return st, c.interact(t.n, false, input.CursorDefault)
	}
	measure, pnt := t.n.ln.Measure, t.n.paint
	t.n.ln.Measure = func(maxW, maxH float32) geom.Size {
		ex := t.suffixWidth(st)
		sz := measure(maxW-ex, maxH)
		sz.W += ex
		return sz
	}
	t.n.paint = func(l *paint.List, r geom.Rect) {
		if st.origin != r {
			c.animating = true // the action buttons follow next frame
		}
		st.origin = r
		ex := t.suffixWidth(st)
		inner := r
		inner.W -= ex
		rtl := c.IsRTL()
		if rtl {
			inner.X += ex // the icons follow the text at its inline end
		}
		pnt(l, inner)
		lay := text.Shape(t.s, t.st, t.opts(inner.W+0.5))
		x, y := r.X, r.Y
		if n := len(lay.Lines); n > 0 {
			ln := lay.Lines[n-1]
			x += ln.Width
			if rtl {
				x = inner.X + alignLine(t.align, inner.W, ln.Width, ln.RTL)
			}
			y += ln.Top
		}
		tk := &c.ThemeFor(theme.CompTypography).Components.Typography
		st.rects = st.rects[:0]
		for _, s := range st.suffixes {
			x += typoGap
			if rtl {
				x -= 2*typoGap + typoIcon // leftwards from the line start
			}
			ir := geom.Rect{X: x, Y: y + (lay.LineHeight-typoIcon)/2, W: typoIcon, H: typoIcon}
			st.rects = append(st.rects, ir)
			hov := c.pointerIn && ir.Expand(2).Contains(c.pointer) && c.hovered(t.n.id)
			col := func(idle theme.Color) theme.Color {
				if hov {
					return tk.ActionHoverColor
				}
				return idle
			}
			switch s.kind {
			case "copy":
				if !st.copiedAt.IsZero() && c.now.Sub(st.copiedAt) < 3*time.Second {
					c.paintIcon(l, icon.Check, ir, tk.CopiedIconColor, 0)
				} else {
					c.paintIcon(l, icon.Copy, ir, col(tk.CopyIconColor), 0)
				}
			case "edit":
				c.paintIcon(l, icon.Edit, ir, col(tk.EditIconColor), 0)
			}
			if !rtl {
				x += typoIcon
			}
		}
	}
	return st, c.interact(t.n, false, input.CursorDefault)
}

// suffixClicked reports a click on the i-th suffix icon (geometry of the
// previous frame).
func (t *Label) suffixClicked(st *typoState, ia Interaction, kind string) bool {
	if !ia.Clicked {
		return false
	}
	for i, s := range st.suffixes {
		if s.kind == kind && i < len(st.rects) && st.rects[i].Expand(3).Contains(t.c.pointer) {
			return true
		}
	}
	return false
}

func (t *Label) suffixHovered(st *typoState, ia Interaction, kind string) (geom.Rect, bool) {
	if !ia.Hovered {
		return geom.Rect{}, false
	}
	for i, s := range st.suffixes {
		if s.kind == kind && i < len(st.rects) && st.rects[i].Expand(3).Contains(t.c.pointer) {
			return st.rects[i], true
		}
	}
	return geom.Rect{}, false
}

// typoTip shows a small tooltip above rect r (relative to anchor id),
// styled by the Tooltip token like Tooltip (shadcn: px-3 py-1.5 text-xs,
// no shadow).
func typoTip(c *Context, anchor ID, r geom.Rect, tip string) {
	tth := c.ThemeFor(theme.CompTooltip)
	tk := &tth.Components.Tooltip
	ar, ok := c.anchorRect(anchor)
	if !ok {
		return
	}
	ov := Overlay(c, OverlayOptions{Anchor: anchor, Place: Top, Offset: 8, ShiftX: r.X + r.W/2 - (ar.X + ar.W/2)}, func() {
		tx := Text(c, tip).Color(tk.Color).NoWrap()
		if tk.ContentFontSize != c.Theme.FontSize {
			tx.Size(tk.ContentFontSize)
		}
	}, layout.PadVH(tk.PaddingBlock, tk.PaddingInline))
	ov.Bg(tk.Bg).Radius(tk.Radius).Shadow(tk.Shadow)
	ov.n.hit = false
	ov.n.over = func(l *paint.List, rr geom.Rect) {
		cx := r.X + r.W/2
		p := paint.NewPath().MoveTo(cx-6, rr.Bottom()-0.5).LineTo(cx, rr.Bottom()+5.5).LineTo(cx+6, rr.Bottom()-0.5).Close()
		l.Path(p, tk.Bg)
	}
}

// Copyable adds a copy icon after the text; clicking it writes the text
// to the clipboard and shows a check mark for three seconds.
func (t *Label) Copyable() *Label { return t.CopyText(t.s) }

// CopyText is Copyable with a different clipboard payload.
func (t *Label) CopyText(payload string) *Label {
	c := t.c
	st, ia := t.addSuffix("copy")
	name := c.Locale().Copy
	if !st.copiedAt.IsZero() && c.now.Sub(st.copiedAt) < 3*time.Second {
		name = c.Locale().Copied
	}
	_, bia := t.typoButton(st, "copy", name, st.suffixRect("copy"))
	if t.suffixClicked(st, ia, "copy") || bia.Clicked {
		if c.win != nil {
			c.win.WriteClipboard(payload)
		}
		st.copiedAt = c.now
		c.invalid = true
	}
	copied := !st.copiedAt.IsZero() && c.now.Sub(st.copiedAt) < 3*time.Second
	if copied {
		c.WakeAt(st.copiedAt.Add(3 * time.Second))
	}
	if r, ok := t.suffixHovered(st, ia, "copy"); ok {
		tip := c.Locale().Copy
		if copied {
			tip = c.Locale().Copied
		}
		typoTip(c, t.n.id, r, tip)
	}
	return t
}

// Copied reports whether the copy icon was clicked within the last 3s.
func (t *Label) Copied() bool {
	st := t.typo()
	return !st.copiedAt.IsZero() && t.c.now.Sub(st.copiedAt) < 3*time.Second
}

// Editable adds an edit icon; clicking it swaps the text for an input.
// Enter or losing focus commits into *val, Escape cancels.
func (t *Label) Editable(val *string) *Label {
	c, th := t.c, t.c.Theme
	st := t.typo()
	if st.editing {
		// Turn the label node into a container hosting the editor.
		t.n.ln.Measure = nil
		t.n.paint = nil
		st.edited = false
		c.body(t.n, func() {
			in := Input(c, &st.draft).Width(max(240, text.Width(st.draft, t.st)+48)).A11yLabel(c.Locale().Edit)
			if !st.started {
				in.Focus()
				st.started = true
				st.caretFix = 2
			}
			if st.caretFix > 0 {
				// Focus selects all; Ant puts the caret at the end instead.
				st.caretFix--
				c.deferred = append(c.deferred, func() { in.e.anc = in.e.cur })
			}
			commit := in.Submitted()
			if c.Focused() == in.ID() && c.Escape() {
				st.editing = false
				c.invalid = true
				return
			}
			if commit || (c.Focused() != in.ID() && st.started && c.pressedAny) {
				*val = st.draft
				st.editing, st.edited = false, true
				c.invalid = true
			}
			_ = th
		})
		return t
	}
	st, ia := t.addSuffix("edit")
	_, bia := t.typoButton(st, "edit", c.Locale().Edit, st.suffixRect("edit"))
	if t.suffixClicked(st, ia, "edit") || bia.Clicked {
		st.editing, st.started, st.draft = true, false, *val
		c.invalid = true
	}
	if r, ok := t.suffixHovered(st, ia, "edit"); ok {
		typoTip(c, t.n.id, r, c.Locale().Edit)
	}
	return t
}

// Editing reports whether the label is being edited.
func (t *Label) Editing() bool { return t.typo().editing }

// Expandable adds an "展开" link when the text is cut by Lines(n); open
// is toggled by the link ("收起" collapses when open).
func (t *Label) Expandable(open *bool) *Label {
	c := t.c
	tk := &c.ThemeFor(theme.CompTypography).Components.Typography
	n := t.opt.MaxLines
	if n <= 0 {
		n = 1
	}
	st := State[typoState](c, t.n.id^0x7791, nil)
	ia := c.interact(t.n, false, input.CursorDefault)
	var bia Interaction
	if !st.linkR.Empty() {
		name := c.Locale().Expand
		if *open {
			name = c.Locale().Collapse
		}
		var bn *node
		bn, bia = t.typoButton(st, "expand", name, st.linkR)
		bn.setState(a11y.Expandable).stateIf(*open, a11y.Expanded)
	}
	if (ia.Clicked && st.linkR.Expand(2).Contains(c.pointer)) || bia.Clicked {
		*open = !*open
		c.invalid = true
	}
	expand, collapse := c.Locale().Expand, c.Locale().Collapse
	linkSt := t.st
	full := func(w float32) *text.Layout { return text.Shape(t.s, t.st, c.textOpts(text.Options{MaxWidth: w})) }
	lw := text.Width(expand, linkSt) + 8
	t.n.ln.Measure = func(maxW, _ float32) geom.Size {
		lay := full(maxW)
		if *open {
			h := lay.Height
			if len(lay.Lines) > 0 && lay.Lines[len(lay.Lines)-1].Width+lw > maxW {
				h += lay.LineHeight
			}
			return geom.Size{W: min(maxW, max(lay.Width, 1)), H: h}
		}
		lines := min(len(lay.Lines), n)
		return geom.Size{W: min(maxW, lay.Width), H: float32(lines) * lay.LineHeight}
	}
	t.n.paint = func(l *paint.List, r geom.Rect) {
		st.origin = r
		col := t.color()
		lay := full(r.W + 0.5)
		// In RTL the lines are start (right) aligned and the link follows
		// the text at its logical end, to the left.
		rtl := c.IsRTL()
		draw := func(lay *text.Layout, y float32, gs []paint.Glyph) {
			if !rtl {
				l.Glyphs(geom.Pt(r.X, y), gs, col)
				return
			}
			cp := *lay
			cp.Glyphs = gs
			paintAlignedGlyphs(l, &cp, func(i int) float32 {
				return r.X + alignLine(AlignStart, r.W, cp.Lines[i].Width, cp.Lines[i].RTL)
			}, y, col)
		}
		linkW := func(s string) float32 { return text.Width(s, linkSt) }
		link := func(x, y float32, s string) {
			ll := text.Shape(s, linkSt, c.textOpts(text.Options{NoWrap: true}))
			nr := geom.Rect{X: x, Y: y, W: ll.Width, H: lay.LineHeight}
			if nr != st.linkR {
				c.animating = true // the link button follows next frame
			}
			st.linkR = nr
			hov := c.pointerIn && st.linkR.Contains(c.pointer) && ia.Hovered
			lc := tk.LinkColor
			if hov {
				lc = tk.LinkHoverColor
			}
			l.Glyphs(geom.Pt(x, y), ll.Glyphs, lc)
		}
		if len(lay.Lines) <= n && !*open {
			draw(lay, r.Y, lay.Glyphs)
			if !st.linkR.Empty() {
				c.animating = true
			}
			st.linkR = geom.Rect{}
			return
		}
		if *open {
			draw(lay, r.Y, lay.Glyphs)
			last := lay.Lines[len(lay.Lines)-1]
			x, y := r.X+last.Width+8, r.Y+last.Top
			if last.Width+lw > r.W {
				x, y = r.X, r.Y+lay.Height
			}
			if rtl {
				x = r.Right() - last.Width - 8 - linkW(collapse)
				if last.Width+lw > r.W {
					x = r.Right() - linkW(collapse)
				}
			}
			link(x, y, collapse)
			return
		}
		// First n-1 lines as laid out, then the n-th line re-shaped with
		// room for the ellipsis and the link.
		cut := lay.Lines[n-1]
		var gs []paint.Glyph
		for _, g := range lay.Glyphs {
			if g.Y <= cut.Top {
				gs = append(gs, g)
			}
		}
		draw(lay, r.Y, gs)
		rest := string(lay.Runes[cut.Start:])
		ll := text.Shape(rest, t.st, c.textOpts(text.Options{MaxWidth: r.W - lw, NoWrap: true, Ellipsis: true}))
		if rtl {
			x := r.Right() - ll.Width
			l.Glyphs(geom.Pt(x, r.Y+cut.Top), ll.Glyphs, col)
			link(x-8-linkW(expand), r.Y+cut.Top, expand)
			return
		}
		l.Glyphs(geom.Pt(r.X, r.Y+cut.Top), ll.Glyphs, col)
		link(r.X+ll.Width+8, r.Y+cut.Top, expand)
	}
	return t
}
