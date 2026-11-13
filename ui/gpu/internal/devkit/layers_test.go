package devkit

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
	*LayerPool[int]
	next    int
	live    map[int]image.Point
	failNew bool
}

func newFakePool() *fakePool {
	fp := &fakePool{live: map[int]image.Point{}}
	fp.LayerPool = &LayerPool[int]{
		NewTex: func(size image.Point) (int, error) {
			if fp.failNew {
				return 0, errors.New("CreateTexture2D failed")
			}
			fp.next++
			fp.live[fp.next] = size
			return fp.next, nil
		},
		Free: func(t int) {
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
	if err := p.Assign(layerFrame(image.Pt(100, 50))); err != nil {
		t.Fatal(err)
	}
	first := p.Tex[1]
	for range 10 {
		if err := p.Assign(layerFrame(image.Pt(100, 50))); err != nil {
			t.Fatal(err)
		}
	}
	if p.Count() != 1 || p.Tex[1] != first {
		t.Fatalf("10 equal frames: %d textures, texture %d (first %d)", p.Count(), p.Tex[1], first)
	}
	// Smaller but at least half: the same texture, its size reported.
	if err := p.Assign(layerFrame(image.Pt(60, 30))); err != nil {
		t.Fatal(err)
	}
	if p.Count() != 1 || p.Size[1] != image.Pt(100, 50) {
		t.Fatalf("60×30 layer: %d textures, size %v", p.Count(), p.Size[1])
	}
	// Less than half: a new texture of its size; the larger one stays.
	if err := p.Assign(layerFrame(image.Pt(40, 20))); err != nil {
		t.Fatal(err)
	}
	if p.Count() != 2 || p.Size[1] != image.Pt(40, 20) {
		t.Fatalf("40×20 layer: %d textures, size %v", p.Count(), p.Size[1])
	}
	// Two layers in sequence share a texture (the first is free after its
	// composite); nested layers (both busy) do not.
	p2 := newFakePool()
	if err := p2.Assign(layerFrame(image.Pt(64, 64), image.Pt(64, 64))); err != nil {
		t.Fatal(err)
	}
	if p2.Count() != 1 || p2.Tex[1] != p2.Tex[2] {
		t.Fatalf("sequential layers: %d textures, %d and %d", p2.Count(), p2.Tex[1], p2.Tex[2])
	}
	nested := &gpu.Frame{
		Size:    image.Pt(200, 200),
		Layers:  []gpu.LayerDesc{{Size: image.Pt(64, 64), Bounds: image.Rect(0, 0, 64, 64)}, {Size: image.Pt(64, 64), Bounds: image.Rect(0, 0, 64, 64)}},
		Batches: []gpu.Batch{{Target: 1}, {Target: 2}, {Target: 1, Layer: 2}, {Layer: 1}},
	}
	if err := p2.Assign(nested); err != nil {
		t.Fatal(err)
	}
	if p2.Count() != 2 || p2.Tex[1] == p2.Tex[2] {
		t.Fatalf("nested layers: %d textures, %d and %d", p2.Count(), p2.Tex[1], p2.Tex[2])
	}
	for _, l := range p2.texs {
		if l.Busy {
			t.Fatal("a texture is still busy after the frame")
		}
	}
}

func TestLayerPoolIdleAndBudget(t *testing.T) {
	p := newFakePool()
	p.Assign(layerFrame(image.Pt(32, 32)))
	for range LayerIdleFrames {
		p.Assign(layerFrame())
	}
	if p.Count() != 1 {
		t.Fatalf("after %d idle frames: %d textures, want 1", LayerIdleFrames, p.Count())
	}
	p.Assign(layerFrame())
	if p.Count() != 0 || len(p.live) != 0 {
		t.Fatalf("after %d idle frames: %d textures (%d live), want none", LayerIdleFrames+1, p.Count(), len(p.live))
	}

	// 64 MiB each; neither fits the other's layer.
	a, b, c := image.Pt(4096, 4096), image.Pt(2000, 8000), image.Pt(8000, 2000)
	p.Assign(layerFrame(a))
	p.Assign(layerFrame(b))
	if p.Count() != 2 {
		t.Fatalf("two 64 MiB textures: %d", p.Count())
	}
	p.Assign(layerFrame(c)) // 192 MiB would exceed the budget: the oldest goes
	if p.Count() != 2 || p.Bytes() > LayerPoolBytes {
		t.Fatalf("after a third: %d textures, %d bytes", p.Count(), p.Bytes())
	}
	for _, l := range p.texs {
		if l.Size == a {
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
	if err := q.Assign(big); err != nil {
		t.Fatal(err)
	}
	if q.Tex[1] == q.Tex[2] || q.Tex[2] == q.Tex[3] || q.Tex[1] == q.Tex[3] {
		t.Fatal("simultaneously busy layers share a texture")
	}
	q.Assign(layerFrame()) // trimmed to the budget when the next frame starts
	if q.Bytes() > LayerPoolBytes {
		t.Fatalf("pool keeps %d bytes after the frame", q.Bytes())
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
		err := p.Assign(f)
		if err == nil || IsAllocError(err) {
			t.Errorf("%s: %v, want an invalid-frame error", name, err)
		}
		for _, l := range p.texs {
			if l.Busy {
				t.Errorf("%s: a texture stays busy", name)
			}
		}
	}
	p := newFakePool()
	p.failNew = true
	if err := p.Assign(layerFrame(sz)); !IsAllocError(err) {
		t.Errorf("creation failure: %v, want an AllocError", err)
	}
	p.failNew = false
	if err := p.Assign(layerFrame(sz)); err != nil || p.Count() != 1 {
		t.Errorf("after a creation failure: %v, %d textures", err, p.Count())
	}
	p.ReleaseAll()
	if len(p.live) != 0 || p.Count() != 0 {
		t.Errorf("releaseAll left %d textures", len(p.live))
	}
}

func TestRescaleComposites(t *testing.T) {
	comp := shaders.Instance{UV: sl.V4(0, 0, 0.5, 1), Kind: sl.V4(shaders.KindPatch, 0.5/64, 0.5/32, 1)}
	patch := shaders.Instance{UV: sl.V4(0, 0, 0.5, 1), Kind: sl.V4(shaders.KindPatch, 0.1, 0.1, 0)}
	ins := []shaders.Instance{comp, patch}
	RescaleComposites(ins, image.Pt(64, 32), image.Pt(128, 128))
	if ins[0].UV != sl.V4(0, 0, 0.25, 0.25) || ins[0].Kind.Y != 0.5/128 || ins[0].Kind.Z != 0.5/128 {
		t.Errorf("composite: UV %v, half-texel %v %v", ins[0].UV, ins[0].Kind.Y, ins[0].Kind.Z)
	}
	if ins[1] != patch {
		t.Error("a patch-page instance was rescaled")
	}
	same := []shaders.Instance{comp}
	RescaleComposites(same, image.Pt(64, 32), image.Pt(64, 32))
	if same[0] != comp {
		t.Error("rescaled to its own size")
	}
}

func TestCheckPartial(t *testing.T) {
	size := image.Pt(100, 50)
	ok := &gpu.Frame{Size: size, Dirty: []image.Rectangle{image.Rect(0, 0, 10, 10), image.Rect(10, 0, 20, 10)},
		Copies: []gpu.CopyRegion{{Src: image.Rect(0, 10, 100, 50), Dst: image.Rect(0, 0, 100, 40)}}}
	if err := CheckPartial(ok); err != nil {
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
		if err := CheckPartial(f); err == nil {
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
		used, create, size, err := ScratchPlan(c.copies, c.have, c.exists, lim)
		if got := (res{used, create, size}); err != nil || got != c.want {
			t.Errorf("%s: %+v, %v; want %+v", c.name, got, err, c.want)
		}
	}
	// Sources stacked taller than the limit: ErrNotRetained (drawn in
	// full), never ErrTargetTooLarge or a failure.
	for _, l := range []int{lim, 8192} {
		_, _, _, err := ScratchPlan([]gpu.CopyRegion{cp(100, l/2), cp(100, l/2+1)}, image.Point{}, false, l)
		if !errors.Is(err, gpu.ErrNotRetained) || errors.Is(err, gpu.ErrTargetTooLarge) {
			t.Errorf("limit %d: %v, want ErrNotRetained", l, err)
		}
	}
}

func TestClearInstances(t *testing.T) {
	dirty := []image.Rectangle{image.Rect(10, 20, 30, 25)}
	ins := ClearInstances(nil, dirty, color.NRGBA{255, 255, 255, 255})
	if len(ins) != 1 {
		t.Fatalf("%d quads", len(ins))
	}
	if ins[0].Rect != sl.V4(9, 19, 22, 7) || ins[0].Color != sl.V4(1, 1, 1, 1) || ins[0].Kind.X != shaders.KindSolid {
		t.Errorf("quad %+v", ins[0])
	}
}
