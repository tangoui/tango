---
title: Mentions 提及输入
---
# Mentions 提及输入

用于在多行文本中输入内容，并在 `@` 等触发字符后选择一个提及对象。

## 何时使用

- 评论、聊天、工单等文本中需要插入用户、文件或其他对象时。
- 提及候选需要随着光标位置和触发字符变化时。
- 只需要普通多行文本时使用 [TextArea](/components/input)；不需要触发字符的候选补全时使用 [AutoComplete 自动完成](/components/autocomplete)。

## 在线体验

<GalleryPreview component="Mentions" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

var comment string
var users = []ui.Option{{Label: "Alice", Value: "alice"}, {Label: "Bob", Value: "bob"}}

func page(c *ui.Context) {
    ui.Mentions(c, &comment, users).
        Placeholder("请输入评论，使用 @ 提及").
        Rows(3)
}
```

默认触发字符是 `@`，默认显示一行。用户输入 `@` 后，候选项按当前查询过滤，选中后会插入 `@`、选项 `Value` 和 `Split` 分隔符（默认空格）。

## 常见用法

### 多个触发字符

`Prefix` 可以同时启用用户提及、话题标签等触发方式。`Query` 返回光标所在提及的前缀和查询文本。

```go
import "github.com/tangoui/tango/ui"

var text string
var candidates = ui.Opts("alice", "release", "design")

func page(c *ui.Context) {
    w := ui.Mentions(c, &text, candidates).Prefix("@", "#").Rows(3)
    if prefix, query, ok := w.Query(); ok {
        _ = prefix
        _ = query
    }
}
```

### 远程候选

`Remote` 在 UI 线程收到防抖后的触发字符和查询文本，默认延迟为 300ms。回调中启动请求，完成后把结果交回 UI 线程并替换选项切片。远程候选直接显示服务端返回的选项。加载期间用 `Loading(true)` 替换候选列表。

```go
import (
    "time"
    "github.com/tangoui/tango/ui"
)

var message string
var people []ui.Option
var loadingPeople bool

func page(c *ui.Context) {
    ui.Mentions(c, &message, people).
        Remote(func(prefix, query string) { go func() { _ = prefix; _ = query }() }).
        Debounce(250 * time.Millisecond).
        Loading(loadingPeople)
}
```

### 整体删除提及

`Whole` 使光标紧邻提及时按 Backspace 一次删除整个提及。需要确认对象确实存在时，用 `WholeCheck` 限制整体删除范围。

```go
import "github.com/tangoui/tango/ui"

var body string
var known = map[string]bool{"alice": true}

func page(c *ui.Context) {
    ui.Mentions(c, &body, ui.Opts("alice", "bob")).WholeCheck(func(prefix, value string) bool {
        return prefix == "@" && known[value]
    }).AllowClear()
}
```

### 输入框状态与样式

`Variant`、`Status`、`Size` 和 `Width` 与其他输入控件一致；`Selected` 返回本帧插入的 `Value`。

```go
import "github.com/tangoui/tango/ui"

var note string

func page(c *ui.Context) {
    w := ui.Mentions(c, &note, ui.Opts("alice", "bob")).
        Variant(ui.FieldFilled).
        Status("error").
        A11yLabel("评论")
    if w.Changed() {
        _ = w.Selected()
    }
}
```

## 主题与样式

- Mentions 使用 Input 的字段令牌和多行编辑器布局，弹出候选列表使用统一的选项列表令牌。
- `Variant` 支持 `FieldOutlined`、`FieldFilled`、`FieldBorderless`、`FieldUnderlined`；`Rows` 控制编辑器的行数。
- 主题切换会同步调整背景、边框、焦点环和候选高亮，无需在页面中写颜色。
- `ConfigProvider.Defaults("Mentions", ui.Props{...})` 支持 `size`、`placeholder`、`rows`、`prefix`（单个触发字符）。

## 注意事项

- `val` 必须放在稳定状态中；即时模式每帧创建组件是安全的，但不能每帧重新赋值文本。
- `Remote` 的结果可能乱序，业务层应按查询或请求版本丢弃过期结果。若请求在后台执行，可预先保存 `wake := c.Invalidator()`，交回结果后调用 `wake()` 请求新帧。
- 候选多于 100 条时自动虚拟化。`Virtual(false)` 关闭，`Virtual(true)` 强制开启。虚拟列表只构建可见行和键盘高亮行，保留完整的列表尺寸和滚动范围。视口外的候选不在无障碍树里。
- 本地候选的匹配缓存按切片身份、长度、查询和 `DataVersion` 区分，标签测宽缓存还会随主题变化。候选的标签、值和禁用状态会实时读取。原地修改同一切片时，递增版本并每帧传给 `DataVersion(version)`，或传入新切片，使匹配结果和列表宽度重新计算。远程候选每帧测宽。
- Enter 选择候选，↑ / ↓ 导航，Escape 关闭；多行输入中的 Enter 仍用于换行，候选打开时才用于选择。
- 为没有表单标签的编辑器设置 `A11yLabel`；默认 `@` 只是提示交互，不是可访问名称。

## 示例代码

<!--@include: ./_gen/mentions-examples.md-->

## API

<!--@include: ./_gen/mentions-api.md-->

## 相关链接

- [源码 ui/entry_mentions.go](https://github.com/tangoui/tango/blob/main/ui/entry_mentions.go)
- [ui Go API](/reference/api/ui)
- [AutoComplete 自动完成](/components/autocomplete)、[Input 输入框](/components/input)
