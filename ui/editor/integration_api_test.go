package editor

import (
	"slices"
	"testing"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

func TestSuggestDetailsAndAccept(t *testing.T) {
	st := NewState("word", "", nil)
	t.Cleanup(st.Close)
	selected, accepted := 0, -1
	open := false
	labels := []string{"first", "second", "third"}
	ed := newEditDriver(st, 640, 220, func(w *ViewW) {
		if open {
			w.Suggest(SuggestOptions{Labels: labels, Selected: &selected, Accept: func(i int) {
				accepted, open = i, false
			}, Details: func(i int) { ui.Text(w.c, "Detail: "+labels[i]) }})
		}
	})
	ed.focus(t)
	open = true
	ed.Frame()
	ed.Key(input.KeyDown, 0)
	if _, ok := ed.FindText("Detail: second"); selected != 1 || !ok {
		t.Fatalf("selected = %d, second detail visible = %t", selected, ok)
	}
	ed.Key(input.KeyTab, 0)
	if accepted != 1 || st.Snapshot().String() != "word" {
		t.Fatalf("accepted = %d, text = %q", accepted, st.Snapshot().String())
	}
}

func TestEditorMountUsesDocumentOffset(t *testing.T) {
	st := NewState("first\nsecond", "", nil)
	t.Cleanup(st.Close)
	var anchor geom.Rect
	var ok bool
	ed := newEditDriver(st, 400, 200, func(w *ViewW) {
		anchor, ok = w.OffsetAnchor(6)
		if off, hit := w.HoverOffset(); hit && off == 6 {
			w.MountContent(off, ui.BottomStart, func() { ui.Text(w.c, "Hover") }, layout.W(100))
		}
	})
	ed.Move(ed.at(t, st, 1, 0))
	r, found := ed.FindText("Hover")
	if !ok || !found || r.Y < anchor.Bottom() || r.X < anchor.X {
		t.Fatalf("anchor %v (%t), popup %v (%t)", anchor, ok, r, found)
	}
}

func TestCompletionInputAndAnchor(t *testing.T) {
	st := NewState("sel", "", nil)
	index, accepted, dismissed := 0, -1, false
	var changes []string
	ed := newEditDriver(st, 450, 200, func(w *ViewW) {
		w.OnDocumentChange(func(change ModelChange) {
			changes = append(changes, change.New.String())
			if st.Primary().Head != change.New.Len() {
				t.Error("change notification saw the old caret")
			}
		}).CompletionKeys(3, &index, func(i int) { accepted = i }, func() { dismissed = true })
	})
	ed.focus(t)
	st.MoveCaret(3, false)
	ed.Type("e")
	ed.Key(input.KeyDown, 0)
	ed.Key(input.KeyEnter, 0)
	ed.Key(input.KeyEscape, 0)
	if accepted != 1 || !dismissed || !slices.Equal(changes, []string{"sele"}) {
		t.Fatalf("accepted = %d, dismissed = %v, changes = %v", accepted, dismissed, changes)
	}
	w := *ed.vw
	r, ok := w.CompletionAnchor()
	if p := ed.at(t, st, 0, 4); !ok || !r.Contains(p) {
		t.Fatalf("completion anchor = %v, %v, want it to contain %v", r, ok, p)
	}
}

func TestGutterClickAndMapping(t *testing.T) {
	st := NewState("first line\nsecond line\nlast line", "", nil)
	var got []GutterClick
	ed := newEditDriver(st, 450, 220, func(w *ViewW) {
		w.OnGutterClick(func(click GutterClick) { got = append(got, click) })
	})
	w := *ed.vw
	p := geom.Pt(w.vs.gutter.X+3, ed.at(t, st, 1, 0).Y)
	if line, ok := w.GutterLineAtPoint(p); !ok || line != 1 {
		t.Fatalf("line = %d, %v", line, ok)
	}
	ed.Click(p)
	ed.RightClick(p)
	want := []GutterClick{{Line: 1, Position: p, Button: input.ButtonLeft}, {Line: 1, Position: p, Button: input.ButtonRight}}
	if !slices.Equal(got, want) || st.Primary().Head != 0 {
		t.Fatalf("clicks = %+v, caret = %d", got, st.Primary().Head)
	}
}

func TestInlayHintsWrapHitAndLifetime(t *testing.T) {
	st := NewState("ab cd ef\n", "", nil)
	st.Options.WordWrap, st.Options.WordWrapColumn = WrapColumn, 9
	ver := st.Snapshot().Version()
	if !st.SetInlayHints(ver, []InlayHint{{Offset: 3, Text: "type"}, {Offset: 3, Text: "!"}, {Offset: 8, Text: "end"}, {Offset: 9, Text: "empty"}}) {
		t.Fatal("current hints rejected")
	}
	var w *ViewW
	d := ui.NewDriver(func(c *ui.Context) { w = View(c, st) }, ui.DriverOptions{Width: 450, Height: 200})
	d.Frame()
	if st.breaks(0).rows() < 2 {
		t.Fatal("hint width did not participate in wrapping")
	}
	for _, off := range []int{3, 8, 9} {
		line, row := st.rowPos(off, false)
		lv := w.rowAt(st.Snapshot(), line, row)
		found := 0
		lv.eachGap(func(in injection, x0, x1 float32) {
			if in.col+st.Snapshot().LineStart(line) == off {
				found++
				p := geom.Pt(w.vs.text.X+(x0+x1)/2, w.vs.text.Y+(float32(lv.vi)+0.5)*st.Viewport.LineHeight)
				if got, ok := w.OffsetAtPoint(p); !ok || got != off {
					t.Errorf("hint at %d hit = %d, %v", off, got, ok)
				}
				if got, ok := w.InlayHintAtPoint(p); !ok || got != off {
					t.Errorf("hint query at %d = %d, %v", off, got, ok)
				}
			}
		})
		if found == 0 {
			t.Errorf("no gap at %d", off)
		}
	}
	st.Insert("x")
	if st.SetInlayHints(ver, []InlayHint{{Offset: 0, Text: "stale"}}) {
		t.Fatal("stale hints accepted")
	}
	d.Frame()
	if w.st.vl.cfg.hints != nil {
		t.Fatal("stale hints survived the edit")
	}
}
