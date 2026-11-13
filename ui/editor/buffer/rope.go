package buffer

import (
	"strings"
	"unicode/utf8"
)

// Leaf size policy. Leaves are built at about leafTarget bytes, never exceed
// leafMax, and edits keep them at least leafMin bytes (except when the whole
// document is smaller) so the tree does not fragment over long sessions.
const (
	leafTarget = 1024
	leafMax    = 2048
	leafMin    = 512
)

// node is one node of a persistent, height-balanced (AVL) binary rope.
// Nodes are never mutated after construction, so subtrees are shared freely
// between snapshots and may be read from any goroutine.
//
// A leaf has no children and holds a non-empty chunk of text that starts on a
// rune boundary. Internal nodes always have two children and cache the total
// byte and newline counts of their subtree.
type node struct {
	left, right *node
	text        string // leaf only
	bytes       int    // total bytes in subtree
	lines       int    // number of '\n' in subtree
	height      int32  // 0 for leaves
}

func (n *node) isLeaf() bool { return n.left == nil }

func newLeaf(text string) *node {
	return &node{text: text, bytes: len(text), lines: strings.Count(text, "\n")}
}

func concat2(l, r *node) *node {
	return &node{
		left: l, right: r,
		bytes:  l.bytes + r.bytes,
		lines:  l.lines + r.lines,
		height: max(l.height, r.height) + 1,
	}
}

// balance builds a node from l and r whose heights differ by at most 2,
// rotating as needed to restore the AVL property.
func balance(l, r *node) *node {
	switch {
	case l.height > r.height+1:
		if l.left.height >= l.right.height {
			return concat2(l.left, concat2(l.right, r))
		}
		lr := l.right
		return concat2(concat2(l.left, lr.left), concat2(lr.right, r))
	case r.height > l.height+1:
		if r.right.height >= r.left.height {
			return concat2(concat2(l, r.left), r.right)
		}
		rl := r.left
		return concat2(concat2(l, rl.left), concat2(rl.right, r.right))
	}
	return concat2(l, r)
}

// join concatenates two ropes (either may be nil) in O(|h(l) - h(r)|).
func join(l, r *node) *node {
	if l == nil {
		return r
	}
	if r == nil {
		return l
	}
	switch {
	case l.height > r.height+1:
		return balance(l.left, join(l.right, r))
	case r.height > l.height+1:
		return balance(join(l, r.left), r.right)
	}
	return concat2(l, r)
}

// prefix returns the leaves lying wholly before off. off must be a leaf
// boundary. Cost is O(log n) because join costs telescope along the path.
func prefix(n *node, off int) *node {
	if n == nil || off <= 0 {
		return nil
	}
	if off >= n.bytes {
		return n
	}
	if off <= n.left.bytes {
		return prefix(n.left, off)
	}
	return join(n.left, prefix(n.right, off-n.left.bytes))
}

// suffix returns the leaves lying wholly at or after off. off must be a leaf
// boundary.
func suffix(n *node, off int) *node {
	if n == nil || off >= n.bytes {
		return nil
	}
	if off <= 0 {
		return n
	}
	if off >= n.left.bytes {
		return suffix(n.right, off-n.left.bytes)
	}
	return join(suffix(n.left, off), n.right)
}

// leafAt returns the leaf containing byte off and the leaf's start offset.
// For off == n.bytes it returns the last leaf.
func leafAt(n *node, off int) (*node, int) {
	start := 0
	for !n.isLeaf() {
		if off < n.left.bytes {
			n = n.left
		} else {
			off -= n.left.bytes
			start += n.left.bytes
			n = n.right
		}
	}
	return n, start
}

// chunkBounds returns the end offsets of the pieces text should be split into
// so that each piece is at most leafMax bytes and roughly leafTarget bytes
// long. Cuts are moved forward to rune boundaries.
func chunkBounds(text string) []int {
	n := len(text)
	if n == 0 {
		return nil
	}
	if n <= leafMax {
		return []int{n}
	}
	k := (n + leafTarget - 1) / leafTarget
	ends := make([]int, 0, k)
	prev := 0
	for i := 1; i < k; i++ {
		cut := n * i / k
		// Advance to the next rune start (at most utf8.UTFMax-1 bytes).
		for j := 0; j < utf8.UTFMax-1 && cut < n && !utf8.RuneStart(text[cut]); j++ {
			cut++
		}
		if cut > prev && cut < n {
			ends = append(ends, cut)
			prev = cut
		}
	}
	return append(ends, n)
}

// build returns a balanced rope holding text, split into leaves of about
// leafTarget bytes. Nodes are slab-allocated to keep loading 100 MB cheap.
func build(text string) *node {
	ends := chunkBounds(text)
	if len(ends) == 0 {
		return nil
	}
	leaves := make([]node, len(ends))
	ptrs := make([]*node, len(ends))
	start := 0
	for i, end := range ends {
		t := text[start:end]
		leaves[i] = node{text: t, bytes: len(t), lines: strings.Count(t, "\n")}
		ptrs[i] = &leaves[i]
		start = end
	}
	if len(ptrs) == 1 {
		return ptrs[0]
	}
	inner := make([]node, 0, len(ptrs)-1)
	var rec func(p []*node) *node
	rec = func(p []*node) *node {
		if len(p) == 1 {
			return p[0]
		}
		mid := len(p) / 2
		l, r := rec(p[:mid]), rec(p[mid:])
		inner = append(inner, node{
			left: l, right: r,
			bytes:  l.bytes + r.bytes,
			lines:  l.lines + r.lines,
			height: max(l.height, r.height) + 1,
		})
		return &inner[len(inner)-1]
	}
	return rec(ptrs)
}

// replace returns a rope equal to n with bytes [start, end) replaced by text.
// Only the leaves touching the edit are rewritten; the rest is shared.
func replace(n *node, start, end int, text string) *node {
	if n == nil {
		return build(text)
	}
	l1, lo := leafAt(n, start)
	l2, a2 := leafAt(n, end)
	hi := a2 + l2.bytes
	content := l1.text[:start-lo] + text + l2.text[end-a2:]

	// Fast path: the edit stays inside one leaf and the leaf stays in range.
	if l1 == l2 && len(content) >= leafMin && len(content) <= leafMax {
		return replaceLeaf(n, start, newLeaf(content))
	}

	// Absorb a neighbour so no undersized leaf is left behind.
	if len(content) < leafMin {
		if lo > 0 {
			prev, pstart := leafAt(n, lo-1)
			content = prev.text + content
			lo = pstart
		} else if hi < n.bytes {
			next, _ := leafAt(n, hi)
			content += next.text
			hi += next.bytes
		}
	}
	return join(join(prefix(n, lo), build(content)), suffix(n, hi))
}

// replaceLeaf path-copies n, swapping the leaf that leafAt(n, off) finds for
// leaf. The tree shape is unchanged so no rebalancing is needed.
func replaceLeaf(n *node, off int, leaf *node) *node {
	if n.isLeaf() {
		return leaf
	}
	if off < n.left.bytes {
		return concat2(replaceLeaf(n.left, off, leaf), n.right)
	}
	return concat2(n.left, replaceLeaf(n.right, off-n.left.bytes, leaf))
}

// newlinesBefore returns the number of '\n' bytes in [0, off).
func newlinesBefore(n *node, off int) int {
	lines := 0
	for !n.isLeaf() {
		if off < n.left.bytes {
			n = n.left
		} else {
			off -= n.left.bytes
			lines += n.left.lines
			n = n.right
		}
	}
	return lines + strings.Count(n.text[:off], "\n")
}

// afterNewline returns the offset just past the k-th '\n' (k >= 1).
func afterNewline(n *node, k int) int {
	off := 0
	for !n.isLeaf() {
		if k <= n.left.lines {
			n = n.left
		} else {
			k -= n.left.lines
			off += n.left.bytes
			n = n.right
		}
	}
	t := n.text
	for {
		i := strings.IndexByte(t, '\n')
		k--
		if k == 0 {
			return off + i + 1
		}
		off += i + 1
		t = t[i+1:]
	}
}

// slice returns bytes [start, end) of n. Ranges inside one leaf are returned
// without copying.
func slice(n *node, start, end int) string {
	if start >= end {
		return ""
	}
	for !n.isLeaf() {
		switch {
		case end <= n.left.bytes:
			n = n.left
		case start >= n.left.bytes:
			start -= n.left.bytes
			end -= n.left.bytes
			n = n.right
		default:
			var b strings.Builder
			b.Grow(end - start)
			appendRange(&b, n, start, end, "")
			return b.String()
		}
	}
	return n.text[start:end]
}

// appendRange writes bytes [start, end) of n to b. When eol is non-empty each
// '\n' is written as eol instead.
func appendRange(b *strings.Builder, n *node, start, end int, eol string) {
	if start >= end {
		return
	}
	if n.isLeaf() {
		t := n.text[start:end]
		if eol == "" {
			b.WriteString(t)
			return
		}
		for {
			i := strings.IndexByte(t, '\n')
			if i < 0 {
				b.WriteString(t)
				return
			}
			b.WriteString(t[:i])
			b.WriteString(eol)
			t = t[i+1:]
		}
	}
	lb := n.left.bytes
	if start < lb {
		appendRange(b, n.left, start, min(end, lb), eol)
	}
	if end > lb {
		appendRange(b, n.right, max(start-lb, 0), end-lb, eol)
	}
}
