package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
)

var moreTagDemo struct {
	basic [3]bool
	hot   [4]bool
	typed [5]bool
	dis   [2]bool
}

func moreTagReset() {
	moreTagDemo.basic = [3]bool{true, false, false}
	moreTagDemo.hot = [4]bool{false, true, true, false}
	moreTagDemo.typed = [5]bool{true, true, true, true, true}
	moreTagDemo.dis = [2]bool{true, false}
}

func init() {
	moreTagReset()
	morePage(gallery.Page{Name: "CheckableTag", Title: "CheckableTag 可选择标签", Group: gallery.Display, Order: 31,
		Render: func(c *ui.Context) {
			s := &moreTagDemo
			gallery.Demo(c, "基本用法", "点击切换选中状态，类似 Checkbox。", func() {
				moreRow(c, func() {
					for i, l := range []string{"Tag1", "Tag2", "Tag3"} {
						ui.CheckableTag(c, l, &s.basic[i])
					}
				})
			})
			gallery.Demo(c, "热门标签", "选择感兴趣的分类；指针悬停时高亮（Ant 文字变主色，Element 背景加深）。", func() {
				moreRow(c, func() {
					ui.Text(c, "分类：").Secondary()
					for i, l := range []string{"电影", "图书", "音乐", "体育"} {
						ui.CheckableTag(c, l, &s.hot[i])
					}
				})
			})
			gallery.Demo(c, "类型与图标", "Type 指定选中态色板；Icon 在文字前放图标。", func() {
				moreRow(c, func() {
					for i, ty := range []string{"primary", "success", "info", "warning", "error"} {
						ui.CheckableTag(c, ty, &s.typed[i]).Type(ty)
					}
					ui.CheckableTag(c, "收藏", &s.basic[2]).Icon(icon.Star)
				})
			})
			gallery.Demo(c, "禁用", "", func() {
				moreRow(c, func() {
					ui.CheckableTag(c, "已选禁用", &s.dis[0]).Disabled(true)
					ui.CheckableTag(c, "未选禁用", &s.dis[1]).Disabled(true)
				})
			})
		},
		Setup: func(d *ui.Driver) {
			if r, ok := d.Find("体育"); ok {
				d.Move(r.Center())
			}
		},
	}, moreTagReset)
}
