package flexbox

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

func TestMeasureMemoConstraintsAndGeneration(t *testing.T) {
	calls := 0
	leaf := &Node{Measure: func(w, h float32) geom.Size { calls++; return geom.Size{W: w / 2, H: h / 2} }}
	gen++
	for i := 1; i <= 8; i++ {
		w := float32(i * 10)
		if got := measure(leaf, w, 100, false, false); got != (geom.Size{W: w / 2, H: 50}) {
			t.Fatalf("constraint %d: %+v", i, got)
		}
	}
	before := calls
	// The cache still holds all six most recent constraints, including
	// entries that spill beyond the small inline prefix.
	for i := 3; i <= 8; i++ {
		measure(leaf, float32(i*10), 100, false, false)
	}
	if calls != before {
		t.Fatal("six-entry memo capacity was not preserved")
	}
	// Another solve must not reuse the old measurement, including when a
	// different tree has been solved between uses of this node.
	Solve(&Node{}, 10, 10)
	gen++
	measure(leaf, 80, 100, false, false)
	if calls != before+1 {
		t.Fatal("memo survived a solve generation")
	}
}

func TestSmallLayoutNoScratchAllocations(t *testing.T) {
	for _, count := range []int{1, 8} {
		root := &Node{Style: Style{Dir: layout.Row}}
		for i := 0; i < count; i++ {
			root.Children = append(root.Children, &Node{Style: Style{Width: 10, Height: 20}})
		}
		Solve(root, 640, 480)
		if allocs := testing.AllocsPerRun(100, func() { Solve(root, 640, 480) }); allocs != 0 {
			t.Fatalf("%d children: %g scratch allocations", count, allocs)
		}
	}
}

func BenchmarkSmallLayout(b *testing.B) {
	for _, count := range []int{1, 8, 32} {
		b.Run(fmtCount(count), func(b *testing.B) {
			root := &Node{Style: Style{Dir: layout.Row}}
			for i := 0; i < count; i++ {
				root.Children = append(root.Children, &Node{Style: Style{Width: 10, Height: 20}})
			}
			Solve(root, 640, 480)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				Solve(root, 640, 480)
			}
		})
	}
}

func fmtCount(n int) string {
	switch n {
	case 1:
		return "one"
	case 8:
		return "eight"
	default:
		return "many"
	}
}
