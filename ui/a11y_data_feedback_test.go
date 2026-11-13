package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
)

// Accessibility of the display and feedback components: Badge, Tag,
// Avatar, Divider, Statistic, Tooltip, Alert, Spin, Loading, Popconfirm,
// Confirm, VirtualList, InfiniteScroll and Markdown.

func TestA11yBadgeTagAvatar(t *testing.T) {
	var loc *Locale
	closed := 0
	page := func(c *Context) {
		loc = c.Locale()
		Badge(c).Count(5).Body(func() { Button(c, "Inbox") })
		Badge(c).Count(120).Body(func() { Avatar(c, "") })
		Badge(c).Count(3)
		Badge(c).Status("success", "Online")
		if Tag(c, "Go").Closable().Closed() {
			closed++
		}
		Tag(c, "Plain")
		Avatar(c, "AB").A11yLabel("Alice Brown")
		Divider(c, "Section")
		Statistic(c, "Users", 1234, 0).Suffix("pts")
	}
	d := NewDriver(page, DriverOptions{})
	d.EnableAccessibility()
	if b := mustNode(t, d, a11y.RoleButton, "Inbox"); b.Description != "5" {
		t.Errorf("badge count not on the wrapped button: %s", b)
	}
	if a := mustNode(t, d, a11y.RoleImage, loc.A11y.Avatar); a.Description != "99+" {
		t.Errorf("badge on avatar: %s", a)
	}
	mustNode(t, d, a11y.RoleText, "3")
	mustNode(t, d, a11y.RoleText, "Online")
	mustNode(t, d, a11y.RoleText, "Plain")
	mustNode(t, d, a11y.RoleImage, "Alice Brown")
	if s := mustNode(t, d, a11y.RoleSeparator, "Section"); !s.States.Has(a11y.Horizontal) {
		t.Errorf("divider: %s", s)
	}
	if m := mustNode(t, d, a11y.RoleMeter, "Users"); m.Value != "1,234 pts" {
		t.Errorf("statistic: %s", m)
	}
	// The closable tag is a "remove" button reachable with Tab; Enter and
	// Delete close it.
	rm := mustNode(t, d, a11y.RoleButton, loc.A11y.Remove+" Go")
	for i := 0; i < 4; i++ {
		d.Key(input.KeyTab, 0)
		if f := d.A11yFocused(); f != nil && f.ID == rm.ID {
			break
		}
	}
	if f := d.A11yFocused(); f == nil || f.ID != rm.ID {
		t.Fatalf("tag not reachable with Tab: %v", f)
	}
	d.Key(input.KeyEnter, 0)
	d.Key(input.KeyDelete, 0)
	if closed != 2 {
		t.Errorf("keyboard close count %d, want 2", closed)
	}
	if err := d.PerformAction(rm.ID, a11y.Press, ""); err != nil || closed != 3 {
		t.Errorf("press action: %v closed=%d", err, closed)
	}
}

func TestA11yBadgeProcessingStill(t *testing.T) {
	page := func(c *Context) { Badge(c).Status("processing", "Running") }
	d := NewDriver(page, DriverOptions{Animations: true, ReduceMotion: true})
	d.Settle()
	if d.NeedsFrame() {
		t.Error("processing badge animates under reduced motion")
	}
	d2 := NewDriver(page, DriverOptions{Animations: true})
	d2.Frame()
	if !d2.NeedsFrame() {
		t.Error("processing badge does not animate normally")
	}
}

func TestA11yTooltipAndAlerts(t *testing.T) {
	var loc *Locale
	page := func(c *Context) {
		loc = c.Locale()
		Tooltip(c, "Saves the file", func() { Button(c, "Save") })
		Alert(c, "error", "Failed").Description("Disk full").Closable().Show()
		Alert(c, "info", "Heads up").Show()
		Spin(c).Tip("Fetching")
		Spin(c).Percent(40)
		Loading(c, true).Body(func() { Text(c, "content") })
	}
	d := NewDriver(page, DriverOptions{})
	d.EnableAccessibility()
	save := mustNode(t, d, a11y.RoleButton, "Save")
	if save.Description != "Saves the file" {
		t.Errorf("tooltip trigger description: %s", save)
	}
	if _, ok := d.A11yFind(a11y.RoleTooltip, ""); ok {
		t.Fatal("tooltip shown before hover or focus")
	}
	d.Key(input.KeyTab, 0)
	if f := d.A11yFocused(); f == nil || f.ID != save.ID {
		t.Fatalf("focus %v", f)
	}
	mustNode(t, d, a11y.RoleTooltip, "Saves the file")
	al := mustNode(t, d, a11y.RoleAlert, "Failed")
	if al.Description != "Disk full" || al.Live != a11y.LiveAssertive {
		t.Errorf("alert: %s", al)
	}
	mustNode(t, d, a11y.RoleButton, loc.Close)
	if st := mustNode(t, d, a11y.RoleStatus, "Heads up"); st.Live != a11y.LivePolite {
		t.Errorf("info alert: %s", st)
	}
	if sp := mustNode(t, d, a11y.RoleStatus, "Fetching"); !sp.States.Has(a11y.Busy) {
		t.Errorf("spin: %s", sp)
	}
	if pb := mustNode(t, d, a11y.RoleProgressBar, loc.A11y.Loading); !pb.HasRange || pb.ValueNow != 40 || pb.ValueMax != 100 {
		t.Errorf("spin percent: %s", pb)
	}
	if n := len(d.A11yFindAll(a11y.RoleStatus, loc.A11y.Loading)); n != 1 {
		t.Errorf("loading mask status nodes: %d", n)
	}
}

func TestA11yPopconfirmKeyboard(t *testing.T) {
	var loc *Locale
	confirmed := false
	page := func(c *Context) {
		loc = c.Locale()
		Button(c, "Before")
		if Popconfirm(c, "Delete it?", "This cannot be undone", func() { Button(c, "Delete") }).Confirmed() {
			confirmed = true
		}
	}
	d := NewDriver(page, DriverOptions{})
	d.EnableAccessibility()
	del := mustNode(t, d, a11y.RoleButton, "Delete")
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyTab, 0)
	if f := d.A11yFocused(); f == nil || f.ID != del.ID {
		t.Fatalf("trigger focus %v", f)
	}
	d.Key(input.KeyEnter, 0)
	dlg := mustNode(t, d, a11y.RoleAlertDialog, "Delete it?")
	if dlg.Description != "This cannot be undone" || !dlg.Actions.Has(a11y.Dismiss) {
		t.Errorf("popconfirm: %s %s", dlg, dlg.Actions)
	}
	ok := mustNode(t, d, a11y.RoleButton, loc.OK)
	if f := d.A11yFocused(); f == nil || f.ID != ok.ID {
		t.Fatalf("focus did not move into the bubble: %v", f)
	}
	d.Key(input.KeyEscape, 0)
	if _, open := d.A11yFind(a11y.RoleAlertDialog, ""); open {
		t.Fatal("Escape did not close the popconfirm")
	}
	if f := d.A11yFocused(); f == nil || f.ID != del.ID {
		t.Fatalf("focus not returned to the trigger: %v", f)
	}
	d.Key(input.KeyEnter, 0)
	d.Key(input.KeyEnter, 0) // OK has focus
	if !confirmed {
		t.Error("keyboard confirm failed")
	}
	if f := d.A11yFocused(); f == nil || f.ID != del.ID {
		t.Errorf("focus after confirm: %v", f)
	}
}

func TestA11yConfirmDialog(t *testing.T) {
	var loc *Locale
	cancelled := false
	page := func(c *Context) {
		loc = c.Locale()
		if Button(c, "Open").Clicked() {
			Confirm(c, ConfirmOptions{Title: "Discard?", Content: "Changes will be lost", OnCancel: func() { cancelled = true }})
		}
		Button(c, "Other")
	}
	d := NewDriver(page, DriverOptions{})
	d.EnableAccessibility()
	open := mustNode(t, d, a11y.RoleButton, "Open")
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyEnter, 0)
	dlg := mustNode(t, d, a11y.RoleAlertDialog, "Discard?")
	if !dlg.States.Has(a11y.Modal) || dlg.Description != "Changes will be lost" || !dlg.Actions.Has(a11y.Dismiss) {
		t.Errorf("confirm: %s %s", dlg, dlg.Actions)
	}
	if _, ok := d.A11yFind(a11y.RoleButton, "Other"); ok {
		t.Error("page below the modal box is exposed")
	}
	if f := d.A11yFocused(); f == nil || f.Name != loc.OK {
		t.Fatalf("initial focus %v", f)
	}
	// Tab stays inside the box.
	for i := 0; i < 3; i++ {
		d.Key(input.KeyTab, 0)
		if f := d.A11yFocused(); f == nil || (f.Name != loc.OK && f.Name != loc.Cancel) {
			t.Fatalf("Tab left the box: %v", f)
		}
	}
	if err := d.PerformAction(dlg.ID, a11y.Dismiss, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if !cancelled {
		t.Error("dismiss did not cancel")
	}
	if f := d.A11yFocused(); f == nil || f.ID != open.ID {
		t.Errorf("focus not returned to the opener: %v", f)
	}
}

func TestA11yVirtualListAndMarkdown(t *testing.T) {
	var loc *Locale
	page := func(c *Context) {
		loc = c.Locale()
		VirtualList(c, 1000, 30, func(i int) { Text(c, "row") }).Height(120).A11yLabel("Rows")
		Markdown(c, "# Title\n\nSome *text*.\n\n- one\n- two\n\n- [x] done task\n\n![a cat](cat.png)\n\n| A | B |\n|---|---|\n| 1 | 2 |\n")
		InfiniteScroll(c, func() { Text(c, "item") }).Height(100).Loading(true)
	}
	d := NewDriver(page, DriverOptions{})
	d.EnableAccessibility()
	lst := mustNode(t, d, a11y.RoleList, "Rows")
	if lst.RowCount != 1000 {
		t.Errorf("virtual list count %d", lst.RowCount)
	}
	tr := d.AccessibilityTree()
	k := tr.Node(lst.Children[0])
	if k.Role != a11y.RoleListItem || k.PosInSet != 1 || k.SetSize != 1000 {
		t.Errorf("first row: %s", k)
	}
	if h := mustNode(t, d, a11y.RoleHeading, "Title"); h.Level != 1 {
		t.Errorf("heading: %s", h)
	}
	mustNode(t, d, a11y.RoleParagraph, "Some text.")
	if it := mustNode(t, d, a11y.RoleListItem, "one"); it.SetSize != 3 {
		t.Errorf("markdown list item: %s", it)
	}
	if cb := mustNode(t, d, a11y.RoleCheckbox, "done task"); !cb.States.Has(a11y.Checked) {
		t.Errorf("task: %s", cb)
	}
	mustNode(t, d, a11y.RoleImage, "a cat")
	mustNode(t, d, a11y.RoleTable, "")
	mustNode(t, d, a11y.RoleColumnHeader, "A")
	mustNode(t, d, a11y.RoleCell, "2")
	if st := mustNode(t, d, a11y.RoleStatus, loc.Loading+"..."); !st.States.Has(a11y.Busy) {
		t.Errorf("infinite scroll footer: %s", st)
	}
	rep := d.AuditNoContrast()
	for _, is := range rep.Issues {
		t.Errorf("audit: %+v", is)
	}
}
