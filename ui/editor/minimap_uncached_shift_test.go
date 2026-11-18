package editor

import (
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/theme"
)

// minimapUncachedView is minimapShiftView with a token window of a few
// lines: most miniature rows lie outside it, so their tokens are not
// cached and the rows are tokenized on their own (TokenizeSpan).
func minimapUncachedView(t *testing.T) (*State, *ViewW) {
	t.Helper()
	goText(t)
	var doc strings.Builder
	for i := range 600 {
		doc.WriteString("\tvalue := compute(")
		doc.WriteString(strings.Repeat("a", i%23))
		doc.WriteString(") // line\n")
	}
	st := NewState(doc.String(), "shift.go", theme.Base("vs-dark"))
	t.Cleanup(st.Close)
	hl := st.Highlighter()
	hl.TokenWindow = 4
	hl.HighlightFor(st.hlKey, st.Snapshot(), st.Snapshot().LineCount(), time.Second)
	st.Viewport.TopLine = 300
	d, vw := viewDriver(st, 640, 400, nil)
	for range 3 {
		d.Frame()
	}
	w := *vw
	w.prepareMinimap(st.Snapshot(), st.Theme, time.Now().Add(time.Second))
	if len(w.vs.mini.cache) < 50 {
		t.Fatalf("only %d miniature rows cached", len(w.vs.mini.cache))
	}
	return st, w
}

// TestMinimapReusesUncachedRowsShiftedByLine: a line break typed above
// miniature rows whose tokens are not cached moves them by a line without
// changing them; they are reused, not tokenized again, and match a fresh
// build. (Before, only rows with cached tokens were matched by their
// text, and pressing Enter retokenized every uncached row in view.)
func TestMinimapReusesUncachedRowsShiftedByLine(t *testing.T) {
	st, w := minimapUncachedView(t)
	for _, edit := range []string{"\tinserted()\n", "\n"} {
		at := st.Snapshot().LineStart(200)
		if err := st.Edit([]buffer.Edit{{Start: at, End: at, Text: edit}}, Selection{}); err != nil {
			t.Fatal(err)
		}
		snap := st.Snapshot()
		st.Highlighter().HighlightFor(st.hlKey, snap, snap.LineCount(), time.Second)
		m := &w.vs.mini
		rebuilt := m.rebuilt
		w.prepareMinimap(snap, st.Theme, time.Now().Add(time.Second))
		if got := m.rebuilt - rebuilt; got > 3 {
			t.Fatalf("inserting %q above the minimap rebuilt %d uncached rows; want at most 3", edit, got)
		}
		assertMinimapFresh(t, st, w)
	}
	// Deleting it again moves them back up.
	snap := st.Snapshot()
	a, b := snap.LineStart(200), snap.LineStart(201)
	if err := st.Edit([]buffer.Edit{{Start: a, End: b}}, Selection{}); err != nil {
		t.Fatal(err)
	}
	snap = st.Snapshot()
	st.Highlighter().HighlightFor(st.hlKey, snap, snap.LineCount(), time.Second)
	rebuilt := w.vs.mini.rebuilt
	w.prepareMinimap(snap, st.Theme, time.Now().Add(time.Second))
	if got := w.vs.mini.rebuilt - rebuilt; got > 3 {
		t.Fatalf("deleting a line above the minimap rebuilt %d uncached rows; want at most 3", got)
	}
	assertMinimapFresh(t, st, w)
}
