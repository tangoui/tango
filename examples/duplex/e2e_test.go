package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/automation"
)

// TestE2EDuplex drives the duplex example's page through the automation
// channel: an Echo round trip and a 2 MiB Upload over real WebView
// streams. Skipped unless TANGO_E2E is set:
//
//	TANGO_E2E=1 go test ./examples/duplex -run E2E -v
func TestE2EDuplex(t *testing.T) {
	app := automation.Launch(t, ".")
	win := app.Window("Tango Duplex")
	if win.Kind() != automation.KindWebView {
		t.Fatalf("kind %q", win.Kind())
	}
	win.WaitJS(`document.readyState === "complete" && !!window.tango && !!window.duplex`)

	if got := win.EvalString(`window.duplex.echoOnce("ping")`); got != "ping" {
		t.Fatalf("echo %q", got)
	}
	// Which lanes the page got depends on the backend (macOS: up:post by
	// default; elsewhere fallback until the backend reports fast lane
	// capabilities) and on the inherited environment; logged, not asserted
	// (benchSmoke asserts the macOS default with the switches cleared).
	t.Logf("tango.lanes(): %s", win.EvalString(`JSON.stringify(tango.lanes())`))

	// The Echo buttons go through a tango.socket.
	win.ClickSelector("#echo-json")
	win.ClickSelector("#echo-bytes")
	win.WaitJS(`(t => t.includes("← json:") && t.includes("← bytes:"))(document.getElementById("echo-out").textContent)`)

	var up struct {
		Path  string `json:"path"`
		Bytes int    `json:"bytes"`
	}
	win.EvalInto(`(() => {
		const b = new Uint8Array(2097152);
		for (let i = 0; i < b.length; i++) b[i] = i & 0xff;
		return window.duplex.upload(new Blob([b]), "e2e.bin");
	})()`, &up)
	if up.Bytes != 2097152 {
		t.Fatalf("upload bytes %d, want 2097152", up.Bytes)
	}
	if up.Path == "" {
		t.Fatal("upload returned no path")
	}
	st, err := os.Stat(up.Path)
	if err != nil || st.Size() != 2097152 {
		t.Fatalf("uploaded file %q: %v %v", up.Path, st, err)
	}
	os.Remove(up.Path)
	app.Quit()

	benchSmoke(t)
}

// benchSmoke runs go run . -bench (docs/ipc-bench.md) and checks that it
// prints its result as one JSON line with the fields the benchmark
// analysis reads, then quits on its own. With the lane switches cleared,
// macOS uses its default (up:post on).
func benchSmoke(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "run", ".", "-bench")
	cmd.Env = append(os.Environ(), "TANGO_IPC_LANES_DARWIN=", "TANGO_IPC_LANES=", "TANGO_SCHEME_PROBE=")
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("-bench: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	var res struct {
		Lane              struct{ Down, Up string } `json:"lane"`
		LatencyBatchP99Ms *float64                  `json:"latencyBatchP99Ms"`
		UpMBps            float64                   `json:"upMBps"`
		DownMBps          float64                   `json:"downMBps"`
	}
	found := false
	for _, line := range strings.Split(stdout.String(), "\n") {
		if !strings.HasPrefix(line, "{") || !strings.Contains(line, `"latencyBatchP99Ms"`) {
			continue
		}
		if err := json.Unmarshal([]byte(line), &res); err != nil {
			t.Fatalf("-bench line %q: %v", line, err)
		}
		found = true
	}
	if !found || res.LatencyBatchP99Ms == nil {
		t.Fatalf("-bench printed no JSON line with latencyBatchP99Ms:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	t.Logf("-bench: lane %+v, batch p99 %.2f ms, up %.1f MB/s, down %.1f MB/s", res.Lane, *res.LatencyBatchP99Ms, res.UpMBps, res.DownMBps)
	if res.UpMBps <= 0 || res.DownMBps <= 0 || *res.LatencyBatchP99Ms <= 0 {
		t.Errorf("-bench result out of range: %+v", res)
	}
	if runtime.GOOS == "darwin" && res.Lane.Up != "up:post" {
		t.Errorf("-bench on macOS with default switches: up lane %q, want up:post (on by default)", res.Lane.Up)
	}
}

// TestE2EPanel launches with -panel: the page terminal runs ls through
// Term, the page's Echo keeps working, and the Go-drawn panel (its own
// Echo stream through Router.Open on the same Router) redraws as echoes
// arrive. With TANGO_E2E_SHOT=path it saves a screenshot there:
//
//	TANGO_E2E=1 go test ./examples/duplex -run E2EPanel -v
func TestE2EPanel(t *testing.T) {
	app := automation.Launch(t, ".", "-panel")
	win := app.Window("Tango Duplex")
	win.WaitJS(`document.readyState === "complete" && !!window.tango && document.body.classList.contains("with-panel")`)

	// The terminal: type ls and press Enter in its input.
	win.Eval(`(() => {
		const el = document.getElementById("term-in");
		el.value = "ls";
		el.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter" }));
		return true;
	})()`)
	// ls runs in the package directory, the test's working directory.
	win.WaitJS(`(t => t.includes("main.go") && t.includes("panel.go"))(document.getElementById("term-out").textContent)`)

	if got := win.EvalString(`window.duplex.echoOnce("ping")`); got != "ping" {
		t.Fatalf("page echo %q", got)
	}

	// The Go panel draws its echoes: Panel.Stats counts the frames it drew
	// and the echoes it showed (one a second); both must move.
	stats := func() (frames, echoes int64) {
		var st struct{ Frames, Echoes int64 }
		win.EvalInto(`tango.call("Panel.Stats")`, &st)
		return st.Frames, st.Echoes
	}
	f0, e0 := stats()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(200 * time.Millisecond) {
		f, e := stats()
		if f > f0 && e > e0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the Go panel did not show a new echo within 5s (frames %d -> %d, echoes %d -> %d)", f0, f, e0, e)
		}
	}

	// The panel is not in the automation tree (uihost.Attach does not
	// register attached canvases) and the WebView screenshot shows only
	// the page, so on macOS capture the window itself (screencapture -l:
	// this window's pixels only, canvases included) and watch the panel's
	// corner: its echo list changes once a second. That needs the Screen
	// Recording permission for the process running the test (the terminal):
	// without it macOS hides other apps' window titles and screencapture
	// gets no window content, so the pixel check is skipped then (logged).
	shot := os.Getenv("TANGO_E2E_SHOT")
	if shot != "" {
		win.SaveScreenshot(strings.TrimSuffix(shot, ".png") + "-page.png") // the page only
	}
	if runtime.GOOS != "darwin" {
		t.Log("panel pixels not checked: the window capture is macOS only")
		app.Quit()
		return
	}
	if !screenCaptureAllowed(t) {
		t.Log("panel pixels not checked: this process has no Screen Recording permission " +
			"(System Settings > Privacy & Security > Screen Recording, for the terminal running go test)")
		app.Quit()
		return
	}
	id := cgWindowID(t, win.Title())
	first := panelPixels(t, captureWindow(t, id))
	deadline := time.Now().Add(5 * time.Second)
	for bytes.Equal(first, panelPixels(t, captureWindow(t, id))) {
		if time.Now().After(deadline) {
			t.Fatal("the Go panel did not redraw within 5s")
		}
		time.Sleep(200 * time.Millisecond)
	}
	if shot != "" {
		time.Sleep(3 * time.Second) // a few echoes in the list
		if err := os.WriteFile(shot, captureWindow(t, id), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	app.Quit()
}

// screenCaptureAllowed reports whether this process (and so the swift and
// screencapture tools it starts) may capture other apps' windows: macOS's
// Screen Recording permission (CGPreflightScreenCaptureAccess).
func screenCaptureAllowed(t *testing.T) bool {
	t.Helper()
	f := filepath.Join(t.TempDir(), "access.swift")
	if err := os.WriteFile(f, []byte("import CoreGraphics\nprint(CGPreflightScreenCaptureAccess())\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("swift", f).Output()
	if err != nil {
		t.Fatalf("swift: %v %s", err, out)
	}
	return strings.TrimSpace(string(out)) == "true"
}

// cgWindowID returns the CoreGraphics id of the newest on-screen window
// titled title (macOS; looked up with a one-line Swift script).
func cgWindowID(t *testing.T, title string) string {
	t.Helper()
	script := `import CoreGraphics
let l = CGWindowListCopyWindowInfo(.optionOnScreenOnly, kCGNullWindowID) as! [[String: Any]]
let ids = l.filter { ($0["kCGWindowName"] as? String) == CommandLine.arguments[1] }.compactMap { $0["kCGWindowNumber"] as? Int }
if let id = ids.max() { print(id) }`
	f := filepath.Join(t.TempDir(), "winid.swift")
	if err := os.WriteFile(f, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("swift", f, title).Output()
	id := strings.TrimSpace(string(out))
	if err != nil || id == "" {
		t.Fatalf("window id of %q: %v %s", title, err, out)
	}
	return id
}

// captureWindow returns a PNG of window id (frame included, no shadow),
// taken by macOS screencapture.
func captureWindow(t *testing.T, id string) []byte {
	t.Helper()
	f := filepath.Join(t.TempDir(), "window.png")
	if out, err := exec.Command("screencapture", "-x", "-o", "-l"+id, f).CombinedOutput(); err != nil {
		t.Fatalf("screencapture: %v %s", err, out)
	}
	b, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// panelPixels returns the RGBA pixels of the top-right 300×300 logical
// pixels of a window capture, where the panel card is drawn.
func panelPixels(t *testing.T, shot []byte) []byte {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(shot))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	scale := max(b.Dx()/1060, 1) // the -panel window is 1060 logical pixels wide
	var out []byte
	for y := b.Min.Y; y < b.Min.Y+300*scale && y < b.Max.Y; y++ {
		for x := b.Max.X - 300*scale; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			out = append(out, byte(r>>8), byte(g>>8), byte(bl>>8), byte(a>>8))
		}
	}
	return out
}
