# 终端性能 A→B 最终报告

日期：2026-11-16
分支：`feat/terminal-perf`
设计：`docs/specs/2026-11-15-terminal-performance-design.md`

## 结论

终端性能 A→B 两阶段完成：

- 输入从稳定约 33ms 两帧延迟降到约一帧：macOS 实机 3 次 p95 为 **16.45–16.58ms**，p99 为 **16.52–16.60ms**。
- VT/视图现在使用版本化只读快照；VT 锁只覆盖快照复制/复用，绘制在锁外完成。
- 行缓存按稳定行 ID、版本、字体指标、主题和选区键控；滚动时跨 viewport 位置复用未变化行的分段计划。
- idle frame 的受控 benchmark 从 A 前约 92–104µs/1.6KB 降为约 **90–93µs/848B**；单脏行约 **190µs/20.4KB**，滚动约 **196–206µs/20.6KB**。
- 50MB 实机输出 3 次为 **333–567ms、89–152MB/s、57–58fps**；吞吐受桌面 vsync/调度和输出批次影响，帧率稳定但尚未达到严格 60fps。

## A→B 改动

### A

- 终端事件在 `Runner.HandleEvent` 阶段进入终端专用实验性通道；普通文本、Enter、方向键等不再等到 paint 才发送。
- 普通快捷键、AltGr、Option-as-Meta、IME 组合仍由 frame 路径保护，避免剪贴板、焦点和组字回归。
- `SendInput` 为交互输入保留额度；大 payload 超限整批拒绝并报告截断。
- 读缓冲从 32KiB 增至 256KiB，单次持锁仍受 256KiB 上限约束。
- 普通 Resize 不再在生命周期锁内调用 Conn；Close 能解除并发 Resize。初始 Start Resize 保持旧锁顺序以保留 Start/Close no-late-resize 保证。

### B

- `vt.Snapshot` 提供只读、可复用的 viewport 行、样式、光标、scrollback 元数据；快照在 VT 版本不变时直接复用。
- 增量快照按稳定行 ID 跨滚动复用未变化行的 Cell/组合字符数据；只复制新行或内容变化行。
- `rowCache` 缓存行级分段计划，容量上限 2,048 行/16MiB；缓存键包含行 ID/version、列数、metrics、完整 Scheme 和选区范围。
- `ViewW.paint` 解锁 VT 后使用快照绘制；cursor、IME、selection overlay 保持动态，避免旧光标和旧选区进入缓存。

## 验证

- `go test ./ui/terminal/... ./sys/pty/...`：通过。
- `go test -race ./ui/terminal/... ./sys/pty/...`：通过。
- `CGO_ENABLED=0 GOOS={darwin,linux,windows} go vet ./ui/terminal ./sys/pty`：通过。
- `go test ./...`：通过。
- 快照测试：独立性、行版本、样式代数、Release、未变化行复用：通过。
- 行缓存测试：命中、行版本/选区失效、容量上限、真实 View 命中：通过。
- macOS 实机 `-auto`：3 次 100 键全部完成，p95 **16.45–16.58ms**。
- macOS 实机 `-bench-cat`：3 次 50MB 输出 **333–567ms**，**57–58fps**。

## 最终 benchmark（Apple M4，正常 Go runtime，3 次）

| 场景 | 结果 |
|---|---:|
| ASCII VT | 0.67–0.72ms / 94.2KiB 输入 |
| ANSI VT | 0.99–1.01ms / 127.0KiB 输入 |
| PTY 风格 VT | 0.85–0.86ms / 90.1KiB 输入 |
| Idle frame | 90–93µs，848B，21 allocs |
| 单脏行 frame | 190–192µs，约 20.4KB，226 allocs |
| 滚动 frame | 196–206µs，约 20.6KB，226 allocs |

## 未达标/后续

- 实机输出帧率为 57–58fps，距离严格 60fps 仍有差距；下一步应继续 profile `paint.CPU.Render`、Present/vsync 和宿主调度，不能只继续优化 VT 解析。
- 50MB 吞吐还没有和 xterm.js WebGL 做同机对照，因此只能记录实测，不能宣称超过其 1.5 倍目标。
- 缓存已经实现硬上限，但当前报告没有单独测完整 10,000 行回滚的 Go 堆增量；合并前应补一次内存采样。
- 全仓测试本次通过；若后续其他会话再次触发独立 `ui/editor` 性能阈值失败，应单独标注，不归因终端。
