package main

import (
	"fmt"
	"strings"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

func init() {
	tbPage("TableBasic", "Table 表格 · 基础", 101, 1080, 0, true, func() (func(*ui.Context), func(*ui.Driver)) {
		data := tbPeople(5)
		return func(c *ui.Context) {
				cols := func(c *ui.Context) []ui.TableColumn {
					return []ui.TableColumn{
						{Key: "name", Title: "姓名", Width: 150, Render: func(i int) {
							ui.Avatar(c, data[i].name[:3]).Size(24)
							ui.Link(c, data[i].name)
						}},
						{Key: "age", Title: "年龄", Width: 80, Align: ui.AlignRight, Text: func(i int) string { return fmt.Sprint(data[i].age) }},
						{Key: "addr", Title: "住址", MinWidth: 160, Ellipsis: true, Text: func(i int) string { return data[i].addr + "（靠近地铁站 " + data[i].city + "东站 C 口）" }},
						{Key: "status", Title: "状态", Width: 110, Render: func(i int) {
							ui.Badge(c).Status(tbStatusBadge[data[i].status], tbStatusText[data[i].status])
						}},
						{Key: "tags", Title: "标签", Width: 180, Render: func(i int) {
							for _, t := range data[i].tags {
								ui.Tag(c, t).Color(tbTagColor(t))
							}
						}},
						{Key: "score", Title: "完成度", Width: 150, Render: func(i int) {
							ui.Progress(c, data[i].score).StrokeWidth(6).Layout(layout.Grow(1), layout.MinW(0))
						}},
						{Key: "op", Title: "操作", Width: 130, Render: func(i int) {
							ui.Link(c, "邀请")
							ui.Link(c, "删除")
						}},
					}
				}
				tbDemo(c, "基本用法", "自定义单元格可放任意组件：头像、链接、徽标、标签、进度条；住址列超长省略，悬停显示全文。", func() {
					ui.Table(c, cols(c), len(data)).RowKey(func(i int) string { return fmt.Sprint("p", data[i].id) }).Show()
				})
				tbDemo(c, "三种尺寸 · 边框 · 斑马纹", "Size(ui.Middle) 中等、Size(ui.Small) 紧凑；Bordered 显示全部边框，Striped 斑马纹。", func() {
					short := []ui.TableColumn{
						{Key: "name", Title: "姓名", Width: 120, Text: func(i int) string { return data[i].name }},
						{Key: "city", Title: "城市", Width: 100, Text: func(i int) string { return data[i].city }},
						{Key: "addr", Title: "住址", Text: func(i int) string { return data[i].addr }},
						{Key: "amount", Title: "金额", Width: 140, Align: ui.AlignRight, Text: func(i int) string { return tbMoney(data[i].amount) }},
					}
					ui.Row(c, func() {
						ui.Column(c, func() {
							ui.Text(c, "中等 · 斑马纹").Tertiary()
							tbGap(c, 8)
							ui.Table(c, short, 3).Size(ui.Middle).Striped().Show()
						}, layout.Grow(1), layout.MinW(0))
						ui.Column(c, func() {
							ui.Text(c, "紧凑 · 边框").Tertiary()
							tbGap(c, 8)
							ui.Table(c, short, 3).Size(ui.Small).Bordered().Show()
						}, layout.Grow(1), layout.MinW(0))
					}, layout.Gap(24))
				})
				tbDemo(c, "空状态 · 加载中", "", func() {
					short := []ui.TableColumn{
						{Key: "name", Title: "姓名", Text: func(i int) string { return data[i].name }},
						{Key: "city", Title: "城市", Text: func(i int) string { return data[i].city }},
						{Key: "addr", Title: "住址", Text: func(i int) string { return data[i].addr }},
					}
					ui.Row(c, func() {
						ui.Column(c, func() { ui.Table(c, short, 0).Size(ui.Middle).Show() }, layout.Grow(1), layout.MinW(0))
						ui.Column(c, func() { ui.Table(c, short, 3).Size(ui.Middle).Loading(true).Show() }, layout.Grow(1), layout.MinW(0))
					}, layout.Gap(24))
				})
			}, func(d *ui.Driver) {
				// Hover a truncated address to show its tooltip.
				if r, ok := d.Find("row:p2"); ok {
					// The address column starts after name (150) and age (80).
					p := r.Center()
					p.X = r.X + 150 + 80 + 60
					d.Move(p)
					d.Settle()
				}
			}
	})
	_ = strings.Repeat
}
