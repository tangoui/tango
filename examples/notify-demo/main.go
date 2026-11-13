// Command notify-demo smoke-tests the system integration packages on a
// real backend: clipboard images (written, read back, compared; the
// clipboard's text is saved first and restored at the end), a global
// hotkey registered and unregistered, a notification (from a bare binary
// it must fail with a clear error rather than crash), and a window
// shortcut triggered by a synthetic keydown. It logs PASS/FAIL per check,
// quits within 5 seconds and exits 1 if anything failed.
//
// For unattended runs: TANGO_SMOKE_SKIP is a comma-separated list of check
// groups to skip (clipboard, hotkey, notify, shortcut), e.g. notify on a
// headless Linux session without a notification server;
// TANGO_SMOKE_TIMEOUT_SCALE=3 stretches the timeouts for slow machines.
// Unset, every check runs with the default timings.
//
//	go run ./examples/notify-demo
package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/sys/clipboard"
	"github.com/tangoui/tango/sys/hotkey"
	"github.com/tangoui/tango/sys/notify"
)

var (
	start  = time.Now()
	mu     sync.Mutex
	failed bool
	seen   = map[string]bool{}
)

func report(name string, ok bool, detail string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	seen[name] = true
	status := "PASS"
	if !ok {
		status, failed = "FAIL", true
	}
	fmt.Printf("[%5.2fs] %s %-16s %s\n", time.Since(start).Seconds(), status, name, fmt.Sprintf(detail, args...))
}

func testImage() []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 3))
	for y := 0; y < 3; y++ {
		for x := 0; x < 4; x++ {
			img.SetNRGBA(x, y, color.NRGBA{uint8(60 * x), uint8(80 * y), 200, 255})
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

func samePixels(a, b []byte) bool {
	ia, err1 := png.Decode(bytes.NewReader(a))
	ib, err2 := png.Decode(bytes.NewReader(b))
	if err1 != nil || err2 != nil || ia.Bounds() != ib.Bounds() {
		return false
	}
	for y := ia.Bounds().Min.Y; y < ia.Bounds().Max.Y; y++ {
		for x := ia.Bounds().Min.X; x < ia.Bounds().Max.X; x++ {
			r1, g1, b1, a1 := ia.At(x, y).RGBA()
			r2, g2, b2, a2 := ib.At(x, y).RGBA()
			if r1>>8 != r2>>8 || g1>>8 != g2>>8 || b1>>8 != b2>>8 || a1>>8 != a2>>8 {
				return false
			}
		}
	}
	return true
}

func clipboardChecks(a *tango.App) {
	saved, err := clipboard.ReadText(a)
	if err != nil {
		report("clipboard-save", false, "%v", err)
		return
	}
	defer func() {
		err := clipboard.WriteText(a, saved)
		back, _ := clipboard.ReadText(a)
		report("clipboard-restore", err == nil && back == saved, "restored %d bytes of text", len(saved))
	}()
	img := testImage()
	if err := clipboard.WriteImage(a, img); err != nil {
		report("clipboard-image", false, "write: %v", err)
		return
	}
	got, err := clipboard.ReadImage(a)
	report("clipboard-image", err == nil && samePixels(img, got), "wrote %d bytes, read %d bytes (err %v)", len(img), len(got), err)
	files, err := clipboard.ReadFiles(a)
	report("clipboard-files", err == nil && len(files) == 0, "image on clipboard -> files %v err %v", files, err)
	if err := clipboard.WriteImage(a, []byte("nope")); !errors.Is(err, clipboard.ErrNotPNG) {
		report("clipboard-badpng", false, "%v", err)
	} else {
		report("clipboard-badpng", true, "rejected non-PNG")
	}
}

func hotkeyChecks(a *tango.App) {
	const acc = "CmdOrCtrl+Alt+Shift+F19"
	h, err := hotkey.Register(a, acc, func() {})
	if err != nil {
		report("hotkey-register", false, "%v", err)
		return
	}
	report("hotkey-register", true, "%s", acc)
	_, err = hotkey.Register(a, acc, func() {})
	report("hotkey-duplicate", err != nil, "second registration: %v", err)
	report("hotkey-unregister", h.Unregister() == nil && h.Unregister() == hotkey.ErrUnregistered, "unregistered")
	h2, err := hotkey.Register(a, acc, func() {})
	report("hotkey-reregister", err == nil, "after unregister: %v", err)
	if h2 != nil {
		h2.Unregister()
	}
}

func notifyChecks(a *tango.App) {
	ok, perr := notify.RequestPermission(a)
	id, err := notify.Send(a, notify.Message{Title: "Tango", Subtitle: "notify-demo", Body: "Hello from the smoke test"})
	switch {
	case err == nil:
		report("notify", true, "sent %s (permission %v %v)", id, ok, perr)
	case runtime.GOOS == "darwin" && perr != nil && err.Error() == perr.Error():
		report("notify", true, "not bundled, clear error: %v", err)
	default:
		report("notify", false, "permission (%v, %v), send: %v", ok, perr, err)
	}
}

const page = `<!doctype html><title>notify-demo</title><body><p>notify-demo</p></body>`

func main() {
	a := tango.New(tango.Options{Name: "notify-demo", ID: "dev.tango.notify-demo",
		Assets: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte(page))
		})})
	a.OnReady(func() {
		s, err := a.NewSurface(tango.SurfaceOptions{Title: "notify-demo", Width: 360, Height: 200})
		if err != nil {
			report("surface", false, "%v", err)
			a.Quit()
			return
		}
		fired := make(chan struct{}, 1)
		if _, err := hotkey.Bind(s, "CmdOrCtrl+Shift+K", func() { fired <- struct{}{} }); err != nil {
			report("window-shortcut", false, "bind: %v", err)
		}
		loaded := make(chan struct{}, 1)
		s.OnLoaded(func() {
			select {
			case loaded <- struct{}{}:
			default:
			}
		})
		go func() {
			if !skipped("clipboard") {
				clipboardChecks(a)
			}
			if !skipped("hotkey") {
				hotkeyChecks(a)
			}
			if !skipped("notify") {
				notifyChecks(a)
			}
			if skipped("shortcut") {
				a.Quit()
				return
			}
			select {
			case <-loaded:
				meta, ctrl := "true", "false"
				if runtime.GOOS != "darwin" {
					meta, ctrl = "false", "true"
				}
				s.Eval(`window.dispatchEvent(new KeyboardEvent("keydown", {key: "K", code: "KeyK", shiftKey: true, metaKey: ` + meta + `, ctrlKey: ` + ctrl + `, bubbles: true}))`)
				select {
				case <-fired:
					report("window-shortcut", true, "synthetic keydown fired the binding")
				case <-time.After(1500 * time.Millisecond):
					report("window-shortcut", false, "binding did not fire")
				}
			case <-time.After(scaled(2 * time.Second)):
				report("window-shortcut", false, "page did not load")
			}
			a.Quit()
		}()
	})
	go func() {
		time.Sleep(scaled(4500 * time.Millisecond))
		report("timeout", false, "quitting at the deadline")
		a.Quit()
	}()
	if err := a.Run(); err != nil {
		report("run", false, "%v", err)
	}
	if failed {
		os.Exit(1)
	}
	fmt.Println("ALL PASS")
}

// scaled stretches d by TANGO_SMOKE_TIMEOUT_SCALE (a factor >= 1, default 1)
// so slow CI machines get more time; unset, the timings are unchanged.
func scaled(d time.Duration) time.Duration {
	if f, err := strconv.ParseFloat(os.Getenv("TANGO_SMOKE_TIMEOUT_SCALE"), 64); err == nil && f > 1 {
		return time.Duration(float64(d) * f)
	}
	return d
}

// skipped reports whether TANGO_SMOKE_SKIP names the check group, and logs
// the skip.
func skipped(group string) bool {
	for _, g := range strings.Split(os.Getenv("TANGO_SMOKE_SKIP"), ",") {
		if strings.TrimSpace(g) == group {
			fmt.Printf("[%5.2fs] SKIP %-16s TANGO_SMOKE_SKIP\n", time.Since(start).Seconds(), group)
			return true
		}
	}
	return false
}
