package diffview

import (
	"hash/maphash"
	"image/color"
	"math"
	"slices"

	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/diff"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// The diff view's editor state (docs/specs/diff-view.md, 3.5–3.7): the
// line and character decorations of the changes, the hidden areas of the
// folded unchanged regions and the view zones of both sides (alignment
// padding, the "N hidden lines" placeholders, the hunk action bars and,
// inline, the deleted lines). They are owned by "diff" on each side's
// State, so a modified Model open in other editors shows none of them.

const owner = "diff"

// Zone IDs: the alignment padding keeps align's ids (a changed block's
// change<<1|side, an unchanged wrapped line's 1<<63|line<<1|side); the
// others take high bits of their own.
const (
	idBar         = 1 << 62 // | change<<1 | side: hunk action bar
	idPlaceholder = 1 << 61 // | region start (original line)<<1 | side
	idDeleted     = 1 << 60 // | change: inline deleted lines
	idWrapPart    = 1<<63 | 1<<59
)

// layoutState is what the view set on the two editor states, and when.
type layoutState struct {
	cols     colors
	opts     decoOptions
	decoKey  decoKey
	decoSet  bool
	al       *alignment
	alKey    alignKey
	zoneKey  zoneKey
	zoneSet  bool
	zones    [2][]zoneSig
	dirty    bool // a placeholder or bar changed its drawing (hover)
	places   map[uint64]*placeholder
	bars     map[uint64]*actionBar
	deleted  []*deletedZone
	actionOn bool
	fontSize float32
	glyphs   glyphCache
	rtl      bool
	texts    texts

	blockOf     []int // change -> alignment block (changeBlock)
	blockOfAl   *alignment
	revertHover int
	ov          overviewMarks
}

type decoKey struct {
	shapeGen   uint64
	mode       Mode
	cols       uint64
	indicators bool
	lines      [2]int
}

type alignKey struct {
	shapeGen uint64
	mode     Mode
	wrapped  bool
}

type zoneKey struct {
	shapeGen, regionGen uint64
	mode                Mode
	cols                uint64
	actions             bool
	hide                bool
	texts               uint64
	rtl                 bool
}

// zoneSig is what identifies a zone's layout and drawing.
type zoneSig struct {
	id        uint64
	afterLine int
	rows      int
	version   uint64
}

// decoOptions are the decoration options of the current colors (shared
// by all decorations, as Monaco registers them once).
type decoOptions struct {
	line, text [2]*editor.DecorationOptions
	ovRemoved  *editor.DecorationOptions // inline: deleted lines' overview mark
}

func (d *State) makeDecoOptions(c colors, indicators bool) decoOptions {
	var o decoOptions
	sign := [2]string{"−", "+"}
	lineBg := [2]color.NRGBA{c.removedLine, c.insertedLine}
	gutBg := [2]color.NRGBA{c.gutterRemoved, c.gutterInserted}
	textBg := [2]color.NRGBA{c.removedText, c.insertedText}
	border := [2]color.NRGBA{c.removedBorder, c.insertedBorder}
	for s := range 2 {
		lo := &editor.DecorationOptions{Description: "diff-line", WholeLine: true, Background: lineBg[s], MarginBackground: gutBg[s]}
		if indicators {
			lo.Gutter = &editor.GutterMark{Sign: sign[s], SignColor: c.sign}
		}
		o.line[s] = lo
		o.text[s] = &editor.DecorationOptions{Description: "diff-char", Stickiness: editor.NeverGrowsWhenTypingAtEdges, Background: textBg[s], Border: border[s]}
	}
	o.ovRemoved = &editor.DecorationOptions{Description: "diff-deleted", Overview: &editor.OverviewStyle{Color: c.overviewRemoved, Lane: 1}}
	return o
}

// apply brings both editor states up to date with the current result,
// regions, mode and colors. It does the work only when one of them
// changed (wrapping aside: the alignment of wrapped lines is measured
// around the viewport every frame).
func (d *State) apply(mode Mode) {
	if !d.hasRes {
		return
	}
	ly := &d.lay
	wrapped := mode == SideBySide && (d.Original.WordWrap() != editor.WrapOff || d.Modified.WordWrap() != editor.WrapOff)

	dk := decoKey{shapeGen: d.shapeGen, mode: mode, cols: ly.cols.hash, indicators: d.Options.RenderIndicators,
		lines: [2]int{d.Original.Snapshot().LineCount(), d.Modified.Snapshot().LineCount()}}
	if !ly.decoSet || dk != ly.decoKey {
		if !ly.decoSet || dk.cols != ly.decoKey.cols || dk.indicators != ly.decoKey.indicators {
			ly.opts = d.makeDecoOptions(ly.cols, dk.indicators)
		}
		ly.decoKey, ly.decoSet = dk, true
		for s := diff.Original; s <= diff.Modified; s++ {
			d.side(s).SetDecorations(owner, d.decorations(s, mode))
		}
	}

	d.applyHidden(mode)

	ak := alignKey{shapeGen: d.shapeGen, mode: mode, wrapped: wrapped}
	if ly.al == nil || ak != ly.alKey || wrapped {
		var rows func(diff.Side, int) int
		if wrapped {
			rows = d.rowsNearViewport()
		}
		al := align(d.res, rows, wrapped)
		if ly.al != nil && wrapped && ak == ly.alKey && samePads(al, ly.al) {
			al = ly.al
		}
		if al != ly.al {
			ly.al, ly.alKey = al, ak
			ly.zoneSet = false
		}
	}

	o := d.Options.HideUnchangedRegions
	zk := zoneKey{shapeGen: d.shapeGen, regionGen: d.regionGen, mode: mode, cols: ly.cols.hash, actions: d.actions != nil,
		hide: o.Enabled, texts: ly.texts.hash, rtl: ly.rtl}
	if !ly.zoneSet || zk != ly.zoneKey || ly.dirty {
		ly.zoneKey, ly.zoneSet, ly.dirty = zk, true, false
		d.setZones(mode)
	}
}

// samePads reports whether two alignments pad the same lines by the same
// rows.
func samePads(a, b *alignment) bool {
	return slices.Equal(a.pads[0], b.pads[0]) && slices.Equal(a.pads[1], b.pads[1])
}

// rowsNearViewport returns the row counts align uses while wrapping:
// measured (editor.State.RowsOf) for the lines from a viewport above to a
// viewport below each side's viewport, 1 (the editor's estimate for lines
// not measured yet) elsewhere.
func (d *State) rowsNearViewport() func(diff.Side, int) int {
	var lo, hi [2]int
	for s, st := range d.sides() {
		line, _ := st.LineAtTop()
		page := 1
		if lh := st.Viewport.LineHeight; lh > 0 {
			page = int(st.Viewport.Height/lh) + 1
		}
		lo[s], hi[s] = line-page, line+2*page
	}
	return func(s diff.Side, line int) int {
		if line < lo[s] || line > hi[s] {
			return 1
		}
		return d.side(s).RowsOf(line) // 0 for a hidden line, on both sides
	}
}

// decorations returns side s's decorations of the current result.
func (d *State) decorations(s diff.Side, mode Mode) []editor.Decoration {
	st := d.side(s)
	snap := st.Snapshot()
	n := snap.LineCount()
	o := &d.lay.opts
	var ds []editor.Decoration
	for _, c := range d.res.Changes {
		lr := c.Original
		if s == diff.Modified {
			lr = c.Modified
		}
		if !lr.Empty() && lr.Start < n {
			ds = append(ds, editor.LineDecoration(snap, lr.Start, min(lr.End, n)-1, o.line[s]))
		}
		for _, in := range c.Inner {
			r := in.Original
			if s == diff.Modified {
				r = in.Modified
			}
			if r.Start.Line >= n {
				continue
			}
			a, b := snap.OffsetOf(r.Start), snap.OffsetOf(r.End)
			if r.End.Line >= n {
				b = snap.Len()
			}
			if b > a {
				ds = append(ds, editor.Decoration{Start: a, End: b, Options: o.text[s]})
			}
		}
		if mode == Inline && s == diff.Modified && !c.Original.Empty() {
			ds = append(ds, editor.LineDecoration(snap, max(c.Modified.Start-1, 0), max(c.Modified.Start-1, 0), o.ovRemoved))
		}
	}
	return ds
}

// applyHidden hides the folded unchanged regions on both sides.
func (d *State) applyHidden(mode Mode) {
	enabled := d.Options.HideUnchangedRegions.Enabled
	for s := diff.Original; s <= diff.Modified; s++ {
		st := d.side(s)
		var want []editor.LineRange
		if enabled {
			for _, h := range d.regions.hidden(s) {
				want = append(want, editor.LineRange{Start: h.Start, End: h.End})
			}
		}
		if !slices.Equal(want, st.HiddenAreas(owner)) {
			st.SetHiddenAreas(owner, want)
			d.needSync = true
		}
	}
}

// zoneEntry is a zone of a side with its place among the zones after the
// same line: the padding of a wrapped line, then a hunk action bar (above
// its change), then a changed block's padding (at the block's bottom),
// then a placeholder (it belongs to the unchanged lines below).
type zoneEntry struct {
	prio int
	z    editor.ViewZone
}

// setZones sets the zones of both sides (only those that changed are
// set again, see editor.State.SetViewZones).
func (d *State) setZones(mode Mode) {
	ly := &d.lay
	var es [2][]zoneEntry
	add := func(s diff.Side, prio int, z editor.ViewZone) {
		es[s] = append(es[s], zoneEntry{prio: prio, z: z})
	}
	// Alignment padding (side by side) or deleted lines (inline).
	if mode == SideBySide {
		for s := diff.Original; s <= diff.Modified; s++ {
			for _, p := range ly.al.pads[s] {
				changeRows := 0
				if p.id>>63 == 0 {
					changeRows = ly.al.changePad[s][p.block]
				}
				if w := p.rows - changeRows; w > 0 {
					add(s, 0, d.padZone(s, idWrapPart|uint64(p.afterLine)<<1|uint64(s), p.afterLine, w))
				}
				if changeRows > 0 {
					add(s, 2, d.padZone(s, p.id, p.afterLine, changeRows))
				}
			}
		}
		ly.deleted = nil
	} else {
		ly.deleted = d.deletedZones()
		for _, dz := range ly.deleted {
			add(diff.Modified, 2, dz.zone(d))
		}
	}
	// Hunk action bars, on both sides (side by side) to keep them aligned.
	bars := map[uint64]*actionBar{}
	if d.actions != nil {
		for i, c := range d.res.Changes {
			acts := d.actions(i, c)
			if len(acts) == 0 {
				continue
			}
			for s := diff.Original; s <= diff.Modified; s++ {
				if mode == Inline && s == diff.Original {
					continue
				}
				lr := c.Original
				if s == diff.Modified {
					lr = c.Modified
				}
				id := idBar | uint64(i)<<1 | uint64(s)
				b := ly.bars[id]
				if b == nil {
					b = &actionBar{hover: -1, press: -1}
				}
				b.change, b.side = i, s
				b.actions = nil
				if s == diff.Modified {
					b.actions = acts
				}
				bars[id] = b
				add(s, 1, b.zone(d, id, lr.Start-1))
			}
		}
	}
	ly.bars = bars
	// Placeholders of the folded regions.
	places := map[uint64]*placeholder{}
	if d.Options.HideUnchangedRegions.Enabled {
		for ri, g := range d.regions.regions {
			for s := diff.Original; s <= diff.Modified; s++ {
				if mode == Inline && s == diff.Original {
					continue
				}
				h := g.hidden(s)
				if h.Empty() {
					continue
				}
				id := idPlaceholder | uint64(g.orig)<<1 | uint64(s)
				p := ly.places[id]
				if p == nil {
					p = &placeholder{hover: -1, press: -1}
				}
				p.region, p.side, p.hidden = ri, s, h.Len()
				p.canAbove, p.canBelow = true, true
				places[id] = p
				add(s, 3, p.zone(d, id, h.Start-1))
			}
		}
	}
	ly.places = places

	for s := diff.Original; s <= diff.Modified; s++ {
		if mode == Inline && s == diff.Original {
			continue
		}
		e := es[s]
		slices.SortStableFunc(e, func(a, b zoneEntry) int {
			if a.z.AfterLine != b.z.AfterLine {
				return a.z.AfterLine - b.z.AfterLine
			}
			return a.prio - b.prio
		})
		zs := make([]editor.ViewZone, len(e))
		sig := make([]zoneSig, len(e))
		for i := range e {
			zs[i] = e[i].z
			sig[i] = zoneSig{id: zs[i].ID, afterLine: zs[i].AfterLine, rows: zs[i].Rows, version: zs[i].Version}
		}
		if slices.Equal(sig, ly.zones[s]) && sig != nil {
			continue
		}
		ly.zones[s] = sig
		d.side(s).SetViewZones(owner, zs)
		d.needSync = true
	}
}

// padZone returns an alignment padding zone: blank rows with Monaco's
// diagonal fill.
func (d *State) padZone(s diff.Side, id uint64, after, rows int) editor.ViewZone {
	st := d.side(s)
	col := d.lay.cols.diagonal
	return editor.ViewZone{
		ID: id, AfterLine: after, Rows: rows, ShowInHidden: true, SuppressMouseDown: true,
		Version: d.lay.cols.hash | 1,
		Paint: func(l *paint.List, z editor.ZonePaint) {
			paintDiagonal(l, z, st.Viewport.Height, col)
		},
	}
}

// paintDiagonal fills the visible rows of a zone with diagonal stripes
// (diffEditor.diagonalFill, Monaco's 8px linear-gradient pattern),
// anchored at the zone's corner so that scrolling moves them with it.
func paintDiagonal(l *paint.List, z editor.ZonePaint, viewH float32, col color.NRGBA) {
	if col.A == 0 {
		return
	}
	const step = 8
	r := z.Rect
	y0 := r.Y + float32(z.FirstRow)*z.RowH
	y1 := min(r.Bottom(), y0+viewH+z.RowH)
	ya, yb := y0-step, y1+step
	k0 := int(math.Floor(float64(ya-r.Y) / step))
	k1 := int(math.Ceil(float64(r.W+yb-r.Y) / step))
	for k := k0; k <= k1; k++ {
		c := float32(k) * step
		// The stripe x - r.X + y - r.Y = c.
		l.Line(geom.Pt(r.X+c-(ya-r.Y), ya), geom.Pt(r.X+c-(yb-r.Y), yb), 1, col)
	}
}

var seed = maphash.MakeSeed()

// hashOf hashes values for zone versions.
func hashOf(vals ...uint64) uint64 {
	var h maphash.Hash
	h.SetSeed(seed)
	var b [8]byte
	for _, v := range vals {
		for i := range b {
			b[i] = byte(v >> (8 * i))
		}
		h.Write(b[:])
	}
	return h.Sum64()
}

func hashString(s string) uint64 { return maphash.String(seed, s) }
