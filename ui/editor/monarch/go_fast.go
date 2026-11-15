package monarch

import (
	"go/scanner"
	"go/token"
)

// fastGoLine tokenizes a self-contained Go line with the standard scanner.
// Lines that may carry Monarch state across a newline, or that scanner cannot
// classify without recovery, return ok=false and use the normal grammar path.
func fastGoLine(line string, dst []Token) (out []Token, ok bool) {
	if out, ok = manualGoLine(line, dst); ok {
		return out, true
	}
	return scanGoLine(line, dst)
}

func manualGoLine(line string, dst []Token) (out []Token, ok bool) {
	out = dst
	prev := 0
	for i := 0; i < len(line); {
		c := line[i]
		if c >= 0x80 {
			return dst, false
		}
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			i++
			continue
		}
		start := i
		switch {
		case isGoIdentStart(c):
			i++
			for i < len(line) && isGoIdentPart(line[i]) {
				i++
			}
			lit := line[start:i]
			typ := "identifier.go"
			if keywordType, keyword := fastGoKeywordType(lit); keyword {
				typ = keywordType
			}
			out = appendFastGoToken(out, len(dst), &prev, start, i, typ)
		case c >= '0' && c <= '9' || c == '.' && i+1 < len(line) && line[i+1] >= '0' && line[i+1] <= '9':
			i++
			for i < len(line) {
				d := line[i]
				if (d >= '0' && d <= '9') || (d >= 'a' && d <= 'f') || (d >= 'A' && d <= 'F') || d == '.' || d == '_' || d == 'e' || d == 'E' || d == 'x' || d == 'X' || d == 'b' || d == 'B' || d == 'o' || d == 'O' {
					i++
					continue
				}
				if (d == '+' || d == '-') && i > start && (line[i-1] == 'e' || line[i-1] == 'E') {
					i++
					continue
				}
				break
			}
			typ := "number.go"
			if hasGoFloat(line[start:i]) {
				typ = "number.float.go"
			}
			out = appendFastGoToken(out, len(dst), &prev, start, i, typ)
		case c == '"' || c == '\'' || c == '`':
			quote := c
			i++
			closed := false
			for i < len(line) {
				if line[i] == '\\' && quote != '`' {
					i += 2
					continue
				}
				if line[i] == quote {
					i++
					closed = true
					break
				}
				i++
			}
			if !closed {
				return dst, false
			}
			out = appendFastGoToken(out, len(dst), &prev, start, i, "string.go")
		case c == '/' && i+1 < len(line) && line[i+1] == '/':
			out = appendFastGoToken(out, len(dst), &prev, start, len(line), "comment.go")
			prev = len(line)
			i = len(line)
		case c == '/' && i+1 < len(line) && line[i+1] == '*':
			end := start + 2
			for end+1 < len(line) && !(line[end] == '*' && line[end+1] == '/') {
				end++
			}
			if end+1 >= len(line) {
				return dst, false
			}
			i = end + 2
			out = appendFastGoToken(out, len(dst), &prev, start, i, "comment.go")
		case c == '{' || c == '}':
			i++
			out = appendFastGoToken(out, len(dst), &prev, start, i, "delimiter.curly.go")
		case c == '(' || c == ')':
			i++
			out = appendFastGoToken(out, len(dst), &prev, start, i, "delimiter.parenthesis.go")
		case c == '[' || c == ']':
			i++
			out = appendFastGoToken(out, len(dst), &prev, start, i, "delimiter.square.go")
		case c == '<' || c == '>':
			if i+1 < len(line) && (line[i+1] == '=' || line[i+1] == '<' || line[i+1] == '>' || line[i+1] == '-') {
				return dst, false
			}
			i++
			out = appendFastGoToken(out, len(dst), &prev, start, i, "delimiter.angle.go")
		case isGoOperator(c):
			i++
			for i < len(line) && isGoOperator(line[i]) {
				i++
			}
			out = appendFastGoToken(out, len(dst), &prev, start, i, "delimiter.go")
		default:
			return dst, false
		}
	}
	if prev < len(line) {
		out = append(out, Token{ByteStart: prev})
	}
	return out, true
}

func appendFastGoToken(out []Token, base int, prev *int, off, end int, typ string) []Token {
	if off > *prev {
		out = append(out, Token{ByteStart: *prev})
	}
	if len(out) > base && off == *prev && out[len(out)-1].Type == typ {
		*prev = end
		return out
	}
	out = append(out, Token{ByteStart: off, Type: typ})
	*prev = end
	return out
}

func scanGoLine(line string, dst []Token) (out []Token, ok bool) {
	file := token.NewFileSet().AddFile("", -1, len(line))
	var scan scanner.Scanner
	bad := false
	scan.Init(file, []byte(line), func(token.Position, string) { bad = true }, scanner.ScanComments)
	out = dst
	prev := 0
	for {
		pos, tok, lit := scan.Scan()
		if tok == token.EOF {
			break
		}
		off := file.Offset(pos)
		if tok == token.SEMICOLON && lit == "\n" {
			continue // implicit semicolon at the synthetic line ending
		}
		if off < prev {
			return dst, false
		}
		if off > prev {
			out = append(out, Token{ByteStart: prev})
		}
		end := off + len(lit)
		if len(lit) == 0 {
			end = off + len(tok.String())
		}
		if end <= off || end > len(line) {
			return dst, false
		}
		typ := fastGoTokenType(tok, lit)
		if len(out) > len(dst) && off == prev && out[len(out)-1].Type == typ {
			prev = end
			continue
		}
		out = append(out, Token{ByteStart: off, Type: typ})
		prev = end
	}
	if bad {
		return dst, false
	}
	if prev < len(line) {
		out = append(out, Token{ByteStart: prev})
	}
	return out, true
}

func isGoIdentStart(c byte) bool { return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isGoIdentPart(c byte) bool  { return isGoIdentStart(c) || c >= '0' && c <= '9' }
func isGoOperator(c byte) bool {
	for _, x := range "=><!~?:&|+-*/^%.,;" {
		if c == byte(x) {
			return true
		}
	}
	return false
}
func hasGoFloat(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '.' || s[i] == 'e' || s[i] == 'E' {
			return true
		}
	}
	return false
}

func fastGoTokenType(tok token.Token, lit string) string {
	switch {
	case tok == token.COMMENT:
		return "comment.go"
	case tok == token.IDENT:
		if typ, ok := fastGoKeywordType(lit); ok {
			return typ
		}
		return "identifier.go"
	case tok.IsKeyword():
		if typ, ok := fastGoKeywordType(lit); ok {
			return typ
		}
		return "keyword." + lit + ".go"
	case tok == token.INT:
		return "number.go"
	case tok == token.FLOAT || tok == token.IMAG:
		return "number.float.go"
	case tok == token.CHAR || tok == token.STRING:
		return "string.go"
	case tok == token.LBRACE || tok == token.RBRACE:
		return "delimiter.curly.go"
	case tok == token.LPAREN || tok == token.RPAREN:
		return "delimiter.parenthesis.go"
	case tok == token.LBRACK || tok == token.RBRACK:
		return "delimiter.square.go"
	case tok == token.LSS || tok == token.GTR:
		return "delimiter.angle.go"
	default:
		return "delimiter.go"
	}
}

func fastGoKeyword(lit string) bool {
	_, ok := fastGoKeywordType(lit)
	return ok
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
	"bool": "keyword.bool.go", "byte": "keyword.byte.go", "complex64": "keyword.complex64.go", "complex128": "keyword.complex128.go", "error": "keyword.error.go",
	"float32": "keyword.float32.go", "float64": "keyword.float64.go", "int": "keyword.int.go", "int8": "keyword.int8.go", "int16": "keyword.int16.go",
	"int32": "keyword.int32.go", "int64": "keyword.int64.go", "rune": "keyword.rune.go", "string": "keyword.string.go", "uint": "keyword.uint.go",
	"uint8": "keyword.uint8.go", "uint16": "keyword.uint16.go", "uint32": "keyword.uint32.go", "uint64": "keyword.uint64.go", "uintptr": "keyword.uintptr.go",
	"true": "keyword.true.go", "false": "keyword.false.go", "nil": "keyword.nil.go",
}
