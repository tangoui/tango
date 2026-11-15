// duplex-mobile is the iOS and Android self-test of Tango's ipc streams: the duplex
// examples' services (package examples/duplex/services: Echo, Upload,
// Sink, Source, Bench.Report) behind an ipc.Router wired like
// examples/duplex, and a page (web/selftest.js) that exercises them and
// reports to Go, which logs one line per result.
//
//	bash mobile/ios/build.sh examples/duplex-mobile simulator
//	GOFLAGS=-tags=tango_selftest bash mobile/android/build.sh examples/duplex-mobile
//
// On Android the variables below come from the launch extra TANGO_TEST_ENV
// (tango_selftest builds, debuggable APK: am start ... --esa TANGO_TEST_ENV
// TANGO_SCHEME_PROBE=1,TANGO_SCHEME_SELFTEST=1; scripts/smoke.sh
// android-selftest). The Android lanes (down:android, up:android) are on
// by default; TANGO_IPC_LANES_ANDROID=0 turns them off.
//
// With TANGO_SCHEME_PROBE=1 (simctl launch: SIMCTL_CHILD_TANGO_SCHEME_PROBE=1)
// the page opens as index.html?probe and runs the WebKit streaming probe
// instead: the iOS backend answers /__tango/probe/stream itself with 8
// chunks of 1 KiB, 200 ms apart, and the page logs when each one arrived
// ("probe: {...}"); scripts/smoke.sh ios-probe judges it. TANGO_SCHEME_PROBE=1
// also adds the scheme-handler test routes (/__test/..., package
// examples/duplex/services) and the ipc method Debug.Stats. With
// TANGO_SCHEME_SELFTEST=1 as well the page opens as index.html?selftest
// instead and runs the scheme-handler self-test: the request sequence of
// internal/platform/darwin's TestAsyncSchemeParity, checked against the
// expected answers, and an aborted never-ending stream whose Go handler
// must return, and the IPC fast lanes the page got (smoke.sh launches with
// TANGO_IPC_LANES_DARWIN=1, which turns them on: iOS lanes are off by
// default; on macOS, where up:post is on by default, the =1 changes
// nothing) and the lane scenarios of
// examples/duplex/services/lanetests.js ("selftest: {...}";
// scripts/smoke.sh ios-selftest judges it). With TANGO_IPC_LANE_FAULT set
// too (a tango_dev or tango_selftest build, scripts/smoke.sh ios-lanefault:
// TANGO_IPC_LANE_FAULT=up:post:3) the page opens as
// index.html?selftest&lanefault and runs only the lane fault scenario
// ("lanefault: {...}"). With TANGO_DUPLEX_BENCH=1 the page opens as
// index.html?bench whatever the other variables say (it is checked first)
// and runs the duplex benchmark of /bench.js (package
// examples/duplex/services, the same code as go run ./examples/duplex
// -bench; "bench: {...}", docs/ipc-bench.md). TANGO_SCHEME_PROBE=1 may be
// added for Debug.Stats' up:post counters in the result (the benchmark
// verdict runs in docs/ipc-bench.md were made without it). On iOS add
// TANGO_IPC_LANES_DARWIN=1 to measure up:post (off by default there); on
// the desktop up:post is on by default and TANGO_IPC_LANES_DARWIN=0 gives
// the fallback configuration. It also runs on the desktop
// (go run ./examples/duplex-mobile).
package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"os"
	"runtime"
	"strings"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/asset"
	"github.com/tangoui/tango/examples/duplex/services"
	"github.com/tangoui/tango/ipc"
	"github.com/tangoui/tango/runtime-js/runtimejs"
)

//go:embed web
var web embed.FS

func main() {
	site, _ := fs.Sub(web, "web")
	app := tango.New(tango.Options{Name: "Tango Duplex", ID: "dev.tango.duplexios", Assets: services.TestRoutes(services.BenchRoute(asset.FS(site, asset.Options{})))})

	r := ipc.NewRouter()
	services.Register(r)
	if services.Probe() {
		// The self-test's memory check: this process's phys_footprint in
		// bytes (0 off Apple platforms).
		r.Handle("Debug.Footprint", func(context.Context, json.RawMessage) (any, error) {
			return footprint(), nil
		})
	}
	// The platform the page runs on, for the lanes it should expect:
	// {"goos", "androidLanes"}. androidLanes: TANGO_IPC_LANES_ANDROID is not
	// off (on by default)
	// (read as internal/platform/android does) and the test hook
	// TANGO_ANDROID_NO_ARRAYBUFFER, which makes the backend treat the
	// WebView as one without ArrayBuffer messages, is not.
	r.Handle("Debug.Platform", func(context.Context, json.RawMessage) (any, error) {
		on := !switchOff(os.Getenv("TANGO_IPC_LANES_ANDROID")) && !switchOn(os.Getenv("TANGO_ANDROID_NO_ARRAYBUFFER"))
		return map[string]any{"goos": runtime.GOOS, "androidLanes": on}, nil
	})
	// The page's self-test results: one log line each ("probe: {...}",
	// "parity: [...]", "selftest: {...}"). On Android, where logcat cuts a
	// line at about 4 KB, a result over reportChunk bytes is split into
	// "kind#1: ...", "kind#2: ", ... and a last line "kind#end";
	// scripts/smoke.sh joins them.
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

	app.AddBridgeScript(runtimejs.Script)
	app.OnMessage(func(s *tango.Surface, m string) {
		services.CountMessage(m) // Debug.Stats (test hooks only)
		r.HandleMessage(s, m)
	})
	// Byte frames on IPC fast lanes, when the page gets them.
	app.OnBinary(func(s *tango.Surface, gen uint64, p []byte) {
		services.CountUp(p) // Debug.Stats (test hooks only)
		r.HandleBinarySession(s, gen, p)
	})

	app.OnReady(func() {
		url := "/index.html"
		switch {
		case os.Getenv("TANGO_DUPLEX_BENCH") == "1":
			url = "/index.html?bench"
		case services.Probe() && os.Getenv("TANGO_SCHEME_SELFTEST") == "1" && os.Getenv("TANGO_IPC_LANE_FAULT") != "":
			// up:post:N (iOS) runs the up:post fault scenario; down:android:N
			// and up:android:N the Android ones.
			switch f := os.Getenv("TANGO_IPC_LANE_FAULT"); {
			case strings.HasPrefix(f, "down:android:"):
				url = "/index.html?selftest&lanefault=down"
			case strings.HasPrefix(f, "up:android:"):
				url = "/index.html?selftest&lanefault=up"
			default:
				url = "/index.html?selftest&lanefault"
			}
		case services.Probe() && os.Getenv("TANGO_SCHEME_SELFTEST") == "1":
			url = "/index.html?selftest"
		case services.Probe():
			url = "/index.html?probe"
		}
		s, err := app.NewSurface(tango.SurfaceOptions{Title: "Tango Duplex", Width: 420, Height: 640, URL: url, Debug: true})
		if err != nil {
			log.Fatal(err)
		}
		s.OnClosed(func() { r.Drop(s) })
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// reportChunk is the longest self-test result logged as one line on
// Android (logcat keeps about 4 KB of a line).
const reportChunk = 3000

// report logs a self-test result: "kind: line", or on Android, when line is
// longer than reportChunk, "kind#1: ..." to "kind#n: ..." and "kind#end".
func report(kind, line string) {
	if runtime.GOOS != "android" || len(line) <= reportChunk {
		log.Printf("%s: %s", kind, line)
		return
	}
	n := 0
	for len(line) > 0 {
		k := min(reportChunk, len(line))
		n++
		log.Printf("%s#%d: %s", kind, n, line[:k])
		line = line[k:]
	}
	log.Printf("%s#end", kind)
}

// switchOff reports whether v is an "off" value: "0", "false" or "off"
// (any case, surrounding spaces ignored), as the platform backends read
// TANGO_IPC_LANES_ANDROID (unset is on).
func switchOff(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "0", "false", "off":
		return true
	}
	return false
}

// switchOn reads an opt-in variable (TANGO_ANDROID_NO_ARRAYBUFFER): unset,
// empty, "0", "false" and "off" (any case, surrounding spaces ignored) are
// off.
func switchOn(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "0", "false", "off":
		return false
	}
	return true
}
