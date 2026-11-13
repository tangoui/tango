package regex

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// target 是翻译产物要喂给的引擎。
type target int

const (
	toRE2       target = iota // Go regexp（RE2 语法）
	toBacktrack               // regexp2，ECMAScript 模式
)

// errNeedBacktrack 表示该 JS 结构在 RE2 中无法等价表达（前瞻、后顾、反向引用……）。
var errNeedBacktrack = errors.New("regex: construct needs backtracking engine")

// jsFlags 是 JS RegExp 标志中影响语义的部分。g、y 也被接受，但对锚定在开头的单次匹配是空操作。
type jsFlags struct{ i, m, s, u bool }

func parseFlags(flags string) (jsFlags, error) {
	var fl jsFlags
	var g, y bool // 接受但忽略：MatchStart 总是从开头匹配一次
	for _, c := range flags {
		var p *bool
		switch c {
		case 'g':
			p = &g
		case 'y':
			p = &y
		case 'i':
			p = &fl.i
		case 'm':
			p = &fl.m
		case 's':
			p = &fl.s
		case 'u':
			p = &fl.u
		default:
			return fl, fmt.Errorf("regex: unsupported flag %q", c)
		}
		if *p {
			return fl, fmt.Errorf("regex: duplicate flag %q", c)
		}
		*p = true
	}
	return fl, nil
}

// JS 的 \s 集合（WhiteSpace ∪ LineTerminator），按 RE2 字符类内部写法给出，以及其补集。
// regexp2 的 ECMAScript 模式下 \s/\S 本身就是这个集合，因此只有 RE2 需要展开。
const (
	jsSpaceRE2    = `\t\n\v\f\r \x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}`
	jsNonSpaceRE2 = `\x{0}-\x{8}\x{e}-\x{1f}\x{21}-\x{9f}\x{a1}-\x{167f}\x{1681}-\x{1fff}\x{200b}-\x{2027}\x{202a}-\x{202e}\x{2030}-\x{205e}\x{2060}-\x{2fff}\x{3001}-\x{fefe}\x{ff00}-\x{10ffff}`
)

// Translate 把 JS 正则源码翻译为语义相同的 Go RE2 语法。flags 同 Compile。ok=false 表示无法等价
// （前瞻、后顾、反向引用、多行模式下的 ^/$、超过 RE2 重复上限等），或标志/语法无效。
// 产物在 i 标志下以 (?i) 开头，不含锚定；Compile 会再包一层 ^(?:…)。
func Translate(source, flags string) (goSyntax string, ok bool) {
	fl, err := parseFlags(flags)
	if err != nil {
		return "", false
	}
	out, _, err := translate(source, fl, toRE2)
	if err != nil {
		return "", false
	}
	if _, err := regexp.Compile(anchorStd(out)); err != nil {
		return "", false
	}
	return out, true
}

func anchorStd(s string) string       { return `^(?:` + s + `)` }
func anchorBacktrack(s string) string { return `\A(?:` + s + `)` }

// translate 把 JS 源码改写成目标引擎的语法，同时返回按 JS 出现顺序排列的捕获组名（未命名为 ""）。
func translate(source string, fl jsFlags, tgt target) (string, []string, error) {
	t := &translator{src: []rune(source), fl: fl, tgt: tgt}
	t.groups = scanGroups(t.src)
	for _, g := range t.groups {
		if g != "" {
			t.named = true
		}
	}
	if err := t.run(); err != nil {
		return "", nil, err
	}
	return t.b.String(), t.groups, nil
}

type translator struct {
	src    []rune
	pos    int
	fl     jsFlags
	tgt    target
	b      strings.Builder
	groups []string
	named  bool
}

func (t *translator) syntaxErr(msg string) error {
	return fmt.Errorf("regex: invalid pattern %q: %s", string(t.src), msg)
}

func (t *translator) peek(off int) rune {
	if t.pos+off < len(t.src) {
		return t.src[t.pos+off]
	}
	return -1
}

// scanGroups 预扫捕获组，用于区分反向引用与旧式八进制转义，以及组号映射。
func scanGroups(src []rune) []string {
	var names []string
	inClass := false
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case c == '\\':
			i++
		case inClass:
			if c == ']' {
				inClass = false
			}
		case c == '[':
			inClass = true
		case c == '(':
			if i+1 < len(src) && src[i+1] == '?' {
				if i+3 < len(src) && src[i+2] == '<' && src[i+3] != '=' && src[i+3] != '!' {
					j := i + 3
					for j < len(src) && src[j] != '>' {
						j++
					}
					names = append(names, string(src[i+3:j]))
				}
			} else {
				names = append(names, "")
			}
		}
	}
	return names
}

func (t *translator) run() error {
	if t.fl.i && t.tgt == toRE2 {
		t.b.WriteString("(?i)")
	}
	for t.pos < len(t.src) {
		c := t.src[t.pos]
		t.pos++
		var err error
		switch c {
		case '\\':
			err = t.escape()
		case '[':
			err = t.class()
		case '(':
			err = t.group()
		case '.':
			t.dot()
		case '^', '$':
			err = t.anchor(c)
		case '{':
			if q, ok := t.quantifier(); ok {
				t.b.WriteString("{" + q)
			} else {
				t.b.WriteString(`\{`)
			}
		case '}', ']':
			t.b.WriteString(`\` + string(c))
		default: // ) | * + ? 与普通字符在两种语法里含义相同
			t.b.WriteRune(c)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// quantifier 在 '{' 之后识别 {n}、{n,}、{n,m}；不是合法量词时 JS（非 u）把 '{' 当字面量。
func (t *translator) quantifier() (string, bool) {
	i := t.pos
	digits := func() int {
		start := i
		for i < len(t.src) && isDigit(t.src[i]) {
			i++
		}
		return i - start
	}
	if digits() == 0 {
		return "", false
	}
	if i < len(t.src) && t.src[i] == ',' {
		i++
		digits()
	}
	if i >= len(t.src) || t.src[i] != '}' {
		return "", false
	}
	q := string(t.src[t.pos : i+1])
	t.pos = i + 1
	return q, true
}

func (t *translator) dot() {
	switch {
	case t.fl.s && t.tgt == toRE2:
		t.b.WriteString(`(?s:.)`)
	case t.fl.s:
		t.b.WriteString(`[\s\S]`)
	case t.tgt == toRE2:
		t.b.WriteString(`[^\n\r\x{2028}\x{2029}]`)
	default:
		t.b.WriteString(`[^\n\r\u2028\u2029]`)
	}
}

func (t *translator) anchor(c rune) error {
	if t.fl.m {
		// JS 多行模式的行终止符还包括 \r、U+2028、U+2029；RE2 的 (?m) 只认 \n。
		if t.tgt == toRE2 {
			return errNeedBacktrack
		}
		if c == '^' {
			t.b.WriteString(`(?<![^\n\r\u2028\u2029])`)
		} else {
			t.b.WriteString(`(?![^\n\r\u2028\u2029])`)
		}
		return nil
	}
	if c == '$' && t.tgt == toBacktrack {
		t.b.WriteString(`\z`) // regexp2 的 $ 会匹配末尾换行之前，JS 不会
		return nil
	}
	t.b.WriteRune(c)
	return nil
}

func (t *translator) group() error {
	if t.peek(0) != '?' {
		t.b.WriteByte('(')
		return nil
	}
	switch t.peek(1) {
	case ':':
		t.pos += 2
		t.b.WriteString("(?:")
		return nil
	case '=', '!':
		return t.lookaround(2)
	case '<':
		if p := t.peek(2); p == '=' || p == '!' {
			return t.lookaround(3)
		}
		end := t.pos + 2
		for end < len(t.src) && t.src[end] != '>' {
			end++
		}
		if end >= len(t.src) || end == t.pos+2 {
			return t.syntaxErr("invalid group name")
		}
		t.b.WriteString("(?<" + string(t.src[t.pos+2:end]) + ">")
		t.pos = end + 1
		return nil
	}
	// (?ims-ims:…) 修饰符组：会改变 . 等已被显式展开的结构的含义，不支持。
	return t.syntaxErr("unsupported group syntax")
}

func (t *translator) lookaround(n int) error {
	if t.tgt == toRE2 {
		return errNeedBacktrack
	}
	t.b.WriteString("(" + string(t.src[t.pos:t.pos+n]))
	t.pos += n
	return nil
}

// hexCP 以转义形式写一个码点（来自 \x、\u、\c、八进制等数值转义）。
func (t *translator) hexCP(r rune) string {
	if t.tgt == toRE2 {
		return fmt.Sprintf(`\x{%x}`, r)
	}
	if r > 0xFFFF {
		return string(r) // regexp2 按 rune 匹配；增补平面字符不会是元字符
	}
	return fmt.Sprintf(`\u%04x`, r)
}

func (t *translator) spaceSet(neg, inClass bool) string {
	if t.tgt == toBacktrack {
		if neg {
			return `\S`
		}
		return `\s`
	}
	switch {
	case inClass && neg:
		return jsNonSpaceRE2
	case inClass:
		return jsSpaceRE2
	case neg:
		return `[^` + jsSpaceRE2 + `]`
	default:
		return `[` + jsSpaceRE2 + `]`
	}
}

// identityText 写恒等转义 \c 对应的字面字符：正则元字符与 /、- 保留反斜杠，其余写字符本身
// （JS 非 u 模式下 \a、\e 等就是字母本身，而 RE2 里 \a 是响铃）。
func identityText(c rune) string {
	if strings.ContainsRune(`\.+*?()|[]{}^$/-`, c) {
		return `\` + string(c)
	}
	return string(c)
}

// escape 处理字符类外部的 '\' 转义（t.pos 指向 '\' 之后）。
func (t *translator) escape() error {
	if t.pos >= len(t.src) {
		return t.syntaxErr("trailing backslash")
	}
	c := t.src[t.pos]
	t.pos++
	switch c {
	case 'd', 'D', 'w', 'W', 'b', 'B', 'n', 'r', 't', 'v', 'f':
		t.b.WriteString(`\` + string(c))
	case 's', 'S':
		t.b.WriteString(t.spaceSet(c == 'S', false))
	case 'c':
		if r, ok := t.controlEscape(false); ok {
			t.b.WriteString(t.hexCP(r))
		} else {
			t.b.WriteString(`\\`) // \c 后不是字母：'\' 是字面量，'c' 照常解析
			t.pos--
		}
	case 'x':
		if r, ok := t.hexEscape(2); ok {
			t.b.WriteString(t.hexCP(r))
		} else {
			t.b.WriteByte('x')
		}
	case 'u':
		if r, ok := t.unicodeEscape(); ok {
			t.b.WriteString(t.hexCP(r))
		} else {
			t.b.WriteByte('u')
		}
	case 'k':
		if !t.named && !t.fl.u {
			t.b.WriteByte('k')
			return nil
		}
		end := t.pos
		for end < len(t.src) && t.src[end] != '>' {
			end++
		}
		if t.peek(0) != '<' || end >= len(t.src) {
			return t.syntaxErr(`invalid \k`)
		}
		if t.tgt == toRE2 {
			return errNeedBacktrack
		}
		t.b.WriteString(`\k` + string(t.src[t.pos:end+1]))
		t.pos = end + 1
	case 'p', 'P':
		if !t.fl.u {
			t.b.WriteRune(c)
			return nil
		}
		s, err := t.property(c)
		if err != nil {
			return err
		}
		t.b.WriteString(s)
	default:
		if isDigit(c) {
			return t.decimalEscape(c)
		}
		t.b.WriteString(identityText(c))
	}
	return nil
}

// decimalEscape：\N 在 N 不超过捕获组数时是反向引用，否则（Annex B）是旧式八进制或数字本身。
func (t *translator) decimalEscape(c rune) error {
	end := t.pos
	for end < len(t.src) && isDigit(t.src[end]) {
		end++
	}
	n, err := strconv.Atoi(string(t.src[t.pos-1 : end]))
	if c != '0' && err == nil && n <= len(t.groups) {
		if t.tgt == toRE2 {
			return errNeedBacktrack
		}
		t.pos = end
		// regexp2 先给未命名组编号、再给命名组编号，因此按名字或未命名序号引用。
		if name := t.groups[n-1]; name != "" {
			t.b.WriteString(`\k<` + name + `>`)
		} else {
			k := 0
			for _, g := range t.groups[:n] {
				if g == "" {
					k++
				}
			}
			t.b.WriteString(`\` + strconv.Itoa(k) + `(?:)`)
		}
		return nil
	}
	if r, ok := t.legacyOctal(c); ok {
		t.b.WriteString(t.hexCP(r))
	} else {
		t.b.WriteRune(c)
	}
	return nil
}

// legacyOctal 解析 Annex B 的旧式八进制（t.pos 在首位数字之后）；8、9 返回 ok=false。
func (t *translator) legacyOctal(c rune) (rune, bool) {
	if c > '7' {
		return 0, false
	}
	v := c - '0'
	if isOctal(t.peek(0)) {
		v = v*8 + t.src[t.pos] - '0'
		t.pos++
		if c <= '3' && isOctal(t.peek(0)) {
			v = v*8 + t.src[t.pos] - '0'
			t.pos++
		}
	}
	return v, true
}

// controlEscape 解析 \cX 的 X（t.pos 在 'c' 之后）；字符类内部还允许数字与 '_'（Annex B）。
func (t *translator) controlEscape(inClass bool) (rune, bool) {
	x := t.peek(0)
	if isLetter(x) || (inClass && (isDigit(x) || x == '_')) {
		t.pos++
		return x % 32, true
	}
	return 0, false
}

func (t *translator) hexEscape(n int) (rune, bool) {
	if t.pos+n > len(t.src) {
		return 0, false
	}
	v, err := strconv.ParseUint(string(t.src[t.pos:t.pos+n]), 16, 32)
	if err != nil || strings.ContainsAny(string(t.src[t.pos:t.pos+n]), "+-") {
		return 0, false
	}
	t.pos += n
	return rune(v), true
}

// unicodeEscape 解析 \uXXXX、代理对 😀，以及 u 标志下的 \u{…}（t.pos 在 'u' 之后）。
func (t *translator) unicodeEscape() (rune, bool) {
	if t.fl.u && t.peek(0) == '{' {
		end := t.pos + 1
		for end < len(t.src) && t.src[end] != '}' {
			end++
		}
		if end < len(t.src) && end > t.pos+1 {
			v, err := strconv.ParseUint(string(t.src[t.pos+1:end]), 16, 32)
			if err == nil && v <= 0x10FFFF {
				t.pos = end + 1
				return rune(v), true
			}
		}
		return 0, false
	}
	hi, ok := t.hexEscape(4)
	if !ok {
		return 0, false
	}
	if hi >= 0xD800 && hi <= 0xDBFF && t.peek(0) == '\\' && t.peek(1) == 'u' {
		save := t.pos
		t.pos += 2
		if lo, ok := t.hexEscape(4); ok && lo >= 0xDC00 && lo <= 0xDFFF {
			return 0x10000 + (hi-0xD800)<<10 + (lo - 0xDC00), true
		}
		t.pos = save
	}
	return hi, true
}

// property 解析 u 标志下的 \p{…}/\P{…}（t.pos 在 'p'/'P' 之后）。
func (t *translator) property(c rune) (string, error) {
	if t.peek(0) != '{' {
		return "", t.syntaxErr(`invalid \p`)
	}
	end := t.pos + 1
	for end < len(t.src) && t.src[end] != '}' {
		end++
	}
	if end >= len(t.src) {
		return "", t.syntaxErr(`invalid \p`)
	}
	name := string(t.src[t.pos+1 : end])
	t.pos = end + 1
	if k, v, ok := strings.Cut(name, "="); ok {
		switch k {
		case "Script", "sc", "General_Category", "gc":
			name = v
		default: // Script_Extensions 等 RE2 没有对应物
			if t.tgt == toRE2 {
				return "", errNeedBacktrack
			}
		}
	}
	return `\` + string(c) + `{` + name + `}`, nil
}

// classItem 是字符类中的一个原子：单个字符（可作范围端点）或一个集合。
type classItem struct {
	text    string
	cp      rune
	isChar  bool
	rawDash bool // 未转义的 '-'，可能是范围运算符
}

// class 处理 [...]（t.pos 在 '[' 之后）。
func (t *translator) class() error {
	negate := false
	if t.peek(0) == '^' {
		negate = true
		t.pos++
	}
	var items []classItem
	closed := false
	for t.pos < len(t.src) {
		c := t.src[t.pos]
		t.pos++
		if c == ']' {
			closed = true
			break
		}
		if c == '\\' {
			it, err := t.classEscape()
			if err != nil {
				return err
			}
			items = append(items, it)
			continue
		}
		text := string(c)
		if strings.ContainsRune(`[]^-`, c) {
			text = `\` + text
		}
		items = append(items, classItem{text: text, cp: c, isChar: true, rawDash: c == '-'})
	}
	if !closed {
		return t.syntaxErr("unterminated character class")
	}
	if len(items) == 0 {
		switch {
		case negate && t.tgt == toRE2: // [^] 匹配任意字符
			t.b.WriteString(`[` + jsSpaceRE2 + jsNonSpaceRE2 + `]`)
		case negate:
			t.b.WriteString(`[\s\S]`)
		case t.tgt == toRE2: // [] 不匹配任何字符
			t.b.WriteString(`[^\x{0}-\x{10ffff}]`)
		default:
			t.b.WriteString(`(?!)`)
		}
		return nil
	}
	t.b.WriteByte('[')
	if negate {
		t.b.WriteByte('^')
	}
	for i := 0; i < len(items); {
		a := items[i]
		if a.isChar && i+2 < len(items) && items[i+1].rawDash && items[i+2].isChar {
			z := items[i+2]
			if a.cp > z.cp {
				return t.syntaxErr("range out of order in character class")
			}
			t.b.WriteString(a.text + "-" + z.text)
			i += 3
			continue
		}
		t.b.WriteString(a.text) // 与集合相邻的 '-' 是字面量（Annex B），已写成 \-
		i++
	}
	t.b.WriteByte(']')
	return nil
}

func (t *translator) classEscape() (classItem, error) {
	if t.pos >= len(t.src) {
		return classItem{}, t.syntaxErr("trailing backslash")
	}
	c := t.src[t.pos]
	t.pos++
	char := func(r rune) classItem { return classItem{text: t.hexCP(r), cp: r, isChar: true} }
	switch c {
	case 'd', 'D', 'w', 'W':
		return classItem{text: `\` + string(c)}, nil
	case 's', 'S':
		return classItem{text: t.spaceSet(c == 'S', true)}, nil
	case 'b':
		return char(8), nil
	case 'n', 'r', 't', 'v', 'f':
		cp := map[rune]rune{'n': '\n', 'r': '\r', 't': '\t', 'v': '\v', 'f': '\f'}[c]
		return classItem{text: `\` + string(c), cp: cp, isChar: true}, nil
	case 'c':
		if r, ok := t.controlEscape(true); ok {
			return char(r), nil
		}
		t.pos-- // '\' 是字面量，'c' 留给下一个原子
		return classItem{text: `\\`, cp: '\\', isChar: true}, nil
	case 'x':
		if r, ok := t.hexEscape(2); ok {
			return char(r), nil
		}
	case 'u':
		if r, ok := t.unicodeEscape(); ok {
			return char(r), nil
		}
	case 'p', 'P':
		if t.fl.u {
			s, err := t.property(c)
			return classItem{text: s}, err
		}
	default:
		if isDigit(c) {
			if r, ok := t.legacyOctal(c); ok {
				return char(r), nil
			}
		}
	}
	return classItem{text: identityText(c), cp: c, isChar: true}, nil
}

func isDigit(r rune) bool  { return r >= '0' && r <= '9' }
func isOctal(r rune) bool  { return r >= '0' && r <= '7' }
func isLetter(r rune) bool { return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' }
