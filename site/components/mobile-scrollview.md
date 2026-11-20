---
title: Mobile ScrollView 滚动视图
---
# Mobile ScrollView 滚动视图

提供触摸滚动、惯性、回弹、下拉刷新、上拉加载和定高行虚拟构建的移动端滚动容器。

## 何时使用

- 页面内容超出可视区域时使用 `ScrollView`。
- 下拉刷新使用 `PullToRefresh`，分页加载使用 `InfiniteList`。
- 行高固定且数量很大时使用 `Rows`，让组件只构建可见行。
- 横向轮播、滚轮或可吸附控件应使用 `Swiper`、`PickerView`，不要把 ScrollView 当作通用分页器。

## 在线体验

真实 gallery 同时展示下拉刷新和上拉加载，拖动列表观察状态变化。

<GalleryPreview component="mobile.ScrollView" />

## 基本用法

ScrollView 的内容通过 `Body` 构建；在即时模式下，滚动位置由组件按稳定 ID 保存。

```go
import (
    "fmt"
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
    "github.com/tangoui/tango/ui/mobile"
)

func page(c *ui.Context) {
    mobile.ScrollView(c).Layout(layout.Grow(1)).Body(func() {
        mobile.List(c, "消息").Body(func() {
            for i := 0; i < 30; i++ {
                i := i
                ui.Key(c, i, func() {
                    mobile.Cell(c, fmt.Sprintf("消息 %d", i+1))
                })
            }
        })
    })
}
```

## 常见用法

### 下拉刷新

用户下拉超过阈值并松手时，组件将 `refreshing` 设为 `true`，`Refreshed()` 在这一帧返回 `true`。用它启动刷新，数据加载完成后将 `refreshing` 设回 `false`。

```go
var refreshing bool

func refreshPage(c *ui.Context, startRefresh func()) {
    w := mobile.PullToRefresh(c, &refreshing).Layout(layout.Grow(1))
    w.Body(func() { mobile.List(c, "").Body(func() { mobile.Cell(c, "最新内容") }) })
    if w.Refreshed() {
        startRefresh() // 数据更新到 UI 线程后将 refreshing 设回 false。
    }
}
```

### 无限加载

列表进入末端阈值且可以加载下一批时，组件将 `loading` 设为 `true`，并在这一帧通过 `LoadMore()` 报告。追加数据后将 `loading` 设回 `false`，全部加载完时设置 `Finished(true)`。`Offset(px)` 调整触发距离，默认 50px。

```go
var loading bool

func infinitePage(c *ui.Context, startLoad func()) {
    w := mobile.InfiniteList(c, &loading).Layout(layout.Grow(1))
    w.Body(func() { mobile.List(c, "更多").Body(func() { mobile.Cell(c, "列表项") }) })
    if w.LoadMore() {
        startLoad() // 在 UI 线程追加数据并将 loading 设回 false。
    }
}
```

### 定高行

`Rows` 只构建可视窗口附近的行，内置键为下标。数据会插入或重排时，可在回调中为行内控件设置 `ui.Key`。

```go
func virtualPage(c *ui.Context) {
    mobile.ScrollView(c).Layout(layout.Grow(1)).Rows(10000, 48, func(i int) {
        mobile.Cell(c, fmt.Sprintf("第 %d 行", i+1))
    })
}
```

### 程序控制滚动

用 `ScrollOffset` 读取当前位置，用 `ScrollTo` 跳到指定偏移；不要通过重建内容来模拟滚动。

```go
func controlled(c *ui.Context, top bool) {
    w := mobile.ScrollView(c).Body(func() { mobile.Cell(c, "内容") })
    if top {
        w.ScrollTo(0)
    }
    _ = w.ScrollOffset()
}
```

## 注意事项

滚动容器和内容的布局必须给出可用高度，页面常用 `layout.Grow(1)`。刷新和加载的 bool 指针放在稳定状态中；网络请求不要阻塞 UI 构建。大数据量使用 `Rows` 或分页，不要在每帧为数万行创建控件。滚动容器中的交互控件依赖手势仲裁，避免再套一层自定义垂直 Area。

## 示例代码

<!--@include: ./_gen/mobile-scrollview-examples.md-->

## API

<!--@include: ./_gen/mobile-scrollview-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/mobile/scrollview.go)
- [Scroller 滚动物理](/components/mobile-scroller)
- [List 列表](/components/mobile-list)
- [ui mobile Go API](/reference/api/ui-mobile)
