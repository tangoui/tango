package linux

// Pure logic of the streaming custom-scheme response (scheme_linux.go,
// schemestream_linux.go): what WebKit gets as status line and headers
// before the body streams. No build tag: tested on every host.

import (
	"net/http"
	"strconv"
)

// streamHead is the response head of a streamed asset response, decided
// when the handler first writes, flushes or returns: status 200 if unset,
// Content-Type sniffed from the first chunk when the handler set none (as
// net/http does), and the body length — the handler's Content-Length if it
// set a valid one, else -1 (unknown: WebKit reads until EOF). done reports
// that the handler already returned, so first is the whole body and its
// length is known.
func streamHead(h http.Header, status int, first []byte, method string, done bool) (assetResponse, int64) {
	if status == 0 {
		status = http.StatusOK
	}
	h = h.Clone()
	ct := h.Get("Content-Type")
	if _, set := h["Content-Type"]; !set && len(first) > 0 {
		ct = http.DetectContentType(first)
	}
	h.Del("Content-Type")
	length := int64(-1)
	if v := h.Get("Content-Length"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			length = n
		} else {
			h.Del("Content-Length")
		}
	}
	if done && length < 0 {
		length = int64(len(first))
		if len(first) > 0 || method != http.MethodHead {
			h.Set("Content-Length", strconv.Itoa(len(first)))
		}
	}
	if method == http.MethodHead || status == http.StatusNoContent || status == http.StatusNotModified {
		length = 0
	}
	return assetResponse{
		Status:      status,
		Reason:      http.StatusText(status),
		ContentType: ct,
		Headers:     headerPairs(h),
	}, length
}
