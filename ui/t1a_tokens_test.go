package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// t1aDriver renders body (English locale) and settles.
func t1aDriver(body func(c *Context)) *Driver {
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Locale(EnUS).Body(func() { Column(c, func() { body(c) }, layout.Gap(16)) })
	}, DriverOptions{Width: 900, Height: 900})
	d.Settle()
	d.Settle()
	return d
}

// t1aRect is the rect of the node labelled label.
func t1aRect(t *testing.T, d *Driver, label string) geom.Rect {
	t.Helper()
	r, ok := d.Find(label)
	if !ok {
		t.Fatalf("node %q not drawn", label)
	}
	return r
}

// t1aFilled reports a fill of col overlapping r.
func t1aFilled(d *Driver, col theme.Color, r geom.Rect) bool {
	for _, o := range d.r.List().Ops {
		if f, ok := o.(paint.FillRect); ok && f.Color == col && f.Rect.Intersect(r).W > 1 && f.Rect.Intersect(r).H > 1 {
			return true
		}
	}
	return false
}

// t1aScoped checks that a token override painted col on the node labelled
// in and not on the node labelled out.
func t1aScoped(t *testing.T, d *Driver, col theme.Color, in, out string) {
	t.Helper()
	if !t1aFilled(d, col, t1aRect(t, d, in)) {
		t.Errorf("override not painted on %q in scope", in)
	}
	if t1aFilled(d, col, t1aRect(t, d, out)) {
		t.Errorf("override leaked to %q outside the scope", out)
	}
}

var t1aCol = theme.Hex("#12ab34")

func TestT1aTreeTokenOverride(t *testing.T) {
	tree := func(c *Context, title string) {
		sel := []string{"k"}
		Tree(c, []TreeNode{{Key: "k", Title: title}, {Key: "o", Title: title + " other"}}).Selected(&sel).Show()
	}
	d := t1aDriver(func(c *Context) {
		ConfigProvider(c).Components(theme.Components{Tree: theme.TreeToken{NodeSelectedBg: t1aCol, TitleHeight: 40}}).Body(func() {
			tree(c, "Scoped")
		})
		tree(c, "Plain")
	})
	t1aScoped(t, d, t1aCol, "Scoped", "Plain")
	if h := t1aRect(t, d, "Scoped").H; !near(h, 40) {
		t.Errorf("scoped title height %v, want 40", h)
	}
	if h := t1aRect(t, d, "Plain").H; !near(h, d.r.c.Theme.Components.Tree.TitleHeight) {
		t.Errorf("plain title height %v", h)
	}
}

func TestT1aVirtualTreeTokenOverride(t *testing.T) {
	tree := func(c *Context, title string) {
		cur := "k"
		VirtualTree(c, []TreeNode{{Key: "k", Title: title}}).Current(&cur).Height(60).Show()
	}
	d := t1aDriver(func(c *Context) {
		ConfigProvider(c).Components(theme.Components{Tree: theme.TreeToken{NodeSelectedBg: t1aCol}}).Body(func() {
			tree(c, "Scoped")
		})
		tree(c, "Plain")
	})
	t1aScoped(t, d, t1aCol, "Scoped", "Plain")
}

func TestT1aTreeSelectTokenOverride(t *testing.T) {
	nodes := func(p string) []TreeSelectNode {
		return []TreeSelectNode{{Label: p + " root", Value: "r", Children: []TreeSelectNode{{Label: p + " leaf", Value: "l"}}}}
	}
	d := t1aDriver(func(c *Context) {
		ConfigProvider(c).Components(theme.Components{Tree: theme.TreeToken{NodeSelectedBg: t1aCol, DropdownNodeHeight: 44}}).Body(func() {
			v := "l"
			TreeSelect(c, &v, nodes("Scoped")).Width(200).DefaultExpandAll().Open()
		})
		v := "l"
		Row(c, func() { TreeSelect(c, &v, nodes("Plain")).Width(200).DefaultExpandAll().Open() }, layout.Margin(geom.Insets{Left: 400}))
	})
	t1aScoped(t, d, t1aCol, "Scoped leaf", "Plain leaf")
	if h := t1aRect(t, d, "Scoped leaf").H; !near(h, 44) {
		t.Errorf("scoped row height %v, want 44", h)
	}
	if h := t1aRect(t, d, "Plain leaf").H; !near(h, d.r.c.Theme.Components.Tree.DropdownNodeHeight) {
		t.Errorf("plain row height %v", h)
	}
}

func TestT1aCascaderTokenOverride(t *testing.T) {
	opts := func(p string) []CascaderOption {
		return []CascaderOption{{Label: p + " A", Value: "a", Children: []CascaderOption{{Label: p + " B", Value: "b"}}}}
	}
	d := t1aDriver(func(c *Context) {
		ConfigProvider(c).Components(theme.Components{Cascader: theme.CascaderToken{OptionSelectedBg: t1aCol, OptionHeight: 44}}).Body(func() {
			v := []string{"a", "b"}
			Cascader(c, &v, opts("Scoped")).Width(200).Open().Expand("a")
		})
		v := []string{"a", "b"}
		Row(c, func() { Cascader(c, &v, opts("Plain")).Width(200).Open().Expand("a") }, layout.Margin(geom.Insets{Left: 400}))
	})
	t1aScoped(t, d, t1aCol, "Scoped A", "Plain A")
	if h := t1aRect(t, d, "Scoped A").H; !near(h, 44) {
		t.Errorf("scoped option height %v, want 44", h)
	}
	if h := t1aRect(t, d, "Plain A").H; !near(h, d.r.c.Theme.Components.Cascader.OptionHeight) {
		t.Errorf("plain option height %v", h)
	}
}

func TestT1aTransferTokenOverride(t *testing.T) {
	items := func(p string) []TransferItem { return []TransferItem{{Key: "1", Title: p + " one"}} }
	d := t1aDriver(func(c *Context) {
		ConfigProvider(c).Components(theme.Components{Transfer: theme.TransferToken{ItemSelectedBg: t1aCol, ItemHeight: 44}}).Body(func() {
			var target []string
			Transfer(c, items("Scoped"), &target).DefaultChecked("1")
		})
		var target []string
		Transfer(c, items("Plain"), &target).DefaultChecked("1")
	})
	t1aScoped(t, d, t1aCol, "Scoped one", "Plain one")
	if h := t1aRect(t, d, "Scoped one").H; !near(h, 44) {
		t.Errorf("scoped item height %v, want 44", h)
	}
	if h := t1aRect(t, d, "Plain one").H; !near(h, d.r.c.Theme.Components.Transfer.ItemHeight) {
		t.Errorf("plain item height %v", h)
	}
}

func TestT1aDropdownTokenOverride(t *testing.T) {
	menu := func(c *Context, p string) {
		open := true
		Dropdown(c, []MenuItem{{Key: "s", Label: p + " sel"}, {Key: "o", Label: p + " other"}}).Trigger(TriggerClick).Open(&open).Selected("s").
			Body(func() { Button(c, p) })
	}
	d := t1aDriver(func(c *Context) {
		Row(c, func() {
			ConfigProvider(c).Components(theme.Components{Dropdown: theme.DropdownToken{ItemSelectedBg: t1aCol, ItemHeight: 44}}).Body(func() {
				menu(c, "Scoped")
			})
			menu(c, "Plain")
		}, layout.Gap(300))
	})
	t1aScoped(t, d, t1aCol, "Scoped sel", "Plain sel")
	if h := t1aRect(t, d, "Scoped sel").H; !near(h, 44) {
		t.Errorf("scoped item height %v, want 44", h)
	}
	if h := t1aRect(t, d, "Plain sel").H; !near(h, 32) {
		t.Errorf("plain item height %v", h)
	}
}

// TestT1aSelectPanelToken: the shared select-like panel (entStylePanel)
// takes its background from the Select token of its scope.
func TestT1aSelectPanelToken(t *testing.T) {
	opts := func(p string) []CascaderOption { return []CascaderOption{{Label: p + " A", Value: "a"}} }
	d := t1aDriver(func(c *Context) {
		ConfigProvider(c).Components(theme.Components{Select: theme.SelectToken{PopupBg: t1aCol}}).Body(func() {
			var v []string
			Cascader(c, &v, opts("Scoped")).Width(200).Open()
		})
		var v []string
		Row(c, func() { Cascader(c, &v, opts("Plain")).Width(200).Open() }, layout.Margin(geom.Insets{Left: 400}))
	})
	t1aScoped(t, d, t1aCol, "Scoped A", "Plain A")
}

// TestT1aTokenDefaults: the derived tokens reproduce the values the
// components painted before they read them.
func TestT1aTokenDefaults(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.AntDesign(theme.Dark()), theme.ElementPlus(), theme.ElementPlus(theme.Dark()), theme.Shadcn(), theme.Shadcn(theme.Dark())} {
		k := th.Components
		el, sh := th.Flavor == theme.Element, th.Flavor == theme.StyleShadcn
		tr, cs, tf, dd := k.Tree, k.Cascader, k.Transfer, k.Dropdown
		switch {
		case el:
			if tr.TitleHeight != 26 || tr.NodeGap != 0 || tr.IndentSize != 18 || tr.NodeSelectedBg != th.Primary.Bg || tr.SwitcherColor != th.TextQuaternary ||
				cs.ColumnWidth != 180 || cs.OptionHeight != 34 || cs.OptionSelectedColor != th.Primary.Base || cs.OptionSelectedBg.A != 0 ||
				tf.ItemHeight != 30 || tf.ListHeight != 280 || tf.HeaderBg != th.FillTertiary || tf.ItemHoverBg.A != 0 ||
				dd.PopupBorderColor != th.BorderSecondary || dd.ItemPaddingInline != 16 || dd.ItemSelectedBg.A != 0 || dd.DangerHoverBg.A != 0 {
				t.Errorf("element dark=%v: %+v %+v %+v %+v", th.Dark, tr, cs, tf, dd)
			}
		case sh:
			if tr.TitleHeight != 28 || tr.NodeGap != 2 || tr.SwitcherSize != 20 || tr.DropdownNodeHeight != shadTreeItemH || tr.NodeBorderRadius != th.Radius ||
				cs.ColumnWidth != 128 || cs.OptionPadding != 8 || tf.ItemPaddingInline != 8 || len(tf.ListShadow) != len(th.CardShadow) ||
				dd.ItemIconColor != th.TextTertiary || dd.IconSize != 16 || dd.DangerHoverBg != th.Error.Bg {
				t.Errorf("shadcn dark=%v: %+v %+v %+v %+v", th.Dark, tr, cs, tf, dd)
			}
		default:
			if tr.TitleHeight != 24 || tr.NodeGap != 4 || tr.DropdownNodeHeight != 28 || tr.NodeSelectedBg != th.ItemSelectedBg || tr.SearchHighlightColor != theme.Hex("#ff5500") ||
				cs.ColumnWidth != 111 || cs.OptionHeight != th.ControlHeight || cs.OptionSelectedFontWeight != 600 ||
				tf.ItemSelectedHoverBg != th.Primary.BgHover || tf.ListBorderRadius != th.RadiusLG ||
				dd.PopupRadius != th.RadiusLG || dd.ItemSelectedBg != th.Primary.Bg || dd.DangerHoverColor != th.TextOnPrimary {
				t.Errorf("antd dark=%v: %+v %+v %+v %+v", th.Dark, tr, cs, tf, dd)
			}
		}
		// The cascader options match the Select options they replaced.
		if cs.OptionHeight != k.Select.OptionHeight {
			t.Errorf("%v dark=%v: cascader option height %v, select %v", th.Flavor, th.Dark, cs.OptionHeight, k.Select.OptionHeight)
		}
	}
}
