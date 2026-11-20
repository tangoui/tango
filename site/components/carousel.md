---
title: Carousel 走马灯
---
# Carousel 走马灯

在有限区域内循环展示多张内容，支持自动播放、指示器、箭头、淡入淡出、垂直切换和拖拽滑动。

## 何时使用

- 首页横幅、产品截图、宣传内容等需要按页浏览的内容。
- 需要把多张图片或多组信息放在同一个固定高度区域时。
- 需要同时比较多项内容时用 [Card 卡片](/components/card) 或普通布局容器；需要连续滚动的信息流时用 [InfiniteScroll 无限滚动](/components/infinitescroll)。

## 在线体验

示例包含自动播放、箭头、淡入淡出、垂直指示器、外置指示器、拖拽和不循环模式。

<GalleryPreview component="Carousel" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
    "fmt"

    "github.com/tangoui/tango/ui"
)

func page(c *ui.Context) {
    slide := func(i int) {
        ui.Text(c, fmt.Sprintf("第 %d 张", i+1)).Size(24)
    }
    ui.Carousel(c, 4, slide).Height(200).Show()
}
```

`Carousel(c, n, slide)` 声明 `n` 张幻灯片，`slide(i)` 构建第 `i` 张内容。组件默认使用滑动过渡和底部指示器，首尾可以循环切换。用 `Autoplay` 设置自动播放间隔，`Height` 设置幻灯片区域高度。

## 常见用法

以下片段省略了 `import`。

### 自动播放与受控当前页

`Current` 把当前页索引绑定到应用变量，`Autoplay` 设置播放间隔，传入 `0` 可关闭自动播放。`Changed()` 在 `Show` 后报告本帧是否换页。

```go
var carouselPage int

func autoplayCarousel(c *ui.Context) {
    carousel := ui.Carousel(c, 4, func(i int) {
        ui.Text(c, fmt.Sprintf("内容 %d", i+1))
    }).Current(&carouselPage).Autoplay(3 * time.Second)
    carousel.Show()
    if carousel.Changed() {
        ui.Text(c, fmt.Sprintf("当前第 %d 张", carouselPage+1)).Tertiary()
    }
}
```

### 箭头、淡入淡出与指示器

`Arrows` 支持 `always`、`hover`、`never`。`Fade` 将滑动切换改为交叉淡入淡出，`Dots(false)` 隐藏指示器。`DotPlacement` 支持 `top`、`bottom`、`start`、`end`，其中 `start` 和 `end` 还会切换到竖向轮播。

```go
func carouselVariants(c *ui.Context) {
    ui.Row(c, func() {
        ui.Carousel(c, 4, func(i int) { ui.Text(c, fmt.Sprint(i+1)) }).
            Arrows("always").Height(180).Show()
        ui.Carousel(c, 4, func(i int) { ui.Text(c, fmt.Sprint(i+1)) }).
            Fade().Arrows("always").Dots(true).Height(180).Show()
    }, layout.Gap(16))
}
```

### 垂直和外置指示器

`Vertical` 让内容上下切换，并把指示器放到末端。`DotOutside()` 把指示器放在框架之外，适合图片本身覆盖整个画面。

```go
func verticalCarousel(c *ui.Context) {
    ui.Carousel(c, 4, func(i int) {
        ui.Text(c, fmt.Sprintf("场景 %d", i+1))
    }).Vertical().Height(180).Show()
    ui.Carousel(c, 4, func(i int) {
        ui.Text(c, fmt.Sprintf("图片 %d", i+1))
    }).DotOutside().Height(180).Show()
}
```

### 拖拽、滑动与不循环

`Draggable(true)` 支持鼠标拖拽和触摸滑动，默认移动超过幻灯片 20% 才换页；`SwipeThreshold` 可调整比例。`Loop(false)` 在首尾停止，箭头和自动播放不会越界。

```go
func draggableCarousel(c *ui.Context) {
    ui.Carousel(c, 4, func(i int) {
        ui.Text(c, fmt.Sprintf("可拖拽第 %d 张", i+1))
    }).Draggable(true).SwipeThreshold(0.3).Loop(false).
        Arrows("always").Height(180).Show()
}
```

### 卡片模式

`Card` 显示中间的大卡片和两侧缩小、变暗的相邻卡片；点击侧边卡片可以切换。它适合展示封面或商品卡片，不适合承载需要同时交互的复杂表单。

```go
func cardCarousel(c *ui.Context) {
    ui.Carousel(c, 5, func(i int) {
        ui.Text(c, fmt.Sprintf("商品 %d", i+1)).Strong()
    }).Card().Height(220).Show()
}
```

## 主题与样式

Ant Design 默认是底部指示器和平滑滑动；Element Plus 在箭头悬停、圆点尺寸和卡片模式上更接近其 Carousel；shadcn 使用卡片化幻灯片和更明显的焦点环。组件令牌在 `theme.Components.Carousel`（`theme.CarouselToken`），包括圆点、箭头、圆角和卡片遮罩等视觉参数。

`ConfigProvider.Defaults("Carousel", ...)` 支持 `"autoplay"`（`time.Duration`）、`"dots"`、`"arrows"`、`"dotPlacement"`、`"type"`（`card`）、`"effect"`（`fade`）、`"height"`（`float32`）、`"trigger"`（`click` 或 `hover`）和 `"pauseOnHover"`。旧键 `"dotPosition"` 仍可使用。实例方法优先于默认值。

## 注意事项

- **状态放在哪**：当前页索引绑定到 `Current` 的应用变量；自动播放由组件按帧推进，页面不要在每帧重置索引。
- **可访问性**：指示器是可聚焦的 Tab 列表，箭头可用键盘操作；隐藏指示器时，框架本身接收方向键。每张幻灯片应有能说明内容的标题或文本。
- **自动播放**。默认在悬停、聚焦或拖拽时暂停。`PauseOnHover(false)` 可让悬停时继续播放。用户开启减少动效偏好时自动播放关闭。
- **性能**：普通滑动主要构建当前页及相邻页；淡入淡出和卡片模式可能同时构建多张，幻灯片内容应保持轻量。
- **RTL**：水平上一张/下一张方向和箭头位置自动镜像，垂直模式仍使用上下方向。

## 示例代码

<!--@include: ./_gen/carousel-examples.md-->

## API

<!--@include: ./_gen/carousel-api.md-->

## 相关链接

- 源码：[ui/data_carousel.go](https://github.com/tangoui/tango/blob/main/ui/data_carousel.go)
- [ui Go API 参考](/reference/api/ui)
- [Image 图片](/components/image)、[Card 卡片](/components/card)、[InfiniteScroll 无限滚动](/components/infinitescroll)
- [组件总览](/components/)、[组件截图目录](/components/screenshots)
