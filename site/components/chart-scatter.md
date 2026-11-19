---
title: Scatter 散点图与气泡图
---
# Scatter 散点图与气泡图

用点的位置展示两个数值字段的关系，也可用气泡大小编码第三个数值。

## 何时使用

- 分析两个连续数值之间的相关性、分布和离群点。
- 需要按类别着色，或用第三个数值表达规模时。
- 类别轴上的少量比较不要用散点图，改用 [Column 柱状图](/components/chart-column)；只有两个变量的趋势可用 [Line 折线图](/components/chart-line)。

## 在线体验

Gallery 展示按性别分系列的身高体重散点图，以及用 GDP、预期寿命和人口绘制的气泡图。

<GalleryPreview component="chart.Scatter" />

## 基本用法

`X()` 和 `Y()` 都应指向数值字段；`Series()` 按分类拆分颜色和图例。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

var points = []chart.Row{
    {"group": "A", "height": 160.2, "weight": 54.1},
    {"group": "A", "height": 164.5, "weight": 58.3},
    {"group": "B", "height": 174.0, "weight": 70.2},
}

func page(c *ui.Context) {
    chart.Scatter(c, points).X("height").Y("weight").Series("group").Show()
}
```

## 常见用法

### 气泡图

`Bubble()` 在散点基础上用 `Size()` 映射数值字段；`Radius(min, max)` 限制气泡半径，避免极端值压过其他点。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func bubbles(c *ui.Context) {
    rows := []chart.Row{
        {"country": "中国", "gdp": 17.9, "life": 78.2, "pop": 141},
        {"country": "美国", "gdp": 25.4, "life": 76.4, "pop": 33},
        {"country": "日本", "gdp": 4.2, "life": 84.5, "pop": 12.5},
    }
    chart.Bubble(c, rows).X("gdp").Y("life").Size("pop").Series("country").Legend(chart.LegendNone).Radius(6, 30).Show()
}
```

### 坐标范围和格式

坐标轴默认贴近数据范围，`Zero()` 让两个轴都包含零。`XFormat()` 格式化横轴和对应提示值，`Format()` 格式化纵轴和对应提示值。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func formattedScatter(c *ui.Context) {
    rows := []chart.Row{{"x": 1200, "y": 0.12}, {"x": 2400, "y": 0.18}, {"x": 3600, "y": 0.15}}
    chart.Scatter(c, rows).X("x").Y("y").NoZero().XFormat(chart.Thousands(0)).Format(chart.Percent(0)).Show()
}
```

### 交互和无障碍

散点默认按最近点响应悬停；数据仍在加载时使用 `Loading(true)`，静态摘要可用 `NoTooltip()`，并用 `A11yLabel()` 描述两个轴的含义。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func accessibleScatter(c *ui.Context) {
    rows := []chart.Row{{"x": 1, "y": 2}, {"x": 2, "y": 3}}
    chart.Scatter(c, rows).X("x").Y("y").A11yLabel("样本的 x 与 y 关系").Loading(false).Show()
}
```

## 主题与样式

Scatter 使用主题分类色板、点半径和悬停 halo；Bubble 的半径只表达 `Size()` 字段，系列身份仍由图例和颜色表达。可用 `Colors()` 或 `Style(func(*chart.Style) { ... })` 做单图覆盖，Ant Design、Element Plus 和 shadcn 会分别采用各自的图表表面和色板。

## 注意事项

- `Size()` 应使用可比较的非负数；半径范围只是视觉限制，不要把半径直接当成线性面积解读。
- 点过多时先聚合、抽样或按筛选拆分；系列数较多时保留图例并提供表格或文字说明。
- 数据字段转换和筛选放在页面状态中，图表只在当前帧读取行；条件增删时用稳定的 `Key()` 保留交互状态。
- RTL 默认不反转数据坐标；要让绘图区从右向左时显式使用 `Mirror()`。

## 示例代码

<!--@include: ./_gen/chart-scatter-examples.md-->

## API

<!--@include: ./_gen/chart-scatter-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/chart/scatter.go)
- [ui Go API](/reference/api/ui)
- [Radar 雷达图](/components/chart-radar)
- [Table 表格](/components/table)
