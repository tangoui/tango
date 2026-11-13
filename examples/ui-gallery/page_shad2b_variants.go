package main

import (
	"fmt"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// Element / dark variants of data-display pages registered elsewhere
// without them (docs/specs/ui-coverage.md 5.2), plus demos of the
// responsive column counts and the virtual Tree.

// shad2bVariantPages: page name → which variants to add.
var shad2bVariantPages = []struct {
	name     string
	element  bool
	dark     bool
	elemDark bool
}{
	{"Table", true, true, false},
	{"Tag", true, true, false},
	{"Tooltip", true, true, false},
	{"Divider", true, true, false},
	{"Typography", true, true, false},
	{"BorderBeam", false, true, false},
	{"InfiniteScroll", false, true, false},
	{"Listy", false, true, false},
	{"Markdown", false, true, false},
	{"MarkdownEditor", false, true, false},
	{"Masonry", false, true, false},
	{"RichText", false, true, false},
	{"VirtualList", false, true, false},
	{"VirtualTree", false, true, false},
}

func init() {
	byName := map[string]gallery.Page{}
	for _, p := range gallery.Pages() {
		if !p.Variant {
			byName[p.Name] = p
		}
	}
	for _, v := range shad2bVariantPages {
		p, ok := byName[v.name]
		if !ok {
			continue // page registered by a later file or renamed
		}
		if v.element {
			shad2bVariant(p, "-Element", "（Element Plus）", func() *theme.Theme { return theme.ElementPlus() })
		}
		if v.dark {
			shad2bVariant(p, "-Dark", "（暗色）", func() *theme.Theme { return theme.AntDesign(theme.Dark()) })
		}
		if v.elemDark {
			shad2bVariant(p, "-element-dark", "（Element Plus 暗色）", func() *theme.Theme { return theme.ElementPlus(theme.Dark()) })
		}
	}

	gallery.Register(gallery.Page{Name: "DescriptionsResponsive", Title: "Descriptions / List 列数随断点变化", Parent: "Descriptions", Group: gallery.Display, Order: 6, Width: 960,
		Render: func(c *ui.Context) {
			bp := ui.ScreenBreakpoint(c)
			ui.Text(c, fmt.Sprintf("当前断点：%s（窗口宽度决定）；ColumnAt(XS,1) · ColumnAt(SM,2) · ColumnAt(LG,3)", bp)).Tertiary()
			ui.Column(c, nil, layout.H(12))
			items := []ui.DescItem{{Label: "产品", Value: "云数据库"}, {Label: "计费模式", Value: "预付费"}, {Label: "自动续费", Value: "是"},
				{Label: "下单时间", Value: "2018-04-24"}, {Label: "使用时间", Value: "2019-04-24"}, {Label: "状态", Value: "运行中"}}
			ui.Descriptions(c, items).Title("响应式列数").Bordered().ColumnAt(ui.XS, 1).ColumnAt(ui.SM, 2).ColumnAt(ui.LG, 3)
			ui.Column(c, nil, layout.H(24))
			ui.Text(c, "List.GridAt(XS,1) · GridAt(MD,3) · GridAt(XL,4)").Tertiary()
			ui.Column(c, nil, layout.H(12))
			ui.List(c, 6).Grid(1, 16).GridAt(ui.MD, 3).GridAt(ui.XL, 4).Item(func(i int) {
				ui.Card(c, fmt.Sprintf("卡片 %d", i+1)).Small().Body(func() { ui.Text(c, "列表网格项") })
			})
		}})

	gallery.Register(gallery.Page{Name: "TreeVirtual", Title: "Tree 虚拟滚动（Height）", Parent: "Tree", Group: gallery.Display, Order: 16,
		Render: func(c *ui.Context) {
			nodes := shad2bBigTree()
			ui.Text(c, "Height(320) 后只构建视口内的行：1000 个一级节点、每个 5 个子节点。").Tertiary()
			ui.Column(c, nil, layout.H(12))
			w := ui.Tree(c, nodes).Checkable(nil).Expanded(&shad2bTreeExpanded).Height(320).A11yLabel("virtual tree").Show()
			ui.Column(c, nil, layout.H(8))
			ui.Text(c, fmt.Sprintf("本帧构建行数：%d", w.Built())).Tertiary()
		}})
}

var shad2bTreeExpanded = []string{"n0", "n2"}

var shad2bBigTreeCache []ui.TreeNode

func shad2bBigTree() []ui.TreeNode {
	if shad2bBigTreeCache != nil {
		return shad2bBigTreeCache
	}
	out := make([]ui.TreeNode, 1000)
	for i := range out {
		kids := make([]ui.TreeNode, 5)
		for j := range kids {
			kids[j] = ui.TreeNode{Key: fmt.Sprintf("n%d-%d", i, j), Title: fmt.Sprintf("节点 %d-%d", i, j)}
		}
		out[i] = ui.TreeNode{Key: fmt.Sprintf("n%d", i), Title: fmt.Sprintf("节点 %d", i), Children: kids}
	}
	shad2bBigTreeCache = out
	return out
}

// shad2bVariant registers p rendered under another theme.
func shad2bVariant(p gallery.Page, suffix, title string, mk func() *theme.Theme) {
	inter := p.Interact
	if inter == nil {
		inter = p.Setup
	}
	v := p
	v.Variant = true
	v.Name, v.Title = p.Name+suffix, p.Title+title
	v.Interact = nil
	v.Setup = func(d *ui.Driver) {
		t := mk()
		t.BgLayout = t.BgContainer
		d.SetTheme(t)
		d.Settle()
		if inter != nil {
			inter(d)
		}
	}
	gallery.Register(v)
}
