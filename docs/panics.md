# panic 处理

框架调用应用代码的每个边界都会 recover：一个回调 panic 不会让整个应用直接崩溃，而是交给统一的处理。

## 拦截范围

| `Panic.Where` | 场景 |
|---|---|
| `main-thread callback` | `App.Post`、`Await`、`AwaitAsync` 在主线程执行的函数 |
| `OnReady` | `App.OnReady` 回调 |
| `event handler` | 各类 `OnXxx` 处理器：生命周期、`OnMessage`、`OnEvent`/`OnOpenURL`、`OnQuitRequest`、`Surface.OnClose/OnClosed/OnLoaded/OnFocused/OnResized/...`、WebView 的权限/导航/新窗口/认证/下载处理器、`DragFiles` 的 done；sys 包的 `power`/`network`/`notify`/`jumplist`/`purchase`/`single` 的 `OnXxx`、`location.Watch`、`flyout` 的 `OnOpen`/`OnClose`、`hotkey.Bind` 等 |
| `menu item` | `sys/menu` 菜单项的 `OnClick`（应用菜单、窗口菜单、托盘菜单、右键菜单） |
| `tray click` | `sys/tray` 的 `OnClick`/`OnRightClick`/`OnDoubleClick` |
| `hotkey` | `hotkey.Register` 的全局快捷键回调 |
| `background task` | `background.OnTask` 的任务函数（只报告：任务照常按失败报告给系统，不退出） |
| `canvas callback` | 画布窗口的帧、输入、尺寸、焦点、拖放回调 |
| `ui frame` | uihost 的 Go 自绘帧（含页面函数） |
| `goroutine` | `App.Go` 启动的 goroutine |
| `binding Svc.Method` / `stream Svc.Method` | ipc 绑定调用与流（只报告，见下） |

一次 panic 只影响当前这个回调：主线程队列继续执行后面的任务，同一事件的其他处理器照常调用。有返回值的处理器 panic 时按"没有意见"处理：`OnClose`/`OnQuitRequest` 视为允许关闭/退出，权限、新窗口、认证走后续处理器或默认策略，导航处理器 panic 则**拦截**这次导航，下载处理器 panic 取消下载。

在主线程之外调用 `Await` 而函数 panic 时返回零值；`TryAwaitAsync` 返回 `ErrPanicked`。在主线程上直接调用时 panic 照常传给调用方（再由外层边界拦截）。

## 默认行为（`Options.OnPanic` 为 nil）

1. 用 `log/slog` 以 Error 级别记录：`where`、`panic`（值）、`stack`（完整调用栈）。
2. 开发构建（`-tags tango_dev`，`tan dev` 自动加）：继续运行。
3. 生产构建（默认，即普通 `go build`/`go run`/`tan build`）：弹出原生错误对话框"<应用名> 遇到问题，需要关闭。"（系统语言非中文时为英文），附简短错误信息，用户关闭后以退出码 2 退出。主线程卡住 5 秒内无法弹框时直接退出。见 [开发功能与生产构建](dev-builds.md)。

## 自定义：`Options.OnPanic`

```go
app := tango.New(tango.Options{
    Name: "Demo",
    OnPanic: func(p tango.Panic) (keepRunning bool) {
        slog.Error("panic", "where", p.Where, "value", p.Value, "stack", string(p.Stack))
        report(p)        // 上报崩溃
        return false     // false：以退出码 2 退出；true：继续运行
    },
})
```

`OnPanic` 可能在任意 goroutine 上调用。返回 false 时框架不再弹默认对话框，直接退出（需要提示就在 `OnPanic` 里自己做）。`OnPanic` 自身 panic 时两次 panic 都会记日志，并按默认规则决定是否退出。

ipc 绑定和流的 panic 仍按原样变成 `internal` 错误（`"panic: ..."`）返回给页面，同时报告给 `OnPanic`/默认处理一次，但**只报告不退出**，`keepRunning` 对它们不生效。连接不是 `*tango.Surface` 时，可以用 `Router.OnPanic` 自己接收，否则只写 slog 日志。

## `App.Go` 与裸 goroutine

```go
app.Go(func() { work() })   // panic 走上面的处理，Where = "goroutine"
go work()                   // panic 框架拦截不到：进程立即崩溃
```

Go 的 panic 只能在发生它的 goroutine 里 recover。用 `go` 语句直接启动的 goroutine 框架无从插手，其中的 panic 会按 Go 运行时的默认方式打印栈并终止进程，不写 slog、不弹框。应用自己的 goroutine 请用 `App.Go`，或在里面自己 `defer recover()`。

## 日志位置

默认处理写到 `slog.Default()`。没有配置时它经标准库 `log` 输出到 stderr（`tan dev` 下即终端）。要写入文件，用 `service/log` 创建 Logger 并设为默认：`l, _ := applog.New(applog.Options{AppID: "com.example.demo"}); slog.SetDefault(l.Logger)`，之后 panic 日志同时写入应用日志目录下按大小轮转的 `app.log`（`l.Path()`）。
