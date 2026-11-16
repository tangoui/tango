# 编辑器性能优化实测（2026-11-16）

本次保持编辑功能、语法高亮、小地图、主题和现有 UI，减少重复绘制时的临时分配。基线提交为 `a9ba605`，修改位于 `editor-performance` 分支。

## 修改

- 小地图复用 token 查询缓冲和当前帧的行保留集合。缓存行仍复制并独立持有 token，避免串色。
- 常见 120 列小地图使用 128 个字形单元的栈空间，超宽行仍可正常扩容。
- 文字排版复用 shaping 输入和颜色覆盖的边界缓冲。文字缓存复制输入，各行最终绘制颜色继续独立持有。
- 缓冲属于各个视图，只保留遇到的最大局部工作量，不为整个文件增加逐行缓存。

## 方法和限制

Apple M4，macOS arm64，GOMAXPROCS=10。现有 TypeTS20k / ScrollTS20k 基准使用 2 万行 TypeScript、完整高亮、1920×1080 逻辑尺寸、2× 离屏 Metal、完整重绘，操作计时包含等待 GPU 完成。

这些数据是基准操作耗时及分配量，不是物理键盘到屏幕的延迟，也不是显示器 FPS。GPU 等待受系统负载影响；临时分配减少不代表常驻内存 RSS 按相同比例下降，也没有测量文件打开速度。

原始配置每项运行 100 次、重复 3 组，并保存 CPU / 内存 profile。为减少前后负载差异，另用保存的两份测试二进制交替运行 5 轮，轮次间交换先后顺序；每份二进制每轮每项 100 次，汇总 5 轮的中位数。

5 轮交替测量中位数：

| 场景 / 指标 | 修改前 | 修改后 | 变化 |
| --- | ---: | ---: | ---: |
| 输入：临时内存 | 1,116,449 B/op | 783,379 B/op | -29.8% |
| 输入：分配次数 | 9,925 次/op | 9,386 次/op | -5.4% |
| 输入：整帧耗时 | 4.356 ms/op | 4.509 ms/op | +3.5% |
| 输入：CPU 段 | 2.201 ms/op | 2.279 ms/op | +3.5% |
| 输入：GPU 等待 | 2.143 ms/op | 2.130 ms/op | -0.6% |
| 滚动：临时内存 | 952,026 B/op | 475,260 B/op | -50.1% |
| 滚动：分配次数 | 5,114 次/op | 4,308 次/op | -15.8% |
| 滚动：整帧耗时 | 3.379 ms/op | 3.332 ms/op | -1.4% |
| 滚动：CPU 段 | 1.296 ms/op | 1.238 ms/op | -4.5% |
| 滚动：GPU 等待 | 2.067 ms/op | 2.097 ms/op | +1.5% |

临时分配明显下降。整帧耗时未测出稳定提升：输入中位数略增 3.5%，滚动略降 1.4%，单轮之间也有波动，因此不声称本轮降低输入延迟或提高显示帧率。输入 CPU 段略增、滚动 CPU 段略降；后续若要优化延迟，应另行测量真实交互并分析渲染与高亮热点。第一轮未交替测量曾显示较大耗时下降，但 GPU 等待与后台负载不同，故不将该下降作为收益。

这些数值是每操作累计分配的临时内存，不能等同于常驻 RSS，也不能推断进程内存减半。

## 微基准与回归

120 列小地图字形生成：12,272 → 4,144 B/op，9 → 2 allocs/op；3 组耗时中位数约 3.04 → 2.30 µs。预热小地图从每帧 505 次分配降到测试预算 25 次以内。预热普通行排版从 2 次分配降到 1 次，带颜色覆盖从 3 次降到 1 次，保留独立绘制颜色的分配。

验证通过：

- `go test -p 1 -short -timeout 20m ./...`：129 个有测试的包通过，40 个包无测试；这是仓库短模式回归，包含 gallery 短测试；未运行 gallery 耗时的完整主题文字对比扫描。编辑器另行运行下述完整回归。
- `go test ./ui/editor/... ./examples/editor-demo`：编辑器及子包完整回归通过，含既有像素、主题、lexer、Unicode、换行、导航与重绘检查。
- `go test -race ./ui/editor -run 'Test(Minimap|WarmMinimap|WarmStyleRuns|StyleRuns|OverrideBounds|ViewDamageVerified|Navigation|Sticky)' -count=1`：通过（158.344s）。
- `go vet ./ui/editor/... ./examples/editor-demo`、示例构建、gofmt 与 `git diff --check`：通过。
- `cmd/tan` 子模块 `go test ./...`：通过。

独立只读审查未发现 Critical / Important 问题。延后的可选测试加强：使用不同 token 流并显式覆盖小地图的查询缓冲，验证缓存 token 的所有权；当前实现的 slices.Clone 及既有主题、lexer、像素回归已核对。

## 复现

从本分支根目录运行：

```sh
go test ./ui/editor -run '^$' \
  -bench '^(BenchmarkTypeTS20k|BenchmarkScrollTS20k)$' \
  -benchtime=100x -count=3 -benchmem

go test ./ui/editor -run '^$' \
  -bench '^BenchmarkMinimapGlyphBitmap$' -benchmem -count=3
```

本机原始日志、profile、交替测量脚本和二进制保存在 `/Users/admin/min/editor-performance-results`。新版示例可从本分支运行 `go run ./examples/editor-demo`，或运行该目录的 `tango-editor`。已打开的旧进程不会自动替换。
