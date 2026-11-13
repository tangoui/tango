package paint

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/tangoui/tango/ui/geom"
)

var (
	red   = color.NRGBA{255, 0, 0, 255}
	green = color.NRGBA{0, 255, 0, 255}
	blu   = color.NRGBA{0, 0, 255, 255}
)

func isColor(c color.RGBA, want color.NRGBA, tol int) bool {
	d := func(a, b uint8) bool { return int(a)-int(b) <= tol && int(b)-int(a) <= tol }
	return d(c.R, want.R) && d(c.G, want.G) && d(c.B, want.B)
}

func TestTransformRotate90Rect(t *testing.T) {
	l := &List{Background: white}
	l.Transform(geom.RotateAround(math.Pi/2, geom.Pt(20, 20)))
	// A horizontal bar right of the centre becomes a vertical bar below it.
	l.Fill(geom.R(20, 18, 15, 4), geom.Radii{}, blue)
	l.Pop()
	img := render(t, l, 40, 40, 1).Image()
	for _, p := range []image.Point{{18, 20}, {21, 34}, {20, 27}} {
		if c := img.RGBAAt(p.X, p.Y); !isColor(c, blue, 0) {
			t.Fatalf("inside %v = %v", p, c)
		}
	}
	for _, p := range []image.Point{{17, 27}, {22, 27}, {20, 35}, {27, 20}, {20, 19}} {
		if c := img.RGBAAt(p.X, p.Y); !isColor(c, white, 0) {
			t.Fatalf("outside %v = %v", p, c)
		}
	}
}

func TestTransformScaleHalfGroupWithClip(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		l := &List{Background: white}
		l.Transform(geom.Scale(0.5, 0.5))
		l.Fill(geom.R(0, 0, 40, 40), geom.Radii{}, blue)
		l.Clip(geom.R(40, 0, 30, 30), geom.Radii{})
		l.Fill(geom.R(0, 0, 80, 80), geom.Radii{}, red) // clipped to 20..35 × 0..15
		l.Pop()
		l.Pop()
		l.Fill(geom.R(0, 30, 10, 10), geom.Radii{}, green) // after Pop: untransformed
		img := render(t, l, 50, 50, scale).Image()
		at := func(x, y float32) color.RGBA { return img.RGBAAt(int(x*scale), int(y*scale)) }
		checks := []struct {
			x, y float32
			c    color.NRGBA
		}{{19.5, 19.5, blue}, {20.5, 19.5, white}, {19.5, 20.5, white}, {21, 14.5, red}, {34.5, 1, red}, {35.5, 1, white}, {21, 15.5, white}, {5, 35, green}}
		for _, ch := range checks {
			if c := at(ch.x, ch.y); !isColor(c, ch.c, 0) {
				t.Fatalf("scale %v at (%v,%v) = %v want %v", scale, ch.x, ch.y, c, ch.c)
			}
		}
	}
}

func TestTransformUniformMatchesDirect(t *testing.T) {
	draw := func(l *List, k, dx float32) {
		l.Fill(geom.R(3*k+dx, 4*k, 10*k, 6*k), geom.Rad(2*k), blue)
		l.Stroke(geom.R(2*k+dx, 12*k, 12*k, 5*k), geom.Rad(1*k), 1*k, red)
		l.Shadows(geom.R(4*k+dx, 20*k, 8*k, 6*k), geom.Rad(2*k), []Shadow{{Y: 1 * k, Blur: 3 * k, Color: color.NRGBA{0, 0, 0, 80}}})
	}
	a := &List{Background: white}
	a.Transform(geom.Translate(5, 0).Mul(geom.Scale(2, 2)))
	draw(a, 1, 0)
	a.Pop()
	b := &List{Background: white}
	draw(b, 2, 5)
	ia := render(t, a, 40, 60, 1.5).Image()
	ib := render(t, b, 40, 60, 1.5).Image()
	for i := range ia.Pix {
		if d := int(ia.Pix[i]) - int(ib.Pix[i]); d > 1 || d < -1 {
			t.Fatalf("pixel %d differs: %d vs %d", i/4, ia.Pix[i], ib.Pix[i])
		}
	}
}

func quadImage() *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, 2, 2))
	m.Set(0, 0, red)
	m.Set(1, 0, green)
	m.Set(0, 1, blu)
	m.Set(1, 1, white)
	return m
}

func TestTransformRotateImage(t *testing.T) {
	l := &List{Background: color.NRGBA{0, 0, 0, 255}}
	l.Transform(geom.RotateAround(math.Pi/2, geom.Pt(10, 10)))
	l.DrawImage(quadImage(), geom.R(0, 0, 20, 20), geom.Radii{})
	l.Pop()
	img := render(t, l, 20, 20, 1).Image()
	// Clockwise: top-left (red) → top-right, bottom-left (blue) → top-left.
	want := map[image.Point]color.NRGBA{{15, 4}: red, {4, 4}: blu, {15, 15}: green, {4, 15}: white}
	for p, w := range want {
		if c := img.RGBAAt(p.X, p.Y); !isColor(c, w, 2) {
			t.Fatalf("%v = %v want %v", p, c, w)
		}
	}
	// 45°: the corners of the target square stay background, the centre is
	// a bilinear mix and edges are antialiased.
	l = &List{Background: color.NRGBA{0, 0, 0, 255}}
	l.Transform(geom.RotateAround(math.Pi/4, geom.Pt(10, 10)))
	l.DrawImage(quadImage(), geom.R(2, 2, 16, 16), geom.Radii{})
	l.Pop()
	img = render(t, l, 20, 20, 1).Image()
	if c := img.RGBAAt(0, 0); c != (color.RGBA{0, 0, 0, 255}) {
		t.Fatalf("corner should be background: %v", c)
	}
	if c := img.RGBAAt(10, 10); c.R < 60 || c.G < 60 || c.B < 60 {
		t.Fatalf("centre should blend all four: %v", c)
	}
}

func TestTransformNestedOpacityAndRoundClip(t *testing.T) {
	l := &List{Background: white}
	l.Opacity(0.5)
	l.Transform(geom.RotateAround(math.Pi/4, geom.Pt(20, 20)))
	l.Clip(geom.R(10, 10, 20, 20), geom.Rad(4))
	l.Fill(geom.R(0, 0, 40, 40), geom.Radii{}, blue)
	l.Pop()
	l.Pop()
	l.Pop()
	img := render(t, l, 40, 40, 1).Image()
	// Rotated clip is a diamond: centre filled at half opacity, the
	// unrotated clip's corner (10,10) is outside it.
	c := img.RGBAAt(20, 20)
	if c.R < 120 || c.R > 145 {
		t.Fatalf("centre should be 50%% blue over white: %v", c)
	}
	if c := img.RGBAAt(11, 11); !isColor(c, white, 0) {
		t.Fatalf("rotated clip corner %v", c)
	}
	if c := img.RGBAAt(20, 9); c.R > 160 {
		t.Fatalf("top vertex of the diamond should be painted: %v", c)
	}
}

func TestTransformRotatedShadowSymmetric(t *testing.T) {
	mk := func(rot bool) *image.RGBA {
		l := &List{Background: white}
		if rot {
			l.Transform(geom.RotateAround(math.Pi/2, geom.Pt(30, 30)))
		}
		l.Shadows(geom.R(20, 20, 20, 20), geom.Radii{}, []Shadow{{Blur: 8, Color: color.NRGBA{0, 0, 0, 160}}})
		if rot {
			l.Pop()
		}
		return render(t, l, 60, 60, 1).Image()
	}
	a, b := mk(false), mk(true)
	for _, p := range []image.Point{{30, 30}, {18, 30}, {30, 16}, {42, 42}, {14, 14}} {
		ca, cb := a.RGBAAt(p.X, p.Y), b.RGBAAt(p.X, p.Y)
		if d := int(ca.R) - int(cb.R); d > 6 || d < -6 {
			t.Fatalf("%v: %v vs rotated %v", p, ca, cb)
		}
	}
}

type squareGlyphs struct{}

// GlyphPath returns a size×size square sitting on the baseline.
func (squareGlyphs) GlyphPath(_ uint32, size float32) *Path {
	return NewPath().Rect(geom.R(0, -size, size, size))
}

func TestTransformGlyphs(t *testing.T) {
	g := []Glyph{{Src: squareGlyphs{}, ID: 1, Size: 10}}
	// Rotated 90° about the glyph origin: the square hangs right of x=20
	// and below… after rotating (0,-10)..(10,0) clockwise it covers
	// x∈[20,30], y∈[20,30].
	l := &List{Background: white}
	l.Transform(geom.RotateAround(math.Pi/2, geom.Pt(20, 20)))
	l.Glyphs(geom.Pt(20, 20), g, blue)
	l.Pop()
	r := render(t, l, 40, 40, 1)
	img := r.Image()
	if c := img.RGBAAt(25, 25); !isColor(c, blue, 0) {
		t.Fatalf("rotated glyph inside %v", c)
	}
	if c := img.RGBAAt(25, 15); !isColor(c, white, 0) {
		t.Fatalf("rotated glyph outside %v", c)
	}
	// Uniform scale 2 goes through the cached path at double size.
	l = &List{Background: white}
	l.Transform(geom.Scale(2, 2))
	l.Glyphs(geom.Pt(5, 15), g, blue)
	l.Pop()
	img = render(t, l, 40, 40, 1).Image()
	if c := img.RGBAAt(29, 11); !isColor(c, blue, 0) {
		t.Fatalf("scaled glyph inside %v", c)
	}
	if c := img.RGBAAt(31, 11); !isColor(c, white, 0) {
		t.Fatalf("scaled glyph outside %v", c)
	}
}

func TestGlyphSkew(t *testing.T) {
	g := []Glyph{{Src: squareGlyphs{}, ID: 1, Size: 20, Skew: 0.5}}
	l := &List{Background: white}
	l.Glyphs(geom.Pt(10, 30), g, blue)
	img := render(t, l, 50, 40, 1).Image()
	// Near the top (y≈11, 19 px above the baseline) the square is shifted
	// right by ~9.5 px; at the baseline it is not.
	if c := img.RGBAAt(12, 11); !isColor(c, white, 0) {
		t.Fatalf("top-left should have moved right: %v", c)
	}
	if c := img.RGBAAt(37, 11); !isColor(c, blue, 0) {
		t.Fatalf("top-right should have moved right: %v", c)
	}
	if c := img.RGBAAt(11, 29); !isColor(c, blue, 0) {
		t.Fatalf("baseline stays: %v", c)
	}
}

func TestGradientStops(t *testing.T) {
	l := &List{Background: white}
	l.LinearStops(geom.R(0, 0, 100, 10), geom.Radii{}, geom.Pt(0, 0), geom.Pt(100, 0),
		Stop{0, red}, Stop{0.5, green}, Stop{1, blu})
	l.Gradient(Gradient{Kind: GradientRadial, Rect: geom.R(0, 20, 40, 40), Start: geom.Pt(20, 40), End: geom.Pt(40, 40), Stops: []Stop{{0, red}, {1, blu}}})
	l.Gradient(Gradient{Kind: GradientConic, Rect: geom.R(50, 20, 40, 40), Start: geom.Pt(70, 40), End: geom.Pt(70, 0), Stops: []Stop{{0, red}, {0.5, green}, {1, blu}}})
	img := render(t, l, 100, 60, 1).Image()
	if c := img.RGBAAt(0, 5); !isColor(c, red, 3) {
		t.Fatalf("left %v", c)
	}
	if c := img.RGBAAt(50, 5); !isColor(c, green, 6) {
		t.Fatalf("middle %v", c)
	}
	if c := img.RGBAAt(99, 5); !isColor(c, blu, 3) {
		t.Fatalf("right %v", c)
	}
	if c := img.RGBAAt(20, 40); !isColor(c, red, 10) {
		t.Fatalf("radial centre %v", c)
	}
	if c := img.RGBAAt(1, 40); c.B < 200 {
		t.Fatalf("radial edge %v", c)
	}
	// Conic starts pointing up; clockwise a quarter turn is right (t=.25),
	// half a turn is down (green).
	if c := img.RGBAAt(70, 55); !isColor(c, green, 30) {
		t.Fatalf("conic bottom %v", c)
	}
	if c := img.RGBAAt(70, 22); c.R < 200 && c.B < 200 {
		t.Fatalf("conic top should be red or blue at the seam: %v", c)
	}
}

func TestChecker(t *testing.T) {
	l := &List{Background: color.NRGBA{0, 0, 0, 255}}
	l.Checker(geom.R(0, 0, 16, 16), geom.Radii{}, 4, color.NRGBA{}, color.NRGBA{})
	l.Transform(geom.RotateAround(math.Pi/2, geom.Pt(28, 8)))
	l.Checker(geom.R(20, 0, 16, 16), geom.Radii{}, 8, red, blu)
	l.Pop()
	img := render(t, l, 40, 16, 1).Image()
	if c := img.RGBAAt(1, 1); !isColor(c, CheckerLight, 0) {
		t.Fatalf("cell 0 %v", c)
	}
	if c := img.RGBAAt(5, 1); !isColor(c, CheckerDark, 0) {
		t.Fatalf("cell 1 %v", c)
	}
	// Rotated clockwise: local top-left (red) is now top-right.
	if c := img.RGBAAt(33, 2); !isColor(c, red, 0) {
		t.Fatalf("rotated checker %v", c)
	}
	if c := img.RGBAAt(22, 2); !isColor(c, blu, 0) {
		t.Fatalf("rotated checker %v", c)
	}
}

func TestImageUtilities(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2, 1))
	src.Set(0, 0, color.RGBA{0, 0, 0, 255})
	src.Set(1, 0, color.RGBA{255, 255, 255, 255})
	d := ResizeBilinear(src, 4, 1)
	for x, v := range []uint8{0, 64, 191, 255} {
		if got := d.RGBAAt(x, 0).R; int(got) < int(v)-1 || int(got) > int(v)+1 {
			t.Fatalf("x=%d got %d want %d", x, got, v)
		}
	}
	q := quadImage()
	r := Orient(q, 1, false, false)
	if r.RGBAAt(1, 0) != (color.RGBA{255, 0, 0, 255}) {
		t.Fatal("rotate 90 misplaced pixel")
	}
	if Orient(q, 4, false, false) != q {
		t.Fatal("full turn should return the source")
	}
	c := Resample(q, 1, 0, 1, 1, 3, 3) // crop the green pixel
	if c.RGBAAt(1, 1) != (color.RGBA{0, 255, 0, 255}) {
		t.Fatalf("crop %v", c.RGBAAt(1, 1))
	}
}

func TestClosedStrokeSeam(t *testing.T) {
	l := &List{Background: white}
	l.StrokePath(NewPath().Circle(40, 40, 30), StrokeStyle{Width: 10}, blue)
	img := render(t, l, 80, 80, 2).Image()
	for x := 132; x < 148; x++ { // ring at the path start (3 o'clock)
		if c := img.RGBAAt(x, 80); !isColor(c, blue, 0) {
			t.Fatalf("seam gap at x=%d: %v", x, c)
		}
	}
}

// A rounded clip multiplies into every op drawn under a rotation, and a
// clip pushed under a rotation clips to its rotated outline.
func TestTransformRoundedClipInteraction(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range src.Pix {
		src.Pix[i] = 255
	}
	rot := geom.RotateAround(math.Pi/4, geom.Pt(30, 30))
	ops := map[string]func(l *List){
		"fill": func(l *List) { l.Fill(geom.R(-10, -10, 80, 80), geom.Radii{}, blue) },
		"gradient": func(l *List) {
			l.LinearStops(geom.R(-10, -10, 80, 80), geom.Radii{}, geom.Pt(-10, 0), geom.Pt(70, 0), Stop{0, red}, Stop{1, blu})
		},
		"checker": func(l *List) { l.Checker(geom.R(-10, -10, 80, 80), geom.Radii{}, 4, red, blu) },
		"image":   func(l *List) { l.DrawImage(src, geom.R(-10, -10, 80, 80), geom.Radii{}) },
		"shadow":  func(l *List) { l.Shadows(geom.R(-10, -10, 80, 80), geom.Radii{}, []Shadow{{Color: red, Blur: 4}}) },
	}
	for name, draw := range ops {
		l := &List{Background: color.NRGBA{0, 0, 0, 255}}
		l.Clip(geom.R(10, 10, 40, 40), geom.Rad(15))
		l.Transform(rot)
		draw(l)
		l.Pop()
		l.Pop()
		img := render(t, l, 60, 60, 1).Image()
		if c := img.RGBAAt(11, 11); c != (color.RGBA{0, 0, 0, 255}) {
			t.Fatalf("%s: outside the rounded clip %v", name, c)
		}
		if c := img.RGBAAt(30, 30); c == (color.RGBA{0, 0, 0, 255}) {
			t.Fatalf("%s: centre not drawn", name)
		}
	}

	l := &List{Background: white}
	l.Transform(rot)
	l.Clip(geom.R(20, 20, 20, 20), geom.Radii{})
	l.Fill(geom.R(-10, -10, 80, 80), geom.Radii{}, blue)
	l.Pop()
	l.Pop()
	img := render(t, l, 60, 60, 1).Image()
	for _, p := range []image.Point{{30, 30}, {30, 17}, {42, 30}} {
		if c := img.RGBAAt(p.X, p.Y); !isColor(c, blue, 0) {
			t.Fatalf("inside rotated clip %v = %v", p, c)
		}
	}
	for _, p := range []image.Point{{21, 21}, {38, 38}, {30, 14}} {
		if c := img.RGBAAt(p.X, p.Y); !isColor(c, white, 0) {
			t.Fatalf("outside rotated clip %v = %v", p, c)
		}
	}
}
