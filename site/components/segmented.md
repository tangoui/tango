---
title: Segmented 分段控制器
---
# Segmented 分段控制器

在少量互斥选项之间快速切换，常用于视图、时间范围或模式选择。

## 何时使用

- 选项数量少、互斥且需要直接可见时。
- 切换当前页面视图或筛选范围，而不是提交表单字段时。
- 选项较多或需要搜索时使用 [Select 选择器](/components/select)；允许同时选多个值时使用 Checkbox 或 [MultiSelect](/components/select)。

## 在线体验

<GalleryPreview component="Segmented" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

var rangeName = "本周"

func page(c *ui.Context) {
    ui.Segmented(c, &rangeName, ui.Opts("今日", "本周", "本月"))
}
```

选项的 `Value` 写入绑定字符串，`Label` 用于显示。应在初始化时让绑定值与某个选项的 `Value` 对应。

## 常见用法

### 响应切换

`Changed` 只在当前帧发生切换时返回 `true`，适合刷新列表或重算查询条件。

```go
import "github.com/tangoui/tango/ui"

var mode = "list"

func page(c *ui.Context) {
    if ui.Segmented(c, &mode, []ui.Option{
        {Label: "列表", Value: "list"},
        {Label: "看板", Value: "board"},
    }).Changed() {
        // 根据 mode 重新加载当前视图
    }
}
```

### 块级与竖向布局

`Block` 让各段平均分配容器宽度；`Vertical` 改为上下排列，适合侧栏或窄容器。

```go
import "github.com/tangoui/tango/ui"

var view = "a"

func page(c *ui.Context) {
    ui.Segmented(c, &view, ui.Opts("概览", "详情", "日志")).Block()
    ui.Segmented(c, &view, ui.Opts("概览", "详情", "日志")).Vertical()
}
```

### 图标、尺寸与胶囊形

`Icons` 按选项顺序提供图标；没有文字的选项会使用 `Value` 作为可访问名称。`Shape` 可切换胶囊形外观。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

var layoutMode = "list"

func page(c *ui.Context) {
    ui.Segmented(c, &layoutMode, []ui.Option{{Value: "list"}, {Value: "board"}}).
        Icons(icon.Table, icon.Appstore).
        Shape(ui.SegmentedShapeRound).
        Size(ui.Small)
}
```

### 禁用单项与整组

在 `Option` 上设置 `Disabled` 禁用单个段；`Disabled(true)` 禁用整个控制器。

```go
import "github.com/tangoui/tango/ui"

var status = "ready"

func page(c *ui.Context) {
    ui.Segmented(c, &status, []ui.Option{
        {Label: "就绪", Value: "ready"},
        {Label: "维护中", Value: "maintenance", Disabled: true},
    }).A11yLabel("服务状态")
}
```

## 主题与样式

- 令牌位于 `theme.Components.Segmented`，包括轨道背景、选中滑块、文字颜色、内边距和选中阴影。
- Ant 风格默认圆角随尺寸变化，Element Plus 使用更平的轨道，shadcn 使用其 muted 背景和选中态；页面不需要写主题颜色。
- `Size` 支持 `ui.Small`、默认值和 `ui.Large`；`Shape(ui.SegmentedShapeRound)` 将轨道和滑块变为胶囊形。
- `ConfigProvider.Defaults("Segmented", ui.Props{...})` 支持 `size`、`block`。

## 注意事项

- 绑定字符串要放在稳定状态中，且每个选项的 `Value` 应唯一；每帧重建选项时保持顺序和值稳定。
- Segmented 适合 2–5 个短选项；文本过长或选项很多时会挤压布局，应改用 Select 或 Tabs。
- Tab 聚焦后，左右方向键（竖向时为上下方向键）切换选项；组件提供 radio group 语义，使用 `A11yLabel` 命名整组。
- RTL 布局会自动按逻辑内联方向移动选中滑块。

## 示例代码

<!--@include: ./_gen/segmented-examples.md-->

## API

<!--@include: ./_gen/segmented-api.md-->

## 相关链接

- [源码 ui/entry_segmented.go](https://github.com/tangoui/tango/blob/main/ui/entry_segmented.go)
- [ui Go API](/reference/api/ui)
- [Select 选择器](/components/select)、[Radio 单选框](/components/radio)、[Tabs 标签页](/components/tabs)
