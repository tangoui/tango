package ui

import (
	"strings"
	"sync/atomic"
	"time"

	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// Code highlighting hook
//
// Package ui draws code (CodeBlock, the fenced code blocks of Markdown) but
// does not tokenize it: the language grammars live in ui/editor, which
// imports ui. A CodeHighlighter installed with SetCodeHighlighter (package
// ui/codehl provides one backed by the code editor's highlighter) splits
// code into spans of CodeTokenKind; the viewers color the kinds with the
// CodeBlock component token. Without a highlighter code is plain text.

// CodeTokenKind classifies a span of highlighted code.
//
// Append-only: new kinds go at the end.
type CodeTokenKind uint8

// The token kinds; CodePlain is unhighlighted text.
const (
	CodePlain CodeTokenKind = iota
	CodeKeyword
	CodeString
	CodeNumber
	CodeComment
	CodeType
	CodeFunction
	CodeVariable
	CodeConstant
	CodeOperator
	CodePunctuation
	CodeTag
	CodeAttribute
	CodeProperty
	CodeRegexp
	CodeInvalid
)

// CodeSpan is a highlighted piece of one line: bytes [Start, End) of the
// line (without its "\n").
type CodeSpan struct {
	Start, End int
	Kind       CodeTokenKind
}

// CodeHighlighter splits code into highlighted spans by language.
// Implementations must be safe for use from the UI thread of several
// windows.
type CodeHighlighter interface {
	// Document prepares the highlighting of code (lines separated by
	// "\n") in language lang: a language id, alias or file extension
	// ("go", "ts", "python", "sh" …). It returns nil when the language is
	// unknown, and the code is shown as plain text.
	Document(lang, code string) CodeDocument
}

// CodeDocument is the highlighting of one immutable piece of code.
type CodeDocument interface {
	// Line appends the spans of line i (0-based) to dst[:0]: in order,
	// not overlapping, gaps are plain text. It reports whether they are
	// final; spans that are not final are a guess made while the lines
	// above are still being tokenized, and the viewer asks again in a
	// later frame.
	Line(i int, dst []CodeSpan) ([]CodeSpan, bool)
}

type codeHighlighterBox struct{ h CodeHighlighter }

var codeHL atomic.Pointer[codeHighlighterBox]

// SetCodeHighlighter installs the highlighter CodeBlock and Markdown use
// when they are not given one of their own (nil: plain text).
//
//	ui.SetCodeHighlighter(codehl.New())
func SetCodeHighlighter(h CodeHighlighter) {
	if h == nil {
		codeHL.Store(nil)
		return
	}
	codeHL.Store(&codeHighlighterBox{h})
}

// DefaultCodeHighlighter returns the highlighter installed with
// SetCodeHighlighter (nil if none).
func DefaultCodeHighlighter() CodeHighlighter {
	if b := codeHL.Load(); b != nil {
		return b.h
	}
	return nil
}

// codeKindColor returns the color of kind k (zero for plain text: the
// caller's text color).
func codeKindColor(tk *theme.CodeBlockToken, k CodeTokenKind) theme.Color {
	switch k {
	case CodeKeyword:
		return tk.Keyword
	case CodeString:
		return tk.String
	case CodeNumber:
		return tk.Number
	case CodeComment:
		return tk.Comment
	case CodeType:
		return tk.Type
	case CodeFunction:
		return tk.Function
	case CodeVariable:
		return tk.Variable
	case CodeConstant:
		return tk.Constant
	case CodeOperator:
		return tk.Operator
	case CodePunctuation:
		return tk.Punctuation
	case CodeTag:
		return tk.Tag
	case CodeAttribute:
		return tk.Attribute
	case CodeProperty:
		return tk.Property
	case CodeRegexp:
		return tk.Regexp
	case CodeInvalid:
		return tk.Invalid
	}
	return theme.Color{}
}

// highlightSpans returns code as monospace RichText spans colored by h
// (plain text when h is nil or does not know lang) and whether the
// highlighting is final.
func highlightSpans(h CodeHighlighter, lang, code string, tk *theme.CodeBlockToken) ([]Span, bool) {
	if h == nil || lang == "" {
		return []Span{{Text: code, Mono: true}}, true
	}
	doc := h.Document(lang, code)
	if doc == nil {
		return []Span{{Text: code, Mono: true}}, true
	}
	final := true
	var out []Span
	var spans []CodeSpan
	i := 0
	for ln := range strings.SplitSeq(code, "\n") {
		if i > 0 {
			out = append(out, Span{Text: "\n", Mono: true})
		}
		var ok bool
		spans, ok = doc.Line(i, spans)
		final = final && ok
		pos := 0
		for _, sp := range spans {
			s, e := max(sp.Start, pos), min(sp.End, len(ln))
			if s >= e {
				continue
			}
			if s > pos {
				out = append(out, Span{Text: ln[pos:s], Mono: true})
			}
			out = append(out, Span{Text: ln[s:e], Mono: true, Color: codeKindColor(tk, sp.Kind), Italic: sp.Kind == CodeComment})
			pos = e
		}
		if pos < len(ln) {
			out = append(out, Span{Text: ln[pos:], Mono: true})
		}
		i++
	}
	return out, final
}

// codeHighlightRetry is how soon a viewer asks again for spans that were
// not final.
const codeHighlightRetry = 30 * time.Millisecond
