// Package paint is the display-list layer of the Tango self-drawn UI.
//
// Components record drawing commands (rounded rectangles, borders, Ant-style
// layered box shadows, paths, gradients, checkerboards, images, glyph runs,
// clips, opacity groups, affine transform groups and backdrop blurs) into
// a List in logical
// pixels. A Renderer turns the list into pixels; this
// package ships a pure-Go CPU rasterizer (CPU, built on x/image/vector) that
// writes an *image.RGBA at any device scale. The List is deliberately a flat,
// backend-neutral value so a GPU renderer can consume the same commands later
// without changes to components.
//
// API tiers: only the symbols listed in api/stable/ui_paint.txt are covered
// by the compatibility promise of docs/release.md ("API 稳定性"); every
// other exported symbol is Experimental and may change in any 0.x
// release. The API reference on the documentation site marks the
// experimental symbols.
package paint

import (
	"image"
	"image/color"

	"github.com/tangoui/tango/ui/geom"
)

// Shadow is one CSS box-shadow layer (outer only).
type Shadow struct {
	X, Y, Blur, Spread float32
	Color              color.NRGBA
}

// GlyphSource provides glyph outlines. The text package implements it; paint
// never imports text so renderers stay independent of shaping.
type GlyphSource interface {
	// GlyphPath returns the outline of glyph id at the given pixel size with
	// its origin on the baseline and y growing downwards. It may return nil
	// for empty glyphs (spaces). Implementations should cache.
	GlyphPath(id uint32, size float32) *Path
}

// Glyph is a positioned glyph, origin on the baseline.
type Glyph struct {
	Src  GlyphSource
	ID   uint32
	Size float32 // pixel size (logical)
	X, Y float32 // relative to the Text op origin
	// Skew slants the outline (synthetic italic): x += Skew * height above
	// the baseline. 0 draws upright; ~0.2 matches common obliques.
	Skew float32
}

// Op is any display-list command.
type Op interface{ isOp() }

// FillRect fills a rounded rectangle.
type FillRect struct {
	Rect  geom.Rect
	Radii geom.Radii
	Color color.NRGBA
}

// StrokeRect draws a border inside the rounded rectangle (CSS border-box).
type StrokeRect struct {
	Rect  geom.Rect
	Radii geom.Radii
	Width float32
	Color color.NRGBA
	Dash  []float32
}

// ShadowRect draws a blurred box shadow behind a rounded rectangle.
type ShadowRect struct {
	Rect   geom.Rect
	Radii  geom.Radii
	Shadow Shadow
}

// FillPath fills an arbitrary path (non-zero rule). Renderers may cache
// the path's rasterized coverage by *Path pointer (the GPU renderer does,
// also for a Gradient's Path): the content of a *Path must not change once
// it is drawn; to draw a different shape, use a new Path.
type FillPath struct {
	Path  *Path
	Color color.NRGBA
}

// LinearGradient fills a rounded rect with a two-stop gradient from
// From (at P0) to To (at P1).
type LinearGradient struct {
	Rect     geom.Rect
	Radii    geom.Radii
	P0, P1   geom.Point
	From, To color.NRGBA
}

// Stop is one gradient color stop; Offset runs 0..1 along the gradient.
type Stop struct {
	Offset float32
	Color  color.NRGBA
}

// GradientKind selects the gradient geometry.
//
// Append-only: never reorder or insert; the zero value stays GradientLinear.
type GradientKind uint8

const (
	// GradientLinear runs from Start (offset 0) to End (offset 1).
	GradientLinear GradientKind = iota
	// GradientRadial is centred on Start; End lies on the offset-1 circle.
	GradientRadial
	// GradientConic sweeps clockwise around Start beginning in the
	// direction of End (offset 0) and ending there after a full turn.
	GradientConic
)

// Gradient fills a rounded rect (or Path, when set) with a multi-stop
// gradient. Stops are stably sorted by Offset (two stops at one offset are
// a hard edge); colors are interpolated in non-premultiplied sRGB like CSS
// and clamped beyond the first/last stop (see GradientRamp). A stop with a
// NaN Offset is ignored; ±Inf offsets count as 1 / 0.
//
// Both renderers draw through a ramp of RampWidth entries at t = i/255,
// filtered linearly between entries, so a hard edge (or any two stops
// closer than 1/255) is not perfectly sharp: it resolves to a transition
// 1/255 of the gradient's length wide (on a 255 px long gradient, one
// pixel).
type Gradient struct {
	Kind       GradientKind
	Rect       geom.Rect
	Radii      geom.Radii
	Path       *Path // optional shape; overrides Rect/Radii; immutable once drawn (see FillPath)
	Start, End geom.Point
	Stops      []Stop
}

// Checker fills a rounded rect with a two-color checkerboard (the usual
// "transparent" background behind color swatches). Cells are Cell logical
// pixels square, anchored at Rect's top-left corner which gets color A.
type Checker struct {
	Rect  geom.Rect
	Radii geom.Radii
	Cell  float32
	A, B  color.NRGBA
}

// Default checkerboard colors (used when Checker.A and B are both zero).
var (
	CheckerLight = color.NRGBA{0xff, 0xff, 0xff, 0xff}
	CheckerDark  = color.NRGBA{0xe6, 0xe6, 0xe6, 0xff}
)

// Text draws glyphs.
type Text struct {
	Origin geom.Point
	Glyphs []Glyph
	Color  color.NRGBA
}

// Image draws img scaled into Rect, optionally clipped to rounded corners.
//
// Both renderers sample the image with the same Catmull-Rom filter
// (SampleCatmullRomF, premultiplied, edges clamped) at device pixel
// centers; a source more than twice the drawn size on both axes is first
// reduced to the drawn size (ImageReduction, PrepareImage). Images are
// cached by their image.Image value (the pointer for the usual pointer
// types): after changing an image's pixels in place, call the renderer's
// InvalidateImage (paint.CPU.InvalidateImage, gpu.Renderer.InvalidateImage)
// or draw a new image value.
type Image struct {
	Img   image.Image
	Rect  geom.Rect
	Radii geom.Radii
}

// BackdropBlur blurs what is already drawn under a rounded rectangle
// (CSS backdrop-filter: blur()): the pixels of the current target — the
// window, or inside a PushOpacity group that group's own layer — inside
// Rect (rounded by Radii, clipped by the active clips) are replaced by a
// Gaussian blur of the target as drawn so far, and every later op draws
// over the blurred result. A frosted surface is a BackdropBlur followed by
// a translucent FillRect of the same shape.
//
// Radius is the Gaussian's standard deviation in logical pixels (CSS
// blur()), scaled like any length by the device scale and a uniform
// PushTransform; it is clamped to MaxBackdropBlur, and a Radius <= 0 (or
// NaN) draws nothing. The blur reads up to BackdropBlurReach beyond Rect;
// pixels beyond the window's edges repeat its edge pixels, pixels a
// group's layer holds nothing at are transparent. The blurred copy is
// composited source-over through the rectangle's coverage: where the
// target is opaque this replaces its pixels, where it is translucent (in a
// group) the unblurred pixels show through by the copy's transparency.
//
// Both renderers compute the same blur (PlanBackdropBlur; see blur.go for
// the algorithm and its cost). Under a transform that rotates, skews,
// mirrors one axis or scales non-uniformly the op draws nothing.
//
// Redraw hints: a BackdropBlur's pixels depend on everything drawn under
// it within its reach. Renderers that redraw partially (the GPU renderer)
// redraw the whole blurred region with its source whenever a hinted
// change or scroll touches it; the ui Runner damages the region likewise
// (see ui/damage.go). Code that records Damage hints itself must not rely
// on a ScrollRegion carrying blurred pixels along.
type BackdropBlur struct {
	Rect   geom.Rect
	Radii  geom.Radii
	Radius float32
}

// PushClip restricts subsequent drawing to a (rounded) rectangle until the
// matching Pop.
type PushClip struct {
	Rect  geom.Rect
	Radii geom.Radii
}

// PushOpacity draws subsequent commands into a group composited with alpha.
type PushOpacity struct{ Alpha float32 }

// PushTransform maps every subsequent command through M (composed with any
// enclosing transform: points go through M first, then the outer
// transforms) until the matching Pop. Clips and opacity groups nest freely
// inside and around it.
type PushTransform struct{ M geom.Affine }

// Pop ends the innermost PushClip / PushOpacity / PushTransform.
type Pop struct{}

// Damage is a redraw hint, not a drawing command: Rect is a region whose
// pixels changed since the previous frame. Rect is in the logical
// coordinates of the place it is recorded, i.e. it goes through the
// enclosing PushTransform stack like geometry does (clips and opacity
// groups do not affect it); hints recorded by the ui Runner at the end of
// a frame are therefore in window logical pixels.
//
// A frame that carries no Damage and no ScrollRegion must be redrawn
// completely; once a frame carries any hint, everything that changed is
// covered by the union of its Damage rects and ScrollRegions. Renderers
// that do not do partial redraws ignore both ops (paint.CPU does; the GPU
// renderer in internal/render/gpu redraws only the hinted region when its device keeps
// the previous frame).
type Damage struct{ Rect geom.Rect }

// ScrollRegion is a redraw hint: the pixels inside Rect moved by Delta
// since the previous frame (a pixel at p is now at p+Delta, clipped to
// Rect), so a renderer may copy them instead of redrawing. The strip the
// copy leaves uncovered and anything inside Rect that did not move with
// the content are covered by Damage hints of the same frame. Coordinates
// follow the same rule as Damage. Renderers apply every ScrollRegion of
// a frame before redrawing its Damage.
type ScrollRegion struct {
	Rect  geom.Rect
	Delta geom.Point
}

func (FillRect) isOp()       {}
func (StrokeRect) isOp()     {}
func (ShadowRect) isOp()     {}
func (FillPath) isOp()       {}
func (LinearGradient) isOp() {}
func (Text) isOp()           {}
func (Image) isOp()          {}
func (PushClip) isOp()       {}
func (PushOpacity) isOp()    {}
func (Gradient) isOp()       {}
func (Checker) isOp()        {}
func (PushTransform) isOp()  {}
func (Pop) isOp()            {}
func (Damage) isOp()         {}
func (ScrollRegion) isOp()   {}
func (BackdropBlur) isOp()   {}

// List is a recorded frame.
type List struct {
	Ops []Op
	// Background clears the target before drawing (zero = transparent).
	Background color.NRGBA
	// IconNote, when set, is called by the icon package for every icon
	// drawn into the list: its name, whether it is directional (mirrors in
	// right-to-left layouts), whether this draw mirrored it, and the square
	// it was drawn into (in the list's coordinates at that point). Only
	// tools and tests set it (the RTL mirror check, the accessibility
	// audit); renderers ignore it. It belongs to the list, so parallel
	// tests do not see each other's icons. Reset keeps it.
	IconNote func(name string, directional, mirrored bool, r geom.Rect)
	// IconStyle is the icon style (a *icon.Style) the icon package applies
	// to icons drawn into the list: the theme's replacement icons and
	// stroke width. The ui runner sets it from the theme of the scope
	// being painted; renderers ignore it. Reset keeps it.
	IconStyle any
}

// Reset empties the list for reuse.
func (l *List) Reset() { l.Ops = l.Ops[:0] }

// Add appends a raw op. Add ops by value (l.Add(FillRect{…})), not by
// pointer: paint.CPU skips pointer ops (the GPU renderer dereferences
// them).
func (l *List) Add(op Op) { l.Ops = append(l.Ops, op) }

// Fill fills a rounded rect. Fully transparent colors are dropped.
func (l *List) Fill(r geom.Rect, rad geom.Radii, c color.NRGBA) {
	if c.A == 0 || r.Empty() {
		return
	}
	l.Ops = append(l.Ops, FillRect{r, rad, c})
}

// Stroke draws a border of width w inside r.
func (l *List) Stroke(r geom.Rect, rad geom.Radii, w float32, c color.NRGBA) {
	if c.A == 0 || w <= 0 || r.Empty() {
		return
	}
	l.Ops = append(l.Ops, StrokeRect{Rect: r, Radii: rad, Width: w, Color: c})
}

// StrokeDashed draws a dashed border.
func (l *List) StrokeDashed(r geom.Rect, rad geom.Radii, w float32, c color.NRGBA, dash []float32) {
	if c.A == 0 || w <= 0 || r.Empty() {
		return
	}
	l.Ops = append(l.Ops, StrokeRect{Rect: r, Radii: rad, Width: w, Color: c, Dash: dash})
}

// Shadows draws layered box shadows (in CSS order: first is on top, so they
// are recorded back to front).
func (l *List) Shadows(r geom.Rect, rad geom.Radii, s []Shadow) {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i].Color.A == 0 {
			continue
		}
		l.Ops = append(l.Ops, ShadowRect{r, rad, s[i]})
	}
}

// Path fills p.
func (l *List) Path(p *Path, c color.NRGBA) {
	if p == nil || c.A == 0 {
		return
	}
	l.Ops = append(l.Ops, FillPath{p, c})
}

// StrokePath strokes p.
func (l *List) StrokePath(p *Path, st StrokeStyle, c color.NRGBA) {
	if p == nil || c.A == 0 {
		return
	}
	l.Ops = append(l.Ops, FillPath{StrokeToFill(p, st), c})
}

// Line draws a straight line of width w.
func (l *List) Line(a, b geom.Point, w float32, c color.NRGBA) {
	if a.X == b.X || a.Y == b.Y { // axis-aligned: exact rect
		r := geom.Rect{X: min(a.X, b.X), Y: min(a.Y, b.Y), W: max(a.X, b.X) - min(a.X, b.X), H: max(a.Y, b.Y) - min(a.Y, b.Y)}
		if r.W == 0 {
			r.X -= w / 2
			r.W = w
		} else {
			r.Y -= w / 2
			r.H = w
		}
		l.Fill(r, geom.Radii{}, c)
		return
	}
	l.StrokePath(NewPath().MoveTo(a.X, a.Y).LineTo(b.X, b.Y), StrokeStyle{Width: w}, c)
}

// Glyphs draws a glyph run at origin.
func (l *List) Glyphs(origin geom.Point, g []Glyph, c color.NRGBA) {
	if len(g) == 0 || c.A == 0 {
		return
	}
	l.Ops = append(l.Ops, Text{origin, g, c})
}

// DrawImage draws img into r.
func (l *List) DrawImage(img image.Image, r geom.Rect, rad geom.Radii) {
	if img == nil || r.Empty() {
		return
	}
	l.Ops = append(l.Ops, Image{img, r, rad})
}

// BackdropBlur blurs the pixels already drawn under the rounded rect r
// with a Gaussian of standard deviation radius (logical pixels; see
// BackdropBlur). Empty rects and radii <= 0 are dropped.
func (l *List) BackdropBlur(r geom.Rect, rad geom.Radii, radius float32) {
	if r.Empty() || !(radius > 0) {
		return
	}
	l.Ops = append(l.Ops, BackdropBlur{Rect: r, Radii: rad, Radius: radius})
}

// Clip pushes a clip; pair with Pop.
func (l *List) Clip(r geom.Rect, rad geom.Radii) { l.Ops = append(l.Ops, PushClip{r, rad}) }

// Opacity pushes an opacity group; pair with Pop.
func (l *List) Opacity(a float32) { l.Ops = append(l.Ops, PushOpacity{a}) }

// Transform pushes an affine transform group; pair with Pop. See
// geom.Translate, geom.Scale, geom.Rotate, geom.ScaleAround and
// geom.RotateAround for the usual constructors.
func (l *List) Transform(m geom.Affine) { l.Ops = append(l.Ops, PushTransform{m}) }

// Gradient fills with a multi-stop gradient (see Gradient).
func (l *List) Gradient(g Gradient) {
	if len(g.Stops) == 0 || (g.Path == nil && g.Rect.Empty()) {
		return
	}
	l.Ops = append(l.Ops, g)
}

// LinearStops fills r with a linear gradient from p0 to p1 through stops.
func (l *List) LinearStops(r geom.Rect, rad geom.Radii, p0, p1 geom.Point, stops ...Stop) {
	l.Gradient(Gradient{Kind: GradientLinear, Rect: r, Radii: rad, Start: p0, End: p1, Stops: stops})
}

// Checker fills r with a checkerboard of cell-sized squares. Zero colors
// use CheckerLight / CheckerDark.
func (l *List) Checker(r geom.Rect, rad geom.Radii, cell float32, a, b color.NRGBA) {
	if r.Empty() || cell <= 0 {
		return
	}
	l.Ops = append(l.Ops, Checker{r, rad, cell, a, b})
}

// Pop closes the innermost group.
func (l *List) Pop() { l.Ops = append(l.Ops, Pop{}) }

// Damage records a redraw hint for r (see Damage). Empty rects are dropped.
func (l *List) Damage(r geom.Rect) {
	if r.Empty() {
		return
	}
	l.Ops = append(l.Ops, Damage{r})
}

// Scroll records that the pixels inside r moved by d (see ScrollRegion).
// Empty rects and zero deltas are dropped.
func (l *List) Scroll(r geom.Rect, d geom.Point) {
	if r.Empty() || d == (geom.Point{}) {
		return
	}
	l.Ops = append(l.Ops, ScrollRegion{r, d})
}

// Renderer turns a List into pixels. The CPU renderer targets an
// *image.RGBA; GPU renderers target a swapchain owned by the platform host.
// size is the logical frame size and scale the device pixel ratio.
//
// Implementations must handle every drawing Op type in this package; the
// hint ops Damage and ScrollRegion may be ignored (redraw everything).
// Push* ops
// (PushClip, PushOpacity, PushTransform) form a stack closed by Pop in LIFO
// order; a renderer keeps the current transform on that stack (identity at
// the start of a frame) and applies it to all geometry, including clips,
// shadows, glyphs and images. Renderers that cannot rasterize a rotated
// glyph or image natively may fall back to filling the transformed outline
// (Path.Transform) or sampling the image through the inverse transform, as
// the CPU renderer does.
type Renderer interface {
	Render(l *List, size geom.Size, scale float32) error
}
