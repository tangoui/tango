package ui

import (
	"fmt"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// moreHasLabel reports whether a node labelled s exists in the last build.
func moreHasLabel(n *node, s string) bool {
	if n == nil {
		return false
	}
	if n.label == s {
		return true
	}
	for _, k := range n.kids {
		if moreHasLabel(k, s) {
			return true
		}
	}
	return false
}

func moreBuilt(c *Context, s string) bool {
	if moreHasLabel(c.root, s) {
		return true
	}
	for _, o := range c.overlays {
		if moreHasLabel(o.root, s) {
			return true
		}
	}
	return false
}

func TestMoreInfiniteScrollLoads(t *testing.T) {
	items := 3
	fires := 0
	loading, done, sync := false, false, true
	d := driver(func(c *Context) {
		Column(c, func() {
			inf := InfiniteScroll(c, func() {
				for i := 0; i < items; i++ {
					Key(c, i, func() { Text(c, fmt.Sprintf("条目 %d", i)).Layout(layout.H(40)) })
				}
			}).Height(200).Distance(20).Loading(loading).Done(done)
			if inf.NeedMore() {
				fires++
				if sync {
					items += 10
				} else {
					loading = true
				}
			}
		}, layout.Pad(20))
	})
	// Frame 1 has no geometry yet; frame 2 sees an unfilled viewport.
	d.Frame()
	d.Frame()
	if fires != 1 || items != 13 {
		t.Fatalf("initial fill: fires=%d items=%d", fires, items)
	}
	d.Frame()
	d.Frame()
	if fires != 1 {
		t.Fatalf("fired without scrolling: %d", fires)
	}
	in := geom.Pt(100, 100)
	d.Scroll(in, 1000)
	if fires != 2 || items != 23 {
		t.Fatalf("bottom: fires=%d items=%d", fires, items)
	}
	// Async: NeedMore flips loading on; the footer shows a spinner and no
	// further requests are made while loading.
	sync = false
	d.Scroll(in, 1000)
	d.Frame()
	if fires != 3 || !loading {
		t.Fatalf("async: fires=%d loading=%v", fires, loading)
	}
	if !moreBuilt(d.r.c, "加载中...") {
		t.Fatal("loading footer missing")
	}
	d.Scroll(in, 1000)
	if fires != 3 {
		t.Fatal("fired while loading")
	}
	// Done: "no more" footer, no requests.
	loading, done = false, true
	d.Frame()
	d.Scroll(in, 1000)
	if fires != 3 || !moreBuilt(d.r.c, "没有更多了") || moreBuilt(d.r.c, "加载中...") {
		t.Fatalf("done: fires=%d", fires)
	}
	d.SetTheme(theme.ElementPlus())
	d.Settle()
	if !moreBuilt(d.r.c, "没有更多了") {
		t.Fatal("element footer missing")
	}
}

func TestMoreInfiniteScrollDisabledAndRearm(t *testing.T) {
	fires := 0
	disabled := true
	d := driver(func(c *Context) {
		inf := InfiniteScroll(c, func() {
			for i := 0; i < 20; i++ {
				Key(c, i, func() { Text(c, "行").Layout(layout.H(30)) })
			}
		}, layout.H(150)).Disabled(disabled)
		if inf.NeedMore() {
			fires++
		}
	})
	d.Frame()
	d.Scroll(geom.Pt(50, 50), 2000)
	if fires != 0 {
		t.Fatal("disabled fired")
	}
	disabled = false
	d.Frame()
	if fires != 1 {
		t.Fatalf("enabled at bottom: %d", fires)
	}
	// Same content height: scrolling away and back re-arms the trigger.
	d.Scroll(geom.Pt(50, 50), 2000)
	if fires != 1 {
		t.Fatal("fired twice without re-arm")
	}
	d.Scroll(geom.Pt(50, 50), -300)
	d.Scroll(geom.Pt(50, 50), 300)
	if fires != 2 {
		t.Fatalf("re-arm: %d", fires)
	}
}
