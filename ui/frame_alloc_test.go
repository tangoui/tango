package ui

import (
	"github.com/tangoui/tango/ui/theme"
	"testing"
)

func TestFrameAllocationsKeepOldObjectsIndependent(t *testing.T) {
	c := newContext(theme.AntDesign())
	oldNode := c.allocNode()
	oldNode.label = "old node"
	oldLabel := c.allocLabel()
	oldLabel.s = "old label"
	oldBox := c.allocBox()
	oldBox.n = oldNode
	c.resetFrameAllocations()
	for range 100 {
		n := c.allocNode()
		n.label = "new node"
		l := c.allocLabel()
		l.s = "new label"
		b := c.allocBox()
		b.n = n
	}
	if oldNode.label != "old node" || oldLabel.s != "old label" || oldBox.n != oldNode {
		t.Fatal("new frame mutated retained objects from an older frame")
	}
}

func TestFrameAllocationBatching(t *testing.T) {
	c := newContext(theme.AntDesign())
	allocations := testing.AllocsPerRun(10, func() {
		c.resetFrameAllocations()
		for range 128 {
			c.allocNode()
			c.allocLabel()
			c.allocBox()
		}
	})
	if allocations > 32 {
		t.Fatalf("128 nodes/labels/boxes allocated %.0f objects, want bulk allocation", allocations)
	}
}
