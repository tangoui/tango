package editor

import (
	"image/color"
	"math"
	"strconv"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

// Gutter metrics, after Monaco's defaults: line numbers take at least
// lineNumbersMinChars digit widths, followed by the line decorations
// margin before the text.
const (
	lineNumbersMinChars = 5
	gutterLeftPad       = 8
	lineDecorationsW    = 18
	noNumbersGutterW    = 10 // only the decorations margin
)

// digitGlyphs are the glyphs of 0–9 in the document font: line numbers are
// composed from them instead of shaping a string per number.
type digitGlyphs struct {
	g   [10]paint.Glyph // X relative to the digit's own pen position
	adv float32         // widest digit advance
}

func digitsOf(st text.Style) digitGlyphs {
	var d digitGlyphs
	lay := text.Shape("0123456789", st, text.Options{NoWrap: true})
	for i := range 10 {
		x0, _, _ := lay.Caret(i)
		x1, _, _ := lay.Caret(i + 1)
		d.adv = max(d.adv, x1-x0)
	}
	for k, g := range lay.Glyphs {
		if k >= len(lay.GlyphCluster) {
			break
		}
		i := int(lay.GlyphCluster[k])
		if i < 0 || i > 9 {
			continue
		}
		x0, _, _ := lay.Caret(i)
		g.X -= x0
		d.g[i] = g
	}
	return d
}

// gutterWidth returns the width of the gutter for a document of n lines.
func (w *ViewW) gutterWidth(d digitGlyphs, n int) float32 {
	if !w.opts.lineNumbers() {
		return noNumbersGutterW
	}
	return gutterLeftPad + w.numbersWidth(d, n) + lineDecorationsW
}

func (w *ViewW) numbersWidth(d digitGlyphs, n int) float32 {
	digits := max(len(strconv.Itoa(max(n, 1))), lineNumbersMinChars)
	return float32(math.Ceil(float64(float32(digits) * d.adv)))
}

// paintGutter draws the gutter background and the line numbers of the
// laid out rows: a wrapped line's number on its first row only (as
// Monaco); lineY is a view line's top. The caret's line uses the active
// color.
func (w *ViewW) paintGutter(l *paint.List, gx, gw float32, rows []lineView, lineY func(int) float32, n, caretLine int, d digitGlyphs, bg color.NRGBA) {
	th := w.st.Theme
	l.Fill(rectXYWH(gx, w.vs.rect.Y, gw, w.vs.rect.H), geom.Radii{}, bg)
	if !w.opts.lineNumbers() {
		return
	}
	fg := defaultColor(th, "editorLineNumber.foreground")
	act := defaultColor(th, "editorLineNumber.activeForeground")
	right := gx + gutterLeftPad + w.numbersWidth(d, n)
	var buf [20]byte
	var gl []paint.Glyph
	for k := range rows {
		lv := &rows[k]
		if lv.row != 0 || lv.i >= n {
			continue
		}
		s := strconv.AppendInt(buf[:0], int64(lv.i+1), 10)
		gl = gl[:0]
		x := right - float32(len(s))*d.adv
		for k, ch := range s {
			g := d.g[ch-'0']
			g.X += x + float32(k)*d.adv
			gl = append(gl, g)
		}
		col := fg
		if lv.i == caretLine {
			col = act
		}
		l.Glyphs(geom.Pt(0, lineY(lv.vi)), append([]paint.Glyph(nil), gl...), col)
	}
}
