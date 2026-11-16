package asset

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// Includes response recorder allocation and body copying; this is an HTTP
// handler workload, not an isolated filesystem read or a WebView benchmark.
func BenchmarkEmbeddedAssetResponse(b *testing.B) {
	data := []byte(strings.Repeat("console.log('asset');\n", 2048))
	h := FS(fstest.MapFS{"app.js": {Data: data}}, Options{})
	req := httptest.NewRequest(http.MethodGet, "http://tango.localhost/app.js", nil)
	b.SetBytes(int64(len(data)))
	b.ReportAllocs()
	for b.Loop() {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != 200 || w.Body.Len() != len(data) {
			b.Fatal("bad response")
		}
	}
}
