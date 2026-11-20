# macOS 独立 UI 线程分阶段合入 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 准备默认关闭独立 UI 线程的合并候选，保留实现和内部测试。

**Architecture:** 在现有 `uihost` 请求入口关闭默认线程化；保留 Darwin 平台能力、显式内部测试适配器与共享线程安全改动。本轮只做第一阶段，不宣布 Task 11 通过。

**Tech Stack:** Go、Darwin AppKit、后台回归测试。

**Spec:** `docs/superpowers/specs/2026-11-20-ui-thread-staged-rollout-design.md`

## Global Constraints

- 只改 `/tmp/tango-uithread-design` 的 `docs/ui-thread-design`，保留其他工作区、台账、原始测量数据和 `go.work`。
- 不新增对外配置、环境变量或 API 开关。`Attach`、`tan dev` 和其他平台的线程模型保持原样。
- 不推送或合入 `main`；只允许必要的本地候选提交，作者 `tango <903300326@qq.com>`，无 AI 署名或 trailer。同步方向只能是 `origin/main` → 候选分支，不改写已有历史。
- `GOTOOLCHAIN=go1.27.3 GOWORK=off`，Go 命令 `nice -n 19`、`-p 1`；macOS race 用 `CGO_ENABLED=1`。不抢前台，不开窗口，不重跑计时。

## Review Focus

- 普通 `Open` 走旧线程模型，仍能从后台通过 `Runner.Post` 投递。
- 内部显式线程化及其关闭、回退和线程检查测试仍可用，不能因默认关闭而被整体跳过。
- `Attach` 与 `tan dev` 不误入独立线程；核对已有测试的断言。
- 共享生命周期及与最新主线的交叉改动仍工作；不能通过关路径掩盖共享回归。
- 中英文公开文档和 CHANGELOG 不宣称已默认启用或已通过 Task 11。

### Task 1：构建第一阶段合并候选

**范围：** `uihost/uihost.go`、`uihost/threaded_sink_internal_test.go`、必要回归测试、`internal/platform/darwin/canvas_threaded.go` 的适配器说明、`CHANGELOG.md`、`docs/` 和 `site/` 中与默认线程模型直接相关的文档；合并主线冲突如超出该范围且需要改变语义，停下报告。

- [ ] 核对分支状态，记录 `git fetch origin` 后的 `origin/main` SHA，合并到独立候选分支。保留这份设计和计划，不删除或覆盖其他未提交内容。若遇不明确的并发或渲染冲突，报告，不猜。
- [ ] 将 `TestHostWantsUIThread` 改成 `TestHostDoesNotRequestUIThreadByDefault`，通过 `platform.CanvasThreadedSink` 断言 `WantsUIThread()` 为 false。跑 `go test -p 1 -run '^TestHostDoesNotRequestUIThreadByDefault$' ./uihost`，确认现状因仍请求线程化而失败。
- [ ] 在 `uihost/uihost.go` 将 `(*host).WantsUIThread() bool` 最小改为返回 false，保留 `darwin.ThreadedCanvas`、已迁移的示例和共享修复。复跑定向测试，并核对旧路径的 Post 回归、内部门控测试的显式适配器、Attach 和 devhost 边界；编译但不执行需要 GUI 的测试。
- [ ] 核对并修订公开文档、包注释、中英文站点和 CHANGELOG：默认运行在主线程，独立线程仅为内部验证路径，`Runner.Post` 仍适用；默认开启和 Task 11 留待第二阶段。已归档原始验收不改写。
- [ ] 跑受影响包的功能与竞态测试、`go test -p 1 -short ./...`、Darwin/Linux/Windows `go vet -p 1 ./...`、gofmt、`git diff --check` 和 `go run -p 1 ./internal/cmd/apitxt -check`；检查 `cmd/tan` 若涉及集成，说明工具链、基线失败、跳过项及门控测试仅编译的限制。通过后只做本地候选提交并报告 SHA、完整命令/输出、范围和剩余阻塞项。

合并候选返回后，由主会话核对差异、复跑重点回归并安排最终审查。本计划不授权把候选合入 `main` 或推送。
