package gpu

import (
	"image"
	"reflect"

	"github.com/tangoui/tango/ui/paint"
)

// Image texture limits.
const (
	// maxImageSide is the largest image texture side: Metal's 2D texture
	// limit on every Mac GPU family. A larger image (after the reduction
	// paint.ImageReduction applies) is drawn as a patch. cpuexec.Device
	// keeps a copy of this value (internal/render/gpu/cpuexec/device.go): change both.
	maxImageSide = 16384
	// defaultImageBudget is how many bytes of image textures one frame
	// uploads at most; images beyond it are drawn as patches that frame
	// and uploaded in a later one.
	defaultImageBudget = 64 << 20
	// imageIdleFrames is how many frames an image texture is kept unused
	// before it is freed.
	imageIdleFrames = 300
	// imageCacheBytes bounds the image textures held: beyond it, the least
	// recently used textures the current frame does not use are freed.
	imageCacheBytes = 256 << 20
	// imageStaleBytes bounds the textures the current frame does not draw:
	// beyond it the least recently used of them are freed. A window that
	// goes idle renders no more frames, so whatever the last frame leaves
	// held stays held; and images re-created every frame (a zoomed or
	// resized preview resamples into a new *image.RGBA per placement) would
	// otherwise pile up to imageCacheBytes until imageIdleFrames pass.
	imageStaleBytes = 48 << 20
)

// imageKey identifies an image texture: the image.Image value (pointer
// identity for the usual pointer image types) and the size it is reduced
// to (paint.ImageReduction; 0, 0 for full resolution).
type imageKey struct {
	src  image.Image
	w, h int
}

// imageTex is one image texture.
type imageTex struct {
	id   int
	key  imageKey
	pix  *image.RGBA // paint.PrepareImage's pixels, which the texture holds
	used uint64      // last frame that drew it
	sent bool        // uploaded in a frame the device executed
}

func (t *imageTex) bytes() int { return 4 * t.pix.Rect.Dx() * t.pix.Rect.Dy() }

// imageCache holds the image textures the device keeps (one per image and
// reduction size), assigns their IDs and collects each frame's
// ImageUploads and ImageFrees. Images are converted to premultiplied
// RGBA (paint.PrepareImage) once, when uploaded; the converted pixels
// stay with the texture so a failed frame can upload them again.
type imageCache struct {
	byKey    map[imageKey]*imageTex
	next     int    // last ID assigned
	frame    uint64 // current frame number (starts at 1)
	held     int    // bytes of all textures
	uploaded int    // bytes uploaded this frame
	uploads  []*imageTex
	frees    []int  // IDs to free, kept until a frame carrying them succeeds
	idle     uint64 // frames a texture is kept unused (imageIdleFrames; tests lower it)
}

func newImageCache() *imageCache {
	return &imageCache{byKey: map[imageKey]*imageTex{}, idle: imageIdleFrames}
}

// beginFrame starts a frame.
func (c *imageCache) beginFrame() {
	c.frame++
	c.uploaded = 0
	clear(c.uploads)
	c.uploads = c.uploads[:0]
}

// imageMiss says why get returned no texture.
type imageMiss uint8

const (
	imageOK       imageMiss = iota
	imageTooLarge           // a side beyond maxImageSide
	imageBudget             // this frame's upload budget is spent
	imageNoKey              // the image is not a pointer (see cacheable)
)

// cacheable reports whether src can key the cache: only pointer image
// types (every standard one). A value type can pass reflect's Comparable
// and still panic when hashed (an interface field holding a slice), so
// value images are not cached; they are drawn as patches.
func cacheable(src image.Image) bool {
	return src != nil && reflect.TypeOf(src).Kind() == reflect.Pointer
}

// get returns the texture of src reduced to w×h (0, 0: full resolution),
// uploading it this frame when the device does not hold it, unless it is
// too large or the upload would take this frame past budget bytes.
func (c *imageCache) get(src image.Image, w, h, budget int) (*imageTex, imageMiss) {
	if !cacheable(src) {
		return nil, imageNoKey
	}
	k := imageKey{src, w, h}
	t := c.byKey[k]
	if t != nil && (t.sent || t.used == c.frame) {
		t.used = c.frame
		return t, imageOK
	}
	size := image.Pt(w, h)
	if w <= 0 {
		size = src.Bounds().Size()
	}
	if size.X > maxImageSide || size.Y > maxImageSide {
		return nil, imageTooLarge
	}
	// One image larger than the whole budget still goes up as the
	// frame's first upload, so it is not a patch forever.
	if n := 4 * size.X * size.Y; c.uploaded > 0 && c.uploaded+n > budget {
		return nil, imageBudget
	}
	if t == nil {
		c.next++
		t = &imageTex{id: c.next, key: k, pix: paint.PrepareImage(src, w, h)}
		c.byKey[k] = t
		c.held += t.bytes()
	}
	t.used = c.frame
	c.uploaded += t.bytes()
	c.uploads = append(c.uploads, t)
	return t, imageOK
}

// touch marks the texture of src reduced to w×h (if held) as used this
// frame without drawing or uploading it: a partial frame's image that lies
// outside the dirty region is on screen all the same.
func (c *imageCache) touch(src image.Image, w, h int) {
	if !cacheable(src) {
		return
	}
	if t := c.byKey[imageKey{src, w, h}]; t != nil {
		t.used = c.frame
	}
}

// invalidate frees every texture of src.
func (c *imageCache) invalidate(src image.Image) {
	if !cacheable(src) {
		return
	}
	for k, t := range c.byKey {
		if k.src == src {
			c.free(t)
		}
	}
}

func (c *imageCache) free(t *imageTex) {
	delete(c.byKey, t.key)
	c.held -= t.bytes()
	c.frees = append(c.frees, t.id)
}

// evict frees textures unused for c.idle frames and, while more
// than imageCacheBytes are held or the textures this frame does not use
// hold more than imageStaleBytes, the least recently used of those.
func (c *imageCache) evict() {
	stale := 0
	for _, t := range c.byKey {
		if c.frame-t.used > c.idle {
			c.free(t)
		} else if t.used != c.frame {
			stale += t.bytes()
		}
	}
	for c.held > imageCacheBytes || stale > imageStaleBytes {
		var lru *imageTex
		for _, t := range c.byKey {
			if t.used != c.frame && (lru == nil || t.used < lru.used) {
				lru = t
			}
		}
		if lru == nil {
			return
		}
		stale -= lru.bytes()
		c.free(lru)
	}
}

// frameLists fills f's ImageFrees and ImageUploads (reusing their slices).
func (c *imageCache) frameLists(f *Frame) {
	c.evict()
	f.ImageFrees = append(f.ImageFrees[:0], c.frees...)
	clear(f.ImageUploads)
	f.ImageUploads = f.ImageUploads[:0]
	for _, t := range c.uploads {
		f.ImageUploads = append(f.ImageUploads, ImageUpload{ID: t.id, Size: t.pix.Rect.Size(), Pix: t.pix.Pix, Stride: t.pix.Stride})
	}
}

// executed records the outcome of the frame: on success its uploads are
// held by the device and its frees done. On failure every texture is
// marked unsent and is uploaded again (under its ID, replacing the
// device's) when next drawn: a device may report a GPU failure one frame
// late (Metal returns the previous command buffer's error from the next
// Execute), so uploads of earlier "successful" frames may be lost too, as
// for the mask and ramp textures. Frees are sent again (freeing an ID the
// device does not hold is harmless).
func (c *imageCache) executed(ok bool) {
	if !ok {
		for _, t := range c.byKey {
			t.sent = false
		}
		return
	}
	for _, t := range c.uploads {
		t.sent = true
	}
	clear(c.frees)
	c.frees = c.frees[:0]
}
