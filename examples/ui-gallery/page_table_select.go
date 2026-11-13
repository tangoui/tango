package main

import (
	"fmt"
	"strings"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

func init() {
	tbPage("TableSelect", "Table 表格 · 选择与操作", 102, 1080, 0, false, func() (func(*ui.Context), func(*ui.Driver)) {
		data := tbPeople(46)
		sel := []string{"r2", "r5", "r13"} // r13 lives on page 2: kept across pages
		page, size := 1, 5
		single := []int{1}
		var clicked = -1
		return func(c *ui.Context) {
				cols := []ui.TableColumn{
					{Key: "name", Title: "姓名", Width: 120, Text: func(i int) string { return data[i].name }},
					{Key: "age", Title: "年龄", Width: 80, Text: func(i int) string { return fmt.Sprint(data[i].age) }},
					{Key: "dept", Title: "部门", Width: 100, Text: func(i int) string { return data[i].dept }},
					{Key: "addr", Title: "住址", Text: func(i int) string { return data[i].addr }},
					{Key: "status", Title: "状态", Width: 110, Render: func(i int) {
						ui.Badge(c).Status(tbStatusBadge[data[i].status], tbStatusText[data[i].status])
					}},
				}
				tbDemo(c, "多选 · 全选/半选 · 禁用行 · 跨页保留", "状态为「关闭」的行不可选；表头复选框只作用于当前页可选行；翻页后其他页的选择保留（共 46 行，每页 5 行）。", func() {
					ui.Row(c, func() {
						ui.Text(c, fmt.Sprintf("已选择 %d 项：%s", len(sel), strings.Join(sel, ", ")))
						ui.Spacer(c)
						if ui.Button(c, "清空选择").Small().Disabled(len(sel) == 0).Clicked() {
							sel = sel[:0]
						}
					}, layout.Middle(), layout.Margin(geomBottom(12)))
					ui.Table(c, cols, len(data)).
						RowKey(func(i int) string { return fmt.Sprint("r", data[i].id) }).
						SelectedKeys(&sel).
						SelectDisabled(func(i int) bool { return data[i].status == "closed" }).
						Pagination(&page, &size).
						Size(ui.Middle).
						Show()
				})
				tbDemo(c, "单选 · 点击行 · 当前行高亮", "SingleSelect 为单选；HighlightCurrentRow 点击行后保持高亮（Element 的 highlight-current-row）。", func() {
					t := ui.Table(c, cols[:4], 4).Selection(&single).SingleSelect().HighlightCurrentRow().Size(ui.Middle).Show()
					if r := t.RowClicked(); r >= 0 {
						clicked = r
					}
					tbGap(c, 8)
					msg := "点击任意行"
					if clicked >= 0 {
						msg = "点击了：" + data[clicked].name
					}
					ui.Text(c, msg).Tertiary()
				})
			}, func(d *ui.Driver) {
				if r, ok := d.Find("row:3"); ok {
					p := r.Center()
					p.X = r.X + 300
					d.Click(p)
				}
				if r, ok := d.Find("row:r4"); ok {
					d.Move(r.Center())
				}
			}
	})
}
