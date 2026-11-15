---
title: StatisticCard 指标卡
---
# StatisticCard 指标卡

面向后台仪表盘的指标卡，集中展示数值、趋势、状态、描述和可选的小型图表。

## 何时使用

- 在仪表盘或概览页展示销售额、访问量、转化率等关键指标。
- 需要把数值与同比/环比、状态点、图表或页脚说明放在同一张卡片中时。
- 只需要一个普通数字时用 [Statistic 统计数值](/components/statistic)；需要通用内容容器时用 [ProCard 高级卡片](/components/procard)。

## 在线体验

示例包含基础趋势、状态点、描述、指标组、图标、页脚以及左右放置的小型图表。

<GalleryPreview component="StatisticCard" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    ui.StatisticCard(c, "总销售额").Value(126560, 0).Prefix("¥").Show()
}
```

`Value(value, precision)` 按精度格式化数字并添加千位分隔符，`Prefix` 和 `Suffix` 分别放在数字前后。链式设置完成后调用 `Show()` 构建卡片；`title` 是指标标题，不是卡片头部标题。

## 常见用法

以下片段省略了 `import`。

### 趋势、状态与说明

`Trend` 接受 `up` 或 `down`，`Desc` 添加指标下方的比较项，`Status` 在标题前显示状态点，`Tooltip` 为标题添加说明。

```go
func trendCard(c *ui.Context) {
    ui.StatisticCard(c, "运营活动效果").Tooltip("近 30 天活动带来的转化效果").
        Value(78, 0).Suffix("%").Trend("up").
        Desc("周同比", "12%", "up").Desc("日同比", "11%", "down").
        Status("processing").Show()
}
```

### 图标、颜色与页脚

`Icon` 在指标左侧放置图标，`ValueColor` 修改数值颜色，`Footer` 在分隔线下构建补充信息。页脚回调可以放普通文本、链接或操作。

```go
func decoratedCard(c *ui.Context) {
    ui.StatisticCard(c, "支付金额").Icon(icon.Shop).Value(2176, 0).Prefix("¥").
        ValueColor(c.Theme.Primary.Base).
        Footer(func() {
            ui.Text(c, "今日累计 · 12:00 更新").Tertiary()
        }).Show()
}
```

### 指标组

`StatisticCardGroup` 创建带分隔线的 ProCard，直接在 `Body` 中放多个指标卡。单张卡可以用 `ColSpan`、`Layout` 等方式参与父级布局。

```go
func metricGroup(c *ui.Context) {
    ui.StatisticCardGroup(c).Body(func() {
        ui.StatisticCard(c, "今日 UV").Value(79, 0).Desc("较昨日", "8.04%", "up").Show()
        ui.StatisticCard(c, "订单数").Value(3701928, 0).Status("success").Show()
        ui.StatisticCard(c, "转化率").Value(6.8, 1).Suffix("%").Show()
    })
}
```

### 接入小型图表

`Chart` 接受一个构建任意内容的回调。`StatisticCard` 不直接依赖 `ui/chart`，因此图表包需要由页面显式导入；`ChartPlace` 可使用 `ChartBottom`、`ChartLeft` 或 `ChartRight`。

```go
func chartCard(c *ui.Context) {
    values := []float64{264, 417, 438, 887, 309, 397, 550}
    ui.StatisticCard(c, "访问量").Value(8846, 0).
        ChartPlace(ui.ChartRight).
        Chart(func() { chart.TinyArea(c, values).Smooth().Show() }).
        Layout(layout.W(420)).Show()
}
```

### 加载和可点击卡片

`Loading(true)` 显示卡片加载占位；`Hoverable` 让卡片具有悬浮反馈，点击后通过 `Clicked` 处理导航或展开详情。

```go
var loadingMetric = true

func interactiveCard(c *ui.Context) {
    card := ui.StatisticCard(c, "待处理任务").Value(24, 0).
        Loading(loadingMetric).Hoverable().Show()
    if card.Clicked() {
        ui.Text(c, "打开任务列表").Tertiary()
    }
}
```

## 主题与样式

StatisticCard 使用 ProCard 和 `theme.Components.Statistic` 的指标令牌：标题字号、数值字号、行高和数值颜色由当前主题决定。Ant Design 使用轻量的指标标题与醒目数字；Element Plus 采用 Element 卡片边框；shadcn 使用更重的数值字重和卡片表面。

`ConfigProvider.Defaults("StatisticCard", ...)` 支持 `"bordered"`、`"hoverable"` 和 `"chartPlacement"`（`bottom`、`left`、`right`）。实例上的 `Bordered`、`Hoverable` 和 `ChartPlace` 优先。

## 注意事项

- **调用顺序**：`Chart`、`Footer`、`CardTitle`、`Layout` 等配置在 `Show` 前调用；`Clicked` 在 `Show` 返回后读取。
- **状态放在哪**：指标值、请求加载状态、趋势方向和当前选中卡片由页面状态持有；不要在每帧计算中随机生成指标。
- **图表性能**：卡片内图表适合小型趋势概览；数据量很大时先在应用层聚合，避免每张卡都重复计算全量数据。
- **无障碍**：标题、数值和趋势会作为卡片内容呈现；图表回调仍应提供可读的文字摘要或配套说明。可点击卡片要有明确标题和操作结果。
- **RTL**：前缀、数值、后缀和左右图表布局遵循当前 RTL 方向。

## 示例代码

<!--@include: ./_gen/statisticcard-examples.md-->

## API

<!--@include: ./_gen/statisticcard-api.md-->

## 相关链接

- 源码：[ui/pro2_statistic.go](https://github.com/tangoui/tango/blob/main/ui/pro2_statistic.go)
- [ui Go API 参考](/reference/api/ui)
- [Statistic 统计数值](/components/statistic)、[ProCard 高级卡片](/components/procard)、[ui/chart API](/reference/api/ui-chart)
- [组件总览](/components/)、[组件截图目录](/components/screenshots)
