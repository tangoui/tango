// Command a11y-demo shows how a screen reader sees Tango's self-drawn UI
// (docs/accessibility.md, docs/specs/a11y-bridge.md).
//
//	go run ./examples/a11y-demo          # turn on VoiceOver (Cmd+F5) and explore
//	go run ./examples/a11y-demo -smoke   # self-check, exits 0 on PASS (macOS)
//
// With -smoke (macOS) the program reads its own accessibility tree like an
// assistive technology: through the Accessibility API (AXUIElement*) on a
// background thread when the process is trusted for accessibility
// (System Settings > Privacy & Security > Accessibility, granted to the
// terminal), otherwise by calling the NSAccessibility methods of its own
// views in-process. It checks the button's role and name, presses it
// (AXPress) and sees the click arrive in the page, finds a checkbox and
// its value, hit-tests the button's centre, moves focus to the button and
// sees the focus change (AXFocusedUIElementChanged with the AX API, the
// focused element in-process). TANGO_SMOKE_TIMEOUT_SCALE stretches the
// deadlines; TANGO_SMOKE_SKIP=focus downgrades a focus failure (a locked
// screen or a window that cannot become key) to SKIP;
// A11Y_SMOKE_INPROCESS=1 uses the in-process checks even when trusted.
// When the display link does not fire (locked screen) the check drives
// frames itself with -displayIfNeeded; that only works with the CPU
// renderer (TANGO_RENDERER=cpu), a Metal layer draws nothing then.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/uihost"
)

var (
	presses atomic.Int32
	agree   bool
	volume  = 30.0
)

func page(c *ui.Context) {
	frames.Add(1)
	ui.Column(c, func() {
		ui.Title(c, "Accessibility demo", 3)
		if ui.Button(c, "Press me").Clicked() {
			presses.Add(1)
			c.Announce("Pressed "+strconv.Itoa(int(presses.Load()))+" times", a11y.LivePolite)
		}
		ui.Text(c, "Presses: "+strconv.Itoa(int(presses.Load())))
		ui.Checkbox(c, "I agree", &agree)
		ui.Slider(c, &volume, 0, 100)
	})
}

func main() {
	smoke := flag.Bool("smoke", false, "run the self-check and exit")
	flag.Parse()
	app := tango.New(tango.Options{Name: "A11y Demo"})
	app.OnReady(func() {
		_, s, err := uihost.Open(app, uihost.Options{
			Surface: tango.SurfaceOptions{Title: "Accessibility demo", Width: 480, Height: 360},
		}, page, ui.RunnerOptions{})
		if err != nil {
			log.Fatal(err)
		}
		if *smoke {
			go func() {
				ok := runSmoke(app, s)
				smokeOK.Store(ok)
				app.Post(app.Quit)
			}()
		}
	})
	if *smoke {
		go func() {
			time.Sleep(scaled(60 * time.Second))
			fmt.Println("FAIL: watchdog timeout")
			os.Exit(1)
		}()
	}
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
	if *smoke {
		if !smokeOK.Load() {
			fmt.Println("FAIL: a11y-demo")
			os.Exit(1)
		}
		fmt.Println("PASS: a11y-demo")
	}
}

var smokeOK atomic.Bool

// frames counts page builds (diagnostics: no frames means the window is not drawing).
var frames atomic.Int32

// scaled stretches d by TANGO_SMOKE_TIMEOUT_SCALE (>= 1).
func scaled(d time.Duration) time.Duration {
	if f, err := strconv.ParseFloat(os.Getenv("TANGO_SMOKE_TIMEOUT_SCALE"), 64); err == nil && f > 1 {
		return time.Duration(float64(d) * f)
	}
	return d
}
