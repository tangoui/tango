//go:build darwin

package editor

// Measurement-only breakdown of a keystroke frame in the 1 MB wrapped line
// (BenchmarkTypeLongLineWrap) for the editor M2 failed-gate follow-up
// (Task A): per keystroke, the CPU part of the frame, the GPU wait, the
// frame's batches/instances, the texture uploads (atlas pages and images),
// the damaged area of a partial frame, the minimap's rebuilt rows, line
// bitmaps and strips, allocations and garbage collections.
//
//	TANGO_EDITOR_BREAKDOWN=300 go test ./ui/editor -run '^TestLongLineKeystrokeBreakdown$' -v
//
// It runs the benchmark's keystrokes on the full-redraw host of the gate
// benchmark (newPerfHost) and on the partial-redraw host of an application
// (newPartialPerfHost) and prints the means, percentiles and the slowest
// keys with their attributes.

import (
	"context"
	"fmt"
	"image"
	"os"
	"runtime"
	"runtime/pprof"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
)

// statDevice wraps a device and records the last executed frame's
// commands, uploads and damage. Completion waits do not execute a frame.
type statDevice struct {
	gpu.Device
	batches, instances  int
	uploads, upBytes    int
	images, imageBytes  int
	dirtyArea, fullArea int
	copies              int
}

type retainStatDevice struct{ *statDevice }

func (r retainStatDevice) Retained(size image.Point) bool {
	return r.Device.(gpu.Retainer).Retained(size)
}

func (s *statDevice) Execute(f *gpu.Frame) error {
	s.batches, s.instances = len(f.Batches), 0
	s.uploads, s.upBytes = len(f.Uploads), 0
	for _, u := range f.Uploads {
		s.upBytes += len(u.Pix)
	}
	for _, b := range f.Batches {
		s.instances += len(b.Instances)
		s.uploads += len(b.Uploads)
		for _, u := range b.Uploads {
			s.upBytes += len(u.Pix)
		}
	}
	s.images, s.imageBytes = len(f.ImageUploads), 0
	for _, u := range f.ImageUploads {
		s.imageBytes += len(u.Pix)
	}
	s.fullArea = f.Size.X * f.Size.Y
	s.dirtyArea = s.fullArea
	if len(f.Dirty) > 0 {
		s.dirtyArea = 0
		for _, r := range f.Dirty {
			s.dirtyArea += r.Dx() * r.Dy()
		}
	}
	s.copies = len(f.Copies)
	return s.Device.Execute(f)
}

type keyStat struct {
	total, cpu, wait                time.Duration
	batches, instances              int
	upBytes, images, imageBytes     int
	dirty                           float64
	rebuilt, bitmaps, strips        uint64
	allocBytes, mallocs, gcs        uint64
	miniBytes, bracketBytes, copies int64
	declRects                       int     // Damage hints the frame's List declared
	declArea                        float64 // their summed area, of the window
	scrolls                         int     // ScrollRegion hints
}

func breakdownHost(tb testing.TB, partial bool) (*perfHost, *statDevice) {
	text := longWrapLine(1 << 20)
	st := NewState(text, "bundle.ts", theme.Base("vs"))
	st.Options.WordWrap = WrapOn
	perfApplyOffEnv(tb, st)
	h := &perfHost{st: st, w: &perfWindow{size: perfSize, scale: 2}}
	h.dev = &syncDevice{d: metalDevice(tb)}
	sd := &statDevice{Device: h.dev}
	var dev gpu.Device = sd
	if partial {
		sd.Device = retainDevice{h.dev}
		dev = retainStatDevice{sd}
	}
	ren := gpu.NewRenderer(dev)
	ren.SetVerify(false)
	h.r = ui.NewRunner(func(c *ui.Context) {
		v := View(c, st)
		if partial {
			v = v.Layout(layout.Grow(0), layout.H(perfSize.H-1))
		}
		h.vw = v
	}, ui.RunnerOptions{Renderer: ren, NoAnimations: true})
	h.r.Attach(h.w)
	h.frame(tb)
	h.focus(tb)
	st.MoveCaret(len(text)/2, false)
	st.RevealCaret()
	for range 5 {
		h.frame(tb)
	}
	return h, sd
}

func runBreakdown(t *testing.T, partial bool, keys int) []keyStat {
	h, sd := breakdownHost(t, partial)
	countWork = true
	defer func() { countWork = false }()
	out := make([]keyStat, 0, keys)
	var ms0, ms1 runtime.MemStats
	if path := os.Getenv("TANGO_EDITOR_BREAKDOWN_PROF"); path != "" && !partial {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			t.Fatal(err)
		}
		defer func() { pprof.StopCPUProfile(); f.Close() }()
	}
	for range keys {
		m := &h.vw.vs.mini
		r0, b0, s0 := m.rebuilt, m.charRendered, m.stripsComposed
		resetWork()
		runtime.ReadMemStats(&ms0)
		start := time.Now()
		var ferr error
		pprof.Do(context.Background(), pprof.Labels("key", strconv.Itoa(len(out))), func(context.Context) {
			h.typeKey("x")
			h.w.invalid.Store(false)
			ferr = h.r.Frame()
		})
		if ferr != nil {
			t.Fatal(ferr)
		}
		t1 := time.Now()
		k := keyStat{
			batches: sd.batches, instances: sd.instances,
			upBytes: sd.upBytes, images: sd.images, imageBytes: sd.imageBytes,
			dirty: float64(sd.dirtyArea) / float64(max(1, sd.fullArea)),
		}
		for _, op := range h.r.List().Ops {
			switch o := op.(type) {
			case paint.Damage:
				k.declRects++
				k.declArea += float64(o.Rect.W*o.Rect.H) / float64(perfSize.W*perfSize.H)
			case paint.ScrollRegion:
				k.scrolls++
			}
		}
		if err := h.dev.wait(); err != nil {
			t.Fatal(err)
		}
		end := time.Now()
		runtime.ReadMemStats(&ms1)
		m = &h.vw.vs.mini
		k.total, k.cpu, k.wait = end.Sub(start), t1.Sub(start), end.Sub(t1)
		k.rebuilt, k.bitmaps, k.strips = m.rebuilt-r0, m.charRendered-b0, m.stripsComposed-s0
		k.allocBytes, k.mallocs, k.gcs = ms1.TotalAlloc-ms0.TotalAlloc, ms1.Mallocs-ms0.Mallocs, uint64(ms1.NumGC-ms0.NumGC)
		k.miniBytes, k.bracketBytes, k.copies = workMiniBytes.Load(), workBracketBytes.Load(), workLineCopies.Load()
		out = append(out, k)
	}
	return out
}

func pctOf[T any](ks []keyStat, f func(keyStat) T, less func(a, b T) int, p float64) T {
	v := make([]T, len(ks))
	for i, k := range ks {
		v[i] = f(k)
	}
	slices.SortFunc(v, less)
	return v[int(float64(len(v)-1)*p)]
}

func meanOf(ks []keyStat, f func(keyStat) float64) float64 {
	s := 0.0
	for _, k := range ks {
		s += f(k)
	}
	return s / float64(max(1, len(ks)))
}

func reportBreakdown(t *testing.T, name string, ks []keyStat) {
	cmpD := func(a, b time.Duration) int { return int(a - b) }
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s: %d keys, load %s\n", name, len(ks), loadAvg())
	for _, c := range []struct {
		n string
		f func(keyStat) time.Duration
	}{{"total", func(k keyStat) time.Duration { return k.total }}, {"cpu", func(k keyStat) time.Duration { return k.cpu }}, {"wait", func(k keyStat) time.Duration { return k.wait }}} {
		fmt.Fprintf(&sb, "  %-5s p50 %6.2f p95 %6.2f max %6.2f mean %6.2f ms\n", c.n,
			ms(pctOf(ks, c.f, cmpD, .5)), ms(pctOf(ks, c.f, cmpD, .95)), ms(pctOf(ks, c.f, cmpD, 1)),
			meanOf(ks, func(k keyStat) float64 { return ms(c.f(k)) }))
	}
	fmt.Fprintf(&sb, "  mean per key: batches %.1f instances %.0f atlas-upload %.0f B images %.2f (%.0f B) dirty %.3f of target\n",
		meanOf(ks, func(k keyStat) float64 { return float64(k.batches) }),
		meanOf(ks, func(k keyStat) float64 { return float64(k.instances) }),
		meanOf(ks, func(k keyStat) float64 { return float64(k.upBytes) }),
		meanOf(ks, func(k keyStat) float64 { return float64(k.images) }),
		meanOf(ks, func(k keyStat) float64 { return float64(k.imageBytes) }),
		meanOf(ks, func(k keyStat) float64 { return k.dirty }))
	fmt.Fprintf(&sb, "  mean per key: minimap rows rebuilt %.1f bitmaps %.1f strips %.2f; mini bytes %.0f bracket bytes %.0f line copies %.2f\n",
		meanOf(ks, func(k keyStat) float64 { return float64(k.rebuilt) }),
		meanOf(ks, func(k keyStat) float64 { return float64(k.bitmaps) }),
		meanOf(ks, func(k keyStat) float64 { return float64(k.strips) }),
		meanOf(ks, func(k keyStat) float64 { return float64(k.miniBytes) }),
		meanOf(ks, func(k keyStat) float64 { return float64(k.bracketBytes) }),
		meanOf(ks, func(k keyStat) float64 { return float64(k.copies) }))
	fmt.Fprintf(&sb, "  mean per key: alloc %.0f KB in %.0f mallocs; GCs %.3f/key\n",
		meanOf(ks, func(k keyStat) float64 { return float64(k.allocBytes) / 1024 }),
		meanOf(ks, func(k keyStat) float64 { return float64(k.mallocs) }),
		meanOf(ks, func(k keyStat) float64 { return float64(k.gcs) }))
	full, small := 0, 0
	for _, k := range ks {
		if k.dirty >= 0.9 {
			full++
		} else if k.dirty < 0.1 {
			small++
		}
	}
	var w0, w1 []keyStat
	for _, k := range ks {
		if k.images == 0 {
			w0 = append(w0, k)
		} else {
			w1 = append(w1, k)
		}
	}
	waitOf := func(k keyStat) time.Duration { return k.wait }
	cpuOf := func(k keyStat) time.Duration { return k.cpu }
	if len(w0) > 0 && len(w1) > 0 {
		fmt.Fprintf(&sb, "  keys without image upload: %d, cpu p50 %.2f wait p50 %.2f p95 %.2f; with: %d, cpu p50 %.2f wait p50 %.2f p95 %.2f\n",
			len(w0), ms(pctOf(w0, cpuOf, cmpD, .5)), ms(pctOf(w0, waitOf, cmpD, .5)), ms(pctOf(w0, waitOf, cmpD, .95)),
			len(w1), ms(pctOf(w1, cpuOf, cmpD, .5)), ms(pctOf(w1, waitOf, cmpD, .5)), ms(pctOf(w1, waitOf, cmpD, .95)))
	}
	fmt.Fprintf(&sb, "  damage: %d keys ≥ 90%% of the target, %d keys < 10%%\n", full, small)
	idx := make([]int, len(ks))
	for i := range idx {
		idx[i] = i
	}
	slices.SortFunc(idx, func(a, b int) int { return int(ks[b].total - ks[a].total) })
	fmt.Fprintf(&sb, "  full-damage keys (key: declared rects area scrolls | dirty):")
	nf := 0
	for i, k := range ks {
		if k.dirty >= 0.9 && nf < 12 {
			nf++
			fmt.Fprintf(&sb, " %d:%d/%.2f/%d|%.2f", i, k.declRects, k.declArea, k.scrolls, k.dirty)
		}
	}
	fmt.Fprintf(&sb, "\n  slowest keys (key: total cpu wait | batches inst img/B dirty | rebuilt bmp strips | alloc gcs):\n")
	show := idx[:min(len(idx), 12)]
	if os.Getenv("TANGO_EDITOR_BREAKDOWN_TAIL") != "" {
		show = idx[:min(len(idx), len(idx)/10)] // the slowest 10%: what p95 is made of
	}
	for _, i := range show {
		k := ks[i]
		fmt.Fprintf(&sb, "    %3d: %6.2f %6.2f %6.2f | %d %d %d/%d %.3f | %d %d %d | %dKB %d\n", i, ms(k.total), ms(k.cpu), ms(k.wait),
			k.batches, k.instances, k.images, k.imageBytes, k.dirty, k.rebuilt, k.bitmaps, k.strips, k.allocBytes/1024, k.gcs)
	}
	t.Log(sb.String())
}

func TestLongLineKeystrokeBreakdown(t *testing.T) {
	n, _ := strconv.Atoi(os.Getenv("TANGO_EDITOR_BREAKDOWN"))
	if n <= 0 {
		t.Skip("measurement only: TANGO_EDITOR_BREAKDOWN=<keys>")
	}
	metalDevice(t)
	for _, partial := range []bool{false, true} {
		name := "full redraw (gate host)"
		if partial {
			name = "partial redraw (application host)"
		}
		reportBreakdown(t, name, runBreakdown(t, partial, n))
	}
}
