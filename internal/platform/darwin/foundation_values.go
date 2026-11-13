//go:build darwin && !ios

package darwin

// Conversions between Go values and Foundation objects (NSString, NSData,
// NSURL, NSArray). Every constructor here returns an autoreleased object, so
// callers must be inside an autorelease pool (withPool or the AppKit loop).

import (
	"runtime"
	"unsafe"
)

// Encoding constants: NSUTF8StringEncoding (Foundation) and
// kCFStringEncodingUTF8 (CoreFoundation) name the same encoding with
// different numbering schemes.
const (
	foundationUTF8 = 4
	cfUTF8         = 0x08000100
)

// nsString converts s to an autoreleased NSString. It goes through
// CFStringCreateWithBytes, which takes an explicit length, so strings with
// embedded NULs survive; CFStringRef is toll-free bridged with NSString.
func nsString(s string) id {
	created := cfStringFromBytes(0, unsafe.StringData(s), len(s), cfUTF8, false)
	runtime.KeepAlive(s)
	return autorelease(id(created))
}

// goString returns the UTF-8 contents of an NSString. Unpaired UTF-16
// surrogates, which have no UTF-8 form, are replaced instead of turning the
// whole string into "".
func goString(str id) string {
	if str == 0 {
		return ""
	}
	encoded := send(str, "dataUsingEncoding:allowLossyConversion:", foundationUTF8, 1)
	return string(goBytes(encoded))
}

// nsData wraps a copy of b in an autoreleased NSData.
func nsData(b []byte) id {
	var first *byte
	if len(b) != 0 {
		first = unsafe.SliceData(b)
	}
	obj := send(send(class("NSData"), "alloc"), "initWithBytes:length:", uintptr(unsafe.Pointer(first)), uintptr(len(b)))
	runtime.KeepAlive(b)
	return autorelease(obj)
}

// goBytes copies an NSData's contents into Go memory with -getBytes:length:,
// so no pointer into Objective-C storage is ever held.
func goBytes(data id) []byte {
	if data == 0 {
		return nil
	}
	buf := make([]byte, sendInt(data, "length"))
	if len(buf) > 0 {
		send(data, "getBytes:length:", uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	}
	return buf
}

// urlVia builds an NSURL with one of its string-taking class factories.
func urlVia(factory, text string) id {
	return send(class("NSURL"), factory, uintptr(nsString(text)))
}

func nsURL(s string) id      { return urlVia("URLWithString:", s) }
func fileURL(path string) id { return urlVia("fileURLWithPath:", path) }

// nsArray builds a mutable array from objs in a single
// +arrayWithObjects:count: call (the C-array form, so no nil sentinel).
func nsArray(objs ...id) id {
	if len(objs) == 0 {
		return send(class("NSMutableArray"), "array")
	}
	arr := send(class("NSMutableArray"), "arrayWithObjects:count:", uintptr(unsafe.Pointer(&objs[0])), uintptr(len(objs)))
	runtime.KeepAlive(objs)
	return arr
}

// arrayItems walks an NSArray (or any NSFastEnumeration collection that offers
// -objectEnumerator) in order.
func arrayItems(arr id) []id {
	if arr == 0 {
		return nil
	}
	items := make([]id, 0, sendInt(arr, "count"))
	it := send(arr, "objectEnumerator")
	for obj := send(it, "nextObject"); obj != 0; obj = send(it, "nextObject") {
		items = append(items, obj)
	}
	return items
}
