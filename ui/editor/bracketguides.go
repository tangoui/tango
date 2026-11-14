package editor

import (
	"image/color"
	"math"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/monarch"
	"github.com/tangoui/tango/ui/editor/theme"
)

// Bracket pair guides (Monaco's guides.bracketPairs: the model's
// getLinesBracketGuides, guidesTextModelPart.js, mapped to view lines by
// viewModelLines.getViewLinesBracketGuides and drawn by
// IndentGuidesOverlay, indentGuides.js; Copyright (c) Microsoft
// Corporation, MIT License, see ui/editor/theme/NOTICE).
//
// Every closed bracket pair of the bracket tree (brackets.go) that spans
// several lines gets a one pixel vertical guide at the visible column
// min(opening bracket's column, closing bracket's column, the smallest
// indentation of the non-blank lines after the opening line up to the
// closing line), on the lines between its brackets: on the opening line
// only on the rows after the bracket's row (word wrap), on the closing line
// only when text precedes the closing bracket there (then up to the
// bracket's row). With BracketPairsHorizontal, a pair also gets a
// horizontal line from the guide to the opening bracket, at the bottom of
// the bracket's row, and one to the closing bracket, at the top of its row
// (at the bottom when text precedes it), each when the bracket is right of
// the guide; a pair on one line gets one at the bottom of its rows between
// its brackets. BracketGuidesActive draws the primary caret's pair only:
// the innermost pair whose range strictly contains the caret (single-line
// pairs included, as Monaco). Unclosed pairs get no guide. The colors are
// editorBracketPairGuide.background<level> (activeBackground<level> for
// the active pair with HighlightActiveBracketPair), falling back to the
// bracket colors editorBracketHighlight.foreground<level> (at 30% for the
// inactive ones), the level being the pair's nesting level as bracket pair
// colorization counts it, cycling through the levels whose colors are not
// transparent. On a row with bracket guides, an indentation guide at the
// column of a vertical bracket guide is not drawn and the active
// indentation guide is not highlighted (Monaco's highlightActiveIndentation
// true).
//
// The guides need the bracket tree: a document too large for one (bracket
// pair colorization off) has none. Only the drawn lines are looked at: the
// pairs open across their first line (of each run of lines the folded
// regions separate) and the pairs opening on them, whose closing brackets
// the tree finds within the frame's bracket budget (the same one the
// colors and the matched brackets use).

// Bracket guide kinds.
const (
	bgVertical = iota
	bgTop      // a horizontal line at the top of the row
	bgBottom   // a horizontal line at the bottom of the row
)

// bracketGuide is a guide drawn on a row.
type bracketGuide struct {
	kind   int
	col    int     // the guide's visible column (-1: a line between brackets on one line)
	x0, x1 float32 // content x (vertical: x0)
	level  int     // the pair's nesting level (palette index before cycling)
	active bool    // the primary caret's pair
	hl     bool    // drawn in the active color
}

// bgRow is the bracket guides of a laid out row.
type bgRow struct {
	line, row int
	g         []bracketGuide
}

// guidePair is a closed bracket pair touching the drawn lines.
type guidePair struct {
	oL, oB, oE int // opening bracket: line, bytes [oB, oE)
	cL, cB     int // closing bracket: line, first byte
	level      int
	active     bool
	cols       pairCols // multi-line pairs (columns)
}

// pairCols is the visible columns of a multi-line pair.
type pairCols struct {
	o, c, g    int  // the opening and closing brackets', the guide's
	textBefore bool // text precedes the closing bracket on its line
}

// pairAt identifies a pair by its opening bracket.
type pairAt struct{ line, b int }

// bgCache holds, for one text and tab size, the decided pairs opening on
// lines and the columns of pairs.
type bgCache struct {
	tree  *bracketTree
	buf   *buffer.Buffer
	ver   uint64
	tab   int
	opens map[int][]guidePair // by line: its closed pairs (present: decided)
	cols  map[pairAt]pairCols
	// The active pair for a caret (decided).
	actOK   bool
	actHead int
	act     bracketPair
	actHas  bool
	tries   int // frames requested while undecided (per text and lines)
	// The view lines and top of the last call (tries).
	lo, hi, top int
}

// bracketGuidePairs returns the pairs to draw guides for on the model
// lines of view lines lo..hi, and whether bracket pair guides are on for
// the document (Options, a bracket tree). head is the primary caret.
func (w *ViewW) bracketGuidePairs(snap buffer.Snapshot, vl *viewLines, lo, hi, mhi, head int, deadline time.Time) ([]guidePair, bool) {
	st, vs := w.st, w.vs
	mode, _ := w.opts.bracketGuides()
	if mode == BracketGuidesOff {
		return nil, false
	}
	t := st.Model.brackets()
	if t == nil || len(t.conf.defs) == 0 {
		return nil, false
	}
	tab := max(w.tabs(), 1)
	c := &vs.bgc
	if c.tree != t || c.buf != st.Model.Buffer() || c.ver != snap.Version() || c.tab != tab || c.opens == nil {
		*c = bgCache{tree: t, buf: st.Model.Buffer(), ver: snap.Version(), tab: tab, opens: map[int][]guidePair{}, cols: map[pairAt]pairCols{}}
	}
	if c.lo != lo || c.hi != hi || c.top != st.Viewport.TopLine {
		// Other lines: the undecided pairs get their frames again.
		c.lo, c.hi, c.top, c.tries = lo, hi, st.Viewport.TopLine, 0
	}
	src := modelBrackets{st.Model}
	t.tokenize = false // the pairs follow the Highlighter, as the colors (bracketSpans)
	t.ensure(src, mhi+1, deadline)
	hl := st.Highlighter()
	unsure := false
	seen := map[pairAt]bool{}
	var pairs []guidePair
	add := func(p guidePair) {
		k := pairAt{p.oL, p.oB}
		if !seen[k] {
			seen[k] = true
			pairs = append(pairs, p)
		}
	}
	// level is a stack entry's nesting level as the bracket colors count it
	// (the brief: the guide levels are bracket pair colorization's): the
	// colorized entries of its stack, its own included, from 0. Monaco
	// differs in two ways: its guides take the pair's nestingLevel, which
	// counts every pair around it (colorized or not), and it does not ask
	// whether the closing bracket closes the pair as a colorized pair
	// (closesColorized); with every bracket pair colorized (the usual
	// languages) the levels are the same.
	level := func(n *bnode) int {
		lv := int(n.level)
		if t.conf.colorized[n.open] {
			lv--
		}
		return max(lv, 0)
	}
	closed := func(n *bnode, oL int, o brange, cl brange) guidePair {
		ls := snap.LineStart(oL)
		cL := snap.LineOf(cl.a)
		return guidePair{oL: oL, oB: o.a - ls, oE: o.b - ls, cL: cL, cB: cl.a - snap.LineStart(cL), level: level(n)}
	}
	var evs []bracketEvent
	var toks []monarch.Token
	prev := -2
	for v := lo; v <= hi; v++ {
		i, _ := vl.find(v)
		if i == prev {
			continue
		}
		runStart := i != prev+1
		prev = i
		if i >= t.known() {
			unsure = true
			continue
		}
		ops, done := c.opens[i]
		if done && !runStart {
			for _, p := range ops {
				add(p)
			}
			continue
		}
		text := snap.Line(i)
		toks = toks[:0]
		if hl != nil && len(text) <= maxTokenizationLineLength {
			toks, _ = hl.TokensFor(st.hlKey, snap, i, toks)
		}
		evs = evs[:0]
		t.conf.scanLine(t.start[i], text, toks, func(e bracketEvent) { evs = append(evs, e) })
		if runStart {
			// The pairs open across the line's start.
			for y := t.start[i]; y != nil; y = y.parent {
				state, cl := pairClose(t, src, snap, i, evs, y, deadline, nil)
				if state == pairUnknown {
					unsure = true
				}
				if state != pairClosed {
					continue
				}
				if op, ok := openRange(t, src, snap, i, nil, y, nil); ok {
					add(closed(y, snap.LineOf(op.a), op, cl))
				}
			}
		}
		if done {
			for _, p := range ops {
				add(p)
			}
			continue
		}
		ls := snap.LineStart(i)
		lineSure := true
		ops = nil
		for k, e := range evs {
			if e.kind != evOpen {
				continue
			}
			state, cl := pairClose(t, src, snap, i, evs[k+1:], e.node, deadline, nil)
			switch state {
			case pairUnknown:
				unsure, lineSure = true, false
			case pairClosed:
				p := closed(e.node, i, brange{ls + e.b0, ls + e.b1}, cl)
				ops = append(ops, p)
				add(p)
			}
		}
		if lineSure {
			c.opens[i] = ops
		}
	}

	// The active pair.
	if !c.actOK || c.actHead != head {
		t.unsure = false
		// The innermost pair around the caret, Monaco's active pair: none
		// when it is unclosed.
		p, ok := st.enclosing(head, deadline, true)
		if ok || !t.unsure {
			c.actOK, c.actHead, c.act, c.actHas = true, head, p, ok
		} else {
			unsure = true
		}
	}
	out := pairs[:0]
	colsUnsure := false
	for _, p := range pairs {
		ls := snap.LineStart(p.oL)
		p.active = c.actOK && c.actHas && c.act.open.a == ls+p.oB
		if mode == BracketGuidesActive && !p.active {
			continue
		}
		if p.oL != p.cL {
			var ok bool
			if p.cols, ok = w.pairColumns(snap, c, p, deadline); !ok {
				// Its column is not known yet: drawn once it is (never
				// at a guessed column).
				colsUnsure = true
				continue
			}
		}
		out = append(out, p)
	}
	// Undecided pairs: ask again next frame while the tree can go on (a
	// tree waiting for the Highlighter's final lines is woken by it), or
	// when the budget ran out, for at most maxMatchFrames frames per text.
	// The columns go on next frame in any case.
	if colsUnsure || unsure && (t.complete() || t.known() < src.finalLines(snap)) {
		if c.tries++; c.tries <= maxMatchFrames {
			ui.Invalidate(w.c.Window())
		}
	}
	return out, true
}

// pairColumns returns the columns of the multi-line pair p (cached for the
// text), false when the smallest indentation in it is not known within
// deadline (indentIndex).
func (w *ViewW) pairColumns(snap buffer.Snapshot, c *bgCache, p guidePair, deadline time.Time) (pairCols, bool) {
	k := pairAt{p.oL, p.oB}
	if pc, ok := c.cols[k]; ok {
		return pc, true
	}
	tab := c.tab
	ol, cl := snap.Line(p.oL), snap.Line(p.cL)
	pc := pairCols{o: lineColumnTab(ol, p.oB, tab), c: lineColumnTab(cl, p.cB, tab)}
	fnw := 0
	for fnw < len(cl) && (cl[fnw] == ' ' || cl[fnw] == '\t') {
		fnw++
	}
	pc.textBefore = fnw < p.cB
	// The smallest indentation of the non-blank lines after the opening
	// line, up to the closing line (Monaco's minVisibleColumnIndentation),
	// where it is below the brackets' columns.
	g, ok := w.st.Model.indents(tab).minIndent(snap, tab, p.oL+1, p.cL, min(pc.o, pc.c), deadline)
	if !ok {
		return pairCols{}, false
	}
	pc.g = g
	c.cols[k] = pc
	return pc, true
}

// Indentation index: the smallest indentation of blocks of lines, so the
// guide column of a pair spanning many lines does not read them all on
// every edit (Monaco caches it in its bracket AST). The blocks follow the
// Model's edits (bracketsEdited): an edit marks the blocks of the lines it
// changed, which are read again when asked for; the others keep their
// minimum. A query reads the lines of the blocks at its ends only when
// their minimum is below the smallest indentation found so far.

// indentBlock is the number of lines of a new block; an edit's block grows
// up to 2*indentBlock before it is split.
const indentBlock = 64

// indentFrameLines is how many lines the index reads per frame (from
// beginFrame) even past the deadline, so the frame's bracket work having
// spent its budget does not stop it; beyond that it reads until the
// deadline. A variable for tests.
var indentFrameLines = 16384

// indentFrameBytes bounds the bytes of that floor (long lines).
var indentFrameBytes = 1 << 20

// indentPrefix is how much of a line the index reads: the indentation of
// a line whose first indentPrefix bytes are all blanks is taken as the
// columns of those bytes (a line that long of blanks is not code anyone
// indents by), so a long line costs no more than a short one.
const indentPrefix = 4096

// indentRunBytes is the most bytes of a block read at once (one slice,
// split into lines); a longer block is read line by line, each line's
// first indentPrefix bytes.
const indentRunBytes = 64 << 10

const (
	indDirty = -2            // a block's lines changed: not known
	noIndent = math.MaxInt32 // a block of blank lines
)

type indBlock struct {
	n   int    // lines
	min int32  // smallest indentation of its non-blank lines, indDirty, noIndent
	id  uint64 // new for every block an edit makes (partKey)
}

// partKey is part of a block: lines lo..hi of it (from its first line).
type partKey struct {
	id     uint64
	lo, hi int
}

// indentIndex is the indentation index of a Model's text.
type indentIndex struct {
	ver    uint64 // the text's version (0: none yet)
	lines  int
	tab    int
	blocks []indBlock

	frame      int // lines read since beginFrame
	frameBytes int // bytes read since beginFrame
	scanned    int // lines read (tests)
	bytes      int // bytes read (tests)
	nextID     uint64
	// part holds the minimum of parts of blocks read (a pair's first and
	// last lines are rarely a block's): blocks an edit does not touch keep
	// their parts' minimums.
	part map[partKey]int32
}

func (x *indentIndex) block(n int) indBlock {
	x.nextID++
	return indBlock{n: n, min: indDirty, id: x.nextID}
}

// indentIndexes are a Model's indentation indexes for the two tab sizes
// used last (views of one Model may override the tab size), the latest
// first.
type indentIndexes [2]*indentIndex

// indents returns the Model's indentation index for tab stops every tab
// columns.
func (m *Model) indents(tab int) *indentIndex {
	xs := &m.ind
	switch {
	case xs[0] != nil && xs[0].tab == tab:
	case xs[1] != nil && xs[1].tab == tab:
		xs[0], xs[1] = xs[1], xs[0]
	default:
		xs[0], xs[1] = &indentIndex{tab: tab}, xs[0]
	}
	return xs[0]
}

func (xs *indentIndexes) edit(old, ns buffer.Snapshot, edits []buffer.Edit) {
	for _, x := range xs {
		x.edit(old, ns, edits)
	}
}

func (xs *indentIndexes) beginFrame() {
	for _, x := range xs {
		x.beginFrame()
	}
}

// beginFrame starts a frame's reading (indentFrameLines).
func (x *indentIndex) beginFrame() {
	if x != nil {
		x.frame, x.frameBytes = 0, 0
	}
}

// reset marks every line of snap unknown.
func (x *indentIndex) reset(snap buffer.Snapshot, tab int) {
	n := snap.LineCount()
	x.ver, x.lines, x.tab = snap.Version(), n, tab
	x.blocks = x.blocks[:0]
	clear(x.part)
	for s := 0; s < n; s += indentBlock {
		x.blocks = append(x.blocks, x.block(min(indentBlock, n-s)))
	}
}

// edit follows the edits that turned old into ns (in old's offsets, in
// order); another text than the index's makes it start over when next
// asked.
func (x *indentIndex) edit(old, ns buffer.Snapshot, edits []buffer.Edit) {
	if x == nil || x.ver == 0 {
		return
	}
	if old.Version() != x.ver || old.LineCount() != x.lines {
		x.ver = 0
		return
	}
	type region struct{ sl, el, m int } // old lines sl..el become m lines
	var regs []region
	total := 0
	for _, e := range edits {
		if e.Start == e.End && e.Text == "" {
			continue
		}
		sl, el := old.LineOf(e.Start), old.LineOf(e.End)
		d := strings.Count(e.Text, "\n") + strings.Count(e.Text, "\r") - strings.Count(e.Text, "\r\n")
		total += d - (el - sl)
		if k := len(regs); k > 0 && sl <= regs[k-1].el {
			// As in bracketTree.edit: the edit starts on the region's
			// last old line, its last new line.
			regs[k-1].m += d + sl - regs[k-1].el
			regs[k-1].el = max(el, regs[k-1].el)
			continue
		}
		regs = append(regs, region{sl, el, d + 1})
	}
	if old.LineCount()+total != ns.LineCount() {
		x.ver = 0
		return
	}
	// From the last region back, so the earlier lines keep their index.
	for k := len(regs) - 1; k >= 0; k-- {
		x.replace(regs[k].sl, regs[k].el, regs[k].m)
	}
	x.ver, x.lines = ns.Version(), ns.LineCount()
}

// replace makes lines sl..el m lines: the blocks holding them become
// unknown blocks of the new line count.
func (x *indentIndex) replace(sl, el, m int) {
	s, j0 := 0, 0
	for j0 < len(x.blocks) && s+x.blocks[j0].n <= sl {
		s += x.blocks[j0].n
		j0++
	}
	j1, e := j0, s
	total := 0
	for j1 < len(x.blocks) {
		total += x.blocks[j1].n
		e += x.blocks[j1].n
		j1++
		if e > el {
			break
		}
	}
	n := total - (el - sl + 1) + m
	var nb []indBlock
	for n > 2*indentBlock {
		nb = append(nb, x.block(indentBlock))
		n -= indentBlock
	}
	if n > 0 {
		nb = append(nb, x.block(n))
	}
	x.blocks = append(x.blocks[:j0], append(nb, x.blocks[j1:]...)...)
}

// minIndent returns the smallest indentation (tab stops every tab columns)
// of the non-blank lines a..b of snap when it is below limit, else limit;
// false when the lines it must read for that cannot be read before
// deadline (past indentFrameLines this frame). The blocks read are kept:
// a later call goes on from there.
func (x *indentIndex) minIndent(snap buffer.Snapshot, tab, a, b, limit int, deadline time.Time) (int, bool) {
	if x.ver != snap.Version() || x.tab != tab || x.lines != snap.LineCount() {
		x.reset(snap, tab)
	}
	g := limit
	s := 0
	for j := range x.blocks {
		bl := &x.blocks[j]
		e := s + bl.n // lines s..e-1
		if e <= a {
			s = e
			continue
		}
		if s > b || g <= 0 {
			break
		}
		if bl.min == indDirty {
			if !x.budget(deadline) {
				return 0, false
			}
			bl.min = x.scan(snap, s, e-1)
		}
		if int(bl.min) < g {
			if a <= s && e-1 <= b {
				g = int(bl.min)
			} else {
				k := partKey{bl.id, max(a, s) - s, min(b, e-1) - s}
				m, ok := x.part[k]
				if !ok {
					if !x.budget(deadline) {
						return 0, false
					}
					m = x.scan(snap, max(a, s), min(b, e-1))
					if x.part == nil || len(x.part) > 4096 {
						x.part = map[partKey]int32{}
					}
					x.part[k] = m
				}
				g = min(g, int(m))
			}
		}
		s = e
	}
	return g, true
}

func (x *indentIndex) budget(deadline time.Time) bool {
	return (x.frame < indentFrameLines && x.frameBytes < indentFrameBytes) || !indentPast(deadline)
}

// indentLinesRead counts the lines every index read (tests: indexes
// replaced by others take their counts with them). Atomic: Models on
// other goroutines (parallel tests) scan too.
var indentLinesRead atomic.Int64

// indentPast reports whether deadline has passed (a variable for tests).
var indentPast = func(deadline time.Time) bool { return time.Now().After(deadline) }

// scan reads lines lo..hi and returns their smallest indentation.
func (x *indentIndex) scan(snap buffer.Snapshot, lo, hi int) int32 {
	m := int32(noIndent)
	line := func(s string, truncated bool) {
		if len(s) > indentPrefix { // the same prefix as a line read alone
			s, truncated = s[:indentPrefix], true
		}
		ind := indentOf(s, x.tab)
		if ind < 0 && truncated {
			ind = indentOf(s+"x", x.tab) // blanks to the prefix's end
		}
		if ind >= 0 {
			m = min(m, int32(ind))
		}
	}
	x.scanned += hi - lo + 1
	indentLinesRead.Add(int64(hi - lo + 1))
	x.frame += hi - lo + 1
	end := snap.Len()
	if hi+1 < snap.LineCount() {
		end = snap.LineStart(hi+1) - 1
	}
	if start := snap.LineStart(lo); end-start <= indentRunBytes {
		x.bytes += end - start
		x.frameBytes += end - start
		for _, s := range strings.Split(snap.Slice(start, end), "\n") {
			line(s, false)
		}
		return m
	}
	for i := lo; i <= hi; i++ {
		ls := snap.LineStart(i)
		le := snap.Len()
		if i+1 < snap.LineCount() {
			le = snap.LineStart(i+1) - 1
		}
		n := min(le-ls, indentPrefix)
		x.bytes += n
		x.frameBytes += n
		line(snap.Slice(ls, ls+n), n < le-ls)
	}
	return m
}

// rowBracketGuides returns the bracket guides of the laid out rows.
func (w *ViewW) rowBracketGuides(rows []lineView, pairs []guidePair, colW float32) []bgRow {
	_, horiz := w.opts.bracketGuides()
	hlActive := w.opts.Guides.HighlightActiveBracketPair
	out := make([]bgRow, len(rows))
	for k := range rows {
		lv := &rows[k]
		r := &out[k]
		r.line, r.row = lv.i, lv.row
		for _, p := range pairs {
			if lv.i < p.oL || lv.i > p.cL {
				continue
			}
			hz := horiz == BracketGuidesOn || (horiz == BracketGuidesActive && p.active)
			g := bracketGuide{level: p.level, active: p.active, hl: hlActive && p.active, col: -1}
			if p.oL == p.cL {
				// A pair on one line: a line under the text between its
				// brackets, across its rows.
				if !hz {
					continue
				}
				var ok bool
				switch {
				case rowHas(lv, p.oE):
					g.x0, ok = lv.caretX(lv.runeAt(p.oE))
				case p.oE < lv.b0:
					g.x0, ok = lv.caretX(lv.r0)
				}
				if !ok {
					continue
				}
				switch {
				case rowHas(lv, p.cB):
					g.x1, ok = lv.caretX(lv.runeAt(p.cB))
				case p.cB < lv.b0:
					ok = false
				default:
					g.x1 = lv.endX
				}
				if ok && g.x1 > g.x0 {
					g.kind = bgBottom
					r.g = append(r.g, g)
				}
				continue
			}
			pc := p.cols
			gx := float32(pc.g) * colW
			gx += lv.gapShift(gx, colW) // injected gaps before the column (injected.go)
			vert := false
			switch {
			case lv.i > p.oL && lv.i < p.cL:
				vert = true
			case lv.i == p.oL:
				vert = lv.long == nil && p.oB < lv.b0 // rows after the bracket's
			default: // the closing line
				vert = pc.textBefore && (lv.long != nil || p.cB >= lv.b0)
			}
			if vert {
				v := g
				v.kind, v.col, v.x0, v.x1 = bgVertical, pc.g, gx, gx
				r.g = append(r.g, v)
			}
			if !hz {
				continue
			}
			if lv.i == p.oL && pc.o > pc.g && rowHas(lv, p.oB) {
				if x, ok := lv.caretX(lv.runeAt(p.oB)); ok {
					h := g
					h.kind, h.col, h.x0, h.x1 = bgBottom, pc.g, gx, x
					r.g = append(r.g, h)
				}
			}
			if lv.i == p.cL && pc.c > pc.g && rowHas(lv, p.cB) {
				if x, ok := lv.caretX(lv.runeAt(p.cB)); ok {
					h := g
					h.kind, h.col, h.x0, h.x1 = bgTop, pc.g, gx, x
					if pc.textBefore {
						h.kind = bgBottom
					}
					r.g = append(r.g, h)
				}
			}
		}
	}
	return out
}

// rowHas reports whether the character at byte b of its line is on row
// lv.
func rowHas(lv *lineView, b int) bool {
	if lv.long != nil {
		return true
	}
	return b >= lv.b0 && (b < lv.b1 || lv.last())
}

// bracketGuideColors are the inactive and active colors of the bracket
// guide levels (Monaco's theming participant: levels with a transparent
// color are skipped).
type bracketGuideColors struct{ bg, active []color.NRGBA }

func bracketGuideColorsOf(th *theme.Theme) bracketGuideColors {
	var bc bracketGuideColors
	opaque := func(key string) (color.NRGBA, bool) {
		c, ok := lookupColor(th, key)
		return c, ok && c.A != 0
	}
	for _, k := range [...]string{"1", "2", "3", "4", "5", "6"} {
		fg, okFg := opaque("editorBracketHighlight.foreground" + k)
		g, ok1 := opaque("editorBracketPairGuide.background" + k)
		if !ok1 && okFg {
			g, ok1 = fg, true
			g.A = uint8(math.Round(float64(fg.A) * 0.3))
			ok1 = g.A != 0
		}
		a, ok2 := opaque("editorBracketPairGuide.activeBackground" + k)
		if !ok2 && okFg {
			a, ok2 = fg, true
		}
		if ok1 && ok2 {
			bc.bg = append(bc.bg, g)
			bc.active = append(bc.active, a)
		}
	}
	return bc
}
