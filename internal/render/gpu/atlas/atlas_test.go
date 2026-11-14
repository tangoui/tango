package atlas_test

import (
	"errors"
	"image"
	"testing"

	"github.com/tangoui/tango/internal/render/gpu/atlas"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

// pattern returns a w×h mask whose texels depend on seed, so a mask
// overwritten by another entry is detected.
func pattern(w, h, seed int) *image.Alpha {
	m := image.NewAlpha(image.Rect(0, 0, w, h))
	for i := range m.Pix {
		m.Pix[i] = uint8(1 + (i*7+seed*13)%254)
	}
	return m
}

// same reports whether the atlas texels of e equal mask.
func same(a *atlas.Atlas, e atlas.Entry, mask *image.Alpha) bool {
	px := a.Pixels()
	if e.Rect.Size() != mask.Rect.Size() || !e.Rect.In(px.Rect) {
		return false
	}
	for y := 0; y < e.Rect.Dy(); y++ {
		for x := 0; x < e.Rect.Dx(); x++ {
			if px.AlphaAt(e.Rect.Min.X+x, e.Rect.Min.Y+y) != mask.AlphaAt(mask.Rect.Min.X+x, mask.Rect.Min.Y+y) {
				return false
			}
		}
	}
	return true
}

type src struct{}

func (src) GlyphPath(uint32, float32) *paint.Path { return nil }

func key(id int) atlas.Key { return atlas.Key{Src: src{}, ID: uint32(id), Size: 64} }

func TestAtlasInsertLookup(t *testing.T) {
	a := atlas.New(256, 2)
	if a.Pixels().Rect != image.Rect(0, 0, 256, 256) {
		t.Fatalf("pixels %v", a.Pixels().Rect)
	}
	if _, ok := a.Lookup(key(1)); ok {
		t.Fatal("lookup of a missing key succeeded")
	}
	// A mask whose Rect does not start at the origin.
	m1 := pattern(10, 12, 1).SubImage(image.Rect(2, 3, 9, 12)).(*image.Alpha)
	m2 := pattern(20, 5, 2)
	e1, err := a.Insert(key(1), m1, image.Point{})
	if err != nil {
		t.Fatal(err)
	}
	e2, err := a.Insert(key(2), m2, image.Point{})
	if err != nil {
		t.Fatal(err)
	}
	if e1.Rect.Overlaps(e2.Rect) {
		t.Fatalf("entries overlap: %v %v", e1.Rect, e2.Rect)
	}
	if !same(a, e1, m1) || !same(a, e2, m2) {
		t.Fatal("atlas texels differ from the inserted masks")
	}
	for _, k := range []int{1, 2} {
		got, ok := a.Lookup(key(k))
		want := map[int]atlas.Entry{1: e1, 2: e2}[k]
		if !ok || got != want {
			t.Fatalf("lookup %d = %v %v, want %v", k, got, ok, want)
		}
	}
	// A different sub-pixel bucket is a different key.
	k := key(1)
	k.Sub = 2
	if _, ok := a.Lookup(k); ok {
		t.Fatal("sub-pixel bucket ignored")
	}
	// Re-inserting a present key returns its entry unchanged.
	if e, err := a.Insert(key(1), m1, image.Point{}); err != nil || e != e1 {
		t.Fatalf("re-insert = %v %v", e, err)
	}
	d := a.TakeDirty()
	cover := image.Rectangle{}
	for _, r := range d {
		cover = cover.Union(r)
	}
	if len(d) == 0 || !e1.Rect.In(cover) || !e2.Rect.In(cover) {
		t.Fatalf("dirty %v does not cover %v %v", d, e1.Rect, e2.Rect)
	}
	if d := a.TakeDirty(); len(d) != 0 {
		t.Fatalf("second TakeDirty = %v", d)
	}
	if _, err := a.Insert(key(3), pattern(257, 4, 3), image.Point{}); !errors.Is(err, atlas.ErrTooLarge) {
		t.Fatalf("oversized mask: err %v", err)
	}
}

// cjkMasks returns n distinct masks of 64px CJK glyphs rasterized as
// paint.CPU draws them, or synthetic 64px-sized masks without system fonts.
func cjkMasks(t *testing.T, n int) ([]atlas.Key, []*image.Alpha) {
	var keys []atlas.Key
	var masks []*image.Alpha
	if len(text.Families()) > 0 {
		cpu := paint.NewCPU()
		var s []rune
		for r := rune(0x4E00); len(s) < 3*n; r++ {
			s = append(s, r)
		}
		seen := map[atlas.Key]bool{}
		for _, g := range text.Shape(string(s), text.Style{Size: 64}, text.Options{NoWrap: true}).Glyphs {
			k := atlas.Key{Src: g.Src, ID: g.ID, Size: g.Size, Skew: g.Skew}
			m, _ := cpu.GlyphMask(g, g.Size, 0)
			if g.Src == nil || seen[k] || m == nil || m.Rect.Dx() < 40 {
				continue
			}
			seen[k] = true
			keys, masks = append(keys, k), append(masks, m)
			if len(keys) == n {
				return keys, masks
			}
		}
		t.Logf("only %d CJK glyphs from system fonts; using synthetic masks", len(keys))
	}
	keys, masks = keys[:0], masks[:0]
	for i := range n {
		keys = append(keys, key(i))
		masks = append(masks, pattern(60+i%7, 62+i%5, i))
	}
	return keys, masks
}

func TestAtlasOverflow(t *testing.T) {
	const page, pages = 512, 5
	keys, masks := cjkMasks(t, 320)
	a := atlas.New(page, pages)
	a.BeginFrame()
	entries := map[int]atlas.Entry{}
	// 200 64px glyphs do not fit on one 512px page: new pages open and
	// every glyph stays findable.
	for i := range 200 {
		e, err := a.Insert(keys[i], masks[i], image.Point{})
		if err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
		entries[i] = e
	}
	used := map[int]bool{}
	for i := range 200 {
		e, ok := a.Lookup(keys[i])
		if !ok || e != entries[i] || !same(a, e, masks[i]) {
			t.Fatalf("glyph %d lost after filling (ok %v)", i, ok)
		}
		used[e.Page] = true
		if e.Page != e.Rect.Min.Y/page || !e.Rect.In(image.Rect(0, page*e.Page, page, page*(e.Page+1))) {
			t.Fatalf("entry %v not inside page %d", e.Rect, e.Page)
		}
	}
	if len(used) < 4 {
		t.Fatalf("200 glyphs on %d pages", len(used))
	}
	if a.Pixels().Rect.Dx() != page || a.Pixels().Rect.Dy() != page*len(used) {
		t.Fatalf("pixels %v for %d pages", a.Pixels().Rect, len(used))
	}
	a.TakeDirty()

	// Next frames: only glyphs 0..29 (the first page) are drawn, then new
	// glyphs keep arriving until an old page is evicted.
	evicted := false
	for f := 0; f < 40 && !evicted; f++ {
		a.BeginFrame()
		for i := range 30 {
			if _, ok := a.Lookup(keys[i]); !ok {
				t.Fatalf("frame %d: glyph %d of this frame missing", f, i)
			}
		}
		for i := 200 + 3*f; i < 200+3*f+3; i++ {
			e, err := a.Insert(keys[i], masks[i], image.Point{})
			if err != nil {
				t.Fatalf("insert %d: %v", i, err)
			}
			entries[i] = e
		}
		for i := 30; i < 200; i++ {
			if _, ok := a.Lookup(keys[i]); !ok {
				evicted = true
			}
		}
		// Lookup marks pages used; undo by starting the next frame.
		if evicted {
			// The glyphs used this frame (and the ones just inserted) are
			// intact.
			for i := range 30 {
				e, ok := a.Lookup(keys[i])
				if !ok || !same(a, e, masks[i]) {
					t.Fatalf("frame %d: glyph %d used this frame damaged by eviction", f, i)
				}
			}
			for i := 200; i < 200+3*f+3; i++ {
				e, ok := a.Lookup(keys[i])
				if !ok || e != entries[i] || !same(a, e, masks[i]) {
					t.Fatalf("frame %d: new glyph %d damaged", f, i)
				}
			}
		}
		// Every glyph still found has its own texels.
		for i := range 200 + 3*f + 3 {
			if e, ok := a.Lookup(keys[i]); ok && !same(a, e, masks[i]) {
				t.Fatalf("frame %d: glyph %d maps to foreign texels", f, i)
			}
		}
	}
	if !evicted {
		t.Fatal("no page was ever evicted")
	}
	if a.Pixels().Rect.Dy() > page*pages {
		t.Fatalf("pixels %v exceed %d pages", a.Pixels().Rect, pages)
	}

	// When every page was used this frame nothing can be evicted.
	b := atlas.New(128, 1)
	b.BeginFrame()
	var err error
	for i := 0; err == nil && i < 100; i++ {
		_, err = b.Insert(key(i), pattern(60, 60, i), image.Point{})
	}
	if !errors.Is(err, atlas.ErrFull) {
		t.Fatalf("full atlas: err %v", err)
	}
	if _, ok := b.Lookup(key(0)); !ok {
		t.Fatal("glyph of this frame evicted")
	}
	b.BeginFrame()
	if _, err := b.Insert(key(1000), pattern(60, 60, 1000), image.Point{}); err != nil {
		t.Fatalf("next frame insert: %v", err)
	}
	if _, ok := b.Lookup(key(0)); ok {
		t.Fatal("stale page not evicted")
	}
}

func TestAtlasReset(t *testing.T) {
	a := atlas.New(128, 4)
	for i := range 30 {
		if _, err := a.Insert(key(i), pattern(40, 40, i), image.Point{}); err != nil {
			t.Fatal(err)
		}
	}
	if a.Pixels().Rect.Dy() <= 128 {
		t.Fatalf("expected several pages, pixels %v", a.Pixels().Rect)
	}
	a.Reset()
	for i := range 30 {
		if _, ok := a.Lookup(key(i)); ok {
			t.Fatalf("glyph %d survived Reset", i)
		}
	}
	if d := a.TakeDirty(); len(d) != 0 {
		t.Fatalf("dirty after Reset: %v", d)
	}
	if a.Pixels().Rect != image.Rect(0, 0, 128, 128) {
		t.Fatalf("pixels after Reset %v", a.Pixels().Rect)
	}
	e, err := a.Insert(key(7), pattern(40, 40, 99), image.Point{})
	if err != nil || e.Page != 0 || !same(a, e, pattern(40, 40, 99)) {
		t.Fatalf("insert after Reset: %v %v", e, err)
	}
}

// TestAtlasLRU: with two full pages, a new glyph evicts the page used
// least recently. A (page 0) is used in frame 1 and B (page 1) in frame 2,
// so a new glyph in frame 3 evicts A's page and B stays; then B is the
// older one and goes next. Evictions counts the pages emptied.
func TestAtlasLRU(t *testing.T) {
	const page = 64
	a := atlas.New(page, 2)
	full := func(seed int) *image.Alpha { return pattern(page, page, seed) } // one per page
	insert := func(id int) atlas.Entry {
		t.Helper()
		e, err := a.Insert(key(id), full(id), image.Point{})
		if err != nil {
			t.Fatalf("insert %d: %v", id, err)
		}
		return e
	}
	a.BeginFrame()
	if e := insert(1); e.Page != 0 { // A
		t.Fatalf("A on page %d", e.Page)
	}
	a.BeginFrame()
	if e := insert(2); e.Page != 1 { // B
		t.Fatalf("B on page %d", e.Page)
	}
	if n := a.Evictions(); n != 0 {
		t.Fatalf("%d evictions before the atlas was full", n)
	}
	a.BeginFrame()
	if e := insert(3); e.Page != 0 {
		t.Fatalf("C on page %d, want A's page 0", e.Page)
	}
	if _, ok := a.Lookup(key(1)); ok {
		t.Fatal("A (least recently used) still present")
	}
	if e, ok := a.Lookup(key(2)); !ok || !same(a, e, full(2)) {
		t.Fatal("B (used more recently) evicted")
	}
	if n := a.Evictions(); n != 1 {
		t.Fatalf("evictions = %d, want 1", n)
	}
	// Frame 3 used both pages (C inserted, B looked up); frame 4 uses C
	// only, so in frame 5 B's page is the least recently used.
	a.BeginFrame()
	a.Lookup(key(3))
	a.BeginFrame()
	if e := insert(4); e.Page != 1 {
		t.Fatalf("D on page %d, want B's page 1", e.Page)
	}
	if _, ok := a.Lookup(key(2)); ok {
		t.Fatal("B still present")
	}
	if _, ok := a.Lookup(key(3)); !ok {
		t.Fatal("C (used more recently) evicted")
	}
	if n := a.Evictions(); n != 2 {
		t.Fatalf("evictions = %d, want 2", n)
	}
	// Reset drops everything but keeps counting.
	a.Reset()
	if n := a.Evictions(); n != 2 {
		t.Fatalf("evictions after Reset = %d, want 2", n)
	}
}

// TestAtlasEntryOffset: Insert stores the mask's offset (its top-left
// relative to the pen for a glyph, its device position for a path) in the
// entry, so a Lookup hit needs nothing else to place the mask.
func TestAtlasEntryOffset(t *testing.T) {
	a := atlas.New(64, 1)
	off := image.Pt(-3, -11)
	e, err := a.Insert(key(1), pattern(5, 9, 1), off)
	if err != nil {
		t.Fatal(err)
	}
	if e.Off != off {
		t.Fatalf("entry offset %v, want %v", e.Off, off)
	}
	if got, ok := a.Lookup(key(1)); !ok || got != e {
		t.Fatalf("lookup = %v %v, want %v", got, ok, e)
	}
}

// TestAtlasEmptyMask: an empty mask (a space) is remembered: Lookup hits
// with an empty Rect, so the renderer does not rasterize it again. It takes
// no atlas space and is dropped by Reset.
func TestAtlasEmptyMask(t *testing.T) {
	a := atlas.New(64, 1)
	for _, m := range []*image.Alpha{nil, image.NewAlpha(image.Rectangle{})} {
		a.Reset()
		e, err := a.Insert(key(1), m, image.Pt(2, 2))
		if err != nil || !e.Rect.Empty() {
			t.Fatalf("insert empty = %v %v", e, err)
		}
		got, ok := a.Lookup(key(1))
		if !ok || !got.Rect.Empty() {
			t.Fatalf("lookup empty = %v %v; want a hit with an empty Rect", got, ok)
		}
		if d := a.TakeDirty(); len(d) != 0 {
			t.Fatalf("empty mask dirtied %v", d)
		}
		// The whole page is still free.
		if e, err := a.Insert(key(2), pattern(64, 64, 2), image.Point{}); err != nil || e.Page != 0 {
			t.Fatalf("full-page insert after empty = %v %v", e, err)
		}
	}
	a.Reset()
	if _, ok := a.Lookup(key(1)); ok {
		t.Fatal("empty entry survived Reset")
	}
}

// TestAtlasPathKeys: path keys (Kind 1) are told apart by Path pointer and
// device transform, never collide with glyph keys, and live on pages of
// their own, so pages of one-off path masks can be evicted without the
// glyphs used every frame.
func TestAtlasPathKeys(t *testing.T) {
	a := atlas.New(64, 3)
	p1, p2 := paint.NewPath().Rect(geom.R(0, 0, 4, 4)), paint.NewPath().Rect(geom.R(0, 0, 4, 4))
	pk := func(p *paint.Path, s float32) atlas.Key {
		return atlas.Key{Kind: atlas.KindPath, Path: p, M: geom.Scale(s, s), Size: s}
	}
	g, err := a.Insert(key(1), pattern(8, 8, 1), image.Point{})
	if err != nil {
		t.Fatal(err)
	}
	e1, err := a.Insert(pk(p1, 1), pattern(8, 8, 2), image.Pt(5, 5))
	if err != nil {
		t.Fatal(err)
	}
	if e1.Page == g.Page {
		t.Fatalf("path mask shares page %d with a glyph", e1.Page)
	}
	if _, ok := a.Lookup(pk(p2, 1)); ok {
		t.Fatal("another Path with the same content hit p1's entry")
	}
	if _, ok := a.Lookup(pk(p1, 2)); ok {
		t.Fatal("another device transform hit p1's entry")
	}
	if e, ok := a.Lookup(pk(p1, 1)); !ok || e != e1 || !same(a, e, pattern(8, 8, 2)) {
		t.Fatalf("lookup p1 = %v %v", e, ok)
	}
	// More glyphs go to the glyph page, more paths to the path page.
	if e, _ := a.Insert(key(2), pattern(8, 8, 3), image.Point{}); e.Page != g.Page {
		t.Fatalf("second glyph on page %d, want %d", e.Page, g.Page)
	}
	if e, _ := a.Insert(pk(p2, 1), pattern(8, 8, 4), image.Point{}); e.Page != e1.Page {
		t.Fatalf("second path on page %d, want %d", e.Page, e1.Page)
	}
	// A frame that uses only the glyphs: a new path that needs a fresh
	// page evicts the path page, not the glyph page.
	a.BeginFrame()
	a.Lookup(key(1))
	if _, err := a.Insert(pk(paint.NewPath(), 1), pattern(64, 64, 5), image.Point{}); err != nil {
		t.Fatal(err)
	}
	a.BeginFrame()
	a.Lookup(key(1))
	if _, err := a.Insert(pk(paint.NewPath(), 1), pattern(64, 64, 6), image.Point{}); err != nil {
		t.Fatal(err)
	}
	if e, ok := a.Lookup(key(1)); !ok || !same(a, e, pattern(8, 8, 1)) {
		t.Fatal("glyph evicted by path masks")
	}
}

// TestAtlasEmptyMasksBounded: remembering empty masks is bounded: past
// the limit the old ones are forgotten (Lookup misses them, so they are
// rasterized again), the newest are found, and real entries stay.
func TestAtlasEmptyMasksBounded(t *testing.T) {
	a := atlas.New(64, 1)
	e, err := a.Insert(key(-1), pattern(4, 4, 1), image.Point{})
	if err != nil {
		t.Fatal(err)
	}
	const n = 10000
	for i := range n {
		a.Insert(key(i), nil, image.Point{})
	}
	if _, ok := a.Lookup(key(n - 1)); !ok {
		t.Fatal("newest empty mask forgotten")
	}
	if _, ok := a.Lookup(key(0)); ok {
		t.Fatal("empty masks kept without bound")
	}
	if got, ok := a.Lookup(key(-1)); !ok || got != e {
		t.Fatal("real entry dropped with the empty masks")
	}
}

// TestAtlasPathPagesCapped: path masks never take more than maxPages-1
// pages, so per-frame-new Paths cannot pin every page and starve glyphs:
// in a frame where path masks fill every page they may take (all used
// this frame), a further path mask fails with ErrFull while a glyph still
// gets a page; across frames path masks evict their own pages, not the
// glyphs'.
func TestAtlasPathPagesCapped(t *testing.T) {
	pk := func() atlas.Key {
		return atlas.Key{Kind: atlas.KindPath, Path: paint.NewPath(), M: geom.Scale(1, 1), Size: 1}
	}
	for _, pages := range []int{2, 3, 4} {
		a := atlas.New(64, pages)
		a.BeginFrame()
		// Page-sized path masks, all used in this frame.
		for i := 0; i < pages-1; i++ {
			if _, err := a.Insert(pk(), pattern(64, 64, i), image.Point{}); err != nil {
				t.Fatalf("%d pages: path %d: %v", pages, i, err)
			}
		}
		if _, err := a.Insert(pk(), pattern(64, 64, 9), image.Point{}); !errors.Is(err, atlas.ErrFull) {
			t.Fatalf("%d pages: path mask beyond maxPages-1 pages: err %v, want ErrFull", pages, err)
		}
		g, err := a.Insert(key(1), pattern(64, 64, 1), image.Point{})
		if err != nil {
			t.Fatalf("%d pages: glyph starved by path masks: %v", pages, err)
		}
		// Later frames: new path masks reuse path pages; the glyph stays.
		for f := range 5 {
			a.BeginFrame()
			a.Lookup(key(1))
			if _, err := a.Insert(pk(), pattern(64, 64, 20+f), image.Point{}); err != nil {
				t.Fatalf("%d pages: frame %d: path: %v", pages, f, err)
			}
		}
		if e, ok := a.Lookup(key(1)); !ok || e != g || !same(a, e, pattern(64, 64, 1)) {
			t.Fatalf("%d pages: glyph evicted by path masks", pages)
		}
	}
	// One page: it belongs to glyphs; path masks never get it.
	a := atlas.New(64, 1)
	if _, err := a.Insert(pk(), pattern(8, 8, 1), image.Point{}); !errors.Is(err, atlas.ErrFull) {
		t.Fatalf("1 page: path mask err %v, want ErrFull", err)
	}
}
