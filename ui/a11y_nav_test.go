package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// Accessibility of the navigation, layout and feedback-layer components.

func TestA11yNavBreadcrumbPagination(t *testing.T) {
	loc := ZhCN
	page, ps := 1, 10
	d := NewDriver(func(c *Context) {
		Breadcrumb(c, "Home", "Apps", "List")
		Pagination(c, &page, 50, 10).SizeChanger(&ps).Show()
	}, DriverOptions{Width: 900})
	d.EnableAccessibility()
	d.Settle()
	mustNode(t, d, a11y.RoleNavigation, loc.A11y.Breadcrumb)
	if l := mustNode(t, d, a11y.RoleLink, "Home"); !l.States.Has(a11y.Focusable) {
		t.Fatalf("crumb %s", l)
	}
	if cur := mustNode(t, d, a11y.RoleText, "List"); !cur.States.Has(a11y.Current) {
		t.Fatalf("current crumb %s", cur)
	}
	mustNode(t, d, a11y.RoleNavigation, loc.A11y.Pagination)
	if p1 := mustNode(t, d, a11y.RoleButton, loc.Page(1)); !p1.States.Has(a11y.Current) {
		t.Fatalf("current page %s", p1)
	}
	if prev := mustNode(t, d, a11y.RoleButton, loc.PrevPage); !prev.States.Has(a11y.Disabled) {
		t.Fatalf("prev on page 1: %s", prev)
	}
	mustNode(t, d, a11y.RoleCombobox, loc.A11y.PageSize)
	p3 := mustNode(t, d, a11y.RoleButton, loc.Page(3))
	if err := d.PerformAction(p3.ID, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if page != 3 || !mustNode(t, d, a11y.RoleButton, loc.Page(3)).States.Has(a11y.Current) {
		t.Fatalf("page %d after pressing page 3", page)
	}
}

func navTestItems() []MenuItem {
	return []MenuItem{
		{Key: "a", Label: "Alpha"},
		{Key: "b", Label: "Beta", Children: []MenuItem{{Key: "b1", Label: "Beta 1"}, {Key: "b2", Label: "Beta 2"}}},
		{Divider: true},
		{Key: "c", Label: "Gamma"},
	}
}

func TestA11yNavMenuInline(t *testing.T) {
	sel := "a"
	d := NewDriver(func(c *Context) {
		Menu(c, navTestItems(), &sel).Width(240).Show()
		Button(c, "After")
	}, DriverOptions{Width: 600, Height: 500})
	d.EnableAccessibility()
	d.Settle()
	if a := mustNode(t, d, a11y.RoleMenuItem, "Alpha"); !a.States.Has(a11y.Selected|a11y.Current) || a.SetSize != 3 {
		t.Fatalf("selected item %s", a)
	}
	mustNode(t, d, a11y.RoleSeparator, "")
	b := mustNode(t, d, a11y.RoleMenuItem, "Beta")
	if !b.States.Has(a11y.Expandable) || b.States.Has(a11y.Expanded) || !b.Actions.Has(a11y.Expand) {
		t.Fatalf("submenu item %s %s", b, b.Actions)
	}
	// One Tab stop (the selected item), then out of the menu.
	d.Key(input.KeyTab, 0)
	if f := d.A11yFocused(); f == nil || f.Name != "Alpha" {
		t.Fatalf("first Tab: %v", f)
	}
	d.Key(input.KeyTab, 0)
	if f := d.A11yFocused(); f == nil || f.Name != "After" {
		t.Fatalf("second Tab left the menu at %v", f)
	}
	d.Key(input.KeyTab, input.ModShift)
	d.Key(input.KeyDown, 0)
	if f := d.A11yFocused(); f == nil || f.Name != "Beta" {
		t.Fatalf("Down: %v", f)
	}
	d.Key(input.KeyRight, 0)
	b = mustNode(t, d, a11y.RoleMenuItem, "Beta")
	sub := mustNode(t, d, a11y.RoleMenu, "Beta")
	if !b.States.Has(a11y.Expanded) || b.Controls != sub.ID {
		t.Fatalf("Right did not expand: %s controls=%d sub=%d", b, b.Controls, sub.ID)
	}
	d.Key(input.KeyRight, 0)
	if f := d.A11yFocused(); f == nil || f.Name != "Beta 1" {
		t.Fatalf("Right into submenu: %v", f)
	}
	d.Key(input.KeyEnter, 0)
	if sel != "b1" {
		t.Fatalf("Enter chose %q", sel)
	}
	d.Key(input.KeyLeft, 0)
	if f := d.A11yFocused(); f == nil || f.Name != "Beta" {
		t.Fatalf("Left to parent: %v", f)
	}
	if err := d.PerformAction(mustNode(t, d, a11y.RoleMenuItem, "Beta").ID, a11y.Collapse, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if _, ok := d.A11yFind(a11y.RoleMenuItem, "Beta 1"); ok {
		t.Fatal("Collapse action kept the submenu open")
	}
}

func TestA11yNavMenuPopups(t *testing.T) {
	for _, mode := range []MenuMode{MenuHorizontal, MenuVertical} {
		sel := "a"
		d := NewDriver(func(c *Context) {
			Menu(c, navTestItems(), &sel).Mode(mode).Width(200).Show()
		}, DriverOptions{Width: 800, Height: 500})
		d.EnableAccessibility()
		d.Settle()
		bar := a11y.RoleMenu
		if mode == MenuHorizontal {
			bar = a11y.RoleMenuBar
		}
		if _, ok := d.A11yFind(bar, ""); !ok {
			t.Fatalf("mode %d: no %s:\n%s", mode, bar, d.AccessibilityTree().Dump())
		}
		b := mustNode(t, d, a11y.RoleMenuItem, "Beta")
		if !b.States.Has(a11y.HasPopup | a11y.Expandable) {
			t.Fatalf("mode %d: %s", mode, b)
		}
		// A screen reader's Expand opens the pop-out in keyboard mode.
		if err := d.PerformAction(b.ID, a11y.Expand, ""); err != nil {
			t.Fatal(err)
		}
		d.Settle()
		if f := d.A11yFocused(); f == nil || f.Name != "Beta 1" {
			t.Fatalf("mode %d: Expand focus %v\n%s", mode, f, d.AccessibilityTree().Dump())
		}
		if !mustNode(t, d, a11y.RoleMenuItem, "Beta").States.Has(a11y.Expanded) {
			t.Fatalf("mode %d: not expanded", mode)
		}
		d.Key(input.KeyEscape, 0)
		if f := d.A11yFocused(); f == nil || f.Name != "Beta" {
			t.Fatalf("mode %d: Escape focus %v", mode, f)
		}
		if _, ok := d.A11yFind(a11y.RoleMenuItem, "Beta 1"); ok {
			t.Fatalf("mode %d: Escape kept the pop-out", mode)
		}
		// Keyboard: Down (bar) / Right (vertical) opens and enters it.
		k := input.KeyRight
		if mode == MenuHorizontal {
			k = input.KeyDown
		}
		d.Key(k, 0)
		if f := d.A11yFocused(); f == nil || f.Name != "Beta 1" {
			t.Fatalf("mode %d: key focus %v", mode, f)
		}
		d.Key(input.KeyEnter, 0)
		if sel != "b1" {
			t.Fatalf("mode %d: Enter chose %q", mode, sel)
		}
	}
}

func TestA11yNavDropdown(t *testing.T) {
	items := navTestItems()
	var picked string
	d := NewDriver(func(c *Context) {
		if k := Dropdown(c, items).Trigger(TriggerClick).Body(func() { Button(c, "More") }).Clicked(); k != "" {
			picked = k
		}
		Dropdown(c, items).Body(func() { Text(c, "Hover") })
		Dropdown(c, items).Trigger(TriggerContextMenu).Body(func() {
			Column(c, func() { Text(c, "Area") }, layout.Size(120, 40))
		})
		Dropdown(c, items).Button("Split")
	}, DriverOptions{Width: 800, Height: 600})
	d.EnableAccessibility()
	d.Settle()
	more := mustNode(t, d, a11y.RoleButton, "More")
	if !more.States.Has(a11y.HasPopup|a11y.Expandable) || more.States.Has(a11y.Expanded) {
		t.Fatalf("trigger %s", more)
	}
	// A plain-text trigger is itself a focusable popup button.
	if h := mustNode(t, d, a11y.RoleButton, "Hover"); !h.States.Has(a11y.Focusable | a11y.HasPopup) {
		t.Fatalf("text trigger %s", h)
	}
	mustNode(t, d, a11y.RoleButton, ZhCN.A11y.MoreActions)
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyEnter, 0)
	more = mustNode(t, d, a11y.RoleButton, "More")
	menu, ok := d.A11yFind(a11y.RoleMenu, "")
	if !ok || !more.States.Has(a11y.Expanded) || more.Controls != menu.ID {
		t.Fatalf("open trigger %s controls=%d\n%s", more, more.Controls, d.AccessibilityTree().Dump())
	}
	share := mustNode(t, d, a11y.RoleMenuItem, "Beta")
	if !share.States.Has(a11y.HasPopup | a11y.Expandable) {
		t.Fatalf("submenu item %s", share)
	}
	if f := d.A11yFocused(); f == nil || f.Name != "Alpha" {
		t.Fatalf("Enter moves into the menu: %v", f)
	}
	d.Key(input.KeyDown, 0)
	if f := d.A11yFocused(); f == nil || f.Name != "Beta" {
		t.Fatalf("Down in the menu: %v", f)
	}
	d.Key(input.KeyEscape, 0)
	if f := d.A11yFocused(); f == nil || f.Name != "More" {
		t.Fatalf("Escape returns focus to the trigger: %v", f)
	}
	if _, ok := d.A11yFind(a11y.RoleMenuItem, "Alpha"); ok {
		t.Fatal("Escape kept the menu open")
	}
	d.Key(input.KeyDown, 0) // opens and enters too
	d.Key(input.KeyEnter, 0)
	if picked != "a" {
		t.Fatalf("picked %q", picked)
	}
	if f := d.A11yFocused(); f == nil || f.Name != "More" {
		t.Fatalf("focus after picking: %v", f)
	}
	// Context-menu area: focusable, opens with the Menu key.
	area := mustNode(t, d, a11y.RoleButton, "Area")
	if err := d.PerformAction(area.ID, a11y.Focus, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	d.Key(input.KeyContextMenu, 0)
	if f := d.A11yFocused(); f == nil || f.Name != "Alpha" {
		t.Fatalf("context menu key: focus %v\n%s", f, d.AccessibilityTree().Dump())
	}
}

func TestA11yNavStepsAnchorHeader(t *testing.T) {
	cur := 1
	d := NewDriver(func(c *Context) {
		PageHeader(c, "Orders").Back().Show()
		Steps(c, []StepItem{{Title: "One"}, {Title: "Two", Description: "second"}, {Title: "Three"}}).Clickable(&cur).Show()
		Steps(c, []StepItem{{Title: "Plain"}, {Title: "Bad"}}).Current(1).Status("error").Show()
		Anchor(c, []AnchorItem{{Key: "x", Title: "Intro"}, {Key: "y", Title: "Usage", Children: []AnchorItem{{Key: "z", Title: "Deep"}}}}).Show()
	}, DriverOptions{Width: 900, Height: 700})
	d.EnableAccessibility()
	d.Settle()
	if h := mustNode(t, d, a11y.RoleHeading, "Orders"); h.Level != 1 {
		t.Fatalf("title %s", h)
	}
	mustNode(t, d, a11y.RoleButton, ZhCN.Back)
	mustNode(t, d, a11y.RoleList, ZhCN.Steps)
	two := mustNode(t, d, a11y.RoleButton, "Two")
	if !two.States.Has(a11y.Current|a11y.Focusable) || two.PosInSet != 2 || two.SetSize != 3 || two.Description != "second" {
		t.Fatalf("step %s pos=%d/%d", two, two.PosInSet, two.SetSize)
	}
	if err := d.PerformAction(mustNode(t, d, a11y.RoleButton, "Three").ID, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if cur != 2 {
		t.Fatalf("clicked step: cur=%d", cur)
	}
	if p := mustNode(t, d, a11y.RoleListItem, "Plain"); p.PosInSet != 1 || p.SetSize != 2 {
		t.Fatalf("plain step %s", p)
	}
	if b := mustNode(t, d, a11y.RoleListItem, "Bad"); !b.States.Has(a11y.Current | a11y.Invalid) {
		t.Fatalf("error step %s", b)
	}
	mustNode(t, d, a11y.RoleNavigation, ZhCN.A11y.Navigation)
	if err := d.PerformAction(mustNode(t, d, a11y.RoleLink, "Usage").ID, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if l := mustNode(t, d, a11y.RoleLink, "Usage"); !l.States.Has(a11y.Current) {
		t.Fatalf("active anchor %s", l)
	}
	if l := mustNode(t, d, a11y.RoleLink, "Deep"); l.Level != 2 {
		t.Fatalf("nested anchor %s", l)
	}
}

func TestA11yNavSplitter(t *testing.T) {
	var sp *SplitterW
	d := NewDriver(func(c *Context) {
		sp = Splitter(c).Style(layout.W(606), layout.H(200))
		sp.Body(func() {
			SplitterPanel(c).DefaultPercent(0.5).Collapsible().Body(func() { Text(c, "L") })
			SplitterPanel(c).Body(func() { Text(c, "R") })
		})
	}, DriverOptions{Width: 700, Height: 300})
	d.EnableAccessibility()
	d.Settle()
	d.Settle()
	bar := mustNode(t, d, a11y.RoleSeparator, ZhCN.A11y.Splitter)
	if !bar.States.Has(a11y.Focusable|a11y.Vertical) || !bar.HasRange || bar.ValueNow != 50 ||
		!bar.Actions.Has(a11y.Increment|a11y.Decrement|a11y.SetValue) {
		t.Fatalf("bar %s %s", bar, bar.Actions)
	}
	mustNode(t, d, a11y.RoleButton, ZhCN.Collapse)
	d.Key(input.KeyTab, 0)
	if f := d.A11yFocused(); f == nil || f.ID != bar.ID {
		t.Fatalf("Tab: %v", f)
	}
	d.Key(input.KeyRight, 0)
	if v := mustNode(t, d, a11y.RoleSeparator, ZhCN.A11y.Splitter).ValueNow; v <= 50 {
		t.Fatalf("Right: %g", v)
	}
	if err := d.PerformAction(bar.ID, a11y.SetValue, "30"); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if v := mustNode(t, d, a11y.RoleSeparator, ZhCN.A11y.Splitter).ValueNow; v != 30 {
		t.Fatalf("SetValue 30: %g", v)
	}
	if err := d.PerformAction(bar.ID, a11y.Decrement, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if v := mustNode(t, d, a11y.RoleSeparator, ZhCN.A11y.Splitter).ValueNow; v >= 30 {
		t.Fatalf("Decrement: %g", v)
	}
	d.Key(input.KeyEnter, 0)
	if s := sp.Sizes(); s[0] > 0.001 {
		t.Fatalf("Enter did not collapse: %v", s)
	}
	mustNode(t, d, a11y.RoleButton, ZhCN.Expand)
}

func TestA11yNavCommand(t *testing.T) {
	items := []CommandItem{
		{Value: "cal", Label: "Calendar", Group: "Suggestions", Shortcut: "C"},
		{Value: "emo", Label: "Emoji", Group: "Suggestions"},
		{Value: "set", Label: "Settings", Group: "Settings"},
	}
	var w *CommandW
	d := NewDriver(func(c *Context) { w = Command(c, items); w.Selected() }, DriverOptions{Width: 500, Height: 500})
	d.EnableAccessibility()
	d.Settle()
	cb, ok := d.A11yFind(a11y.RoleCombobox, ZhCN.Command.Placeholder)
	if !ok {
		t.Fatalf("no combobox:\n%s", d.AccessibilityTree().Dump())
	}
	lb := mustNode(t, d, a11y.RoleListbox, ZhCN.Command.Placeholder)
	cal := mustNode(t, d, a11y.RoleOption, "Calendar")
	if !cb.States.Has(a11y.Expanded|a11y.Editable) || cb.Controls != lb.ID || cb.ActiveDescendant != cal.ID {
		t.Fatalf("combobox %s controls=%d active=%d", cb, cb.Controls, cb.ActiveDescendant)
	}
	if !cal.States.Has(a11y.Selected) || cal.PosInSet != 1 || cal.SetSize != 3 || cal.Shortcut != "C" {
		t.Fatalf("option %s", cal)
	}
	mustNode(t, d, a11y.RoleGroup, "Suggestions")
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyDown, 0)
	emo := mustNode(t, d, a11y.RoleOption, "Emoji")
	if cb = mustNode(t, d, a11y.RoleCombobox, ZhCN.Command.Placeholder); cb.ActiveDescendant != emo.ID || !emo.States.Has(a11y.Selected) {
		t.Fatalf("Down: active=%d emoji=%s", cb.ActiveDescendant, emo)
	}
	rep := d.AuditNoContrast()
	for _, is := range rep.Issues {
		t.Errorf("audit: %s", is)
	}
}

func TestA11yNavToggle(t *testing.T) {
	bold := true
	align := []string{"c"}
	d := NewDriver(func(c *Context) {
		Toggle(c, "Bold", &bold)
		ToggleGroup(c, &align, []Option{{Label: "Left", Value: "l"}, {Label: "Center", Value: "c"}, {Label: "Right", Value: "r"}}).
			Single().A11yLabel("Align")
		Kbd(c, "Ctrl K")
	}, DriverOptions{Width: 600})
	d.EnableAccessibility()
	d.Settle()
	b := mustNode(t, d, a11y.RoleButton, "Bold")
	if !b.States.Has(a11y.Toggleable | a11y.Pressed) {
		t.Fatalf("toggle %s", b)
	}
	if err := d.PerformAction(b.ID, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if bold || mustNode(t, d, a11y.RoleButton, "Bold").States.Has(a11y.Pressed) {
		t.Fatal("press did not release the toggle")
	}
	mustNode(t, d, a11y.RoleRadioGroup, "Align")
	if c := mustNode(t, d, a11y.RoleRadio, "Center"); !c.States.Has(a11y.Checked) {
		t.Fatalf("radio %s", c)
	}
	mustNode(t, d, a11y.RoleText, "Ctrl K")
	// Tab (focus is on Bold after the press): the checked toggle only.
	d.Key(input.KeyTab, 0)
	if f := d.A11yFocused(); f == nil || f.Name != "Center" {
		t.Fatalf("Tab into group: %v", f)
	}
	d.Key(input.KeyRight, 0)
	if f := d.A11yFocused(); f == nil || f.Name != "Right" {
		t.Fatalf("Right: %v", f)
	}
	d.Key(input.KeySpace, 0)
	if len(align) != 1 || align[0] != "r" {
		t.Fatalf("Space chose %v", align)
	}
	d.Key(input.KeyTab, 0)
	if f := d.A11yFocused(); f != nil && f.Role == a11y.RoleRadio {
		t.Fatalf("Tab stayed in the group: %v", f)
	}
}

func TestA11yNavFloatAndNotification(t *testing.T) {
	open := false
	d := NewDriver(func(c *Context) {
		FloatButton(c).Tooltip("Help").Right(100)
		FloatButtonGroup(c).Trigger("click").Open(&open).Position(24, 120).Body(func() {
			FloatButton(c).A11yLabel("Chat")
		})
		if Button(c, "Notify").Clicked() {
			Notification(c, "error", "Failed").Description("Disk full")
		}
	}, DriverOptions{Width: 800, Height: 600})
	d.EnableAccessibility()
	d.Settle()
	mustNode(t, d, a11y.RoleButton, "Help")
	tr := mustNode(t, d, a11y.RoleButton, ZhCN.A11y.MoreActions)
	if !tr.States.Has(a11y.Expandable) || tr.States.Has(a11y.Expanded) {
		t.Fatalf("group trigger %s", tr)
	}
	if err := d.PerformAction(tr.ID, a11y.Expand, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if !open || !mustNode(t, d, a11y.RoleButton, ZhCN.A11y.MoreActions).States.Has(a11y.Expanded) {
		t.Fatal("Expand did not open the group")
	}
	mustNode(t, d, a11y.RoleButton, "Chat")
	if err := d.PerformAction(mustNode(t, d, a11y.RoleButton, "Notify").ID, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	n := mustNode(t, d, a11y.RoleAlert, "Failed")
	if n.Live != a11y.LiveAssertive || n.Description != "Disk full" {
		t.Fatalf("notification %s", n)
	}
	mustNode(t, d, a11y.RoleGroup, ZhCN.A11y.Notifications)
	mustNode(t, d, a11y.RoleButton, ZhCN.Close)
}
