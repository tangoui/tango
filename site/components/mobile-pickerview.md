---
title: Mobile PickerView 滚轮选择器
---
# Mobile PickerView 滚轮选择器

以内联滚轮展示单列、多列、级联或日期选择，并把当前选择写回调用方值。

## 何时使用

- 需要把选择器直接嵌入页面时使用 `PickerView`、`CascadePickerView` 或 `DatePickerView`。
- 需要底部弹出、确认后再提交值时使用 `Picker`。
- 选项很多且不适合滚轮时使用 List、Select 或搜索型输入，不要强行压缩成滚轮。
- 日期业务应使用 `DatePickerView` 的 `DatePrecision` 和边界，而不是手写日期字符串滚轮。

## 在线体验

真实 gallery 展示级联、多列和日期滚轮，拖动各列观察绑定值变化。

<GalleryPreview component="mobile.PickerView" />

## 基本用法

`PickerView` 接收每列的 `ui.Option`，`value` 是每列当前选项值的切片。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/mobile"
)

var selected = []string{"周三", "下午"}

func page(c *ui.Context) {
    mobile.PickerView(c, [][]ui.Option{
        ui.Opts("周一", "周二", "周三", "周四", "周五"),
        ui.Opts("上午", "下午", "晚上"),
    }, &selected).Visible(5).Show()
}
```

## 常见用法

### 级联选择

`CascaderOption` 用 `Children` 描述层级，绑定值按当前路径保存。

```go
var area = []string{"zj", "hz", "bj"}

func cascade(c *ui.Context) {
    options := []ui.CascaderOption{
        {Label: "浙江", Value: "zj", Children: []ui.CascaderOption{
            {Label: "杭州", Value: "hz", Children: []ui.CascaderOption{
                {Label: "西湖区", Value: "xh"},
            }},
        }},
    }
    mobile.CascadePickerView(c, options, &area).Show()
}
```

### 日期选择

`DatePickerView` 直接绑定 `time.Time`，精度从年到分钟由 `DatePrecision` 指定。

```go
import "time"

var date = time.Date(2026, 11, 12, 9, 30, 0, 0, time.UTC)

func datePicker(c *ui.Context) {
    mobile.DatePickerView(c, &date, mobile.PrecisionDay).Visible(5).Show()
}
```

### 弹出确认式选择

`Picker` 是带工具栏的底部 Popup；绑定值只有在确认后才更新。

```go
var pickerOpen bool
var week = []string{"周三", "下午"}

func confirmed(c *ui.Context) {
    p := mobile.Picker(c, &pickerOpen, "选择时间").Columns([][]ui.Option{
        ui.Opts("周一", "周二", "周三"), ui.Opts("上午", "下午"),
    }, &week).Show()
    if p.Confirmed() {
        // week 已经是确认后的值
    }
    if p.Cancelled() {
        // 保留原值
    }
}
```

### 处理变更和无障碍

`Changed` 只在当前帧有新选择时为真；可为整列或每列设置标签。

```go
func accessible(c *ui.Context) {
    w := mobile.PickerView(c, [][]ui.Option{ui.Opts("A", "B")}, &selected).
        A11yLabel("工作日").A11yLabels("日期").Show()
    if w.Changed() {
        // 根据 selected 更新预览
    }
}
```

## 注意事项

选择值切片和日期指针要放在稳定状态中；`PickerView` 的值会在滚轮变化时更新，而 `Picker` 的值在确认前保持原值。级联路径中的 Value 必须唯一且与选项树一致。滚轮可见项用 `Visible` 控制，选项很多时应限制数量或改用搜索。为多列提供有意义的 `A11yLabels`，确保键盘和屏幕阅读器能区分列。

## 示例代码

<!--@include: ./_gen/mobile-pickerview-examples.md-->

## API

<!--@include: ./_gen/mobile-pickerview-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/mobile/picker.go)
- [Popup 弹出层](/components/mobile-popup)
- [Scroller 滚动物理](/components/mobile-scroller)
- [ui mobile Go API](/reference/api/ui-mobile)
