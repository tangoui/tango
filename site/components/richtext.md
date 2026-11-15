---
title: RichText 富文本
---
# RichText 富文本

在同一段落中混排粗体、斜体、代码、颜色、链接等行内样式，并按可用宽度自动换行。

## 何时使用

- 展示带有少量行内强调、代码、标记或链接的说明文字。
- 需要用户选择和复制一段格式化文本时使用 `RichText`。
- 需要标题、列表、代码块和表格等块级结构时用 [Markdown 文档](/components/markdown)；只显示普通单色文字时用 [Text 文本](/components/text)。

## 在线体验

<GalleryPreview component="RichText" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    ui.RichText(c,
        ui.Span{Text: "这是一段 "},
        ui.Span{Text: "加粗文字", Bold: true},
        ui.Span{Text: "，以及 "},
        ui.Span{Text: "行内代码", Code: true},
        ui.Span{Text: "。"},
    )
}
```

每个 `Span` 表示一段连续文本；`RichText` 会共享段落基线并自动折行，中文按字符、英文按单词处理。

## 常见用法

### 组合强调、标记和不同字号

`Span` 可以组合 `Bold`、`Italic`、`Strike`、`Underline`、`Mark`、`Code` 和 `Mono`；`Size` 只覆盖当前 span 的字号。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/theme"
)

func page(c *ui.Context) {
    ui.RichText(c,
        ui.Span{Text: "重要：", Bold: true, Color: c.Theme.Error.Base},
        ui.Span{Text: "请在 "},
        ui.Span{Text: "保存前", Mark: true, Underline: true},
        ui.Span{Text: "检查 "},
        ui.Span{Text: "config.yaml", Code: true},
        ui.Span{Text: "。"},
        ui.Span{Text: "次要说明", Italic: true, Size: 13, Color: theme.Fade(c.Theme.Text, 0.65)},
    )
}
```

### 从行内 Markdown 生成

`ParseMarkdownInline` 把常用行内 Markdown 转成 `[]Span`，适合服务端已经提供轻量格式而不需要完整块级解析的场景。

```go
import "github.com/tangoui/tango/ui"

const inline = "支持 **粗体**、*斜体*、~~删除线~~、==高亮==、`code` 和 [链接](https://example.com)。"

func page(c *ui.Context) {
    ui.RichText(c, ui.ParseMarkdownInline(inline)...)
}
```

### 处理链接点击和悬停

给 span 设置 `Link` 后，组件会按链接语义渲染并支持键盘聚焦；`ClickedLink` 和 `HoveredLink` 都返回当前帧的 URL。

```go
import "github.com/tangoui/tango/ui"

var linkStatus string

func page(c *ui.Context) {
    text := ui.RichText(c,
        ui.Span{Text: "打开文档", Link: "https://example.com/docs"},
        ui.Span{Text: " 或返回首页。"},
    )
    if u := text.ClickedLink(); u != "" {
        linkStatus = "已点击：" + u
    } else if u := text.HoveredLink(); u != "" {
        linkStatus = "悬停：" + u
    }
    ui.Text(c, linkStatus).Secondary()
}
```

### 选择、复制和段落布局

`Selectable` 开启指针选择以及 Cmd/Ctrl+A、Cmd/Ctrl+C；`Selection` 返回当前选中文本。`Pre` 保留空格和制表符，`Align`、`Size` 和 `Layout` 控制段落呈现。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var selected string

func page(c *ui.Context) {
    paragraph := ui.RichText(c,
        ui.Span{Text: "    保留前导空格的代码样式\n    第二行"},
    ).Pre().Selectable().Size(13).Align(ui.AlignCenter).Layout(layout.W(520))
    if s := paragraph.Selection(); s != "" {
        selected = s
    }
    ui.Text(c, selected).Secondary()
}
```

## 主题与样式

未显式设置 `Span.Color` 时使用当前主题正文色；链接使用主题链接色，悬停时根据主题变化。`Code` 会使用等宽字体和代码背景，shadcn 风格采用 muted 背景及圆角。`Mark` 使用 Typography 的高亮令牌。`Layout` 接收 `ui/layout` 选项，颜色和字号应优先通过 span 或当前主题设置。

## 注意事项

- **状态保存**：`RichText` 的选择状态由组件按 widget ID 保存；在即时模式中保持同一个组件调用位置，若动态列表会移动顺序请使用稳定的 `Key`。
- **链接行为**：组件只报告 URL，不会自动打开浏览器；应用应在 `ClickedLink` 返回后交给自己的路由或安全的外部链接处理器。
- **选择范围**：`Selection` 返回显示文本，不包含样式标记；没有选择时返回空字符串。
- **无障碍**：包含链接的段落可用 Tab 聚焦、Enter 激活；`Selectable` 适合可复制内容，但不应替代表单输入控件。
- **性能**：长文档或块级 Markdown 应使用 `Markdown`；`RichText` 更适合单段或少量段落的行内格式。

## 示例代码

<!--@include: ./_gen/richtext-examples.md-->

## API

<!--@include: ./_gen/richtext-api.md-->

## 相关链接

- [源码 ui/more_richtext.go](https://github.com/tangoui/tango/blob/main/ui/more_richtext.go)
- [Go API 参考](/reference/api/ui)
- [Markdown 文档](/components/markdown)、[Text 文本](/components/text)
- [组件总览](/components/)
