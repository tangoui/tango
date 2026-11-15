package editor

import (
	"math/rand/v2"
	"slices"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/buffer"
)

type zoneTestPayload struct{ n int }

// bruteHidden marks the lines covered by spans.
func bruteHidden(n int, spans ...[]lineSpan) []bool {
	h := make([]bool, n)
	for _, ss := range spans {
		for _, s := range ss {
			for l := max(s.a, 0); l <= min(s.b, n-1); l++ {
				h[l] = true
			}
		}
	}
	return h
}

// spansOf turns marked lines into sorted spans, merging adjacent ones.
func spansOf(h []bool) []lineSpan {
	var out []lineSpan
	for l, on := range h {
		if !on {
			continue
		}
		if k := len(out); k > 0 && out[k-1].b == l-1 {
			out[k-1].b = l
			continue
		}
		out = append(out, lineSpan{l, l})
	}
	return out
}

func randSpans(r *rand.Rand, lines, n int) []lineSpan {
	out := make([]lineSpan, n)
	for i := range out {
		a := r.IntN(lines)
		out[i] = lineSpan{a, min(lines-1, a+r.IntN(6))}
	}
	return out
}

// TestZoneStoreMatchesBruteForce moves random zones through random edits
// and compares their lines with buffer.Anchors (end of the line, Left
// bias, re-anchored every step), and the resolved layout (zones of hidden
// lines moved up to the nearest visible line or dropped, rows summed per
// line) with a direct computation.
func TestZoneStoreMatchesBruteForce(t *testing.T) {
	r := rand.New(rand.NewPCG(8, 1))
	var init strings.Builder
	for init.Len() < 500 {
		init.WriteString([]string{"ab", "中", "\n", "x\n", "\n\n", "😀"}[r.IntN(6)])
	}
	buf := buffer.New(init.String())
	zs := newZoneStore[zoneTestPayload]()
	owners := []string{"diff", "other"}
	ref := map[string][]zoneSpec[zoneTestPayload]{}
	reset := func(o string) {
		n := buf.Snapshot().LineCount()
		specs := make([]zoneSpec[zoneTestPayload], r.IntN(12))
		for i := range specs {
			specs[i] = zoneSpec[zoneTestPayload]{
				id: uint64(i), afterLine: r.IntN(n+1) - 1, rows: 1 + r.IntN(3),
				showInHidden: r.IntN(2) == 0, z: &zoneTestPayload{i},
			}
		}
		zs.set(o, specs, buf.Snapshot())
		ref[o] = slices.Clone(specs)
	}
	for _, o := range owners {
		reset(o)
	}
	for step := 0; step < 2000; step++ {
		if step%53 == 0 {
			reset(owners[r.IntN(2)])
		}
		edits := randBoundaryEdits(r, buf.Snapshot())
		old := buf.Snapshot()
		anchors := map[string][]buffer.Anchor{}
		for _, o := range owners {
			for _, z := range ref[o] {
				anchors[o] = append(anchors[o], buf.Anchor(lineEndOffset(old, z.afterLine), buffer.Left))
			}
		}
		ns, err := buf.Edit(edits, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		for _, o := range owners {
			for i := range ref[o] {
				if ref[o][i].afterLine >= 0 {
					ref[o][i].afterLine = ns.LineOf(buf.Resolve(anchors[o][i]))
				}
			}
		}
		zs.apply(buffer.NewTracker(edits), ns)
		for _, o := range owners {
			if got := zs.get(o); !slices.Equal(got, ref[o]) {
				t.Fatalf("step %d owner %s: %+v, want %+v", step, o, got, ref[o])
			}
		}

		n := ns.LineCount()
		hidden := spansOf(bruteHidden(n, randSpans(r, n, r.IntN(4))))
		hid := bruteHidden(n, hidden)
		wantRows := map[int]int{}
		for _, o := range owners {
			for _, z := range ref[o] {
				l := z.afterLine
				if l >= 0 && hid[l] {
					if !z.showInHidden {
						continue
					}
					for l >= 0 && hid[l] {
						l--
					}
				}
				wantRows[l] += z.rows
			}
		}
		lead, rows := zs.resolve(hidden)
		if lead != wantRows[-1] {
			t.Fatalf("step %d: lead %d, want %d", step, lead, wantRows[-1])
		}
		delete(wantRows, -1)
		if len(rows) != len(wantRows) || !slices.IsSortedFunc(rows, func(a, b zoneLineRows) int { return a.line - b.line }) {
			t.Fatalf("step %d: rows %+v, want %v", step, rows, wantRows)
		}
		for _, lr := range rows {
			if wantRows[lr.line] != lr.rows || hid[lr.line] {
				t.Fatalf("step %d: rows %+v, want %v (hidden %v)", step, rows, wantRows, hidden)
			}
		}
	}
	v := zs.version
	zs.set("diff", nil, buf.Snapshot())
	if zs.get("diff") != nil || zs.version == v {
		t.Fatal("clearing an owner")
	}
}
