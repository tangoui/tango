package main

// Spec alignment batch 2, group E2: Table behaviour and API
// (docs/specs/align/2-table-api.md): sort directions, default filters,
// Shift range selection, expand by row click, default-expanded rows, title
// and footer, pagination placement, Element Plus' current row and default
// summary, and Table V2 metrics.

import (
	"fmt"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
)

var (
	b2e2Sel    = []int{1}
	b2e2Exp    []string
	b2e2Page   = 1
	b2e2Size   = 3
	b2e2Cur    = "2"
	b2e2Sort   ui.SortState
	b2e2Filter map[string][]string
)

func init() {
	data := tbPeople(8)
	gallery.Register(gallery.Page{Name: "TableApiAlign", Parent: "Table", Title: "Table 行为与 API", Group: gallery.Display, Order: 121, Width: 1000,
		Render: func(c *ui.Context) {
			name := ui.TableColumn{Key: "name", Title: "姓名", Width: 120, Text: func(i int) string { return data[i].name }}
			age := ui.TableColumn{Key: "age", Title: "年龄", Width: 120, Compare: func(a, b int) int { return data[a].age - data[b].age },
				SortDirections: []ui.SortOrder{ui.SortDesc, ui.SortAsc}, Text: func(i int) string { return fmt.Sprint(data[i].age) }}
			city := ui.TableColumn{Key: "city", Title: "城市", Width: 140,
				Filters:       []ui.Option{{Label: "杭州", Value: "杭州"}, {Label: "上海", Value: "上海"}, {Label: "北京", Value: "北京"}},
				DefaultFilter: []string{"杭州", "上海"}, FilterResetToDefault: true,
				OnFilter: func(i int, v string) bool { return data[i].city == v }, Text: func(i int) string { return data[i].city }}
			amount := ui.TableColumn{Key: "amount", Title: "金额", Width: 120, Align: ui.AlignRight, Text: func(i int) string { return fmt.Sprintf("%.2f", data[i].amount) }}
			addr := ui.TableColumn{Key: "addr", Title: "地址", Text: func(i int) string { return data[i].addr }}

			tbDemo(c, "排序方向、默认筛选、Shift 范围选择", "年龄列 SortDirections(降序, 升序)；城市列 DefaultFilter + FilterResetToDefault；按住 Shift 点勾选框选中一段（antd sortDirections / defaultFilteredValue / 多选 Shift）。", func() {
				ui.Table(c, []ui.TableColumn{name, age, city, addr}, len(data)).Sort(&b2e2Sort).Filters(&b2e2Filter).
					Selection(&b2e2Sel).Size(ui.Small).Show()
			})
			tbDemo(c, "按行点击展开、隐藏展开列、标题与页脚", "ExpandRowByClick + HideExpandColumn；Title / Footer（antd expandable.expandRowByClick、showExpandColumn、title、footer）。", func() {
				ui.Table(c, []ui.TableColumn{name, age, addr}, 3).Expandable(func(r int) { ui.Text(c, data[r].addr) }).
					ExpandedKeys(&b2e2Exp).ExpandRowByClick().HideExpandColumn().
					Title(func() { ui.Text(c, "人员列表") }).Footer(func() { ui.Text(c, "点击行展开详情") }).Size(ui.Small).Show()
			})
			tbDemo(c, "分页位置与配置", "PaginationPlacement(topStart)，PaginationConfig 打开总数；默认无总数，小表格用小号分页（antd pagination.placement、getPaginationSize）。", func() {
				ui.Table(c, []ui.TableColumn{name, age, city}, len(data)).Pagination(&b2e2Page, &b2e2Size).
					PaginationPlacement(ui.PageTopStart).PaginationConfig(func(p *ui.PaginationW) { p.ShowTotal() }).Size(ui.Small).Show()
			})
			tbDemo(c, "当前行、合计行（Element Plus）", "CurrentRowKey 绑定 current-row-key；ShowSummary 为 show-summary 默认合计；ShowOverflowTooltip 为表级 show-overflow-tooltip。", func() {
				ui.Table(c, []ui.TableColumn{name, age, amount, addr}, 4).CurrentRowKey(&b2e2Cur).ShowSummary().ShowOverflowTooltip().
					Size(ui.Small).Show()
			})
			tbDemo(c, "Table V2 尺寸", "Virtual + RowHeight(50) + HeaderHeight(50)，对应 el-table-v2 的 row-height / header-height 默认值。", func() {
				ui.Table(c, []ui.TableColumn{name, age, city, addr}, len(data)).Virtual().ScrollY(220).RowHeight(50).HeaderHeight(50).Show()
			})
		}})
}
