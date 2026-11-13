package editor

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/editor/buffer"
	"github.com/tangoui/tango/ui/editor/theme"
	"github.com/tangoui/tango/ui/input"
)

// A language round trip go → plain text → go keeps the one Highlighter, its
// background goroutine keeps working, and the tokens become final again
// (edits made while it was plain text included).
func TestLanguageRoundTripKeepsHighlighter(t *testing.T) {
	m := NewModel("a.go", strings.Repeat("x := `a` // c\n", 200), DefaultRegistry())
	st := NewView(m, nil)
	hl := m.Highlighter()
	if hl == nil {
		t.Fatal("no highlighter for Go")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	snaps := make(chan buffer.Snapshot, 1)
	ready := make(chan struct{}, 1)
	hl.Start(ctx, snaps, func() {
		select {
		case ready <- struct{}{}:
		default:
		}
	})
	snaps <- m.Snapshot()
	waitDone := func(ctx string) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for !hl.Done(m.Snapshot()) {
			if time.Now().After(deadline) {
				t.Fatalf("%s: background goroutine never finished", ctx)
			}
			select {
			case <-ready:
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
	waitDone("initial")

	m.SetLanguage(PlainText)
	if m.Highlighter() != nil || st.Highlighter() != nil {
		t.Fatal("plain text has a highlighter")
	}
	st.MoveCaret(0, false)
	st.Insert("/* open\n") // while plain text: not passed to the highlighter
	m.SetLanguage("go")
	if m.Highlighter() != hl {
		t.Fatal("round trip replaced the highlighter")
	}
	waitDone("after the round trip") // only the started goroutine tokenizes
	snap := m.Snapshot()
	lx, _ := DefaultRegistry().Lexer("go")
	want := fullTokens(lx, snap)
	for i := range want {
		got, final := hl.Tokens(snap, i, nil)
		if !final || !slices.Equal(got, want[i]) {
			t.Fatalf("line %d final=%v got %+v want %+v", i, final, got, want[i])
		}
	}
}

// The View's modifiers override State.Options for their frame only.
func TestModifiersArePerFrame(t *testing.T) {
	goText(t)
	st := NewState("hello", "a.txt", theme.Base("vs"))
	readOnly := true
	ed := newEditDriver(st, 500, 300, func(w *ViewW) {
		if readOnly {
			w.ReadOnly(true).TabSize(8).LineNumbers(false).FontSize(20)
		}
	})
	ed.focus(t)
	st.MoveCaret(5, false)
	ed.Type("x")
	if got := st.Snapshot().String(); got != "hello" {
		t.Fatalf("read-only frame edited: %q", got)
	}
	if st.Options != DefaultOptions() {
		t.Fatalf("modifiers wrote State.Options: %+v", st.Options)
	}
	if w := *ed.vw; !w.opts.ReadOnly || w.tabs() != 8 || w.opts.lineNumbers() || w.opts.fontSize() != 20 {
		t.Fatalf("frame options %+v", w.opts)
	}

	readOnly = false // the modifiers are no longer called: editable again
	ed.Frame()
	ed.Type("x")
	if got := st.Snapshot().String(); got != "hellox" {
		t.Fatalf("frame without ReadOnly(true): %q", got)
	}
	if w := *ed.vw; w.tabs() != DefaultTabSize || !w.opts.lineNumbers() || w.opts.fontSize() != DefaultFontSize {
		t.Fatalf("frame options %+v", w.opts)
	}

	st.Options.ReadOnly = true // the persistent setting
	ed.Frame()
	ed.Type("y")
	ed.Key(input.KeyBackspace, 0)
	if got := st.Snapshot().String(); got != "hellox" {
		t.Fatalf("State.Options.ReadOnly ignored: %q", got)
	}
}

// A subscriber that edits during a notification: the edit is queued, so a
// view that has not yet seen the change maps the two edits in order.
func TestReentrantEditQueued(t *testing.T) {
	m := NewModel("a.txt", "0123456789", nil)
	a := NewView(m, nil)
	b := NewView(m, nil)
	once := false
	m.Subscribe(func(ch ModelChange) {
		if ch.Source != a || once {
			return
		}
		once = true
		// Offsets in the text this listener sees ("012789").
		if err := b.Edit([]buffer.Edit{{Start: 0, End: 0, Text: "XYZ"}}, Selection{Anchor: 3, Head: 3}); err != nil {
			t.Error(err)
		}
		if got := m.Snapshot().String(); got != "012789" {
			t.Errorf("edit applied during the notification: %q", got)
		}
	})
	c := NewView(m, nil) // subscribed after the editing listener
	c.MoveCaret(5, false)
	a.MoveCaret(3, false)
	a.MoveCaret(7, true)
	a.Delete(Backward, Char)
	if got := m.Snapshot().String(); got != "XYZ012789" {
		t.Fatalf("text %q", got)
	}
	if c.Primary() != (Selection{Anchor: 6, Head: 6}) {
		t.Fatalf("third view %+v, want caret at 6 (collapsed to 3, then pushed by XYZ)", c.Primary())
	}
	if b.Primary() != (Selection{Anchor: 3, Head: 3}) || a.Primary() != (Selection{Anchor: 6, Head: 6}) {
		t.Fatalf("b %+v a %+v", b.Primary(), a.Primary())
	}

	// Two queued edits: the second is mapped through the first.
	m2 := NewModel("b.txt", "abc", nil)
	v := NewView(m2, nil)
	w := NewView(m2, nil)
	done := false
	m2.Subscribe(func(ch ModelChange) {
		if done {
			return
		}
		done = true
		w.Edit([]buffer.Edit{{Start: 0, End: 0, Text: "1"}}, Selection{Anchor: 1, Head: 1})
		w.Edit([]buffer.Edit{{Start: 2, End: 2, Text: "2"}}, Selection{Anchor: 3, Head: 3})
	})
	v.MoveCaret(3, false)
	// "abcd"; then "1abcd"; then "2" (at 2 in "abcd") mapped after "1ab".
	v.Insert("d")
	if got := m2.Snapshot().String(); got != "1ab2cd" {
		t.Fatalf("queued edits: %q", got)
	}
}

func TestOpenDuringDisposeCreatesFresh(t *testing.T) {
	p := NewModelPool(nil)
	const uri = "a.txt"
	m := p.Open(uri, "old")
	var fresh *Model
	p.OnWillDispose(func(d *Model) {
		if d == m {
			fresh = p.Open(uri, "new")
		}
	})
	p.Release(m)
	if fresh == nil || fresh == m || fresh.Snapshot().String() != "new" {
		t.Fatalf("Open during dispose returned %p (dying %p)", fresh, m)
	}
	if p.Get(uri) != fresh {
		t.Fatal("the fresh model is not in the pool")
	}
	p.Release(m) // the dead one: nothing
	if p.Get(uri) != fresh {
		t.Fatal("releasing the disposed model touched the fresh one")
	}
	p.Release(fresh)
	if p.Get(uri) != nil {
		t.Fatal("fresh model not disposed")
	}
}

// A view at line 0 stays at the top when lines are inserted above its
// content (Monaco keeps scrollTop 0); further down the first visible line
// follows its text.
func TestTopLineZeroNotMapped(t *testing.T) {
	m := NewModel("a.txt", strings.Repeat("x\n", 99), nil)
	a, b := NewView(m, nil), NewView(m, nil)
	b.Viewport = Viewport{TopLine: 0, Height: 95, Width: 200, LineHeight: 10}
	a.Insert("1\n2\n")
	if b.Viewport.TopLine != 0 {
		t.Fatalf("TopLine %d, want 0", b.Viewport.TopLine)
	}
}

func TestRedoFromOtherView(t *testing.T) {
	m := NewModel("a.txt", "hello", nil)
	a, b := NewView(m, nil), NewView(m, nil)
	a.MoveCaret(5, false)
	a.Insert(" world")
	b.MoveCaret(2, false)
	if !a.Undo() || m.Snapshot().String() != "hello" {
		t.Fatal("undo")
	}
	b.MoveCaret(5, false)
	if !b.Redo() || m.Snapshot().String() != "hello world" {
		t.Fatalf("B's redo: %q", m.Snapshot().String())
	}
	if b.Primary() != (Selection{Anchor: 11, Head: 11}) {
		t.Fatalf("B after its redo: %+v, want the step's after-selection 11", b.Primary())
	}
	if a.Primary() != (Selection{Anchor: 11, Head: 11}) {
		t.Fatalf("A after B's redo: %+v, want caret pushed to 11", a.Primary())
	}
}

func TestLanguageAndEOLChangeKeepSelections(t *testing.T) {
	m := NewModel("a.txt", "let x = 1\nlet y = 2\n", DefaultRegistry())
	a, b := NewView(m, nil), NewView(m, nil)
	a.MoveCaret(4, false)
	b.MoveCaret(12, false)
	b.MoveCaret(15, true)
	sa, sb := a.Primary(), b.Primary()
	m.SetLanguage("typescript")
	m.SetEOL("\r\n")
	if a.Primary() != sa || b.Primary() != sb {
		t.Fatalf("selections moved: a %+v b %+v", a.Primary(), b.Primary())
	}
	if a.Highlighter() == nil || a.Highlighter() != b.Highlighter() {
		t.Fatal("views do not share the highlighter")
	}
	if !b.Undo() || m.EOL() != "\n" || a.Primary() != sa {
		t.Fatalf("undo of SetEOL: EOL %q a %+v", m.EOL(), a.Primary())
	}
}

func TestCloseAfterDispose(t *testing.T) {
	p := NewModelPool(nil)
	m := p.Open("a.txt", "x")
	st := NewView(m, nil)
	p.Release(m)
	st.Close()
	st.Close()
	other := p.Open("a.txt", "y")
	v := NewView(other, nil)
	v.Insert("z")
	v.Close()
}
