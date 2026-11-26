# Tango 应用开发技能 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 让 `tan init` 生成的项目自带 `.agents/skills/tango/` 技能，并新增 `tan skills update` 按项目 go.mod 所依赖的 tango 版本安装/更新它。

**Architecture:** 技能源文件放在 tango 根模块的 `skills/tango/`，随模块版本发布。`cmd/tan` 用 `go list -m` 在项目目录定位所依赖的 tango 模块（复用移动端 `tangoModuleDir` 的机制），把 `skills/tango/` 整体同步到项目的 `.agents/skills/tango/`，安装时把版本写进 SKILL.md frontmatter。

**Tech Stack:** Go（cmd/tan 独立模块）、Markdown（Agent Skills 标准 SKILL.md）。

**Spec:** `docs/superpowers/specs/2026-11-26-app-dev-skill-design.md`

## Global Constraints

- 技能安装目录固定为项目内 `.agents/skills/tango/`；不写 `.claude/skills/` 等其他目录。
- 技能源目录固定为 tango 根模块内 `skills/tango/`；**不要**放进本仓库自己的 `.agents/skills/`。
- 技能正文用英文；SKILL.md 正文约 150 行以内。
- 模块路径常量：`github.com/tangoui/tango`（修复分支已在 init.go 定义 `tangoModule`，复用它）。
- 版本写入 SKILL.md frontmatter 的 `metadata.tango-version`；`go list -m` 的 Version 为空（replace/`-local`）时写 `local`。
- `tan init` 安装技能失败不让 init 失败，只 `warnf` 并提示运行 `tan skills update`。
- 本次不发版、不推送、不合并到 main。提交作者 `tango <903300326@qq.com>`，提交信息不加 Co-Authored-By。
- 推送前检查（虽不推送也要跑）：根模块和 `cmd/tan` 的相关 `go test`、`gofmt -l .`（含构建标签文件）、`GOOS=linux go vet ./...`、`GOOS=windows go vet ./...`、`apitxt -check`。

## Review Focus

1. 项目里 `.agents/skills/` 下有用户自己的其他技能目录：更新只能动 `tango/`，兄弟目录原样保留。→ Task 1 `TestSyncSkillKeepsSiblings`。
2. 用户改过或新增了 `.agents/skills/tango/` 里的文件：更新后被还原/删除，且输出列出这些变化。→ Task 1 `TestSyncSkillRestoresAndPrunes`。
3. 连续运行两次 update：第二次报告"已是最新"，零变化，不重写文件。→ Task 1 `TestSyncSkillIdempotent`。
4. 项目依赖的 tango 版本没有 `skills/tango/`（例如 v0.3.0）：报清楚的错，提到升级 `github.com/tangoui/tango`，不留下半拉目录。→ Task 1 `TestSyncSkillMissingSource`。
5. 在不含 go.mod 的目录、或 go.mod 不依赖 tango 的目录运行 `tan skills update`：报错说明原因，不创建 `.agents/`。→ Task 3 `TestRunSkillsUpdateNotATangoProject`。

---

## 前置：分支基线

本分支 `feat/app-skill`（worktree `/Users/admin/min/.tango-app-skill`）目前只有设计文档提交 ae1e2762，基于 origin/main。开工前由派单会话把它 rebase 到 `fix/app-dev-baseline` 核对完成后的提交上（设计 §3）。实现者开工时先确认 `git log --oneline -8` 里能看到 `a52670a8 fix(tan): keep scaffolded Go and npm Tango versions aligned`，看不到就停下报告。

## 文件结构

- Create `cmd/tan/skills.go`：定位模块、同步技能、`tan skills` 子命令。
- Create `cmd/tan/skills_test.go`。
- Modify `cmd/tan/build_mobile.go`：`tangoModuleDir` 改为调用共用的定位函数。
- Modify `cmd/tan/init.go`：tidy 之后安装技能。
- Modify `cmd/tan/init_test.go`、`cmd/tan/main.go`（命令表）、`cmd/tan/cli_golden_test.go` 与 `cmd/tan/testdata/cli.golden`。
- Create `skills/tango/SKILL.md`、`skills/tango/references/*.md`、`skills/tango/examples/*/`。
- Modify `site/reference/cli.md`（及 `site/en` 对应页，如存在）、`site/guide/getting-started.md`（及英文版）。

---

### Task 1: 定位模块与同步技能（cmd/tan/skills.go 核心）

**Files:**
- Create: `cmd/tan/skills.go`
- Create: `cmd/tan/skills_test.go`
- Modify: `cmd/tan/build_mobile.go`（`tangoModuleDir`）

**Interfaces:**
- Produces:
  - `func locateTango(ctx context.Context, projectDir string) (dir, version string, err error)` — 在 projectDir 运行 `go list -m -f "{{.Dir}}\t{{.Version}}" github.com/tangoui/tango`；Dir 为空时返回错误 `module not downloaded (run go mod download)`（与现有文案一致）。
  - `type skillChanges struct{ Added, Modified, Removed []string }`（相对 `.agents/skills/tango/` 的斜杠路径，排序），方法 `func (c skillChanges) Empty() bool`。
  - `func syncSkill(moduleDir, version, projectDir string) (skillChanges, error)` — 把 `<moduleDir>/skills/tango/` 同步到 `<projectDir>/.agents/skills/tango/`。
  - `const skillDir = ".agents/skills/tango"`。
  - `tangoModuleDir` 保持签名不变，内部改用 `locateTango`，仍检查 `mobile/`。

- [ ] **Step 1: 写失败测试**（用 `t.TempDir()` 构造假模块目录 `<m>/skills/tango/{SKILL.md,references/a.md}` 和项目目录；SKILL.md 夹具含 frontmatter 行 `  tango-version: dev`）

```go
func TestSyncSkillFresh(t *testing.T)          // 新项目：Added == [SKILL.md references/a.md]，文件存在
func TestSyncSkillStampsVersion(t *testing.T)  // version "v0.4.0" → 安装后的 SKILL.md 含 "  tango-version: v0.4.0"；version "" → "  tango-version: local"；源文件不变
func TestSyncSkillRestoresAndPrunes(t *testing.T) // 改 references/a.md、新增 extra.md 后再同步：Modified == [references/a.md]，Removed == [extra.md]，内容与源一致
func TestSyncSkillKeepsSiblings(t *testing.T)  // 预先放 .agents/skills/mine/SKILL.md，同步后仍在且内容不变
func TestSyncSkillIdempotent(t *testing.T)     // 连续两次，第二次 Empty() == true，且文件 ModTime 不变
func TestSyncSkillMissingSource(t *testing.T)  // 模块无 skills/tango：返回错误，消息含 "github.com/tangoui/tango"；项目里不存在 .agents 目录
func TestSyncSkillRequiresVersionLine(t *testing.T) // 源 SKILL.md frontmatter 无 tango-version 行：返回错误
```

- [ ] **Step 2: 运行确认失败**

Run: `cd cmd/tan && go test -run 'TestSyncSkill' ./...`
Expected: FAIL（`syncSkill` 未定义）

- [ ] **Step 3: 实现 `locateTango`、`syncSkill`、`skillChanges`**

版本写入：只替换 SKILL.md 开头 `---` 与下一个 `---` 之间第一处匹配 `^\s*tango-version:` 的行，保留原缩进。变化比较用"将要写入的最终字节"对比现有文件，相同则不写（保证 ModTime 不变）。先在内存里算好全部目标内容再写盘；源缺失时在创建任何目录前返回错误。

- [ ] **Step 4: 运行确认通过，并跑移动端相关测试确认 `tangoModuleDir` 没坏**

Run: `cd cmd/tan && go test ./...`
Expected: PASS

- [ ] **Step 5: 提交** `feat(tan): locate the project's Tango module and sync its skill`

---

### Task 2: 技能内容 `skills/tango/`

**Files:**
- Create: `skills/tango/SKILL.md`
- Create: `skills/tango/references/capabilities.md`、`coming-from-wails-electron.md`、`ui-native.md`、`webview-app.md`、`pitfalls.md`、`verify.md`、`packaging.md`（拆分可微调，SKILL.md 索引要与实际文件一致）

**Interfaces:**
- Produces: `skills/tango/SKILL.md` frontmatter 固定为下面的形状（Task 1 的版本写入依赖 `tango-version` 行）：

```yaml
---
name: tango
description: <英文；写明在用 Tango（github.com/tangoui/tango、tan CLI、tan.json）创建、修改、调试、验证或打包桌面/移动应用时使用>
metadata:
  tango-version: dev
---
```

- [ ] **Step 1: 收集素材**：读设计 §6 和两份摸底日志 `/Users/admin/min/outputs/tango-app-skill-baseline/{a,b}-BASELINE-LOG.md`（§1.1/§2 的 API 对照表、陷阱、文档缺口），以及 `fix/app-dev-baseline` 新增的文档改动（sys/notify、sys/menu、ipc、ui/doc.go）。
- [ ] **Step 2: 写 SKILL.md**：首屏依次是设计 §6.1 的四点（核心规则、能力速查表、判断路线、必须验证），结尾是参考文件索引（每个一句"何时读"）。
- [ ] **Step 3: 写 references/**：覆盖设计 §6.2 全部能力（含"框架没有的"）、§6.3 全部陷阱、§6.5 验证流程。`fix/app-dev-baseline` 已修掉的问题（模板 runtime 版本）不再当陷阱写，改成"保持 go.mod 与 @tangoui/runtime 版本一致"的规则；macOS 通知按修复分支 `site/guide/system/notify.md` 的结论写。`tan mcp` 能力以当前代码为准（修复分支未加 `tango_web_key` 就写"不能"）。
- [ ] **Step 4: 逐项核对符号**：对技能里出现的每个 Go 符号运行 `go doc github.com/tangoui/tango/<pkg> <Symbol>`（在本 worktree 根目录），对 JS 符号对照 `runtime-js` 源码；把核对清单（符号 → 结果）贴到本任务的提交说明正文里。任何不存在的符号必须改掉。
- [ ] **Step 5: 提交** `docs(skills): add the tango app-development skill`

---

### Task 3: 可编译示例

**Files:**
- Create: `skills/tango/examples/native-tray-timer/main.go`（自绘 UI：托盘 + 菜单 + 定时器 + 持久化）
- Create: `skills/tango/examples/webview-notes/`（WebView：绑定 + 事件 + 对话框 + 菜单 + 窗口状态；前端用内联/嵌入的最小静态页，不依赖 npm 构建，保证 `go build` 直接可编译）
- Modify: `skills/tango/SKILL.md`、相关 references（指向示例，片段与示例保持一致）

**Interfaces:**
- Consumes: Task 2 的 references 内容。
- Produces: 两个 `package main`，属于根模块，随 CI 的 `go build ./...` / `go vet ./...` 编译。

- [ ] **Step 1: 写示例**：可参考摸底产物 `/tmp/tango-baseline-a/todopomo`、`/tmp/tango-baseline-b/mdnotes`，按 Tango 风格精简重写，去掉测试用环境变量（如 `MDNOTES_CONFIG_DIR`）。动态列表用 `ui.Key`；菜单改动后 `Update()`；后台改状态用 `runner.Post`。
- [ ] **Step 2: 编译与跨平台 vet**

Run: `go build ./skills/... && GOOS=linux go vet ./skills/... && GOOS=windows go vet ./skills/...`
Expected: 无输出、退出码 0

- [ ] **Step 3: 确认不影响 API 文本**

Run: `apitxt -check`（按仓库现有用法）
Expected: 通过

- [ ] **Step 4: 实际运行一次各示例并截图**（用 `tan mcp` 后台启动；不抢前台；截图放 `/Users/admin/min/outputs/tango-app-skill/examples/`），确认能启动、界面正常。
- [ ] **Step 5: 提交** `docs(skills): add compiled examples for native and WebView apps`

---

### Task 4: `tan skills update` 命令

**Files:**
- Modify: `cmd/tan/skills.go`（加 `runSkills`）
- Modify: `cmd/tan/main.go`（命令表加 `{"skills", "install or update the project's AI skill", runSkills}`）
- Modify: `cmd/tan/cli_golden_test.go`、`cmd/tan/testdata/cli.golden`
- Test: `cmd/tan/skills_test.go`

**Interfaces:**
- Consumes: Task 1 的 `locateTango`、`syncSkill`、`skillChanges`、`skillDir`。
- Produces: `func runSkills(args []string) error`；用法 `tan skills update [-C dir]`（目录参数用现有 `projectFlag(fs)`，即 `-C`）。

- [ ] **Step 1: 写失败测试**（测试项目的 go.mod 用 `replace github.com/tangoui/tango => <fixture module>` 指向临时假模块，离线可用）

```go
func TestRunSkillsUpdate(t *testing.T)                // 安装成功；输出含 "local" 与 ".agents/skills/tango"；再改一个文件后重跑，输出列出 modified
func TestRunSkillsUpdateUpToDate(t *testing.T)        // 第二次运行输出含 "up to date"
func TestRunSkillsUpdateNotATangoProject(t *testing.T) // 无 go.mod / go.mod 不依赖 tango：返回错误，目录下不出现 .agents
func TestRunSkillsUsage(t *testing.T)                 // 无参数或未知子命令：返回错误并打印 "Usage: tan skills update"
```

- [ ] **Step 2: 运行确认失败**：`cd cmd/tan && go test -run 'TestRunSkills' ./...`，Expected: FAIL
- [ ] **Step 3: 实现 `runSkills`**：输出格式——第一行 `tango skill <version> -> <rel path>`，随后 `added N, modified N, removed N` 并逐个列出文件；无变化时输出 `already up to date`（测试断言用 "up to date"）。替换前不交互确认；输出末尾一行说明 `.agents/skills/tango is managed by tan; put your own skills in another directory`。
- [ ] **Step 4: 更新 golden**：在 `cli_golden_test.go` 加 `skills update -h` 的捕获，按仓库现有方式重新生成 `testdata/cli.golden`。
- [ ] **Step 5: 运行** `cd cmd/tan && go test ./...`，Expected: PASS
- [ ] **Step 6: 提交** `feat(tan): add tan skills update`

---

### Task 5: `tan init` 安装技能

**Files:**
- Modify: `cmd/tan/init.go`（`createProject` 末尾，tidy 之后）
- Test: `cmd/tan/init_test.go`

**Interfaces:**
- Consumes: Task 1 的 `locateTango`、`syncSkill`；Task 2 的真实 `skills/tango/`（`-local` 指向本仓库根目录时使用）。

- [ ] **Step 1: 写失败测试**

```go
func TestCreateProjectInstallsSkill(t *testing.T)      // -local 指向仓库根（参考现有 init 测试取仓库根的方式），Tidy=false：生成目录含 .agents/skills/tango/SKILL.md，且含 "tango-version: local"；ui 与 vue-ts 两个模板都测
func TestCreateProjectSkillFailureIsWarning(t *testing.T) // 定位失败（非 -local、Tidy=false、模块不在缓存——用临时 GOMODCACHE/GOFLAGS=-mod=mod 等手段隔离）：createProject 返回 nil，stderr 警告（`warnf` 写 os.Stderr，测试里重定向捕获）含 "tan skills update"
```

- [ ] **Step 2: 运行确认失败**：`cd cmd/tan && go test -run 'TestCreateProject' ./...`，Expected: 新测试 FAIL
- [ ] **Step 3: 实现**：tidy 之后调用 `locateTango(ctx, dir)` + `syncSkill`；任何错误 → `warnf("could not install the Tango AI skill: %v\nrun `tan skills update` in %s later", err, dir)`，继续返回 nil。
- [ ] **Step 4: 运行全部 cmd/tan 测试**：`cd cmd/tan && go test ./...`，Expected: PASS
- [ ] **Step 5: 端到端**：`tan`（用本 worktree 构建：`cd cmd/tan && go build -o /tmp/tan-skill ./`）执行 `/tmp/tan-skill init -local /Users/admin/min/.tango-app-skill -template ui /tmp/tango-skill-e2e/ui` 和 `-template vue-ts .../vue`，确认两个项目都有技能、`go build` 通过；在其中一个项目里改坏一个技能文件后运行 `/tmp/tan-skill skills update`，确认被还原。
- [ ] **Step 6: 提交** `feat(tan): install the Tango AI skill in new projects`

---

### Task 6: 文档

**Files:**
- Modify: `site/reference/cli.md`（加 `tan skills update`；若 `_cli-flags.md` 由生成器产出则按生成方式更新）及其英文版（`site/en/...` 如存在）
- Modify: `site/guide/getting-started.md` 及英文版：一段说明"项目里的 `.agents/skills/tango/` 是给 AI 助手的技能，Codex/Cursor/Gemini CLI 自动读取；Claude Code 需要在对话里让它读该目录；升级 tango 后运行 `tan skills update`"。
- Modify: `docs/release.md`：发版清单加一条"确认 `skills/tango/` 内容与本版 API 一致"。

- [ ] **Step 1: 改文档**
- [ ] **Step 2: 若站点有构建/链接检查，按仓库现有方式运行**（例如 `cd site && npm run build`，以 package.json 为准），Expected: 通过
- [ ] **Step 3: 提交** `docs: document the project AI skill and tan skills update`

---

### Task 7: 全量检查与交付（实现者）

- [ ] **Step 1:** 根目录：`go test ./...`（或仓库惯用的范围）、`gofmt -l .`（应无输出）、`GOOS=linux go vet ./...`、`GOOS=windows go vet ./...`、`apitxt -check`；`cmd/tan`：`go test ./...`、`gofmt -l .`、两个 GOOS 的 vet。
- [ ] **Step 2:** `git status --porcelain --ignored -uall` 确认没有遗留的临时文件；工作区干净。
- [ ] **Step 3:** 最终回复：每个任务的提交哈希、检查命令和结果、Task 2 的符号核对结论、Task 3 截图路径、未做或存疑的部分。

### Task 8: 效果复测（派单会话执行，不由实现者做）

用 `/Users/admin/min/outputs/tango-app-skill-baseline/PROMPTS.md` 的两份提示原文，各派一个 Claude Code general-purpose 子代理；目录改成 `/tmp/tango-retest-a`、`/tmp/tango-retest-b`，项目用本分支构建的 tan 加 `-local` 生成；提示只追加一句"项目里有 `.agents/skills/tango/`，开始前先读它"。对比摸底：第一版凭印象写错的 API 各 ≤ 2 处，陷阱清单问题不再出现。日志存档到 `/Users/admin/min/outputs/tango-app-skill-retest/`。不达标则回到 Task 2/3 补技能再测。
