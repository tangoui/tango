---
title: Masonry 瀑布流
---
# Masonry 瀑布流

把不等高的项目依次放入当前最短列，形成支持响应式列数和虚拟化的瀑布流布局。

## 何时使用

- 卡片、图片或内容块高度不同，又希望自动填充多列时使用。
- 需要按断点改变列数，或需要向已有列表追加项目时使用。
- 项目天然是一行一行对齐、需要表头或列排序时，使用 [Table 表格](/components/table) 或 [List 列表](/components/list) 更合适。
- 单列内容只需要滚动时，直接使用 [Scroll 滚动容器](/components/scroll)。

## 在线体验

<GalleryPreview component="Masonry" />

## 基本用法

`Masonry` 接收项目数量和按索引构建项目的回调；设置已知高度后，布局可以在首帧直接稳定分列。

```go
import (
    "fmt"

    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var cardHeights = []float32{96, 140, 112, 168}

func masonryPage(c *ui.Context) {
    ui.Masonry(c, len(cardHeights), func(i int) {
        ui.Column(c, func() {
            ui.Text(c, fmt.Sprintf("卡片 %d", i+1)).Strong()
            ui.Text(c, "内容块")
        }, layout.Pad(12)).Border(1, c.Theme.BorderSecondary).Radius(c.Theme.RadiusLG)
    }).Columns(2).Gutter(16).
        ItemHeight(func(i int) float32 { return cardHeights[i] }).Show()
}
```

## 常见用法

### 响应式列数与不等间距

`ColumnsAt` 按当前窗口断点覆盖默认列数；`Gutters` 可以分别设置水平和垂直间距。

```go
import "github.com/tangoui/tango/ui"

func responsiveMasonry(c *ui.Context) {
    ui.Masonry(c, 12, func(i int) {
        ui.Text(c, "内容卡片")
    }).Columns(3).
        ColumnsAt(ui.XS, 1).
        ColumnsAt(ui.SM, 2).
        ColumnsAt(ui.MD, 4).
        Gutters(16, 12).
        ItemHeight(func(i int) float32 { return float32(80 + i%3*24) }).Show()
}
```

### 固定项目身份与列

`ItemKey` 在项目顺序可能变化时提供稳定身份；`ItemColumn` 返回从 0 开始的列号，返回 `-1` 让项目继续自动落到最短列。

```go
import "github.com/tangoui/tango/ui"

var masonryIDs = []string{"a", "b", "c", "d"}

func stableMasonry(c *ui.Context) {
    ui.Masonry(c, len(masonryIDs), func(i int) {
        ui.Text(c, masonryIDs[i])
    }).Columns(2).
        ItemKey(func(i int) any { return masonryIDs[i] }).
        ItemColumn(func(i int) int {
            if masonryIDs[i] == "a" { return 0 }
            return -1
        }).
        ItemHeight(func(i int) float32 { return 100 }).Show()
}
```

### 未知高度与估计值

不提供 `ItemHeight` 时，Tango 会在绘制后测量项目高度；首帧使用 `EstimatedHeight` 排布，实测后再修正。

```go
import (
    "fmt"

    "github.com/tangoui/tango/ui"
)

func measuredMasonry(c *ui.Context) {
    ui.Masonry(c, 24, func(i int) {
        ui.Text(c, fmt.Sprintf("第 %d 项：内容长度可能不同", i+1))
    }).Columns(4).Gutter(12).EstimatedHeight(110).Show()
}
```

### 虚拟化大列表

`Virtual(height)` 为瀑布流建立固定视口，只构建视口附近项目；`Built()` 可用于显示本帧实际构建数量。

```go
import (
    "fmt"

    "github.com/tangoui/tango/ui"
)

func virtualMasonry(c *ui.Context) {
    m := ui.Masonry(c, 100000, func(i int) {
        ui.Text(c, fmt.Sprintf("卡片 %d", i+1))
    }).Columns(5).Gutter(12).EstimatedHeight(96).Virtual(360).Show()
    ui.Text(c, fmt.Sprintf("本帧构建 %d 项", m.Built())).Tertiary()
}
```

## 主题与样式

Masonry 只负责分列和滚动，不替项目绘制背景。项目卡片可使用 `c.Theme.BgContainer`、`c.Theme.BorderSecondary`、`c.Theme.RadiusLG` 等当前主题令牌；Ant、Element Plus 和 shadcn 的差异来自这些令牌，而不是 Masonry 自己的变体。

## 注意事项

- **状态**：项目状态放在页面函数外或 `ui.State` 中；虚拟化会反复构建项目，不能把业务状态只放在一次回调调用的局部变量里。
- **稳定性**：追加项目时用 `ItemKey` 和必要的 `ItemColumn`，避免已有项目身份或列位置随数据重排。
- **性能**：大量项目优先提供 `EstimatedHeight` 并使用 `Virtual`；已知固定高度时使用 `ItemHeight` 可减少测量和重排。
- **无障碍**：每个项目会以列表项语义构建；项目内部的图标按钮仍需 `A11yLabel`，不要只用序号作为唯一可读名称。
- **RTL**：列的内联起点会遵循当前方向；`ItemColumn` 仍使用从 0 开始的逻辑列号。

## 示例代码

<!--@include: ./_gen/masonry-examples.md-->

## API

<!--@include: ./_gen/masonry-api.md-->

## 相关链接

- [源码 fill_masonry.go](https://github.com/tangoui/tango/blob/main/ui/fill_masonry.go)
- [ui Go API](/reference/api/ui)
- [Scroll 滚动容器](/components/scroll)、[Card 卡片](/components/card)
