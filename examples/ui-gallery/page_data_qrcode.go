package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/theme"
)

var dataQRText = "https://github.com/tangoui/tango"

func init() {
	dataPage(gallery.Page{Name: "QRCode", Title: "QRCode 二维码", Order: 30, Render: func(c *ui.Context) {
		gallery.Demo(c, "基本使用", "ISO/IEC 18004 自实现编码；带 Logo 时建议使用 H 级纠错。", func() {
			dataRow(c, func() {
				ui.QRCode(c, dataQRText)
				ui.QRCode(c, dataQRText).Level("H").Icon(dataImgAvatar)
				ui.QRCode(c, "Tango 中文二维码 ✓").Bordered(false)
			})
		})
		gallery.Demo(c, "状态", "active / expired / loading / scanned", func() {
			dataRow(c, func() {
				ui.QRCode(c, dataQRText).Status("expired")
				ui.QRCode(c, dataQRText).Status("loading")
				ui.QRCode(c, dataQRText).Status("scanned")
			})
		})
		gallery.Demo(c, "颜色与尺寸 / 纠错等级", "自定义前景/背景色；L、M、Q、H 版本逐级变大。", func() {
			dataRow(c, func() {
				ui.QRCode(c, dataQRText).Color(c.Theme.Success.Base).Size(120)
				ui.QRCode(c, dataQRText).Color(theme.Hex("#1677ff")).Bg(theme.Hex("#f5f5f5")).Size(120)
				for _, l := range []string{"L", "M", "Q", "H"} {
					ui.QRCode(c, dataQRText).Level(l).Size(120)
				}
			})
		})
	}})
}
