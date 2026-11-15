---
title: Anchor 锚点
---
# Anchor 锚点

在页面或滚动容器内显示章节目录，点击跳转并随滚动高亮当前章节。

## 何时使用

- 文档、设置页、长表单需要快速跳转到不同章节。
- 内容在独立 ScrollBox 中滚动，并希望目录同步高亮。
- 页面级菜单或应用路由使用 [Menu 导航菜单](/components/menu)；平级视图切换使用 [Tabs 标签页](/components/tabs)。
- 需要固定目录或工具条时，可与 [Affix 固钉](/components/affix) 一起使用。

## 在线体验

<GalleryPreview component="Anchor" />

## 基本用法

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var anchorTarget ui.ID

var anchorItems = []ui.AnchorItem{
    {Key: "intro", Title: "简介"},
    {Key: "usage", Title: "用法"},
}

func page(c *ui.Context) {
    ui.Row(c, func() {
        ui.Anchor(c, anchorItems).Target(anchorTarget).Show()
        scroll := ui.Scroll(c, func() {
            ui.AnchorSection(c, "intro", func() { ui.Title(c, "简介", 4) })
            ui.AnchorSection(c, "usage", func() { ui.Title(c, "用法", 4) })
        }, layout.H(320), layout.Grow(1))
        anchorTarget = scroll.ID()
    }, layout.Gap(24))
}
```

Anchor 链接的 `Key` 必须与 `AnchorSection` 的 key 一致；`Target` 指向承载这些章节的滚动容器。

## 常见用法

### 多级目录

`AnchorItem.Children` 可表达二级目录，子项同样对应页面中的 `AnchorSection`。

```go
import "github.com/tangoui/tango/ui"

var items = []ui.AnchorItem{
    {Key: "basic", Title: "基础"},
    {Key: "advanced", Title: "进阶", Children: []ui.AnchorItem{{Key: "offset", Title: "偏移量"}}},
}

func page(c *ui.Context) {
    ui.Anchor(c, items).Show()
}
```

### 横向锚点

`Horizontal()` 把链接排成一行，适合页面顶部的章节切换；`NoInk()` 隐藏指示条。

```go
import "github.com/tangoui/tango/ui"

var items = []ui.AnchorItem{{Key: "overview", Title: "概览"}, {Key: "api", Title: "API"}}

func page(c *ui.Context) {
    ui.Anchor(c, items).Horizontal().NoInk().Show()
}
```

### 固定标题偏移

如果滚动容器顶部有固定标题，用 `TargetOffset` 让跳转与高亮判断留出标题高度。

```go
import "github.com/tangoui/tango/ui"

var target ui.ID
var items = []ui.AnchorItem{{Key: "section", Title: "章节"}}

func page(c *ui.Context) {
    ui.Anchor(c, items).Target(target).TargetOffset(56).Show()
}
```

### 监听当前章节

`Active()` 返回当前高亮 key，`Clicked()` 返回本帧点击的 key，可用于同步地址栏或统计。

```go
import "github.com/tangoui/tango/ui"

var items = []ui.AnchorItem{{Key: "a", Title: "A"}, {Key: "b", Title: "B"}}
var lastClick string

func page(c *ui.Context) {
    anchor := ui.Anchor(c, items).Show()
    if key := anchor.Clicked(); key != "" {
        lastClick = key
    }
    ui.Text(c, "当前："+anchor.Active()+"，最近点击："+lastClick)
}
```

## 主题与样式

Anchor 的链接颜色、激活颜色、指示条和间距由 `theme.Components.Anchor` 控制。Ant Design 默认使用竖向指示条，Element Plus 可通过 `NoInk()` 获得无下划线样式，shadcn 使用更低对比度的文本和细线。

## 注意事项

- `AnchorSection` key 必须唯一且与目录项一致；动态内容变化时也要保持 key 稳定。
- `Target` 使用滚动容器 ID；ID 在当前帧创建后才知道，通常保存到页面状态，下一帧再传给 Anchor。
- 章节非常多时，目录应保持层级简短，避免把每一个小段落都放入 Anchor。
- 链接可通过键盘聚焦和激活；章节标题应使用语义清晰的文本。
- RTL 下竖向指示条与横向排列会自动镜像。

## 示例代码

<!--@include: ./_gen/anchor-examples.md-->

## API

<!--@include: ./_gen/anchor-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/nav_affix.go)
- [ui Go API](/reference/api/ui)
- [Affix 固钉](/components/affix)
