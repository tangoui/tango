// Package when parses and evaluates VS Code "when" clause expressions, the
// conditions keybindings and commands use to decide whether they apply
// ("editorTextFocus && !editorReadonly").
//
// Grammar (VS Code's, lowest precedence first):
//
//	expr    = and { "||" and }
//	and     = unary { "&&" unary }
//	unary   = "!" term | primary
//	term    = key | "true" | "false" | "(" expr ")"
//	primary = term
//	        | key op value
//	        | key "=~" /regexp/flags
//	        | key [ "not" ] "in" key
//	op      = "==" | "!=" | "===" | "!==" | "<" | "<=" | ">" | ">="
//
// As in VS Code's parser (_term), "!" applies only to a key, true, false
// or a parenthesized expression: "!a == 'b'" and "!!a" are errors (write
// "a != 'b'", "!(a == 'b')", "a").
//
// Keys and unquoted values are runs of characters other than blanks and
// ( ) ! = < > & | ' ". Values are single-quoted strings ('a b'),
// double-quoted strings ("a b", MinCode's plugin dialect) or bare words.
//
// Semantics follow VS Code (Monaco's contextkey):
//
//   - A bare key is true when its value is truthy (not missing, nil, false,
//     0, NaN or ""); an undefined key is false.
//   - Only the bare words true and false are rewritten: "key == true" means
//     the key is truthy, "key == false" that it is not ("!=" the opposite).
//     A quoted 'true' or "true" is a string and compares as one, so it never
//     equals a boolean value.
//   - == and != compare a string value with the literal; a numeric value is
//     equal when the literal parses as the same number (JavaScript's loose
//     3 == '3'); booleans and nil equal no literal.
//   - < <= > >= need a numeric literal (else the comparison is false); the
//     value is a Go number, or a string that strconv.ParseFloat accepts in
//     full; anything else is false.
//   - =~ tests the key's value (a string, or other values formatted with
//     fmt.Sprint) with a JavaScript regular expression compiled through
//     package editor/regex: a search, not anchored at the start; flags i, s,
//     m, u, g, y.
//   - "a in b" is true when b's value is a slice or array containing a's
//     value, or a map with a's string value as a key; "not in" negates it.
//
// Differences from VS Code:
//
//   - Double-quoted strings are accepted as literals (MinCode's dialect);
//     VS Code's scanner would take the quotes as part of a bare word.
//   - Expressions longer than MaxLength (512, MinCode's limit) are refused.
//   - < <= > >= parse strings strictly ("12px" is not 12); JavaScript's
//     parseFloat would read a numeric prefix.
//   - =~ on an undefined key is false (JavaScript tests the string
//     "undefined"); the regular expression engines differ in corner cases
//     (see package editor/regex).
//   - "in" works on Go slices, arrays and maps with string keys in place of
//     JavaScript arrays and objects.
//   - Monaco's normalizations (constant folding, distributing && over ||)
//     are not applied; Implies compares flattened, order-independent
//     && / || terms instead.
//
// Experimental: this package's API may change in any 0.x release (see
// docs/release.md, "API 稳定性").
package when

import (
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/tangoui/tango/ui/editor/regex"
)

// MaxLength is the longest expression Parse accepts (MinCode's
// WHEN_EXPRESSION_MAX_LENGTH).
const MaxLength = 512

// Context supplies context key values.
type Context interface {
	Value(key string) (any, bool)
}

// MapContext is a Context backed by a map.
type MapContext map[string]any

// Value implements Context.
func (m MapContext) Value(key string) (any, bool) {
	v, ok := m[key]
	return v, ok
}

// ContextFunc adapts a function to Context.
type ContextFunc func(key string) (any, bool)

// Value implements Context.
func (f ContextFunc) Value(key string) (any, bool) { return f(key) }

// Expr is a parsed expression.
type Expr interface {
	// Eval evaluates the expression; a nil ctx has no keys.
	Eval(ctx Context) bool
	// String returns the expression in a canonical form: parsing it again
	// gives an equal expression (the same String).
	String() string
}

// Parse parses expr. An empty or blank expression is an error (callers
// treat an absent when as "always"; see Eval).
func Parse(expr string) (Expr, error) {
	s := strings.TrimSpace(expr)
	if s == "" {
		return nil, errors.New("when: empty expression")
	}
	if len(s) > MaxLength {
		return nil, fmt.Errorf("when: expression exceeds %d characters", MaxLength)
	}
	toks, err := lex(s)
	if err != nil {
		return nil, err
	}
	p := &parser{toks: toks}
	e, err := p.or()
	if err != nil {
		return nil, err
	}
	if p.i < len(p.toks) {
		return nil, fmt.Errorf("when: unexpected %q", p.toks[p.i].text)
	}
	return e, nil
}

// Eval parses and evaluates expr: a blank expression is true, a malformed
// one false (a bad when hides the command, as MinCode's evaluateWhen).
func Eval(expr string, ctx Context) bool {
	if strings.TrimSpace(expr) == "" {
		return true
	}
	e, err := Parse(expr)
	if err != nil {
		return false
	}
	return e.Eval(ctx)
}

// --- lexer ---

type tokKind uint8

const (
	tWord  tokKind = iota // key, bare value, true/false/in/not
	tStr                  // single-quoted string
	tDStr                 // double-quoted string
	tRegex                // /re/flags
	tOp                   // ! && || == != === !== < <= > >= =~ ( )
)

type token struct {
	kind  tokKind
	text  string // word / string contents / operator
	flags string // regex flags
}

func isWordByte(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '(', ')', '!', '=', '<', '>', '&', '|', '\'', '"':
		return false
	}
	return true
}

func lex(s string) ([]token, error) {
	var toks []token
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '(' || c == ')':
			toks = append(toks, token{kind: tOp, text: s[i : i+1]})
			i++
		case c == '!':
			switch {
			case strings.HasPrefix(s[i:], "!=="):
				toks = append(toks, token{kind: tOp, text: "!="})
				i += 3
			case strings.HasPrefix(s[i:], "!="):
				toks = append(toks, token{kind: tOp, text: "!="})
				i += 2
			default:
				toks = append(toks, token{kind: tOp, text: "!"})
				i++
			}
		case c == '=':
			switch {
			case strings.HasPrefix(s[i:], "==="):
				toks = append(toks, token{kind: tOp, text: "=="})
				i += 3
			case strings.HasPrefix(s[i:], "=="):
				toks = append(toks, token{kind: tOp, text: "=="})
				i += 2
			case strings.HasPrefix(s[i:], "=~"):
				toks = append(toks, token{kind: tOp, text: "=~"})
				i += 2
				j := i
				for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
					j++
				}
				t, n, err := lexRegex(s[j:])
				if err != nil {
					return nil, err
				}
				toks = append(toks, t)
				i = j + n
			default:
				return nil, errors.New("when: use == for equality")
			}
		case c == '<' || c == '>':
			if i+1 < len(s) && s[i+1] == '=' {
				toks = append(toks, token{kind: tOp, text: s[i : i+2]})
				i += 2
			} else {
				toks = append(toks, token{kind: tOp, text: s[i : i+1]})
				i++
			}
		case c == '&' || c == '|':
			if i+1 >= len(s) || s[i+1] != c {
				return nil, fmt.Errorf("when: use %c%c", c, c)
			}
			toks = append(toks, token{kind: tOp, text: s[i : i+2]})
			i += 2
		case c == '\'' || c == '"':
			j := strings.IndexByte(s[i+1:], c)
			if j < 0 {
				return nil, errors.New("when: string literal is not terminated")
			}
			k := tStr
			if c == '"' {
				k = tDStr
			}
			toks = append(toks, token{kind: k, text: s[i+1 : i+1+j]})
			i += j + 2
		default:
			j := i
			for j < len(s) && isWordByte(s[j]) {
				j++
			}
			toks = append(toks, token{kind: tWord, text: s[i:j]})
			i = j
		}
	}
	return toks, nil
}

// lexRegex reads /source/flags at the start of s; it returns the token and
// the bytes consumed.
func lexRegex(s string) (token, int, error) {
	if s == "" || s[0] != '/' {
		return token{}, 0, errors.New("when: =~ expects a /regexp/")
	}
	inClass := false
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '[':
			inClass = true
		case ']':
			inClass = false
		case '/':
			if inClass {
				continue
			}
			j := i + 1
			for j < len(s) && strings.IndexByte("gimsuy", s[j]) >= 0 {
				j++
			}
			return token{kind: tRegex, text: s[1:i], flags: s[i+1 : j]}, j, nil
		}
	}
	return token{}, 0, errors.New("when: regexp is not terminated")
}

// --- parser ---

type parser struct {
	toks []token
	i    int
}

func (p *parser) peekOp(op string) bool {
	return p.i < len(p.toks) && p.toks[p.i].kind == tOp && p.toks[p.i].text == op
}

func (p *parser) peekWord(w string) bool {
	return p.i < len(p.toks) && p.toks[p.i].kind == tWord && p.toks[p.i].text == w
}

func (p *parser) or() (Expr, error) {
	e, err := p.and()
	if err != nil {
		return nil, err
	}
	terms := []Expr{e}
	for p.peekOp("||") {
		p.i++
		e, err := p.and()
		if err != nil {
			return nil, err
		}
		terms = append(terms, e)
	}
	if len(terms) == 1 {
		return terms[0], nil
	}
	return orExpr(terms), nil
}

func (p *parser) and() (Expr, error) {
	e, err := p.unary()
	if err != nil {
		return nil, err
	}
	terms := []Expr{e}
	for p.peekOp("&&") {
		p.i++
		e, err := p.unary()
		if err != nil {
			return nil, err
		}
		terms = append(terms, e)
	}
	if len(terms) == 1 {
		return terms[0], nil
	}
	return andExpr(terms), nil
}

func (p *parser) unary() (Expr, error) {
	if p.peekOp("!") {
		p.i++
		e, err := p.term()
		if err != nil {
			return nil, err
		}
		return notExpr{e}, nil
	}
	return p.primary()
}

// term is what "!" applies to: a key, true, false or ( expr ).
func (p *parser) term() (Expr, error) {
	if p.i >= len(p.toks) {
		return nil, errors.New("when: unexpected end of expression after !")
	}
	t := p.toks[p.i]
	switch {
	case t.kind == tOp && t.text == "(":
		return p.primary()
	case t.kind == tWord:
		p.i++
		switch t.text {
		case "true":
			return constExpr(true), nil
		case "false":
			return constExpr(false), nil
		}
		return hasExpr(t.text), nil
	}
	return nil, fmt.Errorf("when: ! expects a key, true, false or (, got %q", t.text)
}

func (p *parser) primary() (Expr, error) {
	if p.i >= len(p.toks) {
		return nil, errors.New("when: unexpected end of expression")
	}
	t := p.toks[p.i]
	if t.kind == tOp && t.text == "(" {
		p.i++
		e, err := p.or()
		if err != nil {
			return nil, err
		}
		if !p.peekOp(")") {
			return nil, errors.New("when: missing )")
		}
		p.i++
		return parenExpr{e}, nil
	}
	if t.kind != tWord {
		return nil, fmt.Errorf("when: expected a context key, got %q", t.text)
	}
	p.i++
	switch t.text {
	case "true":
		return constExpr(true), nil
	case "false":
		return constExpr(false), nil
	}
	key := t.text
	if p.i >= len(p.toks) {
		return hasExpr(key), nil
	}
	n := p.toks[p.i]
	switch {
	case n.kind == tOp && (n.text == "==" || n.text == "!="):
		p.i++
		v, ok := p.value()
		if !ok {
			return nil, fmt.Errorf("when: %s needs a value", n.text)
		}
		var e Expr = eqExpr{key: key, val: v.text, dq: v.kind == tDStr}
		if v.kind == tWord {
			switch v.text {
			case "true":
				e = hasExpr(key)
			case "false":
				e = notExpr{hasExpr(key)}
			}
		}
		if n.text == "!=" {
			if ne, ok := e.(notExpr); ok {
				return ne.e, nil
			}
			return neExpr{e}, nil
		}
		return e, nil
	case n.kind == tOp && (n.text == "<" || n.text == "<=" || n.text == ">" || n.text == ">="):
		p.i++
		v, ok := p.value()
		if !ok {
			return nil, fmt.Errorf("when: %s needs a value", n.text)
		}
		f, err := strconv.ParseFloat(v.text, 64)
		return cmpExpr{key: key, op: n.text, raw: v.text, num: f, ok: err == nil}, nil
	case n.kind == tOp && n.text == "=~":
		p.i++
		if p.i >= len(p.toks) || p.toks[p.i].kind != tRegex {
			return nil, errors.New("when: =~ expects a /regexp/")
		}
		r := p.toks[p.i]
		p.i++
		// A search, not anchored: Compile anchors at the start of the input.
		re, err := regex.Compile(`[\s\S]*?(?:`+r.text+`)`, r.flags)
		if err != nil {
			return nil, fmt.Errorf("when: bad regexp /%s/%s: %w", r.text, r.flags, err)
		}
		return reExpr{key: key, src: r.text, flags: r.flags, re: re}, nil
	case n.kind == tWord && n.text == "in":
		p.i++
		return p.inRest(key, false)
	case n.kind == tWord && n.text == "not" && p.i+1 < len(p.toks) && p.toks[p.i+1].kind == tWord && p.toks[p.i+1].text == "in":
		p.i += 2
		return p.inRest(key, true)
	}
	return hasExpr(key), nil
}

func (p *parser) inRest(key string, not bool) (Expr, error) {
	if p.i >= len(p.toks) || p.toks[p.i].kind != tWord {
		return nil, errors.New("when: in expects a context key")
	}
	other := p.toks[p.i].text
	p.i++
	return inExpr{key: key, in: other, not: not}, nil
}

// value reads a comparison value: a quoted string or a bare word.
func (p *parser) value() (token, bool) {
	if p.i >= len(p.toks) {
		return token{}, false
	}
	t := p.toks[p.i]
	if t.kind != tWord && t.kind != tStr && t.kind != tDStr {
		return token{}, false
	}
	p.i++
	return t, true
}

// --- expressions ---

func lookup(ctx Context, key string) (any, bool) {
	if ctx == nil {
		return nil, false
	}
	return ctx.Value(key)
}

// truthy is JavaScript truthiness for context values.
func truthy(v any) bool {
	switch v := v.(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		return v != ""
	case int:
		return v != 0
	case int64:
		return v != 0
	case int32:
		return v != 0
	case uint:
		return v != 0
	case uint64:
		return v != 0
	case float64:
		return v != 0 && v == v
	case float32:
		return v != 0 && v == v
	}
	return true
}

// number returns v as a float64 (numbers, and strings that parse).
func number(v any) (float64, bool) {
	switch v := v.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint64:
		return float64(v), true
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	}
	return 0, false
}

type constExpr bool

func (e constExpr) Eval(Context) bool { return bool(e) }
func (e constExpr) String() string {
	if e {
		return "true"
	}
	return "false"
}

type hasExpr string

func (e hasExpr) Eval(ctx Context) bool {
	v, ok := lookup(ctx, string(e))
	return ok && truthy(v)
}
func (e hasExpr) String() string { return string(e) }

type notExpr struct{ e Expr }

func (e notExpr) Eval(ctx Context) bool { return !e.e.Eval(ctx) }
func (e notExpr) String() string {
	switch e.e.(type) {
	case hasExpr, constExpr, parenExpr:
		return "!" + e.e.String()
	}
	return "!(" + e.e.String() + ")" // "!" takes only a term
}

type parenExpr struct{ e Expr }

func (e parenExpr) Eval(ctx Context) bool { return e.e.Eval(ctx) }
func (e parenExpr) String() string        { return "(" + e.e.String() + ")" }

type andExpr []Expr

func (e andExpr) Eval(ctx Context) bool {
	for _, t := range e {
		if !t.Eval(ctx) {
			return false
		}
	}
	return true
}
func (e andExpr) String() string { return join(e, " && ") }

type orExpr []Expr

func (e orExpr) Eval(ctx Context) bool {
	for _, t := range e {
		if t.Eval(ctx) {
			return true
		}
	}
	return false
}
func (e orExpr) String() string { return join(e, " || ") }

func join(es []Expr, sep string) string {
	parts := make([]string, len(es))
	for i, t := range es {
		parts[i] = t.String()
	}
	return strings.Join(parts, sep)
}

func quote(s string, dq bool) string {
	if dq {
		return `"` + s + `"`
	}
	return "'" + s + "'"
}

type eqExpr struct {
	key, val string
	dq       bool // double-quoted literal (MinCode dialect)
}

func (e eqExpr) Eval(ctx Context) bool {
	v, ok := lookup(ctx, e.key)
	if !ok {
		return false
	}
	switch v := v.(type) {
	case string:
		return v == e.val
	case bool, nil:
		return false
	}
	if n, ok := number(v); ok {
		f, err := strconv.ParseFloat(e.val, 64)
		return err == nil && f == n
	}
	return fmt.Sprint(v) == e.val
}
func (e eqExpr) String() string { return e.key + " == " + quote(e.val, e.dq) }

// neExpr is the negation of an == comparison (printed with !=).
type neExpr struct{ e Expr }

func (e neExpr) Eval(ctx Context) bool { return !e.e.Eval(ctx) }
func (e neExpr) String() string {
	if eq, ok := e.e.(eqExpr); ok {
		return eq.key + " != " + quote(eq.val, eq.dq)
	}
	return notExpr(e).String()
}

type cmpExpr struct {
	key, op, raw string
	num          float64
	ok           bool // raw is a number
}

func (e cmpExpr) Eval(ctx Context) bool {
	if !e.ok {
		return false
	}
	v, ok := lookup(ctx, e.key)
	if !ok {
		return false
	}
	n, ok := number(v)
	if !ok {
		return false
	}
	switch e.op {
	case "<":
		return n < e.num
	case "<=":
		return n <= e.num
	case ">":
		return n > e.num
	}
	return n >= e.num
}
func (e cmpExpr) String() string { return e.key + " " + e.op + " " + e.raw }

type reExpr struct {
	key, src, flags string
	re              regex.Regexp
}

func (e reExpr) Eval(ctx Context) bool {
	v, ok := lookup(ctx, e.key)
	if !ok || v == nil {
		return false
	}
	s, ok := v.(string)
	if !ok {
		s = fmt.Sprint(v)
	}
	return e.re.MatchStart(s) != nil
}
func (e reExpr) String() string { return e.key + " =~ /" + e.src + "/" + e.flags }

type inExpr struct {
	key, in string
	not     bool
}

func (e inExpr) Eval(ctx Context) bool {
	r := e.contains(ctx)
	if e.not {
		return !r
	}
	return r
}

func (e inExpr) contains(ctx Context) bool {
	item, ok := lookup(ctx, e.key)
	if !ok {
		return false
	}
	src, ok := lookup(ctx, e.in)
	if !ok || src == nil {
		return false
	}
	switch s := src.(type) {
	case []string:
		str, ok := item.(string)
		if !ok {
			return false
		}
		for _, x := range s {
			if x == str {
				return true
			}
		}
		return false
	case []any:
		for _, x := range s {
			if equal(x, item) {
				return true
			}
		}
		return false
	case map[string]bool:
		str, ok := item.(string)
		_, has := s[str]
		return ok && has
	case map[string]any:
		str, ok := item.(string)
		_, has := s[str]
		return ok && has
	}
	rv := reflect.ValueOf(src)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			if x := rv.Index(i); x.CanInterface() && equal(x.Interface(), item) {
				return true
			}
		}
	case reflect.Map:
		str, ok := item.(string)
		if !ok || rv.Type().Key().Kind() != reflect.String {
			return false
		}
		return rv.MapIndex(reflect.ValueOf(str).Convert(rv.Type().Key())).IsValid()
	}
	return false
}

// equal is == for values that may not be comparable (false then).
func equal(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	ta, tb := reflect.TypeOf(a), reflect.TypeOf(b)
	if ta != tb || !ta.Comparable() {
		return false
	}
	return a == b
}

func (e inExpr) String() string {
	if e.not {
		return e.key + " not in " + e.in
	}
	return e.key + " in " + e.in
}
