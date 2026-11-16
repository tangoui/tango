package ui

// Nodes and widgets need independent lifetimes. A retained element of a typed
// allocation block keeps every other element and its references alive. With
// cross-frame node reuse, those references can pin successive offscreen trees.
// Reuse by widget ID still avoids allocations for stable nodes; abandoned
// objects remain valid for callers that retain their widget pointers.
func (c *Context) allocNode() *node   { return new(node) }
func (c *Context) allocLabel() *Label { return new(Label) }
func (c *Context) allocBox() *Box     { return new(Box) }
