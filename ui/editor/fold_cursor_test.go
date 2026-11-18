package editor

import (
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// TestFoldControlPointerCursor: the pointer over a folding control (a
// line where a foldable region starts, folded or not) and over a folded
// line's placeholder is a hand, as Monaco's folding icons and "⋯"; next
// to them (a line without a region, the line numbers, the text) it is not.
func TestFoldControlPointerCursor(t *testing.T) {
	goText(t)
	src := "func a() {\n\tx := 1\n\ty := 2\n}\n\nvar z = 3\n" + strings.Repeat("// pad\n", 20)
	st := NewState(src, "a.go", theme.Base("vs"))
	defer st.Close()
	d, vw := viewDriver(st, 640, 300, nil)
	for range 3 {
		d.Frame()
	}
	w := *vw
	if w.vs.foldX < 0 {
		t.Fatal("no folding controls")
	}
	lh, g := st.Viewport.LineHeight, w.vs.gutter
	rowY := func(r int) float32 { return g.Y + (float32(r)+0.5)*lh - st.Viewport.OffsetY }
	at := func(p geom.Point) input.Cursor {
		d.Move(p)
		d.Frame()
		return d.Cursor()
	}
	control := func(r int) geom.Point { return geom.Pt((*vw).vs.foldX+foldControlW/2, rowY(r)) }
	if c := at(control(0)); c != input.CursorPointer {
		t.Fatalf("over the folding control of line 0: cursor %v, want the pointer", c)
	}
	if c := at(control(5)); c == input.CursorPointer {
		t.Fatal("over the control column of a line without a region: the pointer")
	}
	if c := at(geom.Pt(g.X+2, rowY(0))); c == input.CursorPointer {
		t.Fatal("over the line numbers: the pointer")
	}
	if c := at(geom.Pt((*vw).vs.text.X+4, rowY(0))); c != input.CursorText {
		t.Fatalf("over the text: cursor %v, want the text cursor", c)
	}
	// Fold line 0: its control and its placeholder show the pointer.
	d.Click(control(0))
	d.Frame()
	if len(st.Folded()) != 1 {
		t.Fatalf("folded %v", st.Folded())
	}
	if c := at(control(0)); c != input.CursorPointer {
		t.Fatalf("over the control of a folded line: cursor %v, want the pointer", c)
	}
	w = *vw
	snap := st.Snapshot()
	lv := w.rowAt(snap, 0, 0)
	x0, x1 := w.placeholderX(&lv)
	px := w.vs.text.X - st.Viewport.ScrollX + (x0+x1)/2
	if c := at(geom.Pt(px, rowY(0))); c != input.CursorPointer {
		t.Fatalf("over the folded placeholder: cursor %v, want the pointer", c)
	}
}
