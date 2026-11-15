---
title: Markdown Markdown 文档
---
# Markdown Markdown 文档

将 CommonMark 核心语法和常用 GFM 扩展渲染为可选择、可交互的自绘文档内容。

## 何时使用

- 展示帮助文档、发布说明、聊天消息或服务端返回的 Markdown 内容。
- 需要标题、列表、任务列表、引用、代码块、表格和链接等文档结构时使用 `Markdown`。
- 只有一段需要精细控制的行内样式时用 [RichText 富文本](/components/richtext)；需要用户编辑 Markdown 时用 Markdown 编辑器组件。

## 在线体验

<GalleryPreview component="Markdown" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import "github.com/tangoui/tango/ui"

const document = "# 项目说明\n\nTango 支持 **粗体**、*强调*、`行内代码` 和列表。\n\n- 快速\n- 可读\n"

func page(c *ui.Context) {
    ui.Markdown(c, document)
}
```

`Markdown` 会按内容自动增长；长文档应放进具有滚动能力的外层布局中。原始 HTML 会按文本显示，不会作为 UI 节点执行。

## 常见用法

### 处理链接点击

`ClickedLink` 在当前帧返回被点击的 URL；页面可以把它交给自己的路由或外部链接处理函数。

```go
import "github.com/tangoui/tango/ui"

var openedURL string

func page(c *ui.Context) {
    md := ui.Markdown(c, "访问 [Tango](https://github.com/tangoui/tango) 查看源码。")
    if u := md.ClickedLink(); u != "" {
        openedURL = u
    }
    ui.Text(c, openedURL).Secondary()
}
```

### 交互式任务列表

任务列表的勾选会通过 `TaskToggled` 返回文档顺序中的索引；用 `ToggleMarkdownTask` 生成新的 Markdown 源文本，再在下一帧渲染。

```go
import "github.com/tangoui/tango/ui"

var tasks = "## 发布清单\n\n- [ ] 更新版本号\n- [x] 运行测试\n"

func page(c *ui.Context) {
    md := ui.Markdown(c, tasks)
    if i, ok := md.TaskToggled(); ok {
        tasks = ui.ToggleMarkdownTask(tasks, i)
    }
}
```

### 控制选择和代码块复制

默认正文可选择，代码块显示复制按钮；`NoSelect` 关闭文本选择，`NoCopy` 隐藏代码块复制按钮，适合只读的展示卡片。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func page(c *ui.Context) {
    ui.Markdown(c, "~~~go\nfmt.Println(\"只读示例\")\n~~~", layout.W(460)).
        NoSelect().
        NoCopy().
        Size(14)
}
```

### 渲染图片与调整正文颜色

`Images` 接收图片源并返回解码后的 `image.Image`；返回 `nil` 时显示占位内容。`Color` 适合把 Markdown 放进自定义颜色的聊天气泡。

```go
import (
    "image"

    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/theme"
)

func page(c *ui.Context) {
    ui.Markdown(c, "![架构图](arch.png)\n\n这是气泡中的说明文字。").
        Images(func(src string) image.Image {
            return nil // 图片缓存未命中时显示占位内容
        }).
        Color(theme.RGBA(240, 248, 255, 1))
}
```

### 为代码块提供固定尺寸

可以组合 `Size` 与 `layout` 选项控制文档宽度，代码块和表格会跟随文档区域布局。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func page(c *ui.Context) {
    ui.Markdown(c, "## 变更记录\n\n| 版本 | 状态 |\n| --- | --- |\n| 1.0 | 已发布 |", layout.W(520), layout.H(240)).Size(15)
}
```

## 主题与样式

Markdown 的标题、代码块、引用、表格和链接使用当前主题的 Typography、CodeBlock、Popover 等令牌。Ant Design 主题使用标题层级和代码复制按钮的默认样式；Element Plus 会使用其边框和填充风格；shadcn 会使用排版中的 `h2` 下划线、引用边框和 muted 代码背景。调用 `Color` 会覆盖正文和标题颜色，但链接仍使用主题链接色。

## 注意事项

- **状态保存**：`Markdown` 本身保存解析缓存，但源文本应放在页面级状态中；任务列表变更后必须把 `ToggleMarkdownTask` 的返回值写回源文本。
- **安全性**：Markdown 原始 HTML 作为文本显示，不要把不可信字符串拼接成应用命令或回调；链接点击后由应用决定是否允许导航。
- **图片加载**：`Images` 是同步回调；图片尚未准备好时返回 `nil`，加载完成后更新页面状态并请求重绘。
- **长文档**：组件会随着内容增长，长内容请放入滚动容器；代码块复制依赖当前窗口的剪贴板能力。
- **无障碍**：正文和代码默认可选择并支持 Cmd/Ctrl+C；任务列表使用复选框语义，链接支持 Tab 聚焦和 Enter 激活。

## 示例代码

<!--@include: ./_gen/markdown-examples.md-->

## API

<!--@include: ./_gen/markdown-api.md-->

## 相关链接

- [源码 ui/more_markdown.go](https://github.com/tangoui/tango/blob/main/ui/more_markdown.go)
- [Go API 参考](/reference/api/ui)
- [RichText 富文本](/components/richtext)、[CodeBlock 代码块](/components/code-editor)
- [组件总览](/components/)
