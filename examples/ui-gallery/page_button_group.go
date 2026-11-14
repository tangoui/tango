package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

var buttonAppearances = []struct {
	ap   ui.ButtonAppearance
	name string
}{
	{ui.ButtonSolid, "Solid"}, {ui.ButtonOutlined, "Outlined"}, {ui.ButtonDashed, "Dashed"},
	{ui.ButtonFilled, "Filled"}, {ui.ButtonText, "Text"}, {ui.ButtonLink, "Link"},
}

// buttonComboRows shows the color × appearance matrix (Ant v6) and
// Element's types and plain buttons on the Button page.
func buttonComboRows(c *ui.Context) {
	ui.Text(c, "颜色 × 样式（Color / Appearance）").Secondary().Layout(layout.Margin(geomBottom(8)))
	for _, col := range []struct {
		col  ui.ButtonColor
		name string
	}{{ui.ButtonColorDefault, "默认"}, {ui.ButtonColorPrimary, "主色"}, {ui.ButtonColorDanger, "危险"}} {
		row(c, func() {
			for _, a := range buttonAppearances {
				ui.Button(c, col.name+" "+a.name).Color(col.col).Appearance(a.ap)
			}
		})
	}
	ui.Text(c, "预设色").Secondary().Layout(layout.Margin(geomBottom(8)))
	for _, ap := range []ui.ButtonAppearance{ui.ButtonSolid, ui.ButtonOutlined, ui.ButtonFilled, ui.ButtonText} {
		row(c, func() {
			for _, col := range ui.ButtonPresetColors {
				ui.Button(c, string(col)).Color(col).Appearance(ap).Small()
			}
		})
	}
	ui.Text(c, "类型与朴素按钮（Element type / plain）").Secondary().Layout(layout.Margin(geomBottom(8)))
	row(c, func() {
		ui.Button(c, "Default")
		ui.Button(c, "Primary").Primary()
		ui.Button(c, "Success").Success()
		ui.Button(c, "Info").Info()
		ui.Button(c, "Warning").Warning()
		ui.Button(c, "Danger").Primary().Danger()
	})
	row(c, func() {
		ui.Button(c, "Plain").Plain()
		ui.Button(c, "Primary").Primary().Plain()
		ui.Button(c, "Success").Success().Plain()
		ui.Button(c, "Info").Info().Plain()
		ui.Button(c, "Warning").Warning().Plain()
		ui.Button(c, "Danger").Danger().Plain()
	})
	row(c, func() {
		ui.Button(c, "圆角朴素").Success().Plain().Round()
		ui.Button(c, "").Warning().Plain().Circle().Icon(icon.Star).A11yLabel("收藏")
		ui.Button(c, "禁用").Success().Disabled(true)
		ui.Button(c, "紫色朴素").Color(ui.ButtonColorPurple).Plain()
	})
}

func init() {
	gallery.Register(gallery.Page{Name: "ButtonGroup", Title: "ButtonGroup 按钮组", Group: gallery.General, Order: 1, Parent: "Button",
		Render: func(c *ui.Context) {
			gallery.Demo(c, "基础按钮组", "相邻按钮共用边框，只有首尾保留圆角；组上设置的尺寸、颜色、样式作用于每个按钮，按钮自己的设置优先。", func() {
				row(c, func() {
					ui.ButtonGroup(c).A11yLabel("翻页").Body(func() {
						ui.Button(c, "上一页").Icon(icon.Left)
						ui.Button(c, "下一页").IconRight(icon.Right)
					})
					ui.ButtonGroup(c).Variant(ui.VariantPrimary).A11yLabel("编辑").Body(func() {
						ui.Button(c, "").Icon(icon.Edit).A11yLabel("编辑")
						ui.Button(c, "").Icon(icon.Copy).A11yLabel("复制")
						ui.Button(c, "").Icon(icon.Delete).A11yLabel("删除")
					})
					ui.ButtonGroup(c).Size(ui.Small).A11yLabel("视图").Body(func() {
						ui.Button(c, "日")
						ui.Button(c, "周")
						ui.Button(c, "月").Primary()
					})
				})
			})
			gallery.Demo(c, "统一类型与样式", "Color / Appearance / Variant 设在组上。", func() {
				row(c, func() {
					ui.ButtonGroup(c).Color(ui.ButtonColorSuccess).A11yLabel("成功").Body(func() {
						ui.Button(c, "通过")
						ui.Button(c, "全部通过")
					})
					ui.ButtonGroup(c).Color(ui.ButtonColorPurple).Appearance(ui.ButtonOutlined).A11yLabel("紫色").Body(func() {
						ui.Button(c, "左")
						ui.Button(c, "中")
						ui.Button(c, "右")
					})
					ui.ButtonGroup(c).Color(ui.ButtonColorWarning).Appearance(ui.ButtonPlain).A11yLabel("朴素").Body(func() {
						ui.Button(c, "朴素一")
						ui.Button(c, "朴素二")
					})
					ui.ButtonGroup(c).Size(ui.Large).Appearance(ui.ButtonDashed).A11yLabel("虚线").Body(func() {
						ui.Button(c, "虚线")
						ui.Button(c, "按钮")
					})
				})
			})
			gallery.Demo(c, "竖向排列", "", func() {
				row(c, func() {
					ui.ButtonGroup(c).Vertical().A11yLabel("竖向").Body(func() {
						ui.Button(c, "顶部")
						ui.Button(c, "中间较长")
						ui.Button(c, "底部")
					})
					ui.ButtonGroup(c).Vertical().Variant(ui.VariantPrimary).A11yLabel("竖向主色").Body(func() {
						ui.Button(c, "上移").Icon(icon.Up)
						ui.Button(c, "下移").Icon(icon.Down)
					})
				})
			})
		}})
}
