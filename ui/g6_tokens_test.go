package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// TestG6TableTokenOverride: a Table token overridden in a ConfigProvider
// scope changes only the tables inside it.
func TestG6TableTokenOverride(t *testing.T) {
	hb := theme.Hex("#f0f5ff")
	cols := func(title string) []TableColumn {
		return []TableColumn{{Key: "a", Title: title, Text: func(int) string { return "x" }}}
	}
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			ConfigProvider(c).Components(theme.Components{Table: theme.TableToken{HeaderBg: hb, CellPaddingBlock: 30}}).Body(func() {
				Table(c, cols("Scoped"), 1).Show()
			})
			Table(c, cols("Plain"), 1).Show()
		}, layout.Gap(16))
	}, DriverOptions{Width: 600, Height: 600})
	d.Settle()
	d.Settle()
	in, out := g6Rect(t, d, "Scoped"), g6Rect(t, d, "Plain")
	hasFill := func(r geom.Rect) bool {
		for _, o := range d.r.List().Ops {
			if f, ok := o.(paint.FillRect); ok && f.Color == hb && f.Rect.Intersect(r).W > 1 {
				return true
			}
		}
		return false
	}
	if !hasFill(in) {
		t.Fatal("HeaderBg override not painted in scope")
	}
	if hasFill(out) {
		t.Fatal("HeaderBg override leaked outside the scope")
	}
	// CellPaddingBlock: the scoped (default-size) rows are 22+2*30+1 tall.
	if r := g6Rect(t, d, "row:0"); !near(r.H, 83) {
		t.Fatalf("scoped row height %v, want 83", r.H)
	}
	plain := d.r.c.Theme.Components.Table
	if r := out; !near(r.H, 22+2*plain.CellPaddingBlock+1) {
		t.Fatalf("plain header height %v", r.H)
	}
}

// TestG6TableTokenDefaults: the derived Table token reproduces the values
// the table painted before it read the token, in every design system.
func TestG6TableTokenDefaults(t *testing.T) {
	for _, mk := range []func(...theme.Option) *theme.Theme{theme.AntDesign, theme.ElementPlus} {
		for _, dark := range []bool{false, true} {
			var o []theme.Option
			if dark {
				o = append(o, theme.Dark())
			}
			th := mk(o...)
			k := th.Components.Table
			el := th.Flavor == theme.Element
			if k.HeaderBg != th.TableHeaderBg || k.RowHoverBg != th.TableRowHoverBg || k.BorderColor != th.BorderSecondary ||
				k.SorterIdleColor != th.TableSorterIdle || k.RowStripedBg != th.FillAlter || k.RowCurrentBg != th.Primary.Bg {
				t.Errorf("%v dark=%v: colors", th.Flavor, dark)
			}
			if el && (k.RowSelectedBg.A != 0 || k.HeaderBorderRadius != 0 || k.CellColor != th.TextSecondary || k.CellPaddingInlineMD != 12) {
				t.Errorf("element dark=%v: %+v", dark, k)
			}
			if !el && (k.RowSelectedBg != th.Primary.Bg || k.HeaderBorderRadius != th.RadiusLG || k.CellColor != th.Text) {
				t.Errorf("antd dark=%v: %+v", dark, k)
			}
		}
	}
	sh := theme.Shadcn().Components.Table
	if sh.CellPaddingBlockSM != 6 || sh.RowSelectedHoverBg != sh.RowSelectedBg {
		t.Errorf("shadcn: %+v", sh)
	}
}
