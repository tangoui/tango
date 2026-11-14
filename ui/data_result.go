package ui

import (
	"math"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// Result

// ResultW is a result page: a status icon or illustration, a title, a
// subtitle, actions (Extra) and optional content (Body).
type ResultW struct {
	c        *Context
	status   string // success | error | info | warning | 403 | 404 | 500
	title    string
	subTitle string
	extra    func()
	icon     func()
}

// Result starts a result page; set options, then call Show or Body.
func Result(c *Context, status, title string) *ResultW {
	return &ResultW{c: c, status: status, title: title}
}

// SubTitle sets the secondary text below the title.
func (w *ResultW) SubTitle(s string) *ResultW { w.subTitle = s; return w }

// Extra builds the action area (usually buttons).
func (w *ResultW) Extra(fn func()) *ResultW { w.extra = fn; return w }

// Icon replaces the status icon with custom content.
func (w *ResultW) Icon(fn func()) *ResultW { w.icon = fn; return w }

// Show builds the result without extra content.
func (w *ResultW) Show() *Box { return w.Body(nil) }

// Body builds the result; fn (optional) fills the grey content panel below
// the actions (Ant) or the bottom slot (Element).
func (w *ResultW) Body(fn func()) *Box {
	c := w.c
	tk := &c.ThemeFor(theme.CompResult).Components.Result
	pad := geom.VH(tk.PaddingBlock, tk.PaddingInline)
	return Column(c, func() {
		switch {
		case w.icon != nil:
			Column(c, w.icon, layout.Center(), layout.Margin(geom.Insets{Bottom: 24}))
		case w.status == "403" || w.status == "404" || w.status == "500":
			n := c.add("result-image", w.status)
			n.aHidden = true // decorative: the title says what happened
			n.ln.Width, n.ln.Height = 252, 210
			n.ln.Margin.Bottom = 24
			n.ln.AlignSelf = layout.AlignCenter
			st := w.status
			n.paint = func(l *paint.List, r geom.Rect) { drawResultIllustration(c, l, r, st) }
		default:
			size, mb := tk.IconFontSize, tk.IconMarginBottom
			n := c.add("result-icon", w.status)
			n.aHidden = true // decorative: the title says what happened
			n.ln.Width, n.ln.Height = size, size
			n.ln.Margin.Bottom = mb
			n.ln.AlignSelf = layout.AlignCenter
			status := w.status
			var col theme.Color
			switch status {
			case "success":
				col = tk.SuccessIconColor
			case "info":
				col = tk.InfoIconColor
			case "warning":
				col = tk.WarningIconColor
			case "error", "danger":
				col = tk.ErrorIconColor
			default:
				col = c.ThemeFor(theme.CompResult).ColorOf(status).Base
			}
			n.paint = func(l *paint.List, r geom.Rect) { drawStatusIcon(l, status, r, col, tk.IconGlyphColor) }
		}
		if w.title != "" {
			tt := Text(c, w.title).Align(AlignCenter).Color(tk.TitleColor)
			tt.n.setRole(a11y.RoleHeading)
			if c.a11yOn {
				tt.n.fill(semFunc(func(s *a11y.Node) { s.Level = 2 }))
			}
			tt.Size(tk.TitleFontSize).LineHeight(tk.TitleLineHeight)
		}
		if w.subTitle != "" {
			sn := dataCenterText(c, w.subTitle, tk.SubtitleFontSize, tk.SubtitleColor, layout.Margin(geom.Insets{Top: tk.SubtitleMarginTop}))
			sn.setRole(a11y.RoleParagraph)
		}
		if w.extra != nil {
			Row(c, w.extra, layout.Gap(8), layout.Middle(), layout.JustifyContent(layout.JustifyCenter), layout.Margin(geom.Insets{Top: tk.ExtraMargin}))
		}
		if fn != nil {
			b := Column(c, fn, layout.Margin(geom.Insets{Top: 24}), layout.PadVH(24, 40))
			if tk.ContentBg.A > 0 {
				b.Bg(tk.ContentBg).Radius(tk.ContentRadius)
			}
		}
	}, layout.Padding(pad))
}

// drawResultIllustration paints the 403/404/500 pictures. All drawings are
// original compositions of simple shapes tinted with theme tokens.
func drawResultIllustration(c *Context, l *paint.List, r geom.Rect, status string) {
	th := c.Theme
	s := min(r.W/252, r.H/210)
	ox, oy := r.X+(r.W-252*s)/2, r.Y+(r.H-210*s)/2
	P := func(x, y float32) (float32, float32) { return ox + x*s, oy + y*s }
	pr := th.Primary
	soft := th.Illustration.Soft
	mid := th.Illustration.Mid
	strong := pr.Base
	ground := th.FillSecondary
	line := th.TextQuaternary
	ellipse := func(cx, cy, rx, ry float32) *paint.Path {
		x, y := P(cx, cy)
		rx, ry = rx*s, ry*s
		const k = 0.5523
		return paint.NewPath().MoveTo(x+rx, y).
			CubeTo(x+rx, y+ry*k, x+rx*k, y+ry, x, y+ry).
			CubeTo(x-rx*k, y+ry, x-rx, y+ry*k, x-rx, y).
			CubeTo(x-rx, y-ry*k, x-rx*k, y-ry, x, y-ry).
			CubeTo(x+rx*k, y-ry, x+rx, y-ry*k, x+rx, y).Close()
	}
	rect := func(x, y, w, h, rad float32) (geom.Rect, geom.Radii) {
		px, py := P(x, y)
		return geom.Rect{X: px, Y: py, W: w * s, H: h * s}, geom.Rad(rad * s)
	}
	circle := func(cx, cy, rr float32) *paint.Path {
		x, y := P(cx, cy)
		return paint.NewPath().Circle(x, y, rr*s)
	}
	poly := func(pts ...float32) *paint.Path {
		p := paint.NewPath()
		for i := 0; i+1 < len(pts); i += 2 {
			x, y := P(pts[i], pts[i+1])
			if i == 0 {
				p.MoveTo(x, y)
			} else {
				p.LineTo(x, y)
			}
		}
		return p
	}
	stroke := func(p *paint.Path, w float32, col theme.Color) {
		l.StrokePath(p, paint.StrokeStyle{Width: w * s, Cap: paint.CapRound}, col)
	}
	// Backdrop: a large soft disc, a ground shadow and a few sparkles.
	l.Path(circle(126, 98, 86), soft)
	l.Path(ellipse(126, 190, 96, 9), ground)
	for _, sp := range [][3]float32{{36, 46, 3}, {214, 38, 4}, {226, 120, 2.5}, {28, 132, 2}} {
		stroke(poly(sp[0]-sp[2], sp[1], sp[0]+sp[2], sp[1]), 1.6, mid)
		stroke(poly(sp[0], sp[1]-sp[2], sp[0], sp[1]+sp[2]), 1.6, mid)
	}
	white := theme.Hex("#ffffff")
	switch status {
	case "403":
		// A shield with a keyhole lock, and a stop barrier in front.
		sh := poly(126, 30, 176, 48, 172, 112, 126, 150, 80, 112, 76, 48)
		sh.Close()
		l.Path(sh, strong)
		inner := poly(126, 44, 162, 57, 159, 106, 126, 134, 93, 106, 90, 57)
		inner.Close()
		l.Path(inner, pr.Shades[4])
		shackle := paint.NewPath()
		x0, y0 := P(112, 86)
		x1, _ := P(140, 86)
		shackle.MoveTo(x0, y0).LineTo(x0, y0-10*s)
		shackle.Arc((x0+x1)/2, y0-10*s, (x1-x0)/2, math.Pi, math.Pi)
		shackle.LineTo(x1, y0)
		lock := th.TextOn(pr.Shades[4], white) // on the inner shield
		stroke(shackle, 5, lock)
		br, brad := rect(104, 84, 44, 34, 5)
		l.Fill(br, brad, lock)
		l.Path(circle(126, 97, 4.5), strong)
		kr, krad := rect(124, 99, 4, 10, 2)
		l.Fill(kr, krad, strong)
		// Barrier.
		bar, bard := rect(52, 156, 148, 16, 3)
		l.Fill(bar, bard, th.Error.Base)
		l.Clip(bar, bard)
		for i := 0; i < 7; i++ {
			x := float32(60 + i*24)
			l.Path(poly(x, 156, x+10, 156, x, 172, x-10, 172).Close(), white)
		}
		l.Pop()
		for _, x := range []float32{68, 184} {
			pr1, pr2 := rect(x-3, 172, 6, 18, 2)
			l.Fill(pr1, pr2, line)
		}
	case "404":
		// A signpost with a question mark beside a "404" made of rounded bars.
		pole, pr2 := rect(166, 70, 7, 120, 3)
		l.Fill(pole, pr2, line)
		sign := poly(144, 60, 206, 54, 216, 72, 208, 90, 146, 96)
		sign.Close()
		l.Path(sign, strong)
		lay := text.Shape("?", text.Style{Size: 26 * s, Weight: text.Bold, LineHeight: 30 * s}, text.Options{NoWrap: true})
		sx, sy := P(178, 75)
		l.Glyphs(geom.Pt(sx-lay.Width/2, sy-lay.Height/2), lay.Glyphs, th.TextOn(strong, white))
		big := text.Style{Size: 58 * s, Weight: text.Bold, LineHeight: 72 * s}
		lay = text.Shape("404", big, text.Options{NoWrap: true})
		tx, ty := P(90, 132)
		l.Glyphs(geom.Pt(tx-lay.Width/2, ty-lay.Height/2), lay.Glyphs, mid)
		l.Glyphs(geom.Pt(tx-lay.Width/2-3*s, ty-lay.Height/2-3*s), lay.Glyphs, strong)
		// A small planet with a ring.
		l.Path(circle(52, 52, 12), pr.Shades[3])
		px, py := P(52, 52)
		ring := ellipse(52, 52, 21, 5).Rotated(-0.35, px, py)
		l.StrokePath(ring, paint.StrokeStyle{Width: 2 * s}, strong)
	case "500":
		// A server rack with one failing unit and a warning bolt.
		body, bd := rect(86, 40, 80, 140, 8)
		l.Fill(body, bd, pr.Shades[6])
		for i := 0; i < 4; i++ {
			y := float32(52 + i*31)
			u, ud := rect(96, y, 60, 22, 4)
			col := pr.Shades[4]
			if i == 2 {
				col = th.Error.Shades[4]
			}
			l.Fill(u, ud, col)
			led := th.Success.Base
			if i == 2 {
				led = th.Error.Base
			}
			l.Path(circle(106, y+11, 3), led)
			for k := 0; k < 3; k++ {
				g, gd := rect(122+float32(k)*10, y+7, 6, 8, 1.5)
				l.Fill(g, gd, theme.Fade(th.TextOn(col, white), 0.7))
			}
		}
		feet, fd := rect(92, 180, 68, 6, 3)
		l.Fill(feet, fd, line)
		// Warning bolt badge.
		l.Path(circle(178, 60, 22), th.Warning.Base)
		bolt := poly(182, 44, 170, 62, 178, 62, 174, 76, 188, 56, 180, 56)
		bolt.Close()
		l.Path(bolt, white)
		// Smoke puffs.
		for i, p := range [][3]float32{{60, 70, 8}, {50, 56, 6}, {44, 44, 4}} {
			_ = i
			l.Path(circle(p[0], p[1], p[2]), th.Fill)
		}
	}
}

// ---------------------------------------------------------------------------
// Empty

// EmptyW is an empty state: Ant's default/simple pictures or Element's
// illustration, custom image, size, description and actions.
type EmptyW struct {
	c       *Context
	b       *Box
	desc    string
	noDesc  bool
	simple  bool
	size    float32
	image   func()
	actions func()
	z       *lazy
}

// Empty adds the "no data" placeholder. Modifiers apply until the end of
// the build (or Body); the content is built then.
//
//	ui.Empty(c)
//	ui.Empty(c).Simple().Description("暂无订单")
//	ui.Empty(c).Body(func() { ui.Button(c, "立即创建").Primary() })
func Empty(c *Context) *EmptyW {
	// ConfigProvider.Defaults("Empty", ...) keys: "image" ("simple"),
	// "imageSize" (px).
	e := &EmptyW{c: c, desc: c.Locale().NoData, simple: Prop(c, "Empty", "image", "") == "simple"}
	if v, ok := p2bNum(c, "Empty", "imageSize"); ok && v > 0 {
		e.size = float32(v)
	}
	e.b = c.box("empty", layout.Column, []layout.Option{layout.AlignItems(layout.AlignCenter)})
	e.z = dataLazy(c, e.b.n, e.build)
	return e
}

// Description sets the text ("" hides it).
func (e *EmptyW) Description(s string) *EmptyW { e.desc, e.noDesc = s, s == ""; return e }

// Simple uses the compact picture and spacing (Ant PRESENTED_IMAGE_SIMPLE;
// used inside selects, tables and lists in both design systems).
func (e *EmptyW) Simple() *EmptyW { e.simple = true; return e }

// ImageSize sets the picture height (Ant default 100, simple 40, Element 160).
func (e *EmptyW) ImageSize(px float32) *EmptyW { e.size = px; return e }

// Image replaces the picture with custom content.
func (e *EmptyW) Image(fn func()) *EmptyW { e.image = fn; return e }

// Layout applies layout options to the empty state.
func (e *EmptyW) Layout(o ...layout.Option) *EmptyW { e.b.Style(o...); return e }

// Body adds actions (buttons) below the description and builds the empty
// state.
func (e *EmptyW) Body(fn func()) *Box {
	e.actions = fn
	e.z.run()
	return e.b
}

func (e *EmptyW) build() {
	c := e.c
	th := c.ThemeFor(theme.CompEmpty)
	tk := &th.Components.Empty
	el := th.Flavor == theme.Element && !e.simple
	pad := geom.VH(0, 8)
	switch {
	case e.simple:
		pad = geom.VH(8, 8)
	case el:
		pad = geom.VH(40, 0)
	}
	e.b.n.ln.Padding = pad
	size := e.size
	switch {
	case e.image != nil:
		Column(c, e.image, layout.Center())
	default:
		if size == 0 {
			size = tk.ImageHeight
			if e.simple {
				size = tk.ImageHeightSM
			}
		}
		n := c.add("empty-image", "")
		if e.noDesc {
			n.setRole(a11y.RoleImage).setName(c.Locale().NoData)
		} else {
			n.aHidden = true // decorative: the description says "no data"
		}
		w := size * 1.6
		tile := tk.MediaBg.A > 0 && !e.simple
		if el || tile {
			w = size
		}
		n.ln.Width, n.ln.Height = w, size
		n.ln.AlignSelf = layout.AlignCenter
		simple := e.simple
		n.paint = func(l *paint.List, r geom.Rect) {
			switch {
			case tile:
				// shadcn: a muted tile with a 24px inbox icon (size-6 in size-10).
				l.Fill(r, geom.Rad(tk.MediaRadius), tk.MediaBg)
				is := r.H * .6
				icon.Draw(l, icon.Inbox, geom.Rect{X: r.X + (r.W-is)/2, Y: r.Y + (r.H-is)/2, W: is, H: is}, tk.MediaColor, 0)
			case el:
				drawElementEmpty(c, l, r)
			case simple:
				drawAntEmptySimple(tk, l, r)
			default:
				drawAntEmptyDefault(c, l, r)
			}
		}
	}
	if !e.noDesc {
		t := Text(c, e.desc).Align(AlignCenter)
		switch {
		case el:
			t.Color(tk.DescriptionColor).Layout(layout.Margin(geom.Insets{Top: 20}))
		case e.simple:
			t.Color(tk.DescriptionColorSM).Layout(layout.Margin(geom.Insets{Top: 8}))
		default:
			t.Color(tk.DescriptionColor).Layout(layout.Margin(geom.Insets{Top: 8}))
		}
	}
	if e.actions != nil {
		mt := float32(16)
		if el {
			mt = 20
		}
		Row(c, e.actions, layout.Gap(8), layout.JustifyContent(layout.JustifyCenter), layout.Margin(geom.Insets{Top: mt}))
	}
}

// drawAntEmptyDefault: an open box on a soft ellipse with a little sparkle;
// original shapes in Ant's neutral palette.
func drawAntEmptyDefault(c *Context, l *paint.List, r geom.Rect) {
	th := c.Theme
	s := r.H / 100
	cx := r.X + r.W/2
	P := func(x, y float32) (float32, float32) { return cx + (x-80)*s, r.Y + y*s }
	poly := func(pts ...float32) *paint.Path {
		p := paint.NewPath()
		for i := 0; i+1 < len(pts); i += 2 {
			x, y := P(pts[i], pts[i+1])
			if i == 0 {
				p.MoveTo(x, y)
			} else {
				p.LineTo(x, y)
			}
		}
		return p.Close()
	}
	il := th.Illustration
	shadow, back, front, edge, inner := il.Ground, il.Back, il.Front, il.Edge, il.Inner
	ex, ey := P(80, 88)
	rx, ry := 62*s, 10*s
	const k = 0.5523
	l.Path(paint.NewPath().MoveTo(ex+rx, ey).
		CubeTo(ex+rx, ey+ry*k, ex+rx*k, ey+ry, ex, ey+ry).
		CubeTo(ex-rx*k, ey+ry, ex-rx, ey+ry*k, ex-rx, ey).
		CubeTo(ex-rx, ey-ry*k, ex-rx*k, ey-ry, ex, ey-ry).
		CubeTo(ex+rx*k, ey-ry, ex+rx, ey-ry*k, ex+rx, ey).Close(), shadow)
	// Back flaps and body.
	l.Path(poly(44, 52, 56, 30, 104, 30, 116, 52), back)
	l.Path(poly(56, 34, 104, 34, 110, 52, 50, 52), inner)
	l.Path(poly(44, 52, 116, 52, 116, 86, 44, 86), front)
	l.Path(poly(44, 52, 66, 52, 70, 60, 90, 60, 94, 52, 116, 52, 116, 54, 95, 54, 91, 62, 69, 62, 65, 54, 44, 54), edge)
	l.StrokePath(poly(44, 52, 116, 52, 116, 86, 44, 86), paint.StrokeStyle{Width: 1.2 * s}, edge)
	// Sparkle lines on the left.
	for _, ln := range [][4]float32{{30, 26, 38, 34}, {24, 40, 34, 42}, {40, 16, 42, 26}} {
		x0, y0 := P(ln[0], ln[1])
		x1, y1 := P(ln[2], ln[3])
		l.StrokePath(paint.NewPath().MoveTo(x0, y0).LineTo(x1, y1), paint.StrokeStyle{Width: 2 * s, Cap: paint.CapRound}, edge)
	}
}

// drawAntEmptySimple: flat tray outline.
func drawAntEmptySimple(tk *theme.EmptyToken, l *paint.List, r geom.Rect) {
	s := r.H / 40
	cx := r.X + r.W/2
	P := func(x, y float32) (float32, float32) { return cx + (x-32)*s, r.Y + y*s }
	line, fill := tk.SimpleLine, tk.SimpleFill
	ex, ey := P(32, 33)
	rx, ry := 32*s, 7*s
	const k = 0.5523
	l.Path(paint.NewPath().MoveTo(ex+rx, ey).
		CubeTo(ex+rx, ey+ry*k, ex+rx*k, ey+ry, ex, ey+ry).
		CubeTo(ex-rx*k, ey+ry, ex-rx, ey+ry*k, ex-rx, ey).
		CubeTo(ex-rx, ey-ry*k, ex-rx*k, ey-ry, ex, ey-ry).
		CubeTo(ex+rx*k, ey-ry, ex+rx, ey-ry*k, ex+rx, ey).Close(), tk.SimpleShadow)
	p := paint.NewPath()
	pts := []float32{12, 16, 20, 4, 44, 4, 52, 16, 52, 34, 12, 34}
	for i := 0; i < len(pts); i += 2 {
		x, y := P(pts[i], pts[i+1])
		if i == 0 {
			p.MoveTo(x, y)
		} else {
			p.LineTo(x, y)
		}
	}
	p.Close()
	l.Path(p, fill)
	l.StrokePath(p, paint.StrokeStyle{Width: s}, line)
	q := paint.NewPath()
	pts = []float32{12, 16, 24, 16, 26, 21, 38, 21, 40, 16, 52, 16}
	for i := 0; i < len(pts); i += 2 {
		x, y := P(pts[i], pts[i+1])
		if i == 0 {
			q.MoveTo(x, y)
		} else {
			q.LineTo(x, y)
		}
	}
	l.StrokePath(q, paint.StrokeStyle{Width: s}, line)
}

// drawElementEmpty: Element-flavoured picture — a stack of soft blue-grey
// cards with a document on top (original composition).
func drawElementEmpty(c *Context, l *paint.List, r geom.Rect) {
	th := c.Theme
	s := r.H / 160
	ox := r.X + (r.W-160*s)/2
	R := func(x, y, w, h, rad float32) (geom.Rect, geom.Radii) {
		return geom.Rect{X: ox + x*s, Y: r.Y + y*s, W: w * s, H: h * s}, geom.Rad(rad * s)
	}
	il := th.Illustration
	c1, c2, c3, ink := il.Fill1, il.Fill2, il.Fill3, il.Ink
	g, gr := R(18, 136, 124, 10, 5)
	l.Fill(g, gr, c3)
	a, ar := R(30, 56, 100, 76, 8)
	l.Fill(a, ar, c1)
	b, br := R(42, 34, 76, 92, 6)
	l.Fill(b, br, c2)
	d, dr := R(54, 18, 52, 66, 5)
	l.Fill(d, dr, th.BgContainer)
	l.Stroke(d, dr, max(1, 1.5*s), c1)
	for i := 0; i < 3; i++ {
		ln, lr := R(62, 32+float32(i)*12, 36-float32(i%2)*12, 4, 2)
		l.Fill(ln, lr, ink)
	}
	f, fr := R(30, 92, 100, 40, 8)
	l.Fill(f, fr, c1)
	h, hr := R(66, 104, 28, 6, 3)
	l.Fill(h, hr, c3)
}
