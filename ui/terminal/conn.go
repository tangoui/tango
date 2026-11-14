package terminal

import (
	"io"
	"sync"

	"github.com/tangoui/tango/ui/terminal/vt"
)

// Conn is the other end of a terminal: the program's output and input
// streams.
type Conn interface {
	io.Reader // the program's output
	io.Writer // input to the program: keys, pastes and replies
	// Resize tells the program the terminal's new size in cells and
	// pixels.
	Resize(cols, rows, widthPx, heightPx int) error
	// Close closes the connection. It must unblock a pending Read.
	Close() error
}

// Options configures a [Terminal].
type Options struct {
	Conn Conn
	// Scrollback is the number of lines kept after they scroll off the
	// top. 0 means the default, 10 000; a negative value keeps none.
	Scrollback int
	// OnExit is called once, from the read goroutine, when the read loop
	// ends: err is nil at EOF or after Close, otherwise the read error.
	// It may call [Terminal.Close].
	OnExit func(err error)
}

const (
	// readBufSize is the size of the read loop's buffer.
	readBufSize = 32 << 10
	// maxLockBytes bounds how much output the read loop feeds the
	// emulator in one hold of the lock, so the UI never waits long.
	maxLockBytes = 256 << 10
	// replyQueue is the capacity of the reply channel.
	replyQueue = 64
)

// Size of the emulator before [Terminal.Start].
const defaultCols, defaultRows = 80, 24

// Terminal connects a [Conn] to a [vt.Terminal]. A read goroutine feeds
// the program's output to the emulator under a mutex; the view holds the
// same mutex, via [Terminal.Lock], while it reads the emulator.
//
// Replies the emulator generates (cursor position reports and the like)
// go to the program from a separate writer goroutine, so the read loop
// never writes to the Conn while holding the lock. [Terminal.Send] and
// the writer share a write mutex, so their bytes never interleave.
type Terminal struct {
	mu sync.Mutex // guards vt and pending
	vt *vt.Terminal
	// pending collects replies made during vt.Write; the read loop
	// queues them on replies once it has released mu.
	pending [][]byte

	conn    Conn
	onExit  func(error)
	replies chan []byte
	wmu     sync.Mutex // serializes writes to conn
	wg      sync.WaitGroup

	// smu guards the lifecycle state. It is separate from mu so that
	// Exited and Close work while the view holds the lock.
	smu     sync.Mutex
	started bool
	closing bool
	exited  bool
	exitErr error

	closeOnce sync.Once
	closeErr  error // set once by Close
}

// New returns a Terminal for o.Conn. It does not start reading; the
// view calls [Terminal.Start] once it knows its size.
func New(o Options) *Terminal {
	t := &Terminal{
		conn:    o.Conn,
		onExit:  o.OnExit,
		replies: make(chan []byte, replyQueue),
	}
	t.vt = vt.New(vt.Options{
		Cols:       defaultCols,
		Rows:       defaultRows,
		Scrollback: o.Scrollback,
		Reply:      t.reply,
	})
	return t
}

// reply is the emulator's Reply callback. It runs during vt.Write with
// mu held, so it only copies b; b is not valid after the call.
func (t *Terminal) reply(b []byte) {
	t.pending = append(t.pending, append([]byte(nil), b...))
}

// Start sizes the emulator and the program to cols×rows and starts the
// read loop. invalidate is called, from the read goroutine, after each
// batch of output; it must be safe to call from any goroutine. Start
// does nothing if the Terminal was already started or closed.
func (t *Terminal) Start(cols, rows int, invalidate func()) {
	t.smu.Lock()
	if t.started || t.closing {
		t.smu.Unlock()
		return
	}
	t.started = true
	t.wg.Add(2)
	t.smu.Unlock()

	if invalidate == nil {
		invalidate = func() {}
	}
	t.mu.Lock()
	t.vt.Resize(cols, rows)
	t.mu.Unlock()
	t.conn.Resize(cols, rows, 0, 0)

	go t.writeLoop()
	go t.readLoop(invalidate)
}

// readLoop feeds the program's output to the emulator until Read fails.
func (t *Terminal) readLoop(invalidate func()) {
	buf := make([]byte, readBufSize)
	var err error
	for {
		var n int
		n, err = t.conn.Read(buf)
		if n > 0 {
			t.feed(buf[:n])
			invalidate()
		}
		if err != nil {
			break
		}
	}
	// The read loop is the only sender on replies.
	close(t.replies)

	t.smu.Lock()
	if err == io.EOF || t.closing {
		err = nil
	}
	t.exited, t.exitErr = true, err
	t.smu.Unlock()
	invalidate()
	// Done before OnExit, so OnExit may call Close without deadlock.
	t.wg.Done()
	if t.onExit != nil {
		t.onExit(err)
	}
}

// feed writes b to the emulator, holding mu for at most maxLockBytes at
// a time, then queues the replies it produced for the writer. Queuing
// may block the read loop when the queue is full, never with mu held;
// replies are small and the writer drains continuously.
func (t *Terminal) feed(b []byte) {
	for len(b) > 0 {
		n := min(len(b), maxLockBytes)
		t.mu.Lock()
		t.vt.Write(b[:n])
		pending := t.pending
		t.pending = nil
		t.mu.Unlock()
		b = b[n:]
		for _, r := range pending {
			t.replies <- r
		}
	}
}

// writeLoop writes queued replies to the program until the read loop
// closes the queue. Write errors are ignored: a broken Conn also ends
// the read loop.
func (t *Terminal) writeLoop() {
	defer t.wg.Done()
	for r := range t.replies {
		t.wmu.Lock()
		t.conn.Write(r)
		t.wmu.Unlock()
	}
}

// Lock locks the emulator and returns it. The caller must call
// [Terminal.Unlock] when done and must not keep the pointer past it.
func (t *Terminal) Lock() *vt.Terminal {
	t.mu.Lock()
	return t.vt
}

// Unlock unlocks the emulator locked by [Terminal.Lock].
func (t *Terminal) Unlock() { t.mu.Unlock() }

// Send writes b, encoded input such as keys or a paste, to the program.
// It writes on the caller's goroutine and does not take the emulator
// lock. Send does nothing after Close.
func (t *Terminal) Send(b []byte) {
	t.smu.Lock()
	closing := t.closing
	t.smu.Unlock()
	if closing || len(b) == 0 {
		return
	}
	t.wmu.Lock()
	t.conn.Write(b)
	t.wmu.Unlock()
}

// Resize resizes the emulator to cols×rows cells and tells the program
// the new size, in cells and pixels. A Conn error is ignored.
func (t *Terminal) Resize(cols, rows, wpx, hpx int) {
	t.mu.Lock()
	t.vt.Resize(cols, rows)
	t.mu.Unlock()
	t.smu.Lock()
	closing := t.closing
	t.smu.Unlock()
	if !closing {
		t.conn.Resize(cols, rows, wpx, hpx)
	}
}

// Exited reports whether the read loop has ended and, if it ended with
// an error other than EOF, the error. It does not take the emulator lock.
func (t *Terminal) Exited() (bool, error) {
	t.smu.Lock()
	defer t.smu.Unlock()
	return t.exited, t.exitErr
}

// Close closes the Conn and waits for the read loop and the writer to
// end. It is safe to call more than once, concurrently, before Start
// and from OnExit; every call returns the first call's result.
func (t *Terminal) Close() error {
	t.closeOnce.Do(func() {
		t.smu.Lock()
		t.closing = true
		t.smu.Unlock()
		t.closeErr = t.conn.Close()
		t.wg.Wait()
	})
	return t.closeErr
}
