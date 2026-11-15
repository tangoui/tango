package ui

import (
	"github.com/tangoui/tango/ui/theme"
	"testing"
)

func TestNodeReuseKeepsStableIDsAndReplacesChangedNodes(t *testing.T) {
	c := newContext(theme.AntDesign())
	c.beginNodeReuse()
	root := c.takeNode(1)
	root.id = 1
	child := c.takeNode(2)
	root.kids = []*node{child}
	c.finishNodeReuse()
	c.beginNodeReuse()
	same := c.takeNode(1)
	same.id = 1
	sameChild := c.takeNode(2)
	if same != root || sameChild != child {
		t.Fatal("stable nodes were not reused")
	}
	c.finishNodeReuse()
	c.beginNodeReuse()
	same = c.takeNode(1)
	same.id = 1
	newChild := c.takeNode(3)
	if same != root || newChild == child {
		t.Fatal("changed node was reused under different ID")
	}
	c.finishNodeReuse()
}
