package layout

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/internal/flexbox/flexstyle"
)

// Gap sets the main-axis gap between children (and the wrap line gap).
func Gap(v float32) Option { return Option{func(s *style) { s.Gap, s.LineGap = v, v }} }

// GapXY sets main-axis gap and wrapped-line gap separately.
func GapXY(main, lines float32) Option {
	return Option{func(s *style) { s.Gap, s.LineGap = main, lines }}
}

// Pad sets equal padding.
func Pad(v float32) Option { return Option{func(s *style) { s.Padding = geom.All(v) }} }

// PadVH sets CSS two-value padding.
func PadVH(v, h float32) Option { return Option{func(s *style) { s.Padding = geom.VH(v, h) }} }

// Padding sets per-edge padding.
func Padding(in geom.Insets) Option { return Option{func(s *style) { s.Padding = in }} }

// Margin sets per-edge margin.
func Margin(in geom.Insets) Option { return Option{func(s *style) { s.Margin = in }} }

// Grow sets the flex grow factor (basis defaults to 0).
func Grow(g float32) Option { return Option{func(s *style) { s.Grow = g }} }

// Shrink sets the flex shrink factor.
func Shrink(v float32) Option { return Option{func(s *style) { s.Shrink = v }} }

// Basis sets an explicit flex basis.
func Basis(v float32) Option { return Option{func(s *style) { s.Basis, s.HasBasis = v, true }} }

// W fixes the width.
func W(v float32) Option { return Option{func(s *style) { s.Width = v }} }

// H fixes the height.
func H(v float32) Option { return Option{func(s *style) { s.Height = v }} }

// Size fixes both dimensions.
func Size(w, h float32) Option { return Option{func(s *style) { s.Width, s.Height = w, h }} }

// MinW / MaxW / MinH / MaxH clamp the size.
func MinW(v float32) Option { return Option{func(s *style) { s.MinW = v }} }
func MaxW(v float32) Option { return Option{func(s *style) { s.MaxW = v }} }
func MinH(v float32) Option { return Option{func(s *style) { s.MinH = v }} }
func MaxH(v float32) Option { return Option{func(s *style) { s.MaxH = v }} }

// Align sets cross-axis alignment of children.
func AlignItems(a Align) Option { return Option{func(s *style) { s.Align = a }} }

// Self sets this node's own cross-axis alignment.
func Self(a Align) Option { return Option{func(s *style) { s.AlignSelf = a }} }

// JustifyContent sets main-axis distribution.
func JustifyContent(j Justify) Option { return Option{func(s *style) { s.Justify = j }} }

// Center centers children on both axes.
func Center() Option {
	return Option{func(s *style) { s.Align, s.Justify = AlignCenter, JustifyCenter }}
}

// Middle centers children on the cross axis (vertical centering in rows).
func Middle() Option { return Option{func(s *style) { s.Align = AlignCenter }} }

// Between spreads children with space-between.
func Between() Option { return Option{func(s *style) { s.Justify = JustifySpaceBetween }} }

// End packs children at the end of the main axis.
func End() Option { return Option{func(s *style) { s.Justify = JustifyEnd }} }

// WrapItems enables wrapping.
func WrapItems() Option { return Option{func(s *style) { s.Wrap = true }} }

// Fill makes the node grow on the main axis and stretch on the cross axis.
func Fill() Option { return Option{func(s *style) { s.Grow, s.AlignSelf = 1, AlignStretch }} }

// Abs positions the node absolutely; pass Unset for unused edges.
func Abs(left, top, right, bottom float32) Option {
	return Option{func(s *style) {
		s.Absolute, s.Pos = true, flexstyle.Pos{Left: left, Top: top, Right: right, Bottom: bottom}
	}}
}

// Dir sets the container direction.
func Dir(d Direction) Option { return Option{func(s *style) { s.Dir = d }} }

// Physical keeps the node at its physical (LTR) position inside an RTL
// parent: its x is not mirrored. Use it for things pinned to a physical
// edge regardless of reading direction. It does not affect the node's own
// children; those follow the node's RTL flag.
func Physical() Option { return Option{func(s *style) { s.NoMirror = true }} }

// PadSE sets vertical padding v and inline start/end padding. Left/Right of
// geom.Insets are the start/end sides (they swap physically in RTL boxes).
func PadSE(v, start, end float32) Option {
	return Option{func(s *style) { s.Padding = geom.Insets{Left: start, Top: v, Right: end, Bottom: v} }}
}

// ScrollX lets the content exceed the box horizontally; the container
// scrolls or clips it.
func ScrollX() Option { return Option{func(s *style) { s.ScrollX = true }} }

// ScrollY lets the content exceed the box vertically.
func ScrollY() Option { return Option{func(s *style) { s.ScrollY = true }} }

// NoStretch keeps the node's own cross size where the alignment would
// stretch it; it is then placed at the start of the cross axis.
func NoStretch() Option { return Option{func(s *style) { s.NoStretch = true }} }

// Stretch makes the node fill the cross axis, undoing NoStretch.
func Stretch() Option {
	return Option{func(s *style) { s.NoStretch, s.AlignSelf = false, AlignStretch }}
}
