//go:build android

package android

/*
#cgo LDFLAGS: -llog
#include <stdlib.h>
#include "jni_android.h"
#include "webview_ext_android.h"
*/
import "C"

import (
	"errors"
	"runtime"
	"unicode/utf16"
	"unsafe"
)

// Java classes and methods Go calls. Looked up once in nativeInit, on the
// main thread, where FindClass sees the app's class loader (threads Go
// attaches later only see the system one).
const (
	tangoClassName = "org/tango/host/Tango"
	pageClassName  = "org/tango/host/Page"
)

var tangoClass, pageClass, stringClass C.jclass

type jmethod struct {
	cls    *C.jclass
	static bool
	name   string
	sig    string
	id     C.jmethodID
}

var (
	// Tango (static).
	mWake           = &jmethod{cls: &tangoClass, static: true, name: "wake", sig: "()V"}
	mQuit           = &jmethod{cls: &tangoClass, static: true, name: "quit", sig: "()V"}
	mExit           = &jmethod{cls: &tangoClass, static: true, name: "exit", sig: "()V"}
	mNewPage        = &jmethod{cls: &tangoClass, static: true, name: "newPage", sig: "(JLjava/lang/String;ZZLjava/lang/String;)Lorg/tango/host/Page;"}
	mClipboardRead  = &jmethod{cls: &tangoClass, static: true, name: "clipboardRead", sig: "()Ljava/lang/String;"}
	mClipboardWrite = &jmethod{cls: &tangoClass, static: true, name: "clipboardWrite", sig: "(Ljava/lang/String;)V"}
	mMessageDialog  = &jmethod{cls: &tangoClass, static: true, name: "messageDialog", sig: "(IILjava/lang/String;Ljava/lang/String;[Ljava/lang/String;)V"}
	mOpenDocuments  = &jmethod{cls: &tangoClass, static: true, name: "openDocuments", sig: "(ILjava/lang/String;[Ljava/lang/String;Z)V"}
	mLaneSupported  = &jmethod{cls: &tangoClass, static: true, name: "laneSupported", sig: "()Z"}

	// Page (instance).
	mShow     = &jmethod{cls: &pageClass, name: "show", sig: "()V"}
	mHide     = &jmethod{cls: &pageClass, name: "hide", sig: "()V"}
	mClose    = &jmethod{cls: &pageClass, name: "close", sig: "()V"}
	mFocus    = &jmethod{cls: &pageClass, name: "focus", sig: "()V"}
	mSetTitle = &jmethod{cls: &pageClass, name: "setTitle", sig: "(Ljava/lang/String;)V"}
	mNavigate = &jmethod{cls: &pageClass, name: "navigate", sig: "(Ljava/lang/String;)V"}
	mEval     = &jmethod{cls: &pageClass, name: "eval", sig: "(Ljava/lang/String;)V"}
	mWebView  = &jmethod{cls: &pageClass, name: "webView", sig: "()Landroid/webkit/WebView;"}
	mRespond  = &jmethod{cls: &pageClass, name: "respond", sig: "(ILjava/lang/String;Ljava/lang/String;Ljava/lang/String;[Ljava/lang/String;[BJJ)Landroid/webkit/WebResourceResponse;"}
	mLaneSend = &jmethod{cls: &pageClass, name: "laneSend", sig: "([B)Z"}
)

var allMethods = []*jmethod{
	mWake, mQuit, mExit, mNewPage, mClipboardRead, mClipboardWrite, mMessageDialog, mOpenDocuments, mLaneSupported,
	mShow, mHide, mClose, mFocus, mSetTitle, mNavigate, mEval, mWebView, mRespond, mLaneSend,
}

var errJava = errors.New("tango/android: Java call failed (see logcat)")

// env is the JNIEnv of the current thread.
type env struct{ p *C.JNIEnv }

// lookupJava resolves the classes and methods above. Main thread, from
// nativeInit.
func lookupJava(e env) error {
	for _, c := range []struct {
		dst  *C.jclass
		name string
	}{{&tangoClass, tangoClassName}, {&pageClass, pageClassName}, {&stringClass, "java/lang/String"}} {
		n := C.CString(c.name)
		*c.dst = C.tg_find_class(e.p, n)
		C.free(unsafe.Pointer(n))
		if *c.dst == 0 {
			return errors.New("tango/android: Java class " + c.name + " not found")
		}
	}
	for _, m := range allMethods {
		name, sig := C.CString(m.name), C.CString(m.sig)
		st := C.int(0)
		if m.static {
			st = 1
		}
		m.id = C.tg_method(e.p, *m.cls, name, sig, st)
		C.free(unsafe.Pointer(name))
		C.free(unsafe.Pointer(sig))
		if m.id == nil {
			return errors.New("tango/android: Java method " + m.name + m.sig + " not found")
		}
	}
	return nil
}

// withEnv runs fn with this thread's JNIEnv inside a local reference frame,
// attaching a Go-created thread to the VM for the duration.
func withEnv(fn func(e env)) bool {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var attached C.int
	p := C.tg_env(&attached)
	if p == nil {
		return false
	}
	if attached != 0 {
		defer C.tg_detach()
	}
	if C.tg_push_frame(p, 32) == 0 {
		C.tg_exception(p)
		return false
	}
	defer C.tg_pop_frame(p)
	fn(env{p})
	return true
}

func onMainThread() bool { return C.tg_is_main_thread() != 0 }

// ---- values ----

func jvObj(o C.jobject) C.jvalue {
	var v C.jvalue
	*(*C.jobject)(unsafe.Pointer(&v)) = o
	return v
}

func jvLong(i int64) C.jvalue {
	var v C.jvalue
	*(*C.jlong)(unsafe.Pointer(&v)) = C.jlong(i)
	return v
}

func jvInt(i int) C.jvalue {
	var v C.jvalue
	*(*C.jint)(unsafe.Pointer(&v)) = C.jint(i)
	return v
}

func jvBool(b bool) C.jvalue {
	var v C.jvalue
	if b {
		*(*C.jboolean)(unsafe.Pointer(&v)) = C.JNI_TRUE
	}
	return v
}

// str makes a Java string. Strings cross as UTF-16.
func (e env) str(s string) C.jobject {
	u := utf16.Encode([]rune(s))
	var p *C.jchar
	if len(u) > 0 {
		p = (*C.jchar)(unsafe.Pointer(&u[0]))
	}
	return C.jobject(C.tg_new_string(e.p, p, C.jsize(len(u))))
}

// strOrNull is str, with "" as null.
func (e env) strOrNull(s string) C.jobject {
	if s == "" {
		return 0
	}
	return e.str(s)
}

func (e env) gostr(s C.jstring) string {
	if s == 0 {
		return ""
	}
	n := C.tg_string_length(e.p, s)
	if n <= 0 {
		return ""
	}
	u := make([]uint16, int(n))
	C.tg_string_chars(e.p, s, n, (*C.jchar)(unsafe.Pointer(&u[0])))
	return string(utf16.Decode(u))
}

func (e env) strs(ss []string) C.jobject {
	a := C.tg_new_string_array(e.p, stringClass, C.jsize(len(ss)))
	for i, s := range ss {
		js := e.str(s)
		C.tg_array_set(e.p, a, C.jsize(i), js)
		C.tg_delete_local(e.p, js)
	}
	return C.jobject(a)
}

// gostrs reads a String[]; nil for a null array.
func (e env) gostrs(a C.jobjectArray) []string {
	if a == 0 {
		return nil
	}
	n := int(C.tg_array_length(e.p, C.jarray(a)))
	out := make([]string, n)
	for i := range out {
		js := C.tg_array_get(e.p, a, C.jsize(i))
		out[i] = e.gostr(C.jstring(js))
		C.tg_delete_local(e.p, js)
	}
	return out
}

// bytes makes a byte[]; nil for a nil slice.
func (e env) bytes(b []byte) C.jobject {
	if b == nil {
		return 0
	}
	var p unsafe.Pointer
	if len(b) > 0 {
		p = unsafe.Pointer(&b[0])
	}
	return C.jobject(C.tg_new_bytes(e.p, p, C.jsize(len(b))))
}

// gobytes copies a byte[] into a new Go slice; nil for a null array.
func (e env) gobytes(a C.jbyteArray) []byte {
	if a == 0 {
		return nil
	}
	n := int(C.tg_array_length(e.p, C.jarray(a)))
	b := make([]byte, n)
	if n > 0 {
		C.tg_get_bytes(e.p, a, C.jsize(n), unsafe.Pointer(&b[0])) // webview_ext_android.c
	}
	return b
}

// deleteLocal frees a local reference before the frame ends (in loops:
// withEnv's frame holds 32).
func (e env) deleteLocal(o C.jobject) { C.tg_delete_local(e.p, o) }

// ---- calls ----

func argp(args []C.jvalue) *C.jvalue {
	if len(args) == 0 {
		return nil
	}
	return &args[0]
}

func (e env) check() error {
	if C.tg_exception(e.p) != 0 {
		return errJava
	}
	return nil
}

func (e env) callStaticVoid(m *jmethod, args ...C.jvalue) error {
	C.tg_call_static_void(e.p, *m.cls, m.id, argp(args))
	return e.check()
}

func (e env) callStaticObject(m *jmethod, args ...C.jvalue) (C.jobject, error) {
	o := C.tg_call_static_object(e.p, *m.cls, m.id, argp(args))
	if err := e.check(); err != nil {
		return 0, err
	}
	return o, nil
}

func (e env) callStaticBoolean(m *jmethod, args ...C.jvalue) (bool, error) {
	r := C.tg_call_static_boolean(e.p, *m.cls, m.id, argp(args))
	if err := e.check(); err != nil {
		return false, err
	}
	return r != 0, nil
}

func (e env) callBoolean(obj C.jobject, m *jmethod, args ...C.jvalue) (bool, error) {
	if obj == 0 {
		return false, errJava
	}
	r := C.tg_call_boolean(e.p, obj, m.id, argp(args))
	if err := e.check(); err != nil {
		return false, err
	}
	return r != 0, nil
}

func (e env) callVoid(obj C.jobject, m *jmethod, args ...C.jvalue) error {
	if obj == 0 {
		return errJava
	}
	C.tg_call_void(e.p, obj, m.id, argp(args))
	return e.check()
}

func (e env) callObject(obj C.jobject, m *jmethod, args ...C.jvalue) (C.jobject, error) {
	if obj == 0 {
		return 0, errJava
	}
	o := C.tg_call_object(e.p, obj, m.id, argp(args))
	if err := e.check(); err != nil {
		return 0, err
	}
	return o, nil
}

func (e env) global(o C.jobject) C.jobject { return C.tg_new_global(e.p, o) }
func (e env) deleteGlobal(o C.jobject)     { C.tg_delete_global(e.p, o) }

// ---- logging ----

func init() {
	logWrite = func(prio int, msg string) {
		c := C.CString(msg)
		C.tg_log(C.int(prio), c)
		C.free(unsafe.Pointer(c))
	}
}
