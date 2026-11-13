package editor

import (
	"cmp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/editor/buffer"
)

// Line operations (Monaco's linesOperations contribution): copy, move,
// delete, join, insert, transpose, sort and trim lines, and duplicate the
// selection. Every command acts on every cursor and is one undo step.
//
// Commands are built the way Monaco's are: each selection contributes a
// command (opCmd) with its edits and a rule for its selection afterwards;
// the edits of all of them are applied as one edit set (runOps). Where
// several cursors sit on the same line, the lines are deduplicated so a
// line is copied, moved, deleted, indented or commented once. Every cursor
// stays (several on one line too). Visible differences from Monaco, which
// deduplicates only the shared first line for copy and comments and
// otherwise relies on its edit builder dropping the later of two
// overlapping commands:
//
//   - copyLines copies the union of the lines of selections that share a
//     line (Monaco copies the first selection's lines and drops the
//     other's, even its lines not shared).
//   - moveLines and deleteLines treat selections on neighbouring lines as
//     one block. A block that contains the first line does not move up
//     (nor one with the last line down), so carets on lines 1 and 2 stay
//     put on moveLinesUp, where Monaco moves line 2 above line 1.
//   - duplicateSelection and insertLineBefore/After act once per line for
//     carets on one line (Monaco copies the line, or opens a line, once per
//     caret).
//   - Indenting and commenting shift or comment a shared line once (see
//     shiftOp and lineComment); Monaco drops the later command entirely.

func init() {
	for _, c := range lineCommands() {
		RegisterCommand(c)
	}
}

func lineCommands() []Command {
	edit := func(id, label string, run func(*State) bool) Command {
		return Command{ID: id, Label: label, Precondition: writable, Mutates: true, Run: func(st *State, _ any) bool {
			ok := run(st)
			if ok {
				st.revealCaret()
			}
			return ok
		}}
	}
	return []Command{
		edit("editor.action.copyLinesUpAction", "Copy Line Up", func(st *State) bool { return st.copyLines(false) }),
		edit("editor.action.copyLinesDownAction", "Copy Line Down", func(st *State) bool { return st.copyLines(true) }),
		edit("editor.action.moveLinesUpAction", "Move Line Up", func(st *State) bool { return st.moveLines(false) }),
		edit("editor.action.moveLinesDownAction", "Move Line Down", func(st *State) bool { return st.moveLines(true) }),
		edit("editor.action.deleteLines", "Delete Line", (*State).deleteLines),
		edit("editor.action.joinLines", "Join Lines", (*State).joinLines),
		edit("editor.action.insertLineBefore", "Insert Line Above", func(st *State) bool { return st.insertLine(true) }),
		edit("editor.action.insertLineAfter", "Insert Line Below", func(st *State) bool { return st.insertLine(false) }),
		edit("editor.action.duplicateSelection", "Duplicate Selection", (*State).duplicateSelection),
		edit("editor.action.transpose", "Transpose Characters around the Cursor", (*State).transpose),
		edit("editor.action.sortLinesAscending", "Sort Lines Ascending", func(st *State) bool { return st.sortLines(false) }),
		edit("editor.action.sortLinesDescending", "Sort Lines Descending", func(st *State) bool { return st.sortLines(true) }),
		edit("editor.action.trimTrailingWhitespace", "Trim Trailing Whitespace", (*State).trimTrailingWhitespace),
	}
}

// --- the command runner (Monaco's executeCommands) ---

// opCmd is the command of one selection: its edits (offsets in the current
// snapshot, in any order but not overlapping each other) and how it computes
// its selection afterwards.
type opCmd struct {
	edits []buffer.Edit
	// cursor returns the selection after the edits; nil tracks the
	// selection through them (opResult.track).
	cursor func(r *opResult, sel Selection) Selection
}

// opResult is what a command's cursor rule sees: the snapshot before the
// edits, a tracker through all of them and, per command, where its own
// edits ended up (Monaco's getInverseEditOperations).
type opResult struct {
	snap buffer.Snapshot
	tr   *buffer.Tracker
	inv  [][][2]int // inv[k][i]: the range in the new text of command k's edit i; nil for a dropped command
}

// track maps sel through the edits as Monaco's trackSelection does: a caret
// sticks to the character after it, except at the end of its line where it
// sticks to the one before (an insert at the caret pushes it, an insert
// at a line end does not); a selection never grows when text is inserted
// at its edges.
func (r *opResult) track(sel Selection) Selection {
	if sel.Anchor == sel.Head {
		b := buffer.Right
		if sel.Head == lineEnd(r.snap, r.snap.LineOf(sel.Head)) {
			b = buffer.Left
		}
		o := r.tr.Map(sel.Head, b)
		return Selection{Anchor: o, Head: o}
	}
	a, b := ordered(sel)
	na, nb := r.tr.Map(a, buffer.Right), r.tr.Map(b, buffer.Left)
	nb = max(na, nb)
	if sel.Anchor < sel.Head {
		return Selection{Anchor: na, Head: nb}
	}
	return Selection{Anchor: nb, Head: na}
}

// shift returns how far the edits move text that starts at off (the length
// change of the edits before off; an insert exactly at off does not count).
func (r *opResult) shift(off int) int { return r.tr.Map(off, buffer.Left) - off }

// runOps applies cmds, one per selection (cmds[k] for Sels[k], which must
// be normalized), as one edit set and one undo step, and sets each
// selection to its command's result. Where the edits of two commands
// overlap, the command of the later selection loses all its edits and only
// tracks its selection (Monaco's _getLoserCursorMap). It reports whether
// the text changed.
func (s *State) runOps(cmds []opCmd) bool { return s.runOpsKind(cmds, kindOther) }

// runOpsKind is runOps with the edits grouped into undo steps as kind
// (see State.edit): kindOther is a step of its own, kindType and
// kindDelete continue a step of the same kind.
func (s *State) runOpsKind(cmds []opCmd, kind editKind) bool {
	snap := s.Snapshot()
	type flat struct {
		e      buffer.Edit
		cmd, i int
		same   bool // replaces text by the same text: not applied
	}
	var all []flat
	for k, c := range cmds {
		for i, e := range c.edits {
			if e.Start == e.End && e.Text == "" {
				continue
			}
			// A replacement by the same text changes nothing (and moves no
			// marker): it is not applied, but its command still sees where
			// it is (transposing "a" with nothing moves the caret).
			same := e.End-e.Start == len(e.Text) && snap.Slice(e.Start, e.End) == e.Text
			all = append(all, flat{e, k, i, same})
		}
	}
	if len(all) == 0 {
		return false
	}
	slices.SortStableFunc(all, func(x, y flat) int {
		if c := cmp.Compare(x.e.Start, y.e.Start); c != 0 {
			return c
		}
		return cmp.Compare(x.e.End, y.e.End)
	})
	for {
		loser := -1
		for i := 1; i < len(all); i++ {
			p, q := all[i-1], all[i]
			if q.e.Start < p.e.End && p.cmd != q.cmd {
				loser = max(p.cmd, q.cmd)
				break
			}
		}
		if loser < 0 {
			break
		}
		all = slices.DeleteFunc(all, func(f flat) bool { return f.cmd == loser })
		cmds[loser].cursor = nil
	}
	edits := make([]buffer.Edit, 0, len(all))
	r := &opResult{snap: snap, inv: make([][][2]int, len(cmds))}
	delta := 0
	for _, f := range all {
		if r.inv[f.cmd] == nil {
			r.inv[f.cmd] = make([][2]int, len(cmds[f.cmd].edits))
		}
		start := f.e.Start + delta
		n := storedLen(f.e.Text)
		r.inv[f.cmd][f.i] = [2]int{start, start + n}
		if f.same {
			continue
		}
		edits = append(edits, f.e)
		delta += n - (f.e.End - f.e.Start)
	}
	r.tr = buffer.NewTracker(edits)
	after := make([]Selection, len(s.Sels))
	for k, sel := range s.Sels {
		switch {
		case k < len(cmds) && cmds[k].cursor != nil:
			after[k] = cmds[k].cursor(r, sel)
		default:
			after[k] = r.track(sel)
		}
	}
	if len(edits) == 0 {
		// Nothing to change but the selections.
		was := slices.Clone(s.Sels)
		s.SetSelections(after)
		return !slices.Equal(was, s.Sels)
	}
	return s.applyOps(edits, after, kind)
}

// applyOps applies edits (sorted, non-overlapping) grouped into undo steps
// as kind (kindOther: a step of its own) and sets the selections to after
// (parallel to Sels, in the new text), merged as Monaco merges cursors. It
// reports whether the text changed.
func (s *State) applyOps(edits []buffer.Edit, after []Selection, kind editKind) bool {
	if len(edits) == 0 {
		return false
	}
	v := s.Snapshot().Version()
	after, _, last := mergeSelsLast(after, s.lastAdded)
	if err := s.edit(edits, kind, after, false); err != nil {
		return false
	}
	s.lastAdded = last
	s.clampLast()
	return s.Snapshot().Version() != v
}

// clampedSels normalizes the selections and returns them clamped into snap
// (the State's own Sels may have been set directly).
func (s *State) clampedSels(snap buffer.Snapshot) []Selection {
	s.normalize()
	out := make([]Selection, len(s.Sels))
	for k, x := range s.Sels {
		out[k] = Selection{Anchor: clampOff(snap, x.Anchor), Head: clampOff(snap, x.Head)}
	}
	s.Sels = out
	return out
}

// selLines returns the lines sel covers, as Monaco's line commands count
// them: a selection over several lines that ends at the start of a line
// does not include that line.
func selLines(snap buffer.Snapshot, sel Selection) (first, last int) {
	a, b := ordered(sel)
	first, last = snap.LineOf(a), snap.LineOf(b)
	if last > first && b == snap.LineStart(last) {
		last--
	}
	return first, last
}

// lineBlock is a run of lines and the selections (indices) that cover it.
type lineBlock struct {
	first, last int
	sels        []int
}

// lineBlocks groups the lines of sels into blocks, in document order: the
// lines of selections that share a line merge (several cursors on one line
// act on it once), and with adjacent also blocks that touch (moving or
// deleting two neighbouring lines moves or deletes them together).
func lineBlocks(snap buffer.Snapshot, sels []Selection, adjacent bool) []lineBlock {
	bs := make([]lineBlock, len(sels))
	for k, x := range sels {
		f, l := selLines(snap, x)
		bs[k] = lineBlock{first: f, last: l, sels: []int{k}}
	}
	slices.SortStableFunc(bs, func(x, y lineBlock) int {
		if c := cmp.Compare(x.first, y.first); c != 0 {
			return c
		}
		return cmp.Compare(x.last, y.last)
	})
	out := bs[:0]
	for _, b := range bs {
		if n := len(out); n > 0 {
			p := &out[n-1]
			if b.first <= p.last || (adjacent && b.first == p.last+1) {
				p.last = max(p.last, b.last)
				p.sels = append(p.sels, b.sels...)
				continue
			}
		}
		out = append(out, b)
	}
	return out
}

// shiftSel moves both ends of sel by d.
func shiftSel(sel Selection, d int) Selection {
	return Selection{Anchor: sel.Anchor + d, Head: sel.Head + d}
}

// --- copy, move, delete ---

// copyLines duplicates the lines of every selection above (up) or below
// (down) them (Monaco's CopyLinesCommand): the selection follows the copy
// below when copying down and stays on the upper lines when copying up.
// Selections sharing a line copy it once.
func (s *State) copyLines(down bool) bool {
	snap := s.Snapshot()
	sels := s.clampedSels(snap)
	cmds := make([]opCmd, len(sels))
	for _, bl := range lineBlocks(snap, sels, false) {
		from, to := snap.LineStart(bl.first), lineEnd(snap, bl.last)
		text := snap.Slice(from, to)
		var e buffer.Edit
		move := 0
		if down {
			e = buffer.Edit{Start: from, End: from, Text: text + "\n"}
			move = len(text) + 1
		} else {
			e = buffer.Edit{Start: to, End: to, Text: "\n" + text}
		}
		cmds[bl.sels[0]].edits = []buffer.Edit{e}
		for _, k := range bl.sels {
			cmds[k].cursor = func(r *opResult, sel Selection) Selection {
				return shiftSel(sel, r.shift(from)+move)
			}
		}
	}
	return s.runOps(cmds)
}

// moveLines moves the lines of every selection one line up or down,
// swapping them with the line above or below (Monaco's MoveLinesCommand);
// the selection moves with them. A block at the first line (up) or the last
// line (down) stays. Selections on the same or neighbouring lines move as
// one block.
//
// With autoIndent "full" (Options.AutoIndent) and a language with
// indentation rules, the moved lines are re-indented as Monaco 0.56's
// MoveLinesCommand does: by the onEnter rules of the line that ends up
// above them, else by the indentation rules (onenter.go, moveAutoIndent).
// A selection end inside a re-indented line's indentation stays inside the
// new indentation (Monaco tracks it through the edit; it also shrinks a
// multi-line selection ending in an indentation that becomes empty, which
// the editor does not).
func (s *State) moveLines(down bool) bool {
	snap := s.Snapshot()
	sels := s.clampedSels(snap)
	n := snap.LineCount()
	cmds := make([]opCmd, len(sels))
	for _, bl := range lineBlocks(snap, sels, true) {
		f, l := bl.first, bl.last
		var es, ie []buffer.Edit
		var lens map[int][2]int
		auto := func() (moveIndent, bool) {
			if f == l && snap.Line(f) == "" {
				return moveIndent{}, false // Monaco moves an empty line without re-indenting
			}
			mi, ok := s.moveAutoIndent(snap, f, l, down)
			if ok {
				ie, lens = s.blockIndentEdits(snap, f, l, mi.blockDelta)
			}
			return mi, ok
		}
		d := 0
		switch {
		case !down && f > 0:
			m := snap.Line(f - 1)
			end := lineEnd(snap, l)
			auto()
			es = []buffer.Edit{
				{Start: snap.LineStart(f - 1), End: snap.LineStart(f)},
				{Start: end, End: end, Text: "\n" + m},
			}
			d = -(len(m) + 1)
		case down && l < n-1:
			m := snap.Line(l + 1)
			if mi, ok := auto(); ok {
				m = mi.moving
			}
			start := snap.LineStart(f)
			es = []buffer.Edit{
				{Start: start, End: start, Text: m + "\n"},
				{Start: lineEnd(snap, l), End: lineEnd(snap, l+1)},
			}
			d = len(m) + 1
		default:
			continue
		}
		cmds[bl.sels[0]].edits = append(es, ie...)
		// Where an end of a selection lands: moved by d, plus the
		// indentation changes of the block's lines up to its own.
		moved := func(p int) int {
			i := snap.LineOf(p)
			np := p + d
			for j := f; j <= l && j < i; j++ {
				if x, ok := lens[j]; ok {
					np += x[1] - x[0]
				}
			}
			if x, ok := lens[i]; ok && i >= f && i <= l {
				col := p - snap.LineStart(i)
				if col >= x[0] {
					np += x[1] - x[0]
				} else {
					np += min(col, x[1]) - col
				}
			}
			return np
		}
		for _, k := range bl.sels {
			cmds[k].cursor = func(_ *opResult, sel Selection) Selection {
				return Selection{Anchor: moved(sel.Anchor), Head: moved(sel.Head)}
			}
		}
	}
	return s.runOps(cmds)
}

// deleteLines deletes the lines of every selection with one adjacent line
// break (Monaco's DeleteLinesAction); the caret goes to the line that took
// their place, at the column the caret had (UTF-16 columns, as Monaco's).
// Selections on the same or neighbouring lines delete one block. An empty
// document does nothing.
func (s *State) deleteLines() bool {
	snap := s.Snapshot()
	if snap.Len() == 0 {
		return false
	}
	sels := s.clampedSels(snap)
	n := snap.LineCount()
	cmds := make([]opCmd, len(sels))
	deleted := 0 // bytes deleted by the blocks before
	for _, bl := range lineBlocks(snap, sels, true) {
		f, l := bl.first, bl.last
		head := sels[bl.sels[0]].Head
		hl := snap.LineOf(head)
		col16 := snap.UTF16Col(hl, head-snap.LineStart(hl))
		var e buffer.Edit
		var target, start int // the line the caret lands on (old numbering) and its new start
		switch {
		case l < n-1:
			e = buffer.Edit{Start: snap.LineStart(f), End: snap.LineStart(l + 1)}
			target, start = l+1, snap.LineStart(f)-deleted
		case f > 0:
			e = buffer.Edit{Start: lineEnd(snap, f-1), End: snap.Len()}
			target, start = f-1, snap.LineStart(f-1)-deleted
		default:
			e = buffer.Edit{Start: 0, End: snap.Len()}
			target, start = -1, 0
		}
		caret := start
		if target >= 0 {
			caret += snap.ByteCol(target, col16)
		}
		deleted += e.End - e.Start
		cmds[bl.sels[0]].edits = []buffer.Edit{e}
		for _, k := range bl.sels {
			cmds[k].cursor = func(*opResult, Selection) Selection { return Selection{Anchor: caret, Head: caret} }
		}
	}
	return s.runOps(cmds)
}

// --- join, insert ---

// joinLines joins each selection's lines (a caret's or a one-line
// selection's line with the next one) into one, as Monaco's JoinLinesAction:
// the leading blanks of each joined line are dropped and one space separates
// it from the text before (none when that text is empty or ends in a
// blank, whose trailing blanks then collapse to one space). A caret ends
// where the last line was joined, a selection keeps covering the same text.
func (s *State) joinLines() bool {
	snap := s.Snapshot()
	sels := s.clampedSels(snap)
	n := snap.LineCount()
	// Reduce the selections (sorted) to non-overlapping runs, as Monaco.
	type run struct {
		a, b  int   // the run's selection range
		empty bool  // the run is a caret
		sels  []int // the selections it covers
	}
	order := make([]int, len(sels))
	for i := range order {
		order[i] = i
	}
	slices.SortStableFunc(order, func(x, y int) int {
		xa, xb := ordered(sels[x])
		ya, yb := ordered(sels[y])
		if xa != ya {
			return xa - ya
		}
		return xb - yb
	})
	var runs []run
	for _, k := range order {
		a, b := ordered(sels[k])
		if m := len(runs); m > 0 {
			p := &runs[m-1]
			pl, ql := snap.LineOf(p.b), snap.LineOf(a)
			// A caret (or a one-line selection) joins the next line too.
			single := p.empty || snap.LineOf(p.a) == pl
			if (single && ql <= pl+1) || (!single && ql <= pl) {
				if p.empty && ql == pl {
					// The caret's run is replaced by the selection on its line.
					p.a, p.b, p.empty = a, b, a == b
				} else {
					p.b = max(p.b, b)
					p.empty = false
				}
				p.sels = append(p.sels, k)
				continue
			}
		}
		runs = append(runs, run{a: a, b: b, empty: a == b, sels: []int{k}})
	}
	cmds := make([]opCmd, len(sels))
	delta := 0 // length change of the runs before
	for _, rn := range runs {
		sl, el := snap.LineOf(rn.a), snap.LineOf(rn.b)
		endLine := el
		if rn.empty || sl == el {
			endLine = min(sl+1, n-1)
		}
		joined := snap.Line(sl)
		colDelta := 0
		for i := sl + 1; i <= endLine; i++ {
			text := snap.Line(i)
			ind := firstNonBlank(text)
			if ind < 0 {
				colDelta = 0
				continue
			}
			space := joined != ""
			if space && (strings.HasSuffix(joined, " ") || strings.HasSuffix(joined, "\t")) {
				space = false
				joined = strings.TrimRightFunc(joined, isJSSpace) + " "
			}
			rest := text[ind:]
			if space {
				joined += " "
			}
			joined += rest
			colDelta = len(rest)
			if space {
				colDelta++
			}
		}
		from, to := snap.LineStart(sl), lineEnd(snap, endLine)
		if from == to {
			continue
		}
		newStart := from + delta
		var res Selection
		switch {
		case rn.empty:
			c := newStart + len(joined) - colDelta
			res = Selection{Anchor: c, Head: c}
		case sl == el:
			res = Selection{Anchor: rn.a + delta, Head: rn.b + delta}
		default:
			// The end keeps its distance to the end of its line.
			endRest := lineEnd(snap, el) - rn.b
			res = Selection{Anchor: rn.a + delta, Head: newStart + len(joined) - endRest}
		}
		delta += len(joined) - (to - from)
		cmds[rn.sels[0]].edits = []buffer.Edit{{Start: from, End: to, Text: joined}}
		for _, k := range rn.sels {
			cmds[k].cursor = func(*opResult, Selection) Selection { return res }
		}
	}
	return s.runOps(cmds)
}

// isJSSpace reports whether r is matched by JavaScript's \s (which
// includes U+FEFF and U+00A0; Monaco's joinLines trims those).
func isJSSpace(r rune) bool { return unicode.IsSpace(r) || r == 0xFEFF || r == 0xA0 }

// firstNonBlank returns the index of the first byte of text that is not a
// space or a tab, -1 when there is none (Monaco's firstNonWhitespaceIndex).
func firstNonBlank(text string) int {
	for i := 0; i < len(text); i++ {
		if text[i] != ' ' && text[i] != '\t' {
			return i
		}
	}
	return -1
}

// lastNonBlank returns the index of the last byte of text that is not a
// space or a tab, -1 when there is none (Monaco's lastNonWhitespaceIndex).
func lastNonBlank(text string) int {
	for i := len(text) - 1; i >= 0; i-- {
		if text[i] != ' ' && text[i] != '\t' {
			return i
		}
	}
	return -1
}

// leadingBlanks returns the indentation of text: its leading spaces and
// tabs.
func leadingBlanks(text string) string {
	if i := firstNonBlank(text); i >= 0 {
		return text[:i]
	}
	return text
}

// insertLine opens an empty line above (before) or below each caret's line
// and puts the caret on it, indented as Enter at the end of the line above
// it indents (Monaco's insertLineBefore / insertLineAfter, which run
// EnterOperation there: the onEnter and indentation rules apply). Above the
// first line the new line is not indented. Cursors on the same line open
// one line.
func (s *State) insertLine(before bool) bool {
	snap := s.Snapshot()
	sels := s.clampedSels(snap)
	cmds := make([]opCmd, len(sels))
	owner := map[int]int{}               // line → the command that inserts for it
	res := make([]*Selection, len(sels)) // the owners' carets
	for k, sel := range sels {
		l := snap.LineOf(sel.Head)
		if o, ok := owner[l]; ok {
			cmds[k].cursor = func(r *opResult, sel Selection) Selection {
				if res[o] == nil {
					return r.track(sel)
				}
				return *res[o]
			}
			continue
		}
		owner[l] = k
		var c opCmd
		switch {
		case before && l == 0:
			c = opCmd{edits: []buffer.Edit{{Start: 0, End: 0, Text: "\n"}}, cursor: func(r *opResult, sel Selection) Selection {
				if r.inv[k] == nil {
					return r.track(sel)
				}
				c := r.inv[k][0][0] // the caret stays at the start, now on the new line
				return Selection{Anchor: c, Head: c}
			}}
		case before:
			at := lineEnd(snap, l-1)
			c = s.enterCmd(snap, k, at, at)
		default:
			at := lineEnd(snap, l)
			c = s.enterCmd(snap, k, at, at)
		}
		inner := c.cursor
		c.cursor = func(r *opResult, sel Selection) Selection {
			x := inner(r, sel)
			res[k] = &x
			return x
		}
		cmds[k] = c
	}
	return s.runOps(cmds)
}

// duplicateSelection inserts a copy of each selection after it and selects
// the copy; a caret copies its line below (as copyLinesDown; carets on one
// line copy it once). Monaco's DuplicateSelectionAction.
func (s *State) duplicateSelection() bool {
	snap := s.Snapshot()
	sels := s.clampedSels(snap)
	cmds := make([]opCmd, len(sels))
	copied := map[int]bool{}
	for k, sel := range sels {
		a, b := ordered(sel)
		if a == b {
			l := snap.LineOf(a)
			if !copied[l] {
				copied[l] = true
				from := snap.LineStart(l)
				cmds[k].edits = []buffer.Edit{{Start: from, End: from, Text: snap.Line(l) + "\n"}}
			}
			cmds[k].cursor = func(r *opResult, sel Selection) Selection {
				o := r.tr.Map(sel.Head, buffer.Right)
				return Selection{Anchor: o, Head: o}
			}
			continue
		}
		cmds[k] = opCmd{
			edits: []buffer.Edit{{Start: b, End: b, Text: snap.Slice(a, b)}},
			cursor: func(r *opResult, _ Selection) Selection {
				if r.inv[k] == nil {
					return r.track(sel)
				}
				in := r.inv[k][0]
				return Selection{Anchor: in[0], Head: in[1]}
			},
		}
	}
	return s.runOps(cmds)
}

// --- transpose, sort, trim ---

// transpose swaps the characters (grapheme clusters) around each caret and
// moves the caret after them; at the end of a line it swaps the last
// character with the line break. Selections are left alone. Monaco's
// TransposeAction (which reverses UTF-16 code units; the editor keeps
// clusters whole).
func (s *State) transpose() bool {
	snap := s.Snapshot()
	sels := s.clampedSels(snap)
	cmds := make([]opCmd, len(sels))
	for k, sel := range sels {
		if sel.Anchor != sel.Head {
			continue
		}
		l := snap.LineOf(sel.Head)
		text := snap.Line(l)
		ls := snap.LineStart(l)
		col := sel.Head - ls
		if col >= len(text) {
			if l == snap.LineCount()-1 {
				continue
			}
			from := ls + prevGrapheme(text, col)
			if col == 0 {
				from = ls
			}
			to := snap.LineStart(l + 1)
			cmds[k] = opCmd{
				edits: []buffer.Edit{{Start: from, End: to, Text: "\n" + snap.Slice(from, to-1)}},
				cursor: func(r *opResult, sel Selection) Selection {
					if r.inv[k] == nil {
						return r.track(sel)
					}
					c := r.inv[k][0][1]
					return Selection{Anchor: c, Head: c}
				},
			}
			continue
		}
		p := col
		if col > 0 {
			p = prevGrapheme(text, col)
		}
		q := nextGrapheme(text, col)
		swapped := text[col:q] + text[p:col]
		after := ls + q
		cmds[k] = opCmd{
			edits: []buffer.Edit{{Start: ls + p, End: ls + q, Text: swapped}},
			cursor: func(r *opResult, _ Selection) Selection {
				return r.track(Selection{Anchor: after, Head: after})
			},
		}
	}
	return s.runOps(cmds)
}

// sortLines sorts the lines of each selection (of the whole document when
// there is one selection on one line), as Monaco's SortLinesCommand. When a
// selection covers fewer than two lines or is sorted already, nothing
// happens (Monaco's canRun). Lines compare as Intl.Collator does for the root locale in the
// common cases: letters case-insensitively first (a < B < c), then lower
// case before upper case, then by code point.
func (s *State) sortLines(descending bool) bool {
	snap := s.Snapshot()
	sels := s.clampedSels(snap)
	ranges := make([]Selection, len(sels))
	copy(ranges, sels)
	if len(sels) == 1 && snap.LineOf(sels[0].Anchor) == snap.LineOf(sels[0].Head) {
		ranges[0] = Selection{Anchor: 0, Head: snap.Len()}
	}
	cmds := make([]opCmd, len(sels))
	for k, rg := range ranges {
		a, b := ordered(rg)
		f, l := snap.LineOf(a), snap.LineOf(b)
		if b == snap.LineStart(l) {
			l-- // Monaco: an end at column 1 leaves its line out, even for one line
		}
		if f >= l {
			return false
		}
		lines := make([]string, 0, l-f+1)
		for i := f; i <= l; i++ {
			lines = append(lines, snap.Line(i))
		}
		sorted := slices.Clone(lines)
		slices.SortStableFunc(sorted, collate)
		if descending {
			slices.Reverse(sorted)
		}
		if slices.Equal(lines, sorted) {
			return false // Monaco runs only when every selection changes
		}
		cmds[k].edits = []buffer.Edit{{Start: snap.LineStart(f), End: lineEnd(snap, l), Text: strings.Join(sorted, "\n")}}
	}
	return s.runOps(cmds)
}

// collate compares two lines for sortLines (see there).
func collate(x, y string) int {
	if c := strings.Compare(foldCase(x), foldCase(y)); c != 0 {
		return c
	}
	// Same letters: lower case sorts first (ICU's tertiary strength).
	for x != "" && y != "" {
		rx, nx := utf8.DecodeRuneInString(x)
		ry, ny := utf8.DecodeRuneInString(y)
		if rx != ry {
			lx, ly := unicode.IsLower(rx), unicode.IsLower(ry)
			if lx != ly {
				if lx {
					return -1
				}
				return 1
			}
			return cmp.Compare(rx, ry)
		}
		x, y = x[nx:], y[ny:]
	}
	return cmp.Compare(len(x), len(y))
}

func foldCase(s string) string { return strings.ToLower(s) }

// trimTrailingWhitespace deletes the trailing spaces and tabs of every line
// (whitespace-only lines become empty), as Monaco 0.56's
// TrimTrailingWhitespace action run from a key or the palette: it trims
// every line, the cursors' too, but leaves the blanks alone where they
// start inside a string or a regular expression token (standalone Monaco
// leaves files.trimTrailingWhitespaceInRegexAndStrings undefined, which
// skips them: the blanks are part of a raw string, a template or a
// multi-line string). The tokens come from tokenizing the document with the
// language's lexer (lineTokens). The cursors keep their place, all of them
// (Monaco's command keeps only the primary).
func (s *State) trimTrailingWhitespace() bool {
	snap := s.Snapshot()
	sels := s.clampedSels(snap)
	lt := s.newLineTokens(snap)
	var edits []buffer.Edit
	for i := range snap.LineCount() {
		text := snap.Line(i)
		toks := lt.tokens(i) // every line, to carry the lexer state
		if text == "" {
			continue
		}
		j := lastNonBlank(text) + 1
		if j == len(text) {
			continue
		}
		// Monaco looks up the token at fromColumn used as an offset: one
		// past the first trailing blank.
		if typ := tokenAt(toks, j+1); typ == "string" || typ == "regex" {
			continue
		}
		ls := snap.LineStart(i)
		edits = append(edits, buffer.Edit{Start: ls + j, End: ls + len(text)})
	}
	if len(edits) == 0 {
		return false
	}
	cmds := make([]opCmd, len(sels))
	cmds[0].edits = edits
	return s.runOps(cmds)
}
