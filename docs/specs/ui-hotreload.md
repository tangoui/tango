# 自绘 UI 热加载（tan dev）

状态：设计稿，2026-11-12。依赖 canvas-surface A 步（macOS 上屏）完成后开工；排在 B 步（Windows/Linux 上屏）之前。

## 目标

`tan dev` 下修改自绘 UI 的 Go 代码后：窗口不关闭、不闪烁，编译完成后约 1 秒内显示新界面；可选保留应用状态（表单内容、当前页面）。发布构建零开销。

## 现状

`tan dev` 监视 Go 文件（400ms 轮询），变化后重新编译、杀掉旧进程、启动新进程：窗口关闭重开，状态丢失。WebView 前端走前端开发服务器的 HMR，不在本设计范围。

## 方案：宿主 / 工作进程

```
tan dev
 ├─ 宿主进程（tango 内置的 devhost，开发期间常驻）
 │    窗口（uihost / platform.Canvas）、输入、光标、输入法、剪贴板
 │        ▲ 画面（共享内存）      │ 输入事件 / 尺寸 / 焦点（管道）
 │        │                       ▼
 └─ 工作进程（用户应用，每次编译后替换）
      ui.Runner + 用户页面代码，渲染 CPU 帧
```

- 进入方式：`tan dev` 设置环境变量 `TANGO_UI_HOST=<socket>`。`uihost.Open` 发现它时，不创建本地窗口，而是连接宿主，请它用相同的 `SurfaceOptions` 开窗（已开则复用），并返回一个远程 `ui.Window`。用户代码不用改。
- 帧传输：宿主为每个窗口创建一块共享内存（`mmap` 临时文件；Windows 用 `CreateFileMapping`），大小按最大帧分配，尺寸变化时重建。工作进程渲染后写入并发送"帧就绪"消息，宿主调用 `Canvas.Present`。双缓冲，避免撕裂。
- 控制通道：Unix domain socket（Windows 用命名管道）上的 gob 消息。
  - 宿主 → 工作：`Input(event)`、`Resized(w,h,scale)`、`Focus`、`FrameRequest`、`Close`、`ClipboardText`。
  - 工作 → 宿主：`Open(opts)`、`FrameReady(id, w, h)`、`Invalidate`、`InvalidateAt`、`SetCursor`、`SetTextInput`、`WriteClipboard`、`SetTitle` 等 Surface 调用。
  - 剪贴板读取是同步请求 / 应答。
- 替换流程：新二进制编译成功 → 启动新工作进程并连接 → 旧进程收到 `Snapshot` 请求，返回状态字节（可选）→ 宿主把状态交给新进程 → 新进程首帧就绪后，宿主切换显示并结束旧进程。编译失败时保留旧进程，并在窗口角落叠加一条错误提示（宿主自己绘制）。
- 状态保留（可选 API）：

  ```go
  uihost.Persist(&state) // 指针指向可 gob 编码的值；热加载前编码，新进程启动后解码回填
  ```

  类型变化导致解码失败时，丢弃旧状态并在终端提示。
- WebView 窗口：仍按现状重启。宿主只接管 canvas 窗口；同一个应用里的 WebView 窗口由工作进程自己创建。
- 叠在 WebView 上的 canvas（`uihost.Attach` / `AttachWith`）不转发给宿主：`tan dev` 构建后若二进制链接了 `uihost.AttachWith`，不进热加载，改为整进程重启模式（日志说明原因）；该检查是静态的，链接了但运行时不调用也会走重启模式。
- 生产构建：远程窗口代码放在 `//go:build tango_dev` 文件里，只有 `tan dev` 加这个 tag；默认构建（`go build`、`tan build`）不携带 devhost。（原先是 `!tango_release`，`tango_release` 现在已无作用，见 [docs/dev-builds.md](../dev-builds.md)。）
- 控制通道认证：`tan dev` 每次会话生成随机 token，经 `TANGO_DEV_TOKEN` 传给宿主和工作进程；每个连接的 `Hello` 必须带上它（常量时间比较），否则宿主回 `Welcome{Err}` 并断开。Windows 上通道是 `127.0.0.1` TCP，宿主拿不到 token 时拒绝启动；Unix 上 socket 在 0700 的临时目录里，本身也设为 0600。

## 改动范围

- `uihost`：远程 Window 实现、`Persist`、环境变量检测。
- 新包 `internal/devhost`：宿主进程逻辑（复用 uihost + platform.Canvas）、共享内存、协议。
- `cmd/tan`：`dev` 启动宿主进程（可以是 tan 自身的子命令 `tan devhost`，编译时链接用户项目无关的平台代码），工作进程按热替换流程管理，构建都带 `-tags tango_dev`；`build` 不带（生产构建）。
- 顺带：`tan dev` 重启应用时恢复窗口位置和大小，见下节（没有用 `windowstate` 包：它 import 根包，根包无法反向使用）。

## 重启后恢复窗口位置

`tan dev` 改 Go 代码后整进程重启（WebView 应用），或框架代码变化时重启热加载宿主，窗口原本会回到默认位置。现在：

- `tan dev` 给应用进程（重启模式的应用、热加载的宿主进程，工作进程也继承但忽略）设置环境变量 `TANGO_DEV_WINDOW_STATE=<文件>`，文件放在 dev 构建目录：`<UserCacheDir>/tango/dev/<应用 ID>/windows.json`（与 dev 可执行文件同目录，跨 tan dev 会话保留）。`tan dev -no-restore` 不设置，即关闭此功能。
- 根包 `devstate.go`（`//go:build tango_dev`）：`New` 时若该变量存在且不是热加载工作进程（`TANGO_DEV_ROLE=worker`，它只有隐藏的替身窗口），注册 `OnSurfaceCreated`：
  - 窗口 key：标题（`Surface.Title()`）；标题为空用创建序号 `#n`；同一标题同时开着多个窗口时，后开的依次为 `标题#2`、`标题#3`。
  - 文件里有该 key 时：保存的外框位置（标题栏条带）仍落在某个显示器工作区内才 `SetBounds`（平台不能列显示器时不检查）；状态为 maximized / fullscreen 时再 `SetState`。最小化不保存、不恢复（最小化期间保留之前的记录）。
  - **开发期以"上次位置"为准**：即使 `SurfaceOptions` 显式给了 `X/Y`、`Centered` 或 `State`，也被文件里的记录覆盖。想回到代码里的初始位置，删掉该文件或用 `-no-restore`。
  - 监听 `OnMoved` / `OnResized` / `OnStateChanged`，在主线程更新内存记录（正常状态记边界，最大化/全屏只记状态、保留正常边界），300ms 节流后在后台 goroutine 原子写文件（同目录临时文件 + rename）；收到 `Terminating` 时再同步写一次。
- `devstate_release.go`（`!tango_dev`）为空实现，默认构建与 `tan build` 产物不含这部分代码。
- 限制：窗口按创建后立即恢复，个别平台可能先在默认位置闪一下；`KindSheet` / `KindModal` 等子窗口同样按标题记录；同一文件被多个同时运行的进程写时后写者胜。

## 移动端

`tan dev -platform android`（已连接的设备或模拟器，没有时启动第一个 AVD）和 `tan dev -platform ios-simulator`（已启动的模拟器，没有时启动最近用过的 iPhone）。iOS 真机暂不支持，会提示改用 `tan build -platform ios -debug` 后用 Xcode / `xcrun devicectl` 安装。代码在 `cmd/tan/dev_mobile*.go`。

现状：

- **Go 代码变化**：不做热替换，走"重建 + 重装 + 重启"。只构建设备自己的 ABI（debug 构建），Android `adb install -r -t` 后 `am start -S`，iOS `simctl install` 后 `terminate` + `launch`。编译失败时已安装的应用继续运行。实测：Android 模拟器 Gradle 增量后约 20 秒，iOS 模拟器约 25 秒。
- **前端**：与桌面一样起前端 dev server（或直接服务 dist 目录），应用通过 `asset.FromEnv` 代理到它（移动端 dev 构建同样带 `-tags tango_dev`，`FromEnv` 只接受回环地址）。Android 用 `adb reverse tcp:<端口> tcp:<端口>` 让设备上的 localhost 可达，URL 作为启动 Intent 的 `TANGO_DEV_URL` extra 传入，宿主只在 debuggable APK 里读取（`Mobile.devURL` → `internal/platform/android/devurl_android.go` 在 `main.main` 之前 `os.Setenv`），非 `tango_dev` 构建的 `FromEnv` 也不读它；iOS 模拟器共享宿主网络，用 `SIMCTL_CHILD_TANGO_DEV_URL` 传环境变量。iOS 模拟器上 Vite HMR 正常生效；Android 上页面是 `https://tango.localhost`、WebSocket 不经过资源拦截，HMR 连不上 dev server，因此改为前端源文件变化时重启应用（不重建）。直接服务 dist 目录时两个平台都是重启应用。
- **日志**：Android `adb logcat --pid=<pid>`（每次重启跟随新进程），iOS `simctl launch --console-pty`（应用 stdout/stderr）。Ctrl-C 时停止日志、结束应用、移除 adb reverse；模拟器保持运行。

自绘 UI（uihost）不做移动端热替换，原因：

1. 热替换依赖桌面上的"宿主进程 + 工作进程"：宿主持有窗口，工作进程每次编译后重新启动并通过 Unix socket / 共享内存送帧。iOS/Android 的应用是单进程，由系统拉起，Go 以 c-archive / c-shared 链进原生宿主，无法在设备上另起一个替换用的工作进程，也不能在进程内重新加载 Go 代码（Go 不支持卸载/重载已加载的库）。
2. 移动端 `tan build` 是生产构建（`mobileGoBuildArgs` 不加 `tango_dev`），devhost 代码本来就不链接进去；`tan dev -platform …` 虽然加 `tango_dev`，但不走热替换。
3. 想在宿主机上跑工作进程、把帧推到设备上显示，需要新的跨设备传输和输入转发，延迟和工作量都不划算；移动端自绘 UI 的快速迭代建议先在桌面 `tan dev` 下热加载调好，再上设备验证。

## 验收

1. `tan dev` 跑 ui-window 示例，修改按钮文字，保存后窗口不关闭，新文字在编译完成后 1 秒内出现。
2. 用了 `Persist` 的示例，输入框内容和当前页在热加载后保留。
3. 编译错误时旧界面继续可用，并显示错误提示；修好后自动恢复。
4. 输入法、剪贴板、光标在远程模式下表现与本地一致。
5. Retina 全屏窗口（2880×1800）拖动缩放时帧率可接受（≥ 30fps）。
6. `tan build` 产物不包含 devhost 代码（用 `go version -m` 或符号检查确认）。
