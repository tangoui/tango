# 线程模型

GUI 工具包几乎都要求界面操作在**主线程**上进行。Tango 把这件事收在核心层：你可以在任意 goroutine 里调用公开 API，核心负责把需要主线程的工作送过去。

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
