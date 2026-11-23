package editor

import (
	"image/color"
	"math"
	"slices"
	"sort"
	"time"

	"github.com/tangoui/tango/ui/editor/internal/frameclock"
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
	// A piece of a row split at injected gaps (injected.go) shares the
	// row's layout: rune r0 of the line is rune lr of the layout, and only
	// the glyphs of its runes are drawn.
	lr    int
	piece bool
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
	long   *longLine   // nil for a normal line
	endX   float32     // content x of the row end
	colors []colorSpan // the model line's color literals (lineColors)
	inj    []injection // the injected gaps of the row (injected.go)
	// zone is the view zone shown on this row instead of text (zones.go),
	// nil for a text row. A zone row has no text: i is the line the zone
	// follows (the first shown line for the zones before it), row is past
	// its text rows (negative before the first line).
	zone *zoneRow

	damageInk rowInk
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
	w.opts.zoom = ZoomLevel() // a zoom command may have run with the input
	if w.ia.Focused && !w.st.tabFocus && !w.opts.ReadOnly {
		w.c.CaptureTab(w.box.ID()) // see input.go, "Tab"
	}
	th := themeOf(st)
	vs.pruneRunMemo()
	snap := st.Snapshot()
	n := snap.LineCount()
	lh := w.lineHeight()
	mono := w.monoStyle()
	dg := digitsOf(mono)
	colW, _, _ := text.Shape(" ", mono, text.Options{NoWrap: true}).Caret(1)
	gw := min(w.gutterWidth(dg, n), r.W)
	vs.rect = r
	w.navigationGeometry(r, gw)
	vs.colW = colW
	if buf := st.Model.Buffer(); vs.buf != buf {
		vs.buf, vs.maxW = buf, 0
		clear(vs.long)
	}

	v := &st.Viewport
	v.LineHeight, v.Height, v.Width = lh, vs.text.H, vs.text.W
	v.NoScrollBeyondLastLine = w.opts.NoScrollBeyondLastLine
	// Padding: a view scrolled to the very top stays there when the top
	// padding changes (a new State starts at the top).
	if pt, pb := max(w.opts.Padding.Top, 0), max(w.opts.Padding.Bottom, 0); v.PadTop != pt || v.PadBottom != pb {
		atTop := v.TopLine == 0 && v.OffsetY <= -v.PadTop
		v.PadTop, v.PadBottom = pt, pb
		if atTop {
			v.OffsetY = -pt
		}
		st.clampViewport()
	}
	// Word wrap: the wrapping column follows the text area's width; the
	// rows of the lines around the viewport are computed now (viewlines.go).
	wcfg := w.wrapConfig(vs.text.W, colW)
	st.setWrap(wcfg, colW)
	if vs.wrapCol != wcfg.col {
		vs.wrapCol, vs.maxW = wcfg.col, 0
	}
	// Folding: the folding ranges the controls show are computed now,
	// before the rows are laid out (computing them may change the hidden
	// lines; folding.go).
	showAll, showFolded := w.foldControls()
	st.foldPrepare((showAll || w.stickyOn()) && w.foldRangesNow())
	st.measureViewport()
	if v.NoScrollBeyondLastLine {
		st.clampViewport()
	}
	hlDeadline := frameclock.Now().Add(highlightBudget)
	vs.highlightDeadline = hlDeadline
	w.prepareSticky(snap)
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
			if isLong(st.Model.line(snap, i)) {
				upTo = i
				break
			}
		}
		hl.HighlightViewFor(st.hlKey, snap, upTo, max(0, frameclock.Until(hlDeadline)))
	}

	// Spend the remaining token budget before bracket work, shaping and
	// drawing. A slow renderer must not permanently starve miniature tokens.
	w.prepareMinimap(snap, th, hlDeadline)

	// The frame's bracket work shares one budget, spent before layout:
	// first the carets' matched brackets, then the colors of the brackets
	// of the lines about to be laid out (bracketpairs.go).
	brDeadline := frameclock.Now().Add(bracketFrameBudget)
	st.Model.beginBracketFrame()
	var match []brange
	if w.ia.Focused {
		vis0, _ := visibleSels(snap, st.sels(), snap.LineStart(mlo), lineEnd(snap, mhi))
		caretShown := false
		for _, x := range vis0 {
			caretShown = caretShown || x.a == x.b
		}
		match = w.matched(caretShown, brDeadline)
	}
	vs.brk = w.bracketSpans(snap, vl, lo, hi, mhi, th, brDeadline)
	// The sticky headers are laid out with this frame's bracket colors
	// (bracketSpans colors their lines too).
	sticky := w.layoutStickyRows(snap, th)
	vs.bgPairs, vs.bgOn = w.bracketGuidePairs(snap, vl, lo, hi, mhi, clampOff(snap, st.Primary().Head), brDeadline)

	vs.fColors, vs.fInj, vs.fSegs, vs.arena = vs.fColors[:0], vs.fInj[:0], vs.fSegs[:0], true
	lines := w.layoutRows(snap, lo, hi, hl, th, colW)
	vs.arena = false
	fold := w.foldCodes(lines, showAll, showFolded)
	vs.maxW = max(vs.maxW, zoneMinWidth(lines))
	for k := range lines {
		vs.maxW = max(vs.maxW, lines[k].endX)
		if fold[k]&foldPlaceholder != 0 {
			_, x1 := w.placeholderX(&lines[k])
			vs.maxW = max(vs.maxW, x1)
		}
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

	display := w.displayViewport()
	tx, ty := vs.text.X-display.ScrollX, vs.text.Y-display.OffsetY
	lineY := func(r int) float32 { return ty + float32(r-display.TopLine)*lh }
	a, b := st.selRange()
	head := clampOff(snap, st.Primary().Head)
	caretLine := snap.LineOf(head)
	caretUp := st.caretUp()
	caretRow := st.peekRowOf(head, caretUp)
	caretLV := rowOf(head, caretUp)
	// The selections touching the laid out lines (the primary first) and
	// whether every selection is a caret.
	vis, allEmpty := visibleSels(snap, st.sels(), snap.LineStart(mlo), lineEnd(snap, mhi))
	for k := range vis {
		vis[k].right = vis[k].head == vis[k].b && st.headRight(vis[k].idx) // injected.go
	}

	// IME composition: shown on the caret line, never in the buffer.
	var cmp compView
	if focused && !w.opts.ReadOnly && vs.comp != "" && caretLV != nil {
		cmp = w.compose(snap, caretLV, a, b, head, vs.comp, vs.compCur, th)
	}
	boundRows := !vl.mapped() && vs.comp == "" && w.lnFn == nil

	// 1. Background.
	l.Fill(r, geom.Radii{}, bg)
	l.Clip(vs.text, geom.Radii{})

	// 2. Current line highlight: the lines of the carets, in the text area
	// only when every selection is empty, in the gutter always (Monaco's
	// renderLineHighlight). With word wrap the background fills every row
	// of the caret's line and the border (the themes without a background)
	// frames the caret's row only (Monaco's current-line /
	// current-line-exact).
	hlGutter, hlLine := w.opts.lineHighlight()
	highlight := lineHighlightOf(th, focused)
	isFill := highlight.fill.A > 0
	lineHL := func(on bool) []bool {
		if !on {
			return nil
		}
		marks := make([]bool, len(lines))
		for _, vsl := range vis {
			cr := st.peekRowOf(vsl.head, st.headUp(vsl.idx))
			if isFill {
				cl := snap.LineOf(vsl.head)
				r0 := vl.viewOf(cl)
				for r := max(r0, lo); r <= min(r0+vl.rowsOf(cl)-1, hi); r++ {
					marks[r-lo] = true
				}
			} else if cr >= lo && cr <= hi && highlight.border.A > 0 {
				marks[cr-lo] = true
			}
		}
		return marks
	}
	cur := lineHL(hlLine && allEmpty) // rows with the highlight, for the redraw hints
	curG := lineHL(hlGutter)
	var lineBorderW float32
	if !isFill {
		lineBorderW = max(vs.text.W+display.ScrollX, vs.maxW)
	}
	for k := range cur {
		if !cur[k] {
			continue
		}
		row := rectXYWH(vs.text.X, lineY(lines[k].vi), vs.text.W, lh)
		if isFill {
			l.Fill(row, geom.Radii{}, highlight.fill)
		} else {
			// Monaco's border belongs to the scrollable line box.
			row.X -= display.ScrollX
			row.W = lineBorderW
			l.Stroke(row, geom.Radii{}, highlight.width, highlight.border)
		}
	}

	// Folded lines' background (Monaco's folded-background, the whole
	// line).
	if fb := defaultColor(th, "editor.foldBackground"); fb.A > 0 {
		for k := range lines {
			if fold[k]&foldBackground != 0 {
				l.Fill(rectXYWH(vs.text.X, lineY(lines[k].vi), vs.text.W, lh), geom.Radii{}, fb)
			}
		}
	}

	// The decorations of the Model (decorations.go): whole-line
	// backgrounds over the current line, under the selections.
	ud := w.userDecos(snap, lines)
	w.paintWholeLineDecos(l, lines, ud, lineY, lh)

	// 3. Selections (the primary hidden while composing: the composition
	// replaces it).
	for _, vsl := range vis {
		a, b := vsl.a, vsl.b
		if a == b || (vsl.primary && cmp.on) {
			continue
		}
		for k := range lines {
			lv := &lines[k]
			if lv.zone != nil {
				continue
			}
			ls := snap.LineStart(lv.i)
			le := ls + len(lv.text)
			if b < ls || a > le {
				continue
			}
			y := lineY(lv.vi)
			right := -1
			if vsl.right {
				right = b - ls
			}
			lv.selRects(max(a, ls)-ls, min(b, le)-ls, right, func(x0, x1 float32) {
				rect := rectXYWH(tx+x0, y, x1-x0, lh)
				l.Fill(rect, geom.Radii{}, selBg)
				if boundRows {
					lv.damageInk.sel.add(rect.X, rect.Right())
				}
			})
			if lv.breakSelected(b, le, n) {
				rect := rectXYWH(tx+lv.endX, y, colW/2, lh)
				l.Fill(rect, geom.Radii{}, selBg)
				if boundRows {
					lv.damageInk.sel.add(rect.X, rect.Right())
				}
			}
		}
	}

	// The decorations' range backgrounds, over the selections (Monaco's
	// decorations overlay).
	w.paintRangeDecos(l, snap, lines, ud, tx, lineY, lh)

	// Indentation and bracket pair guides (guides.go, bracketguides.go),
	// the matched brackets' boxes and the control characters' backgrounds
	// (whitespace.go).
	guides := w.guidesFor(snap, lines, caretLine)
	vs.bgRows = nil
	if vs.bgOn {
		vs.bgRows = w.rowBracketGuides(lines, vs.bgPairs, colW) // bracketguides.go
	}
	if guides != nil || vs.bgRows != nil {
		maxX := float32(0)
		if wcfg.on() {
			maxX = float32(wcfg.col) * colW
		}
		paintGuides(l, lines, guides, guideColorsOf(th), vs.bgRows, bracketGuideColorsOf(th), tx, colW, lh, max(w.tabs(), 1), maxX, lineY)
	}
	// The decorations' boxes (occurrences, selection matches, suspicious
	// characters: Monaco's decorations overlay, after the guides).
	decos := w.decorations(snap, lines, vis, focused)
	decoSigs := w.paintDecoBoxes(l, lines, decos, focused, tx, lh, lineY)
	matchRows := paintMatch(l, snap, lines, match, tx, lh, lineY, defaultColor(th, "editorBracketMatch.background"), defaultColor(th, "editorBracketMatch.border"))
	if w.opts.RenderControlCharacters {
		for k := range lines {
			if lines[k].zone == nil {
				start := len(l.Ops)
				w.paintControlBackgrounds(l, &lines[k], tx, lineY(lines[k].vi), lh)
				lines[k].damageInk.fallback = len(l.Ops) != start
			}
		}
	}

	// 4. Text, the whitespace marks and the placeholder after folded lines.
	wsColor := defaultColor(th, "editorWhitespace.foreground")
	textStart := len(l.Ops)
	for k := range lines {
		lv := &lines[k]
		if lv.zone != nil {
			continue
		}
		y := lineY(lv.vi)
		lv.damageInk.start = len(l.Ops) - textStart
		vs.udIn = inlineSpans(snap, lv, ud, k, vs.udIn[:0])
		for _, sg := range lv.segs {
			drawSegInline(l, sg, tx, y, vs.udIn)
		}
		if wcfg.hints != nil && len(lv.inj) > 0 {
			w.paintInlays(l, lv, tx, y, lh, th)
		}
		w.paintWhitespace(l, snap, lv, vis, tx, y, wsColor)
		if fold[k]&foldPlaceholder != 0 {
			w.paintPlaceholder(l, lv, tx, y, th)
		}
		lv.damageInk.end = len(l.Ops) - textStart
	}
	var rowOps []paint.Op
	if boundRows {
		rowOps = l.Ops[textStart:]
	}
	// Links' underlines and the color swatches.
	w.paintDecoOver(l, snap, lines, decos, tx, colW, lh, lineY)
	// The view zones' text area parts (zones.go).
	clear(vs.zfp)
	w.paintZones(l, lines, lineY, tx, false)

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
				if x, ok := lv.caretXSide(lv.runeAt(vsl.head-snap.LineStart(lv.i)), st.headRight(vsl.idx)); ok {
					cx := float32(math.Round(float64(tx + x)))
					cr := rectXYWH(cx, lineY(lv.vi), w.cursorWidth(), lh)
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
	w.paintGutter(l, vs.gutter.X, vs.gutter.W, lines, ud, lineY, n, caretLine, dg, gbg, curG, cur, highlight)
	vs.foldX = -1
	if w.foldControlsShown() {
		vs.foldX = w.foldControlX(vs.gutter.X, dg, n)
		paintFoldControls(l, vs.foldX, lines, fold, lineY, lh, defaultColor(th, "editorGutter.foldingControlForeground"), w.foldOpacity)
	}
	w.paintZones(l, lines, lineY, tx, true)
	l.Pop()

	vs.navPaint = vs.navPaint[:0]
	w.paintSticky(l, sticky, th, display.ScrollX)
	w.paintMinimap(l, th)
	w.paintOverview(l, th)

	aLine, aRow := vl.find(v.TopLine)
	w.declareDamage(l, &paintFrame{
		g: damageGlobal{rect: r, gutter: vs.gutter, text: vs.text, colW: colW, lh: lh, theme: th, opts: w.opts,
			tab: w.tabs(), focused: focused, wrap: wcfg, numW: w.numbersWidth(dg, n), foldGen: st.fold.gen, hidGen: st.hidGen, zoneGen: st.zgen, zoneOpaque: vs.zoneOpaque,
			lnFn: w.lnFn != nil, lnGen: w.lnGen},
		snap: snap, lines: lines, vis: vis, cur: cur, curG: curG, cmp: cmp, caretLine: caretLine, offsetY: display.OffsetY, scrollX: display.ScrollX, marks: marks,
		aLine: aLine, aRow: aRow, fold: fold, guides: guides, bguides: vs.bgRows, match: matchRows, decos: decoSigs, udecos: ud,
		lineBorderW: lineBorderW,
		boundRows:   boundRows, rowOps: rowOps,
	})
	st.checkScroll() // what paint itself scrolled (scroll.go)
}

type lineHighlightStyle struct {
	fill, border color.NRGBA
	width        float32
}

func lineHighlightOf(th *theme.Theme, focused bool) lineHighlightStyle {
	active := defaultColor(th, "editor.lineHighlightBackground")
	fill := active
	if !focused {
		fill = defaultColor(th, "editor.inactiveLineHighlightBackground")
	}
	var border color.NRGBA
	if active.A == 0 {
		border = defaultColor(th, "editor.lineHighlightBorder")
	}
	width := float32(2)
	if themeType(th) >= 2 {
		width = 1
	}
	return lineHighlightStyle{fill, border, width}
}

// wrapConfig returns the wrapping in effect for a text area textW wide.
func (w *ViewW) wrapConfig(textW, colW float32) wrapConfig {
	mode := w.opts.wordWrap()
	if t := w.st.wrapToggle; t != "" {
		mode = t // editor.action.toggleWordWrap
	}
	if w.st.LargeFile() && !w.st.wrapLarge {
		mode = WrapOff // Monaco shows large files unwrapped (largefile.go)
	}
	cfg := wrapConfig{
		col:    wrapColumn(mode, w.opts.WordWrapColumn, textW, colW),
		tab:    w.tabs(),
		indent: wrapIndentMode(w.opts.WrappingIndent),
		inj:    w.injectConfig(colW),
		hints:  w.inlayLayout(),
	}
	if cfg.hints != nil {
		cfg.inj.colW = colW
	}
	return cfg
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
	idx        int  // index in the selections
	right      bool // the head is b, drawn right of a gap (injected.go)
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
	lo, hi := sg.lr, sg.lr+sg.r1-sg.r0 // the layout runes drawn (a piece's)
	in := func(g int) bool {
		if !sg.piece || g >= len(lay.GlyphCluster) {
			return true
		}
		c := int(lay.GlyphCluster[g])
		return c >= lo && c < hi
	}
	for g := 0; g < len(lay.Glyphs); {
		if !in(g) {
			g++
			continue
		}
		run := int32(0)
		if g < len(lay.GlyphRun) {
			run = lay.GlyphRun[g]
		}
		e := g + 1
		for e < len(lay.Glyphs) && e < len(lay.GlyphRun) && lay.GlyphRun[e] == run && in(e) {
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
		r0, r1 := p.r0, p.r1
		if sg.piece {
			r0, r1 = max(r0, lo), min(r1, hi)
			if r0 >= r1 {
				continue
			}
		}
		x0, _, _ := lay.Caret(r0)
		x1, _, _ := lay.Caret(r1)
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
	for k := range lv.segs {
		if sg := &lv.segs[k]; i >= sg.r0 && i <= sg.r1 {
			return sg.caret(i), true
		}
	}
	if len(lv.segs) == 0 && lv.long == nil {
		return 0, true
	}
	return 0, false
}

// isLong reports whether s has more than longLineRunes runes. It counts at
// most utf8.UTFMax*longLineRunes bytes: a longer string has more runes
// than that whatever its content, so the test costs the same on a 1 MB
// line as on an 8 KB one.
func isLong(s string) bool {
	if len(s) <= longLineRunes {
		return false
	}
	if len(s) > utf8.UTFMax*longLineRunes {
		return true
	}
	countAdd(&workRuneBytes, len(s))
	return utf8.RuneCountInString(s) > longLineRunes
}

// layoutLine shapes line i (a normal line) or prepares its chunk table (a
// long line; see layoutChunks).
func (w *ViewW) layoutLine(snap buffer.Snapshot, i int, hl *highlight.Highlighter, th *theme.Theme, colW float32) lineView {
	s := w.st.Model.line(snap, i)
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
	if lv.colors, lv.inj = w.lineGaps(i, s); lv.inj != nil {
		lv.withGaps(w.segArena())
	}
	return lv
}

// layoutRows lays out view lines lo..hi: whole model lines (layoutLine)
// without word wrap, rows of wrapped lines (layoutWrapped) with it.
func (w *ViewW) layoutRows(snap buffer.Snapshot, lo, hi int, hl *highlight.Highlighter, th *theme.Theme, colW float32) []lineView {
	out := make([]lineView, 0, hi-lo+1)
	vl := w.st.lines()
	if !vl.on() {
		// One row per shown line (folded regions hide some), and the rows
		// of the view zones.
		for v := lo; v <= hi; v++ {
			i, row := vl.find(v)
			if row != 0 {
				if lv, ok := w.zoneView(i, row, v); ok {
					out = append(out, lv)
					continue
				}
			}
			lv := w.layoutLine(snap, i, hl, th, colW)
			lv.vi = v
			out = append(out, lv)
		}
		return out
	}
	for v := lo; v <= hi; {
		line, row := vl.find(v)
		if row < 0 || row >= vl.rowsOf(line) {
			if lv, ok := w.zoneView(line, row, v); ok {
				out = append(out, lv)
				v++
				continue
			}
		}
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
	s := st.Model.line(snap, i)
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
	colors, inj := w.lineGaps(i, s)
	for k := k0; k <= k1; k++ {
		b0, b1 := lb.rowRange(k, len(s))
		r0, _ := lb.rowRunes(k, 0)
		r1 := r0 + utf8.RuneCountInString(s[b0:b1])
		sg := w.shapeSeg(i, s[b0:b1], r0, r1, b0, toks, th)
		sg.x0 = float32(lb.rowIndent(k)) * colW
		lv := lineView{i: i, row: k, rows: nr, vi: v0 + k, text: s, runes: r1, b0: b0, b1: b1, r0: r0, r1: r1, segs: []seg{sg}, colors: colors}
		lv.endX = sg.x0 + sg.lay.Width
		if c, _, _ := sg.lay.Caret(r1 - r0); sg.x0+c > lv.endX {
			lv.endX = sg.x0 + c // trailing spaces
		}
		if lv.inj = rowInjections(inj, b0, b1, k == nr-1); lv.inj != nil {
			lv.withGaps(w.segArena())
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
	ll := lv.long
	k0, k1 := w.chunkWindow(ll, colW)
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

// chunkWindow returns the chunks k0..k1 of long line ll that layoutChunks
// shapes at the viewport's horizontal scroll (k0 < 0: none).
func (w *ViewW) chunkWindow(ll *longLine, colW float32) (k0, k1 int) {
	v := w.st.Viewport
	x0 := v.ScrollX - longMargin*colW
	x1 := v.ScrollX + v.Width + longMargin*colW
	k0, k1 = -1, -1
	for k := range len(ll.bytes) - 1 {
		cx0, cx1 := float32(ll.cols[k])*colW, float32(ll.cols[k+1])*colW
		if cx1 >= x0 && cx0 <= x1 {
			if k0 < 0 {
				k0 = k
			}
			k1 = k
		}
	}
	return k0, k1
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
	// Colored brackets and control characters override the token colors.
	over := w.vs.brk[i]
	shaped := s
	if w.opts.RenderControlCharacters && hasControl(s) {
		over = mergeSpans(over, controlSpans(s, b0, nil))
		shaped = controlPictures(s)
	}
	if sp, ok := w.activeLinkSpan(i); ok {
		over = withSpan(over, sp) // the link under the pointer (links.go)
	}
	runs, paints := w.memoRuns(i, s, b0, toks, th, over)
	if shapeHook != nil {
		shapeHook(i, s)
	}
	lay := w.vs.lines.Shape(shaped, runs, w.lineOptions())
	return seg{lay: lay, r0: r0, r1: r1, b0: b0, b1: b0 + len(s), paints: paints}
}

// runMemo is a row's style runs (styleRuns) and the inputs they were made
// from, kept across frames: a row whose text, tokens, color spans, theme
// and text style did not change takes them instead of walking its tokens
// again.
type runMemo struct {
	s      string
	b0     int
	toks   []monarch.Token
	over   []brSpan
	th     *theme.Theme
	base   text.Style
	runs   []text.Run
	paints []runPaint // read only (segs share them)
	used   uint64     // the paint that last used it (viewState.runGen)
}

// runKey identifies a row's memo: its line and first byte.
type runKey struct{ i, b0 int }

// memoRuns returns styleRuns(s, b0, toks, th, over) for the row of line i
// at byte b0, from the row's memo when its inputs are unchanged. The runs
// are valid until the next call on this view; the paints are shared with
// the memo (never written).
func (w *ViewW) memoRuns(i int, s string, b0 int, toks []monarch.Token, th *theme.Theme, over []brSpan) ([]text.Run, []runPaint) {
	vs := w.vs
	base := w.monoStyle()
	k := runKey{i, b0}
	if e := vs.runMemo[k]; e != nil && e.s == s && e.b0 == b0 && e.th == th && e.base == base &&
		slices.Equal(e.toks, toks) && slices.Equal(e.over, over) {
		e.used = vs.runGen
		return e.runs, e.paints
	}
	runs, paints := w.styleRuns(s, b0, toks, th, over)
	vs.runsBuilt++
	if vs.runMemo == nil {
		vs.runMemo = map[runKey]*runMemo{}
	}
	e := vs.runMemo[k]
	if e == nil {
		e = &runMemo{}
		vs.runMemo[k] = e
	}
	*e = runMemo{s: s, b0: b0, toks: append(e.toks[:0], toks...), over: append(e.over[:0], over...), th: th, base: base,
		runs: append(e.runs[:0], runs...), paints: paints, used: vs.runGen}
	return e.runs, e.paints
}

// pruneRunMemo starts a paint for the row memos (memoRuns), dropping the
// rows the last two paints did not lay out.
func (vs *viewState) pruneRunMemo() {
	vs.runGen++
	if len(vs.runMemo) <= runMemoKeep {
		return
	}
	for k, e := range vs.runMemo {
		if vs.runGen-e.used > 2 {
			delete(vs.runMemo, k)
		}
	}
}

// runMemoKeep is how many row memos a view keeps before pruning the ones
// the last paints did not use: a few screens of rows.
const runMemoKeep = 256

// styleRuns turns the tokens over s (bytes [b0, b0+len(s)) of its line)
// into style runs, merging neighbours with the same theme style.
// Runs are valid until the next call on this view; paints are independently owned.
func (w *ViewW) styleRuns(s string, b0 int, toks []monarch.Token, th *theme.Theme, over []brSpan) ([]text.Run, []runPaint) {
	b1 := b0 + len(s)
	def := th.Style("")
	// Boundaries: (byte offset in s, style), in the view's scratch (kept
	// for the next seg: every visible row runs this every frame).
	bounds := append(w.vs.bounds[:0], styleBound{0, def})
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
			bounds = append(bounds, styleBound{at, ts})
		}
	}
	w.vs.bounds = bounds[:0]
	// Spans of other colors (colored brackets, control characters): cut
	// the runs at their ends and take their color inside.
	if len(over) > 0 {
		bounds = overrideBoundsInto(w.vs.overrideScratch, bounds, over, b0, len(s), func(st theme.Style, c color.NRGBA) theme.Style {
			st.Foreground = c
			return st
		})
		w.vs.overrideScratch = bounds[:0]
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
	runs := w.vs.shapeRuns[:0]
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
	w.vs.shapeRuns = runs[:0]
	return runs, paints
}

// styleBound is where a style starts in a seg's text.
type styleBound struct {
	at int
	st theme.Style
}

// overrideBounds cuts the style boundaries of a text of n bytes at byte b0
// of its line at the ends of spans (line bytes, sorted, not overlapping)
// and gives the bytes inside them set(style, span color).
func overrideBounds(bounds []styleBound, over []brSpan, b0, n int, set func(theme.Style, color.NRGBA) theme.Style) []styleBound {
	return overrideBoundsInto(nil, bounds, over, b0, n, set)
}

// dst must not share storage with bounds; the output is temporary until
// the next use of dst.
func overrideBoundsInto(dst, bounds []styleBound, over []brSpan, b0, n int, set func(theme.Style, color.NRGBA) theme.Style) []styleBound {
	// Only the spans that reach into the text count: the spans of a long
	// line's other rows are skipped by a binary search, not walked (and
	// not allocated for), once per row.
	lo := sort.Search(len(over), func(k int) bool { return over[k].b1 > b0 })
	hi := lo
	for hi < len(over) && over[hi].b0 < b0+n {
		hi++
	}
	over = over[lo:hi]
	countAdd(&workOverSpans, len(over))
	need := len(bounds) + 2*len(over)
	out := dst[:0]
	if cap(out) < need {
		out = make([]styleBound, 0, need)
	}
	push := func(b styleBound) {
		if n := len(out); n > 0 && out[n-1].at == b.at {
			out[n-1] = b
			return
		}
		out = append(out, b)
	}
	k := 0
	cur := bounds[0].st // the style in effect at the last cut
	for _, sp := range over {
		a, z := max(sp.b0, b0)-b0, min(sp.b1, b0+n)-b0
		if a >= z {
			continue
		}
		for ; k < len(bounds) && bounds[k].at <= a; k++ {
			if bounds[k].at < a {
				push(bounds[k])
			}
			cur = bounds[k].st
		}
		push(styleBound{a, set(cur, sp.c)})
		for ; k < len(bounds) && bounds[k].at <= z; k++ {
			cur = bounds[k].st
		}
		if z < n {
			push(styleBound{z, cur})
		}
	}
	for _, b := range bounds[k:] {
		push(b)
	}
	return out
}

// mergeSpans merges two sorted span lists.
func mergeSpans(a, b []brSpan) []brSpan {
	if len(a) == 0 {
		return b
	}
	out := make([]brSpan, 0, len(a)+len(b))
	for len(a) > 0 && len(b) > 0 {
		if a[0].b0 <= b[0].b0 {
			out, a = append(out, a[0]), a[1:]
		} else {
			out, b = append(out, b[0]), b[1:]
		}
	}
	return append(append(out, a...), b...)
}

func rectXYWH(x, y, w, h float32) geom.Rect { return geom.Rect{X: x, Y: y, W: max(w, 0), H: max(h, 0)} }

func themeOf(st *State) *theme.Theme {
	if st.Theme != nil {
		return st.Theme
	}
	return theme.Base("vs")
}

// lookupColor returns the theme's color for key (theme.Theme.Color: its
// own value, else Monaco 0.56's registered default for its base type).
func lookupColor(th *theme.Theme, key string) (color.NRGBA, bool) {
	if th == nil {
		return color.NRGBA{}, false
	}
	return th.Color(key)
}

// defaultColor returns the theme's color for key, transparent when Monaco
// has none there; keys Monaco has no default for, but the View draws
// anyway, fall back as Monaco's CSS does.
func defaultColor(th *theme.Theme, key string) color.NRGBA {
	if c, ok := lookupColor(th, key); ok {
		return c
	}
	switch key {
	case "editor.foldPlaceholderForeground":
		// No default in high contrast: the placeholder takes the text color.
		return defaultColor(th, "editor.foreground")
	}
	return color.NRGBA{}
}

// themeType is th's base theme type: 0 vs, 1 vs-dark, 2 hc-black, 3
// hc-light.
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

// foldControls returns which folding controls the gutter shows this frame:
// all of them (showFoldingControls "always", or "mouseover" with the
// pointer over the gutter), and those of folded regions (any mode but
// "never").
func (w *ViewW) foldControls() (all, folded bool) {
	if !w.foldControlsShown() || !w.st.foldingOn() {
		return false, false
	}
	switch w.opts.foldingControls() {
	case FoldingControlsAlways:
		return true, true
	}
	p, in := w.c.Pointer()
	hovered := in && w.ia.Hovered && w.vs.gutter.Contains(p)
	return hovered || (w.opts.AnimateFoldingControls && w.foldOpacity > 0), true
}

// Folding ranges for the gutter's controls are computed at once in a
// document of up to foldSyncLines lines; in a larger one, foldDebounce
// after the last change (Monaco's folding update delay, at least 200 ms),
// so typing does not recompute them on every key.
const (
	foldSyncLines = 20000
	foldDebounce  = 200 * time.Millisecond
)

// foldRangesNow reports whether paint computes the folding ranges for the
// controls this frame (see foldSyncLines); otherwise it wakes the view
// when they are due.
func (w *ViewW) foldRangesNow() bool {
	st, vs := w.st, w.vs
	snap := st.Snapshot()
	if st.foldRangesFresh() || snap.LineCount() <= foldSyncLines {
		return true
	}
	now := w.c.Now()
	if k := st.foldRangesKey(); vs.foldStale != k {
		vs.foldStale, vs.foldDue = k, now.Add(foldDebounce)
	}
	if now.Before(vs.foldDue) {
		w.c.WakeAt(vs.foldDue)
		return false
	}
	return true
}

// foldCodes returns the folding decorations of the laid out rows (see
// foldCtlExpanded). It reads the folding state only: paint calls it after
// laying out the rows.
func (w *ViewW) foldCodes(rows []lineView, all, folded bool) []uint8 {
	st := w.st
	fs := &st.fold
	codes := make([]uint8, len(rows))
	if len(fs.folds) == 0 && !all {
		return codes
	}
	// The ranges of the current text, or while they wait for the debounce
	// (foldRangesNow) those of the last text when it had as many lines.
	ranges := all && fs.rsOK && fs.rsKey.buf == st.Model.Buffer() &&
		(fs.rsKey.ver == st.Snapshot().Version() || fs.rsLines == st.Snapshot().LineCount())
	highlight := w.opts.FoldingHighlight
	for k := range rows {
		lv := &rows[k]
		if lv.zone != nil {
			continue
		}
		isFolded := fs.foldAt(lv.i) >= 0
		var c uint8
		if lv.row == 0 {
			switch {
			case isFolded && folded:
				c |= foldCtlCollapsed
			case !isFolded && ranges && fs.rangeAt(lv.i):
				c |= foldCtlExpanded
			}
		}
		if isFolded {
			if lv.last() {
				c |= foldPlaceholder
			}
			if highlight {
				c |= foldBackground
			}
		}
		codes[k] = c
	}
	return codes
}

// foldEllipsis is Monaco's placeholder after a folded line (U+22EF, the
// inline-folded ::after content).
const foldEllipsis = "⋯"

// placeholderX returns the content x range of the placeholder after row
// lv (Monaco's margin: 0.2em on each side).
func (w *ViewW) placeholderX(lv *lineView) (x0, x1 float32) {
	lay := w.ellipsis()
	em := w.opts.fontSize()
	x0 = lv.endX + 0.2*em
	return x0, x0 + lay.Width + 0.2*em
}

// ellipsis returns the shaped placeholder, cached per font style.
func (w *ViewW) ellipsis() *text.Layout {
	vs := w.vs
	if style := w.monoStyle(); vs.ellipsis == nil || vs.ellipsisFont != style {
		vs.ellipsis = text.Shape(foldEllipsis, style, text.Options{NoWrap: true})
		vs.ellipsisFont = style
	}
	return vs.ellipsis
}

// paintPlaceholder draws the placeholder after row lv, a folded line's
// last row at y; tx is the screen x of content x 0.
func (w *ViewW) paintPlaceholder(l *paint.List, lv *lineView, tx, y float32, th *theme.Theme) {
	x0, _ := w.placeholderX(lv)
	lay := w.ellipsis()
	l.Glyphs(geom.Pt(tx+x0, y), lay.Glyphs, defaultColor(th, "editor.foldPlaceholderForeground"))
}

// Display decorations of Task 13 (occurrences.go, unicode.go, links.go,
// colors.go): ranges of a row's model line, computed from the text of the
// laid out rows only, after layout (they never change the layout).

// decoKind is the kind of a decoration.
type decoKind uint8

const (
	decoOcc     decoKind = iota + 1 // occurrence of the word at the caret
	decoSelHL                       // match of the selection
	decoUnicode                     // suspicious character
	decoLink                        // link (underlined)
	decoSwatch                      // color literal (swatch before it)
)

// deco is a decoration of a model line: its kind and line bytes.
type deco struct {
	kind decoKind
	a, b int
	c    color.NRGBA // the swatch color
	url  string      // the link
}

// decorFrame is what the decorations of a frame depend on.
type decorFrame struct {
	occ, sel []brange // cached, bounded document matches (absolute offsets)
	uo       unicodeOpts
}

// maxUnicodeMarks is Monaco's MAX_RESULT_LENGTH of the unicode
// highlighter: with more marks than this none are drawn (Monaco shows a
// banner instead).
const maxUnicodeMarks = 1000

// decorations returns the decorations of each laid out row (nil when there
// are none at all).
func (w *ViewW) decorations(snap buffer.Snapshot, lines []lineView, vis []visSel, focused bool) [][]deco {
	word := w.occurrenceWord(snap, focused)
	var oq *selQuery
	if word != "" {
		oq = &selQuery{text: word, whole: true}
	}
	// The document searches scan on for one frame's budget; the laid out
	// lines are matched at once (occurrences.go).
	vs, buf := w.vs, w.st.Model.Buffer()
	clock := vs.scanNow
	if clock == nil {
		clock = scanClock
	}
	vs.decoLines = vs.decoLines[:0]
	for k := range lines {
		if n := len(vs.decoLines); n == 0 || lines[k].i > vs.decoLines[n-1] {
			vs.decoLines = append(vs.decoLines, lines[k].i)
		}
	}
	vs.occMatches.now, vs.selMatches.now = clock, clock
	deadline := clock().Add(docScanBudget)
	occ, occDone := vs.occMatches.find(buf, snap, oq, deadline, vs.decoLines)
	sel, selDone := vs.selMatches.find(buf, snap, w.selectionQuery(snap), deadline, vs.decoLines)
	if !occDone || !selDone {
		// The rows declare what the scan changes (damage.go). A scan frame
		// that changes nothing on screen declares nothing: the renderer
		// redraws it whole (ui.Context.Damage).
		w.c.WakeAtDamaged(w.c.Now())
	}
	df := &decorFrame{occ: occ, sel: sel, uo: w.opts.unicodeOpts()}
	var out [][]deco
	var cur []deco
	curLine := -1
	w.vs.decoCount = [decoSwatch + 1]int{}
	for k := range lines {
		lv := &lines[k]
		if lv.zone != nil {
			continue
		}
		if lv.i != curLine {
			curLine = lv.i
			cur = w.lineDecos(snap, lv.i, lv.text, lv.colors, vis, df)
			for _, d := range cur {
				w.vs.decoCount[d.kind]++
			}
		}
		if len(cur) == 0 {
			continue
		}
		if out == nil {
			out = make([][]deco, len(lines))
		}
		out[k] = cur
	}
	if w.vs.decoCount[decoUnicode] > maxUnicodeMarks {
		// Too many suspicious characters: none are marked.
		w.vs.decoCount[decoUnicode] = 0
		for k, ds := range out {
			if k > 0 && lines[k].i == lines[k-1].i {
				out[k] = out[k-1] // the rows of a line share its decorations
				continue
			}
			out[k] = slices.DeleteFunc(ds, func(d deco) bool { return d.kind == decoUnicode })
		}
	}
	return out
}

// lineDecos returns the decorations of model line i (text, whose color
// literals are colors: lineColors, computed when the line was laid out),
// sorted by kind.
func (w *ViewW) lineDecos(snap buffer.Snapshot, i int, text string, colors []colorSpan, vis []visSel, df *decorFrame) []deco {
	if isLong(text) {
		return nil
	}
	var ds []deco
	vs := w.vs
	ls := snap.LineStart(i)
	if len(df.occ) > 0 {
		vs.decoRanges = lineMatches(df.occ, ls, ls+len(text), vs.decoRanges[:0])
		for _, m := range vs.decoRanges {
			ds = append(ds, deco{kind: decoOcc, a: m.a, b: m.b})
		}
	}
	if len(df.sel) > 0 {
		vs.decoRanges = dropSelected(lineMatches(df.sel, ls, ls+len(text), vs.decoRanges[:0]), ls, vis)
		for _, m := range vs.decoRanges {
			ds = append(ds, deco{kind: decoSelHL, a: m.a, b: m.b})
		}
	}
	if df.uo.any() && mayNeedUnicode(text) {
		var tokType func(int) string
		if hl := w.st.Highlighter(); hl != nil && (!df.uo.inComments || !df.uo.inStrings) {
			// The tokens are fetched for the first marked candidate only:
			// lines of plain CJK text have none.
			fetched := false
			tokType = func(at int) string {
				if !fetched {
					vs.decoToks, _ = hl.TokensFor(w.st.hlKey, snap, i, vs.decoToks)
					fetched = true
				}
				t := ""
				for _, tk := range vs.decoToks {
					if tk.ByteStart > at {
						break
					}
					t = tk.Type
				}
				return stdTypeCached(t)
			}
		}
		vs.decoRanges = unicodeMarks(text, df.uo, tokType, vs.decoRanges[:0])
		for _, m := range vs.decoRanges {
			ds = append(ds, deco{kind: decoUnicode, a: m.a, b: m.b})
		}
	}
	vs.decoLinks = w.lineLinks(i, text, vs.decoLinks[:0])
	for _, lk := range vs.decoLinks {
		ds = append(ds, deco{kind: decoLink, a: lk.a, b: lk.b, url: lk.url})
	}
	for _, cs := range colors {
		ds = append(ds, deco{kind: decoSwatch, a: cs.a, b: cs.b, c: cs.c})
	}
	return ds
}

// decoRects calls fn with the content x ranges of the bytes [a, b) of row
// lv (a character without width gets an empty range at its place; an
// injected gap strictly inside the range is covered: injected.go).
func decoRects(lv *lineView, a, b int, fn func(x0, x1 float32)) {
	if a >= lv.b1 || b <= lv.b0 {
		return
	}
	s0, s1 := lv.runeAt(a), lv.runeAt(b)
	for k := range lv.segs {
		sg := &lv.segs[k]
		p0, p1 := max(s0, sg.r0), min(s1, sg.r1)
		if p0 >= p1 {
			continue
		}
		rs := sg.lay.SelectionRects(sg.local(p0), sg.local(p1))
		if len(rs) == 0 {
			x := sg.caret(p0)
			fn(x, x)
			continue
		}
		for _, sr := range rs {
			fn(sg.x0+sr.X, sg.x0+sr.X+sr.W)
		}
	}
	lv.eachGap(func(in injection, x0, x1 float32) {
		if a < in.col && in.col < b {
			fn(x0, x1)
		}
	})
}

// decoStroke draws a one pixel border inside box.
func decoStroke(l *paint.List, box geom.Rect, c color.NRGBA) {
	if c.A == 0 {
		return
	}
	l.Stroke(geom.Rect{X: box.X + 0.5, Y: box.Y + 0.5, W: box.W - 1, H: box.H - 1}, geom.Radii{}, 1, c)
}

// paintDecoBoxes draws the boxes of the decorations under the text
// (occurrences, selection matches, suspicious characters) and returns the
// fingerprint of each row's decorations (damage.go).
func (w *ViewW) paintDecoBoxes(l *paint.List, lines []lineView, decos [][]deco, focused bool, tx, lh float32, lineY func(int) float32) []uint64 {
	if decos == nil {
		return nil
	}
	th := themeOf(w.st)
	occBg, occBd := defaultColor(th, "editor.wordHighlightTextBackground"), defaultColor(th, "editor.wordHighlightTextBorder")
	selBg, selBd := defaultColor(th, "editor.selectionHighlightBackground"), defaultColor(th, "editor.selectionHighlightBorder")
	if !focused {
		selBg.A, selBd = uint8(math.Round(float64(selBg.A)*0.5)), color.NRGBA{}
	}
	uniBg, uniBd := defaultColor(th, "editorUnicodeHighlight.background"), defaultColor(th, "editorUnicodeHighlight.border")
	sigs := make([]uint64, len(lines))
	for k, ds := range decos {
		if len(ds) == 0 {
			// 0, as when the frame has no decorations at all: the first
			// decoration to appear (or the last to go) damages its rows only.
			continue
		}
		lv := &lines[k]
		y := lineY(lv.vi)
		h := uint64(14695981039346656037)
		mix := func(v int) {
			h ^= uint64(v)
			h *= 1099511628211
		}
		for _, d := range ds {
			var bg, bd color.NRGBA
			switch d.kind {
			case decoOcc:
				bg, bd = occBg, occBd
			case decoSelHL:
				bg, bd = selBg, selBd
			case decoUnicode:
				bg, bd = uniBg, uniBd
			}
			decoRects(lv, d.a, d.b, func(x0, x1 float32) {
				mix(int(d.kind))
				mix(int(math.Float32bits(x0)))
				mix(int(math.Float32bits(x1)))
				if d.kind == decoSwatch {
					mix(int(d.c.R) | int(d.c.G)<<8 | int(d.c.B)<<16 | int(d.c.A)<<24)
				}
				if bg.A == 0 && bd.A == 0 {
					return
				}
				box := rectXYWH(tx+x0, y, max(x1-x0, 2), lh)
				if bg.A > 0 {
					l.Fill(box, geom.Radii{}, bg)
				}
				decoStroke(l, box, bd)
			})
		}
		sigs[k] = h
	}
	return sigs
}

// paintDecoOver draws the decorations over the text (the links'
// underlines, the swatches) and records where the links and swatches are
// for the pointer.
func (w *ViewW) paintDecoOver(l *paint.List, snap buffer.Snapshot, lines []lineView, decos [][]deco, tx, colW, lh float32, lineY func(int) float32) {
	vs := w.vs
	vs.linkHits, vs.swatches = vs.linkHits[:0], vs.swatches[:0]
	if decos == nil {
		return
	}
	th := themeOf(w.st)
	swBorder := color.NRGBA{0, 0, 0, 0xff}
	if t := themeType(th); t == 1 || t == 2 {
		swBorder = color.NRGBA{0xee, 0xee, 0xee, 0xff}
	}
	bg := defaultColor(th, "editor.background")
	em := w.opts.fontSize()
	sw := float32(math.Round(float64(0.8 * em)))       // Monaco's 0.8em box
	bw := max(1, float32(math.Round(float64(0.1*em)))) // its 0.1em border
	for k, ds := range decos {
		lv := &lines[k]
		y := lineY(lv.vi)
		ls := snap.LineStart(lv.i)
		for _, d := range ds {
			switch d.kind {
			case decoLink:
				fg := runColorAt(lv, lv.runeAt(d.a))
				base := float32(0)
				if len(lv.segs) > 0 && len(lv.segs[0].lay.Lines) > 0 {
					base = lv.segs[0].lay.Lines[0].Baseline
				}
				decoRects(lv, d.a, d.b, func(x0, x1 float32) {
					yy := y + base + 2
					l.Line(geom.Pt(tx+x0, yy), geom.Pt(tx+x1, yy), 1, fg)
					if r := rectXYWH(tx+x0, y, x1-x0, lh).Intersect(vs.text); !r.Empty() {
						vs.linkHits = append(vs.linkHits, linkHit{r: r, line: lv.i, a: d.a, b: d.b, url: d.url})
					}
				})
			case decoSwatch:
				if d.a < lv.b0 || d.a >= lv.b1 {
					continue // the swatch goes before the literal, on its first row
				}
				// In the literal's gap (injected.go), after its 0.2em
				// margin: a border box of the border color around the
				// color (over the background, for translucent colors).
				gx, found := float32(0), false
				lv.eachGap(func(in injection, x0, _ float32) {
					if in.hint == nil && in.col == d.a {
						gx, found = x0, true
					}
				})
				if !found {
					continue
				}
				box := rectXYWH(float32(math.Round(float64(tx+gx+0.2*em))), float32(math.Round(float64(y+(lh-sw)/2))), sw, sw)
				l.Fill(box, geom.Radii{}, swBorder)
				inner := geom.Rect{X: box.X + bw, Y: box.Y + bw, W: max(0, box.W-2*bw), H: max(0, box.H-2*bw)}
				l.Fill(inner, geom.Radii{}, bg)
				l.Fill(inner, geom.Radii{}, d.c)
				if r := box.Intersect(vs.text); !r.Empty() {
					vs.swatches = append(vs.swatches, swatchHit{r: r, a: ls + d.a, b: ls + d.b, c: d.c})
				}
			}
		}
	}
}

// runColorAt returns the text color of rune i of row lv.
func runColorAt(lv *lineView, i int) color.NRGBA {
	for k := range lv.segs {
		sg := &lv.segs[k]
		if i < sg.r0 || i >= sg.r1 {
			continue
		}
		for _, p := range sg.paints {
			if j := sg.local(i); j >= p.r0 && j < p.r1 {
				return p.fg
			}
		}
	}
	return color.NRGBA{A: 255}
}

// activeLinkSpan returns the color override of the active link on model
// line i (links.go), false for none.
func (w *ViewW) activeLinkSpan(i int) (brSpan, bool) {
	al := w.vs.activeLink
	if al.line != i || !w.opts.Links {
		return brSpan{}, false
	}
	text := w.st.Model.line(w.st.Snapshot(), i)
	for _, lk := range w.lineLinks(i, text, nil) {
		if lk.a == al.a {
			return brSpan{lk.a, lk.b, defaultColor(themeOf(w.st), "editorLink.activeForeground")}, true
		}
	}
	return brSpan{}, false
}

// withSpan returns spans with sp laid over them: the spans it overlaps
// are dropped.
func withSpan(spans []brSpan, sp brSpan) []brSpan {
	out := make([]brSpan, 0, len(spans)+1)
	done := false
	for _, s := range spans {
		if s.b1 > sp.b0 && s.b0 < sp.b1 {
			continue
		}
		if !done && s.b0 >= sp.b1 {
			out = append(out, sp)
			done = true
		}
		out = append(out, s)
	}
	if !done {
		out = append(out, sp)
	}
	return out
}
