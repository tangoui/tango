---
title: CodeBlock 代码块
---
# CodeBlock 代码块

只读的代码展示块：等宽字体、可选行号与标题栏、语法高亮、一键复制，长行横向滚动或自动换行，可拖选文字复制。只排版可见的行，十万行以上的文件也能流畅滚动。

## 何时使用

- 在界面里展示代码片段、配置文件、命令示例、接口响应等需要等宽排版并保持缩进的只读文本。
- 需要行号、标出重点行、显示文件名，或提供“复制代码”按钮时使用。
- 需要编辑代码用 [editor.View 代码编辑器视图](/components/editor-view)；展示持续追加的日志、带 ANSI 颜色的输出用 [LogView 日志查看器](/components/logview)；展示结构化数据的值树用 [JSONView 值树查看器](/components/jsonview)。
- 正文里夹带代码块的富文本内容直接用 [Markdown](/components/markdown)，它的围栏代码块与 CodeBlock 共用同一套高亮。

## 在线体验

<GalleryPreview component="CodeBlock" />

## 基本用法

语法高亮由钩子提供：`ui` 包本身不带语法，需要在应用启动时用 `ui.SetCodeHighlighter` 安装一次 `ui/codehl` 的实现（它复用代码编辑器的语法库）。没有安装高亮器，或没有调用 `Lang`，代码按纯文本显示。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/codehl"
)

const snippet = `package main

import "fmt"

func main() {
	fmt.Println("你好，Tango")
}`

func init() {
    // 启动时安装一次；CodeBlock 和 Markdown 的代码块都会用它。
    ui.SetCodeHighlighter(codehl.New())
}

func page(c *ui.Context) {
    ui.CodeBlock(c, snippet).Lang("go").Title("main.go")
}
```

`Lang` 接受语言 id、别名或文件扩展名（`"go"`、`"ts"`、`"python"`、`"sh"` 等）；高亮器不认识的语言同样按纯文本显示。`Title` 会显示标题栏并把复制按钮放进标题栏；没有标题时，复制按钮在鼠标悬停或键盘聚焦时出现在右上角。代码按 `\n` 分行，`\r\n` 也可以。

## 常见用法

### 行号、起始行与标出重点行

```go
import "github.com/tangoui/tango/ui"

func codeLines(c *ui.Context) {
    ui.CodeBlock(c, "if err != nil {\n\treturn err\n}\nreturn nil").
        Lang("go").
        LineNumbers(true).
        StartLine(120).
        HighlightLines(121, 122)
}
```

`LineNumbers(true)` 显示行号列，行号列不随横向滚动移动。`StartLine(n)` 设置第一行的编号（默认 1），适合展示文件中间的一段。`HighlightLines` 的参数使用显示出来的行号，也就是已经计入 `StartLine` 之后的编号。点击行号可以选中整行。

### 限定高度与大文件

```go
import "github.com/tangoui/tango/ui"

func codeHeight(c *ui.Context, bigSource string) {
    // 内容超过 320px 后出现滚动条；内容更短时块跟着变矮。
    ui.CodeBlock(c, bigSource).Lang("go").LineNumbers(true).MaxHeight(320)

    // 固定高度，内容不足时留白。
    ui.CodeBlock(c, "echo hi").Lang("sh").Height(120)
}
```

不设 `Height` 和 `MaxHeight` 时，块的高度等于全部代码的高度。展示长文件时务必设置 `MaxHeight` 或 `Height`：块内部只排版视口里的行，所以十万行的文件与一百行的文件同样快。

### 横向滚动与自动换行

```go
import "github.com/tangoui/tango/ui"

var wrapLong bool

func codeWrap(c *ui.Context, src string) {
    ui.Toggle(c, "自动换行", &wrapLong)
    ui.CodeBlock(c, src).Lang("json").Wrap(wrapLong).MaxHeight(240)
}
```

默认长行不换行，横向滚动保持代码形状。`Wrap(true)` 改为按块宽度折行，横向滚动条随之消失。`Wrap` 接受 `bool` 而不是指针，状态由业务自己保存。不换行时，单行最多显示 10000 个字符。

### 复制与读取选区

```go
import (
    "fmt"
    "github.com/tangoui/tango/ui"
)

var copyCount int

func codeCopy(c *ui.Context) {
    cb := ui.CodeBlock(c, "go install github.com/tangoui/tango/cmd/tan@latest").Lang("sh").Title("安装")
    if cb.Copied() {
        copyCount++
    }
    ui.Text(c, fmt.Sprintf("点击复制按钮 %d 次；当前选中：%q", copyCount, cb.Selection()))
}
```

复制按钮复制全部代码并弹出“已复制”提示，`Copied()` 在点击的那一帧返回 `true`。拖动、双击选词、三击或点击行号选行、`Cmd/Ctrl+A` 全选，`Cmd/Ctrl+C` 复制选区；`Selection()` 返回当前选中的文本，没有选区时为空串。`NoCopy()` 隐藏复制按钮，选区复制快捷键仍可使用。

这个视图是一个 Tab 停靠点，聚焦后方向键、`PageUp` / `PageDown`、`Home` / `End` 滚动。

### 自定义高亮器

`CodeHighlighter` 只有一个方法 `Document(lang, code)`，返回的 `CodeDocument` 按行给出带类型的片段。下面是一个只给 SQL 关键字着色的高亮器，通过 `Highlighter` 只用于这一个块：

```go
import (
    "strings"

    "github.com/tangoui/tango/ui"
)

type sqlHL struct{}

func (*sqlHL) Document(lang, code string) ui.CodeDocument {
    if lang != "sql" {
        return nil // 不认识的语言：按纯文本显示
    }
    return &sqlDoc{lines: strings.Split(code, "\n")}
}

type sqlDoc struct{ lines []string }

var sqlWords = map[string]bool{"SELECT": true, "FROM": true, "WHERE": true, "ORDER": true, "BY": true}

// Line 返回第 i 行的片段：按字节范围、不重叠、有序；返回 true 表示结果已定稿。
func (d *sqlDoc) Line(i int, dst []ui.CodeSpan) ([]ui.CodeSpan, bool) {
    line := d.lines[i]
    start := -1
    for p := 0; p <= len(line); p++ {
        isWord := p < len(line) && (line[p] >= 'A' && line[p] <= 'Z' || line[p] >= 'a' && line[p] <= 'z' || line[p] == '_')
        if isWord && start < 0 {
            start = p
        }
        if !isWord && start >= 0 {
            if sqlWords[strings.ToUpper(line[start:p])] {
                dst = append(dst, ui.CodeSpan{Start: start, End: p, Kind: ui.CodeKeyword})
            }
            start = -1
        }
    }
    return dst, true
}

var sqlHighlighter = &sqlHL{}

func codeCustom(c *ui.Context) {
    ui.CodeBlock(c, "select id, name\nfrom users\nwhere id > 10").Lang("sql").Highlighter(sqlHighlighter)
}
```

`Line` 的片段字节范围相对该行（不含换行符），空隙按纯文本绘制。`Line` 的第二个返回值为 `false` 表示片段只是猜测（上方行还在分词），CodeBlock 会在稍后的帧再次询问；同步完成的高亮器始终返回 `true`。`Highlighter(nil)` 强制纯文本。

`ui.SetCodeHighlighter` 安装的是全局默认高亮器，`ui.DefaultCodeHighlighter()` 读取它。`codehl.New()` 返回的高亮器并发安全，并缓存最近用过的文档，同一段代码每帧重复请求不会重新分词。

### 与 Markdown 共用高亮

```go
import "github.com/tangoui/tango/ui"

func codeInMarkdown(c *ui.Context) {
    ui.Markdown(c, "示例：\n\n```json\n{\"ok\": true, \"count\": 3}\n```\n")
}
```

Markdown 的围栏代码块使用同一个默认高亮器和同一套 `CodeBlock` 颜色令牌，也带复制按钮。只想让某个 Markdown 用别的高亮器时，用它自己的 `Highlighter` 方法。

## 主题与样式

CodeBlock 的外框、字号、行号列和语法颜色都来自组件令牌 `theme.Components.CodeBlock`（`theme.CodeBlockToken`）：

- 外观：`Bg`、`BorderColor`（零值无边框）、`Radius`、`CodeFontSize`、`LineHeight`、`PaddingBlock`、`PaddingInline`。
- 标题栏与行号：`HeaderColor`、`HeaderBorder`、`GutterColor`、`GutterBorder`、`LineHighlightBg`（`HighlightLines` 的底色）、`SelectionBg`。
- 语法颜色：`Text`（纯文本）以及 `Keyword`、`String`、`Number`、`Comment`、`Type`、`Function`、`Variable`、`Constant`、`Operator`、`Punctuation`、`Tag`、`Attribute`、`Property`、`Regexp`、`Invalid`，分别对应 `ui.CodeKeyword` 等 `CodeTokenKind`。

三套风格（Ant Design / Element Plus / shadcn）各自从自己的色板派生这些颜色，并分别适配浅色、深色和高对比主题，不需要为明暗各写一份。需要整体调整时在创建主题时覆盖：

```go
import "github.com/tangoui/tango/ui/theme"

func codeTheme() *theme.Theme {
    return theme.AntDesign(theme.WithComponents(theme.Components{
        CodeBlock: theme.CodeBlockToken{CodeFontSize: 13, LineHeight: 20},
    }))
}
```

CodeBlock 没有 `ConfigProvider.Defaults` 键。在 RTL 布局中，代码块仍保持从左到右阅读。

## 注意事项

- **高亮是可选的钩子**：`ui` 包不依赖语法库，没有安装高亮器时一切正常，只是没有颜色。`ui/codehl` 目前属于实验性包，API 可能在 0.x 版本里调整。JSON 没有独立的语法，`codehl` 用 JavaScript 语法为它着色字符串、数字和字面量。
- **自定义高亮器要可比较**：CodeBlock 用高亮器值是否变化来决定是否重建高亮文档，所以传给 `Highlighter` 的值必须是可比较的（推荐指针，且在包级变量里复用），不要每帧 new 一个，更不要用含 map 或切片字段的值类型。
- **状态与 Key**：行索引、选区、高亮缓存和滚动位置按 CodeBlock 的节点 ID 保存。同一位置每帧渲染同一个块即可；在会增删、重排的列表里渲染多个块时，用 `ui.Key(c, 稳定的键, fn)` 包住每个块，状态才会跟着数据走。
- **性能**：代码文本变化时才重新建立行索引和高亮文档；对很大的文本，把字符串保存在状态里复用，不要每帧拼接或重新生成。
- **无障碍**：代码区作为一个可聚焦的文本区域暴露给读屏，名称默认取标题、其次语言、最后是本地化的“代码”；用 `A11yLabel` 指定更明确的名称。复制按钮有本地化的文字标签。

## 示例代码

<!--@include: ./_gen/codeblock-examples.md-->

## API

<!--@include: ./_gen/codeblock-api.md-->

## 相关链接

- [源码：codeblock.go](https://github.com/tangoui/tango/blob/main/ui/codeblock.go)、[code_highlight.go](https://github.com/tangoui/tango/blob/main/ui/code_highlight.go)、[viewer_text.go](https://github.com/tangoui/tango/blob/main/ui/viewer_text.go)
- [ui Go API](/reference/api/ui)
- [LogView 日志查看器](/components/logview)
- [JSONView 值树查看器](/components/jsonview)
- [Markdown](/components/markdown)
- [editor.View 代码编辑器视图](/components/editor-view)
