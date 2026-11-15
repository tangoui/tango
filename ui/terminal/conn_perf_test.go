package terminal

import (
	"io"
	"sync"
	"testing"
	"time"
)

type blockingResizeConn struct {
	closeOnce sync.Once
	closed    chan struct{}
}

func newBlockingResizeConn() *blockingResizeConn {
	return &blockingResizeConn{closed: make(chan struct{})}
}

func (c *blockingResizeConn) Read([]byte) (int, error) {
	<-c.closed
	return 0, io.ErrClosedPipe
}
func (c *blockingResizeConn) Write([]byte) (int, error) {
	<-c.closed
	return 0, io.ErrClosedPipe
}
func (c *blockingResizeConn) Resize(int, int, int, int) error {
	<-c.closed
	return io.ErrClosedPipe
}
func (c *blockingResizeConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func TestResizeDoesNotBlockClose(t *testing.T) {
	conn := newBlockingResizeConn()
	term := New(Options{Conn: conn})
	resizeDone := make(chan struct{})
	go func() {
		term.Resize(120, 40, 900, 600)
		close(resizeDone)
	}()
	time.Sleep(10 * time.Millisecond)
	closeDone := make(chan struct{})
	go func() {
		_ = term.Close()
		close(closeDone)
	}()
	select {
	case <-closeDone:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Close blocked behind Conn.Resize")
	}
	select {
	case <-resizeDone:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Resize did not finish after Close released the Conn")
	}
}
