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

var galleryTimeDebt = map[string]string{} // Task 12 populates this at load < 6.
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
		for i := 0; i < 20; i++ {
			galleryInteract(r, i)
		}
		start := time.Now()
		for i := 0; i < 50; i++ {
			galleryInteract(r, i)
		}
		ns := time.Since(start) / 50
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
