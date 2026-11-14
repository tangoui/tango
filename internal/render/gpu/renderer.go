package gpu

import (
	"cmp"
	"errors"
	"image"
	"image/color"
	"log"
	"math"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/tangoui/tango/internal/render/gpu/atlas"
	"github.com/tangoui/tango/internal/render/gpu/shaders"
	"github.com/tangoui/tango/internal/render/shader/sl"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// Renderer implements paint.Renderer on top of a Device.
//
// Mapping of display-list ops:
//   - FillRect: one KindSolid instance (rounded corners by the shader's SDF).
//   - StrokeRect: one KindStroke instance (shaders.StrokeCoverage, the
//     formula paint.CPU draws with): width and dash pattern in device
//     pixels; only the first 4 dash entries are carried (paint.CPU
//     truncates the same way).
//   - Text: one KindMask instance per glyph. The glyph's mask
//     (paint.CPU.GlyphMask, placed by paint.GlyphPlacement exactly as
//     paint.CPU does; TextGamma already applied) lives in the persistent
//     mask atlas (internal/render/gpu/atlas), so only newly inserted glyphs are uploaded.
//     The atlas entry holds the mask's offset from the pen, so a glyph found
//     there is placed without asking paint.CPU again (one map lookup per
//     glyph); empty glyphs (spaces) are remembered too. A glyph not in the
//     atlas is culled before it is rasterized when the square of
//     glyphReach ems (times 1+|Skew|) plus 2 pixels around its pen position
//     misses the scissor; one in the atlas is culled by its exact box.
//     A glyph the atlas cannot take (larger than a page, or no page left
//     that this frame does not use) is drawn as a patch instead.
//   - FillPath: one KindMask instance sampling the path's coverage mask
//     (paint.CPU.PathMask: what paint.CPU fills with) in the mask atlas,
//     cached by (Path pointer, device transform, scale) — so the same
//     *Path must not change content (see paint.FillPath) — and rasterized
//     only on a miss. Under a transform the key holds the transform's
//     linear part and its translation's subpixel phase (1/4 px), and the
//     mask is placed at the translation's whole pixels (see pathMask), so
//     an icon's cached Path drawn inside a Translate is rasterized once
//     for every whole-pixel position. Paths whose device bounds miss the
//     scissor are not rasterized. A mask larger than an atlas page, or one
//     the atlas has no room for this frame, is a patch.
//   - ShadowRect: one KindShadow instance (shaders.ShadowCoverage, the
//     formula paint.CPU's shadow masks hold) on the device geometry
//     paint.ShadowPlacement gives (rectangle snapped to pixels, radii and
//     sigma quantized as paint.CPU's mask cache keys them); a shadow
//     without blur is a KindSolid instance like FillRect.
//   - Gradient and LinearGradient (a two-stop linear Gradient) with a
//     rect shape: one KindGradient instance. The stops' color ramp
//     (paint.GradientRamp, premultiplied by paint.RampPremul) lives in a
//     row of the ramp texture (TexRamp, rampCache: one row per distinct
//     stop set, LRU over rampRows rows, uploaded only when written); the
//     shader evaluates shaders.GradientT (paint.GradientT) at the pixel
//     center, samples the row with linear filtering and applies a 4×4
//     ordered dither before the 8-bit write, which paint.CPU reproduces
//     with paint.RampSample, paint.Dither4x4 and paint.DitherPremul. The
//     ramp texture is RGBA16Unorm so low-contrast gradients keep sub-level
//     steps for the dither to spread. A gradient with a Path shape is the
//     same instance over the path's cached coverage mask (as FillPath's;
//     Kind.W = 1, the mask's atlas rectangle in Dash), which multiplies the
//     dithered ramp color. One beyond the rows the frame's other gradients
//     leave, or whose path mask the atlas does not take, is a patch.
//   - Checker: one KindChecker instance (paint.CheckerSelect per pixel;
//     the two colors in Color and Dash).
//   - Image: one KindImage instance on the device rectangle (radii as
//     FillRect's), batched with its image's texture (Batch.Image, bound to
//     TexImage): the shader filters it with shaders.SampleCatmullRom
//     (paint.SampleCatmullRomF, what paint.CPU draws images with) at the
//     pixel center. The texture holds paint.PrepareImage's pixels
//     (premultiplied RGBA, reduced first per paint.ImageReduction) and is
//     cached by image value and reduction size (imageCache): uploaded once
//     (Frame.ImageUploads), freed after imageIdleFrames unused frames, when
//     the cache exceeds imageCacheBytes, or by InvalidateImage
//     (Frame.ImageFrees). An image over maxImageSide texels on a side, or
//     beyond the frame's upload budget (64 MB by default; logged once), is
//     a patch that frame.
//   - PushOpacity…Pop groups, as paint.CPU draws them (the body drawn
//     without the enclosing rounded clips, then composited through them
//     with the alpha quantized to 8 bits):
//     a group whose alpha quantizes to 0 draws nothing, one at 255 draws
//     its body directly (unless it holds a rounded clip inside an
//     enclosing rounded clip: then it takes a layer, so both apply). A group whose body is a single drawing op whose
//     instance multiplies Color (FillRect, StrokeRect, ShadowRect,
//     FillPath, Checker, one glyph of Text; clips around it allowed, but no
//     rounded clip inside a rounded clip) draws that op directly with its
//     color scaled by the alpha (no layer). Any other group is drawn
//     natively into an offscreen layer (Frame.Layers: the group's device
//     bounds within the scissor, Size rounded up to layerAlign) and
//     composited by one KindPatch instance with Kind.W = 1 sampling the
//     layer (TexLayer) with Color = alpha, under the enclosing clip.
//     Groups nest (a layer drawn into a layer). Before the frame is
//     executed, batches drawing into layers are moved ahead of the
//     frame target's batches (see hoistLayers), so the target is drawn in
//     one render pass however many groups the frame has.
//   - PushTransform…Pop groups: the Renderer keeps the composed transform
//     on its clip stack. Under a transform that only translates and scales
//     uniformly by a positive factor (nativeXform; paint.CPU's own fast
//     path) the body is drawn natively, as paint.CPU draws it: rects,
//     radii, stroke widths, dashes, shadows, images and clips are mapped
//     (mapOp, paint.CPU's execTransformed operation for operation); glyphs
//     are placed and sized at the device scale·m.A with the translation
//     added (glyphPlace, as paint.CPU's text); paths as above; gradients
//     and checkers with mapped points, rects and cells. A subtree whose
//     composed transform rotates, skews, mirrors or scales non-uniformly
//     is rasterized whole by paint.CPU.RenderRegion (with the enclosing
//     transform pushed first) into a patch in the RGBA patch page and
//     drawn as KindPatch instances. A transform composing to the identity
//     is ignored (its body draws natively). Every patch drawn under a
//     native transform (an op the atlas, ramp or image cache does not
//     take) carries that transform the same way.
//   - Ops this Renderer does not know (added to paint later; see
//     opHandling) are patches too, over the whole scissor, and an unknown
//     Push… op's whole group is one patch; hint ops (Damage,
//     ScrollRegion) draw nothing: they drive partial redraws (damage.go,
//     Frame.Dirty, Frame.Copies) on a Device that is a Retainer.
//   - Ops added by pointer (&paint.FillRect{…}, &paint.PushClip{…},
//     &paint.Pop{} …) are dereferenced first and drawn exactly as the same
//     ops added by value; a nil pointer op draws nothing. (paint.CPU
//     matches op values only and skips pointer ops, so add ops by value.)
//   - PushClip/Pop: a clip stack. Every clip's device rectangle (rounded as
//     paint.CPU rounds it) is intersected into the batch scissor; the
//     innermost rounded clip also goes to the instances' Clip/ClipRadii.
//     Known limitation: with more than one rounded clip active, the corners
//     of the outer rounded clips are clipped as plain rectangles for solid
//     and glyph instances and for patches.
//
// The patch page is one fixed square texture of patchPageSize texels,
// shelf-packed from empty every frame. When a patch does not fit, the page
// is flushed: packing restarts at the top, the following patch uploads go
// into Batch.Uploads of a new batch (applied after the earlier batches have
// drawn), and drawing continues; a patch larger than the page is split into
// page-sized tiles, each its own instance. So the texture never grows and
// any amount of patch area renders.
//
// Clip and opacity nesting is as tolerant as paint.CPU: extra Pops are
// ignored, unclosed pushes end with the list, and the depth is unbounded.
// Renderer is not safe for concurrent use.
type Renderer struct {
	dev   Device
	cpu   *paint.CPU
	frame Frame

	atlas      *atlas.Atlas
	atlasScale float32     // scale the atlas glyphs were rasterized at
	maskSent   image.Point // MaskSize the device holds all atlas pixels for; zero = unknown

	ramps *rampCache // gradient ramp rows (TexRamp)

	images      *imageCache // image textures (TexImage)
	imageBudget int         // bytes of image textures one frame uploads at most
	budgetHit   bool        // the image upload budget overflow was logged
	// logf logs (log.Printf; tests replace it).
	logf func(format string, args ...any)
	ops  []paint.Op // this frame's ops with pointer ops dereferenced (when there are any)

	patchSize int       // patch page side in texels
	patches   patchPage // this frame's packing of the patch page
	flushed   bool      // the patch page was flushed: color uploads go to pending
	newBatch  bool      // the next instance must start a new batch
	pending   []Upload  // color uploads for the next emitted instance's batch

	// glyphMask is paint.CPU.GlyphMask (tests count calls through it).
	glyphMask func(g paint.Glyph, size float32, sub uint8) (*image.Alpha, image.Point)
	// pathRaster is paint.CPU.PathMask (tests count calls through it).
	pathRaster func(p *paint.Path) (*image.Alpha, image.Point)
	// onPatch, when set (tests), sees every patch's ops and device region.
	onPatch func(ops []paint.Op, region image.Rectangle)
	// onRaster, when set (tests), sees the region every patch is
	// rasterized over.
	onRaster func(raster image.Rectangle)
	// misrouted counts, per frame, ops opHandling calls native that the
	// dispatch in Render has no case for; they are drawn as patches.
	// TestRendererHandlesEveryPaintOp requires it to stay 0.
	misrouted int

	// per-frame state
	scale float32
	full  image.Rectangle
	clips []clipState

	target     int     // where instances go now: 0 the frame's target, n layer Frame.Layers[n-1]
	alpha      float32 // a single-op group's alpha its op is drawn with (1 otherwise)
	groups     []group // open PushOpacity groups, innermost last
	layerDrawn []bool  // layerDrawn[n]: an instance went into layer n this frame
	scratch    []Batch // hoistLayers' reordering buffer

	// Partial redraw (damage.go).
	verify bool // verify mode (SetVerify)
	// verifyStrict makes a verify difference fail Render (SetVerifyStrict);
	// verifyFails and verifyLast count and keep them; renders counts
	// Render calls, verifyLogged is renders at the last logged difference
	// and verifySilent the differences not logged since.
	verifyStrict bool
	verifyFails  int
	verifyLast   *VerifyError
	renders      uint64
	verifyLogged uint64
	verifySilent int
	res          resident          // what the device's target holds
	root         image.Rectangle   // the root scissor: full, or the dirty rectangles' bounding box
	partial      bool              // this frame is partial (Frame.Dirty)
	dirty        []image.Rectangle // this frame's dirty rectangles (partial)
	copies       []CopyRegion      // this frame's copies (partial)
	scanStack    []scanState       // planPartial's scratch
	scanClips    []scanClip
	prevClips    []scanClip // the clips of the last frame planPartial scanned
	scanGroups   []scanGroup
	scanScrolls  []scanScroll
}

// groupKind is how a PushOpacity group is drawn.
type groupKind uint8

const (
	groupPlain groupKind = iota // alpha 1: the body directly
	groupFast                   // a single op, its color scaled by the alpha
	groupLayer                  // into an offscreen layer, then composited
)

// group is an open PushOpacity group.
type group struct {
	kind  groupKind
	end   int // index of the Pop closing it (len(ops): never closed)
	depth int // len(Renderer.clips) before the group's clip state was pushed
	layer int // groupLayer: the layer (Frame.Layers[layer-1])
	prev  int // groupLayer: the target to composite into
	alpha float32
}

// layerAlign is the granularity, in texels, layer sizes are rounded up
// to, so groups of similar sizes share a device's pooled textures.
const layerAlign = 64

// Default texture sizes. Pages are 2048 texels square (well within every
// Metal device's limit); the glyph atlas grows to 4 pages (2048×8192 R8).
const (
	patchPageSize = 2048
	atlasPageSize = 2048
	atlasPages    = 4
)

// clipState is the effective clip and transform after a push (PushClip,
// PushOpacity or a native PushTransform; the matching Pop restores the
// state before it).
type clipState struct {
	scissor image.Rectangle // device pixels; all active clips intersected
	// full is the scissor a full frame has here (scissor is smaller in a
	// partial frame): patches are rasterized over it, so their pixels do
	// not depend on the dirty region (see patch).
	full  image.Rectangle
	rect  sl.Vec4 // innermost rounded clip (device px); zero = none
	radii sl.Vec4
	// m is the current logical transform when xf is set (always a native
	// one: see nativeXform); without xf the transform is the identity.
	m  geom.Affine
	xf bool
}

// NewRenderer returns a Renderer drawing through dev.
func NewRenderer(dev Device) *Renderer {
	r := &Renderer{dev: dev, cpu: paint.NewCPU(), atlas: atlas.New(atlasPageSize, atlasPages), ramps: newRampCache(),
		images: newImageCache(), imageBudget: defaultImageBudget, logf: log.Printf, patchSize: patchPageSize, verify: verifyDefault,
		verifyStrict: testing.Testing()}
	r.glyphMask = r.cpu.GlyphMask
	r.pathRaster = r.cpu.PathMask
	return r
}

// InvalidateImage drops everything cached for img (by its image.Image
// value): its GPU textures, freed with the next frame, and paint.CPU's
// prepared pixels for patches. Call it after changing an image's pixels in
// place; the next frame drawing img uploads its current pixels.
func (r *Renderer) InvalidateImage(img image.Image) {
	r.images.invalidate(img)
	r.cpu.InvalidateImage(img)
}

// Trim releases what the renderer caches and can rebuild, for a low-memory
// warning (docs/specs/mobile-canvas.md §3.4): the glyph and path mask atlas
// shrinks to one empty page and every image texture is freed. The next
// frame re-rasterizes and re-uploads what it draws (the whole mask atlas
// texture, at its new size), so it costs more than usual but draws the
// same pixels. Main thread, between frames.
func (r *Renderer) Trim() {
	r.atlas.Reset()
	r.maskSent = image.Point{} // upload the shrunken atlas whole
	for _, t := range r.images.byKey {
		r.images.free(t) // freed on the device with the next frame
	}
}

// Render converts l into a Frame and executes it on the device. A frame
// with no pixels (width or height 0 after scaling) is skipped without
// calling the device. When l carries redraw hints and the device retains
// its target, only the damaged part is redrawn (see damage.go); a partial
// frame whose hints leave nothing to redraw is not executed at all.
func (r *Renderer) Render(l *paint.List, size geom.Size, scale float32) error {
	if scale <= 0 {
		scale = 1
	}
	fw, fh := float64(size.W*scale), float64(size.H*scale)
	if !(fw > 0 && fh > 0) || math.IsInf(fw, 0) || math.IsInf(fh, 0) {
		return nil
	}
	w, h := int(math.Ceil(fw)), int(math.Ceil(fh))
	fs := image.Pt(w, h)
	r.renders++
	ops := r.values(l.Ops)
	defer clear(r.ops) // drop references to the list

	r.scale = scale
	partial := r.planPartial(ops, fs, scale, l.Background)
	r.res.ok = false // until this frame has executed
	err := r.frame1(l.Background, ops, fs, scale, partial)
	if partial && errors.Is(err, ErrNotRetained) {
		partial = false
		err = r.frame1(l.Background, ops, fs, scale, false)
	}
	if err == nil && partial && r.verify {
		err = r.verifyPartial(l.Background, ops, fs, scale)
		var ve *VerifyError
		if errors.As(err, &ve) {
			err = r.verifyFailed(ve) // nil unless strict: the target holds the full frame
		}
	}
	r.res = resident{ok: err == nil, size: fs, scale: scale, bg: l.Background}
	return err
}

// frame1 builds the frame for ops (partial: drawing only r.dirty, after
// r.copies) and executes it.
func (r *Renderer) frame1(bg color.NRGBA, ops []paint.Op, fs image.Point, scale float32, partial bool) error {
	w, h := fs.X, fs.Y
	if partial && len(r.dirty) == 0 {
		return nil // nothing changed (planPartial leaves no copies without dirty rectangles)
	}

	// Count this as a frame for the glyph cache that GlyphMask shares, so
	// unused glyphs are evicted as in paint.CPU.Render.
	r.cpu.BeginFrame(scale)
	if scale != r.atlasScale {
		r.atlas.Reset() // glyphs are rasterized at device size
		r.atlasScale = scale
	}
	r.atlas.BeginFrame()
	r.ramps.beginFrame()
	r.images.beginFrame()

	r.scale = scale
	r.full = image.Rect(0, 0, w, h)
	r.root, r.partial = r.full, partial
	if partial {
		r.root = image.Rectangle{}
		for _, d := range r.dirty {
			r.root = r.root.Union(d)
		}
	}
	r.clips = r.clips[:0]
	r.patches.reset(r.patchSize)
	r.flushed, r.newBatch = false, false
	r.misrouted = 0
	r.target, r.alpha = 0, 1
	r.groups = r.groups[:0]
	r.layerDrawn = append(r.layerDrawn[:0], false)
	clear(r.pending)
	r.pending = r.pending[:0]
	f := &r.frame
	f.Size = image.Pt(w, h)
	f.Clear = bg
	f.Dirty, f.Copies = nil, nil
	if partial {
		f.Dirty, f.Copies = r.dirty, r.copies
	}
	// Reuse the slices; drop references to last frame's pixels.
	clear(f.Uploads)
	f.Uploads = f.Uploads[:0]
	all := f.Batches[:cap(f.Batches)]
	for i := range all {
		clear(all[i].Uploads)
		all[i].Uploads = all[i].Uploads[:0]
	}
	f.Batches = f.Batches[:0]
	f.Layers = f.Layers[:0]

	for i := 0; i < len(ops); i++ {
		op := ops[i]
		if c := r.clip(); c.xf {
			// What paint.CPU's execTransformed does for a uniform
			// scale+translate: these ops draw as their mapped selves.
			op = mapOp(op, c.m)
		}
		switch o := op.(type) {
		case paint.FillRect:
			r.fillRect(o)
		case paint.Gradient:
			r.gradient(o)
		case paint.LinearGradient:
			r.gradient(paint.Gradient{Kind: paint.GradientLinear, Rect: o.Rect, Radii: o.Radii, Start: o.P0, End: o.P1,
				Stops: []paint.Stop{{Offset: 0, Color: o.From}, {Offset: 1, Color: o.To}}})
		case paint.Checker:
			r.checker(o)
		case paint.Image:
			r.image(o)
		case paint.StrokeRect:
			r.strokeRect(o)
		case paint.ShadowRect:
			r.shadowRect(o)
		case paint.Text:
			r.text(o)
		case paint.FillPath:
			r.fillPath(o)
		case paint.PushClip:
			r.pushClip(o)
		case paint.Pop:
			if n := len(r.groups); n > 0 && r.groups[n-1].end == i {
				r.endGroup()
			} else if len(r.clips) > 0 {
				r.clips = r.clips[:len(r.clips)-1]
			}
		case paint.PushOpacity:
			i = r.pushOpacity(o, ops, i)
		case paint.PushTransform:
			c := r.clip()
			m := o.M
			if c.xf {
				m = c.m.Mul(o.M)
			}
			switch {
			case m.IsIdentity():
				// Nothing to transform: draw the body natively; the
				// matching Pop pops this copy of the clip state.
				c.m, c.xf = geom.Affine{}, false
				r.clips = append(r.clips, c)
			case nativeXform(m):
				// Translate and uniform positive scale: the body draws
				// natively with its geometry mapped.
				c.m, c.xf = m, true
				r.clips = append(r.clips, c)
			default:
				// Rotation, skew, mirroring, non-uniform scale: paint.CPU
				// draws the whole subtree (transforms included).
				end := matchingPop(ops, i)
				group := ops[i:min(end+1, len(ops))]
				r.patch(r.inXform(group), r.bounds(group), 1)
				i = end
			}
		default:
			kind := opHandling[opName(o)]
			if reflect.TypeOf(o).Kind() == reflect.Pointer {
				// A pointer values left as is (its element is not an Op):
				// the patch draws what paint.CPU draws for it (nothing).
				kind = opPatch
			} else if kind == opNative {
				// The table says native but the switch above has no case:
				// never fail the frame, draw it as paint.CPU does.
				r.misrouted++
				kind = opPatch
			}
			switch kind {
			case opHint:
				// Not drawn by renderers.
			default: // opPatch, or an op this renderer does not know yet
				if isPush(o) {
					end := matchingPop(ops, i)
					group := ops[i:min(end+1, len(ops))]
					r.patch(r.inXform(group), r.bounds(group), 1)
					i = end
				} else {
					r.patch(r.inXform(ops[i:i+1]), r.opBounds(ops[i]), 1)
				}
			}
		}
	}

	for len(r.groups) > 0 { // unclosed groups end with the list
		r.endGroup()
	}
	r.hoistLayers()

	px := r.atlas.Pixels()
	f.MaskSize = px.Rect.Size()
	f.ColorSize = image.Pt(r.patchSize, r.patchSize)
	f.RampSize = r.ramps.size()
	f.Uploads = r.ramps.uploads(f.Uploads)
	r.images.frameLists(f)
	if f.MaskSize != r.maskSent {
		// New texture (atlas grew or shrank, or the last frame failed):
		// upload the whole atlas.
		r.atlas.TakeDirty()
		f.Uploads = append(f.Uploads, Upload{Tex: TexMask, Rect: px.Rect, Pix: px.Pix, Stride: px.Stride})
	} else {
		for _, d := range coalesce(r.atlas.TakeDirty()) {
			f.Uploads = append(f.Uploads, Upload{Tex: TexMask, Rect: d, Pix: px.Pix[px.PixOffset(d.Min.X, d.Min.Y):], Stride: px.Stride})
		}
	}
	// UVs were recorded in texels; normalize now that the pages are sized.
	for bi := range f.Batches {
		ins := f.Batches[bi].Instances
		for k := range ins {
			var tex image.Point
			switch ins[k].Kind.X {
			case shaders.KindGradient:
				if ins[k].Kind.W > 0.5 { // Path-shaped: the mask's atlas rectangle
					tw, th := float32(f.MaskSize.X), float32(f.MaskSize.Y)
					d := &ins[k].Dash
					*d = sl.V4(d.X/tw, d.Y/th, d.Z/tw, d.W/th)
				}
				continue
			case shaders.KindMask:
				tex = f.MaskSize
			case shaders.KindPatch:
				if ins[k].Kind.W > 0.5 {
					continue // a layer composite: normalized by endGroup
				}
				tex = f.ColorSize
			default:
				continue
			}
			tw, th := float32(tex.X), float32(tex.Y)
			u := &ins[k].UV
			*u = sl.V4(u.X/tw, u.Y/th, u.Z/tw, u.W/th)
			ins[k].Kind.Y, ins[k].Kind.Z = 0.5/tw, 0.5/th
		}
	}
	if err := r.dev.Execute(f); err != nil {
		r.maskSent = image.Point{} // the device may have missed this frame's uploads
		r.ramps.sent = false
		r.images.executed(false)
		return err
	}
	r.maskSent = f.MaskSize
	r.ramps.sent = true
	r.images.executed(true)
	return nil
}

// coalesceAbove is the number of dirty atlas rectangles up to which each
// is uploaded on its own. A frame inserting more glyphs (each insert is one
// dirty rectangle) uploads row bands instead: per-upload cost on a device
// (one blit each) dominates small copies — on Metal (Apple M4) a frame
// with 500 20×22 uploads took 3.3–7.6 ms against 0.5–1 ms for one upload of
// the same rows (internal/render/gpu/metal BenchmarkMetalUploads).
const coalesceAbove = 16

// coalesce merges more than coalesceAbove dirty atlas rectangles, in place,
// into row bands: rectangles whose rows overlap (the atlas's shelves) become
// their bounding box. If that still leaves more than coalesceAbove bands,
// they become one bounding box. The texels a band adds between dirty
// rectangles are uploaded unchanged from the atlas image, which the device
// texture mirrors.
func coalesce(d []image.Rectangle) []image.Rectangle {
	if len(d) <= coalesceAbove {
		return d
	}
	slices.SortFunc(d, func(a, b image.Rectangle) int { return cmp.Compare(a.Min.Y, b.Min.Y) })
	out := d[:1]
	for _, r := range d[1:] {
		if last := &out[len(out)-1]; r.Min.Y < last.Max.Y {
			*last = last.Union(r)
		} else {
			out = append(out, r)
		}
	}
	if len(out) > coalesceAbove {
		u := out[0]
		for _, b := range out[1:] {
			u = u.Union(b)
		}
		out = append(out[:0], u)
	}
	return out
}

// How the Renderer draws each paint.Op type, by type name. Every type in
// package paint with an isOp method must be listed
// (TestRendererHandlesEveryPaintOp); the dispatch in Render handles the
// opNative ops itself and uses this table for the rest. An op missing from
// the table (one added to paint later) is drawn as a patch.
type opKind uint8

const (
	opPatch  opKind = iota // rasterized by paint.CPU into a patch
	opNative               // drawn with shader instances (or a clip/group the Renderer tracks)
	opHint                 // not drawn (renderer hints)
)

var opHandling = map[string]opKind{
	"FillRect":       opNative,
	"StrokeRect":     opNative,
	"Text":           opNative,
	"PushClip":       opNative,
	"PushOpacity":    opNative, // a layer, or the body drawn directly (see Renderer)
	"PushTransform":  opNative, // translate/uniform scale: native; other transforms: the subtree is one patch
	"Pop":            opNative,
	"ShadowRect":     opNative,
	"FillPath":       opNative, // too large or no atlas room: a patch (Render's fillPath)
	"LinearGradient": opNative, // a two-stop linear Gradient
	"Gradient":       opNative, // no ramp row or path mask: a patch (Render's gradient)
	"Checker":        opNative,
	"Image":          opNative, // too large or over the upload budget: a patch (Render's image)
	"Damage":         opHint,
	"ScrollRegion":   opHint,
}

// values returns ops with every pointer op (&paint.FillRect{…}) replaced
// by the value it points to, so pointer ops draw (and push/pop) exactly as
// value ops; nil ops and nil pointer ops are dropped (paint.CPU ignores
// them). Without such ops it returns ops itself; otherwise a copy in r.ops
// (cleared after the frame).
func (r *Renderer) values(ops []paint.Op) []paint.Op {
	i := slices.IndexFunc(ops, func(op paint.Op) bool { return op == nil || reflect.TypeOf(op).Kind() == reflect.Pointer })
	if i < 0 {
		return ops
	}
	out := append(r.ops[:0], ops[:i]...)
	for _, op := range ops[i:] {
		if op == nil {
			continue
		}
		if v := reflect.ValueOf(op); v.Kind() == reflect.Pointer {
			if v.IsNil() {
				continue
			}
			if e, ok := v.Elem().Interface().(paint.Op); ok {
				op = e
			}
		}
		out = append(out, op)
	}
	r.ops = out
	return out
}

// opName is the type name of op (its key in opHandling).
func opName(op paint.Op) string {
	t := reflect.TypeOf(op)
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Name()
}

// isPush reports whether op opens a group closed by a Pop: the known
// pushes, and any other op whose type name starts with "Push" (paint's
// naming convention), so a future group op is skipped as a whole.
func isPush(op paint.Op) bool {
	switch op.(type) {
	case paint.PushClip, paint.PushOpacity, paint.PushTransform:
		return true
	case paint.Pop:
		return false
	}
	return strings.HasPrefix(opName(op), "Push")
}

// matchingPop returns the index of the Pop closing the push at ops[i], or
// len(ops) if it is never closed.
func matchingPop(ops []paint.Op, i int) int {
	depth := 0
	for j := i; j < len(ops); j++ {
		if isPush(ops[j]) {
			depth++
			continue
		}
		switch ops[j].(type) {
		case paint.Pop:
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return len(ops)
}

// nativeXform reports whether the Renderer draws content under the
// logical transform m natively: m only translates and scales uniformly by
// a positive factor (paint.CPU's own fast path: execTransformed maps such
// ops and draws them as untransformed ones), with finite coefficients.
func nativeXform(m geom.Affine) bool {
	for _, v := range [...]float32{m.A, m.D, m.E, m.F} {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return false
		}
	}
	return m.AxisAligned() && m.A == m.D
}

// mapOp returns op under the native transform m as the untransformed op
// paint.CPU draws for it (execTransformed's uniform case, operation for
// operation): rects mapped by m.Bounds, radii, stroke widths, dashes and
// shadow offsets, blurs and spreads scaled by m.A. Text, paths, gradients
// and checkers are returned unchanged: their drawing functions apply the
// transform (r.clip().m) themselves, as paint.CPU's do.
func mapOp(op paint.Op, m geom.Affine) paint.Op {
	k := m.A
	mrad := func(rd geom.Radii) geom.Radii {
		return geom.Radii{TL: rd.TL * k, TR: rd.TR * k, BR: rd.BR * k, BL: rd.BL * k}
	}
	switch o := op.(type) {
	case paint.FillRect:
		o.Rect, o.Radii = m.Bounds(o.Rect), mrad(o.Radii)
		return o
	case paint.StrokeRect:
		o.Rect, o.Radii, o.Width = m.Bounds(o.Rect), mrad(o.Radii), o.Width*k
		if len(o.Dash) > 0 {
			d := make([]float32, len(o.Dash))
			for i, v := range o.Dash {
				d[i] = v * k
			}
			o.Dash = d
		}
		return o
	case paint.ShadowRect:
		o.Rect, o.Radii = m.Bounds(o.Rect), mrad(o.Radii)
		o.Shadow.X, o.Shadow.Y = o.Shadow.X*k, o.Shadow.Y*k
		o.Shadow.Blur, o.Shadow.Spread = o.Shadow.Blur*k, o.Shadow.Spread*k
		return o
	case paint.Image:
		o.Rect, o.Radii = m.Bounds(o.Rect), mrad(o.Radii)
		return o
	case paint.PushClip:
		o.Rect, o.Radii = m.Bounds(o.Rect), mrad(o.Radii)
		return o
	}
	return op
}

// inXform returns ops (in the current transform's logical space) for a
// patch: wrapped in a PushTransform of the current transform when there
// is one, since paint.CPU.RenderRegion starts untransformed.
func (r *Renderer) inXform(ops []paint.Op) []paint.Op {
	c := r.clip()
	if !c.xf {
		return ops
	}
	out := make([]paint.Op, 0, len(ops)+2)
	out = append(out, paint.PushTransform{M: c.m})
	out = append(out, ops...)
	return append(out, paint.Pop{})
}

func (r *Renderer) clip() clipState {
	if len(r.clips) == 0 {
		return clipState{scissor: r.root, full: r.full}
	}
	return r.clips[len(r.clips)-1]
}

func (r *Renderer) pushClip(o paint.PushClip) {
	c := r.clip()
	c.scissor = c.scissor.Intersect(r.devRect(o.Rect))
	c.full = c.full.Intersect(r.devRect(o.Rect))
	if !o.Radii.IsZero() {
		s := r.scale
		c.rect = sl.V4(o.Rect.X*s, o.Rect.Y*s, o.Rect.W*s, o.Rect.H*s)
		c.radii = sl.V4(o.Radii.TL*s, o.Radii.TR*s, o.Radii.BR*s, o.Radii.BL*s)
	}
	r.clips = append(r.clips, c)
}

// emit appends in to the current batch, starting a new batch when the
// scissor changes. bounds is the instance's device bounding box, used to
// drop instances that cannot touch the scissor.
func (r *Renderer) emit(in shaders.Instance, bounds image.Rectangle) {
	r.emitTex(in, bounds, 0, 0)
}

// emitImage is emit for an instance sampling image texture img (0: none).
func (r *Renderer) emitImage(in shaders.Instance, bounds image.Rectangle, img int) {
	r.emitTex(in, bounds, img, 0)
}

// emitTex is emit for an instance sampling image texture img and layer
// layer (0: none). It goes into the current target (r.target). A batch
// binds one image texture and one layer, so a different image or layer
// than the current batch's starts a new batch (instances of other kinds
// join any batch), as does a different target. Inside a single-op opacity
// group the instance's color is scaled by the group's alpha.
func (r *Renderer) emitTex(in shaders.Instance, bounds image.Rectangle, img, layer int) {
	c := r.clip()
	if c.scissor.Empty() || !bounds.Overlaps(c.scissor) {
		return
	}
	if r.partial && r.target == 0 && len(r.dirty) > 1 && !overlapsAny(bounds, r.dirty) {
		return // between the dirty rectangles: the device would not draw it
	}
	if r.alpha != 1 {
		in.Color = in.Color.Scale(r.alpha)
		if in.Kind.X == shaders.KindChecker {
			in.Dash = in.Dash.Scale(r.alpha) // its second color
		}
	}
	in.Clip, in.ClipRadii = c.rect, c.radii
	sc := c.scissor
	if sc == r.full && r.target == 0 {
		sc = image.Rectangle{}
	}
	f := &r.frame
	if n := len(f.Batches); n == 0 || r.newBatch || f.Batches[n-1].Scissor != sc || f.Batches[n-1].Target != r.target ||
		(img != 0 && f.Batches[n-1].Image != 0 && f.Batches[n-1].Image != img) ||
		(layer != 0 && f.Batches[n-1].Layer != 0 && f.Batches[n-1].Layer != layer) {
		if n < cap(f.Batches) {
			// Reuse the batch and its slices (Uploads were emptied in Render).
			f.Batches = f.Batches[:n+1]
			b := &f.Batches[n]
			b.Scissor = sc
			b.Image, b.Layer, b.Target = 0, 0, r.target
			b.Instances = b.Instances[:0]
		} else {
			f.Batches = append(f.Batches, Batch{Scissor: sc, Target: r.target})
		}
		r.newBatch = false
	}
	b := &f.Batches[len(f.Batches)-1]
	if img != 0 {
		b.Image = img
	}
	if layer != 0 {
		b.Layer = layer
	}
	r.layerDrawn[r.target] = true
	if len(r.pending) > 0 {
		b.Uploads = append(b.Uploads, r.pending...)
		clear(r.pending)
		r.pending = r.pending[:0]
	}
	b.Instances = append(b.Instances, in)
}

func (r *Renderer) fillRect(o paint.FillRect) {
	if o.Color.A == 0 || o.Rect.Empty() {
		return
	}
	s := r.scale
	r.emit(shaders.Instance{
		Rect:  sl.V4(o.Rect.X*s, o.Rect.Y*s, o.Rect.W*s, o.Rect.H*s),
		Radii: sl.V4(o.Radii.TL*s, o.Radii.TR*s, o.Radii.BR*s, o.Radii.BL*s),
		Color: premul(o.Color),
		Kind:  sl.V4(shaders.KindSolid, 0, 0, 0),
	}, r.outBounds(o.Rect, 1))
}

func (r *Renderer) strokeRect(o paint.StrokeRect) {
	if o.Color.A == 0 || o.Rect.Empty() || !(o.Width > 0) {
		return
	}
	s := r.scale
	var d [4]float32 // longer patterns are truncated, as in paint.CPU
	for i := 0; i < len(o.Dash) && i < len(d); i++ {
		d[i] = o.Dash[i] * s
	}
	r.emit(shaders.Instance{
		Rect:   sl.V4(o.Rect.X*s, o.Rect.Y*s, o.Rect.W*s, o.Rect.H*s),
		Radii:  sl.V4(o.Radii.TL*s, o.Radii.TR*s, o.Radii.BR*s, o.Radii.BL*s),
		Color:  premul(o.Color),
		Kind:   sl.V4(shaders.KindStroke, 0, 0, 0),
		Params: sl.V4(o.Width*s, 0, 0, 0),
		Dash:   sl.V4(d[0], d[1], d[2], d[3]),
	}, r.outBounds(o.Rect, 1))
}

func (r *Renderer) shadowRect(o paint.ShadowRect) {
	if o.Shadow.Color.A == 0 {
		return
	}
	g, blurred := paint.ShadowPlacement(o, r.scale)
	if !blurred {
		r.fillRect(paint.FillRect{Rect: g.Shape, Radii: g.ShapeRadii, Color: o.Shadow.Color})
		return
	}
	if g.Rect.Empty() {
		return
	}
	r.emit(shaders.Instance{
		Rect:   rectV4(g.Rect),
		Radii:  sl.V4(g.Radii.TL, g.Radii.TR, g.Radii.BR, g.Radii.BL),
		Color:  premul(o.Shadow.Color),
		Kind:   sl.V4(shaders.KindShadow, 0, 0, 0),
		Params: sl.V4(g.Sigma, float32(g.Margin), 0, 0),
	}, g.Rect.Inset(-g.Margin-1))
}

// gradient draws a gradient as one KindGradient instance sampling its
// stops' ramp row (see Renderer): over its rounded rect, or over its
// Path's coverage mask; one that gets no ramp row (or path mask) is a
// patch. Gradients paint.CPU drops (no stops, non-finite points, empty
// rect) draw nothing.
func (r *Renderer) gradient(o paint.Gradient) {
	if !paint.GradientDrawable(o) {
		return
	}
	var kind float32 // linear, also for unknown kinds (as in paint.GradientT)
	switch o.Kind {
	case paint.GradientRadial:
		kind = 1
	case paint.GradientConic:
		kind = 2
	}
	s := r.scale
	// Under a native transform the gradient is evaluated at device pixels
	// with mapped points: GradientT is invariant under translation and
	// uniform positive scale, so this is what paint.CPU computes at the
	// pixel's logical position.
	c := r.clip()
	start, end := o.Start, o.End
	rect, rad := o.Rect, o.Radii
	if c.xf {
		start, end = c.m.Apply(start), c.m.Apply(end)
		rect, rad = c.m.Bounds(rect), scaleRadii(rad, c.m.A)
	}
	if o.Path != nil {
		e, at, st := r.pathMask(o.Path)
		if st == pathPatch {
			r.patch(r.inXform([]paint.Op{o}), r.opBounds(o), 1)
		}
		if st != pathOK {
			return
		}
		row, ok := r.ramps.row(o.Stops)
		if !ok {
			r.patch(r.inXform([]paint.Op{o}), r.opBounds(o), 1)
			return
		}
		dr := image.Rectangle{Min: at, Max: at.Add(e.Rect.Size())}
		r.emit(shaders.Instance{
			Rect:   rectV4(dr),
			UV:     sl.V4(0, (float32(row)+0.5)/rampRows, 0, 0), // normalized already
			Kind:   sl.V4(shaders.KindGradient, kind, 0, 1),
			Params: sl.V4(start.X*s, start.Y*s, end.X*s, end.Y*s),
			Dash:   rectV4(e.Rect), // mask texels; normalized in Render
		}, dr)
		return
	}
	if !(rect.W > 0) || !(rect.H > 0) {
		return
	}
	b := r.outBounds(rect, 1)
	if c.scissor.Empty() || !b.Overlaps(c.scissor) {
		return // not drawn: do not take a ramp row for it
	}
	row, ok := r.ramps.row(o.Stops)
	if !ok {
		r.patch(r.inXform([]paint.Op{o}), b, 1)
		return
	}
	r.emit(shaders.Instance{
		Rect:   sl.V4(rect.X*s, rect.Y*s, rect.W*s, rect.H*s),
		Radii:  sl.V4(rad.TL*s, rad.TR*s, rad.BR*s, rad.BL*s),
		UV:     sl.V4(0, (float32(row)+0.5)/rampRows, 0, 0), // normalized already
		Kind:   sl.V4(shaders.KindGradient, kind, 0, 0),
		Params: sl.V4(start.X*s, start.Y*s, end.X*s, end.Y*s),
	}, b)
}

// scaleRadii returns rd scaled by k.
func scaleRadii(rd geom.Radii, k float32) geom.Radii {
	return geom.Radii{TL: rd.TL * k, TR: rd.TR * k, BR: rd.BR * k, BL: rd.BL * k}
}

// checker draws a Checker as one KindChecker instance.
func (r *Renderer) checker(o paint.Checker) {
	if !(o.Cell > 0) || !(o.Rect.W > 0) || !(o.Rect.H > 0) {
		return
	}
	a, b := paint.CheckerColors(o)
	s := r.scale
	if c := r.clip(); c.xf {
		// Cells mapped like the rect (paint.CPU selects at the pixel's
		// logical position: the same cell under a uniform scale+translate).
		o.Rect, o.Radii, o.Cell = c.m.Bounds(o.Rect), scaleRadii(o.Radii, c.m.A), o.Cell*c.m.A
	}
	r.emit(shaders.Instance{
		Rect:   sl.V4(o.Rect.X*s, o.Rect.Y*s, o.Rect.W*s, o.Rect.H*s),
		Radii:  sl.V4(o.Radii.TL*s, o.Radii.TR*s, o.Radii.BR*s, o.Radii.BL*s),
		Color:  premul(a),
		Dash:   premul(b), // KindChecker's second color
		Kind:   sl.V4(shaders.KindChecker, 0, 0, 0),
		Params: sl.V4(o.Cell*s, o.Rect.X*s, o.Rect.Y*s, 0),
	}, r.outBounds(o.Rect, 1))
}

// image draws an Image as one KindImage instance sampling the image's own
// texture (see Renderer); one the texture cache does not take this frame
// (too large, over the upload budget) is a patch. Images paint.CPU draws
// nothing for (nil, empty, non-finite or empty rect) draw nothing.
func (r *Renderer) image(o paint.Image) {
	if !paint.ImageDrawable(o) {
		return
	}
	s := r.scale
	dr := sl.V4(o.Rect.X*s, o.Rect.Y*s, o.Rect.W*s, o.Rect.H*s)
	if !finiteV4(dr) {
		return
	}
	b := r.outBounds(o.Rect, 1)
	w, h := paint.ImageReduction(o.Img.Bounds(), dr.Z, dr.W)
	if c := r.clip(); c.scissor.Empty() || !b.Overlaps(c.scissor) {
		// Not drawn: do not upload it. In a partial frame an image outside
		// the dirty region is still on screen (its pixels are retained), so
		// it counts as used: the cache must not evict it as stale or idle.
		// Inside a layer the clip's full scissor is the partial one (see
		// beginGroup), so there any culled image counts.
		if r.partial && (r.target != 0 || b.Overlaps(c.full)) {
			r.images.touch(o.Img, w, h)
		}
		return
	}
	t, miss := r.images.get(o.Img, w, h, r.imageBudget)
	if miss != imageOK {
		if miss == imageBudget && !r.budgetHit {
			r.budgetHit = true
			r.logf("gpu: image uploads exceed %d bytes in one frame; images over the budget are drawn as patches until a later frame uploads them (logged once)", r.imageBudget)
		}
		r.patch([]paint.Op{o}, b, 1)
		return
	}
	size := t.pix.Rect.Size()
	r.emitImage(shaders.Instance{
		Rect:   dr,
		Radii:  sl.V4(o.Radii.TL*s, o.Radii.TR*s, o.Radii.BR*s, o.Radii.BL*s),
		Kind:   sl.V4(shaders.KindImage, 0, 0, 0),
		Params: sl.V4(float32(size.X), float32(size.Y), 0, 0),
	}, b, t.id)
}

// touchImages marks the textures of the images in ops (a group a partial
// frame skips because it misses the dirty region) that reach full (the
// group's full-frame scissor) as used this frame, as image does for a
// single culled image. m, xf is the transform in effect (as clipState);
// nested native transforms are followed so the texture key (the reduction
// size, from the mapped rect) matches the one image uses, and subtrees
// under other transforms are skipped (patches: they use no image texture).
func (r *Renderer) touchImages(ops []paint.Op, m geom.Affine, xf bool, full image.Rectangle) {
	s := r.scale
	for i := 0; i < len(ops); i++ {
		switch o := ops[i].(type) {
		case paint.PushTransform:
			end := matchingPop(ops, i)
			body := ops[i+1 : end]
			nm := o.M
			if xf {
				nm = m.Mul(o.M)
			}
			switch {
			case nm.IsIdentity():
				r.touchImages(body, geom.Affine{}, false, full)
			case nativeXform(nm):
				r.touchImages(body, nm, true, full)
			}
			i = end
		case paint.Image:
			op := paint.Op(o)
			if xf {
				op = mapOp(op, m)
			}
			mo, ok := op.(paint.Image)
			if !ok || !paint.ImageDrawable(mo) || !r.outBounds(mo.Rect, 1).Overlaps(full) {
				continue
			}
			w, h := paint.ImageReduction(mo.Img.Bounds(), mo.Rect.W*s, mo.Rect.H*s)
			r.images.touch(mo.Img, w, h)
		}
	}
}

func finiteV4(v sl.Vec4) bool {
	for _, f := range [4]float32{v.X, v.Y, v.Z, v.W} {
		if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
			return false
		}
	}
	return true
}

// glyphReach bounds, in ems of a glyph's device size, how far its mask
// may extend from its pen position in any direction; the text culling
// before rasterization (see Renderer) relies on it. Real glyphs stay well
// within it (the widest common ones, such as U+FDFD, are about 3 ems
// wide); a glyph inked farther out could be culled while partly visible.
const glyphReach = 4

// glyphMayTouch reports whether the mask of a glyph of device size size
// and skew, at pen position (ix, iy), may overlap sc (by glyphReach).
func glyphMayTouch(ix, iy int, size, skew float32, sc image.Rectangle) bool {
	m := float64(size)*glyphReach*(1+math.Abs(float64(skew))) + 2
	if !(m < 1<<20) { // NaN or huge: let GlyphMask decide
		return true
	}
	d := int(math.Ceil(m))
	return image.Rect(ix-d, iy-d, ix+d, iy+d).Overlaps(sc)
}

// glyphMayTouchDirty is glyphMayTouch in a partial frame: the reach box
// must also overlap a dirty rectangle when the frame draws into its target.
func (r *Renderer) glyphMayTouchDirty(ix, iy int, size, skew float32, sc image.Rectangle) bool {
	if !glyphMayTouch(ix, iy, size, skew, sc) {
		return false
	}
	if r.target != 0 || len(r.dirty) < 2 {
		return true
	}
	m := float64(size)*glyphReach*(1+math.Abs(float64(skew))) + 2
	if !(m < 1<<20) {
		return true
	}
	d := int(math.Ceil(m))
	return overlapsAny(image.Rect(ix-d, iy-d, ix+d, iy+d), r.dirty)
}

// glyphPlace returns the device pen position, subpixel bucket and device
// size of glyph g of o under the native transform m (xf set) or none, as
// paint.CPU's text places it: without a transform paint.GlyphPlacement;
// under a uniform scale+translate the same rounding with the device scale
// k = scale·m.A and the offset scale·(m.E, m.F) (expression for
// expression, so both round alike).
func (r *Renderer) glyphPlace(o paint.Text, g paint.Glyph, m geom.Affine, xf bool) (ix, iy int, sub uint8, size float32) {
	if !xf {
		ix, iy, sub = paint.GlyphPlacement(o.Origin, g, r.scale)
		return ix, iy, sub, g.Size * r.scale
	}
	k, ex, ey := r.scale*m.A, r.scale*m.E, r.scale*m.F
	px := float64((o.Origin.X+g.X)*k + ex)
	py := math.Round(float64((o.Origin.Y+g.Y)*k + ey))
	fx := math.Floor(px)
	return int(fx), int(py), uint8(math.Floor((px - fx) * 4)), g.Size * k
}

func (r *Renderer) text(o paint.Text) {
	if o.Color.A == 0 {
		return
	}
	cs := r.clip()
	sc := cs.scissor
	if sc.Empty() {
		return
	}
	c := premul(o.Color)
	for i, g := range o.Glyphs {
		if g.Src == nil {
			continue
		}
		ix, iy, sub, size := r.glyphPlace(o, g, cs.m, cs.xf)
		if r.partial && !r.glyphMayTouchDirty(ix, iy, size, g.Skew, sc) {
			// A partial frame's scissor is small: culling by reach first
			// saves most atlas lookups.
			continue
		}
		key := atlas.Key{Src: g.Src, ID: g.ID, Size: size, Sub: sub, Skew: g.Skew}
		e, ok := r.atlas.Lookup(key)
		if !ok {
			if !glyphMayTouch(ix, iy, size, g.Skew, sc) {
				continue // culled before rasterizing
			}
			mask, off := r.glyphMask(g, size, sub)
			if mask == nil || mask.Rect.Empty() {
				r.atlas.Insert(key, nil, off) // remember the empty glyph
				continue
			}
			// Inserted even when it turns out to miss the scissor (it lies
			// within glyphReach of it), so later frames cull it by its
			// entry instead of asking paint.CPU again.
			var err error
			if e, err = r.atlas.Insert(key, mask, off); err != nil {
				// No room in the atlas this frame (or a huge glyph): draw it
				// as a patch, which paint.CPU rasterizes identically (patch
				// draws nothing outside the scissor).
				at := image.Pt(ix+off.X, iy+off.Y)
				dr := image.Rectangle{Min: at, Max: at.Add(mask.Rect.Size())}
				r.patch(r.inXform([]paint.Op{paint.Text{Origin: o.Origin, Glyphs: o.Glyphs[i : i+1], Color: o.Color}}), dr, 1)
				continue
			}
		}
		if e.Rect.Empty() {
			continue // empty glyph
		}
		at := image.Pt(ix+e.Off.X, iy+e.Off.Y)
		dr := image.Rectangle{Min: at, Max: at.Add(e.Rect.Size())}
		if !dr.Overlaps(sc) {
			continue
		}
		r.emit(shaders.Instance{
			Rect:  rectV4(dr),
			Color: c,
			UV:    rectV4(e.Rect), // texels; normalized in Render
			Kind:  sl.V4(shaders.KindMask, 0, 0, 0),
		}, dr)
	}
}

// pathState is how pathMask's path is drawn.
type pathState uint8

const (
	pathNone  pathState = iota // nothing to draw (outside the scissor, or no area)
	pathOK                     // from the returned atlas entry
	pathPatch                  // no atlas entry: as a patch
)

// pathMask returns the atlas entry holding p's coverage mask at this
// frame's scale under the current transform, rasterizing it with
// paint.CPU.PathMask on a miss, and the mask's device position (Entry.Off
// plus the transform's whole-pixel translation). Paths whose device
// bounds miss the scissor are not rasterized (pathNone); a mask larger
// than an atlas page, or one the atlas has no room for this frame, is
// pathPatch.
//
// Without a transform the mask is keyed by (p, scale) and rasterized as
// paint.CPU fills p. Under a native transform m (see nativeXform; icons
// are drawn as a cached Path inside a Translate) the key holds only the
// device transform's linear part (scale·m.A) and the subpixel phase of its
// translation scale·(m.E, m.F), rounded to the nearest 1/4 pixel on each
// axis like glyph subpixel buckets: the mask is rasterized with that phase
// as its only translation and placed at the translation's whole pixels.
// So a path drawn at any number of whole-pixel positions is rasterized
// once, and at most 16 times over all fractional positions; a position
// off the quarter-pixel grid is drawn up to 1/8 pixel from where paint.CPU
// (which rasterizes at the exact position) draws it.
func (r *Renderer) pathMask(p *paint.Path) (atlas.Entry, image.Point, pathState) {
	s := r.scale
	c := r.clip()
	key := atlas.Key{Kind: atlas.KindPath, Path: p, M: geom.Scale(s, s), Size: s}
	var shift image.Point // whole device pixels of the translation
	if c.xf {
		k := s * c.m.A
		tx, ty := float64(s*c.m.E), float64(s*c.m.F)
		ix, iy := math.Floor(tx), math.Floor(ty)
		qx, qy := math.Round((tx-ix)*4), math.Round((ty-iy)*4) // phase in quarter pixels, 0..4
		if qx == 4 {
			ix, qx = ix+1, 0
		}
		if qy == 4 {
			iy, qy = iy+1, 0
		}
		shift = image.Pt(clampInt(ix), clampInt(iy))
		key.M = geom.Affine{A: k, D: k, E: float32(qx) / 4, F: float32(qy) / 4}
	}
	e, ok := r.atlas.Lookup(key)
	if !ok {
		// Rasterized at the scale and phase only (logical units).
		lin := geom.Affine{A: c.m.A, D: c.m.A, E: key.M.E / s, F: key.M.F / s}
		pb := p.Bounds()
		if c.xf {
			pb = lin.Bounds(pb)
		}
		b := r.outBounds(pb, 1) // paint.PathDeviceBounds (up to float rounding of its transparent border)
		if !b.Add(shift).Overlaps(c.scissor) {
			return atlas.Entry{}, image.Point{}, pathNone
		}
		if ps := r.atlas.PageSize(); b.Dx() > ps || b.Dy() > ps {
			return atlas.Entry{}, image.Point{}, pathPatch // do not rasterize it whole
		}
		q := p
		if c.xf {
			q = p.Transform(lin)
		}
		m, at := r.pathRaster(q)
		var err error
		if e, err = r.atlas.Insert(key, m, at); err != nil {
			return atlas.Entry{}, image.Point{}, pathPatch
		}
	}
	if e.Rect.Empty() {
		return atlas.Entry{}, image.Point{}, pathNone
	}
	at := e.Off.Add(shift)
	if !(image.Rectangle{Min: at, Max: at.Add(e.Rect.Size())}).Overlaps(c.scissor) {
		return atlas.Entry{}, image.Point{}, pathNone
	}
	return e, at, pathOK
}

// fillPath draws a FillPath as one KindMask instance over the path's
// cached coverage mask (see Renderer), or as a patch.
func (r *Renderer) fillPath(o paint.FillPath) {
	if o.Path == nil || o.Color.A == 0 {
		return
	}
	e, at, st := r.pathMask(o.Path)
	switch st {
	case pathPatch:
		r.patch(r.inXform([]paint.Op{o}), r.opBounds(o), 1)
	case pathOK:
		dr := image.Rectangle{Min: at, Max: at.Add(e.Rect.Size())}
		r.emit(shaders.Instance{
			Rect:  rectV4(dr),
			Color: premul(o.Color),
			UV:    rectV4(e.Rect), // texels; normalized in Render
			Kind:  sl.V4(shaders.KindMask, 0, 0, 0),
		}, dr)
	}
}

// pushOpacity opens the PushOpacity group at ops[i] (see Renderer) and
// returns the index the dispatch loop continues after: i, or the group's
// Pop when the group draws nothing.
func (r *Renderer) pushOpacity(o paint.PushOpacity, ops []paint.Op, i int) int {
	end := matchingPop(ops, i)
	var ia uint32    // paint.CPU's 8-bit quantization of the group alpha
	if o.Alpha > 0 { // also false for NaN
		ia = uint32(min(1, o.Alpha)*255 + 0.5)
	}
	if ia == 0 {
		return end // skip the group and its Pop
	}
	body := ops[i+1 : end]
	c := r.clip()
	g := group{end: end, depth: len(r.clips), alpha: float32(ia) / 255}
	switch {
	case ia == 255 && !roundedInRounded(body, c):
		// (With a rounded clip inside an enclosing one, the body is drawn
		// into a layer like any group, so both clips apply: the inner one
		// in the layer, the outer one at the composite.)
		g.kind = groupPlain
		r.clips = append(r.clips, c)
	case ia < 255 && r.alpha == 1 && r.singleOp(body, c):
		g.kind = groupFast
		r.alpha = g.alpha
		r.clips = append(r.clips, c)
	default:
		ub := r.bounds(body)
		b := ub.Intersect(c.scissor)
		if b.Empty() {
			if r.partial && (r.target != 0 || ub.Overlaps(c.full)) {
				// Outside the dirty region, still on screen. (Inside a
				// layer c.full is the partial scissor: use the frame.)
				full := c.full
				if r.target != 0 {
					full = r.full
				}
				r.touchImages(body, c.m, c.xf, full)
			}
			return end
		}
		r.frame.Layers = append(r.frame.Layers, LayerDesc{Size: image.Pt(alignUp(b.Dx()), alignUp(b.Dy())), Bounds: b})
		r.layerDrawn = append(r.layerDrawn, false)
		g.kind, g.layer, g.prev = groupLayer, len(r.frame.Layers), r.target
		// paint.CPU draws the body unclipped by the enclosing rounded
		// clips (they apply when the layer is composited).
		fb := b
		if r.partial && r.verify {
			fb = r.bounds(body).Intersect(c.full) // the layer a full frame has (see patch)
		}
		r.clips = append(r.clips, clipState{scissor: b, full: fb, m: c.m, xf: c.xf})
		r.target = g.layer
	}
	r.groups = append(r.groups, g)
	return i
}

func alignUp(n int) int { return (n + layerAlign - 1) / layerAlign * layerAlign }

// roundedInRounded reports whether body pushes a rounded clip while the
// enclosing clip c is rounded: drawn op by op, the instances would carry
// only the innermost one (paint.CPU draws a group's body without the
// enclosing rounded clips and composites it through them).
func roundedInRounded(body []paint.Op, c clipState) bool {
	if !(c.rect.Z > 0 && c.rect.W > 0) {
		return false
	}
	for _, op := range body {
		if o, ok := op.(paint.PushClip); ok && !o.Radii.IsZero() {
			return true
		}
	}
	return false
}

// singleOp reports whether a group body is drawn exactly enough op by op
// with the group's alpha multiplied into the color: it holds exactly one
// drawing op (a Text counts its glyphs), one whose instances multiply
// Color, besides clips, native transforms (translate, uniform positive
// scale: an icon) and hints, and no rounded clip when the enclosing clip c
// is rounded (see roundedInRounded).
func (r *Renderer) singleOp(body []paint.Op, c clipState) bool {
	if roundedInRounded(body, c) {
		return false
	}
	n := 0
	for _, op := range body {
		switch o := op.(type) {
		case paint.PushClip:
		case paint.PushTransform:
			// Native under any native outer transform (the product of
			// two such transforms is one); others are patches.
			if !o.M.IsIdentity() && !nativeXform(o.M) {
				return false
			}
		case paint.Pop:
		case paint.FillRect, paint.StrokeRect, paint.ShadowRect, paint.FillPath, paint.Checker:
			n++
		case paint.Text:
			for _, g := range o.Glyphs {
				if g.Src != nil {
					n++
				}
			}
		default:
			if opHandling[opName(op)] != opHint {
				return false
			}
		}
		if n > 1 {
			return false
		}
	}
	return n == 1
}

// endGroup closes the innermost open group: it restores the clip state
// and target from before the group and composites a layer group's layer
// (when anything was drawn into it) into that target.
func (r *Renderer) endGroup() {
	g := r.groups[len(r.groups)-1]
	r.groups = r.groups[:len(r.groups)-1]
	r.clips = r.clips[:g.depth]
	switch g.kind {
	case groupFast:
		r.alpha = 1
	case groupLayer:
		r.target = g.prev
		if !r.layerDrawn[g.layer] {
			return // nothing drawn: no texture is ever made for it
		}
		ld := r.frame.Layers[g.layer-1]
		b := ld.Bounds
		tw, th := float32(ld.Size.X), float32(ld.Size.Y)
		a := g.alpha
		r.emitTex(shaders.Instance{
			Rect:  rectV4(b),
			Color: sl.V4(a, a, a, a),
			UV:    sl.V4(0, 0, float32(b.Dx())/tw, float32(b.Dy())/th), // normalized already
			Kind:  sl.V4(shaders.KindPatch, 0.5/tw, 0.5/th, 1),
		}, b, 0, g.layer)
	}
}

// hoistLayers moves the batches drawing into layers ahead of the batches
// drawing into the frame's target, keeping the order within each, so a
// device draws the target in one render pass instead of one per group.
// That is safe because layer batches never sample the frame's target, and
// a layer is composited (by a later batch) only after all batches drawing
// into it. Batches carrying Uploads (patch page flushes) are barriers:
// batches are reordered only between them, and a barrier's uploads move to
// the first batch of the reordered run, so every batch samples the patch
// page it was emitted against. The price: every layer of a run is still
// to be composited when the target's pass starts, so a device holds all of
// them at once (it can reuse a texture only after a layer's composite).
func (r *Renderer) hoistLayers() {
	f := &r.frame
	if len(f.Layers) == 0 {
		return
	}
	bs := f.Batches
	out := r.scratch[:0]
	for s := 0; s < len(bs); {
		e := s + 1
		for e < len(bs) && len(bs[e].Uploads) == 0 {
			e++
		}
		start := len(out)
		for _, b := range bs[s:e] {
			if b.Target != 0 {
				out = append(out, b)
			}
		}
		nl := len(out) - start
		for _, b := range bs[s:e] {
			if b.Target == 0 {
				out = append(out, b)
			}
		}
		if bs[s].Target == 0 && nl > 0 {
			// bs[s] is now out[start+nl]; its uploads must still come first.
			out[start].Uploads, out[start+nl].Uploads = out[start+nl].Uploads, out[start].Uploads
		}
		s = e
	}
	copy(bs, out)
	clear(out) // drop the copies' references
	r.scratch = out[:0]
}

// patch rasterizes ops with paint.CPU into the part of bounds inside the
// current scissor and draws the result with opacity alpha: one KindPatch
// instance per patch-page-sized tile of its inked area.
func (r *Renderer) patch(ops []paint.Op, bounds image.Rectangle, alpha float32) {
	c := r.clip()
	region := bounds.Intersect(c.scissor)
	if region.Empty() || len(ops) == 0 {
		return
	}
	if r.partial && r.target == 0 && len(r.dirty) > 1 && !overlapsAny(region, r.dirty) {
		return // between the dirty rectangles
	}
	if r.onPatch != nil {
		r.onPatch(ops, region)
	}
	raster := region
	if r.partial && r.verify {
		// In verify mode, rasterize what a full frame rasterizes:
		// paint.CPU's antialiasing of transformed and path content can
		// differ by a level with the region it starts from, and verify
		// compares the partial frame with the full one exactly. Without
		// verify mode (release builds) the patch is rasterized only over
		// its part of the dirty region, so a partial frame's patch pixels
		// may differ by that much from a full frame's; every other
		// instance is the same either way.
		raster = bounds.Intersect(c.full)
	}
	if r.onRaster != nil {
		r.onRaster(raster)
	}
	img := r.cpu.RenderRegion(ops, raster, r.scale)
	ink := inkBounds(img).Intersect(region)
	if ink.Empty() {
		return
	}
	n := r.patchSize
	for y := ink.Min.Y; y < ink.Max.Y; y += n {
		for x := ink.Min.X; x < ink.Max.X; x += n {
			tile := image.Rect(x, y, min(x+n, ink.Max.X), min(y+n, ink.Max.Y))
			at := r.placePatch(tile.Dx(), tile.Dy())
			ar := image.Rectangle{Min: at, Max: at.Add(tile.Size())}
			up := Upload{Tex: TexColor, Rect: ar, Pix: img.Pix[img.PixOffset(tile.Min.X, tile.Min.Y):], Stride: img.Stride}
			if r.flushed {
				r.pending = append(r.pending, up) // goes to the batch emit picks
			} else {
				r.frame.Uploads = append(r.frame.Uploads, up)
			}
			r.emit(shaders.Instance{
				Rect:  rectV4(tile),
				Color: sl.V4(alpha, alpha, alpha, alpha),
				UV:    rectV4(ar), // texels; normalized in Render
				Kind:  sl.V4(shaders.KindPatch, 0, 0, 0),
			}, tile)
		}
	}
}

// placePatch returns the patch-page position for a w×h tile (at most the
// page size), flushing the page when it is full: earlier instances keep
// what they sampled because later uploads go into a new batch.
func (r *Renderer) placePatch(w, h int) image.Point {
	if at, ok := r.patches.alloc(w, h); ok {
		return at
	}
	r.patches.reset(r.patchSize)
	r.flushed, r.newBatch = true, true
	at, _ := r.patches.alloc(w, h) // fits: the page is empty and the tile no larger
	return at
}

// inkBounds returns the bounding box of img's non-transparent pixels.
func inkBounds(img *image.RGBA) image.Rectangle {
	b := img.Rect
	x0, y0, x1, y1 := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
	for y := b.Min.Y; y < b.Max.Y; y++ {
		row := img.Pix[img.PixOffset(b.Min.X, y):][:4*b.Dx()]
		for i := 3; i < len(row); i += 4 {
			if row[i] != 0 {
				x := b.Min.X + i/4
				x0, x1 = min(x0, x), max(x1, x+1)
				y0, y1 = min(y0, y), max(y1, y+1)
			}
		}
	}
	if x0 >= x1 {
		return image.Rectangle{}
	}
	return image.Rect(x0, y0, x1, y1)
}

// bounds returns a conservative device bounding box of everything ops draw
// under the current transform, with PushTransform groups' boxes mapped
// through their transforms.
func (r *Renderer) bounds(ops []paint.Op) image.Rectangle {
	c := r.clip()
	return r.boundsIn(ops, c.m, c.xf)
}

// boundsIn is bounds under the native transform m (xf set) or none.
func (r *Renderer) boundsIn(ops []paint.Op, m geom.Affine, xf bool) image.Rectangle {
	var b image.Rectangle
	for i := 0; i < len(ops); i++ {
		if o, ok := ops[i].(paint.PushTransform); ok {
			end := matchingPop(ops, i)
			body := ops[i+1 : end]
			nm := o.M
			if xf {
				nm = m.Mul(o.M)
			}
			switch {
			case nm.IsIdentity():
				b = b.Union(r.boundsIn(body, geom.Affine{}, false))
			case nativeXform(nm):
				b = b.Union(r.boundsIn(body, nm, true))
			default:
				b = b.Union(r.transformBounds(nm, r.boundsIn(body, geom.Affine{}, false)))
			}
			i = end
			continue
		}
		b = b.Union(r.opBoundsIn(ops[i], m, xf))
	}
	return b
}

// transformBounds maps the device box b of untransformed content through
// the logical transform m.
func (r *Renderer) transformBounds(m geom.Affine, b image.Rectangle) image.Rectangle {
	if b.Empty() {
		return b
	}
	if b == everywhere {
		return b
	}
	s := r.scale
	g := geom.Rect{X: float32(b.Min.X) / s, Y: float32(b.Min.Y) / s, W: float32(b.Dx()) / s, H: float32(b.Dy()) / s}
	return r.outBounds(m.Bounds(g), 2)
}

// everywhere is the bounding box of ops whose extent the Renderer does not
// know (ops added to paint later): patches of them cover the whole scissor.
var everywhere = image.Rect(-1<<24, -1<<24, 1<<24, 1<<24)

// opBounds returns a conservative device bounding box of op under the
// current transform (empty for clip/opacity markers).
func (r *Renderer) opBounds(op paint.Op) image.Rectangle {
	c := r.clip()
	return r.opBoundsIn(op, c.m, c.xf)
}

// opBoundsIn is opBounds under the native transform m (xf set) or none.
func (r *Renderer) opBoundsIn(op paint.Op, m geom.Affine, xf bool) image.Rectangle {
	rect := func(g geom.Rect) geom.Rect { // the rect of an op mapOp leaves alone
		if xf {
			return m.Bounds(g)
		}
		return g
	}
	if xf {
		op = mapOp(op, m)
	}
	switch o := op.(type) {
	case paint.FillRect:
		return r.outBounds(o.Rect, 1)
	case paint.StrokeRect:
		return r.outBounds(o.Rect, 1)
	case paint.LinearGradient:
		return r.outBounds(rect(o.Rect), 1)
	case paint.Gradient:
		if o.Path != nil {
			return r.outBounds(rect(o.Path.Bounds()), 2)
		}
		return r.outBounds(rect(o.Rect), 1)
	case paint.Checker:
		return r.outBounds(rect(o.Rect), 1)
	case paint.Image:
		return r.outBounds(o.Rect, 1)
	case paint.FillPath:
		if o.Path == nil {
			return image.Rectangle{}
		}
		return r.outBounds(rect(o.Path.Bounds()), 2)
	case paint.ShadowRect:
		g, blurred := paint.ShadowPlacement(o, r.scale)
		if !blurred {
			return r.outBounds(g.Shape, 1)
		}
		if g.Rect.Empty() {
			return image.Rectangle{}
		}
		return g.Rect.Inset(-g.Margin - 1)
	case paint.Text:
		var b image.Rectangle
		for _, g := range o.Glyphs {
			if g.Src == nil {
				continue
			}
			ix, iy, sub, size := r.glyphPlace(o, g, m, xf)
			if e, ok := r.atlas.Lookup(atlas.Key{Src: g.Src, ID: g.ID, Size: size, Sub: sub, Skew: g.Skew}); ok {
				// Placed as text() places it, without asking paint.CPU.
				if !e.Rect.Empty() {
					at := image.Pt(ix+e.Off.X, iy+e.Off.Y)
					b = b.Union(image.Rectangle{Min: at, Max: at.Add(e.Rect.Size())})
				}
				continue
			}
			mask, off := r.cpu.GlyphMask(g, size, sub)
			if mask == nil {
				continue
			}
			at := image.Pt(ix+off.X, iy+off.Y)
			b = b.Union(image.Rectangle{Min: at, Max: at.Add(mask.Rect.Size())})
		}
		return b
	case paint.PushClip, paint.PushOpacity, paint.PushTransform, paint.Pop:
		return image.Rectangle{}
	}
	if opHandling[opName(op)] == opHint {
		return image.Rectangle{}
	}
	return everywhere
}

// outBounds returns the device pixels touched by g, grown by pad pixels.
func (r *Renderer) outBounds(g geom.Rect, pad int) image.Rectangle {
	s := float64(r.scale)
	x0, y0 := math.Floor(float64(g.X)*s), math.Floor(float64(g.Y)*s)
	x1, y1 := math.Ceil(float64(g.Right())*s), math.Ceil(float64(g.Bottom())*s)
	if !(x1 > x0 && y1 > y0) {
		return image.Rectangle{}
	}
	return image.Rect(clampInt(x0)-pad, clampInt(y0)-pad, clampInt(x1)+pad, clampInt(y1)+pad)
}

// devRect rounds a logical rectangle to device pixels as paint.CPU rounds
// clip rectangles.
func (r *Renderer) devRect(g geom.Rect) image.Rectangle {
	s := float64(r.scale)
	return image.Rect(clampInt(math.Round(float64(g.X)*s)), clampInt(math.Round(float64(g.Y)*s)),
		clampInt(math.Round(float64(g.Right())*s)), clampInt(math.Round(float64(g.Bottom())*s)))
}

// clampInt converts f to int, keeping huge, infinite and NaN values well
// defined (they are far outside any target).
func clampInt(f float64) int {
	const lim = 1 << 24
	if !(f > -lim) {
		return -lim
	}
	if f > lim {
		return lim
	}
	return int(f)
}

func rectV4(b image.Rectangle) sl.Vec4 {
	return sl.V4(float32(b.Min.X), float32(b.Min.Y), float32(b.Dx()), float32(b.Dy()))
}

// premul converts c to premultiplied 0..1 RGBA.
func premul(c color.NRGBA) sl.Vec4 {
	a := float32(c.A) / 255
	return sl.V4(float32(c.R)/255*a, float32(c.G)/255*a, float32(c.B)/255*a, a)
}

// patchPage shelf-packs patches into one fixed square page: entries go left
// to right on shelves, a new shelf opens below the last, 1 texel apart.
type patchPage struct {
	size    int
	shelves []shelf
	bottom  int
}

type shelf struct{ y, h, x int }

func (p *patchPage) reset(size int) {
	p.size = size
	p.shelves = p.shelves[:0]
	p.bottom = 0
}

// alloc places a w×h entry, reporting false when the page has no room.
func (p *patchPage) alloc(w, h int) (image.Point, bool) {
	const gap = 1
	if w > p.size || h > p.size {
		return image.Point{}, false
	}
	for i := range p.shelves {
		s := &p.shelves[i]
		if h <= s.h && s.x+w <= p.size {
			at := image.Pt(s.x, s.y)
			s.x += w + gap
			return at, true
		}
	}
	if p.bottom+h > p.size {
		return image.Point{}, false
	}
	at := image.Pt(0, p.bottom)
	p.shelves = append(p.shelves, shelf{y: p.bottom, h: h, x: w + gap})
	p.bottom += h + gap
	return at, true
}

var _ paint.Renderer = (*Renderer)(nil)
