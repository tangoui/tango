package mobile

import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

// ---------------------------------------------------------------------------
// NavBar

type navState struct {
	back, right geom.Rect // tap zones from the last paint (absolute)
	// The same zones and the title relative to the bar (accessibility
	// mirror).
	backRel, rightRel, titleRel geom.Rect
}

// NavBarW is the top navigation bar: back arrow and text at the start
// (left; right in RTL, the arrow mirrored to point right), centred title,
// text or icon at the end.
type NavBarW struct {
	c         *ui.Context
	th        *Theme
	b         *ui.Box
	st        *navState
	pr        Press
	title     string
	back      bool
	backText  string
	rightText string
	rightIcon *icon.Icon
	border    bool
	rtl       bool

	m          *mirror
	backPart   partRes
	rightPart  partRes
	rightName  string
	rightBuilt bool
}

// NavBar adds a navigation bar titled title.
//
// ConfigProvider defaults ("mobile.NavBar"): "bordered" (bool).
func NavBar(c *ui.Context, title string) *NavBarW {
	th := ThemeOf(c)
	w := &NavBarW{c: c, th: th, title: title, border: th.Style == Vant || th.shad(), rtl: c.IsRTL()}
	if v, ok := cfgBool(c, "NavBar", "bordered"); ok {
		w.border = v
	}
	// Screen readers and the keyboard see a heading and buttons laid over
	// the painted bar (see mirror).
	w.m = newMirror(c, true)
	w.b = ui.Custom(c, func(maxW, _ float32) geom.Size { return geom.Size{W: maxW, H: th.NavBarHeight} }, filled(th.Box, w.paint))
	w.st = ui.State[navState](c, w.b.ID(), nil)
	w.pr = touchAs(c, w.b, touchPointer)
	w.m.over(w.b, a11y.RoleNone, "", 0)
	w.m.part(w.st.titleRel, a11y.RoleHeading, title, 0, false)
	return w
}

// Back shows the back arrow with optional text (Ant Mobile shows the
// arrow only by default; Vant styles typically pass "返回").
func (w *NavBarW) Back(text string) *NavBarW {
	if !w.back {
		w.backPart = w.m.part(w.st.backRel, a11y.RoleButton, or(text, w.c.Locale().Back), 0, true)
		w.m.done()
	}
	w.back, w.backText = true, text
	return w
}

// Right shows a text action at the end (right; left in RTL).
func (w *NavBarW) Right(text string) *NavBarW { w.rightText = text; w.rightAction(); return w }

// RightIcon shows an icon action at the end; name it with RightLabel
// unless Right gives it a text.
func (w *NavBarW) RightIcon(ic *icon.Icon) *NavBarW { w.rightIcon = ic; w.rightAction(); return w }

// RightLabel sets the accessible name of the right action (needed for an
// icon-only action).
func (w *NavBarW) RightLabel(s string) *NavBarW { w.rightName = s; w.rightAction(); return w }

// rightAction exposes the right action once and keeps its name current.
func (w *NavBarW) rightAction() {
	if !w.rightBuilt {
		w.rightBuilt = true
		w.rightPart = w.m.part(w.st.rightRel, a11y.RoleButton, "", 0, true)
		w.m.done()
	}
	if b := w.rightPart.box; b != nil {
		b.Semantics(a11y.RoleButton, or(w.rightName, w.rightText))
	}
}

// Border toggles the bottom hairline (Vant and shadcn default on, Ant
// Mobile off).
func (w *NavBarW) Border(on bool) *NavBarW { w.border = on; return w }

// BackClicked reports a tap on the back area.
func (w *NavBarW) BackClicked() bool {
	return w.back && (w.pr.Tap && w.st.back.Contains(w.pr.TapPos) || w.backPart.tap)
}

// RightClicked reports a tap on the right action.
func (w *NavBarW) RightClicked() bool {
	return (w.rightText != "" || w.rightIcon != nil) && (w.pr.Tap && w.st.right.Contains(w.pr.TapPos) || w.rightPart.tap)
}

// ID returns the bar's ID.
func (w *NavBarW) ID() ui.ID { return w.b.ID() }

func (w *NavBarW) paint(l *paint.List, r geom.Rect) {
	th := w.th
	pad := th.PadH
	// Laid out left to right; sp mirrors the parts in RTL.
	sp := spanOf(r, w.rtl)
	vant := th.Style == Vant
	actCol := th.Text
	if vant {
		actCol = th.Primary
	}
	// Left: back.
	w.st.back = geom.Rect{}
	if w.back {
		x := r.X + pad
		as := float32(24)
		if vant {
			as = 18
		} else if th.shad() {
			as = 22
		}
		pressed := w.pr.Pressed && w.st.back.Contains(w.pr.PressPos)
		col := actCol
		if pressed {
			col = fadeCol(col, 0.6)
		}
		sp.icon(l, icon.Left, x-4, r.Y+(r.H-as)/2, as, col)
		x += as - 2
		if w.backText != "" {
			fs := th.Font(th.FontBody, text.Regular)
			x += sp.textMid(l, w.backText, fs, x, r, col)
		}
		w.st.back = sp.R(geom.Rect{X: r.X, Y: r.Y, W: max(x-r.X+pad, th.TouchMin), H: r.H})
	}
	// Right.
	w.st.right = geom.Rect{}
	right := r.Right() - pad
	if w.rightText != "" || w.rightIcon != nil {
		pressed := w.pr.Pressed && w.st.right.Contains(w.pr.PressPos)
		col := actCol
		if pressed {
			col = fadeCol(col, 0.6)
		}
		x := right
		if w.rightIcon != nil {
			x -= 22
			sp.icon(l, w.rightIcon, x, r.Y+(r.H-22)/2, 22, col)
		}
		if w.rightText != "" {
			fs := th.Font(th.FontBody, text.Regular)
			if w.rightIcon != nil {
				x -= 4
			}
			x -= sp.textRight(l, w.rightText, fs, x, r, col)
		}
		w.st.right = sp.R(geom.Rect{X: min(x-pad, r.Right()-th.TouchMin), Y: r.Y, W: r.Right() - min(x-pad, r.Right()-th.TouchMin), H: r.H})
	}
	// Title, centred, cut to the space between the sides.
	fs := th.Font(th.FontTitle, th.TitleWeight)
	side := max(w.st.back.W, w.st.right.W, 60)
	maxW := r.W - 2*side
	lay := text.Shape(w.title, fs, sp.opts(text.Options{MaxWidth: maxW, MaxLines: 1, Ellipsis: true}))
	l.Glyphs(geom.Pt(r.X+(r.W-lay.Width)/2, r.Y+(r.H-lay.Height)/2), lay.Glyphs, th.Text)
	rel := func(z geom.Rect) geom.Rect { return geom.Rect{X: z.X - r.X, Y: z.Y - r.Y, W: z.W, H: z.H} }
	w.st.backRel, w.st.rightRel = rel(w.st.back), rel(w.st.right)
	w.st.titleRel = geom.Rect{X: (r.W - lay.Width) / 2, Y: 0, W: lay.Width, H: r.H}
	if w.border {
		hairline(l, r, r.X, th.Border)
	}
}

func fadeCol(c paintColor, a float64) paintColor {
	c.A = uint8(float64(c.A) * a)
	return c
}

// ---------------------------------------------------------------------------
// TabBar

// TabItem is one entry of a TabBar.
type TabItem struct {
	Key   string
	Title string
	Icon  *icon.Icon
	Badge string // "" none, "dot" for a dot, else the count text
}

// TabBarW is the bottom tab bar.
type TabBarW struct {
	c       *ui.Context
	th      *Theme
	b       *ui.Box
	items   []TabItem
	active  *string
	changed bool
	pr      Press
	safe    float32
	border  bool
	rtl     bool // items run right to left
}

// TabBar adds a bottom tab bar; *active holds the selected item's Key.
// It reserves the bottom safe-area inset below the items.
//
// ConfigProvider defaults ("mobile.TabBar"): "bordered" (bool, the top
// line; default on).
func TabBar(c *ui.Context, items []TabItem, active *string) *TabBarW {
	th := ThemeOf(c)
	w := &TabBarW{c: c, th: th, items: items, active: active, safe: SafeAreaInsets(c).Bottom, border: true, rtl: c.IsRTL()}
	if v, ok := cfgBool(c, "TabBar", "bordered"); ok {
		w.border = v
	}
	m := newMirror(c, true)
	w.b = ui.Custom(c, func(maxW, _ float32) geom.Size { return geom.Size{W: maxW, H: th.TabBarHeight + w.safe} }, filled(th.Box, w.paint))
	w.pr = touchAs(c, w.b, touchPointer)
	pick := func(i int) {
		if *active != items[i].Key {
			*active = items[i].Key
			w.changed = true
			c.Invalidate()
		}
	}
	if w.pr.Tap && len(items) > 0 {
		if r, ok := c.RectOf(w.b.ID()); ok && r.W > 0 {
			i := int((spanOf(r, w.rtl).P(w.pr.TapPos.X) - r.X) / (r.W / float32(len(items))))
			pick(max(0, min(i, len(items)-1)))
		}
	}
	// Screen readers and the keyboard: a tab list over the painted bar,
	// every item a Tab stop, arrow keys move (and select).
	if m.over(w.b, a11y.RoleTabList, "", a11y.Horizontal) != nil && len(items) > 0 {
		r, _ := c.RectOf(w.b.ID())
		iw := r.W / float32(len(items))
		ps := make([]partRes, len(items))
		sp := spanOf(geom.Rect{W: r.W}, w.rtl)
		for i, it := range items {
			ps[i] = m.part(sp.R(geom.Rect{X: float32(i) * iw, W: iw, H: th.TabBarHeight}), a11y.RoleTab, it.Title,
				a11y.Selectable|stateIf(*active == it.Key, a11y.Selected), true)
			if it.Badge != "" && it.Badge != "dot" && c.A11yEnabled() {
				badge := it.Badge
				ps[i].box.A11y(func(s *a11y.Node) { s.Description = badge })
			}
			if ps[i].tap {
				pick(i)
			}
		}
		if to := rovingKeys(c, ps, false); to >= 0 {
			pick(to)
		}
		m.done()
	}
	return w
}

// Border toggles the top line (default on).
func (w *TabBarW) Border(on bool) *TabBarW { w.border = on; return w }

// Changed reports a tab switch this frame.
func (w *TabBarW) Changed() bool { return w.changed }

// ID returns the bar's ID.
func (w *TabBarW) ID() ui.ID { return w.b.ID() }

func (w *TabBarW) paint(l *paint.List, r geom.Rect) {
	th := w.th
	if w.border {
		hairlineTop(l, r, r.X, th.Border)
	}
	n := len(w.items)
	if n == 0 {
		return
	}
	iw := r.W / float32(n)
	is := float32(24)
	if th.Style == Vant || th.shad() {
		is = 22
	}
	fs := th.Font(12, text.Regular)
	fs.LineHeight = 15
	if th.shad() {
		fs = th.Font(th.FontTiny, text.Medium)
		fs.LineHeight = 14
	}
	sp := spanOf(r, w.rtl)
	for i, it := range w.items {
		cell := geom.Rect{X: r.X + float32(i)*iw, Y: r.Y, W: iw, H: th.TabBarHeight}
		col := th.TextSecondary
		if th.shad() {
			col = th.TextWeak // muted-foreground
		}
		if *w.active == it.Key {
			col = th.Primary
		}
		if it.Icon != nil {
			top := cell.Y + (cell.H-is-fs.LineHeight-2)/2
			ix := cell.X + (cell.W-is)/2
			sp.icon(l, it.Icon, ix, top, is, col)
			lay := sp.shape(it.Title, fs)
			l.Glyphs(geom.Pt(sp.X(cell.X+(cell.W-lay.Width)/2, lay.Width), top+is+2), lay.Glyphs, col)
			// The badge sits on the icon's top-end corner.
			switch it.Badge {
			case "":
			case "dot":
				drawBadge(l, th, "", sp.P(ix+is-1), top+2)
			default:
				lay := shape(it.Badge, th.Font(th.FontTiny+1, text.Medium))
				drawBadge(l, th, it.Badge, sp.P(ix+is-2+max(16, lay.Width+9)/2-6), top+3)
			}
		} else {
			sp.textCenter(l, it.Title, th.Font(th.FontMain, text.Regular), cell, col)
		}
	}
}
