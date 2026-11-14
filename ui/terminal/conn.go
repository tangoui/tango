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
	// Close closes the connection. It must unblock a pending Read and a
	// pending Write, as closing an os.File, a pipe or a net.Conn does;
	// [Terminal.Close] waits for both to return.
	Close() error
}

// Options configures a [Terminal].
type Options struct {
	Conn Conn
	// Scrollback is the number of lines kept after they scroll off the
	// top. 0 means the default, 10 000; a negative value keeps none.
	Scrollback int
	// OnExit is called once, from the read goroutine, when the read loop
	// ends: err is nil at EOF or when [Terminal.Close] ended it,
	// otherwise the read error. It may call [Terminal.Close].
	OnExit func(err error)
}

const (
	// readBufSize is the size of the read loop's buffer.
	readBufSize = 32 << 10
	// maxLockBytes bounds how much output the read loop feeds the
	// emulator in one hold of the lock, so the UI never waits long.
	maxLockBytes = 256 << 10
	// sendQueueBytes bounds the bytes queued for the program (replies
	// and Send) while it is not reading its input.
	sendQueueBytes = 8 << 20
)

// Size of the emulator before [Terminal.Start].
const defaultCols, defaultRows = 80, 24

// Terminal connects a [Conn] to a [vt.Terminal]. A read goroutine feeds
// the program's output to the emulator under a mutex; the view holds the
// same mutex, via [Terminal.Lock], while it reads the emulator.
//
// Replies the emulator generates (cursor position reports and the like)
// and the input [Terminal.Send] is given go to the program through one
// FIFO queue drained by a writer goroutine, so neither the read loop nor
// the UI thread ever waits for the program to read its input, and replies
// and input reach the program in the order they were made.
type Terminal struct {
	mu sync.Mutex // guards vt and pending
	vt *vt.Terminal
	// pending collects replies made during vt.Write; the read loop
	// queues them once it has released mu.
	pending [][]byte

	conn   Conn
	onExit func(error)
	done   chan struct{} // closed by Close before it closes conn
	wg     sync.WaitGroup

	// The write queue (see Send). qmu guards it; wake tells the writer
	// that the queue is not empty, space tells a waiting reply that the
	// writer took something. qclosed: the writer is gone (the read loop
	// ended or Close), queued and new bytes are dropped.
	qmu       sync.Mutex
	queue     [][]byte
	qbytes    int
	qlimit    int // sendQueueBytes; tests lower it
	qclosed   bool
	readEnded bool
	sendDrops int // Sends dropped because the queue was full
	wake      chan struct{}
	space     chan struct{}

	// smu guards the lifecycle state. It is separate from mu so that
	// Exited and Close work while the view holds the lock. Lock order:
	// smu is never held while taking mu (the view may call Exited with
	// mu held).
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
		conn:   o.Conn,
		onExit: o.OnExit,
		done:   make(chan struct{}),
		qlimit: sendQueueBytes,
		wake:   make(chan struct{}, 1),
		space:  make(chan struct{}, 1),
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
//
// The initial Conn.Resize error is ignored: the program then simply runs
// at the Conn's previous size, and a failing Conn also fails Read, which
// ends the loop and reports the error through OnExit.
func (t *Terminal) Start(cols, rows int, invalidate func()) {
	t.smu.Lock()
	if t.started || t.closing {
		t.smu.Unlock()
		return
	}
	t.started = true
	t.smu.Unlock()

	if invalidate == nil {
		invalidate = func() {}
	}
	t.mu.Lock()
	t.vt.Resize(cols, rows)
	t.mu.Unlock()

	// Re-check under smu: Close sets closing under smu before it closes
	// the Conn, so neither Resize nor the goroutines run after it, and
	// wg.Add happens before Close's wg.Wait or not at all.
	t.smu.Lock()
	defer t.smu.Unlock()
	if t.closing {
		return
	}
	t.conn.Resize(cols, rows, 0, 0)
	t.wg.Add(2)
	go t.writeLoop()
	go t.readLoop(invalidate)
}

// readLoop feeds the program's output to the emulator until Read fails.
//
// A Read error counts as caused by Close, and is reported as nil, when
// done is already closed when the loop sees the error; Close closes
// done before it closes the Conn. An error that arrives before Close is
// reported as is, except in the narrow window where Close begins between
// Read returning it and the check, where it is reported as nil.
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
	if err == io.EOF || t.isClosing() {
		err = nil
	}
	// The writer drains what is queued, then stops.
	t.qmu.Lock()
	t.readEnded = true
	t.qmu.Unlock()
	signal(t.wake)

	t.smu.Lock()
	t.exited, t.exitErr = true, err
	t.smu.Unlock()
	invalidate()
	// Done before OnExit, so OnExit may call Close without deadlock.
	t.wg.Done()
	if t.onExit != nil {
		t.onExit(err)
	}
}

// isClosing reports whether Close has begun.
func (t *Terminal) isClosing() bool {
	select {
	case <-t.done:
		return true
	default:
		return false
	}
}

// feed writes b to the emulator, holding mu for at most maxLockBytes at
// a time, then queues the replies it produced for the writer. A reply
// that does not fit in the queue makes the read loop wait for space,
// never with mu held, so a program that floods queries without reading
// its input is slowed down rather than growing the queue. After Close,
// replies are dropped instead.
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
			if !t.queueReply(r) {
				return
			}
		}
	}
}

// signal wakes the goroutine waiting on ch, if any.
func signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// queueReply queues reply r, waiting while the queue is full. It reports
// false once the writer is gone.
func (t *Terminal) queueReply(r []byte) bool {
	for {
		t.qmu.Lock()
		if t.qclosed {
			t.qmu.Unlock()
			return false
		}
		if t.qbytes == 0 || t.qbytes+len(r) <= t.qlimit {
			t.queue = append(t.queue, r)
			t.qbytes += len(r)
			t.qmu.Unlock()
			signal(t.wake)
			return true
		}
		t.qmu.Unlock()
		select {
		case <-t.space:
		case <-t.done:
			return false
		}
	}
}

// writeLoop writes the queue to the program, in order, until Close, or
// until the read loop has ended and the queue is empty. A Write in
// progress at Close returns because Close closes the Conn. Write errors
// are ignored: a broken Conn also ends the read loop.
func (t *Terminal) writeLoop() {
	defer t.wg.Done()
	for {
		t.qmu.Lock()
		if len(t.queue) == 0 {
			if t.readEnded {
				t.qclosed = true
				t.qmu.Unlock()
				return
			}
			t.qmu.Unlock()
			select {
			case <-t.wake:
				continue
			case <-t.done:
				return
			}
		}
		b := t.queue[0]
		t.queue[0] = nil
		t.queue = t.queue[1:]
		t.qbytes -= len(b)
		t.qmu.Unlock()
		signal(t.space)
		select {
		case <-t.done:
			return
		default:
		}
		t.conn.Write(b)
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

// Send queues b, encoded input such as keys or a paste, for the
// program and returns at once; the writer goroutine writes it after the
// replies and input queued before it. b is copied. The queue holds at
// most 8 MB: while the program is not reading its input, a Send that
// does not fit is dropped whole (never split, so no escape sequence is
// cut). Input sent before Start waits for it; input sent after Close, or
// after the program's output has ended, is dropped.
func (t *Terminal) Send(b []byte) {
	if len(b) == 0 || t.isClosing() {
		return
	}
	t.qmu.Lock()
	if t.qclosed || t.qbytes+len(b) > t.qlimit {
		if !t.qclosed {
			t.sendDrops++
		}
		t.qmu.Unlock()
		return
	}
	t.queue = append(t.queue, append([]byte(nil), b...))
	t.qbytes += len(b)
	t.qmu.Unlock()
	signal(t.wake)
}

// Resize resizes the emulator to cols×rows cells and tells the program
// the new size, in cells and pixels. A Conn error is ignored. Resize
// does nothing after Close, and never calls Conn.Resize once Close has
// begun: the Conn call is made under smu, which Close takes first.
func (t *Terminal) Resize(cols, rows, wpx, hpx int) {
	if t.isClosing() {
		return
	}
	t.mu.Lock()
	t.vt.Resize(cols, rows)
	t.mu.Unlock()
	t.smu.Lock()
	defer t.smu.Unlock()
	if !t.closing {
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

// Close stops the writer, closes the Conn and waits for the read loop
// and the writer to end; it relies on Conn.Close unblocking a pending
// Read and Write. It is safe to call more than once, concurrently,
// before Start and from OnExit; every call returns the first call's
// result.
func (t *Terminal) Close() error {
	t.closeOnce.Do(func() {
		t.smu.Lock()
		t.closing = true
		close(t.done) // before conn.Close: see readLoop
		t.smu.Unlock()
		t.closeErr = t.conn.Close()
		t.wg.Wait()
		t.qmu.Lock()
		t.qclosed, t.queue, t.qbytes = true, nil, 0
		t.qmu.Unlock()
	})
	return t.closeErr
}
