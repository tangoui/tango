package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

var (
	r2Mark  = theme.Hex("#12ab34")
	r2Mark2 = theme.Hex("#ab1234")
)

// r2Node returns the node labelled label drawn in the last frame, looking
// in the page and in the overlays.
func r2Node(d *Driver, label string) *node {
	c := d.r.c
	var found *node
	var walk func(n *node)
	walk = func(n *node) {
		if found != nil || n == nil {
			return
		}
		if n.label == label && n.drawn == c.pass {
			found = n
			return
		}
		for _, k := range n.kids {
			walk(k)
		}
	}
	walk(c.root)
	for _, o := range c.overlays {
		walk(o.root)
	}
	return found
}

// r2Fills counts the FillRect / FillPath ops of color col (and, for
// FillRect, top-left radius rad when rad >= 0).
func r2Fills(d *Driver, col theme.Color, rad float32) int {
	n := 0
	for _, op := range d.r.List().Ops {
		switch o := op.(type) {
		case paint.FillRect:
			if o.Color == col && (rad < 0 || o.Radii.TL == rad) {
				n++
			}
		case paint.FillPath:
			if o.Color == col && rad < 0 {
				n++
			}
		}
	}
	return n
}

func r2Shadows(d *Driver, col theme.Color) int {
	n := 0
	for _, op := range d.r.List().Ops {
		if s, ok := op.(paint.ShadowRect); ok && s.Shadow.Color == col {
			n++
		}
	}
	return n
}

func r2HoverText(t *testing.T, d *Driver, s string) {
	t.Helper()
	r, ok := d.FindText(s)
	if !ok {
		t.Fatalf("text %q not found", s)
	}
	d.Move(geom.Pt(r.X+r.W/2, r.Y+r.H/2))
	d.Settle()
}

// TestR2SelectPopupTokens: Select's PopupRadius / PopupShadow style its
// own list and the shared select-like panels (entStylePanel), only inside
// the scope.
func TestR2SelectPopupTokens(t *testing.T) {
	opts := []Option{{Value: "a", Label: "Apple"}, {Value: "b", Label: "Banana"}}
	cs := theme.Components{Select: theme.SelectToken{PopupBg: r2Mark, PopupRadius: 17, PopupShadow: []paint.Shadow{{Y: 2, Blur: 6, Color: r2Mark2}}}}
	d := NewDriver(func(c *Context) {
		Row(c, func() {
			ConfigProvider(c).Components(cs).Body(func() {
				v := "a"
				Select(c, &v, opts).Width(160).Open()
			})
			v := "b"
			Select(c, &v, opts).Width(160).Open()
		}, layout.Gap(40), layout.Pad(20))
	}, DriverOptions{Width: 600, Height: 400})
	d.Settle()
	if n := r2Fills(d, r2Mark, 17); n != 1 {
		t.Fatalf("scoped Select panel with PopupBg/PopupRadius: %d fills, want 1", n)
	}
	if n := r2Shadows(d, r2Mark2); n != 1 {
		t.Fatalf("PopupShadow painted %d times, want 1 (scoped panel only)", n)
	}
	th := theme.AntDesign()
	if n := r2Fills(d, th.Components.Select.PopupBg, th.Components.Select.PopupRadius); n < 1 {
		t.Fatal("unscoped Select panel lost its default look")
	}

	// A Cascader panel (entPanel → entStylePanel) follows the Select token.
	d = NewDriver(func(c *Context) {
		Column(c, func() {
			ConfigProvider(c).Components(cs).Body(func() {
				v := []string{}
				Cascader(c, &v, []CascaderOption{{Value: "z", Label: "Zhe"}}).Width(160)
			})
		}, layout.Pad(20))
	}, DriverOptions{Width: 600, Height: 400})
	d.Settle()
	d.Click(geom.Pt(60, 36))
	d.Settle()
	if n := r2Fills(d, r2Mark, 17); n != 1 {
		t.Fatalf("Cascader panel ignores the Select popup tokens: %d", n)
	}
}

// TestR2SelectPopupDefaults: the derived popup radius / shadow are what
// the panels painted before (RadiusLG; Element and shadcn: Radius).
func TestR2SelectPopupDefaults(t *testing.T) {
	for _, th := range r2Themes() {
		k := th.Components.Select
		want := th.RadiusLG
		if th.Flavor == theme.Element || th.Flavor == theme.StyleShadcn {
			want = th.Radius
		}
		if k.PopupRadius != want || len(k.PopupShadow) != len(th.Shadow) {
			t.Errorf("%v dark=%v: radius %v shadow %v", th.Flavor, th.Dark, k.PopupRadius, k.PopupShadow)
		}
	}
}

func r2Themes() []*theme.Theme {
	var out []*theme.Theme
	for _, dark := range []bool{false, true} {
		var o []theme.Option
		if dark {
			o = append(o, theme.Dark())
		}
		out = append(out, theme.AntDesign(o...), theme.ElementPlus(o...))
		if dark {
			out = append(out, theme.Shadcn(theme.Dark()))
		} else {
			out = append(out, theme.Shadcn())
		}
	}
	return out
}

// TestR2MenuPopupIsolated: a Menu's pop-out submenus read the Menu token,
// so a scoped Dropdown override does not change them while a Menu
// override does.
func TestR2MenuPopupIsolated(t *testing.T) {
	items := []MenuItem{{Key: "sub", Label: "Sub", Children: []MenuItem{{Key: "one", Label: "One"}, {Key: "two", Label: "Two"}}}}
	run := func(cs theme.Components) *Driver {
		d := NewDriver(func(c *Context) {
			ConfigProvider(c).Components(cs).Body(func() {
				Column(c, func() {
					sel := ""
					Menu(c, items, &sel).Mode(MenuHorizontal).Show()
				}, layout.Pad(20))
			})
		}, DriverOptions{Width: 600, Height: 400})
		d.Settle()
		r2HoverText(t, d, "Sub")
		if r2Node(d, "One") == nil {
			t.Fatal("submenu did not open")
		}
		return d
	}
	def := theme.AntDesign().Components.Menu

	d := run(theme.Components{Dropdown: theme.DropdownToken{PopupBg: r2Mark, ItemHeight: 50}})
	if n := r2Fills(d, r2Mark, -1); n != 0 {
		t.Fatalf("Dropdown PopupBg leaked into a Menu submenu (%d fills)", n)
	}
	if h := r2Node(d, "One").abs.H; h != def.PopupItemHeight {
		t.Fatalf("Dropdown ItemHeight leaked into a Menu submenu: %v", h)
	}

	d = run(theme.Components{Menu: theme.MenuToken{PopupBg: r2Mark2, PopupItemHeight: 44}})
	if n := r2Fills(d, r2Mark2, -1); n == 0 {
		t.Fatal("Menu PopupBg not painted on its submenu")
	}
	if h := r2Node(d, "One").abs.H; h != 44 {
		t.Fatalf("Menu PopupItemHeight: %v, want 44", h)
	}
}

// TestR2MenuPopupDefaults: the Menu's Popup* defaults equal the Dropdown
// panel look the submenus painted with before.
func TestR2MenuPopupDefaults(t *testing.T) {
	for _, th := range r2Themes() {
		m, k := th.Components.Menu, th.Components.Dropdown
		p := menuPopupToken(&m)
		p.Alias = k.Alias
		if p.PopupBg != k.PopupBg || p.PopupBorderColor != k.PopupBorderColor || p.PopupRadius != k.PopupRadius ||
			len(p.PopupShadow) != len(k.PopupShadow) || p.PaddingBlock != k.PaddingBlock || p.PaddingInline != k.PaddingInline ||
			p.ItemHeight != k.ItemHeight || p.ItemPaddingInline != k.ItemPaddingInline || p.ItemBorderRadius != k.ItemBorderRadius ||
			p.ItemColor != k.ItemColor || p.ItemHoverBg != k.ItemHoverBg || p.ItemHoverColor != k.ItemHoverColor ||
			p.ItemSelectedBg != k.ItemSelectedBg || p.ItemSelectedColor != k.ItemSelectedColor || p.ItemIconColor != k.ItemIconColor ||
			p.IconSize != k.IconSize || p.DangerColor != k.DangerColor || p.DangerHoverBg != k.DangerHoverBg ||
			p.DangerHoverColor != k.DangerHoverColor || p.SubmenuArrowColor != k.SubmenuArrowColor ||
			p.GroupTitleHeight != k.GroupTitleHeight || p.GroupTitleFontSize != k.GroupTitleFontSize ||
			p.GroupTitleFontWeight != k.GroupTitleFontWeight || p.GroupTitleColor != k.GroupTitleColor || p.DividerColor != k.DividerColor ||
			p.ShortcutColor != k.ShortcutColor || p.ShortcutFontSize != k.ShortcutFontSize ||
			p.ShortcutLetterSpacing != k.ShortcutLetterSpacing || p.ShortcutGap != k.ShortcutGap {
			t.Errorf("%v dark=%v: Menu popup defaults differ from Dropdown", th.Flavor, th.Dark)
		}
	}
}

// r2Handles returns the handles node of the first InputNumber.
func r2Handles(d *Driver) *node {
	var found *node
	var walk func(n *node)
	walk = func(n *node) {
		if found != nil || n == nil {
			return
		}
		if n.parent != nil && n.id == hashID(n.parent.id, "handles") {
			found = n
			return
		}
		for _, k := range n.kids {
			walk(k)
		}
	}
	walk(d.r.c.root)
	return found
}

// TestR2InputNumberHandleArrows: the stacked step arrows of Ant / Element
// sit inside their handles (they were drawn from the step nodes' rects
// before those were laid out for the frame, i.e. at the window origin).
func TestR2InputNumberHandleArrows(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn()} {
		d := NewDriver(func(c *Context) {
			Column(c, func() { v := 3.0; InputNumber(c, &v) }, layout.Pad(40))
		}, DriverOptions{Width: 400, Height: 200, Theme: th})
		d.Settle()
		d.Move(geom.Pt(60, 56))
		d.Settle()
		rec, done := d.recordIcons()
		d.Frame()
		done()
		h := r2Handles(d)
		if h == nil {
			t.Fatal("no handles")
		}
		hr := h.abs
		var up, dn int
		for _, n := range rec.notes {
			if n.name != "up" && n.name != "down" {
				continue
			}
			half := geom.Rect{X: hr.X, Y: hr.Y, W: hr.W, H: hr.H / 2}
			if n.name == "down" {
				half.Y += hr.H / 2
				dn++
			} else {
				up++
			}
			if n.r.X < half.X || n.r.Right() > half.Right() || n.r.Y < half.Y || n.r.Bottom() > half.Bottom() {
				t.Errorf("%v: %s arrow at %v outside its handle %v", th.Flavor, n.name, n.r, half)
			}
		}
		if up != 1 || dn != 1 {
			t.Errorf("%v: %d up / %d down arrows, want 1 each", th.Flavor, up, dn)
		}
	}
}

// TestR2InputNumberShadcnHandleTokens: the shadcn steppers read the
// InputNumber token.
func TestR2InputNumberShadcnHandleTokens(t *testing.T) {
	run := func(cs theme.Components) *Driver {
		d := NewDriver(func(c *Context) {
			ConfigProvider(c).Components(cs).Body(func() {
				Column(c, func() { v := 3.0; InputNumber(c, &v) }, layout.Pad(40))
			})
		}, DriverOptions{Width: 400, Height: 200, Theme: theme.Shadcn()})
		d.Settle()
		h := r2Handles(d)
		d.Move(geom.Pt(h.abs.X+h.abs.W/2, h.abs.Y+h.abs.H/4))
		d.Settle()
		return d
	}
	d := run(theme.Components{InputNumber: theme.InputNumberToken{HandleHoverBg: r2Mark, HandleBorderColor: r2Mark2}})
	if r2Fills(d, r2Mark, -1) != 1 || r2Fills(d, r2Mark2, -1) != 2 {
		t.Fatalf("shadcn handles ignore HandleHoverBg / HandleBorderColor: %d %d", r2Fills(d, r2Mark, -1), r2Fills(d, r2Mark2, -1))
	}
	d = run(theme.Components{})
	if r2Fills(d, r2Mark, -1) != 0 || r2Fills(d, theme.Shadcn().Components.InputNumber.HandleHoverBg, -1) == 0 {
		t.Fatal("default shadcn handle hover changed")
	}
}

// TestR2TimeSelectOptionTokens: TimeSelect options read the Select
// token's option fields.
func TestR2TimeSelectOptionTokens(t *testing.T) {
	run := func(cs theme.Components) *Driver {
		d := NewDriver(func(c *Context) {
			ConfigProvider(c).Components(cs).Body(func() {
				Column(c, func() { v := "09:30"; TimeSelect(c, &v) }, layout.Pad(20))
			})
		}, DriverOptions{Width: 400, Height: 400})
		d.Settle()
		d.Click(geom.Pt(60, 36))
		d.Settle()
		return d
	}
	d := run(theme.Components{Select: theme.SelectToken{OptionSelectedBg: r2Mark, OptionHeight: 40}})
	if r2Fills(d, r2Mark, -1) != 1 {
		t.Fatal("TimeSelect ignores OptionSelectedBg")
	}
	if n := r2Node(d, "10:00"); n == nil || n.abs.H != 40 {
		t.Fatal("TimeSelect ignores OptionHeight")
	}
	d = run(theme.Components{})
	if n := r2Node(d, "10:00"); n == nil || n.abs.H != 32 || r2Fills(d, r2Mark, -1) != 0 {
		t.Fatal("default TimeSelect options changed")
	}
}
