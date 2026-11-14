package main

// Spec alignment batch 1, group A: demos of the capabilities added to the
// basic and layout components (docs/specs/align/1-basic.md).

import (
	"fmt"
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

var (
	b1aSeg     = "list"
	b1aSegDir  = "a"
	b1aToggle  = []string{"bold"}
	b1aToggle2 = []string{"left"}
	b1aRate    = 3.0
	b1aRate2   = 4.5
)

func init() {
	gallery.Register(gallery.Page{Name: "ButtonAlign", Parent: "Button", Title: "Button 图标位置与加载", Group: gallery.General, Order: 1,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "图标位置 iconPlacement", "IconPlacement(\"start\" | \"end\")，逻辑方向，RTL 下镜像。", func() {
				row(c, func() {
					ui.Button(c, "上一步").Icon(icon.Left)
					ui.Button(c, "下一步").Primary().Icon(icon.Right).IconPlacement("end")
				})
			})
			gallery.Demo(c, "加载图标与延迟", "LoadingIcon 换掉转圈图标；LoadingDelay 延迟进入加载态（截图里已过延迟）。", func() {
				row(c, func() {
					ui.Button(c, "同步中").Loading(true).LoadingIcon(icon.Sync)
					ui.Button(c, "提交").Primary().Loading(true).LoadingDelay(time.Millisecond)
				})
			})
			gallery.Demo(c, "两字中文自动加空格", "Ant 默认开启 autoInsertSpace，Element / shadcn 默认关闭；AutoInsertSpace(false) 单独关闭。", func() {
				row(c, func() {
					ui.Button(c, "确定").Primary()
					ui.Button(c, "取消")
					ui.Button(c, "取消").AutoInsertSpace(false)
				})
			})
		}})

	gallery.Register(gallery.Page{Name: "TypographyAlign", Parent: "Typography", Title: "Typography 链接下划线与图标", Group: gallery.General, Order: 2,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "链接下划线与图标", "UnderlineMode(\"always\" | \"hover\" | \"never\")；默认随主题（Ant 无、Element 悬停、shadcn 常显）。Icon 在文字前加图标。", func() {
				row(c, func() {
					ui.Link(c, "默认")
					ui.Link(c, "常显").UnderlineMode("always")
					ui.Link(c, "悬停").UnderlineMode("hover")
					ui.Link(c, "从不").UnderlineMode("never")
					ui.Link(c, "查看文档").Icon(icon.Link)
				})
			})
		}})

	gallery.Register(gallery.Page{Name: "DividerAlign", Parent: "Divider", Title: "Divider 方向、标题位置与线型", Group: gallery.General, Order: 4,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "标题位置 titlePlacement", "start / end 为逻辑方向，left / right 为物理方向。", func() {
				ui.Column(c, func() {
					ui.Divider(c, "Start").TitlePlacement("start")
					ui.Divider(c, "Center")
					ui.Divider(c, "End").TitlePlacement("end").Plain()
				})
			})
			gallery.Demo(c, "线型 variant 与尺寸 size", "Variant(\"dashed\" | \"dotted\")；Size(Small) 上下 8px，Size(Middle) 16px。", func() {
				ui.Column(c, func() {
					ui.Text(c, "dashed")
					ui.Divider(c).Variant("dashed").Size(ui.Small)
					ui.Text(c, "dotted")
					ui.Divider(c).Variant("dotted").Size(ui.Small)
					ui.Text(c, "middle")
					ui.Divider(c).Size(ui.Middle)
				})
			})
			gallery.Demo(c, "竖直 vertical", "Divider(c).Vertical() 或 Orientation(\"vertical\")。", func() {
				row(c, func() {
					ui.Text(c, "文本")
					ui.Divider(c).Vertical()
					ui.Link(c, "链接")
					ui.Divider(c).Orientation("vertical").Variant("dashed")
					ui.Link(c, "链接")
				})
			})
		}})

	gallery.Register(gallery.Page{Name: "SpaceAlign", Parent: "Space", Title: "Space orientation 与 separator", Group: gallery.Layout, Order: 11,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "分隔符 separator", "Separator 在相邻两项之间插入分隔内容（Split 已弃用）。", func() {
				ui.Space(c).Separator(func() { ui.Divider(c).Vertical() }).Body(func() {
					ui.Link(c, "编辑")
					ui.Link(c, "复制")
					ui.Link(c, "删除")
				})
			})
			gallery.Demo(c, "方向 orientation", "Orientation(\"vertical\")；Flex 同名方法。", func() {
				ui.Space(c).Orientation("vertical").Body(func() {
					ui.Button(c, "第一项")
					ui.Button(c, "第二项")
				})
			})
		}})

	gallery.Register(gallery.Page{Name: "SegmentedAlign", Parent: "Segmented", Title: "Segmented 自定义选项内容", Group: gallery.Form, Order: 21,
		Render: func(c *ui.Context) {
			opts := []ui.Option{{Label: "列表", Value: "list"}, {Label: "看板", Value: "kanban"}, {Label: "日历", Value: "calendar"}}
			icons := map[string]*icon.Icon{"list": icon.Table, "kanban": icon.Appstore, "calendar": icon.Calendar}
			gallery.Demo(c, "自定义内容 ItemRender", "每个选项内容由回调构建（Element 默认插槽）；选项 Label 仍是无障碍名称。", func() {
				ui.Segmented(c, &b1aSeg, opts).ItemRender(func(o ui.Option, sel bool) {
					ic := ui.Icon(c, icons[o.Value])
					t := ui.Text(c, o.Label).Strong()
					if !sel {
						ic.Color(c.Theme.TextSecondary)
						t.Secondary()
					}
				})
			})
			gallery.Demo(c, "方向 orientation", "Orientation(\"vertical\")。", func() {
				ui.Segmented(c, &b1aSegDir, ui.Opts("a", "b", "c")).Orientation("vertical")
			})
		}})

	gallery.Register(gallery.Page{Name: "ToggleAlign", Parent: "Toggle", Title: "ToggleGroup 间距与尺寸", Group: gallery.General, Order: 5,
		Render: func(c *ui.Context) {
			opts := []ui.Option{{Label: "加粗", Value: "bold"}, {Label: "斜体", Value: "italic"}, {Label: "下划线", Value: "underline"}}
			gallery.Demo(c, "间距 spacing", "默认 0：按钮相连只圆外角；Spacing(n) 间隔 n×4px。", func() {
				ui.Column(c, func() {
					ui.ToggleGroup(c, &b1aToggle, opts).Outline()
					ui.ToggleGroup(c, &b1aToggle, opts).Spacing(2)
				}, layout.Gap(12))
			})
			gallery.Demo(c, "尺寸", "sm / default / lg：内边距 6 / 8 / 10px；组内 12px。", func() {
				row(c, func() {
					o := []ui.Option{{Label: "左", Value: "left"}, {Label: "中", Value: "center"}, {Label: "右", Value: "right"}}
					ui.ToggleGroup(c, &b1aToggle2, o).Single().Size(ui.Small).Variant("outline")
					ui.ToggleGroup(c, &b1aToggle2, o).Single().Variant("outline")
					ui.ToggleGroup(c, &b1aToggle2, o).Single().Size(ui.Large).Variant("outline")
				})
			})
		}})

	gallery.Register(gallery.Page{Name: "RateAlign", Parent: "Rate", Title: "Rate 尺寸、分段颜色与分数格式", Group: gallery.Form, Order: 20,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "尺寸 size", "Small / 默认 / Large（Ant 15 / 20 / 25px）。", func() {
				ui.Column(c, func() {
					ui.Rate(c, &b1aRate).Small()
					ui.Rate(c, &b1aRate)
					ui.Rate(c, &b1aRate).Large()
				}, layout.Gap(8))
			})
			gallery.Demo(c, "分段颜色与分数", "Colors 按 Thresholds（默认 2、4）切换颜色；ScoreFormat 格式化分数。", func() {
				ui.Rate(c, &b1aRate2).AllowHalf().ShowScore().
					Colors(theme.Hex("#99a9bf"), theme.Hex("#f7ba2a"), theme.Hex("#ff9900")).
					ScoreFormat(func(v float64) string { return fmt.Sprintf("%.1f 分", v) })
			})
		}})
}
