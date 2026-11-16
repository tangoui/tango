# UI 组件性能：基线与热点排名（Task 12a，2026-11-17）

本文是 UI 组件极限性能项目的结果文档。设计稿：[`docs/superpowers/specs/2026-11-16-ui-component-performance-design.md`](../superpowers/specs/2026-11-16-ui-component-performance-design.md)。

这一版只包含**进程内基线**（第一层）。跨框架基线（第二层，Task 12b）要在解锁的屏幕上运行，测量期间 macOS 会话屏幕锁定，所以没有运行。因此本文**不包含**任何“比 MyGo / NSGo 快”的结论，所有跨框架列都写“待测（屏幕锁定，见 12b）”。

## 方法

### 两层口径

- **第一层：进程内组件基准**（`ui` 包，`BenchmarkComponents/<组件>/<场景>`）。窗口 1440×900、Scale 2、`NoAnimations: true`、主题 `theme.AntDesign()`。渲染器为 no-op，计时覆盖构建、布局、绘制命令和 damage 指纹，不做光栅化。场景如下：
  - `Mount`：热缓存首次构建，每次迭代新建 Runner。
  - `MountCold`：新进程中第一次构建，只报告，不判定。
  - `Unchanged`：内容不变时强制调用一次 `Frame()`。
  - `Interact`：组件最常见的一次交互。
  - `Large1k` / `Large10k` / `Large100k`：集合类与可放大组件，n 取 1 000 / 10 000 / 100 000，默认交互是滚动。
- **第二层：进程外跨框架 runner**（`tools/uibench`）。发布构建、新进程，比较进程 CPU + WindowServer CPU、帧率、帧间隔 p99 和 footprint，至少 5 轮交替。判定“更快”要求 Tango 各轮中位数 ≤ 对手中位数 × 0.50，四分位区间不重叠，帧率不低于对手，p99 不高于对手；对 MyGo、NSGo 分别判定。**本版未运行。**

### 硬预算（只收紧不放宽）

| 场景 | 预算 | 门槛 |
|---|---|---|
| 首次构建 `Mount` | ≤ 0.5 ms；浮层类 ≤ 2 ms | `TestPerfBudgets` + `perfTimeDebt` |
| 交互帧 `Interact` | ≤ 2 ms | `TestPerfBudgets` + `perfTimeDebt` |
| gallery 整页交互帧 | ≤ 4 ms | `TestGalleryPageBudgets` + `galleryTimeDebt` |
| 10 万项交互帧 `Large100k` | ≤ 4 ms，且 ≤ 同组件 `Large1k` 的 1.5 倍 | `TestPerfBudgets` + `perfTimeDebt` |
| 交互帧脏区 | ≤ 2 × 内容面积（各部件矩形的并集面积） | `TestPerfDamage` + `perfDamageDebt` |
| 静止 | `Settle()` 后 `NeedsFrame()` 为 false（`Animated` 夹具除外） | `TestPerfIdle` |

时间门槛只在 `TANGO_PERF_BUDGET=1` 时运行。棘轮规则：超预算的场景必须登记在债表里，并写明实测值；已在预算内的场景不得留在债表里。

### 证据与环境

- 机器：Apple M4（Mac16,10），10 核，16 GB，macOS 26.6（25G72），Go 1.27.2 darwin/arm64。
- Tango：`5f115d300fe68054765fee1c2ce7a8bd498e2126`（分支 `perf/ui-components-20261116`）。
- 对手版本（供 12b 使用，本版未运行）：MyGo v0.9.0（自绘 `ui` 模式）；NSGo 源码 changelog 为 Unreleased，go tree SHA1 `ca0381d685af758f880c7775824adc38380b9ac7`。
- 原始数据在工作区的 `.superpowers/sdd/2026-11-16-ui-component-performance/baseline-12a/`（已 gitignore，不入库）：`bench-raw.txt`、`bench.json`、`status.md`、`hot.md`、`prof/`、`scratch{1,2}.txt`、`cold14.txt`、`gate.txt`、`load.log`。

### 运行与负载（1 分钟负载均 < 6）

| 步骤 | 时间 | 负载（前 → 后，1/5/15 分钟） |
|---|---|---|
| `BenchmarkComponents -count 3 -benchmem`（未拆分，880 项 × 3 次） | 01:45:36–02:50:05 | 1.32 1.26 1.30 → 1.92 1.80 1.93 |
| 预算测试（uiperf 内置一步，见下文“偏差”） | 02:50:05–02:51:15 | 1.92 → 2.25 |
| 冷启动（每组件一个新进程，`-benchtime 1x`） | 02:51:15–02:51:20 | 2.25 → 2.47 |
| `perfTime` 预算计时 第 1 遍 | 02:51:57–02:54:49 | 1.94 1.87 1.94 → 4.01 2.76 2.29 |
| `perfTime` 预算计时 第 2 遍 | 02:54:49–02:57:37 | 4.01 2.76 2.29 → 2.57 2.48 2.24 |
| 补 14 个冷启动值 | 02:58:00–02:58:01 | 2.19 2.40 2.22 → 2.19 2.40 2.22 |
| `uiperf hot` | 02:58:19–03:01:03 | 1.79 2.29 2.18 → 3.91 2.64 2.31 |
| 旧门槛（固定步数），按 R33 停掉，输出丢弃 | 03:01:53–03:05:28 | 2.40 2.41 2.24 → 2.17 2.38 2.28 |
| **门槛 1**（自适应 `perfTime`），PASS | 03:06:27–03:18:14 | 1.52 2.17 2.20 → 3.01 2.29 2.16 |
| **门槛 2**，PASS | 03:19:05–03:31:15 | 2.08 2.15 2.11 → 5.62 3.78 2.81 |
| 门槛 3，PASS，**受干扰，不采信** | 03:32:23–03:44:28 | 3.21 3.43 2.75 → 6.97 3.86 3.10 |

本任务的测量都是串行运行的。约 03:30 起，另一个会话启动了自己的 GUI 基准（`tango-scenes`、`mygo-scenes grid`，伴随 WindowServer / fontd 活动），所以门槛 2 结束时负载升到 5.62：它的后段（T 开头之后的组件）比门槛 1 慢，最多约 2.7 倍，但仍在 < 6 的范围内，而且全部判定与门槛 1 一致。门槛 3 结束时负载 6.97 ≥ 6，标为受干扰，不用于债表取值；它的判定与门槛 1、2 全部一致，也是 PASS。

门槛命令为 `TANGO_PERF_BUDGET=1 go test ./ui ./examples/ui-gallery -run 'Test(Perf|GalleryPage)Budgets' -count=1 -timeout 0 -v`。`-timeout 0` 只取消 go test 的默认超时，`-v` 用来留下 PERF 行。两个包会并行运行，gallery 测试（约 2.7 s）与 ui 的前几个组件（A 开头）重叠，那几项在门槛中的数值比基准中位数高 2–3.7 倍，但都远在预算内。

### 与 `uiperf bench` 的偏差（如实记录）

`uiperf bench` 按 Task 8 的实现会依次运行基准、预算测试和冷启动，然后写 `bench.json`。这次在本机实测有三处不能直接用：

1. **`go test` 默认 10 分钟超时。** 全量 `BenchmarkComponents -count 3` 实际耗时 64.5 分钟。所以这次用的是工具的临时副本（`/tmp`，不入库）：只给两次 `go test` 加了 `-timeout 0`，把原始输出落盘，并在预算测试失败时继续往下执行。基准参数、聚合方法（各项 3 次取中位数）、冷启动流程都没有改。`-bench 'BenchmarkComponents/.*'` 的第一段不锚定，所以 `BenchmarkComponentsRaster`（光栅化参考）也跑了 3 次，它只在 `bench-raw.txt` 里，不进 `bench.json`。
2. **预算门槛原本按固定步数计时，慢场景跑不完。** 旧版 `perfTime` 每个场景固定跑 3 遍 ×（20 步预热 + 200 步计时）。32 个非虚拟化 `Large100k` 场景每步加起来约 36 s，完整跑一次门槛约 7 小时，每个优化批次都要重跑，没法用。按裁定 R33，`perfTime`（`ui/perf_check_test.go`）改成与 `testing.B` 同一思路的自适应计时：
   - 预热步数取 max(1, min(20, 200 ms 内能跑完的步数))。
   - 计时至少 3 步，之后跑满 200 步或累计 1 s 即停，两者先到为准。`Mount` 仍以 50 步为上限。
   - 仍取 3 遍的中位数。步号在预热与计时之间连续，可逆交互的顺序不变。
   - 新增 `TestPerfTimeSlowStepBounded`：每步 300 ms 时，一遍在 3 s 内跑完，计时 3–4 步。
   - gallery 门槛的固定 20 + 50 帧循环同样改为自适应（预热 ≤ 20 帧且 ≤ 200 ms，计时 ≥ 3 帧，跑满 50 帧或 1 s 即停，计时帧号仍从 0 开始，滚动节奏不变）。

   每步 < 5 ms 的场景（Mount 为 < 20 ms）计时步数与旧版相同，预算值和判定规则都没有改，所以这次改动不放宽任何门槛。改动后完整门槛约 12 分钟。uiperf 内置的那次预算测试在债表为空时必然失败，所以停掉了（停掉前已输出 15 行 PERF，全部 ok）。另用一个不入库的临时测试按旧 `perfTime` 跑了两遍（跳过 `Large10k` 和 25 个基准中位数 ≥ 60 ms 的 `Large100k`），供 `uiperf hot` 合并预算用。债表最终**只采用门槛自身的实测值**。
3. **冷启动正则未逐段锚定。** `^BenchmarkComponents/Form/MountCold$` 按 `/` 分段后，`Form` 会匹配 `FormItem` 等名字，所以有 14 个组件（Form、Input、Item、List、Menu、ProTable、Row、Scroll、Select、Skeleton、Table、Tag、Text、TimePicker）没有冷启动值。这次用逐段锚定的正则在新进程中单独补测，例如 `ui.test -test.run '^$' -test.bench '^BenchmarkComponents$/^Form$/^MountCold$' -test.benchtime 1x`。工具缺陷另开任务修。

`uiperf hot` 用的是**原版工具**，读的 `bench.json` 由基准中位数加上临时测试（旧 `perfTime`）的预算合并而成，得出的超标集合（33 项）与门槛一致。另外，`hot` 解析 `pprof -top -cum` 时要求 flat 列带单位，flat 为 0 的纯调用者行（如 `(*Runner).Frame`、`(*Runner).layoutAndPaint`、`flexbox.Solve`）会被丢掉，所以热点表只含 flat > 0 的函数。这是第二个工具缺陷，也另行处理。

## 结果概要

- 夹具 198 个，基准项 880 个：Mount / MountCold / Unchanged 各 198，Interact 154，Large1k / 10k / 100k 各 44。
- 时间预算的判定以门槛 1、2 为准（两次结果一致；下面的时间是两次中较差的值）。有时间预算的场景共 396 个：
  - `Mount`：198 个，197 个达标，**1 个超标**（Image，660 µs，预算 500 µs）。
  - `Interact`：154 个，**全部达标**。最慢的是 Image，587 µs。
  - `Large100k`：44 个，12 个达标，**32 个超标**。达标的是 CodeBlock、JSONView、JSONViewSource、JSONViewValue、Listy、LogView、MultiSelect、Select、Table、VirtualList、VirtualSelect、VirtualTree。
- `perfTimeDebt`：**33 项**（Why 均为 `baseline`），取值是门槛 1、2 中较差的一次。LogView 全部场景都在预算内（门槛：Mount 87.0 µs，Interact 37.1 µs，Large100k 25.0 µs，预算 36.8 µs），按 R23 不登记。
- `galleryTimeDebt`：**0 项**。全部非变体页面的交互帧都 ≤ 4 ms：uiperf 内置的那次预算测试（旧固定 20 + 50 帧循环）和门槛 1、2（自适应循环）中，`TestGalleryPageBudgets` 均 PASS，且没有输出 GALLERY 超标行。门槛只在超标时打印页面耗时，所以本文没有逐页数值。
- `perfDamageDebt`：146 项（Task 5/6 生成，本任务未改），即大多数交互帧仍在重绘远大于内容的区域。
- **翻转项：无。** 门槛 1、2、3 与基准中位数（`-count 3`）对每一项的超标 / 达标判定都相同。离预算线最近的是 Listy/Large100k（门槛 34.9 / 35.6 µs，预算 46.1 / 47.0 µs，即 1.5 × Large1k，约 76%）和 VirtualTree/Large100k（66.9 / 66.4 µs，预算 98.8 / 99.3 µs）。这两项的预算由同一次运行的 Large1k 决定，有翻转风险，后续批次需要关注。
- 基准与门槛数值差异较大的两项：AutoComplete/Large100k 和 Mentions/Large100k 的基准中位数（1.39 s / 1.42 s）约为门槛值（702 ms / 716 ms）的 2 倍。原因是它们的第 0 步是“点击 + 输入以打开下拉并过滤 10 万项”，之后才是滚动：基准每次只跑 1–2 次迭代，把第 0 步算了进去；门槛把第 0 步放在预热里。两者都远超预算，判定不受影响。

## 逐组件状态表

时间为 `BenchmarkComponents -count 3` 的中位数（ns/op 折算），冷启动为新进程中 `MountCold` 的单次值。“脏区债”取自 `perfDamageDebt`（交互帧脏区 ÷ 内容面积，门槛 2×）。状态的含义：

- 有对手场景（现有 counter / list 场景，或设计稿列为直接可比、要在 T10/T11 建场景的组件），或者有任一硬预算未达标：记“待优化”。
- 其余：记“无对手只看预算”。

在 12b 跑完之前，没有组件能按跨框架口径记“完成”。进程内没有证据把任何差距归到渲染器（no-op 渲染器下渲染器不计时），所以本版没有“受限于渲染器（转交）”。“无对手”是暂定分类，以 T10/T11 核对出的交集表为准。

统计：待优化 154，无对手只看预算 44。

| 组件 | Mount | Unchanged | Interact | Large 1k / 10k / 100k | 冷启动 | 脏区债 | 进程内时间预算达标 | 跨框架场景 | 对 MyGo | 对 NSGo | 状态 |
|---|---:|---:|---:|---:|---:|---:|---|---|---|---|---|
| Addon | 45.5 µs | 2.8 µs | 3.8 µs | — | 7.39 ms | 27.72× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Affix | 56.8 µs | 8.7 µs | 27.5 µs | — | 12.56 ms | 5.62× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Alert | 45.2 µs | 2.8 µs | 2.9 µs | — | 11.39 ms | 10.34× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Anchor | 55.3 µs | 9.8 µs | 28.4 µs | — | 12.57 ms | 2.59× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| App | 44.2 µs | 1.9 µs | 3.1 µs | — | 9.04 ms | 18.00× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Approval | 50.0 µs | 5.3 µs | — | — | 10.76 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| AspectRatio | 44.7 µs | 1.6 µs | 1.6 µs | — | 10.42 ms | 14.40× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| AutoComplete | 45.0 µs | 1.5 µs | 10.3 µs | 1.17 ms / 62.09 ms / 1.39 s | 6.00 ms | 26.47× | 否（Large100k） | 计划 T10/T11 | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Avatar | 44.5 µs | 0.8 µs | 0.8 µs | — | 10.15 ms | 810.00× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| AvatarGroup | 48.3 µs | 3.7 µs | 3.7 µs | 149.5 µs / 1.56 ms / 18.12 ms | 10.14 ms | 316.41× | 否（Large100k） | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| BackTop | 53.2 µs | 6.5 µs | 28.4 µs | — | 12.81 ms | 5.59× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Badge | 45.1 µs | 1.3 µs | 1.3 µs | — | 7.24 ms | 810.00× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| BadgeRibbon | 47.3 µs | 2.7 µs | 2.7 µs | — | 11.83 ms | 7.39× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| BorderBeam | 52.4 µs | 8.1 µs | 8.1 µs | — | 10.17 ms | 33.75× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Breadcrumb | 50.1 µs | 4.8 µs | 12.5 µs | — | 10.13 ms | 21.16× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| BreadcrumbItems | 50.7 µs | 4.8 µs | 14.5 µs | — | 10.17 ms | 32.53× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Bubble | 55.8 µs | 5.5 µs | — | — | 9.17 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| BubbleList | 90.6 µs | 21.2 µs | — | 352.2 µs / 855.6 µs / 5.69 ms | 9.53 ms | — | 否（Large100k） | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Button | 45.2 µs | 1.2 µs | 1.7 µs | — | 9.10 ms | 454.10× | 是 | counter（12b 待测） | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| ButtonGroup | 47.4 µs | 2.7 µs | 3.7 µs | — | 9.15 ms | 210.08× | 是 | counter（12b 待测） | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Calendar | 104.7 µs | 47.2 µs | 47.1 µs | — | 10.85 ms | 14.06× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Card | 52.1 µs | 5.7 µs | 5.7 µs | — | 12.03 ms | 21.18× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Carousel | 57.8 µs | 5.1 µs | 16.6 µs | 1.40 ms / 14.79 ms / 243.53 ms | 10.49 ms | 4.66× | 否（Large100k） | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Cascader | 45.9 µs | 2.0 µs | 27.7 µs | 1.15 ms / 50.35 ms / 609.90 ms | 10.04 ms | 168.75× | 否（Large100k） | 计划 T10/T11 | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| CascaderPanel | 56.3 µs | 8.5 µs | 20.1 µs | 1.07 ms / 50.84 ms / 613.41 ms | 10.53 ms | 21.62× | 否（Large100k） | 计划 T10/T11 | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| CheckAll | 49.3 µs | 4.3 µs | 14.0 µs | 1.40 ms / 221.87 ms / 2.53 s | 10.44 ms | 17.90× | 否（Large100k） | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| CheckCard | 47.4 µs | 2.4 µs | — | — | 9.21 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| CheckCardGroup | 44.1 µs | 0.7 µs | — | — | 108.9 µs | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| CheckableTag | 44.4 µs | 1.1 µs | 3.3 µs | — | 9.08 ms | 1042.64× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Checkbox | 45.2 µs | 1.2 µs | 4.0 µs | — | 8.86 ms | 725.59× | 是 | 计划 T10/T11 | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| CheckboxGroup | 48.1 µs | 3.2 µs | 9.0 µs | 1.41 ms / 220.43 ms / 2.53 s | 7.37 ms | 200.98× | 否（Large100k） | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| CodeBlock | 115.3 µs | 3.9 µs | 4.1 µs | 6.2 µs / 6.2 µs / 6.2 µs | 12.38 ms | 5.50× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Collapse | 53.6 µs | 6.3 µs | 17.8 µs | 2.56 ms / 138.44 ms / 1.72 s | 10.21 ms | 10.55× | 否（Large100k） | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| CollapsibleSider | 48.3 µs | 2.7 µs | 11.2 µs | — | 10.45 ms | 5.33× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| ColorPicker | 46.0 µs | 1.8 µs | 9.9 µs | — | 7.58 ms | 18.35× | 是 | 计划 T10/T11 | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| ColorPickerGradient | 46.7 µs | 2.3 µs | 16.9 µs | — | 7.94 ms | 13.88× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| ColorPickerPanel | 60.1 µs | 11.7 µs | 33.4 µs | — | 10.43 ms | 15.13× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| ColorPickerPanelGradient | 71.8 µs | 22.2 µs | 57.3 µs | — | 10.68 ms | 12.56× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Command | 55.5 µs | 8.0 µs | 22.3 µs | 696.6 µs / 66.43 ms / 768.46 ms | 11.33 ms | 11.14× | 否（Large100k） | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Compact | 50.9 µs | 4.9 µs | 6.1 µs | — | 9.38 ms | 208.68× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| ConfigProvider | 46.6 µs | 1.7 µs | 1.7 µs | — | 9.30 ms | 284.91× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Confirm | 58.6 µs | 8.1 µs | 38.0 µs | — | 11.70 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| ContextMenuAt | 46.9 µs | 2.2 µs | 18.9 µs | — | 9.66 ms | 5.34× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| ContextMenuAtWith | 46.2 µs | 1.3 µs | 15.2 µs | — | 9.01 ms | 18.24× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Conversations | 48.0 µs | 2.4 µs | — | 1.60 ms / 56.58 ms / 694.70 ms | 9.14 ms | — | 否（Large100k） | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Countdown | 45.6 µs | 1.3 µs | 1.5 µs | — | 10.22 ms | 16.05× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Custom | 46.0 µs | 1.5 µs | 1.5 µs | — | 9.31 ms | 14.55× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| DatePicker | 45.9 µs | 1.3 µs | 28.6 µs | — | 9.01 ms | 13.24× | 是 | 计划 T10/T11 | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| DatePickerPanel | 83.1 µs | 29.0 µs | 47.5 µs | — | 12.87 ms | 16.07× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| DateTimePicker | 45.9 µs | 1.5 µs | 72.7 µs | — | 7.44 ms | 8.51× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| DefaultFooter | 44.3 µs | 0.4 µs | — | — | 109.0 µs | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| Descriptions | 64.3 µs | 14.8 µs | 14.8 µs | 2.83 ms / 166.34 ms / 2.07 s | 11.71 ms | 6.13× | 否（Large100k） | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Disabled | 45.1 µs | 1.2 µs | 1.2 µs | — | 9.30 ms | 391.36× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Div | 45.6 µs | 1.1 µs | 1.1 µs | — | 10.26 ms | 40.50× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Divider | 45.7 µs | 0.9 µs | 0.9 µs | — | 10.15 ms | 42.32× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Dock | 50.2 µs | 4.5 µs | — | — | 9.39 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| Drawer | 51.2 µs | 4.7 µs | 15.4 µs | — | 11.75 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| DrawerForm | 62.0 µs | 11.9 µs | — | — | 10.73 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| DropZone | 45.8 µs | 1.3 µs | 1.7 µs | — | 9.23 ms | 20.77× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Dropdown | 47.2 µs | 1.8 µs | 12.0 µs | — | 9.18 ms | 49.44× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| EditableProTable | 73.9 µs | 21.8 µs | — | — | 12.02 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| Empty | 50.4 µs | 5.6 µs | 5.7 µs | — | 10.14 ms | 5.23× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| FieldSet | 54.7 µs | 7.3 µs | 31.0 µs | — | 10.54 ms | 29.09× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Flex | 48.4 µs | 3.2 µs | 3.2 µs | — | 10.44 ms | 84.38× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| FloatButton | 46.4 µs | 1.5 µs | 4.6 µs | — | 136.6 µs | 810.00× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| FloatButtonGroup | 45.9 µs | 1.8 µs | 7.9 µs | — | 129.1 µs | 213.16× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| FooterToolbar | 46.6 µs | 2.1 µs | — | — | 9.37 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| Form | 48.0 µs | 2.7 µs | 4.1 µs | — | 19.38 ms | 29.09× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| GridContent | 46.3 µs | 1.5 µs | — | — | 9.13 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| GridRow | 50.5 µs | 4.1 µs | 4.1 µs | — | 10.32 ms | 42.32× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| HotkeyInput | 48.8 µs | 3.9 µs | 15.8 µs | — | 7.71 ms | 155.77× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Icon | 44.9 µs | 0.8 µs | 0.8 µs | — | 129.7 µs | 2250.00× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| IconButton | 45.4 µs | 0.9 µs | 1.3 µs | — | 134.9 µs | 2677.69× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Image | 654.0 µs | 0.9 µs | 586.2 µs | — | 707.4 µs | 33.75× | 否（Mount） | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| InfiniteScroll | 67.2 µs | 17.3 µs | 218.2 µs | 2.43 ms / 70.78 ms / 867.66 ms | 10.44 ms | 10.38× | 否（Large100k） | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Input | 46.4 µs | 1.5 µs | 5.0 µs | — | 12.78 ms | 112.50× | 是 | 计划 T10/T11 | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| InputNumber | 47.2 µs | 1.9 µs | 8.0 µs | — | 7.43 ms | 184.09× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| InputTag | 48.8 µs | 3.8 µs | 21.6 µs | 5.39 ms / 192.73 ms / 2.25 s | 7.39 ms | 112.50× | 否（Large100k） | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Item | 47.5 µs | 2.1 µs | 8.5 µs | — | 11.74 ms | 11.64× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| ItemGroup | 49.4 µs | 4.1 µs | 16.4 µs | — | 10.66 ms | 5.82× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| JSONView | 73.9 µs | 17.2 µs | 27.0 µs | 20.5 µs / 27.8 µs / 20.8 µs | 10.95 ms | 5.33× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| JSONViewSource | 74.4 µs | 17.3 µs | 27.0 µs | 20.5 µs / 27.6 µs / 20.7 µs | 10.80 ms | 5.33× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| JSONViewValue | 73.6 µs | 17.3 µs | 26.2 µs | 35.7 µs / 47.5 µs / 30.0 µs | 10.63 ms | 5.33× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Kbd | 45.5 µs | 1.0 µs | 1.0 µs | — | 7.43 ms | 1592.02× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Layout | 50.2 µs | 4.1 µs | 4.1 µs | — | 10.38 ms | 4.74× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| LightFilter | 49.2 µs | 3.0 µs | — | — | 9.22 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| Link | 45.4 µs | 1.0 µs | 1.5 µs | — | 9.88 ms | 42.32× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| List | 61.3 µs | 12.5 µs | 36.5 µs | 2.60 ms / 78.32 ms / 1.02 s | 9.53 ms | 3.75× | 否（Large100k） | 计划 T10/T11 | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| ListActions | 66.6 µs | 16.3 µs | 43.1 µs | — | 10.82 ms | 4.31× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Listy | 68.3 µs | 13.3 µs | 41.1 µs | 30.9 µs / 31.2 µs / 33.7 µs | 11.95 ms | 2.59× | 是 | list（12b 待测） | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Loading | 47.9 µs | 2.6 µs | 2.1 µs | — | 10.06 ms | 7.76× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| LogView | 87.4 µs | 13.3 µs | 37.1 µs | 24.6 µs / 24.5 µs / 24.6 µs | 12.02 ms | 4.22× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| LoginForm | 61.8 µs | 12.0 µs | — | — | 9.25 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| LoginFormPage | 63.2 µs | 13.4 µs | — | — | 9.31 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| Markdown | 81.6 µs | 21.4 µs | 22.7 µs | 8.45 ms / 95.22 ms / 1.08 s | 13.18 ms | 9.00× | 否（Large100k） | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| MarkdownEditor | 69.7 µs | 20.2 µs | 64.6 µs | — | 10.39 ms | 4.63× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Masonry | 78.7 µs | 24.3 µs | 24.4 µs | 64.0 µs / 99.1 µs / 441.0 µs | 12.92 ms | — | 否（Large100k） | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Mentions | 46.5 µs | 1.7 µs | 6.8 µs | 1.24 ms / 63.85 ms / 1.42 s | 7.22 ms | 112.50× | 否（Large100k） | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Menu | 51.6 µs | 4.6 µs | 13.4 µs | 508.5 µs / 5.59 ms / 91.04 ms | 9.32 ms | 18.24× | 否（Large100k） | 计划 T10/T11 | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Menubar | 48.3 µs | 2.8 µs | 15.0 µs | — | 9.11 ms | 18.40× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Message | 53.2 µs | 5.0 µs | 13.6 µs | — | 10.14 ms | 27.17× | 是 | 计划 T10/T11 | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| MessageBox | 57.4 µs | 6.9 µs | 32.2 µs | — | 11.82 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| Modal | 51.9 µs | 6.0 µs | 8.1 µs | — | 11.66 ms | — | 是 | 计划 T10/T11 | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| ModalForm | 62.2 µs | 12.2 µs | — | — | 10.83 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| MultiDatePicker | 47.1 µs | 1.9 µs | 29.6 µs | — | 9.59 ms | 14.57× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| MultiSelect | 48.3 µs | 2.4 µs | 19.8 µs | 16.1 µs / 16.1 µs / 16.2 µs | 9.09 ms | 17.46× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| NativeView | 45.1 µs | 0.7 µs | — | — | 131.2 µs | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| Notification | 55.8 µs | 6.6 µs | 13.4 µs | — | 10.23 ms | 17.40× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| NotificationCenter | 46.8 µs | 2.2 µs | 34.4 µs | — | 7.37 ms | 11.68× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| OTP | 51.0 µs | 4.5 µs | 13.2 µs | — | 6.06 ms | 174.57× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Overlay | 47.8 µs | 2.9 µs | 7.4 µs | — | 10.37 ms | 64.06× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| PageContainer | 49.4 µs | 3.9 µs | — | — | 10.83 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| PageHeader | 43.9 µs | 0.5 µs | — | — | 98.9 µs | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| Pagination | 52.7 µs | 6.5 µs | 18.3 µs | — | 9.24 ms | 29.09× | 是 | 计划 T10/T11 | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Paragraph | 45.0 µs | 1.0 µs | 1.0 µs | — | 10.71 ms | 40.91× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Popconfirm | 54.3 µs | 7.6 µs | 5.1 µs | — | 10.70 ms | 55.97× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Popover | 50.1 µs | 3.9 µs | 3.0 µs | — | 11.71 ms | 64.22× | 是 | 计划 T10/T11 | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| ProCard | 47.7 µs | 2.8 µs | — | — | 10.64 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| ProDescriptions | 51.7 µs | 5.1 µs | — | — | 10.44 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| ProFieldOf | 44.2 µs | 0.5 µs | — | — | 108.5 µs | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| ProForm | 57.2 µs | 8.4 µs | — | — | 18.77 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| ProFormFieldSet | 45.5 µs | 1.3 µs | — | — | 9.25 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| ProFormGroup | 47.8 µs | 2.7 µs | — | — | 10.71 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| ProFormItems | 52.8 µs | 5.9 µs | — | — | 9.22 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| ProLayout | 52.3 µs | 5.4 µs | — | — | 11.85 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| ProList | 56.2 µs | 7.7 µs | — | 3.89 ms / 139.50 ms / 1.78 s | 152.9 µs | — | 否（Large100k） | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| ProSettingsProvider | 182.7 µs | 73.1 µs | — | — | 9.40 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| ProSkeleton | 43.8 µs | 0.5 µs | — | — | 109.2 µs | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| ProTable | 107.9 µs | 41.5 µs | — | 1.86 ms / 21.22 ms / 277.77 ms | 13.37 ms | — | 否（Large100k） | 计划 T10/T11 | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Progress | 57.2 µs | 12.1 µs | 13.5 µs | — | 7.60 ms | 19.53× | 是 | 计划 T10/T11 | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Prompt | 58.8 µs | 7.9 µs | 44.0 µs | — | 11.91 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| Prompts | 47.4 µs | 2.2 µs | — | — | 9.20 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| QRCode | 89.0 µs | 6.1 µs | 6.3 µs | — | 251.8 µs | 50.62× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| QueryFilter | 55.4 µs | 7.8 µs | — | — | 9.60 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| Radio | 47.4 µs | 2.4 µs | 8.7 µs | — | 9.09 ms | 362.80× | 是 | 计划 T10/T11 | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| RadioGroup | 48.5 µs | 3.1 µs | 8.6 µs | 1.46 ms / 218.81 ms / 2.51 s | 7.47 ms | 200.98× | 否（Large100k） | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| RangePicker | 46.5 µs | 1.6 µs | 49.9 µs | — | 10.29 ms | 7.64× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Rate | 46.5 µs | 1.9 µs | 1.9 µs | — | 7.32 ms | 366.75× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Result | 50.3 µs | 4.7 µs | 4.8 µs | — | 10.24 ms | 6.62× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| RichText | 55.1 µs | 10.8 µs | 10.9 µs | 12.66 ms / 180.31 ms / 2.02 s | 14.05 ms | 89.01× | 否（Large100k） | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| RouteContext | 46.3 µs | 1.2 µs | — | — | 8.84 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| Row | 49.2 µs | 3.7 µs | 3.8 µs | — | 10.58 ms | 36.16× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| SchemaForm | 56.8 µs | 8.4 µs | — | — | 9.28 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| Scroll | 60.5 µs | 11.6 µs | 18.9 µs | 328.1 µs / 3.42 ms / 50.35 ms | 10.33 ms | 6.33× | 否（Large100k） | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| ScrollH | 59.5 µs | 10.7 µs | 18.1 µs | 326.5 µs / 3.42 ms / 50.78 ms | 10.37 ms | 16.88× | 否（Large100k） | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Search | 48.1 µs | 2.7 µs | 9.7 µs | — | 9.31 ms | 155.77× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Segmented | 48.4 µs | 3.0 µs | 8.7 µs | 855.7 µs / 92.97 ms / 1.11 s | 10.27 ms | 208.36× | 否（Large100k） | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Select | 46.4 µs | 1.5 µs | 5.9 µs | 15.1 µs / 15.1 µs / 15.1 µs | 9.09 ms | 27.00× | 是 | 计划 T10/T11 | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Sender | 48.7 µs | 3.1 µs | 21.6 µs | — | 9.15 ms | 11.35× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| SettingDrawer | 77.4 µs | 24.4 µs | — | — | 12.14 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| Skeleton | 48.2 µs | 2.4 µs | 2.6 µs | — | 145.6 µs | 17.31× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| SkeletonAvatar | 45.1 µs | 0.7 µs | 0.7 µs | — | 106.0 µs | 810.00× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| SkeletonButton | 44.4 µs | 0.7 µs | 0.7 µs | — | 124.8 µs | 632.81× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| SkeletonImage | 44.7 µs | 0.8 µs | 0.8 µs | — | 131.2 µs | 105.47× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| SkeletonInput | 44.7 µs | 0.7 µs | 0.7 µs | — | 107.5 µs | 162.00× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Slider | 47.9 µs | 3.1 µs | 30.4 µs | — | 7.49 ms | 21.16× | 是 | 计划 T10/T11 | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| SliderRange | 47.7 µs | 2.8 µs | 17.9 µs | — | 130.1 µs | 42.32× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Space | 48.7 µs | 3.4 µs | 3.4 µs | — | 10.46 ms | 42.32× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Spin | 46.5 µs | 1.7 µs | 1.7 µs | — | 10.38 ms | 190.54× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Splitter | 48.8 µs | 2.8 µs | 13.2 µs | — | 10.35 ms | 8.91× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| StackBox | 46.1 µs | 1.1 µs | 1.1 µs | — | 10.33 ms | 33.75× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Statistic | 45.8 µs | 1.3 µs | 1.4 µs | — | 10.14 ms | 16.05× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| StatisticCard | 48.9 µs | 3.5 µs | — | — | 9.15 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| StatisticCardGroup | 46.2 µs | 1.5 µs | — | — | 9.43 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| StatisticTimer | 46.2 µs | 1.4 µs | 1.5 µs | — | 10.38 ms | 16.05× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| StatusBar | 46.9 µs | 1.9 µs | — | — | 7.60 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| StatusBarItem | 45.1 µs | 1.1 µs | — | — | 7.55 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| Steps | 50.2 µs | 4.6 µs | 11.8 µs | — | 10.37 ms | 17.24× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| StepsForm | 44.8 µs | 0.7 µs | — | — | 137.8 µs | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| Switch | 46.6 µs | 2.0 µs | 5.1 µs | — | 9.19 ms | 502.69× | 是 | 计划 T10/T11 | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| SwitchValue | 46.1 µs | 2.0 µs | 5.2 µs | — | 7.61 ms | 545.38× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Table | 182.2 µs | 112.0 µs | 195.6 µs | 94.5 µs / 94.5 µs / 94.4 µs | 12.39 ms | — | 是 | 计划 T10/T11 | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Tabs | 52.9 µs | 6.2 µs | 17.9 µs | 2.30 ms / 369.16 ms / 4.44 s | 9.14 ms | 20.24× | 否（Large100k） | 计划 T10/T11 | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Tag | 44.5 µs | 1.1 µs | 1.1 µs | — | 10.21 ms | 467.47× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Text | 45.9 µs | 0.9 µs | 0.9 µs | — | 10.26 ms | 42.32× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| TextArea | 48.5 µs | 2.5 µs | 9.3 µs | — | 10.52 ms | 30.86× | 是 | 计划 T10/T11 | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| ThoughtChain | 65.9 µs | 17.9 µs | 46.0 µs | — | 11.66 ms | 7.63× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| TimePanel | 140.9 µs | 44.2 µs | 80.1 µs | — | 7.29 ms | 33.83× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| TimePicker | 46.3 µs | 1.5 µs | 47.1 µs | — | 9.25 ms | 26.01× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| TimeRangePicker | 47.5 µs | 1.6 µs | 89.5 µs | — | 10.78 ms | 10.65× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| TimeSelect | 47.8 µs | 2.9 µs | 16.1 µs | — | 7.41 ms | 28.12× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Timeline | 72.3 µs | 24.3 µs | 24.5 µs | 3.12 ms / 107.57 ms / 1.37 s | 10.50 ms | 3.88× | 否（Large100k） | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Title | 45.3 µs | 0.9 µs | 0.9 µs | — | 10.64 ms | 24.50× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| TitleBar | 72.7 µs | 26.5 µs | — | — | 7.38 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| Toggle | 46.4 µs | 1.2 µs | 3.7 µs | — | 9.09 ms | 908.20× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| ToggleGroup | 49.2 µs | 3.4 µs | 9.5 µs | 881.6 µs / 215.43 ms / 2.48 s | 7.52 ms | 238.92× | 否（Large100k） | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| ToolCall | 55.4 µs | 9.5 µs | — | — | 10.40 ms | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |
| Tooltip | 48.2 µs | 3.1 µs | 2.6 µs | — | 10.18 ms | 143.33× | 是 | 计划 T10/T11 | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Tour | 55.1 µs | 9.2 µs | 30.6 µs | — | 10.66 ms | — | 是 | 计划 T10/T11 | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Transfer | 61.1 µs | 11.7 µs | 57.6 µs | 337.3 µs / 3.45 ms / 43.67 ms | 10.74 ms | 12.27× | 否（Large100k） | 计划 T10/T11 | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Tree | 76.4 µs | 20.6 µs | 79.9 µs | 4.45 ms / 75.68 ms / 1.09 s | 7.67 ms | 4.16× | 否（Large100k） | 计划 T10/T11 | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| TreeSelect | 47.4 µs | 2.0 µs | 23.1 µs | 43.9 µs / 441.6 µs / 5.82 ms | 9.19 ms | 168.75× | 否（Large100k） | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Upload | 49.5 µs | 3.2 µs | 13.1 µs | — | 10.58 ms | 10.35× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| VirtualList | 67.4 µs | 16.2 µs | 52.1 µs | 39.3 µs / 39.0 µs / 39.0 µs | 10.55 ms | 8.44× | 是 | list（12b 待测） | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| VirtualSelect | 66.8 µs | 2.0 µs | 24.1 µs | 15.7 µs / 15.8 µs / 15.8 µs | 10.42 ms | 168.75× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| VirtualTree | 68.1 µs | 15.7 µs | 62.8 µs | 63.2 µs / 64.1 µs / 64.0 µs | 7.33 ms | 2.91× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| Watermark | 47.9 µs | 2.8 µs | 2.8 µs | — | 10.39 ms | 6.33× | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 待优化 |
| WindowControls | 70.7 µs | 25.0 µs | — | — | 158.8 µs | — | 是 | — | 待测（屏幕锁定，见 12b） | 待测（屏幕锁定，见 12b） | 无对手只看预算 |

## 热点表（`hot.md` 前 30）

得分 = Σ（该函数在某超标场景 profile 中的 cum% × 该场景超出预算的倍数），按 `uiperf hot` 原样计算。超标场景共 33 个（32 个 `Large100k` 加 Image/Mount），每个用 `-benchtime=2s -cpuprofile` 采样。这些场景超出预算几十到上千倍，所以得分由 `Large100k` 主导。表中只有 flat > 0 的函数，原因见上文“偏差”。

| # | 函数 | 得分 | 组件数 | 组件/场景 | 非目标区 |
|---:|---|---:|---:|---|:---:|
| 1 | `github.com/tangoui/tango/ui/internal/flexbox.flex` | 8985.3737 | 21 | AutoComplete/Large100k, Cascader/Large100k, CascaderPanel/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Collapse/Large100k, Command/Large100k, Conversations/Large100k, InfiniteScroll/Large100k, InputTag/Large100k, List/Large100k, Markdown/Large100k, Mentions/Large100k, ProList/Large100k, RadioGroup/Large100k, Scroll/Large100k, ScrollH/Large100k, Segmented/Large100k, Tabs/Large100k, Timeline/Large100k, ToggleGroup/Large100k | 否 |
| 2 | `github.com/tangoui/tango/ui/text.layoutRuns` | 6970.5731 | 15 | Cascader/Large100k, CascaderPanel/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Collapse/Large100k, Command/Large100k, Conversations/Large100k, Descriptions/Large100k, InfiniteScroll/Large100k, InputTag/Large100k, ProList/Large100k, RadioGroup/Large100k, Segmented/Large100k, Tabs/Large100k, ToggleGroup/Large100k | 否 |
| 3 | `github.com/tangoui/tango/ui/internal/flexbox.measure` | 6765.8515 | 15 | AutoComplete/Large100k, Cascader/Large100k, CascaderPanel/Large100k, CheckAll/Large100k, Collapse/Large100k, Command/Large100k, InfiniteScroll/Large100k, List/Large100k, Markdown/Large100k, Mentions/Large100k, ProList/Large100k, RadioGroup/Large100k, Tabs/Large100k, Timeline/Large100k, ToggleGroup/Large100k | 否 |
| 4 | `github.com/tangoui/tango/ui/text.(*compactShaper).append` | 5730.0475 | 15 | AutoComplete/Large100k, Cascader/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Command/Large100k, Conversations/Large100k, Descriptions/Large100k, InputTag/Large100k, Mentions/Large100k, ProList/Large100k, RadioGroup/Large100k, RichText/Large100k, Segmented/Large100k, Tabs/Large100k, ToggleGroup/Large100k | 否 |
| 5 | `github.com/tangoui/tango/ui/text.Shape` | 4645.0030 | 10 | Cascader/Large100k, CascaderPanel/Large100k, CheckboxGroup/Large100k, Conversations/Large100k, Descriptions/Large100k, InputTag/Large100k, Markdown/Large100k, RadioGroup/Large100k, RichText/Large100k, ToggleGroup/Large100k | 否 |
| 6 | `github.com/tangoui/tango/ui/internal/flexbox.measureUncached` | 4409.1442 | 10 | CheckboxGroup/Large100k, Collapse/Large100k, Conversations/Large100k, InfiniteScroll/Large100k, List/Large100k, Mentions/Large100k, ProList/Large100k, RadioGroup/Large100k, Segmented/Large100k, Tabs/Large100k | 否 |
| 7 | `github.com/tangoui/tango/ui/text.prepareText` | 4257.0390 | 7 | Cascader/Large100k, CheckboxGroup/Large100k, Command/Large100k, Mentions/Large100k, Segmented/Large100k, Tabs/Large100k, ToggleGroup/Large100k | 否 |
| 8 | `github.com/go-text/typesetting/harfbuzz.(*otApplyContext).applyForward` | 3341.6358 | 9 | CascaderPanel/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Command/Large100k, InputTag/Large100k, RadioGroup/Large100k, RichText/Large100k, Segmented/Large100k, ToggleGroup/Large100k | 否 |
| 9 | `github.com/go-text/typesetting/harfbuzz.(*otApplyContext).applyString` | 2864.7797 | 7 | Cascader/Large100k, CascaderPanel/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, RadioGroup/Large100k, Segmented/Large100k, ToggleGroup/Large100k | 否 |
| 10 | `github.com/tangoui/tango/ui/internal/flexbox.arrange` | 2720.9565 | 11 | Cascader/Large100k, Command/Large100k, InfiniteScroll/Large100k, List/Large100k, Markdown/Large100k, ProList/Large100k, Scroll/Large100k, ScrollH/Large100k, Segmented/Large100k, Tabs/Large100k, Timeline/Large100k | 否 |
| 11 | `github.com/go-text/typesetting/harfbuzz.(*otMap).apply` | 2632.3651 | 8 | Cascader/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Command/Large100k, InputTag/Large100k, RadioGroup/Large100k, RichText/Large100k, Segmented/Large100k | 否 |
| 12 | `github.com/tangoui/tango/ui/text.shapeItems` | 2500.0638 | 7 | Cascader/Large100k, CascaderPanel/Large100k, Collapse/Large100k, Descriptions/Large100k, ProList/Large100k, Tabs/Large100k, ToggleGroup/Large100k | 否 |
| 13 | `github.com/tangoui/tango/ui/internal/flexbox.measureAxis` | 2092.7610 | 7 | Cascader/Large100k, Collapse/Large100k, List/Large100k, Mentions/Large100k, ProList/Large100k, Scroll/Large100k, Tabs/Large100k | 否 |
| 14 | `github.com/go-text/typesetting/harfbuzz.(*otLayoutLookupAccelerator).apply (inline)` | 1418.8184 | 3 | RadioGroup/Large100k, RichText/Large100k, ToggleGroup/Large100k | 否 |
| 15 | `github.com/tangoui/tango/ui.(*TabsW).tab.func10` | 913.3738 | 1 | Tabs/Large100k | 否 |
| 16 | `github.com/tangoui/tango/ui.(*TabsW).tab.func2` | 911.6732 | 1 | Tabs/Large100k | 否 |
| 17 | `github.com/tangoui/tango/ui/text.Width (inline)` | 880.3567 | 1 | CheckAll/Large100k | 否 |
| 18 | `github.com/go-text/typesetting/harfbuzz.(*otContext).positionComplex` | 618.7104 | 2 | RadioGroup/Large100k, RichText/Large100k | 否 |
| 19 | `github.com/go-text/typesetting/harfbuzz.(*shaperOpentype).shape` | 580.7211 | 3 | Cascader/Large100k, CascaderPanel/Large100k, Segmented/Large100k | 否 |
| 20 | `runtime.gcDrain` | 501.9638 | 10 | AvatarGroup/Large100k, Conversations/Large100k, Descriptions/Large100k, InfiniteScroll/Large100k, Scroll/Large100k, ScrollH/Large100k, Timeline/Large100k, Transfer/Large100k, Tree/Large100k, TreeSelect/Large100k | 否 |
| 21 | `github.com/tangoui/tango/ui.(*CommandW).item.func1` | 476.8034 | 1 | Command/Large100k | 否 |
| 22 | `github.com/tangoui/tango/ui.msOptionList.func1.1.1` | 458.0272 | 1 | Mentions/Large100k | 否 |
| 23 | `runtime.madvise` | 408.9180 | 7 | Descriptions/Large100k, Image/Mount, InfiniteScroll/Large100k, List/Large100k, Masonry/Large100k, ProList/Large100k, Timeline/Large100k | 否 |
| 24 | `github.com/go-text/typesetting/harfbuzz.applicable.apply` | 394.6901 | 2 | InputTag/Large100k, RichText/Large100k | 否 |
| 25 | `github.com/tangoui/tango/ui.(*RichTextW).doLayout` | 300.9932 | 2 | Markdown/Large100k, RichText/Large100k | 否 |
| 26 | `github.com/tangoui/tango/ui.(*Label).measure` | 279.9996 | 2 | Collapse/Large100k, ProList/Large100k | 否 |
| 27 | `github.com/tangoui/tango/ui.(*RichTextW).pieces` | 260.5536 | 2 | Markdown/Large100k, RichText/Large100k | 否 |
| 28 | `github.com/tangoui/tango/ui/text.ShapeInto` | 181.9302 | 2 | Collapse/Large100k, List/Large100k | 否 |
| 29 | `github.com/tangoui/tango/ui.(*DescriptionsW).build` | 133.4361 | 1 | Descriptions/Large100k | 否 |
| 30 | `github.com/tangoui/tango/ui.perfScenes.func2.func1.1` | 131.5992 | 1 | List/Large100k | 否 |

前 30 项没有落在非目标区域（`internal/render`、`uihost`、`macoslite`、`ui/terminal`、`ui/editor`）的函数。前 30 中的 `go-text/typesetting/harfbuzz` 属于文字 shaping 路径，在本项目可改范围内（缓存与调用方式可改，第三方代码本身不改）。`runtime.gcDrain` / `runtime.madvise`（第 20、23 名）是大集合构建分配引起的 GC 与内存归还，属于分配流量，不是常驻内存策略。

## 转交清单

本次进程内热点**没有一项落在非目标区域**。原因之一是 no-op 渲染器把渲染器后端排除在计时之外，渲染器侧的证据要等 12b 的 WindowServer CPU / footprint 数据。下表列出已知要转交、或要留给后续批次的产品问题：

| 编号 | 问题 | 证据 | 归属 |
|---|---|---|---|
| P1 | `NoAnimations` 下 Message 提示关闭后不会从树中移除（不透明度约 0 时仍在） | `ui/nav_notify.go:225-231` 的 `renderMessages` 只在 `start+dur` 处 `WakeAt`，没有在 `start+dur+DurationMid` 处唤醒，最后一次过滤从不发生（Task 5 发现） | 本项目优化批次：补 `WakeAt`，并加回归测试（`DurationMid` 后浮层数为 0） |
| P2 | Command 未虚拟化 | `Command/Large100k` 768.46 ms/步，预算 1.04 ms；Large1k 0.70 ms；`(*CommandW).item.func1` 进入热点前 30 | 本项目第 1 批候选 |
| P3 | LogView 首帧按 200 px 占位高度布局，之后不再请求帧，静止画面缺行 | 夹具在首次构建后多跑一帧绕过；`TestPerfCollLogViewFirstFrame` 修复后会翻转（Task 6）。LogView 本次各场景都在预算内：Mount 87.4 µs，Interact 37.1 µs，Large100k 24.6 µs | `perf/memory-logview` 会话 |
| — | 渲染器后端（合成器 CPU、提交、footprint） | 进程内不可见 | 待 12b；若有证据再转渲染器会话 |
| — | 终端 / 编辑器 | 本项目夹具不覆盖 `ui/terminal`、`ui/editor` 内核，profile 中未出现 | 无转交 |

## 第 1 批热点候选

依据 `hot.md` 和本次实测（以下时间为基准中位数）：

1. **非虚拟化的 10 万项集合**（每步重建并布局全部子项，耗时随 n 线性增长）：Tabs（4.44 s/步）、Tree（1.09 s）、List（1.02 s）、InfiniteScroll（867.66 ms）、Cascader / CascaderPanel（609.90 / 613.41 ms）、Menu（91.04 ms）、Command（768.46 ms，P2）、Scroll / ScrollH（50.35 / 50.78 ms）。同样线性增长的还有 CheckAll、CheckboxGroup、RadioGroup、ToggleGroup（约 2.5 s）、InputTag（2.25 s）、Descriptions（2.07 s）、RichText（2.02 s）、ProList（1.78 s）、Collapse（1.72 s）、Mentions、AutoComplete（约 1.4 s）、Timeline（1.37 s）、Segmented（1.11 s）、Markdown（1.08 s）、Conversations（694.70 ms）、ProTable（277.77 ms）、Carousel（243.53 ms）。其中有些是可放大夹具，产品上未必需要支持 10 万项；这一点在批次里逐个判定是“补虚拟化”还是“定为不适用”，不得为了过门槛改夹具。
2. **已虚拟化但仍超预算**：Transfer（43.36 ms）、AvatarGroup（18.32 ms）、BubbleList（6.06 ms）、TreeSelect（5.62 ms）、Masonry（386 µs，预算 95 µs）。
3. **公共管线**：`flexbox.flex` / `measure` / `measureUncached` / `arrange` / `measureAxis` 出现在 21 个超标场景里，说明布局对不可见子项也做了完整测量。文字路径 `text.layoutRuns` / `compactShaper.append` / `text.Shape` / `prepareText` / `shapeItems` 加上 harfbuzz 出现在 15 个超标场景里，说明重复 shaping 没有命中缓存。这两条是“多个组件共享”的优先目标。
4. **Image/Mount**：653 µs，是唯一超标的首次构建；它的 Interact 也是所有 Interact 中最慢的（586 µs）。
5. **局部重绘**：`perfDamageDebt` 有 146 项，大多数交互帧重绘了整个窗口（如 Icon 2250×、IconButton 2677×、Kbd 1592×、CheckableTag 1042×）。这是 damage 债，与时间预算分开处理。
