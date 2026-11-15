---
title: Pie 饼图与环形图
---
# Pie 饼图与环形图

用扇区角度表示各分类占整体的比例，`Donut()` 在中心留下统计区域。

## 何时使用

- 分类数量较少且重点是各部分占整体的比例。
- 需要在环形图中心放总量、当前筛选结果等摘要时。
- 分类较多、需要精确比较绝对值时不要使用饼图，改用 [Column 柱状图](/components/chart-column)；只有一个分类时改用 [Progress 进度条](/components/progress)。

## 在线体验

Gallery 展示带引导线的饼图、中心统计环形图、内部百分比和环形图标签。

<GalleryPreview component="chart.Pie" />

## 基本用法

`Angle()` 指定数值字段，`Color()` 指定扇区名称字段；相同名称的行会自动求和。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

var channelRows = []chart.Row{
    {"type": "搜索引擎", "value": 1048},
    {"type": "直接访问", "value": 735},
    {"type": "邮件营销", "value": 580},
}

func page(c *ui.Context) {
    chart.Pie(c, channelRows).Angle("value").Color("type").Show()
}
```

## 常见用法

### 环形图和中心统计

`Donut()` 等价于带默认内圈的 `Pie()`；`Center()` 覆盖中心的标题和值，适合把业务摘要与扇区占比放在同一张图里。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func donutSummary(c *ui.Context) {
    rows := []chart.Row{{"type": "新客", "value": 420}, {"type": "老客", "value": 680}}
    chart.Donut(c, rows).Angle("value").Color("type").Center("访问量", "1,100").Show()
}
```

### 标签和内部百分比

默认会在外侧绘制分类标签和引导线；`NoLabel()` 隐藏它们，`InnerLabel()` 把百分比放入足够大的扇区内部。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func pieLabels(c *ui.Context) {
    rows := []chart.Row{{"type": "A", "value": 50}, {"type": "B", "value": 30}, {"type": "C", "value": 20}}
    chart.Pie(c, rows).Angle("value").Color("type").InnerLabel().Height(280).Show()
}
```

### 隐藏图例与格式化

`Legend(chart.LegendNone)` 隐藏图例，适用于标签已经完整表达分类的场景；`Format()` 控制悬停值和中心总量的格式。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func compactPie(c *ui.Context) {
    rows := []chart.Row{{"type": "搜索引擎", "value": 104800}, {"type": "直接访问", "value": 73500}}
    chart.Pie(c, rows).Angle("value").Color("type").Legend(chart.LegendNone).Format(chart.Thousands(0)).Show()
}
```

### 镜像和交互

RTL 场景默认只镜像外围文字和图例；需要扇区逆时针绘制时显式调用 `Mirror()`。悬停提示默认开启，需要静态展示时可用 `NoTooltip()`。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func mirroredPie(c *ui.Context) {
    rows := []chart.Row{{"type": "已完成", "value": 72}, {"type": "处理中", "value": 28}}
    chart.Donut(c, rows).Angle("value").Color("type").Mirror().NoTooltip().Show()
}
```

## 主题与样式

Pie 使用主题 chart style 的分类色板、扇区分隔线和提示卡片。可用 `Colors(theme.Hex("#...")...)` 覆盖当前图表色板，或用 `Style(func(*chart.Style) { ... })` 调整局部令牌；不要通过颜色顺序表达动态排名，类别名称应同时出现在标签、图例或提示中。

## 注意事项

- 饼图至少需要两个有意义的分类；分类过多时合并为“其他”或改用柱状图，避免引导线和文字拥挤。
- 数据行中的 `Angle()` 值必须为正数才会绘制；相同 `Color()` 值会累加，空数据会进入组件空状态。
- 图表状态由即时模式按组件位置保留；条件增删图表时给它稳定的 `Key()`，筛选后仍应保持分类颜色稳定。
- `A11yLabel()` 设置图表可访问名称，图例或标签提供非颜色的分类信息；`LegendNone` 只在外侧标签足够清晰时使用。

## 示例代码

<!--@include: ./_gen/chart-pie-examples.md-->

## API

<!--@include: ./_gen/chart-pie-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/chart/pie.go)
- [ui Go API](/reference/api/ui)
- [Column 柱状图](/components/chart-column)
- [Statistic 统计数值](/components/statistic)
