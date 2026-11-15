package text

import (
	"reflect"
	"testing"
)

func TestDenseClustersPreserveGlyphOrder(t *testing.T) {
	// Multiple glyphs can share one cluster, appear in reverse cluster order
	// in RTL text, or have no glyph at all (a ligature's later rune).
	gl := []sg{{cluster: 3}, {cluster: 3}, {cluster: 1}, {cluster: 0}, {cluster: 0}}
	c := indexGlyphClusters(4, gl)
	want := [][]int{{3, 4}, {2}, {}, {0, 1}, {}}
	for r := range want {
		got := []int{}
		for i := c.first[r]; i >= 0; i = c.next[i] {
			got = append(got, int(i))
		}
		if !reflect.DeepEqual(got, want[r]) {
			t.Fatalf("cluster%d: %v != %v", r, got, want[r])
		}
	}
}

func TestDenseClustersUseOneAllocation(t *testing.T) {
	gl := make([]sg, 128)
	for i := range gl {
		gl[i].cluster = i
	}
	allocations := testing.AllocsPerRun(10, func() {
		c := indexGlyphClusters(128, gl)
		if c.first[0] != 0 {
			panic("bad index")
		}
	})
	if allocations > 1 {
		t.Fatalf("cluster index allocated %.0f times", allocations)
	}
}
