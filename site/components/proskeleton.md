---
title: ProSkeleton 页面骨架屏
---
# ProSkeleton 页面骨架屏

按 Pro 页面结构生成页头、统计卡、工具栏、列表、结果页或详情页的加载占位布局。

## 何时使用

- 页面首次加载时，真实内容结构已知但接口数据尚未返回。
- 需要让列表页、结果页或详情页在加载过程中保持稳定布局时。
- 只需一个局部文本或头像占位时用 [Skeleton 骨架屏](/components/skeleton)；只需忙碌指示时用 [Spin 加载中](/components/spin)。

## 在线体验

示例包含列表页、结果页和详情页三种 Pro 页面骨架布局。

<GalleryPreview component="ProSkeleton" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    ui.ProSkeleton(c, ui.SkeletonList).Show()
}
```

`ProSkeleton` 接受 `SkeletonList`、`SkeletonResult` 或 `SkeletonDescriptions`，列表类型默认显示活动动画、页头、2 个统计占位、工具栏和 5 行列表。最后调用 `Show()` 构建页面骨架。

## 常见用法

以下片段省略了 `import`。

### 列表页骨架

列表类型支持调整统计占位和列表行数，也可以隐藏工具栏或页头。`Active(false)` 适合截图、测试或用户偏好减少动画的场景。

```go
func listSkeleton(c *ui.Context) {
    ui.ProSkeleton(c, ui.SkeletonList).
        Statistic(3).List(6).Toolbar(true).PageHeader(true).Active(true).Show()
}
```

### 结果页和详情页

`SkeletonResult` 生成居中的结果图标、标题、描述和操作占位；`SkeletonDescriptions` 生成详情信息和表格样式占位。两者都可以用 `PageHeader(false)` 去掉页头。

```go
func resultAndDetails(c *ui.Context) {
    ui.ProSkeleton(c, ui.SkeletonResult).PageHeader(false).Show()
    ui.ProSkeleton(c, ui.SkeletonDescriptions).Active(false).Show()
}
```

### 自定义布局尺寸

`Layout` 把布局选项应用到外层骨架。页面容器负责宽度和增长，骨架本身只负责内部页面结构。

```go
func sizedSkeleton(c *ui.Context) {
    ui.ProSkeleton(c, ui.SkeletonList).
        Statistic(4).List(4).Layout(layout.W(960), layout.Grow(1)).Show()
}
```

### 全局默认值

可以在配置提供器中设置页面级默认值，实例方法仍可以覆盖它们。整数键 `statistic` 和 `list` 分别控制对应占位数量。

```go
func skeletonDefaults(c *ui.Context) {
    ui.ConfigProvider(c).Defaults("ProSkeleton", ui.Props{
        "active": true, "pageHeader": true, "toolbar": false,
        "statistic": 3, "list": 5,
    }).Body(func() {
        ui.ProSkeleton(c, ui.SkeletonList).Show()
    })
}
```

## 主题与样式

ProSkeleton 的卡片表面、圆角、内边距、分隔线和骨架动画颜色跟随 ProCard 与 Skeleton 的主题。列表页会根据 Ant Design、Element Plus、shadcn 的卡片和间距基线调整；`Active` 只控制 shimmer，不改变占位结构。

`ConfigProvider.Defaults("ProSkeleton", ...)` 支持 `"active"`、`"pageHeader"`、`"toolbar"`（布尔值）以及 `"statistic"`、`"list"`（数量）。实例上的同名方法优先。

## 注意事项

- **加载状态**：骨架应由真实请求状态控制，数据到达后切换到真实页面，不要用固定延时伪造完成。
- **状态放在哪**：请求状态和页面数据放在页面变量或 `ui.State` 中；骨架组件本身不保存你的请求状态。
- **结构匹配**：列表行数、卡片宽度和页头层级应尽量接近真实内容，避免切换时页面跳动。
- **性能**：只为当前可见页面构建骨架；很多卡片同时 shimmer 会增加动画绘制开销，必要时关闭 `Active`。
- **无障碍**：外层使用忙碌状态和加载语义；加载失败、空数据和真正的错误状态不应一直显示成骨架。
- **RTL**：页头、统计区、列表行和详情占位遵循当前 RTL 布局。

## 示例代码

<!--@include: ./_gen/proskeleton-examples.md-->

## API

<!--@include: ./_gen/proskeleton-api.md-->

## 相关链接

- 源码：[ui/pro2_skeleton.go](https://github.com/tangoui/tango/blob/main/ui/pro2_skeleton.go)
- [ui Go API 参考](/reference/api/ui)
- [Skeleton 骨架屏](/components/skeleton)、[Spin 加载中](/components/spin)、[ProCard 高级卡片](/components/procard)
- [组件总览](/components/)、[组件截图目录](/components/screenshots)
