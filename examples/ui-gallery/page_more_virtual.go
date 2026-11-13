package main

import (
	"fmt"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
)

const moreVirtualTotal = 100000

var moreVirtualDemo struct {
	jump    int
	checked map[int]bool
}

func moreVirtualReset() {
	moreVirtualDemo.jump = -1
	moreVirtualDemo.checked = map[int]bool{3: true, 50002: true}
}

func init() {
	moreVirtualReset()
	morePage(gallery.Page{Name: "VirtualList", Title: "VirtualList 虚拟列表", Group: gallery.Display, Order: 61,
		Render: func(c *ui.Context) {
			s := &moreVirtualDemo
			th := c.Theme
			gallery.Demo(c, "十万条数据", "只构建可见行与少量缓冲；行内控件放在按索引的 Key 作用域里，滚动后状态不串。", func() {
				moreRow(c, func() {
					for _, j := range []int{0, 50000, moreVirtualTotal - 1} {
						if ui.Button(c, fmt.Sprintf("跳到第 %d 条", j+1)).Clicked() {
							s.jump = j
						}
					}
				})
				ui.Gap(c, 12)
				var vl *ui.VirtualListW
				ui.Column(c, func() {
					vl = ui.VirtualList(c, moreVirtualTotal, 40, func(i int) {
						ui.Row(c, func() {
							on := s.checked[i]
							if ui.Checkbox(c, "", &on).A11yLabel(fmt.Sprintf("选择第 %d 条记录", i+1)).Changed() {
								s.checked[i] = on
							}
							ui.Text(c, fmt.Sprintf("第 %d 条记录", i+1)).Layout(layout.Grow(1))
							ui.Text(c, fmt.Sprintf("#%06d", i)).Tertiary()
						}, layout.Gap(12), layout.Middle(), layout.PadVH(0, 16), layout.Grow(1)).Paint(func(l *paint.List, r geom.Rect) {
							l.Fill(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Radii{}, th.Split)
						})
					}).Height(320)
				}).Border(1, th.BorderSecondary).Radius(th.RadiusLG).Clip()
				if s.jump >= 0 {
					vl.ScrollToIndex(s.jump)
					s.jump = -1
				}
				first, end := vl.Range()
				ui.Text(c, fmt.Sprintf("本帧构建第 %d–%d 行（共 %d 行）", first+1, end, moreVirtualTotal)).Tertiary().Layout(layout.Margin(geom.Insets{Top: 8}))
			})
		},
		Setup: func(d *ui.Driver) { d.ClickLabel("跳到第 50001 条") },
	}, moreVirtualReset)
}
