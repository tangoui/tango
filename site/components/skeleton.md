---
title: Skeleton 骨架屏
---
# Skeleton 骨架屏

用接近真实内容的占位形状表示加载中的页面，减少内容跳动和等待感。

## 何时使用

- 首次加载或切换内容时，已知内容的大致结构但数据尚未返回。
- 卡片、文章、用户资料等需要保持布局高度的区域。
- 只需要一个旋转指示时用 [Spin](/components/spin)；数据为空时用 [Empty](/components/empty)。

## 在线体验

<GalleryPreview component="Skeleton" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

func skeletonPage(c *ui.Context) {
    ui.Skeleton(c).Rows(3).Active().Show()
}
```

`Active()` 开启 shimmer 动画，`Rows(n)` 设置段落行数。`Show()` 构建占位内容。

## 常见用法

### 头像、标题和段落

```go
func skeletonProfile(c *ui.Context) {
    ui.Skeleton(c).Avatar(false).Rows(3).Round().Show()
}
```

`Avatar(false)` 添加圆形头像，传入 `true` 使用方形头像；`Round()` 让条带使用胶囊形状。

### 加载完成后切换真实内容

```go
var profileLoading = true

func skeletonLoading(c *ui.Context) {
    ui.Skeleton(c).Loading(profileLoading).Body(func() {
        ui.Text(c, "用户资料已加载")
    })
}
```

`Loading(true)` 显示骨架，`Loading(false)` 构建 `Body` 中的真实内容。把加载状态放在页面级变量或 `ui.State`，不要在渲染函数中重置。

### 控件占位

```go
func skeletonControls(c *ui.Context) {
    ui.SkeletonButton(c, ui.Middle, "default", true)
    ui.SkeletonButton(c, ui.Middle, "round", true)
    ui.SkeletonInput(c, ui.Middle, true)
    ui.SkeletonAvatar(c, 32, false, true)
    ui.SkeletonImage(c, 96, 96, false)
}
```

四个专用构造函数分别提供按钮、输入框、头像和图片形状；`active` 控制动画，尺寸使用 `ui.Small`、`ui.Middle` 或 `ui.Large`。

### 隐藏标题和段落

```go
func skeletonMinimal(c *ui.Context) {
    ui.Skeleton(c).NoTitle().Rows(0).Show()
}
```

`NoTitle()` 隐藏标题条，`Rows(0)` 隐藏段落条，适合只需要局部占位的场景。

## 主题与样式

Skeleton 的条带高度、圆角、动画颜色和间距由主题控制。`ConfigProvider.Defaults("Skeleton", ...)` 支持 `active` 与 `round` 布尔默认值；单个实例的 `Active`、`Round` 优先。大量同时动画会增加绘制开销，应按可见区域拆分骨架。

## 注意事项

骨架形状应尽量匹配最终内容，避免加载完成后大幅跳动。加载状态来自真实请求状态；不要用固定延时伪造完成。动画应避免覆盖错误和空状态，必要时提供可读的加载说明或配合 A11y 文案。大列表优先使用虚拟化或只渲染视口附近的骨架。

## 示例代码

<!--@include: ./_gen/skeleton-examples.md-->

## API

<!--@include: ./_gen/skeleton-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/data_skeleton.go)
- [ui Go API](/reference/api/ui)
- [Spin 加载中](/components/spin)
