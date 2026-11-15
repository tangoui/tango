---
title: MarkdownEditor Markdown 编辑器
---
# MarkdownEditor Markdown 编辑器

提供 Markdown 源码编辑、格式工具栏、编辑 / 分屏 / 预览模式和实时渲染的即时模式编辑器。

## 何时使用

- 编辑公告、周报、说明文档等 Markdown 内容，并希望同时看到渲染结果时。
- 需要工具栏插入标题、强调、列表、代码块、表格或链接时。
- 需要预览中的任务列表可以直接勾选并回写源码时。
- 需要完整语言高亮、文件模型、撤销历史和大文档虚拟滚动时，用 [`editor.View`](/components/editor-view)；只展示 Markdown 时用 [Markdown](/components/markdown) 更轻量。

## 在线体验

<GalleryPreview component="MarkdownEditor" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见[打开自绘窗口](/guide/ui#打开一个自绘窗口)。源码变量必须由页面状态持有，以便跨帧保存编辑内容。

```go
package main

import "github.com/tangoui/tango/ui"

var document = "## 周报\n\n- [x] 完成编辑器\n- [ ] 发布文档\n"

func page(c *ui.Context) {
    editor := ui.MarkdownEditor(c, &document).Height(460)
    if editor.Changed() {
        saveDraft(document)
    }
}

func saveDraft(source string) {}

func main() {}
```

默认高度是 400 像素，默认模式是 `split`。工具栏会修改同一个 `*string`，预览中的任务列表勾选也会写回源码。

## 常见用法

### 编辑、分屏和预览模式

`Mode` 支持 `edit`、`split`、`preview` 三个初始值；用户可以通过工具栏切换模式。`CurrentMode()` 用于在页面状态栏显示当前模式。

```go
package main

import "github.com/tangoui/tango/ui"

var source = "# 发布说明\n\n这里是正文。\n"

func page(c *ui.Context) {
    md := ui.MarkdownEditor(c, &source).Mode("edit").Height(360)
    ui.Text(c, "当前模式："+md.CurrentMode()).Tertiary()
}

func main() {}
```

`edit` 适合纯源码录入，`split` 适合边写边预览，`preview` 适合只读检查渲染结果；即使在预览模式，工具栏格式按钮也不会修改源码。

### 自定义占位提示和外层布局

`Placeholder` 只作用于空源码编辑区，`Layout` 作用于外层编辑器框，可以把它放进可增长的列或固定高度的卡片。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var article string

func page(c *ui.Context) {
    ui.MarkdownEditor(c, &article).
        Placeholder("写下本周的更新……").
        Height(480).
        Layout(layout.Grow(1), layout.MinW(0))
}

func main() {}
```

高度包含工具栏；窄窗口下工具栏会换行，编辑器会根据工具栏实际高度重新计算正文区域。

### 读取链接点击和编辑状态

预览中的链接点击不会自动打开外部页面；应用可以通过 `ClickedLink()` 读取当前帧的 URL，再交给自己的路由或安全的外部打开器。

```go
package main

import "github.com/tangoui/tango/ui"

var helpText = "请阅读 [使用指南](https://tangoui.example/guide)。"

func page(c *ui.Context) {
    md := ui.MarkdownEditor(c, &helpText).Mode("preview")
    if url := md.ClickedLink(); url != "" {
        navigate(url)
    }
}

func navigate(url string) {}

func main() {}
```

`Changed()` 会报告输入、工具栏格式化和任务勾选造成的变化；`ClickedLink()` 只报告预览中当前帧点击的链接。

### 使用 Markdown 编辑辅助函数

需要在自定义快捷键或自己的工具栏中复用格式逻辑时，可以使用导出的 `MarkdownWrap`、`MarkdownLinePrefix` 和 `MarkdownInsertBlock`。它们返回新源码及新的选区范围。

```go
package main

import (
    "fmt"

    "github.com/tangoui/tango/ui"
)

func addBold(source string, start, end int) string {
    next, _, _ := ui.MarkdownWrap(source, start, end, "**", "**", "粗体")
    return next
}

func page(c *ui.Context) {
    source := addBold("标题", 0, 2)
    ui.Text(c, fmt.Sprintf("%s", source))
}

func main() {}
```

这些函数按 rune 索引处理选区；如果选区为空，会插入对应的占位文本。直接使用 MarkdownEditor 时，工具栏和快捷键已经调用这些逻辑。

### 保存草稿和提交正文

编辑器只负责修改字符串，不负责网络保存。可以在 `Changed()` 时保存草稿，也可以在提交按钮点击时读取当前字符串并异步持久化。

```go
package main

import "github.com/tangoui/tango/ui"

var (
    body    = "# 文章标题\n"
    saving  bool
    savedOK bool
)

func page(c *ui.Context) {
    md := ui.MarkdownEditor(c, &body).Height(420)
    if md.Changed() {
        savedOK = false
    }
    if ui.Button(c, "保存").Primary().Loading(saving).Clicked() {
        saving = true
        savedOK = persist(body)
        saving = false
    }
    if savedOK {
        ui.Text(c, "已保存").Tertiary()
    }
}

func persist(source string) bool { return source != "" }

func main() {}
```

真实应用中不要在每个按键上同步阻塞网络请求；应使用异步任务，并在完成后让页面失效重绘。

## 主题与样式

MarkdownEditor 使用 TextArea、Button、CheckableTag、Scroll 和 Markdown 渲染器的主题令牌，外框会随 Ant Design、Element Plus 或 shadcn 风格调整边框、背景和圆角。`ConfigProvider.Defaults("MarkdownEditor", ui.Props{"mode": "split", "placeholder": "写 Markdown"})` 可设置默认模式和占位提示；实例上的 `Mode`、`Placeholder` 优先。

## 注意事项

- `src` 指向的字符串必须稳定保存；在页面函数中每帧写 `src := ""` 会丢失输入、选区和模式状态。
- `Mode("preview")` 仍然允许用户通过预览任务列表修改源码；若需要完全禁止交互，应在页面层禁用外部操作并使用只读的 Markdown 展示。
- `ClickedLink()` 返回 URL 后应做协议、域名和权限校验，不要直接执行未经校验的外部命令。
- 工具栏已经提供键盘快捷键（粗体、斜体、行内代码、链接）和可访问的工具栏语义；自定义工具按钮要补充可访问名称。
- 大型文档若需要更强的语言服务、文件模型和虚拟滚动，应改用 `editor.View`；MarkdownEditor 的预览会解析当前源码。

## 示例代码

<!--@include: ./_gen/markdowneditor-examples.md-->

## API

<!--@include: ./_gen/markdowneditor-api.md-->

## 相关链接

- [MarkdownEditor gallery 示例](https://github.com/tangoui/tango/blob/main/examples/ui-gallery/page_more_mdeditor.go)
- [editor.View 代码编辑器](/components/editor-view)
- [Markdown 渲染](/components/markdown)
- [源码](https://github.com/tangoui/tango/blob/main/ui/more_mdeditor.go)
- [ui Go API](/reference/api/ui)
