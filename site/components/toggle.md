---
title: Toggle 切换按钮 / ToggleGroup
---
# Toggle 切换按钮 / ToggleGroup

显示一个可保持开关状态的按钮，或把多个 Toggle 组成多选、单选和带边框的按钮组。

## 何时使用

- 工具栏中的加粗、斜体、显示网格等可保持状态的动作使用 `Toggle`。
- 同一组互斥或可多选的视图选项使用 `ToggleGroup`。
- 一次性提交、删除或打开对话框使用 [Button 按钮](/components/button)，不要用 Toggle 表示瞬时动作。
- 二态但需要滑块视觉和即时开关语义的设置使用 [Switch 开关](/components/switch)。

## 在线体验

<GalleryPreview component="Toggle" />

## 基本用法

`Toggle` 绑定一个 `*bool`；点击后值会保持，`Changed()` 报告本帧是否发生翻转。

```go
import "github.com/tangoui/tango/ui"

var boldEnabled bool

func togglePage(c *ui.Context) {
    toggle := ui.Toggle(c, "加粗", &boldEnabled)
    if toggle.Changed() {
        ui.Text(c, "加粗状态已更新")
    }
    ui.Text(c, "当前状态："+map[bool]string{true: "开启", false: "关闭"}[boldEnabled])
}
```

## 常见用法

### 图标、outline 和禁用

`Icon` 在文字前放图标，`IconOnly` 只绘制图标，`Outline` 使用带边框变体；图标独占时要提供 `A11yLabel`。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
    "github.com/tangoui/tango/ui/layout"
)

var italicEnabled bool

func styledTogglePage(c *ui.Context) {
    ui.Row(c, func() {
        ui.Toggle(c, "斜体", &italicEnabled).Icon(icon.Edit)
        ui.Toggle(c, "下划线", &italicEnabled).Outline()
        ui.Toggle(c, "", &italicEnabled).IconOnly(icon.Edit).A11yLabel("切换编辑模式")
        ui.Toggle(c, "不可用", &italicEnabled).Disabled(true)
    }, layout.Gap(8))
}
```

### 多选 ToggleGroup

默认的 `ToggleGroup` 绑定 `[]string`，选中值可以同时存在多个；`Option.Disabled` 禁止对应项改变。

```go
import "github.com/tangoui/tango/ui"

var formatOptions = []string{"bold"}

func multiTogglePage(c *ui.Context) {
    group := ui.ToggleGroup(c, &formatOptions, []ui.Option{
        {Label: "加粗", Value: "bold"},
        {Label: "斜体", Value: "italic"},
        {Label: "下划线", Value: "under", Disabled: true},
    }).Outline()
    if group.Changed() {
        ui.Text(c, "格式选项已更新")
    }
}
```

### 单选组和尺寸

`Single()` 让组最多保留一个值并使用单选语义；`Size` 应用于组内所有 Toggle，`Icons` 可以把组显示成图标工具栏。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

var alignOption = []string{"left"}

func singleTogglePage(c *ui.Context) {
    ui.ToggleGroup(c, &alignOption, []ui.Option{
        {Label: "左对齐", Value: "left"},
        {Label: "居中", Value: "center"},
        {Label: "右对齐", Value: "right"},
    }).Single().Outline().Size(ui.Small).Icons(map[string]*icon.Icon{
        "left": icon.Left,
        "center": icon.Home,
        "right": icon.Right,
    }).A11yLabel("对齐方式")
}
```

## 主题与样式

- Ant Design：使用主题的次级填充和选中背景；`Outline` 绘制输入边框和阴影。
- Element Plus：保持同样的 Toggle / ToggleGroup 行为，控件尺寸来自当前主题。
- shadcn：默认是透明背景，按下后使用选中填充；`Outline` 对应带边框的工具栏变体。
- `ConfigProvider.Defaults("Toggle", ui.Props{"variant": "outline", "size": ui.Small})` 和 `ToggleGroup` 的同名默认键可设置默认外观；实例链式方法优先。

## 注意事项

- **状态**：`bool` 或 `[]string` 必须放在页面外部或 `ui.State` 中；不要在页面函数内每帧重新声明并初始化。
- **单选清空**：`Single()` 允许再次点击清空选中值，因此业务层需要处理空切换状态。
- **键盘**：组使用一个 Tab 停靠点，方向键移动，Space / Enter 切换；不要把同一组拆成互不相关的焦点顺序。
- **无障碍**：图标独占 Toggle 必须调用 `A11yLabel`；单选组建议给组设置有意义的可访问名称。
- **RTL**：按钮顺序和组边角会按方向镜像，`[]string` 中的值顺序仍是业务数据顺序。

## 示例代码

<!--@include: ./_gen/toggle-examples.md-->

## API

<!--@include: ./_gen/toggle-api.md-->

## 相关链接

- [源码 shad_toggle.go](https://github.com/tangoui/tango/blob/main/ui/shad_toggle.go)
- [ui Go API](/reference/api/ui)
- [Button 按钮](/components/button)、[Switch 开关](/components/switch)、[Kbd 键帽](/components/kbd)
