package main

import (
	"fmt"
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
)

// p2eTask is a row of the editable table demos.
type p2eTask struct {
	id     string
	title  string
	state  string
	labels []string
	due    time.Time
	budget float64
}

var p2eStates = []ui.ProEnum{{Value: "open", Label: "未解决", Status: "error"}, {Value: "doing", Label: "解决中", Status: "processing"}, {Value: "done", Label: "已解决", Status: "success"}}

func p2eTaskCols() []ui.ProColumn[p2eTask] {
	return []ui.ProColumn[p2eTask]{
		{Key: "title", Title: "活动名称", Width: 200, Value: func(r p2eTask) any { return r.title },
			Editor: &ui.ProField{Required: true, Rules: []ui.Rule{ui.MaxLen(16, "最多 16 个字")}},
			Set:    func(r *p2eTask, v any) { r.title, _ = v.(string) }},
		{Key: "state", Title: "状态", Width: 140, ValueType: ui.ValueStatus, ValueEnum: p2eStates, Value: func(r p2eTask) any { return r.state },
			Editor: &ui.ProField{Required: true}, Set: func(r *p2eTask, v any) { r.state, _ = v.(string) }},
		{Key: "due", Title: "截止日期", Width: 180, ValueType: ui.ValueDate, Value: func(r p2eTask) any { return r.due },
			Editor: &ui.ProField{}, Set: func(r *p2eTask, v any) { r.due, _ = v.(time.Time) }},
		{Key: "budget", Title: "预算", Width: 150, ValueType: ui.ValueMoney, Value: func(r p2eTask) any { return r.budget },
			Editor: &ui.ProField{Min: 0, Max: 1e6}, Set: func(r *p2eTask, v any) { r.budget, _ = v.(float64) }},
		{Key: "op", Title: "操作", ValueType: ui.ValueOption, Width: 160},
	}
}

func p2eTasks() []p2eTask {
	d := func(day int) time.Time { return time.Date(2026, 11, day, 0, 0, 0, 0, time.UTC) }
	return []p2eTask{
		{"t1", "双十一预热活动", "doing", nil, d(10), 12000},
		{"t2", "会员积分翻倍", "open", nil, d(15), 3500},
		{"t3", "新品首发直播", "done", nil, d(20), 48000},
	}
}

func init() {
	p2ePage("EditableProTable", "EditableProTable 可编辑表格", gallery.Display, "", 63, 1080, 0, func() (func(*ui.Context), func(*ui.Driver)) {
		rows := p2eTasks()
		seq := 3
		var e *ui.EditableProTableW[p2eTask]
		return func(c *ui.Context) {
				tbDemo(c, "行编辑 · 新增行 · 行校验", "编辑中的行用 ProField 的控件编辑各列（文本、状态下拉、日期、金额）；保存前跑必填/规则/异步校验，失败时在单元格下方提示；新增行保存后才写入受控数据。", func() {
					e = ui.EditableProTable(c, p2eTaskCols(), &rows).Title("活动列表").
						RowKey(func(r p2eTask) string { return r.id }).
						NewRow(func() p2eTask { seq++; return p2eTask{id: fmt.Sprint("t", seq), state: "open"} }).
						Show()
				})
			}, func(d *ui.Driver) {
				// Add a row and save it empty: the required errors show.
				d.ClickLabel("添加一行数据")
				e.Save("new:1")
				d.Settle()
			}
	})
	p2ePage("DragSortTable", "DragSortTable 拖拽排序表格", gallery.Display, "ProTable", 64, 1080, 0, func() (func(*ui.Context), func(*ui.Driver)) {
		data := tbRules(6)
		return func(c *ui.Context) {
				tbDemo(c, "ProTable.DragSort", "左侧把手拖动行（键盘：聚焦表格后 Alt+↑/↓），松开后回调给出页内位置；这里直接在本页重排。", func() {
					var p *ui.ProTableW[tbRule]
					cols := []ui.ProColumn[tbRule]{
						{Key: "name", Title: "规则名称", Value: func(r tbRule) any { return r.name }},
						{Key: "owner", Title: "负责人", Width: 120, Value: func(r tbRule) any { return r.owner }, HideInSearch: true},
						{Key: "amount", Title: "金额", Width: 150, ValueType: ui.ValueMoney, Value: func(r tbRule) any { return r.amount }, HideInSearch: true},
						{Key: "status", Title: "状态", Width: 120, ValueType: ui.ValueStatus, ValueEnum: p3Status, Value: func(r tbRule) any { return r.status }, HideInSearch: true},
					}
					p = ui.ProTable(c, cols).Title("规则排序").NoSearch().RowKey(func(r tbRule) string { return r.key }).
						Request(func(req ui.ProRequest, done func(ui.ProResult[tbRule])) {
							done(ui.ProResult[tbRule]{Rows: data, Total: len(data)})
						}).
						DragSort(func(from, to int) {
							p.MovePageRow(from, to)
							data = append([]tbRule(nil), p.Rows()...)
						}).Show()
				})
			}, func(d *ui.Driver) {
				// Hold the first row half-way down (the drop line shows).
				if r, ok := d.Find("drag:" + data[0].key); ok {
					p2eHold(d, r.Center(), geom.Pt(r.Center().X, r.Center().Y+120))
				}
			}
	})
}
