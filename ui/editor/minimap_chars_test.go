package editor

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tangoui/tango/internal/render/gpu/shaders"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/paint"
)

var updateMinimapBlocks = flag.Bool("update-minimap-blocks", false, "rewrite testdata/minimap/blocks_ops.txt")

func TestMinimapStripsUseSingleTexel(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			st := NewState(strings.Repeat("func f() { return 1 }\n", 300), "a.go", theme.Base("vs-dark"))
			t.Cleanup(st.Close)
			h := newHintHost(t, st, geom.Size{W: 640, H: 300}, scale)
			h.frame()
			var strips, aligned int
			for _, b := range h.dev.Last().Batches {
				for _, in := range b.Instances {
					if in.Kind.X == shaders.KindImage {
						strips++
						if in.Params.Z == 1 {
							aligned++
						}
					}
				}
			}
			if strips < 2 || aligned != strips {
				t.Fatalf("single-texel strips %d of %d, want all of at least two strips", aligned, strips)
			}
			t.Logf("single-texel strips %d of %d", aligned, strips)
		})
	}
}

// minimapOps returns the minimap's paint ops of the view's last frame,
// one per line, by painting the minimap again into a fresh list.
func minimapOps(w *ViewW) string {
	var l paint.List
	saved := w.vs.navPaint
	w.vs.navPaint = nil
	w.paintMinimap(&l, w.st.Theme)
	w.vs.navPaint = saved
	var sb strings.Builder
	for _, op := range l.Ops {
		fmt.Fprintf(&sb, "%T %v\n", op, op)
	}
	return sb.String()
}

// TestMinimapBlocksWhenOff: with RenderCharacters off the minimap draws
// exactly the token-colored blocks it drew before characters existed
// (testdata/minimap/blocks_ops.txt was recorded with the blocks-only
// minimap).
func TestMinimapBlocksWhenOff(t *testing.T) {
	goText(t)
	var all strings.Builder
	for _, c := range []struct {
		name  string
		text  string
		file  string
		th    string
		size  MinimapSize
		lines int
	}{
		{"go-proportional", strings.Repeat("package main\n\nfunc f(x int) string {\n\treturn \"héllo 世界\" // c\n}\n", 60), "a.go", "vs-dark", MinimapProportional, 120},
		{"txt-fit", manyLines(2000), "a.txt", "vs", MinimapFit, 900},
	} {
		st := NewState(c.text, c.file, theme.Base(c.th))
		st.Options.Minimap.RenderCharacters = false
		st.Options.Minimap.Size = c.size
		st.Options.Minimap.ShowSlider = MinimapSliderAlways
		d, vw := viewDriver(st, 640, 300, nil)
		for range 6 {
			d.Frame()
		}
		st.ScrollBy(float32(c.lines)*st.Viewport.LineHeight, 0)
		for range 4 {
			d.Frame()
		}
		fmt.Fprintf(&all, "== %s\n%s", c.name, minimapOps(*vw))
		st.Close()
	}
	path := filepath.Join("testdata", "minimap", "blocks_ops.txt")
	if *updateMinimapBlocks {
		if err := os.WriteFile(path, []byte(all.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := all.String(); got != string(want) {
		gl, wl := strings.Split(got, "\n"), strings.Split(string(want), "\n")
		for i := range min(len(gl), len(wl)) {
			if gl[i] != wl[i] {
				t.Fatalf("blocks differ at line %d:\n got %s\nwant %s", i+1, gl[i], wl[i])
			}
		}
		t.Fatalf("blocks differ: %d lines, want %d", len(gl), len(wl))
	}
}

// TestMinimapCharacters: with RenderCharacters on a line is an alpha
// bitmap of Monaco's prebaked glyphs in its tokens' foreground colors
// (testdata/minimap/render.json: lines Monaco's own renderChar drew).
func TestMinimapCharacters(t *testing.T) {
	// Glyph shapes: "a" and "." cover differently, as the prebaked data.
	for _, cw := range []int{1, 2} {
		g := minimapGlyphsFor(cw, false, 255)
		fg := color.NRGBA{86, 156, 214, 255}
		bmp := miniCharsAt("a.", 4, g, func(int) color.NRGBA { return fg })
		if bmp == nil || bmp.w != 2*cw || bmp.h != 2*cw {
			t.Fatalf("%dx: bitmap %+v", cw, bmp)
		}
		raw := minimapGlyphs1x[:]
		if cw == 2 {
			raw = minimapGlyphs2x[:]
		}
		cell := cw * 2 * cw
		sum := func(col int) (n int, full bool) {
			full = true
			for y := range 2 * cw {
				for x := range cw {
					p := bmp.pix[(y*bmp.w+col*cw+x)*4:]
					if p[0] != fg.R || p[1] != fg.G || p[2] != fg.B {
						t.Fatalf("%dx: pixel color %v, want token color %v", cw, p[:3], fg)
					}
					want := byte(int(raw[(int('a'-32)+col*('.'-'a'))*cell+y*cw+x]) * 12 / 15)
					if p[3] != want {
						t.Fatalf("%dx col %d (%d,%d): alpha %d, want prebaked %d", cw, col, x, y, p[3], want)
					}
					n += int(p[3])
					full = full && p[3] == 255
				}
			}
			return
		}
		a, af := sum(0)
		dot, df := sum(1)
		if a == dot || af || df || a == 0 || dot == 0 {
			t.Fatalf("%dx: coverage a %d, . %d (full %v %v)", cw, a, dot, af, df)
		}
	}

	// Monaco parity: composited over the background, the bitmap gives
	// renderChar's pixels (±1 for the 8-bit alpha).
	data, err := os.ReadFile(filepath.Join("testdata", "minimap", "render.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Scale, Width, Height, BgAlpha, FgAlpha int
		Text                                   string
		Color, Bg                              [3]uint8
		Light                                  bool
		RGBA                                   []int
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		g := minimapGlyphsFor(c.Scale, c.Light, uint8(c.FgAlpha))
		fg := color.NRGBA{c.Color[0], c.Color[1], c.Color[2], 255}
		bmp := miniCharsAt(c.Text, 4, g, func(int) color.NRGBA { return fg })
		for y := range c.Height {
			for x := range c.Width {
				var a int
				if bmp != nil && x < bmp.w {
					a = int(bmp.pix[(y*bmp.w+x)*4+3])
				}
				for k := range 3 {
					got := (int(c.Color[k])*a + int(c.Bg[k])*(255-a) + 127) / 255
					want := c.RGBA[(y*c.Width+x)*4+k]
					if d := got - want; d < -1 || d > 1 {
						t.Fatalf("%dx %q light %v (%d,%d)[%d]: %d, Monaco %d", c.Scale, c.Text, c.Light, x, y, k, got, want)
					}
				}
			}
		}
	}

	// In the View: the cached line bitmaps carry the token colors, and the
	// minimap is drawn as images.
	goText(t)
	th := theme.Base("vs-dark")
	st := NewState("package main\nvar x = \"hello\"\n", "a.go", th)
	defer st.Close()
	d, vw := viewDriver(st, 640, 300, nil)
	for range 4 {
		d.Frame()
	}
	w := *vw
	if !w.opts.Minimap.RenderCharacters {
		t.Fatal("RenderCharacters off by default")
	}
	ln := w.vs.mini.cache[0]
	if ln == nil || ln.bmp == nil {
		t.Fatal("no line bitmap")
	}
	kw := th.Style("keyword.go").Foreground
	toks, _ := st.Highlighter().TokensFor(st.hlKey, st.Snapshot(), 0, nil)
	if len(toks) > 0 {
		kw = th.Style(toks[0].Type).Foreground
	}
	if p := ln.bmp.pix; p[0] != kw.R || p[1] != kw.G || p[2] != kw.B {
		t.Fatalf("first pixel color %v, want keyword color %v", p[:3], kw)
	}
	ops := minimapOps(w)
	if !strings.Contains(ops, "paint.Image") {
		t.Fatalf("minimap drawn without images:\n%s", ops)
	}
	if strings.Count(ops, "paint.FillRect") > 2 { // the background (and slider)
		t.Fatalf("character minimap still draws blocks:\n%s", ops)
	}
	img := d.Frame()
	r := w.vs.minimapRect
	bg := defaultColor(th, "editor.background")
	if countColor(img, r, bg) == int(r.W)*int(r.H) {
		t.Fatal("no minimap pixels drawn")
	}
}

// minimapImages returns the images the minimap of the view's last frame
// draws.
func minimapImages(w *ViewW) map[image.Image]bool {
	var l paint.List
	saved := w.vs.navPaint
	w.vs.navPaint = nil
	w.paintMinimap(&l, w.st.Theme)
	w.vs.navPaint = saved
	out := map[image.Image]bool{}
	for _, op := range l.Ops {
		if o, ok := op.(paint.Image); ok {
			out[o.Img] = true
		}
	}
	return out
}

// TestMinimapCharactersIncremental: an edit redraws the bitmap of the
// edited line only and replaces one image; an unchanged frame redraws
// nothing; a theme change redraws.
func TestMinimapCharactersIncremental(t *testing.T) {
	goText(t)
	st := NewState(manyLines(80), "a.txt", theme.Base("vs"))
	defer st.Close()
	d, vw := viewDriver(st, 640, 300, nil)
	for range 8 {
		d.Frame()
	}
	m := &(*vw).vs.mini
	before, strips := m.charRendered, m.stripsComposed
	imgs := minimapImages(*vw)
	if len(imgs) < 2 {
		t.Fatalf("%d minimap images", len(imgs))
	}
	off := st.Snapshot().LineStart(3)
	if err := st.Edit([]buffer.Edit{{Start: off, End: off + 1, Text: "X"}}, Selection{}); err != nil {
		t.Fatal(err)
	}
	d.Frame()
	if got := m.charRendered - before; got != 1 {
		t.Fatalf("redrew %d line bitmaps, want 1", got)
	}
	if got := m.stripsComposed - strips; got != 1 {
		t.Fatalf("composed %d strips, want 1", got)
	}
	now := minimapImages(*vw)
	fresh := 0
	for img := range now {
		if !imgs[img] {
			fresh++
		}
	}
	if fresh != 1 || len(now) != len(imgs) {
		t.Fatalf("%d new images of %d (were %d), want 1", fresh, len(now), len(imgs))
	}
	before, strips = m.charRendered, m.stripsComposed
	d.Frame()
	if m.charRendered != before || m.stripsComposed != strips {
		t.Fatal("unchanged frame redrew")
	}
	st.Theme = theme.Base("vs-dark")
	d.Frame()
	if m.charRendered == before {
		t.Fatal("theme failed to invalidate")
	}
	// Turning characters off and on again.
	st.Options.Minimap.RenderCharacters = false
	d.Frame()
	if len(minimapImages(*vw)) != 0 {
		t.Fatal("blocks drawn as images")
	}
	st.Options.Minimap.RenderCharacters = true
	d.Frame()
	if len(minimapImages(*vw)) == 0 {
		t.Fatal("characters not back")
	}
}

// damageMinimapChars runs the character minimap through the verifier:
// typing (one strip changes), Enter (rows move), scrolling the miniature
// (strips move and enter), the slider, a click in the minimap, and
// switching to blocks and back (full frames: an option changed).
func damageMinimapChars(t *testing.T) {
	for _, scale := range []float32{2, 1.25} {
		st := NewState(hintDoc(400), "main.go", theme.Base("vs-dark"))
		st.Options.Minimap.ShowSlider = MinimapSliderAlways
		h := newHintHost(t, st, geom.Size{W: 640, H: 360}, scale)
		h.focusAt(6)
		for i := range 10 {
			h.typeText(string(rune('a' + i)))
			h.step("type")
		}
		h.key(input.KeyEnter, 0)
		h.step("enter")
		h.key(input.KeyBackspace, 0)
		h.step("backspace")
		if len(minimapImages(h.vw)) == 0 {
			t.Fatal("no minimap images")
		}
		p := geom.Pt(200, 180)
		for _, dy := range []float32{8, 16, 4, 2.4, 40, -8, 120, -16} {
			h.r.HandleEvent(input.Scroll{Pos: p, DY: dy})
			h.step(fmt.Sprintf("scroll %v", dy))
		}
		r := h.vw.vs.minimapRect
		h.click(geom.Pt(r.X+4, r.Y+r.H-20), 0)
		h.step("minimap click")
		st.Options.Minimap.RenderCharacters = false
		h.step("blocks")
		st.Options.Minimap.RenderCharacters = true
		h.step("characters")
		h.verified()
		st.Close()
	}
}

// TestMinimapCharactersFillShortDoc: with MinimapFill a short document's
// rows are much taller than a glyph; the glyphs keep their height (2
// logical pixels, drawn pixel for pixel) and sit centered in their rows
// (Monaco's innerLinePadding), not stretched to the row.
func TestMinimapCharactersFillShortDoc(t *testing.T) {
	goText(t)
	for _, scale := range []float32{1, 2} {
		st := NewState(strings.Repeat("func f() { return 1 }\n", 9)+"x", "a.go", theme.Base("vs-dark"))
		st.Options.Minimap.Size = MinimapFill
		var vw *ViewW
		d := ui.NewDriver(func(c *ui.Context) { vw = View(c, st) }, ui.DriverOptions{Width: 640, Height: 300, Scale: scale})
		for range 4 {
			d.Frame()
		}
		unit, _, _ := vw.miniCanvas()
		if unit < 20 {
			t.Fatalf("unit %v: not a tall-row case", unit)
		}
		var l paint.List
		vw.paintMinimapChars(&l, vw.vs.minimapRect)
		n := 0
		cw := vw.vs.mini.glyphs.cw
		for _, op := range l.Ops {
			o, ok := op.(paint.Image)
			if !ok {
				continue
			}
			n++
			img := o.Img.(*image.RGBA)
			if got := o.Rect.H * scale; got != float32(img.Rect.Dy()) {
				t.Fatalf("scale %v: image %d px tall drawn %v device px (stretched)", scale, img.Rect.Dy(), got)
			}
			// Runs of rows with glyph pixels: one glyph height each, one
			// per text row, unit apart, centered.
			var starts []int
			run := 0
			for y := 0; y <= img.Rect.Dy(); y++ {
				ink := false
				for x := 0; y < img.Rect.Dy() && x < img.Rect.Dx(); x++ {
					if img.RGBAAt(x, y).A != 0 {
						ink = true
						break
					}
				}
				if ink {
					if run == 0 {
						starts = append(starts, y)
					}
					run++
				} else if run > 0 {
					if run > 2*cw {
						t.Fatalf("scale %v: glyph run %d px tall, want ≤ %d", scale, run, 2*cw)
					}
					run = 0
				}
			}
			if len(starts) != 10 {
				t.Fatalf("scale %v: %d glyph rows, want 10", scale, len(starts))
			}
			pitch := float64(unit * scale)
			pad := (int(math.Round(pitch)) - 2*cw) / 2
			for j, y := range starts {
				if want := int(math.Round(float64(j)*pitch)) + pad; y != want {
					t.Fatalf("scale %v: row %d glyphs at %d, want %d", scale, j, y, want)
				}
			}
		}
		if n != 1 {
			t.Fatalf("scale %v: %d strip images, want 1", scale, n)
		}
		st.Close()
	}
}

// TestMinimapRowsCoverEveryViewRow: with rows a pixel or taller (every
// Size, any unit) each view row in the miniature's window is sampled
// exactly once and in order, and clicking a row's area scrolls to it
// (float32 sampling skipped rows at units other than 2: 97 lines in a
// 700 px Fill minimap at 1.5x lost rows 25, 41, 50, 59 and 82).
func TestMinimapRowsCoverEveryViewRow(t *testing.T) {
	goText(t)
	for _, c := range []struct {
		lines int
		h     float32
		scale float32
		size  MinimapSize
	}{
		{97, 700, 1.5, MinimapFill},
		{97, 700, 1, MinimapFill},
		{10, 300, 2, MinimapFill},
		{230, 300, 1.25, MinimapFit},
		{140, 300, 1, MinimapFit},
		{1000, 300, 2, MinimapProportional},
		{333, 517, 1.5, MinimapFill},
	} {
		name := fmt.Sprintf("%d lines h%v @%vx %s", c.lines, c.h, c.scale, c.size)
		st := NewState(manyLines(c.lines-1)+"last", "a.txt", theme.Base("vs"))
		st.Options.Minimap.Size = c.size
		var vw *ViewW
		d := ui.NewDriver(func(cx *ui.Context) { vw = View(cx, st) }, ui.DriverOptions{Width: 640, Height: c.h, Scale: c.scale})
		d.Frame()
		unit, offset, _ := vw.miniCanvas()
		if unit < 1 {
			t.Fatalf("%s: unit %v < 1", name, unit)
		}
		rows := vw.vs.miniRows
		first := int(math.Floor(float64(offset)))
		for i, row := range rows {
			if row.vrow != first+i {
				t.Fatalf("%s (unit %v): miniRows[%d].vrow = %d, want %d", name, unit, i, row.vrow, first+i)
			}
		}
		n := st.vl.Count()
		if last := rows[len(rows)-1].vrow; last != n-1 && rows[len(rows)-1].y+unit < vw.vs.minimapRect.Bottom() {
			t.Fatalf("%s: last sampled row %d of %d with room below", name, last, n)
		}
		if c.size != MinimapProportional {
			for _, k := range []int{0, len(rows) / 3, len(rows) / 2, len(rows) - 1} {
				st.Viewport.setScrollY(0, n)
				d.Frame()
				row := vw.vs.miniRows[k]
				want := st.Viewport
				want.setScrollY(float64(row.vrow)*float64(want.LineHeight), n)
				vw.navigateMinimapClick(geom.Pt(vw.vs.minimapRect.X+4, row.y+row.h/2))
				if got := st.Viewport.TopLine; got != want.TopLine {
					t.Fatalf("%s: click on row %d scrolled to %d, want %d", name, row.vrow, got, want.TopLine)
				}
			}
		}
		st.Close()
	}
}
