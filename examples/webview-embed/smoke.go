package main

// Scripted checks for -smoke (macOS: screenshots with screencapture).

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/uihost"
)

var outDir = filepath.Join("build", "webview-embed")

var failures int

func check(name string, ok bool, detail string) {
	state := "PASS"
	if !ok {
		state = "FAIL"
		failures++
	}
	fmt.Printf("%s %s %s\n", state, name, detail)
}

// onUI runs fn on the window's UI thread, which owns the page state, and
// waits (Runner.Post also asks for a frame).
func onUI(fn func()) {
	done := make(chan struct{})
	runner.Post(func() { fn(); close(done) })
	<-done
}

// waitFor polls cond on the UI thread until it holds or timeout.
func waitFor(timeout time.Duration, cond func() bool) bool {
	end := time.Now().Add(timeout)
	for time.Now().Before(end) {
		var ok bool
		onUI(func() { ok = cond() })
		if ok {
			return true
		}
		time.Sleep(30 * time.Millisecond)
	}
	return false
}

func shot(name string) {
	var b tango.Rect
	var ch int
	onUI(func() { b = win.Bounds(); _, ch = win.Size() })
	if ch > b.Height-10 { // Bounds without the title bar: include it
		b.Height = ch + 28
	}
	time.Sleep(250 * time.Millisecond)
	path := filepath.Join(outDir, name)
	r := fmt.Sprintf("%d,%d,%d,%d", b.X, b.Y, b.Width, b.Height)
	if err := exec.Command("screencapture", "-x", "-o", "-R", r, path).Run(); err != nil {
		fmt.Println("screencapture:", err)
		return
	}
	abs, _ := filepath.Abs(path)
	fmt.Println("screenshot", abs)
}

func key(k input.Key, mods input.Modifiers) {
	onUI(func() {
		runner.HandleEvent(input.KeyEvent{Key: k, Down: true, Mods: mods})
		runner.HandleEvent(input.KeyEvent{Key: k, Mods: mods})
	})
}

func runSmoke() {
	defer func() {
		fmt.Printf("smoke done: %d failure(s)\n", failures)
		code := 0
		if failures > 0 {
			code = 1
		}
		tango.Await(app, func() struct{} { app.Quit(); return struct{}{} })
		os.Exit(code)
	}()
	_ = os.MkdirAll(outDir, 0o755)
	onUI(func() { win.Focus() })

	check("loaded", waitFor(15*time.Second, func() bool { return loaded["main"] && loaded["inline"] }), "both pages loaded")
	time.Sleep(300 * time.Millisecond)

	var distinct bool
	onUI(func() {
		a, b := mainWeb.Surface(), inlineWeb.Surface()
		distinct = a != nil && b != nil && a != b && uihost.EmbeddedIn(a) == win && uihost.EmbeddedIn(b) == win
	})
	check("two-webviews", distinct, "two embedded surfaces in the window")

	followsBox := func() (bool, string) {
		var ok bool
		var d string
		onUI(func() {
			r := mainWeb.Rect()
			w, h := mainWeb.Surface().Size()
			ok = abs(float32(w)-r.W) <= 1 && abs(float32(h)-r.H) <= 1
			d = fmt.Sprintf("box %.0fx%.0f native %dx%d", r.W, r.H, w, h)
		})
		return ok, d
	}
	ok, d := followsBox()
	check("main-size", ok, d)
	shot("01-initial.png")

	// Messages both ways.
	onUI(func() { mainWeb.Surface().Eval(`document.getElementById('btn').click()`) })
	check("page-to-go", waitFor(3*time.Second, func() bool { return pageClicks == 1 }), "page button click reached Go")

	// Scroll the list: the inline WebView moves and is clipped.
	onUI(func() { scrollReq = 520 })
	time.Sleep(300 * time.Millisecond)
	shot("02-scrolled-clipped.png")
	onUI(func() { scrollReq = 2000 })
	time.Sleep(300 * time.Millisecond)
	var w2, h2 int
	onUI(func() { w2, h2 = inlineWeb.Surface().Size() })
	check("scrolled-out", w2 > 0 && h2 > 0, fmt.Sprintf("inline still alive %dx%d while scrolled out", w2, h2))
	shot("03-scrolled-out.png")
	onUI(func() { scrollReq = 0 })

	// A dropdown over the main WebView freezes it.
	onUI(func() { menuOpen = true })
	check("menu-freezes", waitFor(3*time.Second, func() bool { return mainWeb.Frozen() }), "main WebView frozen under the menu")
	time.Sleep(200 * time.Millisecond)
	shot("04-menu-over-webview.png")
	onUI(func() { menuOpen = false })
	check("menu-thaws", waitFor(3*time.Second, func() bool { return !mainWeb.Frozen() }), "native view back after closing")

	// A modal covers both.
	onUI(func() { modalOpen = true })
	check("modal-freezes", waitFor(3*time.Second, func() bool { return mainWeb.Frozen() && inlineWeb.Frozen() }), "both frozen under the dialog")
	time.Sleep(400 * time.Millisecond)
	shot("05-modal.png")
	onUI(func() { modalOpen = false })
	waitFor(3*time.Second, func() bool { return !mainWeb.Frozen() && !inlineWeb.Frozen() })

	// Focus: Tab from the menu button enters the page (inline view hidden
	// so the main view is the next Tab stop) ...
	onUI(func() { showInline = false })
	check("inline-closed", waitFor(3*time.Second, func() bool { return inlineClosed }), "removing the box closes its WebView")
	onUI(func() { focusReq = menuID })
	time.Sleep(200 * time.Millisecond)
	key(input.KeyTab, 0)
	check("tab-into-page", waitFor(3*time.Second, func() bool { return pageFocus == "btn" }), "page focused its first element: "+pageFocus)
	// ... and Tab after the last field comes back to the UI, after the box.
	onUI(func() {
		mainWeb.Surface().Eval(`document.getElementById('second').focus();document.activeElement.dispatchEvent(new KeyboardEvent('keydown',{key:'Tab',bubbles:true,cancelable:true}))`)
	})
	check("tab-out-of-page", waitFor(3*time.Second, func() bool { return afterFocused }), "UI focus on the control after the WebView")
	onUI(func() { focusReq = menuID })
	time.Sleep(100 * time.Millisecond)
	key(input.KeyTab, input.ModShift) // backwards away from the page: no page focus change
	keysBefore := 0
	onUI(func() { keysBefore = pageKeys })
	key(input.KeyA, 0)
	time.Sleep(200 * time.Millisecond)
	var keysAfter int
	onUI(func() { keysAfter = pageKeys })
	check("keys-stay-in-ui", keysAfter == keysBefore, "UI keys did not reach the page")
	onUI(func() { showInline = true })
	waitFor(5*time.Second, func() bool { return inlineWeb != nil && inlineWeb.Surface() != nil })

	// Window resize: the WebView follows its box.
	onUI(func() { win.SetSize(900, 600) })
	time.Sleep(500 * time.Millisecond)
	ok, d = followsBox()
	check("resize-follows", ok, d)
	shot("06-resized.png")
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
