# 交接：终端 Resize 与 Close 竞态（CI 间歇失败）

**状态**：未修复，待终端会话处理（2026-11-16，用户决定交给终端会话）。
**症状**：CI `test (ubuntu-latest)` 间歇失败，`-race -count=300` 本地约 10% 复现：

```
--- FAIL: TestTerminalStartRacingClose   conn_test.go: 1 Conn.Resize calls after Close
--- FAIL: TestTerminalResizeRacingClose  conn_test.go: 1 Conn.Resize calls after Close
```

复现命令：

```sh
go test -race -count=300 -run '^TestTerminalStartRacingClose$|^TestTerminalResizeRacingClose$' ./ui/terminal
```

## 根因

两条设计约束冲突：

1. `4c154a1`（fix(terminal): shutdown-aware reply writer; no Conn calls after Close）承诺 **Close 开始后绝不调用 `Conn.Resize`**：当时 `Conn.Resize` 在 `smu` 下调用，Close 先取 `smu` 标记 closing。
2. `76fec74`（perf(terminal): decouple resize from terminal lifecycle lock）为了让**阻塞的 `Conn.Resize` 不卡住 Close**，把 `Conn.Resize` 移出 `smu`（`ui/terminal/conn.go` 的 `resizeConn`）：先在 `smu` 下读 `closing`，释放后再调 `Conn.Resize`。检查与调用之间出现窗口，Close 可在其中关闭 Conn。`TestResizeDoesNotBlockClose`（`conn_perf_test.go`）守护约束 2。

迟到调用的栈均为 `Start` 中的首次 resize（`conn.go` 中 `t.resizeConn(cols, rows, 0, 0)`）。

## 已尝试（均未彻底解决，已撤回）

- 在 `smu` 下登记 `sync.WaitGroup` 后再调用，Close 关闭 Conn 后等待：窗口仍在（登记与调用之间）。
- `RWMutex`：resize 持读锁贯穿检查与调用，Close `TryLock` 成功则先标记再关，失败则先关再等锁：确定性复现测试（用钩子把 Close 卡进检查与调用之间）通过，`TestResizeDoesNotBlockClose` 通过，但上面两个并发测试仍约 10% 失败。
- 第三种变体（检查后再次复核 `isClosing`）结果相同。

## 需要终端会话决定

- **方案 A：改契约**为"Close **返回后**不再调用 `Conn.Resize`"，允许与 Close 并发的那一次调用；同步调整两个测试的判定口径（例如只统计 Close 返回之后的调用）。实现最简单，但放宽了 `4c154a1` 的承诺。
- **方案 B：保持严格契约**，同时满足阻塞 resize 不卡 Close。需要 Conn 层配合，例如让 `Conn.Close` 与 `Conn.Resize` 在 Conn 内部互斥并拒绝关闭后的调用，或给 `Conn` 增加可取消的 resize。

修复后请删除本文件。
