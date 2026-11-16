package editor

import (
	"image/color"
	"slices"
	"time"

	"github.com/tangoui/tango/ui/editor/internal/frameclock"

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

// bracketCommandBudget bounds a bracket command (jumpToBracket,
// selectToBracket), which tokenizes and scans the lines it needs: a key
// press the user waits for, unlike the frame's work.
const bracketCommandBudget = 100 * time.Millisecond

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
	if toks, ok := hl.PeekCompleteTokens(snap, line, dst); ok {
		return toks
	}
	return hl.TokenizeSpan(snap, line, text, dst)
}

func (s modelBrackets) startState(snap buffer.Snapshot, line int) monarch.State {
	if hl := s.m.Highlighter(); hl != nil {
		st, _ := hl.StartState(snap, line)
		return st
	}
	return 0
}

// advance tokenizes with the Highlighter's default view, so the views'
// own token windows stay where they are.
func (s modelBrackets) advance(snap buffer.Snapshot, upTo int, deadline time.Time) {
	if hl := s.m.Highlighter(); hl != nil {
		hl.Highlight(snap, upTo, frameclock.Until(deadline))
	}
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
	gen := m.hl.GrammarGeneration()
	if m.brk == nil || m.brGrammar != gen {
		m.brGrammar = gen
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
	m.ind.edit(old, ns, edits) // the bracket guides' indentation index
}

// bracketTreeFor returns the tree with the lines up to line known when
// they can be within deadline, else nil. Queries (matching and the
// bracket commands) tokenize the lines they need themselves, within
// deadline (tree.tokenize): the default View starts no background
// Highlighter, so the lines past the visible ones may never be tokenized
// otherwise.
func (s *State) bracketTreeFor(line int, deadline time.Time) *bracketTree {
	t := s.Model.brackets()
	if t == nil {
		return nil
	}
	t.tokenize = true
	if t.known() <= line {
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
	// The brackets touching col (of a long line, only those: the scan
	// starts at the checkpoint before them).
	le := t.eventsIn(src, L, col-t.conf.maxLen, col+1, s.Model.line(snap, L))
	evs := le.evs
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
			p.close, ok = closeRange(t, src, snap, L, evs[k+1:], e.node, deadline, &le)
		} else {
			p.close = evRange(snap, L, e)
			p.open, ok = openRange(t, src, snap, L, evs[:k], e.node, &le)
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
// events before rest on line L): on that line, or on a later one. le, when
// not nil, is where rest comes from: a long line's scan paused before its
// end goes on in the line (closeInLine).
func pairClose(t *bracketTree, src bracketSource, snap buffer.Snapshot, L int, rest []bracketEvent, n *bnode, deadline time.Time, le *lineEvs) (int, brange) {
	for _, e := range rest {
		if e.node == n && (e.kind == evClose || e.kind == evUnclosed) {
			if e.kind == evUnclosed {
				return pairUnclosed, brange{}
			}
			return pairClosed, evRange(snap, L, e)
		}
	}
	if e, ok := t.closeInLine(le, n.depth); ok {
		if e.kind == evUnclosed {
			return pairUnclosed, brange{}
		}
		return pairClosed, evRange(snap, L, e)
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
func closeRange(t *bracketTree, src bracketSource, snap buffer.Snapshot, L int, rest []bracketEvent, n *bnode, deadline time.Time, le *lineEvs) (brange, bool) {
	st, r := pairClose(t, src, snap, L, rest, n, deadline, le)
	return r, st == pairClosed
}

// openRange finds the opening bracket of stack entry n, open before the
// events after before on line L. le, when not nil, is where before comes
// from: a long line's scan from a checkpoint goes back in the line
// (openInLine).
func openRange(t *bracketTree, src bracketSource, snap buffer.Snapshot, L int, before []bracketEvent, n *bnode, le *lineEvs) (brange, bool) {
	for k := len(before) - 1; k >= 0; k-- {
		if e := before[k]; e.kind == evOpen && e.node == n {
			return evRange(snap, L, e), true
		}
	}
	if e, ok := t.openInLine(le, n.depth); ok {
		return evRange(snap, L, e), true
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
	return s.enclosing(off, deadline, false)
}

// enclosing is enclosingBrackets; with unclosedStops an unclosed pair
// innermost around off gives none (Monaco's active bracket pair guide:
// the innermost pair, closed or not, must be closed; bracketguides.go).
func (s *State) enclosing(off int, deadline time.Time, unclosedStops bool) (bracketPair, bool) {
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
	le := t.eventsIn(src, L, col-t.conf.maxLen, col+1, s.Model.line(snap, L))
	evs := le.evs
	// The stack at off: the brackets that end before it.
	st := le.s0
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
		state, cl := pairClose(t, src, snap, L, evs[k:], y, deadline, &le)
		if state == pairUnclosed {
			if unclosedStops {
				return bracketPair{}, false
			}
			continue // the pair below
		}
		if state == pairUnknown {
			return bracketPair{}, false
		}
		op, ok := openRange(t, src, snap, L, evs[:k], y, &le)
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
		if src.finalLines(snap) <= j {
			src.advance(snap, j+64, deadline)
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
		if frameclock.Now().After(deadline) {
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
		if frameclock.Now().After(deadline) {
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

// beginBracketCommand starts a bracket command's work (its own budget).
func (s *State) beginBracketCommand() {
	if t := s.Model.brackets(); t != nil {
		t.beginFrame()
	}
}

// beginBracketFrame starts a paint's bracket work: matching the carets'
// brackets and coloring the lines' share the frame's one overrun (and its
// budget). A tree made later in the frame starts without one.
func (m *Model) beginBracketFrame() {
	if m.brk != nil {
		m.brk.beginFrame()
	}
	m.ind.beginFrame()
}

// jumpToBracket moves each caret to the bracket matching the one it
// touches, else to the closing bracket of the enclosing pair, else to the
// next bracket (Monaco's jumpToBracket).
func (s *State) jumpToBracket() {
	s.beginBracketCommand()
	deadline := frameclock.Now().Add(bracketCommandBudget)
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
	s.beginBracketCommand()
	deadline := frameclock.Now().Add(bracketCommandBudget)
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
	res := make([]caretMatch, len(s.sels()))
	s.matchCarets(mode, deadline, 0, res)
	return matchRanges(res)
}

// caretMatch is the match of one selection: decided once its pair, or
// that it has none, is known (a selection that is not a caret has none).
type caretMatch struct {
	done bool
	ok   bool
	p    bracketPair
}

// matchRanges returns the brackets of the decided matches of res, in
// selection order.
func matchRanges(res []caretMatch) []brange {
	var out []brange
	for _, m := range res {
		if m.done && m.ok {
			out = append(out, m.p.open, m.p.close)
		}
	}
	return out
}

// matchCarets decides the undecided matches of res (one per selection),
// querying from selection from on, round, until deadline: a frame whose
// budget runs out before every caret is decided keeps what it decided, and
// the next frame (the same text and selections) resumes at the first
// caret left undecided, which it returns (-1: all are decided). So carets
// with far pairs do not starve the others, and the decided ones are not
// queried again.
func (s *State) matchCarets(mode MatchBracketsMode, deadline time.Time, from int, res []caretMatch) (unsure int) {
	if mode == MatchBracketsNever || len(s.Sels) > maxMatchSelections {
		for i := range res {
			res[i] = caretMatch{done: true}
		}
		return -1
	}
	snap := s.Snapshot()
	t := s.Model.brackets()
	sels := s.sels()
	queried := 0
	for k := range sels {
		i := (from + k) % len(sels)
		x := sels[i]
		if res[i].done {
			continue
		}
		if x.Anchor != x.Head {
			res[i] = caretMatch{done: true}
			continue
		}
		if t != nil && queried > 0 && t.pastDeadline(deadline) {
			// The budget is spent (the first caret queried may use the
			// frame's overrun): this caret and the rest wait for the next
			// frame.
			break
		}
		queried++
		was := false
		if t != nil {
			was, t.unsure = t.unsure, false
		}
		pos := clampOff(snap, x.Head)
		p, ok := s.matchBracket(pos, deadline)
		if !ok && mode == MatchBracketsAlways {
			p, ok = s.enclosingBrackets(pos, deadline)
		}
		if t == nil || !t.unsure {
			res[i] = caretMatch{done: true, ok: ok, p: p}
		}
		if t != nil {
			t.unsure = t.unsure || was
		}
	}
	for k := range sels {
		if i := (from + k) % len(sels); !res[i].done {
			if t != nil {
				t.unsure = true
			}
			return i
		}
	}
	return -1
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
// known yet get none. done, when not nil, receives each line whose
// colors are final for this text (every opening bracket's state known),
// to be cached.
func (s *State) bracketColors(t *bracketTree, lines []int, pal bracketPalette, deadline time.Time, done func(line int, spans []brSpan)) (map[int][]brSpan, bool) {
	return s.bracketColorsIn(t, lines, nil, pal, deadline, done)
}

// bracketColorsIn is bracketColors where the long lines (no tokens) in
// wins only need the brackets in their byte ranges wins[i] (the rows in
// view, a sticky header's chunks; each scanned on its own): those are
// scanned from the line's checkpoint before them, their
// pairs followed by the line's checkpoints (brackets_long.go), and none of
// them goes to done (the colors of a part of a line).
func (s *State) bracketColorsIn(t *bracketTree, lines []int, wins map[int][][2]int, pal bracketPalette, deadline time.Time, done func(line int, spans []brSpan)) (map[int][]brSpan, bool) {
	src := modelBrackets{s.Model}
	snap := s.Snapshot()
	hl := s.Highlighter()
	out := map[int][]brSpan{}
	sure := true
	var evs []bracketEvent
	var toks []monarch.Token
	for _, i := range lines {
		if i >= t.known() {
			sure = false
			continue
		}
		// The line's tokens are final (it is known): those of the view's
		// token window, which layout uses too.
		text := s.Model.line(snap, i)
		toks = toks[:0]
		if hl != nil && len(text) <= maxTokenizationLineLength {
			toks, _ = hl.TokensFor(s.hlKey, snap, i, toks)
		}
		lineSure := true
		var spans []brSpan
		// color appends the spans of the brackets of evs (from le, nil for
		// the whole line) inside win (when part).
		color := func(evs []bracketEvent, le *lineEvs, win [2]int, part bool) {
			for k, e := range evs {
				if part && (e.b1 <= win[0] || e.b0 >= win[1]) {
					continue
				}
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
						if ce, ok := t.closeInLine(le, e.node.depth); ok {
							inLine = true
							if ce.kind == evUnclosed {
								state = 1
							} else if !ce.def.closesColorized(int(e.node.open)) {
								colorized = false
							}
						}
					}
					if !inLine {
						_, ce, found, never := t.closeOf(src, i, e.node.depth, deadline)
						switch {
						case found && ce.kind == evUnclosed, !found && never:
							state = 1
						case !found:
							state = 2
							sure, lineSure = false, false
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
		}
		ws, part := wins[i]
		if part && len(text) > maxTokenizationLineLength && t.longIndex(i) != nil {
			// Each window scanned on its own (a sticky header's chunks and
			// the rows in view may be far apart: never the bytes between).
			for _, win := range ws {
				x := t.eventsIn(src, i, win[0]-t.conf.maxLen, win[1], text)
				color(x.evs, &x, win, true)
			}
			if len(ws) > 1 {
				slices.SortFunc(spans, func(a, b brSpan) int { return a.b0 - b.b0 })
				spans = slices.CompactFunc(spans, func(a, b brSpan) bool { return a.b0 == b.b0 })
			}
		} else {
			part = false
			evs = evs[:0]
			t.conf.scanLine(t.start[i], text, toks, func(e bracketEvent) { evs = append(evs, e) })
			color(evs, nil, [2]int{}, false)
		}
		if len(spans) > 0 {
			out[i] = spans
		}
		if lineSure && done != nil && !part {
			done(i, spans)
		}
	}
	return out, sure
}

// View side.

// bracketFrameBudget bounds the bracket work of one frame: scanning the
// tree for the colors and matching the carets' brackets share it, so an
// edit that unbalances the brackets below costs each frame at most this
// until the tree has caught up.
var bracketFrameBudget = 1500 * time.Microsecond // a variable for tests

// brCache holds the final bracket colors of lines of one text and theme.
type brCache struct {
	tree  *bracketTree // a language change makes a new tree
	buf   *buffer.Buffer
	ver   uint64
	th    *theme.Theme
	lines map[int][]brSpan
}

// bracketSpans returns the colored brackets of the model lines of view
// lines lo..hi (mhi is the last one's model line), nil when bracket pair
// colorization is off for the document (Options, too
// large for a tree). It scans the tree up to the lines until deadline;
// lines whose colors are final are cached until the text or the theme
// changes. When brackets stay undecided and the tree can go on (more
// final lines), the next frame is requested.
func (w *ViewW) bracketSpans(snap buffer.Snapshot, vl *viewLines, lo, hi, mhi int, th *theme.Theme, deadline time.Time) map[int][]brSpan {
	st, vs := w.st, w.vs
	if !w.opts.BracketPairColorization {
		return nil
	}
	t := st.Model.brackets()
	if t == nil || len(t.conf.defs) == 0 {
		return nil
	}
	c := &vs.brc
	if c.tree != t || c.buf != st.Model.Buffer() || c.ver != snap.Version() || c.th != th || c.lines == nil {
		*c = brCache{tree: t, buf: st.Model.Buffer(), ver: snap.Version(), th: th, lines: map[int][]brSpan{}}
	}
	out := map[int][]brSpan{}
	var lines []int
	// The bytes of the rows in view of the wrapped long lines: only their
	// brackets are colored (bracketColorsIn).
	var wins map[int][][2]int
	for v := lo; v <= hi; v++ {
		i, row := vl.find(v)
		if row < 0 || row >= vl.rowsOf(i) {
			continue // a view zone's row (zones.go)
		}
		if lb := vl.breaks[i]; vl.on() && lb != nil {
			if n := lineEnd(snap, i) - snap.LineStart(i); n > maxTokenizationLineLength {
				b0, b1 := lb.rowRange(row, n)
				if wins == nil {
					wins = map[int][][2]int{}
				}
				// The rows in view are contiguous: one window.
				if r, ok := wins[i]; ok {
					b0, b1 = min(b0, r[0][0]), max(b1, r[0][1])
				}
				wins[i] = [][2]int{{b0, b1}}
			}
		}
		if len(lines) > 0 && lines[len(lines)-1] == i {
			continue
		}
		if sp, ok := c.lines[i]; ok {
			if len(sp) > 0 {
				out[i] = sp
			}
			continue
		}
		lines = append(lines, i)
	}
	// The sticky headers (prepareSticky) are colored too, a long one in
	// the chunks it draws (layoutSticky: layoutChunks from the horizontal
	// scroll), whether or not its line is in view.
	for _, i := range vs.stickyLines {
		inList := slices.Contains(lines, i)
		if sp, ok := c.lines[i]; ok && !inList {
			if len(sp) > 0 {
				out[i] = sp
			}
			continue
		}
		if s := st.Model.line(snap, i); len(s) > maxTokenizationLineLength && isLong(s) {
			if r, windowed := wins[i]; windowed || !inList {
				// The chunks as a window of their own: merged with the rows
				// in view, the window would span the bytes between them
				// (the line's start to the rows), as long as the line.
				ll := w.longLineOf(snap, i, s)
				win := [2]int{} // none drawn
				if k0, k1 := w.chunkWindow(ll, vs.colW); k0 >= 0 {
					win = [2]int{ll.bytes[k0], ll.bytes[k1+1]}
				}
				if wins == nil {
					wins = map[int][][2]int{}
				}
				wins[i] = append(r, win)
			}
		}
		if !inList {
			lines = append(lines, i)
		}
	}
	if len(lines) == 0 {
		return out
	}
	src := modelBrackets{st.Model}
	t.tokenize = false // colors follow the Highlighter (bracketTreeFor)
	t.ensure(src, mhi+1, deadline)
	spans, sure := st.bracketColorsIn(t, lines, wins, paletteOf(th), deadline, func(i int, sp []brSpan) { c.lines[i] = sp })
	for i, sp := range spans {
		out[i] = sp
	}
	if !sure && !t.complete() && t.known() < src.finalLines(snap) {
		ui.Invalidate(w.c.Window())
	}
	return out
}

// matchKey is what the matched brackets of a frame depend on.
type matchKey struct {
	grammar uint64       // invalidates even cached definitive negative matches
	conf    *bracketConf // the language's brackets
	buf     *buffer.Buffer
	ver     uint64
	mode    MatchBracketsMode
	sels    string
}

// maxMatchFrames bounds the frames one match (a text and its carets)
// requests while the tree catches up: at bracketFrameBudget each, about
// 1.5 s of work.
const maxMatchFrames = 1000

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
func (w *ViewW) matched(caretShown bool, deadline time.Time) []brange {
	st, vs := w.st, w.vs
	mode := w.opts.matchBrackets()
	if mode == MatchBracketsNever {
		return nil
	}
	var sb []byte
	for _, x := range st.sels() {
		sb = binaryAppend(sb, x.Anchor, x.Head)
	}
	k := matchKey{st.Model.hl.GrammarGeneration(), bracketConfOf(st.Model.Conf()), st.Model.Buffer(), st.Snapshot().Version(), mode, string(sb)}
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
	if len(st.Sels) > maxMatchSelections {
		vs.match, vs.matchKey, vs.matchOK = nil, k, true
		return nil
	}
	if vs.matchKey != k {
		// Another text or selections: the matches start over.
		vs.matchKey, vs.matchOK, vs.matchTries, vs.matchFrom = k, false, 0, 0
		vs.matchRes = vs.matchRes[:0]
	}
	if n := len(st.sels()); len(vs.matchRes) != n {
		vs.matchRes = append(vs.matchRes[:0], make([]caretMatch, n)...)
	}
	if t != nil {
		t.unsure = false
	} else {
		// The local search, debounced above, has a budget of its own.
		deadline = frameclock.Now().Add(bracketBudget)
	}
	unsure := st.matchCarets(mode, deadline, vs.matchFrom, vs.matchRes)
	// Only decided matches are shown, and they stay decided for the key:
	// the shown set grows frame by frame until every caret is decided.
	vs.match = matchRanges(vs.matchRes)
	if unsure < 0 {
		vs.matchOK = true
	} else {
		// Some caret's pair is not decided yet (the tree or the
		// tokenization did not reach it, or the budget ran out): ask again
		// next frame, starting with that caret, for at most
		// maxMatchFrames frames.
		vs.matchFrom = unsure
		if vs.matchTries++; vs.matchTries <= maxMatchFrames {
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
				for _, sr := range sg.lay.SelectionRects(sg.local(p0), sg.local(p1)) {
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
