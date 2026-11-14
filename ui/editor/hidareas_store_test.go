package editor

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/buffer"
)

// TestHidAreaStoreMatchesBruteForce moves random hidden areas through
// random edits (start of the first line with Left bias, end of the last
// line with Right bias, like buffer.Anchors re-anchored every step) and
// compares the union with the folds, and HiddenAreaAt, with a direct
// computation.
func TestHidAreaStoreMatchesBruteForce(t *testing.T) {
	r := rand.New(rand.NewPCG(2, 8))
	var init strings.Builder
	for init.Len() < 500 {
		init.WriteString([]string{"ab", "中", "\n", "x\n", "\n\n", "😀"}[r.IntN(6)])
	}
	buf := buffer.New(init.String())
	hs := newHidAreaStore()
	owners := []string{"b-diff", "a-other"}
	ref := map[string][]lineSpan{}
	reset := func(o string) {
		spans := randSpans(r, buf.Snapshot().LineCount(), r.IntN(5))
		hs.set(o, spans, buf.Snapshot())
		ref[o] = slices.Clone(spans)
	}
	for _, o := range owners {
		reset(o)
	}
	for step := 0; step < 2000; step++ {
		if step%41 == 0 {
			reset(owners[r.IntN(2)])
		}
		edits := randBoundaryEdits(r, buf.Snapshot())
		old := buf.Snapshot()
		type pair struct{ s, e buffer.Anchor }
		anchors := map[string][]pair{}
		for _, o := range owners {
			for _, sp := range ref[o] {
				anchors[o] = append(anchors[o], pair{buf.Anchor(old.LineStart(sp.a), buffer.Left), buf.Anchor(lineEndOffset(old, sp.b), buffer.Right)})
			}
		}
		ns, err := buf.Edit(edits, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, o := range owners {
			for i, p := range anchors[o] {
				ref[o][i] = lineSpan{ns.LineOf(buf.Resolve(p.s)), ns.LineOf(buf.Resolve(p.e))}
			}
		}
		hs.apply(buffer.NewTracker(edits), ns)
		for _, o := range owners {
			if got := hs.get(o); !slices.Equal(got, ref[o]) {
				t.Fatalf("step %d owner %s: %v, want %v", step, o, got, ref[o])
			}
		}
		n := ns.LineCount()
		folds := spansOf(bruteHidden(n, randSpans(r, n, r.IntN(3))))
		want := spansOf(bruteHidden(n, folds, ref[owners[0]], ref[owners[1]]))
		if got := hs.union(folds, nil); !slices.Equal(got, want) {
			t.Fatalf("step %d: union %v, want %v", step, got, want)
		}
		for l := 0; l < n; l++ {
			wantOwner, wantOK := "", false
			for _, o := range []string{"a-other", "b-diff"} { // by name
				if slices.ContainsFunc(ref[o], func(sp lineSpan) bool { return sp.a <= l && l <= sp.b }) {
					wantOwner, wantOK = o, true
					break
				}
			}
			if o, ok := hs.at(l); o != wantOwner || ok != wantOK {
				t.Fatalf("step %d line %d: at = %q %v, want %q %v", step, l, o, ok, wantOwner, wantOK)
			}
		}
	}
}

func TestHidAreaStoreSetClamps(t *testing.T) {
	s := buffer.NewSnapshot("a\nb\nc")
	hs := newHidAreaStore()
	hs.set("x", []lineSpan{{2, 9}, {-3, 0}, {1, 0}}, s)
	if got, want := hs.get("x"), []lineSpan{{2, 2}, {0, 0}}; !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if got := hs.union(nil, nil); !slices.Equal(got, []lineSpan{{0, 0}, {2, 2}}) {
		t.Fatalf("union %v", got)
	}
	v := hs.version
	hs.set("x", nil, s)
	if hs.get("x") != nil || hs.version == v || len(hs.union(nil, nil)) != 0 {
		t.Fatal("clearing an owner")
	}
}
