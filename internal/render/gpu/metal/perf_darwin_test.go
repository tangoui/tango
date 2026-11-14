//go:build darwin

package metal

import (
	"fmt"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/internal/gputest"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// windowSize is the reference window of spec 6.6 (1920×1080 logical).
var windowSize = geom.Size{W: 1920, H: 1080}

// Editor text frame of spec 6.6: about 60 lines × 200 columns of
// highlighted monospace text filling the window.
const (
	editorRows       = 60
	editorCols       = 200
	editorFontSize   = 13
	editorLineHeight = 18 // 60 × 18 = 1080
	editorGutter     = 48
)

var editorPalette = []color.NRGBA{
	{0x1f, 0x1f, 0x1f, 255}, // identifiers
	{0xcf, 0x22, 0x2e, 255}, // keywords
	{0x0a, 0x30, 0x69, 255}, // types
	{0x0a, 0x7e, 0x3e, 255}, // strings
	{0x6e, 0x77, 0x81, 255}, // comments
	{0x82, 0x50, 0xdf, 255}, // numbers
}

var editorWords = []string{"func", "return", "if", "err", "!=", "nil", "{", "}", "for", "range",
	"x", ":=", "make([]byte,", "len(buf))", "\"tango\"", "// comment", "0x7f", "go", "defer", "f.Close()"}

// editorList builds a full-window editor frame: a gutter with line numbers,
// the current-line highlight and editorRows lines of editorCols monospace
// columns, each line split into runs of different colors (highlighting).
func editorList(tb testing.TB) *paint.List {
	tb.Helper()
	if len(text.Families()) == 0 {
		tb.Skip("no system fonts")
	}
	st := text.Style{Size: editorFontSize, LineHeight: editorLineHeight, Mono: true}
	l := &paint.List{Background: color.NRGBA{255, 255, 255, 255}}
	l.Fill(geom.R(0, 0, editorGutter, windowSize.H), geom.Radii{}, color.NRGBA{0xf6, 0xf8, 0xfa, 255})
	l.Fill(geom.R(editorGutter, 12*editorLineHeight, windowSize.W-editorGutter, editorLineHeight), geom.Radii{}, color.NRGBA{0xff, 0xf8, 0xc5, 255})
	w := 0
	for row := 0; row < editorRows; row++ {
		var sb strings.Builder
		for sb.Len() < editorCols {
			sb.WriteString(editorWords[w%len(editorWords)])
			sb.WriteByte(' ')
			w += 7 + row // vary the lines
		}
		line := sb.String()[:editorCols]
		y := float32(row * editorLineHeight)
		num := text.Shape(fmt.Sprintf("%4d", row+1), st, text.Options{NoWrap: true})
		l.Glyphs(geom.Pt(4, y), num.Glyphs, editorPalette[4])
		lay := text.Shape(line, st, text.Options{NoWrap: true})
		g := lay.Glyphs
		for i, run := 0, 0; i < len(g); run++ {
			n := min(len(g)-i, 3+(run*5+row)%9)
			l.Glyphs(geom.Pt(editorGutter+8, y), g[i:i+n], editorPalette[(run+row)%len(editorPalette)])
			i += n
		}
	}
	return l
}

// TestMetalEditorText: the editor frame matches paint.CPU (text
// threshold), so BenchmarkMetalEditorFrame measures a correct frame.
func TestMetalEditorText(t *testing.T) {
	d := newDevice(t)
	l := editorList(t)
	r := gpu.NewRenderer(d)
	for _, scale := range []float32{1, 2} {
		ref := paint.NewCPU()
		ref.Render(l, windowSize, scale)
		if err := r.Render(l, windowSize, scale); err != nil {
			t.Fatal(err)
		}
		gputest.Check(t, "editor", l, scale, ref.Image(), readPixels(t, d), 4)
	}
}

// benchFrames renders l once per iteration with r on td (a timedDevice
// over d) and reports gpu-ms, submit-ms and exec-ms as
// BenchmarkMetalGalleryFrame does, plus frame-ms: from the start of
// Render to the completion of the command buffer (submit, queueing and GPU
// time; what "整帧" of spec 6.6 measures). With cold set, every iteration
// uses a new Renderer (empty glyph atlas and patch cache, spec 6.8's
// first-frame metric).
func benchFrames(b *testing.B, d *Device, l *paint.List, size geom.Size, scale float32, cold bool) {
	td := &timedDevice{d: d}
	r := gpu.NewRenderer(td)
	if err := r.Render(l, size, scale); err != nil { // warm the atlas, size the target
		b.Fatal(err)
	}
	if _, err := d.wait(); err != nil {
		b.Fatal(err)
	}
	var gpuT, submitT, frameT time.Duration
	td.t = 0
	b.ResetTimer()
	for b.Loop() {
		if cold {
			r = gpu.NewRenderer(td)
		}
		start := time.Now()
		if err := r.Render(l, size, scale); err != nil {
			b.Fatal(err)
		}
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

// BenchmarkMetalGalleryWindow renders the gallery page in the reference
// window of spec 6.6 (1920×1080 logical, so 3840×2160 pixels at 2x): the
// same page as BenchmarkMetalGalleryFrame, with the clear and the
// render-target size of a full window.
func BenchmarkMetalGalleryWindow(b *testing.B) {
	for _, scale := range []float32{1, 2} {
		b.Run(fmt.Sprintf("scale%v", scale), func(b *testing.B) {
			d := newDevice(b)
			drv := ui.NewDriver(galleryPage, ui.DriverOptions{Width: windowSize.W, Height: windowSize.H, Scale: scale, Theme: theme.AntDesign()})
			drv.Settle()
			benchFrames(b, d, drv.Runner().List(), windowSize, scale, false)
		})
	}
}

// BenchmarkMetalEditorFrame renders the full-window editor text frame
// (spec 6.6: ≤ 4 ms per frame) with a warm glyph atlas.
func BenchmarkMetalEditorFrame(b *testing.B) {
	for _, scale := range []float32{1, 2} {
		b.Run(fmt.Sprintf("scale%v", scale), func(b *testing.B) {
			d := newDevice(b)
			benchFrames(b, d, editorList(b), windowSize, scale, false)
		})
	}
}

// BenchmarkMetalEditorColdFrame renders the editor frame with a new
// Renderer every iteration (spec 6.8 cold start: first editor frame with
// an empty glyph atlas ≤ 50 ms). The Metal device and its textures are
// reused, as after a font-size or DPI change.
func BenchmarkMetalEditorColdFrame(b *testing.B) {
	for _, scale := range []float32{1, 2} {
		b.Run(fmt.Sprintf("scale%v", scale), func(b *testing.B) {
			d := newDevice(b)
			benchFrames(b, d, editorList(b), windowSize, scale, true)
		})
	}
}

// opacityList is a window of 48 translucent button-like groups (a rounded
// fill, a border and a label each, at opacity 0.65: disabled controls)
// over a background: every group needs a layer.
func opacityList(tb testing.TB) *paint.List {
	l := &paint.List{Background: gputest.White}
	for i := range 48 {
		x, y := float32(i%6)*300+40, float32(i/6)*120+40
		l.Opacity(0.65)
		l.Fill(geom.R(x, y, 220, 64), geom.Rad(8), gputest.Blue)
		l.Stroke(geom.R(x, y, 220, 64), geom.Rad(8), 1, gputest.Ink)
		l.Glyphs(geom.Pt(x+16, y+20), gputest.Shape(tb, "禁用按钮 Disabled", 16), gputest.White)
		l.Pop()
	}
	return l
}

// BenchmarkMetalOpacityGroups renders opacityList in the reference window
// (warm atlas).
func BenchmarkMetalOpacityGroups(b *testing.B) {
	for _, scale := range []float32{1, 2} {
		b.Run(fmt.Sprintf("scale%v", scale), func(b *testing.B) {
			d := newDevice(b)
			benchFrames(b, d, opacityList(b), windowSize, scale, false)
		})
	}
}
