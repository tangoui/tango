# Metal 异步 Command Buffer 与 Present Fence 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (Native). Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 在现有 Metal F/P 基础上，以安全有界的 in-flight command buffers 和 Metal 内部 present diagnostics 降低 CPU/GPU 串行等待，并准确区分 encode、GPU 完成和 drawable back-pressure。

**Architecture:** A1 把 `Device` 的全量等待拆为 oldest/all 两种，统一统计 frame/present command buffers，只有资源依赖安全时才允许延后等待；所有 target、spare、layer、instance/staging 和上传引用由 submission retirement 管理。B1 增加 Metal-only frame token 和 drawable/GPU 时间诊断，不修改 `gpu.Device`、uihost 或非 darwin 公共签名。

**Tech Stack:** Go 1.27、purego Objective-C Metal、现有 `internal/render/gpu/metal`、Go tests/race、编辑器 perfab paired runner。

**Spec:** `docs/specs/2026-11-16-metal-async-present-design.md`

## Global Constraints

- 当前 F/P retained-copy 正确性不能回退；不做 per-resource ring 或三缓冲 retained target。
- frame + present command buffers 统一计入最多 2 个 in-flight；满载时只等待最老提交。
- `Wait`、ReadPixels、Resize、Close、device lost recovery 必须 drain 全部；不能提前释放 GPU 仍引用的资源。
- present diagnostics 只在 Metal 内部启用，默认 nil、无热路径原子/逐像素开销；无真实 FrameDisplayed 时必须 unknown。
- `nextDrawable` 等待单独计时；超过有界等待则跳过本次 present，不阻塞 CPU 约 1 秒。
- 非 darwin 保持 ErrUnsupported/no-op；不修改稳定 `gpu.Device` 接口或 uihost 公共签名。
- 不以 locked-offscreen 数字冒充真实窗口呈现；至少 20 轮低负载 paired，保留 A/A 和所有慢轮次。

## Review Focus

1. oldest command buffer 失败但后续仍 pending：错误传播、device lost 和 retained target 状态不能错误恢复。
2. in-flight 中的 instance/staging、atlas/ramp/color/image、ImageFrees、layer、scratch、F/P spare：必须等对应提交退休才复用或释放。
3. presenter 的 nextDrawable 缺失/超时、drawable wait 与 GPU completion wait：不得互相混入或无限阻塞。
4. Close/Resize/ReadPixels 与 Execute/present 并发：不释放仍被 GPU 引用的 Objective-C 对象，也不死锁。
5. token 时间线缺字段或 GPU 时间不可用：必须标记 unknown，不用 CPU 时间伪造硬件完成。

---

### Task 1: 确定性 Metal Submission/Wait 测试钩子

**Files:**
- Modify: `internal/render/gpu/metal/metal_darwin.go`
- Modify: `internal/render/gpu/metal/metal_other.go`
- Create/Modify: `internal/render/gpu/metal/async_darwin_test.go`
- Modify: `internal/render/gpu/metal/metal_darwin_test.go`

**Interfaces:**
- Consumes: 现有 `Device`, `inflight`, `wait`, F/P tests。
- Produces: internal-only `submission` / `submissionStats` 计数和测试注入的 command-buffer completion seam；不增加 `gpu.Device` 方法。

- [ ] **Step 1: 写失败测试**
  - `TestMetalInFlightBounded`：连续提交 3 个可控 completion 的 frame，断言最多保留 2 个，第三次只退休 oldest。
  - `TestMetalWaitAllDrainsAll`：显式 `Wait` 退休所有提交。
  - `TestMetalExecuteDoesNotDrainAll`：未达上限时 Execute 不调用全量 wait。
- [ ] **Step 2: 运行红灯**
  - `go test ./internal/render/gpu/metal -run 'InFlightBounded|WaitAllDrainsAll|ExecuteDoesNotDrainAll' -v`
  - Expected: 当前无 submission seam/oldest 策略，测试失败。
- [ ] **Step 3: 实现最小 submission 结构**
  - 保存 command buffer、提交序号、提交时间、GPU 时间读取状态和资源 retire 回调/引用集合。
  - 实现 `waitOldest()`、`waitAll()`；测试注入只在 darwin internal test build 生效，生产默认走真实 Objective-C command buffer。
- [ ] **Step 4: 运行 green 和 race**
  - `go test ./internal/render/gpu/metal -run 'InFlightBounded|WaitAllDrainsAll|ExecuteDoesNotDrainAll' -v`
  - `go test -race ./internal/render/gpu/metal -run 'InFlight|Wait' -count=1`
- [ ] **Step 5: Commit**
  - `git commit -m "test(metal): add deterministic submission retirement hooks"`

### Task 2: A1 有界 in-flight 与错误/资源退休

**Files:**
- Modify: `internal/render/gpu/metal/metal_darwin.go`
- Modify: `internal/render/gpu/metal/metal_darwin_test.go`
- Modify: `internal/render/gpu/metal/copy_target_darwin_test.go`
- Modify: `internal/render/gpu/metal/damage_darwin_test.go`

**Interfaces:**
- Consumes: Task 1 的 submission/wait seam。
- Produces: `Execute` 不再无条件 `waitAll`；统一 frame/present in-flight cap=2；`Wait`/ReadPixels/Close/Resize 使用 waitAll；资源释放延迟到 retirement。

- [ ] **Step 1: 写失败测试**
  - `TestMetalErrorAfterDeferredCompletion`：oldest 失败、后续仍 pending，错误只在正确观察点出现，device lost 规则不变。
  - `TestMetalCopyTargetCannotReuseBeforeFence`：target/spare 仍被引用时不能交换/释放。
  - `TestMetalResizeCloseDrainsInflight`：resize/Close 释放前 drain 全部。
  - `TestMetalRetireResourcesAfterFence`：instBuf/staging、images/ImageFrees、layers、scratch/spare 的释放/复用只在 retirement 后发生。
- [ ] **Step 2: 运行红灯**
  - `go test ./internal/render/gpu/metal -run 'DeferredCompletion|CannotReuse|ResizeClose|RetireResources' -v`
  - Expected: 当前 Execute 全量等待/资源即时复用语义不能满足测试。
- [ ] **Step 3: 改 Execute 等待策略**
  - Execute 开头调用 `waitOldest` 仅在 in-flight 达到 2 时；不达到上限不 drain。
  - frame 和 present commit 都进入同一 submission 队列；present 删除现有独立 `maxInflight=4` 全 drain 行为，改用统一 cap。
- [ ] **Step 4: 延后资源退休**
  - 将需要 GPU 完成的资源放入对应 submission 的 retire 集合；resize/Close/device lost 先 waitAll 再 release。
  - F/P 只有在写入它的 submission 退休后才允许 copy target/spare 交换或释放；layer pool 不能复用 busy layer。
  - 若资源 ring 不存在导致依赖冲突，安全等待而不是新增第三个 target。
- [ ] **Step 5: 错误与回归验证**
  - `go test ./internal/render/gpu/metal -run 'DeferredCompletion|CannotReuse|ResizeClose|RetireResources|CopyTarget|Presenter' -v`
  - `go test -race ./internal/render/gpu/metal -count=1`
  - `go test ./ui/editor -run 'TestScrollDamageStrip$' -count=1`
- [ ] **Step 6: Commit**
  - `git commit -m "perf(metal): bound in-flight command buffers safely"`

### Task 3: B1 Metal 内部 Frame/Present diagnostics token

**Files:**
- Modify: `internal/render/gpu/metal/metal_darwin.go`
- Modify: `internal/render/gpu/metal/presenter_darwin.go`
- Modify: `internal/render/gpu/metal/metal_other.go`
- Create: `internal/render/gpu/metal/timeline_darwin_test.go`
- Modify: `internal/render/gpu/metal/presenter_darwin_test.go`

**Interfaces:**
- Consumes: Task 2 submission retirement。
- Produces: Metal-only diagnostics reader/token，默认 nil；字段包含 frame ID、commit CPU time、GPU start/end（unknown 可用）、drawable acquire start/end、present commit 和 completion error。无 `FrameDisplayed` 字段伪造。

- [ ] **Step 1: 写失败测试**
  - `TestMetalFrameTokenTimeline`：frame IDs/commit 顺序单调，GPU 时间在 retirement 后可读，否则 unknown。
  - `TestMetalPresentDoesNotWaitForGPU`：present encode 不隐式 waitAll，显式 Wait 才 drain。
  - `TestMetalDrawableBackpressureMeasuredSeparately`：drawable acquisition 区间独立于 GPU duration。
  - `TestMetalTokenDrainAfterReadPixelsClose`：ReadPixels/Close drain token 后不留悬挂 Objective-C 引用。
- [ ] **Step 2: 运行红灯**
  - `go test ./internal/render/gpu/metal -run 'FrameToken|PresentDoesNot|DrawableBackpressure|TokenDrain' -v`
- [ ] **Step 3: 实现 diagnostics-only timeline**
  - callback/reader 默认 nil；启用时在 submission retirement 填 GPUStart/End，不能在热路径增加原子和逐像素逻辑。
  - presenter 的 `nextDrawable` 包裹有界 monotonic 计时；超时/无 drawable 跳过本次 present并记录状态，下一帧可重试。
  - non-darwin 提供编译所需的 no-op/unknown 类型，不改变公开签名。
- [ ] **Step 4: 运行测试和 race**
  - `go test ./internal/render/gpu/metal -run 'FrameToken|Presenter|ReadPixels|Close' -v`
  - `go test -race ./internal/render/gpu/metal -count=1`
- [ ] **Step 5: Commit**
  - `git commit -m "perf(metal): expose internal frame timeline diagnostics"`

### Task 4: A/B 延迟验收与报告

**Files:**
- Create: `docs/specs/2026-11-16-metal-async-present-report.md`
- Modify: `docs/specs/code-editor-m2-report.md`（只补最新 F/P/A/B 数据和链接）
- Create/Modify: `internal/render/gpu/metal/async_perf_darwin_test.go`

**Interfaces:**
- Consumes: Task 2/3 的 bounded submission 与 timeline reader。
- Produces: 20 轮低负载 paired 数据、A/A 控制、离屏/真实窗口区分报告；不改变性能门槛。

- [ ] **Step 1: 建立基准**
  - F/P 当前主线作为 baseline；A1 开关作为 after；两臂同二进制/输入/scale/Wait 口径。
  - 覆盖 `BenchmarkTypeTS20k`、`BenchmarkScrollTS20k`、`BenchmarkScrollPartial`、`BenchmarkTypeAt1MLinesCPU`；记录 encode、oldest wait、gpu wait、drawable wait、queue latency、p50/p95/p99、fps。
- [ ] **Step 2: 低负载 paired**
  - 至少 20 轮，负载阈值 6，保留所有有效轮，A/A 控制；任何高负载轮按既有驱动作废重跑。
  - 真实窗口仅在前台/解锁/刷新率已知时测；没有 FrameDisplayed 身份则标记 not proven。
- [ ] **Step 3: 内存和资源验证**
  - 记录 in-flight peak、target/spare/layer/instance/staging 原生成本；A1 不得隐式增加第三个 retained target。
  - `ReadPixels`、resize、Close、device lost 和 drawable timeout 场景各跑一次。
- [ ] **Step 4: 写报告**
  - 区分 CPU encode、GPU completion、drawable back-pressure 和真实呈现；如果 A1 无显著收益，保留该结论，不继续盲目增大 cap。
- [ ] **Step 5: 全部验证**
  - `go test ./internal/render/gpu/metal -count=1`
  - `go test -race ./internal/render/gpu/metal -count=1`
  - `for os in darwin linux windows; do CGO_ENABLED=0 GOOS=$os go vet ./internal/render/gpu/metal; done`
  - `go test -p 1 ./...`
- [ ] **Step 6: Commit**
  - `git commit -m "docs(metal): record async command buffer latency results"`

## Review Focus 对应

- Task 1/2：提交失败、oldest failure、pending 后续 CB、资源 retire。
- Task 2：所有资源类别、F/P spare、layer/blur、resize/Close。
- Task 3：drawable timeout、token unknown、GPU/CPU 时间分离。
- Task 4：paired/A-A、低负载、真实窗口与离屏口径、内存代价。
