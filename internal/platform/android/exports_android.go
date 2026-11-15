//go:build android

package android

/*
#include <stdlib.h>
#include "jni_android.h"
*/
import "C"

import (
	"bufio"
	"context"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe" // also for go:linkname

	"github.com/tangoui/tango/internal/platform"
)

// JNI entry points: the native methods of org.tango.host.Tango. All but
// nativeServeAsset and the stream calls run on the Android main thread.

// userMain is the program's main.main, which a c-shared library never runs
// by itself: nativeInit starts it, so the app's main package is the same as
// on desktop.
//
//go:linkname userMain main.main
func userMain()

var (
	hostStarted atomic.Bool
	mainG       atomic.Uint64
)

// MainGoroutine returns the id of the goroutine running main.main, or 0
// before the host started it.
func MainGoroutine() uint64 { return mainG.Load() }

//export Java_org_tango_host_Tango_nativeInit
func Java_org_tango_host_Tango_nativeInit(penv *C.JNIEnv, cls C.jclass) {
	if hostStarted.Load() {
		return
	}
	e := env{penv}
	if C.tg_init(penv) == 0 {
		logf(logError, "GetJavaVM failed")
		return
	}
	if err := lookupJava(e); err != nil {
		logf(logError, "%v", err)
		return
	}
	if err := lookupMobile(e); err != nil {
		logf(logWarn, "%v: mobile capabilities disabled", err)
	}
	if err := lookupWebExt(e); err != nil { // webview_ext_android.go
		logf(logWarn, "%v: advanced WebView features disabled", err)
	}
	lookupCanvas(e)     // canvas_android.go
	lookupCanvasA11y(e) // canvas_a11y_android.go
	hostStarted.Store(true)
	redirectStdio()
	applyDevURL()  // devurl_android.go
	applyTestEnv() // testenv_android.go
	go func() {
		mainG.Store(goroutineID())
		defer withEnv(func(e env) { e.callStaticVoid(mExit) })
		userMain()
	}()
}

//export Java_org_tango_host_Tango_nativeDrain
func Java_org_tango_host_Tango_nativeDrain(penv *C.JNIEnv, cls C.jclass) {
	backend.stepSpins = 0
	backend.wakePending.Store(false)
	backend.drain()
}

//export Java_org_tango_host_Tango_nativeLifecycle
func Java_org_tango_host_Tango_nativeLifecycle(penv *C.JNIEnv, cls C.jclass, ev C.jint) {
	b := backend
	evs := b.life.report(platform.LifecycleEvent(ev))
	if h := b.host(); h != nil {
		b.deliver(h, evs)
	}
}

//export Java_org_tango_host_Tango_nativeShouldClose
func Java_org_tango_host_Tango_nativeShouldClose(penv *C.JNIEnv, cls C.jclass, id C.jlong) C.jboolean {
	h := backend.host()
	if h == nil || h.ShouldClose(platform.SurfaceID(id)) {
		return C.JNI_TRUE
	}
	return C.JNI_FALSE
}

//export Java_org_tango_host_Tango_nativeSurfaceEvent
func Java_org_tango_host_Tango_nativeSurfaceEvent(penv *C.JNIEnv, cls C.jclass, id C.jlong, kind, w, h C.jint, url, errMsg C.jstring) {
	e := env{penv}
	b := backend
	sid := platform.SurfaceID(id)
	ev := platform.SurfaceEvent{
		Kind:   platform.SurfaceEventKind(kind),
		Width:  int(w),
		Height: int(h),
		URL:    e.gostr(url),
		Error:  e.gostr(errMsg),
	}
	s := b.surface(sid)
	if s == nil {
		return
	}
	switch ev.Kind {
	case platform.SurfaceResized:
		s.w, s.h = ev.Width, ev.Height
	case platform.SurfaceClosed:
		b.mu.Lock()
		delete(b.surfaces, sid)
		b.mu.Unlock()
		s.dropOverlays(e) // canvas_attach_android.go
		s.release()
		bodies.dropSurface(sid)
	}
	if hst := b.host(); hst != nil {
		hst.OnSurfaceEvent(sid, ev)
	}
}

// ---- the bridge (token.go) ----

// nativeBeginPage is called when a main-frame document starts loading
// (Page.Client.onPageStarted, main thread). The previous document's token
// stops working. It stores in script[0] the page script for the host to
// inject into a trusted document Go did not serve with the script inside,
// or leaves it null. It returns whether the host keeps its lane reply
// proxy (beginPage, lanes.go): only when the new document's accepted probe
// came before this call.
//
//export Java_org_tango_host_Tango_nativeBeginPage
func Java_org_tango_host_Tango_nativeBeginPage(penv *C.JNIEnv, cls C.jclass, id C.jlong, url C.jstring, script C.jobjectArray) C.jboolean {
	e := env{penv}
	s := backend.surface(platform.SurfaceID(id))
	if s == nil {
		return C.JNI_FALSE
	}
	tok, keep := beginPage(s.tokens, s.lanes, e.gostr(url))
	if tok != "" && script != 0 && C.tg_array_length(e.p, C.jarray(script)) > 0 {
		js := e.str(pageScript(tok, s.bridge, len(s.laneCaps) > 0))
		C.tg_array_set(e.p, script, 0, js)
		e.deleteLocal(js)
	}
	if keep {
		return C.JNI_TRUE
	}
	return C.JNI_FALSE
}

// nativeLanePacket handles an upstream lane message (an ArrayBuffer) from
// the page's WebMessageListener (main thread; main frame only). It returns
// 0 to drop it silently (token or origin mismatch, no lanes), 1 when the
// core took it (for a probe the host keeps the reply proxy and answers
// [0]) and 2 when the core refused it (the host answers [1]). See
// handleUp, lanes.go.
//
//export Java_org_tango_host_Tango_nativeLanePacket
func Java_org_tango_host_Tango_nativeLanePacket(penv *C.JNIEnv, cls C.jclass, id C.jlong, sourceOrigin C.jstring, data C.jbyteArray) C.jint {
	e := env{penv}
	s := backend.surface(platform.SurfaceID(id))
	if s == nil {
		return 0
	}
	return C.jint(s.lanePacket(backend.host(), e.gostr(sourceOrigin), e.gobytes(data)))
}

// nativeTokenOrigin returns the origin of the document token was issued
// to, or null if it is not a current token. Called by the
// JavascriptInterface on the JavaBridge thread, so the origin is fixed
// when the message arrives.
//
//export Java_org_tango_host_Tango_nativeTokenOrigin
func Java_org_tango_host_Tango_nativeTokenOrigin(penv *C.JNIEnv, cls C.jclass, id C.jlong, token C.jstring) C.jstring {
	e := env{penv}
	s := backend.surface(platform.SurfaceID(id))
	if s == nil {
		return 0
	}
	origin, ok := s.tokens.origin(e.gostr(token))
	if !ok {
		logf(logWarn, "dropped a message without the current page token")
		return 0
	}
	return C.jstring(e.str(origin))
}

// nativeMessage delivers a message whose origin nativeTokenOrigin
// established. Main thread.
//
//export Java_org_tango_host_Tango_nativeMessage
func Java_org_tango_host_Tango_nativeMessage(penv *C.JNIEnv, cls C.jclass, id C.jlong, origin, msg C.jstring) {
	e := env{penv}
	b := backend
	sid := platform.SurfaceID(id)
	s := b.surface(sid)
	h := b.host()
	if s == nil || h == nil || s.closed {
		return
	}
	h.OnMessage(sid, e.gostr(origin), e.gostr(msg))
}

// nativeWebMessage handles a message from the page's WebMessageListener
// (main thread). The host only passes main-frame messages; sourceOrigin is
// the sender's origin as the WebView reports it, which must be the origin
// the token was issued for. It returns the reply to post back, or null.
//
//export Java_org_tango_host_Tango_nativeWebMessage
func Java_org_tango_host_Tango_nativeWebMessage(penv *C.JNIEnv, cls C.jclass, id C.jlong, sourceOrigin, data C.jstring) C.jstring {
	e := env{penv}
	b := backend
	sid := platform.SurfaceID(id)
	s := b.surface(sid)
	h := b.host()
	if s == nil || s.closed {
		return 0
	}
	m, ok := s.tokens.check(e.gostr(sourceOrigin), e.gostr(data))
	if !ok {
		return 0
	}
	switch m.kind {
	case wireMessage:
		if !m.valid {
			logf(logWarn, "dropped a message without the current page token or from another origin")
			return 0
		}
		if h != nil {
			h.OnMessage(sid, m.origin, m.payload)
		}
		return 0
	default: // wireBody
		bodyID, ctype, b64, ok := parseWireBody(m.payload)
		if !ok {
			return 0
		}
		stored := m.valid && stashBody(sid, bodyID, ctype, b64)
		// Always answer: the page's request waits for it.
		if stored {
			return C.jstring(e.str("b1" + bodyID))
		}
		return C.jstring(e.str("b0" + bodyID))
	}
}

// ---- assets (WebView request thread) ----

// streams holds responses WebView is still reading, by handle.
var streams struct {
	sync.Mutex
	next int64
	m    map[int64]*assetResult
}

func addStream(res *assetResult) int64 {
	streams.Lock()
	defer streams.Unlock()
	if streams.m == nil {
		streams.m = map[int64]*assetResult{}
	}
	streams.next++
	streams.m[streams.next] = res
	return streams.next
}

func getStream(h int64) *assetResult {
	streams.Lock()
	defer streams.Unlock()
	return streams.m[h]
}

func closeStream(h int64) {
	streams.Lock()
	res := streams.m[h]
	delete(streams.m, h)
	streams.Unlock()
	if res != nil {
		res.closeStream()
	}
}

//export Java_org_tango_host_Tango_nativeServeAsset
func Java_org_tango_host_Tango_nativeServeAsset(penv *C.JNIEnv, cls C.jclass, page C.jobject, id C.jlong,
	method, url C.jstring, headers C.jobjectArray, mainFrame C.jboolean) C.jobject {
	e := env{penv}
	b := backend
	sid := platform.SurfaceID(id)
	rawURL := e.gostr(url)
	s := b.surface(sid)
	token, script := "", ""
	if mainFrame != 0 && s != nil {
		if token = s.tokens.serve(rawURL); token != "" {
			script = pageScript(token, s.bridge, len(s.laneCaps) > 0)
		}
	}
	var res assetResult
	ctx, cancel := context.WithCancel(context.Background())
	hdrs, body, hasBody := bodies.takeBody(sid, e.gostrs(headers))
	r, err := newAssetRequest(ctx, e.gostr(method), rawURL, hdrs)
	if err == nil && hasBody {
		setBody(r, body)
	}
	h := b.host()
	switch {
	case err != nil:
		cancel()
		res = errorResult(400, err.Error())
	case h == nil:
		cancel()
		res = errorResult(503, "tango: app not running")
	default:
		res = serveAsset(func(w http.ResponseWriter, r *http.Request) { h.ServeAsset(sid, w, r) }, r, cancel, script)
	}
	var handle int64
	if res.Stream != nil {
		handle = addStream(&res)
	}
	if res.Injected && token != "" {
		s.tokens.served(token, rawURL)
	}
	resp, err := e.callObject(page, mRespond,
		jvInt(res.Status), jvObj(e.str(res.Reason)), jvObj(e.str(res.MIME)), jvObj(e.strOrNull(res.Encoding)),
		jvObj(e.strs(res.Headers)), jvObj(e.bytes(nonNil(res.Body, res.Stream == nil))), jvLong(handle), jvLong(res.Length))
	if err != nil {
		if handle != 0 {
			closeStream(handle)
		}
		return 0
	}
	return resp
}

// nativeStashBody stores the body of a request the page is about to send
// to the asset origin (body.go). Called on the WebView's JavaBridge thread
// by Page.Channel.body, synchronously from the page's fetch/XHR wrapper,
// when the host uses a JavascriptInterface.
//
//export Java_org_tango_host_Tango_nativeStashBody
func Java_org_tango_host_Tango_nativeStashBody(penv *C.JNIEnv, cls C.jclass, id C.jlong, token, bodyID, ctype, b64 C.jstring) C.jboolean {
	e := env{penv}
	sid := platform.SurfaceID(id)
	s := backend.surface(sid)
	if s == nil {
		return C.JNI_FALSE
	}
	if _, ok := s.tokens.origin(e.gostr(token)); !ok || !stashBody(sid, e.gostr(bodyID), e.gostr(ctype), e.gostr(b64)) {
		return C.JNI_FALSE
	}
	return C.JNI_TRUE
}

func stashBody(sid platform.SurfaceID, bodyID, ctype, b64 string) bool {
	if !bodies.stashBase64(sid, bodyID, ctype, b64) {
		logf(logWarn, "request body refused (over %d MiB per request or %d MiB pending)", maxStashedBody>>20, maxStashedTotal>>20)
		return false
	}
	return true
}

// nonNil keeps an empty whole body from turning into a null byte[] (which
// Page.respond reads as "streamed").
func nonNil(b []byte, whole bool) []byte {
	if whole && b == nil {
		return []byte{}
	}
	return b
}

//export Java_org_tango_host_Tango_nativeStreamRead
func Java_org_tango_host_Tango_nativeStreamRead(penv *C.JNIEnv, cls C.jclass, handle C.jlong, buf C.jbyteArray, off, n C.jint) C.jint {
	res := getStream(int64(handle))
	if res == nil || n <= 0 {
		return -1
	}
	tmp := make([]byte, int(n))
	k, eof := res.Stream.Read(tmp)
	if k > 0 {
		C.tg_set_bytes(penv, buf, off, C.jsize(k), unsafe.Pointer(&tmp[0]))
		return C.jint(k)
	}
	if eof {
		closeStream(int64(handle))
	}
	return -1
}

//export Java_org_tango_host_Tango_nativeStreamClose
func Java_org_tango_host_Tango_nativeStreamClose(penv *C.JNIEnv, cls C.jclass, handle C.jlong) {
	closeStream(int64(handle))
}

// ---- dialogs ----

//export Java_org_tango_host_Tango_nativeDialogDone
func Java_org_tango_host_Tango_nativeDialogDone(penv *C.JNIEnv, cls C.jclass, cb, button C.jint, paths C.jobjectArray, errMsg C.jstring) {
	e := env{penv}
	b := backend
	fn := b.callbacks[int(cb)]
	delete(b.callbacks, int(cb))
	if fn == nil {
		return
	}
	var ps []string
	if paths != 0 {
		ps = e.gostrs(paths)
		if ps == nil {
			ps = []string{}
		}
	}
	fn(int(button), ps, e.gostr(errMsg))
}

// ---- stdio ----

// redirectStdio sends the process' stdout and stderr (fmt.Print, the log
// package, C code) to logcat, line by line, where Android otherwise
// discards them.
func redirectStdio() {
	var fds [2]int
	if err := syscall.Pipe2(fds[:], syscall.O_CLOEXEC); err != nil {
		return
	}
	for _, fd := range []int{1, 2} {
		if err := syscall.Dup3(fds[1], fd, 0); err != nil {
			logf(logWarn, "redirect fd %d: %v", fd, err)
		}
	}
	syscall.Close(fds[1])
	r := os.NewFile(uintptr(fds[0]), "tango-stdio")
	go func() {
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 4096), 64<<10)
		for sc.Scan() {
			logWrite(logInfo, sc.Text())
		}
	}()
}
