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

// ---------------------------------------------------------------------------
// Masonry

var fillMasonryHeights = []float32{120, 60, 150, 90, 70, 130, 80, 160, 100, 60, 140, 90, 110, 75, 125, 95}

var fillMasonryDemo struct{ extra int }

func fillMasonryReset() { fillMasonryDemo.extra = 0 }

func fillTile(c *ui.Context, i int, h float32) {
	th := c.Theme
	ui.Custom(c, func(maxW, _ float32) geom.Size { return geom.Size{W: maxW, H: h} }, func(l *paint.List, r geom.Rect) {
		l.Fill(r, geom.Rad(th.RadiusLG), th.FillSecondary)
		l.Stroke(r, geom.Rad(th.RadiusLG), 1, th.BorderSecondary)
		lay := text.Shape(fmt.Sprint(i+1), text.Style{Size: 16, Weight: text.Semibold, LineHeight: 22}, text.Options{NoWrap: true})
		l.Glyphs(geom.Pt(r.X+(r.W-lay.Width)/2, r.Y+(r.H-lay.Height)/2), lay.Glyphs, th.Text)
	})
}

func init() {
	fillMasonryReset()
	morePage(gallery.Page{Name: "Masonry", Title: "Masonry 瀑布流", Group: gallery.Layout, Order: 40, Height: 1300,
		Render: func(c *ui.Context) {
			d := &fillMasonryDemo
			gallery.Demo(c, "按高度依次放入最短列", "列数随断点变化（xs 2 / md 4），间距 16；新增项用 ItemColumn 固定列，已有项不跳动。", func() {
				n := len(fillMasonryHeights) + d.extra
				if ui.Button(c, "添加一项").Clicked() {
					d.extra++
				}
				ui.Gap(c, 12)
				ui.Masonry(c, n, func(i int) { fillTile(c, i, fillMasonryHeights[i%len(fillMasonryHeights)]) }).
					ColumnsAt(ui.XS, 2).ColumnsAt(ui.MD, 4).Gutter(16).
					ItemHeight(func(i int) float32 { return fillMasonryHeights[i%len(fillMasonryHeights)] }).Show()
			})
			gallery.Demo(c, "虚拟化（10 万项）", "高度未知的项先按估计值排布，绘制后按实测高度重排；只构建视口附近的项。", func() {
				var m *ui.MasonryW
				ui.Column(c, func() {
					m = ui.Masonry(c, 100000, func(i int) {
						ui.Column(c, func() {
							ui.Text(c, fmt.Sprintf("卡片 %d", i+1)).Strong()
							ui.Paragraph(c, "瀑布流里的文字卡片，长度不同、高度由内容决定。"+map[bool]string{true: "这一张多写了一行，让高度不一样。", false: ""}[i%3 == 0])
						}, layout.Pad(12)).Bg(c.Theme.BgContainer).Border(1, c.Theme.BorderSecondary).Radius(c.Theme.RadiusLG)
					}).Columns(5).Gutter(12).EstimatedHeight(110).Virtual(360).Show()
				})
				ui.Text(c, fmt.Sprintf("本帧构建 %d 项", m.Built())).Tertiary().Layout(layout.Margin(geom.Insets{Top: 8}))
			})
		},
	}, fillMasonryReset)
}

// ---------------------------------------------------------------------------
// Listy

var fillListyNames = []string{"Alice", "Amber", "Bob", "Bella", "Carol", "Chen Wei", "David", "Diana", "Ethan", "Eva", "Frank", "Grace", "Henry", "Ivy", "Jack", "Kate"}

type fillListyState struct {
	n       int
	loading bool
	at      time.Time
	jump    bool
}

var fillListyDemo fillListyState

func fillListyReset() { fillListyDemo = fillListyState{n: 30} }

func init() {
	fillListyReset()
	morePage(gallery.Page{Name: "Listy", Title: "Listy 高性能列表", Group: gallery.Display, Order: 62, Height: 1000,
		Render: func(c *ui.Context) {
			d := &fillListyDemo
			th := c.Theme
			ui.Row(c, func() {
				ui.Column(c, func() {
					gallery.Demo(c, "虚拟滚动 + 分组粘性标题", "10 万条按首字母分组，分组标题吸顶。", func() {
						if ui.Button(c, "滚动到 K 组").Clicked() {
							d.jump = true
						}
						ui.Gap(c, 8)
						ui.Column(c, func() {
							l := ui.Listy(c, 100000, func(i int) {
								ui.Row(c, func() {
									ui.Avatar(c, fillListyNames[i%len(fillListyNames)][:1]).Size(32)
									ui.Column(c, func() {
										ui.Text(c, fmt.Sprintf("%s #%d", fillListyNames[i%len(fillListyNames)], i+1))
										ui.Text(c, "user@example.com").Tertiary().Size(th.FontSizeSM)
									})
								}, layout.Gap(12), layout.Middle())
							}).Group(func(i int) string { return fillListyNames[i%len(fillListyNames)][:1] }, func(k string, n int) {
								ui.Text(c, fmt.Sprintf("%s · %d", k, n)).Weight(text.Semibold)
							}).Sticky().Virtual().Height(360)
							if d.jump {
								l.ScrollToGroup("K", ui.AlignTop, 0)
								d.jump = false
							}
							l.Show()
						}).Border(1, th.BorderSecondary).Radius(th.RadiusLG).Clip()
					})
				}, layout.Grow(1), layout.Basis(0))
				ui.Column(c, func() {
					gallery.Demo(c, "富内容与加载更多", "行高由内容决定；滚动到底部触发加载。", func() {
						ui.Column(c, func() {
							l := ui.Listy(c, d.n, func(i int) {
								ui.Text(c, fmt.Sprintf("通知 %d", i+1)).Strong()
								if i%3 == 0 {
									ui.Paragraph(c, "这是一条较长的通知内容，用于演示可变行高：Listy 会在绘制后测量每行高度并修正滚动范围。")
								} else {
									ui.Text(c, "简短内容").Tertiary()
								}
							}).Virtual().Height(360).LoadMore(d.n < 90, d.loading).Show()
							if l.EndReached() && !d.loading {
								d.loading, d.at = true, c.Now()
							}
							if d.loading {
								if c.Now().Sub(d.at) > 800*time.Millisecond {
									d.loading, d.n = false, d.n+30
									c.Invalidate()
								} else {
									c.WakeAt(d.at.Add(800 * time.Millisecond))
								}
							}
						}).Border(1, th.BorderSecondary).Radius(th.RadiusLG).Clip()
					})
				}, layout.Grow(1), layout.Basis(0))
			}, layout.Gap(16))
		},
		Setup: func(d *ui.Driver) {
			d.ClickLabel("滚动到 K 组")
			if r, ok := d.FindText("C · 6250"); ok {
				d.Scroll(r.Center(), 30)
			}
			d.Settle()
		},
	}, fillListyReset)
}

// ---------------------------------------------------------------------------
// BorderBeam

func init() {
	morePage(gallery.Page{Name: "BorderBeam", Title: "BorderBeam 边框光束", Group: gallery.General, Order: 40,
		Render: func(c *ui.Context) {
			th := c.Theme
			gallery.Demo(c, "沿边框流动的光束", "颜色、速度、宽度、长度、数量可配；NoAnimations 时静止，ReduceMotion 时隐藏。", func() {
				ui.Row(c, func() {
					ui.BorderBeam(c).Body(func() {
						ui.Card(c, "AI 助手").Layout(layout.W(240)).Body(func() { ui.Text(c, "默认：主色，6 秒一圈").Tertiary() })
					})
					ui.BorderBeam(c).Gradient(ui.BeamStop{Color: theme.Hex("#1677ff"), Percent: 0}, ui.BeamStop{Color: theme.Hex("#36cfc9"), Percent: 52}, ui.BeamStop{Color: theme.Hex("#95de64"), Percent: 100}).
						LineWidth(2).Size(160).Duration(3 * time.Second).Body(func() {
						ui.Card(c, "Ocean 渐变").Layout(layout.W(240)).Body(func() { ui.Text(c, "2px、160px、3 秒").Tertiary() })
					})
					ui.BorderBeam(c).Color(th.Error.Base).Count(3).Size(56).Body(func() {
						ui.Card(c, "多条光束").Layout(layout.W(240)).Body(func() { ui.Text(c, "Count(3)").Tertiary() })
					})
				}, layout.Gap(24), layout.WrapItems())
				ui.Gap(c, 24)
				b := ui.BorderBeam(c).Radius(th.Radius).Active(fillBeamHover).Body(func() {
					ui.Button(c, "悬停显示光束").Primary()
				})
				fillBeamHover = b.Hovered()
			})
		},
	}, nil)
}

var fillBeamHover bool
