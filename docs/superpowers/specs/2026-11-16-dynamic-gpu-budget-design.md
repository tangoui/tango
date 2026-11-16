# 动态 GPU 资源预算化

## 目标

降低 Tango 默认自绘在 macOS 800×600 动态列表和网格场景中的常驻 physical footprint：

- 当前参考：动态列表约 136–137 MiB，600 格网格约 142–144 MiB；
- mygo 同批参考：列表约 116–122 MiB，网格约 110–127 MiB；
- 目标：同一场景、同一批次、同一口径下，Tango 每个场景 5 组中位数低于 mygo，并且有效样本最大值低于 mygo 有效样本最小值；
- 65 秒连续运行不出现持续增长；
- P99 更新间隔相对基线不恶化超过 10%。

## 非目标

- 第一阶段不改变默认渲染器选择，不扩大 native/CPU 自动切换范围；
- 不修改 WebView、mygo 或基准对手；
- 不通过强制 GC、FreeOSMemory、heap profile 或删除异常样本制造内存优势；
- 不牺牲 glyph 正确性、字体回退、窗口缩放和设备丢失恢复；
- 不为单一 benchmark 写场景特判。

## 当前证据

上一轮同批动态测试显示：

- Tango 默认大窗口报告 `renderer=gpu`；
- 主要成本疑似来自 GPU atlas、Metal image pool、frame/render target 和动态文字/布局缓存；
- 静态小窗口字体共享已把默认空闲成本显著降低，但动态大窗口仍高于 mygo；
- atlas 当前使用固定页大小和较高页数上限，Metal image pool 也会保留可复用纹理。

实现前必须用 probe/heap/vmmap/renderer counters 区分：

1. GPU atlas mask/path pages；
2. Metal image pool 与 render targets；
3. paint CPU glyph/image/shadow 缓存；
4. Go heap 中 Layout/Node/Flex/字符串和临时上传数据；
5. WindowServer/IOSurface 等不可由 Tango 回收的系统成本。

## 方案

### 方案 A：GPU 资源预算化（本阶段选择）

让 GPU renderer 按预算管理可重建资源：

- atlas 页数采用小窗口/大窗口不同上限，按真实 glyph 使用量逐步增长；
- image pool 设置物理字节预算和闲置帧阈值，超预算只释放未被当前 frame 引用的纹理；
- atlas/path cache 设置最近使用帧，淘汰只删除可重新上传的条目；
- Trim 操作保留当前 frame 所需资源，并在下一帧允许重新上传；
- 统计每类资源的当前字节、峰值、淘汰次数、重新上传次数和 evictions；
- 资源预算不由场景名称决定，而由逻辑窗口面积、scale、实际上传量和连续帧节奏决定。

如果设备不支持安全的局部淘汰，优先采用完整 atlas/image pool 重建，保证资源引用和 GPU command buffer 生命周期正确。

### 未选方案

- 大窗口自动切 CPU/native：可能降低内存，但会改变默认渲染策略并带来 CPU/P99 回退；作为 A 失败后的第二阶段。
- 只做 UI 增量重绘：网格 600 格每帧全变，不能解决主要 GPU residency；作为后续 CPU 预算优化。

## API/实现边界

优先修改：

- `internal/render/gpu/renderer.go`：资源预算状态、帧号、统计和 Trim/eviction 入口；
- `internal/render/gpu/atlas/`：按最近使用和页预算释放可重建 atlas 条目；
- `internal/render/gpu/metal/image_pool_darwin.go`：字节预算、idle frame 淘汰和计数器；
- 各平台 GPU image pool/texture 实现：保持非 Darwin 编译和行为契约；
- `uihost/gpu.go`：只转发内存回收/预算配置，不改变默认 renderer 选择；
- 测试：renderer/atlas/image pool 的 fake device 和计数器测试。

不允许新增公开 API；诊断数据只通过测试 hook、受控环境变量或已有 frame trace 输出提供。

## 资源生命周期规则

- 当前 command buffer、当前 frame、present target 引用的资源不得淘汰；
- 淘汰发生在 frame boundary，不能在 GPU 提交期间释放；
- 释放顺序必须先解除 atlas/image pool 的索引引用，再销毁 native texture；
- device lost、window close、scale change 和 Trim 后都必须能从空资源状态重新绘制；
- budget=0 只允许测试使用，生产默认预算必须为正；
- 淘汰失败不能破坏绘制：记录诊断并保留资源，不能返回伪成功。

## 分阶段验收

### 阶段 1：剖析基线

- 增加测试/诊断计数：atlas pages/bytes、image pool bytes/count、uploads、evictions、render target bytes；
- 用相同动态 list/grid 运行一轮，记录资源曲线和 physical footprint；
- 不改变渲染结果。

### 阶段 2：最小预算

- 先限制 image pool 闲置资源；
- 再限制 atlas 页增长；
- 每一步分别跑 renderer 单测、race 和视觉/像素回归；
- 如果 P99 或上传次数明显恶化，回退该单项而不是叠加更多策略。

### 阶段 3：同批复测

- 统一构建 Tango main 和 mygo；
- list/grid 各 5 组 AB/BA，窗口 800×600；
- 预热后每秒读 physical footprint，保留全部有效样本；
- 记录主进程/WebContent、CPU、更新间隔和资源计数；
- 各场景另跑 65 秒，检查头尾中位数；
- 目标未达成时如实报告，不改口径。

## 风险与回滚

- 任何字体缺失、atlas evictions 过多、P99 超过基线 10%、设备恢复失败都阻止合并；
- 预算策略必须可由单一配置开关关闭，关闭后恢复当前 main 行为；
- 先在独立分支提交，未经同批验证不推送 main。
