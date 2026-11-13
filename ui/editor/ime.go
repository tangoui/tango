package editor

import (
	"math"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/monarch"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

// Caret, caret blink and IME composition.
//
// IME lifecycle: while the view is focused and editable it asks the host
// for the text input client every frame (Context.WantTextInput) with the
// caret rectangle, which positions the candidate window. A composition
// (input.Composition) is only shown: the caret line is laid out with the
// pre-edit text in place of the selection (or at the caret), underlined,
// and the caret is drawn at the composition cursor; the Buffer and the undo
// history do not see it, so scrolling or moving the pointer while composing
// loses nothing. The composition is shown at the primary cursor only; the
// commit (input.TextInput) is inserted at every cursor, one edit set and an
// undo step of its own. Losing focus cancels the composition: nothing is written and the
// text input client is turned off; a composition the Context still reports
// after refocus is ignored until it changes. Read-only views do not enable
// the text input client.
//
// The caret blinks like Monaco's (530 ms visible, 530 ms hidden), restarting
// visible on any input and whenever the text, the selection or the
// composition changes. The view asks for the next toggle with
// Context.WakeAtDamaged (a timer frame that may be drawn partially: the
// blink declares the caret rectangles, damage.go), never while it is
// unfocused, the window has no key focus (the caret is then hidden) or its
// caret line is scrolled out of view.

// compView is how the composition is shown on the caret line.
type compView struct {
	on     bool
	r0, r1 int // inline: rune range of the composition in the caret line
	caret  int // rune index of the IME caret: in the caret line (inline) or in overlay

	// Long lines: the composition is drawn over the line at content x x0.
	overlay *text.Layout
	x0      float32
}

// compose lays out the caret line lv with comp in place of the selection
// (when it is within the line) or at the caret. On long lines and on
// wrapped lines (lv is one row) the composition is drawn over the line
// instead, from the caret.
func (w *ViewW) compose(snap buffer.Snapshot, lv *lineView, a, b, head int, comp string, cc int, th *theme.Theme) compView {
	ls := snap.LineStart(lv.i)
	le := ls + len(lv.text)
	cs, ce := head-ls, head-ls
	if a != b && a >= ls && b <= le {
		cs, ce = a-ls, b-ls
	}
	n := utf8.RuneCountInString(comp)
	cc = max(0, min(cc, n))
	if lv.long != nil || lv.rows > 1 {
		x, ok := lv.caretX(lv.runeAt(cs))
		if !ok {
			x = float32(w.st.viewColumn(snap, ls+cs, false)) * w.vs.colW
		}
		lay := w.vs.lines.Shape(comp, []text.Run{{Start: 0, End: n, Style: w.monoStyle()}}, w.lineOptions())
		return compView{on: true, caret: cc, overlay: lay, x0: x}
	}
	nt := lv.text[:cs] + comp + lv.text[ce:]
	var toks []monarch.Token
	if hl := w.st.Highlighter(); hl != nil {
		toks, _ = hl.TokensFor(w.st.hlKey, snap, lv.i, nil)
		// Shift the tokens after the replaced range; the composition takes
		// the style of the token it starts in.
		delta := len(comp) - (ce - cs)
		for k := range toks {
			switch bs := toks[k].ByteStart; {
			case bs >= ce:
				toks[k].ByteStart = bs + delta
			case bs > cs:
				toks[k].ByteStart = cs
			}
		}
	}
	runes := utf8.RuneCountInString(nt)
	sg := w.shapeSeg(lv.i, nt, 0, runes, 0, toks, th)
	r0 := utf8.RuneCountInString(lv.text[:cs])
	lv.text, lv.runes, lv.segs = nt, runes, []seg{sg}
	lv.b1, lv.r1 = len(nt), runes
	lv.endX = sg.x0 + sg.lay.Width
	return compView{on: true, r0: r0, r1: r0 + n, caret: r0 + cc}
}

// paintComposition draws the composition underline (and, on long lines,
// the composition itself over the line).
func (w *ViewW) paintComposition(l *paint.List, lv *lineView, cmp compView, tx, y, lh float32, th *theme.Theme) {
	fg := defaultColor(th, "editor.foreground")
	var x0, x1, base float32
	if lay := cmp.overlay; lay != nil {
		org := geom.Pt(tx+cmp.x0, y)
		l.Fill(rectXYWH(org.X, y, lay.Width, lh), geom.Radii{}, defaultColor(th, "editor.background"))
		l.Glyphs(org, lay.Glyphs, fg)
		x0, x1 = cmp.x0, cmp.x0+lay.Width
		if len(lay.Lines) > 0 {
			base = lay.Lines[0].Baseline
		}
	} else {
		a, ok0 := lv.caretX(cmp.r0)
		b, ok1 := lv.caretX(cmp.r1)
		if !ok0 || !ok1 {
			return
		}
		x0, x1 = min(a, b), max(a, b)
		if len(lv.segs) > 0 && len(lv.segs[0].lay.Lines) > 0 {
			base = lv.segs[0].lay.Lines[0].Baseline
		}
	}
	if base == 0 {
		base = lh * 0.75
	}
	uy := float32(math.Round(float64(y + base + 2)))
	l.Fill(rectXYWH(tx+x0, uy, x1-x0, 1), geom.Radii{}, fg)
}

// paintCaret draws the primary caret when it is in its visible blink phase
// and tells the host where it is (IME candidate window); it reports the
// blink phase (visible), which the secondary carets follow, and the caret
// rectangle with whether it drew it. lv is the caret line when it is
// visible (nil otherwise); y is its top.
func (w *ViewW) paintCaret(l *paint.List, snap buffer.Snapshot, lv *lineView, head int, cmp compView, tx, y, lh float32, th *theme.Theme) (caret geom.Rect, drawn, visible bool) {
	vs, c := w.vs, w.c
	now := c.Now()
	key := blinkKey{ver: snap.Version(), sels: selsKey(w.st.Sels), comp: vs.comp, compCur: vs.compCur}
	if !vs.blinkFocus || key != vs.blinkKey {
		vs.blinkKey, vs.activity = key, now
	}
	phase := max(now.Sub(vs.activity), 0) % blinkPeriod
	visible = phase < blinkPeriod/2
	// While the window has no key focus the caret is hidden (as Monaco's,
	// whose editor loses focus with the window) and asks for no timer
	// frames; it restarts visible when the window is focused again.
	winFocused := c.WindowFocused()
	vs.blinkFocus = winFocused
	if !winFocused {
		visible = false
	}
	// Timer frames only while the caret is on screen.
	// The blink declares its own damage (the caret rectangles, damage.go),
	// so its timer frames may be partial.
	if lv != nil && winFocused {
		if visible {
			c.WakeAtDamaged(now.Add(blinkPeriod/2 - phase))
		} else {
			c.WakeAtDamaged(now.Add(blinkPeriod - phase))
		}
	}

	var x float32
	ok := false
	if lv != nil {
		switch {
		case cmp.on && cmp.overlay != nil:
			cx, _, _ := cmp.overlay.Caret(cmp.caret)
			x, ok = cmp.x0+cx, true
		case cmp.on:
			x, ok = lv.caretX(cmp.caret)
		default:
			x, ok = lv.caretX(lv.runeAt(head - snap.LineStart(lv.i)))
		}
	}
	switch {
	case ok:
		if !cmp.on {
			vs.lastCaret = lastCaretX{ok: true, ver: snap.Version(), head: head, x: x}
		}
	case vs.lastCaret.ok && vs.lastCaret.ver == snap.Version() && vs.lastCaret.head == head:
		// Off screen, for the IME only: the x of the caret when its line
		// was last drawn. Never lay the caret line out here: that would
		// shape it and move the highlighter's token window to it every
		// frame (Tokens), away from the visible lines.
		x = vs.lastCaret.x
	default:
		// Not drawn since it moved: the column grid.
		x = float32(w.st.peekViewColumn(snap, head, w.st.caretUp())) * vs.colW
	}
	cx := float32(math.Round(float64(tx + x)))
	r := rectXYWH(cx, y, 2, lh)
	caret, drawn = r, visible && lv != nil
	if drawn {
		l.Fill(r, geom.Radii{}, defaultColor(th, "editorCursor.foreground"))
	}
	if !w.opts.ReadOnly {
		// Keep the candidate window at the view when the caret is
		// scrolled out of it.
		t := vs.text
		r.X = max(t.X, min(r.X, t.X+t.W-r.W))
		r.Y = max(t.Y, min(r.Y, t.Y+t.H-r.H))
		c.WantTextInput(r)
	}
	return caret, drawn, visible
}
