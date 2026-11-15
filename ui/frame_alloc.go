package ui

// Each build owns its allocations. Small typed blocks amortize allocator and
// GC object overhead without reusing any object from an older build: callers
// retaining a widget pointer still observe that exact, unchanged object.
// Only the current block is retained here; trees and widget references keep
// earlier blocks alive for as long as needed. Reset drops the allocator's
// references rather than clearing/recycling memory.
const frameBlockSize = 16

type frameBlock[T any] struct {
	block *[frameBlockSize]T
	used  int
}

func (b *frameBlock[T]) alloc() *T {
	if b.block == nil || b.used == frameBlockSize {
		b.block = new([frameBlockSize]T)
		b.used = 0
	}
	p := &b.block[b.used]
	b.used++
	return p
}
func (b *frameBlock[T]) reset() { b.block = nil; b.used = 0 }

type frameAllocators struct {
	nodes  frameBlock[node]
	labels frameBlock[Label]
	boxes  frameBlock[Box]
}

func (c *Context) resetFrameAllocations() {
	c.allocs.nodes.reset()
	c.allocs.labels.reset()
	c.allocs.boxes.reset()
}
func (c *Context) allocNode() *node   { return c.allocs.nodes.alloc() }
func (c *Context) allocLabel() *Label { return c.allocs.labels.alloc() }
func (c *Context) allocBox() *Box     { return c.allocs.boxes.alloc() }
