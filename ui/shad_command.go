package ui

import (
	"strings"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// CommandItem is one entry of a Command palette.
type CommandItem struct {
	Value    string
	Label    string
	Group    string     // heading the item is listed under ("" = none)
	Icon     *icon.Icon // optional leading icon
	Shortcut string     // right-aligned hint such as "⌘P"
	Keywords []string   // extra search terms
	Disabled bool
}

// CommandW is a command palette under construction (shadcn/ui Command):
// a search field over a filtered, grouped list navigated with the arrow
// keys. It renders in every design system with the active theme's tokens.
type CommandW struct {
	_ [0]func()

	c       *Context
	box     *Box
	items   []CommandItem
	st      *commandState
	in      *InputW
	keys    []input.KeyEvent
	place   string
	empty   string
	maxH    float32
	picked  *CommandItem
	visible []int
	z       *lazy
	list    ID   // listbox node (0 when nothing matches)
	optIDs  []ID // option nodes, parallel to visible (ActiveDescendant)
}

type commandState struct {
	query  string
	active int
	last   string
	ptr    geom.Point
	moved  bool
}

// Command adds a command palette over items. Typing filters the list
// (label, value and keywords, case-insensitive); Up/Down move the active
// row, Enter or a click picks it (Selected).
func Command(c *Context, items []CommandItem) *CommandW {
	th := c.Theme
	loc := c.Locale()
	w := &CommandW{c: c, items: items, place: loc.Command.Placeholder, empty: loc.Command.Empty, maxH: 300}
	w.box = Div(c, layout.MinW(0))
	w.box.Bg(th.BgElevated).Radius(th.RadiusLG).Border(1, th.Split).Clip()
	w.st = State[commandState](c, hashID(w.box.n.id, "cmd"), nil)
	w.z = c.lazy(w.box.n, w.build)
	return w
}

// Placeholder replaces the search placeholder.
func (w *CommandW) Placeholder(s string) *CommandW { w.place = s; return w }

// Empty replaces the "no results" text.
func (w *CommandW) Empty(s string) *CommandW { w.empty = s; return w }

// MaxHeight limits the list height (default 300).
func (w *CommandW) MaxHeight(h float32) *CommandW { w.maxH = h; return w }

// Shadow gives the palette a floating shadow (as in a dialog).
func (w *CommandW) Shadow() *CommandW { w.box.Shadow(w.c.Theme.Shadow); return w }

// Layout applies layout options to the palette box.
func (w *CommandW) Layout(opts ...layout.Option) *CommandW { w.box.Style(opts...); return w }

// Query returns the current search text.
func (w *CommandW) Query() string { return w.st.query }

// Selected returns the item picked this frame.
func (w *CommandW) Selected() (CommandItem, bool) {
	w.z.run()
	if w.picked == nil {
		return CommandItem{}, false
	}
	return *w.picked, true
}

// ID returns the palette box ID.
func (w *CommandW) ID() ID { return w.box.n.id }

func (w *CommandW) matches(it CommandItem, q string) bool {
	if q == "" {
		return true
	}
	if strings.Contains(strings.ToLower(it.Label), q) || strings.Contains(strings.ToLower(it.Value), q) {
		return true
	}
	for _, k := range it.Keywords {
		if strings.Contains(strings.ToLower(k), q) {
			return true
		}
	}
	return false
}

func (w *CommandW) filter() {
	q := strings.ToLower(strings.TrimSpace(w.st.query))
	w.visible = w.visible[:0]
	// Keep groups together in order of first appearance.
	var groups []string
	seen := map[string]bool{}
	for _, it := range w.items {
		if !seen[it.Group] {
			seen[it.Group] = true
			groups = append(groups, it.Group)
		}
	}
	for _, g := range groups {
		for i, it := range w.items {
			if it.Group == g && w.matches(it, q) {
				w.visible = append(w.visible, i)
			}
		}
	}
}

func (w *CommandW) step(down bool) {
	n := len(w.visible)
	if n == 0 {
		return
	}
	for k := 0; k < n; k++ {
		if down {
			w.st.active = (w.st.active + 1) % n
		} else {
			w.st.active = (w.st.active - 1 + n) % n
		}
		if !w.items[w.visible[w.st.active]].Disabled {
			return
		}
	}
}

func (w *CommandW) build() {
	c, th := w.c, w.c.Theme
	// Search row: icon + borderless input, separated by a bottom border.
	row := Row(c, func() {
		Icon(c, icon.Search).Size(16).Color(theme.Fade(th.Text, .5))
		w.in = Input(c, &w.st.query).Borderless().Placeholder(w.place).Layout(layout.Grow(1))
		// The search field is a combobox driving the always-open listbox:
		// focus stays in it, the active option is its ActiveDescendant.
		w.in.ed.setRole(a11y.RoleCombobox).setState(a11y.Editable | a11y.Expandable | a11y.HasPopup).fill(w)
	}, layout.H(44), layout.PadVH(0, 12), layout.Gap(0), layout.Middle())
	row.Paint(func(l *paint.List, r geom.Rect) {
		l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, th.Split)
	})
	w.keys = msTakeKeys(c, w.in.ed.id, func(k input.KeyEvent) bool {
		return k.Key == input.KeyUp || k.Key == input.KeyDown || k.Key == input.KeyEnter
	})
	w.st.moved = c.pointer != w.st.ptr
	w.st.ptr = c.pointer
	if w.st.query != w.st.last {
		w.st.last, w.st.active = w.st.query, 0
	}
	w.filter()
	if w.st.active >= len(w.visible) {
		w.st.active = 0
	}
	if len(w.visible) > 0 && w.items[w.visible[w.st.active]].Disabled {
		w.step(true)
	}
	for _, k := range w.keys {
		if !k.Down {
			continue
		}
		switch k.Key {
		case input.KeyDown, input.KeyUp:
			w.step(k.Key == input.KeyDown)
		case input.KeyEnter:
			if w.st.active < len(w.visible) {
				if it := w.items[w.visible[w.st.active]]; !it.Disabled {
					w.picked = &it
				}
			}
		}
		c.invalid = true
	}
	w.optIDs = w.optIDs[:0]
	w.list = 0
	if len(w.visible) == 0 {
		Row(c, func() { Text(c, w.empty) }, layout.PadVH(24, 8), layout.Center()).Semantics(a11y.RoleStatus, w.empty)
		return
	}
	// groupRun builds the items visible[from:to] of one group, inside a
	// named group when the group has a heading.
	groupRun := func(from, to int) {
		g := w.items[w.visible[from]].Group
		body := func() {
			if g != "" {
				hs := c.style(th.FontSizeSM, text.Medium)
				Custom(c, func(_, _ float32) geom.Size { return geom.Size{W: text.Width(g, hs) + 16, H: 28} }, func(l *paint.List, r geom.Rect) {
					lay := text.Shape(g, hs, c.textOpts(text.Options{NoWrap: true}))
					l.Glyphs(geom.Pt(c.inline(r).X(8, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, th.TextTertiary)
				}).A11yHidden() // the group's name
			}
			for vi := from; vi < to; vi++ {
				it, vi := w.items[w.visible[vi]], vi
				Key(c, it.Value+"\x00"+it.Label, func() { w.item(it, vi) })
			}
		}
		if g == "" {
			body()
			return
		}
		Column(c, body).Semantics(a11y.RoleGroup, g)
	}
	Scroll(c, func() {
		lb := Column(c, func() {
			from := 0
			for vi := 1; vi <= len(w.visible); vi++ {
				if vi < len(w.visible) && w.items[w.visible[vi]].Group == w.items[w.visible[from]].Group {
					continue
				}
				if from > 0 {
					Custom(c, func(_, _ float32) geom.Size { return geom.Size{H: 9} }, func(l *paint.List, r geom.Rect) {
						l.Fill(geom.Rect{X: r.X - 4, Y: r.Y + 4, W: r.W + 8, H: 1}, geom.Radii{}, th.Split)
					}).A11yHidden()
				}
				groupRun(from, vi)
				from = vi
			}
		})
		lb.Semantics(a11y.RoleListbox, w.place)
		w.list = lb.n.id
	}, layout.MaxH(w.maxH), layout.Pad(4))
}

// semantics completes the search combobox: the listbox it controls and
// the active option.
func (w *CommandW) semantics(s *a11y.Node) {
	if w.list == 0 {
		return
	}
	s.States |= a11y.Expanded
	s.Controls = a11y.NodeID(w.list)
	if w.st.active < len(w.optIDs) {
		s.ActiveDescendant = a11y.NodeID(w.optIDs[w.st.active])
	}
}

func (w *CommandW) item(it CommandItem, vi int) {
	c, th := w.c, w.c.Theme
	n := c.add("commanditem", it.Label)
	st := c.style(th.FontSize, text.Regular)
	small := c.style(th.FontSizeSM, text.Regular)
	n.ln.Measure = func(maxW, _ float32) geom.Size {
		return geom.Size{W: text.Width(it.Label, st) + 16, H: 32}
	}
	ia := c.interact(n, false, input.CursorPointer)
	if it.Disabled {
		n.disabled = true
		n.cursor = input.CursorNotAllowed
	}
	// Options are reached with the arrow keys of the search field.
	n.aKbd = true
	n.setRole(a11y.RoleOption).setState(a11y.Selectable).stateIf(vi == w.st.active, a11y.Selected)
	w.optIDs = append(w.optIDs, n.id)
	if c.a11yOn {
		pos, size, sc := vi+1, len(w.visible), it.Shortcut
		n.fill(semFunc(func(s *a11y.Node) { s.PosInSet, s.SetSize, s.Shortcut = pos, size, sc }))
	}
	if ia.Hovered && !it.Disabled && w.st.moved {
		w.st.active = vi
	}
	if ia.Clicked && !it.Disabled {
		w.st.active = vi
		w.picked = &it
		c.invalid = true
	}
	active := vi == w.st.active
	n.paint = func(l *paint.List, r geom.Rect) {
		if active && !it.Disabled {
			l.Fill(r, geom.Rad(th.RadiusSM), th.ItemHoverBg)
		}
		fg := th.Text
		if active {
			fg = th.ItemSelected
		}
		if it.Disabled {
			l.Opacity(0.5)
			defer l.Pop()
		}
		// Icon and label from the start, the shortcut at the end.
		s := c.inline(r)
		x := float32(8)
		if it.Icon != nil {
			c.drawIconAt(l, it.Icon, s.X(x, 16), r.Y+(r.H-16)/2, 16, th.TextTertiary)
			x += 24
		}
		lay := text.Shape(it.Label, st, c.textOpts(text.Options{NoWrap: true}))
		l.Glyphs(geom.Pt(s.X(x, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, fg)
		if it.Shortcut != "" {
			sl := text.Shape(it.Shortcut, small, text.Options{NoWrap: true})
			l.Glyphs(geom.Pt(s.X(r.W-8-sl.Width, sl.Width), r.Y+(r.H-sl.Height)/2), sl.Glyphs, th.TextTertiary)
		}
	}
}
