package terminal

import (
	"time"

	"github.com/tangoui/tango/ui/terminal/vt"
)

// CursorShape is the shape of a terminal caret.
type CursorShape string

const (
	CursorBlock     CursorShape = "block"     // fills the cell
	CursorUnderline CursorShape = "underline" // along the cell's bottom
	CursorBar       CursorShape = "bar"       // along the cell's left edge
)

// Cursor sets the default caret shape and blink preference for this view.
// The default is a steady block. DECSCUSR can override both settings.
func (w *ViewW) Cursor(shape CursorShape, blink bool) *ViewW {
	w.cursorShape, w.cursorBlink = shape, blink
	return w
}

// FontFamily sets the preferred comma-separated font families for this
// view. Missing fonts or glyphs fall back to Tango's monospace chain.
// Use monospace families to fit the terminal grid.
func (w *ViewW) FontFamily(families string) *ViewW {
	w.fontFamily = families
	return w
}

func (w *ViewW) cursorAppearance(style vt.CursorStyle) (CursorShape, bool) {
	switch style {
	case vt.CursorBlinkBlock, vt.CursorSteadyBlock:
		return CursorBlock, style == vt.CursorBlinkBlock
	case vt.CursorBlinkUnderline, vt.CursorSteadyUnderline:
		return CursorUnderline, style == vt.CursorBlinkUnderline
	case vt.CursorBlinkBar, vt.CursorSteadyBar:
		return CursorBar, style == vt.CursorBlinkBar
	default:
		return w.cursorShape, w.cursorBlink
	}
}

func (w *ViewW) blinkHidden(key cursorKey, blink bool) bool {
	vs := w.vs
	if !blink || !key.shown || !key.focused || key.vy < 0 || key.comp != "" || w.c.ReduceMotion() {
		vs.blinkStart = time.Time{}
		return false
	}
	now := w.c.Now()
	last := vs.dmg.cur
	if vs.blinkStart.IsZero() || last.x != key.x || last.vy != key.vy || last.shape != key.shape {
		vs.blinkStart = now
	}
	const halfPeriod = 500 * time.Millisecond
	phase := max(now.Sub(vs.blinkStart)/halfPeriod, 0)
	w.c.WakeAt(vs.blinkStart.Add((phase + 1) * halfPeriod))
	return phase%2 != 0
}
