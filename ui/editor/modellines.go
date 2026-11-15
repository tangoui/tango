package editor

import (
	"hash/maphash"
	"slices"
	"time"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/editor/highlight"
	"github.com/tangoui/tango/ui/editor/monarch"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// ModelLines (docs/specs/diff-view.md, 2.6): a view zone drawing of
// another Model's lines, with their syntax colors, as the View draws its
// own (the inline diff's deleted lines, from the original side). It is
// drawn through the View that paints the zone (its font, its LineCache,
// its tab size), unwrapped.

// modelLinesBudget is how long one zone drawing may tokenize the source
// Model's lines; the rest are drawn with the tokens there are (the
// Version is 0 until they are final, so the drawing is fingerprinted).
const modelLinesBudget = time.Millisecond

var modelLinesSeed = maphash.MakeSeed()

// ModelLines returns a view zone Paint drawing the lines [from, to) of
// src (clamped to its text) one per row, unwrapped, with src's syntax
// colors and the zone's theme, plus the Background, Border and Inline
// styles of decos (offsets of src's current text; the other options are
// not drawn), scrolling with the text. version is the drawing's
// ViewZone.Version for the current text, decorations and highlighting
// (it changes with them); it is 0 while the lines' tokens are not final,
// so the editor fingerprints the drawing until they are. Call it again
// after src changes. A line longer than 2000 characters is drawn up to
// there.
//
// Experimental: part of the diff view's editor API (docs/specs/diff-view.md).
func ModelLines(src *Model, from, to int, decos []Decoration) (paintFn func(*paint.List, ZonePaint), version uint64) {
	snap := src.Snapshot()
	n := snap.LineCount()
	from, to = max(0, min(from, n)), max(0, min(to, n))
	to = max(from, to)
	ds := slices.Clone(decos)
	styles := make([]*decorationStyle, len(ds))
	seen := map[*DecorationOptions]*decorationStyle{}
	var h maphash.Hash
	h.SetSeed(modelLinesSeed)
	num := func(v uint64) {
		var b [8]byte
		for i := range b {
			b[i] = byte(v >> (8 * i))
		}
		h.Write(b[:])
	}
	num(uint64(snap.Version()))
	num(uint64(from))
	num(uint64(to))
	for i, d := range ds {
		s, ok := seen[d.Options]
		if !ok {
			s = newDecorationStyle(d.Options)
			seen[d.Options] = s
		}
		styles[i] = s
		num(s.fp)
		num(uint64(d.Start))
		num(uint64(d.End))
	}
	hl := src.Highlighter()
	if hl != nil {
		num(hl.GrammarGeneration())
	}
	version = h.Sum64() | 1
	if hl != nil && !hl.FinalBefore(snap, to) {
		version = 0
	}
	key := src.linesKey()
	paintFn = func(l *paint.List, z ZonePaint) {
		w := z.view
		if w == nil || to <= from {
			return
		}
		th := z.Theme
		if th == nil {
			th = themeOf(w.st)
		}
		// The rows in the text area (the zone may be partly scrolled out).
		area := w.vs.text
		k0 := max(0, int((area.Y-z.Rect.Y)/z.RowH))
		k1 := min(to-from, int((area.Bottom()-z.Rect.Y)/z.RowH)+1)
		if hl != nil {
			hl.HighlightFor(key, snap, from+k1, modelLinesBudget)
		}
		var toks []monarch.Token
		var ins []inlineSpan
		for k := k0; k < k1; k++ {
			i := from + k
			s := snap.Line(i)
			if isLong(s) {
				s = cutRunes(s, longLineRunes)
			}
			if hl != nil {
				toks, _ = hl.TokensFor(key, snap, i, toks)
			}
			runs, paints := w.styleRuns(s, 0, toks, th, nil)
			lay := w.vs.lines.Shape(s, runs, w.lineOptions())
			runes := utf8.RuneCountInString(s)
			lv := lineView{i: i, text: s, b1: len(s), runes: runes, r1: runes, rows: 1,
				segs: []seg{{lay: lay, r1: runes, b1: len(s), paints: paints}}}
			y := z.Rect.Y + float32(k)*z.RowH
			ls := snap.LineStart(i)
			ins = ins[:0]
			for j, d := range ds {
				o := &styles[j].o
				a, b := max(d.Start, ls)-ls, min(d.End-ls, len(s))
				if a >= b {
					continue
				}
				if o.Background.A > 0 || o.Border.A > 0 {
					decoRects(&lv, a, b, func(x0, x1 float32) {
						box := rectXYWH(z.OriginX+x0, y, x1-x0, z.RowH)
						l.Fill(box, geom.Radii{}, o.Background)
						decoStroke(l, box, o.Border)
					})
				}
				if o.Inline != nil {
					ins = append(ins, inlineSpan{r0: lv.runeAt(a), r1: lv.runeAt(b), st: o.Inline})
				}
			}
			drawSegInline(l, lv.segs[0], z.OriginX, y, ins)
		}
	}
	return paintFn, version
}

// cutRunes returns the first n runes of s.
func cutRunes(s string, n int) string {
	for i := range s {
		if n == 0 {
			return s[:i]
		}
		n--
	}
	return s
}

// linesKey returns the Highlighter token window ModelLines uses for m.
func (m *Model) linesKey() highlight.ViewKey {
	if !m.linesKeyOK {
		m.linesKeyID, m.linesKeyOK = highlight.NewViewKey(), true
	}
	return m.linesKeyID
}
