// Command system-demo exercises the desktop integration packages (screen,
// theme, shell, appctl, autostart, env) and logs PASS/FAIL for each check.
// It quits by itself within a few seconds.
//
// RevealPath opens a file manager window, so it only runs with
// TANGO_DEMO_REVEAL=1. TANGO_SMOKE_TIMEOUT_SCALE=3 stretches the 5 second
// timeout for slow machines (CI).
package main

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/sys/appctl"
	"github.com/tangoui/tango/sys/autostart"
	"github.com/tangoui/tango/sys/env"
	"github.com/tangoui/tango/sys/screen"
	"github.com/tangoui/tango/sys/shell"
	"github.com/tangoui/tango/sys/theme"
)

var failed int

func check(name string, err error) {
	if err != nil {
		failed++
		log.Printf("FAIL %s: %v", name, err)
		return
	}
	log.Printf("PASS %s", name)
}

func main() {
	log.SetFlags(log.Ltime | log.Lmicroseconds)
	app := tango.New(tango.Options{Name: "System Demo", ID: "dev.tango.systemdemo"})
	theme.OnChange(app, func(t theme.Theme) { log.Printf("theme changed: dark=%v accent=%s", t.Dark, theme.AccentCSS(t.Accent)) })
	screen.OnChange(app, func() { log.Print("displays changed") })
	_ = theme.SyncPage(app)

	app.OnReady(func() {
		go func() {
			run(app)
			app.Quit()
		}()
	})
	time.AfterFunc(scaled(5*time.Second), func() {
		log.Print("FAIL timeout")
		os.Exit(2)
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
	if failed > 0 {
		log.Printf("%d check(s) failed", failed)
		os.Exit(1)
	}
	log.Print("all checks passed")
}

func run(app *tango.App) {
	in := env.Info(app)
	log.Printf("env: %+v", in)
	check("env.Info", nonEmpty(in.OSVersion, "OSVersion", in.WebView, "WebView"))

	ds, err := screen.All(app)
	check("screen.All", err)
	for _, d := range ds {
		log.Printf("display %s %q bounds=%+v work=%+v scale=%g primary=%v", d.ID, d.Name, d.Bounds, d.WorkArea, d.Scale, d.Primary)
	}
	if err == nil && (len(ds) == 0 || !ds[0].Primary || ds[0].Bounds.X != 0 || ds[0].Bounds.Y != 0 || ds[0].Scale <= 0) {
		check("screen primary at origin", fmt.Errorf("%+v", ds))
	}
	if p, err := screen.Primary(app); err == nil {
		d, err := screen.At(app, p.Bounds.Width/2, p.Bounds.Height/2)
		if err == nil && d.ID != p.ID {
			err = fmt.Errorf("At(center of primary) = %s", d.ID)
		}
		check("screen.At", err)
	} else {
		check("screen.Primary", err)
	}

	t, err := theme.System(app)
	log.Printf("theme: dark=%v accent=%s", t.Dark, theme.AccentCSS(t.Accent))
	check("theme.System", err)

	check("appctl.SetBadge 3", appctl.SetBadge(app, "3"))
	check("appctl.SetProgress 0.5", appctl.SetProgress(app, 0.5))
	time.Sleep(800 * time.Millisecond) // visible in the Dock meanwhile
	check("appctl.SetBadge clear", appctl.SetBadge(app, ""))
	check("appctl.SetProgress hide", appctl.SetProgress(app, -1))
	check("appctl.RequestAttention", appctl.RequestAttention(app, false))

	if shell.Allowed("javascript:alert(1)") || !shell.Allowed("https://example.com") {
		check("shell scheme allow-list", errors.New("unexpected result"))
	} else {
		check("shell scheme allow-list", nil)
	}
	tmp, err := os.CreateTemp("", "tango-reveal-*.txt")
	if err == nil {
		tmp.Close()
		defer os.Remove(tmp.Name())
	}
	if os.Getenv("TANGO_DEMO_REVEAL") == "1" && err == nil {
		check("shell.RevealPath", shell.RevealPath(app, tmp.Name()))
	} else {
		log.Print("SKIP shell.RevealPath (set TANGO_DEMO_REVEAL=1; opens a file manager window)")
	}
	err = shell.RevealPath(app, filepath.Join(os.TempDir(), "tango-no-such-file"))
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	} else {
		err = fmt.Errorf("missing path gave %v", err)
	}
	check("shell.RevealPath missing", err)

	dir, err := os.MkdirTemp("", "tango-autostart-*")
	check("autostart temp dir", err)
	if err == nil {
		defer os.RemoveAll(dir)
		o := autostart.Options{ID: "dev.tango.systemdemo", Name: "System Demo", Dir: dir}
		check("autostart.Enable", autostart.Enable(o))
		on, err := autostart.IsEnabled(o)
		if err == nil && !on {
			err = errors.New("not enabled after Enable")
		}
		check("autostart.IsEnabled", err)
		check("autostart.Disable", autostart.Disable(o))
		if on, _ := autostart.IsEnabled(o); on {
			check("autostart disabled", errors.New("still enabled"))
		}
	}
}

func nonEmpty(kv ...string) error {
	for i := 0; i < len(kv); i += 2 {
		if kv[i] == "" {
			return errors.New(kv[i+1] + " empty")
		}
	}
	return nil
}

// scaled stretches d by TANGO_SMOKE_TIMEOUT_SCALE (a factor >= 1, default 1)
// so slow CI machines get more time; unset, the timings are unchanged.
func scaled(d time.Duration) time.Duration {
	if f, err := strconv.ParseFloat(os.Getenv("TANGO_SMOKE_TIMEOUT_SCALE"), 64); err == nil && f > 1 {
		return time.Duration(float64(d) * f)
	}
	return d
}
