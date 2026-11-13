# 安全模型

WebView 里跑的是 Web 内容，Tango 的默认设置假定**页面不完全可信**：页面能做什么由 Go 侧显式开放。

## 1. 消息来源校验

页面发给 Go 的每条消息都带着发送方的 origin。核心只接受两种来源：

- 应用的**资源 origin**（`tango://app` 或 `https://tango.localhost`）；
- 核心**最后一次导航**该 Surface 去的 origin（`SurfaceOptions.URL` 或 `Surface.Navigate` 指定的绝对地址）。

其他来源的消息直接丢弃，不会到达 `App.OnMessage`，也就到不了 IPC 和 `jsapi`。页面里点链接跳到第三方网站后，那个网站无法调用你的 Go 方法。另外后端只投递**主框架**的消息，iframe 里的页面不能直接发消息给 Go。

## 2. jsapi 能力白名单

页面调用系统能力（剪贴板、对话框、通知……）走 `jsapi` 包。能力按组开关，生效集合 = `(Defaults | Allow) &^ Deny`：

```go
jsapi.Install(app, r, jsapi.Options{
    Allow: jsapi.ClipboardWrite | jsapi.Dialogs,
})
```

| 能力 | 默认 | 内容 |
|---|---|---|
| `Window` | 开 | 调用方窗口控制自己（标题、尺寸、状态……） |
| `Menu` | 开 | 在调用方窗口弹出右键菜单，只允许编辑类 role |
| `Info` | 开 | 环境、主题、屏幕（只读）及相关事件 |
| `AppControl` | 关 | 退出、隐藏应用、徽标、进度、请求注意 |
| `ClipboardRead` / `ClipboardWrite` | 关 | 读 / 写剪贴板（读也默认关：剪贴板里常有密码） |
| `Dialogs` | 关 | 消息框、打开、保存对话框 |
| `Notify` | 关 | 系统通知 |
| `OpenURL` / `OpenPath` | 关 | 用默认程序打开 URL / 文件 |
| `Hotkeys` | 关 | 全局快捷键 |

没开的能力调用时 reject `forbidden`，相关事件也不转发。窗口类方法**只作用于发起调用的那个窗口**（`ipc.ConnFrom(ctx)` 识别），页面不能指定别的窗口。完整对照见 [前端运行时 API](/reference/js-api)。

## 3. 收窄的危险操作

- **URL 协议白名单**：`shell.OpenURL`（以及页面的 `Shell.openURL`）只接受 `http`、`https`、`mailto`；`shell.AllowSchemes("tel")` 可以修改，`file:` 永远不允许（用 `OpenPath`）。页面不能靠传一个字符串让应用启动任意协议处理程序或本地程序。
- **菜单 role**：页面描述的右键菜单只能用 `copy`、`paste`、`cut`、`selectAll`、`undo`、`redo`，`quit` 之类返回 `forbidden`。
- **全局快捷键归页面所有**：页面重载或窗口关闭时自动注销，别的页面注销不了。
- **文件服务**：`service/fileserver` 拒绝 `..`，通过 `os.Root` 打开文件，不跟随指向目录外的符号链接。

## 4. WebView 策略

根包提供几个由 Go 决定的钩子（Surface 自己的处理函数优先，然后是 App 级，最后是内置默认）：

```go
// 主框架导航前询问；返回 false 取消。应用自己发起的导航不询问。
app.SetNavigationPolicy(func(s *tango.Surface, nav tango.NavigationRequest) bool { ... })

// 页面请求摄像头、麦克风、定位等权限
app.SetPermissionPolicy(tango.WebPermCamera, tango.PermissionDeny)
app.SetPermissionHandler(func(s *tango.Surface, req tango.PermissionRequest) tango.PermissionDecision { ... })

// window.open / target=_blank
app.SetNewWindowHandler(func(s *tango.Surface, req tango.NewWindowRequest) tango.NewWindowAction { ... })
```

默认值：没有策略时权限请求交给平台询问用户；新窗口请求中，本应用 origin 的页面开新 Surface，`http(s)` 和 `mailto` 交给系统浏览器，其他一律拒绝。

## 5. 开发功能与生产构建

默认构建就是生产构建：普通 `go build`、`go run .`、goreleaser、`tan build`（含 `-debug`）都不含开发期代码，也不响应开发用的环境变量（`TANGO_DEV_URL`、`TANGO_AUTOMATION`、`TANGO_DEV_ROLE` 等）。热加载、自动化通道、前端开发代理只在 `-tags tango_dev` 构建里存在，`tan dev` 会自动加。这是编译期隔离：与应用同用户的进程能随意设置环境变量（如 `open -a App --env …`），运行时检查挡不住。原来的 `tango_release` 标签已无作用。

资源源上 GET/HEAD 以外的请求只接受资源源自身的 `Origin`，外站页面向 `<资源源>/api` 发的表单 POST 会得到 403（`tango.Options.AllowCrossOriginAssets` 可关闭）。

不带 `-debug` 时页面不能打开 Web 检查器（还取决于 `SurfaceOptions.Debug`）。建议通过 `asset.Options.Headers` 给页面加 CSP。

## 你仍需要做的

- 自己用 `bind` / `Handle` 暴露的方法同样会被页面调用：校验参数，别把「执行任意命令」「读任意路径」这类能力直接交给页面。
- 加载远程内容时，考虑用导航策略把页面限制在可信域名内。
- 发现安全问题请按仓库根目录的 [SECURITY.md](https://github.com/tangoui/tango/blob/main/SECURITY.md) 私下报告。
