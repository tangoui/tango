package ui

import (
	"github.com/tangoui/tango/ui/theme"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

func mustNode(t *testing.T, d *Driver, role a11y.Role, name string) *a11y.Node {
	t.Helper()
	n, ok := d.A11yFind(role, name)
	if !ok {
		t.Fatalf("no %s %q in tree:\n%s", role, name, d.AccessibilityTree().Dump())
	}
	return n
}

func TestA11yOffByDefault(t *testing.T) {
	d := NewDriver(func(c *Context) { Button(c, "OK") }, DriverOptions{})
	d.Settle()
	if d.AccessibilityTree() != nil {
		t.Fatal("tree built without a host asking for it")
	}
	if n, _ := d.A11yUpdates(); n != 0 {
		t.Fatalf("host notified %d times while disabled", n)
	}
}

func TestA11yTreeRolesAndStates(t *testing.T) {
	on, sw := true, false
	name := ""
	vol := 30.0
	page := func(c *Context) {
		Title(c, "Settings", 2)
		Text(c, "Plain text")
		Button(c, "Save").Primary()
		Button(c, "Busy").Loading(true)
		Button(c, "Off").Disabled(true)
		Checkbox(c, "Agree", &on)
		Switch(c, &sw).A11yLabel("Wi-Fi")
		Slider(c, &vol, 0, 100).A11yLabel("Volume")
		f := Form(c)
		f.Item("Name").Required().Error("Name is required").Body(func() { Input(c, &name).Placeholder("type") })
	}
	d := NewDriver(page, DriverOptions{})
	tr := d.EnableAccessibility()
	if tr == nil || tr.Node(tr.Root).Role != a11y.RoleWindow {
		t.Fatal("no window root")
	}
	if h := mustNode(t, d, a11y.RoleHeading, "Settings"); h.Level != 2 {
		t.Errorf("heading level %d", h.Level)
	}
	mustNode(t, d, a11y.RoleText, "Plain text")
	save := mustNode(t, d, a11y.RoleButton, "Save")
	if !save.States.Has(a11y.Focusable) || !save.Actions.Has(a11y.Press|a11y.Focus) {
		t.Errorf("save: %s actions %s", save, save.Actions)
	}
	if b := mustNode(t, d, a11y.RoleButton, "Busy"); !b.States.Has(a11y.Busy) {
		t.Errorf("busy: %s", b)
	}
	if b := mustNode(t, d, a11y.RoleButton, "Off"); !b.States.Has(a11y.Disabled) || b.Actions.Has(a11y.Press) {
		t.Errorf("disabled: %s %s", b, b.Actions)
	}
	if cb := mustNode(t, d, a11y.RoleCheckbox, "Agree"); !cb.States.Has(a11y.Checkable | a11y.Checked) {
		t.Errorf("checkbox: %s", cb)
	}
	if s := mustNode(t, d, a11y.RoleSwitch, "Wi-Fi"); s.States.Has(a11y.Checked) {
		t.Errorf("switch: %s", s)
	}
	sl := mustNode(t, d, a11y.RoleSlider, "Volume")
	if !sl.HasRange || sl.ValueNow != 30 || sl.ValueMax != 100 || !sl.Actions.Has(a11y.Increment|a11y.SetValue) {
		t.Errorf("slider: %s %s", sl, sl.Actions)
	}
	in := mustNode(t, d, a11y.RoleTextbox, "Name")
	if !in.States.Has(a11y.Required|a11y.Invalid) || in.Description != "Name is required" || in.Placeholder != "type" || in.LabelledBy == 0 {
		t.Errorf("form field: %s labelledBy=%d", in, in.LabelledBy)
	}
	if mustNode(t, d, a11y.RoleForm, "") == nil {
		t.Error("no form")
	}
	// Button text is folded into the name, not a child.
	if len(save.Children) != 0 {
		t.Errorf("button children %v", save.Children)
	}
	// Bounds are window coordinates of the painted widget.
	r, _ := d.Find("Save")
	if save.Bounds.X != r.X || save.Bounds.W != r.W {
		t.Errorf("bounds %+v vs %+v", save.Bounds, r)
	}
}

func TestA11yIncrementalUpdates(t *testing.T) {
	on := false
	page := func(c *Context) {
		Button(c, "A")
		Checkbox(c, "Check", &on)
		Text(c, "static")
	}
	d := NewDriver(page, DriverOptions{})
	tr := d.EnableAccessibility()
	n0, changed := d.A11yUpdates()
	if n0 == 0 || len(changed) != tr.Len() {
		t.Fatalf("first update: %d calls, %d changed of %d", n0, len(changed), tr.Len())
	}
	d.Frame() // nothing changed: no notification
	if n, _ := d.A11yUpdates(); n != n0 {
		t.Fatalf("static frame notified the host (%d → %d)", n0, n)
	}
	cb := mustNode(t, d, a11y.RoleCheckbox, "Check")
	if err := d.PerformAction(cb.ID, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	if !on {
		t.Fatal("press did not toggle the checkbox")
	}
	_, changed = d.A11yUpdates()
	if !slices.Contains(changed, cb.ID) {
		t.Fatalf("checkbox not reported as changed: %v", changed)
	}
	for _, id := range changed {
		if n := d.AccessibilityTree().Node(id); n != nil && n.Role == a11y.RoleText {
			t.Errorf("unchanged text reported: %s", n)
		}
	}
	if !d.AccessibilityTree().Node(cb.ID).States.Has(a11y.Checked) {
		t.Error("checked state not updated")
	}
	if f := d.A11yFocused(); f == nil || f.ID != cb.ID {
		t.Errorf("focus after press: %v", f)
	}
	// Removed nodes are reported and missing from the tree.
	gone := false
	d2 := NewDriver(func(c *Context) {
		if !gone {
			Button(c, "Temp")
		}
		Button(c, "Stay")
	}, DriverOptions{})
	d2.EnableAccessibility()
	temp := mustNode(t, d2, a11y.RoleButton, "Temp")
	gone = true
	d2.Settle()
	_, changed = d2.A11yUpdates()
	if !slices.Contains(changed, temp.ID) || d2.AccessibilityTree().Node(temp.ID) != nil {
		t.Errorf("removal not reported: %v", changed)
	}
}

func TestA11yActions(t *testing.T) {
	vol := 10.0
	num := 3.0
	text := "old"
	sel := "a"
	page := func(c *Context) {
		Slider(c, &vol, 0, 100).A11yLabel("Volume")
		InputNumber(c, &num).A11yLabel("Count")
		Input(c, &text).A11yLabel("Note")
		Select(c, &sel, []Option{{Label: "Apple", Value: "a"}, {Label: "Banana", Value: "b"}}).A11yLabel("Fruit")
	}
	d := NewDriver(page, DriverOptions{})
	d.EnableAccessibility()
	if err := d.A11yPerform(a11y.RoleSlider, "Volume", a11y.Increment, ""); err != nil || vol != 11 {
		t.Fatalf("increment: %v vol=%v", err, vol)
	}
	if err := d.A11yPerform(a11y.RoleSlider, "Volume", a11y.SetValue, "42"); err != nil || vol != 42 {
		t.Fatalf("set slider: %v vol=%v", err, vol)
	}
	if err := d.A11yPerform(a11y.RoleSpinButton, "Count", a11y.Decrement, ""); err != nil || num != 2 {
		t.Fatalf("decrement: %v num=%v", err, num)
	}
	if err := d.A11yPerform(a11y.RoleTextbox, "Note", a11y.SetValue, "new text"); err != nil || text != "new text" {
		t.Fatalf("set text: %v %q", err, text)
	}
	if n := mustNode(t, d, a11y.RoleTextbox, "Note"); n.Value != "new text" {
		t.Errorf("value %q", n.Value)
	}
	cb := mustNode(t, d, a11y.RoleCombobox, "Fruit")
	if cb.Value != "Apple" || !cb.Actions.Has(a11y.Expand) {
		t.Fatalf("combobox: %s %s", cb, cb.Actions)
	}
	if err := d.PerformAction(cb.ID, a11y.Expand, ""); err != nil {
		t.Fatal(err)
	}
	cb = mustNode(t, d, a11y.RoleCombobox, "Fruit")
	if !cb.States.Has(a11y.Expanded) || cb.Controls == 0 || cb.ActiveDescendant == 0 {
		t.Fatalf("expanded combobox: %s controls=%d active=%d", cb, cb.Controls, cb.ActiveDescendant)
	}
	lb := d.AccessibilityTree().Node(cb.Controls)
	if lb == nil || lb.Role != a11y.RoleListbox {
		t.Fatalf("controls %v", lb)
	}
	opt := mustNode(t, d, a11y.RoleOption, "Banana")
	if opt.SetSize != 2 || opt.PosInSet != 2 {
		t.Errorf("option position %d/%d", opt.PosInSet, opt.SetSize)
	}
	if err := d.PerformAction(opt.ID, a11y.Press, ""); err != nil || sel != "b" {
		t.Fatalf("pick option: %v sel=%q", err, sel)
	}
	if err := d.A11yPerform(a11y.RoleButton, "nope", a11y.Press, ""); err == nil {
		t.Error("missing node accepted")
	}
	if err := d.PerformAction(cb.ID, a11y.Increment, ""); err == nil {
		t.Error("unsupported action accepted")
	}
}

func TestA11yModalTrapAndReturn(t *testing.T) {
	open := false
	page := func(c *Context) {
		Button(c, "Before")
		if Button(c, "Open").Clicked() {
			open = true
		}
		Button(c, "After")
		Modal(c, &open, "Dialog").Body(func() {
			Text(c, "Content")
			Button(c, "Inside")
		})
	}
	d := NewDriver(page, DriverOptions{})
	d.EnableAccessibility()
	// Keyboard: Tab to "Open" and activate it.
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyTab, 0)
	if f := d.A11yFocused(); f == nil || f.Name != "Open" {
		t.Fatalf("focus before opening: %v", f)
	}
	opener := d.A11yFocused().ID
	d.Key(input.KeyEnter, 0)
	if !open {
		t.Fatal("modal not opened")
	}
	dlg := mustNode(t, d, a11y.RoleDialog, "Dialog")
	if !dlg.States.Has(a11y.Modal) || !dlg.Actions.Has(a11y.Dismiss) {
		t.Errorf("dialog: %s %s", dlg, dlg.Actions)
	}
	if _, ok := d.A11yFind(a11y.RoleButton, "Before"); ok {
		t.Error("page under the modal is exposed")
	}
	f := d.A11yFocused()
	if f == nil || d.AccessibilityTree().Ancestor(f.ID, a11y.RoleDialog) == nil {
		t.Fatalf("focus did not move into the dialog: %v", f)
	}
	// Tab cycles inside the dialog only.
	seen := map[string]bool{}
	for i := 0; i < 8; i++ {
		d.Key(input.KeyTab, 0)
		f := d.A11yFocused()
		if f == nil || d.AccessibilityTree().Ancestor(f.ID, a11y.RoleDialog) == nil {
			t.Fatalf("Tab left the dialog: %v", f)
		}
		seen[f.Name] = true
	}
	if !seen["Inside"] || !seen[ZhCN.OK] {
		t.Errorf("Tab stops in dialog: %v", seen)
	}
	// Escape closes it and focus returns to the opener.
	d.Key(input.KeyEscape, 0)
	if open {
		t.Fatal("Escape did not close the modal")
	}
	d.Settle()
	if f := d.A11yFocused(); f == nil || f.ID != opener {
		t.Fatalf("focus not returned to the opener: %v", f)
	}
	// Dismiss action closes it too.
	open = true
	d.Settle()
	dlg = mustNode(t, d, a11y.RoleDialog, "Dialog")
	if err := d.PerformAction(dlg.ID, a11y.Dismiss, ""); err != nil || open {
		t.Fatalf("dismiss: %v open=%v", err, open)
	}
}

func TestA11yRadioGroupArrows(t *testing.T) {
	for _, buttons := range []bool{false, true} {
		v := "b"
		page := func(c *Context) {
			Button(c, "First")
			g := RadioGroup(c, &v, []Option{{Label: "A", Value: "a"}, {Label: "B", Value: "b"}, {Label: "C", Value: "c", Disabled: true}, {Label: "D", Value: "d"}})
			if buttons {
				g.Buttons()
			}
			g.Changed()
			Button(c, "Last")
		}
		d := NewDriver(page, DriverOptions{})
		d.EnableAccessibility()
		d.Key(input.KeyTab, 0)
		d.Key(input.KeyTab, 0)
		if f := d.A11yFocused(); f == nil || f.Name != "B" || f.Role != a11y.RoleRadio {
			t.Fatalf("buttons=%v: Tab should land on the checked radio, got %v", buttons, f)
		}
		d.Key(input.KeyDown, 0)
		if v != "d" { // C is disabled
			t.Fatalf("buttons=%v: ArrowDown selected %q", buttons, v)
		}
		d.Key(input.KeyRight, 0)
		if v != "a" {
			t.Fatalf("buttons=%v: wrap selected %q", buttons, v)
		}
		if f := d.A11yFocused(); f == nil || f.Name != "A" || !f.States.Has(a11y.Checked) {
			t.Fatalf("buttons=%v: focus %v", buttons, f)
		}
		d.Key(input.KeyTab, 0) // one Tab stop for the group
		if f := d.A11yFocused(); f == nil || f.Name != "Last" {
			t.Fatalf("buttons=%v: Tab after group: %v", buttons, f)
		}
		g := mustNode(t, d, a11y.RoleRadioGroup, "")
		if len(g.Children) != 4 {
			t.Errorf("radiogroup children %d", len(g.Children))
		}
	}
}

func TestA11yTabsArrows(t *testing.T) {
	for _, shad := range []bool{false, true} {
		act := "1"
		items := []TabItem{{Key: "1", Label: "One"}, {Key: "2", Label: "Two", Disabled: true}, {Key: "3", Label: "Three"}}
		page := func(c *Context) {
			tb := Tabs(c, &act, items).Show()
			tb.Panel(func() { Text(c, "Panel "+act) })
		}
		d := NewDriver(page, DriverOptions{})
		if shad {
			d.SetTheme(theme.Shadcn())
		}
		d.EnableAccessibility()
		d.Key(input.KeyTab, 0)
		if f := d.A11yFocused(); f == nil || f.Role != a11y.RoleTab || f.Name != "One" {
			t.Fatalf("shad=%v focus %v", shad, f)
		}
		d.Key(input.KeyRight, 0)
		if act != "3" {
			t.Fatalf("shad=%v: Right selected %q", shad, act)
		}
		tab := mustNode(t, d, a11y.RoleTab, "Three")
		if !tab.States.Has(a11y.Selected) || tab.SetSize != 3 || tab.PosInSet != 3 {
			t.Errorf("tab: %s", tab)
		}
		p := d.AccessibilityTree().Node(tab.Controls)
		if p == nil || p.Role != a11y.RoleTabPanel || p.Name != "Three" {
			t.Errorf("panel: %v", p)
		}
		d.Key(input.KeyHome, 0)
		if act != "1" {
			t.Errorf("Home selected %q", act)
		}
		mustNode(t, d, a11y.RoleTabList, "")
	}
}

func TestA11yLiveMessage(t *testing.T) {
	fired := false
	page := func(c *Context) {
		if !fired {
			fired = true
			Message(c, "error", "Upload failed")
			Message(c, "success", "Saved")
		}
	}
	d := NewDriver(page, DriverOptions{})
	d.EnableAccessibility()
	d.Settle()
	if n := mustNode(t, d, a11y.RoleAlert, "Upload failed"); n.Live != a11y.LiveAssertive {
		t.Errorf("error message: %s", n)
	}
	if n := mustNode(t, d, a11y.RoleStatus, "Saved"); n.Live != a11y.LivePolite {
		t.Errorf("success message: %s", n)
	}
}

func TestA11yAnnounceAndSemantics(t *testing.T) {
	ann := false
	page := func(c *Context) {
		b := Custom(c, nil, nil)
		b.n.ln.Width, b.n.ln.Height = 20, 20
		c.Semantics(b.ID(), a11y.RoleImage, "Logo")
		Div(c).Semantics(a11y.RoleGroup, "Box").A11y(func(s *a11y.Node) { s.Description = "described" })
		if ann {
			c.Announce("3 results", a11y.LivePolite)
		}
	}
	d := NewDriver(page, DriverOptions{})
	d.EnableAccessibility()
	mustNode(t, d, a11y.RoleImage, "Logo")
	if g := mustNode(t, d, a11y.RoleGroup, "Box"); g.Description != "described" {
		t.Errorf("filler: %s", g)
	}
	ann = true
	d.Frame()
	if a := d.AccessibilityTree().Announcements; len(a) != 1 || a[0].Text != "3 results" {
		t.Errorf("announcements %v", a)
	}
}

func TestReduceMotion(t *testing.T) {
	var v float32
	target := float32(0)
	page := func(c *Context) { v = c.Anim(1, "x", target, 300*time.Millisecond) }
	for _, reduce := range []bool{false, true} {
		target = 0
		d := NewDriver(page, DriverOptions{Animations: true, ReduceMotion: reduce})
		d.Frame()
		target = 1
		d.Frame()
		if reduce && v != 1 {
			t.Errorf("reduce motion: value %v, want end value", v)
		}
		if !reduce && v == 1 {
			t.Errorf("animations: value jumped to the end")
		}
		if reduce && d.Runner().Animating() {
			t.Error("reduce motion still requests animation frames")
		}
	}
	// SetReduceMotion at runtime.
	d := NewDriver(page, DriverOptions{Animations: true})
	target = 0
	d.Frame()
	d.Runner().SetReduceMotion(true)
	if !d.Runner().Context().ReduceMotion() {
		t.Fatal("SetReduceMotion not applied")
	}
	target = 1
	d.Frame()
	if v != 1 {
		t.Errorf("runtime reduce motion: %v", v)
	}
}

func TestA11yAuditFindsProblems(t *testing.T) {
	page := func(c *Context) {
		Button(c, "").Icon(nil)              // icon-only button without a name
		Div(c).Clicked()                     // clickable box without role or keyboard access
		Switch(c, new(bool)).A11yLabel("ok") // fine
	}
	d := NewDriver(page, DriverOptions{})
	rep := d.Audit()
	if rep.MissingInteractiveNames() != 2 { // the icon button and the unannotated box
		t.Errorf("missing names: %d\n%s", rep.MissingInteractiveNames(), rep)
	}
	if rep.Count(AuditNoRole) != 1 || rep.Count(AuditNotKeyboard) != 1 {
		t.Errorf("no-role %d not-keyboard %d\n%s", rep.Count(AuditNoRole), rep.Count(AuditNotKeyboard), rep)
	}
	if rep.Count(AuditContrast) == 0 {
		t.Error("contrast findings of the Ant theme expected")
	}
	if !strings.Contains(rep.String(), "missing-name") {
		t.Error(rep.String())
	}
}

func TestA11yFocusReturnsToAnchor(t *testing.T) {
	open := false
	page := func(c *Context) {
		b := Button(c, "Menu")
		if b.Clicked() {
			open = true
		}
		Button(c, "Next")
		if open {
			ov := Overlay(c, OverlayOptions{Anchor: b.ID(), Place: BottomStart}, func() {
				if Button(c, "Item").Clicked() {
					open = false
				}
			})
			if c.Dismissed(ov.ID(), b.ID()) {
				open = false
			}
		}
	}
	d := NewDriver(page, DriverOptions{})
	d.EnableAccessibility()
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyEnter, 0)
	if !open {
		t.Fatal("popup not opened")
	}
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyTab, 0)
	if f := d.A11yFocused(); f == nil || f.Name != "Item" {
		t.Fatalf("focus in popup: %v", f)
	}
	d.Key(input.KeyEscape, 0)
	if open {
		t.Fatal("Escape did not close the popup")
	}
	if f := d.A11yFocused(); f == nil || f.Name != "Menu" {
		t.Fatalf("focus after Escape: %v, want the trigger", f)
	}
}

func TestA11ySelectKeyboard(t *testing.T) {
	v := "a"
	page := func(c *Context) {
		Select(c, &v, []Option{{Label: "Apple", Value: "a"}, {Label: "Banana", Value: "b"}}).A11yLabel("Fruit")
	}
	d := NewDriver(page, DriverOptions{})
	d.EnableAccessibility()
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyDown, 0) // opens
	cb := mustNode(t, d, a11y.RoleCombobox, "Fruit")
	if !cb.States.Has(a11y.Expanded|a11y.Focused) || d.AccessibilityTree().Node(cb.ActiveDescendant).Name != "Apple" {
		t.Fatalf("open combobox: %s", cb)
	}
	d.Key(input.KeyDown, 0)
	cb = mustNode(t, d, a11y.RoleCombobox, "Fruit")
	if d.AccessibilityTree().Node(cb.ActiveDescendant).Name != "Banana" {
		t.Fatalf("active option after Down: %d", cb.ActiveDescendant)
	}
	d.Key(input.KeyEnter, 0)
	if v != "b" {
		t.Fatalf("Enter picked %q", v)
	}
	if f := d.A11yFocused(); f == nil || f.Role != a11y.RoleCombobox || f.States.Has(a11y.Expanded) {
		t.Fatalf("focus after picking: %v", f)
	}
}

// Nodes entirely outside a clipping or scrolling ancestor are Offscreen
// even when they are inside the window; partly visible ones are not, and
// keep their full bounds.
func TestA11yOffscreenClippedByAncestor(t *testing.T) {
	page := func(c *Context) {
		Div(c, layout.Size(200, 100)).Clip().Body(func() {
			for _, s := range []string{"In", "Half", "Out"} {
				Div(c, layout.Size(200, 60)).Semantics(a11y.RoleButton, s)
			}
		})
		Scroll(c, func() {
			for _, s := range []string{"S1", "S2", "S3"} {
				Div(c, layout.Size(200, 80)).Semantics(a11y.RoleButton, s)
			}
		}, layout.H(100))
	}
	d := NewDriver(page, DriverOptions{Width: 400, Height: 600, Accessibility: true})
	d.Settle()
	for name, off := range map[string]bool{"In": false, "Half": false, "Out": true, "S1": false, "S2": false, "S3": true} {
		if n := mustNode(t, d, a11y.RoleButton, name); n.States.Has(a11y.Offscreen) != off {
			t.Errorf("%s at %+v: offscreen %v, want %v", name, n.Bounds, !off, off)
		}
	}
	if n := mustNode(t, d, a11y.RoleButton, "Half"); n.Bounds.H != 60 {
		t.Errorf("partly visible node bounds clipped: %+v", n.Bounds)
	}
	// Showing S3 scrolls it in and the first one out.
	if err := d.A11yPerform(a11y.RoleButton, "S3", a11y.ScrollIntoView, ""); err != nil {
		t.Fatal(err)
	}
	if n := mustNode(t, d, a11y.RoleButton, "S3"); n.States.Has(a11y.Offscreen) {
		t.Errorf("S3 still offscreen at %+v", n.Bounds)
	}
	if n := mustNode(t, d, a11y.RoleButton, "S1"); !n.States.Has(a11y.Offscreen) {
		t.Errorf("S1 not offscreen at %+v", n.Bounds)
	}
}
