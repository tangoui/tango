//go:build !linux

package gl

import (
	"image"

	"github.com/tangoui/tango/ui/paint"
)

// NewPresenter presents into a GTK 3 GtkGLArea, which exists only on
// Linux: here it always fails with ErrUnsupported. (On darwin the OpenGL
// device serves tests only; applications render with ui/gpu/metal.)
func NewPresenter(glarea uintptr) (r paint.Renderer, present func(size image.Point, scale float32) error, err error) {
	return nil, nil, ErrUnsupported
}
