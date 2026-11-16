# 编辑器输入与滚动性能实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (用户已选择 Native). Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 降低编辑器输入/滚动路径的重复分配和无效工作，以成对基准验证实际收益，不降低正确性及已有性能门槛。

**Architecture:** 基于 b7a963c（已含 3978731 完整括号 token 缓存）做 profile 驱动的局部优化：小地图 token 查询使用视图自有 scratch；字符位图避免临时逐格数组；按真实 profile 决定是否延迟构造分词 rune 索引。不改变默认显示能力和渲染器。

**Tech Stack:** Go 1.27，真实 ui/editor/Highlighter/Monarch/Metal 离屏基准，Go tests/benchmarks/race。

**Spec:** docs/superpowers/specs/2026-11-15-editor-m2-perf-acceptance-design.md；docs/specs/code-editor.md §6.6；用户本轮已批准“输入和滚动稳定性优先、核心吞吐同步优化”，Native 执行。

## Global Constraints

- 不新增依赖，不复制其他编辑器的代码；不改 GPU、宿主、公共 API。
- 不降低 gateLimit/高亮预算、不关闭默认功能、不修改测试输入制造优势。
- 原主仓库和保留工作区不动；仅在当前 worktree 执行。
- 时间性能至少三次同二进制/基线交替；记录负载、CPU 与 GPU 等待分开，高负载数字仅作诊断；分配计数可独立判定。
- 全仓失败必须按名报告；不能把调度噪声写成已优化的产品瓶颈。

## Review Focus

1. 小地图 scratch 不能被缓存行、tokens、blocks、旧绘制列表持有；测后续行与编辑不会污染旧行。
2. UTF-16 列与字节偏移保持一致；中文、emoji、组合字符、非法 UTF-8 与 tab 的像素结果不变。
3. 词法规则动态状态、嵌入语言、回溯与超时降级保持不变；新增差异测试覆盖 rune 延迟转换。
4. 主题/lexer 重置与折叠/隐藏区域、view zones 的小地图缓存仍准确失效。
5. 多光标/大文件仍遵守原工作预算，收益按真实路径计量，不靠放宽断言。

## Task 1: 小地图缓存查询 scratch

**Files:** ui/editor/minimap.go；新 ui/editor/minimap_perf_test.go。
**Interfaces:** 视图保留状态新增私有 token scratch，PeekTokenRange/TokenizeSpan 保持已有 API；任何长期 token 存储必须 Clone。
- [ ] 用真实 Highlighter 建立未修改小地图预热 fixture，测 prepareMinimap（不含 GPU）及 AllocsPerRun；新增“预热后查询分配不随采样行数线性增加”的失败测试和缓存独立性测试。
- [ ] 跑红灯，保存 before benchmark。
- [ ] 复用私有 tokens buffer 与 keep map，清理引用；不改变缓存重建条件。
- [ ] 跑 green、所有 minimap/golden/damage 测试，bench 前后对比。
- [ ] 单独提交。

## Task 2: 字符位图栅格化减少临时格子分配

**Files:** ui/editor/minimap_chars.go；ui/editor/minimap_chars_test.go 或 minimap_perf_test.go。
**Interfaces:** miniCharsAt 签名不变；colorAt 仍按字节单调调用且每个可绘制 grapheme 仅一次。
- [ ] 写失败的分配上界测试（120 ASCII 字符构造位图不随格子数组增长分配），保留现有 minimap 黄金像素测试。
- [ ] 先扫显示宽度再直接栅格化，不分配临时 cell 列表；不把 callback 调用两次。
- [ ] 逐项验证空白、tab、中文、emoji、combining、非法 UTF-8、1x/2x 像素及 callback 顺序。
- [ ] 跑 benchmark、测试并单独提交。

## Task 3: 分词 rune 索引按需构造（profile 成立时）

**Files:** ui/editor/monarch/lexer.go；新 ui/editor/monarch/lazy_runes_test.go。
**Interfaces:** 不改变 token/state/LineInfo 或预算常量；只在实际尝试 Backtrack matcher 时构造整行 rune 索引。
- [ ] 真实纯 RE2 长行 grammar 先建立分配红灯测试，与短于 256 字节控制组比较；回溯 grammar + Unicode + 动态规则差异测试。
- [ ] 将 newLineRunes 从逐位置无条件准备移到过滤后首次 Backtrack 匹配，按需切分 restRunes。
- [ ] 测完整 token/UTF-16/状态一致、已接受降级/超时回退不变；跑 Monarch/regex/langs 测试及 benchmark。
- [ ] 若无可重复收益则不保留此优化，并写报告；否则单独提交。

## Task 4: 验收与独立终审

**Files:** 新 docs/specs/2026-11-16-editor-input-scroll-perf-report.md，CHANGELOG.md。
- [ ] 基线和优化后二进制交替运行 TypeTS20k / ScrollTS20k / TypeLongLineWrap / ScrollPartial / TypeAt1MLinesCPU、分配微基准；≥3 次，正式耗时判据负载 <6；负载持续过高则明确标记时间结果未验证。
- [ ] 运行 go test ./ui/editor/...；race（不伪造时间预算通过）；三平台 vet；全仓 go test -p 1 ./... 与 API 快照检查。
- [ ] 写前后 CPU、GPU 等待、分配和门槛报告，记录跳过/失败，不把离屏吞吐称为实窗呈现。
- [ ] 一个 fresh reviewer 审整分支，重要问题用 RED→GREEN 修复；通过后按用户授权合并本地 main（不擅自推送）。

## 自检

四个任务均有具体文件、行为测试、性能验证和提交边界；scratch/像素/分词三条产品路径对应 Review Focus。已有括号缓存已在基线，不重复实现。源报告存在 Monaco GPU-complete、IME、Windows 未验证项，本轮不改变其状态。
