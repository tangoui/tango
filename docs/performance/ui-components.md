# UI 组件性能：R1 基线（合并后）与热点排名（2026-11-17）

本文是 UI 组件极限性能项目的结果文档。设计稿：[`docs/superpowers/specs/2026-11-16-ui-component-performance-design.md`](../superpowers/specs/2026-11-16-ui-component-performance-design.md)。

当前版本是 **R1 基线（合并后）**：合并 origin/main 后在 `1969e04` 上重测的进程内基线（第一层），是第 1 批优化的对比基准。Task 12a 在 `5f115d3` 上测的基线保留在文末“历史：Task 12a 基线”一节。跨框架基线（第二层，Task 12b）尚未运行，本文**不包含**任何“比 MyGo / NSGo 快”的结论，跨框架列一律“待测”。

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

### 批次 A/B 对比口径

优化批次判断前后快慢，只用基准中位数（`uiperf bench` 写出的 `bench.json`）或 `uiperf ab`（固定测试二进制、每轮新进程、交替多轮取中位数）。`perfTimeDebt` 里的 Measured 字符串是门槛单次运行的记录值，只用于棘轮登记，不得用作 A/B 对比的依据。

## R1 基线（合并后，当前对比基准）

R1 是合并 origin/main 之后在同一台机器上重测的进程内基线，**第 1 批优化的 A/B 对比以本节为 BASE**。下文“12a”指历史一节的 Task 12a 基线（5f115d3）。跨框架列仍为“待测”，本节不包含任何与 MyGo / NSGo 快慢有关的结论。

### 证据与环境

- 机器与 Go 版本同 12a：Apple M4（Mac16,10），10 核，16 GB，macOS 26.6（25G72），Go 1.27.2 darwin/arm64。
- Tango：`1969e049018d97a221b181eeb40c09c0fd8bdf02`（分支 `perf/ui-components-20261116`）。它包含合并提交 470fac3（带入上游 197 个提交）、M1 的 11 个新夹具（3d6c7b8）、F1 的 uiperf 修正（03d2a72），以及 12b 准备提交 0267a9a、1969e04。后两个提交只改了 `tools/uibench` 和 `internal/cmd/uiperf`：bench 的 `-bench` 第一段锚定为 `^BenchmarkComponents$`（不再顺带跑 `BenchmarkComponentsRaster`），并增加一条 stderr 提示，测量值不受影响。门槛运行时，工作区里只有 `ui/perf_debt_test.go` 加了 CellEditorTable、RowEditorTable 两个 Large100k 的债表键，Measured 值在两次门槛之后才填。门槛只检查键，不检查 Measured 字符串。
- 环境变量里没有 `TANGO_*`，所以 9b9d853 的可选 CoreText 文字路径（`TANGO_TEXT_ENGINE=coretext`）没有启用。
- 原始数据在 `.superpowers/sdd/2026-11-16-ui-component-performance/baseline-R1/`（已 gitignore，不入库）：
  - `bench.json` / `status.md`：基准中位数、uiperf 内置预算测试的预算值、冷启动值；内置预算测试的输出另存为 `budget-inbench.txt`。
  - `hot.md`、`prof/`：热点分析。
  - `gate1.txt`、`gate2.txt`：两次门槛。
  - `debt-worse.txt`：两次门槛中较差的一次。
  - `load.log`、`monitor.log`、`monitor2.log`、`wait.log`：负载与进程记录。
  - 受干扰作废的两次运行：`bench-stdout.attempt1-disturbed.txt`、`gate1.attempt1-disturbed.txt`。

### 运行与负载（采信的步骤 1 分钟负载均 < 6）

| 步骤 | 命令 | 时间 | 负载（前 → 后，1/5/15 分钟） |
|---|---|---|---|
| bench 第 1 次，**受干扰，作废** | `go run ./internal/cmd/uiperf bench -count 3 -out baseline-R1` | 06:17:54–06:45:24 | 2.09 2.25 2.23 → 2.46 2.70 2.67 |
| **bench 第 2 次**（基准 `-count 3`、内置预算测试 `-p 1 -count=1`、冷启动） | 同上 | 07:18:26–08:29:15 | 3.93 2.59 2.34 → 2.14 2.53 2.69（每分钟采样，最高 4.05） |
| `uiperf hot`（35 个超标场景，0 失败） | `go run ./internal/cmd/uiperf hot -out baseline-R1` | 08:29:46–08:32:10 | 2.17 2.50 2.68 → 3.04 2.59 2.67 |
| 门槛第 1 次，**受干扰，作废**（已输出 278 行 PERF 时被停掉） | `TANGO_PERF_BUDGET=1 go test ./ui ./examples/ui-gallery -run 'Test(Perf\|GalleryPage)Budgets' -count=1 -p 1 -timeout 0 -v` | 08:32:39–08:41:25 | 2.37 2.47 2.62 → 6.34 4.11 3.24（08:41:06 为 6.82） |
| **门槛 1**，PASS（ui 793.5 s，gallery 3.2 s） | 同上 | 08:46:18–08:59:37 | 3.68 3.67 3.27 → 4.23 3.34 3.21（采样最高 4.37） |
| **门槛 2**，PASS（ui 791.5 s，gallery 3.3 s） | 同上 | 08:59:50–09:13:07 | 4.20 3.36 3.22 → 3.19 3.00 3.08 |

- bench 第 2 次内部的分段可以从进程记录推出（推算值）：基准 `go test` 约 07:18:26–08:16，共 926 项 × 3 次；内置预算测试约 08:16–08:29（ui 763.7 s，FAIL，原因只是当时债表里还没有两个新的编辑表格条目）；冷启动在 08:29 前后，209 个组件全部 ok。
- **两次受干扰的运行：**
  - bench 第 1 次：06:45:08 另一个会话启动了 `tango-scenes grid`（`/Users/admin/min/outputs/tango-combined-gate-20261117`，随后还有 `/tmp/tango-memprof/probe.sh`）。按协调安排停掉，等对方结束后重跑。
  - 门槛第 1 次：另一个会话在 `/private/tmp/tango-ed` 跑 `go test -race ./ui/editor -run 'Minimap|Miniature'`，负载升到 6.82，停掉。等没有外部 `go test` 且 1 分钟负载 < 4 连续 2 分钟后，再从头跑门槛 1。
- **07:17 的重叠：** min-0f 在约 07:17 跑了约 1 分钟的完整 `go test`，CPU 余波可能与 bench 第 2 次开头重叠，开始时负载为 3.93（之前约 2.1）。按协调规则检查了 A 开头的夹具：45 项 R1/12a 中位数比的中位数为 1.12，四分位 1.04–1.15，单项最高 1.29（App/Interact），没有一项超过 1.5 倍；其余 835 项比值的中位数为 1.07。A 开头的夹具没有明显被抬高，所以保留原始结果，没有补测。
- 步骤全部串行。门槛与内置预算测试都带 `-p 1`，`./ui` 与 gallery 不再并行。

### 结果概要

- 夹具 209 个（12a 为 198 个，M1 新增 11 个），基准项 926 个：Mount / MountCold / Unchanged 各 209，Interact 161，Large1k / 10k / 100k 各 46。
- 时间预算的判定以门槛 1、2 为准。两次门槛与内置预算测试对 416 个有预算的场景**判定完全一致，没有翻转项**。
  - `Mount`：209 个，208 个达标，**1 个超标**（Image，门槛 689 / 665 µs，预算 500 µs）。
  - `Interact`：161 个，**全部达标**。最慢的是 Image，606 µs。
  - `Large100k`：46 个，12 个达标，**34 个超标**。达标的 12 个与 12a 相同：CodeBlock、JSONView、JSONViewSource、JSONViewValue、Listy、LogView、MultiSelect、Select、Table、VirtualList、VirtualSelect、VirtualTree。
- 新夹具：CellEditorTable、RowEditorTable 的 Large100k 超标（非虚拟化编辑表格，每步构建全部 10 万行）。其余 9 个新夹具的 Mount / Interact 全部达标，最慢的是 RowEditorTable/Interact，基准中位数 188.3 µs。
- 离预算线最近的达标项：
  - Listy/Large100k：门槛 37.6 / 38.6 µs，预算 48.0 / 48.1 µs，约 80%（12a 约 76%）。
  - JSONViewSource/Large100k：约 71%。
  - Table/Large100k：约 69%。
  - QRCode/Mount：门槛 334 µs，预算 500 µs，约 67%。12a 时只有约 18%，见“与 12a 的差异”。
- 基准与门槛差异较大的仍是 AutoComplete、Mentions 的 Large100k：基准中位数 1.41 s / 1.43 s，门槛 754–764 ms / 761–765 ms。原因同 12a，第 0 步打开并过滤 10 万项计入了基准，门槛把它放在预热里。

### 债表变化

- `perfTimeDebt`：33 → **35 项**，Why 均为 `baseline`。新增 `CellEditorTable/Large100k`（508.92 ms，预算 4.00 ms）和 `RowEditorTable/Large100k`（500.34 ms，预算 4.00 ms）。没有删除项，原有 33 项的 Measured 全部改为本次两次门槛中较差的值。
- LogView 按 R23 不登记：全部场景在预算内，门槛 1 / 2 分别为 Mount 96.5 / 100.5 µs，Interact 41.0 / 44.0 µs，Large100k 28.1 / 25.8 µs（预算 42.9 / 70.5 µs）。
- `galleryTimeDebt`：0 → 0 项。三次运行的 `TestGalleryPageBudgets` 都 PASS，没有 GALLERY 超标行，只更新了注释。
- `perfDamageDebt`：153 项。M1 已在 3d6c7b8 新增 7 项（12a 时为 146 项），本任务未改。

### 与 12a 的差异

**跨过预算线的项：无。** 12a 的 396 个有预算场景在 R1 的判定与 12a 完全相同，超标集合仍是那 33 项。新增的超标只来自新夹具。

**整体漂移：** 880 个共有基准项的 R1/12a 中位数比如下。比值同时混有上游代码变化和机器状态差异：12a 在 01:45–02:50 运行，负载约 1.3–1.9；R1 在 07:18–08:16 运行，负载约 2.1–4.0。

| 场景 | 项数 | ns/op 比中位数 | allocs/op 比中位数 |
|---|---:|---:|---:|
| Mount | 198 | 1.101 | 1.029 |
| MountCold（进程内） | 198 | 1.103 | 1.029 |
| Unchanged | 198 | 1.017 | 1.000 |
| Interact | 154 | 1.019 | 1.000 |
| Large1k / 10k / 100k | 44 / 44 / 44 | 1.019 / 1.021 / 1.017 | 1.000 |
| 冷启动（新进程首帧） | 198 | **0.725**（四分位 0.689–0.799） | — |

- **冷启动快了约 27.5%**（中位数 10.10 ms → 7.28 ms），这次合并里最大的整体变化。最可能的来源是 acb8e99“perf(text): avoid cold font preparation and sfnt copies”（去掉冷启动时的字体准备和 sfnt 拷贝）。热缓存场景（Mount、Large*）没有出现同等幅度的变化，与该提交只作用于冷路径一致。
- Mount 普遍慢约 10%，allocs/op 多约 3%。这与上游 antd 6.7 对齐批次给许多组件加的节点和属性一致。热路径的 Unchanged / Interact / Large* 基本持平（+2% 左右，在机器漂移范围内）。
- 3898e06（变量字体共享、HVAR 延迟特化）没有看到可单独归因的进程内时间变化。它的提交说明针对的是常驻内存（空闲 footprint 49.8 → 38.2 MiB），不属于本基准测的内容。
- 9b9d853 是可选路径，默认关闭，本次没有启用，对 R1 数值没有贡献。
- 文字和布局的 Large100k 超标项（Tabs、CheckAll、RadioGroup 等）比值都在 0.99–1.06 之间，说明上游文字提交没有改变每步的 shaping 和布局开销，第 1 批的公共管线目标不变。

**单项相对变化最大的（基准中位数，12a → R1）：**

| 项 | 12a | R1 | 比值 | allocs/op | 可见原因 |
|---|---:|---:|---:|---|---|
| QRCode/Mount | 89.0 µs | 281.5 µs | 3.16× | 258 → 371 | daec190（media 对齐）把 antd `boostLevel` 默认设为 true：编码后再对每个更高纠错级别各编码一次，最多多 3 次 `EncodeQR`。仍在 500 µs 预算内（门槛 334 µs） |
| FooterToolbar/Unchanged | 2.1 µs | 6.0 µs | 2.83× | 39 → 79 | e4b7eff（剩余对齐工作，改了 `ui/pro2_page.go`）；Mount 也慢了 1.19× |
| Transfer/Large1k / 10k / 100k | 337.3 µs / 3.45 ms / 43.67 ms | 623.3 µs / 6.41 ms / 81.52 ms | 1.85–1.87× | 100k：2383 → 7154；B/op 411 → 476 MB | 1ceffa2（selectors 对齐）：右侧列表默认按 `targetKeys` 顺序（antd `unshift`）排列，每次构建都对全部条目建 `map[key]index` 和 `seen` 映射；profile 中 GC 和 `madvise` 占大头 |
| Tour/Interact | 30.6 µs | 44.4 µs | 1.45× | 541 → 686 | c00df79 / 019df35（overlays、tags 对齐，改了 `ui/data_popover.go`） |
| OTP/Interact、Unchanged | 13.2 / 4.5 µs | 17.8 / 6.0 µs | 1.35 / 1.34× | 229 → 313 | 5e02ddc（inputs 对齐，改了 `ui/entry_otp.go`） |
| ProList/Large1k、Unchanged | 3.89 ms / 7.7 µs | 4.79 ms / 10.2 µs | 1.23 / 1.33× | 93629 → 109393 | e4b7eff（改了 `ui/pro3_list.go`）；Large100k 1.78 → 2.02 s（1.13×） |
| TreeSelect/Large10k、100k | 441.6 µs / 5.82 ms | 537.9 µs / 7.07 ms | 1.22× | 基本不变 | 1ceffa2 改了 `ui/entry_treeselect.go`；allocs 没变，原因待批次内确认 |
| Tree/Large100k | 1.09 s | 1.25 s | 1.15× | 不变 | 8708477 / b2c53ba（trees 对齐）；allocs 没变，原因待批次内确认 |
| **Alert/Interact、Unchanged** | 2.9 / 2.8 µs | 1.9 / 1.9 µs | **0.67 / 0.68×** | 57 → 27，B/op 5352 → 688 | c00df79（改了 `ui/feedback.go`）：每帧的分配减少一半以上 |
| Message/Interact | 13.6 µs | 11.2 µs | 0.82× | — | 上游 overlays 对齐 |
| Descriptions/Interact、Unchanged | 14.8 µs | 13.1 µs | 0.88–0.89× | — | 上游对齐 |

“可见原因”只指出改过该组件源文件的上游提交，并结合 allocs/op 推断。只有 QRCode、Transfer 两项读过 diff 并确认了机理，其余要在对应批次里用 `uiperf ab` 验证。

### 逐组件状态表（R1）

时间为 R1 `bench.json` 的基准中位数，冷启动为新进程中 `MountCold` 的单次值，脏区债取自当前的 `perfDamageDebt`，“进程内时间预算达标”按门槛 1、2 判定。状态规则同 12a：有对手场景或有任一预算债（时间或脏区）记“待优化”，其余记“无对手只看预算”。新夹具的跨框架场景暂记“—”。12a 的 198 个组件状态都没有变化。新夹具中有脏区债的 7 个记“待优化”，4 个纯构建夹具（PageLoading、ProStatistic、StatisticCardDivider、StatisticCardOperation）记“无对手只看预算”。

统计：待优化 161，无对手只看预算 48。

| 组件 | Mount | Unchanged | Interact | Large 1k / 10k / 100k | 冷启动 | 脏区债 | 进程内时间预算达标 | 跨框架场景 | 对 MyGo | 对 NSGo | 状态 |
|---|---:|---:|---:|---:|---:|---:|---|---|---|---|---|
| Addon | 56.4 µs | 2.9 µs | 4.0 µs | — | 5.34 ms | 27.72× | 是 | — | 待测 | 待测 | 待优化 |
| Affix | 64.8 µs | 9.2 µs | 29.3 µs | — | 7.58 ms | 5.62× | 是 | — | 待测 | 待测 | 待优化 |
| Alert | 50.9 µs | 1.9 µs | 1.9 µs | — | 7.55 ms | 9.90× | 是 | — | 待测 | 待测 | 待优化 |
| Anchor | 63.8 µs | 10.1 µs | 29.2 µs | — | 7.69 ms | 2.59× | 是 | — | 待测 | 待测 | 待优化 |
| App | 51.3 µs | 2.4 µs | 4.0 µs | — | 7.99 ms | 18.00× | 是 | — | 待测 | 待测 | 待优化 |
| Approval | 57.0 µs | 5.8 µs | — | — | 7.77 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| AspectRatio | 51.4 µs | 1.6 µs | 1.6 µs | — | 7.17 ms | 14.40× | 是 | — | 待测 | 待测 | 待优化 |
| AutoComplete | 50.3 µs | 1.6 µs | 10.7 µs | 1.21 ms / 63.39 ms / 1.41 s | 5.21 ms | 26.47× | 否（Large100k） | 计划 T10/T11 | 待测 | 待测 | 待优化 |
| Avatar | 50.1 µs | 885 ns | 933 ns | — | 7.36 ms | 810.00× | 是 | — | 待测 | 待测 | 待优化 |
| AvatarGroup | 55.4 µs | 4.6 µs | 4.6 µs | 155.4 µs / 1.62 ms / 19.16 ms | 7.21 ms | 316.41× | 否（Large100k） | — | 待测 | 待测 | 待优化 |
| BackTop | 59.1 µs | 6.6 µs | 29.2 µs | — | 7.60 ms | 5.59× | 是 | — | 待测 | 待测 | 待优化 |
| Badge | 51.2 µs | 1.4 µs | 1.4 µs | — | 5.09 ms | 810.00× | 是 | — | 待测 | 待测 | 待优化 |
| BadgeRibbon | 52.5 µs | 2.8 µs | 2.8 µs | — | 7.63 ms | 7.39× | 是 | — | 待测 | 待测 | 待优化 |
| BorderBeam | 58.3 µs | 8.6 µs | 8.7 µs | — | 7.22 ms | 33.75× | 是 | — | 待测 | 待测 | 待优化 |
| Breadcrumb | 55.8 µs | 4.9 µs | 12.9 µs | — | 7.28 ms | 21.16× | 是 | — | 待测 | 待测 | 待优化 |
| BreadcrumbItems | 56.2 µs | 4.9 µs | 15.0 µs | — | 7.04 ms | 32.53× | 是 | — | 待测 | 待测 | 待优化 |
| Bubble | 62.3 µs | 5.6 µs | — | — | 7.88 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| BubbleList | 98.0 µs | 21.5 µs | — | 360.2 µs / 868.9 µs / 5.85 ms | 7.58 ms | — | 否（Large100k） | — | 待测 | 待测 | 待优化 |
| Button | 50.1 µs | 1.3 µs | 1.8 µs | — | 7.24 ms | 454.10× | 是 | counter（12b 待测） | 待测 | 待测 | 待优化 |
| ButtonGroup | 52.1 µs | 2.7 µs | 3.8 µs | — | 7.28 ms | 210.08× | 是 | counter（12b 待测） | 待测 | 待测 | 待优化 |
| Calendar | 112.5 µs | 47.9 µs | 47.9 µs | — | 7.55 ms | 14.06× | 是 | — | 待测 | 待测 | 待优化 |
| Card | 57.2 µs | 5.7 µs | 5.8 µs | — | 7.47 ms | 21.18× | 是 | — | 待测 | 待测 | 待优化 |
| Carousel | 64.0 µs | 5.2 µs | 17.0 µs | 1.42 ms / 15.09 ms / 263.52 ms | 7.32 ms | 4.66× | 否（Large100k） | — | 待测 | 待测 | 待优化 |
| Cascader | 51.4 µs | 2.1 µs | 28.3 µs | 1.17 ms / 51.29 ms / 613.28 ms | 7.14 ms | 168.75× | 否（Large100k） | 计划 T10/T11 | 待测 | 待测 | 待优化 |
| CascaderPanel | 61.9 µs | 8.6 µs | 20.3 µs | 1.08 ms / 51.28 ms / 614.09 ms | 7.79 ms | 21.62× | 否（Large100k） | 计划 T10/T11 | 待测 | 待测 | 待优化 |
| CellEditorTable | 68.1 µs | 13.5 µs | 171.9 µs | 2.49 ms / 31.05 ms / 393.06 ms | 7.80 ms | 2.82× | 否（Large100k） | — | 待测 | 待测 | 待优化 |
| CheckAll | 53.8 µs | 4.3 µs | 14.2 µs | 1.42 ms / 221.54 ms / 2.53 s | 7.29 ms | 17.90× | 否（Large100k） | — | 待测 | 待测 | 待优化 |
| CheckCard | 53.1 µs | 2.8 µs | — | — | 8.67 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| CheckCardGroup | 49.8 µs | 768 ns | — | — | 144.9 µs | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| CheckableTag | 49.9 µs | 1.1 µs | 3.3 µs | — | 7.27 ms | 1042.64× | 是 | — | 待测 | 待测 | 待优化 |
| CheckableTagGroup | 54.2 µs | 3.7 µs | 10.1 µs | — | 7.28 ms | 316.72× | 是 | — | 待测 | 待测 | 待优化 |
| Checkbox | 50.0 µs | 1.2 µs | 4.1 µs | — | 7.60 ms | 725.59× | 是 | 计划 T10/T11 | 待测 | 待测 | 待优化 |
| CheckboxGroup | 53.6 µs | 3.3 µs | 9.1 µs | 1.42 ms / 223.75 ms / 2.55 s | 5.27 ms | 200.98× | 否（Large100k） | — | 待测 | 待测 | 待优化 |
| CodeBlock | 118.4 µs | 4.0 µs | 4.2 µs | 6.4 µs / 6.4 µs / 6.4 µs | 10.37 ms | 5.50× | 是 | — | 待测 | 待测 | 待优化 |
| Collapse | 58.2 µs | 6.4 µs | 18.3 µs | 2.64 ms / 140.87 ms / 1.74 s | 7.30 ms | 10.55× | 否（Large100k） | — | 待测 | 待测 | 待优化 |
| CollapsibleSider | 53.2 µs | 2.7 µs | 11.3 µs | — | 7.35 ms | 5.33× | 是 | — | 待测 | 待测 | 待优化 |
| ColorPicker | 51.2 µs | 1.8 µs | 10.1 µs | — | 5.26 ms | 18.35× | 是 | 计划 T10/T11 | 待测 | 待测 | 待优化 |
| ColorPickerGradient | 52.0 µs | 2.4 µs | 17.4 µs | — | 5.19 ms | 13.88× | 是 | — | 待测 | 待测 | 待优化 |
| ColorPickerPanel | 65.7 µs | 11.8 µs | 33.7 µs | — | 7.42 ms | 15.13× | 是 | — | 待测 | 待测 | 待优化 |
| ColorPickerPanelGradient | 78.4 µs | 22.8 µs | 59.0 µs | — | 7.40 ms | 12.56× | 是 | — | 待测 | 待测 | 待优化 |
| Command | 61.1 µs | 8.2 µs | 23.4 µs | 723.2 µs / 67.79 ms / 777.73 ms | 7.66 ms | 11.14× | 否（Large100k） | — | 待测 | 待测 | 待优化 |
| Compact | 55.6 µs | 5.6 µs | 6.9 µs | — | 7.44 ms | 198.54× | 是 | — | 待测 | 待测 | 待优化 |
| ConfigProvider | 50.9 µs | 1.9 µs | 2.0 µs | — | 7.31 ms | 284.91× | 是 | — | 待测 | 待测 | 待优化 |
| Confirm | 64.5 µs | 8.8 µs | 40.1 µs | — | 7.76 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| ContextMenuAt | 51.7 µs | 2.2 µs | 19.1 µs | — | 7.22 ms | 5.39× | 是 | — | 待测 | 待测 | 待优化 |
| ContextMenuAtWith | 50.4 µs | 1.3 µs | 15.4 µs | — | 7.16 ms | 18.75× | 是 | — | 待测 | 待测 | 待优化 |
| Conversations | 52.0 µs | 2.4 µs | — | 1.62 ms / 57.81 ms / 701.08 ms | 7.27 ms | — | 否（Large100k） | — | 待测 | 待测 | 待优化 |
| Countdown | 50.0 µs | 1.3 µs | 1.5 µs | — | 7.20 ms | 16.05× | 是 | — | 待测 | 待测 | 待优化 |
| Custom | 50.5 µs | 1.5 µs | 1.5 µs | — | 7.23 ms | 14.55× | 是 | — | 待测 | 待测 | 待优化 |
| DatePicker | 50.7 µs | 1.3 µs | 29.4 µs | — | 7.23 ms | 12.23× | 是 | 计划 T10/T11 | 待测 | 待测 | 待优化 |
| DatePickerPanel | 89.5 µs | 29.6 µs | 48.7 µs | — | 7.97 ms | 14.61× | 是 | — | 待测 | 待测 | 待优化 |
| DateTimePicker | 51.4 µs | 1.6 µs | 75.2 µs | — | 5.12 ms | 7.85× | 是 | — | 待测 | 待测 | 待优化 |
| DefaultFooter | 48.9 µs | 443 ns | — | — | 120.6 µs | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| Descriptions | 68.3 µs | 13.1 µs | 13.1 µs | 2.61 ms / 172.46 ms / 2.15 s | 7.64 ms | 6.13× | 否（Large100k） | — | 待测 | 待测 | 待优化 |
| Disabled | 50.0 µs | 1.3 µs | 1.3 µs | — | 9.94 ms | 391.36× | 是 | — | 待测 | 待测 | 待优化 |
| Div | 49.7 µs | 1.1 µs | 1.1 µs | — | 7.65 ms | 40.50× | 是 | — | 待测 | 待测 | 待优化 |
| Divider | 49.4 µs | 952 ns | 963 ns | — | 7.32 ms | 38.79× | 是 | — | 待测 | 待测 | 待优化 |
| Dock | 54.6 µs | 4.5 µs | — | — | 7.05 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| Drawer | 55.9 µs | 4.8 µs | 16.0 µs | — | 7.69 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| DrawerForm | 67.5 µs | 12.8 µs | — | — | 7.95 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| DropZone | 50.3 µs | 1.3 µs | 1.7 µs | — | 7.08 ms | 20.77× | 是 | — | 待测 | 待测 | 待优化 |
| Dropdown | 50.6 µs | 1.8 µs | 12.3 µs | — | 7.23 ms | 49.44× | 是 | — | 待测 | 待测 | 待优化 |
| EditableProTable | 81.4 µs | 22.6 µs | — | — | 7.84 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| Empty | 55.1 µs | 5.7 µs | 5.8 µs | — | 7.31 ms | 5.23× | 是 | — | 待测 | 待测 | 待优化 |
| FieldSet | 60.1 µs | 7.5 µs | 32.1 µs | — | 7.40 ms | 29.09× | 是 | — | 待测 | 待测 | 待优化 |
| Flex | 53.0 µs | 3.6 µs | 3.6 µs | — | 7.39 ms | 84.38× | 是 | — | 待测 | 待测 | 待优化 |
| FloatButton | 50.5 µs | 1.5 µs | 4.7 µs | — | 155.1 µs | 810.00× | 是 | — | 待测 | 待测 | 待优化 |
| FloatButtonGroup | 50.7 µs | 1.8 µs | 7.9 µs | — | 179.8 µs | 213.16× | 是 | — | 待测 | 待测 | 待优化 |
| FooterToolbar | 55.5 µs | 6.0 µs | — | — | 7.41 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| Form | 52.4 µs | 2.7 µs | 4.2 µs | — | 7.29 ms | 29.09× | 是 | — | 待测 | 待测 | 待优化 |
| GridContent | 50.4 µs | 1.6 µs | — | — | 7.53 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| GridRow | 54.1 µs | 4.1 µs | 4.1 µs | — | 7.53 ms | 42.32× | 是 | — | 待测 | 待测 | 待优化 |
| HotkeyInput | 55.0 µs | 4.1 µs | 16.6 µs | — | 7.00 ms | 155.77× | 是 | — | 待测 | 待测 | 待优化 |
| Icon | 49.7 µs | 743 ns | 792 ns | — | 140.0 µs | 2250.00× | 是 | — | 待测 | 待测 | 待优化 |
| IconButton | 50.0 µs | 868 ns | 1.3 µs | — | 156.0 µs | 2677.69× | 是 | — | 待测 | 待测 | 待优化 |
| Image | 660.2 µs | 939 ns | 588.1 µs | — | 835.4 µs | 33.75× | 否（Mount） | — | 待测 | 待测 | 待优化 |
| InfiniteScroll | 72.6 µs | 17.5 µs | 220.2 µs | 2.39 ms / 72.45 ms / 889.86 ms | 7.62 ms | 10.38× | 否（Large100k） | — | 待测 | 待测 | 待优化 |
| Input | 50.5 µs | 1.5 µs | 5.1 µs | — | 7.34 ms | 112.50× | 是 | 计划 T10/T11 | 待测 | 待测 | 待优化 |
| InputNumber | 51.4 µs | 1.9 µs | 8.2 µs | — | 4.96 ms | 184.09× | 是 | — | 待测 | 待测 | 待优化 |
| InputTag | 53.6 µs | 3.9 µs | 21.9 µs | 5.47 ms / 194.78 ms / 2.27 s | 5.99 ms | 112.50× | 否（Large100k） | — | 待测 | 待测 | 待优化 |
| Item | 51.6 µs | 2.1 µs | 8.6 µs | — | 7.57 ms | 11.64× | 是 | — | 待测 | 待测 | 待优化 |
| ItemGroup | 55.5 µs | 4.1 µs | 16.4 µs | — | 7.61 ms | 5.82× | 是 | — | 待测 | 待测 | 待优化 |
| JSONView | 79.0 µs | 17.5 µs | 27.3 µs | 20.8 µs / 28.1 µs / 20.8 µs | 8.87 ms | 5.33× | 是 | — | 待测 | 待测 | 待优化 |
| JSONViewSource | 79.5 µs | 17.4 µs | 27.2 µs | 20.8 µs / 27.9 µs / 20.8 µs | 8.68 ms | 5.33× | 是 | — | 待测 | 待测 | 待优化 |
| JSONViewValue | 79.2 µs | 17.5 µs | 26.4 µs | 36.2 µs / 48.0 µs / 30.0 µs | 8.92 ms | 5.33× | 是 | — | 待测 | 待测 | 待优化 |
| Kbd | 50.1 µs | 963 ns | 975 ns | — | 5.12 ms | 1592.02× | 是 | — | 待测 | 待测 | 待优化 |
| Layout | 55.0 µs | 4.1 µs | 4.2 µs | — | 7.13 ms | 4.74× | 是 | — | 待测 | 待测 | 待优化 |
| LightFilter | 54.5 µs | 3.0 µs | — | — | 7.41 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| Link | 50.2 µs | 1.1 µs | 1.6 µs | — | 7.18 ms | 42.32× | 是 | — | 待测 | 待测 | 待优化 |
| List | 67.4 µs | 12.9 µs | 38.1 µs | 2.64 ms / 82.04 ms / 1.08 s | 5.60 ms | 3.75× | 否（Large100k） | 计划 T10/T11 | 待测 | 待测 | 待优化 |
| ListActions | 71.5 µs | 16.6 µs | 44.3 µs | — | 7.44 ms | 4.31× | 是 | — | 待测 | 待测 | 待优化 |
| ListToolBar | 63.8 µs | 10.8 µs | 30.2 µs | — | 7.39 ms | 9.90× | 是 | — | 待测 | 待测 | 待优化 |
| Listy | 73.5 µs | 13.1 µs | 41.2 µs | 31.2 µs / 31.4 µs / 34.0 µs | 7.47 ms | 2.59× | 是 | list（12b 待测） | 待测 | 待测 | 待优化 |
| Loading | 52.3 µs | 2.7 µs | 2.1 µs | — | 7.07 ms | 7.76× | 是 | — | 待测 | 待测 | 待优化 |
| LogView | 92.8 µs | 13.5 µs | 37.9 µs | 25.0 µs / 25.0 µs / 25.0 µs | 9.02 ms | 4.22× | 是 | — | 待测 | 待测 | 待优化 |
| LoginForm | 67.1 µs | 12.3 µs | — | — | 7.17 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| LoginFormPage | 68.6 µs | 13.6 µs | — | — | 7.27 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| Markdown | 85.7 µs | 21.4 µs | 22.6 µs | 8.61 ms / 96.86 ms / 1.10 s | 8.92 ms | 9.00× | 否（Large100k） | — | 待测 | 待测 | 待优化 |
| MarkdownEditor | 75.0 µs | 20.5 µs | 65.6 µs | — | 7.45 ms | 4.63× | 是 | — | 待测 | 待测 | 待优化 |
| Masonry | 84.4 µs | 24.5 µs | 24.6 µs | 65.7 µs / 101.8 µs / 442.7 µs | 7.89 ms | — | 否（Large100k） | — | 待测 | 待测 | 待优化 |
| Mentions | 50.6 µs | 1.7 µs | 6.9 µs | 1.25 ms / 65.16 ms / 1.43 s | 5.47 ms | 112.50× | 否（Large100k） | — | 待测 | 待测 | 待优化 |
| Menu | 56.1 µs | 4.6 µs | 13.5 µs | 514.8 µs / 5.79 ms / 96.12 ms | 7.38 ms | 18.24× | 否（Large100k） | 计划 T10/T11 | 待测 | 待测 | 待优化 |
| Menubar | 53.6 µs | 3.1 µs | 15.5 µs | — | 7.15 ms | 18.40× | 是 | — | 待测 | 待测 | 待优化 |
| Message | 58.6 µs | 5.2 µs | 11.2 µs | — | 7.41 ms | 26.30× | 是 | 计划 T10/T11 | 待测 | 待测 | 待优化 |
| MessageBox | 62.9 µs | 7.3 µs | 33.6 µs | — | 7.80 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| Modal | 58.3 µs | 6.4 µs | 8.6 µs | — | 7.56 ms | — | 是 | 计划 T10/T11 | 待测 | 待测 | 待优化 |
| ModalForm | 68.3 µs | 13.1 µs | — | — | 7.35 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| MultiDatePicker | 51.5 µs | 2.0 µs | 30.4 µs | — | 7.07 ms | 13.36× | 是 | — | 待测 | 待测 | 待优化 |
| MultiSelect | 52.0 µs | 2.4 µs | 20.0 µs | 16.4 µs / 16.4 µs / 16.4 µs | 7.31 ms | 17.46× | 是 | — | 待测 | 待测 | 待优化 |
| NativeView | 49.2 µs | 673 ns | — | — | 125.7 µs | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| Notification | 61.5 µs | 6.9 µs | 14.2 µs | — | 8.49 ms | 17.40× | 是 | — | 待测 | 待测 | 待优化 |
| NotificationCenter | 51.8 µs | 2.2 µs | 34.6 µs | — | 5.15 ms | 11.68× | 是 | — | 待测 | 待测 | 待优化 |
| OTP | 57.8 µs | 6.0 µs | 17.8 µs | — | 5.11 ms | 174.57× | 是 | — | 待测 | 待测 | 待优化 |
| Overlay | 52.7 µs | 3.0 µs | 7.6 µs | — | 8.43 ms | 64.06× | 是 | — | 待测 | 待测 | 待优化 |
| PageContainer | 54.7 µs | 4.1 µs | — | — | 7.68 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| PageHeader | 48.9 µs | 475 ns | — | — | 146.7 µs | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| PageLoading | 51.9 µs | 2.4 µs | — | — | 7.35 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| Pagination | 57.8 µs | 6.3 µs | 18.1 µs | — | 5.65 ms | 29.09× | 是 | 计划 T10/T11 | 待测 | 待测 | 待优化 |
| Paragraph | 50.1 µs | 1.0 µs | 1.0 µs | — | 7.32 ms | 40.91× | 是 | — | 待测 | 待测 | 待优化 |
| Popconfirm | 60.4 µs | 8.3 µs | 5.6 µs | — | 7.68 ms | 55.59× | 是 | — | 待测 | 待测 | 待优化 |
| Popover | 54.0 µs | 4.0 µs | 3.1 µs | — | 8.05 ms | 64.22× | 是 | 计划 T10/T11 | 待测 | 待测 | 待优化 |
| ProBreadcrumb | 55.7 µs | 4.9 µs | 13.1 µs | — | 7.24 ms | 21.16× | 是 | — | 待测 | 待测 | 待优化 |
| ProCard | 52.4 µs | 2.9 µs | — | — | 7.59 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| ProDescriptions | 58.5 µs | 6.1 µs | — | — | 7.17 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| ProFieldOf | 49.2 µs | 538 ns | — | — | 148.0 µs | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| ProForm | 62.8 µs | 9.1 µs | — | — | 7.43 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| ProFormFieldSet | 50.7 µs | 1.4 µs | — | — | 7.11 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| ProFormGroup | 52.7 µs | 2.8 µs | — | — | 8.48 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| ProFormItems | 58.3 µs | 6.3 µs | — | — | 7.22 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| ProLayout | 59.1 µs | 6.2 µs | — | — | 7.22 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| ProList | 64.0 µs | 10.2 µs | — | 4.79 ms / 151.53 ms / 2.02 s | 217.2 µs | — | 否（Large100k） | — | 待测 | 待测 | 待优化 |
| ProSettingsProvider | 206.6 µs | 85.9 µs | — | — | 7.26 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| ProSkeleton | 49.3 µs | 503 ns | — | — | 143.0 µs | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| ProStatistic | 55.4 µs | 4.4 µs | — | — | 7.30 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| ProTable | 115.0 µs | 43.2 µs | — | 1.90 ms / 21.96 ms / 288.02 ms | 8.02 ms | — | 否（Large100k） | 计划 T10/T11 | 待测 | 待测 | 待优化 |
| Progress | 62.6 µs | 12.6 µs | 14.2 µs | — | 5.89 ms | 19.53× | 是 | 计划 T10/T11 | 待测 | 待测 | 待优化 |
| Prompt | 64.9 µs | 8.6 µs | 46.0 µs | — | 8.03 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| Prompts | 52.2 µs | 2.3 µs | — | — | 7.53 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| QRCode | 281.5 µs | 6.6 µs | 6.7 µs | — | 612.4 µs | 50.62× | 是 | — | 待测 | 待测 | 待优化 |
| QueryFilter | 61.0 µs | 8.2 µs | — | — | 7.28 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| Radio | 51.5 µs | 2.4 µs | 8.8 µs | — | 7.94 ms | 362.80× | 是 | 计划 T10/T11 | 待测 | 待测 | 待优化 |
| RadioGroup | 53.1 µs | 3.2 µs | 8.7 µs | 1.49 ms / 219.09 ms / 2.51 s | 5.27 ms | 200.98× | 否（Large100k） | — | 待测 | 待测 | 待优化 |
| RangePicker | 51.7 µs | 1.6 µs | 50.8 µs | — | 7.60 ms | 6.98× | 是 | — | 待测 | 待测 | 待优化 |
| Rate | 52.1 µs | 1.9 µs | 1.9 µs | — | 5.18 ms | 366.75× | 是 | — | 待测 | 待测 | 待优化 |
| Result | 55.8 µs | 4.7 µs | 4.8 µs | — | 7.30 ms | 6.53× | 是 | — | 待测 | 待测 | 待优化 |
| RichText | 61.1 µs | 11.0 µs | 11.0 µs | 12.88 ms / 184.68 ms / 2.05 s | 9.11 ms | 89.01× | 否（Large100k） | — | 待测 | 待测 | 待优化 |
| RouteContext | 52.2 µs | 1.3 µs | — | — | 7.45 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| Row | 55.8 µs | 3.9 µs | 4.0 µs | — | 7.24 ms | 36.16× | 是 | — | 待测 | 待测 | 待优化 |
| RowEditorTable | 70.5 µs | 14.0 µs | 188.3 µs | 2.52 ms / 31.17 ms / 392.85 ms | 8.38 ms | 2.82× | 否（Large100k） | — | 待测 | 待测 | 待优化 |
| SchemaForm | 63.5 µs | 9.2 µs | — | — | 7.25 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| Scroll | 66.6 µs | 11.7 µs | 19.2 µs | 329.1 µs / 3.48 ms / 51.64 ms | 7.28 ms | 6.33× | 否（Large100k） | — | 待测 | 待测 | 待优化 |
| ScrollH | 64.9 µs | 10.9 µs | 18.3 µs | 330.4 µs / 3.46 ms / 51.68 ms | 7.27 ms | 16.88× | 否（Large100k） | — | 待测 | 待测 | 待优化 |
| Search | 52.9 µs | 2.8 µs | 9.8 µs | — | 7.29 ms | 155.77× | 是 | — | 待测 | 待测 | 待优化 |
| Segmented | 53.1 µs | 3.0 µs | 8.7 µs | 865.7 µs / 95.01 ms / 1.13 s | 7.32 ms | 208.36× | 否（Large100k） | — | 待测 | 待测 | 待优化 |
| Select | 50.9 µs | 1.5 µs | 6.1 µs | 15.4 µs / 15.4 µs / 15.4 µs | 7.44 ms | 27.00× | 是 | 计划 T10/T11 | 待测 | 待测 | 待优化 |
| Sender | 53.5 µs | 3.1 µs | 22.0 µs | — | 7.16 ms | 11.35× | 是 | — | 待测 | 待测 | 待优化 |
| SettingDrawer | 84.5 µs | 25.3 µs | — | — | 7.79 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| Skeleton | 52.6 µs | 2.5 µs | 2.7 µs | — | 142.8 µs | 16.07× | 是 | — | 待测 | 待测 | 待优化 |
| SkeletonAvatar | 49.3 µs | 689 ns | 697 ns | — | 130.1 µs | 810.00× | 是 | — | 待测 | 待测 | 待优化 |
| SkeletonButton | 49.4 µs | 691 ns | 699 ns | — | 149.2 µs | 632.81× | 是 | — | 待测 | 待测 | 待优化 |
| SkeletonImage | 49.6 µs | 767 ns | 779 ns | — | 153.8 µs | 105.47× | 是 | — | 待测 | 待测 | 待优化 |
| SkeletonInput | 49.3 µs | 692 ns | 702 ns | — | 139.8 µs | 162.00× | 是 | — | 待测 | 待测 | 待优化 |
| Slider | 52.8 µs | 3.2 µs | 31.7 µs | — | 5.28 ms | 21.16× | 是 | 计划 T10/T11 | 待测 | 待测 | 待优化 |
| SliderRange | 52.8 µs | 2.9 µs | 18.8 µs | — | 150.3 µs | 42.32× | 是 | — | 待测 | 待测 | 待优化 |
| Space | 53.1 µs | 3.4 µs | 3.4 µs | — | 7.36 ms | 42.32× | 是 | — | 待测 | 待测 | 待优化 |
| Spin | 50.9 µs | 1.7 µs | 1.7 µs | — | 7.28 ms | 179.00× | 是 | — | 待测 | 待测 | 待优化 |
| Splitter | 53.5 µs | 2.9 µs | 13.4 µs | — | 7.19 ms | 8.91× | 是 | — | 待测 | 待测 | 待优化 |
| StackBox | 50.4 µs | 1.1 µs | 1.1 µs | — | 7.27 ms | 33.75× | 是 | — | 待测 | 待测 | 待优化 |
| Statistic | 50.3 µs | 1.4 µs | 1.4 µs | — | 7.35 ms | 16.05× | 是 | — | 待测 | 待测 | 待优化 |
| StatisticCard | 56.8 µs | 4.6 µs | — | — | 7.39 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| StatisticCardDivider | 68.2 µs | 14.4 µs | — | — | 7.19 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| StatisticCardGroup | 51.2 µs | 1.5 µs | — | — | 8.07 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| StatisticCardOperation | 73.2 µs | 19.0 µs | — | — | 7.13 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| StatisticTimer | 50.7 µs | 1.4 µs | 1.5 µs | — | 7.23 ms | 16.05× | 是 | — | 待测 | 待测 | 待优化 |
| StatusBar | 51.4 µs | 1.9 µs | — | — | 5.00 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| StatusBarItem | 50.4 µs | 1.1 µs | — | — | 5.11 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| Steps | 55.0 µs | 4.7 µs | 12.2 µs | — | 7.32 ms | 17.24× | 是 | — | 待测 | 待测 | 待优化 |
| StepsForm | 49.3 µs | 681 ns | — | — | 152.5 µs | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| Switch | 51.8 µs | 1.9 µs | 5.1 µs | — | 7.25 ms | 502.69× | 是 | 计划 T10/T11 | 待测 | 待测 | 待优化 |
| SwitchValue | 51.3 µs | 2.0 µs | 5.2 µs | — | 5.23 ms | 545.38× | 是 | — | 待测 | 待测 | 待优化 |
| Table | 192.7 µs | 114.3 µs | 199.6 µs | 96.6 µs / 96.7 µs / 96.6 µs | 10.65 ms | — | 是 | 计划 T10/T11 | 待测 | 待测 | 待优化 |
| TableDropdown | 51.5 µs | 1.3 µs | 7.1 µs | — | 223.8 µs | 93.94× | 是 | — | 待测 | 待测 | 待优化 |
| Tabs | 57.8 µs | 6.3 µs | 18.2 µs | 2.34 ms / 376.67 ms / 4.48 s | 7.25 ms | 20.24× | 否（Large100k） | 计划 T10/T11 | 待测 | 待测 | 待优化 |
| Tag | 50.1 µs | 1.1 µs | 1.1 µs | — | 7.41 ms | 467.47× | 是 | — | 待测 | 待测 | 待优化 |
| Text | 49.7 µs | 912 ns | 922 ns | — | 7.12 ms | 42.32× | 是 | — | 待测 | 待测 | 待优化 |
| TextArea | 52.6 µs | 2.5 µs | 9.1 µs | — | 7.22 ms | 30.86× | 是 | 计划 T10/T11 | 待测 | 待测 | 待优化 |
| ThoughtChain | 70.9 µs | 18.7 µs | 47.8 µs | — | 7.68 ms | 7.63× | 是 | — | 待测 | 待测 | 待优化 |
| TimePanel | 148.4 µs | 44.6 µs | 80.7 µs | — | 5.60 ms | 33.83× | 是 | — | 待测 | 待测 | 待优化 |
| TimePicker | 51.0 µs | 1.5 µs | 48.2 µs | — | 7.06 ms | 26.01× | 是 | — | 待测 | 待测 | 待优化 |
| TimeRangePicker | 51.3 µs | 1.6 µs | 91.8 µs | — | 7.27 ms | 10.65× | 是 | — | 待测 | 待测 | 待优化 |
| TimeSelect | 53.7 µs | 2.9 µs | 16.6 µs | — | 5.09 ms | 28.12× | 是 | — | 待测 | 待测 | 待优化 |
| Timeline | 79.9 µs | 26.6 µs | 26.8 µs | 3.20 ms / 110.03 ms / 1.36 s | 7.30 ms | 3.88× | 否（Large100k） | — | 待测 | 待测 | 待优化 |
| Title | 50.3 µs | 918 ns | 928 ns | — | 7.25 ms | 24.50× | 是 | — | 待测 | 待测 | 待优化 |
| TitleBar | 79.4 µs | 27.8 µs | — | — | 5.30 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| Toggle | 50.6 µs | 1.2 µs | 3.7 µs | — | 7.14 ms | 908.20× | 是 | — | 待测 | 待测 | 待优化 |
| ToggleGroup | 53.7 µs | 3.4 µs | 9.7 µs | 906.3 µs / 217.07 ms / 2.49 s | 5.10 ms | 218.31× | 否（Large100k） | — | 待测 | 待测 | 待优化 |
| ToolCall | 61.6 µs | 9.8 µs | — | — | 7.65 ms | — | 是 | — | 待测 | 待测 | 无对手只看预算 |
| Tooltip | 54.8 µs | 3.6 µs | 2.8 µs | — | 7.20 ms | 143.33× | 是 | 计划 T10/T11 | 待测 | 待测 | 待优化 |
| TopNavHeader | 59.7 µs | 6.3 µs | 19.9 µs | — | 7.20 ms | 16.63× | 是 | — | 待测 | 待测 | 待优化 |
| Tour | 65.3 µs | 11.3 µs | 44.4 µs | — | 7.73 ms | — | 是 | 计划 T10/T11 | 待测 | 待测 | 待优化 |
| Transfer | 69.9 µs | 12.1 µs | 61.7 µs | 623.3 µs / 6.41 ms / 81.52 ms | 7.99 ms | 16.20× | 否（Large100k） | 计划 T10/T11 | 待测 | 待测 | 待优化 |
| Tree | 85.3 µs | 21.6 µs | 84.0 µs | 4.86 ms / 87.65 ms / 1.25 s | 5.68 ms | 4.16× | 否（Large100k） | 计划 T10/T11 | 待测 | 待测 | 待优化 |
| TreeSelect | 53.4 µs | 2.0 µs | 24.2 µs | 50.2 µs / 537.9 µs / 7.07 ms | 12.05 ms | 168.75× | 否（Large100k） | — | 待测 | 待测 | 待优化 |
| Upload | 57.8 µs | 3.3 µs | 13.6 µs | — | 7.27 ms | 10.35× | 是 | — | 待测 | 待测 | 待优化 |
| VirtualList | 74.7 µs | 16.4 µs | 53.5 µs | 40.1 µs / 40.3 µs / 40.3 µs | 7.96 ms | 8.44× | 是 | list（12b 待测） | 待测 | 待测 | 待优化 |
| VirtualSelect | 75.2 µs | 2.0 µs | 24.9 µs | 16.3 µs / 16.3 µs / 16.3 µs | 7.30 ms | 168.75× | 是 | — | 待测 | 待测 | 待优化 |
| VirtualTree | 75.2 µs | 15.9 µs | 63.3 µs | 64.7 µs / 65.8 µs / 65.7 µs | 5.53 ms | 2.91× | 是 | — | 待测 | 待测 | 待优化 |
| Watermark | 53.1 µs | 2.5 µs | 2.5 µs | — | 8.29 ms | 6.33× | 是 | — | 待测 | 待测 | 待优化 |
| WindowControls | 79.4 µs | 27.0 µs | — | — | 186.9 µs | — | 是 | — | 待测 | 待测 | 无对手只看预算 |

### 热点表（R1 `hot.md` 前 30）

得分算法同 12a：Σ（cum% × 超出预算的倍数）。这次一共 35 个超标场景（34 个 `Large100k` 加 Image/Mount），全部采样成功，没有非目标区域函数。

**与 12a 的口径差异：** F1 修正后，flat = 0 的纯调用者行会保留，所以前几名是计时框架和调用链上层的函数：`Runner.Frame`、`Driver.Frame`、`benchPerfScene`、`testing.(*B).runN`、`perfStep`、`perfScenes.func2.func1.1`、`runtime.systemstack`。它们只说明所有超标场景都经过帧管线，本身不是优化目标。另外 `pprof -nodecount=30` 是在每个 profile 内截取的，这些调用者行占掉了名额，一些 12a 进过前 30 的叶子函数在 `hot.md` 中排名靠后，例如 `(*TabsW).tab.func10`、`(*CommandW).item.func1`、`runtime.gcDrain`。所以 12a 与 R1 的得分和名次不能直接比较。

| # | 函数 | 得分 | 组件数 | 组件/场景 | 非目标区 |
|---:|---|---:|---:|---|:---:|
| 1 | `github.com/tangoui/tango/ui.(*Runner).Frame` | 10030.4751 | 35 | AutoComplete/Large100k, AvatarGroup/Large100k, BubbleList/Large100k, Carousel/Large100k, Cascader/Large100k, CascaderPanel/Large100k, CellEditorTable/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Collapse/Large100k, Command/Large100k, Conversations/Large100k, Descriptions/Large100k, Image/Mount, InfiniteScroll/Large100k, InputTag/Large100k, List/Large100k, Markdown/Large100k, Masonry/Large100k, Mentions/Large100k, Menu/Large100k, ProList/Large100k, ProTable/Large100k, RadioGroup/Large100k, RichText/Large100k, RowEditorTable/Large100k, Scroll/Large100k, ScrollH/Large100k, Segmented/Large100k, Tabs/Large100k, Timeline/Large100k, ToggleGroup/Large100k, Transfer/Large100k, Tree/Large100k, TreeSelect/Large100k | 否 |
| 2 | `github.com/tangoui/tango/ui.(*Driver).Frame` | 10029.4669 | 35 | AutoComplete/Large100k, AvatarGroup/Large100k, BubbleList/Large100k, Carousel/Large100k, Cascader/Large100k, CascaderPanel/Large100k, CellEditorTable/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Collapse/Large100k, Command/Large100k, Conversations/Large100k, Descriptions/Large100k, Image/Mount, InfiniteScroll/Large100k, InputTag/Large100k, List/Large100k, Markdown/Large100k, Masonry/Large100k, Mentions/Large100k, Menu/Large100k, ProList/Large100k, ProTable/Large100k, RadioGroup/Large100k, RichText/Large100k, RowEditorTable/Large100k, Scroll/Large100k, ScrollH/Large100k, Segmented/Large100k, Tabs/Large100k, Timeline/Large100k, ToggleGroup/Large100k, Transfer/Large100k, Tree/Large100k, TreeSelect/Large100k | 否 |
| 3 | `github.com/tangoui/tango/ui.benchPerfScene` | 9969.4552 | 35 | AutoComplete/Large100k, AvatarGroup/Large100k, BubbleList/Large100k, Carousel/Large100k, Cascader/Large100k, CascaderPanel/Large100k, CellEditorTable/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Collapse/Large100k, Command/Large100k, Conversations/Large100k, Descriptions/Large100k, Image/Mount, InfiniteScroll/Large100k, InputTag/Large100k, List/Large100k, Markdown/Large100k, Masonry/Large100k, Mentions/Large100k, Menu/Large100k, ProList/Large100k, ProTable/Large100k, RadioGroup/Large100k, RichText/Large100k, RowEditorTable/Large100k, Scroll/Large100k, ScrollH/Large100k, Segmented/Large100k, Tabs/Large100k, Timeline/Large100k, ToggleGroup/Large100k, Transfer/Large100k, Tree/Large100k, TreeSelect/Large100k | 否 |
| 4 | `github.com/tangoui/tango/ui.BenchmarkComponents.func1.1` | 9925.5116 | 35 | AutoComplete/Large100k, AvatarGroup/Large100k, BubbleList/Large100k, Carousel/Large100k, Cascader/Large100k, CascaderPanel/Large100k, CellEditorTable/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Collapse/Large100k, Command/Large100k, Conversations/Large100k, Descriptions/Large100k, Image/Mount, InfiniteScroll/Large100k, InputTag/Large100k, List/Large100k, Markdown/Large100k, Masonry/Large100k, Mentions/Large100k, Menu/Large100k, ProList/Large100k, ProTable/Large100k, RadioGroup/Large100k, RichText/Large100k, RowEditorTable/Large100k, Scroll/Large100k, ScrollH/Large100k, Segmented/Large100k, Tabs/Large100k, Timeline/Large100k, ToggleGroup/Large100k, Transfer/Large100k, Tree/Large100k, TreeSelect/Large100k | 否 |
| 5 | `testing.(*B).runN` | 9877.5751 | 35 | AutoComplete/Large100k, AvatarGroup/Large100k, BubbleList/Large100k, Carousel/Large100k, Cascader/Large100k, CascaderPanel/Large100k, CellEditorTable/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Collapse/Large100k, Command/Large100k, Conversations/Large100k, Descriptions/Large100k, Image/Mount, InfiniteScroll/Large100k, InputTag/Large100k, List/Large100k, Markdown/Large100k, Masonry/Large100k, Mentions/Large100k, Menu/Large100k, ProList/Large100k, ProTable/Large100k, RadioGroup/Large100k, RichText/Large100k, RowEditorTable/Large100k, Scroll/Large100k, ScrollH/Large100k, Segmented/Large100k, Tabs/Large100k, Timeline/Large100k, ToggleGroup/Large100k, Transfer/Large100k, Tree/Large100k, TreeSelect/Large100k | 否 |
| 6 | `testing.(*B).run1.func1` | 9800.9465 | 35 | AutoComplete/Large100k, AvatarGroup/Large100k, BubbleList/Large100k, Carousel/Large100k, Cascader/Large100k, CascaderPanel/Large100k, CellEditorTable/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Collapse/Large100k, Command/Large100k, Conversations/Large100k, Descriptions/Large100k, Image/Mount, InfiniteScroll/Large100k, InputTag/Large100k, List/Large100k, Markdown/Large100k, Masonry/Large100k, Mentions/Large100k, Menu/Large100k, ProList/Large100k, ProTable/Large100k, RadioGroup/Large100k, RichText/Large100k, RowEditorTable/Large100k, Scroll/Large100k, ScrollH/Large100k, Segmented/Large100k, Tabs/Large100k, Timeline/Large100k, ToggleGroup/Large100k, Transfer/Large100k, Tree/Large100k, TreeSelect/Large100k | 否 |
| 7 | `github.com/tangoui/tango/ui.(*Runner).layoutAndPaint` | 8693.8887 | 26 | AutoComplete/Large100k, Carousel/Large100k, Cascader/Large100k, CascaderPanel/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Collapse/Large100k, Command/Large100k, Conversations/Large100k, Image/Mount, InfiniteScroll/Large100k, InputTag/Large100k, List/Large100k, Markdown/Large100k, Mentions/Large100k, Menu/Large100k, ProList/Large100k, RadioGroup/Large100k, RichText/Large100k, Scroll/Large100k, ScrollH/Large100k, Segmented/Large100k, Tabs/Large100k, Timeline/Large100k, ToggleGroup/Large100k, Tree/Large100k | 否 |
| 8 | `github.com/tangoui/tango/ui/internal/flexbox.flex` | 8220.3863 | 21 | AutoComplete/Large100k, Cascader/Large100k, CascaderPanel/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Collapse/Large100k, Command/Large100k, Conversations/Large100k, InfiniteScroll/Large100k, InputTag/Large100k, List/Large100k, Markdown/Large100k, Mentions/Large100k, ProList/Large100k, RadioGroup/Large100k, Scroll/Large100k, ScrollH/Large100k, Segmented/Large100k, Tabs/Large100k, Timeline/Large100k, ToggleGroup/Large100k | 否 |
| 9 | `github.com/tangoui/tango/ui/text.Shape` | 8126.8335 | 21 | AutoComplete/Large100k, Cascader/Large100k, CascaderPanel/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Collapse/Large100k, Command/Large100k, Conversations/Large100k, Descriptions/Large100k, InfiniteScroll/Large100k, InputTag/Large100k, List/Large100k, Markdown/Large100k, Mentions/Large100k, ProList/Large100k, RadioGroup/Large100k, RichText/Large100k, Segmented/Large100k, Tabs/Large100k, Timeline/Large100k, ToggleGroup/Large100k | 否 |
| 10 | `github.com/tangoui/tango/ui/internal/flexbox.measureAxis` | 8068.5893 | 19 | AutoComplete/Large100k, Cascader/Large100k, CascaderPanel/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Collapse/Large100k, Command/Large100k, Conversations/Large100k, InfiniteScroll/Large100k, InputTag/Large100k, List/Large100k, Markdown/Large100k, Mentions/Large100k, ProList/Large100k, RadioGroup/Large100k, Segmented/Large100k, Tabs/Large100k, Timeline/Large100k, ToggleGroup/Large100k | 否 |
| 11 | `github.com/tangoui/tango/ui/internal/flexbox.measure` | 8067.9592 | 19 | AutoComplete/Large100k, Cascader/Large100k, CascaderPanel/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Collapse/Large100k, Command/Large100k, Conversations/Large100k, InfiniteScroll/Large100k, InputTag/Large100k, List/Large100k, Markdown/Large100k, Mentions/Large100k, ProList/Large100k, RadioGroup/Large100k, Segmented/Large100k, Tabs/Large100k, Timeline/Large100k, ToggleGroup/Large100k | 否 |
| 12 | `github.com/tangoui/tango/ui/internal/flexbox.measureUncached` | 8059.2184 | 19 | AutoComplete/Large100k, Cascader/Large100k, CascaderPanel/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Collapse/Large100k, Command/Large100k, Conversations/Large100k, InfiniteScroll/Large100k, InputTag/Large100k, List/Large100k, Markdown/Large100k, Mentions/Large100k, ProList/Large100k, RadioGroup/Large100k, Segmented/Large100k, Tabs/Large100k, Timeline/Large100k, ToggleGroup/Large100k | 否 |
| 13 | `github.com/tangoui/tango/ui/text.Width (inline)` | 7446.1450 | 15 | AutoComplete/Large100k, Cascader/Large100k, CascaderPanel/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Command/Large100k, Descriptions/Large100k, InputTag/Large100k, Markdown/Large100k, Mentions/Large100k, RadioGroup/Large100k, RichText/Large100k, Segmented/Large100k, Tabs/Large100k, ToggleGroup/Large100k | 否 |
| 14 | `github.com/tangoui/tango/ui/text.layout (inline)` | 7304.2673 | 18 | AutoComplete/Large100k, Cascader/Large100k, CascaderPanel/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Collapse/Large100k, Command/Large100k, Conversations/Large100k, Descriptions/Large100k, InfiniteScroll/Large100k, InputTag/Large100k, Mentions/Large100k, ProList/Large100k, RadioGroup/Large100k, Segmented/Large100k, Tabs/Large100k, Timeline/Large100k, ToggleGroup/Large100k | 否 |
| 15 | `github.com/tangoui/tango/ui/text.layoutRuns` | 7303.2591 | 18 | AutoComplete/Large100k, Cascader/Large100k, CascaderPanel/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Collapse/Large100k, Command/Large100k, Conversations/Large100k, Descriptions/Large100k, InfiniteScroll/Large100k, InputTag/Large100k, Mentions/Large100k, ProList/Large100k, RadioGroup/Large100k, Segmented/Large100k, Tabs/Large100k, Timeline/Large100k, ToggleGroup/Large100k | 否 |
| 16 | `github.com/tangoui/tango/ui/internal/flexbox.arrange` | 7023.9355 | 18 | CascaderPanel/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Collapse/Large100k, Command/Large100k, Conversations/Large100k, InfiniteScroll/Large100k, InputTag/Large100k, List/Large100k, Markdown/Large100k, ProList/Large100k, RadioGroup/Large100k, Scroll/Large100k, ScrollH/Large100k, Segmented/Large100k, Tabs/Large100k, Timeline/Large100k, ToggleGroup/Large100k | 否 |
| 17 | `github.com/tangoui/tango/ui/internal/flexbox.Solve (inline)` | 6936.8924 | 17 | CascaderPanel/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Collapse/Large100k, Command/Large100k, Conversations/Large100k, InfiniteScroll/Large100k, InputTag/Large100k, List/Large100k, Markdown/Large100k, RadioGroup/Large100k, Scroll/Large100k, ScrollH/Large100k, Segmented/Large100k, Tabs/Large100k, Timeline/Large100k, ToggleGroup/Large100k | 否 |
| 18 | `github.com/tangoui/tango/ui.perfScenes.func2.func1.1` | 6799.6239 | 32 | AutoComplete/Large100k, AvatarGroup/Large100k, BubbleList/Large100k, Carousel/Large100k, Cascader/Large100k, CascaderPanel/Large100k, CellEditorTable/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Collapse/Large100k, Command/Large100k, Conversations/Large100k, Descriptions/Large100k, InfiniteScroll/Large100k, InputTag/Large100k, List/Large100k, Markdown/Large100k, Masonry/Large100k, Mentions/Large100k, Menu/Large100k, ProTable/Large100k, RadioGroup/Large100k, RichText/Large100k, RowEditorTable/Large100k, Scroll/Large100k, ScrollH/Large100k, Segmented/Large100k, Tabs/Large100k, Timeline/Large100k, ToggleGroup/Large100k, Tree/Large100k, TreeSelect/Large100k | 否 |
| 19 | `github.com/tangoui/tango/ui/text.prepareText` | 6671.8908 | 16 | AutoComplete/Large100k, Cascader/Large100k, CascaderPanel/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Collapse/Large100k, Command/Large100k, Conversations/Large100k, Descriptions/Large100k, InputTag/Large100k, Mentions/Large100k, ProList/Large100k, RadioGroup/Large100k, Segmented/Large100k, Tabs/Large100k, ToggleGroup/Large100k | 否 |
| 20 | `github.com/tangoui/tango/ui.perfStep` | 6143.4830 | 30 | AutoComplete/Large100k, AvatarGroup/Large100k, BubbleList/Large100k, Carousel/Large100k, Cascader/Large100k, CascaderPanel/Large100k, CellEditorTable/Large100k, CheckAll/Large100k, Collapse/Large100k, Command/Large100k, Conversations/Large100k, Descriptions/Large100k, InfiniteScroll/Large100k, List/Large100k, Markdown/Large100k, Masonry/Large100k, Mentions/Large100k, Menu/Large100k, ProTable/Large100k, RadioGroup/Large100k, RichText/Large100k, RowEditorTable/Large100k, Scroll/Large100k, ScrollH/Large100k, Segmented/Large100k, Tabs/Large100k, Timeline/Large100k, ToggleGroup/Large100k, Tree/Large100k, TreeSelect/Large100k | 否 |
| 21 | `github.com/tangoui/tango/ui/text.shapeItems` | 5846.8275 | 15 | AutoComplete/Large100k, Cascader/Large100k, CascaderPanel/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Collapse/Large100k, Command/Large100k, Conversations/Large100k, Descriptions/Large100k, InputTag/Large100k, Mentions/Large100k, RadioGroup/Large100k, Segmented/Large100k, Tabs/Large100k, ToggleGroup/Large100k | 否 |
| 22 | `github.com/tangoui/tango/ui/text.(*compactShaper).append` | 5358.5855 | 15 | AutoComplete/Large100k, Cascader/Large100k, CascaderPanel/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Command/Large100k, Conversations/Large100k, Descriptions/Large100k, InputTag/Large100k, Mentions/Large100k, RadioGroup/Large100k, RichText/Large100k, Segmented/Large100k, Tabs/Large100k, ToggleGroup/Large100k | 否 |
| 23 | `github.com/go-text/typesetting/harfbuzz.(*Buffer).Shape` | 4998.1935 | 14 | AutoComplete/Large100k, Cascader/Large100k, CascaderPanel/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Command/Large100k, Conversations/Large100k, InputTag/Large100k, Mentions/Large100k, RadioGroup/Large100k, RichText/Large100k, Segmented/Large100k, Tabs/Large100k, ToggleGroup/Large100k | 否 |
| 24 | `github.com/go-text/typesetting/harfbuzz.(*shapePlan).execute (inline)` | 4846.6563 | 13 | AutoComplete/Large100k, Cascader/Large100k, CascaderPanel/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Command/Large100k, InputTag/Large100k, Mentions/Large100k, RadioGroup/Large100k, RichText/Large100k, Segmented/Large100k, Tabs/Large100k, ToggleGroup/Large100k | 否 |
| 25 | `github.com/go-text/typesetting/harfbuzz.(*shaperOpentype).shape` | 4355.1268 | 12 | AutoComplete/Large100k, Cascader/Large100k, CascaderPanel/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Command/Large100k, InputTag/Large100k, Mentions/Large100k, RadioGroup/Large100k, RichText/Large100k, Segmented/Large100k, ToggleGroup/Large100k | 否 |
| 26 | `github.com/go-text/typesetting/harfbuzz.(*otMap).apply` | 3361.5551 | 10 | Cascader/Large100k, CascaderPanel/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Command/Large100k, InputTag/Large100k, RadioGroup/Large100k, RichText/Large100k, Segmented/Large100k, ToggleGroup/Large100k | 否 |
| 27 | `github.com/go-text/typesetting/harfbuzz.(*otApplyContext).applyString` | 3202.0439 | 9 | CascaderPanel/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Command/Large100k, InputTag/Large100k, RadioGroup/Large100k, RichText/Large100k, Segmented/Large100k, ToggleGroup/Large100k | 否 |
| 28 | `github.com/go-text/typesetting/harfbuzz.(*otApplyContext).applyForward` | 3132.6814 | 9 | CascaderPanel/Large100k, CheckAll/Large100k, CheckboxGroup/Large100k, Command/Large100k, InputTag/Large100k, RadioGroup/Large100k, RichText/Large100k, Segmented/Large100k, ToggleGroup/Large100k | 否 |
| 29 | `runtime.systemstack` | 2260.1657 | 24 | AvatarGroup/Large100k, BubbleList/Large100k, Carousel/Large100k, Cascader/Large100k, CellEditorTable/Large100k, Collapse/Large100k, Conversations/Large100k, Descriptions/Large100k, Image/Mount, InfiniteScroll/Large100k, List/Large100k, Markdown/Large100k, Masonry/Large100k, Menu/Large100k, ProList/Large100k, ProTable/Large100k, RichText/Large100k, RowEditorTable/Large100k, Scroll/Large100k, ScrollH/Large100k, Timeline/Large100k, Transfer/Large100k, Tree/Large100k, TreeSelect/Large100k | 否 |
| 30 | `github.com/tangoui/tango/ui.entTextW` | 2248.1197 | 6 | AutoComplete/Large100k, Cascader/Large100k, CascaderPanel/Large100k, InputTag/Large100k, Mentions/Large100k, Segmented/Large100k | 否 |

去掉框架和调用链行之后，排在前面的仍是两条公共管线：

- 布局：`flexbox.flex`（21 个场景）、`measureAxis` / `measure` / `measureUncached`（19）、`arrange`（18）。
- 文字：`text.Shape`（21）、`text.Width`（15）、`text.layoutRuns`（18）、`prepareText`（16）、`shapeItems`（15）、`compactShaper.append`（15），以及 harfbuzz `(*Buffer).Shape` 及其下层函数。

`ui.entTextW`（6 个场景）是新进前 30 的组件级函数，出现在 AutoComplete、Cascader、CascaderPanel、InputTag、Mentions、Segmented 中。

### 转交清单（R1 复核）

12a 的 P1–P3 仍然有效，R1 数值如下。没有新增转交项。热点里依然没有非目标区域函数。

- P2 Command：基准 777.73 ms/步，预算 1.15 ms。
- P3 LogView：各场景在预算内，基准中位数 Mount 92.8 µs，Interact 37.9 µs，Large100k 25.0 µs。

### 第 1 批热点候选（R1 刷新）

依据 R1 `hot.md` 和 R1 基准中位数：

1. **非虚拟化的 10 万项集合**，每步重建并布局全部子项，耗时随 n 线性增长：
   - Tabs 4.48 s/步、Tree 1.25 s、List 1.08 s、InfiniteScroll 889.86 ms、Command 777.73 ms（P2）、Cascader / CascaderPanel 613.28 / 614.09 ms、Menu 96.12 ms、Scroll / ScrollH 51.64 / 51.68 ms。
   - **新增 CellEditorTable / RowEditorTable** 393.06 / 392.85 ms。profile 中是 `tblFrame.buildRow` / `cell` / `ui.Key` 对全部行构建，加上 GC。
   - 同样线性增长的还有：CheckAll、CheckboxGroup、RadioGroup、ToggleGroup（2.49–2.55 s）、InputTag 2.27 s、Descriptions 2.15 s、RichText 2.05 s、ProList 2.02 s、Collapse 1.74 s、Mentions / AutoComplete 1.43 / 1.41 s、Timeline 1.36 s、Segmented 1.13 s、Markdown 1.10 s、Conversations 701.08 ms、ProTable 288.02 ms、Carousel 263.52 ms。
   - 逐个判定是“补虚拟化”还是“定为不适用”，不得为了过门槛改夹具。
2. **已虚拟化但仍超预算：**
   - Transfer 81.52 ms：12a 为 43.67 ms，1ceffa2 之后翻倍。按 `targetKeys` 排序时的全量映射构建是第一个要处理的点，并且必须保持 antd `unshift` 语义。
   - AvatarGroup 19.16 ms、TreeSelect 7.07 ms、BubbleList 5.85 ms、Masonry 443 µs（门槛预算约 97 µs）。
3. **公共管线：** 布局（`flexbox.flex` / `measure*` / `arrange`）和文字（`text.Shape` / `layoutRuns` / `prepareText` / `shapeItems` / `compactShaper.append` + harfbuzz）仍是多个组件共享的首要目标。上游文字提交（acb8e99、3898e06）只改善了冷启动和内存，没有改变每步的 shaping 开销。
4. **Image/Mount：** 基准 660.2 µs（门槛较差值 689 µs），仍是唯一超标的首次构建。它的 Interact 是全部 Interact 中最慢的（588.1 µs）。
5. **新观察项（达标，但上游合并后明显变慢）：**
   - QRCode/Mount：281.5 µs，3.16×，约为预算的 67%，boostLevel 每次构建都重复编码，可以缓存。
   - FooterToolbar/Unchanged：2.83×。
   - Tour/Interact：1.45×。
   - 这三项不在债表内，作为低成本附带项。
6. **局部重绘：** `perfDamageDebt` 153 项（含 M1 新增 7 项），与时间预算分开处理。

## 历史：Task 12a 基线（5f115d3）

以下为 Task 12a 的原始记录（含 F1 的更正），保留作对照，数值不再更新。12a 的时间判定与本文 R1 一致的部分见上文“与 12a 的差异”。

### 证据与环境

- 机器：Apple M4（Mac16,10），10 核，16 GB，macOS 26.6（25G72），Go 1.27.2 darwin/arm64。
- Tango：`5f115d300fe68054765fee1c2ce7a8bd498e2126`（分支 `perf/ui-components-20261116`）。
- 对手版本（供 12b 使用，本版未运行）：MyGo v0.9.0（自绘 `ui` 模式）；NSGo 源码 changelog 为 Unreleased，go tree SHA1 `ca0381d685af758f880c7775824adc38380b9ac7`。
- 原始数据在工作区的 `.superpowers/sdd/2026-11-16-ui-component-performance/baseline-12a/`（已 gitignore，不入库）：`bench-raw.txt`、`bench.json`、`status.md`、`hot.md`、`prof/`、`scratch{1,2}.txt`、`cold14.txt`、`gate1.txt`、`gate2.txt`、`gate3.txt`、`bench.orig.json`、`status.orig.md`、`budget-raw.txt`、`load.log`。

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

本任务的各个步骤是串行运行的（上一步结束后才开始下一步），但每次门槛内部 `./ui` 与 `./examples/ui-gallery` 两个包是并行运行的（go test 默认的 `-p`），影响见下一段。Task F1 已让 `uiperf bench` 的预算测试改用 `-count=1 -p 1`（包串行），以后的运行不再有这种重叠。约 03:30 起，另一个会话启动了自己的 GUI 基准（`tango-scenes`、`mygo-scenes grid`，伴随 WindowServer / fontd 活动），所以门槛 2 结束时负载升到 5.62：它的后段（T 开头之后的组件）比门槛 1 慢，最多约 2.7 倍，但仍在 < 6 的范围内，而且全部判定与门槛 1 一致。门槛 3 结束时负载 6.97 ≥ 6，标为受干扰，不用于债表取值；它的判定与门槛 1、2 全部一致，也是 PASS。

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

### 结果概要

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

### 逐组件状态表

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

### 热点表（`hot.md` 前 30）

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

### 转交清单

本次进程内热点**没有一项落在非目标区域**。原因之一是 no-op 渲染器把渲染器后端排除在计时之外，渲染器侧的证据要等 12b 的 WindowServer CPU / footprint 数据。下表列出已知要转交、或要留给后续批次的产品问题：

| 编号 | 问题 | 证据 | 归属 |
|---|---|---|---|
| P1 | `NoAnimations` 下 Message 提示关闭后不会从树中移除（不透明度约 0 时仍在） | `ui/nav_notify.go:225-231` 的 `renderMessages` 只在 `start+dur` 处 `WakeAt`，没有在 `start+dur+DurationMid` 处唤醒，最后一次过滤从不发生（Task 5 发现） | 本项目优化批次：补 `WakeAt`，并加回归测试（`DurationMid` 后浮层数为 0） |
| P2 | Command 未虚拟化 | `Command/Large100k` 768.46 ms/步，预算 1.04 ms；Large1k 0.70 ms；`(*CommandW).item.func1` 进入热点前 30 | 本项目第 1 批候选 |
| P3 | LogView 首帧按 200 px 占位高度布局，之后不再请求帧，静止画面缺行 | 夹具在首次构建后多跑一帧绕过；`TestPerfCollLogViewFirstFrame` 修复后会翻转（Task 6）。LogView 本次各场景都在预算内（基准中位数）：Mount 87.4 µs，Interact 37.1 µs，Large100k 24.6 µs | `perf/memory-logview` 会话 |
| — | 渲染器后端（合成器 CPU、提交、footprint） | 进程内不可见 | 待 12b；若有证据再转渲染器会话 |
| — | 终端 / 编辑器 | 本项目夹具不覆盖 `ui/terminal`、`ui/editor` 内核，profile 中未出现 | 无转交 |

### 第 1 批热点候选

依据 `hot.md` 和本次实测（以下时间为基准中位数）：

1. **非虚拟化的 10 万项集合**（每步重建并布局全部子项，耗时随 n 线性增长）：Tabs（4.44 s/步）、Tree（1.09 s）、List（1.02 s）、InfiniteScroll（867.66 ms）、Cascader / CascaderPanel（609.90 / 613.41 ms）、Menu（91.04 ms）、Command（768.46 ms，P2）、Scroll / ScrollH（50.35 / 50.78 ms）。同样线性增长的还有 CheckAll、CheckboxGroup、RadioGroup、ToggleGroup（约 2.5 s）、InputTag（2.25 s）、Descriptions（2.07 s）、RichText（2.02 s）、ProList（1.78 s）、Collapse（1.72 s）、Mentions、AutoComplete（约 1.4 s）、Timeline（1.37 s）、Segmented（1.11 s）、Markdown（1.08 s）、Conversations（694.70 ms）、ProTable（277.77 ms）、Carousel（243.53 ms）。其中有些是可放大夹具，产品上未必需要支持 10 万项；这一点在批次里逐个判定是“补虚拟化”还是“定为不适用”，不得为了过门槛改夹具。
2. **已虚拟化但仍超预算**：Transfer（43.67 ms）、AvatarGroup（18.12 ms）、BubbleList（5.69 ms）、TreeSelect（5.82 ms）、Masonry（441 µs，预算 95 µs）。
3. **公共管线**：`flexbox.flex` / `measure` / `measureUncached` / `arrange` / `measureAxis` 出现在 21 个超标场景里，说明布局对不可见子项也做了完整测量。文字路径 `text.layoutRuns` / `compactShaper.append` / `text.Shape` / `prepareText` / `shapeItems` 加上 harfbuzz 出现在 15 个超标场景里，说明重复 shaping 没有命中缓存。这两条是“多个组件共享”的优先目标。
4. **Image/Mount**：653 µs，是唯一超标的首次构建；它的 Interact 也是所有 Interact 中最慢的（586 µs）。
5. **局部重绘**：`perfDamageDebt` 有 146 项，大多数交互帧重绘了整个窗口（如 Icon 2250×、IconButton 2677×、Kbd 1592×、CheckableTag 1042×）。这是 damage 债，与时间预算分开处理。
