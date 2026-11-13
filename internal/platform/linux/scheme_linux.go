//go:build linux && !android

// The tango:// custom URI scheme on WebKitGTK.
//
// WebKit hands every request for the scheme to one callback on the main
// thread (WebKitURISchemeRequestCallback). The request is turned into an
// *http.Request and answered by Host.ServeAsset in one of two ways:
//
//   - WebKitGTK 2.36+: streamed from a goroutine through a pipe
//     (schemestream_linux.go);
//   - otherwise: buffered on the main thread and handed back as one
//     GMemoryInputStream (finishScheme below). Releases without
//     webkit_uri_scheme_request_finish_with_response can only report a
//     body and a content type, not a status or headers.

package linux

import (
	"slices"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

// SOUP_MESSAGE_HEADERS_RESPONSE.
const soupMessageHeadersResponse = 1

// schemeErrorCode is the code of the GError a failed request finishes
// with (its domain is the quark "tango-asset").
const schemeErrorCode = 1

// registerScheme installs tango:// on the default WebKitWebContext, which
// every web view of the process shares. The scheme is marked secure (pages
// from it are a secure context) and CORS-enabled (they may fetch() it).
func (b *backend) registerScheme() {
	wctx := webkitWebContextGetDefault()
	onRequest := dispatch.add(func(req, _ ptr) ptr {
		b.serveScheme(req)
		return 0
	})
	// No destroy notify: the context, and so the handler, lives until exit.
	webkitWebContextRegisterURIScheme(wctx, assetScheme, cbArg2, onRequest, 0)

	mgr := webkitWebContextGetSecurityManager(wctx)
	for _, mark := range []cSetStr{
		webkitSecurityManagerRegisterSecure,
		webkitSecurityManagerRegisterCORS,
	} {
		mark(mgr, assetScheme)
	}
}

// ifBound calls getter on req when this WebKitGTK has it; 0 otherwise.
func ifBound(getter cGet, req ptr) ptr {
	if getter == nil {
		return 0
	}
	return getter(req)
}

// serveScheme answers one WebKitURISchemeRequest (main thread).
func (b *backend) serveScheme(req ptr) {
	var sid platform.SurfaceID
	var s *surface
	if s = b.byWeb[webkitURISchemeRequestGetWebView(req)]; s != nil {
		sid = s.id
	}

	// Method, headers and body are only exposed by newer WebKitGTK; an
	// empty method means GET to newAssetRequest.
	method := goStr(ifBound(webkitURISchemeRequestGetHTTPMethod, req))
	var headers [][2]string
	if h := ifBound(webkitURISchemeRequestGetHTTPHeaders, req); h != 0 {
		headers = soupHeaders(h)
	}
	// WebKitGTK sends no Origin on custom-scheme requests, so the core's
	// cross-origin check on state-changing requests would always pass.
	// Name the document that sent it, as the macOS backend does.
	headers = withRequestOrigin(headers, method, s)
	var body []byte
	if in := ifBound(webkitURISchemeRequestGetHTTPBody, req); in != 0 {
		body = readStream(in) // transfer full: we own the stream
		gObjectUnref(in)
	}

	r, err := newAssetRequest(method, goStr(webkitURISchemeRequestGetURI(req)), headers, body)
	if err != nil {
		finishSchemeError(req, err.Error())
		return
	}
	if b.streamScheme(req, sid, r) {
		return
	}
	w := newAssetWriter()
	b.host.ServeAsset(sid, w, r)
	finishScheme(req, w.finish(r.Method))
}

// soupHeaders lists the name/value pairs of a SoupMessageHeaders. The
// foreach callback is the shared 2-argument trampoline, routed to a
// dispatch entry that exists only for this call.
func soupHeaders(h ptr) [][2]string {
	var pairs [][2]string
	visit := dispatch.add(func(name, value ptr) ptr {
		pairs = append(pairs, [2]string{goStr(name), goStr(value)})
		return 0
	})
	defer dispatch.remove(visit)
	soupMessageHeadersForeach(h, cbArg3, visit)
	return pairs
}

// readChunk is how much readStream asks g_input_stream_read_all for at a
// time.
const readChunk = 64 << 10

// readStream drains a GInputStream. g_input_stream_read_all only returns
// fewer bytes than asked at end of stream or on error, so a short read ends
// the loop. Errors are dropped: what arrived before one is still returned.
func readStream(in ptr) []byte {
	var data []byte
	for {
		data = slices.Grow(data, readChunk)
		room := data[len(data):cap(data)]
		var got uintptr
		var gerr ptr
		full := gInputStreamReadAll(in, unsafe.Pointer(unsafe.SliceData(room)), uintptr(len(room)), &got, 0, &gerr)
		data = data[:len(data)+int(got)]
		if gerr != 0 {
			gErrorFree(gerr)
		}
		if !full || int(got) != len(room) {
			return data
		}
	}
}

// finishScheme answers req with a fully buffered response.
func finishScheme(req ptr, resp assetResponse) {
	// g_bytes_new copies, so resp.Body may be collected afterwards.
	var src unsafe.Pointer
	if len(resp.Body) != 0 {
		src = unsafe.Pointer(unsafe.SliceData(resp.Body))
	}
	gb := gBytesNew(src, uintptr(len(resp.Body)))
	in := gMemoryInputStreamNewFromBytes(gb) // holds its own ref on gb
	gBytesUnref(gb)
	defer gObjectUnref(in)
	size := int64(len(resp.Body))

	if webkitURISchemeRequestFinishWithResponse == nil {
		webkitURISchemeRequestFinish(req, in, size, cstr(resp.ContentType))
		return
	}

	out := webkitURISchemeResponseNew(in, size)
	defer gObjectUnref(out)
	webkitURISchemeResponseSetStatus(out, uint32(resp.Status), cstr(resp.Reason))
	if resp.ContentType != "" {
		webkitURISchemeResponseSetContentType(out, resp.ContentType)
	}
	hdrs := soupMessageHeadersNew(soupMessageHeadersResponse)
	for _, kv := range resp.Headers {
		soupMessageHeadersAppend(hdrs, kv[0], kv[1])
	}
	webkitURISchemeResponseSetHTTPHeaders(out, hdrs) // the response now owns hdrs
	webkitURISchemeRequestFinishWithResponse(req, out)
}

// finishSchemeError fails req with msg.
func finishSchemeError(req ptr, msg string) {
	gerr := gErrorNewLiteral(gQuarkFromString("tango-asset"), schemeErrorCode, msg)
	defer gErrorFree(gerr)
	webkitURISchemeRequestFinishError(req, gerr)
}

// withRequestOrigin adds the sending document's origin to a state-changing
// request without one (msggate.go).
func withRequestOrigin(h [][2]string, method string, s *surface) [][2]string {
	origin := ""
	if s != nil {
		origin = s.gate.requestOrigin()
	}
	return addRequestOrigin(h, method, origin)
}
