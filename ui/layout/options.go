package layout

import "github.com/tangoui/tango/ui/geom"

// Gap sets the main-axis gap between children (and the wrap line gap).
func Gap(v float32) Option { return func(s *Style) { s.Gap, s.LineGap = v, v } }

// GapXY sets main-axis gap and wrapped-line gap separately.
func GapXY(main, lines float32) Option { return func(s *Style) { s.Gap, s.LineGap = main, lines } }

// Pad sets equal padding.
func Pad(v float32) Option { return func(s *Style) { s.Padding = geom.All(v) } }

// PadVH sets CSS two-value padding.
func PadVH(v, h float32) Option { return func(s *Style) { s.Padding = geom.VH(v, h) } }

// Padding sets per-edge padding.
func Padding(in geom.Insets) Option { return func(s *Style) { s.Padding = in } }

// Margin sets per-edge margin.
func Margin(in geom.Insets) Option { return func(s *Style) { s.Margin = in } }

// Grow sets the flex grow factor (basis defaults to 0).
func Grow(g float32) Option { return func(s *Style) { s.Grow = g } }

// Shrink sets the flex shrink factor.
func Shrink(v float32) Option { return func(s *Style) { s.Shrink = v } }

// Basis sets an explicit flex basis.
func Basis(v float32) Option { return func(s *Style) { s.Basis, s.HasBasis = v, true } }

// W fixes the width.
func W(v float32) Option { return func(s *Style) { s.Width = v } }

// H fixes the height.
func H(v float32) Option { return func(s *Style) { s.Height = v } }

// Size fixes both dimensions.
func Size(w, h float32) Option { return func(s *Style) { s.Width, s.Height = w, h } }

// MinW / MaxW / MinH / MaxH clamp the size.
func MinW(v float32) Option { return func(s *Style) { s.MinW = v } }
func MaxW(v float32) Option { return func(s *Style) { s.MaxW = v } }
func MinH(v float32) Option { return func(s *Style) { s.MinH = v } }
func MaxH(v float32) Option { return func(s *Style) { s.MaxH = v } }

// Align sets cross-axis alignment of children.
func AlignItems(a Align) Option { return func(s *Style) { s.Align = a } }

// Self sets this node's own cross-axis alignment.
func Self(a Align) Option { return func(s *Style) { s.AlignSelf = a } }

// JustifyContent sets main-axis distribution.
func JustifyContent(j Justify) Option { return func(s *Style) { s.Justify = j } }

// Center centers children on both axes.
func Center() Option {
	return func(s *Style) { s.Align, s.Justify = AlignCenter, JustifyCenter }
}

// Middle centers children on the cross axis (vertical centering in rows).
func Middle() Option { return func(s *Style) { s.Align = AlignCenter } }

// Between spreads children with space-between.
func Between() Option { return func(s *Style) { s.Justify = JustifySpaceBetween } }

// End packs children at the end of the main axis.
func End() Option { return func(s *Style) { s.Justify = JustifyEnd } }

// WrapItems enables wrapping.
func WrapItems() Option { return func(s *Style) { s.Wrap = true } }

// Fill makes the node grow on the main axis and stretch on the cross axis.
func Fill() Option { return func(s *Style) { s.Grow, s.AlignSelf = 1, AlignStretch } }

// Abs positions the node absolutely; pass Unset for unused edges.
func Abs(left, top, right, bottom float32) Option {
	return func(s *Style) { s.Absolute, s.Pos = true, Pos{left, top, right, bottom} }
}

// Dir sets the container direction.
func Dir(d Direction) Option { return func(s *Style) { s.Dir = d } }

// Physical keeps the node at its physical (LTR) position inside an RTL
// parent: its x is not mirrored. Use it for things pinned to a physical
// edge regardless of reading direction. It does not affect the node's own
// children; those follow the node's RTL flag.
func Physical() Option { return func(s *Style) { s.NoMirror = true } }

// PadSE sets vertical padding v and inline start/end padding. Left/Right of
// geom.Insets are the start/end sides (they swap physically in RTL boxes).
func PadSE(v, start, end float32) Option {
	return func(s *Style) { s.Padding = geom.Insets{Left: start, Top: v, Right: end, Bottom: v} }
}
