---
title: TimePicker 时间选择框
---
# TimePicker 时间选择框

输入或选择一天中的某个时间。下拉面板由时 / 分 / 秒滚动列组成，支持步长、12 小时制和禁用时间；还提供时间范围 `TimeRangePicker` 和可单独嵌入的 `TimePanel`。

## 何时使用

- 需要用户输入一个时刻（提醒时间、营业开始时间）时。
- 需要同时选择日期和时间时，用 [DatePicker](/components/datepicker) 中的 `DateTimePicker`；选择一段日期用 `RangePicker`。
- 只能从固定间隔的几个时间点中选择（例如每 30 分钟一个档位）时，用下拉列表风格的 [TimeSelect](/components/catalog/timeselect) 更直接。

## 在线体验

<GalleryPreview component="TimePicker" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
	"time"

	"github.com/tangoui/tango/ui"
)

var remindAt time.Time

func page(c *ui.Context) {
	ui.TimePicker(c, &remindAt).Placeholder("请选择时间")
	if !remindAt.IsZero() {
		ui.Text(c, "提醒时间："+remindAt.Format("15:04:05"))
	}
}
```

值绑定到 `*time.Time`，零值表示未选择；新选出的值的日期部分为当天。既可以在面板里滚动选择，也可以直接在输入框中键入时间。`Changed()` 报告本帧是否提交了新值。

## 常见用法

### 格式与步长

`Format` 使用 Go 时间布局决定显示哪些列：`"15:04:05"`（默认）、`"15:04"`（隐藏秒列）。`HourStep`、`MinuteStep`、`SecondStep` 或一次性的 `Steps(时, 分, 秒)` 设置每列的步长。

```go
import (
	"time"

	"github.com/tangoui/tango/ui"
)

var open, slot time.Time

func page(c *ui.Context) {
	ui.TimePicker(c, &open).Format("15:04").MinuteStep(15)
	ui.TimePicker(c, &slot).Steps(1, 5, 10)
}
```

### 12 小时制

`Use12Hours()` 显示 12 小时制：小时列为 12、01 … 11，并增加上午 / 下午列；默认格式变为 `"03:04:05 PM"`，上午 / 下午的文字取自当前语言包。

```go
import (
	"time"

	"github.com/tangoui/tango/ui"
)

var alarm time.Time

func page(c *ui.Context) {
	ui.TimePicker(c, &alarm).Use12Hours()
	ui.TimePicker(c, &alarm).Use12Hours().Format("03:04 PM")
}
```

### 禁用时间

`DisabledHours`、`DisabledMinutes`、`DisabledSeconds` 标记不可选择也不可键入的时间（小时为 0–23）。`HideDisabledOptions(true)` 直接从列中隐藏被禁用的项，而不是置灰。

```go
import (
	"time"

	"github.com/tangoui/tango/ui"
)

var appointment time.Time

func page(c *ui.Context) {
	ui.TimePicker(c, &appointment).Format("15:04").
		DisabledHours(func(h int) bool { return h < 9 || h >= 18 }).
		DisabledMinutes(func(h, m int) bool { return h == 12 }). // 午休
		HideDisabledOptions(true)
}
```

### 时间范围 TimeRangePicker

`TimeRangePicker` 绑定 `*[2]time.Time`，开始和结束各有一组时间列；步长、12 小时制和禁用时间的用法与 `TimePicker` 相同，`Placeholder(开始, 结束)` 设置两端占位符。

```go
import (
	"time"

	"github.com/tangoui/tango/ui"
)

var businessHours [2]time.Time

func page(c *ui.Context) {
	ui.TimeRangePicker(c, &businessHours).Format("15:04").MinuteStep(15).
		Placeholder("开门时间", "打烊时间").
		DisabledHours(func(h int) bool { return h < 6 })
}
```

### 内嵌时间面板 TimePanel

`TimePanel` 是 TimePicker 的下拉面板本身，可以直接放在页面或自定义弹层里。面板可聚焦：↑ / ↓ 改变当前列的值，← / → 切换列。

```go
import (
	"time"

	"github.com/tangoui/tango/ui"
)

var at time.Time

func page(c *ui.Context) {
	ui.TimePanel(c, &at).Format("15:04").Steps(1, 15, 1).Height(200)
	ui.Text(c, at.Format("15:04"))
}
```

### 尺寸、状态与外观

`Size(ui.Small | ui.Large)`、`Status("error" | "warning")`、`Variant("filled" | "borderless" | "underlined")`、`Width`、`AllowClear(false)`、`Disabled(true)` 调整外观与交互，用法与其他输入类组件一致。

```go
import (
	"time"

	"github.com/tangoui/tango/ui"
)

var t time.Time

func page(c *ui.Context) {
	ui.TimePicker(c, &t).Size(ui.Small)
	ui.TimePicker(c, &t).Status("error").Placeholder("错误状态")
	ui.TimePicker(c, &t).Variant("filled").AllowClear(false).Width(160)
}
```

## 主题与样式

- 输入框外观沿用输入类组件的令牌；下拉面板的列宽、单元格高度和选中色与日期面板共用 `theme.Components.DatePicker`（`DatePickerToken`）中的 `TimeColumnWidth`、`TimeColumnHeight`、`TimeCellHeight`、`TimeCellHoverBg`、`TimeCellActiveBg` 等，三套风格切换无需改代码。
- `ConfigProvider.Defaults("TimePicker", ui.Props{...})` 支持 `format`、`placeholder`、`use12Hours`、`hourStep`、`minuteStep`、`secondStep`、`allowClear`、`width`、`size`、`variant`、`hideDisabledOptions`；其中格式、12 小时制、步长和 `hideDisabledOptions` 对 `TimePanel` 同样生效。`TimeRangePicker` 使用 `"TimeRangePicker"` 键。

## 注意事项

- **日期部分**：TimePicker 只关心时分秒，新值的日期为当天。保存或比较时只取 `Hour()`、`Minute()`、`Second()`，不要依赖日期部分。
- **状态**：值就是你绑定的 `time.Time`；下拉是否展开、正在键入的文本由组件按调用位置保存，循环渲染时用 `ui.Key` 区分。
- **表单校验**：放进 [Form](/components/form) 时，用 `RulesFunc(func() string { ... })` 把时间格式化成字符串再配合 `ui.Required`，或用 `ui.RulesOf` 配合 `ui.CheckValue` 直接检查 `time.Time`。
- **无障碍**：在表单项里自动以表单项标签命名，否则依次使用 `A11yLabel`、占位符；`TimePanel` 默认名称取自语言包的“选择时间”。

## 示例代码

<!--@include: ./_gen/timepicker-examples.md-->

## API

<!--@include: ./_gen/timepicker-api.md-->

`TimeRangePicker` 的方法见 [Go API 参考](/reference/api/ui#TimeRangePicker)，与 `TimePicker` 同名方法含义一致。

## 相关链接

- [源码 ui/entry_time.go](https://github.com/tangoui/tango/blob/main/ui/entry_time.go)、[ui/p2a_time_range.go](https://github.com/tangoui/tango/blob/main/ui/p2a_time_range.go)
- [ui Go API](/reference/api/ui)
- [TimeRangePicker 时间范围](/components/catalog/timerangepicker)、[TimeSelect](/components/catalog/timeselect)
- [DatePicker 日期选择框](/components/datepicker)、[Form 表单](/components/form)
