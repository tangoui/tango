//go:build darwin

package metal

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/internal/gpuconf"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

// editorLines is editorList (perf_darwin_test.go) starting at line first:
// the same gutter and current-line highlight, the text scrolled by whole
// lines.
func editorLines(tb testing.TB, first int) *paint.List {
	tb.Helper()
	if len(text.Families()) == 0 {
		tb.Skip("no system fonts")
	}
	st := text.Style{Size: editorFontSize, LineHeight: editorLineHeight, Mono: true}
	l := &paint.List{Background: color.NRGBA{255, 255, 255, 255}}
	l.Fill(geom.R(0, 0, editorGutter, windowSize.H), geom.Radii{}, color.NRGBA{0xf6, 0xf8, 0xfa, 255})
	l.Fill(geom.R(editorGutter, 12*editorLineHeight, windowSize.W-editorGutter, editorLineHeight), geom.Radii{}, color.NRGBA{0xff, 0xf8, 0xc5, 255})
	for row := 0; row < editorRows; row++ {
		line := first + row
		w := line * 7 // each line's text depends on its number only
		var sb strings.Builder
		for sb.Len() < editorCols {
			sb.WriteString(editorWords[(w+line*3)%len(editorWords)])
			sb.WriteByte(' ')
			w += 7 + line
		}
		y := float32(row * editorLineHeight)
		num := text.Shape(fmt.Sprintf("%4d", line+1), st, text.Options{NoWrap: true})
		l.Glyphs(geom.Pt(4, y), num.Glyphs, editorPalette[4])
		g := text.Shape(sb.String()[:editorCols], st, text.Options{NoWrap: true}).Glyphs
		for i, run := 0, 0; i < len(g); run++ {
			n := min(len(g)-i, 3+(run*5+line)%9)
			l.Glyphs(geom.Pt(editorGutter+8, y), g[i:i+n], editorPalette[(run+line)%len(editorPalette)])
			i += n
		}
	}
	return l
}

// retainedTimed is timedDevice that is a gpu.Retainer, so the Renderer
// draws partial frames on it.
type retainedTimed struct{ timedDevice }

func (rt *retainedTimed) Retained(size image.Point) bool { return rt.d.Retained(size) }

// benchPartial renders frames[i%len(frames)] each iteration (after one
// warm-up pass over all of them) on a Retainer without verify mode and
// reports the metrics of benchFrames plus the frames' dirty fraction.
func benchPartial(b *testing.B, d *Device, warm *paint.List, frames []*paint.List, scale float32) {
	td := &retainedTimed{timedDevice{d: d}}
	r := gpu.NewRenderer(td)
	r.SetVerify(false)
	for _, l := range append([]*paint.List{warm}, frames...) {
		if err := r.Render(l, windowSize, scale); err != nil {
			b.Fatal(err)
		}
	}
	if _, err := d.wait(); err != nil {
		b.Fatal(err)
	}
	var gpuT, submitT, frameT time.Duration
	td.t = 0
	i := 0
	b.ResetTimer()
	for b.Loop() {
		start := time.Now()
		if err := r.Render(frames[i%len(frames)], windowSize, scale); err != nil {
			b.Fatal(err)
		}
		i++
		submitT += time.Since(start)
		g, err := d.wait()
		if err != nil {
			b.Fatal(err)
		}
		frameT += time.Since(start)
		gpuT += g
	}
	n := float64(b.N)
	b.ReportMetric(float64(gpuT.Microseconds())/1000/n, "gpu-ms")
	b.ReportMetric(float64(submitT.Microseconds())/1000/n, "submit-ms")
	b.ReportMetric(float64(td.t.Microseconds())/1000/n, "exec-ms")
	b.ReportMetric(float64(frameT.Microseconds())/1000/n, "frame-ms")
}

// BenchmarkMetalEditorDamage: the full-window editor frame with one line
// damaged (typing; spec 6.6: single-line edit ≤ 1 ms).
func BenchmarkMetalEditorDamage(b *testing.B) {
	for _, scale := range []float32{1, 2} {
		b.Run(fmt.Sprintf("scale%v", scale), func(b *testing.B) {
			d := newDevice(b)
			l := editorLines(b, 0)
			l.Damage(geom.R(0, 20*editorLineHeight, windowSize.W, editorLineHeight))
			benchPartial(b, d, editorLines(b, 0), []*paint.List{l}, scale)
		})
	}
}

// BenchmarkMetalEditorScroll: the editor scrolled by one line per frame
// (down, then back up): a copy of the text area and one redrawn line.
func BenchmarkMetalEditorScroll(b *testing.B) {
	for _, scale := range []float32{1, 2} {
		b.Run(fmt.Sprintf("scale%v", scale), func(b *testing.B) {
			d := newDevice(b)
			down, up := editorScroll(b)
			benchPartial(b, d, editorLines(b, 0), []*paint.List{down, up}, scale)
		})
	}
}

// editorScroll returns the editor scrolled down by one line from line 0
// and back up, with their hints: the gutter and text scroll; the
// current-line highlight stays, so its old and new place are damaged.
func editorScroll(tb testing.TB) (down, up *paint.List) {
	area := geom.R(0, 0, windowSize.W, windowSize.H)
	hl := geom.R(0, 12*editorLineHeight, windowSize.W, editorLineHeight)
	down = editorLines(tb, 1)
	down.Scroll(area, geom.Pt(0, -editorLineHeight))
	down.Damage(hl)
	down.Damage(hl.Offset(0, -editorLineHeight)) // where the copy moved the old highlight
	up = editorLines(tb, 0)
	up.Scroll(area, geom.Pt(0, editorLineHeight))
	up.Damage(hl)
	up.Damage(hl.Offset(0, editorLineHeight))
	return down, up
}

// TestMetalEditorPartial: the benchmarks' partial editor frames (one
// damaged line; one-line scrolls both ways) equal the full frames (verify
// mode) and are partial.
func TestMetalEditorPartial(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			d := newDevice(t)
			lf := &gpuconf.LastFrame{Device: d}
			r := gpu.NewRenderer(lf)
			r.SetVerify(true)
			if err := r.Render(editorLines(t, 0), windowSize, scale); err != nil {
				t.Fatal(err)
			}
			l := editorLines(t, 0)
			l.Damage(geom.R(0, 20*editorLineHeight, windowSize.W, editorLineHeight))
			down, up := editorScroll(t)
			for i, f := range []*paint.List{l, down, up, down} {
				if err := r.Render(f, windowSize, scale); err != nil {
					t.Fatalf("frame %d: %v", i, err)
				}
			}
			r.SetVerify(false)
			if err := r.Render(up, windowSize, scale); err != nil {
				t.Fatal(err)
			}
			if lf.Dirty == 0 || len(lf.Copies) != 1 {
				t.Fatalf("scroll frame: %d dirty, copies %v", lf.Dirty, lf.Copies)
			}
		})
	}
}

// TestMetalScratchLimits: copy sources too tall to stack in one scratch
// texture are refused with ErrNotRetained, which is not a device failure
// (the Renderer draws the frame in full); the scratch shrinks when the
// copies get much smaller and is released after scratchIdleFrames frames
// without copies.
func TestMetalScratchLimits(t *testing.T) {
	d := newDevice(t)
	size := image.Pt(4096, 4096)
	if err := d.Execute(&gpu.Frame{Size: size}); err != nil {
		t.Fatal(err)
	}
	var copies []gpu.CopyRegion
	for i := range 5 { // 5 × 4000 rows > 16384
		x := i * 800
		copies = append(copies, gpu.CopyRegion{Src: image.Rect(x, 10, x+700, 4010), Dst: image.Rect(x, 0, x+700, 4000)})
	}
	dirty := []image.Rectangle{image.Rect(0, 4000, 4096, 4096)}
	for range maxFailures + 1 {
		err := d.Execute(&gpu.Frame{Size: size, Dirty: dirty, Copies: copies})
		if !errors.Is(err, gpu.ErrNotRetained) {
			t.Fatalf("err = %v, want ErrNotRetained", err)
		}
	}
	if d.failures != 0 || d.lost != nil {
		t.Fatalf("failures %d, lost %v: an oversized scratch is not a device failure", d.failures, d.lost)
	}
	// A large copy, then a small one: the scratch shrinks.
	big := []gpu.CopyRegion{{Src: image.Rect(0, 10, 4096, 4010), Dst: image.Rect(0, 0, 4096, 4000)}}
	if err := d.Execute(&gpu.Frame{Size: size, Dirty: dirty, Copies: big}); err != nil {
		t.Fatal(err)
	}
	bigBytes := d.scratchBytes()
	small := []gpu.CopyRegion{{Src: image.Rect(0, 10, 100, 110), Dst: image.Rect(0, 0, 100, 100)}}
	if err := d.Execute(&gpu.Frame{Size: size, Dirty: dirty, Copies: small}); err != nil {
		t.Fatal(err)
	}
	if b := d.scratchBytes(); b == 0 || b*4 > bigBytes {
		t.Fatalf("scratch %d bytes after a small copy (was %d)", b, bigBytes)
	}
	for range scratchIdleFrames + 1 {
		if err := d.Execute(&gpu.Frame{Size: size, Dirty: dirty}); err != nil {
			t.Fatal(err)
		}
	}
	if b := d.scratchBytes(); b != 0 {
		t.Fatalf("scratch %d bytes after %d frames without copies", b, scratchIdleFrames+1)
	}
}
