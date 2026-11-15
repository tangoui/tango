package main

import (
	"io"
	"os"
	"sync"
)

// echoConn is a terminal.Conn without a program: what is typed comes back
// as output, Enter as a new line and Backspace as erasing the previous
// cell, roughly what a terminal in cooked mode with echo shows.
type echoConn struct {
	mu     sync.Mutex
	buf    []byte        // output not read yet
	ready  chan struct{} // signalled when buf becomes non-empty
	done   chan struct{}
	closer sync.Once
}

const echoBanner = "Tango Terminal: echo connection (no shell). Typed text comes back.\r\n$ "

func newEchoConn() *echoConn {
	return &echoConn{buf: []byte(echoBanner), ready: make(chan struct{}, 1), done: make(chan struct{})}
}

func (c *echoConn) Read(b []byte) (int, error) {
	for {
		c.mu.Lock()
		if len(c.buf) > 0 {
			n := copy(b, c.buf)
			c.buf = c.buf[n:]
			c.mu.Unlock()
			return n, nil
		}
		c.mu.Unlock()
		select {
		case <-c.ready:
		case <-c.done:
			return 0, os.ErrClosed
		}
	}
}

func (c *echoConn) Write(b []byte) (int, error) {
	select {
	case <-c.done:
		return 0, os.ErrClosed
	default:
	}
	c.mu.Lock()
	for _, ch := range b {
		switch ch {
		case '\r':
			c.buf = append(c.buf, "\r\n$ "...)
		case 0x7f, '\b':
			c.buf = append(c.buf, "\b \b"...)
		default:
			c.buf = append(c.buf, ch)
		}
	}
	c.mu.Unlock()
	select {
	case c.ready <- struct{}{}:
	default:
	}
	return len(b), nil
}

func (c *echoConn) Resize(cols, rows, widthPx, heightPx int) error { return nil }

func (c *echoConn) Close() error {
	c.closer.Do(func() { close(c.done) })
	return nil
}

// replayConn plays recorded program output, in 4 KB reads, then idles
// until it is closed. Input is discarded.
type replayConn struct {
	data   []byte
	done   chan struct{}
	closer sync.Once
}

func newReplayConn(data []byte) *replayConn {
	return &replayConn{data: data, done: make(chan struct{})}
}

func (c *replayConn) Read(b []byte) (int, error) {
	select {
	case <-c.done:
		return 0, os.ErrClosed
	default:
	}
	if len(c.data) == 0 {
		<-c.done
		return 0, os.ErrClosed
	}
	n := copy(b[:min(len(b), 4096)], c.data)
	c.data = c.data[n:]
	return n, nil
}

func (c *replayConn) Write(b []byte) (int, error) {
	select {
	case <-c.done:
		return 0, os.ErrClosed
	default:
		return len(b), nil
	}
}

func (c *replayConn) Resize(cols, rows, widthPx, heightPx int) error { return nil }

func (c *replayConn) Close() error {
	c.closer.Do(func() { close(c.done) })
	return nil
}

var _ io.ReadWriteCloser = (*echoConn)(nil)
