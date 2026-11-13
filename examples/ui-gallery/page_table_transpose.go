package main

import (
	"fmt"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

func init() {
	tbPage("TableTranspose", "Table 表格 · 转置（记录横向对比）", 113, 1080, 0, false, func() (func(*ui.Context), func(*ui.Driver)) {
		type plan struct {
			name, price, storage, support, tag, tagColor string
			members                                      int
			sso                                          bool
		}
		plans := []plan{
			{"基础版", "¥0", "5 GB", "社区论坛", "免费", "", 3, false},
			{"专业版", "¥49 / 月", "100 GB", "工单 · 48 小时", "推荐", "processing", 20, false},
			{"团队版", "¥199 / 月", "1 TB", "专属顾问 · 4 小时", "热门", "warning", 100, true},
			{"企业版", "联系销售", "不限", "7×24 电话", "定制", "success", 0, true},
		}
		type release struct {
			ver, date, size string
			fixes           int
			lts             bool
		}
		var rels []release
		for i := 0; i < 12; i++ {
			rels = append(rels, release{fmt.Sprintf("v2.%d.0", i), fmt.Sprintf("2026-%02d-15", i+1), fmt.Sprintf("%.1f MB", 41.5+float64(i)*0.8), 12 + i*3, i%4 == 0})
		}
		return func(c *ui.Context) {
			tbDemo(c, "套餐对比", "Transposed：每个 TableColumn 变成一行、每条记录变成一列；TransposedHeader 给每条记录列标题，首列显示字段名（表头样式、固定在起点）。单元格沿用原列的 Text / Render 与 Align。", func() {
				cols := []ui.TableColumn{
					{Key: "tag", Title: "标签", Render: func(i int) { ui.Tag(c, plans[i].tag).Color(plans[i].tagColor) }},
					{Key: "price", Title: "价格", Text: func(i int) string { return plans[i].price }},
					{Key: "storage", Title: "存储空间", Text: func(i int) string { return plans[i].storage }},
					{Key: "members", Title: "成员数", Align: ui.AlignRight, Text: func(i int) string {
						if plans[i].members == 0 {
							return "不限"
						}
						return fmt.Sprint(plans[i].members)
					}},
					{Key: "sso", Title: "单点登录", Align: ui.AlignCenter, Text: func(i int) string {
						if plans[i].sso {
							return "支持"
						}
						return "—"
					}},
					{Key: "support", Title: "支持方式", Ellipsis: true, Text: func(i int) string { return plans[i].support }},
				}
				ui.Table(c, cols, len(plans)).Transposed().
					TransposedHeader(func(i int) string { return plans[i].name }).
					A11yLabel("套餐对比").Show()
			})
			tbDemo(c, "多记录横向滚动 · 边框 · 紧凑", "记录较多时用 ScrollX 横向滚动，首列（字段名）固定不动；TransposedColumnWidth 统一设置记录列宽度。", func() {
				cols := []ui.TableColumn{
					{Key: "date", Title: "发布日期", Text: func(i int) string { return rels[i].date }},
					{Key: "size", Title: "安装包", Align: ui.AlignRight, Text: func(i int) string { return rels[i].size }},
					{Key: "fixes", Title: "修复问题", Align: ui.AlignRight, Text: func(i int) string { return fmt.Sprint(rels[i].fixes) }},
					{Key: "lts", Title: "长期支持", Render: func(i int) {
						if rels[i].lts {
							ui.Tag(c, "LTS").Color("success")
						} else {
							ui.Text(c, "—").Tertiary()
						}
					}},
				}
				ui.Table(c, cols, len(rels)).Transposed().
					TransposedHeader(func(i int) string { return rels[i].ver }).
					TransposedColumnWidth(120).ScrollX(1600).Size(ui.Small).Bordered().Show()
			})
			tbDemo(c, "无列标题", "不设 TransposedHeader 时不显示列标题行，适合详情对照；TransposedHeaderWidth 指定首列宽度。", func() {
				cols := []ui.TableColumn{
					{Key: "name", Title: "套餐", Text: func(i int) string { return plans[i].name }},
					{Key: "price", Title: "价格", Text: func(i int) string { return plans[i].price }},
					{Key: "storage", Title: "存储空间", Text: func(i int) string { return plans[i].storage }},
				}
				ui.Row(c, func() {
					ui.Column(c, func() {
						ui.Table(c, cols, 2).Transposed().TransposedHeaderWidth(120).Size(ui.Middle).Show()
					}, layout.Grow(1), layout.MinW(0))
				})
			})
		}, nil
	})
}
