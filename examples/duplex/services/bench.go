package services

import (
	_ "embed"
	"net/http"
)

// benchJS is the benchmark page script (window.duplexBench).
//
//go:embed bench.js
var benchJS []byte

// BenchRoute returns next with GET /bench.js, the benchmark script that
// examples/duplex and examples/duplex-mobile share, in front of it.
func BenchRoute(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/bench.js" || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(benchJS)
	})
}
