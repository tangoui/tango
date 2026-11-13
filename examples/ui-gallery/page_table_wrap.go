package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
)

var tbNotes = []string{
	"短备注",
	"需要在月底前完成对账，并把差异明细同步给财务部门复核。",
	"客户反馈导出报表时偶发超时，已加索引观察一周；若仍复现，考虑拆分查询并增加缓存层，同时在监控中加一条慢查询告警。",
	"已完成",
	"跨部门协作：设计、研发、测试三方需在周三前对齐验收标准。",
}

func init() {
	tbPage("TableWrap", "Table 表格 · 可变行高（换行与虚拟滚动）", 111, 1080, 0, false, func() (func(*ui.Context), func(*ui.Driver)) {
		data := tbPeople(6)
		return func(c *ui.Context) {
				tbDemo(c, "单元格换行", "Wrap 列文字换行，行高按内容测量；Ellipsis 列保持单行并在悬停时提示全文（二者二选一）。Render 单元格也可 Wrap（标签自动折行）。", func() {
					cols := []ui.TableColumn{
						{Key: "name", Title: "姓名", Width: 100, Text: func(i int) string { return data[i].name }},
						{Key: "note", Title: "备注（Wrap）", Wrap: true, Text: func(i int) string { return tbNotes[i%len(tbNotes)] }},
						{Key: "addr", Title: "住址（Ellipsis）", Width: 180, Ellipsis: true, Text: func(i int) string { return data[i].addr + "（靠近地铁站 " + strconv.Itoa(i+1) + " 号口）" }},
						{Key: "tags", Title: "标签（Wrap）", Width: 200, Wrap: true, Render: func(i int) {
							for k := 0; k <= i%4; k++ {
								ui.Tag(c, tbTagPool[(i+k)%len(tbTagPool)]).Color(tbTagColor(tbTagPool[(i+k)%len(tbTagPool)]))
							}
						}},
					}
					ui.Table(c, cols, len(data)).Bordered().Show()
				})
				tbDemo(c, "可变行高 + 虚拟滚动（10 万行）", "可见行测量后缓存，未测量的行按已测均值估算；上方行高变化时保持首个可见行不跳动。", func() {
					cols := []ui.TableColumn{
						{Key: "id", Title: "编号", Width: 100, Fixed: ui.FixedLeft, Text: func(i int) string { return "#" + strconv.Itoa(i+1) }},
						{Key: "note", Title: "说明", Wrap: true, Text: func(i int) string {
							return strings.Repeat(tbNotes[i%len(tbNotes)], 1+i%3)
						}},
						{Key: "v", Title: "数值", Width: 120, Align: ui.AlignRight, Text: func(i int) string { return fmt.Sprint((i * 37) % 1000) }},
					}
					ui.Table(c, cols, 100000).ScrollY(360).Virtual().Size(ui.Middle).Show()
				})
			}, func(d *ui.Driver) {
				if r, ok := d.FindText("#1"); ok {
					p := r.Center()
					for i := 0; i < 5; i++ {
						d.Inject(input.Scroll{Pos: p, DY: 400000})
					}
					d.Settle()
				}
				_ = layout.Grow
			}
	})
	tbPage("TableSticky", "Table 表格 · 表头吸顶与合计行吸底", 112, 1080, 640, false, func() (func(*ui.Context), func(*ui.Driver)) {
		data := tbPeople(40)
		var box *ui.ScrollBox
		return func(c *ui.Context) {
				tbDemo(c, "Sticky", "表格在外层滚动容器中滚动时，表头吸附在容器顶部（Sticky(offset)），合计行吸附在容器底部（StickySummary）。", func() {
					box = ui.Scroll(c, func() {
						ui.Text(c, "向下滚动查看吸顶效果 ↓").Tertiary().Layout(layout.Margin(geom.Insets{Bottom: 12}))
						cols := []ui.TableColumn{
							{Key: "name", Title: "姓名", Width: 120, Text: func(i int) string { return data[i].name }},
							{Key: "age", Title: "年龄", Width: 90, Text: func(i int) string { return strconv.Itoa(data[i].age) }},
							{Key: "dept", Title: "部门", Width: 120, Text: func(i int) string { return data[i].dept }},
							{Key: "addr", Title: "住址", Text: func(i int) string { return data[i].addr }},
							{Key: "amount", Title: "金额", Width: 140, Align: ui.AlignRight, Text: func(i int) string { return tbMoney(data[i].amount) }},
						}
						ui.Table(c, cols, len(data)).Sticky(0).StickySummary(0).
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
					}, layout.H(400))
				})
			}, func(d *ui.Driver) {
				if box != nil {
					box.ScrollTo(0, 300)
					d.Settle()
				}
			}
	})
}
