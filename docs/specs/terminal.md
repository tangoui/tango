# 终端组件（ui/terminal）

状态：已批准（2026-11-14，用户审阅全文后批准开工；方案与取舍见第 5、10 节）。

## 1. 意图

给 Tango 自绘 UI 加一个**完整的交互式终端组件**，水平对标 xterm.js：在 Tango 应用里开本地 shell 跑日常命令、vim / htop / tmux / less 这类全屏程序，以及 Claude Code 这类大量使用颜色、鼠标和复杂按键的程序，体验与主流终端一致。

终端视图只认**任意双向字节流**：本地伪终端（PTY）只是其中一种连接，应用也可以接 SSH、容器、远程会话或测试回放。因此视图本身在 Tango 支持的**所有平台**（macOS、Windows、Linux、iOS、Android）都可用；本地 shell 只在桌面上有。

为什么全部纯 Go：用户明确要求**零原生依赖**、全平台一致、一个 `go build` 出完整程序。曾比较过 libghostty-vt（mygo 的做法，兼容性和性能起点最好）与 charmbracelet/x/vt（现有最完整的纯 Go 模拟器，但无回滚重排、无 Kitty 键盘协议、依赖重），均不采用（见第 5 节）。

## 2. 目标 / 非目标 / 反目标

**目标（前三档，按里程碑交付，每档可单独使用）**

- M1 普通 shell：ls、git、构建输出、交互式 shell 行编辑。
- M2 全屏程序：vim、htop、tmux、less 等正常使用并通过兼容性验收。
- M3 Claude Code 一类：真彩色、同步输出、超链接、剪贴板、扩展键盘协议，性能有可衡量的基准并与 xterm.js 对比。
- 视图在三套设计系统（Ant、Element、shadcn）与亮 / 暗主题下都可用；在移动端可接字节流使用（含软键盘与输入法）。
- 纯 Go，零新增外部依赖（`golang.org/x/sys` 已是间接依赖，可直接使用）。

**非目标**

- 第四档图片协议（Kitty 图片、Sixel、iTerm2 内联图片）：本项目不做，后续单独立项。设计上为它预留：网格单元可挂"占位"，渲染层已有图片纹理。
- SSH、Telnet、串口等连接的实现：只提供字节流接口与本地 PTY；其他连接由应用自己实现。
- 终端复用（分屏、标签页、会话持久化）：那是应用层的事，组件只提供单个终端视图。
- 移动端本地 shell：iOS 沙盒不允许、Android 受限，`LocalPTY` 在移动端返回 `ErrUnsupported`。
- Kitty 图片动画、终端内的富文本 / 内联 Web 内容。

**反目标（技术上能用但算失败）**

- 常用程序（vim、htop、tmux、less、Claude Code）画面错乱、按键失灵或需要用户特殊配置才正常。
- 大量输出时（`cat` 大文件、编译日志刷屏）界面卡顿或输入延迟明显。
- 回滚在窗口改变大小后乱掉（行断在错误位置、内容丢失）。
- 远端程序能借终端读取本机文件或无提示地读写剪贴板。
- 照搬 mygo、Ghostty、xterm.js 或 charmbracelet 的代码（只借设计思路，按 Tango 惯例自己写）。

## 3. 约束

- Go 1.27，模块 `github.com/tangoui/tango`，MIT；桌面无 cgo；新依赖只允许纯 Go，本设计不新增外部依赖。
- 自绘 UI 现状：即时模式 API + 保留状态（`ui.State`），`ui/text` 排版（一段文字单一 Style，`Style.Mono` 选等宽字体链），`paint.List` 绘制，GPU（Metal / D3D11 / GL）与 CPU 两条渲染路径，局部重绘提示 `Context.Damage` / `ScrollShift`。
- 输入：`Context.KeysFor` / `TextFor` / `CompositionFor` / `EventsFor`；输入法 `WantTextInput`、`WantTextInputState`；Tab 捕获 `Context.CaptureTab`（已为终端预留）；剪贴板经 `Window.ReadClipboard` / `WriteClipboard`。
- **Esc 现状（需要改 `ui/`，见 7.1）**：`ui/runner.go` 把 Esc 排除在 `KeysFor` 之外、只进 `EventsFor`，并置 `c.escape`，浮层（Modal、Drawer、弹出菜单）据此关闭；Esc 之后的 Tab 一律把焦点移出 Tab 捕获控件（`ui/tab_capture.go`）。终端照现状会在 vim 按 Esc 时关掉外层浮层，并在 Esc、Tab 时丢失焦点。
- 目录归属：`ui/terminal/**` 归本项目；改 `ui/` 其他公共文件（如为终端新增 Context 能力）先与对应会话确认。提交只带本项目文件（`git commit -o`），推送前先问用户。
- 验收平台：macOS 完整实机验收；Linux 用桌面实机或容器；Windows ConPTY 交叉编译 + 单测，实机验收等有 Windows 机器；iOS / Android 用模拟器验收视图（接回放字节流或简单回显连接）。

## 4. 架构

三层，单向依赖：

```
ui/terminal（视图，ui 组件）
   │ 读屏幕状态、调用编码器
   ▼
ui/terminal/vt（模拟器，纯逻辑，不依赖 ui）
   ▲
   │ 字节
ui/terminal 的 Conn 接口 ◄── sys/pty（本地 PTY，桌面）
                         ◄── 应用自己的 SSH / 容器 / 回放 …
```

### 4.1 模拟器 `ui/terminal/vt`

输入程序输出的字节，维护屏幕状态；把按键、鼠标、粘贴、焦点变化编码成发给程序的字节。不依赖 `ui`，可在任何地方单独测试。

- **解析器**：DEC ANSI 解析状态机（Paul Williams 的状态表，xterm.js / Alacritty / Ghostty 同源），覆盖 C0/C1、ESC、CSI、OSC、DCS、APC、SOS/PM，参数含子参数（`:`，下划线样式与颜色用）。UTF-8 解码（非法字节按 U+FFFD）。**快路径**：连续的可打印 ASCII 不进状态机，批量写入当前行。
- **字形宽度**：按 Unicode 东亚宽度与 emoji 表判定 1 或 2 格；字形簇（grapheme cluster：组合字符、ZWJ emoji、变体选择符）占一个"格组"。**宽度与字形簇规则以 xterm.js（`@xterm/xterm` 6.x，含 unicode11 / unicode-graphemes addon 的行为）为准**，与兼容性参考一致（10.3）；表随 Unicode 版本生成（`go generate`）。
- **网格与回滚**：
  - 行存在环形缓冲，回滚上限按行数配置（默认 10 000 行，可调）。
  - 单元格紧凑存储：码点（字形簇超出单码点时指向行内附加表）+ 样式索引；样式（前景、背景、下划线颜色、属性位、超链接 ID）放在按终端去重的样式表里，单元格只存 16 位索引。
  - 每行记录"软换行"标志，**改变列数时重排**（reflow）回滚与主屏，光标跟随内容，行为与 xterm.js / Ghostty 一致。备用屏幕不重排、不入回滚。
- **状态**：主屏 / 备用屏、光标（位置、样式、可见、闪烁）、保存与恢复、滚动区域（上下与左右边距）、制表位、字符集（G0–G3、DEC 线框）、各类 DEC 与 ANSI 模式、标题、工作目录、超链接表、调色板（OSC 4 / 10 / 11 / 12 可改可查）。
- **编码器**：按键（传统 VT/xterm 序列、应用光标 / 小键盘模式、xterm modifyOtherKeys、Kitty 键盘协议的 disambiguate escape codes 与 report event types / alternate keys 中能由现有 `input.KeyEvent` 表达的部分，见 10.2）、鼠标（X10 / normal / button / any 事件，SGR / SGR-pixel / UTF-8 / urxvt 格式）、括号粘贴、焦点上报。
- **回复**：设备属性（DA1/DA2/DA3）、设备状态与光标位置（DSR/CPR）、模式查询（DECRQM）、XTVERSION、XTGETTCAP（基本项）、窗口尺寸查询（XTWINOPS 18/14 等）、颜色查询。回复经 Conn 写回程序。
- **并发**：模拟器本身不加锁；`Terminal` 内部用一把互斥锁串行化"读循环写入"和"UI 读取 / 编码"，读循环与 UI 线程分离（见 4.3）。

### 4.2 视图 `ui/terminal`

即时模式组件：

```go
t := terminal.New(terminal.Options{Conn: conn, Scrollback: 10000})
// 每帧：
terminal.View(c, t).Layout(layout.Fill())
```

- **绘制**：等宽字体（`Style.Mono`，字号可配），按行绘制；同一行连续同样式的格合并成一段文字排版并缓存（行内容 + 样式不变则复用）。宽字符占两格；字体缺字走系统回退链。**线框与块字符**（U+2500–259F）与 Powerline 分隔符自己画，保证线条衔接；粗体可选"加亮颜色"（xterm 行为）。下划线（单、双、波浪、点、虚线，含独立颜色）、删除线、上划线、反色、暗淡、闪烁（可关）、隐藏。
- **局部重绘**：模拟器记录脏行；视图只对脏行发 `Context.Damage`，整屏滚动时发 `ScrollShift`（GPU 局部重绘已支持），光标闪烁只重画光标格。
- **尺寸**：视图大小按字体格宽、格高换算成列数与行数，变化时调模拟器重排并通知 Conn（`Resize(cols, rows, px)`）；连续拖动时节流（约 50 ms 一次），避免程序收到大量 SIGWINCH。
- **键盘**：聚焦时 `CaptureTab` 与 `ClaimEscape`（7.1）；离开终端用 Ctrl+Tab / Ctrl+Shift+Tab（与 VS Code 终端一致），终端内不启用"Esc 后 Tab 离开"规则；`KeysFor` 的按键交给编码器；`TextFor` 文本按 UTF-8 发送；`CompositionFor` 的组字在光标处画出（不发给程序，提交后才发），`WantTextInput` 把光标位置报给输入法候选窗。应用级快捷键（复制、粘贴、全选、搜索、清屏、字号缩放）与发给程序的按键冲突时按平台惯例决定（macOS 用 Cmd，其他平台用 Ctrl+Shift），可配置。
- **鼠标**：程序开启鼠标上报时交给编码器（Shift 按住时绕过、留给本地选区，xterm 惯例）；否则本地处理：拖动选择、双击选词、三击选行、Alt / Option 拖动矩形选择、拖出边界自动滚动；滚轮在主屏滚回滚，在备用屏且程序未开鼠标时转成方向键（xterm "alternate scroll"）。
- **选区与复制粘贴**：选区跟随内容滚动；复制时去掉软换行、行尾空白；粘贴走括号粘贴（程序开启时），多行粘贴不加确认（与主流终端一致，安全开关见 4.5）。
- **滚动**：回滚滚动条（overlay，与 Tango 滚动条一致），有新输出时若用户不在底部则不跳动并提示，键入时回到底部（可配）。
- **搜索**：在回滚加屏幕中查找（普通 / 区分大小写 / 正则），高亮全部匹配并逐个跳转。
- **链接**：OSC 8 超链接；可选自动识别 URL。点击需按修饰键（macOS Cmd，其他 Ctrl），交给应用提供的 `OnOpenLink` 回调（与编辑器一致；`ui` 层拿不到 `*tango.App`）。示例与文档推荐在回调里调用 `sys/shell.OpenURL`（受其 scheme 白名单约束）；未设回调时链接不可点击。
- **无障碍**：终端区域是一个可读文本节点，读屏器可读当前屏幕与光标行；新输出按"礼貌"级别播报（可关）。不困住键盘（WCAG 2.1.2）：Ctrl+Tab / Ctrl+Shift+Tab 始终能离开终端（`tabCaptured` 现有规则）；终端认领 Esc，所以"Esc 后 Tab 离开"在终端里不生效（见 7.1、10.1），这一点写进组件文档。
- **主题**：
  - 终端配色：16 色调色板 + 前景、背景、光标、选区；默认按当前主题亮 / 暗选一套内置配色，应用可换（内置少量常用配色：默认亮、默认暗、Solarized、Dracula 等）。
  - 外框、滚动条、搜索框、右键菜单跟随 Ant / Element / shadcn 的组件令牌，新增 `theme.TerminalToken`（遵守令牌约定：放进对应 XToken，不往 Theme 加扁平字段，带对比度标签）。
- **移动端**：软键盘与输入法经现有 M3 通道；视图上方提供可选的辅助键行（Esc、Tab、Ctrl、方向键等，因为软键盘没有）；长按选择、拖柄后续再做，第一版用拖动选择。

### 4.3 连接 `Conn` 与读循环

```go
// Conn 是终端另一头：程序的输出流与输入流。
type Conn interface {
	io.Reader          // 程序输出
	io.Writer          // 发给程序的输入（按键、粘贴、回复）
	Resize(cols, rows, widthPx, heightPx int) error
	Close() error
}
```

`Terminal` 启动一个 goroutine 读 Conn，把字节写进模拟器（持锁），然后 `Invalidate` 窗口。大流量时**按帧合并**：一帧内到达的数据全部写完再请求一次重绘；单次持锁有上限，避免 UI 线程长时间等待。Conn 读到 EOF 或出错时 `Terminal` 进入"已结束"状态（视图显示退出状态，可配置自动关闭）。

### 4.4 本地伪终端 `sys/pty`

```go
pty, err := pty.Start(pty.Options{Path: shell, Args: args, Dir: dir, Env: env, Cols: 80, Rows: 24})
// pty 实现 terminal.Conn，另有 Wait()、Pid()、Signal()
```

- macOS / Linux：`/dev/ptmx` + `ioctl`（授权、解锁、取从设备名；Linux 用 `TIOCSPTLCK` / `TIOCGPTN`），子进程用 `exec.Cmd` 启动，`Setsid` + `Setctty` 使从设备成为控制终端，`TIOCSWINSZ` 设尺寸。经 `golang.org/x/sys/unix`，无 cgo。
- Windows：ConPTY（`CreatePseudoConsole`、`ResizePseudoConsole`，Windows 10 1809+），`syscall` 调用，进程用 `PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE` 启动；旧系统返回明确错误。
- 默认 shell：macOS / Linux 取 `$SHELL`，否则 `/bin/sh`；Windows 取 `pwsh` / `powershell` / `cmd` 中第一个存在的。环境默认继承并设 `TERM=xterm-256color`、`COLORTERM=truecolor`。
- iOS / Android：`Start` 返回 `ErrUnsupported`。

### 4.5 安全

- **剪贴板**：OSC 52 写剪贴板默认允许，**读**默认拒绝；应用可改为"允许 / 询问 / 拒绝"。
- **超链接与 URL**：只经应用的 `OnOpenLink` 回调打开，默认不打开；文档与示例要求回调使用 `sys/shell.OpenURL`（scheme 白名单）。悬停显示目标，点击需修饰键。
- **标题与回复注入**：标题、工作目录长度有上限；任何"把终端内容回显给程序"的查询（如 DECRQSS 返回可被构造的字符串）按 xterm 的安全做法处理，不回显标题（不实现 `CSI 21 t` 的标题回报）。
- **粘贴**：括号粘贴下去掉粘贴内容里的结束标记，防止粘贴内容伪造按键；可选"多行粘贴前确认"（默认关）。
- **资源**：单次 OSC / DCS 载荷有长度上限（默认 1 MB）；回滚、超链接表、样式表有上限，超出按最旧淘汰。

## 5. 方案对比（已与用户确认）

| 方案 | 取舍 | 结论 |
|---|---|---|
| **纯 Go 自研模拟器**（本设计） | 全平台天然可用、零原生依赖、一个 `go build`；兼容性要靠测试套件逐步打磨，性能需专门优化 | **采用**（用户选择，2026-11-14） |
| libghostty-vt（经 purego 加载，mygo 的做法） | 兼容性与性能起点最好；需维护 Zig 构建链与多平台原生库托管，C API 不稳定，移动端未验证，破坏"零原生依赖" | 不采用 |
| charmbracelet/x/vt（现有纯 Go 库） | 现成；无回滚重排、无 Kitty 键盘协议、无性能优化，依赖 Charm 的 TUI 框架等十余个间接依赖 | 不采用（可作对照测试） |

## 6. 里程碑与验收

| 里程碑 | 内容 | 验收 | 估算 |
|---|---|---|---|
| **M1 普通 shell** | 解析器与快路径、UTF-8 与宽度、网格与回滚（固定宽度，暂不重排）、SGR（16 / 256 / 真彩色与基本属性）、光标移动与擦除、视图绘制与局部重绘、选区复制粘贴、滚动条、输入法、传统按键序列、Conn 接口与读循环、`sys/pty` 三个桌面平台、示例 `examples/terminal` | macOS 实机：zsh / bash 交互、`ls --color`、`git log`、长构建输出；回放测试与单测；Linux 容器跑 bash；Windows 交叉编译 + ConPTY 单测；iOS / Android 模拟器用回显连接验证视图与软键盘 | 4–5 人周 |
| **M2 全屏程序** | 备用屏、滚动区域（含左右边距）、光标保存恢复与样式、DEC 模式全集、字符集与线框绘制、插入删除行列字符、制表位、鼠标上报（全部模式与格式）、括号粘贴、焦点上报、**改变列数时重排**、alternate scroll、设备属性与状态回复、搜索、链接 | vttest 主要菜单（光标移动、屏幕特性、字符集、双宽、插删、鼠标）逐项通过并记录；vim、htop、tmux、less、man、nano、top 录制回放与参考快照逐格一致；实机手测 | 4–5 人周（累计 8–10） |
| **M3 Claude Code 一类** | 同步输出（mode 2026）、OSC 8 超链接、OSC 52 剪贴板（含策略）、OSC 0/2/7、OSC 4/10/11/12、扩展下划线与颜色、modifyOtherKeys 与 Kitty 键盘协议（渐进增强各级）、XTVERSION / XTGETTCAP、性能优化与基准、无障碍完善、移动端辅助键行 | Claude Code、neovim（带 Kitty 键盘）、lazygit、btop 实机正常；Shift+Enter 等组合键可区分；**性能基准**（见下）达标并与 xterm.js 对比；安全项（4.5）逐条有测试 | 3–4 人周（累计 11–14） |

**性能目标（M3 验收，macOS Apple Silicon，GPU 渲染）**

- 吞吐：`cat` 100 MB 纯 ASCII 文本，模拟器吞吐 ≥ 200 MB/s（不含渲染）；整体（含渲染、按帧合并）完成时间不超过 xterm.js（WebGL 渲染器）的 1.5 倍。
- 交互：按键到上屏 p95 ≤ 16 ms（含 PTY 往返）。
- 滚动：回滚 10 万行中滚动，60 fps，p99 帧时间 ≤ 33 ms。
- 内存：10 000 行回滚、200 列，模拟器内存 ≤ 30 MB。
- 具体数字在 M1 先测基线，若与 xterm.js 差距过大再调整目标（需用户确认）。

**兼容性测试资产**（M1 起建立，持续积累）

- 单测：每个控制序列的状态转换表驱动测试。
- 回放：录制真实程序的输出字节（PTY 抓取），**参考屏幕由 Node 下无头运行的 xterm.js 生成**（10.3），Tango 模拟器回放后逐格比对（字符、样式、光标）。xterm.js 本身的已知缺陷记在差异清单里，逐条注明 Tango 采用的行为与依据。
- vttest：脚本化跑主要菜单，截屏人工核对一次后存为黄金快照。
- 模糊测试：解析器对随机字节不 panic、不越界、不无限循环（`go test -fuzz`）。

## 7. 依赖与归属

- 新增包：`ui/terminal`、`ui/terminal/vt`、`sys/pty`、`examples/terminal`；`ui/theme` 新增 `comp_terminal.go`（与 min-95 确认后加）。
- 需要的 `ui` 公共能力：见 7.1（Esc 认领，M1 前置）。其余现有接口（`KeysFor` / `TextFor` / `CompositionFor` / `CaptureTab` / `Damage` / `ScrollShift` / 剪贴板）够用；实现中发现新缺口，写进计划单独提出，改前与对应会话确认。
- 开发工具：回放参考屏幕生成器用 Node + `@xterm/headless`，放在 `ui/terminal/vt/internal/refgen`（仅开发时使用，不进产品，与 `ui/editor/langs/gen` 用 Node 的做法一致）；生成的参考快照提交进 testdata，测试本身不需要 Node。
- API 稳定性：遵守 `docs/specs/api-stability.md`。新包必须在 `api/tiers.txt` 登记（否则 `go run ./internal/cmd/apitxt -check` 失败）：`ui/terminal/... experimental`、`sys/pty experimental`（精确行优先于 `sys/... stable`），与新包同一个提交里加，并更新 golden 文件。M3 结束后再决定是否升为稳定（升级需用户批准）。

### 7.1 Esc 认领（M1 前置，改 `ui/`）

新增 `Context.ClaimEscape(id ID)`：与 `CaptureTab` 一样每帧在 build 时调用；`id` 有焦点时，当帧的 Esc 按下进入 `KeysFor`（及 `EventsFor`），**不**置 `c.escape`、不触发浮层关闭，也不记 `tabEscaped`。代码编辑器（vim 模式等）以后也可以用。离开焦点的办法保留 Ctrl+Tab / Ctrl+Shift+Tab（`tabCaptured` 现有规则）。改动在 `ui/runner.go`、`ui/tab_capture.go`（或新文件 `ui/escape_claim.go`），归 min-95 的 RTL / 组件令牌范围，**先与 min-95 确认后由本项目实现**，带回归测试：普通控件 Esc 仍关闭浮层；认领 Esc 的控件在 Modal 里收到 Esc 且 Modal 不关闭；Ctrl+Tab 仍能离开。

## 8. 交给实现者决定的事

- 单元格的具体位布局、样式表与字形簇附加表的数据结构。
- 行内排版缓存的键与淘汰策略。
- 按帧合并的具体上限（单次持锁字节数、时间）。
- 内置配色的具体色值（参考公开的常用配色定义，按 Tango 对比度规则校验）。
- 回放测试的录制工具与文件格式。
- 示例程序的具体界面。

## 9. 待定 / 风险

- **兼容性长尾**：程序用到的冷门序列很多，估算上限主要留给这部分。以回放测试为准逐步覆盖。
- **性能**：纯 Go 解析和每帧排版是主要瓶颈；快路径、行缓存、局部重绘是主要手段。若 M1 基线离目标很远，M2 前先做一轮性能专项。
- **Windows ConPTY**：没有实机，行为差异（例如 ConPTY 自己会重绘、改写部分序列）只能等实机验收；M1 先保证编译与单测。
- **移动端软键盘**：终端需要 Ctrl、Esc、方向键，辅助键行的交互需要在模拟器上调。
- **宽度表与字体不一致**：某些 emoji、私有区字符在字体里的宽度与宽度表不同，以宽度表为准、按格裁剪，与主流终端一致。

## 10. 已定取舍（2026-11-14 用户确认）

### 10.1 Esc
按 7.1 实现 `ClaimEscape`，终端内 Esc 交给程序，Esc 后 Tab 不离开；离开终端用 Ctrl+Tab。

### 10.2 键盘信息：不改 `ui/input`
现有 `input.KeyEvent` 只有布局字符（字母、数字）或美式物理位置、Shift / Ctrl / Alt / Meta 四个修饰键，没有 Caps Lock / Num Lock / Super、F13–F24、媒体键，鼠标没有前进后退键。本项目**不**扩展 `ui/input`（稳定包，涉及五个平台宿主）。Kitty 键盘协议实现 disambiguate escape codes（flag 1）、report event types（flag 2，按下 / 重复 / 抬起），以及 alternate keys（flag 4）中能由 Shift 推出的部分；report all keys as escape codes（flag 8）与 associated text（flag 16）按现有信息尽力实现。已知限制写进文档：非美式键盘布局下 Kitty 的"未按 Shift 时的键"可能不准，锁定键状态不上报，鼠标前进后退键不上报。足以区分 Shift+Enter、Ctrl+I 与 Tab、Ctrl+[ 与 Esc 等 Claude Code、neovim 的常用组合。以后扩展 `ui/input` 另立项目。

### 10.3 兼容性参考：xterm.js
逐格比对的标准答案由 xterm.js 生成（无头，Node 开发工具，见第 7 节）；字符宽度、字形簇、各序列行为以 xterm.js 为准。xterm.js 的已知缺陷或与主流终端明显不一致处，在差异清单里逐条记录 Tango 的选择与依据（例如参考 Ghostty 或 xterm 的行为）。
