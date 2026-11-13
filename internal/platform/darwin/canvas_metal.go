//go:build darwin && !ios

package darwin

// GPU presentation (platform.CanvasMetal). UseMetalLayer turns the canvas
// view from layer-backed (AppKit owns a plain CALayer and calls updateLayer
// when the view needs display) into layer-hosting: the view's layer is a
// CAMetalLayer we created, set with setLayer: before setWantsLayer:YES as
// AppKit documents for layer-hosting views. AppKit then never draws the
// view or calls updateLayer, so setNeedsDisplay (canvas.go) runs the frame
// itself via metalDisplay. Every scheduling path keeps going through
// setNeedsDisplay, so coalescing, the display link, InvalidateAt and the
// synchronous redraw on resize are the same in both modes:
//
//	CPU:   tick/resize/timer → setNeedsDisplay → setNeedsDisplay: → updateLayer → frame → CanvasFrame → Present(img)
//	Metal: tick/resize/timer → setNeedsDisplay → metalDisplay → frame → CanvasFrame → (GPU present) → Present(nil)
//
// AppKit still keeps a hosted layer's frame on the view's bounds; the
// contentsScale is ours (syncMetalLayer, from checkGeometry). The GPU side
// (device, pixel format, drawableSize, rendering) belongs to the caller.

import (
	"errors"

	"github.com/tangoui/tango/internal/platform"
)

var (
	_ platform.CanvasMetal = (*canvas)(nil)
	_ platform.CanvasMetal = (*overlay)(nil)
)

// UseMetalLayer implements platform.CanvasMetal.
func (c *canvas) UseMetalLayer() (uintptr, error) {
	if !isMainThread() {
		return 0, errors.New("tango/darwin: UseMetalLayer must run on the main thread")
	}
	if c.closed {
		return 0, errCanvasClosed
	}
	if c.metalLayer != 0 {
		return uintptr(c.metalLayer), nil
	}
	if a := c.s.ext.appearance; a.Transparent || a.Material != platform.MaterialNone {
		return 0, platform.ErrUnsupported // an opaque CAMetalLayer cannot show the window background
	}
	if !hasClass("CAMetalLayer") {
		return 0, platform.ErrUnsupported
	}
	withPool(func() {
		layer := send(class("CAMetalLayer"), "layer") // autoreleased; the view retains it
		if layer == 0 {
			return
		}
		ca := class("CATransaction")
		send(ca, "begin")
		send(ca, "setDisableActions:", 1)
		send(layer, "setActions:", uintptr(noActions()))
		send(layer, "setOpaque:", 1)
		msgSetFloat(layer, sel("setContentsScale:"), c.scale)
		send(c.view, "setLayer:", uintptr(layer))
		send(c.view, "setWantsLayer:", 1)
		c.metalLayer = layer
		c.setGravity() // an old-size drawable stays top-left during live resize
		send(ca, "commit")
	})
	if c.metalLayer == 0 {
		return 0, errors.New("tango/darwin: UseMetalLayer: cannot create a CAMetalLayer")
	}
	// The layer starts empty and AppKit will not display the view any more:
	// ask for the first GPU frame (from the run loop, never re-entrantly).
	c.enqueue(func(c *canvas) { c.qFrame = true })
	return uintptr(c.metalLayer), nil
}

// UseMetalLayer implements platform.CanvasMetal: an overlay shares its
// window with a WebView (and may be transparent or pass-through), so it
// stays on the CPU path.
func (o *overlay) UseMetalLayer() (uintptr, error) { return 0, platform.ErrUnsupported }

// metalDisplay is setNeedsDisplay for a layer-hosting view: it runs the
// frame now. A request made while a frame runs (the sink resized the
// window, say) waits for the next display refresh, as AppKit would make
// it wait for the next display pass.
func (c *canvas) metalDisplay() {
	if c.inFrame {
		c.requestFrame()
		return
	}
	withPool(c.frame) // drawables are autoreleased: one pool per frame
}

// syncMetalLayer keeps the hosted layer's contentsScale on the backing
// scale. Called from checkGeometry before CanvasResized is reported.
func (c *canvas) syncMetalLayer() {
	if c.metalLayer == 0 {
		return
	}
	withPool(func() {
		ca := class("CATransaction")
		send(ca, "begin")
		send(ca, "setDisableActions:", 1)
		msgSetFloat(c.metalLayer, sel("setContentsScale:"), c.scale)
		send(ca, "commit")
	})
}

// noActions is a layer actions dictionary that disables the implicit
// animations of the properties AppKit and the presenter change.
func noActions() id {
	null := send(class("NSNull"), "null")
	d := send(class("NSMutableDictionary"), "dictionary")
	for _, k := range []string{"bounds", "position", "frame", "anchorPoint", "contents", "contentsScale",
		"contentsRect", "hidden", "opacity", "transform", "sublayers", "onOrderIn", "onOrderOut"} {
		send(d, "setObject:forKey:", uintptr(null), uintptr(nsString(k)))
	}
	return d
}

// presentLayer is the layer Present(img) sets the CGImage on: the view's
// layer in CPU mode. In Metal mode setting contents on the CAMetalLayer
// itself would hide every drawable presented later, so the image goes to a
// sublayer above the drawables, shown until the next Present(nil).
func (c *canvas) presentLayer() id {
	if c.metalLayer == 0 {
		return send(c.view, "layer")
	}
	const kCALayerWidthSizable, kCALayerHeightSizable = 1 << 1, 1 << 4
	ca := class("CATransaction")
	send(ca, "begin")
	send(ca, "setDisableActions:", 1)
	if c.cpuLayer == 0 {
		c.cpuLayer = send(class("CALayer"), "layer") // owned by metalLayer once added
		send(c.cpuLayer, "setActions:", uintptr(noActions()))
		send(c.cpuLayer, "setOpaque:", 1)
		msgSetRect(c.cpuLayer, sel("setFrame:"), msgRect(c.metalLayer, sel("bounds")))
		send(c.cpuLayer, "setAutoresizingMask:", kCALayerWidthSizable|kCALayerHeightSizable)
		send(c.metalLayer, "addSublayer:", uintptr(c.cpuLayer))
		if g := gravityFor(c.cpuLayer); g != 0 {
			send(c.cpuLayer, "setContentsGravity:", g)
		}
	}
	send(c.cpuLayer, "setHidden:", 0)
	send(ca, "commit")
	return c.cpuLayer
}

// hideCPULayer hides (and empties) the Present(img) sublayer once the GPU
// presents again.
func (c *canvas) hideCPULayer() {
	if c.cpuLayer == 0 || sendBool(c.cpuLayer, "isHidden") {
		return
	}
	withPool(func() {
		ca := class("CATransaction")
		send(ca, "begin")
		send(ca, "setDisableActions:", 1)
		send(c.cpuLayer, "setHidden:", 1)
		send(c.cpuLayer, "setContents:", 0)
		send(ca, "commit")
	})
}

// gravityFor is setGravity's top-left gravity for any layer (in its own,
// possibly flipped, coordinates).
func gravityFor(layer id) uintptr {
	if kCAGravityTopLeft == 0 || kCAGravityBottomLeft == 0 {
		return 0
	}
	if sendBool(layer, "contentsAreFlipped") {
		return kCAGravityBottomLeft
	}
	return kCAGravityTopLeft
}
