---
title: Mobile Area 手势区域
---
# Mobile Area 手势区域

把一个 `ui.Box` 注册为可仲裁的触摸手势区域，统一处理拖拽、点按、长按和滑动。

## 何时使用

- 需要自定义横向、纵向或双向拖拽交互时使用 `Area`。
- 需要让区域中的按钮仍能收到点按和长按时，用 `Within` 绑定子控件。
- 只需要标准点击反馈时优先使用 `mobile.Button`、`Cell` 或 `Touchable`，不要自己实现手势仲裁。
- 需要双指缩放时使用 `Gesture.Pinch`，不要把两个单指 Area 拼成缩放逻辑。

## 在线体验

拖动下方真实 gallery 的区域，或点按、长按其中的控件，观察手势状态。

<GalleryPreview component="mobile.Area" />

## 基本用法

`Area` 接收一个已经创建的盒子；内容应放进 `Within`，返回值在当前帧报告手势状态。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/geom"
    "github.com/tangoui/tango/ui/layout"
    "github.com/tangoui/tango/ui/mobile"
)

var offset float32

func page(c *ui.Context) {
    area := ui.Div(c, layout.H(120)).Clip()
    gesture := mobile.Area(c, area, mobile.AxisHorizontal, nil)
    if gesture.Pan.Active {
        offset = gesture.Pan.Delta.X
    }
    area.Body(func() {
        mobile.Within(c, area.ID(), func() {
            mobile.Button(c, "拖动或点按").Primary()
        })
    })
    area.Transform(geom.Translate(offset, 0))
}
```

## 常见用法

### 只接受某个方向

`AxisHorizontal`、`AxisVertical` 和 `AxisBoth` 决定区域可接管的拖拽方向。

```go
func horizontal(c *ui.Context) {
    b := ui.Div(c, layout.H(96))
    g := mobile.Area(c, b, mobile.AxisHorizontal, func(d mobile.Direction) bool {
        return d.Horizontal()
    })
    _ = g
}
```

### 绑定可点击子控件

`Within` 让区域内的 `Touchable` 参与同一个手势 arena；手指未形成拖拽时仍会产生 Tap。

```go
func nestedTouch(c *ui.Context) {
    b := ui.Div(c, layout.Size(240, 80))
    mobile.Area(c, b, mobile.AxisHorizontal, nil)
    b.Body(func() {
        mobile.Within(c, b.ID(), func() {
            if mobile.Button(c, "操作").Clicked() {
                // 处理点按
            }
        })
    })
}
```

### 读取长按和滑动

`LongPress` 与 `Swiped` 只在对应事件发生的帧为真，适合在该帧更新页面状态。

```go
func classify(c *ui.Context) {
    b := ui.Div(c, layout.H(100))
    g := mobile.Area(c, b, mobile.AxisBoth, nil)
    if g.LongPress {
        ui.Text(c, "长按")
    }
    if g.Swiped {
        ui.Text(c, "滑动：" + g.Swipe.Dir.String())
    }
}
```

### 为自绘盒子注册 Touchable

需要自定义外观时，可以在盒子上调用 `Touchable`，并用返回的 `Press` 控制按下态。

```go
func customTouchable(c *ui.Context) {
    b := ui.Div(c, layout.Size(120, 44), layout.Center())
    press := mobile.Touchable(c, b)
    if press.Pressed {
        b.Opacity(0.7)
    }
    b.Body(func() { ui.Text(c, "自绘操作") })
}
```

## 注意事项

手势状态是逐帧值，不要把 `Pan.Delta` 当成累计位移；需要持久位置时在页面状态中累加。`Within` 的 id 必须来自当前 Area 的盒子。区域会优先接管拖拽，标准控件的 Enter、Space 和屏幕阅读器 Press 仍通过 `Touchable` 提供可访问点击。RTL 下方向由 ui 方向作用域解释；不要用物理左、右自行反转业务语义。

## 示例代码

<!--@include: ./_gen/mobile-area-examples.md-->

## API

<!--@include: ./_gen/mobile-area-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/mobile/arena.go)
- [手势识别源码](https://github.com/tangoui/tango/blob/main/ui/mobile/recognizer.go)
- [ScrollView 滚动视图](/components/mobile-scrollview)
- [ui mobile Go API](/reference/api/ui-mobile)
