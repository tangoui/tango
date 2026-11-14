package main

// Spec alignment batch 2, group F (docs/specs/align/2-data.md): Collapse
// collapsible / expandIconPlacement / ghost / sizes, Card variant / inner
// / hover, Descriptions colon / filled / sizes, List vertical layout /
// small size / pager position, Listy group headers and Tree selectable /
// accordion / check on click / switcher icon.

import (
	"fmt"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

var (
	b2fColl1, b2fColl2, b2fColl3, b2fColl4 = []string{"1"}, []string{"2"}, []string{"1"}, []string{"1"}
	b2fListPage                            = 2
	b2fTreeExp                             = []string{"0-0"}
	b2fTreeChk                             []string
	b2fTreeSel                             []string
	b2fVTreeExp                            = []string{"0-0"}
)

func b2fCollItems(c *ui.Context) []ui.CollapseItem {
	body := func() { ui.Text(c, b2fBody) }
	return []ui.CollapseItem{
		{Key: "1", Label: "This is panel header 1", Body: body},
		{Key: "2", Label: "This is panel header 2", Body: body, Extra: func() { ui.Icon(c, icon.Setting).Size(14) }},
		{Key: "3", Label: "This is panel header 3", Body: body, Collapsible: "disabled"},
	}
}

func b2fTreeData() []ui.TreeNode {
	return []ui.TreeNode{
		{Key: "0-0", Title: "parent 1", Children: []ui.TreeNode{
			{Key: "0-0-0", Title: "parent 1-0", Children: []ui.TreeNode{{Key: "0-0-0-0", Title: "leaf"}, {Key: "0-0-0-1", Title: "leaf 2"}}},
			{Key: "0-0-1", Title: "parent 1-1", Children: []ui.TreeNode{{Key: "0-0-1-0", Title: "sss"}}},
		}},
		{Key: "0-1", Title: "parent 2", Children: []ui.TreeNode{{Key: "0-1-0", Title: "leaf 3"}}},
	}
}

// b2fText is a short panel body (the long shared text widens side-by-side demos).
const b2fBody = "A dog is a type of domesticated animal."

func init() {
	gallery.Register(gallery.Page{Name: "CollapseAlign", Title: "Collapse 规范对齐", Group: gallery.Display, Order: 21, Parent: "Collapse",
		Width: 1000, Height: 1100,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "collapsible: header / icon", "只有标题（header）或只有箭头（icon）能展开；第三个面板 disabled。", func() {
				ui.Row(c, func() {
					ui.Collapse(c, &b2fColl1, b2fCollItems(c)).Collapsible("header").Layout(layout.Grow(1), layout.Basis(0), layout.MinW(0)).Show()
					ui.Collapse(c, &b2fColl2, b2fCollItems(c)).Collapsible("icon").Layout(layout.Grow(1), layout.Basis(0), layout.MinW(0)).Show()
				}, layout.Gap(16))
			})
			gallery.Demo(c, "expandIconPlacement=end · 自定义图标 · 小 / 大尺寸", "箭头放在末尾；ExpandIcon 换图标；Small 内容 12、Large 内容 24（antd contentPaddingSM/LG）。", func() {
				ui.Row(c, func() {
					ui.Collapse(c, &b2fColl3, b2fCollItems(c)).ExpandIconPlacement("end").ExpandIcon(icon.Plus).Size(ui.Small).Layout(layout.Grow(1), layout.Basis(0), layout.MinW(0)).Show()
					ui.Collapse(c, &b2fColl4, b2fCollItems(c)).Size(ui.Large).Layout(layout.Grow(1), layout.Basis(0), layout.MinW(0)).Show()
				}, layout.Gap(16), layout.AlignItems(layout.AlignStart))
			})
			gallery.Demo(c, "ghost / borderless", "Ghost 内容上下 12；Borderless 内容 4 16 16。", func() {
				ui.Row(c, func() {
					ui.Collapse(c, nil, b2fCollItems(c)).Ghost().Layout(layout.Grow(1), layout.Basis(0), layout.MinW(0)).Show()
					ui.Collapse(c, nil, b2fCollItems(c)).Borderless().Accordion().Layout(layout.Grow(1), layout.Basis(0), layout.MinW(0)).Show()
				}, layout.Gap(16), layout.AlignItems(layout.AlignStart))
			})
		}})

	gallery.Register(gallery.Page{Name: "CardAlign", Title: "Card 规范对齐", Group: gallery.Display, Order: 4, Parent: "Card",
		Width: 1000, Height: 760,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "variant · type=inner · hoverable", "outlined / borderless（antd 无边框时保留 boxShadowTertiary）；inner 内容 16 24；hoverable 悬停时描边透明并显示 boxShadowCard。", func() {
				ui.Row(c, func() {
					ui.Card(c, "outlined").Extra(func() { ui.Link(c, "More") }).Layout(layout.Grow(1), layout.Basis(0)).Body(func() { ui.Text(c, "Card content") })
					ui.Card(c, "borderless").Variant("borderless").Layout(layout.Grow(1), layout.Basis(0)).Body(func() { ui.Text(c, "Card content") })
					ui.Card(c, "hoverable").Hoverable().Layout(layout.Grow(1), layout.Basis(0)).Body(func() { ui.Text(c, "Card content") })
				}, layout.Gap(16), layout.AlignItems(layout.AlignStart))
			})
			gallery.Demo(c, "inner card", "", func() {
				ui.Card(c, "Card title").Body(func() {
					ui.Card(c, "Inner Card title").Inner().Extra(func() { ui.Link(c, "More") }).Body(func() { ui.Text(c, "Inner Card content") })
				})
			})
		}})

	gallery.Register(gallery.Page{Name: "DescriptionsAlign", Title: "Descriptions 规范对齐", Group: gallery.Display, Order: 5, Parent: "Descriptions",
		Width: 1000, Height: 1000,
		Render: func(c *ui.Context) {
			items := []ui.DescItem{
				{Label: "UserName", Value: "Zhou Maomao"}, {Label: "Telephone", Value: "1810000000"},
				{Label: "Live", Value: "Hangzhou, Zhejiang"}, {Label: "Remark", Value: "empty", Span: 2},
				{Label: "Address", Value: "No. 18, Wantang Road, Xihu District, Hangzhou, Zhejiang, China", Filled: true},
				{Label: "Last", Value: "row has no bottom padding"},
			}
			gallery.Demo(c, "middle · filled · 超出的跨列被截断", "冒号与 8px 间距、最后一行无下边距；Remark span 2、Address span filled。", func() {
				ui.Descriptions(c, items).Title("User Info").Size(ui.Middle)
			})
			gallery.Demo(c, "bordered small", "边框色 colorSplit，标签 colorTextSecondary。", func() {
				ui.Descriptions(c, items).Bordered().Size(ui.Small)
			})
			gallery.Demo(c, "vertical", "竖排：标签带冒号，下边距随尺寸。", func() {
				ui.Descriptions(c, items[:3]).Vertical()
			})
		}})

	gallery.Register(gallery.Page{Name: "ListAlign", Title: "List 规范对齐", Group: gallery.Display, Order: 23, Parent: "List",
		Width: 1000, Height: 980,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "itemLayout=vertical + extra", "动作在内容下方（间距 32），Extra 在右侧一列；标题 16px。", func() {
				ui.List(c, 2).ItemLayout("vertical").Size(ui.Large).ItemExtra(func(i int) {
					ui.Div(c, layout.W(200), layout.H(110)).Bg(c.Theme.FillSecondary).Radius(c.Theme.Radius).Body(func() {})
				}).Item(func(i int) {
					ui.ListItemMeta(c, func() { ui.Avatar(c, "A") }, fmt.Sprintf("ant design part %d", i+1), listDesc)
					ui.Text(c, "We supply a series of design principles, practical patterns and high quality design resources.")
					ui.ListActions(c, func() {
						ui.Text(c, "156").Tertiary()
						ui.Text(c, "2").Tertiary()
					})
				})
			})
			gallery.Demo(c, "small · 分页在上下两侧", "Small 无边框时条目也有 16px 内边距（antd itemPaddingSM）；PaginationPosition(\"both\")、居中。", func() {
				ui.List(c, 6).Size(ui.Small).Header(func() { ui.Text(c, "Header") }).Pagination(&b2fListPage, 2).PaginationPosition("both").PaginationAlign("center").Item(func(i int) {
					ui.Text(c, fmt.Sprintf("Racing car sprays burning fuel into crowd. (%d)", i+1))
				})
			})
		}})

	gallery.Register(gallery.Page{Name: "TreeAlign", Title: "Tree 规范对齐", Group: gallery.Display, Order: 20, Parent: "Tree",
		Width: 1000, Height: 900,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "accordion · 点击叶子勾选 · 自定义展开图标", "Element accordion、check-on-click-leaf；antd switcherIcon。", func() {
				ui.Row(c, func() {
					ui.Tree(c, b2fTreeData()).Expanded(&b2fTreeExp).Checkable(&b2fTreeChk).Accordion().CheckOnClickLeaf(true).
						SwitcherIcon(icon.Down).Layout(layout.Grow(1), layout.Basis(0), layout.MinW(0)).Show()
					ui.Tree(c, b2fTreeData()).Selected(&b2fTreeSel).Selectable(false).DefaultExpandAll().Layout(layout.Grow(1), layout.Basis(0), layout.MinW(0)).Show()
					ui.Tree(c, b2fTreeData()).Disabled(true).DefaultExpandAll().Layout(layout.Grow(1), layout.Basis(0), layout.MinW(0)).Show()
				}, layout.Gap(16), layout.AlignItems(layout.AlignStart))
			})
			gallery.Demo(c, "VirtualTree accordion · 空数据", "el-tree-v2 缩进 16；EmptyText。", func() {
				ui.Row(c, func() {
					ui.Column(c, func() {
						ui.VirtualTree(c, b2fTreeData()).Expanded(&b2fVTreeExp).Accordion().Height(160).Show()
					}, layout.Grow(1), layout.Basis(0))
					ui.Column(c, func() { ui.VirtualTree(c, nil).EmptyText("No Data").Height(160).Show() }, layout.Grow(1), layout.Basis(0))
				}, layout.Gap(16), layout.AlignItems(layout.AlignStart))
			})
		}})
}
