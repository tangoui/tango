---
title: Gauge 仪表盘
---
# Gauge 仪表盘

用弧形刻度展示一个处于范围内的当前值，适合完成率、健康度等单一指标。

## 何时使用

- 需要突出一个当前值及其上下限或完成比例时。
- 需要用分段阈值表达正常、预警和危险区间时。
- 需要同时比较多个指标时不要使用仪表盘，改用 [Column 柱状图](/components/chart-column) 或 [Radar 雷达图](/components/chart-radar)。

## 在线体验

Gallery 展示 0 到 1 的完成率仪表盘，以及 0 到 100 的分段健康度仪表盘。

<GalleryPreview component="chart.Gauge" />

## 基本用法

默认范围是 0 到 1，视觉和提示会按百分比格式显示；`Title()` 为数值添加语义标题。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func page(c *ui.Context) {
    chart.Gauge(c, 0.72).Title("完成率").Show()
}
```

## 常见用法

### 自定义数值范围

`Range(lo, hi)` 设置刻度范围；范围不是 0 到 1 时默认按普通数字格式显示。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func healthGauge(c *ui.Context) {
    chart.Gauge(c, 68).Range(0, 100).Title("健康度").Show()
}
```

### 分段阈值

`Bands()` 接收按升序排列的上限和对应颜色；未提供颜色的段会使用主题分类色板。业务状态仍应配合文字说明。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
    "github.com/tangoui/tango/ui/theme"
)

func thresholdGauge(c *ui.Context) {
    chart.Gauge(c, 68).Range(0, 100).Title("健康度").Bands(
        []float64{40, 70, 100},
        theme.Hex("#ff4d4f"), theme.Hex("#faad14"), theme.Hex("#52c41a"),
    ).Show()
}
```

### 格式化、尺寸和无障碍

`Format()` 控制刻度、中心值和提示的文本；`Height()` 调整绘图区，`A11yLabel()` 提供视觉标题之外的可访问名称。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func formattedGauge(c *ui.Context) {
    chart.Gauge(c, 6850).Range(0, 10000).Title("请求数").Format(chart.Thousands(0)).Height(280).A11yLabel("请求数当前为 6850").Show()
}
```

### 静态和加载状态

悬停提示默认开启；静态大屏可用 `NoTooltip()`，异步值尚未到达时可用 `Loading(true)`。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func staticGauge(c *ui.Context) {
    chart.Gauge(c, 0.45).Title("容量").NoTooltip().Loading(false).Show()
}
```

## 主题与样式

Gauge 使用主题的轨道、刻度、指针和分类色板。可用 `Colors()` 覆盖进度弧颜色，或用 `Style(func(*chart.Style) { ... })` 调整单图令牌；状态色应和图例、文字或阈值标签一起使用，不要只依赖红黄绿。

## 注意事项

- 传入值会按 `Range()` 计算进度并限制到范围内；业务层应保留原始值，避免把异常值静默当成满格展示。
- `Bands()` 的 stops 必须是范围内按升序排列的上限；超过最后一个 stop 的区域不应被误解为另一种状态。
- 即时模式下外部状态保存原始值，更新时下一个渲染帧会产生过渡；条件切换多个仪表盘时使用稳定 `Key()`。
- 指针、颜色和刻度之外，用 `Title()` 和 `A11yLabel()` 清楚表达指标名称与单位；RTL 需要反向弧线时才调用 `Mirror()`。

## 示例代码

<!--@include: ./_gen/chart-gauge-examples.md-->

## API

<!--@include: ./_gen/chart-gauge-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/chart/gauge.go)
- [ui Go API](/reference/api/ui)
- [Liquid 水波图](/components/chart-liquid)
- [Progress 进度条](/components/progress)
