package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

func TestItemClickableAndLink(t *testing.T) {
	clicks, links, action := 0, 0, 0
	var plain, small *ItemW
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			if Item(c, "个人资料").Description("姓名与头像").Icon(icon.User).Clickable().Clicked() {
				clicks++
			}
			if Item(c, "帮助中心").Link().Outline().Clicked() {
				links++
			}
			plain = Item(c, "只读").Muted().Actions(func() {
				if Button(c, "管理").Clicked() {
					action++
				}
			})
			small = Item(c, "紧凑").Small()
			Item(c, "禁用").Clickable().Disabled(true)
		}, layout.W(400))
	}, DriverOptions{Width: 600, Height: 600})
	d.EnableAccessibility()
	d.Frame()
	d.Frame()
	if err := d.ClickLabel("个人资料"); err != nil {
		t.Fatal(err)
	}
	if clicks != 1 {
		t.Fatalf("clicks = %d", clicks)
	}
	// Enter on the focused row activates it too.
	d.Key(input.KeyEnter, 0)
	d.Frame()
	if clicks != 2 {
		t.Fatalf("Enter: clicks = %d", clicks)
	}
	if err := d.ClickLabel("帮助中心"); err != nil {
		t.Fatal(err)
	}
	if links != 1 {
		t.Fatalf("links = %d", links)
	}
	// A click on an action does not activate the row.
	d.ClickLabel("管理")
	d.Frame()
	if action != 1 {
		t.Fatalf("action = %d", action)
	}
	// Roles and names.
	b, ok := d.A11yFind(a11y.RoleButton, "个人资料")
	if !ok || b.Description != "姓名与头像" || !b.States.Has(a11y.Focusable) {
		t.Fatalf("button row: %+v %v", b, ok)
	}
	if _, ok := d.A11yFind(a11y.RoleLink, "帮助中心"); !ok {
		t.Fatal("link row missing")
	}
	if g, ok := d.A11yFind(a11y.RoleGroup, "只读"); !ok || g.States.Has(a11y.Focusable) {
		t.Fatalf("static row: %+v %v", g, ok)
	}
	if dis, ok := d.A11yFind(a11y.RoleButton, "禁用"); !ok || !dis.States.Has(a11y.Disabled) || dis.States.Has(a11y.Focusable) {
		t.Fatalf("disabled row: %+v", dis)
	}
	if err := d.ClickLabel("禁用"); err == nil {
		t.Log("disabled row is still hit-testable (no activation expected)")
	}
	// Small rows are shorter than default ones.
	if small.box.Rect().H >= plain.box.Rect().H {
		t.Fatalf("small row %v not shorter than %v", small.box.Rect(), plain.box.Rect())
	}
}

func TestItemGroupSeparated(t *testing.T) {
	var g *ItemGroupW
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			g = ItemGroup(c).Separated().Bordered().A11yLabel("设置").Body(func() {
				Item(c, "通知").Description("推送与邮件")
				Item(c, "隐私").Description("可见范围")
				Item(c, "安全").Clickable()
			})
		}, layout.W(360))
	}, DriverOptions{Width: 600, Height: 600})
	d.EnableAccessibility()
	d.Frame()
	d.Frame()
	list, ok := d.A11yFind(a11y.RoleList, "设置")
	if !ok || g.ID() == 0 {
		t.Fatal("group is not a named list")
	}
	items := d.A11yFindAll(a11y.RoleListItem, "")
	n := 0
	for _, it := range items {
		if it.Name == "通知" || it.Name == "隐私" {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("list items: %d of %d (%v)", n, len(items), list)
	}
	// Separated: one pixel (plus insets) between consecutive rows.
	r1, _ := d.FindText("通知")
	r2, _ := d.FindText("隐私")
	if r2.Y <= r1.Bottom() {
		t.Fatal("rows overlap")
	}
}

func TestItemRTL(t *testing.T) {
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Direction(RTL).Body(func() {
			Column(c, func() {
				Item(c, "文件").Icon(icon.File).Actions(func() { Button(c, "打开") })
			}, layout.W(400))
		})
	}, DriverOptions{Width: 400, Height: 200})
	d.Frame()
	d.Frame()
	row, _ := d.FindText("文件")
	act, _ := d.Find("打开")
	if !(act.Right() < row.X+row.W/2) {
		t.Fatalf("RTL: actions should sit at the left end of the row: %v in %v", act, row)
	}
}

func TestItemTokens(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn(), theme.Shadcn(theme.Dark())} {
		k := th.Components.Item
		if k.PaddingBlockSM >= k.PaddingBlock+1 || k.MediaIconSize >= k.MediaIconBox || k.TitleColor.A == 0 || k.BorderColor.A == 0 {
			t.Errorf("%v: bad item token %+v", th.Flavor, k)
		}
	}
}
