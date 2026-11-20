package services

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"net/http"

	"github.com/tangoui/tango/ipc"
)

//go:embed selftest.js
var selftestJS []byte

// SelftestRoute serves the script shared by the two duplex examples.
func SelftestRoute(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/selftest.js" || (r.Method != http.MethodGet && r.Method != http.MethodHead) {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(selftestJS)
	})
}

// RegisterSelftestReport forwards the page's result as compact JSON.
func RegisterSelftestReport(r *ipc.Router, report func(kind, result string)) {
	r.Handle("Selftest.Report", func(_ context.Context, args json.RawMessage) (any, error) {
		var a []json.RawMessage
		var kind string
		if err := json.Unmarshal(args, &a); err != nil || len(a) != 2 || json.Unmarshal(a[0], &kind) != nil {
			return nil, ipc.NewError(ipc.CodeBadArgs, "Selftest.Report(kind string, result object)")
		}
		var line bytes.Buffer
		if err := json.Compact(&line, a[1]); err != nil {
			return nil, ipc.NewError(ipc.CodeBadArgs, err.Error())
		}
		report(kind, line.String())
		return nil, nil
	})
}
