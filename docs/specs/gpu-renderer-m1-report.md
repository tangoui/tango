# GPU 渲染器里程碑 1 验收记录（macOS Metal）

状态：2026-11-12 已验收离屏部分和实机窗口部分（第 3 节）。Retina 与 1x 屏之间拖动、手动拖拽改变窗口大小这两项需要人工确认，见 3.5。
对应规格：[gpu-renderer.md](gpu-renderer.md) 6.6（正确性与验收）、6.8（边界决定、性能口径）。

## 0. 环境

- 机器：Apple M4（10 核，16 GB），macOS 26.6。只有一台显示器：外接 4K 的 LS32A70（3840×2160），系统缩放后相当于 1920×1080 @2x，恰好是规格 6.6 的参考窗口配置。没有 1x 显示器。
- 机器由多个会话共用，基准分两轮跑。第一轮负载平均值 **19–28**（10 核）。第二轮负载平均值 **5.6–7.4**：主控要求负载低于 6 时重跑，开跑前 1 分钟负载降到 5.6，但另一个会话的编译让负载回升，实际跑的时候在 7.1–7.4。两轮都不是空载，数据应看作上限参考。
- 每个基准跑 `-count=5`，表中给出 5 次的中位数和范围（最小–最大）。命令：`go test ./ui/gpu/metal/ -run '^$' -bench 'Gallery|Editor' -count=5`。两轮原始输出分别存在 `.superpowers/sdd/2026-11-12-gpu-m1/task-10-shots/bench-load19-28.txt` 和 `bench-load6-7.txt`。

## 1. 黄金图比对（离屏）

测试：`ui/gpu/metal/golden_darwin_test.go`

- `TestGoldenPagesGPU`：渲染 `ui` 黄金测试的页面（antd / element / dark 三个主题），比较 `paint.CPU` 与 Metal 的结果。Metal 图从离屏渲染目标读回。比较时用页面所在缩放创建的 `ui.Driver` 生成 `paint.List`，1x 和 2x 各比一次。页面里有文字，所以按文字阈值判定：每通道差 ≤ 4，超阈值像素 ≤ 0.5%。日志里同时记几何阈值（≤ 2）下的结果。测试失败时，CPU 图、Metal 图和差异图（红色表示超阈值，蓝色表示有差异但在阈值内）会写到 `t.TempDir()`；设置 `TANGO_GPU_DIFF_DIR` 后改写到该目录，测试结束后文件保留。路径打印在日志里。
- `TestGalleryPageIsGoldenPage`：`ui/golden_test.go` 里的 `goldenPage` 属于 package ui 的测试代码，其他包不能导入，所以这里用 `galleryPage` 重建了同一个页面。这个测试用 `paint.CPU` 在 1x 下渲染 `galleryPage`，再与 `ui/testdata/basic_*.png` 比较，容差和 `TestGolden` 一样（每通道 24，超阈值像素 ≤ 0.5%），用来确认比较对象确实是黄金页面。如果字体与录制黄金图时不同，测试会跳过，与 `TestGolden` 的处理方式一致。
- **文字引擎固定**（终审修复）：黄金图是用纯 Go 文字引擎录的，而 `text` 包默认启用系统引擎（CoreText），两者蒙版不同。`ui/gpu` 和 `ui/gpu/metal` 的 `TestMain` 调用 `gputest.Main`，在任何测试运行前把 `text.UseSystemEngine(false)` 设一次（这是进程级开关，`text` 没有别的办法为系统字体构造纯 Go 字形源；`ui.Driver` 也走同一个注册表）。`gputest.Shape` 和 `gputest.CheckListEngine` 会检查每个字形源的光栅化路径，与测试固定的引擎不符就失败；黄金比较在引擎不是 `go` 时直接失败而不是跳过。修复前 `TestGalleryPageIsGoldenPage` 在本机一直 SKIP，`TestGoldenPagesGPU` 比的是 CoreText 对 CoreText，与 `ui/testdata/basic_*.png` 脱钩。
- `TestGoldenPagesGPU` 在 1x 时还把 Metal 读回的图直接与 `ui/testdata/basic_*.png` 比较（`TestGolden` 的容差：每通道 24），把 GPU 结果和黄金图直接挂钩。
- 系统引擎另有一套对拍（`TestRendererMatchesCPUSystemText`、`TestMetalMatchesCPUSystemText`）：用 `gputest.SystemText` 在单个测试内切到系统引擎（不并行，结束时恢复），对文字场景（text、opacity 和新增的 textmix：9–28 px、粗体、斜体、等宽、分数原点）在 1/1.5/2x 下比较 GPU 与 `paint.CPU`，并检查每个引擎蒙版都带 1px 透明边。

结果（`go test ./ui/gpu/metal/ -run 'Golden|GalleryPageIs' -v`，终审修复后、纯 Go 引擎下重跑）：

| 页面 | 缩放 | 最大通道差 | 超过 4 的像素 | 超过 2 的像素 | 结论 |
|---|---|---|---|---|---|
| basic_antd | 1x | 2 | 0（0.000%） | 0（0.000%） | 通过 |
| basic_antd | 2x | 2 | 0（0.000%） | 0（0.000%） | 通过 |
| basic_element | 1x | 2 | 0（0.000%） | 0（0.000%） | 通过 |
| basic_element | 2x | 2 | 0（0.000%） | 0（0.000%） | 通过 |
| basic_dark | 1x | 2 | 0（0.000%） | 0（0.000%） | 通过 |
| basic_dark | 2x | 2 | 0（0.000%） | 0（0.000%） | 通过 |

- `galleryPage`（`paint.CPU`，1x）与 `ui/testdata/basic_{antd,element,dark}.png` 比较，最大通道差都是 0，两者逐像素一致。（初版报告也写了 0，但当时这个测试实际是 SKIP，见上文"文字引擎固定"；这里是修复后真正跑出来的数字。）
- Metal 1x 读回图与 `ui/testdata/basic_{antd,element,dark}.png` 直接比较：三个主题最大通道差都是 2，超过 24 的像素 0。
- 系统引擎（CoreText）对拍：text 最大差 1，opacity 2，textmix 2，1/1.5/2x 下超阈值像素都是 0（CPU 执行器与 Metal 结果相同）。
- 六组结果连更严的几何阈值（≤ 2）都满足。按规格 6.8，只有超阈值时才需要修改 `paint.CPU` 的公式，所以本里程碑**不改 CPU 渲染器，也不重录黄金图**。
- 其他比较（前序任务的测试，本次全部重跑并通过）：`TestMetalMatchesCPU`（9 个对拍场景 × 1/1.5/2x）、`TestMetalGallery`；新增的 `TestMetalEditorText` 检查全屏编辑器文本帧，1x 和 2x 的最大通道差都是 1，没有超阈值的像素。

失败路径已经验证过：临时把阈值改成 -1 后，`basic_dark@2` 按预期失败，三张图正确写出，路径也打在日志里。验证后已改回 4。

## 2. 性能（规格 6.6 / 6.8）

### 2.1 测量口径

基准在 `ui/gpu/metal/gallery_darwin_test.go` 和 `perf_darwin_test.go` 中。测量的都是热缓存稳态帧（规格 6.8），只有冷启动一行例外。`paint.List` 在计时前生成，计时不含 build 和布局。

- **gpu-ms**：命令缓冲区的 `GPUEndTime − GPUStartTime`。
- **submit-ms**（"CPU 端提交"）：从 `Renderer.Render` 开始到 Execute 提交 commit 为止的 CPU 时间，不等 GPU。
- **exec-ms**：submit 里 Execute 自己的部分（staging、编码、commit）。
- **frame-ms**（"整帧"）：从 `Render` 开始到命令缓冲区完成，包括 submit、排队等待和 GPU 执行。

基准：

- `BenchmarkMetalGalleryFrame`：Task 8 已有的基准，画 gallery 页面，画布 640×300 逻辑像素。
- `BenchmarkMetalGalleryWindow`：同一个 gallery 页面，画布换成参考窗口 1920×1080 逻辑像素（2x 时为 3840×2160 设备像素）。窗口变宽后，Alert 等自适应宽度的组件也会跟着变宽。
- `BenchmarkMetalEditorFrame`：全屏编辑器文本帧，1920×1080，60 行 × 200 列等宽 13px 文字，行高 18。每行按语法高亮拆成约 30 段不同颜色的文字，全帧共 1801 个 Text 操作，约 1.2 万个字形；另有行号栏和当前行高亮。
- `BenchmarkMetalEditorColdFrame`：同一个编辑器帧，每帧都新建 Renderer，因此字形图集是空的（patch 页本来就每帧从空开始打包，没有跨帧的 patch 缓存）。Metal 设备沿用，相当于字号或 DPI 变化后的首帧。

### 2.2 结果与目标对照

每格是"中位数（范围）"。**A** 是第一轮（负载 19–28），**B** 是第二轮（负载 7.1–7.4）。结论以负载较低的 B 轮为准。

| 场景 | 缩放 | gpu-ms A / B | submit-ms A / B | frame-ms A / B | 目标 | 结论（B 轮） |
|---|---|---|---|---|---|---|
| gallery 640×300 | 1x | 0.53（0.43–0.85） / 0.45（0.38–0.49） | 1.04（0.75–3.25） / 0.56（0.42–0.64） | — | GPU ≤ 2，提交 ≤ 2 | 达标 |
| gallery 640×300 | 2x | 0.69（0.61–0.91） / 0.79（0.78–1.07） | 1.51（1.18–2.95） / 1.36（1.32–2.31） | — | GPU ≤ 2，提交 ≤ 2 | 达标（最大值超标） |
| gallery 窗口 1920×1080 | 1x | 0.85（0.70–0.93） / 0.64（0.57–0.74） | 2.01（1.19–3.59） / 0.78（0.66–1.17） | 5.20 / 2.68（2.32–3.51） | GPU ≤ 2，提交 ≤ 2 | 达标 |
| **gallery 窗口 1920×1080（参考配置）** | **2x** | 1.38（1.17–1.51） / **1.34（1.22–1.73）** | 2.80（2.02–4.13） / **2.05（1.84–3.13）** | 5.77 / 5.37（5.02–7.15） | GPU ≤ 2，提交 ≤ 2 | GPU 达标；**提交卡在线上（超 0.05 ms，最大值 3.1）** |
| 编辑器全屏文本（热） | 1x | 0.72 / 0.50（0.40–0.65） | 2.22 / 1.23（1.00–1.75） | 6.44 / 4.06（3.01–5.14） | 整帧 ≤ 4 | submit + GPU = 1.7，达标；frame-ms 卡在线上 |
| **编辑器全屏文本（热，参考配置）** | **2x** | 1.21 / **0.89（0.63–1.20）** | 4.10 / **1.23（1.04–1.95）** | 10.0 / **4.37（3.73–6.65）** | 整帧 ≤ 4 | submit + GPU = 2.1，达标；**frame-ms 超标 0.4 ms** |
| 编辑器首帧（冷图集） | 1x | 1.44 / 1.75 | 12.4 / 6.75 | 18.5（13.4–47.3） / 10.4（8.5–11.9） | 首帧 ≤ 50 | 达标 |
| 编辑器首帧（冷图集） | 2x | 1.95 / 2.05 | 8.7 / 3.64 | 14.2（10.8–27.5） / 7.9（7.2–8.0） | 首帧 ≤ 50 | 达标 |
| 单行编辑局部重绘 | — | — | — | — | ≤ 1 | 不在本里程碑：Damage 和局部重绘属于里程碑 2 |
| 静止零帧 | — | — | — | — | 0 帧 | **达标**（实机测量，见 3.3） |
| 二进制增量 | — | — | — | — | ≤ 1 MB | **达标：+117 KiB**（见 3.4） |

exec-ms（Metal 编码和提交）在 B 轮所有热帧中都在 0.11–0.24 ms。

参考数据：Task 8 在负载较低时测过 gallery 640×300，gpu-ms 在 1x 下为 0.13–0.20、2x 下为 0.19–0.30，submit-ms 在 1x 下为 0.39–0.90、2x 下为 1.08–2.6。B 轮的 GPU 时间仍是 Task 8 的 2–3 倍，说明两轮都受共享负载影响。

### 2.3 差距与原因（本里程碑不做专门调优）

1. **gallery 参考窗口 2x 的提交时间（中位数 A 轮 2.8 ms，B 轮 2.05 ms；目标 2 ms）**。按 CPU profile，Renderer.Render 约 85% 的时间花在 `Renderer.patch → paint.CPU.RenderRegion → strokeRect → x/image/vector.Rasterizer`，也就是在 CPU 上光栅化描边、阴影这类 patch。窗口变宽后，Alert 的圆角描边宽约 1888 逻辑像素，2x 时 patch 宽约 3776 像素，光栅化成本跟着变大。Metal 编码只占 0.2–0.5 ms。补救办法是规格里已经排定的里程碑 2 工作：原生描边和阴影着色器，做完后不再需要 patch 光栅化。Task 8 测得的 640×300 2x 最高 2.6 ms 也是这个原因。
2. **编辑器热帧**。B 轮 submit 1.23 ms 加 GPU 0.89 ms 共 2.1 ms，在 4 ms 预算内。A 轮的 4.1 ms 主要是负载造成的：一帧约 1.2 万个字形，每个字形都要查两次表（`paint.CPU.GlyphMask` 的字形缓存和 `atlas.Lookup`），profile 显示主要开销是 `runtime.mapaccess2` 和哈希计算，这类工作在 CPU 被抢占时最先变慢。可以改进的方向留给编辑器性能原型和里程碑 2：按文字操作缓存实例、把两层查表合成一层、局部重绘。
3. **frame-ms 比 submit + GPU 多出约 2 ms**（B 轮：gallery 2x 多 2.0 ms，编辑器 2x 多 2.3 ms；A 轮多 2–5 ms）。多出的部分是 commit 之后排队等 GPU、以及 `waitUntilCompleted` 唤醒等待线程的延迟，和机器负载正相关。真实窗口里，CPU 提交下一帧和 GPU 执行上一帧可以重叠，所以这段延迟不会直接叠加到帧间隔上。按"submit + GPU"计，B 轮各项都在目标内；按 frame-ms 计，编辑器 2x 超出 0.4 ms。要定论需要在没有其他会话的机器上复测。
4. **冷首帧**：B 轮为 7.9–10.4 ms，离 50 ms 的上限还有很大余量。A 轮 1x 的最大值 47 ms 出现在负载峰值期间。A 轮对 2x 跑过一次 CPU profile：46 帧里 `Renderer.Render` 只有约 190 ms 的 CPU 采样，折合每帧约 4 ms，主要是字形光栅化（`GlyphMask`）、图集插入、新纹理和缓冲区清零以及 staging；同一次运行的 submit 墙钟时间约 19 ms，两者相差的部分是负载下进程等不到 CPU 的时间和 GC。规格 6.8 要求"字形光栅化按核数并行"，这项还没有实现，留作后续工作。

## 3. 实机验收（Step 3）

用到的程序是 `examples/ui-window`，它通过 `uihost.Open` 打开窗口，`-renderer cpu|gpu` 参数会设置 `TANGO_RENDERER`，并在日志里打印实际使用的渲染器。`examples/ui-gallery` 用的是无头 `ui.Driver`，只输出 PNG、不开窗口，所以不适用于实机验收。为了避免 `go run` 期间被其他会话的未完成改动打断编译，我把程序先编译成二进制再运行。窗口号用 JXA 调用 `CGWindowListCopyWindowInfo`，按进程 PID 查询；截图命令是 `screencapture -x -o -l<窗口号>`。截图、比较脚本和基准原始输出都存在 `.superpowers/sdd/2026-11-12-gpu-m1/task-10-shots/`。

### 3.1 GPU 与 CPU 窗口截图对照

- `-renderer gpu` 的日志为 `ui-window: renderer: gpu`，`-renderer cpu` 的日志为 `ui-window: renderer: cpu`。
- 两种渲染器下画面都正确：文字清晰，圆角、阴影、开关、滑块、Tabs 下划线都正常，没有错位（`gpu.png`、`gpu-resized2.png`、`cpu-resized.png`）。
- 逐像素比较两个窗口的截图，跳过 64 px 标题栏和 8 px 圆角边缘（标题栏会随窗口是否为当前窗口而变化）。截图是 2x 设备像素，带显示器的 ICC 配置文件（LS32A70）：

| 窗口大小（pt） | 比较方式 | 差 0 | 差 1 | 差 2 | 差 3 | 最大差 | 超过 4 的像素 | 平均有符号差（GPU − CPU） |
|---|---|---|---|---|---|---|---|---|
| 1300×900 | 截图原始值（显示器色彩空间） | 30.5% | 69.5% | 1181 | 13 | 3 | 0 | +0.69 / 通道 |
| 800×600 | 截图原始值（显示器色彩空间） | 43.1% | 56.9% | 968 | 8 | 3 | 0 | +0.56 / 通道 |
| 1300×900 | 两张图都用 `sips --matchTo` 转回 sRGB | **99.55%** | 0.43% | 879 | 7 | 3 | 0 | **0.00** |

### 3.2 GPU 与 CPU 之间 1–3/255 偏差的定位

min-a6 指出 CPU 路径的 CGImage 已经标记为 kCGColorSpaceSRGB（`internal/platform/darwin/canvas.go` 约第 592 行）。Metal 呈现器的 CAMetalLayer 也设置了 sRGB 色彩空间，使用 BGRA8Unorm 格式（`presenter_darwin.go` 第 45–59 行）。所以偏差不是因为缺少色彩空间标记。下面是逐项定位：

1. **两条路径的源像素完全相同**。在 2x 离屏渲染 gallery 页面，读取同一位置：背景处 `paint.CPU` 和 Metal ReadPixels 都是 (245,245,245)，正好是主题 BgLayout `#f5f5f5`；主按钮处两者都是 (22,119,255)，正好是主题 Primary `#1677ff`。
2. **ColorSync 给出的预期值**。生成一张标记为 sRGB 的色板 PNG，用 `sips --matchTo` 转换到截图里嵌入的显示器配置文件 LS32A70。得到：sRGB 245 → 243，(22,119,255) → (30,107,248)，255 → 255。
3. **在两张截图中取大块纯色区域**（9×9 区域，区域内各像素完全一致）：

| 区域 | 主题值（sRGB） | ColorSync 预期（显示器色彩空间） | CPU 截图 | GPU 截图 | 偏离预期的路径 |
|---|---|---|---|---|---|
| 窗口背景 BgLayout | 245,245,245 | 243,243,243 | 243,243,243 ✓ | **244,244,244** | GPU 每通道 +1 |
| 开关填充 Primary | 22,119,255 | 30,107,248 | 30,107,248 ✓ | 30,107,248 ✓ | 无 |
| 卡片和输入框 BgContainer | 255,255,255 | 255,255,255 | 255 ✓ | 255 ✓ | 无 |

4. **逐个灰阶的映射**。统计两张截图中所有灰色像素的值对应关系（CPU 值 → GPU 值）。绝大多数灰阶完全相同，零散的灰阶相差 ±1。例如 243→244（窗口背景，约 308 万像素）、226→227、27→28、158→157、211→209。这不是整体平移，而是两套色彩转换在少数灰阶上的取整不同。窗口背景恰好落在 GPU 偏 +1 的灰阶上，而且面积最大，所以看上去像整张图偏了 +0.6。
5. **截图本身的影响**。`screencapture` 记录的是显示器色彩空间下的帧缓冲值，并嵌入显示器配置文件（`sips -g profile` 为 LS32A70）。两张截图都转回 sRGB 后，99.55% 的像素完全相同，平均差 0.00，最大差 3，背景都还原为 245，Primary 都还原为 (23,119,255)（往返转换带来 1 的取整误差）。

**结论**：偏差出现在 **GPU 呈现路径**，即 CAMetalLayer 的内容由 CoreAnimation 或 WindowServer 转换到显示器色彩空间这一步。在部分灰阶上，它的取整结果与 ColorSync 相差 1，最多 3，例如背景 245 被转换成 244 而不是 243。CPU 路径（标记为 sRGB 的 CGImage）的结果与 ColorSync 完全一致。两条路径写入的源像素逐字节相同，偏差在每通道 ≤ 4 的比对阈值内，肉眼看不出来。按要求没有修改平台代码。如果以后要追求逐字节一致，可以尝试的方向（未验证）：改用 `MTLPixelFormatBGRA8Unorm_sRGB` 或 RGBA16Float 格式的 drawable，或者关闭 CAMetalLayer 的色彩匹配，改为由应用自己转换到显示器色彩空间。

### 3.3 改变窗口大小与静止零帧

- 用代码改变窗口大小：用 System Events 的 `set size of window 1 of (first process whose unix id is <pid>)` 依次设为 1000×792 → 1300×900 → 800×600 → 700×500 → 1400×950 → 900×640。这一步不需要辅助功能权限。每次布局都跟着窗口重新排版，截图尺寸和窗口一致（例如 900×640 pt 截出 1800×1280 px）。画面没有拉伸、模糊、黑边或错位（`gpu-resized2.png`、`gpu-small.png`、`gpu-900.png`）。
- 静止零帧（GPU 渲染器）：用 `ps -o time=` 统计进程 CPU 时间，空闲 10 秒内为 0:00.78 → 0:00.78 和 0:00.36 → 0:00.36，`ps -o %cpu` 每 2 秒采样都是 0.0。连续改变窗口大小之后再空闲 10 秒，CPU 时间为 0:00.45 → 0:00.45，也没有增加。空闲时进程不出帧，CPU 占用为 0。
- 第一次运行的两个窗口在约 4 分钟后同时退出，日志里没有 panic 或错误输出，退出原因不明。我推测是其他会话结束了同名进程，但无法确认。重新启动后，上面的改变大小和空闲测试都正常完成，最后由我用 SIGTERM 结束进程。

### 3.4 二进制增量

`examples/ui-window` 用 `-trimpath -ldflags "-s -w"` 构建两次进行比较。第二次用 `go build -overlay` 把 `uihost/gpu.go` 替换成不引用 `ui/gpu/metal` 的存根，这样不需要改动仓库里的任何文件。用 `go list -deps` 确认过，替换后依赖里没有 `ui/gpu*` 和 `ui/shader*`。purego 在平台层本来就在用，所以不计入增量。三次构建是连续进行的：GPU 版构建了两次，两次大小相同。

| 构建 | 大小（字节） |
|---|---|
| 含 GPU 路径 | 9,693,250 |
| 不含 GPU 路径（overlay 存根） | 9,573,602 |
| **增量** | **119,648（约 117 KiB）**，目标 ≤ 1 MB，达标 |

### 3.5 需要人工确认的项目

本机只有一台显示器（4K，按 2x 缩放），没有 1x 外接屏。用代码模拟拖动改变窗口大小、在屏幕之间移动窗口都不可靠，而且会影响共享机器上的其他会话，所以下面两项需要人工确认：

1. **Retina 与 1x 屏之间拖动**：接一台 1x 显示器（或在"系统设置 → 显示器"里给某台屏选择非 HiDPI 分辨率），运行 `go run ./examples/ui-window -renderer gpu`。把窗口从 2x 屏拖到 1x 屏，再拖回来。确认：两边文字都清晰，1x 屏上没有发糊；没有错位、黑边或错误的缩放；窗口跨在两块屏中间时画面也正常；终端里没有 `GPU lost` 或 `GPU unavailable` 日志。对应的自动化测试是 `ui/gpu/metal` 的 `TestMetalScaleChange`（终审修复新增）：同一个 Device 和 Renderer 依次按 2x → 1x → 2x 渲染同一场景，每帧都与该缩放下的 `paint.CPU` 比较，并确认渲染目标尺寸跟着变。`uihost` 的同名测试用的是假 GPU，只覆盖宿主侧的缩放传递，不覆盖真实设备。
2. **手动拖拽改变窗口大小**：拖动窗口右下角，快速放大和缩小，确认画面跟手，没有拉伸、闪烁或黑边。双击标题栏缩放、进入全屏再退出、最小化再恢复，都应该没有日志报错。
3. 第 1、2 项各用 `-renderer cpu` 再做一遍，对照两种渲染器下的表现。

## 4. 构建与测试

- 离屏部分完成时（约 21:40）：`go build ./... && go vet ./...`、`GOOS=windows/linux go build ./...`、`./ui/gpu/...` 的交叉编译和 vet、`go test ./ui/... ./uihost/...` 全部通过。
- Step 3 完成后（约 22:00）复查：build、vet 和交叉编译都通过；`./ui/gpu/metal`、`atlas`、`cpuexec`、`shaders` 的测试通过。当时 `./ui/gpu` 的 `TestHalfTexelMatters` 和 `TestRendererAtlasOverflow` 失败：另一个会话在 21:46–21:56 接入了系统文字引擎（`ui/text/engine*.go`、`ui/paint/glyph_raster.go`、`ui/paint/cpu.go`），两个测试隐含依赖纯 Go 引擎的蒙版（带透明边）和字形尺寸。之后引擎蒙版统一带 1px 透明边，终审修复又把两个测试改成与蒙版内容无关：`TestHalfTexelMatters` 把条目外的纹素全填 255，比较"带半纹素钳制"在干净/污染纹理上的输出必须逐像素相同，同时要求故意去掉钳制的变体能看出差别；`TestRendererAtlasOverflow` 按实测蒙版尺寸设页大小（每页恰好一个字形），直接断言每帧的淘汰次数和 ErrFull 后改画 patch 的数量。两个测试都在纯 Go 和系统引擎下各跑一遍，现在都通过。

## 5. 终审修复（2026-11-12）

- 文字引擎固定与系统引擎对拍：见第 1 节。
- 图集：`atlas.Atlas.Evictions()` 统计 LRU 淘汰的页数；`TestAtlasLRU` 验证两页时先用的页先被淘汰。`alloc` 的注释改为实际的最佳适配（最矮的放得下的货架）。
- 超大窗口：渲染目标或采样纹理任一边超过 Metal 的 16384 时，`Device.Execute` 在分配任何资源之前返回 `ErrTargetTooLarge`，不计入连续失败次数，也不会变成 `ErrDeviceLost`（修复前会直接触发 Metal 的纹理描述符断言，进程中止）。`TestMetalTargetTooLarge` 覆盖。`uihost` 遇到这个错误目前会请求下一帧，窗口一直超限时会反复重试，是否改为该帧走 CPU 由 `uihost` 决定。
- 着色器布局：见规格 6.4"主机可见布局"。Metal 设备的顶点描述符和 uniform 长度改为取自生成的 `ui/gpu/gen/quad_layout.go`。
