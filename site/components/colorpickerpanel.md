---
title: ColorPickerPanel 取色面板
---
# ColorPickerPanel 取色面板

把 ColorPicker 的取色面板直接嵌入页面，不显示弹出触发器，颜色修改会立即写回。

## 何时使用

- 需要让取色器始终可见，例如主题编辑器、绘图属性或设计面板。
- 需要同时展示色相、饱和度/明度、透明度、预设色和文本输入时。
- 只需要点击色块后再编辑时，用 [ColorPicker 颜色选择器](/components/colorpicker)；只显示颜色结果时用普通文本或色块。
- 需要编辑渐变时使用 `ColorPickerPanelGradient`，而不是把渐变当成一个单色值。

## 在线体验

<GalleryPreview component="ColorPickerPanel" />

## 基本用法

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/theme"
)

var accent = theme.Hex("#1677ff")

func page(c *ui.Context) {
    ui.ColorPickerPanel(c, &accent)
}
```

面板绑定 `*theme.Color`；在所有设计系统中都即时写回，没有确认按钮。

## 常见用法

### 预设色与格式

`Presets` 显示色板，`PresetGroups` 可按标题分组；`Format` 控制文本输入格式，支持 `hex`、`rgb`、`hsb` 和 `hsv`。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/theme"
)

var brand = theme.Hex("#409eff")
var presets = []theme.Color{theme.Hex("#1677ff"), theme.Hex("#67c23a"), theme.Hex("#f56c6c")}

func page(c *ui.Context) {
    ui.ColorPickerPanel(c, &brand).
        Presets(presets...).PresetLabel("品牌色").Format("rgb").NoAlpha()
}
```

### 分组预设与清除

`ColorPresetGroup.Closed` 可让 Ant Design 或 shadcn 主题的分组初始折叠；`AllowClear` 增加无色状态。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/theme"
)

var surface = theme.Hex("#ffffff")

func page(c *ui.Context) {
    ui.ColorPickerPanel(c, &surface).
        PresetGroups(
            ui.ColorPresetGroup{Label: "品牌色", Colors: []theme.Color{theme.Hex("#1677ff"), theme.Hex("#722ed1")}},
            ui.ColorPresetGroup{Label: "状态色", Colors: []theme.Color{theme.Hex("#52c41a"), theme.Hex("#f5222d")}, Closed: true},
        ).
        AllowClear()
}
```

### 线性渐变

`ColorPickerPanelGradient` 绑定 `*theme.Gradient`，面板上方显示色标和角度，下面编辑当前色标的颜色。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/theme"
)

var background = theme.Gradient{Angle: 90, Stops: []theme.GradientStop{
    {Offset: 0, Color: theme.Hex("#1677ff")},
    {Offset: 1, Color: theme.Hex("#722ed1")},
}}

func page(c *ui.Context) {
    ui.ColorPickerPanelGradient(c, &background).NoAlpha()
}
```

### 边框、禁用与变化通知

`Border(false)` 适合已经放在卡片中的面板；`Disabled(true)` 保留内容但阻止编辑。`Changed` 在本帧颜色写回后返回 `true`。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/theme"
)

var chartColor = theme.Hex("#13c2c2")

func page(c *ui.Context) {
    panel := ui.ColorPickerPanel(c, &chartColor).Border(false)
    if panel.Changed() {
        ui.Message(c, "success", "图表颜色已更新")
    }
}
```

## 主题与样式

- 面板使用 `theme.Components.ColorPicker` 的 `ColorPickerToken`，包括 `PanelWidth`、`PaletteHeight`、`BarHeight`、`HandleSize` 和 `SwatchSize`。
- Ant Design、Element Plus、shadcn 会分别调整面板圆角、输入区、预设色分组和边框；`Border` 只控制外层边框。
- `ConfigProvider.Defaults("ColorPicker", ...)` 的 `size`、`allowClear`、`disabledAlpha`、`format` 默认值也会作用于面板。

## 注意事项

- `theme.Color` 和 `theme.Gradient` 必须放在稳定状态中；颜色零值表示无色，不等于黑色。
- 面板没有取消操作，若业务需要撤销，请在打开或编辑前保存旧值并自行恢复。
- 渐变应至少保留两个色标；面板会把角度、色标位置和颜色写回同一个 `theme.Gradient`。
- 面板本身没有触发器标签；放在表单项中使用表单标签，独立使用时给相关控件或容器提供可理解的无障碍名称。

## 示例代码

<!--@include: ./_gen/colorpickerpanel-examples.md-->

## API

<!--@include: ./_gen/colorpickerpanel-api.md-->

## 相关链接

- [源码 ui/fill_color_panel.go](https://github.com/tangoui/tango/blob/main/ui/fill_color_panel.go)
- [源码 ui/entry_color.go](https://github.com/tangoui/tango/blob/main/ui/entry_color.go)
- [ui Go API](/reference/api/ui)
- [ColorPicker 颜色选择器](/components/colorpicker)、[Form 表单](/components/form)
