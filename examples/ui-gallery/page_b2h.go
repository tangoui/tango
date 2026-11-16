package main

import (
	"fmt"
	"image"
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

// Spec alignment batch 2, group H: the Tabs, Timeline, Image, Carousel,
// Calendar, QRCode, Watermark and Masonry options added to match antd 6.7,
// Element Plus 2.14.8 and shadcn/ui v4 (docs/specs/align/2-media.md).

type b2hDemo struct {
	tabSize, tabStart, tabStretch, tabGuard, tabLine string
	guardMsg                                         string
	carDot, carStart                                 int
	day, week                                        time.Time
	masonryCols                                      int
	masonryChanges                                   int
	imgs                                             []image.Image
}

var b2h = &b2hDemo{}

func b2hReset() {
	*b2h = b2hDemo{
		tabSize: "a", tabStart: "b", tabStretch: "a", tabGuard: "a", tabLine: "a",
		day:         time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC),
		week:        time.Date(2026, 3, 18, 0, 0, 0, 0, time.UTC),
		masonryCols: 3,
	}
	b2h.imgs = []image.Image{dataImgScene(480, 320, 0), dataImgScene(480, 320, 1), dataImgScene(480, 320, 2)}
}

func b2hTabItems() []ui.TabItem {
	return []ui.TabItem{{Key: "a", Label: "概览"}, {Key: "b", Label: "成员"}, {Key: "c", Label: "设置"}, {Key: "d", Label: "日志", Disabled: true}}
}

func init() {
	b2hReset()
	navPage(gallery.Page{Name: "TabsAlign", Title: "Tabs 尺寸、指示条、等分与切换拦截", Group: gallery.Navigation, Order: 3, Parent: "Tabs", Width: 1000, Height: 1180, Render: func(c *ui.Context) {
		d := b2h
		gallery.Demo(c, "尺寸", "Size(Large / Small)：线条标签上下内边距 16 / 8，卡片高度 48 / 32（antd cardHeightLG / SM）。", func() {
			ui.Column(c, func() {
				ui.Tabs(c, &d.tabSize, b2hTabItems()).Size(ui.Large).Show()
				ui.Tabs(c, &d.tabSize, b2hTabItems()).Size(ui.Small).Show()
				ui.Tabs(c, &d.tabSize, b2hTabItems()).Card().Size(ui.Large).Show()
				ui.Tabs(c, &d.tabSize, b2hTabItems()).Card().Size(ui.Small).Show()
			}, layout.Gap(8))
		})
		gallery.Demo(c, "TabPlacement 与 Indicator", "TabPlacement(\"start\")：标签在起始侧；Indicator(16, \"center\")：指示条固定 16px 居中。", func() {
			ui.Tabs(c, &d.tabStart, b2hTabItems()).TabPlacement("start").Indicator(16, "center").Show().Panel(func() {
				ui.Text(c, "当前："+d.tabStart)
			})
		})
		gallery.Demo(c, "Stretch、TabBarGutter 与 BeforeLeave", "Stretch() 等分整行；TabBarGutter(12) 标签间距 12；BeforeLeave 拒绝离开“概览”以外的标签时保持不变。", func() {
			ui.Column(c, func() {
				ui.Tabs(c, &d.tabStretch, b2hTabItems()).Stretch().Show()
				ui.Tabs(c, &d.tabGuard, b2hTabItems()).TabBarGutter(12).BeforeLeave(func(n, o string) bool {
					if o == "b" {
						d.guardMsg = "“成员”有未保存的修改，已拦截切换"
						return false
					}
					d.guardMsg = ""
					return true
				}).Show()
				if d.guardMsg != "" {
					ui.Text(c, d.guardMsg).Type("warning")
				}
			}, layout.Gap(8))
		})
		gallery.Demo(c, "Variant(\"line\")", "shadcn TabsList 的 line 变体：透明列表、选中项下方 2px 前景色下划线；Ant / Element 本来就是线条标签。", func() {
			ui.Tabs(c, &d.tabLine, b2hTabItems()).Variant("line").Show()
		})
	}, Setup: func(d *ui.Driver) { _ = d.ClickLabel("成员") }}, b2hReset)

	dataPage(gallery.Page{Name: "TimelineAlign", Parent: "Timeline", Title: "Timeline v6：标题列、单项位置、加载与实心点", Order: 23, Width: 1000, Height: 860, Render: func(c *ui.Context) {
		ui.Row(c, func() {
			ui.Column(c, func() {
				gallery.Demo(c, "Title 与 TitleSpan(6)", "带 Title 的时间轴居中排布；TitleSpan(6) 让标题列占 6/24。", func() {
					ui.Timeline(c, []ui.TimelineItem{
						{Title: "2026-01-02", Content: "创建项目"},
						{Title: "2026-02-14", Content: "发布 0.1"},
						{Title: "2026-03-20", Content: "规范对齐", Loading: true},
					}).TitleSpan(6).Show()
				})
				gallery.Demo(c, "单项 Placement 与 Loading", "第二项 Placement(\"end\") 放到另一侧；Loading 项是转圈的点，之后的轴线为点线。", func() {
					ui.Timeline(c, []ui.TimelineItem{
						{Content: "提交申请"},
						{Content: "主管审批", Placement: "end"},
						{Content: "财务复核中", Loading: true},
						{Content: "完成", Color: "gray"},
					}).Mode("alternate").Show()
				})
			}, layout.Grow(1), layout.Basis(0))
			ui.Column(c, func() {
				gallery.Demo(c, "Variant(\"filled\") 与 Center", "实心点；Center 让点在多行内容中垂直居中（Element center）。", func() {
					ui.Timeline(c, []ui.TimelineItem{
						{Content: "单行事件", Color: "success"},
						{Body: func() {
							ui.Column(c, func() {
								ui.Text(c, "多行事件")
								ui.Text(c, "第二行说明").Type("secondary")
								ui.Text(c, "第三行说明").Type("secondary")
							})
						}, Center: true},
						{Content: "结束", Color: "error"},
					}).Variant("filled").Show()
				})
				gallery.Demo(c, "Mode(\"alternate-reverse\")", "Element 的反向交替：第一项在轴线前侧。", func() {
					ui.Timeline(c, []ui.TimelineItem{{Content: "第一项"}, {Content: "第二项"}, {Content: "第三项"}}).Mode("alternate-reverse").Show()
				})
			}, layout.Grow(1), layout.Basis(0))
		}, layout.Gap(24), layout.AlignItems(layout.AlignStart))
	}})

	dataPage(gallery.Page{Name: "ImageAlign", Parent: "Image", Title: "Image 预览：缩放范围、到头即停与计数", Order: 33, Width: 1000, Height: 640, Render: func(c *ui.Context) {
		gallery.Demo(c, "预览组（打开）", "Ant：缩放 1–50 倍、每步 ×1.5，首张时“上一张”禁用、方向键不循环，底部显示“1 / 3”；Element：0.2–7 倍、每步 ×1.2、循环，ShowProgress(true) 显示计数。", func() {
			ui.Row(c, func() {
				for i, img := range b2h.imgs {
					ui.Image(c, img).Size(160, 106).Alt(fmt.Sprintf("风景 %d", i+1)).PreviewGroup(b2h.imgs, i).ShowProgress(true).Radius(6)
				}
			}, layout.Gap(12))
		})
	}, Setup: func(d *ui.Driver) { _ = d.ClickLabel("风景 1"); d.Settle() }})

	dataPage(gallery.Page{Name: "CarouselAlign", Parent: "Carousel", Title: "Carousel 指示点位置、播放进度与箭头", Order: 70, Width: 1000, Height: 900, Render: func(c *ui.Context) {
		slide := func(i int) { dataSceneSlide(c, i) }
		ui.Row(c, func() {
			ui.Column(c, func() {
				gallery.Demo(c, "DotDuration 与箭头", "Autoplay + DotDuration(true)：当前指示点随播放间隔填满；Arrows(\"always\") 为 antd 16px 箭头，距边 8px。", func() {
					ui.Carousel(c, 3, slide).Current(&b2h.carDot).Autoplay(3 * time.Second).DotDuration(true).Arrows("always").Height(180).Show()
				})
				gallery.Demo(c, "CardScale(0.7)", "卡片模式两侧缩放到 0.7（Element cards-scale）。", func() {
					ui.Carousel(c, 4, slide).Card().CardScale(0.7).Height(160).Show()
				})
			}, layout.Grow(1), layout.Basis(0))
			ui.Column(c, func() {
				gallery.Demo(c, "DotPlacement(\"start\")", "指示点在起始侧，同时纵向切换（antd v6 dotPlacement）。", func() {
					ui.Carousel(c, 3, slide).Current(&b2h.carStart).DotPlacement("start").Height(180).Show()
				})
				gallery.Demo(c, "Trigger(\"hover\")", "悬停指示点即切换（Element 默认）。", func() {
					ui.Carousel(c, 3, slide).Trigger("hover").DotOutside().Height(120).Show()
				})
			}, layout.Grow(1), layout.Basis(0))
		}, layout.Gap(64), layout.AlignItems(layout.AlignStart), layout.PadVH(0, 48))
	}})

	dataPage(gallery.Page{Name: "CalendarAlign", Parent: "Calendar", Title: "Calendar 可选范围、周数与下拉切换", Order: 71, Width: 1000, Height: 1100, Render: func(c *ui.Context) {
		ui.Column(c, func() {
			gallery.Demo(c, "ValidRange 与 ShowWeek", "只能选 3 月 10 日到 4 月 20 日；左侧是 ISO 周数（antd validRange / showWeek）。", func() {
				ui.Calendar(c, &b2h.week).Card().ShowWeek(true).
					ValidRange(time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC), time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)).
					Layout(layout.W(380))
			})
			gallery.Demo(c, "ControllerType(\"select\")", "Element 日历头部用年 / 月下拉切换（controller-type）；Formatter 改写下拉文字。", func() {
				ui.Calendar(c, &b2h.day).ControllerType("select").Formatter(func(v int, kind string) string {
					if kind == "month" {
						return fmt.Sprintf("%02d 月", v)
					}
					return ""
				}).CellHeight(48).Layout(layout.W(640))
			})
		}, layout.Gap(16))
	}})

	dataPage(gallery.Page{Name: "QRCodeAlign", Parent: "QRCode", Title: "QRCode 纠错提升、留白与无边框", Order: 31, Width: 1000, Height: 360, Render: func(c *ui.Context) {
		gallery.Demo(c, "BoostLevel / MarginSize / Bordered(false)", "默认在版本不变的前提下提升纠错级别（antd boostLevel）；BoostLevel(false) 保持 L；MarginSize(4) 留 4 个模块的静区；无边框时去掉内边距。", func() {
			ui.Row(c, func() {
				ui.QRCode(c, "https://tango.dev").Level("L").Size(140)
				ui.QRCode(c, "https://tango.dev").Level("L").BoostLevel(false).Size(140)
				ui.QRCode(c, "https://tango.dev").MarginSize(4).Size(140)
				ui.QRCode(c, "https://tango.dev").Bordered(false).Size(140)
			}, layout.Gap(16))
		})
	}})

	dataPage(gallery.Page{Name: "WatermarkAlign", Parent: "Watermark", Title: "Watermark antd 排布：旋转包围盒与错列", Order: 34, Width: 1000, Height: 480, Render: func(c *ui.Context) {
		ui.Row(c, func() {
			gallery.Demo(c, "两行文字", "标记按旋转后的包围盒加间距排成网格，每隔一列下移半格（antd useClips）。", func() {
				ui.Watermark(c, "Tango", "内部资料").Body(func() {
					ui.Div(c, layout.W(360), layout.H(300))
				})
			})
			gallery.Demo(c, "斜体与固定标记尺寸", "FontStyle(\"italic\")；MarkSize(140, 22) 固定标记尺寸；Gap(60, 60)。图片标记同样随 Rotate 旋转。", func() {
				ui.Watermark(c, "CONFIDENTIAL").FontStyle("italic").MarkSize(140, 22).Gap(60, 60).Body(func() {
					ui.Div(c, layout.W(360), layout.H(300))
				})
			})
		}, layout.Gap(24))
	}})

	morePage(gallery.Page{Name: "MasonryAlign", Parent: "Masonry", Title: "Masonry 布局变化通知", Group: gallery.Layout, Order: 41, Width: 1000, Height: 520, Render: func(c *ui.Context) {
		d := b2h
		gallery.Demo(c, "LayoutChanged", "列数变化后有项换了列时 LayoutChanged 为真（antd onLayoutChange），Assignment 给出每项所在列。", func() {
			ui.Row(c, func() {
				for _, n := range []int{2, 3, 4} {
					n := n
					b := ui.Button(c, fmt.Sprintf("%d 列", n))
					if d.masonryCols == n {
						b.Primary()
					}
					if b.Clicked() {
						d.masonryCols = n
					}
				}
				ui.Text(c, fmt.Sprintf("布局变化 %d 次", d.masonryChanges)).Type("secondary")
			}, layout.Gap(8), layout.Middle())
			ui.Gap(c, 12)
			hs := []float32{80, 120, 60, 100, 140, 70, 90, 110}
			m := ui.Masonry(c, len(hs), func(i int) { fillTile(c, i, hs[i]) }).Columns(d.masonryCols).Gutter(12).
				ItemHeight(func(i int) float32 { return hs[i] }).Show()
			if m.LayoutChanged() {
				d.masonryChanges++
			}
		})
	}}, b2hReset)
}
