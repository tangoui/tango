package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync"

	"github.com/tangoui/tango/examples/duplex/services"
	"github.com/tangoui/tango/ipc"
)

//go:embed selftest.html
var selftestHTML []byte

func selftestAssets(next http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/", services.SelftestRoute(next))
	mux.HandleFunc("GET /index.html", func(w http.ResponseWriter, r *http.Request) {
		if !r.URL.Query().Has("selftest") {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(selftestHTML)
	})
	return mux
}

func registerSelftest(r *ipc.Router, quit func(), fault string) {
	r.Handle("Debug.Platform", func(context.Context, json.RawMessage) (any, error) {
		return map[string]string{"goos": runtime.GOOS, "fault": fault}, nil
	})
	var once sync.Once
	services.RegisterSelftestReport(r, func(_, result string) {
		once.Do(func() {
			fmt.Println("selftest: " + result)
			quit()
		})
	})
}
