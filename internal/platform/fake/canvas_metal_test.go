package fake

import (
	"errors"
	"testing"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/input"
)

type nopSink struct{ frames int }

func (s *nopSink) CanvasFrame()                   { s.frames++ }
func (s *nopSink) CanvasInput(input.Event)        {}
func (s *nopSink) CanvasResized(w, h, sc float32) {}
func (s *nopSink) CanvasFocus(bool)               {}

func TestCanvasMetal(t *testing.T) {
	b := New()
	_, cv, err := b.NewCanvasSurface(1, platform.SurfaceOptions{Width: 100, Height: 80}, platform.SurfaceExtras{}, &nopSink{})
	if err != nil {
		t.Fatal(err)
	}
	cm, ok := cv.(platform.CanvasMetal)
	if !ok {
		t.Fatal("fake Canvas does not implement platform.CanvasMetal")
	}
	if _, err := cm.UseMetalLayer(); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatalf("disabled: err = %v, want ErrUnsupported", err)
	}
	b.EnableCanvasMetal(0)
	l1, err := cm.UseMetalLayer()
	if err != nil || l1 != FakeMetalLayer {
		t.Fatalf("enabled: %#x, %v", l1, err)
	}
	b.EnableCanvasMetal(0x1234) // the first handle sticks
	if l2, err := cm.UseMetalLayer(); err != nil || l2 != l1 {
		t.Fatalf("second call: %#x, %v; want %#x", l2, err, l1)
	}
	c := cv.(*Canvas)
	if c.MetalCalls() != 3 || c.MetalLayer() != l1 {
		t.Fatalf("calls=%d layer=%#x", c.MetalCalls(), c.MetalLayer())
	}
	if err := cv.Present(nil); err != nil {
		t.Fatalf("Present(nil) in Metal mode: %v", err)
	}
}

func TestAttachedCanvasMetalUnsupported(t *testing.T) {
	b := New()
	b.EnableCanvasMetal(0)
	if _, err := b.NewSurface(2, platform.SurfaceOptions{Width: 100, Height: 80}); err != nil {
		t.Fatal(err)
	}
	cv, err := b.AttachCanvas(2, platform.CanvasLayer{}, &nopSink{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cv.(platform.CanvasMetal).UseMetalLayer(); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatalf("attached: err = %v, want ErrUnsupported", err)
	}
	if cv.(*AttachedCanvas).MetalCalls() != 1 {
		t.Fatal("call not recorded")
	}
}
