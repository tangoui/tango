# 终端性能 A 阶段报告

日期：2026-11-16
对应设计：`docs/specs/2026-11-15-terminal-performance-design.md`

## 结论

A 阶段已完成：输入事件直通、交互输入保留额度、读缓冲扩大、Resize/Close 生命周期解耦均已通过终端回归和竞态测试。输入直通解决了原先按键必须等 `paint` 才发送的问题；IME、AltGr、Option-as-Meta、Ctrl+Tab、滚动回底和应用快捷键仍由正确性路径保护。

A 阶段没有继续做激进的 `paintRow` 微优化。受控 profile 显示 CPU renderer 的主要成本是通用 rasterizer 的 `paint.CPU.Render`、`fillRect` 和 glyph 栅格化；`ViewW.paintRow` 约占累计 CPU 的 4.7%。继续优化它不会改变整体上限，下一阶段应优先做版本化快照、行级绘制缓存和 damage/renderer 配合。

## 改动

- `ui.Context.TerminalEvents` + `Runner.HandleEvent`：终端事件在下一帧前进入终端专用处理器。
- `Terminal.SendInput`：小型交互输入使用保留额度；大 payload 超限时整批拒绝并返回 `SendResult{Truncated:true}`，不静默丢弃。
- 读缓冲从 32KiB 调整为 256KiB；单次持 VT 锁仍受 `maxLockBytes` 限制。
- 普通 View Resize 不再在终端生命周期锁内调用 Conn；Conn 必须能在 Close 时解除并发 Resize。初始 Start Resize 保持原锁顺序，避免 Start/Close 竞态产生 late resize。

## 测试

- `go test ./ui/terminal/... ./sys/pty/...`：通过。
- `go test -race ./ui/terminal/... ./sys/pty/...`：通过。
- macOS/Linux/Windows `CGO_ENABLED=0 GOOS=<os> go vet ./ui/terminal ./sys/pty`：通过。
- `go test ./...`：通过（本阶段复跑时全仓绿色）。
- 新增输入测试：事件在下一帧前发送、IME 提交顺序、重复 build 不重复发送、队列保留额度和大 payload 截断。
- 新增生命周期测试：阻塞 Conn.Resize 时 Close 不阻塞，Start/Close 和多次 Resize 竞态回归通过。

## 基准（Apple M4，GOMAXPROCS=1，GOGC=off，Task 4 后）

| 场景 | 实测 |
|---|---:|
| 纯 ASCII VT | 1.51–1.66ms / 94.2KiB 输入，约 57–62MB/s |
| 彩色 ANSI VT | 1.46–1.98ms / 127.0KiB 输入，约 64–87MB/s |
| PTY 风格回放 VT | 1.19–1.90ms / 90.1KiB 输入，约 47–76MB/s |
| Idle frame | 92–104µs，1,624B，27 allocs |
| 单脏行 frame | 154–164µs，约 9.8KiB，212 allocs |
| 滚动 frame | 159–161µs，约 10.0KiB，212 allocs |

这些 benchmark 测的是固定 90–127KiB 输入的新 VT 实例，不直接等同于 M1 的 50MB PTY 实机结果；B 阶段将统一三类输入的生命周期、吞吐单位和快照/缓存指标。

## Profile 结论

受控 `BenchmarkViewFrameIdle` profile（GOMAXPROCS=1、GOGC=off）中：

- `paint.CPU.Render` 约 22.5% 累计；
- `paint.CPU.fillRect` 约 12.8%；
- `image/draw.DrawMask` 约 9.1%；
- `ViewW.paintRow` 约 4.7%；
- 其余主要是通用 UI/CPU renderer 和 benchmark 分配成本。

因此 B 阶段必须减少未变化行进入通用绘制列表，不能只继续优化 `paintRow` 内部字符串/切片。
