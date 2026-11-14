package ui

import (
	"math"
	"strconv"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// StepItem is one entry of Steps.
type StepItem struct {
	_ [0]func()

	Title       string
	SubTitle    string
	Description string
	Icon        *icon.Icon // custom icon replaces the numbered circle
	Status      string     // override: "wait", "process", "finish", "error"
	Disabled    bool       // not clickable
}

// StepsW shows progress through a sequence: horizontal / vertical, dot
// style, small size, label placement, custom icons and clickable steps.
type StepsW struct {
	_ [0]func()

	c         *Context
	items     []StepItem
	current   int
	cur       *int
	status    string
	vertical  bool
	dot       bool
	size      ControlSize
	sizeSet   bool
	labelVert bool
	changed   bool
	finish    string            // Element finish-status ("finish" or "success")
	nav       bool              // navigation type (Element simple)
	percent   float64           // progress of the current step (<0: none)
	th        *theme.Theme      // c.ThemeFor(theme.CompSteps), set by Show
	tk        *theme.StepsToken // &th.Components.Steps
}

// Steps starts a step bar; chain options, then Show:
//
//	ui.Steps(c, items).Current(1).Status("error").Show()
//
// ConfigProvider.Defaults("Steps", ...) keys: "type" ("default" |
// "navigation"), "size" (ControlSize or "small"), "labelPlacement"
// ("vertical"), "direction" ("vertical"), "progressDot" (bool).
func Steps(c *Context, items []StepItem) *StepsW {
	s := &StepsW{c: c, items: items, finish: "finish", percent: -1}
	s.Type(Prop(c, "Steps", "type", ""))
	if v, ok := c.ConfigProp("Steps", "size"); ok {
		if sz, ok := v.(ControlSize); ok {
			s.size, s.sizeSet = sz, true
		}
	}
	if Prop(c, "Steps", "labelPlacement", "") == "vertical" {
		s.labelVert = true
	}
	s.defaults() // size names, direction, progressDot
	return s
}

// Type selects "default" or "navigation" (Ant type="navigation": steps as
// tabs with chevrons, the current one underlined; Element's simple steps
// look the same way in the Element theme; "simple" is accepted too).
func (s *StepsW) Type(t string) *StepsW { s.nav = t == "navigation" || t == "simple"; return s }

// Percent shows the progress (0..100) of the current step as a ring
// around its icon (Ant percent).
func (s *StepsW) Percent(p float64) *StepsW { s.percent = max(0, min(100, p)); return s }

// Current sets the 0-based current step.
func (s *StepsW) Current(i int) *StepsW { s.current = i; return s }

// Clickable binds the current step and lets the user click any step.
func (s *StepsW) Clickable(cur *int) *StepsW { s.cur = cur; s.current = *cur; return s }

// Status sets the status of the current step ("process", "error", "finish", "wait").
func (s *StepsW) Status(st string) *StepsW { s.status = st; return s }

// FinishStatus sets the status shown by finished steps ("success" paints
// them green, Element finish-status).
func (s *StepsW) FinishStatus(st string) *StepsW { s.finish = st; return s }

// Vertical stacks the steps.
func (s *StepsW) Vertical() *StepsW { s.vertical = true; return s }

// Dot uses the dot style (labels under the dots).
func (s *StepsW) Dot() *StepsW { s.dot = true; s.labelVert = true; return s }

// Small uses the small size.
func (s *StepsW) Small() *StepsW { s.size, s.sizeSet = Small, true; return s }

// Size sets the size (Small or Middle).
func (s *StepsW) Size(sz ControlSize) *StepsW { s.size, s.sizeSet = sz, true; return s }

// LabelVertical puts titles under the icons.
func (s *StepsW) LabelVertical() *StepsW { s.labelVert = true; return s }

// Changed reports a click on another step.
func (s *StepsW) Changed() bool { return s.changed }

func (s *StepsW) statusOf(i int) string {
	it := s.items[i]
	if it.Status != "" {
		return it.Status
	}
	switch {
	case i < s.current:
		return s.finish
	case i == s.current:
		if s.status != "" {
			return s.status
		}
		return "process"
	}
	return "wait"
}

type stepColors struct {
	iconBg, iconFg, iconBorder theme.Color
	title, desc, tail          theme.Color
}

func (s *StepsW) colors(st string, hover bool) stepColors {
	tk := s.tk
	var k stepColors
	switch st {
	case "process":
		k = stepColors{tk.ProcessIconBg, tk.ProcessIconColor, tk.ProcessIconBorderColor, tk.ProcessTitleColor, tk.ProcessDescriptionColor, tk.TailColor}
	case "finish":
		k = stepColors{tk.FinishIconBg, tk.FinishIconColor, tk.FinishIconBorderColor, tk.FinishTitleColor, tk.FinishDescriptionColor, tk.FinishTailColor}
	case "success":
		k = stepColors{tk.SuccessIconBg, tk.SuccessIconColor, tk.SuccessIconBorderColor, tk.SuccessTitleColor, tk.SuccessDescriptionColor, tk.SuccessTailColor}
	case "error":
		k = stepColors{tk.ErrorIconBg, tk.ErrorIconColor, tk.ErrorIconBorderColor, tk.ErrorTitleColor, tk.ErrorDescriptionColor, tk.TailColor}
	default:
		k = stepColors{tk.WaitIconBg, tk.WaitIconColor, tk.WaitIconBorderColor, tk.WaitTitleColor, tk.WaitDescriptionColor, tk.TailColor}
	}
	if !hover || st == "process" {
		return k
	}
	// Which parts of a clickable step follow the pointer differs by design
	// system: Element highlights waiting steps only; shadcn the title (and
	// a waiting step's icon); Ant the title and description (and a waiting
	// step's icon).
	switch s.th.Flavor {
	case theme.Element:
		if st == "wait" {
			k.title, k.desc, k.iconFg, k.iconBorder = tk.HoverColor, tk.HoverColor, tk.HoverColor, tk.HoverIconBorderColor
		}
	case theme.StyleShadcn:
		k.title = tk.HoverColor
		if st == "wait" {
			k.iconFg, k.iconBorder = tk.HoverColor, tk.HoverIconBorderColor
		}
	default:
		k.title, k.desc = tk.HoverColor, tk.HoverColor
		if st == "wait" {
			k.iconFg, k.iconBorder = tk.HoverColor, tk.HoverIconBorderColor
		}
	}
	return k
}

// dotColor is the color of a dot or custom icon in Ant / shadcn steps.
func (s *StepsW) dotColor(st string) theme.Color {
	switch st {
	case "process", "finish":
		return s.tk.DotActiveColor
	case "error":
		return s.tk.DotErrorColor
	}
	return s.tk.DotWaitColor
}

// Show builds the steps.
func (s *StepsW) Show() *StepsW {
	c := s.c
	if !s.sizeSet {
		if sz, ok := c.ComponentSize(); ok && sz == Small {
			s.size = Small
		}
	}
	if s.cur != nil {
		s.current = *s.cur
	}
	s.th = c.ThemeFor(theme.CompSteps)
	s.tk = &s.th.Components.Steps
	dir := layout.Row
	if s.vertical {
		dir = layout.Column
	}
	b := c.box("steps", dir, nil)
	b.n.setRole(a11y.RoleList).setName(c.Locale().Steps)
	if s.vertical {
		b.n.setState(a11y.Vertical)
	} else {
		b.n.setState(a11y.Horizontal)
	}
	b.n.ln.Align = layout.AlignStart
	if s.vertical {
		b.n.ln.Align = layout.AlignStretch
	}
	if s.nav && !s.vertical {
		b.n.ln.Align = layout.AlignStretch
		if navEl(c) {
			// Element simple: a light rounded strip, 13px / 8% padding.
			b.n.ln.Padding = geom.VH(13, 0)
			if r, ok := c.rects[b.n.id]; ok {
				b.n.ln.Padding.Left, b.n.ln.Padding.Right = r.W*0.08, r.W*0.08
			}
			b.Bg(s.tk.NavBg).Radius(s.tk.NavRadius)
		}
	}
	b.Body(func() {
		for i := range s.items {
			if s.nav && !s.vertical {
				s.navItem(i)
				continue
			}
			s.item(i)
		}
	})
	return s
}

// stepSem marks step node n (shared by both step styles).
func (s *StepsW) stepSem(n *node, i int, it StepItem) {
	c := s.c
	n.setRole(a11y.RoleListItem).stateIf(i == s.current, a11y.Current)
	if s.statusOf(i) == "error" {
		n.setState(a11y.Invalid)
	}
	if s.cur != nil {
		n.setRole(a11y.RoleButton)
		if it.Disabled {
			n.disabled = true
		}
	}
	if c.a11yOn {
		pos, size := i+1, len(s.items)
		pct := s.percent
		cur := i == s.current
		n.fill(semFunc(func(sn *a11y.Node) {
			sn.PosInSet, sn.SetSize = pos, size
			if it.SubTitle != "" && it.Description != "" {
				sn.Description = it.SubTitle + " " + it.Description
			} else {
				sn.Description = it.SubTitle + it.Description
			}
			if cur && pct >= 0 {
				sn.Value = strconv.FormatFloat(pct, 'f', -1, 64) + "%"
			}
		}))
	}
}

// drawPercent paints the progress ring of the current step around ir.
func (s *StepsW) drawPercent(l *paint.List, ir geom.Rect) {
	tk := s.tk
	cx, cy := ir.X+ir.W/2, ir.Y+ir.H/2
	rad := ir.W/2 + 4
	l.StrokePath(paint.NewPath().Circle(cx, cy, rad), paint.StrokeStyle{Width: 2}, tk.ProgressTrailColor)
	if s.percent > 0 {
		l.StrokePath(paint.NewPath().Arc(cx, cy, rad, -math.Pi/2, 2*math.Pi*s.percent/100), paint.StrokeStyle{Width: 2, Cap: paint.CapRound}, tk.ProgressColor)
	}
}

// navItem builds a step of the navigation type: icon and title centered
// in an equal share of the row, a chevron toward the next step and the
// current step underlined (Ant); Element's simple style puts the chevron
// in the gap and has no underline.
func (s *StepsW) navItem(i int) {
	c, th, tk := s.c, s.th, s.tk
	el := navEl(c)
	it := s.items[i]
	last := i == len(s.items)-1
	n := c.add("step", it.Title)
	s.stepSem(n, i, it)
	small := s.size == Small
	tst := c.style(tk.NavTitleFontSize, text.Regular)
	is, nfs := tk.NavIconSize, tk.NavIconFontSize
	if small {
		tst = c.style(tk.NavTitleFontSizeSM, text.Regular)
		is, nfs = tk.NavIconSizeSM, tk.NavIconFontSizeSM
	}
	sst := c.style(th.FontSize, text.Regular)
	dst := c.style(tk.DescriptionFontSize, text.Regular)
	clickable := s.cur != nil && !it.Disabled
	var ia Interaction
	if clickable {
		ia = c.interact(n, true, input.CursorPointer)
		if ia.Clicked && *s.cur != i {
			*s.cur = i
			s.changed = true
			c.invalid = true
		}
	}
	contentW := func() float32 {
		w := text.Width(it.Title, tst)
		if it.SubTitle != "" {
			w += 8 + text.Width(it.SubTitle, sst)
		}
		if it.Description != "" {
			w = max(w, text.Width(it.Description, dst))
		}
		return is + 8 + w
	}
	padV := float32(12)
	if el {
		padV = 0
	}
	n.ln.Measure = func(_, _ float32) geom.Size {
		h := max(is, tst.LineHeight)
		if it.Description != "" {
			h = max(is, tst.LineHeight+dst.LineHeight)
		}
		return geom.Size{W: contentW() + 32, H: h + 2*padV}
	}
	n.ln.Grow, n.ln.Shrink, n.ln.MinW = 1, 1, 0
	if el && last {
		n.ln.Grow = 0
	}
	cur := i == s.current
	n.paint = func(l *paint.List, r geom.Rect) {
		st := s.statusOf(i)
		hov := clickable && ia.Hovered && !cur
		k := s.colors(st, hov)
		cw := contentW()
		x := r.X + (r.W-cw)/2
		if el {
			x = r.X
		}
		x = max(r.X, x)
		top := r.Y + padV
		h := r.H - 2*padV
		ir := geom.Rect{X: x, Y: top + (min(h, max(is, tst.LineHeight))-is)/2, W: is, H: is}
		if it.Description != "" {
			ir.Y = top
		}
		// Positions are worked out left to right; px / pr mirror them in
		// RTL (icon, then title towards the end, chevron at the end).
		px, pr := stepsMirror(c, r)
		irL := ir
		ir = pr(ir)
		// Icon.
		switch {
		case it.Icon != nil:
			c.paintIcon(l, it.Icon, ir.Expand(-2), k.iconFg, 0)
		case el:
			l.Stroke(ir, geom.Rad(is/2), 1, k.iconBorder)
			switch st {
			case "finish", "success":
				icon.Draw(l, icon.Check, ir.Expand(-3), k.iconFg, 0)
			case "error":
				icon.Draw(l, icon.Close, ir.Expand(-3), k.iconFg, 0)
			default:
				lay := navLabel(c, strconv.Itoa(i+1), nfs, text.Regular)
				l.Glyphs(geom.Pt(ir.X+(ir.W-lay.Width)/2, ir.Y+(ir.H-lay.Height)/2), lay.Glyphs, k.iconFg)
			}
		default:
			l.Fill(ir, geom.Rad(is/2), k.iconBg)
			switch st {
			case "finish", "success":
				icon.Draw(l, icon.Check, ir.Expand(-is*0.27), k.iconFg, 0)
			case "error":
				icon.Draw(l, icon.Close, ir.Expand(-is*0.27), k.iconFg, 0)
			default:
				lay := navLabel(c, strconv.Itoa(i+1), nfs, text.Regular)
				l.Glyphs(geom.Pt(ir.X+(ir.W-lay.Width)/2, ir.Y+(ir.H-lay.Height)/2), lay.Glyphs, k.iconFg)
			}
		}
		if cur && s.percent >= 0 && !el {
			s.drawPercent(l, ir)
		}
		// Title, sub title, description.
		tx := irL.Right() + 8
		ty := top + (min(h, max(is, tst.LineHeight))-tst.LineHeight)/2
		if it.Description != "" {
			ty = top
		}
		tl := text.Shape(it.Title, tst, c.textOpts(text.Options{NoWrap: true, Ellipsis: true, MaxWidth: max(0, r.Right()-tx-16)}))
		l.Glyphs(geom.Pt(px(tx, tl.Width), ty), tl.Glyphs, k.title)
		if it.SubTitle != "" {
			sub := text.Shape(it.SubTitle, sst, c.textOpts(text.Options{NoWrap: true}))
			l.Glyphs(geom.Pt(px(tx+tl.Width+8, sub.Width), ty+(tst.LineHeight-sub.Height)/2), sub.Glyphs, tk.SubTitleColor)
		}
		if it.Description != "" {
			dl := text.Shape(it.Description, dst, c.textOpts(text.Options{NoWrap: true, Ellipsis: true, MaxWidth: max(0, r.Right()-tx-16)}))
			l.Glyphs(geom.Pt(px(tx, dl.Width), ty+tst.LineHeight), dl.Glyphs, k.desc)
		}
		// Chevron toward the next step (mirrored in RTL).
		if !last {
			col := tk.NavArrowColor
			ax := r.Right() - 6
			if el {
				ax = r.Right() - 16
			}
			cy := r.Y + r.H/2
			p := paint.NewPath().MoveTo(px(ax-5, 0), cy-9).LineTo(px(ax+1, 0), cy).LineTo(px(ax-5, 0), cy+9)
			l.StrokePath(p, paint.StrokeStyle{Width: 1}, col)
		}
		// The current step is underlined (Ant).
		if !el {
			a := c.Anim(n.id, "ink", b2f(cur), th.DurationMid)
			if a > 0 {
				w := r.W * a
				l.Fill(geom.Rect{X: r.X + (r.W-w)/2, Y: r.Bottom() - 2, W: w, H: 2}, geom.Radii{}, tk.NavInkBarColor)
			}
		}
		if clickable && ia.FocusVisible {
			l.Stroke(r.Expand(-1), geom.Rad(th.RadiusSM), 2, th.FocusRing)
		}
	}
}

// stepsMirror returns helpers that map an x (or rectangle) worked out left
// to right inside r to its physical position: unchanged in LTR, reflected
// about r's center in RTL. Text keeps its shape; only its origin moves.
func stepsMirror(c *Context, r geom.Rect) (px func(x, w float32) float32, pr func(geom.Rect) geom.Rect) {
	px = func(x, _ float32) float32 { return x }
	if c.IsRTL() {
		px = func(x, w float32) float32 { return 2*r.X + r.W - x - w }
	}
	pr = func(q geom.Rect) geom.Rect { q.X = px(q.X, q.W); return q }
	return
}

func (s *StepsW) iconSize() float32 {
	if s.size == Small {
		return s.tk.IconSizeSM
	}
	return s.tk.IconSize
}

func (s *StepsW) item(i int) {
	c, th, tk := s.c, s.th, s.tk
	el := navEl(c)
	it := s.items[i]
	last := i == len(s.items)-1
	n := c.add("step", it.Title)
	small := s.size == Small
	titleSize, nfs := tk.TitleFontSize, tk.IconFontSize
	if small {
		titleSize, nfs = tk.TitleFontSizeSM, tk.IconFontSizeSM
	}
	titleW := text.Regular
	tst := c.style(titleSize, titleW)
	dst := c.style(tk.DescriptionFontSize, text.Regular)
	sst := c.style(th.FontSize, text.Regular)
	is := s.iconSize()
	if s.dot {
		is = tk.DotSize
	}
	vertLabels := (s.labelVert || el) && !s.vertical
	clickable := s.cur != nil && !it.Disabled
	var ia Interaction
	// Steps are list items; clickable ones are buttons (numbered within
	// the list by semantics) and the current step is marked Current.
	s.stepSem(n, i, it)
	if clickable {
		ia = c.interact(n, true, input.CursorPointer)
		if ia.Clicked && *s.cur != i {
			*s.cur = i
			s.changed = true
			c.invalid = true
		}
	}
	descMaxW := tk.DescriptionMaxWidth
	if s.vertical {
		n.ln.Measure = func(maxW, _ float32) geom.Size {
			h := max(is, tst.LineHeight)
			if it.Description != "" {
				lay := text.Shape(it.Description, dst, text.Options{MaxWidth: max(40, maxW-is-16)})
				h = tst.LineHeight + lay.Height
				if el {
					h += 4
				}
			}
			if !last {
				h += 24
				if s.dot {
					h += 0
				}
			}
			if s.dot && !last {
				h = max(h, 22+24)
			}
			return geom.Size{W: maxW, H: max(h, is)}
		}
	} else {
		n.ln.Measure = func(_, _ float32) geom.Size {
			tw := text.Width(it.Title, tst)
			if it.SubTitle != "" {
				tw += 8 + text.Width(it.SubTitle, sst)
			}
			dw := float32(0)
			if it.Description != "" {
				dw = min(descMaxW, text.Width(it.Description, dst))
			}
			if vertLabels {
				w := max(tw, dw, is)
				h := is + 8 + tst.LineHeight
				if el {
					h = is + 14 + tst.LineHeight
				}
				if it.Description != "" {
					h += dst.LineHeight * 2
				}
				if !el {
					w = max(w, 140)
				}
				return geom.Size{W: w, H: h}
			}
			w := is + 8 + max(tw, dw)
			h := max(is, tst.LineHeight)
			if it.Description != "" {
				lay := text.Shape(it.Description, dst, text.Options{MaxWidth: descMaxW})
				h = max(is, tst.LineHeight) + lay.Height
			}
			return geom.Size{W: w + 16, H: h}
		}
		if !last || (vertLabels && !el) {
			n.ln.Grow = 1
			n.ln.Shrink = 1
			n.ln.MinW = 0
			if !vertLabels {
				n.ln.Margin.Right = 16
			}
		}
	}
	id := n.id
	n.paint = func(l *paint.List, r geom.Rect) {
		st := s.statusOf(i)
		hov := clickable && ia.Hovered && i != s.current
		k := s.colors(st, hov)
		_ = c.Anim(id, "hover", b2f(hov), th.DurationMid)
		var ir geom.Rect
		switch {
		case s.vertical:
			ir = geom.Rect{X: r.X, Y: r.Y, W: is, H: is}
			if s.dot {
				ir = geom.Rect{X: r.X + 4, Y: r.Y + 7, W: 8, H: 8}
			}
		case vertLabels:
			cx := r.X + r.W/2
			if el {
				cx = r.X + is/2
			}
			ir = geom.Rect{X: cx - is/2, Y: r.Y, W: is, H: is}
			if s.dot {
				ir.Y = r.Y + 2
			}
		default:
			ir = geom.Rect{X: r.X, Y: r.Y, W: is, H: is}
		}
		// Positions are worked out left to right; px / pr mirror them in
		// RTL (the layout mirrors the order of the steps).
		px, pr := stepsMirror(c, r)
		// Tail line.
		if !last {
			lineW := tk.TailWidth
			if s.dot && !s.vertical {
				lineW = tk.DotTailWidth
			}
			switch {
			case s.vertical:
				x := ir.X + ir.W/2 - lineW/2
				y0 := ir.Bottom() + 4
				if el {
					y0 = ir.Bottom()
				}
				y1 := r.Bottom() - 4
				if el {
					y1 = r.Bottom()
				}
				l.Fill(pr(geom.Rect{X: x, Y: y0, W: lineW, H: y1 - y0}), geom.Rad(lineW/2), k.tail)
			case vertLabels:
				y := ir.Y + ir.H/2 - lineW/2
				x0 := ir.Right() + 8
				x1 := r.Right() + (r.W)/2 - 8
				if el {
					x0, x1 = ir.Right(), r.Right()
				}
				if s.dot {
					x0, x1 = ir.Right()+6, r.Right()+r.W/2-6
				}
				nx := x1
				if el {
					nx = r.Right()
				}
				l.Fill(pr(geom.Rect{X: x0, Y: y, W: max(0, nx-x0), H: lineW}), geom.Rad(lineW/2), k.tail)
			}
		}
		// Icon.
		irL := ir
		ir = pr(ir)
		switch {
		case s.dot:
			d := tk.DotSize
			if st == "process" {
				d = tk.DotCurrentSize
			}
			cx, cy := ir.X+ir.W/2, ir.Y+ir.H/2
			col := k.iconFg
			if !el {
				col = s.dotColor(st)
			}
			l.Path(paint.NewPath().Circle(cx, cy, d/2), col)
		case it.Icon != nil:
			col := k.iconFg
			if !el {
				col = s.dotColor(st)
			}
			c.paintIcon(l, it.Icon, ir.Expand(-2), col, 0)
		case el:
			l.Fill(ir, geom.Rad(is/2), k.iconBg)
			l.Stroke(ir, geom.Rad(is/2), 2, k.iconBorder)
			switch st {
			case "finish", "success":
				icon.Draw(l, icon.Check, ir.Expand(-6), k.iconFg, 0)
			case "error":
				icon.Draw(l, icon.Close, ir.Expand(-6), k.iconFg, 0)
			default:
				lay := navLabel(c, strconv.Itoa(i+1), nfs, text.Semibold)
				l.Glyphs(geom.Pt(ir.X+(ir.W-lay.Width)/2, ir.Y+(ir.H-lay.Height)/2), lay.Glyphs, k.iconFg)
			}
		default:
			if i == s.current && s.percent >= 0 && !s.dot {
				s.drawPercent(l, ir)
			}
			l.Fill(ir, geom.Rad(is/2), k.iconBg)
			if k.iconBorder.A > 0 {
				l.Stroke(ir, geom.Rad(is/2), 1, k.iconBorder) // outlined / hovered steps
			}
			switch st {
			case "finish", "success":
				icon.Draw(l, icon.Check, ir.Expand(-is*0.27), k.iconFg, 0)
			case "error":
				icon.Draw(l, icon.Close, ir.Expand(-is*0.27), k.iconFg, 0)
			default:
				lay := navLabel(c, strconv.Itoa(i+1), nfs, text.Regular)
				l.Glyphs(geom.Pt(ir.X+(ir.W-lay.Width)/2, ir.Y+(ir.H-lay.Height)/2), lay.Glyphs, k.iconFg)
			}
		}
		// Texts.
		ir = irL
		tw := text.Shape(it.Title, func() text.Style {
			x := tst
			if el && st == "process" {
				x.Weight = text.Bold
			}
			return x
		}(), c.textOpts(text.Options{NoWrap: true}))
		var tx, ty float32
		switch {
		case s.vertical:
			tx = ir.Right() + 16
			if el {
				tx = ir.Right() + 10
			}
			if s.dot {
				tx = r.X + 28
			}
			ty = r.Y + (is-tst.LineHeight)/2
			if s.dot {
				ty = r.Y
			}
			if el {
				ty = r.Y + (is-tst.LineHeight)/2
			}
		case vertLabels:
			tx = ir.X + ir.W/2 - tw.Width/2
			if el {
				tx = r.X
			}
			ty = ir.Bottom() + 8
			if el {
				ty = ir.Bottom() + 6
			}
		default:
			tx = ir.Right() + 8
			ty = r.Y + (is-tst.LineHeight)/2
		}
		l.Glyphs(geom.Pt(px(tx, tw.Width), ty), tw.Glyphs, k.title)
		endX := tx + tw.Width
		if it.SubTitle != "" {
			sub := text.Shape(it.SubTitle, sst, c.textOpts(text.Options{NoWrap: true}))
			l.Glyphs(geom.Pt(px(endX+8, sub.Width), ty+(tst.LineHeight-sub.Height)/2), sub.Glyphs, tk.SubTitleColor)
			endX += 8 + sub.Width
		}
		if it.Description != "" {
			maxW := descMaxW
			if s.vertical {
				maxW = r.Right() - tx
			}
			if vertLabels && !el {
				maxW = r.W
			}
			lay := text.Shape(it.Description, dst, c.textOpts(text.Options{MaxWidth: maxW}))
			dx := tx
			if vertLabels && !el {
				dx = ir.X + ir.W/2 - lay.Width/2
			}
			dy := ty + tst.LineHeight
			if el && s.vertical {
				dy += 2
			}
			l.Glyphs(geom.Pt(px(dx, lay.Width), dy), lay.Glyphs, k.desc)
		}
		// Ant horizontal: tail after the title (into the gap towards the
		// next step).
		if !last && !s.vertical && !vertLabels {
			lx := endX + 16
			l.Fill(pr(geom.Rect{X: lx, Y: ty + tst.LineHeight/2, W: max(0, r.Right()+16-lx), H: 1}), geom.Radii{}, k.tail)
		}
		if clickable && ia.FocusVisible {
			l.Stroke(r.Expand(2), geom.Rad(th.RadiusSM), 2, th.FocusRing)
		}
	}
}
