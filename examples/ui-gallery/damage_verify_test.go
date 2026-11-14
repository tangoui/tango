package main

import (
	"image"
	"testing"
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/cpuexec"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/paint"
)

// hintWin is a ui.Window without a screen.
type hintWin struct{ size geom.Size }

func (w *hintWin) Size() geom.Size              { return w.size }
func (w *hintWin) Scale() float32               { return 1 }
func (w *hintWin) Invalidate()                  {}
func (w *hintWin) InvalidateAt(time.Time)       {}
func (w *hintWin) SetCursor(input.Cursor)       {}
func (w *hintWin) ReadClipboard() string        { return "" }
func (w *hintWin) WriteClipboard(string)        {}
func (w *hintWin) SetTextInput(bool, geom.Rect) {}
func (w *hintWin) Present(*image.RGBA) error    { return nil }

// hinted renders a gallery page with a 1×1 Damage declared every frame:
// every frame carries hints, so everything else that changes must be
// found by the Runner (its own tracking and the paint fingerprints).
func hinted(p gallery.Page) func(*ui.Context) {
	return func(c *ui.Context) {
		c.Damage(geom.Rect{W: 1, H: 1})
		if p.Bare {
			p.Render(c)
			return
		}
		gallery.Frame(c, p.Title, func() { p.Render(c) })
	}
}

// TestGalleryForcedHintsVerified drives a few gallery pages with forced
// hints through a gpu.Renderer on cpuexec in strict verify mode (every
// partial frame is compared with a full redraw): hover, wheel scrolling,
// clicks and typing must never leave stale pixels.
func TestGalleryForcedHintsVerified(t *testing.T) {
	if testing.Short() {
		t.Skip("rasterizes every partial frame twice on the CPU")
	}
	for _, name := range []string{"Table", "Form", "Tree", "Collapse", "Input", "Select", "Button", "VirtualTree"} {
		t.Run(name, func(t *testing.T) {
			p, ok := gallery.Find(name)
			if !ok {
				t.Skipf("no page %s", name)
			}
			dash, ov = newDash(), newOverlayState()
			dev := cpuexec.NewDevice()
			dev.SampledOnly = true
			ren := gpu.NewRenderer(dev)
			ren.SetVerify(true)
			ren.SetVerifyStrict(true)
			now := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
			r := ui.NewRunner(hinted(p), ui.RunnerOptions{Renderer: ren, NoAnimations: true, Clock: func() time.Time { return now }})
			size := geom.Size{W: 960, H: 900}
			r.Attach(&hintWin{size: size})
			frames := 0
			frame := func(what string) {
				t.Helper()
				now = now.Add(16 * time.Millisecond)
				if err := r.Frame(); err != nil {
					t.Fatalf("%s: %v", what, err)
				}
				frames++
			}
			for range 3 {
				frame("settle")
			}
			// Hover across the page.
			for y := float32(60); y < size.H; y += 140 {
				for x := float32(40); x < size.W; x += 230 {
					r.HandleEvent(input.Pointer{Kind: input.Move, Pos: geom.Pt(x, y)})
					frame("hover")
				}
			}
			// Wheel scrolling (the page and any scroller under the pointer).
			for _, pt := range []geom.Point{{X: 480, Y: 450}, {X: 300, Y: 300}} {
				for _, dy := range []float32{40, 40, 13, -25, 60} {
					r.HandleEvent(input.Scroll{Pos: pt, DY: dy})
					frame("scroll")
				}
			}
			// Clicks.
			for _, pt := range []geom.Point{{X: 120, Y: 200}, {X: 400, Y: 320}, {X: 200, Y: 520}, {X: 600, Y: 260}} {
				r.HandleEvent(input.Pointer{Kind: input.Move, Pos: pt})
				r.HandleEvent(input.Pointer{Kind: input.Press, Pos: pt, Button: input.ButtonLeft})
				frame("press")
				r.HandleEvent(input.Pointer{Kind: input.Release, Pos: pt, Button: input.ButtonLeft})
				frame("release")
				frame("after click")
			}
			// Keyboard: click the first visible text box and type (an
			// Input changes its text without declaring damage).
			r.EnableAccessibility(true)
			frame("a11y")
			if tb, ok := firstTextbox(r.AccessibilityTree(), size); ok {
				r.HandleEvent(input.Pointer{Kind: input.Move, Pos: tb})
				r.HandleEvent(input.Pointer{Kind: input.Press, Pos: tb, Button: input.ButtonLeft})
				r.HandleEvent(input.Pointer{Kind: input.Release, Pos: tb, Button: input.ButtonLeft})
				frame("focus input")
			}
			for _, s := range []string{"a", "b", "c", "中"} {
				r.HandleEvent(input.TextInput{Text: s})
				frame("type")
			}
			r.HandleEvent(input.KeyEvent{Key: input.KeyBackspace, Down: true})
			r.HandleEvent(input.KeyEvent{Key: input.KeyBackspace})
			frame("backspace")
			fails, _ := ren.VerifyFailures()
			st := dev.Stats()
			t.Logf("%d frames, %d partial, %d verify failures", frames, st.Partial, fails)
			if fails != 0 {
				t.Fatalf("%d verify failures", fails)
			}
		})
	}
}

// firstTextbox returns the center of the first text box inside the
// window.
func firstTextbox(t *a11y.Tree, size geom.Size) (geom.Point, bool) {
	if t == nil {
		return geom.Point{}, false
	}
	for _, n := range t.Nodes() {
		b := n.Bounds
		if n.Role == a11y.RoleTextbox && b.W > 0 && b.X >= 0 && b.Y >= 0 && b.Y+b.H <= size.H {
			return geom.Pt(b.X+b.W/2, b.Y+b.H/2), true
		}
	}
	return geom.Point{}, false
}

// nopRenderer skips rasterization (the benchmark measures the Runner).
type nopRenderer struct{}

func (nopRenderer) Render(*paint.List, geom.Size, float32) error { return nil }

// BenchmarkGalleryTableFrame measures a frame of the Table page without
// rasterization, without hints (the fingerprints are only recorded) and
// with a forced hint (every frame hashes and compares them).
func BenchmarkGalleryTableFrame(b *testing.B) {
	p, ok := gallery.Find("Table")
	if !ok {
		b.Skip("no Table page")
	}
	for _, force := range []bool{false, true} {
		name := "nohints"
		if force {
			name = "hinted"
		}
		b.Run(name, func(b *testing.B) {
			dash, ov = newDash(), newOverlayState()
			page := func(c *ui.Context) { gallery.Frame(c, p.Title, func() { p.Render(c) }) }
			if force {
				page = hinted(p)
			}
			now := time.Date(2026, 11, 13, 9, 0, 0, 0, time.UTC)
			r := ui.NewRunner(page, ui.RunnerOptions{Renderer: nopRenderer{}, NoAnimations: true, Clock: func() time.Time { return now }})
			r.Attach(&hintWin{size: geom.Size{W: 960, H: 1400}})
			for range 3 {
				r.Frame()
			}
			b.ReportAllocs()
			for b.Loop() {
				r.Frame()
			}
			b.ReportMetric(float64(len(r.List().Ops)), "ops")
		})
	}
}
