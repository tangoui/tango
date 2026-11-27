# 输入法框架三端补齐：修饰键、模式与菜单、候选窗、Fcitx5、打包

日期：2026-11-27 · 分支 `feat/ime-platform`（基于 main 1c0e6ee6）

## 目的

用户立项做一个对标搜狗、百度、豆包的中文输入法，要求 macOS、Windows、Linux 三端同时做。现有的 Tango `sys/ime`（macOS IMK、Windows TSF、Linux IBus）和 `KindOverlay`（仅 macOS）已经能跑通"按键 → 组字 → 上屏 → 候选窗"，但离一个能发布的产品还缺几块框架能力。本项目只补框架，输入法产品（拼音引擎、词库、设置程序）是下一个项目。

## 目标

1. **修饰键事件**：应用能收到单独按下和松开的修饰键（Shift、Ctrl、Alt、Meta、CapsLock，分左右），以此实现"单击 Shift 切换中英文"。`Key` 增加大写锁定状态和"自动重复"标志。三端都要。
2. **模式状态与输入法菜单**：应用能设置当前状态（如"中"/"英"、全角/半角），显示在系统的输入法指示处；能提供菜单项（偏好设置、关于、简繁切换等），用户点击后回调到应用。macOS 用 IMK 菜单和输入模式，Windows 用语言栏按钮，IBus 用 property，Fcitx5 用状态区 action。
3. **候选词列表 API**：`Session.SetCandidates` 把候选列表交给框架。Linux 上由 IBus/Fcitx5 自己的面板绘制（这是 Wayland 下唯一可行的方式：普通应用拿不到光标位置，也不能把窗口放到任意位置）。macOS、Windows 上系统不画，应用用 `KindOverlay` 自己画。通过 `ime.SystemDrawsCandidates()` 让应用知道该不该自己画。
4. **Windows 的 KindOverlay**：不激活、不抢焦点、置顶的浮层，行为约定与 macOS 一致。
5. **Fcitx5 支持**：Linux 上同时支持 IBus 和 Fcitx5，运行时自动选择当前桌面在用的那一个；两个都在时优先 Fcitx5。
6. **三端打包**：`tan build` / `tan package` 打出可以直接安装的输入法包。
   - macOS：已有，补上输入模式和菜单图标所需的 Info.plist 键。
   - Windows：Tango 提供一个通用的、预编译的 TSF 桥接 DLL（x64 和 ARM64），GUID、语言配置、连接名都从安装时写入的注册表或旁路配置读取，不再需要每个应用自己编译 C++。`tan package` 的 NSIS 安装包负责复制和注册（regsvr32）。
   - Linux：deb/rpm 安装 IBus 组件 XML（`/usr/share/ibus/component/`），以及 Fcitx5 的桥接插件（`.so` 和 addon/inputmethod 配置文件）。
7. 示例 `examples/inputmethod` 用上述能力：单击 Shift 切换中英文、状态显示"中"/"英"、菜单里有"关于"、三端都有候选列表。

## 非目标

- Linux 的 KindOverlay（X11/Wayland）不做。Linux 的候选窗走 IBus/Fcitx5 面板，不需要它。
- 不做拼音引擎、词库、设置程序、皮肤和云服务，这些属于下一个项目。
- 不支持 Windows 32 位（x86）应用里的输入。桥接 DLL 只出 x64 和 ARM64；32 位应用加载 x64 DLL 会失败，记入已知限制。
- 不做 macOS 的 IMKCandidates（系统候选窗）。macOS 用自绘浮层，体验可控。

## 反目标

- 不能破坏现有 `sys/ime` API：只做新增，experimental 层级允许改，但现有示例和测试必须继续通过。
- 修饰键事件不能让宿主应用的快捷键失灵：修饰键默认放行，只有应用明确返回"已处理"时才吞掉。
- Windows 桥接 DLL 被加载进所有接收输入的进程，它在任何情况下都不能让宿主崩溃或卡住：连不上 Go 进程、超时、Go 进程退出时，按键一律放行。
- Fcitx5 插件同样被加载进 fcitx5 进程，规则相同。

## 约束

- Go 侧只用 purego，不开 cgo。
- Windows 和 Linux 的原生桥接（TSF DLL、Fcitx5 插件）是 C++，源码放在 Tango 仓库（如 `internal/imebridge/windows`、`internal/imebridge/fcitx5`）。Windows DLL 的预编译产物随 Tango 发布，tan 打包时取用（参照现有的 Release 二进制流程）；Fcitx5 插件依赖发行版的 fcitx5 头文件和 ABI，在 Linux 上由 `tan package` 调用 cmake 现场编译，缺少 fcitx5 开发包时给出明确提示并跳过 Fcitx5。
- 桥接与 Go 进程之间沿用 Windows 现有的"命名管道 + 定长线协议"思路；Fcitx5 用 Unix socket 走同一套协议，协议版本号要能升级。
- 三端行为写进 `sys/ime` 的文档注释；某个平台做不到的能力返回 `ErrUnsupported`，或者在文档里写明被忽略。

## 思路比较

**候选窗**
1. **Linux 交给 IBus/Fcitx5 面板，macOS 和 Windows 用 KindOverlay 自绘（选用）**：Linux 下 Wayland 不允许应用自定位窗口，系统面板是唯一能在 X11 和 Wayland 下都工作的方式；而且面板的样式跟随用户的桌面主题，符合 Linux 用户的习惯。代价是 Linux 上候选窗的样式不能完全自定义。
2. 三端都自绘：外观统一，但 Linux Wayland 下做不到，X11 也要再实现一遍浮层。
3. 三端都用系统候选窗：macOS 的 IMKCandidates 和 Windows 的系统候选 UI 都难以定制，做不出搜狗那样的体验。

**Windows 桥接 DLL**
1. **Tango 提供通用的预编译 DLL，配置在安装时写入（选用）**：应用开发者不需要 C++ 工具链，`tan package` 一步出安装包。
2. 每个应用自己用 CMake 编译（现状）：门槛高，也违背 Tango"免 C 工具链"的方向。

**Fcitx5**
1. **C++ 插件 + Unix socket 桥接到 Go 进程（选用）**：Fcitx5 没有像 IBus 那样的进程外引擎协议，第三方引擎只能以 C++ 插件形式加载。
2. 让 Fcitx5 用户切到 IBus：不符合"都支持"的要求。

（以上选择均为作者根据"按你建议的来"做出的决定。）

## 分阶段

本项目按阶段交付，每个阶段单独写计划、单独合并：

1. **API 与 macOS**：修饰键事件、Key 新字段、模式状态、菜单、`SetCandidates` / `SystemDrawsCandidates` 的公开 API；macOS 实现（含 Info.plist 的输入模式键）；示例用上单击 Shift 和状态显示。
2. **Windows**：通用 TSF 桥接 DLL（修饰键、语言栏状态与菜单、光标位置）、Windows KindOverlay、`tan package` 生成 NSIS 安装包并注册。
3. **Linux**：IBus 补上修饰键、property（状态与菜单）、lookup table（候选）；新增 Fcitx5 插件与后端自动选择；`tan package` 的 deb/rpm 安装组件文件与插件。
4. **三端真机验收**。

## 验收

- 每个阶段：单元测试和协议测试覆盖新 API 和线协议；Windows/Linux 的交叉 vet；`apitxt -check`；改了 cmd/tan 就用 GOWORK=off 单独编译和测试。
- 真机（必须先征得用户同意；切换输入源会截走用户的真实键入）：
  - macOS：单击 Shift 在中/英之间切换，菜单栏状态随之变化；输入法菜单里点"关于"有回调；Shift+字母仍是正常的大写字母；Cmd+C 等快捷键不受影响。
  - Windows：安装包装好后，在记事本和一个 Electron 应用里打字；候选浮层跟随光标且不抢焦点；单击 Shift 切换、语言栏状态更新；卸载干净。
  - Linux：在 GNOME（IBus，Wayland）和 KDE（Fcitx5，Wayland 与 X11 各一次）下打字；候选列表由系统面板显示；单击 Shift 切换、状态更新。

### 验收环境（用户 2026-11-27 确认）

- **Windows**：用户在自己的 Windows 电脑上验证。实现方要交付可以直接安装的测试包和逐条验证步骤（存到 `outputs/`），并在 Windows 阶段结束时请用户验证。Mac 上能做的部分（交叉编译、线协议测试、在 Mac 上用 MinGW/zig 交叉编译 DLL 是否可行）尽量在 Mac 上先做完。
- **Linux**：在这台 Mac（Apple Silicon）上能做多少做多少。优先装一个本地 arm64 Linux 虚拟机，比如 UTM 或 Lima，跑 GNOME（IBus）和 KDE（Fcitx5）真实桌面；装不了图形桌面时，退而用无头会话（weston headless 或 Xvfb，加 dbus、ibus-daemon/fcitx5）跑自动化冒烟，真实桌面验收标为"待验收"。在 Mac 上安装虚拟化工具前，先告诉用户要装什么。

## 留给实现者

- 具体的 API 命名（如 `Handler.Modifier`、`ime.SetStatus`、`MenuItem` 的字段），以设计目标为准，保持与 Tango 现有风格一致。
- 线协议的消息布局与版本升级方式。
- 预编译 DLL 的构建与签名流程，要和现有的 Release 流程对齐（`docs/release.md`）。

## 实现者审查后补充的决定（作者的决定，用户可推翻）

1. **模式状态**：在 tan.json 里预先声明一组模式（如 `zh`、`en`），每个模式带 id、显示名、短文字（"中"/"英"）和可选图标；当前模式是**全局一份**（与搜狗一致），不按会话分开。API 形如 `ime.SetMode(app, "en")`，再加一个 `Handler.ModeChanged` 回调，用户在系统菜单或快捷键里切换模式时会收到。macOS 映射到 `ComponentInputModeDict` 和 `selectInputMode:`；Windows 语言栏图标从安装目录的图标文件读取，没有图标时由 DLL 把短文字画成图标；IBus/Fcitx5 直接显示短文字。理由：macOS 只有这条正规路径能让菜单栏图标随模式变化。
2. **Fcitx5 插件在打包机上编译**：只在 Linux 打包机上、与目标架构一致时编译 `.so`；在 macOS 上打包或跨架构打包时跳过 Fcitx5，并给出提示。deb/rpm 对 fcitx5 加版本依赖。第一批支持的发行版：Ubuntu 24.04、Debian 12、Fedora 41、Arch（fcitx5 5.1.x）。不在用户安装时编译。
3. **Windows 覆盖范围对标搜狗**：管理员权限运行的程序、开始菜单搜索、"设置"、UWP/商店应用里都要能打字。因此安装包用机器级安装（HKLM、Program Files，需要 UAC）；Go 宿主改为常驻（登录时自启），不再由 DLL 拉起；管道 ACL 加上 `ALL APPLICATION PACKAGES` 和低完整性标签。管理员进程连同一个用户宿主（按会话 id 区分），模式状态只有一份。这是 Windows 阶段工作量最大的部分。
4. **多个 Tango 输入法可以共存**：每个应用带一份自己的 DLL 副本，有自己的 CLSID；DLL 从模块旁边的配置文件读取 CLSID、语言配置和连接名。升级时用带版本号的 DLL 文件名并重新注册，旧文件用 `MoveFileEx` 安排在重启时删除，所以升级和卸载后不需要立即重启。
5. **取代现有的 `IME1` 协议和 CMake 示例**：通用 DLL 用新协议（长连接、双向推送、版本号可升级），不兼容旧的按应用编译的 DLL；`examples/inputmethod/windows` 的 CMake 路径和 `IME1` 测试相应改写或删除（sys/ime 仍是 experimental）。

## 用户回答（2026-11-27）

- **没有 Windows 代码签名证书**：桥接 DLL 和宿主默认不签名。测试时由用户在自己的电脑上关闭"智能应用控制"。`tan package` 保留签名挂钩：tan.json 里配置了 `windows.certificate`（沿用现有 `tan sign` 流程）时，对 DLL 和宿主 EXE 一并签名。文档写明未签名时在哪些机器上会被拒绝加载，以及正式发布前需要证书。
- **用户的 Windows 电脑是 Intel**：ARM64 版 DLL（包括 ARM64X 转发）只做到能编译，不做实机验证，不作为完成标准。验收以 x64 为准。

## 自审补充

- **tan.json 结构**：模式表是三端共用的，所以新增顶层 `inputMethod` 段（`connection`、`language`、`icon`、`modes`），并加 Windows 专用的 `clsid`/`profile` 以及 Linux 专用的引擎名。现有的 `macos.inputMethod` 保留为兼容别名：两者同时出现时报错，只有旧写法时照旧工作，并在文档里标为已弃用。
- **预编译 DLL 的构建**：现有 Release 流水线只有 ubuntu runner，所以 DLL 用 llvm-mingw（mingw-w64 自带 TSF/COM 头文件）在 Linux 上交叉编译出 x64 和 ARM64，不引入 Windows runner。本地开发时，Mac 上也能用同一套交叉编译器构建，便于在 Mac 上先跑通编译和线协议测试。
- **分阶段时各平台的行为**：第 1 阶段先定公开 API。Windows 和 Linux 的新能力在各自的阶段完成之前，返回 `ErrUnsupported` 或按文档说明被忽略，旧能力（按键、组字、上屏）保持可用。每个阶段合并后，main 都是可以发布的状态。
- **修饰键的判定留给应用**：框架只转发修饰键按下和松开的原始事件（带左右之分）。"单击 Shift"的判定（按下与松开之间没有其他键、时间不超过阈值）由应用或示例自己做，示例里给出参考实现，因为不同输入法对这件事的规则不一样。
