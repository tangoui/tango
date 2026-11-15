---
title: Affix 固钉
---
# Affix 固钉

当内容滚动到指定位置时把它固定在窗口或滚动容器中，适合保持工具条可见。

## 何时使用

- 长页面中的操作工具条、筛选栏需要在滚动后保持可见。
- 页面在一个独立 ScrollBox 中滚动时，可用 `Target` 固定在该容器内。
- 只需要一个可滚动侧栏或固定顶部布局时，优先用 [Layout 布局](/components/layout)。
- 页面目录与章节跳转使用 [Anchor 锚点](/components/anchor)。

## 在线体验

<GalleryPreview component="Affix" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    ui.Affix(c).OffsetTop(0).Body(func() {
        ui.Button(c, "保存").Primary()
    })
}
```

默认以窗口为视口，内容滚过顶部偏移量后固定。

## 常见用法

### 在滚动容器中固定

内容在 ScrollBox 内滚动时，把 `scroll.ID()` 传给 `Target`。目标 ID 在当前帧创建容器后才能取得，因此示例用状态保存上一帧的 ID。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var affixTarget ui.ID

func page(c *ui.Context) {
    scroll := ui.Scroll(c, func() {
        ui.Affix(c).OffsetTop(0).Target(affixTarget).Body(func() {
            ui.Button(c, "保存").Primary()
        })
        for i := 0; i < 20; i++ {
            ui.Paragraph(c, "滚动内容")
        }
    }, layout.H(240))
    affixTarget = scroll.ID()
}
```

### 固定到底部

`OffsetBottom` 让内容在接近视口底部时固定，例如保存栏。

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    ui.Affix(c).OffsetBottom(16).Body(func() {
        ui.Button(c, "提交").Primary()
    })
}
```

### 读取固定状态

`Affixed()` 表示当前是否固定，`Changed()` 表示上一帧检测到固定状态变化。

```go
import "github.com/tangoui/tango/ui"

var affixMessage = "未固定"

func page(c *ui.Context) {
    affix := ui.Affix(c).OffsetTop(8).Body(func() { ui.Text(c, "工具条") })
    if affix.Changed() {
        if affix.Affixed() {
            affixMessage = "已固定"
        } else {
            affixMessage = "未固定"
        }
    }
    ui.Text(c, affixMessage)
}
```

## 主题与样式

Affix 只负责定位，不绘制默认背景；固定后的工具条应在 Body 内设置背景、边框、阴影或圆角。`ConfigProvider.Defaults("Affix", ui.Props{"offsetTop": 8})` 可设置默认顶部偏移。

## 注意事项

- 页面不是窗口滚动而是容器滚动时，必须设置 `Target`，否则偏移计算会以窗口为准。
- 目标容器 ID 需要稳定保存；不要在每帧用新值覆盖不同容器，否则固定状态会跳动。
- 固定内容不要遮住焦点元素；如顶部还有标题栏，给 `OffsetTop` 留出空间。
- 固定工具条里的图标按钮仍需要 `A11yLabel`，键盘顺序不因固定而改变。
- RTL 只改变内容的方向，不改变上下偏移语义。

## 示例代码

<!--@include: ./_gen/affix-examples.md-->

## API

<!--@include: ./_gen/affix-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/nav_affix.go)
- [ui Go API](/reference/api/ui)
- [Anchor 锚点](/components/anchor)
