package buffer

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeClock is an injectable Buffer.Now.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time          { return c.t }
func (c *fakeClock) advance(d time.Duration) { c.t = c.t.Add(d) }
func newClockBuffer(text string) (*Buffer, *fakeClock) {
	b := New(text)
	c := &fakeClock{t: time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)}
	b.Now = c.now
	return b, c
}

func caret(off int) []Selection { return []Selection{{Anchor: off, Head: off}} }

func mustEdit(t *testing.T, b *Buffer, edits []Edit, before, after []Selection) Snapshot {
	t.Helper()
	s, err := b.Edit(edits, before, after)
	if err != nil {
		t.Fatalf("Edit(%v): %v", edits, err)
	}
	if s.Version() != b.Snapshot().Version() || s.String() != b.Snapshot().String() {
		t.Fatalf("Edit returned a snapshot other than Buffer.Snapshot()")
	}
	return s
}

func TestUndoRedo(t *testing.T) {
	b := New("hello")
	if b.Snapshot().String() != "hello" || b.Snapshot().Version() != 1 {
		t.Fatalf("New: %q v%d", b.Snapshot().String(), b.Snapshot().Version())
	}
	if _, _, ok := b.Undo(); ok {
		t.Fatal("Undo on fresh buffer reported ok")
	}
	if _, _, ok := b.Redo(); ok {
		t.Fatal("Redo on fresh buffer reported ok")
	}

	mustEdit(t, b, []Edit{{Start: 5, End: 5, Text: " world"}}, caret(5), caret(11))
	b.PushUndoStop()
	mustEdit(t, b, []Edit{{Start: 0, End: 1, Text: "J"}, {Start: 6, End: 11, Text: "there"}}, caret(1), caret(1))
	b.PushUndoStop()
	if got := b.Snapshot().String(); got != "Jello there" {
		t.Fatalf("after edits: %q", got)
	}

	steps := []string{"hello world", "hello"}
	for _, want := range steps {
		s, _, ok := b.Undo()
		if !ok || s.String() != want || b.Snapshot().String() != want {
			t.Fatalf("Undo: ok=%v %q, want %q", ok, s.String(), want)
		}
		checkInvariants(t, s)
	}
	if _, _, ok := b.Undo(); ok {
		t.Fatal("Undo past the beginning reported ok")
	}
	for _, want := range []string{"hello world", "Jello there"} {
		s, _, ok := b.Redo()
		if !ok || s.String() != want {
			t.Fatalf("Redo: ok=%v %q, want %q", ok, s.String(), want)
		}
	}
	if _, _, ok := b.Redo(); ok {
		t.Fatal("Redo past the end reported ok")
	}

	// A new edit after an undo discards the redo stack.
	b.Undo()
	mustEdit(t, b, []Edit{{Start: 0, End: 0, Text: ">"}}, caret(0), caret(1))
	if _, _, ok := b.Redo(); ok {
		t.Fatal("Redo after a new edit reported ok")
	}
	if got := b.Snapshot().String(); got != ">hello world" {
		t.Fatalf("after branch edit: %q", got)
	}

	// Failed and empty edits leave text and history alone.
	v := b.Snapshot().Version()
	if _, err := b.Edit([]Edit{{Start: 0, End: 100}}, nil, nil); !errors.Is(err, ErrEditRange) {
		t.Fatalf("bad edit err = %v", err)
	}
	if _, err := b.Edit([]Edit{{Start: 100, End: 100}}, nil, nil); !errors.Is(err, ErrEditRange) {
		t.Fatalf("out-of-range no-op edit err = %v", err)
	}
	if s, err := b.Edit([]Edit{{Start: 1, End: 1}}, nil, nil); err != nil || s.Version() != v {
		t.Fatalf("no-op edit: v%d err %v, want v%d", s.Version(), err, v)
	}
	if s, err := b.Edit(nil, nil, nil); err != nil || s.Version() != v {
		t.Fatalf("empty edit: v%d err %v, want v%d", s.Version(), err, v)
	}
	if b.Snapshot().Version() != v {
		t.Fatalf("failed/empty edit changed the version")
	}
	s, _, _ := b.Undo()
	if s.String() != "hello world" {
		t.Fatalf("undo after failed edit: %q", s.String())
	}
}

func TestUndoRestoresSelections(t *testing.T) {
	b := New("ab")
	before := []Selection{{Anchor: 0, Head: 0}, {Anchor: 1, Head: 2}}
	after := []Selection{{Anchor: 1, Head: 1}, {Anchor: 3, Head: 3}}
	mustEdit(t, b, []Edit{{Start: 0, End: 0, Text: "x"}, {Start: 1, End: 2, Text: "y"}}, before, after)
	// The caller's slices are copied.
	before[0].Head, after[0].Head = 99, 99

	_, sel, ok := b.Undo()
	if !ok || !slices.Equal(sel, []Selection{{Anchor: 0, Head: 0}, {Anchor: 1, Head: 2}}) {
		t.Fatalf("Undo selections = %v (ok=%v)", sel, ok)
	}
	_, sel, ok = b.Redo()
	if !ok || !slices.Equal(sel, []Selection{{Anchor: 1, Head: 1}, {Anchor: 3, Head: 3}}) {
		t.Fatalf("Redo selections = %v (ok=%v)", sel, ok)
	}

	// A merged step restores the selections from before its first edit and
	// after its last one.
	b2, clk := newClockBuffer("")
	for i := range 3 {
		mustEdit(t, b2, []Edit{{Start: i, End: i, Text: "a"}}, caret(i), caret(i+1))
		clk.advance(50 * time.Millisecond)
	}
	_, sel, _ = b2.Undo()
	if !slices.Equal(sel, caret(0)) {
		t.Fatalf("merged Undo selections = %v, want %v", sel, caret(0))
	}
	_, sel, _ = b2.Redo()
	if !slices.Equal(sel, caret(3)) {
		t.Fatalf("merged Redo selections = %v, want %v", sel, caret(3))
	}
}

func TestAltVersion(t *testing.T) {
	b := New("x")
	if b.AltVersion() != 1 {
		t.Fatalf("fresh AltVersion = %d", b.AltVersion())
	}
	mustEdit(t, b, []Edit{{Start: 1, End: 1, Text: "y"}}, caret(1), caret(2))
	b.PushUndoStop()
	if v, a := b.Snapshot().Version(), b.AltVersion(); v != 2 || a != 2 {
		t.Fatalf("after edit: version %d alt %d, want 2 2", v, a)
	}
	mustEdit(t, b, []Edit{{Start: 2, End: 2, Text: "z"}}, caret(2), caret(3))
	b.PushUndoStop()

	type state struct {
		version, alt uint64
		text         string
	}
	check := func(what string, want state) {
		t.Helper()
		got := state{b.Snapshot().Version(), b.AltVersion(), b.Snapshot().String()}
		if got != want {
			t.Fatalf("%s: got %+v, want %+v", what, got, want)
		}
	}
	check("two edits", state{3, 3, "xyz"})
	b.Undo()
	check("undo 1", state{4, 2, "xy"})
	b.Undo()
	check("undo 2", state{5, 1, "x"})
	b.Redo()
	check("redo 1", state{6, 2, "xy"})
	b.Redo()
	check("redo 2", state{7, 3, "xyz"})
	b.Undo()
	check("undo 3", state{8, 2, "xy"})

	// A new edit gets a fresh alternative version equal to the version.
	mustEdit(t, b, []Edit{{Start: 0, End: 0, Text: "!"}}, caret(0), caret(1))
	check("new edit", state{9, 9, "!xy"})
	b.Undo()
	check("undo new edit", state{10, 2, "xy"})

	// Merged edits: undo returns to the value before the first one, redo to
	// the value after the last one.
	b2, clk := newClockBuffer("")
	for i := range 3 {
		mustEdit(t, b2, []Edit{{Start: i, End: i, Text: "a"}}, caret(i), caret(i+1))
		clk.advance(10 * time.Millisecond)
	}
	if b2.AltVersion() != 4 {
		t.Fatalf("merged alt = %d, want 4", b2.AltVersion())
	}
	b2.Undo()
	if b2.AltVersion() != 1 || b2.Snapshot().Version() != 5 {
		t.Fatalf("merged undo: alt %d version %d, want 1 5", b2.AltVersion(), b2.Snapshot().Version())
	}
	b2.Redo()
	if b2.AltVersion() != 4 || b2.Snapshot().Version() != 6 {
		t.Fatalf("merged redo: alt %d version %d, want 4 6", b2.AltVersion(), b2.Snapshot().Version())
	}
}

func TestTypingMerges(t *testing.T) {
	typeWord := func(gap time.Duration) (*Buffer, *fakeClock) {
		b, clk := newClockBuffer("")
		for i, c := range "hello" {
			mustEdit(t, b, []Edit{{Start: i, End: i, Text: string(c)}}, caret(i), caret(i+1))
			clk.advance(gap)
		}
		return b, clk
	}

	b, _ := typeWord(100 * time.Millisecond)
	s, _, ok := b.Undo()
	if !ok || s.String() != "" {
		t.Fatalf("100ms typing: one undo left %q", s.String())
	}
	if _, _, ok := b.Undo(); ok {
		t.Fatal("100ms typing: more than one undo step")
	}

	// 400 ms gap between two bursts of typing: two steps.
	b, clk := newClockBuffer("")
	for i, c := range "ab" {
		mustEdit(t, b, []Edit{{Start: i, End: i, Text: string(c)}}, caret(i), caret(i+1))
		clk.advance(100 * time.Millisecond)
	}
	clk.advance(300 * time.Millisecond) // 400 ms since the last keystroke
	for i, c := range "cd" {
		mustEdit(t, b, []Edit{{Start: 2 + i, End: 2 + i, Text: string(c)}}, caret(2+i), caret(3+i))
		clk.advance(100 * time.Millisecond)
	}
	if s, _, _ := b.Undo(); s.String() != "ab" {
		t.Fatalf("400ms gap: first undo left %q, want \"ab\"", s.String())
	}
	if s, _, _ := b.Undo(); s.String() != "" {
		t.Fatalf("400ms gap: second undo left %q, want \"\"", s.String())
	}

	// 400 ms between every keystroke: each one is its own step.
	b, _ = typeWord(400 * time.Millisecond)
	for _, want := range []string{"hell", "hel", "he", "h", ""} {
		if s, _, _ := b.Undo(); s.String() != want {
			t.Fatalf("400ms typing: undo left %q, want %q", s.String(), want)
		}
	}

	// A cursor jump breaks the merge even within 300 ms.
	b, clk = newClockBuffer("")
	mustEdit(t, b, []Edit{{Start: 0, End: 0, Text: "ab"}}, caret(0), caret(2))
	clk.advance(10 * time.Millisecond)
	mustEdit(t, b, []Edit{{Start: 0, End: 0, Text: "c"}}, caret(0), caret(1))
	if s, _, _ := b.Undo(); s.String() != "ab" {
		t.Fatalf("cursor jump: undo left %q, want \"ab\"", s.String())
	}

	// PushUndoStop always ends a step, however fast the typing.
	b, _ = newClockBuffer("")
	mustEdit(t, b, []Edit{{Start: 0, End: 0, Text: "a"}}, caret(0), caret(1))
	b.PushUndoStop()
	mustEdit(t, b, []Edit{{Start: 1, End: 1, Text: "b"}}, caret(1), caret(2))
	if s, _, _ := b.Undo(); s.String() != "a" {
		t.Fatalf("explicit stop: undo left %q, want \"a\"", s.String())
	}

	// Edits between two PushUndoStop calls form one step, whatever their
	// timing and cursors.
	b, clk = newClockBuffer("")
	b.PushUndoStop()
	mustEdit(t, b, []Edit{{Start: 0, End: 0, Text: "a"}}, caret(0), caret(1))
	clk.advance(time.Second)
	mustEdit(t, b, []Edit{{Start: 0, End: 0, Text: "b"}}, caret(0), caret(1))
	b.PushUndoStop()
	mustEdit(t, b, []Edit{{Start: 2, End: 2, Text: "c"}}, caret(2), caret(3))
	b.PushUndoStop()
	for _, want := range []string{"ba", ""} {
		if s, _, _ := b.Undo(); s.String() != want {
			t.Fatalf("explicit group: undo left %q, want %q", s.String(), want)
		}
	}
}

func TestAnchorBias(t *testing.T) {
	b := New("0123456789")
	left := b.Anchor(4, Left)
	right := b.Anchor(4, Right)
	before := b.Anchor(2, Right)
	after := b.Anchor(7, Left)
	if left.Bias() != Left || right.Bias() != Right {
		t.Fatal("Bias accessor")
	}

	// Insert at the anchor: Left stays, Right moves after the insertion.
	mustEdit(t, b, []Edit{{Start: 4, End: 4, Text: "abc"}}, nil, nil)
	if got := b.Resolve(left); got != 4 {
		t.Errorf("Left after insert at anchor = %d, want 4", got)
	}
	if got := b.Resolve(right); got != 7 {
		t.Errorf("Right after insert at anchor = %d, want 7", got)
	}
	if got := b.Resolve(before); got != 2 {
		t.Errorf("anchor before insert = %d, want 2", got)
	}
	if got := b.Resolve(after); got != 10 {
		t.Errorf("anchor after insert = %d, want 10", got)
	}

	// Delete a range containing an anchor: it moves to the deletion start.
	// Text is now "0123abc456789"; delete [3,9) = "3abc45".
	mid := b.Anchor(5, Right)
	mustEdit(t, b, []Edit{{Start: 3, End: 9}}, nil, nil)
	if got := b.Snapshot().String(); got != "0126789" {
		t.Fatalf("text = %q", got)
	}
	for name, a := range map[string]Anchor{"left": left, "right": right, "mid": mid} {
		if got := b.Resolve(a); got != 3 {
			t.Errorf("%s anchor inside deletion = %d, want 3", name, got)
		}
	}
	if got := b.Resolve(after); got != 4 {
		t.Errorf("anchor after deletion = %d, want 4", got)
	}

	// Several inserts at one offset in one Edit: Left stays before all of
	// them, Right moves after all of them.
	b = New("ab")
	l, r := b.Anchor(1, Left), b.Anchor(1, Right)
	mustEdit(t, b, []Edit{{Start: 1, End: 1, Text: "xx"}, {Start: 1, End: 1, Text: "yyy"}}, nil, nil)
	if b.Resolve(l) != 1 || b.Resolve(r) != 6 {
		t.Errorf("multi insert: left %d right %d, want 1 6", b.Resolve(l), b.Resolve(r))
	}

	// Replacing a range that starts at the anchor keeps it at the start. At
	// the end of a replaced range that grows (Monaco's nodeAcceptEdit): a
	// Left anchor stays at its offset (inside the new text), a Right anchor
	// lands after the new text.
	b = New("abcdef")
	start, startR, end, endR := b.Anchor(2, Left), b.Anchor(2, Right), b.Anchor(4, Left), b.Anchor(4, Right)
	mustEdit(t, b, []Edit{{Start: 2, End: 4, Text: "XYZ"}}, nil, nil)
	if b.Resolve(start) != 2 || b.Resolve(startR) != 2 || b.Resolve(end) != 4 || b.Resolve(endR) != 5 {
		t.Errorf("replace: start %d startR %d end %d endR %d, want 2 2 4 5",
			b.Resolve(start), b.Resolve(startR), b.Resolve(end), b.Resolve(endR))
	}

	// Anchors use the inserted length after EOL normalisation.
	b = New("ab")
	r = b.Anchor(1, Right)
	mustEdit(t, b, []Edit{{Start: 1, End: 1, Text: "x\r\ny"}}, nil, nil)
	if got := b.Resolve(r); got != 4 {
		t.Errorf("CRLF insert: right anchor %d, want 4", got)
	}

	// Undo and redo move anchors like the edits they replay.
	b = New("hello")
	r = b.Anchor(5, Right)
	mustEdit(t, b, []Edit{{Start: 0, End: 0, Text: ">> "}}, nil, nil)
	if b.Resolve(r) != 8 {
		t.Fatalf("before undo: %d", b.Resolve(r))
	}
	b.Undo()
	if got := b.Resolve(r); got != 5 {
		t.Errorf("after undo: %d, want 5", got)
	}
	b.Redo()
	if got := b.Resolve(r); got != 8 {
		t.Errorf("after redo: %d, want 8", got)
	}
	// An anchor created inside text that undo removes collapses to the
	// removal point.
	in := b.Anchor(2, Left)
	b.Undo()
	if got := b.Resolve(in); got != 0 {
		t.Errorf("anchor inside undone insert: %d, want 0", got)
	}

	// Anchor offsets clamp into the document.
	b = New("abc")
	if b.Resolve(b.Anchor(-5, Left)) != 0 || b.Resolve(b.Anchor(99, Right)) != 3 {
		t.Error("Anchor does not clamp")
	}
}

// TestStickiness expresses Monaco's four TrackedRangeStickiness modes as
// (start bias, end bias) pairs and checks typing at both edges of a range.
func TestStickiness(t *testing.T) {
	cases := []struct {
		name               string
		startBias, endBias Bias
		atStart, atEnd     [2]int // resolved [start,end) after typing at the start / end
	}{
		{"AlwaysGrowsWhenTypingAtEdges", Left, Right, [2]int{2, 5}, [2]int{2, 5}},
		{"NeverGrowsWhenTypingAtEdges", Right, Left, [2]int{3, 5}, [2]int{2, 4}},
		{"GrowsOnlyWhenTypingBefore", Left, Left, [2]int{2, 5}, [2]int{2, 4}},
		{"GrowsOnlyWhenTypingAfter", Right, Right, [2]int{3, 5}, [2]int{2, 5}},
	}
	for _, c := range cases {
		for i, at := range []int{2, 4} {
			b := New("abcdef")
			s, e := b.Anchor(2, c.startBias), b.Anchor(4, c.endBias)
			mustEdit(t, b, []Edit{{Start: at, End: at, Text: "x"}}, nil, nil)
			want := c.atStart
			if i == 1 {
				want = c.atEnd
			}
			if got := [2]int{b.Resolve(s), b.Resolve(e)}; got != want {
				t.Errorf("%s typing at %d: range %v, want %v", c.name, at, got, want)
			}
		}
	}
}

// TestBufferRandomHistory keeps every Nth snapshot during a long random
// multi-leaf edit sequence, re-verifies them all at the end (old snapshots
// must survive later edits that share their nodes) and then undoes back
// through the history checking each kept state.
func TestBufferRandomHistory(t *testing.T) {
	r := rand.New(rand.NewPCG(7, 11))
	var init strings.Builder
	for init.Len() < 20_000 {
		init.WriteString(randPieces[r.IntN(len(randPieces))])
	}
	b := New(init.String())
	model := modelEOL.Replace(init.String())

	const steps, every = 3_000, 25
	type kept struct {
		step int
		snap Snapshot
		text string
	}
	var keep []kept
	var texts []string // text before each recorded undo step
	// Anchors must stay inside the document and on character boundaries
	// through every edit, undo and redo.
	anchors := []Anchor{b.Anchor(0, Left), b.Anchor(b.Snapshot().Len(), Right)}
	for range 20 {
		anchors = append(anchors, b.Anchor(runeFloor(model, r.IntN(len(model)+1)), Bias(r.IntN(2))))
	}
	checkAnchors := func(what string) {
		t.Helper()
		s := b.Snapshot()
		for i, a := range anchors {
			off := b.Resolve(a)
			if off < 0 || off > s.Len() || s.splitsRune(off) {
				t.Fatalf("%s: anchor %d resolved to bad offset %d (len %d)", what, i, off, s.Len())
			}
		}
		if got := b.Resolve(anchors[0]); got != 0 {
			t.Fatalf("%s: start anchor at %d", what, got)
		}
		if got := b.Resolve(anchors[1]); got != s.Len() {
			t.Fatalf("%s: end anchor at %d, want %d", what, got, s.Len())
		}
	}
	for step := range steps {
		before := model
		n := 1 + r.IntN(3)
		offs := make([]int, 2*n)
		for i := range offs {
			offs[i] = runeFloor(model, r.IntN(len(model)+1))
		}
		sortInts(offs)
		var edits []Edit
		for i := range n {
			start, end := offs[2*i], offs[2*i+1]
			if len(model) > 40_000 || end-start > 2_000 {
				end = runeFloor(model, start+r.IntN(min(end-start, 2_000)+1))
			}
			edits = append(edits, Edit{Start: start, End: end, Text: randText(r)})
		}
		v := b.Snapshot().Version()
		s := mustEdit(t, b, edits, nil, nil)
		b.PushUndoStop()
		if s.Version() != v { // all-no-op edits record no undo step
			texts = append(texts, before)
		}
		for i := len(edits) - 1; i >= 0; i-- {
			e := edits[i]
			model = model[:e.Start] + modelEOL.Replace(e.Text) + model[e.End:]
		}
		if step%every == 0 {
			checkAnchors("edit")
			keep = append(keep, kept{step, s, model})
		}
	}
	if b.Snapshot().String() != model {
		t.Fatal("final text mismatch")
	}
	for _, k := range keep {
		if k.snap.String() != k.text {
			t.Fatalf("kept snapshot from step %d changed", k.step)
		}
		checkInvariants(t, k.snap)
	}
	if len(texts) < steps*9/10 {
		t.Fatalf("only %d of %d steps recorded", len(texts), steps)
	}
	for i := len(texts) - 1; i >= 0; i-- {
		s, _, ok := b.Undo()
		if !ok {
			t.Fatalf("undo %d failed", i)
		}
		if i%every == 0 || i < 10 {
			checkAnchors("undo")
			if s.String() != texts[i] {
				t.Fatalf("undo to before recorded step %d: text mismatch", i)
			}
		}
	}
	if _, _, ok := b.Undo(); ok {
		t.Fatal("undo past the initial text reported ok")
	}
	if b.Snapshot().String() != texts[0] {
		t.Fatal("full undo did not restore the initial text")
	}
	if b.AltVersion() != 1 {
		t.Fatalf("full undo AltVersion = %d, want 1", b.AltVersion())
	}
}

// TestConcurrentReaders reads snapshots from other goroutines while the
// Buffer keeps editing; run with -race.
func TestConcurrentReaders(t *testing.T) {
	for _, err := range concurrentReaders(readWholeSnapshot) {
		t.Error(err)
	}
}

// TestConcurrentReadersDrainsOnError checks the harness itself: readers that
// hit an error must keep draining the channel, so a failing check reports
// errors instead of blocking the editing goroutine forever.
func TestConcurrentReadersDrainsOnError(t *testing.T) {
	done := make(chan []error, 1)
	go func() {
		done <- concurrentReaders(func(s Snapshot) error { return fmt.Errorf("v%d: injected", s.Version()) })
	}()
	select {
	case errs := <-done:
		if len(errs) != concurrentReaderSnaps {
			t.Fatalf("%d errors, want one per snapshot (%d)", len(errs), concurrentReaderSnaps)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("concurrentReaders hung after reader errors")
	}
}

// readWholeSnapshot reads s line by line twice and checks it did not change.
func readWholeSnapshot(s Snapshot) error {
	want := s.String()
	for range 2 {
		var sb strings.Builder
		for i := range s.LineCount() {
			sb.WriteString(s.Line(i))
			if i+1 < s.LineCount() {
				sb.WriteByte('\n')
			}
			_ = s.UTF16Col(i, 1<<20)
		}
		if sb.String() != want {
			return fmt.Errorf("snapshot v%d changed while being read", s.Version())
		}
	}
	return nil
}

const concurrentReaderSnaps = 60 // snapshots sent by concurrentReaders (every 5th of 300 edits)

// concurrentReaders hands snapshots to 4 reader goroutines running check while
// the Buffer keeps editing (run with -race), and returns the check errors.
// Readers never stop early: after an error they keep draining the channel so
// the editing loop cannot block on a full channel.
func concurrentReaders(check func(Snapshot) error) []error {
	var init strings.Builder
	for i := range 2_000 {
		init.WriteString("line ")
		init.WriteString(strings.Repeat("中", i%7))
		init.WriteString("\n")
	}
	b := New(init.String())
	snaps := make(chan Snapshot, 8)
	var (
		wg   sync.WaitGroup
		mu   sync.Mutex
		errs []error
	)
	for range 4 {
		wg.Go(func() {
			for s := range snaps {
				if err := check(s); err != nil {
					mu.Lock()
					errs = append(errs, err)
					mu.Unlock()
					continue // keep draining
				}
			}
		})
	}
	r := rand.New(rand.NewPCG(3, 4))
	for i := range 300 {
		s := b.Snapshot()
		off := s.LineStart(r.IntN(s.LineCount()))
		end := off
		if r.IntN(2) == 0 && off < s.Len() {
			end = s.LineStart(s.LineOf(off)+1) - 1
		}
		if _, err := b.Edit([]Edit{{Start: off, End: end, Text: strings.Repeat("x😀\n", r.IntN(100))}}, nil, nil); err != nil {
			mu.Lock()
			errs = append(errs, err)
			mu.Unlock()
		}
		b.PushUndoStop()
		if i%5 == 0 {
			snaps <- b.Snapshot()
		}
		if i%7 == 0 {
			b.Undo()
		}
	}
	close(snaps)
	wg.Wait()
	return errs
}
