package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

const moreMDSample = "# Tango 文档\n\n" +
	"Tango 是一个**自绘**跨平台框架，支持 *强调*、~~删除线~~、==高亮==、`行内代码` 与 [链接](https://example.com)。" +
	"中英文混排会按字符折行，标点不会出现在行首。Mixed English words wrap at spaces, e.g. https://github.com/tangoui/tango.\n\n" +
	"## 列表\n\n" +
	"- 第一项\n- 第二项，带 **粗体**\n  - 嵌套项 A\n  - 嵌套项 B\n- 第三项\n\n" +
	"1. 安装依赖\n2. 运行 `go run ./examples/ui-gallery`\n3. 查看截图\n\n" +
	"- [x] 已完成的任务\n- [ ] 待办任务\n\n" +
	"## 引用与代码\n\n" +
	"> 引用块中的文字使用次要颜色。\n> 第二行会接在后面。\n\n" +
	"```go\nfunc main() {\n\tfmt.Println(\"你好，Tango\")\n}\n```\n\n" +
	"## 表格\n\n" +
	"| 组件 | 状态 | 数量 |\n|:---|:---:|---:|\n| Markdown | 可用 | 1 |\n| RichText | 可用 | 12 |\n\n" +
	"---\n\n![架构图](arch.png)\n"

var moreMDDemo struct {
	src  string
	link string
}

func moreMDReset() { moreMDDemo.src, moreMDDemo.link = moreMDSample, "" }

func init() {
	moreMDReset()
	morePage(gallery.Page{Name: "Markdown", Title: "Markdown 文档渲染", Group: gallery.Display, Order: 70,
		Render: func(c *ui.Context) {
			s := &moreMDDemo
			gallery.Demo(c, "Markdown", "ui.Markdown 渲染 CommonMark + GFM 常用扩展；ClickedLink / TaskToggled 回报交互。", func() {
				md := ui.Markdown(c, s.src)
				if u := md.ClickedLink(); u != "" {
					s.link = u
				}
				if i, ok := md.TaskToggled(); ok {
					s.src = ui.ToggleMarkdownTask(s.src, i)
				}
			})
		},
	}, moreMDReset)
	morePage(gallery.Page{Name: "RichText", Title: "RichText 富文本段落", Group: gallery.Display, Order: 71,
		Render: func(c *ui.Context) {
			th := c.Theme
			gallery.Demo(c, "混合样式", "一个段落里混排粗体、代码、链接、颜色与字号，统一基线、自动折行。", func() {
				ui.Column(c, func() {
					ui.RichText(c,
						ui.Span{Text: "RichText 支持 "},
						ui.Span{Text: "粗体", Bold: true},
						ui.Span{Text: "、"},
						ui.Span{Text: "删除线", Strike: true},
						ui.Span{Text: "、"},
						ui.Span{Text: "下划线", Underline: true},
						ui.Span{Text: "、"},
						ui.Span{Text: "高亮", Mark: true},
						ui.Span{Text: "、"},
						ui.Span{Text: "inline code", Code: true},
						ui.Span{Text: "、"},
						ui.Span{Text: "彩色文字", Color: th.Error.Base},
						ui.Span{Text: "、"},
						ui.Span{Text: "大号", Size: 20, Bold: true},
						ui.Span{Text: "以及"},
						ui.Span{Text: "超链接", Link: "https://example.com"},
						ui.Span{Text: "。长段落会按宽度自动折行，中文逐字断行，英文按单词断行，“引号”和句号等标点遵守避头尾规则。"},
					)
					ui.RichText(c, ui.ParseMarkdownInline("也可以从行内 Markdown 生成：**粗体**、*斜体*、`code`、[链接](https://example.com)。")...)
					ui.RichText(c, ui.Span{Text: "居中对齐的段落", Color: th.TextSecondary}).Align(ui.AlignCenter)
				}, layout.Gap(12), layout.W(560))
			})
		},
	}, nil)
	_ = theme.Ant
}
