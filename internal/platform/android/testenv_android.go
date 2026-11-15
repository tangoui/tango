//go:build android

package android

/*
#include <stdlib.h>
#include "jni_android.h"
*/
import "C"

import (
	"os"
	"strings"
	"unsafe"
)

// applyTestEnv copies the TANGO_TEST_ENV launch extra into the environment
// (test hook): a string array of KEY=VALUE entries, passed with
// am start ... --esa TANGO_TEST_ENV TANGO_A=1,TANGO_B=2 (am splits on
// commas, so values cannot contain one). Only keys starting with TANGO_
// are taken. An Android process cannot be given environment variables,
// hence the extra, as for TANGO_DEV_URL (applyDevURL).
//
// It does nothing in builds without the self-test hooks (testHooks), and
// the Java side (Mobile.testEnv) answers only in debuggable APKs. Main
// thread, from nativeInit, after applyDevURL and before main.main runs.
func applyTestEnv() {
	if !testHooks || mobileClass == 0 {
		return
	}
	var kvs []string
	withEnv(func(e env) {
		name, sig := C.CString("testEnv"), C.CString("()[Ljava/lang/String;")
		m := C.tg_method(e.p, mobileClass, name, sig, 1)
		C.free(unsafe.Pointer(name))
		C.free(unsafe.Pointer(sig))
		if m == nil {
			return
		}
		jm := &jmethod{cls: &mobileClass, static: true, name: "testEnv", id: m}
		if o, err := e.callStaticObject(jm); err == nil {
			kvs = e.gostrs(C.jobjectArray(o))
		}
	})
	for _, kv := range kvs {
		k, v, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(k, "TANGO_") {
			logf(logWarn, "TANGO_TEST_ENV: ignored %q (want TANGO_KEY=value)", kv)
			continue
		}
		os.Setenv(k, v)
		logf(logInfo, "TANGO_TEST_ENV: %s=%s", k, v)
	}
}
