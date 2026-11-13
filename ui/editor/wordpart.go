package editor

import "github.com/tangoui/tango/ui/editor/buffer"

// Word movement and deletion (Monaco 0.56's WordOperations and
// WordPartOperations, contrib wordOperations / wordPartOperations).
//
// Words are runs of one character class by Monaco's default
// wordSeparators option (`~!@#$%^&*()-=+[{]}\|;:'",.<>/?): regular
// characters (letters, digits, '_' and every non-ASCII character),
// separators, and blanks (space, tab) between them. The commands are
// Monaco's:
//
//	cursorWordStartLeft / cursorWordEndLeft / cursorWordLeft (WordStartFast)
//	cursorWordStartRight / cursorWordEndRight / cursorWordRight (WordEnd)
//	cursorWordAccessibilityLeft / Right (skip separator-only words)
//	cursorWordPartLeft / Right (camelCase, snake_case and kebab-case parts)
//	deleteWordStartLeft / deleteWordEndLeft / deleteWordLeft
//	deleteWordStartRight / deleteWordEndRight / deleteWordRight
//	deleteWordPartLeft / Right, deleteInsideWord
//
// each cursor* also as *Select (extending the selections), and the aliases
// cursorWordPartStartLeft(Select). deleteWordLeft/Right and the WordPart
// deletes first delete a run of two or more blanks next to the caret
// (Monaco's whitespaceHeuristics), and deleteWordLeft and
// deleteWordPartLeft delete an auto-closed pair around the caret as
// Backspace does. Monaco's Intl word segmentation (wordSegmenterLocales) is
// off by default and not implemented. State.Move and State.Delete by Word
// keep milestone 1's words (the language's wordPattern).

func init() {
	for _, c := range wordCommands() {
		RegisterCommand(c)
	}
}

// Monaco's WordNavigationType.
type wordNav int

const (
	navWordStart wordNav = iota
	navWordStartFast
	navWordEnd
	navWordAccessibility
)

// Monaco's WordCharacterClass and WordType.
const (
	classRegular = iota
	classWhitespace
	classSeparator
)

const (
	wordNone = iota
	wordRegular
	wordSeparator
)

func wordClass(b byte) int {
	switch {
	case b == ' ' || b == '\t':
		return classWhitespace
	case b < 0x80 && containsByte(wordSeparators, b):
		return classSeparator
	}
	return classRegular
}

func containsByte(s string, b byte) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return true
		}
	}
	return false
}

// lineWord is a word on a line: [start, end) in bytes, its type, and the
// class of the character that ended the search for it.
type lineWord struct {
	start, end, typ, next int
}

// prevWordOnLine is Monaco's _findPreviousWordOnLine from byte column col.
func prevWordOnLine(text string, col int) (lineWord, bool) {
	typ := wordNone
	for i := col - 1; i >= 0; i-- {
		c := wordClass(text[i])
		switch c {
		case classRegular:
			if typ == wordSeparator {
				return lineWord{i + 1, endOfWord(text, typ, i+1), typ, c}, true
			}
			typ = wordRegular
		case classSeparator:
			if typ == wordRegular {
				return lineWord{i + 1, endOfWord(text, typ, i+1), typ, c}, true
			}
			typ = wordSeparator
		case classWhitespace:
			if typ != wordNone {
				return lineWord{i + 1, endOfWord(text, typ, i+1), typ, c}, true
			}
		}
	}
	if typ != wordNone {
		return lineWord{0, endOfWord(text, typ, 0), typ, classWhitespace}, true
	}
	return lineWord{}, false
}

func endOfWord(text string, typ, start int) int {
	for i := start; i < len(text); i++ {
		c := wordClass(text[i])
		if c == classWhitespace || (typ == wordRegular && c == classSeparator) || (typ == wordSeparator && c == classRegular) {
			return i
		}
	}
	return len(text)
}

// nextWordOnLine is Monaco's _findNextWordOnLine from byte column col.
func nextWordOnLine(text string, col int) (lineWord, bool) {
	typ := wordNone
	for i := max(col, 0); i < len(text); i++ {
		c := wordClass(text[i])
		switch c {
		case classRegular:
			if typ == wordSeparator {
				return lineWord{startOfWord(text, typ, i-1), i, typ, c}, true
			}
			typ = wordRegular
		case classSeparator:
			if typ == wordRegular {
				return lineWord{startOfWord(text, typ, i-1), i, typ, c}, true
			}
			typ = wordSeparator
		case classWhitespace:
			if typ != wordNone {
				return lineWord{startOfWord(text, typ, i-1), i, typ, c}, true
			}
		}
	}
	if typ != wordNone {
		return lineWord{startOfWord(text, typ, len(text)-1), len(text), typ, classWhitespace}, true
	}
	return lineWord{}, false
}

func startOfWord(text string, typ, start int) int {
	for i := start; i >= 0; i-- {
		c := wordClass(text[i])
		if c == classWhitespace || (typ == wordRegular && c == classSeparator) || (typ == wordSeparator && c == classRegular) {
			return i + 1
		}
	}
	return 0
}

// pos splits off into its line and byte column.
func pos(snap buffer.Snapshot, off int) (int, int) {
	l := snap.LineOf(off)
	return l, off - snap.LineStart(l)
}

// moveWordLeft is Monaco's WordOperations.moveWordLeft from off.
func moveWordLeft(snap buffer.Snapshot, off int, nav wordNav, multi bool) int {
	l, col := pos(snap, off)
	if col == 0 && l > 0 {
		l--
		col = len(snap.Line(l))
	}
	text := snap.Line(l)
	ls := snap.LineStart(l)
	w, ok := prevWordOnLine(text, col)
	at := func(ok bool, c int) int {
		if ok {
			return ls + c
		}
		return ls
	}
	switch nav {
	case navWordStart:
		return at(ok, w.start)
	case navWordStartFast:
		// Skip a one-character separator word followed by a regular
		// character (one cursor only).
		if !multi && ok && w.typ == wordSeparator && w.end-w.start == 1 && w.next == classRegular {
			w, ok = prevWordOnLine(text, w.start)
		}
		return at(ok, w.start)
	case navWordAccessibility:
		for ok && w.typ == wordSeparator {
			w, ok = prevWordOnLine(text, w.start)
		}
		return at(ok, w.start)
	}
	if ok && col <= w.end {
		w, ok = prevWordOnLine(text, w.start)
	}
	return at(ok, w.end)
}

// moveWordRight is Monaco's WordOperations.moveWordRight from off.
func moveWordRight(snap buffer.Snapshot, off int, nav wordNav) int {
	l, col := pos(snap, off)
	movedDown := false
	if col == len(snap.Line(l)) && l < snap.LineCount()-1 {
		movedDown = true
		l, col = l+1, 0
	}
	text := snap.Line(l)
	ls := snap.LineStart(l)
	w, ok := nextWordOnLine(text, col)
	switch nav {
	case navWordEnd:
		if ok && w.typ == wordSeparator && w.end-w.start == 1 && w.next == classRegular {
			w, ok = nextWordOnLine(text, w.end)
		}
		if ok {
			return ls + w.end
		}
	case navWordAccessibility:
		if movedDown {
			col = -1 // before the line's first character
		}
		for ok && (w.typ == wordSeparator || w.start <= col) {
			w, ok = nextWordOnLine(text, w.end)
		}
		if ok {
			return ls + w.start
		}
	default:
		if ok && !movedDown && col >= w.start {
			w, ok = nextWordOnLine(text, w.end)
		}
		if ok {
			return ls + w.start
		}
	}
	return ls + len(text)
}

func isLowerASCII(b byte) bool { return 'a' <= b && b <= 'z' }
func isUpperASCII(b byte) bool { return 'A' <= b && b <= 'Z' }
func isDigitASCII(b byte) bool { return '0' <= b && b <= '9' }

// moveWordPartLeft is Monaco's WordOperations._moveWordPartLeft: the
// previous camelCase, snake_case or kebab-case boundary on the line.
func moveWordPartLeft(snap buffer.Snapshot, off int) int {
	l, col := pos(snap, off)
	if col == 0 {
		if l > 0 {
			return lineEnd(snap, l-1)
		}
		return off
	}
	text := snap.Line(l)
	ls := snap.LineStart(l)
	for c := col - 1; c > 0; c-- {
		if partBoundary(text, c, true) {
			return ls + c
		}
	}
	return ls
}

// moveWordPartRight is Monaco's WordOperations._moveWordPartRight.
func moveWordPartRight(snap buffer.Snapshot, off int) int {
	l, col := pos(snap, off)
	text := snap.Line(l)
	if col == len(text) {
		if l < snap.LineCount()-1 {
			return snap.LineStart(l + 1)
		}
		return off
	}
	ls := snap.LineStart(l)
	for c := col + 1; c < len(text); c++ {
		if partBoundary(text, c, false) {
			return ls + c
		}
	}
	return ls + len(text)
}

// partBoundary reports whether byte column c of text (0 < c < len) is a
// word part boundary moving left (before an underscore or dash run's end)
// or right (before its start).
func partBoundary(text string, c int, left bool) bool {
	l, r := text[c-1], text[c]
	if left {
		if (l == '_' && r != '_') || (l == '-' && r != '-') {
			return true
		}
	} else if (l != '_' && r == '_') || (l != '-' && r == '-') {
		return true
	}
	if (isLowerASCII(l) || isDigitASCII(l)) && isUpperASCII(r) {
		return true // camelCase
	}
	if isUpperASCII(l) && isUpperASCII(r) && c+1 < len(text) {
		rr := text[c+1]
		return isLowerASCII(rr) || isDigitASCII(rr) // HTMLParser: before the P
	}
	return false
}

// moveWordPartLeftAll is WordPartOperations.moveWordPartLeft: the nearest
// of the previous word start, word end and word part boundary.
func moveWordPartLeftAll(snap buffer.Snapshot, off int, multi bool) int {
	return max(moveWordLeft(snap, off, navWordStart, multi), moveWordLeft(snap, off, navWordEnd, multi), moveWordPartLeft(snap, off))
}

// moveWordPartRightAll is WordPartOperations.moveWordPartRight.
func moveWordPartRightAll(snap buffer.Snapshot, off int) int {
	return min(moveWordRight(snap, off, navWordStart), moveWordRight(snap, off, navWordEnd), moveWordPartRight(snap, off))
}

// --- deleting ---

// delRange is a range to delete; ok false is Monaco's null (nothing).
type delRange struct {
	a, b int
	ok   bool
}

// deleteWordLeftRange is Monaco's WordOperations.deleteWordLeft for sel.
func (s *State) deleteWordLeftRange(snap buffer.Snapshot, sel Selection, nav wordNav, heuristics bool) delRange {
	if sel.Anchor != sel.Head {
		a, b := ordered(sel)
		return delRange{a, b, true}
	}
	off := sel.Head
	if s.pairDeleteAt(sel) {
		a, b := pairRange(snap, off)
		return delRange{a, b, true}
	}
	if off == 0 {
		return delRange{}
	}
	l, col := pos(snap, off)
	text := snap.Line(l)
	ls := snap.LineStart(l)
	if heuristics {
		last := col - 2
		for last >= 0 && (text[last] == ' ' || text[last] == '\t') {
			last--
		}
		if last+1 < col-1 {
			return delRange{ls + last + 1, off, true}
		}
	}
	w, ok := prevWordOnLine(text, col)
	if nav != navWordStart && ok && col <= w.end {
		w, ok = prevWordOnLine(text, w.start)
	}
	switch {
	case ok && nav == navWordStart:
		return delRange{ls + w.start, off, true}
	case ok:
		return delRange{ls + w.end, off, true}
	case col > 0:
		return delRange{ls, off, true}
	}
	return delRange{lineEnd(snap, l-1), off, true}
}

// deleteWordRightRange is Monaco's WordOperations.deleteWordRight for sel.
func deleteWordRightRange(snap buffer.Snapshot, sel Selection, nav wordNav, heuristics bool) delRange {
	if sel.Anchor != sel.Head {
		a, b := ordered(sel)
		return delRange{a, b, true}
	}
	off := sel.Head
	l, col := pos(snap, off)
	text := snap.Line(l)
	ls := snap.LineStart(l)
	last := l == snap.LineCount()-1
	if last && col == len(text) {
		return delRange{}
	}
	if heuristics {
		fnw := col
		for fnw < len(text) && (text[fnw] == ' ' || text[fnw] == '\t') {
			fnw++
		}
		if col+1 < fnw {
			return delRange{off, ls + fnw, true}
		}
	}
	w, ok := nextWordOnLine(text, col)
	if nav != navWordEnd && ok && col >= w.start {
		w, ok = nextWordOnLine(text, w.end)
	}
	switch {
	case ok && nav == navWordEnd:
		return delRange{off, ls + w.end, true}
	case ok:
		return delRange{off, ls + w.start, true}
	case col < len(text) || last:
		return delRange{off, ls + len(text), true}
	}
	nl := snap.Line(l + 1)
	nls := snap.LineStart(l + 1)
	if w, ok := nextWordOnLine(nl, 0); ok {
		return delRange{off, nls + w.start, true}
	}
	return delRange{off, nls + len(nl), true}
}

// pairDeleteAt is autoClosingPairDelete for the one selection sel (Monaco's
// deleteWordLeft checks each cursor alone).
func (s *State) pairDeleteAt(sel Selection) bool {
	saved := s.Sels
	s.Sels = []Selection{sel}
	ok := s.autoClosingPairDelete()
	s.Sels = saved
	return ok
}

// deleteWordPartLeftRange is WordPartOperations.deleteWordPartLeft: of the
// word start, word end and word part deletes, the shortest.
func (s *State) deleteWordPartLeftRange(snap buffer.Snapshot, sel Selection) delRange {
	var cands []delRange
	for _, nav := range []wordNav{navWordStart, navWordEnd} {
		if r := s.deleteWordLeftRange(snap, sel, nav, true); r.ok {
			cands = append(cands, r)
		}
	}
	if sel.Anchor != sel.Head {
		a, b := ordered(sel)
		cands = append(cands, delRange{a, b, true})
	} else {
		cands = append(cands, delRange{moveWordPartLeft(snap, sel.Head), sel.Head, true})
	}
	if len(cands) < 3 {
		return delRange{} // Monaco picks the third candidate
	}
	best := cands[0]
	for _, c := range cands[1:] { // by end, then start: the last
		if c.b > best.b || (c.b == best.b && c.a >= best.a) {
			best = c
		}
	}
	return best
}

// deleteWordPartRightRange is WordPartOperations.deleteWordPartRight.
func deleteWordPartRightRange(snap buffer.Snapshot, sel Selection) delRange {
	var cands []delRange
	for _, nav := range []wordNav{navWordStart, navWordEnd} {
		if r := deleteWordRightRange(snap, sel, nav, true); r.ok {
			cands = append(cands, r)
		}
	}
	if sel.Anchor != sel.Head {
		a, b := ordered(sel)
		cands = append(cands, delRange{a, b, true})
	} else {
		cands = append(cands, delRange{sel.Head, moveWordPartRight(snap, sel.Head), true})
	}
	best := cands[0]
	for _, c := range cands[1:] { // by start, then end: the first
		if c.a < best.a || (c.a == best.a && c.b < best.b) {
			best = c
		}
	}
	return best
}

// deleteInsideWordRange is Monaco's WordOperations.deleteInsideWord.
func deleteInsideWordRange(snap buffer.Snapshot, sel Selection, onlyWord bool) delRange {
	if sel.Anchor != sel.Head {
		a, b := ordered(sel)
		return delRange{a, b, true}
	}
	off := sel.Head
	l, col := pos(snap, off)
	text := snap.Line(l)
	ls := snap.LineStart(l)
	blank := func(i int) bool { return i >= 0 && i < len(text) && (text[i] == ' ' || text[i] == '\t') }
	// Inside a run of blanks: delete the run.
	if text != "" {
		li, ri := max(col-1, 0), min(col, len(text)-1)
		if blank(li) && blank(ri) {
			for li > 0 && blank(li-1) {
				li--
			}
			for ri+1 < len(text) && blank(ri+1) {
				ri++
			}
			return delRange{ls + li, ls + ri + 1, true}
		}
	}
	if text == "" {
		switch {
		case l > 0:
			return delRange{lineEnd(snap, l-1), ls, true}
		case l < snap.LineCount()-1:
			return delRange{ls, snap.LineStart(l + 1), true}
		}
		return delRange{ls, ls, true}
	}
	touches := func(w lineWord) bool { return w.start <= col && col <= w.end }
	withPos := func(a, b int) delRange { return delRange{ls + min(a, col), ls + max(b, col), true} }
	wordAndBlanks := func(w lineWord) delRange {
		a, b := w.start, w.end
		if onlyWord {
			return withPos(a, b)
		}
		right := false
		for b < len(text) && blank(b) {
			right = true
			b++
		}
		if !right {
			for a > 0 && blank(a-1) {
				a--
			}
		}
		return withPos(a, b)
	}
	prev, pok := prevWordOnLine(text, col)
	if pok && touches(prev) {
		return wordAndBlanks(prev)
	}
	next, nok := nextWordOnLine(text, col)
	if nok && touches(next) {
		return wordAndBlanks(next)
	}
	switch {
	case pok && nok:
		return withPos(prev.end, next.start)
	case pok:
		return withPos(prev.start, prev.end)
	case nok:
		return withPos(next.start, next.end)
	}
	return withPos(0, len(text))
}

// --- commands ---

// moveWords moves every cursor to move(head) (keeping the anchors when
// extend), as Monaco's MoveWordCommand.
func (s *State) moveWords(extend bool, move func(snap buffer.Snapshot, off int, multi bool) int) {
	s.normalize()
	snap := s.Snapshot()
	sels := s.sels()
	out := make([]Selection, len(sels))
	for k, sel := range sels {
		to := move(snap, clampOff(snap, sel.Head), len(sels) > 1)
		if extend {
			out[k] = Selection{Anchor: sel.Anchor, Head: to}
		} else {
			out[k] = Selection{Anchor: to, Head: to}
		}
	}
	s.setSels(out, nil)
}

// deleteWords deletes the range rangeOf gives each selection, as one undo
// step of its own (Monaco's DeleteWordCommand).
func (s *State) deleteWords(rangeOf func(snap buffer.Snapshot, sel Selection) delRange) bool {
	s.normalize()
	snap := s.Snapshot()
	sels := s.clampedSels(snap)
	cmds := make([]opCmd, len(sels))
	for k, sel := range sels {
		r := rangeOf(snap, sel)
		if !r.ok || r.a == r.b {
			continue
		}
		cmds[k] = replaceCmd(k, buffer.Edit{Start: r.a, End: r.b})
	}
	return s.runOps(cmds)
}

// InsideWordArgs are the arguments of deleteInsideWord (Monaco's
// {onlyWord}): OnlyWord leaves the blanks around the word.
type InsideWordArgs struct{ OnlyWord bool }

func wordCommands() []Command {
	var cs []Command
	move := func(id, label string, f func(snap buffer.Snapshot, off int, multi bool) int) {
		for _, sel := range []bool{false, true} {
			c := Command{ID: id, Label: label, Run: func(st *State, _ any) bool {
				st.moveWords(sel, f)
				st.revealCaret()
				return true
			}}
			if sel {
				c.ID += "Select"
				c.Label += " Select"
			}
			cs = append(cs, c)
		}
	}
	left := func(nav wordNav) func(buffer.Snapshot, int, bool) int {
		return func(snap buffer.Snapshot, off int, multi bool) int { return moveWordLeft(snap, off, nav, multi) }
	}
	right := func(nav wordNav) func(buffer.Snapshot, int, bool) int {
		return func(snap buffer.Snapshot, off int, _ bool) int { return moveWordRight(snap, off, nav) }
	}
	move("cursorWordStartLeft", "Cursor Word Start Left", left(navWordStart))
	move("cursorWordEndLeft", "Cursor Word End Left", left(navWordEnd))
	move("cursorWordLeft", "Cursor Word Left", left(navWordStartFast))
	move("cursorWordStartRight", "Cursor Word Start Right", right(navWordStart))
	move("cursorWordEndRight", "Cursor Word End Right", right(navWordEnd))
	move("cursorWordRight", "Cursor Word Right", right(navWordEnd))
	move("cursorWordAccessibilityLeft", "Cursor Word Accessibility Left", left(navWordAccessibility))
	move("cursorWordAccessibilityRight", "Cursor Word Accessibility Right", right(navWordAccessibility))
	partLeft := func(snap buffer.Snapshot, off int, multi bool) int { return moveWordPartLeftAll(snap, off, multi) }
	partRight := func(snap buffer.Snapshot, off int, _ bool) int { return moveWordPartRightAll(snap, off) }
	move("cursorWordPartLeft", "Cursor Word Part Left", partLeft)
	move("cursorWordPartStartLeft", "Cursor Word Part Left", partLeft) // Monaco's alias
	move("cursorWordPartRight", "Cursor Word Part Right", partRight)

	del := func(id, label string, f func(st *State, snap buffer.Snapshot, sel Selection) delRange) {
		cs = append(cs, Command{ID: id, Label: label, Precondition: writable, Mutates: true, Run: func(st *State, _ any) bool {
			ok := st.deleteWords(func(snap buffer.Snapshot, sel Selection) delRange { return f(st, snap, sel) })
			st.revealCaret()
			return ok
		}})
	}
	delLeft := func(nav wordNav, heur bool) func(*State, buffer.Snapshot, Selection) delRange {
		return func(st *State, snap buffer.Snapshot, sel Selection) delRange {
			return st.deleteWordLeftRange(snap, sel, nav, heur)
		}
	}
	delRight := func(nav wordNav, heur bool) func(*State, buffer.Snapshot, Selection) delRange {
		return func(_ *State, snap buffer.Snapshot, sel Selection) delRange {
			return deleteWordRightRange(snap, sel, nav, heur)
		}
	}
	del("deleteWordStartLeft", "Delete Word Start Left", delLeft(navWordStart, false))
	del("deleteWordEndLeft", "Delete Word End Left", delLeft(navWordEnd, false))
	del("deleteWordLeft", "Delete Word Left", delLeft(navWordStart, true))
	del("deleteWordStartRight", "Delete Word Start Right", delRight(navWordStart, false))
	del("deleteWordEndRight", "Delete Word End Right", delRight(navWordEnd, false))
	del("deleteWordRight", "Delete Word Right", delRight(navWordEnd, true))
	del("deleteWordPartLeft", "Delete Word Part Left", func(st *State, snap buffer.Snapshot, sel Selection) delRange {
		return st.deleteWordPartLeftRange(snap, sel)
	})
	del("deleteWordPartRight", "Delete Word Part Right", func(_ *State, snap buffer.Snapshot, sel Selection) delRange {
		return deleteWordPartRightRange(snap, sel)
	})
	cs = append(cs, Command{ID: "deleteInsideWord", Label: "Delete Word", Precondition: writable, Mutates: true, Run: func(st *State, args any) bool {
		only := false
		switch a := args.(type) {
		case InsideWordArgs:
			only = a.OnlyWord
		case *InsideWordArgs:
			only = a != nil && a.OnlyWord
		case map[string]any:
			only, _ = a["onlyWord"].(bool)
		}
		ok := st.deleteWords(func(snap buffer.Snapshot, sel Selection) delRange { return deleteInsideWordRange(snap, sel, only) })
		st.revealCaret()
		return ok
	}})
	return cs
}
