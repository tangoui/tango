# 快速开始

本页从零做出一个 Tango 应用：安装命令行工具 `tan`，用模板创建项目，开发模式运行，再做发布构建。桌面和移动端各走一遍。

::: warning 当前状态
Tango 是 0.x 早期版本。桌面端只在 **macOS** 上实机验证过；Windows、Linux 的后端已实现并通过交叉编译与 CI 冒烟，但尚未在实机上系统验证。移动端（iOS、Android）属于**实验性**，只在模拟器上验证过，没有上真机。
:::

## 准备环境

| | 需要 |
|---|---|
| 所有平台 | Go 1.27 或更新版本 |
| macOS | Xcode 命令行工具（签名、`lipo`、`hdiutil`）；系统自带 WKWebView |
| Windows | WebView2 运行时（Windows 10 1803+ / 11 一般已自带）；打安装包需要 NSIS（`makensis`） |
| Linux | GTK 3、WebKitGTK 4.1、（托盘）libayatana-appindicator3；打 AppImage 需要 `appimagetool` |
| 前端模板（可选） | Node.js 与 npm：只有基于 Vite 的模板需要；默认的 `vanilla` 和 `ui` 模板不需要 |
| iOS（可选） | macOS + Xcode（含模拟器） |
| Android（可选） | JDK 17、Android SDK 与 NDK |

桌面端编译**不需要 cgo**：后端用 purego / syscall 调用系统库，所以可以在一台机器上交叉编译三个桌面平台。移动端用 cgo（`c-archive` / `c-shared`），由 `tan build` 处理。

## 安装 tan

```bash
go install github.com/tangoui/tango/cmd/tan@latest
tan version
tan doctor        # 检查工具链和平台依赖，缺什么会列出来
```

也可以从 [Releases](https://github.com/tangoui/tango/releases) 下载对应平台的 `tan` 二进制。

::: details 还没有发布版本时：从源码安装
```bash
git clone https://github.com/tangoui/tango
cd tango/cmd/tan && go install .
```
用本地仓库创建项目时加 `-local`，生成的 go.mod 会用 replace 指向这个目录：`tan init -local ~/src/tango hello`。
:::

## 创建项目

```bash
tan init hello        # 指定目录：直接用默认值创建
tan init              # 在终端里不带参数：逐项询问目录、名字、ID、模板
tan init -yes         # 不询问，全部用默认值（目录 tango-app）
cd hello
```

`tan init` 的参数（参数写在目录前面）：

| 参数 | 默认 | 说明 |
|---|---|---|
| `-template` | `vanilla` | 项目模板，见下表 |
| `-name` | 目录名 | 应用显示名 |
| `-id` | `com.example.<name>` | 反向域名形式的应用 ID（Bundle ID / applicationId） |
| `-module` | 目录名 | Go 模块路径 |
| `-mobile` | `true` | 在 tan.json 写入 `ios`、`android` 两节（安全的默认值，不含签名） |
| `-local` | | 使用本地 Tango 仓库（replace） |
| `-yes` | `false` | 不询问 |

| 模板 | 内容 |
|---|---|
| `vanilla` | 纯 HTML/CSS/JS，不需要 npm |
| `vanilla-ts` | TypeScript + Vite |
| `react-ts` | React + TypeScript + Vite |
| `vue-ts` | Vue + TypeScript + Vite |
| `shadcn` | React + TypeScript + Vite，带 Tailwind CSS 与 shadcn/ui 组件，隐藏标题栏 |
| `ui` | Go 自绘界面（`ui` 包），没有 WebView、不需要 npm（实验性） |
| `hybrid` | WebView 页面（TypeScript + Vite）上叠一个自绘工具面板（实验性，同窗叠放目前只在 macOS 实现） |

模板列表以 `tan init -h` 为准。

生成的项目大致是：

```
hello/
├── tan.json        # 项目配置：名字、ID、版本、图标、前端命令、移动端设置
├── main.go         # Go 入口：App、IPC 路由、一个绑定的 Greeter 服务、主窗口
├── icon.png
└── frontend/       # 前端源码（vanilla 模板直接作为静态文件提供）
```

`main.go` 的核心只有几行（摘自模板）：

```go
app := tango.New(tango.Options{
    Name:   "hello",
    ID:     "com.example.hello",
    Assets: asset.FromEnv(files, asset.Options{SPA: true}), // tan dev 时代理到前端开发服务器
})

r := ipc.NewRouter()
bind.Register(r, "Greeter", &Greeter{})                  // 页面里 tango.call("Greeter.Hello", "Tom")
app.AddBridgeScript(runtimejs.Script)                     // 注入 window.tango 运行时
app.OnMessage(func(s *tango.Surface, msg string) { r.HandleMessage(s, msg) })

app.OnReady(func() {
    app.NewSurface(tango.SurfaceOptions{Title: "hello", Width: 960, Height: 640, Resizable: true})
})
app.Run()
```

## 开发模式：tan dev

```bash
tan dev
```

`tan dev` 会：

1. 有 `frontend.dev` 命令时（Vite 模板是 `npm run dev`）先装依赖、启动前端开发服务器，读出它打印的 URL，通过 `TANGO_DEV_URL` 交给应用（`tan dev` 用 `-tags tango_dev` 构建，只有这种开发构建才读这个变量），前端改动走 Vite 的 HMR；`vanilla` 模板直接提供 `frontend/` 目录；
2. 编译并运行 Go 程序；
3. 监视 Go 源码，变化后重新编译并重启应用，默认恢复窗口位置和大小（`-no-restore` 关闭）；
4. 应用里用到自绘 UI（`uihost`）时，默认对自绘窗口做**热加载**：窗口留在宿主进程里，只替换工作进程，见 [自绘 UI](/guide/ui#热加载)。

常用参数：`-url http://localhost:5173` 使用已在运行的开发服务器；`-C <dir>` 指定项目目录；`--` 之后的参数传给应用。

## 发布构建：tan build

```bash
tan build                                        # 本机平台
tan build -platform darwin/universal             # macOS 通用二进制（需要 lipo）
tan build -platform windows/amd64,linux/arm64    # 交叉编译
```

`tan build` 先执行 `frontend.build`（有的话），再编译生产构建（不带 `tango_dev`，没有热加载宿主、自动化通道、开发代理等开发期功能；直接 `go build`/`go run .` 得到的也是生产构建），产物放在 `build/<os>-<arch>/`：

| 平台 | 产物 |
|---|---|
| macOS | `<Name>.app`（写好 Info.plist，ad-hoc 签名） |
| Windows | `<Name>.exe`（嵌入图标、应用清单和版本信息） |
| Linux | 可执行文件 |

`-debug` 保留符号、允许打开 Web 检查器。要做安装包（.dmg、NSIS 安装器、.tar.gz / AppImage）用 `tan package`，签名与公证用 `tan sign`，见 [打包、签名与自动更新](/guide/packaging)。

## 移动端路径（实验性）

同一个项目不需要改代码就能构建到移动端。`tan.json` 里的 `name`、`id`、`version`、`icon` 会被共用；用到相机、定位等能力时在 `permissions` 里声明，见 [tan.json](/reference/tan-json)。

### iOS 模拟器

```bash
tan run -platform ios-simulator      # 构建、安装到模拟器并启动，Go 日志接到终端
tan build -platform ios-simulator    # 只构建 .app
tan build -platform ios              # 真机 .app + .ipa，需要签名身份和描述文件
```

### Android

```bash
tan run -platform android                    # 构建设备 ABI 的 debug APK，安装、启动、显示日志
tan build -platform android -debug           # debug APK（arm64-v8a + x86_64）
tan build -platform android/arm64,android/amd64   # release：签名的 APK 和 AAB
```

`tan run -platform android` 没有连接设备时会启动第一个 AVD（`-avd` 指定）。运行模型、能力列表、签名配置见 [移动端](/guide/mobile)。

## 下一步

- 想知道 `App`、`Surface`、主线程、IPC 是怎么配合的：[核心概念](/guide/concepts/app-surface)
- 页面里调用剪贴板、对话框、通知：[前端运行时 API](/reference/js-api) 与 [系统能力](/guide/system/)
- 看一个完整的演示应用：仓库里的 `examples/showcase`（`go run ./examples/showcase`）
