---
title: DatePicker 日期选择框
---
# DatePicker 日期选择框

输入或选择日期的控件。除了单个日期 `DatePicker`，还提供日期范围 `RangePicker`、多选日期 `MultiDatePicker` 和日期时间 `DateTimePicker`，并支持周 / 月 / 季度 / 年等粒度、预设快捷项和禁用日期。

## 何时使用

- 需要输入一个日期（生日、截止日期）或一个时间段（统计区间、请假时间）时。
- 只需要一天中的时间，不需要日期时，用 [TimePicker 时间选择框](/components/timepicker)。
- 需要在页面上直接展示整月日程或事件时，用 [Calendar 日历](/components/catalog/calendar)。

## 在线体验

<GalleryPreview component="DatePicker" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
	"time"

	"github.com/tangoui/tango/ui"
)

var deadline time.Time

func page(c *ui.Context) {
	ui.DatePicker(c, &deadline).Placeholder("请选择截止日期")
	if !deadline.IsZero() {
		ui.Text(c, "截止："+deadline.Format("2006-01-02"))
	}
}
```

值绑定到 `*time.Time`，零值表示未选择。点击或按 Enter 打开面板，方向键移动、PageUp / PageDown 翻月、Enter 选中；也可以直接在输入框中键入日期，Enter 或离开输入框时生效。选出的值是 **UTC 零点**。`Format` 设置显示格式（Go 时间布局，默认 `"2006-01-02"`）。

## 常见用法

### 选择器粒度

`Picker` 切换粒度：`ui.PickerWeek`、`ui.PickerMonth`、`ui.PickerQuarter`、`ui.PickerYear`。值为对应周期的第一天（UTC 零点）。

```go
import (
	"time"

	"github.com/tangoui/tango/ui"
)

var week, month, quarter, year time.Time

func page(c *ui.Context) {
	ui.DatePicker(c, &week).Picker(ui.PickerWeek)
	ui.DatePicker(c, &month).Picker(ui.PickerMonth).Format("2006-01")
	ui.DatePicker(c, &quarter).Picker(ui.PickerQuarter)
	ui.DatePicker(c, &year).Picker(ui.PickerYear)
}
```

### 日期范围 RangePicker

`RangePicker` 绑定 `*[2]time.Time`：第一次点击选一端，第二次点击选另一端，结果自动按先后排序。`Placeholder(开始, 结束)` 设置两端占位符；`Picker` 同样适用于范围；`ShowTime()` 在面板下方加入开始 / 结束时间列，点“确定”后提交。

```go
import (
	"time"

	"github.com/tangoui/tango/ui"
)

var (
	span     [2]time.Time
	shift    [2]time.Time
	quarters [2]time.Time
)

func page(c *ui.Context) {
	ui.RangePicker(c, &span).Placeholder("开始日期", "结束日期")
	ui.RangePicker(c, &shift).ShowTime().Format("2006-01-02 15:04")
	ui.RangePicker(c, &quarters).Picker(ui.PickerQuarter)
}
```

### 预设快捷项

`Presets(ui.DatePreset{...})` 在面板侧边显示命名的快捷项，点击后立即选中。`DatePicker` / `DateTimePicker` 使用 `Value`，`RangePicker` 使用 `Range`。

```go
import (
	"time"

	"github.com/tangoui/tango/ui"
)

var (
	day    time.Time
	period [2]time.Time
)

func page(c *ui.Context) {
	now := time.Now().UTC().Truncate(24 * time.Hour)
	ui.DatePicker(c, &day).Presets(
		ui.DatePreset{Label: "今天", Value: now},
		ui.DatePreset{Label: "昨天", Value: now.AddDate(0, 0, -1)},
		ui.DatePreset{Label: "一周前", Value: now.AddDate(0, 0, -7)},
	)
	ui.RangePicker(c, &period).Presets(
		ui.DatePreset{Label: "最近 7 天", Range: [2]time.Time{now.AddDate(0, 0, -7), now}},
		ui.DatePreset{Label: "最近 30 天", Range: [2]time.Time{now.AddDate(0, 0, -30), now}},
	)
}
```

### 禁用日期与时间

`DisabledDate(func(time.Time) bool)` 返回 `true` 的日期不可选也不可键入（使用 `Picker` 时传入的是周期的第一天）。带时间的选择器还可以用 `DisabledTime` 返回 `ui.DisabledTimes`，按天禁用小时、分钟、秒；`RangePicker` 的回调多一个 `end` 参数区分开始和结束端。

```go
import (
	"time"

	"github.com/tangoui/tango/ui"
)

var (
	visit   time.Time
	meeting time.Time
)

func weekend(d time.Time) bool { return d.Weekday() == time.Saturday || d.Weekday() == time.Sunday }

func page(c *ui.Context) {
	ui.DatePicker(c, &visit).DisabledDate(weekend)
	ui.DateTimePicker(c, &meeting).DisabledDate(weekend).Format("2006-01-02 15:04").
		DisabledTime(func(day time.Time) ui.DisabledTimes {
			return ui.DisabledTimes{Hours: func(h int) bool { return h < 9 || h >= 18 }}
		})
}
```

### 日期时间 DateTimePicker

`DateTimePicker` 在日期面板旁边加上时 / 分 / 秒列，绑定 `*time.Time`。`Steps(时, 分, 秒)` 设置步长，`Format` 中不含秒时隐藏秒列。

```go
import (
	"time"

	"github.com/tangoui/tango/ui"
)

var startAt time.Time

func page(c *ui.Context) {
	ui.DateTimePicker(c, &startAt).Format("2006-01-02 15:04").Steps(1, 15, 1).Placeholder("开始时间")
}
```

### 多选日期 MultiDatePicker

`MultiDatePicker` 绑定 `*[]time.Time`：每次点击切换一个日期，面板保持打开，选中项按时间排序并以标签显示（Backspace 删除最后一个）。`MaxTagCount` 折叠多余标签，`NeedConfirm(true)` 让改动在点“确定”后才生效。

```go
import (
	"time"

	"github.com/tangoui/tango/ui"
)

var dutyDays, months []time.Time

func page(c *ui.Context) {
	ui.MultiDatePicker(c, &dutyDays).MaxTagCount(3).A11yLabel("值班日期")
	ui.MultiDatePicker(c, &months).Picker(ui.PickerMonth).NeedConfirm(true).A11yLabel("多个月份")
}
```

### 需要确认、状态与外观

`NeedConfirm(true)` 让选中的日期先作为草稿，点“确定”（或在同一天再按 Enter）才提交，关闭面板则丢弃。`AllowClear(false)` 去掉悬停时的清除按钮；`Status("error" | "warning")`、`Variant("filled" | "borderless" | "underlined")`、`Size`、`Width` 调整外观。

```go
import (
	"time"

	"github.com/tangoui/tango/ui"
)

var signedAt time.Time

func page(c *ui.Context) {
	ui.DatePicker(c, &signedAt).NeedConfirm(true)
	ui.DatePicker(c, &signedAt).Status("error").AllowClear(false).Width(240)
	ui.DatePicker(c, &signedAt).Variant("filled").Size(ui.Large)
}
```

## 主题与样式

- 三套风格共用同一套面板结构，单元格尺寸、圆角、选中色和范围高亮色来自各自的设计令牌；切换风格无需改代码。
- 组件令牌：`theme.Components.DatePicker`（`DatePickerToken`），包含单元格尺寸（`CellWidth`、`CellHeight`）、圆角（`CellRadius`）以及悬停、选中、范围、今天、禁用等状态颜色（`CellActiveBg`、`CellRangeBg`、`CellTodayBorder`…）。
- `ConfigProvider.Defaults("DatePicker", ui.Props{...})` 支持 `format`、`placeholder`、`picker`（`ui.PickerType`）、`allowClear`、`width`、`size`、`variant`；`"RangePicker"` 另有 `format`、`separator`、`allowClear`、`showTime`、`width`、`size`、`variant`、`picker`、`placeholder`（`[2]string`）；`"DateTimePicker"`、`"TimeRangePicker"` 也各有自己的默认值键。

## 注意事项

- **时区**：面板选出的日期是 UTC 零点。展示给用户或与本地时间比较时，先按需要转换时区，避免跨日偏差。
- **状态**：值就是你绑定的 `time.Time` / `[2]time.Time` / `[]time.Time`；面板是否展开、正在浏览的月份等由组件按调用位置保存。在循环中渲染多个日期框时用 `ui.Key` 包住每一行。
- **表单校验**：放进 [Form](/components/form) 时用 `ui.RulesOf(f.Item("截止日期"), &deadline, ui.CheckValue(func(t time.Time) string { ... }))`，或用 `RulesFunc` 把日期格式化成字符串后使用 `ui.Required`。
- **无障碍**：在表单项里自动以表单项标签命名，否则依次使用 `A11yLabel`、占位符。面板可完全用键盘操作。
- **截图与测试**：`Open()` 强制展开面板，只用于截图和测试，不要在正式界面里常驻调用。

## 示例代码

<!--@include: ./_gen/datepicker-examples.md-->

## API

<!--@include: ./_gen/datepicker-api.md-->

`RangePicker`、`MultiDatePicker`、`DateTimePicker` 的完整方法见 [Go API 参考](/reference/api/ui#RangePicker)；它们与 `DatePicker` 的同名方法含义一致。

## 相关链接

- [源码 ui/date.go](https://github.com/tangoui/tango/blob/main/ui/date.go)、[ui/entry_daterange.go](https://github.com/tangoui/tango/blob/main/ui/entry_daterange.go)、[ui/date_multi.go](https://github.com/tangoui/tango/blob/main/ui/date_multi.go)、[ui/entry_datetime.go](https://github.com/tangoui/tango/blob/main/ui/entry_datetime.go)
- [ui Go API](/reference/api/ui)
- [RangePicker 日期范围选择](/components/catalog/rangepicker)、[DateTimePicker 日期时间选择](/components/catalog/datetimepicker)
- [TimePicker 时间选择框](/components/timepicker)、[Calendar 日历](/components/catalog/calendar)、[Form 表单](/components/form)
