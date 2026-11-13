package main

import (
	"strings"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
)

// TreeSelect / Cascader: value by checked strategy (StrategyValue) and
// controlled expansion (ExpandedKeys).

var (
	shadTsStrat   = []string{"hz", "nj"}
	shadTsKeys    = []string{"zj"}
	shadTsKeyVal  = ""
	shadCasStrat  = [][]string{{"zj", "hz"}}
	shadCasStratC = [][]string{{"zj", "hz", "xh"}, {"zj", "hz", "gs"}, {"zj", "hz", "bj"}}
)

func init() {
	entryPage(gallery.Page{Name: "TreeSelectStrategy", Parent: "TreeSelect", Title: "TreeSelect / Cascader 按勾选策略取值与受控展开", Order: 72, Width: 1240, Height: 640,
		Render: func(c *ui.Context) {
			entryRow(c, func() {
				p2bSelDemo(c, "StrategyValue + ShowParent（值："+strings.Join(shadTsStrat, ", ")+"）", 320, func() {
					ui.TreeSelect(c, nil, miscTree).Multiple(&shadTsStrat).Checkable().ShowCheckedStrategy(ui.ShowParent).StrategyValue(true).
						DefaultExpandAll().Width(320).A11yLabel("城市（按策略取值）").Open()
				})
				p2bSelDemo(c, "ExpandedKeys（受控："+strings.Join(shadTsKeys, ", ")+"）", 260, func() {
					ui.TreeSelect(c, &shadTsKeyVal, miscTree).ExpandedKeys(&shadTsKeys).A11yLabel("受控展开").Open()
				})
			})
			ui.Gap(c, 330)
			entryRow(c, func() {
				p2bSelDemo(c, "Cascader StrategyValue（值存父路径）", 320, func() {
					ui.Cascader(c, nil, selRegions).Multiple(&shadCasStrat).StrategyValue(true).A11yLabel("地区（按策略取值）")
					ui.Text(c, "值："+shadCasPaths(shadCasStrat)).Tertiary()
				})
				p2bSelDemo(c, "默认（值存叶子路径）", 320, func() {
					ui.Cascader(c, nil, selRegions).Multiple(&shadCasStratC).A11yLabel("地区（叶子取值）")
					ui.Text(c, "值："+shadCasPaths(shadCasStratC)).Tertiary()
				})
			})
		}})
}

func shadCasPaths(ps [][]string) string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = strings.Join(p, "/")
	}
	return strings.Join(out, ", ")
}
