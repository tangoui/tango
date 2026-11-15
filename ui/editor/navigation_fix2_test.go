package editor

import (
	"bytes"
	"fmt"
	"image"
	"math"
	"strings"
	"testing"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/cpuexec"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// Capture at Execute, before strict verify replaces Device.Last with its full
// reference frame. Keep only immutable metadata, not renderer-owned slices.
type navExecution struct {
	size   image.Point
	dirty  []image.Rectangle
	copies int
}
type navTraceDevice struct {
	*cpuexec.Device
	frames []navExecution
}

func (d *navTraceDevice) Execute(f *gpu.Frame) error {
	d.frames = append(d.frames, navExecution{f.Size, append([]image.Rectangle(nil), f.Dirty...), len(f.Copies)})
	return d.Device.Execute(f)
}

type navStepResult struct {
	calls, executed, partial int
	frames                   []navExecution
}

func (h *hintHost) tracedStep(what string) navStepResult {
	h.t.Helper()
	before := h.dev.Stats()
	start := len(h.trace.frames)
	h.step(what)
	after := h.dev.Stats()
	result := navStepResult{after.Calls - before.Calls, after.Executed - before.Executed, after.Partial - before.Partial, h.trace.frames[start:]}
	kind := "no-draw"
	if result.executed > 0 {
		kind = "full"
		if result.partial > 0 {
			kind = "partial"
		}
	}
	h.t.Logf("%s: %s Execute=%d success=%d partial=%d captured=%v", what, kind, result.calls, result.executed, result.partial, result.frames)
	if n, last := h.ren.VerifyFailures(); n != 0 || last != nil {
		h.t.Fatalf("%s: strict comparison failures %d: %v", what, n, last)
	}
	return result
}
func (h *hintHost) requirePartialStep(what string) navStepResult {
	h.t.Helper()
	r := h.tracedStep(what)
	if r.calls != 2 || r.executed != 2 || r.partial != 1 || len(r.frames) != 2 {
		dam, shift := h.lineHints()
		h.t.Logf("failed partial: hints=%v shifts=%v focus=%v viewport=%+v", dam, shift, h.vw.Focused(), h.st.Viewport)
		h.t.Fatalf("%s must execute partial + strict full reference, got %+v", what, r)
	}
	main, reference := r.frames[0], r.frames[1]
	if len(main.dirty) == 0 || len(reference.dirty) != 0 {
		h.t.Fatalf("%s: main Dirty=%v reference Dirty=%v", what, main.dirty, reference.dirty)
	}
	area := 0
	for _, rect := range main.dirty {
		if rect.Empty() || !rect.In(image.Rectangle{Max: main.size}) {
			h.t.Fatalf("%s invalid Dirty %v", what, rect)
		}
		area += rect.Dx() * rect.Dy()
	}
	if area == 0 || 2*area > main.size.X*main.size.Y {
		h.t.Fatalf("%s: dirty area %d, target %v", what, area, main.size)
	}
	return r
}

func (h *hintHost) requireExecutedStep(what string) navStepResult {
	h.t.Helper()
	r := h.tracedStep(what)
	if r.calls < 1 || r.executed != r.calls || len(r.frames) != r.calls {
		h.t.Fatalf("%s must actually execute, got %+v", what, r)
	}
	if r.partial > 0 {
		if r.partial != 1 || r.calls != 2 || len(r.frames[0].dirty) == 0 || len(r.frames[1].dirty) != 0 {
			h.t.Fatalf("%s invalid partial/reference sequence: %+v", what, r)
		}
	} else if r.calls != 1 || len(r.frames[0].dirty) != 0 {
		h.t.Fatalf("%s invalid full execution: %+v", what, r)
	}
	return r
}

// Capture changes are the deliberate framework exception: ui/damage.go
// damages the whole active node on press/release. Never count them as partial.
// A future opt-out API may make them partial; then the strict reference must
// execute too. Full is accepted only with an explicit full-editor hint.
func (h *hintHost) requireCaptureStep(what string) {
	h.t.Helper()
	r := h.requireExecutedStep(what)
	if r.partial == 0 {
		dam, _ := h.lineHints()
		fullHint := false
		for _, rect := range dam {
			fullHint = fullHint || rect == h.vw.vs.rect
		}
		if !fullHint || !h.dev.Retained(r.frames[0].size) {
			h.t.Fatalf("%s: full is not justified by framework capture damage: %+v hints%v", what, r, dam)
		}
	}
}

func TestNavigationInteractionExecution(t *testing.T) {
	goText(t)
	for _, scale := range []float32{1, 1.25, 2} {
		for _, mode := range []MinimapSize{MinimapProportional, MinimapFit, MinimapFill} {
			t.Run(fmt.Sprintf("scale%v/%s", scale, mode), func(t *testing.T) {
				st := NewState(strings.Repeat("plain line\n", 39)+"last", "a.txt", theme.Base("vs"))
				defer st.Close()
				st.Options.Minimap.Size = mode
				st.Options.OccurrencesHighlight = OccurrencesOff
				h := newHintHost(t, st, geom.Size{W: 640, H: 300}, scale)
				h.focusAt(2)
				slider := h.vw.vs.minimapSlider
				p := geom.Pt(slider.X+5, slider.Y+slider.H/2)
				h.r.HandleEvent(input.Pointer{Kind: input.Move, Pos: p})
				h.requirePartialStep("slider hover")
				beforeGrab := append([]byte(nil), h.dev.Target().Pix...)
				viewBeforeGrab := st.Viewport
				if h.vw.c.Dragging(h.vw.box.ID()) {
					t.Fatal("unexpected capture before press")
				}
				h.r.HandleEvent(input.Pointer{Kind: input.Press, Pos: p})
				h.requireCaptureStep("slider grab (framework active damage)")
				if !h.vw.c.Dragging(h.vw.box.ID()) || st.Viewport != viewBeforeGrab || bytes.Equal(beforeGrab, h.dev.Target().Pix) {
					t.Fatal("grab must change thumb pixels/capture, not scroll the body")
				}
				// Pick a tiny drag whose DOCUMENT displacement is exactly two device
				// pixels. Arbitrary subpixel scroll copies can legitimately fall back.
				unit, _, _ := h.vw.miniCanvas()
				dy := float32(2) / scale
				p.Y += dy / st.Viewport.LineHeight * unit
				h.r.HandleEvent(input.Pointer{Kind: input.Move, Pos: p})
				h.requirePartialStep("small aligned slider drag")
				if math.Abs(st.Viewport.scrollY()-float64(dy)) > .001 {
					t.Fatalf("drag did not scroll expected %v pixels: %v", dy, st.Viewport.scrollY())
				}
				beforeRelease := append([]byte(nil), h.dev.Target().Pix...)
				viewBeforeRelease := st.Viewport
				h.r.HandleEvent(input.Pointer{Kind: input.Release, Pos: p})
				h.requireCaptureStep("slider release (framework active damage)")
				if h.vw.c.Dragging(h.vw.box.ID()) || st.Viewport != viewBeforeRelease || bytes.Equal(beforeRelease, h.dev.Target().Pix) {
					t.Fatal("release must change thumb pixels/capture, not scroll the body")
				}
				stable := append([]byte(nil), h.dev.Target().Pix...)
				h.r.HandleEvent(input.Pointer{Kind: input.Move, Pos: p})
				h.tracedStep("unchanged pointer")
				// An unchanged frame may be omitted or explicitly rendered;
				// it has no required Execute count, but must preserve pixels.
				if !bytes.Equal(stable, h.dev.Target().Pix) {
					t.Fatal("unchanged pointer altered pixels")
				}
			})
		}
	}
}

func TestStickyFractionalPushReallyPartial(t *testing.T) {
	goText(t)
	for _, scale := range []float32{1, 1.25, 2} {
		t.Run(fmt.Sprintf("scale%v", scale), func(t *testing.T) {
			st := NewState(strings.Repeat("scope text\n", 40), "a.txt", theme.Base("vs"))
			defer st.Close()
			st.Options.OccurrencesHighlight = OccurrencesOff
			st.setFoldingProvider(&testProvider{rs: []FoldingRange{{Start: 0, End: 30}, {Start: 3, End: 8}}})
			h := newHintHost(t, st, geom.Size{W: 640, H: 300}, scale)
			h.focusAt(2)
			lh := st.Viewport.LineHeight
			st.Viewport.setScrollY(float64(7.5*lh), st.viewCount())
			h.frame()
			beforeY := h.vw.vs.stickyY[1]
			beforeClip := h.vw.vs.stickyClips[1]
			// Fractional logical positions, but a one-device-pixel displacement.
			delta := float32(1) / scale
			st.ScrollBy(delta, 0)
			h.requirePartialStep("fractional sticky push and clip")
			afterY := h.vw.vs.stickyY[1]
			afterClip := h.vw.vs.stickyClips[1]
			if abs32((beforeY-afterY)-delta) > .001 || abs32((beforeClip.H-afterClip.H)-delta) > .001 {
				t.Fatalf("header did not push/clip by %v: y %v -> %v clip %v -> %v", delta, beforeY, afterY, beforeClip, afterClip)
			}
		})
	}
}

// Extract only pixels inside fixed header clips. A body scroll cannot make
// these assertions pass, unlike a difference of two whole-window images.
func headerPixels(img *image.RGBA, r geom.Rect) []byte {
	box := image.Rect(int(math.Ceil(float64(r.X))), int(math.Ceil(float64(r.Y))), int(math.Floor(float64(r.Right()))), int(math.Floor(float64(r.Bottom())))).Intersect(img.Bounds())
	var out []byte
	for y := box.Min.Y; y < box.Max.Y; y++ {
		a := img.PixOffset(box.Min.X, y)
		out = append(out, img.Pix[a:a+box.Dx()*4]...)
	}
	return out
}
func TestStickyWrappedFoldedHeaderClips(t *testing.T) {
	goText(t)
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = "body"
	}
	lines[0] = "outer(" + strings.Repeat("argument,", 10) + ")"
	lines[3] = "inner(argument)"
	lines[4] = "block"
	st := NewState(strings.Join(lines, "\n"), "a.txt", theme.Base("vs"))
	defer st.Close()
	st.Options.WordWrap = WrapColumn
	st.Options.WordWrapColumn = 10
	st.Options.LineNumbers = LineNumbersOff
	st.setFoldingProvider(&testProvider{rs: []FoldingRange{{Start: 0, End: 30}, {Start: 3, End: 8}, {Start: 4, End: 6}}})
	st.MoveCaret(st.Snapshot().LineStart(4), false)
	st.Execute("editor.fold", nil)
	st.Viewport.TopLine, st.Viewport.OffsetY = 0, 0
	d, vw := viewDriver(st, 640, 300, nil)
	d.Frame()
	w := *vw
	lh := st.Viewport.LineHeight
	if st.vl.rowsOf(0) < 5 || len(st.Folded()) != 1 {
		t.Fatal("fixture must independently wrap the outer header and hide nested rows")
	}
	entry := float32(st.vl.viewOf(3))*lh - lh*.5
	st.Viewport.setScrollY(float64(entry), st.viewCount())
	d.Frame()
	w = *vw
	if len(w.vs.stickyLines) != 2 || w.vs.stickyLines[0] != 0 || w.vs.stickyLines[1] != 3 || abs32(w.vs.stickyY[1]-lh) > .001 {
		t.Fatalf("wrapped entry geometry %v %v", w.vs.stickyLines, w.vs.stickyY)
	}
	end := float32(st.vl.viewOf(8)+st.vl.rowsOf(8)) * lh
	st.Viewport.setScrollY(float64(end-lh*1.5), st.viewCount())
	raw := d.Frame()
	before := image.NewRGBA(raw.Bounds())
	copy(before.Pix, raw.Pix)
	w = *vw
	oldClip := w.vs.stickyClips[1]
	if abs32(w.vs.stickyY[0]) > .001 || abs32(w.vs.stickyY[1]-lh*.5) > .001 || abs32(oldClip.Y-lh) > .001 || abs32(oldClip.H-lh*.5) > .001 {
		t.Fatalf("wrapped/folded push geometry y=%v clips=%v", w.vs.stickyY, w.vs.stickyClips)
	}
	outer := w.vs.stickyClips[0]
	st.ScrollBy(lh*.25, 0)
	after := d.Frame()
	w = *vw
	if len(st.Folded()) != 1 || abs32(w.vs.stickyY[1]-lh*.25) > .001 || abs32(w.vs.stickyClips[1].Y-lh) > .001 || abs32(w.vs.stickyClips[1].H-lh*.25) > .001 {
		t.Fatalf("wrapped/folded clip progression y=%v clips=%v", w.vs.stickyY, w.vs.stickyClips)
	}
	if !bytes.Equal(headerPixels(before, outer), headerPixels(after, outer)) {
		t.Fatal("outer fixed header changed while only inner pushes")
	}
	common := oldClip.Intersect(w.vs.stickyClips[1]).Intersect(w.vs.text)
	if common.Empty() || bytes.Equal(headerPixels(before, common), headerPixels(after, common)) {
		t.Fatal("inner-header-only pixels did not move; body changes cannot satisfy this test")
	}
}

func TestMinimapScrolledSampleClick(t *testing.T) {
	goText(t)
	for _, mode := range []MinimapSize{MinimapProportional, MinimapFit, MinimapFill} {
		t.Run(string(mode), func(t *testing.T) {
			st := NewState(strings.Repeat("a", 350)+"\n"+strings.Repeat("tail\n", 2000), "a.txt", theme.Base("vs"))
			defer st.Close()
			st.Options.WordWrap = WrapColumn
			st.Options.WordWrapColumn = 10
			st.Options.Minimap.Size = mode
			st.setFoldingProvider(&testProvider{rs: []FoldingRange{{Start: 1, End: 600}}})
			st.MoveCaret(st.Snapshot().LineStart(1), false)
			st.Execute("editor.fold", nil)
			st.Viewport.TopLine, st.Viewport.OffsetY = 0, 0
			d, vw := viewDriver(st, 640, 300, nil)
			d.Frame()
			st.ScrollBy(333*st.Viewport.LineHeight+.5, 0)
			d.Frame()
			w := *vw
			if st.Viewport.TopLine != 333 {
				t.Fatalf("fixture scroll %v", st.Viewport)
			}
			if mode == MinimapProportional && w.vs.miniRows[0].vrow == 0 {
				t.Fatal("miniature window did not move before test click")
			}
			sample := w.vs.miniRows[100]
			d.Click(geom.Pt(w.vs.minimapRect.X+4, sample.y+sample.h*.25))
			d.Frame()
			line, _ := st.vl.find(st.Viewport.TopLine)
			if st.Viewport.TopLine != sample.vrow || line != sample.line {
				t.Fatalf("scrolled preview sample line%d row%d -> line%d row%d", sample.line, sample.vrow, line, st.Viewport.TopLine)
			}
			if len(st.Folded()) != 1 {
				t.Fatal("scrolled miniature click unfolded hidden lines")
			}
		})
	}
}
