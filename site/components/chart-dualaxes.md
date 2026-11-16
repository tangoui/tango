---
title: DualAxes 双轴图
---
# DualAxes 双轴图

在共享分类横轴上同时绘制左轴柱状系列和右轴折线系列，适合明确不同量纲的对照。

## 何时使用

- 需要在相同月份或阶段上对照数量与比例等不同量纲指标时。
- 柱状系列表达规模、折线系列表达率或趋势时。
- 两个指标可以共用量纲时优先用 [Line 折线图](/components/chart-line) 或 [Column 柱状图](/components/chart-column)，避免双轴造成误读。

## 在线体验

Gallery 展示销售额、订单量和转化率的柱线组合，并包含别名、百分比格式和平滑曲线。

<GalleryPreview component="chart.DualAxes" />

## 基本用法

`Column()` 添加左轴柱状字段，`Line()` 添加右轴折线字段；两者共享 `X()` 分类字段。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

var dualRows = []chart.Row{
    {"month": "1月", "sales": 320, "rate": 0.12},
    {"month": "2月", "sales": 460, "rate": 0.15},
    {"month": "3月", "sales": 390, "rate": 0.13},
}

func page(c *ui.Context) {
    chart.DualAxes(c, dualRows).X("month").Column("sales").Line("rate").LineFormat(chart.Percent(0)).Show()
}
```

## 常见用法

### 多个柱状字段

一次 `Column()` 可以接收多个字段，它们会在左轴上分组显示；每个字段都应使用相同量纲。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func dualColumns(c *ui.Context) {
    rows := []chart.Row{{"month": "1月", "sales": 320, "orders": 180, "rate": 0.12}, {"month": "2月", "sales": 460, "orders": 260, "rate": 0.15}}
    chart.DualAxes(c, rows).X("month").Column("sales", "orders").Line("rate").Show()
}
```

### 别名和格式化

`Alias()` 同时影响图例和提示名称；`LineFormat()` 只格式化右轴和折线值，普通 `Format()` 适用于左轴。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func namedDualAxes(c *ui.Context) {
    rows := []chart.Row{{"month": "1月", "sales": 320, "rate": 0.12}, {"month": "2月", "sales": 460, "rate": 0.15}}
    chart.DualAxes(c, rows).X("month").Column("sales").Line("rate").
        Alias("sales", "销售额").Alias("rate", "转化率").
        Format(chart.Thousands(0)).LineFormat(chart.Percent(0)).Show()
}
```

### 平滑曲线和高度

`Smooth()` 只影响右轴折线；`Height()` 适合在统计卡片下方固定图表高度。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func smoothDualAxes(c *ui.Context) {
    rows := []chart.Row{{"month": "1月", "sales": 320, "rate": 0.12}, {"month": "2月", "sales": 460, "rate": 0.15}}
    chart.DualAxes(c, rows).X("month").Column("sales").Line("rate").Smooth().Height(340).Show()
}
```

### 交互和无障碍

默认悬停会同时展示两侧轴上的值；静态报告可用 `NoTooltip()`，并在 `A11yLabel()` 中说清两个量纲。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func accessibleDualAxes(c *ui.Context) {
    rows := []chart.Row{{"month": "1月", "sales": 320, "rate": 0.12}, {"month": "2月", "sales": 460, "rate": 0.15}}
    chart.DualAxes(c, rows).X("month").Column("sales").Line("rate").A11yLabel("月度销售额与转化率").NoTooltip().Show()
}
```

## 主题与样式

DualAxes 使用主题的柱状、折线、轴线和图例令牌；可用 `Colors()` 固定字段身份，或用 `Style(func(*chart.Style) { ... })` 调整单图。两侧轴的文字、别名和单位必须保持高对比度，不能只靠两种颜色区分。

## 注意事项

- 右轴是 TanGo `DualAxes` 的真实能力，但双轴容易夸大相关性；若量纲可以统一，请改用单轴图。
- `Column()` 与 `Line()` 中的字段必须存在于每行；缺失值应在业务层明确处理，不能让用户猜测空柱或断线含义。
- 两侧格式器应分别表达单位，例如左轴用 `Thousands(0)`、右轴用 `Percent(0)`；图例别名应包含业务语义。
- 即时模式下数据放在外部状态，条件切换图表时用 `Key()`；RTL 需要反向绘图区时才调用 `Mirror()`。

## 示例代码

<!--@include: ./_gen/chart-dualaxes-examples.md-->

## API

<!--@include: ./_gen/chart-dualaxes-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/chart/dual.go)
- [ui Go API](/reference/api/ui)
- [Line 折线图](/components/chart-line)
- [Column 柱状图](/components/chart-column)
