//go:build darwin && !ios

// Command smoke drives the macOS backend end to end without the root tango
// package: a window, an asset page served through tango://, a JS→Go message,
// an Eval round trip, a fetch POST and a streamed response, a sheet dialog,
// the clipboard and the app menu. It quits after about 3 seconds and exits 0
// only if every check passed. TANGO_SMOKE_TIMEOUT_SCALE=3 stretches the
// timings for slow machines (CI).
//
//	go run ./internal/platform/darwin/internal/smoke
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/darwin"
)

func init() { runtime.LockOSThread() }

const surfaceID platform.SurfaceID = 1

type host struct {
	b     platform.Backend
	mu    sync.Mutex
	queue []func()

	start   time.Time
	checks  map[string]bool
	surface platform.Surface
}

func (h *host) logf(format string, args ...any) {
	fmt.Printf("[%6.3fs] %s\n", time.Since(h.start).Seconds(), fmt.Sprintf(format, args...))
}

func (h *host) pass(name, detail string) {
	if !h.checks[name] {
		h.checks[name] = true
		h.logf("PASS %-14s %s", name, detail)
	}
}

// post queues fn for the main thread from any goroutine.
func (h *host) post(fn func()) {
	h.mu.Lock()
	h.queue = append(h.queue, fn)
	h.mu.Unlock()
	h.b.Wake()
}

func (h *host) after(d time.Duration, fn func()) {
	go func() { time.Sleep(d); h.post(fn) }()
}

func (h *host) Drain() {
	for {
		h.mu.Lock()
		q := h.queue
		h.queue = nil
		h.mu.Unlock()
		if len(q) == 0 {
			return
		}
		for _, fn := range q {
			fn()
		}
	}
}

var lifecycleNames = map[platform.LifecycleEvent]string{
	platform.LifecycleStarted: "started", platform.LifecycleForeground: "foreground",
	platform.LifecycleBackground: "background", platform.LifecycleTerminating: "terminating",
}

func (h *host) OnLifecycle(ev platform.LifecycleEvent) {
	h.logf("lifecycle %s", lifecycleNames[ev])
	switch ev {
	case platform.LifecycleStarted:
		h.pass("started", "")
		h.onStarted()
	case platform.LifecycleTerminating:
		h.pass("terminating", "")
	}
}

func (h *host) onStarted() {
	// Wake from another goroutine must lead to a Drain on the main thread.
	go h.post(func() {
		h.pass("wake-drain", "posted from a goroutine")
		// Nested, non-blocking Step from inside a Drain.
		if h.b.Step(false) {
			h.pass("step", "Step(false) returned true")
		}
	})

	if m, ok := h.b.(platform.AppMenu); ok {
		m.SetAppMenu(0, []platform.MenuItem{
			{Role: "appMenu"},
			{Label: "File", Submenu: []platform.MenuItem{
				{ID: 1, Label: "New Window", Accelerator: "CmdOrCtrl+Shift+N"},
				{Separator: true},
				{Role: "close"},
			}},
			{Role: "editMenu"},
			{Role: "windowMenu"},
		}, func(id uint32) { h.logf("menu click %d", id) })
		h.pass("menu", "SetAppMenu ok")
	}

	if c, ok := h.b.(platform.Clipboard); ok {
		old, _ := c.ClipboardReadText()
		want := fmt.Sprintf("tango-smoke-%d", time.Now().UnixNano())
		if err := c.ClipboardWriteText(want); err != nil {
			h.logf("clipboard write: %v", err)
		} else if got, _ := c.ClipboardReadText(); got == want {
			h.pass("clipboard", "write/read round trip")
		} else {
			h.logf("clipboard read %q, want %q", got, want)
		}
		_ = c.ClipboardWriteText(old) // restore the user's clipboard
	}

	s, err := h.b.NewSurface(surfaceID, platform.SurfaceOptions{
		Title: "Tango darwin smoke", Width: 640, Height: 420, MinWidth: 320, MinHeight: 240,
		Resizable: true, Visible: true, Debug: true,
		URL:          h.b.AssetOrigin() + "/index.html",
		BridgeScript: `window.__bridge = "ok";`,
	})
	if err != nil {
		h.logf("NewSurface: %v", err)
		h.b.Quit()
		return
	}
	h.surface = s
	w, ht := s.Size()
	h.pass("surface", fmt.Sprintf("native=%#x size=%dx%d", s.Native(), w, ht))

	h.after(scaled(3*time.Second), func() {
		h.logf("time is up: closing surface and quitting")
		if h.surface != nil {
			h.surface.Close()
		}
		h.b.Quit()
	})
}

func (h *host) ShouldClose(id platform.SurfaceID) bool { return true }

func (h *host) OnSurfaceEvent(id platform.SurfaceID, ev platform.SurfaceEvent) {
	switch ev.Kind {
	case platform.SurfaceLoaded:
		h.pass("loaded", ev.URL)
	case platform.SurfaceClosed:
		h.surface = nil
		h.pass("closed", fmt.Sprintf("surface %d", id))
	case platform.SurfaceResized:
		h.logf("resized %dx%d", ev.Width, ev.Height)
	case platform.SurfaceFocused:
		h.logf("focused")
	case platform.SurfaceBlurred:
		h.logf("blurred")
	}
}

func (h *host) OnMessage(id platform.SurfaceID, origin, msg string) {
	h.logf("message from %s: %s", origin, msg)
	var m struct{ T, Origin, Bridge, Text, V string }
	if err := json.Unmarshal([]byte(msg), &m); err != nil {
		return
	}
	switch m.T {
	case "hello":
		if origin == "tango://app" && m.Origin == "tango://app" {
			h.pass("message", "origin "+origin)
		}
		if m.Bridge == "ok" {
			h.pass("bridge-script", "BridgeScript ran at document start")
		}
		h.surface.Eval(`document.getElementById("s").textContent = "eval-ok";` +
			`__tangoNative.post(JSON.stringify({t: "eval", v: document.getElementById("s").textContent}))`)
	case "eval":
		if m.V == "eval-ok" {
			h.pass("eval", "Go→JS Eval wrote the page and posted back")
		}
		h.showDialog()
	case "stream":
		if m.Text == "part1-part2" {
			h.pass("stream", "flushed response arrived whole")
		}
	case "echo":
		if m.Text == "echo:ping" {
			h.pass("fetch-post", "request body reached ServeAsset")
		}
	case "error":
		h.logf("page error: %s", m.Text)
	}
}

func (h *host) showDialog() {
	d, ok := h.b.(platform.Dialogs)
	if !ok {
		return
	}
	d.MessageDialog(surfaceID, platform.MessageOptions{
		Kind: platform.MessageInfo, Title: "Tango smoke", Message: "Sheet dialog from the darwin backend.",
		Buttons: []string{"OK", "Cancel"},
	}, func(button int, err error) {
		h.pass("dialog-done", fmt.Sprintf("button=%d err=%v", button, err))
	})
	h.pass("dialog-shown", "MessageDialog returned without blocking")
	go func() {
		time.Sleep(700 * time.Millisecond)
		out, err := exec.Command("screencapture", "-x", "/tmp/tango-darwin-smoke.png").CombinedOutput()
		h.post(func() {
			if err != nil {
				h.logf("screencapture failed (non-fatal): %v %s", err, out)
				return
			}
			h.logf("screenshot saved to /tmp/tango-darwin-smoke.png")
		})
	}()
}

const indexHTML = `<!doctype html>
<html><head><meta charset="utf-8"><title>Tango smoke</title>
<style>body{font:16px -apple-system,sans-serif;margin:40px;background:#f4f1ea;color:#222}
#s{font-size:28px;font-weight:600;color:#1a7f37}</style></head>
<body><h1>Tango darwin backend</h1><p>status: <span id="s">waiting</span></p>
<script>
const post = o => __tangoNative.post(JSON.stringify(o));
post({t: "hello", origin: location.origin, bridge: window.__bridge});
fetch("/stream.txt").then(r => r.text()).then(text => post({t: "stream", text}))
  .catch(e => post({t: "error", text: String(e)}));
fetch("/echo", {method: "POST", body: "ping"}).then(r => r.text()).then(text => post({t: "echo", text}))
  .catch(e => post({t: "error", text: String(e)}));
</script></body></html>`

func (h *host) ServeAsset(id platform.SurfaceID, w http.ResponseWriter, r *http.Request) {
	h.logf("asset %s %s (surface %d)", r.Method, r.URL.Path, id)
	switch r.URL.Path {
	case "/", "/index.html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, indexHTML)
		h.pass("asset", "index.html served")
	case "/stream.txt":
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, "part1-")
		w.(http.Flusher).Flush()
		io.WriteString(w, "part2")
	case "/echo":
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, "echo:"+string(body))
	default:
		http.NotFound(w, r)
	}
}

var required = []string{
	"started", "wake-drain", "step", "menu", "clipboard", "surface", "asset", "loaded",
	"message", "bridge-script", "eval", "stream", "fetch-post",
	"dialog-shown", "dialog-done", "closed", "terminating",
}

func main() {
	go func() {
		time.Sleep(scaled(20 * time.Second))
		fmt.Println("FAIL: watchdog timeout")
		os.Exit(2)
	}()
	h := &host{b: darwin.New(), start: time.Now(), checks: map[string]bool{}}
	if err := h.b.Init(h, platform.AppOptions{Name: "Tango Smoke", ID: "dev.tango.smoke"}); err != nil {
		fmt.Println("Init:", err)
		os.Exit(1)
	}
	if err := h.b.Run(); err != nil {
		fmt.Println("Run:", err)
		os.Exit(1)
	}
	var missing []string
	for _, c := range required {
		if !h.checks[c] {
			missing = append(missing, c)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		fmt.Printf("FAIL: missing checks: %s\n", strings.Join(missing, ", "))
		os.Exit(1)
	}
	fmt.Printf("PASS: all %d checks\n", len(required))
}

// scaled stretches d by TANGO_SMOKE_TIMEOUT_SCALE (a factor >= 1, default 1)
// so slow CI machines get more time; unset, the timings are unchanged.
func scaled(d time.Duration) time.Duration {
	if f, err := strconv.ParseFloat(os.Getenv("TANGO_SMOKE_TIMEOUT_SCALE"), 64); err == nil && f > 1 {
		return time.Duration(float64(d) * f)
	}
	return d
}
