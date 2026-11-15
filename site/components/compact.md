---
title: Compact 紧凑布局
---
# Compact 紧凑布局

把相邻输入框、选择器、按钮和附加标签拼接为一个整体，只保留外侧圆角。

## 何时使用

- 输入框与按钮、协议选择与地址输入、金额与单位等组合控件。
- 一组按钮需要首尾相连，形成按钮组。
- 只是给控件之间留间距，使用 [Space 间距](/components/space)。
- 表单中的标签、校验和布局交给 [Form 表单](/components/form)，Compact 只负责控件拼接。

## 在线体验

<GalleryPreview component="Compact" />

## 基本用法

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var keyword string

func page(c *ui.Context) {
    ui.Compact(c).Body(func() {
        ui.Input(c, &keyword).Placeholder("搜索").Layout(layout.W(220))
        ui.Button(c, "搜索").Primary()
    })
}
```

Compact 让相邻控件边框重叠 1px，连接处的圆角被消除，形成一个视觉整体。

## 常见用法

### 前置和后置标签

`Addon` 可以放在输入框两侧，表达协议、单位或固定后缀。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var site string

func page(c *ui.Context) {
    ui.Compact(c).Body(func() {
        ui.Addon(c, "https://")
        ui.Input(c, &site).Layout(layout.W(220)).A11yLabel("网址")
        ui.Addon(c, ".com")
    })
}
```

### 选择器与按钮组合

不同控件可以混排；每个控件仍保留自身的状态、焦点和禁用逻辑。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var (
    protocol = "https://"
    host     string
)

func page(c *ui.Context) {
    ui.Compact(c).Body(func() {
        ui.Select(c, &protocol, ui.Opts("https://", "http://")).Width(100).A11yLabel("协议")
        ui.Input(c, &host).Placeholder("站点").Layout(layout.W(200))
        ui.Button(c, "访问").Primary()
    })
}
```

### 竖向按钮组与撑满

`Vertical()` 上下拼接，`Block()` 让组合宽度撑满容器。

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    ui.Compact(c).Vertical().Block().Body(func() {
        ui.Button(c, "上移")
        ui.Button(c, "下移")
        ui.Button(c, "删除").Danger()
    })
}
```

## 主题与样式

Compact 不改变子控件自己的颜色和尺寸，只处理拼接边缘和外侧圆角；三套主题会沿用 Input、Select、Button 和 Addon 的视觉。`ConfigProvider.Defaults("Compact", ui.Props{"block": true})` 可设置默认撑满。

## 注意事项

- Compact 只作用于直接子控件；包装多层容器可能影响边缘识别，建议保持子控件平铺。
- 子控件的值仍由各自绑定的变量保存，Compact 不保存状态。
- 每个输入和图标按钮都需要可读的标签，例如 `A11yLabel`，否则组合后的语义不清楚。
- 错误态、禁用态应设置在具体控件上；Compact 不会把一个控件的状态传给其他控件。
- RTL 下拼接顺序与外侧圆角自动镜像。

## 示例代码

<!--@include: ./_gen/compact-examples.md-->

## API

<!--@include: ./_gen/compact-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/entry_compact.go)
- [ui Go API](/reference/api/ui)
- [Space 间距](/components/space)
