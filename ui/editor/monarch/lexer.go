// Structure follows Monaco's monarchLexer/monarchCompile (vs/editor/standalone/common/monarch),
// Copyright (c) Microsoft Corporation, MIT License. See NOTICE in this directory.

package monarch

import (
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/editor/regex"
)

// MaxLineLength 是参与分词的最大行长（UTF-16 代码单元），与 Monaco 默认
// editor.maxTokenizationLineLength 相同。更长的行只对前 MaxLineLength 个单元分词，
// 其余部分作为一个 "" 类型 token，行尾状态等于输入状态（长行不影响后续行）。
const MaxLineLength = 20000

// LineBudget 是一行分词的时间预算。超过预算（或一行内第一次回溯正则超时，见 regex.MatchTimeout）
// 时该行降级：已产出的 token 保留，其余部分作为一个 "" token，行尾状态等于输入状态（同长行规则），
// LineInfo.Degraded=true。预算按本行分词的墙钟时间计（分词时间几乎全是正则匹配），
// 每 budgetCheckEvery 次匹配迭代或每次回溯引擎匹配后检查一次。第一次进入嵌入语言时编译其语法的耗时不计入。
// -race 构建中预算放大 10 倍（race.go）。
const LineBudget = 50 * time.Millisecond

const budgetCheckEvery = 16

// lineBudgetDur 是实际使用的预算：LineBudget 乘以 raceFactor（-race 构建慢 5~20 倍且停顿不可预测，
// 见 race.go）。只有测试会改它（隔离长行规则等与墙钟无关的语义）。
var lineBudgetDur = LineBudget * raceFactor

var nErrors, nPanics, nDegraded atomic.Int64

// Stats 返回全局计数：TokenizeLine 因规则执行出错而输出安全结果的次数（含 recover 的 panic），
// 其中由 recover 兜住的 panic 次数，以及因超时/超预算而降级的行数（见 LineInfo）。
// 内置语法上三者都应为 0（降级只出现在病态输入上）。
func Stats() (runtimeErrors, recoveredPanics, degradedLines int64) {
	return nErrors.Load(), nPanics.Load(), nDegraded.Load()
}

// LineInfo 是 TokenizeLineInfo 关于一行的附加信息。
type LineInfo struct {
	// Degraded 表示该行因病态正则（回溯超时或超过 LineBudget）没有完整分词：降级点之后是一个
	// "" token，返回的状态是输入状态，因而可能与真实的行尾状态不同。调用方不应把"行尾状态未变"
	// 当作可以停止向下重新分词的依据（例如可在空闲时重试，或继续向下分词若干行）。
	Degraded bool
}

// errDegraded 表示从行内字节偏移 off 起放弃分词（超时或超预算）。
type errDegraded struct{ off int }

func (e *errDegraded) Error() string { return fmt.Sprintf("line degraded at byte %d", e.off) }

// Token 是一行中一个 token 的起点与类型。
type Token struct {
	Start     int    // UTF-16 列，对应 Monaco 的 startIndex
	ByteStart int    // 行内 UTF-8 字节偏移，供渲染使用
	Type      string // 带 tokenPostfix 的类型，如 "keyword.go"；"" 表示无类型
}

// Lexer 是一种语言编译后的 Monarch 分词器。可被多个 goroutine 并发使用。
type Lexer struct {
	reg     *Registry
	id      string
	g       *grammar
	intern  interner
	initial State
}

func newLexer(reg *Registry, id string, g *grammar) *Lexer {
	l := &Lexer{reg: reg, id: id, g: g}
	l.intern.stacks = map[stackKey]*stackElem{}
	l.intern.ids = map[lineState]State{}
	l.initial = l.internState(lineState{stack: l.stackElem(nil, g.start)})
	return l
}

// ID 返回语言 ID。
func (l *Lexer) ID() string { return l.id }

// Warnings 返回编译时无法编译、因而永不匹配的正则等提示（Monaco 中这些正则可用）。
func (l *Lexer) Warnings() []string { return append([]string(nil), l.g.warnings...) }

// Initial 返回行首的初始状态。
func (l *Lexer) Initial() State { return l.initial }

// TokenizeLine 对一行（不含换行符）分词，把 token 追加到 dst，返回追加后的切片与行尾状态。
// st 为上一行的行尾状态（第一行用 Initial()；0 或未知状态视同 Initial()）。
// 规则执行出错时（Monaco 会抛异常的情形）与 Monaco 编辑器一样，整行输出一个 "" token，状态不变。
// 等同 TokenizeLineInfo 但丢弃 LineInfo；需要知道行是否降级的调用方用 TokenizeLineInfo。
func (l *Lexer) TokenizeLine(line string, st State, dst []Token) ([]Token, State) {
	dst, st, _ = l.TokenizeLineInfo(line, st, dst)
	return dst, st
}

// TokenizeLineInfo 同 TokenizeLine，另返回 LineInfo。病态正则（回溯超时，或本行分词超过
// LineBudget）使该行降级：降级点之前的 token 保留，其后是一个 "" token，返回输入状态 st，
// Degraded=true。见 LineInfo。
func (l *Lexer) TokenizeLineInfo(line string, st State, dst []Token) ([]Token, State, LineInfo) {
	ls, ok := l.lookupState(st)
	if !ok {
		st, ls = l.initial, l.mustState(l.initial)
	}
	base := len(dst)
	cut, cutCol := -1, 0
	if len(line) > MaxLineLength { // UTF-16 长度不超过字节长度
		cut, cutCol = cutUTF16(line, MaxLineLength)
	}
	c := collector{toks: dst, budget: lineBudget{start: time.Now()}}
	text, hasEOL := line, true
	if cut >= 0 {
		text, hasEOL = line[:cut], false
	}
	end, err := l.safeTokenize(text, hasEOL, ls, &c)
	dst = c.toks
	if de, ok := errors.AsType[*errDegraded](err); ok {
		nDegraded.Add(1)
		cut = de.off
		for len(dst) > base && dst[len(dst)-1].ByteStart >= cut {
			dst = dst[:len(dst)-1]
		}
		if n := len(dst); n > base && dst[n-1].Type == "" { // 已是 "" token：保持合并
			fillUTF16(line, dst[base:])
			return dst, st, LineInfo{Degraded: true}
		}
		dst = append(dst, Token{ByteStart: cut})
		fillUTF16(line, dst[base:])
		return dst, st, LineInfo{Degraded: true}
	}
	if err != nil {
		nErrors.Add(1)
		return append(dst[:base], Token{}), st, LineInfo{}
	}
	fillUTF16(line, dst[base:])
	if cut >= 0 {
		if n := len(dst); n > base && dst[n-1].Type == "" { // 已是 "" token：保持合并
			return dst, st, LineInfo{}
		}
		return append(dst, Token{Start: cutCol, ByteStart: cut}), st, LineInfo{}
	}
	return dst, l.internState(end), LineInfo{}
}

// lineBudget 记录一行分词的开始时间，用于 LineBudget 检查。
type lineBudget struct {
	start time.Time
	n     int
}

// check 每 budgetCheckEvery 次（force 时每次）检查本行是否超过 LineBudget；超过返回 errDegraded。
func (b *lineBudget) check(off int, force bool) error {
	b.n++
	if !force && b.n%budgetCheckEvery != 0 {
		return nil
	}
	if time.Since(b.start) > lineBudgetDur {
		return &errDegraded{off: off}
	}
	return nil
}

// match 执行一次锚定匹配；回溯超时转为 errDegraded（off 为本次匹配在行内的字节起点）。
func (b *lineBudget) match(re regex.Regexp, s string, rs []rune, off int) ([]int, error) {
	m, err := re.TryMatchStart(s, rs)
	if err != nil {
		return nil, &errDegraded{off: off}
	}
	return m, nil
}

func (l *Lexer) mustState(st State) lineState {
	ls, _ := l.lookupState(st)
	return ls
}

// cutUTF16 返回前 max 个 UTF-16 单元的字节长度与实际列数（不切开代理对）；行不超长时返回 -1。
func cutUTF16(s string, max int) (cutByte, col int) {
	for i, r := range s {
		w := 1
		if r >= 0x10000 {
			w = 2
		}
		if col+w > max {
			return i, col
		}
		col += w
	}
	return -1, col
}

// fillUTF16 由 ByteStart 计算 Start（UTF-16 列）。
func fillUTF16(line string, toks []Token) {
	ascii := true
	for i := 0; i < len(line); i++ {
		if line[i] >= utf8.RuneSelf {
			ascii = false
			break
		}
	}
	if ascii {
		for i := range toks {
			toks[i].Start = toks[i].ByteStart
		}
		return
	}
	b, u := 0, 0
	for i := range toks {
		off := toks[i].ByteStart
		if off < b {
			b, u = 0, 0
		}
		for b < off && b < len(line) {
			r, size := utf8.DecodeRuneInString(line[b:])
			b += size
			if r >= 0x10000 {
				u += 2
			} else {
				u++
			}
		}
		toks[i].Start = u
	}
}

func decodeRune(s string) (rune, int) { return utf8.DecodeRuneInString(s) }

// collector 对应 Monaco 的 MonarchClassicTokensCollector：相邻同类型（且同语言）的 token 合并。
type collector struct {
	toks     []Token
	budget   lineBudget
	lang     string
	lastType string
	lastLang string
	hasLast  bool
}

func (c *collector) emit(off int, typ string) {
	if c.hasLast && c.lastType == typ && c.lastLang == c.lang {
		return
	}
	c.hasLast, c.lastType, c.lastLang = true, typ, c.lang
	c.toks = append(c.toks, Token{ByteStart: off, Type: typ})
}

// nested 用嵌入语言分词一段文本，返回嵌入语言的新状态。
func (c *collector) nested(line string, hasEOL bool, e embedded, off int) (State, error) {
	if e.lexer == nil {
		c.lang = "\x00" + e.lang
		c.emit(off, "")
		return e.state, nil
	}
	c.hasLast, c.lang = false, ""
	var st State
	if utf16Len(line) >= MaxLineLength { // 嵌入语言自己的 tokenize 也有长度检查（nullTokenize）
		c.toks = append(c.toks, Token{ByteStart: off})
		st = e.state
	} else {
		ls, ok := e.lexer.lookupState(e.state)
		if !ok {
			ls = e.lexer.mustState(e.lexer.initial)
		}
		end, err := e.lexer.tokenizeLS(line, hasEOL, ls, off, c)
		if err != nil {
			return 0, err
		}
		st = e.lexer.internState(end)
	}
	c.hasLast, c.lang = false, ""
	return st, nil
}

func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r >= 0x10000 {
			n++
		}
	}
	return n
}

func (l *Lexer) errorf(format string, a ...any) error {
	return fmt.Errorf("monarch: %s: %s", l.id, fmt.Sprintf(format, a...))
}

// safeTokenize 是 TokenizeLine 的分词入口：不可信插件语法引发的 panic 转为错误
// （调用方据此输出安全结果），并计入 Stats 的 recoveredPanics。
func (l *Lexer) safeTokenize(line string, hasEOL bool, ls lineState, c *collector) (end lineState, err error) {
	defer func() {
		if p := recover(); p != nil {
			nPanics.Add(1)
			end, err = ls, l.errorf("panic in tokenizer: %v", p)
		}
	}()
	return l.tokenizeLS(line, hasEOL, ls, 0, c)
}

// tokenizeLS 对应 MonarchTokenizer._tokenize。
func (l *Lexer) tokenizeLS(line string, hasEOL bool, ls lineState, off int, c *collector) (lineState, error) {
	if ls.emb.active {
		return l.nestedTokenize(line, hasEOL, ls, off, c)
	}
	return l.myTokenize(line, hasEOL, ls, off, c)
}

// findLeavingNestedOffset 对应 _findLeavingNestedLanguageOffset：在当前状态里找带
// nextEmbedded @pop 的规则，返回最早的匹配位置（字节）；都不匹配返回 -1。
func (l *Lexer) findLeavingNestedOffset(line string, ls lineState, off int, c *collector) (int, error) {
	st := ls.stack
	if !st.ok {
		return 0, l.errorf("tokenizer state is not defined: %s", st.state)
	}
	pop, hasPopRule := -1, false
	for _, r := range st.rules {
		a := r.action
		if !(a.kind == actToken && a.nextEmbed == "@pop" || a.kind == actCases && a.hasEmbeddedEndInCases) {
			continue
		}
		hasPopRule = true
		res, err := r.m.resolve(st.state).searchIndex(line, off, &c.budget)
		if err != nil {
			return 0, err
		}
		if res == -1 || res != 0 && r.atLineStart {
			continue
		}
		if pop == -1 || res < pop {
			pop = res
		}
	}
	if !hasPopRule {
		return 0, l.errorf(`no rule containing nextEmbedded: "@pop" in tokenizer embedded state: %s`, st.state)
	}
	return pop, nil
}

func (l *Lexer) nestedTokenize(line string, hasEOL bool, ls lineState, off int, c *collector) (lineState, error) {
	pop, err := l.findLeavingNestedOffset(line, ls, off, c)
	if err != nil {
		return ls, err
	}
	if pop == -1 {
		st, err := c.nested(line, hasEOL, ls.emb, off)
		if err != nil {
			return ls, err
		}
		e := ls.emb
		e.state = st
		return lineState{stack: ls.stack, emb: e}, nil
	}
	if pop > 0 {
		if _, err := c.nested(line[:pop], false, ls.emb, off); err != nil {
			return ls, err
		}
	}
	return l.myTokenize(line[pop:], hasEOL, ls, off+pop, c)
}

// longLineBytes：剩余部分超过这个字节数时，整行转成一次 []rune 供回溯引擎的规则复用。
// 否则 regexp2 每次匹配都要把"行的剩余部分"转成 []rune，长行上逐位置尝试就是平方级的。
const longLineBytes = 256

type lineRunes struct {
	runes []rune
	index []int32 // 字节偏移 → rune 下标（非法字节各算一个 rune，与 []rune(s) 一致）
}

func newLineRunes(line string) *lineRunes {
	lr := &lineRunes{runes: make([]rune, 0, len(line)), index: make([]int32, len(line)+1)}
	for i, r := range line {
		_, size := utf8.DecodeRuneInString(line[i:])
		for k := 0; k < size; k++ {
			lr.index[i+k] = int32(len(lr.runes))
		}
		lr.runes = append(lr.runes, r)
	}
	lr.index[len(line)] = int32(len(lr.runes))
	return lr
}

// maxEmptyRun 是允许的连续无进展次数（空匹配，或 goBack 后没有越过本行已到达的最远位置；内置语法远低于此值）。
const maxEmptyRun = 1000

type groupEntry struct {
	action  *action
	matched capt
}

type groupMatching struct {
	rule    *rule
	matches []capt
	groups  []groupEntry
}

// myTokenize 对应 MonarchTokenizer._myTokenize。偏移均为字节；off 是 lineNoLF 在整行中的起点。
func (l *Lexer) myTokenize(lineNoLF string, hasEOL bool, ls lineState, off int, c *collector) (lineState, error) {
	g := l.g
	c.lang = l.id
	lineNoLFLen := len(lineNoLF)
	line := lineNoLF
	if hasEOL && g.includeLF {
		line += "\n"
	}
	lineLen := len(line)
	emb := ls.emb
	stack := ls.stack
	pos := 0
	var gm *groupMatching
	force := true // 空行也至少执行一次规则（monaco-editor#1235）
	ctx := &matchCtx{budget: &c.budget}
	var lr *lineRunes // 长行上整行的 []rune，供回溯引擎复用（见 longLineBytes）
	emptyRun := 0     // 连续无进展（没有越过 high）的次数
	high := 0         // 本行目前到达过的最远位置；只有越过它才算进展

	for force || pos < lineLen {
		pos0 := pos
		depth0 := stack.depth
		groupLen0 := 0
		if gm != nil {
			groupLen0 = len(gm.groups)
		}
		state := stack.state
		var (
			matches  []capt
			matched  string
			act      *action
			r        *rule
			entering string
			hasEnter bool
		)
		if gm != nil && len(gm.groups) > 0 { // 同 Monaco：空分组动作留下 0 个分组，走普通匹配
			matches = gm.matches
			ge := gm.groups[0]
			gm.groups = gm.groups[1:]
			if !ge.matched.ok {
				// JS 中 undefined.length 抛 TypeError
				return ls, l.errorf("group did not participate in the match in rule: %s", gm.rule.name)
			}
			matched, act, r = ge.matched.s, ge.action, gm.rule
			if len(gm.groups) == 0 {
				gm = nil
			}
		} else {
			if !force && pos >= lineLen {
				break
			}
			force = false
			if !stack.ok {
				return ls, l.errorf("tokenizer state is not defined: %s", state)
			}
			rest := line[pos:]
			if lr == nil && len(rest) > longLineBytes {
				lr = newLineRunes(line)
			}
			var restRunes []rune
			if lr != nil {
				restRunes = lr.runes[lr.index[pos]:]
			}
			backtracked := false
			for _, ru := range stack.rules {
				if pos != 0 && ru.atLineStart {
					continue
				}
				re := ru.m.resolve(state).re
				if re == nil {
					continue
				}
				backtracked = backtracked || re.Engine() == regex.Backtrack
				m, err := c.budget.match(re, rest, restRunes, off+pos)
				if err != nil {
					return ls, err
				}
				if m != nil {
					matches = toCaptures(rest, m)
					matched, act, r = matches[0].s, ru.action, ru
					break
				}
			}
			if err := c.budget.check(off+pos, backtracked); err != nil {
				return ls, err
			}
		}
		if matches == nil {
			matches = []capt{{"", true}}
			matched = ""
		}
		if act == nil {
			// 没有规则匹配：前进一个字符，按 defaultToken 输出
			if pos < lineLen {
				_, size := decodeRune(line[pos:])
				matched = line[pos : pos+size]
				matches = []capt{{matched, true}}
			}
			act = g.defAction
		}
		pos += len(matched)

		ctx.matched, ctx.matches, ctx.state, ctx.eos = matched, matches, state, pos == lineLen
		ctx.timedOut = false
		for act.kind == actCases {
			act = act.test(ctx, g.defAction)
		}
		if ctx.timedOut { // cases 里 ~regex 守卫的回溯匹配超时
			return ls, &errDegraded{off: off + pos0}
		}

		var result string
		var group []*action
		switch act.kind {
		case actString:
			result = act.token
		case actGroup:
			group = act.group
		case actToken:
			result = act.token
			if act.tokenSubst {
				result = g.substitute(act.token, ctx, g.lexerAttrString)
			}
			if act.nextEmbed != "" {
				switch {
				case act.nextEmbed == "@pop":
					if !emb.active {
						return ls, l.errorf("cannot pop embedded language if not inside one")
					}
					emb = embedded{}
				case emb.active:
					return ls, l.errorf("cannot enter embedded language from within an embedded language")
				default:
					entering, hasEnter = g.substitute(act.nextEmbed, ctx, g.lexerAttrString), true
				}
			}
			if act.goBack > 0 {
				// Monaco 不限制回退量；这里不退到本次匹配起点之前，pos 才单调不减（见下方进展判断）
				pos = goBack(line, pos, pos0, act.goBack)
			}
			switch {
			case act.switchTo != "":
				ns := strings.TrimPrefix(g.substitute(act.switchTo, ctx, g.lexerAttrString), "@")
				if _, ok := g.findRules(ns); !ok {
					return ls, l.errorf("trying to switch to a state '%s' that is undefined in rule: %s", ns, ruleName(r))
				}
				stack = l.stackElem(stack.parent, ns)
			case act.next == "@push":
				if stack.depth >= maxStack {
					return ls, l.errorf("maximum tokenizer stack size reached: [%s,...]", stack.state)
				}
				stack = l.stackElem(stack, state)
			case act.next == "@pop":
				if stack.depth <= 1 {
					return ls, l.errorf("trying to pop an empty stack in rule: %s", ruleName(r))
				}
				stack = stack.parent
			case act.next == "@popall":
				stack = stack.popall()
			case act.next != "":
				ns := strings.TrimPrefix(g.substitute(act.next, ctx, g.lexerAttrString), "@")
				if _, ok := g.findRules(ns); !ok {
					return ls, l.errorf("trying to set a next state '%s' that is undefined in rule: %s", ns, ruleName(r))
				}
				if stack.depth >= 1000 { // Monaco 对具名 next 不设上限；这里防止状态无界增长
					return ls, l.errorf("tokenizer stack too deep in rule: %s", ruleName(r))
				}
				stack = l.stackElem(stack, ns)
			}
		}

		enterEmbedded := func() (lineState, error) {
			// 第一次进入某个嵌入语言时会在这里编译它的语法：编译耗时不计入本行的匹配预算
			t0 := time.Now()
			e := l.reg.embeddedFor(entering)
			c.budget.start = c.budget.start.Add(time.Since(t0))
			if pos < lineLen {
				rest := ""
				if pos < lineNoLFLen {
					rest = lineNoLF[pos:]
				}
				return l.nestedTokenize(rest, hasEOL, lineState{stack: stack, emb: e}, off+pos, c)
			}
			return lineState{stack: stack, emb: e}, nil
		}

		if act.kind == actGroup {
			if gm != nil && len(gm.groups) > 0 {
				return ls, l.errorf("groups cannot be nested: %s", ruleName(r))
			}
			if len(matches) != len(group)+1 {
				return ls, l.errorf("matched number of groups does not match the number of actions in rule: %s", ruleName(r))
			}
			total := 0
			for _, m := range matches[1:] {
				if !m.ok {
					return ls, l.errorf("group did not participate in the match in rule: %s", ruleName(r))
				}
				total += len(m.s)
			}
			if total != len(matched) {
				return ls, l.errorf("with groups, all characters should be matched in consecutive groups in rule: %s", ruleName(r))
			}
			gm = &groupMatching{rule: r, matches: matches, groups: make([]groupEntry, len(group))}
			for i, a := range group {
				gm.groups[i] = groupEntry{action: a, matched: matches[i+1]}
			}
			pos -= len(matched)
			if len(group) == 0 {
				// 空分组动作（只能配空匹配）没有分组项可执行，会在原地反复展开（Monaco 死循环）：计为无进展
				emptyRun++
				if emptyRun > maxEmptyRun {
					return ls, l.errorf("no progress in tokenizer (empty group action) in rule: %s", ruleName(r))
				}
			}
			continue
		}

		if result == "@rematch" {
			// @rematch 从本次匹配起点重来，忽略 goBack（Monaco 是 pos -= matched.length，
			// 与 goBack 叠加会退到匹配之前甚至负数）
			pos = pos0
			matched = ""
			result = ""
			if hasEnter {
				return enterEmbedded()
			}
		}
		if len(matched) == 0 {
			gl := 0
			if gm != nil {
				gl = len(gm.groups)
			}
			if lineLen == 0 || depth0 != stack.depth || state != stack.state || gl != groupLen0 {
				// Monaco 把状态变化算作进展；但两个状态靠空匹配来回切换（push/pop、switchTo）
				// 会永远循环，这里限制同一位置上连续空匹配的次数。
				emptyRun++
				if emptyRun > maxEmptyRun {
					return ls, l.errorf("no progress in tokenizer (empty-match cycle) in rule: %s", ruleName(r))
				}
				continue
			}
			return ls, l.errorf("no progress in tokenizer in rule: %s", ruleName(r))
		}
		progress := pos > high
		if progress {
			high, emptyRun = pos, 0
		} else {
			// 非空匹配但 goBack 退回、没有越过已到达的最远位置：同样算无进展，与空匹配共用上限
			emptyRun++
			if emptyRun > maxEmptyRun {
				return ls, l.errorf("no progress in tokenizer (goBack without advancing) in rule: %s", ruleName(r))
			}
		}
		var typ string
		if strings.HasPrefix(result, "@brackets") {
			b := g.findBracket(matched)
			if b == nil {
				return ls, l.errorf("@brackets token returned but no bracket defined as: %s", matched)
			}
			typ = sanitize(b.token + result[len("@brackets"):])
		} else if result != "" {
			typ = sanitize(result + g.tokenPostfix)
		}
		// 无进展的迭代不输出 token（其宽度会被同位置的下一个 token 覆盖为 0），token 数因此不超过行长
		if progress && pos0 < lineNoLFLen {
			c.emit(pos0+off, typ)
		}
		if hasEnter {
			return enterEmbedded()
		}
	}
	return lineState{stack: stack, emb: emb}, nil
}

// test 依次尝试 cases 分支，返回第一个命中的值；都不命中返回 def（defaultToken）。
func (a *action) test(c *matchCtx, def *action) *action {
	for _, arm := range a.cases {
		if arm.test == nil || arm.test(c) {
			return arm.value
		}
	}
	return def
}

func ruleName(r *rule) string {
	if r == nil {
		return "(unknown)"
	}
	return r.name
}

func toCaptures(s string, m []int) []capt {
	out := make([]capt, len(m)/2)
	for i := range out {
		a, b := m[2*i], m[2*i+1]
		if a >= 0 && b >= a {
			out[i] = capt{s[a:b], true}
		}
	}
	return out
}

// goBack 把 pos 回退 n 个 UTF-16 单元，但不小于 lo（本次匹配的起点）。
func goBack(line string, pos, lo, n int) int {
	for u := 0; pos > lo && u < n; {
		r, size := utf8.DecodeLastRuneInString(line[:pos])
		pos -= size
		u++
		if r >= 0x10000 {
			u++
		}
	}
	return pos
}

// sanitize 同 Monaco：token 类型里的 & < > ' " _ 换成 -。
func sanitize(s string) string {
	if !strings.ContainsAny(s, `&<>'"_`) {
		return s
	}
	b := []byte(s)
	for i, ch := range b {
		switch ch {
		case '&', '<', '>', '\'', '"', '_':
			b[i] = '-'
		}
	}
	return string(b)
}
