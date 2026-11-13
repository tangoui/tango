package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
)

func pro2SkeletonPage(c *ui.Context) {
	gallery.Demo(c, "列表页 list", "页头、统计、工具栏与列表行。", func() {
		pro2Canvas(c, func() { ui.ProSkeleton(c, ui.SkeletonList).Statistic(4).List(3).Active(false).Show() })
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
