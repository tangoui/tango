package editor

import (
	"regexp"
	"slices"
	"strings"

	"github.com/tangoui/tango/ui/editor/buffer"
)

// Multiple cursors (Monaco's cursor collection).
//
// State.Sels holds every selection, the primary first (Monaco's
// getSelections()[0]); the others keep the order they were added in.
// Whenever a command sets the selections they are merged as Monaco's
// CursorCollection.normalize does (multiCursorMergeOverlapping): sorted by
// start, two selections merge when they overlap, or touch while one of them
// is empty; the merged selection takes the place, and the direction, of the
// one added first (the primary always survives).
//
// Every edit and move command acts on every cursor. An edit is one set of
// buffer edits (one undo step, all the cursors recorded in the history);
// the selections after it are computed in one pass over the edits sorted by
// position, so typing at n cursors costs O(n log n), not O(n × document).
// Selections of other views are mapped through an edit the same way (see
// mapSels).

// cursorHistoryMax bounds the cursor undo stack (Monaco's
// CursorUndoRedoController keeps 50).
const cursorHistoryMax = 50

// Primary returns the primary selection, Sels[0] (Monaco's getSelection()).
func (s *State) Primary() Selection {
	if len(s.Sels) == 0 {
		return Selection{}
	}
	return s.Sels[0]
}

// sels returns the selections, never empty (a State whose Sels were
// emptied gets a caret at the start).
func (s *State) sels() []Selection {
	if len(s.Sels) == 0 {
		s.Sels = []Selection{{}}
	}
	return s.Sels
}

// SetSelections sets the selections, the primary first: each is clamped
// to the document and onto character boundaries, and overlapping (or
// touching, when one is empty) selections merge as in Monaco. A change of
// selection ends the current undo step and is recorded for cursorUndo. An
// empty sels does nothing.
func (s *State) SetSelections(sels []Selection) {
	if len(sels) == 0 {
		return
	}
	snap := s.Snapshot()
	out := make([]Selection, len(sels))
	for i, x := range sels {
		out[i] = clampSel(snap, x)
	}
	s.setSels(out, nil)
}

// setSels sets the selections after a selection-only change (a move, a
// click, a selection command): it merges them, ends the undo step and
// records the previous selections for cursorUndo when they changed, and
// keeps goals (goal columns of vertical moves, parallel to sels) or resets
// them when nil.
func (s *State) setSels(sels []Selection, goals []int) {
	// The last added cursor (Monaco's lastAddedCursorIndex): a change that
	// adds cursors adds them at the end.
	last := s.lastAdded
	if len(sels) > len(s.Sels) {
		last = len(sels) - 1
	}
	if last >= len(sels) || len(sels) == 1 {
		last = len(sels) - 1
	}
	sels, kept, last := mergeSelsLast(sels, last)
	if goals != nil && kept != nil {
		g := make([]int, len(kept))
		for i, k := range kept {
			g[i] = goals[k]
		}
		goals = g
	}
	if !slices.Equal(sels, s.Sels) {
		s.Model.buf.PushUndoStop()
		s.lastKind = kindNone
		s.recordCursor()
	}
	s.Sels = sels
	s.goals = goals
	s.ups = caretAffinity{}
	s.lastAdded = last
	s.clampLast()
	s.validateAutoClosed()
	s.revealFolds()
}

// ordered returns sel as an ordered range.
func ordered(sel Selection) (int, int) {
	return min(sel.Anchor, sel.Head), max(sel.Anchor, sel.Head)
}

// apart reports whether p and q, p starting first (by start, then end),
// stay separate cursors: Monaco merges overlapping selections, and
// touching ones when either is empty.
func apart(pa, pb, qa, qb int) bool {
	if pa > qa || (pa == qa && pb > qb) {
		return false // not in order
	}
	if pa == pb || qa == qb {
		return qa > pb
	}
	return qa >= pb
}

// mergeSels is mergeSelsLast with no last added cursor.
func mergeSels(sels []Selection) ([]Selection, []int) {
	out, kept, _ := mergeSelsLast(sels, -1)
	return out, kept
}

// mergeSelsLast merges overlapping selections as Monaco's
// CursorCollection.normalize: kept are the indices into sels of the
// selections returned (nil when nothing merged and sels is returned as
// is). The result keeps the order of sels; a merged selection covers the
// union of its parts and sits at the place of the part that came first in
// sels. It takes that part's direction, except that when the other part is
// the last added cursor (index last, -1 for none) it takes the direction
// of the last added one, which then lives on in the merged selection (an
// Alt+drag over another cursor keeps its caret at the pointer). The
// returned last is the index in the result of the last added cursor.
func mergeSelsLast(sels []Selection, last int) ([]Selection, []int, int) {
	n := len(sels)
	if n <= 1 {
		return sels, nil, last
	}
	sorted := true
	pa, pb := ordered(sels[0])
	for i := 1; i < n; i++ {
		qa, qb := ordered(sels[i])
		if !apart(pa, pb, qa, qb) {
			sorted = false
			break
		}
		pa, pb = qa, qb
	}
	if sorted {
		return sels, nil, last // in document order and apart: the usual case
	}
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	slices.SortFunc(idx, func(x, y int) int {
		xa, xb := ordered(sels[x])
		ya, yb := ordered(sels[y])
		if xa != ya {
			return xa - ya
		}
		if xb != yb {
			return xb - yb
		}
		return x - y
	})
	type group struct {
		a, b int
		win  int // index in sels of the surviving selection
		sel  Selection
	}
	groups := make([]group, 0, n)
	for _, i := range idx {
		qa, qb := ordered(sels[i])
		if m := len(groups); m > 0 {
			g := &groups[m-1]
			empty := g.a == g.b || qa == qb
			if (empty && qa <= g.b) || (!empty && qa < g.b) {
				win, ws, lose, ls := g.win, g.sel, i, sels[i]
				if i < win {
					win, ws, lose, ls = i, sels[i], g.win, g.sel
				}
				dir := ws // the winner takes it all...
				if lose == last {
					dir, last = ls, win // ...unless the loser was added last
				}
				a, b := min(g.a, qa), max(g.b, qb)
				g.a, g.b, g.win = a, b, win
				if dir.Anchor <= dir.Head {
					g.sel = Selection{Anchor: a, Head: b}
				} else {
					g.sel = Selection{Anchor: b, Head: a}
				}
				continue
			}
		}
		groups = append(groups, group{a: qa, b: qb, win: i, sel: sels[i]})
	}
	slices.SortFunc(groups, func(x, y group) int { return x.win - y.win })
	out := make([]Selection, len(groups))
	kept := make([]int, len(groups))
	lastOut := -1
	for i, g := range groups {
		out[i], kept[i] = g.sel, g.win
		if g.win == last {
			lastOut = i
		}
	}
	return out, kept, lastOut
}

// storedLen is the length text has once stored (buffer.Snapshot.Apply
// stores "\r\n" as one byte).
func storedLen(text string) int { return len(text) - strings.Count(text, "\r\n") }

// mapSels maps every selection through edits (sorted, non-overlapping, in
// the old text) as mapOffset does, in one pass over the offsets sorted by
// position: O((selections + edits) log), not O(selections × edits).
func mapSels(edits []buffer.Edit, sels []Selection) []Selection {
	out := make([]Selection, len(sels))
	if len(edits) == 0 {
		copy(out, sels)
		return out
	}
	type ref struct{ off, slot int }
	refs := make([]ref, 0, 2*len(sels))
	for i, x := range sels {
		refs = append(refs, ref{x.Anchor, 2 * i}, ref{x.Head, 2*i + 1})
	}
	cmp := func(x, y ref) int { return x.off - y.off }
	if !slices.IsSortedFunc(refs, cmp) {
		slices.SortFunc(refs, cmp)
	}
	j, delta := 0, 0
	for _, r := range refs {
		res := -1
		for j < len(edits) {
			e := edits[j]
			if e.Start == e.End && e.Text == "" {
				j++
				continue
			}
			if r.off < e.Start {
				break
			}
			if e.Start != e.End && r.off < e.End {
				res = e.Start + delta // inside a deleted or replaced range
				break
			}
			delta += storedLen(e.Text) - (e.End - e.Start)
			j++
		}
		if res < 0 {
			res = r.off + delta
		}
		if r.slot&1 == 0 {
			out[r.slot/2].Anchor = res
		} else {
			out[r.slot/2].Head = res
		}
	}
	return out
}

// mapSelsSince maps selections through the changes since.
func mapSelsSince(sels []Selection, since [][]buffer.Edit) []Selection {
	for _, c := range since {
		sels = mapSels(c, sels)
	}
	return sels
}

// clampSels clamps sels into snap: onto character boundaries when exact,
// else only into the document (offsets the State computed itself).
func clampSels(snap buffer.Snapshot, sels []Selection, exact bool) []Selection {
	n := snap.Len()
	out := sels
	for i, x := range sels {
		y := x
		if exact {
			y = clampSel(snap, x)
		} else {
			y = Selection{Anchor: max(0, min(x.Anchor, n)), Head: max(0, min(x.Head, n))}
		}
		if y != x {
			if &out[0] == &sels[0] {
				out = slices.Clone(sels)
			}
			out[i] = y
		}
	}
	return out
}

// cursorEdit is the edit of one cursor: [a, b) is replaced by text and the
// caret goes after the text.
type cursorEdit struct {
	a, b int
	text string
}

// applyCursorEdits applies ces, one per selection (ces[k] for Sels[k]),
// as one buffer edit set. Overlapping ranges (two carets deleting the same
// character) merge into one edit whose carets all end after it; the
// resulting carets are merged as Monaco merges cursors.
//
// Deviation from Monaco: overlapping ranges take their union (every
// cursor's delete happens). Monaco's edit operation builder
// (_getLoserCursorMap) instead drops the operations of the cursor with the
// higher index wherever two cursors' ranges overlap, so it can delete less:
// when the ranges overlap partly (word deletes from two carets in one
// word), and when one range contains another and the larger belongs to the
// higher-index cursor. In "abcdef" with the primary caret at 2 and a
// secondary at 5, deleteWordLeft deletes [0,2) and [0,5): Monaco keeps
// only the primary's and gives "cdef", this gives "f".
func (s *State) applyCursorEdits(ces []cursorEdit, kind editKind) error {
	n := len(ces)
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	inOrder := func(x, y int) int {
		if ces[x].a != ces[y].a {
			return ces[x].a - ces[y].a
		}
		if ces[x].b != ces[y].b {
			return ces[x].b - ces[y].b
		}
		return x - y
	}
	if !slices.IsSortedFunc(order, inOrder) {
		slices.SortFunc(order, inOrder)
	}
	edits := make([]buffer.Edit, 0, n)
	group := make([]int, n) // edit of each cursor
	noop := false
	for _, k := range order {
		c := ces[k]
		if m := len(edits); m > 0 {
			p := &edits[m-1]
			if c.a < p.End || (c.a == p.Start && p.Start == p.End) {
				p.End = max(p.End, c.b)
				p.Text += c.text
				group[k] = m - 1
				continue
			}
		}
		if c.a == c.b && c.text == "" {
			noop = true
		}
		edits = append(edits, buffer.Edit{Start: c.a, End: c.b, Text: c.text})
		group[k] = len(edits) - 1
	}
	// The end of each edit in the new text.
	ends := make([]int, len(edits))
	delta := 0
	for i, e := range edits {
		ends[i] = e.Start + delta + storedLen(e.Text)
		delta += storedLen(e.Text) - (e.End - e.Start)
	}
	after := make([]Selection, n)
	for k := range ces {
		c := ends[group[k]]
		after[k] = Selection{Anchor: c, Head: c}
	}
	after, _, last := mergeSelsLast(after, s.lastAdded)
	if noop {
		edits = slices.DeleteFunc(edits, func(e buffer.Edit) bool { return e.Start == e.End && e.Text == "" })
	}
	if err := s.edit(edits, kind, after, false); err != nil {
		return err
	}
	s.lastAdded = last
	s.clampLast()
	return nil
}

// clampLast keeps lastAdded a valid index of Sels after any change of the
// selections (0, the primary, means none: the primary never loses a
// merge). Paths that keep the selections' order remap it through their
// merge instead (setSels, normalize, applyCursorEdits).
func (s *State) clampLast() {
	s.lastAdded = max(0, min(s.lastAdded, len(s.Sels)-1))
}

// replaceSels replaces every selection with text(k, a, b), the text for
// selection k spanning [a, b), as one edit set.
func (s *State) replaceSels(kind editKind, text func(k, a, b int) string) error {
	s.normalize()
	snap := s.Snapshot()
	sels := s.Sels
	ces := make([]cursorEdit, len(sels))
	for k, sel := range sels {
		a, b := ordered(Selection{Anchor: clampOff(snap, sel.Anchor), Head: clampOff(snap, sel.Head)})
		ces[k] = cursorEdit{a: a, b: b, text: text(k, a, b)}
	}
	return s.applyCursorEdits(ces, kind)
}

// normalize merges the selections in place (Sels may have been set
// directly) without recording a selection change.
func (s *State) normalize() {
	sels, kept, last := mergeSelsLast(s.sels(), s.lastAdded)
	if kept != nil {
		s.Sels, s.goals, s.lastAdded = sels, nil, last
	}
	s.clampLast()
}

// allEmpty reports whether every selection is a caret.
func (s *State) allEmpty() bool {
	for _, x := range s.sels() {
		if x.Anchor != x.Head {
			return false
		}
	}
	return true
}

// --- cursor undo (Monaco's CursorUndoRedoController) ---

// cursorStep is a cursor history entry: the selections and the scroll
// position.
type cursorStep struct {
	sels          []Selection
	top           int
	offY, scrollX float32
}

func (s *State) cursorStepNow() cursorStep {
	return cursorStep{sels: slices.Clone(s.sels()), top: s.Viewport.TopLine, offY: s.Viewport.OffsetY, scrollX: s.Viewport.ScrollX}
}

// recordCursor pushes the current selections on the cursor undo stack
// before a selection-only change (unless they are its top already), and
// clears the redo stack.
func (s *State) recordCursor() {
	if s.cursorRestoring || len(s.Sels) == 0 {
		return
	}
	if n := len(s.curUndo); n > 0 && slices.Equal(s.curUndo[n-1].sels, s.Sels) {
		return
	}
	s.curUndo = append(s.curUndo, s.cursorStepNow())
	s.curRedo = nil
	if len(s.curUndo) > cursorHistoryMax {
		s.curUndo = slices.Delete(s.curUndo, 0, 1)
	}
}

// clearCursorHistory forgets the cursor history: any change of the text
// does (Monaco clears it on onDidChangeModelContent).
func (s *State) clearCursorHistory() { s.curUndo, s.curRedo = nil, nil }

// CursorUndo restores the selections (and scroll position) from before
// the last selection change (Monaco's cursorUndo); it reports false when
// there is none. Edits clear the cursor history.
func (s *State) CursorUndo() bool { return s.cursorHistory(false) }

// CursorRedo re-applies the selections CursorUndo undid (cursorRedo).
func (s *State) CursorRedo() bool { return s.cursorHistory(true) }

func (s *State) cursorHistory(redo bool) bool {
	from, to := &s.curUndo, &s.curRedo
	if redo {
		from, to = to, from
	}
	if len(*from) == 0 {
		return false
	}
	st := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	*to = append(*to, s.cursorStepNow())
	s.cursorRestoring = true
	s.SetSelections(st.sels)
	s.cursorRestoring = false
	v := &s.Viewport
	v.TopLine, v.OffsetY, v.ScrollX = st.top, st.offY, st.scrollX
	s.clampViewport()
	v.clampX()
	return true
}

// --- adding and removing cursors ---

// RemoveSecondaryCursors keeps only the primary selection (Esc).
func (s *State) RemoveSecondaryCursors() bool {
	if len(s.Sels) <= 1 {
		return false
	}
	s.setSels([]Selection{s.Sels[0]}, nil)
	return true
}

// AddCursor adds a caret at off (Alt+click); while there are other
// cursors, a selection containing off (ends included) is removed instead
// (Monaco's createCursor).
func (s *State) AddCursor(off int) {
	snap := s.Snapshot()
	off = snap.OffsetOf(snap.PosOf(off))
	sels := s.sels()
	if len(sels) > 1 {
		for i, x := range sels {
			if a, b := ordered(x); a <= off && off <= b {
				s.setSels(slices.Delete(slices.Clone(sels), i, i+1), nil)
				return
			}
		}
	}
	s.setSels(append(slices.Clone(sels), Selection{Anchor: off, Head: off}), nil)
}

// insertCursorVertical adds, for every cursor, a copy of it one line up
// (Backward) or down (Monaco's addCursorUp / addCursorDown with logical
// lines, MoveOperations.translateUp/Down): both ends keep their visual
// columns, the caret its goal column (a cursor added from a short line
// goes back to the column it came from on a longer one), and the new
// cursor inherits the goal. On the first (last) line the copy stays where
// it is (Monaco's allowMoveOnEdgeLine false) and merges away.
func (s *State) insertCursorVertical(dir Direction) {
	snap := s.Snapshot()
	sels := s.sels()
	goals := s.goals
	if len(goals) != len(sels) {
		goals = make([]int, len(sels))
		for k, x := range sels {
			goals[k] = s.column(snap, clampOff(snap, x.Head))
		}
	}
	move := func(off, goal int) int {
		l := snap.LineOf(off)
		if (dir == Backward && l == 0) || (dir == Forward && l+1 >= snap.LineCount()) {
			return off
		}
		return s.verticalModel(snap, off, dir, 1, goal)
	}
	out := make([]Selection, 0, 2*len(sels))
	outGoals := make([]int, 0, 2*len(sels))
	for k, x := range sels {
		a, h := clampOff(snap, x.Anchor), clampOff(snap, x.Head)
		nh := move(h, goals[k])
		na := nh
		if a != h {
			na = move(a, -1)
		}
		out = append(out, x, Selection{Anchor: na, Head: nh})
		outGoals = append(outGoals, goals[k], goals[k])
	}
	s.setSels(out, outGoals)
}

// insertCursorsAtLineEnds puts a caret at the end of every line each
// selection covers (Monaco's insertCursorAtEndOfEachLineSelected): every
// line but the last at its end, the last at the selection end unless that
// is at column 0. Empty selections add nothing.
func (s *State) insertCursorsAtLineEnds() bool {
	snap := s.Snapshot()
	var out []Selection
	for _, x := range s.sels() {
		a, b := ordered(x)
		a, b = clampOff(snap, a), clampOff(snap, b)
		if a == b {
			continue
		}
		la, lb := snap.LineOf(a), snap.LineOf(b)
		for l := la; l < lb; l++ {
			e := lineEnd(snap, l)
			out = append(out, Selection{Anchor: e, Head: e})
		}
		if b > snap.LineStart(lb) {
			out = append(out, Selection{Anchor: b, Head: b})
		}
	}
	if len(out) == 0 {
		return false
	}
	s.setSels(out, nil)
	return true
}

// --- column selection ---

// colSelect is the box of the last column selection (Monaco's
// columnSelectData), valid while the selections are the ones it made.
type colSelect struct {
	fromLine, fromCol, toLine, toCol int
	sels                             []Selection
	ver                              uint64
}

// columnData returns the current column selection box: the last one while
// its selections are current, else the primary selection's anchor and
// head (lines and visual columns).
func (s *State) columnData() colSelect {
	snap := s.Snapshot()
	if c := s.colSel; c != nil && c.ver == snap.Version() && slices.Equal(c.sels, s.Sels) {
		return *c
	}
	p := s.Primary()
	a, h := clampOff(snap, p.Anchor), clampOff(snap, p.Head)
	return colSelect{
		fromLine: snap.LineOf(a), fromCol: s.column(snap, a),
		toLine: snap.LineOf(h), toCol: s.column(snap, h),
	}
}

// ColumnSelect selects the box from visual column fromCol of line
// fromLine to visual column toCol of line toLine (Monaco's column
// selection, Shift+Alt+drag): one selection per line, from the character
// boundary nearest fromCol to the one nearest toCol, the line fromLine
// primary. Visual columns count as vertical moves do (tabs to the next tab
// stop, wide characters two columns), the column grid the View draws on.
// Lines too short to reach into the box get no selection; when no line
// does, every line gets a caret at its end.
func (s *State) ColumnSelect(fromLine, fromCol, toLine, toCol int) {
	snap := s.Snapshot()
	last := snap.LineCount() - 1
	fromLine, toLine = max(0, min(fromLine, last)), max(0, min(toLine, last))
	fromCol, toCol = max(0, fromCol), max(0, toCol)
	step := 1
	if toLine < fromLine {
		step = -1
	}
	ltr, rtl := fromCol < toCol, fromCol > toCol
	var out []Selection
	for l := fromLine; ; l += step {
		text, ls := snap.Line(l), snap.LineStart(l)
		sc, ec := s.nearestColumn(text, fromCol), s.nearestColumn(text, toCol)
		vs, ve := s.lineColumn(text, sc), s.lineColumn(text, ec)
		skip := (ltr && (vs > toCol || ve < fromCol)) || (rtl && (ve > fromCol || vs < toCol))
		if !skip {
			out = append(out, Selection{Anchor: ls + sc, Head: ls + ec})
		}
		if l == toLine {
			break
		}
	}
	if len(out) == 0 {
		for l := fromLine; ; l += step {
			e := lineEnd(snap, l)
			out = append(out, Selection{Anchor: e, Head: e})
			if l == toLine {
				break
			}
		}
	}
	s.setSels(out, nil)
	s.colSel = &colSelect{fromLine, fromCol, toLine, toCol, slices.Clone(s.Sels), snap.Version()}
}

// columnSelectMove extends the column selection box by one column left
// or right, or by n lines up or down (Monaco's columnSelectLeft/Right/
// Up/Down).
func (s *State) columnSelectMove(dir Direction, vertical bool, n int) {
	d := s.columnData()
	snap := s.Snapshot()
	switch {
	case vertical && dir == Backward:
		d.toLine = max(0, d.toLine-n)
	case vertical:
		d.toLine = min(snap.LineCount()-1, d.toLine+n)
	case dir == Backward:
		if d.toCol > 0 {
			d.toCol--
		}
	default:
		maxCol := 0
		for l := min(d.fromLine, d.toLine); l <= max(d.fromLine, d.toLine); l++ {
			text := snap.Line(l)
			maxCol = max(maxCol, s.lineColumn(text, len(text)))
		}
		if d.toCol < maxCol {
			d.toCol++
		}
	}
	s.ColumnSelect(d.fromLine, d.fromCol, d.toLine, d.toCol)
}

// nearestColumn returns the byte column of the cluster boundary of text
// nearest visual column goal, the earlier one on a tie (Monaco's
// columnFromVisibleColumn).
func (s *State) nearestColumn(text string, goal int) int {
	if goal <= 0 {
		return 0
	}
	col := 0
	for i := 0; i < len(text); {
		j := nextGrapheme(text, i)
		next := s.advance(col, text[i:j])
		if next >= goal {
			if next-goal < goal-col {
				return j
			}
			return i
		}
		col, i = next, j
	}
	return len(text)
}

// --- find matches (Monaco's MultiCursorSession) ---

// findSession is a run of addSelectionToNextFindMatch: what it searches
// for, and the selections it last set (another selection, or an edit,
// starts a new session).
type findSession struct {
	text    string
	whole   bool           // whole words only
	re      *regexp.Regexp // case-insensitive matcher; nil: match case
	pending *Selection     // the word under the caret, the first "match"
	sels    []Selection
	ver     uint64
}

// wordSeparators is Monaco's default wordSeparators option.
const wordSeparators = "`~!@#$%^&*()-=+[{]}\\|;:'\",.<>/?"

// regularByte reports whether b is a word character for Monaco's
// configured words (not a separator, blank or line break; bytes of
// non-ASCII characters are word characters).
func regularByte(b byte) bool {
	return b >= 0x80 || (b != ' ' && b != '\t' && b != '\n' && b != '\r' && strings.IndexByte(wordSeparators, b) < 0)
}

// configuredWordAt returns the word touching off by the wordSeparators
// option (Monaco's getConfiguredWordAtPosition), false when there is
// none.
func configuredWordAt(snap buffer.Snapshot, off int) (int, int, bool) {
	l := snap.LineOf(off)
	ls := snap.LineStart(l)
	text := snap.Line(l)
	a, b := off-ls, off-ls
	for a > 0 && regularByte(text[a-1]) {
		a--
	}
	for b < len(text) && regularByte(text[b]) {
		b++
	}
	if a == b {
		return 0, 0, false
	}
	return ls + a, ls + b, true
}

// wholeWordMatch reports whether the match [a, b) of text is a whole word
// (Monaco's isValidMatch with wordSeparators).
func wholeWordMatch(text string, a, b int) bool {
	left := a == 0 || !regularByte(text[a-1]) || (b > a && !regularByte(text[a]))
	right := b == len(text) || !regularByte(text[b]) || (b > a && !regularByte(text[b-1]))
	return left && right
}

// findSessionNow returns the current find session, starting one from the
// primary selection when there is none: its text, or the word under an
// empty primary caret. As in Monaco's MultiCursorSession, a session
// started from a single empty caret matches case-sensitively and whole
// words only (the "disconnected from find" case); any other uses the find
// options, whose defaults are neither (case-insensitive, any substring;
// the editor has no find widget yet to change them).
func (s *State) findSessionNow() *findSession {
	snap := s.Snapshot()
	if f := s.find; f != nil && f.ver == snap.Version() && slices.Equal(f.sels, s.Sels) {
		return f
	}
	s.find = nil
	sels := s.sels()
	p := sels[0]
	f := &findSession{whole: len(sels) == 1 && p.Anchor == p.Head}
	if p.Anchor == p.Head {
		a, b, ok := configuredWordAt(snap, clampOff(snap, p.Head))
		if !ok {
			return nil
		}
		f.text = snap.Slice(a, b)
		f.pending = &Selection{Anchor: a, Head: b}
	} else {
		a, b := ordered(p)
		f.text = snap.Slice(clampOff(snap, a), clampOff(snap, b))
		if f.text == "" {
			return nil
		}
	}
	if !f.whole {
		f.re = regexp.MustCompile("(?i)" + regexp.QuoteMeta(f.text))
	}
	s.find = f
	return f
}

// index returns the first match of f's text in text at or after lo, by
// its case rule but not the whole-word rule; a < 0 when there is none.
func (f *findSession) index(text string, lo int) (a, b int) {
	if f.re == nil {
		i := strings.Index(text[lo:], f.text)
		if i < 0 {
			return -1, -1
		}
		return lo + i, lo + i + len(f.text)
	}
	m := f.re.FindStringIndex(text[lo:])
	if m == nil {
		return -1, -1
	}
	return lo + m[0], lo + m[1]
}

// findFrom returns the first match of f at or after off, wrapping around
// the end of the document.
func (f *findSession) findFrom(text string, off int) (int, int, bool) {
	try := func(lo, hi int) (int, int, bool) {
		for lo <= hi {
			a, b := f.index(text, lo)
			if a < 0 || a > hi {
				return 0, 0, false
			}
			if !f.whole || wholeWordMatch(text, a, b) {
				return a, b, true
			}
			lo = a + 1
		}
		return 0, 0, false
	}
	off = max(0, min(off, len(text)))
	if a, b, ok := try(off, len(text)); ok {
		return a, b, true
	}
	return try(0, off)
}

// all returns every match of f, in order, not overlapping.
func (f *findSession) all(text string) [][2]int {
	var out [][2]int
	for lo := 0; lo <= len(text); {
		a, b := f.index(text, lo)
		if a < 0 || b == a {
			break
		}
		if !f.whole || wholeWordMatch(text, a, b) {
			out = append(out, [2]int{a, b})
			lo = b
		} else {
			lo = a + 1
		}
	}
	return out
}

// setFindSels sets selections a find session made and remembers them.
func (s *State) setFindSels(f *findSession, sels []Selection) {
	s.setSels(sels, nil)
	f.sels, f.ver = slices.Clone(s.Sels), s.Snapshot().Version()
	s.find = f
}

// AddSelectionToNextFindMatch adds a selection at the next occurrence of
// the primary selection's text after the last added selection, wrapping
// around (Monaco's editor.action.addSelectionToNextFindMatch, Cmd+D). From
// a single caret it first selects the word under it; later presses then
// add the next occurrences of that word, case-sensitively and as a whole
// word; from a selection, its text matches case-insensitively anywhere
// (see findSessionNow). When several selections do not all hold the same
// text (ignoring case, as Monaco's modelRangesContainSameText with the
// default find options), it instead selects the word under every caret. It reports the selection it added (the primary
// selection for the first press from a caret), false when there is none.
func (s *State) AddSelectionToNextFindMatch() (Selection, bool) {
	snap := s.Snapshot()
	sels := s.sels()
	if f := s.find; len(sels) > 1 && (f == nil || f.ver != snap.Version() || !slices.Equal(f.sels, s.Sels)) {
		var first string
		same := true
		// Monaco's modelRangesContainSameText: an empty selection after
		// the first never counts as the same text, so several carets all
		// get their words.
		for i, x := range sels {
			a, b := ordered(x)
			t := snap.Slice(clampOff(snap, a), clampOff(snap, b))
			if i == 0 {
				first = t
			} else if a == b || !strings.EqualFold(t, first) {
				same = false
				break
			}
		}
		if !same {
			out := make([]Selection, len(sels))
			for i, x := range sels {
				out[i] = x
				if x.Anchor == x.Head {
					if a, b, ok := configuredWordAt(snap, clampOff(snap, x.Head)); ok {
						out[i] = Selection{Anchor: a, Head: b}
					}
				}
			}
			s.setSels(out, nil)
			return s.Primary(), true
		}
	}
	f := s.findSessionNow()
	if f == nil {
		return Selection{}, false
	}
	var m Selection
	if f.pending != nil {
		m, f.pending = *f.pending, nil
	} else {
		last := sels[len(sels)-1]
		_, from := ordered(last)
		a, b, ok := f.findFrom(snap.String(), from)
		if !ok {
			return Selection{}, false
		}
		m = Selection{Anchor: a, Head: b}
	}
	s.setFindSels(f, append(slices.Clone(sels), m))
	return m, true
}

// SelectHighlights selects every occurrence of the primary selection's
// text (or of the word under an empty caret) with the same matching rules
// as AddSelectionToNextFindMatch (Monaco's editor.action.selectHighlights,
// Cmd+Shift+L); the occurrence at the primary selection stays primary. It
// reports false when there is nothing to select.
func (s *State) SelectHighlights() bool {
	f := s.findSessionNow()
	if f == nil {
		return false
	}
	ms := f.all(s.Snapshot().String())
	if len(ms) == 0 {
		return false
	}
	pa, pb := ordered(s.Primary())
	out := make([]Selection, len(ms))
	for i, m := range ms {
		out[i] = Selection{Anchor: m[0], Head: m[1]}
	}
	for i, m := range ms {
		if m[0] <= pb && pa <= m[1] {
			out[0], out[i] = out[i], out[0]
			break
		}
	}
	f.pending = nil
	s.setFindSels(f, out)
	return true
}

// --- commands ---

func init() {
	for _, c := range cursorCommands() {
		RegisterCommand(c)
	}
}

func cursorCommands() []Command {
	cs := []Command{
		{ID: "editor.action.insertCursorAbove", Label: "Add Cursor Above", Run: func(st *State, _ any) bool {
			if st.readOnly() { // as Monaco
				return false
			}
			st.insertCursorVertical(Backward)
			st.revealOffset(st.topHead())
			return true
		}},
		{ID: "editor.action.insertCursorBelow", Label: "Add Cursor Below", Run: func(st *State, _ any) bool {
			if st.readOnly() {
				return false
			}
			st.insertCursorVertical(Forward)
			st.revealOffset(st.bottomHead())
			return true
		}},
		{ID: "editor.action.insertCursorAtEndOfEachLineSelected", Label: "Add Cursors to Line Ends", Run: func(st *State, _ any) bool {
			return st.insertCursorsAtLineEnds()
		}},
		{ID: "editor.action.addSelectionToNextFindMatch", Label: "Add Selection to Next Find Match", Run: func(st *State, _ any) bool {
			m, ok := st.AddSelectionToNextFindMatch()
			if ok {
				st.revealOffset(m.Head)
			}
			return ok
		}},
		{ID: "editor.action.selectHighlights", Label: "Select All Occurrences of Find Match", Run: func(st *State, _ any) bool {
			ok := st.SelectHighlights()
			if ok {
				st.revealCaret()
			}
			return ok
		}},
		{ID: "removeSecondaryCursors", Label: "Remove Secondary Cursors", Precondition: CtxEditorHasMultipleSelections, Run: func(st *State, _ any) bool {
			ok := st.RemoveSecondaryCursors()
			st.revealCaret()
			return ok
		}},
		{ID: "cursorUndo", Label: "Cursor Undo", Run: func(st *State, _ any) bool { return st.CursorUndo() }},
		{ID: "cursorRedo", Label: "Cursor Redo", Run: func(st *State, _ any) bool { return st.CursorRedo() }},
	}
	col := func(id, label string, d Direction, vertical, page bool) {
		cs = append(cs, Command{ID: id, Label: label, Run: func(st *State, _ any) bool {
			n := 1
			if page {
				n = st.pageLines()
			}
			st.columnSelectMove(d, vertical, n)
			st.revealOffset(st.columnHead())
			return true
		}})
	}
	col("cursorColumnSelectLeft", "Column Select Left", Backward, false, false)
	col("cursorColumnSelectRight", "Column Select Right", Forward, false, false)
	col("cursorColumnSelectUp", "Column Select Up", Backward, true, false)
	col("cursorColumnSelectDown", "Column Select Down", Forward, true, false)
	col("cursorColumnSelectPageUp", "Column Select Page Up", Backward, true, true)
	col("cursorColumnSelectPageDown", "Column Select Page Down", Forward, true, true)
	return cs
}

// topHead and bottomHead are the first and last caret in the document.
func (s *State) topHead() int {
	h := s.Primary().Head
	for _, x := range s.Sels {
		h = min(h, x.Head)
	}
	return h
}

func (s *State) bottomHead() int {
	h := s.Primary().Head
	for _, x := range s.Sels {
		h = max(h, x.Head)
	}
	return h
}

// columnHead is the caret on the moving edge of a column selection (its
// last line).
func (s *State) columnHead() int {
	if len(s.Sels) == 0 {
		return 0
	}
	return s.Sels[len(s.Sels)-1].Head
}
