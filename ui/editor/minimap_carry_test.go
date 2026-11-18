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

// TestMinimapSkipsSourceOfShiftedRows: a line break added or removed above
// the miniature rows moves them by a line; their rows are carried to the
// new lines without reading their source again (only the lines the edit
// touched are read), and match a fresh build.
func TestMinimapSkipsSourceOfShiftedRows(t *testing.T) {
	st, w := minimapShiftView(t)
	m := &w.vs.mini
	for _, c := range []struct {
		name  string
		edits func(s buffer.Snapshot) []buffer.Edit
	}{
		{"insert a line", func(s buffer.Snapshot) []buffer.Edit {
			a := s.LineStart(200)
			return []buffer.Edit{{Start: a, End: a, Text: "\tinserted()\n"}}
		}},
		{"Enter at a line end", func(s buffer.Snapshot) []buffer.Edit {
			a := s.LineStart(201) - 1
			return []buffer.Edit{{Start: a, End: a, Text: "\n\t"}}
		}},
		{"join two lines", func(s buffer.Snapshot) []buffer.Edit {
			a := s.LineStart(201) - 1
			return []buffer.Edit{{Start: a, End: a + 1}}
		}},
		{"delete two lines", func(s buffer.Snapshot) []buffer.Edit {
			return []buffer.Edit{{Start: s.LineStart(150), End: s.LineStart(152)}}
		}},
		{"two edits", func(s buffer.Snapshot) []buffer.Edit {
			a, b := s.LineStart(100), s.LineStart(250)
			return []buffer.Edit{{Start: a, End: a, Text: "\n\n"}, {Start: b, End: b, Text: "x\n"}}
		}},
	} {
		if err := st.Edit(c.edits(st.Snapshot()), Selection{}); err != nil {
			t.Fatal(err)
		}
		snap := st.Snapshot()
		st.Highlighter().HighlightFor(st.hlKey, snap, snap.LineCount(), time.Second)
		reads := m.sourceReads
		w.prepareMinimap(snap, st.Theme, time.Now().Add(time.Second))
		if got := m.sourceReads - reads; got > 4 {
			t.Fatalf("%s: read %d rows' source, want at most 4", c.name, got)
		}
		assertMinimapFresh(t, st, w)
	}
}

// TestMinimapShiftAfterTwoEdits: two changes before a frame (a letter
// typed on a line, then a line inserted above it): the last change alone
// does not say the typed line changed, but its row was made before the
// first change, so it is read again, not carried.
func TestMinimapShiftAfterTwoEdits(t *testing.T) {
	st, w := minimapShiftView(t)
	snap := st.Snapshot()
	at := snap.LineStart(320) + 3
	if err := st.Edit([]buffer.Edit{{Start: at, End: at, Text: "TYPED"}}, Selection{}); err != nil {
		t.Fatal(err)
	}
	a := st.Snapshot().LineStart(200)
	if err := st.Edit([]buffer.Edit{{Start: a, End: a, Text: "\tinserted()\n"}}, Selection{}); err != nil {
		t.Fatal(err)
	}
	snap = st.Snapshot()
	st.Highlighter().HighlightFor(st.hlKey, snap, snap.LineCount(), time.Second)
	w.prepareMinimap(snap, st.Theme, time.Now().Add(time.Second))
	assertMinimapFresh(t, st, w)
}
