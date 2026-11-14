package icon

import (
	"image/color"
	"math"
	"sync"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

func TestDirectionalWhitelist(t *testing.T) {
	want := map[*Icon]bool{
		Left: true, Right: true, DoubleLeft: true, DoubleRight: true, ArrowRight: true,
		MenuFold: true, MenuUnfold: true, Logout: true, Message: true,
		Profile: true, Form: true, FileText: true,
		ArrowLeft: true, CaretLeft: true, CaretRight: true, ExternalLink: true, PanelLeft: true,
		Undo: true, Redo: true, Send: true, Sound: true,
	}
	for _, n := range Names() {
		ic := ByName(n)
		if ic.IsDirectional() != want[ic] {
			t.Errorf("%s: directional=%v want %v", n, ic.IsDirectional(), want[ic])
		}
	}
	for _, ic := range []*Icon{Rise, Fall, Up, Down, Clock, Loading, RotateLeft, RotateRight, Swap, Play, History, Sync} {
		if ic.IsDirectional() {
			t.Errorf("%s must not be directional", ic.Name)
		}
	}
	var nilIcon *Icon
	if nilIcon.IsDirectional() {
		t.Fatal("nil icon")
	}
	if ic := New("x", "M5 12H19", "").Directional(); !ic.IsDirectional() {
		t.Fatal("custom Directional")
	}
}

// render draws ic in a 24×24 canvas and returns the alpha grid.
func render(t *testing.T, ic *Icon, o DrawOpts) [24][24]uint8 {
	t.Helper()
	var l paint.List
	DrawWith(&l, ic, geom.R(0, 0, 24, 24), color.NRGBA{A: 255}, o)
	r := paint.NewCPU()
	if err := r.Render(&l, geom.Size{W: 24, H: 24}, 1); err != nil {
		t.Fatal(err)
	}
	var out [24][24]uint8
	img := r.Image()
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			out[y][x] = img.RGBAAt(x, y).A
		}
	}
	return out
}

// near allows small antialiasing differences between mirrored renditions.
func near(a, b uint8) bool { return int(a)-int(b) <= 12 && int(b)-int(a) <= 12 }

func TestMirrorDraw(t *testing.T) {
	// Right mirrored in RTL equals the horizontal flip of Right in LTR.
	ltr := render(t, Right, DrawOpts{})
	rtl := render(t, Right, DrawOpts{RTL: true})
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			if !near(rtl[y][x], ltr[y][23-x]) {
				t.Fatalf("pixel %d,%d: %d vs flipped %d", x, y, rtl[y][x], ltr[y][23-x])
			}
		}
	}
	// Non-directional icons ignore RTL, Mirror forces it.
	if render(t, Clock, DrawOpts{RTL: true}) != render(t, Clock, DrawOpts{}) {
		t.Fatal("clock mirrored")
	}
	if render(t, Right, DrawOpts{Mirror: true}) != rtl {
		t.Fatal("Mirror != RTL on directional icon")
	}
	// Rotate first, then mirror: Right rotated 90° points down in both
	// directions (the down chevron is symmetric about the vertical axis).
	down := render(t, Right, DrawOpts{Angle: math.Pi / 2})
	downRTL := render(t, Right, DrawOpts{Angle: math.Pi / 2, RTL: true})
	for y := 0; y < 24; y++ {
		for x := 0; x < 24; x++ {
			if !near(downRTL[y][x], down[y][23-x]) {
				t.Fatalf("rotated pixel %d,%d", x, y)
			}
		}
	}
	// The tip of the down chevron is near the bottom centre either way.
	if downRTL[15][12] == 0 && downRTL[15][11] == 0 {
		t.Fatal("rotated RTL chevron does not point down")
	}
}

func TestMirrorCacheKey(t *testing.T) {
	ic := New("arrow", "M4 12H19.5M13 5.5L19.5 12L13 18.5", "").Directional()
	var a, b, c paint.List
	r := geom.R(0, 0, 16, 16)
	Draw(&a, ic, r, color.NRGBA{A: 255}, 0)
	DrawWith(&b, ic, r, color.NRGBA{A: 255}, DrawOpts{RTL: true})
	DrawWith(&c, ic, geom.R(40, 3, 16, 16), color.NRGBA{A: 255}, DrawOpts{RTL: true})
	pa, pb, pc := paths(a), paths(b), paths(c)
	if len(pa) != 1 || pa[0] == pb[0] {
		t.Fatal("mirrored rendition must be a separate cached path")
	}
	if pb[0] != pc[0] {
		t.Fatal("mirrored rendition must be reused")
	}
	if bb := pb[0].Bounds(); bb.X < -0.01 || bb.Right() > 16.01 {
		t.Fatalf("mirrored path outside the square: %+v", bb)
	}
}

func TestDrawConcurrent(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var l paint.List
			DrawWith(&l, ArrowRight, geom.R(0, 0, 16, 16), color.NRGBA{A: 255}, DrawOpts{RTL: i%2 == 0})
		}(i)
	}
	wg.Wait()
}

func TestIconNote(t *testing.T) {
	type note struct {
		name          string
		dir, mirrored bool
		r             geom.Rect
	}
	var got []note
	l := &paint.List{IconNote: func(name string, dir, mirrored bool, r geom.Rect) {
		got = append(got, note{name, dir, mirrored, r})
	}}
	black := color.NRGBA{A: 255}
	r := geom.Rect{X: 10, Y: 20, W: 16, H: 16}
	Draw(l, Right, r, black, 0)
	DrawWith(l, Right, r, black, DrawOpts{RTL: true})
	DrawWith(l, Search, r, black, DrawOpts{RTL: true})
	DrawWith(l, Search, r, black, DrawOpts{Mirror: true})
	DrawWith(l, Right, r, color.NRGBA{}, DrawOpts{RTL: true}) // transparent: not drawn, not noted
	want := []note{
		{"right", true, false, r},
		{"right", true, true, r},
		{"search", false, false, r},
		{"search", false, true, r},
	}
	if len(got) != len(want) {
		t.Fatalf("notes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("note %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	l.Reset()
	if l.IconNote == nil {
		t.Fatal("Reset dropped IconNote")
	}
}
