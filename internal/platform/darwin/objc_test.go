//go:build darwin && !ios

package darwin

import (
	"bytes"
	"runtime"
	"testing"
	"unsafe"
)

// TestFoundationRoundTrip exercises the Go <-> Foundation conversions and the
// selector / class caches without touching AppKit UI state.
func TestFoundationRoundTrip(t *testing.T) {
	load()
	withPool(func() {
		for _, s := range []string{"", "hello", "中文 ✓ 🎉", "a\x00b"} {
			if got := goString(nsString(s)); got != s {
				t.Errorf("string round trip %q -> %q", s, got)
			}
		}
		if nsString("\xff\xfe") != 0 {
			t.Error("invalid UTF-8 should yield nil")
		}
		for _, b := range [][]byte{nil, {}, {0, 1, 2, 255}} {
			got := goBytes(nsData(b))
			if !bytes.Equal(got, b) || got == nil {
				t.Errorf("data round trip %v -> %v", b, got)
			}
		}
		a, b2 := nsString("x"), nsString("y")
		items := arrayItems(nsArray(a, b2, a))
		if len(items) != 3 || items[0] != a || items[1] != b2 || items[2] != a {
			t.Errorf("array round trip = %v", items)
		}
		if n := len(arrayItems(nsArray())); n != 0 {
			t.Errorf("empty array has %d items", n)
		}
		if got := goString(send(nsURL("https://example.com/a?b=1"), "absoluteString")); got != "https://example.com/a?b=1" {
			t.Errorf("nsURL = %q", got)
		}
		if got := goString(send(fileURL("/tmp/x y"), "path")); got != "/tmp/x y" {
			t.Errorf("fileURL = %q", got)
		}
		if sel("length") != sel("length") || class("NSString") != class("NSString") {
			t.Error("caches are not stable")
		}
		if hasClass("TangoNoSuchClassXYZ") || !hasClass("NSObject") {
			t.Error("hasClass")
		}
		if !respondsTo(a, "length") || respondsTo(a, "tangoNoSuchMethod") || respondsTo(0, "length") {
			t.Error("respondsTo")
		}
		if !sendBool(a, "isEqualToString:", uintptr(nsString("x"))) || sendInt(nsString("abc"), "length") != 3 {
			t.Error("sendBool / sendInt")
		}
		// More than three arguments takes the slice path in send.
		raw := []byte("four-args")
		r := send(send(class("NSString"), "alloc"), "initWithBytesNoCopy:length:encoding:freeWhenDone:",
			uintptr(unsafe.Pointer(&raw[0])), uintptr(len(raw)), foundationUTF8, boolArg(false))
		if got := goString(r); got != "four-args" {
			t.Errorf("4-arg send = %q", got)
		}
		release(r)
		runtime.KeepAlive(raw)
		obj := alloc("NSObject")
		if retain(obj) != obj {
			t.Error("retain")
		}
		release(obj)
		release(obj)
		if kCFRunLoopCommonModes == 0 || goString(id(kCFRunLoopCommonModes)) != "kCFRunLoopCommonModes" {
			t.Error("constPtr kCFRunLoopCommonModes")
		}
	})
}
