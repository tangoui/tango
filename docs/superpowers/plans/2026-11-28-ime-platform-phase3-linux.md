# 输入法框架补齐 · 第 3 阶段（Linux：IBus + Fcitx5）实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Linux 上同一个 Tango 输入法既能作为 IBus 引擎，也能作为 Fcitx5 输入法使用。候选列表、中/英模式和菜单都由系统面板显示（X11 和 Wayland 都适用），支持单击 Shift，`tan package` 的 deb/rpm 能直接安装。

**Architecture:**
- **IBus**：沿用现有的 purego + libibus 实现，补上松开键与修饰键、lookup table（候选）、IBusProperty（模式与菜单）。注册方式从"运行时临时注册"改为安装组件文件 `/usr/share/ibus/component/<name>.xml`：由 ibus-daemon 按 `<exec>` 用 `--ibus` 拉起宿主，同时保留运行时注册，供开发时使用。
- **Fcitx5**：新增 C++ 插件 `internal/imebridge/fcitx5`（InputMethodEngineV2），通过 Unix socket，用第 2 阶段的协议 v2（复用 `internal/imebridge/windows/proto.h`，移到共享目录）连接 Go 宿主。宿主不在时，插件按配置拉起一次；超时或断线时放行按键。
- Go 侧 Linux 后端同时服务两条通道：IBus（宿主由 ibus 拉起，或处在 IBus 会话中）和 Fcitx5 socket（`$XDG_RUNTIME_DIR/tango-ime/<connection>.sock`，权限 0600）。`SystemDrawsCandidates()` 在 Linux 返回 true。

**Tech Stack:** Go（purego，不开 cgo）、libibus-1.0、C++17 + Fcitx5 开发库（Fcitx5Core、Fcitx5Utils）、CMake、deb/rpm。

**Spec:** `docs/superpowers/specs/2026-11-27-ime-platform-design.md`（全文，尤其"实现者审查后补充的决定"第 2 条：Fcitx5 插件在打包机编译、首批发行版），第 1 阶段接口，第 2 阶段协议 `internal/imebridge/proto`。

## Global Constraints

- Fcitx5 插件被加载进 fcitx5 进程：任何错误、断线、宿主 150ms 内不回复，都要放行按键；不能让 fcitx5 崩溃或卡住；不在插件初始化时阻塞等待宿主。
- Go 侧不开 cgo；C++ 只放在 `internal/imebridge/`。
- Fcitx5 插件只在与目标架构相同的 Linux 打包机上编译。在 macOS 上打 Linux 包，或交叉架构打包时，跳过 Fcitx5 并给出明确提示；deb/rpm 对 fcitx5 加版本依赖：按打包机上 fcitx5 的主次版本生成，例如打包机是 5.1.x 时依赖为 ≥5.1、<5.2（Debian 12 是 5.0.x，Ubuntu 24.04、Fedora 41、Arch 是 5.1.x，所以要按发行版分别出包）。首批支持：Ubuntu 24.04、Debian 12、Fedora 41、Arch。
- 不做 Linux 的 KindOverlay；候选窗只用系统面板。
- 提交作者 `tango <903300326@qq.com>`，不加 Co-Authored-By。每次提交前：gofmt -l；GOOS=windows/linux 交叉 vet；`apitxt -check`；cmd/tan GOWORK=off 编译和测试。不推送、不合并。
- 本机是 Mac（Apple Silicon），没有 Linux，磁盘剩余空间只有约 12 GB。纯逻辑测试（D-Bus/IBus 消息构造、协议、socket 会话复用、打包脚本生成）都在 Mac 上跑。需要真实 Linux 环境的部分（编译 Fcitx5 插件、IBus/Fcitx5 无头冒烟），**到 Task 6 时停下来，由用户选择环境**，不要自己安装虚拟机，也不要推送分支。

## Review Focus

1. 宿主进程不在，或卡住时，在 Fcitx5 下打字直接放行，fcitx5 不卡。→ Task 3 的放行测试。
2. 同一个宿主同时被 IBus 和 Fcitx5 使用（用户中途切换框架）时，会话 id 不冲突，模式状态全局一致。→ Task 1 和 Task 4 的会话测试。
3. Wayland 下没有光标坐标：候选由系统面板显示，`Session.Caret()` 返回零值，示例不再尝试自绘。→ Task 5。
4. socket 不能被同机的其他用户连接（权限 0600，放在用户的 XDG_RUNTIME_DIR 下），连接时用 SO_PEERCRED 校验 uid。→ Task 4 的测试。
5. 卸载 deb/rpm 后，组件文件、插件 .so 和配置都被清理；fcitx5/ibus 重启后不再出现该输入法。→ Task 6 的打包测试，以及真机或冒烟时的清单项。

---

### Task 1: 协议共享与 Linux 会话复用

**Files:** 把 `internal/imebridge/windows/proto.h` 移到 `internal/imebridge/common/proto.h`，Windows 的构建和测试相应改路径；把第 2 阶段 Windows 后端里与平台无关的会话复用（`inputmethod_session.go`）提到 `internal/platform/imesession`（或同等位置），Windows 和 Linux 共用。

- [ ] **Step 1:** 写测试。现有 Windows 的会话测试迁移后保持通过；新增 `TestSessionMuxTwoTransports`：两个来源（模拟 IBus 和 Fcitx5）各开会话，id 不冲突，SetMode 向两边广播。
- [ ] **Step 2–4:** 失败 → 实现 → 通过；Windows DLL 的 build.sh 和 C++ 测试仍然通过。
- [ ] **Step 5:** 提交 `ime: share protocol and session mux across Windows and Linux`。

### Task 2: IBus 补全

**Files:** Modify `internal/platform/linux/inputmethod_linux.go` 和 `inputmethod_keys.go`；新建 `inputmethod_ibus_props.go`（IBusProperty 与 lookup table 的构造）；测试。

做法：引擎声明会接收松开键（process-key-event 时检查 IBUS_RELEASE_MASK = 1<<30），转成 IMEModifier 和 Key.Repeat；修饰键的 keyval（Shift_L 0xffe1、Shift_R 0xffe2、Control_L/R、Alt_L/R、Super_L/R、Caps_Lock 0xffe5）映射成 KeyboardEvent.code 风格的名字。`IMESetCandidates` 调 `ibus_engine_update_lookup_table`（页大小 9，Comment 显示在 label 旁）；模式用一个 PROP_TYPE_NORMAL 属性显示短文字；菜单用 PROP_TYPE_MENU，在 property-activate 时回调 IMEMenuClicked 或 IMEModeChanged。

- [ ] **Step 1:** 写纯函数测试（Mac 上跑）。`TestIBusModifierKeyval`：0xffe1 加按下得到 {ShiftLeft, Down:true}；带 RELEASE_MASK 得到 Down:false；0xffe5 得到 CapsLock。`TestIBusCapsState`：state 含 LOCK_MASK(1<<1) 时 CapsLock 为 true。`TestIBusLookupTablePaging`：12 个候选、selected=10 时页为第 2 页，光标位置为 1。`TestIBusPropsFromModes`：zh/en 两个模式加 3 个菜单项，生成的属性树结构正确（key、类型、label）。libibus 调用部分用 `GOOS=linux go vet` 确认能编译。
- [ ] **Step 2–4:** 失败 → 实现 → 通过。
- [ ] **Step 5:** 提交 `linux: IBus modifier keys, lookup tables, modes and menus`。

### Task 3: Fcitx5 桥接插件

**Files:** Create `internal/imebridge/fcitx5/`：`CMakeLists.txt`、`tangobridge.cpp/.h`（AddonFactory + InputMethodEngineV2：keyEvent、reset、activate/deactivate，候选用 CommonCandidateList，模式和菜单用 SimpleAction 加 StatusArea）、`pipe_logic.h/.cpp`（与平台无关的连接状态机，复用 Windows 的思路，并用 Mac 上的 clang++ 单测）、`addon.conf.in`、`inputmethod.conf.in`（文件名和名称由 tan 填入）、`README.md`。

做法：插件从 `/usr/share/<app>/tango-ime.conf`（tan 安装）读取 socket 路径、宿主可执行文件路径和连接名；socket 不存在时 posix_spawn 宿主一次，然后异步重连；按键用同步请求，150ms 超时就放行；宿主推送的 Commit 和 Compose 在 fcitx 事件循环里执行（用 EventDispatcher 从读线程投递）。

- [ ] **Step 1:** 写测试。`pipe_logic_test.cpp`（Mac 上的 clang++）：超时放行、断线退避（1s、2s 直到 30s）、seq 不匹配就丢弃、宿主推送排队到事件循环的顺序与收到的顺序一致。`TestFcitx5ConfTemplates`（Go）：tan 填好的 addon.conf 和 inputmethod.conf 字段正确（Name、Library、Category=InputMethod、LangCode=zh_CN、Icon、Label）。插件的 C++ 主体在 Mac 上无法编译，留到 Task 6 在 Linux 上编译。
- [ ] **Step 2–4:** 失败 → 实现 → 通过（Mac 上能跑的部分）。
- [ ] **Step 5:** 提交 `imebridge: Fcitx5 bridge addon`。

### Task 4: Go 宿主的 Fcitx5 通道与框架选择

**Files:** Modify `internal/platform/linux/inputmethod_linux.go`；新建 `inputmethod_fcitx_linux.go`（Unix socket 监听、SO_PEERCRED 校验、协议 v2）；`sys/ime` 的 `SystemDrawsCandidates()` 在 Linux 返回 true，并更新文档。

做法：`ServeInputMethod` 总是启动 Fcitx5 socket；IBus 可用时（存在 IBUS_ADDRESS，或进程是用 `--ibus` 启动的）同时注册 IBus 引擎。两者都不可用时返回错误，说明原因。

- [ ] **Step 1:** 写测试。`TestFcitxSocketPath`：XDG_RUNTIME_DIR 下的路径，权限 0600，父目录 0700；没有 XDG_RUNTIME_DIR 时报错，不退回 /tmp。`TestFcitxPeerCred`（纯函数）：uid 不同就拒绝。`TestSystemDrawsCandidatesLinux`（构建标签）。会话层沿用 Task 1 的测试。
- [ ] **Step 2–4:** 失败 → 实现 → 通过（Linux 文件用 `GOOS=linux go vet` 和 `GOOS=linux go test -c`）。
- [ ] **Step 5:** 提交 `linux: serve Fcitx5 alongside IBus`。

### Task 5: 示例与 tan 打包

**Files:** `examples/inputmethod`（Linux 上 `SystemDrawsCandidates()` 为 true 时调用 `SetCandidates`，不建浮层；README 的 Linux 部分改为 deb/rpm 安装和 fcitx5/ibus 的使用说明）；`cmd/tan/deb.go`、`cmd/tan/rpm.go`、`cmd/tan/config.go`（`inputMethod.linux`：ibus 引擎名、fcitx5 输入法名，默认由 id 生成）；新建 `cmd/tan/ime_linux_package.go`（生成 IBus 组件 XML、tango-ime.conf，在 Linux 同架构机器上调用 cmake 编译 Fcitx5 插件，否则跳过并提示）；更新 docs/packaging.md。

- [ ] **Step 1:** 写测试。`TestIBusComponentXML`：`<exec>` 指向 `/usr/bin/<exe> --ibus`，engine 含 name、longname、language=zh_CN、layout=default、icon，能被 xmllint 解析（有 xmllint 时）。`TestDebIMEFiles`：deb 里包含组件 XML 和 tango-ime.conf；在非 Linux 机器上打包时没有 .so，并输出包含 "Fcitx5" 的提示；Depends 含 `ibus | fcitx5`。`TestRpmIMEFiles` 同理。`TestDebPostinst`：postinst 和 postrm 会运行 `ibus write-cache`（如果存在），并提示重启 fcitx5 或 ibus。没有 inputMethod 的应用，deb/rpm 内容与现状一致。
- [ ] **Step 2–4:** 失败 → 实现 → 通过，cmd/tan 全量测试通过。
- [ ] **Step 5:** 提交 `tan: package Linux input methods for IBus and Fcitx5`。

### Task 6: 在真实 Linux 上编译和冒烟（必须先问用户选环境）

**到这里先停下，向主会话汇报，并说明两种可选环境，等用户决定：**
- **A. GitHub Actions**：新增工作流 `.github/workflows/ime-linux.yml`，在 ubuntu-24.04（以及 fedora 容器）里 apt/dnf 安装 ibus、fcitx5 及开发包，编译插件，在 Xvfb + dbus 下跑 IBus 和 Fcitx5 无头冒烟（用一个 GTK 测试窗口加 xdotool 发键，读回文本），并上传产物。需要用户同意把 feat/ime-platform 推送到 GitHub 的一个分支上运行。本机不占磁盘。
- **B. 本机 Lima 虚拟机**：`brew install lima`，建一个 Ubuntu 24.04 arm64 虚拟机，大约需要 6–8 GB 磁盘，本机目前只剩约 12 GB。需要用户同意安装。

用户选定后执行：编译 Fcitx5 插件（x86_64 和/或 arm64）；IBus 和 Fcitx5 各跑一遍冒烟：`nihao` 加空格得到"你好"，候选出现在系统面板（用日志或 D-Bus 核对 lookup table / candidate list），单击 Shift 切换模式并核对状态；宿主被 kill 后打字仍然放行；deb 安装和卸载干净。证据存 `/Users/admin/min/outputs/tango-ime-platform/phase3/`。

- [ ] **Step 1:** 停下来询问（经主会话转达给用户）。
- [ ] **Step 2:** 按用户的选择实施冒烟；发现问题时按 TDD 修复并提交。
- [ ] **Step 3:** 先对整个第 3 阶段做独立审查，把问题修完，然后汇报每个 Task 的提交、测试、冒烟结果，以及仍需真实桌面（GNOME/KDE 图形界面）验收的项目。
