package mobile

import (
	"math"
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

type paintColor = theme.Color

// scrollY lets content overflow a box vertically (we move it ourselves).
var scrollY = layout.ScrollY()

// scrollX lets content overflow a box horizontally.
var scrollX = layout.ScrollX()

// or returns s, or def when s is empty.
func or(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func b2f(b bool) float32 {
	if b {
		return 1
	}
	return 0
}

// anchor creates an invisible, out-of-flow node whose ID keys the
// retained state of widgets that render only in overlays.
func anchor(c *ui.Context) *ui.Box {
	return ui.Div(c, layout.Abs(0, 0, layout.Unset, layout.Unset), layout.Size(0, 0))
}

// shape lays out a single line.
func shape(s string, st text.Style) *text.Layout {
	return text.Shape(s, st, text.Options{NoWrap: true})
}

// drawText draws s with its top-left at (x, y) and returns its width.
func drawText(l *paint.List, s string, st text.Style, x, y float32, col theme.Color) float32 {
	lay := shape(s, st)
	l.Glyphs(geom.Pt(x, y), lay.Glyphs, col)
	return lay.Width
}

// drawTextMid draws s vertically centred in r starting at x.
func drawTextMid(l *paint.List, s string, st text.Style, x float32, r geom.Rect, col theme.Color) float32 {
	lay := shape(s, st)
	l.Glyphs(geom.Pt(x, r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
	return lay.Width
}

// drawTextCenter draws s centred in r.
func drawTextCenter(l *paint.List, s string, st text.Style, r geom.Rect, col theme.Color) {
	lay := shape(s, st)
	l.Glyphs(geom.Pt(r.X+(r.W-lay.Width)/2, r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
}

// drawTextRight draws s right-aligned at right, vertically centred in r.
func drawTextRight(l *paint.List, s string, st text.Style, right float32, r geom.Rect, col theme.Color) float32 {
	lay := shape(s, st)
	l.Glyphs(geom.Pt(right-lay.Width, r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
	return lay.Width
}

// drawEllipsis draws s cut to maxW with an ellipsis.
func drawEllipsis(l *paint.List, s string, st text.Style, x, y, maxW float32, col theme.Color) {
	lay := text.Shape(s, st, text.Options{MaxWidth: maxW, MaxLines: 1, Ellipsis: true})
	l.Glyphs(geom.Pt(x, y), lay.Glyphs, col)
}

func textW(s string, st text.Style) float32 { return text.Width(s, st) }

// hairline draws a 0.5px separator (one device pixel at 2×) along the
// bottom of r from x0.
func hairline(l *paint.List, r geom.Rect, x0 float32, col theme.Color) {
	l.Fill(geom.Rect{X: x0, Y: r.Bottom() - 0.5, W: r.Right() - x0, H: 0.5}, geom.Radii{}, col)
}

func hairlineTop(l *paint.List, r geom.Rect, x0 float32, col theme.Color) {
	l.Fill(geom.Rect{X: x0, Y: r.Y, W: r.Right() - x0, H: 0.5}, geom.Radii{}, col)
}

func drawIcon(l *paint.List, ic *icon.Icon, x, y, s float32, col theme.Color) {
	icon.Draw(l, ic, geom.Rect{X: x, Y: y, W: s, H: s}, col, 0)
}

// ---------------------------------------------------------------------------
// Right-to-left painting
//
// The ui package mirrors the layout of RTL scopes, but custom painters
// place their parts themselves. They compute positions left to right as
// before and pass them through a span, which maps them to physical
// positions: the identity in LTR (output unchanged, bit for bit) and a
// reflection around the box's vertical centre line in RTL.

// span maps logical (left-to-right) x positions inside a box to physical
// ones for the box's direction.
type span struct {
	rtl  bool
	a    float32 // r.X + r.Right(): an element at x of width w lands at a-x-w
	data bool    // text is user content: keep the DirAuto paragraph direction
}

// spanOf returns the span of r; rtl is the direction of the widget's scope
// (Context.IsRTL when it was built).
func spanOf(r geom.Rect, rtl bool) span { return span{rtl: rtl, a: 2*r.X + r.W} }

// X is the physical left of an element of width w laid out at x.
func (s span) X(x, w float32) float32 {
	if s.rtl {
		return s.a - x - w
	}
	return x
}

// P is the physical position of the point x.
func (s span) P(x float32) float32 {
	if s.rtl {
		return s.a - x
	}
	return x
}

// R is the physical rectangle of the logical rectangle r.
func (s span) R(r geom.Rect) geom.Rect {
	r.X = s.X(r.X, r.W)
	return r
}

// Pt maps a physical pointer position to the logical one (the reflection is
// its own inverse).
func (s span) Pt(p geom.Point) geom.Point { return geom.Pt(s.P(p.X), p.Y) }

// Radii swaps the left and right corners in RTL.
func (s span) Radii(r geom.Radii) geom.Radii {
	if s.rtl {
		r.TL, r.TR, r.BL, r.BR = r.TR, r.TL, r.BR, r.BL
	}
	return r
}

// content returns the span for user content (values, data): its text
// keeps the DirAuto paragraph direction like a ui.Input value, so "3 条"
// is not reordered in RTL; positions are still mirrored.
func (s span) content() span { s.data = true; return s }

// opts sets the RTL paragraph direction of interface text in RTL; in LTR
// (and for content) o is returned unchanged.
func (s span) opts(o text.Options) text.Options {
	if s.rtl && !s.data && o.Direction == text.DirAuto {
		o.Direction = text.DirRTL
	}
	return o
}

// shape is shape for the span's direction.
func (s span) shape(str string, st text.Style) *text.Layout {
	if !s.rtl || s.data {
		return shape(str, st)
	}
	return text.Shape(str, st, s.opts(text.Options{NoWrap: true}))
}

// text draws str with its logical top-left at (x, y) and returns its width.
func (s span) text(l *paint.List, str string, st text.Style, x, y float32, col theme.Color) float32 {
	lay := s.shape(str, st)
	l.Glyphs(geom.Pt(s.X(x, lay.Width), y), lay.Glyphs, col)
	return lay.Width
}

// textMid draws str vertically centred in r from logical x.
func (s span) textMid(l *paint.List, str string, st text.Style, x float32, r geom.Rect, col theme.Color) float32 {
	lay := s.shape(str, st)
	l.Glyphs(geom.Pt(s.X(x, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
	return lay.Width
}

// textRight draws str ending at logical right, vertically centred in r.
func (s span) textRight(l *paint.List, str string, st text.Style, right float32, r geom.Rect, col theme.Color) float32 {
	lay := s.shape(str, st)
	l.Glyphs(geom.Pt(s.X(right-lay.Width, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
	return lay.Width
}

// textCenter draws str centred in the logical rectangle r.
func (s span) textCenter(l *paint.List, str string, st text.Style, r geom.Rect, col theme.Color) {
	lay := s.shape(str, st)
	l.Glyphs(geom.Pt(s.X(r.X+(r.W-lay.Width)/2, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
}

// ellipsis draws str cut to maxW with an ellipsis from logical x.
func (s span) ellipsis(l *paint.List, str string, st text.Style, x, y, maxW float32, col theme.Color) {
	lay := text.Shape(str, st, s.opts(text.Options{MaxWidth: maxW, MaxLines: 1, Ellipsis: true}))
	l.Glyphs(geom.Pt(s.X(x, lay.Width), y), lay.Glyphs, col)
}

// icon draws ic in the logical square (x, y, size); directional icons
// (arrows, chevrons) mirror in RTL.
func (s span) icon(l *paint.List, ic *icon.Icon, x, y, size float32, col theme.Color) {
	if !s.rtl {
		drawIcon(l, ic, x, y, size, col)
		return
	}
	icon.DrawWith(l, ic, geom.Rect{X: s.X(x, size), Y: y, W: size, H: size}, col, icon.DrawOpts{RTL: true})
}

// fill fills the logical rectangle r.
func (s span) fill(l *paint.List, r geom.Rect, rad geom.Radii, col theme.Color) {
	l.Fill(s.R(r), s.Radii(rad), col)
}

// hairline is hairline with the separator starting at logical x0.
func (s span) hairline(l *paint.List, r geom.Rect, x0 float32, col theme.Color) {
	l.Fill(s.R(geom.Rect{X: x0, Y: r.Bottom() - 0.5, W: r.Right() - x0, H: 0.5}), geom.Radii{}, col)
}

// hairlineTop is hairlineTop with the separator starting at logical x0.
func (s span) hairlineTop(l *paint.List, r geom.Rect, x0 float32, col theme.Color) {
	l.Fill(s.R(geom.Rect{X: x0, Y: r.Y, W: r.Right() - x0, H: 0.5}), geom.Radii{}, col)
}

// inlineKey maps Left / Right to the previous / next item in the visual
// direction: swapped in RTL, where the next item is on the left.
func inlineKey(rtl bool, k input.Key) (prev, next bool) {
	switch k {
	case input.KeyLeft:
		return !rtl, rtl
	case input.KeyRight:
		return rtl, !rtl
	}
	return false, false
}

// spinAngle returns the rotation of a loading spinner keyed by id and keeps
// frames coming while it is shown (static when animations are off).
func spinAngle(c *ui.Context, id ui.ID) float64 {
	if !pump(c, id, 0) {
		return 0
	}
	ms := c.Now().UnixMilli() % 800
	return float64(ms) / 800 * 2 * math.Pi
}

// drawSpinner draws a circular loading arc of radius r centred at (cx, cy).
func drawSpinner(l *paint.List, cx, cy, r float32, angle float64, col theme.Color) {
	p := paint.NewPath().Arc(cx, cy, r, angle-math.Pi/2, 1.5*math.Pi)
	l.StrokePath(p, paint.StrokeStyle{Width: max(1.5, r/5), Cap: paint.CapRound}, col)
}

// drawBadge draws a badge (count text or a dot when s is "") whose
// top-right corner sits at (right, top) shifted half outwards.
func drawBadge(l *paint.List, th *Theme, s string, cx, cy float32) {
	white := theme.Hex("#ffffff")
	if s == "" {
		r := float32(4.5)
		l.Fill(geom.Rect{X: cx - r - 1, Y: cy - r - 1, W: 2*r + 2, H: 2*r + 2}, geom.Rad(r+1), th.Box)
		l.Fill(geom.Rect{X: cx - r, Y: cy - r, W: 2 * r, H: 2 * r}, geom.Rad(r), th.BadgeColor)
		return
	}
	st := th.Font(th.FontTiny+1, text.Medium)
	st.LineHeight = 16
	w := max(16, textW(s, st)+9)
	r := geom.Rect{X: cx - w/2, Y: cy - 8, W: w, H: 16}
	l.Fill(r.Expand(1), geom.Rad(9), th.Box)
	l.Fill(r, geom.Rad(8), th.BadgeColor)
	drawTextCenter(l, s, st, r, white)
}

// drawSwitch draws a mobile switch (51×31 Ant Mobile, 52×30 Vant, 48×28
// shadcn) in r.
func drawSwitch(l *paint.List, th *Theme, r geom.Rect, on bool, k float32) {
	drawSwitchDir(l, th, r, on, k, false)
}

// drawSwitchDir is drawSwitch for a direction: the knob of an on switch
// sits at the end (the left in RTL).
func drawSwitchDir(l *paint.List, th *Theme, r geom.Rect, on bool, k float32, rtl bool) {
	col := theme.Lerp(th.SwitchOff, th.Primary, k)
	l.Fill(r, geom.Rad(r.H/2), col)
	d := r.H - 4
	x := r.X + 2 + (r.W-4-d)*k
	if rtl {
		x = r.X + 2 + (r.W-4-d)*(1-k)
	}
	knob := geom.Rect{X: x, Y: r.Y + 2, W: d, H: d}
	sh := []paint.Shadow{{Y: 2, Blur: 4, Color: theme.RGBA(0, 35, 11, .2)}}
	if th.shad() {
		sh = []paint.Shadow{{Y: 1, Blur: 3, Color: theme.RGBA(0, 0, 0, .1)}, {Y: 1, Blur: 2, Spread: -1, Color: theme.RGBA(0, 0, 0, .1)}}
	}
	l.Shadows(knob, geom.Rad(d/2), sh)
	l.Fill(knob, geom.Rad(d/2), theme.Lerp(th.SwitchThumb, th.SwitchThumbOn, k))
}

// switchSize returns the switch track size for the style.
func switchSize(th *Theme) geom.Size { return geom.Size{W: th.SwitchW, H: th.SwitchH} }

// screenLayer opens a full-window layer (masks, popups) detached from
// any gesture area of the page.
func screenLayer(c *ui.Context, fn func(), opts ...layout.Option) *ui.Box {
	return ui.Overlay(c, ui.OverlayOptions{Place: ui.ScreenFill}, func() { detached(c, fn) }, opts...)
}

// motion returns the open progress (0 closed … 1 open) of an overlay keyed
// by id, animating both ways.
func motion(c *ui.Context, id ui.ID, open bool, dur time.Duration) float32 {
	return c.AnimFrom(id, "mobile.open", 0, b2f(open), dur)
}

// nop is an empty painter for leaves that add their painter with Paint.
func nop(*paint.List, geom.Rect) {}

// DrawBadge paints a badge centred at (cx, cy): s is the count text, "" or
// "dot" for a dot. Use it in custom painters (avatars, icons).
func DrawBadge(l *paint.List, th *Theme, s string, cx, cy float32) {
	if s == "dot" {
		s = ""
	}
	drawBadge(l, th, s, cx, cy)
}

// filled paints bg under p. ui.Custom leaves draw only their painter, so
// Box.Bg has no effect on them.
func filled(bg theme.Color, p ui.PaintFunc) ui.PaintFunc {
	return func(l *paint.List, r geom.Rect) {
		l.Fill(r, geom.Radii{}, bg)
		p(l, r)
	}
}

// ---------------------------------------------------------------------------
// Accessibility helpers

// stateIf returns s when on, else 0.
func stateIf(on bool, s a11y.State) a11y.State {
	if on {
		return s
	}
	return 0
}

// pressSem offers the Press action on a node without a ui hit (touchables
// inside gesture areas, mirror parts): the widget reads it back with
// Context.A11yAction.
func pressSem(s *a11y.Node) {
	if s.States.Has(a11y.Disabled) {
		return
	}
	switch s.Role {
	case a11y.RoleStatus, a11y.RoleAlert, a11y.RoleText, a11y.RoleParagraph, a11y.RoleHeading, a11y.RoleList,
		a11y.RoleProgressBar, a11y.RoleDialog, a11y.RoleAlertDialog, a11y.RoleForm, a11y.RoleTabPanel:
		return // not activatable
	}
	s.Actions |= a11y.Press
}

// stepSem offers Increment / Decrement on a node that may not be focusable
// (the default actions need focus); the widget reads them back with
// Context.A11yAction.
func stepSem(s *a11y.Node) {
	if !s.States.Has(a11y.Disabled) {
		s.Actions |= a11y.Increment | a11y.Decrement
	}
}

// focusRing paints the keyboard focus outline just inside r (mobile
// widgets are often edge to edge, an outside ring would be clipped).
func focusRing(th *Theme) ui.PaintFunc {
	return func(l *paint.List, r geom.Rect) { drawFocusRing(l, th, r) }
}

func drawFocusRing(l *paint.List, th *Theme, r geom.Rect) {
	w := max(2, th.Base.FocusRingWidth)
	l.Stroke(r.Inset(geom.All(w/2)), geom.Rad(4), w, th.Ring)
}

// keyPresses returns the keys pressed (Down events) for focused widget id
// and consumes its key events.
func keyPresses(c *ui.Context, id ui.ID) []input.Key {
	var out []input.Key
	for _, k := range c.KeysFor(id) {
		if k.Down {
			out = append(out, k.Key)
		}
	}
	return out
}

// mirror exposes the sub-controls of a custom-painted widget (the back
// arrow of a NavBar, the items of a TabBar or Grid) to screen readers and
// the keyboard. A Custom box cannot have children without losing its
// measure function, so the mirror is an invisible tree of boxes laid over
// the widget's rectangle of the last frame:
//
//	origin   zero-size, absolutely positioned at the parent's origin
//	└ frame  over the widget (its role: tablist, navigation, status …)
//	  └ part one per sub-control (button, tab …)
//
// The mirror is built just before the widget, so the widget, painted
// later, keeps receiving the pointer while the parts are keyboard focus
// stops (outside gesture areas; inside one the parts only offer the
// Press action). The widget itself is hidden from the semantics tree.
type mirror struct {
	c      *ui.Context
	st     *mirrorState
	n      int // rectangles used this frame
	origin *ui.Box
	frame  *ui.Box
	owner  *ui.Box
	focus  bool // parts may be focus stops
	off    geom.Point
	rtl    bool      // the mirror lays out right to left (see part)
	ownerW float32   // width of the owner, last frame
	ring   geom.Rect // focused part relative to the owner
	ringOn bool
}

// newMirror starts a mirror; call it right before creating the widget's
// box and attach the box with over. kbd makes parts focus stops when the
// widget is not inside a gesture area. It returns nil (all methods are
// no-ops) when neither the keyboard nor accessibility need it.
func newMirror(c *ui.Context, kbd bool) *mirror {
	m := &mirror{c: c, focus: kbd && arenaOf(c).top() == 0, rtl: c.IsRTL()}
	if !m.focus && !c.A11yEnabled() {
		return nil
	}
	// A separate identity scope keeps the IDs of the widget and its
	// siblings independent of whether the mirror is built.
	ui.Key(c, "mobile.a11y", func() {
		m.origin = ui.Div(c, layout.Abs(0, 0, layout.Unset, layout.Unset), layout.Size(0, 0))
	})
	m.st = ui.State[mirrorState](c, m.origin.ID(), nil)
	return m
}

// mirrorState remembers the geometry the mirror was built with, so a
// change (the widget's rectangles come from the previous frame) asks for
// one more frame.
type mirrorState struct {
	geo []geom.Rect // by call order within a frame
}

// use records the i-th rectangle the mirror is laid out with this frame;
// a change from the last frame asks the window for another frame (the
// geometry comes from the previous paint).
func (m *mirror) use(r geom.Rect) {
	i := m.n
	m.n++
	switch {
	case i >= len(m.st.geo):
		m.st.geo = append(m.st.geo, r)
	case m.st.geo[i] != r:
		m.st.geo[i] = r
	default:
		return
	}
	m.c.Invalidator()()
}

// over lays the frame over owner (hidden from the semantics tree) with
// role, name and states.
func (m *mirror) over(owner *ui.Box, role a11y.Role, name string, st a11y.State) *ui.Box {
	if m == nil {
		return nil
	}
	owner.A11yHidden()
	return m.beside(owner, role, name, st)
}

// beside is over for an owner that stays exposed (its parts are extra
// controls, e.g. the step buttons of a spin button).
func (m *mirror) beside(owner *ui.Box, role a11y.Role, name string, st a11y.State) *ui.Box {
	if m == nil {
		return nil
	}
	c := m.c
	m.owner = owner
	r, _ := c.RectOf(owner.ID())
	o, ok := c.RectOf(m.origin.ID())
	if !ok {
		o = r
	}
	m.off = geom.Pt(r.X-o.X, r.Y-o.Y)
	if m.rtl {
		// The mirror boxes are laid out (and mirrored) like any other:
		// the zero-size origin sits at the parent's right edge and the
		// frame's Abs left is its offset from there to the owner's right.
		m.off.X = o.X - r.Right()
	}
	m.ownerW = r.W
	m.use(geom.Rect{X: m.off.X, Y: m.off.Y, W: r.W, H: r.H})
	m.origin.Body(func() {
		m.frame = ui.Div(c, layout.Abs(m.off.X, m.off.Y, layout.Unset, layout.Unset), layout.Size(r.W, r.H))
		m.frame.Semantics(role, name).A11yState(st)
	})
	return m.frame
}

// partRes is the state of a mirror part this frame.
type partRes struct {
	id      ui.ID
	box     *ui.Box
	tap     bool // Enter / Space, or a screen reader's Press
	focused bool
}

// part adds a sub-control at rel (the physical rectangle relative to the
// owner, from the last frame). focus makes it a keyboard focus stop when
// the mirror allows it.
func (m *mirror) part(rel geom.Rect, role a11y.Role, name string, st a11y.State, focus bool) partRes {
	var p partRes
	if m == nil || m.frame == nil {
		return p
	}
	c := m.c
	m.use(rel)
	m.frame.Body(func() {
		x := rel.X
		if m.rtl {
			x = m.ownerW - rel.X - rel.W // the RTL frame mirrors it back
		}
		b := ui.Div(c, layout.Abs(x, rel.Y, layout.Unset, layout.Unset), layout.Size(max(0, rel.W), max(0, rel.H)))
		b.Semantics(role, name).A11yState(st)
		p.id, p.box = b.ID(), b
		if m.focus && focus && !st.Has(a11y.Disabled) {
			ia := b.Focusable(input.CursorPointer)
			p.focused = ia.Focused
			p.tap = ia.Clicked
			if ia.FocusVisible {
				m.ring, m.ringOn = rel, true
			}
		} else if c.A11yEnabled() {
			b.A11y(pressSem)
		}
		if act, _, ok := c.A11yAction(p.id); ok && act == a11y.Press {
			p.tap = true
		}
	})
	return p
}

// done paints the focus ring of the focused part over the owner (call
// it after the parts; it may be called more than once).
func (m *mirror) done() {
	if m == nil || !m.ringOn || m.owner == nil {
		return
	}
	th, rel := ThemeOf(m.c), m.ring
	m.owner.PaintOver(func(l *paint.List, r geom.Rect) {
		drawFocusRing(l, th, geom.Rect{X: r.X + rel.X, Y: r.Y + rel.Y, W: rel.W, H: rel.H})
	})
}

// rovingKeys moves focus between parts with the arrow keys (prev / next
// along the axis, Home / End) and returns the index focus moved to, or -1.
func rovingKeys(c *ui.Context, ps []partRes, vertical bool) int {
	cur := -1
	for i, p := range ps {
		if p.focused {
			cur = i
		}
	}
	if cur < 0 || len(ps) == 0 {
		return -1
	}
	to := -1
	n := len(ps)
	rtl := c.IsRTL()
	prev := func(k input.Key) bool { p, _ := inlineKey(rtl, k); return p }
	next := func(k input.Key) bool { _, nx := inlineKey(rtl, k); return nx }
	for _, k := range keyPresses(c, ps[cur].id) {
		switch {
		case k == input.KeyHome:
			to = 0
		case k == input.KeyEnd:
			to = n - 1
		case vertical && k == input.KeyUp, !vertical && prev(k):
			to = (cur - 1 + n) % n
		case vertical && k == input.KeyDown, !vertical && next(k):
			to = (cur + 1) % n
		default:
			continue
		}
		cur = to
	}
	if to >= 0 && ps[to].id != 0 {
		c.SetFocus(ps[to].id)
	}
	return to
}
