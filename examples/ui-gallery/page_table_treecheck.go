package main

import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
)

func init() {
	tbPage("TableTreeCheck", "Table 表格 · 树形数据勾选联动", 106, 1080, 0, false, func() (func(*ui.Context), func(*ui.Driver)) {
		type org struct {
			name, lead string
			kids       []int
		}
		nodes := []org{
			{name: "总部", lead: "王伟", kids: []int{1, 4}},
			{name: "研发中心", lead: "李芳", kids: []int{2, 3}},
			{name: "前端组", lead: "张娜"},
			{name: "后端组", lead: "刘敏"},
			{name: "市场中心", lead: "陈静", kids: []int{5, 6}},
			{name: "品牌组", lead: "杨丽"},
			{name: "渠道组", lead: "赵强"},
		}
		sel := []int{2}
		sel2 := []int{1}
		open := []string{"0", "1", "4"}
		open2 := []string{"0", "1"}
		cols := []ui.TableColumn{
			{Key: "name", Title: "部门", Text: func(i int) string { return nodes[i].name }},
			{Key: "lead", Title: "负责人", Width: 200, Text: func(i int) string { return nodes[i].lead }},
		}
		return func(c *ui.Context) {
				tbDemo(c, "父子联动（默认）", "勾选父节点会勾选全部子孙；子节点全选时父节点自动勾选，部分勾选时显示半选。", func() {
					ui.Table(c, cols, len(nodes)).Tree([]int{0}, func(i int) []int { return nodes[i].kids }).
						TreeOpenKeys(&open).Selection(&sel).Size(ui.Middle).Show()
				})
				tbDemo(c, "CheckStrictly", "关闭联动：父子节点各自独立勾选。", func() {
					ui.Table(c, cols, len(nodes)).Tree([]int{0}, func(i int) []int { return nodes[i].kids }).
						TreeOpenKeys(&open2).Selection(&sel2).CheckStrictly().Size(ui.Middle).Show()
				})
			}, func(d *ui.Driver) {
				// Check 市场中心: its two children follow.
				if r, ok := d.FindText("市场中心"); ok { // first table
					if t, ok := d.FindText("勾选父节点会勾选全部子孙；子节点全选时父节点自动勾选，部分勾选时显示半选。"); ok {
						d.Click(geom.Pt(t.X+24, r.Y+r.H/2))
					}
				}
			}
	})
}
