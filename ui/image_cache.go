package ui

import "image"

type imageInvalidator interface {
	InvalidateImage(image.Image)
}

// InvalidateImage drops the renderer's cached copies of img. Call it on
// the UI goroutine during building or painting, before rendering, after
// changing image pixels or discarding a private derived image. A later
// draw of the same image value prepares it again. Renderers without image
// caches may ignore this hint.
//
// This does not declare redraw damage. In-place pixel changes also need
// Damage (or a full redraw); replacing a drawn image value changes its
// paint fingerprint normally. Recorded image pixels remain caller-owned.
func (c *Context) InvalidateImage(img image.Image) {
	c.checkThread()
	if c != nil && img != nil && c.imageCache != nil {
		c.imageCache.InvalidateImage(img)
	}
}
