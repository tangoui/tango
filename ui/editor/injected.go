package editor

import (
	"math"
	"strings"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/editor/buffer"
)

// Injected gaps (Monaco's injected text: the `before` content of a model
// decoration, textModel.js LineInjectedText, modelLineProjectionData.js).
//
// Color decorators (colors.go) are one source: Monaco's
// ColorDetector injects a no-break space before each color literal, styled
// as an inline-block 0.8em square with 0.2em margins left and right
// (colorPicker.css), so the literal and the rest of its line move right
// by 1.2em. Here that is an injection: a byte column of the model line
// and a width in logical pixels, colorSwatchW(fontSize) = round(1.2em).
// The injection is not text: offsets, the column grid (State.advance, tab
// stops, vertical goal columns, column selection) and the model are
// unchanged. Only content x moves: a line with injections is shaped
// whole, as any other line, and drawn as pieces of that one layout split
// at the injections (seg.lr), each piece shifted by the gaps before it.
// Everything that measures through the pieces (carets, selections,
// decoration boxes, whitespace marks, bracket boxes, hit testing) follows
// the gap; the few column-grid consumers convert through lineView.gapShift
// and lineView.gridX. Lines without injections are laid out exactly as
// before (one piece, no conversion).
//
// Caret stops (Monaco's InjectedTextCursorStops.Both, the default of the
// color decorator's injection): the literal's start is one offset drawn
// on either side of the gap, never inside it. Monaco's view cursor keeps
// the side: Left arrives on the right side (the injected text's end), Right
// and every other move on the left side, Down on the right side and Up on
// the left side (PositionAffinity.RightOfInjectedText /
// LeftOfInjectedText), and a click on the gap's left half on the left side,
// on its right half on the right side (both the literal's start). State
// keeps that side per caret (headRight), like the row affinity at wrap
// boundaries (headUp).
//
// Selections and decoration ranges take Monaco's range conversion: a
// range starting at the literal starts right of the gap, one ending there
// ends left of it, and a range across the gap covers it.
//
// Word wrap counts each gap at its width in pixels: a row's characters on
// the column grid plus its gaps must fit the wrapping column's pixels
// (Monaco's "simple" strategy counts its one injected character as one
// column instead, so a row there may end up to about a column past the
// wrapping column). A gap is glued to the literal's first character: a
// break before the literal puts the gap on the next row.
//
// Deviations: sticky scroll headers and the minimap show no gaps (the
// sticky headers draw no swatches; Monaco's minimap renders the model
// text); while an IME composition is on a line with gaps, the composition
// is drawn over the line from the caret (as on wrapped lines); up and down
// keep their goal column on the model column grid (Monaco counts the
// injected character as one visible column); the swatch box is centered
// vertically in the row (Monaco aligns its inline block to the baseline).
// Models over Monaco's worker sync limit have no color decorators, so no
// gaps (largefile.go).

// injection is a gap injected before byte column col of a model line, w
// logical pixels wide.
type injection struct {
	col  int     // byte column
	w    float32 // logical pixels
	hint *inlayGlyph
}

// colorSwatchW is the width of a color decorator's gap: Monaco's 0.8em
// box with 0.2em margins on each side.
func colorSwatchW(fontSize float32) float32 { return float32(math.Round(float64(1.2 * fontSize))) }

// injectCfg is how the gaps count where the column grid needs them: w is
// the width in pixels of a color decorator's gap (0: no gaps), colW the
// column width word wrap measures it against. It is part of wrapConfig, so
// a change re-measures the lines.
type injectCfg struct {
	w    float32
	colW float32
}

func (c injectCfg) on() bool { return c.w > 0 && c.colW > 0 }

// cols returns the width of a gap in columns (fractional).
func (c injectCfg) cols() float64 { return float64(c.w) / float64(c.colW) }

// mayHaveColors reports whether s may hold a color literal: a "#" followed
// by a hex digit, or "rgb" / "hsl" (computeColors' literals). Lines
// without any (most code) are rejected without parsing.
func mayHaveColors(s string) bool {
	for i := 0; ; {
		j := strings.IndexByte(s[i:], '#')
		if j < 0 {
			break
		}
		i += j + 1
		if i < len(s) && isHex(s[i]) {
			return true
		}
	}
	return strings.Contains(s, "rgb") || strings.Contains(s, "hsl")
}

// injections appends the gaps of line s to dst (none when off).
func (c injectCfg) injections(s string, dst []injection) []injection {
	if !c.on() || isLong(s) || !mayHaveColors(s) {
		return dst
	}
	var buf [8]colorSpan
	return swatchInjections(computeColors(s, buf[:0]), c.w, dst)
}

// swatchInjections appends a gap of width w before each color literal of
// cs to dst.
func swatchInjections(cs []colorSpan, w float32, dst []injection) []injection {
	for _, c := range cs {
		dst = append(dst, injection{col: c.a, w: w})
	}
	return dst
}

// lineInjections appends the gaps of model line line (text) to dst: the
// color decorators' (lineColors), the only source.
func (w *ViewW) lineInjections(line int, text string, dst []injection) []injection {
	_ = line
	var buf [8]colorSpan
	return swatchInjections(w.lineColors(text, buf[:0]), colorSwatchW(w.opts.fontSize()), dst)
}

// lineGaps returns the color literals of line s (lineColors) and their
// gaps, nil when it has none; during paint's layout of the rows (vs.arena)
// in the frame's scratch, which the next paint reuses.
func (w *ViewW) lineGaps(line int, s string) ([]colorSpan, []injection) {
	vs := w.vs
	sw := colorSwatchW(w.opts.fontSize())
	hints := w.inlayLayout()
	if !vs.arena {
		cs := w.lineColors(s, nil)
		return cs, appendInlays(swatchInjections(cs, sw, nil), 0, hints, line)
	}
	n := len(vs.fColors)
	vs.fColors = w.lineColors(s, vs.fColors)
	cs := vs.fColors[n:len(vs.fColors):len(vs.fColors)]
	m := len(vs.fInj)
	vs.fInj = swatchInjections(cs, sw, vs.fInj)
	vs.fInj = appendInlays(vs.fInj, m, hints, line)
	if len(vs.fInj) == m {
		return cs, nil
	}
	return cs, vs.fInj[m:len(vs.fInj):len(vs.fInj)]
}

// segArena returns the frame's scratch for seg pieces during paint's
// layout of the rows, nil otherwise.
func (w *ViewW) segArena() *[]seg {
	if w.vs.arena {
		return &w.vs.fSegs
	}
	return nil
}

// injectConfig returns the gaps' configuration for word wrap with columns
// colW wide: on when the color decorators are.
func (w *ViewW) injectConfig(colW float32) injectCfg {
	if !w.opts.ColorDecorators || w.st.tooLargeForWorker() || colW <= 0 {
		return injectCfg{}
	}
	return injectCfg{w: colorSwatchW(w.opts.fontSize()), colW: colW}
}

// splitSeg appends to dst the pieces of sg, the layout of a whole row,
// split at the injections in the row (byte columns in [sg.b0, sg.b1) of
// text, the line): the pieces share the layout, each shifted right by the
// gaps before it.
func splitSeg(dst []seg, sg seg, text string, inj []injection) []seg {
	out := dst
	cur := sg
	dx := float32(0)
	for _, in := range inj {
		if in.col < sg.b0 || in.col > sg.b1 {
			continue
		}
		r := cur.r0 + utf8.RuneCountInString(text[cur.b0:in.col])
		left := cur
		left.b1, left.r1, left.piece = in.col, r, true
		out = append(out, left)
		dx += in.w
		cur.b0, cur.r0, cur.lr, cur.piece = in.col, r, sg.lr+r-sg.r0, true
		cur.x0 = sg.x0 + dx
	}
	return append(out, cur)
}

// rowInjections returns the injections of inj in the row [b0, b1).
func rowInjections(inj []injection, b0, b1 int, last bool) []injection {
	i := 0
	for i < len(inj) && inj[i].col < b0 {
		i++
	}
	j := i
	for j < len(inj) && (inj[j].col < b1 || last && inj[j].col == b1) {
		j++
	}
	if i == j {
		return nil
	}
	return inj[i:j]
}

// withGaps splits the single seg of row lv at its injections (lv.inj),
// appending the pieces to *arena (nil: a new array), and recomputes the
// row end.
func (lv *lineView) withGaps(arena *[]seg) {
	if len(lv.inj) == 0 || len(lv.segs) != 1 {
		return
	}
	var dst []seg
	if arena != nil {
		dst = *arena
	}
	n := len(dst)
	dst = splitSeg(dst, lv.segs[0], lv.text, lv.inj)
	lv.segs = dst[n:len(dst):len(dst)]
	if arena != nil {
		*arena = dst
	}
	last := &lv.segs[len(lv.segs)-1]
	lv.endX = max(last.x0+last.lay.Width, last.caret(last.r1))
}

// eachGap calls fn with each gap of row lv, in order, and its content x
// range.
func (lv *lineView) eachGap(fn func(in injection, x0, x1 float32)) {
	if len(lv.inj) == 0 || len(lv.segs) != len(lv.inj)+1 {
		return
	}
	for k, in := range lv.inj {
		x1 := lv.segs[k+1].startX()
		fn(in, x1-in.w, x1)
	}
}

// gapShift returns how far the gaps of row lv push content x x of the
// column grid (a column's left edge) to the right: the gaps at or before
// that column.
func (lv *lineView) gapShift(x, colW float32) float32 {
	sh := float32(0)
	lv.eachGap(func(in injection, x0, x1 float32) {
		if x0-sh <= x+colW/2 {
			sh += in.w
		}
	})
	return sh
}

// gridX converts content x of row lv to the column grid: the gaps before
// it are removed, and x inside a gap is the gap's start.
func (lv *lineView) gridX(x float32) float32 {
	sh := float32(0)
	done := false
	lv.eachGap(func(in injection, x0, x1 float32) {
		switch {
		case done:
		case x >= x1:
			sh += in.w
		case x > x0:
			x, done = x0, true
		default:
			done = true
		}
	})
	return x - sh
}

// caretXSide is caretX on the right side of a gap at rune i (right), else
// on its left side.
func (lv *lineView) caretXSide(i int, right bool) (float32, bool) {
	if !right || len(lv.inj) == 0 {
		return lv.caretX(i)
	}
	for k := len(lv.segs) - 1; k >= 0; k-- {
		if sg := &lv.segs[k]; i >= sg.r0 && i <= sg.r1 {
			return sg.caret(i), true
		}
	}
	return lv.caretX(i)
}

// selRects calls fn with the content x ranges of the bytes [a, b) of the
// line on row lv, as a selection covers them: through the gaps strictly
// inside the range (Monaco's range conversion), not those at its ends,
// except a gap at b when the selection's head is there drawn right of it
// (right == b: Monaco draws the cursor's view selection, which then ends
// after the gap; -1 for none).
func (lv *lineView) selRects(a, b, right int, fn func(x0, x1 float32)) {
	s0, s1 := lv.runeAt(a), lv.runeAt(b)
	for k := range lv.segs {
		sg := &lv.segs[k]
		p0, p1 := max(s0, sg.r0), min(s1, sg.r1)
		if p0 >= p1 {
			continue
		}
		for _, sr := range sg.lay.SelectionRects(sg.local(p0), sg.local(p1)) {
			fn(sg.x0+sr.X, sg.x0+sr.X+sr.W)
		}
	}
	lv.eachGap(func(in injection, x0, x1 float32) {
		if a < in.col && (in.col < b || in.col == b && right == b) {
			fn(x0, x1)
		}
	})
}

// --- seg pieces ---

// local returns the index in sg.lay of rune i of the line.
func (sg *seg) local(i int) int { return i - sg.r0 + sg.lr }

// caret returns the content x of the caret before rune i of the line.
func (sg *seg) caret(i int) float32 {
	x, _, _ := sg.lay.Caret(sg.local(i))
	return sg.x0 + x
}

// startX returns the content x of the seg's first caret.
func (sg *seg) startX() float32 {
	if !sg.piece {
		return sg.x0
	}
	return sg.caret(sg.r0)
}

// --- State: the caret's side of a gap ---

// injectionAt reports whether offset off is the column of a gap (the start
// of a color literal) as of the last paint.
func (s *State) injectionAt(snap buffer.Snapshot, off int) bool {
	cfg := s.vl.cfg
	if !cfg.inj.on() && cfg.hints == nil {
		return false
	}
	off = clampOff(snap, off)
	l := snap.LineOf(off)
	col := off - snap.LineStart(l)
	var buf [8]injection
	if s.inlays == nil || s.inlays.version != snap.Version() {
		cfg.hints = nil
	}
	text := snap.Line(l)
	if isLong(text) && !cfg.on() {
		return false
	}
	for _, in := range cfg.injections(l, text, buf[:0]) {
		if in.col == col {
			return true
		}
	}
	return false
}

// headRight reports whether caret k (of Sels) is drawn on the right side
// of the gap at its offset (see injected.go), as Monaco's view cursor keeps
// it. It holds while that caret stays there in the same text.
func (s *State) headRight(k int) bool {
	u := &s.rights
	return k >= 0 && k < len(u.heads) && k < len(s.Sels) && u.heads[k] >= 0 && u.heads[k] == s.Sels[k].Head &&
		u.ver == s.Snapshot().Version()
}

// caretRight is headRight of the primary caret.
func (s *State) caretRight() bool { return s.headRight(0) }

// setCaretRight sets the primary caret's side of a gap, keeping the
// others'; it is only set at a gap's column.
func (s *State) setCaretRight(on bool) {
	u := &s.rights
	if ver := s.Snapshot().Version(); u.ver != ver || len(u.heads) != len(s.Sels) {
		u.ver = ver
		u.heads = make([]int, len(s.Sels))
		for i := range u.heads {
			u.heads[i] = -1
		}
	}
	if len(u.heads) == 0 {
		return
	}
	u.heads[0] = -1
	if on && s.injectionAt(s.Snapshot(), s.Sels[0].Head) {
		u.heads[0] = s.Sels[0].Head
	}
}

// setRights sets the gap sides after setSels(out): the selections of out
// whose right is true keep it, when their head is at a gap.
//
// It has the final say after a move: without a caret arriving at a gap
// every side is cleared (without allocating).
func (s *State) setRights(out []Selection, right []bool) {
	if !s.vl.cfg.inj.on() && s.vl.cfg.hints == nil {
		s.clearRights()
		return
	}
	snap := s.Snapshot()
	found := false
	for k, sel := range out {
		if right[k] && s.injectionAt(snap, sel.Head) {
			found = true
			break
		}
	}
	if !found {
		s.clearRights() // nothing allocated: no caret at a gap
		return
	}
	m := make(map[Selection]bool, len(out))
	for k, sel := range out {
		if right[k] {
			m[sel] = true
		}
	}
	s.rights = caretAffinity{ver: snap.Version(), heads: make([]int, len(s.Sels))}
	for i, sel := range s.Sels {
		s.rights.heads[i] = -1
		if m[sel] && s.injectionAt(snap, sel.Head) {
			s.rights.heads[i] = sel.Head
		}
	}
}

// clearRights draws every caret on the left side of its gap.
func (s *State) clearRights() {
	if s.rights.heads != nil {
		s.rights = caretAffinity{}
	}
}

// keepRights keeps, after setSels, the gap sides r of the selections old
// (the previous Sels) that are still selections (Monaco keeps the view
// state of the cursors that survive adding or removing others).
func (s *State) keepRights(old []Selection, r caretAffinity) {
	if len(r.heads) != len(old) || r.ver != s.Snapshot().Version() {
		return
	}
	var keep map[Selection]bool
	for k, h := range r.heads {
		if h >= 0 && h == old[k].Head {
			if keep == nil {
				keep = map[Selection]bool{}
			}
			keep[old[k]] = true
		}
	}
	if keep == nil {
		return
	}
	s.rights = caretAffinity{ver: r.ver, heads: make([]int, len(s.Sels))}
	for i, sel := range s.Sels {
		s.rights.heads[i] = -1
		if keep[sel] {
			s.rights.heads[i] = sel.Head
		}
	}
}

// gapGridX converts content x of row [b0, b1) of line s (indented by
// indent columns) to the column grid for the View-less mapping
// (viewLines.ViewToModel): the gaps before x are removed, x inside a gap
// is the gap's start.
func (vl *viewLines) gapGridX(line int, s string, b0, b1, indent int, x float32) float32 {
	cfg := vl.cfg
	if !cfg.on() && isLong(s) {
		cfg.hints = nil
	}
	if !cfg.inj.on() && cfg.hints == nil {
		return x
	}
	var buf [8]injection
	sh := float32(0)
	for _, in := range cfg.injections(line, s, buf[:0]) {
		if in.col < b0 || in.col > b1 || in.col == b1 && b1 < len(s) {
			continue
		}
		gx := float32(indent+lineColumnTab(s[b0:], in.col-b0, max(vl.cfg.tab, 1)))*vl.colW + sh
		if x < gx {
			break
		}
		if x < gx+in.w {
			return gx - sh
		}
		sh += in.w
	}
	return x - sh
}

// gapsBefore returns the width of the gaps of row [b0, b1) of line s
// before byte column col (the caret at a gap's column is drawn on its left
// side, Monaco's default affinity).
func (vl *viewLines) gapsBefore(line int, s string, b0, col int) float32 {
	cfg := vl.cfg
	if !cfg.on() && isLong(s) {
		cfg.hints = nil
	}
	if !cfg.inj.on() && cfg.hints == nil {
		return 0
	}
	var buf [8]injection
	sh := float32(0)
	for _, in := range cfg.injections(line, s, buf[:0]) {
		if in.col >= b0 && in.col < col {
			sh += in.w
		}
	}
	return sh
}
