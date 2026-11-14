//go:build linux && !android

package linux

// Window test of embedded web views (webembed_linux.go) on a real GTK and
// WebKitGTK; skips without a display (see canvas_gl_linux_test.go):
//
//	xvfb-run -a -s "-screen 0 1280x800x24" go test ./internal/platform/linux/ -run Embedded

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/tangoui/tango/internal/platform"
)

var embedTestFFI struct {
	once          sync.Once
	err           error
	getAllocation func(w ptr, a *gdkRect)
	getMapped     func(w ptr) bool
	isFocus       func(w ptr) bool
	getChildren   func(c ptr) ptr
	parentWindow  func(w ptr) ptr
	windowPos     func(win ptr, x, y *int32)
	listFree      func(l ptr)
}

func loadEmbedTestFFI() error {
	f := &embedTestFFI
	f.once.Do(func() {
		var bd binder
		t := libGTK
		bd.req(&f.getAllocation, "gtk_widget_get_allocation", t)
		bd.req(&f.getMapped, "gtk_widget_get_mapped", t)
		bd.req(&f.isFocus, "gtk_widget_is_focus", t)
		bd.req(&f.getChildren, "gtk_container_get_children", t)
		bd.req(&f.listFree, "g_list_free", libGLib)
		bd.req(&f.parentWindow, "gtk_widget_get_parent_window", t)
		bd.req(&f.windowPos, "gdk_window_get_position", canvasFFI.gdk, t)
		if len(bd.missing) > 0 {
			f.err = fmt.Errorf("missing native symbols: %v", bd.missing)
		}
	})
	return f.err
}

// overlayIndex is the position of w among the overlay's children (-1).
func overlayIndex(overlay, w ptr) int {
	f := &embedTestFFI
	l := f.getChildren(overlay)
	defer f.listFree(l)
	i := 0
	for n := l; n != 0; n = (*gListNode)(toPtr(n)).next {
		if (*gListNode)(toPtr(n)).data == w {
			return i
		}
		i++
	}
	return -1
}

func TestEmbeddedWebView(t *testing.T) {
	needGTK(t)
	if err := loadEmbedTestFFI(); err != nil {
		t.Fatal(err)
	}
	f := &embedTestFFI
	s, c, _ := newGLTestCanvas(t)
	ps := s.(*surface)
	const id platform.SurfaceID = 9001

	var (
		mu     sync.Mutex
		events []platform.SurfaceEvent // for id, then the parent's SurfaceClosed
		order  []platform.SurfaceID    // ids of SurfaceClosed events
	)
	onMain(func() {
		gtkTest.host.onEvent = func(sid platform.SurfaceID, ev platform.SurfaceEvent) {
			mu.Lock()
			defer mu.Unlock()
			if sid == id {
				events = append(events, ev)
			}
			if ev.Kind == platform.SurfaceClosed {
				order = append(order, sid)
			}
		}
	})
	t.Cleanup(func() { onMain(func() { gtkTest.host.onEvent = nil }) })
	has := func(k platform.SurfaceEventKind) bool {
		mu.Lock()
		defer mu.Unlock()
		for _, ev := range events {
			if ev.Kind == k {
				return true
			}
		}
		return false
	}

	var (
		ev  platform.EmbeddedWebView
		err error
	)
	onMain(func() {
		ev, err = gtkTest.b.NewEmbeddedWebView(id, ps.id, platform.SurfaceOptions{
			URL: "data:text/html,<p>embedded</p>",
		}, platform.WebOptions{UserAgent: "tango-embed-test"})
	})
	if err != nil {
		t.Fatalf("NewEmbeddedWebView: %v", err)
	}
	e := ev.(*embeddedWeb)
	var mapped bool
	onMain(func() { mapped = f.getMapped(e.layout) })
	if mapped {
		t.Fatal("embedded view shown before any placement")
	}
	if !waitMain(30*time.Second, func() bool { return has(platform.SurfaceLoaded) }) {
		t.Fatal("no SurfaceLoaded for the embedded id")
	}

	// Frame partly above the canvas: clipped at the top.
	onMain(func() {
		e.SetPlacement(platform.WebPlacement{
			Frame: platform.RectF{X: 20, Y: -30, W: 200, H: 150},
			Clip:  platform.RectF{X: 0, Y: 0, W: 320, H: 240},
		})
	})
	// The overlay puts the layout in a child window of its own at the
	// clip's position; the layout's allocation is relative to it.
	var la, wa gdkRect
	var cx, cy int32
	ok := waitMain(5*time.Second, func() bool {
		f.getAllocation(e.layout, &la)
		f.getAllocation(e.w.web, &wa)
		if cw := f.parentWindow(e.layout); cw != 0 {
			f.windowPos(cw, &cx, &cy)
		}
		return f.getMapped(e.layout) && cx == 20 && cy == 0 && la.Width == 200 && la.Height == 120 &&
			wa.Width == 200 && wa.Height == 150 && wa.Y == -30
	})
	if !ok {
		t.Fatalf("clip window at %d,%d, layout %+v, web view %+v; want clip window at 20,0, layout 200×120, web view 200×150 at y -30", cx, cy, la, wa)
	}
	if w, h := e.Size(); w != 200 || h != 150 {
		t.Errorf("Size %d×%d, want 200×150", w, h)
	}

	// Focus round trip.
	onMain(e.FocusWeb)
	if !waitMain(3*time.Second, func() bool { return f.isFocus(e.w.web) }) {
		t.Error("FocusWeb: the web view is not the focus widget")
	}
	onMain(e.BlurWeb)
	if !waitMain(3*time.Second, func() bool { return f.isFocus(c.widget) }) {
		t.Error("BlurWeb: the canvas is not the focus widget")
	}

	// Hide wins over the placement; Show restores it.
	onMain(e.Hide)
	if !waitMain(3*time.Second, func() bool { return !f.getMapped(e.layout) }) {
		t.Error("Hide: still mapped")
	}
	onMain(e.Show)
	if !waitMain(3*time.Second, func() bool { return f.getMapped(e.layout) }) {
		t.Error("Show: not mapped")
	}

	// A GLArea switched on later stays below the web view.
	var area ptr
	onMain(func() { area, err = c.UseGLArea() })
	if err == nil && area != 0 {
		var ia, il int
		onMain(func() { ia, il = overlayIndex(c.glOverlay, area), overlayIndex(c.glOverlay, e.layout) })
		if ia < 0 || il < 0 || ia > il {
			t.Errorf("overlay order: GLArea %d, embedded layout %d; want the GLArea first", ia, il)
		}
	} else {
		t.Logf("UseGLArea: %v (order not checked)", err)
	}

	// Closing the parent closes the embedded view first.
	onMain(s.Close)
	mu.Lock()
	defer mu.Unlock()
	if len(order) < 2 || order[0] != id || order[1] != ps.id {
		t.Errorf("SurfaceClosed order %v, want [%d %d]", order, id, ps.id)
	}
	if !e.w.closed {
		t.Error("embedded view not closed with its parent")
	}
}
