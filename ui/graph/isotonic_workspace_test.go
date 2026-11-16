package graph

import (
	"reflect"
	"testing"
)

func TestIsotonicWorkspaceReusesStorage(t *testing.T) {
	var ws isotonicWorkspace
	d, w, gap := []float64{3, 0}, []float64{1, 1}, []float64{2}
	if got := ws.fit(d, w, gap); !reflect.DeepEqual(got, []float64{0.5, 2.5}) {
		t.Fatalf("fit = %v", got)
	}
	allocs := testing.AllocsPerRun(100, func() { ws.fit(d, w, gap) })
	if allocs != 0 {
		t.Fatalf("warm fit allocates %.0f times", allocs)
	}
	if got := ws.fit([]float64{-2}, []float64{4}, nil); !reflect.DeepEqual(got, []float64{-2}) {
		t.Fatal(got)
	}
	if got := ws.fit(d, w, gap); !reflect.DeepEqual(got, []float64{0.5, 2.5}) {
		t.Fatal(got)
	}
	if got := ws.fit(nil, nil, nil); len(got) != 0 {
		t.Fatal(got)
	}
}

func TestIsotonicGapResultOwnership(t *testing.T) {
	a := isotonicGap([]float64{3, 0}, []float64{1, 1}, []float64{2})
	isotonicGap([]float64{100, 200}, []float64{1, 1}, []float64{2})
	if !reflect.DeepEqual(a, []float64{0.5, 2.5}) {
		t.Fatalf("prior result overwritten: %v", a)
	}
}

func TestLayeredLayoutAllocationBudget(t *testing.T) {
	base := randomDAG(1000, 2, 3)
	allocs := testing.AllocsPerRun(3, func() {
		g := &Graph{Nodes: append([]Node(nil), base.Nodes...), Edges: append([]Edge(nil), base.Edges...)}
		Layout(g, LayoutOptions{})
	})
	if allocs > 35000 {
		t.Fatalf("1000-node layout allocated %.0f times, want <=35000", allocs)
	}
}
