package editor

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
)

// The package's tests scan without a time budget unless they set a clock
// (see scanClock): whether a search finishes in the one frame a test
// expects must not depend on the wall clock.
func init() {
	stopped := time.Unix(0, 0)
	scanClock = func() time.Time { return stopped }
}

// fakeScanClock is a clock that moves by tick at every reading.
type fakeScanClock struct {
	t     time.Time
	tick  time.Duration
	reads int
}

func (f *fakeScanClock) now() time.Time {
	f.reads++
	f.t = f.t.Add(f.tick)
	return f.t
}

// oneShotMatches is the reference: the document's first maxFindMatches
// matches, scanned in one go.
func oneShotMatches(snap buffer.Snapshot, q *selQuery) []brange {
	var out, scratch []brange
	for line := 0; line < snap.LineCount() && len(out) < maxFindMatches; line++ {
		scratch = q.matchesLimit(snap.Line(line), scratch[:0], maxFindMatches-len(out))
		ls := snap.LineStart(line)
		for _, m := range scratch {
			out = append(out, brange{ls + m.a, ls + m.b})
		}
	}
	return out
}

// rareDoc is a document under 20 MB whose word "needle" is only on its
// last line.
func rareDoc() string {
	line := strings.Repeat("lorem ipsum dolor sit amet ", 3) + "\n" // 82 bytes
	return strings.Repeat(line, 200_000) + "the needle is here\n"
}

// TestDocumentMatchesResumable: a rare word at the end of a large document
// (under 20 MB): the first step returns within its budget with the visible
// line's match already there; stepping on until done gives the one-shot
// scan's result.
func TestDocumentMatchesResumable(t *testing.T) {
	buf := buffer.New(rareDoc())
	snap := buf.Snapshot()
	if snap.Len() >= 20<<20 {
		t.Fatalf("document of %d bytes", snap.Len())
	}
	q := &selQuery{text: "needle", whole: true}
	last := snap.LineCount() - 2
	want := oneShotMatches(snap, q)
	if len(want) != 1 {
		t.Fatalf("reference: %v", want)
	}
	// Wall time, retried for load spikes: the first frame's step must not
	// block the frame (16 ms, spec 6.6).
	var elapsed time.Duration
	for try := 0; try < 3; try++ {
		c := &documentMatches{now: time.Now}
		start := time.Now()
		done := c.step(buf, snap, q, start.Add(docScanBudget))
		elapsed = time.Since(start)
		if done {
			t.Fatal("a 16 MB scan finished in one frame's budget")
		}
		if elapsed <= 16*time.Millisecond {
			break
		}
	}
	if elapsed > 16*time.Millisecond {
		t.Fatalf("first step took %v", elapsed)
	}
	// Deterministic: with a fake clock, the visible line is matched in the
	// first frame although the scan has not reached it.
	fc := &fakeScanClock{t: time.Unix(0, 0), tick: time.Millisecond}
	c := &documentMatches{now: fc.now}
	ms, done := c.find(buf, snap, q, fc.now().Add(docScanBudget), []int{last - 1, last})
	if done || c.cur.next > last {
		t.Fatalf("scan done in the first frame (next %d)", c.cur.next)
	}
	if !slices.Equal(ms, want) {
		t.Fatalf("first frame: %v, want the visible match %v", ms, want)
	}
	frames := 1
	for !done {
		ms, done = c.find(buf, snap, q, fc.now().Add(docScanBudget), []int{last - 1, last})
		frames++
		if frames > 10_000 {
			t.Fatal("scan does not finish")
		}
	}
	if !slices.Equal(ms, want) || !slices.Equal(c.cur.ranges, want) {
		t.Fatalf("done: %v, want %v", ms, want)
	}
	if c.scans != 1 {
		t.Fatalf("%d scans, want 1", c.scans)
	}
	t.Logf("%d frames", frames)

	// The quota: the result equals the one-shot scan's first 999, and a
	// visible match past them goes when the scan establishes that.
	text := strings.Repeat("ab\n", 3000)
	qb := &selQuery{text: "ab", whole: true}
	buf = buffer.New(text)
	snap = buf.Snapshot()
	fc = &fakeScanClock{t: time.Unix(0, 0), tick: time.Millisecond}
	c = &documentMatches{now: fc.now}
	c.chunk = 64 // a few lines per clock reading
	ms, done = c.find(buf, snap, qb, fc.now().Add(docScanBudget), []int{2500})
	if done || len(ms) == 0 || ms[len(ms)-1].a != 2500*3 {
		t.Fatalf("provisional visible match missing: done %v, %d ranges", done, len(ms))
	}
	for !done {
		ms, done = c.find(buf, snap, qb, fc.now().Add(docScanBudget), []int{2500})
	}
	if !slices.Equal(ms, oneShotMatches(snap, qb)) || len(ms) != maxFindMatches {
		t.Fatalf("done: %d ranges, want the first %d", len(ms), maxFindMatches)
	}
}

// TestDocumentMatchesEditCancels: an edit during a scan abandons it; the
// new version is scanned from its start, and none of the old version's
// ranges appear in its result.
func TestDocumentMatchesEditCancels(t *testing.T) {
	text := strings.Repeat("xx foo\n", 5000)
	buf := buffer.New(text)
	snap := buf.Snapshot()
	q := &selQuery{text: "foo", whole: true}
	fc := &fakeScanClock{t: time.Unix(0, 0), tick: time.Millisecond}
	c := &documentMatches{now: fc.now}
	c.chunk = 256
	if c.step(buf, snap, q, fc.now().Add(docScanBudget)) {
		t.Fatal("done in one step")
	}
	if c.cur.next == 0 || len(c.cur.ranges) == 0 {
		t.Fatal("no progress")
	}
	// Insert one byte at the start: every match moves by one.
	ns, err := buf.Edit([]buffer.Edit{{Start: 0, End: 0, Text: "y"}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	ms, done := c.find(buf, ns, q, fc.now().Add(docScanBudget), nil)
	if c.cur.ver != ns.Version() || c.scans != 2 {
		t.Fatalf("version %d scans %d", c.cur.ver, c.scans)
	}
	for !done {
		ms, done = c.find(buf, ns, q, fc.now().Add(docScanBudget), nil)
	}
	want := oneShotMatches(ns, q)
	if !slices.Equal(ms, want) {
		t.Fatalf("after the edit: %d ranges, want %d", len(ms), len(want))
	}
	if ms[0].a != 4 {
		t.Fatalf("old version's offsets kept: %v", ms[0])
	}
	for _, e := range c.ents {
		if e != nil && e.buf == buf && e.ver != ns.Version() {
			t.Fatalf("stale scan of version %d kept", e.ver)
		}
	}
}

// TestDocumentMatchesLRU: A, B, then A again finds A's scan in the cache;
// at most docCacheSize queries are kept.
func TestDocumentMatchesLRU(t *testing.T) {
	buf := buffer.New(strings.Repeat("alpha beta gamma delta epsilon\n", 100))
	snap := buf.Snapshot()
	c := &documentMatches{}
	find := func(w string) []brange {
		ms, done := c.find(buf, snap, &selQuery{text: w, whole: true}, time.Now().Add(time.Hour), nil)
		if !done {
			t.Fatalf("%s not done", w)
		}
		return ms
	}
	a := slices.Clone(find("alpha"))
	find("beta")
	if c.scans != 2 {
		t.Fatalf("scans %d", c.scans)
	}
	if got := find("alpha"); !slices.Equal(got, a) || c.scans != 2 {
		t.Fatalf("A→B→A rescanned (scans %d)", c.scans)
	}
	// The same text, other options: another key.
	c.find(buf, snap, &selQuery{text: "alpha"}, time.Now().Add(time.Hour), nil)
	if c.scans != 3 {
		t.Fatalf("whole-word option not in the key (scans %d)", c.scans)
	}
	find("gamma")
	find("delta") // evicts the least recently used: beta
	if c.scans != 5 {
		t.Fatalf("scans %d", c.scans)
	}
	find("alpha")
	if c.scans != 5 {
		t.Fatal("recently used entry evicted")
	}
	find("beta")
	if c.scans != 6 {
		t.Fatal("least recently used entry kept past docCacheSize")
	}
	// An incomplete scan resumes where it was: A (unfinished), B, A.
	big := buffer.New(strings.Repeat("alpha beta\n", 50_000))
	bs := big.Snapshot()
	fc := &fakeScanClock{t: time.Unix(0, 0), tick: time.Millisecond}
	c = &documentMatches{now: fc.now}
	qa := &selQuery{text: "zeta", whole: true}
	c.step(big, bs, qa, fc.now().Add(docScanBudget))
	next := c.cur.next
	c.step(big, bs, &selQuery{text: "eta", whole: true}, fc.now().Add(docScanBudget))
	c.step(big, bs, qa, fc.now().Add(docScanBudget))
	if c.scans != 2 || c.cur.next <= next {
		t.Fatalf("A→B→A restarted A's scan (scans %d, next %d→%d)", c.scans, next, c.cur.next)
	}
}

// TestOccurrencesFirstFrameBudget: with a clock that moves at every
// reading, one frame's scanning work is bounded by the budget: at most
// (budget / tick + 1) chunks of lines, however large the document, and the
// view requests the next frame (a damaged wake-up) until the scan is done.
func TestOccurrencesFirstFrameBudget(t *testing.T) {
	goText(t)
	var sb strings.Builder
	for i := range 20_000 {
		fmt.Fprintf(&sb, "line %d of the text\n", i)
	}
	sb.WriteString("rare\n")
	st := NewState(sb.String(), "a.txt", theme.Base("vs"))
	h := newHintHost(t, st, geom.Size{W: 400, H: 200}, 1)
	h.focusAt(0)
	vs := h.vw.vs
	tick := 500 * time.Microsecond
	fc := &fakeScanClock{t: time.Unix(0, 0), tick: tick}
	vs.scanNow = fc.now
	vs.occMatches.chunk, vs.selMatches.chunk = 1024, 1024
	// The last line in view, the caret on "rare".
	snap := st.Snapshot()
	last := snap.LineCount() - 2
	st.MoveCaret(snap.LineStart(last)+1, false)
	st.Viewport.TopLine = last - 3
	h.frame()
	h.now = h.now.Add(occDebounce)
	h.frame()
	if vs.decoCount[decoOcc] != 1 {
		t.Fatalf("first frame: %d occurrences, want the visible one", vs.decoCount[decoOcc])
	}
	e := vs.occMatches.cur
	if e == nil || e.done {
		t.Fatal("the scan finished in the first frame")
	}
	// Each chunk scans at least one line of at most chunk bytes (each line
	// counts docScanLineCost more), and at most docScanLines lines.
	maxLines := (int(docScanBudget/tick) + 1) * min(1024/docScanLineCost+1, docScanLines)
	if e.next > maxLines {
		t.Fatalf("first frame scanned %d lines, budget allows %d", e.next, maxLines)
	}
	if next := h.r.NextWake(); next.After(h.now) {
		t.Fatalf("no frame requested for the scan (next wake in %v)", next.Sub(h.now))
	}
	frames := 0
	for prev := e.next; !e.done; prev = e.next {
		h.frame()
		frames++
		if e.next-prev > maxLines {
			t.Fatalf("a frame scanned %d lines", e.next-prev)
		}
		if vs.decoCount[decoOcc] != 1 {
			t.Fatalf("frame %d: %d occurrences", frames, vs.decoCount[decoOcc])
		}
	}
	if frames < 2 {
		t.Fatalf("%d frames", frames)
	}
	if next := h.r.NextWake(); !next.After(h.now) {
		t.Fatal("frames still requested after the scan finished")
	}
}

// damageDocScan runs a resumable scan through the verifier (part of
// TestViewDamageVerified): the visible selection matches past the
// document's first maxFindMatches are drawn provisionally in the first
// frame, the scan's frames that change nothing declare nothing (the
// renderer draws them whole: ui.Context.Damage), and the frame that finds
// the quota exhausted damages (only) the rows whose matches go.
// The same for the occurrences of the word at the caret.
func damageDocScan(t *testing.T) {
	// Matches on lines 0..1199, then on lines 1220 and 1225 only.
	xs := strings.Repeat("xx yy\n", 4)
	text := strings.Repeat("ab cd\n", 1200) + strings.Repeat(xs, 5) + "ab cd\n" + xs + "ab cd\n" + strings.Repeat(xs, 5)
	st := NewState(text, "a.txt", theme.Base("vs-dark"))
	h := newHintHost(t, st, geom.Size{W: 400, H: 360}, 1.25)
	h.focusAt(0)
	vs := h.vw.vs
	fc := &fakeScanClock{t: time.Unix(0, 0), tick: time.Millisecond}
	vs.scanNow = fc.now
	vs.occMatches.chunk, vs.selMatches.chunk = 512, 512
	snap := st.Snapshot()
	st.Viewport.TopLine = 1212
	h.frame() // scrolled far: a full frame
	l := snap.LineStart(1220)
	scan := func(what string, c *documentMatches, kind decoKind) {
		t.Helper()
		if vs.decoCount[kind] == 0 || c.cur == nil || c.cur.done {
			t.Fatalf("%s: provisional matches %d (scan %+v)", what, vs.decoCount[kind], c.cur)
		}
		frames := 0
		for !c.cur.done {
			if next := h.r.NextWake(); next.After(h.now) {
				t.Fatalf("%s: scan frame not requested", what)
			}
			partial := h.frame()
			frames++
			if c.cur.done && !partial {
				t.Fatalf("%s: the frame removing the matches was not partial", what)
			}
			if dam, _ := h.lineHints(); !c.cur.done && len(dam) != 0 {
				t.Fatalf("%s: scan frame %d changing nothing declared %v", what, frames, dam)
			}
		}
		if vs.decoCount[kind] != 0 || len(c.ranges) != maxFindMatches {
			t.Fatalf("%s: %d drawn, %d found after the scan", what, vs.decoCount[kind], len(c.ranges))
		}
		t.Logf("%s: %d frames", what, frames)
	}
	st.SetSelections([]Selection{{Anchor: l, Head: l + 2}})
	h.step("select ab")
	scan("selection", &vs.selMatches, decoSelHL)
	st.MoveCaret(l+4, false) // in "cd"
	h.step("caret on cd")
	h.now = h.now.Add(occDebounce)
	h.step("occurrences")
	scan("occurrences", &vs.occMatches, decoOcc)
	h.verified() // every frame verified; the counted ones partial
}

// TestDocumentMatchesShortLinesBudget: with the real clock, a step over a
// document of short lines stays within its budget (the clock is read every
// docScanLines lines, not only every docScanChunk bytes), plus a generous
// slack for one block and scheduling.
func TestDocumentMatchesShortLinesBudget(t *testing.T) {
	buf := buffer.New(strings.Repeat("ab\n", 200_000))
	snap := buf.Snapshot()
	q := &selQuery{text: "zz", whole: true}
	// One block (a step whose deadline has passed scans exactly one).
	c := &documentMatches{now: time.Now}
	start := time.Now()
	c.step(buf, snap, q, start)
	block := time.Since(start)
	if c.cur.next != docScanLines {
		t.Fatalf("a block scanned %d lines (%v), want %d", c.cur.next, block, docScanLines)
	}
	if block > docScanBudget/2 {
		t.Skipf("one block of %d lines took %v: machine too slow to measure", c.cur.next, block)
	}
	limit := docScanBudget + time.Millisecond
	var elapsed time.Duration
	for try := 0; try < 3; try++ {
		c := &documentMatches{now: time.Now}
		start := time.Now()
		if c.step(buf, snap, q, start.Add(docScanBudget)) {
			t.Fatal("200 000 lines scanned in one budget")
		}
		if elapsed = time.Since(start); elapsed <= limit {
			return
		}
	}
	t.Fatalf("a step took %v, budget %v", elapsed, docScanBudget)
}

// TestDocumentMatchesProvisionalQuota: with 990 matches scanned, a visible
// unscanned line of 20 matches shows only the 9 the quota leaves.
func TestDocumentMatchesProvisionalQuota(t *testing.T) {
	text := strings.Repeat("ab\n", 990) + strings.Repeat("\n", 10) + strings.Repeat("ab ", 20) + "\n"
	buf := buffer.New(text)
	snap := buf.Snapshot()
	q := &selQuery{text: "ab", whole: true}
	fc := &fakeScanClock{t: time.Unix(0, 0), tick: time.Millisecond}
	c := &documentMatches{now: fc.now, chunk: 1} // one line per step
	for c.cur == nil || c.cur.next < 990 {
		c.step(buf, snap, q, time.Unix(0, 0))
	}
	if len(c.cur.ranges) != 990 || c.cur.done {
		t.Fatalf("scanned %d matches (done %v)", len(c.cur.ranges), c.cur.done)
	}
	ms, done := c.find(buf, snap, q, time.Time{}, []int{1000})
	ls := snap.LineStart(1000)
	if done || len(ms) != maxFindMatches {
		t.Fatalf("%d ranges, want %d", len(ms), maxFindMatches)
	}
	for k, m := range ms[990:] {
		if m.a != ls+3*k {
			t.Fatalf("provisional %d at %d, want %d", k, m.a, ls+3*k)
		}
	}
}
