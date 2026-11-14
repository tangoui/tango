// Package flexstyle holds the layout input of a node, shared by the public
// option constructors (package ui/layout) and the solver (package
// ui/internal/flexbox).
//
// Style is generic over the enum types only so that it can live below
// ui/layout: the enums are defined there, and ui/layout cannot import the
// solver, which needs them. Both packages use the single instantiation
// Style[layout.Direction, layout.Align, layout.Justify].
package flexstyle

import (
	"math"

	"github.com/tangoui/tango/ui/geom"
)

// Pos holds absolute offsets; NaN (Unset) means unset.
type Pos struct{ Left, Top, Right, Bottom float32 }

// Unset is the value of an unset Pos edge.
var Unset = float32(math.NaN())

// Style is the layout input of a node. The zero value is an auto-sized
// Column with stretch alignment.
type Style[D, A, J ~uint8] struct {
	Dir                    D
	Width, Height          float32 // fixed border-box size; 0 = auto
	MinW, MinH, MaxW, MaxH float32 // 0 = none
	Grow, Shrink           float32
	Basis                  float32
	HasBasis               bool
	Padding                geom.Insets
	Margin                 geom.Insets
	Gap                    float32 // between items on the main axis
	LineGap                float32 // between wrapped lines
	Align                  A       // cross alignment of children
	AlignSelf              A
	Justify                J
	Wrap                   bool
	Absolute               bool // out of flow; positioned by Pos in the parent's box
	Pos                    Pos
	ScrollX, ScrollY       bool // content may exceed the box on this axis
	// NoStretch keeps the item's own cross size when the resolved alignment
	// is Stretch (it is then placed at the start). Buttons and tags use it
	// so they don't widen in columns.
	NoStretch bool
	// RTL mirrors the children's x inside this box after the logical (LTR)
	// solve: X' = span - X - W, span being the box width (or the content
	// width when ScrollX). Padding/Margin/Pos Left then act as the start
	// side. Set by the ui package from the scope's direction.
	RTL bool
	// NoMirror keeps this node at its physical position inside an RTL
	// parent (layout.Physical).
	NoMirror bool
}

// ApplyOption runs opt (a layout.Option) on style (a *Style of the
// solver's instantiation); a zero option does nothing. Package ui/layout
// installs it in its init. It is not part of any public API: it exists so
// that layout.Option can stay opaque.
var ApplyOption func(opt, style any)
