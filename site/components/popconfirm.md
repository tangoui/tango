---
title: Popconfirm 气泡确认框
---
# Popconfirm 气泡确认框

在触发控件旁弹出轻量确认气泡，适合就地确认一个明确操作，不需要遮罩式对话框。

## 何时使用

- 删除、移除、发布等按钮旁需要二次确认的操作。
- 用户仍需看到当前页面上下文，确认内容不复杂时使用 `Popconfirm`。
- 需要长说明、多个字段或强制聚焦时用 [Confirm 确认框](/components/confirm)；不需要确认的结果反馈用 [Message 全局提示](/components/message)。

## 在线体验

<GalleryPreview component="Popconfirm" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import "github.com/tangoui/tango/ui"

var deleted bool

func page(c *ui.Context) {
    confirm := ui.Popconfirm(c, "删除任务", "确定要删除这个任务吗？", func() {
        ui.Button(c, "删除").Danger()
    })
    if confirm.Confirmed() {
        deleted = true
    }
    if deleted {
        ui.Text(c, "任务已删除").Secondary()
    }
}
```

`Popconfirm` 包裹 `trigger` 内容；点击触发内容后打开气泡，点击确定或取消后通过 `Confirmed`、`Cancelled` 报告当前帧的结果。

## 常见用法

### 自定义按钮文字和危险样式

`OkText`、`CancelText` 修改按钮文字，`OkDanger` 把确定按钮标记为危险操作，`Description` 添加标题下方的补充说明。

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    ui.Popconfirm(c, "清空回收站", "删除后无法恢复。", func() {
        ui.Button(c, "清空").Danger()
    }).
        Description("回收站中的所有文件都会被永久删除。每个文件都无法恢复。\n").
        OkText("清空回收站").
        CancelText("再想想").
        OkDanger()
}
```

### 自定义图标和位置

`Icon` 替换默认警告图标，`IconColor` 设置自定义图标颜色，`Placement` 使用 Popover 的位置名称，例如 `bottomLeft`、`right`。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

func page(c *ui.Context) {
    ui.Popconfirm(c, "发布版本", "发布后所有用户都能看到。", func() {
        ui.Button(c, "发布").Primary()
    }).
        Icon(icon.Question).
        IconColor(c.Theme.Primary.Base).
        Placement("bottomLeft").
        OkText("立即发布")
}
```

### 隐藏取消按钮

对没有歧义的轻量操作可以用 `ShowCancel(false)` 隐藏取消按钮；如果操作具有破坏性，通常应保留取消按钮并使用危险样式。

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    ui.Popconfirm(c, "标记为已读", "确定将全部通知标为已读吗？", func() {
        ui.Button(c, "全部已读")
    }).ShowCancel(false).Placement("right")
}
```

### 读取取消和打开状态

`IsOpen` 可用于展示当前气泡状态，`Cancelled` 在当前帧报告取消按钮、关闭或外部关闭等取消结果；禁用后不会打开气泡。

```go
import "github.com/tangoui/tango/ui"

var popStatus string

func page(c *ui.Context) {
    pop := ui.Popconfirm(c, "移除成员", "确定移除这个成员吗？", func() {
        ui.Button(c, "移除").Danger()
    }).Disabled(false)
    if pop.Confirmed() {
        popStatus = "已确认"
    }
    if pop.Cancelled() {
        popStatus = "已取消"
    }
    if pop.IsOpen() {
        popStatus = "等待选择"
    }
    ui.Text(c, popStatus).Secondary()
}
```

## 主题与样式

Popconfirm 使用 `theme.Components.Popover` 的背景、圆角、阴影、内边距和箭头令牌。Ant Design 风格默认在触发器上方显示并带箭头；Element Plus 和 shadcn 使用各自的气泡边框与圆角。可通过 `ConfigProvider.Defaults("Popconfirm", ...)` 设置 `okText`、`cancelText`、`placement` 和 `showCancel` 默认值。

## 注意事项

- **触发内容**：`trigger` 函数应只构建一个明确的可交互控件，避免在一个气泡里混放多个独立触发器。
- **即时模式**：每帧按相同顺序创建 Popconfirm；组件按触发器节点 ID 保存打开状态，动态列表请保持稳定的 `Key`。
- **结果读取**：`Confirmed` 和 `Cancelled` 是当前帧事件，不是长期状态；要保存业务结果，请在返回 `true` 时写入页面状态。
- **焦点与关闭**：气泡打开后支持键盘操作，点击外部区域会关闭；禁用状态不会打开。
- **位置**：空间不足时会根据 `Placement` 和可用空间翻转；若触发器靠近窗口边缘，优先选择允许翻转的位置。

## 示例代码

<!--@include: ./_gen/popconfirm-examples.md-->

## API

<!--@include: ./_gen/popconfirm-api.md-->

## 相关链接

- [源码 ui/feedback.go](https://github.com/tangoui/tango/blob/main/ui/feedback.go)
- [Go API 参考](/reference/api/ui)
- [Confirm 确认框](/components/confirm)、[Popover 气泡卡片](/components/popover)
- [组件总览](/components/)
