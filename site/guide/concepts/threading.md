# 线程模型

GUI 工具包几乎都要求界面操作在**主线程**上进行。TanGo 把这件事收在核心层：你可以在任意 goroutine 里调用公开 API，核心负责把需要主线程的工作送过去。

## 规则

1. `App.Run` 在 `main` goroutine 里调用。桌面上根包在 `init()` 里 `runtime.LockOSThread()`，保证它就是进程主线程。
2. 回调（`OnReady`、`OnLifecycle`、`OnMessage`、`Surface.OnClose`、菜单点击、托盘点击……）**都在主线程上运行**。保持简短，耗时工作放到 goroutine 里。
3. 其他公开方法（`Surface.SetTitle`、`clipboard.ReadText`、`dialog.Open`……）任意 goroutine 都能调用，内部会切到主线程。

## Post、Await、AwaitAsync

```go
// 排队到主线程执行，立即返回
app.Post(func() { s.SetTitle("done") })

// 在主线程执行并取回结果；调用方已在主线程时直接执行
size := tango.Await(app, func() int { return len(app.Surfaces()) })

// 在主线程启动异步工作，阻塞调用方直到 done 被调用
path := tango.AwaitAsync(app, func(done func(string)) {
    showPickerSomehow(func(p string) { done(p) })
})
```

`AwaitAsync` 的关键点：**如果调用方本身就在主线程**（比如在菜单回调里弹对话框），它不会傻等，而是循环驱动原生事件（`Backend.Step`）直到 `done` 被调用。所以嵌套使用不会死锁——`dialog.Message` 在点击回调里直接调用就能工作。

## 退出之后

`Run` 返回后：`Post` 丢弃函数，`Await` / `AwaitAsync` 不执行任何东西、返回零值，Surface 方法什么也不做，`NewSurface` 返回 `ErrNotRunning`。

在 `Run` 之前的 `main` goroutine 上调用 `Await` / `AwaitAsync` 会死锁，所以它们会 **panic** 提示你；把这类代码放进 `OnReady`。

## 自绘窗口的 UI 线程

自绘界面（`ui` 包）的页面函数、`ui.Runner` 的方法和回调运行在窗口的 **UI 线程**上：

- **macOS 上的 `uihost.Open` 窗口**：每个窗口一条自己的 UI 线程，从 `Open` 返回前启动、窗口关闭后退出，与主线程和其他窗口真正并行。按设计，主线程被 AppKit 占住（菜单跟踪、模态对话框）时窗口照常出帧。
- **其他平台、`uihost.Attach`、`tan dev` 里的窗口、无头 `ui.Driver`**：UI 线程就是主线程（`Driver` 是调用者的线程）。

规则（所有平台都这样写，代码就不用分平台）：

1. 改页面读取的状态、调用 `Runner` 的方法：`r.Post(fn)`。任何 goroutine 可调用，在两帧之间按调用顺序运行，之后自动请求一帧；在 UI 线程上调用时排到当前帧之后；窗口关闭后丢弃。
2. 上面"都在主线程"的回调（菜单、托盘、`uihost.Shortcut`、`OnMessage`、`Surface.OnClose`）**不在** UI 线程上；要改页面状态就在回调里 `r.Post`。
3. 必须在主线程的调用（`uihost.Open`、`uihost.Attach`、`uihost.FollowSystemTheme`）从页面里用 `app.Post` 或 `tango.Await` 送过去。反过来，`Runner` 的方法只能在 UI 线程上调用，`Post`、`Invalidate`、`InvalidateAt`、`Close` 除外（任何 goroutine 都安全）。
4. `dialog` 等阻塞接口可以直接在 UI 线程上调用：只阻塞这个窗口。`Surface` 的方法在 UI 线程上调用时同步等主线程执行，主线程忙时这个窗口也跟着等。
5. 需要同步回答的主线程回调（`OnClose`、`OnQuitRequest`）不能等 UI 线程，见 [迁移说明](https://github.com/tangoui/tango/blob/main/docs/release.md#迁移说明) 的退出写法。

```go
uihost.Shortcut(s, "CmdOrCtrl+N", func() { r.Post(newEntry) }) // 主线程回调 → UI 线程
app.Post(func() {
    uihost.FollowSystemTheme(r, false)                    // 主线程
    r.Post(func() { r.SetTheme(theme.ElementPlus()) })    // UI 线程
})
```

开发构建（`-tags tango_dev`）只对 macOS 上直接运行（`go run -tags tango_dev`、`automation.Launch`）的 `uihost.Open` 窗口检查调用线程，违规时 panic；`tan dev`、`Attach` 和其他平台不检查，应用自己的变量也查不到。建议用 `go test -race` 跑一遍。

## 移动端的差异

移动端主线程属于原生宿主（iOS 的 `UIApplicationMain`、Android 的 Activity），Go 的 `main` 在一个普通 goroutine 里运行，不锁线程。规则对用户代码不变：回调仍在主线程，公开 API 仍可从任意 goroutine 调用。

有一处要注意：Android 的主线程不能嵌套运行事件循环。在主线程回调里调用需要等待用户的函数（对话框、分享、权限请求等）时，`tango.TryAwaitAsync` 返回 `tango.ErrNestedWait`，`AwaitAsync` 立即返回零值。**在 Android 上请从 goroutine 调用这类函数**：

```go
btn.OnClick(func() {
    go func() {
        st, err := permission.Request(app, permission.Camera)
        // ...
    }()
})
```

细节见 [移动端](/guide/mobile)。
