package d3d11

import (
	"errors"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/gpu"
	"github.com/tangoui/tango/ui/gpu/internal/devkit"
	"github.com/tangoui/tango/ui/gpu/shaders"
	"github.com/tangoui/tango/ui/shader/sl"
)

// The layer pool, partial-frame, scratch and clearing-quad tests live
// with the code in ui/gpu/internal/devkit; this checks the d3d11 wrappers.

func TestDevkitWrappers(t *testing.T) {
	sz := image.Pt(16, 16)
	ld := gpu.LayerDesc{Size: sz, Bounds: image.Rectangle{Max: sz}}
	p := &layerPool[int]{devkit.LayerPool[int]{
		NewTex: func(image.Point) (int, error) { return 0, errors.New("d3d11: CreateTexture2D failed") },
		Free:   func(int) {},
	}}
	err := p.assign(&gpu.Frame{Layers: []gpu.LayerDesc{ld}, Batches: []gpu.Batch{{Layer: 1}, {Target: 1}}})
	if err == nil || isAllocError(err) || !strings.HasPrefix(err.Error(), "d3d11: batch 0 samples layer 1") {
		t.Errorf("invalid frame: %v", err)
	}
	err = p.assign(&gpu.Frame{Layers: []gpu.LayerDesc{ld}, Batches: []gpu.Batch{{Target: 1}, {Layer: 1}}})
	if !isAllocError(err) || err.Error() != "d3d11: CreateTexture2D failed" {
		t.Errorf("creation failure: %v, want the allocError as is", err)
	}
	size := image.Pt(10, 10)
	if err := checkPartial(&gpu.Frame{Size: size, Dirty: []image.Rectangle{image.Rect(0, 0, 11, 1)}}); err == nil || !strings.HasPrefix(err.Error(), "d3d11: dirty rectangle") {
		t.Errorf("checkPartial: %v", err)
	}
	cp := []gpu.CopyRegion{{Src: image.Rect(0, 0, 4, 6), Dst: image.Rect(0, 0, 4, 6)}}
	if _, _, _, err := scratchPlan(cp, image.Point{}, false, 4); !errors.Is(err, gpu.ErrNotRetained) || !strings.HasPrefix(err.Error(), "d3d11: copy sources") {
		t.Errorf("scratchPlan: %v", err)
	}
	if got := clearInstances(nil, []image.Rectangle{image.Rect(0, 0, 1, 1)}, color.NRGBA{A: 255}); len(got) != 1 || got[0].Color != sl.V4(0, 0, 0, 1) {
		t.Errorf("clearInstances: %+v", got)
	}
}

func TestImageUploads(t *testing.T) {
	pix := make([]byte, 3*12+8)
	for i := range pix {
		pix[i] = byte(i)
	}
	// Stride 12, two texels per row: the last row is short, so the pixels
	// are packed into whole rows.
	u := gpu.ImageUpload{ID: 1, Size: image.Pt(2, 4), Pix: pix, Stride: 12}
	if err := checkImageUpload(u); err != nil {
		t.Fatal(err)
	}
	got, pitch := imageUploadPix(u)
	if pitch != 8 || len(got) != 32 || got[8] != 12 || got[24] != 36 {
		t.Fatalf("packed: pitch %d, %d bytes, row 1 at %d, row 3 at %d", pitch, len(got), got[8], got[24])
	}
	whole := gpu.ImageUpload{ID: 1, Size: image.Pt(2, 3), Pix: pix[:36], Stride: 12}
	if got, pitch := imageUploadPix(whole); pitch != 12 || &got[0] != &pix[0] {
		t.Errorf("whole rows were copied (pitch %d)", pitch)
	}
	for name, u := range map[string]gpu.ImageUpload{
		"zero id":  {ID: 0, Size: image.Pt(1, 1), Pix: make([]byte, 4), Stride: 4},
		"empty":    {ID: 1, Size: image.Pt(0, 1)},
		"short":    {ID: 1, Size: image.Pt(2, 2), Pix: make([]byte, 15), Stride: 8},
		"negative": {ID: 1, Size: image.Pt(1, 2), Pix: make([]byte, 64), Stride: -4},
	} {
		if err := checkImageUpload(u); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestInstGather(t *testing.T) {
	comp := func(a float32) shaders.Instance {
		return shaders.Instance{UV: sl.V4(0, 0, 1, 1), Color: sl.V4(a, a, a, a), Kind: sl.V4(shaders.KindPatch, 0.5/32, 0.5/32, 1)}
	}
	solid := shaders.Instance{Kind: sl.V4(shaders.KindSolid, 0, 0, 0)}
	l1 := []shaders.Instance{comp(0.5), solid}
	l2 := []shaders.Instance{comp(0.25)}
	plain := []shaders.Instance{solid, solid}
	f := &gpu.Frame{
		Size:   image.Pt(100, 100),
		Clear:  color.NRGBA{255, 255, 255, 255},
		Layers: []gpu.LayerDesc{{Size: image.Pt(32, 32)}, {Size: image.Pt(32, 32)}},
		Batches: []gpu.Batch{
			{Target: 1, Instances: plain},
			{Target: 2, Instances: plain},
			{Layer: 1, Instances: l1}, // layer 1 in a 64×64 pooled texture: rescaled
			{Layer: 2, Instances: l2}, // layer 2 in its own size: as is
			{Instances: nil},
		},
		Dirty: []image.Rectangle{image.Rect(0, 0, 10, 10)},
	}
	layerSize := []image.Point{{}, image.Pt(64, 64), image.Pt(32, 32)}
	var g instGather
	got := g.gather(f, layerSize)
	if len(got) != len(f.Batches)+1 {
		t.Fatalf("%d slices, want %d batches and the clearing quads", len(got), len(f.Batches))
	}
	if &got[0][0] != &plain[0] || &got[3][0] != &l2[0] || len(got[4]) != 0 {
		t.Error("batches that need no rescaling are not the frame's own slices")
	}
	if &got[2][0] == &l1[0] {
		t.Fatal("the rescaled batch aliases the frame's instances")
	}
	if l1[0].UV != sl.V4(0, 0, 1, 1) {
		t.Error("the frame's instances were modified")
	}
	if got[2][0].UV != sl.V4(0, 0, 0.5, 0.5) || got[2][0].Kind.Y != 0.5/64 || got[2][1] != solid {
		t.Errorf("rescaled batch %+v", got[2])
	}
	if cap(got[2]) != len(got[2]) {
		t.Error("the rescaled slice can grow into its neighbour")
	}
	if len(got[5]) != 1 || got[5][0].Rect != sl.V4(-1, -1, 12, 12) {
		t.Errorf("clearing quads %+v", got[5])
	}
	// A full frame ends with an empty clearing entry; buffers are reused.
	f.Dirty = nil
	got = g.gather(f, layerSize)
	if len(got) != len(f.Batches)+1 || len(got[5]) != 0 {
		t.Fatalf("full frame: %d slices, last %d", len(got), len(got[len(got)-1]))
	}
	g.reset()
	for _, s := range g.batches[:cap(g.batches)][:len(f.Batches)] {
		if s != nil {
			t.Fatal("reset kept a reference to the frame")
		}
	}
}
