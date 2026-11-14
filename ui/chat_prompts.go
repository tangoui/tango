package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// Prompts (Ant Design X Prompts)

// PromptItem is one suggestion card of Prompts.
type PromptItem struct {
	_ [0]func()

	Key         string
	Label       string
	Description string
	Icon        *icon.Icon
	Disabled    bool
}

// PromptsW is a group of clickable prompt suggestion cards (the empty
// state of a chat, follow-up questions under an answer). Not to be
// confused with Prompt, the input dialog.
type PromptsW struct {
	_ [0]func()

	c        *Context
	b        *Box
	z        *lazy
	loc      *Locale
	items    []PromptItem
	title    string
	vertical bool
	wrap     bool
	minW     float32
	cardW    float32
	clicked  string
}

// Prompts adds prompt cards for items.
//
//	if k, ok := ui.Prompts(c, prompts).Title("试试这样问").Wrap().Clicked(); ok { ask(k) }
func Prompts(c *Context, items []PromptItem) *PromptsW {
	w := &PromptsW{c: c, items: items, loc: c.Locale(), minW: 200}
	w.b = c.box("prompts", layout.Column, nil)
	w.b.n.ln.Gap = 8
	w.b.n.setRole(a11y.RoleGroup).setName(w.loc.Chat.Prompts)
	w.z = c.lazy(w.b.n, w.build)
	return w
}

// Title shows a heading above the cards (also the accessible name).
func (w *PromptsW) Title(s string) *PromptsW { w.title = s; w.b.n.setName(s); return w }

// Vertical stacks the cards in a column.
func (w *PromptsW) Vertical() *PromptsW { w.vertical = true; return w }

// Wrap lets the cards wrap onto more rows instead of shrinking.
func (w *PromptsW) Wrap() *PromptsW { w.wrap = true; return w }

// CardWidth sets the minimum card width when wrapping (default 200).
func (w *PromptsW) CardWidth(px float32) *PromptsW { w.minW = px; return w }

// Layout applies layout options to the group.
func (w *PromptsW) Layout(opts ...layout.Option) *PromptsW {
	w.b.Style(opts...)
	return w
}

// Clicked returns the key of the card activated this frame.
func (w *PromptsW) Clicked() (string, bool) { w.z.run(); return w.clicked, w.clicked != "" }

func (w *PromptsW) build() {
	c := w.c
	th := c.ThemeFor(theme.CompChat)
	tk := &th.Components.Chat
	if w.title != "" {
		Text(c, w.title).Color(th.TextSecondary).n.aHidden = true
	}
	dir := layout.Row
	if w.vertical {
		dir = layout.Column
	}
	list := c.box("prompts-list", dir, []layout.Option{layout.Gap(8)})
	cw := float32(0) // card width of a wrapped grid
	if w.wrap && !w.vertical {
		list.n.ln.Wrap = true
		cw = w.minW
		if r, ok := c.rects[list.n.id]; ok && r.W > 0 {
			// Equal columns: as many minimum-width cards as fit per row.
			const gap = 8
			cols := max(1, int((r.W+gap)/(w.minW+gap)))
			cw = (r.W - gap*float32(cols-1)) / float32(cols)
		} else {
			c.animating = true // lay out again with the measured width
		}
	}
	w.cardW = cw
	c.body(list.n, func() {
		for i := range w.items {
			it := w.items[i]
			k := it.Key
			if k == "" {
				k = it.Label
			}
			Key(c, k, func() { w.card(th, tk, it, k) })
		}
	})
}

func (w *PromptsW) card(th *theme.Theme, tk *theme.ChatToken, it PromptItem, k string) {
	c := w.c
	card := Row(c, nil, layout.Gap(10), layout.PadVH(10, 12))
	n := card.n
	switch {
	case w.vertical:
	case w.wrap:
		n.ln.Width = w.cardW - 0.01 // rounding must not push a card to the next row
	default:
		n.ln.Grow, n.ln.MinW = 1, 0
		n.ln.Basis, n.ln.HasBasis = 0, true
	}
	n.label = it.Label
	n.setRole(a11y.RoleButton).setName(it.Label)
	if it.Disabled {
		n.disabled = true
	}
	ia := c.interact(n, true, input.CursorPointer)
	h := c.Anim(n.id, "hover", b2f(ia.Hovered && !it.Disabled), th.DurationFast)
	card.Bg(theme.Lerp(tk.PromptBg, tk.PromptHoverBg, h)).Radius(tk.PromptRadius)
	if tk.PromptBorder.A > 0 {
		card.Border(1, tk.PromptBorder)
	}
	if it.Disabled {
		card.Opacity(0.5)
	}
	n.over = func(l *paint.List, r geom.Rect) {
		if ia.FocusVisible {
			drawFocusRing(l, th, r, geom.Rad(tk.PromptRadius))
		}
	}
	if ia.Clicked && !it.Disabled {
		w.clicked = k
	}
	c.body(n, func() {
		if it.Icon != nil {
			ic := c.add("prompt-icon", "")
			ic.aHidden = true
			ic.ln.Width, ic.ln.Height = 18, 18
			ic.ln.Margin.Top = (th.LineHeightFor(th.FontSize) - 18) / 2
			glyph := it.Icon
			ic.paint = func(l *paint.List, r geom.Rect) { c.paintIcon(l, glyph, r, tk.PromptIconColor, 0) }
		}
		Column(c, func() {
			Text(c, it.Label).Strong().n.aHidden = true
			if it.Description != "" {
				Text(c, it.Description).Size(th.FontSizeSM).Color(tk.PromptDescColor).n.aHidden = true
			}
		}, layout.Gap(2), layout.Grow(1), layout.Basis(0), layout.MinW(0))
	})
	if it.Description != "" {
		n.fill(semFunc(func(s *a11y.Node) { s.Description = it.Description }))
	}
}
