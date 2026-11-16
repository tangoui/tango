//go:build darwin && !ios && arm64

package darwin

import (
	"errors"
	"github.com/tangoui/tango/internal/platform"
)

var _ platform.CanvasCoreGraphics = (*canvas)(nil)

func (c *canvas) UseCoreGraphicsLayer() (uintptr, error) {
	if !isMainThread() {
		return 0, errors.New("tango/darwin: UseCoreGraphicsLayer must run on the main thread")
	}
	if c.closed {
		return 0, errCanvasClosed
	}
	if c.nativeLayer != 0 {
		return uintptr(c.nativeLayer), nil
	}
	if c.metalLayer != 0 {
		return 0, platform.ErrUnsupported
	}
	if a := c.s.ext.appearance; a.Transparent || a.Material != platform.MaterialNone {
		return 0, platform.ErrUnsupported
	}
	withPool(func() {
		layer := send(class("CALayer"), "layer")
		if layer == 0 {
			return
		}
		ca := class("CATransaction")
		send(ca, "begin")
		send(ca, "setDisableActions:", 1)
		send(layer, "setActions:", uintptr(noActions()))
		send(layer, "setGeometryFlipped:", 1)
		msgSetFloat(layer, sel("setContentsScale:"), c.scale)
		c.tiles = nil
		send(c.view, "setLayer:", uintptr(layer))
		send(c.view, "setWantsLayer:", 1)
		c.nativeLayer = layer
		c.setGravity()
		send(ca, "commit")
	})
	if c.nativeLayer == 0 {
		return 0, errors.New("tango/darwin: cannot create native content layer")
	}
	c.enqueue(func(c *canvas) { c.qFrame = true })
	return uintptr(c.nativeLayer), nil
}

// WebView overlays retain their CPU hit-testing and transparency path.
func (*overlay) UseCoreGraphicsLayer() (uintptr, error) { return 0, platform.ErrUnsupported }
