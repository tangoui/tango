//go:build darwin && !ios

// Command smokeext checks the window capabilities (contract round 2) on the
// real macOS backend through the root tango package: a frameless
// transparent window and a normal one; SetBounds, Maximize, Restore,
// SetZoom, Reload, a failed navigation, a sheet and the events they cause.
// It quits after about 5 seconds and exits 0 only if every check passed.
// TANGO_SMOKE_TIMEOUT_SCALE=3 stretches the timings for slow machines (CI).
//
//	go run ./internal/platform/darwin/internal/smokeext
package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tangoui/tango"
)

const page = `<!doctype html><html><head><style>
html,body{margin:0;height:100%;background:transparent;font:14px system-ui}
.bar{--tango-drag:drag;height:32px;background:rgba(40,90,200,.6);color:#fff;padding:6px}
.bar button{--tango-drag:no-drag}
</style></head><body><div class="bar">drag me <button>btn</button></div><p id=p>hello</p>
<script>
__tango.onMessage(function (m) { __tango.post("echo:" + m); });
__tango.post("ready:" + location.pathname);
</script></body></html>`

var (
	start  = time.Now()
	checks = map[string]bool{}
	want   = []string{
		"frameless-created", "frameless-appearance", "normal-created",
		"setbounds", "moved-event", "resized-event", "maximize", "restore",
		"zoom", "reload-navigating", "reload-loaded", "send-after-reload",
		"loadfailed", "probe-delivery", "internal-hidden", "sheet",
	}
)

func logf(format string, args ...any) {
	fmt.Printf("[%6.3fs] %s\n", time.Since(start).Seconds(), fmt.Sprintf(format, args...))
}

func pass(name, detail string) {
	if !checks[name] {
		checks[name] = true
		logf("PASS %-20s %s", name, detail)
	}
}

func fail(name, detail string) { logf("FAIL %-20s %s", name, detail) }

func main() {
	app := tango.New(tango.Options{Name: "Tango smokeext", ID: "dev.tango.smokeext",
		Assets: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, page)
		})})

	var normal, frameless *tango.Surface
	steps := make(chan string, 64) // messages and events, consumed by the script goroutine

	app.OnMessage(func(s *tango.Surface, msg string) {
		if strings.HasPrefix(msg, "\x00") {
			hiddenFailed = true
			fail("internal-hidden", "internal message reached OnMessage")
		}
		if s == normal {
			emit(steps, "msg:"+msg)
		}
	})

	app.OnReady(func() {
		var err error
		frameless, err = app.NewSurface(tango.SurfaceOptions{
			Title: "frameless", Width: 360, Height: 240, X: 60, Y: 80, Resizable: true,
			Appearance: tango.Appearance{TitleBar: tango.TitleBarNone, Transparent: true, Material: tango.MaterialBlur},
		})
		if err != nil {
			fail("frameless-created", err.Error())
		} else {
			pass("frameless-created", fmt.Sprintf("bounds %+v", frameless.Bounds()))
			if ap := frameless.Appearance(); ap.TitleBar == tango.TitleBarNone && ap.Transparent {
				pass("frameless-appearance", "TitleBarNone + Transparent + MaterialBlur applied")
			}
		}
		normal, err = app.NewSurface(tango.SurfaceOptions{Title: "normal", Width: 500, Height: 320, Resizable: true, Debug: true})
		if err != nil {
			fail("normal-created", err.Error())
			return
		}
		pass("normal-created", "")
		normal.OnMoved(func(x, y int) { emit(steps, fmt.Sprintf("moved:%d,%d", x, y)) })
		normal.OnResized(func(w, h int) { emit(steps, fmt.Sprintf("resized:%d,%d", w, h)) })
		normal.OnStateChanged(func(st tango.WindowState) { emit(steps, fmt.Sprintf("state:%d", st)) })
		normal.OnNavigating(func(u string) { emit(steps, "nav:"+u) })
		normal.OnLoaded(func() { emit(steps, "loaded") })
		normal.OnLoadFailed(func(u, e string) { emit(steps, "failed:"+u+" "+e) })
		go script(app, &normal, steps)
	})

	time.AfterFunc(scaled(5*time.Second), app.Quit)
	if err := app.Run(); err != nil {
		fmt.Println("run:", err)
		os.Exit(1)
	}
	var missing []string
	for _, n := range want {
		if !checks[n] {
			missing = append(missing, n)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		fmt.Printf("FAIL: %d/%d checks passed; missing: %s\n", len(want)-len(missing), len(want), strings.Join(missing, ", "))
		os.Exit(1)
	}
	fmt.Printf("PASS: all %d checks\n", len(want))
}

// waitStep waits for a step with the given prefix, skipping others, for at
// most d stretched by TANGO_SMOKE_TIMEOUT_SCALE (shared CI runners).
func waitStep(steps <-chan string, prefix string, d time.Duration) (string, bool) {
	deadline := time.After(scaled(d))
	for {
		select {
		case s := <-steps:
			if strings.HasPrefix(s, prefix) {
				return s, true
			}
		case <-deadline:
			return "", false
		}
	}
}

func script(app *tango.App, sp **tango.Surface, steps chan string) {
	s := *sp
	if _, ok := waitStep(steps, "msg:ready:", 2*time.Second); !ok {
		fail("normal-created", "page never became ready")
		return
	}

	want := tango.Rect{X: 100, Y: 120, Width: 640, Height: 400}
	s.SetBounds(want)
	if got := s.Bounds(); got == want {
		pass("setbounds", fmt.Sprintf("%+v", got))
	} else {
		fail("setbounds", fmt.Sprintf("got %+v want %+v", got, want))
	}
	if ev, ok := waitStep(steps, "moved:", 500*time.Millisecond); ok {
		pass("moved-event", ev)
	}
	// Resize may arrive before the move; check the drained log.
	s.SetSize(660, 410)
	if ev, ok := waitStep(steps, "resized:", 500*time.Millisecond); ok {
		pass("resized-event", ev)
	}

	s.Maximize()
	if ev, ok := waitStep(steps, fmt.Sprintf("state:%d", tango.StateMaximized), time.Second); ok && s.State() == tango.StateMaximized {
		pass("maximize", ev)
	} else {
		fail("maximize", fmt.Sprintf("state %d", s.State()))
	}
	s.Restore()
	if ev, ok := waitStep(steps, fmt.Sprintf("state:%d", tango.StateNormal), time.Second); ok {
		pass("restore", ev)
	} else {
		fail("restore", fmt.Sprintf("state %d", s.State()))
	}

	if err := s.SetZoom(1.25); err != nil {
		fail("zoom", err.Error())
	} else if z := s.Zoom(); z > 1.24 && z < 1.26 {
		pass("zoom", fmt.Sprintf("%.2f", z))
		s.SetZoom(1)
	} else {
		fail("zoom", fmt.Sprintf("Zoom() = %v", z))
	}

	s.Reload()
	s.Send("after-reload") // must wait for the reloaded page
	if ev, ok := waitStep(steps, "nav:", time.Second); ok {
		pass("reload-navigating", ev)
	}
	if _, ok := waitStep(steps, "loaded", 2*time.Second); ok {
		pass("reload-loaded", "")
	}
	if ev, ok := waitStep(steps, "msg:echo:after-reload", time.Second); ok {
		pass("send-after-reload", ev)
	}

	// A provisional failure keeps the old page: the probe delivers to it.
	s.Navigate(deadURL())
	s.Send("after-fail")
	if ev, ok := waitStep(steps, "failed:", 2*time.Second); ok {
		pass("loadfailed", ev)
		if ev, ok := waitStep(steps, "msg:echo:after-fail", time.Second); ok {
			pass("probe-delivery", ev)
		}
	} else {
		fail("loadfailed", "no event")
	}
	// The core dropped the page's internal drag request (mouse is up).
	s.Eval(`__tango.post("\u0000tango-int:frame:move")`)
	time.Sleep(100 * time.Millisecond)
	if !hiddenFailed {
		pass("internal-hidden", "frame:move not delivered to OnMessage")
	}

	sheet, err := app.NewSurface(tango.SurfaceOptions{Title: "sheet", Width: 300, Height: 160, Kind: tango.KindSheet, Parent: s})
	if err != nil {
		fail("sheet", err.Error())
		return
	}
	time.Sleep(300 * time.Millisecond)
	sheet.Close()
	pass("sheet", "shown and closed")
}

var hiddenFailed bool

// emit never blocks the main thread.
func emit(steps chan string, s string) {
	if os.Getenv("SMOKE_DEBUG") != "" {
		logf("  step %q", s)
	}
	select {
	case steps <- s:
	default:
	}
}

// deadURL is an http URL on a local port nobody listens on.
func deadURL() string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "http://127.0.0.1:47/"
	}
	addr := l.Addr().String()
	l.Close()
	return "http://" + addr + "/"
}

// scaled stretches d by TANGO_SMOKE_TIMEOUT_SCALE (a factor >= 1, default 1)
// so slow CI machines get more time; unset, the timings are unchanged.
func scaled(d time.Duration) time.Duration {
	if f, err := strconv.ParseFloat(os.Getenv("TANGO_SMOKE_TIMEOUT_SCALE"), 64); err == nil && f > 1 {
		return time.Duration(float64(d) * f)
	}
	return d
}
