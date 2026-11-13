package fake

import (
	"errors"
	"testing"

	"github.com/tangoui/tango/internal/platform"
)

func TestFakeCanvasGL(t *testing.T) {
	b := New()
	_, cv, err := b.NewCanvasSurface(1, platform.SurfaceOptions{Width: 100, Height: 80}, platform.SurfaceExtras{}, &nopSink{})
	if err != nil {
		t.Fatal(err)
	}
	cg := cv.(platform.CanvasGL)
	if _, err := cg.UseGLArea(); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatalf("disabled: err = %v, want ErrUnsupported", err)
	}
	b.EnableCanvasGL(0)
	a1, err := cg.UseGLArea()
	if err != nil || a1 != FakeGLArea {
		t.Fatalf("enabled: %#x, %v", a1, err)
	}
	b.EnableCanvasGL(0x1234) // the first handle sticks
	if a2, err := cg.UseGLArea(); err != nil || a2 != a1 {
		t.Fatalf("second call: %#x, %v; want %#x", a2, err, a1)
	}
	c := cv.(*Canvas)
	if c.GLAreaCalls() != 3 || c.GLArea() != a1 || c.MetalCalls() != 0 || c.GPUWindowCalls() != 0 {
		t.Fatalf("calls=%d area=%#x metal=%d hwnd=%d", c.GLAreaCalls(), c.GLArea(), c.MetalCalls(), c.GPUWindowCalls())
	}
}

func TestFakeAttachedCanvasGLUnsupported(t *testing.T) {
	b := New()
	b.EnableCanvasGL(0)
	if _, err := b.NewSurface(2, platform.SurfaceOptions{Width: 100, Height: 80}); err != nil {
		t.Fatal(err)
	}
	cv, err := b.AttachCanvas(2, platform.CanvasLayer{}, &nopSink{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cv.(platform.CanvasGL).UseGLArea(); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatalf("attached: err = %v, want ErrUnsupported", err)
	}
	if cv.(*AttachedCanvas).GLAreaCalls() != 1 {
		t.Fatal("call not recorded")
	}
}

func TestFakeCanvasGLRelease(t *testing.T) {
	b := New()
	b.EnableCanvasGL(0)
	_, cv, err := b.NewCanvasSurface(1, platform.SurfaceOptions{Width: 100, Height: 80}, platform.SurfaceExtras{}, &nopSink{})
	if err != nil {
		t.Fatal(err)
	}
	c := cv.(*Canvas)
	c.ReleaseGLArea() // before UseGLArea: no-op
	if n, done := c.GLAreaReleases(); n != 1 || done {
		t.Fatalf("early release: calls %d done %v", n, done)
	}
	if a, err := c.UseGLArea(); err != nil || a != FakeGLArea {
		t.Fatalf("UseGLArea: %#x, %v", a, err)
	}
	c.ReleaseGLArea()
	if n, done := c.GLAreaReleases(); n != 2 || !done || c.GLArea() != 0 {
		t.Fatalf("release: calls %d done %v area %#x", n, done, c.GLArea())
	}
	if _, err := c.UseGLArea(); !errors.Is(err, platform.ErrUnsupported) {
		t.Fatalf("UseGLArea after release: %v, want ErrUnsupported", err)
	}
}
