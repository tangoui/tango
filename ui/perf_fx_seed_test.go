package ui

import (
	"strconv"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

// Seed fixtures: one per path kind (plain control, text editing, dropdown
// overlay, data collection, modal overlay). Per-page values live in the
// page's Context (State), so drivers never share them.

var (
	perfInputID  = hashID("perf.input")
	perfSelectID = hashID("perf.select")
	perfModalID  = hashID("perf.modal")
)

const perfInputText = "Tango UI 组件性能基准：中英混合 input 文本。" // 30 runes

var perfSelectOpts = Opts("定时调度", "事件触发", "手动执行", "接口回调", "数据同步")

// perfOutside is a point of the page padding, outside every fixture.
var perfOutside = geom.Point{X: 2, Y: 2}

// perfTableCols are six typical columns (perfTableCell gives the text).
func perfTableCols() []TableColumn {
	cell := func(k int) func(int) string { return func(i int) string { return perfTableCell(i, k) } }
	return []TableColumn{
		{Key: "id", Title: "编号", Width: 100, Text: cell(0)},
		{Key: "name", Title: "规则名称", Width: 160, Text: cell(1)},
		{Key: "desc", Title: "描述", MinWidth: 200, Ellipsis: true, Text: cell(2)},
		{Key: "calls", Title: "服务调用次数", Width: 150, Align: AlignRight, Text: cell(3)},
		{Key: "status", Title: "状态", Width: 120, Text: cell(4)},
		{Key: "time", Title: "上次调度时间", Width: 190, Text: cell(5)},
	}
}

func perfTableCell(i, k int) string {
	switch k {
	case 0:
		return "R-" + strconv.Itoa(i+1)
	case 1:
		return "TradeCode " + strconv.Itoa(i%997)
	case 2:
		return "这是一段描述 rule description " + strconv.Itoa(i)
	case 3:
		return strconv.Itoa((i*7919)%100000) + " 万"
	case 4:
		return [...]string{"关闭", "运行中", "已上线", "异常"}[i%4]
	}
	return "2026-11-" + strconv.Itoa(10+i%20) + " 09:30:00"
}

func init() {
	registerPerf(
		perfFixture{
			Name:  "Button",
			Build: func(c *Context) { Button(c, "新建规则").Primary() },
			Interact: func(r *perfRun, i int) {
				if i%2 == 0 {
					r.D.Move(r.Center())
				} else {
					r.D.Move(perfOutside)
				}
			},
		},
		perfFixture{
			Name: "Input",
			Build: func(c *Context) {
				v := State(c, perfInputID, func() string { return perfInputText })
				Input(c, v).Placeholder("请输入规则名称").Layout(layout.W(360))
			},
			Interact: func(r *perfRun, i int) {
				if i%2 == 0 {
					r.D.Click(r.Center())
					r.D.Type("a")
				} else {
					r.D.Key(input.KeyBackspace, 0)
					r.D.Click(perfOutside) // blur: the settled screen has no caret
				}
			},
		},
		perfFixture{
			Name: "Select",
			Build: func(c *Context) {
				v := State(c, perfSelectID, func() string { return "" })
				Select(c, v, perfSelectOpts).Placeholder("请选择规则类型").Width(240)
			},
			Interact: func(r *perfRun, i int) {
				if i%2 == 0 {
					r.D.Click(r.Center())
				} else {
					r.D.Key(input.KeyEscape, 0)
					r.D.Click(perfOutside) // blur and unhover
				}
			},
			Large: func(n int) func(c *Context) {
				opts := make([]Option, n)
				for i := range opts {
					s := "选项 option " + strconv.Itoa(i+1)
					opts[i] = Option{Label: s, Value: s}
				}
				return func(c *Context) {
					v := State(c, perfSelectID, func() string { return "" })
					Select(c, v, opts).Placeholder("请选择").Width(240).Open()
				}
			},
		},
		perfFixture{
			Name: "Table",
			Build: func(c *Context) {
				Table(c, perfTableCols(), 50).Show()
			},
			Interact: func(r *perfRun, i int) {
				if i%2 == 0 {
					row, ok := r.D.FindText(perfTableCell(2, 0)) // 3rd row
					if !ok {
						panic("perf: table row 3 not found")
					}
					r.D.Move(geom.Point{X: r.Target.X + r.Target.W/2, Y: row.Center().Y})
				} else {
					r.D.Move(perfOutside)
				}
			},
			Large: func(n int) func(c *Context) {
				cols := perfTableCols()
				return func(c *Context) {
					Table(c, cols, n).ScrollY(760).Virtual().Show()
				}
			},
		},
		perfFixture{
			Name:    "Modal",
			Overlay: true,
			Build: func(c *Context) {
				open := State(c, perfModalID, func() bool { return true })
				Modal(c, open, "新建规则").Width(560).Body(func() {
					Text(c, "对话框的内容：规则名称、类型与负责人。")
					Text(c, "Modal supports any component inside.")
				})
			},
			Interact: func(r *perfRun, i int) {
				if i%2 == 0 {
					r.D.Key(input.KeyEscape, 0)
				} else {
					*State(r.D.r.c, perfModalID, func() bool { return true }) = true
					r.D.r.Invalidate()
					r.D.Settle()
				}
			},
		},
	)
}
