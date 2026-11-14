// Package layout holds the layout options of the Tango self-drawn UI:
// Row/Column/Stack containers take Options built here (Grow, W, H, Pad,
// Gap, Align, Justify, Abs ...), and Box.Style applies more. The layout
// solver itself (flexbox-style: Grow/Shrink/Basis, Gap, Padding, Margin,
// Align, Justify, Wrap, Stack and absolute positioning) is internal.
//
// API tiers: only the symbols listed in api/stable/ui_layout.txt are covered
// by the compatibility promise of docs/release.md ("API 稳定性"); every
// other exported symbol is Experimental and may change in any 0.x
// release. The API reference on the documentation site marks the
// experimental symbols.
package layout

import (
	"math"

	"github.com/tangoui/tango/ui/internal/flexbox/flexstyle"
)

// Direction is the main axis of a container.
//
// Append-only: never reorder or insert; the zero value stays Column.
type Direction uint8

const (
	Column Direction = iota // children top to bottom (default)
	Row                     // children left to right
	Stack                   // children overlap, aligned in both axes
)

// Align positions items on the cross axis.
//
// Append-only: never reorder or insert; the zero value stays AlignAuto.
type Align uint8

const (
	AlignAuto    Align = iota // AlignSelf: inherit; Align: same as Stretch
	AlignStretch              // fill the cross axis
	AlignStart
	AlignCenter
	AlignEnd
)

// Justify distributes free space on the main axis.
//
// Append-only: never reorder or insert; the zero value stays JustifyStart.
type Justify uint8

const (
	JustifyStart Justify = iota
	JustifyCenter
	JustifyEnd
	JustifySpaceBetween
	JustifySpaceAround
	JustifySpaceEvenly
)

// Unset marks an unused edge of Abs. It is a variable only because Go has
// no NaN constant: treat it as a constant and never assign to it.
var Unset = float32(math.NaN())

// style is the solver's node style that options write to.
type style = flexstyle.Style[Direction, Align, Justify]

// Option sets one aspect of a node's layout. It is opaque: build it with
// the functions of this package and pass it to containers (ui.Row,
// ui.Column, ui.Div ...) or Box.Style. The zero Option does nothing.
type Option struct {
	apply func(*style)
}

func init() {
	flexstyle.ApplyOption = func(opt, s any) {
		if f := opt.(Option).apply; f != nil {
			f(s.(*style))
		}
	}
}
