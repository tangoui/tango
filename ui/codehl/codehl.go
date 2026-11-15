// Package codehl implements ui.CodeHighlighter with the code editor's
// language support: the Monarch grammars of ui/editor/langs and the
// incremental tokenizer of ui/editor/highlight. Install it once to color
// ui.CodeBlock and the fenced code blocks of ui.Markdown:
//
//	ui.SetCodeHighlighter(codehl.New())
//
// Token types are mapped to ui.CodeTokenKind (see Kind); the colors come
// from the CodeBlock component token of the current ui theme, so code
// follows the light, dark and high-contrast themes of every design system.
//
// Experimental: this package's API may change in any 0.x release (see
// docs/release.md, "API 稳定性").
package codehl

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/highlight"
	"github.com/tangoui/tango/ui/editor/langs"
	"github.com/tangoui/tango/ui/editor/monarch"
)

// Tuning of the foreground tokenization and the document cache.
const (
	// frameBudget is how long one document may tokenize on the UI thread
	// per frameWindow; the rest of a long document is tokenized by a
	// background goroutine (documents of backgroundLines lines or more)
	// or in later frames.
	frameBudget     = 6 * time.Millisecond
	frameWindow     = 16 * time.Millisecond
	backgroundLines = 2000
	cacheSize       = 32
)

// Highlighter is a ui.CodeHighlighter over a Monarch registry. It keeps the
// most recently used documents, so asking again for the same code (a
// Markdown block re-rendered every frame) continues where it stopped. It is
// safe for concurrent use.
type Highlighter struct {
	reg *monarch.Registry

	mu    sync.Mutex
	docs  map[docKey]*document
	order []docKey // least recently used first
}

type docKey struct{ lang, code string }

// New returns a Highlighter over the code editor's shared registry
// (editor.DefaultRegistry), so grammars compiled for editors are reused.
func New() *Highlighter { return NewWithRegistry(editor.DefaultRegistry()) }

// NewWithRegistry returns a Highlighter over reg (grammars added to reg
// with monarch.Registry.Add are available by their id).
func NewWithRegistry(reg *monarch.Registry) *Highlighter {
	return &Highlighter{reg: reg, docs: map[docKey]*document{}}
}

// Install sets a new Highlighter as ui's default (ui.SetCodeHighlighter).
func Install() { ui.SetCodeHighlighter(New()) }

// LanguageID resolves lang (an id, an alias such as "golang" or "js", or a
// file extension with or without the dot) to a built-in language id, ""
// when none matches.
func LanguageID(lang string) string {
	lang = strings.TrimSpace(lang)
	if lang == "" {
		return ""
	}
	if l := langs.ByID(lang); l != nil {
		return l.ID
	}
	low := strings.ToLower(lang)
	if l := langs.ByID(low); l != nil {
		return l.ID
	}
	if id, ok := shorthands[low]; ok {
		return id
	}
	for _, l := range langs.Builtin() {
		for _, a := range l.Aliases {
			if strings.EqualFold(a, low) {
				return l.ID
			}
		}
	}
	if l := langs.ForFilename("file." + strings.TrimPrefix(low, ".")); l != nil {
		return l.ID
	}
	if l := langs.ForFilename(low); l != nil { // "Dockerfile", "Makefile"
		return l.ID
	}
	return ""
}

// shorthands are fence names the Monaco language data has no alias for.
// JSON has no Monarch grammar (Monaco tokenizes it in a worker); the
// JavaScript grammar colors its strings, numbers and literals.
var shorthands = map[string]string{
	"golang": "go", "json": "javascript", "jsonc": "javascript", "json5": "javascript", "jsonl": "javascript",
	"zsh": "shell", "console": "shell", "shellscript": "shell",
}

// Document implements ui.CodeHighlighter: nil for an unknown language or
// one without a grammar.
func (h *Highlighter) Document(lang, code string) ui.CodeDocument {
	k := docKey{lang, code}
	h.mu.Lock()
	defer h.mu.Unlock()
	if d := h.docs[k]; d != nil {
		h.touch(k)
		return d
	}
	id := LanguageID(lang)
	if id == "" {
		return nil
	}
	lx, err := h.reg.Lexer(id)
	if err != nil || lx == nil {
		return nil
	}
	d := newDocument(lx, code)
	h.docs[k] = d
	h.order = append(h.order, k)
	if len(h.order) > cacheSize {
		delete(h.docs, h.order[0])
		h.order = h.order[1:]
	}
	return d
}

func (h *Highlighter) touch(k docKey) {
	for i, o := range h.order {
		if o == k {
			copy(h.order[i:], h.order[i+1:])
			h.order[len(h.order)-1] = k
			return
		}
	}
}

// document is the highlighting of one piece of code.
type document struct {
	snap buffer.Snapshot
	hl   *highlight.Highlighter

	mu     sync.Mutex
	toks   []monarch.Token
	window time.Time     // start of the current budget window
	spent  time.Duration // foreground tokenizing in the window
}

func newDocument(lx *monarch.Lexer, code string) *document {
	d := &document{snap: buffer.NewSnapshot(code), hl: highlight.New(lx)}
	d.hl.Sync(d.snap)
	if d.snap.LineCount() >= backgroundLines {
		ctx, cancel := context.WithCancel(context.Background())
		ch := make(chan buffer.Snapshot, 1)
		ch <- d.snap
		d.hl.Start(ctx, ch, func() {})
		// The goroutine holds the Highlighter, not d: stop it once d is
		// unreachable (evicted from the cache and dropped by the viewers).
		runtime.AddCleanup(d, func(cancel context.CancelFunc) { cancel() }, cancel)
	}
	return d
}

// Line implements ui.CodeDocument.
func (d *document) Line(i int, dst []ui.CodeSpan) ([]ui.CodeSpan, bool) {
	dst = dst[:0]
	if i < 0 || i >= d.snap.LineCount() {
		return dst, true
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now()
	if now.Sub(d.window) > frameWindow {
		d.window, d.spent = now, 0
	}
	if budget := frameBudget - d.spent; budget > 0 {
		d.hl.Highlight(d.snap, i+1, budget)
		d.spent += time.Since(now)
	}
	toks, final := d.hl.Tokens(d.snap, i, d.toks[:0])
	d.toks = toks
	n := len(d.snap.Line(i))
	for k, t := range toks {
		kind := Kind(t.Type)
		if kind == ui.CodePlain {
			continue
		}
		end := n
		if k+1 < len(toks) {
			end = toks[k+1].ByteStart
		}
		if end <= t.ByteStart {
			continue
		}
		if m := len(dst); m > 0 && dst[m-1].Kind == kind && dst[m-1].End == t.ByteStart {
			dst[m-1].End = end
			continue
		}
		dst = append(dst, ui.CodeSpan{Start: t.ByteStart, End: end, Kind: kind})
	}
	return dst, final
}

// Kind maps a Monarch token type ("keyword.go", "string.key.json",
// "type.identifier.ts" …) to a ui.CodeTokenKind.
func Kind(tokenType string) ui.CodeTokenKind {
	if tokenType == "" {
		return ui.CodePlain
	}
	first, rest, _ := strings.Cut(tokenType, ".")
	switch first {
	case "keyword":
		return ui.CodeKeyword
	case "string":
		if strings.HasPrefix(rest, "key") {
			return ui.CodeProperty
		}
		return ui.CodeString
	case "number":
		return ui.CodeNumber
	case "comment":
		return ui.CodeComment
	case "type", "namespace", "class", "interface", "struct":
		return ui.CodeType
	case "delimiter", "bracket":
		return ui.CodePunctuation
	case "operator", "operators":
		return ui.CodeOperator
	case "tag", "metatag":
		return ui.CodeTag
	case "attribute":
		if strings.HasPrefix(rest, "value") {
			return ui.CodeString
		}
		return ui.CodeAttribute
	case "annotation", "decorator":
		return ui.CodeAttribute
	case "regexp", "regex":
		return ui.CodeRegexp
	case "variable":
		return ui.CodeVariable
	case "constant":
		return ui.CodeConstant
	case "predefined", "function", "builtin":
		return ui.CodeFunction
	case "key":
		return ui.CodeProperty
	case "invalid":
		return ui.CodeInvalid
	}
	return ui.CodePlain
}
