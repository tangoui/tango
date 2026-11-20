# tan dev 可选并行启动 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将已测得热启动收益的并行准备方式加入桌面 `tan dev`，仅供项目显式开启。

**Architecture:** 串行路径不变；并行路径以有取消能力的就绪关口把前端 URL 与首次应用启动连接起来。继续使用原有 Go 构建、签名和热重载路径。

**Tech Stack:** Go、`cmd/tan`、项目 JSON Schema、MinCode WebView 自动化。

**Spec:** `docs/superpowers/specs/2026-11-20-tan-dev-parallel-start-design.md`

## Global Constraints

- 仅当桌面 `tan dev` 自己负责启动 `frontend.dev` 时生效；未设置或为 `false` 仍走原串行路径。
- `-url`、`frontend.devURL`、静态前端、自绘无前端、移动端、发布构建都保持原行为。
- 不引入构建产物复用，也不改构建次数或标签。
- 用基于 `origin/main` 的独立分支，避开已有构建复用变更。不提交、合并、推送或安装全局 `tan`，除非另有授权。

## Review Focus

- 单一启动状态区分 URL 已收到、前端已退出、用户取消；URL 到达只停止 90 秒计时，不停止退出监听。应用或宿主启动前最后复查退出与取消，确保已观测到退出时不启动。
- Go 构建失败后重试不可吞掉前端失败或文件变更。
- 热重载能力探测和普通回退只能共享就绪结果，不共享未启用的构建产物复用逻辑。
- 上下文取消时，不留下等待 URL 的 goroutine 或仍在构建的子进程。
- 前端 URL 和应用环境不在两条执行路径间无同步地读写。

### Task 1: 配置、调度与兼容性

**Files:** `cmd/tan/config.go`、`cmd/tan/tan.schema.json`、`cmd/tan/dev.go`、`cmd/tan/dev_hot.go`、相关测试；中英文 `site/*/reference/tan-json.md` 和 `site/reference/tan-json.md`。

**Interfaces:** `Frontend.ParallelDevBuild bool`，JSON `parallelDevBuild`；内部就绪关口 `waitFrontend(ctx context.Context) (string, error)`（可封装在私有会话类型中），可跨热重载探测和普通回退传递，应用进程构造前由拥有者设置环境。

- [ ] 写 `TestDevParallelBuildStartsBeforeFrontendURL`：前端替身在见到构建信号后才报 URL，显式开启时应启动，且应用不得早于构建和 URL 就绪；默认串行用独立夹具断言继续先报 URL。记下前者的 RED 证据。
- [ ] 补 `TestDevParallelFrontendExitBeforeLaunch`：用通道控制前端报 URL 后退出、Go 构建尚未结束；断言取消构建、返回前端退出原因，且未启动应用。再覆盖 URL 和退出都已可读的情况；避免用固定 sleep 制造时序。
- [ ] 补 `TestDevParallelStartupCancellation`：安装失败不构建；前端未报 URL 超时、用户取消均不启动旧产物，并用启动子进程的构建替身验证构建进程树已退出。Windows 上不能先杀构建父进程再用 `taskkill /T` 查树；交叉 vet 不替代 Windows 实机进程树验证。
- [ ] 补 `TestDevParallelBuildRetry`：首构建失败但前端存活时，编辑 Go 文件可重试成功；若等待编辑期间前端退出，直接报前端错误，无需再改文件。另覆盖热路径及非热回退共享同一 URL 结果、显式外部 URL 不启动前端命令。
- [ ] 接入配置和并行关口。保留串行分支；并行模式以启动上下文驱动构建及重试等待，前端异常须保留诊断而不能被当成普通用户取消。构建用可回收进程树的路径，不能只依赖 `exec.CommandContext` 杀父进程。普通启动与热重载宿主创建前复查同一前端状态，首次启动结束或退出时收齐监听；不要把短生命周期启动上下文用于已启动的长期运行进程。
- [ ] 同步 Schema 和中英文说明，包括生成文件的限制；用配置解析测试覆盖缺省、`false`、`true`。
- [ ] 复跑回归及 `cd cmd/tan && go test ./... -count=1 && go vet ./...`，Linux/Windows 交叉 vet，相关 race 测试与仓库 `apitxt -check`。独立只读审查失败/取消、数据竞争和生命周期；核对测试证据。

### Task 2: MinCode 验收与开启

- [ ] 使用同一 `tan` 二进制、隔离 MinCode 源、同一 Go 工具链及标签，在夹具 `tan.json` 交替开启/关闭；以原 `pnpm dev` 做一次开启/关闭配对验证，使用已就绪的包管理器环境避免下载；另用直接 Vite 对照分离包管理器等待，结果分开报告。分别记录启动到主界面挂载及首次 IPC 成功，核对 Go 构建次数和进程退出。
- [ ] 协调机位串行跑至少五对热启动及两个独立空输出/缓存冷启动对照，不抢前台。记录中位数、配对差值及失败，不把受扰或超时轮次算成收益。
- [ ] 验证通过后，协调 MinCode `tan.json` 的文件归属与新 CLI 的可用性；只在兼容的开发环境显式开启，避免旧版 CLI 读取未知字段。保持项目和框架改动分别可审查。
