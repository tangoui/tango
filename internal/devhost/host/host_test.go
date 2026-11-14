package host

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/devhost"
	"github.com/tangoui/tango/internal/platform/fake"
	"github.com/tangoui/tango/internal/platform/fake/faketest"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

const wait = 5 * time.Second

type env struct {
	t    *testing.T
	app  *tango.App
	fb   *fake.Backend
	h    *Host
	addr string
	ctl  *devhost.Conn
	ctlc chan any
}

func start(t *testing.T) *env {
	t.Helper()
	a, fb := faketest.Run(t, tango.Options{})
	dir, err := os.MkdirTemp("", "dh")
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
	h := New(a)
	go h.Serve(ln)
	e := &env{t: t, app: a, fb: fb, h: h, addr: addr, ctlc: make(chan any, 100)}
	e.ctl, err = devhost.Dial(addr, wait)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := devhost.Handshake(e.ctl, devhost.RoleCtl); err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			m, err := e.ctl.Recv()
			if err != nil {
				return
			}
			e.ctlc <- m
		}
	}()
	t.Cleanup(func() { e.ctl.Close() })
	return e
}

// rawWorker speaks the worker protocol directly.
type rawWorker struct {
	t    *testing.T
	cl   *devhost.Client
	msgs chan any
	fws  map[int]*devhost.FrameWriter
	acks chan devhost.FrameAck
}

func (e *env) worker() *rawWorker {
	e.t.Helper()
	w := &rawWorker{t: e.t, msgs: make(chan any, 1000), fws: map[int]*devhost.FrameWriter{}, acks: make(chan devhost.FrameAck, 1000)}
	cl, err := devhost.Connect(e.addr, "", func(m any) {
		if a, ok := m.(devhost.FrameAck); ok {
			w.acks <- a
			return
		}
		w.msgs <- m
	})
	if err != nil {
		e.t.Fatal(err)
	}
	w.cl = cl
	e.t.Cleanup(func() { cl.Close() })
	return w
}

// next returns the next message for which keep is true.
func next[T any](w *rawWorker) T {
	w.t.Helper()
	timeout := time.After(wait)
	for {
		select {
		case m := <-w.msgs:
			if v, ok := m.(T); ok {
				return v
			}
		case <-timeout:
			var z T
			w.t.Fatalf("no %T", z)
			return z
		}
	}
}

func (w *rawWorker) open(title string, seq int) devhost.Opened {
	w.t.Helper()
	m, err := w.cl.Request(func(id int) any {
		return devhost.OpenWindow{Req: id, Seq: seq, Opts: devhost.WindowOpts{Title: title, Width: 300, Height: 200}}
	}, wait)
	if err != nil {
		w.t.Fatal(err)
	}
	o := m.(devhost.Opened)
	if o.Err != "" {
		w.t.Fatal(o.Err)
	}
	w.fws[o.Win] = devhost.NewFrameWriter(o.Win, "", w.cl.Send)
	return o
}

// frame answers the next FrameRequest for win with a solid frame.
func (w *rawWorker) frame(win int, c color.RGBA) {
	w.t.Helper()
	req := next[devhost.FrameRequest](w)
	if req.Win != win {
		w.t.Fatalf("request for window %d, want %d", req.Win, win)
	}
	img := image.NewRGBA(image.Rect(0, 0, devSize(req.W, req.Scale), devSize(req.H, req.Scale)))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	go func() {
		// Apply acks as the uihost client does.
		for a := range w.acks {
			w.fws[a.Win].Ack(a.Seg, a.Slot)
		}
	}()
	if err := w.fws[win].Write(img, req.Scale); err != nil {
		w.t.Fatal(err)
	}
}

func ctlNext[T any](e *env) T {
	e.t.Helper()
	timeout := time.After(wait)
	for {
		select {
		case m := <-e.ctlc:
			if v, ok := m.(T); ok {
				return v
			}
		case <-timeout:
			var z T
			e.t.Fatalf("ctl: no %T", z)
			return z
		}
	}
}

func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(wait)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func pixel(cv *fake.Canvas, x, y int) color.RGBA {
	img := cv.LastFrame()
	if img == nil {
		return color.RGBA{}
	}
	return img.RGBAAt(x, y)
}

var (
	red  = color.RGBA{200, 0, 0, 255}
	blue = color.RGBA{0, 0, 200, 255}
)

func (e *env) canvas() *fake.Canvas {
	e.t.Helper()
	ss := e.fb.Surfaces()
	if len(ss) == 0 {
		e.t.Fatal("no host surface")
	}
	return e.fb.Canvas(ss[0].ID())
}

func TestSwapKeepsWindow(t *testing.T) {
	e := start(t)
	w1 := e.worker()
	o1 := w1.open("Main", 0)
	if o1.W != 300 || o1.H != 200 || o1.Scale != 1 {
		t.Fatalf("opened %+v", o1)
	}
	w1.frame(o1.Win, red)
	cv := e.canvas()
	eventually(t, "first frame", func() bool { return pixel(cv, 10, 10) == red })
	if s := ctlNext[devhost.Swapped](e); s.Old != 0 || s.New != os.Getpid() {
		t.Fatalf("first swap %+v", s)
	}

	// Input reaches the active worker.
	at := geom.Point{X: 5, Y: 6}
	cv.Input(input.Pointer{Kind: input.Move, Pos: at})
	if in := next[devhost.Input](w1); in.Win != o1.Win || in.Ev != (input.Pointer{Kind: input.Move, Pos: at}) {
		t.Fatalf("input %+v", in)
	}

	// A new worker asks for the old one's state.
	w2 := e.worker()
	snapc := make(chan devhost.SnapshotData, 1)
	go func() {
		m, err := w2.cl.Request(func(id int) any { return devhost.SnapshotGet{Req: id} }, wait)
		if err != nil {
			t.Error(err)
			return
		}
		snapc <- m.(devhost.SnapshotData)
	}()
	req := next[devhost.SnapshotReq](w1)
	entries := []devhost.Entry{{Key: "main.state#0", Type: "main.state", Data: []byte{9}}}
	w1.cl.Send(devhost.SnapshotReply{Req: req.Req, Entries: entries})
	if s := <-snapc; len(s.Entries) != 1 || s.Entries[0].Key != "main.state#0" {
		t.Fatalf("snapshot %+v", s)
	}
	// Input after the snapshot is held back for the new worker.
	key := input.KeyEvent{Key: input.KeyA, Down: true}
	cv.Input(key)

	o2 := w2.open("Main", 0)
	if o2.Win != o1.Win {
		t.Fatalf("new worker got window %d, want %d", o2.Win, o1.Win)
	}
	if n := len(e.fb.Surfaces()); n != 1 {
		t.Fatalf("%d host surfaces", n)
	}
	// The old worker still draws until the new one has a frame.
	if pixel(cv, 10, 10) != red {
		t.Fatal("window changed before the new worker's first frame")
	}
	w2.frame(o2.Win, blue)
	eventually(t, "new worker's frame", func() bool { return pixel(cv, 10, 10) == blue })
	if s := ctlNext[devhost.Swapped](e); s.Old != os.Getpid() || s.New != os.Getpid() {
		t.Fatalf("swap %+v", s)
	}
	select {
	case <-w1.cl.Done():
	case <-time.After(wait):
		t.Fatal("old worker not disconnected")
	}
	if in := next[devhost.Input](w2); in.Ev != key {
		t.Fatalf("held input %+v", in)
	}
	if cv.Surface().Closed() {
		t.Fatal("window closed")
	}

	// Error banner over the held frame, then cleared.
	e.ctl.Send(devhost.Status{Kind: "build", Text: "main.go:3:1: syntax error"})
	eventually(t, "banner", func() bool { return pixel(cv, 100, 0) == color.RGBA{0xff, 0x4d, 0x4f, 0xff} })
	if pixel(cv, 10, 190) != blue {
		t.Fatal("banner covered the whole frame")
	}
	e.ctl.Send(devhost.Status{})
	eventually(t, "frame wanted", func() bool { return cv.FlushFrame() })
	w2.frame(o2.Win, blue)
	eventually(t, "banner cleared", func() bool { return pixel(cv, 100, 0) == blue })

	// A user close asks the worker, which lets the window close.
	e.fb.RequestClose(cv.Surface().ID())
	if m := next[devhost.CloseRequest](w2); m.Win != o2.Win {
		t.Fatalf("close request %+v", m)
	}
	if cv.Surface().Closed() {
		t.Fatal("window closed before the worker agreed")
	}
	w2.cl.Send(devhost.CloseWindow{Win: o2.Win})
	if m := next[devhost.WindowClosed](w2); m.Win != o2.Win {
		t.Fatalf("closed %+v", m)
	}
}

func TestPendingWorkerDiesKeepsOld(t *testing.T) {
	e := start(t)
	w1 := e.worker()
	o1 := w1.open("Main", 0)
	w1.frame(o1.Win, red)
	cv := e.canvas()
	eventually(t, "first frame", func() bool { return pixel(cv, 1, 1) == red })
	ctlNext[devhost.Swapped](e)

	w2 := e.worker()
	go w2.cl.Request(func(id int) any { return devhost.SnapshotGet{Req: id} }, wait)
	req := next[devhost.SnapshotReq](w1)
	w1.cl.Send(devhost.SnapshotReply{Req: req.Req})
	o2 := w2.open("Main", 0)
	cv.Input(input.TextInput{Text: "x"}) // held: w1 is frozen
	w2.cl.Close()                        // crashes before its first frame
	if l := ctlNext[devhost.WorkerLeft](e); l.Active {
		t.Fatalf("left %+v", l)
	}
	// The old worker gets the held input and keeps the window.
	if in := next[devhost.Input](w1); in.Ev != (input.TextInput{Text: "x"}) || in.Win != o2.Win {
		t.Fatalf("replayed %+v", in)
	}
	cv.Input(input.TextInput{Text: "y"})
	if in := next[devhost.Input](w1); in.Ev != (input.TextInput{Text: "y"}) {
		t.Fatalf("input %+v", in)
	}
	if cv.Surface().Closed() || pixel(cv, 1, 1) != red {
		t.Fatal("window lost")
	}

	// The active worker crashing keeps the window on screen.
	w1.cl.Close()
	if l := ctlNext[devhost.WorkerLeft](e); !l.Active {
		t.Fatalf("left %+v", l)
	}
	e.ctl.Send(devhost.Status{Kind: "crash", Text: "exit status 2"})
	eventually(t, "crash banner", func() bool { return pixel(cv, 100, 0) == color.RGBA{0xff, 0x4d, 0x4f, 0xff} })
	// A new worker takes the window over.
	w3 := e.worker()
	o3 := w3.open("Main", 0)
	if o3.Win != o1.Win {
		t.Fatalf("window %d, want %d", o3.Win, o1.Win)
	}
	w3.frame(o3.Win, blue)
	eventually(t, "recovered", func() bool { return pixel(cv, 1, 150) == blue })
}

func TestWorkerClosesWindowAndNewWindows(t *testing.T) {
	e := start(t)
	w1 := e.worker()
	a := w1.open("A", 0)
	b := w1.open("B", 1)
	if a.Win == b.Win {
		t.Fatal("same window")
	}
	w1.frame(a.Win, red)
	eventually(t, "two windows", func() bool { return len(e.fb.Surfaces()) == 2 })
	ctlNext[devhost.Swapped](e)
	w1.cl.Send(devhost.CloseWindow{Win: b.Win})
	eventually(t, "B closed", func() bool { return e.fb.Surfaces()[1].Closed() })

	// The new worker renames A (matched by sequence number) and does not
	// reopen anything else.
	w2 := e.worker()
	o := w2.open("A2", 0)
	if o.Win != a.Win {
		t.Fatalf("window %d, want %d", o.Win, a.Win)
	}
	w2.frame(o.Win, blue)
	cv := e.fb.Canvas(e.fb.Surfaces()[0].ID())
	eventually(t, "swap", func() bool { return pixel(cv, 1, 1) == blue })
	eventually(t, "title", func() bool { return e.fb.Surfaces()[0].Title() == "A2" })
}

func TestCursorAndTextInputFollowOwner(t *testing.T) {
	e := start(t)
	w1 := e.worker()
	o := w1.open("Main", 0)
	w1.frame(o.Win, red)
	cv := e.canvas()
	eventually(t, "frame", func() bool { return pixel(cv, 1, 1) == red })
	w1.cl.Send(devhost.SetCursor{Win: o.Win, Cursor: input.CursorText})
	caret := geom.Rect{X: 1, Y: 2, W: 1, H: 14}
	w1.cl.Send(devhost.SetTextInput{Win: o.Win, Enabled: true, Caret: caret})
	eventually(t, "cursor", func() bool { return cv.Cursor() == input.CursorText })
	eventually(t, "text input", func() bool { on, c := cv.TextInput(); return on && c == caret })
	w1.cl.Send(devhost.Invalidate{Win: o.Win})
	cv.Frame()
	next[devhost.FrameRequest](w1)
}

func TestStateAcrossHostRestart(t *testing.T) {
	// tan dev asks the old host for the state...
	e := start(t)
	w1 := e.worker()
	o := w1.open("Main", 0)
	w1.frame(o.Win, red)
	ctlNext[devhost.Swapped](e)
	e.ctl.Send(devhost.SnapshotGet{Req: 7})
	req := next[devhost.SnapshotReq](w1)
	entries := []devhost.Entry{{Key: "main.state#0", Type: "main.state", Data: []byte{1, 2}}}
	w1.cl.Send(devhost.SnapshotReply{Req: req.Req, Entries: entries})
	d := ctlNext[devhost.SnapshotData](e)
	if d.Req != 7 || len(d.Entries) != 1 {
		t.Fatalf("ctl snapshot %+v", d)
	}
	// ... and seeds the new host with it; the first worker gets it once.
	e2 := start(t)
	e2.ctl.Send(devhost.SnapshotData{Entries: d.Entries})
	// The control and worker connections are served independently: let the
	// seed land before the worker asks (tan dev starts the app well after).
	eventually(t, "seed", func() bool {
		n := make(chan int, 1)
		e2.h.post(func() { n <- len(e2.h.seed) })
		return <-n == 1
	})
	nw := e2.worker()
	for i, want := range []int{1, 0} {
		m, err := nw.cl.Request(func(id int) any { return devhost.SnapshotGet{Req: id} }, wait)
		if err != nil {
			t.Fatal(err)
		}
		if got := len(m.(devhost.SnapshotData).Entries); got != want {
			t.Fatalf("request %d: %d entries, want %d", i, got, want)
		}
	}
}

// get asks the host for window win's state.
func (w *rawWorker) get(win int) devhost.SurfaceInfo {
	w.t.Helper()
	m, err := w.cl.Request(func(id int) any { return devhost.SurfaceOp{Req: id, Win: win, Op: devhost.OpGet} }, wait)
	if err != nil {
		w.t.Fatal(err)
	}
	return m.(devhost.SurfaceInfo)
}

func TestSurfaceStateAcrossSwap(t *testing.T) {
	e := start(t)
	w1 := e.worker()
	o1 := w1.open("Main", 0)
	w1.frame(o1.Win, red)
	cv := e.canvas()
	hs := cv.Surface()
	eventually(t, "first frame", func() bool { return pixel(cv, 10, 10) == red })
	ctlNext[devhost.Swapped](e)

	// The on-screen worker's calls apply at once.
	w1.cl.Send(devhost.SurfaceOp{Win: o1.Win, Op: devhost.OpSetTitle, Title: "Counter 1"})
	w1.cl.Send(devhost.SurfaceOp{Win: o1.Win, Op: devhost.OpSetBounds, Rect: devhost.Rect{X: 20, Y: 30, W: 420, H: 260}})
	if i := w1.get(o1.Win); i.Title != "Counter 1" || i.Bounds != (devhost.Rect{X: 20, Y: 30, W: 420, H: 260}) || i.W != 420 {
		t.Fatalf("info %+v", i)
	}
	// Host window events reach it.
	e.fb.Move(hs.ID(), 70, 80)
	if ev := next[devhost.SurfaceEvent](w1); ev.Kind != devhost.EvMoved || ev.X != 70 || ev.Y != 80 {
		t.Fatalf("event %+v", ev)
	}
	// Its menu shows on the host; clicks come back.
	w1.cl.Send(devhost.SetMenu{Items: []devhost.MenuItem{{Label: "App", Submenu: []devhost.MenuItem{{ID: 7, Label: "Old"}}}}})
	eventually(t, "menu", func() bool { m := e.fb.Menu(0); return len(m) == 1 && m[0].Submenu[0].Label == "Old" })
	e.fb.ClickMenu(0, 7)
	if c := next[devhost.MenuClick](w1); c.ID != 7 || c.Win != 0 {
		t.Fatalf("click %+v", c)
	}

	// A new worker sees the current window state; its calls wait for the
	// swap.
	w2 := e.worker()
	o2 := w2.open("Main", 0)
	if o2.Win != o1.Win {
		t.Fatalf("new worker got window %d", o2.Win)
	}
	if i := w2.get(o2.Win); i.Title != "Main" && i.Title != "Counter 1" || i.Bounds.X != 70 || i.W != 420 {
		t.Fatalf("new worker sees %+v", i)
	}
	w2.cl.Send(devhost.SurfaceOp{Win: o2.Win, Op: devhost.OpSetTitle, Title: "Counter 2"})
	w2.cl.Send(devhost.SetMenu{Items: []devhost.MenuItem{{Label: "App", Submenu: []devhost.MenuItem{{ID: 9, Label: "New"}}}}})
	w2.get(o2.Win) // the calls above were handled
	if hs.Title() == "Counter 2" || e.fb.Menu(0)[0].Submenu[0].Label != "Old" {
		t.Fatal("pending worker's calls applied before the swap")
	}
	w2.frame(o2.Win, blue)
	eventually(t, "swap", func() bool { return pixel(cv, 10, 10) == blue })
	eventually(t, "title and menu after the swap", func() bool {
		m := e.fb.Menu(0)
		return hs.Title() == "Counter 2" && len(m) == 1 && m[0].Submenu[0].Label == "New"
	})
	e.fb.ClickMenu(0, 9)
	if c := next[devhost.MenuClick](w2); c.ID != 9 {
		t.Fatalf("click %+v", c)
	}
	e.fb.Resize(hs.ID(), 500, 300)
	if ev := next[devhost.SurfaceEvent](w2); ev.Kind != devhost.EvResized || ev.W != 500 {
		t.Fatalf("event %+v", ev)
	}

	// A worker that sets no menu gets the host's default back.
	w3 := e.worker()
	o3 := w3.open("Main", 0)
	w3.frame(o3.Win, red)
	eventually(t, "default menu", func() bool {
		m := e.fb.Menu(0)
		return len(m) == 0 || m[0].Label != "App"
	})
}

func TestHiddenWindow(t *testing.T) {
	e := start(t)
	w1 := e.worker()
	m, err := w1.cl.Request(func(id int) any {
		return devhost.OpenWindow{Req: id, Opts: devhost.WindowOpts{Title: "H", Width: 300, Height: 200, Hidden: true}}
	}, wait)
	if err != nil {
		t.Fatal(err)
	}
	o := m.(devhost.Opened)
	w1.fws[o.Win] = devhost.NewFrameWriter(o.Win, "", w1.cl.Send)
	hs := e.canvas().Surface()
	if hs.Visible() {
		t.Fatal("hidden window visible")
	}
	if i := w1.get(o.Win); i.Visible {
		t.Fatal("info says visible")
	}
	w1.cl.Send(devhost.SurfaceOp{Win: o.Win, Op: devhost.OpShow})
	eventually(t, "shown", hs.Visible)
}

// A host with a session token refuses peers that do not present it, on
// TCP (Windows) as on unix sockets.
func TestSessionToken(t *testing.T) {
	a, _ := faketest.Run(t, tango.Options{})
	ln, err := devhost.Listen("tcp:127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	h := New(a)
	h.Token = "s3cret"
	go h.Serve(ln)
	addr := "tcp:" + ln.Addr().String()
	for _, tc := range []struct {
		token string
		ok    bool
	}{{"", false}, {"wrong", false}, {"s3cre", false}, {"s3cret", true}} {
		c, err := devhost.Dial(addr, wait)
		if err != nil {
			t.Fatal(err)
		}
		_, err = devhost.HandshakeToken(c, devhost.RoleCtl, tc.token)
		c.Close()
		if (err == nil) != tc.ok {
			t.Errorf("token %q: handshake error %v, want ok=%v", tc.token, err, tc.ok)
		}
	}
	// The worker side reads the token from the environment.
	t.Setenv(devhost.EnvToken, "s3cret")
	c, err := devhost.Dial(addr, wait)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := devhost.Handshake(c, devhost.RoleCtl); err != nil {
		t.Fatalf("handshake with %s: %v", devhost.EnvToken, err)
	}
}
