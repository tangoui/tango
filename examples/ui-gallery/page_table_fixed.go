package main

import (
	"fmt"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

func init() {
	tbPage("TableFixed", "Table 表格 · 固定列与表头分组", 104, 1080, 0, false, func() (func(*ui.Context), func(*ui.Driver)) {
		data := tbPeople(30)
		sel := []int{2}
		return func(c *ui.Context) {
				cols := []ui.TableColumn{
					{Key: "name", Title: "姓名", Width: 110, Fixed: ui.FixedLeft, Text: func(i int) string { return data[i].name }},
					{Key: "age", Title: "年龄", Width: 80, Fixed: ui.FixedLeft, Text: func(i int) string { return fmt.Sprint(data[i].age) }},
				}
				for k := 1; k <= 8; k++ {
					k := k
					cols = append(cols, ui.TableColumn{Key: fmt.Sprint("c", k), Title: fmt.Sprint("列 ", k), Width: 130, Text: func(i int) string {
						return fmt.Sprintf("%s · %d", data[(i+k)%len(data)].city, (i*7+k*13)%100)
					}})
				}
				cols = append(cols, ui.TableColumn{Key: "op", Title: "操作", Width: 100, Fixed: ui.FixedRight, Render: func(i int) { ui.Link(c, "编辑") }})
				tbDemo(c, "左右固定列 · 固定表头 · 横纵滚动 · 列宽拖拽", "ScrollX 设置内容宽度，ScrollY 限制表体高度；滚动时固定列出现阴影。拖动表头右边缘可调整列宽。", func() {
					ui.Table(c, cols, len(data)).RowKey(func(i int) string { return fmt.Sprint("f", i) }).Selection(&sel).ScrollX(1500).ScrollY(240).Resizable().Size(ui.Middle).Show()
				})
				tbDemo(c, "表头分组", "Children 组成多级表头。", func() {
					g := []ui.TableColumn{
						{Key: "name", Title: "姓名", Width: 110, Fixed: ui.FixedLeft, Text: func(i int) string { return data[i].name }},
						{Key: "other", Title: "其他", Children: []ui.TableColumn{
							{Key: "age", Title: "年龄", Width: 90, Text: func(i int) string { return fmt.Sprint(data[i].age) }},
							{Key: "addr", Title: "住址", Children: []ui.TableColumn{
								{Key: "city", Title: "城市", Width: 100, Text: func(i int) string { return data[i].city }},
								{Key: "street", Title: "街道", Text: func(i int) string { return data[i].addr }},
							}},
						}},
						{Key: "company", Title: "公司", Children: []ui.TableColumn{
							{Key: "dept", Title: "部门", Width: 110, Text: func(i int) string { return data[i].dept }},
							{Key: "amount", Title: "薪资", Width: 130, Align: ui.AlignRight, Text: func(i int) string { return tbMoney(data[i].amount) }},
						}},
						{Key: "gender", Title: "状态", Width: 100, Fixed: ui.FixedRight, Text: func(i int) string { return tbStatusText[data[i].status] }},
					}
					ui.Table(c, g, 4).Bordered().Size(ui.Middle).Show()
				})
			}, func(d *ui.Driver) {
				if r, ok := d.Find("row:f1"); ok {
					d.Inject(scrollX(r.Center(), 260))
					d.Settle()
					d.Move(r.Center())
				}
			}
	})
}

func scrollX(p geom.Point, dx float32) input.Event { return input.Scroll{Pos: p, DX: dx} }
