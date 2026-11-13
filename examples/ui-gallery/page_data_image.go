package main

import (
	"image"
	"image/color"
	"math"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

// dataImgScene paints a procedural landscape (no bundled assets): a sky
// gradient, a sun and two mountain ranges; seed varies the palette.
func dataImgScene(w, h int, seed int) *image.RGBA {
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	skies := [][2]color.RGBA{
		{{79, 172, 254, 255}, {0, 242, 254, 255}},
		{{250, 112, 154, 255}, {254, 225, 64, 255}},
		{{102, 126, 234, 255}, {118, 75, 162, 255}},
		{{67, 233, 123, 255}, {56, 249, 215, 255}},
	}
	sky := skies[seed%len(skies)]
	lerp := func(a, b uint8, t float64) uint8 { return uint8(float64(a) + (float64(b)-float64(a))*t) }
	sunX, sunY, sunR := float64(w)*(0.7-0.1*float64(seed%3)), float64(h)*0.3, float64(h)*0.12
	for y := 0; y < h; y++ {
		t := float64(y) / float64(h)
		for x := 0; x < w; x++ {
			c := color.RGBA{lerp(sky[0].R, sky[1].R, t), lerp(sky[0].G, sky[1].G, t), lerp(sky[0].B, sky[1].B, t), 255}
			if d := math.Hypot(float64(x)-sunX, float64(y)-sunY); d < sunR {
				c = color.RGBA{255, 244, 214, 255}
			} else if d < sunR*1.6 {
				k := (d - sunR) / (sunR * 0.6)
				c = color.RGBA{lerp(255, c.R, k), lerp(244, c.G, k), lerp(214, c.B, k), 255}
			}
			fx := float64(x) / float64(w)
			far := 0.55 + 0.12*math.Sin(fx*7+float64(seed)) + 0.05*math.Sin(fx*23)
			near := 0.72 + 0.1*math.Sin(fx*4.3+2+float64(seed)) + 0.03*math.Sin(fx*31)
			if t > near {
				k := (t - near) * 2
				c = color.RGBA{lerp(46, 20, k), lerp(110, 60, k), lerp(80, 50, k), 255}
			} else if t > far {
				c = color.RGBA{lerp(c.R, 70, 0.7), lerp(c.G, 100, 0.7), lerp(c.B, 130, 0.7), 255}
			}
			m.SetRGBA(x, y, c)
		}
	}
	return m
}

var dataImgs = func() []image.Image {
	out := make([]image.Image, 4)
	for i := range out {
		out[i] = dataImgScene(800, 520, i)
	}
	return out
}()

var dataImgAvatar = dataImgScene(96, 96, 2)

func dataImageBasics(c *ui.Context) {
	gallery.Demo(c, "基础用法", "可预览的图片，点击放大；悬停出现预览遮罩（Ant）。", func() {
		dataRow(c, func() {
			ui.Image(c, dataImgs[0]).Width(200).Preview(true).Alt("风景")
			ui.Image(c, dataImgs[1]).Size(200, 130).Fit("cover").Radius(8).Preview(true)
		})
	})
	gallery.Demo(c, "填充模式 Fit", "fill / contain / cover / scale-down", func() {
		dataRow(c, func() {
			for _, f := range []string{"fill", "contain", "cover", "scale-down"} {
				ui.Column(c, func() {
					ui.Div(c).Bg(c.Theme.FillTertiary).Body(func() {
						ui.Image(c, dataImgs[2]).Size(120, 120).Fit(f)
					})
					ui.Text(c, f).Tertiary()
				}, layout.Gap(4))
			}
		})
	})
	gallery.Demo(c, "加载占位与失败兜底", "加载中显示占位，加载失败显示兜底图。", func() {
		dataRow(c, func() {
			ui.Image(c, nil).Size(200, 130).Loading(true)
			ui.Image(c, nil).Size(200, 130).Failed(true)
			ui.Image(c, dataImgs[3]).Size(200, 130).Failed(true).Fallback(dataImgAvatar).Fit("contain")
		})
	})
	gallery.Demo(c, "多图预览", "PreviewGroup：左右切换并显示序号。", func() {
		dataRow(c, func() {
			for i, img := range dataImgs {
				ui.Image(c, img).Size(120, 80).Fit("cover").Radius(4).PreviewGroup(dataImgs, i)
			}
		}, 8)
	})
}

func init() {
	dataPage(gallery.Page{Name: "Image", Title: "Image 图片", Order: 31, Render: dataImageBasics})
	dataPage(gallery.Page{Name: "ImagePreview", Parent: "Image", Title: "Image 图片预览", Order: 32, Width: 1000, Height: 640,
		Render: func(c *ui.Context) {
			dataRow(c, func() {
				for i, img := range dataImgs {
					ui.Image(c, img).Size(160, 100).Fit("cover").PreviewGroup(dataImgs, i).Alt("预览图" + string(rune('A'+i)))
				}
			})
		},
		Setup: func(d *ui.Driver) {
			d.ClickLabel("预览图B")
			d.ClickLabel("向右旋转")
		}})
}
