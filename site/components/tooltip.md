---
title: Tooltip 文字提示
---
# Tooltip 文字提示

简单的文字气泡：鼠标悬停或键盘聚焦到元素上时，在旁边显示一段说明，离开后消失。

## 何时使用

- 给图标按钮、缩写、被截断的文字补充说明，例如“刷新”“复制链接”“最后修改于 2026-11-13 10:24”。
- 只承载简短的纯文字（或少量富文本），不放可交互的内容。
- 需要标题、按钮、链接等可点击内容时，用 [Popover 气泡卡片](/components/popover)；需要确认操作时用 [Popconfirm](/components/catalog/popconfirm)；表单字段的说明可以用表单项的 `Tooltip` 方法。

## 在线体验

<GalleryPreview component="Tooltip" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
	"github.com/tangoui/tango/ui"
)

func page(c *ui.Context) {
	ui.Tooltip(c, "这是一段文字提示", func() {
		ui.Button(c, "悬停显示提示")
	})
}
```

`ui.Tooltip(c, tip, fn)` 把 `fn` 里构建的内容作为触发区域，悬停（或键盘焦点进入其中）时在上方显示 `tip`。不需要保存任何状态，打开与关闭由组件自己管理。

## 常见用法

### 位置

`Placement` 支持 12 个方向：`top`（默认）、`topLeft`、`topRight`、`bottom`、`bottomLeft`、`bottomRight`、`left`、`leftTop`、`leftBottom`、`right`、`rightTop`、`rightBottom`。空间不够时自动翻转到对侧。`left` / `right` 是逻辑方向，RTL 下镜像。

```go
import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

func page(c *ui.Context) {
	ui.Row(c, func() {
		for _, p := range []string{"top", "bottom", "left", "right"} {
			ui.Tooltip(c, "出现在 "+p, func() { ui.Button(c, p) }).Placement(p)
		}
	}, layout.Gap(16))
}
```

### 触发方式与延迟

`Trigger` 取 `"hover"`（默认，键盘聚焦同样显示）、`"click"`（点击切换，点外部关闭）或 `"focus"`（只在聚焦时显示）。`MouseEnterDelay` / `MouseLeaveDelay` 设置悬停出现和离开消失的延迟，避免鼠标掠过时频繁闪现。

```go
import (
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

func page(c *ui.Context) {
	ui.Row(c, func() {
		ui.Tooltip(c, "点击后出现，点外部关闭", func() { ui.Button(c, "点击触发") }).Trigger("click")
		ui.Tooltip(c, "聚焦时出现", func() { ui.Button(c, "聚焦触发") }).Trigger("focus")
		ui.Tooltip(c, "延迟 300ms 出现", func() { ui.Button(c, "延迟显示") }).
			MouseEnterDelay(300 * time.Millisecond).MouseLeaveDelay(100 * time.Millisecond)
	}, layout.Gap(16))
}
```

### 颜色、亮色与箭头

`Color` 设置气泡颜色：状态名（`"success"`、`"error"`……）、antd 预设色名（`"blue"`、`"magenta"`……）或 `"#rrggbb"`，文字颜色自动取对比色。`Effect("light")` 使用带边框的浅色气泡（Element Plus 的 `effect="light"`）。`Arrow(false)` 去掉箭头。

```go
import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

func page(c *ui.Context) {
	ui.Row(c, func() {
		ui.Tooltip(c, "预设色", func() { ui.Button(c, "blue") }).Color("blue")
		ui.Tooltip(c, "自定义颜色", func() { ui.Button(c, "#722ed1") }).Color("#722ed1")
		ui.Tooltip(c, "亮色提示", func() { ui.Button(c, "light") }).Effect("light")
		ui.Tooltip(c, "没有箭头", func() { ui.Button(c, "无箭头") }).Arrow(false)
	}, layout.Gap(16))
}
```

### 富内容

`Content(fn)` 用自定义内容替换文字，例如两行信息、加粗标题。`tip` 参数仍然保留，作为读屏软件读出的描述。深色气泡上的文字用 `c.Theme.TextOnSpotlight` 取色。

```go
import (
	"github.com/tangoui/tango/ui"
)

func page(c *ui.Context) {
	ui.Tooltip(c, "订单详情", func() { ui.Link(c, "悬停查看订单") }).Content(func() {
		ui.Text(c, "订单 #20261113").Strong().Color(c.Theme.TextOnSpotlight)
		ui.Text(c, "金额 ¥ 1,280.00").Color(c.Theme.TextOnSpotlight)
	})
}
```

### 图标按钮的说明

只有图标的按钮最常配 Tooltip。图标按钮自己的无障碍名称来自 `ui.IconButton` 的 label 参数，Tooltip 的文字会作为补充描述。

```go
import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
)

var copied int

func page(c *ui.Context) {
	ui.Tooltip(c, "复制链接到剪贴板", func() {
		if ui.IconButton(c, icon.Copy, "复制链接").Clicked() {
			copied++
		}
	})
}
```

## 主题与样式

| | Ant Design | Element Plus | shadcn |
| --- | --- | --- | --- |
| 气泡 | 深色，最小高度 32，内边距 6 × 8 | 深色，内边距 5 × 11；`Effect("light")` 为浅色带边框 | 深色，12px 小字号，内边距 6 × 12 |

组件令牌在 `theme.Components.Tooltip`（`theme.TooltipToken`）：`Bg`、`Color`、`ContentFontSize`、`MaxWidth`（默认 250，超出换行）、`PaddingBlock`、`PaddingInline`、`MinHeight`、`Radius`、`Shadow`。

`ConfigProvider.Defaults("Tooltip", ui.Props{...})` 支持：`placement`、`trigger`、`color`、`effect`（字符串）、`arrow`（布尔）、`mouseEnterDelay`、`mouseLeaveDelay`（`time.Duration`）。

```go
import (
	"time"

	"github.com/tangoui/tango/ui"
)

func page(c *ui.Context) {
	ui.ConfigProvider(c).Defaults("Tooltip", ui.Props{"placement": "bottom", "mouseEnterDelay": 200 * time.Millisecond}).Body(func() {
		ui.Tooltip(c, "默认在下方、延迟 200ms", func() { ui.Button(c, "全局默认") })
	})
}
```

## 注意事项

- **状态**：显示状态由组件按触发区域的 ID 保存，不需要外部变量。在循环里生成多个带提示的元素时，用 `ui.Key(c, key, fn)` 包住每一项，增删项时提示状态才不会串。
- **选项何时生效**：链式选项在本帧构建结束时（或调用 `IsOpen()` 时）才应用，所以可以在 `ui.Tooltip(...)` 之后继续链式设置。
- **键盘与无障碍**：hover 触发的提示在键盘焦点进入触发区域时也会显示，Esc 可以隐藏（直到焦点离开）。提示文字作为触发控件的描述暴露给读屏软件，不要只靠 Tooltip 承载必要信息。
- **触摸设备**：没有悬停，必要说明请改用 `Trigger("click")` 或直接写在界面上。

## 示例代码

<!--@include: ./_gen/tooltip-examples.md-->

## API

<!--@include: ./_gen/tooltip-api.md-->

## 相关链接

- [源码 ui/display.go](https://github.com/tangoui/tango/blob/main/ui/display.go)
- [Go API 参考](/reference/api/ui)
- [Popover 气泡卡片](/components/popover)、[Popconfirm](/components/catalog/popconfirm)
- [组件总览](/components/)
