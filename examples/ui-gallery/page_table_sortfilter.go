package main

import (
	"fmt"
	"strings"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

func init() {
	tbPage("TableSortFilter", "Table 表格 · 排序与筛选", 103, 1080, 0, false, func() (func(*ui.Context), func(*ui.Driver)) {
		data := tbPeople(8)
		var sort1 ui.SortState // controlled single sort
		var multi []ui.SortState
		filters := map[string][]string{}
		minAge := ""
		return func(c *ui.Context) {
				cityOpts := []ui.Option{}
				for _, ct := range tbCities {
					cityOpts = append(cityOpts, ui.Option{Label: ct, Value: ct})
				}
				cols := []ui.TableColumn{
					{Key: "name", Title: "姓名", Width: 120, Text: func(i int) string { return data[i].name }},
					{Key: "age", Title: "年龄", Width: 110, Align: ui.AlignRight,
						Compare: func(a, b int) int { return tbCmp(data[a].age, data[b].age) },
						Text:    func(i int) string { return fmt.Sprint(data[i].age) },
						FilterPanel: func(p *ui.TableFilterPanel) {
							c := p.Context()
							ui.Column(c, func() {
								ui.Text(c, "最小年龄")
								ui.Input(c, &minAge).Placeholder("如 30").Width(160)
								ui.Row(c, func() {
									if ui.Button(c, "重置").Small().Clicked() {
										minAge = ""
										p.Reset()
									}
									if ui.Button(c, "筛选").Primary().Small().Clicked() {
										*p.Values = []string{minAge}
										p.Confirm()
									}
								}, layout.Gap(8), layout.End())
							}, layout.Gap(8))
						},
						OnFilter: func(i int, v string) bool {
							var n int
							fmt.Sscan(v, &n)
							return data[i].age >= n
						}},
					{Key: "city", Title: "城市", Width: 120, Filters: cityOpts, FilterSearch: true,
						OnFilter: func(i int, v string) bool { return data[i].city == v },
						Text:     func(i int) string { return data[i].city }},
					{Key: "status", Title: "状态", Width: 120, FilterSingle: true,
						Filters:  []ui.Option{{Label: "运行中", Value: "running"}, {Label: "已上线", Value: "online"}, {Label: "关闭", Value: "closed"}, {Label: "异常", Value: "error"}},
						OnFilter: func(i int, v string) bool { return data[i].status == v },
						Render:   func(i int) { ui.Badge(c).Status(tbStatusBadge[data[i].status], tbStatusText[data[i].status]) }},
					{Key: "addr", Title: "住址", Text: func(i int) string { return data[i].addr }},
				}
				tbDemo(c, "单列排序（受控）· 下拉筛选 · 搜索 · 自定义筛选面板", "点击表头循环 升序 → 降序 → 取消；城市列带搜索，状态列单选，年龄列为自定义面板。", func() {
					t := ui.Table(c, cols, len(data)).Sort(&sort1).Filters(&filters).Size(ui.Middle).Show()
					rows := t.DisplayedRows()
					names := make([]string, len(rows))
					for i, r := range rows {
						names[i] = data[r].name
					}
					tbGap(c, 8)
					ui.Text(c, fmt.Sprintf("排序：%s %d；筛选：%v；显示 %d 行", sort1.Column, sort1.Order, filters, len(rows))).Tertiary()
				})
				tbDemo(c, "多列排序", "SortMultiple 越大优先级越高：语文 > 数学 > 英语。", func() {
					score := func(key, title string, prio int, v func(p tbPerson) int) ui.TableColumn {
						return ui.TableColumn{Key: key, Title: title, Width: 130, Align: ui.AlignRight, SortMultiple: prio,
							Compare: func(a, b int) int { return tbCmp(v(data[a]), v(data[b])) },
							Text:    func(i int) string { return fmt.Sprint(v(data[i])) }}
					}
					mc := []ui.TableColumn{
						{Key: "name", Title: "姓名", Text: func(i int) string { return data[i].name }},
						score("chinese", "语文", 3, func(p tbPerson) int { return p.chinese / 10 * 10 }),
						score("math", "数学", 2, func(p tbPerson) int { return p.math / 10 * 10 }),
						score("english", "英语", 1, func(p tbPerson) int { return p.english }),
					}
					ui.Table(c, mc, len(data)).MultiSort(&multi).Size(ui.Small).Show()
					var parts []string
					for _, s := range multi {
						parts = append(parts, fmt.Sprintf("%s:%d", s.Column, s.Order))
					}
					tbGap(c, 8)
					ui.Text(c, "当前排序："+strings.Join(parts, " , ")).Tertiary()
				})
			}, func(d *ui.Driver) {
				d.ClickLabel("年龄")
				d.ClickLabel("语文")
				d.ClickLabel("数学")
				d.ClickLabel("数学")
				d.ClickLabel("city")
				d.ClickLabel("杭州")
				d.ClickLabel("上海")
			}
	})
}
