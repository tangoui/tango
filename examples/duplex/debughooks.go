package main

// Test hooks that act on the calling page's Surface, for the down:poll tests
// of internal/platform/darwin (downpoll_e2e_test.go). Registered only with
// TANGO_SCHEME_PROBE=1 (services.Probe), like Debug.Stats.
//
//   - "Debug.SendEval" [rounds, throwAt, pad]: for i in 0..rounds-1, from
//     the handler's goroutine, Send("a:i"), Eval(a script that pushes "b:i"
//     to window.__dbgLog, and throws in round throwAt), Send("c:i"). With
//     pad > 0 every round starts with Send("f:" + pad bytes), a message too
//     large for down:poll's idle rule (internal/downq IdleEvalBytes), which
//     keeps the poll lane busy so that the rest queue there and the Evals
//     become barriers. Replies {} after the last call.
//   - "Debug.Burst" [n, prefix]: Send(prefix + i) for i in 0..n-1; replies
//     {"t0": ms} with the wall clock (Unix ms) before the first Send.
//   - "Debug.OccludedBurst" [n, waitMs, restoreMs]: minimizes the window,
//     waits waitMs, sends n messages "o:i" (as Burst), and restores the
//     window restoreMs after the first Send. Replies {"t0", "t1"}: the wall
//     clock before the first and after the last Send. The reply itself
//     goes after the messages.
//   - "Debug.Restore": restores the window.
//   - "Debug.WindowState": {"minimized": bool} (the page's
//     visibilityState says little: WebKit also reports hidden for an
//     occluded window, or with the display asleep).

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/examples/duplex/services"
	"github.com/tangoui/tango/ipc"
)

// debugSurface is the part of *tango.Surface the hooks use.
type debugSurface interface {
	Send(string)
	Eval(string)
	Minimize() error
	Restore() error
	State() tango.WindowState
}

func registerDebug(r *ipc.Router) {
	if !services.Probe() {
		return
	}
	r.Handle("Debug.SendEval", debugSendEval)
	r.Handle("Debug.Burst", debugBurst)
	r.Handle("Debug.OccludedBurst", debugOccludedBurst)
	r.Handle("Debug.Restore", func(ctx context.Context, _ json.RawMessage) (any, error) {
		s, err := debugConn(ctx)
		if err != nil {
			return nil, err
		}
		return struct{}{}, s.Restore()
	})
	r.Handle("Debug.WindowState", func(ctx context.Context, _ json.RawMessage) (any, error) {
		s, err := debugConn(ctx)
		if err != nil {
			return nil, err
		}
		return map[string]bool{"minimized": s.State() == tango.StateMinimized}, nil
	})
}

func debugConn(ctx context.Context) (debugSurface, error) {
	c, ok := ipc.ConnFrom(ctx)
	s, ok2 := c.(debugSurface)
	if !ok || !ok2 {
		return nil, ipc.NewError(ipc.CodeInternal, "Debug: the caller is not a Surface")
	}
	return s, nil
}

func debugArgs(args json.RawMessage, dst ...any) error {
	var a []json.RawMessage
	if err := json.Unmarshal(args, &a); err != nil {
		return ipc.NewError(ipc.CodeBadArgs, err.Error())
	}
	for i := range min(len(a), len(dst)) {
		if err := json.Unmarshal(a[i], dst[i]); err != nil {
			return ipc.NewError(ipc.CodeBadArgs, err.Error())
		}
	}
	return nil
}

func debugSendEval(ctx context.Context, args json.RawMessage) (any, error) {
	rounds, throwAt, pad := 0, -1, 0
	if err := debugArgs(args, &rounds, &throwAt, &pad); err != nil {
		return nil, err
	}
	filler := "f:" + strings.Repeat("x", max(pad, 0))
	s, err := debugConn(ctx)
	if err != nil {
		return nil, err
	}
	for i := range rounds {
		n := strconv.Itoa(i)
		if pad > 0 {
			s.Send(filler)
		}
		s.Send("a:" + n)
		js := `(window.__dbgLog = window.__dbgLog || []).push("b:` + n + `")`
		if i == throwAt {
			js += `; throw new Error("Debug.SendEval: round ` + n + ` throws")`
		}
		s.Eval(js)
		s.Send("c:" + n)
	}
	return struct{}{}, nil
}

func burst(s debugSurface, n int, prefix string) {
	for i := range n {
		s.Send(prefix + strconv.Itoa(i))
	}
}

func debugBurst(ctx context.Context, args json.RawMessage) (any, error) {
	n, prefix := 0, "m:"
	if err := debugArgs(args, &n, &prefix); err != nil {
		return nil, err
	}
	s, err := debugConn(ctx)
	if err != nil {
		return nil, err
	}
	t0 := time.Now().UnixMilli()
	burst(s, n, prefix)
	return map[string]int64{"t0": t0}, nil
}

func debugOccludedBurst(ctx context.Context, args json.RawMessage) (any, error) {
	n, waitMs, restoreMs := 0, 0, 0
	if err := debugArgs(args, &n, &waitMs, &restoreMs); err != nil {
		return nil, err
	}
	s, err := debugConn(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.Minimize(); err != nil {
		return nil, err
	}
	time.Sleep(time.Duration(waitMs) * time.Millisecond)
	t0 := time.Now().UnixMilli()
	time.AfterFunc(time.Duration(restoreMs)*time.Millisecond, func() { s.Restore() })
	burst(s, n, "o:")
	return map[string]int64{"t0": t0, "t1": time.Now().UnixMilli()}, nil
}
