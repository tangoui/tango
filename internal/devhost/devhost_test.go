package devhost

import (
	"image"
	"image/color"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

func pipe(t *testing.T) (*Conn, *Conn) {
	t.Helper()
	a, b := net.Pipe()
	t.Cleanup(func() { a.Close(); b.Close() })
	return NewConn(a), NewConn(b)
}

func TestMessagesRoundTrip(t *testing.T) {
	a, b := pipe(t)
	msgs := []any{
		Hello{Proto: ProtoVersion, Role: RoleWorker, PID: 42, Marker: Marker},
		OpenWindow{Req: 1, Seq: 2, Opts: WindowOpts{Title: "标题", Width: 800, Height: 600, Resizable: true, Background: 0xff0000ff}},
		Input{Win: 3, Ev: input.Pointer{Kind: input.Press, Pos: geom.Point{X: 1.5, Y: 2}, Button: input.ButtonLeft, Clicks: 2}},
		Input{Win: 3, Ev: input.KeyEvent{Key: input.KeyEnter, Down: true, Mods: input.ModShift | input.ModMeta}},
		Input{Win: 3, Ev: input.TextInput{Text: "你好"}},
		Input{Win: 3, Ev: input.Composition{Text: "ni", Cursor: 2}},
		Input{Win: 3, Ev: input.Scroll{Pos: geom.Point{X: 3}, DY: -4}},
		SetTextInput{Win: 3, Enabled: true, Caret: geom.Rect{X: 1, Y: 2, W: 3, H: 4}},
		FrameReady{Win: 3, Seg: 1, Slot: 1, W: 10, H: 20, Stride: 40, Scale: 2},
		SnapshotData{Req: 9, Entries: []Entry{{Key: "main.s#0", Type: "main.s", Data: []byte{1, 2, 3}}}},
		Status{Kind: "build", Text: "./main.go:1: syntax error"},
		Swapped{Old: 1, New: 2, Took: 300 * time.Millisecond},
	}
	go func() {
		for _, m := range msgs {
			if err := a.Send(m); err != nil {
				t.Error(err)
				return
			}
		}
	}()
	for i, want := range msgs {
		got, err := b.Recv()
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("message %d: got %#v, want %#v", i, got, want)
		}
	}
}

func TestHandshakeRefused(t *testing.T) {
	a, b := pipe(t)
	go func() {
		m, _ := b.Recv()
		if h, ok := m.(Hello); !ok || h.Role != RoleCtl || h.Proto != ProtoVersion || h.PID != os.Getpid() {
			t.Errorf("hello = %#v", m)
		}
		b.Send(Welcome{Err: "no"})
	}()
	if _, err := Handshake(a, RoleCtl); err == nil {
		t.Fatal("refusal not reported")
	}
}

func TestToken(t *testing.T) {
	tok, err := NewToken()
	if err != nil || len(tok) != 64 {
		t.Fatalf("NewToken = %q, %v", tok, err)
	}
	if tok2, _ := NewToken(); tok2 == tok {
		t.Fatal("tokens repeat")
	}
	if !TokenOK(tok, tok) || TokenOK("", tok) || TokenOK(tok[:63], tok) || !TokenOK("anything", "") {
		t.Fatal("TokenOK wrong")
	}
}

func TestListenUnixMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no unix socket modes")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "s")
	ln, err := Listen("unix:" + p)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm()&0o077 != 0 {
		t.Fatalf("socket mode %v, want owner-only", st.Mode().Perm())
	}
}

func TestListenDial(t *testing.T) {
	dir, err := os.MkdirTemp("", "dh")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	for _, addr := range []string{"unix:" + filepath.Join(dir, "s"), "tcp:127.0.0.1:0"} {
		ln, err := Listen(addr)
		if err != nil {
			t.Fatal(err)
		}
		if addr[:3] == "tcp" {
			addr = "tcp:" + ln.Addr().String()
		}
		go func() {
			c, err := ln.Accept()
			if err == nil {
				cc := NewConn(c)
				m, _ := cc.Recv()
				cc.Send(m)
			}
		}()
		c, err := Dial(addr, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		c.Send(Invalidate{Win: 7})
		if m, err := c.Recv(); err != nil || m != (Invalidate{Win: 7}) {
			t.Fatalf("%s: echo %v %v", addr, m, err)
		}
		c.Close()
		ln.Close()
	}
	if _, err := Listen("bogus"); err == nil {
		t.Fatal("bad address accepted")
	}
}

func TestSegmentShared(t *testing.T) {
	a, err := CreateSegment("", 1<<16)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := OpenSegment(a.Name, 1<<16)
	if err != nil {
		t.Fatal(err)
	}
	b.Unlink()
	if _, err := os.Stat(a.Name); !os.IsNotExist(err) {
		t.Fatalf("segment file still there: %v", err)
	}
	a.Bytes()[100] = 42
	b.Bytes()[200] = 7
	if b.Bytes()[100] != 42 || a.Bytes()[200] != 7 {
		t.Fatal("writes not shared")
	}
	b.Close()
	if _, err := OpenSegment(filepath.Join(os.TempDir(), "no-such-tango-seg"), 10); err == nil {
		t.Fatal("opened a missing segment")
	}
}

func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

// fakeHost records what a FrameWriter sends and maps segments as the host.
type fakeHost struct {
	mu     sync.Mutex
	segs   map[int]*Segment
	frames []FrameReady
}

func (h *fakeHost) send(m any) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch m := m.(type) {
	case ShmAttach:
		s, err := OpenSegment(m.Name, m.Size)
		if err != nil {
			return err
		}
		s.Unlink()
		h.segs[m.Seg] = s
	case FrameReady:
		h.frames = append(h.frames, m)
	}
	return nil
}

func (h *fakeHost) last(t *testing.T) (FrameReady, *image.RGBA) {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	f := h.frames[len(h.frames)-1]
	img, err := FrameImage(h.segs[f.Seg], f)
	if err != nil {
		t.Fatal(err)
	}
	return f, img
}

func TestFrameWriterDoubleBufferAndGrow(t *testing.T) {
	h := &fakeHost{segs: map[int]*Segment{}}
	fw := NewFrameWriter(5, "", h.send)
	defer fw.Close()
	red := color.RGBA{255, 0, 0, 255}
	if err := fw.Write(solid(4, 3, red), 2); err != nil {
		t.Fatal(err)
	}
	f1, img := h.last(t)
	if f1.Win != 5 || f1.Seg != 1 || f1.Slot != 0 || f1.W != 4 || f1.H != 3 || f1.Scale != 2 {
		t.Fatalf("frame 1 %+v", f1)
	}
	if img.RGBAAt(3, 2) != red {
		t.Fatalf("pixel %v", img.RGBAAt(3, 2))
	}
	// Slot 0 is held: the next frame goes to slot 1.
	blue := color.RGBA{0, 0, 255, 255}
	if err := fw.Write(solid(4, 3, blue), 2); err != nil {
		t.Fatal(err)
	}
	f2, img2 := h.last(t)
	if f2.Slot != 1 || img2.RGBAAt(0, 0) != blue {
		t.Fatalf("frame 2 %+v %v", f2, img2.RGBAAt(0, 0))
	}
	if _, img1 := h.frames[0], img; img1.RGBAAt(0, 0) != red {
		t.Fatal("slot 0 overwritten while held")
	}
	// Both held: Write waits for an ack.
	done := make(chan error, 1)
	go func() { done <- fw.Write(solid(4, 3, red), 2) }()
	select {
	case err := <-done:
		t.Fatalf("Write did not wait: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	fw.Ack(1, 0)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if f3, _ := h.last(t); f3.Slot != 0 {
		t.Fatalf("frame 3 slot %d", f3.Slot)
	}
	// A bigger frame gets a new segment without waiting for the held slots.
	green := color.RGBA{0, 255, 0, 255}
	big := solid(400, 300, green)
	if err := fw.Write(big, 1); err != nil {
		t.Fatal(err)
	}
	f4, img4 := h.last(t)
	if f4.Seg != 2 || f4.Slot != 0 || img4.RGBAAt(399, 299) != green {
		t.Fatalf("grown frame %+v", f4)
	}
	// The old segment is still readable by the host.
	if old, _ := FrameImage(h.segs[1], f2); old.RGBAAt(0, 0) != blue {
		t.Fatal("old segment changed")
	}
	// Acks for the old segment are ignored.
	fw.Ack(1, 1)
	// Sub-images with a larger stride are packed.
	sub := solid(10, 10, red).SubImage(image.Rect(2, 2, 6, 5)).(*image.RGBA)
	if err := fw.Write(sub, 1); err != nil {
		t.Fatal(err)
	}
	if f5, img5 := h.last(t); f5.W != 4 || f5.H != 3 || f5.Stride != 16 || img5.RGBAAt(3, 2) != red {
		t.Fatalf("sub frame %+v", f5)
	}
}

func TestFrameWriterTimeout(t *testing.T) {
	old := SlotTimeout
	SlotTimeout = 30 * time.Millisecond
	defer func() { SlotTimeout = old }()
	h := &fakeHost{segs: map[int]*Segment{}}
	fw := NewFrameWriter(1, "", h.send)
	defer fw.Close()
	img := solid(2, 2, color.RGBA{A: 255})
	fw.Write(img, 1)
	fw.Write(img, 1)
	if err := fw.Write(img, 1); err != ErrNoSlot {
		t.Fatalf("err = %v", err)
	}
}

func TestFrameImageBounds(t *testing.T) {
	s, err := CreateSegment("", 1024)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close(); s.Unlink() }()
	if _, err := FrameImage(s, FrameReady{Slot: 0, W: 16, H: 16, Stride: 64}); err == nil {
		t.Fatal("oversized frame accepted")
	}
	if _, err := FrameImage(s, FrameReady{Slot: 2, W: 1, H: 1, Stride: 4}); err == nil {
		t.Fatal("bad slot accepted")
	}
}

type formState struct {
	Name  string
	Count int
	Tab   string
}

func TestSnapshotRegistry(t *testing.T) {
	var r Registry
	a := formState{Name: "张三", Count: 3, Tab: "table"}
	b := formState{Name: "second"}
	n := 7
	if _, _, err := r.Add(a); err == nil {
		t.Fatal("non-pointer accepted")
	}
	ka, typ, _ := r.Add(&a)
	kb, _, _ := r.Add(&b)
	kn, _, _ := r.Add(&n)
	if typ != "devhost.formState" || ka != "devhost.formState#0" || kb != "devhost.formState#1" || kn != "int#0" {
		t.Fatalf("keys %q %q %q type %q", ka, kb, kn, typ)
	}
	entries, err := r.Encode()
	if err != nil || len(entries) != 3 {
		t.Fatalf("encode: %v %d", err, len(entries))
	}
	// Ship through the wire as the host does.
	c1, c2 := pipe(t)
	go c1.Send(SnapshotData{Req: 1, Entries: entries})
	m, err := c2.Recv()
	if err != nil {
		t.Fatal(err)
	}
	entries = m.(SnapshotData).Entries

	var a2, b2 formState
	var n2 int
	for _, x := range []struct {
		key string
		ptr any
	}{{ka, &a2}, {kb, &b2}, {kn, &n2}} {
		if found, err := Decode(entries, x.key, x.ptr); !found || err != nil {
			t.Fatalf("%s: %v %v", x.key, found, err)
		}
	}
	if a2 != a || b2 != b || n2 != 7 {
		t.Fatalf("decoded %+v %+v %d", a2, b2, n2)
	}
	// A compatible change (a field added) still decodes.
	type formV2 struct {
		Name  string
		Count int
		Extra bool
	}
	var v2 formV2
	if found, err := Decode(entries, ka, &v2); !found || err != nil || v2.Name != "张三" || v2.Count != 3 {
		t.Fatalf("v2: %v %v %+v", found, err, v2)
	}
	// An incompatible change fails and leaves the value alone.
	type formV3 struct{ Name []int }
	v3 := formV3{Name: []int{1}}
	if found, err := Decode(entries, ka, &v3); !found || err == nil || len(v3.Name) != 1 {
		t.Fatalf("v3: %v %v %+v", found, err, v3)
	}
	if found, _ := Decode(entries, "nope#0", &n2); found {
		t.Fatal("found a missing key")
	}
}

func TestClientRequests(t *testing.T) {
	a, b := pipe(t)
	got := make(chan any, 4)
	go func() {
		m, _ := b.Recv() // Hello
		if _, ok := m.(Hello); !ok {
			t.Errorf("first message %T", m)
		}
		b.Send(Welcome{Gen: 3})
		m, _ = b.Recv()
		req := m.(ReadClipboard)
		b.Send(Invalidate{Win: 1}) // unsolicited: goes to the handler
		b.Send(ClipboardText{Req: req.Req, Text: "clip"})
		b.Close()
	}()
	cl, err := NewClient(a, "", func(m any) { got <- m })
	if err != nil {
		t.Fatal(err)
	}
	if cl.Gen != 3 {
		t.Fatalf("gen %d", cl.Gen)
	}
	m, err := cl.Request(func(id int) any { return ReadClipboard{Req: id} }, time.Second)
	if err != nil || m.(ClipboardText).Text != "clip" {
		t.Fatalf("reply %v %v", m, err)
	}
	if m := <-got; m != (Invalidate{Win: 1}) {
		t.Fatalf("handler got %v", m)
	}
	<-cl.Done()
	if _, err := cl.Request(func(id int) any { return SnapshotGet{Req: id} }, time.Second); err == nil {
		t.Fatal("request after close succeeded")
	}
}
