# 更新日志

本文件记录 Tango 各版本的用户可见变化，格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。Go 模块、`cmd/tan` 与 `@tangoui/runtime` 使用同一个版本号，见 [docs/release.md](docs/release.md)。

## [未发布]

### 新增

- **公开 API 分层与兼容性检查**：Go 包分为稳定与实验性两层（`ui`、`uihost`、`mobile`、`service/updater` 为实验性）；CI 新增 `API` 工作流，用 `scripts/apidiff.sh`（基于 apidiff）检查稳定包的不兼容变化，需要破坏兼容时给 PR 加 `api-break` 标签。见 [docs/release.md](docs/release.md#api-稳定性)。

### 安全

- **开发功能改为编译期开关 `tango_dev`**：默认构建（普通 `go build`/`go run`、goreleaser、`tan build` 含 `-debug`）即生产构建，不再响应 `TANGO_DEV_URL`、`TANGO_AUTOMATION*`、`TANGO_DEV_ROLE`、`TANGO_DEV_WINDOW_STATE` 等开发用环境变量，热加载宿主、自动化通道、开发代理不链接进二进制；未处理的 panic 默认弹框退出。`tan dev` 自动加 `-tags tango_dev`；`automation.Launch` 构建被测程序时也加。`tango_release` 标签不再起作用，仅作兼容保留。见 [docs/dev-builds.md](docs/dev-builds.md)。
- `asset.DevProxy` 默认只接受回环目标（`DevProxyOptions.AllowRemote` 放开）。签名变为 `DevProxy(target string, opts ...DevProxyOptions)`，原有调用不用改；把它当作 `func(string) http.Handler` 类型的值使用的代码需要包一层。
- 资源源上 GET/HEAD 以外的跨源请求回 403（`Options.AllowCrossOriginAssets` 关闭）；macOS/iOS 后端用主文档 URL 补上 WebKit 缺失的 `Origin`。
- `tan dev` 热加载控制通道要求会话 token（`TANGO_DEV_TOKEN`），unix socket 权限 0600。

## [0.1.0] - 未发布

首个公开版本。0.x 期间公开 API 可能有不兼容的变化。

**验证状态**：桌面端只在 macOS 实机验证；Windows、Linux 后端已实现，通过交叉编译与 CI 冒烟，尚未在实机上系统验证。iOS、Android 为实验性，只在模拟器上验证。自绘 UI 为实验性，只在 macOS 上实机验证。

### 新增

- **核心运行时**（根包 `tango`）：`App` / `Surface`，主线程队列与 `Post`、`Await`、`AwaitAsync`、`TryAwaitAsync`；生命周期事件、统一的应用事件（深链、打开文件、主题与显示器变化、Dock 重新打开、通知点击）、用户退出拦截、关闭最后一个窗口时的平台默认行为。
- **桌面后端**：macOS（AppKit + WKWebView）、Windows（Win32 + WebView2）、Linux（GTK 3 + WebKitGTK 4.1），均不需要 cgo；资源通过自定义 scheme 提供，不开本地端口。
- **窗口**：位置、尺寸限制、状态、居中、外观（隐藏标题栏 / 无边框、透明、毛玻璃与 Mica、置顶、不透明度）、面板 / 模态 / 表单窗口、无边框拖动区域、文件拖入与拖出、缩放、打印与导出 PDF、截图、页内查找、Cookie 与网站数据、User-Agent 与请求头。
- **WebView 策略**：导航策略、页面权限请求、新窗口请求、下载、HTTP 认证的处理函数。
- **IPC**（`ipc`、`bind`）：调用、双向事件、可取消且带背压的流（JS 打开或 Go 打开），反射绑定；`tan generate bindings` 生成 TypeScript 客户端。
- **前端运行时** `@tangoui/runtime`：`call` / `on` / `emit` / `stream`，以及 `Window`、`App`、`Theme`、`Screens`、`Clipboard`、`Dialog`、`Notify`、`Shell`、`Menu`、`Hotkey`、`Events` 模块；Go 侧 `jsapi` 按能力白名单开放，默认只开窗口自身、右键菜单和只读信息。
- **安全**：只接受资源 origin 与核心导航目标 origin 的消息；`shell` 协议白名单；`service/fileserver` 限制在目录内。
- **系统能力**（`sys/`）：剪贴板（文本、图片、文件）、对话框、菜单（应用、窗口、右键）、托盘、通知、全局与窗口快捷键、显示器、系统主题与页面同步、打开 URL 与文件、应用控制（徽标、进度、请求注意、Dock 图标）、开机启动、运行环境、最近文档、单实例、窗口状态记忆。
- **移动端**（实验性）：iOS（c-archive + 原生宿主）与 Android（c-shared + Java 宿主），用户代码零改动；权限、分享、触感、安全区、屏幕方向、生物识别、安全存储、相册与拍照、定位、远程推送（APNs；FCM 需自行接入）。
- **服务**（`service/`）：服务生命周期、应用目录、键值存储、日志、文件服务；签名的自动更新（`service/updater`：防降级、二进制补丁、启动确认与回滚，实验性）。
- **多语言**（`i18n`）：系统语言、语言匹配（简繁分流）、带具名参数与 CLDR 复数的文案目录；原生菜单标准项和对话框按钮支持 en、zh-CN、zh-TW、ja。
- **自绘 UI**（实验性，`ui`、`uihost`）：即时模式 API、Ant Design 与 Element Plus 两套主题令牌、约 90 个组件、无头 `Driver` 测试、`tan dev` 热加载；macOS 上 WebView 与自绘同窗（`uihost.Attach`）。
- **命令行 `tan`**：`init`（多种模板）、`dev`、`build`（桌面、iOS、Android）、`run`、`package`（dmg、NSIS、MSIX、tar.gz、AppImage）、`sign`（macOS 签名与公证、Windows Authenticode）、`keygen`、`release-manifest`、`sign-update`（更新清单与签名）、`generate bindings`、`doctor`。
- **示例**：`hello`、`runtime-demo`、`showcase`（IPC、流、系统能力、托盘、菜单、通知、主题跟随，带 `-smoke` 自检）等。
- **工程**：CI（lint、三平台测试与真实窗口冒烟、iOS 模拟器与 Android 模拟器冒烟）、文档站（VitePress，中文为主、英文首页与核心页面）。

[未发布]: https://github.com/tangoui/tango/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/tangoui/tango/releases/tag/v0.1.0
