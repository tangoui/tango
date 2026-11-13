package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

// ---------------------------------------------------------------------------
// ProCard (Ant Design Pro ProCard)
//
//	ui.ProCard(c, "Title").SubTitle("sub").Tooltip("tip").Extra(func() {...}).Body(func() {...})
//	ui.ProCard(c, "").Split(ui.SplitVertical).Bordered().Body(func() {
//		ui.ProCard(c, "Left").ColSpan(8).Body(left)
//		ui.ProCard(c, "Right").Body(right)
//	})
//
// A ProCard whose body directly holds other ProCards lays them out in a
// row (Direction("column") stacks them) with Gutter spacing; ColSpan (of
// 24) or ColWidth size each child. With Split the children share the
// parent's surface and are separated by divider lines.

// Split directions of ProCard.Split.
const (
	SplitVertical   = "vertical"   // children side by side, vertical dividers
	SplitHorizontal = "horizontal" // children stacked, horizontal dividers
)

// ProCardTab is one tab of a tabbed ProCard.
type ProCardTab struct {
	Key, Label string
	Icon       *icon.Icon
	Disabled   bool
	Body       func()
}

// ProCardW builds a ProCard.
type ProCardW struct {
	c              *Context
	title, sub     string
	tip            string
	extra          func()
	split          string
	dir            string
	gutter         float32
	wrap           bool
	colSpan        int
	colWidth       float32
	collapsible    bool
	collapsed      *bool
	defCollapsed   bool
	loading        bool
	bordered       bool
	borderSet      bool
	headerBordered bool
	ghost          bool
	hoverable      bool
	small          bool
	center         bool
	bodyPad        *geom.Insets
	opts           []layout.Option
	tabs           []ProCardTab
	tabActive      *string
	tabsCard       bool
	tabExtra       func()

	box     *Box
	toggled bool
	tabHit  bool
	ia      Interaction
	inSplit string // split direction of the parent card, "" when not split
}

type pro2CardState struct{ collapsed bool }

// pro2CardFrame records the ProCards built directly in a card body.
type pro2CardFrame struct {
	body  *node
	split string
	kids  []*ProCardW
}

type pro2CardStack struct{ frames []*pro2CardFrame }

var pro2CardStackID = hashID("pro2-card-stack")

func pro2Stack(c *Context) *pro2CardStack {
	return State[pro2CardStack](c, pro2CardStackID, nil)
}

// ProCard starts a ProCard ("" title = no header unless Extra, Tooltip or
// Collapsible need one); chain options, then call Body.
//
// ConfigProvider.Defaults("ProCard", ...) keys: "size" ("small" |
// "default" or a ControlSize), "bordered", "headerBordered", "ghost",
// "hoverable", "collapsible" (bool), "gutter" (number).
func ProCard(c *Context, title string) *ProCardW {
	w := &ProCardW{c: c, title: title, small: c.defaultSize() == Small}
	if s, ok := proPropSize(c, "ProCard"); ok {
		w.small = s == Small
	}
	if v, ok := p2bBool(c, "ProCard", "bordered"); ok {
		w.bordered, w.borderSet = v, true
	}
	w.headerBordered = Prop(c, "ProCard", "headerBordered", false)
	w.ghost = Prop(c, "ProCard", "ghost", false)
	w.hoverable = Prop(c, "ProCard", "hoverable", false)
	w.collapsible = Prop(c, "ProCard", "collapsible", false)
	if v, ok := p2bNum(c, "ProCard", "gutter"); ok {
		w.gutter = float32(v)
	}
	return w
}

// SubTitle sets the secondary title shown after the title.
func (w *ProCardW) SubTitle(s string) *ProCardW { w.sub = s; return w }

// Tooltip adds a question icon after the title showing tip on hover.
func (w *ProCardW) Tooltip(tip string) *ProCardW { w.tip = tip; return w }

// Extra builds the actions at the right of the header.
func (w *ProCardW) Extra(fn func()) *ProCardW { w.extra = fn; return w }

// Split lays the child ProCards out on one surface separated by dividers
// (SplitVertical or SplitHorizontal).
func (w *ProCardW) Split(dir string) *ProCardW { w.split = dir; return w }

// Direction sets how child ProCards are arranged: "row" (default) or
// "column".
func (w *ProCardW) Direction(d string) *ProCardW { w.dir = d; return w }

// Gutter sets the space between child ProCards.
func (w *ProCardW) Gutter(px float32) *ProCardW { w.gutter = px; return w }

// Wrap lets child ProCards wrap onto new lines.
func (w *ProCardW) Wrap() *ProCardW { w.wrap = true; return w }

// ColSpan sizes the card inside a parent ProCard row: span of 24 columns.
func (w *ProCardW) ColSpan(span int) *ProCardW { w.colSpan = span; return w }

// ColWidth gives the card a fixed width inside a parent ProCard row.
func (w *ProCardW) ColWidth(px float32) *ProCardW { w.colWidth = px; return w }

// Collapsible lets a click on the header fold the body.
func (w *ProCardW) Collapsible() *ProCardW { w.collapsible = true; return w }

// Collapsed binds the folded state (implies Collapsible).
func (w *ProCardW) Collapsed(v *bool) *ProCardW { w.collapsible, w.collapsed = true, v; return w }

// DefaultCollapsed starts an uncontrolled collapsible card folded.
func (w *ProCardW) DefaultCollapsed() *ProCardW {
	w.collapsible, w.defCollapsed = true, true
	return w
}

// Loading shows the animated loading placeholder instead of the body.
func (w *ProCardW) Loading(on bool) *ProCardW { w.loading = on; return w }

// Bordered draws a border around the card.
func (w *ProCardW) Bordered() *ProCardW { w.bordered, w.borderSet = true, true; return w }

// Borderless removes the border (Element cards are bordered by default).
func (w *ProCardW) Borderless() *ProCardW { w.bordered, w.borderSet = false, true; return w }

// HeaderBordered draws a divider under the header.
func (w *ProCardW) HeaderBordered() *ProCardW { w.headerBordered = true; return w }

// Ghost removes the background and the body padding.
func (w *ProCardW) Ghost() *ProCardW { w.ghost = true; return w }

// Hoverable lifts the card with a shadow on hover.
func (w *ProCardW) Hoverable() *ProCardW { w.hoverable = true; return w }

// Small uses the compact paddings.
func (w *ProCardW) Small() *ProCardW { w.small = true; return w }

// Centered centres the body content on both axes.
func (w *ProCardW) Centered() *ProCardW { w.center = true; return w }

// BodyPadding overrides the body padding.
func (w *ProCardW) BodyPadding(in geom.Insets) *ProCardW { w.bodyPad = &in; return w }

// Layout applies layout options to the card.
func (w *ProCardW) Layout(o ...layout.Option) *ProCardW { w.opts = append(w.opts, o...); return w }

// Tabs turns the card into a tab card bound to active; the active tab's
// Body is shown as the card body.
func (w *ProCardW) Tabs(active *string, tabs []ProCardTab) *ProCardW {
	w.tabActive, w.tabs = active, tabs
	return w
}

// TabsCard uses card-style tabs.
func (w *ProCardW) TabsCard() *ProCardW { w.tabsCard = true; return w }

// TabExtra builds content at the right of the tab bar.
func (w *ProCardW) TabExtra(fn func()) *ProCardW { w.tabExtra = fn; return w }

// Toggled reports a collapse / expand by the user this frame.
func (w *ProCardW) Toggled() bool { return w.toggled }

// TabChanged reports a tab switch this frame.
func (w *ProCardW) TabChanged() bool { return w.tabHit }

// Clicked reports a click on a hoverable card.
func (w *ProCardW) Clicked() bool { return w.ia.Clicked }

// IsCollapsed reports whether the body is folded.
func (w *ProCardW) IsCollapsed() bool {
	if w.collapsed != nil {
		return *w.collapsed
	}
	if w.box != nil && w.collapsible {
		return State[pro2CardState](w.c, w.box.n.id, nil).collapsed
	}
	return false
}

// ID returns the card's widget ID (valid after Body).
func (w *ProCardW) ID() ID {
	if w.box == nil {
		return 0
	}
	return w.box.n.id
}

// Box returns the card container (valid after Body).
func (w *ProCardW) Box() *Box { return w.box }

// pads returns the header and body paddings.
func (w *ProCardW) pads() (head, body geom.Insets) {
	el, shad := pro2El(w.c), pro2Shad(w.c)
	v, h := float32(16), float32(24)
	switch {
	case w.small:
		v, h = 8, 12
	case el:
		v, h = 18, 20
	case shad:
		v, h = 24, 24
	}
	head = geom.Insets{Top: v, Left: h, Right: h}
	if w.headerBordered {
		head.Bottom = v
	}
	body = geom.Insets{Top: v, Bottom: v, Left: h, Right: h}
	if el && !w.small {
		body = geom.All(20)
	}
	if w.ghost {
		head = geom.Insets{Bottom: v}
		body = geom.Insets{}
	}
	return
}

// Body builds the card with fn as its content.
func (w *ProCardW) Body(fn func()) *ProCardW {
	c, th := w.c, w.c.Theme
	el := pro2El(c)
	stack := pro2Stack(c)
	var parent *pro2CardFrame
	if k := len(stack.frames); k > 0 && stack.frames[k-1].body == c.cur {
		parent = stack.frames[k-1]
	}
	if parent != nil {
		parent.kids = append(parent.kids, w)
	}
	if parent != nil {
		w.inSplit = parent.split
	}

	b := pro2Box(c, "pro2-card", w.title, layout.Column, w.opts)
	w.box = b
	b.n.ln.MinW = 0
	b.Semantics(a11y.RoleGroup, w.title)
	if w.loading {
		b.A11yState(a11y.Busy)
	}

	shad := pro2Shad(c)
	rad := th.RadiusLG
	switch {
	case el:
		rad = th.Radius
	case shad:
		rad = shad2dCardRadius(th) // Card: rounded-xl
	}
	bordered := w.bordered
	if !w.borderSet && (el || shad) {
		bordered = true // el-card and shadcn Card are outlined
	}
	if w.inSplit != "" || w.ghost {
		bordered = false
	}
	border := th.BorderSecondary
	if th.CardBorder.A > 0 {
		border = th.CardBorder
	}
	if w.inSplit == "" && !w.ghost {
		b.Bg(th.BgContainer).Radius(rad)
		if bordered {
			b.PaintOver(func(l *paint.List, r geom.Rect) { l.Stroke(r, geom.Rad(rad), 1, border) })
		}
		switch {
		case w.hoverable:
			w.ia = b.Interact(input.CursorPointer)
			a := c.Anim(b.n.id, "hover", b2f(w.ia.Hovered), th.DurationMid)
			if a > 0 {
				b.Shadow(pro2FadeShadows(th.Shadow, a))
			}
		case el && bordered:
			b.Shadow(th.ShadowTertiary)
		case len(th.CardShadow) > 0:
			b.Shadow(th.CardShadow)
		}
	}

	// Collapse state.
	folded := false
	if w.collapsible {
		if w.collapsed != nil {
			folded = *w.collapsed
		} else {
			folded = State[pro2CardState](c, b.n.id, func() pro2CardState {
				return pro2CardState{collapsed: w.defCollapsed}
			}).collapsed
		}
	}

	if el && !w.ghost && w.inSplit == "" {
		w.headerBordered = true // el-card header divider
	}
	headPad, bodyPad := w.pads()
	if w.bodyPad != nil {
		bodyPad = *w.bodyPad
	}
	hasHead := w.title != "" || w.extra != nil || w.tip != "" || w.collapsible
	if folded && !w.headerBordered && !w.ghost {
		headPad.Bottom = headPad.Top
	}

	b.Body(func() {
		if hasHead {
			folded = w.header(headPad, folded)
		}
		if folded {
			return
		}
		if len(w.tabs) > 0 && w.tabActive != nil {
			w.tabsBody(fn, bodyPad)
			return
		}
		if w.loading {
			Column(c, func() { pro2CardLoading(c) }, layout.Padding(bodyPad))
			return
		}
		w.content(fn, bodyPad)
	})
	return w
}

// header builds the title row; it returns the (possibly toggled) folded
// state.
func (w *ProCardW) header(pad geom.Insets, folded bool) bool {
	c, th := w.c, w.c.Theme
	el := pro2El(c)
	fs := th.FontSizeLG
	if w.small || el {
		fs = th.FontSize
		if el && !w.small {
			fs = th.FontSizeLG
		}
	}
	hd := pro2Box(c, "pro2-card-head", "", layout.Row, []layout.Option{layout.Padding(pad), layout.Middle(), layout.Gap(8)})
	if w.collapsible {
		ia := c.interact(hd.n, true, input.CursorPointer)
		name := w.title
		if name == "" {
			name = c.Locale().Expand
			if !folded {
				name = c.Locale().Collapse
			}
		}
		hd.n.setRole(a11y.RoleButton).setName(name).setState(a11y.Expandable).stateIf(!folded, a11y.Expanded)
		if ia.Clicked {
			folded = !folded
			if w.collapsed != nil {
				*w.collapsed = folded
			} else {
				State[pro2CardState](c, w.box.n.id, nil).collapsed = folded
			}
			w.toggled = true
			c.invalid = true
		}
		hd.PaintOver(func(l *paint.List, r geom.Rect) {
			if ia.FocusVisible {
				drawFocusRing(l, th, r, geom.Rad(th.RadiusSM))
			}
		})
	}
	if w.headerBordered {
		col := th.BorderSecondary
		hd.Paint(func(l *paint.List, r geom.Rect) { dataHLine(l, r.X, r.Bottom()-1, r.W, col) })
	}
	if pro2Shad(c) {
		if w.sub != "" {
			hd.n.ln.Style.Apply(layout.AlignItems(layout.AlignStart))
		}
		hd.Body(func() { w.shadHeader(hd, folded) })
		return folded
	}
	hd.Body(func() {
		Row(c, func() {
			if w.collapsible {
				rot := c.Anim(hd.n.id, "rot", map[bool]float32{true: 0, false: 90}[folded], th.DurationMid)
				Icon(c, icon.Right).Size(12).Color(th.Text).Rotate(float64(rot))
			}
			if w.title != "" {
				t := Text(c, w.title).Size(fs).NoWrap().Ellipsis()
				if !el {
					t.Weight(text.Medium)
				}
				t.Layout(layout.MinW(0))
			}
			if w.tip != "" {
				Tooltip(c, w.tip, func() {
					Icon(c, icon.Question).Size(14).Color(th.TextTertiary)
				}).Semantics(a11y.RoleImage, c.Locale().ProCard.MoreInfo)
			}
			if w.sub != "" {
				Text(c, w.sub).Color(th.TextTertiary).NoWrap().Ellipsis().Layout(layout.MinW(0))
			}
		}, layout.Gap(8), layout.Middle(), layout.Grow(1), layout.MinW(0))
		if w.extra != nil {
			Row(c, w.extra, layout.Gap(8), layout.Middle())
		}
	})
	return folded
}

// tabsBody builds the tab bar and the active tab's body.
func (w *ProCardW) tabsBody(fn func(), pad geom.Insets) {
	c := w.c
	items := make([]TabItem, len(w.tabs))
	var body func()
	for i, t := range w.tabs {
		items[i] = TabItem{Key: t.Key, Label: t.Label, Icon: t.Icon, Disabled: t.Disabled}
	}
	Column(c, func() {
		tb := Tabs(c, w.tabActive, items)
		if w.tabsCard {
			tb.Card()
		}
		if w.tabExtra != nil {
			tb.Extra(w.tabExtra)
		}
		tb.Show()
		w.tabHit = tb.Changed()
		if k := pro2LastKid(c); k != nil {
			k.ln.Margin.Bottom = 0
		}
	}, layout.Padding(geom.Insets{Top: pad.Top / 2, Left: pad.Left, Right: pad.Right}))
	for _, t := range w.tabs {
		if t.Key == *w.tabActive {
			body = t.Body
		}
	}
	if body == nil {
		body = fn
	}
	if w.loading {
		Column(c, func() { pro2CardLoading(c) }, layout.Padding(pad))
		return
	}
	w.content(body, pad)
}

// content builds the body box and arranges the child ProCards.
func (w *ProCardW) content(fn func(), pad geom.Insets) {
	c, th := w.c, w.c.Theme
	if w.split != "" {
		pad = geom.Insets{}
	}
	opts := []layout.Option{layout.Padding(pad), layout.Grow(1), layout.MinW(0)}
	if w.center {
		opts = append(opts, layout.Center())
	}
	body := pro2Box(c, "pro2-card-body", "", layout.Column, opts)
	frame := &pro2CardFrame{body: body.n, split: w.split}
	stack := pro2Stack(c)
	stack.frames = append(stack.frames, frame)
	body.Body(fn)
	stack.frames = stack.frames[:len(stack.frames)-1]
	if len(frame.kids) == 0 {
		return
	}
	row := w.dir != "column"
	if w.split == SplitHorizontal {
		row = false
	} else if w.split == SplitVertical {
		row = true
	}
	if row {
		body.n.ln.Dir = layout.Row
		body.n.ln.Align = layout.AlignStretch
	}
	if w.split == "" {
		body.n.ln.Gap, body.n.ln.LineGap = w.gutter, w.gutter
		body.n.ln.Wrap = w.wrap
	}
	// Sizes: spans are fractions of 24 sharing the line; unspanned
	// children split what is left.
	sum, free := 0, 0
	for _, k := range frame.kids {
		switch {
		case k.colWidth > 0:
		case k.colSpan > 0:
			sum += k.colSpan
		default:
			free++
		}
	}
	rest := float32(1)
	if sum > 0 && free > 0 && sum < 24 {
		rest = float32(24-sum) / float32(free)
	}
	split := th.Split
	for i, k := range frame.kids {
		ln := &k.box.n.ln
		if row {
			ln.MinW = 0
			switch {
			case k.colWidth > 0:
				ln.Width, ln.Grow = k.colWidth, 0
			case k.colSpan > 0:
				ln.Grow, ln.Basis, ln.HasBasis = float32(k.colSpan), 0, true
			default:
				ln.Grow, ln.Basis, ln.HasBasis = rest, 0, true
			}
		} else if k.colWidth > 0 {
			ln.Width = k.colWidth
			ln.NoStretch = true
		}
		if i > 0 && w.split != "" {
			vertical := row
			k.box.Paint(func(l *paint.List, r geom.Rect) {
				if vertical {
					dataVLine(l, dataStartX(c, r), r.Y, r.H, split)
				} else {
					dataHLine(l, r.X, r.Y, r.W, split)
				}
			})
		}
	}
}

// pro2CardLoading paints ProCard's loading placeholder: rows of animated
// blocks with varied widths.
func pro2CardLoading(c *Context) {
	rows := [][]int{{22}, {8, 15}, {6, 18}, {13, 9}, {4, 3, 16}}
	Column(c, func() {
		for _, spans := range rows {
			Row(c, func() {
				total := 0
				for _, s := range spans {
					b := skelBlock(c, true, 0, 14, false, false, layout.Grow(float32(s)), layout.Basis(0))
					b.n.ln.MinW = 0
					total += s
				}
				if total < 24 {
					Div(c, layout.Grow(float32(24-total)), layout.Basis(0))
				}
			}, layout.Gap(8))
		}
	}, layout.Gap(8))
}
