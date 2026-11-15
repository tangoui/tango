---
title: Statistic 统计数值
---
# Statistic 统计数值

突出展示金额、数量、增长值以及倒计时或计时器。

## 何时使用

- 仪表盘、概览卡片和详情页中强调关键指标。
- 需要实时截止时间用 `Countdown` 或 `StatisticTimer`；静态数值用 `Statistic`。
- 趋势图和多维比较不应塞进单个统计数值，应使用图表。

## 在线体验

<GalleryPreview component="Statistic" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    ui.Statistic(c, "活跃用户", 12345, 0).Suffix(" 人")
}
```

## 常见用法

### 前后缀和颜色

```go
func money(c *ui.Context) {
    ui.Statistic(c, "收入", 1280.5, 2).Prefix("¥").Color(c.Theme.Success.Base)
}
```

### 倒计时

```go
import "time"

func deadline(c *ui.Context) {
    ui.Countdown(c, "活动剩余", time.Now().Add(2*time.Hour)).Format("HH:mm:ss")
}
```

### 正向计时

```go
func elapsed(c *ui.Context) {
    ui.StatisticTimer(c, "运行时长", ui.TimerCountup, time.Now().Add(-5*time.Minute)).Format("HH:mm:ss")
}
```

## 主题与样式

Statistic 令牌控制标题、数值、前后缀和加载态颜色；`ValueSize` 可调整数值字号。倒计时支持 `Format`、`Prefix`、`Suffix`、`Color` 和 `OnChange`。

## 注意事项

倒计时目标时间应稳定保存，不能在每帧重新计算。定时器结束可读取 `Finished`；高频更新时只在 `OnChange` 中做轻量工作。颜色需配合标题或数值传达语义。

## 示例代码

<!--@include: ./_gen/statistic-examples.md-->

## API

<!--@include: ./_gen/statistic-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/display.go)
- [ProCard 高级卡片](/components/procard)
