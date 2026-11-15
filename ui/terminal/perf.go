package terminal

import "sync/atomic"

// perfStats is the test-visible performance counter snapshot. It is kept
// internal so measuring the implementation does not become product API.
type perfStats struct {
	FeedBytes     uint64
	FeedBatches   uint64
	FeedLockWait  uint64 // nanoseconds spent waiting for the VT mutex
	PaintRows     uint64
	CacheHits     uint64
	CacheMisses   uint64
	SnapshotBytes uint64
}

type perfCounters struct {
	feedBytes     atomic.Uint64
	feedBatches   atomic.Uint64
	feedLockWait  atomic.Uint64
	paintRows     atomic.Uint64
	cacheHits     atomic.Uint64
	cacheMisses   atomic.Uint64
	snapshotBytes atomic.Uint64
}

func (p *perfCounters) snapshot() perfStats {
	return perfStats{
		FeedBytes:     p.feedBytes.Load(),
		FeedBatches:   p.feedBatches.Load(),
		FeedLockWait:  p.feedLockWait.Load(),
		PaintRows:     p.paintRows.Load(),
		CacheHits:     p.cacheHits.Load(),
		CacheMisses:   p.cacheMisses.Load(),
		SnapshotBytes: p.snapshotBytes.Load(),
	}
}

func (t *Terminal) perfStats() perfStats { return t.perf.snapshot() }

func (w *ViewW) perfStats() perfStats { return w.t.perfStats() }
