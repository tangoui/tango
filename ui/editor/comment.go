package editor

import (
	"slices"
	"strings"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/langs"
)

// Comment commands (Monaco's comment contribution): toggle, add and
// remove line comments, and toggle block comments, with the language's
// comment tokens (langs.Conf.LineComment, BlockComment). A language
// without a line comment token (HTML, CSS) toggles a block comment around
// each selection's lines instead, as Monaco does.

func init() {
	edit := func(id, label string, run func(*State) bool) Command {
		return Command{ID: id, Label: label, Precondition: writable, Mutates: true, Run: func(st *State, _ any) bool {
			ok := run(st)
			if ok {
				st.revealCaret()
			}
			return ok
		}}
	}
	for _, c := range []Command{
		edit("editor.action.commentLine", "Toggle Line Comment", func(st *State) bool { return st.lineComment(commentToggle) }),
		edit("editor.action.addCommentLine", "Add Line Comment", func(st *State) bool { return st.lineComment(commentAdd) }),
		edit("editor.action.removeCommentLine", "Remove Line Comment", func(st *State) bool { return st.lineComment(commentRemove) }),
		edit("editor.action.blockComment", "Toggle Block Comment", (*State).blockComment),
	} {
		RegisterCommand(c)
	}
}

// commentType is what a line comment command does (Monaco's Type).
type commentType uint8

const (
	commentToggle commentType = iota
	commentAdd
	commentRemove
)

// commentOpts returns the Comments options in effect: the last View's for
// this frame, else Options.Comments.
func (s *State) commentOpts() CommentsOptions {
	if s.view != nil {
		return s.view.opts.Comments
	}
	return s.Options.Comments
}

// lineComment toggles, adds or removes line comments on the lines of every
// selection (Monaco's CommentLineAction and LineCommentCommand). Each
// selection decides for its own lines: toggling removes the comments when
// every non-blank line has one, else comments every line, the tokens
// aligned at the smallest indentation (rounded down to a tab stop). Blank
// lines are left alone (Comments.IgnoreEmptyLines) unless every line is
// blank. Where selections share a line, it is commented once (see below);
// every cursor stays, also several on one line.
func (s *State) lineComment(typ commentType) bool {
	conf := s.Model.Conf()
	if conf == nil {
		return false
	}
	snap := s.Snapshot()
	sels := s.clampedSels(snap)
	// Selections sorted by start.
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
	// A selection starting on the last line of the ones before it (the
	// greatest end line so far, so chains of selections sharing lines skip
	// each shared line once) leaves that line to them. As in Monaco, a
	// one-line selection added later than the one after it gives its line
	// up instead.
	skipFirst := make([]bool, len(sels))
	maxEnd, owner := -1, -1
	for _, c := range order {
		ca, cb := ordered(sels[c])
		cs, ce := snap.LineOf(ca), snap.LineOf(cb)
		if owner >= 0 && cs == maxEnd {
			oa, _ := ordered(sels[owner])
			if owner > c && snap.LineOf(oa) == maxEnd && !skipFirst[owner] {
				skipFirst[owner] = true
			} else {
				skipFirst[c] = true
			}
		}
		if ce >= maxEnd {
			maxEnd, owner = ce, c
		}
	}
	cmds := make([]opCmd, len(sels))
	for k, sel := range sels {
		cmds[k] = s.lineCommentOp(snap, sel, typ, skipFirst[k], conf)
	}
	return s.runOps(cmds)
}

// lineCommentOp is Monaco's LineCommentCommand for one selection.
func (s *State) lineCommentOp(snap buffer.Snapshot, sel Selection, typ commentType, skipFirst bool, conf *langs.Conf) opCmd {
	a, b := ordered(sel)
	sl, el := snap.LineOf(a), snap.LineOf(b)
	if sl == el && skipFirst {
		return opCmd{}
	}
	// A selection over several lines ending at the start of a line leaves
	// that line out, and its end goes back there afterwards.
	origEnd := el
	endDown := false
	if el > sl && b == snap.LineStart(el) {
		endDown = true
		el--
		b = lineEnd(snap, el)
	}
	adj := Selection{Anchor: a, Head: b}
	if sel.Head < sel.Anchor {
		adj = Selection{Anchor: b, Head: a}
	}
	finish := func(r *opResult, res Selection, delta int) Selection {
		movedHead := false
		if endDown {
			end := r.tr.Map(snap.LineStart(origEnd), buffer.Left)
			if res.Anchor <= res.Head {
				res.Head, movedHead = end, true
			} else {
				res.Anchor = end
			}
		}
		if delta != 0 && !movedHead {
			if res.Anchor == res.Head {
				res.Anchor += delta
			}
			res.Head += delta
		}
		return res
	}
	opts := s.commentOpts()
	tok := conf.LineComment
	if tok == "" {
		return s.blockFallback(snap, adj, conf, opts.InsertSpace, finish)
	}
	type lineData struct {
		ignore bool
		off, n int // the token's offset in the line, and its length with a following space
	}
	lines := make([]lineData, el-sl+1)
	remove := typ != commentAdd
	onlyBlank := true
	for i := range lines {
		ld := &lines[i]
		ld.n = len(tok)
		l := sl + i
		if l == sl && skipFirst {
			ld.ignore = true
			continue
		}
		text := snap.Line(l)
		fnw := firstNonBlank(text)
		if fnw < 0 {
			ld.ignore = opts.IgnoreEmptyLines
			ld.off = len(text)
			continue
		}
		onlyBlank = false
		ld.off = fnw
		if remove && !hasTokenAt(text, tok, fnw) {
			switch typ {
			case commentToggle:
				remove = false
			case commentRemove:
				ld.ignore = true
			}
		}
		if remove && opts.InsertSpace {
			if e := fnw + ld.n; e < len(text) && text[e] == ' ' {
				ld.n++
			}
		}
	}
	if typ == commentToggle && onlyBlank {
		remove = false
		for i := range lines {
			lines[i].ignore = false
		}
	}
	var edits []buffer.Edit
	delta := 0
	if remove {
		for i, ld := range lines {
			if ld.ignore {
				continue
			}
			ls := snap.LineStart(sl + i)
			end := min(ld.off+ld.n, len(snap.Line(sl+i)))
			if end > ld.off {
				edits = append(edits, buffer.Edit{Start: ls + ld.off, End: ls + end})
			}
		}
	} else {
		// Align the tokens at the smallest indentation, rounded down to
		// a tab stop (_normalizeInsertionPoint).
		ts := s.indentSize()
		next := func(vc int, tab bool) int {
			if tab {
				return vc + ts - vc%ts
			}
			return vc + 1
		}
		minVC := int(^uint(0) >> 1)
		for i, ld := range lines {
			if ld.ignore {
				continue
			}
			text := snap.Line(sl + i)
			vc := 0
			for j := 0; vc < minVC && j < ld.off; j++ {
				vc = next(vc, text[j] == '\t')
			}
			minVC = min(minVC, vc)
		}
		if minVC < int(^uint(0)>>1) {
			minVC = minVC / ts * ts
		}
		ins := tok
		if opts.InsertSpace {
			ins += " "
		}
		head := adj.Head
		for i := range lines {
			ld := &lines[i]
			if ld.ignore {
				continue
			}
			text := snap.Line(sl + i)
			vc, j := 0, 0
			for ; vc < minVC && j < ld.off; j++ {
				vc = next(vc, text[j] == '\t')
			}
			if vc > minVC {
				ld.off = j - 1
			} else {
				ld.off = j
			}
			at := snap.LineStart(sl+i) + ld.off
			edits = append(edits, buffer.Edit{Start: at, End: at, Text: ins})
			// A caret at the end of its line, where the token goes,
			// stays before it when tracked: move it after (_deltaColumn).
			if at == head && head == lineEnd(snap, sl+i) {
				delta = len(ins)
			}
		}
	}
	return opCmd{edits: edits, cursor: func(r *opResult, _ Selection) Selection {
		return finish(r, r.track(adj), delta)
	}}
}

// blockFallback is LineCommentCommand._executeBlockComment: for a language
// without a line comment, the selection's lines (a caret's line from its
// first non-blank character) get a block comment, or lose the one around
// them.
func (s *State) blockFallback(snap buffer.Snapshot, sel Selection, conf *langs.Conf, space bool, finish func(*opResult, Selection, int) Selection) opCmd {
	st, et := conf.BlockComment[0], conf.BlockComment[1]
	if st == "" || et == "" {
		return opCmd{}
	}
	a, b := ordered(sel)
	sl, el := snap.LineOf(a), snap.LineOf(b)
	edits := removeBlockAround(snap, sel, st, et)
	delta := 0
	if edits == nil {
		from := snap.LineStart(sl) + max(0, firstNonBlank(snap.Line(sl)))
		to := lineEnd(snap, el)
		if a == b {
			if fnw := firstNonBlank(snap.Line(sl)); fnw < 0 {
				from = to
			}
		}
		edits = addBlockEdits(from, to, st, et, space)
		if len(edits) == 1 {
			delta = len(st) + 1
		}
	}
	return opCmd{edits: edits, cursor: func(r *opResult, _ Selection) Selection {
		return finish(r, r.track(sel), delta)
	}}
}

// removeBlockAround is LineCommentCommand._attemptRemoveBlockComment: the
// edits that remove the block comment around sel (its tokens with a space
// inside each), nil when there is none.
func removeBlockAround(snap buffer.Snapshot, sel Selection, st, et string) []buffer.Edit {
	a, b := ordered(sel)
	sl, el := snap.LineOf(a), snap.LineOf(b)
	sc, ec := a-snap.LineStart(sl), b-snap.LineStart(el)
	allowed := len(et) + max(firstNonBlank(snap.Line(sl))+1, sc+1)
	sti := lastIndexFrom(snap.Line(sl), st, allowed-1)
	eti := indexFrom(snap.Line(el), et, ec-len(st))
	if sti >= 0 && eti < 0 {
		eti = indexFrom(snap.Line(sl), et, sti+len(st))
		el = sl
	}
	if sti < 0 && eti >= 0 {
		sti = lastIndexFrom(snap.Line(el), st, eti)
		sl = el
	}
	if a == b && (sti < 0 || eti < 0) {
		sti = strings.Index(snap.Line(sl), st)
		if sti >= 0 {
			eti = indexFrom(snap.Line(sl), et, sti+len(st))
		}
	}
	if sti >= 0 {
		if t := snap.Line(sl); sti+len(st) < len(t) && t[sti+len(st)] == ' ' {
			st += " "
		}
	}
	if eti > 0 {
		if t := snap.Line(el); t[eti-1] == ' ' {
			et = " " + et
			eti--
		}
	}
	if sti < 0 || eti < 0 {
		return nil
	}
	return removeBlockEdits(snap, sl, sti+len(st), el, eti, st, et)
}

// removeBlockEdits deletes block comment tokens st (ending at column sc of
// line sl) and et (starting at column ec of line el); Monaco's
// _createRemoveBlockCommentOperations. Like Monaco's Range, a start after
// the end is swapped; the deletes are clamped to their lines.
func removeBlockEdits(snap buffer.Snapshot, sl, sc, el, ec int, st, et string) []buffer.Edit {
	if sl > el || (sl == el && sc > ec) {
		sl, sc, el, ec = el, ec, sl, sc
	}
	pos := func(l, c int) int {
		return snap.LineStart(l) + max(0, min(c, len(snap.Line(l))))
	}
	if sl == el && sc == ec {
		return []buffer.Edit{{Start: pos(sl, sc-len(st)), End: pos(el, ec+len(et))}}
	}
	es := []buffer.Edit{
		{Start: pos(sl, sc-len(st)), End: pos(sl, sc)},
		{Start: pos(el, ec), End: pos(el, ec+len(et))},
	}
	return slices.DeleteFunc(es, func(e buffer.Edit) bool { return e.Start == e.End })
}

// addBlockEdits wraps [from, to) in block comment tokens, inside spaces
// with space; an empty range gets both tokens with two spaces between
// (Monaco's _createAddBlockCommentOperations).
func addBlockEdits(from, to int, st, et string, space bool) []buffer.Edit {
	if from == to {
		return []buffer.Edit{{Start: from, End: to, Text: st + "  " + et}}
	}
	sp := ""
	if space {
		sp = " "
	}
	return []buffer.Edit{
		{Start: from, End: from, Text: st + sp},
		{Start: to, End: to, Text: sp + et},
	}
}

// blockComment toggles a block comment around every selection (Monaco's
// BlockCommentAction and BlockCommentCommand): the tokens next to the
// selection's ends (inside or just outside it, with a space inside each)
// are removed when both are there and no end token sits between them,
// else the selection is wrapped (a caret gets an empty comment and goes
// inside it). Afterwards the selection covers the text between the tokens.
func (s *State) blockComment() bool {
	conf := s.Model.Conf()
	if conf == nil || conf.BlockComment[0] == "" || conf.BlockComment[1] == "" {
		return false
	}
	st, et := conf.BlockComment[0], conf.BlockComment[1]
	space := s.commentOpts().InsertSpace
	snap := s.Snapshot()
	sels := s.clampedSels(snap)
	cmds := make([]opCmd, len(sels))
	for k, sel := range sels {
		a, b := ordered(sel)
		sl, el := snap.LineOf(a), snap.LineOf(b)
		sc, ec := a-snap.LineStart(sl), b-snap.LineStart(el)
		slText, elText := snap.Line(sl), snap.Line(el)
		sti := lastIndexFrom(slText, st, sc+len(st))
		eti := indexFrom(elText, et, ec-len(et))
		if sti >= 0 && eti >= 0 {
			var inside bool
			if sl == el {
				inside = strings.Contains(substringJS(slText, sti+len(st), eti), et)
			} else {
				inside = strings.Contains(slText[sti+len(st):], et) || strings.Contains(elText[:eti], et)
			}
			if inside {
				sti, eti = -1, -1
			}
		}
		var edits []buffer.Edit
		usedEnd := false
		if sti >= 0 && eti >= 0 {
			stT, etT := st, et
			if space && sti+len(st) < len(slText) && slText[sti+len(st)] == ' ' {
				stT += " "
			}
			if space && eti > 0 && elText[eti-1] == ' ' {
				etT = " " + et
				eti--
			}
			edits = removeBlockEdits(snap, sl, sti+len(stT), el, eti, stT, etT)
		} else {
			edits = addBlockEdits(a, b, st, et, space)
			usedEnd = len(edits) == 1
		}
		cmds[k] = opCmd{edits: edits, cursor: func(r *opResult, sel Selection) Selection {
			inv := r.inv[k]
			switch len(inv) {
			case 2:
				return Selection{Anchor: inv[0][1], Head: inv[1][0]}
			case 1:
				c := inv[0][1]
				if usedEnd {
					c -= len(et) + 1
				}
				return Selection{Anchor: c, Head: c}
			}
			return r.track(sel)
		}}
	}
	return s.runOps(cmds)
}

// hasTokenAt reports whether text has tok at offset off, ASCII letters
// compared case-insensitively (Monaco's _haystackHasNeedleAtOffset).
func hasTokenAt(text, tok string, off int) bool {
	if off < 0 || off+len(tok) > len(text) {
		return false
	}
	for i := 0; i < len(tok); i++ {
		x, y := text[off+i], tok[i]
		if x == y || ('A' <= x && x <= 'Z' && x+32 == y) || ('A' <= y && y <= 'Z' && y+32 == x) {
			continue
		}
		return false
	}
	return true
}

// lastIndexFrom is JavaScript's s.lastIndexOf(sub, from): the last index
// of sub in s that is at most from.
func lastIndexFrom(s, sub string, from int) int {
	from = max(0, min(from, len(s)))
	return strings.LastIndex(s[:min(len(s), from+len(sub))], sub)
}

// indexFrom is JavaScript's s.indexOf(sub, from): the first index of sub
// in s that is at least from.
func indexFrom(s, sub string, from int) int {
	from = max(0, from)
	if from > len(s) {
		return -1
	}
	i := strings.Index(s[from:], sub)
	if i < 0 {
		return -1
	}
	return from + i
}

// substringJS is JavaScript's s.substring(i, j): the indices are clamped
// and swapped when i > j.
func substringJS(s string, i, j int) string {
	i, j = max(0, min(i, len(s))), max(0, min(j, len(s)))
	if i > j {
		i, j = j, i
	}
	return s[i:j]
}
