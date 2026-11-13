package highlight

import (
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
)

const pathoGrammar = `{"tokenPostfix":"","tokenizer":{"root":[
	["x","x"],
	["\\{",{"token":"open","next":"@inner"}],
	["\\}","close"]
],"inner":[["x","x"],["(a+)+(?=b)","a"],["\\}",{"token":"close","next":"@pop"}]]}}`

// pathoLine degrades under pathoGrammar's inner state.
var pathoLine = "xx" + strings.Repeat("a", 200) + "c"

// B1: Tokens re-tokenizing a line whose slot was dropped may degrade (a
// wall-clock budget). The result is cached (repeated calls do not pay the
// budget again), is not final, and goes through the normal retry flow: once
// the retries are used up it is accepted and the line becomes final.
func TestTokensDegradedRecomputeNotFinal(t *testing.T) {
	lx := grammarLexer(t, "patho", pathoGrammar)
	s := buffer.NewSnapshot("{\n" + pathoLine + "\n}\nx")
	h := New(lx)
	h.TokenWindow = 0 // only the focused line keeps tokens
	h.RetryDelay = 0
	h.MaxRetries = 1
	highlightAll(t, h, s) // degrades, retries once, accepts
	if _, final := h.Tokens(s, 3, nil); !final {
		t.Fatal("line 3 not final after accepting the degraded line")
	}
	// Simulate a line that tokenized cleanly in Highlight (degrading is a
	// wall-clock effect): forget its retry record. Line 1's slot was dropped
	// when the window moved to line 3; Tokens recomputes it and the
	// recompute degrades.
	h.mu.Lock()
	h.st.deg = h.st.deg[:0]
	h.mu.Unlock()
	n0 := h.tokenized.Load()
	for range 3 {
		if _, final := h.Tokens(s, 1, nil); final {
			t.Fatal("degraded recompute reported final")
		}
	}
	if d := h.tokenized.Load() - n0; d != 1 {
		t.Fatalf("tokenized %d times, want 1 (degraded result cached)", d)
	}
	// The retry is due at once; it degrades again, the retries are used up
	// and the result is accepted.
	if !h.Highlight(s, s.LineCount(), time.Hour) {
		t.Fatal("not ready after the retry")
	}
	if _, final := h.Tokens(s, 1, nil); !final {
		t.Fatal("line 1 not final after its retries were used up")
	}
	// An accepted line stays final when its slot is dropped and recomputed
	// (degrading again): no new retry cycle.
	h.Tokens(s, 3, nil)
	if _, final := h.Tokens(s, 1, nil); !final {
		t.Fatal("accepted degraded line lost final after a window move")
	}
	if h.st.minPending() != h.st.n() {
		t.Fatal("accepted degraded line queued for retry again")
	}
}

// B2: a background batch whose lines the foreground already tokenized in the
// same generation must not apply them again (that would count a degraded
// line's retry twice).
func TestBackgroundSkipsLinesForegroundDid(t *testing.T) {
	lx := grammarLexer(t, "patho", pathoGrammar)
	s := buffer.NewSnapshot("{\n" + pathoLine + "\n}\nx")
	h := New(lx)
	h.RetryDelay = time.Hour
	h.MaxRetries = 3
	h.mu.Lock()
	h.resync(s)
	h.mu.Unlock()
	testHookBatchTokenized = func() {
		h.Highlight(s, s.LineCount(), time.Hour)
	}
	defer func() { testHookBatchTokenized = nil }()
	res := h.batch()
	if res.applied != 0 {
		t.Fatalf("background applied %d lines the foreground already did", res.applied)
	}
	if len(h.st.deg) != 1 || h.st.deg[0].attempts != 1 {
		t.Fatalf("degraded records %+v, want line 1 with 1 attempt", h.st.deg)
	}
}

// B3: an edit inside the token window that keeps the line count does not
// reallocate the window.
func TestEditInWindowNoAlloc(t *testing.T) {
	if raceEnabled {
		t.Skip("allocation counts are not meaningful under -race")
	}
	lx := tsLexer(t)
	s := buffer.NewSnapshot(tsDoc(t, 10000))
	h := New(lx)
	h.Highlight(s, 100, time.Hour)
	off := s.LineStart(50)
	var snaps []buffer.Snapshot
	var edits [][]buffer.Edit
	cur := s
	for range 20 {
		e := []buffer.Edit{{Start: off, End: off, Text: "x"}}
		ns, err := cur.Apply(e)
		if err != nil {
			t.Fatal(err)
		}
		snaps = append(snaps, ns)
		edits = append(edits, e)
		cur = ns
	}
	prev, i := s, 0
	h.Edit(prev, snaps[0], edits[0]) // warm up the region buffer
	prev, i = snaps[0], 1
	allocs := testing.AllocsPerRun(10, func() {
		h.Edit(prev, snaps[i], edits[i])
		prev = snaps[i]
		i++
	})
	if allocs != 0 {
		t.Fatalf("Edit inside the window: %v allocs, want 0", allocs)
	}
}
