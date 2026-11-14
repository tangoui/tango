package graph

import (
	"math"
	"slices"

	"github.com/tangoui/tango/ui/geom"
)

// LayoutOptions configure Layout.
type LayoutOptions struct {
	Direction Direction
	RankSep   float32 // gap between ranks in graph units (default 64)
	NodeSep   float32 // gap between neighbouring nodes of a rank (default 32)
	// Sweeps is the number of crossing-reduction sweeps (each one down
	// and one up; default 12).
	Sweeps int
	// DefaultSize is used for nodes whose Size is zero (default 140×40).
	DefaultSize geom.Size
}

func (o LayoutOptions) norm() LayoutOptions {
	if o.RankSep <= 0 {
		o.RankSep = 64
	}
	if o.NodeSep <= 0 {
		o.NodeSep = 32
	}
	if o.Sweeps <= 0 {
		o.Sweeps = 12
	}
	if o.DefaultSize.W <= 0 || o.DefaultSize.H <= 0 {
		o.DefaultSize = geom.Size{W: 140, H: 40}
	}
	return o
}

// Layout places every node of g with a layered (Sugiyama-style) layout and
// sets the bend points of edges that span several ranks:
//
//  1. cycles are broken by reversing DFS back edges;
//  2. nodes get the longest-path rank, sources pulled next to their
//     successors;
//  3. long edges are split by virtual nodes, one per crossed rank;
//  4. barycentric down / up sweeps order each rank, keeping the order with
//     the fewest crossings;
//  5. positions across the ranks are refined towards the neighbours'
//     average under the minimum-gap constraints (an exact weighted
//     isotonic fit per rank), which straightens long edges.
//
// The result's top-left is at the origin. Nodes with a zero Size get
// o.DefaultSize. Self loops and edges to missing nodes are ignored.
func Layout(g *Graph, o LayoutOptions) {
	o = o.norm()
	n := len(g.Nodes)
	if n == 0 {
		return
	}
	g.reindex()
	sizes := make([]geom.Size, n)
	for i := range g.Nodes {
		s := g.Nodes[i].Size
		if s.W <= 0 || s.H <= 0 {
			s = o.DefaultSize
			g.Nodes[i].Size = s
		}
		sizes[i] = s
	}
	horiz := o.Direction.horizontal()
	cross := func(s geom.Size) float32 { // extent across the ranks
		if horiz {
			return s.H
		}
		return s.W
	}
	along := func(s geom.Size) float32 { // extent along the flow
		if horiz {
			return s.W
		}
		return s.H
	}

	// Edges as index pairs (edge index kept for bends).
	type link struct{ u, v, e int }
	links := make([]link, 0, len(g.Edges))
	for ei := range g.Edges {
		e := &g.Edges[ei]
		u, ok1 := g.index[e.From]
		v, ok2 := g.index[e.To]
		if !ok1 || !ok2 || u == v {
			continue
		}
		links = append(links, link{u, v, ei})
	}
	l := &layered{n: n}
	ll := make([][2]int, len(links))
	for i, k := range links {
		ll[i] = [2]int{k.u, k.v}
	}
	rev := l.breakCycles(ll)
	for i := range ll {
		if rev[i] {
			ll[i][0], ll[i][1] = ll[i][1], ll[i][0]
		}
	}
	l.rank(ll)

	// Virtual nodes for long edges. Vertex ids >= n are virtual.
	crossSize := make([]float32, n, n+len(ll))
	for i := range n {
		crossSize[i] = cross(sizes[i])
	}
	chains := make([][]int, len(ll)) // virtual vertices along each link, from u to v
	l.succ = make([][]int, n)
	l.pred = make([][]int, n)
	addV := func(rank int) int {
		id := len(l.ranks)
		l.ranks = append(l.ranks, rank)
		l.succ = append(l.succ, nil)
		l.pred = append(l.pred, nil)
		crossSize = append(crossSize, 2)
		return id
	}
	connect := func(a, b int) {
		l.succ[a] = append(l.succ[a], b)
		l.pred[b] = append(l.pred[b], a)
	}
	for i, k := range ll {
		u, v := k[0], k[1]
		prev := u
		for r := l.ranks[u] + 1; r < l.ranks[v]; r++ {
			w := addV(r)
			chains[i] = append(chains[i], w)
			connect(prev, w)
			prev = w
		}
		connect(prev, v)
	}
	l.order(o.Sweeps)
	xs := l.coords(crossSize, o.NodeSep)

	// Rank bands along the flow.
	nr := len(l.layers)
	bandSize := make([]float32, nr)
	for i := range n {
		if a := along(sizes[i]); a > bandSize[l.ranks[i]] {
			bandSize[l.ranks[i]] = a
		}
	}
	bandStart := make([]float32, nr)
	var acc float32
	for r := range nr {
		bandStart[r] = acc
		acc += bandSize[r] + o.RankSep
	}
	total := acc - o.RankSep
	mainOf := func(v int) float32 { // centre along the flow
		r := l.ranks[v]
		c := bandStart[r] + bandSize[r]/2
		if o.Direction == BottomUp || o.Direction == RightLeft {
			c = total - c
		}
		return c
	}
	minX := float32(math.Inf(1))
	for v := range xs {
		if lo := xs[v] - crossSize[v]/2; lo < minX {
			minX = lo
		}
	}
	at := func(v int) geom.Point { // centre of vertex v in graph units
		cx := xs[v] - minX
		if horiz {
			return geom.Pt(mainOf(v), cx)
		}
		return geom.Pt(cx, mainOf(v))
	}
	for i := range n {
		c := at(i)
		g.Nodes[i].Pos = geom.Pt(c.X-sizes[i].W/2, c.Y-sizes[i].H/2)
	}
	for i := range g.Edges {
		g.Edges[i].Bends = nil
	}
	for i, k := range links {
		ch := chains[i]
		if len(ch) == 0 {
			continue
		}
		// Each virtual node becomes a straight run across its rank band
		// (entry and exit points), so edges keep to the channel the layout
		// reserved and turn only between ranks.
		bends := make([]geom.Point, 0, 2*len(ch))
		for _, w := range ch {
			c := at(w)
			r := l.ranks[w]
			a, b := bandStart[r], bandStart[r]+bandSize[r]
			if o.Direction == BottomUp || o.Direction == RightLeft {
				a, b = total-a, total-b
			}
			if horiz {
				bends = append(bends, geom.Pt(a, c.Y), geom.Pt(b, c.Y))
			} else {
				bends = append(bends, geom.Pt(c.X, a), geom.Pt(c.X, b))
			}
		}
		if rev[i] {
			slices.Reverse(bends)
		}
		g.Edges[k.e].Bends = bends
	}
}

// layered is the working state of Layout over vertex ids: 0..n-1 are the
// graph's nodes, the rest virtual nodes of long edges.
type layered struct {
	n          int
	ranks      []int
	succ, pred [][]int
	layers     [][]int // vertex ids of each rank in order
	pos        []int   // index of each vertex in its layer
}

// breakCycles returns which links to reverse so the graph is acyclic: the
// back edges of an iterative depth-first search in node order.
func (l *layered) breakCycles(ll [][2]int) []bool {
	out := make([][]int, l.n) // link indices by source
	for i, k := range ll {
		out[k[0]] = append(out[k[0]], i)
	}
	rev := make([]bool, len(ll))
	state := make([]uint8, l.n) // 0 new, 1 on stack, 2 done
	type frame struct{ v, next int }
	var stack []frame
	for s := range l.n {
		if state[s] != 0 {
			continue
		}
		stack = append(stack[:0], frame{s, 0})
		state[s] = 1
		for len(stack) > 0 {
			f := &stack[len(stack)-1]
			if f.next == len(out[f.v]) {
				state[f.v] = 2
				stack = stack[:len(stack)-1]
				continue
			}
			li := out[f.v][f.next]
			f.next++
			w := ll[li][1]
			switch state[w] {
			case 0:
				state[w] = 1
				stack = append(stack, frame{w, 0})
			case 1:
				rev[li] = true
			}
		}
	}
	return rev
}

// rank assigns longest-path ranks on the acyclic links, then moves every
// source down to just above its nearest successor so it does not stretch
// its edges across the whole drawing.
func (l *layered) rank(ll [][2]int) {
	n := l.n
	succ := make([][]int, n)
	indeg := make([]int, n)
	for _, k := range ll {
		succ[k[0]] = append(succ[k[0]], k[1])
		indeg[k[1]]++
	}
	rank := make([]int, n)
	queue := make([]int, 0, n)
	for v := range n {
		if indeg[v] == 0 {
			queue = append(queue, v)
		}
	}
	topo := make([]int, 0, n)
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		topo = append(topo, v)
		for _, w := range succ[v] {
			if rank[v]+1 > rank[w] {
				rank[w] = rank[v] + 1
			}
			if indeg[w]--; indeg[w] == 0 {
				queue = append(queue, w)
			}
		}
	}
	hasPred := make([]bool, n)
	for _, k := range ll {
		hasPred[k[1]] = true
	}
	for i := len(topo) - 1; i >= 0; i-- {
		v := topo[i]
		if hasPred[v] || len(succ[v]) == 0 {
			continue
		}
		m := math.MaxInt
		for _, w := range succ[v] {
			m = min(m, rank[w])
		}
		rank[v] = m - 1
	}
	l.ranks = rank
}

// order builds the layers and reduces crossings with barycentric sweeps.
func (l *layered) order(sweeps int) {
	nv := len(l.ranks)
	maxR := 0
	for _, r := range l.ranks {
		maxR = max(maxR, r)
	}
	l.layers = make([][]int, maxR+1)
	// Initial order: depth-first from the sources in node order, so
	// connected nodes start out close together.
	seen := make([]bool, nv)
	var visit func(v int)
	visit = func(v int) {
		stack := []int{v}
		for len(stack) > 0 {
			v := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if seen[v] {
				continue
			}
			seen[v] = true
			l.layers[l.ranks[v]] = append(l.layers[l.ranks[v]], v)
			for i := len(l.succ[v]) - 1; i >= 0; i-- {
				if w := l.succ[v][i]; !seen[w] {
					stack = append(stack, w)
				}
			}
		}
	}
	for v := range l.n {
		if len(l.pred[v]) == 0 {
			visit(v)
		}
	}
	for v := range nv {
		if !seen[v] {
			visit(v)
		}
	}
	l.pos = make([]int, nv)
	l.reindex()
	best := l.snapshot()
	bestC := l.crossings()
	bary := make([]keyed, 0, 64)
	for it := 0; it < sweeps && bestC > 0; it++ {
		for r := 1; r < len(l.layers); r++ {
			l.sortLayer(r, l.pred, &bary)
		}
		for r := len(l.layers) - 2; r >= 0; r-- {
			l.sortLayer(r, l.succ, &bary)
		}
		if c := l.crossings(); c < bestC {
			bestC, best = c, l.snapshot()
		}
	}
	l.layers = best
	l.reindex()
}

func (l *layered) reindex() {
	for _, layer := range l.layers {
		for i, v := range layer {
			l.pos[v] = i
		}
	}
}

func (l *layered) snapshot() [][]int {
	out := make([][]int, len(l.layers))
	for i, layer := range l.layers {
		out[i] = slices.Clone(layer)
	}
	return out
}

// sortLayer orders layer r by the mean position of each vertex's
// neighbours in the adjacent layer (nbr = pred or succ); vertices without
// neighbours keep their place.
func (l *layered) sortLayer(r int, nbr [][]int, buf *[]keyed) {
	layer := l.layers[r]
	b := (*buf)[:0]
	for i, v := range layer {
		ns := nbr[v]
		k := float64(i)
		if len(ns) > 0 {
			s := 0
			for _, w := range ns {
				s += l.pos[w]
			}
			k = float64(s) / float64(len(ns))
		}
		b = append(b, keyed{k, v})
	}
	*buf = b
	slices.SortStableFunc(b, func(x, y keyed) int {
		switch {
		case x.k < y.k:
			return -1
		case x.k > y.k:
			return 1
		}
		return 0
	})
	for i, e := range b {
		layer[i] = e.v
		l.pos[e.v] = i
	}
}

type keyed struct {
	k float64
	v int
}

// crossings counts edge crossings between all adjacent layers (inversion
// count with a Fenwick tree, O(E log V)).
func (l *layered) crossings() int {
	total := 0
	var pairs [][2]int
	var tree []int
	for r := 0; r+1 < len(l.layers); r++ {
		pairs = pairs[:0]
		for _, u := range l.layers[r] {
			for _, w := range l.succ[u] {
				pairs = append(pairs, [2]int{l.pos[u], l.pos[w]})
			}
		}
		if len(pairs) < 2 {
			continue
		}
		slices.SortFunc(pairs, func(a, b [2]int) int {
			if a[0] != b[0] {
				return a[0] - b[0]
			}
			return a[1] - b[1]
		})
		m := len(l.layers[r+1])
		if cap(tree) < m+1 {
			tree = make([]int, m+1)
		}
		tree = tree[:m+1]
		clear(tree)
		seen := 0
		for _, p := range pairs {
			// Count earlier edges ending strictly right of p[1].
			le := 0
			for i := p[1] + 1; i > 0; i -= i & -i {
				le += tree[i]
			}
			total += seen - le
			for i := p[1] + 1; i <= m; i += i & -i {
				tree[i]++
			}
			seen++
		}
	}
	return total
}

// coords assigns each vertex its centre across the ranks: an initial
// packing, then alternating sweeps that pull every vertex towards the mean
// of its neighbours in the previous layer of the sweep, solved exactly per
// layer under the minimum-gap constraints. Virtual vertices weigh more, so
// long edges come out straight.
func (l *layered) coords(size []float32, sep float32) []float32 {
	nv := len(l.ranks)
	x := make([]float32, nv)
	for _, layer := range l.layers {
		var acc float32
		for i, v := range layer {
			if i > 0 {
				acc += sep
			}
			x[v] = acc + size[v]/2
			acc += size[v]
		}
	}
	// Centre layers on each other before refining.
	for _, layer := range l.layers {
		if len(layer) == 0 {
			continue
		}
		w := x[layer[len(layer)-1]] + size[layer[len(layer)-1]]/2
		for _, v := range layer {
			x[v] -= w / 2
		}
	}
	var want, wt, gap []float64
	fit := func(layer []int, nbr, other [][]int) {
		want, wt, gap = want[:0], wt[:0], gap[:0]
		for i, v := range layer {
			s, k := 0.0, 0
			for _, w := range nbr[v] {
				s += float64(x[w])
				k++
			}
			if other != nil {
				for _, w := range other[v] {
					s += float64(x[w])
					k++
				}
			}
			d := float64(x[v])
			weight := 0.25 // no neighbours: stay put, but yield to others
			if k > 0 {
				d = s / float64(k)
				weight = 1
				if v >= l.n {
					weight = 4
				}
			}
			want = append(want, d)
			wt = append(wt, weight)
			if i > 0 {
				p := layer[i-1]
				gap = append(gap, float64(size[p]/2+sep+size[v]/2))
			}
		}
		out := isotonicGap(want, wt, gap)
		for i, v := range layer {
			x[v] = float32(out[i])
		}
	}
	// The last sweep looks both ways, balancing parents and children.
	for it := range 8 {
		both := it == 7
		if it%2 == 0 {
			for r := 1; r < len(l.layers); r++ {
				fit(l.layers[r], l.pred, pick(both, l.succ))
			}
		} else {
			for r := len(l.layers) - 2; r >= 0; r-- {
				fit(l.layers[r], l.succ, pick(both, l.pred))
			}
		}
	}
	return x
}

// isotonicGap returns x minimising Σ w_i (x_i − d_i)² subject to
// x_{i+1} − x_i ≥ gap_i, by pool-adjacent-violators on the shifted values
// y_i = x_i − Σ_{j<i} gap_j (which must be non-decreasing).
func isotonicGap(d, w, gap []float64) []float64 {
	n := len(d)
	shift := make([]float64, n)
	for i := 1; i < n; i++ {
		shift[i] = shift[i-1] + gap[i-1]
	}
	type block struct {
		sum, wsum float64
		count     int
	}
	blocks := make([]block, 0, n)
	for i := range n {
		y := d[i] - shift[i]
		blocks = append(blocks, block{y * w[i], w[i], 1})
		for len(blocks) > 1 {
			a, b := blocks[len(blocks)-2], blocks[len(blocks)-1]
			if a.sum/a.wsum <= b.sum/b.wsum {
				break
			}
			blocks = blocks[:len(blocks)-2]
			blocks = append(blocks, block{a.sum + b.sum, a.wsum + b.wsum, a.count + b.count})
		}
	}
	out := make([]float64, n)
	i := 0
	for _, b := range blocks {
		m := b.sum / b.wsum
		for k := 0; k < b.count; k++ {
			out[i] = m + shift[i]
			i++
		}
	}
	return out
}

func pick(on bool, v [][]int) [][]int {
	if on {
		return v
	}
	return nil
}
