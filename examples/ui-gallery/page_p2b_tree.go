package main

import (
	"fmt"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
)

// Cascader / TreeSelect: lazy loading, checked strategies, virtual tree
// dropdowns.

var (
	p2bCasLazy  []string
	p2bCasChild = [][]string{{"zj", "hz", "xh"}, {"zj", "hz", "gs"}, {"zj", "hz", "bj"}, {"js", "nj", "xw"}}
	p2bCasAll   = [][]string{{"zj", "hz", "xh"}, {"zj", "hz", "gs"}, {"zj", "hz", "bj"}}
	p2bTsLazy   = ""
	p2bTsBig    = "n3-120"
	p2bTsParent = []string{"xh", "yh", "nb", "nj"}
	p2bTsLazyN  = []ui.TreeSelectNode{{Label: "浙江省", Value: "zj"}, {Label: "江苏省", Value: "js"}, {Label: "香港特别行政区", Value: "hk", IsLeaf: true}}
)

var p2bCasRoots = []ui.CascaderOption{{Label: "浙江", Value: "zj"}, {Label: "江苏", Value: "js"}, {Label: "台湾", Value: "tw", IsLeaf: true}}

var p2bTsBigTree = func() []ui.TreeSelectNode {
	out := make([]ui.TreeSelectNode, 20)
	for g := range out {
		kids := make([]ui.TreeSelectNode, 500)
		for i := range kids {
			v := fmt.Sprintf("n%d-%d", g, i)
			kids[i] = ui.TreeSelectNode{Label: fmt.Sprintf("节点 %d-%03d", g, i), Value: v}
		}
		out[g] = ui.TreeSelectNode{Label: fmt.Sprintf("分组 %02d", g), Value: fmt.Sprint("g", g), Children: kids}
	}
	return out
}()

func init() {
	entryPage(gallery.Page{Name: "CascaderAdvanced", Parent: "Cascader", Title: "Cascader / TreeSelect 懒加载、勾选策略与虚拟滚动", Order: 65, Width: 1240, Height: 900,
		Render: func(c *ui.Context) {
			entryRow(c, func() {
				p2bSelDemo(c, "Cascader 懒加载（加载中）", 300, func() {
					ui.Cascader(c, &p2bCasLazy, p2bCasRoots).Placeholder("请选择地区").
						LoadData(func(path []string, done func([]ui.CascaderOption)) {}). // never answers: shows the spinner
						Open().Expand("zj")
				})
				p2bSelDemo(c, "多选 ShowChild（逐个叶子）", 300, func() {
					ui.Cascader(c, nil, selRegions).Multiple(&p2bCasChild).ShowCheckedStrategy(ui.ShowChild).MaxTagCount(3).A11yLabel("地区（子节点）")
					ui.Text(c, "多选 ShowAll（父与子）").Tertiary()
					ui.Cascader(c, nil, selRegions).Multiple(&p2bCasAll).ShowCheckedStrategy(ui.ShowAll).A11yLabel("地区（全部）")
					ui.Text(c, "多选 ShowParent（默认）").Tertiary()
					ui.Cascader(c, nil, selRegions).Multiple(&p2bCasAll).A11yLabel("地区（父节点）")
				})
			})
			ui.Gap(c, 40)
			entryRow(c, func() {
				p2bSelDemo(c, "TreeSelect 懒加载（展开时加载）", 260, func() {
					ui.TreeSelect(c, &p2bTsLazy, p2bTsLazyN).A11yLabel("懒加载城市").LoadData(func(v string) bool { return true }).Open()
				})
				p2bSelDemo(c, "1 万节点（虚拟滚动）", 260, func() {
					ui.TreeSelect(c, &p2bTsBig, p2bTsBigTree).DefaultExpandAll().A11yLabel("大量节点").Open()
				})
				p2bSelDemo(c, "勾选 ShowParent", 320, func() {
					ui.TreeSelect(c, nil, miscTree).Multiple(&p2bTsParent).Checkable().ShowCheckedStrategy(ui.ShowParent).Width(320).A11yLabel("城市（父节点）").Open()
				})
			})
		}})
}
