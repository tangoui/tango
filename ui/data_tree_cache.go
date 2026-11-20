package ui

import "slices"

// Tree Height + DataVersion: the cache that makes a Height tree's frame
// follow the viewport instead of the node count (see TreeW.DataVersion).
// Without DataVersion, or without Height, none of this runs and every
// frame walks the whole tree as before.
//
// Everything cached is derived (maps of keys, pointers into the caller's
// nodes, flattened rows); no TreeNode is copied, so titles, Disabled and
// the other fields are always read live. What the cache keeps is the
// structure: which nodes exist under which parent, which are shown, and
// their check states. A structural change made in place (same top-level
// slice, same length) shows only after DataVersion changes.

// treeDataKey identifies the node data: the top-level slice (identity and
// length) and the DataVersion. nodes (&nodes[0]) keeps the caller's
// previous slice reachable until the key is replaced or the tree's state
// is dropped: the price of never matching a new slice at a reused address.
type treeDataKey struct {
	nodes *TreeNode
	n     int
	ver   int
}

// treeVisKey identifies the input of the Filter visibility map besides the
// data (a FilterMethod function is assumed to depend only on the keyword
// and the node: change DataVersion when its result changes otherwise).
type treeVisKey struct {
	search string
	fn     bool
}

type treeVerCache struct {
	data   treeDataKey
	dataOK bool
	// Data-derived: key → parent key, key → node (a pointer into the
	// caller's nodes).
	parents map[string]string
	index   map[string]*TreeNode

	// Filter visibility (w.visible) and its generation.
	visKey treeVisKey
	visOK  bool
	vis    map[string]bool
	visGen int

	// Flattened visible rows, built for the expanded keys exp (a copy) and
	// the visibility generation visGen; rowIdx maps a key to its row.
	rowsOK  bool
	exp     []string
	rowsVis int
	rows    []treeFlatRow
	rowIdx  map[string]int

	// Check states for the checked keys chk (a copy) and strict.
	chkOK      bool
	chk        []string
	strict     bool
	full, half map[string]bool
}

// DataVersion turns on the cache for a virtual scrolling tree and tells
// it that its nodes were modified in place. The tree keeps the parent
// map, Filter visibility, flattened visible rows and check states
// between frames while the top-level nodes slice (identity and length),
// v, the expanded keys, the search
// keyword and the checked keys stay the same, so a frame costs O(rows in
// view) instead of O(nodes). Titles and the other node fields are read
// live; a structural change made in place (children added, removed or
// replaced, keys changed, a title edit that changes a Filter match) shows
// only after v changes (or with a new slice). The check states, parents'
// checked and half-checked marks included, are cached too: a child's
// Disabled or DisableCheckbox changed in place (which changes how checks
// cascade) also needs a change of v. Without DataVersion the tree
// rebuilds all of this every frame, as before.
func (w *TreeW) DataVersion(v int) *TreeW { w.version, w.verSet = v, true; return w }

// cached reports whether the Height + DataVersion cache is in use.
func (w *TreeW) cached() bool { return w.verSet && w.height > 0 && w.virtual }

// syncData drops the cache when the node data changed and returns it with
// the data-derived maps built.
func (w *TreeW) syncData() *treeVerCache {
	vc := &w.st.vc
	k := treeDataKey{n: len(w.nodes), ver: w.version}
	if len(w.nodes) > 0 {
		k.nodes = &w.nodes[0]
	}
	if vc.dataOK && vc.data == k {
		return vc
	}
	*vc = treeVerCache{data: k, dataOK: true, visGen: vc.visGen + 1,
		parents: map[string]string{}, index: map[string]*TreeNode{}}
	treeWalk(w.nodes, func(n *TreeNode, path []string) {
		if len(path) > 0 {
			vc.parents[n.Key] = path[len(path)-1]
		}
		vc.index[n.Key] = n // the last of equal keys, as find
	})
	return vc
}

// cachedVisible returns the Filter visibility map for the current search
// (the same map as show's uncached walk).
func (w *TreeW) cachedVisible(vc *treeVerCache) map[string]bool {
	k := treeVisKey{search: w.search, fn: w.filterFn != nil}
	if vc.visOK && vc.visKey == k {
		return vc.vis
	}
	vis := map[string]bool{}
	treeWalk(w.nodes, func(n *TreeNode, path []string) {
		if w.matches(n) {
			vis[n.Key] = true
			for _, p := range path {
				vis[p] = true
			}
		}
	})
	vc.visKey, vc.visOK, vc.vis = k, true, vis
	vc.visGen++
	return vis
}

// rowsNow returns the flattened visible rows: flatRows itself without the
// cache, the cached rows while their inputs are unchanged with it.
func (w *TreeW) rowsNow() []treeFlatRow {
	if !w.cached() {
		return w.flatRows()
	}
	vc := &w.st.vc
	gen := 0
	if w.visible != nil {
		gen = vc.visGen
	}
	if vc.rowsOK && vc.rowsVis == gen && slices.Equal(vc.exp, *w.expanded) {
		return vc.rows
	}
	exp := make(map[string]bool, len(*w.expanded))
	for _, k := range *w.expanded {
		exp[k] = true
	}
	w.expIdx = exp
	rows := w.flatRows()
	w.expIdx = nil
	idx := make(map[string]int, len(rows))
	for i, r := range rows {
		if _, dup := idx[r.nd.Key]; !dup {
			idx[r.nd.Key] = i // the first, as treeFlatIndex
		}
	}
	vc.rowsOK, vc.rowsVis, vc.rows, vc.rowIdx = true, gen, rows, idx
	vc.exp = append(vc.exp[:0], *w.expanded...)
	return rows
}

// rowIndex is treeFlatIndex(rows, key) for rows returned by rowsNow.
func (w *TreeW) rowIndex(rows []treeFlatRow, key string) int {
	if !w.cached() || key == "" {
		return treeFlatIndex(rows, key)
	}
	if i, ok := w.st.vc.rowIdx[key]; ok {
		return i
	}
	return -1
}

// cachedChecks returns treeConduct's (or CheckStrictly's) check states for
// the checked keys, reusing them while the keys and strict are unchanged.
func (w *TreeW) cachedChecks(vc *treeVerCache) (full, half map[string]bool) {
	if vc.chkOK && vc.strict == w.strict && slices.Equal(vc.chk, *w.checked) {
		return vc.full, vc.half
	}
	if w.strict {
		full, half = map[string]bool{}, map[string]bool{}
		for _, k := range *w.checked {
			full[k] = true
		}
	} else {
		full, half = treeConduct(w.nodes, *w.checked)
	}
	vc.chkOK, vc.strict, vc.full, vc.half = true, w.strict, full, half
	vc.chk = append(vc.chk[:0], *w.checked...)
	return full, half
}
