# 生命周期与应用事件

## 启动与退出

```go
app.OnReady(func() { /* 应用已启动，在主线程：创建窗口 */ })
app.OnLifecycle(func(ev tango.Lifecycle) {
    switch ev {
    case tango.Started:      // 与 OnReady 同时
    case tango.Foreground:   // 移动端回到前台
    case tango.Background:   // 移动端进入后台：保存状态
    case tango.Terminating:  // 即将退出；之后 Run 返回
    }
})
```

- `OnReady` 在 `Started` 后的主线程上调用；注册时应用已启动的话，会尽快补调一次。
- 桌面上 `Foreground` / `Background` 主要用于移动端，桌面窗口的焦点用 `Surface.OnFocused` / `OnBlurred`。
- `App.Quit()` 可以在任意 goroutine 调用：核心投递 `Terminating`，`Run` 返回，`main` 函数继续执行它的 defer。
- 移动端系统终止应用时同样投递 `Terminating`，并最多等 2 秒让 `main` 跑完。

### 拦截用户退出

```go
app.OnQuitRequest(func() bool {
    return !hasUnsavedChanges() // 返回 false 取消这次退出
})
```

只拦截**用户**发起的退出（Cmd+Q、退出菜单项）；代码里调用 `App.Quit` 不会被拦截。

### 关闭最后一个窗口

`Options.QuitOnLastClose`：`nil` 时 macOS 不退出、Windows 和 Linux 退出；移动端忽略。带托盘的应用通常设为 `false`，见 [菜单与托盘](/guide/menu-tray)。

辅助窗口不算在内：用 `SurfaceOptions{Auxiliary: true}` 创建的窗口（更新窗口、工具面板这类）不会让应用继续运行，最后一个普通窗口关闭时应用照常退出，辅助窗口随之关闭；关掉辅助窗口本身从不让应用退出（例如没有普通窗口的后台应用只弹出了更新提示）。`updateui` 的更新窗口就是辅助窗口。

## 应用事件

和窗口无关的系统通知统一走 `App.OnEvent`（或只取一种的 `OnEventKind`），事件里带 `Kind` 和对应数据：

| Kind | 何时 | 数据 |
|---|---|---|
| `EventOpenURL` | 深链（自定义 scheme、通用链接 / App Links），来自启动参数、第二实例或系统 | `URL` |
| `EventOpenFiles` | 文件关联、拖到 Dock 图标、「打开方式」 | `Paths` |
| `EventThemeChanged` | 系统深浅色或强调色变化 | `Theme` |
| `EventDisplaysChanged` | 显示器插拔、排列或缩放变化 | |
| `EventReopen` | macOS：没有可见窗口时点击 Dock 图标 | |
| `EventNotificationOpened` | 点击了上一次运行发出的通知（包括点通知启动应用） | `Notification` |
| `EventSafeAreaChanged` 等 | 移动端：安全区、方向、推送令牌/消息、WebView 被系统回收后重建 | 见 [移动端](/guide/mobile) |

常用的有快捷方法：

```go
app.OnOpenURL(func(u string) { route(u) })
app.OnOpenFiles(func(paths []string) { for _, p := range paths { open(p) } })
```

启动应用的那个深链会在 `OnReady` 之前到达，核心先保存起来，注册监听后补发，所以不会丢。`Options.URLSchemes` 声明应用处理的 scheme；向系统注册由打包完成（`tan.json` 的 `urlSchemes`）。

## 单实例

桌面上第二次启动应用时，通常希望把参数交给已经在运行的那个实例：

```go
primary, err := single.Claim(app, single.Options{})
if !primary {
    return // 已转交给正在运行的实例
}
```

第二实例的深链和文件参数会在主实例里按 `EventOpenURL` / `EventOpenFiles` 投递，并把第一个窗口显示到前台。详见 [single](/guide/system/single)。

## 服务的生命周期

`service` 包把长期存在的 Go 值挂到应用生命周期上：实现 `Startup(ctx)` 的在应用就绪后按注册顺序启动，实现 `Shutdown()` 的在退出时逆序关闭。

```go
service.Register(app, db, syncer)
service.Bind(app, r, "Notes", notes) // 同时把导出方法注册为 Notes.*（Startup/Shutdown 不暴露）
```

任何一个 `Startup` 失败，已启动的服务会被关闭、应用退出，`service.Err(app)` 给出原因。
