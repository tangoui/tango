# Metal 异步 Command Buffer 与 Present Fence 报告

日期：2026-11-16
分支：`worktree-metal-fp-latency`（基于 origin/main `fb03780`）
设计：`docs/specs/2026-11-16-metal-async-present-design.md`
计划：`docs/superpowers/plans/2026-11-16-metal-async-present.md`

## 1. 做了什么

| 提交 | 内容 |
|------|------|
| `65a42cb` | `submissionQueue`：按提交顺序退休的 command buffer 队列，替代 `[]id` |
| `516666e` | A1：Execute 不再 drain 全部；只退休 frame/readback 提交（它们引用的 instance/staging 是单份共享缓冲），present 提交可跨 Execute 保持 in-flight；frame+present 统一上限 2，present 满时只退休最老一个；同一帧 draw+present 的失败只计一次 |
| `39e868d` | B1：`Device.SetTimeline` 内部诊断，退休时报告种类、帧号、commit 时间、GPU start/end（不可得为 unknown）、present 的 `nextDrawable` 等待（与 GPU 时间分开）；拿不到 drawable 报告 miss |
| `de9f53e`、`aa1cc26` | `BenchmarkPresentLoop`：同一二进制两臂（`drain` = A1 之前行为，仅测试可开的字段；`a1`），全窗编辑器帧 3840×2160，Render+present 无 Wait，与 uihost 一致；含关闭 display sync 的场景 |

公共 API 不变：`gpu.Device`、uihost、非 darwin 签名都没改；非 darwin 只多了一个空的 `SetTimeline`。

## 2. 安全边界（为什么 A1 只能重叠 present）

A1 明确不做资源 ring。Execute 每帧都会改写单份共享的 instance buffer、staging buffer 以及 mask/color/ramp/image 纹理；GPU 还在读上一帧时覆盖它们会画错。所以 A1 在编码前仍然退休所有 frame 提交，只放开 present：present 只读 retained target 并写 drawable，Metal 同一队列按提交顺序执行，下一帧对 target 的写入天然排在 present 之后。

## 3. 测量

环境：Apple M4，离屏 `CAMetalLayer`（无窗口、无显示器关联），2x，编辑器帧（`editorList`，1920×1080 逻辑）。

| 场景（各 100 帧） | 臂 | p50 | p95 | p99 | frame GPU p50 | present GPU p50 | drawable 等待 p50 |
|---|---|---|---|---|---|---|---|
| display sync 开 | drain | 8.35 ms | 9.20 | 9.31 | 1.83 | 0.29 | 7.28 |
| display sync 开 | a1 | 8.32 ms | 8.95 | 9.26 | 1.83 | 0.29 | 7.30 |
| display sync 关 | drain | 8.35 ms | 9.12 | 9.46 | 1.83 | 0.28 | 7.36 |
| display sync 关 | a1 | 8.31 ms | 9.24 | 9.49 | 1.84 | 0.28 | 7.32 |

这些是单轮试跑，测量时 1 分钟负载 10–14（其他会话在跑测试），高于计划的 6 阈值。20 轮低负载配对脚本（`.perf/metal-async/present_ab.sh`，负载 ≥6 自动作废重跑）已就绪，见 §5。

## 4. 结论

- **A1 在这个场景下没有可测收益**：两臂 p50 差 <0.05 ms，在噪声内。
- **原因已用 B1 时间线定位**：每帧约 8.3 ms 中，`nextDrawable` 阻塞约 7.3 ms；frame GPU 1.8 ms、present GPU 0.3 ms。瓶颈是 drawable 供给节奏（约 120 Hz），不是 command buffer 完成等待。关闭 `displaySyncEnabled` 后离屏 layer 仍按约 8.3 ms 发 drawable，说明这是 Core Animation 对离屏 layer 的节奏，而不是 GPU。
- GPU 每帧总工作约 2.1 ms，远低于帧间隔，所以 CPU 原先 drain 时等的那段本来就短；A1 去掉的只是这 2 ms 内的部分重叠，被 7 ms 的 drawable 等待完全掩盖。
- 按设计，"A1 无显著收益"是有效结论；不据此增大上限或做三缓冲。

## 5. 未完成 / 未证明

- **与已批准决策的偏差 1：drawable 有界等待没有实现。** 设计 §5 要求 `nextDrawable` 超过上限就跳过 present、不阻塞约 1 秒。`allowsNextDrawableTimeout` 默认就是 YES，显式设置它不改变行为；`nextDrawable` 仍可能在持有 Device 锁时阻塞约 1 秒（窗口不可见、drawable 全部被占用时），这与 A1 之前相同。要有更短的上限，必须把 drawable 获取移出加锁的同步路径，属于新设计，未做。
- **与已批准决策的偏差 2：resize 不先 drain。** 设计 §6 写"resize 先 drain 再释放 target/spare"。实现里 present 仍在 GPU 上时，`ensureTextures`/`dropCopyTarget` 就会 `release` 旧 target。这是安全的：所有 command buffer 都用 `commandBuffer`（保留引用、默认 hazard tracking），GPU 用完前纹理不会真正释放。
- **审查修复**：最终审查发现失败计数两处回归（present 持续失败、draw 成功时设备永不判定丢失；已计过的旧失败在新帧提交后再次上报并清掉新帧的 `hasFrame`），已用 `TestMetalAlternatingFailuresLoseDevice`、`TestMetalStaleFailureKeepsNewFrame` 复现并修复。
- **20 轮低负载配对**：机器负载持续 >6，脚本在等负载降下来；结论目前基于单轮试跑。差值远小于噪声，预期不会改变结论，但未证明。
- **真实窗口**：没有在前台真实窗口测；离屏 layer 的 drawable 节奏不等于真实显示器。CAMetalLayer 没有逐帧"已显示"反馈，所以任何 "photon latency" 都 not proven。
- **编辑器 perfab ABAB**（`BenchmarkTypeTS20k` 等）：那套 harness 每帧显式 `Wait()`，A1 对它在定义上不改变行为（Wait 仍 drain 全部），所以没跑。
- device lost / resize / Close / ReadPixels / drawable 缺失由现有与新增测试覆盖，没有单独的性能场景。

## 6. 建议下一步

真正的延迟在 drawable 节奏上，不在 command buffer。值得做的是：在真实窗口里用 B1 时间线测 `nextDrawable` 等待，判断是否需要把 `nextDrawable` 移到编码之后（先编码帧，最后才取 drawable，缩短持有时间）或调整 `maximumDrawableCount`。这需要新的设计确认，不在本专项范围内。
