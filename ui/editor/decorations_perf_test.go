package editor

import (
	"image/color"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

// Decoration budgets (docs/specs/diff-view.md, 2.1): 100 000 lines with
// 10 000 decorations.

// decoPerfDoc returns a 100 000-line document and 10 000 decorations over
// it (every tenth line: a whole-line and a range one, alternating).
func decoPerfDoc(tb testing.TB) (*Model, []Decoration) {
	tb.Helper()
	var sb strings.Builder
	for i := range 100_000 {
		sb.WriteString("func f(x int) string { return fmt.Sprint(x * 42) } // line ")
		sb.WriteString(strings.Repeat("y", i%17))
		sb.WriteByte('\n')
	}
	m := NewModelWithOptions("big.go", sb.String(), nil, ModelOptions{})
	snap := m.Snapshot()
	whole := &DecorationOptions{WholeLine: true, Background: color.NRGBA{0, 0x80, 0, 0x40}, Overview: &OverviewStyle{Color: color.NRGBA{0, 0x80, 0, 0xff}, Lane: 4}}
	inner := &DecorationOptions{Background: color.NRGBA{0x80, 0, 0, 0x40}, Stickiness: NeverGrowsWhenTypingAtEdges}
	ds := make([]Decoration, 0, 10_000)
	for l := 0; l < 100_000; l += 10 {
		if l%20 == 0 {
			ds = append(ds, LineDecoration(snap, l, l, whole))
		} else {
			ds = append(ds, Decoration{Start: snap.LineStart(l) + 5, End: snap.LineStart(l) + 12, Options: inner})
		}
	}
	return m, ds
}

// BenchmarkSetDecorations10k: replacing an owner's 10 000 decorations
// (budget 3 ms).
func BenchmarkSetDecorations10k(b *testing.B) {
	m, ds := decoPerfDoc(b)
	b.ReportAllocs()
	for b.Loop() {
		m.SetDecorations("diff", ds)
	}
}

// BenchmarkDecorationsTypeShift: one typed character in the middle of the
// document moves the 10 000 decorations (budget 50 µs: the ones after it
// only shift); the Tracker is the only allocation.
func BenchmarkDecorationsTypeShift(b *testing.B) {
	m, ds := decoPerfDoc(b)
	m.SetDecorations("diff", ds)
	st := NewView(m, nil)
	at := m.Snapshot().LineStart(50_000) + 3
	st.MoveCaret(at, false)
	b.ReportAllocs()
	i := 0
	for b.Loop() {
		if i%2 == 0 {
			m.tracked(m.Snapshot(), typedEdit(at))
		} else {
			m.tracked(m.Snapshot(), deletedEdit(at))
		}
		i++
	}
}

func typedEdit(at int) []buffer.Edit   { return []buffer.Edit{{Start: at, End: at, Text: "x"}} }
func deletedEdit(at int) []buffer.Edit { return []buffer.Edit{{Start: at, End: at + 1}} }

// TestDecorationsTypeShiftAllocs: moving the decorations through a typed
// character allocates only the Tracker.
func TestDecorationsTypeShiftAllocs(t *testing.T) {
	m, ds := decoPerfDoc(t)
	m.SetDecorations("diff", ds)
	at := m.Snapshot().LineStart(50_000) + 3
	ins, del := typedEdit(at), deletedEdit(at)
	snap := m.Snapshot()
	allocs := testing.AllocsPerRun(50, func() {
		m.tracked(snap, ins)
		m.tracked(snap, del)
	})
	trackers := testing.AllocsPerRun(50, func() {
		buffer.NewTracker(ins)
		buffer.NewTracker(del)
	})
	if allocs > trackers {
		t.Fatalf("%v allocations per insert and delete, want only the Trackers' %v", allocs, trackers)
	}
}

// BenchmarkDecorationsFrame: the decorations of one frame (about 60 rows,
// 6 of them decorated): the query, the three layers and the gutter
// (budget 100 µs).
func BenchmarkDecorationsFrame(b *testing.B) {
	prev := text.UseSystemEngine(false)
	defer text.UseSystemEngine(prev)
	m, ds := decoPerfDoc(b)
	m.SetDecorations("diff", ds)
	st := NewView(m, theme.Base("vs"))
	d, vw := viewDriver(st, 800, 60*21, nil)
	st.Viewport.TopLine = 50_000
	d.Frame()
	w := *vw
	snap := st.Snapshot()
	lines := w.layoutRows(snap, 50_000, 50_059, nil, theme.Base("vs"), w.vs.colW)
	lineY := func(r int) float32 { return float32(r-50_000) * 21 }
	var l paint.List
	b.ReportAllocs()
	for b.Loop() {
		l.Reset()
		ud := w.userDecos(snap, lines)
		w.paintWholeLineDecos(&l, lines, ud, lineY, 21)
		w.paintRangeDecos(&l, snap, lines, ud, 0, lineY, 21)
		for k := range lines {
			w.vs.udIn = inlineSpans(snap, &lines[k], ud, k, w.vs.udIn[:0])
		}
		w.paintGutterMarks(&l, 0, 60, lines, ud, lineY, snap.LineCount(), digitsOf(w.monoStyle()))
		w.paintOverviewDecos(func(line int) float32 { return float32(line) / 100 }, func(y0, y1 float32, c color.NRGBA, lane int) {
			l.Fill(geom.Rect{Y: y0, W: 4, H: y1 - y0}, geom.Radii{}, c)
		})
	}
}
