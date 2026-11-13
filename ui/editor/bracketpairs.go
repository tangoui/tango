package editor

import (
	"image/color"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/monarch"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// Bracket matching (Monaco's BracketMatchingController and the bracket
// pairs queries matchBracket, findEnclosingBrackets, findNextBracket), the
// bracket commands and bracket pair colorization; brackets.go has the
// bracket tree.

// bracketBudget bounds the bracket work of one query or frame: scanning
// lines for the tree, or the local search of a document too large for it
// (Monaco gives its searches 20 ms, off the typing path; here they run in
// the frame).
const bracketBudget = 3 * time.Millisecond

// maxMatchSelections: with more selections no brackets are matched
// (Monaco's limit of 100).
const maxMatchSelections = 100

// brange is a bracket's byte range in the document.
type brange struct{ a, b int }

func (r brange) contains(off int) bool { return r.a <= off && off <= r.b }

// bracketPair is a pair's opening and closing brackets.
type bracketPair struct{ open, close brange }

// modelBrackets is the bracketSource of a Model: the Highlighter's final
// tokens.
type modelBrackets struct{ m *Model }

func (s modelBrackets) tokens(snap buffer.Snapshot, line int, text string, dst []monarch.Token) []monarch.Token {
	hl := s.m.Highlighter()
	if hl == nil || len(text) > maxTokenizationLineLength {
		return dst[:0]
	}
	return hl.TokenizeSpan(snap, line, text, dst)
}

func (s modelBrackets) finalLines(snap buffer.Snapshot) int {
	if hl := s.m.Highlighter(); hl != nil {
		return hl.FinalLines(snap)
	}
	return snap.LineCount()
}

// brackets returns the Model's bracket tree, nil when the document is too
// large for one (maxBracketTreeLen).
func (m *Model) brackets() *bracketTree {
	snap := m.buf.Snapshot()
	if snap.Len() > maxBracketTreeLen {
		m.brk = nil
		return nil
	}
	if m.brk == nil {
		m.brk = newBracketTree(bracketConfOf(m.conf), snap)
	} else if m.brk.snap.Version() != snap.Version() {
		m.brk.reset(snap) // a change that bypassed edit (not expected)
	}
	return m.brk
}

// bracketsEdited passes a change to the bracket tree.
func (m *Model) bracketsEdited(old, ns buffer.Snapshot, edits []buffer.Edit) {
	if m.brk != nil {
		m.brk.edit(old, ns, edits)
	}
}

// bracketTreeFor returns the tree with the lines up to line known when
// they can be within deadline (the Highlighter is first asked to tokenize
// them), else nil.
func (s *State) bracketTreeFor(line int, deadline time.Time) *bracketTree {
	t := s.Model.brackets()
	if t == nil {
		return nil
	}
	if t.known() <= line {
		if hl := s.Highlighter(); hl != nil {
			hl.HighlightFor(s.hlKey, s.Snapshot(), line+1, time.Until(deadline))
		}
		t.ensure(modelBrackets{s.Model}, line+1, deadline)
	}
	if t.known() <= line {
		t.unsure = true
		return nil
	}
	return t
}

// abs turns an event of line i into a document range.
func evRange(snap buffer.Snapshot, i int, e bracketEvent) brange {
	ls := snap.LineStart(i)
	return brange{ls + e.b0, ls + e.b1}
}

// matchBracket returns the pair whose opening or closing bracket touches
// off (Monaco's matchBracket: a pair with both brackets; of several, the
// one whose touching bracket starts last).
func (s *State) matchBracket(off int, deadline time.Time) (bracketPair, bool) {
	snap := s.Snapshot()
	L := snap.LineOf(off)
	t := s.bracketTreeFor(L, deadline)
	if t == nil {
		if s.Model.brackets() == nil {
			return s.legacyMatch(off, deadline)
		}
		return bracketPair{}, false
	}
	src := modelBrackets{s.Model}
	col := off - snap.LineStart(L)
	evs := t.lineEvents(src, L, nil)
	var best bracketPair
	bestAt, found := -1, false
	for k, e := range evs {
		if e.b0 > col || col > e.b1 || (e.kind != evOpen && e.kind != evClose) {
			continue
		}
		var p bracketPair
		ok := false
		if e.kind == evOpen {
			p.open = evRange(snap, L, e)
			p.close, ok = closeRange(t, src, snap, L, evs[k+1:], e.node, deadline)
		} else {
			p.close = evRange(snap, L, e)
			p.open, ok = openRange(t, src, snap, L, evs[:k], e.node)
		}
		if ok && e.b0 >= bestAt {
			best, bestAt, found = p, e.b0, true
		}
	}
	return best, found
}

// Close states of an open bracket.
const (
	pairClosed = iota
	pairUnclosed
	pairUnknown // not known yet (lines not scanned)
)

// pairClose finds the closing bracket of stack entry n (open after the
// events before rest on line L): on that line, or on a later one.
func pairClose(t *bracketTree, src bracketSource, snap buffer.Snapshot, L int, rest []bracketEvent, n *bnode, deadline time.Time) (int, brange) {
	for _, e := range rest {
		if e.node == n && (e.kind == evClose || e.kind == evUnclosed) {
			if e.kind == evUnclosed {
				return pairUnclosed, brange{}
			}
			return pairClosed, evRange(snap, L, e)
		}
	}
	j, e, found, never := t.closeOf(src, L, n.depth, deadline)
	switch {
	case found && e.kind == evClose:
		return pairClosed, evRange(snap, j, e)
	case found || never:
		return pairUnclosed, brange{}
	}
	return pairUnknown, brange{}
}

// closeRange is pairClose for a closed pair.
func closeRange(t *bracketTree, src bracketSource, snap buffer.Snapshot, L int, rest []bracketEvent, n *bnode, deadline time.Time) (brange, bool) {
	st, r := pairClose(t, src, snap, L, rest, n, deadline)
	return r, st == pairClosed
}

// openRange finds the opening bracket of stack entry n, open before the
// events after before on line L.
func openRange(t *bracketTree, src bracketSource, snap buffer.Snapshot, L int, before []bracketEvent, n *bnode) (brange, bool) {
	for k := len(before) - 1; k >= 0; k-- {
		if e := before[k]; e.kind == evOpen && e.node == n {
			return evRange(snap, L, e), true
		}
	}
	k, e, ok := t.openOf(src, L, n.depth)
	if !ok {
		return brange{}, false
	}
	return evRange(snap, k, e), true
}

// enclosingBrackets returns the innermost pair (with both brackets) whose
// range strictly contains off (Monaco's findEnclosingBrackets).
func (s *State) enclosingBrackets(off int, deadline time.Time) (bracketPair, bool) {
	snap := s.Snapshot()
	L := snap.LineOf(off)
	t := s.bracketTreeFor(L, deadline)
	if t == nil {
		if s.Model.brackets() == nil {
			return s.legacyEnclosing(off, deadline)
		}
		return bracketPair{}, false
	}
	src := modelBrackets{s.Model}
	col := off - snap.LineStart(L)
	evs := t.lineEvents(src, L, nil)
	// The stack at off: the brackets that end before it.
	st := t.start[L]
	k := 0
	for ; k < len(evs) && evs[k].b1 <= col; k++ {
		switch e := evs[k]; e.kind {
		case evOpen:
			st = e.node
		case evClose:
			st = e.node.parent
		}
	}
	for y := st; y != nil; y = y.parent {
		state, cl := pairClose(t, src, snap, L, evs[k:], y, deadline)
		if state == pairUnclosed {
			continue // the pair below
		}
		if state == pairUnknown {
			return bracketPair{}, false
		}
		op, ok := openRange(t, src, snap, L, evs[:k], y)
		if !ok {
			return bracketPair{}, false
		}
		return bracketPair{op, cl}, true
	}
	return bracketPair{}, false
}

// nextBracket returns the first bracket starting at or after off (Monaco's
// findNextBracket).
func (s *State) nextBracket(off int, deadline time.Time) (brange, bool) {
	snap := s.Snapshot()
	bc := bracketConfOf(s.Model.conf)
	src := modelBrackets{s.Model}
	L := snap.LineOf(off)
	col := off - snap.LineStart(L)
	var toks []monarch.Token
	for j := L; j < snap.LineCount(); j++ {
		if hl := s.Highlighter(); hl != nil && src.finalLines(snap) <= j {
			hl.HighlightFor(s.hlKey, snap, j+1, time.Until(deadline))
		}
		text := snap.Line(j)
		toks = src.tokens(snap, j, text, toks[:0])
		r, found := brange{}, false
		bc.forBrackets(text, toks, func(_ *bracketDef, b0, b1 int) {
			if !found && (j > L || b0 >= col) {
				ls := snap.LineStart(j)
				r, found = brange{ls + b0, ls + b1}, true
			}
		})
		if found {
			return r, true
		}
		if time.Now().After(deadline) {
			break
		}
	}
	return brange{}, false
}

// Legacy search: documents without a bracket tree (Monaco's fallback when
// the document is too large for the AST). Brackets are counted per opening
// bracket kind, line by line from off, until deadline.

type foundBracket struct {
	def  *bracketDef
	r    brange
	line int
}

// lineBrackets returns the brackets of line j.
func (s *State) lineBrackets(snap buffer.Snapshot, j int, toks *[]monarch.Token) []foundBracket {
	bc := bracketConfOf(s.Model.conf)
	text := snap.Line(j)
	*toks = modelBrackets{s.Model}.tokens(snap, j, text, (*toks)[:0])
	ls := snap.LineStart(j)
	var out []foundBracket
	bc.forBrackets(text, *toks, func(d *bracketDef, b0, b1 int) {
		out = append(out, foundBracket{d, brange{ls + b0, ls + b1}, j})
	})
	return out
}

func hasOpen(d *bracketDef, ids []int) bool {
	for _, id := range ids {
		if d.open == id {
			return true
		}
	}
	return false
}

func closesAny(d *bracketDef, ids []int) bool {
	for _, o := range d.opens {
		for _, id := range ids {
			if o == id {
				return true
			}
		}
	}
	return false
}

// legacySearch walks the brackets after (forward) or before off and
// returns the one where count, started at 1, reaches 0; step returns the
// count change of a bracket.
func (s *State) legacySearch(off int, forward bool, step func(*bracketDef) int, deadline time.Time) (foundBracket, bool) {
	snap := s.Snapshot()
	L := snap.LineOf(off)
	var toks []monarch.Token
	count := 1
	for j := L; j >= 0 && j < snap.LineCount(); {
		bs := s.lineBrackets(snap, j, &toks)
		for k := range bs {
			if !forward {
				k = len(bs) - 1 - k
			}
			b := bs[k]
			if forward && b.r.a < off || !forward && b.r.b > off {
				continue
			}
			if count += step(b.def); count == 0 {
				return b, true
			}
		}
		if time.Now().After(deadline) {
			break
		}
		if forward {
			j++
		} else {
			j--
		}
	}
	return foundBracket{}, false
}

func (s *State) legacyPartner(b foundBracket, deadline time.Time) (bracketPair, bool) {
	if b.def.open >= 0 {
		id := []int{b.def.open}
		c, ok := s.legacySearch(b.r.b, true, func(d *bracketDef) int {
			switch {
			case hasOpen(d, id):
				return 1
			case closesAny(d, id):
				return -1
			}
			return 0
		}, deadline)
		return bracketPair{b.r, c.r}, ok
	}
	ids := b.def.opens
	o, ok := s.legacySearch(b.r.a, false, func(d *bracketDef) int {
		switch {
		case closesAny(d, ids):
			return 1
		case hasOpen(d, ids):
			return -1
		}
		return 0
	}, deadline)
	return bracketPair{o.r, b.r}, ok
}

func (s *State) legacyMatch(off int, deadline time.Time) (bracketPair, bool) {
	snap := s.Snapshot()
	var toks []monarch.Token
	var best bracketPair
	bestAt, found := -1, false
	for _, b := range s.lineBrackets(snap, snap.LineOf(off), &toks) {
		if !b.r.contains(off) {
			continue
		}
		if p, ok := s.legacyPartner(b, deadline); ok && b.r.a >= bestAt {
			best, bestAt, found = p, b.r.a, true
		}
	}
	return best, found
}

func (s *State) legacyEnclosing(off int, deadline time.Time) (bracketPair, bool) {
	counts := map[int]int{}
	c, ok := s.legacySearch(off, true, func(d *bracketDef) int {
		if d.open >= 0 {
			counts[d.open]++
			return 0
		}
		for _, o := range d.opens {
			if counts[o]--; counts[o] == -1 {
				return -1
			}
		}
		return 0
	}, deadline)
	if !ok {
		return bracketPair{}, false
	}
	return s.legacyPartner(c, deadline)
}

// Commands.

func init() {
	RegisterCommand(Command{ID: "editor.action.jumpToBracket", Label: "Go to Bracket", Run: func(st *State, _ any) bool {
		st.jumpToBracket()
		return true
	}})
	RegisterCommand(Command{ID: "editor.action.selectToBracket", Label: "Select to Bracket", Run: func(st *State, args any) bool {
		sel := true
		if m, ok := args.(map[string]any); ok {
			if v, ok := m["selectBrackets"].(bool); ok {
				sel = v
			}
		}
		st.selectToBracket(sel)
		return true
	}})
}

// jumpToBracket moves each caret to the bracket matching the one it
// touches, else to the closing bracket of the enclosing pair, else to the
// next bracket (Monaco's jumpToBracket).
func (s *State) jumpToBracket() {
	deadline := time.Now().Add(20 * time.Millisecond)
	snap := s.Snapshot()
	sels := s.sels()
	out := make([]Selection, len(sels))
	for i, x := range sels {
		pos, _ := ordered(Selection{Anchor: clampOff(snap, x.Anchor), Head: clampOff(snap, x.Head)})
		to := pos
		if p, ok := s.matchBracket(pos, deadline); ok {
			switch {
			case p.open.contains(pos) && !p.close.contains(pos):
				to = p.close.a
			case p.close.contains(pos):
				to = p.open.a
			}
		} else if p, ok := s.enclosingBrackets(pos, deadline); ok {
			to = p.close.a
		} else if r, ok := s.nextBracket(pos, deadline); ok {
			to = r.a
		}
		out[i] = Selection{Anchor: to, Head: to}
	}
	s.setSels(out, nil)
	s.revealCaret()
}

// selectToBracket selects from each caret's bracket pair's opening to its
// closing bracket, including them when sel (Monaco's selectToBracket with
// selectBrackets).
func (s *State) selectToBracket(sel bool) {
	deadline := time.Now().Add(20 * time.Millisecond)
	snap := s.Snapshot()
	var out []Selection
	for _, x := range s.sels() {
		pos, _ := ordered(Selection{Anchor: clampOff(snap, x.Anchor), Head: clampOff(snap, x.Head)})
		p, ok := s.matchBracket(pos, deadline)
		if !ok {
			p, ok = s.enclosingBrackets(pos, deadline)
		}
		if !ok {
			if r, found := s.nextBracket(pos, deadline); found {
				p, ok = s.matchBracket(r.a, deadline)
			}
		}
		if !ok {
			continue
		}
		from, to := p.open.b, p.close.a
		if sel {
			from, to = p.open.a, p.close.b
		}
		if p.close.contains(pos) {
			from, to = to, from
		}
		out = append(out, Selection{Anchor: from, Head: to})
	}
	if len(out) > 0 {
		s.setSels(out, nil)
		s.revealCaret()
	}
}

// matchedBrackets returns the bracket ranges matchBrackets highlights: for
// each caret (not selection), its matching pair, else with "always" its
// enclosing pair (Monaco's _recomputeBrackets); none with more than
// maxMatchSelections selections.
func (s *State) matchedBrackets(mode MatchBracketsMode, deadline time.Time) []brange {
	if mode == MatchBracketsNever || len(s.Sels) > maxMatchSelections {
		return nil
	}
	snap := s.Snapshot()
	var out []brange
	for _, x := range s.sels() {
		if x.Anchor != x.Head {
			continue
		}
		pos := clampOff(snap, x.Head)
		p, ok := s.matchBracket(pos, deadline)
		if !ok && mode == MatchBracketsAlways {
			p, ok = s.enclosingBrackets(pos, deadline)
		}
		if ok {
			out = append(out, p.open, p.close)
		}
	}
	return out
}

// Bracket pair colorization.

// brSpan is a colored bracket of a line: bytes [b0, b1).
type brSpan struct {
	b0, b1 int
	c      color.NRGBA
}

// bracketPalette is the colors of the nesting levels and of invalid
// brackets under a theme (Monaco's theming participant: the transparent
// foregrounds are skipped).
type bracketPalette struct {
	levels  []color.NRGBA
	invalid color.NRGBA
}

func paletteOf(th *theme.Theme) bracketPalette {
	var p bracketPalette
	for _, k := range [...]string{"1", "2", "3", "4", "5", "6"} {
		if c, ok := lookupColor(th, "editorBracketHighlight.foreground"+k); ok && c.A != 0 {
			p.levels = append(p.levels, c)
		}
	}
	p.invalid = defaultColor(th, "editorBracketHighlight.unexpectedBracket.foreground")
	return p
}

func (p bracketPalette) level(l int32) (color.NRGBA, bool) {
	if len(p.levels) == 0 || l < 0 {
		return color.NRGBA{}, false
	}
	return p.levels[int(l)%len(p.levels)], true
}

// bracketColors returns the colored brackets of model lines lines (in
// order), and whether every opening bracket's state was known (else a
// later frame may color them differently). Lines whose start stack is not
// known yet get none.
func (s *State) bracketColors(t *bracketTree, lines []int, pal bracketPalette, deadline time.Time) (map[int][]brSpan, bool) {
	src := modelBrackets{s.Model}
	out := map[int][]brSpan{}
	sure := true
	var evs []bracketEvent
	for _, i := range lines {
		if i >= t.known() {
			sure = false
			continue
		}
		evs = t.lineEvents(src, i, evs[:0])
		var spans []brSpan
		for k, e := range evs {
			switch e.kind {
			case evUnexpected:
				spans = append(spans, brSpan{e.b0, e.b1, pal.invalid})
			case evClose:
				if c, ok := pal.level(e.node.level - 1); ok && e.def.closesColorized(int(e.node.open)) {
					spans = append(spans, brSpan{e.b0, e.b1, c})
				}
			case evOpen:
				colorized := t.conf.colorized[e.node.open]
				// Closed? On this line, else on a later one.
				state := 0 // 0 closed, 1 unclosed, 2 unknown
				inLine := false
				for _, x := range evs[k+1:] {
					if x.node == e.node && (x.kind == evClose || x.kind == evUnclosed) {
						inLine = true
						if x.kind == evUnclosed {
							state = 1
						} else if !x.def.closesColorized(int(e.node.open)) {
							colorized = false
						}
						break
					}
				}
				if !inLine {
					_, ce, found, never := t.closeOf(src, i, e.node.depth, deadline)
					switch {
					case found && ce.kind == evUnclosed, !found && never:
						state = 1
					case !found:
						state = 2
						sure = false
					case !ce.def.closesColorized(int(e.node.open)):
						colorized = false
					}
				}
				if state == 1 {
					spans = append(spans, brSpan{e.b0, e.b1, pal.invalid})
				} else if c, ok := pal.level(e.node.level - 1); ok && colorized {
					spans = append(spans, brSpan{e.b0, e.b1, c})
				}
			}
		}
		if len(spans) > 0 {
			out[i] = spans
		}
	}
	return out, sure
}

// View side.

// bracketSpans returns the colored brackets of the model lines of view
// lines lo..hi (mhi is the last one's model line), nil when bracket pair
// colorization is off for the document (Options, Model.LargeFile, too
// large for a tree). It scans the tree up to the lines within the frame's
// budget; when brackets stay undecided and the tree can go on (more final
// lines), the next frame is requested.
func (w *ViewW) bracketSpans(snap buffer.Snapshot, vl *viewLines, lo, hi, mhi int, th *theme.Theme) map[int][]brSpan {
	st := w.st
	if !w.opts.BracketPairColorization || st.Model.LargeFile() {
		return nil
	}
	t := st.Model.brackets()
	if t == nil || len(t.conf.defs) == 0 {
		return nil
	}
	deadline := time.Now().Add(bracketBudget)
	src := modelBrackets{st.Model}
	t.ensure(src, mhi+1, deadline)
	var lines []int
	for v := lo; v <= hi; v++ {
		if i, _ := vl.find(v); len(lines) == 0 || lines[len(lines)-1] != i {
			lines = append(lines, i)
		}
	}
	spans, sure := st.bracketColors(t, lines, paletteOf(th), deadline)
	if !sure && !t.complete() && t.known() < src.finalLines(snap) {
		ui.Invalidate(w.c.Window())
	}
	return spans
}

// matchKey is what the matched brackets of a frame depend on.
type matchKey struct {
	buf  *buffer.Buffer
	ver  uint64
	mode MatchBracketsMode
	sels string
}

// matchDelay is how long the text must stay unchanged after an edit
// before its brackets are matched in a document too large for a bracket
// tree (Monaco's 50 ms RunOnceScheduler), so typing does not pay for the
// local search on every key. With a tree the match is cheap and is
// computed at once.
const matchDelay = 50 * time.Millisecond

// matched returns the brackets matchBrackets highlights this frame,
// recomputed when the text, the selections or the mode changed; in a
// document without a bracket tree, matchDelay after an edit (none are
// drawn meanwhile; Monaco keeps its decorations through edits until it
// recomputes them), waking the view then when a caret is in view
// (caretShown).
func (w *ViewW) matched(caretShown bool) []brange {
	st, vs := w.st, w.vs
	mode := w.opts.matchBrackets()
	if mode == MatchBracketsNever {
		return nil
	}
	var sb []byte
	for _, x := range st.sels() {
		sb = binaryAppend(sb, x.Anchor, x.Head)
	}
	k := matchKey{st.Model.Buffer(), st.Snapshot().Version(), mode, string(sb)}
	if vs.matchOK && vs.matchKey == k {
		return vs.match
	}
	t := st.Model.brackets()
	if p := vs.matchKey; t == nil && (!vs.matchOK || p.buf != k.buf || p.ver != k.ver) {
		// An edit (or the first match): wait for the text to rest.
		now := w.c.Now()
		if vs.matchPend.buf != k.buf || vs.matchPend.ver != k.ver {
			vs.matchPend, vs.matchDue = k, now.Add(matchDelay)
		}
		if now.Before(vs.matchDue) {
			if caretShown {
				// The View declares what the match changes (damage.go).
				w.c.WakeAtDamaged(vs.matchDue)
			}
			return nil
		}
	}
	if t != nil {
		t.unsure = false
	}
	vs.match = st.matchedBrackets(mode, time.Now().Add(bracketBudget))
	vs.matchKey, vs.matchOK = k, true
	if t != nil && t.unsure {
		// The tree did not reach the pairs yet: ask again next frame,
		// when it can go on.
		vs.matchOK = false
		if t.known() < (modelBrackets{st.Model}).finalLines(st.Snapshot()) {
			ui.Invalidate(w.c.Window())
		}
	}
	return vs.match
}

func binaryAppend(b []byte, vs ...int) []byte {
	for _, v := range vs {
		for k := 0; k < 8; k++ {
			b = append(b, byte(v>>(8*k)))
		}
	}
	return b
}

// matchRow is a matched bracket drawn on a row (for the redraw hints):
// the row's index in the laid out rows and the bracket's runes.
type matchRow struct{ k, r0, r1 int }

// paintMatch draws the boxes of the matched brackets on the laid out rows
// (Monaco's .bracket-match: the background and a one pixel border) and
// returns where it drew them.
func paintMatch(l *paint.List, snap buffer.Snapshot, rows []lineView, match []brange, tx, lh float32, lineY func(int) float32, bg, border color.NRGBA) []matchRow {
	if len(match) == 0 {
		return nil
	}
	var out []matchRow
	for k := range rows {
		lv := &rows[k]
		ls := snap.LineStart(lv.i)
		b0, b1 := lv.b0, lv.b1
		if lv.long != nil {
			b0, b1 = 0, len(lv.text)
		}
		for _, m := range match {
			if m.a < ls+b0 || m.a >= ls+b1 {
				continue
			}
			r0, r1 := lv.runeAt(m.a-ls), lv.runeAt(m.b-ls)
			if r0 >= r1 {
				continue
			}
			drawn := false
			for _, sg := range lv.segs {
				p0, p1 := max(r0, sg.r0), min(r1, sg.r1)
				if p0 >= p1 {
					continue
				}
				for _, sr := range sg.lay.SelectionRects(p0-sg.r0, p1-sg.r0) {
					box := rectXYWH(tx+sg.x0+sr.X, lineY(lv.vi), sr.W, lh)
					l.Fill(box, geom.Radii{}, bg)
					l.Stroke(geom.Rect{X: box.X + 0.5, Y: box.Y + 0.5, W: box.W - 1, H: box.H - 1}, geom.Radii{}, 1, border)
					drawn = true
				}
			}
			if drawn {
				out = append(out, matchRow{k, r0, r1})
			}
		}
	}
	return out
}
