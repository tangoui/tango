package editor

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/theme"
)

// minimapShiftView is a highlighted Go document with the view scrolled to
// line 300, so the visible miniature rows all lie below an edit at line 200.
func minimapShiftView(t *testing.T) (*State, *ViewW) {
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
	hl.TokenWindow = 1000
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

// TestMinimapReusesRowsShiftedByInsertedLine: a line inserted above the
// minimap rows moves every row down by one line without changing it. The
// rows must be reused, not re-tokenized and re-rasterized (before this was
// fixed the cache was keyed by line number only and every row missed).
func TestMinimapReusesRowsShiftedByInsertedLine(t *testing.T) {
	st, w := minimapShiftView(t)
	at := st.Snapshot().LineStart(200)
	if err := st.Edit([]buffer.Edit{{Start: at, End: at, Text: "\tinserted()\n"}}, Selection{}); err != nil {
		t.Fatal(err)
	}
	snap := st.Snapshot()
	st.Highlighter().HighlightFor(st.hlKey, snap, snap.LineCount(), time.Second)
	m := &w.vs.mini
	rebuilt, bitmaps := m.rebuilt, m.charRendered
	w.prepareMinimap(snap, st.Theme, time.Now().Add(time.Second))
	if got := m.rebuilt - rebuilt; got > 3 {
		t.Fatalf("inserting one line above the minimap rebuilt %d rows; want at most 3", got)
	}
	if got := m.charRendered - bitmaps; got > 3 {
		t.Fatalf("inserting one line above the minimap redrew %d bitmaps; want at most 3", got)
	}
}

// TestMinimapShiftedRowsMatchFreshBuild: the reused rows hold exactly what
// a fresh build of the same snapshot produces (text, start state, tokens,
// blocks, bitmap pixels), so reuse never shows a stale row.
func TestMinimapShiftedRowsMatchFreshBuild(t *testing.T) {
	st, w := minimapShiftView(t)
	at := st.Snapshot().LineStart(200)
	if err := st.Edit([]buffer.Edit{{Start: at, End: at, Text: "\tinserted()\n"}}, Selection{}); err != nil {
		t.Fatal(err)
	}
	snap := st.Snapshot()
	st.Highlighter().HighlightFor(st.hlKey, snap, snap.LineCount(), time.Second)
	w.prepareMinimap(snap, st.Theme, time.Now().Add(time.Second))
	reused := w.vs.mini.cache
	w.vs.mini.cache = nil
	w.prepareMinimap(snap, st.Theme, time.Now().Add(time.Second))
	fresh := w.vs.mini.cache
	if len(reused) != len(fresh) {
		t.Fatalf("reused %d rows, fresh build %d", len(reused), len(fresh))
	}
	for key, f := range fresh {
		r := reused[key]
		if r == nil {
			t.Fatalf("row %d missing from the reused cache", key)
		}
		if r.text != f.text || r.start != f.start || r.final != f.final || r.base != f.base ||
			!slices.Equal(r.tokens, f.tokens) || !slices.Equal(r.blocks, f.blocks) {
			t.Fatalf("row %d differs from a fresh build:\nreused %q %+v\nfresh  %q %+v", key, r.text, r.tokens, f.text, f.tokens)
		}
		if (r.bmp == nil) != (f.bmp == nil) || r.bmp != nil && !slices.Equal(r.bmp.pix, f.bmp.pix) {
			t.Fatalf("row %d bitmap differs from a fresh build", key)
		}
	}
}

// TestMinimapReusesRowsShiftedByDeletedLine: deleting a line above the rows
// moves them up; they are reused too, and match a fresh build.
func TestMinimapReusesRowsShiftedByDeletedLine(t *testing.T) {
	st, w := minimapShiftView(t)
	snap := st.Snapshot()
	a, b := snap.LineStart(200), snap.LineStart(201)
	if err := st.Edit([]buffer.Edit{{Start: a, End: b}}, Selection{}); err != nil {
		t.Fatal(err)
	}
	snap = st.Snapshot()
	st.Highlighter().HighlightFor(st.hlKey, snap, snap.LineCount(), time.Second)
	m := &w.vs.mini
	rebuilt := m.rebuilt
	w.prepareMinimap(snap, st.Theme, time.Now().Add(time.Second))
	if got := m.rebuilt - rebuilt; got > 3 {
		t.Fatalf("deleting one line above the minimap rebuilt %d rows; want at most 3", got)
	}
	assertMinimapFresh(t, st, w)
}

// TestMinimapMovedRowsKeepTheirOwnStartState: identical text on lines in
// different tokenizer states (inside and outside a block comment) must
// not swap rows: a moved row is reused only with the same start state.
func TestMinimapMovedRowsKeepTheirOwnStartState(t *testing.T) {
	goText(t)
	var doc strings.Builder
	for i := range 400 {
		if i%20 == 10 {
			doc.WriteString("/*\n")
		}
		doc.WriteString("same text here\n")
		if i%20 == 14 {
			doc.WriteString("*/\n")
		}
	}
	st := NewState(doc.String(), "dup.go", theme.Base("vs-dark"))
	t.Cleanup(st.Close)
	hl := st.Highlighter()
	hl.TokenWindow = 1000
	hl.HighlightFor(st.hlKey, st.Snapshot(), st.Snapshot().LineCount(), time.Second)
	st.Viewport.TopLine = 200
	d, vw := viewDriver(st, 640, 400, nil)
	for range 3 {
		d.Frame()
	}
	w := *vw
	w.prepareMinimap(st.Snapshot(), st.Theme, time.Now().Add(time.Second))
	for _, edit := range []string{"\n", "/*\n", "*/\n"} {
		at := st.Snapshot().LineStart(150)
		if err := st.Edit([]buffer.Edit{{Start: at, End: at, Text: edit}}, Selection{}); err != nil {
			t.Fatal(err)
		}
		snap := st.Snapshot()
		hl.HighlightFor(st.hlKey, snap, snap.LineCount(), time.Second)
		w.prepareMinimap(snap, st.Theme, time.Now().Add(time.Second))
		assertMinimapFresh(t, st, w)
	}
}

// assertMinimapFresh compares the cache with a fresh build of the current
// snapshot (restoring the incremental cache afterwards).
func assertMinimapFresh(t *testing.T, st *State, w *ViewW) {
	t.Helper()
	snap := st.Snapshot()
	reused := w.vs.mini.cache
	w.vs.mini.cache = nil
	w.prepareMinimap(snap, st.Theme, time.Now().Add(time.Second))
	fresh := w.vs.mini.cache
	defer func() { w.vs.mini.cache = reused }()
	if len(reused) != len(fresh) {
		t.Fatalf("reused %d rows, fresh build %d", len(reused), len(fresh))
	}
	for key, f := range fresh {
		r := reused[key]
		if r == nil {
			t.Fatalf("row %d missing from the reused cache", key)
		}
		if r.text != f.text || r.start != f.start || r.final != f.final || r.base != f.base ||
			!slices.Equal(r.tokens, f.tokens) || !slices.Equal(r.blocks, f.blocks) {
			t.Fatalf("row %d differs from a fresh build:\nreused %q start=%v %+v\nfresh  %q start=%v %+v", key, r.text, r.start, r.tokens, f.text, f.start, f.tokens)
		}
		if (r.bmp == nil) != (f.bmp == nil) || r.bmp != nil && !slices.Equal(r.bmp.pix, f.bmp.pix) {
			t.Fatalf("row %d bitmap differs from a fresh build", key)
		}
	}
}
