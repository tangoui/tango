package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
)

func TestA11yRateSlider(t *testing.T) {
	v := 2.0
	d := driver(func(c *Context) { Rate(c, &v).AllowHalf() })
	d.EnableAccessibility()
	loc := ZhCN
	n := mustNode(t, d, a11y.RoleSlider, loc.A11y.Rating)
	if !n.HasRange || n.ValueMax != 5 || n.Step != 0.5 || n.Value != loc.Rating(2, 5) {
		t.Fatalf("rate semantics %+v", n)
	}
	if err := d.PerformAction(n.ID, a11y.Increment, ""); err != nil || v != 2.5 {
		t.Fatalf("increment: %v value %v", err, v)
	}
	if err := d.PerformAction(n.ID, a11y.SetValue, "4"); err != nil || v != 4 {
		t.Fatalf("set value: %v value %v", err, v)
	}
}

func TestA11ySegmentedRadioGroup(t *testing.T) {
	v := "a"
	d := driver(func(c *Context) {
		Segmented(c, &v, []Option{{Label: "A", Value: "a"}, {Label: "B", Value: "b"}, {Value: "icon"}}).A11yLabel("View")
	})
	d.EnableAccessibility()
	g := mustNode(t, d, a11y.RoleRadioGroup, "View")
	a := mustNode(t, d, a11y.RoleRadio, "A")
	if !a.States.Has(a11y.Checked) || g.ActiveDescendant != a.ID || a.SetSize != 3 {
		t.Fatalf("segment A %v group active %v", a, g.ActiveDescendant)
	}
	mustNode(t, d, a11y.RoleRadio, "icon") // icon-only segment named by its value
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyRight, 0)
	d.Settle()
	if v != "b" || !mustNode(t, d, a11y.RoleRadio, "B").States.Has(a11y.Checked) {
		t.Fatalf("arrow key: value %q", v)
	}
}

func TestA11yOTPCells(t *testing.T) {
	v := ""
	d := driver(func(c *Context) { OTP(c, &v, 4).A11yLabel("Code") })
	d.EnableAccessibility()
	mustNode(t, d, a11y.RoleGroup, "Code")
	first := mustNode(t, d, a11y.RoleTextbox, ZhCN.Digit(1, 4))
	mustNode(t, d, a11y.RoleTextbox, ZhCN.Digit(4, 4))
	if err := d.PerformAction(first.ID, a11y.SetValue, "1234"); err != nil || v != "1234" {
		t.Fatalf("set value: %v value %q", err, v)
	}
	if n := mustNode(t, d, a11y.RoleTextbox, ZhCN.Digit(3, 4)); n.Value != "3" {
		t.Fatalf("cell 3 value %q", n.Value)
	}
}

func TestA11yTextAreaSearch(t *testing.T) {
	a, q := "hi", ""
	searched := false
	d := driver(func(c *Context) {
		TextArea(c, &a).Placeholder("Notes").ShowCount().MaxLength(10)
		if Search(c, &q).Searched() {
			searched = true
		}
	})
	d.EnableAccessibility()
	ta := mustNode(t, d, a11y.RoleTextbox, "Notes")
	if !ta.States.Has(a11y.Multiline) || ta.Value != "hi" || ta.Description != "2 / 10" {
		t.Fatalf("textarea %v %q", ta, ta.Description)
	}
	sb := mustNode(t, d, a11y.RoleSearchbox, ZhCN.Search)
	if !sb.Actions.Has(a11y.SetValue) {
		t.Fatalf("searchbox %v", sb)
	}
	if err := d.A11yPerform(a11y.RoleButton, ZhCN.Search, a11y.Press, ""); err != nil || !searched {
		t.Fatalf("search button: %v searched %v", err, searched)
	}
}

func TestA11yPasswordEye(t *testing.T) {
	pw := "secret"
	d := driver(func(c *Context) { Input(c, &pw).Password().A11yLabel("Password") })
	d.EnableAccessibility()
	if n := mustNode(t, d, a11y.RoleTextbox, "Password"); !n.States.Has(a11y.Protected) || n.Value != "" {
		t.Fatalf("password %v", n)
	}
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyTab, 0)
	d.Settle()
	if f := d.A11yFocused(); f == nil || f.Name != ZhCN.A11y.ShowPassword {
		t.Fatalf("second Tab stop %v", f)
	}
	d.Key(input.KeyEnter, 0)
	d.Settle()
	mustNode(t, d, a11y.RoleButton, ZhCN.A11y.HidePassword)
}

func TestA11yInputClearButton(t *testing.T) {
	s := "abc"
	d := driver(func(c *Context) { Input(c, &s).AllowClear().A11yLabel("Name") })
	d.EnableAccessibility()
	if err := d.A11yPerform(a11y.RoleButton, ZhCN.Clear, a11y.Press, ""); err != nil || s != "" {
		t.Fatalf("clear: %v value %q", err, s)
	}
	if _, ok := d.A11yFind(a11y.RoleButton, ZhCN.Clear); ok {
		t.Fatal("clear button shown for an empty input")
	}
}

func TestA11yUploadList(t *testing.T) {
	files := []UploadFile{{Name: "a.txt", Status: "done"}, {Name: "b.txt", Status: "uploading", Percent: 40}}
	cards := []UploadFile{{Name: "c.png", Status: "done"}}
	prev := -1
	d := NewDriver(func(c *Context) {
		Upload(c, &files)
		if i := Upload(c, &cards).PictureCard().Previewed(); i >= 0 {
			prev = i
		}
		Upload(c, &[]UploadFile{}).Dragger()
	}, DriverOptions{Width: 600, Height: 700})
	d.EnableAccessibility()
	mustNode(t, d, a11y.RoleButton, ZhCN.Upload.Button)
	mustNode(t, d, a11y.RoleButton, ZhCN.Upload.Trigger)
	mustNode(t, d, a11y.RoleButton, ZhCN.Upload.DragTitle)
	if b := mustNode(t, d, a11y.RoleListItem, "b.txt"); !b.States.Has(a11y.Busy) || b.ValueNow != 40 {
		t.Fatalf("uploading item %v", b)
	}
	if err := d.A11yPerform(a11y.RoleButton, ZhCN.Upload.Preview, a11y.Press, ""); err != nil || prev != 0 {
		t.Fatalf("preview: %v %d", err, prev)
	}
	if err := d.A11yPerform(a11y.RoleButton, ZhCN.RemoveItem("a.txt"), a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Name != "b.txt" {
		t.Fatalf("files after remove %v", files)
	}
}

func TestA11yCheckableTag(t *testing.T) {
	on := false
	d := driver(func(c *Context) { CheckableTag(c, "News", &on) })
	d.EnableAccessibility()
	n := mustNode(t, d, a11y.RoleButton, "News")
	if !n.States.Has(a11y.Toggleable) || n.States.Has(a11y.Pressed) {
		t.Fatalf("tag %v", n)
	}
	d.PerformAction(n.ID, a11y.Press, "")
	if !on || !mustNode(t, d, a11y.RoleButton, "News").States.Has(a11y.Pressed) {
		t.Fatal("tag not pressed after Press")
	}
}

func TestA11yMarkdownEditorToolbar(t *testing.T) {
	src := ""
	d := NewDriver(func(c *Context) { MarkdownEditor(c, &src) }, DriverOptions{Width: 900, Height: 500})
	d.EnableAccessibility()
	loc := ZhCN.MarkdownEditor
	mustNode(t, d, a11y.RoleToolbar, ZhCN.A11y.Toolbar)
	if n := mustNode(t, d, a11y.RoleTextbox, loc.Placeholder); !n.States.Has(a11y.Multiline) {
		t.Fatalf("editor %v", n)
	}
	d.Key(input.KeyTab, 0)
	d.Settle()
	if f := d.A11yFocused(); f == nil || f.Name != loc.Heading {
		t.Fatalf("first Tab stop %v", f)
	}
	d.Key(input.KeyRight, 0)
	d.Settle()
	if f := d.A11yFocused(); f == nil || f.Name != loc.Bold {
		t.Fatalf("after Right %v", f)
	}
	d.Key(input.KeyTab, 0) // one Tab stop for the whole toolbar
	d.Settle()
	if f := d.A11yFocused(); f == nil || f.Role != a11y.RoleTextbox {
		t.Fatalf("Tab after toolbar %v", f)
	}
	mustNode(t, d, a11y.RoleButton, loc.Split)
}

func TestA11yRichTextLink(t *testing.T) {
	got := ""
	d := driver(func(c *Context) {
		w := RichText(c, Span{Text: "Read "}, Span{Text: "the docs", Link: "https://doc"}, Span{Text: " now."})
		if u := w.ClickedLink(); u != "" {
			got = u
		}
	})
	d.EnableAccessibility()
	d.Settle() // link nodes are placed from the first paint (it requested this frame)
	mustNode(t, d, a11y.RoleParagraph, "Read the docs now.")
	mustNode(t, d, a11y.RoleLink, "the docs")
	d.Key(input.KeyTab, 0)
	d.Settle()
	if f := d.A11yFocused(); f == nil || f.Role != a11y.RoleLink || f.Name != "the docs" {
		t.Fatalf("Tab stop %v", f)
	}
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if got != "https://doc" {
		t.Fatalf("link activation %q", got)
	}
}

func TestA11yQueryFilterAndFormList(t *testing.T) {
	type row struct{ Name string }
	rows := []row{{"a"}, {"b"}}
	d := NewDriver(func(c *Context) {
		QueryFilter(c, []ProField{{Key: "a", Label: "Alpha"}, {Key: "b", Label: "Beta"}, {Key: "c", Label: "Gamma"}, {Key: "d", Label: "Delta"}}).Show()
		f := Form(c)
		FormList(f, "Rows", &rows).Body(func(r *FormListRow[row]) { Input(c, &r.Value.Name).A11yLabel("Row name") })
	}, DriverOptions{Width: 1200, Height: 600})
	d.EnableAccessibility()
	if n := mustNode(t, d, a11y.RoleTextbox, "Alpha"); n.LabelledBy == 0 {
		t.Fatalf("query field %v", n)
	}
	exp := mustNode(t, d, a11y.RoleButton, ZhCN.Expand)
	if !exp.States.Has(a11y.Expandable) || exp.States.Has(a11y.Expanded) {
		t.Fatalf("expand button %v", exp)
	}
	ups := d.A11yFindAll(a11y.RoleButton, ZhCN.Form.MoveUp)
	if len(ups) != 2 || !ups[0].States.Has(a11y.Disabled) || ups[1].States.Has(a11y.Disabled) {
		t.Fatalf("move up buttons %v", ups)
	}
}
