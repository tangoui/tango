package windows

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// Adapting WebView2's WebResourceRequested to net/http.

// newAssetRequest builds the server-side request handed to Host.ServeAsset.
func newAssetRequest(method, uri string, header http.Header, body []byte) (*http.Request, error) {
	if method == "" {
		method = http.MethodGet
	}
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("tango: bad asset URL %q: %w", uri, err)
	}
	r, err := http.NewRequest(method, uri, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if header == nil {
		header = http.Header{}
	}
	r.Header = header
	r.Host = u.Host
	r.RequestURI = u.RequestURI()
	r.ContentLength = int64(len(body))
	if len(body) == 0 {
		r.Body = http.NoBody
	}
	r.Proto, r.ProtoMajor, r.ProtoMinor = "HTTP/1.1", 1, 1
	r.RemoteAddr = "127.0.0.1:0"
	return r, nil
}

// parseHeaderBlock parses "Name: value\r\n" lines.
func parseHeaderBlock(s string) http.Header {
	h := http.Header{}
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimRight(line, "\r")
		name, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(name) == "" {
			continue
		}
		h.Add(strings.TrimSpace(name), strings.TrimSpace(value))
	}
	return h
}

// headerBlock serializes h as WebView2's CreateWebResourceResponse expects:
// "Name: value" lines separated by CRLF, in sorted order. CR and LF inside
// values are replaced so a handler cannot inject headers.
func headerBlock(h http.Header) string {
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	clean := strings.NewReplacer("\r", " ", "\n", " ")
	var b strings.Builder
	for _, k := range keys {
		for _, v := range h[k] {
			if b.Len() > 0 {
				b.WriteString("\r\n")
			}
			b.WriteString(clean.Replace(k))
			b.WriteString(": ")
			b.WriteString(clean.Replace(v))
		}
	}
	return b.String()
}

// assetResponse is the http.ResponseWriter given to Host.ServeAsset. The
// body is buffered and handed to WebView2 as one IStream once the handler
// returns.
type assetResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func newAssetResponse() *assetResponse { return &assetResponse{header: http.Header{}} }

func (r *assetResponse) Header() http.Header { return r.header }

func (r *assetResponse) WriteHeader(code int) {
	if r.status != 0 {
		return
	}
	if code < 100 || code > 999 {
		code = http.StatusInternalServerError
	}
	r.status = code
}

func (r *assetResponse) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.WriteHeader(http.StatusOK)
	}
	return r.body.Write(p)
}

// Flush is a no-op: the body is delivered whole.
func (r *assetResponse) Flush() {}

// finish returns what to pass to CreateWebResourceResponse. It mirrors
// net/http: status 200 by default and a sniffed Content-Type when the
// handler set none (an explicit nil Content-Type disables sniffing).
func (r *assetResponse) finish() (status int, reason, headers string, body []byte) {
	status = r.status
	if status == 0 {
		status = http.StatusOK
	}
	body = r.body.Bytes()
	h := r.header.Clone()
	if h == nil {
		h = http.Header{}
	}
	if _, set := h["Content-Type"]; !set && len(body) > 0 {
		h.Set("Content-Type", http.DetectContentType(body))
	}
	if v, ok := h["Content-Type"]; ok && len(v) == 0 {
		delete(h, "Content-Type")
	}
	if h.Get("Content-Length") == "" && status != http.StatusNoContent && status != http.StatusNotModified {
		h.Set("Content-Length", strconv.Itoa(len(body)))
	}
	reason = http.StatusText(status)
	if reason == "" {
		reason = "Status " + strconv.Itoa(status)
	}
	return status, reason, headerBlock(h), body
}

// originOf reduces a document URL to its origin, scheme://host[:port], or
// "null" for URLs without one (about:blank, data:).
func originOf(uri string) string {
	u, err := url.Parse(uri)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "null"
	}
	return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host)
}
