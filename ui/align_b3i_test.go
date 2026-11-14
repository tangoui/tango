package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// Batch 3, group I (docs/specs/align/3-menu.md): Menu, Dropdown, Menubar,
// ContextMenu and Command behaviour.

var b3iItems = []MenuItem{
	{Key: "home", Label: "Home"},
	{Key: "sub", Label: "Sub", Children: []MenuItem{{Key: "a", Label: "Alpha"}, {Key: "b", Label: "Beta"}}},
	{Key: "sub2", Label: "Sub2", Children: []MenuItem{{Key: "c", Label: "Gamma"}}},
	{Key: "off", Label: "Off", Disabled: true},
}

func b3iDriver(page func(*Context), th *theme.Theme) *Driver {
	return NewDriver(page, DriverOptions{Width: 960, Height: 600, Theme: th})
}

// b3iTimed is b3iDriver with animations (hover delays) on.
func b3iTimed(page func(*Context), th *theme.Theme) *Driver {
	return NewDriver(page, DriverOptions{Width: 960, Height: 600, Theme: th, Animations: true})
}

// b3iTextX returns the origin x of the first text op drawn in color col.
func b3iTextX(d *Driver, col theme.Color) (float32, bool) {
	for _, op := range d.r.List().Ops {
		if t, ok := op.(paint.Text); ok && theme.Color(t.Color) == col {
			return t.Origin.X, true
		}
	}
	return 0, false
}

func TestMenuOpenKeysControlled(t *testing.T) {
	open := []string{"sub2"}
	changed := false
	sel := ""
	d := b3iDriver(func(c *Context) {
		changed = changed || Menu(c, b3iItems, &sel).Width(240).OpenKeys(&open).Show().OpenChanged()
	}, nil)
	d.Frame()
	if _, ok := d.Find("Gamma"); !ok {
		t.Fatal("bound open key sub2 not open")
	}
	if _, ok := d.Find("Alpha"); ok {
		t.Fatal("sub open without its key")
	}
	d.ClickLabel("Sub")
	d.Frame()
	if !changed || len(open) != 2 || open[0] != "sub" || open[1] != "sub2" {
		t.Fatalf("open keys after click = %v (changed %v), want [sub sub2]", open, changed)
	}
	open = nil // the app closes everything
	d.Frame()
	if _, ok := d.Find("Alpha"); ok {
		t.Fatal("cleared open keys left sub open")
	}
}

func TestMenuOutOfRangeKeys(t *testing.T) {
	sel := "missing"
	open := []string{"nope", "home"}
	d := b3iDriver(func(c *Context) { Menu(c, b3iItems, &sel).Width(240).OpenKeys(&open).Show() }, nil)
	d.EnableAccessibility()
	d.Frame()
	for _, n := range d.A11yFindAll(a11y.RoleMenuItem, "") {
		if n.States&a11y.Selected != 0 {
			t.Fatalf("unknown selected key selected %q", n.Name)
		}
	}
	if _, ok := d.Find("Alpha"); ok {
		t.Fatal("unknown open keys opened a submenu")
	}
}

func TestMenuSelectableFalse(t *testing.T) {
	sel := "home"
	var clicked string
	d := b3iDriver(func(c *Context) {
		if k := Menu(c, b3iItems, &sel).Width(240).Selectable(false).DefaultOpen("sub").Show().Clicked(); k != "" {
			clicked = k
		}
	}, nil)
	d.Frame()
	d.ClickLabel("Alpha")
	if clicked != "a" || sel != "home" {
		t.Fatalf("Selectable(false): clicked %q sel %q, want a / home", clicked, sel)
	}
}

func TestMenuSubMenuDelay(t *testing.T) {
	sel := ""
	d := b3iTimed(func(c *Context) { Menu(c, b3iItems, &sel).Mode(MenuVertical).Width(200).Show() }, nil)
	d.Frame()
	r, _ := d.Find("Sub")
	d.Move(r.Center())
	d.Frame()
	if _, ok := d.Find("Alpha"); ok {
		t.Fatal("antd submenu opened before subMenuOpenDelay 0.1 s")
	}
	d.Advance(110 * time.Millisecond)
	d.Frame()
	d.Frame()
	if _, ok := d.Find("Alpha"); !ok {
		t.Fatal("submenu not open after 0.1 s")
	}
	d.Move(geom.Pt(900, 500))
	d.Frame()
	d.Advance(50 * time.Millisecond)
	d.Frame()
	if _, ok := d.Find("Alpha"); !ok {
		t.Fatal("submenu closed before subMenuCloseDelay")
	}
	d.Advance(80 * time.Millisecond)
	d.Frame()
	d.Frame()
	if _, ok := d.Find("Alpha"); ok {
		t.Fatal("submenu still open after subMenuCloseDelay")
	}
}

func TestMenuTriggerSubMenuClick(t *testing.T) {
	sel := ""
	d := b3iDriver(func(c *Context) {
		Menu(c, b3iItems, &sel).Mode(MenuHorizontal).TriggerSubMenuAction(TriggerClick).Show()
	}, nil)
	d.Frame()
	r, _ := d.Find("Sub")
	d.Move(r.Center())
	d.Advance(time.Second)
	d.Frame()
	d.Frame()
	if _, ok := d.Find("Alpha"); ok {
		t.Fatal("click trigger opened on hover")
	}
	d.ClickLabel("Sub")
	d.Frame()
	if _, ok := d.Find("Alpha"); !ok {
		t.Fatal("click did not open the submenu")
	}
	d.Move(geom.Pt(900, 500))
	d.Advance(time.Second)
	d.Frame()
	if _, ok := d.Find("Alpha"); !ok {
		t.Fatal("click submenu closed when the pointer left")
	}
	d.ClickLabel("Alpha")
	d.Frame()
	if sel != "a" {
		t.Fatalf("chose %q", sel)
	}
	if _, ok := d.Find("Alpha"); ok {
		t.Fatal("submenu stayed open after a choice")
	}
	// A press outside closes a click-opened submenu.
	d.ClickLabel("Sub")
	d.Frame()
	if _, ok := d.Find("Alpha"); !ok {
		t.Fatal("click did not reopen the submenu")
	}
	d.Click(geom.Pt(900, 500))
	d.Frame()
	if _, ok := d.Find("Alpha"); ok {
		t.Fatal("outside press left the submenu open")
	}
}

func TestMenuCollapsedClickTooltip(t *testing.T) {
	sel := ""
	d := b3iDriver(func(c *Context) { Menu(c, b3iItems, &sel).Collapsed(true).TriggerSubMenuAction(TriggerClick).Show() }, nil)
	d.Frame()
	r, _ := d.Find("Home")
	d.Move(r.Center())
	d.Advance(time.Second)
	d.Frame()
	d.Frame()
	if _, ok := d.FindText("Home"); !ok {
		t.Fatal("no Home")
	}
	n := 0
	for _, o := range d.r.c.overlays {
		if o.root.drawn == d.r.c.pass {
			n++
		}
	}
	if n == 0 {
		t.Fatal("collapsed leaf lost its tooltip with a click trigger")
	}
	r, _ = d.Find("Sub")
	d.Move(r.Center())
	d.Advance(time.Second)
	d.Frame()
	d.Frame()
	if _, ok := d.Find("Alpha"); ok {
		t.Fatal("click trigger opened a submenu on hover")
	}
}

func TestMenuSubItemHeightFollowsItemHeight(t *testing.T) {
	sel := ""
	d := b3iDriver(func(c *Context) {
		th := c.ThemeFor(theme.CompMenu)
		comps := th.Components
		comps.Menu.ItemHeight = 48
		ConfigProvider(c).Components(comps).Body(func() { Menu(c, b3iItems, &sel).Width(240).DefaultOpen("sub").Show() })
	}, nil)
	d.Settle()
	if h := r2Node(d, "Alpha").abs.H; h != 48 {
		t.Fatalf("inline sub item %v high, want ItemHeight 48", h)
	}
}

func TestMenuDarkDanger(t *testing.T) {
	sel := ""
	items := []MenuItem{{Key: "x", Label: "Delete", Danger: true}, {Key: "y", Label: "Keep"}}
	d := b3iDriver(func(c *Context) { Menu(c, items, &sel).Dark().Width(240).Show() }, nil)
	d.Frame()
	tk := d.r.c.Theme.Components.Menu
	if _, ok := b3iTextX(d, tk.DarkDangerItemColor); !ok {
		t.Fatal("dark danger item not drawn in darkDangerItemColor")
	}
	sel = "x"
	d.Frame()
	if r2Fills(d, tk.DarkDangerItemSelectedBg, -1) == 0 {
		t.Fatal("selected dark danger item not filled with darkDangerItemSelectedBg")
	}
}

func TestMenuTooltipOff(t *testing.T) {
	sel := ""
	for _, on := range []bool{true, false} {
		d := b3iDriver(func(c *Context) { Menu(c, b3iItems, &sel).Collapsed(true).Tooltip(on).Show() }, nil)
		d.Frame()
		r, _ := d.Find("Home")
		d.Move(r.Center())
		d.Advance(time.Second)
		d.Frame()
		d.Frame()
		n := 0
		for _, o := range d.r.c.overlays {
			if o.root.drawn == d.r.c.pass {
				n++
			}
		}
		if (n > 0) != on {
			t.Errorf("Tooltip(%v): %d overlays", on, n)
		}
	}
}

func TestMenuDangerItem(t *testing.T) {
	sel := ""
	items := []MenuItem{{Key: "x", Label: "Delete", Danger: true}, {Key: "y", Label: "Keep"}}
	d := b3iDriver(func(c *Context) { Menu(c, items, &sel).Width(240).Show() }, nil)
	d.Frame()
	th := d.r.c.Theme
	if _, ok := b3iTextX(d, th.Components.Menu.DangerItemColor); !ok {
		t.Fatal("danger item not drawn in dangerItemColor")
	}
	sel = "x"
	d.Frame()
	if r2Fills(d, th.Components.Menu.DangerItemSelectedBg, -1) == 0 {
		t.Fatal("selected danger item not filled with dangerItemSelectedBg")
	}
}

func TestMenuInlineIndent(t *testing.T) {
	sel := ""
	items := []MenuItem{{Key: "x", Label: "Item"}}
	x := func(px float32) float32 {
		d := b3iDriver(func(c *Context) {
			m := Menu(c, items, &sel).Width(240)
			if px > 0 {
				m.InlineIndent(px)
			}
			m.Show()
		}, nil)
		d.Frame()
		v, ok := b3iTextX(d, d.r.c.Theme.Components.Menu.ItemColor)
		if !ok {
			t.Fatal("label not drawn")
		}
		return v
	}
	if a, b := x(0), x(40); b-a != 16 {
		t.Fatalf("InlineIndent(40) moved the label by %v, want 16 (40 - antd 24)", b-a)
	}
}

func TestMenuHorizontalEllipsis(t *testing.T) {
	sel := ""
	var items []MenuItem
	for _, s := range []string{"One", "Two", "Three", "Four", "Five", "Six", "Seven", "Eight"} {
		items = append(items, MenuItem{Key: s, Label: s + " navigation"})
	}
	for _, on := range []bool{true, false} {
		d := NewDriver(func(c *Context) {
			Column(c, func() { Menu(c, items, &sel).Mode(MenuHorizontal).Ellipsis(on).Show() }, layout.Pad(16))
		}, DriverOptions{Width: 500, Height: 300})
		for range 3 {
			d.Settle()
		}
		_, more := d.Find(d.r.c.Locale().More)
		if more != on {
			t.Errorf("Ellipsis(%v): overflow item shown = %v", on, more)
		}
	}
}

func TestMenuItemPanel(t *testing.T) {
	sel := ""
	items := []MenuItem{{Key: "docs", Label: "Docs"}, {Key: "home", Label: "Home"}}
	d := b3iTimed(func(c *Context) {
		Menu(c, items, &sel).Mode(MenuHorizontal).ItemPanel("docs", func() { Text(c, "Panel body") }).Show()
	}, theme.Shadcn())
	d.EnableAccessibility()
	d.Frame()
	if n, ok := d.A11yFind(a11y.RoleMenuItem, "Docs"); !ok || n.States&a11y.HasPopup == 0 {
		t.Fatal("ItemPanel item not announced with a popup")
	}
	r, _ := d.Find("Docs")
	d.Move(r.Center())
	d.Advance(300 * time.Millisecond)
	d.Frame()
	d.Frame()
	if _, ok := d.FindText("Panel body"); !ok {
		t.Fatal("hover did not open the item panel")
	}
	if _, ok := d.A11yFind(a11y.RoleGroup, "Docs"); !ok {
		t.Fatal("item panel not named after its item")
	}
	// From the keyboard: Down opens it, Escape closes it and focus stays
	// on the item.
	d.Move(geom.Pt(900, 550))
	d.Advance(time.Second)
	d.Frame()
	d.Frame()
	if _, ok := d.FindText("Panel body"); ok {
		t.Fatal("leaving did not close the item panel")
	}
	press(d, input.KeyTab)
	wantFocus(t, d, "Docs")
	press(d, input.KeyDown)
	d.Frame()
	if _, ok := d.FindText("Panel body"); !ok {
		t.Fatal("Down did not open the item panel")
	}
	press(d, input.KeyEscape)
	d.Frame()
	if _, ok := d.FindText("Panel body"); ok {
		t.Fatal("Escape did not close the item panel")
	}
	wantFocus(t, d, "Docs")
}

func TestMenuShadcnSidebarMetrics(t *testing.T) {
	sel := ""
	d := b3iDriver(func(c *Context) { Menu(c, b3iItems, &sel).Collapsed(true).Show() }, theme.Shadcn())
	d.Settle()
	n := r2Node(d, "Home")
	if n == nil || n.parent == nil {
		t.Fatal("no Home item")
	}
	if w := d.r.c.rects[n.parent.id].W; w != 48 {
		t.Fatalf("collapsed shadcn sidebar %v wide, want 48 (--sidebar-width-icon 3rem)", w)
	}
	sel2 := ""
	d = b3iDriver(func(c *Context) { Menu(c, b3iItems, &sel2).Width(256).DefaultOpen("sub").Show() }, theme.Shadcn())
	d.Settle()
	if h := r2Node(d, "Alpha").abs.H; h != 28 {
		t.Fatalf("SidebarMenuSubButton %v high, want 28 (h-7)", h)
	}
	if x := r2Node(d, "Alpha").abs.X; x != 33 {
		t.Fatalf("sub button at x %v, want 33 (p-2 + mx-3.5 + border + px-2.5)", x)
	}
}

func TestMenuDisabledConfig(t *testing.T) {
	sel := ""
	d := b3iDriver(func(c *Context) {
		ConfigProvider(c).ComponentDisabled(true).Body(func() { Menu(c, b3iItems, &sel).Width(240).Show() })
	}, nil)
	d.EnableAccessibility()
	d.Frame()
	d.ClickLabel("Home")
	d.ClickLabel("Sub")
	d.Frame()
	if sel != "" {
		t.Fatalf("disabled menu selected %q", sel)
	}
	if _, ok := d.Find("Alpha"); ok {
		t.Fatal("disabled menu opened a submenu")
	}
	if n, ok := d.A11yFind(a11y.RoleMenuItem, "Home"); !ok || n.States&a11y.Disabled == 0 {
		t.Fatal("ComponentDisabled item not announced disabled")
	}
}

func TestMenuA11yDisabledItem(t *testing.T) {
	sel := ""
	d := b3iDriver(func(c *Context) { Menu(c, b3iItems, &sel).Width(240).Show() }, nil)
	d.EnableAccessibility()
	d.Frame()
	if n, ok := d.A11yFind(a11y.RoleMenuItem, "Off"); !ok || n.States&a11y.Disabled == 0 {
		t.Fatal("disabled item not announced disabled")
	}
	if _, ok := d.A11yFind(a11y.RoleMenu, ""); !ok {
		t.Fatal("no menu role")
	}
}

// --- Dropdown ---------------------------------------------------------

var b3iDropItems = []MenuItem{{Key: "a", Label: "Alpha"}, {Key: "b", Label: "Beta"}, {Key: "c", Label: "Gamma"}, {Key: "x", Label: "Off", Disabled: true}}

func TestDropdownHoverDelays(t *testing.T) {
	d := b3iTimed(func(c *Context) {
		Dropdown(c, b3iDropItems).MouseEnterDelay(400 * time.Millisecond).Body(func() { Button(c, "Open") })
	}, nil)
	d.Frame()
	r, _ := d.Find("Open")
	d.Move(r.Center())
	d.Frame()
	d.Advance(300 * time.Millisecond)
	d.Frame()
	if _, ok := d.Find("Alpha"); ok {
		t.Fatal("opened before MouseEnterDelay")
	}
	d.Advance(150 * time.Millisecond)
	d.Frame()
	d.Frame()
	if _, ok := d.Find("Alpha"); !ok {
		t.Fatal("not open after MouseEnterDelay")
	}
}

func TestDropdownElementLeaveDelay(t *testing.T) {
	d := b3iTimed(func(c *Context) {
		Dropdown(c, b3iDropItems).Body(func() { Button(c, "Open") })
	}, theme.ElementPlus())
	d.Frame()
	r, _ := d.Find("Open")
	d.Move(r.Center())
	d.Advance(200 * time.Millisecond)
	d.Frame()
	d.Frame()
	if _, ok := d.Find("Alpha"); !ok {
		t.Fatal("not open")
	}
	d.Move(geom.Pt(900, 550))
	d.Frame()
	d.Advance(120 * time.Millisecond)
	d.Frame()
	if _, ok := d.Find("Alpha"); !ok {
		t.Fatal("Element closed before hide-timeout 150 ms")
	}
	d.Advance(60 * time.Millisecond)
	d.Frame()
	d.Frame()
	if _, ok := d.Find("Alpha"); ok {
		t.Fatal("Element still open after hide-timeout")
	}
}

func TestDropdownCheckRadio(t *testing.T) {
	bold := false
	align := "a"
	items := []MenuItem{{Key: "bold", Label: "Bold"}, {Key: "a", Label: "Left"}, {Key: "b", Label: "Right"}}
	d := b3iDriver(func(c *Context) {
		Dropdown(c, items).Trigger(TriggerClick).Check("bold", &bold).Radio(&align, "a", "b").Body(func() { Button(c, "Format") })
	}, nil)
	d.EnableAccessibility()
	d.Frame()
	d.ClickLabel("Format")
	d.Frame()
	n, ok := d.A11yFind(a11y.RoleMenuItem, "Left")
	if !ok || n.States&a11y.Checkable == 0 || n.States&a11y.Checked == 0 {
		t.Fatal("radio item Left not checked")
	}
	d.ClickLabel("Bold")
	if !bold {
		t.Fatal("check item did not toggle")
	}
	d.ClickLabel("Format")
	d.Frame()
	d.ClickLabel("Right")
	if align != "b" {
		t.Fatalf("radio value %q, want b", align)
	}
}

func TestDropdownHideOnClickFalse(t *testing.T) {
	var got []string
	d := b3iDriver(func(c *Context) {
		if k := Dropdown(c, b3iDropItems).Trigger(TriggerClick).HideOnClick(false).Body(func() { Button(c, "Open") }).Clicked(); k != "" {
			got = append(got, k)
		}
	}, nil)
	d.Frame()
	d.ClickLabel("Open")
	d.Frame()
	d.ClickLabel("Alpha")
	d.Frame()
	d.ClickLabel("Beta")
	d.Frame()
	if len(got) != 2 {
		t.Fatalf("chose %v, want two choices with the menu kept open", got)
	}
	if _, ok := d.Find("Gamma"); !ok {
		t.Fatal("menu closed")
	}
}

func TestDropdownPopupRender(t *testing.T) {
	d := b3iDriver(func(c *Context) {
		Dropdown(c, b3iDropItems).Trigger(TriggerClick).PopupRender(func(menu func()) {
			menu()
			Text(c, "Footer text")
		}).Body(func() { Button(c, "Open") })
	}, nil)
	d.Frame()
	d.ClickLabel("Open")
	d.Frame()
	if _, ok := d.FindText("Footer text"); !ok {
		t.Fatal("PopupRender content missing")
	}
	if _, ok := d.Find("Alpha"); !ok {
		t.Fatal("menu missing inside PopupRender")
	}
}

func TestDropdownAutoFocusAndSource(t *testing.T) {
	var src []string
	d := b3iDriver(func(c *Context) {
		dd := Dropdown(c, b3iDropItems).Trigger(TriggerClick).AutoFocus(true).Body(func() { Button(c, "Open") })
		if dd.OpenChanged() {
			src = append(src, dd.OpenSource())
		}
	}, nil)
	d.Frame()
	d.ClickLabel("Open")
	d.Frame()
	d.Frame()
	wantFocus(t, d, "Alpha")
	d.ClickLabel("Beta")
	d.Frame()
	if len(src) != 2 || src[0] != "trigger" || src[1] != "menu" {
		t.Fatalf("open sources %v, want [trigger menu]", src)
	}
}

func TestDropdownArrowSides(t *testing.T) {
	for _, pl := range []Placement{BottomStart, Right, Left, TopEnd} {
		d := b3iDriver(func(c *Context) {
			Column(c, func() {
				Dropdown(c, b3iDropItems).Trigger(TriggerClick).Placement(pl).ArrowPointAtCenter(true).Body(func() { Button(c, "Open") })
			}).Style(layout.Pad(300))
		}, nil)
		d.Frame()
		d.ClickLabel("Open")
		d.Frame()
		if r2Fills(d, d.r.c.Theme.Components.Dropdown.PopupBg, -1) < 2 {
			t.Errorf("placement %v: arrow not drawn", pl)
		}
	}
}

func TestDropdownEscTopmostInModal(t *testing.T) {
	open := true
	d := b3iDriver(func(c *Context) {
		Modal(c, &open, "Dialog").Body(func() {
			Dropdown(c, b3iDropItems).Trigger(TriggerClick).Body(func() { Button(c, "Open") })
		})
	}, nil)
	d.Frame()
	d.Frame()
	d.ClickLabel("Open")
	d.Frame()
	press(d, input.KeyDown)
	d.Frame()
	if _, ok := d.Find("Alpha"); !ok {
		t.Fatal("dropdown not open in the modal")
	}
	press(d, input.KeyEscape)
	d.Frame()
	if _, ok := d.Find("Alpha"); ok {
		t.Fatal("Escape left the dropdown open")
	}
	if !open {
		t.Fatal("Escape closed the modal under the dropdown")
	}
	wantFocus(t, d, "Open")
	press(d, input.KeyEscape)
	d.Frame()
	if open {
		t.Fatal("second Escape did not close the modal")
	}
}

func TestDropdownDisabledConfig(t *testing.T) {
	d := b3iDriver(func(c *Context) {
		ConfigProvider(c).ComponentDisabled(true).Body(func() {
			Dropdown(c, b3iDropItems).Trigger(TriggerClick).Body(func() { Button(c, "Open") })
		})
	}, nil)
	d.Frame()
	d.ClickLabel("Open")
	d.Frame()
	if _, ok := d.Find("Alpha"); ok {
		t.Fatal("ComponentDisabled dropdown opened")
	}
}

func TestDropdownOutOfRangeSelected(t *testing.T) {
	d := b3iDriver(func(c *Context) {
		Dropdown(c, b3iDropItems).Trigger(TriggerClick).Selected("missing").Body(func() { Button(c, "Open") })
	}, nil)
	d.EnableAccessibility()
	d.Frame()
	d.ClickLabel("Open")
	d.Frame()
	for _, n := range d.A11yFindAll(a11y.RoleMenuItem, "") {
		if n.States&a11y.Selected != 0 {
			t.Fatalf("unknown selected key selected %q", n.Name)
		}
	}
}

// --- Menubar ----------------------------------------------------------

func TestMenubarShadcnPopupOffset(t *testing.T) {
	menus := []MenubarMenu{{Label: "File", Items: []MenubarItem{{Key: "new", Label: "New"}}}}
	d := b3iDriver(func(c *Context) {
		Column(c, func() { Menubar(c, menus) }).Style(layout.Pad(40))
	}, theme.Shadcn())
	d.Frame()
	d.ClickLabel("File")
	d.Frame()
	tr, _ := d.Find("File")
	var panel geom.Rect
	for _, o := range d.r.c.overlays {
		if o.root.drawn == d.r.c.pass {
			panel = d.r.c.rects[o.root.id]
		}
	}
	if panel.Y-tr.Bottom() != 8 || tr.X-panel.X != 4 || panel.W < 192 {
		t.Fatalf("panel %v under trigger %v: want sideOffset 8, alignOffset -4, min-w 192", panel, tr)
	}
}

func TestMenubarDisabledItemKeyboard(t *testing.T) {
	menus := []MenubarMenu{{Label: "File", Items: []MenubarItem{{Key: "a", Label: "Alpha"}, {Key: "x", Label: "Off", Disabled: true}, {Key: "b", Label: "Beta"}}}}
	var got string
	d := b3iDriver(func(c *Context) {
		if k := Menubar(c, menus).Clicked(); k != "" {
			got = k
		}
	}, nil)
	d.Frame()
	press(d, input.KeyTab)
	press(d, input.KeyDown)
	d.Frame()
	wantFocus(t, d, "Alpha")
	press(d, input.KeyDown)
	wantFocus(t, d, "Beta")
	press(d, input.KeyEnter)
	if got != "b" {
		t.Fatalf("chose %q", got)
	}
}

// --- Command ----------------------------------------------------------

var b3iCmdItems = []CommandItem{
	{Value: "cal", Label: "Calendar", Group: "Suggestions"},
	{Value: "emoji", Label: "Search Emoji", Group: "Suggestions"},
	{Value: "calc", Label: "Calculator", Group: "Suggestions", Disabled: true},
	{Value: "prof", Label: "Profile", Group: "Settings"},
	{Value: "bill", Label: "Billing", Group: "Settings"},
}

func b3iCommand(t *testing.T, opt func(*CommandW)) (*Driver, *CommandW, *string) {
	t.Helper()
	var w *CommandW
	picked := new(string)
	d := NewDriver(func(c *Context) {
		w = Command(c, b3iCmdItems)
		if opt != nil {
			opt(w)
		}
		if it, ok := w.Selected(); ok {
			*picked = it.Value
		}
	}, DriverOptions{Width: 400, Height: 500, Theme: theme.Shadcn()})
	d.Settle()
	d.Click(d.r.c.rects[w.in.box.id].Center())
	d.Settle()
	return d, w, picked
}

func TestCommandLoopHomeEnd(t *testing.T) {
	d, w, picked := b3iCommand(t, nil)
	d.Key(input.KeyUp, 0) // cmdk loop false: stays on the first item
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if *picked != "cal" {
		t.Fatalf("Up on the first item without Loop picked %q", *picked)
	}
	d.Key(input.KeyEnd, 0)
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if *picked != "bill" {
		t.Fatalf("End picked %q", *picked)
	}
	d.Key(input.KeyHome, 0)
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if *picked != "cal" {
		t.Fatalf("Home picked %q", *picked)
	}
	_ = w
	d, _, picked = b3iCommand(t, func(w *CommandW) { w.Loop(true) })
	d.Key(input.KeyUp, 0)
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if *picked != "bill" {
		t.Fatalf("Loop: Up from the first picked %q, want bill", *picked)
	}
}

func TestCommandVimBindings(t *testing.T) {
	d, _, picked := b3iCommand(t, nil)
	d.Key(input.KeyN, input.ModCtrl)
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if *picked != "emoji" {
		t.Fatalf("Ctrl+N picked %q", *picked)
	}
	d.Key(input.KeyK, input.ModCtrl)
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if *picked != "cal" {
		t.Fatalf("Ctrl+K picked %q", *picked)
	}
}

func TestCommandFuzzyRank(t *testing.T) {
	if commandMatch("Billing", "blg") <= 0 {
		t.Fatal("letters in order did not match")
	}
	if commandMatch("Billing", "gb") != 0 {
		t.Fatal("letters out of order matched")
	}
	if !(commandMatch("Calendar", "calendar") > commandMatch("Calendar", "cal") &&
		commandMatch("Calendar", "cal") > commandMatch("Search Emoji", "emo") &&
		commandMatch("Search Emoji", "emo") > commandMatch("Billing", "ill") &&
		commandMatch("Billing", "ill") > commandMatch("Billing", "blg")) {
		t.Fatal("rank order: exact > prefix > word start > substring > scattered")
	}
	// "e" hits Search Emoji at a word start, Calendar inside: the
	// Suggestions group ranks Search Emoji first.
	d, _, picked := b3iCommand(t, nil)
	d.Type("emo")
	d.Settle()
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if *picked != "emoji" {
		t.Fatalf("picked %q", *picked)
	}
	d.Key(input.KeyBackspace, 0)
	d.Key(input.KeyBackspace, 0)
	d.Key(input.KeyBackspace, 0)
	d.Type("prl")
	d.Settle()
	if _, ok := d.Find("Profile"); !ok {
		t.Fatal("scattered match missing")
	}
	if _, ok := d.Find("Calendar"); ok {
		t.Fatal("non-match listed")
	}
}

func TestCommandValueControlled(t *testing.T) {
	val := "prof"
	changed := false
	d, _, picked := b3iCommand(t, func(w *CommandW) {
		w.Value(&val)
		changed = changed || w.ValueChanged()
	})
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if *picked != "prof" {
		t.Fatalf("bound value did not pick the active item: %q", *picked)
	}
	d.Key(input.KeyDown, 0)
	d.Settle()
	if val != "bill" || !changed {
		t.Fatalf("value after Down %q (changed %v), want bill", val, changed)
	}
	// Idle frames report no change.
	for range 3 {
		changed = false
		d.Frame()
		if changed {
			t.Fatal("ValueChanged on an idle frame")
		}
	}
}

func TestCommandValueOutOfRange(t *testing.T) {
	val := "missing"
	d, _, picked := b3iCommand(t, func(w *CommandW) { w.Value(&val) })
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if *picked != "cal" || val != "cal" {
		t.Fatalf("unknown value: picked %q value %q, want the first item", *picked, val)
	}
}

func TestCommandShouldFilterAndFilter(t *testing.T) {
	d, _, _ := b3iCommand(t, func(w *CommandW) { w.ShouldFilter(false) })
	d.Type("zzz")
	d.Settle()
	if _, ok := d.Find("Calendar"); !ok {
		t.Fatal("ShouldFilter(false) filtered")
	}
	d, _, picked := b3iCommand(t, func(w *CommandW) {
		w.Filter(func(value, search string, _ []string) float64 {
			if value == "bill" {
				return 1
			}
			return 0
		})
	})
	d.Type("x")
	d.Settle()
	if _, ok := d.Find("Calendar"); ok {
		t.Fatal("custom filter kept Calendar")
	}
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if *picked != "bill" {
		t.Fatalf("picked %q", *picked)
	}
}

func TestCommandSearchBinding(t *testing.T) {
	q := "bil"
	d, w, _ := b3iCommand(t, func(w *CommandW) { w.Search(&q) })
	if _, ok := d.Find("Calendar"); ok {
		t.Fatal("bound search not applied")
	}
	d.Type("x")
	d.Settle()
	if q != "bilx" || w.Query() != "bilx" {
		t.Fatalf("search binding %q / %q", q, w.Query())
	}
}

func TestCommandDisablePointerSelection(t *testing.T) {
	d, _, picked := b3iCommand(t, func(w *CommandW) { w.DisablePointerSelection(true) })
	r, _ := d.Find("Billing")
	d.Move(r.Center())
	d.Settle()
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if *picked != "cal" {
		t.Fatalf("hover moved the active item: picked %q", *picked)
	}
}

func TestCommandA11yDisabled(t *testing.T) {
	d, _, picked := b3iCommand(t, nil)
	d.EnableAccessibility()
	d.Settle()
	n, ok := d.A11yFind(a11y.RoleOption, "Calculator")
	if !ok || n.States&a11y.Disabled == 0 {
		t.Fatal("disabled option not announced disabled")
	}
	if _, ok := d.A11yFind(a11y.RoleCombobox, ""); !ok {
		t.Fatal("no combobox")
	}
	d.ClickLabel("Calculator")
	if *picked != "" {
		t.Fatalf("disabled item picked: %q", *picked)
	}
}

func TestMenuDarkThemeToken(t *testing.T) {
	sel := ""
	items := []MenuItem{{Key: "x", Label: "Item"}}
	for _, on := range []bool{true, false} {
		// Component overrides merge non-zero fields only, so the switch is
		// set on the resolved theme of the menu.
		th := theme.AntDesign()
		d := b3iDriver(func(c *Context) {
			m := Menu(c, items, &sel).Dark().Width(240)
			m.th.Components.Menu.DarkTheme = on
			m.Show()
		}, th)
		d.Frame()
		tk := d.r.c.Theme.Components.Menu
		dark := r2Fills(d, tk.DarkItemBg, -1) > 0
		if dark != on {
			t.Errorf("DarkTheme %v: dark sider background painted = %v", on, dark)
		}
	}
	if theme.Shadcn().Components.Menu.DarkTheme || !theme.AntDesign().Components.Menu.DarkTheme || !theme.ElementPlus().Components.Menu.DarkTheme {
		t.Error("DarkTheme: want false under shadcn, true under Ant Design and Element Plus")
	}
}
