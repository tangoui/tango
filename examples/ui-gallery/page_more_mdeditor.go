package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
)

var moreMDEDemo struct{ src string }

func moreMDEReset() {
	moreMDEDemo.src = "## 周报\n\n本周完成了 **Markdown 编辑器**，支持工具栏、实时预览与 `任务列表`。\n\n" +
		"- [x] 渲染器\n- [x] 富文本段落\n- [ ] 所见即所得编辑\n\n> 选中文字后点击工具栏按钮即可加粗、加链接。\n\n" +
		"| 模块 | 进度 |\n| --- | ---: |\n| 预览 | 100% |\n| 编辑 | 80% |\n"
}

func init() {
	moreMDEReset()
	morePage(gallery.Page{Name: "MarkdownEditor", Title: "MarkdownEditor Markdown 编辑器", Group: gallery.Form, Order: 80,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "分屏编辑", "左侧源码、右侧实时预览；工具栏对选区加格式，预览中勾选任务会回写源码。", func() {
				ui.MarkdownEditor(c, &moreMDEDemo.src).Height(460)
			})
		},
		Setup: func(d *ui.Driver) { d.Frame(); d.Frame() },
	}, moreMDEReset)
}
