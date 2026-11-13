//go:build android

package android

/*
#include <stdlib.h>
#include <sys/system_properties.h>
#include "jni_android.h"

// tg_a11y_prop copies the system property name into out (at most n bytes,
// NUL-terminated) and returns its length; 0 when unset.
static int tg_a11y_prop(const char *name, char *out, int n) {
	char v[PROP_VALUE_MAX];
	int l = __system_property_get(name, v);
	if (l <= 0 || n <= 0) return 0;
	if (l >= n) l = n - 1;
	for (int i = 0; i < l; i++) out[i] = v[i];
	out[l] = 0;
	return l;
}
*/
import "C"

import (
	"time"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/internal/platform/a11ybridge"
	"github.com/tangoui/tango/ui/a11y"
)

// Accessibility of the Android canvas (docs/specs/mobile-canvas.md §5.2):
// CanvasView's AccessibilityNodeProvider (CanvasA11y.java) asks Go for
// every node synchronously (nativeA11yNode) and gets a packed record
// (a11ybridge.PackAndroidNode) built from the last tree; the root node is
// the view itself (HOST_VIEW_ID), every other node a virtual view.
// Actions come back through nativeA11yAction and go to the sink;
// content changes, focus and announcements go to Java as
// CanvasPage.a11yEvent / a11yFocus / a11yAnnounce, which queues them and
// sends AccessibilityEvents after the frame.
//
// Activation: Java reports AccessibilityManager.isEnabled() and its
// changes, and treats any getAccessibilityNodeProvider call as activation
// (so uiautomator dump works without a screen reader). The first query
// that finds no tree draws a frame synchronously (nativeA11yActive), so
// even the first dump sees the whole tree.

var (
	// CanvasHost (CanvasPage and CanvasOverlay); optional: an older Java host without them
	// keeps working without accessibility.
	mCvA11yEvent    = &jmethod{cls: &canvasHostClass, name: "a11yEvent", sig: "(II)V"}
	mCvA11yFocus    = &jmethod{cls: &canvasHostClass, name: "a11yFocus", sig: "(I)V"}
	mCvA11yAnnounce = &jmethod{cls: &canvasHostClass, name: "a11yAnnounce", sig: "(Ljava/lang/String;I)V"}
	mCvA11ySelfTest = &jmethod{cls: &canvasHostClass, name: "a11ySelfTest", sig: "(II)V"}
)

var a11yMethods = []*jmethod{mCvA11yEvent, mCvA11yFocus, mCvA11yAnnounce, mCvA11ySelfTest}

// a11yJava: the Java host has the accessibility methods.
var a11yJava bool

// a11ySelfTest is the name of the node the debug self-test clicks through
// the AccessibilityNodeProvider once it is visible (adb shell setprop
// debug.tango.a11y.click <name>, read when the app starts).
var a11ySelfTest string

// lookupCanvasA11y resolves the accessibility methods. Main thread, from
// nativeInit, after lookupCanvas.
func lookupCanvasA11y(e env) {
	if canvasClass == 0 {
		return
	}
	for _, m := range a11yMethods {
		name, sig := C.CString(m.name), C.CString(m.sig)
		m.id = C.tg_method(e.p, *m.cls, name, sig, 0)
		C.free(unsafe.Pointer(name))
		C.free(unsafe.Pointer(sig))
		if m.id == nil {
			logf(logWarn, "%v: canvas accessibility disabled", errJavaClass(m.name+m.sig))
			return
		}
	}
	a11yJava = true
	var buf [128]C.char
	pn := C.CString("debug.tango.a11y.click")
	if n := C.tg_a11y_prop(pn, &buf[0], C.int(len(buf))); n > 0 {
		a11ySelfTest = C.GoStringN(&buf[0], n)
	}
	C.free(unsafe.Pointer(pn))
}

var _ platform.CanvasAccessible = (*canvas)(nil)

// ---- platform.CanvasAccessible ----

func (c *canvas) AccessibilityActive() bool { return c.ax.active.Load() }

func (c *canvas) SetAccessibilityTree(tree *a11y.Tree) {
	if c.closed {
		return
	}
	c.ax.setTree(tree, time.Now())
	if canvasDebug > 0 {
		logf(logInfo, "canvas %d: accessibility tree with %d nodes", c.id, tree.Len())
	}
	c.a11yEvent(a11ybridge.AndroidHostID, a11ybridge.AndroidChangeSubtree)
	c.a11ySelfTestCheck()
}

func (c *canvas) UpdateAccessibilityNodes(tree *a11y.Tree, changed []a11y.NodeID) {
	if c.closed {
		return
	}
	evs := c.ax.update(tree, changed, time.Now())
	if canvasDebug > 1 {
		logf(logDebug, "canvas %d: accessibility update: %d changed, %d events", c.id, len(changed), len(evs))
	}
	for _, e := range evs {
		c.a11yEvent(e.VID, e.Changes)
	}
	c.a11ySelfTestCheck()
}

func (c *canvas) AccessibilityFocusChanged(id a11y.NodeID) {
	if c.closed {
		return
	}
	c.ax.focus = id
	vid := c.ax.vid(id)
	if a11yJava {
		withEnv(func(e env) { e.callVoid(c.page, mCvA11yFocus, jvInt(int(vid))) })
	}
}

// AccessibilityAnnounce: a live region's change (source set) is spoken by
// the screen reader from the node's live-region mode and the content
// change already sent; only ui.Context.Announce (source 0), or a source
// that is not a live node, uses View.announceForAccessibility.
func (c *canvas) AccessibilityAnnounce(text string, live a11y.Live, source a11y.NodeID) {
	if c.closed || text == "" || !a11yJava {
		return
	}
	vid := a11ybridge.AndroidNoID
	if n := c.ax.tree.Node(source); n != nil && n.Live != a11y.LiveOff {
		vid = c.ax.vid(source)
	}
	if vid != a11ybridge.AndroidNoID {
		// Make sure the region's change reaches the screen reader even
		// when nothing in the update touched the region itself.
		c.a11yEvent(vid, a11ybridge.AndroidChangeText)
		return
	}
	withEnv(func(e env) { e.callVoid(c.page, mCvA11yAnnounce, jvObj(e.str(text)), jvInt(int(live))) })
}

func (c *canvas) a11yEvent(vid int32, ch a11ybridge.AndroidChange) {
	if !a11yJava || c.closed {
		return
	}
	withEnv(func(e env) { e.callVoid(c.page, mCvA11yEvent, jvInt(int(vid)), jvInt(int(ch))) })
}

// a11ySelfTestCheck starts the debug self-test once its node is visible:
// Java posts an ACTION_CLICK through the provider (after this frame).
func (c *canvas) a11ySelfTestCheck() {
	if a11ySelfTest == "" || c.ax.selfDone {
		return
	}
	c.ax.selfTest = a11ySelfTest
	vid := c.ax.selfTestTarget()
	if vid == a11ybridge.AndroidNoID {
		return
	}
	c.ax.selfDone = true
	logf(logInfo, "canvas %d: accessibility self-test: ACTION_CLICK on %q (virtual view %d)", c.id, a11ySelfTest, vid)
	withEnv(func(e env) { e.callVoid(c.page, mCvA11ySelfTest, jvInt(int(vid)), jvInt(int(a11ybridge.AndroidClick))) })
}

// ---- JNI ----

// nativeA11yActive: an assistive technology started (on) or stopped
// reading the canvas. With prime (a query found no tree yet) a frame is
// drawn synchronously so the answer can include the
// whole tree; the scheduler's answer goes back to Java as for the other
// frame calls.
//
//export Java_org_tango_host_TangoCanvas_nativeA11yActive
func Java_org_tango_host_TangoCanvas_nativeA11yActive(penv *C.JNIEnv, cls C.jclass, id C.jlong, on, prime C.jboolean, uptime C.jlong) C.jlong {
	c := canvasFor(id)
	if c == nil {
		return 0
	}
	want := on != 0
	was := c.ax.active.Swap(want)
	if was != want {
		if canvasDebug > 0 || want {
			logf(logInfo, "canvas %d: accessibility %s", c.id, map[bool]string{true: "active", false: "inactive"}[want])
		}
		c.Invalidate()
	}
	if !want || prime == 0 || c.ax.tree != nil || c.inFrame || c.win == nil {
		return 0
	}
	now := time.Now()
	s := c.sched.SyncFrame()
	if s.Frame {
		c.frame()
	}
	return encodeAction(s, int64(uptime)+time.Since(now).Milliseconds(), time.Now())
}

// nativeA11yNode returns the packed record of virtual view vid
// (a11ybridge.PackAndroidNode), or null when it is not in the tree.
//
//export Java_org_tango_host_TangoCanvas_nativeA11yNode
func Java_org_tango_host_TangoCanvas_nativeA11yNode(penv *C.JNIEnv, cls C.jclass, id C.jlong, vid C.jint) C.jobject {
	c := canvasFor(id)
	if c == nil {
		return 0
	}
	b := c.ax.node(int32(vid))
	if b == nil {
		return 0
	}
	return env{penv}.bytes(b)
}

// nativeA11yHit returns the virtual view at (x, y), device pixels in the
// view (touch exploration), or AndroidNoID.
//
//export Java_org_tango_host_TangoCanvas_nativeA11yHit
func Java_org_tango_host_TangoCanvas_nativeA11yHit(penv *C.JNIEnv, cls C.jclass, id C.jlong, x, y C.jfloat) C.jint {
	c := canvasFor(id)
	if c == nil {
		return C.jint(a11ybridge.AndroidNoID)
	}
	return C.jint(c.ax.hit(float32(x), float32(y), c.density))
}

// nativeA11yFocus returns the virtual view with input focus, or
// AndroidNoID.
//
//export Java_org_tango_host_TangoCanvas_nativeA11yFocus
func Java_org_tango_host_TangoCanvas_nativeA11yFocus(penv *C.JNIEnv, cls C.jclass, id C.jlong) C.jint {
	c := canvasFor(id)
	if c == nil {
		return C.jint(a11ybridge.AndroidNoID)
	}
	return C.jint(c.ax.focusVID())
}

// nativeA11yAction performs an action (a11ybridge.AndroidAction) on
// virtual view vid: text is ACTION_SET_TEXT's argument, progress
// ACTION_SET_PROGRESS's. It reports whether the core accepted it.
//
//export Java_org_tango_host_TangoCanvas_nativeA11yAction
func Java_org_tango_host_TangoCanvas_nativeA11yAction(penv *C.JNIEnv, cls C.jclass, id C.jlong, vid, act C.jint, text C.jstring, progress C.jfloat) C.jboolean {
	c := canvasFor(id)
	if c == nil || c.inFrame {
		return C.JNI_FALSE
	}
	as, ok := c.sink.(platform.CanvasAccessibleSink)
	if !ok {
		return C.JNI_FALSE
	}
	nid, a, arg, ok := c.ax.action(int32(vid), a11ybridge.AndroidAction(act), env{penv}.gostr(text), float64(progress))
	if !ok {
		if canvasDebug > 0 {
			logf(logInfo, "canvas %d: accessibility action %#x on virtual view %d refused", c.id, int(act), int(vid))
		}
		return C.JNI_FALSE
	}
	err := as.AccessibilityAction(nid, a, arg)
	if canvasDebug > 0 || a11ySelfTest != "" {
		logf(logInfo, "canvas %d: accessibility action %v on node %d (virtual view %d): %v", c.id, a, nid, int(vid), err)
	}
	return jbool(err == nil)
}
