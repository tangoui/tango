---
title: Heatmap 热力图
---
# Heatmap 热力图

用二维分类网格和连续色阶展示每个交叉位置的数值强弱。

## 何时使用

- 查看星期与小时、日期与指标等二维维度的密度和峰值。
- 数据点较多但需要快速发现热点、空洞和周期性时。
- 需要精确逐行比较时使用 [Table 表格](/components/table)，只有一个维度的趋势使用 [Line 折线图](/components/chart-line)。

## 在线体验

Gallery 展示一周 24 小时访问量热力图，单元格支持标签和悬停提示。

<GalleryPreview component="chart.Heatmap" />

## 基本用法

`X()` 和 `Y()` 指定两个分类字段，`Value()` 指定映射到连续色阶的数值字段。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

var heatRows = []chart.Row{
    {"hour": "09:00", "day": "周一", "count": 32},
    {"hour": "12:00", "day": "周一", "count": 86},
    {"hour": "09:00", "day": "周二", "count": 48},
    {"hour": "12:00", "day": "周二", "count": 92},
}

func page(c *ui.Context) {
    chart.Heatmap(c, heatRows).X("hour").Y("day").Value("count").Show()
}
```

## 常见用法

### 单元格标签和尺寸

`Label()` 在单元格足够宽高时显示格式化数值，`Height()` 给行列网格分配稳定高度。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func labeledHeatmap(c *ui.Context) {
    rows := []chart.Row{{"hour": "09:00", "day": "周一", "count": 32}, {"hour": "12:00", "day": "周一", "count": 86}}
    chart.Heatmap(c, rows).X("hour").Y("day").Value("count").Label().Format(chart.Thousands(0)).Height(340).Show()
}
```

### 自定义连续色阶

`Ramp()` 接收从低到高的 `chart.Color`；使用单色连续阶梯表达量级，不要把连续值映射成彩虹色。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
    "github.com/tangoui/tango/ui/theme"
)

func rampHeatmap(c *ui.Context) {
    rows := []chart.Row{{"x": "A", "y": "一", "v": 10}, {"x": "B", "y": "一", "v": 80}}
    chart.Heatmap(c, rows).X("x").Y("y").Value("v").Ramp(
        theme.Hex("#e6f4ff"), theme.Hex("#91caff"), theme.Hex("#1677ff"),
    ).Show()
}
```

### 交互和空值

悬停单元格会显示分类组合和值；没有数值的交叉位置保持空白，加载阶段可用 `Loading(true)`，静态导出可用 `NoTooltip()`。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func heatmapState(c *ui.Context) {
    rows := []chart.Row{{"x": "A", "y": "一", "v": 10}, {"x": "B", "y": "一", "v": 20}}
    chart.Heatmap(c, rows).X("x").Y("y").Value("v").A11yLabel("按分类统计的热力图").Loading(false).Show()
}
```

## 主题与样式

Heatmap 默认使用当前 chart style 的 `Sequential` 单色连续色阶和数值图例。可用 `Ramp()` 只为单图替换色阶，`Colors()` 不替代连续色阶；Ant Design、Element Plus 和 shadcn 的背景、文字和悬停边框会随主题适配。

## 注意事项

- `X()` / `Y()` 的分类顺序来自数据首次出现的顺序；需要固定星期或小时顺序时，先按期望顺序构造数据。
- 缺失交叉位置不是零值，不要用零填充来掩盖“没有数据”和“数值为零”的差异。
- 连续色阶必须同时配数值图例，颜色深浅之外还应提供标签或表格；大量网格可先聚合，避免标签拥挤。
- 即时模式下替换外部数据切片来更新图表；用 `Key()` 区分不同指标热力图，RTL 需要反转绘图区时再用 `Mirror()`。

## 示例代码

<!--@include: ./_gen/chart-heatmap-examples.md-->

## API

<!--@include: ./_gen/chart-heatmap-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/chart/heatmap.go)
- [ui Go API](/reference/api/ui)
- [Table 表格](/components/table)
- [Line 折线图](/components/chart-line)
