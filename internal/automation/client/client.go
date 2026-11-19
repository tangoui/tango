// Package client is the automation channel's client without a test
// harness: it connects to a Tango development build, authenticates and
// exchanges requests (Conn), builds and starts such a program (Build,
// Launch) and writes the page-side JavaScript of WebView selector
// operations. Package automation wraps it for Go tests; tan mcp uses it
// directly.
package client

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	wire "github.com/tangoui/tango/internal/automation"
)

// Conn is an authenticated connection to an app's control channel. It is
// safe for concurrent use; requests are sent one at a time.
type Conn struct {
	mu     sync.Mutex
	conn   net.Conn
	nextID uint64
	broken error
}

// Dial connects to addr (5s), authenticates with token (10s) and checks
// the app speaks this protocol version.
func Dial(addr, token string) (*Conn, error) {
	conn, err := wire.Dial(addr, 5*time.Second)
	if err != nil {
		return nil, err
	}
	c := &Conn{conn: conn}
	r, err := c.Do(&wire.Request{Op: wire.OpHello, Token: token}, 10*time.Second)
	if err == nil {
		err = r.Err()
	}
	if err != nil {
		conn.Close()
		return nil, err
	}
	if r.Version != wire.Version {
		conn.Close()
		return nil, fmt.Errorf("protocol version %d, want %d (rebuild the app with this Tango)", r.Version, wire.Version)
	}
	return c, nil
}

// Do sends one request and reads its response within timeout. A
// transport error breaks the connection for good; the app's own error is
// in the response (resp.Err).
func (c *Conn) Do(req *wire.Request, timeout time.Duration) (*wire.Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.broken != nil {
		return nil, c.broken
	}
	c.nextID++
	req.ID = c.nextID
	c.conn.SetDeadline(time.Now().Add(timeout))
	defer c.conn.SetDeadline(time.Time{})
	if err := wire.WriteFrame(c.conn, req); err != nil {
		c.broken = fmt.Errorf("connection lost: %v", err)
		return nil, c.broken
	}
	var r wire.Response
	if err := wire.ReadFrame(c.conn, &r); err != nil {
		c.broken = fmt.Errorf("connection lost: %v", err)
		return nil, c.broken
	}
	if r.ID != req.ID {
		c.broken = fmt.Errorf("protocol error: response %d for request %d", r.ID, req.ID)
		return nil, c.broken
	}
	return &r, nil
}

// Close closes the connection; later requests fail, and so does a request
// in flight (the socket is closed before Close waits for it).
func (c *Conn) Close() {
	c.conn.Close()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.broken == nil {
		c.broken = errors.New("connection closed")
	}
}
