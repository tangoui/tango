package editor

import (
	"regexp"
	"strings"
	"sync"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/langs"
	"github.com/tangoui/tango/ui/editor/monarch"
	"github.com/tangoui/tango/ui/editor/regex"
)

// Indentation commands: Tab and Shift+Tab, indent and outdent lines
// (Monaco's ShiftCommand and TabOperation), re-indent by the language's
// indentation rules (getReindentEditOperations) and convert the
// indentation between spaces and tabs.
//
// Monaco's indentSize is its tabSize (indentSize "tabSize", the default);
// useTabStops is true (its default): indenting moves the indentation to
// the next or previous multiple of the tab size, except that a line
// indented off the tab stops by the spaces an onEnter rule appended (a
// JSDoc " * " line) keeps those spaces after its indentation.

func init() {
	for _, c := range indentCommands() {
		RegisterCommand(c)
	}
}

func indentCommands() []Command {
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
		edit("outdent", "Outdent", func(st *State) bool { return st.shiftLines(true) }),
		edit("editor.action.indentLines", "Indent Line", func(st *State) bool { return st.shiftLines(false) }),
		edit("editor.action.outdentLines", "Outdent Line", func(st *State) bool { return st.shiftLines(true) }),
		edit("editor.action.reindentlines", "Reindent Lines", (*State).reindentLines),
		edit("editor.action.indentationToSpaces", "Convert Indentation to Spaces", func(st *State) bool { return st.convertIndentation(true) }),
		edit("editor.action.indentationToTabs", "Convert Indentation to Tabs", func(st *State) bool { return st.convertIndentation(false) }),
	}
}

// --- columns and indent strings ---

// indentColumn returns the visible column at the end of ind, an
// indentation (spaces and tabs) at the start of a line.
func indentColumn(ind string, tabSize int) int {
	c := 0
	for i := 0; i < len(ind); i++ {
		if ind[i] == '\t' {
			c += tabSize - c%tabSize
		} else {
			c++
		}
	}
	return c
}

// shiftIndent returns the indentation one level deeper than visible column
// col: the next multiple of the tab size, in spaces or tabs (Monaco's
// ShiftCommand.shiftIndent).
func shiftIndent(col, tabSize int, spaces bool) string {
	stop := col - col%tabSize + tabSize
	return indentString(stop, tabSize, spaces)
}

// unshiftIndent returns the indentation one level shallower than visible
// column col: the previous multiple of the tab size (ShiftCommand.unshiftIndent).
func unshiftIndent(col, tabSize int, spaces bool) string {
	stop := max(0, col-1-(col-1)%tabSize)
	return indentString(stop, tabSize, spaces)
}

// indentString returns an indentation of col columns, col a multiple of
// tabSize: spaces, or tabs.
func indentString(col, tabSize int, spaces bool) string {
	if spaces {
		return strings.Repeat(" ", col)
	}
	return strings.Repeat("\t", col/tabSize)
}

// normalizeIndentation rewrites the leading blanks of s in the indentation
// style: their visible width (a tab advances to the next tab stop) as
// spaces, or as tabs followed by the remaining spaces (Monaco's
// normalizeIndentation).
func normalizeIndentation(s string, tabSize int, spaces bool) string {
	i := firstNonBlank(s)
	if i < 0 {
		i = len(s)
	}
	w := indentColumn(s[:i], tabSize)
	ind := ""
	if !spaces {
		ind = strings.Repeat("\t", w/tabSize)
		w %= tabSize
	}
	return ind + strings.Repeat(" ", w) + s[i:]
}

// isBlankLine reports whether text is empty or only white space (/^\s*$/).
func isBlankLine(text string) bool { return strings.TrimFunc(text, isJSSpace) == "" }

// --- Tab, Shift+Tab, indent and outdent lines ---

// tab is the Tab key (Monaco's TabOperation): a caret, or a selection
// inside one line that is not the whole line, is replaced by spaces to
// the next tab stop (Model.InsertSpaces) or a tab; any other selection indents
// its lines.
//
// A caret on a blank line first goes to the indentation the language's
// rules give that line (TabOperation._goodIndentForLine: the indentation
// rules with autoIndent "full", else the onEnter action of the nearest
// non-blank line above; one tab stop when they give none), unless the line
// already starts with it.
func (s *State) tab() bool {
	snap := s.Snapshot()
	sels := s.clampedSels(snap)
	cmds := make([]opCmd, len(sels))
	done := map[int]bool{}
	for k, sel := range sels {
		a, b := ordered(sel)
		l := snap.LineOf(a)
		whole := a == snap.LineStart(l) && b == lineEnd(snap, l)
		if text := snap.Line(l); a == b && isBlankLine(text) && s.cheapTokens(snap, l) {
			good, ok := s.goodIndentForTab(snap, l)
			if !ok {
				good = "\t"
			}
			if t := s.norm(good); !strings.HasPrefix(text, t) {
				ls := snap.LineStart(l)
				cmds[k] = replaceCmd(k, buffer.Edit{Start: ls, End: ls + len(text), Text: t})
				continue
			}
		}
		if a == b || (snap.LineOf(b) == l && !whole) {
			cmds[k] = opCmd{
				edits: []buffer.Edit{{Start: a, End: b, Text: s.tabText(a)}},
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
		cmds[k] = s.shiftOp(snap, sel, false, done)
	}
	return s.runOps(cmds)
}

// shiftLines indents (or, unshift, outdents) the lines of every selection
// (Monaco's indent / outdent: Shift+Tab, Cmd+] and Cmd+[). A line touched
// by several selections shifts once, by the first selection's command, and
// every cursor stays; Monaco drops all the edits of the later of two
// selections sharing a line, also on its other lines.
func (s *State) shiftLines(unshift bool) bool {
	snap := s.Snapshot()
	sels := s.clampedSels(snap)
	cmds := make([]opCmd, len(sels))
	done := map[int]bool{}
	for k, sel := range sels {
		cmds[k] = s.shiftOp(snap, sel, unshift, done)
	}
	return s.runOps(cmds)
}

// shiftOp is Monaco's ShiftCommand (useTabStops) for one selection: each of
// its lines gets its indentation moved to the next (previous, unshift)
// multiple of the Model's tab size, in the style of Model.InsertSpaces.
// Indenting skips empty lines unless the selection is on one line;
// outdenting skips lines without indentation. Lines in done were shifted
// by another selection's command and are left to it.
//
// The selection follows the edit, except that a caret on a blank line
// goes to the end of the new indentation, and a selection starting inside
// the indentation of its first line keeps its start column.
func (s *State) shiftOp(snap buffer.Snapshot, sel Selection, unshift bool, done map[int]bool) opCmd {
	ts := s.indentSize()
	spaces := s.insertSpaces()
	first, last := selLines(snap, sel)
	empty := sel.Anchor == sel.Head
	indentEmpty := first == last
	a, _ := ordered(sel)
	startCol := a - snap.LineStart(first)
	toEnd := empty && isBlankLine(snap.Line(first)) // Monaco's _useLastEditRangeForCursorEndPosition
	endOfIndent := -1                               // toEnd: the length of the first line's new indentation
	staysPut := false
	var edits []buffer.Edit
	prevExtra, extra := 0, 0 // the previous and this line's "misalignment" (Monaco's extraSpaces)
	for l := first; l <= last; l, prevExtra = l+1, extra {
		extra = 0
		text := snap.Line(l)
		ie := firstNonBlank(text)
		if unshift && (text == "" || ie == 0) {
			continue
		}
		if !indentEmpty && !unshift && text == "" {
			continue
		}
		if ie < 0 {
			ie = len(text)
		}
		// A line indented off the tab stops by the spaces an onEnter rule
		// appends to the line above (JSDoc's " * ") keeps them after its
		// indentation.
		if l > 0 && indentColumn(text[:ie], ts)%ts != 0 && s.cheapTokens(snap, l-1) {
			end := lineEnd(snap, l-1)
			if r, ok := s.enterAction(snap, end, end, s.Options.autoIndent()); ok {
				extra = prevExtra
				for j := 0; j < len(r.appendText) && extra < ts && r.appendText[j] == ' '; j++ {
					extra++
				}
				if r.removeText > 0 {
					extra = max(0, extra-r.removeText)
				}
				for j := 0; j < extra && ie > 0 && text[ie-1] == ' '; j++ {
					ie--
				}
			}
		}
		if unshift && ie == 0 {
			continue
		}
		col := indentColumn(text[:ie], ts)
		want := shiftIndent(col, ts, spaces)
		if unshift {
			want = unshiftIndent(col, ts, spaces)
		}
		if l == first {
			endOfIndent = len(want)
			if !empty {
				staysPut = startCol <= ie
			}
		}
		if done[l] {
			continue
		}
		done[l] = true
		if want != text[:ie] {
			ls := snap.LineStart(l)
			edits = append(edits, buffer.Edit{Start: ls, End: ls + ie, Text: want})
		}
	}
	ls := snap.LineStart(first)
	return opCmd{edits: edits, cursor: func(r *opResult, sel Selection) Selection {
		if toEnd {
			if endOfIndent < 0 {
				return r.track(sel) // nothing to outdent on an empty line
			}
			c := r.tr.Map(ls, buffer.Left) + endOfIndent
			return Selection{Anchor: c, Head: c}
		}
		res := r.track(sel)
		if staysPut {
			keep := r.tr.Map(ls, buffer.Left) + startCol
			if res.Anchor <= res.Head && res.Anchor > keep {
				res.Anchor = keep
			} else if res.Head < res.Anchor && res.Head > keep {
				res.Head = keep
			}
		}
		return res
	}}
}

// --- converting the indentation ---

// convertIndentation rewrites the indentation of every line (a blank line
// is all indentation): each tab as tab-size spaces (toSpaces), or each run
// of tab-size spaces as a tab; then sets the Model's InsertSpaces to match
// (Monaco's indentationToSpaces / indentationToTabs), so every view of the
// document indents the new way.
func (s *State) convertIndentation(toSpaces bool) bool {
	snap := s.Snapshot()
	sels := s.clampedSels(snap)
	ts := s.indentSize()
	sp := strings.Repeat(" ", ts)
	var edits []buffer.Edit
	for i := range snap.LineCount() {
		text := snap.Line(i)
		ie := firstNonBlank(text)
		if ie < 0 {
			ie = len(text)
		}
		if ie == 0 {
			continue
		}
		ind := text[:ie]
		var want string
		if toSpaces {
			want = strings.ReplaceAll(ind, "\t", sp)
		} else {
			want = strings.ReplaceAll(ind, sp, "\t")
		}
		if want != ind {
			ls := snap.LineStart(i)
			edits = append(edits, buffer.Edit{Start: ls, End: ls + ie, Text: want})
		}
	}
	changed := false
	if len(edits) > 0 {
		cmds := make([]opCmd, len(sels))
		cmds[0].edits = edits
		changed = s.runOps(cmds)
	}
	s.Model.SetInsertSpaces(toSpaces)
	return changed
}

// --- re-indenting by the language's indentation rules ---

// indentRules are a language's compiled indentationRules.
type indentRules struct {
	increase, decrease, indentNext, unindented regex.Regexp
	brackets                                   *regexp.Regexp // the bracket strings, removed from strings, comments and regexes before matching; nil for none
}

var indentRulesCache sync.Map // *langs.Conf → *indentRules (nil: none)

// rulesFor returns conf's indentation rules, nil when it has none (or no
// increase/decrease pattern compiles).
func rulesFor(conf *langs.Conf) *indentRules {
	if conf == nil {
		return nil
	}
	if v, ok := indentRulesCache.Load(conf); ok {
		return v.(*indentRules)
	}
	ir := &indentRules{
		increase:   compileTest(conf.Indentation.Increase),
		decrease:   compileTest(conf.Indentation.Decrease),
		indentNext: compileTest(conf.Indentation.IndentNextLine),
		unindented: compileTest(conf.Indentation.Unindented),
	}
	if ir.increase == nil && ir.decrease == nil {
		ir = nil
	} else {
		ir.brackets = bracketRegexp(conf.Brackets)
	}
	v, _ := indentRulesCache.LoadOrStore(conf, ir)
	return v.(*indentRules)
}

// compileTest compiles p (a JavaScript regex) for RegExp.test: a match
// anywhere in the line, so the compiled pattern is anchored after a lazy
// prefix. Patterns that do not compile are nil (never match).
func compileTest(p *langs.RegexSpec) regex.Regexp {
	if p == nil || p.Source == "" {
		return nil
	}
	re, err := regex.Compile(`[\s\S]*?(?:`+p.Source+`)`, p.Flags)
	if err != nil {
		return nil
	}
	return re
}

func test(re regex.Regexp, s string) bool { return re != nil && re.MatchStart(s) != nil }

// standardTokenType is Monaco's StandardTokenType of a Monarch token type
// (toStandardTokenType): the match is "comment", "string", "regex" or
// "regexp", none for other tokens.
var standardTokenType = regexp.MustCompile(`\b(comment|string|regex|regexp)\b`)

// stdType returns the standard token type of a Monarch token type: one of
// "comment", "string", "regex" ("regexp" counts as "regex"), or "".
func stdType(typ string) string {
	m := standardTokenType.FindString(typ)
	if m == "regexp" {
		return "regex"
	}
	return m
}

// lineTokens tokenizes the document's lines in order with the language's
// Monarch lexer, synchronously and from the first line, so the tokens are
// accurate (the Highlighter's may still be catching up). Without a lexer
// (plain text, a language without a grammar) lines have no tokens.
type lineTokens struct {
	snap  buffer.Snapshot
	lx    *monarch.Lexer
	state monarch.State
	next  int // the next line to tokenize
	toks  []monarch.Token
}

// newLineTokens returns a lineTokens for the State's document.
func (s *State) newLineTokens(snap buffer.Snapshot) *lineTokens {
	lt := &lineTokens{snap: snap}
	if m := s.Model; m.reg != nil && !m.plain {
		if lx, err := m.reg.Lexer(m.lang); err == nil {
			lt.lx, lt.state = lx, lx.Initial()
		}
	}
	return lt
}

// tokens returns line i's tokens, nil without a lexer. Lines must be asked
// for in increasing order (asking for the same line again is free); the
// result is valid until the next call.
func (lt *lineTokens) tokens(i int) []monarch.Token {
	if lt.lx == nil {
		return nil
	}
	for lt.next <= i {
		lt.toks, lt.state = lt.lx.TokenizeLine(lt.snap.Line(lt.next), lt.state, lt.toks[:0])
		lt.next++
	}
	return lt.toks
}

// tokenAt returns the standard type of the token of toks at byte offset off
// (the last token starting at or before it, as Monaco's
// findTokenIndexAtOffset), "" for none.
func tokenAt(toks []monarch.Token, off int) string {
	typ := ""
	for _, tk := range toks {
		if tk.ByteStart > off {
			break
		}
		typ = tk.Type
	}
	return stdType(typ)
}

// indentLines gives the re-indenter the text of the document's lines as
// Monaco's ProcessedIndentRulesSupport sees them: with the bracket strings
// removed from string, comment and regex tokens. Without a lexer the lines
// are used as is.
type indentLines struct {
	lineTokens
	rules *indentRules
}

// line returns line i's processed text and whether it starts in a string
// token (lines in increasing order).
func (il *indentLines) line(i int) (string, bool) {
	text := il.snap.Line(i)
	toks := il.tokens(i)
	if toks == nil {
		return text, false
	}
	return il.process(text, toks), toks[0].ByteStart == 0 && stdType(toks[0].Type) == "string"
}

// process removes the bracket strings from the string, comment and regex
// tokens of text.
func (il *indentLines) process(text string, toks []monarch.Token) string {
	return processBrackets(text, toks, il.rules.brackets, 0, len(text))
}

// bracketRegexp is Monaco's bracket regular expression
// (LanguageBracketsConfiguration.getBracketRegExp, createBracketOrRegExp):
// the opening then the closing bracket strings, ASCII case-insensitive
// (JavaScript's "i" flag without "u"; Go's (?i) would also fold K and ſ),
// a bracket made of word characters and spaces ("begin", "end") matched as
// a whole word only. Nil when there are no brackets.
func bracketRegexp(pairs [][2]string) *regexp.Regexp {
	var pieces []string
	seen := map[string]bool{}
	for _, side := range []int{0, 1} {
		for _, p := range pairs {
			b := p[side]
			if p[0] == "" || p[1] == "" || seen[b] {
				continue
			}
			seen[b] = true
			pieces = append(pieces, bracketPattern(b))
		}
	}
	if len(pieces) == 0 {
		return nil
	}
	return regexp.MustCompile(`(?:` + strings.Join(pieces, ")|(?:") + `)`)
}

// bracketPattern is Monaco's prepareBracketForRegExp, each ASCII letter
// matching both cases.
func bracketPattern(b string) string {
	var sb strings.Builder
	for _, r := range b {
		switch {
		case 'a' <= r && r <= 'z', 'A' <= r && r <= 'Z':
			lo, up := r|0x20, r&^0x20
			sb.WriteString("[" + string(lo) + string(up) + "]")
		default:
			sb.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	if wordBracket(b) {
		return `\b` + sb.String() + `\b`
	}
	return sb.String()
}

// wordBracket reports whether b is made of word characters and spaces
// only (/^[\w ]+$/): such a bracket matches as a whole word.
func wordBracket(b string) bool {
	if b == "" {
		return false
	}
	for i := 0; i < len(b); i++ {
		if !isWordByte(b[i]) && b[i] != ' ' {
			return false
		}
	}
	return true
}

// processBrackets returns text[from:to] with the bracket strings removed
// from the parts in string, comment and regex tokens (Monaco's
// IndentationLineProcessor.getProcessedTokens over a slice of the line's
// tokens). Without tokens the slice is returned as is.
func processBrackets(text string, toks []monarch.Token, brackets *regexp.Regexp, from, to int) string {
	if brackets == nil || len(toks) == 0 {
		return text[from:to]
	}
	var b strings.Builder
	if first := toks[0].ByteStart; from < first {
		b.WriteString(text[from:min(first, to)])
	}
	for j, tk := range toks {
		end := len(text)
		if j+1 < len(toks) {
			end = toks[j+1].ByteStart
		}
		lo, hi := max(tk.ByteStart, from), min(end, to)
		if lo >= hi {
			continue
		}
		part := text[lo:hi]
		if stdType(tk.Type) != "" {
			part = brackets.ReplaceAllString(part, "")
		}
		b.WriteString(part)
	}
	return b.String()
}

// reindentLines re-indents every line by the language's indentation rules
// (Monaco's ReindentLinesAction, getReindentEditOperations): from the first
// line on, a line matching increaseIndentPattern indents the lines after
// it, one matching decreaseIndentPattern outdents itself and the rest,
// one matching indentNextLinePattern indents only the next line, and lines
// matching unIndentedLinePattern keep the indentation they get but do not
// change it for the others. Lines starting inside a string are left alone.
// The indentation is written in the Model's InsertSpaces style. A language without
// indentation rules does nothing.
func (s *State) reindentLines() bool {
	rules := rulesFor(s.Model.Conf())
	snap := s.Snapshot()
	if rules == nil || snap.Len() == 0 {
		return false
	}
	sels := s.clampedSels(snap)
	ts, spaces := s.indentSize(), s.insertSpaces()
	il := &indentLines{lineTokens: *s.newLineTokens(snap), rules: rules}
	shift := func(ind string) string { return shiftIndent(indentColumn(ind, ts), ts, spaces) }
	unshift := func(ind string) string { return unshiftIndent(indentColumn(ind, ts), ts, spaces) }
	withIndent := func(text, ind string) string { return ind + text[len(leadingBlanks(text)):] }

	n := snap.LineCount()
	start := 0
	for start < n {
		if t, _ := il.line(start); !test(rules.unindented, t) {
			break
		}
		start++
	}
	if start > n-2 {
		return false
	}
	first, _ := il.line(start)
	global := leadingBlanks(snap.Line(start))
	ideal := global
	if test(rules.increase, first) {
		ideal, global = shift(ideal), shift(global)
	} else if test(rules.indentNext, first) {
		ideal = shift(ideal)
	}
	var edits []buffer.Edit
	edited := map[int]bool{}
	for l := start + 1; l < n; l++ {
		proc, inString := il.line(l)
		if inString {
			continue
		}
		old := leadingBlanks(snap.Line(l))
		cur := ideal
		if test(rules.decrease, withIndent(proc, cur)) {
			ideal, global = unshift(ideal), unshift(global)
		}
		if old != ideal {
			ls := snap.LineStart(l)
			edits = append(edits, buffer.Edit{Start: ls, End: ls + len(old), Text: normalizeIndentation(ideal, ts, spaces)})
			edited[l] = true
		}
		pc := withIndent(proc, cur)
		switch {
		case test(rules.unindented, proc):
		case test(rules.increase, pc):
			global = shift(global)
			ideal = global
		case test(rules.indentNext, pc):
			ideal = shift(ideal)
		default:
			ideal = global
		}
	}
	if len(edits) == 0 {
		return false
	}
	// Monaco applies the edits with forceMoveMarkers: a cursor in or at the
	// end of a re-indented indentation goes to the end of the new one.
	cmds := make([]opCmd, len(sels))
	cmds[0].edits = edits
	moved := func(r *opResult, off int) int {
		l := snap.LineOf(off)
		ls := snap.LineStart(l)
		if ie := len(leadingBlanks(snap.Line(l))); edited[l] && off <= ls+ie {
			off = ls + ie
		}
		return r.tr.Map(off, buffer.Right)
	}
	for k := range cmds {
		cmds[k].cursor = func(r *opResult, sel Selection) Selection {
			return Selection{Anchor: moved(r, sel.Anchor), Head: moved(r, sel.Head)}
		}
	}
	return s.runOps(cmds)
}
