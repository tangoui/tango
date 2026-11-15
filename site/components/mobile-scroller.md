---
title: Mobile Scroller 滚动物理
---
# Mobile Scroller 滚动物理

提供独立于绘制的单轴滚动物理，支持拖拽、惯性、回弹和吸附。

## 何时使用

- 自定义滚动容器、轮盘或分页吸附时使用 `mobile.Scroller`。
- 只需要滚动页面内容时使用更高层的 `mobile.ScrollView`。
- 行高固定且数量很大时使用 `ScrollViewW.Rows`，不要手动为所有行创建盒子。
- 组件需要手势接管时，先用 `Area` 或现成滚动组件，不要在 UI 回调里直接改位置而跳过物理状态。

## 在线体验

下方真实 gallery 展示下拉刷新和上拉加载；它们内部使用 Scroller 的惯性和边缘处理。

<GalleryPreview component="mobile.Scroller" />

## 基本用法

`Scroller` 是纯状态类型：调用方传入当前时间给 `Step`，再读取 `Pos`。

```go
import (
    "time"
    "github.com/tangoui/tango/ui/mobile"
)

func physics() float64 {
    s := mobile.Scroller{Min: 0, Max: 720, Dim: 360}
    s.BeginDrag()
    s.DragBy(80)
    s.Release(-240, time.Now())
    s.Settle()
    return s.Pos
}
```

## 常见用法

### 拖拽与释放

拖拽开始前调用 `BeginDrag`，每次手指移动调用 `DragBy`，抬手时把速度交给 `Release`。

```go
func drag(s *mobile.Scroller, now time.Time) {
    s.BeginDrag()
    s.DragBy(32)
    s.Release(-180, now)
}
```

### 逐帧推进惯性

在页面刷新循环中用同一个时钟持续调用 `Step`，返回 false 时表示已经停止。

```go
func tick(s *mobile.Scroller, now time.Time) bool {
    return s.Step(now)
}
```

### 禁用回弹

`NoBounce` 让边缘位置直接夹紧，适合不希望内容越过边界的自定义滚动器。

```go
func clamped() float64 {
    s := mobile.Scroller{Min: 0, Max: 300, Dim: 200, NoBounce: true}
    s.BeginDrag()
    s.DragBy(-500)
    s.Settle()
    return s.Pos
}
```

### 吸附到固定间距

`Snap` 表示从 `Min` 开始的吸附间距，适合分页和滚轮；需要明确目标时可调用 `SnapTo`。

```go
func snapped(s *mobile.Scroller, now time.Time) {
    s.Snap = 48
    s.SnapTo(96, now)
}
```

## 注意事项

`Pos` 是滚动偏移，不是屏幕坐标；内容向上或向左移动时偏移增大。每个滚动物理实例应放在稳定的页面状态中，不能每帧重新初始化，否则惯性会被清零。调用 `Step` 时使用单调递增时间。触摸事件、可见行裁剪和布局请优先交给 `ScrollView`。

## 示例代码

<!--@include: ./_gen/mobile-scroller-examples.md-->

## API

<!--@include: ./_gen/mobile-scroller-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/mobile/physics.go)
- [ScrollView 滚动视图](/components/mobile-scrollview)
- [PickerView 选择器视图](/components/mobile-pickerview)
- [ui mobile Go API](/reference/api/ui-mobile)
