# Metal 异步 Command Buffer 与 Present Fence 设计

日期：2026-11-16
状态：已批准（2026-11-16，用户确认 A1 默认决策）
范围：`internal/render/gpu/metal/**`、必要的 `internal/render/gpu/**`/`uihost` 接口和 Metal 专项测试；不回退或重写现有 F/P retained-copy。

## 1. 意图

当前 Metal F/P 已减少 retained-copy 的像素搬运，但 `Device.Execute` 在编码新帧前会等待上一批 in-flight command buffers，Presenter 又把 drawable copy 放在独立 command buffer 上，编辑器基准每帧还会显式调用 `Wait`。这使 CPU encode、GPU 执行和 present 基本串行，输入/滚动的端到端 p95 仍受 command-buffer wait 和 drawable back-pressure 影响。

本专项的目标不是伪造更快的离屏数字，而是让 CPU 能在安全边界内提前准备下一帧，并把“提交完成、GPU 完成、drawable 呈现”分别计量；任何必须等待的地方都有明确的 bounded back-pressure 和资源生命周期约束。

## 2. 当前事实

- F/P 已在主线：严格 copy 像素、blur、readback/present、resize、device-lost、Close 和 race 测试通过。
- `Device.Execute` 当前先调用 `d.wait()`，然后 encode/commit；`Device.Wait` 等待所有 in-flight command buffers。
- `presenter.present` 使用 CAMetalLayer drawable，drawable 数量和 `nextDrawable` 自带 back-pressure；present command buffer 在 frame command buffer 之后提交。
- F/P 诊断中 copy 路径的 GPU wait 有约 8.9% 的下降，但 FPS 没有显著改善；剩余成本不能默认归因于 copy。
- 非 macOS backend 的 `Device` 没有 Metal command buffer 语义，必须保持 no-op/ErrUnsupported，不改变公共稳定 API。

## 3. 成功标准

- 正确性：现有 Metal 严格像素、F/P copy、blur、readback/present、resize、failure、device-lost、Close 和 race 测试继续通过。
- A 阶段：CPU encode 不再无条件等待上一帧；in-flight 数量有硬上限（默认 2），超过上限时等待最老的完成帧；内存和 target 复用安全。
- B 阶段：能分别记录 encode duration、queue latency、GPU start/end、present enqueue、drawable acquisition/back-pressure 和 completion wait；不把 CPU 提交时间称为屏幕呈现时间。
- 性能：同一输入、同一机器、同一 renderer，至少 20 轮低负载 paired 测量；输入/滚动 p95/p99 或 GPU wait 有统计上可解释的改善，若没有则保留“不改善”结论，不放宽门槛。
- 资源：retained target、spare target、layer drawable、instance/staging buffer 只能在对应 command buffer 完成后复用；Close、resize、device lost 必须等待或取消全部资源引用。
- 非目标平台：darwin 之外编译、vet、测试通过，行为不变。

## 4. 方案与已选路线

### A：有限 in-flight command buffers（第一阶段，采用）

1. `Device` 增加有界 in-flight policy：默认最多 2 个未完成 command buffer；`Execute` 在达到上限时只等待最老的一批，不在每帧无条件 drain 全部。
2. 将旧的 `d.wait()` 拆为：`waitOldest()`（释放到上限以下）和 `waitAll()`（Close、ReadPixels、显式 Wait、resize/device lost）。
3. Execute 的错误传播保持原语义：command buffer completion error 由下一次需要观察完成状态的调用报告；pending error 不静默吞掉；device lost 仍在原阈值触发。
4. F/P retained target 的安全边界保持：如果下一帧会读写上一帧 target，必须等对应 fence；只有真正独立的 target/资源可提前 encode。不能仅靠增加 in-flight 数绕过同一纹理读写依赖。
5. 为 tests 提供 internal counters，不放入对外 API：inflight peak、oldest waits、waitAll waits、encode time、commit count、queue latency、GPU duration。

A 的核心风险：当前 target 是 retained 单 target，许多 partial/full 路径仍然必须等待上一帧；A 可能只降低 CPU 空转而不提高真实帧率，这是可接受的测量结果，不能为了让 A“有效”而错误复用 target。

### B：present fence / frame feedback（第二阶段，采用）

1. 增加实验性 internal `FrameToken`/`PresentToken`，只由 Metal presenter 产生，包含 frame id、commit time、GPU start/end（可用时）、drawable acquisition time 和 completion status。
2. `present` 不立即等待 GPU；它提交 drawable copy 后返回 token 或 token 记录到 internal diagnostics。需要截图、ReadPixels、关闭和 resize 时显式 drain token。
3. 平台宿主不要求每帧同步反馈：无 `CAMetalLayer`、非 darwin 或不支持 hardware feedback 时 token 字段为 unknown，不能伪造 `FrameDisplayed`。
4. 提供 diagnostics-only callback/reader，默认 nil、无热路径原子计数和逐像素成本；测试可启用后获取 queue latency/GPU time/present back-pressure。
5. 以真实窗口和离屏两种场景分别报告：离屏只能证明 command buffer/GPU 完成，真实窗口才能讨论 drawable/vsync；两者不可混比。

### 不先做 C：三缓冲 retained target

三目标可以进一步减少单 target 依赖，但每个 Retina target 约 33MB，且 F/P 的 scroll/copy、blur、layer 和 resize 生命周期会复杂化。只有 A/B 计量证明 target 复用是主要瓶颈，且能接受原生内存预算后，才另立 C 设计。

## 5. 已确认的 A1 决策

用户确认以下默认策略：

- **安全优先，不做 per-resource ring**：普通单 retained target/共享 upload buffer 等路径如果有读写依赖，允许继续等待；A1 不承诺一定产生 GPU 重叠收益，不通过错误复用资源换 benchmark 数字。
- **统一 in-flight 上限 2**：Execute 帧 command buffer 和 present command buffer 统一计入同一个上限；达到上限只退休最老提交，而不是无条件 drain 全部。
- **B1 先做 Metal 内部 diagnostics token**：不修改 `gpu.Device`、`uihost` 或非 darwin 签名；未提供真实 FrameDisplayed 时标记 unknown。
- **drawable 有界等待**：`nextDrawable` 的等待单独计时；超出上限则跳过本次 present，不阻塞 CPU 约 1 秒；下一帧继续尝试。
- `Wait`、ReadPixels、Resize、Close、device lost recovery 继续 drain 全部；F/P target/spare/layer/instance/staging 只在相关提交退休后复用或释放。

A1 若在安全边界下没有真实帧率收益，也视为有效结论；下一步只有在证据证明需要更多资源并且用户重新批准后，才能设计 per-resource ring 或三缓冲。


## 6. 接口与生命周期决策

- `gpu.Device` 稳定接口不直接新增 fence 方法；Metal internal `Device` 和 presenter 使用实验性内部接口/类型。
- `Device.Wait()` 保持“等待所有已提交工作并返回错误”的明确语义；A 的普通 Execute 不调用它，改用内部 `waitOldest`。
- `ReadPixels`、Close、resize、device lost recovery 必须调用 `waitAll` 或等价 drain；不能返回仍引用即将释放纹理的图像。
- F/P spare target 只有在其上一次写入的 command buffer 完成后才能交换/释放；resize 先 drain 再释放 target/spare；Close 先停止新提交，再 drain，再释放 Objective-C 对象。
- `nextDrawable` 的阻塞时间单独计入 `drawableWait`，不放进 GPU duration；获取不到 drawable 的既有降级行为保持。
- 计时使用 monotonic `time.Now` 记录 CPU 区间；GPU 时间使用 `GPUEndTime`，不可用时为 unknown。

## 7. 测试策略

### A 红绿测试

- `TestMetalInFlightBounded`：连续提交 frame，峰值不超过 2，第三帧只等待 oldest。
- `TestMetalExecuteDoesNotDrainAll`：未到上限时 Execute 不调用 waitAll；显式 Wait 仍 drain all。
- `TestMetalErrorAfterDeferredCompletion`：command buffer error 在正确的观察点报告，pending/device-lost 语义不变。
- `TestMetalCopyTargetCannotReuseBeforeFence`：F/P target/spare 在未完成时拒绝复用或等待 fence。
- `TestMetalResizeCloseDrainsInflight`、`TestMetalPresentCloseDrainsDrawable`：resize/Close 无 dangling target/layer 引用。
- `-race` 覆盖 Device、presenter、diagnostics。

### B 红绿测试

- `TestMetalFrameTokenTimeline`：token 的 frame id、commit、GPU、present 字段顺序单调；unknown feedback 不伪造值。
- `TestMetalPresentDoesNotWaitForGPU`：present 的 encode 路径不隐式调用全量 wait，显式 drain 才等待。
- `TestMetalDrawableBackpressureMeasuredSeparately`：nextDrawable 阻塞只计入 drawable wait。
- `TestMetalTokenDrainAfterReadPixelsClose`：ReadPixels/Close 先 drain token，结果和错误正确。

## 8. 验收矩阵

- Metal package：普通测试、`-race`、严格像素/生命周期/F/P 全部通过。
- 编辑器：`BenchmarkTypeTS20k`、`BenchmarkScrollTS20k`、`BenchmarkScrollPartial`、`BenchmarkTypeAt1MLinesCPU`，至少 20 轮低负载 paired，记录 A/A 控制臂；不删除慢轮次。
- 真实窗口：仅在可确认窗口解锁、显示器、刷新率和应用前台状态时测；记录 drawable wait 和 GPU feedback 是否可用。
- Linux/Windows：`go test`、`go vet`、交叉编译；无 Metal 代码路径变化。
- 全仓：`go test -p 1 ./...`；race 失败必须列明，不能因为调度慢放宽产品门槛。

## 9. 风险

- GPU/Metal 是外部调度系统，A 可能改善 CPU queue latency 但不改善显示 p99；报告必须明确区分。
- CAMetalLayer 没有可靠的逐帧 `FrameDisplayed` identity 时，B 只能报告 drawable enqueue/back-pressure 和 GPU completion，不能声称光子级 latency。
- 增加 in-flight 会增加资源峰值；默认上限保持 2，若需要 3 必须单独给出原生纹理/内存证据。
