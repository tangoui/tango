package ui

import (
	"math"
	"math/big"
	"strconv"
	"strings"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// sideRadii rounds the two corners on the physical left side of a box
// (right side when right is set) with radius rad.
func sideRadii(rad float32, right bool) geom.Radii {
	if right {
		return geom.Radii{TR: rad, BR: rad}
	}
	return geom.Radii{TL: rad, BL: rad}
}

// prefixWidth is the room the InputNumber prefix takes (with its gap).
func (w *InputNumberW) prefixWidth() float32 {
	c := w.c
	is := inputFontSize(&w.th.Components.Input, w.size)
	pw := float32(0)
	if w.prefix != nil {
		pw += is + 4
	}
	if w.prefixTxt != "" {
		pw += text.Width(w.prefixTxt, c.style(is, text.Regular)) + 4
	}
	return pw
}

func (w *InputNumberW) paintPrefix(l *paint.List, r geom.Rect) {
	if w.prefix == nil && w.prefixTxt == "" {
		return
	}
	c, th := w.c, w.th
	s := c.inline(r) // the prefix sits at the inline start
	off := w.padL()
	if w.ctrlPos == "sides" {
		off = r.H + 8
	}
	col := th.Text
	if w.box.disabled {
		col = th.TextDisabled
	}
	is := inputFontSize(&th.Components.Input, w.size)
	if w.prefix != nil {
		c.drawIconAt(l, w.prefix, s.X(off, is), r.Y+(r.H-is)/2, is, col)
		off += is + 4
	}
	if w.prefixTxt != "" {
		lay := text.Shape(w.prefixTxt, c.style(is, text.Regular), text.Options{NoWrap: true})
		l.Glyphs(geom.Pt(s.X(off, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
	}
}

// padL is the left padding before the prefix / value (shadcn px-3).
func (w *InputNumberW) padL() float32 {
	if w.th.Flavor == theme.StyleShadcn {
		return w.th.ControlPaddingH
	}
	return 11
}

// layoutParts places the editor and the step buttons once the chained
// options are known (prefix room, ControlsPosition "sides").
func (w *InputNumberW) layoutParts() {
	pw := w.prefixWidth()
	if w.ctrlPos != "sides" {
		w.ed.ln.Margin.Left = w.padL() + pw
		return
	}
	c, th := w.c, w.th
	h := w.box.ln.Height
	if w.box.ln.Width == w.tk.ControlWidth {
		w.box.ln.Width = 150 // Element el-input-number default width
	}
	for _, b := range []*node{w.down, w.up} {
		b.parent = w.box
		b.ln.Grow = 0
		b.ln.Width, b.ln.Height = h, h
		b.aHidden = true // Increment / Decrement actions of the spin button
	}
	w.ed.ln.Margin = geom.Insets{Left: 8 + pw, Right: 8}
	w.box.kids = []*node{w.down, w.ed, w.up}
	rad := controlRadiusOf(th, w.size)
	side := func(n *node, ia *Interaction, ic *icon.Icon, up bool) {
		n.paint = func(l *paint.List, r geom.Rect) {
			// − sits at the inline start of the field, + at the end; each
			// rounds its outer corners and draws the divider facing the value.
			s := c.inline(r)
			in := s.Rect(1, r.Y+1, r.W-1, r.H-2)
			radii := sideRadii(rad-1, c.IsRTL())
			sep := s.Rect(r.W-1, r.Y+1, 1, r.H-2)
			if up {
				in = s.Rect(0, r.Y+1, r.W-1, r.H-2)
				radii = sideRadii(rad-1, !c.IsRTL())
				sep = s.Rect(0, r.Y+1, 1, r.H-2)
			}
			tk := w.tk
			l.Fill(in, radii, tk.SideHandleBg)
			l.Fill(sep, geom.Radii{}, tk.HandleBorderColor)
			col := tk.SideHandleColor
			switch {
			case w.box.disabled || w.atLimit(up):
				col = tk.HandleDisabledColor
			case ia.Hovered:
				col = tk.SideHandleHoverColor
			}
			is := tk.SideHandleFontSize
			drawIcon(l, ic, r.X+(r.W-is)/2, r.Y+(r.H-is)/2, is, col)
		}
	}
	side(w.down, &w.dnIA, icon.Minus, false)
	side(w.up, &w.upIA, icon.Plus, true)
}

// ---------------------------------------------------------------------------
// Exact decimals for InputNumber StringMode: m × 10^-exp.

type dec struct {
	m   *big.Int
	exp int
}

// parseDec reads "-12.340", "+.5", "3e-2" (exact).
func parseDec(s string) (dec, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return dec{}, false
	}
	e := 0
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		n, err := strconv.Atoi(s[i+1:])
		if err != nil || n > 1000 || n < -1000 {
			return dec{}, false
		}
		e, s = n, s[:i]
	}
	neg := false
	switch {
	case strings.HasPrefix(s, "-"):
		neg, s = true, s[1:]
	case strings.HasPrefix(s, "+"):
		s = s[1:]
	}
	intPart, frac, _ := strings.Cut(s, ".")
	if intPart == "" && frac == "" {
		return dec{}, false
	}
	digits := intPart + frac
	for _, r := range digits {
		if r < '0' || r > '9' {
			return dec{}, false
		}
	}
	m, ok := new(big.Int).SetString("0"+digits, 10)
	if !ok {
		return dec{}, false
	}
	if neg {
		m.Neg(m)
	}
	d := dec{m: m, exp: len(frac) - e}
	if d.exp < 0 {
		d.m.Mul(d.m, pow10(-d.exp))
		d.exp = 0
	}
	return d, true
}

func pow10(n int) *big.Int { return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil) }

// decFromFloat converts with the shortest decimal that round-trips.
func decFromFloat(f float64) dec {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		f = 0
	}
	d, _ := parseDec(strconv.FormatFloat(f, 'f', -1, 64))
	return d
}

func (a dec) scaled(exp int) *big.Int {
	if exp == a.exp {
		return new(big.Int).Set(a.m)
	}
	return new(big.Int).Mul(a.m, pow10(exp-a.exp))
}

func (a dec) add(b dec) dec {
	e := max(a.exp, b.exp)
	return dec{m: new(big.Int).Add(a.scaled(e), b.scaled(e)), exp: e}
}

func (a dec) neg() dec { return dec{m: new(big.Int).Neg(a.m), exp: a.exp} }

func (a dec) cmp(b dec) int {
	e := max(a.exp, b.exp)
	return a.scaled(e).Cmp(b.scaled(e))
}

// round rounds to p fraction digits, halves away from zero.
func (a dec) round(p int) dec {
	if a.exp <= p {
		return a
	}
	div := pow10(a.exp - p)
	q, r := new(big.Int).QuoRem(a.m, div, new(big.Int))
	r.Abs(r).Mul(r, big.NewInt(2))
	if r.Cmp(div) >= 0 {
		if a.m.Sign() < 0 {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	return dec{m: q, exp: p}
}

// fixed formats with exactly p fraction digits.
func (a dec) fixed(p int) string {
	a = a.round(p)
	return a.format(p)
}

func (a dec) format(p int) string {
	m := a.scaled(p)
	neg := m.Sign() < 0
	s := new(big.Int).Abs(m).String()
	if p > 0 {
		if len(s) <= p {
			s = strings.Repeat("0", p-len(s)+1) + s
		}
		s = s[:len(s)-p] + "." + s[len(s)-p:]
	}
	if neg {
		s = "-" + s
	}
	return s
}

// String formats without trailing fraction zeros.
func (a dec) String() string {
	s := a.format(a.exp)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	if s == "-0" {
		s = "0"
	}
	return s
}

func (a dec) float() float64 {
	f, _ := strconv.ParseFloat(a.format(a.exp), 64)
	return f
}
