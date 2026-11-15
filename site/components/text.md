---
title: Typography 排版
---
# Typography 排版

提供统一的文本、标题、段落和链接排版层级。

## 何时使用

- 页面标题使用 `Title`，正文使用 `Paragraph` 或 `Text`。
- 需要可点击的文字操作时使用 `Link`，不要用普通文本模拟链接。
- 复杂富文本或代码编辑使用专用组件。

## 在线体验

<GalleryPreview component="Text" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    ui.Title(c, "项目概览", 1)
    ui.Paragraph(c, "这里是一段用于说明页面内容的正文。")
    ui.Link(c, "查看详情")
}
```

## 常见用法

### 层级标题

```go
func headings(c *ui.Context) {
    ui.Title(c, "一级标题", 1)
    ui.Title(c, "二级标题", 2)
    ui.Title(c, "三级标题", 3)
}
```

### 文本装饰

```go
func decoration(c *ui.Context) {
    ui.Text(c, "重点").Mark()
    ui.Text(c, "代码").Code()
    ui.Text(c, "已删除").Delete().Underline()
}
```

### 链接交互

```go
func action(c *ui.Context, open *bool) {
    if ui.Link(c, "打开详情").Clicked() { *open = true }
}
```

## 主题与样式

Typography 令牌控制字号、字重、行高、标题颜色和链接状态；三套主题只改变令牌配方，标题层级仍为 1–5。`Layout` 可为文本应用布局选项。

## 注意事项

标题层级应按页面结构递进；动态文本不要拼接未转义的用户输入。链接提供清晰的动作名，并确保可用 Tab 聚焦、Enter 激活；RTL 下文本方向和对齐跟随作用域。

## 示例代码

<!--@include: ./_gen/text-examples.md-->

## API

<!--@include: ./_gen/text-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/text.go)
- [Button 按钮](/components/button)
