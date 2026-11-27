# 输入法框架补齐 · 第 2 阶段（Windows）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Windows 上用 Tango 写的输入法能一键安装、在管理员程序、开始菜单搜索、"设置"、UWP 里都能打字；支持单击 Shift、模式指示和菜单、浮层候选窗；输入法开发者不需要 C++ 工具链。

**Architecture:** Tango 提供通用 TSF 桥接 DLL（C++，源码在 `internal/imebridge/windows`），它被加载进每个接收输入的进程，从 DLL 旁的 INI 读取 CLSID、语言配置 GUID、连接名，通过新的长连接线协议（v2）连到每个登录会话里常驻的 Go 宿主（即输入法应用本身）。连接失败或超时时，按键一律放行。Go 侧 Windows 后端改为管道服务端，实现第 1 阶段定下的修饰键、模式、菜单接口。候选窗由宿主用 Windows 版 KindOverlay 绘制。`tan package` 用 NSIS 做机器级安装，DLL 从 Tango Release 下载并校验 sha256（参照 `cmd/tan/nsis_download.go`），本地开发可用环境变量指向自己编译的 DLL。

**Tech Stack:** Go（x/sys/windows、purego 风格）、C++17（mingw-w64 x64；ARM64 用 zig 或 llvm-mingw，只要求能编译）、TSF、NSIS。

**Spec:** `docs/superpowers/specs/2026-11-27-ime-platform-design.md`（全文必读，尤其"实现者审查后补充的决定"第 3–5 条、"用户回答""自审补充"），第 1 阶段计划 `docs/superpowers/plans/2026-11-27-ime-platform-phase1.md`（接口来源）。

## Global Constraints

- 桥接 DLL 绝不能让宿主进程崩溃或卡住：任何错误、连接断开、Go 宿主没响应（同步判键超时 150ms），都要放行按键；不得持有锁跨越 IPC 调用；不在 DllMain 里做 IPC。
- 机器级安装（HKLM、Program Files、UAC）。Go 宿主在每个登录会话里常驻（HKLM Run 键自启），不再由 DLL 拉起。DLL 在非 AppContainer 进程里发现宿主没运行时，可以尝试按 INI 里的路径拉起一次；AppContainer 里只连接，不拉起。
- 管道名包含连接名和会话 id；管道 ACL 允许当前用户 SID、`ALL APPLICATION PACKAGES`，并带低完整性标签，让 UWP 和"设置"也能连上；管理员进程连同一个宿主。
- 可以共存：每个输入法带自己的 DLL 副本和自己的 CLSID。升级时用带版本号的 DLL 文件名并重新注册，旧文件用 MoveFileEx 安排重启时删除。卸载后不需要立即重启。
- 新协议 v2 取代 IME1；`examples/inputmethod/windows`（CMake + tip.cpp）删除或改为指向通用 DLL。
- 没有签名证书：默认不签名。`windows.certificate` 配置了就对 DLL 和 EXE 签名（沿用 `tan sign`）。文档写明未签名在"智能应用控制"下会被拒绝。
- x64 是验收标准；ARM64（含 ARM64X 转发）只要求能编译。
- Go 侧不开 cgo。提交作者 `tango <903300326@qq.com>`，不加 Co-Authored-By。每次提交前：gofmt -l、GOOS=windows/linux 交叉 vet、`apitxt -check`、cmd/tan GOWORK=off 编译和测试。不推送、不合并。
- 用户只有一台 Intel 的 Windows 电脑用于验证。凡是能在 Mac 上做的（交叉编译、协议测试、Go 的 `GOOS=windows go test -c` 编译检查、C++ 协议编解码用本机 clang 跑单测）都先在 Mac 上做完；最后交付一个可安装的测试包和逐条验证清单，再请用户去 Windows 上验证。

## Review Focus

1. Go 宿主没运行或卡住时，在记事本里打字照常是英文直通，系统不卡顿。→ Task 2 的超时与放行测试（C++ 单测），以及 Task 7 的清单项。
2. 以管理员身份运行的程序和 UWP（开始菜单搜索框）里能打字。→ Task 3 的管道 ACL 测试，以及 Task 7 的清单项。
3. 点击候选浮层选词：上屏到原输入框，原程序不失焦；浮层不出现在任务栏和 Alt+Tab 里。→ Task 4、Task 6，以及 Task 7 的清单项。
4. 升级安装（DLL 正被占用）和卸载都不需要立即重启，卸载后注册表和文件不残留（重启后删除的除外）。→ Task 5 的 NSIS 脚本测试，以及 Task 7 的清单项。
5. 同时打开多个程序、在它们之间快速切换焦点时，组字状态不串到别的会话。→ Task 3 的会话复用测试。

---

### Task 1: 线协议 v2（Go 与 C++ 两端）

**Files:** Create `internal/imebridge/proto/`（Go：帧格式、消息类型、编解码，以及 golden 向量 `testdata/*.bin`）、`internal/imebridge/windows/proto.h`（C++ 编解码，只依赖标准库）、`internal/imebridge/windows/proto_test.cpp`（读取同一批 golden 向量）。

**Interfaces（Produces）：** 帧格式为 `u32 长度 | u16 类型 | u16 版本(=2) | 负载`，字符串用 UTF-16LE 加长度前缀。消息分两类：DLL→宿主的 Hello{pid, exe, appcontainer bool, integrity}、SessionOpen/Close{sid}、Key{sid, vk, scan, flags(down/up/repeat/ext), mods, text, caret rect}、TestKey（同步，要求在 150ms 内收到 KeyResult{sid, eaten bool}）、Focus{sid, on}、MenuOpen{sid}、MenuPick{id}、ModePick{id}；宿主→DLL 的 KeyResult、Compose{sid, text, cursor}、Commit{sid, text}、Mode{id, short, icon path}、Menu{items}。每条消息带 seq，KeyResult 按 seq 对应请求。具体字段以实现为准，但必须两端共用 golden 向量。

- [ ] **Step 1:** 写测试。Go 侧 `TestProtoGolden`：对每种消息，编码结果与 golden 文件逐字节相同，解码后与原值相等。截断帧、超长帧（大于 64 KiB）、未知类型、错误版本都返回错误，不能 panic。C++ 侧 `proto_test.cpp`：读取同一批 golden 文件，解码后再编码，结果逐字节相同；对坏帧返回 false。用 `clang++ -std=c++17` 在 Mac 上编译运行。Go 测试里加一个用例：本机有 clang++ 时自动编译并运行 C++ 测试，没有就 Skip。
- [ ] **Step 2:** 运行测试，应为 FAIL。
- [ ] **Step 3:** 实现。
- [ ] **Step 4:** 测试通过。
- [ ] **Step 5:** 提交 `imebridge: wire protocol v2 with shared golden vectors`。

### Task 2: 通用 TSF 桥接 DLL

**Files:** Create `internal/imebridge/windows/`：`tip.cpp`（TextInputProcessorEx、ThreadMgrEventSink、KeyEventSink、CompositionSink）、`langbar.cpp`（GUID_LBI_INPUTMODE 按钮，带菜单和图标；没有图标时把短文字画成图标）、`pipe.cpp`（长连接、读线程、重连、超时、失败放行）、`config.cpp`（读取 DLL 旁的 `<dll 名>.ini`）、`register.cpp`（DllRegisterServer/Unregister：CLSID、TSF profile、分类 GUID_TFCAT_TIPCAP_IMMERSIVESUPPORT / SYSTRAYSUPPORT / UIELEMENTENABLED 等）、`build.sh`（mingw x64，zig 或 llvm-mingw arm64，静态链接 libstdc++，不依赖 MSVC 运行库）、`README.md`。

做法：宿主异步推送（例如鼠标点选导致的 Commit）时，读线程把消息投递到 TSF 线程的隐藏窗口，由那个窗口 RequestEditSession(TF_ES_ASYNCDONTCARE) 执行；光标矩形在每次按键时用 ITfContextView::GetTextExt 取到并随 Key 上报。修饰键：OnTestKeyDown/Up 都要转发，以支持单击 Shift。AppContainer 检测用 GetTokenInformation(TokenIsAppContainer)。

- [ ] **Step 1:** 写测试。在 `proto_test.cpp` 的同一套机制下加 `pipe_logic_test.cpp`，把"连接状态机和超时放行"抽成与 Win32 无关的类，用假的传输层测试：宿主不回复时 TestKey 在 150ms 后返回"不吞"；断线后再来的按键直接放行，并按 1s、2s、4s（上限 30s）退避重连；收到不匹配 seq 的 KeyResult 时丢弃。INI 解析测试：缺少 CLSID 时 DllRegisterServer 返回 E_FAIL。在 Mac 上用 clang++ 编译运行。另加 Go 测试 `TestBridgeDLLBuilds`：本机有 x86_64-w64-mingw32-g++ 时运行 build.sh，用 objdump 检查导出了 DllGetClassObject、DllCanUnloadNow、DllRegisterServer、DllUnregisterServer，并且没有依赖 libstdc++-6.dll 或 libgcc_s；没有交叉编译器时 Skip。
- [ ] **Step 2:** 运行测试，应为 FAIL。
- [ ] **Step 3:** 实现。
- [ ] **Step 4:** 测试通过；在 Mac 上用 mingw 产出 x64 DLL，ARM64 至少能编译通过（做不到就在 README 写明原因）。
- [ ] **Step 5:** 提交 `imebridge: generic TSF bridge DLL`。

### Task 3: Go Windows 后端改为常驻管道服务端

**Files:** Modify `internal/platform/windows/inputmethod_windows.go`，把 `inputmethod_wire.go` 换成基于 `internal/imebridge/proto` 的实现；新建 `inputmethod_pipe_windows.go`（管道 ACL、每个会话一个监听、多客户端）和 `inputmethod_session.go`（与平台无关的会话复用和路由，可以在 Mac 上测试）；删除 IME1 的代码和测试。

**Interfaces:** Consumes 第 1 阶段的 `platform.InputMethods`、`InputMethodModifiers`、`InputMethodModes`、`InputMethodMenus`。模式表来自宿主可执行文件旁的配置，由 tan 写入：`<exe>.ime.json`，包含 modes、connection、clsid。

- [ ] **Step 1:** 写测试（与平台无关的部分在 Mac 上跑）。`TestSessionMux`：两个 DLL 连接各开一个 sid，按键路由到正确的 Session，SessionClose 后触发 IMEEnd。`TestModifierUpDown`：Key 消息的 up/down 标志转成 IMEModifier。`TestModeBroadcast`：SetMode 向所有连接推送 Mode 消息；DLL 发来的 ModePick 触发 IMEModeChanged，并且自己发起的切换不会回声。`TestMenuRoundTrip`：MenuOpen 返回 Handler.Menu 的条目，MenuPick 到达 MenuClicked。`TestPipeSecurityDescriptor`：生成的 SDDL 包含当前用户、`AC`（ALL APPLICATION PACKAGES）和低完整性标签（`S:(ML;;NW;;;LW)`）；SDDL 生成函数是纯函数，可以在 Mac 上测。Windows 专属代码用 `GOOS=windows go test -c` 确认能编译。
- [ ] **Step 2:** 运行测试，应为 FAIL。
- [ ] **Step 3:** 实现。
- [ ] **Step 4:** 测试通过；交叉 vet 通过。
- [ ] **Step 5:** 提交 `windows: persistent IME host over protocol v2`。

### Task 4: Windows 的 KindOverlay

**Files:** Modify `internal/platform/windows/surface_windows.go`（prepareExtras：WS_EX_NOACTIVATE | WS_EX_TOPMOST | WS_EX_TOOLWINDOW，WS_POPUP；Show 用 SW_SHOWNOACTIVATE 加 SetWindowPos(HWND_TOPMOST, SWP_NOACTIVATE)；WM_MOUSEACTIVATE 返回 MA_NOACTIVATE；Focus 和 IsFocused 按 KindOverlay 的约定处理；SetAppearance 套用浮层规则）；backend 实现 `SupportsOverlay()`；新增测试。

- [ ] **Step 1:** 写测试。`TestOverlayExStyle`（纯函数）：结果包含 0x08000000 | 0x8 | 0x80，不包含 WS_EX_APPWINDOW(0x40000)。`TestOverlayShowCmd`：SW_SHOWNOACTIVATE(4)。`TestOverlayMouseActivate`：返回 MA_NOACTIVATE(3)。核心层 `TestOverlayUnsupported` 中 Windows 不再返回 ErrUnsupported（用构建标签区分）。
- [ ] **Step 2:** 运行测试（纯函数部分在 Mac 上跑，Windows 文件用 `GOOS=windows go test -c`），应为 FAIL。
- [ ] **Step 3:** 实现，并更新 KindOverlay 文档和 site/guide 中"支持的平台"。
- [ ] **Step 4:** 测试通过。
- [ ] **Step 5:** 提交 `windows: KindOverlay surfaces`。

### Task 5: tan 打包 Windows 输入法

**Files:** Modify `cmd/tan/config.go`（`inputMethod.windows`：clsid、profile、langid，默认 0x0804）、`cmd/tan/package_nsis.go`（有 inputMethod 时强制 Scope=machine；安装 `TangoIMEBridge-<ver>-x64.dll`、arm64 版、INI、`<exe>.ime.json`；HKLM Run 键；regsvr32；升级和卸载流程）；新建 `cmd/tan/imebridge_download.go`（版本号取应用 go.mod 里依赖的 github.com/tangoui/tango 版本，而不是 tan 自身的版本；replace 指向本地目录时要求设置 TANGO_IME_BRIDGE_DIR；从 `https://github.com/tangoui/tango/releases/download/v<ver>/` 下载 DLL，校验内置的 sha256 表；`TANGO_IME_BRIDGE_DIR` 环境变量可以改用本地目录）；`.github/workflows/release.yml` 增加在 ubuntu 上交叉编译 DLL 并上传的步骤；更新 docs/packaging.md。

- [ ] **Step 1:** 写测试。`TestIMEConfigWindows`：缺 clsid 时自动从 id 生成稳定的 GUID（用 UUIDv5，命名空间固定），profile 同理；显式填写的 GUID 必须格式合法。`TestNSISIMEScript`：生成的 nsi 里有 RequestExecutionLevel admin、HKLM Run 键、`regsvr32 /s` 指向带版本号的 DLL、卸载段里有 `regsvr32 /u`，删除被占用文件时走 Delete /REBOOTOK。`TestIMEBridgeDownload`：用 httptest 提供假的 DLL，sha256 不符时报错；设置了 TANGO_IME_BRIDGE_DIR 时不下载。没有 inputMethod 的应用，生成的 nsi 与现状逐字节相同（golden 文件）。
- [ ] **Step 2:** 运行 `cd cmd/tan && GOWORK=off go test -run 'IME|NSIS' .`，应为 FAIL。
- [ ] **Step 3:** 实现。sha256 表先放占位值，在 README 和 release 文档里写明"发版时由 CI 回填"；本地验证时用 TANGO_IME_BRIDGE_DIR。
- [ ] **Step 4:** 测试通过，cmd/tan 全量测试通过。
- [ ] **Step 5:** 提交 `tan: package Windows input methods with the TSF bridge`。

### Task 6: 示例的 Windows 适配

**Files:** Modify `examples/inputmethod/`（tan.json 增加 windows 段；candidates.go 在 Windows 上同样用 KindOverlay；删除 `examples/inputmethod/windows/`；更新 README 中 Windows 部分）。

- [ ] **Step 1:** 写测试。`TestCandidateBoundsWindows`：用 Windows 坐标（GetTextExt 返回的是屏幕物理像素），经 DPI 换算后得到逻辑像素位置。换算函数放在示例里，或者 sys/ime 返回逻辑像素，二选一，并在汇报里说明选了哪个。
- [ ] **Step 2–4:** 失败 → 实现 → 通过；`GOOS=windows go build ./examples/inputmethod` 通过。
- [ ] **Step 5:** 提交 `examples: Windows input method via the generic bridge`。

### Task 7: 交付给用户在 Windows 上验证

- [ ] **Step 1:** 在 Mac 上用 `TANGO_IME_BRIDGE_DIR` 指向 Task 2 编出的 DLL，`tan package -platform windows/amd64` 打出 `Tango Pinyin-setup.exe`，放到 `/Users/admin/min/outputs/tango-ime-platform/phase2/`，附上 sha256。
- [ ] **Step 2:** 写 `phase2/windows-checklist.md`（中文，逐条可勾选）：关闭智能应用控制的方法；安装（UAC）；在"设置 → 时间和语言 → 语言 → 中文 → 键盘"添加 Tango Pinyin；Win+空格切换；在记事本、Edge 地址栏、开始菜单搜索、"设置"搜索框、以管理员身份运行的记事本里分别打 `nihao` 加空格上屏"你好"，并确认候选浮层位置；单击 Shift 切换中英文，任务栏指示变"英"/"中"；在任务栏指示上右键或点击打开菜单，点"关于"；结束 Tango Pinyin 进程后在记事本打字，英文正常、不卡；升级安装（再装一次新版本）不要求重启；卸载后语言列表里不再有 Tango；遇到问题时要收集的东西（日志路径、截图）。
- [ ] **Step 2b:** 先对整个分支做一次独立审查，把发现的问题修完，再进行 Step 3。
- [ ] **Step 3:** 停下来，汇报每个 Task 的提交、测试和审查结果，并请用户按清单在 Windows 上验证。
