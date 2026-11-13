package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

func init() {
	tbPage("TableEdit", "Table 表格 · 可编辑与拖拽排序", 107, 1080, 0, false, func() (func(*ui.Context), func(*ui.Driver)) {
		data := tbPeople(5)
		editing := []string{"e1"}
		order := []int{0, 1, 2, 3, 4}
		deptOpts := ui.Opts(tbDepts...)
		return func(c *ui.Context) {
				var tbl *ui.TableW
				required := func(v string) string {
					if strings.TrimSpace(v) == "" {
						return "此项为必填项"
					}
					return ""
				}
				cols := []ui.TableColumn{
					{Key: "name", Title: "姓名（文本）", Width: 170, Text: func(i int) string { return data[i].name },
						Editor: &ui.TableEditor{Set: func(i int, v string) { data[i].name = v }, Validate: required}},
					{Key: "age", Title: "年龄（数字）", Width: 150, Align: ui.AlignRight, Text: func(i int) string { return fmt.Sprint(data[i].age) },
						Editor: &ui.TableEditor{Kind: "number", Set: func(i int, v string) { data[i].age, _ = strconv.Atoi(v) },
							Validate: func(v string) string {
								if n, _ := strconv.Atoi(v); n < 18 || n > 70 {
									return "年龄需在 18–70 之间"
								}
								return ""
							}}},
					{Key: "dept", Title: "部门（下拉）", Width: 170, Text: func(i int) string { return data[i].dept },
						Editor: &ui.TableEditor{Kind: "select", Options: deptOpts, Set: func(i int, v string) { data[i].dept = v }}},
					{Key: "date", Title: "入职日期（日期）", Width: 200, Text: func(i int) string { return data[i].created.Format("2006-01-02") },
						Editor: &ui.TableEditor{Kind: "date", Set: func(i int, v string) {}}},
					{Key: "op", Title: "操作", Render: func(i int) {
						if tbl.IsEditing(i) {
							if ui.Link(c, "保存").Clicked() {
								tbl.Save(i)
							}
							if ui.Link(c, "取消").Clicked() {
								tbl.Cancel(i)
							}
							return
						}
						if ui.Link(c, "编辑").Clicked() {
							tbl.Edit(i)
						}
					}},
				}
				tbDemo(c, "单元格编辑 · 整行编辑", "点击单元格进入编辑：回车提交、Esc 取消、失焦提交，校验失败显示提示；第二行处于整行编辑模式（保存/取消）。", func() {
					tbl = ui.Table(c, cols, len(data)).RowKey(func(i int) string { return fmt.Sprint("e", i) }).EditableRows(&editing).Bordered()
					tbl.Show()
				})
				tbDemo(c, "拖拽排序", "按住左侧把手拖动行，松开后通过回调重排数据。", func() {
					dc := []ui.TableColumn{
						{Key: "name", Title: "姓名", Width: 160, Text: func(i int) string { return data[order[i]].name }},
						{Key: "dept", Title: "部门", Width: 160, Text: func(i int) string { return data[order[i]].dept }},
						{Key: "addr", Title: "住址", Text: func(i int) string { return data[order[i]].addr }},
					}
					ui.Table(c, dc, len(order)).Size(ui.Middle).DragSort(func(from, to int) {
						v := order[from]
						order = append(order[:from], order[from+1:]...)
						order = append(order[:to], append([]int{v}, order[to:]...)...)
					}).Show()
				})
				_ = layout.Grow
			}, func(d *ui.Driver) {
				// Start editing the first name, clear it and submit: the
				// validation error stays visible.
				if r, ok := d.Find("row:e0"); ok {
					p := r.Center()
					p.X = r.X + 60
					d.Click(p)
					for i := 0; i < 4; i++ {
						d.Key(input.KeyBackspace, 0)
					}
					d.Key(input.KeyEnter, 0)
				}
			}
	})
}
