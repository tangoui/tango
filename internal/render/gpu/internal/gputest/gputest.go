// Package gputest holds the test scenes and image comparison shared by the
// GPU renderer's tests (internal/render/gpu with the CPU executor, internal/render/gpu/metal with a
// real device), so every backend is checked against paint.CPU on the same
// display lists with the same thresholds. It is imported only by tests.
package gputest

import (
	"image"
	"image/color"
	"image/color/palette"
	"math"
	"os"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

// Colors used by the scenes.
var (
	White = color.NRGBA{255, 255, 255, 255}
	Blue  = color.NRGBA{0x16, 0x77, 0xff, 255}
	Red   = color.NRGBA{0xf5, 0x22, 0x2d, 255}
	Green = color.NRGBA{0x52, 0xc4, 0x1a, 200}
	Ink   = color.NRGBA{0x1f, 0x1f, 0x1f, 235}
)

// SceneSize is the logical size every scene is drawn at.
var SceneSize = geom.Size{W: 96, H: 56}

// Scales are the device scales every scene is compared at.
var Scales = []float32{1, 1.5, 2}

// Result summarizes a comparison of two images.
type Result struct {
	MaxDiff int     // largest channel difference
	Over    int     // pixels with a channel difference above the threshold
	Frac    float64 // Over / all pixels
}

// MaxFrac is the largest fraction of pixels allowed over the threshold.
const MaxFrac = 0.005

// Compare compares two images of the same size channel by channel.
func Compare(a, b *image.RGBA, thr int) Result {
	var r Result
	for i := 0; i < len(a.Pix); i += 4 {
		px := 0
		for c := 0; c < 4; c++ {
			d := int(a.Pix[i+c]) - int(b.Pix[i+c])
			if d < 0 {
				d = -d
			}
			px = max(px, d)
		}
		r.MaxDiff = max(r.MaxDiff, px)
		if px > thr {
			r.Over++
		}
	}
	r.Frac = float64(r.Over) / float64(len(a.Pix)/4)
	return r
}

// Check compares got with want (the paint.CPU rendering of l), logs the
// numbers and fails t when more than MaxFrac of the pixels differ by more
// than thr, splitting the failures into rounded corners and elsewhere.
func Check(t testing.TB, name string, l *paint.List, scale float32, want, got *image.RGBA, thr int) Result {
	t.Helper()
	if got.Rect != want.Rect {
		t.Fatalf("%s: size %v, want %v", name, got.Rect, want.Rect)
	}
	r := Compare(got, want, thr)
	t.Logf("%-10s scale %.1f: max diff %d, over %d (%.3f%%) thr %d", name, scale, r.MaxDiff, r.Over, r.Frac*100, thr)
	if r.Frac > MaxFrac {
		corner, other, maxC, maxO := SplitCorners(got, want, thr, CornerBoxes(l, scale))
		t.Errorf("%s scale %v: %.3f%% pixels differ by more than %d (max %d); in rounded corners %d (max %d), elsewhere %d (max %d)",
			name, scale, r.Frac*100, thr, r.MaxDiff, corner, maxC, other, maxO)
	}
	return r
}

// CornerBoxes returns the device boxes (grown by a pixel) of the rounded
// corners of every rounded FillRect and PushClip in l, to tell the shader's
// SDF corner approximation apart from other differences in reports.
func CornerBoxes(l *paint.List, s float32) []image.Rectangle {
	var out []image.Rectangle
	add := func(r geom.Rect, rad geom.Radii) {
		rad = rad.Clamp(r.W, r.H)
		box := func(x0, y0, x1, y1 float32) {
			out = append(out, image.Rect(int(x0*s)-1, int(y0*s)-1, int(x1*s)+2, int(y1*s)+2))
		}
		box(r.X, r.Y, r.X+rad.TL, r.Y+rad.TL)
		box(r.Right()-rad.TR, r.Y, r.Right(), r.Y+rad.TR)
		box(r.Right()-rad.BR, r.Bottom()-rad.BR, r.Right(), r.Bottom())
		box(r.X, r.Bottom()-rad.BL, r.X+rad.BL, r.Bottom())
	}
	for _, op := range l.Ops {
		switch o := op.(type) {
		case paint.FillRect:
			if !o.Radii.IsZero() {
				add(o.Rect, o.Radii)
			}
		case paint.PushClip:
			if !o.Radii.IsZero() {
				add(o.Rect, o.Radii)
			}
		}
	}
	return out
}

// SplitCorners counts the pixels over thr inside and outside boxes.
func SplitCorners(a, b *image.RGBA, thr int, boxes []image.Rectangle) (corner, other, maxC, maxO int) {
	for y := a.Rect.Min.Y; y < a.Rect.Max.Y; y++ {
		for x := a.Rect.Min.X; x < a.Rect.Max.X; x++ {
			d := 0
			for c := 0; c < 4; c++ {
				i := a.PixOffset(x, y) + c
				v := int(a.Pix[i]) - int(b.Pix[i])
				d = max(d, v, -v)
			}
			if d <= thr {
				continue
			}
			in := false
			for _, bx := range boxes {
				in = in || image.Pt(x, y).In(bx)
			}
			if in {
				corner, maxC = corner+1, max(maxC, d)
			} else {
				other, maxO = other+1, max(maxO, d)
			}
		}
	}
	return
}

// Text engines.
//
// The parity and golden tests compare against paint.CPU and against ui's
// golden images (ui/testdata/basic_*.png), which are recorded with the
// pure-Go text engine: glyph outlines parsed and rasterized in Go, the
// same on every machine with the same fonts. The system text engine
// (CoreText, DirectWrite, FreeType; see text.UseSystemEngine) rasterizes
// with the platform's hinting, so its masks differ from the goldens.
//
// The engine is a process-wide switch in package text, and only new glyph
// sources pick up a switch (text has no API to build pure-Go faces for
// system fonts otherwise; ui.Driver shapes through the same registry), so
// every package using gputest pins the Go engine once, before any test
// runs, by calling Main from its TestMain. Tests that compare the system
// engine call SystemText, which switches the engine for that test only;
// they must not run in parallel with other text tests (no gputest
// package uses t.Parallel).

// systemText is the engine the current test asked for: false (Main) for
// the Go engine, true inside SystemText.
var systemText bool

// Main pins the pure-Go text engine for the whole test binary and runs the
// tests. Call it from TestMain:
//
//	func TestMain(m *testing.M) { gputest.Main(m) }
func Main(m *testing.M) {
	text.UseSystemEngine(false)
	systemText = false
	os.Exit(m.Run())
}

// SystemText switches text to the system text engine for the rest of t
// (restoring the previous engine, the Go engine under Main, in
// t.Cleanup), skipping t when the platform has
// none. Scenes made (Scene.Make, Shape) afterwards draw with system-engine
// glyph sources. Not for parallel tests.
func SystemText(t testing.TB) {
	t.Helper()
	prevEngine, prevSystem := text.UseSystemEngine(true), systemText
	systemText = true
	t.Cleanup(func() {
		text.UseSystemEngine(prevEngine)
		systemText = prevSystem
	})
	if e := text.Engine(); e == "go" {
		t.Skip("no system text engine on this platform")
	}
	if len(text.Families()) == 0 {
		t.Skip("no system fonts")
	}
}

// Shape shapes s with the system fonts, skipping t when there are none. It
// fails t when the glyphs do not come from the engine the test pinned
// (Main: the pure-Go engine; SystemText: the system engine), so a test
// cannot silently compare another engine's glyphs.
func Shape(t testing.TB, s string, size float32) []paint.Glyph {
	t.Helper()
	if len(text.Families()) == 0 {
		t.Skip("no system fonts")
	}
	gs := text.Shape(s, text.Style{Size: size}, text.Options{}).Glyphs
	CheckEngine(t, gs)
	return gs
}

// CheckEngine fails t when a glyph of gs is rasterized by another text
// engine than the one the test pinned (see Main and SystemText).
func CheckEngine(t testing.TB, gs []paint.Glyph) {
	t.Helper()
	for _, g := range gs {
		f, ok := g.Src.(*text.Face)
		if !ok {
			continue
		}
		if f.SystemRasterized() != systemText {
			want := "the pure-Go engine (gputest.Main in TestMain)"
			if systemText {
				want = "the system engine (gputest.SystemText)"
			}
			t.Fatalf("glyph %d of %s is rasterized by the %s engine, want %s", g.ID, f.Family, engineName(f.SystemRasterized()), want)
		}
	}
}

// CheckListEngine runs CheckEngine on every Text op of l (lists built by
// ui.Driver, which shapes through package text's registry).
func CheckListEngine(t testing.TB, l *paint.List) {
	t.Helper()
	for _, op := range l.Ops {
		if o, ok := op.(paint.Text); ok {
			CheckEngine(t, o.Glyphs)
		}
	}
}

func engineName(system bool) string {
	if system {
		return "system"
	}
	return "pure-Go"
}

// ShapeStyle is Shape with a full text style.
func ShapeStyle(t testing.TB, s string, st text.Style) []paint.Glyph {
	t.Helper()
	if len(text.Families()) == 0 {
		t.Skip("no system fonts")
	}
	gs := text.Shape(s, st, text.Options{}).Glyphs
	CheckEngine(t, gs)
	return gs
}

// TextScenes are the scenes that draw text: the text parity scenes of
// Scenes plus "textmix" (sizes from 9 to 28 px, bold, italic, monospace,
// fractional origins), used by the system-engine parity tests.
func TextScenes() []Scene {
	var out []Scene
	for _, sc := range Scenes {
		if sc.Text {
			out = append(out, sc)
		}
	}
	return append(out, Scene{Name: "textmix", Thr: 4, Text: true, Make: func(t testing.TB) *paint.List {
		l := &paint.List{Background: White}
		l.Glyphs(geom.Pt(2.25, 1.5), ShapeStyle(t, "tiny 小字 9px", text.Style{Size: 9}), Ink)
		l.Glyphs(geom.Pt(48.6, 1.2), ShapeStyle(t, "Bold 粗", text.Style{Size: 12.5, Weight: text.Bold}), Blue)
		l.Glyphs(geom.Pt(2.7, 13.4), ShapeStyle(t, "Italic 斜体", text.Style{Size: 13, Italic: true}), Red)
		l.Glyphs(geom.Pt(52.1, 14), ShapeStyle(t, "mono{}", text.Style{Size: 11, Mono: true}), Ink)
		l.Glyphs(geom.Pt(3.4, 27.3), ShapeStyle(t, "大字 Ag", text.Style{Size: 28}), Ink)
		return l
	}})
}

// TestImage is a small image with varying color and alpha.
func TestImage() image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, 24, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 24; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(x * 10), uint8(y * 15), uint8(255 - x*8), uint8(155 + y*6)})
		}
	}
	return img
}

// Scene is a display list compared between paint.CPU and a GPU backend.
type Scene struct {
	Name string
	Thr  int  // per-channel threshold
	Text bool // draws text
	Make func(t testing.TB) *paint.List
}

// Scenes are the parity scenes (drawn at SceneSize).
var Scenes = []Scene{
	{Name: "solid", Thr: 2, Make: func(t testing.TB) *paint.List {
		l := &paint.List{Background: White}
		l.Fill(geom.R(4, 4, 30, 20), geom.Radii{}, Blue)
		l.Fill(geom.R(10.3, 12.6, 40.5, 15.2), geom.Radii{}, Green)
		l.Fill(geom.R(50.25, 3.75, 0.5, 30), geom.Radii{}, Red)
		l.Line(geom.Pt(2, 40.5), geom.Pt(90, 40.5), 1, Ink)
		return l
	}},
	{Name: "rounded", Thr: 2, Make: func(t testing.TB) *paint.List {
		l := &paint.List{Background: White}
		l.Fill(geom.R(4, 4, 40, 30), geom.Rad(6), Blue)
		l.Fill(geom.R(50.5, 6.3, 36, 24), geom.Radii{TL: 12, TR: 2, BR: 8, BL: 0}, Green)
		l.Fill(geom.R(10, 40, 70, 8), geom.Rad(20), Red) // pill: radii clamp
		return l
	}},
	{Name: "nestclip", Thr: 2, Make: func(t testing.TB) *paint.List {
		l := &paint.List{Background: White}
		l.Clip(geom.R(5, 5, 80, 40), geom.Radii{})
		l.Fill(geom.R(0, 0, 100, 60), geom.Radii{}, color.NRGBA{240, 240, 240, 255})
		l.Clip(geom.R(10.5, 8.5, 60, 30), geom.Rad(8))
		l.Fill(geom.R(0, 0, 100, 60), geom.Radii{}, Blue)
		l.Clip(geom.R(20, 15, 80, 10), geom.Radii{})
		l.Fill(geom.R(0, 0, 100, 60), geom.Radii{}, Red)
		l.Pop()
		l.Fill(geom.R(40, 30, 50, 20), geom.Rad(4), Green)
		l.Pop()
		l.Pop()
		l.Fill(geom.R(60, 40, 30, 15), geom.Radii{}, Ink)
		return l
	}},
	{Name: "text", Thr: 4, Text: true, Make: func(t testing.TB) *paint.List {
		l := &paint.List{Background: White}
		l.Glyphs(geom.Pt(4.3, 3.6), Shape(t, "Tango 你好，世界", 15), Ink)
		l.Glyphs(geom.Pt(6, 26), Shape(t, "GPU 渲染 glyphs", 13), Blue)
		return l
	}},
	{Name: "card", Thr: 2, Make: func(t testing.TB) *paint.List {
		l := &paint.List{Background: color.NRGBA{245, 245, 245, 255}}
		r := geom.R(12, 10, 60, 34)
		l.Shadows(r, geom.Rad(8), []paint.Shadow{
			{Y: 6, Blur: 16, Spread: -8, Color: color.NRGBA{0, 0, 0, 20}},
			{Y: 3, Blur: 6, Spread: -4, Color: color.NRGBA{0, 0, 0, 30}},
			{Y: 9, Blur: 28, Spread: 8, Color: color.NRGBA{0, 0, 0, 12}},
		})
		l.Fill(r, geom.Rad(8), White)
		l.Stroke(r, geom.Rad(8), 1, color.NRGBA{0xd9, 0xd9, 0xd9, 255})
		return l
	}},
	{Name: "gradient", Thr: 2, Make: func(t testing.TB) *paint.List {
		l := &paint.List{Background: White}
		l.Add(paint.LinearGradient{Rect: geom.R(5, 5, 80, 30), Radii: geom.Rad(6),
			P0: geom.Pt(5, 5), P1: geom.Pt(85, 35), From: Blue, To: color.NRGBA{0xeb, 0x2f, 0x96, 180}})
		return l
	}},
	{Name: "dashed", Thr: 2, Make: func(t testing.TB) *paint.List {
		l := &paint.List{Background: White}
		l.StrokeDashed(geom.R(6, 6, 70, 30), geom.Rad(4), 1, Ink, []float32{4, 2})
		l.StrokePath(paint.NewPath().MoveTo(5, 45).LineTo(85, 40), paint.StrokeStyle{Width: 2}, Red)
		return l
	}},
	{Name: "image", Thr: 4, Make: func(t testing.TB) *paint.List {
		l := &paint.List{Background: White}
		img := TestImage()
		l.DrawImage(img, geom.R(4, 4, 48, 32), geom.Radii{})
		l.DrawImage(img, geom.R(55.5, 8.5, 30, 20), geom.Rad(6))
		return l
	}},
	{Name: "images", Thr: 4, Make: func(t testing.TB) *paint.List {
		// Image extremes: reduced to 1×1 and to a small size, enlarged
		// 16×, a 1×1 source, *image.Gray and *image.Paletted sources, one
		// inside a rounded clip.
		l := &paint.List{Background: White}
		big := image.NewRGBA(image.Rect(0, 0, 64, 64))
		for y := 0; y < 64; y++ {
			for x := 0; x < 64; x++ {
				big.SetRGBA(x, y, color.RGBA{uint8(x * 4), uint8(y * 4), 128, 255})
			}
		}
		small := image.NewNRGBA(image.Rect(0, 0, 4, 3))
		for i := range small.Pix {
			small.Pix[i] = uint8(i*37 + 20)
		}
		one := image.NewRGBA(image.Rect(0, 0, 1, 1))
		one.SetRGBA(0, 0, color.RGBA{10, 120, 200, 220})
		gray := image.NewGray(image.Rect(0, 0, 5, 4))
		for i := range gray.Pix {
			gray.Pix[i] = uint8(i * 13)
		}
		pal := image.NewPaletted(image.Rect(0, 0, 6, 5), palette.Plan9)
		for i := range pal.Pix {
			pal.Pix[i] = uint8(i * 7)
		}
		l.DrawImage(big, geom.R(2, 2, 1, 1), geom.Radii{})
		l.DrawImage(big, geom.R(5, 2, 13, 9.5), geom.Radii{})
		l.DrawImage(small, geom.R(20, 2, 64, 48), geom.Rad(5))
		l.DrawImage(one, geom.R(2, 14, 14, 10), geom.Radii{})
		l.DrawImage(gray, geom.R(2.5, 27.25, 20, 16), geom.Radii{TL: 6})
		l.Clip(geom.R(60, 20, 34, 34), geom.Rad(9))
		l.DrawImage(pal, geom.R(55.5, 18, 40, 38), geom.Radii{})
		l.Pop()
		return l
	}},
	{Name: "opacity", Thr: 2, Text: true, Make: func(t testing.TB) *paint.List {
		l := &paint.List{Background: White}
		l.Fill(geom.R(0, 20, 100, 10), geom.Radii{}, Red)
		l.Opacity(0.6)
		l.Fill(geom.R(5, 5, 50, 40), geom.Rad(6), Blue)
		l.Glyphs(geom.Pt(10, 10), Shape(t, "半透明 Tango", 14), White)
		l.Opacity(0.5)
		l.Fill(geom.R(40, 15, 40, 30), geom.Radii{}, Green)
		l.Pop()
		l.Pop()
		l.Fill(geom.R(70, 2, 20, 10), geom.Radii{}, Ink)
		return l
	}},
	{Name: "opacitynest", Thr: 4, Text: true, Make: func(t testing.TB) *paint.List {
		// Three nested 0.5 groups inside a rounded clip, with a rounded
		// clip and text inside them; group contents overlap, so they must
		// be composited as groups, not drawn op by op.
		l := &paint.List{Background: White}
		l.Fill(geom.R(0, 24, 96, 8), geom.Radii{}, Red)
		l.Clip(geom.R(3.5, 2.5, 88, 50), geom.Rad(10))
		l.Opacity(0.5)
		l.Fill(geom.R(0, 0, 70, 56), geom.Radii{}, Blue)
		l.Glyphs(geom.Pt(6, 6), Shape(t, "外层 Outer", 12), White)
		l.Opacity(0.5)
		l.Clip(geom.R(20.25, 14, 60, 34), geom.Rad(8))
		l.Fill(geom.R(10, 10, 80, 40), geom.Radii{}, Green)
		l.Glyphs(geom.Pt(24, 18), Shape(t, "中层 Mid", 12), Ink)
		l.Opacity(0.5)
		l.Fill(geom.R(30, 28, 50, 16), geom.Rad(4), Red)
		l.Glyphs(geom.Pt(34, 30), Shape(t, "内层 in", 11), White)
		l.Pop()
		l.Pop()
		l.Pop()
		l.Pop()
		l.Pop()
		l.Fill(geom.R(84, 40, 10, 14), geom.Radii{}, Ink)
		return l
	}},
	{Name: "opacitygrad", Thr: 2, Make: func(t testing.TB) *paint.List {
		// Gradients (dithered) inside 0.5 groups whose layers start at
		// non-zero, non-multiple-of-4 device positions.
		l := &paint.List{Background: White}
		l.Opacity(0.5)
		l.LinearStops(geom.R(13.25, 7.5, 70, 18), geom.Radii{}, geom.Pt(13, 0), geom.Pt(83, 0),
			paint.Stop{Offset: 0, Color: color.NRGBA{100, 120, 140, 255}}, paint.Stop{Offset: 1, Color: color.NRGBA{104, 123, 146, 255}})
		l.Fill(geom.R(60, 12, 20, 20), geom.Rad(3), Blue)
		l.Pop()
		l.Opacity(0.5)
		l.Gradient(paint.Gradient{Kind: paint.GradientRadial, Rect: geom.R(6.5, 31.3, 49, 22), Radii: geom.Rad(6), Start: geom.Pt(30, 41), End: geom.Pt(55, 41),
			Stops: []paint.Stop{{Offset: 0, Color: White}, {Offset: 1, Color: color.NRGBA{0x16, 0x77, 0xff, 255}}}})
		l.Gradient(paint.Gradient{Kind: paint.GradientLinear, Rect: geom.R(40, 33, 50, 18), Start: geom.Pt(40, 0), End: geom.Pt(90, 0),
			Stops: []paint.Stop{{Offset: 0, Color: color.NRGBA{200, 60, 60, 120}}, {Offset: 1, Color: color.NRGBA{210, 64, 66, 200}}}})
		l.Pop()
		return l
	}},
	{Name: "strokes", Thr: 2, Make: func(t testing.TB) *paint.List {
		l := &paint.List{Background: White}
		l.Stroke(geom.R(4, 4, 40, 22), geom.Rad(6), 1, Ink)
		l.Stroke(geom.R(50.5, 4.25, 40, 22), geom.Radii{TL: 10, TR: 2, BR: 8}, 2.5, Blue)
		l.StrokeDashed(geom.R(4, 30, 40, 22), geom.Rad(4), 1, Red, []float32{4, 2})
		l.StrokeDashed(geom.R(50, 30, 40, 22), geom.Radii{TL: 8, BR: 3}, 1.5, Green, []float32{1, 1, 3, 1})
		l.Stroke(geom.R(30, 20, 12, 10), geom.Rad(2), 7, Ink) // width > half size: filled
		l.Clip(geom.R(20, 10, 50, 36), geom.Rad(12))
		l.Stroke(geom.R(15, 8, 60, 30), geom.Rad(3), 3, Red)
		l.Pop()
		return l
	}},
	{Name: "shadows", Thr: 2, Make: func(t testing.TB) *paint.List {
		l := &paint.List{Background: color.NRGBA{245, 245, 245, 255}}
		l.Shadows(geom.R(8, 6, 30, 18), geom.Rad(6), []paint.Shadow{
			{Y: 2, Blur: 8, Color: color.NRGBA{0, 0, 0, 60}},
			{Y: 1, Blur: 2, Spread: -1, Color: color.NRGBA{0, 0, 0, 40}},
		})
		l.Shadows(geom.R(50.3, 8.6, 32.5, 14.25), geom.Radii{TL: 12, BR: 3}, []paint.Shadow{{X: 2, Y: 3, Blur: 5, Spread: 2, Color: color.NRGBA{0x16, 0x77, 0xff, 140}}})
		l.Shadows(geom.R(10, 34, 24, 12), geom.Rad(4), []paint.Shadow{{Spread: 2, Color: color.NRGBA{0x16, 0x77, 0xff, 90}}}) // focus ring: no blur
		l.Shadows(geom.R(60, 36, 20, 8), geom.Rad(20), []paint.Shadow{{Blur: 40, Color: color.NRGBA{0, 0, 0, 160}}})          // blur wider than the rect, pill radii
		l.Clip(geom.R(40, 28, 44, 20), geom.Rad(6))
		l.Shadows(geom.R(36, 30, 20, 14), geom.Radii{}, []paint.Shadow{{Y: 2, Blur: 6, Color: color.NRGBA{0, 0, 0, 120}}})
		l.Pop()
		return l
	}},
	{Name: "gradients", Thr: 2, Make: func(t testing.TB) *paint.List {
		// Linear (multi-stop, translucent), radial and conic gradients with
		// rounded corners, one inside a rounded clip, and a LinearGradient.
		l := &paint.List{Background: White}
		l.LinearStops(geom.R(3, 3, 42, 22), geom.Rad(6), geom.Pt(3, 3), geom.Pt(45, 25),
			paint.Stop{Offset: 0, Color: Blue}, paint.Stop{Offset: 0.4, Color: color.NRGBA{0x52, 0xc4, 0x1a, 120}}, paint.Stop{Offset: 1, Color: Red})
		l.Gradient(paint.Gradient{Kind: paint.GradientRadial, Rect: geom.R(48.5, 2.25, 30, 24), Radii: geom.Radii{TL: 12, BR: 4},
			Start: geom.Pt(60, 12), End: geom.Pt(75, 18), Stops: []paint.Stop{{Offset: 0, Color: White}, {Offset: 0.6, Color: Blue}, {Offset: 1, Color: color.NRGBA{0x16, 0x77, 0xff, 0}}}})
		l.Gradient(paint.Gradient{Kind: paint.GradientConic, Rect: geom.R(4, 29, 24, 24), Radii: geom.Rad(12),
			Start: geom.Pt(16, 41), End: geom.Pt(16, 30), Stops: []paint.Stop{{Offset: 0, Color: Red}, {Offset: 0.33, Color: color.NRGBA{0xfa, 0xdb, 0x14, 255}}, {Offset: 0.66, Color: Blue}, {Offset: 1, Color: Red}}})
		l.Clip(geom.R(34, 30, 40, 22), geom.Rad(8))
		l.Gradient(paint.Gradient{Kind: paint.GradientLinear, Rect: geom.R(30, 28, 50, 26), Start: geom.Pt(30, 54), End: geom.Pt(80, 28),
			Stops: []paint.Stop{{Offset: 0, Color: Ink}, {Offset: 1, Color: Green}}})
		l.Pop()
		l.Add(paint.LinearGradient{Rect: geom.R(80, 30, 14, 24), Radii: geom.Rad(3), P0: geom.Pt(80, 30), P1: geom.Pt(80, 54), From: Green, To: color.NRGBA{0, 0, 0, 0}})
		return l
	}},
	{Name: "gradstops", Thr: 2, Make: func(t testing.TB) *paint.List {
		// Degenerate stops: a single stop, a hard edge (two stops at one
		// offset), unsorted stops, offsets outside 0..1, Start == End.
		l := &paint.List{Background: White}
		l.LinearStops(geom.R(3, 3, 28, 20), geom.Rad(4), geom.Pt(3, 0), geom.Pt(31, 0), paint.Stop{Offset: 0.5, Color: Blue})
		l.LinearStops(geom.R(34, 3, 28, 20), geom.Radii{}, geom.Pt(34, 0), geom.Pt(62, 0),
			paint.Stop{Offset: 0, Color: Red}, paint.Stop{Offset: 0.5, Color: Red}, paint.Stop{Offset: 0.5, Color: Blue}, paint.Stop{Offset: 1, Color: Blue})
		l.LinearStops(geom.R(65, 3, 28, 20), geom.Rad(6), geom.Pt(65, 3), geom.Pt(93, 23),
			paint.Stop{Offset: 1, Color: Green}, paint.Stop{Offset: 0, Color: Red}, paint.Stop{Offset: 0.3, Color: Blue})
		l.Gradient(paint.Gradient{Kind: paint.GradientRadial, Rect: geom.R(3, 27, 28, 26), Start: geom.Pt(17, 40), End: geom.Pt(17, 52),
			Stops: []paint.Stop{{Offset: -0.5, Color: Red}, {Offset: 0.4, Color: Blue}, {Offset: 0.4, Color: White}, {Offset: 1.5, Color: Ink}}})
		l.Gradient(paint.Gradient{Kind: paint.GradientConic, Rect: geom.R(34, 27, 28, 26), Radii: geom.Rad(5), Start: geom.Pt(48, 40), End: geom.Pt(48, 40),
			Stops: []paint.Stop{{Offset: 0, Color: Red}, {Offset: 1, Color: Blue}}})
		l.Gradient(paint.Gradient{Kind: paint.GradientLinear, Rect: geom.R(65, 27, 28, 26), Start: geom.Pt(70, 30), End: geom.Pt(70, 30),
			Stops: []paint.Stop{{Offset: 0, Color: Green}, {Offset: 1, Color: Blue}}})
		return l
	}},
	{Name: "checker", Thr: 2, Make: func(t testing.TB) *paint.List {
		l := &paint.List{Background: White}
		l.Checker(geom.R(3, 3, 40, 24), geom.Rad(4), 4, color.NRGBA{}, color.NRGBA{})
		l.Checker(geom.R(46.5, 3.25, 45, 22.5), geom.Radii{TL: 10, BR: 2}, 5.5, Blue, color.NRGBA{0xf5, 0x22, 0x2d, 90})
		l.Checker(geom.R(3, 30, 30, 22), geom.Radii{}, 3, Ink, White)
		l.Clip(geom.R(40, 30, 50, 22), geom.Rad(9))
		l.Checker(geom.R(36, 28, 58, 26), geom.Radii{}, 6, Green, color.NRGBA{})
		l.Pop()
		return l
	}},
	{Name: "paths", Thr: 2, Make: func(t testing.TB) *paint.List {
		l := &paint.List{Background: White}
		l.Path(paint.NewPath().MoveTo(4.3, 3.7).LineTo(40, 10).LineTo(12, 30.5).Close(), Blue)
		l.Path(paint.NewPath().Circle(60.5, 18.25, 12.3), color.NRGBA{0xeb, 0x2f, 0x96, 160})
		star := paint.NewPath().MoveTo(20, 34).LineTo(26, 52).LineTo(10, 40).LineTo(30, 40).LineTo(14, 52).Close()
		l.Path(star, Green) // self-intersecting: non-zero rule
		l.StrokePath(paint.NewPath().MoveTo(36, 50).QuadTo(50, 30, 70, 50), paint.StrokeStyle{Width: 2.5}, Red)
		l.Path(paint.NewPath().Circle(94, 50, 9), Ink) // partly outside the target
		l.Clip(geom.R(68, 30, 24, 20), geom.Rad(7))
		l.Path(paint.NewPath().MoveTo(60, 28).CubeTo(80, 20, 100, 60, 64, 54).Close(), Blue)
		l.Pop()
		l.Path(paint.NewPath().Circle(200, 200, 10), Ink) // outside: nothing
		return l
	}},
	{Name: "pathgrad", Thr: 2, Make: func(t testing.TB) *paint.List {
		l := &paint.List{Background: White}
		stops := []paint.Stop{{Offset: 0, Color: Red}, {Offset: 0.5, Color: color.NRGBA{0xfa, 0xad, 0x14, 200}}, {Offset: 1, Color: Blue}}
		area := paint.NewPath().MoveTo(4, 50).LineTo(4, 30).LineTo(20, 12.5).LineTo(36, 24).LineTo(52, 6).LineTo(52, 50).Close()
		l.Gradient(paint.Gradient{Kind: paint.GradientLinear, Path: area, Start: geom.Pt(0, 6), End: geom.Pt(0, 50), Stops: stops})
		l.Gradient(paint.Gradient{Kind: paint.GradientRadial, Path: paint.NewPath().Circle(72, 20, 15.5), Start: geom.Pt(68, 16),
			End: geom.Pt(86, 20), Stops: stops})
		l.Clip(geom.R(56, 34, 36, 18), geom.Rad(6))
		l.Gradient(paint.Gradient{Kind: paint.GradientConic, Path: paint.NewPath().Circle(74, 44, 14), Start: geom.Pt(74, 44),
			End: geom.Pt(90, 44), Stops: stops})
		l.Pop()
		return l
	}},
	{Name: "xform", Thr: 4, Text: true, Make: func(t testing.TB) *paint.List {
		// Axis-aligned transforms (translate, uniform positive scale)
		// around every op kind; drawn natively by the GPU renderer.
		l := &paint.List{Background: White}
		icon := paint.StrokeToFill(paint.NewPath().MoveTo(1, 1).LineTo(7, 7).MoveTo(7, 1).LineTo(1, 7), paint.StrokeStyle{Width: 1.5, Cap: paint.CapRound})
		l.Transform(geom.Translate(2.5, 1.25))
		l.Shadows(geom.R(2, 2, 30, 16), geom.Rad(4), []paint.Shadow{{Y: 1, Blur: 3, Color: color.NRGBA{0, 0, 0, 80}}})
		l.Fill(geom.R(2, 2, 30, 16), geom.Rad(4), Blue)
		l.Stroke(geom.R(2, 2, 30, 16), geom.Rad(4), 1.5, Ink)
		l.Glyphs(geom.Pt(4, 5), Shape(t, "Tango 变", 9), White)
		l.Pop()
		// Icons at whole and half logical pixels: on the quarter-pixel
		// grid the GPU renderer keys translated path masks by at every
		// scale here (other phases are drawn up to 1/8 px off).
		for i, x := range []float32{36, 45.5, 54.5, 63} {
			l.Transform(geom.Translate(x, 2+float32(i%2)*0.5))
			l.Path(icon, Ink)
			l.Pop()
		}
		l.Transform(geom.ScaleAround(2, 2, geom.Pt(74, 2)))
		l.Fill(geom.R(74, 2, 9, 5), geom.Rad(1.5), Red)
		l.Path(paint.NewPath().Circle(78.5, 4.5, 1.5), White)
		l.Pop()
		l.Transform(geom.Translate(0.5, 21))
		l.Transform(geom.Scale(1.5, 1.5))
		l.Clip(geom.R(1, 1, 26, 12), geom.Rad(4))
		l.LinearStops(geom.R(0, 0, 28, 14), geom.Radii{}, geom.Pt(0, 0), geom.Pt(28, 0),
			paint.Stop{Offset: 0, Color: Blue}, paint.Stop{Offset: 1, Color: Red})
		l.Checker(geom.R(14, 7, 12, 6), geom.Radii{}, 2, color.NRGBA{}, color.NRGBA{})
		l.Pop()
		l.DrawImage(TestImage(), geom.R(29, 1, 14, 10), geom.Rad(2))
		l.StrokeDashed(geom.R(45, 1, 14, 10), geom.Rad(2), 1, Red, []float32{2, 1})
		l.Opacity(0.5)
		l.Fill(geom.R(30, 12, 20, 9), geom.Rad(2), Green)
		l.Glyphs(geom.Pt(31, 13), Shape(t, "半 ab", 7), Ink)
		l.Pop()
		l.Gradient(paint.Gradient{Kind: paint.GradientRadial, Path: paint.NewPath().Circle(8, 18, 4), Start: geom.Pt(8, 18), End: geom.Pt(12, 18),
			Stops: []paint.Stop{{Offset: 0, Color: White}, {Offset: 1, Color: Blue}}})
		l.Pop()
		l.Pop()
		return l
	}},
	{Name: "xformrot", Thr: 4, Text: true, Make: func(t testing.TB) *paint.List {
		// A rotated subtree (inside a translated one) next to native
		// content: the rotated part is a patch, the rest native.
		l := &paint.List{Background: White}
		l.Fill(geom.R(2, 2, 40, 20), geom.Rad(4), Blue)
		l.Transform(geom.Translate(30, 4))
		l.Transform(geom.RotateAround(math.Pi/6, geom.Pt(30, 20)))
		l.Fill(geom.R(14, 12, 32, 16), geom.Rad(4), Red)
		l.Glyphs(geom.Pt(16, 15), Shape(t, "旋转 rot", 9), White)
		l.DrawImage(TestImage(), geom.R(36, 26, 12, 8), geom.Radii{})
		l.Pop()
		l.Fill(geom.R(40, 38, 20, 10), geom.Radii{}, Green)
		l.Pop()
		l.Transform(geom.Scale(-1, 1)) // mirrored: also a patch
		l.Fill(geom.R(-20, 40, 14, 10), geom.Rad(3), Ink)
		l.Pop()
		return l
	}},
	{Name: "xformnest", Thr: 4, Text: true, Make: func(t testing.TB) *paint.List {
		// Translate ∘ scale ∘ translate, with clips and a group inside.
		l := &paint.List{Background: White}
		l.Transform(geom.Translate(10, 4))
		l.Fill(geom.R(0, 0, 30, 10), geom.Rad(3), Blue)
		l.Transform(geom.Scale(1.5, 1.5))
		l.Clip(geom.R(0, 8, 50, 24), geom.Rad(5))
		l.Transform(geom.Translate(-3, 2))
		l.Fill(geom.R(4, 8, 40, 20), geom.Radii{}, Green)
		l.Glyphs(geom.Pt(6, 10), Shape(t, "嵌套 Nest", 8), Ink)
		l.Path(paint.NewPath().MoveTo(30, 12).LineTo(44, 14).LineTo(34, 26).Close(), Red)
		l.Transform(geom.Identity())
		l.Stroke(geom.R(6, 20, 20, 6), geom.Rad(2), 1, Red)
		l.Pop()
		l.Pop()
		l.Pop()
		l.Fill(geom.R(40, 2, 8, 4), geom.Radii{}, Ink) // after the clip's Pop, still scaled
		l.Pop()
		l.Pop()
		l.Fill(geom.R(88, 46, 6, 6), geom.Radii{}, Red) // untransformed again
		return l
	}},
}

// SceneNamed returns the parity scene called name.
func SceneNamed(name string) Scene {
	for _, sc := range Scenes {
		if sc.Name == name {
			return sc
		}
	}
	panic("gputest: no scene " + name)
}
