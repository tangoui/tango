//go:build darwin

package metal

import (
	"image"
	"testing"
)

func candidate(w, h int) imageCandidate { return imageCandidate{size: image.Pt(w, h)} }

func TestImagePoolBudgetEvictsIdleCandidates(t *testing.T) {
	var p imageTexturePool
	p.SetBudgetForTest(2*16*16*4, 2, 100)
	p.put(candidate(16, 16))
	p.put(candidate(16, 16))
	p.put(candidate(16, 16))
	if p.n != 2 || p.bytes > 2*16*16*4 {
		t.Fatalf("n=%d bytes=%d", p.n, p.bytes)
	}
	p.BeginFrame(1)
	p.put(candidate(16, 16))
	p.EndFrame()
	p.BeginFrame(101)
	if p.n > 2 || p.bytes > 2*16*16*4 {
		t.Fatalf("budget exceeded after idle eviction: n=%d bytes=%d", p.n, p.bytes)
	}
}

func TestImagePoolTakeProtectsActiveSize(t *testing.T) {
	var p imageTexturePool
	p.SetBudgetForTest(16*16*4, 1, 100)
	p.put(candidate(16, 16))
	p.put(candidate(8, 8))
	if got := p.take(image.Pt(16, 16)); got != 0 {
		t.Fatalf("fake texture=%v", got)
	}
	p.BeginFrame(101)
	if p.n != 0 || p.bytes != 0 {
		t.Fatalf("pool retained detached textures: n=%d bytes=%d", p.n, p.bytes)
	}
}

func TestImagePoolResetBudget(t *testing.T) {
	var p imageTexturePool
	p.SetBudgetForTest(16*16*4, 1, 0)
	p.put(candidate(16, 16))
	p.BeginFrame(1)
	p.EndFrame()
	p.clear()
	p.put(candidate(8, 8))
	if p.n != 1 || p.bytes != 8*8*4 {
		t.Fatalf("after reset n=%d bytes=%d", p.n, p.bytes)
	}
}
