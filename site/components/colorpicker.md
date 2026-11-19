---
title: ColorPicker 颜色选择器
---
# ColorPicker 颜色选择器

选择并编辑单色或线性渐变颜色，适合主题配置、标记颜色和绘图属性。

## 何时使用

- 用户需要从色板或预设色中选择颜色时。
- 需要编辑 HEX、RGB、HSB 或透明度时。
- 需要将取色面板直接嵌入页面时，用 [ColorPickerPanel 取色面板](/components/colorpickerpanel)。颜色值只是普通文本（如 CSS 变量名）时用 [Input 输入框](/components/input)。

## 在线体验

<GalleryPreview component="ColorPicker" />

## 基本用法

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/theme"
)

var accent = theme.Hex("#1677ff")

func page(c *ui.Context) {
    ui.ColorPicker(c, &accent).ShowText().A11yLabel("强调色")
}
```

颜色由 `*theme.Color` 绑定，面板中的修改会即时写回；零值颜色表示尚未设置颜色。

## 常见用法

### 指定编辑格式与透明度

`Format` 控制面板中的编辑格式，支持 `hex`、`rgb`、`hsb` 和 `hsv`。`NoAlpha` 隐藏透明度控制，`AllowClear` 允许恢复零值。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/theme"
)

var color = theme.RGBA(22, 119, 255, .6)

func page(c *ui.Context) {
    ui.ColorPicker(c, &color).Format("rgb").NoAlpha().AllowClear().ShowText()
}
```

### 预设颜色

`Presets` 展示一组颜色；需要分组、标题或折叠组时用 `PresetGroups`。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/theme"
)

var color = theme.Hex("#13c2c2")
var presets = []theme.Color{theme.Hex("#1677ff"), theme.Hex("#52c41a"), theme.Hex("#f5222d")}

func page(c *ui.Context) {
    ui.ColorPicker(c, &color).Presets(presets...).PresetLabel("推荐颜色")
    ui.ColorPicker(c, &color).PresetGroups(ui.ColorPresetGroup{
        Label: "品牌色", Colors: presets,
    })
}
```

### 线性渐变

`ColorPickerGradient` 绑定 `theme.Gradient`，支持增加、移动和删除色标，并编辑角度和当前色标颜色。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/theme"
)

var gradient = theme.Gradient{Angle: 90, Stops: []theme.GradientStop{
    {Offset: 0, Color: theme.Hex("#1677ff")},
    {Offset: 1, Color: theme.Hex("#722ed1")},
}}

func page(c *ui.Context) {
    ui.ColorPickerGradient(c, &gradient).ShowText().A11yLabel("背景渐变")
}
```

### 触发方式与状态

默认点击色块打开面板；`Trigger(ui.TriggerHover)` 改为悬停触发。可通过 `Changed`、`Cleared` 处理本帧变化。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/theme"
)

var color = theme.Hex("#fa8c16")

func page(c *ui.Context) {
    w := ui.ColorPicker(c, &color).Trigger(ui.TriggerHover).Status("warning")
    if w.Changed() || w.Cleared() {
        // 保存颜色或刷新预览
    }
}
```

## 主题与样式

- 组件令牌：`theme.Components.ColorPicker`（`ColorPickerToken`），包括 `PanelWidth`、`PaletteHeight`、`BarHeight`、`HandleSize`、`SwatchSize` 等；Ant、Element Plus、shadcn 会分别调整面板圆角、输入控件和预设色布局。
- `Size` 支持 `ui.Small`、默认值和 `ui.Large`；`Status("error" | "warning")` 用于表单反馈。
- 渐变和单色选择器共用颜色编辑器令牌，渐变额外包含色标轨道和角度输入。
- `ConfigProvider.Defaults("ColorPicker", ui.Props{...})` 支持 `size`、`showText`、`allowClear`、`disabledAlpha`、`format`（`"hex"`、`"rgb"`、`"hsb"`、`"hsv"`）。

## 注意事项

- 将 `theme.Color` 或 `theme.Gradient` 放在稳定状态中；零值表示清空，不要把零值误认为黑色。
- 颜色变化是即时的；若要取消编辑，应在业务层保存旧值并自行恢复。
- 键盘可在色标轨道上用 Left / Right 移动、Up / Down 切换色标，Enter / Insert 添加，Delete / Backspace 删除；渐变至少保留两个色标。
- 触发器没有可见文字时必须设置 `A11yLabel`；在表单项中组件会继承表单标签。

## 示例代码

<!--@include: ./_gen/colorpicker-examples.md-->

## API

<!--@include: ./_gen/colorpicker-api.md-->

## 相关链接

- [源码 ui/entry_color.go](https://github.com/tangoui/tango/blob/main/ui/entry_color.go)
- [ui Go API](/reference/api/ui)
- [Input 输入框](/components/input)、[Form 表单](/components/form)
