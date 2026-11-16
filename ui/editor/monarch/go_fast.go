package monarch

import "strings"

// The fast path implements the built-in grammar's ASCII root rules. Anything
// outside this subset uses Monarch, including multiline constructs. In
// particular, go/scanner is not an equivalent fallback: its lexical and error
// recovery rules differ from the editor grammar.
func fastGoLine(line string, dst []Token) ([]Token, bool) { return manualGoLine(line, dst) }

func manualGoLine(line string, dst []Token) ([]Token, bool) {
	for i := 0; i < len(line); i++ {
		if line[i] >= 0x80 || line[i] == '\r' || line[i] == '\n' {
			return dst, false
		}
	}
	out := dst
	base, prev := len(dst), 0
	for i := 0; i < len(line); {
		c := line[i]
		start := i
		if c == ' ' || c == '\t' {
			i++
			continue
		}
		typ := ""
		switch {
		case isGoIdentStart(c):
			i++
			for i < len(line) && isGoIdentPart(line[i]) {
				i++
			}
			typ = "identifier.go"
			if t, ok := fastGoKeywordType(line[start:i]); ok {
				typ = t
			}
		case c >= '0' && c <= '9' || c == '.' && i+1 < len(line) && goDigit(line[i+1]):
			var n int
			n, typ = goNumber(line[i:])
			i += n
		case c == '"':
			// Check closure before appending tokens, so an unterminated string retains
			// the grammar's string.invalid and end-of-line state behavior.
			end := i + 1
			for end < len(line) && line[end] != '"' {
				if line[end] == '\\' {
					end++
				}
				end++
			}
			if end >= len(line) {
				return dst, false
			}
			end++
			part := i
			i++
			for i < end-1 {
				if line[i] != '\\' {
					i++
					continue
				}
				if part < i {
					out = appendFastGoToken(out, base, &prev, part, i, "string.go")
				}
				n, valid := goEscape(line[i : end-1])
				t := "string.escape.invalid.go"
				if valid {
					t = "string.escape.go"
				}
				out = appendFastGoToken(out, base, &prev, i, i+n, t)
				i += n
				part = i
			}
			out = appendFastGoToken(out, base, &prev, part, end, "string.go")
			i = end
			continue
		case c == '`':
			n := strings.IndexByte(line[i+1:], '`')
			if n < 0 {
				return dst, false
			}
			if strings.IndexByte(line[i+1:i+n+1], '\\') >= 0 {
				return dst, false
			}
			i += n + 2
			typ = "string.go"
		case c == '\'':
			return dst, false // rune validation and grouped escapes stay with Monarch
		case c == '/' && i+1 < len(line) && line[i+1] == '/':
			i = len(line)
			typ = "comment.go"
		case c == '/' && i+1 < len(line) && line[i+1] == '*':
			return dst, false // includes doc comments and cross-line state transitions
		case c == '[' && i+1 < len(line) && line[i+1] == '[':
			return dst, false // annotation has priority over square brackets
		case c == '{' || c == '}':
			i++
			typ = "delimiter.curly.go"
		case c == '(' || c == ')':
			i++
			typ = "delimiter.parenthesis.go"
		case c == '[' || c == ']':
			i++
			typ = "delimiter.square.go"
		case (c == '<' || c == '>') && (i+1 == len(line) || !isGoSymbol(line[i+1])):
			i++
			typ = "delimiter.angle.go"
		case isGoSymbol(c):
			i++
			for i < len(line) && isGoSymbol(line[i]) {
				i++
			}
			if goOperators[line[start:i]] {
				typ = "delimiter.go"
			}
		case c == ';' || c == ',' || c == '.':
			i++
			typ = "delimiter.go"
		default:
			return dst, false
		}
		out = appendFastGoToken(out, base, &prev, start, i, typ)
	}
	if prev < len(line) {
		out = appendFastGoToken(out, base, &prev, prev, len(line), "")
	}
	return out, true
}

func appendFastGoToken(out []Token, base int, prev *int, off, end int, typ string) []Token {
	if off > *prev && (len(out) == base || out[len(out)-1].Type != "") {
		out = append(out, Token{ByteStart: *prev})
	}
	if len(out) == base || out[len(out)-1].Type != typ {
		out = append(out, Token{ByteStart: off, Type: typ})
	}
	*prev = end
	return out
}

func isGoIdentStart(c byte) bool { return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isGoIdentPart(c byte) bool  { return isGoIdentStart(c) || goDigit(c) }
func goDigit(c byte) bool        { return c >= '0' && c <= '9' }
func goHex(c byte) bool          { return goDigit(c) || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' }
func isGoSymbol(c byte) bool {
	switch c {
	case '=', '>', '<', '!', '~', '?', ':', '&', '|', '+', '-', '*', '/', '^', '%':
		return true
	}
	return false
}

// goNumber follows the grammar's rule order (including its permissive
// incomplete exponent syntax), rather than Go's compiler tokenization.
func goNumber(s string) (int, string) {
	digits := 0
	for digits < len(s) && goDigit(s[digits]) {
		digits++
	}
	if digits > 0 && digits < len(s) && (s[digits] == 'e' || s[digits] == 'E') {
		end := digits + 1
		n := end
		if n < len(s) && (s[n] == '-' || s[n] == '+') {
			n++
		}
		first := n
		for n < len(s) && goDigit(s[n]) {
			n++
		}
		if n > first {
			end = n
		}
		return end, "number.float.go"
	}
	if digits < len(s) && s[digits] == '.' && digits+1 < len(s) && goDigit(s[digits+1]) {
		n := digits + 2
		for n < len(s) && goDigit(s[n]) {
			n++
		}
		end := n
		if n < len(s) && (s[n] == 'e' || s[n] == 'E') {
			n++
			if n < len(s) && (s[n] == '-' || s[n] == '+') {
				n++
			}
			first := n
			for n < len(s) && goDigit(s[n]) {
				n++
			}
			if n > first {
				end = n
			}
		}
		return end, "number.float.go"
	}
	if s[0] == '0' && len(s) > 1 {
		start := 1
		kind := "number.octal.go"
		base := byte(8)
		if s[1] == 'x' || s[1] == 'X' {
			start = 2
			kind = "number.hex.go"
			base = 16
		} else if s[1] == 'b' || s[1] == 'B' {
			start = 2
			kind = "number.binary.go"
			base = 2
		}
		last := start
		for n := start; n < len(s); n++ {
			c := s[n]
			valid := c >= '0' && c < '0'+base
			if base == 16 {
				valid = goHex(c)
			}
			if valid {
				last = n + 1
			} else if c != '\'' {
				break
			}
		}
		if last > start {
			return last, kind
		}
	}
	n := 1
	for n < len(s) && (goDigit(s[n]) || s[n] == '\'') {
		n++
	}
	return n, "number.go"
}

func goEscape(s string) (int, bool) {
	if len(s) < 2 {
		return 1, false
	}
	c := s[1]
	switch c {
	case 'a', 'b', 'f', 'n', 'r', 't', 'v', '\\', '"', '\'':
		return 2, true
	case 'x', 'u', 'U':
		min, max := 4, 4
		if c == 'x' {
			min, max = 1, 4
		} else if c == 'U' {
			min, max = 8, 8
		}
		n := 2
		for n < len(s) && n < 2+max && goHex(s[n]) {
			n++
		}
		if n >= 2+min {
			return n, true
		}
	}
	return 2, false
}

var goOperators = map[string]bool{
	"+": true, "-": true, "*": true, "/": true, "%": true, "&": true, "|": true, "^": true, "<<": true, ">>": true, "&^": true,
	"+=": true, "-=": true, "*=": true, "/=": true, "%=": true, "&=": true, "|=": true, "^=": true, "<<=": true, ">>=": true, "&^=": true,
	"&&": true, "||": true, "<-": true, "++": true, "--": true, "==": true, "<": true, ">": true, "=": true, ":": true, "!": true, "!=": true, "<=": true, ">=": true, ":=": true,
}

func fastGoKeywordType(lit string) (string, bool) {
	typ, ok := goKeywordTypes[lit]
	return typ, ok
}

var goKeywordTypes = map[string]string{
	"break": "keyword.break.go", "default": "keyword.default.go", "func": "keyword.func.go", "interface": "keyword.interface.go", "select": "keyword.select.go",
	"case": "keyword.case.go", "defer": "keyword.defer.go", "go": "keyword.go.go", "map": "keyword.map.go", "struct": "keyword.struct.go",
	"chan": "keyword.chan.go", "else": "keyword.else.go", "goto": "keyword.goto.go", "package": "keyword.package.go", "switch": "keyword.switch.go",
	"const": "keyword.const.go", "fallthrough": "keyword.fallthrough.go", "if": "keyword.if.go", "range": "keyword.range.go", "type": "keyword.type.go",
	"continue": "keyword.continue.go", "for": "keyword.for.go", "import": "keyword.import.go", "return": "keyword.return.go", "var": "keyword.var.go",
	"bool": "keyword.bool.go", "byte": "keyword.byte.go", "complex64": "keyword.complex64.go", "complex128": "keyword.complex128.go",
	"float32": "keyword.float32.go", "float64": "keyword.float64.go", "int": "keyword.int.go", "int8": "keyword.int8.go", "int16": "keyword.int16.go",
	"int32": "keyword.int32.go", "int64": "keyword.int64.go", "rune": "keyword.rune.go", "string": "keyword.string.go", "uint": "keyword.uint.go",
	"uint8": "keyword.uint8.go", "uint16": "keyword.uint16.go", "uint32": "keyword.uint32.go", "uint64": "keyword.uint64.go", "uintptr": "keyword.uintptr.go",
	"true": "keyword.true.go", "false": "keyword.false.go", "nil": "keyword.nil.go",
}
