package atlas

import (
	"image"
	"testing"

	"github.com/tangoui/tango/ui/paint"
)

type testSource struct{}

func (testSource) GlyphPath(uint32, float32) *paint.Path { return nil }

func mask(w, h int) *image.Alpha { return image.NewAlpha(image.Rect(0, 0, w, h)) }
func key(id uint32) Key          { return Key{Kind: KindGlyph, Src: testSource{}, ID: id, Size: 12} }

func TestEvictBudgetKeepsCurrentFramePage(t *testing.T) {
	a := New(8, 8)
	a.SetBudgetForTest(2, 1)
	a.BeginFrame(1)
	for i := uint32(0); i < 3; i++ {
		if _, err := a.Insert(key(i), mask(7, 7), image.Point{}); err != nil {
			t.Fatal(err)
		}
	}
	a.EndFrame()
	// Page 0 was used in frame 1; frame 2 protects a newly looked-up entry.
	a.BeginFrame(2)
	if _, ok := a.Lookup(key(0)); !ok {
		t.Fatal("missing current glyph")
	}
	if got := a.Evict(2); got != 1 {
		t.Fatalf("evicted=%d, want 1", got)
	}
	if _, ok := a.Lookup(key(0)); !ok {
		t.Fatal("current-frame glyph was evicted")
	}
	if a.PageCount() > 2 {
		t.Fatalf("pages=%d, want <=2", a.PageCount())
	}
}

func TestEvictZeroBudgetRejected(t *testing.T) {
	a := New(8, 2)
	if err := a.SetBudget(AtlasBudget{}); err == nil {
		t.Fatal("zero budget must be rejected")
	}
}

func TestEvictedMaskCanBeInsertedAgain(t *testing.T) {
	a := New(8, 2)
	a.SetBudgetForTest(1, 0)
	a.BeginFrame(1)
	k := key(1)
	if _, err := a.Insert(k, mask(7, 7), image.Point{}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Insert(key(2), mask(7, 7), image.Point{}); err != nil {
		t.Fatal(err)
	}
	a.EndFrame()
	a.BeginFrame(2)
	if got := a.Evict(2); got != 1 {
		t.Fatalf("evicted=%d", got)
	}
	if _, ok := a.Lookup(k); !ok {
		t.Fatal("current first key should remain available")
	}
	if _, err := a.Insert(key(2), mask(7, 7), image.Point{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.Lookup(key(2)); !ok {
		t.Fatal("reinserted key missing")
	}
}
