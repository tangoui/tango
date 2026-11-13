package main

import (
	"fmt"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

// Transfer: custom rows, pagination, one-way, tree source, virtual lists.

var (
	p2bTfPage  = []string{"3", "7"}
	p2bTfOne   = []string{"k2", "k5"}
	p2bTfTree  = []string{"react"}
	p2bTfBig   []string
	p2bTfItems = func() []ui.TransferItem {
		out := make([]ui.TransferItem, 23)
		for i := range out {
			out[i] = ui.TransferItem{Key: fmt.Sprint(i), Title: fmt.Sprintf("内容 %02d", i+1), Disabled: i == 4}
		}
		return out
	}()
	p2bTfMany = func() []ui.TransferItem {
		out := make([]ui.TransferItem, 2000)
		for i := range out {
			out[i] = ui.TransferItem{Key: fmt.Sprint(i), Title: fmt.Sprintf("用户 %04d", i)}
		}
		return out
	}()
	p2bTfTreeNodes = []ui.TreeNode{
		{Key: "fe", Title: "前端", Children: []ui.TreeNode{{Key: "vue", Title: "Vue"}, {Key: "react", Title: "React"}, {Key: "svelte", Title: "Svelte"}}},
		{Key: "be", Title: "后端", Children: []ui.TreeNode{{Key: "go", Title: "Go"}, {Key: "java", Title: "Java"}}},
		{Key: "ops", Title: "运维"},
	}
)

func init() {
	oneItems := []ui.TransferItem{}
	for i, t := range []string{"内容一", "内容二", "内容三", "内容四", "内容五", "内容六"} {
		oneItems = append(oneItems, ui.TransferItem{Key: fmt.Sprint("k", i+1), Title: t})
	}
	entryPage(gallery.Page{Name: "TransferAdvanced", Parent: "Transfer", Title: "Transfer 自定义渲染 / 分页 / 单向 / 树穿梭 / 虚拟列表", Order: 72, Width: 1120, Height: 900,
		Render: func(c *ui.Context) {
			entryRow(c, func() {
				entryCol(c, func() {
					ui.Text(c, "分页（每页 6 条）+ 自定义渲染").Tertiary()
					ui.Transfer(c, p2bTfItems, &p2bTfPage).Pagination(6).ListSize(220, 300).Render(func(o ui.Option) {
						ui.Row(c, func() {
							ui.Icon(c, icon.User).Size(14)
							ui.Text(c, o.Label)
							ui.Text(c, "#"+o.Value).Tertiary().Size(12)
						}, layout.Gap(6), layout.Middle())
					})
				})
				entryCol(c, func() {
					ui.Text(c, "单向（右侧可删除）").Tertiary()
					ui.Transfer(c, oneItems, &p2bTfOne).OneWay().ListSize(200, 300)
				})
			})
			ui.Gap(c, 24)
			entryRow(c, func() {
				entryCol(c, func() {
					ui.Text(c, "树穿梭框（TreeSource）").Tertiary()
					ui.Transfer(c, nil, &p2bTfTree).TreeSource(p2bTfTreeNodes).Titles("全部技术栈", "已选").ListSize(220, 300).DefaultChecked("go")
				})
				entryCol(c, func() {
					ui.Text(c, "2000 条（虚拟列表）+ 搜索").Tertiary()
					ui.Transfer(c, p2bTfMany, &p2bTfBig).Searchable().ListSize(220, 300)
				})
			})
		}})
}
