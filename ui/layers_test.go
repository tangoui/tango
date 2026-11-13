package ui

import (
	"sync"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// stackPage builds two dismissible overlays the way a page builds a Modal
// and then a popup on top of it: the lower one checks for dismissal before
// the upper one even exists in this build.
func stackPage(openA, openB *bool) func(*Context) {
	return func(c *Context) {
		Column(c, func() { Button(c, "页面") }, layout.Pad(20))
		if *openA {
			a := Overlay(c, OverlayOptions{Place: ScreenCenter}, func() {
				Button(c, "A内")
				Div(c, layout.Size(300, 160))
			}, layout.Pad(8))
			if c.Dismissed(a.ID()) {
				*openA = false
			}
		}
		if *openB {
			b := Overlay(c, OverlayOptions{Place: ScreenTop, Margin: 8}, func() {
				Button(c, "B内")
			}, layout.Pad(8))
			if c.Dismissed(b.ID()) {
				*openB = false
			}
		}
	}
}

func TestLayerEscapeClosesTopFirst(t *testing.T) {
	openA, openB := true, true
	d := driver(stackPage(&openA, &openB))
	d.Settle()
	d.Key(input.KeyEscape, 0)
	if !openA || openB {
		t.Fatalf("first Escape: A=%v B=%v, want only B closed", openA, openB)
	}
	d.Key(input.KeyEscape, 0)
	if openA {
		t.Fatal("second Escape did not close A")
	}
}

func TestLayerPressOnUpperIsNotOutsideLower(t *testing.T) {
	openA, openB := true, true
	d := driver(stackPage(&openA, &openB))
	d.Settle()
	a, b := mustFind(t, d, "A内"), mustFind(t, d, "B内")
	if a.Intersect(b.Expand(8)).W > 0 {
		t.Fatalf("test setup: panels overlap %v %v", a, b)
	}
	d.Click(b.Center()) // inside B, outside A's rectangle
	if !openA || !openB {
		t.Fatalf("press in upper layer: A=%v B=%v", openA, openB)
	}
	d.Click(a.Center()) // inside A: lower layer, so outside B
	if !openA || openB {
		t.Fatalf("press in lower layer: A=%v B=%v", openA, openB)
	}
	d.Click(geom.Pt(5, 395)) // on the page
	if openA {
		t.Fatal("press on page did not close A")
	}
}

func TestLayerModalThenConfirm(t *testing.T) {
	open := true
	var ref *ConfirmRef
	d := driver(func(c *Context) {
		Modal(c, &open, "对话框").Body(func() {
			if Button(c, "删除").Clicked() {
				ref = Confirm(c, ConfirmOptions{Title: "确认删除"})
			}
		})
	})
	d.Settle()
	d.ClickLabel("删除")
	if ref == nil || !ref.IsOpen() {
		t.Fatal("confirm not open")
	}
	d.Key(input.KeyEscape, 0)
	if ref.IsOpen() || !open {
		t.Fatalf("Escape: confirm open=%v modal open=%v", ref.IsOpen(), open)
	}
	d.Key(input.KeyEscape, 0)
	if open {
		t.Fatal("second Escape did not close the modal")
	}
}

func TestGlobalLayerDeferredRuns(t *testing.T) {
	ran := 0
	saved := globalLayers
	globalLayers = append(append([]func(*Context){}, saved...), func(c *Context) {
		c.deferred = append(c.deferred, func() { ran++ })
	})
	t.Cleanup(func() { globalLayers = saved })
	d := driver(func(c *Context) {})
	d.Frame()
	if ran == 0 {
		t.Fatal("deferred work of a global layer did not run")
	}
}

func TestInvalidatorFromGoroutines(t *testing.T) {
	var inv func()
	d := driver(func(c *Context) {
		if inv == nil {
			inv = c.Invalidator()
		}
		Text(c, "x")
	})
	d.Frame()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				inv()
				d.Runner().Invalidate()
				Invalidate(d.Runner().Context().Window())
			}
		}()
	}
	for i := 0; i < 20; i++ {
		d.Frame()
		_ = d.NeedsFrame()
	}
	wg.Wait()
	inv()
	if !d.NeedsFrame() {
		t.Fatal("Invalidator did not request a frame")
	}
}

func TestDriverFindText(t *testing.T) {
	open := true
	d := driver(func(c *Context) {
		Column(c, func() {
			Title(c, "标题", 3)
			Paragraph(c, "第一段落的长文字")
		}, layout.Pad(20))
		if open {
			Overlay(c, OverlayOptions{Place: ScreenCenter}, func() { Text(c, "浮层文字") })
		}
	})
	d.Settle()
	if _, ok := d.Find("标题"); ok {
		t.Fatal("Find should only see interactive widgets")
	}
	r, ok := d.FindText("标题")
	if !ok || r.Y < 20 || r.H <= 0 {
		t.Fatalf("FindText(标题) = %v %v", r, ok)
	}
	if r, ok := d.FindText("长文字"); !ok || r.W <= 0 {
		t.Fatalf("substring match failed: %v %v", r, ok)
	}
	if r, ok := d.FindText("浮层文字"); !ok || r.Center().X < 250 {
		t.Fatalf("overlay text: %v %v", r, ok)
	}
	if _, ok := d.FindText("没有"); ok {
		t.Fatal("found missing text")
	}
}

func TestOverlaySidePlacementFirstFrame(t *testing.T) {
	for _, tc := range []struct {
		place Placement
		check func(a, p geom.Rect) bool
	}{
		{Right, func(a, p geom.Rect) bool { return p.X == a.Right()+6 && layNear(p.Center().Y, a.Center().Y) }},
		{Left, func(a, p geom.Rect) bool { return p.Right() == a.X-6 && layNear(p.Center().Y, a.Center().Y) }},
		{RightEnd, func(a, p geom.Rect) bool { return p.X == a.Right()+6 && layNear(p.Bottom(), a.Bottom()) }},
		{LeftEnd, func(a, p geom.Rect) bool { return p.Right() == a.X-6 && layNear(p.Bottom(), a.Bottom()) }},
	} {
		var anchor, panel ID
		d := driver(func(c *Context) {
			b := Div(c, layout.Abs(250, 150, layout.Unset, layout.Unset), layout.Size(60, 30))
			anchor = b.ID()
			panel = Overlay(c, OverlayOptions{Anchor: anchor, Place: tc.place, Offset: 6}, func() {
				Div(c, layout.Size(80, 120))
			}).ID()
		})
		d.Frame() // a single frame: placement must not depend on last frame's panel size
		a, p := d.r.c.rects[anchor], d.r.c.rects[panel]
		if !tc.check(a, p) {
			t.Errorf("place %d: anchor %v panel %v", tc.place, a, p)
		}
	}
	// Flips to the other side when there is no room.
	var panel ID
	d := driver(func(c *Context) {
		b := Div(c, layout.Abs(540, 150, layout.Unset, layout.Unset), layout.Size(50, 30))
		panel = Overlay(c, OverlayOptions{Anchor: b.ID(), Place: Right, Offset: 6, Flip: true}, func() {
			Div(c, layout.Size(80, 40))
		}).ID()
	})
	d.Frame()
	if p := d.r.c.rects[panel]; p.Right() != 534 {
		t.Fatalf("Right did not flip: %v", p)
	}
}

func TestPopoverRightCenteredWithoutExtraFrame(t *testing.T) {
	open := true
	d := driver(func(c *Context) {
		Column(c, func() {
			Popover(c).Trigger("click").Placement("right").Open(&open).Content(func() {
				Div(c, layout.Size(100, 120))
			}).Body(func() { Button(c, "触发") })
		}, layout.Pad(150))
	})
	d.Frame()
	if d.r.Animating() {
		t.Fatal("popover asked for a settling frame")
	}
	a := mustFind(t, d, "触发")
	var panel geom.Rect
	for _, o := range d.r.c.overlays {
		panel = o.root.abs
	}
	if !layNear(panel.Center().Y, a.Center().Y) || panel.X < a.Right() {
		t.Fatalf("anchor %v panel %v", a, panel)
	}
}

func TestContextMenuOpensDropdown(t *testing.T) {
	items := []MenuItem{{Key: "copy", Label: "复制"}, {Key: "paste", Label: "粘贴"}}
	var area ID
	var got string
	var at geom.Point
	var atOK bool
	d := driver(func(c *Context) {
		dd := Dropdown(c, items).Trigger(TriggerContextMenu).Body(func() {
			area = Div(c, layout.Size(200, 120)).ID()
		})
		if k := dd.Clicked(); k != "" {
			got = k
		}
		if p, ok := c.ContextMenuFor(area); ok {
			at, atOK = p, true
		}
	})
	d.Settle()
	r := d.r.c.rects[area]
	if r.Empty() {
		t.Fatal("trigger area not laid out")
	}
	pt := geom.Pt(r.X+50, r.Y+40)
	d.RightClick(pt)
	if !atOK || at != pt {
		t.Fatalf("ContextMenuFor = %v %v", at, atOK)
	}
	m, ok := d.Find("复制")
	if !ok {
		t.Fatal("right click did not open the menu")
	}
	if m.X < pt.X-1 || m.Y < pt.Y-1 {
		t.Fatalf("menu not at the pointer: %v", m)
	}
	d.ClickLabel("粘贴")
	if got != "paste" {
		t.Fatalf("clicked %q", got)
	}
	// A left click does not report a context menu.
	atOK = false
	d.Click(pt)
	if atOK {
		t.Fatal("left click reported as context menu")
	}
}

func TestComponentSizeFollowsConfigProvider(t *testing.T) {
	var s string
	var f float64
	rows := map[string]ID{}
	row := func(c *Context, name string, fn func()) {
		rows[name] = Row(c, fn).ID()
	}
	d := driver(func(c *Context) {
		ConfigProvider(c).Size(Large).Body(func() {
			Column(c, func() {
				row(c, "button", func() { Button(c, "大") })
				row(c, "small", func() { Button(c, "小").Small() })
				row(c, "input", func() { Input(c, &s) })
				row(c, "select", func() { Select(c, &s, Opts("a", "b")) })
				row(c, "number", func() { InputNumber(c, &f) })
			})
		})
		row(c, "outside", func() { Button(c, "默认") })
	})
	d.Settle()
	th := d.r.c.Theme
	h := func(name string) float32 { return d.r.c.rects[rows[name]].H }
	for _, name := range []string{"button", "input", "select", "number"} {
		if h(name) != th.ControlHeightLG {
			t.Errorf("%s height %v, want %v", name, h(name), th.ControlHeightLG)
		}
	}
	if h("small") != th.ControlHeightSM {
		t.Errorf("explicit Small: %v", h("small"))
	}
	if h("outside") != th.ControlHeight {
		t.Errorf("outside provider: %v", h("outside"))
	}
}

func layNear(a, b float32) bool { return a-b < 0.51 && b-a < 0.51 }
