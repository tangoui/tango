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

// main runs fn on the main thread and waits.
func onMain(fn func()) {
	tango.Await(app, func() struct{} { fn(); runner.Invalidate(); return struct{}{} })
}

// waitFor polls cond on the main thread until it holds or timeout.
func waitFor(timeout time.Duration, cond func() bool) bool {
	end := time.Now().Add(timeout)
	for time.Now().Before(end) {
		if tango.Await(app, cond) {
			return true
		}
		time.Sleep(30 * time.Millisecond)
	}
	return false
}

func shot(name string) {
	var b tango.Rect
	var ch int
	onMain(func() { b = win.Bounds(); _, ch = win.Size() })
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
	onMain(func() {
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
	onMain(func() { win.Focus() })

	check("loaded", waitFor(15*time.Second, func() bool { return loaded["main"] && loaded["inline"] }), "both pages loaded")
	time.Sleep(300 * time.Millisecond)

	var distinct bool
	onMain(func() {
		a, b := mainWeb.Surface(), inlineWeb.Surface()
		distinct = a != nil && b != nil && a != b && uihost.EmbeddedIn(a) == win && uihost.EmbeddedIn(b) == win
	})
	check("two-webviews", distinct, "two embedded surfaces in the window")

	followsBox := func() (bool, string) {
		var ok bool
		var d string
		onMain(func() {
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
	onMain(func() { mainWeb.Surface().Eval(`document.getElementById('btn').click()`) })
	check("page-to-go", waitFor(3*time.Second, func() bool { return pageClicks == 1 }), "page button click reached Go")

	// Scroll the list: the inline WebView moves and is clipped.
	onMain(func() { scrollReq = 520 })
	time.Sleep(300 * time.Millisecond)
	shot("02-scrolled-clipped.png")
	onMain(func() { scrollReq = 2000 })
	time.Sleep(300 * time.Millisecond)
	var w2, h2 int
	onMain(func() { w2, h2 = inlineWeb.Surface().Size() })
	check("scrolled-out", w2 > 0 && h2 > 0, fmt.Sprintf("inline still alive %dx%d while scrolled out", w2, h2))
	shot("03-scrolled-out.png")
	onMain(func() { scrollReq = 0 })

	// A dropdown over the main WebView freezes it.
	onMain(func() { menuOpen = true })
	check("menu-freezes", waitFor(3*time.Second, func() bool { return mainWeb.Frozen() }), "main WebView frozen under the menu")
	time.Sleep(200 * time.Millisecond)
	shot("04-menu-over-webview.png")
	onMain(func() { menuOpen = false })
	check("menu-thaws", waitFor(3*time.Second, func() bool { return !mainWeb.Frozen() }), "native view back after closing")

	// A modal covers both.
	onMain(func() { modalOpen = true })
	check("modal-freezes", waitFor(3*time.Second, func() bool { return mainWeb.Frozen() && inlineWeb.Frozen() }), "both frozen under the dialog")
	time.Sleep(400 * time.Millisecond)
	shot("05-modal.png")
	onMain(func() { modalOpen = false })
	waitFor(3*time.Second, func() bool { return !mainWeb.Frozen() && !inlineWeb.Frozen() })

	// Focus: Tab from the menu button enters the page (inline view hidden
	// so the main view is the next Tab stop) ...
	onMain(func() { showInline = false })
	check("inline-closed", waitFor(3*time.Second, func() bool { return inlineClosed }), "removing the box closes its WebView")
	onMain(func() { focusReq = menuID })
	time.Sleep(200 * time.Millisecond)
	key(input.KeyTab, 0)
	check("tab-into-page", waitFor(3*time.Second, func() bool { return pageFocus == "btn" }), "page focused its first element: "+pageFocus)
	// ... and Tab after the last field comes back to the UI, after the box.
	onMain(func() {
		mainWeb.Surface().Eval(`document.getElementById('second').focus();document.activeElement.dispatchEvent(new KeyboardEvent('keydown',{key:'Tab',bubbles:true,cancelable:true}))`)
	})
	check("tab-out-of-page", waitFor(3*time.Second, func() bool { return afterFocused }), "UI focus on the control after the WebView")
	onMain(func() { focusReq = menuID })
	time.Sleep(100 * time.Millisecond)
	key(input.KeyTab, input.ModShift) // backwards away from the page: no page focus change
	keysBefore := 0
	onMain(func() { keysBefore = pageKeys })
	key(input.KeyA, 0)
	time.Sleep(200 * time.Millisecond)
	var keysAfter int
	onMain(func() { keysAfter = pageKeys })
	check("keys-stay-in-ui", keysAfter == keysBefore, "UI keys did not reach the page")
	onMain(func() { showInline = true })
	waitFor(5*time.Second, func() bool { return inlineWeb != nil && inlineWeb.Surface() != nil })

	// Window resize: the WebView follows its box.
	onMain(func() { win.SetSize(900, 600) })
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
