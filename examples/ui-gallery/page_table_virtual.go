package main

import (
	"fmt"
	"strconv"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/input"
)

// tbVirtualColumns builds n generated columns over computed cells.
func tbVirtualColumns(c *ui.Context, n int) []ui.TableColumn {
	cols := []ui.TableColumn{
		{Key: "id", Title: "编号", Width: 100, Fixed: ui.FixedLeft, Text: func(i int) string { return "#" + strconv.Itoa(i+1) }},
		{Key: "name", Title: "姓名", Width: 100, Fixed: ui.FixedLeft, Text: func(i int) string {
			return tbFamily[i%len(tbFamily)] + tbGiven[(i/7)%len(tbGiven)]
		}},
	}
	for k := 0; k < n-3; k++ {
		k := k
		cols = append(cols, ui.TableColumn{Key: "c" + strconv.Itoa(k), Title: fmt.Sprintf("指标 %02d", k+1), Width: 120, Align: ui.AlignRight,
			Text: func(i int) string { return strconv.Itoa((i*31 + k*17) % 10000) }})
	}
	cols = append(cols, ui.TableColumn{Key: "op", Title: "操作", Width: 90, Fixed: ui.FixedRight, Render: func(i int) { ui.Link(c, "查看") }})
	return cols
}

func init() {
	tbPage("TableVirtual", "Table 表格 · 虚拟滚动（10 万行 × 50 列）", 108, 1080, 0, false, func() (func(*ui.Context), func(*ui.Driver)) {
		var sel []int
		return func(c *ui.Context) {
				tbDemo(c, "Table V2：10 万行 × 50 列", "ScrollY 后只构建可见行与可见列；固定列、选择、键盘导航照常工作。拖动滚动条或滚轮浏览。", func() {
					ui.Table(c, tbVirtualColumns(c, 50), 100000).Selection(&sel).ScrollY(440).Virtual().Size(ui.Small).Show()
				})
			}, func(d *ui.Driver) {
				if r, ok := d.Find("row:0"); ok {
					p := r.Center()
					for i := 0; i < 20; i++ {
						d.Inject(input.Scroll{Pos: p, DY: 120000})
					}
					d.Inject(input.Scroll{Pos: p, DX: 600})
					d.Settle()
					d.Move(p)
				}
			}
	})
}
