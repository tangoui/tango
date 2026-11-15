# 编辑器 M2 性能验收实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在 `origin/main` `b2de59a` 基线上复测编辑器 M2 性能门禁，根据证据只做必要的 `ui/editor` 定点优化，并用新数据完成验收报告。

**Architecture:** 先固定当前工作区和基准输入，使用已有 `ui/editor/perfab` 成对驱动及独立 benchmark 复测；只有当 CPU/profile 或稳定计数证据把超限归因到 `ui/editor`，或已独立审查的 F/P Metal 候选明确改善同一 Wait 口径下的 GPU copy 路径时，才新增回归测试并做最小修复。最终重新测量并更新报告；其他 GPU/宿主帧调度瓶颈不跨边界处理。

**Tech Stack:** Go 1.27、`testing` benchmark、darwin/arm64 GPU 离屏测试、`ui/editor/perfab/{arms.sh,run.sh,summarize.py}`、pprof；不新增 Go/npm 依赖。

**Spec:** `docs/superpowers/specs/2026-11-15-editor-m2-perf-acceptance-design.md`

## Global Constraints

- 正式基线是当前 `origin/main` 的 `b2de59a`；计划 Tasks 1–9 视为已有实现，本轮不重新实现这些功能。
- 正式成对测量避开其他会话的高负载工作，负载必须低于 6；无效轮次不得用于产品判定。
- 性能门槛保持原值：百万行输入 p95 ≤ 16 ms；连续滚动平均 ≥ 60 fps、p99 ≤ 33 ms；100 MB 打开 ≤ 1 s；TS 2 万行后台高亮 ≤ 2 s；新增显示能力成对退化 ≤ 1.10；Monaco 更好时按 Monaco 同场景门槛。
- 不关闭默认能力、不缩小视口、不跳过正确性检查，不改变测试口径或内存口径。
- 产品代码改动限 `ui/editor/**`；本轮用户批准的 F fence 适配和独立审查通过的 P retained-copy 候选可额外修改 `internal/render/gpu/metal/**`，并必须通过 Metal 包测试、race 测试和编辑器回归；报告改动限 `docs/specs/code-editor-m2-report.md`；不得修改其他 `internal/render/**`、`ui/` 框架、`ui/text/**` 或平台宿主。
- 不回退或格式化其他会话的工作；不启动会干扰 `tango-mem` 的 GUI/内存基准。
- `go test -race ./ui/editor/...` 全量若只因并行减速触发墙钟预算失败，但单独预算测试通过且无数据竞争，按已知环境限制记录，不扩大测试基础设施。
- 不执行 `git add`、`commit`、`stash`、`reset`、`checkout`、`clean` 或其他会破坏未提交工作的 Git 写操作；提交由用户另行决定。

## Review Focus

- **长单行中段编辑、64 KiB 粘贴和整段删除**：换行结果必须与完整重算相同，扫描/每键工作不能随 1 MB 行长线性增长；由 Task 2 的既有 `TestWrapIncrementalMatchesFull`、`TestWrapIncrementalBounded`、`TestLongLineKeystrokeWork` 和新回归计数测试覆盖。
- **CPU 工作与 GPU 等待的区分**：同一输入 p95 超限时不能把提交后的 GPU 等待误优化成编辑器代码；由 Task 1 的 cpu-ms/gpuwait 分项数据与 CPU profile 记录覆盖。
- **默认能力与关闭能力的语义**：`TANGO_EDITOR_PERF_OFF` 只能改变对应能力，`all` 才关闭全部，未知项必须失败；由 Task 1 的 `TestPerfApplyOff` 和成对逐项测量覆盖。
- **高负载和进程间噪声**：负载 ≥ 6 的轮次必须作废，A/A 控制臂用于识别噪声，不能拿单轮最好数字判定；由 Task 1 的驱动日志检查和 Task 3 的最终数据审查覆盖。
- **race 下的预算测试**：并行减速不能被误报成数据竞争，单独运行仍须通过；由 Task 4 的单独测试、race 输出分类和报告记录覆盖。

---

### Task 1: 固定基线并完成第一次性能复测

**Files:**
- Read/Run: `ui/editor/perf_test.go`
- Read/Run: `ui/editor/perf_task10_test.go`
- Read/Run: `ui/editor/perf_wrap_test.go`
- Run: `ui/editor/perfab/arms.sh`
- Run: `ui/editor/perfab/run.sh`
- Output (uncommitted evidence): `.superpowers/sdd/2026-11-15-editor-m2-perf-acceptance/`

**Interfaces:**
- Consumes: 当前 `origin/main` `b2de59a`、已有 `TANGO_EDITOR_PERF_OFF` 开关、既有生成文件和 benchmark 名称。
- Produces: 可复现的机器/提交元数据、每轮负载、长行 p50/p95、输入 p95/cpu-ms/gpuwait、滚动 fps/p99、打开 open-ms、TS20k、内存和 A/A 控制数据；无效轮次及作废原因。

- [ ] **Step 1: 检查基线和资源占用**：确认工作区干净、`git rev-parse HEAD` 为 `b2de59a` 或等价当前 `origin/main`，记录 `go version`、`go env GOOS GOARCH`、机器信息和测试二进制哈希；确认 `tango-mem`、down-poll、文档构建等高负载任务没有占用 GUI 性能时段。
- [ ] **Step 2: 运行编辑器正确性前置检查**：运行 `go test ./ui/editor/... -count=1`，记录通过/失败；失败时停止性能判定，先报告失败，不修改测试口径。
- [ ] **Step 3: 运行已有独立基准**：执行长行换行、长行工作、百万行输入、滚动（全量/局部）、100 MB 打开、TS20k 后台高亮和内存 benchmark；每类至少满足现有计划规定的有效轮次，输出保存在本轮证据目录。
- [ ] **Step 4: 运行成对归因测量**：按 `arms.sh`/`run.sh` 当前接口执行 M1→当前默认、当前默认→`TANGO_EDITOR_PERF_OFF=all`、逐项关闭能力三组；每组采用 ABAB 交替、新进程、有效负载 < 6，保留中位数、A/A 和 95% CI。
- [ ] **Step 5: 运行 Monaco 对照（若本机工具可用）**：使用已有 `tools/monaco-bench` 同场景记录滚动、输入、打开和 TS20k 数据；如果 Edge/Monaco 当前不可用，只记录阻塞原因，不伪造对照数据。
- [ ] **Step 6: 审查第一次结果**：按“长行 p95 > 16 ms”“默认/M1 或默认/关闭项 > 1.10”“绝对门槛失败”“Monaco 更好”分类；把 GPU wait 占比、A/A 噪声和无效轮次写入 evidence summary。若全部通过且 profile 没有稳定热点，跳到 Task 3。

### Task 2: 针对已证实的 `ui/editor` 热点增加回归测试并优化

**Files:**
- Modify: 由 Task 1 证据确定的 `ui/editor/**` 文件，或用户已批准且独立审查通过的 `internal/render/gpu/metal/**` F/P 候选文件；优先 `wrap.go`、`viewlines.go`、`longwork.go`、`paint.go`、`minimap.go`、`occurrences.go` 或对应实际热点文件
- Test: 与热点同包的现有测试文件；必要时新建一个聚焦的 `ui/editor/*_perf_regression_test.go` 或 Metal 候选回归测试
- Read/Profile: Task 1 生成的 `cpu.pprof`、alloc profile 和 benchmark 输出

**Interfaces:**
- Consumes: Task 1 的超限清单、CPU/gpuwait 分解、profile 和 A/A 证据。
- Produces: 每个修复都有稳定的计数/分配/正确性回归测试；默认行为不变；代码只降低已证实的编辑器工作量。

- [ ] **Step 1: 为每一个拟修复热点写失败的计数型测试**：优先断言扫描 rune 数、每帧重复计算次数、分配次数或局部重绘次数；长行继续要求 `rewrapEdit` 与 `computeBreaks` 完全一致，不能只用墙钟断言。
- [ ] **Step 2: 单独运行新测试确认失败或复现超限**：使用 `go test ./ui/editor -run '<focused test>' -count=1` 或对应 benchmark；如果测试并未稳定复现，撤销该优化方向并回到 profile，不修改测试门槛。
- [ ] **Step 3: 实现最小修复**：优先消除帧内重复取行/分词/装饰、每帧分配和不必要整帧重绘；保持现有大文件降级、分词暂停保护、minimap grab/release 过渡和正确性语义。
- [ ] **Step 4: 运行聚焦回归**：执行新增测试、相关既有测试、`TestViewDamageVerified`（若触及渲染路径）和严格导航测试；失败时先修实现，不放宽断言。
- [ ] **Step 5: 运行局部 benchmark/profile**：确认 CPU 工作量或分项指标确实改善，且 A/A 噪声没有被误认为收益；若热点属于 GPU wait、渲染器或宿主，停止本任务并记录“不在本轮边界”。

> 如果 Task 1 没有稳定的 `ui/editor` 超限热点，本任务无代码修改，直接标记为不需要执行并进入 Task 3。

### Task 3: 重新跑完整门禁并判定验收结果

**Files:**
- Run: Task 1 的所有 benchmark 和成对驱动
- Read/Compare: `.superpowers/sdd/2026-11-15-editor-m2-perf-acceptance/`
- Output (uncommitted evidence): 同一证据目录的 final 子目录

**Interfaces:**
- Consumes: Task 2 的代码和回归测试，或 Task 1 确认的“无需代码修改”结论。
- Produces: 与第一次测量完全同口径的最终数据表、超限/通过清单和是否满足自动化验收的判定。

- [ ] **Step 1: 在安静时段重复 Task 1 的独立基准**：使用相同生成文件、视口、渲染器、环境变量和测试二进制构建规则；保留最终二进制哈希。
- [ ] **Step 2: 重复三组成对测量**：有效轮次、负载门槛、ABAB 顺序、A/A 控制和 CI 规则与 Task 1 完全一致；不把两次测量的最好值拼成一张“最终”表。
- [ ] **Step 3: 重复 Monaco 对照和长行 p95**：分别报告端到端、CPU 工作和 GPU wait；长行 p95 既报告中位数也报告各轮范围，不能只报中位数掩盖超限轮次。
- [ ] **Step 4: 做差异审查**：确认每个改善都能由代码/环境变化解释；如果 A/A 噪声仍超过改善幅度，标为统计上未判定，不宣称性能回归修复。
- [ ] **Step 5: 分类最终结论**：每个门槛标为“通过”“未通过”或“统计上未判定”；Monaco 滚动端到端仍不达标时保留未通过，不能用 CPU 工作更好替代规格门槛。

### Task 4: 全量验证并收尾验收报告

**Files:**
- Modify: `docs/specs/code-editor-m2-report.md`
- Read/Run: `ui/editor/**`、`examples/editor-demo/**`、跨平台 vet/build 命令
- Output (uncommitted evidence): `.superpowers/sdd/2026-11-15-editor-m2-perf-acceptance/final/`

**Interfaces:**
- Consumes: Task 3 最终数据、Task 2 回归测试结果、race 分类结果和人工验收未完成清单。
- Produces: 报告与实际测量一致；不改变已完成能力清单，不虚报实窗/IME/Windows 验收。

- [ ] **Step 1: 运行全量静态和单元检查**：`go build ./...`；`go vet ./ui/editor/... ./examples/editor-demo`；`GOOS=windows go vet ./ui/editor/...`；`GOOS=linux go vet ./ui/editor/...`；`go test ./ui/editor/... -count=1`；改动 Go 文件执行 `gofmt -l` 并要求无输出。
- [ ] **Step 2: 运行 race 检查并分类结果**：执行 `go test -race ./ui/editor/... -count=1`；若全量只因墙钟预算失败，分别运行受影响预算测试确认通过且无 race，并将命令输出和分类写入证据；若出现真正数据竞争，阻塞报告收尾并修复。
- [ ] **Step 3: 构建 demo**：执行 `go build -o /dev/null ./examples/editor-demo`，确认验收报告中的复现命令仍适用。
- [ ] **Step 4: 更新报告性能章节**：用 Task 3 的新日期、环境、成对数据、CI、A/A、长行各轮范围、CPU/gpuwait 分解和 Monaco 对照替换旧结论；旧数据保留时明确标为历史，不混入最终判定。
- [ ] **Step 5: 更新报告结论和剩余项**：只有所有自动化门槛通过才写“自动化验收通过”；否则逐项列明原因。实窗、IME、Windows 实机仍列待人工验收；如果滚动瓶颈不在 `ui/editor`，附 profile 证据和后续边界说明。
- [ ] **Step 6: 做最终一致性检查**：检查报告命令、benchmark 名称、数据路径和结论互相一致；确认没有修改其他会话文件，也没有执行 Git 写操作。

## 执行后交付

- 性能证据目录：`.superpowers/sdd/2026-11-15-editor-m2-perf-acceptance/`（不入库）。
- 代码与回归测试：仅在确有 profile 证据时产生，范围限 `ui/editor/**`。
- 验收报告：`docs/specs/code-editor-m2-report.md`。
- 不自动提交或合并；完成后先向用户报告测试结果、未通过项和工作区变更，等待后续 Git 指令。
