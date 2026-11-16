# Tango 全模块性能执行计划

> 执行方式：当前会话直接推进，按批完成独立可验证结果。

**Goal:** 所有模块建立性能覆盖表；每次优化必须有稳定正确性证据和基准证据。
**Architecture:** 模块清单 → 串行基线 → profile → 失败回归 → 最小优化 → 同场景复测。
**Spec:** docs/superpowers/specs/2026-11-16-all-module-performance-design.md

## 约束

- 保留全部语义/API/平台能力；不修改其他会话工作。
- 隔离分支 perf/all-modules-20261116，基线 40ed885。
- 对所有包明确记录覆盖状态；不冒称每个模块都有 MyGo 等价对照。

## 任务

- [x] 盘点当前平台所有 171 个包，记录 20 个 benchmark 包。
- [x] 顺序运行 14 个包、81 个子场景，记录 ns/op、B/op、allocs/op、负载、命令及提交。
- [x] 第一批：为 graph.coords 和 isotonicGap 建立 CPU/alloc profile；若重复工作区分配明显，增加复用/所有权回归，局部复用工作区。
- [x] 第一批：为 chart.LTTB 的稀疏/稠密和 NaN 输入建立对照及分配回归，减少临时索引成本。
- [ ] 第一批：评估 buffer.NewSnapshot CRLF，保留换行统计/EOL 与 Unicode 语义。
- [x] 第一批：用通用 Monarch 对照快速 Go lexer 的数字、转义、注释、运算符及自定义 grammar；出现偏差先修复。
- [x] 补齐未跑 benchmark 的 editor/highlight、editor/diffview、Metal、gallery 和 shader/compiler 库；无法在本机有效测量的平台标注待设备。
- [ ] 补齐 IPC/绑定/服务/CLI/i18n/资源的代表负载和基准。
- [ ] 每项修改后运行针对性测试、语义/分配回归及 A/B；批末运行适当全量检查，更新覆盖表。

## 重点审查

- 临时切片复用不能把结果暴露为可变共享缓冲。
- NaN、无穷和退化图/空图的输出保持不变。
- CR/LF 混合、连续 CR 和多字节字符的归一化/EOL 判定保持不变。
- lexer 优化不能通过遗漏 token、使用错误状态或忽略自定义 grammar 赢得基准。
- 性能测量不能并行运行或混合初始化和稳态指标。

## 第一批验证结果

- 26 包、138 个场景完成测量；完整测试、修改相关包 race、全项目 vet/格式/diff check 通过。
- Graph：分配字节 -66.5%、分配次数 -79.0%、耗时 -9.7%。
- LTTB：分配字节 -85.8%、耗时 -19.2%。
- KV Set/1000：分配字节 -99.96%、耗时 -98.5%。
- Go lexer：语义修正后的固定语料新鲜对照比 MyGo v0.9.0 耗时低 61.6%，分配仍更多。
- 全部模块持续优化任务尚未完成，详见 docs/performance/2026-11-16-module-coverage.md；不把第一批当作整个项目验收。
