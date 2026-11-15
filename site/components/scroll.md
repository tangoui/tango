---
title: Scroll 滚动容器
---
# Scroll 滚动容器

创建带覆盖式滚动条的纵向或横向滚动区域，支持拖拽滚动条、滚轮和代码控制滚动位置。

## 何时使用

- 内容高度超过可用空间，需要一个独立的纵向滚动区域时使用 `Scroll`。
- 一行内容需要横向滚动时使用 `ScrollH`，两个方向都可能溢出时使用 `Both`。
- 百万行等规则数据需要只构建视口内容时，优先使用 [Table 表格](/components/table) 或表格的虚拟滚动。
- 页面级布局需要侧栏、页头和内容区时，使用 [Layout 布局](/components/layout) 或 [ProLayout 高级布局](/components/prolayout)。

## 在线体验

<GalleryPreview component="Scroll" />

## 基本用法

滚动容器必须有边界高度；在已有定高父容器中可用 `layout.Grow(1)`，独立示例可直接使用 `layout.H`。

```go
import (
    "fmt"

    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func scrollPage(c *ui.Context) {
    ui.Scroll(c, func() {
        for i := 1; i <= 20; i++ {
            ui.Text(c, fmt.Sprintf("第 %d 行", i))
        }
    }, layout.H(220)).Always()
}
```

## 常见用法

### 横向滚动

`ScrollH` 把内容构建为横向滚动行，项目本身不要压缩到视口宽度。

```go
import (
    "fmt"

    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func horizontalScrollPage(c *ui.Context) {
    ui.ScrollH(c, func() {
        ui.Row(c, func() {
            for i := 1; i <= 12; i++ {
                ui.Column(c, func() { ui.Text(c, fmt.Sprintf("卡片 %d", i)) }, layout.Size(120, 64)).Border(1, c.Theme.BorderSecondary)
            }
        }, layout.Gap(8))
    }, layout.H(96)).Always()
}
```

### 双向滚动

`Both()` 在同一个滚动区域开启两个方向；适合宽表格、画布或代码预览。`Scroll` 默认只处理纵向滚动。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func bothScrollPage(c *ui.Context) {
    ui.Scroll(c, func() {
        ui.Column(c, func() {
            for i := 0; i < 8; i++ {
                ui.Text(c, "一段较宽的内容，超出视口后可横向滚动")
            }
        }, layout.W(900))
    }, layout.Size(360, 180)).Both().Always()
}
```

### 保存滚动容器 ID

`ID()` 可提供给需要知道目标滚动容器的其他组件；即时模式下把本帧 ID 写入持久变量，下一帧再使用。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var scrollTarget ui.ID

func targetScrollPage(c *ui.Context) {
    sb := ui.Scroll(c, func() {
        for i := 0; i < 30; i++ { ui.Text(c, "可滚动内容") }
    }, layout.H(240))
    scrollTarget = sb.ID()
}
```

### 代码滚动与读取偏移

`ScrollTo` 设置逻辑偏移，`Reveal` 只滚动使指定范围可见，`Offset` 读取最近一次绘制的偏移；平滑滚动需要传入时长。

```go
import (
    "fmt"
    "time"

    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var jumpToEnd bool

func controlledScrollPage(c *ui.Context) {
    sb := ui.Scroll(c, func() {
        for i := 0; i < 40; i++ { ui.Text(c, "行内容") }
    }, layout.H(200))
    if ui.Button(c, "跳到末尾").Clicked() { jumpToEnd = true }
    if jumpToEnd {
        sb.ScrollToSmooth(9999, 240*time.Millisecond)
        jumpToEnd = false
    }
    _, y := sb.Offset()
    ui.Text(c, fmt.Sprint("当前纵向偏移：", y))
}
```

## 主题与样式

滚动条使用当前主题的组件令牌，在 Ant、Element Plus 和 shadcn 下分别跟随各自的轨道、滑块和悬停色。`Always()` 只改变滚动条的可见策略，不改变内容布局；需要设置内容背景或边框时，在回调内包裹 `Column` 或 `Div`。

## 注意事项

- **边界**：没有 `layout.H`、`layout.MaxH` 或可增长的定高父容器时，滚动区域没有可计算的视口高度。
- **状态**：滚动位置由容器按 ID 保留；不要每帧用新的条件重建不同的滚动容器。需要跳转时在页面状态中保存请求。
- **性能**：Scroll 会构建回调中的内容；大数据量使用 `VirtualList`、Masonry `Virtual` 或表格 `Virtual`，不要把百万行直接塞进普通 Scroll。
- **无障碍**：滚动条之外要保证键盘焦点能进入内容；可见区域改变不应依赖鼠标滚轮，列表项需要有可读文本。
- **RTL**：`ScrollTo` 的横向 `x=0` 表示内联起点，在 RTL 中是右侧；`Offset` 的横向值也使用这一逻辑坐标。

## 示例代码

<!--@include: ./_gen/scroll-examples.md-->

## API

<!--@include: ./_gen/scroll-api.md-->

## 相关链接

- [源码 scroll.go](https://github.com/tangoui/tango/blob/main/ui/scroll.go)
- [ui Go API](/reference/api/ui)
- [Table 表格](/components/table)、[Layout 布局](/components/layout)
