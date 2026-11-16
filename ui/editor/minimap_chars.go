package editor

import (
	"image"
	"image/color"
	"math"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/editor/monarch"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// The minimap's characters (Monaco's minimap.renderCharacters: true,
// RenderMinimap.Text; minimap.js _renderLine, MinimapCharRenderer):
//
// Each character is a cell of Monaco's prebaked glyph alphas
// (minimap_chars_gen.go): 1×2 pixels on a 1x display, 2×4 on a 2x one
// (Monaco's minimap scale 1, doubled at pixel ratio ≥ 2), one cell per
// display column (two for a wide character, as Monaco renders a full
// width character twice), spaces and tabs left empty. Characters outside
// U+0020–U+007E take the glyph Monaco's getCharIndex gives them at these
// scales (an arbitrary ASCII shape: the first UTF-16 code unit, minus 32,
// modulo 96; U+0080, which Monaco reads past its table for, draws
// nothing). Cells follow graphemes and display columns (as the blocks and
// the editor's column model), where Monaco steps per UTF-16 code unit: a
// combining sequence is one cell here, an astral character one (two when
// wide) cell of its high surrogate's glyph. The alphas are softened as
// Monaco's charDataNormal (×12/15)
// or, over a light editor background (relative luminance ≥ 0.5),
// charDataLight (×50/60), and scaled by the alpha of
// minimap.foregroundOpacity. A pixel is the token's foreground color at
// that alpha over the minimap background: Monaco's bg + (fg − bg)·α.
//
// A line's bitmap is cached with its miniature line (minimap.go: rebuilt
// only when the line changes). The bitmaps are composed into strips of
// minimapStripRows rows, each drawn as one image (paint.List.DrawImage):
// a strip is composed again, as a new image, only when one of its rows
// that is drawn changes, so an edit uploads one strip and scrolling the
// miniature uploads the strips that enter it (and the edge strips as
// their rows come into view), never the whole minimap. A strip is placed
// on the device pixel grid so the cells are not resampled when the rows
// are 2 logical pixels tall at an integer scale.

// minimapStripRows is the number of miniature rows in a strip image.
const minimapStripRows = 16

// miniGlyphs are the glyph cells at one scale, font and opacity.
type miniGlyphs struct {
	cw    int    // cell width in pixels; the cell is cw×2cw
	alpha []byte // 96 cells of cw·2cw alphas
	light bool
	fgA   uint8
}

// minimapGlyphsFor returns the cells of scale cw (1 or 2), softened for a
// light or dark background, at foreground opacity fgA.
func minimapGlyphsFor(cw int, light bool, fgA uint8) *miniGlyphs {
	raw := minimapGlyphs1x[:]
	if cw >= 2 {
		cw, raw = 2, minimapGlyphs2x[:]
	}
	ratio := 12.0 / 15.0
	if light {
		ratio = 50.0 / 60.0
	}
	g := &miniGlyphs{cw: cw, alpha: make([]byte, len(raw)), light: light, fgA: fgA}
	for i, v := range raw {
		soft := int(float64(v) * ratio) // Monaco's toUint8 truncates
		g.alpha[i] = byte((soft*int(fgA) + 127) / 255)
	}
	return g
}

// miniBitmap is a line of the miniature: w×h non-premultiplied RGBA.
type miniBitmap struct {
	w, h int
	pix  []byte
}

// minimapGlyphIndex is Monaco's getCharIndex at font scales ≤ 2 for the
// grapheme g (its first UTF-16 code unit); -1 for none.
func minimapGlyphIndex(g string) int {
	r, n := utf8.DecodeRuneInString(g)
	u := int(r)
	if r == utf8.RuneError && n <= 1 {
		u = 0xfffd
	} else if r >= 0x10000 {
		u = 0xd800 + (int(r)-0x10000)>>10
	}
	i := u - 32
	if i < 0 || i > 96 {
		i = (i + 96) % 96
	}
	if i == 96 {
		return -1
	}
	return i
}

// miniCharsAt draws the characters of s into a new bitmap (nil when s
// has none to draw); colorAt gives the color of the character at a byte
// of s (called with increasing bytes).
func miniCharsAt(s string, tab int, g *miniGlyphs, colorAt func(i int) color.NRGBA) *miniBitmap {
	type cell struct {
		col, n, glyph int
		c             color.NRGBA
	}
	// Common minimap rows are at most 120 columns. Keep their transient
	// glyph cells on the stack; unusually wide previews can still grow.
	var local [128]cell
	cells := local[:0]
	col, end := 0, 0
	for i := 0; i < len(s); {
		j := nextGrapheme(s, i)
		next := advanceCol(col, s[i:j], tab)
		if gr := s[i:j]; gr != " " && gr != "\t" {
			if gi := minimapGlyphIndex(gr); gi >= 0 && next > col {
				cells = append(cells, cell{col, next - col, gi, colorAt(i)})
				end = next
			}
		}
		col = next
		i = j
	}
	if end == 0 {
		return nil
	}
	cw := g.cw
	b := &miniBitmap{w: end * cw, h: 2 * cw}
	b.pix = make([]byte, 4*b.w*b.h)
	size := cw * 2 * cw
	for _, c := range cells {
		src := g.alpha[c.glyph*size : (c.glyph+1)*size]
		for k := range c.n {
			x0 := (c.col + k) * cw
			for y := range 2 * cw {
				for x := range cw {
					p := b.pix[4*(y*b.w+x0+x):]
					p[0], p[1], p[2], p[3] = c.c.R, c.c.G, c.c.B, src[y*cw+x]
				}
			}
		}
	}
	return b
}

// miniLineBitmap draws the miniature line s (at byte base of its line)
// in its tokens' foreground colors.
func miniLineBitmap(s string, base int, toks []monarch.Token, th *theme.Theme, tab int, g *miniGlyphs) *miniBitmap {
	k := 0
	fg := th.Style("").Foreground
	return miniCharsAt(s, tab, g, func(i int) color.NRGBA {
		for k < len(toks) && toks[k].ByteStart <= base+i {
			fg = th.Style(toks[k].Type).Foreground
			k++
		}
		return fg
	})
}

// relativeLuminance is Monaco's Color.getRelativeLuminance.
func relativeLuminance(c color.NRGBA) float64 {
	ch := func(v uint8) float64 {
		f := float64(v) / 255
		if f <= 0.03928 {
			return f / 12.92
		}
		return math.Pow((f+0.055)/1.055, 2.4)
	}
	return 0.2126*ch(c.R) + 0.7152*ch(c.G) + 0.0722*ch(c.B)
}

// miniStrip is one strip image of the character minimap.
type miniStrip struct {
	rows [minimapStripRows]*miniLine // the lines composed (nil: blank)
	img  *image.RGBA                 // premultiplied, ready for either renderer
	gen  uint64                      // composition number (damage key)
	tall float32                     // row pitch in device pixels when rows exceed a glyph (0: packed)
	used bool                        // drawn this frame
}

type miniStripWant struct {
	id   int
	rows [minimapStripRows]*miniLine
	vis  [minimapStripRows]bool
}

// miniGlyphsFor returns the cells the view's minimap draws with now,
// dropping the cached bitmaps and strips when they change.
func (w *ViewW) miniGlyphsFor(th *theme.Theme) *miniGlyphs {
	m := &w.vs.mini
	cw := 1
	if w.c.Window().Scale() >= 2 {
		cw = 2
	}
	light := relativeLuminance(defaultColor(th, "editor.background")) >= 0.5
	fgA := uint8(255)
	if c, ok := lookupColor(th, "minimap.foregroundOpacity"); ok {
		fgA = c.A
	}
	if g := m.glyphs; g != nil && g.cw == cw && g.light == light && g.fgA == fgA {
		return g
	}
	m.glyphs = minimapGlyphsFor(cw, light, fgA)
	for _, ln := range m.cache {
		ln.bmp, ln.bmpDone = nil, false
	}
	m.strips = nil
	return m.glyphs
}

// ensureMiniBitmap draws ln's bitmap when characters are on and it has
// none yet.
// prepareMinimap has called miniGlyphsFor this frame.
func (w *ViewW) ensureMiniBitmap(ln *miniLine, th *theme.Theme, tab int) {
	g := w.vs.mini.glyphs
	if !w.opts.Minimap.RenderCharacters || ln == nil || ln.bmpDone || g == nil {
		return
	}
	ln.bmp = miniLineBitmap(ln.text, ln.base, ln.tokens, th, tab, g)
	ln.bmpDone = true
	w.vs.mini.charRendered++
}

// paintMinimapChars draws the miniature rows as strip images (see above).
func (w *ViewW) paintMinimapChars(l *paint.List, r geom.Rect) {
	vs := w.vs
	m := &vs.mini
	if m.glyphs == nil {
		return
	}
	cw := m.glyphs.cw
	unit, offset, _ := w.miniCanvas()
	sampling := unit < 1
	if sampling != m.stripSampling {
		m.strips, m.stripSampling = nil, sampling
	}
	if m.strips == nil {
		m.strips = map[int]*miniStrip{}
	}
	for _, s := range m.strips {
		s.used = false
	}
	// The rows of each strip drawn this frame, in order.
	clear(m.stripWants)
	wants := m.stripWants[:0]
	prev := -1
	for k, row := range vs.miniRows {
		slot := row.vrow
		if sampling {
			slot = k
		}
		if slot == prev || slot < 0 {
			continue
		}
		prev = slot
		id, j := slot/minimapStripRows, slot%minimapStripRows
		if n := len(wants); n == 0 || wants[n-1].id != id {
			wants = append(wants, miniStripWant{id: id})
		}
		wt := &wants[len(wants)-1]
		wt.rows[j], wt.vis[j] = m.cache[row.key], true
	}
	m.stripWants = wants
	scale := max(0.01, w.c.Window().Scale())
	colScale := min(1, r.W/float32(w.miniColumns()))
	for i := range wants {
		wt := &wants[i]
		s := m.strips[wt.id]
		if s == nil {
			s = &miniStrip{}
			m.strips[wt.id] = s
		}
		s.used = true
		dirty := s.gen == 0
		for j, v := range wt.vis {
			if v && s.rows[j] != wt.rows[j] {
				dirty = true
			}
		}
		// Rows taller than a glyph (MinimapFill on a short document) keep
		// the glyph height: each row's glyphs sit at its own device row,
		// centered in the row as Monaco's innerLinePadding.
		tall := float32(0)
		if !sampling && unit > 2 {
			tall = unit * scale
		}
		if s.tall != tall {
			dirty = true
		}
		if dirty {
			// Rows not drawn this frame lie outside the minimap (clipped):
			// they are left out and composed when they come into view.
			s.rows, s.tall = wt.rows, tall
			s.img = composeMiniStrip(&s.rows, cw, tall)
			m.stripGen++
			s.gen = m.stripGen
			m.stripsComposed++
		}
		if s.img == nil {
			continue
		}
		y0, pitch := r.Y+(float32(wt.id*minimapStripRows)-offset)*unit, unit
		if sampling {
			y0, pitch = r.Y+float32(wt.id*minimapStripRows), 1
		}
		y0 = float32(math.Round(float64(y0*scale))) / scale
		dst := geom.Rect{X: r.X, Y: y0, W: float32(s.img.Rect.Dx()/cw) * colScale, H: minimapStripRows * pitch}
		if tall > 0 {
			dst.H = float32(s.img.Rect.Dy()) / scale // device pixel for pixel
		}
		l.DrawImage(s.img, dst, geom.Radii{})
		if fr := dst.Intersect(r); !fr.Empty() {
			vs.navPaint = append(vs.navPaint, fixedPaint{fr, s.gen | 1<<63})
		}
	}
	for id, s := range m.strips {
		if !s.used {
			delete(m.strips, id)
		}
	}
	// Do not retain rows which the minimap cache drops next frame.
	clear(m.stripWants)
}

// composeMiniStrip copies the rows' bitmaps into a new strip image (nil
// when all are blank). With tall 0 the rows are packed one glyph height
// apart (the image is drawn scaled to the row pitch); otherwise tall is
// the row pitch in device pixels and row j's glyphs are at round(j·tall)
// plus Monaco's innerLinePadding, floor((round(tall) − glyph height) / 2),
// the image ending below the last row drawn (it is drawn one image pixel
// per device pixel).
func composeMiniStrip(rows *[minimapStripRows]*miniLine, cw int, tall float32) *image.RGBA {
	h := 2 * cw
	top := func(j int) int { return j * h }
	if tall > 0 {
		pad := max(0, (int(math.Round(float64(tall)))-h)/2)
		top = func(j int) int { return int(math.Round(float64(float32(j)*tall))) + pad }
	}
	w, bottom := 0, 0
	for j, ln := range rows {
		if ln != nil && ln.bmp != nil {
			w = max(w, ln.bmp.w)
			bottom = top(j) + h
		}
	}
	if w == 0 {
		return nil
	}
	if tall == 0 {
		bottom = minimapStripRows * h
	}
	img := image.NewRGBA(image.Rect(0, 0, w, bottom))
	for j, ln := range rows {
		if ln == nil || ln.bmp == nil {
			continue
		}
		b := ln.bmp
		for y := range min(h, b.h) {
			dst := img.Pix[(top(j)+y)*img.Stride:][:4*b.w]
			src := b.pix[y*4*b.w : (y+1)*4*b.w]
			for x := 0; x < len(src); x += 4 {
				s, d := src[x:x+4:x+4], dst[x:x+4:x+4]
				// Match image/draw's NRGBA -> RGBA rounding exactly. The
				// completed strip is immutable and needs no renderer copy.
				a := uint32(s[3]) * 0x101
				d[0] = uint8(uint32(s[0]) * a / 0xff >> 8)
				d[1] = uint8(uint32(s[1]) * a / 0xff >> 8)
				d[2] = uint8(uint32(s[2]) * a / 0xff >> 8)
				d[3] = s[3]
			}
		}
	}
	return img
}
