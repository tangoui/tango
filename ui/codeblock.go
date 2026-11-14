package ui

import (
	"strings"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// CodeBlock: read-only, syntax-highlighted code.

// CodeBlockW is a read-only code viewer. Long lines scroll sideways; a
// block taller than its Height / MaxHeight scrolls and only the visible
// lines are shaped, so it stays fast for files of hundreds of thousands of
// lines. The text is selectable (drag, double click a word, triple click
// or click the line number for a line, Cmd/Ctrl+A) and copies with
// Cmd/Ctrl+C; the copy button copies everything.
type CodeBlockW struct {
	_ [0]func()

	c       *Context
	b       *Box
	code    string
	lang    string
	title   string
	numbers bool
	start   int
	copy    bool
	wrap    bool
	hl      CodeHighlighter
	hlSet   bool
	height  float32
	maxH    float32
	marked  map[int]bool
	label   string
	copied  bool
	v       *txView
	z       *lazy
}

type codeBlockState struct {
	src     string // the caller's code
	code    string // src with "\n" line breaks
	starts  []int  // byte offset of each line
	cells   []int32
	ver     uint64
	doc     CodeDocument
	docLang string
	docVer  uint64
	docHL   CodeHighlighter
	spans   map[int][]CodeSpan // final spans of recently drawn lines
	spansTh *theme.Theme
	copied  time.Time
}

// CodeBlock adds a read-only code block for code (lines separated by "\n";
// "\r\n" is accepted).
//
//	ui.CodeBlock(c, src).Lang("go").LineNumbers(true).MaxHeight(400)
func CodeBlock(c *Context, code string, opts ...layout.Option) *CodeBlockW {
	w := &CodeBlockW{c: c, code: code, start: 1, copy: true}
	w.b = c.box("codeblock", layout.Column, opts)
	w.z = c.lazy(w.b.n, w.build)
	return w
}

// Lang sets the language for highlighting: an id, alias or file extension
// ("go", "ts", "python" …).
func (w *CodeBlockW) Lang(lang string) *CodeBlockW { w.lang = lang; return w }

// Title shows a header row with title (a file name, the language …) and
// the copy button.
func (w *CodeBlockW) Title(s string) *CodeBlockW { w.title = s; return w }

// LineNumbers shows the line-number gutter.
func (w *CodeBlockW) LineNumbers(on bool) *CodeBlockW { w.numbers = on; return w }

// StartLine sets the number of the first line (default 1).
func (w *CodeBlockW) StartLine(n int) *CodeBlockW { w.start = n; return w }

// NoCopy hides the copy button.
func (w *CodeBlockW) NoCopy() *CodeBlockW { w.copy = false; return w }

// Wrap wraps long lines instead of scrolling sideways.
func (w *CodeBlockW) Wrap(on bool) *CodeBlockW { w.wrap = on; return w }

// Highlighter sets the highlighter (default DefaultCodeHighlighter; nil:
// plain text).
func (w *CodeBlockW) Highlighter(h CodeHighlighter) *CodeBlockW { w.hl, w.hlSet = h, true; return w }

// Height fixes the height of the code area (it scrolls).
func (w *CodeBlockW) Height(px float32) *CodeBlockW { w.height = px; return w }

// MaxHeight lets the code area grow with its lines up to px, then scroll.
// Without Height or MaxHeight the block is as tall as its code.
func (w *CodeBlockW) MaxHeight(px float32) *CodeBlockW { w.maxH = px; return w }

// HighlightLines tints lines (numbers as shown, see StartLine).
func (w *CodeBlockW) HighlightLines(lines ...int) *CodeBlockW {
	if w.marked == nil {
		w.marked = map[int]bool{}
	}
	for _, n := range lines {
		w.marked[n] = true
	}
	return w
}

// A11yLabel sets the accessible name of the code area (default: the
// title or language).
func (w *CodeBlockW) A11yLabel(s string) *CodeBlockW { w.label = s; return w }

// Layout applies layout options to the block.
func (w *CodeBlockW) Layout(opts ...layout.Option) *CodeBlockW {
	flexbox.Apply(&w.b.n.ln.Style, opts...)
	return w
}

// Copied reports a click on the copy button this frame.
func (w *CodeBlockW) Copied() bool { w.z.run(); return w.copied }

// Selection returns the selected text ("" if none).
func (w *CodeBlockW) Selection() string {
	w.z.run()
	if w.v == nil {
		return ""
	}
	return w.v.selection()
}

// ID returns the block's ID.
func (w *CodeBlockW) ID() ID { return w.b.n.id }

// sync indexes the code's lines when it changed.
func (st *codeBlockState) sync(code string) {
	if st.starts != nil && st.src == code {
		return
	}
	st.src = code
	if strings.Contains(code, "\r") {
		code = strings.ReplaceAll(strings.ReplaceAll(code, "\r\n", "\n"), "\r", "\n")
	}
	st.code = code
	st.ver++
	st.starts = append(st.starts[:0], 0)
	st.cells = st.cells[:0]
	prev := 0
	for i := 0; i < len(code); i++ {
		if code[i] == '\n' {
			st.cells = append(st.cells, int32(txvCells(code[prev:i])))
			prev = i + 1
			st.starts = append(st.starts, prev)
		}
	}
	st.cells = append(st.cells, int32(txvCells(code[prev:])))
	st.spans = nil
}

func (st *codeBlockState) line(i int) string {
	end := len(st.code)
	if i+1 < len(st.starts) {
		end = st.starts[i+1] - 1
	}
	return st.code[st.starts[i]:end]
}

func (w *CodeBlockW) build() {
	c := w.c
	th := c.ThemeFor(theme.CompCodeBlock)
	tk := &th.Components.CodeBlock
	loc := c.Locale()
	st := State[codeBlockState](c, w.b.n.id, nil)
	st.sync(w.code)
	// The highlighter document (rebuilt for new code, language or highlighter).
	h := w.hl
	if !w.hlSet {
		h = DefaultCodeHighlighter()
	}
	if st.docLang != w.lang || st.docVer != st.ver || st.docHL != h {
		st.docLang, st.docVer, st.docHL, st.spans, st.doc = w.lang, st.ver, h, nil, nil
		if h != nil && w.lang != "" {
			st.doc = h.Document(w.lang, st.code)
		}
	}
	if st.spansTh != th {
		st.spans, st.spansTh = nil, th
	}
	w.b.Bg(tk.Bg).Radius(tk.Radius).Clip()
	if tk.BorderColor.A > 0 {
		w.b.Border(1, tk.BorderColor)
	}
	copyNow := func() {
		w.copied = true
		st.copied = c.now
		if c.win != nil {
			c.win.WriteClipboard(st.code)
		}
		Message(c, "success", loc.Markdown.Copied)
	}
	copyIcon := icon.Copy
	if !st.copied.IsZero() && c.now.Sub(st.copied) < 2*time.Second {
		copyIcon = icon.Check
		c.WakeAt(st.copied.Add(2 * time.Second))
	}
	hovered := w.b.Hovered()
	if w.title != "" {
		hd := Row(c, func() {
			Text(c, w.title).Size(th.FontSizeSM).Color(tk.HeaderColor).Layout(layout.Grow(1))
			if w.copy && IconButton(c, copyIcon, loc.Markdown.CopyCode).Size(22, 14).Clicked() {
				copyNow()
			}
		}, layout.Middle(), layout.PadVH(4, tk.PaddingInline-4), layout.Padding(geom.Insets{Left: tk.PaddingInline, Right: 6, Top: 4, Bottom: 4}))
		if tk.HeaderBorder.A > 0 {
			hd.PaintOver(func(l *paint.List, r geom.Rect) {
				l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, tk.HeaderBorder)
			})
		}
	}
	v := newTxView(c, hashID(w.b.n.id, "view"))
	w.v = v
	lines := len(st.starts)
	v.count = lines
	v.line = st.line
	v.cells = func(i int) int { return int(st.cells[i]) }
	v.ver = st.ver
	v.wrap = w.wrap
	v.size, v.lineH = tk.CodeFontSize, tk.LineHeight
	v.padX, v.padY = tk.PaddingInline, tk.PaddingBlock
	v.fg, v.gutter, v.gutterB, v.selBg = tk.Text, tk.GutterColor, tk.GutterBorder, tk.SelectionBg
	v.name = w.label
	if v.name == "" {
		v.name = w.title
	}
	if v.name == "" {
		v.name = w.lang
	}
	if v.name == "" {
		v.name = loc.CodeBlock.Code
	}
	if w.numbers {
		start := w.start
		v.numbers = func(i int) int { return start + i }
		v.maxNo = start + lines - 1
		v.padX = min(v.padX, 10)
	}
	if len(w.marked) > 0 {
		start, marked, bg := w.start, w.marked, tk.LineHighlightBg
		v.lineBg = func(i int) theme.Color {
			if marked[start+i] {
				return bg
			}
			return theme.Color{}
		}
	}
	if st.doc != nil {
		doc := st.doc
		var buf []CodeSpan
		pending := false
		v.runs = func(i int, dst []txvRun) []txvRun {
			spans, ok := st.spans[i]
			if !ok {
				var final bool
				buf, final = doc.Line(i, buf[:0])
				spans = buf
				if final {
					if st.spans == nil || len(st.spans) > 4096 {
						st.spans = map[int][]CodeSpan{}
					}
					st.spans[i] = append([]CodeSpan(nil), buf...)
				} else if !pending {
					pending = true
					c.WakeAt(c.now.Add(codeHighlightRetry))
				}
			}
			for _, sp := range spans {
				dst = append(dst, txvRun{start: sp.Start, end: sp.End, fg: codeKindColor(tk, sp.Kind), italic: sp.Kind == CodeComment})
			}
			return dst
		}
	}
	// Height: the content, capped.
	rows := lines
	if w.wrap {
		v.syncWrap() // rows at the last painted width
		rows = v.rows()
	}
	contentH := float32(rows)*v.lineH + 2*v.padY
	if !w.wrap {
		contentH += 8 // room for the horizontal scrollbar
	}
	var opts []layout.Option
	switch {
	case w.height > 0:
		opts = append(opts, layout.H(w.height))
	case w.maxH > 0:
		opts = append(opts, layout.H(min(contentH, w.maxH)))
	default:
		opts = append(opts, layout.H(contentH))
	}
	// The code reads left to right in RTL layouts too.
	ConfigProvider(c).Direction(LTR).Body(func() {
		StackBox(c, func() {
			v.show("codeblock-view", append(opts, layout.Grow(1)))
			if w.copy && w.title == "" {
				d := Div(c, layout.Abs(layout.Unset, 6, 6, layout.Unset))
				focused := false
				d.Body(func() {
					b := IconButton(c, copyIcon, loc.Markdown.CopyCode).Size(24, 14)
					focused = b.ia.Focused
					if b.Clicked() {
						copyNow()
					}
				})
				// Shown while the block is hovered or the button focused.
				a := c.Anim(w.b.n.id, "copy", b2f(hovered || focused), th.DurationMid)
				d.Opacity(max(a, 0.001))
			}
		}, layout.Grow(1))
	})
}

// CodeBlockLocale holds the CodeBlock texts (the copy button uses
// Markdown.CopyCode and Markdown.Copied).
type CodeBlockLocale struct {
	_ [0]func()

	Code string // accessible name of a block without title or language
}
