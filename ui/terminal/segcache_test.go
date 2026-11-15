package terminal

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/terminal/vt"
)

func TestSegCacheFitKeepsAFloorAndEvicts(t *testing.T) {
	c := newSegCache(segCacheMin)
	shape := func() *segShape { return &segShape{} }
	for i := range 10 {
		c.get(segKey{s: fmt.Sprint(i)}, shape)
	}
	c.fit(1)
	if c.cap != segCacheMin {
		t.Fatalf("cap = %d, want the floor %d", c.cap, segCacheMin)
	}
	c.fit(4 * segCacheMin)
	if c.cap != 4*segCacheMin {
		t.Fatalf("cap = %d, want %d", c.cap, 4*segCacheMin)
	}
	for i := range 4 * segCacheMin {
		c.get(segKey{s: fmt.Sprint(i)}, shape)
	}
	c.fit(segCacheMin)
	if c.ll.Len() != segCacheMin || len(c.m) != segCacheMin {
		t.Fatalf("after shrinking: %d entries, %d keys, want %d", c.ll.Len(), len(c.m), segCacheMin)
	}
	// The most recently used entries stay.
	misses := c.misses
	c.get(segKey{s: fmt.Sprint(4*segCacheMin - 1)}, shape)
	if c.misses != misses {
		t.Fatal("the newest entry was evicted by fit")
	}
}

// A screen with more distinct segments than the cache floor (ls --color
// in a large window) is repainted without shaping anything again: an
// LRU smaller than the screen misses on every row of every frame.
func TestViewRepaintOfALargeColorfulScreenHitsTheCache(t *testing.T) {
	g := newViewRig(t, 1440, 900, nil)
	var sb strings.Builder
	segs := 0
	for i := range 200 {
		for j := range 12 {
			fmt.Fprintf(&sb, "\x1b[%dmf%d_%d\x1b[0m ", 31+(i+j)%7, i, j)
		}
		sb.WriteString("\r\n")
	}
	g.output(sb.String(), func(v *vt.Terminal) bool {
		_, rows := v.Screen().Size()
		segs = rows * 12
		return rowText(v, rows-2) != ""
	})
	if segs <= segCacheMin {
		t.Fatalf("screen has %d segments, the test needs more than %d", segs, segCacheMin)
	}
	g.d.Frame()
	before := g.vw.vs.cache.misses
	g.vw.vs.needFull = true // repaint every row
	g.d.Frame()
	if got := g.vw.vs.cache.misses - before; got != 0 {
		t.Fatalf("repaint shaped %d segments again, want 0", got)
	}
}
