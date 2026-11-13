package main

import (
	"fmt"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
)

var moreInfDemo struct {
	a, b     int
	loadingB bool
	loads    int
	totalB   int
}

func moreInfReset() {
	moreInfDemo.a, moreInfDemo.b = 8, 8
	moreInfDemo.loadingB, moreInfDemo.loads, moreInfDemo.totalB = false, 0, 24
}

// moreInfRow draws one list item like Ant List / Element demos.
func moreInfRow(c *ui.Context, i int) {
	ui.Key(c, i, func() {
		ui.Row(c, func() {
			ui.Avatar(c, string(rune('A'+i%26))).Size(32)
			ui.Column(c, func() {
				ui.Text(c, fmt.Sprintf("用户 %02d", i+1)).Strong()
				ui.Text(c, "滚动到底部时自动加载下一页数据。").Tertiary()
			}, layout.Grow(1))
		}, layout.Gap(12), layout.Middle(), layout.PadVH(12, 16)).Paint(func(l *paint.List, r geom.Rect) {
			l.Fill(geom.Rect{X: r.X + 16, Y: r.Bottom() - 1, W: r.W - 32, H: 1}, geom.Radii{}, c.Theme.Split)
		})
	})
}

// moreInfFrame wraps a list in a bordered, clipped box.
func moreInfFrame(c *ui.Context, fn func()) {
	ui.Column(c, fn, layout.Grow(1)).Border(1, c.Theme.BorderSecondary).Radius(c.Theme.RadiusLG).Clip()
}

func init() {
	moreInfReset()
	morePage(gallery.Page{Name: "InfiniteScroll", Title: "InfiniteScroll 无限滚动", Group: gallery.Display, Order: 60,
		Render: func(c *ui.Context) {
			s := &moreInfDemo
			gallery.Demo(c, "同步追加", "NeedMore() 在接近底部（Distance 内）时返回 true，每个内容高度只触发一次；这里每次追加 8 条。", func() {
				moreInfFrame(c, func() {
					inf := ui.InfiniteScroll(c, func() {
						for i := 0; i < s.a; i++ {
							moreInfRow(c, i)
						}
					}).Height(260).Distance(20).Done(s.a >= 40)
					if inf.NeedMore() {
						s.a += 8
					}
				})
			})
			gallery.Demo(c, "加载中 / 没有更多", "Loading(true) 时显示加载指示且不再触发；Done(true) 后显示“没有更多了”。", func() {
				ui.Row(c, func() {
					moreInfFrame(c, func() {
						ui.InfiniteScroll(c, func() {
							for i := 0; i < 3; i++ {
								moreInfRow(c, i)
							}
						}).Height(260).Loading(true)
					})
					moreInfFrame(c, func() {
						inf := ui.InfiniteScroll(c, func() {
							for i := 0; i < s.b; i++ {
								moreInfRow(c, i)
							}
						}).Height(260).Done(s.b >= s.totalB)
						if inf.NeedMore() {
							s.b += 8
						}
					})
				}, layout.Gap(24))
			})
		},
		Setup: func(d *ui.Driver) {
			for i := 0; i < 6; i++ {
				d.Frame()
			}
			// Scroll the right list to its end.
			p := geom.Pt(700, 640)
			for i := 0; i < 6; i++ {
				d.Scroll(p, 400)
				d.Frame()
			}
		},
	}, moreInfReset)
}
