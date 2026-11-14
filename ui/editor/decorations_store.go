package editor

import (
	"slices"
	"sort"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/editor/buffer"
)

// This file holds the storage of the model decorations (docs/specs/
// diff-view.md, 2.1): a sorted array of ranges that moves with the text.
// It is not wired to Model yet; the public API (Model.SetDecorations and
// friends) and the painting come with it.

// decorationStickiness is what a decoration's ends do when text is typed
// exactly at them (Monaco's TrackedRangeStickiness). Each value is a pair
// of buffer.Anchor biases (see the buffer.Anchor doc).
type decorationStickiness uint8

const (
	decorationAlwaysGrows decorationStickiness = iota // start Left, end Right (Monaco's default)
	decorationNeverGrows                              // start Right, end Left
	decorationGrowsBefore                             // start Left, end Left
	decorationGrowsAfter                              // start Right, end Right
)

// biases returns the anchor biases of the start and the end.
func (s decorationStickiness) biases() (start, end buffer.Bias) {
	switch s {
	case decorationNeverGrows:
		return buffer.Right, buffer.Left
	case decorationGrowsBefore:
		return buffer.Left, buffer.Left
	case decorationGrowsAfter:
		return buffer.Right, buffer.Right
	}
	return buffer.Left, buffer.Right
}

// decorationSpanLimit is the longest range kept in the sorted array; the
// queries scan back that far. Longer ranges (a decoration over a whole
// file) go to a small list that is scanned linearly.
const decorationSpanLimit = 64 << 10

// decorationSpec is one decoration as an owner sets and reads it: a
// half-open UTF-8 byte range and its options (O, the options type, is
// opaque to the store).
type decorationSpec[O any] struct {
	start, end int
	stick      decorationStickiness
	opts       *O
}

// decorationEntry is a stored decoration.
type decorationEntry[O any] struct {
	start, end int
	opts       *O
	view       *State // nil: shown in every view; else only in this one
	idx        int32  // position in the owner's list
	owner      uint16 // index into decorationStore.names
	stick      decorationStickiness
}

// decorationOwner identifies a decoration set: an owner name, either for
// the model (view nil) or for one view (Monaco's ownerId filter).
type decorationOwner struct {
	name string
	view *State
}

// decorationStore holds all decoration sets of one model. Ranges are
// offsets into the model's current snapshot; apply moves them through each
// edit, in O(log D + affected) plus one addition per decoration after the
// edits.
type decorationStore[O any] struct {
	es        []decorationEntry[O] // spans <= spanLimit, sorted by start
	big       []decorationEntry[O] // longer spans, sorted by start
	maxSpan   int                  // >= the longest span in es
	spanLimit int

	ids    map[decorationOwner]uint16
	names  []decorationOwner // by id; a zero entry is a free slot
	counts []int             // decorations per id
	free   []uint16

	// version changes whenever any range or set changes (for caches such
	// as the overview ruler's); sets only when a set changes (not when the
	// ranges move with an edit).
	version uint64
	sets    uint64
}

func newDecorationStore[O any]() *decorationStore[O] {
	return &decorationStore[O]{spanLimit: decorationSpanLimit, ids: map[decorationOwner]uint16{}}
}

// len returns the number of stored decorations.
func (st *decorationStore[O]) len() int { return len(st.es) + len(st.big) }

// set replaces the decorations of owner name for view (nil: the model)
// with ds, whose ranges are offsets of snap; empty ds removes the owner.
// Ranges are clamped to the text, ordered, and widened to whole
// characters. O(D + n log n) for n new decorations among D.
func (st *decorationStore[O]) set(name string, view *State, ds []decorationSpec[O], snap buffer.Snapshot) {
	key := decorationOwner{name, view}
	id, ok := st.ids[key]
	if ok {
		st.removeOwner(id)
	}
	st.version++
	st.sets++
	if len(ds) == 0 {
		if ok {
			delete(st.ids, key)
			st.names[id] = decorationOwner{}
			st.free = append(st.free, id)
		}
		return
	}
	if !ok {
		if k := len(st.free); k > 0 {
			id = st.free[k-1]
			st.free = st.free[:k-1]
			st.names[id] = key
		} else {
			if len(st.names) > 0xFFFF {
				panic("editor: more than 65536 decoration owners")
			}
			id = uint16(len(st.names))
			st.names = append(st.names, key)
			st.counts = append(st.counts, 0)
		}
		st.ids[key] = id
	}
	st.counts[id] = len(ds)
	add := make([]decorationEntry[O], 0, len(ds))
	for i, d := range ds {
		a, b := d.start, d.end
		if b < a {
			a, b = b, a
		}
		a = snapToRune(snap, a, buffer.Left)
		b = snapToRune(snap, b, buffer.Right)
		e := decorationEntry[O]{start: a, end: b, opts: d.opts, view: view, idx: int32(i), owner: id, stick: d.stick}
		if b-a > st.spanLimit {
			st.big = append(st.big, e)
			continue
		}
		add = append(add, e)
	}
	// Ties keep the owner's order (idx is unique within the owner); owners
	// usually pass sorted ranges, which skips the sort.
	byStart := func(a, b decorationEntry[O]) int {
		if a.start != b.start {
			return a.start - b.start
		}
		return int(a.idx) - int(b.idx)
	}
	if !slices.IsSortedFunc(add, byStart) {
		slices.SortFunc(add, byStart)
	}
	slices.SortStableFunc(st.big, byStart)
	// Merge the new entries into es.
	merged := make([]decorationEntry[O], 0, len(st.es)+len(add))
	i, j := 0, 0
	for i < len(st.es) && j < len(add) {
		if add[j].start < st.es[i].start {
			merged = append(merged, add[j])
			j++
		} else {
			merged = append(merged, st.es[i])
			i++
		}
	}
	merged = append(append(merged, st.es[i:]...), add[j:]...)
	st.es = merged
	st.maxSpan = 0
	for _, e := range st.es {
		st.maxSpan = max(st.maxSpan, e.end-e.start)
	}
}

// removeOwner drops the entries of owner id (keeping its slot).
func (st *decorationStore[O]) removeOwner(id uint16) {
	drop := func(e decorationEntry[O]) bool { return e.owner == id }
	st.es = slices.DeleteFunc(st.es, drop)
	st.big = slices.DeleteFunc(st.big, drop)
	st.counts[id] = 0
}

// get returns the current decorations of owner name for view, in the order
// they were set; nil when there are none.
func (st *decorationStore[O]) get(name string, view *State) []decorationSpec[O] {
	id, ok := st.ids[decorationOwner{name, view}]
	if !ok {
		return nil
	}
	out := make([]decorationSpec[O], st.counts[id])
	for _, list := range [2][]decorationEntry[O]{st.es, st.big} {
		for _, e := range list {
			if e.owner == id {
				out[e.idx] = decorationSpec[O]{start: e.start, end: e.end, stick: e.stick, opts: e.opts}
			}
		}
	}
	return out
}

// dropView removes every decoration set of view (State.Close).
func (st *decorationStore[O]) dropView(view *State) {
	for key := range st.ids {
		if key.view == view {
			st.set(key.name, view, nil, buffer.Snapshot{})
		}
	}
}

// apply moves every decoration through edits (sorted, non-overlapping, in
// the previous snapshot's offsets; t is buffer.NewTracker(edits), shared
// with the other consumers of the same change) into snap, the text after
// them. Each end moves like a buffer.Anchor with its stickiness' bias
// (Buffer.Resolve: Monaco's marker rule, clamped and snapped to a
// character), and a range whose ends crossed collapses to its start
// (Monaco checks this after each edit of a batch, this after the batch;
// they only differ for ranges inside a multi-edit batch's touching edits).
//
// Only the decorations that can touch the edits are mapped one by one (a
// binary search finds the first: no range in es is longer than maxSpan);
// the ones after the last edit shift by its total length change, with no
// allocation.
func (st *decorationStore[O]) apply(edits []buffer.Edit, t *buffer.Tracker, snap buffer.Snapshot) {
	first, last := -1, -1
	for _, e := range edits {
		if e.Start == e.End && e.Text == "" {
			continue
		}
		if first < 0 {
			first = e.Start
		}
		last = e.End
	}
	if first < 0 || st.len() == 0 {
		return
	}
	st.version++
	delta := t.Map(last+1, buffer.Left) - (last + 1)
	es := st.es
	i0 := sort.Search(len(es), func(i int) bool { return es[i].start >= first-st.maxSpan })
	i1 := i0 + sort.Search(len(es)-i0, func(i int) bool { return es[i0+i].start > last })
	moved := false
	for i := i0; i < i1; i++ {
		e := &es[i]
		if e.end < first {
			continue
		}
		mapDecoration(e, t, snap)
		span := e.end - e.start
		st.maxSpan = max(st.maxSpan, span)
		moved = moved || span > st.spanLimit
	}
	for i := i1; i < len(es); i++ {
		es[i].start += delta
		es[i].end += delta
	}
	// Mapped starts stay at or after first and before the shifted tail;
	// ends with different biases may have reordered ties inside the window.
	for i := i0 + 1; i < i1; i++ {
		for j := i; j > i0 && es[j].start < es[j-1].start; j-- {
			es[j], es[j-1] = es[j-1], es[j]
		}
	}
	for i := range st.big {
		mapDecoration(&st.big[i], t, snap)
	}
	if moved {
		for _, e := range es[i0:i1] {
			if e.end-e.start > st.spanLimit {
				st.big = append(st.big, e)
			}
		}
		st.es = slices.DeleteFunc(es, func(e decorationEntry[O]) bool { return e.end-e.start > st.spanLimit })
	}
	if len(st.big) > 1 {
		slices.SortStableFunc(st.big, func(a, b decorationEntry[O]) int { return a.start - b.start })
	}
}

// mapDecoration moves e through t into snap.
func mapDecoration[O any](e *decorationEntry[O], t *buffer.Tracker, snap buffer.Snapshot) {
	sb, eb := e.stick.biases()
	e.start = snapToRune(snap, t.Map(e.start, sb), sb)
	e.end = max(e.start, snapToRune(snap, t.Map(e.end, eb), eb))
}

// query appends to dst the decorations intersecting or touching [start,
// end] (a range ending at start or beginning at end counts, as in Monaco's
// getDecorationsInRange) that view shows: model decorations, and with a
// non-nil view also that view's own. The result is sorted by start.
// O(log D + k) plus the ranges starting up to maxSpan before start.
func (st *decorationStore[O]) query(start, end int, view *State, dst []decorationEntry[O]) []decorationEntry[O] {
	es := st.es
	i := sort.Search(len(es), func(i int) bool { return es[i].start >= start-st.maxSpan })
	for ; i < len(es) && es[i].start <= end; i++ {
		if e := &es[i]; e.end >= start && (e.view == nil || e.view == view) {
			dst = append(dst, *e)
		}
	}
	n := len(dst)
	for _, e := range st.big {
		if e.start <= end && e.end >= start && (e.view == nil || e.view == view) {
			dst = append(dst, e)
		}
	}
	if len(dst) > n {
		slices.SortStableFunc(dst, func(a, b decorationEntry[O]) int { return a.start - b.start })
	}
	return dst
}

// snapToRune clamps off to snap and, when it falls inside a multi-byte
// character, moves it to the character's start (Left) or end (Right): the
// same adjustment Buffer.Resolve makes.
func snapToRune(snap buffer.Snapshot, off int, bias buffer.Bias) int {
	n := snap.Len()
	off = min(max(off, 0), n)
	if off == 0 || off == n {
		return off
	}
	// One byte never spans rope leaves, so the common case allocates nothing.
	if b := snap.Slice(off, off+1); utf8.RuneStart(b[0]) {
		return off
	}
	lo := max(0, off-(utf8.UTFMax-1))
	w := snap.Slice(lo, min(n, lo+2*utf8.UTFMax))
	at := off - lo
	for i := at - 1; i >= 0; i-- {
		if utf8.RuneStart(w[i]) {
			_, size := utf8.DecodeRuneInString(w[i:])
			if size <= 1 || i+size <= at {
				return off // invalid UTF-8: leave it
			}
			if bias == buffer.Right {
				return lo + i + size
			}
			return lo + i
		}
	}
	return off
}
