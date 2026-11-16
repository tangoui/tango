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
