package buffer

import "sort"

// Tracker maps offsets through one set of edits by the rule Resolve uses
// (Monaco's tracked ranges, nodeAcceptEdit), without a Buffer: an editor
// command computes where its selections land before it applies the edits
// (the selections after an edit are recorded with it for redo). The edits
// are as Snapshot.Apply takes them: sorted, non-overlapping, in the
// coordinates of the text they apply to.
//
// Map costs O(log n) plus the edits touching the offset, so mapping the
// selections of many cursors through the edits of many cursors stays
// O((cursors + edits) log edits).
type Tracker struct {
	bt  batch
	pre []int // pre[i]: length change of bt[:i]
}

// NewTracker returns a Tracker for edits (no-op edits are ignored; inserted
// lengths are measured after EOL normalisation, as Apply stores them).
func NewTracker(edits []Edit) *Tracker {
	t := &Tracker{bt: newBatch(edits)}
	t.pre = make([]int, len(t.bt)+1)
	for i, sp := range t.bt {
		t.pre[i+1] = t.pre[i] + sp.n - (sp.end - sp.start)
	}
	return t
}

// Map returns where off, an offset in the text before the edits, lands in
// the text after them: an anchor of the given bias resolved through the
// edits (see Resolve; it is not clamped or snapped to a character).
func (t *Tracker) Map(off int, bias Bias) int {
	bt := t.bt
	// Spans starting after off leave it alone; the others are processed
	// last to first (as transform does) while they can still touch it, and
	// every span before those only shifts it.
	j := sort.Search(len(bt), func(i int) bool { return bt[i].start > off })
	for j > 0 {
		sp := bt[j-1]
		if sp.end < off {
			break
		}
		off = acceptEdit(off, sp.start, sp.end-sp.start, sp.n, bias == Left)
		j--
	}
	return off + t.pre[j]
}
