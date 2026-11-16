package editor

import (
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/theme"
)

var allocationGuides []rowGuides

func visibleGuideFixture(tb testing.TB) (*ViewW, []lineView) {
	tb.Helper()
	st := NewState(strings.Repeat("    const value = 42; // "+strings.Repeat("x", 80)+"\n", 20000), "a.txt", theme.Base("vs"))
	tb.Cleanup(st.Close)
	snap := st.Snapshot()
	rows := make([]lineView, 40)
	for k := range rows {
		i := 10000 + k
		rows[k] = lineView{i: i, text: snap.Line(i), rows: 1}
	}
	return &ViewW{st: st, vs: &viewState{}, opts: DefaultOptions()}, rows
}

func BenchmarkVisibleIndentGuides(b *testing.B) {
	w, rows := visibleGuideFixture(b)
	snap := w.st.Snapshot()
	b.ReportAllocs()
	for b.Loop() {
		allocationGuides = w.guidesFor(snap, rows, 10010)
	}
}

func TestVisibleGuidesAllocationBudget(t *testing.T) {
	result := testing.Benchmark(BenchmarkVisibleIndentGuides)
	if got := result.AllocedBytesPerOp(); got > 16000 {
		t.Fatalf("visible guide calculation allocates %d B/frame, want <=16000 without rereading source blocks", got)
	}
}

func TestVisibleGuideSourcesWithWrapAndZones(t *testing.T) {
	w, rows := visibleGuideFixture(t)
	// Wrapped continuations still carry the whole model-line source.
	rows[5].row = 1
	rows[5].segs = []seg{{x0: 0}}
	rows[6].row = 1
	rows[6].segs = []seg{{x0: 20}}
	// A zone may refer to an adjacent line, but its empty text is not source.
	zone := lineView{i: rows[9].i, zone: &zoneRow{}}
	rows = append(rows[:10:10], append([]lineView{zone}, rows[10:]...)...)
	got := w.guidesFor(w.st.Snapshot(), rows, 10010)
	for k, g := range got {
		want := 1
		if k == 5 || k == 10 {
			want = 0
		}
		if g.level != want || g.active != want {
			t.Fatalf("row %d: guides %+v, want level/active=%d", k, g, want)
		}
	}
}
