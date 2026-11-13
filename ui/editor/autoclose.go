package editor

import (
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/langs"
	"github.com/tangoui/tango/ui/editor/monarch"
)

// Typing assistance (Monaco 0.56's TypeOperations.typeWithInterceptors,
// cursorTypeEditOperations): text typed from the keyboard (State.Type, the
// type command, committed text) goes through it one character at a time,
// and the first of these that applies handles the character for every
// cursor:
//
//   - Enter ("\n"): the language's onEnter rules, indentation rules and
//     the line's indentation (onenter.go);
//   - auto-indent on type (autoIndent "full"): a character that makes the
//     line match the language's decreaseIndentPattern re-indents it;
//   - overtype: a closing bracket or quote typed just before the same
//     character that the editor auto-inserted moves over it;
//   - auto-closing: an opening bracket or quote, or the last character of
//     a longer opening sequence ("/**"), also inserts its closing text,
//     when the character after the caret is one the language allows
//     (autoCloseBefore) and the caret is not in a token the pair's notIn
//     lists (string, comment);
//   - surround: a bracket or quote typed over selections wraps them;
//   - electric brackets: a closing bracket typed as the first character of
//     a line moves the line to the indentation of the line of its opening
//     bracket (one cursor only);
//   - otherwise the character is inserted as typed.
//
// Backspace between an auto-closed pair deletes both characters
// (AutoClosingDelete).
//
// The token checks use the Model's Highlighter: the caret's line is
// tokenized from the stored state of the line above. As Monaco does when a
// line is not "cheap to tokenize", the checks that need tokens are skipped
// while the lines above the caret are not yet final after a short
// synchronous catch-up (assistBudget; the background goroutine and the
// View's paint catch up the rest).
//
// Differences from Monaco: an IME commit is inserted as is (Monaco runs
// auto-closing and overtype on a one-character composition); an embedded
// language's configuration (a script block in HTML) is not used, the
// Model's language applies to the whole line.

// assistBudget is how long one typing command may tokenize the lines
// above its cursors synchronously, in all, before treating the lines not
// yet final as not cheap to tokenize; for a cursor near the top of the
// document (assistNear lines) the command may take up to assistNearBudget,
// as catching those lines up is bounded and done once.
const (
	assistBudget     = 2 * time.Millisecond
	assistNear       = 1000
	assistNearBudget = 20 * time.Millisecond
)

// Monaco's default autoCloseBefore sets (CharacterPairSupport), used when
// the language sets none.
const (
	autoCloseBeforeQuotes   = ";:.,=}])> \n\t"
	autoCloseBeforeBrackets = "'\"`;:.,=}])> \n\t"
)

// acPair is a compiled auto-closing pair (Monaco's
// StandardAutoClosingPairConditional).
type acPair struct {
	open, close                  string
	inString, inComment, inRegex bool
	neutral                      string // findNeutralCharacter, "" for none
}

// isOK reports whether the pair may be auto-closed in a token of standard
// type std ("" is Other).
func (p *acPair) isOK(std string) bool {
	switch std {
	case "comment":
		return p.inComment
	case "string":
		return p.inString
	case "regex":
		return p.inRegex
	}
	return true
}

// typingConf is a language configuration compiled for typing assistance.
type typingConf struct {
	pairs                        []*acPair
	openByEnd, openByStart       map[string][]*acPair // keyed by the open's last / first character
	closeByStart, closeByEnd     map[string][]*acPair // keyed by the close's first / last character
	closeSingle                  map[string]bool      // closes of one-character pairs
	surround                     map[string]string    // surrounding pairs, open → close
	beforeQuotes, beforeBrackets string               // autoCloseBefore sets
	blockStart                   string               // block comment start token
	onEnter                      *onEnterSupport      // nil: the language has no onEnter support
	brackets                     [][2]string          // the language's brackets (lower case)
	electric                     map[string]bool      // last characters of the closing brackets
	closeBracket, openBracket    map[string]int       // bracket text → index in brackets
	procBrackets                 *regexp.Regexp       // the bracket regexp: brackets removed from strings, comments and regexes for the indentation rules
}

var typingConfs sync.Map // *langs.Conf → *typingConf

// emptyTyping is the configuration of plain text: no pairs, no rules.
var emptyTyping = newTypingConf(&langs.Conf{})

// typing returns the Model's compiled typing configuration.
func (s *State) typing() *typingConf {
	conf := s.Model.Conf()
	if conf == nil {
		return emptyTyping
	}
	if v, ok := typingConfs.Load(conf); ok {
		return v.(*typingConf)
	}
	v, _ := typingConfs.LoadOrStore(conf, newTypingConf(conf))
	return v.(*typingConf)
}

func newTypingConf(conf *langs.Conf) *typingConf {
	tc := &typingConf{
		openByEnd:      map[string][]*acPair{},
		openByStart:    map[string][]*acPair{},
		closeByStart:   map[string][]*acPair{},
		closeByEnd:     map[string][]*acPair{},
		closeSingle:    map[string]bool{},
		surround:       map[string]string{},
		beforeQuotes:   autoCloseBeforeQuotes,
		beforeBrackets: autoCloseBeforeBrackets,
		blockStart:     conf.BlockComment[0],
		electric:       map[string]bool{},
		closeBracket:   map[string]int{},
		openBracket:    map[string]int{},
	}
	// Monaco: the auto-closing pairs, else the brackets.
	src := conf.AutoClosingPairs
	if src == nil {
		for _, b := range conf.Brackets {
			src = append(src, langs.Pair{Open: b[0], Close: b[1]})
		}
	}
	for _, p := range src {
		ap := &acPair{open: p.Open, close: p.Close, inString: true, inComment: true, inRegex: true}
		for _, n := range p.NotIn {
			switch n {
			case "string":
				ap.inString = false
			case "comment":
				ap.inComment = false
			case "regex":
				ap.inRegex = false
			}
		}
		ap.neutral = neutralChar(p.Open, p.Close)
		tc.pairs = append(tc.pairs, ap)
		if p.Open == "" || p.Close == "" {
			continue
		}
		add := func(m map[string][]*acPair, k string) { m[k] = append(m[k], ap) }
		add(tc.openByStart, firstChar(p.Open))
		add(tc.openByEnd, lastChar(p.Open))
		add(tc.closeByStart, firstChar(p.Close))
		add(tc.closeByEnd, lastChar(p.Close))
		if utf8.RuneCountInString(p.Open) == 1 && utf8.RuneCountInString(p.Close) == 1 {
			tc.closeSingle[p.Close] = true
		}
	}
	// Surrounding pairs, else the auto-closing pairs.
	if conf.SurroundingPairs != nil {
		for _, p := range conf.SurroundingPairs {
			tc.surround[p.Open] = p.Close
		}
	} else {
		for _, p := range tc.pairs {
			tc.surround[p.open] = p.close
		}
	}
	if conf.AutoCloseBefore != "" {
		tc.beforeQuotes, tc.beforeBrackets = conf.AutoCloseBefore, conf.AutoCloseBefore
	}
	for i, b := range conf.Brackets {
		o, c := asciiLower(b[0]), asciiLower(b[1])
		tc.brackets = append(tc.brackets, [2]string{o, c})
		if _, ok := tc.openBracket[o]; !ok {
			tc.openBracket[o] = i
		}
		if _, ok := tc.closeBracket[c]; !ok {
			tc.closeBracket[c] = i
		}
		if c != "" {
			tc.electric[lastChar(c)] = true
		}
	}
	tc.procBrackets = bracketRegexp(conf.Brackets)
	tc.onEnter = newOnEnterSupport(conf)
	return tc
}

// neutralChar is Monaco's findNeutralCharacter: the first of 0-9, a-z, A-Z
// in neither open nor close.
func neutralChar(open, close string) string {
	for _, rg := range [][2]byte{{'0', '9'}, {'a', 'z'}, {'A', 'Z'}} {
		for c := rg[0]; c <= rg[1]; c++ {
			if !strings.ContainsRune(open, rune(c)) && !strings.ContainsRune(close, rune(c)) {
				return string(c)
			}
		}
	}
	return ""
}

// firstChar and lastChar return the first and last character of s ("" for
// an empty s).
func firstChar(s string) string {
	_, n := utf8.DecodeRuneInString(s)
	return s[:n]
}

func lastChar(s string) string {
	_, n := utf8.DecodeLastRuneInString(s)
	return s[len(s)-n:]
}

// asciiLower lowers the ASCII letters of s byte by byte, keeping every
// byte offset (strings.ToLower can shorten non-ASCII text: İ, K, Ω).
// Monaco's bracket regexps use the "i" flag without "u", which folds
// ASCII letters only for the brackets the languages define.
func asciiLower(s string) string {
	for i := 0; i < len(s); i++ {
		if 'A' <= s[i] && s[i] <= 'Z' {
			b := []byte(s)
			for j := i; j < len(b); j++ {
				if 'A' <= b[j] && b[j] <= 'Z' {
					b[j] += 'a' - 'A'
				}
			}
			return string(b)
		}
	}
	return s
}

// isQuote is Monaco's isQuote.
func isQuote(ch string) bool { return ch == "'" || ch == "\"" || ch == "`" }

// regularRune reports whether r is a word character by Monaco's default
// wordSeparators (WordCharacterClass.Regular).
func regularRune(r rune) bool {
	return r != ' ' && r != '\t' && (r >= utf8.RuneSelf || strings.IndexByte(wordSeparators, byte(r)) < 0)
}

// --- tokens ---

// tokGate is the "cheap to tokenize" state of one command (reset by
// State.Execute and per typed character) and snapshot: the lines known
// final and the synchronous catch-up time left.
type tokGate struct {
	ver      uint64
	okUpTo   int // the lines before it are final
	start    time.Time
	deadline time.Time
	spent    bool // a catch-up came back without finishing
}

// cheapTokens reports whether line's tokens can be trusted (Monaco's
// isCheapToTokenize): the lines above it are final in the Highlighter.
// Lines not yet final are tokenized synchronously, for at most
// assistBudget per command in all (assistNearBudget for lines near the
// top), so a command over many cursors or lines does not force
// tokenization at each; once the time is spent only final lines count.
// Plain text always is cheap.
func (s *State) cheapTokens(snap buffer.Snapshot, line int) bool {
	hl := s.Model.Highlighter()
	if hl == nil {
		return true
	}
	g := s.gate
	if g == nil || g.ver != snap.Version() {
		now := time.Now()
		g = &tokGate{ver: snap.Version(), start: now, deadline: now.Add(assistBudget)}
		s.gate = g
	}
	if line <= g.okUpTo || hl.FinalBefore(snap, line) {
		g.okUpTo = max(g.okUpTo, line)
		return true
	}
	if g.spent {
		return false
	}
	if line < assistNear {
		g.deadline = later(g.deadline, g.start.Add(assistNearBudget))
	}
	left := time.Until(g.deadline)
	if left <= 0 {
		g.spent = true
		return false
	}
	if hl.HighlightFor(s.hlKey, snap, line, left) {
		g.okUpTo = max(g.okUpTo, line)
		return true
	}
	g.spent = true
	return false
}

func later(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}

// lineToks returns the tokens of text taken as line i of snap (the line
// itself, or the line as it would be after an edit), from the
// Highlighter's stored state of the line above; nil for plain text.
func (s *State) lineToks(snap buffer.Snapshot, i int, text string) []monarch.Token {
	hl := s.Model.Highlighter()
	if hl == nil {
		return nil
	}
	return hl.TokenizeSpan(snap, i, text, nil)
}

// lastSelLine returns the last line a selection ends on: the lines up to
// it are cheap to tokenize when it is (one catch-up for every cursor).
func (s *State) lastSelLine(snap buffer.Snapshot) int {
	m := 0
	for _, x := range s.Sels {
		_, b := ordered(x)
		m = max(m, snap.LineOf(clampOff(snap, b)))
	}
	return m
}

// --- the auto-closed characters (Monaco's AutoClosedAction) ---

// autoClosedAction records the closing characters one auto-closing command
// inserted: chars[k] is a closing text, encl[k] the range from its opening
// character to its end. Both follow the edits without growing when text is
// inserted at their edges; the action ends when a selection leaves its
// enclosing range or a range spans lines.
type autoClosedAction struct {
	chars, encl [][2]int
}

// autoClosedEdited maps the auto-closed ranges through edits (sorted, in
// the text before them) and drops the actions that are no longer valid.
func (s *State) autoClosedEdited(edits []buffer.Edit) {
	if len(s.autoClosed) == 0 {
		return
	}
	tr := buffer.NewTracker(edits)
	mapR := func(rs [][2]int) {
		for i, r := range rs {
			a, b := tr.Map(r[0], buffer.Right), tr.Map(r[1], buffer.Left)
			rs[i] = [2]int{a, max(a, b)}
		}
	}
	for _, a := range s.autoClosed {
		mapR(a.chars)
		mapR(a.encl)
	}
	s.validateAutoClosed()
}

// validateAutoClosed drops the auto-closed actions the selections left
// (Monaco's _validateAutoClosedActions): an action stays while each
// selection, in document order, is strictly inside the enclosing range of
// the same rank and every enclosing range is on one line.
func (s *State) validateAutoClosed() {
	if len(s.autoClosed) == 0 {
		return
	}
	snap := s.Snapshot()
	sels := make([][2]int, len(s.Sels))
	for i, x := range s.Sels {
		a, b := ordered(x)
		sels[i] = [2]int{a, b}
	}
	byStart := func(x, y [2]int) int {
		if x[0] != y[0] {
			return x[0] - y[0]
		}
		return x[1] - y[1]
	}
	slices.SortFunc(sels, byStart)
	s.autoClosed = slices.DeleteFunc(s.autoClosed, func(a *autoClosedAction) bool {
		encl := slices.Clone(a.encl)
		for _, r := range encl {
			if r[1] > snap.Len() || snap.LineOf(r[0]) != snap.LineOf(r[1]) {
				return true
			}
		}
		slices.SortFunc(encl, byStart)
		for i, x := range sels {
			if i >= len(encl) || !(encl[i][0] < x[0] && x[1] < encl[i][1]) {
				return true
			}
		}
		return false
	})
}

// isAutoClosedAt reports whether an auto-closed character starts at off.
func (s *State) isAutoClosedAt(off int) bool {
	for _, a := range s.autoClosed {
		for _, r := range a.chars {
			if r[0] == off {
				return true
			}
		}
	}
	return false
}

// --- typing ---

// Type types text as from the keyboard: each character (grapheme cluster)
// goes through the typing assistance in turn, with the Enter rules for a
// line break ("\r\n" and "\r" count as one). Use Insert to insert text as
// is (paste, programmatic edits).
func (s *State) Type(text string) {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	for i := 0; i < len(text); {
		j := nextGrapheme(text, i)
		s.typeChar(text[i:j])
		i = j
	}
}

// typeChar types one character (Monaco's typeWithInterceptors).
func (s *State) typeChar(ch string) {
	s.gate = nil // each character gets its own tokenization budget
	s.normalize()
	s.clampedSels(s.Snapshot())
	switch {
	case ch == "\n":
		s.enter()
	case s.autoIndentType(ch):
	case s.overtype(ch):
	case s.autoCloseOpen(ch):
	case s.surroundType(ch):
	case s.electricChar(ch):
	default:
		s.Insert(ch)
	}
}

// overtype types a closing character over the same character after every
// caret (Monaco's AutoClosingOvertypeOperation).
func (s *State) overtype(ch string) bool {
	mode := closingEdit(s.Options.AutoClosingOvertype)
	tc := s.typing()
	if mode == AutoClosingEditNever || !tc.closeSingle[ch] {
		return false
	}
	snap := s.Snapshot()
	for _, sel := range s.Sels {
		if sel.Anchor != sel.Head {
			return false
		}
		l := snap.LineOf(sel.Head)
		text := snap.Line(l)
		col := sel.Head - snap.LineStart(l)
		if !strings.HasPrefix(text[col:], ch) {
			return false
		}
		// Not over a quote after a backslash.
		if isQuote(ch) && col > 0 && text[col-1] == '\\' {
			return false
		}
		if mode == AutoClosingEditAuto && !s.isAutoClosedAt(sel.Head) {
			return false
		}
	}
	cmds := make([]opCmd, len(s.Sels))
	for k, sel := range s.Sels {
		cmds[k] = replaceCmd(k, buffer.Edit{Start: sel.Head, End: sel.Head + len(ch), Text: ch})
	}
	return s.runOpsKind(cmds, kindType)
}

// replaceCmd is Monaco's ReplaceCommand for command k: the edit, and the
// caret after the inserted text.
func replaceCmd(k int, e buffer.Edit) opCmd {
	return opCmd{edits: []buffer.Edit{e}, cursor: func(r *opResult, sel Selection) Selection {
		if r.inv[k] == nil {
			return r.track(sel)
		}
		c := r.inv[k][0][1]
		return Selection{Anchor: c, Head: c}
	}}
}

// autoCloseOpen types ch and the closing text of the auto-closing pair it
// completes at every caret (Monaco's AutoClosingOpenCharTypeOperation).
func (s *State) autoCloseOpen(ch string) bool {
	closeText, ok := s.autoClosingPairClose(ch)
	if !ok {
		return false
	}
	return s.typeWithAutoClosing(ch, closeText, nil)
}

// typeWithAutoClosing inserts ch+closeText at every caret, the caret
// between them, as an undo step continued by further typing, and records
// the closing texts as auto-closed. indent, when set, gives each command
// an edit that re-indents its line first (Monaco's
// TypeWithIndentationAndAutoClosingCommand).
func (s *State) typeWithAutoClosing(ch, closeText string, indent []buffer.Edit) bool {
	cmds := make([]opCmd, len(s.Sels))
	ends := make([]int, len(s.Sels)) // the end of each inserted text, -1 when dropped
	for k := range ends {
		ends[k] = -1
	}
	for k, sel := range s.Sels {
		var es []buffer.Edit
		if indent != nil {
			es = append(es, indent[k])
		}
		es = append(es, buffer.Edit{Start: sel.Head, End: sel.Head, Text: ch + closeText})
		last := len(es) - 1
		cmds[k] = opCmd{edits: es, cursor: func(r *opResult, sel Selection) Selection {
			if r.inv[k] == nil {
				return r.track(sel)
			}
			end := r.inv[k][last][1]
			ends[k] = end
			c := end - len(closeText)
			return Selection{Anchor: c, Head: c}
		}}
	}
	s.Model.buf.PushUndoStop()
	if !s.runOpsKind(cmds, kindType) {
		return false
	}
	act := &autoClosedAction{}
	for _, end := range ends {
		if end < 0 {
			continue
		}
		act.chars = append(act.chars, [2]int{end - len(closeText), end})
		act.encl = append(act.encl, [2]int{end - len(closeText) - len(ch), end})
	}
	if len(act.chars) > 0 {
		s.autoClosed = append(s.autoClosed, act)
	}
	return true
}

// autoClosingPairClose returns the closing text typing ch at every caret
// auto-closes (Monaco's getAutoClosingPairClose), false when it does not.
func (s *State) autoClosingPairClose(ch string) (string, bool) {
	tc := s.typing()
	if len(tc.openByEnd[ch]) == 0 {
		return "", false
	}
	for _, sel := range s.Sels {
		if sel.Anchor != sel.Head {
			return "", false
		}
	}
	snap := s.Snapshot()
	pair := s.findAutoClosingPairOpen(snap, ch)
	if pair == nil {
		return "", false
	}
	var cfg AutoClosingStrategy
	var before string
	if isQuote(ch) {
		cfg, before = closing(s.Options.AutoClosingQuotes), tc.beforeQuotes
	} else if tc.blockStart != "" && strings.Contains(pair.open, tc.blockStart) {
		cfg, before = closing(s.Options.AutoClosingComments), tc.beforeBrackets
	} else {
		cfg, before = closing(s.Options.AutoClosingBrackets), tc.beforeBrackets
	}
	if cfg == AutoCloseNever {
		return "", false
	}
	shouldCloseBefore := func(c string) bool {
		switch cfg {
		case AutoCloseAlways:
			return true
		case AutoCloseBeforeWhitespace:
			return c == " " || c == "\t"
		}
		return strings.Contains(before, c)
	}
	contained := tc.containedPair(pair)
	containedClose := ""
	if contained != nil {
		containedClose = contained.close
	}
	containedPresent := true
	if !s.cheapTokens(snap, s.lastSelLine(snap)) {
		return "", false
	}
	for _, sel := range s.Sels {
		l := snap.LineOf(sel.Head)
		text := snap.Line(l)
		col := sel.Head - snap.LineStart(l)
		lineBefore, lineAfter := text[:col], text[col:]
		if !strings.HasPrefix(lineAfter, containedClose) {
			containedPresent = false
		}
		// Only before an allowed character or a closing bracket.
		if lineAfter != "" && !tc.beforeClosingBrace(lineAfter) && !shouldCloseBefore(firstChar(lineAfter)) {
			return "", false
		}
		// Not a ' or " after a word character.
		if len(pair.open) == 1 && (ch == "'" || ch == "\"") && cfg != AutoCloseAlways && lineBefore != "" {
			if r, _ := utf8.DecodeLastRuneInString(lineBefore); regularRune(r) {
				return "", false
			}
		}
		// Not in a token the pair's notIn lists (the token of the
		// character before the caret).
		if toks := s.lineToks(snap, l, text); len(toks) > 0 && !pair.isOK(tokenAt(toks, max(col-1, 0))) {
			return "", false
		}
		// Would typing a neutral character here be in a token the pair
		// may not close in (the quote would end a string)?
		if pair.neutral != "" {
			t2 := lineBefore + pair.neutral + lineAfter
			if !pair.isOK(tokenAt(s.lineToks(snap, l, t2), col)) {
				return "", false
			}
		}
	}
	if containedPresent {
		return pair.close[:len(pair.close)-len(containedClose)], true
	}
	return pair.close, true
}

// findAutoClosingPairOpen returns the longest auto-closing pair whose
// opening text typing ch completes at every caret.
func (s *State) findAutoClosingPairOpen(snap buffer.Snapshot, ch string) *acPair {
	var res *acPair
	for _, c := range s.typing().openByEnd[ch] {
		if res != nil && len(c.open) <= len(res.open) {
			continue
		}
		match := true
		for _, sel := range s.Sels {
			ls := snap.LineStart(snap.LineOf(sel.Head))
			from := max(sel.Head-(len(c.open)-len(ch)), ls)
			if snap.Slice(from, sel.Head)+ch != c.open {
				match = false
				break
			}
		}
		if match {
			res = c
		}
	}
	return res
}

// containedPair returns the longest other pair whose open is inside p's
// and whose close ends p's (Monaco's _findContainedAutoClosingPair: with
// ( ) and (* *), typing * after ( inserts only *).
func (tc *typingConf) containedPair(p *acPair) *acPair {
	if len(p.open) <= 1 {
		return nil
	}
	var res *acPair
	for _, c := range tc.closeByEnd[lastChar(p.close)] {
		if c.open != p.open && strings.Contains(p.open, c.open) && strings.HasSuffix(p.close, c.close) {
			if res == nil || len(c.open) > len(res.open) {
				res = c
			}
		}
	}
	return res
}

// beforeClosingBrace reports whether after starts with a closing text of
// an auto-closing pair and not with an opening one (Monaco's
// _isBeforeClosingBrace).
func (tc *typingConf) beforeClosingBrace(after string) bool {
	c := firstChar(after)
	for _, p := range tc.openByStart[c] {
		if strings.HasPrefix(after, p.open) {
			return false
		}
	}
	for _, p := range tc.closeByStart[c] {
		if strings.HasPrefix(after, p.close) {
			return true
		}
	}
	return false
}

// surroundType wraps every selection in the surrounding pair ch opens
// (Monaco's SurroundSelectionOperation): an undo step of its own; each
// selection then covers the text inside the pair.
func (s *State) surroundType(ch string) bool {
	tc := s.typing()
	closeText, ok := tc.surround[ch]
	if !ok || !shouldSurround(s.Options.surround(), ch) {
		return false
	}
	snap := s.Snapshot()
	for _, sel := range s.Sels {
		a, b := ordered(sel)
		if a == b {
			return false
		}
		if strings.Trim(snap.Slice(a, b), " \t\n") == "" {
			return false // only blanks (and line breaks) selected
		}
		if isQuote(ch) && snap.LineOf(a) == snap.LineOf(b) && isQuote(snap.Slice(a, b)) {
			return false // a quote typed over a quote
		}
	}
	cmds := make([]opCmd, len(s.Sels))
	for k, sel := range s.Sels {
		a, b := ordered(sel)
		cmds[k] = opCmd{
			edits: []buffer.Edit{{Start: a, End: a, Text: ch}, {Start: b, End: b, Text: closeText}},
			cursor: func(r *opResult, sel Selection) Selection {
				if r.inv[k] == nil {
					return r.track(sel)
				}
				return Selection{Anchor: r.inv[k][0][1], Head: r.inv[k][1][1] - len(closeText)}
			},
		}
	}
	return s.runOps(cmds)
}

// shouldSurround is Monaco's shouldSurroundChar.
func shouldSurround(mode AutoSurroundStrategy, ch string) bool {
	if isQuote(ch) {
		return mode == AutoSurroundQuotes || mode == AutoSurroundLanguageDefined
	}
	return mode == AutoSurroundBrackets || mode == AutoSurroundLanguageDefined
}

// --- deleting ---

// autoClosingPairDelete reports whether Backspace at every caret deletes
// an auto-closing pair around it (Monaco's isAutoClosingPairDelete): the
// caret sits between a one-character pair's open and close, and with
// AutoClosingDelete "auto" the close was auto-inserted.
func (s *State) autoClosingPairDelete() bool {
	brackets, quotes := closing(s.Options.AutoClosingBrackets), closing(s.Options.AutoClosingQuotes)
	mode := closingEdit(s.Options.AutoClosingDelete)
	if (brackets == AutoCloseNever && quotes == AutoCloseNever) || mode == AutoClosingEditNever {
		return false
	}
	tc := s.typing()
	snap := s.Snapshot()
	for _, sel := range s.sels() {
		if sel.Anchor != sel.Head {
			return false
		}
		off := clampOff(snap, sel.Head)
		l := snap.LineOf(off)
		text := snap.Line(l)
		col := off - snap.LineStart(l)
		if col == 0 || col >= len(text) {
			return false
		}
		r, n := utf8.DecodeLastRuneInString(text[:col])
		open := string(r)
		after := firstChar(text[col:])
		cands := tc.openByEnd[open]
		if len(cands) == 0 || n == 0 {
			return false
		}
		if isQuote(open) {
			if quotes == AutoCloseNever {
				return false
			}
		} else if brackets == AutoCloseNever {
			return false
		}
		found := false
		for _, c := range cands {
			if c.open == open && c.close == after {
				found = true
			}
		}
		if !found {
			return false
		}
		if mode == AutoClosingEditAuto && !s.isAutoClosedAt(off) {
			return false
		}
	}
	return true
}

// pairRange is the range Backspace deletes at caret off when it deletes
// the auto-closing pair around it.
func pairRange(snap buffer.Snapshot, off int) (int, int) {
	l := snap.LineOf(off)
	text := snap.Line(l)
	col := off - snap.LineStart(l)
	_, n := utf8.DecodeLastRuneInString(text[:col])
	_, m := utf8.DecodeRuneInString(text[col:])
	return off - n, off + m
}

// deletePairs deletes the auto-closing pair around every caret (Monaco's
// _runAutoClosingPairDelete) when autoClosingPairDelete holds: a new undo
// step that further Backspaces continue.
func (s *State) deletePairs() bool {
	s.normalize()
	if !s.autoClosingPairDelete() {
		return false
	}
	snap := s.Snapshot()
	sels := s.clampedSels(snap)
	cmds := make([]opCmd, len(sels))
	for k, sel := range sels {
		a, b := pairRange(snap, sel.Head)
		cmds[k] = replaceCmd(k, buffer.Edit{Start: a, End: b})
	}
	s.Model.buf.PushUndoStop()
	return s.runOpsKind(cmds, kindDelete)
}
