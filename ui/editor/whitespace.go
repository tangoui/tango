package editor

import (
	"image/color"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

// Whitespace and control characters (Monaco's renderWhitespace and
// renderControlCharacters, viewLineRenderer.js).
//
// A blank drawn as a mark keeps its place and width in the text: the View
// draws a middle dot (U+00B7) over a space and a rightwards arrow (U+2192)
// at the start of a tab, in editorWhitespace.foreground. Which blanks are
// marked follows Monaco's _applyRenderWhitespace, per row (a wrapped
// line's rows each on their own, as Monaco's view lines): "boundary" marks
// the leading and trailing blanks, tabs and runs of two or more spaces;
// "selection" the blanks inside a selection; "trailing" the blanks after
// the row's last non-blank on a row that does not continue on the next one
// (every blank of a blank row).
//
// A control character (U+0000–U+001F but tab, U+007F) is shaped as its
// Control Pictures symbol (U+2400 + code, U+2421 for DEL), one character
// for one, so offsets and carets are unchanged, and drawn white on
// rgb(150, 0, 0) (Monaco's .mtkcontrol). Monaco also shows the bidi
// formatting characters (U+202A–U+202E, U+2066–U+2069, U+200E, U+200F,
// U+061C) as "[U+XXXX]"; the View leaves them to the shaper.

// Control character colors (Monaco's .mtkcontrol).
var (
	controlFG = color.NRGBA{0xff, 0xff, 0xff, 0xff}
	controlBG = color.NRGBA{150, 0, 0, 0xff}
)

func isControlByte(b byte) bool { return b < 0x20 && b != '\t' || b == 0x7f }

func hasControl(s string) bool {
	for i := 0; i < len(s); i++ {
		if isControlByte(s[i]) {
			return true
		}
	}
	return false
}

// controlPictures returns s with its control characters replaced by their
// Control Pictures symbols.
func controlPictures(s string) string {
	buf := make([]byte, 0, len(s)+16)
	for i := 0; i < len(s); i++ {
		b := s[i]
		switch {
		case b == 0x7f:
			buf = utf8.AppendRune(buf, 0x2421)
		case isControlByte(b):
			buf = utf8.AppendRune(buf, 0x2400+rune(b))
		default:
			buf = append(buf, b)
		}
	}
	return string(buf)
}

// controlSpans returns the control characters of s (bytes [b0, b0+len(s))
// of its line) as white spans.
func controlSpans(s string, b0 int, dst []brSpan) []brSpan {
	for i := 0; i < len(s); i++ {
		if isControlByte(s[i]) {
			dst = append(dst, brSpan{b0 + i, b0 + i + 1, controlFG})
		}
	}
	return dst
}

// paintControlBackgrounds fills the boxes of row lv's control characters
// (y is its top).
func (w *ViewW) paintControlBackgrounds(l *paint.List, lv *lineView, tx, y, lh float32) {
	for _, sg := range lv.segs {
		s := lv.text[sg.b0:sg.b1]
		if !hasControl(s) {
			continue
		}
		r := sg.r0
		for _, ch := range s {
			if ch < utf8.RuneSelf && isControlByte(byte(ch)) {
				for _, sr := range sg.lay.SelectionRects(r-sg.r0, r-sg.r0+1) {
					l.Fill(rectXYWH(tx+sg.x0+sr.X, y, sr.W, lh), geom.Radii{}, controlBG)
				}
			}
			r++
		}
	}
}

// whitespaceMarks holds the shaped marks, per font size.
type whitespaceMarks struct {
	size       float32
	dot, arrow []paint.Glyph
}

func (w *ViewW) marks() *whitespaceMarks {
	vs := w.vs
	if sz := w.opts.fontSize(); vs.wsMarks == nil || vs.wsMarks.size != sz {
		st := w.monoStyle()
		vs.wsMarks = &whitespaceMarks{
			size:  sz,
			dot:   text.Shape("·", st, text.Options{NoWrap: true}).Glyphs,
			arrow: text.Shape("→", st, text.Options{NoWrap: true}).Glyphs,
		}
	}
	return vs.wsMarks
}

func isBlank(b byte) bool { return b == ' ' || b == '\t' }

// whitespaceMarked returns the bytes of row (a row of a line, starting at
// byte b0 of it) drawn as whitespace marks in mode: sels are selections
// as line byte ranges ("selection"), last whether the row ends its line
// ("trailing"). It follows Monaco's _applyRenderWhitespace.
func whitespaceMarked(row string, b0 int, mode RenderWhitespaceMode, sels [][2]int, last bool, dst []int) []int {
	switch {
	case mode == RenderWhitespaceNone,
		mode == RenderWhitespaceSelection && len(sels) == 0,
		mode == RenderWhitespaceTrailing && !last:
		return dst
	}
	first, lastNB := -1, -1
	for i := 0; i < len(row); i++ {
		if !isBlank(row[i]) {
			first = i
			break
		}
	}
	blankRow := first < 0
	if blankRow {
		first, lastNB = len(row), len(row)
	} else {
		for i := len(row) - 1; i >= 0; i-- {
			if !isBlank(row[i]) {
				lastNB = i
				break
			}
		}
	}
	was := false
	for k := 0; k < len(row); k++ {
		ch := row[k]
		ws := false
		switch {
		case !isBlank(ch):
		case k < first || k > lastNB, ch == '\t':
			ws = true
		case mode == RenderWhitespaceBoundary:
			ws = was || k+1 < len(row) && isBlank(row[k+1])
		default:
			ws = true
		}
		if ws && mode == RenderWhitespaceSelection {
			ws = false
			for _, s := range sels {
				if s[0] <= b0+k && b0+k < s[1] {
					ws = true
					break
				}
			}
		}
		if ws && mode == RenderWhitespaceTrailing {
			ws = blankRow || k > lastNB
		}
		was = ws
		if ws {
			dst = append(dst, b0+k)
		}
	}
	return dst
}

// paintWhitespace draws the whitespace marks of row lv (y is its top; vis
// are the frame's selections, for "selection").
func (w *ViewW) paintWhitespace(l *paint.List, snap buffer.Snapshot, lv *lineView, vis []visSel, tx, y float32, c color.NRGBA) {
	mode := w.opts.renderWhitespace()
	if mode == RenderWhitespaceNone || c.A == 0 {
		return
	}
	b0, b1 := lv.b0, lv.b1
	if lv.long != nil {
		b0, b1 = 0, len(lv.text)
	}
	ls := snap.LineStart(lv.i)
	var sels [][2]int
	if mode == RenderWhitespaceSelection {
		for _, x := range vis {
			if x.a == x.b || x.b < ls+b0 || x.a > ls+b1 {
				continue
			}
			sels = append(sels, [2]int{x.a - ls, x.b - ls})
		}
	}
	marked := whitespaceMarked(lv.text[b0:b1], b0, mode, sels, lv.last(), nil)
	if len(marked) == 0 {
		return
	}
	mk := w.marks()
	var out []paint.Glyph
	m := 0
	for _, sg := range lv.segs {
		for m < len(marked) && marked[m] < sg.b0 {
			m++
		}
		pos, r := sg.b0, sg.r0
		for ; m < len(marked) && marked[m] < sg.b1; m++ {
			at := marked[m]
			r += utf8.RuneCountInString(lv.text[pos:at])
			pos = at
			x, _, _ := sg.lay.Caret(r - sg.r0)
			g := mk.dot
			if lv.text[at] == '\t' {
				g = mk.arrow
			}
			for _, gl := range g {
				gl.X += sg.x0 + x
				out = append(out, gl)
			}
		}
	}
	if len(out) > 0 {
		l.Glyphs(geom.Pt(tx, y), out, c)
	}
}
