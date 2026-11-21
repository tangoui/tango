package terminal

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/terminal/vt"
)

func TestNativePasteCanBeDeferredAndRewritten(t *testing.T) {
	var submit func(string)
	g := newViewRig(t, 480, 160, nil, func(w *ViewW) {
		w.OnPaste(func(text string, accept func(string)) {
			if text != "raw" {
				t.Fatalf("clipboard: %q", text)
			}
			// The guard may read the emulator.
			w.t.Lock()
			w.t.Unlock()
			submit = accept
		})
	})
	g.focus()
	g.d.SetClipboard("raw")
	g.d.Inject(input.KeyEvent{Key: input.KeyV, Code: input.KeyPaste, Down: true, Mods: input.ModCtrl})
	g.d.Frame()
	if submit == nil || g.input() != "" {
		t.Fatalf("paste was not deferred: handler=%v input=%q", submit != nil, g.input())
	}
	g.term.feed([]byte("\x1b[?2004h"))
	submit("edited\nnext")
	submit("duplicate")
	g.waitInput("\x1b[200~edited\rnext\x1b[201~")
}

func TestEchoConfirmationAndReconciliation(t *testing.T) {
	v := vt.New(vt.Options{Cols: 14, Rows: 3})
	var e echoState
	now := time.Unix(1, 0)
	add := func(s string) { e.input(v, []byte(s), true, now) }
	output := func(s string) {
		v.Write([]byte(s))
		e.output(v, []byte(s), now)
	}
	want := func(s string) {
		t.Helper()
		if p, _ := e.preview(v, now); p.text != s {
			t.Fatalf("preview %q, want %q", p.text, s)
		}
	}
	add("a")
	add("b")
	want("")
	output("a")
	want("b")
	if v.Screen().Line(0).Cells[1].Rune != 0 {
		t.Fatal("prediction entered the real screen")
	}
	add("c")
	output("b")
	want("c")
	output("c")
	want("")
	add("d")
	want("d")
	output("\r\nPassword: ")
	add("p")
	add("w")
	want("")
	now = now.Add(echoTimeout)
	want("")
	output("\x1b[?1049h")
	add("z")
	output("z")
	add("x")
	want("")
	output("\x1b[?1049l\x1bc")
	add("n")
	output("n")
	add("m")
	want("m")
	now = now.Add(echoTimeout)
	want("")
}

func TestLogicalLineRangesAndWrapPadding(t *testing.T) {
	v := vt.New(vt.Options{Cols: 5, Rows: 2})
	v.Write([]byte("abcd界e\u0301\r\nnext"))
	s := v.Screen()
	before := s.ViewOffset()
	l := logicalLine(s, int(s.Evicted())+1)
	if l.Text != "abcd界e\u0301" || s.ViewOffset() != before {
		t.Fatalf("logical line = %q, offset %d", l.Text, s.ViewOffset())
	}
	got, ok := l.Range(3, len(l.Text))
	if !ok || got != (CellRange{CellPosition{0, 3}, CellPosition{1, 3}}) {
		t.Fatalf("cell range = %+v, %v", got, ok)
	}
}

func TestLinkAtAndPrimaryClick(t *testing.T) {
	var opened []string
	g := newViewRig(t, 150, 160, nil, func(w *ViewW) {
		w.FontSize(16).OnOpenLink(func(target string) { opened = append(opened, target) })
	})
	const target = "https://example.com/some/path"
	g.term.feed([]byte("(" + target + ")"))
	g.d.Frame()
	r, m := g.vw.Box().Rect(), g.vw.vs.m
	pt := geom.Pt(r.X+2.5*m.cellW, r.Y+1.5*m.lineH)
	hit, ok := g.vw.LinkAt(pt)
	if !ok || hit.Target != target || hit.Range.Start != (CellPosition{0, 1}) || hit.Range.End.Row == 0 {
		t.Fatalf("link at %v = %+v, %v", pt, hit, ok)
	}
	g.d.Move(pt)
	if g.d.Cursor() != input.CursorPointer {
		t.Fatalf("hover cursor = %v", g.d.Cursor())
	}
	g.d.Drag(pt, pt.Add(geom.Pt(2*m.cellW, 0)))
	if len(opened) != 0 || g.vw.SelectedText() == "" {
		t.Fatalf("drag opened=%v selected=%q", opened, g.vw.SelectedText())
	}
	g.d.Advance(time.Second)
	g.d.Click(pt)
	if len(opened) != 1 || opened[0] != target {
		t.Fatalf("opened = %v", opened)
	}
}
