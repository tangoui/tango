//go:build windows

// Command smoke exercises the Windows backend without the tango root
// package: it opens a window, serves a page through ServeAsset, exchanges
// messages with it, sets a menu, and quits after 3 seconds. Exit status 0
// means the page loaded and its message round-tripped.
// TANGO_SMOKE_TIMEOUT_SCALE=3 stretches the 3 seconds for slow machines (CI).
//
//	GOOS=windows go build -o smoke.exe ./internal/platform/windows/internal/smoke
package main

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/windows"
)

func init() { runtime.LockOSThread() }

const page = `<!doctype html>
<meta charset="utf-8">
<title>Tango smoke</title>
<h1>Tango Windows smoke test</h1>
<pre id="log"></pre>
<script>
  const log = s => document.getElementById('log').textContent += s + '\n';
  window.__smokeRecv = s => { log('from Go: ' + s); window.__tangoNative.post('ack:' + s); };
  log('origin: ' + location.origin);
  fetch('/data.json').then(r => r.json()).then(j => {
    log('fetched: ' + JSON.stringify(j));
    window.__tangoNative.post('hello from JS, fetched ' + j.n);
  });
</script>`

// bridge stands in for the core's BridgeScript: it proves injection order.
const bridge = `window.__smokeBridge = typeof window.__tangoNative === 'object';`

type host struct {
	b       platform.Backend
	mu      sync.Mutex
	queue   []func()
	surface platform.Surface

	loaded, gotMessage, gotAck bool
}

func (h *host) post(fn func()) {
	h.mu.Lock()
	h.queue = append(h.queue, fn)
	h.mu.Unlock()
	h.b.Wake()
}

func (h *host) Drain() {
	h.mu.Lock()
	q := h.queue
	h.queue = nil
	h.mu.Unlock()
	for _, fn := range q {
		fn()
	}
}

func (h *host) OnLifecycle(ev platform.LifecycleEvent) { fmt.Println("lifecycle:", ev) }

func (h *host) ShouldClose(id platform.SurfaceID) bool {
	fmt.Println("should close:", id)
	return true
}

func (h *host) OnSurfaceEvent(id platform.SurfaceID, ev platform.SurfaceEvent) {
	fmt.Printf("surface %d event: %+v\n", id, ev)
	switch ev.Kind {
	case platform.SurfaceLoaded:
		h.loaded = true
		h.surface.Eval(`window.__tangoNative.post('bridge ran first: ' + window.__smokeBridge)`)
	case platform.SurfaceClosed:
		h.b.Quit()
	}
}

func (h *host) OnMessage(id platform.SurfaceID, origin, msg string) {
	fmt.Printf("message from %d (%s): %q\n", id, origin, msg)
	if origin != h.b.AssetOrigin() {
		fmt.Println("FAIL: unexpected origin")
		return
	}
	if len(msg) >= 4 && msg[:4] == "ack:" {
		h.gotAck = true
		return
	}
	if !h.gotMessage {
		h.gotMessage = true
		h.surface.Eval(`window.__smokeRecv(` + strconv.Quote("hello from Go") + `)`)
	}
}

func (h *host) ServeAsset(id platform.SurfaceID, w http.ResponseWriter, r *http.Request) {
	fmt.Printf("asset %d: %s %s\n", id, r.Method, r.URL)
	switch r.URL.Path {
	case "/", "/index.html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, page)
	case "/data.json":
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"n":42}`)
	default:
		http.NotFound(w, r)
	}
}

func main() {
	b := windows.New()
	h := &host{b: b}
	if err := b.Init(h, platform.AppOptions{Name: "Tango Smoke", ID: "dev.tango.smoke"}); err != nil {
		fmt.Println("init:", err)
		os.Exit(1)
	}
	s, err := b.NewSurface(1, platform.SurfaceOptions{
		Title: "Tango smoke", Width: 800, Height: 600, Resizable: true, Visible: true,
		URL: b.AssetOrigin() + "/", Debug: true, BridgeScript: bridge,
	})
	if err != nil {
		fmt.Println("new surface:", err)
		os.Exit(1)
	}
	h.surface = s
	if m, ok := b.(platform.AppMenu); ok {
		m.SetAppMenu(0, []platform.MenuItem{
			{Label: "&File", Submenu: []platform.MenuItem{
				{ID: 1, Label: "Say hi", Accelerator: "CmdOrCtrl+Shift+H"},
				{Separator: true},
				{Role: "quit"},
			}},
			{Role: "editMenu"},
		}, func(id uint32) {
			fmt.Println("menu click:", id)
			s.Eval(`window.__smokeRecv("menu ` + strconv.Itoa(int(id)) + `")`)
		})
	}
	if c, ok := b.(platform.Clipboard); ok {
		if err := c.ClipboardWriteText("tango smoke ✓"); err != nil {
			fmt.Println("clipboard write:", err)
		} else if got, err := c.ClipboardReadText(); err != nil || got != "tango smoke ✓" {
			fmt.Printf("FAIL clipboard: %q %v\n", got, err)
		} else {
			fmt.Println("clipboard ok")
		}
	}
	w, hgt := s.Size()
	fmt.Printf("logical size %dx%d, hwnd %#x\n", w, hgt, s.Native())

	go func() {
		time.Sleep(scaled(3 * time.Second))
		h.post(b.Quit)
	}()
	if err := b.Run(); err != nil {
		fmt.Println("run:", err)
		os.Exit(1)
	}
	fmt.Printf("loaded=%v message=%v ack=%v\n", h.loaded, h.gotMessage, h.gotAck)
	if !h.loaded || !h.gotMessage || !h.gotAck {
		fmt.Println("FAIL")
		os.Exit(1)
	}
	fmt.Println("PASS")
}

// scaled stretches d by TANGO_SMOKE_TIMEOUT_SCALE (a factor >= 1, default 1)
// so slow CI machines get more time; unset, the timings are unchanged.
func scaled(d time.Duration) time.Duration {
	if f, err := strconv.ParseFloat(os.Getenv("TANGO_SMOKE_TIMEOUT_SCALE"), 64); err == nil && f > 1 {
		return time.Duration(float64(d) * f)
	}
	return d
}
