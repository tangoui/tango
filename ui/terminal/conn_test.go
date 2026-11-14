package terminal

import (
	"bytes"
	"io"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/terminal/vt"
)

// pipeConn is a Conn made of two io.Pipes. The test plays the program:
// it writes output to progOut and reads the terminal's input from progIn.
type pipeConn struct {
	outR *io.PipeReader // terminal reads program output
	outW *io.PipeWriter // test writes program output
	inR  *io.PipeReader // test reads terminal input
	inW  *io.PipeWriter // terminal writes input

	mu      sync.Mutex
	resizes [][4]int
	closed  int
}

func newPipeConn() *pipeConn {
	c := &pipeConn{}
	c.outR, c.outW = io.Pipe()
	c.inR, c.inW = io.Pipe()
	return c
}

func (c *pipeConn) Read(b []byte) (int, error)  { return c.outR.Read(b) }
func (c *pipeConn) Write(b []byte) (int, error) { return c.inW.Write(b) }

func (c *pipeConn) Resize(cols, rows, w, h int) error {
	c.mu.Lock()
	c.resizes = append(c.resizes, [4]int{cols, rows, w, h})
	c.mu.Unlock()
	return nil
}

func (c *pipeConn) Close() error {
	c.mu.Lock()
	c.closed++
	c.mu.Unlock()
	c.outR.Close()
	c.inW.Close()
	return nil
}

// rowText returns the text of active row y, trailing blanks trimmed.
func rowText(v *vt.Terminal, y int) string {
	var sb strings.Builder
	for _, c := range v.Screen().Line(y).Cells {
		if c.Width == 0 {
			continue
		}
		if c.Rune == 0 {
			sb.WriteByte(' ')
		} else {
			sb.WriteRune(c.Rune)
		}
	}
	return strings.TrimRight(sb.String(), " ")
}

// waitFor polls cond until it holds or d passes.
func waitFor(t *testing.T, d time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// waitGoroutines waits for the goroutine count to fall to at most n.
func waitGoroutines(t *testing.T, n int) {
	t.Helper()
	waitFor(t, 5*time.Second, "goroutines to exit", func() bool {
		return runtime.NumGoroutine() <= n
	})
}

func TestTerminalReadLoop(t *testing.T) {
	c := newPipeConn()
	term := New(Options{Conn: c})
	defer term.Close()
	var inv atomic.Int32
	term.Start(20, 5, func() { inv.Add(1) })

	c.mu.Lock()
	got := c.resizes
	c.mu.Unlock()
	if len(got) != 1 || got[0] != [4]int{20, 5, 0, 0} {
		t.Errorf("Start resizes = %v, want [[20 5 0 0]]", got)
	}

	if _, err := c.outW.Write([]byte("hi")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, "invalidate", func() bool { return inv.Load() > 0 })
	v := term.Lock()
	cols, rows := v.Screen().Size()
	line := rowText(v, 0)
	term.Unlock()
	if cols != 20 || rows != 5 {
		t.Errorf("size = %dx%d, want 20x5", cols, rows)
	}
	if line != "hi" {
		t.Errorf("row 0 = %q, want %q", line, "hi")
	}
}

func TestTerminalReply(t *testing.T) {
	c := newPipeConn()
	term := New(Options{Conn: c})
	defer term.Close()
	term.Start(10, 3, func() {})

	go c.outW.Write([]byte("\x1b[6n"))
	want := "\x1b[1;1R"
	buf := make([]byte, len(want))
	if _, err := io.ReadFull(c.inR, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != want {
		t.Errorf("reply = %q, want %q", buf, want)
	}
}

func TestTerminalSend(t *testing.T) {
	c := newPipeConn()
	term := New(Options{Conn: c})
	defer term.Close()
	term.Start(10, 3, func() {})

	go term.Send([]byte("ls\r"))
	buf := make([]byte, 3)
	if _, err := io.ReadFull(c.inR, buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "ls\r" {
		t.Errorf("sent = %q, want %q", buf, "ls\r")
	}
}

func TestTerminalResize(t *testing.T) {
	c := newPipeConn()
	term := New(Options{Conn: c})
	defer term.Close()
	term.Start(10, 3, func() {})
	term.Resize(30, 7, 300, 140)

	v := term.Lock()
	cols, rows := v.Screen().Size()
	term.Unlock()
	if cols != 30 || rows != 7 {
		t.Errorf("size = %dx%d, want 30x7", cols, rows)
	}
	c.mu.Lock()
	got := c.resizes
	c.mu.Unlock()
	if len(got) != 2 || got[1] != [4]int{30, 7, 300, 140} {
		t.Errorf("resizes = %v, want last [30 7 300 140]", got)
	}
}

func TestTerminalConnEOF(t *testing.T) {
	base := runtime.NumGoroutine()
	c := newPipeConn()
	var calls atomic.Int32
	var gotErr atomic.Value
	done := make(chan struct{})
	term := New(Options{Conn: c, OnExit: func(err error) {
		gotErr.Store(errBox{err})
		if calls.Add(1) == 1 {
			close(done)
		}
	}})
	if ex, err := term.Exited(); ex || err != nil {
		t.Fatalf("Exited before start = %v, %v", ex, err)
	}
	term.Start(10, 3, func() {})
	c.outW.Close() // program exits: EOF

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("OnExit not called")
	}
	if e := gotErr.Load().(errBox).err; e != nil {
		t.Errorf("OnExit err = %v, want nil", e)
	}
	if ex, err := term.Exited(); !ex || err != nil {
		t.Errorf("Exited = %v, %v, want true, nil", ex, err)
	}
	if err := term.Close(); err != nil {
		t.Errorf("Close = %v", err)
	}
	if err := term.Close(); err != nil {
		t.Errorf("second Close = %v", err)
	}
	c.inR.Close()
	waitGoroutines(t, base)
	if n := calls.Load(); n != 1 {
		t.Errorf("OnExit called %d times, want 1", n)
	}
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed != 1 {
		t.Errorf("Conn closed %d times, want 1", closed)
	}
}

type errBox struct{ err error }

func TestTerminalCloseStopsLoop(t *testing.T) {
	base := runtime.NumGoroutine()
	c := newPipeConn()
	var calls atomic.Int32
	var gotErr atomic.Value
	term := New(Options{Conn: c, OnExit: func(err error) {
		gotErr.Store(errBox{err})
		calls.Add(1)
	}})
	term.Start(10, 3, func() {})
	if err := term.Close(); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, "OnExit", func() bool { return calls.Load() == 1 })
	if e := gotErr.Load().(errBox).err; e != nil {
		t.Errorf("OnExit err after Close = %v, want nil", e)
	}
	if ex, err := term.Exited(); !ex || err != nil {
		t.Errorf("Exited = %v, %v, want true, nil", ex, err)
	}
	c.outW.Close()
	c.inR.Close()
	waitGoroutines(t, base)
}

func TestTerminalCloseBeforeStart(t *testing.T) {
	c := newPipeConn()
	called := false
	term := New(Options{Conn: c, OnExit: func(error) { called = true }})
	if err := term.Close(); err != nil {
		t.Fatal(err)
	}
	term.Start(10, 3, func() {}) // no-op after Close
	if called {
		t.Error("OnExit called without a read loop")
	}
	term.Send([]byte("x")) // must not block or panic
}

func TestTerminalLargeOutputYieldsLock(t *testing.T) {
	c := newPipeConn()
	term := New(Options{Conn: c})
	defer term.Close()
	term.Start(80, 24, func() {})

	const total = 2 << 20
	line := []byte(strings.Repeat("x", 79) + "\r\n")
	data := bytes.Repeat(line, total/len(line)+1)[:total]
	writeDone := make(chan struct{})
	go func() {
		c.outW.Write(data)
		close(writeDone)
	}()

	var worst time.Duration
	n := 0
loop:
	for {
		select {
		case <-writeDone:
			break loop
		default:
		}
		start := time.Now()
		term.Lock()
		d := time.Since(start)
		term.Unlock()
		if d > worst {
			worst = d
		}
		n++
		time.Sleep(100 * time.Microsecond)
	}
	if worst > 200*time.Millisecond {
		t.Errorf("worst Lock wait %v over %d attempts, want ≤ 200ms", worst, n)
	}
	t.Logf("worst Lock wait %v over %d attempts", worst, n)
}

func TestTerminalCloseFromOnExit(t *testing.T) {
	c := newPipeConn()
	done := make(chan error, 1)
	var term *Terminal
	term = New(Options{Conn: c, OnExit: func(error) { done <- term.Close() }})
	term.Start(10, 3, func() {})
	c.outW.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Close from OnExit = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Close from OnExit deadlocked")
	}
}
