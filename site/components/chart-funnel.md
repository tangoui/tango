---
title: Funnel 漏斗图
---
# Funnel 漏斗图

按阶段顺序展示数量逐步减少的转化流程，并给出相邻阶段的转化率。

## 何时使用

- 展示访问、加购、下单、支付等顺序转化流程。
- 需要同时查看每个阶段的绝对量和相邻阶段转化率时。
- 阶段不是单向递减或需要比较多个分类时改用 [Column 柱状图](/components/chart-column) 或 [Line 折线图](/components/chart-line)。

## 在线体验

Gallery 展示带悬停提示和阶段转化率的电商转化漏斗。

<GalleryPreview component="chart.Funnel" />

## 基本用法

`X()` 指定阶段字段，`Y()` 指定数量字段；阶段按行顺序保留，通常应从最大阶段到最小阶段排列。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

var funnelRows = []chart.Row{
    {"stage": "浏览网站", "count": 50000},
    {"stage": "放入购物车", "count": 35000},
    {"stage": "生成订单", "count": 25000},
    {"stage": "完成交易", "count": 8000},
}

func page(c *ui.Context) {
    chart.Funnel(c, funnelRows).X("stage").Y("count").Format(chart.Thousands(0)).Show()
}
```

## 常见用法

### 隐藏转化率

默认在图中显示相邻阶段的转化率。`NoConversion()` 隐藏图中的转化率标注，悬停提示仍可显示转化率。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func funnelWithoutRate(c *ui.Context) {
    rows := []chart.Row{{"stage": "访问", "count": 1000}, {"stage": "注册", "count": 620}, {"stage": "购买", "count": 180}}
    chart.Funnel(c, rows).X("stage").Y("count").NoConversion().Show()
}
```

### 格式化和高度

`Format()` 影响数量和悬停值，`Height()` 适合在卡片或仪表盘中为漏斗预留稳定空间。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func sizedFunnel(c *ui.Context) {
    rows := []chart.Row{{"stage": "线索", "count": 120000}, {"stage": "商机", "count": 45000}, {"stage": "成交", "count": 9000}}
    chart.Funnel(c, rows).X("stage").Y("count").Format(chart.Compact("zh-CN")).Height(320).Show()
}
```

### 交互和无障碍

每个阶段默认可悬停查看数值；静态报告可调用 `NoTooltip()`，并用 `A11yLabel()` 说明漏斗的业务过程。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/chart"
)

func accessibleFunnel(c *ui.Context) {
    rows := []chart.Row{{"stage": "访问", "count": 1000}, {"stage": "注册", "count": 620}}
    chart.Funnel(c, rows).X("stage").Y("count").A11yLabel("注册转化漏斗").NoTooltip().Show()
}
```

## 主题与样式

Funnel 使用主题分类色板、阶段间隙、标签文字和提示卡片。可用 `Colors()` 或 `Style(func(*chart.Style) { ... })` 做单图覆盖；阶段名称、数值和转化率应保留文字表达，不能只靠漏斗宽度和颜色传达含义。

## 注意事项

- `Funnel()` 不会替你排序，数据行顺序就是流程顺序；阶段数量不递减时应先解释业务含义或改用其他图表。
- 转化率以相邻可见阶段计算，零值和缺失值会影响后续阶段比例。
- 即时模式下数据、当前筛选范围和加载状态放在外部状态；条件渲染时用 `Key()` 保持阶段图例和动画稳定。
- RTL 下 `Mirror()` 会改变漏斗和转化率标签的方向；仍应使用阶段文字和 `A11yLabel()` 传达流程。

## 示例代码

<!--@include: ./_gen/chart-funnel-examples.md-->

## API

<!--@include: ./_gen/chart-funnel-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/chart/funnel.go)
- [ui Go API](/reference/api/ui)
- [Steps 步骤条](/components/steps)
- [Column 柱状图](/components/chart-column)
