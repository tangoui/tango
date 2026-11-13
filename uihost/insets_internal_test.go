package uihost

import (
	"image"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// plainCanvas is a platform.Canvas without optional interfaces, like a
// desktop canvas.
type plainCanvas struct{}

func (plainCanvas) Size() (float32, float32)     { return 100, 100 }
func (plainCanvas) Scale() float32               { return 1 }
func (plainCanvas) Invalidate()                  {}
func (plainCanvas) InvalidateAt(time.Time)       {}
func (plainCanvas) SetCursor(input.Cursor)       {}
func (plainCanvas) SetTextInput(bool, geom.Rect) {}
func (plainCanvas) Present(*image.RGBA) error    { return nil }

type insetCanvas struct {
	plainCanvas
	safe, kb geom.Insets
}

func (c insetCanvas) SafeAreaInsets() geom.Insets { return c.safe }
func (c insetCanvas) KeyboardInset() geom.Insets  { return c.kb }

func TestWindowInsets(t *testing.T) {
	tests := []struct {
		name     string
		w        *window
		safe, kb geom.Insets
	}{
		{"canvas without CanvasInsets", &window{cv: plainCanvas{}}, geom.Insets{}, geom.Insets{}},
		{"canvas with CanvasInsets",
			&window{cv: insetCanvas{safe: geom.Insets{Top: 59, Bottom: 34}, kb: geom.Insets{Bottom: 291}}},
			geom.Insets{Top: 59, Bottom: 34}, geom.Insets{Bottom: 291}},
	}
	for _, tt := range tests {
		if got := tt.w.SafeAreaInsets(); got != tt.safe {
			t.Errorf("%s: SafeAreaInsets %v, want %v", tt.name, got, tt.safe)
		}
		if got := tt.w.KeyboardInset(); got != tt.kb {
			t.Errorf("%s: KeyboardInset %v, want %v", tt.name, got, tt.kb)
		}
	}
}
