---
title: App 应用容器
---
# App 应用容器

为一段应用子树统一配置并访问 Message、Notification 和 Confirm 等反馈实例。

## 何时使用

- 页面或应用需要统一设置消息持续时间、通知位置或同时显示数量时使用。
- 子组件需要通过 `UseApp` 获取与当前应用容器一致的反馈配置时使用。
- 根部已经有明确的全局消息策略时，把 App 放在窗口内容的根部；小页面不需要统一策略时可直接调用 `Message` 或 `Notification`。
- App 不是路由或页面布局容器；需要后台外壳时使用 [ProLayout 高级布局](/components/prolayout)。

## 在线体验

gallery 的 App 场景把消息持续时间、通知位置和 `MaxCount` 下发给子树，点击按钮可观察 `UseApp` 返回的句柄和数量限制。

<GalleryPreview component="App" />

## 基本用法

`App` 链式配置后调用 `Body`，在其回调中用 `UseApp(c)` 取得当前最内层 App 的反馈 API。

```go
import "github.com/tangoui/tango/ui"

func appPage(c *ui.Context) {
    ui.App(c).Body(func() {
        app := ui.UseApp(c)
        if ui.Button(c, "保存").Primary().Clicked() {
            app.Message("success", "保存成功")
        }
    })
}
```

`UseApp(c).InApp()` 可判断当前句柄是否来自外层 App；不在 App 中时，它仍会返回可用句柄并使用普通反馈默认值。

## 常见用法

### 配置消息

`AppMessageConfig` 的 `Duration` 设置消息默认持续时间，`MaxCount` 限制同时显示的消息数量；超出数量时最早的消息会关闭。

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
)

func appMessagePage(c *ui.Context) {
    ui.App(c).Message(ui.AppMessageConfig{
        Duration: 5 * time.Second,
        MaxCount: 3,
    }).Body(func() {
        app := ui.UseApp(c)
        if ui.Button(c, "提示").Clicked() {
            app.Message("info", "这条消息最多显示 5 秒")
        }
    })
}
```

### 配置通知

`AppNotificationConfig` 可设置通知位置、持续时间、数量上限和进度条；给零值位置时要把 `PlacementSet` 设为 false，使用默认的顶部末端位置。

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
)

func appNotificationPage(c *ui.Context) {
    ui.App(c).Notification(ui.AppNotificationConfig{
        Placement:    ui.NotifyBottomRight,
        PlacementSet: true,
        Duration:     8 * time.Second,
        MaxCount:     4,
        ShowProgress: true,
    }).Body(func() {
        app := ui.UseApp(c)
        if ui.Button(c, "发布完成").Clicked() {
            app.Notification("success", "发布完成")
        }
    })
}
```

### 消息与通知共用一个句柄

`AppAPI` 同时提供 `Message`、`Notification`、`Confirm` 和 `Info`；把句柄传给页面回调，可以让页面动作不关心反馈实例如何配置。

```go
import "github.com/tangoui/tango/ui"

func appActionsPage(c *ui.Context) {
    ui.App(c).Body(func() {
        app := ui.UseApp(c)
        if ui.Button(c, "同步").Clicked() {
            app.Message("loading", "正在同步")
            app.Notification("info", "同步任务已提交")
        }
    })
}
```

### 从 ConfigProvider 设置 App 默认数量

`ConfigProvider.Defaults("App", ...)` 可以给作用域内创建的 App 设定消息和通知的数量上限；单个 `AppMessageConfig` 或 `AppNotificationConfig` 的值会覆盖默认值。

```go
import "github.com/tangoui/tango/ui"

func appDefaultsPage(c *ui.Context) {
    ui.ConfigProvider(c).Defaults("App", ui.Props{
        "messageMaxCount":       2,
        "notificationMaxCount":  3,
    }).Body(func() {
        ui.App(c).Body(func() {
            app := ui.UseApp(c)
            if ui.Button(c, "打开消息").Clicked() {
                app.Message("success", "最多保留两条")
            }
        })
    })
}
```

### 与布局一起使用

App 自身会在父容器中建立填满剩余空间的纵向列；需要额外布局选项时使用 `Layout`，反馈调用仍然放在 `Body` 的子树内。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func appLayoutPage(c *ui.Context) {
    ui.App(c).Layout(layout.Pad(16)).Body(func() {
        ui.Text(c, "应用内容")
        ui.Button(c, "操作").Primary()
    })
}
```

## 主题与样式

- App 不单独重置文字样式；内容仍读取当前主题的文字、背景和字体令牌。
- Message、Notification 和 Confirm 的外观、颜色和动画分别由对应组件令牌控制；App 只提供实例配置和子树边界。
- `ConfigProvider.Defaults("App", ...)` 支持 `messageMaxCount` 和 `notificationMaxCount` 数值键；Duration、Placement 等实例配置通过 `AppMessageConfig` / `AppNotificationConfig` 传入。

## 注意事项

- **状态放在哪**：App 不保存业务状态。点击次数、表单数据和请求状态放在页面变量或 `ui.State` 中；App 的配置可以每帧重建，但应保持值稳定。
- **取最近的 App**：嵌套 App 时，`UseApp` 返回最内层容器的配置；没有 App 时返回使用普通默认值的句柄。
- **数量限制**：`MaxCount` 只限制由该 `UseApp` 句柄打开的反馈；普通的顶层 `Message` / `Notification` 不会读取 App 的句柄配置。
- **无障碍**：反馈文案应说明结果，加载消息不应永久显示；触发反馈的按钮仍需提供清晰的可见或可访问名称。
- **RTL**：通知位置使用 `NotifyPlacement`，逻辑位置会遵循当前方向；需要固定物理边时应按对应通知 API 选择物理位置。

## 示例代码

<!--@include: ./_gen/app-examples.md-->

## API

<!--@include: ./_gen/app-api.md-->

## 相关链接

- 源码：[ui/pro4_app.go](https://github.com/tangoui/tango/blob/main/ui/pro4_app.go)
- [ui Go API](/reference/api/ui)
- 相关组件：[Message 消息提示](/components/message)、[Notification 通知提醒](/components/notification)、[ConfigProvider 全局配置](/components/configprovider)
