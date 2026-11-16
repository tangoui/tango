# M1 → final 成对矩阵准备说明

状态：只读/准备完成，尚未构建、运行或采样。

## 基线与适配

- M1：`0ef80e4`，由 `ui/editor/perfab/m1-final.sh` 用 `git archive` 导出到临时目录。
- final：当前隔离 worktree（含 F Wait 适配、P retained-copy 候选和已修复的 `ui/editor/diffview/perf_test.go` Wait benchmark helper）；因此候选不是“纯 F+P-only”树，主仓不参与。
- 两臂统一：1920×1080 逻辑像素、scale 2（3840×2160 设备像素）、同一 `perfTemplates` 生成文件、同一 Metal 离屏渲染路径和独立进程。
- M1 原始 `ui/gpu/metal.Device` 没有公开 `Wait`，overlay 在临时树中给它增加同实现语义的 `Device.Wait()`；overlay 同时替换 M1 `ui/editor/perf_test.go` 的 `syncDevice.wait`，删除合成 clear/1px frame，保留 M1 的 benchmark 名称、输入、视口、尺寸和渲染宿主。
- overlay 不把 F/P 生产 Metal 候选复制到 M1；M1→final 是版本演进比较，F-only→F+P 仍是另一组单变量成本比较，不能互换。

## 可直接比较的共享 benchmark

后续正式运行时只使用两臂都存在、且输入与尺寸相同的共同 gate：

- `BenchmarkTypeAt1MLines/{mid,end,mid-bg}`
- `BenchmarkScroll/scale2`
- `BenchmarkOpen100MB`

脚本不把 `Open100MBWrap`、`Open100MBSteps`、`Memory100MB`、`TypeAt1MLinesPartial`、`ScrollCaretFar`、`ScrollJump` 或 TS20k 等重/专项 benchmark 默认塞进这组 20 轮矩阵；它们可在单独预算和资源交接后补跑。真正 build 前脚本会：

1. 检查两臂原始 `perfTemplates` hash，并输出实际 hash；
2. 只在各自 module root 内执行 `go test -overlay ... ./ui/editor`，避免临时 M1 树落到主 module 外；
3. 输出两臂 `git`/二进制标识，候选若含额外 diffview Wait 修复必须单列。

建议共同 gate 至少 20 个有效轮，并保留 M1 A/A 控制臂；每轮按 ABAB/轮换顺序运行，新进程、负载 `< 6`，丢弃高负载轮，输出 p50/p95/p99、fps/open-ms、cpu-ms、gpuwait-ms 和 paired CI。

## 不可直接比较或只能补充报告的项目

- `BenchmarkScrollPartial`：final 的 Retainer/ScrollRegion 局部 copy 专项，M1 没有同名 benchmark；不能把它当 M1→final 的共享滚动门槛。
- `BenchmarkTypeTS20k`、`BenchmarkScrollTS20k`、`BenchmarkTypeAt1MLinesCPU`：final 新增，M1 没有；只能作为 final 补充绝对数，不能形成 M1 比值。
- `BenchmarkM2ColdOpen100MB`、`BenchmarkM2SingleLineEdit`、`BenchmarkTypeLongLineWrap`：final/M2 后续专项，M1 没有同口径基准；不能替代 M1 paired。
- `TANGO_EDITOR_PERF_OFF`：M1 保持默认配置，不需要支持开关；正式共同矩阵可使用四臂 `m1`、`m1-aa`、`final-default`、`final-alloff`，其中 `final-alloff` 只用于 final 默认→all-off 的补充成本比较。M1→final 不得把 all-off 误当作 M1 臂；逐项 OFF 仍另报告。
- Monaco 对照：headless 历史数据不 GPU-complete；本矩阵不引入 Monaco 数字，也不把 Monaco trace 探针结果混进 M1→final。
- 真实窗口、present/vsync、IME、Windows/Linux 实机：两臂都是离屏 benchmark，不能从本矩阵判定。

## M1 与 final 的接口/路径差异

1. **包路径**：M1 使用 `ui/gpu`、`ui/gpu/metal`；final 使用 `internal/render/gpu`、`internal/render/gpu/metal`。overlay 只解决 M1 测试树的 Wait 适配，不证明两套 Metal 后端实现逐行等价。
2. **完成等待**：M1 原始 helper 会提交合成 clear/1px frame；正式矩阵必须使用 overlay 后的真实 `Wait()`，否则会把计时器修正误报成产品性能收益。所有臂必须使用同一等待口径。
3. **局部重绘**：M1 的共享 `TypeAt1MLinesPartial` 使用旧 retained/syncDevice 夹具；final 同名 benchmark 使用当前 Retainer 和 damage 逻辑，结果代表 M1→final 的真实演进，但不等价于 final 的 `ScrollPartial` P copy 专项。
4. **功能集合**：final 有括号参考线、颜色注入、小地图字符、稀有查询续扫、detectIndentation 等 M2 能力；M1 没有。M1→final 的结果是整体版本退化/收益，不是每项能力的因果归因。
5. **Metal 候选**：P 的 spare/ping-pong、33.18 MB/Device 原生成本只存在 final；M1 arm 不启用 P。P 成本必须另列，不能把 F-only→F+P 结果写成 M1→final 结果。

## 哪些门槛只在百万行降级场景生效

规格中的“每项显示能力新增成本 ≤10%”只在百万行 paired 场景判定。大文件降级后，以下能力默认关闭：

- minimap
- sticky scroll
- folding
- occurrences / selection highlight
- links
- color decorators
- bracket pair coloring

百万行默认仍实际作用、需要纳入 M1→final 统一口径的是：overview、indentation guides、match brackets、Unicode highlight、line highlight，以及基础编辑/布局/高亮路径。

因此：

- 百万行输入 p95、滚动 fps/p99、100 MB 打开和 HeapInuse 是正式绝对门槛候选；
- M1→final 的完整显示能力比值只能对共享默认场景判定，不能从 TS20k 的 aggregate cost 外推到百万行；
- TS20k 仅补充“非大文件能力成本”和后台高亮结果，不替代 M1≤10% 门槛；
- 1 MB 长行换行和 final 的专项 benchmark 需要单独报告，不冒充 M1 共享矩阵。

## 后续执行边界

当前只准备了 `ui/editor/perfab/m1-final.sh`，默认只导出临时树、生成 overlay 并打印命令；本轮没有执行 `go test -c`、浏览器、benchmark 或采样。待资源明确交接且用户允许正式采样后，先审查 overlay/build，再跑矩阵；任何锁屏/解锁切换都要分组，不能混合统计。
