//go:build linux || darwin

package gl

import (
	"fmt"
	"image"
	"unsafe"

	"github.com/tangoui/tango/ui/gpu"
)

// texture is a 2D texture of the device's context (0: none) and the size
// it was asked for (it is at least 1×1).
type texture struct {
	tex  uint32
	size image.Point
}

// texFormat says how a texture is allocated and uploaded.
type texFormat struct {
	internal int32  // sized internal format
	format   uint32 // pixel transfer format
	typ      uint32 // pixel transfer type
	bpp      int    // bytes per texel in Upload.Pix
	// half: Upload.Pix holds 16-bit unorm texels that are converted to
	// half floats before the upload (the ramp on ES without
	// GL_EXT_texture_norm16, gles.go).
	half bool
}

// Texture formats: the target, the patch page, images and layers are
// RGBA8 (premultiplied), the glyph atlas R8 (sampled as (r, 0, 0, 1), as
// Metal's R8Unorm), the gradient ramps RGBA16 with 16-bit unsigned
// normalized channels, uploaded as little-endian uint16 (the byte order
// of every platform the backend builds for: amd64, arm64).
var (
	rgba8Format  = texFormat{internal: glRGBA8, format: glRGBA, typ: glUNSIGNED_BYTE, bpp: 4}
	r8Format     = texFormat{internal: glR8, format: glRED, typ: glUNSIGNED_BYTE, bpp: 1}
	rgba16Format = texFormat{internal: glRGBA16, format: glRGBA, typ: glUNSIGNED_SHORT, bpp: 8}
)

// sampledFormats are the formats of the textures of gpu.TexMask,
// gpu.TexColor and gpu.TexRamp, by index (a Device starts from them in
// Device.formats; on ES the ramp may differ, see gles.go).
var sampledFormats = [3]texFormat{gpu.TexMask: r8Format, gpu.TexColor: rgba8Format, gpu.TexRamp: rgba16Format}

// newTexture creates a texture of size (a zero or negative side is
// created as 1 texel, so something is always bound) in format, bound to
// texture unit unit, with init as its contents when not nil (tightly
// packed rows). Allocation failures surface as GL_OUT_OF_MEMORY at the
// frame's error check.
func newTexture(f *glFuncs, unit uint32, size image.Point, tf texFormat, init []byte) texture {
	w, h := max(1, size.X), max(1, size.Y)
	t := texture{size: size}
	f.GenTextures(1, &t.tex)
	f.ActiveTexture(glTEXTURE0 + unit)
	f.BindTexture(glTEXTURE_2D, t.tex)
	// One level: complete with any filter. The sampler objects set the
	// filters and wrapping.
	f.TexParameteri(glTEXTURE_2D, glTEXTURE_BASE_LEVEL, 0)
	f.TexParameteri(glTEXTURE_2D, glTEXTURE_MAX_LEVEL, 0)
	var pix unsafe.Pointer
	if init != nil {
		f.PixelStorei(glUNPACK_ALIGNMENT, 1)
		f.PixelStorei(glUNPACK_ROW_LENGTH, 0)
		pix = unsafe.Pointer(&init[0])
	}
	f.TexImage2D(glTEXTURE_2D, 0, tf.internal, int32(w), int32(h), 0, tf.format, tf.typ, pix)
	if init != nil {
		restoreUnpack(f)
	}
	return t
}

// release deletes the texture. GL keeps its storage until the commands
// already issued that use it have run.
func (t *texture) release(f *glFuncs) {
	if t.tex != 0 {
		f.DeleteTextures(1, &t.tex)
	}
	*t = texture{}
}

// ensureTextures (re)creates the render target (with its framebuffer
// attachment) and the mask, color and ramp textures whose size changed
// (or that do not exist). The old contents are dropped: a new target
// holds no frame (hasFrame false) until a frame completes, and the
// Renderer re-uploads what it samples from a recreated texture. An
// incomplete framebuffer is an error (a failure the caller counts).
func (d *Device) ensureTextures(fr *gpu.Frame) error {
	f := d.f
	if d.target.tex == 0 || d.target.size != fr.Size {
		d.target.release(f)
		d.hasFrame = false
		d.target = newTexture(f, gpu.TexImage, fr.Size, rgba8Format, nil)
		if d.fbo == 0 {
			f.GenFramebuffers(1, &d.fbo)
		}
		f.BindFramebuffer(glFRAMEBUFFER, d.fbo)
		f.FramebufferTexture2D(glFRAMEBUFFER, glCOLOR_ATTACHMENT0, glTEXTURE_2D, d.target.tex, 0)
		if st := f.CheckFramebufferStatus(glFRAMEBUFFER); st != glFRAMEBUFFER_COMPLETE {
			// Not a valid target (allocation failed, say): do not keep it.
			f.FramebufferTexture2D(glFRAMEBUFFER, glCOLOR_ATTACHMENT0, glTEXTURE_2D, 0, 0)
			d.target.release(f)
			return fmt.Errorf("gl: render target %d×%d: framebuffer status %#x", fr.Size.X, fr.Size.Y, st)
		}
	}
	for i, want := range [3]image.Point{gpu.TexMask: fr.MaskSize, gpu.TexColor: fr.ColorSize, gpu.TexRamp: fr.RampSize} {
		if d.tex[i].tex != 0 && d.tex[i].size == want {
			continue
		}
		d.tex[i].release(f)
		d.tex[i] = newTexture(f, uint32(i), want, d.formats[i], nil)
	}
	return nil
}

// uploadBPP returns the bytes per texel of texture tex (gpu.TexMask,
// gpu.TexColor or gpu.TexRamp), or 0 for any other index.
func uploadBPP(tex int) int {
	switch tex {
	case gpu.TexMask, gpu.TexColor, gpu.TexRamp:
		return sampledFormats[tex].bpp
	}
	return 0
}

// checkUpload validates u against its texture's size: a known texture, a
// rectangle inside it and enough pixels for its rows at u.Stride. An
// empty rectangle is valid (nothing is copied).
func checkUpload(u gpu.Upload, size image.Point) error {
	bpp := uploadBPP(u.Tex)
	if bpp == 0 {
		return fmt.Errorf("gl: upload to unknown texture %d", u.Tex)
	}
	if !u.Rect.In(image.Rectangle{Max: size}) {
		return fmt.Errorf("gl: upload %v outside texture %d of size %v", u.Rect, u.Tex, size)
	}
	w, h := u.Rect.Dx(), u.Rect.Dy()
	if w <= 0 || h <= 0 {
		return nil
	}
	if u.Stride < 0 || u.Stride > 1<<30 {
		return fmt.Errorf("gl: upload %v with stride %d", u.Rect, u.Stride)
	}
	if len(u.Pix) < (h-1)*u.Stride+w*bpp {
		return fmt.Errorf("gl: upload %v: %d bytes with stride %d", u.Rect, len(u.Pix), u.Stride)
	}
	return nil
}

// checkUploads validates every upload of fr (the frame's and each
// batch's) before anything is drawn, so an invalid frame changes nothing
// but the textures ensureTextures recreated.
func (d *Device) checkUploads(fr *gpu.Frame) error {
	check := func(ups []gpu.Upload) error {
		for _, u := range ups {
			var size image.Point
			if uploadBPP(u.Tex) != 0 {
				size = d.tex[u.Tex].size
			}
			if err := checkUpload(u, size); err != nil {
				return err
			}
		}
		return nil
	}
	if err := check(fr.Uploads); err != nil {
		return err
	}
	for i := range fr.Batches {
		if err := check(fr.Batches[i].Uploads); err != nil {
			return fmt.Errorf("batch %d: %w", i, err)
		}
	}
	return nil
}

// upload writes validated uploads (checkUploads) into their textures, in
// order, with glTexSubImage2D. GL orders each write after the draws
// issued before it (the driver copies the pixels before returning and
// renames or waits as needed): earlier batches keep sampling the old
// contents, as gpu.Batch.Uploads requires. Rows are read
// GL_UNPACK_ROW_LENGTH = Stride/bpp texels apart; a stride that is no
// multiple of the texel size, or shorter than a row (GL_UNPACK_ROW_LENGTH
// 0 would mean "w"), is uploaded one row at a time.
func (d *Device) upload(ups []gpu.Upload) {
	if len(ups) == 0 {
		return
	}
	f := d.f
	f.PixelStorei(glUNPACK_ALIGNMENT, 1)
	for i := range ups {
		u := &ups[i]
		w, h := u.Rect.Dx(), u.Rect.Dy()
		if w <= 0 || h <= 0 {
			continue
		}
		tf := d.formats[u.Tex]
		f.ActiveTexture(glTEXTURE0 + uint32(u.Tex))
		f.BindTexture(glTEXTURE_2D, d.tex[u.Tex].tex)
		x, y := int32(u.Rect.Min.X), int32(u.Rect.Min.Y)
		if tf.half { // ES ramp without norm16: converted, tightly packed
			d.halfBuf = halfRows(d.halfBuf, u.Pix, w, h, u.Stride)
			f.PixelStorei(glUNPACK_ROW_LENGTH, 0)
			f.TexSubImage2D(glTEXTURE_2D, 0, x, y, int32(w), int32(h), tf.format, tf.typ, unsafe.Pointer(&d.halfBuf[0]))
			continue
		}
		if h == 1 || u.Stride%tf.bpp == 0 && u.Stride >= w*tf.bpp {
			rowLen := int32(0) // one row: the row length does not matter
			if h > 1 {
				rowLen = int32(u.Stride / tf.bpp)
			}
			f.PixelStorei(glUNPACK_ROW_LENGTH, rowLen)
			// Reads (h-1)·Stride + w·bpp bytes, inside u.Pix (checkUpload).
			f.TexSubImage2D(glTEXTURE_2D, 0, x, y, int32(w), int32(h), tf.format, tf.typ, unsafe.Pointer(&u.Pix[0]))
			continue
		}
		f.PixelStorei(glUNPACK_ROW_LENGTH, 0)
		for r := range h {
			f.TexSubImage2D(glTEXTURE_2D, 0, x, y+int32(r), int32(w), 1, tf.format, tf.typ, unsafe.Pointer(&u.Pix[r*u.Stride]))
		}
	}
	restoreUnpack(f)
}

// restoreUnpack puts the unpack state back to GL's defaults, which a
// presenter's toolkit sharing the context may rely on.
func restoreUnpack(f *glFuncs) {
	f.PixelStorei(glUNPACK_ROW_LENGTH, 0)
	f.PixelStorei(glUNPACK_ALIGNMENT, 4)
}

// checkTextureSizes returns an error wrapping gpu.ErrTargetTooLarge when a
// texture of fr (the render target, the mask atlas, the patch page, the
// ramp texture, an image or a layer) is larger than limit on a side.
func checkTextureSizes(fr *gpu.Frame, limit int) error {
	too := func(p image.Point) bool { return p.X > limit || p.Y > limit }
	for _, t := range []struct {
		what string
		size image.Point
	}{{"render target", fr.Size}, {"mask texture", fr.MaskSize}, {"color texture", fr.ColorSize}, {"ramp texture", fr.RampSize}} {
		if too(t.size) {
			return fmt.Errorf("gl: %w: %s %d×%d (max %d)", gpu.ErrTargetTooLarge, t.what, t.size.X, t.size.Y, limit)
		}
	}
	for _, u := range fr.ImageUploads {
		if too(u.Size) {
			return fmt.Errorf("gl: %w: image texture %d×%d (max %d)", gpu.ErrTargetTooLarge, u.Size.X, u.Size.Y, limit)
		}
	}
	for _, l := range fr.Layers {
		if too(l.Size) {
			return fmt.Errorf("gl: %w: layer texture %d×%d (max %d)", gpu.ErrTargetTooLarge, l.Size.X, l.Size.Y, limit)
		}
	}
	return nil
}
