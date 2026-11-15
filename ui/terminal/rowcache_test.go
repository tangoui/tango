package terminal

import (
	"testing"

	"github.com/tangoui/tango/ui/terminal/vt"
)

func TestRowCacheReusesUnchangedRows(t *testing.T) {
	c := newRowCache(8, 1024)
	key := rowCacheKey{id: 7, version: 1, cols: 80, metrics: metrics{cellW: 9, lineH: 16}, scheme: SchemeDark}
	plan := &rowPlan{segments: []rowSegment{{x: 0, n: 1}}}
	c.put(key, plan, 1)
	got, ok := c.get(key)
	if !ok || len(got.segments) != 1 || c.hits != 1 {
		t.Fatalf("get = %v, %v; hits=%d, want cached row", got, ok, c.hits)
	}
}

func TestRowCacheInvalidatesDirtyRowAndSelection(t *testing.T) {
	c := newRowCache(8, 1024)
	base := rowCacheKey{id: 7, version: 1, cols: 80, scheme: SchemeDark}
	c.put(base, nil, 1)
	for _, key := range []rowCacheKey{
		{id: 7, version: 2, cols: 80, scheme: SchemeDark},
		{id: 7, version: 1, cols: 80, scheme: SchemeDark, selLo: 2, selHi: 4},
	} {
		if _, ok := c.get(key); ok {
			t.Fatalf("key %+v unexpectedly hit", key)
		}
	}
}

func TestRowCacheEvictionBounded(t *testing.T) {
	c := newRowCache(2, 10)
	for i := uint64(0); i < 8; i++ {
		c.put(rowCacheKey{id: i, version: 1}, nil, 8)
	}
	if c.len() > 2 || c.bytes > 10 {
		t.Fatalf("cache size = entries:%d bytes:%d, want <=2 and <=10", c.len(), c.bytes)
	}
}

func TestViewRowCacheReusesUnchangedRows(t *testing.T) {
	g := newViewRig(t, 600, 300, nil)
	g.output("stable\r\n", func(v *vt.Terminal) bool { return rowText(v, 0) == "stable" })
	g.d.Frame()
	before := g.vw.vs.rowCache.hits
	g.d.Frame()
	if g.vw.vs.rowCache.hits <= before {
		t.Fatalf("row cache hits = %d before %d, want a hit on unchanged frame", g.vw.vs.rowCache.hits, before)
	}
}
