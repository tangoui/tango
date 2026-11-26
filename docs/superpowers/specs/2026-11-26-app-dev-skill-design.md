# Tango 应用开发技能（app-dev skill）设计

日期：2026-11-26
状态：已定稿（用户授权自审后实施；本次不发版）

## 1. 意图

给**用 Tango 开发应用的人**（外部开发者，以及用户自己）的 AI 助手一份技能，让 AI 在写第一行代码之前就知道 Tango 的 API 长什么样、常见需求去哪个包、哪些坑会让代码"能编译但行为不对"，以及怎么把应用跑起来自己验证。

这不是给开发 tango 框架本身用的技能。框架贡献者的规则在根目录 `AGENTS.md`，与本技能无关。

### 为什么现在做：摸底结果

2026-11-26 让两个不带任何 Tango 背景的 AI 从 `tan init` 开始各写一个小应用（日志：`/tmp/tango-baseline-a/BASELINE-LOG.md` 自绘 `ui` 模板的待办+番茄钟；`/tmp/tango-baseline-b/BASELINE-LOG.md` `vue-ts` 模板的 Markdown 笔记）。结论：

- 第一版凭印象写的代码两边各错约 9 处，**错因相同**：按 Wails/Electron 习惯去 `App` 上找方法（`app.SetMenu`、`app.Emit`、`app.NewTray`、`app.Notify`、`app.DataDir`、`dialog.OpenDirectory`……），而 Tango 的系统能力在 `sys/*`、`service/*` 子包里，形如 `pkg.Func(app, …)`。
- 查过 `go doc` 后第二版**一次编译通过**——包文档本身质量好，缺的是"需求 → 包"的入口和对照。
- 剩下的问题是"编译通过、行为不对"的陷阱（见 §6.3）和"怎么验证"（`tan mcp` 的能力边界）。

所以技能的核心价值是：**索引 + 对照表 + 陷阱清单 + 验证流程**，而不是重写一遍 API 文档。

## 2. 目标 / 非目标 / 反目标

**目标**
- 新建项目（`tan init`）自带技能；已有项目可用一条命令装上或更新。
- 技能内容与项目实际依赖的 tango 版本**严格对应**：项目升级 tango 后更新技能，拿到的就是新版本的写法。
- 覆盖两条主路线：自绘 UI（`ui`、`go` 等模板）和 WebView 前端（`vue-ts`、`react-ts`、`shadcn`、`vanilla(-ts)`、`hybrid`）。
- 用摸底复测证明有效：带技能重跑同样的两个任务，第一版凭印象写错的 API 明显减少。

**非目标**
- 不往 `.claude/skills/`、`.cursor/skills/` 等其他目录放副本（用户决定：只放 `.agents/skills/`）。后果：Claude Code 不会自动加载该技能（它只读 `.claude/skills/`），这一点用户已知悉并接受。
- 第一期不覆盖移动端（iOS/Android）专题，只在入口里指向 `site/guide/mobile.md`。
- 不替代 `go doc` 和站点文档；技能里引用它们，不抄全文。
- **本次不发版**（用户决定）：做到合入本地 main、用 `-local` 验证并复测通过为止；发版（打标签、npm runtime、升 `cmd/tan` 依赖、发 tan）之后单独进行，届时才有不带 `-local` 的真实用户路径。
- 不修复摸底发现的框架问题（模板 runtime 版本、macOS 通知、AGENTS.md 说明、文档注释、mcp web 按键）——这些在另一条分支 `fix/app-dev-baseline` 里由 Codex 处理。

**反目标（即使技能能用也算失败）**
- 技能里写的 API 名称、签名与项目所依赖版本不一致——误导比没有更糟。
- 更新命令覆盖了用户自己写的内容。
- 技能入口太长，AI 加载后被淹没、反而漏掉速查表。

## 3. 约束

- 技能格式遵循 Agent Skills 开放标准（agentskills.io）：`SKILL.md` + YAML frontmatter（`name`、`description`），可附带参考文件。Codex、Cursor、Gemini CLI 都从项目的 `.agents/skills/` 读取。
- 正文用**英文**（我的决定：读者是各国开发者的 AI，英文对所有模型最稳；Tango README 也是多语言）。
- `cmd/tan` 是独立 Go 模块；tango 库是根模块 `github.com/tangoui/tango`。
- 已有先例：`cmd/tan/build_mobile.go` 的 `tangoModuleDir` 用 `go list -m -f {{.Dir}} github.com/tangoui/tango` 在项目目录定位所依赖的 tango 模块目录（遵循 replace 和 go.work），移动端构建从那里取原生宿主文件。本设计复用同一机制。
- `tan init` 已有 `Tidy` 选项（跑 `go mod tidy`，失败只警告）。
- 推送前检查照仓库惯例：gofmt -l（含构建标签文件）、`GOOS=linux/windows go vet ./...`、`apitxt -check`、相关测试。提交作者 `tango <903300326@qq.com>`，不加 Co-Authored-By。
- 与 `fix/app-dev-baseline` 分支在 `cmd/tan/init.go`、`AGENTS.md` 等处重叠（那边让生成项目的 Go 与 npm 的 tango 版本对齐、给 AGENTS.md 加贡献者说明、补文档注释）。两条分支都进下一个版本，所以本分支**基于 `fix/app-dev-baseline` 完成并核对后的提交**开发（我的决定），避免两边各改 init.go 再合并冲突；合入 main 时按 fix 在前、本分支在后的顺序。

## 4. 方案（已选定）

### 4.1 技能组织

| 方案 | 取舍 |
|---|---|
| 按开发流程拆成多个技能（起项目 / 界面 / 系统能力 / 验证 / 打包） | 每次加载内容少；但 AI 要自己判断该加载哪个，摸底里最严重的错误发生在写第一行代码前，最可能漏加载 |
| **一个技能：入口写核心规则和速查，细节按主题放参考文件** ✅ | 只有一个入口，AI 一定先看到速查表；参考文件按需读，入口保持短 |

用户选了第二种。

### 4.2 技能内容的来源

| 方案 | 取舍 |
|---|---|
| A. 内容嵌进 `tan` 二进制 | 最简单、离线可用；但技能跟 CLI 版本走，CLI 与库版本不一致时（摸底里就是 0.2.2 vs v0.3.0）技能会误导 |
| **B. 内容放在 tango 库模块里，`tan` 按项目 go.mod 的版本从模块缓存取** ✅ | 技能和代码同仓库同版本号，改 API 顺手改技能；`-local`/replace 自然生效；代价是模块未下载时需要先下载，而 `go build` 本来就要 |
| C. 从 GitHub 按标签下载 | 每次联网，且 B 已能精确对版本，无额外好处 |

用户选了 B。

## 5. 已定的做法

### 5.1 源文件位置（tango 仓库内）

- 技能源目录：`skills/tango/`（根模块内，随模块版本发布）。不用以 `.` 开头的目录，避免与仓库自身工具配置混淆；也**不**放进仓库自己的 `.agents/skills/`，否则会被框架贡献者的 AI 当成自己的技能。
- 结构（参考文件的拆分可由实现者微调，见 §7）：

```
skills/tango/
  SKILL.md                    # 入口：核心规则 + 能力速查 + 参考文件索引
  references/
    capabilities.md           # 需求 → 包/函数 全表，每项一段最小可编译代码
    coming-from-wails-electron.md  # 习惯写法 → Tango 实际写法 对照
    ui-native.md              # 自绘 UI 编程模型与组件要点
    webview-app.md            # WebView 应用骨架、绑定、事件、前端运行时
    pitfalls.md               # 编译通过但行为不对的陷阱
    verify.md                 # tan dev / tan mcp 验证流程与能力边界
    packaging.md              # tan build / sign / package、tan.json 要点
  examples/                   # 可选：完整示例，见 §6.4
```

### 5.2 安装位置（应用项目内）

- 写到项目的 `.agents/skills/tango/`。整个目录归 `tan` 管理，更新时**整体替换**。
- 用户自己的技能放在 `.agents/skills/` 下其他目录，`tan` 不碰。
- 来源版本**安装时由 tan 写入**（我的决定），不写死在源文件里，免得每次发版改字段：tan 用 `go list -m -f {{.Version}}` 取版本，写进 `SKILL.md` frontmatter 的 `metadata.tango-version`。replace/`-local` 时 Version 为空，写 `local`。`tan skills update` 判断"内容是否变化"时，比较的是本次要写入的最终内容（含版本字段）与现有文件，这样版本升级本身也算变化。

### 5.3 `tan init`

- 生成项目、跑完 `go mod tidy` 后，按 §5.5 的定位方式从所依赖的 tango 模块复制 `skills/tango/` 到 `.agents/skills/tango/`。
- 定位失败（离线、跳过 tidy、tidy 失败）**不让 init 失败**：给一条警告，提示之后运行 `tan skills update`。
- `-local` 时自然取本地仓库里的技能（go.mod 的 replace 指过去）。
- 所有模板都装。

### 5.4 `tan skills update`

- 新增子命令组 `tan skills`，第一期只有 `update`：在项目目录（含 `tan.json` 的目录，或用现有的目录参数约定）定位依赖的 tango 模块，用其中的 `skills/tango/` 替换 `.agents/skills/tango/`。
- 已存在且内容不同：先列出将被替换/删除的文件，然后替换（我的决定：不加交互确认，因为该目录声明归 tan 管理；输出里说明"自定义内容请放到其他目录"）。
- 输出：来源版本、目标路径、新增/修改/删除的文件数。
- 模块里没有 `skills/tango/`（项目依赖的 tango 版本早于本功能）：报错并说明需要升级 github.com/tangoui/tango。具体最低版本号要等发版才知道，本期文案不写死版本号。
- 模块未下载：提示先 `go mod download`（与 `tangoModuleDir` 的处理一致），不自动联网下载（我的决定，保持与现有行为一致；实现者若发现 `go list -m` 本身会触发下载，以实际行为为准并在 help 里写清）。

### 5.5 定位所依赖的 tango 模块

- 抽出/复用 `tangoModuleDir` 的核心逻辑：`go list -m -f {{.Dir}} github.com/tangoui/tango`，工作目录为项目目录。移动端那份额外检查 `mobile/` 目录，技能这边检查 `skills/tango/SKILL.md`。

## 6. 技能内容要求

### 6.1 SKILL.md（入口）

- `description` 要让 AI 在"用 Tango 写/改/调试/打包应用"时触发，写明关键词（Tango、tan、tan.json、github.com/tangoui/tango）。
- 正文控制在约 150 行以内，开头第一屏就是：
  1. **最重要的规则**：系统能力在 `sys/*`、`service/*` 子包，形如 `pkg.Func(app, …)`，不是 `App` 的方法；不确定就先 `go doc github.com/tangoui/tango/<pkg>`，不要凭 Wails/Electron 印象写。
  2. 能力速查表（一行一项：需求 → 包 → 关键函数），完整版指向 `references/capabilities.md`。
  3. 先判断项目路线（看 tan.json / 是否有 frontend），再读对应参考文件。
  4. 写完必须跑起来验证，指向 `references/verify.md`。
- 结尾是参考文件索引（每个一句话说明何时读）。

### 6.2 能力速查至少覆盖（以摸底实际用到/查到的为准，实现时逐项用 `go doc` 核对当前版本）

窗口（`*tango.Surface`、`SurfaceOptions`、显隐 `IsVisible`、`Bounds`/`OnMoved`/`OnResized`）、窗口状态记忆 `sys/windowstate.Track`、应用菜单 `sys/menu`（`menu.New`、`Submenu`、`Add(...).Accelerator`、`SetAppMenu`/`SetSurfaceMenu`、Edit 菜单 role）、托盘 `sys/tray`（`tray.New(app, Options{Icon: []byte…})`）、通知 `sys/notify`（`RequestPermission`、`Send`；macOS 只能在 .app 里）、对话框 `sys/dialog`（`Open(app, FileOptions{PickFolders})`、`ErrCanceled`）、数据目录 `service/paths.Data(appID)`、应用显隐/Dock `sys/appctl`、生命周期事件（`OnReady`、`OnEventKind(tango.EventReopen…)`、`QuitOnLastClose`）、Go→页面事件 `ipc.Router.Emit(surface, …)`（无广播，多窗口遍历 `app.Surfaces()`）、绑定方法取当前窗口 `ipc.ConnFrom(ctx)`、剪贴板、单实例等现有 `sys/*` 包。**框架没有的**也要写（如文件监听 → 用 fsnotify）。

### 6.3 陷阱清单至少包含（来自摸底）

- 改了菜单项（`SetLabel`、`SetChecked`…）后必须 `Menu.Update()`。
- 自绘 UI 动态列表必须用 `ui.Key` 包裹，否则删除后状态错位。
- 后台 goroutine 改界面状态用 `runner.Post`；定时刷新用 `c.WakeAt`；`app.Post` 不会重绘自绘界面。
- `uihost.Open` 返回 `(runner, surface, err)`。
- 组件参数顺序例外（如 `ui.Checkbox(c, label, &v)`）。
- WebView：`@tangoui/runtime` 版本必须与 go.mod 的 tango 一致；改了 Go 绑定方法签名要手动 `tan generate bindings`，`tan dev`/`tan build` 不会自动生成；直接 `go build` 前要先构建前端（embed 的是 dist）。
- macOS：通知只能在 .app 里测，且 .app 放在临时目录（如 /tmp）时系统会拒绝授权，要放到 ~/Applications 等常规位置（以 `fix/app-dev-baseline` 的结论和文档为准）；第一个 Submenu 是应用菜单；WebView 里 ⌘C/⌘V 需要 Edit 菜单 role；托盘图标宜用单色 Template。
- 模块缓存里 tango 根目录的 AGENTS.md 是写给框架贡献者的，写应用时不适用。

（`fix/app-dev-baseline` 修掉的问题，修复合入后从陷阱清单删掉或改写。）

### 6.4 示例

- 两个完整的小示例，各对应一条路线：自绘 UI（托盘 + 菜单 + 定时器 + 持久化）、WebView（绑定 + 事件 + 对话框 + 菜单 + 窗口状态）。可以从摸底产物（`/tmp/tango-baseline-a/todopomo`、`/tmp/tango-baseline-b/mdnotes`）整理精简而来，但要按 Tango 风格重写、去掉测试用的环境变量 hack。
- **示例和参考文件里的 Go 代码必须能编译**：加一个测试（放在 tango 仓库，CI 会跑），把示例作为可编译的包检查（例如示例放成 `skills/tango/examples/<name>/main.go` 并带构建标签或独立的检查方式，避免进入普通 `go build ./...`；具体手段由实现者定。若放进根模块作为普通包，要能通过 `GOOS=linux/windows go vet ./...`，且不能引入导出 API 影响 `apitxt -check`），防止 API 改了技能没跟上。参考文件里的片段至少要能对应到编译过的示例或 `go doc` 中存在的符号。

### 6.5 验证流程（verify.md）

- `tan dev` 热重载；`tan mcp`（stdio 配 `.mcp.json`，或 `-http` 加 curl）后台启动、截图、操作自绘 UI 语义树。
- 能力边界写清：原生菜单和托盘不在语义树里；WebView 窗口不能用 `tango_ui_key` 发按键（若 `fix/app-dev-baseline` 加了 `tango_web_key`，以合入后为准）；macOS 通知只能在 `tan build` 出的 .app 里测。
- 测试时用环境变量或参数把数据目录重定向，别写用户真实数据。

## 7. 留给实现者

- 参考文件的最终拆分和命名（§5.1 是建议）。
- 示例可编译检查的具体手段（构建标签、独立 go.mod、或测试里调用 `go build`）。
- `tan skills` 的参数细节（目录参数、`-h` 文案），遵循现有子命令约定；文档更新 `site/reference/cli.md` 和入门指南（中英文按现有文档覆盖范围）。
- `tan init` 在测试中如何模拟模块目录（参考现有 init 测试）。

## 8. 验收

1. `tan init -local <实现分支> -template ui` 和 `-template vue-ts` 生成的项目里有 `.agents/skills/tango/SKILL.md`，frontmatter 版本与 go.mod 一致。
2. `tan skills update` 在已有项目里能装上、能更新（改动一个技能文件后再跑会被还原，并列出变化），对旧版本 tango 报清楚的错。
3. 示例/片段可编译检查在 CI 中运行并通过。
4. **效果复测**：摸底的提示原文和日志已存档在 `/Users/admin/min/outputs/tango-app-skill-baseline/`（`PROMPTS.md`、`a-/b-BASELINE-LOG.md`）。复测用与摸底**相同的代理类型**（Claude Code general-purpose 子代理），提示保持原文，只把目录改成新的 `/tmp` 目录，并追加一句"项目里有 `.agents/skills/tango/`，开始前先读它"（我的决定：Claude Code 不会自动加载该目录，追加这一句是唯一差异，结果才可比）。项目用 `-local` 指向实现分支生成（发版前没有带技能的正式版本）。对比摸底日志：第一版凭印象写错的 API 数量应明显下降（目标：各自 ≤ 2 处），陷阱清单里的问题不再出现。复测日志存档到 `/Users/admin/min/outputs/`。
5. 推送前检查全部通过（不代表授权推送）。
