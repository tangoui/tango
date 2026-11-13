// Command app-demo exercises the app-level features: quit interception,
// deep links / opened files, and single instance.
//
//	go run ./examples/app-demo            # interactive: try Cmd+Q twice, relaunch with a URL
//	go run ./examples/app-demo -smoke     # self-test, prints PASS/FAIL, exits within 10 s
//
// In -smoke mode the first instance launches itself again with a deep link
// argument; the second instance forwards it through package single and
// exits. Then (macOS) the demo asks AppKit to terminate twice: the first
// request is refused by OnQuitRequest, the second is allowed.
// TANGO_SMOKE_TIMEOUT_SCALE=3 stretches the timeouts for slow machines (CI).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/sys/single"
)

const page = `<!doctype html><meta charset=utf-8><title>App demo</title>
<body style="font:14px system-ui;padding:2em"><h1>App demo</h1>
<p>Quit (Cmd+Q) is refused once. Relaunch with <code>demo://...</code> to see single instance.</p>`

func main() {
	smoke := flag.Bool("smoke", false, "run the self-test")
	flag.Parse()

	app := tango.New(tango.Options{
		Name:       "App Demo",
		ID:         "dev.tango.app-demo",
		URLSchemes: []string{"demo"},
		Assets:     staticPage(page),
	})
	primary, err := single.Claim(app, single.Options{
		OnSecondInstance: func(args []string, cwd string) {
			fmt.Printf("second instance: args=%q cwd=%s\n", args, cwd)
		},
	})
	if !primary {
		fmt.Printf("forwarded to the running instance (err=%v)\n", err)
		return
	}
	if err != nil {
		fmt.Println("single instance disabled:", err)
	}

	var mu sync.Mutex
	var log []string
	note := func(s string) {
		fmt.Println(s)
		mu.Lock()
		log = append(log, s)
		mu.Unlock()
	}

	gotURL := make(chan string, 4)
	gotFiles := make(chan []string, 4)
	gotReopen := make(chan struct{}, 4)
	app.OnOpenURL(func(u string) { note("open url: " + u); gotURL <- u })
	app.OnOpenFiles(func(p []string) { note(fmt.Sprintf("open files: %q", p)); gotFiles <- p })
	app.OnEvent(func(ev tango.AppEvent) {
		if ev.Kind == tango.EventReopen {
			note("reopen")
			gotReopen <- struct{}{}
		}
	})
	asked := 0
	app.OnQuitRequest(func() bool {
		asked++
		allow := asked > 1
		note(fmt.Sprintf("quit request #%d -> allow=%v", asked, allow))
		return allow
	})

	app.OnReady(func() {
		if _, err := app.NewSurface(tango.SurfaceOptions{Title: "App demo", Width: 640, Height: 400}); err != nil {
			note("surface: " + err.Error())
		}
		if *smoke {
			go runSmoke(app, gotURL, gotFiles, gotReopen, &asked, note)
		}
	})
	if err := app.Run(); err != nil {
		fmt.Println("run:", err)
		os.Exit(1)
	}
	if *smoke {
		mu.Lock()
		defer mu.Unlock()
		if smokeFailed.Load() {
			fmt.Println("FAIL")
			os.Exit(1)
		}
		if asked != 2 {
			fmt.Printf("FAIL: quit asked %d times\n", asked)
			os.Exit(1)
		}
		fmt.Println("PASS")
	}
}

var smokeFailed atomic.Bool

func runSmoke(app *tango.App, gotURL <-chan string, gotFiles <-chan []string, gotReopen <-chan struct{}, asked *int, note func(string)) {
	fail := func(msg string) {
		note("FAIL: " + msg)
		smokeFailed.Store(true)
		app.Quit()
	}
	go func() {
		time.Sleep(scaled(10 * time.Second))
		fmt.Println("FAIL: timeout")
		os.Exit(2)
	}()

	// 1. Single instance: launch a second copy with a deep link.
	exe, err := os.Executable()
	if err != nil {
		fail(err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), scaled(5*time.Second))
	defer cancel()
	out, err := exec.CommandContext(ctx, exe, "demo://hello?from=second").CombinedOutput()
	note("second instance said: " + strings.TrimSpace(string(out)))
	if err != nil || !strings.Contains(string(out), "forwarded") {
		fail(fmt.Sprintf("second instance did not forward: %v", err))
		return
	}
	select {
	case u := <-gotURL:
		if u != "demo://hello?from=second" {
			fail("wrong url " + u)
			return
		}
	case <-time.After(3 * time.Second):
		fail("primary did not receive the URL")
		return
	}

	// 2. macOS: the app delegate's URL / file / reopen callbacks.
	if appKitEvents(app, exe) {
		select {
		case u := <-gotURL:
			if u != "demo://from-appkit" {
				fail("wrong url " + u)
				return
			}
		case <-time.After(3 * time.Second):
			fail("no URL from application:openURLs:")
			return
		}
		select {
		case p := <-gotFiles:
			if len(p) != 1 || p[0] != exe {
				fail(fmt.Sprintf("wrong files %q", p))
				return
			}
		case <-time.After(3 * time.Second):
			fail("no files from application:openURLs:")
			return
		}
		select {
		case <-gotReopen:
		case <-time.After(3 * time.Second):
			fail("no reopen event")
			return
		}
	}

	// 3. Quit interception: a user quit through the platform's quit path.
	if !userQuit(app) {
		note("user quit not scriptable here; SKIP quit interception, quitting from code")
		app.Post(func() { *asked = 2; app.Quit() })
		return
	}
	time.Sleep(300 * time.Millisecond)
	if tango.Await(app, func() int { return *asked }) != 1 {
		fail("first quit request not asked")
		return
	}
	note("still running after refused quit")
	userQuit(app)
}

// scaled stretches d by TANGO_SMOKE_TIMEOUT_SCALE (a factor >= 1, default 1)
// so slow CI machines get more time; unset, the timings are unchanged.
func scaled(d time.Duration) time.Duration {
	if f, err := strconv.ParseFloat(os.Getenv("TANGO_SMOKE_TIMEOUT_SCALE"), 64); err == nil && f > 1 {
		return time.Duration(float64(d) * f)
	}
	return d
}
