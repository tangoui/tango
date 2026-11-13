//go:build darwin && !ios

package darwin

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
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
// times, then finish.
type taskSink interface {
	respond(status int, header http.Header)
	data(p []byte)
	finish()
}

// streamChunk is how much a handler may write before the writer passes the
// bytes on without an explicit Flush.
const streamChunk = 64 << 10

var errResponseDone = errors.New("tango/darwin: response already finished")

// assetWriter is the http.ResponseWriter (and http.Flusher) handed to
// Host.ServeAsset. It is used on the main thread only, during ServeAsset:
// the response ends when ServeAsset returns, as with net/http.
type assetWriter struct {
	sink    taskSink
	path    string // request path, to guess a Content-Type
	header  http.Header
	status  int
	buf     bytes.Buffer
	sent    bool // respond called
	done    bool // finish called (or the task stopped)
	stopped bool
}

func newAssetWriter(sink taskSink, reqPath string) *assetWriter {
	return &assetWriter{sink: sink, path: reqPath, header: http.Header{}}
}

func (w *assetWriter) Header() http.Header { return w.header }

func (w *assetWriter) WriteHeader(code int) {
	if w.status != 0 || w.sent || code < 100 || code > 999 {
		return
	}
	if code < 200 { // informational responses are not forwarded
		return
	}
	w.status = code
}

func (w *assetWriter) Write(p []byte) (int, error) {
	if w.done {
		if w.stopped {
			return 0, context.Canceled
		}
		return 0, errResponseDone
	}
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	w.buf.Write(p)
	if w.buf.Len() >= streamChunk {
		w.Flush()
	}
	return len(p), nil
}

// Flush sends the headers (once) and everything written so far to WebKit.
func (w *assetWriter) Flush() {
	if w.done {
		return
	}
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if !w.sent {
		w.sent = true
		if w.header.Get("Content-Type") == "" && bodyAllowed(w.status) {
			w.header.Set("Content-Type", guessContentType(w.path, w.buf.Bytes()))
		}
		w.sink.respond(w.status, w.header.Clone())
	}
	if w.buf.Len() > 0 {
		w.sink.data(bytes.Clone(w.buf.Bytes()))
		w.buf.Reset()
	}
}

// finish completes the response after the handler returned.
func (w *assetWriter) finish() {
	if w.done {
		return
	}
	if !w.sent && w.header.Get("Content-Length") == "" && bodyAllowed(w.status) {
		w.header.Set("Content-Length", strconv.Itoa(w.buf.Len()))
	}
	w.Flush()
	w.done = true
	w.sink.finish()
}

// stop is called when WebKit cancelled the task: nothing more may reach it.
func (w *assetWriter) stop() {
	w.done, w.stopped = true, true
	w.buf.Reset()
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
