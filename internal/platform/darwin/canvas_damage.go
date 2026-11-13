//go:build darwin && !ios

package darwin

// Partial frames (platform.CanvasDamagePresenter) on the CPU path. A whole
// frame (Present) is one CGImage set as the contents of the view's layer.
// PresentDamage leaves it alone and covers the changed parts with tile
// sublayers instead: the frame is cut into canvasTileSize-pixel tiles and
// every tile a dirty rectangle touches gets a CGImage of that tile of the
// new frame, so Core Animation uploads only those tiles. A tile stays
// shown (above the base contents, which are older there) until the next
// whole frame, which hides every tile in the same transaction as it
// replaces the base contents.
//
// The whole frame is presented instead when the partial path cannot be
// trusted: no whole frame yet or one of another size (the grid belongs to
// the last whole frame), Metal mode (the view's layer is a CAMetalLayer and
// frames go through the GPU or its CPU sublayer), a damaged tile with
// non-opaque pixels (a tile cannot clear what the base shows beneath it),
// or a damage list that does not pay off (platform.DamageRects, or tiles
// covering more than half of the frame).

import (
	"image"

	"github.com/tangoui/tango/internal/platform"
)

var _ platform.CanvasDamagePresenter = (*canvas)(nil)

// canvasDamageMaxRects is the longest dirty list presented partially.
const canvasDamageMaxRects = 64

// canvasTiles is the tile state of a CPU canvas. Main thread.
type canvasTiles struct {
	grid    canvasTileGrid // of the last whole frame
	layers  []id           // per tile, 0 until first used; sublayers of the view's layer
	shown   []int          // tiles currently visible
	scratch []byte         // one tile's pixels
}

// tilesReset records a whole frame of size px: every tile is hidden (and
// emptied); a grid of another size drops its layers. Inside the
// CATransaction of the whole frame. Not in Metal mode.
func (c *canvas) tilesReset(px image.Point) {
	if c.metalLayer != 0 {
		return
	}
	t := c.tiles
	if t == nil {
		c.tiles = &canvasTiles{grid: newCanvasTileGrid(px)}
		return
	}
	for _, i := range t.shown {
		send(t.layers[i], "setHidden:", 1)
		send(t.layers[i], "setContents:", 0)
	}
	t.shown = t.shown[:0]
	if t.grid.px == px {
		return
	}
	for _, l := range t.layers {
		if l != 0 {
			send(l, "removeFromSuperlayer") // the superlayer held the only reference
		}
	}
	*t = canvasTiles{grid: newCanvasTileGrid(px), scratch: t.scratch}
}

// tileLayer returns tile i's sublayer, creating it on first use.
func (c *canvas) tileLayer(i int) id {
	t := c.tiles
	if t.layers == nil {
		t.layers = make([]id, t.grid.cols*t.grid.rows)
	}
	if l := t.layers[i]; l != 0 {
		return l
	}
	l := send(class("CALayer"), "layer") // autoreleased; owned by the view's layer once added
	send(l, "setActions:", uintptr(noActions()))
	send(l, "setOpaque:", 1)
	send(l, "setHidden:", 1)
	send(send(c.view, "layer"), "addSublayer:", uintptr(l))
	t.layers[i] = l
	return l
}

// PresentDamage implements platform.CanvasDamagePresenter.
func (c *canvas) PresentDamage(img *image.RGBA, dirty []image.Rectangle) error {
	if c.closed {
		return errCanvasClosed
	}
	if img == nil || c.metalLayer != 0 || c.tiles == nil {
		return c.Present(img)
	}
	b := img.Bounds()
	t := c.tiles
	if b.Size() != t.grid.px {
		return c.Present(img)
	}
	rects, full := platform.DamageRects(dirty, b, canvasDamageMaxRects)
	if full {
		return c.Present(img)
	}
	for i := range rects {
		rects[i] = rects[i].Sub(b.Min) // grid coordinates
	}
	tiles, area := t.grid.touched(rects)
	if len(tiles) == 0 {
		return nil // nothing changed
	}
	if area*2 > b.Dx()*b.Dy() {
		return c.Present(img)
	}

	// Build every tile image first, so that a failure or a non-opaque tile
	// leaves the screen untouched before the whole frame replaces it.
	if n := canvasTileSize * canvasTileSize * 4; len(t.scratch) < n {
		t.scratch = make([]byte, n)
	}
	imgs := make([]uintptr, 0, len(tiles))
	release := func() {
		for _, cg := range imgs {
			cgImageRelease(cg)
		}
	}
	for _, i := range tiles {
		r := t.grid.rect(i)
		if !canvasCopyTile(t.scratch, img, r.Add(b.Min)) {
			release()
			return c.Present(img)
		}
		cg, err := newCGImage(t.scratch, r.Dx()*4, r.Dx(), r.Dy())
		if err != nil {
			release()
			return err
		}
		imgs = append(imgs, cg)
	}
	withPool(func() {
		parent := send(c.view, "layer")
		flipped := sendBool(parent, "contentsAreFlipped")
		layerH := msgRect(parent, sel("bounds")).Size.Height
		ca := class("CATransaction")
		send(ca, "begin")
		send(ca, "setDisableActions:", 1)
		for k, i := range tiles {
			l := c.tileLayer(i)
			x, y, w, h := canvasTileFrame(t.grid.rect(i), c.scale, flipped, layerH)
			msgSetRect(l, sel("setFrame:"), nsRect{Origin: nsPoint{X: x, Y: y}, Size: nsSize{Width: w, Height: h}})
			send(l, "setContents:", imgs[k])
			msgSetFloat(l, sel("setContentsScale:"), c.scale)
			if sendBool(l, "isHidden") {
				send(l, "setHidden:", 0)
				t.shown = append(t.shown, i)
			}
		}
		send(ca, "commit")
	})
	release()
	return nil
}
