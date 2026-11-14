package editor

import (
	"encoding/binary"
	"hash/maphash"
	"image/color"
	"math"
	"slices"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// Model decorations (docs/specs/diff-view.md, 2.1): ranges of a Model's
// text with options that every view (or one view) draws, moving with the
// text. They are stored per Model in a decorationStore (decorations_store.go)
// and drawn in three layers by paint.go: whole-line backgrounds under the
// selections, range backgrounds over them, and inline styles over the
// glyphs; the gutter shows their margin background, line number color and
// marks, the overview ruler their marks.

// Stickiness is what a decoration's ends do when text is typed exactly at
// them (Monaco's TrackedRangeStickiness).
//
// Experimental: part of the decorations API (docs/specs/diff-view.md).
type Stickiness uint8

const (
	// AlwaysGrowsWhenTypingAtEdges: text typed at either end goes inside
	// (Monaco's default).
	AlwaysGrowsWhenTypingAtEdges Stickiness = iota
	// NeverGrowsWhenTypingAtEdges: text typed at either end stays outside.
	NeverGrowsWhenTypingAtEdges
	// GrowsOnlyWhenTypingBefore: text typed at the start goes inside, at
	// the end outside.
	GrowsOnlyWhenTypingBefore
	// GrowsOnlyWhenTypingAfter: text typed at the end goes inside, at the
	// start outside.
	GrowsOnlyWhenTypingAfter
)

// stick returns the store's stickiness of s.
func (s Stickiness) stick() decorationStickiness {
	switch s {
	case NeverGrowsWhenTypingAtEdges:
		return decorationNeverGrows
	case GrowsOnlyWhenTypingBefore:
		return decorationGrowsBefore
	case GrowsOnlyWhenTypingAfter:
		return decorationGrowsAfter
	}
	return decorationAlwaysGrows
}

// DecorationOptions is how a decoration is drawn: a subset of Monaco's
// IModelDecorationOptions. One *DecorationOptions may be shared by many
// decorations (Monaco's ModelDecorationOptions.register); do not change it
// after passing it to SetDecorations (the editor copies it then, so a later
// change is not seen until it is set again).
//
// Experimental: part of the decorations API (docs/specs/diff-view.md).
type DecorationOptions struct {
	// Description names the decoration for debugging and screen readers.
	Description string
	// Stickiness is what the ends do when text is typed at them.
	Stickiness Stickiness
	// ZIndex orders overlapping decorations: higher ones are drawn later
	// (over the lower ones). Ties are ordered by owner name, then by the
	// position in the owner's list.
	ZIndex int
	// WholeLine draws Background (and Border) across the text area on
	// every line the range touches, from the line of Start to the line of
	// End (Monaco's isWholeLine; a range ending at the start of a line
	// includes that line).
	WholeLine bool
	// ShowIfCollapsed draws an empty range (on its line: whole-line
	// background, margin, line number and overview marks); without it an
	// empty range is not drawn at all.
	ShowIfCollapsed bool

	// Background fills the range's characters (Monaco's className), or
	// with WholeLine its lines. Transparent: none.
	Background color.NRGBA
	// Border strokes a one pixel border inside each filled box.
	Border color.NRGBA
	// Inline changes how the range's glyphs are drawn without changing
	// their layout (Monaco's inlineClassName); nil: no change.
	Inline *InlineStyle

	// Gutter is a mark in the gutter's line decorations margin on every
	// line of the range (Monaco's linesDecorationsClassName); nil: none.
	Gutter *GutterMark
	// MarginBackground fills the whole gutter on the range's lines
	// (Monaco's marginClassName). Transparent: none.
	MarginBackground color.NRGBA
	// LineNumberColor is the color of the line numbers of the range's
	// lines (Monaco's lineNumberClassName). Transparent: unchanged.
	LineNumberColor color.NRGBA

	// Overview marks the range's lines on the overview ruler; nil: none.
	Overview *OverviewStyle
}

// InlineStyle changes how glyphs are drawn without changing their layout
// (no bold or italic: those change the shaping).
//
// Experimental: part of the decorations API (docs/specs/diff-view.md).
type InlineStyle struct {
	// Color replaces the token color (transparent: unchanged).
	Color color.NRGBA
	// Underline and Strikethrough draw a line under or through the glyphs.
	Underline, Strikethrough bool
	// Opacity multiplies the glyphs' alpha (0: unchanged).
	Opacity float32
}

// GutterMark is a decoration's mark in the gutter's line decorations
// margin (the 18 pixels between the line numbers and the text).
//
// Experimental: part of the decorations API (docs/specs/diff-view.md).
type GutterMark struct {
	// Background fills the margin's cell.
	Background color.NRGBA
	// Bar is a 3 pixel bar at the cell's left edge (a change bar).
	Bar color.NRGBA
	// Sign is one or two characters centred in the cell on the line's
	// first row (a diff's "+" or "−"), drawn in SignColor.
	Sign      string
	SignColor color.NRGBA
}

// OverviewStyle is a decoration's mark on the overview ruler.
//
// Experimental: part of the decorations API (docs/specs/diff-view.md).
type OverviewStyle struct {
	Color color.NRGBA
	// Lane is a bit mask as OverviewMark.Lane: 1 left, 2 middle, 4 right.
	Lane int
}

// Decoration is a decorated range: UTF-8 byte offsets [Start, End) of the
// text, and how it is drawn.
//
// Experimental: part of the decorations API (docs/specs/diff-view.md).
type Decoration struct {
	Start, End int
	Options    *DecorationOptions
}

// decorationStyle is the editor's copy of a DecorationOptions, with its
// fingerprint (the rows' redraw hints, damage.go).
type decorationStyle struct {
	src *DecorationOptions
	o   DecorationOptions // Inline, Gutter and Overview point to the copies below
	in  InlineStyle
	gm  GutterMark
	ov  OverviewStyle
	fp  uint64
}

var decorationSeed = maphash.MakeSeed()

// newDecorationStyle copies o (nil: draws nothing).
func newDecorationStyle(o *DecorationOptions) *decorationStyle {
	ds := &decorationStyle{src: o}
	if o == nil {
		return ds
	}
	ds.o = *o
	if o.Inline != nil {
		ds.in = *o.Inline
		ds.o.Inline = &ds.in
	}
	if o.Gutter != nil {
		ds.gm = *o.Gutter
		ds.o.Gutter = &ds.gm
	}
	if o.Overview != nil {
		ds.ov = *o.Overview
		ds.o.Overview = &ds.ov
	}
	var h maphash.Hash
	h.SetSeed(decorationSeed)
	var b [8]byte
	num := func(v int) { h.Write(binary.LittleEndian.AppendUint64(b[:0], uint64(v))) }
	col := func(c color.NRGBA) { num(int(c.R) | int(c.G)<<8 | int(c.B)<<16 | int(c.A)<<24) }
	flag := func(on bool) {
		if on {
			num(1)
		} else {
			num(0)
		}
	}
	num(ds.o.ZIndex)
	flag(ds.o.WholeLine)
	flag(ds.o.ShowIfCollapsed)
	col(ds.o.Background)
	col(ds.o.Border)
	flag(ds.o.Inline != nil)
	col(ds.in.Color)
	flag(ds.in.Underline)
	flag(ds.in.Strikethrough)
	num(int(math.Float32bits(ds.in.Opacity)))
	flag(ds.o.Gutter != nil)
	col(ds.gm.Background)
	col(ds.gm.Bar)
	h.WriteString(ds.gm.Sign)
	col(ds.gm.SignColor)
	col(ds.o.MarginBackground)
	col(ds.o.LineNumberColor)
	flag(ds.o.Overview != nil)
	col(ds.ov.Color)
	num(ds.ov.Lane)
	ds.fp = h.Sum64() | 1
	return ds
}

// decorationEntryOf is the stored decoration type.
type decorationEntryOf = decorationEntry[decorationStyle]

// decorations returns the Model's decoration store, creating it.
func (m *Model) decorations() *decorationStore[decorationStyle] {
	if m.decos == nil {
		m.decos = newDecorationStore[decorationStyle]()
	}
	return m.decos
}

// hasDecorations reports whether the Model has any decoration.
func (m *Model) hasDecorations() bool { return m.decos != nil && m.decos.len() > 0 }

// setDecorations sets owner's decorations for view (nil: every view).
func (m *Model) setDecorations(owner string, view *State, ds []Decoration) {
	st := m.decorations()
	specs := make([]decorationSpec[decorationStyle], len(ds))
	styles := make(map[*DecorationOptions]*decorationStyle)
	for i, d := range ds {
		s, ok := styles[d.Options]
		if !ok {
			s = newDecorationStyle(d.Options)
			styles[d.Options] = s
		}
		specs[i] = decorationSpec[decorationStyle]{start: d.Start, end: d.End, stick: s.o.Stickiness.stick(), opts: s}
	}
	st.set(owner, view, specs, m.buf.Snapshot())
}

// decorationsOf returns owner's decorations for view.
func (m *Model) decorationsOf(owner string, view *State) []Decoration {
	if m.decos == nil {
		return nil
	}
	specs := m.decos.get(owner, view)
	if specs == nil {
		return nil
	}
	out := make([]Decoration, len(specs))
	for i, s := range specs {
		out[i] = Decoration{Start: s.start, End: s.end, Options: s.opts.src}
	}
	return out
}

// SetDecorations replaces owner's decorations, shown in every view of the
// Model; no decorations remove the owner. The ranges are offsets of the
// current text: they are copied, ordered (an End before Start is swapped),
// clamped to the text and widened to whole characters, and from then on
// move with every edit, undo and redo (by their Options' Stickiness; a
// range whose text is deleted collapses). Decorations are not part of the
// undo history: an undo moves them as any edit does. O(D + n log n) for n
// decorations among D. Called from the UI goroutine, like every Model
// method; the views draw the change from their next frame.
//
// Experimental: part of the decorations API (docs/specs/diff-view.md).
func (m *Model) SetDecorations(owner string, ds []Decoration) { m.setDecorations(owner, nil, ds) }

// Decorations returns owner's current decorations (moved with the edits
// since they were set), in the order they were set: an index is a stable
// ID within the owner. Nil when the owner has none.
//
// Experimental: part of the decorations API (docs/specs/diff-view.md).
func (m *Model) Decorations(owner string) []Decoration { return m.decorationsOf(owner, nil) }

// DecorationsIn appends to dst the Model-wide decorations (of every owner,
// not those of a single view: State.SetDecorations) intersecting or
// touching the offsets [start, end], sorted by Start. O(log D + k).
//
// Experimental: part of the decorations API (docs/specs/diff-view.md).
func (m *Model) DecorationsIn(start, end int, dst []Decoration) []Decoration {
	if m.decos == nil {
		return dst
	}
	for _, e := range m.decos.query(start, end, nil, nil) {
		dst = append(dst, Decoration{Start: e.start, End: e.end, Options: e.opts.src})
	}
	return dst
}

// SetDecorations replaces owner's decorations shown in this view only
// (Monaco's ownerId filter): they are stored on the Model and move with
// its text like Model.SetDecorations', and go away with Close. The owner
// names of a view and of the Model are separate.
//
// Experimental: part of the decorations API (docs/specs/diff-view.md).
func (s *State) SetDecorations(owner string, ds []Decoration) { s.Model.setDecorations(owner, s, ds) }

// Decorations returns the current decorations set by SetDecorations for
// owner on this view.
//
// Experimental: part of the decorations API (docs/specs/diff-view.md).
func (s *State) Decorations(owner string) []Decoration { return s.Model.decorationsOf(owner, s) }

// LineDecoration returns a decoration of the lines first..last of snap
// (clamped): from the start of first to the end of last (not the start of
// the next line, which a WholeLine decoration would include).
//
// Experimental: part of the decorations API (docs/specs/diff-view.md).
func LineDecoration(snap buffer.Snapshot, first, last int, o *DecorationOptions) Decoration {
	n := snap.LineCount()
	first = max(0, min(first, n-1))
	last = max(first, min(last, n-1))
	return Decoration{Start: snap.LineStart(first), End: lineEnd(snap, last), Options: o}
}

// DecorationFromLSP returns the decoration of an LSP range: zero-based
// lines and UTF-16 columns of snap (clamped to the text).
//
// Experimental: part of the decorations API (docs/specs/diff-view.md).
func DecorationFromLSP(snap buffer.Snapshot, startLine, startUTF16, endLine, endUTF16 int, o *DecorationOptions) Decoration {
	off := func(line, col int) int {
		n := snap.LineCount()
		switch {
		case line < 0:
			return 0
		case line >= n:
			return snap.Len()
		}
		return snap.LineStart(line) + snap.ByteCol(line, max(col, 0))
	}
	return Decoration{Start: off(startLine, startUTF16), End: off(endLine, endUTF16), Options: o}
}

// --- tracking ---

// tracked moves everything that follows the text through a change (the
// decorations, and the view zones and hidden areas of the views that
// have some) from old to ns: one Tracker for all of them. It runs before
// the source view's selection update and the subscribers.
func (m *Model) tracked(ns buffer.Snapshot, edits []buffer.Edit) {
	if len(edits) == 0 {
		return
	}
	deco := m.hasDecorations()
	if !deco && len(m.trackers) == 0 {
		return
	}
	t := buffer.NewTracker(edits)
	if deco {
		m.decos.apply(edits, t, ns)
	}
	for _, s := range m.trackers {
		s.trackEdits(t, ns)
	}
}

// --- frame ---

// udRow is the user decorations of one laid out row: those touching its
// model line, in paint order, and their fingerprint.
type udRow struct {
	ds  []decorationEntryOf
	sig uint64
}

// userDecos returns the decorations of the laid out rows (nil when there
// are none): per row, those of its model line that are shown, in paint
// order (ZIndex, owner name, index).
func (w *ViewW) userDecos(snap buffer.Snapshot, lines []lineView) []udRow {
	st := w.st
	m := st.Model
	if !m.hasDecorations() || len(lines) == 0 {
		return nil
	}
	lo, hi := -1, -1
	for k := range lines {
		if lines[k].zone != nil {
			continue
		}
		if lo < 0 {
			lo = lines[k].i
		}
		hi = lines[k].i
	}
	if lo < 0 {
		return nil
	}
	vs := w.vs
	vs.udq = m.decos.query(snap.LineStart(lo), lineEnd(snap, hi), st, vs.udq[:0])
	if len(vs.udq) == 0 {
		return nil
	}
	names := m.decos.names
	out := make([]udRow, len(lines))
	cur := -1
	var row udRow
	for k := range lines {
		lv := &lines[k]
		if lv.zone != nil {
			continue
		}
		if lv.i != cur {
			cur = lv.i
			ls := snap.LineStart(lv.i)
			le := ls + len(lv.text)
			var ds []decorationEntryOf
			for _, e := range vs.udq {
				if e.start > le {
					break
				}
				if e.end < ls || e.start == e.end && !e.opts.o.ShowIfCollapsed {
					continue
				}
				ds = append(ds, e)
			}
			slices.SortStableFunc(ds, func(a, b decorationEntryOf) int {
				if a.opts.o.ZIndex != b.opts.o.ZIndex {
					return a.opts.o.ZIndex - b.opts.o.ZIndex
				}
				if na, nb := names[a.owner].name, names[b.owner].name; na != nb {
					if na < nb {
						return -1
					}
					return 1
				}
				if (a.view == nil) != (b.view == nil) {
					if a.view == nil {
						return -1
					}
					return 1
				}
				return int(a.idx) - int(b.idx)
			})
			h := uint64(14695981039346656037)
			mix := func(v uint64) {
				h ^= v
				h *= 1099511628211
			}
			for _, e := range ds {
				mix(e.opts.fp)
				mix(uint64(e.start - ls))
				mix(uint64(e.end - ls))
			}
			row = udRow{ds: ds}
			if len(ds) > 0 {
				row.sig = h | 1
			}
		}
		out[k] = row
	}
	return out
}

// paintWholeLineDecos draws the whole-line backgrounds of the rows.
func (w *ViewW) paintWholeLineDecos(l *paint.List, lines []lineView, ud []udRow, lineY func(int) float32, lh float32) {
	if ud == nil {
		return
	}
	text := w.vs.text
	for k := range lines {
		for _, e := range ud[k].ds {
			o := &e.opts.o
			if !o.WholeLine || o.Background.A == 0 && o.Border.A == 0 {
				continue
			}
			box := rectXYWH(text.X, lineY(lines[k].vi), text.W, lh)
			if o.Background.A > 0 {
				l.Fill(box, geom.Radii{}, o.Background)
			}
			if o.Border.A > 0 {
				decoStroke(l, box, o.Border)
			}
		}
	}
}

// paintRangeDecos draws the range backgrounds of the rows (decoRects: the
// injected gaps inside a range are covered; on a long line, the shaped
// chunks).
func (w *ViewW) paintRangeDecos(l *paint.List, snap buffer.Snapshot, lines []lineView, ud []udRow, tx float32, lineY func(int) float32, lh float32) {
	if ud == nil {
		return
	}
	for k := range lines {
		lv := &lines[k]
		if len(ud[k].ds) == 0 {
			continue
		}
		ls := snap.LineStart(lv.i)
		y := lineY(lv.vi)
		for _, e := range ud[k].ds {
			o := &e.opts.o
			if o.WholeLine || o.Background.A == 0 && o.Border.A == 0 || e.start == e.end {
				continue
			}
			a, b := max(e.start, ls)-ls, min(e.end-ls, len(lv.text))
			decoRects(lv, a, b, func(x0, x1 float32) {
				box := rectXYWH(tx+x0, y, x1-x0, lh)
				if o.Background.A > 0 {
					l.Fill(box, geom.Radii{}, o.Background)
				}
				if o.Border.A > 0 {
					decoStroke(l, box, o.Border)
				}
			})
		}
	}
}

// inlineSpans returns the inline styles of row k (line runes, in paint
// order), appended to dst.
func inlineSpans(snap buffer.Snapshot, lv *lineView, ud []udRow, k int, dst []inlineSpan) []inlineSpan {
	if ud == nil || len(ud[k].ds) == 0 {
		return dst
	}
	ls := snap.LineStart(lv.i)
	for _, e := range ud[k].ds {
		in := e.opts.o.Inline
		if in == nil || e.start == e.end {
			continue
		}
		a, b := max(e.start, ls)-ls, min(e.end-ls, len(lv.text))
		if a >= b || a >= lv.b1 || b <= lv.b0 {
			continue
		}
		dst = append(dst, inlineSpan{r0: lv.runeAt(a), r1: lv.runeAt(b), st: in})
	}
	return dst
}

// gutterDecos returns, for a row's decorations, the margin background, the
// line number color (transparent: unchanged) and the marks, in paint order.
func gutterDecos(ds []decorationEntryOf, marks []*GutterMark) (margin []color.NRGBA, num color.NRGBA, out []*GutterMark) {
	for _, e := range ds {
		o := &e.opts.o
		if o.MarginBackground.A > 0 {
			margin = append(margin, o.MarginBackground)
		}
		if o.LineNumberColor.A > 0 {
			num = o.LineNumberColor
		}
		if o.Gutter != nil {
			marks = append(marks, o.Gutter)
		}
	}
	return margin, num, marks
}

// inlineSpan is an inline style over the line runes [r0, r1) of a row.
type inlineSpan struct {
	r0, r1 int
	st     *InlineStyle
}

// inlineColor returns fg with the inline styles of rune r applied (the
// last one setting a color wins; opacities multiply).
func inlineColor(fg color.NRGBA, r int, ins []inlineSpan) color.NRGBA {
	for _, in := range ins {
		if r < in.r0 || r >= in.r1 {
			continue
		}
		if in.st.Color.A > 0 {
			fg = in.st.Color
		}
		if o := in.st.Opacity; o > 0 && o < 1 {
			fg.A = uint8(math.Round(float64(fg.A) * float64(o)))
		}
	}
	return fg
}

// drawSegInline is drawSeg with the inline styles ins over the line's
// runes: colors and opacities per glyph, then their underlines and
// strikethroughs. The layout is unchanged.
func drawSegInline(l *paint.List, sg seg, tx, y float32, ins []inlineSpan) {
	if len(ins) == 0 {
		drawSeg(l, sg, tx, y)
		return
	}
	lay := sg.lay
	org := geom.Pt(tx+sg.x0, y)
	lo, hi := sg.lr, sg.lr+sg.r1-sg.r0
	// glyphRune returns the line rune of glyph g, -1 when unknown.
	glyphRune := func(g int) int {
		if g >= len(lay.GlyphCluster) {
			return -1
		}
		return sg.r0 + int(lay.GlyphCluster[g]) - sg.lr
	}
	in := func(g int) bool {
		if !sg.piece || g >= len(lay.GlyphCluster) {
			return true
		}
		c := int(lay.GlyphCluster[g])
		return c >= lo && c < hi
	}
	colorOf := func(g int) color.NRGBA {
		run := int32(0)
		if g < len(lay.GlyphRun) {
			run = lay.GlyphRun[g]
		}
		fg := color.NRGBA{A: 255}
		if int(run) < len(sg.paints) {
			fg = sg.paints[run].fg
		}
		if r := glyphRune(g); r >= 0 {
			fg = inlineColor(fg, r, ins)
		}
		return fg
	}
	for g := 0; g < len(lay.Glyphs); {
		if !in(g) {
			g++
			continue
		}
		fg := colorOf(g)
		e := g + 1
		for e < len(lay.Glyphs) && in(e) && colorOf(e) == fg {
			e++
		}
		l.Glyphs(org, lay.Glyphs[g:e], fg)
		g = e
	}
	if len(lay.Lines) == 0 {
		return
	}
	base := lay.Lines[0].Baseline
	line := func(r0, r1 int, yy float32, c color.NRGBA) {
		x0, _, _ := lay.Caret(sg.local(r0))
		x1, _, _ := lay.Caret(sg.local(r1))
		x0, x1 = min(x0, x1), max(x0, x1)
		l.Line(geom.Pt(org.X+x0, yy), geom.Pt(org.X+x1, yy), 1, c)
	}
	// The runs' own underlines and strikethroughs (as drawSeg).
	for _, p := range sg.paints {
		if !p.underline && !p.strike {
			continue
		}
		r0, r1 := p.r0, p.r1
		if sg.piece {
			r0, r1 = max(r0, lo), min(r1, hi)
			if r0 >= r1 {
				continue
			}
		}
		x0, _, _ := lay.Caret(r0)
		x1, _, _ := lay.Caret(r1)
		x0, x1 = min(x0, x1), max(x0, x1)
		if p.underline {
			yy := y + base + 2
			l.Line(geom.Pt(org.X+x0, yy), geom.Pt(org.X+x1, yy), 1, p.fg)
		}
		if p.strike {
			yy := y + base - lay.Ascent*0.3
			l.Line(geom.Pt(org.X+x0, yy), geom.Pt(org.X+x1, yy), 1, p.fg)
		}
	}
	// The inline styles' lines, in the color of their first glyph.
	for _, sp := range ins {
		if !sp.st.Underline && !sp.st.Strikethrough {
			continue
		}
		r0, r1 := max(sp.r0, sg.r0), min(sp.r1, sg.r1)
		if r0 >= r1 {
			continue
		}
		c := sp.st.Color
		if c.A == 0 {
			c = color.NRGBA{A: 255}
			for _, p := range sg.paints {
				if j := sg.local(r0); j >= p.r0 && j < p.r1 {
					c = p.fg
					break
				}
			}
		}
		c = inlineColor(c, r0, ins)
		if sp.st.Underline {
			line(r0, r1, y+base+2, c)
		}
		if sp.st.Strikethrough {
			line(r0, r1, y+base-lay.Ascent*0.3, c)
		}
	}
}

// --- overview ruler ---

// udMark is a decoration's overview ruler mark: its lines and its style
// (an index in udOverview.keys).
type udMark struct {
	l0, l1 int
	key    int
}

// udKey is an overview mark style.
type udKey struct {
	c    color.NRGBA
	lane int
}

// udOverview caches the overview ruler marks of the decorations a view
// shows, per decoration store version and text version: the decorations
// are not walked every frame.
type udOverview struct {
	st          *State
	store, snap uint64
	sets        uint64    // the store's sets when built
	built       time.Time // when built
	ok          bool
	marks       []udMark
	keys        []udKey
	runs        []udRun // scratch of paintOverviewDecos, by key
}

type udRun struct {
	y0, y1 float32
	on     bool
}

// overviewDecos returns the overview ruler marks of the decorations st
// shows, sorted by first line, and their styles (cached).
func (w *ViewW) overviewDecos() *udOverview {
	st, m := w.st, w.st.Model
	if !m.hasDecorations() {
		return nil
	}
	c := &w.vs.udOv
	snap := st.Snapshot()
	if c.ok && c.st == st && c.store == m.decos.version && c.snap == snap.Version() {
		return c
	}
	// The ranges only moved with edits: typing rebuilds the marks (a
	// LineOf per decoration) at most every overviewDecoDelay; the marks
	// meanwhile are those before the edits (the ruler is a coarse
	// picture; its redraw hints compare what is drawn).
	now := w.c.Now()
	if c.ok && c.st == st && c.sets == m.decos.sets {
		if due := c.built.Add(overviewDecoDelay); now.Before(due) {
			w.c.WakeAt(due)
			return c
		}
	}
	*c = udOverview{st: st, store: m.decos.version, snap: snap.Version(), sets: m.decos.sets, built: now, ok: true,
		marks: c.marks[:0], keys: c.keys[:0], runs: c.runs}
	idx := map[udKey]int{}
	add := func(es []decorationEntryOf) {
		for i := range es {
			e := &es[i]
			o := &e.opts.o
			if o.Overview == nil || o.Overview.Color.A == 0 || e.view != nil && e.view != st || e.start == e.end && !o.ShowIfCollapsed {
				continue
			}
			k := udKey{o.Overview.Color, o.Overview.Lane}
			ki, ok := idx[k]
			if !ok {
				ki = len(c.keys)
				idx[k] = ki
				c.keys = append(c.keys, k)
			}
			c.marks = append(c.marks, udMark{l0: snap.LineOf(e.start), l1: snap.LineOf(e.end), key: ki})
		}
	}
	add(m.decos.es)
	add(m.decos.big)
	slices.SortStableFunc(c.marks, func(a, b udMark) int { return a.l0 - b.l0 })
	c.marks = slices.Compact(c.marks)
	return c
}

// overviewDecoDelay is how often typing rebuilds the decorations' overview
// ruler marks (overviewDecos).
const overviewDecoDelay = 250 * time.Millisecond

// paintOverviewDecos draws the decorations' overview marks with draw
// (lines l0..l1 in color c on lanes lane), merging the marks of one color
// and lane that overlap on the ruler (at most a few per pixel row).
func (w *ViewW) paintOverviewDecos(y func(line int) float32, draw func(y0, y1 float32, c color.NRGBA, lane int)) {
	c := w.overviewDecos()
	if c == nil || len(c.marks) == 0 {
		return
	}
	runs := append(c.runs[:0], make([]udRun, len(c.keys))...)
	c.runs = runs
	var order []int // keys by first mark (draw order)
	for _, m := range c.marks {
		y0 := y(m.l0)
		y1 := y0
		if m.l1 != m.l0 {
			y1 = y(m.l1)
		}
		y1 += 3
		r := &runs[m.key]
		switch {
		case !r.on:
			*r = udRun{y0, y1, true}
			order = append(order, m.key)
		case y0 <= r.y1:
			r.y1 = max(r.y1, y1)
		default:
			k := c.keys[m.key]
			draw(r.y0, r.y1, k.c, k.lane)
			*r = udRun{y0, y1, true}
		}
	}
	for _, i := range order {
		k := c.keys[i]
		draw(runs[i].y0, runs[i].y1, k.c, k.lane)
	}
}
