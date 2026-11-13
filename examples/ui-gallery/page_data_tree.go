package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

func treeDemoNodes() []ui.TreeNode {
	return []ui.TreeNode{
		{Key: "0-0", Title: "parent 1", Children: []ui.TreeNode{
			{Key: "0-0-0", Title: "parent 1-0", Children: []ui.TreeNode{
				{Key: "0-0-0-0", Title: "leaf"},
				{Key: "0-0-0-1", Title: "leaf", Disabled: true},
				{Key: "0-0-0-2", Title: "leaf"},
			}},
			{Key: "0-0-1", Title: "parent 1-1", Children: []ui.TreeNode{
				{Key: "0-0-1-0", Title: "sss"},
			}},
			{Key: "0-0-2", Title: "parent 1-2", Children: []ui.TreeNode{
				{Key: "0-0-2-0", Title: "leaf"},
				{Key: "0-0-2-1", Title: "leaf"},
			}},
		}},
	}
}

func treeSearchNodes() []ui.TreeNode {
	return []ui.TreeNode{
		{Key: "a", Title: "浙江省", Children: []ui.TreeNode{
			{Key: "a1", Title: "杭州市", Children: []ui.TreeNode{
				{Key: "a11", Title: "西湖区"}, {Key: "a12", Title: "滨江区"}, {Key: "a13", Title: "余杭区"},
			}},
			{Key: "a2", Title: "宁波市", Children: []ui.TreeNode{{Key: "a21", Title: "海曙区"}}},
		}},
		{Key: "b", Title: "江苏省", Children: []ui.TreeNode{
			{Key: "b1", Title: "南京市", Children: []ui.TreeNode{{Key: "b11", Title: "玄武区"}}},
		}},
	}
}

var (
	treeExp1     = []string{"0-0", "0-0-0", "0-0-1"}
	treeSel1     = []string{"0-0-1"}
	treeChk1     = []string{"0-0-0-0", "0-0-1"}
	treeExp2     = []string{"0-0", "0-0-0", "0-0-2"}
	treeSel2     = []string{"0-0-0-0"}
	treeExp3     []string
	treeSearchKw = "区"
	treeLazy     = []ui.TreeNode{
		{Key: "l1", Title: "展开以加载"},
		{Key: "l2", Title: "已加载", Children: []ui.TreeNode{{Key: "l21", Title: "子节点", IsLeaf: true}}},
		{Key: "l3", Title: "叶子节点", IsLeaf: true},
	}
	treeLazyExp = []string{"l1", "l2"}
)

func init() {
	dataPage(gallery.Page{Name: "Tree", Title: "Tree 树形控件", Order: 20, Width: 1000, Render: func(c *ui.Context) {
		ui.Row(c, func() {
			gallery.Demo(c, "可勾选（含半选）", "勾选父节点会联动子节点，部分子节点勾选时父节点半选。", func() {
				ui.Tree(c, treeDemoNodes()).Expanded(&treeExp1).Selected(&treeSel1).Checkable(&treeChk1).Show()
			})
			gallery.Demo(c, "连接线 + 图标", "ShowLine 显示连接线，ShowIcon 显示图标。", func() {
				ui.Tree(c, treeDemoNodes()).Expanded(&treeExp2).Selected(&treeSel2).ShowLine().ShowIcon().Show()
			})
		}, layout.Gap(16), layout.AlignItems(layout.AlignStart))
		ui.Row(c, func() {
			gallery.Demo(c, "搜索高亮", "关键字“区”：自动展开匹配节点的父节点并高亮。", func() {
				ui.Input(c, &treeSearchKw).Placeholder("搜索").Layout(layout.Margin(geom.Insets{Bottom: 8}))
				ui.Tree(c, treeSearchNodes()).Expanded(&treeExp3).Search(treeSearchKw).Draggable().BlockNode().Show()
			})
			gallery.Demo(c, "异步加载", "展开没有子节点的节点时调用 LoadData，加载中显示转圈。", func() {
				ui.Tree(c, treeLazy).Expanded(&treeLazyExp).LoadData(func(string) bool { return true }).Show()
			})
		}, layout.Gap(16), layout.AlignItems(layout.AlignStart))
	}})
}
