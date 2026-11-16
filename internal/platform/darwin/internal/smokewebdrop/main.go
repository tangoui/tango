//go:build darwin && !ios

// Command smokewebdrop checks file drags over a WebView surface created
// with AcceptFiles on the real macOS backend: the page sees the hover
// (dragenter / dragover / dragleave, with "Files" in the types) so it can
// highlight a drop target, the drop goes to OnFilesDropped (paths and
// point) instead of to the page or a navigation, and a drag without files
// is left to WebKit. The drag is simulated (an NSDraggingInfo test object
// handed to the WKWebView, as WebKit's own tests do); a real drag from
// Finder is a manual check. Exits 0 only if every check passed.
//
//	go run ./internal/platform/darwin/internal/smokewebdrop
package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/internal/platform"
)

const page = `<!doctype html><html><body style="margin:0;height:100vh">
<script>
for (const t of ["dragenter", "dragover", "dragleave", "drop"]) {
  document.addEventListener(t, function (e) {
    e.preventDefault();
    __tango.post("ev:" + t + ":" + e.clientX + "," + e.clientY + ":" + Array.from(e.dataTransfer.types).join("|"));
  });
}
__tango.post("ready");
</script></body></html>`

type hook interface {
	SmokeWebViewDrag(sid platform.SurfaceID, phase string, paths []string, x, y float64) (uint, error)
}

var (
	start  = time.Now()
	failed bool
	steps  = make(chan string, 256)
)

func logf(format string, args ...any) {
	fmt.Printf("[%6.3fs] %s\n", time.Since(start).Seconds(), fmt.Sprintf(format, args...))
}

func check(name string, ok bool, detail string) {
	if ok {
		logf("PASS %-18s %s", name, detail)
	} else {
		failed = true
		logf("FAIL %-18s %s", name, detail)
	}
}

func emit(s string) {
	if os.Getenv("SMOKE_DEBUG") != "" {
		logf("  step %q", s)
	}
	select {
	case steps <- s:
	default:
	}
}

// collect returns the steps that arrive within d.
func collect(d time.Duration) []string {
	var out []string
	deadline := time.After(d)
	for {
		select {
		case s := <-steps:
			out = append(out, s)
		case <-deadline:
			return out
		}
	}
}

// waitFor waits for a step equal to want.
func waitFor(want string, d time.Duration) bool {
	deadline := time.After(d)
	for {
		select {
		case s := <-steps:
			if s == want {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

func has(list []string, prefix string) (string, bool) {
	for _, s := range list {
		if strings.HasPrefix(s, prefix) {
			return s, true
		}
	}
	return "", false
}

func main() {
	app := tango.New(tango.Options{Name: "Tango smokewebdrop", ID: "dev.tango.smokewebdrop",
		Assets: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprint(w, page)
		})})
	app.OnMessage(func(_ *tango.Surface, m string) { emit(m) })
	app.OnReady(func() {
		s, err := app.NewSurface(tango.SurfaceOptions{Title: "drop", Width: 400, Height: 300, AcceptFiles: true})
		if err != nil {
			check("created", false, err.Error())
			app.Quit()
			return
		}
		s.OnFilesDropped(func(paths []string, x, y int) {
			emit(fmt.Sprintf("dropped:%s@%d,%d", strings.Join(paths, "|"), x, y))
		})
		s.OnNavigating(func(u string) { emit("nav:" + u) })
		go func() {
			defer app.Quit()
			run(app, s)
		}()
	})
	time.AfterFunc(30*time.Second, func() { logf("timeout"); failed = true; app.Quit() })
	if err := app.Run(); err != nil {
		fmt.Println("run:", err)
		os.Exit(1)
	}
	if failed {
		fmt.Println("FAIL")
		os.Exit(1)
	}
	fmt.Println("PASS")
}

func run(app *tango.App, s *tango.Surface) {
	if !waitFor("ready", 10*time.Second) {
		check("ready", false, "page never became ready")
		return
	}
	hk, ok := app.Backend().(hook)
	if !ok {
		check("hook", false, "backend has no SmokeWebViewDrag")
		return
	}
	drag := func(phase string, paths []string, x, y float64) uint {
		var err error
		op := tango.Await(app, func() uint {
			var op uint
			op, err = hk.SmokeWebViewDrag(s.ID(), phase, paths, x, y)
			return op
		})
		if err != nil {
			check("drag-"+phase, false, err.Error())
		}
		return op
	}
	files := []string{"/tmp/tango a.txt", "/tmp/b.png"}

	// Hover, then leave: the page sees it, nothing is dropped.
	op := drag("enter", files, 100, 80)
	time.Sleep(150 * time.Millisecond)
	drag("move", files, 150, 90)
	got := collect(400 * time.Millisecond)
	enter, okEnter := has(got, "ev:dragenter:")
	over, okOver := has(got, "ev:dragover:150,90:")
	check("hover-enter", op == 1 && okEnter && strings.Contains(enter, "Files"), fmt.Sprintf("op %d %q", op, enter))
	check("hover-over", okOver && strings.Contains(over, "Files"), fmt.Sprintf("%q (all: %q)", over, got))
	drag("leave", nil, 150, 90)
	got = collect(400 * time.Millisecond)
	_, okLeave := has(got, "ev:dragleave:")
	_, dropped := has(got, "dropped:")
	check("hover-leave", okLeave && !dropped, fmt.Sprintf("%q", got))

	// Hover, then drop: the host gets the files; the page gets dragleave,
	// not drop, and nothing navigates.
	drag("enter", files, 100, 80)
	time.Sleep(150 * time.Millisecond)
	drag("move", files, 120, 60)
	time.Sleep(150 * time.Millisecond)
	op = drag("drop", files, 120, 60)
	got = collect(600 * time.Millisecond)
	d, okDrop := has(got, "dropped:")
	check("drop-host", op == 1 && okDrop && d == "dropped:/tmp/tango a.txt|/tmp/b.png@120,60", fmt.Sprintf("op %d %q", op, d))
	_, pageDrop := has(got, "ev:drop:")
	_, okLeave = has(got, "ev:dragleave:")
	_, nav := has(got, "nav:")
	check("drop-page", okLeave && !pageDrop && !nav, fmt.Sprintf("%q", got))

	// A drag without files stays WebKit's: no host drop.
	drag("enter", nil, 100, 80)
	time.Sleep(150 * time.Millisecond)
	drag("leave", nil, 100, 80)
	got = collect(400 * time.Millisecond)
	_, dropped = has(got, "dropped:")
	check("no-files", !dropped, fmt.Sprintf("%q", got))
}
