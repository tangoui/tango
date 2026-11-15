//go:build darwin

package editor

// Measurement-only breakdown of a scroll frame of BenchmarkScrollPartial
// (1M-line file, two lines per frame, partial redraws on a Retainer) for
// the editor M2 failed-gate follow-up (Task C):
//
//	TANGO_EDITOR_SCROLL_BREAKDOWN=300 go test ./ui/editor -run '^TestScrollFrameBreakdown$' -v
//
// Per frame: the CPU part (event + Runner.Frame) split into the time
// spent in Device.Execute (encode + commit, after the Renderer built the
// gpu.Frame) and before it, the GPU wait, the gpu.Frame's dirty rectangles
// and copies, batches/instances, texture and image uploads, the List's
// Damage/ScrollRegion hints and the allocations. It then isolates the
// device's share of a frame by executing synthetic frames on the same
// target: the copies alone (with a one-pixel dirty rectangle), the dirty
// rectangles alone, and both.
//
// TANGO_EDITOR_SCROLL_BREAKDOWN_PROF=<file> writes a CPU profile of the
// scroll frames.

import (
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
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
)

// scrollStatDevice records the last executed frame (not the host's wait
// frames: the caller reads it before waiting) and the time Execute took.
type scrollStatDevice struct {
	*syncDevice
	exec, render, build time.Duration
	ren                 *gpu.Renderer
	dirty               []image.Rectangle
	copies              []gpu.CopyRegion
	batches, instances  int
	upBytes, images     int
	imageBytes          int
	size                image.Point
	ups                 []string // the last frame's uploads: texture, rect, bytes
}

func (s *scrollStatDevice) Render(l *paint.List, size geom.Size, scale float32) error {
	start := time.Now()
	err := s.ren.Render(l, size, scale)
	s.render = time.Since(start)
	return err
}

func (s *scrollStatDevice) Retained(size image.Point) bool { return s.d.Retained(size) }

func (s *scrollStatDevice) Execute(f *gpu.Frame) error {
	s.dirty = append(s.dirty[:0], f.Dirty...)
	s.copies = append(s.copies[:0], f.Copies...)
	s.batches, s.instances, s.upBytes = len(f.Batches), 0, 0
	s.ups = s.ups[:0]
	for _, u := range f.Uploads {
		s.upBytes += len(u.Pix)
		s.ups = append(s.ups, fmt.Sprintf("tex%d %v %dB", u.Tex, u.Rect, len(u.Pix)))
	}
	for i, b := range f.Batches {
		s.instances += len(b.Instances)
		for _, u := range b.Uploads {
			s.upBytes += len(u.Pix)
			s.ups = append(s.ups, fmt.Sprintf("batch%d tex%d %v %dB", i, u.Tex, u.Rect, len(u.Pix)))
		}
	}
	s.images, s.imageBytes = len(f.ImageUploads), 0
	for _, u := range f.ImageUploads {
		s.imageBytes += len(u.Pix)
	}
	s.size = f.Size
	t := time.Now()
	err := s.syncDevice.Execute(f)
	s.exec = time.Since(t)
	return err
}

type scrollStat struct {
	build, render          time.Duration
	total, cpu, exec, wait time.Duration
	dirtyRects, copies     int
	dirtyArea, copyArea    int
	bbox                   image.Rectangle
	batches, instances     int
	upBytes, images        int
	imageBytes             int
	damages, scrolls       int
	damageArea             float64 // logical px²
	allocBytes, mallocs    uint64
	dirty                  []image.Rectangle
	final, top             int // FinalLines and TopLine after the frame
}

// scrollBreakdownHost is the host of BenchmarkScrollPartial with a
// scrollStatDevice, scrolled to line 100 000.
func scrollBreakdownHost(tb testing.TB) (*perfHost, *scrollStatDevice) {
	path := perfFile(tb, 1_000_000)
	data, err := os.ReadFile(path)
	if err != nil {
		tb.Fatal(err)
	}
	st := NewState(string(data), path, theme.Base("vs"))
	perfApplyOffEnv(tb, st)
	h := &perfHost{st: st, w: &perfWindow{size: perfSize, scale: 2}}
	h.dev = &syncDevice{d: metalDevice(tb), keep: true}
	sd := &scrollStatDevice{syncDevice: h.dev}
	ren := gpu.NewRenderer(sd)
	ren.SetVerify(false)
	sd.ren = ren
	h.r = ui.NewRunner(func(c *ui.Context) {
		start := time.Now()
		h.vw = View(c, st).Layout(layout.Grow(0), layout.H(perfSize.H-1))
		sd.build += time.Since(start)
	}, ui.RunnerOptions{Renderer: sd, NoAnimations: true})
	h.r.Attach(h.w)
	h.frame(tb)
	h.caretTo(tb, 100_000)
	for range 3 {
		h.frame(tb)
	}
	return h, sd
}

// scrollFrame runs one benchmark scroll frame and its statistics.
func scrollFrame(tb testing.TB, h *perfHost, sd *scrollStatDevice) scrollStat {
	p := geom.Pt(perfSize.W/2, perfSize.H/2)
	dy := 2 * h.st.Viewport.LineHeight
	var ms0, ms1 runtime.MemStats
	runtime.ReadMemStats(&ms0)
	sd.build = 0
	start := time.Now()
	h.w.invalid.Store(false)
	h.r.HandleEvent(input.Scroll{Pos: p, DY: dy})
	if err := h.r.Frame(); err != nil {
		tb.Fatal(err)
	}
	t1 := time.Now()
	if err := h.dev.wait(); err != nil {
		tb.Fatal(err)
	}
	end := time.Now()
	runtime.ReadMemStats(&ms1)
	k := scrollStat{
		build: sd.build, render: sd.render,
		total: end.Sub(start), cpu: t1.Sub(start), exec: sd.exec, wait: end.Sub(t1),
		dirtyRects: len(sd.dirty), copies: len(sd.copies),
		batches: sd.batches, instances: sd.instances,
		upBytes: sd.upBytes, images: sd.images, imageBytes: sd.imageBytes,
		allocBytes: ms1.TotalAlloc - ms0.TotalAlloc, mallocs: ms1.Mallocs - ms0.Mallocs,
		dirty: slices.Clone(sd.dirty),
		top:   h.st.Viewport.TopLine,
	}
	if hl := h.st.Highlighter(); hl != nil {
		k.final = hl.FinalLines(h.st.Snapshot())
	}
	for _, r := range sd.dirty {
		k.dirtyArea += r.Dx() * r.Dy()
		k.bbox = k.bbox.Union(r)
	}
	if len(sd.dirty) == 0 {
		k.dirtyArea = sd.size.X * sd.size.Y
		k.bbox = image.Rectangle{Max: sd.size}
	}
	for _, c := range sd.copies {
		k.copyArea += c.Dst.Dx() * c.Dst.Dy()
	}
	for _, op := range h.r.List().Ops {
		switch o := op.(type) {
		case paint.Damage:
			k.damages++
			k.damageArea += float64(o.Rect.W * o.Rect.H)
		case paint.ScrollRegion:
			k.scrolls++
		}
	}
	return k
}

func scrollPct(ks []scrollStat, f func(scrollStat) time.Duration, p float64) float64 {
	v := make([]time.Duration, len(ks))
	for i, k := range ks {
		v[i] = f(k)
	}
	slices.Sort(v)
	return ms(v[int(float64(len(v)-1)*p)])
}

func scrollMean(ks []scrollStat, f func(scrollStat) float64) float64 {
	s := 0.0
	for _, k := range ks {
		s += f(k)
	}
	return s / float64(max(1, len(ks)))
}

// synthGPU executes n synthetic partial frames (no batches) on the
// device's target and returns the mean time from Execute to GPU done.
func synthGPU(tb testing.TB, h *perfHost, size image.Point, dirty []image.Rectangle, copies []gpu.CopyRegion, n int) float64 {
	// Keep atlas dimensions equal to the host's wait frame: alternating
	// zero/nonzero sizes would recreate those textures on every Execute
	// and measure texture allocation rather than the synthetic GPU work.
	f := h.dev.last
	f.Size, f.Dirty, f.Copies = size, dirty, copies
	var sum time.Duration
	for range n {
		t := time.Now()
		if err := h.dev.d.Execute(&f); err != nil {
			tb.Fatal(err)
		}
		if err := h.dev.wait(); err != nil {
			tb.Fatal(err)
		}
		sum += time.Since(t)
	}
	return ms(sum / time.Duration(n))
}

func TestScrollFrameBreakdown(t *testing.T) {
	n, _ := strconv.Atoi(os.Getenv("TANGO_EDITOR_SCROLL_BREAKDOWN"))
	if n <= 0 {
		t.Skip("measurement only: TANGO_EDITOR_SCROLL_BREAKDOWN=<frames>")
	}
	metalDevice(t)
	h, sd := scrollBreakdownHost(t)
	if os.Getenv("TANGO_EDITOR_SCROLL_BREAKDOWN_HL") != "" {
		// Variant: the lines the run shows already final (the highlighter
		// front past them), so no frame tokenizes the lines above.
		hl := h.st.Highlighter()
		if !hl.Highlight(h.st.Snapshot(), 100_000+2*n+200, time.Minute) {
			t.Fatal("not highlighted")
		}
		h.frame(t)
	}
	if path := os.Getenv("TANGO_EDITOR_SCROLL_BREAKDOWN_PROF"); path != "" {
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		if err := pprof.StartCPUProfile(f); err != nil {
			t.Fatal(err)
		}
		defer func() { pprof.StopCPUProfile(); f.Close() }()
	}
	ks := make([]scrollStat, 0, n)
	for range n {
		ks = append(ks, scrollFrame(t, h, sd))
	}
	var sb strings.Builder
	full := perfSize.W * perfSize.H * 4
	fmt.Fprintf(&sb, "scroll partial: %d frames, load %s, target %v\n", n, loadAvg(), sd.size)
	for _, c := range []struct {
		n string
		f func(scrollStat) time.Duration
	}{
		{"total", func(k scrollStat) time.Duration { return k.total }},
		{"cpu", func(k scrollStat) time.Duration { return k.cpu }},
		{"build-cb", func(k scrollStat) time.Duration { return k.build }},
		// Residual includes event processing and framework layout/paint;
		// the public Runner API does not expose those separately.
		{"layout+paint+events", func(k scrollStat) time.Duration { return k.cpu - k.render - k.build }},
		{"render-plan", func(k scrollStat) time.Duration { return k.render - k.exec }},
		{"pre-exec", func(k scrollStat) time.Duration { return k.cpu - k.exec }},
		{"exec", func(k scrollStat) time.Duration { return k.exec }},
		{"wait", func(k scrollStat) time.Duration { return k.wait }},
	} {
		fmt.Fprintf(&sb, "  %-8s p50 %6.2f p99 %6.2f max %6.2f mean %6.2f ms\n", c.n,
			scrollPct(ks, c.f, .5), scrollPct(ks, c.f, .99), scrollPct(ks, c.f, 1),
			scrollMean(ks, func(k scrollStat) float64 { return ms(c.f(k)) }))
	}
	fmt.Fprintf(&sb, "  mean: dirty rects %.2f area %.4f of target, bbox area %.4f; copies %.2f area %.4f\n",
		scrollMean(ks, func(k scrollStat) float64 { return float64(k.dirtyRects) }),
		scrollMean(ks, func(k scrollStat) float64 { return float64(k.dirtyArea) / float64(full) }),
		scrollMean(ks, func(k scrollStat) float64 { return float64(k.bbox.Dx()*k.bbox.Dy()) / float64(full) }),
		scrollMean(ks, func(k scrollStat) float64 { return float64(k.copies) }),
		scrollMean(ks, func(k scrollStat) float64 { return float64(k.copyArea) / float64(full) }))
	fmt.Fprintf(&sb, "  mean: batches %.1f instances %.0f atlas upload %.0f B images %.2f (%.0f B); hints: damage %.1f (%.0f px²) scroll %.1f\n",
		scrollMean(ks, func(k scrollStat) float64 { return float64(k.batches) }),
		scrollMean(ks, func(k scrollStat) float64 { return float64(k.instances) }),
		scrollMean(ks, func(k scrollStat) float64 { return float64(k.upBytes) }),
		scrollMean(ks, func(k scrollStat) float64 { return float64(k.images) }),
		scrollMean(ks, func(k scrollStat) float64 { return float64(k.imageBytes) }),
		scrollMean(ks, func(k scrollStat) float64 { return float64(k.damages) }),
		scrollMean(ks, func(k scrollStat) float64 { return k.damageArea }),
		scrollMean(ks, func(k scrollStat) float64 { return float64(k.scrolls) }))
	nUp, nFull := 0, 0
	for _, k := range ks {
		if k.upBytes > 0 {
			nUp++
		}
		if k.dirtyRects == 0 {
			nFull++
		}
	}
	fmt.Fprintf(&sb, "  frames with atlas uploads %d, full frames %d\n", nUp, nFull)
	fmt.Fprintf(&sb, "  mean: alloc %.1f KB in %.0f mallocs\n",
		scrollMean(ks, func(k scrollStat) float64 { return float64(k.allocBytes) / 1024 }),
		scrollMean(ks, func(k scrollStat) float64 { return float64(k.mallocs) }))
	last := ks[len(ks)-1]
	fmt.Fprintf(&sb, "  final lines: first frame %d (top %d), last %d (top %d)\n", ks[0].final, ks[0].top, last.final, last.top)
	fmt.Fprintf(&sb, "  last frame: dirty %v copies %v\n  last frame uploads: %v\n", last.dirty, sd.copies, sd.ups)
	// Rectangle shapes over the run (how often each set of rects occurs).
	shapes := map[string]int{}
	for _, k := range ks {
		var s []string
		for _, r := range k.dirty {
			s = append(s, fmt.Sprintf("%dx%d@%d,%d", r.Dx(), r.Dy(), r.Min.X, r.Min.Y))
		}
		shapes[strings.Join(s, " ")]++
	}
	keys := make([]string, 0, len(shapes))
	for s := range shapes {
		keys = append(keys, s)
	}
	slices.SortFunc(keys, func(a, b string) int { return shapes[b] - shapes[a] })
	for _, s := range keys[:min(len(keys), 6)] {
		fmt.Fprintf(&sb, "    %4d× %s\n", shapes[s], s)
	}
	idx := make([]int, len(ks))
	for i := range idx {
		idx[i] = i
	}
	slices.SortFunc(idx, func(a, b int) int { return int(ks[b].total - ks[a].total) })
	fmt.Fprintf(&sb, "  slowest (frame: total cpu exec wait | rects area | batches inst img | alloc KB):\n")
	for _, i := range idx[:min(len(idx), 8)] {
		k := ks[i]
		fmt.Fprintf(&sb, "    %3d: %6.2f %6.2f %6.2f %6.2f | %d %.4f | %d %d %d | %d\n", i, ms(k.total), ms(k.cpu), ms(k.exec), ms(k.wait),
			k.dirtyRects, float64(k.dirtyArea)/float64(full), k.batches, k.instances, k.images, k.allocBytes/1024)
	}
	// Device share: synthetic frames with the last frame's dirty
	// rectangles and copies but no batches.
	const m = 100
	px := []image.Rectangle{image.Rect(0, sd.size.Y-1, 1, sd.size.Y)}
	fmt.Fprintf(&sb, "  synthetic device frames (Execute → GPU done incl. the host's 1px wait frame, mean of %d):\n"+
		"    1px dirty %.3f ms; copies+1px %.3f ms; dirty only %.3f ms; copies+dirty %.3f ms; full clear (no Dirty) %.3f ms\n", m,
		synthGPU(t, h, sd.size, px, nil, m),
		synthGPU(t, h, sd.size, px, sd.copies, m),
		synthGPU(t, h, sd.size, last.dirty, nil, m),
		synthGPU(t, h, sd.size, last.dirty, sd.copies, m),
		synthGPU(t, h, sd.size, nil, nil, m))
	t.Log(sb.String())
}
