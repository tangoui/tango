package android

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// Adapting WebViewClient.shouldInterceptRequest to net/http.
//
// The handler runs on its own goroutine. The request thread waits until
// the response is committed (WriteHeader, the first Write or Flush) or the
// handler returns; a finished response is handed to WebView whole, an
// unfinished one is streamed through an InputStream that reads from the
// writer, so slow or never-ending handlers (server-sent events) work and
// never involve the main thread. Main-frame HTML and redirects are always
// waited for in full: the first to insert the bridge script, the second
// because WebResourceResponse rejects 3xx statuses, so same-origin
// redirects are followed here.

// maxRedirects bounds same-origin redirects followed per request.
const maxRedirects = 5

// streamBuffer is how much a streaming handler may write ahead of the
// WebView reading it before Write blocks.
const streamBuffer = 256 << 10

// errClosed is returned by Write once WebView stopped reading.
var errClosed = errors.New("tango: asset response closed by the WebView")

// headersToSkip are request headers not forwarded to the handler. The
// conditional ones would let a handler answer 304, which WebResourceResponse
// cannot carry (and intercepted responses are not cached anyway).
var headersToSkip = map[string]bool{
	"If-None-Match":     true,
	"If-Modified-Since": true,
}

// newAssetRequest builds the server-side request handed to Host.ServeAsset
// from what shouldInterceptRequest offers: method, URL and headers as a flat
// name, value list. The body is empty: Android does not expose it; the
// caller adds the one the page stashed (body.go, setBody).
func newAssetRequest(ctx context.Context, method, rawURL string, headers []string) (*http.Request, error) {
	if method == "" {
		method = http.MethodGet
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("tango: bad asset URL %q: %w", rawURL, err)
	}
	r, err := http.NewRequestWithContext(ctx, method, rawURL, http.NoBody)
	if err != nil {
		return nil, err
	}
	h := http.Header{}
	for i := 0; i+1 < len(headers); i += 2 {
		k := http.CanonicalHeaderKey(strings.TrimSpace(headers[i]))
		if k == "" || headersToSkip[k] {
			continue
		}
		h.Add(k, headers[i+1])
	}
	r.Header = h
	r.Host = u.Host
	r.RequestURI = u.RequestURI()
	r.Proto, r.ProtoMajor, r.ProtoMinor = "HTTP/1.1", 1, 1
	r.RemoteAddr = "127.0.0.1:0"
	return r, nil
}

// assetWriter is the http.ResponseWriter (and Flusher) given to
// Host.ServeAsset, and the source of the body WebView reads.
type assetWriter struct {
	header http.Header

	mu        sync.Mutex
	cond      sync.Cond
	status    int         // 0 until committed
	committed http.Header // header snapshot at commit
	buf       []byte      // body not yet read
	streaming bool        // the reader takes the body as it comes: Write may block
	done      bool        // handler returned
	closed    bool        // reader gone

	commitCh chan struct{} // closed at commit
	doneCh   chan struct{} // closed when the handler returned
}

func newAssetWriter() *assetWriter {
	w := &assetWriter{header: http.Header{}, commitCh: make(chan struct{}), doneCh: make(chan struct{})}
	w.cond.L = &w.mu
	return w
}

func (w *assetWriter) Header() http.Header { return w.header }

func (w *assetWriter) WriteHeader(code int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.commitLocked(code, nil)
}

// commitLocked fixes status and headers, sniffing a Content-Type from the
// first body bytes when the handler set none, as net/http does.
func (w *assetWriter) commitLocked(code int, first []byte) {
	if w.status != 0 {
		return
	}
	if code < 100 || code > 999 {
		code = http.StatusInternalServerError
	}
	h := w.header.Clone()
	if h == nil {
		h = http.Header{}
	}
	if _, set := h["Content-Type"]; !set && len(first) > 0 {
		h.Set("Content-Type", http.DetectContentType(first))
	}
	w.status = code
	w.committed = h
	close(w.commitCh)
}

func (w *assetWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.commitLocked(http.StatusOK, p)
	if w.closed {
		return 0, errClosed
	}
	for w.streaming && len(w.buf) >= streamBuffer && !w.closed {
		w.cond.Wait()
	}
	if w.closed {
		return 0, errClosed
	}
	w.buf = append(w.buf, p...)
	w.cond.Broadcast()
	return len(p), nil
}

// Flush commits the headers so a streaming response reaches WebView.
func (w *assetWriter) Flush() {
	w.mu.Lock()
	w.commitLocked(http.StatusOK, nil)
	w.mu.Unlock()
}

// finish marks the handler returned. A handler that wrote nothing commits
// 200 with an empty body.
func (w *assetWriter) finish() {
	w.mu.Lock()
	w.commitLocked(http.StatusOK, nil)
	w.done = true
	w.cond.Broadcast()
	w.mu.Unlock()
	close(w.doneCh)
}

// stream switches to streaming: from now on Write waits for Read when the
// buffer is full.
func (w *assetWriter) stream() {
	w.mu.Lock()
	w.streaming = true
	w.mu.Unlock()
}

// Read reads body bytes as the handler writes them; io.EOF-like (0, true)
// once the handler returned and everything was read.
func (w *assetWriter) Read(p []byte) (n int, eof bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for len(w.buf) == 0 && !w.done && !w.closed {
		w.cond.Wait()
	}
	if len(w.buf) == 0 {
		return 0, true
	}
	n = copy(p, w.buf)
	w.buf = w.buf[n:]
	if len(w.buf) == 0 {
		w.buf = nil
	}
	w.cond.Broadcast()
	return n, false
}

// closeRead tells the handler WebView stopped reading.
func (w *assetWriter) closeRead() {
	w.mu.Lock()
	w.closed = true
	w.buf = nil
	w.cond.Broadcast()
	w.mu.Unlock()
}

// body returns the whole body. Only after doneCh.
func (w *assetWriter) body() []byte {
	w.mu.Lock()
	defer w.mu.Unlock()
	b := w.buf
	w.buf = nil
	return b
}

// assetResult is what the request thread turns into a WebResourceResponse.
type assetResult struct {
	Status   int
	Reason   string
	MIME     string   // e.g. "text/html"; never empty
	Encoding string   // charset, or ""
	Headers  []string // flat name, value list, without Content-Type and Content-Length
	// Length is the handler's Content-Length, or -1. It is not passed as a
	// header: Android WebView adds a Content-Length of its own, the
	// InputStream's available() at the start, so a header would make two.
	// Page.respond reports Length through available() instead.
	Length int64
	Body     []byte   // whole body when Stream is nil
	Stream   *assetWriter
	Injected bool // the bridge script was inserted into this HTML
	cancel   context.CancelFunc
}

// serveFunc is Host.ServeAsset with the surface already bound.
type serveFunc func(w http.ResponseWriter, r *http.Request)

// serveAsset runs serve for r and waits for a response it can hand to
// WebView. script, if not empty, is inserted into a main-frame HTML
// response. cancel is called once the response is fully consumed: by
// serveAsset itself for whole bodies, by closeStream for streams.
func serveAsset(serve serveFunc, r *http.Request, cancel context.CancelFunc, script string) assetResult {
	origin := r.URL.Scheme + "://" + r.URL.Host
	for hop := 0; ; hop++ {
		w := newAssetWriter()
		go runHandler(serve, w, r)
		select {
		case <-w.commitCh:
		case <-w.doneCh:
		}
		status := w.status
		h := w.committed

		if status >= 300 && status < 400 {
			<-w.doneCh
			loc := h.Get("Location")
			next, err := r.URL.Parse(loc)
			if loc == "" || err != nil || status == http.StatusNotModified {
				cancel()
				return errorResult(http.StatusBadGateway, fmt.Sprintf("tango: status %d cannot be delivered to Android WebView", status))
			}
			if next.Scheme+"://"+next.Host != origin || hop >= maxRedirects {
				cancel()
				return errorResult(http.StatusBadGateway, "tango: Android WebView cannot follow a redirect to "+next.String())
			}
			method := r.Method
			if status != http.StatusTemporaryRedirect && status != http.StatusPermanentRedirect {
				method = http.MethodGet
			}
			nr, err := newAssetRequest(r.Context(), method, next.String(), flatHeader(r.Header))
			if err != nil {
				cancel()
				return errorResult(http.StatusBadGateway, err.Error())
			}
			if method == r.Method && r.GetBody != nil {
				// 307/308 repeat the request with its body.
				if rc, err := r.GetBody(); err == nil {
					data, _ := io.ReadAll(rc)
					setBody(nr, stashedBody{data: data, ctype: r.Header.Get("Content-Type")})
				}
			}
			r = nr
			continue
		}
		if status < 100 || status > 599 {
			<-w.doneCh
			cancel()
			return errorResult(http.StatusInternalServerError, fmt.Sprintf("tango: invalid status %d", status))
		}

		mt, enc := splitContentType(h.Get("Content-Type"))
		res := assetResult{Status: status, Reason: reason(status), MIME: mt, Encoding: enc, Length: contentLength(h), cancel: cancel}
		inject := script != "" && mt == "text/html" && h.Get("Content-Encoding") == ""
		h = h.Clone()
		h.Del("Content-Type")
		h.Del("Content-Length")
		if inject {
			<-w.doneCh
			res.Body = injectHTML(w.body(), script)
			res.Injected = true
			res.Length = -1 // the body's own length (WebView: available())
			h.Del("Content-Md5")
		}
		res.Headers = flatHeader(h)
		if inject {
			cancel()
			return res
		}
		select {
		case <-w.doneCh:
			res.Body = w.body()
			cancel()
		default:
			w.stream()
			res.Stream = w
		}
		return res
	}
}

// runHandler calls serve, turning a panic into a 500 like net/http.
func runHandler(serve serveFunc, w *assetWriter, r *http.Request) {
	defer w.finish()
	defer func() {
		if err := recover(); err != nil {
			if err == http.ErrAbortHandler {
				return
			}
			w.mu.Lock()
			fresh := w.status == 0
			w.mu.Unlock()
			if fresh {
				// As the darwin and iOS backends answer: the handler's
				// headers are dropped, the cause is in the body.
				w.mu.Lock()
				w.header = http.Header{"Content-Type": {"text/plain; charset=utf-8"}}
				w.mu.Unlock()
				w.WriteHeader(http.StatusInternalServerError)
				fmt.Fprintf(w, "tango: asset handler panic: %v", err)
			}
			logf(logError, "asset handler for %s panicked: %v", r.URL, err)
		}
	}()
	serve(w, r)
}

// closeStream ends a streamed response: WebView closed its InputStream.
func (res *assetResult) closeStream() {
	if res.Stream != nil {
		res.Stream.closeRead()
	}
	if res.cancel != nil {
		res.cancel()
	}
}

func errorResult(status int, msg string) assetResult {
	return assetResult{
		Status:   status,
		Reason:   reason(status),
		MIME:     "text/plain",
		Encoding: "utf-8",
		Body:     []byte(msg + "\n"),
		Length:   -1,
	}
}

// contentLength is h's Content-Length: a single non-negative decimal
// (repeated equal values allowed, as net/http does), else -1.
func contentLength(h http.Header) int64 {
	vs := h.Values("Content-Length")
	if len(vs) == 0 {
		return -1
	}
	n := int64(-1)
	for _, v := range vs {
		for _, f := range strings.Split(v, ",") {
			m, err := strconv.ParseInt(strings.TrimSpace(f), 10, 64)
			if err != nil || m < 0 || (n >= 0 && m != n) {
				return -1
			}
			n = m
		}
	}
	return n
}

// reason is the non-empty ASCII reason phrase WebResourceResponse requires.
func reason(status int) string {
	if s := http.StatusText(status); s != "" {
		return s
	}
	return "Status " + strconv.Itoa(status)
}

// splitContentType separates "text/html; charset=utf-8" into the MIME type
// and encoding WebResourceResponse takes.
func splitContentType(ct string) (mimeType, encoding string) {
	if ct == "" {
		return "application/octet-stream", ""
	}
	mt, params, err := mime.ParseMediaType(ct)
	if err != nil {
		mt, _, _ = strings.Cut(ct, ";")
		mt = strings.ToLower(strings.TrimSpace(mt))
	}
	if mt == "" {
		mt = "application/octet-stream"
	}
	return mt, params["charset"]
}

// flatHeader turns h into a name, value list with CR and LF removed from
// values. Java's header map (WebResourceResponse) holds one value per
// name, so repeated values are combined as RFC 9110 §5.3 allows: joined
// with ", ". Set-Cookie is the exception (its values may contain commas,
// RFC 6265 §3): its values are joined with "\n", which cannot occur in a
// cleaned value, and Page.java hands each one to CookieManager instead of
// passing the header to the WebView (which ignores cookies of intercepted
// responses).
func flatHeader(h http.Header) []string {
	clean := strings.NewReplacer("\r", " ", "\n", " ")
	out := make([]string, 0, 2*len(h))
	for k, vs := range h {
		if len(vs) == 0 {
			continue
		}
		sep := ", "
		if k == "Set-Cookie" {
			sep = "\n"
		}
		cl := make([]string, len(vs))
		for i, v := range vs {
			cl[i] = clean.Replace(v)
		}
		out = append(out, k, strings.Join(cl, sep))
	}
	return out
}

// originOf reduces a page URL to its origin, scheme://host[:port], or ""
// for URLs without one (about:blank, data:).
func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
}
