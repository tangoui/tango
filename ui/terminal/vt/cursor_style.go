package vt

// CursorStyle is the cursor appearance requested by DECSCUSR.
type CursorStyle uint8

const (
	CursorDefault         CursorStyle = iota // use the view's preference
	CursorBlinkBlock                         // blinking block
	CursorSteadyBlock                        // steady block
	CursorBlinkUnderline                     // blinking underline
	CursorSteadyUnderline                    // steady underline
	CursorBlinkBar                           // blinking vertical bar
	CursorSteadyBar                          // steady vertical bar
)

func (t *Terminal) setCursorStyle(p *Params) {
	if p.Len() > 1 || len(p.Sub(0)) > 1 {
		return
	}
	switch n := p.Get(0, 0); {
	case n == 0:
		t.cursorStyle = CursorBlinkBlock
	case n >= 1 && n <= 6:
		t.cursorStyle = CursorStyle(n)
	}
}

// EchoCursor returns a position suitable for considering local echo. It
// requires plain text parser state, the normal screen at its live end and a
// visible cursor without a pending wrap. Callers still need to confirm echo.
func (t *Terminal) EchoCursor() (x, y int, ok bool) {
	return t.cx, t.cy, t.p.state == sGround && t.p.nUTF == 0 &&
		t.alternateMode == 0 && t.s.offset == 0 && t.modes.ShowCursor && !t.pending
}
