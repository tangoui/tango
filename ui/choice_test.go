package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// rectOfID returns the last-frame rectangle of a node id.
func rectOfID(t *testing.T, d *Driver, id ID) geom.Rect {
	t.Helper()
	r, ok := d.r.c.rects[id]
	if !ok {
		t.Fatalf("node %v not laid out", id)
	}
	return r
}

func TestSwitchValueAnyType(t *testing.T) {
	mode := "auto"
	level := 0
	var ids [2]ID
	d := driver(func(c *Context) {
		Column(c, func() {
			ids[0] = SwitchValue(c, &mode, "manual", "auto").A11yLabel("mode").n.id
			ids[1] = SwitchValue(c, &level, 100, 0).A11yLabel("level").n.id
		}, layout.Pad(10), layout.Gap(10))
	})
	d.Frame()
	d.Click(rectOfID(t, d, ids[0]).Center())
	if mode != "manual" {
		t.Fatalf("on value: %q", mode)
	}
	d.Click(rectOfID(t, d, ids[0]).Center())
	if mode != "auto" {
		t.Fatalf("off value: %q", mode)
	}
	d.Click(rectOfID(t, d, ids[1]).Center())
	if level != 100 {
		t.Fatalf("int on value: %d", level)
	}
	// A value that is neither shows the switch off; toggling writes on.
	mode = "other"
	d.EnableAccessibility()
	d.Frame()
	if n := mustNode(t, d, a11y.RoleSwitch, "mode"); n.States.Has(a11y.Checked) {
		t.Fatal("unknown value shown on")
	}
	d.Click(rectOfID(t, d, ids[0]).Center())
	if mode != "manual" {
		t.Fatalf("from unknown: %q", mode)
	}
}

func TestSwitchBeforeChange(t *testing.T) {
	on := false
	allow := false
	asked := 0
	var id ID
	d := driver(func(c *Context) {
		id = Switch(c, &on).BeforeChange(func(next bool) bool {
			asked++
			if !next {
				t.Error("next should be true")
			}
			return allow
		}).n.id
	})
	d.Frame()
	d.Click(rectOfID(t, d, id).Center())
	if on || asked != 1 {
		t.Fatalf("rejected: on=%v asked=%d", on, asked)
	}
	allow = true
	d.Click(rectOfID(t, d, id).Center())
	if !on {
		t.Fatal("accepted toggle not applied")
	}
}

func TestSwitchBeforeChangeAsync(t *testing.T) {
	on := false
	var done func(bool)
	calls, changes := 0, 0
	var id ID
	d := driver(func(c *Context) {
		w := Switch(c, &on).A11yLabel("sync").BeforeChangeAsync(func(next bool, d func(bool)) {
			calls++
			done = d
		})
		id = w.n.id
		if w.Changed() {
			changes++
		}
	})
	d.EnableAccessibility()
	d.Frame()
	d.Click(rectOfID(t, d, id).Center())
	if on || calls != 1 || done == nil {
		t.Fatalf("pending: on=%v calls=%d", on, calls)
	}
	if n := mustNode(t, d, a11y.RoleSwitch, "sync"); !n.States.Has(a11y.Busy) {
		t.Fatalf("pending switch not busy: %s", n)
	}
	// Clicks while pending are ignored.
	d.Click(rectOfID(t, d, id).Center())
	if calls != 1 {
		t.Fatalf("click while pending asked again: %d", calls)
	}
	go done(true) // from another goroutine
	for i := 0; i < 100 && !on; i++ {
		d.Frame()
	}
	if !on || changes != 1 {
		t.Fatalf("accepted: on=%v changes=%d", on, changes)
	}
	if n := mustNode(t, d, a11y.RoleSwitch, "sync"); n.States.Has(a11y.Busy) {
		t.Fatal("still busy after done")
	}
	// Reject: stays on.
	d.Click(rectOfID(t, d, id).Center())
	if calls != 2 {
		t.Fatalf("second ask: %d", calls)
	}
	done(false)
	d.Frame()
	d.Frame()
	if !on {
		t.Fatal("rejected async toggle applied")
	}
	// done called synchronously inside fn applies in the same frame.
	on2 := false
	var id2 ID
	d2 := driver(func(c *Context) {
		id2 = Switch(c, &on2).BeforeChangeAsync(func(next bool, d func(bool)) { d(true) }).n.id
	})
	d2.Frame()
	d2.Click(rectOfID(t, d2, id2).Center())
	if !on2 {
		t.Fatal("synchronous done not applied")
	}
}

func TestSwitchSizesWidthAndTexts(t *testing.T) {
	on := true
	var ids [6]ID
	d := driver(func(c *Context) {
		Column(c, func() {
			ids[0] = Switch(c, &on).n.id
			ids[1] = Switch(c, &on).Large().n.id
			ids[2] = Switch(c, &on).Small().n.id
			ids[3] = Switch(c, &on).Width(80).n.id
			ids[4] = Switch(c, &on).OuterTexts("On", "Off").A11yLabel("outer").n.id
			ids[5] = Switch(c, &on).Icons(icon.Check, icon.Close).Size(Large).n.id
		}, layout.Pad(10), layout.Gap(10))
	})
	d.EnableAccessibility()
	d.Frame()
	tk := theme.AntDesign().Components.Switch
	mid, lg, sm := rectOfID(t, d, ids[0]), rectOfID(t, d, ids[1]), rectOfID(t, d, ids[2])
	if lg.H != tk.TrackHeightLG || lg.W != tk.TrackMinWidthLG || lg.H <= mid.H || sm.H >= mid.H {
		t.Fatalf("sizes: mid %v large %v small %v", mid, lg, sm)
	}
	if w := rectOfID(t, d, ids[3]).W; w != 80 {
		t.Fatalf("width: %v", w)
	}
	outer := rectOfID(t, d, ids[4])
	if outer.W <= mid.W+20 {
		t.Fatalf("outer texts not measured: %v", outer)
	}
	if n := mustNode(t, d, a11y.RoleSwitch, "outer"); n.Value != "On" {
		t.Fatalf("outer value: %q", n.Value)
	}
	// Clicking the off label toggles the switch.
	d.Click(geom.Pt(outer.X+4, outer.Y+outer.H/2))
	if on {
		t.Fatal("label click did not toggle")
	}
	if r := rectOfID(t, d, ids[5]); r.H != tk.TrackHeightLG {
		t.Fatalf("Size(Large): %v", r)
	}
}

func TestSwitchLargeTokens(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn()} {
		k := th.Components.Switch
		if k.TrackHeightLG <= k.TrackHeight || k.TrackMinWidthLG <= k.TrackMinWidth || k.HandleSizeLG <= k.HandleSize || k.HandleSizeLG >= k.TrackHeightLG {
			t.Errorf("%v: large switch tokens %+v", th.Flavor, k)
		}
	}
	if k := theme.ElementPlus().Components.Switch; k.TrackHeightLG != 24 || k.TrackMinWidthLG != 50 || k.HandleSizeLG != 20 {
		t.Errorf("element large: %v %v %v", k.TrackHeightLG, k.TrackMinWidthLG, k.HandleSizeLG)
	}
}

func TestCheckboxGroupMinMax(t *testing.T) {
	sel := []string{"a"}
	d := driver(func(c *Context) {
		CheckboxGroup(c, &sel, Opts("a", "b", "c", "d")).Min(1).Max(2)
	})
	d.Frame()
	d.ClickLabel("a") // the only checked one: cannot be unchecked
	if len(sel) != 1 {
		t.Fatalf("min: %v", sel)
	}
	d.ClickLabel("b")
	if len(sel) != 2 {
		t.Fatalf("add: %v", sel)
	}
	d.ClickLabel("c") // max reached: c is disabled
	if len(sel) != 2 || contains(sel, "c") {
		t.Fatalf("max: %v", sel)
	}
	d.ClickLabel("a") // two checked: a can be unchecked now
	if len(sel) != 1 || sel[0] != "b" {
		t.Fatalf("uncheck above min: %v", sel)
	}
	d.ClickLabel("c")
	if len(sel) != 2 {
		t.Fatalf("below max again: %v", sel)
	}
}

func TestCheckboxGroupDisabledAndLegacy(t *testing.T) {
	sel := []string{}
	legacy := []string{}
	changed := false
	d := driver(func(c *Context) {
		Column(c, func() {
			CheckboxGroup(c, &sel, Opts("x", "y")).Disabled(true).A11yLabel("grp")
			if CheckboxGroup(c, &legacy, Opts("p", "q")).Changed() {
				changed = true
			}
		})
	})
	d.EnableAccessibility()
	d.Frame()
	d.ClickLabel("x")
	if len(sel) != 0 {
		t.Fatalf("disabled group toggled: %v", sel)
	}
	if n := mustNode(t, d, a11y.RoleCheckbox, "y"); !n.States.Has(a11y.Disabled) {
		t.Fatalf("not disabled: %s", n)
	}
	mustNode(t, d, a11y.RoleGroup, "grp")
	d.ClickLabel("q")
	if !changed || len(legacy) != 1 || legacy[0] != "q" {
		t.Fatalf("legacy CheckboxGroup: %v %v", changed, legacy)
	}
}

func TestCheckboxBorderButtonSize(t *testing.T) {
	on, off := true, false
	sel := []string{"B"}
	var ids [4]ID
	d := driver(func(c *Context) {
		Column(c, func() {
			ids[0] = Checkbox(c, "plain", &on).n.id
			ids[1] = Checkbox(c, "framed", &on).Border().n.id
			ids[2] = Checkbox(c, "small", &off).Border().Size(Small).n.id
			ids[3] = Checkbox(c, "btn", &off).Button().n.id
			CheckboxGroup(c, &sel, Opts("A", "B", "C")).Buttons()
		}, layout.Pad(10), layout.Gap(8))
	})
	d.Frame()
	th := theme.AntDesign()
	plain, framed, small, btn := rectOfID(t, d, ids[0]), rectOfID(t, d, ids[1]), rectOfID(t, d, ids[2]), rectOfID(t, d, ids[3])
	if framed.H != th.ControlHeight || small.H != th.ControlHeightSM || btn.H != th.ControlHeight {
		t.Fatalf("heights: framed %v small %v btn %v", framed.H, small.H, btn.H)
	}
	if plain.H != 22 {
		t.Fatalf("plain height changed: %v", plain.H)
	}
	d.ClickLabel("btn")
	if !off {
		t.Fatal("button checkbox did not toggle")
	}
	// Joined buttons overlap by 1px.
	a, b := mustFind(t, d, "A"), mustFind(t, d, "B")
	if b.X != a.Right()-1 {
		t.Fatalf("buttons not joined: %v %v", a, b)
	}
	d.ClickLabel("A")
	d.ClickLabel("B")
	if len(sel) != 1 || sel[0] != "A" {
		t.Fatalf("button group: %v", sel)
	}
}

func TestCheckboxElementSmallBox(t *testing.T) {
	if k := theme.ElementPlus().Components.Checkbox; k.ControlInteractiveSizeSM != 12 {
		t.Fatalf("element small box: %v", k.ControlInteractiveSizeSM)
	}
	if k := theme.AntDesign().Components.Checkbox; k.ControlInteractiveSizeSM != k.ControlInteractiveSize {
		t.Fatal("ant small box should keep the size")
	}
}

func TestRadioGroupDisabledAndBorder(t *testing.T) {
	v, v2 := "A", "A"
	d := driver(func(c *Context) {
		Column(c, func() {
			RadioGroup(c, &v, Opts("A", "B")).Disabled(true).A11yLabel("plain")
			RadioGroup(c, &v2, Opts("X", "Y")).Border().A11yLabel("framed")
			RadioGroup(c, &v, Opts("P", "Q")).Buttons().Disabled(true)
		}, layout.Pad(10), layout.Gap(10))
	})
	d.EnableAccessibility()
	d.Frame()
	d.ClickLabel("B")
	d.ClickLabel("Q")
	if v != "A" {
		t.Fatalf("disabled group changed: %q", v)
	}
	if n := mustNode(t, d, a11y.RoleRadio, "B"); !n.States.Has(a11y.Disabled) {
		t.Fatalf("radio not disabled: %s", n)
	}
	if n := mustNode(t, d, a11y.RoleRadio, "Q"); !n.States.Has(a11y.Disabled) {
		t.Fatalf("button not disabled: %s", n)
	}
	y := mustFind(t, d, "Y")
	if y.H != theme.AntDesign().ControlHeight {
		t.Fatalf("bordered radio height: %v", y.H)
	}
	d.Click(y.Center())
	if v2 != "Y" {
		t.Fatalf("bordered radio: %q", v2)
	}
	// Keyboard: arrows do nothing in a disabled group (no tab stop).
	d.Key(input.KeyTab, 0)
	if f := d.A11yFocused(); f != nil && (f.Name == "A" || f.Name == "B") {
		t.Fatalf("disabled radio focused: %s", f)
	}
}

func TestChoiceFillMirrorCheck(t *testing.T) {
	on, off := true, false
	sel := []string{"A"}
	v := "A"
	tags := []string{"go", "rust"}
	p := func(c *Context) {
		Column(c, func() {
			Checkbox(c, "framed", &on).Border()
			CheckboxGroup(c, &sel, Opts("A", "B", "C")).Buttons()
			RadioGroup(c, &v, Opts("A", "B")).Border()
			Switch(c, &on).OuterTexts("On", "Off").A11yLabel("s1")
			Switch(c, &off).Icons(icon.Check, icon.Close).Large().A11yLabel("s2")
			InputTag(c, &tags).Draggable().Width(300).A11yLabel("tags")
		}, layout.Pad(10), layout.Gap(10))
	}
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn()} {
		d := NewDriver(p, DriverOptions{Width: 600, Height: 500, Theme: th})
		if rep := d.MirrorCheck(p); !rep.OK() {
			t.Errorf("%v: %s", th.Flavor, rep)
		}
	}
}
