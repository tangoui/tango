package editor

import (
	"regexp"
	"strings"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/langs"
	"github.com/tangoui/tango/ui/editor/regex"
)

// Enter and the auto-indentation (Monaco 0.56's EnterOperation, onEnter.js,
// enterAction.js, autoIndent.js, TabOperation._goodIndentForLine,
// InterceptorElectricCharOperation and MoveLinesCommand's autoIndent path),
// by the Options.AutoIndent level:
//
//   - none: Enter inserts a bare line break;
//   - keep: Enter keeps the indentation of the caret's line (up to the
//     caret);
//   - brackets: Enter after an opening bracket indents, and between a
//     bracket pair puts the caret on an indented line of its own with the
//     closing bracket on the line after;
//   - advanced: the language's onEnterRules apply first (JSDoc " * ",
//     Python's indent after ":");
//   - full (the default): the language's indentation rules also give the
//     indentation of the new line, of a line whose typed character makes it
//     match decreaseIndentPattern, of Tab on a blank line and of lines
//     moved up or down.
//
// The text the rules see is the line with the language's bracket strings
// removed from its string, comment and regex tokens, as Monaco processes
// it. Line numbers in the functions ported from autoIndent.js are 1-based,
// as in Monaco, to keep their boundary checks recognisable.

// --- onEnter rules (Monaco's OnEnterSupport) ---

type enterRule struct {
	before, after, prev regex.Regexp
	action              langs.EnterAction
}

type enterBracket struct{ open, close regex.Regexp }

// onEnterSupport is a language's compiled onEnter support.
type onEnterSupport struct {
	rules    []enterRule
	brackets []enterBracket
	needPrev bool // a rule has previousLineText
}

// newOnEnterSupport compiles conf's onEnter support; nil when the language
// has neither brackets, indentation rules nor onEnter rules (Monaco has no
// OnEnterSupport then). A rule whose patterns do not compile is dropped;
// the brackets default to (), {} and [].
func newOnEnterSupport(conf *langs.Conf) *onEnterSupport {
	ind := conf.Indentation
	hasIndent := ind.Increase != nil || ind.Decrease != nil || ind.IndentNextLine != nil || ind.Unindented != nil
	if conf.Brackets == nil && !hasIndent && conf.OnEnterRules == nil {
		return nil
	}
	o := &onEnterSupport{}
	for _, r := range conf.OnEnterRules {
		er := enterRule{before: compileTest(r.BeforeText), action: r.Action}
		ok := er.before != nil
		if r.AfterText != nil {
			er.after = compileTest(r.AfterText)
			ok = ok && er.after != nil
		}
		if r.PreviousLineText != nil {
			er.prev = compileTest(r.PreviousLineText)
			ok = ok && er.prev != nil
		}
		if ok {
			o.rules = append(o.rules, er)
			o.needPrev = o.needPrev || er.prev != nil
		}
	}
	brackets := conf.Brackets
	if brackets == nil {
		brackets = [][2]string{{"(", ")"}, {"{", "}"}, {"[", "]"}}
	}
	for _, b := range brackets {
		if b[0] == "" || b[1] == "" {
			continue
		}
		open := regexp.QuoteMeta(b[0])
		if isWordByte(b[0][0]) {
			open = `\b` + open
		}
		closeRe := `^\s*` + regexp.QuoteMeta(b[1])
		if isWordByte(b[1][len(b[1])-1]) {
			closeRe += `\b`
		}
		eb := enterBracket{
			open:  compileTest(&langs.RegexSpec{Source: open + `\s*$`}),
			close: compileTest(&langs.RegexSpec{Source: closeRe}),
		}
		if eb.open != nil && eb.close != nil {
			o.brackets = append(o.brackets, eb)
		}
	}
	return o
}

// isWordByte reports whether b is matched by JavaScript's \w.
func isWordByte(b byte) bool {
	return b == '_' || '0' <= b && b <= '9' || 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z'
}

// onEnter is Monaco's OnEnterSupport.onEnter: the first onEnter rule
// matching (at level advanced and above), else indentOutdent between a
// bracket pair, else indent after an opening bracket (level brackets and
// above).
func (o *onEnterSupport) onEnter(level int, prev, before, after string) (langs.EnterAction, bool) {
	if level >= indentAdvanced {
		for _, r := range o.rules {
			if test(r.before, before) && (r.after == nil || test(r.after, after)) && (r.prev == nil || test(r.prev, prev)) {
				return r.action, true
			}
		}
	}
	if level >= indentBrackets && before != "" {
		if after != "" {
			for _, b := range o.brackets {
				if test(b.open, before) && test(b.close, after) {
					return langs.EnterAction{Indent: "indentOutdent"}, true
				}
			}
		}
		for _, b := range o.brackets {
			if test(b.open, before) {
				return langs.EnterAction{Indent: "indent"}, true
			}
		}
	}
	return langs.EnterAction{}, false
}

// --- the indentation context ---

// indentCtx is the text model the auto-indent functions read (Monaco's
// ITextModel or one of its "virtual models"): the document's lines, with
// optional overrides. Line numbers are 1-based.
type indentCtx struct {
	st    *State
	snap  buffer.Snapshot
	tc    *typingConf
	rules *indentRules // nil: the language has no indentation rules

	mapper   func(n int) int            // the document line a line's tokens come from; nil: itself
	override func(n int) (string, bool) // a line's content override
	procOver func(n int) (string, bool) // a line's processed-text override
}

func (s *State) indentCtx(snap buffer.Snapshot) *indentCtx {
	return &indentCtx{st: s, snap: snap, tc: s.typing(), rules: rulesFor(s.Model.Conf())}
}

// with returns a copy of c with the given overrides (a virtual model).
func (c *indentCtx) with(mapper func(int) int, override, procOver func(int) (string, bool)) *indentCtx {
	v := *c
	v.mapper, v.override, v.procOver = mapper, override, procOver
	return &v
}

func (c *indentCtx) real(n int) int {
	if c.mapper != nil {
		return c.mapper(n)
	}
	return n
}

// content returns line n's text.
func (c *indentCtx) content(n int) string {
	if c.override != nil {
		if t, ok := c.override(n); ok {
			return t
		}
	}
	return c.snap.Line(c.real(n) - 1)
}

// processed returns line n's text as the indentation rules see it: the
// bracket strings removed from its string, comment and regex tokens (the
// tokens, and so the text, of the line the mapper gives).
func (c *indentCtx) processed(n int) string {
	if c.procOver != nil {
		if t, ok := c.procOver(n); ok {
			return t
		}
	}
	i := c.real(n) - 1
	return c.st.processedLine(c.snap, i, c.tc)
}

// processedLine returns document line i (0-based) with the bracket strings
// removed from its string, comment and regex tokens.
func (s *State) processedLine(snap buffer.Snapshot, i int, tc *typingConf) string {
	text := snap.Line(i)
	return s.processedSlice(snap, i, text, 0, len(text), tc)
}

// processedSlice returns text[from:to], text being line i (0-based),
// processed as processedLine. Lines without a bracket string are not
// tokenized.
func (s *State) processedSlice(snap buffer.Snapshot, i int, text string, from, to int, tc *typingConf) string {
	if tc.procBrackets == nil || !tc.procBrackets.MatchString(text[from:to]) {
		return text[from:to]
	}
	return processBrackets(text, s.lineToks(snap, i, text), tc.procBrackets, from, to)
}

func (c *indentCtx) shouldIncrease(n int) bool { return test(c.rules.increase, c.processed(n)) }
func (c *indentCtx) shouldDecrease(n int) bool { return test(c.rules.decrease, c.processed(n)) }
func (c *indentCtx) shouldIndentNext(n int) bool {
	return test(c.rules.indentNext, c.processed(n))
}
func (c *indentCtx) shouldIgnore(n int) bool { return test(c.rules.unindented, c.processed(n)) }

// Indent metadata bits (Monaco's IndentConsts), of a raw line.
const (
	metaIncrease = 1 << iota
	metaDecrease
	metaIndentNext
	metaUnindent
)

func (r *indentRules) metadata(text string) int {
	m := 0
	if test(r.increase, text) {
		m |= metaIncrease
	}
	if test(r.decrease, text) {
		m |= metaDecrease
	}
	if test(r.indentNext, text) {
		m |= metaIndentNext
	}
	if test(r.unindented, text) {
		m |= metaUnindent
	}
	return m
}

// shift and unshift are Monaco's indentConverter: one level deeper or
// shallower than the visible width of ind, in the Model's style.
func (s *State) shift(ind string) string {
	ts := s.indentSize()
	return shiftIndent(indentColumn(ind, ts), ts, s.insertSpaces())
}

func (s *State) unshift(ind string) string {
	ts := s.indentSize()
	return unshiftIndent(indentColumn(ind, ts), ts, s.insertSpaces())
}

// norm is the CursorConfiguration's normalizeIndentation.
func (s *State) norm(str string) string {
	return normalizeIndentation(str, s.indentSize(), s.insertSpaces())
}

// --- autoIndent.js ---

// inherited is the result of getInheritIndentForLine; line 0 means none.
type inherited struct {
	indentation string
	indent      bool // action IndentAction.Indent
	line        int
}

// precedingValidLine is Monaco's getPrecedingValidLine: the nearest line
// above n that is neither blank nor matches unIndentedLinePattern, -1 when
// there is none (Monaco's resultLineNumber only matters at an embedded
// language's boundary, which the editor does not have).
func (c *indentCtx) precedingValidLine(n int) int {
	for last := n - 1; last >= 1; last-- {
		if c.shouldIgnore(last) || isBlankLine(c.content(last)) {
			continue
		}
		return last
	}
	return -1
}

// inheritIndent is Monaco's getInheritIndentForLine (autoIndent "full"
// only, which callers check): the indentation line n inherits from the
// lines above it, false when it cannot tell.
func (c *indentCtx) inheritIndent(n int, honor bool) (inherited, bool) {
	if c.rules == nil {
		return inherited{}, false
	}
	if n <= 1 {
		return inherited{}, true
	}
	// No indentation when every line above is empty.
	for p := n - 1; p > 0; p-- {
		if c.content(p) != "" {
			break
		}
		if p == 1 {
			return inherited{}, true
		}
	}
	p := c.precedingValidLine(n)
	if p < 0 {
		return inherited{}, false
	} else if p < 1 {
		return inherited{}, true
	}
	ws := func(l int) string { return leadingBlanks(c.content(l)) }
	if c.shouldIncrease(p) || c.shouldIndentNext(p) {
		return inherited{indentation: ws(p), indent: true, line: p}, true
	}
	if c.shouldDecrease(p) {
		return inherited{indentation: ws(p), line: p}, true
	}
	if p == 1 {
		return inherited{indentation: ws(1), line: 1}, true
	}
	prev := p - 1
	if m := c.rules.metadata(c.content(prev)); m&(metaIncrease|metaDecrease) == 0 && m&metaIndentNext != 0 {
		stop := 0
		for i := prev - 1; i > 0; i-- {
			if c.shouldIndentNext(i) {
				continue
			}
			stop = i
			break
		}
		return inherited{indentation: ws(stop + 1), line: stop + 1}, true
	}
	if honor {
		return inherited{indentation: ws(p), line: p}, true
	}
	for i := p; i > 0; i-- {
		switch {
		case c.shouldIncrease(i):
			return inherited{indentation: ws(i), indent: true, line: i}, true
		case c.shouldIndentNext(i):
			// Monaco's inner search tests line i, not j, so it never stops
			// early: the result is always line 1.
			return inherited{indentation: ws(1), line: 1}, true
		case c.shouldDecrease(i):
			return inherited{indentation: ws(i), line: i}, true
		}
	}
	return inherited{indentation: ws(1), line: 1}, true
}

// goodIndentForLine is Monaco's getGoodIndentForLine: the indentation line
// n should have by the rules (autoIndent "full"), false when they do not
// tell.
func (c *indentCtx) goodIndentForLine(n int, level int) (string, bool) {
	if level < indentFull || c.rules == nil {
		return "", false
	}
	s := c.st
	ind, ok := c.inheritIndent(n, true)
	if !ok {
		return "", false
	}
	if ind.line != 0 {
		apply := true
		for between := ind.line; between < n-1; between++ {
			if !isBlankLine(c.content(between)) {
				apply = false
				break
			}
		}
		if apply && c.tc.onEnter != nil {
			if r, ok := c.tc.onEnter.onEnter(level, "", c.content(ind.line), ""); ok {
				indentation := leadingBlanks(c.content(ind.line))
				if r.RemoveText > 0 {
					indentation = indentation[:max(0, len(indentation)-r.RemoveText)]
				}
				switch r.Indent {
				case "indent", "indentOutdent":
					indentation = s.shift(indentation)
				case "outdent":
					indentation = s.unshift(indentation)
				}
				if c.shouldDecrease(n) {
					indentation = s.unshift(indentation)
				}
				indentation += r.AppendText
				return leadingBlanks(indentation), true
			}
		}
	}
	if c.shouldDecrease(n) {
		if ind.indent {
			return ind.indentation, true
		}
		return s.unshift(ind.indentation), true
	}
	if ind.indent {
		return s.shift(ind.indentation), true
	}
	return ind.indentation, true
}

// enterResult is Monaco's CompleteEnterAction.
type enterResult struct {
	indent      string // "none", "indent", "indentOutdent", "outdent"
	appendText  string
	removeText  int
	indentation string
}

// enterAction is Monaco's getEnterAction for Enter replacing [a, b): the
// onEnter rules over the processed text before a, after b and of the line
// above.
func (s *State) enterAction(snap buffer.Snapshot, a, b, level int) (enterResult, bool) {
	tc := s.typing()
	if tc.onEnter == nil {
		return enterResult{}, false
	}
	l := snap.LineOf(a)
	text := snap.Line(l)
	col := a - snap.LineStart(l)
	before := s.processedSlice(snap, l, text, 0, col, tc)
	bl := snap.LineOf(b)
	btext := snap.Line(bl)
	after := s.processedSlice(snap, bl, btext, b-snap.LineStart(bl), len(btext), tc)
	prev := ""
	if l > 0 && tc.onEnter.needPrev && level >= indentAdvanced {
		prev = s.processedLine(snap, l-1, tc)
	}
	r, ok := tc.onEnter.onEnter(level, prev, before, after)
	if !ok {
		return enterResult{}, false
	}
	app := r.AppendText
	switch {
	case app == "" && (r.Indent == "indent" || r.Indent == "indentOutdent"):
		app = "\t"
	case app != "" && r.Indent == "indent":
		app = "\t" + app
	}
	ind := leadingBlanks(text)
	ind = ind[:min(len(ind), col)]
	if r.RemoveText > 0 {
		ind = ind[:max(0, len(ind)-r.RemoveText)]
	}
	return enterResult{indent: r.Indent, appendText: app, removeText: r.RemoveText, indentation: ind}, true
}

// indentForEnter is Monaco's getIndentForEnter (autoIndent "full"): the
// indentation of the new line after Enter replaces [a, b), by the
// indentation rules, false when the language has none.
func (s *State) indentForEnter(snap buffer.Snapshot, a, b, level int) (afterEnter string, ok bool) {
	c := s.indentCtx(snap)
	if level < indentFull || c.rules == nil {
		return "", false
	}
	l := snap.LineOf(a)
	text := snap.Line(l)
	before := s.processedSlice(snap, l, text, 0, a-snap.LineStart(l), c.tc)
	bl := snap.LineOf(b)
	btext := snap.Line(bl)
	after := s.processedSlice(snap, bl, btext, b-snap.LineStart(bl), len(btext), c.tc)
	beforeIndent := leadingBlanks(before)
	n := l + 1 // 1-based line of a
	own := func(m int) (string, bool) {
		if m == n {
			return before, true
		}
		return "", false
	}
	v := c.with(nil, own, own)
	ae, ok := v.inheritIndent(n+1, true)
	if !ok {
		return beforeIndent, true
	}
	ind := ae.indentation
	if ae.indent {
		ind = s.shift(ind)
	}
	if test(c.rules.decrease, after) {
		ind = s.unshift(ind)
	}
	return ind, true
}

// --- Enter ---

// enter is Enter at every selection (Monaco's EnterOperation): an undo
// step of its own (Monaco continues the step with the typing that
// follows; the editor keeps milestone 1's grouping).
func (s *State) enter() bool {
	snap := s.Snapshot()
	sels := s.Sels
	cmds := make([]opCmd, len(sels))
	for k, sel := range sels {
		a, b := ordered(sel)
		cmds[k] = s.enterCmd(snap, k, a, b)
	}
	return s.runOps(cmds)
}

// enterCmd is Monaco's EnterOperation._enter for command k replacing
// [a, b) (keepPosition false).
func (s *State) enterCmd(snap buffer.Snapshot, k, a, b int) opCmd {
	level := s.Options.autoIndent()
	at := func(start, end int, text string, caret func(r *opResult) int) opCmd {
		return opCmd{edits: []buffer.Edit{{Start: start, End: end, Text: text}}, cursor: func(r *opResult, sel Selection) Selection {
			if r.inv[k] == nil {
				return r.track(sel)
			}
			c := caret(r)
			return Selection{Anchor: c, Head: c}
		}}
	}
	plain := func(text string) opCmd {
		return at(a, b, text, func(r *opResult) int { return r.inv[k][0][1] })
	}
	if level == indentNone {
		return plain("\n")
	}
	l := snap.LineOf(a)
	text := snap.Line(l)
	col := a - snap.LineStart(l)
	indentation := leadingBlanks(text)
	indentation = indentation[:min(len(indentation), col)]
	if level == indentKeep || !s.cheapTokens(snap, l) {
		return plain("\n" + s.norm(indentation))
	}
	if r, ok := s.enterAction(snap, a, b, level); ok {
		switch r.indent {
		case "indentOutdent":
			normal := s.norm(r.indentation)
			inc := s.norm(r.indentation + r.appendText)
			return at(a, b, "\n"+inc+"\n"+normal, func(r *opResult) int { return r.inv[k][0][0] + 1 + len(inc) })
		case "outdent":
			return plain("\n" + s.norm(s.unshift(r.indentation)+r.appendText))
		default: // none, indent
			return plain("\n" + s.norm(r.indentation+r.appendText))
		}
	}
	if level >= indentFull {
		if ae, ok := s.indentForEnter(snap, a, b, level); ok {
			ts := s.indentSize()
			viewCol := s.columnWith(snap, b, ts)
			el := snap.LineOf(b)
			els := snap.LineStart(el)
			ecol := b - els
			fnw := firstNonBlank(snap.Line(el))
			end := lineEnd(snap, el)
			if fnw >= 0 {
				end = els + max(ecol, fnw)
			}
			ins := s.norm(ae)
			off := 0
			if ecol <= fnw {
				if !s.insertSpaces() {
					viewCol = (viewCol + ts - 1) / ts
				}
				off = min(viewCol+1-len(ins)-1, 0)
			}
			return at(a, end, "\n"+ins, func(r *opResult) int {
				return max(r.inv[k][0][1]+off, r.inv[k][0][0]+1)
			})
		}
	}
	return plain("\n" + s.norm(indentation))
}

// --- auto-indent on type (Monaco's AutoIndentOperation) ---

// indentForType is Monaco's getIndentActionForType for typing ch over
// [a, b): the indentation the line gets, false for none.
func (s *State) indentForType(c *indentCtx, a, b int, ch string, level int) (string, bool) {
	if level < indentFull || c.rules == nil {
		return "", false
	}
	snap := c.snap
	l := snap.LineOf(a)
	text := snap.Line(l)
	before := s.processedSlice(snap, l, text, 0, a-snap.LineStart(l), c.tc)
	bl := snap.LineOf(b)
	btext := snap.Line(bl)
	after := s.processedSlice(snap, bl, btext, b-snap.LineStart(bl), len(btext), c.tc)
	around, with := before+after, before+ch+after
	n := l + 1
	if !test(c.rules.decrease, around) && test(c.rules.decrease, with) {
		r, ok := c.inheritIndent(n, false)
		if !ok {
			return "", false
		}
		ind := r.indentation
		if !r.indent {
			ind = s.unshift(ind)
		}
		return ind, true
	}
	if n > 1 && test(c.rules.indentNext, snap.Line(l-1)) && test(c.rules.increase, with) {
		if r, ok := c.inheritIndent(n, false); ok {
			eq := s.shift(r.indentation) == leadingBlanks(text)
			if eq && len(c.tc.openByEnd[ch]) > 0 && isBlankLine(around) {
				return r.indentation, true
			}
		}
	}
	return "", false
}

// autoIndentType re-indents the line of every selection when typing ch
// makes it match the indentation rules (Monaco's AutoIndentOperation), and
// types ch (with its auto-closing pair when one applies). It does nothing
// unless every selection gets a new indentation.
func (s *State) autoIndentType(ch string) bool {
	level := s.Options.autoIndent()
	if level < indentFull || rulesFor(s.Model.Conf()) == nil {
		return false
	}
	snap := s.Snapshot()
	c := s.indentCtx(snap)
	if !s.cheapTokens(snap, s.lastSelLine(snap)) {
		return false
	}
	type job struct {
		a, b   int
		indent string
	}
	jobs := make([]job, len(s.Sels))
	for k, sel := range s.Sels {
		a, b := ordered(sel)
		ind, ok := s.indentForType(c, a, b, ch, level)
		if !ok {
			return false
		}
		l := snap.LineOf(a)
		cur := leadingBlanks(snap.Line(l))
		cur = cur[:min(len(cur), a-snap.LineStart(l))]
		if ind == s.norm(cur) {
			return false
		}
		jobs[k] = job{a, b, ind}
	}
	// The edit replacing the start of the line up to the selection's end:
	// the new indentation and the text from the first non-blank character
	// to the selection's start (Monaco's _getEditFromIndentationAndSelection).
	edit := func(j job, withCh bool) buffer.Edit {
		l := snap.LineOf(j.a)
		ls := snap.LineStart(l)
		text := snap.Line(l)
		t := s.norm(j.indent)
		if fnw := firstNonBlank(text); fnw >= 0 {
			x, y := fnw, j.a-ls
			if x > y {
				x, y = y, x // JavaScript's substring swaps its arguments
			}
			t += text[x:y]
		}
		if withCh {
			t += ch
		}
		return buffer.Edit{Start: ls, End: j.b, Text: t}
	}
	if closeText, ok := s.autoClosingPairClose(ch); ok {
		ind := make([]buffer.Edit, len(jobs))
		for k, j := range jobs {
			e := edit(j, false)
			e.End = j.a // the selections are carets here
			ind[k] = e
		}
		return s.typeWithAutoClosing(ch, closeText, ind)
	}
	cmds := make([]opCmd, len(jobs))
	for k, j := range jobs {
		cmds[k] = replaceCmd(k, edit(j, true))
	}
	s.Model.buf.PushUndoStop()
	return s.runOpsKind(cmds, kindType)
}

// --- electric brackets (Monaco's InterceptorElectricCharOperation) ---

// electricBudget bounds the search for an opening bracket (Monaco gives
// findMatchingBracketUp 500 ms; a keystroke cannot wait that long).
const electricBudget = 20 * time.Millisecond

// electricChar types a closing bracket that is the first character of its
// line at the indentation of the line of its opening bracket (one caret
// only).
func (s *State) electricChar(ch string) bool {
	tc := s.typing()
	if len(s.Sels) != 1 || !tc.electric[ch] {
		return false
	}
	sel := s.Sels[0]
	if sel.Anchor != sel.Head {
		return false
	}
	snap := s.Snapshot()
	l := snap.LineOf(sel.Head)
	if !s.cheapTokens(snap, l) {
		return false
	}
	ls := snap.LineStart(l)
	text := snap.Line(l)
	col := sel.Head - ls
	// Not in a string, comment or regex.
	if stdType := tokenAt(s.lineToks(snap, l, text), col); stdType != "" {
		return false
	}
	t := asciiLower(text[:col] + ch)
	// The last bracket of the text up to and with ch.
	st, br := -1, ""
	for e := len(t); e > 0 && st < 0; e-- {
		for _, b := range tc.brackets {
			for _, x := range b {
				if x != "" && strings.HasSuffix(t[:e], x) && len(x) > len(br) && wholeBracket(t, e-len(x), x) {
					st, br = e-len(x), x
				}
			}
		}
	}
	if st < 0 {
		return false
	}
	if _, isOpen := tc.openBracket[br]; isOpen {
		return false
	}
	pi, isClose := tc.closeBracket[br]
	if !isClose || !isBlankLine(text[:min(st, len(text))]) {
		return false
	}
	ml, ok := s.matchBracketUp(snap, tc.brackets[pi], l, strings.LastIndex(t, br))
	if !ok || ml == l {
		return false
	}
	prefix := ""
	if fnw := firstNonBlank(text); fnw >= 0 {
		x, y := fnw, col
		if x > y {
			x, y = y, x
		}
		prefix = text[x:y]
	}
	typeText := s.norm(leadingBlanks(snap.Line(ml))) + prefix + ch
	if !s.runOpsKind([]opCmd{replaceCmd(0, buffer.Edit{Start: ls, End: sel.Head, Text: typeText})}, kindType) {
		return false
	}
	s.Model.buf.PushUndoStop()
	s.lastKind = kindNone
	return true
}

// matchBracketUp returns the line of the opening bracket of pair that
// matches a closing bracket at byte column col of line l, searching up
// (Monaco's legacy _findMatchingBracketUp): brackets in string, comment
// and regex tokens are skipped. False when none is found within
// electricBudget.
func (s *State) matchBracketUp(snap buffer.Snapshot, pair [2]string, l, col int) (int, bool) {
	open, closeText := pair[0], pair[1]
	deadline := time.Now().Add(electricBudget)
	count := -1
	for line := l; line >= 0; line-- {
		if (l-line)%64 == 63 && time.Now().After(deadline) {
			return 0, false
		}
		text := snap.Line(line)
		end := len(text)
		if line == l {
			end = min(col, len(text))
		}
		low := asciiLower(text) // same byte offsets as text
		if !strings.Contains(low[:end], open) && !strings.Contains(low[:end], closeText) {
			continue
		}
		toks := s.lineToks(snap, line, text)
		for i := end - 1; i >= 0; i-- {
			var d int
			switch {
			case strings.HasPrefix(low[i:end], open) && wholeBracket(low, i, open):
				d = 1
			case strings.HasPrefix(low[i:end], closeText) && wholeBracket(low, i, closeText):
				d = -1
			default:
				continue
			}
			if tokenAt(toks, i) != "" {
				continue
			}
			count += d
			if count == 0 {
				return line, true
			}
		}
	}
	return 0, false
}

// wholeBracket reports whether bracket b at byte i of text is a bracket by
// Monaco's bracket regexps: a word bracket ("end") only as a whole word,
// not inside "append".
func wholeBracket(text string, i int, b string) bool {
	if !wordBracket(b) {
		return true
	}
	j := i + len(b)
	return (i == 0 || !isWordByte(text[i-1])) && (j >= len(text) || !isWordByte(text[j]))
}

// --- Tab in a blank line (Monaco's TabOperation._goodIndentForLine) ---

// goodIndentForTab returns the indentation Tab gives the blank line i
// (0-based), false for none.
func (s *State) goodIndentForTab(snap buffer.Snapshot, i int) (string, bool) {
	level := s.Options.autoIndent()
	c := s.indentCtx(snap)
	n := i + 1
	indentation, indent := "", false
	var inh inherited
	ok := false
	if level >= indentFull {
		inh, ok = c.inheritIndent(n, false)
	}
	if ok {
		indentation, indent = inh.indentation, inh.indent
	} else if n > 1 {
		last := n - 1
		for ; last >= 1; last-- {
			if lastNonBlank(snap.Line(last-1)) >= 0 {
				break
			}
		}
		if last < 1 {
			return "", false
		}
		end := lineEnd(snap, last-1)
		if r, ok := s.enterAction(snap, end, end, level); ok {
			indentation = r.indentation + r.appendText
		}
	}
	if indent {
		indentation = s.norm(s.shift(indentation))
	}
	if indentation == "" {
		return "", false
	}
	return indentation, true
}

// --- moving lines (MoveLinesCommand with autoIndent "full") ---

// spaceCount is Monaco's getSpaceCnt: a tab counts tabSize columns.
func spaceCount(ind string, ts int) int {
	n := 0
	for i := 0; i < len(ind); i++ {
		if ind[i] == '\t' {
			n += ts
		} else {
			n++
		}
	}
	return n
}

// generateIndent is Monaco's generateIndent.
func generateIndent(n, ts int, spaces bool) string {
	n = max(n, 0)
	if spaces {
		return strings.Repeat(" ", n)
	}
	return strings.Repeat("\t", n/ts) + strings.Repeat(" ", n%ts)
}

// moveIndent is the indentation adjustment of a block of lines moved by
// moveLines: the block's lines change their indentation by blockDelta
// columns, and a line moved down over the block is inserted as moving.
type moveIndent struct {
	blockDelta int
	moving     string // the text of the line moved down past the block
}

// moveAutoIndent computes MoveLinesCommand's autoIndent adjustment for the
// block of lines [f, l] (0-based) moving down or up; ok is false when the
// lines keep their indentation (autoIndent below "full", a language
// without indentation rules, lines not cheap to tokenize).
func (s *State) moveAutoIndent(snap buffer.Snapshot, f, l int, down bool) (moveIndent, bool) {
	level := s.Options.autoIndent()
	c := s.indentCtx(snap)
	if level < indentFull || c.rules == nil || !s.cheapTokens(snap, f) {
		return moveIndent{}, false
	}
	ts, spaces := s.indentSize(), s.insertSpaces()
	start, end := f+1, l+1 // 1-based, Monaco's s.startLineNumber / endLineNumber
	line := func(n int) string { return snap.Line(n - 1) }
	trimStart := func(t string) string { return strings.TrimLeftFunc(t, isJSSpace) }
	// parseEnterResult: the indentation change of line n by an enter
	// action, false when the action does not apply to it.
	parse := func(n int, r enterResult, ok bool) (int, bool) {
		if !ok {
			return 0, false
		}
		prefix := r.indentation
		switch r.indent {
		case "none", "indent":
			prefix = r.indentation + r.appendText
		case "outdent":
			prefix = s.unshift(r.indentation) + r.appendText
		}
		if !strings.Contains(trimStart(line(n)), trimStart(prefix)) {
			return 0, false
		}
		ni := leadingBlanks(prefix)
		if c.rules.metadata(line(n))&metaDecrease != 0 {
			ni = s.unshift(ni)
		}
		return spaceCount(ni, ts) - spaceCount(leadingBlanks(line(n)), ts), true
	}
	enterAtEnd := func(n int) (enterResult, bool) {
		e := lineEnd(snap, n-1)
		return s.enterAction(snap, e, e, level)
	}
	// matchEnterRule: the enter action at the end of the nearest non-blank
	// line at or above above, applied to line n.
	matchEnterRule := func(n, above int, aboveText *string) (int, bool) {
		v := above
		for v >= 1 {
			t := line(v)
			if v == above && aboveText != nil {
				t = *aboveText
			}
			if lastNonBlank(t) >= 0 {
				break
			}
			v--
		}
		if v < 1 || n > snap.LineCount() {
			return 0, false
		}
		r, ok := enterAtEnd(v)
		return parse(n, r, ok)
	}
	var res moveIndent
	if down {
		m := end + 1 // the moving line
		mt := line(m)
		inserting := mt
		if d, ok := matchEnterRule(m, start-1, nil); ok {
			inserting = generateIndent(d+spaceCount(leadingBlanks(mt), ts), ts, spaces) + trimStart(mt)
		} else {
			v := c.with(func(n int) int {
				if n == start {
					return m
				}
				return n
			}, nil, nil)
			if gi, ok := v.goodIndentForLine(start, level); ok {
				ns, os := spaceCount(gi, ts), spaceCount(leadingBlanks(mt), ts)
				if ns != os {
					inserting = generateIndent(ns, ts, spaces) + trimStart(mt)
				}
			}
		}
		res.moving = inserting
		// matchEnterRuleMovingDown
		var d int
		var ok bool
		if lastNonBlank(inserting) >= 0 {
			r, rok := enterAtEnd(m)
			d, ok = parse(start, r, rok)
		} else {
			d, ok = matchEnterRule(start, start-1, nil)
		}
		if ok {
			res.blockDelta = d
			return res, true
		}
		v := c.with(func(n int) int {
			switch {
			case n == start:
				return m
			case n >= start+1 && n <= end+1:
				return n - 1
			}
			return n
		}, func(n int) (string, bool) {
			if n == start {
				return inserting, true
			}
			return "", false
		}, nil)
		if gi, ok := v.goodIndentForLine(start+1, level); ok {
			res.blockDelta = spaceCount(gi, ts) - spaceCount(leadingBlanks(line(start)), ts)
		}
		return res, true
	}
	moving := start - 1
	if d, ok := matchEnterRule(start, start-2, nil); ok {
		res.blockDelta = d
		return res, true
	}
	v := c.with(func(n int) int {
		if n == moving {
			return start
		}
		return n
	}, nil, nil)
	if gi, ok := v.goodIndentForLine(moving, level); ok {
		res.blockDelta = spaceCount(gi, ts) - spaceCount(leadingBlanks(line(start)), ts)
	}
	return res, true
}

// blockIndentEdits returns the edits that change the indentation of lines
// [f, l] (0-based) by delta columns (Monaco's getIndentEditsOfMovingBlock),
// and per line the old and new indentation lengths.
func (s *State) blockIndentEdits(snap buffer.Snapshot, f, l, delta int) ([]buffer.Edit, map[int][2]int) {
	if delta == 0 {
		return nil, nil
	}
	ts, spaces := s.indentSize(), s.insertSpaces()
	var es []buffer.Edit
	lens := map[int][2]int{}
	for i := f; i <= l; i++ {
		old := leadingBlanks(snap.Line(i))
		ni := generateIndent(spaceCount(old, ts)+delta, ts, spaces)
		if ni != old {
			ls := snap.LineStart(i)
			es = append(es, buffer.Edit{Start: ls, End: ls + len(old), Text: ni})
			lens[i] = [2]int{len(old), len(ni)}
		}
	}
	return es, lens
}
