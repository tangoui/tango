package editor

import (
	"image/color"
	"math"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/highlight"
	"github.com/tangoui/tango/ui/editor/monarch"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

// highlightBudget is how long one frame may tokenize dirty lines above the
// visible ones; the Highlighter's background goroutine does the rest.
const highlightBudget = 3 * time.Millisecond

// Long lines: a line of more than longLineRunes runes is never tokenized
// or shaped whole. It is cut into chunks of about longChunk runes (at
// grapheme cluster boundaries); chunk k starts at content x cols[k]*colW,
// where cols counts columns as vertical caret movement does (tabs to the
// next stop, wide characters two columns, see State.advance). Only chunks
// overlapping the visible x range widened by longMargin columns on each
// side are tokenized (Highlighter.TokenizeSpan from the line's stored start
// state, applied at the first visible chunk: exact when that chunk is the
// first, a display hint like non-final tokens otherwise) and shaped, each on its own
// through the LineCache, so horizontal scrolling reuses the chunks it has
// already shaped. Glyph and caret positions inside a chunk come from its
// Layout; chunk origins come from the column grid, so wide glyphs whose
// advance is not exactly two columns may make a chunk slightly overlap or
// leave a gap before the next one.
const (
	longLineRunes = 2000
	longChunk     = 512
	longMargin    = 64
	longCacheMax  = 32
)

// seg is one shaped piece of a visible line: the whole line, or a chunk of
// a long line.
type seg struct {
	lay    *text.Layout
	x0     float32 // content x of the layout origin
	r0, r1 int     // rune range of the line it covers
	b0, b1 int     // byte range of the line it covers
	paints []runPaint
}

// runPaint is how a style run of a seg is drawn.
type runPaint struct {
	fg                color.NRGBA
	underline, strike bool
	bold, italic      bool // in the run's text.Style; kept for the row fingerprints (damage.go)
	r0, r1            int  // rune range within the seg
}

// lineView is a visible view line (row) ready to draw: a whole model line,
// or one row of a wrapped one.
type lineView struct {
	i      int // model line
	row    int // row in the model line (0 without wrapping)
	rows   int // rows of the model line
	vi     int // view line
	text   string
	runes  int // runes of text (of the row's end on a wrapped line)
	b0, b1 int // byte range of the row in text
	r0, r1 int // rune range of the row
	segs   []seg
	long   *longLine // nil for a normal line
	endX   float32   // content x of the row end
}

// last reports whether lv is its model line's last row.
func (lv *lineView) last() bool { return lv.row >= lv.rows-1 }

type longKey struct {
	buf  *buffer.Buffer
	ver  uint64
	line int
	tab  int // the tab size the columns were counted with
}

// longLine is the chunk table of a long line.
type longLine struct {
	bytes []int // chunk byte starts, then len(text)
	runes []int // chunk rune starts, then the rune count
	cols  []int // chunk column starts, then the total columns
}

func (w *ViewW) paint(l *paint.List, r geom.Rect) {
	st, vs := w.st, w.vs
	if !w.done {
		// The application did not ask Changed: apply the input now, and
		// redraw the rest of the UI with the new text next frame.
		w.finish()
		if w.changed {
			ui.Invalidate(w.c.Window())
		}
	}
	if w.ia.Focused && !w.st.tabFocus && !w.opts.ReadOnly {
		w.c.CaptureTab(w.box.ID()) // see input.go, "Tab"
	}
	th := themeOf(st)
	snap := st.Snapshot()
	n := snap.LineCount()
	lh := w.lineHeight()
	mono := w.monoStyle()
	dg := digitsOf(mono)
	colW, _, _ := text.Shape(" ", mono, text.Options{NoWrap: true}).Caret(1)
	gw := min(w.gutterWidth(dg, n), r.W)
	vs.rect = r
	vs.gutter = rectXYWH(r.X, r.Y, gw, r.H)
	vs.text = rectXYWH(r.X+gw, r.Y, r.W-gw, r.H)
	vs.colW = colW
	if buf := st.Model.Buffer(); vs.buf != buf {
		vs.buf, vs.maxW = buf, 0
		clear(vs.long)
	}

	v := &st.Viewport
	v.LineHeight, v.Height, v.Width = lh, vs.text.H, vs.text.W
	// Word wrap: the wrapping column follows the text area's width; the
	// rows of the lines around the viewport are computed now (viewlines.go).
	wcfg := w.wrapConfig(vs.text.W, colW)
	st.setWrap(wcfg, colW)
	if vs.wrapCol != wcfg.col {
		vs.wrapCol, vs.maxW = wcfg.col, 0
	}
	st.measureViewport()
	vl := st.lines()
	first, last := st.VisibleLines()
	nv := vl.Count()
	lo, hi := max(0, first-2), min(nv-1, last+2)
	mlo, _ := vl.find(lo)
	mhi, _ := vl.find(hi)

	hl := st.Highlighter()
	if hl != nil {
		// Tokenize up to the buffer lines, but never a long line in the
		// frame: the background goroutine handles it.
		upTo := mhi + 1
		for i := mlo; i <= mhi; i++ {
			if isLong(snap.Line(i)) {
				upTo = i
				break
			}
		}
		hl.HighlightFor(st.hlKey, snap, upTo, highlightBudget)
	}

	lines := w.layoutRows(snap, lo, hi, hl, th, colW)
	for k := range lines {
		vs.maxW = max(vs.maxW, lines[k].endX)
	}
	v.ContentWidth = vs.maxW + colW // room for the caret after the widest line
	v.clampX()
	for k := range lines {
		if lines[k].long != nil {
			w.layoutChunks(&lines[k], th, colW)
		}
	}
	// rowOf returns the laid out row showing offset off (up: the end of
	// the earlier row at a row boundary), nil when it is not laid out.
	// From here on paint measures no line (peekRowOf): measuring a line
	// above the top would move TopLine under the rows laid out above.
	rowOf := func(off int, up bool) *lineView {
		if r := st.peekRowOf(off, up); r >= lo && r-lo < len(lines) {
			return &lines[r-lo]
		}
		return nil
	}

	// Colors.
	bg := defaultColor(th, "editor.background")
	focused := w.ia.Focused
	selBg := defaultColor(th, "editor.selectionBackground")
	if !focused {
		selBg = defaultColor(th, "editor.inactiveSelectionBackground")
	}

	tx, ty := vs.text.X-v.ScrollX, vs.text.Y-v.OffsetY
	lineY := func(r int) float32 { return ty + float32(r-v.TopLine)*lh }
	a, b := st.selRange()
	head := clampOff(snap, st.Primary().Head)
	caretLine := snap.LineOf(head)
	caretUp := st.caretUp()
	caretRow := st.peekRowOf(head, caretUp)
	caretLV := rowOf(head, caretUp)
	// The selections touching the laid out lines (the primary first) and
	// whether every selection is a caret.
	vis, allEmpty := visibleSels(snap, st.sels(), snap.LineStart(mlo), lineEnd(snap, mhi))

	// IME composition: shown on the caret line, never in the buffer.
	var cmp compView
	if focused && !w.opts.ReadOnly && vs.comp != "" && caretLV != nil {
		cmp = w.compose(snap, caretLV, a, b, head, vs.comp, vs.compCur, th)
	}

	// 1. Background.
	l.Fill(r, geom.Radii{}, bg)
	l.Clip(vs.text, geom.Radii{})

	// 2. Current line highlight: the lines of the carets, only when no
	// selection is empty (as Monaco). With word wrap the background fills
	// every row of the caret's line and the border (the themes without a
	// background) frames the caret's row only (Monaco's current-line /
	// current-line-exact).
	var cur []bool // rows with the highlight, for the redraw hints
	if allEmpty {
		cur = make([]bool, len(lines))
		fill, isFill := lookupColor(th, "editor.lineHighlightBackground")
		border := defaultColor(th, "editor.lineHighlightBorder")
		for _, vsl := range vis {
			cr := st.peekRowOf(vsl.head, st.headUp(vsl.idx))
			if isFill {
				cl := snap.LineOf(vsl.head)
				r0 := vl.viewOf(cl)
				for r := max(r0, lo); r <= min(r0+vl.rowsOf(cl)-1, hi); r++ {
					cur[r-lo] = true
				}
			} else if cr >= lo && cr <= hi && border.A > 0 {
				cur[cr-lo] = true
			}
		}
		for k := range lines {
			if !cur[k] {
				continue
			}
			row := rectXYWH(vs.text.X, lineY(lines[k].vi), vs.text.W, lh)
			if isFill {
				l.Fill(row, geom.Radii{}, fill)
			} else {
				l.Stroke(row, geom.Radii{}, 2, border)
			}
		}
	}

	// 3. Selections (the primary hidden while composing: the composition
	// replaces it).
	for _, vsl := range vis {
		a, b := vsl.a, vsl.b
		if a == b || (vsl.primary && cmp.on) {
			continue
		}
		for k := range lines {
			lv := &lines[k]
			ls := snap.LineStart(lv.i)
			le := ls + len(lv.text)
			if b < ls || a > le {
				continue
			}
			y := lineY(lv.vi)
			s0, s1 := lv.runeAt(max(a, ls)-ls), lv.runeAt(min(b, le)-ls)
			for _, sg := range lv.segs {
				p0, p1 := max(s0, sg.r0), min(s1, sg.r1)
				if p0 >= p1 {
					continue
				}
				for _, sr := range sg.lay.SelectionRects(p0-sg.r0, p1-sg.r0) {
					l.Fill(rectXYWH(tx+sg.x0+sr.X, y, sr.W, lh), geom.Radii{}, selBg)
				}
			}
			if lv.breakSelected(b, le, n) {
				l.Fill(rectXYWH(tx+lv.endX, y, colW/2, lh), geom.Radii{}, selBg)
			}
		}
	}

	// 4. Text.
	for k := range lines {
		lv := &lines[k]
		y := lineY(lv.vi)
		for _, sg := range lv.segs {
			drawSeg(l, sg, tx, y)
		}
	}

	// 5. Composition and caret.
	if cmp.on {
		w.paintComposition(l, caretLV, cmp, tx, lineY(caretRow), lh, th)
	}
	// Marks drawn over the rows (damage.go): the carets and the drag
	// insertion bar.
	var marks []geom.Rect
	if focused {
		cr, drawn, visible := w.paintCaret(l, snap, caretLV, head, cmp, tx, lineY(caretRow), lh, th)
		if drawn {
			marks = append(marks, cr)
		}
		if visible {
			// The secondary carets blink with the primary one.
			cc := defaultColor(th, "editorCursor.foreground")
			for _, vsl := range vis {
				if vsl.primary {
					continue
				}
				lv := rowOf(vsl.head, st.headUp(vsl.idx))
				if lv == nil {
					continue
				}
				if x, ok := lv.caretX(lv.runeAt(vsl.head - snap.LineStart(lv.i))); ok {
					cx := float32(math.Round(float64(tx + x)))
					cr := rectXYWH(cx, lineY(lv.vi), 2, lh)
					l.Fill(cr, geom.Radii{}, cc)
					marks = append(marks, cr)
				}
			}
		}
	} else {
		vs.blinkFocus = false
	}
	// The insertion point of a text drag (dnd.go).
	if dr, ok := w.paintDnd(l, snap, rowOf, tx, lineY, lh, dndColor(themeType(th))); ok {
		marks = append(marks, dr)
	}
	l.Pop()

	// 6. Gutter, clipped to its area (the numbers of the rows above and
	// below the visible ones stay inside the view).
	gbg := defaultColor(th, "editorGutter.background")
	l.Clip(vs.gutter, geom.Radii{})
	w.paintGutter(l, r.X, gw, lines, lineY, n, caretLine, dg, gbg)
	l.Pop()

	aLine, aRow := vl.find(v.TopLine)
	w.declareDamage(&paintFrame{
		g: damageGlobal{rect: r, gutter: vs.gutter, text: vs.text, colW: colW, lh: lh, theme: th, opts: w.opts,
			tab: w.tabs(), focused: focused, wrap: wcfg, numW: w.numbersWidth(dg, n)},
		snap: snap, lines: lines, vis: vis, cur: cur, cmp: cmp, caretLine: caretLine, scrollX: v.ScrollX, marks: marks,
		aLine: aLine, aRow: aRow,
	})
}

// wrapConfig returns the wrapping in effect for a text area textW wide.
func (w *ViewW) wrapConfig(textW, colW float32) wrapConfig {
	mode := w.opts.wordWrap()
	if t := w.st.wrapToggle; t != "" {
		mode = t // editor.action.toggleWordWrap
	}
	return wrapConfig{
		col:    wrapColumn(mode, w.opts.WordWrapColumn, textW, colW),
		tab:    w.tabs(),
		indent: wrapIndentMode(w.opts.WrappingIndent),
	}
}

// breakSelected reports whether the line break after lv is drawn selected
// for a selection ending at b (the line ends at offset le; the document
// has n lines): only on the line's last row.
func (lv *lineView) breakSelected(b, le, n int) bool {
	return b > le && lv.i+1 < n && lv.last()
}

// visSel is a selection drawn this frame: its ordered range and caret.
type visSel struct {
	a, b, head int
	idx        int // index in the selections
	primary    bool
}

// visibleSels returns the selections touching [from, to] (clamped,
// ordered; the primary first when it is among them) and whether every
// selection is a caret. It is linear in the selections, with no sorting,
// so thousands of cursors cost little per frame.
func visibleSels(snap buffer.Snapshot, sels []Selection, from, to int) ([]visSel, bool) {
	var out []visSel
	allEmpty := true
	for i, x := range sels {
		a, b := clampOff(snap, x.Anchor), clampOff(snap, x.Head)
		h := b
		a, b = min(a, b), max(a, b)
		if a != b {
			allEmpty = false
		}
		if b < from || a > to {
			continue
		}
		out = append(out, visSel{a: a, b: b, head: h, idx: i, primary: i == 0})
	}
	return out, allEmpty
}

// drawSeg draws the glyphs of sg with their run colors and decorations;
// (tx, y) is the screen position of content x 0 and of the line top.
func drawSeg(l *paint.List, sg seg, tx, y float32) {
	lay := sg.lay
	org := geom.Pt(tx+sg.x0, y)
	for g := 0; g < len(lay.Glyphs); {
		run := int32(0)
		if g < len(lay.GlyphRun) {
			run = lay.GlyphRun[g]
		}
		e := g + 1
		for e < len(lay.Glyphs) && e < len(lay.GlyphRun) && lay.GlyphRun[e] == run {
			e++
		}
		fg := color.NRGBA{A: 255}
		if int(run) < len(sg.paints) {
			fg = sg.paints[run].fg
		}
		l.Glyphs(org, lay.Glyphs[g:e], fg)
		g = e
	}
	if len(lay.Lines) == 0 {
		return
	}
	base := lay.Lines[0].Baseline
	for _, p := range sg.paints {
		if !p.underline && !p.strike {
			continue
		}
		x0, _, _ := lay.Caret(p.r0)
		x1, _, _ := lay.Caret(p.r1)
		x0, x1 = min(x0, x1), max(x0, x1)
		if p.underline {
			yy := y + base + 2
			l.Line(geom.Pt(org.X+x0, yy), geom.Pt(org.X+x1, yy), 1, p.fg)
		}
		if p.strike {
			yy := y + base - lay.Ascent*0.3
			l.Line(geom.Pt(org.X+x0, yy), geom.Pt(org.X+x1, yy), 1, p.fg)
		}
	}
}

// runeAt returns the rune index of byte column col of the line; on a row
// of a wrapped line, col is clamped to the row.
func (lv *lineView) runeAt(col int) int {
	col = max(0, min(col, len(lv.text)))
	if lv.long == nil {
		col = max(lv.b0, min(col, lv.b1))
		return lv.r0 + utf8.RuneCountInString(lv.text[lv.b0:col])
	}
	ll := lv.long
	k := sort.SearchInts(ll.bytes, col+1) - 1
	k = max(0, min(k, len(ll.runes)-1))
	return ll.runes[k] + utf8.RuneCountInString(lv.text[ll.bytes[k]:col])
}

// caretX returns the content x of the caret before rune i, false when that
// part of a long line is not shaped (outside the visible range).
func (lv *lineView) caretX(i int) (float32, bool) {
	for _, sg := range lv.segs {
		if i >= sg.r0 && i <= sg.r1 {
			x, _, _ := sg.lay.Caret(i - sg.r0)
			return sg.x0 + x, true
		}
	}
	if len(lv.segs) == 0 && lv.long == nil {
		return 0, true
	}
	return 0, false
}

func isLong(s string) bool {
	return len(s) > longLineRunes && utf8.RuneCountInString(s) > longLineRunes
}

// layoutLine shapes line i (a normal line) or prepares its chunk table (a
// long line; see layoutChunks).
func (w *ViewW) layoutLine(snap buffer.Snapshot, i int, hl *highlight.Highlighter, th *theme.Theme, colW float32) lineView {
	s := snap.Line(i)
	lv := lineView{i: i, vi: i, rows: 1, text: s, b1: len(s)}
	if isLong(s) {
		lv.long = w.longLineOf(snap, i, s)
		lv.runes = lv.long.runes[len(lv.long.runes)-1]
		lv.r1 = lv.runes
		lv.endX = float32(lv.long.cols[len(lv.long.cols)-1]) * colW
		return lv
	}
	lv.runes = utf8.RuneCountInString(s)
	lv.r1 = lv.runes
	var toks []monarch.Token
	if hl != nil {
		// Final or not, the tokens are drawn: non-final tokens are the
		// best guess until the background goroutine catches up.
		w.vs.toks, _ = hl.TokensFor(w.st.hlKey, snap, i, w.vs.toks)
		toks = w.vs.toks
	}
	sg := w.shapeSeg(i, s, 0, lv.runes, 0, toks, th)
	lv.segs = []seg{sg}
	lv.endX = sg.x0 + sg.lay.Width
	if c, _, _ := sg.lay.Caret(lv.runes); sg.x0+c > lv.endX {
		lv.endX = sg.x0 + c // trailing spaces
	}
	return lv
}

// layoutRows lays out view lines lo..hi: whole model lines (layoutLine)
// without word wrap, rows of wrapped lines (layoutWrapped) with it.
func (w *ViewW) layoutRows(snap buffer.Snapshot, lo, hi int, hl *highlight.Highlighter, th *theme.Theme, colW float32) []lineView {
	out := make([]lineView, 0, hi-lo+1)
	vl := w.st.lines()
	if !vl.on() {
		for i := lo; i <= hi; i++ {
			out = append(out, w.layoutLine(snap, i, hl, th, colW))
		}
		return out
	}
	for v := lo; v <= hi; {
		line, row := vl.find(v)
		k := len(out)
		out = w.layoutWrapped(out, snap, line, row, row+hi-v, hl, th, colW)
		if len(out) == k {
			break
		}
		v += len(out) - k
	}
	return out
}

// layoutWrapped appends rows k0..k1 (clamped to the line's rows) of model
// line i to dst, each shaped on its own; continuation rows start at their
// indentation. The tokens are the line's (TokensFor), or for a long line
// those of the span of the rows (TokenizeSpan, as layoutChunks).
func (w *ViewW) layoutWrapped(dst []lineView, snap buffer.Snapshot, i, k0, k1 int, hl *highlight.Highlighter, th *theme.Theme, colW float32) []lineView {
	st := w.st
	s := snap.Line(i)
	lb := st.breaks(i)
	nr := lb.rows()
	k1 = min(k1, nr-1)
	if k0 > k1 {
		return dst
	}
	var toks []monarch.Token
	if hl != nil {
		if isLong(s) {
			sb0, _ := lb.rowRange(k0, len(s))
			_, sb1 := lb.rowRange(k1, len(s))
			w.vs.toks = hl.TokenizeSpan(snap, i, s[sb0:sb1], w.vs.toks[:0])
			toks = w.vs.toks
			for j := range toks {
				toks[j].ByteStart += sb0
			}
		} else {
			w.vs.toks, _ = hl.TokensFor(st.hlKey, snap, i, w.vs.toks)
			toks = w.vs.toks
		}
	}
	v0 := st.vl.viewOf(i)
	for k := k0; k <= k1; k++ {
		b0, b1 := lb.rowRange(k, len(s))
		r0, _ := lb.rowRunes(k, 0)
		r1 := r0 + utf8.RuneCountInString(s[b0:b1])
		sg := w.shapeSeg(i, s[b0:b1], r0, r1, b0, toks, th)
		sg.x0 = float32(lb.rowIndent(k)) * colW
		lv := lineView{i: i, row: k, rows: nr, vi: v0 + k, text: s, runes: r1, b0: b0, b1: b1, r0: r0, r1: r1, segs: []seg{sg}}
		lv.endX = sg.x0 + sg.lay.Width
		if c, _, _ := sg.lay.Caret(r1 - r0); sg.x0+c > lv.endX {
			lv.endX = sg.x0 + c // trailing spaces
		}
		dst = append(dst, lv)
	}
	return dst
}

// longLineOf returns the chunk table of long line i, cached per snapshot.
func (w *ViewW) longLineOf(snap buffer.Snapshot, i int, s string) *longLine {
	k := longKey{w.st.Model.Buffer(), snap.Version(), i, w.st.tabSize()}
	if ll := w.vs.long[k]; ll != nil {
		return ll
	}
	if len(w.vs.long) >= longCacheMax {
		clear(w.vs.long)
	}
	ll := &longLine{bytes: []int{0}, runes: []int{0}, cols: []int{0}}
	col, runes, inChunk := 0, 0, 0
	for p := 0; p < len(s); {
		q := nextGrapheme(s, p)
		col = w.st.advance(col, s[p:q])
		nr := utf8.RuneCountInString(s[p:q])
		runes += nr
		inChunk += nr
		p = q
		if inChunk >= longChunk && p < len(s) {
			ll.bytes = append(ll.bytes, p)
			ll.runes = append(ll.runes, runes)
			ll.cols = append(ll.cols, col)
			inChunk = 0
		}
	}
	ll.bytes = append(ll.bytes, len(s))
	ll.runes = append(ll.runes, runes)
	ll.cols = append(ll.cols, col)
	w.vs.long[k] = ll
	return ll
}

// layoutChunks tokenizes and shapes the chunks of a long line that overlap
// the visible x range.
func (w *ViewW) layoutChunks(lv *lineView, th *theme.Theme, colW float32) {
	v := w.st.Viewport
	ll := lv.long
	x0 := v.ScrollX - longMargin*colW
	x1 := v.ScrollX + v.Width + longMargin*colW
	nc := len(ll.bytes) - 1
	k0, k1 := -1, -1
	for k := range nc {
		cx0, cx1 := float32(ll.cols[k])*colW, float32(ll.cols[k+1])*colW
		if cx1 >= x0 && cx0 <= x1 {
			if k0 < 0 {
				k0 = k
			}
			k1 = k
		}
	}
	if k0 < 0 {
		return
	}
	var toks []monarch.Token
	if hl := w.st.Highlighter(); hl != nil {
		span := lv.text[ll.bytes[k0]:ll.bytes[k1+1]]
		w.vs.toks = hl.TokenizeSpan(w.st.Snapshot(), lv.i, span, w.vs.toks[:0])
		toks = w.vs.toks
		for j := range toks {
			toks[j].ByteStart += ll.bytes[k0]
		}
	}
	for k := k0; k <= k1; k++ {
		b0, b1 := ll.bytes[k], ll.bytes[k+1]
		sg := w.shapeSeg(lv.i, lv.text[b0:b1], ll.runes[k], ll.runes[k+1], b0, toks, th)
		sg.x0 = float32(ll.cols[k]) * colW
		lv.segs = append(lv.segs, sg)
	}
}

// lineOptions are the shaping options of every editor line. As in Monaco
// (VS Code), a line's base direction is always left to right and lines
// start at the left edge; right-to-left runs (Hebrew, Arabic) are reordered
// within the line by the bidi algorithm, so "// שלום x" keeps "//" on the
// left. Left / Right keys still move in logical order (milestone 1).
func (w *ViewW) lineOptions() text.LineOptions {
	return text.LineOptions{TabSize: w.tabs(), Direction: text.DirLTR}
}

// shapeSeg shapes s, the runes [r0,r1) at byte b0 of line i, with the
// token styles of toks (ByteStart relative to the line start).
func (w *ViewW) shapeSeg(i int, s string, r0, r1, b0 int, toks []monarch.Token, th *theme.Theme) seg {
	runs, paints := w.styleRuns(s, b0, toks, th)
	if shapeHook != nil {
		shapeHook(i, s)
	}
	lay := w.vs.lines.Shape(s, runs, w.lineOptions())
	return seg{lay: lay, r0: r0, r1: r1, b0: b0, b1: b0 + len(s), paints: paints}
}

// styleRuns turns the tokens over s (bytes [b0, b0+len(s)) of its line)
// into style runs, merging neighbours with the same theme style.
func (w *ViewW) styleRuns(s string, b0 int, toks []monarch.Token, th *theme.Theme) ([]text.Run, []runPaint) {
	b1 := b0 + len(s)
	def := th.Style("")
	// Boundaries: (byte offset in s, style).
	type bound struct {
		at int
		st theme.Style
	}
	bounds := []bound{{0, def}}
	for k, tk := range toks {
		end := b1
		if k+1 < len(toks) {
			end = toks[k+1].ByteStart
		}
		if end <= b0 || tk.ByteStart >= b1 {
			continue
		}
		at := max(tk.ByteStart, b0) - b0
		ts := th.Style(tk.Type)
		if bounds[len(bounds)-1].at == at {
			bounds[len(bounds)-1].st = ts
		} else {
			bounds = append(bounds, bound{at, ts})
		}
	}
	// Merge equal neighbours.
	m := bounds[:1]
	for _, bd := range bounds[1:] {
		if bd.st != m[len(m)-1].st {
			m = append(m, bd)
		}
	}
	fallback := defaultColor(th, "editor.foreground")
	base := w.monoStyle()
	runs := make([]text.Run, 0, len(m))
	paints := make([]runPaint, 0, len(m))
	rpos, bpos := 0, 0
	for k, bd := range m {
		end := len(s)
		if k+1 < len(m) {
			end = m[k+1].at
		}
		rs := rpos + utf8.RuneCountInString(s[bpos:bd.at])
		re := rs + utf8.RuneCountInString(s[bd.at:end])
		rpos, bpos = re, end
		ts := base
		if bd.st.Bold {
			ts.Weight = text.Bold
		}
		ts.Italic = bd.st.Italic
		fg := bd.st.Foreground
		if fg.A == 0 {
			fg = fallback
		}
		runs = append(runs, text.Run{Start: rs, End: re, Style: ts})
		paints = append(paints, runPaint{fg: fg, underline: bd.st.Underline, strike: bd.st.Strikethrough, bold: bd.st.Bold, italic: bd.st.Italic, r0: rs, r1: re})
	}
	return runs, paints
}

func rectXYWH(x, y, w, h float32) geom.Rect { return geom.Rect{X: x, Y: y, W: max(w, 0), H: max(h, 0)} }

func themeOf(st *State) *theme.Theme {
	if st.Theme != nil {
		return st.Theme
	}
	return theme.Base("vs")
}

// Default colors.
//
// The values are Monaco 0.56's registered defaults (light = vs, dark =
// vs-dark, hcDark = hc-black, hcLight = hc-light) from
// monaco-editor/esm/vs/platform/theme/common/colors/editorColors.js
// (editor.background, editor.foreground, editor.selectionBackground,
// editor.inactiveSelectionBackground = selectionBackground at 50% alpha,
// 70% for high contrast), esm/vs/editor/common/core/editorColorRegistry.js
// (editorLineNumber.foreground, editorLineNumber.activeForeground via
// editorActiveLineNumber.foreground, editor.lineHighlightBorder,
// editorCursor.foreground, editorGutter.background = editor.background) and
// platform/theme/common/colors/baseColors.js (contrastBorder,
// focusBorder/contrastActiveBorder used by the high contrast entries).
// Copyright (c) Microsoft Corporation, MIT License (see
// ui/editor/theme/NOTICE). A theme's own colors table
// (theme.Theme.Color) takes precedence; the base themes of
// esm/vs/editor/standalone/common/themes.js define some keys themselves
// (vs: editor.background #FFFFFE).
//
// editor.lineHighlightBackground has no default in Monaco (null): the
// current line then gets a 2px editor.lineHighlightBorder frame instead.
var monacoDefaults = map[string][4]string{
	//                                       vs           vs-dark      hc-black     hc-light
	"editor.background":                 {"#FFFFFF", "#1E1E1E", "#000000", "#FFFFFF"},
	"editor.foreground":                 {"#333333", "#BBBBBB", "#FFFFFF", "#292929"},
	"editorLineNumber.foreground":       {"#237893", "#858585", "#FFFFFF", "#292929"},
	"editorLineNumber.activeForeground": {"#0B216F", "#C6C6C6", "#F38518", "#006BBD"},
	"editor.lineHighlightBorder":        {"#EEEEEE", "#282828", "#F38518", "#0F4A85"},
	"editor.selectionBackground":        {"#ADD6FF", "#264F78", "#F3F518", "#0F4A85"},
	"editorCursor.foreground":           {"#000000", "#AEAFAD", "#FFFFFF", "#0F4A85"},
}

// lookupColor returns the theme's own value of key.
func lookupColor(th *theme.Theme, key string) (color.NRGBA, bool) {
	if th == nil {
		return color.NRGBA{}, false
	}
	return th.Color(key)
}

// defaultColor returns the theme's value of key, or Monaco's default for
// the theme's type (see monacoDefaults), following Monaco's fallbacks
// between keys.
func defaultColor(th *theme.Theme, key string) color.NRGBA {
	if c, ok := lookupColor(th, key); ok {
		return c
	}
	switch key {
	case "editorGutter.background":
		return defaultColor(th, "editor.background")
	case "editorLineNumber.activeForeground":
		if c, ok := lookupColor(th, "editorActiveLineNumber.foreground"); ok {
			return c
		}
	case "editor.inactiveSelectionBackground":
		c := defaultColor(th, "editor.selectionBackground")
		k := 0.5
		if t := themeType(th); t >= 2 {
			k = 0.7
		}
		c.A = uint8(math.Round(float64(c.A) * k))
		return c
	}
	if v, ok := monacoDefaults[key]; ok {
		return hexColor(v[themeType(th)])
	}
	return color.NRGBA{}
}

// themeType is the column of monacoDefaults for th.
func themeType(th *theme.Theme) int {
	if th == nil {
		return 0
	}
	switch th.BaseName() {
	case "vs-dark":
		return 1
	case "hc-black":
		return 2
	case "hc-light":
		return 3
	}
	return 0
}

// hexColor parses "#RRGGBB" or "#RRGGBBAA" (the table's own literals).
func hexColor(s string) color.NRGBA {
	h := func(i int) uint8 {
		v := func(b byte) uint8 {
			switch {
			case b >= '0' && b <= '9':
				return b - '0'
			case b >= 'a' && b <= 'f':
				return b - 'a' + 10
			case b >= 'A' && b <= 'F':
				return b - 'A' + 10
			}
			return 0
		}
		return v(s[i])<<4 | v(s[i+1])
	}
	c := color.NRGBA{h(1), h(3), h(5), 255}
	if len(s) == 9 {
		c.A = h(7)
	}
	return c
}
