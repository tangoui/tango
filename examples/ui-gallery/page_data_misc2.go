package main

import (
	"fmt"
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

var (
	dataTypoTitle  = "这是一段可编辑的文本"
	dataTypoEdit   = "编辑中的文本"
	dataTypoOpen   bool
	dataTypoOpen2  = true
	dataCalVal     = time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	dataCalVal2    = time.Date(2026, 1, 8, 0, 0, 0, 0, time.UTC)
	dataCalMode    = "month"
	dataCalYear    = "year"
	dataTourOpen   = true
	dataTourCur    = 1
	dataTourTarget [3]ui.ID
	dataCarCur     = 0
	dataPopOpen    = true
)

const dataLongText = "Ant Design，一套企业级 UI 设计语言和 React 组件库。蚂蚁集团体验技术部出品，致力于提升设计者与开发者的研发体验。它提炼自企业级中后台产品的交互语言和视觉风格，开箱即用的高质量组件，共享的设计资源，以及全链路的开发工具和国际化支持。Element Plus 则是基于 Vue 3 的桌面端组件库，面向设计师和开发者。"

func init() {
	dataPage(gallery.Page{Name: "TypographyEditable", Parent: "Typography", Title: "Typography 可复制与可编辑", Order: 66, Render: func(c *ui.Context) {
		th := c.Theme
		ui.Row(c, func() { ui.Text(c, "这是一段可以复制的文本。").Copyable() })
		ui.Column(c, nil, layout.H(12))
		ui.Row(c, func() { ui.Text(c, "复制自定义内容（复制后显示对勾）").CopyText("custom payload") })
		ui.Column(c, nil, layout.H(12))
		ui.Row(c, func() { ui.Text(c, dataTypoTitle).Editable(&dataTypoTitle) })
		ui.Column(c, nil, layout.H(12))
		ui.Row(c, func() { ui.Text(c, dataTypoEdit).Editable(&dataTypoEdit) })
		ui.Column(c, nil, layout.H(12))
		ui.Row(c, func() { ui.Text(c, "可编辑且可复制").Editable(&dataTypoTitle).Copyable() })
		ui.Column(c, nil, layout.H(20))
		ui.Title(c, "多行省略 · 展开", 5)
		ui.Paragraph(c, dataLongText).Lines(2).Expandable(&dataTypoOpen)
		ui.Title(c, "已展开 · 收起", 5)
		ui.Paragraph(c, dataLongText).Lines(2).Expandable(&dataTypoOpen2)
		ui.Text(c, "单行省略："+dataLongText).Ellipsis().Color(th.TextSecondary)
	}, Setup: func(d *ui.Driver) {
		// Copy (shows the check + tooltip), then start editing a text.
		if r, ok := d.Find("复制自定义内容（复制后显示对勾）"); ok {
			d.Click(geom.Pt(r.Right()-8, r.Y+r.H/2))
		}
		if r, ok := d.Find(dataTypoEdit); ok {
			d.Click(geom.Pt(r.Right()-8, r.Y+r.H/2))
		}
		d.Settle()
	}})

	dataPage(gallery.Page{Name: "Popover", Title: "Popover 气泡卡片", Order: 67, Height: 720, Width: 1100, Render: func(c *ui.Context) {
		content := func() {
			ui.Text(c, "气泡卡片内容")
			ui.Text(c, "更多信息")
		}
		pop := func(place string) {
			ui.Popover(c).Title("标题").Content(content).Placement(place).Trigger("manual").Open(&dataPopOpen).Body(func() {
				ui.Button(c, place).Layout(layout.W(96))
			})
		}
		ui.Column(c, func() {
			ui.Row(c, func() {
				pop("bottomLeft")
				pop("bottom")
				pop("bottomRight")
			}, layout.Gap(200), layout.JustifyContent(layout.JustifyCenter))
			ui.Row(c, func() {
				ui.Column(c, func() { pop("right") }, layout.W(96))
				ui.Spacer(c)
				ui.Column(c, func() { pop("left") }, layout.W(96))
			}, layout.Margin(geom.Insets{Top: 200}))
			ui.Row(c, func() {
				pop("topLeft")
				pop("top")
				pop("topRight")
			}, layout.Gap(200), layout.JustifyContent(layout.JustifyCenter), layout.Margin(geom.Insets{Top: 200}))
		}, layout.PadVH(0, 24), layout.W(1040))
	}})

	dataPage(gallery.Page{Name: "Tour", Title: "Tour 漫游式引导", Order: 68, Height: 520, Render: func(c *ui.Context) {
		ui.Row(c, func() {
			dataTourTarget[0] = ui.Button(c, "上传").ID()
			dataTourTarget[1] = ui.Button(c, "保存").Primary().ID()
			dataTourTarget[2] = ui.Button(c, "更多操作").ID()
		}, layout.Gap(8))
		ui.Divider(c)
		ui.Text(c, "Tour 用遮罩挖洞高亮目标，面板带箭头指向目标，底部为步骤指示与上一步/下一步。").Tertiary()
		ui.Text(c, "每步可单独设置 Mask / Type / 按钮文字；IndicatorsRender、ActionsRender 自定义指示与操作区；切换步骤时目标自动滚入可见区。").Tertiary()
		steps := []ui.TourStep{
			{Target: dataTourTarget[0], Title: "上传文件", Description: "把文件放在这里。", NextText: "知道了，下一步"},
			{Target: dataTourTarget[1], Title: "保存", Description: "保存你的修改，支持快捷键 Ctrl+S。", Cover: func() {
				ui.Custom(c, func(maxW, _ float32) geom.Size { return geom.Size{W: maxW, H: 120} }, func(l *paint.List, r geom.Rect) {
					l.Clip(r, geom.Rad(6))
					dataPaintScene(l, r, 2)
					l.Pop()
				})
			}},
			{Target: dataTourTarget[2], Title: "其他操作", Description: "这一步是主色面板、没有遮罩。", Placement: "right",
				Type: ui.TourTypePrimary, Mask: ui.TourMaskOff, PrevText: "返回"},
		}
		ui.Tour(c, &dataTourOpen, &dataTourCur, steps).
			IndicatorsRender(func(cur, total int) {
				ui.Text(c, fmt.Sprintf("第 %d 步，共 %d 步", cur+1, total)).Size(12).Tertiary()
			}).
			ActionsRender(func(a *ui.TourActions) {
				if a.Current < a.Total-1 && ui.Button(c, "跳过").Small().Text().Clicked() {
					a.Close()
				}
				a.Default()
			}).Show()
	}})

	dataPage(gallery.Page{Name: "Carousel", Title: "Carousel 走马灯", Order: 69, Render: func(c *ui.Context) {
		th := c.Theme
		slide := func(i int) {
			bg := theme.Hex("#364d79")
			fg := theme.Hex("#ffffff")
			if th.Flavor == theme.Element {
				bg = theme.Hex("#99a9bf")
				if i%2 == 1 {
					bg = theme.Hex("#d3dce6")
				}
				fg = theme.Hex("#475669")
			}
			ui.Column(c, func() {
				ui.Text(c, string(rune('1'+i))).Size(24).Color(fg)
			}, layout.Grow(1), layout.Center()).Bg(bg)
		}
		ui.Carousel(c, 4, slide).Current(&dataCarCur).Autoplay(3 * time.Second).Show()
		ui.Column(c, nil, layout.H(24))
		ui.Row(c, func() {
			ui.Column(c, func() {
				ui.Carousel(c, 4, slide).Arrows("always").Height(200).Show()
			}, layout.Grow(1))
			ui.Column(c, func() {
				ui.Carousel(c, 4, func(i int) { dataSceneSlide(c, i) }).Fade().Height(200).Arrows("always").Show()
			}, layout.Grow(1))
		}, layout.Gap(16))
		ui.Column(c, nil, layout.H(24))
		ui.Row(c, func() {
			ui.Column(c, func() {
				ui.Carousel(c, 4, slide).Vertical().Height(160).Show()
			}, layout.Grow(1))
			ui.Column(c, func() {
				ui.Carousel(c, 4, slide).DotPosition("outside").Height(160).Show()
			}, layout.Grow(1))
		}, layout.Gap(16))
		ui.Column(c, nil, layout.H(24))
		ui.Row(c, func() {
			ui.Column(c, func() {
				ui.Text(c, "Draggable：拖动或滑动切换，未过阈值回弹").Secondary()
				ui.Carousel(c, 4, slide).Draggable(true).Height(160).Show()
			}, layout.Grow(1), layout.Gap(8))
			ui.Column(c, func() {
				ui.Text(c, "Loop(false)：首尾不绕回，箭头在两端禁用").Secondary()
				ui.Carousel(c, 4, slide).Draggable(true).Loop(false).Arrows("always").Height(160).Show()
			}, layout.Grow(1), layout.Gap(8))
		}, layout.Gap(16))
	}, Setup: func(d *ui.Driver) {
		// Advance autoplay mid-transition is not deterministic; just move
		// to slide 2 by clicking its indicator-free next arrow.
		if r, ok := d.Find("下一张"); ok {
			d.Click(r.Center())
		}
	}})

	dataPage(gallery.Page{Name: "Calendar", Title: "Calendar 日历", Order: 70, Width: 1100, Render: func(c *ui.Context) {
		th := c.Theme
		events := map[int][]struct{ st, s string }{
			8:  {{"warning", "常规事件"}, {"success", "已完成"}},
			10: {{"warning", "常规事件"}, {"success", "已完成"}, {"error", "故障"}},
			15: {{"warning", "季度复盘"}, {"processing", "评审会"}, {"error", "上线窗口"}},
			22: {{"success", "发布"}},
		}
		ui.Calendar(c, &dataCalVal).Mode(&dataCalMode).CellRender(func(d time.Time) {
			if d.Month() != time.January {
				return
			}
			for _, e := range events[d.Day()] {
				ui.Badge(c).Status(e.st, e.s)
			}
		})
		ui.Column(c, nil, layout.H(24))
		ui.Row(c, func() {
			ui.Column(c, func() {
				ui.Calendar(c, &dataCalVal2).Card().DisabledDate(func(d time.Time) bool { return d.Weekday() == time.Sunday })
			}, layout.W(320))
			ui.Column(c, func() {
				ui.Calendar(c, &dataCalVal2).Mode(&dataCalYear).CellHeight(80).MonthCellRender(func(m time.Time) {
					if m.Month() == time.March || m.Month() == time.September {
						ui.Text(c, "1394").Size(28).Color(th.TextHeading)
						ui.Text(c, "Backlog number").Tertiary()
					}
				})
			}, layout.Grow(1))
		}, layout.Gap(24), layout.AlignItems(layout.AlignStart))
	}})
}

func dataSceneSlide(c *ui.Context, i int) {
	ui.Custom(c, func(maxW, maxH float32) geom.Size { return geom.Size{W: maxW, H: maxH} }, func(l *paint.List, r geom.Rect) {
		dataPaintScene(l, r, i)
		lay := text.Shape("风景 "+string(rune('1'+i)), text.Style{Size: 18, LineHeight: 26}, text.Options{NoWrap: true})
		l.Glyphs(geom.Pt(r.X+16, r.Bottom()-36), lay.Glyphs, theme.Hex("#ffffff"))
	}, layout.Grow(1))
}
