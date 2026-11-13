package ui

import (
	"github.com/tangoui/tango/ui/layout"
)

// navGapOf maps a ControlSize to the Space/Flex gap token.
func navGapOf(s ControlSize) float32 {
	switch s {
	case Small:
		return 8
	case Large:
		return 24
	}
	return 16
}

// ---------------------------------------------------------------------------
// Flex

// FlexW is a flex container with design-token gaps (Ant Flex).
type FlexW struct {
	c   *Context
	b   *Box
	dir layout.Direction
}

// Flex starts a horizontal flex box; chain options, then Body.
func Flex(c *Context) *FlexW {
	f := &FlexW{c: c, dir: layout.Row}
	f.b = c.box("flex", layout.Row, nil)
	return f
}

// Vertical lays children top to bottom.
func (f *FlexW) Vertical() *FlexW { f.b.n.ln.Dir = layout.Column; return f }

// Justify distributes free space on the main axis.
func (f *FlexW) Justify(j layout.Justify) *FlexW { f.b.n.ln.Justify = j; return f }

// Align positions children on the cross axis.
func (f *FlexW) Align(a layout.Align) *FlexW { f.b.n.ln.Align = a; return f }

// Gap sets the gap with a size token (Small 8, Middle 16, Large 24).
func (f *FlexW) Gap(s ControlSize) *FlexW {
	f.b.n.ln.Gap, f.b.n.ln.LineGap = navGapOf(s), navGapOf(s)
	return f
}

// GapPx sets the gap in pixels.
func (f *FlexW) GapPx(px float32) *FlexW { f.b.n.ln.Gap, f.b.n.ln.LineGap = px, px; return f }

// Wrap wraps children onto new lines.
func (f *FlexW) Wrap() *FlexW { f.b.n.ln.Wrap = true; return f }

// Style applies extra layout options.
func (f *FlexW) Style(opts ...layout.Option) *FlexW { f.b.Style(opts...); return f }

// Body adds the children.
func (f *FlexW) Body(fn func()) *Box { return f.b.Body(fn) }

// ---------------------------------------------------------------------------
// Space

// SpaceW sets the spacing between inline items (Ant Space).
type SpaceW struct {
	c       *Context
	b       *Box
	split   func()
	sizeSet bool
}

// Space starts a horizontal group with the Small (8px) gap and centered
// items; chain options, then Body. ConfigProvider's Defaults("Space",
// Props{"size": ...}) changes the default gap.
//
//	ui.Space(c).Body(func() { ui.Button(c, "A"); ui.Button(c, "B") })
//	ui.Space(c).Vertical().Size(ui.Large).Body(fn)
//	ui.Space(c).Split(func() { ui.VDivider(c) }).Body(fn)
func Space(c *Context) *SpaceW {
	s := &SpaceW{c: c}
	s.b = c.box("space", layout.Row, nil)
	s.b.n.ln.Align = layout.AlignCenter
	return s
}

// Vertical stacks the items.
func (s *SpaceW) Vertical() *SpaceW {
	s.b.n.ln.Dir = layout.Column
	s.b.n.ln.Align = layout.AlignStart
	return s
}

// Size sets the gap with a size token (Small 8, Middle 16, Large 24).
func (s *SpaceW) Size(sz ControlSize) *SpaceW {
	s.b.n.ln.Gap, s.b.n.ln.LineGap = navGapOf(sz), navGapOf(sz)
	s.sizeSet = true
	return s
}

// SizePx sets horizontal and vertical gaps in pixels.
func (s *SpaceW) SizePx(h, v float32) *SpaceW {
	s.b.n.ln.Gap, s.b.n.ln.LineGap = h, v
	s.sizeSet = true
	return s
}

// Align positions items on the cross axis (default center; start when
// vertical).
func (s *SpaceW) Align(a layout.Align) *SpaceW { s.b.n.ln.Align = a; return s }

// Wrap wraps items onto new lines (horizontal only).
func (s *SpaceW) Wrap() *SpaceW { s.b.n.ln.Wrap = true; return s }

// Split builds a separator between every two items (e.g. VDivider).
func (s *SpaceW) Split(fn func()) *SpaceW { s.split = fn; return s }

// Layout applies extra layout options (margins, width, ...).
func (s *SpaceW) Layout(o ...layout.Option) *SpaceW { s.b.Style(o...); return s }

// Body adds the items.
func (s *SpaceW) Body(fn func()) *Box {
	c := s.c
	if !s.sizeSet {
		g := float32(8)
		if v, ok := c.ConfigProp("Space", "size"); ok {
			if sz, ok := v.(ControlSize); ok {
				g = navGapOf(sz)
			}
		}
		s.b.n.ln.Gap, s.b.n.ln.LineGap = g, g
	}
	s.b.Body(fn)
	if s.split == nil || len(s.b.n.kids) < 2 {
		return s.b
	}
	items := append([]*node(nil), s.b.n.kids...)
	s.b.n.kids = s.b.n.kids[:0]
	out := make([]*node, 0, 2*len(items))
	for i, k := range items {
		if i > 0 {
			before := len(s.b.n.kids)
			Key(c, i, func() { c.body(s.b.n, s.split) })
			out = append(out, s.b.n.kids[before:]...)
		}
		out = append(out, k)
	}
	s.b.n.kids = out
	return s.b
}
