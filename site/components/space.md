---
title: Space 间距
---
# Space 间距

为一组相邻内容提供统一间距，常用于按钮、链接和标签等内联控件。

## 何时使用

- 一组按钮、链接、标签或小型控件需要统一水平或垂直间距。
- 需要在相邻项之间插入分隔符时使用 `Split`。
- 输入框和按钮需要拼成一个整体时使用 [Compact 紧凑布局](/components/compact)。
- 需要 flex 主轴分配、两端对齐或复杂排版时使用 [Flex 弹性布局](/components/flex)。

## 在线体验

<GalleryPreview component="Space" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    ui.Space(c).Body(func() {
        ui.Button(c, "新建").Primary()
        ui.Button(c, "导入")
        ui.Link(c, "查看帮助")
    })
}
```

Space 默认横向排列、使用 Small（8px）间距，并在交叉轴居中。

## 常见用法

### 尺寸与竖排

`Size` 使用设计令牌；`Vertical()` 竖向排列，适合紧凑的操作列表。

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    ui.Space(c).Vertical().Size(ui.Middle).Body(func() {
        ui.Button(c, "编辑")
        ui.Button(c, "复制")
        ui.Button(c, "删除").Danger()
    })
}
```

### 分隔符

`Split` 会在每两个子项之间渲染分隔内容，不会在开头或末尾额外添加。

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    ui.Space(c).Split(func() { ui.VDivider(c) }).Body(func() {
        ui.Link(c, "编辑")
        ui.Link(c, "复制")
        ui.Link(c, "删除")
    })
}
```

### 换行与像素间距

`Wrap()` 只用于横向 Space；`SizePx(h, v)` 分别设置水平与换行后的垂直间距。

```go
import (
    "fmt"
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func page(c *ui.Context) {
    ui.Space(c).Wrap().SizePx(8, 12).Layout(layout.MaxW(520)).Body(func() {
        for i := 0; i < 10; i++ {
            ui.Button(c, fmt.Sprintf("按钮 %d", i+1))
        }
    })
}
```

## 主题与样式

Space 不绘制背景和边框，只控制间距；尺寸令牌为 `Small=8`、`Middle=16`、`Large=24`。可以用 `ConfigProvider.Defaults("Space", ui.Props{"size": ui.Large})` 修改某个作用域内的默认间距。

## 注意事项

- Space 适合内联项；包含大块卡片或二维布局时，用 Grid 或 Flex 更容易控制伸缩。
- `Wrap()` 需要宽度约束，例如 `Layout(layout.MaxW(...))`，否则可能按内容宽度扩展。
- Split 的分隔符只是视觉分隔；纯装饰分隔不要承载交互。
- 键盘顺序与子项顺序一致；图标按钮需要 `A11yLabel`。
- RTL 下横向顺序和间距方向会自动镜像。

## 示例代码

<!--@include: ./_gen/space-examples.md-->

## API

<!--@include: ./_gen/space-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/nav_space.go)
- [ui Go API](/reference/api/ui)
- [Compact 紧凑布局](/components/compact)
