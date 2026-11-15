---
title: Tag 标签
---
# Tag 标签

用紧凑的彩色文本标记分类、状态或筛选条件。

## 何时使用

- 展示状态、类别和可关闭的筛选条件。
- 需要用户选择一个或多个选项时使用 [Select 选择器](/components/select)。
- 需要数字角标时使用 [Badge 徽章](/components/badge)。

## 在线体验

<GalleryPreview component="Tag" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    ui.Tag(c, "处理中").Color("processing")
    ui.Tag(c, "已完成").Color("success")
}
```

## 常见用法

### 状态颜色

```go
func statuses(c *ui.Context) {
    ui.Tag(c, "成功").Color("success")
    ui.Tag(c, "警告").Color("warning")
    ui.Tag(c, "失败").Color("error")
    ui.Tag(c, "自定义").Color("#722ed1")
}
```

### 可关闭标签

```go
var closed bool
func closable(c *ui.Context) {
    t := ui.Tag(c, "筛选条件").Closable()
    if t.Closed() { closed = true }
    _ = closed
}
```

### 图标和无边框

```go
func decorated(c *ui.Context) {
    ui.Tag(c, "发布").Borderless().Effect("light")
}
```

## 主题与样式

`Color` 支持状态名和十六进制颜色；`Size`、`Effect`、`Borderless` 分别控制尺寸、填充效果和边框。默认键为 `size`、`effect`、`bordered`。

## 注意事项

`Closed` 是当前帧的交互结果，业务状态应在外部变量中保存。标签文字不要承担唯一的颜色语义，并为可关闭操作保留清晰的键盘焦点。

## 示例代码

<!--@include: ./_gen/tag-examples.md-->

## API

<!--@include: ./_gen/tag-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/display.go)
- [Select 选择器](/components/select)
