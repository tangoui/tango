package mobile

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

func mustNode(t *testing.T, d *ui.Driver, role a11y.Role, name string) *a11y.Node {
	t.Helper()
	n, ok := d.A11yFind(role, name)
	if !ok {
		t.Fatalf("no %s %q in tree:\n%s", role, name, d.AccessibilityTree().Dump())
	}
	return n
}

func perform(t *testing.T, d *ui.Driver, role a11y.Role, name string, act a11y.Action, arg string) {
	t.Helper()
	if err := d.A11yPerform(role, name, act, arg); err != nil {
		t.Fatalf("%v\n%s", err, d.AccessibilityTree().Dump())
	}
}

// tabTo presses Tab until the focused node has role and name.
func tabTo(t *testing.T, d *ui.Driver, role a11y.Role, name string) {
	t.Helper()
	for i := 0; i < 40; i++ {
		d.Key(input.KeyTab, 0)
		d.Settle()
		if f := d.A11yFocused(); f != nil && f.Role == role && f.Name == name {
			return
		}
	}
	t.Fatalf("Tab never reached %s %q:\n%s", role, name, d.AccessibilityTree().Dump())
}

func enter(d *ui.Driver) {
	d.Key(input.KeyEnter, 0)
	d.Settle()
}

func TestA11yTouchableKeyboardAndPress(t *testing.T) {
	top, inner := 0, 0
	d := phone(func(c *ui.Context) {
		ui.Column(c, func() {
			if Button(c, "Top").Clicked() {
				top++
			}
			ScrollView(c).Layout(layout.H(300)).Body(func() {
				if Button(c, "Inner").Clicked() {
					inner++
				}
			})
		})
	}, false)
	d.EnableAccessibility()
	// Outside gesture areas a touchable is a Tab stop; Enter taps it.
	tabTo(t, d, a11y.RoleButton, "Top")
	enter(d)
	if top != 1 {
		t.Fatalf("Enter: %d", top)
	}
	// Inside a scroll view it has no hit but still offers Press.
	n := mustNode(t, d, a11y.RoleButton, "Inner")
	if !n.Actions.Has(a11y.Press) || n.States.Has(a11y.Focusable) {
		t.Fatalf("inner button: %v %v", n, n.Actions)
	}
	perform(t, d, a11y.RoleButton, "Inner", a11y.Press, "")
	perform(t, d, a11y.RoleButton, "Top", a11y.Press, "")
	if inner != 1 || top != 2 {
		t.Fatalf("press: inner %d top %d", inner, top)
	}
}

func TestA11yNavBarTabBar(t *testing.T) {
	back, right := 0, 0
	tab := "home"
	d := phone(func(c *ui.Context) {
		ui.Column(c, func() {
			nb := NavBar(c, "Title").Back("").RightIcon(icon.More).RightLabel("More")
			if nb.BackClicked() {
				back++
			}
			if nb.RightClicked() {
				right++
			}
			ui.Spacer(c)
			TabBar(c, []TabItem{{Key: "home", Title: "Home"}, {Key: "me", Title: "Me", Badge: "5"}}, &tab)
		}, layout.Grow(1))
	}, false)
	d.EnableAccessibility()
	d.Settle() // the mirror follows the bar's geometry of the previous frame
	loc := ui.ZhCN
	mustNode(t, d, a11y.RoleHeading, "Title")
	b := mustNode(t, d, a11y.RoleButton, loc.Back)
	if b.Bounds.W < 30 || b.Bounds.H < 30 {
		t.Fatalf("back bounds: %+v", b.Bounds)
	}
	tabTo(t, d, a11y.RoleButton, loc.Back)
	enter(d)
	perform(t, d, a11y.RoleButton, "More", a11y.Press, "")
	if back != 1 || right != 1 {
		t.Fatalf("back %d right %d", back, right)
	}
	// The pointer still works through the painted bar.
	d.Click(geom.Pt(b.Bounds.X+b.Bounds.W/2, b.Bounds.Y+b.Bounds.H/2))
	if back != 2 {
		t.Fatalf("pointer back: %d", back)
	}

	list := mustNode(t, d, a11y.RoleTabList, "")
	home := mustNode(t, d, a11y.RoleTab, "Home")
	me := mustNode(t, d, a11y.RoleTab, "Me")
	if !home.States.Has(a11y.Selected) || me.States.Has(a11y.Selected) || me.Description != "5" || home.Parent != list.ID {
		t.Fatalf("tabs: %v %v", home, me)
	}
	perform(t, d, a11y.RoleTab, "Me", a11y.Press, "")
	if tab != "me" {
		t.Fatalf("press tab: %q", tab)
	}
	// Arrow keys move between tabs and select.
	tabTo(t, d, a11y.RoleTab, "Me")
	d.Key(input.KeyRight, 0)
	d.Settle()
	if tab != "home" || d.A11yFocused().Name != "Home" {
		t.Fatalf("arrow: %q %v", tab, d.A11yFocused())
	}
}

func TestA11yListCellsAndSwipe(t *testing.T) {
	wifi := false
	clicked := 0
	key := ""
	d := phone(func(c *ui.Context) {
		ScrollView(c).Layout(layout.H(800)).Body(func() {
			List(c, "Net").Body(func() {
				Cell(c, "Wi-Fi").Switch(&wifi)
				if Cell(c, "Profile").Desc("photo").Extra("Ann").Arrow().Clicked() {
					clicked++
				}
				Cell(c, "Plain")
				sw := SwipeCell(c).Right(SwipeAction{Key: "del", Text: "Delete", Color: "danger"})
				sw.Body(func() { Cell(c, "Row") })
				if k := sw.ActionKey(); k != "" {
					key = k
				}
			})
		})
	}, false)
	d.EnableAccessibility()
	mustNode(t, d, a11y.RoleHeading, "Net")
	mustNode(t, d, a11y.RoleList, "Net")
	sw := mustNode(t, d, a11y.RoleSwitch, "Wi-Fi")
	if sw.States.Has(a11y.Checked) || !sw.Actions.Has(a11y.Press) {
		t.Fatalf("switch: %v", sw)
	}
	perform(t, d, a11y.RoleSwitch, "Wi-Fi", a11y.Press, "")
	if !wifi || !mustNode(t, d, a11y.RoleSwitch, "Wi-Fi").States.Has(a11y.Checked) {
		t.Fatal("press did not toggle the switch")
	}
	p := mustNode(t, d, a11y.RoleButton, "Profile")
	if p.Description != "photo" || p.Value != "Ann" {
		t.Fatalf("cell: %v", p)
	}
	perform(t, d, a11y.RoleButton, "Profile", a11y.Press, "")
	if clicked != 1 {
		t.Fatalf("cell press: %d", clicked)
	}
	if n := mustNode(t, d, a11y.RoleListItem, "Plain"); n.Actions.Has(a11y.Press) {
		t.Fatalf("plain row is not activatable: %v", n.Actions)
	}
	// Swipe actions are buttons a screen reader presses directly.
	perform(t, d, a11y.RoleButton, "Delete", a11y.Press, "")
	if key != "del" {
		t.Fatalf("swipe action: %q", key)
	}
}

func TestA11yDialogPopupActionSheet(t *testing.T) {
	open := true
	ok := false
	d := phone(func(c *ui.Context) {
		if Dialog(c, &open, "Delete?").Content("Gone for good").Confirm("Yes", "No").Confirmed() {
			ok = true
		}
	}, false)
	d.EnableAccessibility()
	dlg := mustNode(t, d, a11y.RoleAlertDialog, "Delete?")
	if !dlg.States.Has(a11y.Modal) || !dlg.Actions.Has(a11y.Dismiss) || dlg.Description != "Gone for good" {
		t.Fatalf("dialog: %v %v", dlg, dlg.Actions)
	}
	// Focus moves into the modal dialog.
	if f := d.A11yFocused(); f == nil || f.Role != a11y.RoleButton {
		t.Fatalf("focus: %v", f)
	}
	tabTo(t, d, a11y.RoleButton, "Yes")
	enter(d)
	if !ok || open {
		t.Fatalf("confirm: %v %v", ok, open)
	}
	open = true
	d.Settle()
	perform(t, d, a11y.RoleAlertDialog, "Delete?", a11y.Dismiss, "")
	if open {
		t.Fatal("Dismiss did not close the dialog")
	}

	pop := true
	d2 := phone(func(c *ui.Context) {
		Popup(c, &pop).A11yLabel("Sheet").Body(func() { ui.Div(c, layout.H(200)) })
	}, false)
	d2.EnableAccessibility()
	if n := mustNode(t, d2, a11y.RoleDialog, "Sheet"); !n.States.Has(a11y.Modal) {
		t.Fatalf("popup: %v", n)
	}
	if rep := d2.AuditNoContrast(); len(rep.Issues) != 0 {
		t.Fatalf("popup audit: %v", rep)
	}

	as := true
	sel := -1
	d3 := phone(func(c *ui.Context) {
		if i, picked := ActionSheet(c, &as, []Action{{Text: "Copy", Description: "to clipboard"}, {Text: "Off", Disabled: true}}).Extra("Choose").Cancel("").Selected(); picked {
			sel = i
		}
	}, false)
	d3.EnableAccessibility()
	mustNode(t, d3, a11y.RoleDialog, "Choose")
	if n := mustNode(t, d3, a11y.RoleButton, "Copy"); n.Description != "to clipboard" {
		t.Fatalf("action: %v", n)
	}
	if n := mustNode(t, d3, a11y.RoleButton, "Off"); !n.States.Has(a11y.Disabled) || n.Actions.Has(a11y.Press) {
		t.Fatalf("disabled action: %v", n)
	}
	mustNode(t, d3, a11y.RoleButton, ui.ZhCN.Cancel)
	perform(t, d3, a11y.RoleButton, "Copy", a11y.Press, "")
	if sel != 0 || as {
		t.Fatalf("select: %d %v", sel, as)
	}
}

func TestA11yToast(t *testing.T) {
	ok, fail := true, true
	d := phone(func(c *ui.Context) {
		Toast(c, &ok, "Saved").Duration(0).Show()
		Toast(c, &fail, "Failed").Icon("fail").Duration(0).Show()
	}, false)
	d.EnableAccessibility()
	if n := mustNode(t, d, a11y.RoleStatus, "Saved"); n.Live != a11y.LivePolite {
		t.Fatalf("toast: %v", n)
	}
	if n := mustNode(t, d, a11y.RoleAlert, "Failed"); n.Live != a11y.LiveAssertive {
		t.Fatalf("fail toast: %v", n)
	}
}

func TestA11yPickerWheels(t *testing.T) {
	val := []string{"b"}
	when := time.Date(2026, 3, 10, 9, 30, 0, 0, time.UTC)
	d := phone(func(c *ui.Context) {
		PickerView(c, [][]ui.Option{ui.Opts("a", "b", "c", "d")}, &val).A11yLabel("Letter").Show()
		DatePickerView(c, &when, PrecisionDay).Show()
	}, false)
	d.EnableAccessibility()
	w := mustNode(t, d, a11y.RoleSlider, "Letter")
	if w.Value != "b" || w.ValueNow != 1 || w.ValueMax != 3 || !w.Actions.Has(a11y.Increment|a11y.Decrement|a11y.SetValue) {
		t.Fatalf("wheel: %v %v", w, w.Actions)
	}
	perform(t, d, a11y.RoleSlider, "Letter", a11y.Increment, "")
	if val[0] != "c" {
		t.Fatalf("increment: %v", val)
	}
	perform(t, d, a11y.RoleSlider, "Letter", a11y.Decrement, "")
	perform(t, d, a11y.RoleSlider, "Letter", a11y.Decrement, "")
	if val[0] != "a" {
		t.Fatalf("decrement: %v", val)
	}
	perform(t, d, a11y.RoleSlider, "Letter", a11y.SetValue, "d")
	if val[0] != "d" {
		t.Fatalf("set value: %v", val)
	}
	// Keyboard: the wheel is a Tab stop, Up / Down step.
	tabTo(t, d, a11y.RoleSlider, "Letter")
	d.Key(input.KeyUp, 0)
	d.Settle()
	if val[0] != "c" {
		t.Fatalf("key up: %v", val)
	}
	// Date columns are named by their unit.
	loc := ui.ZhCN
	if n := mustNode(t, d, a11y.RoleSlider, loc.Calendar.Month); n.Value != loc.Month(3) {
		t.Fatalf("month: %v", n)
	}
	perform(t, d, a11y.RoleSlider, loc.A11y.Day, a11y.Increment, "")
	if when.Day() != 11 {
		t.Fatalf("day: %v", when)
	}
}

func TestA11yPickerPopup(t *testing.T) {
	open := true
	val := []string{"x"}
	confirmed := false
	d := phone(func(c *ui.Context) {
		if Picker(c, &open, "Pick").Columns([][]ui.Option{ui.Opts("x", "y")}, &val).Confirmed() {
			confirmed = true
		}
	}, false)
	d.EnableAccessibility()
	mustNode(t, d, a11y.RoleDialog, "Pick")
	mustNode(t, d, a11y.RoleHeading, "Pick")
	perform(t, d, a11y.RoleSlider, "Pick", a11y.Increment, "")
	perform(t, d, a11y.RoleButton, ui.ZhCN.Confirm, a11y.Press, "")
	if !confirmed || val[0] != "y" || open {
		t.Fatalf("picker: %v %v %v", confirmed, val, open)
	}
}

func TestA11yFormAndStepper(t *testing.T) {
	name, q := "", ""
	qty := 2.0
	d := phone(func(c *ui.Context) {
		ui.Column(c, func() {
			SearchBar(c, &q, "Find", "")
			Form(c).Header("Profile").Body(func() {
				FormItem(c, "Name").Required().Error("Too short").Body(func() { Field(c, &name) })
				FormItem(c, "Qty").Body(func() { Stepper(c, &qty).Min(1).Max(3) })
				FormItem(c, "Birthday").Arrow().Body(func() { ui.Text(c, "1990") })
			})
		})
	}, false)
	d.EnableAccessibility()
	mustNode(t, d, a11y.RoleSearchbox, "Find")
	mustNode(t, d, a11y.RoleForm, "Profile")
	tb := mustNode(t, d, a11y.RoleTextbox, "Name")
	if !tb.States.Has(a11y.Required | a11y.Invalid) {
		t.Fatalf("field: %v", tb)
	}
	if g := mustNode(t, d, a11y.RoleGroup, "Name"); g.Description != "Too short" {
		t.Fatalf("item: %v", g)
	}
	if b := mustNode(t, d, a11y.RoleButton, "Birthday 1990"); !b.States.Has(a11y.HasPopup) {
		t.Fatalf("arrow item: %v", b)
	}
	sp := mustNode(t, d, a11y.RoleSpinButton, "Qty")
	if sp.Value != "2" || sp.ValueMin != 1 || sp.ValueMax != 3 {
		t.Fatalf("stepper: %v", sp)
	}
	perform(t, d, a11y.RoleSpinButton, "Qty", a11y.Increment, "")
	if qty != 3 {
		t.Fatalf("increment: %v", qty)
	}
	perform(t, d, a11y.RoleSpinButton, "Qty", a11y.Increment, "") // clamped
	perform(t, d, a11y.RoleButton, ui.ZhCN.A11y.Decrease, a11y.Press, "")
	if qty != 2 {
		t.Fatalf("decrease button: %v", qty)
	}
	perform(t, d, a11y.RoleSpinButton, "Qty", a11y.SetValue, "1")
	if qty != 1 {
		t.Fatalf("set value: %v", qty)
	}
	tabTo(t, d, a11y.RoleSpinButton, "Qty")
	d.Key(input.KeyUp, 0)
	d.Settle()
	d.Key(input.KeyEnd, 0)
	d.Settle()
	if qty != 3 {
		t.Fatalf("keys: %v", qty)
	}
}

func TestA11yTabsSwiper(t *testing.T) {
	active, cur := 0, 0
	d := phone(func(c *ui.Context) {
		ui.Column(c, func() {
			Tabs(c, []string{"One", "Two", "Three"}, &active).Body(func(i int) { ui.Div(c, layout.H(100)) })
			Swiper(c, &cur).Autoplay(time.Second).Slides(3, func(i int) { ui.Div(c, layout.Grow(1)) })
		})
	}, false)
	d.EnableAccessibility()
	list := mustNode(t, d, a11y.RoleTabList, "")
	one := mustNode(t, d, a11y.RoleTab, "One")
	if one.Parent != list.ID || !one.States.Has(a11y.Selected) {
		t.Fatalf("tabs: %v %v", list, one)
	}
	mustNode(t, d, a11y.RoleTabPanel, "One")
	perform(t, d, a11y.RoleTab, "Three", a11y.Press, "")
	if active != 2 {
		t.Fatalf("press tab: %d", active)
	}
	if n := mustNode(t, d, a11y.RoleTabList, ""); !n.States.Has(a11y.Focusable) {
		t.Fatalf("tab list focus: %v", n)
	}
	d.Runner().Context().SetFocus(ui.ID(list.ID))
	d.Key(input.KeyHome, 0)
	d.Settle()
	if active != 0 {
		t.Fatalf("home: %d", active)
	}
	if l := mustNode(t, d, a11y.RoleTabList, ""); l.ActiveDescendant != a11y.NodeID(mustNode(t, d, a11y.RoleTab, "One").ID) {
		t.Fatalf("active descendant: %v", l.ActiveDescendant)
	}

	loc := ui.ZhCN
	mustNode(t, d, a11y.RoleGroup, loc.Slide(1, 3))
	slides := mustNode(t, d, a11y.RoleGroup, loc.Slide(1, 3)).Parent
	if err := d.PerformAction(slides, a11y.Increment, ""); err != nil {
		t.Fatal(err)
	}
	if cur != 1 {
		t.Fatalf("swiper next: %d", cur)
	}
	mustNode(t, d, a11y.RoleGroup, loc.Slide(2, 3))
}

func TestA11ySwiperAutoplayReduceMotion(t *testing.T) {
	cur := 0
	page := func(c *ui.Context) {
		Swiper(c, &cur).Autoplay(500*time.Millisecond).Slides(3, func(i int) { ui.Div(c, layout.Grow(1)) })
	}
	d := ui.NewDriver(page, ui.DriverOptions{Width: 390, Height: 844, Animations: true, ReduceMotion: true})
	d.Frame()
	run(d, 2*time.Second)
	if cur != 0 {
		t.Fatalf("autoplay ran under reduced motion: %d", cur)
	}
	d2 := ui.NewDriver(page, ui.DriverOptions{Width: 390, Height: 844, Animations: true})
	d2.Frame()
	run(d2, 2*time.Second)
	if cur == 0 {
		t.Fatal("autoplay did not run")
	}
}

func TestA11yDisplayWidgets(t *testing.T) {
	cell := -1
	closed := false
	d := phone(func(c *ui.Context) {
		ui.Column(c, func() {
			if i, ok := Grid(c, 2, []GridItem{{Text: "Scan", Badge: "3"}, {Text: "Pay"}}).Clicked(); ok {
				cell = i
			}
			if NoticeBar(c, "Maintenance tonight").Closeable().Closed() {
				closed = true
			}
			Badge(c, "5", func() { ui.Div(c, layout.Size(40, 40)) })
			Tag(c, "New")
			Skeleton(c)
			Empty(c)
			Result(c, "success", "Done").Description("All set")
			Button(c, "Busy").Loading(true)
			Button(c, "Off").Disabled(true)
		})
	}, false)
	d.EnableAccessibility()
	if n := mustNode(t, d, a11y.RoleButton, "Scan"); n.Description != "3" {
		t.Fatalf("grid: %v", n)
	}
	tabTo(t, d, a11y.RoleButton, "Pay")
	enter(d)
	if cell != 1 {
		t.Fatalf("grid enter: %d", cell)
	}
	mustNode(t, d, a11y.RoleStatus, "Maintenance tonight")
	mustNode(t, d, a11y.RoleText, "New")
	if n := mustNode(t, d, a11y.RoleStatus, ui.ZhCN.A11y.Loading); !n.States.Has(a11y.Busy) {
		t.Fatalf("skeleton: %v", n)
	}
	mustNode(t, d, a11y.RoleText, ui.ZhCN.NoData)
	if n := mustNode(t, d, a11y.RoleStatus, "Done"); n.Description != "All set" {
		t.Fatalf("result: %v", n)
	}
	if n := mustNode(t, d, a11y.RoleButton, "Busy"); !n.States.Has(a11y.Busy) || n.Actions.Has(a11y.Press) {
		t.Fatalf("loading button: %v", n)
	}
	if n := mustNode(t, d, a11y.RoleButton, "Off"); !n.States.Has(a11y.Disabled) {
		t.Fatalf("disabled button: %v", n)
	}
	if len(d.A11yFindAll(a11y.RoleGroup, "")) == 0 {
		t.Fatal("badge group missing")
	}
	perform(t, d, a11y.RoleButton, ui.ZhCN.Close, a11y.Press, "")
	if !closed {
		t.Fatal("notice bar close")
	}
	if _, ok := d.A11yFind(a11y.RoleStatus, "Maintenance tonight"); ok {
		t.Fatal("closed notice bar still exposed")
	}
}

func TestA11yIndexBarBubble(t *testing.T) {
	tapped := 0
	d := phone(func(c *ui.Context) {
		ui.Column(c, func() {
			IndexBar(c, []string{"A", "B", "C"}).Layout(layout.Grow(1)).Body(func(i int) {
				for k := 0; k < 20; k++ {
					ui.Key(c, k, func() { Cell(c, "row") })
				}
			})
		}, layout.Grow(1))
		if FloatingBubble(c, icon.Message, AxisBoth, true).A11yLabel("Chat").Clicked() {
			tapped++
		}
	}, false)
	d.EnableAccessibility()
	d.Settle() // the scroll range is known from the second frame
	mustNode(t, d, a11y.RoleHeading, "B")
	a := mustNode(t, d, a11y.RoleTab, "A")
	if !a.States.Has(a11y.Selected) {
		t.Fatalf("index: %v", a)
	}
	perform(t, d, a11y.RoleTab, "C", a11y.Press, "")
	if n := mustNode(t, d, a11y.RoleTab, "C"); !n.States.Has(a11y.Selected) {
		t.Fatalf("jump: %v", n)
	}
	perform(t, d, a11y.RoleButton, "Chat", a11y.Press, "")
	tabTo(t, d, a11y.RoleButton, "Chat")
	enter(d)
	if tapped != 2 {
		t.Fatalf("bubble: %d", tapped)
	}
}

func TestA11yPullAndInfiniteStatus(t *testing.T) {
	refreshing, loading, failed := true, false, true
	d := phone(func(c *ui.Context) {
		ui.Column(c, func() {
			PullToRefresh(c, &refreshing).Layout(layout.H(300)).Body(func() { Cell(c, "a") })
			InfiniteList(c, &loading).Failed(&failed).Layout(layout.H(300)).Body(func() { Cell(c, "b") })
		})
	}, false)
	d.EnableAccessibility()
	if n := mustNode(t, d, a11y.RoleStatus, ui.ZhCN.Loading); !n.States.Has(a11y.Busy) || n.Live != a11y.LivePolite {
		t.Fatalf("refresh status: %v", n)
	}
	var retry *a11y.Node
	d.AccessibilityTree().Walk(func(n *a11y.Node, _ int) bool {
		if n.Role == a11y.RoleButton && n.Name != "" {
			retry = n
		}
		return true
	})
	if retry == nil {
		t.Fatalf("no retry button:\n%s", d.AccessibilityTree().Dump())
	}
	if err := d.PerformAction(retry.ID, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	if failed || !loading {
		t.Fatalf("retry: failed %v loading %v", failed, loading)
	}
}

// The tab header keeps its size whether or not its accessibility children
// are built.
func TestA11yTabsHeaderSize(t *testing.T) {
	active := 0
	var id ui.ID
	page := func(c *ui.Context) {
		Tabs(c, []string{"One", "Two"}, &active).Header()
		id = c.Focused()
	}
	off := phone(page, false)
	off.Settle()
	on := phone(page, false)
	on.EnableAccessibility()
	tl := mustNode(t, on, a11y.RoleTabList, "")
	r1, _ := on.Runner().Context().RectOf(ui.ID(tl.ID))
	r0, _ := off.Runner().Context().RectOf(ui.ID(tl.ID))
	if r0 != r1 || r1.W != 390 {
		t.Fatalf("header size: off %v on %v", r0, r1)
	}
	_ = id
}

func TestA11yMobileAudit(t *testing.T) {
	q := ""
	tab, active := "home", 0
	v := 2.0
	d := phone(func(c *ui.Context) {
		ui.Column(c, func() {
			NavBar(c, "Title").Back("").RightIcon(icon.More).RightLabel("More")
			SearchBar(c, &q, "", "Cancel")
			Tabs(c, []string{"A", "B"}, &active).Header()
			NoticeBar(c, "notice").Closeable()
			Grid(c, 2, []GridItem{{Text: "g1"}, {Text: "g2"}})
			Stepper(c, &v).A11yLabel("Count")
			Cell(c, "row").Arrow()
			Button(c, "OK")
			ui.Spacer(c)
			TabBar(c, []TabItem{{Key: "home", Title: "Home"}, {Key: "me", Title: "Me"}}, &tab)
		}, layout.Grow(1))
	}, false)
	if rep := d.AuditNoContrast(); len(rep.Issues) != 0 {
		t.Fatalf("audit:\n%v\n%s", rep, d.AccessibilityTree().Dump())
	}
}

// Tabbing to a field below the fold scrolls the touch scroll view.
func TestA11yScrollViewFollowsFocus(t *testing.T) {
	var sv *ScrollViewW
	vals := make([]string, 12)
	d := phone(func(c *ui.Context) {
		sv = ScrollView(c).Layout(layout.H(400)).Body(func() {
			Form(c).Body(func() {
				for i := range vals {
					ui.Key(c, i, func() {
						FormItem(c, "F"+string(rune('A'+i))).Body(func() { Field(c, &vals[i]) })
					})
				}
			})
		})
	}, false)
	d.EnableAccessibility()
	d.Settle()
	tabTo(t, d, a11y.RoleTextbox, "FL")
	d.Settle()
	if sv.ScrollOffset() <= 0 {
		t.Fatalf("not scrolled: %v", sv.ScrollOffset())
	}
	n := mustNode(t, d, a11y.RoleTextbox, "FL")
	if n.Bounds.Y+n.Bounds.H > 400.5 || n.States.Has(a11y.Offscreen) {
		t.Fatalf("focused field not visible: %+v", n.Bounds)
	}
}

// Rows of a touch scroll view hidden behind the end of the view (under the
// tab bar) are Offscreen and offer ScrollIntoView; performing it scrolls
// the row fully into the view, and back.
func TestA11yScrollViewShowsRow(t *testing.T) {
	var sv *ScrollViewW
	var view geom.Rect
	tab := "home"
	d := phone(func(c *ui.Context) {
		ui.Column(c, func() {
			SafeArea(c, Top)
			NavBar(c, "Title")
			sv = ScrollView(c).Layout(layout.Grow(1)).Body(func() {
				List(c, "Rows").Body(func() {
					for i := 1; i <= 20; i++ {
						ui.Key(c, i, func() {
							Cell(c, "Row "+string(rune('A'+i-1))).Desc("desc").Arrow()
						})
					}
				})
			})
			view, _ = c.RectOf(sv.ID())
			TabBar(c, []TabItem{{Key: "home", Title: "Home"}, {Key: "me", Title: "Me"}}, &tab)
		}, layout.Grow(1))
	}, false)
	d.SetSafeArea(geom.Insets{Top: 52, Bottom: 48})
	d.EnableAccessibility()
	d.Settle()
	d.Frame()
	bar := mustNode(t, d, a11y.RoleTabList, "")
	if view.Bottom() > bar.Bounds.Y+0.5 {
		t.Fatalf("scroll view %v extends under the tab bar %+v", view, bar.Bounds)
	}
	visible := func(n *a11y.Node) bool {
		return n.Bounds.Y >= view.Y-0.5 && n.Bounds.Y+n.Bounds.H <= view.Bottom()+0.5
	}
	for i := 1; i <= 20; i++ {
		n := mustNode(t, d, a11y.RoleButton, "Row "+string(rune('A'+i-1)))
		if !n.Actions.Has(a11y.ScrollIntoView) {
			t.Fatalf("%s lacks ScrollIntoView: %s", n, n.Actions)
		}
		if out := n.Bounds.Y >= view.Bottom(); n.States.Has(a11y.Offscreen) != out {
			t.Errorf("%s at %+v (view %v): offscreen %v", n.Name, n.Bounds, view, !out)
		}
	}
	last := mustNode(t, d, a11y.RoleButton, "Row T")
	if !last.States.Has(a11y.Offscreen) {
		t.Fatalf("last row visible: %+v", last.Bounds)
	}
	perform(t, d, a11y.RoleButton, "Row P", a11y.ScrollIntoView, "")
	d.Frame()
	if n := mustNode(t, d, a11y.RoleButton, "Row P"); !visible(n) || n.States.Has(a11y.Offscreen) {
		t.Fatalf("Row P not fully shown at %+v (view %v, offset %v)", n.Bounds, view, sv.ScrollOffset())
	} else if n.Bounds.Y+n.Bounds.H < view.Bottom()-0.5 {
		t.Errorf("scrolled more than needed: Row P at %+v (view %v)", n.Bounds, view)
	}
	if n := mustNode(t, d, a11y.RoleButton, "Row A"); !n.States.Has(a11y.Offscreen) {
		t.Errorf("Row A still on screen at %+v", n.Bounds)
	}
	perform(t, d, a11y.RoleButton, "Row A", a11y.ScrollIntoView, "")
	d.Frame()
	if n := mustNode(t, d, a11y.RoleButton, "Row A"); !visible(n) {
		t.Fatalf("Row A not shown at %+v (view %v)", n.Bounds, view)
	}
}
