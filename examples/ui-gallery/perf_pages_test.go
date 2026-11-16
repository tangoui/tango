package main

import (
	"fmt"
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"os"
	"testing"
	"time"
)

// galleryTimeDebt lists pages whose Interact frame is over 4ms. Empty:
// the Task 12a baseline (5f115d3, 1-min load < 6) measured every
// non-variant page within budget.
var galleryTimeDebt = map[string]string{}

type galleryPerfWin struct{ hintWin }

func (galleryPerfWin) Scale() float32 { return 2 }
func newGalleryRunner(p gallery.Page) (*ui.Runner, *time.Time) {
	dash, ov = newDash(), newOverlayState()
	now := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
	r := ui.NewRunner(func(c *ui.Context) { p.Render(c) }, ui.RunnerOptions{Renderer: nopRenderer{}, NoAnimations: true, Clock: func() time.Time { return now }})
	r.Attach(&galleryPerfWin{hintWin{size: geom.Size{W: 1440, H: 900}}})
	for range 3 {
		r.Frame()
	}
	return r, &now
}
func galleryInteract(r *ui.Runner, i int) {
	r.HandleEvent(input.Scroll{Pos: geom.Pt(720, 450), DY: func() float32 {
		if i%40 < 20 {
			return 120
		}
		return -120
	}()})
	r.Frame()
}
func BenchmarkGalleryPages(b *testing.B) {
	for _, p := range gallery.Pages() {
		if gallery.IsVariant(p) {
			continue
		}
		b.Run(p.Name+"/Unchanged", func(b *testing.B) {
			r, _ := newGalleryRunner(p)
			b.ResetTimer()
			for b.Loop() {
				r.Frame()
			}
		})
		b.Run(p.Name+"/Interact", func(b *testing.B) {
			r, _ := newGalleryRunner(p)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				galleryInteract(r, i)
			}
		})
	}
}
func TestGalleryPageBudgets(t *testing.T) {
	if os.Getenv("TANGO_PERF_BUDGET") != "1" {
		t.Skip("set TANGO_PERF_BUDGET=1")
	}
	for _, p := range gallery.Pages() {
		if gallery.IsVariant(p) {
			continue
		}
		r, _ := newGalleryRunner(p)
		ns := galleryTime(r)
		key := p.Name + "/Interact"
		over := ns > 4*time.Millisecond
		d, indebt := galleryTimeDebt[key]
		if over && !indebt {
			t.Errorf("%s: %v over 4ms; add to galleryTimeDebt", key, ns)
		}
		if !over && indebt {
			t.Errorf("%s: %v within budget (debt %s); remove debt", key, ns, d)
		}
		if over {
			t.Logf("GALLERY %s %v %s", key, ns, fmt.Sprint(d))
		}
	}
}

// galleryTime times a page's Interact frame: up to 20 warm-up frames that
// fit in 200 ms (at least one), then at least 3 timed frames, continuing
// until 50 frames or 1 s of timed work, like perfTime in package ui. Timed
// frames restart at index 0, as the fixed 20 + 50 loop did, so the scroll
// pattern (20 down, 20 up, …) is unchanged. Precondition: the warm-up
// reaches the full 20 frames only when a frame takes less than about 10 ms
// (20 × 10 ms = the 200 ms window); slower pages get fewer warm-up frames,
// down to one.
func galleryTime(r *ui.Runner) time.Duration {
	start := time.Now()
	for i := 0; i < 20 && (i == 0 || time.Since(start) < 200*time.Millisecond); i++ {
		galleryInteract(r, i)
	}
	start = time.Now()
	n := 0
	var el time.Duration
	for n < 3 || (n < 50 && el < time.Second) {
		galleryInteract(r, n)
		n++
		el = time.Since(start)
	}
	return el / time.Duration(n)
}
