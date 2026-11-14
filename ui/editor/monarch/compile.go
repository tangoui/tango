// Structure follows Monaco's monarchLexer/monarchCompile (vs/editor/standalone/common/monarch),
// Copyright (c) Microsoft Corporation, MIT License. See NOTICE in this directory.

package monarch

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/tangoui/tango/ui/editor/regex"
)

// grammar 是编译后的 Monarch 语言定义（对应 Monaco 的 ILexer）。
type grammar struct {
	id           string
	src          *node // 原始定义，编译期 @attr 与 cases 的 @name 从这里取
	ignoreCase   bool
	unicode      bool
	includeLF    bool
	start        string
	tokenPostfix string
	defaultToken string
	stateNames   map[string]bool
	tokenizer    map[string][]*rule
	brackets     []bracket
	defAction    *action // 无规则匹配或 cases 全不中时的动作：defaultToken 字符串
	warnings     []string
}

type bracket struct{ open, close, token string }

const maxStack = 100

// rule 是一条编译后的规则。
type rule struct {
	name        string
	m           *matcher
	atLineStart bool // 源码以 ^ 开头：只在行（或嵌入后剩余片段）开头尝试
	action      *action
}

type actKind uint8

const (
	actString actKind = iota // 纯 token 字符串
	actToken                 // {token, next, switchTo, ...}
	actGroup                 // 数组：按捕获组分配
	actCases                 // {cases: ...}
)

type action struct {
	kind       actKind
	token      string
	tokenSubst bool
	next       string
	switchTo   string
	goBack     int
	nextEmbed  string
	group      []*action
	cases      []caseArm
	// hasEmbeddedEndInCases 同 Monaco：某个分支（递归）带 nextEmbedded @pop/@popall。
	hasEmbeddedEndInCases bool
}

type caseArm struct {
	test  func(*matchCtx) bool // nil 表示 @default
	value *action
}

// capt 是一个捕获组；ok=false 对应 JS 的 undefined（组未参与匹配）。
type capt struct {
	s  string
	ok bool
}

type matchCtx struct {
	matched  string
	matches  []capt
	state    string
	eos      bool
	budget   *lineBudget
	timedOut bool // guard failure latched until the current token is abandoned
}

func (c *matchCtx) stopped() bool {
	return c != nil && (c.timedOut || c.budget != nil && c.budget.exhausted)
}

type compileError struct{ msg string }

func (e *compileError) Error() string { return e.msg }

func (g *grammar) errorf(format string, a ...any) error {
	return &compileError{g.id + ": " + fmt.Sprintf(format, a...)}
}

// compileGrammar 把 Monarch JSON 编译成 grammar（对应 monarchCompile.compile）。
func compileGrammar(id string, src *node) (g *grammar, err error) {
	if src == nil || src.kind != kObject {
		return nil, fmt.Errorf("monarch: %s: expecting a language definition object", id)
	}
	g = &grammar{
		id:           id,
		src:          src,
		ignoreCase:   boolProp(src.get("ignoreCase"), false),
		unicode:      boolProp(src.get("unicode"), false),
		includeLF:    boolProp(src.get("includeLF"), false),
		tokenPostfix: stringProp(src.get("tokenPostfix"), "."+id),
		defaultToken: stringProp(src.get("defaultToken"), "source"),
		stateNames:   map[string]bool{},
		tokenizer:    map[string][]*rule{},
	}
	if s := src.get("start"); s.isString() {
		g.start = s.str
	}
	g.defAction = &action{kind: actString, token: g.defaultToken}
	tk := src.get("tokenizer")
	if tk == nil || tk.kind != kObject {
		return nil, g.errorf("a language definition must define the 'tokenizer' attribute as an object")
	}
	for _, k := range tk.keys {
		g.stateNames[k] = true
	}
	for i, key := range tk.keys {
		if g.start == "" {
			g.start = key
		}
		var rules []*rule
		if err := g.addRules("tokenizer."+key, &rules, tk.vals[i], 0); err != nil {
			return nil, err
		}
		g.tokenizer[key] = rules
	}
	if err := g.compileBrackets(); err != nil {
		return nil, err
	}
	return g, nil
}

func boolProp(n *node, def bool) bool {
	if n != nil && n.kind == kBool {
		return n.b
	}
	return def
}

func stringProp(n *node, def string) string {
	if n.isString() {
		return n.str
	}
	return def
}

func (g *grammar) addRules(state string, out *[]*rule, rules *node, depth int) error {
	if depth > 64 {
		return g.errorf("include nesting too deep (cycle?) at: %s", state)
	}
	if rules == nil || rules.kind != kArray {
		return g.errorf("state rules must be an array at: %s", state)
	}
	for _, rn := range rules.arr {
		if inc := rn.get("include"); inc.truthy() {
			if !inc.isString() {
				return g.errorf("an 'include' attribute must be a string at: %s", state)
			}
			name := strings.TrimPrefix(inc.str, "@")
			target := g.src.get("tokenizer").get(name)
			if target == nil {
				return g.errorf("include target '%s' is not defined at: %s", name, state)
			}
			if err := g.addRules(state+"."+name, out, target, depth+1); err != nil {
				return err
			}
			continue
		}
		r := &rule{name: state}
		var actNode *node
		switch {
		case rn.kind == kArray && len(rn.arr) >= 1 && len(rn.arr) <= 3:
			if err := g.setRegex(r, rn.arr[0]); err != nil {
				return err
			}
			if len(rn.arr) >= 3 {
				switch a1 := rn.arr[1]; a1.kind {
				case kString:
					actNode = &node{kind: kObject, keys: []string{"token", "next"}, vals: []*node{a1, rn.arr[2]}}
				case kObject:
					// Monaco 直接给对象赋 next（rule1.next = rule[2]）。
					c := *a1
					c.keys = append([]string(nil), a1.keys...)
					c.vals = append([]*node(nil), a1.vals...)
					if i := indexOf(c.keys, "next"); i >= 0 {
						c.vals[i] = rn.arr[2]
					} else {
						c.keys = append(c.keys, "next")
						c.vals = append(c.vals, rn.arr[2])
					}
					actNode = &c
				case kArray: // JS 里数组也是对象：next 被挂在数组上，编译成组动作时被忽略
					actNode = a1
				default:
					return g.errorf("a next state as the last element of a rule can only be given if the action is either an object or a string, at: %s", state)
				}
			} else if len(rn.arr) == 2 {
				actNode = rn.arr[1]
			}
		default:
			re := rn.get("regex")
			if !re.truthy() {
				return g.errorf("a rule must either be an array, or an object with a 'regex' or 'include' field at: %s", state)
			}
			if nm := rn.get("name"); nm.isString() && nm.str != "" {
				r.name = nm.str
			}
			if err := g.setRegex(r, re); err != nil {
				return err
			}
			actNode = rn.get("action")
		}
		a, err := g.compileAction(r.name, actNode)
		if err != nil {
			return err
		}
		r.action = a
		*out = append(*out, r)
	}
	return nil
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

func (g *grammar) setRegex(r *rule, re *node) error {
	var src string
	switch {
	case re.isString():
		src = re.str
	case re != nil && re.kind == kRegex:
		src = re.str
	default:
		return g.errorf("rules must start with a match string or regular expression: %s", r.name)
	}
	r.atLineStart = len(src) > 0 && src[0] == '^'
	r.name += ": " + src
	if r.atLineStart {
		src = src[1:]
	}
	m, err := g.compileRegExp(src, true)
	if err != nil {
		return err
	}
	r.m = m
	return nil
}

// attr 返回编译期可见的属性（Monaco 的 lexerMin：语言定义本身，外加 languageId、defaultToken）。
func (g *grammar) attr(name string) *node {
	switch name {
	case "languageId":
		return &node{kind: kString, str: g.id}
	case "defaultToken":
		return &node{kind: kString, str: g.defaultToken}
	}
	return g.src.get(name)
}

func isWordByte(c byte) bool {
	return c == '_' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// maxExpandedRegex 是 @attr 展开后正则源码的最大长度（字节）。属性可以互相引用，
// 5 轮展开的长度随引用数指数增长（几百字节的语法就能展开到 GB 级）；超过上限即编译错误。
// 内置语法展开后最长不到几 KiB。
const maxExpandedRegex = 64 << 10

// expandAttrs 展开正则源码里的 @attr（最多 5 轮，@@ 表示字面 @）。
// 展开过程中长度一旦超过 maxExpandedRegex 就报错，不会先构造出巨大的字符串。
func (g *grammar) expandAttrs(str string) (string, error) {
	str = strings.ReplaceAll(str, "@@", "\x01")
	for n := 0; n < 5; n++ {
		had := false
		var sb strings.Builder
		for i := 0; i < len(str); {
			if str[i] != '@' {
				k := strings.IndexByte(str[i:], '@')
				if k < 0 {
					k = len(str) - i
				}
				if sb.Len()+k > maxExpandedRegex {
					return "", g.errorf("regular expression too long after expanding attributes (> %d bytes), at: %.200s", maxExpandedRegex, str)
				}
				sb.WriteString(str[i : i+k])
				i += k
				continue
			}
			j := i + 1
			for j < len(str) && isWordByte(str[j]) {
				j++
			}
			if j == i+1 {
				sb.WriteByte('@')
				i++
				continue
			}
			had = true
			name := str[i+1 : j]
			a := g.attr(name)
			var sub string
			switch {
			case a.isString():
				sub = a.str
			case a != nil && a.kind == kRegex:
				sub = a.str
			case a == nil:
				return "", g.errorf("language definition does not contain attribute '%s', used at: %s", name, str)
			default:
				return "", g.errorf("attribute reference '%s' must be a string, used at: %s", name, str)
			}
			if sub != "" {
				if sb.Len()+len(sub)+4 > maxExpandedRegex {
					return "", g.errorf("regular expression too long after expanding attributes (> %d bytes), at: %.200s", maxExpandedRegex, str)
				}
				sb.WriteString("(?:" + sub + ")")
			}
			i = j
		}
		str = sb.String()
		if !had {
			break
		}
	}
	return strings.ReplaceAll(str, "\x01", "@"), nil
}

func (g *grammar) flags() string {
	f := ""
	if g.ignoreCase {
		f += "i"
	}
	if g.unicode {
		f += "u"
	}
	return f
}

var reStateRef = regexp.MustCompile(`\$[sS](\d\d?)`)

// compileRegExp 对应 Monaco 的 compileRegExp：展开 @attr；handleSn 时含 $Sn 的正则按状态动态生成。
func (g *grammar) compileRegExp(str string, handleSn bool) (*matcher, error) {
	str, err := g.expandAttrs(str)
	if err != nil {
		return nil, err
	}
	str = approxXID(str)
	m := &matcher{g: g, src: str, flags: g.flags()}
	if handleSn && reStateRef.MatchString(str) {
		m.dynamic = true
		m.cache = map[string]*compiledRe{}
		return m, nil
	}
	m.static = g.newCompiledRe(str, m.flags)
	return m, nil
}

// matcher 是一条规则的正则；动态（含 $Sn）时按状态名缓存编译结果。
type matcher struct {
	g       *grammar
	src     string
	flags   string
	dynamic bool
	static  *compiledRe
	mu      sync.Mutex
	cache   map[string]*compiledRe
}

type compiledRe struct {
	src, flags string
	re         regex.Regexp // 锚定在开头；编译失败为 nil（永不匹配）
	searchOnce sync.Once
	search     *regexp.Regexp // 非锚定搜索；不能翻译成 RE2 时为 nil，退回逐位置 MatchStart
}

func (g *grammar) newCompiledRe(src, flags string) *compiledRe {
	re, err := regex.Compile(src, flags)
	if err != nil {
		g.warnings = append(g.warnings, fmt.Sprintf("regex %q: %v", src, err))
		re = nil
	}
	return &compiledRe{src: src, flags: flags, re: re}
}

func (m *matcher) resolve(state string) *compiledRe {
	if !m.dynamic {
		return m.static
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.cache[state]; ok {
		return c
	}
	src := substituteRe(m.g, m.src, state)
	re, err := regex.Compile(src, m.flags)
	if err != nil {
		re = nil
	}
	c := &compiledRe{src: src, flags: m.flags, re: re}
	if len(m.cache) > 1024 { // 状态名可由输入决定，防止无界增长
		clear(m.cache)
	}
	m.cache[state] = c
	return c
}

// searchIndex 对应 JS 的 line.search(re)：第一个匹配的起点（字节），无匹配 -1。
// 逐位置回退匹配时受行预算约束：超时或超预算返回 errDegraded（off 为 line 在整行中的起点）。
func (c *compiledRe) searchIndex(line string, off int, b *lineBudget) (int, error) {
	// The optimized RE2 search bypasses match; honor the same line latch.
	if b.exhausted {
		return 0, b.stop(off)
	}
	if c.re == nil {
		return -1, nil
	}
	c.searchOnce.Do(func() {
		if s, ok := regex.Translate(c.src, c.flags); ok {
			c.search, _ = regexp.Compile(s)
		}
	})
	if c.search != nil {
		if loc := c.search.FindStringIndex(line); loc != nil {
			return loc[0], nil
		}
		return -1, nil
	}
	rs := []rune(line) // 整行转换一次，逐位置复用子切片
	for i, ri := 0, 0; ; ri++ {
		m, err := b.match(c.re, line[i:], rs[ri:], off+i)
		if err != nil {
			return 0, err
		}
		if m != nil {
			return i, nil
		}
		if err := b.check(off+i, c.re.Engine() == regex.Backtrack); err != nil {
			return 0, err
		}
		if i >= len(line) {
			return -1, nil
		}
		_, size := decodeRune(line[i:])
		i += size
	}
}

// substituteRe 对应 substituteMatchesRe：把 $Sn 换成状态名的第 n 段（正则转义）。
func substituteRe(g *grammar, str, state string) string {
	parts := stateParts(state)
	return reStateRef.ReplaceAllStringFunc(str, func(s string) string {
		n, _ := strconv.Atoi(s[2:])
		if n < len(parts) {
			return escapeRegExp(g.fixCase(parts[n]))
		}
		return ""
	})
}

func stateParts(state string) []string {
	return append([]string{state}, strings.Split(state, ".")...)
}

func escapeRegExp(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(`\{}*+?|^$.[]()`, s[i]) >= 0 {
			sb.WriteByte('\\')
		}
		sb.WriteByte(s[i])
	}
	return sb.String()
}

func (g *grammar) fixCase(s string) string {
	if g.ignoreCase {
		return strings.ToLower(s)
	}
	return s
}

// approxXID 把 \p{XID_Start}/\p{XID_Continue}（两种引擎都不支持；wgsl 使用）换成近似集合：
// XID_Start ≈ [\p{L}\p{Nl}_]，XID_Continue ≈ [\p{L}\p{Nl}\p{Mn}\p{Mc}\p{Nd}\p{Pc}]。
// 与 Unicode 定义的差异只在 Other_ID_Start/Other_ID_Continue 等少数码点及 NFKC 例外。
func approxXID(s string) string {
	if !strings.Contains(s, "XID_") {
		return s
	}
	sets := map[string]string{
		`\p{XID_Start}`:    `\p{L}\p{Nl}_`,
		`\p{XID_Continue}`: `\p{L}\p{Nl}\p{Mn}\p{Mc}\p{Nd}\p{Pc}`,
	}
	var sb strings.Builder
	inClass := false
	for i := 0; i < len(s); {
		if !inClass || s[i] == '\\' {
			replaced := false
			for k, v := range sets {
				if strings.HasPrefix(s[i:], k) {
					if inClass {
						sb.WriteString(v)
					} else {
						sb.WriteString("[" + v + "]")
					}
					i += len(k)
					replaced = true
					break
				}
			}
			if replaced {
				continue
			}
		}
		c := s[i]
		switch {
		case c == '\\' && i+1 < len(s):
			sb.WriteString(s[i : i+2])
			i += 2
			continue
		case c == '[' && !inClass:
			inClass = true
		case c == ']' && inClass:
			inClass = false
		}
		sb.WriteByte(c)
		i++
	}
	return sb.String()
}

// compileAction 对应 Monaco 的 compileAction。
func (g *grammar) compileAction(ruleName string, n *node) (*action, error) {
	if !n.truthy() {
		return &action{kind: actToken}, nil
	}
	switch n.kind {
	case kString:
		return &action{kind: actString, token: n.str}, nil
	case kArray:
		a := &action{kind: actGroup}
		for _, e := range n.arr {
			sub, err := g.compileAction(ruleName, e)
			if err != nil {
				return nil, err
			}
			a.group = append(a.group, sub)
		}
		return a, nil
	case kObject:
	default:
		return nil, g.errorf("an action must be a string, an object with a 'token' or 'cases' attribute, or an array of actions; in rule: %s", ruleName)
	}
	if tok := n.get("token"); tok.truthy() || tok.isString() {
		if !tok.isString() {
			return nil, g.errorf("a 'token' attribute must be of type string, in rule: %s", ruleName)
		}
		a := &action{kind: actToken, token: tok.str, tokenSubst: strings.Contains(tok.str, "$")}
		if b := n.get("bracket"); b.isString() && b.str != "@open" && b.str != "@close" {
			return nil, g.errorf("a 'bracket' attribute must be either '@open' or '@close', in rule: %s", ruleName)
		}
		if nx := n.get("next"); nx.truthy() {
			if !nx.isString() {
				return nil, g.errorf("the next state must be a string value in rule: %s", ruleName)
			}
			next := nx.str
			if next != "@pop" && next != "@push" && next != "@popall" {
				next = strings.TrimPrefix(next, "@")
				if !strings.Contains(next, "$") && !g.stateExists(next) {
					return nil, g.errorf("the next state '%s' is not defined in rule: %s", nx.str, ruleName)
				}
			}
			a.next = next
		}
		if gb := n.get("goBack"); gb != nil && gb.kind == kNumber && gb.num >= 1 {
			// 超范围 float→int 的结果由实现决定；回退量运行时还会被限制在本次匹配内
			a.goBack = int(min(gb.num, 1<<30))
		}
		if s := n.get("switchTo"); s.isString() {
			a.switchTo = s.str
		}
		if s := n.get("nextEmbedded"); s.isString() {
			a.nextEmbed = s.str
		}
		return a, nil
	}
	cs := n.get("cases")
	if !cs.truthy() {
		return nil, g.errorf("an action must be a string, an object with a 'token' or 'cases' attribute, or an array of actions; in rule: %s", ruleName)
	}
	a := &action{kind: actCases}
	keys, vals := cs.keys, cs.vals
	if cs.kind == kArray {
		keys, vals = nil, cs.arr
		for i := range cs.arr {
			keys = append(keys, strconv.Itoa(i))
		}
	}
	for i, key := range keys {
		val, err := g.compileAction(ruleName, vals[i])
		if err != nil {
			return nil, err
		}
		var arm caseArm
		switch key {
		case "@default", "@", "":
			arm = caseArm{value: val}
		case "@eos":
			arm = caseArm{test: func(c *matchCtx) bool { return c.eos }, value: val}
		default:
			test, err := g.createGuard(ruleName, key)
			if err != nil {
				return nil, err
			}
			arm = caseArm{test: test, value: val}
		}
		a.cases = append(a.cases, arm)
		if !a.hasEmbeddedEndInCases && val.kind != actString {
			a.hasEmbeddedEndInCases = val.hasEmbeddedEndInCases || val.nextEmbed == "@pop" || val.nextEmbed == "@popall"
		}
	}
	return a, nil
}

func (g *grammar) stateExists(state string) bool {
	for state != "" {
		if g.stateNames[state] {
			return true
		}
		i := strings.LastIndexByte(state, '.')
		if i < 0 {
			return false
		}
		state = state[:i]
	}
	return false
}

var (
	reGuardKey  = regexp.MustCompile(`^\$(([sS]?)(\d\d?)|#)(.*)$`)
	reWord      = regexp.MustCompile(`^[0-9A-Za-z_]*$`)
	reGuardOp   = regexp.MustCompile(`^(@|!@|~|!~|==|!=)(.*)$`)
	reWordsAlts = regexp.MustCompile(`^[0-9A-Za-z_|]*$`)
)

func (g *grammar) keywordSet(words []string) map[string]struct{} {
	set := make(map[string]struct{}, len(words))
	for _, w := range words {
		set[g.fixCase(w)] = struct{}{}
	}
	return set
}

// createGuard 对应 Monaco 的 createGuard：解析 "$n op pattern" 形式的 cases 键。
func (g *grammar) createGuard(ruleName, key string) (func(*matchCtx) bool, error) {
	scrut := -1 // -1: 整个匹配；0-99: $n；100+n: $Sn
	oppat := key
	if m := reGuardKey.FindStringSubmatch(key); m != nil {
		if m[3] != "" {
			scrut, _ = strconv.Atoi(m[3])
			if m[2] != "" {
				scrut += 100
			}
		}
		oppat = m[4]
	}
	op, pat := "~", oppat
	switch {
	case oppat == "":
		op, pat = "!=", ""
	case reWord.MatchString(pat):
		op = "=="
	default:
		if m := reGuardOp.FindStringSubmatch(oppat); m != nil {
			op, pat = m[1], m[2]
		}
	}
	var tester func(s string, c *matchCtx) bool
	switch {
	case (op == "~" || op == "!~") && reWordsAlts.MatchString(pat):
		set := g.keywordSet(strings.Split(pat, "|"))
		want := op == "~"
		tester = func(s string, _ *matchCtx) bool { _, ok := set[g.fixCase(s)]; return ok == want }
	case op == "@" || op == "!@":
		words := g.src.get(pat)
		if words == nil {
			return nil, g.errorf("the @ match target '%s' is not defined, in rule: %s", pat, ruleName)
		}
		list, ok := stringArray(words)
		if !ok {
			return nil, g.errorf("the @ match target '%s' must be an array of strings, in rule: %s", pat, ruleName)
		}
		set := g.keywordSet(list)
		want := op == "@"
		tester = func(s string, _ *matchCtx) bool { _, ok := set[g.fixCase(s)]; return ok == want }
	case op == "~" || op == "!~":
		if !strings.Contains(pat, "$") {
			m, err := g.compileRegExp("^"+pat+"$", false)
			if err != nil {
				return nil, err
			}
			t, err := newTestRe(m.src, m.flags)
			if err != nil {
				g.warnings = append(g.warnings, fmt.Sprintf("guard regex %q: %v", m.src, err))
			}
			want := op == "~"
			tester = func(s string, c *matchCtx) bool { return t.test(s, c) == want }
		} else {
			// Monaco 的动态分支忽略 !~，总是返回 re.test(s)。
			var mu sync.Mutex
			cache := map[string]*testRe{}
			// 用 defer 解锁：TokenizeLine 会 recover panic，锁不能因此一直被持有
			lookup := func(src string) (*testRe, bool) {
				mu.Lock()
				defer mu.Unlock()
				if t, ok := cache[src]; ok {
					return t, true
				}
				expanded, err := g.expandAttrs(src)
				if err != nil {
					return nil, false
				}
				t, _ := newTestRe(approxXID(expanded), g.flags())
				if len(cache) > 1024 {
					clear(cache)
				}
				cache[src] = t
				return t, true
			}
			tester = func(s string, c *matchCtx) bool {
				t, ok := lookup("^" + g.substitute(pat, c, g.attrString) + "$")
				if !ok {
					return false
				}
				return t.test(s, c)
			}
		}
	default: // == / !=
		patx := g.fixCase(pat)
		want := op == "=="
		if !strings.Contains(pat, "$") {
			tester = func(s string, _ *matchCtx) bool { return (s == patx) == want }
		} else {
			tester = func(s string, c *matchCtx) bool { return (s == g.substitute(patx, c, g.attrString)) == want }
		}
	}
	if scrut == -1 {
		return func(c *matchCtx) bool { return tester(c.matched, c) }, nil
	}
	return func(c *matchCtx) bool { return tester(selectScrutinee(c, scrut), c) }, nil
}

func stringArray(n *node) ([]string, bool) {
	if n.kind != kArray {
		return nil, false
	}
	out := make([]string, len(n.arr))
	for i, e := range n.arr {
		if !e.isString() {
			return nil, false
		}
		out[i] = e.str
	}
	return out, true
}

func selectScrutinee(c *matchCtx, num int) string {
	if num < len(c.matches) {
		return c.matches[num].s // 未参与的组为 ""
	}
	if num >= 100 {
		parts := stateParts(c.state)
		if num-100 < len(parts) {
			return parts[num-100]
		}
	}
	return ""
}

// testRe 对应 JS 的 re.test(s)（非锚定搜索）：编译 [\s\S]*?(?:src)，用锚定匹配实现搜索。
type testRe struct{ re regex.Regexp }

func newTestRe(src, flags string) (*testRe, error) {
	re, err := regex.Compile(`[\s\S]*?(?:`+src+`)`, flags)
	if err != nil {
		return &testRe{}, err
	}
	return &testRe{re: re}, nil
}

// test 执行 re.test(s)；回溯超时记入 c.timedOut（c 可为 nil）并按不匹配返回。
func (t *testRe) test(s string, c *matchCtx) bool {
	if c.stopped() {
		return false
	}
	if t.re == nil {
		return false
	}
	var m []int
	var err error
	if c != nil && c.budget != nil {
		// Include successful, cumulatively expensive guard matches too. The
		// caller supplies the actual token offset when c.timedOut is observed.
		m, err = c.budget.match(t.re, s, nil, 0)
	} else {
		m, err = t.re.TryMatchStart(s, nil)
	}
	if err != nil && c != nil {
		c.timedOut = true
	}
	return m != nil
}

// attrString 是编译期属性（语言定义）中的字符串属性，用于 cases 里 $@attr 的替换。
func (g *grammar) attrString(name string) (string, bool) {
	a := g.attr(name)
	if a.isString() {
		return a.str, true
	}
	return "", false
}

// lexerAttrString 是运行期 Monaco ILexer 上的字符串属性，用于 token/next 等里的 $@attr。
func (g *grammar) lexerAttrString(name string) (string, bool) {
	switch name {
	case "languageId":
		return g.id, true
	case "start":
		return g.start, true
	case "tokenPostfix":
		return g.tokenPostfix, true
	case "defaultToken":
		return g.defaultToken, true
	}
	return "", false
}

// substitute 对应 substituteMatches：$$、$#、$n、$Sn、$@attr。
func (g *grammar) substitute(str string, c *matchCtx, attr func(string) (string, bool)) string {
	if !strings.Contains(str, "$") {
		return str
	}
	var sb strings.Builder
	var parts []string
	for i := 0; i < len(str); {
		if str[i] != '$' || i+1 >= len(str) {
			sb.WriteByte(str[i])
			i++
			continue
		}
		c1 := str[i+1]
		switch {
		case c1 == '$':
			sb.WriteByte('$')
			i += 2
			continue
		case c1 == '#':
			sb.WriteString(g.fixCase(c.matched))
			i += 2
			continue
		case isDigit(c1):
			j := i + 2
			if j < len(str) && isDigit(str[j]) {
				j++
			}
			n, _ := strconv.Atoi(str[i+1 : j])
			if n < len(c.matches) {
				if c.matches[n].ok {
					sb.WriteString(g.fixCase(c.matches[n].s))
				} else {
					sb.WriteString("undefined") // JS 把 undefined 写成字符串
				}
			}
			i = j
			continue
		case (c1 == 's' || c1 == 'S') && i+2 < len(str) && isDigit(str[i+2]):
			j := i + 3
			if j < len(str) && isDigit(str[j]) {
				j++
			}
			n, _ := strconv.Atoi(str[i+2 : j])
			if parts == nil {
				parts = stateParts(c.state)
			}
			if n < len(parts) {
				sb.WriteString(g.fixCase(parts[n]))
			}
			i = j
			continue
		case c1 == '@' && i+2 < len(str) && isWordByte(str[i+2]):
			j := i + 2
			for j < len(str) && isWordByte(str[j]) {
				j++
			}
			if v, ok := attr(str[i+2 : j]); ok {
				sb.WriteString(v)
			}
			i = j
			continue
		}
		sb.WriteByte('$')
		i++
	}
	return sb.String()
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func (g *grammar) compileBrackets() error {
	bn := g.src.get("brackets")
	var list []*node
	if bn != nil && bn.kind != kNull {
		if bn.kind != kArray {
			return g.errorf("the 'brackets' attribute must be defined as an array")
		}
		list = bn.arr
	} else {
		g.brackets = []bracket{
			{"{", "}", "delimiter.curly" + g.tokenPostfix},
			{"[", "]", "delimiter.square" + g.tokenPostfix},
			{"(", ")", "delimiter.parenthesis" + g.tokenPostfix},
			{"<", ">", "delimiter.angle" + g.tokenPostfix},
		}
		return nil
	}
	for _, el := range list {
		var open, close, token *node
		if el.kind == kArray && len(el.arr) == 3 {
			open, close, token = el.arr[0], el.arr[1], el.arr[2]
		} else {
			open, close, token = el.get("open"), el.get("close"), el.get("token")
		}
		if !open.isString() || !close.isString() || !token.isString() {
			return g.errorf("every element in the 'brackets' array must be a '{open,close,token}' object or array")
		}
		if open.str == close.str {
			return g.errorf("open and close brackets in a 'brackets' attribute must be different: %s", open.str)
		}
		g.brackets = append(g.brackets, bracket{g.fixCase(open.str), g.fixCase(close.str), token.str + g.tokenPostfix})
	}
	return nil
}

func (g *grammar) findBracket(matched string) *bracket {
	if matched == "" {
		return nil
	}
	matched = g.fixCase(matched)
	for i := range g.brackets {
		if g.brackets[i].open == matched || g.brackets[i].close == matched {
			return &g.brackets[i]
		}
	}
	return nil
}

// findRules 按点分前缀查找状态的规则（"a.b.c" → "a.b" → "a"）；ok=false 表示状态未定义。
func (g *grammar) findRules(state string) (rules []*rule, ok bool) {
	for state != "" {
		if r, ok := g.tokenizer[state]; ok {
			return r, true
		}
		i := strings.LastIndexByte(state, '.')
		if i < 0 {
			return nil, false
		}
		state = state[:i]
	}
	return nil, false
}
