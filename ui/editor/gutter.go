package editor

import (
	"image/color"
	"math"
	"strconv"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

// GutterClick reports a zero-based document Line and the pointer Position
// in UI logical coordinates. Button is ButtonLeft or ButtonRight.
type GutterClick struct {
	Line     int
	Position geom.Point
	Button   input.Button
}

// GutterLineAtPoint maps a point in the laid-out gutter to a zero-based
// document line. Wrapped rows share a line. Padding and view zones have
// no line. Sticky rows report the line they show.
func (w *ViewW) GutterLineAtPoint(p geom.Point) (int, bool) {
	if !w.hasGeom() || w.vs.buf != w.st.Model.Buffer() || !w.vs.gutter.Contains(p) {
		return 0, false
	}
	for i, clip := range w.vs.stickyClips {
		if clip.Y <= p.Y && p.Y < clip.Bottom() {
			return w.vs.stickyLines[i], true
		}
	}
	v := w.pointRow(p)
	vl := w.st.lines()
	if v < 0 || v >= vl.Count() {
		return 0, false
	}
	line, row := vl.find(v)
	return line, 0 <= row && row < vl.rowsOf(line)
}

// OnGutterClick handles primary clicks and secondary presses in the
// gutter. Handled clicks leave the selection in place. An active folding
// control still receives primary clicks. Call Changed during build to
// handle the event there, for example to open a context menu.
func (w *ViewW) OnGutterClick(fn func(GutterClick)) *ViewW {
	w.gutterClick = fn
	return w
}

func (w *ViewW) gutterInput(p pending, rail bool) bool {
	vs := w.vs
	fn := w.gutterClick
	if fn == nil {
		vs.gutterPressLine = 0
		return false
	}
	if p.gutterContext {
		if line, ok := w.GutterLineAtPoint(p.gutterPos); ok {
			fn(GutterClick{Line: line, Position: p.gutterPos, Button: input.ButtonRight})
		}
	}
	if p.press {
		vs.gutterPressLine = 0
		if line, ok := w.GutterLineAtPoint(p.pressPos); ok && !rail {
			if _, fold := w.foldControlAt(p.pressPos); !fold {
				vs.gutterPressLine = line + 1
			}
		}
	}
	start := vs.gutterPressLine
	if p.cancel || p.release {
		vs.gutterPressLine = 0
	}
	if start == 0 {
		return false
	}
	if p.release && !p.cancel && !rail {
		if line, ok := w.GutterLineAtPoint(p.relPos); ok && line+1 == start {
			fn(GutterClick{Line: line, Position: p.relPos, Button: input.ButtonLeft})
		}
	}
	return true
}

// Gutter metrics, after Monaco's defaults: line numbers take at least
// lineNumbersMinChars digit widths, followed by the line decorations
// margin before the text.
const (
	lineNumbersMinChars = 5
	lineDecorationsW    = 10
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
		return lineDecorationsW + fw
	}
	return w.numbersWidth(d, n) + fw + lineDecorationsW
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
	return gx + w.numbersWidth(d, n)
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
	if w.lnFn != nil {
		digits = max(w.lnChars, lineNumbersMinChars) // LineNumbersFunc
	}
	return float32(math.Round(float64(float32(digits) * d.adv)))
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
		// Deviation: Monaco compares with the view model's getLineCount(),
		// the count of view lines (a folded line counts 0, each row of a
		// wrapped line 1), so with folds or wrapping its "last line" test
		// rarely hits the document's last line. Tango deliberately uses
		// the document's line count n: the last line shows its number.
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
// The caret's line uses the active color. ud are the rows' Model
// decorations (nil for none): their margin backgrounds, marks and line
// number colors.
func (w *ViewW) paintGutter(l *paint.List, gx, gw float32, rows []lineView, ud []udRow, lineY func(int) float32, n, caretLine int, d digitGlyphs, bg color.NRGBA, curG, cur []bool, hl lineHighlightStyle) {
	th := w.st.Theme
	l.Fill(rectXYWH(gx, w.vs.rect.Y, gw, w.vs.rect.H), geom.Radii{}, bg)
	lh := w.lineHeight()
	// The decorations' margin backgrounds (decorations.go), under the
	// current line's.
	for k := range ud {
		if len(ud[k].ds) == 0 {
			continue
		}
		margin, _, _ := gutterDecos(ud[k].ds, nil)
		for _, c := range margin {
			l.Fill(rectXYWH(gx, lineY(rows[k].vi), gw, lh), geom.Radii{}, c)
		}
	}
	for k := range curG {
		if !curG[k] {
			continue
		}
		row := rectXYWH(gx, lineY(rows[k].vi), gw, lh)
		if hl.fill.A > 0 {
			l.Fill(row, geom.Radii{}, hl.fill)
		} else {
			if cur != nil && cur[k] {
				row.W += hl.width // clipped at the join with the text highlight
			}
			l.Stroke(row, geom.Radii{}, hl.width, hl.border)
		}
	}
	w.paintGutterMarks(l, gx, gw, rows, ud, lineY, n, d)
	if !w.opts.lineNumbers() {
		return
	}
	fg := defaultColor(th, "editorLineNumber.foreground")
	act := defaultColor(th, "editorLineNumber.activeForeground")
	left := gx
	right := left + w.numbersWidth(d, n)
	var buf [20]byte
	var gl []paint.Glyph
	for k := range rows {
		lv := &rows[k]
		if lv.row != 0 || lv.i >= n {
			continue
		}
		var s []byte
		leftAlign := false
		if w.lnFn != nil {
			label, ok := w.lnFn(lv.i)
			if !ok || label == "" {
				continue
			}
			s = append(buf[:0], label...)
		} else {
			num, show, la := w.opts.lineNumber(lv.i, caretLine, n)
			if !show {
				continue
			}
			s, leftAlign = strconv.AppendInt(buf[:0], int64(num), 10), la
		}
		gl = gl[:0]
		if !allDigits(s) {
			// A label of other characters (LineNumbersFunc): shaped, right
			// aligned.
			lay := w.sign(string(s))
			for _, g := range lay.Glyphs {
				g.X += right - lay.Width
				gl = append(gl, g)
			}
		} else {
			x := right - float32(len(s))*d.adv
			if leftAlign {
				x = left
			}
			for k, ch := range s {
				g := d.g[ch-'0']
				g.X += x + float32(k)*d.adv
				gl = append(gl, g)
			}
		}
		col := fg
		if lv.i == caretLine {
			col = act
		}
		if k < len(ud) && len(ud[k].ds) > 0 {
			if _, c, _ := gutterDecos(ud[k].ds, nil); c.A > 0 {
				col = c
			}
		}
		l.Glyphs(geom.Pt(0, lineY(lv.vi)), append([]paint.Glyph(nil), gl...), col)
	}
}

// decoMargin returns the x and width of the line decorations margin of a
// gutter at gx (after the folding controls' box).
func (w *ViewW) decoMargin(gx, gw float32, d digitGlyphs, n int) (x, width float32) {
	fw := float32(0)
	if w.foldControlsShown() {
		fw = foldControlW
	}
	if !w.opts.lineNumbers() {
		return gx + fw, max(0, gw-fw)
	}
	x = w.foldControlX(gx, d, n) + fw
	return x, max(0, min(lineDecorationsW, gx+gw-x))
}

// signKey is a shaped gutter sign: its text at a font size.
type signKey struct {
	s    string
	size float32
}

// sign returns the shaped gutter sign s.
func (w *ViewW) sign(s string) *text.Layout {
	vs := w.vs
	k := signKey{s, w.opts.fontSize()}
	if lay := vs.signs[k]; lay != nil {
		return lay
	}
	if vs.signs == nil || len(vs.signs) > 64 {
		vs.signs = map[signKey]*text.Layout{}
	}
	lay := text.Shape(s, w.monoStyle(), text.Options{NoWrap: true})
	vs.signs[k] = lay
	return lay
}

// paintGutterMarks draws the decorations' marks in the line decorations
// margin: the cell's background, the bar at its left edge and the sign
// (on the line's first row).
func (w *ViewW) paintGutterMarks(l *paint.List, gx, gw float32, rows []lineView, ud []udRow, lineY func(int) float32, n int, d digitGlyphs) {
	if ud == nil {
		return
	}
	mx, mw := w.decoMargin(gx, gw, d, n)
	if mw <= 0 {
		return
	}
	lh := w.lineHeight()
	var marks []*GutterMark
	for k := range ud {
		if len(ud[k].ds) == 0 {
			continue
		}
		_, _, marks = gutterDecos(ud[k].ds, marks[:0])
		y := lineY(rows[k].vi)
		for _, m := range marks {
			if m.Background.A > 0 {
				l.Fill(rectXYWH(mx, y, mw, lh), geom.Radii{}, m.Background)
			}
			if m.Bar.A > 0 {
				l.Fill(rectXYWH(mx, y, min(3, mw), lh), geom.Radii{}, m.Bar)
			}
			if m.Sign != "" && m.SignColor.A > 0 && rows[k].row == 0 {
				lay := w.sign(m.Sign)
				x := float32(math.Round(float64(mx + (mw-lay.Width)/2)))
				l.Glyphs(geom.Pt(x, y), lay.Glyphs, m.SignColor)
			}
		}
	}
}

// allDigits reports whether s is ASCII digits only.
func allDigits(s []byte) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// LineNumberFunc returns the line number label of model line line
// (zero-based); ok false (or an empty label) shows none.
//
// Experimental: part of the diff view's editor API (docs/specs/diff-view.md).
type LineNumberFunc func(line int) (label string, ok bool)

// LineNumbersFunc shows fn's labels instead of the line numbers this
// frame, while line numbers are shown (Options.LineNumbers not off): right
// aligned, the caret's line in the active color (the relative and
// interval modes do not apply). The numbers' column is maxChars digits
// wide (at least lineNumbersMinChars, 5); gen tells the View that the
// labels changed: increment it whenever fn would return other labels for
// the same lines (the whole view is redrawn then). nil fn restores the
// numbers.
//
// Experimental: part of the diff view's editor API (docs/specs/diff-view.md).
func (w *ViewW) LineNumbersFunc(fn LineNumberFunc, maxChars int, gen uint64) *ViewW {
	w.lnFn, w.lnChars, w.lnGen = fn, maxChars, gen
	return w
}
