# tan dev 首次构建产物复用 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 桌面 `tan dev` 首次判定应用不支持热重载时，只执行一次成功的 `go build` 就能启动应用。

**Architecture:** 热重载入口只在首次能力探测失败时，把本轮成功产物的路径随 `errNotHot` 交给普通会话。普通会话先安置该产物，再和普通构建共用打包、签名和启动流程。

**Tech Stack:** Go、`cmd/tan`、现有进程测试夹具、MinCode 后台 WebView 自动化。

**Spec:** `docs/superpowers/specs/2026-11-20-tan-dev-build-reuse-design.md`

## Global Constraints

- 不改 CLI 参数、构建标签、前端启动顺序、发布构建或移动端，也不跨会话缓存产物。
- 保留稳定应用路径、macOS `.app` 和签名行为；`-hot=false`、热重载中途回退和后续文件变更重编维持原行为。
- 只使用当前会话刚成功构建的产物；失败或取消不能启动旧程序。普通重编失败仍保留正在运行的应用。
- 使用隔离分支；不提交、不合并、不推送，除非另有授权。编译与测量避开其他会话的前台时段。

## Review Focus

- 热重载运行途中返回 `errNotHot`：不得误把它解释成可复用的首次产物。
- 同一运行前已有旧普通二进制：交接或签名失败后不得执行它。
- 首次构建期间源文件有变动：交接须保留原文件监听快照或检测该次变动并重建，不能吞掉编辑。
- 旧目标已存在或交接失败：保持完整文件，不在目标位置写半成品；交接失败才允许回到普通构建。
- 信号取消恰好发生在交接后：不启动，也不再执行构建；清理本轮临时文件。

---

### Task 1: 首次产物交接与普通启动

**Files:** `cmd/tan/dev.go`、`cmd/tan/dev_hot.go`、`cmd/tan/dev_test.go`、`cmd/tan/dev_hot_test.go`。

**Interfaces:** 保持 `(*devSession).hotLoop(ctx context.Context, watchFramework bool) error`；增加私有 `initialHotFallback` 错误（含本轮产物路径，通过 `Unwrap() error` 保持 `errors.Is(err, errNotHot)`）；将普通循环改为 `(*devSession).loop(ctx context.Context, initialBinary string) error`；拆出 `(*devSession).prepareApp(ctx context.Context) error`。首次监听基准用同一次构建前的 watcher；如果交接前检测到变化则不复用，按现有普通路径重建。

- [ ] **Step 1：先写回归测试。** `TestDevInitialHotFallbackBuildsOnce` 用现有进程测试风格和临时项目运行默认桌面 dev 路径，记录 `go build` 调用，断言 `len(builds) == 1`，应用确实执行且使用 `devBinary` 对应的路径。`TestDevInitialHotFallbackAfterEdit` 断言构建期间修改 Go 源不会遗漏。`TestDevInitialHotFallbackFailureKeepsOldAppStopped` 覆盖交接失败后普通构建也失败、签名失败与取消：断言未执行旧目标；取消不产生后续构建。另断言 `-hot=false` 不探测、支持热重载不走普通路径、首次之后的裸 `errNotHot` 不携带产物，以及后续重编不复用。
- [ ] **Step 2：验证 RED。** `cd cmd/tan && go test -run '^TestDevInitialHotFallback' -count=1 -v`。先跑能直接驱动现有接口的核心测试：预期因记录到两个 `go build` 失败，不以编译错误代替复现。
- [ ] **Step 3：实现。** 只包装 `hotSession.loop` 首次 `!hotCapable(path)` 分支；`dev` 用 `errors.As` 取出路径。普通循环只消费一次首次路径，调用现有 `installBinary` 的同目录替换模式；如需加强临时文件清理，保持已有热重载调用语义。`prepareApp` 为两个普通启动分支共用；仅在成功写好 bundle 后设置 `bundled`，签名失败不进入 `startProcess`。交接失败记录原因并走现有普通构建；启动前检查 `ctx.Err()`。不改变运行中 `rebuild` 的返回值或停止顺序。
- [ ] **Step 4：验证 GREEN。** 原回归命令转绿；`gofmt -l`、`git diff --check`、`cd cmd/tan && go test ./... -count=1 && go vet ./...`；分别用 `GOOS=linux`、`GOOS=windows` 执行 `go vet ./...`，再运行仓库 API 清单检查。报告真实失败，不以交叉 vet 当实机证据。

### Task 2: MinCode 冷热 A/B

**Interfaces:** 仅改变旧、 新 `tan` 可执行文件；复用同一隔离 MinCode 夹具和同一 Tango 依赖树、构建标签、身份配置；测试脚本与原始日志留在本地采样目录。

- [ ] **Step 1：准备同源 A/B。** 用同一个 Go 工具链从基线和 Task 1 结果各构建一份 `tan`，记录实际提交/补丁与版本。不清共享缓存，不软链生产 `node_modules`。
- [ ] **Step 2：冷、热分别交替运行。** 冷组每样本独立空 Go/Vite 缓存且依赖已下载；热组各先预热，再交替采样。至少 3 对冷、5 对热；机器时段协调后串行采样，后台窗口、两组同焦点条件。记录失败和受扰样本，不将启动失败计为变快。
- [ ] **Step 3：验收。** 比较实际 `go build` 计数、Vite URL 就绪、构建时长、签名、进程启动、主界面 DOM + 成功 `get_app_version` 往返的总时间。预期改后默认非热重载回退 1 次构建；性能报告列原始样本、中位数和范围，不把历史 11.58 秒当实际收益。
