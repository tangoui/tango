//go:build darwin

package metal

import (
	"bytes"
	"image"
	"slices"
	"testing"

	"github.com/ebitengine/purego/objc"
	"github.com/tangoui/tango/internal/render/gpu"
	"github.com/tangoui/tango/internal/render/gpu/shaders"
	"github.com/tangoui/tango/internal/render/shader/sl"
)

func TestMetalPartialDrawsOnlyIntersectingInstances(t *testing.T) {
	d := newDevice(t)
	f := gpu.Frame{
		Size: image.Pt(96, 32),
		Batches: []gpu.Batch{
			{Instances: []shaders.Instance{
				{Rect: sl.V4(4, 4, 10, 16), Color: sl.V4(.5, 0, 0, .5)},
				{Rect: sl.V4(52, 4, 18, 16), Color: sl.V4(0, .5, 0, .5)},
				{Rect: sl.V4(20, 8, 10, 8), Color: sl.V4(0, 0, 0, .75),
					Kind: sl.V4(shaders.KindShadow, 0, 0, 0), Params: sl.V4(4, 12, 0, 0)},
				{Rect: sl.V4(6, 9, 62, 6), Color: sl.V4(.25, 0, .25, .5)},
				{Rect: sl.V4(8, 4, 7, 16), Color: sl.V4(0, 0, .5, .5)},
			}},
			{Scissor: image.Rect(48, 0, 75, 32), Instances: []shaders.Instance{
				{Rect: sl.V4(0, 12, 96, 6), Color: sl.V4(.25, .25, 0, .5)},
				{Rect: sl.V4(82, 4, 6, 16), Color: sl.V4(.25, 0, .25, .5)},
			}},
		},
	}
	if err := d.Execute(&f); err != nil {
		t.Fatal(err)
	}
	full := readPixels(t, d)
	// The shadow reaches the first dirty rectangle even though its Rect
	// is outside it. The last dirty rectangle only needs a clear.
	if p := full.RGBAAt(15, 15); p.A == 0 {
		t.Fatal("fixture does not render a shadow outside its Rect")
	}
	f.Dirty = []image.Rectangle{image.Rect(2, 2, 16, 22), image.Rect(50, 2, 73, 22), image.Rect(80, 2, 91, 22)}

	var draws []uint64
	err := func() error {
		draw := msgDraw
		defer func() { msgDraw = draw }()
		msgDraw = func(o id, s objc.SEL, prim, start, count, instances uint64) {
			draws = append(draws, instances)
			draw(o, s, prim, start, count, instances)
		}
		return d.Execute(&f)
	}()
	if err != nil {
		t.Fatal(err)
	}
	if partial := readPixels(t, d); !bytes.Equal(partial.Pix, full.Pix) {
		t.Error("partial redraw differs from the full redraw")
	}
	want := []uint64{1, 1, 1, 4, 2, 1}
	if !slices.Equal(draws, want) {
		t.Errorf("instances per draw (including three clears) = %v, want %v", draws, want)
	}
}
