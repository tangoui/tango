# `tan mcp`：给 AI 代理用的项目与界面操作（设计）

日期：2026-11-19 · 状态：待用户审阅

## 目的

让 AI 代理（Claude Code 等支持 MCP 的客户端）通过一个 MCP 服务器做两类事：

1. **项目操作**：新建、检查、诊断、构建、打包 Tango 项目，生成绑定，以开发模式运行。
2. **界面操作**：运行 Tango 应用，读它的语义树（控件树），点击、输入、按键、截图，判断界面是否符合预期。

wails3 自带 `wails3 mcp`，但只做第 1 类。Tango 已经有进程内的自动化通道（`docs/automation.md`），不需要窗口在前台、不依赖系统辅助功能或屏幕录制权限。把它开放给代理之后，功能类 GUI 验证就不用再抢前台、等测量时段，也不受 cua 授权状态影响。这是用户这次要做 MCP 的主要原因。

## 目标

- `tan mcp` 子命令，通过标准输入输出（stdio）与客户端通信，也可以选本机回环地址上的 HTTP。客户端只需配置一次，例如 `{"command": "tan", "args": ["mcp", "-root", "<项目目录>"]}`。
- 项目工具：覆盖 `tan` 现有的 init、build、dev、package、generate、doctor，加上只读的项目检查。耗时的操作放到后台任务里跑，提供查状态、读输出、停止。
- 界面工具：启动自绘或 WebView 应用的开发构建，连上自动化通道，提供读树、查找、动作（按下、聚焦、设值……）、坐标输入、按键、文字输入、截图、等待、退出；也能连接一个已经在跑的开发构建。
- 应用默认在后台启动（`TANGO_LAUNCH_BACKGROUND=1`），不抢前台、不遮挡别的窗口。`dev` 任务启动的应用同样带上这个变量。这个开关目前只在 macOS 生效；Windows、Linux 上窗口仍可能出现在前面，`docs/mcp.md` 写明这一限制。
- 返回结果是给代理读的：结构化 JSON 与简短文字，截图以 MCP 图片内容返回，树的文字形式沿用 `a11y.Tree.Dump`。

## 非目标

- **热重载（`tan dev`）下的界面操作**：自绘窗口画在 dev host 进程里，worker 一侧没有语义树，自动化通道目前不支持（`docs/automation.md`）。`tan mcp` 的界面工具一律另起开发构建，不带热重载。热重载下的界面操作留作后续：需要先让 devhost 协议转发语义树。
- 发布构建上的任何界面操作。发布构建没有自动化通道（编译时就去掉了），这一点不变。
- 远程访问：只监听本机回环地址，不提供局域网或公网访问。
- 执行任意 shell 命令：工具只接受定义好的参数，不接受命令行字符串。
- 前台性能测量、输入法验收：这两类仍需真实前台窗口，按现有约定协调时段，不属于这次范围。

## 反目标（技术上能用也算失败的情况）

- 代理调用一次界面工具就把窗口抢到前台、打断用户。
- 自动化通道或 MCP 的某个开关让发布构建能被远程控制。
- MCP 按代理给的路径读写、启动 `-root` 以外的文件或程序（tan 自己的临时目录、Go 工具链与缓存除外，见"根目录限制"）。
- 工具失败时只给一句"failed"，代理无从判断原因。错误要带原因，找不到节点时附当前树。

## 约束

- Tango 自研：只借鉴 Wails 的思路（工具清单、后台任务、根目录限制），接口和实现按 Tango 风格写，不照搬代码。
- MCP 服务器代码放在 `cmd/tan` 模块，第三方依赖（如 MCP SDK）只进 `cmd/tan`。tango 主模块只新增一个内部客户端包（见下一条），没有新的第三方依赖；用户应用只在测试里用到 `automation` 包时才会链接它，发布构建体积不变。
- 复用现有的自动化协议 `internal/automation`（`cmd/tan` 的导入路径在 `github.com/tangoui/tango` 之下，可以导入）。不新增应用侧协议。
- `automation` 包目前面向测试，依赖 `testing.TB`、`TANGO_E2E`。不依赖测试的客户端（连接、鉴权、请求，以及构建开发版并启动、读 token 文件）抽到主模块的内部包 `internal/automation/client`；`automation` 包改为基于它，对外 API 不变。`cmd/tan` 在 `github.com/tangoui/tango` 路径之下，可以导入这个内部包。
- 发布：tango 主模块、`cmd/tan`、`@tangoui/runtime` 同一个版本号一起发布（`docs/release.md`），所以客户端放主模块不会有先后顺序问题；本地开发靠仓库根的 `go.work`。
- 新增的 `tan mcp` 子命令和参数要更新 `cmd/tan/testdata/cli.golden`（`TestCLIGolden -update`）。
- 提交作者固定为 `tango <903300326@qq.com>`，不加 Co-Authored-By。

## 做法

考虑过三种，用户选了 A。

- **A（选定）：`tan mcp` 一个入口。** 项目工具包装现有子命令；界面工具由 MCP 自己启动开发构建并连自动化通道，或连接已在跑的开发构建。入口只有一个，进程和权限由 tan 统一管理，发布构建的安全边界不变。代价是要把自动化客户端从测试依赖里抽出来。
- B：开发构建的应用自己对外提供 MCP。少一层转发，但每个应用一个端口、两个 MCP 入口，应用里多一套对外接口，安全面变大，与"开发功能只走自动化通道"的设计相反。
- C：单独的 `tango-mcp` 程序。要多装一个程序并保持和 tan 版本一致，相比 A 没有实际好处。

## 已定的部分

### 启动与传输

- `tan mcp [-root DIR] [-stdio | -http] [-addr 127.0.0.1:PORT] [-token TOKEN]`。
- 默认 stdio。`-http` 时只接受回环地址（`127.0.0.1`、`localhost`、`[::1]`），其他地址直接报错退出。
- HTTP 模式必须带令牌：请求头 `Authorization: Bearer <令牌>`，常量时间比较；没有 `-token` 时随机生成一个，打印到 stderr。用 MCP 的 streamable HTTP 传输，并校验 `Origin` 和 `Host` 只能是回环地址（防止网页通过 DNS 重绑定访问本机端口）。stdio 模式不需要令牌，能连上 stdio 的就是启动它的进程。

### 根目录限制

- 工具参数里的路径（项目目录、`init` 目标、`app_start` 的包或可执行文件）都按 `-root` 解析，解析后（包括跟随符号链接）必须仍在根目录内，否则拒绝。
- 例外：tan 自己管理的临时目录（构建出的开发版、token 文件）和 Go 的模块缓存、构建缓存。`go build` 本来就会读写它们，也会执行项目里的构建逻辑，这属于"构建你自己的项目"，不算越界。
- `app_attach` 用地址加令牌，或 token 文件路径连接；token 文件可以在根目录外，因为能否连上由令牌决定，与路径无关。
- `-root` 默认当前目录。

### 项目工具

名称用 `tango_` 前缀，比如 `tango_project_inspect`。

| 工具 | 作用 | 执行方式 |
|---|---|---|
| inspect | 读 `tan.json`、模块路径、前端类型、目标平台、可用的示例和入口，不改文件 | 立即返回 |
| doctor | `tan doctor` 的结果，结构化为每项的名称、状态、说明（直接复用 doctor 的检查列表） | 立即返回 |
| init | 从模板新建项目，目标目录必须在根目录内且为空 | 立即返回 |
| generate | 生成绑定 | 后台任务 |
| build | `tan build`，参数限于平台、debug、tags 等现有选项 | 后台任务 |
| package | `tan package` | 后台任务 |
| dev | `tan dev`（热重载，供人看或代理读日志） | 后台任务 |
| job_status | 任务状态、退出码、最近若干行输出（有上限） | 立即返回 |
| job_stop | 停止任务及其子进程 | 立即返回 |
| jobs | 列出本服务器启动的任务 | 立即返回 |

### 界面工具

| 工具 | 作用 |
|---|---|
| app_start | 以开发构建启动应用（包，或已用 `-tags tango_dev` 构建的可执行文件），后台启动，返回会话 ID 和窗口列表。可传应用参数 args、环境变量 env、额外构建标签 tags；env 不能设置 `TANGO_` 开头的变量（自动化通道、后台启动由 tan 控制），这一版不提供前台启动 |
| app_attach | 连接已在跑的开发构建，参数是 token 文件路径或地址加令牌 |
| app_stop | 让应用退出，超时再结束进程 |
| windows | 列出窗口 |
| tree | 某个窗口的语义树（`a11y.Tree.Dump` 文字形式：缩进、角色、名称、值、状态；可限制最大节点数） |
| find | 按角色、名称（完全匹配、包含、正则）、状态、值查找，可等待出现 |
| act | 对节点执行动作：按下、聚焦、设值、展开、折叠、增减、滚动到可见、关闭 |
| click / type / key | 坐标点击、文字输入、组合键（如 `Cmd+S`） |
| wait | 等节点出现、消失或取某个值 |
| screenshot | 窗口截图，以 MCP 图片内容返回；可选只截某个节点的区域 |
| eval | WebView 窗口执行 JS 表达式并返回结果 |
| web_click / web_fill / web_text / web_wait | WebView 窗口按 CSS 选择器点击、填值、读文字、等待元素或 JS 条件（与 `automation.Window` 的 `ClickSelector`、`Fill`、`TextOf`、`WaitSelector`、`WaitJS` 对应，基于 eval 实现）。默认模板 `vanilla` 是 WebView 项目，这组工具是必需的 |
| output | 应用的标准输出和错误（最近若干 KB） |

- 节点用查询定位：角色、名称（完全匹配、包含、正则）、第 n 个、状态、值，和现有协议的 `Query` 一致。`tree` 的文字输出沿用 `a11y.Tree.Dump`，供代理阅读、写查询。协议没有按节点 ID 定位的请求，这一版不新增；重名时用"第 n 个"或加状态、值区分。
- 会等待的工具（`find`、`wait`、`web_wait`、`app_start`）都有超时参数，有默认值和上限，超时返回当前状态而不是一直挂着。
- 自绘窗口：`tree`、`find`、`act`、`click`、`type`、`key`、`wait` 可用。WebView 窗口：`eval` 和 `web_*` 可用，`screenshot` 两种都可用。用错时返回明确的提示，指出该用哪组工具。
- 一个服务器可以同时管理多个应用会话。会话属于服务器，不属于某条 MCP 连接：HTTP 客户端断开不关应用，服务器退出时关闭它启动的所有应用（`app_attach` 连上的不关，只断开）。
- 界面工具的应用和项目工具的 `dev` 任务互不影响。代理要操作界面时用 `app_start`，不用 `dev`。

### 失败信息

- 找不到节点：返回查询条件、窗口和当前树（截断）。
- 应用提前退出：返回退出码和最后的输出。
- 不是开发构建：返回明确的提示（需要 `-tags tango_dev`），与 `automation.Launch` 现有的报错一致。

### 文档与发布

- 新增 `docs/mcp.md`：配置方法（Claude Code 的 `.mcp.json` 示例）、工具清单、安全模型、热重载限制。
- 更新 `docs/automation.md`（提到 MCP 入口）、`CHANGELOG.md`。

## 留给实现者决定

- 用官方 Go MCP SDK 还是自写最小的 JSON-RPC 实现。倾向官方 SDK，前提是它的版本稳定、依赖少；只影响 `cmd/tan` 模块。
- 后台任务输出缓冲的上限和截断方式；树文字的默认节点上限。
- 项目工具的执行方式。已知的约束：`tan` 子命令直接写 `os.Stdout`，在 stdio 模式下会破坏协议，所以 build、package、generate、dev、init 以子进程方式运行 tan 自身（`os.Executable()`），输出进任务缓冲。
- 各工具的参数和返回的确切 JSON 结构。
- 测试方式：项目工具用临时项目目录；界面工具至少用一个自绘示例（`examples/ledger`）和一个 WebView 示例（`examples/hello`）跑端到端（需 `TANGO_E2E=1`，后台启动）。
