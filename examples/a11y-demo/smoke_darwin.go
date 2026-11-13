//go:build darwin && !ios

package main

// The -smoke self-check on macOS: read our own accessibility tree like an
// assistive technology.

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/tangoui/tango"
)

type checks struct {
	failed int
}

func (c *checks) pass(name, detail string) { fmt.Printf("PASS %-12s %s\n", name, detail) }
func (c *checks) fail(name, detail string) {
	for _, s := range strings.Split(os.Getenv("TANGO_SMOKE_SKIP"), ",") {
		if strings.TrimSpace(s) == name {
			fmt.Printf("SKIP %-12s %s (TANGO_SMOKE_SKIP)\n", name, detail)
			return
		}
	}
	fmt.Printf("FAIL %-12s %s\n", name, detail)
	c.failed++
}

// ---- CoreFoundation / Accessibility API (ApplicationServices) ----

type cfRef = uintptr

var (
	axTrusted        func() bool
	axCreateApp      func(pid int32) cfRef
	axCopyAttr       func(el, attr cfRef, out *cfRef) int32
	axSetAttr        func(el, attr, value cfRef) int32
	axPerform        func(el, action cfRef) int32
	axAtPosition     func(app cfRef, x, y float32, out *cfRef) int32
	axSetTimeout     func(el cfRef, seconds float32) int32
	axValueGet       func(v cfRef, typ int32, out unsafe.Pointer) bool
	axObserverCreate func(pid int32, cb uintptr, out *cfRef) int32
	axObserverAdd    func(obs, el, note, refcon cfRef) int32
	axObserverSource func(obs cfRef) cfRef

	cfStringCreate   func(alloc cfRef, s string, enc uint32) cfRef
	cfStringGet      func(s cfRef, buf *byte, n int, enc uint32) bool
	cfGetTypeID      func(v cfRef) uintptr
	cfStringTypeID   func() uintptr
	cfArrayCount     func(a cfRef) int
	cfArrayAt        func(a cfRef, i int) cfRef
	cfNumberGet      func(n cfRef, typ int, out *int64) bool
	cfEqual          func(a, b cfRef) bool
	cfRelease        func(v cfRef)
	cfRunLoopCurrent func() cfRef
	cfRunLoopAdd     func(rl, src, mode cfRef)
	cfRunLoopRunIn   func(mode cfRef, seconds float64, returnAfterSource bool) int32

	cfBooleanTrue cfRef
	cfDefaultMode cfRef
)

const cfUTF8 = 0x08000100

func loadAX() error {
	as, err := purego.Dlopen("/System/Library/Frameworks/ApplicationServices.framework/ApplicationServices", purego.RTLD_GLOBAL)
	if err != nil {
		return err
	}
	cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_GLOBAL)
	if err != nil {
		return err
	}
	purego.RegisterLibFunc(&axTrusted, as, "AXIsProcessTrusted")
	purego.RegisterLibFunc(&axCreateApp, as, "AXUIElementCreateApplication")
	purego.RegisterLibFunc(&axCopyAttr, as, "AXUIElementCopyAttributeValue")
	purego.RegisterLibFunc(&axSetAttr, as, "AXUIElementSetAttributeValue")
	purego.RegisterLibFunc(&axPerform, as, "AXUIElementPerformAction")
	purego.RegisterLibFunc(&axAtPosition, as, "AXUIElementCopyElementAtPosition")
	purego.RegisterLibFunc(&axSetTimeout, as, "AXUIElementSetMessagingTimeout")
	purego.RegisterLibFunc(&axValueGet, as, "AXValueGetValue")
	purego.RegisterLibFunc(&axObserverCreate, as, "AXObserverCreate")
	purego.RegisterLibFunc(&axObserverAdd, as, "AXObserverAddNotification")
	purego.RegisterLibFunc(&axObserverSource, as, "AXObserverGetRunLoopSource")
	purego.RegisterLibFunc(&cfStringCreate, cf, "CFStringCreateWithCString")
	purego.RegisterLibFunc(&cfStringGet, cf, "CFStringGetCString")
	purego.RegisterLibFunc(&cfGetTypeID, cf, "CFGetTypeID")
	purego.RegisterLibFunc(&cfStringTypeID, cf, "CFStringGetTypeID")
	purego.RegisterLibFunc(&cfArrayCount, cf, "CFArrayGetCount")
	purego.RegisterLibFunc(&cfArrayAt, cf, "CFArrayGetValueAtIndex")
	purego.RegisterLibFunc(&cfNumberGet, cf, "CFNumberGetValue")
	purego.RegisterLibFunc(&cfEqual, cf, "CFEqual")
	purego.RegisterLibFunc(&cfRelease, cf, "CFRelease")
	purego.RegisterLibFunc(&cfRunLoopCurrent, cf, "CFRunLoopGetCurrent")
	purego.RegisterLibFunc(&cfRunLoopAdd, cf, "CFRunLoopAddSource")
	purego.RegisterLibFunc(&cfRunLoopRunIn, cf, "CFRunLoopRunInMode")
	cfBooleanTrue = global(cf, "kCFBooleanTrue")
	cfDefaultMode = global(cf, "kCFRunLoopDefaultMode")
	return nil
}

func global(lib uintptr, name string) cfRef {
	addr, err := purego.Dlsym(lib, name)
	if err != nil || addr == 0 {
		return 0
	}
	return **(**cfRef)(unsafe.Pointer(&addr))
}

var cfStrings = map[string]cfRef{}

func cfStr(s string) cfRef {
	if v, ok := cfStrings[s]; ok {
		return v
	}
	v := cfStringCreate(0, s, cfUTF8) // kept for the life of the program
	cfStrings[s] = v
	return v
}

func goStr(v cfRef) string {
	if v == 0 || cfGetTypeID(v) != cfStringTypeID() {
		return ""
	}
	buf := make([]byte, 1024)
	if !cfStringGet(v, &buf[0], len(buf), cfUTF8) {
		return ""
	}
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}

func attr(el cfRef, name string) cfRef {
	var v cfRef
	if axCopyAttr(el, cfStr(name), &v) != 0 {
		return 0
	}
	return v
}

func attrStr(el cfRef, name string) string {
	v := attr(el, name)
	if v == 0 {
		return ""
	}
	defer cfRelease(v)
	return goStr(v)
}

func attrInt(el cfRef, name string) (int64, bool) {
	v := attr(el, name)
	if v == 0 {
		return 0, false
	}
	defer cfRelease(v)
	var n int64
	return n, cfNumberGet(v, 4 /* kCFNumberSInt64Type */, &n)
}

// find walks the AX tree below el depth first (children retained by the
// arrays are retained again for the result).
func find(el cfRef, depth int, match func(cfRef) bool) cfRef {
	if depth > 30 {
		return 0
	}
	kids := attr(el, "AXChildren")
	if kids == 0 {
		return 0
	}
	defer cfRelease(kids)
	for i := range cfArrayCount(kids) {
		k := cfArrayAt(kids, i)
		if match(k) {
			return retainCF(k)
		}
		if r := find(k, depth+1, match); r != 0 {
			return r
		}
	}
	return 0
}

var cfRetain func(v cfRef) cfRef

func retainCF(v cfRef) cfRef {
	if cfRetain == nil {
		cf, _ := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_GLOBAL)
		purego.RegisterLibFunc(&cfRetain, cf, "CFRetain")
	}
	return cfRetain(v)
}

func waitFor(d time.Duration, cond func() bool) bool {
	end := time.Now().Add(scaled(d))
	for time.Now().Before(end) {
		if cond() {
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return cond()
}

func runSmoke(app *tango.App, s *tango.Surface) bool {
	time.Sleep(scaled(500 * time.Millisecond)) // first frames
	if err := loadAX(); err != nil {
		fmt.Println("FAIL load ApplicationServices:", err)
		return false
	}
	var c checks
	stop := pumpDisplay(app, s)
	defer stop()
	if axTrusted() && os.Getenv("A11Y_SMOKE_INPROCESS") == "" {
		fmt.Println("mode: Accessibility API (process is trusted)")
		axSmoke(&c)
	} else {
		fmt.Println("SKIP ax-api      not trusted for accessibility (or A11Y_SMOKE_INPROCESS): checking in-process NSAccessibility instead")
		inProcessSmoke(app, s, &c)
	}
	return c.failed == 0
}

// ---- AX API (out-of-process view of ourselves, from a background thread) ----

var focusNotes atomic.Int32
var focusDesc atomic.Value

func axSmoke(c *checks) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	appEl := axCreateApp(int32(os.Getpid()))
	axSetTimeout(appEl, 5)
	isButton := func(el cfRef) bool {
		return attrStr(el, "AXRole") == "AXButton" && attrStr(el, "AXDescription") == "Press me"
	}
	var btn cfRef
	// The first query turns the bridge on; the tree follows one frame later.
	waitFor(5*time.Second, func() bool { btn = find(appEl, 0, isButton); return btn != 0 })
	if btn == 0 {
		c.fail("button", fmt.Sprintf("no AXButton \"Press me\" in the window (%d frames)", frames.Load()))
		if w := attr(appEl, "AXWindows"); w != 0 {
			fmt.Printf("AXWindows: %d\n", cfArrayCount(w))
			for i := range cfArrayCount(w) {
				dump(cfArrayAt(w, i), 1)
			}
		} else {
			fmt.Println("AXWindows: none")
			dump(appEl, 10)
		}
		return
	}
	c.pass("button", "AXButton \"Press me\"")

	before := presses.Load()
	if e := axPerform(btn, cfStr("AXPress")); e != 0 {
		c.fail("press", fmt.Sprintf("AXUIElementPerformAction error %d", e))
	} else if waitFor(3*time.Second, func() bool { return presses.Load() == before+1 }) {
		c.pass("press", "AXPress clicked the button")
	} else {
		c.fail("press", "AXPress did not reach the page")
	}

	cb := find(appEl, 0, func(el cfRef) bool { return attrStr(el, "AXRole") == "AXCheckBox" })
	if cb == 0 {
		c.fail("checkbox", "no AXCheckBox")
	} else {
		v, ok := attrInt(cb, "AXValue")
		if desc := attrStr(cb, "AXDescription"); desc == "I agree" && ok && v == 0 {
			c.pass("checkbox", "AXCheckBox \"I agree\" value 0")
		} else {
			c.fail("checkbox", fmt.Sprintf("name %q value %d/%v", desc, v, ok))
		}
		cfRelease(cb)
	}

	// Hit test at the button's centre (AX coordinates: top-left origin).
	var pos [2]float64
	var size [2]float64
	if p, sz := attr(btn, "AXPosition"), attr(btn, "AXSize"); p != 0 && sz != 0 &&
		axValueGet(p, 1, unsafe.Pointer(&pos)) && axValueGet(sz, 2, unsafe.Pointer(&size)) {
		var hit cfRef
		x, y := float32(pos[0]+size[0]/2), float32(pos[1]+size[1]/2)
		if e := axAtPosition(appEl, x, y, &hit); e == 0 && hit != 0 && cfEqual(hit, btn) {
			c.pass("hittest", fmt.Sprintf("(%.0f,%.0f) is the button (frame %.0fx%.0f)", x, y, size[0], size[1]))
		} else {
			c.fail("hittest", fmt.Sprintf("error %d, got %q", e, attrStr(hit, "AXDescription")))
		}
	} else {
		c.fail("hittest", "no AXPosition/AXSize")
	}

	// Focus: observe AXFocusedUIElementChanged, then focus the button.
	var obs cfRef
	cbFn := purego.NewCallback(func(o, el, note, ref uintptr) uintptr {
		focusNotes.Add(1)
		focusDesc.Store(attrStr(el, "AXDescription"))
		return 0
	})
	if e := axObserverCreate(int32(os.Getpid()), cbFn, &obs); e != 0 {
		c.fail("focus", fmt.Sprintf("AXObserverCreate error %d", e))
		return
	}
	axObserverAdd(obs, appEl, cfStr("AXFocusedUIElementChanged"), 0)
	cfRunLoopAdd(cfRunLoopCurrent(), axObserverSource(obs), cfDefaultMode)
	if e := axSetAttr(btn, cfStr("AXFocused"), cfBooleanTrue); e != 0 {
		c.fail("focus", fmt.Sprintf("set AXFocused error %d", e))
		return
	}
	end := time.Now().Add(scaled(3 * time.Second))
	for time.Now().Before(end) && focusNotes.Load() == 0 {
		cfRunLoopRunIn(cfDefaultMode, 0.1, false)
	}
	d, _ := focusDesc.Load().(string)
	focused := attr(appEl, "AXFocusedUIElement")
	switch {
	case focusNotes.Load() > 0 && d == "Press me":
		c.pass("focus", "AXFocusedUIElementChanged for the button")
	case focused != 0 && cfEqual(focused, btn):
		c.fail("focus", "the button is AXFocusedUIElement but no notification arrived")
	default:
		c.fail("focus", fmt.Sprintf("notifications %d (%q); window not key? (locked screen)", focusNotes.Load(), d))
	}
	cfRelease(btn)
}

// ---- in-process NSAccessibility (no trust needed) ----

func msg[T any](recv objc.ID, s string, args ...any) T {
	return objc.Send[T](recv, objc.RegisterName(s), args...)
}

func nsStr(v objc.ID) string {
	if v == 0 {
		return ""
	}
	buf := make([]byte, 4*msg[int](v, "length")+1)
	if !msg[bool](v, "getCString:maxLength:encoding:", unsafe.SliceData(buf), len(buf), 4 /* NSUTF8StringEncoding */) {
		return ""
	}
	for i, ch := range buf {
		if ch == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}

// onMain runs fn on the main thread and waits.
func onMain(app *tango.App, fn func()) {
	done := make(chan struct{})
	app.Post(func() { fn(); close(done) })
	<-done
}

func canvasView(win objc.ID) objc.ID {
	var walk func(v objc.ID) objc.ID
	walk = func(v objc.ID) objc.ID {
		if v == 0 {
			return 0
		}
		if nsStr(msg[objc.ID](msg[objc.ID](v, "className"), "description")) == "TangoCanvasView" {
			return v
		}
		subs := msg[objc.ID](v, "subviews")
		for i := range msg[int](subs, "count") {
			if r := walk(msg[objc.ID](subs, "objectAtIndex:", i)); r != 0 {
				return r
			}
		}
		return 0
	}
	return walk(msg[objc.ID](win, "contentView"))
}

func axFind(el objc.ID, match func(objc.ID) bool) objc.ID {
	kids := msg[objc.ID](el, "accessibilityChildren")
	for i := range msg[int](kids, "count") {
		k := msg[objc.ID](kids, "objectAtIndex:", i)
		if match(k) {
			return k
		}
		if r := axFind(k, match); r != 0 {
			return r
		}
	}
	return 0
}

func inProcessSmoke(app *tango.App, s *tango.Surface, c *checks) {
	win := objc.ID(s.Native())
	var view objc.ID
	onMain(app, func() { view = canvasView(win) })
	if view == 0 {
		c.fail("view", "no TangoCanvasView")
		return
	}
	isButton := func(el objc.ID) bool {
		return nsStr(msg[objc.ID](el, "accessibilityRole")) == "AXButton" &&
			nsStr(msg[objc.ID](el, "accessibilityLabel")) == "Press me"
	}
	var btn objc.ID
	waitFor(5*time.Second, func() bool {
		onMain(app, func() { btn = axFind(view, isButton) })
		return btn != 0
	})
	if btn == 0 {
		var kids int
		onMain(app, func() { kids = msg[int](msg[objc.ID](view, "accessibilityChildren"), "count") })
		c.fail("button", fmt.Sprintf("no AXButton \"Press me\" among the view's %d accessibility children (%d frames)", kids, frames.Load()))
		return
	}
	c.pass("button", "AXButton \"Press me\" (in-process)")

	before := presses.Load()
	var ok bool
	onMain(app, func() { ok = msg[bool](btn, "accessibilityPerformPress") })
	if ok && waitFor(3*time.Second, func() bool { return presses.Load() == before+1 }) {
		c.pass("press", "accessibilityPerformPress clicked the button")
	} else {
		c.fail("press", fmt.Sprintf("perform returned %v, presses %d", ok, presses.Load()-before))
	}

	var cbName string
	var cbVal int
	onMain(app, func() {
		cb := axFind(view, func(el objc.ID) bool { return nsStr(msg[objc.ID](el, "accessibilityRole")) == "AXCheckBox" })
		if cb != 0 {
			cbName = nsStr(msg[objc.ID](cb, "accessibilityLabel"))
			cbVal = msg[int](msg[objc.ID](cb, "accessibilityValue"), "integerValue")
		}
	})
	if cbName == "I agree" && cbVal == 0 {
		c.pass("checkbox", "AXCheckBox \"I agree\" value 0")
	} else {
		c.fail("checkbox", fmt.Sprintf("name %q value %d", cbName, cbVal))
	}

	type rect struct{ X, Y, W, H float64 }
	type point struct{ X, Y float64 }
	var hitOK bool
	var fr rect
	onMain(app, func() {
		fr = msg[rect](btn, "accessibilityFrame")
		hit := msg[objc.ID](view, "accessibilityHitTest:", point{fr.X + fr.W/2, fr.Y + fr.H/2})
		hitOK = hit == btn
	})
	if hitOK && fr.W > 0 {
		c.pass("hittest", fmt.Sprintf("centre of frame %.0fx%.0f at (%.0f,%.0f) is the button", fr.W, fr.H, fr.X, fr.Y))
	} else {
		c.fail("hittest", fmt.Sprintf("frame %+v", fr))
	}

	onMain(app, func() {
		msg[objc.ID](win, "makeKeyAndOrderFront:", objc.ID(0))
		msg[objc.ID](btn, "setAccessibilityFocused:", true)
	})
	var focused objc.ID
	if waitFor(3*time.Second, func() bool {
		onMain(app, func() { focused = msg[objc.ID](view, "accessibilityFocusedUIElement") })
		return focused == btn
	}) {
		var f bool
		onMain(app, func() { f = msg[bool](btn, "isAccessibilityFocused") })
		if f {
			c.pass("focus", "setAccessibilityFocused: moved focus to the button")
		} else {
			c.fail("focus", "focused element is the button but isAccessibilityFocused is NO (window not key? locked screen)")
		}
	} else {
		c.fail("focus", "accessibilityFocusedUIElement is not the button")
	}
}

// dump prints the AX tree below el (failure diagnostics).
func dump(el cfRef, depth int) {
	if depth > 12 {
		return
	}
	fmt.Printf("%s%s/%s %q %q\n", strings.Repeat("  ", depth), attrStr(el, "AXRole"), attrStr(el, "AXSubrole"),
		attrStr(el, "AXDescription"), attrStr(el, "AXTitle"))
	kids := attr(el, "AXChildren")
	if kids == 0 {
		return
	}
	defer cfRelease(kids)
	for i := range cfArrayCount(kids) {
		dump(cfArrayAt(kids, i), depth+1)
	}
}

// pumpDisplay works around a display link that does not fire (locked
// screen, no active display): when no frame arrived for 300 ms it asks
// AppKit to display the canvas view, which runs a frame through
// updateLayer as a normal display would. It reports when it had to.
func pumpDisplay(app *tango.App, s *tango.Surface) (stop func()) {
	done := make(chan struct{})
	go func() {
		var view objc.ID
		onMain(app, func() { view = canvasView(objc.ID(s.Native())) })
		last, idle, noted := frames.Load(), 0, false
		t := time.NewTicker(100 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
			}
			if n := frames.Load(); n != last {
				last, idle = n, 0
				continue
			}
			if idle++; idle < 3 || view == 0 {
				continue
			}
			if !noted {
				noted = true
				fmt.Println("NOTE display link idle (locked screen?): driving frames with -displayIfNeeded / -[CALayer display]")
			}
			app.Post(func() {
				msg[objc.ID](view, "setNeedsDisplay:", true)
				msg[objc.ID](view, "displayIfNeeded")
				if frames.Load() == last {
					msg[objc.ID](msg[objc.ID](view, "layer"), "display") // runs updateLayer
				}
			})
		}
	}()
	return func() { close(done) }
}
