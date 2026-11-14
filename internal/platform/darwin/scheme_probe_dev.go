//go:build darwin && !ios && tango_dev

package darwin

import (
	"net/http"
	"os"
	"sync"
	"time"
)

// Streaming probe (test hook, development builds only: -tags tango_dev;
// scheme_probe_release.go leaves it out). With TANGO_SCHEME_PROBE=1 in the
// environment, GET /__tango/probe/stream is answered here, without
// Host.ServeAsset: the response head and a first 1 KiB chunk at once, then
// one more 1 KiB chunk every probeInterval (probeChunks in all), then
// didFinish. A page that records when each chunk reaches its fetch()
// ReadableStream shows whether WebKit delivers a scheme task's
// didReceiveData: calls incrementally (examples/duplex/frontend/probe.html,
// streamprobe_test.go). Without the variable nothing here runs.
const (
	probePath     = "/__tango/probe/stream"
	probeChunks   = 8
	probeChunk    = 1 << 10
	probeInterval = 200 * time.Millisecond
)

var schemeProbe = sync.OnceValue(func() bool { return os.Getenv("TANGO_SCHEME_PROBE") == "1" })

// serveProbe answers native if it asks for probePath and reports whether it
// did. Main thread. The chunks are sent from the main thread too (a Go
// timer hands each one to runOnMain); once WebKit stopped the task
// (abandon closes it) nothing more is sent.
func serveProbe(native id) bool {
	if goString(send(send(send(native, "request"), "URL"), "path")) != probePath {
		return false
	}
	t := &schemeTask{native: retain(native), cancel: func() {}}
	assetBridge.inFlight[native] = t
	t.respond(http.StatusOK, http.Header{
		"Content-Type":           {"application/octet-stream"},
		"Cache-Control":          {"no-store"},
		"X-Content-Type-Options": {"nosniff"},
	})
	chunk := make([]byte, probeChunk)
	for i := range chunk {
		chunk[i] = byte(i)
	}
	sent := 0
	var step func()
	step = func() {
		if !t.open() {
			return // stopped by WebKit
		}
		t.data(chunk)
		if sent++; sent == probeChunks {
			t.finish()
			return
		}
		time.AfterFunc(probeInterval, func() { runOnMain(step) })
	}
	step()
	return true
}
