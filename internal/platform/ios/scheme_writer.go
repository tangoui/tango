package ios

// The pure-Go half of the tango:// scheme handler: it turns a native
// request into an *http.Request and an http.Handler's response into calls
// on the WKURLSchemeTask (respond, data, finish). Kept free of build tags
// and cgo so it is tested on any host (like canvas_keys.go); scheme.go
// wires it to bridge.m.
//
// The handler (Host.ServeAsset) runs on its own goroutine. The writer
// keeps the response's actions in its own FIFO and asks for a drain on the
// main thread (kick; in production tg_task_kick, a block on the main run
// loop in the common modes). The drain takes actions from the front of
// the FIFO and makes the task calls (taskSink). The order therefore does
// not depend on the order in which the run loop runs blocks: CFRunLoop
// detaches its block list while it runs a pass, so a block that starts a
// nested run loop lets later blocks run before earlier ones of the same
// pass. A drain only says "go on with this task"; any drain, early or
// late, delivers the same next action.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync"

	"github.com/tangoui/tango/internal/platform"
)

// buildRequest makes a server-style request (RequestURI set) for an
// http.Handler. headers holds "Key: value" lines.
func buildRequest(ctx context.Context, method, rawURL, headers string, body []byte) (*http.Request, error) {
	if method == "" {
		method = http.MethodGet
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if u.Path == "" {
		u.Path = "/"
	}
	var rd io.Reader = http.NoBody
	if len(body) > 0 {
		rd = bytes.NewReader(body)
	}
	r, err := http.NewRequestWithContext(ctx, method, u.String(), rd)
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(headers, "\n") {
		if k, v, ok := strings.Cut(line, ": "); ok {
			r.Header.Add(k, v)
		}
	}
	r.Host = u.Host
	r.RequestURI = u.RequestURI()
	r.Proto, r.ProtoMajor, r.ProtoMinor = "HTTP/1.1", 1, 1
	r.ContentLength = int64(len(body))
	r.RemoteAddr = "webview"
	return r, nil
}

// flushAt is how much a handler may write before it is passed on without an
// explicit Flush.
const flushAt = 64 << 10

// pendingLimit bounds the bytes of one response that were passed on but
// have not reached WebKit yet (they wait for the main thread). Beyond it
// Write and Flush block until the main thread caught up, so a fast
// long-lived response cannot pile up memory while the main thread is busy.
const pendingLimit = 1 << 20

// taskSink makes the WKURLSchemeTask calls. Main thread only, in order:
// respond once, data any number of times, then finish.
type taskSink interface {
	respond(status int, header http.Header)
	data(p []byte)
	finish()
}

// Action kinds.
const (
	opRespond = 1
	opData    = 2
	opFinish  = 3
)

// taskAction is one taskSink call waiting for the main thread.
type taskAction struct {
	op     int
	status int         // opRespond
	header http.Header // opRespond; the writer's copy, owned by the action
	data   []byte      // opData; owned by the action
}

// responseWriter is the http.ResponseWriter (http.Flusher, FlushError)
// handed to Host.ServeAsset. Its methods are safe for concurrent use; the
// response ends when the handler returns (finish).
//
// Ordering: respond is queued exactly once and before any data, data in
// write order, finish last; drain hands them to the sink strictly from the
// front of the queue. After stop (webView:stopURLSchemeTask:) queued
// actions are dropped and nothing more reaches the sink; blocked and later
// Write / FlushError calls return context.Canceled.
type responseWriter struct {
	sink taskSink
	kick func() // asks for a drain on the main thread; never blocks
	ctx  context.Context
	path string // request path, to guess a Content-Type

	header http.Header // the handler's; cloned when the head is queued

	mu        sync.Mutex
	cond      sync.Cond // pending went down, or the task stopped / ctx ended
	status    int
	buf       bytes.Buffer // written, not yet queued
	sent      bool         // respond queued
	done      bool         // finish queued (or the task stopped)
	stopped   bool
	actions   []taskAction // waiting for the main thread, in order
	scheduled bool         // a drain was kicked and will see actions
	draining  bool         // a drain is on the main thread's stack
	pending   int          // data bytes queued, not yet handed to the sink
	unwatch   func() bool  // stops the ctx watcher
}

func newResponseWriter(ctx context.Context, sink taskSink, kick func(), reqPath string) *responseWriter {
	w := &responseWriter{sink: sink, kick: kick, ctx: ctx, path: reqPath, header: http.Header{}}
	w.cond.L = &w.mu
	w.unwatch = context.AfterFunc(ctx, func() {
		w.mu.Lock()
		w.cond.Broadcast()
		w.mu.Unlock()
	})
	return w
}

func (w *responseWriter) Header() http.Header { return w.header }

func (w *responseWriter) WriteHeader(code int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.status != 0 || w.sent || code < 200 || code > 999 {
		return // informational responses are not forwarded
	}
	w.status = code
}

// errLocked is why the writer takes no more data, or nil. w.mu held.
func (w *responseWriter) errLocked() error {
	switch {
	case w.stopped:
		return context.Canceled
	case w.done:
		return platform.ErrResponseFinished
	}
	return w.ctx.Err()
}

func (w *responseWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	if err := w.errLocked(); err != nil {
		w.mu.Unlock()
		return 0, err
	}
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.buf.Write(p)
	kick := false
	if w.buf.Len() >= flushAt {
		kick = w.flushLocked()
	}
	w.mu.Unlock()
	if kick {
		w.kick()
	}
	if err := w.wait(); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Flush is FlushError without the error (http.Flusher).
func (w *responseWriter) Flush() { _ = w.FlushError() }

// FlushError queues the head (once) and everything written so far for
// WebKit. It blocks while more than pendingLimit bytes wait for the main
// thread, and returns context.Canceled once the task was stopped.
func (w *responseWriter) FlushError() error {
	w.mu.Lock()
	if err := w.errLocked(); err != nil {
		w.mu.Unlock()
		return err
	}
	kick := w.flushLocked()
	w.mu.Unlock()
	if kick {
		w.kick()
	}
	return w.wait()
}

// flushLocked queues the head (once) and the buffered body. It reports
// whether the caller must kick a drain (after unlocking). w.mu held.
func (w *responseWriter) flushLocked() (kick bool) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if !w.sent {
		w.sent = true
		if w.header.Get("Content-Type") == "" && hasBody(w.status) {
			w.header.Set("Content-Type", contentType(w.path, w.buf.Bytes()))
		}
		kick = w.queueLocked(taskAction{op: opRespond, status: w.status, header: w.header.Clone()})
	}
	if w.buf.Len() > 0 {
		p := w.buf.Bytes()
		w.buf = bytes.Buffer{} // the action owns p now
		w.pending += len(p)
		kick = w.queueLocked(taskAction{op: opData, data: p}) || kick
	}
	return kick
}

// queueLocked appends a and reports whether a drain must be kicked for it.
// w.mu held.
func (w *responseWriter) queueLocked(a taskAction) bool {
	w.actions = append(w.actions, a)
	if w.scheduled {
		return false
	}
	w.scheduled = true
	return true
}

// wait blocks while more than pendingLimit bytes wait for the main thread.
func (w *responseWriter) wait() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	for w.pending > pendingLimit && !w.stopped && w.ctx.Err() == nil {
		w.cond.Wait()
	}
	if w.stopped {
		return context.Canceled
	}
	return w.ctx.Err()
}

// drain runs on the main thread and hands queued actions to the sink, one
// at a time from the front of the queue. It runs at most the actions
// present when it started and kicks itself again for the rest, so a fast
// writer cannot hold the main thread.
//
// A drain that starts while another drain of this writer is on the stack
// (a sink call ran a nested run loop, which ran a later kick) returns at
// once: the outer drain goes on from the front when the sink call
// returns, so no action is delivered twice, skipped or overtaken.
func (w *responseWriter) drain() {
	w.mu.Lock()
	if w.draining {
		w.mu.Unlock()
		return
	}
	w.draining = true
	n := len(w.actions)
	w.mu.Unlock()
	for range n {
		w.mu.Lock()
		if w.stopped || len(w.actions) == 0 {
			w.mu.Unlock()
			break
		}
		a := w.actions[0]
		w.actions[0] = taskAction{}
		w.actions = w.actions[1:]
		w.mu.Unlock()

		// Not under w.mu: a sink call may re-enter (WebKit can stop the
		// task from inside one; a nested run loop can run another drain).
		switch a.op {
		case opRespond:
			w.sink.respond(a.status, a.header)
		case opData:
			w.sink.data(a.data)
			w.mu.Lock()
			if !w.stopped {
				w.pending -= len(a.data)
				w.cond.Broadcast()
			}
			w.mu.Unlock()
		case opFinish:
			w.sink.finish()
		}
	}
	w.mu.Lock()
	w.draining = false
	again := len(w.actions) > 0 && !w.stopped
	if !again {
		w.actions, w.scheduled = nil, false
	}
	w.mu.Unlock()
	if again {
		w.kick()
	}
}

// finish completes the response after the handler returned. A response
// whose head was not sent yet gets an exact Content-Length.
func (w *responseWriter) finish() {
	w.mu.Lock()
	if w.done {
		w.mu.Unlock()
		return
	}
	if !w.sent && w.header.Get("Content-Length") == "" && hasBody(w.status) {
		w.header.Set("Content-Length", strconv.Itoa(w.buf.Len()))
	}
	kick := w.flushLocked()
	w.done = true
	kick = w.queueLocked(taskAction{op: opFinish}) || kick
	w.mu.Unlock()
	w.unwatch()
	if kick {
		w.kick()
	}
}

// stop runs on the main thread when WebKit stopped the task: queued
// actions are dropped, nothing more reaches the sink, and blocked writers
// wake up with context.Canceled.
func (w *responseWriter) stop() {
	w.mu.Lock()
	w.done, w.stopped = true, true
	w.buf.Reset()
	w.actions, w.pending = nil, 0
	w.cond.Broadcast()
	w.mu.Unlock()
	w.unwatch()
}

// replaceWithError discards whatever the handler buffered and turns the
// response into a 500, unless the head is already on its way to WebKit
// (then the truncated body is all we can do).
func (w *responseWriter) replaceWithError(cause any) {
	w.mu.Lock()
	if w.sent || w.done {
		w.mu.Unlock()
		return
	}
	w.header, w.status = http.Header{}, 0
	w.buf.Reset()
	w.mu.Unlock()
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, "tango: asset handler panic: %v", cause)
}

// runAssetHandler runs serve (the handler) and completes w; a panic
// becomes a 500 when nothing was sent yet.
func runAssetHandler(w *responseWriter, serve func()) {
	if crash := catchPanic(serve); crash != nil {
		w.replaceWithError(crash)
	}
	w.finish()
}

func catchPanic(fn func()) (crash any) {
	defer func() { crash = recover() }()
	fn()
	return nil
}

func hasBody(status int) bool {
	return status != http.StatusNoContent && status != http.StatusNotModified
}

func contentType(p string, body []byte) string {
	if t := mime.TypeByExtension(strings.ToLower(path.Ext(p))); t != "" {
		return t
	}
	if len(body) == 0 {
		return "application/octet-stream"
	}
	return http.DetectContentType(body)
}
