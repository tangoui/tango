---
title: Grid 栅格
---
# Grid 栅格

把一行内容划分为 24 列，统一处理列宽、间距、偏移和响应式断点。

## 何时使用

- 表单、仪表盘和卡片需要稳定列宽与响应式换行时。
- 需要 24 列精确控制，或需要 offset、push、pull、order 时使用 Grid。
- 仅需一维排列和间距用 [Flex 弹性布局](/components/flex)；仅需均匀留白用 [Space 间距](/components/space)。
- 页面整体框架用 [Layout 布局](/components/layout)，不要用 Grid 代替 Layout。

## 在线体验

<GalleryPreview component="GridRow" />

## 基本用法

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func page(c *ui.Context) {
    ui.GridRow(c).Gutter(16, 8).Body(func() {
        ui.Col(c, 8).Body(func() { ui.Column(c, func() { ui.Text(c, "左") }, layout.Pad(12)) })
        ui.Col(c, 16).Body(func() { ui.Column(c, func() { ui.Text(c, "右") }, layout.Pad(12)) })
    })
}
```

一行总宽为 24，`Col(c, 8)` 占三分之一，`Col(c, 16)` 占三分之二。`Gutter(horizontal, vertical)` 设置列间距。

## 常见用法

### 等分列

每个 `Col` 使用相同 span 即可构成等分布局，所有列放进同一个 `GridRow.Body`。

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    ui.GridRow(c).Gutter(12, 12).Body(func() {
        for i := 0; i < 4; i++ {
            i := i
            ui.Col(c, 6).Body(func() { ui.Text(c, "卡片 " + string(rune('A'+i))) })
        }
    })
}
```

### 响应式列宽

用 `XS`、`SM`、`MD`、`LG`、`XL`、`XXL` 覆盖对应断点的 span；可选的第二个参数是 offset。

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    ui.GridRow(c).Gutter(16, 16).Body(func() {
        ui.Col(c, 24).MD(12).LG(6).Body(func() { ui.Text(c, "响应式卡片") })
        ui.Col(c, 24).MD(12).LG(6).Body(func() { ui.Text(c, "响应式卡片") })
    })
}
```

### 偏移与对齐

`Offset` 留出空列；`Justify` 分配整行剩余空间，`Align` 控制列在交叉轴的对齐方式。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func page(c *ui.Context) {
    ui.GridRow(c).Justify(layout.JustifySpaceBetween).Align(layout.AlignCenter).Body(func() {
        ui.Col(c, 6).Body(func() { ui.Text(c, "起点") })
        ui.Col(c, 6).Offset(6).Body(func() { ui.Text(c, "偏移后") })
    })
}
```

### Push、Pull 与顺序

`Push`、`Pull` 移动列的视觉位置，`Order` 调整 flex 顺序；优先用结构顺序表达语义，只有布局需要时再改变视觉顺序。

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    ui.GridRow(c).Body(func() {
        ui.Col(c, 8).Order(2).Body(func() { ui.Text(c, "第二个视觉位置") })
        ui.Col(c, 16).Order(1).Body(func() { ui.Text(c, "第一个视觉位置") })
    })
}
```

## 主题与样式

Grid 本身只负责列计算和 gutter；列内容的背景、圆角和内边距由 `Col.Body` 内的容器负责。Ant、Element Plus、shadcn 的主要差异在断点语义与外层容器样式，Tango 断点统一使用 `XS` 至 `XXL`。

## 注意事项

- `span`、offset、push、pull 等列数以 24 为基准，合计超过 24 时会换行或产生溢出布局。
- `GutterAt` 可按断点改变水平间距；大量卡片应通过响应式 span 降低窄屏拥挤。
- Grid 是 flex 布局，列内长文本和控件仍需设置合理宽度或换行策略。
- `Order` 只改变视觉顺序，屏幕阅读器仍可能按 DOM 顺序阅读；无障碍重要内容不要只依赖视觉重排。
- RTL 下 offset、push、pull 与顺序会按布局方向处理。

## 示例代码

<!--@include: ./_gen/gridrow-examples.md-->

## API

<!--@include: ./_gen/gridrow-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/nav_grid.go)
- [ui Go API](/reference/api/ui)
- [Flex 弹性布局](/components/flex)
