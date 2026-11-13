package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/i18n"
	"github.com/tangoui/tango/sys/background"
	"github.com/tangoui/tango/sys/biometric"
	"github.com/tangoui/tango/sys/device"
	"github.com/tangoui/tango/sys/haptics"
	"github.com/tangoui/tango/sys/location"
	"github.com/tangoui/tango/sys/media"
	"github.com/tangoui/tango/sys/network"
	"github.com/tangoui/tango/sys/notify"
	"github.com/tangoui/tango/sys/orientation"
	"github.com/tangoui/tango/sys/permission"
	"github.com/tangoui/tango/sys/power"
	"github.com/tangoui/tango/sys/purchase"
	"github.com/tangoui/tango/sys/push"
	"github.com/tangoui/tango/sys/safearea"
	"github.com/tangoui/tango/sys/securestore"
	"github.com/tangoui/tango/sys/share"
	"github.com/tangoui/tango/sys/shell"
	"github.com/tangoui/tango/sys/widget"
)

// The capability demo (web/caps.html). The page posts JSON requests
//
//	{"id": 1, "op": "share"}
//
// and Go answers with {"id": 1, "result": ...} or {"id": 1, "error": "..."};
// app events arrive as {"event": "...", "data": ...}. Every request runs on
// its own goroutine: on Android, calls that wait for native UI must not
// block the main thread.

type request struct {
	ID    int    `json:"id"`
	Op    string `json:"op"`
	Arg   string `json:"arg"`
	Value string `json:"value"`
}

type reply struct {
	ID     int    `json:"id,omitempty"`
	Event  string `json:"event,omitempty"`
	Result any    `json:"result,omitempty"`
	Data   any    `json:"data,omitempty"`
	Error  string `json:"error,omitempty"`
	Kind   string `json:"kind,omitempty"` // "unsupported" for ErrUnsupported
}

func send(s *tango.Surface, r reply) {
	b, _ := json.Marshal(r)
	s.Send(string(b))
}

// broadcast sends an event to every page.
func broadcast(app *tango.App, name string, data any) {
	for _, s := range app.Surfaces() {
		send(s, reply{Event: name, Data: data})
	}
}

var startedAt = time.Now()

var restored struct {
	sync.Mutex
	n int
}

var watchStop func()

var awake *power.Block // keepAwake toggles it

var bgRuns struct {
	sync.Mutex
	n int
}

// demoProduct is the in-app purchase product the demo queries; it only
// exists in a StoreKit configuration or App Store Connect, so the store
// normally reports it as invalid.
const demoProduct = "dev.tango.mobilehello.pro"

// autorunEnv lists buttons the page taps by itself, for testing on a
// simulator without a UI driver: "op" or "op:arg", comma-separated, e.g.
// TANGO_CAPS_AUTORUN=safearea,secureSet,secureGet (simctl launch passes it
// as SIMCTL_CHILD_TANGO_CAPS_AUTORUN). The app then opens caps.html first.
const autorunEnv = "TANGO_CAPS_AUTORUN"

// autorunProp is the Android system property carrying the same list
// (adb shell setprop debug.tango.caps.autorun notify,network), since adb
// cannot set an app's environment. Read when the app starts.
const autorunProp = "debug.tango.caps.autorun"

// autorun returns the buttons to tap by themselves.
func autorun() string {
	if v := os.Getenv(autorunEnv); v != "" {
		return v
	}
	if runtime.GOOS == "android" {
		if out, err := exec.Command("/system/bin/getprop", autorunProp).Output(); err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return ""
}

// startURL is the first page: caps.html when autorunning.
func startURL() string {
	if autorun() != "" {
		return "/caps.html"
	}
	return ""
}

// setupCaps registers the app-level parts of the demo. Called before Run.
func setupCaps(app *tango.App) {
	if err := safearea.SyncPage(app); err != nil {
		log.Printf("caps: safearea.SyncPage: %v", err)
	}
	safearea.OnChange(app, func(in safearea.Insets) { broadcast(app, "safearea", in) })
	orientation.OnChange(app, func(o orientation.Orientation) { broadcast(app, "orientation", orientation.String(o)) })
	push.OnReceive(app, func(m push.Message) { broadcast(app, "push", m) })
	push.OnOpen(app, func(m push.Message) { broadcast(app, "pushOpened", m) })
	app.OnOpenURL(func(u string) {
		log.Printf("caps: open URL %s", u)
		broadcast(app, "openURL", u)
	})
	device.OnKeyboard(app, func(h float64) { broadcast(app, "keyboard", h) })
	notify.OnClick(app, func(id string) {
		log.Printf("caps: notification clicked %s", id)
		broadcast(app, "notificationClicked", id)
	})
	network.OnChange(app, func(st network.Status) {
		log.Printf("caps: network changed %+v", st)
		broadcast(app, "network", st)
	})
	power.OnChange(app, func(st power.Status) {
		log.Printf("caps: power changed %+v", st)
		broadcast(app, "power", st)
	})
	purchase.OnUpdate(app, func(t purchase.Transaction) {
		log.Printf("caps: transaction %+v", t)
		broadcast(app, "transaction", t)
		if t.State == purchase.Purchased || t.State == purchase.Restored {
			purchase.Finish(app, t)
		}
	})
	background.OnTask(app, func(ctx context.Context, t background.Task) error {
		bgRuns.Lock()
		bgRuns.n++
		n := bgRuns.n
		bgRuns.Unlock()
		log.Printf("caps: background task kind %d #%d, deadline in %v", t.Kind, n, time.Until(t.Deadline).Round(time.Second))
		broadcast(app, "background", map[string]any{"kind": int(t.Kind), "run": n})
		widget.Update(app, widget.Content{Title: "Tango Hello", Text: fmt.Sprintf("background run #%d at %s", n, time.Now().Format("15:04:05"))})
		select {
		case <-time.After(2 * time.Second):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	app.OnWebViewRestored(func(s *tango.Surface, url string) {
		restored.Lock()
		restored.n++
		n := restored.n
		restored.Unlock()
		log.Printf("caps: web view of surface %d restored (%s), #%d", s.ID(), url, n)
	})
}

// handleCaps reports whether msg was a demo request.
func handleCaps(app *tango.App, s *tango.Surface, msg string) bool {
	var req request
	if len(msg) == 0 || msg[0] != '{' || json.Unmarshal([]byte(msg), &req) != nil || req.Op == "" {
		return false
	}
	go func() {
		res, err := run(app, s, req)
		r := reply{ID: req.ID, Result: res}
		if err != nil {
			r.Error = err.Error()
			if errors.Is(err, tango.ErrNotRunning) {
				return
			}
			if errors.Is(err, share.ErrUnsupported) {
				r.Kind = "unsupported"
			}
		}
		log.Printf("caps: %s(%s) -> %v %v", req.Op, req.Arg, res, err)
		send(s, r)
	}()
	return true
}

func run(app *tango.App, s *tango.Surface, req request) (any, error) {
	switch req.Op {
	case "info":
		restored.Lock()
		n := restored.n
		restored.Unlock()
		bgRuns.Lock()
		bg := bgRuns.n
		bgRuns.Unlock()
		return map[string]any{"os": runtime.GOOS + "/" + runtime.GOARCH, "restored": n, "auto": autorun(),
			"background": bg, "pid": os.Getpid(), "started": startedAt.Format("15:04:05")}, nil

	case "share":
		return nil, share.Share(app, share.Options{Text: "Shared from a Tango app", URL: "https://github.com/tangoui/tango"})
	case "shareFile":
		f := filepath.Join(os.TempDir(), "tango-share.txt")
		if err := os.WriteFile(f, []byte("Hello from Tango at "+time.Now().Format(time.RFC3339)+"\n"), 0o644); err != nil {
			return nil, err
		}
		return nil, share.Share(app, share.Options{Files: []string{f}})

	case "haptic":
		kinds := map[string]haptics.Kind{"light": haptics.Light, "medium": haptics.Medium, "heavy": haptics.Heavy,
			"selection": haptics.Selection, "success": haptics.Success, "warning": haptics.Warning, "error": haptics.Error}
		k, ok := kinds[req.Arg]
		if !ok {
			return nil, fmt.Errorf("unknown haptic %q", req.Arg)
		}
		return nil, haptics.Play(app, k)

	case "safearea":
		return safearea.Get(app)
	case "orientation":
		o, err := orientation.Get(app)
		return orientation.String(o), err
	case "lock":
		masks := map[string]orientation.Mask{"portrait": orientation.PortraitOnly, "landscape": orientation.Landscape, "none": 0}
		return nil, orientation.Lock(app, masks[req.Arg])

	case "biometry":
		k, err := biometric.Available(app)
		return biometric.String(k), err
	case "authenticate":
		return "verified", biometric.Authenticate(app, biometric.Options{Reason: "Show that Tango can verify you", AllowPasscode: req.Arg == "passcode"})

	case "secureSet":
		return nil, securestore.SetString(app, "demo", req.Value)
	case "secureGet":
		return securestore.GetString(app, "demo")
	case "secureDelete":
		return nil, securestore.Delete(app, "demo")

	case "pick":
		paths, err := media.Pick(app, media.PickOptions{Types: media.Images | media.Videos, Limit: 3})
		return describe(paths), err
	case "photo":
		p, err := media.TakePhoto(app)
		if err != nil {
			return nil, err
		}
		return describe([]string{p}), nil

	case "location":
		return location.Current(app, location.Options{Accuracy: location.High})
	case "watch":
		if watchStop != nil {
			watchStop()
			watchStop = nil
			return "stopped", nil
		}
		stop, err := location.Watch(app, location.Options{Accuracy: location.Balanced}, func(l location.Location, err error) {
			if err != nil {
				broadcast(app, "location", map[string]string{"error": err.Error()})
				return
			}
			broadcast(app, "location", l)
		})
		if err != nil {
			return nil, err
		}
		watchStop = stop
		return "watching", nil

	case "push":
		return push.Register(app)

	case "permissions":
		out := map[string]string{}
		for _, k := range permission.All {
			st, err := permission.Check(app, k)
			if err != nil {
				if errors.Is(err, permission.ErrUnsupported) {
					return nil, err
				}
				out[string(k)] = "error: " + err.Error()
				continue
			}
			out[string(k)] = string(st)
		}
		return out, nil
	case "request":
		st, err := permission.Request(app, permission.Kind(req.Arg))
		return string(st), err

	case "review":
		return nil, device.RequestReview(app)
	case "settings":
		return nil, device.OpenSettings(app)
	case "keepOn":
		return req.Arg, device.KeepScreenOn(app, req.Arg == "on")
	case "brightness":
		switch req.Arg {
		case "":
		case "reset":
			if err := device.ResetBrightness(app); err != nil {
				return nil, err
			}
		default:
			var v float64
			fmt.Sscan(req.Arg, &v)
			if err := device.SetBrightness(app, v); err != nil {
				return nil, err
			}
		}
		v, err := device.Brightness(app)
		return fmt.Sprintf("%.2f", v), err
	case "statusbar":
		sb := map[string]device.StatusBar{
			"light":  {Style: device.StatusBarLight},
			"dark":   {Style: device.StatusBarDark},
			"hidden": {Hidden: true},
			"color":  {Style: device.StatusBarLight, Color: "#c0392b"},
			"reset":  {Color: "#00000000"},
		}[req.Arg]
		return req.Arg, device.SetStatusBar(app, sb)
	case "keyboard":
		return device.KeyboardHeight(app)
	case "hideKeyboard":
		return nil, device.HideKeyboard(app)

	case "bgSchedule":
		k := background.Refresh
		if req.Arg == "processing" {
			k = background.Processing
		}
		var later time.Duration
		if req.Value == "later" {
			later = time.Hour // run it with adb shell cmd jobscheduler run -f / the simulator debugger
		}
		return "scheduled", background.Schedule(app, background.Request{Kind: k, Earliest: later})
	case "bgCancel":
		background.Cancel(app, background.Processing)
		return "canceled", background.Cancel(app, background.Refresh)

	case "canPay":
		return purchase.CanMakePayments(app), nil
	case "products":
		ps, bad, err := purchase.Products(app, demoProduct)
		return map[string]any{"products": ps, "invalid": bad}, err
	case "buy":
		return purchase.Buy(app, demoProduct)
	case "restore":
		return "restored", purchase.Restore(app)
	case "receipt":
		r, err := purchase.Receipt(app)
		return fmt.Sprintf("%d bytes", len(r)), err

	case "openURL":
		return "opened", shell.OpenURL(app, "https://github.com/tangoui/tango")
	case "openPath":
		f := filepath.Join(os.TempDir(), "tango-open.txt")
		if err := os.WriteFile(f, []byte("Opened by shell.OpenPath at "+time.Now().Format(time.RFC3339)+"\n"), 0o644); err != nil {
			return nil, err
		}
		return f, shell.OpenPath(app, f)
	case "notify":
		return notify.Send(app, notify.Message{Title: "Tango Hello", Subtitle: "sys/notify",
			Body: "A local notification sent at " + time.Now().Format("15:04:05")})
	case "notifyPerm":
		return notify.RequestPermission(app)
	case "network":
		st, err := network.Get(app)
		log.Printf("caps: network %+v %v", st, err)
		return st, err
	case "power":
		st, err := power.Get(app)
		log.Printf("caps: power %+v %v", st, err)
		return st, err
	case "keepAwake":
		if awake != nil {
			awake.Release()
			awake = nil
			return "released", nil
		}
		b, err := power.KeepDisplayOn(app, "caps demo")
		if err != nil {
			return nil, err
		}
		awake = b
		return "held", nil
	case "locale":
		v := map[string]any{"preferred": i18n.Preferred(), "locale": i18n.Locale()}
		log.Printf("caps: locale %v", v)
		return v, nil

	case "widget":
		text := "updated at " + time.Now().Format("15:04:05")
		return text, widget.Update(app, widget.Content{Title: "Tango Hello", Text: text})
	}
	return nil, fmt.Errorf("unknown op %q", req.Op)
}

// describe lists picked files with their sizes.
func describe(paths []string) []string {
	var out []string
	for _, p := range paths {
		if fi, err := os.Stat(p); err == nil {
			out = append(out, fmt.Sprintf("%s (%d KB)", filepath.Base(p), fi.Size()/1024))
		} else {
			out = append(out, p+" ("+err.Error()+")")
		}
	}
	return out
}
