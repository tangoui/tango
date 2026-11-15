---
title: Mobile NavBar 导航栏
---
# Mobile NavBar 导航栏

移动端页面顶部导航栏，提供返回、居中标题和右侧文字或图标操作。

## 何时使用

- 页面需要移动端返回入口和标题时使用 `NavBar`。
- 右侧只有一个明确操作时使用 `Right` 或 `RightIcon`；复杂工具栏应单独布局。
- 不要用 NavBar 代替桌面端 `PageHeader` 或应用级导航。
- 返回事件由 `BackClicked` 读取，实际路由返回仍由页面负责。

## 在线体验

切换真实 gallery 的返回和右侧操作，观察 NavBar 在手机画布中的布局。

<GalleryPreview component="mobile.NavBar" />

## 基本用法

链式方法配置两侧内容；`Back("")` 使用默认返回图标并隐藏返回文字。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
    "github.com/tangoui/tango/ui/mobile"
)

func page(c *ui.Context) {
    nav := mobile.NavBar(c, "设置").Back("").RightIcon(icon.More).RightLabel("更多")
    if nav.BackClicked() {
        // 调用应用的返回逻辑
    }
    if nav.RightClicked() {
        ui.Text(c, "打开更多菜单")
    }
}
```

## 常见用法

### 返回文字

传入非空文本可同时显示返回箭头和文字。

```go
func backText(c *ui.Context) {
    mobile.NavBar(c, "详情").Back("返回")
}
```

### 右侧文字按钮

`Right` 适合短文本动作，点击状态从同一个 builder 读取。

```go
func rightAction(c *ui.Context) {
    nav := mobile.NavBar(c, "编辑").Back("").Right("保存")
    if nav.RightClicked() {
        ui.Text(c, "已保存")
    }
}
```

### 图标和辅助标签

用 `RightIcon` 提供图标，用 `RightLabel` 提供屏幕阅读器或视觉辅助标签。

```go
func iconAction(c *ui.Context) {
    mobile.NavBar(c, "消息").RightIcon(icon.Plus).RightLabel("新建消息")
}
```

### 控制分隔线

`Border(false)` 可关闭底部分隔线，让导航栏与自定义首屏内容无缝连接。

```go
func borderless(c *ui.Context) {
    mobile.NavBar(c, "首页").Border(false)
}
```

## 主题与样式

NavBar 读取当前 Provider 的移动主题令牌；Ant Mobile、Vant 和 shadcn 会调整高度、边框和文字层级。不要直接写颜色覆盖标题，应用内容可通过 `mobile.ThemeOf(c)` 读取令牌。

## 注意事项

NavBar 的 builder 应在每帧调用一次，并在同一帧读取 `BackClicked` 或 `RightClicked`。返回文字要保持简短；为图标动作设置 `RightLabel`，保证屏幕阅读器能说明动作。RTL 下返回箭头和左右槽位会自动镜像，不要手动交换内容。

## 示例代码

<!--@include: ./_gen/mobile-navbar-examples.md-->

## API

<!--@include: ./_gen/mobile-navbar-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/mobile/navbar.go)
- [Provider 移动端提供器](/components/mobile-provider)
- [List 列表](/components/mobile-list)
- [ui mobile Go API](/reference/api/ui-mobile)
