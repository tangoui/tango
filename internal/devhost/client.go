package devhost

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// Client is a worker's connection to the host.
type Client struct {
	conn *Conn
	Gen  int
	Dir  string // where frame segments are created

	mu      sync.Mutex
	nextReq int
	waiting map[int]chan any
	done    chan struct{}
	err     error
}

// ErrClosed is returned by requests after the connection ended.
var ErrClosed = errors.New("devhost: connection to the host closed")

// Connect dials the host at addr and starts reading. handler receives every
// message that is not an answer to a request, on the reading goroutine;
// it must not block for long.
func Connect(addr, dir string, handler func(any)) (*Client, error) {
	conn, err := Dial(addr, 10*time.Second)
	if err != nil {
		return nil, err
	}
	return NewClient(conn, dir, handler)
}

// NewClient performs the worker handshake on conn and starts reading.
func NewClient(conn *Conn, dir string, handler func(any)) (*Client, error) {
	w, err := Handshake(conn, RoleWorker)
	if err != nil {
		conn.Close()
		return nil, err
	}
	c := &Client{conn: conn, Gen: w.Gen, Dir: dir, waiting: map[int]chan any{}, done: make(chan struct{})}
	go c.read(handler)
	return c, nil
}

func (c *Client) read(handler func(any)) {
	for {
		m, err := c.conn.Recv()
		if err != nil {
			c.mu.Lock()
			c.err = err
			for _, ch := range c.waiting {
				close(ch)
			}
			c.waiting = nil
			c.mu.Unlock()
			close(c.done)
			return
		}
		if req, ok := replyID(m); ok {
			c.mu.Lock()
			ch := c.waiting[req]
			delete(c.waiting, req)
			c.mu.Unlock()
			if ch != nil {
				ch <- m
				continue
			}
		}
		if handler != nil {
			handler(m)
		}
	}
}

func replyID(m any) (int, bool) {
	switch m := m.(type) {
	case Opened:
		return m.Req, true
	case ClipboardText:
		return m.Req, true
	case SnapshotData:
		return m.Req, true
	case SurfaceInfo:
		return m.Req, true
	}
	return 0, false
}

// Send sends a message.
func (c *Client) Send(m any) error { return c.conn.Send(m) }

// Request sends the message build(id) and waits for the reply with the
// same id.
func (c *Client) Request(build func(id int) any, timeout time.Duration) (any, error) {
	ch := make(chan any, 1)
	c.mu.Lock()
	if c.waiting == nil {
		c.mu.Unlock()
		return nil, ErrClosed
	}
	c.nextReq++
	id := c.nextReq
	c.waiting[id] = ch
	c.mu.Unlock()
	if err := c.conn.Send(build(id)); err != nil {
		c.forget(id)
		return nil, err
	}
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case m, ok := <-ch:
		if !ok {
			return nil, ErrClosed
		}
		return m, nil
	case <-t.C:
		c.forget(id)
		return nil, fmt.Errorf("devhost: no answer from the host in %v", timeout)
	}
}

func (c *Client) forget(id int) {
	c.mu.Lock()
	if c.waiting != nil {
		delete(c.waiting, id)
	}
	c.mu.Unlock()
}

// Done is closed when the connection ends.
func (c *Client) Done() <-chan struct{} { return c.done }

// Close closes the connection.
func (c *Client) Close() error { return c.conn.Close() }
