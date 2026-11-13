package fake

import (
	"errors"
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestCanvasHWND(t *testing.T) {
	b := New()
	_, cv, err := b.NewCanvasSurface(1, platform.SurfaceOptions{Width: 100, Height: 80}, platform.SurfaceExtras{}, &nopSink{})
	if err != nil {
		t.Fatal(err)
	}
	ch := cv.(platform.CanvasHWND)
	if _, err := ch.UseGPUWindow(); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatalf("disabled: err = %v, want ErrUnsupported", err)
	}
	b.EnableCanvasHWND(0)
	h1, err := ch.UseGPUWindow()
	if err != nil || h1 != FakeHWND {
		t.Fatalf("enabled: %#x, %v", h1, err)
	}
	b.EnableCanvasHWND(0x1234) // the first handle sticks
	if h2, err := ch.UseGPUWindow(); err != nil || h2 != h1 {
		t.Fatalf("second call: %#x, %v; want %#x", h2, err, h1)
	}
	c := cv.(*Canvas)
	if c.GPUWindowCalls() != 3 || c.GPUWindow() != h1 || c.MetalCalls() != 0 {
		t.Fatalf("calls=%d hwnd=%#x metal=%d", c.GPUWindowCalls(), c.GPUWindow(), c.MetalCalls())
	}
}

func TestAttachedCanvasHWNDUnsupported(t *testing.T) {
	b := New()
	b.EnableCanvasHWND(0)
	if _, err := b.NewSurface(2, platform.SurfaceOptions{Width: 100, Height: 80}); err != nil {
		t.Fatal(err)
	}
	cv, err := b.AttachCanvas(2, platform.CanvasLayer{}, &nopSink{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cv.(platform.CanvasHWND).UseGPUWindow(); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatalf("attached: err = %v, want ErrUnsupported", err)
	}
	if cv.(*AttachedCanvas).GPUWindowCalls() != 1 {
		t.Fatal("call not recorded")
	}
}
