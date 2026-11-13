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
	// foldControlW is the room of the folding controls, at the left of
	// the decorations margin (Monaco widens lineDecorationsWidth by 16
	// while folding is on and showFoldingControls is not "never").
	foldControlW = 16
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
	fw := float32(0)
	if w.foldControlsShown() {
		fw = foldControlW
	}
	if !w.opts.lineNumbers() {
		return noNumbersGutterW + fw
	}
	return gutterLeftPad + w.numbersWidth(d, n) + fw + lineDecorationsW
}

// foldControlsShown reports whether the gutter has room for the folding
// controls (folding on and showFoldingControls not "never", as Monaco
// lays out the gutter; a large file keeps the room, without controls).
func (w *ViewW) foldControlsShown() bool {
	return w.opts.Folding && w.opts.foldingControls() != FoldingControlsNever
}

// foldControlX returns the x of the folding controls' box (foldControlW
// wide) in a gutter at gx.
func (w *ViewW) foldControlX(gx float32, d digitGlyphs, n int) float32 {
	if !w.opts.lineNumbers() {
		return gx
	}
	return gx + gutterLeftPad + w.numbersWidth(d, n)
}

// Folding decorations of a row (paintFrame.fold, damage.go).
const (
	foldCtlExpanded  = 1 << iota // the control of an unfolded range
	foldCtlCollapsed             // the control of a folded region
	foldPlaceholder              // the folded line's "⋯" (its last row)
	foldBackground               // the folded line's background
)

// paintFoldControls draws the folding controls of the laid out rows (a
// line's control on its first row): a chevron pointing down for an
// unfolded range, right for a folded region (Monaco's codicons
// folding-expanded / folding-collapsed), in a box foldControlW wide at x.
func paintFoldControls(l *paint.List, x float32, rows []lineView, codes []uint8, lineY func(int) float32, lh float32, c color.NRGBA) {
	for k := range rows {
		code := codes[k]
		if code&(foldCtlExpanded|foldCtlCollapsed) == 0 {
			continue
		}
		cx := x + 2 + (foldControlW-2)/2 // Monaco's margin-left: 2px
		cy := lineY(rows[k].vi) + lh/2
		const a = 4 // half the chevron's width
		if code&foldCtlCollapsed != 0 {
			l.Line(geom.Pt(cx-a/2, cy-a), geom.Pt(cx+a/2, cy), 1.5, c)
			l.Line(geom.Pt(cx+a/2, cy), geom.Pt(cx-a/2, cy+a), 1.5, c)
		} else {
			l.Line(geom.Pt(cx-a, cy-a/2), geom.Pt(cx, cy+a/2), 1.5, c)
			l.Line(geom.Pt(cx, cy+a/2), geom.Pt(cx+a, cy-a/2), 1.5, c)
		}
	}
}

func (w *ViewW) numbersWidth(d digitGlyphs, n int) float32 {
	digits := max(len(strconv.Itoa(max(n, 1))), lineNumbersMinChars)
	return float32(math.Ceil(float64(float32(digits) * d.adv)))
}

// lineNumber returns what the gutter shows for model line i (0-based) of
// a document of n lines with the primary caret on caretLine (Monaco's
// _getLineRenderLineNumber): the number, whether there is one, and
// whether it is left aligned (relative mode's caret line).
func (o *Options) lineNumber(i, caretLine, n int) (num int, show, left bool) {
	switch o.lineNumbersMode() {
	case LineNumbersOff:
		return 0, false, false
	case LineNumbersRelative:
		if i == caretLine {
			return i + 1, true, true
		}
		return max(i-caretLine, caretLine-i), true, false
	case LineNumbersInterval:
		if i == caretLine || (i+1)%10 == 0 || i+1 == n {
			return i + 1, true, false
		}
		return 0, false, false
	}
	return i + 1, true, false
}

// paintGutter draws the gutter background, the current line's gutter
// highlight (rows marked in curG; renderLineHighlight "gutter" and
// "all") and the line numbers of the laid out rows: a wrapped line's
// number on its first row only (as Monaco); lineY is a view line's top.
// The caret's line uses the active color.
func (w *ViewW) paintGutter(l *paint.List, gx, gw float32, rows []lineView, lineY func(int) float32, n, caretLine int, d digitGlyphs, bg color.NRGBA, curG []bool, fill color.NRGBA, isFill bool, border color.NRGBA) {
	th := w.st.Theme
	l.Fill(rectXYWH(gx, w.vs.rect.Y, gw, w.vs.rect.H), geom.Radii{}, bg)
	lh := w.lineHeight()
	for k := range curG {
		if !curG[k] {
			continue
		}
		row := rectXYWH(gx, lineY(rows[k].vi), gw, lh)
		if isFill {
			l.Fill(row, geom.Radii{}, fill)
		} else {
			l.Stroke(row, geom.Radii{}, 2, border)
		}
	}
	if !w.opts.lineNumbers() {
		return
	}
	fg := defaultColor(th, "editorLineNumber.foreground")
	act := defaultColor(th, "editorLineNumber.activeForeground")
	left := gx + gutterLeftPad
	right := left + w.numbersWidth(d, n)
	var buf [20]byte
	var gl []paint.Glyph
	for k := range rows {
		lv := &rows[k]
		if lv.row != 0 || lv.i >= n {
			continue
		}
		num, show, leftAlign := w.opts.lineNumber(lv.i, caretLine, n)
		if !show {
			continue
		}
		s := strconv.AppendInt(buf[:0], int64(num), 10)
		gl = gl[:0]
		x := right - float32(len(s))*d.adv
		if leftAlign {
			x = left
		}
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
