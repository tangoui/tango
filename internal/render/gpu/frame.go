// Package gpu is the device-independent half of Tango's GPU renderer.
//
// Renderer turns a paint.List into a Frame: instances of the quad
// uber-shader (internal/render/gpu/shaders) grouped into scissored batches, plus the
// texture uploads they sample. A Device (Metal, …) executes the Frame. The
// package itself never touches a GPU API, so the whole pipeline can be
// tested on the CPU with internal/render/gpu/cpuexec.
package gpu

import (
	"errors"
	"image"
	"image/color"

	"github.com/tangoui/tango/internal/render/gpu/shaders"
)

// Texture indices for Upload.Tex; they match the quad shader's texture
// slots.
const (
	TexMask  = 0 // R8 glyph atlas (shaders.Mask): its pages stacked vertically, 1 byte per texel
	TexColor = 1 // premultiplied RGBA patch page (shaders.Color), 4 bytes per texel
	// TexRamp holds the gradient color ramps (shaders.Ramp, sampled with
	// linear filtering): one row of paint.RampWidth premultiplied RGBA16Unorm
	// texels per gradient, 8 bytes per texel (4 little-endian uint16).
	TexRamp = 2
	// TexImage is the slot of the per-batch image texture (shaders.Img,
	// Batch.Image). Image textures are not written with Upload but created
	// whole by Frame.ImageUploads.
	TexImage = 3
	// TexLayer is the slot of the offscreen layer a batch's layer
	// composites sample (shaders.Layer, Batch.Layer). Layers are drawn by
	// the frame's batches (Batch.Target), never uploaded.
	TexLayer = 4
)

// LayerDesc describes an offscreen layer: a premultiplied RGBA8 render
// target the Renderer draws an opacity group into, then composites onto
// the target that encloses the group (see Frame.Layers).
//
// The layer holds the device pixels of Bounds (main-target coordinates):
// Bounds.Min is its texel (0, 0). Size is the layer texture's size, at
// least Bounds.Size() (the Renderer rounds it up so a device can reuse
// textures of one size for different groups); texels beyond Bounds are
// never sampled.
type LayerDesc struct {
	Size   image.Point
	Bounds image.Rectangle // device pixels, main-target coordinates
}

// ImageUpload creates the image texture ID (replacing a texture of that ID
// the device holds) of Size texels from premultiplied RGBA pixels: row y
// starts at Pix[y*Stride:] and holds Size.X texels of 4 bytes. The device
// keeps the texture across frames until ImageFrees names its ID.
type ImageUpload struct {
	ID     int
	Size   image.Point
	Pix    []byte
	Stride int
}

// Upload copies pixels into a texture before the frame's batches run.
// Rect is the destination rectangle in texels; row y of it starts at
// Pix[y*Stride:] and holds Rect.Dx() texels (1 byte each for TexMask,
// 4 for TexColor, 8 for TexRamp).
type Upload struct {
	Tex    int // TexMask, TexColor or TexRamp
	Rect   image.Rectangle
	Pix    []byte
	Stride int
}

// Batch is a run of instances drawn with one scissor rectangle.
type Batch struct {
	// Uploads are applied, in order, after every earlier batch has been
	// drawn and before this batch is drawn: the Renderer reuses the patch
	// page within a frame when it fills up, so a Device must keep uploads
	// and draws in command order (for example by copying from a staging
	// buffer with a blit encoded between the draws), not write the texture
	// while earlier draws may still read it. Usually empty.
	Uploads   []Upload
	Instances []shaders.Instance
	// Image is the ID of the image texture (ImageUpload) bound to TexImage
	// for this batch's KindImage instances; 0 when the batch has none (the
	// device binds any texture, which nothing samples).
	Image int
	// Scissor is the device-pixel rectangle drawing is limited to, in
	// main-target coordinates also when Target is a layer (the device
	// shifts it by the layer's -Bounds.Min and clips it to the layer); the
	// empty rectangle means the whole target.
	Scissor image.Rectangle
	// Target is the render target the batch draws into: 0 the frame's
	// target, n > 0 the layer Frame.Layers[n-1].
	Target int
	// Layer is the layer (n > 0: Frame.Layers[n-1]) bound to TexLayer for
	// the batch's layer composites (KindPatch instances with Kind.W = 1);
	// 0 when the batch has none. A batch never samples its own Target, and
	// only layers an earlier batch of the frame has drawn into: a device
	// rejects a frame whose batch samples a layer nothing drew into
	// (Execute returns an error).
	Layer int
}

// Frame is everything a Device needs to draw one frame. The Renderer
// reuses it between frames: a Device must not keep references to it (or to
// its slices) after Execute returns.
type Frame struct {
	// Size is the render target size in device pixels. A frame with a
	// zero or negative dimension has no pixels: Execute returns nil
	// without drawing or submitting anything.
	Size image.Point
	// Clear is the color the target is cleared to first: all of it, or in
	// a partial frame (see Dirty) only the Dirty rectangles.
	Clear color.NRGBA
	// Dirty, when not empty, makes the frame partial: the frame's target
	// keeps the pixels of the previous frame (the Renderer sends a partial
	// frame only to a Retainer that reports them retained), Copies are
	// applied, then only the Dirty rectangles are cleared to Clear and
	// drawn: every batch drawing into the frame's target is drawn once per
	// Dirty rectangle with its scissor intersected with that rectangle.
	// Dirty rectangles are disjoint, inside the target and at most
	// maxDirty. Layers are drawn as in a full frame. A device that does not
	// hold the previous frame (its target was recreated, or a frame failed
	// in a way that may have touched the target) returns an error wrapping
	// ErrNotRetained without drawing anything.
	Dirty []image.Rectangle
	// Copies move pixels within the frame's target before anything is
	// cleared or drawn (partial frames only). Every Src reads the pixels
	// as they were before any copy of the frame (a copy's own Src and Dst
	// may overlap), and the copies never overlap each other: no copy's Src
	// or Dst overlaps another copy's Src or Dst (the Renderer drops
	// overlapping ScrollRegions), so their order does not matter. A frame
	// with Copies but no Dirty is invalid: the device returns an error
	// instead of executing it as a full frame (which would clear the
	// target and drop the copies).
	Copies []CopyRegion
	// Uploads are applied, in order, before any batch is drawn (and before
	// the first batch's own Uploads).
	Uploads []Upload
	Batches []Batch
	// MaskSize, ColorSize and RampSize are the sizes of the TexMask,
	// TexColor and TexRamp textures. When one changes the device recreates
	// that texture; the old contents are dropped and the Renderer
	// re-uploads what it samples. A zero size is valid for a frame that
	// neither uploads to nor samples that texture (the device binds a
	// placeholder).
	MaskSize, ColorSize, RampSize image.Point
	// ImageFrees are image texture IDs the device releases, then
	// ImageUploads are created; both before Uploads and any batch. An ID
	// is never reused after it is freed.
	ImageFrees   []int
	ImageUploads []ImageUpload
	// Layers are this frame's offscreen layers (Batch.Target,
	// Batch.Layer): layer n is Layers[n-1]. A layer exists for this frame
	// only. It is transparent before the first batch drawing into it; a
	// device allocates its texture at that batch (one no batch draws into
	// needs none) and may give the texture to another layer, of the same
	// Size, once no later batch draws into or samples the layer.
	//
	// A device may also back a layer with a larger texture (one pooled for
	// a larger group): the layer then occupies the texture's top-left
	// Size texels, the batches drawing into it run with ViewSize the
	// texture's size, and the device rescales the UVs (and the half-texel
	// in Kind.Y, Kind.Z) of the layer composites sampling it, which the
	// Renderer normalizes by Size, to the texture's size.
	//
	// A batch drawing into a layer runs the quad shader with
	// shaders.FrameU.ViewSize the layer's Size (or its texture's size, see
	// above) and FrameU.Origin its
	// Bounds.Min, so instance geometry stays in main-target device pixels
	// (and so do the pixel positions the shader evaluates gradients,
	// dithering and checkerboards at, as paint.CPU evaluates them inside
	// groups); for the frame's target Origin is zero.
	Layers []LayerDesc
}

// CopyRegion copies the pixels of Src to Dst (same size; both inside the
// frame's target).
type CopyRegion struct {
	Src, Dst image.Rectangle
}

// ErrNotRetained is returned (wrapped) by a device asked to execute a
// partial frame (Frame.Dirty) whose target does not hold the previous
// frame's pixels; the Renderer then draws the whole frame.
var ErrNotRetained = errors.New("gpu: device does not retain the previous frame")

// Retainer is implemented by a Device whose frame target keeps its pixels
// from one Execute to the next, so the Renderer may send it partial frames
// (Frame.Dirty, Frame.Copies). Retained reports whether the target holds
// the last frame executed, at size; the Renderer also requires that frame
// to have been executed successfully by itself.
type Retainer interface {
	Retained(size image.Point) bool
}

// PixelReader is implemented by a Device that can read its frame target
// back (premultiplied RGBA of the last frame's Size): the Renderer's verify
// mode (SetVerify) uses it to compare a partial redraw with a full one.
// ReadPixels returns an error when the target holds no completed frame:
// before the first frame with pixels, and after a failed Execute that
// recreated the target (whose contents are then undefined).
type PixelReader interface {
	ReadPixels() (*image.RGBA, error)
}

// Device executes frames on a GPU (or, in tests, on the CPU).
//
// Batches run in order, each into its Target (see Frame.Layers): a device
// may split the frame into render passes wherever the target changes or a
// batch has Uploads, but must keep every batch's draws after those of the
// batches before it.
//
// Texture contract: textures persist across frames until the Frame says
// otherwise. An ImageUpload whose ID the device already holds replaces
// that texture in place (contents and size), without an ImageFrees entry
// first: after a failed Execute the Renderer cannot know whether the
// frame's uploads arrived, so it uploads the image again under the same
// ID, and the device must then sample the new pixels, not the old ones
// (TestImageReuploadReplacesHeldTexture). Likewise an Upload overwrites
// its rectangle, and a changed MaskSize, ColorSize or RampSize recreates
// that texture.
type Device interface {
	Execute(f *Frame) error
}
