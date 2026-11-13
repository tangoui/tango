package d3d11

import (
	"errors"
	"image"
	"image/color"
	"testing"

	"github.com/tangoui/tango/ui/gpu"
	"github.com/tangoui/tango/ui/gpu/shaders"
	"github.com/tangoui/tango/ui/shader/sl"
)

// fakePool is a layer pool over integer texture handles that records
// creations and releases.
type fakePool struct {
	*layerPool[int]
	next    int
	live    map[int]image.Point
	failNew bool
}

func newFakePool() *fakePool {
	fp := &fakePool{live: map[int]image.Point{}}
	fp.layerPool = &layerPool[int]{
		newTex: func(size image.Point) (int, error) {
			if fp.failNew {
				return 0, errors.New("CreateTexture2D failed")
			}
			fp.next++
			fp.live[fp.next] = size
			return fp.next, nil
		},
		free: func(t int) {
			if _, ok := fp.live[t]; !ok {
				panic("double free")
			}
			delete(fp.live, t)
		},
	}
	return fp
}

// layerFrame is a frame drawing into each layer (one batch) and then
// compositing it onto the target (another batch), layer after layer.
func layerFrame(sizes ...image.Point) *gpu.Frame {
	f := &gpu.Frame{Size: image.Pt(800, 600)}
	for i, s := range sizes {
		f.Layers = append(f.Layers, gpu.LayerDesc{Size: s, Bounds: image.Rectangle{Max: s}})
		f.Batches = append(f.Batches, gpu.Batch{Target: i + 1}, gpu.Batch{Layer: i + 1})
	}
	return f
}

func TestLayerPoolReuse(t *testing.T) {
	p := newFakePool()
	if err := p.assign(layerFrame(image.Pt(100, 50))); err != nil {
		t.Fatal(err)
	}
	first := p.tex[1]
	for range 10 {
		if err := p.assign(layerFrame(image.Pt(100, 50))); err != nil {
			t.Fatal(err)
		}
	}
	if p.count() != 1 || p.tex[1] != first {
		t.Fatalf("10 equal frames: %d textures, texture %d (first %d)", p.count(), p.tex[1], first)
	}
	// Smaller but at least half: the same texture, its size reported.
	if err := p.assign(layerFrame(image.Pt(60, 30))); err != nil {
		t.Fatal(err)
	}
	if p.count() != 1 || p.size[1] != image.Pt(100, 50) {
		t.Fatalf("60×30 layer: %d textures, size %v", p.count(), p.size[1])
	}
	// Less than half: a new texture of its size; the larger one stays.
	if err := p.assign(layerFrame(image.Pt(40, 20))); err != nil {
		t.Fatal(err)
	}
	if p.count() != 2 || p.size[1] != image.Pt(40, 20) {
		t.Fatalf("40×20 layer: %d textures, size %v", p.count(), p.size[1])
	}
	// Two layers in sequence share a texture (the first is free after its
	// composite); nested layers (both busy) do not.
	p2 := newFakePool()
	if err := p2.assign(layerFrame(image.Pt(64, 64), image.Pt(64, 64))); err != nil {
		t.Fatal(err)
	}
	if p2.count() != 1 || p2.tex[1] != p2.tex[2] {
		t.Fatalf("sequential layers: %d textures, %d and %d", p2.count(), p2.tex[1], p2.tex[2])
	}
	nested := &gpu.Frame{
		Size:    image.Pt(200, 200),
		Layers:  []gpu.LayerDesc{{Size: image.Pt(64, 64), Bounds: image.Rect(0, 0, 64, 64)}, {Size: image.Pt(64, 64), Bounds: image.Rect(0, 0, 64, 64)}},
		Batches: []gpu.Batch{{Target: 1}, {Target: 2}, {Target: 1, Layer: 2}, {Layer: 1}},
	}
	if err := p2.assign(nested); err != nil {
		t.Fatal(err)
	}
	if p2.count() != 2 || p2.tex[1] == p2.tex[2] {
		t.Fatalf("nested layers: %d textures, %d and %d", p2.count(), p2.tex[1], p2.tex[2])
	}
	for _, l := range p2.texs {
		if l.busy {
			t.Fatal("a texture is still busy after the frame")
		}
	}
}

func TestLayerPoolIdleAndBudget(t *testing.T) {
	p := newFakePool()
	p.assign(layerFrame(image.Pt(32, 32)))
	for range layerIdleFrames {
		p.assign(layerFrame())
	}
	if p.count() != 1 {
		t.Fatalf("after %d idle frames: %d textures, want 1", layerIdleFrames, p.count())
	}
	p.assign(layerFrame())
	if p.count() != 0 || len(p.live) != 0 {
		t.Fatalf("after %d idle frames: %d textures (%d live), want none", layerIdleFrames+1, p.count(), len(p.live))
	}

	// 64 MiB each; neither fits the other's layer.
	a, b, c := image.Pt(4096, 4096), image.Pt(2000, 8000), image.Pt(8000, 2000)
	p.assign(layerFrame(a))
	p.assign(layerFrame(b))
	if p.count() != 2 {
		t.Fatalf("two 64 MiB textures: %d", p.count())
	}
	p.assign(layerFrame(c)) // 192 MiB would exceed the budget: the oldest goes
	if p.count() != 2 || p.bytes() > layerPoolBytes {
		t.Fatalf("after a third: %d textures, %d bytes", p.count(), p.bytes())
	}
	for _, l := range p.texs {
		if l.size == a {
			t.Fatal("the least recently used texture was kept")
		}
	}
	// One frame needing more than the budget at once keeps them all for
	// that frame (they are busy together).
	q := newFakePool()
	big := &gpu.Frame{Size: image.Pt(10, 10)}
	for i := range 3 {
		big.Layers = append(big.Layers, gpu.LayerDesc{Size: a, Bounds: image.Rectangle{Max: a}})
		big.Batches = append(big.Batches, gpu.Batch{Target: i + 1})
	}
	for i := range 3 {
		big.Batches = append(big.Batches, gpu.Batch{Layer: i + 1})
	}
	if err := q.assign(big); err != nil {
		t.Fatal(err)
	}
	if q.tex[1] == q.tex[2] || q.tex[2] == q.tex[3] || q.tex[1] == q.tex[3] {
		t.Fatal("simultaneously busy layers share a texture")
	}
	q.assign(layerFrame()) // trimmed to the budget when the next frame starts
	if q.bytes() > layerPoolBytes {
		t.Fatalf("pool keeps %d bytes after the frame", q.bytes())
	}
}

func TestLayerPoolErrors(t *testing.T) {
	sz := image.Pt(16, 16)
	ld := gpu.LayerDesc{Size: sz, Bounds: image.Rectangle{Max: sz}}
	for name, f := range map[string]*gpu.Frame{
		"sample undrawn": {Layers: []gpu.LayerDesc{ld}, Batches: []gpu.Batch{{Layer: 1}, {Target: 1}}},
		"out of range":   {Layers: []gpu.LayerDesc{ld}, Batches: []gpu.Batch{{Target: 2}}},
		"negative":       {Layers: []gpu.LayerDesc{ld}, Batches: []gpu.Batch{{Layer: -1}}},
		"own target":     {Layers: []gpu.LayerDesc{ld, ld}, Batches: []gpu.Batch{{Target: 2}, {Target: 1, Layer: 1}}},
		"bad size":       {Layers: []gpu.LayerDesc{{Size: image.Pt(8, 8), Bounds: image.Rect(0, 0, 9, 8)}}, Batches: []gpu.Batch{{Target: 1}}},
		"zero size":      {Layers: []gpu.LayerDesc{{}}, Batches: []gpu.Batch{{Target: 1}}},
	} {
		p := newFakePool()
		err := p.assign(f)
		if err == nil || isAllocError(err) {
			t.Errorf("%s: %v, want an invalid-frame error", name, err)
		}
		for _, l := range p.texs {
			if l.busy {
				t.Errorf("%s: a texture stays busy", name)
			}
		}
	}
	p := newFakePool()
	p.failNew = true
	if err := p.assign(layerFrame(sz)); !isAllocError(err) {
		t.Errorf("creation failure: %v, want an allocError", err)
	}
	p.failNew = false
	if err := p.assign(layerFrame(sz)); err != nil || p.count() != 1 {
		t.Errorf("after a creation failure: %v, %d textures", err, p.count())
	}
	p.releaseAll()
	if len(p.live) != 0 || p.count() != 0 {
		t.Errorf("releaseAll left %d textures", len(p.live))
	}
}

func TestRescaleComposites(t *testing.T) {
	comp := shaders.Instance{UV: sl.V4(0, 0, 0.5, 1), Kind: sl.V4(shaders.KindPatch, 0.5/64, 0.5/32, 1)}
	patch := shaders.Instance{UV: sl.V4(0, 0, 0.5, 1), Kind: sl.V4(shaders.KindPatch, 0.1, 0.1, 0)}
	ins := []shaders.Instance{comp, patch}
	rescaleComposites(ins, image.Pt(64, 32), image.Pt(128, 128))
	if ins[0].UV != sl.V4(0, 0, 0.25, 0.25) || ins[0].Kind.Y != 0.5/128 || ins[0].Kind.Z != 0.5/128 {
		t.Errorf("composite: UV %v, half-texel %v %v", ins[0].UV, ins[0].Kind.Y, ins[0].Kind.Z)
	}
	if ins[1] != patch {
		t.Error("a patch-page instance was rescaled")
	}
	same := []shaders.Instance{comp}
	rescaleComposites(same, image.Pt(64, 32), image.Pt(64, 32))
	if same[0] != comp {
		t.Error("rescaled to its own size")
	}
}

func TestCheckPartial(t *testing.T) {
	size := image.Pt(100, 50)
	ok := &gpu.Frame{Size: size, Dirty: []image.Rectangle{image.Rect(0, 0, 10, 10), image.Rect(10, 0, 20, 10)},
		Copies: []gpu.CopyRegion{{Src: image.Rect(0, 10, 100, 50), Dst: image.Rect(0, 0, 100, 40)}}}
	if err := checkPartial(ok); err != nil {
		t.Fatalf("valid partial frame: %v", err)
	}
	for name, f := range map[string]*gpu.Frame{
		"empty dirty":   {Size: size, Dirty: []image.Rectangle{image.Rect(3, 3, 3, 9)}},
		"dirty outside": {Size: size, Dirty: []image.Rectangle{image.Rect(90, 0, 101, 5)}},
		"overlap":       {Size: size, Dirty: []image.Rectangle{image.Rect(0, 0, 10, 10), image.Rect(9, 9, 12, 12)}},
		"copy sizes":    {Size: size, Dirty: ok.Dirty, Copies: []gpu.CopyRegion{{Src: image.Rect(0, 0, 4, 4), Dst: image.Rect(0, 0, 4, 5)}}},
		"copy outside":  {Size: size, Dirty: ok.Dirty, Copies: []gpu.CopyRegion{{Src: image.Rect(0, 0, 4, 4), Dst: image.Rect(98, 0, 102, 4)}}},
		"copy empty":    {Size: size, Dirty: ok.Dirty, Copies: []gpu.CopyRegion{{}}},
	} {
		if err := checkPartial(f); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestScratchPlan(t *testing.T) {
	const lim = 16384
	cp := func(w, h int) gpu.CopyRegion {
		return gpu.CopyRegion{Src: image.Rect(0, 0, w, h), Dst: image.Rect(1, 1, 1+w, 1+h)}
	}
	type res struct {
		used, create bool
		size         image.Point
	}
	for _, c := range []struct {
		name   string
		copies []gpu.CopyRegion
		have   image.Point
		exists bool
		want   res
	}{
		{"no copies", nil, image.Pt(10, 10), true, res{}},
		{"first", []gpu.CopyRegion{cp(300, 100), cp(200, 50)}, image.Point{}, false, res{true, true, image.Pt(300, 150)}},
		{"fits", []gpu.CopyRegion{cp(300, 140)}, image.Pt(300, 150), true, res{true, false, image.Pt(300, 150)}},
		{"grows, keeping the larger side", []gpu.CopyRegion{cp(200, 200)}, image.Pt(300, 150), true, res{true, true, image.Pt(300, 200)}},
		{"more than 4× too large", []gpu.CopyRegion{cp(100, 50)}, image.Pt(400, 200), true, res{true, true, image.Pt(100, 50)}},
		{"exactly 4×", []gpu.CopyRegion{cp(200, 100)}, image.Pt(400, 200), true, res{true, false, image.Pt(400, 200)}},
		{"grow from a much larger one", []gpu.CopyRegion{cp(20, 900)}, image.Pt(1000, 100), true, res{true, true, image.Pt(20, 900)}},
		{"at the limit", []gpu.CopyRegion{cp(lim, 8000), cp(10, 8384)}, image.Point{}, false, res{true, true, image.Pt(lim, lim)}},
	} {
		used, create, size, err := scratchPlan(c.copies, c.have, c.exists, lim)
		if got := (res{used, create, size}); err != nil || got != c.want {
			t.Errorf("%s: %+v, %v; want %+v", c.name, got, err, c.want)
		}
	}
	// Sources stacked taller than the limit: ErrNotRetained (drawn in
	// full), never ErrTargetTooLarge or a failure.
	for _, l := range []int{lim, 8192} {
		_, _, _, err := scratchPlan([]gpu.CopyRegion{cp(100, l/2), cp(100, l/2+1)}, image.Point{}, false, l)
		if !errors.Is(err, gpu.ErrNotRetained) || errors.Is(err, gpu.ErrTargetTooLarge) {
			t.Errorf("limit %d: %v, want ErrNotRetained", l, err)
		}
	}
}

func TestClearInstances(t *testing.T) {
	dirty := []image.Rectangle{image.Rect(10, 20, 30, 25)}
	ins := clearInstances(nil, dirty, color.NRGBA{255, 255, 255, 255})
	if len(ins) != 1 {
		t.Fatalf("%d quads", len(ins))
	}
	if ins[0].Rect != sl.V4(9, 19, 22, 7) || ins[0].Color != sl.V4(1, 1, 1, 1) || ins[0].Kind.X != shaders.KindSolid {
		t.Errorf("quad %+v", ins[0])
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
