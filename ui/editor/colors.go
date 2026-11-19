package editor

import (
	"fmt"
	"image/color"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/geom"
	uitheme "github.com/tangoui/tango/ui/theme"
)

// Color decorators (Monaco's ColorDetector with its default document color
// provider, defaultDocumentColorsComputer.js):
//
// With Options.ColorDecorators, the CSS color literals of each drawn row's
// model line are found as Monaco's computeColors does: "#" and 3, 4, 6 or 8
// hex digits not followed by a word character, at the line start or after
// a quote or white space; rgb(), rgba(), hsl() and hsla() (after a word
// boundary) whose arguments Monaco's regular expressions accept (commas or
// blanks between them, "/" before the alpha of the CSS Level 4 syntax). A
// literal gets a swatch: a 0.8em square of its color with a 0.1em border
// (black, #eee on dark themes: Monaco's .colorpicker-color-decoration).
//
// As Monaco's `before` injected text, the swatch has a gap of its own
// before the literal, colorSwatchW (round(1.2em): the box and its 0.2em
// margins) wide, pushing the literal and the rest of its line aside; carets
// never stop inside it and word wrap counts it (injected.go). Only the
// swatch itself takes the press; a press on the gap's margins places the
// caret at the literal's start.
//
// A press on a swatch opens a color picker (ui.ColorPickerPanel in an
// overlay below the swatch, following it as the view scrolls) instead of
// moving the caret. Each frame in which the picked color changed replaces
// the literal (Monaco writes the color when the pointer is released over
// its picker; here dragging in the picker rewrites the text frame by
// frame), written in the presentation Monaco's picker
// chooses for it (ColorPickerModel.guessColorPresentation over the default
// provider's rgb / hsl / hex presentations); all the changes made while the
// picker is open are one undo step. The picker closes on Escape, a press
// outside it, the swatch scrolling out of view, or a change of the text by
// anything else. A read-only view
// shows the picker disabled. Long lines are not searched; models over
// Monaco's worker sync limit have no color decorators (largefile.go).

// colorSpan is a color literal of a line: its bytes and color.
type colorSpan struct {
	a, b int
	c    color.NRGBA
}

var (
	reRGB  = regexp.MustCompile(`^\(\s*(25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9][0-9]|[0-9])\s*[\s,]\s*(25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9][0-9]|[0-9])\s*[\s,]\s*(25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9][0-9]|[0-9])\s*\)$`)
	reRGBA = regexp.MustCompile(`^\(\s*(25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9][0-9]|[0-9])\s*[\s,]\s*(25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9][0-9]|[0-9])\s*[\s,]\s*(25[0-5]|2[0-4][0-9]|1[0-9]{2}|[1-9][0-9]|[0-9])\s*(?:[\s,]|[\s]*\/)\s*(0[.][0-9]+|[.][0-9]+|[01][.]|[01])\s*\)$`)
	reHSL  = regexp.MustCompile(`^\(\s*((?:360(?:\.0+)?|(?:36[0]|3[0-5][0-9]|[12][0-9][0-9]|[1-9]?[0-9])(?:\.\d+)?))\s*[\s,]\s*(100(?:\.0+)?|\d{1,2}[.]\d*|\d{1,2})%\s*[\s,]\s*(100(?:\.0+)?|\d{1,2}[.]\d*|\d{1,2})%\s*\)$`)
	reHSLA = regexp.MustCompile(`^\(\s*((?:360(?:\.0+)?|(?:36[0]|3[0-5][0-9]|[12][0-9][0-9]|[1-9]?[0-9])(?:\.\d+)?))\s*[\s,]\s*(100(?:\.0+)?|\d{1,2}[.]\d*|\d{1,2})%\s*[\s,]\s*(100(?:\.0+)?|\d{1,2}[.]\d*|\d{1,2})%\s*(?:[\s,]|[\s]*\/)\s*(0[.][0-9]+|[.][0-9]+|[01][.]0*|[01])\s*\)$`)
)

func isHex(b byte) bool { return b >= '0' && b <= '9' || b|0x20 >= 'a' && b|0x20 <= 'f' }

// computeColors appends the color literals of line to dst.
func computeColors(line string, dst []colorSpan) []colorSpan {
	for i := 0; i < len(line); i++ {
		switch b := line[i]; {
		case b == '#':
			if i > 0 {
				if p, _ := utf8.DecodeLastRuneInString(line[:i]); p != '\'' && p != '"' && !isJSSpace(p) {
					continue
				}
			}
			n := 0
			for i+1+n < len(line) && isHex(line[i+1+n]) {
				n++
			}
			if n != 3 && n != 4 && n != 6 && n != 8 {
				continue
			}
			if e := i + 1 + n; e < len(line) && isWordByte(line[e]) {
				continue
			}
			if c, ok := parseHexColor(line[i+1 : i+1+n]); ok {
				dst = append(dst, colorSpan{i, i + 1 + n, c})
			}
			i += n
		case b == 'r' || b == 'h':
			if i > 0 && isWordByte(line[i-1]) {
				continue
			}
			if sp, ok := funcColor(line, i); ok {
				dst = append(dst, sp)
				i = sp.b - 1
			}
		}
	}
	return dst
}

// funcColor parses rgb(), rgba(), hsl() or hsla() at byte i of line.
func funcColor(line string, i int) (colorSpan, bool) {
	var name string
	for _, n := range [...]string{"rgba", "rgb", "hsla", "hsl"} {
		if strings.HasPrefix(line[i:], n+"(") {
			name = n
			break
		}
	}
	if name == "" {
		return colorSpan{}, false
	}
	p := i + len(name)
	e := p + 1
	for e < len(line) && strings.IndexByte("0123456789 \t,.%/", line[e]) >= 0 {
		e++
	}
	if e >= len(line) || line[e] != ')' {
		return colorSpan{}, false
	}
	args := line[p : e+1]
	num := func(s string) float64 { v, _ := strconv.ParseFloat(s, 64); return v }
	var c color.NRGBA
	switch name {
	case "rgb", "rgba":
		re := reRGB
		if name == "rgba" {
			re = reRGBA
		}
		m := re.FindStringSubmatch(args)
		if m == nil {
			return colorSpan{}, false
		}
		a := 1.0
		if name == "rgba" {
			a = num(m[4])
		}
		c = color.NRGBA{uint8(num(m[1])), uint8(num(m[2])), uint8(num(m[3])), alpha255(a)}
	default:
		re := reHSL
		if name == "hsla" {
			re = reHSLA
		}
		m := re.FindStringSubmatch(args)
		if m == nil {
			return colorSpan{}, false
		}
		a := 1.0
		if name == "hsla" {
			a = num(m[4])
		}
		c = hslToRGB(num(m[1]), num(m[2])/100, num(m[3])/100, a)
	}
	return colorSpan{i, e + 1, c}, true
}

func alpha255(a float64) uint8 { return uint8(math.Round(max(0, min(a, 1)) * 255)) }

// parseHexColor parses 3, 4, 6 or 8 hex digits (Monaco's
// Color.Format.CSS.parseHex).
func parseHexColor(h string) (color.NRGBA, bool) {
	d := func(i int) uint8 {
		v, _ := strconv.ParseUint(h[i:i+1], 16, 8)
		return uint8(v)
	}
	switch len(h) {
	case 3, 4:
		c := color.NRGBA{d(0) * 17, d(1) * 17, d(2) * 17, 255}
		if len(h) == 4 {
			c.A = d(3) * 17
		}
		return c, true
	case 6, 8:
		dd := func(i int) uint8 { return d(i)<<4 | d(i+1) }
		c := color.NRGBA{dd(0), dd(2), dd(4), 255}
		if len(h) == 8 {
			c.A = dd(6)
		}
		return c, true
	}
	return color.NRGBA{}, false
}

// hslToRGB is Monaco's HSLA.toRGBA (h in degrees, s and l in 0..1).
func hslToRGB(h, s, l, a float64) color.NRGBA {
	h = float64(int(max(0, min(h, 360))))
	s, l = round3(max(0, min(s, 1))), round3(max(0, min(l, 1)))
	h /= 360
	var r, g, b float64
	if s == 0 {
		r, g, b = l, l, l
	} else {
		q := l + s - l*s
		if l < 0.5 {
			q = l * (1 + s)
		}
		p := 2*l - q
		r, g, b = hue2rgb(p, q, h+1.0/3), hue2rgb(p, q, h), hue2rgb(p, q, h-1.0/3)
	}
	c8 := func(v float64) uint8 { return uint8(math.Round(v * 255)) }
	return color.NRGBA{c8(r), c8(g), c8(b), alpha255(a)}
}

func hue2rgb(p, q, t float64) float64 {
	if t < 0 {
		t++
	}
	if t > 1 {
		t--
	}
	switch {
	case t < 1.0/6:
		return p + (q-p)*6*t
	case t < 1.0/2:
		return q
	case t < 2.0/3:
		return p + (q-p)*(2.0/3-t)*6
	}
	return p
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }

// rgbToHSL is Monaco's HSLA.fromRGBA: the hue in whole degrees, saturation
// and lightness in 0..1.
func rgbToHSL(c color.NRGBA) (h int, s, l float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	mx, mn := max(r, g, b), min(r, g, b)
	l = (mn + mx) / 2
	ch := mx - mn
	hf := 0.0
	if ch > 0 {
		if l <= 0.5 {
			s = min(ch/(2*l), 1)
		} else {
			s = min(ch/(2-2*l), 1)
		}
		switch mx {
		case r:
			hf = (g - b) / ch
			if g < b {
				hf += 6
			}
		case g:
			hf = (b-r)/ch + 2
		default:
			hf = (r-g)/ch + 4
		}
		hf = math.Round(hf * 60)
	}
	return int(max(0, min(hf, 360))), round3(s), round3(l)
}

// jsFixed is JavaScript's v.toFixed(n).
func jsFixed(v float64, n int) string { return strconv.FormatFloat(v, 'f', n, 64) }

// colorAlpha returns c's alpha as Monaco's RGBA keeps it (three decimals).
func colorAlpha(c color.NRGBA) float64 { return round3(float64(c.A) / 255) }

// The default provider's presentations (provideColorPresentations): rgb,
// hsl and hex.
func formatRGB(c color.NRGBA) string {
	if a := colorAlpha(c); a != 1 {
		f, _ := strconv.ParseFloat(jsFixed(a, 2), 64)
		return fmt.Sprintf("rgba(%d, %d, %d, %s)", c.R, c.G, c.B, strconv.FormatFloat(f, 'f', -1, 64))
	}
	return fmt.Sprintf("rgb(%d, %d, %d)", c.R, c.G, c.B)
}

func formatHSL(c color.NRGBA) string {
	h, s, l := rgbToHSL(c)
	sp, lp := int(math.Round(s*100)), int(math.Round(l*100))
	if a := colorAlpha(c); a != 1 {
		return fmt.Sprintf("hsla(%d, %d%%, %d%%, %s)", h, sp, lp, jsFixed(a, 2))
	}
	return fmt.Sprintf("hsl(%d, %d%%, %d%%)", h, sp, lp)
}

func formatHex(c color.NRGBA) string {
	if colorAlpha(c) == 1 {
		return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
	}
	return fmt.Sprintf("#%02x%02x%02x%02x", c.R, c.G, c.B, c.A)
}

var colorFormats = [...]func(color.NRGBA) string{formatRGB, formatHSL, formatHex}

// guessPresentation is Monaco's ColorPickerModel.guessColorPresentation:
// the presentation of c whose label is the literal (ignoring case), else
// the first one starting with the literal's text before "(", else rgb.
func guessPresentation(c color.NRGBA, literal string) int {
	low := strings.ToLower(literal)
	for i, f := range colorFormats {
		if f(c) == low {
			return i
		}
	}
	prefix, _, _ := strings.Cut(low, "(")
	for i, f := range colorFormats {
		if strings.HasPrefix(strings.ToLower(f(c)), prefix) {
			return i
		}
	}
	return 0
}

// lineColors returns the color literals of model line i (none when color
// decorators are off for the document or the line is long).
func (w *ViewW) lineColors(text string, dst []colorSpan) []colorSpan {
	if !w.opts.ColorDecorators || w.st.tooLargeForWorker() || isLong(text) {
		return dst
	}
	if !mayHaveColors(text) {
		return dst
	}
	return computeColors(text, dst)
}

// swatchHit is a drawn swatch, for the pointer: its box (window
// coordinates), the literal's document bytes and color.
type swatchHit struct {
	r    geom.Rect
	a, b int
	c    color.NRGBA
}

// pickerState is an open color picker (colors.go).
type pickerState struct {
	buf    *buffer.Buffer
	ver    uint64 // the text it was opened on, or last wrote
	a, b   int    // the literal
	col    uitheme.Color
	last   uitheme.Color // the color the literal holds
	pres   int           // presentation (colorFormats)
	anchor geom.Rect     // the swatch
	edited bool          // an undo step was opened
}

// swatchPress opens the picker when p is a press on a swatch, and reports
// whether it was.
func (w *ViewW) swatchPress(p pending) bool {
	vs, st := w.vs, w.st
	for _, h := range vs.swatches {
		if !h.r.Contains(p.pressPos) {
			continue
		}
		w.closePicker()
		snap := st.Snapshot()
		lit := snap.Slice(h.a, h.b)
		vs.picker = &pickerState{buf: st.Model.Buffer(), ver: snap.Version(), a: h.a, b: h.b, col: h.c, last: h.c,
			pres: guessPresentation(h.c, lit), anchor: h.r}
		ui.Invalidate(w.c.Window())
		return true
	}
	return false
}

// closePicker closes the color picker, ending its undo step.
func (w *ViewW) closePicker() {
	vs := w.vs
	if pk := vs.picker; pk != nil {
		if pk.edited {
			pk.buf.PushUndoStop()
		}
		vs.picker = nil
		ui.Invalidate(w.c.Window())
	}
}

// colorPicker builds the open overlay during View. Text mutation is deferred
// to finish, after the caller's current-frame modifiers have been applied.
func (w *ViewW) colorPicker() {
	vs, st, c := w.vs, w.st, w.c
	pk := vs.picker
	if pk == nil {
		return
	}
	snap := st.Snapshot()
	if pk.buf != st.Model.Buffer() || pk.ver != snap.Version() || !w.opts.ColorDecorators {
		w.closePicker() // the text changed under it
		return
	}
	// The swatch as drawn by the last paint: the picker follows it when
	// the view scrolls, and closes when it is scrolled out of view.
	found := false
	for _, h := range vs.swatches {
		if h.a == pk.a {
			// Wheel input has already updated the viewport, but this
			// frame has not painted yet. Translate the last paint's
			// swatch by that scroll before placing the overlay.
			v, old := w.displayViewport(), vs.dmg
			dx := old.scrollX - v.ScrollX
			dy := float32(old.topLine-v.TopLine)*v.LineHeight + old.offsetY - v.OffsetY
			r := h.r.Offset(dx, dy).Intersect(vs.text)
			if !r.Empty() {
				pk.anchor, found = r, true
			}
			break
		}
	}
	if !found {
		w.closePicker()
		return
	}
	at := pk.anchor
	x, y := at.X, at.Bottom()+2
	if r, ok := c.RectOf(vs.pickerID); ok {
		// Keep the panel in the window (its size from the last frame); above
		// the swatch when it does not fit below.
		sz := c.Size()
		if y+r.H > sz.H && at.Y-2-r.H >= 0 {
			y = at.Y - 2 - r.H
		}
		x = max(0, min(x, sz.W-r.W))
	}
	ro := st.readOnly()
	box := ui.Overlay(c, ui.OverlayOptions{Place: ui.ScreenTopLeft, ShiftX: x, ShiftY: y}, func() {
		ui.ColorPickerPanel(c, &pk.col).Disabled(ro)
	})
	vs.pickerID = box.ID()
	if c.Dismissed(box.ID()) {
		w.closePicker()
		return
	}
}

// commitPicker runs only with finalized frame options. The overlay is built
// earlier, so its initial Disabled state is not authorization to edit.
func (w *ViewW) commitPicker() {
	pk := w.vs.picker
	if pk == nil {
		return
	}
	if w.opts.ReadOnly {
		pk.col = pk.last
		return
	}
	if pk.col != pk.last {
		w.writeColor(pk)
	}
}

// writeColor replaces the literal with the picked color, inside the
// picker's undo step.
func (w *ViewW) writeColor(pk *pickerState) {
	st := w.st
	if w.opts.ReadOnly || !w.opts.ColorDecorators || pk.buf != st.Model.Buffer() || pk.ver != st.Snapshot().Version() {
		return
	}
	defer st.Model.command()()
	text := colorFormats[pk.pres](pk.col)
	pk.last = pk.col
	snap := st.Snapshot()
	if snap.Slice(pk.a, pk.b) == text {
		return
	}
	if !pk.edited {
		pk.buf.PushUndoStop()
		pk.edited = true
	}
	edits := []buffer.Edit{{Start: pk.a, End: pk.b, Text: text}}
	if err := st.edit(edits, kindNone, mapSels(edits, st.sels()), false); err != nil {
		return
	}
	pk.b = pk.a + len(text)
	pk.ver = st.Snapshot().Version()
	w.changed = true
}
