---
title: Empty 空状态
---
# Empty 空状态

在列表、表格或容器没有可展示内容时，提供明确的空状态说明和下一步操作。

## 何时使用

- 查询结果为空、用户尚未创建数据或数据暂时不可用时使用。
- 需要引导用户创建第一条数据时，在 `Body` 中放置主要操作。
- 正在加载时用 [Skeleton](/components/skeleton) 或 [Spin](/components/spin)；发生错误时用 [Result](/components/result)。

## 在线体验

<GalleryPreview component="Empty" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

func emptyPage(c *ui.Context) {
    ui.Empty(c).Description("暂无订单").Body(func() {
        ui.Button(c, "立即创建").Primary()
    })
}
```

默认显示空状态插画和本地化的无数据文案。`Description("")` 可隐藏说明文字。

## 常见用法

### 紧凑空状态

```go
func emptySimple(c *ui.Context) {
    ui.Empty(c).Simple().Description("暂无筛选结果")
}
```

`Simple()` 使用紧凑插画和间距，适合 Select、Table、List 等嵌入空间。

### 自定义尺寸

```go
func emptySize(c *ui.Context) {
    ui.Empty(c).ImageSize(60).Description("暂无图片")
}
```

`ImageSize` 设置图片高度；Ant 默认插画约 100，Simple 约 40，Element 约 160。

### 自定义图片与操作

```go
func emptyCustom(c *ui.Context) {
    ui.Empty(c).Image(func() {
        ui.Text(c, "○")
    }).Description("还没有项目").Body(func() {
        ui.Button(c, "新建项目").Primary()
    })
}
```

`Image` 替换默认图片内容；`Body` 回调中构建按钮或其他轻量操作。

`Title(s)` 在说明文字上方显示主标题，`Description(s)` 可提供补充信息。

### 放入布局

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func emptyLayout(c *ui.Context) {
    ui.Empty(c).Layout(layout.Grow(1)).Description("暂无消息")
}
```

`Layout` 让空状态参与外层布局；不要用固定高度模拟垂直居中。

## 主题与样式

Empty 的插画、颜色、间距和字体随主题切换。`Simple` 是跨 Ant Design、Element Plus、shadcn 的紧凑语义；如需自定义插画优先使用 `Image`，不要把主题颜色硬编码到页面。

## 注意事项

要区分“确实为空”“尚未加载”和“加载失败”，不要在三种状态间复用同一文案。查询条件变化时，空状态应由当前结果状态驱动。空状态操作应能通过键盘访问，并用文字说明动作；RTL 布局由主题和布局系统处理。

## 示例代码

<!--@include: ./_gen/empty-examples.md-->

## API

<!--@include: ./_gen/empty-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/data_result.go)
- [ui Go API](/reference/api/ui)
- [Skeleton 骨架屏](/components/skeleton)
