package ui

import (
	"fmt"
	"image/color"
	"math"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// Tests of the navigation / display / feedback additions (Tabs editable,
// placement, overflow; Pagination jumper, simple, hide, disabled,
// background; structured Breadcrumb; Menu groups and overflow; Tooltip
// options; Popconfirm options; Avatar, Divider, Tag, Alert, Steps,
// Carousel, Grid, Sider, ProLayout), each in the Ant and Element themes.

func p2cThemes(t *testing.T, fn func(t *testing.T, th func() *theme.Theme)) {
	for _, tc := range []struct {
		name string
		th   func() *theme.Theme
	}{{"ant", func() *theme.Theme { return theme.AntDesign() }}, {"element", func() *theme.Theme { return theme.ElementPlus() }}} {
		t.Run(tc.name, func(t *testing.T) { fn(t, tc.th) })
	}
}

func p2cDriver(th func() *theme.Theme, w, h float32, page func(c *Context)) *Driver {
	d := NewDriver(func(c *Context) { Column(c, func() { page(c) }, layout.Pad(16)) }, DriverOptions{Width: w, Height: h, Theme: th()})
	d.Settle()
	d.Settle()
	return d
}

func p2cFind(t *testing.T, d *Driver, label string) geom.Rect {
	t.Helper()
	r, ok := d.Find(label)
	if !ok {
		t.Fatalf("no widget %q", label)
	}
	return r
}

func p2cText(t *testing.T, d *Driver, label string) geom.Rect {
	t.Helper()
	r, ok := d.FindText(label)
	if !ok {
		t.Fatalf("no text %q", label)
	}
	return r
}

func p2cA11y(t *testing.T, d *Driver, role a11y.Role, name string) *a11y.Node {
	t.Helper()
	n, ok := d.A11yFind(role, name)
	if !ok {
		t.Fatalf("no %s %q in the accessibility tree", role, name)
	}
	return n
}

func p2cPixel(d *Driver, x, y float32) color.RGBA {
	return d.Image().RGBAAt(int(x), int(y))
}

func p2cLuma(c color.RGBA) float64 {
	return (0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)) / 255
}

// ---------------------------------------------------------------------------
// Tabs

func TestP2CTabsEditable(t *testing.T) {
	p2cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		items := []TabItem{{Key: "a", Label: "Alpha"}, {Key: "b", Label: "Beta"}, {Key: "c", Label: "Gamma", NoClose: true}}
		active, closed, adds, added := "a", "", 0, 0
		d := p2cDriver(th, 800, 300, func(c *Context) {
			tb := Tabs(c, &active, items).Closable().OnAdd(func() { adds++ }).Show()
			if k := tb.Closed(); k != "" {
				closed = k
				for i, it := range items {
					if it.Key == k {
						items = append(items[:i:i], items[i+1:]...)
						break
					}
				}
			}
			if tb.Added() {
				added++
			}
		})
		d.EnableAccessibility()
		p2cA11y(t, d, a11y.RoleTab, "Beta")
		p2cA11y(t, d, a11y.RoleButton, ZhCN.RemoveItem("Beta"))
		if _, ok := d.A11yFind(a11y.RoleButton, ZhCN.RemoveItem("Gamma")); ok {
			t.Fatal("NoClose tab has a close button")
		}
		d.Click(p2cFind(t, d, ZhCN.RemoveItem("Beta")).Center())
		if closed != "b" || len(items) != 2 || active != "a" {
			t.Fatalf("close: closed=%q items=%d active=%q", closed, len(items), active)
		}
		d.Click(p2cFind(t, d, ZhCN.Tabs.Add).Center())
		if adds != 1 || added != 1 {
			t.Fatalf("add: %d %d", adds, added)
		}
		// Delete on the focused tab closes it.
		d.Click(p2cFind(t, d, "Alpha").Center())
		d.Key(input.KeyDelete, 0)
		if closed != "a" || len(items) != 1 {
			t.Fatalf("delete key: closed=%q items=%d", closed, len(items))
		}
	})
}

func TestP2CTabsPlacement(t *testing.T) {
	p2cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		items := []TabItem{{Key: "a", Label: "Alpha"}, {Key: "b", Label: "Beta"}, {Key: "c", Label: "Gamma"}}
		for _, p := range []Placement{Left, Right, Bottom} {
			active := "a"
			d := p2cDriver(th, 800, 400, func(c *Context) {
				tb := Tabs(c, &active, items).Placement(p).Show()
				tb.Panel(func() { Text(c, "Panel body") })
			})
			d.EnableAccessibility()
			a, b := p2cFind(t, d, "Alpha"), p2cFind(t, d, "Beta")
			body := p2cText(t, d, "Panel body")
			switch p {
			case Left:
				if !(b.Y > a.Bottom()-1 && math.Abs(float64(a.X-b.X)) < 1 && body.X >= a.Right()) {
					t.Fatalf("left: a=%v b=%v body=%v", a, b, body)
				}
			case Right:
				if !(b.Y > a.Bottom()-1 && body.Right() <= a.X+1) {
					t.Fatalf("right: a=%v b=%v body=%v", a, b, body)
				}
			case Bottom:
				if !(b.X > a.Right()-1 && body.Bottom() <= a.Y) {
					t.Fatalf("bottom: a=%v b=%v body=%v", a, b, body)
				}
			}
			list := p2cA11y(t, d, a11y.RoleTabList, "")
			vert := p == Left || p == Right
			if vert != (list.States&a11y.Vertical != 0) {
				t.Fatalf("%v: tablist orientation %v", p, list.States)
			}
			panel := p2cA11y(t, d, a11y.RoleTabPanel, "Alpha")
			if tab := p2cA11y(t, d, a11y.RoleTab, "Alpha"); tab.Controls != panel.ID {
				t.Fatalf("%v: tab controls %v, panel %v", p, tab.Controls, panel.ID)
			}
			// Arrow keys follow the orientation.
			d.Click(a.Center())
			k := input.KeyRight
			if vert {
				k = input.KeyDown
			}
			d.Key(k, 0)
			if active != "b" {
				t.Fatalf("%v: arrow key -> %q", p, active)
			}
		}
	})
}

func TestP2CTabsCenteredAndMargin(t *testing.T) {
	p2cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		items := []TabItem{{Key: "a", Label: "Alpha"}, {Key: "b", Label: "Beta"}, {Key: "c", Label: "Gamma"}}
		active := "a"
		noMargin := false
		d := p2cDriver(th, 800, 300, func(c *Context) {
			tb := Tabs(c, &active, items).Centered()
			if noMargin {
				tb.NoMargin()
			}
			tb.Show()
			Text(c, "Below")
		})
		a, g := p2cFind(t, d, "Alpha"), p2cFind(t, d, "Gamma")
		mid := (a.X + g.Right()) / 2
		if math.Abs(float64(mid-400)) > 2 {
			t.Fatalf("centered: tabs span %v..%v", a.X, g.Right())
		}
		gap := p2cText(t, d, "Below").Y - a.Bottom()
		noMargin = true
		d.Settle()
		gap0 := p2cText(t, d, "Below").Y - p2cFind(t, d, "Alpha").Bottom()
		if gap < 14 || math.Abs(float64(gap0)) > 0.5 {
			t.Fatalf("margin %v, NoMargin %v", gap, gap0)
		}
	})
}

func TestP2CTabsOverflow(t *testing.T) {
	p2cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		var items []TabItem
		for i := 1; i <= 12; i++ {
			items = append(items, TabItem{Key: fmt.Sprint("k", i), Label: fmt.Sprint("Tab number ", i)})
		}
		active := "k1"
		changed := false
		d := NewDriver(func(c *Context) {
			Column(c, func() {
				if Tabs(c, &active, items).Show().Changed() {
					changed = true
				}
			}, layout.Pad(16))
		}, DriverOptions{Width: 420, Height: 400, Theme: th(), Animations: true})
		for i := 0; i < 4; i++ {
			d.Settle()
		}
		d.EnableAccessibility()
		more := p2cA11y(t, d, a11y.RoleButton, ZhCN.Tabs.More)
		if more.States&a11y.HasPopup == 0 {
			t.Fatalf("more button states %v", more.States)
		}
		// The list scrolls with the wheel.
		first := p2cFind(t, d, "Tab number 1")
		d.Scroll(first.Center(), 120)
		// (hit rects are clipped to the list viewport)
		if moved, ok := d.Find("Tab number 1"); ok && moved.W > first.W-60 {
			t.Fatalf("wheel did not scroll: %v -> %v", first, moved)
		}
		// The more menu lists hidden tabs; choosing one selects it and
		// scrolls it into view.
		mr := geom.Rect{X: more.Bounds.X, Y: more.Bounds.Y, W: more.Bounds.W, H: more.Bounds.H}
		d.Move(mr.Center())
		d.Advance(400 * time.Millisecond)
		d.Settle()
		d.Settle()
		item, ok := d.Find("Tab number 12")
		if !ok {
			t.Fatal("hidden tab not in the more menu")
		}
		d.Click(item.Center())
		for i := 0; i < 4; i++ {
			d.Advance(time.Second)
			d.Settle()
		}
		if active != "k12" || !changed {
			t.Fatalf("more menu pick: %q %v", active, changed)
		}
		tab := p2cFind(t, d, "Tab number 12")
		if tab.Right() > 420 || tab.X < 0 {
			t.Fatalf("active tab not revealed: %v", tab)
		}
		// Wide window: no overflow, no more button.
		d.Resize(2400, 400)
		for i := 0; i < 3; i++ {
			d.Settle()
		}
		if _, ok := d.A11yFind(a11y.RoleButton, ZhCN.Tabs.More); ok {
			t.Fatal("more button without overflow")
		}
	})
}

// ---------------------------------------------------------------------------
// Pagination

func TestP2CPaginationJumperSimple(t *testing.T) {
	p2cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		page, sp := 1, 3
		var changed bool
		d := p2cDriver(th, 900, 300, func(c *Context) {
			if Pagination(c, &page, 500, 10).ShowQuickJumper().Show().Changed() {
				changed = true
			}
			Gap(c, 16)
			Pagination(c, &sp, 500, 10).Simple().Show()
		})
		d.EnableAccessibility()
		jump := p2cA11y(t, d, a11y.RoleTextbox, ZhCN.Pagination.Jump)
		d.Click(geom.Rect{X: jump.Bounds.X, Y: jump.Bounds.Y, W: jump.Bounds.W, H: jump.Bounds.H}.Center())
		d.Type("7")
		d.Key(input.KeyEnter, 0)
		if page != 7 || !changed {
			t.Fatalf("jumper: page %d", page)
		}
		if n := p2cA11y(t, d, a11y.RoleTextbox, ZhCN.Pagination.Jump); n.Value != "" {
			t.Fatalf("jumper not cleared: %q", n.Value)
		}
		// Out of range clamps.
		d.Type("999")
		d.Key(input.KeyEnter, 0)
		if page != 50 {
			t.Fatalf("clamp: %d", page)
		}
		// Simple pager: the box shows the page; typing jumps; arrows step.
		cur := p2cA11y(t, d, a11y.RoleTextbox, ZhCN.Pagination.Current)
		if cur.Value != "3" {
			t.Fatalf("simple value %q", cur.Value)
		}
		d.Click(geom.Rect{X: cur.Bounds.X, Y: cur.Bounds.Y, W: cur.Bounds.W, H: cur.Bounds.H}.Center())
		d.Key(input.KeyBackspace, 0)
		d.Type("12")
		d.Key(input.KeyEnter, 0)
		if sp != 12 {
			t.Fatalf("simple jump: %d", sp)
		}
		if _, ok := d.FindText("/"); !ok {
			t.Fatal("simple pager separator")
		}
		nexts := d.A11yFindAll(a11y.RoleButton, ZhCN.NextPage)
		if len(nexts) != 2 {
			t.Fatalf("next buttons %d", len(nexts))
		}
		nb := nexts[0].Bounds
		if nexts[1].Bounds.Y > nb.Y {
			nb = nexts[1].Bounds
		}
		d.Click(geom.Rect{X: nb.X, Y: nb.Y, W: nb.W, H: nb.H}.Center())
		if sp != 13 || p2cA11y(t, d, a11y.RoleTextbox, ZhCN.Pagination.Current).Value != "13" {
			t.Fatalf("simple next: %d", sp)
		}
	})
}

func TestP2CPaginationHideDisabledBackground(t *testing.T) {
	p2cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		page := 1
		hide, disabled := true, false
		bg := false
		total := 5
		d := p2cDriver(th, 900, 300, func(c *Context) {
			Column(c, func() {
				p := Pagination(c, &page, total, 10).Disabled(disabled).Background(bg)
				if hide {
					p.HideOnSinglePage()
				}
				p.Show()
			}, layout.Pad(8)).Bg(c.Theme.BgContainer)
		})
		d.EnableAccessibility()
		if _, ok := d.A11yFind(a11y.RoleNavigation, ZhCN.A11y.Pagination); ok {
			t.Fatal("single page not hidden")
		}
		hide = false
		d.Settle()
		p2cA11y(t, d, a11y.RoleNavigation, ZhCN.A11y.Pagination)
		total = 100
		disabled = true
		d.Settle()
		d.Click(p2cFind(t, d, "2").Center())
		if page != 1 {
			t.Fatal("disabled pager changed page")
		}
		disabled = false
		d.Settle()
		// Background: filled page buttons.
		r := p2cFind(t, d, "3")
		plain := p2cPixel(d, r.X+2, r.Y+r.H/2)
		bg = true
		d.Settle()
		filled := p2cPixel(d, r.X+2, r.Y+r.H/2)
		if p2cLuma(plain) < 0.97 || p2cLuma(filled) > 0.97 {
			t.Fatalf("background: plain %v filled %v", plain, filled)
		}
		d.Click(r.Center())
		if page != 3 {
			t.Fatalf("click page: %d", page)
		}
	})
}

// ---------------------------------------------------------------------------
// Breadcrumb

func TestP2CBreadcrumbItems(t *testing.T) {
	p2cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		homes, href, menu := 0, "", ""
		oldIdx := -1
		d := p2cDriver(th, 900, 400, func(c *Context) {
			b := BreadcrumbItems(c, []BreadcrumbItem{
				{Label: "Home", Icon: icon.Home, OnClick: func() { homes++ }},
				{Label: "List", Href: "/list"},
				{Label: "Apps", Menu: []MenuItem{{Key: "x", Label: "App X"}, {Key: "y", Label: "App Y"}}},
				{Label: "Detail"},
			}).Separator(">").Show()
			if h := b.ClickedHref(); h != "" {
				href = h
			}
			if k := b.MenuClicked(); k != "" {
				menu = k
			}
			Gap(c, 24)
			if i := Breadcrumb(c, "Root", "Leaf"); i >= 0 {
				oldIdx = i
			}
		})
		d.EnableAccessibility()
		if _, ok := d.FindText(">"); !ok {
			t.Fatal("custom separator")
		}
		p2cA11y(t, d, a11y.RoleNavigation, ZhCN.A11y.Breadcrumb)
		p2cA11y(t, d, a11y.RoleLink, "Home")
		if n := p2cA11y(t, d, a11y.RoleLink, "Apps"); n.States&a11y.HasPopup == 0 {
			t.Fatalf("menu item states %v", n.States)
		}
		d.Click(p2cFind(t, d, "Home").Center())
		d.Click(p2cFind(t, d, "List").Center())
		if homes != 1 || href != "/list" {
			t.Fatalf("clicks: home %d href %q", homes, href)
		}
		d.Move(p2cFind(t, d, "Apps").Center())
		d.Advance(400 * time.Millisecond)
		d.Settle()
		d.Click(p2cFind(t, d, "App Y").Center())
		if menu != "y" {
			t.Fatalf("menu pick %q", menu)
		}
		// The string form still returns the clicked index.
		d.Click(p2cFind(t, d, "Root").Center())
		if oldIdx != 0 {
			t.Fatalf("Breadcrumb index %d", oldIdx)
		}
		if _, ok := d.FindText("/"); !ok {
			t.Fatal("default separator")
		}
	})
}

// ---------------------------------------------------------------------------
// Menu

func TestP2CMenuGroups(t *testing.T) {
	p2cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		sel := ""
		items := []MenuItem{
			{Group: true, Label: "Group One", Children: []MenuItem{{Key: "1", Label: "Option 1"}, {Key: "2", Label: "Option 2"}}},
			{Key: "3", Label: "Option 3"},
			{Key: "sub", Label: "More things", Children: []MenuItem{
				{Group: true, Label: "Inner", Children: []MenuItem{{Key: "g1", Label: "Grouped 1"}}},
				{Key: "s2", Label: "Plain 2"},
			}},
		}
		d := p2cDriver(th, 900, 600, func(c *Context) {
			Row(c, func() {
				Menu(c, items, &sel).Width(220).DefaultOpen("sub").Show()
				Menu(c, items, &sel).Mode(MenuVertical).Width(220).Show()
			}, layout.Gap(40), layout.AlignItems(layout.AlignStart))
		})
		d.EnableAccessibility()
		title := p2cText(t, d, "Group One")
		o1 := p2cFind(t, d, "Option 1")
		if title.Y >= o1.Y {
			t.Fatal("group title below its items")
		}
		p2cA11y(t, d, a11y.RoleGroup, "Group One")
		p2cText(t, d, "Inner") // inline submenu group
		d.Click(p2cFind(t, d, "Option 2").Center())
		if sel != "2" {
			t.Fatalf("group item click: %q", sel)
		}
		// Keyboard walks through grouped items.
		d.Key(input.KeyUp, 0)
		if f := d.A11yFocused(); f == nil || f.Name != "Option 1" {
			t.Fatalf("focus after Up: %+v", f)
		}
		// The vertical menu's pop-out panel shows the grouped children.
		var vert geom.Rect
		for _, h := range d.r.c.hits {
			if h.label == "More things" && h.rect.X > vert.X {
				vert = h.rect
			}
		}
		d.Move(vert.Center())
		d.Advance(400 * time.Millisecond)
		d.Settle()
		d.Settle()
		if r, ok := d.Find("Grouped 1"); !ok || r.X < vert.Right() {
			t.Fatalf("popup group children: %v %v", r, ok)
		}
	})
}

func TestP2CMenuHorizontalOverflow(t *testing.T) {
	p2cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		sel := "i1"
		var items []MenuItem
		for i := 1; i <= 8; i++ {
			items = append(items, MenuItem{Key: fmt.Sprint("i", i), Label: fmt.Sprint("Item ", i)})
		}
		d := NewDriver(func(c *Context) {
			Column(c, func() { Menu(c, items, &sel).Mode(MenuHorizontal).Show() }, layout.Pad(16))
		}, DriverOptions{Width: 420, Height: 400, Theme: th()})
		for i := 0; i < 3; i++ {
			d.Settle()
		}
		d.EnableAccessibility()
		more := p2cA11y(t, d, a11y.RoleMenuItem, ZhCN.More)
		if more.States&a11y.HasPopup == 0 {
			t.Fatal("overflow item is not a submenu")
		}
		if _, ok := d.Find("Item 8"); ok {
			t.Fatal("overflowed item still in the bar")
		}
		d.Move(geom.Rect{X: more.Bounds.X, Y: more.Bounds.Y, W: more.Bounds.W, H: more.Bounds.H}.Center())
		d.Advance(400 * time.Millisecond)
		d.Settle()
		d.Settle()
		d.Click(p2cFind(t, d, "Item 8").Center())
		if sel != "i8" {
			t.Fatalf("overflow pick %q", sel)
		}
		if n := p2cA11y(t, d, a11y.RoleMenuItem, ZhCN.More); n.States&a11y.Selected == 0 {
			t.Fatal("overflow item should show the selection inside it")
		}
		d.Resize(1400, 400)
		for i := 0; i < 3; i++ {
			d.Settle()
		}
		if _, ok := d.A11yFind(a11y.RoleMenuItem, ZhCN.More); ok {
			t.Fatal("overflow item on a wide bar")
		}
		p2cFind(t, d, "Item 8")
	})
}

// ---------------------------------------------------------------------------
// Tooltip

func TestP2CTooltipOptions(t *testing.T) {
	p2cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		var bottom, click, focus, rich *TooltipW
		var anchor *Box
		d := p2cDriver(th, 900, 500, func(c *Context) {
			Row(c, func() {
				bottom = Tooltip(c, "Below tip", func() { Button(c, "Bottom") }).Placement("bottom").Color("#ff0000")
				click = Tooltip(c, "Click tip", func() { Button(c, "Clicker") }).Trigger("click").Placement("right")
				focus = Tooltip(c, "Focus tip", func() { Button(c, "Focuser") }).Trigger("focus").Arrow(false)
				rich = Tooltip(c, "Rich", func() { Text(c, "Hover rich") }).Content(func() { Text(c, "Rich body").Strong() }).Effect("light")
				anchor = bottom.Box
			}, layout.Gap(24), layout.Margin(geom.Insets{Top: 120}))
		})
		d.EnableAccessibility()
		if anchor == nil || anchor.Rect().W == 0 {
			t.Fatal("embedded anchor box")
		}
		b := p2cFind(t, d, "Bottom")
		d.Move(b.Center())
		tip := p2cA11y(t, d, a11y.RoleTooltip, "Below tip")
		if !bottom.IsOpen() || tip.Bounds.Y < b.Bottom() {
			t.Fatalf("bottom placement: tip %v trigger %v", tip.Bounds, b)
		}
		if px := p2cPixel(d, tip.Bounds.X+3, tip.Bounds.Y+3); px.R < 200 || px.G > 80 {
			t.Fatalf("tooltip color %v", px)
		}
		if btn := p2cA11y(t, d, a11y.RoleButton, "Bottom"); btn.DescribedBy != tip.ID || btn.Description != "Below tip" {
			t.Fatalf("describedby %v / %v, desc %q", btn.DescribedBy, tip.ID, btn.Description)
		}
		// Click trigger: hover does nothing, a click opens, outside closes.
		cr := p2cFind(t, d, "Clicker")
		d.Move(cr.Center())
		if click.IsOpen() {
			t.Fatal("click tooltip opened on hover")
		}
		d.Click(cr.Center())
		ct := p2cA11y(t, d, a11y.RoleTooltip, "Click tip")
		if ct.Bounds.X < cr.Right() {
			t.Fatalf("right placement %v", ct.Bounds)
		}
		d.Click(geom.Pt(800, 450))
		if _, ok := d.A11yFind(a11y.RoleTooltip, "Click tip"); ok {
			t.Fatal("click tooltip not dismissed")
		}
		// Focus trigger.
		d.Click(p2cFind(t, d, "Focuser").Center())
		if !focus.IsOpen() {
			t.Fatal("focus tooltip")
		}
		d.Key(input.KeyEscape, 0)
		if _, ok := d.A11yFind(a11y.RoleTooltip, "Focus tip"); ok {
			t.Fatal("Escape did not hide the tooltip")
		}
		// Rich content.
		d.Move(p2cText(t, d, "Hover rich").Center())
		if !rich.IsOpen() {
			t.Fatal("rich tooltip")
		}
		p2cText(t, d, "Rich body")
	})
}

func TestP2CTooltipDelays(t *testing.T) {
	p2cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		d := NewDriver(func(c *Context) {
			Column(c, func() {
				Tooltip(c, "Slow tip", func() { Button(c, "Slow") }).MouseEnterDelay(300 * time.Millisecond).MouseLeaveDelay(200 * time.Millisecond)
			}, layout.Pad(80))
		}, DriverOptions{Width: 600, Height: 400, Theme: th(), Animations: true, Accessibility: true})
		d.Settle()
		r := p2cFind(t, d, "Slow")
		d.Move(r.Center())
		d.Frame()
		if _, ok := d.A11yFind(a11y.RoleTooltip, "Slow tip"); ok {
			t.Fatal("shown before the enter delay")
		}
		d.Advance(350 * time.Millisecond)
		d.Frame()
		d.Frame()
		p2cA11y(t, d, a11y.RoleTooltip, "Slow tip")
		d.Move(geom.Pt(550, 350))
		d.Frame()
		if _, ok := d.A11yFind(a11y.RoleTooltip, "Slow tip"); !ok {
			t.Fatal("hidden before the leave delay")
		}
		d.Advance(250 * time.Millisecond)
		d.Frame()
		d.Frame()
		if _, ok := d.A11yFind(a11y.RoleTooltip, "Slow tip"); ok {
			t.Fatal("still shown after the leave delay")
		}
	})
}

// ---------------------------------------------------------------------------
// Popconfirm

func TestP2CPopconfirmOptions(t *testing.T) {
	p2cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		var yes, no int
		disabled := true
		var pc *PopconfirmW
		d := p2cDriver(th, 900, 500, func(c *Context) {
			Column(c, func() {
				pc = Popconfirm(c, "Delete it?", "", func() { Button(c, "Remove") }).
					Description("This cannot be undone").OkText("Yes").CancelText("No").
					Icon(icon.Question).Placement("bottom").OkDanger().Disabled(disabled)
				if pc.Confirmed() {
					yes++
				}
				if pc.Cancelled() {
					no++
				}
				Popconfirm(c, "Plain?", "", func() { Button(c, "Second") }).ShowCancel(false).Placement("right")
			}, layout.Gap(16), layout.Margin(geom.Insets{Top: 40}))
		})
		d.EnableAccessibility()
		tr := p2cFind(t, d, "Remove")
		d.Click(tr.Center())
		if pc.IsOpen() {
			t.Fatal("disabled popconfirm opened")
		}
		disabled = false
		d.Settle()
		d.Click(tr.Center())
		dlg := p2cA11y(t, d, a11y.RoleAlertDialog, "Delete it?")
		if dlg.Description != "This cannot be undone" || dlg.Bounds.Y < tr.Bottom() {
			t.Fatalf("dialog %q at %v (trigger %v)", dlg.Description, dlg.Bounds, tr)
		}
		p2cText(t, d, "This cannot be undone")
		d.Click(p2cFind(t, d, "No").Center())
		d.Click(tr.Center())
		d.Click(p2cFind(t, d, "Yes").Center())
		if yes != 1 || no != 1 {
			t.Fatalf("yes %d no %d", yes, no)
		}
		s := p2cFind(t, d, "Second")
		d.Click(s.Center())
		p2cA11y(t, d, a11y.RoleAlertDialog, "Plain?")
		if _, ok := d.Find(ZhCN.Cancel); ok {
			t.Fatal("ShowCancel(false) shows cancel")
		}
		if dl := p2cA11y(t, d, a11y.RoleAlertDialog, "Plain?"); dl.Bounds.X < s.Right() {
			t.Fatalf("right placement %v", dl.Bounds)
		}
	})
}

// ---------------------------------------------------------------------------
// Avatar, Divider, Tag, Alert

func TestP2CDisplaySmall(t *testing.T) {
	p2cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		var av *AvatarW
		var small, large, dark *TagW
		undo := 0
		d := p2cDriver(th, 800, 700, func(c *Context) {
			Column(c, func() {
				av = Avatar(c, "").Icon(icon.Team).Size(40)
				Divider(c, "Left title").Orientation("left")
				Divider(c, "Right title").Orientation("right").Plain()
				Divider(c).Dashed()
				Row(c, func() {
					small = Tag(c, "Small").Size(Small)
					large = Tag(c, "Large").Size(Large)
					dark = Tag(c, "Dark").Effect("dark").Color("success")
					Tag(c, "Plain").Effect("plain")
				}, layout.Gap(8))
				Alert(c, "info", "Saved").Action(func() {
					if Button(c, "Undo").Small().Clicked() {
						undo++
					}
				}).Show()
			}).Bg(c.Theme.BgContainer)
		})
		if r := av.n.abs; r.W != 40 {
			t.Fatalf("avatar %v", r)
		}
		d.EnableAccessibility()
		// Left: the line before the title is 5% of the width; the gap
		// after it (title padding) is free of ink. Right: mirrored.
		lb := p2cA11y(t, d, a11y.RoleSeparator, "Left title").Bounds
		rb := p2cA11y(t, d, a11y.RoleSeparator, "Right title").Bounds
		ink := func(x, cy float32) bool {
			return min(p2cLuma(p2cPixel(d, x, cy-0.5)), p2cLuma(p2cPixel(d, x, cy))) < 0.985
		}
		lcy, rcy := lb.Y+lb.H/2, rb.Y+rb.H/2
		if !ink(lb.X+lb.W*0.05-6, lcy) || ink(lb.X+lb.W*0.05+8, lcy) || !ink(lb.X+lb.W*0.5, lcy) {
			t.Fatal("left orientation")
		}
		if !ink(rb.X+rb.W*0.95+6, rcy) || ink(rb.X+rb.W*0.95-8, rcy) {
			t.Fatal("right orientation")
		}
		// The dashed line alternates ink and gaps.
		var dash *node
		var walk func(n *node)
		walk = func(n *node) {
			if dv, ok := n.paint, n.label == "" && n.ln.Margin.Top == 24; dv != nil && ok && n.abs.H == 1 {
				dash = n
			}
			for _, k := range n.kids {
				walk(k)
			}
		}
		walk(d.r.c.root)
		if dash == nil {
			t.Fatal("no dashed divider")
		}
		changes, last := 0, false
		for x := dash.abs.X; x < dash.abs.Right(); x++ {
			on := ink(x, dash.abs.Y+0.5)
			if on != last {
				changes++
				last = on
			}
		}
		if changes < 20 {
			t.Fatalf("dashed divider looks solid (%d changes)", changes)
		}
		if small.n.abs.H != 20 || large.n.abs.H < 28 {
			t.Fatalf("tag sizes %v %v", small.n.abs, large.n.abs)
		}
		if px := p2cPixel(d, dark.n.abs.X+2, dark.n.abs.Y+dark.n.abs.H/2); p2cLuma(px) > 0.8 {
			t.Fatalf("dark tag %v", px)
		}
		u := p2cFind(t, d, "Undo")
		if u.X < p2cText(t, d, "Saved").Right() {
			t.Fatal("alert action not at the right")
		}
		d.Click(u.Center())
		if undo != 1 {
			t.Fatal("alert action click")
		}
	})
}

// ---------------------------------------------------------------------------
// Steps, Carousel, Grid

func TestP2CStepsNavigationPercent(t *testing.T) {
	p2cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		cur := 0
		items := []StepItem{{Title: "First"}, {Title: "Second", SubTitle: "sub"}, {Title: "Third", Description: "desc"}}
		d := p2cDriver(th, 900, 400, func(c *Context) {
			Steps(c, items).Type("navigation").Clickable(&cur).Show()
			Gap(c, 24)
			Steps(c, []StepItem{{Title: "Upload"}, {Title: "Check"}}).Current(1).Percent(60).Show()
		})
		d.EnableAccessibility()
		a, b := p2cFind(t, d, "First"), p2cFind(t, d, "Second")
		if math.Abs(float64(a.Y-b.Y)) > 0.5 || b.X < a.Right()-1 {
			t.Fatalf("navigation steps %v %v", a, b)
		}
		d.Click(p2cFind(t, d, "Third").Center())
		if cur != 2 {
			t.Fatalf("navigation click %d", cur)
		}
		if n := p2cA11y(t, d, a11y.RoleButton, "Third"); n.States&a11y.Current == 0 {
			t.Fatal("current step state")
		}
		if n := p2cA11y(t, d, a11y.RoleListItem, "Check"); n.Value != "60%" {
			t.Fatalf("percent value %q", n.Value)
		}
	})
}

func TestP2CCarouselCard(t *testing.T) {
	p2cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		cur := 0
		d := p2cDriver(th, 632, 400, func(c *Context) {
			Carousel(c, 4, func(i int) {
				Column(c, func() { Text(c, fmt.Sprint("Card ", i)) }, layout.Grow(1), layout.Center())
			}).Card().Height(200).Current(&cur).Show()
		})
		mid, right, left := p2cText(t, d, "Card 0"), p2cText(t, d, "Card 1"), p2cText(t, d, "Card 3")
		if !(left.X < mid.X && mid.X < right.X) {
			t.Fatalf("cards %v %v %v", left, mid, right)
		}
		if _, ok := d.FindText("Card 2"); ok {
			t.Fatal("the card opposite the current one is shown")
		}
		d.Click(geom.Pt(16+600-20, 16+100))
		if cur != 1 {
			t.Fatalf("side card click: %d", cur)
		}
		d.Click(geom.Pt(16+20, 16+100))
		if cur != 0 {
			t.Fatalf("left card click: %d", cur)
		}
	})
}

func TestP2CGridPushPullOrder(t *testing.T) {
	p2cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		d := p2cDriver(th, 816, 300, func(c *Context) {
			GridRow(c).Body(func() {
				Col(c, 18).Push(6).Body(func() { Button(c, "col-18 push") })
				Col(c, 6).Pull(18).Body(func() { Button(c, "col-6 pull") })
			})
			GridRow(c).Body(func() {
				Col(c, 12).Order(2).Body(func() { Text(c, "order 2") })
				Col(c, 12).Order(1).Body(func() { Text(c, "order 1") })
			})
		})
		push, pull := p2cFind(t, d, "col-18 push"), p2cFind(t, d, "col-6 pull")
		if math.Abs(float64(push.X-(16+6*784.0/24))) > 1 || math.Abs(float64(pull.X-16)) > 1 {
			t.Fatalf("push/pull %v %v", push, pull)
		}
		o1, o2 := p2cText(t, d, "order 1"), p2cText(t, d, "order 2")
		if !(o1.X < o2.X) {
			t.Fatalf("order %v %v", o1, o2)
		}
	})
}

// ---------------------------------------------------------------------------
// Sider, ProLayout, PageContainer, ConfigProvider

func TestP2CSiderTriggerBreakpoint(t *testing.T) {
	p2cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		collapsed := false
		var side *Box
		var sw *SiderW
		d := NewDriver(func(c *Context) {
			Layout(c, func() {
				sw = CollapsibleSider(c, 200, &collapsed).Breakpoint(LG)
				side = sw.Body(func() { Text(c, "menu") })
				Content(c, func() { Text(c, "content") })
			})
		}, DriverOptions{Width: 1600, Height: 500, Theme: th(), Accessibility: true})
		d.Settle()
		if collapsed || side.Rect().W != 200 {
			t.Fatalf("wide: collapsed %v width %v", collapsed, side.Rect().W)
		}
		trig := p2cA11y(t, d, a11y.RoleButton, ZhCN.CollapseMenu)
		if trig.States&a11y.Expanded == 0 {
			t.Fatal("trigger expanded state")
		}
		d.Click(p2cFind(t, d, ZhCN.CollapseMenu).Center())
		if !collapsed || side.Rect().W != d.r.c.Theme.SiderCollapsed {
			t.Fatalf("trigger: collapsed %v width %v", collapsed, side.Rect().W)
		}
		d.Click(p2cFind(t, d, ZhCN.ExpandMenu).Center())
		if collapsed {
			t.Fatal("trigger did not expand")
		}
		// Breakpoint: narrow collapses, wide expands again.
		d.Resize(700, 500)
		d.Settle()
		if !collapsed || !sw.Broken() {
			t.Fatal("breakpoint collapse")
		}
		d.Resize(1600, 500)
		d.Settle()
		if collapsed {
			t.Fatal("breakpoint expand")
		}
	})
}

func TestP2CProLayoutModes(t *testing.T) {
	p2cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		menu := []MenuItem{{Key: "a", Label: "Dashboard"}, {Key: "b", Label: "Orders"}}
		sel := "a"
		collapsed := false
		mode, nopad := "top", false
		d := NewDriver(func(c *Context) {
			ProLayout(c, ProLayoutConfig{Title: "Admin", Menu: menu, Selected: &sel, Collapsed: &collapsed,
				Layout: mode, NoPadding: nopad, Breadcrumb: []string{"Home", "Orders"}, UserName: "Lee"}, func() { Text(c, "Page body") })
		}, DriverOptions{Width: 1200, Height: 600, Theme: th(), Accessibility: true})
		d.Settle()
		p2cA11y(t, d, a11y.RoleMenuBar, "")
		if _, ok := d.A11yFind(a11y.RoleNavigation, ZhCN.A11y.Navigation); ok {
			t.Fatal("top layout has a sider")
		}
		d.Click(p2cFind(t, d, "Orders").Center())
		if sel != "b" {
			t.Fatalf("top menu click %q", sel)
		}
		top := p2cText(t, d, "Page body")
		mode = "mix"
		d.Settle()
		d.Settle()
		p2cA11y(t, d, a11y.RoleNavigation, ZhCN.A11y.Navigation)
		logo := p2cText(t, d, "Admin")
		body := p2cText(t, d, "Page body")
		if logo.Y > body.Y || body.X < d.r.c.Theme.SiderWidth {
			t.Fatalf("mix: logo %v body %v", logo, body)
		}
		d.Click(p2cFind(t, d, ZhCN.CollapseMenu).Center())
		if !collapsed {
			t.Fatal("mix trigger")
		}
		mode = "side"
		collapsed = false
		d.Settle()
		padded := p2cText(t, d, "Page body")
		nopad = true
		d.Settle()
		flush := p2cText(t, d, "Page body")
		if math.Abs(float64(padded.X-flush.X-24)) > 0.5 || math.Abs(float64(padded.Y-flush.Y-24)) > 0.5 {
			t.Fatalf("NoPadding: %v -> %v", padded, flush)
		}
		_ = top
	})
}

func TestP2CPageContainerTabs(t *testing.T) {
	p2cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		tab := "a"
		d := p2cDriver(th, 900, 500, func(c *Context) {
			PageContainer(c, "Page title").Tabs(&tab, []TabItem{{Key: "a", Label: "Overview"}, {Key: "b", Label: "Detail"}}).
				Body(func() { Text(c, "Body text") })
		})
		tr := p2cFind(t, d, "Overview")
		body := p2cText(t, d, "Body text")
		pad := float32(24)
		if navEl(d.r.c) {
			pad = 20
		}
		if gap := body.Y - tr.Bottom(); math.Abs(float64(gap-pad)) > 0.5 {
			t.Fatalf("tab bar to body gap %v (want %v)", gap, pad)
		}
		d.EnableAccessibility()
		title := p2cA11y(t, d, a11y.RoleHeading, "Page title").Bounds
		if math.Abs(float64(title.X-tr.X)) > 0.5 {
			t.Fatalf("title %v and tabs %v not aligned", title, tr)
		}
		d.Click(p2cFind(t, d, "Detail").Center())
		if tab != "b" {
			t.Fatal("page container tab")
		}
	})
}

func TestP2CConfigDefaults(t *testing.T) {
	p2cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		active, page := "a", 1
		var tag *TagW
		var tip *TooltipW
		d := p2cDriver(th, 900, 700, func(c *Context) {
			ConfigProvider(c).
				Defaults("Tabs", Props{"placement": "left", "type": "card"}).
				Defaults("Pagination", Props{"showQuickJumper": true}).
				Defaults("Breadcrumb", Props{"separator": "»"}).
				Defaults("Tag", Props{"size": Small}).
				Defaults("Tooltip", Props{"placement": "bottom"}).
				Defaults("Popconfirm", Props{"okText": "Sure"}).
				Defaults("Steps", Props{"type": "navigation"}).
				Body(func() {
					Tabs(c, &active, []TabItem{{Key: "a", Label: "One"}, {Key: "b", Label: "Two"}}).Show()
					Pagination(c, &page, 100, 10).Show()
					Breadcrumb(c, "x", "y")
					tag = Tag(c, "t")
					tip = Tooltip(c, "Tip", func() { Button(c, "Tipped") })
					Popconfirm(c, "Sure?", "", func() { Button(c, "Ask") })
				})
		})
		d.EnableAccessibility()
		if l := p2cA11y(t, d, a11y.RoleTabList, ""); l.States&a11y.Vertical == 0 {
			t.Fatal("Tabs placement default")
		}
		p2cA11y(t, d, a11y.RoleTextbox, ZhCN.Pagination.Jump)
		p2cText(t, d, "»")
		if tag.n.abs.H != 20 {
			t.Fatalf("Tag size default %v", tag.n.abs)
		}
		b := p2cFind(t, d, "Tipped")
		d.Move(b.Center())
		if tt := p2cA11y(t, d, a11y.RoleTooltip, "Tip"); !tip.IsOpen() || tt.Bounds.Y < b.Bottom() {
			t.Fatal("Tooltip placement default")
		}
		d.Click(p2cFind(t, d, "Ask").Center())
		p2cFind(t, d, "Sure")
	})
}

func TestP2CPageContainerInProLayout(t *testing.T) {
	p2cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		ghost := false
		d := NewDriver(func(c *Context) {
			ProLayout(c, ProLayoutConfig{Title: "Admin", Menu: []MenuItem{{Key: "a", Label: "Home"}}}, func() {
				pc := PageContainer(c, "Orders")
				if ghost {
					pc.Ghost()
				}
				pc.Body(func() { Text(c, "Order list") })
			})
		}, DriverOptions{Width: 1200, Height: 600, Theme: th(), Accessibility: true})
		d.Settle()
		group := p2cA11y(t, d, a11y.RoleGroup, "Orders").Bounds
		sw := d.r.c.Theme.SiderWidth
		if math.Abs(float64(group.X-sw)) > 0.5 || math.Abs(float64(group.Y-d.r.c.Theme.HeaderHeight)) > 0.5 {
			t.Fatalf("page container not flush with the content area: %v", group)
		}
		ghost = true
		d.Settle()
		if g := p2cA11y(t, d, a11y.RoleGroup, "Orders").Bounds; math.Abs(float64(g.X-sw-24)) > 0.5 {
			t.Fatalf("ghost page container should keep the padding: %v", g)
		}
	})
}
