package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// Tests of the navigation / feedback round: Tabs overflow in every style
// and placement, Dropdown groups, shadcn looks and ConfigProvider defaults.

func shad2cThemes() map[string]func() *theme.Theme {
	return map[string]func() *theme.Theme{
		"ant":     func() *theme.Theme { return theme.AntDesign() },
		"element": func() *theme.Theme { return theme.ElementPlus() },
		"shadcn":  func() *theme.Theme { return theme.Shadcn() },
	}
}

func shad2cTabItems(n int) []TabItem {
	var items []TabItem
	for i := 1; i <= n; i++ {
		items = append(items, TabItem{Key: fmt.Sprint(i), Label: fmt.Sprint("Tab ", i)})
	}
	return items
}

// settleFrames runs a few extra frames (reveal retries wake the loop).
func settleFrames(d *Driver, n int) {
	d.Settle()
	for i := 0; i < n; i++ {
		d.Frame()
	}
}

func TestShad2cTabsOverflowMore(t *testing.T) {
	for name, mk := range shad2cThemes() {
		for _, pl := range []Placement{Top, Left, Right} {
			t.Run(fmt.Sprint(name, "-", pl), func(t *testing.T) {
				act := "9"
				items := shad2cTabItems(14)
				d := NewDriver(func(c *Context) {
					Column(c, func() {
						tb := Tabs(c, &act, items).Placement(pl).Height(220).Show()
						tb.Panel(func() { Text(c, "panel "+act) })
					}, layout.W(420))
				}, DriverOptions{Width: 600, Height: 500, Theme: mk()})
				settleFrames(d, 4)
				more, ok := d.Find(ZhCN.Tabs.More)
				if !ok {
					t.Fatalf("no more button")
				}
				// The active tab was scrolled into the (shrunk) viewport.
				vr, ok := d.FindText("Tab 9")
				if !ok {
					t.Fatal("no active tab")
				}
				if pl == Top {
					if vr.Right() > more.X+0.5 {
						t.Fatalf("active tab %v under the more button %v", vr, more)
					}
				} else if vr.Bottom() > more.Y+0.5 || vr.Y < 0 {
					t.Fatalf("active tab %v not above the more button %v", vr, more)
				}
				// The more menu lists hidden tabs and selects one.
				d.Click(more.Center())
				d.Settle()
				if r, ok := d.Find("Tab 1"); ok {
					d.Click(r.Center())
					settleFrames(d, 2)
				} else if r, ok := d.Find("Tab 14"); ok {
					d.Click(r.Center())
					settleFrames(d, 2)
				} else {
					t.Fatal("hidden tabs missing from the more menu")
				}
				if act != "1" && act != "14" {
					t.Fatalf("active %q after choosing from the more menu", act)
				}
			})
		}
	}
}

func TestShad2cTabsShadcnCombinations(t *testing.T) {
	act := "1"
	closed, added := "", false
	d := NewDriver(func(c *Context) {
		tb := Tabs(c, &act, shad2cTabItems(3)).Closable().OnAdd(func() { added = true }).Show()
		if k := tb.Closed(); k != "" {
			closed = k
		}
		Tabs(c, &act, shad2cTabItems(3)).Card().Placement(Bottom).Show().Panel(func() {})
		Tabs(c, &act, shad2cTabItems(3)).BorderCard().Show().Panel(func() {})
	}, DriverOptions{Width: 700, Theme: theme.Shadcn()})
	d.EnableAccessibility()
	d.Settle()
	if r, ok := d.Find(ZhCN.RemoveItem("Tab 2")); !ok {
		t.Fatal("no close button")
	} else {
		d.Click(r.Center())
	}
	if closed != "2" {
		t.Fatalf("closed %q", closed)
	}
	if r, ok := d.Find(ZhCN.Tabs.Add); !ok {
		t.Fatal("no add button")
	} else {
		d.Click(r.Center())
	}
	if !added {
		t.Fatal("add not reported")
	}
	if n := mustNode(t, d, a11y.RoleTab, "Tab 1"); !n.States.Has(a11y.Selected) {
		t.Fatalf("tab %s", n)
	}
}

func TestShad2cTabsExtraLeftAndDefaults(t *testing.T) {
	act := "1"
	var tb *TabsW
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Defaults("Tabs", Props{"type": "border-card", "tabPosition": "left"}).Body(func() {
			tb = Tabs(c, &act, shad2cTabItems(3)).ExtraLeft(func() { Button(c, "L") }).Show()
		})
	}, DriverOptions{Width: 600, Theme: theme.ElementPlus()})
	d.Settle()
	if !tb.borderCard || tb.place != Left {
		t.Fatalf("defaults not applied: border %v place %v", tb.borderCard, tb.place)
	}
	l, _ := d.Find("L")
	t1, _ := d.FindText("Tab 1")
	if l.Bottom() > t1.Y+0.5 {
		t.Fatalf("left extra %v not before the tabs %v", l, t1)
	}
}

var shad2cGroups = []MenuItem{
	{Key: "g1", Label: "Account", Group: true, Children: []MenuItem{{Key: "profile", Label: "Profile"}, {Key: "billing", Label: "Billing"}}},
	{Key: "g2", Label: "Team", Group: true, Children: []MenuItem{{Key: "members", Label: "Members"}}},
	{Key: "logout", Label: "Log out"},
}

func TestShad2cDropdownGroups(t *testing.T) {
	for name, mk := range shad2cThemes() {
		t.Run(name, func(t *testing.T) {
			got := ""
			d := NewDriver(func(c *Context) {
				dd := Dropdown(c, shad2cGroups).Trigger(TriggerClick).Body(func() { Button(c, "Open") })
				if k := dd.Clicked(); k != "" {
					got = k
				}
			}, DriverOptions{Width: 600, Height: 500, Theme: mk()})
			d.EnableAccessibility()
			d.Settle()
			d.ClickLabel("Open")
			// Titles are shown, not items; the group carries the name.
			if _, ok := d.FindText("Account"); !ok {
				t.Fatal("group title not shown")
			}
			if _, ok := d.Find("Account"); ok {
				t.Fatal("group title is interactive (drawn as a submenu item)")
			}
			g := mustNode(t, d, a11y.RoleGroup, "Account")
			if len(g.Children) == 0 {
				t.Fatalf("group without items: %s", g)
			}
			mustNode(t, d, a11y.RoleGroup, "Team")
			if n := mustNode(t, d, a11y.RoleMenuItem, "Profile"); n.States.Has(a11y.Expandable) {
				t.Fatalf("item %s", n)
			}
			// Keyboard: Down from the trigger lands on the first item, past
			// the title; moving on crosses into the next group.
			d.ClickLabel("Open") // close
			d.Key(input.KeyTab, 0)
			if f := d.A11yFocused(); f == nil || f.Name != "Open" {
				d.Key(input.KeyTab, 0)
			}
			d.Key(input.KeyDown, 0)
			if f := d.A11yFocused(); f == nil || f.Name != "Profile" {
				t.Fatalf("focus %v after opening, want Profile", f)
			}
			d.Key(input.KeyDown, 0)
			d.Key(input.KeyDown, 0)
			if f := d.A11yFocused(); f == nil || f.Name != "Members" {
				t.Fatalf("focus %v, want Members", f)
			}
			d.Key(input.KeyUp, 0)
			d.Key(input.KeyUp, 0)
			d.Key(input.KeyUp, 0) // wraps past the title to the last item
			if f := d.A11yFocused(); f == nil || f.Name != "Log out" {
				t.Fatalf("focus %v, want Log out", f)
			}
			d.Key(input.KeyEnter, 0)
			if got != "logout" {
				t.Fatalf("chose %q", got)
			}
		})
	}
}

func TestShad2cMenuPopupGroups(t *testing.T) {
	sel := ""
	items := []MenuItem{{Key: "u", Label: "Users", Children: shad2cGroups[:2]}}
	d := NewDriver(func(c *Context) {
		Column(c, func() { Menu(c, items, &sel).Mode(MenuVertical).Show() }, layout.W(200))
	}, DriverOptions{Width: 600, Height: 400, Theme: theme.Shadcn()})
	d.EnableAccessibility()
	d.Settle()
	r, _ := d.Find("Users")
	d.Move(r.Center())
	d.Advance(300 * time.Millisecond)
	d.Settle()
	if _, ok := d.FindText("Account"); !ok {
		t.Fatal("submenu group title missing")
	}
	mustNode(t, d, a11y.RoleGroup, "Team")
	if _, ok := d.Find("Members"); !ok {
		t.Fatal("group items missing")
	}
}

func TestShad2cDefaults(t *testing.T) {
	var m *ModalW
	var dd *DropdownW
	var st *StepsW
	var pg *PaginationW
	var dr *DrawerW
	var sp *SpinW
	open := false
	page := 1
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).
			Defaults("Modal", Props{"width": 640, "centered": true, "okText": "Go"}).
			Defaults("Dropdown", Props{"trigger": "click", "placement": "topRight", "arrow": true}).
			Defaults("Steps", Props{"direction": "vertical", "size": "small"}).
			Defaults("Pagination", Props{"disabled": true, "size": "small"}).
			Defaults("Drawer", Props{"placement": "left", "width": 500}).
			Defaults("Spin", Props{"size": "large", "tip": "Wait"}).
			Body(func() {
				m = Modal(c, &open, "M")
				dd = Dropdown(c, nil)
				st = Steps(c, nil)
				pg = Pagination(c, &page, 50, 10)
				dr = Drawer(c, &open, "D")
				sp = Spin(c)
				// Explicit modifiers win.
				_ = Modal(c, &open, "N").Width(300)
			})
	}, DriverOptions{Width: 600})
	d.Settle()
	if m.width != 640 || !m.centered || m.okText != "Go" {
		t.Fatalf("modal %v %v %q", m.width, m.centered, m.okText)
	}
	if dd.trigger != TriggerClick || dd.place != TopEnd || !dd.arrow {
		t.Fatalf("dropdown %v %v %v", dd.trigger, dd.place, dd.arrow)
	}
	if !st.vertical || st.size != Small {
		t.Fatalf("steps %v %v", st.vertical, st.size)
	}
	if !pg.disabled || !pg.small {
		t.Fatalf("pagination %v %v", pg.disabled, pg.small)
	}
	// Ant placement names are logical (spec Q2): "left" is the start
	// edge, the left one in LTR.
	if dr.place != DrawerStart || dr.place.resolve(false) != DrawerLeft || dr.size != 500 {
		t.Fatalf("drawer %v %v", dr.place, dr.size)
	}
	if sp.size != Large || sp.tip != "Wait" {
		t.Fatalf("spin %v %q", sp.size, sp.tip)
	}
}

func TestShad2cPaginationSimpleReadOnly(t *testing.T) {
	page := 2
	d := NewDriver(func(c *Context) {
		Pagination(c, &page, 50, 10).Simple().Size(Small).ReadOnly().Show()
	}, DriverOptions{Width: 300})
	d.Settle()
	if _, ok := d.FindText("2 / 5"); !ok {
		t.Fatal("read-only simple pager text missing")
	}
	d.ClickLabel(ZhCN.NextPage)
	if page != 3 {
		t.Fatalf("page %d", page)
	}
	prev, _ := d.Find(ZhCN.PrevPage)
	next, _ := d.Find(ZhCN.NextPage)
	if w := next.Right() - prev.X; w > 130 {
		t.Fatalf("narrow pager is %v wide", w)
	}
}
