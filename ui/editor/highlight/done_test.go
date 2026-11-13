package highlight

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
)

// Done is a pure query: it reports full highlighting of the current
// snapshot without tokenizing, moving the token window or adopting an
// unseen snapshot.
func TestDone(t *testing.T) {
	if !New(nil).Done(buffer.NewSnapshot("x")) {
		t.Fatal("plain text not done")
	}
	h := New(tsLexer(t))
	s := buffer.NewSnapshot(tsDoc(t, 5000))
	if h.Done(s) {
		t.Fatal("done before any Highlight")
	}
	if h.have {
		t.Fatal("Done adopted the snapshot")
	}
	h.Highlight(s, 100, time.Hour)
	winFocus := func() int {
		if w := h.st.window(DefaultView); w != nil {
			return w.focus
		}
		return -1
	}
	focus, n0 := winFocus(), h.tokenized.Load()
	if h.Done(s) {
		t.Fatal("done with 4900 lines left")
	}
	if winFocus() != focus || h.tokenized.Load() != n0 {
		t.Fatalf("Done moved the window (focus %d -> %d) or tokenized (%d -> %d)", focus, winFocus(), n0, h.tokenized.Load())
	}
	highlightAll(t, h, s)
	h.Highlight(s, 100, time.Hour) // window back at the top
	focus = winFocus()
	if !h.Done(s) {
		t.Fatal("not done after highlighting everything")
	}
	if winFocus() != focus {
		t.Fatalf("Done moved the window: focus %d -> %d", focus, winFocus())
	}
	edits := []buffer.Edit{{Start: s.Len(), End: s.Len(), Text: "\nlet x = 1;"}}
	ns, err := s.Apply(edits)
	if err != nil {
		t.Fatal(err)
	}
	if h.Done(ns) {
		t.Fatal("unseen snapshot reported done")
	}
	h.Edit(s, ns, edits)
	if h.Done(ns) {
		t.Fatal("done right after an edit")
	}
	if h.Done(s) {
		t.Fatal("old snapshot reported done after an edit")
	}
	highlightAll(t, h, ns)
	if !h.Done(ns) {
		t.Fatal("not done after re-highlighting the edit")
	}
}
