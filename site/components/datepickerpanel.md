---
title: DatePickerPanel 日期面板
---
# DatePickerPanel 日期面板

把日期选择面板直接嵌入页面，不带输入框和弹出层，支持日、周、月、季度、年和范围选择。

## 何时使用

- 需要在筛选栏、仪表盘或设置页中常驻展示日期面板时。
- 需要用户直接浏览并选择一个日期周期，而不是先点击输入框打开弹层时。
- 需要输入框式弹出选择器时，用 [DatePicker 日期选择框](/components/datepicker)。
- 需要两个端点的紧凑触发器时，用 [RangePicker 日期范围选择](/components/rangepicker)。
- 需要展示事件和日程而不是选择日期时，用 [Calendar 日历](/components/calendar)。

## 在线体验

<GalleryPreview component="DatePickerPanel" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
)

var day = time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)

func page(c *ui.Context) {
    panel := ui.DatePickerPanel(c, &day)
    if panel.Changed() {
        ui.Text(c, "已选择："+day.Format("2006-01-02"))
    }
}
```

面板直接绑定 `*time.Time`；选中的值会规范为 UTC 零点。`PickerDate` 是默认的日视图，点击面板标题可在日期、月份和年份视图间切换。

## 常见用法

### 选择不同粒度

`Picker` 支持 `PickerDate`、`PickerWeek`、`PickerMonth`、`PickerQuarter` 和 `PickerYear`。周、月、季度、年的绑定值会规范到对应周期的开始日。

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
)

var (
    week time.Time
    month time.Time
    quarter time.Time
    year time.Time
)

func page(c *ui.Context) {
    ui.DatePickerPanel(c, &week).Picker(ui.PickerWeek)
    ui.DatePickerPanel(c, &month).Picker(ui.PickerMonth)
    ui.DatePickerPanel(c, &quarter).Picker(ui.PickerQuarter)
    ui.DatePickerPanel(c, &year).Picker(ui.PickerYear)
}
```

### 范围选择

`Range(end)` 把面板变成范围选择器，开始值和结束值分别由两个 `*time.Time` 保存。第二次点击完成范围，面板会并排显示两个选择区域。

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
)

var (
    from = time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
    to   = time.Date(2026, 12, 8, 0, 0, 0, 0, time.UTC)
)

func page(c *ui.Context) {
    ui.DatePickerPanel(c, &from).Range(&to)
}
```

范围端点在第二次选择后都会写入；`Changed()` 只有范围完成或已提交的端点发生变化时才报告变化。

### 禁用日期与无边框面板

`DisabledDate` 接收周期的第一天，返回 `true` 的周期不可选择。`Border(false)` 适合已经有卡片或表单边框的布局，`Disabled(true)` 则禁用整个面板。

`MinDate(t)` 和 `MaxDate(t)` 限制可选日期和标题栏翻页范围。日视图可用 `ShowWeek(true)` 显示 ISO 周数，周选择器始终显示周数。

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
)

var selected time.Time

func weekend(day time.Time) bool {
    return day.Weekday() == time.Saturday || day.Weekday() == time.Sunday
}

func page(c *ui.Context) {
    ui.DatePickerPanel(c, &selected).DisabledDate(weekend).Border(false)
}
```

### Locale 与周起始日

面板使用当前 `ConfigProvider` 的 locale；周视图的起始日来自 `Locale.WeekStart`，不应在业务层手工偏移日期。

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
)

var monday = func() ui.Locale {
    locale := *ui.EnUS
    locale.WeekStart = time.Monday
    return locale
}()

var localeDay time.Time

func page(c *ui.Context) {
    ui.ConfigProvider(c).Locale(&monday).Body(func() {
        ui.DatePickerPanel(c, &localeDay).Picker(ui.PickerWeek)
    })
}
```

### 读取视图与变化

`View()` 返回当前展示的粒度；日视图返回 `PickerDate`。在需要展示当前选择结果或触发查询时，使用 `Changed()` 配合绑定值。

```go
import (
    "fmt"
    "time"

    "github.com/tangoui/tango/ui"
)

var (
    panelDateValue time.Time
    panelDateText  string
)

func page(c *ui.Context) {
    w := ui.DatePickerPanel(c, &panelDateValue)
    if w.Changed() {
        panelDateText = fmt.Sprint(w.View())
    }
    ui.Text(c, panelDateText)
}
```

## 主题与样式

- DatePickerPanel、DatePicker、RangePicker 和 DateTimePicker 共用 `theme.Components.DatePicker`（`DatePickerToken`）。
- 日期面板相关令牌包括 `CellWidth`、`CellHeight`、`CellInnerSize`、`CellRadius`、`CellActiveBg`、`CellRangeBg`、`CellRangePreviewBg`、`PanelBg`、`PanelBorderColor` 和 `SplitColor`。
- `ConfigProvider.Defaults("DatePicker", ui.Props{"picker": ui.PickerMonth})` 可以设置默认粒度；显式 `Picker` 方法优先。面板没有输入框的 `placeholder` 或 `allowClear` 行为。

## 注意事项

- **时区**：选中值是 UTC 零点，并且会归一化到周期开始；与本地日期显示或比较时先明确时区转换策略。
- **状态**：绑定的 `time.Time` 和范围端点由应用保存；面板当前浏览视图、光标和进行中的范围由调用位置保存。循环中渲染多个面板时用 `ui.Key`。
- **键盘**：面板支持方向键移动、PageUp / PageDown 翻页、Enter 选择；禁用周期不会被键盘选中。
- **无障碍**：日期网格和标题提供键盘语义；放在表单中时用 `FormItem` 标签说明选择内容，独立面板可用外围文本说明。
- **禁用回调**：`DisabledDate` 的参数是周期第一天，不要在回调中按当前光标的任意日期判断。

## 示例代码

<!--@include: ./_gen/datepickerpanel-examples.md-->

## API

<!--@include: ./_gen/datepickerpanel-api.md-->

## 相关链接

- [源码 ui/fill_date_panel.go](https://github.com/tangoui/tango/blob/main/ui/fill_date_panel.go)、[ui/date_util.go](https://github.com/tangoui/tango/blob/main/ui/date_util.go)
- [ui Go API](/reference/api/ui#DatePickerPanel)
- [DatePicker 日期选择框](/components/datepicker)、[RangePicker 日期范围选择](/components/rangepicker)、[Calendar 日历](/components/calendar)
