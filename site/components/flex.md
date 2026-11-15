---
title: Flex 弹性布局
---
# Flex 弹性布局

用一维 flex 容器排列内容，提供方向、间距、对齐、分布和换行能力。

## 何时使用

- 工具栏、按钮行、状态行和卡片内部的一维排列。
- 需要在主轴分配空间或在窄窗口换行时。
- 需要 24 列响应式栅格用 [Grid 栅格](/components/gridrow)；需要固定分隔面板用 [Splitter 分割面板](/components/splitter)。
- 只想给内联控件增加统一间距时，用 [Space 间距](/components/space)。

## 在线体验

<GalleryPreview component="Flex" />

## 基本用法

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func page(c *ui.Context) {
    ui.Flex(c).Gap(ui.Small).Align(layout.AlignCenter).Body(func() {
        ui.Button(c, "取消")
        ui.Button(c, "保存").Primary()
    })
}
```

Flex 默认横向排列，`Gap` 使用 `Small`、`Middle`、`Large` 设计令牌。

## 常见用法

### 两端对齐工具栏

`JustifySpaceBetween` 把首尾内容推向两端，适合标题和操作按钮共存的行。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func page(c *ui.Context) {
    ui.Flex(c).Justify(layout.JustifySpaceBetween).Align(layout.AlignCenter).Body(func() {
        ui.Text(c, "项目设置").Strong()
        ui.Button(c, "保存").Primary()
    })
}
```

### 竖向内容

`Vertical()` 将主轴改为纵向；可配合 `Gap` 和 `Style` 的布局选项构造内容区。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func page(c *ui.Context) {
    ui.Flex(c).Vertical().Gap(ui.Middle).Style(layout.Pad(16)).Body(func() {
        ui.Text(c, "标题").Strong()
        ui.Text(c, "说明文字").Secondary()
    })
}
```

### 换行与像素间距

`Wrap()` 允许子项换行，`GapPx` 适合设计稿给出明确像素值的场景。

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    ui.Flex(c).Wrap().GapPx(12).Body(func() {
        for _, label := range []string{"一", "二", "三", "四"} {
            ui.Button(c, label)
        }
    })
}
```

## 主题与样式

Flex 的间距令牌是 `Small=8`、`Middle=16`、`Large=24`，不同主题只改变控件外观，不改变这些布局语义。需要精确值可用 `GapPx`，需要外边距、宽度或高度可用 `Style(layout.Option...)`。

## 注意事项

- `Flex` 适合一维布局；不要用大量嵌套 Flex 模拟二维栅格。
- 子节点需要占满剩余空间时，用子容器的 `layout.Grow(1)`；不要把不确定的百分比硬编码到每个子项。
- 换行后主轴和交叉轴的对齐仍由 `Justify`、`Align` 控制，窄屏应检查长文本和按钮是否可读。
- 键盘顺序遵循子节点顺序；不要仅用 `Justify` 或视觉顺序表达操作流程。
- RTL 会自动镜像主轴方向，避免用绝对定位覆盖 Flex 排版。

## 示例代码

<!--@include: ./_gen/flex-examples.md-->

## API

<!--@include: ./_gen/flex-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/nav_space.go)
- [ui Go API](/reference/api/ui)
- [Space 间距](/components/space)
