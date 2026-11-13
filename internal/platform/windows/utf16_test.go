package windows

import (
	"runtime"
	"testing"
	"unicode/utf16"
	"unsafe"
)

func TestUTF16RoundTrip(t *testing.T) {
	for _, s := range []string{"", "hello", "你好，世界", "emoji 😀 + 𝄞", "tab\tnew\r\nline"} {
		z := utf16z(s)
		if z[len(z)-1] != 0 {
			t.Fatalf("%q: not NUL-terminated", s)
		}
		if got := utf16Units(z); got != s {
			t.Errorf("utf16Units(utf16z(%q)) = %q", s, got)
		}
		p := uintptr(unsafe.Pointer(&z[0]))
		if got := wstr(p); got != s {
			t.Errorf("wstr = %q, want %q", got, s)
		}
		if got := wstrN(p, len(z)); got != s {
			t.Errorf("wstrN = %q, want %q", got, s)
		}
		runtime.KeepAlive(z)
	}
}

func TestUTF16Details(t *testing.T) {
	if z := utf16z("a\x00b"); len(z) != 3 || z[0] != 'a' || z[1] != 'b' {
		t.Errorf("embedded NUL not dropped: %v", z)
	}
	z := utf16z("😀")
	if len(z) != 3 || !utf16.IsSurrogate(rune(z[0])) {
		t.Errorf("surrogate pair: %v", z)
	}
	if got := utf16Units([]uint16{'h', 'i', 0, 'x'}); got != "hi" {
		t.Errorf("stops at NUL: %q", got)
	}
	if got := utf16Units([]uint16{'h', 'i'}); got != "hi" {
		t.Errorf("no NUL: %q", got)
	}
	// Clipboard data sized in bytes may be cut mid-string: wstrN bounds.
	buf := []uint16{'a', 'b', 'c', 'd'}
	if got := wstrN(uintptr(unsafe.Pointer(&buf[0])), 2); got != "ab" {
		t.Errorf("wstrN bound: %q", got)
	}
	runtime.KeepAlive(buf)
	if wstr(0) != "" || wstrN(0, 5) != "" {
		t.Error("nil pointer must read as empty")
	}
}

func TestParseGUID(t *testing.T) {
	g, ok := parseGUID("dc1c5a9c-e88a-4dde-a5a1-60f82a20aef7")
	if !ok || g.Data1 != 0xdc1c5a9c || g.Data2 != 0xe88a || g.Data3 != 0x4dde ||
		g.Data4 != [8]byte{0xa5, 0xa1, 0x60, 0xf8, 0x2a, 0x20, 0xae, 0xf7} {
		t.Errorf("parseGUID = %+v %v", g, ok)
	}
	if g2, ok := parseGUID("{DC1C5A9C-E88A-4DDE-A5A1-60F82A20AEF7}"); !ok || g2 != g {
		t.Errorf("braced/upper = %+v %v", g2, ok)
	}
	for _, bad := range []string{"", "dc1c5a9c-e88a-4dde-a5a1-60f82a20aef", "zc1c5a9c-e88a-4dde-a5a1-60f82a20aef7", "dc1c5a9ce88a-4dde-a5a1-60f82a20aef7x"} {
		if _, ok := parseGUID(bad); ok {
			t.Errorf("parseGUID(%q) accepted", bad)
		}
	}
	if unsafe.Sizeof(GUID{}) != 16 {
		t.Error("GUID must be 16 bytes")
	}
}

func TestTopFrameOnly(t *testing.T) {
	got := topFrameOnly(nativeShim)
	want := "if (window === window.top) {\n" + nativeShim + "\n}"
	if got != want {
		t.Errorf("topFrameOnly = %q", got)
	}
	if nativeShim != `window.__tangoNative={post:s=>chrome.webview.postMessage(String(s))};` {
		t.Errorf("shim changed: %s", nativeShim)
	}
}
