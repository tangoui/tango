---
title: BorderBeam 边框光束
---
# BorderBeam 边框光束

沿内容边框绘制流动光束的装饰容器，用于强调进行中、推荐或需要关注的内容。

## 何时使用

- 需要在卡片、按钮或其他内容外围加入轻量的动态强调效果时使用。
- 需要通过颜色、速度、线宽、长度或数量表达状态层级时使用。
- 只是需要普通边框、分割线或焦点环时，使用容器的 `Border`、[Divider 分割线](/components/divider) 或组件自带焦点样式。
- 不要用 BorderBeam 承载交互逻辑；它只是包裹内容并绘制装饰。

## 在线体验

gallery 展示默认主色光束、三色渐变、三条短光束和仅在悬停时激活的光束，并会按动画设置展示静止或隐藏状态。

<GalleryPreview component="BorderBeam" />

## 基本用法

`BorderBeam` 通过 `Body` 包裹内容；默认使用主题主色、1px 光束、100px 长度、单条光束和 6 秒一圈。

```go
import "github.com/tangoui/tango/ui"

func borderBeamPage(c *ui.Context) {
    ui.BorderBeam(c).Body(func() {
        ui.Card(c, "实时同步").Body(func() {
            ui.Text(c, "正在连接服务器……")
        })
    })
}
```

返回值是普通 `*ui.Box`，可以继续使用布局和容器方法；光束绘制在内容上方，不改变业务内容的点击语义。

## 常见用法

### 单色、线宽和速度

`Color` 设置单色光束，`LineWidth` 设置线宽，`Size` 设置光束长度，`Duration` 设置一圈所需时间。

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func borderBeamOptionsPage(c *ui.Context) {
    ui.BorderBeam(c).Color(c.Theme.Primary.Base).
        LineWidth(2).Size(140).Duration(3 * time.Second).
        Body(func() {
            ui.Card(c, "重点任务").Layout(layout.W(240)).Body(func() {
                ui.Text(c, "2px、140px、3 秒").Tertiary()
            })
        })
}
```

### 渐变与多条光束

`Gradient` 的 `BeamStop.Percent` 从 0 到 100 表示从光束尾部到头部；`Count` 将多条光束沿边框均匀分布。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/theme"
)

func borderBeamGradientPage(c *ui.Context) {
    ui.BorderBeam(c).Gradient(
        ui.BeamStop{Color: theme.Hex("#1677ff"), Percent: 0},
        ui.BeamStop{Color: theme.Hex("#36cfc9"), Percent: 52},
        ui.BeamStop{Color: theme.Hex("#95de64"), Percent: 100},
    ).Count(3).Size(56).Body(func() {
        ui.Text(c, "渐变多光束")
    })
}
```

### 圆角、外扩和布局

`Radius` 覆盖默认的卡片圆角；`Outset` 可以把光束移到边框外侧或内侧；`Layout` 给包裹容器传入布局选项。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func borderBeamLayoutPage(c *ui.Context) {
    ui.BorderBeam(c).Radius(12).Outset(1).Layout(layout.W(320)).Body(func() {
        ui.Text(c, "固定宽度的强调区域")
    })
}
```

### 只在悬停时显示

`Active` 可以绑定到包裹内容的 `Hovered()` 结果，使光束只在用户关注该区域时绘制。

```go
import "github.com/tangoui/tango/ui"

var beamHover bool

func borderBeamHoverPage(c *ui.Context) {
    box := ui.BorderBeam(c).Active(beamHover).Body(func() {
        ui.Button(c, "悬停显示光束").Primary()
    })
    beamHover = box.Hovered()
}
```

### 降低动画干扰

组件会在 `NoAnimations` 下停在固定位置，在 `ReduceMotion` 下隐藏光束；业务代码只需保持装饰可选，不要把动画状态当作业务状态。

```go
import "github.com/tangoui/tango/ui"

var beamEnabled = true

func borderBeamTogglePage(c *ui.Context) {
    ui.Switch(c, &beamEnabled).A11yLabel("光束")
    if beamEnabled {
        ui.Text(c, "光束已启用")
    }
    ui.BorderBeam(c).Active(beamEnabled).Body(func() {
        ui.Text(c, "可选的装饰效果")
    })
}
```

## 主题与样式

- 默认渐变使用当前主题的 `Primary.Hover` 到 `Primary.Base`；不传 `Color` 或 `Gradient` 时无需硬编码颜色。
- 组件没有独立的公开 token 结构，尺寸、圆角和布局由 `BorderBeamW` 的方法及包裹内容主题共同决定。
- Ant Design、Element Plus 和 shadcn 都保留 BorderBeam 作为 Tango 的装饰扩展；基础卡片、按钮和文字仍使用各自主题令牌。

## 注意事项

- **状态放在哪**：`Active` 的值来自页面状态或包级变量；`Body` 每帧都会构建，不能把状态只放在回调局部变量中。
- **性能**：活动光束会请求动画帧并沿圆角矩形分段绘制；只在需要强调时开启，页面大量卡片不应全部持续动画。
- **无障碍**：光束不提供语义和状态，重要状态必须由文字、状态组件或无障碍标签表达；减少动画时内容仍应完整可读。
- **层级**：光束画在内容上方，外扩值过大可能超出父容器边界；与 `Clip` 或阴影组合时要检查视觉裁剪。

## 示例代码

<!--@include: ./_gen/borderbeam-examples.md-->

## API

<!--@include: ./_gen/borderbeam-api.md-->

## 相关链接

- 源码：[ui/fill_border_beam.go](https://github.com/tangoui/tango/blob/main/ui/fill_border_beam.go)
- [ui Go API](/reference/api/ui)
- 相关组件：[Card 卡片](/components/card)、[Button 按钮](/components/button)
