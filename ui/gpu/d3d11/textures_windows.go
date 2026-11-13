//go:build windows && (amd64 || arm64)

package d3d11

import (
	"fmt"
	"image"
	"runtime"
	"slices"
	"unsafe"

	"github.com/tangoui/tango/ui/gpu"
)

// texture is a 2D texture with the views the device uses. Every non-zero
// field is a reference the texture owns.
type texture struct {
	tex  uintptr     // ID3D11Texture2D
	srv  uintptr     // ID3D11ShaderResourceView, if sampled
	rtv  uintptr     // ID3D11RenderTargetView, if rendered into
	size image.Point // the size asked for (the texture is at least 1×1)
}

// Views newTexture creates.
const (
	viewSRV = 1 << iota
	viewRTV
)

// textureSpec says how newTexture creates a texture.
type textureSpec struct {
	format uint32 // DXGI_FORMAT
	usage  uint32 // D3D11_USAGE
	bind   uint32 // D3D11_BIND_FLAG
	cpu    uint32 // D3D11_CPU_ACCESS_FLAG
	views  int    // viewSRV, viewRTV
}

// Texture kinds. The target is also a shader resource: the presenter
// reads it into the swap chain's back buffer.
var (
	targetSpec   = textureSpec{format: formatR8G8B8A8Unorm, usage: usageDefault, bind: bindRenderTarget | bindShaderResource, views: viewRTV | viewSRV}
	maskSpec     = textureSpec{format: formatR8Unorm, usage: usageDefault, bind: bindShaderResource, views: viewSRV}
	colorSpec    = textureSpec{format: formatR8G8B8A8Unorm, usage: usageDefault, bind: bindShaderResource, views: viewSRV}
	rampSpec     = textureSpec{format: formatR16G16B16A16Unorm, usage: usageDefault, bind: bindShaderResource, views: viewSRV}
	readbackSpec = textureSpec{format: formatR8G8B8A8Unorm, usage: usageStaging, cpu: cpuAccessRead}
	// Images are immutable: created whole from their pixels.
	imageSpec = textureSpec{format: formatR8G8B8A8Unorm, usage: usageImmutable, bind: bindShaderResource, views: viewSRV}
	// Layers are drawn into, then sampled by their composites.
	layerSpec = textureSpec{format: formatR8G8B8A8Unorm, usage: usageDefault, bind: bindRenderTarget | bindShaderResource, views: viewSRV | viewRTV}
	// The copy scratch is only a CopySubresourceRegion source and
	// destination.
	scratchSpec = textureSpec{format: formatR8G8B8A8Unorm, usage: usageDefault, bind: bindShaderResource}
)

// sampledSpecs are the specs of the textures of gpu.TexMask, gpu.TexColor
// and gpu.TexRamp, by index.
var sampledSpecs = [3]textureSpec{gpu.TexMask: maskSpec, gpu.TexColor: colorSpec, gpu.TexRamp: rampSpec}

// newTexture creates a texture of size (a zero or negative side is
// created as 1 texel, so something is always bound) and the views spec
// asks for. init, when not nil, is the initial contents: rows of pitch
// bytes, every row whole (len(init) >= rows × pitch). On error nothing is
// left allocated.
func newTexture(dev uintptr, size image.Point, spec textureSpec, init []byte, pitch int) (t texture, err error) {
	w, h := max(1, size.X), max(1, size.Y)
	desc := texture2DDesc{
		Width:          uint32(w),
		Height:         uint32(h),
		MipLevels:      1,
		ArraySize:      1,
		Format:         spec.format,
		SampleDesc:     sampleDesc{Count: 1},
		Usage:          spec.usage,
		BindFlags:      spec.bind,
		CPUAccessFlags: spec.cpu,
	}
	var initData *subresourceData
	if init != nil {
		if pitch <= 0 || len(init) < h*pitch {
			return texture{}, fmt.Errorf("d3d11: %d bytes of initial data for %d rows of %d", len(init), h, pitch)
		}
		initData = &subresourceData{SysMem: unsafe.Pointer(&init[0]), SysMemPitch: uint32(pitch)}
	}
	// CreateTexture2D(pDesc, pInitialData, ppTexture2D). initData points
	// at init (Go memory inside a structure): kept alive below.
	hr := checkHR("CreateTexture2D", comCall(dev, devCreateTexture2D,
		uintptr(unsafe.Pointer(&desc)), uintptr(unsafe.Pointer(initData)), uintptr(unsafe.Pointer(&t.tex))))
	runtime.KeepAlive(init)
	runtime.KeepAlive(initData)
	if err := hrError(fmt.Sprintf("CreateTexture2D(%d×%d, format %d)", w, h, spec.format), hr); err != nil {
		release(t.tex) // non-zero only when the call succeeded and a test injected the failure
		return texture{}, err
	}
	t.size = size
	defer func() {
		if err != nil {
			t.release()
			t = texture{}
		}
	}()
	if spec.views&viewSRV != 0 {
		// CreateShaderResourceView(pResource, pDesc (NULL: the whole
		// texture in its own format), ppSRView)
		hr := comCall(dev, devCreateShaderResourceView, t.tex, 0, uintptr(unsafe.Pointer(&t.srv)))
		if err := hrError("CreateShaderResourceView", hr); err != nil {
			t.srv = 0
			return t, err
		}
	}
	if spec.views&viewRTV != 0 {
		// CreateRenderTargetView(pResource, pDesc (NULL: mip 0 in the
		// texture's format), ppRTView)
		hr := comCall(dev, devCreateRenderTargetView, t.tex, 0, uintptr(unsafe.Pointer(&t.rtv)))
		if err := hrError("CreateRenderTargetView", hr); err != nil {
			t.rtv = 0
			return t, err
		}
	}
	return t, nil
}

// release drops the views and the texture.
func (t *texture) release() {
	release(t.rtv)
	release(t.srv)
	release(t.tex)
	*t = texture{}
}

// ensureTextures (re)creates the render target and the mask, color and
// ramp textures whose size changed (or that do not exist). The old
// contents are dropped: a new target holds no frame (hasFrame false)
// until a frame completes, and the Renderer re-uploads what it samples
// from a recreated texture.
func (d *Device) ensureTextures(f *gpu.Frame) error {
	if d.target.tex == 0 || d.target.size != f.Size {
		d.target.release()
		d.hasFrame = false
		t, err := newTexture(d.dev, f.Size, targetSpec, nil, 0)
		if err != nil {
			return err
		}
		d.target = t
	}
	for i, want := range [3]image.Point{gpu.TexMask: f.MaskSize, gpu.TexColor: f.ColorSize, gpu.TexRamp: f.RampSize} {
		if d.tex[i].tex != 0 && d.tex[i].size == want {
			continue
		}
		d.tex[i].release()
		t, err := newTexture(d.dev, want, sampledSpecs[i], nil, 0)
		if err != nil {
			return err
		}
		d.tex[i] = t
	}
	return nil
}

// checkUploads validates every upload of f (the frame's and each batch's)
// before anything is issued, so an invalid frame changes nothing but the
// textures ensureTextures recreated.
func (d *Device) checkUploads(f *gpu.Frame) error {
	check := func(ups []gpu.Upload) error {
		for _, u := range ups {
			var size image.Point
			if bpp := uploadBPP(u.Tex); bpp != 0 {
				size = d.tex[u.Tex].size
			}
			if _, err := checkUpload(u, size); err != nil {
				return err
			}
		}
		return nil
	}
	if err := check(f.Uploads); err != nil {
		return err
	}
	for i := range f.Batches {
		if err := check(f.Batches[i].Uploads); err != nil {
			return fmt.Errorf("batch %d: %w", i, err)
		}
	}
	return nil
}

// upload writes validated uploads (checkUploads) into their textures, in
// order. UpdateSubresource is queued in command order on the immediate
// context: draws issued earlier still sample the texture's old contents
// (the runtime or driver copies the pixels before returning and renames
// or stalls as needed), which is what gpu.Batch.Uploads requires.
func (d *Device) upload(ups []gpu.Upload) {
	for i := range ups {
		u := &ups[i]
		w, h := u.Rect.Dx(), u.Rect.Dy()
		if w <= 0 || h <= 0 {
			continue
		}
		bpp := uploadBPP(u.Tex)
		pitch := u.Stride
		if h == 1 {
			pitch = max(pitch, w*bpp) // unused for one row; keep it plausible
		}
		b := uploadBox(u.Rect)
		// UpdateSubresource(pDstResource, DstSubresource, pDstBox,
		// pSrcData, SrcRowPitch, SrcDepthPitch). It reads h-1 rows of
		// pitch plus one of w*bpp bytes from pSrcData, all inside u.Pix
		// (checkUpload), and copies them before returning.
		comCall(d.ctx, ctxUpdateSubresource, d.tex[u.Tex].tex, 0,
			uintptr(unsafe.Pointer(&b)), uintptr(unsafe.Pointer(&u.Pix[0])), uintptr(pitch), 0)
	}
}

// checkImages validates f's image uploads and checks that every batch's
// Image is held after ImageFrees and ImageUploads, before anything is
// changed (an invalid frame is not a failure).
func (d *Device) checkImages(f *gpu.Frame) error {
	for _, u := range f.ImageUploads {
		if err := checkImageUpload(u); err != nil {
			return err
		}
	}
	for i := range f.Batches {
		id := f.Batches[i].Image
		if id == 0 {
			continue
		}
		held := d.images[id].tex != 0 && !slices.Contains(f.ImageFrees, id)
		uploaded := slices.ContainsFunc(f.ImageUploads, func(u gpu.ImageUpload) bool { return u.ID == id })
		if !held && !uploaded {
			return fmt.Errorf("d3d11: batch %d binds image %d, which the device does not hold", i, id)
		}
	}
	return nil
}

// applyImages releases the textures of f.ImageFrees, then creates an
// immutable texture from each of f.ImageUploads (validated by
// checkImages), replacing one of the same ID. A texture still bound by an
// earlier, queued draw stays alive until that draw ran (the context holds
// its own reference). A failed creation returns an allocError.
func (d *Device) applyImages(f *gpu.Frame) error {
	if d.images == nil {
		d.images = map[int]texture{}
	}
	for _, id := range f.ImageFrees {
		t := d.images[id]
		t.release()
		delete(d.images, id)
	}
	for _, u := range f.ImageUploads {
		pix, pitch := imageUploadPix(u)
		t, err := newTexture(d.dev, u.Size, imageSpec, pix, pitch)
		if err != nil {
			return &allocError{Err: err}
		}
		old := d.images[u.ID]
		old.release()
		d.images[u.ID] = t
	}
	return nil
}
