//go:build tango_dev

package uihost_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/devhost"
	"github.com/tangoui/tango/internal/devhost/host"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/sys/menu"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/uihost"
)

type devState struct {
	Count int
	Text  string
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// startHost runs an in-process dev host on its own fake app.
func startHost(t *testing.T) (string, *fake.Backend) {
	t.Helper()
	ha, hfb := faketest.Run(t, tango.Options{})
	hfb.SetCanvasScale(2)
	dir, err := os.MkdirTemp("", "uh")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	addr := "unix:" + filepath.Join(dir, "s")
	ln, err := devhost.Listen(addr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go host.New(ha).Serve(ln)
	return addr, hfb
}

func TestRemoteWindow(t *testing.T) {
	addr, hfb := startHost(t)
	wa, wfb := faketest.Run(t, tango.Options{})
	if err := uihost.ConnectDev(addr, ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(uihost.ResetDev)

	st := devState{Text: "start"}
	uihost.Persist(&st) // first worker: nothing to restore
	if st.Text != "start" {
		t.Fatalf("state changed: %+v", st)
	}
	var size geom.Size
	var scale float32
	o := tango.Await(wa, func() opened {
		r, s, err := uihost.Open(wa, uihost.Options{Surface: tango.SurfaceOptions{Title: "Remote", Width: 300, Height: 200}},
			func(c *ui.Context) {
				size, scale = c.Size(), c.Window().Scale()
				if ui.Button(c, "OK").Block().Clicked() {
					st.Count++
				}
			}, ui.RunnerOptions{})
		return opened{r, s, err}
	})
	if o.err != nil {
		t.Fatal(o.err)
	}
	// The window is in the host; the worker has only a stand-in surface
	// without a native window.
	eventually(t, "host window", func() bool { return len(hfb.Surfaces()) == 1 })
	hs := hfb.Surfaces()[0]
	if hs.Title() != "Remote" {
		t.Fatalf("host title %q", hs.Title())
	}
	if o.s == nil || len(wfb.Surfaces()) != 0 {
		t.Fatal("worker stand-in surface missing or native")
	}
	cv := hfb.Canvas(hs.ID())
	eventually(t, "first frame", func() bool { w, h := cv.LastFrameSize(); return w == 600 && h == 400 })
	wfb.Do(func() {
		if size != (geom.Size{W: 300, H: 200}) || scale != 2 {
			t.Errorf("page saw %v at %v", size, scale)
		}
	})

	// Click the button through the host.
	at := geom.Point{X: 150, Y: 10}
	cv.Input(input.Pointer{Kind: input.Move, Pos: at})
	cv.Input(input.Pointer{Kind: input.Press, Pos: at, Button: input.ButtonLeft})
	cv.Input(input.Pointer{Kind: input.Release, Pos: at, Button: input.ButtonLeft})
	eventually(t, "click", func() bool {
		cv.FlushFrame()
		n := 0
		wfb.Do(func() { n = st.Count })
		return n == 1
	})
	eventually(t, "pointer cursor", func() bool { return cv.Cursor() == input.CursorPointer })

	// Clipboard goes through the host.
	got := tango.Await(wa, func() string {
		w := o.r.Context().Window()
		w.WriteClipboard("héllo")
		eventually(t, "clipboard write", func() bool { return hfb.ClipboardText() == "héllo" })
		return w.ReadClipboard()
	})
	if got != "héllo" {
		t.Fatalf("clipboard %q", got)
	}

	// A replacement worker gets the state.
	wfb.Do(func() { st.Text = "typed" })
	cl, err := devhost.Connect(addr, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	m, err := cl.Request(func(id int) any { return devhost.SnapshotGet{Req: id} }, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var restored devState
	if found, err := devhost.Decode(m.(devhost.SnapshotData).Entries, "uihost_test.devState#0", &restored); !found || err != nil {
		t.Fatalf("decode %v %v", found, err)
	}
	if restored != (devState{Count: 1, Text: "typed"}) {
		t.Fatalf("restored %+v", restored)
	}

	// Closing the host window closes the stand-in, and so the runner.
	closed := make(chan struct{})
	tango.Await(wa, func() struct{} { o.s.OnClosed(func() { close(closed) }); return struct{}{} })
	hfb.RequestClose(hs.ID())
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("stand-in surface not closed")
	}
}

// openRemote connects a worker to a fresh host and opens one window.
func openRemote(t *testing.T, so tango.SurfaceOptions) (*tango.App, *fake.Backend, *fake.Surface, opened) {
	t.Helper()
	addr, hfb := startHost(t)
	wa, _ := faketest.Run(t, tango.Options{})
	if err := uihost.ConnectDev(addr, ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(uihost.ResetDev)
	o := tango.Await(wa, func() opened {
		r, s, err := uihost.Open(wa, uihost.Options{Surface: so}, func(c *ui.Context) { ui.Text(c, "hi") }, ui.RunnerOptions{})
		return opened{r, s, err}
	})
	if o.err != nil {
		t.Fatal(o.err)
	}
	eventually(t, "host window", func() bool { return len(hfb.Surfaces()) == 1 })
	return wa, hfb, hfb.Surfaces()[0], o
}

func TestRemoteSurfaceCalls(t *testing.T) {
	wa, hfb, hs, o := openRemote(t, tango.SurfaceOptions{Title: "Calls", Width: 300, Height: 200, Hidden: true})
	s := o.s
	// Hidden: the host window waits for Show.
	cv := hfb.Canvas(hs.ID())
	eventually(t, "first frame", func() bool { w, _ := cv.LastFrameSize(); return w > 0 })
	if hs.Visible() || s.IsVisible() {
		t.Fatal("hidden window shown")
	}
	s.Show()
	eventually(t, "shown", func() bool { return hs.Visible() })
	if !s.IsVisible() {
		t.Fatal("IsVisible false after Show")
	}

	s.SetTitle("New title")
	eventually(t, "host title", func() bool { return hs.Title() == "New title" })
	if got := s.Title(); got != "New title" {
		t.Fatalf("Title %q", got)
	}
	s.SetSize(500, 320)
	if w, h := s.Size(); w != 500 || h != 320 {
		t.Fatalf("Size %dx%d", w, h)
	}
	if err := s.SetBounds(tango.Rect{X: 40, Y: 50, Width: 410, Height: 310}); err != nil {
		t.Fatal(err)
	}
	if b := s.Bounds(); b != (tango.Rect{X: 40, Y: 50, Width: 410, Height: 310}) {
		t.Fatalf("Bounds %+v", b)
	}
	if err := s.SetSizeLimits(100, 80, 900, 700); err != nil {
		t.Fatal(err)
	}
	if err := s.Center(); err != nil {
		t.Fatal(err)
	}
	s.SetState(tango.StateMaximized) // waits for nothing; the getter does
	if st := s.State(); st != tango.StateMaximized {
		t.Fatalf("State %v", st)
	}
	if err := s.SetAppearance(tango.Appearance{AlwaysOnTop: true, Opacity: 0.5}); err != nil {
		t.Fatal(err)
	}
	if !s.Appearance().AlwaysOnTop {
		t.Fatal("Appearance not remembered")
	}
	s.Focus()
	eventually(t, "host calls", func() bool {
		minW, _, maxW, _ := hs.SizeLimits()
		ap, _ := hs.CurrentAppearance()
		_, _, centers, _ := hs.WindowCounts()
		return minW == 100 && maxW == 900 && ap.AlwaysOnTop && ap.Opacity == 0.5 && centers == 1 && hs.Focuses() == 1
	})
	s.Hide()
	eventually(t, "hidden", func() bool { return !hs.Visible() })
	_ = wa
}

func TestRemoteSurfaceEvents(t *testing.T) {
	wa, hfb, hs, o := openRemote(t, tango.SurfaceOptions{Title: "Events", Width: 300, Height: 200})
	type evs struct {
		resized, moved [2]int
		state          tango.WindowState
		focused        int
		blurred        int
	}
	var mu = make(chan evs, 1)
	mu <- evs{}
	upd := func(f func(*evs)) { e := <-mu; f(&e); mu <- e }
	get := func() evs { e := <-mu; mu <- e; return e }
	s := o.s
	s.OnResized(func(w, h int) { upd(func(e *evs) { e.resized = [2]int{w, h} }) })
	s.OnMoved(func(x, y int) { upd(func(e *evs) { e.moved = [2]int{x, y} }) })
	s.OnStateChanged(func(st tango.WindowState) { upd(func(e *evs) { e.state = st }) })
	s.OnFocused(func() { upd(func(e *evs) { e.focused++ }) })
	s.OnBlurred(func() { upd(func(e *evs) { e.blurred++ }) })

	hfb.Resize(hs.ID(), 700, 500)
	hfb.Move(hs.ID(), 30, 60)
	hfb.ChangeState(hs.ID(), tango.StateMaximized)
	hfb.FocusChange(hs.ID(), true)
	hfb.FocusChange(hs.ID(), false)
	eventually(t, "events in the worker", func() bool {
		e := get()
		return e.resized == [2]int{700, 500} && e.moved == [2]int{30, 60} && e.state == tango.StateMaximized &&
			e.focused == 1 && e.blurred == 1
	})
	if w, h := s.Size(); w != 700 || h != 500 {
		t.Fatalf("Size %dx%d after host resize", w, h)
	}

	// OnClose can keep the window open.
	allow := false
	asked := make(chan struct{}, 10)
	tango.Await(wa, func() struct{} {
		s.OnClose(func() bool { asked <- struct{}{}; return allow })
		return struct{}{}
	})
	closed := make(chan struct{})
	s.OnClosed(func() { close(closed) })
	hfb.RequestClose(hs.ID())
	select {
	case <-asked:
	case <-time.After(5 * time.Second):
		t.Fatal("OnClose not asked")
	}
	time.Sleep(50 * time.Millisecond)
	if hs.Closed() {
		t.Fatal("host window closed although OnClose refused")
	}
	tango.Await(wa, func() struct{} { allow = true; return struct{}{} })
	hfb.RequestClose(hs.ID())
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("stand-in not closed")
	}
	eventually(t, "host window closed", hs.Closed)
}

func TestRemoteSurfaceClose(t *testing.T) {
	_, _, hs, o := openRemote(t, tango.SurfaceOptions{Title: "Close", Width: 300, Height: 200})
	closed := make(chan struct{})
	o.s.OnClosed(func() { close(closed) })
	o.s.Close() // from the app: the host window goes too
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("stand-in not closed")
	}
	eventually(t, "host window closed", hs.Closed)
}

func TestRemoteMenus(t *testing.T) {
	wa, hfb, hs, o := openRemote(t, tango.SurfaceOptions{Title: "Menus", Width: 300, Height: 200})
	clicks := make(chan string, 10)
	m := menu.New()
	file := m.Submenu("File")
	file.Add("Hello", func() { clicks <- "hello" })
	chk := file.Check("Flag", false, nil)
	if err := menu.SetAppMenu(wa, m); err != nil {
		t.Fatal(err)
	}
	var hello, flag uint32
	eventually(t, "host app menu", func() bool {
		items := hfb.Menu(0)
		if len(items) != 1 || items[0].Label != "File" || len(items[0].Submenu) != 2 {
			return false
		}
		hello, flag = items[0].Submenu[0].ID, items[0].Submenu[1].ID
		return items[0].Submenu[0].Label == "Hello"
	})
	if !hfb.ClickMenu(0, hello) {
		t.Fatal("no click router on the host")
	}
	select {
	case c := <-clicks:
		if c != "hello" {
			t.Fatal(c)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("click did not reach the worker")
	}
	// A check item toggles in the worker and the host menu is updated.
	hfb.ClickMenu(0, flag)
	eventually(t, "checked", func() bool {
		items := hfb.Menu(0)
		return chk.IsChecked() && len(items) == 1 && len(items[0].Submenu) == 2 && items[0].Submenu[1].Checked
	})

	// The window menu goes to the host window.
	wm := menu.New()
	wm.Add("Win item", func() { clicks <- "win" })
	if err := menu.SetSurfaceMenu(wa, o.s, wm); err != nil {
		t.Fatal(err)
	}
	var id uint32
	eventually(t, "host window menu", func() bool {
		items := hfb.Menu(hs.ID())
		if len(items) == 1 && items[0].Label == "Win item" {
			id = items[0].ID
			return true
		}
		return false
	})
	hfb.ClickMenu(hs.ID(), id)
	select {
	case c := <-clicks:
		if c != "win" {
			t.Fatal(c)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("window menu click did not reach the worker")
	}

	// Clearing goes back to the host's default.
	if err := menu.SetAppMenu(wa, nil); err != nil {
		t.Fatal(err)
	}
	eventually(t, "app menu cleared", func() bool {
		items := hfb.Menu(0)
		return len(items) == 0 || items[0].Label != "File"
	})
}
