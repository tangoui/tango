---
title: RangePicker 日期范围选择
---
# RangePicker 日期范围选择

用一个输入框触发两个相邻面板，选择并保存日期、周期或日期时间范围。

## 何时使用

- 需要查询报表、日志或日程的开始和结束日期时。
- 需要把两个端点放在紧凑输入框中，并支持键盘输入或预设范围时。
- 需要直接把日期面板嵌入页面时，用 [DatePickerPanel 日期面板](/components/datepickerpanel)。
- 只选择一个日期时，用 [DatePicker 日期选择框](/components/datepicker)。
- 只选择时刻、不包含日期时，用 [TimePicker 时间选择框](/components/timepicker)。

## 在线体验

<GalleryPreview component="RangePicker" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
)

var period [2]time.Time

func page(c *ui.Context) {
    picker := ui.RangePicker(c, &period).Placeholder("开始日期", "结束日期")
    if picker.Changed() && !period[0].IsZero() && !period[1].IsZero() {
        ui.Text(c, period[0].Format("2006-01-02")+" 至 "+period[1].Format("2006-01-02"))
    }
}
```

范围绑定到 `*[2]time.Time`，零值表示没有对应端点。第一次点击选择开始端，第二次点击选择结束端，结果会自动按时间先后排序；也可以在两个输入段中直接键入日期。

## 常见用法

### 周、月、季度和年范围

`Picker` 同时改变两个端点的粒度。周期范围保存为周期第一天的 UTC 零点，`ShowTime` 对这些粒度不起作用。

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
)

var (
    weeks [2]time.Time
    months [2]time.Time
    quarters [2]time.Time
    years [2]time.Time
)

func page(c *ui.Context) {
    ui.RangePicker(c, &weeks).Picker(ui.PickerWeek)
    ui.RangePicker(c, &months).Picker(ui.PickerMonth)
    ui.RangePicker(c, &quarters).Picker(ui.PickerQuarter)
    ui.RangePicker(c, &years).Picker(ui.PickerYear)
}
```

### 日期时间范围

`ShowTime` 在两个日期面板下方增加时间列；草稿会在点“确定”或按 Enter 后提交。使用不含秒的 Go 时间布局可以隐藏秒列。

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
)

var meeting [2]time.Time

func page(c *ui.Context) {
    ui.RangePicker(c, &meeting).ShowTime().
        Format("2006-01-02 15:04").Placeholder("开始时间", "结束时间")
}
```

### 预设范围

`Presets` 在面板中显示快捷项，点击后直接写入 `DatePreset.Range`。日期值建议明确使用 UTC，避免本地时区导致边界日期变化。

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
)

var reportPeriod [2]time.Time

func page(c *ui.Context) {
    today := time.Now().UTC().Truncate(24 * time.Hour)
    ui.RangePicker(c, &reportPeriod).Presets(
        ui.DatePreset{Label: "最近 7 天", Range: [2]time.Time{today.AddDate(0, 0, -6), today}},
        ui.DatePreset{Label: "最近 30 天", Range: [2]time.Time{today.AddDate(0, 0, -29), today}},
    )
}
```

### 禁用日期与时间

`DisabledDate` 禁止日期，`DisabledTime` 按开始端或结束端分别返回不可选的小时、分钟和秒。禁用的端点不会被键盘输入绕过。

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
)

var booking [2]time.Time

func weekend(day time.Time) bool {
    return day.Weekday() == time.Saturday || day.Weekday() == time.Sunday
}

func page(c *ui.Context) {
    ui.RangePicker(c, &booking).DisabledDate(weekend).ShowTime().
        DisabledTime(func(day time.Time, end bool) ui.DisabledTimes {
            return ui.DisabledTimes{
                Hours: func(hour int) bool { return hour < 9 || hour >= 18 },
            }
        })
}
```

回调的 `end` 为 `false` 表示开始端，为 `true` 表示结束端；`day` 是对应端的日期。

### 外观、宽度与无障碍

`Variant` 支持 outlined、filled、borderless 和 underlined；`Status`、`Size`、`Width` 和 `A11yLabel` 可用于表单反馈、布局和辅助技术。

```go
import "github.com/tangoui/tango/ui"

var styledPeriod [2]time.Time

func page(c *ui.Context) {
    ui.RangePicker(c, &styledPeriod).Variant("filled").Width(320).
        Status("warning").Size(ui.Large).A11yLabel("报告时间范围")
}
```

## 主题与样式

- RangePicker 的日历、范围带、预览范围和时间列共用 `theme.Components.DatePicker`（`DatePickerToken`）；触发器边框沿用输入框令牌。
- 日期范围相关令牌包括 `CellRangeBg`、`CellRangePreviewBg`、`CellRangeHoverBorder`、`CellActiveBg`、`PanelBg`、`TimeCellActiveBg` 和 `FooterHeight`。
- `ConfigProvider.Defaults("RangePicker", ui.Props{...})` 支持 `format`、`separator`、`allowClear`、`showTime`、`width`、`size`、`variant`、`picker` 和 `placeholder`（类型为 `[2]string`）。显式链式方法优先。

## 注意事项

- **提交时机**：普通日期范围第二次选择后提交；`ShowTime` 的日期和时间先是草稿，点“确定”或按 Enter 才提交。`Changed()` 只报告已提交的范围变化。
- **状态**：使用稳定的 `*[2]time.Time` 变量保存值；打开状态、当前端点、月份和键盘光标由组件按调用位置保存。循环中使用 `ui.Key` 区分多个范围框。
- **时区**：默认日期布局是 UTC 日期语义；`ShowTime` 解析时会参考已有值的 location，业务层应统一时间存储和展示时区。
- **无障碍**：RangePicker 暴露可展开的组合框语义，打开后可用方向键、PageUp / PageDown、Enter 和 Escape；独立使用时用 `A11yLabel`。
- **测试与截图**：`Open()` 强制打开面板，`IsOpen()` 查询状态，适合测试和 gallery，不要在正常交互中永久调用 `Open()`。

## 示例代码

<!--@include: ./_gen/rangepicker-examples.md-->

## API

<!--@include: ./_gen/rangepicker-api.md-->

## 相关链接

- [源码 ui/entry_daterange.go](https://github.com/tangoui/tango/blob/main/ui/entry_daterange.go)、[ui/p2a_range_time.go](https://github.com/tangoui/tango/blob/main/ui/p2a_range_time.go)
- [ui Go API](/reference/api/ui#RangePicker)
- [DatePicker 日期选择框](/components/datepicker)、[DatePickerPanel 日期面板](/components/datepickerpanel)、[TimeRangePicker 时间范围](/components/timepicker)
