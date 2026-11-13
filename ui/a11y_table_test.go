package ui

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
)

func a11yTableCols(names []string) []TableColumn {
	return []TableColumn{
		{Key: "name", Title: "Name", Text: func(i int) string { return names[i] },
			Compare: func(a, b int) int { return strings.Compare(names[a], names[b]) }},
		{Key: "n", Title: "No", Text: func(i int) string { return strconv.Itoa(i + 1) },
			Filters: []Option{{Label: "Odd", Value: "odd"}, {Label: "Even", Value: "even"}}},
	}
}

func childRoles(tr *a11y.Tree, n *a11y.Node, role a11y.Role) []*a11y.Node {
	var out []*a11y.Node
	var walk func(id a11y.NodeID)
	walk = func(id a11y.NodeID) {
		k := tr.Node(id)
		if k == nil {
			return
		}
		if k.Role == role {
			out = append(out, k)
		}
		for _, c := range k.Children {
			walk(c)
		}
	}
	for _, c := range n.Children {
		walk(c)
	}
	return out
}

func TestA11yTableGrid(t *testing.T) {
	names := []string{"Carol", "Alice", "Bob"}
	var sel []int
	d := NewDriver(func(c *Context) {
		Table(c, a11yTableCols(names), len(names)).Selection(&sel).A11yLabel("People").Show()
	}, DriverOptions{Width: 800, Height: 600})
	tr := d.EnableAccessibility()
	g := mustNode(t, d, a11y.RoleGrid, "People")
	if g.RowCount != 4 || g.ColCount != 3 || !g.States.Has(a11y.Focusable|a11y.Multiselectable) {
		t.Fatalf("grid %s rows=%d cols=%d", g, g.RowCount, g.ColCount)
	}
	h := mustNode(t, d, a11y.RoleColumnHeader, "Name")
	if h.RowIndex != 1 || h.ColIndex != 2 || h.Sort != a11y.SortNone || !h.Actions.Has(a11y.Press|a11y.Focus) {
		t.Fatalf("header %s row=%d col=%d sort=%s actions=%s", h, h.RowIndex, h.ColIndex, h.Sort, h.Actions)
	}
	mustNode(t, d, a11y.RoleCheckbox, ZhCN.SelectAll)
	rows := childRoles(tr, g, a11y.RoleRow)
	if len(rows) != 4 { // header row + 3 body rows
		t.Fatalf("%d rows:\n%s", len(rows), tr.Dump())
	}
	r1 := rows[1]
	if r1.RowIndex != 2 || !r1.States.Has(a11y.Selectable) || r1.States.Has(a11y.Selected) || strings.HasPrefix(r1.Name, "row:") {
		t.Fatalf("row %s index=%d", r1, r1.RowIndex)
	}
	cell := mustNode(t, d, a11y.RoleCell, "Carol")
	if cell.RowIndex != 2 || cell.ColIndex != 2 {
		t.Errorf("cell position %d,%d", cell.RowIndex, cell.ColIndex)
	}
	cb := mustNode(t, d, a11y.RoleCheckbox, fmt.Sprintf(ZhCN.Table.SelectRowFmt, 1))
	if cb.States.Has(a11y.Checked) {
		t.Fatal("checked before selection")
	}

	// Sort with the accessibility action, then with Enter on the header.
	if err := d.PerformAction(h.ID, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	if h = mustNode(t, d, a11y.RoleColumnHeader, "Name"); h.Sort != a11y.SortAscending {
		t.Fatalf("sort %s", h.Sort)
	}
	if c := childRoles(d.AccessibilityTree(), mustNode(t, d, a11y.RoleGrid, "People"), a11y.RoleCell); len(c) < 2 || c[1].Name != "Alice" {
		t.Errorf("rows not sorted: %v", c)
	}
	if err := d.PerformAction(h.ID, a11y.Focus, ""); err != nil {
		t.Fatal(err)
	}
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if h = mustNode(t, d, a11y.RoleColumnHeader, "Name"); h.Sort != a11y.SortDescending {
		t.Fatalf("Enter on header: sort %s", h.Sort)
	}

	// Keyboard on the grid: arrows move the active cell, Space selects.
	g = mustNode(t, d, a11y.RoleGrid, "People")
	if err := d.PerformAction(g.ID, a11y.Focus, ""); err != nil {
		t.Fatal(err)
	}
	d.Key(input.KeyDown, 0)
	d.Key(input.KeyDown, 0)
	d.Settle()
	g = mustNode(t, d, a11y.RoleGrid, "People")
	act := d.AccessibilityTree().Node(g.ActiveDescendant)
	if act == nil || act.Role != a11y.RoleCell || act.RowIndex != 4 { // the first key activates row 1, then moves
		t.Fatalf("active descendant %v", act)
	}
	d.Key(input.KeySpace, 0)
	d.Settle()
	if len(sel) != 1 {
		t.Fatalf("space: selection %v", sel)
	}
	rows = childRoles(d.AccessibilityTree(), mustNode(t, d, a11y.RoleGrid, "People"), a11y.RoleRow)
	if !rows[3].States.Has(a11y.Selected) {
		t.Errorf("selected row not reported: %s", rows[3])
	}
	if f := d.A11yFocused(); f == nil || f.Role != a11y.RoleGrid {
		t.Errorf("focus %v", f)
	}
	if rep := d.Audit(); rep.MissingInteractiveNames() > 0 || rep.Count(AuditNoRole) > 0 || rep.Count(AuditNotKeyboard) > 0 {
		t.Errorf("audit:\n%s", rep)
	}
}

func TestA11yTableRowCheckboxNotTabStop(t *testing.T) {
	var sel []int
	d := NewDriver(func(c *Context) {
		Table(c, []TableColumn{{Key: "a", Title: "A", Text: func(i int) string { return "x" + strconv.Itoa(i) }}}, 3).Selection(&sel).Show()
	}, DriverOptions{Width: 600, Height: 400})
	d.EnableAccessibility()
	seen := map[a11y.Role]int{}
	for i := 0; i < 4; i++ {
		d.Key(input.KeyTab, 0)
		d.Settle()
		if f := d.A11yFocused(); f != nil {
			seen[f.Role]++
			if f.Role == a11y.RoleCheckbox && f.Name != ZhCN.SelectAll {
				t.Fatalf("row checkbox is a Tab stop: %s", f)
			}
		}
	}
	if seen[a11y.RoleGrid] == 0 {
		t.Errorf("grid never focused: %v", seen)
	}
}

func TestA11yTableFilterAndResize(t *testing.T) {
	names := []string{"a", "b"}
	d := NewDriver(func(c *Context) {
		Table(c, a11yTableCols(names), 2).Resizable().Show()
	}, DriverOptions{Width: 800, Height: 600})
	d.EnableAccessibility()
	trig := mustNode(t, d, a11y.RoleButton, ZhCN.Table.Filter)
	if !trig.States.Has(a11y.Focusable|a11y.Expandable|a11y.HasPopup) || trig.States.Has(a11y.Expanded) {
		t.Fatalf("filter trigger %s", trig)
	}
	if err := d.PerformAction(trig.ID, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	if trig = mustNode(t, d, a11y.RoleButton, ZhCN.Table.Filter); !trig.States.Has(a11y.Expanded) {
		t.Fatalf("not expanded: %s", trig)
	}
	lb := mustNode(t, d, a11y.RoleListbox, "No")
	if !lb.States.Has(a11y.Multiselectable) {
		t.Errorf("listbox %s", lb)
	}
	opt := mustNode(t, d, a11y.RoleOption, "Odd")
	if opt.SetSize != 2 || opt.States.Has(a11y.Selected) {
		t.Fatalf("option %s %d", opt, opt.SetSize)
	}
	if err := d.PerformAction(opt.ID, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	if opt = mustNode(t, d, a11y.RoleOption, "Odd"); !opt.States.Has(a11y.Selected) {
		t.Fatalf("option not selected: %s", opt)
	}
	if rep := d.Audit(); rep.MissingInteractiveNames() > 0 || rep.Count(AuditNoRole) > 0 {
		t.Errorf("audit with filter open:\n%s", rep)
	}
	d.Key(input.KeyEscape, 0)
	d.Settle()

	seps := d.A11yFindAll(a11y.RoleSeparator, ZhCN.A11y.Resize)
	if len(seps) != 2 || !seps[0].HasRange || !seps[0].States.Has(a11y.Focusable) {
		t.Fatalf("resize handles %v", seps)
	}
	w0 := seps[0].ValueNow
	if err := d.PerformAction(seps[0].ID, a11y.Increment, ""); err != nil {
		t.Fatal(err)
	}
	if s := d.A11yFindAll(a11y.RoleSeparator, ZhCN.A11y.Resize)[0]; s.ValueNow != w0+10 {
		t.Errorf("width %v -> %v", w0, s.ValueNow)
	}
}

func TestA11yTableExpandTreeDrag(t *testing.T) {
	var keys []string
	moved := [2]int{-1, -1}
	cols := []TableColumn{{Key: "a", Title: "A", Text: func(i int) string { return "r" + strconv.Itoa(i) }}}
	d := NewDriver(func(c *Context) {
		Table(c, cols, 3).Expandable(func(int) { Text(c, "detail") }).ExpandedKeys(&keys).
			DragSort(func(from, to int) { moved = [2]int{from, to} }).Show()
	}, DriverOptions{Width: 600, Height: 400})
	tr := d.EnableAccessibility()
	g := mustNode(t, d, a11y.RoleGrid, "")
	rows := childRoles(tr, g, a11y.RoleRow)
	r := rows[1]
	if !r.States.Has(a11y.Expandable) || r.States.Has(a11y.Expanded) || !r.Actions.Has(a11y.Expand) {
		t.Fatalf("row %s %s", r, r.Actions)
	}
	mustNode(t, d, a11y.RoleButton, ZhCN.ExpandRow)
	if len(d.A11yFindAll(a11y.RoleButton, ZhCN.A11y.DragHandle)) != 3 {
		t.Fatal("drag handles not named")
	}
	if err := d.PerformAction(r.ID, a11y.Expand, ""); err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 {
		t.Fatalf("expand action: %v", keys)
	}
	rows = childRoles(d.AccessibilityTree(), mustNode(t, d, a11y.RoleGrid, ""), a11y.RoleRow)
	if !rows[1].States.Has(a11y.Expanded) {
		t.Fatalf("not expanded: %s", rows[1])
	}
	mustNode(t, d, a11y.RoleButton, ZhCN.CollapseRow)

	// Alt+Down moves the active row (drag handle alternative).
	g = mustNode(t, d, a11y.RoleGrid, "")
	_ = d.PerformAction(g.ID, a11y.Focus, "")
	d.Key(input.KeyDown, input.ModAlt) // activates the first row and moves it
	d.Settle()
	if moved != [2]int{0, 1} {
		t.Errorf("Alt+Down moved %v", moved)
	}
	if rep := d.Audit(); rep.MissingInteractiveNames() > 0 || rep.Count(AuditNoRole) > 0 || rep.Count(AuditNotKeyboard) > 0 {
		t.Errorf("audit:\n%s", rep)
	}

	// Tree rows carry their level.
	roots := []int{0}
	kids := func(i int) []int {
		if i == 0 {
			return []int{1, 2}
		}
		return nil
	}
	open := []string{"0"}
	d2 := NewDriver(func(c *Context) {
		Table(c, cols, 3).Tree(roots, kids).TreeOpenKeys(&open).Show()
	}, DriverOptions{Width: 600, Height: 400})
	tr2 := d2.EnableAccessibility()
	rows = childRoles(tr2, mustNode(t, d2, a11y.RoleGrid, ""), a11y.RoleRow)
	if len(rows) != 4 || rows[1].Level != 1 || !rows[1].States.Has(a11y.Expandable|a11y.Expanded) || rows[2].Level != 2 {
		t.Fatalf("tree rows:\n%s", tr2.Dump())
	}
	if err := d2.PerformAction(rows[1].ID, a11y.Collapse, ""); err != nil || len(open) != 0 {
		t.Fatalf("collapse: %v %v", err, open)
	}
}

func TestA11yTableVirtualRowCount(t *testing.T) {
	cols := []TableColumn{{Key: "a", Title: "A", Text: func(i int) string { return "v" + strconv.Itoa(i) }}}
	d := NewDriver(func(c *Context) {
		Table(c, cols, 10000).ScrollY(300).Virtual().Summary(func(string, []int) string { return "sum" }).Show()
	}, DriverOptions{Width: 600, Height: 500})
	tr := d.EnableAccessibility()
	g := mustNode(t, d, a11y.RoleGrid, "")
	rows := childRoles(tr, g, a11y.RoleRow)
	if g.RowCount != 10002 || len(rows) > 40 {
		t.Fatalf("row count %d, %d rows built", g.RowCount, len(rows))
	}
	if last := rows[len(rows)-1]; last.RowIndex != 10002 {
		t.Errorf("summary row index %d", last.RowIndex)
	}
	_ = d.PerformAction(g.ID, a11y.Focus, "")
	d.Key(input.KeyEnd, input.ModCtrl)
	d.Settle()
	g = mustNode(t, d, a11y.RoleGrid, "")
	if act := d.AccessibilityTree().Node(g.ActiveDescendant); act == nil || act.Name != "v9999" || act.RowIndex != 10001 {
		t.Fatalf("active cell after Ctrl+End: %v", act)
	}
}

func TestA11yProTableToolbar(t *testing.T) {
	type row struct{ name string }
	d := NewDriver(func(c *Context) {
		ProTable(c, []ProColumn[row]{{Key: "name", Title: "Name", Value: func(r row) any { return r.name }}}).
			Title("Users").NoSearch().
			Request(func(_ ProRequest, done func(ProResult[row])) { done(ProResult[row]{Rows: []row{{"a"}}, Total: 1}) }).Show()
	}, DriverOptions{Width: 1000, Height: 700})
	d.EnableAccessibility()
	mustNode(t, d, a11y.RoleGrid, "Users")
	for _, name := range []string{ZhCN.ProTable.Reload, ZhCN.ProTable.Density, ZhCN.ProTable.ColumnSetting, ZhCN.Fullscreen} {
		if b := mustNode(t, d, a11y.RoleButton, name); !b.States.Has(a11y.Focusable) {
			t.Errorf("%s not focusable", name)
		}
	}
	dens := mustNode(t, d, a11y.RoleButton, ZhCN.ProTable.Density)
	if err := d.PerformAction(dens.ID, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	mustNode(t, d, a11y.RoleMenu, ZhCN.ProTable.Density)
	cur := mustNode(t, d, a11y.RoleMenuItem, ZhCN.ProTable.DensityDefault)
	if !cur.States.Has(a11y.Checked) {
		t.Errorf("current density not checked: %s", cur)
	}
	if f := d.A11yFocused(); f == nil || f.ID != cur.ID {
		t.Fatalf("focus not in the menu: %v", f)
	}
	d.Key(input.KeyDown, 0)
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if _, open := d.A11yFind(a11y.RoleMenu, ZhCN.ProTable.Density); open {
		t.Fatal("menu still open")
	}
	if b := mustNode(t, d, a11y.RoleButton, ZhCN.ProTable.Density); b.States.Has(a11y.Expanded) {
		t.Errorf("trigger expanded after close: %s", b)
	}
	if rep := d.Audit(); rep.MissingInteractiveNames() > 0 || rep.Count(AuditNoRole) > 0 || rep.Count(AuditNotKeyboard) > 0 {
		t.Errorf("audit:\n%s", rep)
	}

	set := mustNode(t, d, a11y.RoleButton, ZhCN.ProTable.ColumnSetting)
	if err := d.PerformAction(set.ID, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	mustNode(t, d, a11y.RoleDialog, ZhCN.ProTable.ColumnSetting)
	mustNode(t, d, a11y.RoleCheckbox, "Name")
	mustNode(t, d, a11y.RoleButton, ZhCN.A11y.DragHandle)
	mustNode(t, d, a11y.RoleButton, ZhCN.Table.PinLeft)
	if rep := d.Audit(); rep.MissingInteractiveNames() > 0 || rep.Count(AuditNoRole) > 0 || rep.Count(AuditNotKeyboard) > 0 {
		t.Errorf("audit with column settings:\n%s", rep)
	}
}
