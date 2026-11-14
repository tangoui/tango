// Command duplex is the bidirectional stream example: an embedded page
// talks to Go services over ipc streams that carry JSON and bytes both ways.
//
//   - Echo: the page sends JSON and bytes, Go writes each frame back
//     (Router.HandleDuplex, Stream.Receive / Write / WriteBytes).
//   - Upload: file.stream().pipeTo(stream.writable); Go writes the bytes to a
//     temporary file as they arrive and reports progress on the same stream.
//   - Terminal: a local shell (Go "Term", os/exec pipes through
//     Stream.ReadWriter) driven by the page's stream.readable/writable.
//   - Go panel (-panel): a Go-drawn card over the page that uses the same
//     Echo service in process (Router.Open), next to the page's own.
//   - Benchmark: 64 B echo latency (p50/p99) and 64 KiB throughput in both
//     directions, printed as one JSON line.
//
// Run it with:
//
//	go run ./examples/duplex          # interactive
//	go run ./examples/duplex -panel   # with the Go-drawn Echo panel
//	go run ./examples/duplex -bench   # runs the benchmark, prints JSON, quits
//
// -page NAME loads another page of frontend/ instead of index.html, e.g.
// probe.html, the WebKit streaming probe of internal/platform/darwin's
// TestStreamingDeliveryProbe (needs TANGO_SCHEME_PROBE=1 and, on macOS, a
// development build: -tags tango_dev, which automation adds), or csp.html, the
// app's page under a CSP that blocks fetch to the asset origin
// (TestLaneCSPBlocked; a test route, needs TANGO_SCHEME_PROBE=1).
//
// TANGO_SCHEME_PROBE=1 also adds the test routes (/__test/...) and the
// "Debug.Stats" ipc method of package services, used by the scheme
// handler and lane tests of internal/platform/darwin. On macOS the backend's
// own counters in Debug.Stats (and the bench's upMain) and
// TANGO_IPC_LANES_DARWIN=all need a development build too
// (go run -tags tango_dev ./examples/duplex); a plain build reports the
// counters as zero and treats =all like the default. frontend/iframe.html
// is a same-origin subframe page for TestLaneSubframeAccess.
package main

import (
	"embed"
	"flag"
	"io/fs"
	"log"
	"os"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/asset"
	"github.com/tangoui/tango/examples/duplex/services"
	"github.com/tangoui/tango/ipc"
	"github.com/tangoui/tango/runtime-js/runtimejs"
)

//go:embed frontend
var frontend embed.FS

func main() {
	bench := flag.Bool("bench", false, "run the benchmark, print its JSON result on stdout and quit")
	panel := flag.Bool("panel", false, "attach a Go-drawn panel that uses the Echo service through Router.Open")
	page := flag.String("page", "index.html", "the page of frontend/ the window loads (probe.html: the WebKit streaming probe)")
	flag.Parse()

	site, _ := fs.Sub(frontend, "frontend")
	app := tango.New(tango.Options{Name: "Tango Duplex", ID: "dev.tango.duplex", Assets: services.TestRoutes(services.BenchRoute(asset.FS(site, asset.Options{})))})

	r := ipc.NewRouter()
	registerServices(r)
	if *bench {
		services.SetBenchDone(app.Quit)
	}

	app.AddBridgeScript(runtimejs.Script)
	app.OnMessage(func(s *tango.Surface, m string) {
		if os.Getenv("TANGO_DEBUG") != "" {
			log.Printf("msg: %.200s", m)
		}
		services.CountMessage(m) // Debug.Stats (test hooks only)
		r.HandleMessage(s, m)
	})
	// Byte frames on IPC fast lanes, when the page gets them.
	app.OnBinary(func(s *tango.Surface, gen uint64, p []byte) {
		services.CountUp(p) // Debug.Stats (test hooks only)
		r.HandleBinarySession(s, gen, p)
	})

	app.OnReady(func() {
		url, width := "/"+*page, 760
		if *page == "index.html" {
			url = "/"
		}
		switch {
		case *bench:
			url = "/" + *page + "?bench"
			time.AfterFunc(2*time.Minute, func() { log.Print("FAIL benchmark timeout"); os.Exit(1) })
		case *panel:
			url, width = "/"+*page+"?panel", 1060 // the page leaves room on the right
		}
		s, err := app.NewSurface(tango.SurfaceOptions{Title: "Tango Duplex", Width: width, Height: 640, URL: url, Resizable: true, Debug: true})
		if err != nil {
			log.Fatal(err)
		}
		s.OnClosed(func() { r.Drop(s) })
		if *panel {
			// The panel shares r with the page: one Echo service for both.
			if err := attachPanel(app, s, r); err != nil {
				log.Fatal(err)
			}
		}
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
