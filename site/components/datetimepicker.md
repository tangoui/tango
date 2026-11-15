---
title: DateTimePicker 日期时间选择
---
# DateTimePicker 日期时间选择

在一个输入框中同时选择日期和时间，确认后把结果写回 `time.Time`。

## 何时使用

- 预约、发布、截止时间等需要精确到时分秒的单个时间点。
- 需要限制工作日、工作时段，或提供“明早”“月底”等快捷时间时。
- 只选日期用 [DatePicker 日期选择](/components/datepicker)；只选时间用 [TimePicker 时间选择](/components/timepicker)。
- 需要两个时间点组成区间时，用 [RangePicker](/components/rangepicker) 或 [TimeRangePicker 时间范围](/components/timerangepicker)。

## 在线体验

<GalleryPreview component="DateTimePicker" />

## 基本用法

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
)

var publishAt = time.Date(2026, 1, 15, 9, 30, 0, 0, time.UTC)

func page(c *ui.Context) {
    ui.DateTimePicker(c, &publishAt).A11yLabel("发布时间")
}
```

`DateTimePicker` 绑定 `*time.Time`；零值表示没有选择。面板中的日期和时间先作为草稿，点击“确定”或按 Enter 后才提交。

## 常见用法

### 自定义格式与步长

`Format` 使用 Go 的时间布局；省略秒会隐藏秒列。`Steps` 分别设置时、分、秒列的步长。

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
)

var startAt time.Time

func page(c *ui.Context) {
    ui.DateTimePicker(c, &startAt).
        Format("2006-01-02 15:04").
        Steps(1, 15, 0).
        Width(240).
        Placeholder("选择开始时间")
}
```

### 禁用日期和时间

`DisabledDate` 收到 UTC 午夜时间；`DisabledTime` 收到当前草稿日期，并返回需要禁用的时、分、秒。禁用规则也会阻止手动输入或预设值提交。

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
)

var meetingAt time.Time

func page(c *ui.Context) {
    ui.DateTimePicker(c, &meetingAt).
        DisabledDate(func(day time.Time) bool {
            return day.Weekday() == time.Saturday || day.Weekday() == time.Sunday
        }).
        DisabledTime(func(day time.Time) ui.DisabledTimes {
            return ui.DisabledTimes{
                Hours: func(h int) bool { return h < 9 || h > 18 },
                Minutes: func(h, m int) bool { return h == 18 && m != 0 },
            }
        })
}
```

### 快捷时间

`Presets` 接受 `DatePreset`。填写 `Value` 表示单个时间点；`Range` 也可用于兼容范围预设，但 DateTimePicker 只采用预设的第一个值。

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
)

var dueAt = time.Time{}

func page(c *ui.Context) {
    ui.DateTimePicker(c, &dueAt).Presets(
        ui.DatePreset{Label: "明早", Value: time.Date(2026, 1, 16, 9, 0, 0, 0, time.UTC)},
        ui.DatePreset{Label: "月底", Value: time.Date(2026, 1, 31, 18, 0, 0, 0, time.UTC)},
    )
}
```

### 状态、清除与提交反馈

`Status("error")` 或 `Status("warning")` 改变输入框反馈；`AllowClear(true)` 显示清除操作。`Changed` 只在本帧确认或清除后返回 `true`。

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
)

var deadline time.Time

func page(c *ui.Context) {
    picker := ui.DateTimePicker(c, &deadline).
        Status("warning").AllowClear(true).Open()
    if picker.Changed() {
        ui.Message(c, "success", "时间已更新")
    }
}
```

## 主题与样式

- `Size(ui.Small)`、默认尺寸和 `Size(ui.Large)` 控制输入框尺寸；`Variant("outlined" | "filled" | "borderless" | "underlined")` 控制外框。
- `DateTimePicker` 使用日期、时间输入组件的主题令牌；Ant Design、Element Plus、shadcn 会分别调整面板、输入列和确认区的密度。
- `ConfigProvider.Defaults("DateTimePicker", ...)` 支持 `format`、`placeholder`、`allowClear`、`width`、`size`、`variant`。

## 注意事项

- 把 `time.Time` 放在页面函数外的稳定状态中；即时模式每帧重建控件，但不会替你保存业务状态。
- `Format` 是 Go 时间布局，不是前端常见的 `YYYY-MM-DD` 写法，例如分钟必须写 `04`。
- `DisabledDate` 与 `DisabledTime` 会同时约束点击、键盘输入和预设值；回调中的日期按 UTC 午夜传入。
- 输入框和面板支持键盘操作；没有可见表单标签时设置 `A11yLabel`。

## 示例代码

<!--@include: ./_gen/datetimepicker-examples.md-->

## API

<!--@include: ./_gen/datetimepicker-api.md-->

## 相关链接

- [源码 ui/entry_datetime.go](https://github.com/tangoui/tango/blob/main/ui/entry_datetime.go)
- [ui Go API](/reference/api/ui)
- [DatePicker 日期选择](/components/datepicker)、[TimePicker 时间选择](/components/timepicker)、[RangePicker 日期范围](/components/rangepicker)
