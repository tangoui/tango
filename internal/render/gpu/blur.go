package gpu

import (
	"image"

	"github.com/tangoui/tango/internal/render/gpu/shaders"
	"github.com/tangoui/tango/internal/render/shader/sl"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
)

// Backdrop blur (paint.BackdropBlur).
//
// The Renderer runs the plan paint.CPU runs (paint.PlanBackdropBlur, the
// algorithm at the top of ui/paint/blur.go) with layers:
//
//   - a grab batch (Batch.Grab) copies the plan's Src, as far as the
//     current target holds it (the frame; or the current layer's Bounds,
//     beyond which a group holds nothing: the passes read transparent
//     there), into layer G;
//   - when the plan downsamples, a BlurDown pass into layer D (the reduced
//     size, Bounds a stand-in rectangle at Src.Min, Small large);
//   - the BlurH pass into layer H and the BlurV pass into layer V (each
//     reading the one before);
//   - the BlurComposite instance, drawn into the current target like any
//     instance (clip, scissor, partial-frame dirty rectangles), sampling V.
//
// The grab must see the target exactly as a full frame has it at that
// point: planPartial (damage.go) redraws a blur's whole Src whenever a
// dirty rectangle or a copy touches it, so in a partial frame a blur
// either has all its source redrawn or does not reach a dirty rectangle
// at all (and is skipped). hoistLayers keeps the blur's batches in place.
//
// The pass layers are this frame's like any layer; a device may back them
// with larger pooled textures (the passes' UVs are normalized like a
// composite's and rescaled with them).

// backdropBlur draws o (mapped by mapOp under a native transform).
func (r *Renderer) backdropBlur(o paint.BackdropBlur) {
	s := r.scale
	c := r.clip()
	dr := geom.Rect{X: o.Rect.X * s, Y: o.Rect.Y * s, W: o.Rect.W * s, H: o.Rect.H * s}
	p, ok := paint.PlanBackdropBlur(dr, paint.BlurSigma(o.Radius, s), c.full, r.full)
	if !ok || c.scissor.Empty() || !p.Out.Overlaps(c.scissor) {
		return
	}
	if r.partial && r.target == 0 && !overlapsAny(p.Out, r.dirty) {
		return // nothing it changes is redrawn
	}
	held := r.full
	if r.target != 0 {
		held = r.frame.Layers[r.target-1].Bounds
	}
	g := p.Src.Intersect(held)
	if g.Empty() {
		return // the group holds nothing there: the blurred copy is transparent
	}
	f := &r.frame
	first := len(f.Batches)
	src := r.target
	gl := r.addLayer(g)
	r.addBatch(src, 0, gl, nil)
	small := image.Rectangle{Min: p.Src.Min, Max: p.Src.Min.Add(p.Small)}
	// The first pass reads the grab through the Src space; later ones the
	// previous reduced layer whole.
	srcSpace := sl.V4(float32(g.Min.X-p.Src.Min.X), float32(g.Min.Y-p.Src.Min.Y), float32(p.Src.Dx()), float32(p.Src.Dy()))
	smallSpace := sl.V4(0, 0, float32(p.Small.X), float32(p.Small.Y))
	in, space := gl, srcSpace
	if p.Down > 1 {
		dl := r.addLayer(small)
		r.addPass(dl, in, shaders.BlurDown, sl.V4(float32(p.Down), 0, float32(small.Min.X), float32(small.Min.Y)), space)
		in, space = dl, smallSpace
	}
	hl := r.addLayer(small)
	r.addPass(hl, in, shaders.BlurH, sl.V4(p.Sigma, float32(p.Taps), float32(small.Min.X), float32(small.Min.Y)), space)
	vl := r.addLayer(small)
	r.addPass(vl, hl, shaders.BlurV, sl.V4(p.Sigma, float32(p.Taps), float32(small.Min.X), float32(small.Min.Y)), smallSpace)

	ld := f.Layers[vl-1]
	tw, th := float32(ld.Size.X), float32(ld.Size.Y)
	r.newBatch = true
	n := len(f.Batches)
	r.emitTex(shaders.Instance{
		Rect:   sl.V4(dr.X, dr.Y, dr.W, dr.H),
		Radii:  sl.V4(o.Radii.TL*s, o.Radii.TR*s, o.Radii.BR*s, o.Radii.BL*s),
		UV:     sl.V4(0, 0, float32(p.Small.X)/tw, float32(p.Small.Y)/th), // normalized already
		Kind:   sl.V4(shaders.KindPatch, 0.5/tw, 0.5/th, shaders.BlurComposite),
		Params: sl.V4(float32(p.Down), 0, float32(p.Src.Min.X), float32(p.Src.Min.Y)),
		Dash:   smallSpace,
	}, p.Out, 0, vl)
	last := len(f.Batches) - 1
	if last < n {
		last = n - 1 // the composite was culled: the span ends with the passes
	}
	r.blurSpans = append(r.blurSpans, [2]int{first, last})
	r.newBatch = true // what follows does not join the composite's batch
}

// addLayer appends a layer of bounds b to the frame and returns its number.
func (r *Renderer) addLayer(b image.Rectangle) int {
	r.frame.Layers = append(r.frame.Layers, LayerDesc{Size: image.Pt(alignUp(b.Dx()), alignUp(b.Dy())), Bounds: b})
	r.layerDrawn = append(r.layerDrawn, false)
	return len(r.frame.Layers)
}

// addPass appends a batch drawing the blur pass mode into layer target
// from layer src over target's whole bounds: params as BlurPass reads
// them, space the source space (Dash).
func (r *Renderer) addPass(target, src int, mode float32, params, space sl.Vec4) {
	ld := r.frame.Layers[src-1]
	tw, th := float32(ld.Size.X), float32(ld.Size.Y)
	in := shaders.Instance{
		Rect:   rectV4(r.frame.Layers[target-1].Bounds),
		UV:     sl.V4(0, 0, float32(ld.Bounds.Dx())/tw, float32(ld.Bounds.Dy())/th), // normalized already
		Kind:   sl.V4(shaders.KindPatch, 0.5/tw, 0.5/th, mode),
		Params: params,
		Dash:   space,
	}
	r.addBatch(target, src, 0, &in)
}

// addBatch appends a batch of its own (no scissor, no image) drawing in
// into target and sampling layer, or, with grab > 0, the grab batch
// copying into layer grab from target.
func (r *Renderer) addBatch(target, layer, grab int, in *shaders.Instance) {
	f := &r.frame
	n := len(f.Batches)
	if n < cap(f.Batches) {
		f.Batches = f.Batches[:n+1] // Uploads were emptied in frame1
		b := &f.Batches[n]
		b.Scissor, b.Image, b.Layer, b.Target, b.Grab = image.Rectangle{}, 0, layer, target, grab
		b.Instances = b.Instances[:0]
	} else {
		f.Batches = append(f.Batches, Batch{Target: target, Layer: layer, Grab: grab})
	}
	b := &f.Batches[n]
	if in != nil {
		b.Instances = append(b.Instances, *in)
		r.layerDrawn[target] = true
	}
	if grab > 0 {
		r.layerDrawn[grab] = true
	}
	r.newBatch = true
}
