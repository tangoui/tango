package ui

import (
	"math"
	"strconv"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// Badge

// BadgeW is a badge: count / dot / text bubble, overflow, show-zero,
// preset or custom color, offset, small size, standalone use and the
// status form with the "processing" ripple.
type BadgeW struct {
	c        *Context
	n        *node
	count    int
	text     string
	dot      bool
	overflow int
	showZero bool
	color    string
	small    bool
	offset   geom.Point
	status   string
	label    string
	wrapped  bool
	th       *theme.Theme
	tk       *theme.BadgeToken
}

// Badge adds a badge. Body(fn) wraps content and draws the bubble at its
// top-end corner (top-right; top-left in RTL); without Body the badge stands alone (an inline bubble,
// or a status dot with a label when Status is set).
//
//	ui.Badge(c).Count(5).Body(func() { ui.Avatar(c, "U") })
//	ui.Badge(c).Status("success", "成功")
//
// ConfigProvider.Defaults("Badge", ...) keys: "size" ("small" / Small),
// "overflowCount" (int), "showZero" (bool), "color" (string), "dot" (bool).
func Badge(c *Context) *BadgeW {
	b := &BadgeW{c: c, overflow: 99, th: c.ThemeFor(theme.CompBadge)}
	b.tk = &b.th.Components.Badge
	if s, ok := dataPropSize(c, "Badge"); ok {
		b.small = s == Small
	}
	if v, ok := p2bNum(c, "Badge", "overflowCount"); ok {
		b.overflow = int(v)
	}
	b.showZero = Prop(c, "Badge", "showZero", false)
	b.color = Prop(c, "Badge", "color", "")
	b.dot = Prop(c, "Badge", "dot", false)
	b.n = c.add("badge", "")
	b.n.ln.NoStretch = true
	b.n.ln.AlignSelf = layout.AlignAuto
	dataLazy(c, b.n, b.finish)
	return b
}

// Count sets the number (<= 0 hides the bubble unless ShowZero).
func (b *BadgeW) Count(n int) *BadgeW { b.count = n; return b }

// Text shows a word instead of a number (Element value="new").
func (b *BadgeW) Text(s string) *BadgeW { b.text = s; return b }

// Dot shows a small dot.
func (b *BadgeW) Dot() *BadgeW { b.dot = true; return b }

// Max sets the overflow count (99 → "99+").
func (b *BadgeW) Max(n int) *BadgeW { b.overflow = n; return b }

// ShowZero shows a zero count.
func (b *BadgeW) ShowZero() *BadgeW { b.showZero = true; return b }

// Color: status name ("success", "warning", "processing", ...),
// Ant preset ("pink", "green", ...) or hex.
func (b *BadgeW) Color(col string) *BadgeW { b.color = col; return b }

// Small uses the 14px bubble.
func (b *BadgeW) Small() *BadgeW { b.small = true; return b }

// Offset moves the bubble by (x, y) from the top-end corner; x points
// away from the content (mirrored in RTL).
func (b *BadgeW) Offset(x, y float32) *BadgeW { b.offset = geom.Pt(x, y); return b }

// Status turns a standalone badge into a status dot with a label
// ("success", "processing" (ripples), "default", "error", "warning", or
// "custom" with Color).
func (b *BadgeW) Status(status, label string) *BadgeW {
	b.status, b.label = status, label
	b.n.label = label
	return b
}

// Layout applies layout options to the badge.
func (b *BadgeW) Layout(o ...layout.Option) *BadgeW { b.n.ln.Style.Apply(o...); return b }

// antPreset maps Ant's preset color names to base colors.
var antPreset = map[string]string{
	"blue": "#1677ff", "purple": "#722ed1", "cyan": "#13c2c2", "green": "#52c41a",
	"magenta": "#eb2f96", "pink": "#eb2f96", "red": "#f5222d", "orange": "#fa8c16",
	"yellow": "#fadb14", "volcano": "#fa541c", "geekblue": "#2f54eb", "lime": "#a0d911", "gold": "#faad14",
}

func (b *BadgeW) bubbleColor() theme.Color {
	th := b.th
	switch b.color {
	case "":
		return b.tk.CountBg
	case "success", "warning", "error", "danger", "info", "primary":
		return th.ColorOf(b.color).Base
	case "processing":
		return th.Primary.Base
	case "default":
		return th.TextQuaternary
	}
	if hx, ok := antPreset[b.color]; ok {
		return theme.Hex(hx)
	}
	if col, err := theme.ParseHex(b.color); err == nil {
		return col
	}
	return th.Error.Base
}

func (b *BadgeW) content() (string, bool) {
	if b.text != "" {
		return b.text, true
	}
	if b.count <= 0 && !b.showZero {
		return "", false
	}
	if b.count > b.overflow {
		return strconv.Itoa(b.overflow) + "+", true
	}
	return strconv.Itoa(b.count), true
}

func (b *BadgeW) bubbleH() float32 {
	_, h, _ := b.bubbleSize("0")
	return h
}

func (b *BadgeW) bubbleSize(s string) (w, h float32, st text.Style) {
	tk := b.tk
	h, fs, pad := tk.IndicatorHeight, tk.TextFontSize, tk.IndicatorPaddingInline
	if b.small {
		h, fs, pad = tk.IndicatorHeightSM, tk.TextFontSizeSM, tk.IndicatorPaddingInlineSM
	}
	st = text.Style{Size: fs, Weight: text.Regular, LineHeight: b.th.LineHeightFor(fs)}
	if tk.TextFontWeight != 0 {
		st.Weight = text.Weight(tk.TextFontWeight)
	}
	w = max(h, text.Width(s, st)+2*pad)
	if len([]rune(s)) == 1 {
		w = h
	}
	return
}

func (b *BadgeW) paintBubble(l *paint.List, cx, cy float32) {
	th, tk := b.th, b.tk
	col := b.bubbleColor()
	ring := tk.RingColor
	if b.dot {
		d := tk.DotSize
		r := geom.Rect{X: cx - d/2, Y: cy - d/2, W: d, H: d}
		if th.Flavor == theme.Element {
			l.Fill(r.Expand(1), geom.Rad(d), ring)
		}
		l.Fill(r, geom.Rad(d/2), col)
		return
	}
	s, ok := b.content()
	if !ok {
		return
	}
	w, h, st := b.bubbleSize(s)
	r := geom.Rect{X: cx - w/2, Y: cy - h/2, W: w, H: h}
	l.Fill(r.Expand(1), geom.Rad(h/2+1), ring)
	l.Fill(r, geom.Rad(h/2), col)
	lay := text.Shape(s, st, text.Options{NoWrap: true})
	l.Glyphs(geom.Pt(r.X+(w-lay.Width)/2, r.Y+(h-lay.Height)/2), lay.Glyphs, th.TextOn(col, tk.TextColor))
}

// Body wraps fn's content and draws the bubble at its top-end corner.
func (b *BadgeW) Body(fn func()) *Box {
	c := b.c
	b.wrapped = true
	b.n.ln.Dir = layout.Stack
	c.body(b.n, fn)
	b.n.over = func(l *paint.List, r geom.Rect) {
		cx, cy := r.Right()+b.offset.X, r.Y+b.offset.Y
		if b.th.Flavor == theme.Element && !b.dot {
			// Element anchors the bubble's left edge half its height
			// inside the content (translateX(50%)).
			if _, ok := b.content(); ok {
				cx = r.Right() - b.bubbleH()/2 + b.offset.X
			}
		}
		if c.IsRTL() {
			cx = 2*r.X + r.W - cx // top-left corner
		}
		b.paintBubble(l, cx, cy)
	}
	// The count is announced with the content it decorates (an avatar, a
	// bell button); without a semantic child the badge itself carries it.
	desc := func() string { s, _ := b.content(); return s }
	if !dataDescribe(c, b.n, desc) {
		b.n.setRole(a11y.RoleGroup)
		b.n.fill(semFunc(func(s *a11y.Node) { addDescription(s, desc()) }))
	}
	return &Box{c: c, n: b.n}
}

// finish turns an unwrapped badge into the standalone form.
func (b *BadgeW) finish() {
	if b.wrapped {
		return
	}
	if b.status != "" || b.label != "" {
		b.showStatus()
		return
	}
	n := b.n
	if s, ok := b.content(); ok && !b.dot {
		n.setRole(a11y.RoleText).setName(s)
	}
	n.ln.AlignSelf = layout.AlignCenter
	n.ln.Measure = func(_, _ float32) geom.Size {
		if b.dot {
			return geom.Size{W: 8, H: 8}
		}
		s, ok := b.content()
		if !ok {
			return geom.Size{}
		}
		w, h, _ := b.bubbleSize(s)
		return geom.Size{W: w + 2, H: h + 2}
	}
	n.paint = func(l *paint.List, r geom.Rect) { b.paintBubble(l, r.X+r.W/2, r.Y+r.H/2) }
}

func (b *BadgeW) showStatus() {
	c, th, tk := b.c, b.th, b.tk
	n := b.n
	st := text.Style{Size: th.FontSize, Weight: text.Regular, LineHeight: th.LineHeightFor(th.FontSize)}
	sz := tk.StatusSize
	n.ln.Measure = func(_, _ float32) geom.Size {
		w := sz
		if b.label != "" {
			w += 8 + text.Width(b.label, st)
		}
		return geom.Size{W: w, H: 22}
	}
	status := b.status
	processing := status == "processing"
	if b.label != "" {
		n.setRole(a11y.RoleText).setName(b.label)
	}
	var t float32
	if processing && !c.still() {
		t = float32(c.now.UnixMilli()%1200) / 1200
		c.animating = true
	}
	n.paint = func(l *paint.List, r geom.Rect) {
		var col theme.Color
		switch status {
		case "", "custom":
			col = b.bubbleColor()
		case "default":
			col = tk.StatusDefaultColor
		case "processing":
			col = th.Primary.Base
		default:
			col = th.ColorOf(status).Base
		}
		if status == "" && b.color == "" {
			col = th.TextQuaternary
		}
		sp := c.inline(r)
		cx, cy := sp.X(0, sz)+sz/2, r.Y+r.H/2
		l.Path(paint.NewPath().Circle(cx, cy, sz/2), col)
		if processing && !c.still() {
			// Ripple: a ring growing from the dot and fading out.
			rr := sz/2 + sz/2*float32(EaseOut(float64(t)))
			l.StrokePath(paint.NewPath().Circle(cx, cy, rr), paint.StrokeStyle{Width: 1}, theme.Fade(col, float64(1-t)))
		}
		if b.label != "" {
			lay := text.Shape(b.label, st, c.textOpts(text.Options{NoWrap: true}))
			l.Glyphs(geom.Pt(sp.X(sz+8, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, tk.StatusTextColor)
		}
	}
}

// ---------------------------------------------------------------------------
// Badge.Ribbon

// BadgeRibbon wraps fn's content (usually a card) with a corner ribbon.
// placement is "end" (default) or "start" (logical: mirrored in RTL);
// color as in Badge.Color.
func BadgeRibbon(c *Context, label, color, placement string, fn func()) *Box {
	th := c.ThemeFor(theme.CompBadge)
	tk := &th.Components.Badge
	bx := &BadgeW{c: c, color: color, th: th, tk: tk}
	box := c.box("ribbon", layout.Stack, nil)
	box.n.setRole(a11y.RoleGroup).setName(label)
	box.Body(fn)
	st := text.Style{Size: tk.RibbonFontSize, Weight: text.Regular, LineHeight: th.LineHeightFor(tk.RibbonFontSize)}
	box.n.over = func(l *paint.List, r geom.Rect) {
		col := tk.RibbonBg
		if color != "" {
			col = bx.bubbleColor()
		}
		lay := text.Shape(label, st, c.textOpts(text.Options{NoWrap: true}))
		h := tk.RibbonHeight
		w := lay.Width + 16
		y := r.Y + 8
		var x float32
		start := (placement == "start") != c.IsRTL() // physical left
		if start {
			x = r.X - 8
		} else {
			x = r.Right() - w + 8
		}
		rad := geom.Rad(tk.RibbonRadius)
		if start {
			rad.BL = 0
		} else {
			rad.BR = 0
		}
		l.Fill(geom.Rect{X: x, Y: y, W: w, H: h}, rad, col)
		// The folded corner: a small triangle in a darker shade.
		dark := theme.Lerp(col, theme.Hex("#000000"), 0.25)
		p := paint.NewPath()
		if start {
			p.MoveTo(x, y+h).LineTo(x+8, y+h).LineTo(x+8, y+h+8).Close()
		} else {
			p.MoveTo(x+w, y+h).LineTo(x+w-8, y+h).LineTo(x+w-8, y+h+8).Close()
		}
		l.Path(p, dark)
		l.Glyphs(geom.Pt(x+8, y+(h-lay.Height)/2), lay.Glyphs, th.TextOn(col, tk.RibbonColor))
	}
	return box
}

// ---------------------------------------------------------------------------
// Avatar.Group

// AvatarGroupW stacks avatars with overlap.
type AvatarGroupW struct {
	c       *Context
	max     int
	size    float32
	overlap float32
	moreBg  theme.Color
}

// AvatarGroup starts a group; call Body with the avatars.
func AvatarGroup(c *Context) *AvatarGroupW {
	return &AvatarGroupW{c: c, overlap: -c.ThemeFor(theme.CompAvatar).Components.Avatar.GroupOverlapping}
}

// Max folds avatars beyond n into a "+k" avatar.
func (g *AvatarGroupW) Max(n int) *AvatarGroupW { g.max = n; return g }

// Size sets the size of the "+k" avatar (match your avatars; default 32).
func (g *AvatarGroupW) Size(px float32) *AvatarGroupW { g.size = px; return g }

// MoreBg sets the background of the "+k" avatar.
func (g *AvatarGroupW) MoreBg(col theme.Color) *AvatarGroupW { g.moreBg = col; return g }

// Body builds the avatars.
func (g *AvatarGroupW) Body(fn func()) *Box {
	c, th := g.c, g.c.ThemeFor(theme.CompAvatar)
	tk := &th.Components.Avatar
	box := c.box("avatar-group", layout.Row, []layout.Option{layout.Middle()})
	box.n.setRole(a11y.RoleGroup)
	box.n.ln.NoStretch = true
	box.Body(func() {
		fn()
		kids := box.n.kids
		if g.max > 0 && len(kids) > g.max {
			extra := len(kids) - g.max
			box.n.kids = kids[:g.max]
			bg := g.moreBg
			if bg.A == 0 {
				bg = theme.Hex("#fde3cf")
				if th.Flavor == theme.Element {
					bg = th.Info.Shades[2]
				}
			}
			size := g.size
			if size == 0 {
				size = tk.ContainerSize
			}
			a := Avatar(c, "+"+strconv.Itoa(extra)).Size(size)
			if th.Flavor != theme.StyleShadcn || g.moreBg.A > 0 {
				a.Bg(bg) // shadcn keeps the muted fallback avatar
			}
			if th.Flavor == theme.Ant {
				// Ant's default "+N" avatar uses orange text on peach.
				prev := a.n.paint
				fg := theme.Hex("#f56a00")
				a.n.paint = func(l *paint.List, r geom.Rect) {
					l.Fill(r, geom.Rad(r.W/2), bg)
					lay := text.Shape("+"+strconv.Itoa(extra), c.style(14, text.Regular), text.Options{NoWrap: true})
					l.Glyphs(geom.Pt(r.X+(r.W-lay.Width)/2, r.Y+(r.H-lay.Height)/2), lay.Glyphs, fg)
				}
				_ = prev
			}
		}
		for i, k := range box.n.kids {
			if i > 0 {
				k.ln.Margin.Left = -g.overlap
			}
			prev := k.paint
			if prev == nil {
				continue
			}
			ring := tk.GroupBorderColor
			k.paint = func(l *paint.List, r geom.Rect) {
				l.Path(paint.NewPath().Circle(r.X+r.W/2, r.Y+r.H/2, r.W/2+1.5), ring)
				prev(l, r)
			}
		}
	})
	return box
}

// avatarAutoFont returns a font size so label fits inside an avatar of
// size px (Ant scales the text to the circle with a 4px gap).
func avatarAutoFont(label string, px float32) float32 {
	fs := float32(14)
	if px >= 40 {
		fs = 18
	}
	w := text.Width(label, text.Style{Size: fs})
	avail := px - 8
	if w > avail && w > 0 {
		fs = float32(math.Floor(float64(fs * avail / w)))
	}
	return max(fs, 8)
}

var _ = time.Second
