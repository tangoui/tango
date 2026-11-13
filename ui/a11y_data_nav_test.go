package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// Accessibility of Tree, Calendar, Carousel and Collapse.

func auditClean(t *testing.T, d *Driver) {
	t.Helper()
	rep := d.AuditNoContrast()
	for _, i := range rep.Issues {
		t.Errorf("audit: %s", i)
	}
}

func TestA11yTree(t *testing.T) {
	nodes := []TreeNode{
		{Key: "a", Title: "Alpha", Children: []TreeNode{{Key: "a1", Title: "Alpha one"}, {Key: "a2", Title: "Alpha two"}}},
		{Key: "b", Title: "Beta"},
	}
	var checked, selected []string
	expanded := []string{}
	d := NewDriver(func(c *Context) {
		Tree(c, nodes).Expanded(&expanded).Selected(&selected).Checkable(&checked).A11yLabel("Files").Show()
	}, DriverOptions{})
	d.EnableAccessibility()
	tree := mustNode(t, d, a11y.RoleTree, "Files")
	a := mustNode(t, d, a11y.RoleTreeItem, "Alpha")
	if a.Level != 1 || a.PosInSet != 1 || a.SetSize != 2 || !a.States.Has(a11y.Expandable|a11y.Checkable) || a.States.Has(a11y.Expanded) {
		t.Fatalf("alpha: %s", a)
	}
	if !a.Actions.Has(a11y.Expand | a11y.Press) {
		t.Fatalf("alpha actions %s", a.Actions)
	}
	if err := d.PerformAction(a.ID, a11y.Expand, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if len(expanded) != 1 || len(selected) != 0 {
		t.Fatalf("expand action: expanded=%v selected=%v", expanded, selected)
	}
	one := mustNode(t, d, a11y.RoleTreeItem, "Alpha one")
	if one.Level != 2 || one.PosInSet != 1 || one.SetSize != 2 {
		t.Fatalf("child: %s", one)
	}
	if err := d.A11yPerform(a11y.RoleTreeItem, "Alpha", a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if len(selected) != 1 || selected[0] != "a" || len(expanded) != 1 {
		t.Fatalf("press: selected=%v expanded=%v", selected, expanded)
	}
	// Keyboard (Press focused the tree; focus is reported on its active
	// item): Down twice, Space checks "Alpha two".
	if f := d.A11yFocused(); f == nil || (f.ID != tree.ID && f.ID != a.ID) {
		t.Fatalf("focus %v", f)
	}
	d.Key(input.KeyDown, 0)
	d.Key(input.KeyDown, 0)
	d.Key(input.KeySpace, 0)
	d.Settle()
	two := mustNode(t, d, a11y.RoleTreeItem, "Alpha two")
	if tr := mustNode(t, d, a11y.RoleTree, "Files"); tr.ActiveDescendant != two.ID {
		t.Fatalf("active descendant %d, want %d", tr.ActiveDescendant, two.ID)
	}
	if !two.States.Has(a11y.Checked) {
		t.Fatalf("space did not check: %s (%v)", two, checked)
	}
	if a := mustNode(t, d, a11y.RoleTreeItem, "Alpha"); !a.States.Has(a11y.Mixed) || !a.States.Has(a11y.Expanded) {
		t.Fatalf("parent: %s", a)
	}
	d.Key(input.KeyLeft, 0) // to the parent
	d.Key(input.KeyLeft, 0) // collapse
	d.Settle()
	if len(expanded) != 0 {
		t.Fatalf("left did not collapse: %v", expanded)
	}
	auditClean(t, d)
}

func TestA11yCalendar(t *testing.T) {
	day := time.Date(2024, 5, 15, 0, 0, 0, 0, time.UTC)
	d := NewDriver(func(c *Context) { Calendar(c, &day).Card() }, DriverOptions{Width: 600, Height: 600})
	d.EnableAccessibility()
	loc := d.Runner().Context().Locale()
	name := loc.Date(2024, 5, 15)
	cell := mustNode(t, d, a11y.RoleCell, name)
	if !cell.States.Has(a11y.Selected | a11y.Focusable) {
		t.Fatalf("cell: %s", cell)
	}
	grid := mustNode(t, d, a11y.RoleGrid, dtMonthTitle(loc, day))
	if len(grid.Children) < 2 {
		t.Fatalf("grid rows: %v", grid.Children)
	}
	mustNode(t, d, a11y.RoleCombobox, loc.Calendar.Year)
	mustNode(t, d, a11y.RoleRadio, loc.Calendar.Month)
	// Only the selected date is a Tab stop; arrows move by day / week.
	if err := d.PerformAction(cell.ID, a11y.Focus, ""); err != nil {
		t.Fatal(err)
	}
	d.Key(input.KeyRight, 0)
	d.Settle()
	if f := d.A11yFocused(); f == nil || f.Name != loc.Date(2024, 5, 16) {
		t.Fatalf("after Right: %v", f)
	}
	d.Key(input.KeyDown, 0)
	d.Key(input.KeyDown, 0)
	d.Key(input.KeyDown, 0)
	d.Settle()
	// May 16 + 21 days = June 6: the grid turned the page.
	if f := d.A11yFocused(); f == nil || f.Name != loc.Date(2024, 6, 6) {
		t.Fatalf("after Down x3: %v", f)
	}
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if day.Month() != 6 || day.Day() != 6 {
		t.Fatalf("enter picked %v", day)
	}
	auditClean(t, d)
}

func TestA11yCarousel(t *testing.T) {
	cur := 0
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			Carousel(c, 3, func(i int) { Text(c, "slide") }).Current(&cur).Arrows("always").Show()
		}, layout.W(400))
	}, DriverOptions{})
	d.EnableAccessibility()
	loc := d.Runner().Context().Locale()
	mustNode(t, d, a11y.RoleGroup, loc.Slide(1, 3))
	mustNode(t, d, a11y.RoleButton, loc.Carousel.Next)
	mustNode(t, d, a11y.RoleTabList, "")
	dot := mustNode(t, d, a11y.RoleTab, loc.Slide(1, 3))
	if !dot.States.Has(a11y.Selected) || dot.SetSize != 3 {
		t.Fatalf("dot: %s", dot)
	}
	if err := d.PerformAction(dot.ID, a11y.Focus, ""); err != nil {
		t.Fatal(err)
	}
	d.Key(input.KeyRight, 0)
	d.Settle()
	if cur != 1 {
		t.Fatalf("Right on dots: cur=%d", cur)
	}
	if f := d.A11yFocused(); f == nil || f.Name != loc.Slide(2, 3) {
		t.Fatalf("focus %v", f)
	}
	if err := d.A11yPerform(a11y.RoleButton, loc.Carousel.Prev, a11y.Press, ""); err != nil || cur != 0 {
		t.Fatalf("prev: %v cur=%d", err, cur)
	}
	auditClean(t, d)
}

func TestA11yCarouselAutoplayReduceMotion(t *testing.T) {
	cur := 0
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			Carousel(c, 3, func(i int) { Text(c, "slide") }).Current(&cur).Autoplay(time.Second).Show()
		}, layout.W(400))
	}, DriverOptions{Animations: true, ReduceMotion: true})
	d.Settle()
	d.Advance(3 * time.Second)
	d.Frame()
	if cur != 0 {
		t.Fatalf("autoplay ran with reduced motion: cur=%d", cur)
	}
}

func TestA11yCollapse(t *testing.T) {
	var active []string
	d := NewDriver(func(c *Context) {
		Collapse(c, &active, []CollapseItem{
			{Key: "1", Label: "First", Body: func() { Text(c, "one") }},
			{Key: "2", Label: "Second", Body: func() { Text(c, "two") }, Disabled: true},
		}).Show()
	}, DriverOptions{})
	d.EnableAccessibility()
	h := mustNode(t, d, a11y.RoleButton, "First")
	if !h.States.Has(a11y.Expandable|a11y.Focusable) || h.States.Has(a11y.Expanded) || !h.Actions.Has(a11y.Expand) {
		t.Fatalf("header: %s %s", h, h.Actions)
	}
	if s := mustNode(t, d, a11y.RoleButton, "Second"); !s.States.Has(a11y.Disabled) || s.States.Has(a11y.Focusable) {
		t.Fatalf("disabled header: %s", s)
	}
	d.Key(input.KeyTab, 0)
	d.Key(input.KeySpace, 0)
	d.Settle()
	if len(active) != 1 {
		t.Fatalf("space did not open: %v", active)
	}
	h = mustNode(t, d, a11y.RoleButton, "First")
	p := mustNode(t, d, a11y.RoleGroup, "First")
	if !h.States.Has(a11y.Expanded) || h.Controls != p.ID {
		t.Fatalf("open header: %s controls=%d panel=%d", h, h.Controls, p.ID)
	}
	if err := d.PerformAction(h.ID, a11y.Collapse, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if len(active) != 0 {
		t.Fatalf("collapse action: %v", active)
	}
	auditClean(t, d)
}
