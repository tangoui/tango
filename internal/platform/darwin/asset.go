//go:build darwin && !ios

package darwin

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

// This file holds the pure-Go half of the WKURLSchemeHandler adapter: it
// turns a native request into *http.Request and collects an http.Handler's
// response into calls on a taskSink (a WKURLSchemeTask in production, a fake
// in tests).

// nativeRequest is what the scheme handler reads from NSURLRequest.
type nativeRequest struct {
	Method string
	URL    string
	Header [][2]string
	Body   []byte
}

// withOrigin adds an Origin header naming the origin of docURL to h unless
// h has one (WebKit omits it on custom-scheme requests).
func withOrigin(h [][2]string, docURL string) [][2]string {
	for _, kv := range h {
		if strings.EqualFold(kv[0], "Origin") {
			return h
		}
	}
	u, err := url.Parse(docURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return h // about:blank, data: ...: no origin to name
	}
	return append(h, [2]string{"Origin", strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)})
}

// buildRequest converts a native request into a server-style *http.Request
// (RequestURI set, Host from the URL) ready for an http.Handler.
func buildRequest(ctx context.Context, nr nativeRequest) (*http.Request, error) {
	method := nr.Method
	if method == "" {
		method = http.MethodGet
	}
	u, err := url.Parse(nr.URL)
	if err != nil {
		return nil, err
	}
	if u.Path == "" {
		u.Path = "/"
	}
	var body io.Reader = http.NoBody
	if len(nr.Body) > 0 {
		body = bytes.NewReader(nr.Body)
	}
	r, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	for _, kv := range nr.Header {
		r.Header.Add(kv[0], kv[1])
	}
	r.Host = u.Host
	r.RequestURI = u.RequestURI()
	r.Proto, r.ProtoMajor, r.ProtoMinor = "HTTP/1.1", 1, 1
	r.ContentLength = int64(len(nr.Body))
	r.RemoteAddr = "webview"
	return r, nil
}

// taskSink receives the response, in order: respond once, data any number of
// times, then finish. Its methods run on the main thread (in production
// they call the WKURLSchemeTask).
type taskSink interface {
	respond(status int, header http.Header)
	data(p []byte)
	finish()
}

// streamChunk is how much a handler may write before the writer passes the
// bytes on without an explicit Flush.
const streamChunk = 64 << 10

// pendingLimit bounds the bytes of one response that were passed on but
// have not reached the sink yet (they wait for the main thread). Beyond it
// Write and Flush block until the main thread caught up, so a fast
// long-lived response cannot pile up memory while the main thread is busy.
const pendingLimit = 1 << 20

// sinkAction is one taskSink call waiting for the main thread.
type sinkAction struct {
	kind   byte // 'r' respond, 'd' data, 'f' finish
	status int
	header http.Header
	data   []byte
}

// assetWriter is the http.ResponseWriter (http.Flusher, FlushError) handed
// to Host.ServeAsset. The handler runs on its own goroutine; the writer
// turns what it writes into sink calls that run, in order, through post
// (the main-thread queue). Its methods are safe for concurrent use. The
// response ends when the handler returns (finish).
//
// Ordering: respond is queued exactly once and before any data, data in
// write order, finish last. After stop (webView:stopURLSchemeTask:) queued
// calls are dropped and nothing more reaches the sink; blocked and later
// Write / FlushError calls return context.Canceled.
type assetWriter struct {
	sink taskSink
	post func(func()) // runs fn on the main thread, in call order
	ctx  context.Context
	path string // request path, to guess a Content-Type; set before serving

	header http.Header // the handler's; cloned when the head is queued

	mu        sync.Mutex
	cond      sync.Cond // pending went down, or the task stopped / ctx ended
	status    int
	buf       bytes.Buffer // written, not yet queued
	sent      bool         // respond queued
	done      bool         // finish queued (or the task stopped)
	stopped   bool
	actions   []sinkAction // waiting for the main thread
	scheduled bool         // a drain is posted and will see actions
	draining  bool         // a drain is on the main thread's stack
	pending   int          // data bytes in actions
	unwatch   func() bool  // stops the ctx watcher
}

func newAssetWriter(ctx context.Context, sink taskSink, post func(func()), reqPath string) *assetWriter {
	w := &assetWriter{sink: sink, post: post, ctx: ctx, path: reqPath, header: http.Header{}}
	w.cond.L = &w.mu
	w.unwatch = context.AfterFunc(ctx, func() {
		w.mu.Lock()
		w.cond.Broadcast()
		w.mu.Unlock()
	})
	return w
}

func (w *assetWriter) Header() http.Header { return w.header }

func (w *assetWriter) WriteHeader(code int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.status != 0 || w.sent || code < 100 || code > 999 {
		return
	}
	if code < 200 { // informational responses are not forwarded
		return
	}
	w.status = code
}

// errLocked is why the writer takes no more data, or nil. w.mu held.
func (w *assetWriter) errLocked() error {
	switch {
	case w.stopped:
		return context.Canceled
	case w.done:
		return platform.ErrResponseFinished
	}
	return w.ctx.Err()
}

func (w *assetWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	if err := w.errLocked(); err != nil {
		w.mu.Unlock()
		return 0, err
	}
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.buf.Write(p)
	schedule := false
	if w.buf.Len() >= streamChunk {
		schedule = w.flushLocked()
	}
	w.mu.Unlock()
	if schedule {
		w.post(w.drain)
	}
	if err := w.wait(); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Flush is FlushError without the error (http.Flusher).
func (w *assetWriter) Flush() { _ = w.FlushError() }

// FlushError queues the head (once) and everything written so far for
// WebKit. It blocks while more than pendingLimit bytes wait for the main
// thread, and returns context.Canceled once the task was stopped.
func (w *assetWriter) FlushError() error {
	w.mu.Lock()
	if err := w.errLocked(); err != nil {
		w.mu.Unlock()
		return err
	}
	schedule := w.flushLocked()
	w.mu.Unlock()
	if schedule {
		w.post(w.drain)
	}
	return w.wait()
}

// flushLocked queues the head (once) and the buffered body. It reports
// whether the caller must post a drain (after unlocking: an inline post
// runs drain, which takes w.mu). w.mu held.
func (w *assetWriter) flushLocked() (schedule bool) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if !w.sent {
		w.sent = true
		if w.header.Get("Content-Type") == "" && bodyAllowed(w.status) {
			w.header.Set("Content-Type", guessContentType(w.path, w.buf.Bytes()))
		}
		schedule = w.queueLocked(sinkAction{kind: 'r', status: w.status, header: w.header.Clone()})
	}
	if w.buf.Len() > 0 {
		p := w.buf.Bytes()
		w.buf = bytes.Buffer{} // the action owns p now
		w.pending += len(p)
		schedule = w.queueLocked(sinkAction{kind: 'd', data: p}) || schedule
	}
	return schedule
}

// queueLocked appends a and reports whether a drain must be posted for it.
// w.mu held.
func (w *assetWriter) queueLocked(a sinkAction) bool {
	w.actions = append(w.actions, a)
	if w.scheduled {
		return false
	}
	w.scheduled = true
	return true
}

// wait blocks while more than pendingLimit bytes wait for the main thread.
func (w *assetWriter) wait() error {
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

// drain runs on the main thread and hands the queued actions to the sink
// in order, one at a time from the front of the queue. It runs at most the
// actions present when it started and posts itself again for the rest, so
// a fast writer cannot hold the main thread.
//
// A drain that starts while another drain of this writer is on the stack
// (a sink call ran a nested run loop that ran a posted drain) returns at
// once: the outer drain goes on from the front when the sink call returns,
// so no action is delivered twice, skipped or overtaken. (Today a second
// drain cannot be posted while one runs, because scheduled stays true until
// the drain ends; the guard keeps it that way if that changes, and matches
// internal/platform/ios's writer.)
func (w *assetWriter) drain() {
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
		w.actions[0] = sinkAction{}
		w.actions = w.actions[1:]
		w.mu.Unlock()

		// Not under w.mu: a sink call may re-enter (WebKit can stop the
		// task from inside one; a nested run loop can run another drain).
		switch a.kind {
		case 'r':
			w.sink.respond(a.status, a.header)
		case 'd':
			w.sink.data(a.data)
			w.mu.Lock()
			if !w.stopped { // stop zeroed pending; keep it at zero
				w.pending -= len(a.data)
				w.cond.Broadcast()
			}
			w.mu.Unlock()
		case 'f':
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
		w.post(w.drain)
	}
}

// finish completes the response after the handler returned. A response
// whose head was not sent yet gets an exact Content-Length.
func (w *assetWriter) finish() {
	w.mu.Lock()
	if w.done {
		w.mu.Unlock()
		return
	}
	if !w.sent && w.header.Get("Content-Length") == "" && bodyAllowed(w.status) {
		w.header.Set("Content-Length", strconv.Itoa(w.buf.Len()))
	}
	schedule := w.flushLocked()
	w.done = true
	schedule = w.queueLocked(sinkAction{kind: 'f'}) || schedule
	w.mu.Unlock()
	w.unwatch()
	if schedule {
		w.post(w.drain)
	}
}

// stop runs on the main thread when WebKit stopped the task: queued
// actions are dropped, nothing more reaches the sink, and blocked writers
// wake up with context.Canceled.
func (w *assetWriter) stop() {
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
func (w *assetWriter) replaceWithError(cause any) {
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
func runAssetHandler(w *assetWriter, serve func()) {
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

func bodyAllowed(status int) bool {
	return status != http.StatusNoContent && status != http.StatusNotModified && status >= 200
}

func guessContentType(p string, body []byte) string {
	if ext := path.Ext(p); ext != "" {
		if t := mime.TypeByExtension(strings.ToLower(ext)); t != "" {
			return t
		}
	}
	if len(body) == 0 {
		return "application/octet-stream"
	}
	return http.DetectContentType(body)
}
