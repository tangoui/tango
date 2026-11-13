package linux

import (
	"bytes"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// assetScheme and assetOrigin: pages are served from tango://app/...
const (
	assetScheme = "tango"
	assetOrigin = "tango://app"
)

// newAssetRequest builds the *http.Request handed to Host.ServeAsset from
// what WebKitGTK reports about a custom-scheme request. method "" means GET
// (WebKitGTK before 2.36 cannot report it).
func newAssetRequest(method, uri string, headers [][2]string, body []byte) (*http.Request, error) {
	if method == "" {
		method = http.MethodGet
	}
	var rd io.Reader
	if len(body) > 0 {
		rd = bytes.NewReader(body)
	}
	r, err := http.NewRequest(method, uri, rd)
	if err != nil {
		return nil, err
	}
	for _, kv := range headers {
		if http.CanonicalHeaderKey(kv[0]) == "Host" {
			continue
		}
		r.Header.Add(kv[0], kv[1])
	}
	if r.Body == nil {
		r.Body = http.NoBody // as for server requests
	}
	r.RequestURI = r.URL.RequestURI()
	r.RemoteAddr = "webkit"
	return r, nil
}

// assetWriter is the http.ResponseWriter handed to Host.ServeAsset. The
// contract calls ServeAsset synchronously on the main thread, so the
// response is buffered and given to WebKit as one GMemoryInputStream once
// the handler returns.
type assetWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newAssetWriter() *assetWriter { return &assetWriter{header: http.Header{}} }

func (w *assetWriter) Header() http.Header { return w.header }

func (w *assetWriter) WriteHeader(code int) {
	if w.status != 0 || code < 100 || code > 999 {
		return
	}
	if code < 200 { // informational responses are not forwarded
		return
	}
	w.status = code
}

func (w *assetWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.body.Write(p)
}

// Flush satisfies http.Flusher; buffered, so a no-op.
func (w *assetWriter) Flush() {}

// assetResponse is the finished response in the shape WebKit wants.
type assetResponse struct {
	Status      int
	Reason      string
	ContentType string
	Headers     [][2]string // sorted by name, values in order; no Content-Type
	Body        []byte
}

// finish completes the response the way net/http would: status 200 if
// unset, Content-Type sniffed when the handler set none, Content-Length set
// from the body.
func (w *assetWriter) finish(method string) assetResponse {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	h := w.header.Clone()
	body := w.body.Bytes()
	ct := h.Get("Content-Type")
	if _, set := h["Content-Type"]; !set && len(body) > 0 {
		ct = http.DetectContentType(body)
	}
	h.Del("Content-Type")
	if len(body) > 0 || method != http.MethodHead {
		h.Set("Content-Length", strconv.Itoa(len(body)))
	}
	return assetResponse{
		Status:      w.status,
		Reason:      http.StatusText(w.status),
		ContentType: ct,
		Headers:     headerPairs(h),
		Body:        body,
	}
}

// headerPairs flattens h into name/value pairs sorted by name, keeping the
// order of repeated values.
func headerPairs(h http.Header) [][2]string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var out [][2]string
	for _, k := range keys {
		for _, v := range h[k] {
			out = append(out, [2]string{k, v})
		}
	}
	return out
}

// originOf returns scheme://host[:port] of uri, lowercased scheme/host, or
// "" for URIs without an authority (about:blank, data:).
func originOf(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
}
