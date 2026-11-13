package main

import (
	"fmt"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

// tbNode is a tree record.
type tbNode struct {
	name, size, kind string
	kids             []int
	lazy             bool // children load on demand
}

func init() {
	tbPage("TableTree", "Table 表格 · 展开行与树形数据", 105, 1080, 0, false, func() (func(*ui.Context), func(*ui.Driver)) {
		data := tbPeople(5)
		expanded := []string{"1"}
		nodes := []tbNode{
			{name: "src", size: "-", kind: "目录", kids: []int{1, 2, 5}},
			{name: "main.go", size: "4 KB", kind: "Go 源文件"},
			{name: "ui", size: "-", kind: "目录", kids: []int{3, 4}},
			{name: "table.go", size: "32 KB", kind: "Go 源文件"},
			{name: "table_test.go", size: "18 KB", kind: "Go 测试"},
			{name: "assets（懒加载）", size: "-", kind: "目录", lazy: true},
			{name: "README.md", size: "2 KB", kind: "Markdown"},
			{name: "logo.png", size: "12 KB", kind: "图片"},
			{name: "icon.svg", size: "3 KB", kind: "图片"},
		}
		open := []string{"0", "2"}
		loads := 0
		return func(c *ui.Context) {
				cols := []ui.TableColumn{
					{Key: "name", Title: "姓名", Width: 140, Text: func(i int) string { return data[i].name }},
					{Key: "age", Title: "年龄", Width: 90, Text: func(i int) string { return fmt.Sprint(data[i].age) }},
					{Key: "addr", Title: "住址", Text: func(i int) string { return data[i].addr }},
					{Key: "op", Title: "操作", Width: 100, Render: func(i int) { ui.Link(c, "删除") }},
				}
				tbDemo(c, "可展开行", "Expandable 展开详情；RowExpandable 控制哪些行可展开（第 3 行不可展开）。", func() {
					ui.Table(c, cols, len(data)).
						Expandable(func(i int) {
							ui.Descriptions(c, []ui.DescItem{
								{Label: "姓名", Value: data[i].name}, {Label: "城市", Value: data[i].city}, {Label: "部门", Value: data[i].dept},
								{Label: "住址", Value: data[i].addr, Span: 2}, {Label: "金额", Value: tbMoney(data[i].amount)},
							})
						}).
						ExpandedKeys(&expanded).
						RowExpandable(func(i int) bool { return i != 2 }).
						Size(ui.Middle).Show()
				})
				tbDemo(c, "树形数据 · 懒加载子节点", "Tree 指定根与子节点；TreeLazy 在展开时加载（加载中显示转圈）。", func() {
					tc := []ui.TableColumn{
						{Key: "name", Title: "名称", Text: func(i int) string { return nodes[i].name }},
						{Key: "size", Title: "大小", Width: 120, Align: ui.AlignRight, Text: func(i int) string { return nodes[i].size }},
						{Key: "kind", Title: "类型", Width: 160, Text: func(i int) string { return nodes[i].kind }},
					}
					ui.Table(c, tc, len(nodes)).
						Tree([]int{0, 6}, func(i int) []int { return nodes[i].kids }).
						TreeLazy(func(i int) bool { return nodes[i].lazy }, func(i int) {
							loads++
							if loads > 1 { // the second request "arrives"
								nodes[i].kids = []int{7, 8}
							}
						}).
						TreeOpenKeys(&open).
						Size(ui.Middle).Show()
				})
				_ = layout.Grow
			}, func(d *ui.Driver) {
				d.ClickLabel("tree:5")
			}
	})
}
