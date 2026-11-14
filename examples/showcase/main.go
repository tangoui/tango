// Command showcase is Tango's demo app: a WebView page (plain HTML/CSS/JS,
// no npm) that shows the IPC layer (bound calls, errors, events, streams
// with back-pressure and cancel), system capabilities reached from the page
// through jsapi (window, clipboard, dialogs, notifications, displays,
// theme), and the native side driven from Go (tray icon, app menu,
// notifications, theme following).
//
//	go run ./examples/showcase          # interactive
//	go run ./examples/showcase -smoke   # runs every unattended check, prints PASS/FAIL, exits
//
// In -smoke mode the page runs its checks and reports them with the "done"
// event; checks that need a person (dialogs, menus, tray clicks) are only
// listed as skipped. Notifications need a .app bundle on macOS: from a bare
// binary notify fails with a clear error, which the smoke run reports as a
// skip. TANGO_SMOKE_TIMEOUT_SCALE=3 stretches the timeout for slow machines.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"io/fs"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/asset"
	"github.com/tangoui/tango/bind"
	"github.com/tangoui/tango/ipc"
	"github.com/tangoui/tango/jsapi"
	"github.com/tangoui/tango/runtime-js/runtimejs"
	"github.com/tangoui/tango/sys/menu"
	"github.com/tangoui/tango/sys/notify"
	"github.com/tangoui/tango/sys/theme"
	"github.com/tangoui/tango/sys/tray"
)

//go:embed frontend
var frontend embed.FS

// shell holds the native pieces the page asks about and the tray and menu
// talk to.
type shell struct {
	app *tango.App
	r   *ipc.Router

	mu     sync.Mutex
	win    *tango.Surface
	tray   *tray.Tray
	status Status
}

// Status is what Showcase.Status reports to the page.
type Status struct {
	Tray      bool   `json:"tray"`
	TrayError string `json:"trayError,omitempty"`
	Menu      bool   `json:"menu"`
	MenuError string `json:"menuError,omitempty"`
	ThemeSync bool   `json:"themeSync"`
	Smoke     bool   `json:"smoke"`
}

func main() {
	smoke := flag.Bool("smoke", false, "run the unattended checks and exit")
	flag.Parse()
	log.SetFlags(log.Ltime | log.Lmicroseconds)

	site, err := fs.Sub(frontend, "frontend")
	if err != nil {
		log.Fatal(err)
	}
	keepRunning := false // the tray keeps the app alive after the window closes
	app := tango.New(tango.Options{
		Name:            "Tango Showcase",
		ID:              "dev.tango.showcase",
		Assets:          asset.FS(site, asset.Options{}),
		QuitOnLastClose: &keepRunning,
	})

	r := ipc.NewRouter()
	sh := &shell{app: app, r: r}
	sh.status.Smoke = *smoke

	svc := &Showcase{sh: sh, started: time.Now()}
	if err := bind.Register(r, "Showcase", svc); err != nil {
		log.Fatal(err)
	}
	r.HandleStream("Showcase.Ticker", svc.ticker)

	app.AddBridgeScript(runtimejs.Script)
	app.OnMessage(func(s *tango.Surface, m string) {
		if os.Getenv("TANGO_DEBUG") != "" {
			log.Printf("msg: %.300s", m)
		}
		r.HandleMessage(s, m)
	})
	// Byte frames on IPC fast lanes, when the page gets them.
	app.OnBinary(func(s *tango.Surface, gen uint64, p []byte) { r.HandleBinarySession(s, gen, p) })
	// Everything that leaves the window is off by default; turn on what
	// the demo shows. Hotkeys and OpenPath stay off on purpose, so the page
	// can show what a forbidden capability looks like.
	jsapi.Install(app, r, jsapi.Options{
		Allow: jsapi.ClipboardRead | jsapi.ClipboardWrite | jsapi.Dialogs |
			jsapi.Notify | jsapi.OpenURL | jsapi.AppControl,
	})

	// Theme following: the WebView applies prefers-color-scheme by itself;
	// SyncPage also puts data-tango-theme and --tango-accent on <html>.
	if err := theme.SyncPage(app); err == nil {
		sh.status.ThemeSync = true
	}
	theme.OnChange(app, func(t theme.Theme) {
		sh.emit("go:theme", map[string]any{"dark": t.Dark, "accent": theme.AccentCSS(t.Accent)})
	})

	// Events from the page.
	r.On("ping", func(c ipc.Conn, data json.RawMessage) {
		_ = r.Emit(c, "pong", map[string]any{"echo": data, "at": time.Now().Format(time.RFC3339Nano)})
	})
	r.On("log", func(_ ipc.Conn, data json.RawMessage) { log.Printf("page: %s", data) })
	r.On("done", func(_ ipc.Conn, data json.RawMessage) { finish(app, *smoke, data) })

	app.OnReady(func() {
		sh.setupMenu()
		sh.setupTray()
		sh.openWindow(*smoke)
		if *smoke {
			time.AfterFunc(scaled(30*time.Second), func() { log.Print("FAIL timeout"); os.Exit(1) })
		}
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// finish handles the page's "done" report.
func finish(app *tango.App, smoke bool, data json.RawMessage) {
	var got struct {
		Passed   []string `json:"passed"`
		Skipped  []string `json:"skipped"`
		Failures []string `json:"failures"`
	}
	_ = json.Unmarshal(data, &got)
	for _, p := range got.Passed {
		log.Printf("PASS %s", p)
	}
	for _, s := range got.Skipped {
		log.Printf("SKIP %s", s)
	}
	for _, f := range got.Failures {
		log.Printf("FAIL %s", f)
	}
	if !smoke {
		return
	}
	if len(got.Failures) > 0 || len(got.Passed) == 0 {
		log.Printf("FAIL showcase: %d failed, %d passed", len(got.Failures), len(got.Passed))
		os.Exit(1)
	}
	log.Printf("PASS showcase (%d checks, %d skipped)", len(got.Passed), len(got.Skipped))
	app.Quit()
}

// openWindow creates the main window, or shows it again.
func (sh *shell) openWindow(smoke bool) {
	sh.mu.Lock()
	w := sh.win
	sh.mu.Unlock()
	if w != nil {
		w.Show()
		w.Focus()
		return
	}
	url := "/"
	if smoke {
		url = "/?smoke"
	}
	s, err := sh.app.NewSurface(tango.SurfaceOptions{
		Title: "Tango Showcase", Width: 980, Height: 680, MinWidth: 720, MinHeight: 480,
		URL: url, Resizable: true, Debug: true, AcceptFiles: true,
	})
	if err != nil {
		log.Fatal(err)
	}
	s.OnClosed(func() {
		sh.r.Drop(s)
		sh.mu.Lock()
		sh.win = nil
		hasTray := sh.tray != nil
		sh.mu.Unlock()
		if !hasTray {
			sh.app.Quit() // nothing left to bring the window back
		}
	})
	sh.mu.Lock()
	sh.win = s
	sh.mu.Unlock()
}

// emit sends an app event to the main window's page, if it is open.
func (sh *shell) emit(name string, data any) {
	sh.mu.Lock()
	w := sh.win
	sh.mu.Unlock()
	if w != nil {
		_ = sh.r.Emit(w, name, data)
	}
}

// sendNotification is shared by the tray and the app menu.
func (sh *shell) sendNotification(from string) {
	go func() {
		id, err := notify.Send(sh.app, notify.Message{
			Title: "Tango Showcase",
			Body:  "这条通知来自 Go（" + from + "）",
		})
		if err != nil {
			sh.emit("go:notice", map[string]any{"from": from, "error": err.Error()})
			return
		}
		sh.emit("go:notice", map[string]any{"from": from, "id": id})
	}()
}

func (sh *shell) setupMenu() {
	m := menu.New()
	m.Role(menu.AppMenu)
	demo := m.Submenu("演示")
	demo.Add("显示窗口", func() { sh.openWindow(false) }).Accelerator = "CmdOrCtrl+1"
	demo.Add("从 Go 发送通知", func() { sh.sendNotification("应用菜单") }).Accelerator = "CmdOrCtrl+Shift+N"
	demo.Separator()
	var onTop *menu.Item
	onTop = demo.Check("窗口置顶", false, func() {
		sh.mu.Lock()
		w := sh.win
		sh.mu.Unlock()
		if w != nil {
			a := w.Appearance()
			a.AlwaysOnTop = onTop.IsChecked()
			_ = w.SetAppearance(a)
		}
		sh.emit("go:menu", map[string]any{"item": "alwaysOnTop", "checked": onTop.IsChecked()})
	})
	m.Role(menu.EditMenu)
	m.Role(menu.WindowMenu)
	err := menu.SetAppMenu(sh.app, m)
	sh.mu.Lock()
	sh.status.Menu = err == nil
	if err != nil {
		sh.status.MenuError = err.Error()
	}
	sh.mu.Unlock()
}

func (sh *shell) setupTray() {
	t, err := tray.New(sh.app, tray.Options{
		Icon: trayIcon(), Template: true, Tooltip: "Tango Showcase",
		MenuOn: tray.MenuOnSecondary, // left click: OnClick; right click: the menu
	})
	sh.mu.Lock()
	defer sh.mu.Unlock()
	if err != nil {
		sh.status.TrayError = err.Error()
		return
	}
	sh.tray = t
	sh.status.Tray = true

	m := menu.New()
	m.Add("显示窗口", func() { sh.openWindow(false) })
	m.Add("从 Go 发送通知", func() { sh.sendNotification("托盘") })
	var paused *menu.Item
	paused = m.Check("暂停行情流", false, func() {
		sh.emit("go:menu", map[string]any{"item": "pauseTicker", "checked": paused.IsChecked()})
	})
	m.Separator()
	m.Role(menu.Quit)
	if err := t.SetMenu(m); err != nil {
		sh.status.TrayError = err.Error()
	}
	t.OnClick(func() {
		sh.openWindow(false)
		sh.emit("go:tray", "click")
	})
}

// Showcase is bound to the page as "Showcase.*" with bind.Register.
type Showcase struct {
	sh      *shell
	started time.Time

	mu       sync.Mutex
	canceled int // streams the page canceled
}

// Info describes the Go side of the app.
func (s *Showcase) Info() map[string]any {
	return map[string]any{
		"go":         runtime.Version(),
		"os":         runtime.GOOS,
		"arch":       runtime.GOARCH,
		"cpus":       runtime.NumCPU(),
		"pid":        os.Getpid(),
		"uptimeMs":   time.Since(s.started).Milliseconds(),
		"goroutines": runtime.NumGoroutine(),
	}
}

// Greet shows argument decoding and a plain string result.
func (s *Showcase) Greet(name string, times int) (string, error) {
	if times < 1 || times > 5 {
		return "", ipc.NewError(ipc.CodeBadArgs, "times 应在 1 到 5 之间")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "陌生人"
	}
	return strings.Repeat("你好，"+name+"！", times), nil
}

// Divide shows a Go error turning into a rejected promise with a code.
func (s *Showcase) Divide(a, b float64) (float64, error) {
	if b == 0 {
		return 0, ipc.NewError(ipc.CodeBadArgs, "除数不能为 0")
	}
	return a / b, nil
}

// Status reports whether the tray, the app menu and theme sync are up.
func (s *Showcase) Status() Status {
	s.sh.mu.Lock()
	defer s.sh.mu.Unlock()
	return s.sh.status
}

// Canceled reports how many Ticker streams the page has canceled.
func (s *Showcase) Canceled() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.canceled
}

// Notify sends a notification from Go (rather than through jsapi).
func (s *Showcase) Notify() { s.sh.sendNotification("页面调用 Go") }

// ticker is the Showcase.Ticker stream: count samples of the Go runtime,
// one every intervalMs. Write blocks while the stream's byte-credit window
// (1 MiB by default, each sample charged its size plus 64 bytes) is full,
// so a slow page slows the producer down instead of piling up messages.
func (s *Showcase) ticker(ctx context.Context, args json.RawMessage, w ipc.StreamWriter) error {
	var a []int
	if err := json.Unmarshal(args, &a); err != nil || len(a) != 2 || a[0] < 1 || a[1] < 0 {
		return ipc.NewError(ipc.CodeBadArgs, "Ticker(count, intervalMs)")
	}
	count, every := a[0], time.Duration(a[1])*time.Millisecond
	var ms runtime.MemStats
	for i := 1; i <= count; i++ {
		runtime.ReadMemStats(&ms)
		err := w.Write(map[string]any{
			"seq":        i,
			"heapKB":     ms.HeapAlloc / 1024,
			"goroutines": runtime.NumGoroutine(),
			"t":          time.Since(s.started).Milliseconds(),
		})
		if err != nil {
			s.mu.Lock()
			s.canceled++
			s.mu.Unlock()
			return err
		}
		if every > 0 {
			select {
			case <-time.After(every):
			case <-ctx.Done():
				s.mu.Lock()
				s.canceled++
				s.mu.Unlock()
				return ctx.Err()
			}
		}
	}
	return nil
}

// scaled stretches d by TANGO_SMOKE_TIMEOUT_SCALE (a factor >= 1, default 1).
func scaled(d time.Duration) time.Duration {
	if f, err := strconv.ParseFloat(os.Getenv("TANGO_SMOKE_TIMEOUT_SCALE"), 64); err == nil && f > 1 {
		return time.Duration(float64(d) * f)
	}
	return d
}
