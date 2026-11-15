---
title: Calendar 日历
---
# Calendar 日历

按月或年展示日期，并允许在日期格中呈现日程数据。

## 何时使用

- 排班、日程、预约和按日期查看数据的页面。
- 只需要在表单中选日期时使用 [DatePicker 日期选择框](/components/datepicker)。

## 在线体验

<GalleryPreview component="Calendar" />

## 基本用法

```go
import (
    "time"
    "github.com/tangoui/tango/ui"
)

var selectedDay time.Time

func page(c *ui.Context) {
    ui.Calendar(c, &selectedDay)
}
```

`selectedDay` 为零值时显示今天且无选中日期。

## 常见用法

### 卡片模式与禁用日期

```go
func compact(c *ui.Context, day *time.Time) {
    ui.Calendar(c, day).Card().DisabledDate(func(d time.Time) bool {
        return d.Weekday() == time.Saturday || d.Weekday() == time.Sunday
    })
}
```

### 日期格内容

```go
func schedule(c *ui.Context, day *time.Time) {
    ui.Calendar(c, day).CellRender(func(d time.Time) {
        if d.Day() == 15 { ui.Badge(c).Status("success", "评审") }
    })
}
```

### 年/月切换

```go
var calendarMode = "month"

func modes(c *ui.Context, day *time.Time) {
    ui.Calendar(c, day).Mode(&calendarMode)
}
```

## 主题与样式

Calendar 令牌控制日期格高度、今天、选中、禁用和头部样式。`CellHeight`、`Range`、`NoHeader` 可调整全屏日历形态。

## 注意事项

选中值和视图模式要保存在页面状态；日期格渲染函数每帧调用，应只读取已准备好的日程数据。键盘方向键可在日期间移动，禁用规则要与业务校验一致。

## 示例代码

<!--@include: ./_gen/calendar-examples.md-->

## API

<!--@include: ./_gen/calendar-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/data_calendar.go)
- [DatePicker 日期选择框](/components/datepicker)
