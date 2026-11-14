package ui

import (
	"strconv"
	"testing"

	"github.com/tangoui/tango/ui/input"
)

// TestTableVirtualColumns confirms column virtualization: only the
// columns inside the horizontal viewport are built (header and body),
// fixed columns always are, and scrolling sideways builds the newly
// visible ones.
func TestTableVirtualColumns(t *testing.T) {
	d := tblDriver(func(c *Context) {
		var cols []TableColumn
		for k := 0; k < 60; k++ {
			k := k
			col := TableColumn{Key: strconv.Itoa(k), Title: "H" + strconv.Itoa(k) + ":", Width: 100,
				Render: func(i int) { Text(c, "v"+strconv.Itoa(k)+":"+strconv.Itoa(i)) }}
			switch k {
			case 0:
				col.Fixed = FixedLeft
			case 59:
				col.Fixed = FixedRight
			}
			cols = append(cols, col)
		}
		Table(c, cols, 1000).ScrollX(6000).ScrollY(300).Virtual().Show()
	})
	built := func(k int) int { return countKind(d.r.c.root, "v"+strconv.Itoa(k)+":") }
	if built(0) == 0 || built(59) == 0 {
		t.Fatal("fixed columns must always be built")
	}
	if built(3) == 0 {
		t.Fatal("visible column 3 not built")
	}
	if n := built(30); n != 0 {
		t.Fatalf("off-screen column 30 built %d cells", n)
	}
	if countKind(d.r.c.root, "H3:") == 0 {
		t.Fatal("visible header cell not found")
	}
	if countKind(d.r.c.root, "H30:") != 0 {
		t.Fatal("off-screen header cell built")
	}
	cols := 0
	for k := 1; k < 59; k++ {
		if built(k) > 0 {
			cols++
		}
	}
	if cols > 10 {
		t.Fatalf("%d scrolling columns built in a 780px viewport", cols)
	}
	// Scroll sideways (Shift+wheel is horizontal; DX directly here).
	r, ok := d.Find("row:0")
	if !ok {
		t.Fatal("row missing")
	}
	d.Inject(input.Scroll{Pos: r.Center(), DX: 2900})
	d.Settle()
	if built(30) == 0 {
		t.Fatal("column 30 not built after scrolling")
	}
	if built(3) != 0 {
		t.Fatal("column 3 still built after scrolling away")
	}
	if built(0) == 0 || built(59) == 0 {
		t.Fatal("fixed columns lost after scrolling")
	}
}
