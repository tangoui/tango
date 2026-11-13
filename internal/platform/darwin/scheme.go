//go:build darwin && !ios

package darwin

// WKURLSchemeHandler adapter for the app's asset scheme.
//
// WebKit hands the handler a WKURLSchemeTask per request (see Apple's
// WKURLSchemeHandler / WKURLSchemeTask reference). The task is answered with
// exactly one didReceiveResponse:, any number of didReceiveData:, and one
// didFinish — and must not be touched at all once WebKit has sent
// webView:stopURLSchemeTask:. Everything here runs on the main thread, which is
// where WebKit calls the handler. The http.Handler plumbing itself is in
// asset.go.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
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
		withPool(func() { serveSchemeTask(webView, task) })
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
// assetWriter.
type schemeTask struct {
	native id // retained WKURLSchemeTask; zero once closed
	w      *assetWriter
	cancel context.CancelFunc
}

func (t *schemeTask) open() bool { return t.native != 0 }

// close drops our reference and forgets the task. Safe to call repeatedly.
func (t *schemeTask) close() {
	if !t.open() {
		return
	}
	native := t.native
	t.native = 0
	t.cancel()
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

func serveSchemeTask(webView, native id) {
	ctx, cancel := context.WithCancel(context.Background())
	t := &schemeTask{native: retain(native), cancel: cancel}
	assetBridge.inFlight[native] = t
	t.w = newAssetWriter(t, "")
	defer t.close()

	req, err := buildRequest(ctx, requestFromNative(send(native, "request")))
	if err != nil {
		t.w.WriteHeader(http.StatusBadRequest)
		t.w.finish()
		return
	}
	t.w.path = req.URL.Path

	b := theBackend
	var sid platform.SurfaceID
	if s, ok := b.byWebView[webView]; ok && s != nil {
		sid = s.id
	}
	if crash := callAssetHandler(b.h, sid, t.w, req); crash != nil {
		t.w.replaceWithError(crash)
	}
	t.w.finish()
}

// callAssetHandler runs the app's handler and converts a panic into a value,
// so one bad asset cannot take down the UI thread.
func callAssetHandler(h platform.Host, sid platform.SurfaceID, w *assetWriter, req *http.Request) (crash any) {
	defer func() { crash = recover() }()
	h.ServeAsset(sid, w, req)
	return nil
}

// replaceWithError discards whatever the handler buffered and turns the
// response into a 500, unless headers already reached WebKit (then the
// truncated body is all we can do).
func (w *assetWriter) replaceWithError(cause any) {
	if w.sent {
		return
	}
	w.header, w.status = http.Header{}, 0
	w.buf.Reset()
	w.WriteHeader(http.StatusInternalServerError)
	fmt.Fprintf(w, "tango: asset handler panic: %v", cause)
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
		out.Body = goBytes(body)
	case stream != 0:
		out.Body = drainInputStream(stream)
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

func drainInputStream(stream id) []byte {
	send(stream, "open")
	body, _ := io.ReadAll(nsInputStream{stream}) // keep what arrived before an error
	send(stream, "close")
	return body
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
}

func (t *schemeTask) finish() {
	if !t.open() {
		return
	}
	send(t.native, "didFinish")
	t.close()
}
