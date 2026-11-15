---
title: CheckableTag 可选择标签
---
# CheckableTag 可选择标签

在选中和未选中之间切换的标签，适合轻量的分类、过滤和偏好选择。

## 何时使用

- 在一组标签中选择一个或多个筛选条件，例如内容分类、兴趣和状态。
- 需要比复选框更紧凑、可直接嵌入工具栏的切换控件时。
- 需要说明性标签但不允许交互时用 [Tag 标签](/components/tag)；需要表单字段标签、校验和复杂选项时用 [Checkbox 多选框](/components/checkbox) 或 [Radio 单选框](/components/radio)。

## 在线体验

示例包含基础标签、分类选择、状态色、图标和禁用状态。

<GalleryPreview component="CheckableTag" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import "github.com/tangoui/tango/ui"

var favorite bool

func page(c *ui.Context) {
    tag := ui.CheckableTag(c, "收藏", &favorite)
    if tag.Changed() {
        ui.Text(c, "收藏状态已更新").Tertiary()
    }
}
```

`CheckableTag` 直接绑定一个 `*bool`。点击标签，或聚焦后按 Enter / 空格，会切换这个值；`Changed()` 只在发生切换的当前帧返回 `true`，`Checked()` 读取当前值。

## 常见用法

以下片段省略了 `import`。

### 多个分类标签

每个标签绑定自己的布尔值。筛选结果由页面状态决定，标签组件只负责记录切换。

```go
var filters = map[string]bool{}

func categoryTags(c *ui.Context) {
    for _, label := range []string{"电影", "图书", "音乐", "体育"} {
        checked := filters[label]
        if ui.CheckableTag(c, label, &checked).Changed() {
            filters[label] = checked
        }
    }
}
```

### 状态色与图标

`Type` 可用 `primary`、`success`、`info`、`warning`、`error`，颜色作用于选中态；`Icon` 在文字前放置图标。

```go
var statusSelected = [5]bool{}

func coloredTags(c *ui.Context) {
    for i, status := range []string{"primary", "success", "info", "warning", "error"} {
        ui.CheckableTag(c, status, &statusSelected[i]).Type(status)
    }
    ui.CheckableTag(c, "收藏", &favorite).Icon(icon.Star)
}
```

### 禁用与读取状态

`Disabled(true)` 会阻止鼠标和键盘切换。禁用标签仍显示绑定值，因此可以分别表达「已选但暂不可改」和「未选且暂不可用」。

```go
var unavailable = true

func disabledTag(c *ui.Context) {
    tag := ui.CheckableTag(c, "暂不可用", &unavailable).Disabled(true)
    ui.Text(c, fmt.Sprintf("当前值：%t", tag.Checked())).Tertiary()
}
```

### 图标标签与无障碍名称

没有可见文字的标签必须通过 `A11yLabel` 命名。通常仍建议保留简短文字，便于不依赖颜色或图标的用户理解状态。

```go
var compactFavorite bool

func labeledIconTag(c *ui.Context) {
    ui.CheckableTag(c, "", &compactFavorite).
        Icon(icon.Star).A11yLabel("收藏").Type("warning")
}
```

## 主题与样式

Ant Design 使用轻量文字标签，选中后采用主色背景；Element Plus 同时给未选中态加入状态色调并使用更紧凑的高；shadcn 对照 Badge 的紧凑内边距和中等字重。`Type` 的状态色来自当前主题色板，不需要手写颜色。

CheckableTag 没有单独的 `ConfigProvider.Defaults` 键；尺寸、圆角、字体和状态色随全局主题与 `theme` 色板变化。

## 注意事项

- **状态放在哪**：布尔值属于页面状态，渲染函数不要每帧重新赋值。动态增删标签时，用稳定的业务键保存对应值。
- **事件读取**：`Changed()` 和 `Checked()` 会触发必要的当前帧计算，应在构建后读取；不要把切换逻辑藏在标签文本生成中。
- **键盘与无障碍**：组件使用可切换按钮语义，Enter / 空格可操作；图标-only 标签必须设置 `A11yLabel`。不要只用颜色表达选中状态。
- **禁用**：`Disabled(true)` 是组件级阻止交互，不会修改绑定的布尔值。
- **RTL**：图标与文字间距、标签内容和焦点环遵循当前 RTL 方向。

## 示例代码

<!--@include: ./_gen/checkabletag-examples.md-->

## API

<!--@include: ./_gen/checkabletag-api.md-->

## 相关链接

- 源码：[ui/more_checktag.go](https://github.com/tangoui/tango/blob/main/ui/more_checktag.go)
- [ui Go API 参考](/reference/api/ui)
- [Tag 标签](/components/tag)、[Checkbox 多选框](/components/checkbox)、[Radio 单选框](/components/radio)
- [组件总览](/components/)、[组件截图目录](/components/screenshots)
