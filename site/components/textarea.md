---
title: TextArea 文本域
---
# TextArea 文本域

绑定一个 `string` 的多行文本输入，支持行数、字数统计、超限提示、自适应高度、清空和滚动。

## 何时使用

- 备注、简介、描述、代码片段等需要换行的文本输入使用 `TextArea`。
- 单行短文本使用 [Input 输入框](/components/input)，搜索使用 [Search 搜索框](/components/search)。
- 需要结构化的多字段编辑和统一校验时，使用 [Form 表单](/components/form) 包裹 TextArea。
- 需要富文本、Markdown 或协同编辑时，使用对应的编辑器组件，而不是把 TextArea 当富文本编辑器。

## 在线体验

<GalleryPreview component="TextArea" />

## 基本用法

TextArea 直接绑定 `*string`；值和焦点状态必须由页面外部变量或状态持有，不能每帧重置。

```go
import (
    "fmt"

    "github.com/tangoui/tango/ui"
)

var description string

func textareaPage(c *ui.Context) {
    ui.TextArea(c, &description).
        Placeholder("请输入描述").
        Rows(4).
        A11yLabel("描述")
    ui.Text(c, "字数："+fmt.Sprint(len([]rune(description))))
}
```

## 常见用法

### 字数统计与最大长度

`ShowCount` 显示计数，`MaxLength` 限制字符数；默认超出会限制输入，`Exceed` 则允许超出并标记错误状态。

```go
import "github.com/tangoui/tango/ui"

var summary string

func countedTextareaPage(c *ui.Context) {
    ui.TextArea(c, &summary).
        Placeholder("不超过 100 字").
        Rows(3).
        ShowCount().
        MaxLength(100)
}
```

```go
import "github.com/tangoui/tango/ui"

var longSummary string

func exceedingTextareaPage(c *ui.Context) {
    ui.TextArea(c, &longSummary).
        Rows(2).
        ShowCount().
        MaxLength(20).
        Exceed().
        A11yLabel("允许超出的摘要")
}
```

### 自适应高度和滚动

`AutoSize(minRows, maxRows)` 让高度在行数范围内随内容增长；超过最大行数后，内部内容滚动。`maxRows <= 0` 表示不设上限。

```go
import "github.com/tangoui/tango/ui"

var article string

func autosizeTextareaPage(c *ui.Context) {
    ui.TextArea(c, &article).
        Placeholder("最多展示 4 行").
        AutoSize(2, 4)
}
```

### 清空、状态和禁用

`AllowClear` 提供清空操作；`Status("warning")` 或 `Status("error")` 只改变校验展示，业务校验仍由页面或 Form 负责；`Disabled(true)` 禁止编辑。

```go
import "github.com/tangoui/tango/ui"

var note = "已有备注"

func statesTextareaPage(c *ui.Context) {
    ui.TextArea(c, &note).Rows(3).AllowClear().Status("warning").A11yLabel("备注")
    ui.TextArea(c, &note).Rows(3).Disabled(true).A11yLabel("只读备注")
}
```

### 监听编辑和焦点

`Changed()` 报告当前帧是否编辑或清空，`Focused()` 报告内部编辑器是否拥有焦点；适合驱动未保存提示或快捷操作。

```go
import "github.com/tangoui/tango/ui"

var draft string
var draftDirty bool

func trackedTextareaPage(c *ui.Context) {
    area := ui.TextArea(c, &draft).Rows(5).A11yLabel("草稿")
    if area.Changed() { draftDirty = true }
    if area.Focused() && draftDirty { ui.Text(c, "有未保存修改").Color(c.Theme.Warning.Base) }
}
```

## 主题与样式

- Ant Design：文本域使用输入框容器、焦点环和主题边框；字数统计位于控件底部区域。
- Element Plus：状态色和禁用态跟随 Element 输入控件令牌，`AutoSize` 保持相同的行数语义。
- shadcn：采用边框输入区和主题的焦点环；`AllowClear` 的按钮保持在输入区内。
- `ConfigProvider.Defaults("TextArea", ...)` 可提供组件默认值；常用的单实例配置通过 `Rows`、`Status`、`MaxLength` 和 `AutoSize` 链式设置。

## 注意事项

- **状态**：绑定字符串放在包级变量、页面状态结构或 `ui.State`；不要在页面函数内写 `value := ""` 再传指针，否则每帧都会丢失输入。
- **字符数**：`MaxLength` 按 Unicode 字符限制，不要用字节长度推断用户看到的计数；自定义旁边的计数时也应按 rune 计算。
- **性能**：超长内容需要 `AutoSize` 上限，让控件在内部滚动；不要让输入框无限增长挤出整个页面。
- **无障碍**：独立 TextArea 请用 `A11yLabel`，Form 内可由 `FormItem` 标签提供名称；状态错误应同时提供可理解的提示文本。
- **RTL**：编辑区文本和光标遵循 RTL 设置，行数、字符计数和垂直滚动语义不变。

## 示例代码

<!--@include: ./_gen/textarea-examples.md-->

## API

<!--@include: ./_gen/textarea-api.md-->

## 相关链接

- [源码 entry_inputx.go](https://github.com/tangoui/tango/blob/main/ui/entry_inputx.go)
- [ui Go API](/reference/api/ui)
- [Input 输入框](/components/input)、[Form 表单](/components/form)、[Input 输入框](/components/input)
