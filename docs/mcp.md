# tan mcp：给 AI 代理用的 MCP 服务器

`tan mcp` 是一个 [MCP](https://modelcontextprotocol.io) 服务器，让 Claude Code 等 AI 代理直接操作 Tango 项目和运行中的 Tango 应用。工具分两类：

- **项目工具**：查看项目、检查环境、新建项目、生成绑定、构建、打包、`tan dev`，耗时的命令作为后台任务运行，代理随时查状态和输出。
- **界面工具**：以开发构建启动应用（或连接已在运行的开发构建），读语义树、按角色和名称找节点、执行无障碍动作、坐标点击、输入文字、按键、截图；WebView 窗口用 CSS 选择器和 JavaScript 操作。走的是[整机自动化](automation.md)的同一条控制通道，能力和 `automation` 包一致。

界面工具只对开发构建（`-tags tango_dev`）有效，发布构建里没有控制通道。`tango_app_start` 启动 Go 包时自动用 `-tags tango_dev` 构建，不需要自己加；传可执行文件时，它必须是用 `-tags tango_dev` 构建的。在 macOS 上，应用在后台启动，窗口排在其他应用后面。

## 配置

### Claude Code（stdio，推荐）

在项目根目录放一个 `.mcp.json`：

```json
{
  "mcpServers": {
    "tango": {
      "command": "tan",
      "args": ["mcp", "-root", "."]
    }
  }
}
```

`-root` 是工具能访问的项目目录，默认当前目录。stdio 方式不需要令牌：能连上 stdin/stdout 的只有启动它的进程。代理退出（关闭 stdin）时 `tan mcp` 随之退出，并关闭它启动的所有应用和后台任务。

### 验证首次连接（不启动应用）

1. 在含 `tan.json` 的项目根目录运行 `tan mcp -h`。如果提示找不到 `mcp` 子命令，先按[入门指南](../site/guide/getting-started.md)安装包含 MCP 的 CLI；如果客户端找不到 `tan`，把 `.mcp.json` 的 `command` 改成 `tan` 的绝对路径。
2. 从该目录启动 Claude Code，按提示确认项目的 MCP 配置。stdio 服务器由客户端启动，不需要另开终端运行 `tan mcp`。
3. 让助手执行：“请用 `{}` 调用 `tango_project_inspect`，告诉我项目的模块名和界面类型，不要构建或启动应用。” 返回结果中的 `module`、`kind` 和 `config` 应对应当前项目。若返回了别的项目，检查客户端的启动目录和 `-root`。

### HTTP

需要多个客户端共用一个服务器，或客户端不能启动子进程时，用 streamable HTTP：

```sh
tan mcp -root . -http -addr 127.0.0.1:7801 -token my-secret
# stderr：tan mcp: listening on http://127.0.0.1:7801 (token my-secret)
```

- `-addr` 只接受回环地址：`127.0.0.1`、`localhost`、`[::1]`，端口 0 自动选择（默认 `127.0.0.1:0`）。其他地址直接报错退出。
- 每个请求必须带 `Authorization: Bearer <令牌>`，否则返回 401。不给 `-token` 时随机生成一个 256 位令牌，和监听地址一起打印到 stderr。
- `-addr`、`-token` 只能和 `-http` 一起用；`-stdio` 与 `-http` 互斥。

Claude Code 的配置：

```json
{
  "mcpServers": {
    "tango": {
      "type": "http",
      "url": "http://127.0.0.1:7801",
      "headers": { "Authorization": "Bearer my-secret" }
    }
  }
}
```

应用会话属于服务器，不属于某条连接：HTTP 客户端断开后应用继续运行，重新连上的客户端用同一个 `session` 继续操作。服务器退出（Ctrl+C / SIGTERM）时关闭它启动的所有应用。

## 典型流程

1. `tango_app_start {"target": "./examples/ledger"}` → `{"session": "app-1", "windows": [...]}`
2. `tango_ui_tree {"session": "app-1"}` 看语义树，写查询
3. `tango_ui_act {"session": "app-1", "query": {"role": "button", "name": "记一笔"}, "action": "press"}`
4. `tango_ui_type`、`tango_ui_wait`、`tango_ui_find` 继续操作和断言，`tango_ui_screenshot` 看结果
5. `tango_app_stop {"session": "app-1"}` → `{"exitCode": 0}`

前端用 Vite 等工具构建的模板，Go 代码通过 `go:embed` 嵌入 `dist`：先 `tango_project_build`（或至少让前端构建跑一次）再 `tango_app_start`，否则 `dist` 不存在，编译失败或页面是旧的。

WebView 示例 `examples/hello` 把第 1 步的 `target` 换成 `./examples/hello`，再把第 2–4 步换成 `tango_web_fill {"session": "app-1", "selector": "#name", "text": "..."}`、`tango_web_click {"session": "app-1", "selector": "#go"}`、`tango_web_text {"session": "app-1", "selector": "#out"}`。

完整的端到端例子见 `cmd/tan/mcp_e2e_test.go`（`TANGO_E2E=1 go test -run TestMCPE2E -v ./cmd/tan`）。

## 工具清单

所有工具的输出都是 JSON 对象（截图除外，返回 MCP 图片内容）。相对路径从 `-root` 解析。`tango_app_attach` 的 `tokenFile` 也接受绝对路径，并可位于根目录外。

### 项目工具

| 工具 | 主要输入 | 输出 |
|---|---|---|
| `tango_project_inspect` | `dir` | `dir`、`config`（`tan.json` 内容）、`module`、`kind`（`webview`、`ui` 自绘、`unknown`）、`hasFrontendBuild`、`error` |
| `tango_project_doctor` | 无（检查根目录的项目） | `checks`：每项 `name`、`status`（`ok`、`missing`、`warn`、`error`、`-`）、`detail`、`required` |
| `tango_project_init` | `dir`（必须不存在或为空）、`template`（默认 `vanilla`）、`name`、`module` | `dir`、`output`；等待完成，最多 60 秒 |
| `tango_project_generate` | `dir` | 后台任务（见下） |
| `tango_project_build` | `dir`、`platforms`（如 `darwin/arm64`、`windows/amd64`、`ios`、`android`）、`debug`、`tags` | 后台任务 |
| `tango_project_package` | `dir`、`platforms` | 后台任务 |
| `tango_project_dev` | `dir` | 后台任务，一直运行到 `tango_job_stop` |
| `tango_jobs` | 无 | `jobs`：所有任务，不含输出 |
| `tango_job_status` | `id`、`lines`（默认 200 行，最多 64 KiB） | 任务：`id`、`kind`、`args`、`dir`、`state`（`running`、`exited`、`stopped`）、`exitCode`、`started`、`output`、`truncated` |
| `tango_job_stop` | `id` | 任务；先礼貌地发信号，5 秒后结束整个进程组 |

后台任务的工具立即返回任务对象（`id` 形如 `job-1`），用 `tango_job_status` 跟进。

`tan build`、`tan package` 把 `tan.json` 里的前端命令（`frontend.install`、`frontend.build`）放在单独的进程组里运行，`tango_job_stop` 的信号不直接到达它们；tan 收到信号后自己结束这些进程组，`tan mcp` 退出时同样如此。

### 界面工具：会话

| 工具 | 主要输入 | 输出 |
|---|---|---|
| `tango_app_start` | `target`（包目录或开发构建的可执行文件，默认根目录）、`args`、`env`（`KEY=VALUE`，不能是 `TANGO_` 变量）、`tags`、`timeoutMs`（构建后等控制通道，默认 60000，最多 300000） | `session`、`windows` |
| `tango_app_attach` | `tokenFile`（应用写出的 token 文件），或 `addr`（回环地址 `127.0.0.1:端口` 等，或 `unix:` 套接字；其他地址在连接前就拒绝）+ `token` | `session`、`windows` |
| `tango_app_stop` | `session` | `exitCode`（连接的应用没有）、`warning`；启动的应用先请求退出，5 秒不退就结束进程，连接的应用只断开 |
| `tango_app_output` | `session`、`bytes`（默认 16384，最多 262144） | `output`、`truncated`；保留最近 256 KiB 的 stdout 和 stderr；`tango_app_attach` 连上的会话不捕获输出，调用直接报错 |
| `tango_ui_windows` | `session` | `windows`：每个窗口 `id`、`title`、`kind`（`canvas` 自绘、`webview`）、`visible`、`focused`、`width`、`height` |
| `tango_ui_screenshot` | `session`、`window`、`query`（只截该节点区域，自绘窗口） | PNG 图片；自绘和 WebView 窗口都可用 |

`window` 参数都是可选的：窗口 ID 或完整标题，默认唯一的窗口，否则有焦点的窗口，否则第一个。

### 界面工具：自绘窗口

| 工具 | 主要输入 | 输出 |
|---|---|---|
| `tango_ui_tree` | `session`、`window`、`maxNodes`（默认 400，最多 20000） | `tree`（缩进文本，每行一个节点：角色、"名称"、值、[状态]）、`truncated`（省略的节点数） |
| `tango_ui_find` | `session`、`window`、`query`、`timeoutMs` | `node`、`count`（匹配数） |
| `tango_ui_act` | `session`、`window`、`query`、`action`（`press`、`increment`、`decrement`、`expand`、`collapse`、`setValue`、`focus`、`scrollIntoView`、`dismiss`）、`arg`（`setValue` 的值）、`timeoutMs` | `node` |
| `tango_ui_wait` | `session`、`window`、`query`、`gone`（等到不匹配）、`timeoutMs` | `node`（`gone` 时没有） |
| `tango_ui_click` | `session`、`window`、`x`、`y`（窗口内容左上角起的逻辑像素，与节点 `bounds` 同一坐标系）、`button`（`left`、`right`、`middle`）、`clicks`（2 为双击） | `{}` |
| `tango_ui_type` | `session`、`window`、`text` | `{}`；把文字提交给有焦点的输入框，相当于输入法上屏 |
| `tango_ui_key` | `session`、`window`、`keys`（如 `Enter`、`Cmd+S`、`CmdOrCtrl+Shift+Z`） | `{}` |

`query` 与[整机自动化](automation.md)的节点查找相同：`role`（`button`、`textbox` 等 a11y 角色名）、`name`、`match`（`exact` 默认、`contains`、`regexp`）、`nth`（负数从末尾数）、`states`（如 `checked`、`!disabled`）、`value`。节点输出有 `id`、`role`、`name`、`value`、`description`、`states`、`actions`（可用于 `tango_ui_act` 的动作，`|` 分隔）、`bounds`。

会等待的工具 `timeoutMs` 默认 10000，最多 120000。找不到节点时，错误信息里带查询条件和当前语义树（前 200 行）。

在 macOS 独立 UI 线程的内部验证路径上，等待窗口线程超时会返回 `busy`。`tango_ui_act`、`tango_ui_click`、`tango_ui_type` 等动作收到 `busy` 后，先用 `tango_ui_find` 或 `tango_ui_wait` 确认界面状态，已排队的请求可能稍后执行。

### 界面工具：WebView 窗口

| 工具 | 主要输入 | 输出 |
|---|---|---|
| `tango_web_eval` | `session`、`window`、`expr`（JavaScript 表达式，Promise 会等待；多条语句包成 `(() => { ...; return x })()`）、`timeoutMs` | `value`（JSON，`undefined` 为 `null`）；序列化后超过 64 KiB 时 `value` 改为该 JSON 前 64 KiB 的字符串，并带 `"truncated": true` |
| `tango_web_click` | `session`、`window`、`selector`、`timeoutMs` | `{}`；等元素出现，滚到可见、聚焦并点击 |
| `tango_web_fill` | `session`、`window`、`selector`、`text`、`timeoutMs` | `{}`；替换 input / textarea / contenteditable 的值，触发 `input` 和 `change` |
| `tango_web_text` | `session`、`window`、`selector`、`timeoutMs` | `text`（元素的 `innerText`） |
| `tango_web_wait` | `session`、`window`、`selector` 或 `expr`（二选一）、`timeoutMs` | `{}` |

对 WebView 窗口用 `tango_ui_*`（截图除外）、对自绘窗口用 `tango_web_*` 会直接报错，并指出该用哪组工具。

## 安全模型

**允许这些工具，就等于允许代理运行项目里的代码。** `tango_project_build`、`tango_project_package`、`tango_project_dev` 会执行 `tan.json` 里配置的前端命令（`frontend.install`、`frontend.build`、`frontend.dev` 都是交给 shell 运行的命令行），`tango_app_start` 会编译并运行根目录下任意 Go 包或开发构建，`go build` 本身也会执行项目的构建逻辑。在 Claude Code 里放行 `mcp__tango__*`，和放行代理在这个项目里执行这些命令是一回事：只对你信任其内容的项目这样做，`tan.json` 和代码来自他人时先审阅。下面的限制约束的是工具参数（路径、选项、环境变量），不限制项目自身配置的命令能做什么。

- **根目录限制**：工具参数里的路径（项目目录、`init` 目标、`tango_app_start` 的包或可执行文件）都按 `-root` 解析，解析后（包括跟随符号链接，不存在的路径按最近的已存在父目录解析）必须仍在根目录内，否则拒绝。例外：tan 自己的临时目录（构建出的开发版、token 文件）和 Go 的模块缓存、构建缓存——`go build` 本来就会读写它们，也会执行项目里的构建逻辑，这属于“构建你自己的项目”。`tango_app_attach` 的 token 文件可以在根目录外：能否连上由文件里的令牌决定，与路径无关。
- **参数不能变成选项**：`platforms`、`tags` 等列表值不能以 `-` 开头或含空白，工具输入不能给 tan 的命令行多加选项或参数。这只防止参数注入；项目配置的命令照样会执行（见上）。
- **`TANGO_` 环境变量不可覆盖**：`tango_app_start` 的 `env` 拒绝 `TANGO_` 开头的变量。控制通道地址、token 文件和后台启动由 `tan mcp` 设置。
- **HTTP 只监听回环、校验令牌和来源**：监听地址只能是回环地址；每个请求必须带正确的 Bearer 令牌（常量时间比较）；`Host`，以及有 `Origin` 时的 `Origin`，必须是回环地址，否则返回 403——防止网页通过 DNS 重绑定访问本机端口。
- **发布构建没有通道**：界面工具依赖开发构建的控制通道，发布构建里这部分代码整个编译掉（见 [开发功能与生产构建](dev-builds.md)、[整机自动化](automation.md#应用侧启用条件与安全)），`tan mcp` 不改变这一点。连接应用时同样要过控制通道自己的令牌鉴权。
- **退出时清理**：`tan mcp` 退出时停止它启动的所有后台任务（连同子进程）和应用，包括正在启动中的应用和 `tango_project_init`；`tango_app_attach` 连上的应用只断开，不关闭。收到第一个 Ctrl+C / SIGTERM 立即开始清理，进行中的界面请求马上失败；清理期间再收到一次信号，直接强制结束所有子进程后退出。

## 限制

- **热重载不支持自绘界面工具**：`tan dev` 的自绘窗口画在 dev host 进程里，没有语义树（见 [整机自动化 · 限制](automation.md#限制)）。要自动化自绘界面，用 `tango_app_start` 启动开发构建，或用 `tango_app_attach` 连接启用了控制通道的非热重载构建。`tan dev` 的 WebView 窗口启用控制通道后仍可用 WebView 工具。
- **后台启动只在 macOS 生效**：macOS 上应用启动时不激活、窗口排在其他应用后面；Windows 和 Linux 上窗口照常出现在前台。所有操作和截图都走控制通道、在进程内完成，窗口在后面不影响结果。这一版不提供前台启动。
- **前台测量与输入法验收不在范围内**：`tango_ui_click`、`tango_ui_type`、`tango_ui_key` 是合成到 runner 的事件，不经过系统输入法和系统事件队列；截图是进程内渲染，不代表真实呈现。测帧率、测真实呈现或验收输入法，仍要人在前台操作或用平台自己的 UI 自动化工具。
- 协议没有按节点 ID 定位的请求：重名节点用 `nth`、`states` 或 `value` 区分。
