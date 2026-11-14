package terminal

import (
	"bytes"
	"errors"
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

	mu          sync.Mutex
	resizes     [][4]int
	closed      int
	lateResizes int // Resize calls after Close
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
	if c.closed > 0 {
		c.lateResizes++
	}
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

// closeWithin calls term.Close and fails if it does not return within d.
func closeWithin(t *testing.T, term *Terminal, d time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		term.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatal("Close did not return")
	}
}

// TestTerminalCloseBlockedWriter: the program floods cursor position
// requests and never reads its input, so the writer blocks in Conn.Write
// and the reply queue fills, blocking the read loop. Close must still
// end both goroutines.
func TestTerminalCloseBlockedWriter(t *testing.T) {
	base := runtime.NumGoroutine()
	c := newPipeConn()
	exited := make(chan error, 1)
	term := New(Options{Conn: c, OnExit: func(err error) { exited <- err }})
	term.qlimit = 64
	term.Start(10, 3, func() {})

	flood := []byte(strings.Repeat("\x1b[6n", 64))
	go func() {
		for {
			if _, err := c.outW.Write(flood); err != nil {
				return
			}
		}
	}()
	// Wait until the reply queue is full: the read loop is then blocked
	// enqueueing and the writer is blocked in Write.
	waitFor(t, 5*time.Second, "reply queue to fill", func() bool {
		term.qmu.Lock()
		defer term.qmu.Unlock()
		return term.qbytes+len("\x1b[1;1R") > term.qlimit
	})
	closeWithin(t, term, 5*time.Second)
	if err := <-exited; err != nil {
		t.Errorf("OnExit err = %v, want nil", err)
	}
	c.outW.Close()
	c.inR.Close()
	waitGoroutines(t, base)
}

func TestTerminalSendRacingClose(t *testing.T) {
	base := runtime.NumGoroutine()
	for range 50 {
		c := newPipeConn()
		term := New(Options{Conn: c})
		term.Start(10, 3, func() {})
		var wg sync.WaitGroup
		for range 4 {
			wg.Add(1)
			go func() { // nobody reads: Send queues and returns
				defer wg.Done()
				term.Send([]byte("x"))
			}()
		}
		closeWithin(t, term, 5*time.Second)
		wg.Wait()
		term.Send([]byte("y")) // after Close: no-op, no block
		c.outW.Close()
		c.inR.Close()
	}
	waitGoroutines(t, base)
}

func TestTerminalResizeRacingClose(t *testing.T) {
	for range 50 {
		c := newPipeConn()
		term := New(Options{Conn: c})
		term.Start(10, 3, func() {})
		var wg sync.WaitGroup
		for i := range 4 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for j := range 20 {
					term.Resize(20+i, 5+j, 0, 0)
				}
			}()
		}
		closeWithin(t, term, 5*time.Second)
		wg.Wait()
		c.mu.Lock()
		late := c.lateResizes
		c.mu.Unlock()
		if late != 0 {
			t.Fatalf("%d Conn.Resize calls after Close", late)
		}
		v := term.Lock()
		cols, rows := v.Screen().Size()
		term.Unlock()
		term.Resize(70, 30, 0, 0) // after Close: no-op
		v = term.Lock()
		cols2, rows2 := v.Screen().Size()
		term.Unlock()
		if cols2 != cols || rows2 != rows {
			t.Fatalf("Resize after Close changed size %dx%d -> %dx%d", cols, rows, cols2, rows2)
		}
		c.outW.Close()
		c.inR.Close()
	}
}

func TestTerminalStartRacingClose(t *testing.T) {
	base := runtime.NumGoroutine()
	for range 100 {
		c := newPipeConn()
		var exits atomic.Int32
		term := New(Options{Conn: c, OnExit: func(error) { exits.Add(1) }})
		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); term.Start(10, 3, func() {}) }()
		go func() { defer wg.Done(); term.Close() }()
		wg.Wait()
		term.Close() // waits for any loop Start launched
		c.mu.Lock()
		late := c.lateResizes
		c.mu.Unlock()
		if late != 0 {
			t.Fatalf("%d Conn.Resize calls after Close", late)
		}
		if n := exits.Load(); n > 1 {
			t.Fatalf("OnExit called %d times", n)
		}
		c.outW.Close()
		c.inR.Close()
	}
	waitGoroutines(t, base)
}

func TestTerminalStartTwice(t *testing.T) {
	base := runtime.NumGoroutine()
	c := newPipeConn()
	term := New(Options{Conn: c})
	term.Start(10, 3, func() {})
	n := runtime.NumGoroutine()
	term.Start(40, 9, func() {})
	if got := runtime.NumGoroutine(); got > n {
		t.Errorf("second Start added goroutines: %d -> %d", n, got)
	}
	c.mu.Lock()
	got := len(c.resizes)
	c.mu.Unlock()
	if got != 1 {
		t.Errorf("Conn.Resize called %d times, want 1", got)
	}
	v := term.Lock()
	cols, rows := v.Screen().Size()
	term.Unlock()
	if cols != 10 || rows != 3 {
		t.Errorf("size = %dx%d, want 10x3", cols, rows)
	}
	closeWithin(t, term, 5*time.Second)
	c.outW.Close()
	c.inR.Close()
	waitGoroutines(t, base)
}

// errConn fails Read with err.
type errConn struct {
	*pipeConn
	err error
}

func (c errConn) Read([]byte) (int, error) { return 0, c.err }

func TestTerminalReadError(t *testing.T) {
	want := errors.New("boom")
	c := errConn{newPipeConn(), want}
	exited := make(chan error, 1)
	term := New(Options{Conn: c, OnExit: func(err error) { exited <- err }})
	term.Start(10, 3, func() {})
	select {
	case err := <-exited:
		if err != want {
			t.Errorf("OnExit err = %v, want %v", err, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("OnExit not called")
	}
	if ex, err := term.Exited(); !ex || err != want {
		t.Errorf("Exited = %v, %v, want true, %v", ex, err, want)
	}
	closeWithin(t, term, 5*time.Second)
	if ex, err := term.Exited(); !ex || err != want {
		t.Errorf("Exited after Close = %v, %v, want true, %v", ex, err, want)
	}
}

// gateConn is a pipeConn whose Write blocks until the gate opens (a
// program that is not reading its input).
type gateConn struct {
	*pipeConn
	gate   chan struct{}
	closed chan struct{}
	once   sync.Once
}

func newGateConn() *gateConn {
	return &gateConn{pipeConn: newPipeConn(), gate: make(chan struct{}), closed: make(chan struct{})}
}

func (c *gateConn) Write(b []byte) (int, error) {
	select {
	case <-c.gate:
	case <-c.closed:
		return 0, io.ErrClosedPipe
	}
	return c.pipeConn.Write(b)
}

func (c *gateConn) Close() error {
	c.once.Do(func() { close(c.closed) })
	return c.pipeConn.Close()
}

func queueLen(term *Terminal) int {
	term.qmu.Lock()
	defer term.qmu.Unlock()
	return len(term.queue)
}

// TestTerminalSendQueued: Send returns while the program is not reading,
// and replies and input reach it in the order they were made.
func TestTerminalSendQueued(t *testing.T) {
	c := newGateConn()
	term := New(Options{Conn: c})
	defer term.Close()
	term.Start(10, 3, func() {})

	term.Send([]byte("k1"))
	waitFor(t, 5*time.Second, "writer to take k1", func() bool { return queueLen(term) == 0 })
	if _, err := c.outW.Write([]byte("\x1b[6n")); err != nil {
		t.Fatal(err)
	}
	waitFor(t, 5*time.Second, "reply queued", func() bool { return queueLen(term) == 1 })
	paste := bytes.Repeat([]byte("p"), 64<<10)
	start := time.Now()
	term.Send([]byte("k2"))
	term.Send(paste)
	if d := time.Since(start); d > time.Second {
		t.Fatalf("Send blocked for %v", d)
	}
	want := "k1" + "\x1b[1;1R" + "k2" + string(paste)
	got := make(chan string, 1)
	go func() {
		buf := make([]byte, len(want))
		io.ReadFull(c.inR, buf)
		got <- string(buf)
	}()
	close(c.gate)
	select {
	case s := <-got:
		if s != want {
			t.Fatalf("program read %q..., want %q...", s[:min(len(s), 20)], want[:20])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("queued bytes not written")
	}
}

func TestTerminalSendOverLimitDropped(t *testing.T) {
	c := newGateConn()
	term := New(Options{Conn: c})
	term.qlimit = 8
	term.Send([]byte("12345"))
	term.Send([]byte("6789")) // 9 bytes > 8: dropped whole
	term.Send([]byte("678"))
	term.qmu.Lock()
	n, drops, q := term.qbytes, term.sendDrops, len(term.queue)
	term.qmu.Unlock()
	if n != 8 || q != 2 || drops != 1 {
		t.Fatalf("queued %d bytes in %d items, %d drops; want 8, 2, 1", n, q, drops)
	}
	term.Close()
}

func TestTerminalCloseFullQueue(t *testing.T) {
	base := runtime.NumGoroutine()
	c := newGateConn()
	term := New(Options{Conn: c})
	term.qlimit = 1 << 10
	term.Start(10, 3, func() {})
	for range 100 {
		term.Send(bytes.Repeat([]byte("x"), 100))
	}
	closeWithin(t, term, 5*time.Second)
	term.qmu.Lock()
	q := len(term.queue)
	term.qmu.Unlock()
	if q != 0 {
		t.Errorf("%d items still queued after Close", q)
	}
	term.Send([]byte("y")) // after Close: dropped
	c.outW.Close()
	c.inR.Close()
	waitGoroutines(t, base)
}
