package uihost

import (
	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/paint"
	"image"
	"time"
)

// Nil on platforms whose native layer ABI has not been validated.
var nativeNewPresenter presenterFunc = platformNativePresenter

type nativeRendererState struct {
	canvas      platform.Canvas
	content     ui.ContentKind // ContentData ignores keyboard and animation promotion
	lastEnd     time.Time
	start       time.Time
	slow, full  int
	anim        int
	inputDriven bool // the next presented frame follows input or a resize
	ready       bool
}

const (
	nativeIdleGap    = 100 * time.Millisecond
	nativeAnimFrames = 60 // about a second at 60 Hz
)

func (s *nativeRendererState) interrupted(now time.Time) bool {
	return s.lastEnd.IsZero() || now.Before(s.lastEnd) || now.Sub(s.lastEnd) > nativeIdleGap
}

func (s *nativeRendererState) record(work time.Duration, full bool, start, end time.Time) bool {
	if s.interrupted(start) {
		s.slow, s.full, s.anim = 0, 0, 0
	}
	if s.content != ui.ContentData && !s.inputDriven {
		s.anim++
	}
	s.inputDriven = false
	if work > 8*time.Millisecond {
		s.slow++
	} else {
		s.slow = 0
	}
	if full {
		s.full++
	} else {
		s.full = 0
	}
	s.lastEnd = end
	// Sticky: a keyboardInput request survives frames recorded before Render.
	s.ready = s.ready || s.slow >= 6 || s.full >= 6
	return s.ready
}

// noteInput runs on the renderer's UI thread. The keyboard and sustained
// cost conditions can still request promotion on an input-driven frame.
func (s *nativeRendererState) noteInput() {
	if s != nil {
		s.anim = 0
		s.inputDriven = true
	}
}

// maybePromoteAnimation runs just before the window's next frame. Wait for
// that frame before requesting Metal: an idle gap or input can end the
// streak after the last presentation, and promotion must not wake a window.
func (s *nativeRendererState) maybePromoteAnimation() {
	if s != nil && s.anim >= nativeAnimFrames && !s.interrupted(time.Now()) {
		s.ready = true
	}
}

func startNative(cv platform.Canvas, forced bool, content ui.ContentKind) *gpuRenderer {
	if nativeNewPresenter == nil {
		return nil
	}
	target, ok := cv.(platform.CanvasCoreGraphics)
	if !ok {
		return nil
	}
	layer, err := target.UseCoreGraphicsLayer()
	if err != nil {
		return nil
	}
	g, err := buildNative(cv, forced, content, layer)
	if err != nil {
		// Metal can take over the layer (as in promoteNative); a large or
		// typing window would move there anyway. Both targets show CPU frames.
		gpuLogf("tango: native renderer unavailable, trying Metal: %v", err)
		if g := startGPU(cv, false); g != nil {
			return g
		}
		return &gpuRenderer{cpu: paint.NewCPU()}
	}
	return g
}

func buildNative(cv platform.Canvas, forced bool, content ui.ContentKind, layer uintptr) (*gpuRenderer, error) {
	r, present, err := nativeNewPresenter(layer)
	if err != nil {
		return nil, err
	}
	g := &gpuRenderer{gpu: r, present: present, nativeMode: true}
	if !forced {
		g.native = &nativeRendererState{canvas: cv, content: content}
	}
	return g, nil
}
func (g *gpuRenderer) promoteNative() {
	cv := g.native.canvas
	g.native = nil // attempt once, including a failed target/presenter
	promoted := startGPUOn(g.main, cv, false)
	g.releaseGPU()
	if promoted != nil && promoted.gpu != nil {
		g.gpu, g.present = promoted.gpu, promoted.present
		g.attachTimeline() // TANGO_FRAME_TRACE: the new Metal device's timeline
		return
	}
	// Target acquisition may already have replaced the native layer with Metal.
	// Both hosted targets support a visible CPU frame on this same Render call.
	g.cpu = paint.NewCPU()
}
func (g *gpuRenderer) nativePresented() {
	if g.native == nil {
		return
	}
	if m, ok := g.gpu.(interface{ FrameMetrics() (time.Duration, bool) }); ok {
		work, full := m.FrameMetrics()
		g.native.record(work, full, g.native.start, time.Now())
	}
}

// InvalidateImage forwards the display-list contract through backend switches.
func (g *gpuRenderer) InvalidateImage(img image.Image) {
	if g.closed {
		return
	}
	if g.cpu != nil {
		g.cpu.InvalidateImage(img)
	}
	if g.big != nil {
		g.big.InvalidateImage(img)
	}
	if r, ok := g.gpu.(interface{ InvalidateImage(image.Image) }); ok {
		r.InvalidateImage(img)
	}
}

// isKeyboardInput reports input that means the user is typing into the
// window: a non-modifier key press (auto-repeats and shortcuts included),
// committed text, or an IME composition. Scrolling, pointers, touches,
// focus changes, key releases and lone modifiers or Caps Lock are not.
func isKeyboardInput(ev input.Event) bool {
	switch ev := ev.(type) {
	case input.KeyEvent:
		return ev.Down && ev.Key != 0 && ev.Key.Modifier() == 0 && ev.Key != input.KeyCapsLock
	case input.TextInput:
		return ev.Text != ""
	case input.Composition:
		return ev.Text != ""
	}
	return false
}

func isUserDrivenInput(ev input.Event) bool {
	switch ev := ev.(type) {
	case input.Pointer, input.Scroll, input.TouchEvent, input.TextInput, input.Composition:
		return true
	case input.KeyEvent:
		return ev.Down
	}
	return false
}

// keyboardInput promotes a default native window to Metal on its next
// Render: Metal's input-to-photon latency is about two frames lower. It does
// nothing for a nil renderer, forced native, a window already promoted or one
// whose promotion failed (native is nil in all of those), or a data window
// (ui.ContentData), which keyboard browsing leaves on native layers and only
// sustained frame cost promotes.
func (g *gpuRenderer) keyboardInput() {
	if g != nil && g.native != nil && g.native.content != ui.ContentData {
		g.native.ready = true
	}
}
