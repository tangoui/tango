package ui

import (
	"math"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

func navDriver(page func(*Context)) *Driver {
	return NewDriver(page, DriverOptions{Width: 960, Height: 600})
}

func navDrag(d *Driver, from, to geom.Point) {
	d.Inject(input.Pointer{Kind: input.Press, Pos: from, Button: input.ButtonLeft})
	d.Frame()
	d.Move(to)
	d.Inject(input.Pointer{Kind: input.Release, Pos: to, Button: input.ButtonLeft})
	d.Frame()
}

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 1.5 }

func TestNavLocaleAndConfig(t *testing.T) {
	var inner, outer *Locale
	var size ControlSize
	var hasSize bool
	var spaceProp ControlSize
	d := navDriver(func(c *Context) {
		outer = c.Locale()
		ConfigProvider(c).Locale(EnUS).Size(Small).Defaults("Space", Props{"size": Large}).Body(func() {
			inner = c.Locale()
			size, hasSize = c.ComponentSize()
			ConfigProvider(c).Body(func() { spaceProp = Prop(c, "Space", "size", Middle) })
		})
	})
	d.Frame()
	if outer != ZhCN || inner != EnUS {
		t.Fatalf("locale outer=%s inner=%s", outer.Code, inner.Code)
	}
	if !hasSize || size != Small || spaceProp != Large {
		t.Fatalf("size=%v %v prop=%v", size, hasSize, spaceProp)
	}
	if ZhCN.Total(85) != "共 85 条" || EnUS.Total(85) != "Total 85 items" {
		t.Fatal("total format")
	}
	// Partial locales fall back to en-US.
	// (a made-up code, so the built-in packs stay registered for other tests)
	part := &Locale{Code: "eo-XX", OK: "Bone", Cancel: "Nuligi", Image: ImageLocale{ZoomIn: "Zomi"}}
	RegisterLocale(part)
	got, ok := LocaleByCode("eo_xx")
	if !ok || got.Cancel != "Nuligi" || got.NoData != EnUS.NoData || got.Weekdays[0] != EnUS.Weekdays[0] ||
		got.Image.ZoomIn != "Zomi" || got.Image.ZoomOut != EnUS.Image.ZoomOut || got.Upload.Button != EnUS.Upload.Button {
		t.Fatalf("fallback failed: %+v", got)
	}
	if l, ok := LocaleByCode("en"); !ok || l.Code != "en-US" {
		t.Fatal("language-only lookup")
	}
}

func TestNavConfigTheme(t *testing.T) {
	d := navDriver(func(c *Context) {
		ConfigProvider(c).Flavor(theme.Element).Dark(true).Body(func() { Text(c, "x") })
	})
	d.Frame()
	th := d.Runner().Context().Theme
	if th.Flavor != theme.Element || !th.Dark {
		t.Fatalf("theme not switched: %v %v", th.Flavor, th.Dark)
	}
}

func TestNavHoverDelay(t *testing.T) {
	var open bool
	hover := false
	d := NewDriver(func(c *Context) {
		open = HoverDelay(c, ID(42), hover, 100*time.Millisecond, 200*time.Millisecond)
	}, DriverOptions{Width: 100, Height: 100, Animations: true})
	d.Frame()
	hover = true
	d.Frame()
	if open {
		t.Fatal("opened before delay")
	}
	d.Advance(120 * time.Millisecond)
	d.Frame()
	if !open {
		t.Fatal("not opened after delay")
	}
	hover = false
	d.Advance(50 * time.Millisecond)
	d.Frame()
	if !open {
		t.Fatal("closed before leave delay")
	}
	d.Advance(250 * time.Millisecond)
	d.Frame()
	if open {
		t.Fatal("still open")
	}
}

func TestNavAffixAnchorBackTop(t *testing.T) {
	var sb *ScrollBox
	var aff *AffixW
	var anc *AnchorW
	var target ID
	items := []AnchorItem{{Key: "a", Title: "章节A"}, {Key: "b", Title: "章节B"}, {Key: "c", Title: "章节C"}}
	d := navDriver(func(c *Context) {
		Row(c, func() {
			anc = Anchor(c, items).Target(target).Show()
			sb = Scroll(c, func() {
				Gap(c, 50)
				aff = Affix(c).Target(target).Body(func() { Button(c, "固定按钮") })
				for _, it := range items {
					AnchorSection(c, it.Key, func() { Column(c, func() {}, layout.H(400)) })
				}
			}, layout.H(300), layout.Grow(1))
			target = sb.ID()
		}, layout.AlignItems(layout.AlignStart))
		BackTop(c, target).VisibilityHeight(200).Show()
	})
	d.Settle()
	d.Frame()
	if aff.Affixed() {
		t.Fatal("affixed before scrolling")
	}
	if anc.Active() != "a" {
		t.Fatalf("active=%q", anc.Active())
	}
	d.Scroll(geom.Pt(600, 150), 500)
	d.Frame()
	d.Frame()
	if !aff.Affixed() {
		t.Fatal("not affixed after scrolling")
	}
	r := mustFind(t, d, "固定按钮")
	vr := d.Runner().Context().rects[target]
	if !near(r.Y, vr.Y) {
		t.Fatalf("affix at %v, view at %v", r.Y, vr.Y)
	}
	if anc.Active() != "b" {
		t.Fatalf("active after scroll=%q", anc.Active())
	}
	// Click a link: smooth scroll to section C.
	d.Click(mustFind(t, d, "章节C").Center())
	d.Settle()
	d.Frame()
	if anc.Active() != "c" {
		t.Fatalf("active after click=%q", anc.Active())
	}
	// Back to top.
	d.Click(mustFind(t, d, ZhCN.BackTop).Center())
	d.Settle()
	d.Frame()
	if _, y := sb.Offset(); y != 0 {
		t.Fatalf("offset after back top = %v", y)
	}
	if aff.Affixed() {
		t.Fatal("still affixed at top")
	}
}

func TestStepsClickable(t *testing.T) {
	cur := 0
	changed := false
	d := navDriver(func(c *Context) {
		changed = Steps(c, []StepItem{{Title: "一"}, {Title: "二"}, {Title: "三", Disabled: true}}).Clickable(&cur).Show().Changed() || changed
	})
	d.Frame()
	d.Click(mustFind(t, d, "二").Center())
	if cur != 1 || !changed {
		t.Fatalf("cur=%d", cur)
	}
	if _, ok := d.Find("三"); ok {
		t.Fatal("disabled step is clickable")
	}
}

func TestDropdownSubmenu(t *testing.T) {
	items := []MenuItem{{Key: "a", Label: "甲"}, {Key: "sub", Label: "子菜单", Children: []MenuItem{{Key: "x", Label: "叉"}}}}
	var got string
	d := navDriver(func(c *Context) {
		dd := Dropdown(c, items).Trigger(TriggerClick).Body(func() { Button(c, "打开") })
		if k := dd.Clicked(); k != "" {
			got = k
		}
	})
	d.Frame()
	d.ClickLabel("打开")
	d.Frame()
	d.Move(mustFind(t, d, "子菜单").Center())
	d.Frame()
	d.Click(mustFind(t, d, "叉").Center())
	if got != "x" {
		t.Fatalf("clicked %q", got)
	}
	if _, ok := d.Find("甲"); ok {
		t.Fatal("menu stayed open")
	}
	// Hover trigger opens without clicking; context menu opens by long press.
	var open bool
	d2 := NewDriver(func(c *Context) {
		open = Dropdown(c, items).Body(func() { Button(c, "悬停") }).IsOpen()
		Dropdown(c, items).Trigger(TriggerContextMenu).Body(func() { Column(c, func() {}, layout.Size(100, 50)) })
	}, DriverOptions{Width: 600, Height: 400})
	d2.Frame()
	d2.Move(mustFind(t, d2, "悬停").Center())
	d2.Frame()
	if !open {
		t.Fatal("hover did not open")
	}
}

func TestMenuCollapsedPopup(t *testing.T) {
	sel := ""
	items := []MenuItem{{Key: "d", Label: "仪表盘", Children: []MenuItem{{Key: "m", Label: "监控页"}}}, {Key: "f", Label: "表单页"}}
	var clicked string
	d := navDriver(func(c *Context) {
		if k := Menu(c, items, &sel).Collapsed(true).Show().Clicked(); k != "" {
			clicked = k
		}
	})
	d.Frame()
	d.Move(mustFind(t, d, "仪表盘").Center())
	d.Frame()
	d.Click(mustFind(t, d, "监控页").Center())
	if sel != "m" || clicked != "m" {
		t.Fatalf("sel=%q clicked=%q", sel, clicked)
	}
}

func TestNavGrid(t *testing.T) {
	var a, b *Box
	d := navDriver(func(c *Context) {
		Column(c, func() {
			GridRow(c).Gutter(16, 0).Body(func() {
				Col(c, 8).Body(func() { a = Div(c, layout.H(10)) })
				Col(c, 8).Offset(8).Body(func() { b = Div(c, layout.H(10)) })
			})
		}, layout.W(816))
	})
	d.Frame()
	d.Frame()
	ra, rb := a.Rect(), b.Rect()
	// Row 816 + gutter 16 = 832 / 24 = 34.67 per column; col 8 = 277.3 - 16.
	if !near(ra.W, 832.0/3-16) || !near(rb.X-ra.X, 2*832.0/3) {
		t.Fatalf("a=%v b=%v", ra, rb)
	}
	// Responsive: md span applies at 960px (Ant md >= 768).
	var w float32
	d2 := navDriver(func(c *Context) {
		Column(c, func() {
			GridRow(c).Body(func() { Col(c, 24).MD(12).Body(func() { a = Div(c, layout.H(5)) }) })
		}, layout.W(800))
		w = a.Rect().W
	})
	d2.Frame()
	d2.Frame()
	if !near(w, 400) {
		t.Fatalf("responsive width %v", w)
	}
}

func TestNavSpace(t *testing.T) {
	var sp, cp *Box
	d := navDriver(func(c *Context) {
		sp = Space(c).Split(func() { VDivider(c) }).Body(func() {
			Text(c, "一")
			Text(c, "二")
			Text(c, "三")
		})
		cp = Compact(c).Body(func() {
			Button(c, "左")
			Button(c, "右")
		})
	})
	d.Frame()
	if n := len(sp.n.kids); n != 5 {
		t.Fatalf("split kids = %d", n)
	}
	l, r := mustFind(t, d, "左"), mustFind(t, d, "右")
	if !near(r.X, l.Right()-1) {
		t.Fatalf("compact overlap: %v %v", l, r)
	}
	_ = cp
}

func TestNavSplitterDrag(t *testing.T) {
	var s *SplitterW
	d := navDriver(func(c *Context) {
		s = Splitter(c).Style(layout.W(606), layout.H(100))
		s.Body(func() {
			SplitterPanel(c).Body(func() {})
			SplitterPanel(c).Min(100).Collapsible().Body(func() {})
		})
	})
	d.Frame()
	d.Frame()
	bar := mustFind(t, d, "splitter-bar-1")
	navDrag(d, bar.Center(), geom.Pt(bar.Center().X+100, bar.Center().Y))
	if f := s.Sizes(); !near(f[0]*600, 400) {
		t.Fatalf("sizes after drag %v", f)
	}
	// The min size of the second pane stops the bar.
	bar = mustFind(t, d, "splitter-bar-1")
	navDrag(d, bar.Center(), geom.Pt(bar.Center().X+300, bar.Center().Y))
	if f := s.Sizes(); !near(f[1]*600, 100) {
		t.Fatalf("min not honored %v", f)
	}
	// Collapse the second pane into the first.
	d.Click(mustFind(t, d, "collapse-end").Center())
	if f := s.Sizes(); f[1] != 0 {
		t.Fatalf("not collapsed %v", f)
	}
	d.Click(mustFind(t, d, "collapse-start").Center())
	if f := s.Sizes(); f[1] == 0 {
		t.Fatalf("not restored %v", f)
	}
}

func TestLayoutDirection(t *testing.T) {
	var outer, inner, side *Box
	var content *Box
	d := navDriver(func(c *Context) {
		outer = Layout(c, func() {
			Header(c, func() {})
			inner = Layout(c, func() {
				side = Sider(c, 200, false, func() {}).Box
				content = Content(c, func() {})
			})
			Footer(c, func() {})
		})
	})
	d.Frame()
	if outer.n.ln.Dir != layout.Column || inner.n.ln.Dir != layout.Row {
		t.Fatal("layout direction")
	}
	if inner.n.ln.Grow != 1 || content.n.ln.Grow != 1 {
		t.Fatal("nested layout / content should grow")
	}
	if r := side.Rect(); r.W != 200 {
		t.Fatalf("sider width %v", r.W)
	}
}

func TestModalDragAndEscape(t *testing.T) {
	open := true
	d := navDriver(func(c *Context) {
		Modal(c, &open, "标题").Draggable().Body(func() { Text(c, "内容") })
	})
	d.Frame()
	d.Frame()
	h := mustFind(t, d, "标题")
	navDrag(d, h.Center(), geom.Pt(h.Center().X+50, h.Center().Y+30))
	d.Frame()
	h2 := mustFind(t, d, "标题")
	if !near(h2.X-h.X, 50) || !near(h2.Y-h.Y, 30) {
		t.Fatalf("drag moved %v -> %v", h, h2)
	}
	d.Key(input.KeyEscape, 0)
	d.Frame()
	if open {
		t.Fatal("escape did not close")
	}
}

func TestDrawerNestedPush(t *testing.T) {
	parent, child := true, false
	d := navDriver(func(c *Context) {
		Drawer(c, &parent, "父").Size(400).Body(func() {
			Button(c, "父按钮")
			Drawer(c, &child, "子").Size(300).Body(func() { Text(c, "子内容") })
		})
	})
	d.Settle()
	before := mustFind(t, d, "父按钮")
	child = true
	d.Frame()
	d.Settle()
	after := mustFind(t, d, "父按钮")
	if !near(before.X-after.X, 180) {
		t.Fatalf("push %v -> %v", before.X, after.X)
	}
	// Top placement spans the window width.
	top := true
	d2 := navDriver(func(c *Context) {
		Drawer(c, &top, "顶").Placement(DrawerTop).Size(200).Body(func() { Button(c, "顶按钮") })
	})
	d2.Settle()
	if r := mustFind(t, d2, "顶按钮"); r.Y > 200 {
		t.Fatalf("top drawer content at %v", r)
	}
}

func TestNavNotificationAndMessage(t *testing.T) {
	var c0 *Context
	d := NewDriver(func(c *Context) { c0 = c; Text(c, "page") }, DriverOptions{Width: 960, Height: 600})
	d.Frame()
	Notification(c0, "", "标题一").Key("k").Duration(time.Second)
	d.Frame()
	mustFind(t, d, ZhCN.Close)
	// Hovering pauses the countdown.
	d.Move(mustFind(t, d, ZhCN.Close).Center())
	d.Advance(2 * time.Second)
	d.Frame()
	if _, ok := d.Find(ZhCN.Close); !ok {
		t.Fatal("expired while hovered")
	}
	d.Move(geom.Pt(10, 590))
	d.Frame()
	d.Advance(2 * time.Second)
	d.Frame()
	d.Frame()
	if len(c0.navNotes().list) != 0 {
		t.Fatalf("notification not expired: %d", len(c0.navNotes().list))
	}
	// Message: same key updates in place.
	Message(c0, "loading", "保存中").Key("save").Duration(0)
	d.Frame()
	Message(c0, "success", "已保存").Key("save")
	d.Frame()
	if n := len(c0.toasts().msgs); n != 1 || c0.toasts().msgs[0].title != "已保存" {
		t.Fatalf("messages=%d", n)
	}
	MessageDestroy(c0, "save")
	d.Advance(time.Second)
	d.Frame()
	if len(c0.toasts().msgs) != 0 {
		t.Fatal("message not destroyed")
	}
}

func TestSpinDelay(t *testing.T) {
	d := NewDriver(func(c *Context) {
		Spin(c).Tip("等待").Delay(300 * time.Millisecond)
	}, DriverOptions{Width: 200, Height: 200, Animations: true})
	d.Frame()
	if _, ok := navFindNode(d, "等待"); ok {
		t.Fatal("spinner shown before delay")
	}
	d.Advance(400 * time.Millisecond)
	d.Frame()
	if _, ok := navFindNode(d, "等待"); !ok {
		t.Fatal("spinner not shown after delay")
	}
}

func navFindNode(d *Driver, label string) (*node, bool) {
	var found *node
	var walk func(n *node)
	walk = func(n *node) {
		if n.label == label {
			found = n
		}
		for _, k := range n.kids {
			walk(k)
		}
	}
	walk(d.Runner().Context().root)
	return found, found != nil
}

func TestNavScrollbarDrag(t *testing.T) {
	var sb *ScrollBox
	d := navDriver(func(c *Context) {
		sb = Scroll(c, func() {
			Column(c, func() {}, layout.H(1000))
		}, layout.H(200), layout.W(300)).Always()
	})
	d.Frame()
	d.Frame()
	r := d.Runner().Context().rects[sb.ID()]
	// Drag the thumb from its top down by 80px of track.
	from := geom.Pt(r.Right()-5, r.Y+10)
	navDrag(d, from, geom.Pt(from.X, from.Y+80))
	_, y := sb.Offset()
	if y <= 0 {
		t.Fatalf("thumb drag did not scroll: %v", y)
	}
	// Track click near the bottom jumps to the end.
	d.Click(geom.Pt(r.Right()-5, r.Bottom()-3))
	if _, y := sb.Offset(); !near(float32(y), 800) {
		t.Fatalf("track click offset %v", y)
	}
}

func TestNavPageHeaderBack(t *testing.T) {
	back := false
	d := navDriver(func(c *Context) {
		if PageHeader(c, "标题").Back().Show().BackClicked() {
			back = true
		}
	})
	d.Frame()
	d.ClickLabel(ZhCN.Back)
	if !back {
		t.Fatal("back not reported")
	}
}

func TestNavFloatGroupToggle(t *testing.T) {
	open := false
	d := navDriver(func(c *Context) {
		FloatButtonGroup(c).Trigger("click").Open(&open).Body(func() {
			FloatButton(c).Tooltip("成员")
		})
	})
	d.Frame()
	if _, ok := d.Find("成员"); ok {
		t.Fatal("member visible while closed")
	}
	d.ClickLabel("float")
	d.Frame()
	if !open {
		t.Fatal("group did not open")
	}
	mustFind(t, d, "成员")
}
