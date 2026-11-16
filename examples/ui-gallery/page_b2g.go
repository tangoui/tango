package main

// Spec alignment batch 2, group G: demos of the capabilities added to the
// tag, badge and status components (docs/specs/align/2-display.md).

import (
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

var (
	b2gTagGroup  = []string{"Books"}
	b2gTagGroupM = []string{"Movies", "Music"}
	b2gTipOpen   = true
)

func init() {
	gallery.Register(gallery.Page{Name: "TagAlign", Parent: "Tag", Title: "Tag 变体、禁用与 Badge 外观", Group: gallery.Display, Order: 1,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "变体 variant", "antd 6：filled（默认，无边框）/ solid / outlined；Element 默认 outlined（light 效果）。", func() {
				for _, v := range []string{"filled", "solid", "outlined"} {
					row(c, func() {
						ui.Tag(c, v).Variant(v)
						ui.Tag(c, "success").Color("success").Variant(v)
						ui.Tag(c, "error").Color("error").Variant(v)
						ui.Tag(c, "blue").Color("blue").Variant(v)
						ui.Tag(c, "magenta").Color("magenta").Variant(v)
					})
				}
			})
			gallery.Demo(c, "禁用与自定义关闭图标", "Disabled(true) 置灰且不能关闭；CloseIcon 换关闭图标。", func() {
				row(c, func() {
					ui.Tag(c, "禁用").Disabled(true)
					ui.Tag(c, "禁用可关闭").Closable().Disabled(true)
					ui.Tag(c, "禁用描边").Variant("outlined").Disabled(true)
					ui.Tag(c, "自定义关闭").CloseIcon(icon.CloseCircle)
				})
			})
			gallery.Demo(c, "shadcn Badge 变体", "Color(\"primary\" | \"secondary\" | \"destructive\" | \"outline\" | \"ghost\" | \"link\")；shadcn 下为 rounded-full、font-medium，暗色 destructive 为 destructive/60。", func() {
				row(c, func() {
					for _, v := range []string{"primary", "secondary", "destructive", "outline", "ghost", "link"} {
						ui.Tag(c, v).Color(v)
					}
				})
			})
		}})

	gallery.Register(gallery.Page{Name: "CheckableTagAlign", Parent: "CheckableTag", Title: "CheckableTagGroup 可选择标签组", Group: gallery.Display, Order: 1,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "单选与多选", "antd 6 Tag.CheckableTagGroup：默认单选（再点取消），Multiple(true) 多选；间距 8。", func() {
				ui.Column(c, func() {
					ui.CheckableTagGroup(c, &b2gTagGroup, ui.Opts("Movies", "Books", "Music", "Sports"))
					ui.CheckableTagGroup(c, &b2gTagGroupM, ui.Opts("Movies", "Books", "Music", "Sports")).Multiple(true)
					ui.CheckableTagGroup(c, &b2gTagGroupM, ui.Opts("Movies", "Books")).Multiple(true).Disabled(true)
				}, layout.Gap(12))
			})
		}})

	gallery.Register(gallery.Page{Name: "BadgeAlign", Parent: "Badge", Title: "Badge 负数、隐藏与标题", Group: gallery.Display, Order: 1,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "计数", "负数照常显示；多位数左右内边距 8（antd paddingXS）；Hidden 隐藏；Title 作为读屏文字。", func() {
				row(c, func() {
					ui.Badge(c).Count(-5).Body(func() { ui.Avatar(c, "A").Square() })
					ui.Badge(c).Count(25).Body(func() { ui.Avatar(c, "B").Square() })
					ui.Badge(c).Count(120).Body(func() { ui.Avatar(c, "C").Square() })
					ui.Badge(c).Count(5).Hidden(true).Body(func() { ui.Avatar(c, "D").Square() })
					ui.Badge(c).Count(7).Title("7 条未读").Body(func() { ui.Avatar(c, "E").Square() })
				})
			})
		}})

	gallery.Register(gallery.Page{Name: "AvatarAlign", Title: "Avatar 尺寸、形状与折叠", Group: gallery.Display, Order: 32,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "尺寸与形状", "Small / Large 用尺寸令牌；Shape(\"square\") 方形圆角随尺寸（antd 4/6/8）；文字超宽时按 Gap(4) 缩小。", func() {
				row(c, func() {
					ui.Avatar(c, "U").Small()
					ui.Avatar(c, "U")
					ui.Avatar(c, "U").Large()
					ui.Avatar(c, "U").Small().Shape("square")
					ui.Avatar(c, "U").Shape("square")
					ui.Avatar(c, "U").Large().Shape("square")
					ui.Avatar(c, "").Large()
					ui.Avatar(c, "Tango")
					ui.Avatar(c, "Tango").Large()
				})
			})
			gallery.Demo(c, "折叠与弹层", "Max(2) 把其余头像折进 “+N”，悬停显示被折叠的头像（antd max.popover）。", func() {
				ui.AvatarGroup(c).Max(2).Body(func() {
					for _, s := range []string{"A", "B", "C", "D"} {
						ui.Avatar(c, s)
					}
				})
			})
		}})

	gallery.Register(gallery.Page{Name: "StatisticAlign", Parent: "Statistic", Title: "Statistic 分隔符与精度", Group: gallery.Display, Order: 1,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "分隔符与精度", "GroupSeparator / DecimalSeparator；precision 截断不四舍五入（antd、Element 一致）。", func() {
				row(c, func() {
					ui.Statistic(c, "欧式写法", 1234567.891, 2).GroupSeparator(".").DecimalSeparator(",")
					ui.Statistic(c, "截断", 1.239, 2)
					ui.Statistic(c, "原样小数", 3.14159, -1)
				})
			})
		}})

	gallery.Register(gallery.Page{Name: "EmptyAlign", Parent: "Empty", Title: "Empty 标题与间距", Group: gallery.Display, Order: 1,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "标题", "Title 在描述上方加标题（shadcn EmptyTitle）；间距来自令牌（shadcn p-6 gap-6）。", func() {
				ui.Empty(c).Title("暂无项目").Description("创建一个项目开始使用。").Body(func() {
					ui.Button(c, "新建项目").Primary()
				})
			})
		}})

	gallery.Register(gallery.Page{Name: "SkeletonAlign", Parent: "Skeleton", Title: "Skeleton 默认行数与宽度", Group: gallery.Display, Order: 1,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "头像与行数", "带头像时默认两行、头像 40；TitleWidth / ParagraphWidth 设宽度。", func() {
				ui.Column(c, func() {
					ui.Skeleton(c).Avatar(false).Show()
					ui.Skeleton(c).TitleWidth(0.6).ParagraphWidth(0.9, 0.7, 0.4).Show()
				}, layout.Gap(24))
			})
			gallery.Demo(c, "重复", "Count(2) 重复两份（Element count）。", func() {
				ui.Skeleton(c).Count(2).Rows(1).Show()
			})
		}})

	gallery.Register(gallery.Page{Name: "ProgressAlign", Parent: "Progress", Title: "Progress 文字位置与缺口方向", Group: gallery.Display, Order: 1,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "文字位置 percentPosition", "PercentPosition(align, type)：start / center / end，outer / inner。", func() {
				ui.Column(c, func() {
					ui.Progress(c, 40).PercentPosition("start", "outer").Layout(layout.W(320))
					ui.Progress(c, 40).PercentPosition("center", "outer").Layout(layout.W(320))
					ui.Progress(c, 60).PercentPosition("start", "inner").Layout(layout.W(320))
					ui.Progress(c, 60).PercentPosition("center", "inner").Layout(layout.W(320))
				}, layout.Gap(12))
			})
			gallery.Demo(c, "缺口方向 gapPlacement", "GapDegree 让 circle 也有缺口；GapPlacement(\"top\" | \"bottom\" | \"start\" | \"end\")。", func() {
				row(c, func() {
					for _, g := range []string{"bottom", "top", "start", "end"} {
						ui.Progress(c, 70).Circle(80).GapDegree(70).GapPlacement(g)
					}
					ui.Progress(c, 70).Type("circle").Small()
				})
			})
			gallery.Demo(c, "颜色函数与轨道色", "ColorFunc 按百分比取色（Element color 函数）；RailColor 替代已弃用的 TrailColor。", func() {
				ui.Column(c, func() {
					for _, v := range []float64{30, 70} {
						ui.Progress(c, v).ColorFunc(func(p float64) theme.Color {
							if p > 50 {
								return theme.Hex("#52c41a")
							}
							return theme.Hex("#faad14")
						}).Layout(layout.W(320))
					}
					ui.Progress(c, 50).Steps(5).StepsGap(6)
				}, layout.Gap(12))
			})
		}})

	gallery.Register(gallery.Page{Name: "TooltipAlign", Parent: "Tooltip", Title: "Tooltip 受控、禁用与延迟", Group: gallery.Display, Order: 1,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "受控与延迟", "Open(&v) 受控显示；延迟默认随主题（antd 0.1s/0.1s、Element 0/0.2s、shadcn 0/0）。", func() {
				ui.Column(c, func() {
					ui.Tooltip(c, "受控提示", func() { ui.Button(c, "受控显示") }).Open(&b2gTipOpen)
				}, layout.Pad(40))
				row(c, func() {
					ui.Tooltip(c, "不会显示", func() { ui.Button(c, "禁用提示") }).Disabled(true)
					ui.Tooltip(c, "慢一点", func() { ui.Button(c, "延迟 0.5s") }).MouseEnterDelay(500 * time.Millisecond)
				})
			})
		}})

	gallery.Register(gallery.Page{Name: "PopoverAlign", Parent: "Popover", Title: "Popover 宽度与延迟", Group: gallery.Display, Order: 1,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "默认宽度与延迟", "Element 默认宽 150、shadcn w-72（288）；MouseEnterDelay / MouseLeaveDelay；Arrow(false) 关箭头。", func() {
				row(c, func() {
					ui.Popover(c).Title("标题").Text("悬停 0.5 秒后打开").MouseEnterDelay(500 * time.Millisecond).Body(func() { ui.Button(c, "延迟打开") })
					ui.Popover(c).Text("没有箭头").Arrow(false).Body(func() { ui.Button(c, "无箭头") })
				})
			})
		}})
}
