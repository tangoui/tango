---
title: Line 折线图
---
# Line 折线图

用连续的线展示一个或多个指标随分类或时间变化的趋势。

## 何时使用

- 比较月度、日度等有顺序数据的变化趋势。
- 需要同时观察多个系列，并通过图例切换系列时。
- 只有一个离散数值时不要使用折线图，改用 [Statistic 统计数值](/components/statistic)；少量阶段进度可用 [Steps 步骤条](/components/steps)。

## 在线体验

Gallery 包含基础、平滑、阶梯、面积、堆叠面积和大数据降采样场景，可切换 Ant Design、Element Plus 及暗色变体。

<GalleryPreview component="chart.Line" />

## 基本用法

下面示例把行数据映射为横轴 `month`、数值轴 `value`，并用 `kind` 拆分系列；`Show()` 返回可读取悬停状态的 `chart.View`。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

var trendRows = []chart.Row{
    {"month": "1月", "kind": "本期", "value": 120},
    {"month": "2月", "kind": "本期", "value": 168},
    {"month": "1月", "kind": "上期", "value": 96},
    {"month": "2月", "kind": "上期", "value": 142},
}

func page(c *ui.Context) {
    chart.Line(c, trendRows).X("month").Y("value").Series("kind").Show()
}
```

## 常见用法

### 平滑曲线和阶梯线

`Smooth()` 使用平滑曲线，`Step()` 使用阶梯线；两者都保留同样的字段映射和悬停提示。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func smoothAndStep(c *ui.Context) {
    rows := []chart.Row{{"month": "1月", "value": 12}, {"month": "2月", "value": 18}, {"month": "3月", "value": 15}}
    chart.Line(c, rows).X("month").Y("value").Smooth().Show()
    chart.Line(c, rows).X("month").Y("value").Step().Show()
}
```

### 面积和堆叠面积

用 `Area()` 填充线下区域；多个系列需要叠加时同时调用 `Series()` 和 `Stack()`。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func areaTrend(c *ui.Context) {
    rows := []chart.Row{
        {"month": "1月", "channel": "直接访问", "value": 32},
        {"month": "1月", "channel": "搜索引擎", "value": 58},
        {"month": "2月", "channel": "直接访问", "value": 40},
        {"month": "2月", "channel": "搜索引擎", "value": 64},
    }
    chart.Area(c, rows).X("month").Y("value").Series("channel").Stack().Smooth().Show()
}
```

### 时间轴和格式化

`X()` 可以指向 `time.Time` 字段，组件会使用时间轴；`Format()` 同时影响数值轴和悬停值。

```go
package main

import (
    "time"

    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func timeSeries(c *ui.Context) {
    rows := []chart.Row{
        {"date": time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC), "pv": 3200},
        {"date": time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC), "pv": 4100},
    }
    chart.Line(c, rows).X("date").Y("pv").Format(chart.Thousands(0)).Height(280).Show()
}
```

### 固定坐标范围和数据点

`YMin()` / `YMax()` 固定数值轴边界，`Points()` 显示每个点；若要隐藏悬停十字线和提示，可使用 `NoTooltip()`。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func fixedTrend(c *ui.Context) {
    rows := []chart.Row{{"month": "1月", "value": 72}, {"month": "2月", "value": 86}, {"month": "3月", "value": 78}}
    chart.Line(c, rows).X("month").Y("value").YMin(0).YMax(100).Points().Show()
}
```

## 主题与样式

图表样式从当前 `c.Theme` 派生：Ant Design 使用 G2 风格，Element Plus 使用 ECharts 风格，shadcn 使用对应的中性表面和调色板。单个图表可用 `Colors(theme.Hex(...))` 覆盖系列色，或用 `Style(func(*chart.Style) { ... })` 调整该图表的令牌；优先保持同一页面的系列颜色顺序稳定。

## 注意事项

- 即时模式下数据放在页面函数外的稳定变量中；数据替换后下一帧会重绘。条件渲染中增删图表时，用 `Key()` 保持图例和动画状态对应同一张图。
- 多系列图表保留图例，不要只用颜色表达系列含义；可用 `A11yLabel("月度访问趋势")` 设置可访问名称，并确保数值轴语义明确。
- 长折线会按绘图区宽度使用 LTTB 降采样；如果需要逐点查看，拆成更小的范围或改用表格。
- RTL 作用域默认保持数据可视化的从左到右绘图区；只有确实需要镜像绘图区时才调用 `Mirror()`。

## 示例代码

<!--@include: ./_gen/chart-line-examples.md-->

## API

<!--@include: ./_gen/chart-line-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/chart/line.go)
- [ui Go API](/reference/api/ui)
- [Column 柱状图](/components/chart-column)
- [TinyLine 迷你折线图](/components/chart-tinyline)
