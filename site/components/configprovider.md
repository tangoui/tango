---
title: ConfigProvider 全局配置 / Locale 国际化
---
# ConfigProvider 全局配置 / Locale 国际化

为一段组件子树提供统一的尺寸、语言、方向、默认属性、禁用状态和主题配置。

## 何时使用

- 应用根部需要统一主题、暗色模式、主色、紧凑尺寸或高对比模式。
- 一个页面区域需要独立的语言、RTL 方向或组件默认属性时使用嵌套 Provider。
- 想让一组控件继承相同尺寸或统一禁用，而不是逐个设置时使用。
- 只需要改变单个实例外观时，直接使用组件方法或令牌，不必增加 Provider 层级。

## 在线体验

gallery 展示全局 small / middle / large、zh-CN / en-US、组件默认属性与全局禁用，并包含 RTL 方向场景。

<GalleryPreview component="ConfigProvider" />

## 基本用法

`ConfigProvider(c).Body(fn)` 只影响回调构建的组件；Provider 自身可以继续链式配置。

```go
import "github.com/tangoui/tango/ui"

func configProviderPage(c *ui.Context) {
    ui.ConfigProvider(c).Size(ui.Small).Body(func() {
        ui.Button(c, "小尺寸按钮").Primary()
        ui.Input(c, new(string)).Placeholder("小尺寸输入框")
    })
}
```

外层未显式指定尺寸的组件会读取 Provider 的尺寸；组件自己的 `Small`、`Large` 等显式设置优先。

## 常见用法

### 全局尺寸

`Size` 支持 `ui.Small`、`ui.Middle` 和 `ui.Large`，常用于让表单或工具栏保持一致密度。

```go
import "github.com/tangoui/tango/ui"

func configSizePage(c *ui.Context) {
    ui.ConfigProvider(c).Size(ui.Large).Body(func() {
        ui.Button(c, "大号操作")
        ui.Select(c, new(string), []ui.Option{{Label: "全部", Value: "all"}})
    })
}
```

### 国际化

传入内置 Locale 指针，或用 `LocaleCode` 按代码选择已注册的语言。组件内置文案（例如分页、确认、空状态）会从当前作用域读取语言。

```go
import "github.com/tangoui/tango/ui"

func configLocalePage(c *ui.Context) {
    ui.ConfigProvider(c).Locale(ui.ZhCN).Body(func() {
        ui.Pagination(c, new(int), 85, 10).Show()
    })
    ui.ConfigProvider(c).LocaleCode("en-US").Body(func() {
        ui.Pagination(c, new(int), 85, 10).Show()
    })
}
```

### 默认属性与全局禁用

`Defaults(component, props)` 给对应组件的作用域默认属性；`ComponentDisabled(true)` 会禁用子树中的可交互组件。gallery 使用 `Space` 的 `size` 默认值演示了这一组合。

```go
import "github.com/tangoui/tango/ui"

func configDefaultsPage(c *ui.Context) {
    ui.ConfigProvider(c).Defaults("Space", ui.Props{"size": ui.Large}).ComponentDisabled(true).Body(func() {
        ui.Space(c).Body(func() {
            ui.Button(c, "禁用按钮")
            ui.Button(c, "禁用主按钮").Primary()
        })
    })
}
```

### RTL 方向

`Direction(ui.RTL)` 创建从右到左的作用域，逻辑起止边、文本和支持 RTL 的组件会按方向布局。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

func configRTLPage(c *ui.Context) {
    ui.ConfigProvider(c).Direction(ui.RTL).Body(func() {
        ui.Row(c, func() {
            ui.Icon(c, icon.ArrowRight)
            ui.Text(c, "从右到左的工具栏")
            ui.Button(c, "继续").Primary()
        })
    })
}
```

### 主题和主色

`Dark`、`PrimaryColor`、`Compact` 和 `HighContrast` 作用于子树；`Theme` 或 `Components` 可提供更底层的主题对象和组件令牌覆盖。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/theme"
)

func configThemePage(c *ui.Context) {
    ui.ConfigProvider(c).
        Dark(true).
        PrimaryColor(theme.Hex("#1677ff")).
        Compact(true).
        HighContrast(false).
        Body(func() {
            ui.Button(c, "暗色紧凑按钮").Primary()
        })
}
```

## 主题与样式

- Ant Design、Element Plus 和 shadcn 主题由当前 `theme.Flavor` 决定基础令牌和组件外观；`ConfigProvider` 只改变它包裹的子树。
- `PrimaryColor` 改变主题主色，`Dark` 选择暗色算法，`Compact` 选择紧凑尺寸，`HighContrast` 开启高对比变体。
- `Defaults` 使用组件公开的默认属性键，例如 `Space` 的 `size`、`FloatButton` 的 `shape` / `type`；具体键以组件 API 注释和生成 API 表为准。
- 需要精确覆盖组件令牌时，用 `Components(theme.Components{...})` 传入组件令牌；不要在文档或业务代码中估算 token 数值。

## 注意事项

- **作用域边界**：只有在 `Body` 回调中创建的组件继承 Provider；把组件创建在回调外不会获得该配置。
- **状态放在哪**：Provider 不保存业务状态。表单值、选中项、展开状态和 locale 选择应放在页面变量或 `ui.State` 中。
- **嵌套优先级**：内层 Provider 覆盖自己设置的字段，未设置的字段继续继承外层；组件显式方法通常优先于默认属性。
- **RTL**：方向只影响包裹的子树；物理位置必须固定时，使用组件提供的物理方向选项，不要假设 `Right` 永远位于屏幕右侧。
- **性能**：Provider 会影响子树的主题计算，应用根部配置应保持稳定；不要为每个单独控件重复创建相同 Provider。

## 示例代码

<!--@include: ./_gen/configprovider-examples.md-->

## API

<!--@include: ./_gen/configprovider-api.md-->

## 相关链接

- 源码：[ui/nav_config.go](https://github.com/tangoui/tango/blob/main/ui/nav_config.go)
- [ui Go API](/reference/api/ui)
- 相关组件：[App 应用容器](/components/app)、[Layout 布局](/components/layout)
