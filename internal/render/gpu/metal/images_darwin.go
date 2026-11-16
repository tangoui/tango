//go:build darwin

package metal

import (
	"fmt"
	"image"

	"github.com/tangoui/tango/internal/render/gpu"
)

// Only candidates detached by the current frame can be handed off here. These
// bounds limit transient retention when uploads need different sizes; unused
// candidates may enter the separately bounded small spare pool at the end.
const (
	imageHandoffCount = 32
	imageHandoffBytes = 8 << 20
)

type imageCandidate struct {
	tex  id
	size image.Point
}

// applyImages runs after Execute waited for previous submissions. Every
// reused texture is fully overwritten by stage/blit before current draws.
func (d *Device) applyImages(f *gpu.Frame) (err error) {
	d.imagePool.beginFrame(len(f.ImageUploads) == 0)
	pooling := len(f.ImageUploads) > 0 && !d.imageTrimmed
	d.imageTrimmed = false
	if d.images == nil {
		d.images = map[int]id{}
	}
	if d.imageSizes == nil {
		d.imageSizes = map[int]image.Point{}
	}
	var candidates [imageHandoffCount]imageCandidate
	count, heldBytes := 0, 0
	defer func() {
		for i := 0; i < count; i++ {
			if err == nil && pooling {
				d.imagePool.put(candidates[i])
			} else {
				release(candidates[i].tex)
			}
		}
		if err != nil {
			d.imagePool.clear()
		}
	}()
	for _, key := range f.ImageFrees {
		tex, size := d.images[key], d.imageSizes[key]
		delete(d.images, key)
		delete(d.imageSizes, key)
		if tex == 0 {
			continue
		}
		bytes := 4 * size.X * size.Y
		if len(f.ImageUploads) > 0 && size.X > 0 && size.Y > 0 && count < len(candidates) && bytes <= imageHandoffBytes-heldBytes {
			candidates[count] = imageCandidate{tex: tex, size: size}
			count++
			heldBytes += bytes
		} else {
			release(tex)
		}
	}
	if d.noImage == 0 {
		t, err := d.newTexture(1, 1, pixelFormatRGBA8Unorm, usageShaderRead)
		if err != nil {
			return err
		}
		d.noImage = t
	}
	for _, u := range f.ImageUploads {
		w, h := u.Size.X, u.Size.Y
		if u.ID <= 0 || w <= 0 || h <= 0 {
			return fmt.Errorf("metal: image upload %d of size %v", u.ID, u.Size)
		}
		if len(u.Pix) < (h-1)*u.Stride+4*w {
			return fmt.Errorf("metal: image upload %d: %d bytes with stride %d for %v", u.ID, len(u.Pix), u.Stride, u.Size)
		}
		old := d.images[u.ID]
		tex := old
		if tex == 0 || d.imageSizes[u.ID] != u.Size {
			tex = 0
			for i := count - 1; i >= 0; i-- {
				if candidates[i].size == u.Size {
					tex = candidates[i].tex
					count--
					candidates[i] = candidates[count]
					candidates[count] = imageCandidate{}
					break
				}
			}
			if tex == 0 && pooling {
				tex = d.imagePool.take(u.Size)
			}
			if tex == 0 {
				t, err := d.newTexture(w, h, pixelFormatRGBA8Unorm, usageShaderRead)
				if err != nil {
					return err
				}
				tex = t
			}
			release(old)
		}
		d.images[u.ID] = tex
		d.imageSizes[u.ID] = u.Size
	}
	for i := range f.Batches {
		if key := f.Batches[i].Image; key != 0 && d.images[key] == 0 {
			return fmt.Errorf("metal: batch %d binds image %d, which the device does not hold", i, key)
		}
	}
	return nil
}
