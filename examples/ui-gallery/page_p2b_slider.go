package main

import (
	"fmt"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

// Slider: range, marks and dots, vertical, tooltips, attached input.

var (
	p2bSlLo, p2bSlHi = 20.0, 50.0
	p2bSlMark        = 37.0
	p2bSlDots        = 40.0
	p2bSlTip         = 30.0
	p2bSlIn          = 6.0
	p2bSlV1          = 30.0
	p2bSlVLo         = 20.0
	p2bSlVHi         = 70.0
	p2bSlDisLo       = 10.0
	p2bSlDisHi       = 40.0
	p2bSlNcLo        = 30.0
	p2bSlNcHi        = 60.0
	p2bSlPuLo        = 30.0
	p2bSlPuHi        = 45.0
	p2bSlInLo        = 20.0
	p2bSlInHi        = 80.0
	p2bSlVIn         = 40.0
)

var p2bSlMarks = map[float64]string{0: "0°C", 26: "26°C", 37: "37°C", 100: "100°C"}

func init() {
	entryPage(gallery.Page{Name: "SliderAdvanced", Title: "Slider 范围 / 刻度 / 竖向 / 提示 / 输入框", Order: 66, Width: 1100, Height: 900,
		Render: func(c *ui.Context) {
			ui.Row(c, func() {
				entryCol(c, func() {
					ui.Text(c, "范围选择").Tertiary()
					ui.SliderRange(c, &p2bSlLo, &p2bSlHi, 0, 100).A11yLabel("价格区间")
					ui.Text(c, "刻度标记").Tertiary()
					ui.Slider(c, &p2bSlMark, 0, 100).Marks(p2bSlMarks).A11yLabel("温度")
					ui.Text(c, "步长 10 + 间断点").Tertiary()
					ui.Slider(c, &p2bSlDots, 0, 100).Step(10).Dots().A11yLabel("间断点")
					ui.Text(c, "提示格式化（常显）").Tertiary()
					ui.Gap(c, 28)
					ui.Slider(c, &p2bSlTip, 0, 100).Tooltip(func(v float64) string { return fmt.Sprintf("%.0f%%", v) }).TooltipOpen(true).A11yLabel("音量")
					ui.Text(c, "带输入框（show-input）").Tertiary()
					ui.Slider(c, &p2bSlIn, 0, 10).Step(0.5).WithInput().A11yLabel("等级")
					ui.Text(c, "范围 + 输入框").Tertiary()
					ui.SliderRange(c, &p2bSlInLo, &p2bSlInHi, 0, 100).WithInput().A11yLabel("输入区间")
					ui.Text(c, "不可交叉（AllowCross(false)）").Tertiary()
					ui.SliderRange(c, &p2bSlNcLo, &p2bSlNcHi, 0, 100).AllowCross(false).A11yLabel("不可交叉区间")
					ui.Text(c, "保持最小间距 10（Pushable）").Tertiary()
					ui.SliderRange(c, &p2bSlPuLo, &p2bSlPuHi, 0, 100).Pushable(10).A11yLabel("推动区间")
					ui.Text(c, "禁用").Tertiary()
					ui.SliderRange(c, &p2bSlDisLo, &p2bSlDisHi, 0, 100).Disabled(true).A11yLabel("禁用区间")
				}, layout.W(560), layout.Gap(14))
				ui.Row(c, func() {
					entryCol(c, func() {
						ui.Text(c, "竖向").Tertiary()
						ui.Slider(c, &p2bSlV1, 0, 100).Vertical(300).A11yLabel("竖向")
					})
					entryCol(c, func() {
						ui.Text(c, "竖向范围 + 刻度").Tertiary()
						ui.SliderRange(c, &p2bSlVLo, &p2bSlVHi, 0, 100).Vertical(300).Marks(map[float64]string{0: "0", 50: "50", 100: "100"}).A11yLabel("竖向区间")
					})
					entryCol(c, func() {
						ui.Text(c, "竖向 + 输入框").Tertiary()
						ui.Slider(c, &p2bSlVIn, 0, 100).Vertical(300).WithInput().A11yLabel("竖向输入")
					})
				}, layout.Gap(48), layout.Margin(geom.Insets{Left: 40}))
			}, layout.AlignItems(layout.AlignStart))
		}})
}
