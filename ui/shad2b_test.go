package ui

import (
	"image/color"
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/theme"
)

func shad2bPx(d *Driver, x, y float32) color.NRGBA {
	return color.NRGBAModel.Convert(d.Image().At(int(x), int(y))).(color.NRGBA)
}

// TestShad2bTableTokens: Ant keeps tinting the sorted column, shadcn has
// no column tint and no header fill.
func TestShad2bTableTokens(t *testing.T) {
	ant := theme.AntDesign()
	if ant.TableSortedBg != ant.FillQuaternary || ant.TableHeaderActiveBg != ant.FillTertiary {
		t.Errorf("Ant sort tokens changed: %v %v", ant.TableSortedBg, ant.TableHeaderActiveBg)
	}
	for _, th := range []*theme.Theme{theme.Shadcn(), theme.Shadcn(theme.Dark())} {
		if th.TableSortedBg.A != 0 || th.TableHeaderActiveBg.A != 0 {
			t.Errorf("shadcn dark=%v tints sorted columns", th.Dark)
		}
		if th.CardRadius != th.RadiusLG+4 {
			t.Errorf("shadcn card radius %v, want rounded-xl %v", th.CardRadius, th.RadiusLG+4)
		}
	}
	if theme.AntDesign().CardRadius != 0 || theme.ElementPlus().CardRadius != 0 {
		t.Error("Ant/Element CardRadius must stay zero (design default)")
	}
}

// TestShad2bTableLook checks the shadcn data-table geometry, the absence
// of the sorted-column tint and the sort arrow after the title.
func TestShad2bTableLook(t *testing.T) {
	cols := []TableColumn{
		{Key: "name", Title: "Name", Text: func(i int) string { return []string{"b", "a", "c"}[i] }, Width: 200},
		{Key: "age", Title: "Age", Sortable: true, Compare: func(a, b int) int { return a - b }, Text: func(i int) string { return []string{"2", "1", "3"}[i] }, Width: 200},
	}
	sort := SortState{Column: "age", Order: SortAsc}
	page := func(c *Context) { Table(c, cols, 3).Sort(&sort).Show() }
	d := NewDriver(page, DriverOptions{Width: 600, Height: 300})
	d.SetTheme(theme.Shadcn())
	d.Settle()
	hd, ok := d.Find("Age")
	if !ok {
		t.Fatal("header missing")
	}
	if hd.H != 41 {
		t.Errorf("header height %v, want 41 (h-10 + border)", hd.H)
	}
	c1, ok := d.FindText("1")
	if !ok {
		t.Fatal("cell missing")
	}
	// The sorted column's cell background stays the page background.
	bg := theme.Shadcn().BgContainer
	if got := shad2bPx(d, hd.X+hd.W-6, c1.Y+c1.H/2); !nearColor(got, bg, 2) {
		t.Errorf("sorted column tinted: %v, want %v", got, bg)
	}
	if got := shad2bPx(d, hd.X+hd.W-6, hd.Y+4); !nearColor(got, bg, 2) {
		t.Errorf("sorted header tinted: %v, want %v", got, bg)
	}
	// Ant still tints the sorted column.
	d.SetTheme(theme.AntDesign())
	d.Settle()
	hd, _ = d.Find("Age")
	c1, _ = d.FindText("1")
	if got := shad2bPx(d, hd.X+hd.W-30, c1.Y+c1.H/2); nearColor(got, color.NRGBA{255, 255, 255, 255}, 0) {
		t.Errorf("Ant sorted column lost its tint")
	}
}

// TestShad2bTableRowHeight: shadcn rows are p-2 + leading-5 + border.
func TestShad2bTableRowHeight(t *testing.T) {
	cols := []TableColumn{{Key: "a", Title: "A", Text: func(i int) string { return "row" + itoa(i) }}}
	page := func(c *Context) { Table(c, cols, 2).Show() }
	d := NewDriver(page, DriverOptions{Width: 300, Height: 200})
	d.SetTheme(theme.Shadcn())
	d.Settle()
	r0, ok0 := d.FindText("row0")
	r1, ok1 := d.FindText("row1")
	if !ok0 || !ok1 {
		t.Fatal("rows missing")
	}
	if got := r1.Y - r0.Y; got != 37 {
		t.Errorf("row pitch %v, want 37", got)
	}
}

// TestShad2bDefaults checks ConfigProvider.Defaults wiring of the data
// components and that explicit modifiers win.
func TestShad2bDefaults(t *testing.T) {
	var card *CardW
	var tbl *TableW
	var desc *DescriptionsW
	var list *ListW
	var tree *TreeW
	var badge *BadgeW
	var tl *TimelineW
	var pop *PopoverW
	page := func(c *Context) {
		ConfigProvider(c).
			Defaults("Card", Props{"size": "small", "variant": "borderless"}).
			Defaults("Table", Props{"size": "small", "bordered": true}).
			Defaults("Descriptions", Props{"column": map[Breakpoint]int{XS: 1, MD: 2}, "bordered": true}).
			Defaults("List", Props{"bordered": true, "split": false}).
			Defaults("Tree", Props{"showLine": true, "blockNode": true, "height": 200}).
			Defaults("Badge", Props{"overflowCount": 9, "showZero": true}).
			Defaults("Timeline", Props{"mode": "alternate"}).
			Defaults("Popover", Props{"placement": "bottom", "arrow": false}).
			Body(func() {
				card = Card(c, "c")
				card.Body(nil)
				tbl = Table(c, nil, 0)
				desc = Descriptions(c, nil)
				list = List(c, 0)
				tree = Tree(c, nil)
				badge = Badge(c).Count(3)
				tl = Timeline(c, nil)
				pop = Popover(c)
				Descriptions(c, nil).Column(5)
			})
	}
	d := NewDriver(page, DriverOptions{Width: 900, Height: 400})
	d.Settle()
	if !card.small || card.bordered {
		t.Errorf("Card defaults: small=%v bordered=%v", card.small, card.bordered)
	}
	if !tbl.sizeSet || tbl.size != Small || !tbl.bordered {
		t.Errorf("Table defaults: %v %v %v", tbl.sizeSet, tbl.size, tbl.bordered)
	}
	if !desc.bordered || desc.cols != 2 {
		t.Errorf("Descriptions defaults: bordered=%v cols=%v (900px = MD)", desc.bordered, desc.cols)
	}
	if !list.bordered || list.split {
		t.Errorf("List defaults: %v %v", list.bordered, list.split)
	}
	if !tree.showLine || !tree.block || tree.height != 200 {
		t.Errorf("Tree defaults: %v %v %v", tree.showLine, tree.block, tree.height)
	}
	if badge.overflow != 9 || !badge.showZero {
		t.Errorf("Badge defaults: %v %v", badge.overflow, badge.showZero)
	}
	if tl.mode != "alternate" {
		t.Errorf("Timeline mode %q", tl.mode)
	}
	if pop.place != "bottom" || pop.arrow {
		t.Errorf("Popover defaults: %q %v", pop.place, pop.arrow)
	}
}

// TestShad2bDescriptionsColumnAt: the column count follows the window
// breakpoint; an explicit Column beats a default map.
func TestShad2bDescriptionsColumnAt(t *testing.T) {
	var w, w2 *DescriptionsW
	items := []DescItem{{Label: "a", Value: "1"}, {Label: "b", Value: "2"}, {Label: "c", Value: "3"}}
	page := func(c *Context) {
		w = Descriptions(c, items).ColumnAt(XS, 1).ColumnAt(MD, 2).ColumnAt(XL, 3)
		ConfigProvider(c).Defaults("Descriptions", Props{"column": map[Breakpoint]int{XS: 4}}).Body(func() {
			w2 = Descriptions(c, items).Column(1)
		})
	}
	for _, tc := range []struct {
		width float32
		want  int
	}{{400, 1}, {800, 2}, {1300, 3}} {
		d := NewDriver(page, DriverOptions{Width: tc.width, Height: 300})
		d.Settle()
		if w.cols != tc.want {
			t.Errorf("width %v: %d columns, want %d", tc.width, w.cols, tc.want)
		}
		if w2.cols != 1 {
			t.Errorf("explicit Column lost to the default map: %d", w2.cols)
		}
	}
}

// TestShad2bListGridAt: the grid column count follows the breakpoint.
func TestShad2bListGridAt(t *testing.T) {
	page := func(c *Context) {
		List(c, 6).Grid(1, 8).GridAt(MD, 3).Item(func(i int) { Text(c, "item"+itoa(i)) })
	}
	count := func(width float32) int {
		d := NewDriver(page, DriverOptions{Width: width, Height: 600})
		d.Settle()
		first, _ := d.FindText("item0")
		n := 0
		for i := 0; i < 6; i++ {
			if r, ok := d.FindText("item" + itoa(i)); ok && r.Y == first.Y {
				n++
			}
		}
		return n
	}
	if n := count(400); n != 1 {
		t.Errorf("XS: %d per row, want 1", n)
	}
	if n := count(800); n != 3 {
		t.Errorf("MD: %d per row, want 3", n)
	}
}

// TestShad2bTreeVirtual: with Height only the rows in view are built, the
// keyboard cursor scrolls into view and selection still works.
func TestShad2bTreeVirtual(t *testing.T) {
	nodes := make([]TreeNode, 500)
	for i := range nodes {
		nodes[i] = TreeNode{Key: string(rune('a'+i%26)) + itoa(i), Title: "node " + itoa(i)}
	}
	var sel []string
	var w *TreeW
	page := func(c *Context) { w = Tree(c, nodes).Selected(&sel).Height(240).A11yLabel("t").Show() }
	d := NewDriver(page, DriverOptions{Width: 400, Height: 400})
	d.Settle()
	if w.Built() == 0 || w.Built() > 20 {
		t.Fatalf("built %d rows, want only the visible ones", w.Built())
	}
	if _, ok := d.FindText("node 400"); ok {
		t.Fatal("far row built")
	}
	if err := d.ClickLabel("node 1"); err != nil {
		t.Fatal(err)
	}
	if len(sel) != 1 || sel[0] != nodes[1].Key {
		t.Fatalf("selected %v", sel)
	}
	d.Key(input.KeyEnd, 0)
	d.Settle()
	if _, ok := d.FindText("node 499"); !ok {
		t.Error("End did not scroll the last row into view")
	}
	// Without Height every row is built.
	page2 := func(c *Context) { w = Tree(c, nodes[:50]).Show() }
	d2 := NewDriver(page2, DriverOptions{Width: 400, Height: 400})
	d2.Settle()
	if w.Built() != 50 {
		t.Errorf("plain tree built %d rows, want 50", w.Built())
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}

// TestShad2bCarouselCardKeyboard: card-mode side cards are pointer
// shortcuts with a keyboard equivalent (the audit's not-keyboard items).
func TestShad2bCarouselCardKeyboard(t *testing.T) {
	page := func(c *Context) {
		Carousel(c, 5, func(i int) { Text(c, "slide") }).Card().Height(200).Show()
	}
	d := NewDriver(page, DriverOptions{Width: 600, Height: 300})
	d.EnableAccessibility()
	d.Settle()
	for _, is := range d.Audit().Issues {
		if is.Kind == AuditNotKeyboard {
			t.Errorf("audit: %+v", is)
		}
	}
}

// TestShad2bPopoverArrow: shadcn popovers have no arrow by default.
func TestShad2bPopoverArrow(t *testing.T) {
	var p *PopoverW
	page := func(c *Context) { p = Popover(c) }
	d := NewDriver(page, DriverOptions{Width: 200, Height: 100})
	d.SetTheme(theme.Shadcn())
	d.Settle()
	if p.arrow {
		t.Error("shadcn popover has an arrow")
	}
	d.SetTheme(theme.AntDesign())
	d.Settle()
	if !p.arrow {
		t.Error("Ant popover lost its arrow")
	}
}

// TestShad2bTreeShadcnMetrics: shadcn tree rows are h-7 with a 2px gap.
func TestShad2bTreeShadcnMetrics(t *testing.T) {
	nodes := []TreeNode{{Key: "a", Title: "A"}, {Key: "b", Title: "B"}}
	page := func(c *Context) { Tree(c, nodes).A11yLabel("t").Show() }
	d := NewDriver(page, DriverOptions{Width: 300, Height: 200})
	d.SetTheme(theme.Shadcn())
	d.EnableAccessibility()
	d.Settle()
	a, okA := d.A11yFind(a11y.RoleTreeItem, "A")
	b, okB := d.A11yFind(a11y.RoleTreeItem, "B")
	if !okA || !okB {
		t.Fatal("tree items missing")
	}
	if got := b.Bounds.Y - a.Bounds.Y; got != 30 {
		t.Errorf("row pitch %v, want 30 (h-7 + gap)", got)
	}
}
