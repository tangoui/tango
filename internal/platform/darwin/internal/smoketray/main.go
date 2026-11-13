//go:build darwin && !ios

// Command smoketray checks menus, context menus and the tray icon on the
// real macOS backend through the public tango, menu and tray packages: it
// creates a tray icon with a menu, updates it, clicks tray menu items,
// opens a context menu with menu.Popup and one from the page
// (data-tango-menu), each closed programmatically. It prints PASS/FAIL per
// check, quits within about 5 seconds and exits 0 only if all passed.
// TANGO_SMOKE_TIMEOUT_SCALE=3 stretches the deadlines for slow machines (CI).
//
//	go run ./internal/platform/darwin/internal/smoketray
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/sys/menu"
	"github.com/tangoui/tango/sys/tray"
)

type hooks interface {
	SmokeClosePopup(choose int) bool
	SmokeTrayMenu() [][]string
	SmokeTrayChoose(index int) bool
}

var (
	start   = time.Now()
	mu      sync.Mutex
	results = map[string]bool{}
	order   []string
	ctxHit  chan menu.Context
)

func logf(format string, args ...any) {
	fmt.Printf("[%6.3fs] %s\n", time.Since(start).Seconds(), fmt.Sprintf(format, args...))
}

func check(name string, ok bool, detail string) {
	mu.Lock()
	if _, seen := results[name]; !seen {
		order = append(order, name)
	}
	results[name] = ok
	mu.Unlock()
	verdict := "PASS"
	if !ok {
		verdict = "FAIL"
	}
	logf("%s %-12s %s", verdict, name, detail)
}

func icon(size int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	c := float64(size-1) / 2
	for y := range size {
		for x := range size {
			dx, dy := float64(x)-c, float64(y)-c
			if dx*dx+dy*dy <= c*c {
				img.Set(x, y, color.NRGBA{0, 0, 0, 255})
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

const page = `<!doctype html><html><body style="font:14px system-ui">
<div id="row" data-tango-menu="row" data-tango-menu-data="7" style="padding:20px;background:#eee">right-click me</div>
<div id="css" style="--tango-contextmenu: row; padding:20px">css menu</div>
</body></html>`

func main() {
	a := tango.New(tango.Options{Name: "TraySmoke", Assets: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(page))
	})})

	ctxHit = make(chan menu.Context, 2)
	ctxMenu := menu.New()
	ctxMenu.Add("Delete row", nil)
	if err := menu.RegisterContextFunc(a, "row", func(c menu.Context) *menu.Menu {
		ctxHit <- c
		return ctxMenu
	}); err != nil {
		check("ctx-register", false, err.Error())
	}

	a.OnReady(func() {
		logf("ready")
		go run(a)
	})
	go func() {
		time.Sleep(scaled(5 * time.Second))
		check("deadline", false, "still running at the deadline")
		a.Quit()
	}()
	if err := a.Run(); err != nil {
		logf("Run: %v", err)
		os.Exit(1)
	}
	mu.Lock()
	defer mu.Unlock()
	failed := 0
	for _, n := range order {
		if !results[n] {
			failed++
		}
	}
	logf("%d checks, %d failed", len(order), failed)
	if failed > 0 || len(order) == 0 {
		os.Exit(1)
	}
}

func run(a *tango.App) {
	defer a.Quit()
	h, ok := a.Backend().(hooks)
	if !ok {
		check("hooks", false, "backend has no smoke hooks")
		return
	}
	loaded := make(chan struct{}, 1)
	s, err := a.NewSurface(tango.SurfaceOptions{Title: "Tray smoke", Width: 400, Height: 300, URL: "/"})
	if err != nil {
		check("surface", false, err.Error())
		return
	}
	s.OnLoaded(func() {
		select {
		case loaded <- struct{}{}:
		default:
		}
	})

	logf("surface created")
	// Tray icon with a menu.
	tr, err := tray.New(a, tray.Options{Icon: icon(36), Template: true, Tooltip: "Tango smoke", Title: "T"})
	if err != nil {
		check("tray-new", false, err.Error())
		return
	}
	defer tr.Remove()
	clicked := make(chan string, 8)
	m := menu.New()
	status := m.Add("Status: idle", nil)
	status.Disabled = true
	dark := m.Check("Dark mode", false, func() { clicked <- "dark" })
	m.Separator()
	small := m.Radio("size", "Small", true, func() { clicked <- "small" })
	large := m.Radio("size", "Large", false, func() { clicked <- "large" })
	m.Separator()
	withIcon := m.Add("With icon", nil)
	withIcon.Icon = icon(32)
	if err := tr.SetMenu(m); err != nil {
		check("tray-menu", false, err.Error())
		return
	}
	snap := func() []string {
		rows := tango.Await(a, h.SmokeTrayMenu)
		if len(rows) != 1 {
			return nil
		}
		return rows[0]
	}
	got := snap()
	check("tray-menu", slices.Equal(got, []string{
		"Status: idle|off|disabled", "Dark mode|off|enabled", "|off|disabled",
		"Small|on|enabled", "Large|off|enabled", "|off|disabled", "With icon|off|enabled|icon",
	}), strings.Join(got, ", "))

	// Dynamic update.
	status.SetLabel("Status: busy")
	status.SetDisabled(false)
	dark.SetChecked(true)
	if err := m.Update(); err != nil {
		check("update", false, err.Error())
	}
	got = snap()
	check("update", len(got) == 7 && got[0] == "Status: busy|off|enabled" && got[1] == "Dark mode|on|enabled", strings.Join(got, ", "))

	// Clicking tray menu items: radio exclusivity and check toggle, pushed
	// back to the native menu.
	tango.Await(a, func() bool { return h.SmokeTrayChoose(4) }) // Large
	tango.Await(a, func() bool { return h.SmokeTrayChoose(1) }) // Dark mode (toggle off)
	c1, c2 := recv(clicked), recv(clicked)
	got = snap()
	check("radio-check", c1 == "large" && c2 == "dark" && large.IsChecked() && !small.IsChecked() && !dark.IsChecked() &&
		len(got) == 7 && got[3] == "Small|off|enabled" && got[4] == "Large|on|enabled" && got[1] == "Dark mode|off|enabled",
		fmt.Sprintf("%s %s %s", c1, c2, strings.Join(got, ", ")))
	tr.SetTitle("")
	tr.SetIcon(icon(44), true)

	// menu.Popup: blocks until closed; close it after ~1s choosing item 0.
	pm := menu.New()
	picked := make(chan bool, 1)
	pm.Add("Pick me", func() { picked <- true })
	pm.Add("Other", nil)
	s.Focus()
	var held time.Duration
	pickedOK := false
	for attempt := 0; attempt < 2 && !pickedOK; attempt++ {
		// A popup can be dismissed by the system (activation changes while
		// the app starts); try once more if it closed before we chose.
		closed := make(chan struct{})
		go func() {
			select {
			case <-time.After(time.Second):
				a.Post(func() {
					if !h.SmokeClosePopup(0) {
						logf("no popup open to close")
					}
				})
			case <-closed:
			}
		}()
		t0 := time.Now()
		err = menu.Popup(s, 40, 40, pm)
		held = time.Since(t0)
		close(closed)
		select {
		case pickedOK = <-picked:
		default:
			logf("popup closed after %v without a choice", held.Round(time.Millisecond))
		}
	}
	check("popup", err == nil && pickedOK && held > 800*time.Millisecond, fmt.Sprintf("err=%v picked=%v held=%v", err, pickedOK, held.Round(time.Millisecond)))

	select {
	case <-loaded:
	case <-time.After(scaled(2 * time.Second)):
		check("load", false, "page did not load")
		return
	}

	// By now the menu bar has placed the icon.
	r, ok := tr.Bounds()
	check("tray-bounds", ok && r.Width > 0 && r.Height > 0 && r.Y >= 0 && r.Y < 100, fmt.Sprintf("%+v %v", r, ok))

	// Page-declared context menu: synthesize a contextmenu event.
	go func() {
		time.Sleep(300 * time.Millisecond)
		a.Post(func() { check("ctx-popup", h.SmokeClosePopup(-1), "page context menu was open and closed") })
	}()
	s.Eval(`document.getElementById("row").dispatchEvent(new MouseEvent("contextmenu", {bubbles: true, cancelable: true, clientX: 30, clientY: 25}))`)
	select {
	case c := <-ctxHit:
		check("ctx-page", c.Name == "row" && c.Data == "7" && c.X == 30 && c.Y == 25 && c.Surface == s, fmt.Sprintf("%+v", c))
	case <-time.After(1500 * time.Millisecond):
		check("ctx-page", false, "no context menu request from the page")
	}
	time.Sleep(200 * time.Millisecond) // let the posted close run
}

func recv(c chan string) string {
	select {
	case v := <-c:
		return v
	case <-time.After(time.Second):
		return "(none)"
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
