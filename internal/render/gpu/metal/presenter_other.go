//go:build !darwin

package metal

import (
	"fmt"
	"image"

	"github.com/tangoui/tango/ui/paint"
)

// NewPresenter attaches a GPU renderer to a CAMetalLayer obtained from
// platform.CanvasMetal.UseMetalLayer. Main thread only. On non-darwin it
// returns an error wrapping ErrUnsupported.
func NewPresenter(layer uintptr) (r paint.Renderer, present func(size image.Point, scale float32) error, err error) {
	return nil, nil, fmt.Errorf("NewPresenter: %w", ErrUnsupported)
}
