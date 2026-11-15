---
title: editor.View 代码编辑器视图
---
# editor.View 代码编辑器视图

`editor.View` 把 `ui/editor.State` 渲染成可编辑的代码视图，提供语法高亮、行号、选择、撤销重做、只读和大文档虚拟滚动。

## 何时使用

- 需要在自绘页面中编辑 Go、TypeScript、Python 等带语法高亮的代码时。
- 多个文件需要各自保留模型、选区、滚动位置和撤销历史时。
- 需要只读查看、行号、Tab 缩进或链接点击等编辑器交互时。
- 只编辑 Markdown 文本并需要工具栏和实时预览时，用 [MarkdownEditor](/components/markdowneditor)；只展示短代码片段时，用 [CodeBlock](/components/catalog/codeblock) 更轻量。

## 在线体验

<GalleryPreview component="editor.View" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见[打开自绘窗口](/guide/ui#打开一个自绘窗口)。`State` 和 `Model` 应在页面状态中复用，而不是每帧新建。

```go
package main

import (
    "time"

    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/editor"
    editortheme "github.com/tangoui/tango/ui/editor/theme"
    "github.com/tangoui/tango/ui/layout"
)

var codeState *editor.State

func page(c *ui.Context) {
    if codeState == nil {
        model := editor.NewModel("main.go", "package main\n\nfunc main() {}\n", editor.DefaultRegistry())
        codeState = editor.NewView(model, editortheme.Base("vs"))
    }
    if h := codeState.Highlighter(); h != nil {
        snapshot := codeState.Snapshot()
        if !h.Highlight(snapshot, snapshot.LineCount(), 3*time.Millisecond) {
            c.WakeAt(c.Now().Add(16 * time.Millisecond))
        }
    }
    editor.View(c, codeState).LineNumbers(true).Layout(layout.H(320))
}

func main() {}
```

`Model` 保存文档和撤销历史，`State` 保存选区、视口和编辑器设置，`View` 每帧负责绘制和接收输入。有限的高亮预算让大文档在前台分帧完成高亮；视图销毁时调用 `codeState.Close()` 释放模型订阅。

## 常见用法

### 只读、行号、字体和 Tab 大小

视图修饰器只影响当前帧；应用级的持久设置应写入 `State.Options` 或 `Model` 的设置。`ReadOnly(true)` 仍允许选择、滚动和复制，但阻止修改。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/editor"
    editortheme "github.com/tangoui/tango/ui/editor/theme"
    "github.com/tangoui/tango/ui/layout"
)

var readOnlyState = editor.NewView(
    editor.NewModel("config.json", "{\n  \"enabled\": true\n}\n", editor.DefaultRegistry()),
    editortheme.Base("vs-dark"),
)

func page(c *ui.Context) {
    readOnly := true
    editor.View(c, readOnlyState).
        ReadOnly(readOnly).
        LineNumbers(true).
        FontSize(14).
        TabSize(2).
        Layout(layout.H(260))
}

func main() {}
```

### 切换多个文件并保留编辑状态

每个文件使用自己的 `Model` 和 `State`，切换时从 map 取回原状态即可。不要只替换同一个 Model 的文本来模拟文件，否则会混用撤销历史和选区。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/editor"
    editortheme "github.com/tangoui/tango/ui/editor/theme"
    "github.com/tangoui/tango/ui/layout"
)

var (
    activeFile = "main.go"
    fileStates = map[string]*editor.State{}
    fileText = map[string]string{
        "main.go": "package main\n\nfunc main() {}\n",
        "app.ts":  "export const ready: boolean = true;\n",
    }
)

func stateFor(file string) *editor.State {
    if st := fileStates[file]; st != nil {
        return st
    }
    st := editor.NewView(editor.NewModel(file, fileText[file], editor.DefaultRegistry()), editortheme.Base("vs"))
    fileStates[file] = st
    return st
}

func page(c *ui.Context) {
    editor.View(c, stateFor(activeFile)).LineNumbers(true).Layout(layout.H(300))
}

func main() {}
```

文件关闭时遍历对应的 `State` 调用 `Close()`；如果只是暂时切换文件，保留状态对象即可。

### 接收编辑变化并自定义布局

`Changed()` 表示当前帧发生了文本编辑、撤销或重做。`Layout` 接收普通布局选项，可以把编辑器放进固定高度、可增长的列或卡片内容区。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/editor"
    editortheme "github.com/tangoui/tango/ui/editor/theme"
    "github.com/tangoui/tango/ui/layout"
)

var changedState = editor.NewView(
    editor.NewModel("note.txt", "先编辑这段文本。\n", editor.DefaultRegistry()),
    editortheme.Base("vs"),
)

func page(c *ui.Context) {
    view := editor.View(c, changedState).Layout(layout.H(220), layout.Grow(1))
    if view.Changed() {
        markDocumentDirty()
    }
    if view.Focused() {
        ui.Text(c, "编辑器已聚焦").Tertiary()
    }
}

func markDocumentDirty() {}

func main() {}
```

`Changed()` 应在渲染同一帧中读取；实际保存内容时从 `changedState.Snapshot()` 取得快照，而不是依赖渲染层的临时文本。

### 处理编辑器中的链接

编辑器会识别文档中的链接。`OnOpenLink` 接收用户点击的 URL，由应用决定是否在外部浏览器、路由或内嵌页面中打开。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/editor"
    editortheme "github.com/tangoui/tango/ui/editor/theme"
)

var linkState = editor.NewView(
    editor.NewModel("README.md", "查看 https://tangoui.example/docs\n", editor.DefaultRegistry()),
    editortheme.Base("vs"),
)

func page(c *ui.Context) {
    editor.View(c, linkState).OnOpenLink(func(url string) {
        openURL(url)
    })
}

func openURL(url string) {}

func main() {}
```

不要在回调中无条件执行外部命令；生产应用应按允许的协议和域名校验 URL。

## 主题与样式

`editor.NewView` 接收 `*theme.Theme`，常用 `editortheme.Base("vs")` 或 `editortheme.Base("vs-dark")`；编辑器会据此绘制语法颜色和背景。`LineNumbers`、`FontSize`、`TabSize`、`ReadOnly` 是视图级修饰器，`Keymap` 和 `Context` 用于高级快捷键集成。编辑器不是普通表单控件，不读取 `theme.Components` 的输入框令牌。

## 注意事项

- `Model`、`State` 和高亮器要在稳定状态中复用；每帧调用 `NewView` 会丢失光标、撤销历史和滚动位置。
- 大文档只在可见行附近布局和着色；应用层不要在每帧复制完整文本。需要保存时使用 `Snapshot()`。
- UI 线程驱动 `State`，异步加载或保存完成后通过页面的失效机制唤醒 UI，不要从后台 goroutine 直接修改选区。
- `ReadOnly` 不是安全边界；保存或执行代码前仍应在应用层做权限检查。自定义语义包装可使用 `view.Box()` 设置 `A11yLabel`。
- 视图关闭时调用 `State.Close()`；多个 State 共享 Model 时，只有所有视图都不再使用后才释放 Model。

## 示例代码

<!--@include: ./_gen/editor-view-examples.md-->

## API

<!--@include: ./_gen/editor-view-api.md-->

## 相关链接

- [代码编辑器 gallery 示例](https://github.com/tangoui/tango/blob/main/examples/ui-gallery/page_code_editor.go)
- [MarkdownEditor Markdown 编辑器](/components/markdowneditor)
- [ui/editor Go API](/reference/api/ui-editor)
- [源码](https://github.com/tangoui/tango/blob/main/ui/editor/view.go)
