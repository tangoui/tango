package uihost_test

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/internal/uithread"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/uihost"
)

// Main-thread decisions of threaded windows read the UI state of the
// latest frame (design §5.7).

func snapshotOf(t *testing.T, r *ui.Runner) uihost.Snapshot {
	t.Helper()
	s, ok := uihost.SnapshotOf(r)
	if !ok {
		t.Fatal("runner has no UI thread")
	}
	return s
}

// The text-input state shortcuts match against is the one the latest frame
// published: a change on the UI thread counts from the next frame on.
func TestShortcutMatchUsesSnapshot(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	r, s, tc, cv := openThreaded(t, a, fb, func(c *ui.Context) {})
	for tc.flush(t) {
	}
	var editKey, cmdK int
	shortcut(t, s, "CmdOrCtrl+A", uihost.ShortcutOptions{}, func() { editKey++ })
	shortcut(t, s, "CmdOrCtrl+K", uihost.ShortcutOptions{}, func() { cmdK++ })
	counts := func() (int, int) {
		var e, k int
		fb.Do(func() { e, k = editKey, cmdK })
		return e, k
	}
	if snapshotOf(t, r).EditableFocus {
		t.Fatal("snapshot reports editable focus before any text field")
	}
	tc.th.Do(func() { r.Context().Window().SetTextInput(true, geom.Rect{W: 2, H: 16}) })
	press(cv, input.KeyA, cmd())
	if e, _ := counts(); e != 1 {
		t.Fatalf("before the next frame Cmd+A fired %d times, want 1 (the snapshot still has no text field)", e)
	}
	tc.tick(t) // publishes editable focus
	if !snapshotOf(t, r).EditableFocus {
		t.Fatal("frame did not publish editable focus")
	}
	press(cv, input.KeyA, cmd())
	press(cv, input.KeyK, cmd())
	if e, k := counts(); e != 1 || k != 1 {
		t.Fatalf("with a text field focused: Cmd+A %d (want 1), Cmd+K %d (want 1)", e, k)
	}
	// A real text field: focus and composition come from the runner.
	r2, s2, tc2, cv2 := openThreaded(t, a, fb, (&keyPage{focus: true}).page)
	for tc2.flush(t) {
	}
	if st := snapshotOf(t, r2); !st.EditableFocus || st.Composing {
		t.Fatalf("focused input: snapshot %+v", st)
	}
	n := 0
	shortcut(t, s2, "CmdOrCtrl+C", uihost.ShortcutOptions{}, func() { n++ })
	press(cv2, input.KeyC, cmd())
	cv2.Input(input.Composition{Text: "に", Cursor: 1})
	for tc2.flush(t) {
	}
	if !snapshotOf(t, r2).Composing {
		t.Fatal("snapshot does not report the composition")
	}
	fb.Do(func() {
		if n != 0 {
			t.Errorf("Cmd+C fired %d times in a focused text field", n)
		}
	})
}

// A key a shortcut takes (press, repeats, release) is never queued to the
// UI thread; other keys are.
func TestShortcutConsumedKeyNeverReachesUI(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	var mu sync.Mutex
	var got []input.KeyEvent
	_, s, tc, cv := openThreaded(t, a, fb, func(c *ui.Context) {
		b := ui.Div(c, layout.Size(50, 50))
		b.Focusable(input.CursorDefault)
		c.SetFocus(b.ID())
		c.TerminalEvents(b.ID(), func(ev input.Event) bool { // every queued event
			if k, ok := ev.(input.KeyEvent); ok {
				mu.Lock()
				got = append(got, k)
				mu.Unlock()
			}
			return true
		})
	})
	for tc.flush(t) {
	}
	fired, onMain := 0, false
	main := mainGID(a)
	shortcut(t, s, "CmdOrCtrl+K", uihost.ShortcutOptions{}, func() { fired++; onMain = uithread.GoroutineID() == main })
	cv.Input(input.KeyEvent{Key: input.KeyK, Mods: cmd(), Down: true})
	cv.Input(input.KeyEvent{Key: input.KeyK, Mods: cmd(), Down: true, Repeat: true})
	cv.Input(input.KeyEvent{Key: input.KeyK, Mods: cmd()})
	press(cv, input.KeyF6, 0)
	tc.sync(t)
	fb.Do(func() {
		if fired != 1 || !onMain {
			t.Errorf("shortcut fired %d times (on main: %v)", fired, onMain)
		}
	})
	mu.Lock()
	defer mu.Unlock()
	if slices.ContainsFunc(got, func(k input.KeyEvent) bool { return k.Key == input.KeyK }) {
		t.Fatalf("consumed key queued to the UI: %v", got)
	}
	if len(got) != 2 || got[0].Key != input.KeyF6 {
		t.Fatalf("UI got %v, want the F6 press and release", got)
	}
}

// A press on a title bar's drag area moves the window from the main thread,
// in the press's own callback; the UI never sees it. A double click there
// restores or maximizes; presses elsewhere (the window buttons) still go to
// the UI.
func TestDragRegionStartsMoveOnMain(t *testing.T) {
	a, fb := faketest.Run(t, tango.Options{})
	var mu sync.Mutex
	presses := 0
	r, s, tc, _ := openThreaded(t, a, fb, func(c *ui.Context) {
		ui.TitleBar(c).Title("App").Controls(ui.WindowControlsCaption)
		if c.PressedOutside() { // any primary press on the page
			mu.Lock()
			presses++
			mu.Unlock()
		}
	})
	for tc.flush(t) {
	}
	bar, maxBtn := geom.Pt(30, 18), geom.Pt(200-46-23, 18) // the window is 200 wide
	regions := snapshotOf(t, r).DragRegions
	in := func(p geom.Point) bool {
		return slices.ContainsFunc(regions, func(r geom.Rect) bool { return r.Contains(p) })
	}
	if !in(bar) || in(maxBtn) {
		t.Fatalf("drag regions %v: want the bar at %v without the maximize button at %v", regions, bar, maxBtn)
	}
	moves := func() int { _, _, _, m := fb.Canvas(s.ID()).Surface().WindowCounts(); return m }
	fb.Do(func() {
		tc.sink.CanvasInput(input.Pointer{Kind: input.Press, Pos: bar, Button: input.ButtonLeft})
		if m := moves(); m != 1 {
			t.Errorf("moves after the press, in its callback: %d, want 1", m)
		}
	})
	fb.Do(func() { tc.sink.CanvasInput(input.Pointer{Kind: input.Release, Pos: bar, Button: input.ButtonLeft}) })
	for tc.flush(t) {
	}
	syncAppPosts(a)
	mu.Lock()
	n := presses
	mu.Unlock()
	if m := moves(); m != 1 || n != 0 {
		t.Fatalf("after the frames: moves %d (want 1), UI presses %d (want 0)", m, n)
	}
	// The maximize button is not a drag area: the UI handles it.
	fb.Do(func() {
		tc.sink.CanvasInput(input.Pointer{Kind: input.Move, Pos: maxBtn})
		tc.sink.CanvasInput(input.Pointer{Kind: input.Press, Pos: maxBtn, Button: input.ButtonLeft})
		tc.sink.CanvasInput(input.Pointer{Kind: input.Release, Pos: maxBtn, Button: input.ButtonLeft})
	})
	for tc.flush(t) {
	}
	syncAppPosts(a)
	if st := tango.Await(a, s.State); st != tango.StateMaximized {
		t.Fatalf("maximize button: state %v", st)
	}
	// A double click on the bar restores, on main as well.
	for tc.flush(t) {
	}
	time.Sleep(time.Millisecond)
	fb.Do(func() {
		for range 2 {
			tc.sink.CanvasInput(input.Pointer{Kind: input.Press, Pos: bar, Button: input.ButtonLeft})
			tc.sink.CanvasInput(input.Pointer{Kind: input.Release, Pos: bar, Button: input.ButtonLeft})
		}
		if st := s.State(); st != tango.StateNormal {
			t.Errorf("double click on the bar: state %v, want restored", st)
		}
	})
	if m := moves(); m != 2 {
		t.Fatalf("moves after the double click: %d, want 2", m)
	}
}
