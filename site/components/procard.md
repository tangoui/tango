---
title: ProCard 高级卡片
---
# ProCard 高级卡片

页面级卡片容器，支持标题区、分栏、栅格、折叠、标签页和加载状态。

## 何时使用

- 后台页面中组织多个信息区块或统计区域。
- 需要卡片内分栏、多卡片栅格或卡片标签页时使用；简单容器可用 [Card 卡片](/components/card)。
- 需要指标卡时可配合 `StatisticCard` 使用。

## 在线体验

<GalleryPreview component="ProCard" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    ui.ProCard(c, "项目概览").SubTitle("本周").Bordered().Body(func() {
        ui.Statistic(c, "完成率", 86, 0).Suffix("%")
    })
}
```

## 常见用法

### 分栏卡片

```go
func split(c *ui.Context) {
    ui.ProCard(c, "").Split(ui.SplitVertical).Bordered().Body(func() {
        ui.ProCard(c, "左侧").ColSpan(8).Body(func() { ui.Text(c, "详情") })
        ui.ProCard(c, "右侧").Body(func() { ui.Text(c, "趋势") })
    })
}
```

### 栅格与间距

```go
func grid(c *ui.Context) {
    ui.ProCard(c, "").Ghost().Gutter(16).Wrap().Body(func() {
        ui.ProCard(c, "A").ColSpan(12).Body(nil)
        ui.ProCard(c, "B").ColSpan(12).Body(nil)
    })
}
```

### 折叠和标签页

```go
var folded bool
var activeTab = "base"

func interactive(c *ui.Context) {
    ui.ProCard(c, "可折叠").Collapsed(&folded).Body(func() { ui.Text(c, "内容") })
    ui.ProCard(c, "").Tabs(&activeTab, []ui.ProCardTab{
        {Key: "base", Label: "基础", Body: func() { ui.Text(c, "基础信息") }},
        {Key: "log", Label: "日志", Body: func() { ui.Text(c, "日志") }},
    }).Body(nil)
}
```

## 主题与样式

ProCard 使用 Pro 卡片令牌控制边框、头部分隔线、间距、悬停和 shadcn 头部样式。默认键为 `size`、`bordered`、`headerBordered`、`ghost`、`hoverable`、`collapsible`、`gutter`。

## 注意事项

折叠和标签状态应绑定到页面变量；子 ProCard 的栅格只在父卡片 Body 中直接生效。加载态用于等待数据，不应阻塞整个页面；可折叠头部和标签页支持键盘操作。

## 示例代码

<!--@include: ./_gen/procard-examples.md-->

## API

<!--@include: ./_gen/procard-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/pro2_card.go)
- [Card 卡片](/components/card)
