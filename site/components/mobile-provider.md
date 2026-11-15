---
title: Mobile Provider 移动端提供器
---
# Mobile Provider 移动端提供器

为一段页面选择移动端视觉风格，并把安全区信息传给 NavBar、Popup、SafeArea 等组件。

## 何时使用

- 需要在自绘页面中组合移动端组件时，在页面根部调用 `mobile.Provider`。
- 需要切换 Ant Design Mobile、Vant 或 shadcn 移动风格时使用 `Options.Style`。
- 宿主已经提供安全区时通常不设置 `Options.SafeArea`；模拟设备或需要覆盖宿主值时再传入 `*geom.Insets`。
- 不要把 Provider 当作全局主题配置；桌面组件的主题仍由 `ui.ConfigProvider` 和 `ui.Theme` 管理。

## 在线体验

下方示例展示 Provider 包裹的真实移动端页面，可切换主题和明暗模式。

<GalleryPreview component="mobile.Provider" />

## 基本用法

Provider 通过闭包建立作用域；作用域内的移动组件会读取同一套风格和安全区。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
    "github.com/tangoui/tango/ui/mobile"
)

func page(c *ui.Context) {
    mobile.Provider(c, mobile.Options{Style: mobile.AntMobile}, func() {
        ui.Column(c, func() {
            mobile.NavBar(c, "账户").Back("")
            mobile.List(c, "").Body(func() {
                mobile.Cell(c, "个人资料").Arrow()
            })
        }, layout.Grow(1))
    })
}
```

## 常见用法

### 选择 Vant 风格

`Style` 只影响移动组件的令牌和外观，组件调用方式不变。

```go
func vantPage(c *ui.Context) {
    mobile.Provider(c, mobile.Options{Style: mobile.Vant}, func() {
        mobile.Button(c, "确认").Primary()
    })
}
```

### 覆盖安全区

`SafeArea` 覆盖当前 Provider 读取到的窗口安全区，适合设备模拟和测试。

```go
import "github.com/tangoui/tango/ui/geom"

func devicePage(c *ui.Context) {
    insets := geom.Insets{Top: 47, Bottom: 34}
    mobile.Provider(c, mobile.Options{SafeArea: &insets}, func() {
        mobile.NavBar(c, "设备预览")
        mobile.SafeArea(c, mobile.Bottom)
    })
}
```

### 为组件设置默认值

ConfigProvider 的默认值使用 `mobile.<Component>` 命名空间；显式链式方法优先于默认值。

```go
func defaultsPage(c *ui.Context) {
    ui.ConfigProvider(c).
        Defaults("mobile.Button", ui.Props{"size": "large", "shape": "round"}).
        Body(func() {
            mobile.Provider(c, mobile.Options{}, func() {
                mobile.Button(c, "大号按钮")
            })
        })
}
```

### 读取移动主题令牌

`ThemeOf` 返回当前 Provider 的移动主题，可用于自定义内容而不硬编码颜色。

```go
func customPage(c *ui.Context) {
    mobile.Provider(c, mobile.Options{}, func() {
        th := mobile.ThemeOf(c)
        ui.Text(c, "跟随移动主题").Color(th.TextSecondary)
    })
}
```

## 主题与样式

Provider 支持 `AntMobile`、`Vant` 和 `Shadcn`。它们共享组件 API，但令牌、圆角、间距和浮层视觉不同；暗色由外层 ui 主题提供。组件默认值可通过 `ConfigProvider.Defaults("mobile.Button", ...)` 等键配置，显式 modifier 总是覆盖默认值。

## 注意事项

Provider 只建立当前页面构建期间的作用域，状态仍放在页面函数外的变量或页面闭包中。安全区不要重复叠加：如果 Provider 已配置 `SafeArea`，底部再按需要调用一次 `mobile.SafeArea(c, mobile.Bottom)`。RTL 应由 `ui.ConfigProvider(c).Direction(ui.RTL)` 设置，Provider 会让移动组件跟随方向。

## 示例代码

<!--@include: ./_gen/mobile-provider-examples.md-->

## API

<!--@include: ./_gen/mobile-provider-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/mobile/config.go)
- [移动端主题源码](https://github.com/tangoui/tango/blob/main/ui/mobile/theme.go)
- [移动端组件总览](/components/)
- [ui mobile Go API](/reference/api/ui-mobile)
