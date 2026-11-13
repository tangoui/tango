package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"strings"
	"unicode"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// RichText: a paragraph of mixed inline styles.
//
// The text package shapes one style per call, so RichText splits its spans
// into break pieces (Latin words, single CJK characters, spaces), measures
// each with its own style and does line breaking itself (with simple
// kinsoku: no break before closing / after opening punctuation). Runs of the
// same span on a line are shaped together and aligned on a shared baseline.

// Span is a run of styled inline text.
type Span struct {
	Text      string
	Bold      bool
	Italic    bool // synthetic oblique (text.Style.Italic)
	Strike    bool
	Underline bool
	Code      bool // inline code box, monospace
	Mono      bool // monospace without the box
	Mark      bool // highlighted background
	Link      string
	Color     theme.Color // zero: default color
	Size      float32     // 0: paragraph size
}

// RichTextW is a paragraph of spans.
type RichTextW struct {
	c       *Context
	n       *node
	spans   []Span
	size    float32
	col     theme.Color
	colSet  bool
	pre     bool
	align   TextAlign
	st      *moreRichState
	ia      Interaction
	links   bool
	clicked string
	hovered string
	cache   []*moreRichLayout
	sel     bool // selectable
	inter   bool // registered for hit testing
	z       *lazy
}

type moreRichState struct {
	links    []moreRichLink // absolute link rects of the last paint
	anc, cur int            // selection in display runes (anc == cur: none)
	origin   geom.Point     // paragraph origin of the last paint
	width    float32        // paragraph width of the last paint
	rtl      bool           // direction of the last paint (Start/End alignment)
}

type moreRichLink struct {
	url  string
	r    geom.Rect
	span int
}

// RichText adds a wrapping paragraph made of spans.
//
//	ui.RichText(c, ui.Span{Text: "普通、"}, ui.Span{Text: "加粗", Bold: true}, ui.Span{Text: "链接", Link: "https://…"})
func RichText(c *Context, spans ...Span) *RichTextW {
	var sb strings.Builder
	w := &RichTextW{c: c, spans: spans}
	for _, s := range spans {
		sb.WriteString(s.Text)
		if s.Link != "" {
			w.links = true
		}
	}
	w.n = c.add("richtext", sb.String())
	measure := func(maxW, _ float32) geom.Size {
		lay := w.layout(maxW)
		return geom.Size{W: lay.w, H: lay.h}
	}
	if w.links {
		// The link nodes (linkNodes) are absolute children; a measured
		// child holds the paragraph size since a node with children is
		// sized by them.
		w.n.ln.Dir = layout.Column
		body := &node{id: hashID(w.n.id, "body"), parent: w.n}
		body.ln.Ref = body
		body.ln.Measure = measure
		w.n.kids = append(w.n.kids, body)
	} else {
		w.n.ln.Measure = measure
	}
	w.n.paint = w.paint
	w.n.setRole(a11y.RoleParagraph)
	w.st = State[moreRichState](c, w.n.id, nil)
	if w.links {
		w.ia = c.interact(w.n, false, input.CursorDefault)
		w.inter = true
		w.n.aKbd = true // the links are focusable child nodes
	}
	w.z = c.lazy(w.n, w.finish)
	return w
}

// Selectable lets the user select the text with the pointer (double click
// selects a word, triple click the paragraph) and copy it with Cmd/Ctrl+C;
// Cmd/Ctrl+A selects the whole paragraph while it has focus.
func (w *RichTextW) Selectable() *RichTextW {
	w.sel = true
	w.n.ownKeys = true
	w.n.noTab = true // click to focus; Tab skips text
	if !w.inter {
		w.ia = w.c.interact(w.n, true, input.CursorText)
		w.inter = true
	} else if !w.n.disabled {
		w.n.focus = true
		w.n.cursor = input.CursorText
		if w.ia.PressedNow {
			w.c.SetFocus(w.n.id)
		}
		w.ia.Focused = w.c.focus == w.n.id
	}
	return w
}

// Selection returns the selected text ("" if none).
func (w *RichTextW) Selection() string {
	w.z.run()
	a, b := w.st.anc, w.st.cur
	if a > b {
		a, b = b, a
	}
	rs := w.displayRunes()
	a, b = max(0, min(a, len(rs))), max(0, min(b, len(rs)))
	return string(rs[a:b])
}

func (w *RichTextW) finish() {
	c := w.c
	st := w.st
	if !w.inter || w.n.disabled {
		return
	}
	w.linkNodes()
	link := ""
	if w.ia.Hovered || w.ia.Clicked {
		for _, l := range st.links {
			if l.r.Contains(c.pointer) {
				link = l.url
				break
			}
		}
	}
	if link != "" {
		w.hovered = link
		w.n.cursor = input.CursorPointer
	}
	if !w.sel {
		if w.ia.Clicked && link != "" {
			w.clicked = link
		}
		return
	}
	n := len(w.displayRunes())
	if w.ia.PressedNow {
		i := w.indexAt(c.pointer)
		switch {
		case w.ia.Clicks >= 3 || c.clickCount >= 3:
			st.anc, st.cur = 0, n
		case c.clickCount == 2:
			st.anc, st.cur = w.wordAt(i)
		default:
			st.anc, st.cur = i, i
		}
		c.invalid = true
	} else if c.Dragging(w.n.id) {
		if i := w.indexAt(c.pointer); i != st.cur {
			st.cur = i
			c.invalid = true
		}
	}
	if w.ia.Clicked && link != "" && st.anc == st.cur {
		w.clicked = link
	}
	if c.focus != w.n.id {
		return
	}
	for _, k := range c.KeysFor(w.n.id) {
		if !k.Down || !k.Mods.Shortcut() {
			continue
		}
		switch k.Key {
		case input.KeyA:
			st.anc, st.cur = 0, n
			c.invalid = true
		case input.KeyC:
			if s := w.Selection(); s != "" && c.win != nil {
				c.win.WriteClipboard(s)
			}
		}
	}
}

// linkNodes adds a focusable link node over the first painted piece of
// every link span (positions from the last paint), so links are announced
// and can be followed with Tab + Enter.
func (w *RichTextW) linkNodes() {
	c := w.c
	st := w.st
	if st.width == 0 && len(st.links) == 0 {
		c.invalid = true // not painted yet: build the link nodes next frame
	}
	seen := -1
	for _, l := range st.links {
		if l.span == seen || l.span < 0 || l.span >= len(w.spans) {
			continue
		}
		seen = l.span
		k := &node{id: hashID(w.n.id, "link"+itoaEnt(l.span)), parent: w.n, label: w.spans[l.span].Text}
		k.ln.Ref = k
		off := l.r.X - st.origin.X
		if st.rtl {
			off = st.width - off - l.r.W // Abs left is the start edge in RTL
		}
		k.ln.Style.Apply(layout.Abs(off, l.r.Y-st.origin.Y, layout.Unset, layout.Unset))
		k.ln.Width, k.ln.Height = l.r.W, l.r.H
		k.setRole(a11y.RoleLink)
		w.n.kids = append(w.n.kids, k)
		ia := c.interact(k, true, input.CursorPointer)
		k.over = func(pl *paint.List, r geom.Rect) {
			if ia.FocusVisible {
				entFocusRing(c, pl, r, 2)
			}
		}
		if ia.Hovered {
			w.hovered = l.url
		}
		if ia.Clicked {
			w.clicked = l.url
		}
	}
}

// Size sets the base font size.
func (w *RichTextW) Size(px float32) *RichTextW { w.size = px; w.cache = nil; return w }

// Color sets the default text color.
func (w *RichTextW) Color(col theme.Color) *RichTextW { w.col, w.colSet = col, true; return w }

// Pre keeps spaces and tabs and only breaks at newlines or when a line is
// too long (code blocks).
func (w *RichTextW) Pre() *RichTextW { w.pre = true; w.cache = nil; return w }

// Align aligns lines inside the paragraph box.
func (w *RichTextW) Align(a TextAlign) *RichTextW { w.align = a; return w }

// Layout applies layout options.
func (w *RichTextW) Layout(opts ...layout.Option) *RichTextW {
	w.n.ln.Style.Apply(opts...)
	return w
}

// ClickedLink returns the URL of a link clicked this frame ("" if none).
func (w *RichTextW) ClickedLink() string { w.z.run(); return w.clicked }

// HoveredLink returns the URL under the pointer ("" if none).
func (w *RichTextW) HoveredLink() string { w.z.run(); return w.hovered }

// ID returns the paragraph's widget ID.
func (w *RichTextW) ID() ID { return w.n.id }

// ---------------------------------------------------------------------------
// Layout

type moreRichPiece struct {
	text       string
	span       int
	start      int     // display rune offset
	w          float32 // text width
	padL, padR float32 // code box padding + margin at span edges
	space      bool
	brk        bool // hard line break
	glue       bool // no break allowed before this piece
}

type moreRichRun struct {
	text       string
	span       int
	start      int     // display rune offset of text[0]
	x, w       float32 // w includes padding
	padL, padR float32
}

type moreRichLine struct {
	runs      []moreRichRun
	y, h, asc float32
	w         float32
}

type moreRichLayout struct {
	maxW  float32
	lines []moreRichLine
	w, h  float32
}

func (w *RichTextW) baseSize() float32 {
	if w.size > 0 {
		return w.size
	}
	return w.c.Theme.FontSize
}

func (w *RichTextW) styleOf(i int) text.Style {
	sp := w.spans[i]
	size := w.baseSize()
	if sp.Size > 0 {
		size = sp.Size
	}
	if sp.Code {
		size = float32(int(size*0.86 + 0.5))
	}
	wt := text.Regular
	if sp.Bold {
		// <strong> renders at least semibold (Element's strong token is 500,
		// which many CJK fonts draw like regular).
		wt = text.Weight(max(w.c.Theme.FontWeightStrong, int(text.Semibold)))
	}
	st := w.c.style(size, wt)
	if sp.Code {
		// Code keeps the surrounding line height so mixed lines stay even.
		st.LineHeight = w.c.Theme.LineHeightFor(w.baseSize())
	}
	st.Mono = sp.Code || sp.Mono
	st.Italic = sp.Italic
	return st
}

const (
	moreClosePunct = "，。、；：？！）」』》〉】”’,.;:?!)]}…%" // i18n:ok line-breaking rules
	moreOpenPunct  = "（「『《〈【“‘([{"                // i18n:ok line-breaking rules
)

func moreIsCJK(r rune) bool {
	return (r >= 0x2E80 && r <= 0x9FFF) || (r >= 0xAC00 && r <= 0xD7AF) || (r >= 0xF900 && r <= 0xFAFF) ||
		(r >= 0xFF00 && r <= 0xFFEF) || (r >= 0x3000 && r <= 0x303F) || r >= 0x20000
}

// pieces splits the spans into break pieces.
func (w *RichTextW) pieces() []moreRichPiece {
	var out []moreRichPiece
	pos := 0 // display rune offset of the span start
	for si := range w.spans {
		st := w.styleOf(si)
		start := len(out)
		rs := w.spanRunes(si)
		for i := 0; i < len(rs); {
			at := pos + i
			r := rs[i]
			var p moreRichPiece
			switch {
			case r == '\n':
				p = moreRichPiece{brk: true}
				i++
			case r == ' ' || r == '\t' || r == '\r':
				j := i
				for j < len(rs) && (rs[j] == ' ' || rs[j] == '\t' || rs[j] == '\r') {
					j++
				}
				p = moreRichPiece{text: string(rs[i:j]), space: true}
				i = j
			case moreIsCJK(r) || strings.ContainsRune(moreClosePunct, r) || strings.ContainsRune(moreOpenPunct, r):
				p = moreRichPiece{text: string(r)}
				i++
			default:
				j := i
				for j < len(rs) && rs[j] != '\n' && !unicode.IsSpace(rs[j]) && !moreIsCJK(rs[j]) {
					j++
				}
				p = moreRichPiece{text: string(rs[i:j])}
				i = j
			}
			p.span, p.start = si, at
			switch {
			case p.space:
				// text.Width trims trailing spaces; measure them between glyphs.
				p.w = moreSpaceWidth(st) * float32(len([]rune(p.text)))
			case !p.brk:
				p.w = text.Width(p.text, st)
			}
			out = append(out, p)
		}
		pos += len(rs)
		if sp := w.spans[si]; sp.Code && len(out) > start {
			out[start].padL = 6
			out[len(out)-1].padR = 6
		}
	}
	// Glue: closing punctuation sticks to what precedes it, opening
	// punctuation to what follows, and Latin words continue across spans.
	for i := 1; i < len(out); i++ {
		p, q := &out[i], out[i-1]
		if p.brk || p.space || q.brk || q.space {
			continue
		}
		fr, _ := moreFirstRune(p.text)
		lr := moreLastRune(q.text)
		switch {
		case strings.ContainsRune(moreClosePunct, fr) && (moreIsCJK(fr) || len([]rune(p.text)) == 1):
			p.glue = true
		case strings.ContainsRune(moreOpenPunct, lr):
			p.glue = true
		case !moreIsCJK(fr) && !moreIsCJK(lr) && p.span != q.span:
			p.glue = true
		}
	}
	return out
}

func moreFirstRune(s string) (rune, bool) {
	for _, r := range s {
		return r, true
	}
	return 0, false
}

func moreLastRune(s string) rune {
	rs := []rune(s)
	if len(rs) == 0 {
		return 0
	}
	return rs[len(rs)-1]
}

func (w *RichTextW) layout(maxW float32) *moreRichLayout {
	for _, l := range w.cache {
		if l.maxW == maxW {
			return l
		}
	}
	lay := w.doLayout(maxW)
	if len(w.cache) >= 3 {
		w.cache = w.cache[1:]
	}
	w.cache = append(w.cache, lay)
	return lay
}

func (w *RichTextW) doLayout(maxW float32) *moreRichLayout {
	ps := w.pieces()
	// Split words that cannot fit any line into characters.
	var fixed []moreRichPiece
	for _, p := range ps {
		if !p.space && !p.brk && p.w+p.padL+p.padR > maxW && len([]rune(p.text)) > 1 {
			st := w.styleOf(p.span)
			rs := []rune(p.text)
			for k, r := range rs {
				q := moreRichPiece{text: string(r), span: p.span, start: p.start + k, w: text.Width(string(r), st)}
				if k == 0 {
					q.padL, q.glue = p.padL, p.glue
				}
				if k == len(rs)-1 {
					q.padR = p.padR
				}
				fixed = append(fixed, q)
			}
			continue
		}
		fixed = append(fixed, p)
	}
	ps = fixed
	lay := &moreRichLayout{maxW: maxW}
	var cur []moreRichPiece
	x := float32(0)
	flush := func() {
		lay.lines = append(lay.lines, w.makeLine(cur))
		cur, x = nil, 0
	}
	for i := 0; i < len(ps); {
		p := ps[i]
		if p.brk {
			flush()
			i++
			continue
		}
		if p.space && len(cur) == 0 && !w.pre {
			i++
			continue
		}
		// The glue group starting at i.
		j := i + 1
		gw := p.w + p.padL + p.padR
		for j < len(ps) && ps[j].glue {
			gw += ps[j].w + ps[j].padL + ps[j].padR
			j++
		}
		if len(cur) > 0 && x+gw > maxW+0.01 && !p.space {
			if gw > maxW {
				// A group wider than a line: break inside it.
				for k := i; k < j; k++ {
					q := ps[k]
					qw := q.w + q.padL + q.padR
					if len(cur) > 0 && x+qw > maxW+0.01 && !(k > i && moreIsClosePunct(q.text)) {
						flush()
					}
					cur = append(cur, q)
					x += qw
				}
				i = j
				continue
			}
			flush()
		}
		for k := i; k < j; k++ {
			cur = append(cur, ps[k])
			x += ps[k].w + ps[k].padL + ps[k].padR
		}
		i = j
	}
	if len(cur) > 0 || len(lay.lines) == 0 {
		flush()
	}
	y := float32(0)
	for i := range lay.lines {
		ln := &lay.lines[i]
		ln.y = y
		y += ln.h
		lay.w = max(lay.w, ln.w)
	}
	lay.h = y
	return lay
}

// makeLine merges pieces of the same span into runs and computes the
// shared baseline.
func (w *RichTextW) makeLine(ps []moreRichPiece) moreRichLine {
	if !w.pre {
		for len(ps) > 0 && ps[len(ps)-1].space {
			ps = ps[:len(ps)-1]
		}
	}
	var ln moreRichLine
	x := float32(0)
	for _, p := range ps {
		pw := p.w + p.padL + p.padR
		if n := len(ln.runs); n > 0 && ln.runs[n-1].span == p.span && ln.runs[n-1].padR == 0 && p.padL == 0 {
			r := &ln.runs[n-1]
			r.text += p.text
			r.w += pw
			r.padR = p.padR
		} else {
			ln.runs = append(ln.runs, moreRichRun{text: p.text, span: p.span, start: p.start, x: x, w: pw, padL: p.padL, padR: p.padR})
		}
		x += pw
	}
	ln.w = x
	// Baseline: the largest baseline offset; height covers every run's line box.
	base := w.c.style(w.baseSize(), text.Regular)
	ln.asc = moreBaseOff(base)
	desc := base.LineHeight - ln.asc
	for _, r := range ln.runs {
		st := w.styleOf(r.span)
		bo := moreBaseOff(st)
		ln.asc = max(ln.asc, bo)
		desc = max(desc, st.LineHeight-bo)
	}
	ln.h = max(ln.h, ln.asc+desc)
	return ln
}

// ---------------------------------------------------------------------------
// Paint

func (w *RichTextW) paint(l *paint.List, r geom.Rect) {
	th := w.c.Theme
	lay := w.layout(r.W + 0.5)
	w.st.links = w.st.links[:0]
	w.st.origin, w.st.width, w.st.rtl = r.Min(), r.W, w.c.IsRTL()
	selA, selB := w.st.anc, w.st.cur
	if selA > selB {
		selA, selB = selB, selA
	}
	showSel := w.sel && selA < selB && w.c.focus == w.n.id
	selCol := theme.Fade(th.Primary.Base, 0.22)
	def := th.Text
	if w.colSet {
		def = w.col
	}
	if w.n.disabled {
		def = th.TextDisabled
	}
	for _, ln := range lay.lines {
		top := r.Y + ln.y
		baseline := top + ln.asc
		shift := w.lineShift(&ln, r.W)
		for _, run := range ln.runs {
			sp := w.spans[run.span]
			st := w.styleOf(run.span)
			x := r.X + shift + run.x
			col := def
			if sp.Color.A > 0 {
				col = sp.Color
			}
			if sp.Link != "" {
				col = th.TextLink
				if w.hovered == sp.Link {
					col = th.TextLinkHover
				}
			}
			txt := run.text
			if !w.pre {
				txt = strings.TrimRight(txt, " ")
			}
			shaped := text.Shape(txt, st, text.Options{NoWrap: true})
			tw := shaped.Width
			if n := len([]rune(run.text)); showSel && selA < run.start+n && selB > run.start {
				full := text.Shape(run.text, st, text.Options{NoWrap: true})
				x0, _, _ := full.Caret(max(0, selA-run.start))
				x1, _, _ := full.Caret(min(n, selB-run.start))
				l.Fill(geom.Rect{X: x + run.padL + x0, Y: top, W: x1 - x0, H: ln.h}, geom.Radii{}, selCol)
			}
			if sp.Mark {
				// RichText's tint (a warning shade) follows the theme's
				// lightness, unlike the Label MarkBg (Typography token).
				tk := &w.c.ThemeFor(theme.CompTypography).Components.Typography
				mh := st.Size + 6
				l.Fill(geom.Rect{X: x, Y: baseline - st.Size*0.36 - mh/2, W: run.w, H: mh}, geom.Radii{}, tk.HighlightBg)
				if tk.HighlightColor.A > 0 {
					col = tk.HighlightColor
				}
			}
			if sp.Code {
				bx := x
				bw := run.w
				if run.padL > 0 {
					bx += 2
					bw -= 2
				}
				if run.padR > 0 {
					bw -= 2
				}
				h := st.Size + 8
				box := geom.Rect{X: bx, Y: baseline - st.Size*0.36 - h/2, W: bw, H: h}
				if th.Flavor == theme.StyleShadcn {
					l.Fill(box, geom.Rad(th.RadiusSM), th.FillSecondary) // bg-muted rounded
				} else {
					l.Fill(box, geom.Rad(3), theme.RGBA(150, 150, 150, .1))
					l.Stroke(box, geom.Rad(3), 1, theme.RGBA(100, 100, 100, .2))
				}
			}
			tx := x + run.padL
			l.Glyphs(geom.Pt(tx, baseline-moreBaseOff(st)), shaped.Glyphs, col)
			if sp.Underline || (sp.Link != "" && w.hovered == sp.Link && th.Flavor == theme.Element) || (sp.Link != "" && th.Flavor == theme.StyleShadcn) {
				l.Fill(geom.Rect{X: tx, Y: baseline + 2, W: tw, H: 1}, geom.Radii{}, col)
			}
			if sp.Strike {
				l.Fill(geom.Rect{X: tx, Y: baseline - st.Size*0.33, W: tw, H: 1}, geom.Radii{}, col)
			}
			if sp.Link != "" {
				w.st.links = append(w.st.links, moreRichLink{url: sp.Link, r: geom.Rect{X: x, Y: top, W: run.w, H: ln.h}, span: run.span})
			}
		}
	}
}

// PlainText returns the concatenated text of spans.
func PlainText(spans []Span) string {
	var sb strings.Builder
	for _, s := range spans {
		sb.WriteString(s.Text)
	}
	return sb.String()
}

// moreSpaceWidth is the advance of one space in st.
func moreSpaceWidth(st text.Style) float32 {
	return text.Width("x x", st) - text.Width("xx", st)
}

// moreBaseOff is the baseline offset from the top of a line box of st, as
// placed by text.Shape (half-leading + ascent).
func moreBaseOff(st text.Style) float32 {
	lay := text.Shape("中", st, text.Options{NoWrap: true}) // i18n:ok CJK probe glyph for metrics
	if len(lay.Glyphs) > 0 {
		return lay.Glyphs[0].Y
	}
	return lay.Ascent
}

// moreIsClosePunct reports a single closing punctuation piece.
func moreIsClosePunct(s string) bool {
	rs := []rune(s)
	return len(rs) == 1 && strings.ContainsRune(moreClosePunct, rs[0])
}

// spanRunes is span i's display text: tabs expand to four spaces in Pre
// mode and become single spaces otherwise; CR is dropped to a space.
func (w *RichTextW) spanRunes(i int) []rune {
	s := w.spans[i].Text
	if w.pre {
		s = strings.ReplaceAll(s, "\t", "    ")
	} else {
		s = strings.ReplaceAll(s, "\t", " ")
	}
	return []rune(strings.ReplaceAll(s, "\r", " "))
}

// displayRunes is the paragraph's display text; selections index it.
func (w *RichTextW) displayRunes() []rune {
	var out []rune
	for i := range w.spans {
		out = append(out, w.spanRunes(i)...)
	}
	return out
}

// lineShift is the alignment offset of a line in a paragraph of width rw.
// Start and End follow the paragraph's scope direction (as CSS text-align
// start/end follow the element's direction), recorded at paint; runs
// themselves are still placed left to right.
func (w *RichTextW) lineShift(ln *moreRichLine, rw float32) float32 {
	return alignLine(w.align, rw, ln.w, w.st.rtl)
}

// indexAt maps an absolute point to a display rune offset using the last
// painted geometry.
func (w *RichTextW) indexAt(p geom.Point) int {
	st := w.st
	if st.width <= 0 {
		return 0
	}
	lay := w.layout(st.width + 0.5)
	if len(lay.lines) == 0 {
		return 0
	}
	x, y := p.X-st.origin.X, p.Y-st.origin.Y
	li := len(lay.lines) - 1
	for i, ln := range lay.lines {
		if y < ln.y+ln.h {
			li = i
			break
		}
	}
	if y < 0 {
		li = 0
	}
	ln := &lay.lines[li]
	x -= w.lineShift(ln, st.width)
	if len(ln.runs) == 0 {
		return w.lineStart(lay, li)
	}
	if x <= ln.runs[0].x {
		return ln.runs[0].start
	}
	for _, r := range ln.runs {
		if x < r.x+r.w {
			sh := text.Shape(r.text, w.styleOf(r.span), text.Options{NoWrap: true})
			return r.start + sh.IndexAt(x-r.x-r.padL, 0)
		}
	}
	last := ln.runs[len(ln.runs)-1]
	return last.start + len([]rune(last.text))
}

// lineStart is the display offset where line li begins (for empty lines,
// right after the previous content).
func (w *RichTextW) lineStart(lay *moreRichLayout, li int) int {
	for i := li; i >= 0; i-- {
		if rs := lay.lines[i].runs; len(rs) > 0 {
			if i == li {
				return rs[0].start
			}
			r := rs[len(rs)-1]
			return r.start + len([]rune(r.text)) + (li - i)
		}
	}
	return li
}

// wordAt returns the word (or single CJK character / space run) around i.
func (w *RichTextW) wordAt(i int) (int, int) {
	rs := w.displayRunes()
	if len(rs) == 0 {
		return 0, 0
	}
	i = max(0, min(i, len(rs)-1))
	class := func(r rune) int {
		switch {
		case unicode.IsSpace(r):
			return 0
		case moreIsCJK(r):
			return 1
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_':
			return 2
		}
		return 3
	}
	k := class(rs[i])
	if k == 1 {
		return i, i + 1
	}
	a, b := i, i+1
	for a > 0 && class(rs[a-1]) == k {
		a--
	}
	for b < len(rs) && class(rs[b]) == k {
		b++
	}
	return a, b
}
