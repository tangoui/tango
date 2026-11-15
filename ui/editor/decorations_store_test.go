package editor

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/buffer"
)

// decoStoreTestOpts stands in for the decoration options the store will
// carry (the store only keeps and returns the pointer).
type decoStoreTestOpts struct{ name string }

var decoStoreStickiness = []decorationStickiness{
	decorationAlwaysGrows, decorationNeverGrows, decorationGrowsBefore, decorationGrowsAfter,
}

// randRuneOffset returns a random offset of s on a character boundary.
func randRuneOffset(r *rand.Rand, s buffer.Snapshot) int {
	return snapToRune(s, r.IntN(s.Len()+1), buffer.Left)
}

// randBoundaryEdits returns 1-4 sorted, non-overlapping edits of s whose
// ends are character boundaries, with multi-byte and line-break text.
func randBoundaryEdits(r *rand.Rand, s buffer.Snapshot) []buffer.Edit {
	pieces := []string{"", "a", "中", "😀", "\n", "\r\n", "ab\ncd", "é", "xyz", "\r"}
	k := 1 + r.IntN(4)
	offs := make([]int, 2*k)
	for i := range offs {
		offs[i] = randRuneOffset(r, s)
	}
	slices.Sort(offs)
	var out []buffer.Edit
	for i := 0; i < len(offs); i += 2 {
		start, end := offs[i], offs[i+1]
		if r.IntN(3) == 0 {
			end = start // pure insertion
		}
		if k := len(out); k > 0 && out[k-1].End > start {
			continue
		}
		out = append(out, buffer.Edit{Start: start, End: end, Text: pieces[r.IntN(len(pieces))]})
	}
	return out
}

type refDeco struct {
	start, end int
	opts       *decoStoreTestOpts
}

func randDecos(r *rand.Rand, s buffer.Snapshot, n int, stick decorationStickiness) ([]decorationSpec[decoStoreTestOpts], []refDeco) {
	specs := make([]decorationSpec[decoStoreTestOpts], n)
	ref := make([]refDeco, n)
	for i := range specs {
		a, b := randRuneOffset(r, s), randRuneOffset(r, s)
		if r.IntN(4) == 0 {
			b = a // empty decorations too
		}
		a, b = min(a, b), max(a, b)
		o := &decoStoreTestOpts{name: string(rune('a' + i%26))}
		specs[i] = decorationSpec[decoStoreTestOpts]{start: a, end: b, stick: stick, opts: o}
		ref[i] = refDeco{a, b, o}
	}
	return specs, ref
}

// checkDecoStore compares the store with the reference ranges and checks
// its internal invariants.
func checkDecoStore(t *testing.T, step int, st *decorationStore[decoStoreTestOpts], owners []string, views []*State, ref [][]refDeco) {
	t.Helper()
	for i, o := range owners {
		got := st.get(o, views[i])
		if len(got) != len(ref[i]) {
			t.Fatalf("step %d owner %s: %d decorations, want %d", step, o, len(got), len(ref[i]))
		}
		for k, g := range got {
			w := ref[i][k]
			if g.start != w.start || g.end != w.end || g.opts != w.opts || g.stick != decoStoreStickiness[i] {
				t.Fatalf("step %d owner %s (%v) #%d: [%d,%d), want [%d,%d)", step, o, decoStoreStickiness[i], k, g.start, g.end, w.start, w.end)
			}
		}
	}
	if !slices.IsSortedFunc(st.es, func(a, b decorationEntry[decoStoreTestOpts]) int { return a.start - b.start }) {
		t.Fatalf("step %d: entries out of order", step)
	}
	for _, e := range st.es {
		if e.end-e.start > st.maxSpan || e.end-e.start > st.spanLimit {
			t.Fatalf("step %d: entry [%d,%d) breaks maxSpan %d / limit %d", step, e.start, e.end, st.maxSpan, st.spanLimit)
		}
	}
}

// TestDecorationStoreMatchesAnchors runs 3000 random multi-edit steps on a
// document with multi-byte characters and line breaks, with one owner per
// stickiness (one of them State-level), and compares every decoration
// with a reference that tracks each end as a buffer.Anchor with the
// stickiness' bias (Buffer.Resolve), re-anchored every step, then
// collapses inverted ranges to their start. Queries are compared with a
// brute-force scan of the reference.
func TestDecorationStoreMatchesAnchors(t *testing.T) {
	r := rand.New(rand.NewPCG(21, 4))
	var init strings.Builder
	for init.Len() < 600 {
		init.WriteString([]string{"ab", "中文", "😀", "\n", "é", "  x", "\n\n"}[r.IntN(7)])
	}
	buf := buffer.New(init.String())
	st := newDecorationStore[decoStoreTestOpts]()
	st.spanLimit = 40 // exercise the long-span list on a small document
	view := &State{}
	owners := []string{"always", "never", "before", "after"}
	views := []*State{nil, nil, view, nil}
	ref := make([][]refDeco, len(owners))
	reset := func(i int) {
		specs, rd := randDecos(r, buf.Snapshot(), 5+r.IntN(30), decoStoreStickiness[i])
		st.set(owners[i], views[i], specs, buf.Snapshot())
		ref[i] = rd
	}
	for i := range owners {
		reset(i)
	}
	checkDecoStore(t, -1, st, owners, views, ref)
	for step := 0; step < 3000; step++ {
		if step%97 == 0 {
			reset(r.IntN(len(owners)))
		}
		edits := randBoundaryEdits(r, buf.Snapshot())
		type anchored struct{ s, e buffer.Anchor }
		anchors := make([][]anchored, len(owners))
		for i := range owners {
			sb, eb := decoStoreStickiness[i].biases()
			for _, d := range ref[i] {
				anchors[i] = append(anchors[i], anchored{buf.Anchor(d.start, sb), buf.Anchor(d.end, eb)})
			}
		}
		ns, err := buf.Edit(edits, nil, nil)
		if err != nil {
			t.Fatalf("step %d: %v", step, err)
		}
		for i := range owners {
			for k, a := range anchors[i] {
				s, e := buf.Resolve(a.s), buf.Resolve(a.e)
				ref[i][k].start, ref[i][k].end = s, max(s, e)
			}
		}
		st.apply(edits, buffer.NewTracker(edits), ns)
		checkDecoStore(t, step, st, owners, views, ref)

		// Queries: random windows, for the model and for the view.
		for q := 0; q < 3; q++ {
			a, b := randRuneOffset(r, ns), randRuneOffset(r, ns)
			a, b = min(a, b), max(a, b)
			for _, v := range []*State{nil, view} {
				var want []refDeco
				for i := range owners {
					if views[i] != nil && views[i] != v {
						continue
					}
					for _, d := range ref[i] {
						if d.start <= b && d.end >= a {
							want = append(want, d)
						}
					}
				}
				got := st.query(a, b, v, nil)
				if len(got) != len(want) || !slices.IsSortedFunc(got, func(x, y decorationEntry[decoStoreTestOpts]) int { return x.start - y.start }) {
					t.Fatalf("step %d query [%d,%d] view %v: %d results (sorted %v), want %d", step, a, b, v != nil, len(got), slices.IsSortedFunc(got, func(x, y decorationEntry[decoStoreTestOpts]) int { return x.start - y.start }), len(want))
				}
				for _, w := range want {
					if !slices.ContainsFunc(got, func(g decorationEntry[decoStoreTestOpts]) bool {
						return g.start == w.start && g.end == w.end && g.opts == w.opts
					}) {
						t.Fatalf("step %d query [%d,%d]: missing %+v", step, a, b, w)
					}
				}
			}
		}
	}
	if len(st.big) == 0 {
		t.Log("note: no long decorations at the end")
	}
	st.dropView(view)
	if got := st.get("before", view); got != nil {
		t.Fatalf("dropView left %d decorations", len(got))
	}
	ref[2] = nil
	checkDecoStore(t, 3000, st, owners[:2], views[:2], ref[:2])
}

func TestDecorationStoreSet(t *testing.T) {
	s := buffer.NewSnapshot("a中b\nxyz")
	st := newDecorationStore[decoStoreTestOpts]()
	o := &decoStoreTestOpts{}
	st.set("x", nil, []decorationSpec[decoStoreTestOpts]{
		{start: 2, end: 3, opts: o},   // inside 中: widened to the character
		{start: 6, end: 1, opts: o},   // reversed
		{start: -5, end: 99, opts: o}, // clamped
	}, s)
	got := st.get("x", nil)
	want := [][2]int{{1, 4}, {1, 6}, {0, s.Len()}}
	for i, g := range got {
		if [2]int{g.start, g.end} != want[i] {
			t.Fatalf("#%d: [%d,%d), want %v", i, g.start, g.end, want[i])
		}
	}
	v := st.version
	st.set("x", nil, nil, s)
	if st.get("x", nil) != nil || st.len() != 0 || st.version == v {
		t.Fatalf("clearing the owner: %d left, version %d", st.len(), st.version)
	}
	// Owner ids are reused.
	for i := 0; i < 70000; i++ {
		st.set("o", nil, []decorationSpec[decoStoreTestOpts]{{start: 0, end: 1}}, s)
		st.set("o", nil, nil, s)
	}
	if len(st.names) > 1 {
		t.Fatalf("%d owner slots after reuse", len(st.names))
	}
}

// decoBench builds a 100k-line document with 10k decorations spread over it.
func decoBench(b testing.TB) (buffer.Snapshot, []decorationSpec[decoStoreTestOpts]) {
	var sb strings.Builder
	for i := 0; i < 100_000; i++ {
		sb.WriteString("\tfunc example(a, b int) int { return a + b }\n")
	}
	s := buffer.NewSnapshot(sb.String())
	r := rand.New(rand.NewPCG(1, 1))
	o := &decoStoreTestOpts{}
	specs := make([]decorationSpec[decoStoreTestOpts], 10_000)
	for i := range specs {
		a := r.IntN(s.Len() - 100)
		specs[i] = decorationSpec[decoStoreTestOpts]{start: a, end: a + r.IntN(80), stick: decoStoreStickiness[i%4], opts: o}
	}
	return s, specs
}

func BenchmarkDecorationStoreSet10k(b *testing.B) {
	s, specs := decoBench(b)
	st := newDecorationStore[decoStoreTestOpts]()
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		st.set("diff", nil, specs, s)
	}
}

// typeShift returns a store with 10k decorations and a keystroke in the
// middle of the document and its undo, with prebuilt trackers.
func typeShift(b testing.TB) (st *decorationStore[decoStoreTestOpts], step func()) {
	s0, specs := decoBench(b)
	st = newDecorationStore[decoStoreTestOpts]()
	st.set("diff", nil, specs, s0)
	at := s0.LineStart(50_000) + 7
	ins := []buffer.Edit{{Start: at, End: at, Text: "x"}}
	del := []buffer.Edit{{Start: at, End: at + 1}}
	s1, _ := s0.Apply(ins)
	tIns, tDel := buffer.NewTracker(ins), buffer.NewTracker(del)
	return st, func() {
		st.apply(ins, tIns, s1)
		st.apply(del, tDel, s0)
	}
}

// BenchmarkDecorationStoreTypeShift measures one keystroke and its undo
// (two shifts) with 10k decorations on 100k lines.
func BenchmarkDecorationStoreTypeShift(b *testing.B) {
	_, step := typeShift(b)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		step()
	}
}

func TestDecorationStoreTypeShiftAllocs(t *testing.T) {
	_, step := typeShift(t)
	if n := testing.AllocsPerRun(20, step); n != 0 {
		t.Fatalf("%v allocations per keystroke shift, want 0", n)
	}
}

// BenchmarkDecorationStoreQueryFrame measures the per-frame query of a
// 60-line viewport.
func BenchmarkDecorationStoreQueryFrame(b *testing.B) {
	s, specs := decoBench(b)
	st := newDecorationStore[decoStoreTestOpts]()
	st.set("diff", nil, specs, s)
	lo, hi := s.LineStart(40_000), s.LineStart(40_060)
	var dst []decorationEntry[decoStoreTestOpts]
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		dst = st.query(lo, hi, nil, dst[:0])
	}
	if len(dst) == 0 {
		b.Fatal("no decorations in the viewport")
	}
}
