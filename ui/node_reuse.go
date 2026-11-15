package ui

// Stable node index for retained immediate-mode rebuilds. IDs are already the
// widget identity contract; reusing by ID preserves interaction geometry while
// each frame still resets all per-frame fields and child links.
func (c *Context) beginNodeReuse() {
	if c.nodeIndex == nil {
		c.nodeIndex = map[ID]*node{}
		c.nextNodeIndex = map[ID]*node{}
		return
	}
	for k := range c.nodeIndex {
		delete(c.nodeIndex, k)
	}
	c.nodeIndex, c.nextNodeIndex = c.nextNodeIndex, c.nodeIndex
}
func (c *Context) finishNodeReuse() {}
func (c *Context) takeNode(id ID) *node {
	if n := c.nodeIndex[id]; n != nil {
		delete(c.nodeIndex, id)
		c.nextNodeIndex[id] = n
		return n
	}
	n := c.allocNode()
	c.nextNodeIndex[id] = n
	return n
}
func resetNode(n *node) {
	box, label := n.boxWidget, n.labelWidget
	*n = node{boxWidget: box, labelWidget: label}
}
