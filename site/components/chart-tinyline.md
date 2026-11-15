---
title: TinyLine 迷你图
---
# TinyLine 迷你图

在统计卡片等紧凑区域内展示没有坐标轴和图例的趋势线、面积、迷你柱或进度条。

## 何时使用

- 在 Statistic、ProCard 或摘要卡片中补充短期趋势。
- 需要用很小的空间表达方向，不需要读精确坐标时。
- 需要完整坐标、图例和多系列比较时改用 [Line 折线图](/components/chart-line) 或 [Column 柱状图](/components/chart-column)。

## 在线体验

Gallery 展示 TinyLine、TinyArea、TinyColumn、TinyProgress，以及空状态和加载态组合。

<GalleryPreview component="chart.TinyLine" />

## 基本用法

`TinyLine()` 接收 `[]float64`，不绘制坐标轴或图例；`Smooth()` 让曲线更平滑。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

var visits = []float64{264, 417, 438, 887, 309, 397, 550, 575}

func page(c *ui.Context) {
    chart.TinyLine(c, visits).Smooth().Show()
}
```

## 常见用法

### 面积和迷你柱

`TinyArea()` 填充趋势线下方区域，`TinyColumn()` 用紧凑柱形显示各点；它们都共享同一组数值输入。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func tinyVariants(c *ui.Context) {
    vals := []float64{264, 417, 438, 887, 309, 397, 550, 575}
    chart.TinyArea(c, vals).Smooth().Show()
    chart.TinyColumn(c, vals).Show()
}
```

### Tooltip 标签

`Names()` 为每个点提供悬停标签，例如日期；标签数量可以少于数据点，未提供的点不显示名称。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func namedTinyLine(c *ui.Context) {
    vals := []float64{12, 18, 15}
    chart.TinyLine(c, vals).Names("周一", "周二", "周三").Show()
}
```

### 进度和目标线

`TinyProgress()` 接收 0 到 1 的比例；`Target()` 添加目标标记线，适合卡片中的完成率和目标对比。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func tinyProgress(c *ui.Context) {
    chart.TinyProgress(c, 0.78).Target(0.85).A11yLabel("完成率 78%，目标 85%").Show()
}
```

### 颜色和状态

`Color()` 只覆盖当前迷你图的标记颜色；如果数据表示状态，仍要在卡片文字或图标中写出状态。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
    "github.com/tangoui/tango/ui/theme"
)

func coloredTiny(c *ui.Context) {
    chart.TinyLine(c, []float64{8, 10, 9, 13}).Color(theme.Hex("#1677ff")).NoTooltip().Show()
}
```

## 主题与样式

Tiny 图表复用当前主题的首个系列色、轨道色和 tooltip；`Color()` 适合局部强调，`Colors()` 或 `Style(func(*chart.Style) { ... })` 可用于更系统的单图覆盖。它没有坐标轴和图例，颜色不能承载唯一语义。

## 注意事项

- Tiny 图表刻意省略坐标轴，精确数值应放在卡片标题、统计值、Tooltip 或可展开的完整图表中。
- `TinyProgress()` 和 `Target()` 使用 0 到 1 的比例，不要传入 78 这样的百分数；超出范围的绘制会被限制，但业务值仍应在外部校验。
- 即时模式下数组放在外部状态；更新时替换数组或更新状态后重绘，条件切换多个卡片时使用 `Key()`。
- `Names()` 的顺序必须与数值顺序一致；为动态数据设置 `A11yLabel()`，并提供文字化趋势或状态说明。

## 示例代码

<!--@include: ./_gen/chart-tinyline-examples.md-->

## API

<!--@include: ./_gen/chart-tinyline-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/chart/tiny.go)
- [ui Go API](/reference/api/ui)
- [Statistic 统计数值](/components/statistic)
- [Line 折线图](/components/chart-line)
