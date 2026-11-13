package linux

import (
	"reflect"
	"testing"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

func TestStateFromGdk(t *testing.T) {
	cases := []struct {
		flags uint32
		want  platform.WindowState
	}{
		{0, platform.StateNormal},
		{gdkStateMaximized, platform.StateMaximized},
		{gdkStateFullscreen, platform.StateFullscreen},
		{gdkStateFullscreen | gdkStateMaximized, platform.StateFullscreen},
		{gdkStateIconified | gdkStateMaximized, platform.StateMinimized},
		{1 << 7, platform.StateNormal}, // FOCUSED only
	}
	for _, c := range cases {
		if got := stateFromGdk(c.flags); got != c.want {
			t.Errorf("stateFromGdk(%#x) = %v, want %v", c.flags, got, c.want)
		}
	}
}

func TestStateOps(t *testing.T) {
	cases := []struct {
		cur  uint32
		want platform.WindowState
		ops  []stateOp
	}{
		{0, platform.StateNormal, nil},
		{0, platform.StateMaximized, []stateOp{opMaximize}},
		{0, platform.StateMinimized, []stateOp{opIconify}},
		{0, platform.StateFullscreen, []stateOp{opFullscreen}},
		{gdkStateMaximized, platform.StateNormal, []stateOp{opUnmaximize}},
		{gdkStateMaximized, platform.StateMaximized, nil},
		{gdkStateIconified | gdkStateMaximized, platform.StateMaximized, []stateOp{opDeiconify}},
		{gdkStateIconified, platform.StateNormal, []stateOp{opDeiconify}},
		{gdkStateFullscreen, platform.StateMaximized, []stateOp{opUnfullscreen, opMaximize}},
		{gdkStateFullscreen | gdkStateMaximized, platform.StateNormal, []stateOp{opUnfullscreen, opUnmaximize}},
		{gdkStateIconified, platform.StateMinimized, nil},
	}
	for _, c := range cases {
		if got := stateOps(c.cur, c.want); !reflect.DeepEqual(got, c.ops) {
			t.Errorf("stateOps(%#x, %v) = %v, want %v", c.cur, c.want, got, c.ops)
		}
	}
}

func TestResizeEdge(t *testing.T) {
	want := map[string]int32{
		"nw": 0, "n": 1, "ne": 2, "w": 3, "e": 4, "sw": 5, "s": 6, "se": 7, "SE": 7,
	}
	for in, w := range want {
		got, ok := resizeEdge(in)
		if !ok || got != w {
			t.Errorf("resizeEdge(%q) = %d,%v want %d", in, got, ok, w)
		}
	}
	for _, bad := range []string{"", "x", "nn", "north"} {
		if _, ok := resizeEdge(bad); ok {
			t.Errorf("resizeEdge(%q) accepted", bad)
		}
	}
}

func TestGeometryHints(t *testing.T) {
	if unsafe.Sizeof(gdkGeometry{}) != 56 {
		t.Fatalf("gdkGeometry size %d, want 56", unsafe.Sizeof(gdkGeometry{}))
	}
	if g, m := geometryHints(0, 0, 0, 0); m != 0 || g != (gdkGeometry{}) {
		t.Errorf("no limits: %+v %#x", g, m)
	}
	g, m := geometryHints(200, 100, 0, 0)
	if m != gdkHintMinSize || g.MinWidth != 200 || g.MinHeight != 100 {
		t.Errorf("min only: %+v %#x", g, m)
	}
	g, m = geometryHints(0, 0, 800, 0)
	if m != gdkHintMaxSize || g.MaxWidth != 800 || g.MaxHeight != noLimit {
		t.Errorf("max width only: %+v %#x", g, m)
	}
	g, m = geometryHints(500, 300, 400, 600)
	if m != gdkHintMinSize|gdkHintMaxSize || g.MaxWidth != 500 || g.MaxHeight != 600 {
		t.Errorf("max below min must clamp: %+v %#x", g, m)
	}
}

func TestRGBAColor(t *testing.T) {
	got := rgbaColor(0xff800000)
	if got[0] != 1 || got[1] != 128.0/255 || got[2] != 0 || got[3] != 0 {
		t.Errorf("rgbaColor = %v", got)
	}
	if rgbaColor(0x000000ff)[3] != 1 {
		t.Error("alpha")
	}
}

func TestFileURIToPath(t *testing.T) {
	ok := map[string]string{
		"file:///home/u/a.txt":                "/home/u/a.txt",
		"file://localhost/tmp/x":              "/tmp/x",
		"file:///home/u/My%20Doc%E4%B8%AD.md": "/home/u/My Doc中.md",
		"  file:///tmp/trim\r\n":              "/tmp/trim",
		"FILE:///tmp/upper":                   "/tmp/upper",
	}
	for in, want := range ok {
		got, good := fileURIToPath(in)
		if !good || got != want {
			t.Errorf("fileURIToPath(%q) = %q,%v want %q", in, got, good, want)
		}
	}
	for _, bad := range []string{"", "http://x/y", "file://remote/tmp/x", "file:relative", "/tmp/x"} {
		if p, good := fileURIToPath(bad); good {
			t.Errorf("fileURIToPath(%q) accepted as %q", bad, p)
		}
	}
}

func TestExtTable(t *testing.T) {
	tb := newExtTable()
	var seen [7]uintptr
	id := tb.add(func(a [7]uintptr) uintptr { seen = a; return 1 })
	if tb.call(id, [7]uintptr{1, 2, 3, 4, 5, 6, 7}) != 1 || seen[6] != 7 {
		t.Fatalf("call: %v", seen)
	}
	tb.remove(id)
	if tb.call(id, [7]uintptr{}) != 0 || tb.size() != 0 {
		t.Fatal("removed handler still live")
	}
	if id2 := tb.add(func([7]uintptr) uintptr { return 0 }); id2 == id {
		t.Fatal("id reused")
	}
}

func TestScreenOffset(t *testing.T) {
	// Secondary 1920x1080 left of a 2560x1440 primary: GDK puts the
	// secondary at 0,180 and the primary at 1920,0.
	geos := []platform.Rect{{X: 0, Y: 180, Width: 1920, Height: 1080}, {X: 1920, Y: 0, Width: 2560, Height: 1440}}
	o := primaryOffset(geos, 1)
	if o != (screenOffset{1920, 0}) {
		t.Fatalf("primaryOffset = %+v", o)
	}
	if got := o.rect(geos[0]); got != (platform.Rect{X: -1920, Y: 180, Width: 1920, Height: 1080}) {
		t.Errorf("secondary in contract space = %+v", got)
	}
	if got := o.rect(geos[1]); got != (platform.Rect{Width: 2560, Height: 1440}) {
		t.Errorf("primary in contract space = %+v", got)
	}
	if x, y := o.toContract(2020, 50); x != 100 || y != 50 {
		t.Errorf("toContract = %d,%d", x, y)
	}
	if x, y := o.toGdk(-100, 300); x != 1820 || y != 300 {
		t.Errorf("toGdk = %d,%d", x, y)
	}
	// No primary (Wayland) or bad index: monitor 0; no monitors: zero.
	if o := primaryOffset(geos, -1); o != (screenOffset{0, 180}) {
		t.Errorf("fallback = %+v", o)
	}
	if o := primaryOffset(geos, 5); o != (screenOffset{0, 180}) {
		t.Errorf("out of range = %+v", o)
	}
	if o := primaryOffset(nil, 0); o != (screenOffset{}) {
		t.Errorf("empty = %+v", o)
	}
}
