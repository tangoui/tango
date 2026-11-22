package ui

import (
	"slices"
	"strings"
	"testing"
)

func TestCodeBlockSearchAndRevealWrappedTail(t *testing.T) {
	prefix := strings.Repeat("abc\t中文", 2000)
	code := prefix + "CAFÉ\r\nnext\ncafé\nNEXT"
	query, index := "café\nnext", 0
	var w *CodeBlockW
	d := NewDriver(func(c *Context) {
		w = CodeBlock(c, code).Wrap(true).Height(140).Search(query, false).ActiveMatch(index)
	}, DriverOptions{Width: 500, Height: 150})
	d.Frame()
	d.Frame()
	d.Frame()
	first := len(prefix)
	second := first + len("CAFÉ\nnext\n")
	matchLen := len(query)
	if got, want := w.SearchMatches(), []CodeBlockMatch{{first, first + matchLen}, {second, second + matchLen}}; !slices.Equal(got, want) {
		t.Fatalf("got matches %v, want %v", got, want)
	}
	if got := w.v.sb.s.off.Y; got < 1000 {
		t.Fatalf("active match's wrapped segment did not scroll into view: offset %v", got)
	}
	if marks := w.v.marks(1, nil); len(marks) != 1 || !marks[0].current || marks[0].end != len("next") {
		t.Fatalf("cross-line marks: %+v", marks)
	}
	index = 1
	d.Frame()
	if marks := w.v.marks(2, nil); len(marks) != 1 || !marks[0].current {
		t.Fatalf("next marks: %+v", marks)
	}
	query = ""
	d.Frame()
	if got := w.SearchMatches(); len(got) != 0 || w.v.marks != nil {
		t.Fatalf("clearing query left matches %v", got)
	}
}

func TestCodeBlockWrapBoundaryParity(t *testing.T) {
	for _, code := range []string{
		"x\t中文e\u0301\t9\t",
		strings.Repeat("123456789界\u0301\t", 30),
		strings.Repeat("中文🚀", 30),
	} {
		st := &codeBlockState{}
		st.sync(code)
		for cols := 8; cols <= 22; cols++ {
			v := &txView{st: &txvState{wrapCols: cols}, wrap: true, index: st.wrapBoundaries, cells: func(int) int { return int(st.cells[0]) }}
			for seg := 0; seg*cols < txvCells(code); seg++ {
				a, b, r := v.segment(code, seg)
				gotA, gotB, gotR := v.segmentAt(0, seg, code)
				if a != gotA || b != gotB || r != gotR {
					t.Fatalf("cols %d segment %d: got (%d,%d,%d), want (%d,%d,%d)", cols, seg, gotA, gotB, gotR, a, b, r)
				}
			}
		}
	}
}
