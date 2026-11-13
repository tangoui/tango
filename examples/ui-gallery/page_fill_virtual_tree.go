package main

import (
	"fmt"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

// fillVTNodes builds 10 × 100 × 100 = 100,000 leaves under 1,010 parents.
func fillVTNodes() []ui.TreeNode {
	out := make([]ui.TreeNode, 10)
	for a := range out {
		ka := fmt.Sprint(a + 1)
		out[a] = ui.TreeNode{Key: ka, Title: "区域 " + ka, Children: make([]ui.TreeNode, 100)}
		for b := range out[a].Children {
			kb := fmt.Sprintf("%s-%d", ka, b+1)
			n := ui.TreeNode{Key: kb, Title: "门店 " + kb, Children: make([]ui.TreeNode, 100)}
			for c := range n.Children {
				kc := fmt.Sprintf("%s-%d", kb, c+1)
				n.Children[c] = ui.TreeNode{Key: kc, Title: "设备 " + kc, Disabled: c == 7}
			}
			out[a].Children[b] = n
		}
	}
	return out
}

var (
	fillVTData []ui.TreeNode
	fillVTDemo struct {
		exp, chk []string
		cur      string
		query    string
		lazy     []ui.TreeNode
		lazyExp  []string
		strict   []string
		strictE  []string
		loadAt   map[string]int
	}
)

func fillVTReset() {
	if fillVTData == nil {
		fillVTData = fillVTNodes()
	}
	d := &fillVTDemo
	d.exp = []string{"1", "1-1", "1-2"}
	d.chk = []string{"1-1-2", "1-1-3", "1-2"}
	d.cur = "1-1-1"
	d.query = ""
	d.lazy = []ui.TreeNode{{Key: "r1", Title: "展开以加载"}, {Key: "r2", Title: "另一个目录"}, {Key: "r3", Title: "叶子", IsLeaf: true}}
	d.lazyExp = nil
	d.strict = []string{"s1"}
	d.strictE = []string{"s"}
	d.loadAt = map[string]int{}
}

func init() {
	fillVTReset()
	morePage(gallery.Page{Name: "VirtualTree", Title: "VirtualTree 虚拟化树", Group: gallery.Display, Order: 21, Height: 900,
		Render: func(c *ui.Context) {
			d := &fillVTDemo
			gallery.Demo(c, "十万节点", "只构建视口内的行；勾选含半选联动，键盘上下左右导航，过滤时显示匹配路径。", func() {
				ui.Search(c, &d.query).Placeholder("过滤节点，如 3-40-7").Layout(layout.W(280), layout.Margin(geomBottom(12)))
				t := ui.VirtualTree(c, fillVTData).Expanded(&d.exp).Checkable(&d.chk).Current(&d.cur).Filter(d.query).Height(320).Show()
				ui.Text(c, fmt.Sprintf("可见 %d 行，本帧构建 %d 行；已勾选 %d 个，半选 %d 个", t.VisibleCount(), rangeLen(t.Range()), len(d.chk), len(t.HalfChecked()))).Tertiary()
			})
			ui.Row(c, func() {
				ui.Column(c, func() {
					gallery.Demo(c, "懒加载", "", func() {
						ui.VirtualTree(c, d.lazy).Expanded(&d.lazyExp).LoadData(func(key string) bool {
							d.loadAt[key]++
							if d.loadAt[key] < 30 {
								return true
							}
							for i := range d.lazy {
								if d.lazy[i].Key == key && len(d.lazy[i].Children) == 0 {
									d.lazy[i].Children = []ui.TreeNode{{Key: key + "-a", Title: "子节点 A", IsLeaf: true}, {Key: key + "-b", Title: "子节点 B", IsLeaf: true}}
								}
							}
							return false
						}).Height(150).Show()
					})
				}, layout.Grow(1))
				ui.Column(c, func() {
					gallery.Demo(c, "CheckStrictly 与自定义节点", "", func() {
						nodes := []ui.TreeNode{{Key: "s", Title: "父节点", Children: []ui.TreeNode{{Key: "s1", Title: "子节点 1"}, {Key: "s2", Title: "子节点 2"}}}}
						ui.VirtualTree(c, nodes).Expanded(&d.strictE).Checkable(&d.strict).CheckStrictly().NodeRender(func(n *ui.TreeNode, depth int) {
							ic := icon.FileText
							if len(n.Children) > 0 {
								ic = icon.Folder
							}
							ui.Icon(c, ic).Size(14)
							ui.Text(c, n.Title).Layout(layout.Margin(geomLeft(6)))
							ui.Spacer(c)
							ui.Tag(c, fmt.Sprintf("L%d", depth))
						}).Height(150).Show()
					})
				}, layout.Grow(1))
			}, layout.Gap(16))
		},
		Setup: func(d *ui.Driver) {
			d.ClickLabel("设备 1-1-4")
			d.Settle()
		},
	}, fillVTReset)
}

func rangeLen(a, b int) int { return b - a }
