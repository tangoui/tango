package main

// Spec alignment batch 2, group E1: Table appearance and tokens
// (docs/specs/align/2-table-look.md): sorted / hovered headers, sorter and
// filter icons, grouped headers without borders, expand icons and the
// expanded panel, bordered frame, summary and sizes.

import (
	"fmt"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

var (
	b2e1Sort = ui.SortState{Column: "age", Order: ui.SortAsc}
	b2e1Open = []string{"p0"}
)

func init() {
	data := tbPeople(4)
	gallery.Register(gallery.Page{Name: "TableLookAlign", Parent: "Table", Title: "Table 外观与令牌", Group: gallery.Display, Order: 120, Width: 1000,
		Render: func(c *ui.Context) {
			name := ui.TableColumn{Key: "name", Title: "姓名", Width: 120, Text: func(i int) string { return data[i].name }}
			age := ui.TableColumn{Key: "age", Title: "年龄", Width: 110, Sortable: true, Compare: func(a, b int) int { return data[a].age - data[b].age },
				Text: func(i int) string { return fmt.Sprint(data[i].age) }}
			city := ui.TableColumn{Key: "city", Title: "城市", Width: 130, Sortable: true, Compare: func(a, b int) int { return len(data[a].city) - len(data[b].city) },
				Filters:  []ui.Option{{Label: "杭州", Value: "杭州"}, {Label: "上海", Value: "上海"}},
				OnFilter: func(i int, v string) bool { return data[i].city == v }, Text: func(i int) string { return data[i].city }}
			addr := ui.TableColumn{Key: "addr", Title: "住址", Text: func(i int) string { return data[i].addr }}
			amount := ui.TableColumn{Key: "amount", Title: "金额", Width: 120, Align: ui.AlignRight, Text: func(i int) string { return tbMoney(data[i].amount) }}

			gallery.Demo(c, "排序与筛选表头", "已排序列表头与单元格着色（Element / shadcn 不着色）；悬停可排序表头着色、未激活的排序箭头变深；筛选按钮悬停底色与圆角取令牌。", func() {
				ui.Table(c, []ui.TableColumn{name, age, city, addr}, len(data)).Sort(&b2e1Sort).Show()
			})
			gallery.Demo(c, "分组表头（无边框）", "antd：只有叶子列且不是本行最后一格时画短分隔线，分组单元格（非最后一行）不画下边线；Element 分组表头整体用 fill-color-light。", func() {
				ui.Table(c, []ui.TableColumn{name, {Key: "info", Title: "其他", Children: []ui.TableColumn{age, city}}, addr}, 2).Size(ui.Middle).Show()
			})
			gallery.Demo(c, "展开行 · 汇总 · 边框", "展开图标为 17px 盒按 16/17 缩放（圆角 borderRadius）；展开面板底色 rowExpandedBg；带边框表格只圆上角（antd radius.js），shadcn 四角 rounded-md；汇总行字重取 FooterFontWeight。", func() {
				ui.Row(c, func() {
					ui.Column(c, func() {
						ui.Table(c, []ui.TableColumn{name, age, amount}, 2).RowKey(func(i int) string { return fmt.Sprint("p", i) }).
							ExpandedKeys(&b2e1Open).Expandable(func(i int) { ui.Text(c, data[i].addr) }).Size(ui.Middle).Show()
					}, layout.Grow(1), layout.MinW(0))
					ui.Column(c, func() {
						ui.Table(c, []ui.TableColumn{name, age, amount}, 3).Bordered().Size(ui.Small).
							Summary(func(col string, rows []int) string {
								switch col {
								case "name":
									return "合计"
								case "amount":
									s := 0.0
									for _, r := range rows {
										s += data[r].amount
									}
									return tbMoney(s)
								}
								return ""
							}).Show()
					}, layout.Grow(1), layout.MinW(0))
				}, layout.Gap(24))
			})
		},
		Interact: func(d *ui.Driver) {
			// Hover the sortable "城市" header of the first table.
			if r, ok := d.FindText("城市"); ok {
				d.Move(r.Center())
				d.Settle()
			}
		},
	})
}
