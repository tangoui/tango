package main

import (
	"fmt"

	"github.com/tangoui/tango/ui"
)

func init() {
	tbPage("TableSpan", "Table 表格 · 合并单元格与总结栏", 106, 1080, 0, false, func() (func(*ui.Context), func(*ui.Driver)) {
		type sale struct {
			region, city string
			q1, q2, q3   float64
		}
		sales := []sale{
			{"华东", "杭州", 1200, 1350, 1580}, {"华东", "上海", 2300, 2410, 2890}, {"华东", "南京", 980, 1020, 1100},
			{"华北", "北京", 2100, 1990, 2250}, {"华北", "天津", 760, 800, 845},
			{"西南", "成都", 1320, 1400, 1510}, {"西南", "重庆", 1180, 1210, 1300}, {"西南", "昆明", 640, 700, 720},
		}
		data := tbPeople(40)
		return func(c *ui.Context) {
			num := func(v float64) string { return ui.FormatNumber(v, 0) }
			cols := []ui.TableColumn{
				{Key: "region", Title: "大区", Width: 100, Text: func(i int) string { return sales[i].region }},
				{Key: "city", Title: "城市", Width: 120, Text: func(i int) string { return sales[i].city }},
				{Key: "q1", Title: "一季度", Align: ui.AlignRight, Text: func(i int) string { return num(sales[i].q1) }},
				{Key: "q2", Title: "二季度", Align: ui.AlignRight, Text: func(i int) string { return num(sales[i].q2) }},
				{Key: "q3", Title: "三季度", Align: ui.AlignRight, Text: func(i int) string { return num(sales[i].q3) }},
				{Key: "note", Title: "备注", Width: 200, Text: func(i int) string {
					if i == 4 {
						return "跨两列合并的备注"
					}
					return "—"
				}},
			}
			sum := func(key string, rows []int) string {
				var s float64
				for _, r := range rows {
					switch key {
					case "region":
						return "合计"
					case "q1":
						s += sales[r].q1
					case "q2":
						s += sales[r].q2
					case "q3":
						s += sales[r].q3
					default:
						return ""
					}
				}
				return num(s)
			}
			tbDemo(c, "合并单元格（rowSpan / colSpan）· 合计行", "相同大区纵向合并；天津行的「三季度」与「备注」横向合并。", func() {
				ui.Table(c, cols, len(sales)).Bordered().Size(ui.Middle).
					Span(func(s ui.TableSpan) (int, int) {
						if s.Key == "region" {
							if s.Pos > 0 && sales[s.Pos-1].region == sales[s.Pos].region {
								return 0, 0
							}
							n := 1
							for k := s.Pos + 1; k < len(sales) && sales[k].region == sales[s.Pos].region; k++ {
								n++
							}
							return n, 1
						}
						if s.Pos == 4 && s.Key == "q3" {
							return 1, 2
						}
						if s.Pos == 4 && s.Key == "note" {
							return 0, 0
						}
						return 1, 1
					}).
					Summary(sum).
					Show()
			})
			tbDemo(c, "固定在底部的总结栏", "ScrollY + SummaryFixed：表体滚动时合计行固定在底部，并随横向滚动同步。", func() {
				pc := []ui.TableColumn{
					{Key: "name", Title: "姓名", Width: 140, Text: func(i int) string { return data[i].name }},
					{Key: "dept", Title: "部门", Width: 140, Text: func(i int) string { return data[i].dept }},
					{Key: "city", Title: "城市", Text: func(i int) string { return data[i].city }},
					{Key: "amount", Title: "金额", Width: 180, Align: ui.AlignRight, Text: func(i int) string { return tbMoney(data[i].amount) }},
				}
				ui.Table(c, pc, len(data)).ScrollY(220).Size(ui.Small).
					Summary(func(key string, rows []int) string {
						switch key {
						case "name":
							return fmt.Sprintf("合计 %d 人", len(rows))
						case "amount":
							var s float64
							for _, r := range rows {
								s += data[r].amount
							}
							return tbMoney(s)
						}
						return ""
					}).
					Summary(func(key string, rows []int) string {
						switch key {
						case "name":
							return "平均"
						case "amount":
							var s float64
							for _, r := range rows {
								s += data[r].amount
							}
							return tbMoney(s / float64(max(1, len(rows))))
						}
						return ""
					}).
					SummaryFixed().Show()
			})
		}, nil
	})
}
