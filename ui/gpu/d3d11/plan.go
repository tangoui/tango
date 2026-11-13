package d3d11

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"strings"

	"github.com/tangoui/tango/ui/gpu"
	"github.com/tangoui/tango/ui/gpu/gen"
	"github.com/tangoui/tango/ui/gpu/internal/devkit"
)

// The parts of executing a frame that need no Direct3D: validation, the
// texture-size limit, scissor and upload-box arithmetic, the instance
// buffer's ring allocation and the failure accounting. They build on every
// platform so plan_test.go runs them on the development machine; the
// *_windows.go files only issue the COM calls these decide on.

// maxFailures consecutive failed frames mark the device lost (as
// ui/gpu/metal's maxFailures).
const maxFailures = 3

// maxTextureSizeFor is the largest 2D texture side of a device of feature
// level fl: D3D11_REQ_TEXTURE2D_U_OR_V_DIMENSION (16384) from 11_0 on,
// D3D10_REQ_TEXTURE2D_U_OR_V_DIMENSION (8192) on 10_0 and 10_1.
func maxTextureSizeFor(fl uint32) int {
	if fl >= featureLevel11_0 {
		return maxTexture2DDimension
	}
	return 8192
}

// shaderProfiles returns the HLSL targets to compile the quad shader's
// vertex and pixel stages for on a device of feature level fl: shader
// model 5.0 from 11_0 on, 4.0 below (a 10_x device rejects 5.0 bytecode).
func shaderProfiles(fl uint32) (vs, ps string) {
	if fl >= featureLevel11_0 {
		return gen.QuadVSTarget, gen.QuadPSTarget
	}
	return gen.QuadVSTargetSM4, gen.QuadPSTargetSM4
}

// checkTextureSizes returns an error wrapping gpu.ErrTargetTooLarge when a
// texture of f (the render target, the mask atlas, the patch page, the
// ramp texture, an image or a layer) is larger than limit on a side.
func checkTextureSizes(f *gpu.Frame, limit int) error {
	too := func(p image.Point) bool { return p.X > limit || p.Y > limit }
	for _, t := range []struct {
		what string
		size image.Point
	}{{"render target", f.Size}, {"mask texture", f.MaskSize}, {"color texture", f.ColorSize}, {"ramp texture", f.RampSize}} {
		if too(t.size) {
			return fmt.Errorf("d3d11: %w: %s %d×%d (max %d)", gpu.ErrTargetTooLarge, t.what, t.size.X, t.size.Y, limit)
		}
	}
	for _, u := range f.ImageUploads {
		if too(u.Size) {
			return fmt.Errorf("d3d11: %w: image texture %d×%d (max %d)", gpu.ErrTargetTooLarge, u.Size.X, u.Size.Y, limit)
		}
	}
	for _, l := range f.Layers {
		if too(l.Size) {
			return fmt.Errorf("d3d11: %w: layer texture %d×%d (max %d)", gpu.ErrTargetTooLarge, l.Size.X, l.Size.Y, limit)
		}
	}
	return nil
}

// checkFrame rejects a frame with Copies but no Dirty (invalid: run as a
// full frame it would clear the target and drop the copies) and returns
// the number of instances the frame writes: its batches' and, in a
// partial frame, one clearing quad per dirty rectangle. More than the
// instance buffer can hold (maxInstBuffer) is an error wrapping
// gpu.ErrTargetTooLarge: like an oversized target it is no GPU failure,
// and the CPU renderer can draw the frame.
func checkFrame(f *gpu.Frame) (nInst int, err error) {
	if len(f.Dirty) == 0 && len(f.Copies) > 0 {
		return 0, errors.New("d3d11: frame with Copies but no Dirty")
	}
	for i := range f.Batches {
		nInst += len(f.Batches[i].Instances)
	}
	nInst += len(f.Dirty)
	if limit := maxInstBuffer / gen.QuadInstanceStride; nInst > limit {
		return 0, fmt.Errorf("d3d11: instance count exceeds the device buffer limit: %d instances (max %d) (%w)", nInst, limit, gpu.ErrTargetTooLarge)
	}
	return nInst, nil
}

// uploadBPP returns the bytes per texel of texture tex (gpu.TexMask,
// gpu.TexColor or gpu.TexRamp), or 0 for any other index.
func uploadBPP(tex int) int {
	switch tex {
	case gpu.TexMask:
		return 1
	case gpu.TexColor:
		return 4
	case gpu.TexRamp:
		return 8
	}
	return 0
}

// checkUpload validates u against its texture's size: a known texture, a
// rectangle inside it and enough pixels for its rows at u.Stride. It
// returns the bytes per texel. An empty rectangle is valid (nothing is
// copied).
func checkUpload(u gpu.Upload, size image.Point) (bpp int, err error) {
	bpp = uploadBPP(u.Tex)
	if bpp == 0 {
		return 0, fmt.Errorf("d3d11: upload to unknown texture %d", u.Tex)
	}
	if !u.Rect.In(image.Rectangle{Max: size}) {
		return 0, fmt.Errorf("d3d11: upload %v outside texture %d of size %v", u.Rect, u.Tex, size)
	}
	w, h := u.Rect.Dx(), u.Rect.Dy()
	if w <= 0 || h <= 0 {
		return bpp, nil
	}
	if u.Stride < 0 || u.Stride > 1<<30 {
		return 0, fmt.Errorf("d3d11: upload %v with stride %d", u.Rect, u.Stride)
	}
	if len(u.Pix) < (h-1)*u.Stride+w*bpp {
		return 0, fmt.Errorf("d3d11: upload %v: %d bytes with stride %d", u.Rect, len(u.Pix), u.Stride)
	}
	return bpp, nil
}

// uploadBox is the D3D11_BOX UpdateSubresource writes r into (r inside the
// texture, not empty: see checkUpload).
func uploadBox(r image.Rectangle) box {
	return box{Left: uint32(r.Min.X), Top: uint32(r.Min.Y), Front: 0, Right: uint32(r.Max.X), Bottom: uint32(r.Max.Y), Back: 1}
}

// scissorFor returns the scissor rectangle, in the target's texels, of a
// batch whose Scissor is s (main-target coordinates; empty means the whole
// target) drawing into a target whose texel (0, 0) is at origin and whose
// drawable texels are full. ok is false when nothing would be drawn.
func scissorFor(s image.Rectangle, origin image.Point, full image.Rectangle) (r rect, ok bool) {
	sc := full
	if !s.Empty() {
		sc = s.Sub(origin).Intersect(full)
	}
	if sc.Empty() {
		return rect{}, false
	}
	return rect{Left: int32(sc.Min.X), Top: int32(sc.Min.Y), Right: int32(sc.Max.X), Bottom: int32(sc.Max.Y)}, true
}

// clearColor is the float color a target is cleared to for c: paint.CPU
// fills the target with the 8-bit premultiplied color, and c/255 converts
// back to exactly those bytes in an R8G8B8A8_UNORM target.
func clearColor(c color.NRGBA) [4]float32 { return devkit.ClearColor(c) }

// inputFormat returns the DXGI_FORMAT of a vertex attribute of n
// components of kind ("f32", "i32", "u32"), as gen.QuadInstanceAttrs
// describes them.
func inputFormat(kind string, n int) (uint32, error) {
	formats := map[string][4]uint32{
		"f32": {formatR32Float, formatR32G32Float, formatR32G32B32Float, formatR32G32B32A32Float},
		"i32": {formatR32Sint, formatR32G32Sint, formatR32G32B32Sint, formatR32G32B32A32Sint},
		"u32": {formatR32Uint, formatR32G32Uint, formatR32G32B32Uint, formatR32G32B32A32Uint},
	}
	fs, ok := formats[kind]
	if !ok || n < 1 || n > 4 {
		return 0, fmt.Errorf("d3d11: no input format for %d × %s", n, kind)
	}
	return fs[n-1], nil
}

// samplerFilter returns the D3D11_FILTER of a gen.TextureDesc filter name.
func samplerFilter(name string) (uint32, error) {
	switch name {
	case "nearest":
		return filterMinMagMipPoint, nil
	case "linear":
		return filterMinMagMipLinear, nil
	}
	return 0, fmt.Errorf("d3d11: unknown sampler filter %q", name)
}

// ---- Instance buffer ring ----

// instRing allocates each frame's instances from one dynamic vertex
// buffer used as a ring: a frame's instances go after the previous
// frame's (mapped with D3D11_MAP_WRITE_NO_OVERWRITE, so the driver need
// not wait for or rename the buffer) until they no longer fit, then the
// buffer is mapped with D3D11_MAP_WRITE_DISCARD (the driver hands out
// fresh memory while earlier frames may still read the old) and filling
// starts over at 0. A frame larger than the buffer replaces it with a
// bigger one.
type instRing struct {
	cap   int  // bytes of the buffer; 0 when there is none
	pos   int  // bytes in use since the last discard
	fresh bool // the buffer was just created: its first map discards
}

// minInstBuffer is the smallest instance buffer created (455 instances).
const minInstBuffer = 64 << 10

// maxInstBuffer bounds the instance buffer: D3D11 limits a resource to
// D3D11_REQ_RESOURCE_SIZE_IN_MEGABYTES_EXPRESSION_A_TERM (128 MiB) times
// a hardware-dependent factor; 128 MiB (932067 instances) is always
// allowed.
const maxInstBuffer = 128 << 20

// ringAlloc is where a frame's n bytes of instances go.
type ringAlloc struct {
	off     int // byte offset of the frame's first instance
	discard bool
	grow    int // > 0: replace the buffer with one of grow bytes first
}

// alloc reserves n > 0 bytes for a frame and returns where they go. It
// fails for a frame larger than maxInstBuffer. The ring commits to the
// allocation: call reset if the frame is then not written (the buffer's
// state is unknown).
func (r *instRing) alloc(n int) (ringAlloc, error) {
	if n <= 0 {
		return ringAlloc{}, fmt.Errorf("d3d11: instance allocation of %d bytes", n)
	}
	if n > maxInstBuffer {
		return ringAlloc{}, fmt.Errorf("d3d11: %d bytes of instances exceed the %d-byte instance buffer limit", n, maxInstBuffer)
	}
	var a ringAlloc
	switch {
	case n > r.cap:
		a.grow = min(max(n, 2*r.cap, minInstBuffer), maxInstBuffer)
		r.cap, r.fresh = a.grow, true
		fallthrough
	case r.fresh, r.pos+n > r.cap:
		a.discard = true
		r.pos, r.fresh = 0, false
	}
	a.off = r.pos
	r.pos += n
	return a, nil
}

// reset forgets the buffer (it was released or failed to be created or
// mapped): the next alloc creates a new one.
func (r *instRing) reset() { *r = instRing{} }

// ---- Failure accounting ----

// health counts failed frames and decides when the device is lost (as
// ui/gpu/metal's gpuFailed and syncFailed): maxFailures failed frames in
// a row, or one error saying the device was removed, reset or hung, lose
// it for good. A frame rejected as invalid or too large is not a failure
// and never reaches health.
//
// Presents (the presenter's ResizeBuffers and Present1) have a counter of
// their own, reset only by a successful present: the platform presents
// only after a successful Render, so with one shared counter every
// Execute would reset it and a present failing forever (ResizeBuffers
// refused for an outstanding back-buffer reference, say) would never
// reach maxFailures — the window would freeze and the main thread spin
// instead of falling back to the CPU renderer.
type health struct {
	failures        int   // consecutive failed frames
	presentFailures int   // consecutive failed presents
	lost            error // non-nil (wrapping gpu.ErrDeviceLost) once the device is lost
}

// fail records a failed frame and returns the error to report: err, or
// the loss error once the device counts as lost.
func (h *health) fail(err error) error {
	h.failures++
	if h.lost == nil && (isDeviceLost(err) || h.failures >= maxFailures) {
		h.lost = fmt.Errorf("%w: %w", gpu.ErrDeviceLost, err)
	}
	if h.lost != nil {
		return h.lost
	}
	return err
}

// lose records a failure that loses the device whatever it is (the
// device reports a removed reason) and returns the loss error.
func (h *health) lose(err error) error {
	h.failures++
	if h.lost == nil {
		h.lost = fmt.Errorf("%w: %w", gpu.ErrDeviceLost, err)
	}
	return h.lost
}

// succeeded records a frame submitted without error.
func (h *health) succeeded() { h.failures = 0 }

// presentFailed records a failed present and returns the error to report:
// err, or the loss error once maxFailures presents in a row failed (or err
// says the device is gone). Frames executed in between do not reset the
// count.
func (h *health) presentFailed(err error) error {
	h.presentFailures++
	if h.lost == nil && (isDeviceLost(err) || h.presentFailures >= maxFailures) {
		h.lost = fmt.Errorf("%w: %w", gpu.ErrDeviceLost, err)
	}
	if h.lost != nil {
		return h.lost
	}
	return err
}

// presentSucceeded records a successful present.
func (h *health) presentSucceeded() { h.presentFailures = 0 }

// compilerLog is the text of D3DCompile's message blob, whose C string
// ends in a NUL (included in the blob's size).
func compilerLog(b []byte) string {
	return strings.TrimSpace(strings.TrimRight(string(b), "\x00"))
}
