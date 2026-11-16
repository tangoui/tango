package ipc

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// forever registers a stream handler that writes `"x"` (charged 67) until
// a write fails.
func forever(r *Router, name string) {
	r.HandleStream(name, func(ctx context.Context, _ json.RawMessage, w StreamWriter) error {
		for {
			if err := w.Write("x"); err != nil {
				return err
			}
		}
	})
}

// helloWith sends a hello carrying extra JSON fields and consumes the reply.
func helloWith(t *testing.T, r *Router, c *fakeConn, extra string) {
	t.Helper()
	send(t, r, c, `{"tango":"hello","proto":2%s}`, extra)
	if o := c.next(t); o.Tango != "hello" || o.Proto != ProtoVersion || o.Error != nil {
		t.Fatalf("hello reply %+v", o)
	}
}

// The page's "win" in its hello is Go's send window toward it.
func TestPageWindowFromHello(t *testing.T) {
	r := NewRouter()
	c := newFake()
	forever(r, "Forever")
	helloWith(t, r, c, `,"win":8192`)
	openJS(t, r, c, 1, "Forever")
	drainData(t, c, 1, 8192/67)
	c.none(t, 50*time.Millisecond) // blocked on the page's 8 KiB window
	r.Drop(c)
}

// A hello without "win" (a runtime-js older than this field), or with an
// unusable one, leaves Go at DefaultWindowBytes; the hello is still
// answered. A window above MaxPeerWindowBytes is clamped.
func TestPageWindowAbsentInvalidOrHuge(t *testing.T) {
	for _, tc := range []struct {
		extra string
		want  int
	}{
		{``, DefaultWindowBytes},
		{`,"win":null`, DefaultWindowBytes},
		{`,"win":"4096"`, DefaultWindowBytes},
		{`,"win":4095`, DefaultWindowBytes},
		{`,"win":0`, DefaultWindowBytes},
		{`,"win":-1`, DefaultWindowBytes},
		{`,"win":8192.5`, DefaultWindowBytes},
		{`,"win":{}`, DefaultWindowBytes},
		{`,"win":4096`, 4096},
		{`,"win":4194304`, 4 << 20},
		{`,"win":1e12`, MaxPeerWindowBytes},
	} {
		r := NewRouter()
		c := newFake()
		helloWith(t, r, c, tc.extra)
		if got := pageWindow(r, c); got != tc.want {
			t.Errorf("hello%s: page window %d, want %d", tc.extra, got, tc.want)
		}
	}
}

// A later {"tango":"win"} replaces the window for new streams and raises
// (never lowers) that of open ones: a writer blocked on the old window
// resumes. Invalid ones are ignored.
func TestPageWindowUpdate(t *testing.T) {
	r := NewRouter()
	c := newFake()
	forever(r, "Forever")
	helloWith(t, r, c, `,"win":4096`)
	openJS(t, r, c, 1, "Forever")
	drainData(t, c, 1, 4096/67)
	c.none(t, 30*time.Millisecond)

	send(t, r, c, `{"tango":"win","win":"big"}`)
	send(t, r, c, `{"tango":"win","win":100}`)
	c.none(t, 30*time.Millisecond)
	if got := pageWindow(r, c); got != 4096 {
		t.Fatalf("page window %d after invalid updates", got)
	}

	send(t, r, c, `{"tango":"win","win":8192}`)
	for i := 4096 / 67; i < 8192/67; i++ {
		if o := c.next(t); o.Op != "data" || o.ID != 1 || o.Seq != uint64(i) {
			t.Fatalf("frame %d: got %+v", i, o)
		}
	}
	c.none(t, 30*time.Millisecond)

	// Lower: new streams get it, the open one keeps 8 KiB.
	send(t, r, c, `{"tango":"win","win":4096}`)
	openJS(t, r, c, 3, "Forever")
	drainData(t, c, 3, 4096/67)
	c.none(t, 30*time.Millisecond)
	r.Drop(c)
}

// A win before any hello creates no connection state and is ignored.
func TestPageWindowUpdateBeforeHello(t *testing.T) {
	r := NewRouter()
	c := newFake()
	send(t, r, c, `{"tango":"win","win":8192}`)
	c.none(t, 20*time.Millisecond)
	if got := pageWindow(r, c); got != DefaultWindowBytes {
		t.Fatalf("page window %d", got)
	}
}
