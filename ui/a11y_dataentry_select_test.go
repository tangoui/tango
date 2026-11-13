package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
)

// Accessibility of the select-like data entry widgets (MultiSelect,
// AutoComplete, Cascader, TreeSelect, InputTag, Mentions, Transfer,
// CheckAll).

func enDriver(body func(c *Context)) *Driver {
	return NewDriver(func(c *Context) {
		ConfigProvider(c).Locale(EnUS).Body(func() { Column(c, func() { body(c) }) })
	}, DriverOptions{Width: 900, Height: 700})
}

func TestA11yMultiSelect(t *testing.T) {
	vals := []string{"a", "b"}
	opts := []Option{{Label: "Apple", Value: "a"}, {Label: "Banana", Value: "b"}, {Label: "Cherry", Value: "c"}}
	d := enDriver(func(c *Context) { MultiSelect(c, &vals, opts).A11yLabel("Fruit") })
	d.EnableAccessibility()
	cb := mustNode(t, d, a11y.RoleCombobox, "Fruit")
	if cb.Value != "Apple, Banana" || !cb.States.Has(a11y.Expandable|a11y.HasPopup) || cb.States.Has(a11y.Expanded) {
		t.Fatalf("combobox %+v", cb)
	}
	// Tag remove buttons.
	if err := d.A11yPerform(a11y.RoleButton, "Remove Apple", a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if len(vals) != 1 || vals[0] != "b" {
		t.Fatalf("remove button: %v", vals)
	}
	// Open with the keyboard: Tab focuses the editor (reported on the
	// combobox), Down opens the multi-selectable listbox.
	d.Key(input.KeyTab, 0)
	d.Settle()
	if f := d.A11yFocused(); f == nil || f.Role != a11y.RoleCombobox {
		t.Fatalf("focus %v", f)
	}
	d.Key(input.KeyDown, 0)
	d.Settle()
	cb = mustNode(t, d, a11y.RoleCombobox, "Fruit")
	lb := mustNode(t, d, a11y.RoleListbox, "Fruit")
	if !cb.States.Has(a11y.Expanded) || cb.Controls != lb.ID || !lb.States.Has(a11y.Multiselectable) {
		t.Fatalf("open combobox %+v listbox %+v", cb, lb)
	}
	ban := mustNode(t, d, a11y.RoleOption, "Banana")
	if !ban.States.Has(a11y.Selected) || mustNode(t, d, a11y.RoleOption, "Apple").States.Has(a11y.Selected) {
		t.Fatal("option selected states")
	}
	if cb.ActiveDescendant != mustNode(t, d, a11y.RoleOption, "Apple").ID {
		t.Fatalf("active descendant %d", cb.ActiveDescendant)
	}
	// Backspace removes the last tag.
	d.Key(input.KeyBackspace, 0)
	d.Settle()
	if len(vals) != 0 {
		t.Fatalf("backspace: %v", vals)
	}
	if rep := d.AuditNoContrast(); rep.MissingInteractiveNames() != 0 || rep.Count(AuditNoRole) != 0 || rep.Count(AuditNotKeyboard) != 0 {
		t.Fatalf("audit:\n%s", rep)
	}
}

func TestA11yAutoComplete(t *testing.T) {
	v := ""
	d := enDriver(func(c *Context) {
		AutoComplete(c, &v, Opts("Downing Street", "Wall Street")).Placeholder("Address").Open()
	})
	d.EnableAccessibility()
	cb := mustNode(t, d, a11y.RoleCombobox, "Address")
	lb := mustNode(t, d, a11y.RoleListbox, "Address")
	if !cb.States.Has(a11y.Editable|a11y.Expanded) || cb.Controls != lb.ID || !cb.Actions.Has(a11y.SetValue) {
		t.Fatalf("combobox %+v", cb)
	}
	if o := mustNode(t, d, a11y.RoleOption, "Downing Street"); cb.ActiveDescendant != o.ID || o.SetSize != 2 {
		t.Fatalf("option %+v active %d", o, cb.ActiveDescendant)
	}
	if err := d.PerformAction(cb.ID, a11y.SetValue, "Wall"); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if v != "Wall" {
		t.Fatalf("set value %q", v)
	}
}

func TestA11yCascader(t *testing.T) {
	var val []string
	d := enDriver(func(c *Context) {
		Cascader(c, &val, casTestOpts).Placeholder("Region").Open().Expand("zj", "hz")
	})
	d.EnableAccessibility()
	mustNode(t, d, a11y.RoleCombobox, "Region")
	mustNode(t, d, a11y.RoleListbox, "Region")
	col2 := mustNode(t, d, a11y.RoleListbox, "浙江")
	mustNode(t, d, a11y.RoleListbox, "杭州")
	zj := mustNode(t, d, a11y.RoleOption, "浙江")
	if !zj.States.Has(a11y.Expandable | a11y.Expanded) {
		t.Fatalf("parent option %s", zj.States)
	}
	if xh := mustNode(t, d, a11y.RoleOption, "西湖"); xh.States.Has(a11y.Expandable) {
		t.Fatal("leaf is expandable")
	}
	if len(col2.Children) != 3 {
		t.Fatalf("second column has %d options", len(col2.Children))
	}
	if !mustNode(t, d, a11y.RoleOption, "温州").States.Has(a11y.Disabled) {
		t.Fatal("disabled option")
	}
	if err := d.A11yPerform(a11y.RoleOption, "西湖", a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if len(val) != 3 || val[2] != "xh" {
		t.Fatalf("picked %v", val)
	}
}

func TestA11yCascaderMultipleMixed(t *testing.T) {
	var val []string
	multi := [][]string{{"zj", "hz", "xh"}}
	d := enDriver(func(c *Context) {
		Cascader(c, &val, casTestOpts).Placeholder("Region").Multiple(&multi).Open().Expand("zj", "hz")
	})
	d.EnableAccessibility()
	if lb := mustNode(t, d, a11y.RoleListbox, "Region"); !lb.States.Has(a11y.Multiselectable) {
		t.Fatal("not multiselectable")
	}
	if s := mustNode(t, d, a11y.RoleOption, "杭州").States; !s.Has(a11y.Checkable|a11y.Mixed) || s.Has(a11y.Checked) {
		t.Fatalf("partially checked parent %s", s)
	}
	if s := mustNode(t, d, a11y.RoleOption, "西湖").States; !s.Has(a11y.Checked) {
		t.Fatalf("checked leaf %s", s)
	}
}

func TestA11yTreeSelect(t *testing.T) {
	v := "xh"
	d := enDriver(func(c *Context) { TreeSelect(c, &v, tsTestTree).A11yLabel("Area").Open() })
	d.EnableAccessibility()
	cb := mustNode(t, d, a11y.RoleCombobox, "Area")
	tree := mustNode(t, d, a11y.RoleTree, "Area")
	if cb.Value != "西湖区" || cb.Controls != tree.ID {
		t.Fatalf("combobox %+v", cb)
	}
	zj := mustNode(t, d, a11y.RoleTreeItem, "浙江省")
	xh := mustNode(t, d, a11y.RoleTreeItem, "西湖区")
	if zj.Level != 1 || !zj.States.Has(a11y.Expanded) || zj.PosInSet != 1 || zj.SetSize != 2 {
		t.Fatalf("root item %+v", zj)
	}
	if xh.Level != 3 || !xh.States.Has(a11y.Selected) || xh.PosInSet != 1 || xh.SetSize != 3 {
		t.Fatalf("leaf item %+v", xh)
	}
	nb := mustNode(t, d, a11y.RoleTreeItem, "宁波市")
	if nb.Level != 2 || nb.PosInSet != 2 {
		t.Fatalf("second-level item %+v", nb)
	}
	if err := d.PerformAction(zj.ID, a11y.Collapse, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if _, ok := d.A11yFind(a11y.RoleTreeItem, "宁波市"); ok {
		t.Fatal("collapse did not hide children")
	}
	if v != "xh" {
		t.Fatalf("collapse changed the value to %q", v)
	}
	if s := mustNode(t, d, a11y.RoleTreeItem, "浙江省").States; s.Has(a11y.Expanded) || !s.Has(a11y.Expandable) {
		t.Fatalf("collapsed item %s", s)
	}
}

func TestA11yTreeSelectCheckable(t *testing.T) {
	v := ""
	vals := []string{"xh"}
	d := enDriver(func(c *Context) {
		TreeSelect(c, &v, tsTestTree).Multiple(&vals).Checkable().A11yLabel("Area").DefaultExpandAll().Open()
	})
	d.EnableAccessibility()
	if !mustNode(t, d, a11y.RoleTree, "Area").States.Has(a11y.Multiselectable) {
		t.Fatal("tree not multiselectable")
	}
	if s := mustNode(t, d, a11y.RoleTreeItem, "杭州市").States; !s.Has(a11y.Checkable|a11y.Mixed) || s.Has(a11y.Checked) {
		t.Fatalf("half-checked %s", s)
	}
	if s := mustNode(t, d, a11y.RoleTreeItem, "西湖区").States; !s.Has(a11y.Checked) {
		t.Fatalf("checked %s", s)
	}
	mustNode(t, d, a11y.RoleButton, "Remove 西湖区")
}

func TestA11yInputTag(t *testing.T) {
	tags := []string{"go", "ui"}
	d := enDriver(func(c *Context) { InputTag(c, &tags).A11yLabel("Tags").AllowClear() })
	d.EnableAccessibility()
	tb := mustNode(t, d, a11y.RoleTextbox, "Tags")
	if !tb.Actions.Has(a11y.SetValue) {
		t.Fatalf("textbox %+v", tb)
	}
	if _, ok := d.A11yFind(a11y.RoleCombobox, ""); ok {
		t.Fatal("InputTag exposed as a combobox")
	}
	if err := d.A11yPerform(a11y.RoleButton, "Remove ui", a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if len(tags) != 1 || tags[0] != "go" {
		t.Fatalf("tags %v", tags)
	}
	d.Key(input.KeyTab, 0)
	d.Settle()
	if f := d.A11yFocused(); f == nil || f.Name != "Tags" {
		t.Fatalf("focus %v", f)
	}
	d.Type("new")
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if len(tags) != 2 || tags[1] != "new" {
		t.Fatalf("typed tag: %v", tags)
	}
}

func TestA11yMentions(t *testing.T) {
	v := "@"
	users := Opts("alice", "bob")
	d := enDriver(func(c *Context) { Mentions(c, &v, users).Placeholder("Comment").Open() })
	d.EnableAccessibility()
	tb := mustNode(t, d, a11y.RoleTextbox, "Comment")
	lb := mustNode(t, d, a11y.RoleListbox, "Comment")
	if !tb.States.Has(a11y.Multiline|a11y.HasPopup) || tb.Controls != lb.ID || tb.Value != "@" {
		t.Fatalf("textbox %+v", tb)
	}
	if tb.ActiveDescendant != mustNode(t, d, a11y.RoleOption, "alice").ID {
		t.Fatal("active descendant")
	}
}

func TestA11yTransfer(t *testing.T) {
	items := []TransferItem{{Key: "1", Title: "One"}, {Key: "2", Title: "Two"}, {Key: "3", Title: "Three", Disabled: true}, {Key: "4", Title: "Four"}}
	target := []string{"4"}
	d := enDriver(func(c *Context) { Transfer(c, items, &target) })
	d.EnableAccessibility()
	src := mustNode(t, d, a11y.RoleListbox, "Source")
	mustNode(t, d, a11y.RoleListbox, "Target")
	if !src.States.Has(a11y.Multiselectable) || len(src.Children) != 3 {
		t.Fatalf("source %+v", src)
	}
	mustNode(t, d, a11y.RoleButton, "Move to target")
	mustNode(t, d, a11y.RoleButton, "Move to source")
	if len(d.A11yFindAll(a11y.RoleCheckbox, "Select all")) != 2 {
		t.Fatal("select-all checkboxes")
	}
	// Tab: select-all checkbox, then the source list (one stop).
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyTab, 0)
	d.Settle()
	if f := d.A11yFocused(); f == nil || f.Name != "One" || f.Role != a11y.RoleOption {
		t.Fatalf("focus %v", f)
	}
	d.Key(input.KeyDown, 0)
	d.Settle()
	d.Key(input.KeySpace, 0)
	d.Settle()
	two := mustNode(t, d, a11y.RoleOption, "Two")
	if !two.States.Has(a11y.Focused) || !two.States.Has(a11y.Checked|a11y.Selected) {
		t.Fatalf("Two %s", two.States)
	}
	// Down skips the disabled item and wraps.
	d.Key(input.KeyDown, 0)
	d.Settle()
	if f := d.A11yFocused(); f == nil || f.Name != "One" {
		t.Fatalf("after skip: %v", f)
	}
	if err := d.A11yPerform(a11y.RoleButton, "Move to target", a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if len(target) != 2 || target[1] != "2" {
		t.Fatalf("target %v", target)
	}
	if rep := d.AuditNoContrast(); rep.MissingInteractiveNames() != 0 || rep.Count(AuditNotKeyboard) != 0 || rep.Count(AuditNoRole) != 0 {
		t.Fatalf("audit:\n%s", rep)
	}
}

func TestA11yCheckAllMixed(t *testing.T) {
	sel := []string{"a"}
	d := enDriver(func(c *Context) { CheckAll(c, &sel, Opts("a", "b")) })
	d.EnableAccessibility()
	if s := mustNode(t, d, a11y.RoleCheckbox, "Select all").States; !s.Has(a11y.Mixed) || s.Has(a11y.Checked) {
		t.Fatalf("check all %s", s)
	}
	if err := d.A11yPerform(a11y.RoleCheckbox, "Select all", a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if s := mustNode(t, d, a11y.RoleCheckbox, "Select all").States; !s.Has(a11y.Checked) || s.Has(a11y.Mixed) {
		t.Fatalf("after press %s", s)
	}
}
