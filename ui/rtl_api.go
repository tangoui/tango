package ui

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// Exported RTL helpers for components written outside package ui (the
// mobile and chart packages, third-party widgets). They are thin wrappers
// over the helpers the built-in components use; see AUTHORING.md §18.

// InlineSpan places things along a rectangle's inline axis by start/end
// offsets: in RTL scopes offsets run from the right edge.
type InlineSpan = inlineSpan

// Inline returns the inline span of r in the current scope's direction.
// Call it during build, or during paint (the node's own scope is current).
func (c *Context) Inline(r geom.Rect) InlineSpan { return c.inline(r) }

// InlineKey reports whether k means "previous" or "next" along the inline
// axis: Left/Right swap in RTL scopes.
func (c *Context) InlineKey(k input.Key) (prev, next bool) { return c.inlineKey(k) }

// TextOpts sets the paragraph base direction of interface text (labels,
// titles) from the scope: RTL scopes get text.DirRTL, LTR scopes keep the
// options unchanged. User content should keep text.DirAuto instead.
func (c *Context) TextOpts(o text.Options) text.Options { return c.textOpts(o) }

// PaintIcon draws ic in r, mirroring directional icons in RTL scopes
// (rotation first, then the mirror).
func (c *Context) PaintIcon(l *paint.List, ic *icon.Icon, r geom.Rect, col theme.Color, angle float64) {
	c.paintIcon(l, ic, r, col, angle)
}
