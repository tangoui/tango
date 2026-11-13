package gpu

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/gpu/internal/gputest"
	"github.com/tangoui/tango/ui/gpu/shaders"
	"github.com/tangoui/tango/ui/paint"
)

var damageSize = geom.Size{W: 240, H: 160}

// buttonsList draws four buttons (rounded fill, border, label); button i
// gets color cs[i].
func buttonsList(t *testing.T, cs [4]color.NRGBA) *paint.List {
	l := &paint.List{Background: white}
	l.Fill(geom.R(0, 0, damageSize.W, 24), geom.Radii{}, color.NRGBA{0xf0, 0xf0, 0xf0, 255})
	for i, c := range cs {
		r := buttonRect(i)
		l.Fill(r, geom.Rad(6), c)
		l.Stroke(r, geom.Rad(6), 1, ink)
		l.Glyphs(geom.Pt(r.X+10, r.Y+20), shape(t, fmt.Sprintf("按钮 %d", i), 13), white)
	}
	return l
}

func buttonRect(i int) geom.Rect {
	return geom.R(12+float32(i%2)*110, 36+float32(i/2)*56, 96, 40)
}

// fullRender draws l with a new Renderer on a new device (a full frame).
func fullRender(t *testing.T, l *paint.List, size geom.Size, scale float32) *image.RGBA {
	t.Helper()
	dev := &cpuDevice{}
	if err := NewRenderer(dev).Render(l, size, scale); err != nil {
		t.Fatal(err)
	}
	return dev.img
}

// mustSame fails unless got equals want pixel for pixel.
func mustSame(t *testing.T, what string, got, want *image.RGBA) {
	t.Helper()
	if got.Rect != want.Rect {
		t.Fatalf("%s: size %v, want %v", what, got.Rect, want.Rect)
	}
	if res := compare(got, want, 0); res.Over > 0 {
		t.Fatalf("%s: %d pixels differ from the full frame (max %d)", what, res.Over, res.MaxDiff)
	}
}

// targetInstanceBounds returns the device bounds of the instances drawing
// into the frame's target.
func targetInstanceBounds(f *Frame) []image.Rectangle {
	var out []image.Rectangle
	for _, b := range f.Batches {
		if b.Target != 0 {
			continue
		}
		for _, in := range b.Instances {
			r := in.Rect
			g := image.Rect(int(math.Floor(float64(r.X))), int(math.Floor(float64(r.Y))),
				int(math.Ceil(float64(r.X+r.Z))), int(math.Ceil(float64(r.Y+r.W))))
			if in.Kind.X == shaders.KindShadow {
				g = g.Inset(-int(in.Params.Y) - 1)
			}
			out = append(out, g)
		}
	}
	return out
}

func TestDamageRedrawsOnlyDirty(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			dev := &cpuDevice{}
			r := NewRenderer(dev)
			cs := [4]color.NRGBA{blue, blue, blue, blue}
			if err := r.Render(buttonsList(t, cs), damageSize, scale); err != nil {
				t.Fatal(err)
			}
			if len(dev.last.Dirty) != 0 {
				t.Fatalf("first frame partial: %v", dev.last.Dirty)
			}
			full1 := len(targetInstanceBounds(dev.last))

			cs[2] = red
			l := buttonsList(t, cs)
			l.Damage(buttonRect(2).Inset(geom.Insets{Top: -1, Right: -1, Bottom: -1, Left: -1}))
			if err := r.Render(l, damageSize, scale); err != nil {
				t.Fatal(err) // verify mode (on in tests) compares with the full frame
			}
			// The last executed frame is verify mode's full frame; check the
			// partial one by rendering again without verify.
			r.SetVerify(false)
			if err := r.Render(l, damageSize, scale); err != nil {
				t.Fatal(err)
			}
			f := dev.last
			want := r.outBounds(buttonRect(2).Inset(geom.Insets{Top: -1, Right: -1, Bottom: -1, Left: -1}), 0)
			if len(f.Dirty) != 1 || f.Dirty[0] != want || len(f.Copies) != 0 {
				t.Fatalf("Dirty %v Copies %v, want [%v]", f.Dirty, f.Copies, want)
			}
			ib := targetInstanceBounds(f)
			if len(ib) == 0 || len(ib) >= full1 {
				t.Fatalf("%d instances in the partial frame, %d in the full one", len(ib), full1)
			}
			for _, b := range ib {
				if !b.Overlaps(f.Dirty[0]) {
					t.Fatalf("instance %v outside the dirty region %v", b, f.Dirty[0])
				}
			}
			mustSame(t, "partial", dev.img, fullRender(t, l, damageSize, scale))
		})
	}
}

// listRows draws a scrolled list: a header, then a viewport (clipped)
// of rows scrolled by off pixels, then a footer.
func listRows(t *testing.T, off float32, extra func(l *paint.List)) *paint.List {
	return listRowsOpt(t, off, false, extra)
}

// listRowsOpt is listRows; with rowClip every row's label is clipped to
// its row (clips that scroll with the content).
func listRowsOpt(t *testing.T, off float32, rowClip bool, extra func(l *paint.List)) *paint.List {
	l := &paint.List{Background: white}
	l.Fill(geom.R(0, 0, damageSize.W, 20), geom.Radii{}, color.NRGBA{0x30, 0x30, 0x30, 255})
	l.Clip(listViewport, geom.Radii{})
	l.Fill(listViewport, geom.Radii{}, color.NRGBA{0xfa, 0xfa, 0xfa, 255})
	for i := range 20 {
		y := listViewport.Y + float32(i)*20 - off
		if i%2 == 1 {
			l.Fill(geom.R(0, y, damageSize.W, 20), geom.Radii{}, color.NRGBA{0xe8, 0xf0, 0xff, 255})
		}
		if rowClip {
			l.Clip(geom.R(4, y+2, 120, 16), geom.Radii{})
		}
		l.Glyphs(geom.Pt(8, y+15), shape(t, fmt.Sprintf("第 %d 行 row", i), 12), ink)
		if rowClip {
			l.Pop()
		}
	}
	l.Pop()
	l.Fill(geom.R(0, 140, damageSize.W, 20), geom.Radii{}, color.NRGBA{0x30, 0x30, 0x30, 255})
	if extra != nil {
		extra(l)
	}
	return l
}

var listViewport = geom.R(0, 20, damageSize.W, 120)

// listSize is the frame of the list tests: the viewport is under half of
// it, so redrawing the whole viewport is still a partial frame.
var listSize = geom.Size{W: damageSize.W, H: 400}

func TestScrollRegionShiftsPixels(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			dev := &cpuDevice{}
			r := NewRenderer(dev)
			if err := r.Render(listRows(t, 0, nil), listSize, scale); err != nil {
				t.Fatal(err)
			}
			r.SetVerify(false)
			// Scrolled 40 px: content moves up; the bottom 40 px are new.
			l := listRows(t, 40, func(l *paint.List) {
				l.Scroll(listViewport, geom.Pt(0, -40))
				l.Damage(geom.R(0, listViewport.Bottom()-40, damageSize.W, 40))
			})
			if err := r.Render(l, listSize, scale); err != nil {
				t.Fatal(err)
			}
			f := dev.last
			vp := r.outBounds(listViewport, 0)
			d := int(40 * scale)
			wantCopy := CopyRegion{Src: image.Rect(vp.Min.X, vp.Min.Y+d, vp.Max.X, vp.Max.Y), Dst: image.Rect(vp.Min.X, vp.Min.Y, vp.Max.X, vp.Max.Y-d)}
			if len(f.Copies) != 1 || f.Copies[0] != wantCopy {
				t.Fatalf("Copies %v, want [%v]", f.Copies, wantCopy)
			}
			strip := image.Rect(vp.Min.X, vp.Max.Y-d, vp.Max.X, vp.Max.Y)
			if len(f.Dirty) != 1 || f.Dirty[0] != strip {
				t.Fatalf("Dirty %v, want [%v] (only the exposed strip)", f.Dirty, strip)
			}
			for _, b := range targetInstanceBounds(f) {
				if !b.Overlaps(strip.Inset(-1)) { // culled by bounds padded 1 px
					t.Fatalf("instance %v outside the exposed strip %v", b, strip)
				}
			}
			mustSame(t, "scrolled", dev.img, fullRender(t, l, listSize, scale))

			// Verify mode agrees.
			r.SetVerify(true)
			l2 := listRows(t, 20, func(l *paint.List) {
				l.Scroll(listViewport, geom.Pt(0, 20))
				l.Damage(geom.R(0, listViewport.Y, damageSize.W, 20))
			})
			if err := r.Render(l2, listSize, scale); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestScrollRegionUnsafeFallsBack(t *testing.T) {
	for _, tc := range []struct {
		name  string
		extra func(l *paint.List)
		off   float32
	}{
		{"crosses clip", func(l *paint.List) {
			// The region reaches into the footer, below the viewport's
			// clip: the clip edge would be copied along.
			l.Scroll(geom.R(0, 20, damageSize.W, 140), geom.Pt(0, -40))
			l.Damage(geom.R(0, 100, damageSize.W, 40))
		}, 40},
		{"inside clip partly", func(l *paint.List) {
			l.Clip(geom.R(0, 20, damageSize.W, 60), geom.Radii{})
			l.Scroll(listViewport, geom.Pt(0, -40))
			l.Pop()
			l.Damage(geom.R(0, 100, damageSize.W, 40))
		}, 40},
		{"inside opacity", func(l *paint.List) {
			l.Opacity(0.5)
			l.Scroll(listViewport, geom.Pt(0, -40))
			l.Pop()
			l.Damage(geom.R(0, 100, damageSize.W, 40))
		}, 40},
		{"overlaps opacity group", func(l *paint.List) {
			l.Opacity(0.5)
			l.Fill(geom.R(100, 120, 60, 40), geom.Radii{}, red) // over the region's edge
			l.Pop()
			l.Scroll(listViewport, geom.Pt(0, -40))
			l.Damage(geom.R(0, 100, damageSize.W, 40))
		}, 40},
		{"overlapping scrolls", func(l *paint.List) {
			l.Scroll(listViewport, geom.Pt(0, -40))
			l.Scroll(geom.R(0, 60, damageSize.W, 40), geom.Pt(0, -20))
			l.Damage(geom.R(0, 100, damageSize.W, 40))
		}, 40},
		{"fractional delta", func(l *paint.List) {
			l.Scroll(listViewport, geom.Pt(0, -10.3))
			l.Damage(geom.R(0, 100, damageSize.W, 40))
		}, 10.3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dev := &cpuDevice{}
			r := NewRenderer(dev)
			if err := r.Render(listRows(t, 0, nil), listSize, 1); err != nil {
				t.Fatal(err)
			}
			r.SetVerify(false)
			l := listRows(t, tc.off, tc.extra)
			if tc.name == "overlaps opacity group" {
				// Frame 1 must have had the group too.
				l0 := listRows(t, 0, func(l *paint.List) {
					l.Opacity(0.5)
					l.Fill(geom.R(100, 120, 60, 40), geom.Radii{}, red)
					l.Pop()
				})
				if err := r.Render(l0, listSize, 1); err != nil {
					t.Fatal(err)
				}
			}
			if err := r.Render(l, listSize, 1); err != nil {
				t.Fatal(err)
			}
			f := dev.last
			if len(f.Copies) != 0 {
				t.Fatalf("unsafe scroll region copied: %v", f.Copies)
			}
			vp := r.outBounds(listViewport, 0)
			if len(f.Dirty) == 0 || !vp.In(unionRects(f.Dirty)) {
				t.Fatalf("Dirty %v does not redraw the region %v", f.Dirty, vp)
			}
			mustSame(t, tc.name, dev.img, fullRender(t, l, listSize, 1))
		})
	}
}

func unionRects(rs []image.Rectangle) image.Rectangle {
	var u image.Rectangle
	for _, r := range rs {
		u = u.Union(r)
	}
	return u
}

func TestVerifyModeCatchesWrongDamage(t *testing.T) {
	dev := &cpuDevice{}
	r := NewRenderer(dev)
	if !r.verify && verifyDefault {
		t.Fatal("verify mode off by default in a development build")
	}
	r.SetVerify(true)
	cs := [4]color.NRGBA{blue, blue, blue, blue}
	if err := r.Render(buttonsList(t, cs), damageSize, 1); err != nil {
		t.Fatal(err)
	}
	cs[3] = red
	l := buttonsList(t, cs)
	l.Damage(buttonRect(0)) // button 3 changed, button 0 is declared
	err := r.Render(l, damageSize, 1)
	if err == nil || !strings.Contains(err.Error(), "differs") {
		t.Fatalf("verify mode: err = %v, want a difference", err)
	}
	b3 := r.outBounds(buttonRect(3), 0)
	if !strings.Contains(err.Error(), "(") || !strings.Contains(err.Error(), fmt.Sprint(b3.Min.Y)) {
		t.Logf("error: %v", err)
	}
	// The target holds the full frame now; the next frame is full (the
	// last one errored).
	if err := r.Render(l, damageSize, 1); err != nil {
		t.Fatal(err)
	}
	if len(dev.last.Dirty) != 0 {
		t.Fatalf("frame after a verify error is partial: %v", dev.last.Dirty)
	}

	// Without verify mode the wrong hint goes unnoticed: stale pixels.
	r.SetVerify(false)
	cs[3] = green
	l = buttonsList(t, cs)
	l.Damage(buttonRect(0))
	if err := r.Render(l, damageSize, 1); err != nil {
		t.Fatal(err)
	}
	if res := compare(dev.img, fullRender(t, l, damageSize, 1), 0); res.Over == 0 {
		t.Fatal("wrong damage without verify mode redrew button 3 anyway")
	}
}

func TestNoHintsFullRedraw(t *testing.T) {
	cs := [4]color.NRGBA{blue, blue, blue, blue}
	hinted := func() *paint.List {
		l := buttonsList(t, cs)
		l.Damage(buttonRect(1))
		return l
	}
	dev := &cpuDevice{}
	r := NewRenderer(dev)
	r.SetVerify(false) // dev.last is the partial frame, not verify's full one
	partial := func(l *paint.List, size geom.Size, scale float32) bool {
		t.Helper()
		if err := r.Render(l, size, scale); err != nil {
			t.Fatal(err)
		}
		return len(dev.last.Dirty) > 0
	}
	if partial(hinted(), damageSize, 1) {
		t.Fatal("first frame partial")
	}
	if partial(buttonsList(t, cs), damageSize, 1) {
		t.Fatal("frame without hints partial")
	}
	if !partial(hinted(), damageSize, 1) {
		t.Fatal("hinted frame not partial")
	}
	if partial(hinted(), damageSize, 2) {
		t.Fatal("scale change partial")
	}
	if partial(hinted(), geom.Size{W: 200, H: 160}, 2) {
		t.Fatal("size change partial")
	}
	l := hinted()
	l.Background = red
	if partial(l, geom.Size{W: 200, H: 160}, 2) {
		t.Fatal("background change partial")
	}
	// Damage over more than half the frame.
	l = buttonsList(t, cs)
	l.Damage(geom.R(0, 0, damageSize.W, damageSize.H*0.6))
	if partial(l, damageSize, 1) || partial(l, damageSize, 1) {
		t.Fatal("damage over 60% of the frame partial")
	}
	// A Damage under an unknown Push… op (whose effect on coordinates the
	// Renderer cannot know): full.
	l = buttonsList(t, cs)
	l.Add(PushFuture{})
	l.Damage(buttonRect(1))
	l.Pop()
	if partial(l, damageSize, 1) {
		t.Fatal("damage under an unknown push partial")
	}
	// A device that does not retain its target.
	dev.noRetain = true
	if partial(hinted(), damageSize, 1) {
		t.Fatal("partial frame on a device that does not retain")
	}
	dev.noRetain = false
	if !partial(hinted(), damageSize, 1) {
		t.Fatal("hinted frame not partial")
	}

	// A failed frame: the next is full.
	fd := &flakyDevice{cpuDevice: cpuDevice{}}
	r2 := NewRenderer(fd)
	r2.SetVerify(false)
	if err := r2.Render(hinted(), damageSize, 1); err != nil {
		t.Fatal(err)
	}
	fd.fail = true
	if err := r2.Render(hinted(), damageSize, 1); err == nil {
		t.Fatal("no error")
	}
	fd.fail = false
	if err := r2.Render(hinted(), damageSize, 1); err != nil {
		t.Fatal(err)
	}
	if len(fd.last.Dirty) != 0 {
		t.Fatal("frame after a failed one is partial")
	}

	// Hints that leave nothing to redraw: nothing is executed (but verify
	// mode's full frame).
	l = buttonsList(t, cs)
	l.Damage(geom.R(-50, -50, 10, 10))
	for _, verify := range []bool{false, true} {
		r.SetVerify(verify)
		n := dev.executed
		if err := r.Render(l, damageSize, 1); err != nil {
			t.Fatal(err)
		}
		want := 0
		if verify {
			want = 1
		}
		if dev.executed-n != want {
			t.Fatalf("verify %v: %d frames executed, want %d", verify, dev.executed-n, want)
		}
	}
}

// flakyDevice is a cpuDevice whose Execute fails while fail is set.
type flakyDevice struct {
	cpuDevice
	fail bool
}

func (d *flakyDevice) Execute(f *Frame) error {
	if d.fail {
		return fmt.Errorf("flaky")
	}
	return d.cpuDevice.Execute(f)
}

func TestDamageUnderTransform(t *testing.T) {
	inner := geom.R(10, 6, 30, 14)
	for i, tc := range []struct {
		name string
		m    geom.Affine
	}{
		{"scale1.2", geom.Translate(40, 30).Mul(geom.Scale(1.2, 1.2))},
		{"rotate30", geom.Translate(120, 40).Mul(geom.Rotate(math.Pi / 6))},
	} {
		for j, scale := range []float32{1, 2} {
			t.Run(fmt.Sprintf("%s@%v", tc.name, scale), func(t *testing.T) {
				raceSkip(t, i == j) // scale1.2@1, rotate30@2
				mk := func(c color.NRGBA, hint bool) *paint.List {
					l := &paint.List{Background: white}
					l.Fill(geom.R(0, 0, damageSize.W, damageSize.H), geom.Radii{}, color.NRGBA{0xf4, 0xf4, 0xf4, 255})
					l.Transform(tc.m)
					l.Fill(geom.R(0, 0, 60, 30), geom.Rad(4), gray)
					l.Fill(inner, geom.Rad(3), c)
					if hint {
						l.Damage(inner)
					}
					l.Pop()
					return l
				}
				dev := &cpuDevice{}
				r := NewRenderer(dev)
				if err := r.Render(mk(blue, false), damageSize, scale); err != nil {
					t.Fatal(err)
				}
				l := mk(red, true)
				if err := r.Render(l, damageSize, scale); err != nil {
					t.Fatal(err) // verify mode: the redrawn box covers the change
				}
				r.SetVerify(false)
				if err := r.Render(l, damageSize, scale); err != nil {
					t.Fatal(err)
				}
				pad := 0
				if tc.m.B != 0 {
					pad = 1
				}
				g := tc.m.Bounds(inner)
				s := float64(scale)
				want := image.Rect(int(math.Floor(float64(g.X)*s))-pad, int(math.Floor(float64(g.Y)*s))-pad,
					int(math.Ceil(float64(g.Right())*s))+pad, int(math.Ceil(float64(g.Bottom())*s))+pad)
				if f := dev.last; len(f.Dirty) != 1 || f.Dirty[0] != want {
					t.Fatalf("Dirty %v, want [%v] (device bounds of the transformed rect)", f.Dirty, want)
				}
				// Without verify mode the rotated subtree's patch is
				// rasterized over the dirty region only: within 2 levels.
				if res := compare(dev.img, fullRender(t, l, damageSize, scale), 2); res.Over > 0 {
					t.Fatalf("transformed: %+v", res)
				}
			})
		}
	}
}

var gray = color.NRGBA{0x80, 0x80, 0x80, 255}

func TestMergeDirty(t *testing.T) {
	var rs []image.Rectangle
	for i := range 20 {
		rs = append(rs, image.Rect(i*10, (i%3)*10, i*10+5, (i%3)*10+5))
	}
	rs = append(rs, image.Rect(2, 2, 8, 8), image.Rectangle{}) // overlaps the first; empty
	out := mergeDirty(rs, maxDirty)
	if len(out) > maxDirty {
		t.Fatalf("%d rects", len(out))
	}
	for i, a := range out {
		for _, b := range out[i+1:] {
			if a.Overlaps(b) {
				t.Fatalf("%v and %v overlap", a, b)
			}
		}
	}
	for _, r := range rs[:21] {
		covered := false
		for _, o := range out {
			if r.In(o) {
				covered = true
			}
		}
		if !covered {
			t.Fatalf("%v not covered by %v", r, out)
		}
	}
}

// TestPartialRedrawScenes redraws every parity scene partially (one
// damaged rectangle across the middle) after a frame of a different
// background content, with verify mode on: layers, patches, gradients,
// images and text in a partial frame equal the full frame.
func TestPartialRedrawScenes(t *testing.T) {
	for i, sc := range scenes {
		for j, scale := range []float32{1, 2} {
			t.Run(fmt.Sprintf("%s@%v", sc.Name, scale), func(t *testing.T) {
				raceSkip(t, j == i%2) // every scene, at one scale each
				l := sc.Make(t)
				dev := &cpuDevice{}
				r := NewRenderer(dev)
				if err := r.Render(l, gputest.SceneSize, scale); err != nil {
					t.Fatal(err)
				}
				// Same content, two damaged rectangles: the result must not
				// change, and verify mode compares it with the full frame.
				l2 := &paint.List{Background: l.Background, Ops: append(append([]paint.Op(nil), l.Ops...),
					paint.Damage{Rect: geom.R(5, 20, 50, 12)}, paint.Damage{Rect: geom.R(60, 3, 20, 50)})}
				if err := r.Render(l2, gputest.SceneSize, scale); err != nil {
					t.Fatal(err)
				}
				r.SetVerify(false)
				if err := r.Render(l2, gputest.SceneSize, scale); err != nil {
					t.Fatal(err)
				}
				if len(dev.last.Dirty) != 2 {
					t.Fatalf("Dirty %v", dev.last.Dirty)
				}
				mustSame(t, sc.Name, dev.img, fullRender(t, l, gputest.SceneSize, scale))
			})
		}
	}
}

// TestScrollRegionMovedClips: clips of rows that scroll with the content
// (their edges inside the region match the previous frame's shifted by the
// delta) keep the copy; the result equals the full frame.
func TestScrollRegionMovedClips(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			raceSkip(t, scale == 2)
			dev := &cpuDevice{}
			r := NewRenderer(dev)
			if err := r.Render(listRowsOpt(t, 0, true, nil), listSize, scale); err != nil {
				t.Fatal(err)
			}
			offs := []float32{0, 30, 10, 50}
			for k := 1; k < len(offs); k++ {
				d := offs[k-1] - offs[k]
				l := listRowsOpt(t, offs[k], true, func(l *paint.List) {
					l.Scroll(listViewport, geom.Pt(0, d))
				})
				r.SetVerify(true)
				if err := r.Render(l, listSize, scale); err != nil {
					t.Fatal(err)
				}
				r.SetVerify(false)
				// Again from the same previous frame, to inspect the frame.
				prevL := listRowsOpt(t, offs[k-1], true, nil)
				if err := r.Render(prevL, listSize, scale); err != nil {
					t.Fatal(err)
				}
				if err := r.Render(l, listSize, scale); err != nil {
					t.Fatal(err)
				}
				if len(dev.last.Copies) != 1 {
					t.Fatalf("scroll %v: Copies %v, Dirty %v; row clips should move along", d, dev.last.Copies, dev.last.Dirty)
				}
				mustSame(t, "scrolled", dev.img, fullRender(t, l, listSize, scale))
			}
		})
	}
}

// TestScrollRegionSubpixelShift: a shift that rounds to no device pixels
// (the tail of an inertial scroll) makes no copy; the region is redrawn
// and the frame stays correct, with and without verify mode, whether the
// hint comes from List.Scroll or is appended directly.
func TestScrollRegionSubpixelShift(t *testing.T) {
	for _, verify := range []bool{false, true} {
		for _, direct := range []bool{false, true} {
			t.Run(fmt.Sprintf("verify=%v,direct=%v", verify, direct), func(t *testing.T) {
				dev := &cpuDevice{}
				r := NewRenderer(dev)
				r.SetVerify(verify)
				if err := r.Render(listRows(t, 0, nil), listSize, 1); err != nil {
					t.Fatal(err)
				}
				l := listRows(t, 0, func(l *paint.List) {
					if direct {
						l.Add(paint.ScrollRegion{Rect: listViewport, Delta: geom.Pt(0, 0.0004)})
					} else {
						l.Scroll(listViewport, geom.Pt(0, 0.0004))
					}
				})
				if err := r.Render(l, listSize, 1); err != nil {
					t.Fatal(err)
				}
				if len(dev.last.Copies) != 0 {
					t.Fatalf("sub-pixel shift copied %v", dev.last.Copies)
				}
				if !verify {
					vp := r.outBounds(listViewport, 0)
					if len(dev.last.Dirty) != 1 || dev.last.Dirty[0] != vp {
						t.Fatalf("Dirty %v, want [%v]", dev.last.Dirty, vp)
					}
				}
				mustSame(t, "sub-pixel", dev.img, fullRender(t, l, listSize, 1))
			})
		}
	}
}

// TestCopiesWithoutDirtyRejected: the test device refuses a frame with
// Copies but no Dirty (it would be executed as a full frame).
func TestCopiesWithoutDirtyRejected(t *testing.T) {
	dev := &cpuDevice{}
	f := &Frame{Size: image.Pt(8, 8), Copies: []CopyRegion{{Src: image.Rect(0, 0, 2, 2), Dst: image.Rect(2, 2, 4, 4)}}}
	if err := dev.Execute(f); err == nil {
		t.Fatal("copies in a full frame accepted")
	}
}

// fullRowClips is a list whose every row is clipped to exactly its own
// row rectangle (how list components clip), scrolled by off.
func fullRowClips(t *testing.T, off float32, extra func(*paint.List)) *paint.List {
	l := &paint.List{Background: white}
	l.Clip(listViewport, geom.Radii{})
	l.Fill(listViewport, geom.Radii{}, color.NRGBA{0xfa, 0xfa, 0xfa, 255})
	for i := range 20 {
		row := geom.R(0, listViewport.Y+float32(i)*20-off, damageSize.W, 20)
		l.Clip(row, geom.Radii{})
		if i%2 == 1 {
			l.Fill(row, geom.Radii{}, color.NRGBA{0xe8, 0xf0, 0xff, 255})
		}
		l.Glyphs(geom.Pt(8, row.Y+15), shape(t, fmt.Sprintf("第 %d 行 row", i), 12), ink)
		l.Pop()
	}
	l.Pop()
	if extra != nil {
		extra(l)
	}
	return l
}

// TestScrollRegionFullRowClips: rows clipped to exactly their own
// rectangles, the previous frame aligned with the viewport, scrolled by
// whole rows and by part of one: the copy is kept and the result equals
// the full frame.
func TestScrollRegionFullRowClips(t *testing.T) {
	for j, scale := range []float32{1, 2} {
		for k, step := range []float32{20, 40, 30, -20} {
			t.Run(fmt.Sprintf("%v@%v", step, scale), func(t *testing.T) {
				raceSkip(t, k == 2*j) // whole rows at 1, part of a row at 2
				dev := &cpuDevice{}
				r := NewRenderer(dev)
				r.SetVerify(false)
				from := float32(60)
				if err := r.Render(fullRowClips(t, from, nil), listSize, scale); err != nil {
					t.Fatal(err)
				}
				l := fullRowClips(t, from+step, func(l *paint.List) { l.Scroll(listViewport, geom.Pt(0, -step)) })
				if err := r.Render(l, listSize, scale); err != nil {
					t.Fatal(err)
				}
				if len(dev.last.Copies) != 1 {
					t.Fatalf("Copies %v, Dirty %v: full-height row clips should scroll along", dev.last.Copies, dev.last.Dirty)
				}
				mustSame(t, "row clips", dev.img, fullRender(t, l, listSize, scale))
				// And verify mode agrees (from the same previous frame).
				r.SetVerify(true)
				if err := r.Render(fullRowClips(t, from, nil), listSize, scale); err != nil {
					t.Fatal(err)
				}
				if err := r.Render(l, listSize, scale); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

// TestPatchRasterExtent: in a partial frame a patch (a rotated subtree)
// is rasterized over its full-frame region in verify mode (so the partial
// frame equals the full one exactly) and only over its part of the dirty
// region without (release builds); the latter is within the patch
// threshold of the full frame.
func TestPatchRasterExtent(t *testing.T) {
	m := geom.Translate(120, 20).Mul(geom.Rotate(math.Pi / 7))
	mk := func(c color.NRGBA) *paint.List {
		l := &paint.List{Background: white}
		l.Transform(m)
		l.Fill(geom.R(0, 0, 90, 60), geom.Rad(6), gray)
		l.Fill(geom.R(4, 4, 12, 12), geom.Radii{}, c)
		l.Pop()
		return l
	}
	for _, verify := range []bool{true, false} {
		t.Run(fmt.Sprint(verify), func(t *testing.T) {
			dev := &cpuDevice{}
			r := NewRenderer(dev)
			r.SetVerify(verify)
			if err := r.Render(mk(blue), damageSize, 1); err != nil {
				t.Fatal(err)
			}
			var full image.Rectangle
			r.onRaster = func(g image.Rectangle) { full = full.Union(g) }
			if err := r.Render(mk(blue), damageSize, 1); err != nil {
				t.Fatal(err)
			}
			fullArea := full.Dx() * full.Dy()
			l := mk(red)
			l.Transform(m)
			l.Damage(geom.R(4, 4, 12, 12))
			l.Pop()
			var rasters []image.Rectangle
			r.onRaster = func(g image.Rectangle) { rasters = append(rasters, g) }
			if err := r.Render(l, damageSize, 1); err != nil {
				t.Fatal(err)
			}
			if len(rasters) == 0 {
				t.Fatal("no patch")
			}
			partialRaster := rasters[0] // the partial frame's (verify's full frame follows)
			if verify {
				if partialRaster != full {
					t.Fatalf("verify mode rasterized %v, want the full-frame %v", partialRaster, full)
				}
				return
			}
			if a := partialRaster.Dx() * partialRaster.Dy(); !(a*4 < fullArea) || !partialRaster.In(dev.last.Dirty[0]) {
				t.Fatalf("release rasterized %v (full frame %v, dirty %v)", partialRaster, full, dev.last.Dirty)
			}
			if res := compare(dev.img, fullRender(t, l, damageSize, 1), 2); res.Over > 0 {
				t.Fatalf("partial patch frame differs beyond 2: %+v", res)
			}
		})
	}
}
