package editor

import (
	"strings"
	"sync/atomic"

	"github.com/tangoui/tango/ui/editor/buffer"
)

// Work counters of the paths a keystroke in a long line takes (bracket
// scanning, the long-line test, the style runs of colored brackets, the
// minimap's prefixes, the copies of long lines out of the rope). Tests set
// countWork and read them to check that the work of one keystroke does not
// grow with the length of the line (Task 5b of the M2 acceptance plan);
// with countWork false (always, outside tests) they cost a load of a bool.
var (
	countWork        bool
	workBracketBytes atomic.Int64 // bytes the bracket scanner looked at
	workRuneBytes    atomic.Int64 // bytes isLong counted runes over
	workOverSpans    atomic.Int64 // spans overrideBounds looked at
	workMiniBytes    atomic.Int64 // bytes boundedMiniature read
	workMiniScan     atomic.Int64 // bytes boundedMiniature walked grapheme by grapheme
	workLineCopies   atomic.Int64 // long lines lineMemo copied out of the rope
	workBracketToks  atomic.Int64 // lines the bracket tree tokenized (no summary, no cached tokens)
)

// resetWork zeroes the work counters.
func resetWork() {
	workBracketBytes.Store(0)
	workRuneBytes.Store(0)
	workOverSpans.Store(0)
	workMiniBytes.Store(0)
	workMiniScan.Store(0)
	workLineCopies.Store(0)
	workBracketToks.Store(0)
}

// countAdd adds n to c when countWork is on.
func countAdd(c *atomic.Int64, n int) {
	if countWork {
		c.Add(int64(n))
	}
}

// lineMemo keeps the last long lines read from the snapshots of one
// Buffer. A keystroke in a long line reads it in several places (the
// rewrap, the layout, the brackets, the caret's coordinates), and each
// Snapshot.Line of a line spread over the rope's leaves is a copy of it:
// the memo makes that one copy per version of the line. A line the
// Buffer's last change did not touch (typing in another line) is carried
// to the new version without a copy.
//
// Retention: lines of memoMinLen..memoMaxLen bytes only (shorter ones are
// cheap to copy, longer ones too large to keep), at most len(ent) of them
// and memoMaxBytes in all; an entry not of the newest version or the one
// before it (the old text an incremental rewrap reads) is dropped when a
// line of a newer version comes in. UI goroutine only.
type lineMemo struct {
	buf  *buffer.Buffer
	ent  [3]memoLine
	next int
}

type memoLine struct {
	ver   uint64
	i, ls int
	s     string
}

const (
	memoMinLen   = 32 << 10
	memoMaxLen   = 16 << 20
	memoMaxBytes = 2 * memoMaxLen
)

// line returns snap.Line(i), snap a snapshot of b.
func (m *lineMemo) line(b *buffer.Buffer, snap buffer.Snapshot, i int) string {
	ls := snap.LineStart(i)
	n := lineEnd(snap, i) - ls
	if m == nil || b == nil || n < memoMinLen || n > memoMaxLen {
		return snap.Line(i)
	}
	if m.buf != b {
		*m = lineMemo{buf: b}
	}
	ver := snap.Version()
	// A version names one text of the Buffer (Snapshot.Version).
	for _, e := range m.ent {
		if e.s != "" && e.ver == ver && e.i == i && e.ls == ls && len(e.s) == n {
			return e.s
		}
	}
	if s, ok := m.carry(b, snap, i, ls, n); ok {
		return s
	}
	s := snap.Line(i)
	countAdd(&workLineCopies, 1)
	m.store(memoLine{ver: ver, i: i, ls: ls, s: s})
	return s
}

// carry finds line i (at ls, n bytes) of snap, the Buffer's current
// snapshot, among the lines of the snapshot before its last change that
// the change did not touch, and moves that entry to snap's version.
func (m *lineMemo) carry(b *buffer.Buffer, snap buffer.Snapshot, i, ls, n int) (string, bool) {
	if b.Snapshot().Version() != snap.Version() {
		return "", false
	}
	old, edits := b.LastChange()
	if len(edits) == 0 || old.Version() == snap.Version() {
		return "", false
	}
	for k := range m.ent {
		e := &m.ent[k]
		if e.s == "" || e.ver != old.Version() || len(e.s) != n {
			continue
		}
		// The line and its line breaks unchanged: every edit ends before
		// the break before it or starts after the break after it.
		nls, ni, ok := e.ls, e.i, true
		for _, x := range edits {
			switch {
			case x.End < e.ls:
				nls += len(x.Text) - (x.End - x.Start)
				ni += lineBreakCount(x.Text) - lineBreakCount(old.Slice(x.Start, x.End))
			case x.Start > e.ls+len(e.s):
			default:
				ok = false
			}
		}
		if ok && nls == ls && ni == i {
			s := e.s
			// Keep the old version's entry for a rewrap reading it.
			m.store(memoLine{ver: snap.Version(), i: i, ls: ls, s: s})
			return s, true
		}
	}
	return "", false
}

// store adds e, dropping the entries of versions older than the newest
// two and, beyond memoMaxBytes, the oldest ones.
func (m *lineMemo) store(e memoLine) {
	prev := uint64(0) // the newest version below e's
	for _, x := range m.ent {
		if x.s != "" && x.ver < e.ver {
			prev = max(prev, x.ver)
		}
	}
	total := len(e.s)
	for k := range m.ent {
		if x := m.ent[k]; x.s != "" && x.ver != e.ver && x.ver != prev && x.ver < e.ver {
			m.ent[k] = memoLine{}
		}
	}
	m.ent[m.next] = e
	m.next = (m.next + 1) % len(m.ent)
	// Over the byte cap: drop the oldest (from the slot after e).
	for k := range m.ent {
		total += len(m.ent[k].s)
	}
	total -= len(e.s)
	for k := 1; k < len(m.ent) && total > memoMaxBytes; k++ {
		j := (m.next - 1 + k) % len(m.ent)
		total -= len(m.ent[j].s)
		m.ent[j] = memoLine{}
	}
}

// line returns snap.Line(i), snap a snapshot of the Model's Buffer, from
// the Model's memo of long lines.
func (m *Model) line(snap buffer.Snapshot, i int) string { return m.lines.line(m.buf, snap, i) }

// line is Model.line for the synced Buffer's snapshots.
func (vl *viewLines) line(snap buffer.Snapshot, i int) string {
	return vl.memo.line(vl.buf, snap, i)
}

// runMaxBytes is the most text eachLine reads in one slice of the rope.
const runMaxBytes = 256 << 10

// eachLine calls fn with lines lo..hi-1 of snap in order, reading them in
// runs (one slice of the rope for many lines: a Snapshot.Line per line
// walks the tree twice). With long not nil, a run of more than
// runMaxBytes is split in halves until it is one line, and a line of at
// least memoMinLen bytes comes from long (Model.line: the long line a
// keystroke's frame has read already) instead of a copy of it.
func eachLine(snap buffer.Snapshot, lo, hi int, long func(i int) string, fn func(i int, line string)) {
	if lo >= hi {
		return
	}
	start := snap.LineStart(lo)
	end := snap.Len()
	if hi < snap.LineCount() {
		end = snap.LineStart(hi) - 1
	}
	if long != nil && end-start > runMaxBytes {
		if hi-lo == 1 {
			if end-start >= memoMinLen {
				fn(lo, long(lo))
			} else {
				fn(lo, snap.Slice(start, end))
			}
			return
		}
		mid := lo + (hi-lo)/2
		eachLine(snap, lo, mid, long, fn)
		eachLine(snap, mid, hi, long, fn)
		return
	}
	text := snap.Slice(start, end)
	for l := lo; l < hi; l++ {
		line := text
		if i := strings.IndexByte(text, '\n'); i >= 0 {
			line, text = text[:i], text[i+1:]
		}
		fn(l, line)
	}
}
