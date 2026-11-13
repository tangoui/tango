package editor

import (
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/langs"
	"github.com/tangoui/tango/ui/editor/monarch"
)

// Bracket pairs (Monaco's BracketPairsTextModelPart).
//
// Brackets are the language's brackets plus its colorizedBracketPairs
// (Monaco's LanguageBracketsConfiguration); without colorizedBracketPairs
// every bracket pair but "<" ">" is colorized. Brackets are found the way
// Monaco's bracket tokenizer finds them: only in tokens whose standard type
// is Other (not in strings, comments or regular expressions), adjacent
// tokens of one type merged, matched case-insensitively and longest text
// first, with word boundaries around brackets that start or end with a
// word character. Lines longer than maxTokenizationLineLength are one Other
// token, as Monaco does not tokenize them. Embedded languages use the
// document's brackets.
//
// Pairs follow Monaco's bracket AST (bracketPairsTree/parser.js): an
// opening bracket opens a pair; a closing bracket closes the innermost open
// pair it closes, and the pairs opened inside that one stay unclosed; a
// closing bracket no open pair accepts is unexpected. Unclosed opening
// brackets and unexpected closing brackets are invalid
// (editorBracketHighlight.unexpectedBracket.foreground); the others take
// editorBracketHighlight.foreground1..6 by their nesting level among
// colorized pairs, the transparent ones skipped (Monaco's theming
// participant). Nesting deeper than maxBracketDepth makes further opening
// brackets plain text, as Monaco's parser does.
//
// Incremental state: the bracketTree keeps, per line, the stack of open
// brackets at its start (persistent stack nodes, shared between lines) and
// the lowest stack depth the line reaches. Like the Highlighter it keeps a
// sorted list of dirty lines: an edit marks its first line dirty; scanning
// a dirty line recomputes the next line's start stack, which becomes dirty
// unless the stack is unchanged, so an edit is rescanned only until the
// stacks agree again, and only as far as a caller asks (ensure). Lines are
// scanned only once the Highlighter's tokens for them are final.
//
// Pairs are built by the tree only for documents of up to
// maxBracketTreeLen bytes (Monaco's canBuildAST: 5 000 000 characters,
// counted here in UTF-8 bytes); bracket pair colorization needs them. In a
// larger document matchBrackets and the bracket commands use a local
// search (brackets_legacy.go), Monaco's fallback.
//
// The semantics follow Monaco 0.56 (vs/editor/common/model/
// bracketPairsTextModelPart), Copyright (c) Microsoft Corporation, MIT
// License (see ui/editor/theme/NOTICE).

const (
	maxBracketTreeLen         = 5_000_000
	maxBracketDepth           = 150   // Monaco's parser: level > 300, two levels per nesting
	maxTokenizationLineLength = 20000 // Monaco's maxTokenizationLineLength default
)

// bracketDef is one bracket text.
type bracketDef struct {
	text  string
	open  int   // opener id, -1 for a closing bracket
	opens []int // closing: the opener ids it closes
	color []int // closing: the opener ids it closes as a colorized pair
	wordL bool  // \b before (the text starts with a word character or space)
	wordR bool  // \b after
}

// bracketConf is a language's compiled bracket configuration.
type bracketConf struct {
	defs      []*bracketDef          // in Monaco's regexp order (texts sorted, reversed)
	byText    map[string]*bracketDef // lower-cased lookup as Monaco's getToken (closing wins)
	first     [256]bool              // first bytes of the texts, both cases
	colorized []bool                 // per opener id: closed colorized by some closing bracket
	opener    []string               // opener texts by id
}

// plainBrackets is Monaco's plaintext language configuration: brackets
// for matching, none colorized.
var plainBrackets = &langs.Conf{
	Brackets:              [][2]string{{"(", ")"}, {"[", "]"}, {"{", "}"}},
	ColorizedBracketPairs: [][2]string{},
}

var bracketConfs sync.Map // *langs.Conf → *bracketConf

// bracketConfOf returns the compiled brackets of conf (nil: plain text).
func bracketConfOf(conf *langs.Conf) *bracketConf {
	if conf == nil {
		conf = plainBrackets
	}
	if v, ok := bracketConfs.Load(conf); ok {
		return v.(*bracketConf)
	}
	v, _ := bracketConfs.LoadOrStore(conf, newBracketConf(conf))
	return v.(*bracketConf)
}

func validPairs(ps [][2]string) [][2]string {
	var out [][2]string
	for _, p := range ps {
		if p[0] != "" && p[1] != "" {
			out = append(out, p)
		}
	}
	return out
}

func newBracketConf(conf *langs.Conf) *bracketConf {
	bc := &bracketConf{byText: map[string]*bracketDef{}}
	opens := map[string]*bracketDef{}
	closes := map[string]*bracketDef{}
	opener := func(t string) *bracketDef {
		d := opens[t]
		if d == nil {
			d = &bracketDef{text: t, open: len(bc.opener)}
			opens[t] = d
			bc.opener = append(bc.opener, t)
			bc.colorized = append(bc.colorized, false)
		}
		return d
	}
	closer := func(t string) *bracketDef {
		d := closes[t]
		if d == nil {
			d = &bracketDef{text: t, open: -1}
			closes[t] = d
		}
		return d
	}
	pairs := validPairs(conf.Brackets)
	colorized := pairs
	if conf.ColorizedBracketPairs != nil {
		colorized = validPairs(conf.ColorizedBracketPairs)
	} else {
		colorized = slices.DeleteFunc(slices.Clone(pairs), func(p [2]string) bool { return p[0] == "<" && p[1] == ">" })
	}
	for _, p := range pairs {
		o, c := opener(p[0]), closer(p[1])
		if !slices.Contains(c.opens, o.open) {
			c.opens = append(c.opens, o.open)
		}
	}
	for _, p := range colorized {
		o, c := opener(p[0]), closer(p[1])
		if !slices.Contains(c.opens, o.open) {
			c.opens = append(c.opens, o.open)
		}
		if !slices.Contains(c.color, o.open) {
			c.color = append(c.color, o.open)
		}
		bc.colorized[o.open] = true
	}
	// The lookup map: openings first, closings overwrite (Monaco's map.set
	// order); keys are the texts as configured.
	texts := map[string]bool{}
	for _, d := range opens {
		bc.byText[d.text] = d
		texts[d.text] = true
	}
	for _, d := range closes {
		bc.byText[d.text] = d
		texts[d.text] = true
	}
	keys := make([]string, 0, len(texts))
	for t := range texts {
		keys = append(keys, t)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(keys)))
	for _, t := range keys {
		d := bc.byText[t]
		d.wordL = isWordOrSpace(t[0])
		d.wordR = isWordOrSpace(t[len(t)-1])
		bc.defs = append(bc.defs, d)
		bc.first[lowerASCII(t[0])] = true
		bc.first[upperASCII(t[0])] = true
	}
	return bc
}

func isWordOrSpace(b byte) bool { return isWordByte(b) || b == ' ' }

func lowerASCII(b byte) byte {
	if 'A' <= b && b <= 'Z' {
		return b + 'a' - 'A'
	}
	return b
}

func upperASCII(b byte) byte {
	if 'a' <= b && b <= 'z' {
		return b - 'a' + 'A'
	}
	return b
}

// closesColorized reports whether closing bracket d closes opener id as a
// colorized pair.
func (d *bracketDef) closesColorized(id int) bool { return slices.Contains(d.color, id) }

// bnode is an entry of a stack of open brackets; stacks are persistent
// (an entry's parent is the stack below it) and shared between lines. The
// empty stack is nil.
type bnode struct {
	parent *bnode
	open   int32  // opener id
	depth  int32  // entries including this one
	level  int32  // colorized entries including this one
	hash   uint64 // of the whole stack
}

// unknownStack is the start stack of a line not scanned yet: it equals no
// stack.
var unknownStack = &bnode{depth: -1}

func stackDepth(s *bnode) int32 {
	if s == nil {
		return 0
	}
	return s.depth
}

func stackLevel(s *bnode) int32 {
	if s == nil {
		return 0
	}
	return s.level
}

func stackEqual(a, b *bnode) bool {
	for a != b {
		if a == nil || b == nil || a == unknownStack || b == unknownStack ||
			a.depth != b.depth || a.open != b.open || a.hash != b.hash {
			return false
		}
		a, b = a.parent, b.parent
	}
	return true
}

func (bc *bracketConf) push(s *bnode, id int) *bnode {
	h := uint64(14695981039346656037)
	if s != nil {
		h = s.hash
	}
	h = (h ^ uint64(id+1)) * 1099511628211
	lv := stackLevel(s)
	if bc.colorized[id] {
		lv++
	}
	return &bnode{parent: s, open: int32(id), depth: stackDepth(s) + 1, level: lv, hash: h}
}

// Bracket events of a line scan.
const (
	evOpen       = iota // an opening bracket: node is its new stack entry
	evClose             // a closing bracket closing node's pair
	evUnexpected        // a closing bracket no open pair accepts
	evUnclosed          // node's pair stays unclosed (popped by a closing bracket of an outer pair)
)

type bracketEvent struct {
	kind   int
	b0, b1 int // the bracket's bytes in the line (evUnclosed: the closing bracket's)
	node   *bnode
	def    *bracketDef
}

// scanLine runs the bracket parser over one line from stack s. toks are
// the line's tokens (none: all text is Other). It returns the stack at the
// line end and the lowest depth reached; ev, when not nil, receives every
// bracket.
func (bc *bracketConf) scanLine(s *bnode, line string, toks []monarch.Token, ev func(bracketEvent)) (*bnode, int32) {
	minD := stackDepth(s)
	if len(bc.defs) == 0 {
		return s, minD
	}
	handle := func(d *bracketDef, b0, b1 int) {
		if d.open >= 0 {
			if stackDepth(s) >= maxBracketDepth {
				return // plain text, as Monaco's parser beyond its depth limit
			}
			s = bc.push(s, d.open)
			if ev != nil {
				ev(bracketEvent{kind: evOpen, b0: b0, b1: b1, node: s, def: d})
			}
			return
		}
		x := s
		for x != nil && !slices.Contains(d.opens, int(x.open)) {
			x = x.parent
		}
		if x == nil {
			if ev != nil {
				ev(bracketEvent{kind: evUnexpected, b0: b0, b1: b1, def: d})
			}
			return
		}
		for y := s; y != x; y = y.parent {
			if ev != nil {
				ev(bracketEvent{kind: evUnclosed, b0: b0, b1: b1, node: y, def: d})
			}
		}
		if ev != nil {
			ev(bracketEvent{kind: evClose, b0: b0, b1: b1, node: x, def: d})
		}
		s = x.parent
		minD = min(minD, stackDepth(s))
	}
	bc.forBrackets(line, toks, handle)
	return s, minD
}

// forBrackets calls fn with every bracket of line, in order: the
// brackets in its Other tokens (all of it without tokens, or when it is
// longer than maxTokenizationLineLength).
func (bc *bracketConf) forBrackets(line string, toks []monarch.Token, fn func(d *bracketDef, b0, b1 int)) {
	if len(bc.defs) == 0 {
		return
	}
	if len(toks) == 0 || len(line) > maxTokenizationLineLength {
		bc.scanText(line, 0, len(line), fn)
		return
	}
	for k := 0; k < len(toks); {
		// Merge the following tokens of the same type (Monaco merges
		// tokens with the same metadata).
		e := k + 1
		for e < len(toks) && toks[e].Type == toks[k].Type {
			e++
		}
		b0 := max(toks[k].ByteStart, 0)
		b1 := len(line)
		if e < len(toks) {
			b1 = min(toks[e].ByteStart, len(line))
		}
		if b0 < b1 && stdTypeCached(toks[k].Type) == "" {
			bc.scanText(line, b0, b1, fn)
		}
		k = e
	}
}

// scanText finds the brackets in line[b0:b1], one Other token, as
// Monaco's tokenizer does: the bracket regexp runs on the rest of the
// token after each bracket; a match that is no bracket text (a case
// variant) skips the rest of the token.
func (bc *bracketConf) scanText(line string, b0, b1 int, fn func(d *bracketDef, b0, b1 int)) {
	sub := b0 // start of the text the regexp runs on
	for p := b0; p < b1; p++ {
		if !bc.first[line[p]] {
			continue
		}
		for _, d := range bc.defs {
			n := len(d.text)
			if p+n > b1 || !strings.EqualFold(line[p:p+n], d.text) {
				continue
			}
			// \b: the text the regexp runs on is line[sub:b1].
			if d.wordL && (p > sub && isWordByte(line[p-1])) == isWordByte(line[p]) {
				continue
			}
			if d.wordR && (p+n < b1 && isWordByte(line[p+n])) == isWordByte(line[p+n-1]) {
				continue
			}
			got := bc.byText[strings.ToLower(line[p:p+n])]
			if got == nil {
				return
			}
			fn(got, p, p+n)
			p += n - 1
			sub = p + 1
			break
		}
	}
}

var stdTypes sync.Map // token type → standard type

// stdTypeCached is stdType, cached per token type.
func stdTypeCached(typ string) string {
	if typ == "" {
		return ""
	}
	if v, ok := stdTypes.Load(typ); ok {
		return v.(string)
	}
	v := stdType(typ)
	stdTypes.Store(typ, v)
	return v
}

// bracketSource gives the bracket tree the tokens of a line and how many
// leading lines have final tokens.
type bracketSource interface {
	tokens(snap buffer.Snapshot, line int, text string, dst []monarch.Token) []monarch.Token
	finalLines(snap buffer.Snapshot) int
}

// bracketTree is the incremental bracket state of one document (see the
// top of the file).
type bracketTree struct {
	conf  *bracketConf
	snap  buffer.Snapshot
	start []*bnode // start[i]: the stack at line i's start
	minD  []int32  // minD[i]: the lowest depth line i reaches
	dirty []int    // sorted lines to scan again
	toks  []monarch.Token

	// A run of lines read at once (lineText): reading line by line costs
	// a rope walk per line.
	chunkVer uint64
	chunkLo  int
	chunk    []string

	scanned int  // lines scanned (tests)
	unsure  bool // a query stopped at the end of the known lines
}

func newBracketTree(conf *bracketConf, snap buffer.Snapshot) *bracketTree {
	t := &bracketTree{conf: conf}
	t.reset(snap)
	return t
}

func (t *bracketTree) reset(snap buffer.Snapshot) {
	n := snap.LineCount()
	t.snap = snap
	t.start = slices.Grow(t.start[:0], n)[:n]
	t.minD = slices.Grow(t.minD[:0], n)[:n]
	for i := range t.start {
		t.start[i] = unknownStack
	}
	t.start[0] = nil
	t.dirty = append(t.dirty[:0], 0)
}

// known returns how many leading lines have a trusted start stack and
// lowest depth.
func (t *bracketTree) known() int {
	if len(t.dirty) > 0 {
		return t.dirty[0]
	}
	return len(t.start)
}

// complete reports whether every line is known.
func (t *bracketTree) complete() bool { return len(t.dirty) == 0 }

// ensure scans dirty lines until the lines before upTo are known, the
// deadline passes (a zero deadline: no limit) or the source's final lines
// end. It reports whether the lines before upTo are known.
func (t *bracketTree) ensure(src bracketSource, upTo int, deadline time.Time) bool {
	n := len(t.start)
	upTo = min(upTo, n)
	final := -1
	for len(t.dirty) > 0 && t.dirty[0] < upTo {
		i := t.dirty[0]
		if final < 0 || i >= final {
			if final = src.finalLines(t.snap); i >= final {
				return false
			}
		}
		text := t.lineText(i)
		t.toks = src.tokens(t.snap, i, text, t.toks[:0])
		end, md := t.conf.scanLine(t.start[i], text, t.toks, nil)
		t.scanned++
		t.minD[i] = md
		switch {
		case i+1 >= n:
			t.dirty = t.dirty[1:]
		case stackEqual(end, t.start[i+1]):
			t.dirty = t.dirty[1:]
		default:
			t.start[i+1] = end
			if len(t.dirty) > 1 && t.dirty[1] == i+1 {
				t.dirty = t.dirty[1:]
			} else {
				t.dirty[0] = i + 1
			}
		}
		if !deadline.IsZero() && t.scanned%64 == 0 && time.Now().After(deadline) {
			break
		}
	}
	return t.known() >= upTo
}

// lineText returns line i of the tree's snapshot, reading runs of lines.
func (t *bracketTree) lineText(i int) string {
	const run = 256
	if t.chunkVer != t.snap.Version() || i < t.chunkLo || i >= t.chunkLo+len(t.chunk) {
		n := t.snap.LineCount()
		last := min(i+run, n) - 1
		end := t.snap.Len()
		if last+1 < n {
			end = t.snap.LineStart(last+1) - 1
		}
		t.chunk = strings.Split(t.snap.Slice(t.snap.LineStart(i), end), "\n")
		t.chunkVer, t.chunkLo = t.snap.Version(), i
	}
	return t.chunk[i-t.chunkLo]
}

// markDirty adds line i to the dirty lines.
func (t *bracketTree) markDirty(i int) {
	k, found := slices.BinarySearch(t.dirty, i)
	if !found {
		t.dirty = slices.Insert(t.dirty, k, i)
	}
}

// edit follows the edits that turned old (the tree's snapshot) into
// new; any other snapshot rebuilds the tree.
func (t *bracketTree) edit(old, new buffer.Snapshot, edits []buffer.Edit) {
	if old.Version() != t.snap.Version() || old.LineCount() != len(t.start) {
		t.reset(new)
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
			regs[k-1].m += d - (el - regs[k-1].el)
			regs[k-1].el = el
			continue
		}
		regs = append(regs, region{sl, el, d + 1})
	}
	if old.LineCount()+total != new.LineCount() {
		t.reset(new)
		return
	}
	t.snap = new
	if len(regs) == 0 {
		return
	}
	if total == 0 && !slices.ContainsFunc(regs, func(r region) bool { return r.m != r.el-r.sl+1 }) {
		for _, r := range regs {
			t.markDirty(r.sl)
			for i := r.sl + 1; i <= r.el; i++ {
				t.start[i] = unknownStack
			}
		}
		return
	}
	// Rebuild the arrays: unchanged lines keep their state at their new
	// index; the lines of a region after its first are unknown.
	n := new.LineCount()
	start := make([]*bnode, 0, n)
	minD := make([]int32, 0, n)
	var dirty []int
	di := 0
	prev := 0
	shift := 0
	for _, r := range regs {
		start = append(start, t.start[prev:r.sl+1]...)
		minD = append(minD, t.minD[prev:r.sl+1]...)
		for ; di < len(t.dirty) && t.dirty[di] < r.sl; di++ {
			dirty = append(dirty, t.dirty[di]+shift)
		}
		for ; di < len(t.dirty) && t.dirty[di] <= r.el; di++ {
		}
		dirty = append(dirty, r.sl+shift)
		for k := 1; k < r.m; k++ {
			start = append(start, unknownStack)
			minD = append(minD, 0)
		}
		shift += r.m - (r.el - r.sl + 1)
		prev = r.el + 1
	}
	start = append(start, t.start[prev:]...)
	minD = append(minD, t.minD[prev:]...)
	for ; di < len(t.dirty); di++ {
		dirty = append(dirty, t.dirty[di]+shift)
	}
	t.start, t.minD, t.dirty = start, minD, dirty
}

// lineEvents scans line i (known) and returns its bracket events.
func (t *bracketTree) lineEvents(src bracketSource, i int, dst []bracketEvent) []bracketEvent {
	text := t.snap.Line(i)
	t.toks = src.tokens(t.snap, i, text, t.toks[:0])
	t.conf.scanLine(t.start[i], text, t.toks, func(e bracketEvent) { dst = append(dst, e) })
	return dst
}

// closeOf finds where the stack entry at depth d, open at the end of line
// i, is popped: the first line j > i whose lowest depth falls below d, and
// the event there (evClose: its pair closed; evUnclosed: it stays
// unclosed). It scans further lines (ensure) until deadline. found is
// false when no known line pops it; never then reports that every line is
// known (the entry stays open to the end of the document).
func (t *bracketTree) closeOf(src bracketSource, i int, d int32, deadline time.Time) (j int, e bracketEvent, found, never bool) {
	for j = i + 1; ; j++ {
		if j >= t.known() {
			if t.complete() {
				return -1, bracketEvent{}, false, true
			}
			k := t.known()
			t.ensure(src, k+4096, deadline)
			if t.known() <= j {
				t.unsure = true
				return -1, bracketEvent{}, false, false
			}
		}
		if t.minD[j] < d {
			for _, ev := range t.lineEvents(src, j, nil) {
				if (ev.kind == evClose || ev.kind == evUnclosed) && ev.node.depth == d {
					return j, ev, true, false
				}
			}
			return -1, bracketEvent{}, false, false
		}
	}
}

// openOf finds the opening bracket of the stack entry at depth d that is
// open at the start of line i: the last line k < i whose lowest depth is
// below d pushed it (line 0 starts empty, so there is one).
func (t *bracketTree) openOf(src bracketSource, i int, d int32) (k int, e bracketEvent, ok bool) {
	for k = i - 1; k >= 0; k-- {
		if t.minD[k] >= d {
			continue
		}
		for _, ev := range t.lineEvents(src, k, nil) {
			if ev.kind == evOpen && ev.node.depth == d {
				e, ok = ev, true
			}
		}
		return k, e, ok
	}
	return -1, bracketEvent{}, false
}
