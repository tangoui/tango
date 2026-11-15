package editor

import (
	"sort"

	"github.com/tangoui/tango/ui/editor/buffer"
)

// Long lines in the bracket tree.
//
// A line longer than maxTokenizationLineLength has no tokens (all of it is
// one Other token), so its brackets are a function of its text and its
// start stack alone. The tree keeps, for each such line it scanned, the
// parser's state every brCkEvery bytes or so (brIndex): the position, the
// regexp's start (scanRange's fresh), the stack, and the lowest depth of the
// segment up to the next checkpoint. Then
//
//   - an edit inside the line (no line break, same line count) rescans from
//     the last checkpoint whose state read only bytes before the change and
//     stops at the first old checkpoint after it whose state the scan
//     reaches again (same position, regexp start and stack), reusing the
//     old checkpoints after it, shifted (brIndex.rescan);
//   - the queries that need the brackets around an offset (matching, the
//     enclosing pair, the colors of the rows in view) scan from the
//     checkpoint before it, and follow a pair out of the scanned part by
//     the segments' lowest depths: the first later segment that falls below
//     a depth holds the bracket that pops it, the last earlier one the
//     bracket that pushed it (as closeOf and openOf do with lines).
//
// So a keystroke in the middle of a 1 MB line scans a few checkpoints'
// bytes, not the line, unless the edit changes the stack after it (an
// unbalanced bracket typed): then the rest of the line is rescanned, as its
// end stack, and the brackets after the edit, do change. The checkpoints
// themselves (about len/brCkEvery of them) are shifted, not rescanned.

// brCkEvery is the spacing of a long line's checkpoints: rescans keep it
// between brCkEvery/2 and 2*brCkEvery.
const brCkEvery = 4096

// bracketLongIndexOff turns the long-line index off (tests compare the
// queries with and without it).
var bracketLongIndexOff = false

// brCk is the bracket parser's state at byte p of a long line.
type brCk struct {
	p     int
	fresh bool   // the regexp runs from p (scanRange)
	s     *bnode // the stack at p
	min   int32  // the lowest depth from p to the next checkpoint (or the line end)
}

// brIndex is the checkpoint index of one long line (see the top of the
// file).
type brIndex struct {
	n     int    // the line's length
	start *bnode // the stack the line was scanned from
	cks   []brCk // cks[0] is at 0
	end   *bnode // the stack at the line end
	stop  bool   // a case variant stopped the scan in the last segment
	// dlo, dhi are the bytes edits changed since the scan (dlo < 0:
	// none): the checkpoints before stay, those after are shifted.
	dlo, dhi int
	spare    []brCk
	// text is the line's text while the index is up to date ("" after an
	// edit until the rescan): queries across lines (longClose, longOpen)
	// read it instead of copying the line out of the rope again. It costs
	// the line's size per indexed line (documents of up to
	// maxBracketTreeLen bytes).
	text string
}

// minD returns the lowest depth the line reaches.
func (ix *brIndex) minD() int32 {
	m := stackDepth(ix.start)
	for _, c := range ix.cks {
		m = min(m, c.min)
	}
	return m
}

// ckAt returns the last checkpoint at or before byte p.
func (ix *brIndex) ckAt(p int) int {
	return max(0, sort.Search(len(ix.cks), func(k int) bool { return ix.cks[k].p > p })-1)
}

// segEnd returns where checkpoint k's segment ends.
func (ix *brIndex) segEnd(k int) int {
	if k+1 < len(ix.cks) {
		return ix.cks[k+1].p
	}
	return ix.n
}

// indexLine scans long line text from stack start into a new index.
func (bc *bracketConf) indexLine(text string, start *bnode) *brIndex {
	ix := &brIndex{dlo: -1}
	bc.reindex(ix, text, start)
	return ix
}

// reindex scans the whole line again into ix.
func (bc *bracketConf) reindex(ix *brIndex, text string, start *bnode) {
	ix.n, ix.start, ix.dlo, ix.dhi = len(text), start, -1, 0
	cks := append(ix.spare[:0], brCk{p: 0, fresh: true, s: start})
	ix.spare = ix.cks
	bc.indexScan(ix, text, cks, nil, 0, nil, false)
}

// rescan brings ix up to date with text after edits (ix.dlo >= 0, the same
// start stack): from the last checkpoint not depending on the changed
// bytes to the first old one past them the scan reaches in the same state.
func (bc *bracketConf) rescan(ix *brIndex, text string) {
	if ix.dlo < 0 {
		return
	}
	// The last checkpoint whose state read only bytes before dlo (cks[0]
	// is the start stack, whatever the text).
	k0 := 0
	for k := len(ix.cks) - 1; k > 0; k-- {
		if ix.cks[k].p+bc.maxLen <= ix.dlo {
			k0 = k
			break
		}
	}
	old := ix.cks
	cks := append(ix.spare[:0], old[:k0+1]...)
	dhi := ix.dhi
	ix.dlo, ix.dhi = -1, 0
	bc.indexScan(ix, text, cks, old[k0+1:], dhi, ix.end, ix.stop)
	ix.spare = old
}

// indexScan scans text from the last of cks (kept: its segment is scanned
// again) to the line end, adding checkpoints, and stops early at the first
// of old (the old checkpoints after the change, shifted) past dhi that the
// scan reaches in the same state: old's rest, oldEnd and oldStop hold.
func (bc *bracketConf) indexScan(ix *brIndex, text string, cks, old []brCk, dhi int, oldEnd *bnode, oldStop bool) {
	last := cks[len(cks)-1]
	st := brState{s: last.s, minD: stackDepth(last.s)}
	p, fresh := last.p, last.fresh
	apply := func(d *bracketDef, b0, b1 int) { bc.apply(&st, d, b0, b1, nil) }
	m := 0 // the next old checkpoint that may match
	for {
		limit := min(len(text), p+brCkEvery)
		// Pause at an old checkpoint past the change, so the scan may stop
		// there; one too close to the last checkpoint is skipped (the
		// spacing stays within brCkEvery/2 .. 2*brCkEvery).
		for m < len(old) && (old[m].p < p+brCkEvery/2 || old[m].p <= dhi) {
			m++
		}
		if m < len(old) && old[m].p <= p+2*brCkEvery {
			limit = min(len(text), old[m].p)
		}
		var stopped bool
		p, fresh, stopped = bc.scanRange(text, p, fresh, len(text), limit, apply)
		cks[len(cks)-1].min = st.minD
		if stopped || p >= len(text) {
			ix.end, ix.stop = st.s, stopped
			break
		}
		// The state after p reads text[p-1:], unchanged past dhi.
		if m < len(old) && p == old[m].p && fresh == old[m].fresh && stackEqual(st.s, old[m].s) {
			cks = append(cks, old[m:]...)
			ix.end, ix.stop = oldEnd, oldStop
			break
		}
		cks = append(cks, brCk{p: p, fresh: fresh, s: st.s})
		st.minD = stackDepth(st.s)
	}
	ix.cks = cks
}

// edited follows an edit of the line: bytes [at, at+oldLen) replaced by
// newLen bytes (no line break).
func (ix *brIndex) edited(at, oldLen, newLen int) {
	delta := newLen - oldLen
	mapPos := func(x int) int {
		switch {
		case x <= at:
			return x
		case x >= at+oldLen:
			return x + delta
		}
		return at + newLen
	}
	if ix.dlo < 0 {
		ix.dlo, ix.dhi = at, at+newLen
	} else {
		ix.dlo, ix.dhi = min(ix.dlo, at), max(mapPos(ix.dhi), at+newLen)
	}
	for k := range ix.cks {
		ix.cks[k].p = mapPos(ix.cks[k].p)
	}
	ix.n += delta
	ix.text = ""
}

// longIndex returns the up-to-date index of known line i, nil when the
// line has none (a line of tokens, or the index is off).
func (t *bracketTree) longIndex(i int) *brIndex {
	if bracketLongIndexOff || i >= t.known() {
		return nil
	}
	ix := t.long[i]
	if ix == nil || ix.dlo >= 0 || len(ix.text) != ix.n || ix.n != lineEnd(t.snap, i)-t.snap.LineStart(i) || !stackEqual(ix.start, t.start[i]) {
		return nil
	}
	return ix
}

// scanLong is ensure's scan of long line i (text): incremental when its
// index follows the edits from the same start stack.
func (t *bracketTree) scanLong(i int, text string) (*bnode, int32) {
	if t.long == nil {
		t.long = map[int]*brIndex{}
	}
	ix := t.long[i]
	switch {
	case ix == nil:
		ix = t.conf.indexLine(text, t.start[i])
		t.long[i] = ix
	case ix.n != len(text) || !stackEqual(ix.start, t.start[i]):
		t.conf.reindex(ix, text, t.start[i])
	default:
		t.conf.rescan(ix, text) // nothing when the edits did not reach it
	}
	ix.text = text
	return ix.end, ix.minD()
}

// editLong follows single-line edits (old's coordinates, no line breaks,
// the line count unchanged) in the indexes of their lines.
func (t *bracketTree) editLong(old buffer.Snapshot, edits []buffer.Edit) {
	if len(t.long) == 0 {
		return
	}
	// Last edit first: an edit's offsets hold until the earlier ones apply.
	for k := len(edits) - 1; k >= 0; k-- {
		e := edits[k]
		if e.Start == e.End && e.Text == "" {
			continue
		}
		l := old.LineOf(e.Start)
		if ix := t.long[l]; ix != nil {
			ls := old.LineStart(l)
			ix.edited(e.Start-ls, e.End-e.Start, len(e.Text))
		}
	}
}

// moveLong follows edits that change the line count: the indexes of the
// lines edited are dropped, the others move with their lines. regions are
// the edited old lines [sl, el] and the lines m they became.
func (t *bracketTree) moveLong(regs [][3]int) {
	if len(t.long) == 0 {
		return
	}
	moved := make(map[int]*brIndex, len(t.long))
	for l, ix := range t.long {
		shift, drop := 0, false
		for _, r := range regs {
			if l > r[1] {
				shift += r[2] - (r[1] - r[0] + 1)
				continue
			}
			drop = l >= r[0]
			break
		}
		if !drop {
			moved[l+shift] = ix
		}
	}
	t.long = moved
}

// lineEvs are the bracket events of a part of line i: those of the whole
// line (ix nil), or of a long line from checkpoint k on, the scan paused
// at p (fresh, stopped, stack s).
type lineEvs struct {
	i     int
	text  string
	evs   []bracketEvent
	ix    *brIndex
	k     int
	s0    *bnode // the stack before evs
	p     int
	fresh bool
	stop  bool
	s     *bnode
}

// eventsIn returns the events of line i (known) covering at least the
// brackets that start in [from, to): of the whole line, or of a long line
// from the checkpoint at or before from.
func (t *bracketTree) eventsIn(src bracketSource, i, from, to int, text string) lineEvs {
	le := lineEvs{i: i, text: text, s0: t.start[i]}
	ix := t.longIndex(i)
	if ix == nil {
		le.evs = t.lineEvents(src, i, nil)
		return le
	}
	le.ix, le.k = ix, ix.ckAt(max(from, 0))
	c := ix.cks[le.k]
	le.s0 = c.s
	st := brState{s: c.s, minD: stackDepth(c.s)}
	le.p, le.fresh, le.stop = t.conf.scanRange(text, c.p, c.fresh, len(text), min(max(to, c.p), len(text)), func(d *bracketDef, b0, b1 int) {
		t.conf.apply(&st, d, b0, b1, func(e bracketEvent) { le.evs = append(le.evs, e) })
	})
	le.s = st.s
	return le
}

// partial reports whether le misses brackets of the line after its events
// (a long line's scan paused before the end).
func (le *lineEvs) partial() bool {
	return le != nil && le.ix != nil && !le.stop && le.p < len(le.text)
}

// closeInLine finds, after le's events, the bracket of long line le.i that
// pops the stack entry at depth d (open at the pause): the rest of the
// paused segment, else the first later segment falling below d.
func (t *bracketTree) closeInLine(le *lineEvs, d int32) (bracketEvent, bool) {
	if !le.partial() {
		return bracketEvent{}, false
	}
	ix, text := le.ix, le.text
	k := ix.ckAt(le.p)
	if e, ok, stopped := t.findPop(text, le.p, le.fresh, le.s, ix.segEnd(k), d); ok || stopped {
		return e, ok
	}
	for j := k + 1; j < len(ix.cks); j++ {
		if c := ix.cks[j]; c.min < d {
			e, ok, _ := t.findPop(text, c.p, c.fresh, c.s, ix.segEnd(j), d)
			return e, ok
		}
	}
	return bracketEvent{}, false
}

// findPop scans text from state (p, fresh, s) to end for the first event
// popping the entry at depth d.
func (t *bracketTree) findPop(text string, p int, fresh bool, s *bnode, end int, d int32) (e bracketEvent, ok, stopped bool) {
	st := brState{s: s, minD: stackDepth(s)}
	// Only the events up to the pop are needed, but scanRange cannot stop
	// in the middle: the segment is short.
	_, _, stopped = t.conf.scanRange(text, p, fresh, len(text), min(end, len(text)), func(dd *bracketDef, b0, b1 int) {
		t.conf.apply(&st, dd, b0, b1, func(x bracketEvent) {
			if !ok && (x.kind == evClose || x.kind == evUnclosed) && x.node.depth == d {
				e, ok = x, true
			}
		})
	})
	return e, ok, stopped
}

// openInLine finds, before le's events, the opening bracket of long line
// le.i of the stack entry at depth d open where they start: the last
// earlier segment falling below d pushed it.
func (t *bracketTree) openInLine(le *lineEvs, d int32) (bracketEvent, bool) {
	if le == nil || le.ix == nil {
		return bracketEvent{}, false
	}
	ix, text := le.ix, le.text
	for j := le.k - 1; j >= 0; j-- {
		if c := ix.cks[j]; c.min < d {
			return t.lastPush(text, c, ix.segEnd(j), d)
		}
	}
	return bracketEvent{}, false
}

// lastPush scans text from checkpoint c to end for the last opening
// bracket pushing depth d.
func (t *bracketTree) lastPush(text string, c brCk, end int, d int32) (e bracketEvent, ok bool) {
	st := brState{s: c.s, minD: stackDepth(c.s)}
	t.conf.scanRange(text, c.p, c.fresh, len(text), min(end, len(text)), func(dd *bracketDef, b0, b1 int) {
		t.conf.apply(&st, dd, b0, b1, func(x bracketEvent) {
			if x.kind == evOpen && x.node.depth == d {
				e, ok = x, true
			}
		})
	})
	return e, ok
}

// longClose is scanClose's search of line j (minD[j] < d) when it is long:
// the first segment falling below d pops the entry at depth d.
func (t *bracketTree) longClose(j int, d int32) (bracketEvent, bool, bool) {
	ix := t.longIndex(j)
	if ix == nil {
		return bracketEvent{}, false, false
	}
	text := ix.text
	for k, c := range ix.cks {
		if c.min < d {
			e, ok, _ := t.findPop(text, c.p, c.fresh, c.s, ix.segEnd(k), d)
			return e, ok, true
		}
	}
	return bracketEvent{}, false, true
}

// longOpen is scanOpen's search of line k (minD[k] < d) when it is long:
// the last segment falling below d pushed the entry at depth d.
func (t *bracketTree) longOpen(k int, d int32) (bracketEvent, bool, bool) {
	ix := t.longIndex(k)
	if ix == nil {
		return bracketEvent{}, false, false
	}
	text := ix.text
	for j := len(ix.cks) - 1; j >= 0; j-- {
		if c := ix.cks[j]; c.min < d {
			e, ok := t.lastPush(text, c, ix.segEnd(j), d)
			return e, ok, true
		}
	}
	return bracketEvent{}, false, true
}
