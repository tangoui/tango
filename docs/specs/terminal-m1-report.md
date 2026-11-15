# 终端组件 里程碑 1 验收报告

日期：2026-11-15。对应规格：`docs/specs/terminal.md`（第 6 节 M1）；计划：`docs/plans/2026-11-14-terminal-m1.md`（Task 11）。

结论：**M1 完成**。普通 shell 的日常使用（彩色输出、行编辑、历史、补全、中断、中文、回滚、复制粘贴、改窗口大小）在 macOS 实机上全部通过，Linux、Windows、iOS、Android 按计划完成了能做的验证。验收中发现并修复了一个绘制缺陷（宽字符之后的文字画错位置）。三项性能目标里，模拟器吞吐达标，按键回显延迟和整体 `cat` 耗时没有达标，按计划不改目标，列为 M2 前的性能专项（第 4 节）。另发现一个 Android 宿主的输入法问题，不在终端组件内，交给移动端处理（第 5 节）。

## 1. 环境

- 机器：Apple M4（10 核），16 GB，macOS（Darwin 25.6），Go 1.27.4。共享机器，测量期间 1 分钟负载 18–25（其他会话在编译和测试），绝对耗时偏保守。
- 示例：`examples/terminal`（本任务提交）。窗口 900×600 逻辑像素，Retina 2 倍；默认 GPU 渲染器（Metal）。
- shell：验收用 `/bin/zsh -f`（不读用户的 rc 文件，结果可复现），`PS1='$ '`。

## 2. macOS 实机验收（计划 Task 11 第 2 步）

自动化：`examples/terminal/e2e_test.go`，用 `automation` 包启动示例的开发构建、注入按键、读取终端的无障碍值（视口文字）并截图。复现：

```
TANGO_E2E=1 go test ./examples/terminal -v                       # 全部项目
TANGO_E2E=1 E2E_SHOTS=/tmp/shots go test ./examples/terminal -v  # 同时保存截图
```

连续 3 次全部通过（加上收尾时的 1 次，共 4 次），单次约 18 s。

| 验收项 | 做法 | 结果 |
|---|---|---|
| zsh 交互、彩色输出 | `ls --color=always -la`、`git log --oneline --color=always \| head -20` | 通过；颜色、对齐正确（截图 `01-ls-color`、`02-git-log`） |
| 方向键调历史 | `echo hist-one` 后按 Up | 通过 |
| Tab 补全 | `ls examples/termi` + Tab | 通过（补成 `examples/terminal/`） |
| Ctrl+C 中断 | `sleep 10; echo not-interrupted` 后按 Ctrl+C | 通过，后续命令立即执行 |
| Ctrl+D | 在内层 `zsh -f` 按 Ctrl+D 回到外层 | 通过；外层 `exit` 后应用随程序退出（`OnExit`） |
| 中文 | 输入 `echo 你好` 回车 | 通过；输入行与输出行都正确（截图 `03-chinese`） |
| 长输出时回滚 | 循环输出 1500 行期间滚轮上滚，0.7 s 内视口不动；之后键入回到底部并跟随到输出结束 | 通过（截图 `04-scrolled-back`）。计划写的是 `go build -v ./ui/...`，改用可控的输出循环（同样是持续输出，时长可控） |
| 括号粘贴 | 剪贴板放两行命令，Cmd+V | 通过：两行留在命令行上，回车后才执行（截图 `05-paste`） |
| 选择复制 | 拖选一行，Cmd+C，读系统剪贴板 | 通过（截图 `06-selection`）；测试前后保存、恢复用户剪贴板 |
| Cmd+K 清屏 | | 通过 |
| 改窗口大小 | System Events 把窗口从 900×600 改为 640×400，`stty size` 前后对比 | 通过：37×105 → 23×75（截图 `07-resized`） |
| GPU 与 CPU 渲染器对照 | 两种渲染器回放同一样本（属性、16 色前景/背景、真彩色、256 色、中文、emoji、组合字符、线框与块字符、制表符），`screencapture` 截真实窗口逐像素比 | 通过：最大差 **2/255**（要求 ≤ 4/255），差值 > 4 的像素 0 个 |
| 拼音输入法组字 | — | **自动化未覆盖**：需要切换系统输入法，会影响正在使用的机器。组字绘制由单元测试 `TestViewIMEComposition`、`TestViewKeysDuringCompositionGoToIME` 覆盖；Android 模拟器上用系统输入法实测了组字与提交（第 3 节）。macOS 拼音实机需人工补一次 |

截图在计划工作区 `.superpowers/sdd/2026-11-14-terminal-m1/shots/t11/`（git 忽略；可用上面的命令重新生成）。

### 2.1 验收中发现并修复的缺陷

- **宽字符之后的同样式文字画在错误位置**（`3ef631d`）。`paintRow` 遇到宽字符或带组合符号的字符时结束当前文字段，但没有重置段样式；后面同样式的文字接在旧段上，从旧段起点画出，和前面的文字重叠。`ab你cd` 会把 `cd` 画在 `ab` 上。任何混排中文或 emoji 的行（中文提交信息、中文文件名）都会出错。回放测试比对的是模拟器的格子而不是绘制结果，所以没有发现；GPU/CPU 对照截图暴露了它。修复：结束一段时同时结束其样式；回归测试 `TestViewTextAfterWideCharStartsAtItsCell`（宽字符、emoji、组合字符三种）。
- 段缓存容量固定 512，一屏彩色输出就超过它，每帧都会重新排版（`f53c05d`，本任务开始前修复，见第 4 节）。

### 2.2 观察到但未改的显示差异

- emoji 画成单色字形（取前景色），没有彩色 emoji。规格未要求彩色 emoji；如需要，归入 M3 或图片协议立项。
- 中文按两格居中，字间有空隙（字形宽度小于两格），主流终端同样如此。
- DEC 线框字符集（`ESC ( 0`）未实现，`lqqk` 原样显示；`┌─┬─┐` 用字体字形绘制，线条不衔接。两者都是 M2 的范围（规格第 6 节"字符集与线框绘制"）。

## 3. 其他平台（第 4 步）

| 平台 | 做法 | 结果 |
|---|---|---|
| Linux（容器，arm64） | podman `tango-linuxtest:bookworm-fonts`（Debian bookworm + DejaVu / Noto CJK），`go test ./sys/pty/ ./ui/terminal/...` | 全部通过。注意：在**没有安装字体**的镜像（`golang:1.27-bookworm`）里，依赖排版的 7 个视图测试失败（文字排不出字形），属于环境要求，与其他依赖字体的视图测试一致 |
| Windows | `GOOS=windows GOARCH=amd64 go test -c` 编译 `sys/pty`、`ui/terminal`、`ui/terminal/vt`；示例 `GOARCH=arm64` 编译 | 全部编译通过；ConPTY 运行时仍待实机（规格第 9 节） |
| iOS 模拟器（iPhone 17e，iOS 27.2） | `tan build -platform ios-simulator`，无 PTY 时示例自动改用回显连接 | 视图、GPU 渲染、软键盘（拼音）正常显示；`-auto` 在应用内输入 100 个字符全部回显，延迟 p95 33.1 ms（与桌面一致）。本机 Xcode 没有 Simulator 图形界面，无法从外部点软键盘，真实软键盘点击未验证 |
| Android 模拟器（tango36，API 36） | `tan build -platform android -debug`，adb 安装 | 视图、软键盘正常；`adb input text` 输入回显正确；点软键盘按键组字（`hi` 带下划线画在光标处）、点 ✓ 提交并换行均正确——**但要在窗口重新获得焦点之后**，见第 5 节 |

示例为移动端做了两处调整（本任务提交）：`tan.json`；视图放在安全区内、软键盘上方（`c.SafeArea()` 与 `c.KeyboardInset()`），否则会画到状态栏下面、光标行被键盘挡住（iOS 截图暴露）。

## 4. 性能基线（第 3 步）与规格目标对照

| 项目 | 规格目标（M3 验收） | 实测 | 结论 |
|---|---|---|---|
| 解析器吞吐（`BenchmarkParserASCII`） | — | 1202–1378 MB/s，0 分配 | — |
| 模拟器吞吐（`BenchmarkTerminalCat`，不含渲染） | ≥ 200 MB/s | **207–240 MB/s** | 达标 |
| 回滚滚动（`BenchmarkScrollUp`） | — | 131–181 ns/行 | — |
| 实机 `cat` 50 MB（含 PTY、模拟器、渲染） | 不超过 xterm.js（WebGL）的 1.5 倍 | **1.57–2.15 s**（23.6–32.3 MB/s），期间 58–60 fps | 未与 xterm.js 对比（本机无对照环境）；见下 |
| 同一文件只走 PTY（无界面） | — | 0.59–0.60 s（约 85 MB/s）；加模拟器 0.59–0.70 s | 界面链路多花约 1–1.5 s |
| 按键到回显（含 PTY 往返） | p95 ≤ 16 ms | **p95 33.2 ms**（两次 100 键，p50 33.1，非常稳定）；iOS 模拟器同为 33.1 ms | **未达标** |
| 打字时帧间隔 | — | 平均 16.67 ms，p99 16.8 ms（每帧赶上 vsync） | — |
| 视图每帧绘制（1440×900，约 52 行 × 12 段彩色文字，无界面驱动） | — | 段缓存修复前约 0.52 ms/帧、每帧分配约 460 KB；修复后约 0.26 ms/帧、约 70 KB | — |

**M2 前的性能专项建议**（不改目标）：

1. **按键延迟多一帧**。按键先进 `pending`，在下一帧绘制时由 `applyInput` 编码并发给程序；回显到达后还要再等一帧才画出来，所以稳定在两帧（33 ms）。把按键编码和发送挪到事件到达时（UI 线程收到事件即 `Send`，不等绘制），回显在下一个 vsync 就能上屏，预计 p95 ≈ 一帧（≤ 17 ms）。
2. **整体吞吐**。PTY 本身 85 MB/s、模拟器 200+ MB/s，整体只有 24–32 MB/s：读循环每读一次（缓冲 32 KB）就把数据写进模拟器并请求重绘，UI 线程每帧要拿模拟器锁读屏幕，读循环与绘制争锁（每次持锁最多写 256 KB，`maxLockBytes`）。建议先测锁等待时间，再考虑：绘制只在锁内拷贝可见行、锁外画；大流量时降低每帧对整屏的工作（同步输出 mode 2026 属 M3）。
3. 对比 xterm.js 需要一个浏览器对照环境（WebGL 渲染器），M3 验收前搭建。

## 5. 未验收项与原因

| 项目 | 原因 | 建议 |
|---|---|---|
| macOS 拼音输入法实机组字 | 自动化需要切换系统输入法，会干扰正在使用的机器 | 人工补一次：示例里用拼音输入 `echo 你好` 回车 |
| Windows ConPTY 运行 | 没有 Windows 机器 | 等实机 |
| iOS 真实软键盘点击 | 本机 Xcode 没有 Simulator 图形界面，无法注入触摸 | 真机或装好 Simulator 后补 |
| **Android：应用在第一帧就打开文字输入时，软键盘的输入送不到应用** | 宿主问题，不在终端组件内：`CanvasView` 在窗口获得焦点之前 `restartInput`，系统忽略，软键盘显示但没有有效的输入连接；窗口重新获得焦点（切到后台再回来）后一切正常。任何启动即聚焦输入控件的应用都会遇到（`mobile-ui` 的输入框要用户点击才聚焦，所以没遇到） | 交给移动画布负责人：窗口获得焦点（`onWindowFocusChanged`）时若 `textOn` 则补一次 `restartInput` |
| 与 xterm.js 的吞吐对比 | 无对照环境 | 见第 4 节 |

## 6. 已知差异（`ui/terminal/vt/testdata/replay/DIFFERENCES.md` 汇总）

回放参考为无头 xterm.js 6.0.0 + `@xterm/addon-unicode-graphemes` 0.4.0。Tango 有意不同的地方：

1. 待换行状态下的光标位置报告（CPR）报最后一列，xterm.js 报 `cols+1`（屏幕外）。
2. 冒号形式三段 RGB（`38:2:r:g:b`）按 r:g:b 读（与 Ghostty、foot、WezTerm 一致），xterm.js 会读错颜色。
3. 改变列数时不重排（截断或补空），重排是 M2。
4. **字形簇**（ZWJ emoji、肤色修饰、VS16、国旗对、部分印度系文字的组合元音）按码点分别占格，xterm.js 按簇占一个格组：`👨‍👩‍👧` 6 列（xterm.js 2 列）、`❤️` 1 列（2 列）。这是最大的缺口，需要单独任务（字形簇切分数据）。
5. 格式控制字符（U+200B 等）在字符后面时宽度 0、并入前一格，xterm.js 给它们一格。
6. Unicode 14.0–15.1 新增或变更的字符按 15.1 数据（xterm.js 的表较旧）。

## 7. M2 建议

- 规格 M2 范围：备用屏、滚动区域、DEC 模式、字符集与线框绘制、插删行列、制表位、鼠标上报、括号粘贴以外的模式、焦点上报、**改变列数时重排**、alternate scroll、设备属性回复、搜索、链接。
- 本报告第 4 节两项性能专项（按键延迟、整体吞吐）建议在 M2 开始前做，因为它们决定视图和读循环的结构。
- 字形簇（第 6 节第 4 条）：规格 4.1 要求，M1 裁定推迟（计划台账），建议在 M2 内立一个任务。
- 计划台账里各任务延后的小问题（`minor (deferred)`），包括：失去焦点时清理残留组字、`pending.events` 只构建不绘制时无上限、`Conn.Resize` 在锁内调用需在文档中要求"不得阻塞"、`Signal()` 加入 `sys/pty`、Windows `resolvePath` 与环境变量细节、颜色 API 命名等，M2 计划开始前逐条过一遍。
- 彩色 emoji（第 2.2 节）视需求决定是否立项。
