# Windows 跨框架性能对比环境（uibench on Windows）设计

状态：已与用户确认测量条件（2026-11-20）

## 1. 意图

用户目标：Tango 在 **Windows** 上的性能要强过 MyGo、NSGo、Viva，具体是**内存低、CPU 省、速度快**。

现状：仓库里所有跨框架对比（`tools/uibench`、`tools/editor-gui-bench`、各 `docs/perf` 报告）都只在 macOS（Apple M4）上测过，Windows 上没有任何一条数据；`docs/superpowers/specs/2026-11-16-ui-component-performance-design.md:28` 明确排除了 Windows 实机对比。不先在 Windows 上量清楚差距，就无法有针对性地优化。

本阶段：**搭好 Windows 上的四家对比环境，并跑出第一份基线**。之后的优化另立项目，以本阶段的基线为起点。

## 2. 目标 / 非目标 / 反目标

**目标**

1. `tools/uibench` 能在 Windows（本机：windows/amd64，Intel Iris Xe）上构建并测量 Tango、MyGo、NSGo、Viva 四家。
2. 四家都有同一批场景的实现（见 §4）。Viva 是新增的第四个场景程序。
3. 测量指标覆盖用户关心的三项（见 §5）：内存、CPU、速度。
4. 跑出第一份基线，写成 `docs/perf/2026-11-2x-windows-baseline.md`：四家逐场景对比表，标明 Tango 输在哪些项上、差多少。

**非目标**

- 本阶段不改 Tango 的产品代码（`ui/`、`internal/`、`uihost/` 等）。发现的问题只记录，留给优化阶段。
- 不移植 `tools/editor-gui-bench`（它基于 Swift 和 ScreenCaptureKit，完全绑定 macOS）。编辑器打字延迟的 Windows 对比以后另做。
- 不做 Linux、移动端对比。
- 不一次性做全部 42 个场景，先做 §4 的子集。
- 不调整本机的显示缩放、电源计划等系统设置；需要的条件由 runner 检查并在不满足时拒测。

**反目标**（技术上能跑但算失败的情况）

- 对比不公平：四家窗口尺寸、场景内容、每帧工作量不一致；有的是调试构建，有的是发布构建；有的被遮挡不绘制。
- 在机器忙时测量：本机常有另一个会话同时跑 `go test`（`ui/text` 的测试一跑就占 3GB 内存），这时的数据无效，必须被拦下或标记，不能混进基线。
- macOS 上现有的 uibench 流程被改坏：Windows 支持必须用平台文件或构建标签加，darwin 路径的行为和测试保持不变。

## 3. 约束

- 每个目录是独立 Go 模块，用 `GOWORK=off` 构建；`tools/uibench` 不属于 Tango 产品模块，也不进 API 快照。
- 沿用现有的构建清单机制：`runner -build` 写 `bin/manifest.json`，runner 校验清单和自身 SHA-256 才肯测量。Windows 构建要有自己固定的 `GOOS=windows GOARCH=amd64`，工具链沿用 `go1.27.3`，`CGO_ENABLED=0`（MyGo、NSGo、Viva 在 Windows 上都不需要 cgo）。
- 对手版本必须固定：
  - MyGo：`v0.9.0`（本机模块缓存已有，`E:\go\pkg\mod\github.com\egoist\mygo@v0.9.0`）。
  - NSGo：沿用 `setup.sh` 的方式，把一份源码放到 `third_party/nsgo`，并记录提交号；默认取 macOS 测量时用的提交（`7eba948` 或 `2449cc5`，以 `third_party/nsgo.version` 历史记录为准），在 Windows 上编不过时改用最新发布版，并在报告里写明。
  - Viva：`v0.6.6`（当前最新发布版），同样放到 `third_party/viva` 并记录版本。
- 本机条件：Windows 11、Intel Iris Xe（驱动 31.0.101.4146）、笔记本，CPU 计时抖动大（同样的工作量耗时能差一倍以上）。所以每个场景至少跑 5 轮，判定用中位数，沿用 runner 现有的轮次和判定逻辑。

## 4. 场景

首批 8 个场景，四家都要实现：

| 场景 | 已有实现 | 用途 |
|---|---|---|
| counter | Tango、MyGo、NSGo | 空闲内存和空闲 CPU（期望 fps 为 0） |
| list | Tango、MyGo、NSGo | 长列表持续滚动 |
| grid | Tango、MyGo、NSGo | 大量单元格重绘 |
| buttons | Tango、MyGo、NSGo | 简单组件的每帧固定开销（macOS 上 Tango 输） |
| dialog | Tango、MyGo、NSGo | 浮层和动画（macOS 上 Tango 输得多） |
| table | Tango、MyGo、NSGo | 数据表格 |
| tree | Tango、MyGo、NSGo | macOS 上 Tango 唯一明确赢的场景，作对照 |
| textarea | Tango、MyGo、NSGo | 文本输入和排版 |

Viva 的这 8 个场景要新写（`tools/uibench/viva/`），内容和每帧工作量按 `scenes.md` 的定义，组件用 Viva 自带组件里最接近的那个。如果 Viva 没有对应组件（比如没有现成的 tree），用它的基础绘制拼出等价界面，并在 `scenes.md` 里写明差异。

## 5. 指标

沿用 uibench 的协议和统计方法，Windows 上的取值方式：

| 指标 | macOS（现有） | Windows（新增） |
|---|---|---|
| 进程 CPU | `proc_pid_rusage` | `GetProcessTimes`（用户态 + 内核态） |
| 合成器 CPU | WindowServer 的 `ps -o time=` | `dwm.exe` 的 `GetProcessTimes` |
| 内存 | `phys_footprint` 及峰值 | `PROCESS_MEMORY_COUNTERS_EX` 的 `PrivateUsage`（主指标）、`WorkingSetSize`、`PeakWorkingSetSize` |
| 启动 | exec 到 `bench ready` | 同上，不变 |
| fps、帧间隔 p50/p99 | 场景程序自报 | 不变 |

"速度快"在本阶段指启动时间、帧率和帧间隔 p99。按键延迟需要截屏测量工具，不在本阶段。

## 6. Windows 上需要补的 runner 部分

1. **进程采样**：`proc_windows.go`，实现 CPU 和内存读取（§5）。
2. **合成器采样**：`dwm_windows.go`，读 `dwm.exe` 的 CPU，接入现有的空闲合成器基线逻辑（R44、R46）。
3. **窗口前置和校验**：用 `EnumWindows` 按进程号找到窗口，`SetForegroundWindow` 前置，`GetForegroundWindow` + `GetWindowThreadProcessId` 校验，替代 `osascript`/`lsappinfo`。
4. **锁屏检测**：`OpenInputDesktop` 失败或会话被锁时拒绝开始，测量中途锁屏则标记 `screen_locked`。
5. **负载门槛**：Windows 没有 1 分钟负载。改为"其他进程的 CPU 占用"（`GetSystemTimes` 减去本次场景进程）加上"有没有其他 `*.test.exe` 或 `go.exe` 在跑"。忙则等待，最长 10 分钟，超时则标记 `disturbed`，和现有逻辑一致。
6. **进程组停止**：Ctrl-C 时用 Job Object 结束场景程序及其子进程，替代 Unix 进程组。
7. **构建**：`manifest.go` 的 `pinnedGOOS`/`pinnedGOARCH` 改为按运行平台选择；`build.sh` 之外提供 `build.ps1`（或让 `runner -build` 直接可用），`setup.sh` 同样提供 Windows 版本。
8. **远程桌面和投屏程序清单**：Windows 版列出向日葵、ToDesk、AnyDesk、TeamViewer、Parsec 等，仅记录，不干预，和 macOS 一致。

## 7. 交付物

- `tools/uibench` 支持 Windows 的代码和测试，macOS 路径不变。
- `tools/uibench/viva/` 场景程序及其无窗口测试。
- `tools/uibench/README.md` 增加 Windows 使用说明。
- `docs/perf/2026-11-2x-windows-baseline.md`：四家 × 8 场景的基线结果，含机器信息、各家版本、每个场景 Tango 的输赢和差距，以及对优化方向的初步判断。

## 8. 已定的测量条件

用户决定：

- **显示缩放**：按本机当前缩放测量（不改成 100%）。runner 校验四家窗口实际绘制的像素尺寸（客户区物理像素），不一致、或有一家靠系统位图拉伸（不感知 DPI）占便宜时，该轮拒测并在报告中标明。
- **电源**：必须接通交流电。runner 开测前用 `GetSystemPowerStatus` 检查，未接电源就拒绝开始；测量中途拔电则标记该轮 `disturbed`。当前电源模式写入每轮记录和报告。
- **场景跑不起来或画错**：任何一家（包括 Tango）某个场景崩溃或明显画错，都记为"无法测量"并写明原因，继续跑剩下的，先出基线。Tango 自己的问题留到优化阶段修。

实施者的决定（标注为我的判断，用户可推翻）：

- **NSGo 来源**：NSGo 官方说明 Windows 版用 Direct2D/DirectWrite、状态为"可运行"。取它最新的发布版放进 `third_party/nsgo`，记录提交号（`setup` 改为同时记录 git 提交号）。在 Windows 上编不过或跑不起来时，按上一条记为"无法测量"，先出三家对比。
- **多进程口径**：四家统一按整棵进程树计量。每个场景程序放进一个 Job Object，CPU 取 Job 的累计时间，内存取 Job 内所有进程之和。若 Viva 发布形态是宿主/worker 双进程，这样也能公平计入。
- **按框架的构建参数**：manifest 允许每个框架登记自己的发布构建 tag（例如 Viva 去掉开发功能的 tag），按各自官方的发布构建方式构建，命令和 tag 记进 manifest。
- **判定阈值重新标定**：合成器空闲阈值（macOS 上是 0.1 核 / 0.6 核 / 0.02 核漂移）和 R51 的期望帧率，在本机按实测重新标定：先在无场景程序时采样 `dwm.exe` 若干轮取空闲水平，期望帧率取显示器当前刷新率（`EnumDisplaySettings`）。标定值写进报告。

## 9. 留给实施者决定的事

- Viva 各场景具体用哪个组件、怎么拼出等价界面。
- Windows 负载门槛的具体阈值，以"另一个会话的 `go test` 在跑时能被拦下"为准。
- runner 的 Windows 代码按平台文件拆分的方式。
