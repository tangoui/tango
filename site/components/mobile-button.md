---
title: Mobile Button 移动端按钮
---
# Mobile Button 移动端按钮

提供适合手指触控的移动端按钮，支持主要、危险、加载、尺寸、块级和圆角样式。

## 何时使用

- 需要提交、确认、打开页面等明确操作时使用 `mobile.Button`。
- 表单底部的主提交按钮通常使用 `Primary().Block()`。
- 行内切换、导航或列表操作应使用对应的 Cell、Tabs 或 NavBar，不要全部改成按钮。
- 正在等待异步结果时使用 `Loading(true)`，避免重复提交。

## 在线体验

真实 gallery 展示默认、主要、成功、危险、加载、尺寸、圆角和禁用按钮。

<GalleryPreview component="mobile.Button" />

## 基本用法

按钮是否被点击由当前帧的 `Clicked` 返回；页面状态需要由调用方保存。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/mobile"
)

var saved bool

func page(c *ui.Context) {
    if mobile.Button(c, "保存").Primary().Clicked() {
        saved = true
    }
    if saved {
        ui.Text(c, "已保存")
    }
}
```

## 常见用法

### 主要和块级按钮

`Primary` 表示主要操作，`Block` 让按钮占据可用宽度。

```go
func submitButton(c *ui.Context) {
    mobile.Button(c, "提交").Primary().Block().Size(mobile.Large)
}
```

### 颜色和填充

`Color` 使用移动组件约定的颜色名，`Fill("outline")` 生成描边按钮。

```go
func variants(c *ui.Context) {
    mobile.Button(c, "成功").Color("success")
    mobile.Button(c, "危险").Danger().Fill("outline")
}
```

### 尺寸和圆角

尺寸使用 `Mini`、`Small`、`Middle`、`Large`；`Round` 适合短标签或胶囊式操作。

```go
func sizes(c *ui.Context) {
    mobile.Button(c, "迷你").Size(mobile.Mini)
    mobile.Button(c, "小号").Size(mobile.Small)
    mobile.Button(c, "圆角").Round().Primary()
}
```

### 加载和禁用

加载态会阻止重复点击；禁用态应由业务状态决定。

```go
var submitting bool

func states(c *ui.Context) {
    if mobile.Button(c, "发送").Primary().Loading(submitting).Clicked() {
        submitting = true
    }
    mobile.Button(c, "不可用").Disabled(true)
}
```

### 图标和无障碍标签

图标按钮或文案不完整时用 `Icon` 和 `A11yLabel` 补充语义。

```go
import "github.com/tangoui/tango/ui/icon"

func iconButton(c *ui.Context) {
    mobile.Button(c, "").Icon(icon.Plus).A11yLabel("新建消息")
}
```

## 主题与样式

Button 的主色、危险色、圆角、尺寸和加载态读取 Provider 的移动主题。Ant Mobile、Vant、shadcn 的填充和边框配方不同；`ConfigProvider.Defaults("mobile.Button", ui.Props{"size": mobile.Large})` 可设置默认属性，显式方法优先。

## 注意事项

`Clicked` 是当前帧事件，提交状态应放在稳定变量中。异步提交期间设置 Loading，完成或失败后恢复。按钮文案要描述动作，图标按钮必须设置 `A11yLabel`；确保触控区域足够大并支持 Enter、Space 和屏幕阅读器 Press。危险按钮应配合确认，而不是只依赖红色。

## 示例代码

<!--@include: ./_gen/mobile-button-examples.md-->

## API

<!--@include: ./_gen/mobile-button-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/mobile/display.go)
- [Form 表单](/components/mobile-form)
- [Popup 弹出层](/components/mobile-popup)
- [ui mobile Go API](/reference/api/ui-mobile)
