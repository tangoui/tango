package main

import (
	"fmt"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

var tableSel = []int{1}
var tableSort = ui.SortState{Column: "age", Order: ui.SortAsc}

func init() {
	gallery.Register(gallery.Page{Name: "Table", Title: "Table 表格", Group: gallery.Display, Order: 1, Width: 1000, Render: func(c *ui.Context) {
		names := []string{"胡彦斌", "胡彦祖", "李大嘴", "王小二", "张三丰"}
		addr := []string{"西湖区湖底公园1号", "滨江区网商路699号", "朝阳区望京SOHO", "徐汇区漕河泾", "武当山紫霄宫"}
		tags := [][]string{{"nice", "developer"}, {"loser"}, {"cool", "teacher"}, {"designer"}, {"master"}}
		cols := []ui.TableColumn{
			{Key: "name", Title: "姓名", Width: 140, Render: func(i int) { ui.Link(c, names[i]) }},
			{Key: "age", Title: "年龄", Width: 100, Sortable: true, Text: func(i int) string { return fmt.Sprint(28 + i*7%30) }},
			{Key: "addr", Title: "住址", Text: func(i int) string { return addr[i] }},
			{Key: "tags", Title: "标签", Width: 200, Render: func(i int) {
				for _, t := range tags[i] {
					col := "processing"
					if t == "loser" {
						col = "error"
					} else if len(t) > 6 {
						col = "success"
					}
					ui.Tag(c, t).Color(col)
				}
			}},
			{Key: "op", Title: "操作", Width: 140, Render: func(i int) { ui.Link(c, "邀请"); ui.Link(c, "删除") }},
		}
		ui.Table(c, cols, 5).Selection(&tableSel).Sort(&tableSort).Show()
		ui.Column(c, func() {}, layout.H(24))
		ui.Table(c, cols[:3], 3).Bordered().Striped().Size(ui.Small).Show()
		ui.Column(c, func() {}, layout.H(24))
		ui.Table(c, cols[:3], 0).Show()
	}})
	gallery.Register(gallery.Page{Name: "Tag", Title: "Tag 标签", Group: gallery.Display, Order: 2, Render: func(c *ui.Context) {
		row(c, func() {
			ui.Tag(c, "默认")
			ui.Tag(c, "success").Color("success").Icon(icon.CheckCircle)
			ui.Tag(c, "processing").Color("processing")
			ui.Tag(c, "warning").Color("warning")
			ui.Tag(c, "error").Color("error")
			ui.Tag(c, "magenta").Color("#eb2f96")
			ui.Tag(c, "可关闭").Closable()
		})
	}})
	gallery.Register(gallery.Page{Name: "Tooltip", Title: "Tooltip 文字提示", Group: gallery.Display, Order: 9, Height: 220,
		Render: func(c *ui.Context) {
			ui.Row(c, func() {
				ui.Tooltip(c, "这是一段文字提示", func() { ui.Button(c, "悬停显示提示") })
			}, layout.PadVH(64, 120))
		},
		Setup: func(d *ui.Driver) {
			if r, ok := d.Find("悬停显示提示"); ok {
				d.Move(r.Center())
			}
		}})
	navPage(gallery.Page{Name: "Space", Title: "Space 间距", Group: gallery.Layout, Order: 11, Render: spacePage}, nil)
}

func spacePage(c *ui.Context) {
	navDemo(c, "基本用法", "默认 Small 间距，交叉轴居中。", func() {
		ui.Space(c).Body(func() {
			ui.Button(c, "按钮一").Primary()
			ui.Button(c, "按钮二")
			ui.Button(c, "按钮三")
			ui.Link(c, "链接")
		})
	})
	navDemo(c, "尺寸 / 竖排", "Size(Small/Middle/Large) 或 SizePx；Vertical 竖向排列。", func() {
		ui.Row(c, func() {
			ui.Space(c).Vertical().Size(ui.Small).Body(func() {
				ui.Button(c, "竖排一")
				ui.Button(c, "竖排二")
			})
			ui.Space(c).Size(ui.Middle).Body(func() {
				ui.Button(c, "中间距")
				ui.Button(c, "中间距")
			})
			ui.Space(c).Size(ui.Large).Body(func() {
				ui.Button(c, "大间距")
				ui.Button(c, "大间距")
			})
		}, layout.Gap(48))
	})
	navDemo(c, "分隔符 / 换行", "Split 在相邻两项之间插入分隔符；Wrap 自动换行。", func() {
		ui.Space(c).Split(func() { ui.VDivider(c) }).Body(func() {
			ui.Link(c, "链接一")
			ui.Link(c, "链接二")
			ui.Link(c, "链接三")
		})
		ui.Gap(c, 16)
		ui.Space(c).Wrap().SizePx(8, 12).Layout(layout.MaxW(520)).Body(func() {
			for i := 0; i < 12; i++ {
				ui.Button(c, fmt.Sprintf("按钮 %d", i+1))
			}
		})
	})
}
