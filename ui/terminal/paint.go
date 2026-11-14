package terminal

import (
	"container/list"
	"strings"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/terminal/vt"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// Drawing.
//
// Each viewport row is drawn in three passes: the backgrounds that differ
// from the default (runs of equal color), the selection, then the text.
// Consecutive cells of one width with the same style and selection state
// form a segment, shaped as one string (blanks at either end trimmed)
// and cached by text and font style (segCache); its glyphs are then
// snapped to the cell grid, so a font whose advance is not a whole number
// of device pixels cannot drift from the cells. Wide characters and
// characters with combining marks are shaped on their own and centered in
// their cells. Inverse swaps the colors, Faint halves the foreground's
// alpha, Invisible draws no text, Underline and Strike are 1px lines at
// the bottom and the middle of the cells; Blink draws as normal text (M1).
// The cursor is a block (the character redrawn in CursorText on it) while
// the view has focus, a hollow box otherwise, and is not drawn while the
// program hides it; it does not blink (M1).
//
// Redraw hints (declareDamage): the view damages the rows TakeDirty
// reports, shifts its pixels with Box.ScrollShift by the rows TakeScroll
// reports, and damages the old and new cursor rows and the scrollbar
// strip when they change; anything else that changes (size, colors,
// selection, the exit message, a full redraw from the emulator) damages
// the whole view.

// segKey identifies a shaped segment.
type segKey struct {
	s            string
	bold, italic bool
	single       bool // a wide or combining character, centered
}

// segShape is a shaped segment: glyphs relative to the segment's first
// cell (snapped to the grid unless single) and the shaped width.
type segShape struct {
	glyphs []paint.Glyph
	width  float32
}

type segEntry struct {
	k segKey
	v *segShape
}

// segCache is an LRU cache of shaped segments.
type segCache struct {
	cap int
	ll  *list.List
	m   map[segKey]*list.Element
}

func newSegCache(n int) *segCache {
	return &segCache{cap: n, ll: list.New(), m: make(map[segKey]*list.Element, n)}
}

func (c *segCache) reset() { c.ll.Init(); clear(c.m) }

// get returns the shape of k, shaping it with make on a miss.
func (c *segCache) get(k segKey, make func() *segShape) *segShape {
	if e, ok := c.m[k]; ok {
		c.ll.MoveToFront(e)
		return e.Value.(*segEntry).v
	}
	v := make()
	c.m[k] = c.ll.PushFront(&segEntry{k, v})
	if c.ll.Len() > c.cap {
		old := c.ll.Back()
		c.ll.Remove(old)
		delete(c.m, old.Value.(*segEntry).k)
	}
	return v
}

// frame is the per-paint drawing context.
type frame struct {
	l      *paint.List
	r      geom.Rect
	m      metrics
	sch    *Scheme
	styles *vt.StyleTable
	// selFg is the text color of selected cells when the selection color
	// is opaque (zero alpha: keep the cells' colors).
	selFg theme.Color
	runes []rune
	one   []rune // a wide or combining character
}

// cursorKey is what the cursor drawing depends on.
type cursorKey struct {
	vy, x, n       int
	shown, focused bool
	comp           string
	compCur        int
}

// damageRec is what the last paint drew, for the redraw hints.
type damageRec struct {
	ok       bool
	r        geom.Rect
	m        metrics
	sch      Scheme
	cur      cursorKey
	selA     selPos
	selB     selPos
	selOK    bool
	bar      geom.Rect
	exited   bool
	viewRows int
}

// schemeFrom returns the scheme this frame: ViewW.Scheme or the token's.
func (w *ViewW) schemeFrom(tok *theme.TerminalToken) Scheme {
	if w.scheme != nil {
		return *w.scheme
	}
	return Scheme{Background: tok.Background, Foreground: tok.Foreground, Cursor: tok.Cursor,
		CursorText: tok.CursorText, Selection: tok.Selection, Palette: tok.Palette}
}

func (w *ViewW) paint(l *paint.List, r geom.Rect) {
	c, vs := w.c, w.vs
	tok := &c.ThemeFor(theme.CompTerminal).Components.Terminal
	sch := w.schemeFrom(tok)
	px := w.fontSize
	if px <= 0 {
		px = tok.ContentFontSize
	}
	if px <= 0 {
		px = defaultFontSize
	}
	scale := float32(1)
	if win := c.Window(); win != nil && win.Scale() > 0 {
		scale = win.Scale()
	}
	m := vs.metricsFor(px, scale)
	w.sizeTo(max(int(r.W/m.cellW), 2), max(int(r.H/m.lineH), 1), m)
	exited, _ := w.t.Exited()
	focused := w.ia.Focused && c.WindowFocused()

	if !w.done {
		w.readPaste() // the clipboard is read outside the emulator lock
	}
	v := w.t.Lock()
	s := v.Screen()
	n := v.TakeScroll()
	dirty, all := v.TakeDirty()
	dirty = append(vs.dirty[:0], dirty...) // TakeDirty reuses its slice
	vs.trackEviction(s)
	out := w.applyInput(v, r)
	vs.trackEviction(s) // input may have dropped lines (Cmd+K)
	// Input may have scrolled the viewport or written to the screen
	// (Cmd+K): take those changes too.
	if n2 := v.TakeScroll(); n2 > 0 {
		all = true
	}
	d2, all2 := v.TakeDirty()
	dirty = append(dirty, d2...)
	all = all || all2
	vs.dirty = dirty

	f := &frame{l: l, r: r, m: m, sch: &sch, styles: s.Styles()}
	if sch.Selection.A == 255 {
		f.selFg = sch.Foreground
	}
	l.Fill(r, geom.Radii{}, sch.Background)
	_, vrows := s.Size()
	selA, selB, selOK := vs.sel.bounds()
	for y := range vrows {
		line := s.ViewLine(y)
		top := r.Y + float32(y)*m.lineH
		lo, hi := 0, 0
		if selOK {
			lo, hi, _ = rowRange(line, w.stableRow(s, y), selA, selB)
		}
		w.paintRow(f, line, top, lo, hi)
	}
	cur := w.paintCursor(f, v, focused)
	if exited {
		w.paintExited(f, v)
	}
	bar := geom.Rect{}
	if thumb, ok := w.thumbRect(s, r); ok && (vs.hover || vs.bar.drag) {
		bar = thumb
		l.Fill(geom.Rect{X: thumb.X + 1, Y: thumb.Y, W: thumb.W - 2, H: thumb.H}, geom.Rad(3), tok.ScrollbarThumb)
	}
	if c.A11yEnabled() {
		vs.a11yText = screenText(s)
	}
	w.t.Unlock()

	rec := damageRec{ok: true, r: r, m: m, sch: sch, cur: cur, selA: selA, selB: selB, selOK: selOK,
		bar: bar, exited: exited, viewRows: vrows}
	w.declareDamage(rec, n, dirty, all)
	vs.dmg = rec
	vs.needFull = false

	w.runEffects(out)
}

// colors returns the foreground and background of style st.
func (f *frame) colors(st vt.Style) (fg, bg theme.Color) {
	if st.Attr&vt.AttrInverse != 0 {
		fg, bg = ResolveColor(st.Bg, f.sch, false), ResolveColor(st.Fg, f.sch, true)
	} else {
		fg, bg = ResolveColor(st.Fg, f.sch, true), ResolveColor(st.Bg, f.sch, false)
	}
	if st.Attr&vt.AttrFaint != 0 {
		fg.A /= 2
	}
	return fg, bg
}

// cellRect returns the rect of cells [x0, x1) of the row at top.
func (f *frame) cellRect(x0, x1 int, top float32) geom.Rect {
	cw := f.m.cellW
	return geom.Rect{X: f.r.X + float32(x0)*cw, Y: top, W: float32(x1-x0) * cw, H: f.m.lineH}
}

// paintRow draws one row; cells [selLo, selHi) are selected.
func (w *ViewW) paintRow(f *frame, line *vt.Line, top float32, selLo, selHi int) {
	l, cells := f.l, line.Cells
	// Backgrounds.
	start, col := -1, theme.Color{}
	flush := func(end int) {
		if start >= 0 {
			l.Fill(f.cellRect(start, end, top), geom.Radii{}, col)
			start = -1
		}
	}
	lastIdx, lastBg := -1, theme.Color{}
	for x, c := range cells {
		if int(c.Style) != lastIdx {
			lastIdx = int(c.Style)
			_, lastBg = f.colors(f.styles.Get(c.Style))
		}
		if lastBg == f.sch.Background || start >= 0 && lastBg != col {
			flush(x)
		}
		if lastBg != f.sch.Background && start < 0 {
			start, col = x, lastBg
		}
	}
	flush(len(cells))
	if selLo < selHi {
		l.Fill(f.cellRect(selLo, selHi, top), geom.Radii{}, f.sch.Selection)
	}

	// Text.
	seg := f.runes[:0]
	segStart, segStyle, segSel := 0, -1, false
	flushSeg := func() {
		if len(seg) > 0 {
			w.drawText(f, seg, segStart, len(seg), top, uint16(segStyle), segSel, false)
		}
		seg = seg[:0]
	}
	for x := 0; x < len(cells); {
		c := cells[x]
		if c.Width == 0 {
			x++ // the right half of a wide character
			continue
		}
		sel := x >= selLo && x < selHi && f.selFg.A != 0
		var marks []rune
		if c.Rune != 0 {
			marks = line.Combining(x)
		}
		if c.Width > 1 || len(marks) > 0 {
			flushSeg()
			n := max(int(c.Width), 1)
			if c.Rune != 0 {
				f.one = append(append(f.one[:0], c.Rune), marks...)
				w.drawText(f, f.one, x, n, top, c.Style, sel, true)
			}
			x += n
			continue
		}
		if int(c.Style) != segStyle || sel != segSel {
			flushSeg()
			segStart, segStyle, segSel = x, int(c.Style), sel
		}
		r := c.Rune
		if r == 0 {
			r = ' '
		}
		seg = append(seg, r)
		x++
	}
	flushSeg()
	f.runes = seg[:0]
}

// drawText draws runes rs over cells [x, x+n) of the row at top in style
// idx: a segment of one-cell characters, or one wide or combining
// character (single), centered in its cells.
func (w *ViewW) drawText(f *frame, rs []rune, x, n int, top float32, idx uint16, sel, single bool) {
	st := f.styles.Get(idx)
	fg, _ := f.colors(st)
	if sel {
		fg = f.selFg
	}
	w.drawRunes(f, rs, x, n, top, st.Attr, fg, single)
}

// drawRunes draws rs with attributes attr in color fg (see drawText).
func (w *ViewW) drawRunes(f *frame, rs []rune, x, n int, top float32, attr vt.Attr, fg theme.Color, single bool) {
	if attr&vt.AttrInvisible != 0 {
		return
	}
	l := f.l
	cell := f.cellRect(x, x+n, top)
	if attr&vt.AttrUnderline != 0 {
		l.Fill(geom.Rect{X: cell.X, Y: cell.Bottom() - 1, W: cell.W, H: 1}, geom.Radii{}, fg)
	}
	if attr&vt.AttrStrike != 0 {
		l.Fill(geom.Rect{X: cell.X, Y: top + float32(int(f.m.lineH/2)), W: cell.W, H: 1}, geom.Radii{}, fg)
	}
	lead, trail := 0, len(rs)
	if !single {
		for lead < trail && rs[lead] == ' ' {
			lead++
		}
		for trail > lead && rs[trail-1] == ' ' {
			trail--
		}
		if lead == trail {
			return
		}
	}
	k := segKey{s: string(rs[lead:trail]), bold: attr&vt.AttrBold != 0, italic: attr&vt.AttrItalic != 0, single: single}
	sh := w.vs.cache.get(k, func() *segShape { return shapeSeg(k, f.m) })
	if len(sh.glyphs) == 0 {
		return
	}
	org := geom.Pt(f.r.X+float32(x+lead)*f.m.cellW, top)
	if single {
		org.X += (cell.W - sh.width) / 2
	}
	l.Glyphs(org, sh.glyphs, fg)
}

// shapeSeg shapes k in the cell font of m.
func shapeSeg(k segKey, m metrics) *segShape {
	st := m.st
	if k.bold {
		st.Weight = text.Bold
	}
	st.Italic = k.italic
	lay := text.Shape(k.s, st, text.Options{NoWrap: true, Direction: text.DirLTR})
	g := append([]paint.Glyph(nil), lay.Glyphs...)
	if !k.single {
		// Snap each cluster to its cell: segments hold one-cell
		// characters only, so the rune index is the cell offset.
		cl, base := int32(-1), float32(0)
		for i := range g {
			c := int32(i)
			if i < len(lay.GlyphCluster) {
				c = lay.GlyphCluster[i]
			}
			if c != cl {
				cl, base = c, g[i].X
			}
			g[i].X = float32(c)*m.cellW + (g[i].X - base)
		}
	}
	return &segShape{glyphs: g, width: lay.Width}
}

// paintCursor draws the cursor or the IME composition and reports the
// caret rect to the host. v is locked.
func (w *ViewW) paintCursor(f *frame, v *vt.Terminal, focused bool) cursorKey {
	vs, s := w.vs, v.Screen()
	cols, rows := s.Size()
	cx, cy, shown := v.Cursor()
	vy := cy + s.ViewOffset()
	key := cursorKey{vy: vy, x: cx, n: 1, shown: shown, focused: focused, comp: vs.comp, compCur: vs.compCur}
	if vy >= rows {
		key.vy, key.shown = -1, false
	}
	top := f.r.Y + float32(vy)*f.m.lineH
	var cell vt.Cell
	if key.vy >= 0 {
		line := s.ViewLine(vy)
		cx = min(cx, cols-1)
		if cx > 0 && line.Cells[cx].Width == 0 {
			cx-- // on the right half: the cursor covers the wide character
		}
		cell = line.Cells[cx]
		key.x, key.n = cx, max(int(cell.Width), 1)
	}
	rect := f.cellRect(key.x, key.x+key.n, top)
	if w.ia.Focused {
		w.c.WantTextInput(rect)
	}
	if key.vy < 0 {
		return key
	}
	l := f.l
	if vs.comp != "" && focused {
		sh := shapeSeg(segKey{s: vs.comp, single: true}, f.m)
		bg := geom.Rect{X: rect.X, Y: top, W: max(sh.width, f.m.cellW), H: f.m.lineH}
		l.Fill(bg, geom.Radii{}, f.sch.Background)
		l.Glyphs(geom.Pt(rect.X, top), sh.glyphs, f.sch.Foreground)
		l.Fill(geom.Rect{X: bg.X, Y: bg.Bottom() - 1, W: bg.W, H: 1}, geom.Radii{}, f.sch.Foreground)
		return key
	}
	if !shown {
		return key
	}
	if !focused {
		l.Stroke(geom.Rect{X: rect.X + 0.5, Y: rect.Y + 0.5, W: rect.W - 1, H: rect.H - 1}, geom.Radii{}, 1, f.sch.Cursor)
		return key
	}
	l.Fill(rect, geom.Radii{}, f.sch.Cursor)
	if cell.Rune != 0 {
		st := f.styles.Get(cell.Style)
		rs := append([]rune{cell.Rune}, s.ViewLine(vy).Combining(cx)...)
		w.drawRunes(f, rs, cx, key.n, top, st.Attr&^(vt.AttrUnderline|vt.AttrStrike|vt.AttrInvisible), f.sch.CursorText, true)
	}
	return key
}

// paintExited draws exitedText on the row below the output (the last
// non-blank row or the cursor's, whichever is lower), or over the last
// row when the output fills the view. v is locked.
func (w *ViewW) paintExited(f *frame, v *vt.Terminal) {
	s := v.Screen()
	_, rows := s.Size()
	_, cy, _ := v.Cursor()
	last := cy
	for y := rows - 1; y > last; y-- {
		if !blankLine(s.Line(y)) {
			last = y
			break
		}
	}
	vy := last + 1 + s.ViewOffset()
	if vy >= rows {
		if s.ViewOffset() > 0 {
			return // below the viewport
		}
		vy = rows - 1
	}
	top := f.r.Y + float32(vy)*f.m.lineH
	f.l.Fill(geom.Rect{X: f.r.X, Y: top, W: f.r.W, H: f.m.lineH}, geom.Radii{}, f.sch.Background)
	sh := w.vs.cache.get(segKey{s: exitedText}, func() *segShape { return shapeSeg(segKey{s: exitedText}, f.m) })
	f.l.Glyphs(geom.Pt(f.r.X, top), sh.glyphs, f.sch.Foreground)
}

func blankLine(l *vt.Line) bool {
	for _, c := range l.Cells {
		if c.Rune != 0 && c.Rune != ' ' {
			return false
		}
	}
	return true
}

// screenText returns the viewport's text for the semantics tree, one line
// per row, trailing blanks dropped. s is locked.
func screenText(s *vt.Screen) string {
	_, rows := s.Size()
	var sb strings.Builder
	for y := range rows {
		var line strings.Builder
		l := s.ViewLine(y)
		for x, c := range l.Cells {
			switch {
			case c.Width == 0:
			case c.Rune == 0:
				line.WriteByte(' ')
			default:
				line.WriteRune(c.Rune)
				for _, m := range l.Combining(x) {
					line.WriteRune(m)
				}
			}
		}
		if y > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(strings.TrimRight(line.String(), " "))
	}
	return strings.TrimRight(sb.String(), "\n")
}

// declareDamage declares what changed since the last paint: n rows
// scrolled, the dirty rows, or all.
func (w *ViewW) declareDamage(cur damageRec, n int, dirty []int, all bool) {
	c, vs := w.c, w.vs
	prev := vs.dmg
	r, lh := cur.r, cur.m.lineH
	if all || vs.needFull || !prev.ok || prev.r != cur.r || prev.m != cur.m || prev.sch != cur.sch ||
		prev.exited != cur.exited || prev.viewRows != cur.viewRows ||
		prev.selOK != cur.selOK || cur.selOK && (prev.selA != cur.selA || prev.selB != cur.selB) {
		c.Damage(r)
		return
	}
	row := func(y int) {
		if y >= 0 && y < cur.viewRows {
			c.Damage(geom.Rect{X: r.X, Y: r.Y + float32(y)*lh, W: r.W, H: lh})
		}
	}
	if n > 0 {
		grid := geom.Rect{X: r.X, Y: r.Y, W: r.W, H: float32(cur.viewRows) * lh}
		w.box.ScrollShift(grid, geom.Pt(0, -float32(n)*lh))
	}
	for _, y := range dirty {
		row(y)
	}
	if n > 0 || prev.cur != cur.cur {
		if prev.cur.vy >= 0 {
			row(prev.cur.vy - n) // where the scroll carried the old cursor
		}
		row(cur.cur.vy)
	}
	if prev.bar != cur.bar || n > 0 && !cur.bar.Empty() {
		c.Damage(geom.Rect{X: r.Right() - scrollbarWidth, Y: r.Y, W: scrollbarWidth, H: r.H})
	}
}
