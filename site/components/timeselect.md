---
title: TimeSelect 时间选择
---
# TimeSelect 时间选择

从按固定间隔生成的时间列表中选择一个 `HH:MM` 时间点。

## 何时使用

- 预约时段、班次或营业时间等只能选固定档位的场景。
- 需要通过开始、结束和步长自动生成选项，而不是手写 `[]ui.Option` 时。
- 需要滚动时分秒或自由输入时间时，用 [TimePicker 时间选择](/components/timepicker)。
- 需要两个时间点组成区间时，用 [TimeRangePicker 时间范围](/components/timerangepicker)。

## 在线体验

<GalleryPreview component="TimeSelect" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

var deliveryTime string

func page(c *ui.Context) {
    ui.TimeSelect(c, &deliveryTime).
        Start("08:30").End("18:30").Step("00:15").
        Placeholder("选择配送时间")
}
```

值是 `"HH:MM"` 字符串；默认范围为 09:00–18:00，步长为 30 分钟，空字符串表示尚未选择。

## 常见用法

### 生成固定间隔

`Start` 和 `End` 是包含边界的时间，`Step` 是增量，三者都使用 `HH:MM` 格式。

```go
import "github.com/tangoui/tango/ui"

var slot string

func page(c *ui.Context) {
    ui.TimeSelect(c, &slot).Start("09:00").End("12:00").Step("00:30").Width(200)
}
```

### 限制可选范围

`MinTime` 会禁用小于等于该值的选项，`MaxTime` 会禁用大于等于该值的选项。它们适合让结束时间晚于开始时间。

```go
import "github.com/tangoui/tango/ui"

var begin, finish string

func page(c *ui.Context) {
    ui.Row(c, func() {
        ui.TimeSelect(c, &begin).
            Start("08:30").End("18:30").Step("00:15").MaxTime(finish).
            Placeholder("开始时间")
        ui.TimeSelect(c, &finish).
            Start("08:30").End("18:30").Step("00:15").MinTime(begin).
            Placeholder("结束时间")
    })
}
```

### 查看生成的选项

`Options` 返回当前范围生成的 `[]ui.Option`；每个选项的 `Disabled` 会反映 `MinTime` 和 `MaxTime`。可用于显示摘要或调试配置。

```go
import (
    "fmt"

    "github.com/tangoui/tango/ui"
)

var selected string

func page(c *ui.Context) {
    picker := ui.TimeSelect(c, &selected).Start("09:00").End("10:00").Step("00:15")
    ui.Text(c, fmt.Sprintf("共 %d 个时间点", len(picker.Options())))
}
```

### 状态与变更

`Status` 显示错误或警告状态；`Changed` 只在本帧选中、清除或键盘提交后返回 `true`。`Disabled(true)` 禁止打开列表。

```go
import "github.com/tangoui/tango/ui"

var cutoff string

func page(c *ui.Context) {
    picker := ui.TimeSelect(c, &cutoff).Status("error").Clearable(true)
    if picker.Changed() {
        ui.Message(c, "success", "时间已更新")
    }
}
```

## 主题与样式

- `Size(ui.Small)`、默认尺寸和 `Size(ui.Large)` 控制控件尺寸；`Variant("outlined" | "filled" | "borderless" | "underlined")` 控制外框。
- 下拉列表沿用 Select 的选项高度、焦点和禁用项令牌；三套主题会分别调整输入框和列表的密度。
- `ConfigProvider.Defaults("TimeSelect", ...)` 支持 `start`、`end`、`step`、`placeholder`、`clearable`/`allowClear`、`size`、`width`、`variant`。

## 注意事项

- 把字符串值放在稳定状态中；不要在页面函数中每帧重新声明它，否则选择结果会被重置。
- `Start`、`End`、`Step` 使用 24 小时制 `HH:MM`；非法配置会生成空选项列表。
- `MinTime`/`MaxTime` 是边界禁用规则，不是自动调整值；开始时间变化后，结束时间若落在禁用项上不会被偷偷改写。
- 组件提供 combobox/listbox 键盘语义；没有可见标签时设置 `A11yLabel`。

## 示例代码

<!--@include: ./_gen/timeselect-examples.md-->

## API

<!--@include: ./_gen/timeselect-api.md-->

## 相关链接

- [源码 ui/more_timeselect.go](https://github.com/tangoui/tango/blob/main/ui/more_timeselect.go)
- [ui Go API](/reference/api/ui)
- [TimePicker 时间选择](/components/timepicker)、[TimeRangePicker 时间范围](/components/timerangepicker)、[Select 选择器](/components/select)
