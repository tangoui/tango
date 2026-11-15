package terminal

import (
	"container/list"

	"github.com/tangoui/tango/ui/terminal/vt"
)

// rowCacheKey covers every input that can change a row's static display plan.
// Dynamic cursor/IME overlays are deliberately painted outside this cache.
type rowCacheKey struct {
	id, version uint64
	cols        int
	metrics     metrics
	scheme      Scheme
	selLo       int
	selHi       int
}

type rowSegment struct {
	x, n     int
	idx      uint16
	attr     vt.Attr
	runes    []rune
	single   bool
	selected bool
}

type rowPlan struct {
	segments []rowSegment
}

type rowCacheEntry struct {
	key   rowCacheKey
	plan  *rowPlan
	bytes int
}

type rowCache struct {
	maxEntries int
	maxBytes   int
	bytes      int
	ll         *list.List
	items      map[rowCacheKey]*list.Element
	hits       uint64
	misses     uint64
	evictions  uint64
}

func newRowCache(maxEntries, maxBytes int) *rowCache {
	return &rowCache{
		maxEntries: max(maxEntries, 1),
		maxBytes:   max(maxBytes, 1),
		ll:         list.New(),
		items:      make(map[rowCacheKey]*list.Element),
	}
}

func (c *rowCache) get(key rowCacheKey) (*rowPlan, bool) {
	if e, ok := c.items[key]; ok {
		c.ll.MoveToFront(e)
		c.hits++
		return e.Value.(*rowCacheEntry).plan, true
	}
	c.misses++
	return nil, false
}

func (c *rowCache) put(key rowCacheKey, plan *rowPlan, bytes int) {
	if bytes > c.maxBytes {
		return
	}
	if e, ok := c.items[key]; ok {
		entry := e.Value.(*rowCacheEntry)
		c.bytes -= entry.bytes
		entry.plan, entry.bytes = plan, bytes
		c.bytes += bytes
		c.ll.MoveToFront(e)
	} else {
		e := c.ll.PushFront(&rowCacheEntry{key: key, plan: plan, bytes: bytes})
		c.items[key] = e
		c.bytes += bytes
	}
	for c.ll.Len() > c.maxEntries || c.bytes > c.maxBytes {
		e := c.ll.Back()
		if e == nil {
			break
		}
		entry := e.Value.(*rowCacheEntry)
		delete(c.items, entry.key)
		c.bytes -= entry.bytes
		c.ll.Remove(e)
		c.evictions++
	}
}

func (c *rowCache) len() int { return c.ll.Len() }
