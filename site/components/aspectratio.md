---
title: AspectRatio 宽高比
---
# AspectRatio 宽高比

保持固定宽高比的容器（对标 shadcn/ui AspectRatio 与 CSS `aspect-ratio`）：宽度取父容器给的宽度，高度按比例算出，内容铺满整个盒子。

## 何时使用

- 封面图、视频占位、缩略图卡片等需要在不同宽度下保持同一比例（16:9、4:3、1:1……）的区域。
- 需要按比例裁剪圆角：`Radius` 同时给盒子和内容裁圆角。
- 只需要固定宽高的盒子用 [Flex 弹性布局](/components/flex) 的尺寸选项或 `layout.W` / `layout.H`；需要一组等宽卡片的网格用 [Grid 栅格](/components/gridrow)。

## 在线体验

<GalleryPreview component="AspectRatio" />

## 基本用法

`ui.AspectRatio(c, ratio, fn)` 的 `ratio` 是宽 / 高（`16.0/9` 表示 16:9），`fn` 在盒子里构建内容，内容会被拉伸铺满整个盒子。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func page(c *ui.Context) {
    ui.Column(c, func() {
        ui.AspectRatio(c, 16.0/9, func() {
            ui.Div(c).Bg(c.Theme.Primary.Bg)
            ui.Column(c, func() {
                ui.Text(c, "16 / 9").Strong()
            }, layout.Center())
        }).Radius(c.Theme.RadiusLG).A11yLabel("封面")
    }, layout.W(320))
}
```

在列里，它占满列的宽度，因此外层列（或父容器）要有确定的宽度；高度 = 宽度 / ratio。`ratio` 小于等于 0 或不是数字时按 1 处理。

## 常见用法

### 多种比例并排

放在 `Row` 里时，盒子本身没有宽度来源，需要用一个能分配宽度的容器包一层（例如 `layout.Grow(1)` 的列），或直接给 `AspectRatio` 传布局选项。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func ratioRow(c *ui.Context) {
    ratios := []struct {
        name  string
        ratio float32
    }{{"16 / 9", 16.0 / 9}, {"4 / 3", 4.0 / 3}, {"1 / 1", 1}}

    ui.Row(c, func() {
        for _, r := range ratios {
            ui.Column(c, func() {
                ui.AspectRatio(c, r.ratio, func() {
                    ui.Div(c).Bg(c.Theme.Primary.Bg)
                    ui.Column(c, func() { ui.Text(c, r.name) }, layout.Center())
                }).Radius(8).A11yLabel(r.name)
            }, layout.Grow(1))
        }
    }, layout.Gap(16), layout.AlignItems(layout.AlignStart))
}
```

`AlignStart` 让各列顶对齐，高度不同的盒子不会被拉伸。

### 固定宽度与最大宽度

没有任何东西给它宽度时（例如直接放在 `Row` 或叠放容器里），盒子宽度为 0，可以用布局选项给定宽度。`MaxWidth(px)` 限制最大宽度，高度同步缩小。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func ratioSized(c *ui.Context) {
    ui.Row(c, func() {
        ui.AspectRatio(c, 2, func() {
            ui.Div(c).Bg(c.Theme.Primary.Bg)
        }, layout.W(200))
    })
    ui.Column(c, func() {
        ui.AspectRatio(c, 21.0/9, func() {
            ui.Div(c).Bg(c.Theme.Primary.BgHover)
        }).MaxWidth(420).Radius(c.Theme.RadiusLG)
    })
}
```

### 圆角裁剪

`Radius(r)` 给盒子设置圆角并裁剪内容，内容不会溢出圆角；需要保持直角时不调用即可。

### 读取尺寸

`Ratio()` 返回实际生效的比例（非法值已修正为 1），`Rect()` 返回上一帧的盒子矩形，`ID()` 是盒子的标识，可用于定位或测试。

```go
import (
    "fmt"

    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func ratioInfo(c *ui.Context) {
    ui.Column(c, func() {
        box := ui.AspectRatio(c, 4.0/3, func() {
            ui.Div(c).Bg(c.Theme.Primary.Bg)
        })
        r := box.Rect()
        ui.Text(c, fmt.Sprintf("比例 %.2f，上一帧尺寸 %.0f×%.0f", box.Ratio(), r.W, r.H)).Tertiary()
    }, layout.W(280))
}
```

## 主题与样式

AspectRatio 本身不绘制任何东西，也没有组件令牌和 `ConfigProvider.Defaults` 键；背景、圆角取值请从内容和 `c.Theme`（例如 `RadiusLG`）取。三套风格下表现一致。

## 注意事项

- **宽度来源**：高度完全由宽度推算。放在 `Column` 里会取列宽；放在 `Row`、`StackBox` 等不分配宽度的父容器里时要用布局选项（`layout.W`、`layout.Grow`）或 `MaxWidth` 提供宽度，否则盒子是 0×0。
- **内容铺满**：内容放在一个叠放容器中，`Div`、`Column` 这类容器会被拉伸到整个盒子。`ui.Image` 不会拉伸，仍按自己的尺寸绘制；要用图片填满盒子，需要自绘（例如 `ui.Custom`）或先把图片按盒子尺寸处理。
- **状态**：AspectRatio 没有内部状态，比例由调用方每帧传入，改变比例下一帧就生效。
- **无障碍**：`A11yLabel(s)` 把盒子暴露为带名称的分组（图片说明）；纯装饰区域不设置即可。

## 示例代码

<!--@include: ./_gen/aspectratio-examples.md-->

## API

<!--@include: ./_gen/aspectratio-api.md-->

## 相关链接

- [源码：aspect_ratio.go](https://github.com/tangoui/tango/blob/main/ui/aspect_ratio.go)
- [ui Go API](/reference/api/ui)
- [Flex 弹性布局](/components/flex)
- [Image 图片](/components/image)
