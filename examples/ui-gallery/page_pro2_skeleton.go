package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
)

func pro2SkeletonPage(c *ui.Context) {
	gallery.Demo(c, "列表页 list", "默认 2 个统计位（按 Pro v3 浏览器实际行为），工具栏与列表行独立控制。", func() {
		pro2Canvas(c, func() { ui.ProSkeleton(c, ui.SkeletonList).List(3).Active(false).Show() })
	})
	gallery.Demo(c, "toolbar only", "List(0) 只隐藏列表行；工具栏单独由 Toolbar 控制。", func() {
		pro2Canvas(c, func() { ui.ProSkeleton(c, ui.SkeletonList).PageHeader(false).Statistic(0).List(0).Active(false).Show() })
	})
	gallery.Demo(c, "结果页 result", "", func() {
		pro2Canvas(c, func() { ui.ProSkeleton(c, ui.SkeletonResult).PageHeader(false).Active(false).Show() })
	})
	gallery.Demo(c, "详情页 descriptions", "", func() {
		pro2Canvas(c, func() { ui.ProSkeleton(c, ui.SkeletonDescriptions).Active(false).Show() })
	})
}

func init() {
	pro2Page(gallery.Page{Name: "ProSkeleton", Title: "ProSkeleton 页面骨架屏", Group: gallery.Display, Order: 62, Width: 1000,
		Render: pro2SkeletonPage}, nil)
}
