package editor

import (
	"slices"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/geom"
)

func TestStickyScrollHeaders(t *testing.T) {
	goText(t)
	st := NewState("func outer() {\n  func inner() {\n"+strings.Repeat("    work()\n", 40)+"  }\n}\n", "a.go", theme.Base("vs-dark"))
	defer st.Close()
	d, vw := viewDriver(st, 640, 300, nil)
	d.Frame()
	st.Viewport.TopLine = 10
	d.Frame()
	if got := (*vw).vs.stickyLines; !slices.Equal(got, []int{0, 1}) {
		t.Fatalf("headers %v", got)
	}
	r := (*vw).vs.stickyRect
	d.Click(geom.Pt(r.X+90, r.Y+(*vw).lineHeight()*1.5))
	d.Frame()
	if st.Snapshot().LineOf(st.Primary().Head) != 1 {
		t.Fatal("sticky click did not jump")
	}
	if o := DefaultOptions().StickyScroll; !o.Enabled || o.MaxLineCount != 5 {
		t.Fatalf("defaults %+v", o)
	}
}

func TestStickyDoesNotEvictVisibleTokens(t *testing.T) {
	goText(t)
	st := NewState("func outer() {\n"+strings.Repeat("  return 1\n", 2000)+"}\n", "a.go", theme.Base("vs"))
	defer st.Close()
	st.Highlighter().TokenWindow = 24
	d, vw := viewDriver(st, 640, 300, nil)
	d.Frame()
	st.Viewport.TopLine = 1000
	d.Frame()
	hl := st.Highlighter()
	snap := st.Snapshot()
	if _, ok := hl.PeekTokens(snap, 1000, nil); !ok {
		t.Fatal("visible tokens not cached")
	}
	(*vw).prepareSticky(snap)
	(*vw).layoutStickyRows(snap, st.Theme)
	if _, ok := hl.PeekTokens(snap, 1000, nil); !ok {
		t.Fatal("sticky header evicted visible tokens")
	}
}

func TestStickyMaxLines(t *testing.T) {
	goText(t)
	var b strings.Builder
	for i := 0; i < 7; i++ {
		b.WriteString(strings.Repeat("  ", i) + "scope {\n")
	}
	b.WriteString(strings.Repeat("                body\n", 40))
	for i := 6; i >= 0; i-- {
		b.WriteString(strings.Repeat("  ", i) + "}\n")
	}
	st := NewState(b.String(), "a.txt", theme.Base("vs"))
	defer st.Close()
	d, vw := viewDriver(st, 640, 400, nil)
	d.Frame()
	st.Viewport.TopLine = 15
	d.Frame()
	if got := (*vw).vs.stickyLines; !slices.Equal(got, []int{0, 1, 2, 3, 4}) {
		t.Fatalf("max five: %v", got)
	}
	st.Options.StickyScroll.Enabled = false
	d.Frame()
	if len((*vw).vs.stickyLines) != 0 {
		t.Fatal("disabled sticky")
	}
}
