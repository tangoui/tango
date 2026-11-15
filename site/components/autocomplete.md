---
title: AutoComplete 自动完成
---
# AutoComplete 自动完成

在可编辑输入框中根据当前文本展示建议，并可用键盘或鼠标选中建议值。

## 何时使用

- 输入内容通常来自一组候选值，但仍允许用户先自由输入时。
- 地址、用户名、文件名等候选较多，需要边输入边缩小范围时。
- 只需要固定选项且不允许自由文本时，使用 [Select 选择器](/components/select)；需要在文本中插入提及时，使用 [Mentions 提及输入](/components/mentions)。

## 在线体验

<GalleryPreview component="AutoComplete" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

var address string
var addresses = ui.Opts("Downing Street", "Wall Street", "Baker Street")

func page(c *ui.Context) {
    ui.AutoComplete(c, &address, addresses).
        Placeholder("请输入地址").
        AllowClear().
        Highlight()
}
```

`val` 保存当前输入文本；静态选项会按输入内容匹配 `Label` 和 `Value`。选中建议后，`val` 会写入该选项的 `Value`。

## 常见用法

### 自定义过滤

`Source` 接收当前输入，返回本帧要显示的选项，适合拼音、别名或业务规则过滤。

```go
import (
    "strings"
    "github.com/tangoui/tango/ui"
)

var query string
var cities = []ui.Option{{Label: "杭州西湖", Value: "xihu"}, {Label: "上海浦东", Value: "pudong"}}

func page(c *ui.Context) {
    ui.AutoComplete(c, &query, cities).Source(func(q string) []ui.Option {
        var out []ui.Option
        for _, o := range cities {
            if strings.Contains(strings.ToLower(o.Label), strings.ToLower(q)) {
                out = append(out, o)
            }
        }
        return out
    })
}
```

### 聚焦即显示建议

`TriggerOnFocus` 在输入框获得焦点且尚未输入时打开建议；`Selected` 可读取本帧刚刚选中的值。

```go
import "github.com/tangoui/tango/ui"

var user string

func page(c *ui.Context) {
    w := ui.AutoComplete(c, &user, ui.Opts("alice", "bob")).TriggerOnFocus()
    if value, ok := w.Selected(); ok {
        _ = value // 在这里触发联动或加载详情
    }
}
```

### 远程搜索

`Remote` 在 `Debounce` 延迟后收到查询。回调中启动后台请求，完成后更新传给组件的选项并唤醒窗口；请求期间用 `Loading(true)` 显示加载状态。

```go
import (
    "time"
    "github.com/tangoui/tango/ui"
)

var remoteQuery string
var remoteOptions []ui.Option
var remoteBusy bool

func page(c *ui.Context) {
    ui.AutoComplete(c, &remoteQuery, remoteOptions).
        Remote(func(q string) { go func() { _ = q /* 请求完成后更新 remoteOptions */ }() }).
        Debounce(200 * time.Millisecond).
        Loading(remoteBusy)
}
```

### 外观与可访问名称

可用 `Variant`、`Size`、`Status`、`Width` 设置输入框外观；不在 `Form` 中使用时，用 `A11yLabel` 提供可访问名称。

```go
import "github.com/tangoui/tango/ui"

var keyword string

func page(c *ui.Context) {
    ui.AutoComplete(c, &keyword, nil).
        Variant(ui.FieldFilled).
        Width(280).
        Status("error").
        A11yLabel("搜索关键字")
}
```

## 主题与样式

- AutoComplete 复用 Input 的输入框令牌，弹出列表使用 Select 的选项令牌；三套设计风格会自动调整边框、圆角、选中背景和匹配高亮。
- 输入框支持 `outlined`、`filled`、`borderless`、`underlined` 四种 `Variant`，尺寸可用 `ui.Small`、默认值或 `ui.Large`。
- `ConfigProvider.Defaults("AutoComplete", ui.Props{...})` 支持 `size`、`placeholder`、`allowClear`；内部输入框同时继承 `Defaults("Input", ...)` 的 `variant`、`showCount`、`maxLength` 等键。

## 注意事项

- 即时模式下 `val` 应放在页面函数外或稳定的状态对象中；不要在每帧用局部变量重置输入值。
- 静态选项适合小规模候选；远程候选应配合 `Debounce`、`Loading`，并在异步完成后调用 `c.Invalidate()` 让窗口重绘。
- Enter 选择当前建议，↑ / ↓ 移动，Escape 关闭；远程结果应保留查询版本，避免旧请求覆盖新输入。
- 组件自动提供可展开组合框语义；在表单外使用时设置 `A11yLabel`。

## 示例代码

<!--@include: ./_gen/autocomplete-examples.md-->

## API

<!--@include: ./_gen/autocomplete-api.md-->

## 相关链接

- [源码 ui/entry_autocomplete.go](https://github.com/tangoui/tango/blob/main/ui/entry_autocomplete.go)
- [ui Go API](/reference/api/ui)
- [Select 选择器](/components/select)、[Mentions 提及输入](/components/mentions)、[Input 输入框](/components/input)
