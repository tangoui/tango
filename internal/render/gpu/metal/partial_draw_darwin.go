//go:build darwin

package metal

import (
	"image"
	"slices"

	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/shaders"
)

// dirtyDrawPlan packs the main target's partial draws into the instance
// buffer in batch and dirty-rectangle order. Each draw copies runs of
// consecutive instances straight from the batch to the shared buffer, in
// their original order. The plan retains no references to the frame.
type dirtyDrawPlan struct {
	batches []dirtyBatch
	draws   []dirtyDraw
	runs    []instanceRun
	bounds  []quadBounds
}

type dirtyBatch struct{ drawEnd, runEnd int }

type dirtyDraw struct {
	scissor image.Rectangle
	count   int
}

type instanceRun struct{ first, end int }

type quadBounds struct{ minX, minY, maxX, maxY float32 }

func instanceQuad(in *shaders.Instance) quadBounds {
	// Match shaders.VS's rasterized quad, including its one-pixel pad
	// and shadow reach. Glyph Rects already describe the rasterized mask.
	pad := float32(1)
	if in.Kind.X > shaders.KindStroke+0.5 && in.Kind.X < shaders.KindShadow+0.5 {
		pad += max(in.Params.Y, 0)
	}
	x0, y0 := in.Rect.X-pad, in.Rect.Y-pad
	x1, y1 := x0+(in.Rect.Z+2*pad), y0+(in.Rect.W+2*pad)
	return quadBounds{min(x0, x1), min(y0, y1), max(x0, x1), max(y0, y1)}
}

func (p *dirtyDrawPlan) build(f *gpu.Frame) int {
	p.batches = slices.Grow(p.batches[:0], len(f.Batches))
	p.draws, p.runs = p.draws[:0], p.runs[:0]
	total := 0
	for i := range f.Batches {
		b := &f.Batches[i]
		switch {
		case b.Target != 0:
			total += len(b.Instances)
		case len(b.Instances) != 0:
			sc := image.Rectangle{Max: f.Size}
			if !b.Scissor.Empty() {
				sc = sc.Intersect(b.Scissor)
			}
			bounds := p.bounds[:0]
			for _, r := range f.Dirty {
				s := sc.Intersect(r)
				if s.Empty() {
					continue
				}
				if len(bounds) == 0 {
					// Only compute footprints for batches that reach a
					// dirty rectangle, and reuse them across rectangles.
					bounds = slices.Grow(bounds, len(b.Instances))[:len(b.Instances)]
					for j := range b.Instances {
						bounds[j] = instanceQuad(&b.Instances[j])
					}
					p.bounds = bounds
				}
				total += p.addDraw(s, bounds)
			}
		}
		p.batches = append(p.batches, dirtyBatch{len(p.draws), len(p.runs)})
	}
	return total
}

func (p *dirtyDrawPlan) addDraw(sc image.Rectangle, bounds []quadBounds) int {
	x0, y0 := float32(sc.Min.X), float32(sc.Min.Y)
	x1, y1 := float32(sc.Max.X), float32(sc.Max.Y)
	first, count := -1, 0
	for i, b := range bounds {
		// Compare against pixel edges (the samples are half a pixel
		// inside), leaving room for viewport and rasterizer rounding.
		if b.maxY >= y0 && b.minY <= y1 && b.maxX >= x0 && b.minX <= x1 {
			if first < 0 {
				first = i
			}
		} else if first >= 0 {
			p.runs = append(p.runs, instanceRun{first, i})
			count += i - first
			first = -1
		}
	}
	if first >= 0 {
		p.runs = append(p.runs, instanceRun{first, len(bounds)})
		count += len(bounds) - first
	}
	if count > 0 {
		p.draws = append(p.draws, dirtyDraw{sc, count})
	}
	return count
}
