//go:build linux && !android

// Command smoke is a manual check of the Linux backend on a real desktop:
//
//	go run ./internal/platform/linux/internal/smoke
//
// It opens a window, serves a page from tango://app through ServeAsset,
// exchanges messages both ways, round-trips the clipboard, installs a menu,
// searches the page (a hit, then a miss that must be answered by WebKit's
// failed-to-find-text signal, not by the backend's timeout) and quits after
// 3 seconds. It exits non-zero if the page never loaded, no echo came back
// or a find check failed. TANGO_SMOKE_TIMEOUT_SCALE=3 stretches the 3
// seconds for slow machines (CI). It uses a minimal Host of its own, not
// package tango.
package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/linux"
)

func init() { runtime.LockOSThread() }

const page = `<!doctype html><meta charset="utf-8"><title>tango smoke</title>
<h1>Tango Linux smoke</h1><pre id="log"></pre>
<script>
const log = s => document.getElementById('log').textContent += s + '\n';
window.__smokeRecv = m => { log('from Go: ' + m); window.__tangoNative.post('echo:' + m); };
fetch('/data.json').then(r => r.json()).then(j => {
  log('fetch: ' + JSON.stringify(j));
  window.__tangoNative.post('hello from ' + location.origin);
});
</script>`

const bridge = `window.__smokeBridge = true;`

type host struct {
	b platform.Backend

	mu    sync.Mutex
	queue []func()

	loaded, echoed bool
	surface        platform.Surface

	findStarted bool
	findResult  string // "" until the find checks finished; "ok" or why they failed
}

// checkFind searches for text on the page (it has one match), then for text
// that is not there. WebKit answers a miss with failed-to-find-text; when
// that handler is not connected, the backend's 3 s timeout answers instead
// with Matches -1, which this check rejects.
func (h *host) checkFind() {
	f, ok := h.surface.(platform.WebFinder)
	if !ok || h.findStarted {
		return
	}
	h.findStarted = true
	f.Find("Linux smoke", platform.FindOptions{}, func(r platform.FindResult, err error) {
		fmt.Printf("find hit: %+v %v\n", r, err)
		if err == platform.ErrUnsupported {
			h.findResult = "ok" // WebKitGTK without find support: nothing to check
			return
		}
		if err != nil || !r.Found || r.Matches != 1 {
			h.findResult = fmt.Sprintf("hit: got %+v, %v; want 1 match", r, err)
			return
		}
		start := time.Now()
		f.Find("no such text on this page", platform.FindOptions{}, func(r platform.FindResult, err error) {
			took := time.Since(start)
			fmt.Printf("find miss: %+v %v after %v\n", r, err, took.Round(time.Millisecond))
			switch {
			case err != nil || r.Found || r.Matches != 0:
				h.findResult = fmt.Sprintf("miss: got %+v, %v; want 0 matches", r, err)
			case took >= 2*time.Second:
				h.findResult = fmt.Sprintf("miss: answered after %v (timeout, not failed-to-find-text)", took)
			default:
				h.findResult = "ok"
				f.StopFind()
			}
		})
	})
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
		if strings.HasPrefix(ev.URL, h.b.AssetOrigin()) {
			h.loaded = true
		}
	case platform.SurfaceClosed:
		h.b.Quit()
	}
}

func (h *host) OnMessage(id platform.SurfaceID, origin, msg string) {
	fmt.Printf("message from %d (%s): %q\n", id, origin, msg)
	if strings.HasPrefix(msg, "echo:") {
		h.echoed = true
		h.checkFind() // the page has rendered by now
		return
	}
	lit, _ := json.Marshal("hi from Go")
	h.surface.Eval("window.__smokeRecv(" + string(lit) + ")")
}

func (h *host) ServeAsset(id platform.SurfaceID, w http.ResponseWriter, r *http.Request) {
	fmt.Printf("asset %s %s (surface %d)\n", r.Method, r.URL, id)
	switch r.URL.Path {
	case "/", "/index.html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, page)
	case "/data.json":
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true}`)
	default:
		http.NotFound(w, r)
	}
}

func main() {
	b := linux.New()
	h := &host{b: b}
	if err := b.Init(h, platform.AppOptions{Name: "Tango Smoke", ID: "dev.tango.smoke"}); err != nil {
		fmt.Fprintln(os.Stderr, "init:", err)
		os.Exit(1)
	}
	s, err := b.NewSurface(1, platform.SurfaceOptions{
		Title: "Tango smoke", Width: 800, Height: 600, Resizable: true, Visible: true,
		URL: b.AssetOrigin() + "/index.html", Debug: true, BridgeScript: bridge,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "surface:", err)
		os.Exit(1)
	}
	h.surface = s

	if m, ok := b.(platform.AppMenu); ok {
		m.SetAppMenu(0, []platform.MenuItem{
			{Role: "appMenu"},
			{Role: "editMenu"},
			{Label: "&Test", Submenu: []platform.MenuItem{{ID: 1, Label: "&Ping", Accelerator: "CmdOrCtrl+P"}}},
		}, func(id uint32) { fmt.Println("menu click:", id) })
	}
	if c, ok := b.(platform.Clipboard); ok {
		_ = c.ClipboardWriteText("tango smoke ✓")
		got, err := c.ClipboardReadText()
		fmt.Printf("clipboard: %q %v\n", got, err)
	}

	go func() {
		time.Sleep(scaled(3 * time.Second))
		h.post(func() {
			w, hh := s.Size()
			fmt.Println("size before quit:", w, hh)
			b.Quit()
		})
	}()
	if err := b.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "run:", err)
		os.Exit(1)
	}
	fmt.Printf("loaded=%v echoed=%v find=%q\n", h.loaded, h.echoed, h.findResult)
	if !h.loaded || !h.echoed {
		os.Exit(2)
	}
	if h.findStarted && h.findResult != "ok" {
		fmt.Fprintln(os.Stderr, "find check failed:", cmp.Or(h.findResult, "no answer before quit"))
		os.Exit(3)
	}
}

// scaled stretches d by TANGO_SMOKE_TIMEOUT_SCALE (a factor >= 1, default 1)
// so slow CI machines get more time; unset, the timings are unchanged.
func scaled(d time.Duration) time.Duration {
	if f, err := strconv.ParseFloat(os.Getenv("TANGO_SMOKE_TIMEOUT_SCALE"), 64); err == nil && f > 1 {
		return time.Duration(float64(d) * f)
	}
	return d
}
