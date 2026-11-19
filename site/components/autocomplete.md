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

`val` 保存当前输入文本。静态选项按 `Label` 做不区分大小写的子串匹配；选中建议后，组件把该选项的 `Value` 写入 `val`。

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

`TriggerOnFocus` 在输入框刚获得焦点时按当前文本打开建议，文本为空时展示全部静态选项。搭配 `Remote` 时，聚焦可立即触发一次查询。`Selected` 可读取本帧刚刚选中的值。

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

`Remote` 在 UI 线程上收到查询，输入变化后的防抖延迟默认是 300 毫秒，可用 `Debounce` 覆盖。远程选项直接展示，设置 `Source` 时优先展示 `Source` 的结果。请求期间用 `Loading(true)` 显示加载状态。下面用 `app.Post` 把结果交回 UI 线程，再请求新的一帧。

```go
import (
    "time"

    "github.com/tangoui/tango"
    "github.com/tangoui/tango/ui"
)

// 创建一次页面函数。fetch 由应用提供，在后台获取候选项。
func remotePage(app *tango.App, fetch func(string) []ui.Option) func(*ui.Context) {
    var query string
    var options []ui.Option
    var busy bool
    var request int
    return func(c *ui.Context) {
        wake := c.Invalidator()
        ui.AutoComplete(c, &query, options).
            Debounce(200 * time.Millisecond).
            Loading(busy).
            Remote(func(q string) {
                request++
                id := request
                busy = true
                c.Invalidate()
                go func() {
                    hits := fetch(q)
                    app.Post(func() {
                        if id == request && query == q {
                            options, busy = hits, false
                            wake()
                        }
                    })
                }()
            })
    }
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
- `ConfigProvider.Defaults("AutoComplete", ui.Props{...})` 支持 `size`、`placeholder`、`allowClear`、`variant` 和 `placement`。内部输入框同时继承 `Defaults("Input", ...)` 的 `variant`、`showCount`、`maxLength` 等键。

## 注意事项

- 即时模式下 `val` 应放在页面函数外或稳定的状态对象中；不要在每帧用局部变量重置输入值。
- 建议多于 100 条时下拉列表自动虚拟化，只构建可见行和键盘高亮行，保留完整列表的宽度、高度和滚动范围。无障碍树只暴露已构建的建议。用 `Virtual(false)` 构建所有行，或用 `Virtual(true)` 强制开启虚拟化。
- 虚拟化时，静态选项的匹配结果和宽度会复用。原地修改同一切片且长度不变时，传入递增的 `DataVersion(version)` 或换一个新切片，下一帧会重新计算。`Source` 和 `Remote` 的结果直接用于当前帧。
- Enter 选择当前建议，↑ / ↓ 移动，Escape 关闭。
- 组件自动提供可展开组合框语义；在表单外使用时设置 `A11yLabel`。

## 示例代码

<!--@include: ./_gen/autocomplete-examples.md-->

## API

<!--@include: ./_gen/autocomplete-api.md-->

## 相关链接

- [源码 ui/entry_autocomplete.go](https://github.com/tangoui/tango/blob/main/ui/entry_autocomplete.go)
- [ui Go API](/reference/api/ui)
- [Select 选择器](/components/select)、[Mentions 提及输入](/components/mentions)、[Input 输入框](/components/input)
