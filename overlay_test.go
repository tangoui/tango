package tango

import (
	"errors"
	"testing"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/fake"
)

type overlayBackend struct {
	*fake.Backend
	supported bool
}

func (b overlayBackend) SupportsOverlay() bool { return b.supported }

func TestOverlayUnsupported(t *testing.T) {
	for _, declared := range []bool{false, true} {
		fb := fake.New()
		var b platform.Backend = fb
		if declared {
			b = overlayBackend{Backend: fb}
		}
		a := newWithBackend(Options{}, b)
		runApp(t, a)
		opts := SurfaceOptions{Kind: KindOverlay}
		if _, err := a.NewSurface(opts); !errors.Is(err, ErrUnsupported) {
			t.Fatalf("NewSurface: %v", err)
		}
		err := Await(a, func() error {
			_, _, err := a.createCanvasSurface(opts, nil)
			return err
		})
		if !errors.Is(err, ErrUnsupported) {
			t.Fatalf("canvas: %v", err)
		}
		if fb.Surface(1) != nil || fb.Surface(2) != nil {
			t.Fatal("unsupported overlay reached the backend")
		}
	}
}

func TestOverlaySupported(t *testing.T) {
	fb := fake.New()
	a := newWithBackend(Options{}, overlayBackend{Backend: fb, supported: true})
	runApp(t, a)
	s, err := a.NewSurface(SurfaceOptions{Kind: KindOverlay})
	if err != nil {
		t.Fatal(err)
	}
	if ex, ok := fb.Surface(s.ID()).Extras(); !ok || ex.Kind != platform.KindOverlay {
		t.Fatalf("extras = %+v, present = %t", ex, ok)
	}
}

func TestOverlayIgnoresParent(t *testing.T) {
	fb := fake.New()
	a := newWithBackend(Options{}, overlayBackend{Backend: fb, supported: true})
	runApp(t, a)
	parent, err := a.NewSurface(SurfaceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := a.NewSurface(SurfaceOptions{Kind: KindOverlay, Parent: parent})
	if err != nil {
		t.Fatal(err)
	}
	if ex, _ := fb.Surface(s.ID()).Extras(); ex.Parent != 0 {
		t.Fatalf("overlay parent = %d", ex.Parent)
	}
}
