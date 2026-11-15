---
title: Kbd 键帽
---
# Kbd 键帽

以紧凑的键帽样式显示一个按键或快捷键提示，帮助用户发现键盘操作。

## 何时使用

- 按钮、命令面板或帮助文字旁需要展示快捷键时使用。
- 要表达单个按键、组合键或平台快捷键提示时使用。
- 需要真正监听和执行快捷键时，使用事件或 [快捷键输入](/guide/system/hotkey)，Kbd 本身不注册键盘事件。
- 需要可点击的操作控件时，使用 [Button 按钮](/components/button)，不要把 Kbd 当作按钮。

## 在线体验

<GalleryPreview component="Toggle" />

## 基本用法

`Kbd` 接收要显示的字符串并返回一个可放入行布局的键帽；它只负责呈现，不改变焦点或状态。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func kbdPage(c *ui.Context) {
    ui.Row(c, func() {
        ui.Text(c, "保存")
        ui.Kbd(c, "⌘ S")
    }, layout.Gap(8), layout.Middle())
}
```

## 常见用法

### 组合快捷键

可以把整个组合键作为一个 Kbd，也可以拆成多个键帽；拆开时用 `Gap` 保持按键之间的节奏。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func shortcutKbdPage(c *ui.Context) {
    ui.Row(c, func() {
        ui.Kbd(c, "⌘")
        ui.Kbd(c, "Shift")
        ui.Kbd(c, "P")
    }, layout.Gap(4), layout.Middle())
}
```

### 命令面板中的提示

`CommandItem.Shortcut` 会在命令条目右侧显示快捷键；需要在面板外说明触发方式时，可以用 Kbd 与文字并排展示。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
    "github.com/tangoui/tango/ui/layout"
)

func commandShortcutPage(c *ui.Context) {
    ui.Row(c, func() {
        ui.Command(c, []ui.CommandItem{{Value: "search", Label: "搜索", Icon: icon.Search, Shortcut: "⌘K"}}).Layout(layout.W(300))
        ui.Text(c, "打开命令面板")
        ui.Kbd(c, "⌘ K")
    }, layout.Gap(12), layout.Middle())
}
```

### 平台差异文案

快捷键字符串由应用决定；可以在页面状态中根据平台选择 `⌘` 或 `Ctrl`，Kbd 不会自行转换文案。

```go
import "github.com/tangoui/tango/ui"

var isMacOS = true

func platformKbdPage(c *ui.Context) {
    modifier := "Ctrl"
    if isMacOS { modifier = "⌘" }
    ui.Text(c, "打开搜索")
    ui.Kbd(c, modifier+" K")
}
```

## 主题与样式

Kbd 使用小号文字、次级填充和小圆角。Ant 与 Element Plus 会额外绘制边框，shadcn 使用更轻的填充键帽；颜色、圆角和字体都跟随当前主题，不提供单独的尺寸或变体方法。

## 注意事项

- **语义**：Kbd 使用文本语义，内容会被屏幕阅读器读作文字；请写可理解的按键名称，不要只画图标。
- **状态**：Kbd 没有内部状态；快捷键是否可用、显示哪套平台文案由页面状态决定。
- **可用性**：显示快捷键不等于注册快捷键；必须在应用事件层实现相同的操作并处理焦点范围。
- **布局**：Kbd 是紧凑的不可拉伸节点，多个键帽用 `Row` 和 `layout.Gap` 组合。
- **RTL**：文字和组合键按当前文本方向绘制；快捷键本身建议用平台惯用的字符串。

## 示例代码

<!--@include: ./_gen/kbd-examples.md-->

## API

<!--@include: ./_gen/kbd-api.md-->

## 相关链接

- [源码 shad_kbd.go](https://github.com/tangoui/tango/blob/main/ui/shad_kbd.go)
- [ui Go API](/reference/api/ui)
- [Command 命令面板](/components/command)、[Button 按钮](/components/button)
