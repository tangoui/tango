//go:build ios

package ios

/*
#include <stdlib.h>
#include "bridge.h"
*/
import "C"

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
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

// The tango:// scheme handler (bridge.m) hands each WKURLSchemeTask to
// tangoIOSTaskStart, which turns it into an *http.Request, runs
// Host.ServeAsset synchronously on the main thread and streams the response
// back through tg_task_*. The response ends when ServeAsset returns.

// schemeTask is one live WKURLSchemeTask. Main thread only.
type schemeTask struct {
	handle  C.uint64_t
	cancel  context.CancelFunc
	stopped bool // WebKit cancelled it: nothing more may reach it
	sent    bool // response head sent
	ended   bool
}

//export tangoIOSTaskStart
func tangoIOSTaskStart(sid, handle C.uint64_t, method, rawURL, headers *C.char, body unsafe.Pointer, n C.int) {
	b := current.Load()
	t := &schemeTask{handle: handle}
	if b == nil || !b.ready.Load() {
		t.fail(http.StatusServiceUnavailable, "app not running")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.cancel = cancel
	defer cancel()
	b.tasks[uint64(handle)] = t
	defer delete(b.tasks, uint64(handle))

	var data []byte
	if n > 0 {
		data = C.GoBytes(body, n)
	}
	req, err := buildRequest(ctx, C.GoString(method), C.GoString(rawURL), C.GoString(headers), data)
	if err != nil {
		t.fail(http.StatusBadRequest, err.Error())
		return
	}
	w := &responseWriter{t: t, header: http.Header{}, path: req.URL.Path}
	func() {
		defer func() {
			if p := recover(); p != nil && !t.sent {
				w.header, w.status = http.Header{}, 0
				w.buf.Reset()
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "tango: asset handler panic: %v", p)
			}
		}()
		b.h.ServeAsset(platform.SurfaceID(sid), w, req)
	}()
	w.end()
}

//export tangoIOSTaskStop
func tangoIOSTaskStop(handle C.uint64_t) {
	b := current.Load()
	if b == nil {
		return
	}
	if t := b.tasks[uint64(handle)]; t != nil {
		t.stopped = true
		t.cancel()
	}
}

func (t *schemeTask) fail(status int, msg string) {
	t.respond(status, http.Header{"Content-Type": {"text/plain; charset=utf-8"}})
	t.data([]byte(msg))
	t.finish()
}

func (t *schemeTask) respond(status int, h http.Header) {
	if t.stopped || t.sent {
		return
	}
	t.sent = true
	var sb strings.Builder
	for k, vs := range h {
		sb.WriteString(k)
		sb.WriteString(": ")
		sb.WriteString(strings.Join(vs, ", "))
		sb.WriteByte('\n')
	}
	hs, free := cstring(sb.String())
	defer free()
	C.tg_task_respond(t.handle, C.int(status), hs)
}

func (t *schemeTask) data(p []byte) {
	if t.stopped || len(p) == 0 {
		return
	}
	C.tg_task_data(t.handle, unsafe.Pointer(&p[0]), C.size_t(len(p)))
}

func (t *schemeTask) finish() {
	if t.stopped || t.ended {
		return
	}
	t.ended = true
	C.tg_task_finish(t.handle)
}

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

// responseWriter is the http.ResponseWriter (and http.Flusher) given to
// Host.ServeAsset.
type responseWriter struct {
	t      *schemeTask
	path   string
	header http.Header
	status int
	buf    bytes.Buffer
}

func (w *responseWriter) Header() http.Header { return w.header }

func (w *responseWriter) WriteHeader(code int) {
	if w.status != 0 || w.t.sent || code < 200 || code > 999 {
		return // informational responses are not forwarded
	}
	w.status = code
}

func (w *responseWriter) Write(p []byte) (int, error) {
	if w.t.stopped {
		return 0, context.Canceled
	}
	if w.t.ended {
		return 0, http.ErrBodyNotAllowed
	}
	if w.status == 0 {
		w.status = http.StatusOK
	}
	w.buf.Write(p)
	if w.buf.Len() >= flushAt {
		w.Flush()
	}
	return len(p), nil
}

func (w *responseWriter) Flush() {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if !w.t.sent {
		if w.header.Get("Content-Type") == "" && hasBody(w.status) {
			w.header.Set("Content-Type", contentType(w.path, w.buf.Bytes()))
		}
		w.t.respond(w.status, w.header)
	}
	if w.buf.Len() > 0 {
		w.t.data(w.buf.Bytes())
		w.buf.Reset()
	}
}

func (w *responseWriter) end() {
	if !w.t.sent && w.header.Get("Content-Length") == "" && hasBody(w.status) {
		w.header.Set("Content-Length", strconv.Itoa(w.buf.Len()))
	}
	w.Flush()
	w.t.finish()
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
