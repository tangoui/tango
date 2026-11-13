# App 与 Surface

Tango 的根包 `github.com/tangoui/tango` 只有两个核心类型：**App** 是整个应用，**Surface** 是一块界面。

## App

```go
app := tango.New(tango.Options{
    Name:       "Notes",                 // 显示名
    ID:         "com.example.notes",     // 反向域名 ID：单实例、配置目录、通知都用它
    Assets:     asset.FS(site, asset.Options{}), // 页面资源，见「资源服务」
    URLSchemes: []string{"notes"},       // 处理 notes://... 深链
})
app.OnReady(func() { /* 创建窗口 */ })
if err := app.Run(); err != nil {        // 阻塞到应用退出
    log.Fatal(err)
}
```

App 拥有原生事件循环和一个**主线程工作队列**。`Run` 必须在 `main` goroutine 里调用（根包在 `init` 里把它锁到进程主线程上）；其余方法都可以从任意 goroutine 调用。

`Options.QuitOnLastClose` 决定关掉最后一个窗口时是否退出：`nil` 用平台习惯——macOS 不退出（应用留在 Dock 里），Windows 和 Linux 退出。

## Surface

Surface 在桌面上是一个**窗口**，在移动端是一个**页面**（iOS 的视图控制器、Android Activity 里的一层）。默认承载一个系统 WebView：

| 平台 | WebView | 资源 origin |
|---|---|---|
| macOS | WKWebView | `tango://app` |
| Windows | WebView2 | `https://tango.localhost` |
| Linux | WebKitGTK 4.1 | `tango://app` |
| iOS | WKWebView | 见 [移动端](/guide/mobile) |
| Android | 系统 WebView | 见 [移动端](/guide/mobile) |

```go
s, err := app.NewSurface(tango.SurfaceOptions{
    Title: "Notes", Width: 960, Height: 640, Resizable: true,
    URL:   "/index.html",   // 相对路径按资源 origin 解析；也可以是绝对 URL
    Debug: true,            // 允许打开 Web 检查器
})
s.OnClose(func() bool { return confirmDiscard() }) // 用户关窗时询问；返回 false 不关
s.OnClosed(func() { r.Drop(s) })
```

在 `OnReady` 里（或之后）创建 Surface。`Run` 之前在 `main` goroutine 里调用 `NewSurface` 会返回 `ErrNotRunning`，而不是死锁。

窗口位置、大小、状态、外观、拖放文件等见 [窗口与外观](/guide/window)。

## 自绘 Surface

同一个 App 里也可以打开 Go 自绘的窗口，或者把自绘层叠在 WebView 窗口上：

```go
runner, win, err := uihost.Open(app, uihost.Options{ /* 标题、尺寸 */ }, page, ui.RunnerOptions{})
runner, att, err := uihost.Attach(webSurface, uihost.Layer{Above: true}, page, ui.RunnerOptions{})
```

`uihost.Attach`（WebView 与自绘同窗）目前只在 macOS 实现。详见 [自绘 UI](/guide/ui)。

## 原始消息

页面和 Go 之间最底层只有一条**字符串消息**通道：

- 页面：`window.__tango.post(str)` 发给 Go，`window.__tango.onMessage(fn)` 接收；
- Go：`App.OnMessage(func(s *tango.Surface, msg string))` 接收，`Surface.Send(msg)` 发出。页面加载完成前 `Send` 的消息会缓存，加载后按顺序送达。

调用、事件、流都建立在它之上，见 [IPC](/guide/concepts/ipc)。不需要 IPC 的场景（比如只想把一段 JSON 推给页面）直接用原始消息也可以。

## 能力包

窗口之外的系统能力每个一个包，放在 `sys/` 下，第一个参数总是 `*tango.App`：

```go
import "github.com/tangoui/tango/sys/clipboard"

text, err := clipboard.ReadText(app)
if errors.Is(err, clipboard.ErrUnsupported) { /* 当前平台没有 */ }
```

平台不支持时返回 `ErrUnsupported`，应用退出后返回 `tango.ErrNotRunning`。完整列表见 [系统能力](/guide/system/)。
