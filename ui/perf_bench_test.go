package ui

import (
	"strings"
	"testing"
	"time"
)

// perfScene is one timed scenario of a fixture. Setup prepares it outside
// the timed region and returns the step to repeat (i counts steps from 0)
// and the driver the last step ran on.
type perfScene struct {
	Name  string
	Setup func() (step func(i int), last func() *Driver)
}

var perfLargeSizes = []struct {
	name string
	n    int
}{{"Large1k", 1000}, {"Large10k", 10000}, {"Large100k", 100000}}

// perfScenes lists fx's scenes. With raster the CPU rasterizer runs too
// (reference numbers only).
func perfScenes(fx perfFixture, raster bool) []perfScene {
	mount := func(warm bool) func() (func(int), func() *Driver) {
		return func() (func(int), func() *Driver) {
			var d *Driver
			if warm { // glyphs, shaping, theme and icon caches warm
				d = newPerfDriver(perfPage(fx.Build), raster)
				d.Frame()
			}
			return func(int) {
				d = newPerfDriver(perfPage(fx.Build), raster)
				d.Frame()
			}, func() *Driver { return d }
		}
	}
	interact := func(page func(*Context), act func(*perfRun, int)) func() (func(int), func() *Driver) {
		return func() (func(int), func() *Driver) {
			d := newPerfDriver(page, raster)
			d.Settle()
			r := &perfRun{D: d}
			return func(i int) {
				d.Advance(16 * time.Millisecond)
				perfStep(r, act, i)
			}, func() *Driver { return d }
		}
	}
	sc := []perfScene{
		{"Mount", mount(true)},
		{"MountCold", mount(false)}, // meaningful only as the first run of a fresh process (-benchtime 1x)
		{"Unchanged", func() (func(int), func() *Driver) {
			d := newPerfDriver(perfPage(fx.Build), raster)
			d.Settle()
			return func(int) { d.Frame() }, func() *Driver { return d }
		}},
	}
	if fx.Interact != nil {
		sc = append(sc, perfScene{"Interact", interact(perfPage(fx.Build), fx.Interact)})
	}
	if fx.Large != nil {
		act := fx.LargeInteract
		if act == nil {
			act = perfScroll
		}
		for _, l := range perfLargeSizes {
			sc = append(sc, perfScene{l.name, interact(perfPage(fx.Large(l.n)), act)})
		}
	}
	if fx.LargeVirtual != nil {
		act := fx.LargeInteract
		if act == nil {
			act = perfScroll
		}
		for _, l := range perfLargeSizes {
			sc = append(sc, perfScene{"LargeVirtual" + strings.TrimPrefix(l.name, "Large"), interact(perfPage(fx.LargeVirtual(l.n)), act)})
		}
	}
	if fx.LargeVersioned != nil {
		act := fx.LargeInteract
		if act == nil {
			act = perfScroll
		}
		for _, l := range perfLargeSizes {
			sc = append(sc, perfScene{"LargeVersioned" + strings.TrimPrefix(l.name, "Large"), interact(perfPage(fx.LargeVersioned(l.n)), act)})
		}
	}
	return sc
}

func benchPerfScene(b *testing.B, sc perfScene) {
	b.ReportAllocs()
	step, last := sc.Setup()
	i := 0
	for b.Loop() {
		step(i)
		i++
	}
	if d := last(); d != nil {
		sz := d.r.win.Size()
		b.ReportMetric(float64(perfDirtyArea(d)/(sz.W*sz.H)), "dirty/op")
	}
}

// BenchmarkComponents times every fixture's scenes without rasterizing:
// build, layout, display list and damage (what a GPU host pays on the
// CPU). go test ./ui -run '^$' -bench 'BenchmarkComponents/Button'
func BenchmarkComponents(b *testing.B) {
	for _, fx := range perfFixtures() {
		if fx.Skip != "" {
			continue
		}
		b.Run(fx.Name, func(b *testing.B) {
			for _, sc := range perfScenes(fx, false) {
				b.Run(sc.Name, func(b *testing.B) { benchPerfScene(b, sc) })
			}
		})
	}
}

// BenchmarkComponentsRaster times the Interact scene with the CPU
// rasterizer, for reference only.
func BenchmarkComponentsRaster(b *testing.B) {
	for _, fx := range perfFixtures() {
		if fx.Skip != "" || fx.Interact == nil {
			continue
		}
		b.Run(fx.Name, func(b *testing.B) {
			for _, sc := range perfScenes(fx, true) {
				if sc.Name == "Interact" {
					b.Run(sc.Name, func(b *testing.B) { benchPerfScene(b, sc) })
				}
			}
		})
	}
}
