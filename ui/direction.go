package ui

// Direction is the inline (horizontal) layout direction of a scope:
// left-to-right or right-to-left. It is unrelated to layout.Direction
// (Row / Column) and text.Direction (paragraph base direction).
//
// Append-only: never reorder or insert; the zero value stays DirAuto.
type Direction uint8

const (
	// DirAuto derives the direction from the scope's Locale.
	DirAuto Direction = iota
	// LTR lays out from left to right.
	LTR
	// RTL lays out from right to left: rows start at the right edge,
	// Padding/Margin Left act as the start side, directional icons mirror.
	RTL
)

// dirState is the resolved direction of the current build scope. The node
// env capture (docs/specs/rtl-and-component-tokens.md §2) switches it during
// paint and measure.
type dirState struct {
	rtl bool
}

// Direction reports the resolved direction of the current scope: LTR or RTL,
// never DirAuto.
func (c *Context) Direction() Direction {
	if c.dir.rtl {
		return RTL
	}
	return LTR
}

// IsRTL reports whether the current scope lays out right to left. During
// paint and measure callbacks it reports the direction of the node being
// painted or measured.
func (c *Context) IsRTL() bool { return c.dir.rtl }
