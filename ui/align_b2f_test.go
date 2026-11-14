package ui

// Spec alignment batch 2, group F (trees, lists, cards, collapse): Tree,
// VirtualTree, List, Listy, VirtualList, Card, Descriptions and Collapse
// against antd 6.7 / Element Plus 2.14.8 / shadcn v4
// (docs/specs/align/2-data.md).

import (
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

func b2fItems() []CollapseItem {
	return []CollapseItem{
		{Key: "a", Label: "Alpha", Body: func() {}},
		{Key: "b", Label: "Beta", Body: func() {}},
	}
}

// b2fText returns the rectangle of the smallest node labelled label (the
// text itself rather than a container named after it).
func b2fText(t *testing.T, d *Driver, label string) geom.Rect {
	t.Helper()
	var best geom.Rect
	found := false
	b2fWalk(d.r.c.root, func(n *node) {
		if n.label == label && (!found || n.abs.W*n.abs.H < best.W*best.H) {
			best, found = n.abs, true
		}
	})
	if !found {
		t.Fatalf("text %q not found", label)
	}
	return best
}

func b2fCenter(t *testing.T, d *Driver, label string) geom.Point {
	t.Helper()
	return b2fText(t, d, label).Center()
}

// ---------------------------------------------------------------------------
// Collapse

// TestCollapseExpandIconPlacement: antd expandIconPlacement "end" (and
// the deprecated expandIconPosition key) puts the arrow after the label.
func TestCollapseExpandIconPlacement(t *testing.T) {
	for _, key := range []string{"expandIconPlacement", "expandIconPosition"} {
		var w *CollapseW
		d := NewDriver(func(c *Context) {
			ConfigProvider(c).Defaults("Collapse", Props{key: "end"}).Body(func() {
				w = Collapse(c, nil, b2fItems()).Show()
			})
		}, DriverOptions{Width: 400, Height: 300})
		d.Settle()
		if !w.iconEnd {
			t.Fatalf("%s=end: arrow not at the end", key)
		}
	}
	var end geom.Rect
	d := NewDriver(func(c *Context) {
		Collapse(c, nil, []CollapseItem{{Key: "a", Label: "Alpha"}}).ExpandIconPlacement("end").Show()
	}, DriverOptions{Width: 400, Height: 300})
	d.Settle()
	lab, _ := d.FindText("Alpha")
	b2fWalk(d.r.c.root, func(n *node) {
		if n.label == "" && n.ln.Width == 12 && n.ln.Height == 12 {
			end = n.abs
		}
	})
	if end.X <= lab.X {
		t.Fatalf("arrow x %v not after label x %v", end.X, lab.X)
	}
}

func b2fWalk(n *node, fn func(*node)) {
	fn(n)
	for _, k := range n.kids {
		b2fWalk(k, fn)
	}
}

// TestCollapseCollapsibleIcon: collapsible "icon" toggles only from the
// arrow; the title does nothing (rc-collapse Panel.js collapsibleProps).
func TestCollapseCollapsibleIcon(t *testing.T) {
	var open []string
	var w *CollapseW
	d := NewDriver(func(c *Context) {
		w = Collapse(c, &open, b2fItems()).Collapsible("icon").Show()
	}, DriverOptions{Width: 400, Height: 300})
	d.EnableAccessibility()
	d.Settle()
	d.Click(b2fCenter(t, d, "Alpha"))
	d.Settle()
	if len(open) != 0 {
		t.Fatalf("title click opened %v with collapsible=icon", open)
	}
	btn, ok := d.A11yFind(a11y.RoleButton, "Alpha")
	if !ok {
		t.Fatal("no arrow button named Alpha")
	}
	b := btn.Bounds
	d.Click(geom.Pt(float32(b.X+b.W/2), float32(b.Y+b.H/2)))
	d.Settle()
	if !w.IsOpen("a") {
		t.Fatalf("arrow click did not open: %v", open)
	}
}

// TestCollapseCollapsibleHeader: collapsible "header" toggles from the
// title but not from the free space of the header.
func TestCollapseCollapsibleHeader(t *testing.T) {
	var open []string
	d := NewDriver(func(c *Context) {
		Collapse(c, &open, b2fItems()).Collapsible("header").Show()
	}, DriverOptions{Width: 400, Height: 300})
	d.Settle()
	r := b2fText(t, d, "Alpha")
	d.Click(geom.Pt(380, r.Y+r.H/2)) // empty space at the end of the header
	d.Settle()
	if len(open) != 0 {
		t.Fatalf("free header space opened %v", open)
	}
	d.Click(r.Center())
	d.Settle()
	if len(open) != 1 || open[0] != "a" {
		t.Fatalf("title click: open = %v", open)
	}
}

// TestCollapseDisabledCollapsible: collapsible "disabled", a disabled
// item and ConfigProvider.ComponentDisabled ignore clicks and keys.
func TestCollapseDisabledCollapsible(t *testing.T) {
	cases := []struct {
		name  string
		build func(c *Context, open *[]string) *CollapseW
	}{
		{"group", func(c *Context, open *[]string) *CollapseW {
			return Collapse(c, open, b2fItems()).Collapsible("disabled").Show()
		}},
		{"item", func(c *Context, open *[]string) *CollapseW {
			it := b2fItems()
			it[0].Collapsible = "disabled"
			return Collapse(c, open, it).Show()
		}},
		{"icon-mode-provider", func(c *Context, open *[]string) *CollapseW {
			var w *CollapseW
			ConfigProvider(c).ComponentDisabled(true).Body(func() { w = Collapse(c, open, b2fItems()).Collapsible("icon").Show() })
			return w
		}},
	}
	for _, tc := range cases {
		var open []string
		var w *CollapseW
		d := NewDriver(func(c *Context) { w = tc.build(c, &open) }, DriverOptions{Width: 400, Height: 300})
		d.Settle()
		r := b2fText(t, d, "Alpha")
		d.Click(r.Center())
		d.Click(geom.Pt(r.X-20, r.Y+r.H/2)) // the start arrow
		d.Settle()
		if len(open) != 0 || w.Changed() {
			t.Fatalf("%s: disabled panel toggled by pointer: %v", tc.name, open)
		}
		d.Key(input.KeyTab, 0) // skips the disabled header
		d.Key(input.KeyEnter, 0)
		d.Settle()
		if w.IsOpen("a") {
			t.Fatalf("%s: disabled panel toggled: %v", tc.name, open)
		}
	}
}

// TestCollapseBeforeCollapse: Element before-collapse returning false
// keeps the panel as it is.
func TestCollapseBeforeCollapse(t *testing.T) {
	var open []string
	allow := false
	asked := ""
	d := NewDriver(func(c *Context) {
		Collapse(c, &open, b2fItems()).BeforeCollapse(func(k string) bool { asked = k; return allow }).Show()
	}, DriverOptions{Width: 400, Height: 300})
	d.Settle()
	d.Click(b2fCenter(t, d, "Beta"))
	d.Settle()
	if asked != "b" || len(open) != 0 {
		t.Fatalf("asked %q, open %v", asked, open)
	}
	allow = true
	d.Click(b2fCenter(t, d, "Beta"))
	d.Settle()
	if len(open) != 1 || open[0] != "b" {
		t.Fatalf("allowed toggle: open %v", open)
	}
}

// TestCollapseA11yAccordion: rc-collapse uses tablist / tab / tabpanel in
// accordion mode; Enter and Space toggle the focused header.
func TestCollapseA11yAccordion(t *testing.T) {
	open := []string{"a"}
	d := NewDriver(func(c *Context) {
		Collapse(c, &open, []CollapseItem{
			{Key: "a", Label: "Alpha", Body: func() { Text(c, "AA") }},
			{Key: "b", Label: "Beta", Body: func() { Text(c, "BB") }},
		}).Accordion().Show()
	}, DriverOptions{Width: 400, Height: 300})
	d.EnableAccessibility()
	d.Settle()
	if _, ok := d.A11yFind(a11y.RoleTabList, ""); !ok {
		t.Fatal("no tablist")
	}
	tab, ok := d.A11yFind(a11y.RoleTab, "Alpha")
	if !ok || tab.States&a11y.Expanded == 0 {
		t.Fatalf("tab Alpha missing or not expanded: %+v", tab)
	}
	if _, ok := d.A11yFind(a11y.RoleTabPanel, "Alpha"); !ok {
		t.Fatal("no tabpanel for Alpha")
	}
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyTab, 0)
	d.Key(input.KeySpace, 0)
	d.Settle()
	if len(open) != 1 || open[0] != "b" {
		t.Fatalf("Space on Beta: open = %v", open)
	}
}

// TestCollapseOutOfRangeActive: open keys that no item has and a nil item
// list do not panic; the matching panels stay closed.
func TestCollapseOutOfRangeActive(t *testing.T) {
	open := []string{"ghost"}
	var w *CollapseW
	d := NewDriver(func(c *Context) {
		w = Collapse(c, &open, b2fItems()).Show()
		Collapse(c, nil, nil).Show()
	}, DriverOptions{Width: 400, Height: 300})
	d.Settle()
	if w.IsOpen("a") || !w.IsOpen("ghost") {
		t.Fatal("unknown key changed state")
	}
	d.Click(b2fCenter(t, d, "Alpha"))
	d.Settle()
	if !w.IsOpen("a") {
		t.Fatal("click after unknown key did not open")
	}
}

// TestCollapseExpandIcon: ExpandIcon replaces the arrow.
func TestCollapseExpandIcon(t *testing.T) {
	d := NewDriver(func(c *Context) {
		Collapse(c, nil, []CollapseItem{{Key: "a", Label: "Alpha"}}).ExpandIcon(icon.Plus).Show()
	}, DriverOptions{Width: 400, Height: 300})
	d.Settle() // drawing a custom icon must not panic
}

// ---------------------------------------------------------------------------
// Card

func b2fOps(d *Driver) (strokes, shadows int) {
	for _, op := range d.r.List().Ops {
		switch op.(type) {
		case paint.StrokeRect:
			strokes++
		case paint.ShadowRect:
			shadows++
		}
	}
	return
}

// TestCardVariantBorderless: antd 6 variant="borderless" drops the
// outline and keeps boxShadowTertiary (.ant-card:not(.ant-card-bordered));
// the deprecated Borderless and Defaults "variant" do the same.
func TestCardVariantBorderless(t *testing.T) {
	run := func(fn func(c *Context)) (int, int) {
		d := NewDriver(fn, DriverOptions{Width: 400, Height: 300})
		d.Settle()
		return b2fOps(d)
	}
	so, sho := run(func(c *Context) { Card(c, "T").Body(func() { Text(c, "x") }) })
	sb, shb := run(func(c *Context) { Card(c, "T").Variant("borderless").Body(func() { Text(c, "x") }) })
	sd, shd := run(func(c *Context) {
		ConfigProvider(c).Defaults("Card", Props{"variant": "borderless"}).Body(func() { Card(c, "T").Body(func() { Text(c, "x") }) })
	})
	sl, shl := run(func(c *Context) { Card(c, "T").Borderless().Body(func() { Text(c, "x") }) })
	if so != 1 || sho != 0 {
		t.Fatalf("outlined: %d strokes %d shadows, want 1 0", so, sho)
	}
	for _, v := range [][2]int{{sb, shb}, {sd, shd}, {sl, shl}} {
		if v[0] != 0 || v[1] == 0 {
			t.Fatalf("borderless: %d strokes %d shadows, want 0 and some", v[0], v[1])
		}
	}
}

// TestCardInnerBodyPadding: antd type inner pads the body 16 24.
func TestCardInnerBodyPadding(t *testing.T) {
	d := NewDriver(func(c *Context) {
		Card(c, "Inner").Inner().Layout(layout.W(300)).Body(func() { Text(c, "body") })
	}, DriverOptions{Width: 400, Height: 300})
	d.Settle()
	hd, _ := d.FindText("Inner")
	b, _ := d.FindText("body")
	if b.X > 24.5 || b.X < 23.5 {
		t.Fatalf("inline padding %v, want 24", b.X)
	}
	_ = hd
	// The header is 56 tall; the body text starts 16 below it.
	if top := b.Y - 56; top < 15 || top > 17.5 {
		t.Fatalf("body top padding %v, want 16", top)
	}
}

// TestCardHoverBorder: antd hides a hoverable card's outline under the
// pointer (&:hover { borderColor: transparent }).
func TestCardHoverBorder(t *testing.T) {
	d := NewDriver(func(c *Context) {
		Card(c, "H").Hoverable().Layout(layout.W(200)).Body(func() { Text(c, "x") })
	}, DriverOptions{Width: 400, Height: 300})
	d.Settle()
	outline := func() theme.Color {
		for _, op := range d.r.List().Ops {
			if s, ok := op.(paint.StrokeRect); ok && s.Rect.W > 150 {
				return s.Color
			}
		}
		return theme.Color{}
	}
	if outline().A == 0 {
		t.Fatal("no outline at rest")
	}
	d.Move(geom.Pt(100, 40))
	d.Settle()
	if a := outline().A; a != 0 {
		t.Fatalf("outline alpha %d under the pointer, want 0", a)
	}
}

// ---------------------------------------------------------------------------
// Descriptions

func b2fSpans(rows [][]DescItem) [][]int {
	out := make([][]int, len(rows))
	for i, r := range rows {
		for _, it := range r {
			out[i] = append(out[i], it.Span)
		}
	}
	return out
}

// TestDescriptionsRowSpans: antd useRow cuts an item that overflows the
// row instead of wrapping it, 'filled' ends the row, and the last item of
// a short row stretches.
func TestDescriptionsRowSpans(t *testing.T) {
	one := func(i int) int { return 0 }
	items := []DescItem{{Label: "a"}, {Label: "b", Span: 3}, {Label: "c"}, {Label: "d", Filled: true}, {Label: "e"}}
	spanOf := func(i int) int { return items[i].Span }
	got := b2fSpans(descRows(items, 3, spanOf))
	want := [][]int{{1, 2}, {1, 2}, {3}}
	if len(got) != len(want) {
		t.Fatalf("rows %v, want %v", got, want)
	}
	for i := range want {
		for j := range want[i] {
			if got[i][j] != want[i][j] {
				t.Fatalf("rows %v, want %v", got, want)
			}
		}
	}
	if r := descRows(nil, 3, one); len(r) != 0 {
		t.Fatalf("nil items: %v", r)
	}
}

// TestDescriptionsResponsiveSpan: DescItem.Spans picks the span of the
// window's breakpoint.
func TestDescriptionsResponsiveSpan(t *testing.T) {
	var d *DescriptionsW
	dr := NewDriver(func(c *Context) {
		d = Descriptions(c, []DescItem{{Label: "a", Spans: map[Breakpoint]int{XS: 1, LG: 2}}, {Label: "b"}, {Label: "c"}})
	}, DriverOptions{Width: 1300, Height: 300})
	dr.Settle()
	if got := b2fSpans(d.rows()); got[0][0] != 2 {
		t.Fatalf("lg span: %v", got)
	}
}

// TestDescriptionsEmpty: an empty list and out-of-range spans do not
// panic.
func TestDescriptionsEmpty(t *testing.T) {
	d := NewDriver(func(c *Context) {
		Descriptions(c, nil).Title("T")
		Descriptions(c, []DescItem{{Label: "x", Span: 99}, {Label: "y", Span: -3}}).Bordered()
		Descriptions(c, []DescItem{{Label: "x", Filled: true}}).Vertical()
	}, DriverOptions{Width: 600, Height: 400})
	d.Settle()
}

// TestDescriptionsLastRowPadding: borderless rows keep itemPaddingBottom
// (16 / 12 / 8 by size) except the last row, and the label shows the
// locale colon (antd colon) except under Element.
func TestDescriptionsLastRowPadding(t *testing.T) {
	var w *DescriptionsW
	d := NewDriver(func(c *Context) {
		w = Descriptions(c, []DescItem{{Label: "A", Value: "1"}, {Label: "B", Value: "2"}}).Column(1).Size(Middle)
	}, DriverOptions{Width: 400, Height: 300})
	d.Settle()
	box := d.r.c.rects[w.b.n.id]
	b, _ := d.FindText("2")
	if gap := box.Bottom() - b.Bottom(); gap > 1 {
		t.Fatalf("last row keeps %v px below", gap)
	}
	a, _ := d.FindText("1")
	if dy := b.Y - a.Y; dy < a.H+11.5 || dy > a.H+12.5 {
		t.Fatalf("row pitch %v, want text height %v + 12", dy, a.H)
	}
	if _, ok := d.FindText(d.r.c.Locale().LabelColon); !ok {
		t.Fatal("no colon after the label")
	}
	d.SetTheme(theme.ElementPlus())
	d.Settle()
	if _, ok := d.FindText(d.r.c.Locale().LabelColon); ok {
		t.Fatal("Element labels have no colon")
	}
}

// ---------------------------------------------------------------------------
// List

// TestListItemLayoutVertical: itemLayout vertical puts ListActions under
// the content and ItemExtra in its own column at the end.
func TestListItemLayoutVertical(t *testing.T) {
	d := NewDriver(func(c *Context) {
		List(c, 1).ItemLayout("vertical").ItemExtra(func(i int) { Text(c, "EXTRA") }).Item(func(i int) {
			ListItemMeta(c, nil, "Title", "Desc")
			Text(c, "Content")
			ListActions(c, func() { Text(c, "Act1"); Text(c, "Act2") })
		})
	}, DriverOptions{Width: 600, Height: 400})
	d.Settle()
	tt, _ := d.FindText("Title")
	co, _ := d.FindText("Content")
	ac, _ := d.FindText("Act1")
	ex, _ := d.FindText("EXTRA")
	if !(ac.Y > co.Y && co.Y > tt.Y) {
		t.Fatalf("vertical order: title %v content %v action %v", tt.Y, co.Y, ac.Y)
	}
	if ex.X < co.Right() {
		t.Fatalf("extra x %v not after content", ex.X)
	}
	// The large title (fontSizeLG) is taller than a body line.
	if tt.H <= co.H {
		t.Fatalf("title height %v not larger than %v", tt.H, co.H)
	}
}

// TestListSmallPadding: antd .ant-list-sm .ant-list-item pads 8 16 even
// without a border.
func TestListSmallPadding(t *testing.T) {
	d := NewDriver(func(c *Context) {
		List(c, 1).Size(Small).Item(func(i int) { Text(c, "row") })
	}, DriverOptions{Width: 400, Height: 200})
	d.Settle()
	r, _ := d.FindText("row")
	if r.X < 15.5 || r.X > 16.5 {
		t.Fatalf("small item text x %v, want 16", r.X)
	}
}

// TestListEmptyOutOfRange: no items shows the Empty description; a page
// beyond the last shows the last page without panicking.
func TestListEmptyOutOfRange(t *testing.T) {
	page := 99
	d := NewDriver(func(c *Context) {
		List(c, 0).Item(func(i int) {})
		List(c, -5).Item(func(i int) {})
		List(c, 12).Pagination(&page, 5).PaginationPosition("both").Item(func(i int) { Text(c, "item"+string(rune('A'+i))) })
	}, DriverOptions{Width: 600, Height: 800})
	d.Settle()
	if _, ok := d.FindText(d.r.c.Locale().NoData); !ok {
		t.Fatal("empty list shows no Empty description")
	}
	if _, ok := d.FindText("itemK"); !ok {
		t.Fatal("out-of-range page does not show the last page")
	}
}

// ---------------------------------------------------------------------------
// Listy

// TestListyGroupHeaderLook: antd group header in colorTextDescription on
// colorFillAlter, 38 px (fontHeight + paddingXS * 2), no rule.
func TestListyGroupHeaderLook(t *testing.T) {
	d := NewDriver(func(c *Context) {
		Listy(c, 2, func(i int) { Text(c, "row") }).Group(func(i int) string { return "G" }, nil).Show()
	}, DriverOptions{Width: 400, Height: 300})
	d.Settle()
	th := d.r.c.Theme
	found := false
	for _, op := range d.r.List().Ops {
		if tx, ok := op.(paint.Text); ok && tx.Color == th.TextTertiary {
			found = true
		}
	}
	if !found {
		t.Fatal("group header text is not colorTextDescription")
	}
	r, _ := d.FindText("G")
	_ = r
	var head geom.Rect
	b2fWalk(d.r.c.root, func(n *node) {
		if n.role == a11y.RoleHeading {
			head = n.abs
		}
	})
	if head.H != 38 {
		t.Fatalf("group header height %v, want 38", head.H)
	}
}

// ---------------------------------------------------------------------------
// VirtualList

// TestVirtualListEmptyOutOfRange: zero rows and out-of-range scroll
// requests do not panic.
func TestVirtualListEmptyOutOfRange(t *testing.T) {
	d := NewDriver(func(c *Context) {
		VirtualList(c, 0, 30, func(i int) {}).Height(100).ScrollToIndex(50).ScrollIntoView(-3)
		VirtualList(c, 10, 30, func(i int) { Text(c, "v") }).Height(100).ScrollToIndex(500)
	}, DriverOptions{Width: 400, Height: 300})
	d.Settle()
}

// ---------------------------------------------------------------------------
// Tree

func b2fTree() []TreeNode {
	return []TreeNode{
		{Key: "p", Title: "Parent", Children: []TreeNode{{Key: "c1", Title: "Child1"}, {Key: "c2", Title: "Child2"}}},
		{Key: "q", Title: "Other", Children: []TreeNode{{Key: "d1", Title: "Deep1"}}},
	}
}

// TestTreeSelectableFalse: antd selectable={false} ignores title clicks
// and Enter.
func TestTreeSelectableFalse(t *testing.T) {
	var sel []string
	d := NewDriver(func(c *Context) {
		Tree(c, b2fTree()).Selected(&sel).Selectable(false).Show()
	}, DriverOptions{Width: 400, Height: 300})
	d.Settle()
	d.Click(b2fCenter(t, d, "Parent"))
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if len(sel) != 0 {
		t.Fatalf("unselectable tree selected %v", sel)
	}
}

// TestTreeDisabled: Tree.Disabled and ComponentDisabled block selection,
// expansion, checking and focus.
func TestTreeDisabled(t *testing.T) {
	for _, viaProvider := range []bool{false, true} {
		var sel, exp, chk []string
		d := NewDriver(func(c *Context) {
			build := func() { Tree(c, b2fTree()).Selected(&sel).Expanded(&exp).Checkable(&chk).Disabled(!viaProvider).Show() }
			if viaProvider {
				ConfigProvider(c).ComponentDisabled(true).Body(build)
			} else {
				build()
			}
		}, DriverOptions{Width: 400, Height: 300})
		d.Settle()
		r := b2fCenter(t, d, "Parent")
		d.Click(r)
		d.Click(geom.Pt(12, r.Y)) // switcher
		d.Click(geom.Pt(32, r.Y)) // checkbox
		d.Key(input.KeyTab, 0)
		d.Key(input.KeyRight, 0)
		d.Key(input.KeySpace, 0)
		d.Settle()
		if len(sel)+len(exp)+len(chk) != 0 {
			t.Fatalf("provider=%v: disabled tree changed sel %v exp %v chk %v", viaProvider, sel, exp, chk)
		}
	}
}

// TestTreeDefaultExpandAll: every parent opens on the first show only.
func TestTreeDefaultExpandAll(t *testing.T) {
	var exp []string
	d := NewDriver(func(c *Context) { Tree(c, b2fTree()).Expanded(&exp).DefaultExpandAll().Show() }, DriverOptions{Width: 400, Height: 300})
	d.Settle()
	if len(exp) != 2 {
		t.Fatalf("expanded %v, want p q", exp)
	}
	exp = nil
	d.Settle()
	if len(exp) != 0 {
		t.Fatalf("DefaultExpandAll re-applied: %v", exp)
	}
}

// TestTreeAccordion: Element accordion collapses the siblings of the
// expanded node.
func TestTreeAccordion(t *testing.T) {
	exp := []string{"p"}
	d := NewDriver(func(c *Context) { Tree(c, b2fTree()).Expanded(&exp).Accordion().Show() }, DriverOptions{Width: 400, Height: 300})
	d.Settle()
	r := b2fCenter(t, d, "Other")
	d.Click(geom.Pt(12, r.Y))
	d.Settle()
	if len(exp) != 1 || exp[0] != "q" {
		t.Fatalf("expanded %v, want only q", exp)
	}
}

// TestTreeCheckOnClick: Element check-on-click-node / -leaf.
func TestTreeCheckOnClick(t *testing.T) {
	var chk []string
	exp := []string{"p"}
	leafOnly := true
	d := NewDriver(func(c *Context) {
		w := Tree(c, b2fTree()).Expanded(&exp).Checkable(&chk)
		if leafOnly {
			w.CheckOnClickLeaf(true)
		} else {
			w.CheckOnClick(true)
		}
		w.Show()
	}, DriverOptions{Width: 400, Height: 300})
	d.Settle()
	d.Click(b2fCenter(t, d, "Parent"))
	d.Settle()
	if len(chk) != 0 {
		t.Fatalf("leaf-only: parent click checked %v", chk)
	}
	d.Click(b2fCenter(t, d, "Child1"))
	d.Settle()
	if len(chk) != 1 || chk[0] != "c1" {
		t.Fatalf("leaf click: checked %v", chk)
	}
	leafOnly = false
	d.Click(b2fCenter(t, d, "Parent"))
	d.Settle()
	if len(chk) != 3 {
		t.Fatalf("node click on parent: checked %v, want p c1 c2", chk)
	}
}

// TestTreeAllowDragDrop: AllowDrag / AllowDrop gate the drag (antd
// nodeDraggable / allowDrop, Element allow-drag / allow-drop).
func TestTreeAllowDragDrop(t *testing.T) {
	var w *TreeW
	allowDrop := false
	d := NewDriver(func(c *Context) {
		w = Tree(c, []TreeNode{{Key: "a", Title: "Aaa"}, {Key: "b", Title: "Bbb"}, {Key: "x", Title: "Xxx"}}).Draggable().
			AllowDrag(func(k string) bool { return k != "x" }).
			AllowDrop(func(drag, drop string, pos int) bool { return allowDrop }).Show()
	}, DriverOptions{Width: 400, Height: 300})
	d.Settle()
	a, b, x := b2fCenter(t, d, "Aaa"), b2fCenter(t, d, "Bbb"), b2fCenter(t, d, "Xxx")
	d.Drag(a, b)
	d.Settle()
	if _, _, _, ok := w.Dropped(); ok {
		t.Fatal("drop allowed although AllowDrop said no")
	}
	allowDrop = true
	d.Drag(x, b)
	d.Settle()
	if _, _, _, ok := w.Dropped(); ok {
		t.Fatal("dragged a node AllowDrag refused")
	}
	dropped := false
	d2 := NewDriver(func(c *Context) {
		tw := Tree(c, []TreeNode{{Key: "a", Title: "Aaa"}, {Key: "b", Title: "Bbb"}}).Draggable().
			AllowDrop(func(drag, drop string, pos int) bool { return true }).Show()
		if _, _, _, ok := tw.Dropped(); ok {
			dropped = true
		}
	}, DriverOptions{Width: 400, Height: 300})
	d2.Settle()
	d2.Drag(b2fCenter(t, d2, "Aaa"), b2fCenter(t, d2, "Bbb"))
	d2.Settle()
	if !dropped {
		t.Fatal("allowed drop not reported")
	}
}

// TestTreeEmptyUnknownKeys: no nodes renders nothing under antd and the
// Element empty text; selected / expanded keys that no node has do not
// panic.
func TestTreeEmptyUnknownKeys(t *testing.T) {
	sel, exp := []string{"ghost"}, []string{"nope"}
	d := NewDriver(func(c *Context) {
		Tree(c, nil).Show()
		Tree(c, b2fTree()).Selected(&sel).Expanded(&exp).Show()
	}, DriverOptions{Width: 400, Height: 300})
	d.Settle()
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyDown, 0)
	d.Settle()
	if _, ok := d.FindText(d.r.c.Locale().NoData); ok {
		t.Fatal("antd Tree shows an empty text")
	}
	d.SetTheme(theme.ElementPlus())
	d.Settle()
	if _, ok := d.FindText(d.r.c.Locale().NoData); !ok {
		t.Fatal("Element Tree shows no empty text")
	}
}

// TestTreeA11ySwitcherIcon: tree / treeitem roles with expanded state
// survive a custom switcher icon; Right expands from the keyboard.
func TestTreeA11ySwitcherIcon(t *testing.T) {
	var exp []string
	d := NewDriver(func(c *Context) { Tree(c, b2fTree()).Expanded(&exp).SwitcherIcon(icon.Down).A11yLabel("Files").Show() }, DriverOptions{Width: 400, Height: 300})
	d.EnableAccessibility()
	d.Settle()
	if _, ok := d.A11yFind(a11y.RoleTree, "Files"); !ok {
		t.Fatal("no tree named Files")
	}
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyDown, 0)
	d.Key(input.KeyRight, 0)
	d.Settle()
	it, ok := d.A11yFind(a11y.RoleTreeItem, "Parent")
	if !ok || it.States&a11y.Expanded == 0 || len(exp) != 1 {
		t.Fatalf("Right did not expand Parent: %v %+v", exp, it)
	}
}

// ---------------------------------------------------------------------------
// VirtualTree

// TestVirtualTreeAccordionLeafCheck: Accordion and CheckOnClickLeaf.
func TestVirtualTreeAccordionLeafCheck(t *testing.T) {
	exp := []string{"p"}
	var chk []string
	var w *VirtualTreeW
	d := NewDriver(func(c *Context) {
		w = VirtualTree(c, b2fTree()).Expanded(&exp).Checkable(&chk).Accordion().CheckOnClickLeaf(true).ExpandOnClick(true).Show()
	}, DriverOptions{Width: 400, Height: 300})
	d.Settle()
	d.Click(b2fCenter(t, d, "Child1"))
	d.Settle()
	if len(chk) != 1 || chk[0] != "c1" {
		t.Fatalf("leaf click: checked %v", chk)
	}
	d.Click(b2fCenter(t, d, "Other"))
	d.Settle()
	if len(exp) != 1 || exp[0] != "q" {
		t.Fatalf("accordion: expanded %v", exp)
	}
	_ = w
}

// TestVirtualTreeEmptyText: Element el-tree-v2 empty text; antd keeps the
// simple Empty.
func TestVirtualTreeEmptyText(t *testing.T) {
	d := NewDriver(func(c *Context) {
		VirtualTree(c, nil).EmptyText("Nothing").Show()
	}, DriverOptions{Width: 400, Height: 300})
	d.Settle()
	if _, ok := d.FindText("Nothing"); !ok {
		t.Fatal("EmptyText not shown")
	}
}

// TestTreeFilterMethod: FilterMethod replaces the title match of Search /
// Filter (antd filterTreeNode, Element filter-node-method).
func TestTreeFilterMethod(t *testing.T) {
	var exp []string
	d := NewDriver(func(c *Context) {
		Tree(c, b2fTree()).Expanded(&exp).Search("deep").Filter().
			FilterMethod(func(kw string, n *TreeNode) bool { return n.Key == "d1" }).Show()
	}, DriverOptions{Width: 400, Height: 300})
	d.Settle()
	if len(exp) != 1 || exp[0] != "q" {
		t.Fatalf("expanded %v, want the parent of d1", exp)
	}
	if _, ok := d.FindText("Parent"); ok {
		t.Fatal("filtered-out node still shown")
	}
	if _, ok := d.FindText("Deep1"); !ok {
		t.Fatal("match not shown")
	}
}

// TestCollapseCollapsibleHeaderNoArrow: in header mode without an arrow
// the title is the trigger (rc-collapse Panel.js gives the title span
// role, tabIndex 0, Enter / Space and aria-expanded).
func TestCollapseCollapsibleHeaderNoArrow(t *testing.T) {
	var open []string
	d := NewDriver(func(c *Context) {
		Collapse(c, &open, []CollapseItem{{Key: "a", Label: "Alpha", NoArrow: true, Body: func() {}}}).Collapsible("header").Show()
	}, DriverOptions{Width: 400, Height: 300})
	d.EnableAccessibility()
	d.Settle()
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if len(open) != 1 {
		t.Fatalf("Tab + Enter on the title: open = %v", open)
	}
	btn, ok := d.A11yFind(a11y.RoleButton, "Alpha")
	if !ok || btn.States&a11y.Expanded == 0 {
		t.Fatalf("title button missing or not expanded: %+v", btn)
	}
}

// TestCollapseAccordionFirstKey: in accordion mode only activeKey[0] is
// open (rc-collapse useItems.js); the app's slice is left alone until a
// toggle.
func TestCollapseAccordionFirstKey(t *testing.T) {
	open := []string{"b", "a"}
	var w *CollapseW
	d := NewDriver(func(c *Context) { w = Collapse(c, &open, b2fItems()).Accordion().Show() }, DriverOptions{Width: 400, Height: 300})
	d.Settle()
	if !w.IsOpen("b") || w.IsOpen("a") || len(open) != 2 {
		t.Fatalf("accordion open: a=%v b=%v slice %v", w.IsOpen("a"), w.IsOpen("b"), open)
	}
	d.Click(b2fCenter(t, d, "Beta"))
	d.Settle()
	if len(open) != 0 {
		t.Fatalf("closing the open panel: %v", open)
	}
}

// TestCardBorderlessHoverable: a borderless hoverable Ant card keeps
// boxShadowTertiary at rest and shows cardShadow under the pointer.
func TestCardBorderlessHoverable(t *testing.T) {
	d := NewDriver(func(c *Context) {
		Card(c, "H").Variant("borderless").Hoverable().Layout(layout.W(200)).Body(func() { Text(c, "x") })
	}, DriverOptions{Width: 400, Height: 300})
	d.Settle()
	if _, sh := b2fOps(d); sh == 0 {
		t.Fatal("borderless hoverable card lost its resting shadow")
	}
	d.Move(geom.Pt(100, 40))
	d.Settle()
	if _, sh := b2fOps(d); sh == 0 {
		t.Fatal("no shadow under the pointer")
	}
}

// TestDescriptionsNoColonGap: without a colon antd keeps the ::after
// margins (2 + 8 = 10 px between label and content).
func TestDescriptionsNoColonGap(t *testing.T) {
	d := NewDriver(func(c *Context) {
		Descriptions(c, []DescItem{{Label: "Lbl", Value: "Val"}}).NoColon()
	}, DriverOptions{Width: 400, Height: 200})
	d.Settle()
	l, v := b2fText(t, d, "Lbl"), b2fText(t, d, "Val")
	if gap := v.X - l.Right(); gap < 9.5 || gap > 10.5 {
		t.Fatalf("label to content gap %v, want 10", gap)
	}
}

// TestTreeA11yUnselectable: Selectable(false) does not announce
// selectable tree items.
func TestTreeA11yUnselectable(t *testing.T) {
	d := NewDriver(func(c *Context) { Tree(c, b2fTree()).Selectable(false).Show() }, DriverOptions{Width: 400, Height: 300})
	d.EnableAccessibility()
	d.Settle()
	it, ok := d.A11yFind(a11y.RoleTreeItem, "Parent")
	if !ok || it.States&a11y.Selectable != 0 {
		t.Fatalf("tree item %+v announces Selectable", it)
	}
}
