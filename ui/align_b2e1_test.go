package ui

// Spec alignment batch 2, group E1 (Table appearance): the drawing reads
// the Table token as recorded in docs/specs/align/2-table-look.md.

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// b2e1Fills returns the FillRect ops painted in color col.
func b2e1Fills(d *Driver, col theme.Color) []paint.FillRect {
	var out []paint.FillRect
	for _, o := range d.r.List().Ops {
		if f, ok := o.(paint.FillRect); ok && f.Color == col {
			out = append(out, f)
		}
	}
	return out
}

func b2e1Cols() []TableColumn {
	txt := func(int) string { return "x" }
	return []TableColumn{
		{Key: "name", Title: "Name", Width: 100, Text: txt},
		{Key: "g", Title: "Group", Children: []TableColumn{
			{Key: "age", Title: "Age", Width: 100, Sortable: true, Text: txt},
			{Key: "city", Title: "City", Width: 100, Text: txt},
		}},
		{Key: "addr", Title: "Addr", Text: txt},
	}
}

func b2e1Driver(tk theme.TableToken, page func(c *Context)) *Driver {
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Components(theme.Components{Table: tk}).Body(func() { page(c) })
	}, DriverOptions{Width: 700, Height: 500})
	d.Settle()
	d.Settle()
	return d
}

// TestAlignB2E1HeaderSplitLeafOnly: antd draws the short header split
// only on leaf cells that are not the last of their header row
// (th:not(:last-child):not([colspan])::before), and a group cell above the
// last header row has no bottom border unless bordered.
func TestAlignB2E1HeaderSplitLeafOnly(t *testing.T) {
	split, line := theme.Hex("#ff00aa"), theme.Hex("#00aaff")
	d := b2e1Driver(theme.TableToken{HeaderSplitColor: split, BorderColor: line}, func(c *Context) {
		Table(c, b2e1Cols(), 1).Show()
	})
	// Name (row 0, before Group) and Age (row 1, before City).
	if n := len(b2e1Fills(d, split)); n != 2 {
		t.Fatalf("header splits = %d, want 2 (Name, Age)", n)
	}
	g := g6Rect(t, d, "Group")
	for _, f := range b2e1Fills(d, line) {
		if f.Rect.H == 1 && near(f.Rect.Y, g.Bottom()-1) && f.Rect.Intersect(g).W > 1 {
			t.Fatalf("group header cell draws a bottom border at %v", f.Rect)
		}
	}
	// Element / shadcn: no split (token zero).
	el := theme.ElementPlus().Components.Table
	if el.HeaderSplitColor.A != 0 {
		t.Fatal("Element header split should be zero")
	}
}

// TestAlignB2E1BorderedGroupBottom: with Bordered every header cell keeps
// its bottom border.
func TestAlignB2E1BorderedGroupBottom(t *testing.T) {
	line := theme.Hex("#00aaff")
	d := b2e1Driver(theme.TableToken{BorderColor: line}, func(c *Context) {
		Table(c, b2e1Cols(), 1).Bordered().Show()
	})
	g := g6Rect(t, d, "Group")
	found := false
	for _, f := range b2e1Fills(d, line) {
		if f.Rect.H == 1 && near(f.Rect.Y, g.Bottom()-1) && f.Rect.Intersect(g).W > 1 {
			found = true
		}
	}
	if !found {
		t.Fatal("bordered group header cell lost its bottom border")
	}
}

// TestAlignB2E1SortHeaderBg: the sorted header uses HeaderSortActiveBg,
// a hovered sortable header HeaderSortHoverBg (antd sorter.js).
func TestAlignB2E1SortHeaderBg(t *testing.T) {
	act, hov := theme.Hex("#aa0000"), theme.Hex("#00aa00")
	st := SortState{Column: "a", Order: SortAsc}
	txt := func(int) string { return "x" }
	cols := []TableColumn{
		{Key: "a", Title: "A", Width: 100, Sortable: true, Text: txt},
		{Key: "b", Title: "B", Width: 100, Sortable: true, Text: txt},
	}
	d := b2e1Driver(theme.TableToken{HeaderSortActiveBg: act, HeaderSortHoverBg: hov}, func(c *Context) {
		Table(c, cols, 1).Sort(&st).Show()
	})
	if len(b2e1Fills(d, act)) == 0 {
		t.Fatal("sorted header not tinted with HeaderSortActiveBg")
	}
	if len(b2e1Fills(d, hov)) != 0 {
		t.Fatal("hover tint without hover")
	}
	d.Move(g6Rect(t, d, "B").Center())
	d.Settle()
	if len(b2e1Fills(d, hov)) == 0 {
		t.Fatal("hovered sortable header not tinted with HeaderSortHoverBg")
	}
}

// TestAlignB2E1ExpandedPanelBg: the expanded panel is tinted with
// RowExpandedBg (antd colorFillAlter); Element's is not.
func TestAlignB2E1ExpandedPanelBg(t *testing.T) {
	bg := theme.Hex("#123456")
	open := []string{"0"}
	cols := []TableColumn{{Key: "a", Title: "A", Text: func(int) string { return "x" }}}
	d := b2e1Driver(theme.TableToken{RowExpandedBg: bg}, func(c *Context) {
		Table(c, cols, 1).ExpandedKeys(&open).Expandable(func(int) { Text(c, "detail") }).Show()
	})
	if len(b2e1Fills(d, bg)) == 0 {
		t.Fatal("expanded panel not tinted with RowExpandedBg")
	}
	if theme.ElementPlus().Components.Table.RowExpandedBg.A != 0 {
		t.Fatal("Element expanded cell is fill-color-blank")
	}
}

// TestAlignB2E1FilterTriggerHover: the filter trigger paints
// FilterHoverBg with FilterRadius on hover (antd filter.js).
func TestAlignB2E1FilterTriggerHover(t *testing.T) {
	bg := theme.Hex("#654321")
	cols := []TableColumn{{Key: "a", Title: "A", Width: 160, Filters: []Option{{Label: "x", Value: "x"}}, Text: func(int) string { return "x" }}}
	d := b2e1Driver(theme.TableToken{FilterHoverBg: bg, FilterRadius: 6}, func(c *Context) {
		Table(c, cols, 1).Show()
	})
	if len(b2e1Fills(d, bg)) != 0 {
		t.Fatal("filter hover bg without hover")
	}
	trig := g6Rect(t, d, "a") // the filter-trigger node is labelled with the column key
	d.Move(trig.Center())
	d.Settle()
	fs := b2e1Fills(d, bg)
	if len(fs) == 0 || fs[0].Radii.TL != 6 {
		t.Fatalf("filter hover fills %v, want FilterHoverBg with radius 6", fs)
	}
}

// TestAlignB2E1ExpandIconScaled: the expand icon box is the scaled token
// size (antd expand.js 17 × 16/17) with the scaled borderRadius.
func TestAlignB2E1ExpandIconScaled(t *testing.T) {
	tk := theme.AntDesign().Components.Table
	if s := tblExpandSize(&tk); !near(s, 16) {
		t.Fatalf("Ant expand icon drawn at %v, want 16", s)
	}
	l := &paint.List{}
	tblDrawExpand(&tk, l, geom.Rect{W: 17, H: 17}, 0, false, false)
	var box paint.FillRect
	for _, o := range l.Ops {
		if f, ok := o.(paint.FillRect); ok && f.Color == tk.ExpandIconBg {
			box = f
		}
	}
	if !near(box.Rect.W, 16) || !near(box.Radii.TL, 6*16.0/17) {
		t.Fatalf("expand box %v radius %v", box.Rect, box.Radii.TL)
	}
}

// TestAlignB2E1SorterTooltip: the header tooltip names the next click's
// direction from SortDirections (E2's sorterTip) and ShowSorterTooltip(false)
// hides it.
func TestAlignB2E1SorterTooltip(t *testing.T) {
	for _, show := range []bool{true, false} {
		var tb *TableW
		cols := []TableColumn{{Key: "a", Title: "A", Width: 120, Sortable: true, SortDirections: []SortOrder{SortDesc}, Text: func(int) string { return "x" }}}
		d := b2e1Driver(theme.TableToken{}, func(c *Context) {
			tb = Table(c, cols, 1).ShowSorterTooltip(show)
			tb.Show()
		})
		d.Move(g6Rect(t, d, "A").Center())
		for i := 0; i < 4; i++ {
			d.Settle()
		}
		loc := d.r.c.Locale()
		_, got := d.FindText(loc.SortDesc)
		if got != show {
			t.Fatalf("show=%v: descending tooltip shown %v", show, got)
		}
		if _, ok := d.FindText(loc.SortAsc); ok {
			t.Fatal("tooltip names ascending though SortDirections is [descend]")
		}
	}
}

// TestAlignB2E1FilterResetToDefault: the dropdown's reset button applies
// DefaultFilter when FilterResetToDefault is set (E2's filterResetValues).
func TestAlignB2E1FilterResetToDefault(t *testing.T) {
	vals := []string{"x", "y"}
	for _, toDef := range []bool{true, false} {
		var tb *TableW
		cols := []TableColumn{{Key: "a", Title: "A", Width: 160,
			Filters: []Option{{Label: "X", Value: "x"}, {Label: "Y", Value: "y"}}, DefaultFilter: []string{"x"}, FilterResetToDefault: toDef,
			OnFilter: func(i int, v string) bool { return vals[i] == v }, Text: func(i int) string { return vals[i] }}}
		d := b2e1Driver(theme.TableToken{}, func(c *Context) {
			tb = Table(c, cols, 2).Show()
		})
		if n := len(tb.DisplayedRows()); n != 1 {
			t.Fatalf("default filter shows %d rows, want 1", n)
		}
		d.Click(g6Rect(t, d, "a").Center())
		d.Settle()
		loc := d.r.c.Locale()
		if err := d.ClickLabel(loc.FilterReset); err != nil {
			t.Fatal(err)
		}
		d.Settle()
		d.Settle()
		want := 2
		if toDef {
			want = 1
		}
		if n := len(tb.DisplayedRows()); n != want {
			t.Fatalf("toDefault=%v: %d rows after reset, want %d", toDef, n, want)
		}
	}
}

// TestAlignB2E1SelectAllCentred: the selection cells use the token's inline
// padding (antd selection.js paddingXS 8, EP 14px) with the checkbox
// centred, so the header select-all checkbox is hit at the column centre
// in every theme, bordered or not.
func TestAlignB2E1SelectAllCentred(t *testing.T) {
	themes := []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn()}
	for _, th := range themes {
		for _, bordered := range []bool{false, true} {
			var keys []string
			d := NewDriver(func(c *Context) {
				cols := []TableColumn{{Key: "n", Title: "N", Text: func(i int) string { return "x" }}}
				tb := Table(c, cols, 3).RowKey(func(i int) string { return string(rune('a' + i)) }).SelectedKeys(&keys)
				if bordered {
					tb.Bordered()
				}
				tb.Show()
			}, DriverOptions{Width: 600, Height: 400, Theme: th})
			d.Settle()
			d.Settle()
			row := mustFind(t, d, "row:a")
			w := th.Components.Table.SelectionColumnWidth
			if bordered && th.Flavor != theme.Element {
				w += 16 // antd selection.js: bordered + 2 × paddingXS
			}
			// Body checkbox centred in its cell.
			d.Click(geom.Pt(row.X+w/2, row.Y+row.H/2))
			if len(keys) != 1 {
				t.Fatalf("%v bordered=%v: body checkbox at the column centre not hit: %v", th.Flavor, bordered, keys)
			}
			// Header select-all at the column centre.
			d.Click(geom.Pt(row.X+w/2, row.Y-row.H/2)) // header row above the first row
			if len(keys) != 3 {
				t.Fatalf("%v bordered=%v: select-all at the column centre: %v", th.Flavor, bordered, keys)
			}
		}
	}
}
