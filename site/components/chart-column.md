---
title: Column / Bar 柱状图与条形图
---
# Column / Bar 柱状图与条形图

用柱形长度比较分类数据，支持纵向柱状图、横向条形图、分组和堆叠。

## 何时使用

- 比较不同分类的数量、金额或排名。
- 需要在同一分类内比较多个系列，或观察组成结构时。
- 分类非常多、名称较长时优先用 `Bar()` 横向条形图；连续趋势优先用 [Line 折线图](/components/chart-line)。

## 在线体验

Gallery 展示基础、分组、堆叠、百分比堆叠、排行条形图和堆叠条形图。

<GalleryPreview component="chart.Column" />

## 基本用法

`Column()` 创建纵向柱状图，`X()` 指定分类字段，`Y()` 指定数值字段；不设置 `Series()` 时每个分类只有一根柱。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

var salesRows = []chart.Row{
    {"month": "1月", "sales": 320},
    {"month": "2月", "sales": 460},
    {"month": "3月", "sales": 390},
}

func page(c *ui.Context) {
    chart.Column(c, salesRows).X("month").Y("sales").Show()
}
```

## 常见用法

### 分组柱状图

设置 `Series()` 后，同一分类的不同系列会并排显示，图例会提供系列切换。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func groupedColumns(c *ui.Context) {
    rows := []chart.Row{
        {"month": "1月", "channel": "直接访问", "value": 32},
        {"month": "1月", "channel": "搜索引擎", "value": 58},
        {"month": "2月", "channel": "直接访问", "value": 40},
        {"month": "2月", "channel": "搜索引擎", "value": 64},
    }
    chart.Column(c, rows).X("month").Y("value").Series("channel").Show()
}
```

### 堆叠和百分比堆叠

`Stack()` 保留原始数值并叠加系列，`Percent()` 将每个分类归一化为 100%；百分比堆叠可配合 `Label()` 显示段内百分比。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func stackedColumns(c *ui.Context) {
    rows := []chart.Row{
        {"month": "1月", "channel": "直接访问", "value": 32},
        {"month": "1月", "channel": "搜索引擎", "value": 58},
        {"month": "2月", "channel": "直接访问", "value": 40},
        {"month": "2月", "channel": "搜索引擎", "value": 64},
    }
    chart.Column(c, rows).X("month").Y("value").Series("channel").Percent().Label().Show()
}
```

### 排行条形图

`Bar()` 将分类轴转为横向，适合长名称和排行；`MaxWidth()` 限制条厚度，`Radius()` 设置条端圆角。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func rankingBars(c *ui.Context) {
    rows := []chart.Row{
        {"shop": "工专路 0 号店", "sales": 323234},
        {"shop": "工专路 1 号店", "sales": 293200},
        {"shop": "工专路 2 号店", "sales": 251500},
    }
    chart.Bar(c, rows).X("shop").Y("sales").Format(chart.Thousands(0)).Label().MaxWidth(18).Radius(2).Show()
}
```

### 加载态和固定高度

数据尚未到达时使用 `Loading(true)`，不要用空数组伪装成零值；`Height()` 设置绘图区高度，宽度仍由父容器决定。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func loadingColumns(c *ui.Context) {
    rows := []chart.Row{{"month": "1月", "value": 32}, {"month": "2月", "value": 58}}
    chart.Column(c, rows).X("month").Y("value").Loading(true).Height(280).Show()
}
```

## 主题与样式

柱子的颜色、圆角、间距、轴线和悬停底色由当前主题的 chart style 控制。可使用 `Colors()` 做局部系列色覆盖，使用 `Style(func(*chart.Style) { ... })` 调整单张图表；Ant Design、Element Plus 和 shadcn 的默认柱宽、图例位置和表面色会随主题适配。

## 注意事项

- 柱状图默认包含零基线，避免截断造成长度比较失真；只有明确需要突出小幅差异时才使用 `NoZero()`，并在页面说明轴范围。
- `Series()` 的值决定图例和颜色，筛选数据时不要让同一系列换颜色；大批量排行可优先使用横向 `Bar()`。
- 即时模式中的行数据应由外部状态持有；当条件渲染多个图表时使用不同的 `Key()`，避免动画和图例状态串图。
- `A11yLabel()` 为图表提供语义名称，数值标签和图例不要只依赖颜色；RTL 需要镜像绘图区时再使用 `Mirror()`。

## 示例代码

<!--@include: ./_gen/chart-column-examples.md-->

## API

<!--@include: ./_gen/chart-column-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/chart/column.go)
- [ui Go API](/reference/api/ui)
- [Line 折线图](/components/chart-line)
- [Table 表格](/components/table)
