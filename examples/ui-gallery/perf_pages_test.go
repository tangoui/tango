package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"os"
	"testing"
	"time"
)

var galleryTimeDebt = map[string]string{}

func galleryBenchPage(p gallery.Page, interact bool) func(int) {
	return func(i int) {
		_ = i
	}
}
func BenchmarkGalleryPages(b *testing.B) {
	for _, p := range gallery.Pages() {
		if gallery.IsVariant(p) {
			continue
		}
		b.Run(p.Name+"/Unchanged", func(b *testing.B) {
			dash, ov = newDash(), newOverlayState()
			now := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
			r := ui.NewRunner(func(c *ui.Context) { p.Render(c) }, ui.RunnerOptions{Renderer: nopRenderer{}, NoAnimations: true, Clock: func() time.Time { return now }})
			r.Attach(&hintWin{size: geom.Size{W: 1440, H: 900}})
			for range 3 {
				r.Frame()
			}
			b.ResetTimer()
			for b.Loop() {
				r.Frame()
			}
		})
		b.Run(p.Name+"/Interact", func(b *testing.B) {
			dash, ov = newDash(), newOverlayState()
			now := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
			r := ui.NewRunner(func(c *ui.Context) { p.Render(c) }, ui.RunnerOptions{Renderer: nopRenderer{}, NoAnimations: true, Clock: func() time.Time { return now }})
			r.Attach(&hintWin{size: geom.Size{W: 1440, H: 900}})
			for range 3 {
				r.Frame()
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				r.HandleEvent(input.Scroll{Pos: geom.Pt(720, 450), DY: func() float32 {
					if i%40 < 20 {
						return 120
					}
					return -120
				}()})
				r.Frame()
			}
		})
	}
}
func TestGalleryPageBudgets(t *testing.T) {
	if os.Getenv("TANGO_PERF_BUDGET") != "1" {
		t.Skip("set TANGO_PERF_BUDGET=1")
	}
	t.Log("galleryTimeDebt intentionally empty; Task 12 populates it at load < 6")
}
