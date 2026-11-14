package gpu

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// gradientRows is a list of rows filled with linear gradients (dithered)
// inside the clipped viewport, scrolled by off.
func gradientRows(off float32, extra func(*paint.List)) *paint.List {
	l := &paint.List{Background: white}
	l.Clip(listViewport, geom.Radii{})
	for i := range 12 {
		y := listViewport.Y + float32(i)*20 - off
		l.LinearStops(geom.R(0, y, damageSize.W, 20), geom.Radii{}, geom.Pt(0, y), geom.Pt(damageSize.W, y+20),
			paint.Stop{Offset: 0, Color: color.NRGBA{0x20, 0x40, byte(0x60 + i*8), 255}},
			paint.Stop{Offset: 1, Color: color.NRGBA{0x30, 0x48, byte(0x66 + i*8), 255}})
	}
	l.Pop()
	if extra != nil {
		extra(l)
	}
	return l
}

// TestVerifyFractionalScroll: whole-device-pixel scrolls of content at
// fractional offsets, at fractional scales, and of dithered gradient rows
// pass verify mode (strict): copies may differ from a fresh render by one
// level (see verifyPartial), nothing else.
func TestVerifyFractionalScroll(t *testing.T) {
	type tc struct {
		name  string
		scale float32
		step  float32 // logical; step·scale is whole device pixels
		mk    func(off float32, extra func(*paint.List)) *paint.List
	}
	text := func(off float32, extra func(*paint.List)) *paint.List { return listRows(t, off, extra) }
	for _, c := range []tc{
		{"text@1", 1, 3, text},
		{"text@1.25", 1.25, 4, text},
		{"text@1.5", 1.5, 2, text},
		{"text@1.75", 1.75, 4, text},
		{"gradient@1", 1, 10, gradientRows},
		{"gradient@1.25", 1.25, 4, gradientRows},
		{"gradient@1.5", 1.5, 6, gradientRows},
	} {
		t.Run(c.name, func(t *testing.T) {
			raceSkip(t, c.name == "text@1.25" || c.name == "gradient@1.5")
			dev := &cpuDevice{}
			r := NewRenderer(dev)
			r.SetVerify(true)
			r.SetVerifyStrict(true)
			off := float32(3.3)
			if err := r.Render(c.mk(off, nil), listSize, c.scale); err != nil {
				t.Fatal(err)
			}
			copies := 0
			for k, d := range []float32{1, 2, -1, 3, -2, 1} {
				dy := d * c.step
				off += dy
				l := c.mk(off, func(l *paint.List) { l.Scroll(listViewport, geom.Pt(0, -dy)) })
				if err := r.Render(l, listSize, c.scale); err != nil {
					t.Fatalf("frame %d (scroll %v): %v", k, dy, err)
				}
				copies++
			}
			if n, _ := r.VerifyFailures(); n != 0 || copies == 0 {
				t.Fatalf("%d verify failures", n)
			}
		})
	}
}

// TestConsecutivePartialFrames: many partial frames in a row with verify
// mode off (copies of copies), fractional offsets and scales, then one
// comparison with a full frame: within one level inside the scrolled
// viewport, exact elsewhere.
func TestConsecutivePartialFrames(t *testing.T) {
	for i, c := range []struct {
		scale float32
		step  float32
		grad  bool
	}{{1.25, 4, false}, {1.5, 2, false}, {1.25, 4, true}, {1, 1, true}} {
		t.Run(fmt.Sprintf("%v,grad=%v", c.scale, c.grad), func(t *testing.T) {
			raceSkip(t, i == 1 || i == 2) // text at 1.5, gradients at 1.25
			mk := func(off float32, extra func(*paint.List)) *paint.List {
				if c.grad {
					return gradientRows(off, extra)
				}
				return listRows(t, off, extra)
			}
			dev := &cpuDevice{}
			r := NewRenderer(dev)
			r.SetVerify(false)
			off := float32(3.3)
			if err := r.Render(mk(off, nil), listSize, c.scale); err != nil {
				t.Fatal(err)
			}
			var last *paint.List
			partials := 0
			for k := range 24 {
				d := []float32{1, 2, -1, 3, 1, -2, 2, 1}[k%8] * c.step
				off += d
				last = mk(off, func(l *paint.List) { l.Scroll(listViewport, geom.Pt(0, -d)) })
				if err := r.Render(last, listSize, c.scale); err != nil {
					t.Fatal(err)
				}
				if len(dev.last.Copies) == 1 {
					partials++
				}
			}
			if partials < 20 {
				t.Fatalf("only %d of 24 frames copied", partials)
			}
			want := fullRender(t, last, listSize, c.scale)
			got := dev.img
			vp := r.outBounds(listViewport, 0)
			worst := 0
			for y := got.Rect.Min.Y; y < got.Rect.Max.Y; y++ {
				for x := got.Rect.Min.X; x < got.Rect.Max.X; x++ {
					i := got.PixOffset(x, y)
					px := 0
					for ch := range 4 {
						px = max(px, absInt(int(got.Pix[i+ch])-int(want.Pix[i+ch])))
					}
					lim := 0
					if image.Pt(x, y).In(vp) {
						lim = 1
					}
					if px > lim {
						t.Fatalf("pixel (%d,%d) differs by %d after %d partial frames (limit %d)", x, y, px, partials, lim)
					}
					worst = max(worst, px)
				}
			}
			t.Logf("max difference after %d partial frames: %d", partials, worst)
		})
	}
}

// TestVerifyNotStrictPresents: outside test binaries verify differences do
// not fail the frame (ui.Runner would not present it, although the target
// holds the correct full redraw): Render returns nil, the difference is
// counted and logged at most once every verifyLogEvery frames, and the
// target equals the full frame.
func TestVerifyNotStrictPresents(t *testing.T) {
	if !NewRenderer(&cpuDevice{}).verifyStrict {
		t.Fatal("verify mode not strict in a test binary")
	}
	dev := &cpuDevice{}
	r := NewRenderer(dev)
	var logs []string
	r.logf = func(format string, args ...any) { logs = append(logs, fmt.Sprintf(format, args...)) }
	r.SetVerify(true)
	r.SetVerifyStrict(false)
	cs := [4]color.NRGBA{blue, blue, blue, blue}
	if err := r.Render(buttonsList(t, cs), damageSize, 1); err != nil {
		t.Fatal(err)
	}
	for k := range 3 {
		cs[3] = []color.NRGBA{red, green, blue}[k]
		l := buttonsList(t, cs)
		l.Damage(buttonRect(0)) // misses button 3
		if err := r.Render(l, damageSize, 1); err != nil {
			t.Fatalf("frame %d: not strict, err = %v", k, err)
		}
		mustSame(t, "presented", dev.img, fullRender(t, l, damageSize, 1))
	}
	n, last := r.VerifyFailures()
	if n != 3 || last == nil || !last.Region.Overlaps(r.outBounds(buttonRect(3), 0)) {
		t.Fatalf("VerifyFailures %d, %v", n, last)
	}
	if len(logs) != 1 || !strings.Contains(logs[0], "differs") {
		t.Fatalf("logs %q, want one report", logs)
	}
	// The next frame after a difference may be partial again: the target
	// holds the full frame.
	l := buttonsList(t, cs)
	l.Damage(buttonRect(3))
	r.SetVerify(false)
	if err := r.Render(l, damageSize, 1); err != nil {
		t.Fatal(err)
	}
	if len(dev.last.Dirty) == 0 {
		t.Fatal("frame after a logged difference not partial")
	}
}
