package services

// Test hooks of the duplex examples, active only with TANGO_SCHEME_PROBE=1
// in the environment (the platform tests set it; nothing here exists in a
// normal run):
//
//   - TestRoutes wraps the asset handler with /__test/... routes that the
//     scheme-handler tests of internal/platform/darwin (and the iOS
//     self-test) fetch.
//   - Register adds the ipc method "Debug.Stats", which reports Go-side
//     counters the page (and through it the automation test) cannot see;
//     the examples feed some of them through CountMessage and CountUp.

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tangoui/tango/ipc"
)

// Probe reports whether the test hooks are on (TANGO_SCHEME_PROBE=1).
var Probe = sync.OnceValue(func() bool { return os.Getenv("TANGO_SCHEME_PROBE") == "1" })

// Stats is the result of Debug.Stats. Later counters are added as fields;
// the page reads them by name.
type Stats struct {
	// Goroutines is runtime.NumGoroutine() at the time of the call.
	Goroutines int `json:"goroutines"`
	// HandlerExits counts returns of the /__test/stream handler (the
	// request context ended, i.e. the backend cancelled the request).
	HandlerExits int64 `json:"handlerExits"`

	// Hellos counts runtime hellos that reached the app's OnMessage
	// (CountMessage).
	Hellos int64 `json:"hellos"`
	// UpPackets, UpFrames and UpBytes count the fast lane packets that
	// reached the app's OnBinary, their frames and the frames' payload bytes
	// (CountUp).
	UpPackets int64 `json:"upPackets"`
	UpFrames  int64 `json:"upFrames"`
	UpBytes   int64 `json:"upBytes"`

	// The backend's scheme handler counters (darwin.LaneDownStats,
	// ios.LaneDownStats; zero on other platforms, and on macOS in builds
	// without -tags tango_dev): lane requests it saw and
	// how they were answered.
	DownRequests int64 `json:"downRequests"` // GET /__tango/lane/poll (macOS; /__tango/lane/down on iOS)
	DownConns    int64 `json:"downConns"`    // of them answered 200
	DownActive   int64 `json:"downActive"`   // answered 200 and still open
	DownWrites   int64 `json:"downWrites"`   // data blocks handed to WebKit on those answered 200
	DownRefused  int64 `json:"downRefused"`  // of them answered 403 (macOS only)
	DownOpen     int64 `json:"downOpen"`     // of them not finished yet, held ones included (macOS only)
	UpRequests   int64 `json:"upRequests"`   // POST /__tango/lane/up
	UpAccepted   int64 `json:"upAccepted"`   // of them answered 2xx
	UpActive     int64 `json:"upActive"`     // started and not yet finished or stopped
	// Main-thread time the macOS scheme handler spent on up requests
	// before their handler goroutine started (summed, largest), and their
	// body bytes; zero on iOS and elsewhere.
	UpMainNanos    int64 `json:"upMainNanos"`
	UpMainMaxNanos int64 `json:"upMainMaxNanos"`
	UpBodyBytes    int64 `json:"upBodyBytes"`
}

var (
	handlerExits                 atomic.Int64
	hellos                       atomic.Int64
	upPackets, upFrames, upBytes atomic.Int64
)

func debugStats(context.Context, json.RawMessage) (any, error) {
	st := Stats{
		Goroutines:   runtime.NumGoroutine(),
		HandlerExits: handlerExits.Load(),
		Hellos:       hellos.Load(),
		UpPackets:    upPackets.Load(),
		UpFrames:     upFrames.Load(),
		UpBytes:      upBytes.Load(),
	}
	backendLaneStats(&st)
	return st, nil
}

// Sample is one Debug.Sample measurement.
type Sample struct {
	At         int64 `json:"at"` // wall clock, Unix ms
	Goroutines int   `json:"goroutines"`
	DownOpen   int64 `json:"downOpen"`
}

var (
	samplesMu sync.Mutex
	samples   []Sample
)

// debugSample ([ms, stepMs]) starts sampling runtime.NumGoroutine and
// the open poll requests every stepMs for ms on a goroutine of its own
// (counted in the samples), replacing earlier samples; Debug.Samples
// returns them. It outlives the page that asked, so a test can see what
// happens across a reload.
func debugSample(_ context.Context, args json.RawMessage) (any, error) {
	ms, step := 0, 10
	if err := decodeArgs(args, &ms, &step); err != nil || ms <= 0 || step <= 0 {
		return nil, ipc.NewError(ipc.CodeBadArgs, "Debug.Sample(ms int, stepMs int)")
	}
	samplesMu.Lock()
	samples = nil
	samplesMu.Unlock()
	go func() {
		end := time.Now().Add(time.Duration(ms) * time.Millisecond)
		for t := time.Now(); t.Before(end); t = time.Now() {
			var st Stats
			backendLaneStats(&st)
			s := Sample{At: t.UnixMilli(), Goroutines: runtime.NumGoroutine(), DownOpen: st.DownOpen}
			samplesMu.Lock()
			samples = append(samples, s)
			samplesMu.Unlock()
			time.Sleep(time.Duration(step) * time.Millisecond)
		}
	}()
	return struct{}{}, nil
}

func debugSamples(context.Context, json.RawMessage) (any, error) {
	samplesMu.Lock()
	defer samplesMu.Unlock()
	return append([]Sample{}, samples...), nil
}

// CountMessage counts m in Debug.Stats if it is a runtime hello. The
// duplex examples call it from OnMessage; it does nothing unless the test
// hooks are on.
func CountMessage(m string) {
	if !Probe() || !strings.Contains(m, `"hello"`) {
		return
	}
	var w struct {
		Tango string `json:"tango"`
	}
	if json.Unmarshal([]byte(m), &w) == nil && w.Tango == "hello" {
		hellos.Add(1)
	}
}

// CountUp counts an upstream fast lane packet in Debug.Stats. The duplex
// examples call it from OnBinary before handing p to the router; it does
// nothing unless the test hooks are on.
func CountUp(p []byte) {
	if !Probe() {
		return
	}
	upPackets.Add(1)
	frames, err := ipc.ParsePacket(p)
	if err != nil {
		return
	}
	upFrames.Add(int64(len(frames)))
	for _, f := range frames {
		upBytes.Add(int64(len(f.Payload)))
	}
}

// laneTestsJS is served as /__test/lanetests.js.
//
//go:embed lanetests.js
var laneTestsJS []byte

// Sizes and pacing of the test routes.
const (
	bigSize        = 3 << 20
	streamChunk    = 1 << 10
	streamInterval = 50 * time.Millisecond
)

// CSPBlocked is the Content-Security-Policy of /csp.html: the page's own
// scripts and inline styles run, but fetch may only reach a host that does
// not exist, so no request to the asset origin (lanes included) is allowed.
const CSPBlocked = "default-src 'self' 'unsafe-inline'; connect-src https://example.invalid"

// TestRoutes returns next unchanged unless the test hooks are on; then it
// adds, in front of next:
//
//   - GET /__test/big: 3 MiB, byte i = i % 251, application/octet-stream,
//     with Content-Length (http.ServeContent).
//   - GET /__test/redirect: 302 to /index.html (the core follows it).
//   - POST /__test/echo: the request body back, application/octet-stream.
//   - GET /__test/panic: the handler panics before writing anything.
//   - GET /__test/stream: never ends on its own: a 1 KiB chunk every 50 ms,
//     flushed, until the request context is done; each return counts in
//     Debug.Stats' handlerExits.
//   - GET /__test/lanetests.js: the IPC fast lane scenarios (lanetests.js,
//     window.laneTests) shared by the macOS lane tests and the iOS
//     self-test.
//   - GET /csp.html: the app's index.html under CSPBlocked.
func TestRoutes(next http.Handler) http.Handler {
	if !Probe() {
		return next
	}
	big := make([]byte, bigSize)
	for i := range big {
		big[i] = byte(i % 251)
	}
	mux := http.NewServeMux()
	mux.Handle("/", next)
	mux.HandleFunc("GET /__test/big", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(big))
	})
	mux.HandleFunc("GET /__test/redirect", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/index.html", http.StatusFound)
	})
	mux.HandleFunc("POST /__test/echo", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Write(body)
	})
	mux.HandleFunc("GET /__test/lanetests.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(laneTestsJS)
	})
	// The app's own page under a CSP whose connect-src excludes the asset
	// origin: the lane requests (fetch) are refused, so up:post's start
	// probe fails and the page keeps the message channel
	// (internal/platform/darwin TestLaneCSPBlocked).
	mux.HandleFunc("GET /csp.html", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", CSPBlocked)
		w.Header().Set("Cache-Control", "no-store")
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/index.html"
		next.ServeHTTP(w, r2)
	})
	mux.HandleFunc("GET /__test/panic", func(http.ResponseWriter, *http.Request) {
		panic("services: /__test/panic")
	})
	mux.HandleFunc("GET /__test/stream", func(w http.ResponseWriter, r *http.Request) {
		defer handlerExits.Add(1)
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		rc := http.NewResponseController(w)
		chunk := bytes.Repeat([]byte{'s'}, streamChunk)
		tick := time.NewTicker(streamInterval)
		defer tick.Stop()
		for {
			if _, err := w.Write(chunk); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
			select {
			case <-r.Context().Done():
				return
			case <-tick.C:
			}
		}
	})
	return mux
}
