//go:build darwin && !ios

package darwin_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/tangoui/tango/automation"
)

// probeResult is window.__probe of examples/duplex/frontend/probe.html.
type probeResult struct {
	Status int    `json:"status"`
	Head   int    `json:"head"`
	Bytes  int    `json:"bytes"`
	Error  string `json:"error"`
	Chunks []struct {
		T int `json:"t"` // ms since fetch() was called
		N int `json:"n"` // bytes
	} `json:"chunks"`
}

// TestStreamingDeliveryProbe measures whether WKWebView hands a
// WKURLSchemeTask's separate didReceiveData: calls to the page's fetch()
// ReadableStream as they happen, or only once the task finished. The
// backend's TANGO_SCHEME_PROBE hook (scheme_probe_dev.go, development
// builds only; automation builds the app with -tags tango_dev) sends 8
// chunks of 1 KiB, 200 ms apart; the page records when each one arrives.
// Incremental delivery holds when at least 4 chunks arrived (WebKit may
// merge neighbours) and the first arrived more than 1000 ms before the
// last.
//
//	TANGO_E2E=1 go test -timeout 120s ./internal/platform/darwin -run StreamingDeliveryProbe -v
func TestStreamingDeliveryProbe(t *testing.T) {
	app := automation.LaunchOptions(t, duplexApp(t), automation.Options{
		Args:    []string{"-page", "probe.html"},
		Env:     []string{"TANGO_SCHEME_PROBE=1"},
		Timeout: 60 * time.Second,
	})
	win := app.Window("Tango Duplex")
	win.WaitJS(`window.__probe && window.__probe.done`)
	var p probeResult
	win.EvalInto(`window.__probe`, &p)
	raw, _ := json.Marshal(p)
	t.Logf("probe (macOS): %s", raw)
	app.Quit()

	if p.Error != "" || p.Status != 200 {
		t.Fatalf("probe fetch: status %d, error %q", p.Status, p.Error)
	}
	if p.Bytes != 8<<10 {
		t.Fatalf("probe got %d bytes, want %d", p.Bytes, 8<<10)
	}
	first, last := p.Chunks[0].T, p.Chunks[len(p.Chunks)-1].T
	t.Logf("chunks %d, first at %d ms, last at %d ms, head at %d ms", len(p.Chunks), first, last, p.Head)
	if len(p.Chunks) < 4 || first >= last-1000 {
		t.Fatalf("not delivered incrementally: %d chunks, first %d ms, last %d ms", len(p.Chunks), first, last)
	}
}
