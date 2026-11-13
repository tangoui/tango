//go:build linux && !android

package linux

// Streaming custom-scheme responses (WebKitGTK 2.36+,
// webkit_uri_scheme_request_finish_with_response). Host.ServeAsset runs on
// its own goroutine, as the contract prefers, and writes into a pipe whose
// read end WebKit consumes as a GUnixInputStream, so Server-Sent Events,
// long polls and large files are delivered as they are written instead of
// after the handler returns.
//
// Small responses keep their exact Content-Length: writes are buffered up
// to streamBuffer bytes and sent with the head when the handler returns.
// The head goes out earlier when the handler calls Flush (http.Flusher;
// SSE does), writes more than streamBuffer bytes, or set Content-Length
// itself. If WebKit stops reading (the page went away) writes fail and
// the request's context is canceled.

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"sync"
	"syscall"

	"github.com/tangoui/tango/internal/platform"
)

const streamBuffer = 64 << 10

var ssx struct {
	once         sync.Once
	ok           bool
	objectRef    func(o ptr) ptr
	unixInputNew func(fd int32, closeFD bool) ptr
}

func bindSchemeStream() bool {
	ssx.once.Do(func() {
		if webkitURISchemeRequestFinishWithResponse == nil {
			return
		}
		var bd binder
		bd.req(&ssx.objectRef, "g_object_ref", libGObject)
		bd.req(&ssx.unixInputNew, "g_unix_input_stream_new", libGIO)
		ssx.ok = len(bd.missing) == 0
	})
	return ssx.ok
}

// streamWriter is the http.ResponseWriter of a streamed request. Its
// methods run on the handler's goroutine.
type streamWriter struct {
	req    ptr // WebKitURISchemeRequest, referenced until the head is delivered
	method string
	cancel context.CancelFunc

	mu       sync.Mutex
	header   http.Header
	status   int
	buf      bytes.Buffer // body written before the head went out
	headSent bool
	rfd      int      // pipe read end, owned by GLib once the head is delivered
	w        *os.File // pipe write end
	werr     error
}

// streamScheme serves r streamed; false if WebKit cannot take a streamed
// response (the caller then buffers as before).
func (b *backend) streamScheme(req ptr, sid platform.SurfaceID, r *http.Request) bool {
	if !bindSchemeStream() {
		return false
	}
	var fds [2]int
	if err := syscall.Pipe2(fds[:], syscall.O_CLOEXEC); err != nil {
		return false
	}
	ctx, cancel := context.WithCancel(r.Context())
	sw := &streamWriter{
		req: ssx.objectRef(req), method: r.Method, cancel: cancel,
		header: http.Header{}, rfd: fds[0], w: os.NewFile(uintptr(fds[1]), "tango-asset"),
	}
	r = r.WithContext(ctx)
	go func() {
		defer sw.finish()
		b.host.ServeAsset(sid, sw, r)
	}()
	return true
}

func (w *streamWriter) Header() http.Header { return w.header }

func (w *streamWriter) WriteHeader(code int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.headSent || w.status != 0 || code < 200 || code > 999 {
		return // informational responses are not forwarded
	}
	w.status = code
}

func (w *streamWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.werr != nil {
		return 0, w.werr
	}
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if !w.headSent {
		w.buf.Write(p)
		if w.buf.Len() > streamBuffer || w.header.Get("Content-Length") != "" {
			w.sendHead(false)
		}
		return len(p), w.werr
	}
	return w.write(p)
}

// Flush sends the head and everything written so far.
func (w *streamWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.headSent {
		w.sendHead(false)
	}
}

// finish runs when the handler returned: send the head if still pending
// (with the exact length) and end the body.
func (w *streamWriter) finish() {
	w.mu.Lock()
	if !w.headSent {
		w.sendHead(true)
	}
	w.mu.Unlock()
	w.w.Close()
	w.cancel()
}

// sendHead hands the head to WebKit on the main thread, then writes the
// buffered body into the pipe. w.mu held.
func (w *streamWriter) sendHead(done bool) {
	w.headSent = true
	resp, length := streamHead(w.header, w.status, w.buf.Bytes(), w.method, done)
	req, rfd := w.req, w.rfd
	later(0, func() { deliverStream(req, rfd, resp, length) })
	if w.buf.Len() > 0 && length != 0 {
		w.write(w.buf.Bytes())
	}
	w.buf = bytes.Buffer{}
}

func (w *streamWriter) write(p []byte) (int, error) {
	n, err := w.w.Write(p)
	if err != nil {
		if errors.Is(err, syscall.EPIPE) {
			err = errors.New("tango/linux: the page stopped reading the response")
		}
		w.werr = err
		w.cancel()
	}
	return n, err
}

// deliverStream finishes the scheme request with the pipe's read end as
// the body. Main thread.
func deliverStream(req ptr, rfd int, resp assetResponse, length int64) {
	stream := ssx.unixInputNew(int32(rfd), true) // closes rfd with the stream
	r := webkitURISchemeResponseNew(stream, length)
	webkitURISchemeResponseSetStatus(r, uint32(resp.Status), cstr(resp.Reason))
	if resp.ContentType != "" {
		webkitURISchemeResponseSetContentType(r, resp.ContentType)
	}
	hdrs := soupMessageHeadersNew(soupMessageHeadersResponse)
	for _, kv := range resp.Headers {
		soupMessageHeadersAppend(hdrs, kv[0], kv[1])
	}
	webkitURISchemeResponseSetHTTPHeaders(r, hdrs) // takes ownership
	webkitURISchemeRequestFinishWithResponse(req, r)
	gObjectUnref(r)
	gObjectUnref(stream)
	gObjectUnref(req)
}
