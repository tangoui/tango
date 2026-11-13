---
title: 代码编辑器在线演示
outline: [2, 3]
---
# 代码编辑器在线演示

Tango 的代码编辑器位于 `github.com/tangoui/tango/ui/editor`。下面直接运行 Go 编写的 `editor.View`，可编辑示例代码，并使用同一套模型、语法分词、选区和撤销历史。

## 在线编辑

点击代码区域即可开始编辑。文件菜单提供 **Go、TypeScript、Python、JavaScript** 示例，切换文件会保留各自的编辑内容。场景菜单还提供默认超过 2,000 行的 TypeScript 文档，可以体验代码区域的虚拟滚动。

<GalleryPreview component="editor.View" />

## 可以体验什么

| 操作 | 用法 |
| --- | --- |
| 编辑与语法高亮 | 输入、删除或粘贴代码；分词在后续帧持续更新 |
| 切换文件 | 使用文件菜单，分别保留文档内容与撤销历史 |
| 缩进 | 编辑区内按 Tab；选区可使用 Shift+Tab 反向缩进 |
| 撤销与重做 | 工具栏按钮，或 Ctrl/Cmd+Z、Ctrl+Y / Cmd+Shift+Z |
| 只读 | 勾选「只读」，保留查看与选区操作并阻止编辑 |
| 行号 | 勾选或取消「行号」 |
| 深浅色 | 预览工具栏切换深色；编辑区使用对应的语法配色 |
| 大文档 | 在场景菜单选择「大文档与虚拟滚动」，滚轮在代码区域内操作 |
| 恢复内容 | 「恢复示例」恢复当前文件；「重置」重新初始化整个场景 |

Tab 在编辑区用于缩进。需要回到文档时，先按 Esc，再按 Tab 离开预览；再次点击代码区域即可继续编辑。

## Go 中的基本用法

`Model` 持有文档与撤销历史，`State` 持有选区、视口和设置，`View` 负责渲染与输入。页面每帧调用时复用同一个状态：

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/editor"
    editortheme "github.com/tangoui/tango/ui/editor/theme"
)

var state = editor.NewView(
    editor.NewModel("main.go", "package main\n", editor.DefaultRegistry()),
    editortheme.Base("vs-dark"),
)

func page(c *ui.Context) {
    if h := state.Highlighter(); h != nil {
        snapshot := state.Snapshot()
        if !h.Highlight(snapshot, snapshot.LineCount(), 3*time.Millisecond) {
            c.WakeAt(c.Now().Add(16 * time.Millisecond))
        }
    }
    editor.View(c, state).LineNumbers(true).ReadOnly(false)
}
```

这段示例使用分帧的前台高亮预算，适合文档预览。完整原生应用可以参考 `editor-demo` 的后台高亮与窗口唤醒接入。视图销毁时调用 `state.Close()`，释放它对模型的订阅。

## 运行原生示例

```bash
go run ./examples/editor-demo
go run ./examples/editor-demo -file main.go -theme vs
go run ./examples/editor-demo -gen-lines 10000
```

## 源码和 API

- [Gallery 演示源码](https://github.com/tangoui/tango/blob/main/examples/ui-gallery/page_code_editor.go)
- [原生编辑器示例](https://github.com/tangoui/tango/blob/main/examples/editor-demo/main.go)
- [ui/editor API](/reference/api/ui-editor)
- [全部 GUI 组件](/components/)
