# Tango 终端极致性能实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 通过 A→B 两阶段，把 Tango 终端的输入回显、PTY/VT 吞吐、滚动和重复绘制性能提升到可测量的极致，同时保持终端行为和跨平台兼容性。

**Architecture:** A 阶段先在现有 VT/视图上增加事件路由直通、保留额度发送队列、批量读写、Resize 生命周期解耦和性能计量；B 阶段再增加版本化屏幕快照和稳定行缓存，让 UI 绘制不持有 VT 主锁且未变化行不重复生成绘制 ops。CPU renderer 保持正确性基准，GPU 继续消费现有 `paint.List` 与 damage/scroll 提示。

**Tech Stack:** Go 1.27、Tango 自绘 UI、`ui/terminal/vt`、`terminal.Conn`、Go benchmark、`-race`；不新增依赖、不使用 cgo。

**Spec:** `docs/specs/2026-11-15-terminal-performance-design.md`

## Global Constraints

- 纯 Go、无新增第三方依赖、桌面端不使用 cgo；终端 VT 行为继续以现有回放和单元测试为准。
- 不实现 M2/M3 VT 功能；不引入 GPU 专用终端纹理路径。
- 普通按键、Ctrl 组合、Esc、Enter 使用保留发送额度，不能丢；超大粘贴可截断但必须返回明确状态并记录，不能静默丢弃；UI 线程不能无限阻塞。
- 输入直通使用终端专用实验性通道，不修改稳定的 `ui/input` 事件类型。
- `Conn.Resize` 不得在终端生命周期锁内阻塞；Resize、Close、OnExit 必须保持有序。
- A/B 前后都必须通过终端回放、`-race`、三平台 `CGO_ENABLED=0` `go vet`/编译；CPU/GPU 与移动端回放行为不能回归。
- 终端 Go 堆增量、缓存大小、锁等待和 benchmark 结果必须在报告中记录；性能结果至少运行 3 次并记录机器负载。

## Review Focus

1. 同一帧内 Composition→TextInput→Enter/Backspace 的顺序以及重复 build：每个事件只发送一次，IME 不丢提交文本；由 Task 2 的输入路由测试覆盖。
2. 发送队列达到上限、普通按键与大粘贴同时到达、Send/Close/回复写并发：普通键不丢，粘贴有明确截断状态且不会死锁；由 Task 2 的队列测试覆盖。
3. Read 返回 `n>0, err!=nil`、UTF-8/CSI/OSC 跨 batch、回复额度耗尽和阻塞 Conn：VT 字节不丢、不重复、Close 可完成；由 Task 3 的 Conn 测试覆盖。
4. 快照发布后同时发生输出、scrollback 环形复用、resize、clear、style compaction、选区复制：UI 不读到已复用数据、字符/样式/选区正确；由 Task 5/6 的并发和逐格测试覆盖。
5. 宽字符、组合字符、光标、IME 光标和主题/字号变化时的行缓存失效：宁可重画不能画旧内容；由 Task 6 的缓存失效测试覆盖。

---

### Task 1: 性能基准与可观测性基线

**Files:**
- Create: `ui/terminal/perf_test.go`
- Create: `ui/terminal/vt/perf_test.go`（若现有 benchmark 适合拆分，则在原测试文件中修改）
- Modify: `ui/terminal/conn.go`（只增加内部计量钩子）
- Modify: `ui/terminal/view.go`（只增加测试可读的内部计量）

**Interfaces:**
- Consumes: 现有 `Terminal`, `ViewW`, `vt.Terminal`, `pipeConn` 测试脚手架。
- Produces: `perfStats`（内部测试类型，至少包含 `FeedBytes`、`FeedBatches`、`FeedLockWait`、`PaintRows`、`CacheHits`、`CacheMisses`、`SnapshotBytes`）和三个稳定 benchmark 输入：纯 ASCII、彩色 ANSI、PTY 风格输出；后续任务只使用这些计量，不自行加临时计时器。

- [ ] **Step 1: 写失败的计量测试和基准**
  - `TestPerfStatsRecordFeedLockWait`：写入固定 ANSI 数据后断言 batch/字节/锁等待计数存在。
  - `BenchmarkTerminalASCII`、`BenchmarkTerminalANSI`、`BenchmarkTerminalPTYReplay`：每次 benchmark 新建 emulator，`ResetTimer` 后只测明确生命周期，避免复用状态污染。
  - `BenchmarkViewFrameIdle`、`BenchmarkViewFrameOneDirtyRow`、`BenchmarkViewFrameScroll`：分别记录 paint 行数、段缓存命中和分配。
- [ ] **Step 2: 运行确认失败**
  - `go test ./ui/terminal ./ui/terminal/vt -run 'PerfStats|^$' -bench 'Terminal|ViewFrame' -benchmem -count=1`
  - Expected: 新测试因 `perfStats`/benchmark fixture 未定义而失败。
- [ ] **Step 3: 实现最小内部计量**
  - 在 `Terminal`/`ViewW` 内部增加仅测试可见的计量快照，不能改变生产同步语义；benchmark 输入必须通过 `vt.Terminal.Write` 或现有 `Conn`，不能绕过解析器。
- [ ] **Step 4: 运行并保存基线**
  - `CGO_ENABLED=1 go test ./ui/terminal ./ui/terminal/vt -run 'PerfStats|^$' -bench 'Terminal|ViewFrame' -benchmem -count=3`
  - Expected: PASS；结果写入 `.superpowers/sdd/2026-11-15-terminal-perf/baseline.txt`，记录负载。
- [ ] **Step 5: Commit**
  - `git commit -m "test(terminal): establish performance baselines"`

### Task 2: 输入事件直通与保留额度发送队列

**Files:**
- Create: `ui/terminal/input_fastpath_test.go`
- Modify: `ui/runner.go`
- Modify: `ui/context.go` 或新增 `ui/input_hook.go`
- Modify: `ui/terminal/view.go`
- Modify: `ui/terminal/conn.go`
- Modify: `ui/terminal/conn_test.go`

**Interfaces:**
- Consumes: Task 1 的计量；现有 `input.Event`、`ui.Context.EventsFor`、`Terminal.Send`。
- Produces: 实验性终端事件通道，具体签名为 `Context.TerminalEvents(id ID, fn func(input.Event) input.EventResult)`；事件在 `Runner.HandleEvent` 入队时先交给当前声明且聚焦的终端 handler，handler 返回 `Handled` 后不重复进入普通 frame 队列。`Terminal.SendInput(b []byte) error` 负责保留额度/截断状态；普通 `Send` 的已有异步 FIFO 语义保持。

- [ ] **Step 1: 写失败测试**
  - `TestTerminalEventsSendsKeyBeforeNextFrame`：向 runner 注入按键，断言 Conn 在下一次 `Frame` 前已收到编码字节。
  - `TestTerminalEventsCompositionCommitEnterOrder`：Composition、TextInput、Enter 依次到达，断言字节顺序且无重复。
  - `TestTerminalEventsBuildTwiceDoesNotDuplicate`：同一事件不能在两个 build 中发送两次。
  - `TestSendInputReservesKeyAllowance`：填满粘贴额度后普通 `Enter`/Ctrl-C 仍能进队列；粘贴超限返回截断结果并记录。
- [ ] **Step 2: 运行确认失败**
  - `go test ./ui/terminal -run 'TerminalEvents|SendInput' -v`
  - Expected: FAIL，因为事件通道和 `SendInput` 尚不存在。
- [ ] **Step 3: 实现事件路由**
  - 在 `ui` 中保存当前 frame 声明的 handler 与焦点 ID；`Runner.HandleEvent` 只对终端 handler 做快速回调，线程安全地请求重绘，普通控件行为不变。
  - `View` 注册 handler；Key/Text/Composition 必须按宿主原始顺序处理，IME composition 只在提交时进入终端输入队列。
- [ ] **Step 4: 实现保留额度**
  - 在 `conn.go` 的队列状态中区分 `interactiveBytes` 与 `bulkBytes`；交互输入有固定保留额度，超大粘贴返回 `SendResult{AcceptedBytes, Truncated}`，关闭时所有等待者可退出。
  - 不让 UI handler 等待 Conn.Write；Writer goroutine 维持 FIFO 和 reply 优先级。
- [ ] **Step 5: 运行测试和竞态测试**
  - `go test ./ui/terminal -run 'TerminalEvents|SendInput' -v`
  - `go test -race ./ui/terminal -run 'TerminalEvents|SendInput|Close|Reply' -count=1`
  - Expected: 全部 PASS。
- [ ] **Step 6: Commit**
  - `git commit -m "perf(terminal): send interactive input before paint"`

### Task 3: 读循环批量化与 Resize/Close 生命周期解耦

**Files:**
- Create: `ui/terminal/conn_perf_test.go`
- Modify: `ui/terminal/conn.go`
- Modify: `ui/terminal/conn_test.go`
- Modify: `ui/terminal/view.go`

**Interfaces:**
- Consumes: Task 1 计量、Task 2 的输入队列；现有 `terminal.Conn`。
- Produces: 内部 `feedBatch([]byte)` 和 `resizeAsync(cols, rows, widthPx, heightPx)` 语义；`Conn.Resize` 不在 `smu`/VT 主锁内调用，resize 请求按序列号提交，Close/OnExit 能取消未执行 resize。

- [ ] **Step 1: 写失败测试**
  - `TestFeedReadWithDataAndError`：Conn 一次返回 `n>0, io.EOF`，数据全部进入 VT 后正常退出。
  - `TestFeedBatchBoundaryAcrossUTF8AndCSI`：每个字节边界都回放同一输入，屏幕逐格一致。
  - `TestResizeDoesNotBlockClose`：Conn.Resize 阻塞时 Close 仍能完成并解除 Read/Write。
  - `TestReplyAllowanceDoesNotStarveInput`：回复额度耗尽时交互输入仍可退出。
- [ ] **Step 2: 运行确认失败**
  - `go test ./ui/terminal -run 'FeedReadWithDataAndError|FeedBatch|ResizeDoesNotBlock|ReplyAllowance' -v`
  - Expected: 至少 Resize/边界用例失败，暴露当前锁和 batch 生命周期。
- [ ] **Step 3: 实现有限批量 feed**
  - 保留单次 Read 的 32KiB 上限和 `maxLockBytes`；在锁外拼接有限批次，在锁内按上限写入；不能等待“所有可用数据”，不能跨调用无限缓存。
  - 每批只触发一次 invalidate；回复仍走独立额度；`n>0, err!=nil` 先处理数据再结束。
- [ ] **Step 4: 解耦 Resize**
  - 将 resize 请求放到有序控制队列，由连接控制 goroutine 执行；Close 取消旧请求并等待控制 goroutine 退出；任何 Conn 实现不得在生命周期锁内阻塞。
- [ ] **Step 5: 运行测试、race 和平台检查**
  - `go test ./ui/terminal -run 'FeedReadWithDataAndError|FeedBatch|ResizeDoesNotBlock|ReplyAllowance' -v`
  - `go test -race ./ui/terminal ./sys/pty -count=1`
  - `for os in darwin linux windows; do CGO_ENABLED=0 GOOS=$os go vet ./ui/terminal ./sys/pty; done`
  - Expected: 全部 PASS。
- [ ] **Step 6: Commit**
  - `git commit -m "perf(terminal): batch output and decouple resize lifecycle"`

### Task 4: A 阶段绘制热点优化与门槛验收

**Files:**
- Modify: `ui/terminal/paint.go`
- Modify: `ui/terminal/view.go`
- Modify: `ui/terminal/perf_test.go`
- Create: `docs/specs/2026-11-15-terminal-performance-a-report.md`

**Interfaces:**
- Consumes: Task 1–3 的计量、输入通道、batch/resize 语义。
- Produces: A 阶段性能报告；不新增对外稳定 API。所有绘制优化必须保留 `paintRow` 的宽字符、组合字符、selection、cursor 和 damage 行为。

- [ ] **Step 1: 用 benchmark/profile 写回归测试**
  - `TestViewFrameOneDirtyRow` 断言 idle frame 不重建未变行，scroll frame 只处理新增/脏行；`TestViewPaintWideAndCombiningAfterFastPath` 逐格验证宽字符和组合字符。
- [ ] **Step 2: 运行基线并保存 profile**
  - `go test ./ui/terminal -run '^$' -bench 'ViewFrame' -benchmem -count=5 -cpuprofile /tmp/terminal-a-before.pprof`
  - Expected: 记录 `paintRow`、`shapeSeg`、分配和锁等待占比。
- [ ] **Step 3: 实现热点优化**
  - 只优化 profile 中占比最高的路径：复用行内切片、避免重复字符串转换、扩大/限制段缓存时保持内存上限、减少不变行的 segment 解析；禁止为了 benchmark 删除宽字符/IME/selection 分支。
- [ ] **Step 4: 运行 A 阶段测试**
  - `go test -race ./ui/terminal ./ui/terminal/vt ./sys/pty -count=1`
  - `go test ./ui/terminal -run '^$' -bench 'TerminalASCII|TerminalANSI|TerminalPTYReplay|ViewFrame' -benchmem -count=5`
  - Expected: 回放全过；输入 p95 ≤16.7ms、p99 ≤25ms；三类吞吐不低于规格且相对 baseline 可重复提升；无明显内存回归。
- [ ] **Step 5: 实机/平台验证并写报告**
  - macOS 跑 `examples/terminal -auto`、50MB `-bench-cat`、中文/IME/粘贴/滚动；Linux 字体容器测试；Windows 三平台 vet/交叉编译；记录失败的独立 editor 性能测试但不归因终端。
- [ ] **Step 6: Commit**
  - `git commit -m "perf(terminal): complete phase A latency and throughput pass"`

### Task 5: 版本化 VT 屏幕快照

**Files:**
- Create: `ui/terminal/vt/snapshot.go`
- Create: `ui/terminal/vt/snapshot_test.go`
- Modify: `ui/terminal/vt/terminal.go`
- Modify: `ui/terminal/conn.go`

**Interfaces:**
- Consumes: Task 4 的 A 阶段正确性和 batch 语义；现有 `Screen`, `Line`, `Cell`, `StyleTable`。
- Produces: 实验性 `type Snapshot struct`、`type SnapshotRow struct`、`func (t *Terminal) Snapshot() Snapshot`、`func (s Snapshot) Release()`；快照包含全局版本、cols/rows、view offset、cursor、title、style generation 和每行稳定 ID/version/cell 数据。快照数据在 Release 前只读，不能返回内部 Cell/Line 指针。

- [ ] **Step 1: 写失败测试**
  - `TestSnapshotIsIndependentAfterWrite`：发布快照后继续写入/回滚，旧快照字符和样式不变。
  - `TestSnapshotVersionsOnlyChangeDirtyRows`：只写一行时其他行版本不变。
  - `TestSnapshotStyleCompactionInvalidatesGeneration`：style compaction 后 style generation 改变。
  - `TestSnapshotReleaseAndClose`：发布、释放、Close 并发不 panic、不读已复用切片。
- [ ] **Step 2: 运行确认失败**
  - `go test ./ui/terminal/vt -run 'Snapshot' -v`
  - Expected: FAIL，因为 Snapshot 类型和方法不存在。
- [ ] **Step 3: 实现只读快照**
  - 在 VT 锁内复制必要的行/样式数据，锁外 UI 只消费快照；行 ID 必须随 scrollback 环形淘汰和 resize 正确更新，不能只用可见 y 坐标。
  - Release 归还内部池或释放引用；任何复用策略必须通过 race 和旧快照独立性测试。
- [ ] **Step 4: 运行快照测试和全 VT 回放**
  - `go test -race ./ui/terminal/vt -run 'Snapshot|Replay' -count=1`
  - Expected: PASS，现有逐格回放无差异。
- [ ] **Step 5: Commit**
  - `git commit -m "perf(terminal): add versioned read-only screen snapshots"`

### Task 6: B 阶段稳定行缓存与无锁绘制

**Files:**
- Create: `ui/terminal/rowcache.go`
- Create: `ui/terminal/rowcache_test.go`
- Modify: `ui/terminal/paint.go`
- Modify: `ui/terminal/view.go`
- Modify: `ui/terminal/perf_test.go`

**Interfaces:**
- Consumes: Task 5 的 `vt.Snapshot`/`SnapshotRow`；Task 4 的绘制指标和现有 `paint.List`。
- Produces: 内部 `rowCache`，缓存键至少包含 row ID/version、cols、font metrics、scale、scheme generation、selection range/colour and cursor/IME overlay state；提供命中、未命中、淘汰和字节计数，且有条目/字节硬上限。

- [ ] **Step 1: 写失败测试**
  - `TestRowCacheReusesUnchangedRows`：同一快照连续绘制，第二帧未变化行命中且不重新 shape。
  - `TestRowCacheInvalidatesDirtyRowAndSelection`：行版本或选区改变只失效对应行。
  - `TestRowCacheInvalidatesMetricsSchemeAndStyleGeneration`：字号、scale、主题、style generation 改变不会复用旧 ops。
  - `TestRowCacheEvictionBounded`：缓存超过条目/字节上限会淘汰，内存计数不超限。
- [ ] **Step 2: 运行确认失败**
  - `go test ./ui/terminal -run 'RowCache' -v`
  - Expected: FAIL，因为 rowCache 不存在。
- [ ] **Step 3: 实现 rowCache**
  - 缓存终端自有、可验证生命周期的 paint ops/glyph slices；不保存外部可变指针。selection、cursor、IME 等动态 overlay 与静态行绘制分开，避免光标变化导致所有行失效。
  - 发生 resize、主题/字号切换、快照 style generation 变化时批量淘汰；scrollback 行 ID 变化时只淘汰受影响行。
- [ ] **Step 4: 接入 ViewW**
  - `ViewW.paint` 获取一次 `vt.Snapshot`，解锁后按快照绘制；不再在整个 paint 期间持有 VT 主锁。保留 `ScrollShift`、脏行 damage、无障碍文本和 selection 语义。
  - 对照旧路径逐格检查字符、颜色、属性、宽字符、组合字符、光标、选区和无障碍文本。
- [ ] **Step 5: 并发/回归测试**
  - `go test -race ./ui/terminal ./ui/terminal/vt -run 'RowCache|Snapshot|View|Selection|IME' -count=1`
  - `go test ./ui/terminal -run '^$' -bench 'ViewFrame|Terminal' -benchmem -count=5`
  - Expected: 无数据竞争；命中率、快照复制字节、CPU paint、帧时间和内存均记录，且不低于 A 阶段门槛。
- [ ] **Step 6: Commit**
  - `git commit -m "perf(terminal): cache versioned rows outside the VT lock"`

### Task 7: B 阶段实机验收、性能报告与合并准备

**Files:**
- Create: `docs/specs/2026-11-15-terminal-performance-report.md`
- Modify: `docs/specs/terminal-m1-report.md`（只补链接和最终数据）
- Modify: `examples/terminal/e2e_test.go`（如需增加性能场景）

**Interfaces:**
- Consumes: Task 6 的 rowCache/快照和所有性能计量。
- Produces: 可复现的 A/B 性能报告、最终测试清单和合并候选 HEAD；不直接修改其他保留区。

- [ ] **Step 1: 跑全终端专项验证**
  - `CGO_ENABLED=1 go test -race ./ui/terminal ./ui/terminal/vt ./sys/pty -count=1`
  - `for os in darwin linux windows; do CGO_ENABLED=0 GOOS=$os go vet ./ui/terminal ./sys/pty; done`
  - `go test ./examples/terminal ./ui/terminal/... ./sys/pty/... -count=1`
- [ ] **Step 2: 跑实机 benchmark**
  - macOS 至少 3 次：`-auto`、50MB `-bench-cat`、10,000 行滚动、GPU/CPU；记录负载、p50/p95/p99、fps、lock wait、cache hit、snapshot bytes、allocs。
  - Linux 字体容器跑完整终端测试；Windows `go test -c` 和示例交叉编译；移动端跑 replay/echo/软键盘回显。
- [ ] **Step 3: 写报告并核对门槛**
  - 报告必须区分纯 ASCII、彩色 ANSI、PTY 三类吞吐；列出优化前后差值和未达标项，不能把 editor 性能阈值失败归因于终端。
- [ ] **Step 4: 全仓测试**
  - `CGO_ENABLED=1 go test ./...`
  - Expected: 全部通过；如果仅有已知、可重复的独立 `ui/editor` 性能阈值失败，停止合并并在报告中列出，不能宣称全仓绿色。
- [ ] **Step 5: Commit**
  - `git commit -m "docs(terminal): record A/B performance results"`
- [ ] **Step 6: 合并前检查**
  - 检查 `git diff main...HEAD` 只包含本性能计划的文件；确认所有测试、报告和 review 通过后，才向用户请求把分支合并到 `main`。

## 计划自检

- A 阶段独立可运行：Task 1–4 有各自基准、输入/Conn/绘制测试和实机门槛；B 阶段只在 A 阶段绿色后开始。
- B 阶段接口一致：Task 5 产生 `vt.Snapshot`/`SnapshotRow`，Task 6 只消费快照并负责 rowCache；Task 7 只消费计量和验收产物。
- 关键未覆盖输入已在 Review Focus 中绑定到 Task 2/3/5/6；所有优化都有回放或逐格正确性测试。
