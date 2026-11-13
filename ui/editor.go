package ui

import (
	"cmp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// editState is the retained state of a text editor: caret, selection
// anchor (rune indices), scroll and the last layout for hit testing.
type editState struct {
	cur, anc   int
	scroll     geom.Point
	activity   time.Time
	lay        *text.Layout
	origin     geom.Point // text origin of the last paint (absolute)
	dragging   bool
	wasFocused bool
	// dir is the paragraph base direction of the content; DirAuto (the
	// default for user content) takes it from the first strong character
	// and, when there is none (empty or digits only), from the scope.
	dir text.Direction
	// shift[i] is the x offset of line i of lay inside the editor box:
	// right-to-left lines are aligned to the right (their start edge).
	// Hit testing and caret queries add it to the layout coordinates.
	shift []float32
	// lead selects the second visual position of cur. At a direction
	// boundary a logical position is drawn at two places: the trailing
	// edge of rune cur-1 (text.Layout.Caret, the default) and the leading
	// edge of rune cur. Visual arrow movement (visualStep) sets lead when
	// it lands on the latter; every other caret change clears it. It is
	// never set on a line without right-to-left text.
	lead bool
	// hints describe the field to the input method; the owning widget
	// sets the kind and the return key (Input.Kind, Input.EnterKey) and
	// processEdit completes them from the editor's configuration.
	hints input.TextHints
	// imeHints are the hints in effect this frame (processEdit).
	imeHints input.TextHints
}

// lineAt is the line of lay at layout y (the line IndexAt picks).
func (e *editState) lineAt(y float32) int {
	if e.lay == nil || len(e.lay.Lines) == 0 || e.lay.LineHeight <= 0 {
		return 0
	}
	return max(0, min(int(y/e.lay.LineHeight), len(e.lay.Lines)-1))
}

func (e *editState) shiftOf(n int) float32 {
	if n >= 0 && n < len(e.shift) {
		return e.shift[n]
	}
	return 0
}

// indexAt is the rune index at p, in coordinates relative to e.origin
// (before the per-line alignment shift).
func (e *editState) indexAt(p geom.Point) int {
	return e.lay.IndexAt(p.X-e.shiftOf(e.lineAt(p.Y)), p.Y)
}

// caret is lay.Caret(i) with the line's alignment shift applied.
func (e *editState) caret(i int) (x, top, h float32) {
	x, top, h = e.lay.Caret(i)
	if len(e.lay.Lines) > 0 {
		x += e.shiftOf(e.lay.LineOf(i))
	}
	return
}

// curCaret is caret(e.cur), honoring e.lead.
func (e *editState) curCaret() (x, top, h float32) {
	x, top, h = e.caret(e.cur)
	if e.lead && e.lay != nil && len(e.lay.Lines) > 0 {
		n := e.lay.LineOf(e.cur)
		x = editCaretX(e.lay, e.cur, true) + e.shiftOf(n)
	}
	return
}

// rtlAt reports whether the caret at rune index i sits in a right-to-left
// paragraph (per the last painted layout).
func (e *editState) rtlAt(i int) bool {
	if e.lay == nil || len(e.lay.Lines) == 0 {
		return false
	}
	return e.lay.Lines[e.lay.LineOf(i)].RTL
}

// editLineShifts fills dst with the x offset of every line of lay inside a
// box of width w: right-to-left lines are aligned to their start edge (the
// right), keeping 1px for a caret drawn at that edge; left-to-right lines
// stay at 0. An overflowing single line gets a negative shift so its start
// still sits at the right edge (scrolling reveals the rest).
func editLineShifts(dst []float32, lay *text.Layout, w float32, multiline bool) []float32 {
	dst = dst[:0]
	for _, ln := range lay.Lines {
		var s float32
		if ln.RTL {
			s = w - 1 - ln.Width
			if multiline {
				s = max(0, s)
			}
		}
		dst = append(dst, s)
	}
	return dst
}

// editTextDir is the base direction to shape the editor's text with: an
// explicit dir wins; DirAuto content without any letter (empty, digits,
// punctuation) follows an RTL scope, like HTML dir="auto" falls back to
// the parent direction, so an empty field in RTL has its caret at the
// right.
func (c *Context) editTextDir(dir text.Direction, s string) text.Direction {
	if dir != text.DirAuto || !c.dir.rtl {
		return dir
	}
	for _, r := range s {
		if unicode.IsLetter(r) {
			return text.DirAuto
		}
	}
	return text.DirRTL
}

// Visual caret movement (RTL spec Q12, phase 2).
//
// Left and Right move the caret to the character visually adjacent on
// screen, as native text fields do with bidirectional text. The caret is a
// logical position plus editState.lead; the stops of a line are derived
// from the public text.Layout API alone (per-rune boxes from
// SelectionRects, rune directions from Caret):
//
//   - every rune k with a visible box contributes two candidate stops: its
//     leading edge, which is position k drawn with lead, and its trailing
//     edge, which is position k+1 drawn at Caret(k+1) (dropped when k+1
//     belongs to the next line, i.e. at a soft wrap);
//   - a lead candidate whose position already draws there by default
//     (Caret(k) equals the leading edge, the usual case away from
//     direction boundaries) is turned into a plain one, so lead is only
//     used where it is the sole way to reach an edge;
//   - candidates are grouped by x and sorted left to right. A group can hold
//     two positions (in "abc אבג", 4 after the space and 7 after ג share the
//     x between the space and ג); stepping into such a group picks the
//     position logically nearest the one it leaves (the rune just crossed),
//     ties preferring the plain position.
//
// Past the last stop of a line in the paragraph's forward direction (Right
// in a left-to-right line, Left in a right-to-left one) the caret goes to
// the start edge of the next line; past the first stop backwards it goes to
// the end edge of the previous line. Lines without right-to-left text keep
// the logical rune-by-rune movement of before.

// editRuneBox is the visual box [l, r] (layout x) of one rune of a line and
// its resolved direction; ok is false for runes without a visible box
// (zero-width marks), which contribute no stops.
type editRuneBox struct {
	l, r    float32
	rtl, ok bool
}

func (b editRuneBox) leadX() float32 {
	if b.rtl {
		return b.r
	}
	return b.l
}

func (b editRuneBox) trailX() float32 {
	if b.rtl {
		return b.l
	}
	return b.r
}

// editRuneBoxes returns the box of every rune of line n of lay.
//
// The direction of rune k comes from its trailing edge, Caret(k+1). For the
// last rune of a soft-wrapped line (k+1 is on the next line) it comes from
// the leading edge when k starts the line; otherwise whitespace takes the
// paragraph direction (UAX #9 rule L1 resets trailing whitespace) and any
// other rune (a word broken mid-way) the direction of the rune before it.
func editRuneBoxes(lay *text.Layout, n int) []editRuneBox {
	ln := lay.Lines[n]
	out := make([]editRuneBox, max(0, ln.End-ln.Start))
	for k := ln.Start; k < ln.End; k++ {
		b := &out[k-ln.Start]
		b.rtl = ln.RTL
		rs := lay.SelectionRects(k, k+1)
		if len(rs) == 0 || rs[0].W <= 0 {
			continue
		}
		b.l, b.r, b.ok = rs[0].X, rs[0].X+rs[0].W, true
		switch {
		case k+1 < ln.End || lay.LineOf(k+1) == n:
			t, _, _ := lay.Caret(k + 1)
			b.rtl = abs32(t-b.l) < abs32(t-b.r)
		case k == ln.Start:
			s, _, _ := lay.Caret(k)
			b.rtl = abs32(s-b.r) < abs32(s-b.l)
		case k < len(lay.Runes) && unicode.IsSpace(lay.Runes[k]):
			// b.rtl stays ln.RTL
		case k > ln.Start:
			b.rtl = out[k-1-ln.Start].rtl
		}
	}
	return out
}

// editLineMixed reports whether line n needs visual caret movement: a
// right-to-left paragraph, or any right-to-left rune. Lines made only of
// runes below U+0590 (no Hebrew, Arabic or bidi controls) are decided
// without measuring.
func editLineMixed(lay *text.Layout, n int) bool {
	ln := lay.Lines[n]
	if ln.RTL {
		return true
	}
	need := false
	for k := ln.Start; k < ln.End && k < len(lay.Runes); k++ {
		if lay.Runes[k] >= 0x590 {
			need = true
			break
		}
	}
	if !need {
		return false
	}
	for _, b := range editRuneBoxes(lay, n) {
		if b.ok && b.rtl {
			return true
		}
	}
	return false
}

// editCaretX is the caret x (layout coordinates, before the line shift) of
// position i: the leading edge of rune i when lead is set and rune i is on
// the caret's line with a visible box, Caret(i) otherwise.
func editCaretX(lay *text.Layout, i int, lead bool) float32 {
	x, _, _ := lay.Caret(i)
	if !lead || len(lay.Lines) == 0 {
		return x
	}
	n := lay.LineOf(i)
	ln := lay.Lines[n]
	if i < ln.Start || i >= ln.End {
		return x
	}
	if b := editRuneBoxes(lay, n)[i-ln.Start]; b.ok {
		return b.leadX()
	}
	return x
}

// editStop is one caret stop of a line: position pos, drawn at x (with
// lead: the leading edge of rune pos).
type editStop struct {
	x    float32
	pos  int
	lead bool
}

// editLineStops returns the caret stops of line n grouped by x, left to
// right (see the comment above editRuneBox). A line without visible runes
// has the single stop at its start.
func editLineStops(lay *text.Layout, n int) [][]editStop {
	const eps = 0.01
	ln := lay.Lines[n]
	var cands []editStop
	for k, b := range editRuneBoxes(lay, n) {
		if !b.ok {
			continue
		}
		p := ln.Start + k
		lx := b.leadX()
		if c, _, _ := lay.Caret(p); abs32(c-lx) < eps {
			cands = append(cands, editStop{x: lx, pos: p})
		} else {
			cands = append(cands, editStop{x: lx, pos: p, lead: true})
		}
		tx := b.trailX()
		if lay.LineOf(p+1) == n {
			if c, _, _ := lay.Caret(p + 1); abs32(c-tx) < eps {
				cands = append(cands, editStop{x: tx, pos: p + 1})
			}
		}
	}
	if len(cands) == 0 {
		x, _, _ := lay.Caret(ln.Start)
		return [][]editStop{{{x: x, pos: ln.Start}}}
	}
	slices.SortStableFunc(cands, func(a, b editStop) int { return cmp.Compare(a.x, b.x) })
	var groups [][]editStop
	for _, s := range cands {
		if g := len(groups) - 1; g >= 0 && abs32(groups[g][0].x-s.x) < eps {
			if !slices.ContainsFunc(groups[g], func(o editStop) bool { return o.pos == s.pos && o.lead == s.lead }) {
				groups[g] = append(groups[g], s)
			}
			continue
		}
		groups = append(groups, []editStop{s})
	}
	return groups
}

// pickStop returns the stop of g whose position is nearest to "to", ties
// preferring a plain (non-lead) stop, then the smaller position.
func pickStop(g []editStop, to int) editStop {
	best := g[0]
	for _, s := range g[1:] {
		d, bd := editAbs(s.pos-to), editAbs(best.pos-to)
		if d < bd || (d == bd && best.lead && !s.lead) {
			best = s
		}
	}
	return best
}

func editAbs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// visualStep moves the caret (e.cur, lead) one stop right or left on
// screen in text of nr runes. ok is false when the caret's line has no
// right-to-left text (or the layout is stale): the caller then moves
// logically, exactly as before visual movement existed.
func (e *editState) visualStep(nr int, lead, right bool) (pos int, plead, ok bool) {
	lay := e.lay
	if lay == nil || len(lay.Lines) == 0 || len(lay.Runes) != nr {
		return 0, false, false
	}
	n := lay.LineOf(e.cur)
	ln := lay.Lines[n]
	if !editLineMixed(lay, n) {
		// Left from the start of a left-to-right line below a line with
		// right-to-left text goes to that line's visual end edge (the
		// logical previous position may be drawn mid-line); everything
		// else on such a line stays logical.
		if right || e.cur != ln.Start || n == 0 || !editLineMixed(lay, n-1) {
			return 0, false, false
		}
		return e.lineEnd(n - 1)
	}
	if e.cur < ln.Start || e.cur >= ln.End {
		lead = false
	}
	groups := editLineStops(lay, n)
	at := -1
	find := func(lead bool) {
		for gi, g := range groups {
			for _, s := range g {
				if s.pos == e.cur && s.lead == lead {
					at = gi
					return
				}
			}
		}
	}
	find(lead)
	if at < 0 && lead {
		find(false)
	}
	if at < 0 {
		// Not a stop (e.g. after a zero-width mark): the nearest by x.
		x := editCaretX(lay, e.cur, lead)
		bd := float32(-1)
		for gi, g := range groups {
			if d := abs32(g[0].x - x); bd < 0 || d < bd {
				at, bd = gi, d
			}
		}
	}
	next := at - 1
	if right {
		next = at + 1
	}
	if next >= 0 && next < len(groups) {
		s := pickStop(groups[next], e.cur)
		return s.pos, s.lead, true
	}
	// Past the line's visual edge: forward goes to the start edge of the
	// next line, backward to the end edge of the previous one.
	if right != ln.RTL {
		if n+1 >= len(lay.Lines) {
			return e.cur, lead, true
		}
		nl := lay.Lines[n+1]
		g := editLineStops(lay, n+1)
		edge := g[0]
		if nl.RTL {
			edge = g[len(g)-1]
		}
		s := pickStop(edge, nl.Start)
		return s.pos, s.lead, true
	}
	if n == 0 {
		return e.cur, lead, true
	}
	return e.lineEnd(n - 1)
}

// lineEnd is the stop at the visual end edge of line n (its right in a
// left-to-right line, its left in a right-to-left one).
func (e *editState) lineEnd(n int) (pos int, lead, ok bool) {
	ln := e.lay.Lines[n]
	g := editLineStops(e.lay, n)
	edge := g[len(g)-1]
	if ln.RTL {
		edge = g[0]
	}
	s := pickStop(edge, ln.End)
	return s.pos, s.lead, true
}

func (e *editState) sel() (int, int) {
	if e.cur < e.anc {
		return e.cur, e.anc
	}
	return e.anc, e.cur
}

func (e *editState) hasSel() bool { return e.cur != e.anc }

// editResult reports what happened during processing.
type editResult struct {
	changed, submitted bool
}

type editConfig struct {
	multiline bool
	maxLen    int
	readOnly  bool
	filter    func(r rune) bool // accept rune (InputNumber)
}

func clampIdx(i, n int) int { return max(0, min(i, n)) }

// editHints completes the widget's input-method hints from the editor
// configuration: multi-line editors insert line breaks with the return
// key, filtered editors (InputNumber) get a decimal keypad, passwords
// never autocorrect.
func editHints(h input.TextHints, cfg editConfig) input.TextHints {
	if cfg.multiline {
		h.Multiline = true
		if h.Enter == input.EnterDefault {
			h.Enter = input.EnterNewline
		}
	}
	if cfg.filter != nil && h.Kind == input.TextPlain {
		h.Kind = input.TextDecimal
	}
	if h.Kind == input.TextPassword {
		h.NoAutocorrect = true
	}
	return h
}

// processEdit applies this frame's keyboard, text and pointer input to *val.
func (c *Context) processEdit(id ID, e *editState, val *string, cfg editConfig, ia Interaction) editResult {
	var res editResult
	rs := []rune(*val)
	e.cur = clampIdx(e.cur, len(rs))
	e.anc = clampIdx(e.anc, len(rs))
	focused := c.focus == id
	e.imeHints = editHints(e.hints, cfg)
	if focused && !e.wasFocused {
		e.activity = c.now
		if !ia.PressedNow {
			e.lead = false
			e.cur, e.anc = len(rs), 0 // tab-in selects all, like browsers
			if len(rs) == 0 {
				e.anc = 0
			}
		}
	}
	e.wasFocused = focused
	// Pointer.
	if ia.PressedNow && e.lay != nil {
		e.lead = false
		i := e.indexAt(c.pointer.Sub(e.origin))
		e.cur = i
		e.anc = i
		e.dragging = true
		if c.clickCount >= 2 {
			e.anc, e.cur = wordBounds(rs, i)
		}
		e.activity = c.now
	}
	if e.dragging {
		if c.active != id {
			e.dragging = false
		} else if e.lay != nil && c.clickCount < 2 {
			e.cur = e.indexAt(c.pointer.Sub(e.origin))
			e.lead = false
		}
	}
	if !focused {
		return res
	}
	insert := func(s string) {
		if cfg.readOnly {
			return
		}
		ins := []rune(s)
		if cfg.filter != nil {
			out := ins[:0:0]
			for _, r := range ins {
				if cfg.filter(r) {
					out = append(out, r)
				}
			}
			ins = out
		}
		if !cfg.multiline {
			out := ins[:0:0]
			for _, r := range ins {
				if r != '\n' && r != '\r' {
					out = append(out, r)
				}
			}
			ins = out
		}
		a, b := e.sel()
		if cfg.maxLen > 0 && len(rs)-(b-a)+len(ins) > cfg.maxLen {
			ins = ins[:max(0, cfg.maxLen-(len(rs)-(b-a)))]
		}
		if len(ins) == 0 && a == b {
			return
		}
		nr := make([]rune, 0, len(rs)-(b-a)+len(ins))
		nr = append(nr, rs[:a]...)
		nr = append(nr, ins...)
		nr = append(nr, rs[b:]...)
		rs = nr
		e.cur = a + len(ins)
		e.anc = e.cur
		e.lead = false
		res.changed = true
	}
	if v, ok := c.takeSetValue(id); ok {
		// Accessibility SetValue: replace the whole text.
		e.anc, e.cur = 0, len(rs)
		insert(v)
	}
	// Input-method edits (textstate.go): replace a range, move the
	// selection. Positions are field runes.
	imeEdit := func(ev input.Event) {
		switch ev := ev.(type) {
		case input.TextEdit:
			a, b := clampIdx(min(ev.Start, ev.End), len(rs)), clampIdx(max(ev.Start, ev.End), len(rs))
			e.anc, e.cur = a, b
			insert(ev.Text)
			e.anc = e.cur
		case input.SelectionChange:
			e.anc, e.cur = clampIdx(ev.Start, len(rs)), clampIdx(ev.End, len(rs))
			e.lead = false
		}
		e.activity = c.now
	}
	handleKey := func(k input.KeyEvent) {
		if !k.Down {
			return
		}
		shift := k.Mods.Has(input.ModShift)
		short := k.Mods.Shortcut()
		// Every caret or text change below except a visual step clears
		// lead; keys that change nothing keep it.
		lead := e.lead
		e.lead = false
		cur0, anc0, n0 := e.cur, e.anc, len(rs)
		move := func(to int) {
			e.cur = clampIdx(to, len(rs))
			if !shift {
				e.anc = e.cur
			}
		}
		key := k.Key
		if (key == input.KeyLeft || key == input.KeyRight) && !short && !k.Mods.Has(input.ModAlt) && (shift || !e.hasSel()) {
			// Plain (or Shift-extended) Left/Right move visually on lines
			// with right-to-left text (RTL spec Q12 phase 2, see
			// visualStep); the selection anchor stays a logical position.
			if pos, pl, ok := e.visualStep(len(rs), lead, key == input.KeyRight); ok {
				move(pos)
				e.lead = pl
				e.activity = c.now
				return
			}
		}
		if (key == input.KeyLeft || key == input.KeyRight) && e.rtlAt(e.cur) {
			// Word (Alt), line (Cmd/Ctrl) movement and collapsing a
			// selection stay logical: in a right-to-left paragraph Left
			// is the logical next word / end / selection end and Right
			// the previous one (the phase 1 swap). Plain arrows only get
			// here when the layout is stale.
			if key == input.KeyLeft {
				key = input.KeyRight
			} else {
				key = input.KeyLeft
			}
		}
		switch key {
		case input.KeyLeft:
			if e.hasSel() && !shift {
				a, _ := e.sel()
				move(a)
			} else if short {
				move(0)
			} else if k.Mods.Has(input.ModAlt) {
				move(prevWord(rs, e.cur))
			} else {
				move(e.cur - 1)
			}
		case input.KeyRight:
			if e.hasSel() && !shift {
				_, b := e.sel()
				move(b)
			} else if short {
				move(len(rs))
			} else if k.Mods.Has(input.ModAlt) {
				move(nextWord(rs, e.cur))
			} else {
				move(e.cur + 1)
			}
		case input.KeyHome:
			move(lineStart(rs, e.cur, cfg.multiline))
		case input.KeyEnd:
			move(lineEnd(rs, e.cur, cfg.multiline))
		case input.KeyUp, input.KeyDown:
			if cfg.multiline && e.lay != nil {
				x, top, h := e.curCaret()
				y := top - h/2
				if k.Key == input.KeyDown {
					y = top + h*1.5
				}
				if y < 0 {
					move(0)
				} else if y > e.lay.Height {
					move(len(rs))
				} else {
					move(e.indexAt(geom.Pt(x, y)))
				}
			} else if k.Key == input.KeyUp {
				move(0)
			} else {
				move(len(rs))
			}
		case input.KeyBackspace:
			if cfg.readOnly {
				return
			}
			if !e.hasSel() {
				if e.cur == 0 {
					return
				}
				if k.Mods.Has(input.ModAlt) {
					e.anc = prevWord(rs, e.cur)
				} else {
					e.anc = e.cur - 1
				}
			}
			insert("")
		case input.KeyDelete:
			if cfg.readOnly {
				return
			}
			if !e.hasSel() {
				if e.cur >= len(rs) {
					return
				}
				e.anc = e.cur + 1
			}
			insert("")
		case input.KeyEnter:
			if cfg.multiline {
				insert("\n")
			} else {
				res.submitted = true
			}
		case input.KeyA:
			if short {
				e.anc, e.cur = 0, len(rs)
			}
		case input.KeyC, input.KeyX:
			if short && e.hasSel() && c.win != nil {
				a, b := e.sel()
				c.win.WriteClipboard(string(rs[a:b]))
				if k.Key == input.KeyX {
					insert("")
				}
			}
		case input.KeyV:
			if short && c.win != nil {
				insert(c.win.ReadClipboard())
			}
		}
		if e.cur == cur0 && e.anc == anc0 && len(rs) == n0 {
			e.lead = lead
		}
		e.activity = c.now
	}
	if edits := c.TextEditsFor(id); len(edits) > 0 && len(c.events) > 0 {
		// The ordered stream holds the edits among the keys and text:
		// apply everything in arrival order (an input method deletes
		// around the caret, then commits).
		evs := c.events
		c.events, c.text, c.keys = nil, nil, nil
		for _, ev := range evs {
			switch ev := ev.(type) {
			case input.TextInput:
				insert(ev.Text)
			case input.KeyEvent:
				if ev.Key != input.KeyEscape {
					handleKey(ev)
				}
			case input.TextEdit, input.SelectionChange:
				imeEdit(ev)
			}
		}
	} else {
		for _, t := range c.TextFor(id) {
			insert(t)
		}
		for _, k := range c.KeysFor(id) {
			handleKey(k)
		}
		for _, ev := range edits {
			imeEdit(ev)
		}
	}
	if res.changed {
		*val = string(rs)
		e.activity = c.now
		c.invalid = true
	}
	return res
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

func wordBounds(rs []rune, i int) (int, int) {
	a, b := i, i
	for a > 0 && isWordRune(rs[a-1]) {
		a--
	}
	for b < len(rs) && isWordRune(rs[b]) {
		b++
	}
	return a, b
}

func prevWord(rs []rune, i int) int {
	for i > 0 && !isWordRune(rs[i-1]) {
		i--
	}
	for i > 0 && isWordRune(rs[i-1]) {
		i--
	}
	return i
}

func nextWord(rs []rune, i int) int {
	for i < len(rs) && !isWordRune(rs[i]) {
		i++
	}
	for i < len(rs) && isWordRune(rs[i]) {
		i++
	}
	return i
}

func lineStart(rs []rune, i int, multi bool) int {
	if !multi {
		return 0
	}
	for i > 0 && rs[i-1] != '\n' {
		i--
	}
	return i
}

func lineEnd(rs []rune, i int, multi bool) int {
	if !multi {
		return len(rs)
	}
	for i < len(rs) && rs[i] != '\n' {
		i++
	}
	return i
}

// paintEditor draws text, selection, IME composition and caret inside r
// (already clipped by the caller). It scrolls to keep the caret visible.
func (c *Context) paintEditor(l *paint.List, r geom.Rect, id ID, e *editState, val string, st text.Style, col theme.Color, placeholder string, password bool, multiline bool) {
	th := c.Theme
	focused := c.focus == id
	shown := val
	if password {
		shown = strings.Repeat("•", len([]rune(val)))
	}
	comp, compCur := "", 0
	if focused {
		comp, compCur = c.CompositionFor(id)
	}
	rs := []rune(shown)
	cur := clampIdx(e.cur, len(rs))
	display := shown
	if comp != "" {
		display = string(rs[:cur]) + comp + string(rs[cur:])
	}
	opt := text.Options{NoWrap: !multiline, Direction: c.editTextDir(e.dir, display)}
	if multiline {
		opt.MaxWidth = r.W
	}
	lay := text.Shape(display, st, opt)
	// Layout for hit testing excludes the composition.
	if comp == "" {
		e.lay = lay
	} else {
		e.lay = text.Shape(shown, st, opt)
	}
	e.shift = editLineShifts(e.shift, e.lay, r.W, multiline)
	// Line offsets of the painted layout (the same as e.shift without a
	// composition); nil when every line is left-to-right.
	var shift []float32
	if comp == "" {
		shift = e.shift
	} else {
		shift = editLineShifts(nil, lay, r.W, multiline)
	}
	aligned := false
	for _, v := range shift {
		aligned = aligned || v != 0
	}
	lineShift := func(n int) float32 {
		if n >= 0 && n < len(shift) {
			return shift[n]
		}
		return 0
	}
	caretAt := func(i int) (x, top, h float32) {
		x, top, h = lay.Caret(i)
		if aligned && len(lay.Lines) > 0 {
			x += lineShift(lay.LineOf(i))
		}
		return
	}
	caretIdx := cur
	if comp != "" {
		caretIdx = cur + clampIdx(compCur, len([]rune(comp)))
	}
	cx, ctop, ch := caretAt(caretIdx)
	if e.lead && comp == "" && len(lay.Lines) > 0 {
		// The caret's second visual position (see editState.lead).
		cx = editCaretX(lay, cur, true) + lineShift(lay.LineOf(cur))
	}
	// Horizontal / vertical scroll to keep the caret in view.
	if !multiline {
		if cx-e.scroll.X > r.W-1 {
			e.scroll.X = cx - r.W + 1
		}
		if cx-e.scroll.X < 0 {
			e.scroll.X = cx
		}
		if len(lay.Lines) > 0 && lay.Lines[0].RTL {
			// The start edge is on the right: scroll is <= 0 and reveals
			// overflow on the left.
			e.scroll.X = max(min(0, lineShift(0), cx), min(e.scroll.X, 0))
		} else {
			e.scroll.X = max(0, min(e.scroll.X, max(0, lay.Width-r.W+1)))
		}
		e.scroll.Y = 0
	} else {
		if ctop+ch-e.scroll.Y > r.H {
			e.scroll.Y = ctop + ch - r.H
		}
		if ctop-e.scroll.Y < 0 {
			e.scroll.Y = ctop
		}
	}
	oy := r.Y
	if !multiline {
		oy = r.Y + (r.H-lay.LineHeight)/2
	}
	origin := geom.Pt(r.X-e.scroll.X, oy-e.scroll.Y)
	e.origin = origin
	if display == "" && placeholder != "" {
		// The Input component token (its default is TextQuaternary).
		phCol := c.ThemeFor(theme.CompInput).Components.Input.PlaceholderColor
		// The placeholder is interface text: in an RTL scope it starts at
		// the right.
		pl := text.Shape(placeholder, st, c.textOpts(opt))
		if len(pl.Lines) > 0 && pl.Lines[0].RTL {
			ps := editLineShifts(nil, pl, r.W, multiline)
			paintAlignedGlyphs(l, pl, func(n int) float32 { return origin.X + ps[n] }, origin.Y, phCol)
		} else {
			l.Glyphs(origin, pl.Glyphs, phCol)
		}
	}
	// Selection.
	if e.hasSel() && comp == "" {
		a, b := e.sel()
		selCol := theme.Fade(th.Primary.Base, 0.22)
		if !focused {
			selCol = theme.Fade(th.Text, 0.08)
		}
		for n, ln := range lay.Lines {
			s0, s1 := max(a, ln.Start), min(b, ln.End)
			if s0 > s1 || (s0 == s1 && !(b > ln.End && s0 == ln.End)) {
				continue
			}
			sh := lineShift(n)
			rects := lay.SelectionRects(s0, s1)
			// A selected line break shows as a short stretch past the
			// line's logical end (its right in LTR, its left in RTL).
			if b > ln.End {
				x, _, _ := lay.Caret(ln.End)
				if lay.LineOf(ln.End) != n {
					x = 0
					if !ln.RTL {
						x = ln.Width
					}
				}
				brk := geom.Rect{X: x, Y: ln.Top, W: 4, H: lay.LineHeight}
				if ln.RTL {
					brk.X -= 4
				}
				// Join it to the stretch it touches so one fill covers both.
				joined := false
				for k := range rects {
					if sr := &rects[k]; abs32(sr.X+sr.W-brk.X) < 0.01 {
						sr.W += brk.W
						joined = true
						break
					} else if abs32(sr.X-brk.X-brk.W) < 0.01 {
						sr.X, sr.W = brk.X, sr.W+brk.W
						joined = true
						break
					}
				}
				if !joined {
					rects = append(rects, brk)
				}
			}
			for _, sr := range rects {
				l.Fill(geom.Rect{X: origin.X + sh + sr.X, Y: origin.Y + ln.Top, W: sr.W, H: lay.LineHeight}, geom.Radii{}, selCol)
			}
		}
	}
	if aligned {
		paintAlignedGlyphs(l, lay, func(n int) float32 { return origin.X + lineShift(n) }, origin.Y, col)
	} else {
		l.Glyphs(origin, lay.Glyphs, col)
	}
	if comp != "" {
		x0, top, h := caretAt(cur)
		x1, _, _ := caretAt(cur + len([]rune(comp)))
		if x1 < x0 {
			x0, x1 = x1, x0
		}
		l.Fill(geom.Rect{X: origin.X + x0, Y: origin.Y + top + h - 3, W: x1 - x0, H: 1}, geom.Radii{}, col)
	}
	if focused {
		caret := geom.Rect{X: origin.X + cx, Y: origin.Y + ctop + 2, W: 1, H: ch - 4}
		h := e.imeHints
		if password {
			h.Kind, h.NoAutocorrect = input.TextPassword, true
		}
		srs := rs
		if h.Kind == input.TextPassword && !password {
			srs = []rune(strings.Repeat("•", len(rs))) // shown in clear, still secret
		}
		a, b := e.sel()
		c.WantTextInputState(caret, editTextState(srs, a, b, cur, comp, compCur, h))
		phase := c.now.Sub(e.activity) % (1060 * time.Millisecond)
		if phase < 530*time.Millisecond {
			l.Fill(caret, geom.Radii{}, th.Text)
			c.WakeAt(c.now.Add(530*time.Millisecond - phase))
		} else {
			c.WakeAt(c.now.Add(1060*time.Millisecond - phase))
		}
	}
}
