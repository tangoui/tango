//go:build darwin && !ios

package darwin

// WKURLSchemeHandler adapter for the app's asset scheme.
//
// WebKit hands the handler a WKURLSchemeTask per request (see Apple's
// WKURLSchemeHandler / WKURLSchemeTask reference). The task is answered with
// exactly one didReceiveResponse:, any number of didReceiveData:, and one
// didFinish — and must not be touched at all once WebKit has sent
// webView:stopURLSchemeTask:. WebKit calls the handler on the main thread,
// and every WKURLSchemeTask call is made there too. Host.ServeAsset runs on
// its own goroutine (the platform contract allows any thread and prefers
// one off the main thread), so slow and long-lived responses never block
// the UI; its writer (assetWriter, asset.go) hands the response to the
// main thread in order, with backpressure, and is cancelled by
// webView:stopURLSchemeTask:.

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unsafe"

	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango/internal/platform"
)

// assetBridge owns the handler instance and the tasks currently in flight,
// keyed by the WKURLSchemeTask pointer WebKit gave us.
var assetBridge = struct {
	handler  id
	inFlight map[id]*schemeTask
}{inFlight: make(map[id]*schemeTask)}

// schemeHandler returns the shared handler object, creating it on first use
// (registerSchemeHandler must already have run).
func schemeHandler() id {
	if assetBridge.handler == 0 {
		assetBridge.handler = alloc("TangoSchemeHandler")
	}
	return assetBridge.handler
}

func registerSchemeHandler() {
	start := func(_ id, _ objc.SEL, webView, task id) {
		withPool(func() {
			if schemeProbe() && serveProbe(task) {
				return
			}
			serveSchemeTask(webView, task)
		})
	}
	stop := func(_ id, _ objc.SEL, _, task id) {
		if t, ok := assetBridge.inFlight[task]; ok {
			t.abandon()
		}
	}
	classDef("TangoSchemeHandler", "NSObject", []string{"WKURLSchemeHandler"}, []objc.MethodDef{
		method("webView:startURLSchemeTask:", start),
		method("webView:stopURLSchemeTask:", stop),
	})
}

// schemeTask adapts one WKURLSchemeTask to the taskSink interface used by
// assetWriter. Main thread only.
type schemeTask struct {
	native     id // retained WKURLSchemeTask; zero once closed
	w          *assetWriter
	cancel     context.CancelFunc
	bodyStream id         // retained while the request body is drained off-main
	lane       *laneTrack // lane request counters (test hook, lanes_dev.go); nil in production builds
}

func (t *schemeTask) open() bool { return t.native != 0 }

// close drops our reference and forgets the task. Safe to call repeatedly.
func (t *schemeTask) close() {
	if !t.open() {
		return
	}
	native := t.native
	t.native = 0
	t.lane.done()
	t.cancel()
	if t.bodyStream != 0 {
		send(t.bodyStream, "close")
		release(t.bodyStream)
		t.bodyStream = 0
	}
	delete(assetBridge.inFlight, native)
	release(native)
}

// abandon handles webView:stopURLSchemeTask:. WebKit forbids further calls on
// the task, so the writer is muted before the reference is dropped.
func (t *schemeTask) abandon() {
	if t.w != nil {
		t.w.stop()
	}
	t.close()
}

// serveSchemeTask starts serving one task. Main thread: it retains the task,
// builds the *http.Request (reading the body) and registers the task in
// assetBridge.inFlight, then runs Host.ServeAsset on a goroutine. The writer
// queues the WKURLSchemeTask calls for the main thread (postMain); the
// final didFinish closes the task.
func serveSchemeTask(webView, native id) {
	var start time.Time // the up:post main-thread cost (test hook only)
	if schemeProbe() {
		start = time.Now()
	}
	ctx, cancel := context.WithCancel(context.Background())
	t := &schemeTask{native: retain(native), cancel: cancel}
	assetBridge.inFlight[native] = t
	t.w = newAssetWriter(ctx, t, postMain, "")

	nr := requestFromNative(send(native, "request"))
	if nr.BodyTooLarge {
		t.w.WriteHeader(http.StatusRequestEntityTooLarge)
		t.w.finish()
		return
	}
	stream := nr.BodyStream
	nr.BodyStream = 0 // ownership moves to the scheme task below
	req, err := buildRequest(ctx, nr)
	if err != nil {
		t.w.WriteHeader(http.StatusBadRequest)
		t.w.finish()
		return
	}
	t.w.path = req.URL.Path // before the handler goroutine starts
	t.lane = laneTrackFor(req.URL.Path)
	if t.lane != nil {
		t.lane.mainTime(time.Since(start), req.ContentLength)
	}

	b := theBackend
	var sid platform.SurfaceID
	if s, ok := b.byWebView[webView]; ok && s != nil {
		sid = s.id
	}
	h, w := b.h, t.w
	if stream == 0 {
		go runAssetHandler(w, func() { h.ServeAsset(sid, w, req) })
		return
	}
	// Retain the stream once for the task (so stop can close a blocked read)
	// and once for the worker. No body bytes are read on WebKit's main thread.
	t.bodyStream = retain(stream)
	go func(s id) {
		defer release(s)
		body, err := drainInputStreamLimit(s, maxAssetRequestBody)
		if err != nil {
			if errors.Is(err, errAssetRequestBodyTooLarge) {
				w.WriteHeader(http.StatusRequestEntityTooLarge)
			} else {
				w.WriteHeader(http.StatusBadRequest)
			}
			w.finish()
			return
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
		runAssetHandler(w, func() { h.ServeAsset(sid, w, req) })
	}(retain(stream))
}

// requestFromNative copies the parts of an NSURLRequest the Go side needs.
func requestFromNative(r id) nativeRequest {
	out := nativeRequest{
		Method: goString(send(r, "HTTPMethod")),
		URL:    goString(send(send(r, "URL"), "absoluteString")),
	}
	if dict := send(r, "allHTTPHeaderFields"); dict != 0 {
		keys := send(dict, "keyEnumerator")
		for k := send(keys, "nextObject"); k != 0; k = send(keys, "nextObject") {
			v := send(dict, "objectForKey:", uintptr(k))
			out.Header = append(out.Header, [2]string{goString(k), goString(v)})
		}
	}
	// WebKit sends no Origin header on custom-scheme requests, not even
	// cross-origin ones. The core's CSRF check (package tango,
	// asset_origin.go) needs one: use the main document's origin, which a
	// page cannot forge.
	if doc := send(r, "mainDocumentURL"); doc != 0 {
		out.Header = withOrigin(out.Header, goString(send(doc, "absoluteString")))
	}
	// A body is delivered either inline (HTTPBody) or, for fetch() with a
	// streamed or large body, as an NSInputStream.
	switch body, stream := send(r, "HTTPBody"), send(r, "HTTPBodyStream"); {
	case body != 0:
		if int64(sendInt(body, "length")) > maxAssetRequestBody {
			out.BodyTooLarge = true
		} else {
			out.Body = goBytes(body)
		}
	case stream != 0:
		out.BodyStream = stream
	}
	return out
}

// nsInputStream is an io.Reader over an opened NSInputStream.
type nsInputStream struct{ s id }

func (r nsInputStream) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	// -read:maxLength: returns bytes read, 0 at end of stream, -1 on error.
	switch n := sendInt(r.s, "read:maxLength:", uintptr(unsafe.Pointer(unsafe.SliceData(p))), uintptr(len(p))); {
	case n > 0:
		return n, nil
	case n == 0:
		return 0, io.EOF
	default:
		return 0, errors.New("NSInputStream read failed")
	}
}

const maxAssetRequestBody = 64 << 20

var errAssetRequestBodyTooLarge = errors.New("tango: asset request body too large")

func drainInputStreamLimit(stream id, max int64) ([]byte, error) {
	send(stream, "open")
	body, err := readAssetBody(nsInputStream{stream}, max)
	send(stream, "close")
	return body, err
}

func readAssetBody(r io.Reader, max int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return body, err
	}
	if int64(len(body)) > max {
		return body[:max], errAssetRequestBodyTooLarge
	}
	return body, nil
}

// respond sends the response head. WebKit wants an NSHTTPURLResponse for the
// status line and headers; repeated header values are folded with ", " as
// RFC 9110 allows.
func (t *schemeTask) respond(status int, header http.Header) {
	if !t.open() {
		return
	}
	withPool(func() {
		names := make([]id, 0, len(header))
		values := make([]id, 0, len(header))
		for name, vals := range header {
			names = append(names, nsString(name))
			values = append(values, nsString(strings.Join(vals, ", ")))
		}
		fields := headerDictionary(names, values)
		reqURL := send(send(t.native, "request"), "URL")
		noteAttachment(reqURL, header) // webview_ext.go
		head := send(class("NSHTTPURLResponse"), "alloc")
		head = send(head, "initWithURL:statusCode:HTTPVersion:headerFields:",
			uintptr(reqURL), uintptr(status), uintptr(nsString("HTTP/1.1")), uintptr(fields))
		send(t.native, "didReceiveResponse:", uintptr(head))
		release(head)
	})
	t.lane.respond(status)
}

// headerDictionary builds an NSDictionary from parallel key / value arrays.
func headerDictionary(keys, values []id) id {
	if len(keys) == 0 {
		return send(class("NSDictionary"), "dictionary")
	}
	return send(class("NSDictionary"), "dictionaryWithObjects:forKeys:count:",
		uintptr(unsafe.Pointer(&values[0])), uintptr(unsafe.Pointer(&keys[0])), uintptr(len(keys)))
}

func (t *schemeTask) data(p []byte) {
	if !t.open() || len(p) == 0 {
		return
	}
	withPool(func() {
		chunk := nsData(p)
		send(t.native, "didReceiveData:", uintptr(chunk))
	})
	t.lane.data(len(p))
}

// finish ends the response and closes the task: the writer's last action.
func (t *schemeTask) finish() {
	if !t.open() {
		return
	}
	send(t.native, "didFinish")
	t.close()
}
