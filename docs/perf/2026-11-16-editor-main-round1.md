# 编辑器 main 分支性能：行读取与多光标 Enter

日期：2026-11-16。用户要求直接在本地 `main` 上继续，不建新 worktree；不推送。

## 1. 先集成已有优化

本地合并 `worktree-editor-input-scroll-perf`（合并提交 `81bd3a5`）：

- 小地图复用 token 查询 scratch 和 keep map，缓存行仍拥有独立 token 副本。
- 字符小地图去掉临时 cell 数组，保留 grapheme、颜色回调、黄金像素。
- Monarch 只在真实进入 backtracking 时创建 rune/index，RE2 不付这笔成本。

原报告：[输入/滚动分配优化](../specs/2026-11-16-editor-input-scroll-perf-report.md)。这些旧测量中的 TS20k 输入/滚动每帧分配字节约减少 26%/40%，不是本轮重测的整帧时延结果。该报告保留历史基线与 CI；其中“隔离分支尚未合并”的状态由本节更新。原报告尚未收窄的百万行滚动尾时延 CI，并不因合入而改判为通过。

本轮也修正此前留下的 scratch 引用问题：退出小地图准备时清理完整容量，而非最后一次查询的长度。`TestMinimapPreparationClearsScratchCapacity` 在修改前失败，修改后通过；仍不改变缓存行所有权。

## 2. CPU 热点与改动

在 10,000 行 Go 文本上，1,000 个光标同时 Enter 的 CPU profile 显示：`buffer.afterNewline` 及其逐个换行查找占约 27% 样本（包含测试构造、命令和同步高亮，不能解释为整个应用的占比）。

`Snapshot.Line` 原先顺序调用 `LineStart`、`lineEnd`、`slice`，会重复从根定位、重复扫描同一叶子里的换行。本轮改为内部 `lineAt`：

1. 在 rope 中定位该行开始所在的叶子，只定位一次。
2. 如果结束换行也在同一叶子，直接返回字符串切片。
3. 跨叶长行仍使用既有范围拼接；换行不在叶子时不做无用扫描。

没有增加常驻行索引、node 字段、缓存或公共 API；快照继续不可变，范围错误仍 panic；跨叶行仍需字符串拷贝。

测试覆盖：叶子末端换行、连续空行、最后空行/无换行末行、跨多叶长行、中文/emoji、非法 UTF-8、100 次编辑后的叶子分裂/合并/平衡，以及新旧快照逐行与 `strings.Split` 对照。

## 3. 多光标 Enter 的固定二进制对比

环境：Apple M4、darwin/arm64。10,000 行、1,000 光标，计时目标为 `Type("\n")`，线程固定并单列线程 CPU 时间。文档构建、State 创建、设置光标、预高亮和 Close 都不计时、不计分配。独立审查发现最初基准把公共文档构建计入了墙钟和分配计数：已加初始化计时必须为零的断言（先红后绿），在停止状态下 ResetTimer，再重建二进制重测；以下全部为修正后的结果。

before/after 基于同一份当前源码，**唯一产品差异为 Snapshot.Line 实现**。Go overlay 只把当前 `snapshot.go` 中的 Line 方法换回旧实现，不覆盖其他会话刚合入的 CRLF normalize 优化或 HTML lexer 改动。每组 8 轮交替先后顺序、每臂新进程、每进程 30 次操作，没有选最好一次。

### 固定工作量（Warm）

`BenchmarkEnterManyCursorsWarm` 在计时外将全部行高亮为 final，避免线程调度/查询变快改变 assistBudget 内能追上的高亮行数。各臂的分配次数相同。

| 指标（8 个进程中位数） | before | after |
|---|---:|---:|
| Enter 墙钟平均 | 8.53 ms/op | 7.46 ms/op |
| 调用线程 CPU 平均 | 7.79 ms/op | 6.95 ms/op |
| Go 分配字节 | 3,367,694 B/op | 3,365,302 B/op |
| Go 分配次数 | 19,907 allocs/op | 19,907 allocs/op |

配对 CPU 比值中位数 **0.883**，8 轮范围 **0.781–1.097**：该固定命令的计算成本点估计减少约 12%。测量时机器 1 分钟负载 **23.15–25.38**，不能当作正式低负载时延门槛，更不能当成屏幕输入 p95。未宣称整个编辑器提速 12%。

二进制 SHA-256：

- before：`b694f3bca3e9186fcf8f4559f6bfb25967b03b0db5035d4cbbcb4467b72a1cb6`
- after：`6aa4d195d7eeae09ba72b69d0dbb841d880e5b63db4dd180bf8d9f2d79c88bed`

### Stale 高亮压力场景

`BenchmarkEnterManyCursorsStale` 对应现有 `TestEnterManyCursorsStale`。修正初始化计时后，before/after 线程 CPU 中位数 **17.64 / 14.15ms**、配对 CPU 比值中位数 **0.781**；但分配次数 **59,232 / 67,139**，说明 assistBudget 内实际执行的高亮工作量不同。不能将约 22% 的比值解释为等工作量加速：它同时包含查询变快后多做的高亮/缩进工作。更早一次试测曾得到约 28% 的比值，因初始化计时和该工作量问题，已被本节数据取代，不作为最终收益结论。

## 4. 行读取微基准

`BenchmarkShortLineRead` 将旧的三遍查询作为测试中的 reference，与新实现放在同一二进制。每次随机步进到 10,000 行文档中的一行，不含加载。7 个样本、每个 200,000 次查询：

| 行宽（字节，含 LF） | previous 中位数 | combined 中位数 | 分配 |
|---|---:|---:|---|
| 10 | 1,139 ns/op | 475 ns/op | 均 0 B/op |
| 100 | 269 ns/op | 159 ns/op | 均约 10 B/op（仅跨叶行） |

该组按 benchmark 顺序采样，不是负载受控的低延迟 paired gate；只作为热点操作收益证据。

跨叶 3,000 字节长行另跑 8 轮交替、每臂 100,000 次，previous/combined 中位数分别为 **1,104 / 1,096 ns/op**，均 3,072 B/op、1 alloc/op。轮间抖动较大，不宣称长行提速；单轮明显受负载影响，不用它替代置信区间。

## 5. 验证与限制

- 合并旧优化时并行整包测试曾失败：`TestEnterManyCursorsStale` 为 223ms（门槛 50ms）；之后单独连续五次为 17–21ms。另一轮出现 `TestActiveGuideFirstCallFast` 14ms（门槛 5ms）和 Enter 55ms（门槛 50ms），当时负载约 29。均保留原门槛，没有删除测试。
- 随后完整 `go test -p 1 ./ui/editor/... -count=1` 所有包通过。
- `go vet ./ui/editor/...` 通过。
- `go test -race ./ui/editor/buffer -count=1` 通过。
- 新 lineAt 边界测试先编译红灯（缺少实现），实现后通过；小地图容量清理测试先断言红灯后通过。
- 全仓 `go test -p 1 ./...` 第一次仅 `TestActiveGuideFirstCallFast` 失败（9.90ms >5ms），其余 131 个包通过；门槛不改，完整重跑后退出码 0，全部通过（其中未变化的已通过包使用 Go 测试缓存）。
- `go test -race -p 1 ./ui/editor/buffer ./ui/editor/monarch ./ui/editor/regex -count=1` 全部通过；编辑器 `Minimap|Miniature|Enter|OnEnter|Bracket|ActiveGuide` 定向 race 全部通过。没有声称完整编辑器包 race 已重跑。
- Linux、Windows 的编辑器全部子包及 editor-demo `go vet` 通过；格式与 `git diff --check` 干净。
- 独立审查仅发现基准初始化计时问题，已加计时断言、先红后绿修复并重新测量；未报告行读取或小地图引用所有权问题。
- 没有测真实窗口输入 p95、Monaco/Windows/IME，也没有提高预算或降低既有门槛。

证据位于本地 ignored `.perf/editor-main/`：CPU/alloc profile、overlay、固定二进制、逐轮 Enter/跨叶行日志、负载采样、原始及同二进制微基准、完整测试日志。
