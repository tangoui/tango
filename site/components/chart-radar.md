---
title: Radar 雷达图
---
# Radar 雷达图

在多个共同量纲的维度上同时比较一组或多组指标。

## 何时使用

- 比较同一评价模型下多个维度的能力、评分或完成度。
- 需要在少量维度中看整体轮廓和系列差异时。
- 维度很多或需要精确读数时改用 [Table 表格](/components/table) 或 [Column 柱状图](/components/chart-column)。

## 在线体验

Gallery 展示多系列雷达图，以及圆形网格、仅描边和固定最大值的场景。

<GalleryPreview component="chart.Radar" />

## 基本用法

`X()` 是维度字段，`Y()` 是数值字段，`Series()` 将多行拆成多个雷达轮廓。少于三个维度时显示空状态。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

var skillRows = []chart.Row{
    {"item": "设计", "user": "甲", "score": 70},
    {"item": "开发", "user": "甲", "score": 60},
    {"item": "设计", "user": "乙", "score": 50},
    {"item": "开发", "user": "乙", "score": 80},
}

func page(c *ui.Context) {
    chart.Radar(c, skillRows).X("item").Y("score").Series("user").YMax(100).Show()
}
```

## 常见用法

### 固定最大值

雷达图从零起算。评分、百分比等有明确上限时用 `YMax()` 固定上限，避免不同数据批次改变网格尺度。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func fixedRadar(c *ui.Context) {
    rows := []chart.Row{{"item": "质量", "score": 82}, {"item": "速度", "score": 65}, {"item": "稳定性", "score": 91}}
    chart.Radar(c, rows).X("item").Y("score").YMax(100).Format(chart.Thousands(0)).Show()
}
```

### 圆形网格和仅描边

`Circle()` 将多边形网格改为圆形，`NoArea()` 只绘制轮廓，适合多个系列重叠时减少填充遮挡。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func outlineRadar(c *ui.Context) {
    rows := []chart.Row{{"item": "质量", "user": "甲", "score": 82}, {"item": "速度", "user": "甲", "score": 65}}
    chart.Radar(c, rows).X("item").Y("score").Series("user").Circle().NoArea().Show()
}
```

### 图例和提示

多系列雷达保留图例，用户可切换系列；默认悬停提示显示对应维度的数值，需要展示静态图时可使用 `NoTooltip()`。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func radarA11y(c *ui.Context) {
    rows := []chart.Row{{"item": "设计", "user": "甲", "score": 70}, {"item": "开发", "user": "甲", "score": 60}}
    chart.Radar(c, rows).X("item").Y("score").Series("user").A11yLabel("团队能力雷达图").NoTooltip().Show()
}
```

## 主题与样式

Radar 的网格、填充透明度、轮廓线和分类色由当前 chart style 提供。可用 `Colors()` 固定系列色，或用 `Style(func(*chart.Style) { ... })` 做局部调整；当填充重叠时优先 `NoArea()`，不要只靠颜色区分。

## 注意事项

- 所有维度应使用同一量纲和相同方向，缺失值应先处理，不要把不同单位强行放进同一雷达图。
- 维度顺序会影响轮廓形状，应在数据源中稳定排序；筛选系列不应改变剩余系列的颜色。
- 即时模式下把评分数据放在外部状态，条件切换图表时设置稳定 `Key()`；用 `A11yLabel()` 和图例补充颜色之外的身份信息。
- RTL 只影响外围文字和图例；如需镜像网格方向再调用 `Mirror()`。

## 示例代码

<!--@include: ./_gen/chart-radar-examples.md-->

## API

<!--@include: ./_gen/chart-radar-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/chart/radar.go)
- [ui Go API](/reference/api/ui)
- [Scatter 散点图](/components/chart-scatter)
- [Progress 进度条](/components/progress)
