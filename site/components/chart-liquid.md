---
title: Liquid 水波图
---
# Liquid 水波图

用圆形水位和波纹表现 0 到 1 的容量、完成率或占用比例。

## 何时使用

- 需要在卡片中突出一个容量、占用或完成比例时。
- 希望用比环形进度更具视觉识别度的水位表达单一比例时。
- 需要比较多个指标或显示精确刻度时改用 [Gauge 仪表盘](/components/chart-gauge) 或 [Progress 进度条](/components/progress)。

## 在线体验

Gallery 在 Gauge 页面中展示低水位和高水位水波图；同一 gallery 场景可切换主题和暗色变体。

<GalleryPreview component="chart.Liquid" />

## 基本用法

`Liquid()` 接收 0 到 1 的比例，默认显示百分比和动态波纹；`Title()` 为比例添加说明。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func page(c *ui.Context) {
    chart.Liquid(c, 0.3).Title("容量").Show()
}
```

## 常见用法

### 静态水波

`Still()` 停止波纹运动，适合截图、静态报告和用户启用减少动效时的场景。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func stillLiquid(c *ui.Context) {
    chart.Liquid(c, 0.78).Title("存储使用率").Still().Show()
}
```

### 自定义格式和尺寸

`Format()` 控制中心值和悬停提示的文本，`Height()` 设置绘图区高度；比例仍然按 0 到 1 传入。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func formattedLiquid(c *ui.Context) {
    chart.Liquid(c, 0.625).Format(chart.Percent(1)).Height(240).Title("同步进度").Show()
}
```

### 颜色和无障碍

`Colors()` 覆盖水波颜色，`A11yLabel()` 提供完整语义；状态文字不要只依赖水位颜色。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
    "github.com/tangoui/tango/ui/theme"
)

func accessibleLiquid(c *ui.Context) {
    chart.Liquid(c, 0.42).Colors(theme.Hex("#1677ff")).Title("容量").A11yLabel("容量使用率 42%").Still().Show()
}
```

### 静态展示和交互

水波默认支持悬停提示；静态大屏可用 `NoTooltip()`，异步数据到达前可用 `Loading(true)`。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func staticLiquid(c *ui.Context) {
    chart.Liquid(c, 0.42).Title("容量").NoTooltip().Loading(false).Show()
}
```

## 主题与样式

Liquid 使用主题首个系列色、容器表面和文字对比度计算水波层；可用 `Colors()` 调整颜色，或用 `Style(func(*chart.Style) { ... })` 做单图样式覆盖。暗色和 shadcn 主题会使用各自的背景与文字令牌，中心文字仍应保持可读。

## 注意事项

- 传入值是比例而不是百分数，业务层应先把原始容量换算为 0 到 1 并校验分母为零；绘制会将超范围值限制在可视范围内。
- 动态波纹会持续唤醒 UI；静态导出、低动效偏好或不需要动画时调用 `Still()`。
- 即时模式下把比例放在外部状态，条件切换多个水波图时用稳定 `Key()`；异步状态用 `Loading(true)` 而不是暂时显示 0%。
- `A11yLabel()` 与 `Title()` 应说明指标和单位；RTL 需要反向波纹方向时才调用 `Mirror()`。

## 示例代码

<!--@include: ./_gen/chart-liquid-examples.md-->

## API

<!--@include: ./_gen/chart-liquid-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/chart/liquid.go)
- [ui Go API](/reference/api/ui)
- [Gauge 仪表盘](/components/chart-gauge)
- [Progress 进度条](/components/progress)
