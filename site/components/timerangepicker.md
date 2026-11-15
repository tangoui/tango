---
title: TimeRangePicker 时间范围
---
# TimeRangePicker 时间范围

同时选择开始和结束时间，适合班次、营业时间和同一天内的时段配置。

## 何时使用

- 需要两个时钟值组成一个时间区间时。
- 需要 12 小时制、固定步长或按小时/分钟禁用选项时。
- 只有一个时间点用 [TimePicker 时间选择](/components/timepicker)；固定时间列表用 [TimeSelect 时间选择](/components/timeselect)。
- 日期也参与区间时，用 [RangePicker 日期范围](/components/rangepicker) 的 `ShowTime()`。

## 在线体验

<GalleryPreview component="TimeRangePicker" />

## 基本用法

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
)

var officeHours = [2]time.Time{
    time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC),
    time.Date(2026, 1, 1, 18, 0, 0, 0, time.UTC),
}

func page(c *ui.Context) {
    ui.TimeRangePicker(c, &officeHours).Format("15:04").A11yLabel("营业时间")
}
```

值是 `[2]time.Time{开始, 结束}`；零值表示对应一端尚未选择，确认后组件会按时钟顺序提交区间。

## 常见用法

### 设置步长与 12 小时制

`HourStep`、`MinuteStep`、`SecondStep` 控制三列的间隔。`Use12Hours` 配合 `Format("03:04 PM")` 显示 AM/PM。

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
)

var supportHours [2]time.Time

func page(c *ui.Context) {
    ui.TimeRangePicker(c, &supportHours).
        Use12Hours().Format("03:04 PM").MinuteStep(15)
}
```

### 禁用时间

`DisabledHours`、`DisabledMinutes` 和 `DisabledSeconds` 收到 24 小时制的时、分、秒，即使界面使用 12 小时制也按 0–23 判断。

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
)

var quietHours [2]time.Time

func page(c *ui.Context) {
    ui.TimeRangePicker(c, &quietHours).
        Format("15:04").
        DisabledHours(func(h int) bool { return h < 8 || h >= 22 }).
        DisabledMinutes(func(h, m int) bool { return h == 21 && m > 30 }).
        HideDisabledOptions(true)
}
```

### 占位符、状态与清除

两端可以分别设置占位文字；`Status` 用于表单错误反馈，`AllowClear(false)` 关闭悬停清除。

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
)

var deployWindow [2]time.Time

func page(c *ui.Context) {
    picker := ui.TimeRangePicker(c, &deployWindow).
        Placeholder("开始", "结束").
        Status("error").
        AllowClear(false)
    if picker.Changed() {
        ui.Message(c, "info", "部署窗口已提交")
    }
}
```

### 键盘与展开状态

`Open` 适合截图、引导或需要首帧展开的场景；真实交互中用户可点击输入框或按 Enter 打开，左右键切换开始/结束列，Enter 提交，Escape 关闭。

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
)

var reviewWindow [2]time.Time

func page(c *ui.Context) {
    picker := ui.TimeRangePicker(c, &reviewWindow).Open()
    ui.Text(c, "已展开："+boolText(picker.IsOpen()))
}

func boolText(on bool) string {
    if on {
        return "是"
    }
    return "否"
}
```

## 主题与样式

- `Size(ui.Small)`、默认尺寸和 `Size(ui.Large)` 控制触发器；`Variant("outlined" | "filled" | "borderless" | "underlined")` 控制外观。
- 主题会分别调整两个时间段、分隔符、时间列和禁用项的密度；`HideDisabledOptions(true)` 只改变禁用项的呈现，不改变规则。
- `ConfigProvider.Defaults("TimeRangePicker", ...)` 支持 `format`、`separator`、`use12Hours`、步长、`allowClear`、`width`、`size`、`variant` 和 `placeholder`。

## 注意事项

- 绑定值应放在稳定状态中；即时模式下组件每帧重建，但 `[2]time.Time` 的值由调用方保存。
- `Format` 是 Go 时间布局；使用 12 小时制时省略 `Format` 也会使用默认的 AM/PM 布局。
- 禁用规则按 24 小时制执行；无效或被禁用的手动输入不会提交。
- 没有表单标签时设置 `A11yLabel`；组件的开始/结束占位符会作为无障碍名称的后备文本。

## 示例代码

<!--@include: ./_gen/timerangepicker-examples.md-->

## API

<!--@include: ./_gen/timerangepicker-api.md-->

## 相关链接

- [源码 ui/p2a_time_range.go](https://github.com/tangoui/tango/blob/main/ui/p2a_time_range.go)
- [ui Go API](/reference/api/ui)
- [TimePicker 时间选择](/components/timepicker)、[TimeSelect 时间选择](/components/timeselect)、[RangePicker 日期范围](/components/rangepicker)
