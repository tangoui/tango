package diffview

import (
	"fmt"
	"image/color"
	"strconv"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor"
	"github.com/tangoui/tango/ui/editor/diff"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

// texts are the view's interface texts, from the locale (Locale.T with
// the keys below; the fallbacks are Simplified Chinese, the default
// locale).
type texts struct {
	hidden, showAbove, showBelow, showAll string
	original, originalEditable, modified  string
	revert, unnamed                       string
	ann                                   announceText
	hash                                  uint64
}

func textsOf(l *ui.Locale) texts {
	t := func(key, fallback string) string { return l.T(key, fallback) }
	tx := texts{
		hidden:           t("diffview.hidden", "已隐藏 %d 行"),
		showAbove:        t("diffview.showAbove", "展开上方 %d 行"),
		showBelow:        t("diffview.showBelow", "展开下方 %d 行"),
		showAll:          t("diffview.showAll", "全部展开"),
		original:         t("diffview.original", "原文（只读）— %s"),
		originalEditable: t("diffview.originalEditable", "原文 — %s"),
		modified:         t("diffview.modified", "修改后 — %s"),
		revert:           t("diffview.revert", "还原此处更改"),
		unnamed:          t("diffview.unnamed", "未命名"),
		ann:              announceTexts(t),
	}
	tx.hash = hashString(tx.hidden + tx.showAbove + tx.showBelow + tx.showAll)
	return tx
}

// Gutter geometry of the editor that the zones' gutter parts draw in:
// with folding off (the diff view turns it off) the gutter is the line
// numbers right aligned, then the line decorations margin of
// lineDecorationsW pixels (Monaco's linesDecorationsWidth, ui/editor's
// gutter.go), where the "+" and "−" signs go.
const lineDecorationsW = 18

// glyphCache keeps the shaped strings the zones draw.
type glyphCache map[glyphKey]*text.Layout

type glyphKey struct {
	s    string
	size float32
	mono bool
}

func (g *glyphCache) shape(s string, size float32, mono bool) *text.Layout {
	if *g == nil || len(*g) > 512 {
		*g = glyphCache{}
	}
	k := glyphKey{s, size, mono}
	if lay, ok := (*g)[k]; ok {
		return lay
	}
	lay := text.Shape(s, text.Style{Size: size, Mono: mono}, text.Options{NoWrap: true})
	(*g)[k] = lay
	return lay
}

// fontSize returns the size of the text in zones of rows rowH high (the
// editor's line height is 1.5 times its font size).
func fontSize(rowH float32) float32 { return max(8, float32(int(rowH/1.5+0.5))) }

// drawText draws lay vertically centred in a row at y, its left edge at x.
func drawText(l *paint.List, lay *text.Layout, x, y, rowH float32, c color.NRGBA) {
	l.Glyphs(geom.Pt(x, y+(rowH-lay.Height)/2), lay.Glyphs, c)
}

// --- placeholders ---

// placeholder is the bar of a folded unchanged region on one side ("⋯ 42
// hidden lines" and its buttons; Monaco's CollapsedCodeOverlayWidget,
// one row high here).
type placeholder struct {
	region             int
	side               diff.Side
	hidden             int
	canAbove, canBelow bool
	hover, press       int           // button under the pointer / pressed, -1 none
	btn                [3][2]float32 // the buttons' x ranges from the bar's left, last paint
	x                  float32       // the bar's left edge in the window, last paint
}

// The placeholder's buttons.
const (
	btnAbove = iota
	btnAll
	btnBelow
)

func (p *placeholder) zone(d *State, id uint64, after int) editor.ViewZone {
	ly := &d.lay
	tx := &ly.texts
	rtl := ly.rtl
	cols := ly.cols
	reveal := max(d.Options.HideUnchangedRegions.RevealLineCount, 1)
	label := fmt.Sprintf(tx.hidden, p.hidden)
	btns := [3]string{fmt.Sprintf(tx.showAbove, reveal), tx.showAll, fmt.Sprintf(tx.showBelow, reveal)}
	hover := p.hover
	b2u := func(b bool) uint64 {
		if b {
			return 1
		}
		return 0
	}
	return editor.ViewZone{
		ID: id, AfterLine: after, Rows: 1, Fixed: true, ShowInHidden: true, SuppressMouseDown: true,
		Cursor:  input.CursorPointer,
		Label:   label,
		Version: hashOf(uint64(p.hidden), uint64(hover+1), cols.hash, tx.hash, b2u(rtl), uint64(reveal)) | 1,
		Paint: func(l *paint.List, z editor.ZonePaint) {
			r := z.Rect
			p.x = r.X
			if cols.regionBg.A > 0 {
				l.Fill(r, geom.Radii{}, cols.regionBg)
			}
			l.Fill(geom.Rect{X: r.X, Y: r.Y, W: r.W, H: 1}, geom.Radii{}, cols.regionShadow)
			l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, cols.regionShadow)
			fs := fontSize(z.RowH) - 1
			// From the leading edge: "⋯ N hidden lines", then the buttons.
			const pad, gap = 12, 16
			lay := ly.glyphs.shape("⋯ "+label, fs, false)
			x := float32(pad)
			lx := x
			if rtl {
				lx = r.W - x - lay.Width
			}
			drawText(l, lay, r.X+lx, r.Y, z.RowH, cols.regionFg)
			x += lay.Width + 2*gap
			for i, s := range btns {
				bl := ly.glyphs.shape(s, fs, false)
				x0 := x
				if rtl {
					x0 = r.W - x - bl.Width
				}
				p.btn[i] = [2]float32{x0 - gap/2, x0 + bl.Width + gap/2}
				c := cols.link
				if i == hover {
					c = cols.linkHover
					l.Fill(geom.Rect{X: r.X + x0, Y: r.Y + z.RowH - 4, W: bl.Width, H: 1}, geom.Radii{}, c)
				}
				drawText(l, bl, r.X+x0, r.Y, z.RowH, c)
				x += bl.Width + gap
			}
		},
		OnPointer: func(e editor.ZonePointer) {
			h := -1
			for i, b := range p.btn {
				if e.Pos.X >= b[0] && e.Pos.X < b[1] {
					h = i
				}
			}
			switch e.Kind {
			case input.Move:
				if h != p.hover {
					p.hover = h
					ly.dirty = true
				}
			case input.Press:
				p.press = h
			case input.Release:
				if h >= 0 && h == p.press {
					d.placeholderAction(p.region, h)
				}
				p.press = -1
			}
		},
	}
}

// placeholderAction runs button b of region ri's placeholder.
func (d *State) placeholderAction(ri, b int) {
	if ri < 0 || ri >= len(d.regions.regions) {
		return
	}
	g := &d.regions.regions[ri]
	k := max(d.Options.HideUnchangedRegions.RevealLineCount, 1)
	switch b {
	case btnAbove:
		g.showMoreAbove(k)
	case btnBelow:
		g.showMoreBelow(k)
	default:
		g.showAll()
	}
	d.regionGen++
	d.wake()
}

// --- hunk action bars ---

// HunkAction is an action a host offers on a change (View.HunkActions):
// accepting or rejecting an AI edit, taking a side of a merge conflict.
type HunkAction struct {
	Label   string
	Tooltip string
	// Run runs on the UI goroutine when the action is clicked; the view
	// recomputes the diff right after.
	Run func()
}

// actionBar is the row above a change that shows its hunk actions (on the
// modified side; the original side has a blank row to stay aligned).
type actionBar struct {
	change       int
	side         diff.Side
	actions      []HunkAction
	hover, press int
	btn          [][2]float32
	x            float32 // the bar's left edge in the window, last paint
}

func (b *actionBar) zone(d *State, id uint64, after int) editor.ViewZone {
	ly := &d.lay
	cols := ly.cols
	rtl := ly.rtl
	z := editor.ViewZone{ID: id, AfterLine: after, Rows: 1, Fixed: true, ShowInHidden: true, SuppressMouseDown: true,
		Version: hashOf(cols.hash, 7) | 1}
	if len(b.actions) == 0 {
		return z
	}
	acts := b.actions
	hover := b.hover
	vals := []uint64{cols.hash, uint64(hover + 1), uint64(len(acts))}
	for _, a := range acts {
		vals = append(vals, hashString(a.Label))
	}
	if rtl {
		vals = append(vals, 1)
	}
	z.Version = hashOf(vals...) | 1
	z.Cursor = input.CursorPointer
	z.Label = acts[0].Label
	z.Paint = func(l *paint.List, zp editor.ZonePaint) {
		r := zp.Rect
		b.x = r.X
		fs := fontSize(zp.RowH) - 1
		const pad, gap = 8, 12
		x := float32(pad)
		b.btn = b.btn[:0]
		for i, a := range acts {
			lay := ly.glyphs.shape(a.Label, fs, false)
			x0 := x
			if rtl {
				x0 = r.W - x - lay.Width
			}
			b.btn = append(b.btn, [2]float32{x0 - gap/2, x0 + lay.Width + gap/2})
			c := cols.link
			if i == hover {
				c = cols.linkHover
				l.Fill(geom.Rect{X: r.X + x0, Y: r.Y + zp.RowH - 4, W: lay.Width, H: 1}, geom.Radii{}, c)
			}
			drawText(l, lay, r.X+x0, r.Y, zp.RowH, c)
			x += lay.Width + gap
		}
	}
	z.OnPointer = func(e editor.ZonePointer) {
		h := -1
		for i, r := range b.btn {
			if e.Pos.X >= r[0] && e.Pos.X < r[1] {
				h = i
			}
		}
		switch e.Kind {
		case input.Move:
			if h != b.hover {
				b.hover = h
				ly.dirty = true
			}
		case input.Press:
			b.press = h
		case input.Release:
			if h >= 0 && h == b.press && h < len(acts) && acts[h].Run != nil {
				acts[h].Run()
				d.recompute()
			}
			b.press = -1
		}
	}
	return z
}

// recompute starts a new diff of the current texts without the debounce
// delay (after a revert or a hunk action).
func (d *State) recompute() {
	d.run.now(d.models[0].Snapshot(), d.models[1].Snapshot())
}

// --- inline deleted lines ---

// deletedZone shows the deleted lines of a change in Inline mode: the
// original's lines drawn with its syntax colors (editor.ModelLines) on the
// removed-line background, the original line numbers and "−" in the
// gutter. The lines do not wrap (docs/specs/diff-view.md, Q9).
type deletedZone struct {
	change   int
	from, to int // original lines
	after    int // modified line
	paint    func(*paint.List, editor.ZonePaint)
	version  uint64
}

// deletedZones returns the deleted lines' zones of the current result.
func (d *State) deletedZones() []*deletedZone {
	src := d.models[diff.Original]
	snap := src.Snapshot()
	n := snap.LineCount()
	opts := d.lay.opts.text[diff.Original]
	var out []*deletedZone
	var decos []editor.Decoration
	for i, c := range d.res.Changes {
		if c.Original.Empty() || c.Original.Start >= n {
			continue
		}
		decos = decos[:0]
		for _, in := range c.Inner {
			a, b := snap.OffsetOf(in.Original.Start), snap.OffsetOf(in.Original.End)
			if in.Original.End.Line >= n {
				b = snap.Len()
			}
			if b > a {
				decos = append(decos, editor.Decoration{Start: a, End: b, Options: opts})
			}
		}
		dz := &deletedZone{change: i, from: c.Original.Start, to: min(c.Original.End, n), after: c.Modified.Start - 1}
		dz.paint, dz.version = editor.ModelLines(src, dz.from, dz.to, decos)
		out = append(out, dz)
	}
	return out
}

func (dz *deletedZone) zone(d *State) editor.ViewZone {
	ly := &d.lay
	cols := ly.cols
	indicators := d.Options.RenderIndicators
	var version uint64
	if dz.version != 0 {
		version = hashOf(dz.version, cols.hash, uint64(dz.from), uint64(dz.to)) | 1
	}
	from, to := dz.from, dz.to
	st := d.Modified
	gutBg := cols.gutterRemoved
	ml := dz.paint
	return editor.ViewZone{
		ID: idDeleted | uint64(dz.change), AfterLine: dz.after, Rows: to - from, ShowInHidden: true, SuppressMouseDown: true,
		Version: version,
		Paint: func(l *paint.List, z editor.ZonePaint) {
			l.Fill(z.Rect, geom.Radii{}, cols.removedLine)
			ml(l, z)
		},
		PaintGutter: func(l *paint.List, z editor.ZonePaint) {
			r := z.Rect
			l.Fill(r, geom.Radii{}, gutBg)
			fs := fontSize(z.RowH)
			right := r.Right() - lineDecorationsW
			k1 := min(to-from, z.FirstRow+int(st.Viewport.Height/z.RowH)+2)
			for k := z.FirstRow; k < k1; k++ {
				y := r.Y + float32(k)*z.RowH
				lay := ly.glyphs.shape(strconv.Itoa(from+k+1), fs, true)
				drawText(l, lay, right-lay.Width, y, z.RowH, cols.lineNumber)
				if indicators {
					sg := ly.glyphs.shape("−", fs, true)
					drawText(l, sg, right+(lineDecorationsW-sg.Width)/2, y, z.RowH, cols.sign)
				}
			}
		},
	}
}

// refreshDeleted asks again for the drawings of the visible deleted lines
// whose tokens were not final (their zone Version is 0, fingerprinted
// every frame), and sets the zones again once they are.
func (d *State) refreshDeleted() {
	ly := &d.lay
	if len(ly.deleted) == 0 {
		return
	}
	st := d.Modified
	first, _ := st.LineAtTop()
	rows := 1
	if lh := st.Viewport.LineHeight; lh > 0 {
		rows = int(st.Viewport.Height/lh) + 1
	}
	for _, dz := range ly.deleted {
		if dz.version != 0 || dz.after < first-rows || dz.after > first+2*rows {
			continue
		}
		if _, v := editor.ModelLines(d.models[diff.Original], dz.from, dz.to, nil); v != 0 {
			ly.dirty = true
			break
		}
	}
	if ly.dirty {
		d.apply(Inline)
	}
}
