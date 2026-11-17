package editor

import (
	"math/rand/v2"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
)

// TestMinimapSkipsSourceOfUntouchedRows: a frame reads the source text of
// a miniature row again only when its line changed: none on a frame
// without an edit, the edited line's alone after typing a letter.
func TestMinimapSkipsSourceOfUntouchedRows(t *testing.T) {
	st, w := minimapShiftView(t)
	m := &w.vs.mini
	reads := m.sourceReads
	w.prepareMinimap(st.Snapshot(), st.Theme, time.Now().Add(time.Second))
	if got := m.sourceReads - reads; got != 0 {
		t.Fatalf("a frame without an edit read %d rows' source, want 0", got)
	}
	for k := range 3 {
		snap := st.Snapshot()
		at := snap.LineStart(320+k) + 3
		if err := st.Edit([]buffer.Edit{{Start: at, End: at, Text: "z"}}, Selection{}); err != nil {
			t.Fatal(err)
		}
		snap = st.Snapshot()
		st.Highlighter().HighlightFor(st.hlKey, snap, snap.LineCount(), time.Second)
		reads = m.sourceReads
		w.prepareMinimap(snap, st.Theme, time.Now().Add(time.Second))
		if got := m.sourceReads - reads; got > 1 {
			t.Fatalf("typing a letter on line %d read %d rows' source, want at most 1", 320+k, got)
		}
		assertMinimapFresh(t, st, w)
	}
}

// TestMinimapSourceCarryMatchesFresh: after every kind of edit (typing,
// brackets, comment openers, line breaks added and removed, several edits
// at once, undo and redo) the rows equal a fresh build, and some frames
// skipped the untouched rows' source.
func TestMinimapSourceCarryMatchesFresh(t *testing.T) {
	st, w := minimapShiftView(t)
	m := &w.vs.mini
	r := rand.New(rand.NewPCG(7, 11))
	inserts := []string{"z", "(", ")", "\"", "/*", "*/", "//", "\n", "\t", "  "}
	skipped := 0
	for step := range 150 {
		snap := st.Snapshot()
		line := 290 + r.IntN(60)
		at := snap.LineStart(line) + r.IntN(len(snap.Line(line))+1)
		var edits []buffer.Edit
		switch step % 5 {
		case 0, 1:
			edits = []buffer.Edit{{Start: at, End: at, Text: inserts[r.IntN(len(inserts))]}}
		case 2:
			if at < snap.Len() {
				edits = []buffer.Edit{{Start: at, End: at + 1}}
			}
		case 3:
			a := snap.LineStart(line - 7)
			edits = []buffer.Edit{{Start: a, End: a, Text: inserts[r.IntN(len(inserts))]}, {Start: at, End: at, Text: "q"}}
		case 4:
			if r.IntN(2) == 0 {
				st.Undo()
			} else {
				st.Redo()
			}
		}
		if len(edits) > 0 {
			if err := st.Edit(edits, Selection{}); err != nil {
				continue
			}
		}
		snap = st.Snapshot()
		st.Highlighter().HighlightFor(st.hlKey, snap, snap.LineCount(), time.Second)
		reads := m.sourceReads
		w.prepareMinimap(snap, st.Theme, time.Now().Add(time.Second))
		if m.sourceReads-reads < uint64(len(m.cache))/2 {
			skipped++
		}
		assertMinimapFresh(t, st, w)
	}
	if skipped == 0 {
		t.Fatal("no frame skipped the rows' source: the test does not exercise the carry")
	}
}

// TestMinimapSourceCarryLinesMovedInPlace: one change that breaks a line
// and joins two lines further down keeps the line count but moves every
// line between them; their rows are read again, not carried.
func TestMinimapSourceCarryLinesMovedInPlace(t *testing.T) {
	st, w := minimapShiftView(t)
	snap := st.Snapshot()
	a := snap.LineStart(305) + 4
	b := snap.LineStart(340) - 1 // the line break ending line 339
	if err := st.Edit([]buffer.Edit{{Start: a, End: a, Text: "\n"}, {Start: b, End: b + 1}}, Selection{}); err != nil {
		t.Fatal(err)
	}
	if st.Snapshot().LineCount() != snap.LineCount() {
		t.Fatal("the change altered the line count")
	}
	snap = st.Snapshot()
	st.Highlighter().HighlightFor(st.hlKey, snap, snap.LineCount(), time.Second)
	w.prepareMinimap(snap, st.Theme, time.Now().Add(time.Second))
	assertMinimapFresh(t, st, w)
}
