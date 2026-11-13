# GPU 渲染器里程碑 3 验收记录（Windows Direct3D 11）

状态：2026-11-13 完成了 HLSL 后端、D3D11 设备（整帧、图片、图层、局部帧）和 DXGI flip-model 呈现器。`go build ./...` 在 darwin、windows/amd64、windows/arm64、linux 上都通过；`go vet` 在 darwin 上对全模块（`./...`）运行、在 windows/amd64 和 windows/arm64 上对 `./ui/...` 运行，都通过（linux 只跑了 build，没有跑 vet）；与平台无关的部分（ABI 槽位与结构体尺寸、HLSL 生成、层池、实例环、故障状态机等）在 darwin 上有单元测试并通过。**这些代码都还没有在真实 Windows 上运行过**：本机没有 Windows，规格 6.7 第 3 条允许"实机验收待补"。所有需要真实 D3D11 的测试只做到编译通过（`go test -c`），实机清单见第 5 节。平台侧的 `CanvasHWND` 与 uihost 胶水由 min-ab 实现，契约草案已写进 [canvas-surface.md](canvas-surface.md) 的"GPU 呈现"一节。

对应规格：[gpu-renderer.md](gpu-renderer.md) 6.5（平台呈现）、6.6（正确性与验收）、6.7 第 3 条、第 7 节（字节码获得方式，本里程碑已决定）。计划：`docs/plans/2026-11-13-gpu-m3.md`。M2 记录：[gpu-renderer-m2-report.md](gpu-renderer-m2-report.md)。各任务报告与台账在 `.superpowers/sdd/2026-11-13-gpu-m3/`（`task-{1..7}-report.md`、`progress.md`）。

## 1. 已完成项

| 任务 | 内容 | 主要文件 |
|---|---|---|
| 1 | 设备错误上移到 gpu 包：`gpu.ErrDeviceLost`、`gpu.ErrTargetTooLarge`（文案 `gpu: …`），`metal.ErrX` 改为同值别名，所以 `uihost` 不改也能继续编译，`errors.Is` 照常成立。共享一致性套件 `gpuconf.Conformance(t, open, opts...)`：场景 × 缩放、文字（纯 Go 与系统引擎）、零尺寸、重复帧、批内上传顺序、图层池、图片缓存、色带行、局部帧全组，另有 `gpuconf.Skip` 选项。Metal 测试改为调用它；`frame.go` 补上套件检查的行为文档，语义不变 | `ui/gpu/errors.go`、`ui/gpu/internal/gpuconf/` |
| 2 | HLSL 后端 `ui/shader/hlsl`，入口 `vs_main`/`ps_main`，SM5（同时兼容 SM4）。前端新增 HLSL cbuffer 布局检查：HLSL 和 MSL 的布局都必须等于 Go 布局，出错时报告字段名和两边的偏移。uniform 中禁止 bool 和标量数组。tanshader 额外生成 `quad.hlsl` 与纹理表 `QuadTextures`；`gen.QuadHLSL` 用 embed 嵌入 | `ui/shader/hlsl/`、`ui/shader/front/layout.go`、`ui/gpu/gen/` |
| 3 | 无 cgo 的 COM 绑定：vtable 结构与槽位常量、13 个以上 D3D/DXGI 结构体、HRESULT。DLL 从系统目录按绝对路径懒加载。设备创建先用硬件、再退到 WARP，`TANGO_D3D_WARP=1` 强制 WARP。运行时用 `D3DCompile` 编译着色器，失败时错误带编译器日志 | `ui/gpu/d3d11/abi.go`、`com_windows.go`、`api_windows.go`、`compile_windows.go` |
| 4 | D3D11 设备的整帧部分：常驻 R8G8B8A8 目标、mask/color/ramp 纹理、实例环（NO_OVERWRITE/DISCARD）、`UpdateSubresource` 按命令顺序上传、`ReadPixels`、故障计数与设备丢失、`TANGO_D3D_DEBUG=1`（调试层与编译警告） | `device_windows.go`、`pipeline_windows.go`、`textures_windows.go`、`plan.go` |
| 5 | 图片（IMMUTABLE 纹理）、图层池（逐行移植 Metal 的 assignLayers，预算 128 MiB，空闲 120 帧释放）、局部帧（`gpu.Retainer`，经 scratch 的两次 `CopySubresourceRegion`，无混合的清除四边形）。`TestD3DConformance` 不带跳过，完整运行 | `layers.go`、`layers_windows.go`、`partial_windows.go` |
| 6 | 呈现器 `d3d11.NewPresenter(hwnd uintptr) (paint.Renderer, present func(image.Point, float32) error, error)`，形状与 `metal.NewPresenter` 相同。FLIP_DISCARD，2 个缓冲，BGRA，SCALING_NONE；`present` 开头按需 `ResizeBuffers`；全屏三角形把 RGBA 目标拷到 BGRA 后备缓冲；`SetMaximumFrameLatency(1)`；背景色跟随 `Frame.Clear`；Alt+Enter 关闭。SM 版本常量移到手写的 `gen/profiles.go`，d3d11 不依赖 `ui/shader/hlsl` | `presenter_windows.go`、`present.go`、`ui/gpu/gen/profiles.go` |
| 7 | 平台契约（canvas-surface.md 新增"GPU 呈现"一节）、规格第 7 节记为已决定、本报告 | 文档 |

规格第 7 节"D3D11 字节码获得方式"已按计划决定：运行时加载系统 `d3dcompiler_47.dll`，用 `D3DCompile` 编译（因为本机没有 Windows 生成环境）。DLL 缺失时错误包装 `d3d11.ErrNoDevice`，调用方回退 CPU。gpu-renderer.md 第 7 节已加注日期。

## 2. HLSL 离线检查

- **dxc 1.9（`-HV 2018`）：通过。** 官方 dxc 的 Linux 版只有 x86_64，而且需要 glibc ≥ 2.38，所以在 podman 中用 `--platform linux/amd64` 的 debian:trixie 容器模拟运行（dxc 1.9.2609.5，容器外加了一个包装脚本，仓库里没有任何 dxc 相关文件）。`TANGO_DXC=<包装脚本> go test -run TestDXCCompiles ./ui/shader/hlsl/` 编译 6 份源码：提交的 `quad.hlsl`、basic、scoping、names、exprs，以及手工构造的 select 模块。每份分别按 `-T vs_6_0 -E vs_main` 和 `-T ps_6_0 -E ps_main` 编译，全部退出码 0，没有错误或警告（模拟运行用时 278 s）。`-HV 2018` 是 fxc 实现的语言版本；HLSL 2021 去掉了向量的逐分量 `?:`，只有 select 模块用到它。quad、ok、exprs、names 不加 `-HV` 也能编译。没有 dxc 时这个测试会跳过。
- **fxc（`D3DCompile`，SM5/SM4）：从未运行。** 运行时实际使用的是 fxc，它第一次真正运行会在 Windows 实机的 `TestCompileQuad` 里（这个测试同时编译 vs_5_0/ps_5_0 和 vs_4_0/ps_4_0）。Task 3 逐行检查过 `quad.hlsl`，没有发现 fxc 会直接拒绝的写法：没有用 `pow`/`frac`；只有一个固定 4 次、不访问数组也不采样的循环；采样全部用 `SampleLevel(…, 0)`；没有拿保留字当标识符；插值寄存器 11 个、顶点输入 9 个。仍值得留意的有：X4000（变量可能未初始化）警告、没有 `[loop]` 属性（X3511 风险低）、OL3 下编译耗时，见第 6 节。
- 编译选项：只用 `D3DCOMPILE_OPTIMIZATION_LEVEL3`，**不加** `IEEE_STRICTNESS`（台账 Ruling，与 Metal 默认的 fast-math 对应）。

## 3. 与 Metal 设备的语义对照

以 `ui/gpu/metal/metal_darwin.go` 的 Device 设计注释为规范，D3D11 的设计注释（`ui/gpu/d3d11/device_windows.go`）按同样的段落写。"一致"表示可见行为相同，实现方式可以不同。

| # | 条目（Metal 设计注释） | Metal | D3D11 | 结论 |
|---|---|---|---|---|
| 1 | 渲染目标 | 常驻离屏 `RGBA8Unorm`，预乘 alpha，`Frame.Size` 变化时重建 | 常驻 `R8G8B8A8_UNORM`（RTV，另有供呈现读取的 SRV），预乘 alpha，尺寸变化时重建；重建会把 `hasFrame` 置 false | 一致 |
| 2 | 同步 | 每帧一个 command buffer，commit 后不等待；下一次 Execute、ReadPixels、Close 先等它完成（单缓冲） | 全部走立即上下文；每帧 `Flush` 一次，不等待。CPU 写入正在被读的资源时，由驱动做重命名或等待。ReadPixels 通过 Map staging 拷贝来等待 | 可见行为一致。差异：帧与帧之间没有显式等待；只用离屏设备时没有帧队列节流（台账 T4 minor），有呈现器时由帧延迟 1 节流 |
| 3 | 上传 | 一帧的上传像素先拷进共享 staging 缓冲，再由 blit 拷入纹理；批次带 Uploads 时结束当前 render pass、做 blit、再以 load 开新 pass | 每次上传是一次 `UpdateSubresource`，按命令顺序：`Frame.Uploads`，然后每个批次先上传再绘制。立即上下文保证上传排在之前的绘制之后 | 一致（"后面批次的上传不改变前面批次采样到的内容"） |
| 4 | 纹理重建 | 重建后内容未定义，Renderer 在 Mask/Color/RampSize 变化时重新上传 | 相同 | 一致 |
| 5 | 实例 | 实例缓冲在没有 command buffer 运行时写入；无上限 | 一个 DYNAMIC 顶点缓冲作环：放得下就 NO_OVERWRITE 接着写，否则 DISCARD 从头写；扩容到 `max(n, 2·cap, 64 KiB)`，上限 128 MiB。超出上限（约 93 万个实例，局部帧的清除四边形也计入）时返回包装 `gpu.ErrTargetTooLarge` 的错误，文本为 "instance count exceeds the device buffer limit"，以便和纹理超限区分；不计故障 | **差异**（Metal 不设上限） |
| 6 | 图片 | 每个 `ImageUpload` 新建一个 private 纹理（同 ID 替换），由第一次 blit 填充；`ImageFrees` 在上传前释放；没有图片的批次绑 1×1 占位纹理 | 用像素作初始数据建 IMMUTABLE 纹理（同 ID 替换）；行不完整时先打包成紧凑行；`ImageFrees` 先释放；占位纹理 1×1 | 一致。差异：占位纹理初始化为透明黑（Metal 未定义）；负 Stride 被拒绝（Metal 会 panic）；图片校验失败（ID、尺寸、长度）算无效帧、不计故障（Metal 经 `syncFailed` 计故障）；只有最后一行不完整时也整图打包（T5 minor） |
| 7 | 图层池 | assignLayers：取至少等于、至多两倍于所需尺寸的最小空闲纹理，最后一次使用后归还；预算 `layerPoolBytes`，空闲 `layerIdleFrames` 帧释放；更大的纹理只用左上角，并缩放复合实例的 UV | `layerPool[T]` 逐行移植同一套规则：128 MiB，120 帧，同一个缩放规则（`rescaleComposites`；复合实例先复制再缩放，不在写合并的映射内存上做读改写） | 一致 |
| 8 | 图层的危险跟踪 | Metal 的 hazard tracking 让复用纹理的新 pass 排在采样它的 pass 之后 | 切换渲染目标前，先把图片和图层槽（t3、t4）重置为占位纹理，保证池中纹理不会同时作为 SRV 和 RTV 绑定；执行顺序由立即上下文保证 | 一致（仅凭推理，需调试层实机确认） |
| 9 | 清屏 | 目标的第一个 pass 清成 `Frame.Clear`，图层的第一个 pass 清成透明，之后的 pass 都 load | 主目标在帧开头 `ClearRenderTargetView`；图层在第一次绑定时清成透明 | 可见结果一致 |
| 10 | 局部帧 | `gpu.Retainer`；目标不是上一帧（被重建或之后有失败）时返回 `ErrNotRetained`；Copies 经 scratch 中转；无混合的清除四边形；每个画进目标的批次对每个脏矩形各画一次 | 相同：`CopySubresourceRegion` ×2 经 scratch；清除四边形用 `noBlend`；剪刀 = 批次剪刀 ∩ 脏矩形；scratch 的尺寸决策照搬 `ensureScratch`（`scratchPlan`） | 一致。scratch 上限随特性级别 |
| 11 | 故障 | command buffer 的错误要到等待时才知道（pendingErr），下一次 Execute 返回它；连续 `maxFailures`=3 次失败，或一个表示 GPU 已不可用的错误，判丢失 | 错误同步得到：建纹理或缓冲、Map 的 HRESULT，以及 Flush 后的 `GetDeviceRemovedReason`。所有可能失败的步骤都在发出第一条命令之前完成，失败的帧不会改动目标。连续 3 次失败，或 `DEVICE_REMOVED/RESET/HUNG/DRIVER_INTERNAL_ERROR`，或任何 removed reason（其他调用失败后也会查询）判丢失 | 一致。差异：没有 pendingErr；`DRIVER_INTERNAL_ERROR` 也算丢失（微软文档要求此时重建设备） |
| 12 | 尺寸超限 | 纹理超过 `maxTextureSize`（16384）时预先拒绝，返回 `ErrTargetTooLarge`，不计故障 | 特性级别 11_0 起 16384，10_x 为 8192；同样预先拒绝，返回 `gpu.ErrTargetTooLarge`，不计故障。无效帧（未知纹理、越界上传、有 Copies 无 Dirty）返回普通错误，也不计故障 | 一致。上限随特性级别 |
| 13 | ReadPixels | 只看 `hasFrame`，丢失后仍返回旧像素；在等待中计故障 | 第一帧之前、或重建目标的失败帧之后报错；**丢失后返回 `ErrDeviceLost`**；Map 返回丢失码时判丢失，其他失败只报错、不计故障 | **差异**（T4 carry 6） |
| 14 | 零尺寸帧 | 直接返回 nil，不提交 | 不发出任何命令，直接返回 nil | 一致 |
| 15 | 着色器 | MSL 源码，运行时 `newLibraryWithSource` | HLSL 源码，运行时 `D3DCompile`（OL3）；11_0 起 vs_5_0/ps_5_0，10_x 用 vs_4_0/ps_4_0；10_x 硬件上建管线失败时改用 WARP；缺少编译器 DLL 时返回 `ErrNoDevice`，不重试 | 机制对应 |
| 16 | 设备选择 | 系统默认 Metal 设备 | 先硬件，再 WARP（`TANGO_D3D_WARP=1` 只用 WARP）；`TANGO_D3D_DEBUG=1` 时启用调试层（没装就去掉调试层重试，并记一次日志） | D3D 特有 |
| 17 | 串行化 | 方法由互斥锁串行；一个 Device 服务一个 Renderer | 相同（立即上下文本身不是线程安全的） | 一致 |
| 18 | 关闭 | `ErrClosed` | `d3d11.ErrClosed`；Close 幂等，先 ClearState/Flush 再释放 | 一致 |
| 19 | 失败帧与保留（`frame.go`：目标被重建，或某帧以可能改动目标的方式失败，则不再保留） | command buffer 失败（`gpuFailed`）清掉 `hasFrame`，因为失败的帧可能已经部分画进目标；编码前的失败（`syncFailed`）不清 | 所有可能失败的步骤都在第一条命令之前完成，计数的失败帧不会改动目标，所以 `hasFrame` 保持不变，`Retained` 仍为 true（重建目标或设备丢失时除外） | **差异**：同一种失败，Metal 下一帧整帧重画，D3D11 仍可局部重画；两者都符合 `frame.go` 的表述 |

呈现器（对照 `presenter_darwin.go`）：

| # | 条目 | Metal | D3D11 | 结论 |
|---|---|---|---|---|
| P1 | 目标 | `CAMetalLayer*`，设置 drawableSize | HWND 上的 DXGI 交换链，`present` 开头尺寸变化时调用 `ResizeBuffers` | 机制对应 |
| P2 | 错误报告 | 延迟到下一次 wait 才报告 | `Present1`/`ResizeBuffers` 同步返回 | 差异（更早得知） |
| P3 | 背压 | `nextDrawable` 阻塞 + maxInflight | `SetMaximumFrameLatency(1)` + vsync（`Present1(1, 0, …)`） | 等价 |
| P4 | 拿不到 drawable/被遮挡 | NoDrawable 的情况 | `DXGI_STATUS_OCCLUDED` 按成功处理，下一帧不带提示 | 对应 |
| P5 | 故障计数 | present 的 command buffer 与帧共用计数，等待成功后清零 | present 有**独立的**连续失败计数，只有成功的 present 才清零，成功的 Execute 不清零；连续 3 次判丢失（否则“Render 成功 → present 失败”的循环永远到不了上限） | **差异**（Final fix wave） |
| P6 | 脏矩形提示 | 无 | 只在 FLIP_SEQUENTIAL 上传；Win10+ 的常规路径 FLIP_DISCARD 不传（Ruling） | D3D 特有，常规路径与 Metal 相同（整幅） |
| P7 | 拷贝 | RGBA 目标 → BGRA drawable | 全屏三角形 `Load` 到 BGRA 后备缓冲；后备缓冲更大时重复最后一行和一列，更小时裁掉 | 一致 |
| P8 | 调整大小时露出的区域 | — | 交换链背景色 = 上一成功帧的 `Frame.Clear`（按不透明处理） | D3D 特有 |
| P9 | 尺寸 0 | 直接返回 | 返回 nil，不调用 ResizeBuffers，不提交 | 一致 |
| P10 | 丢失与关闭 | 返回 `ErrDeviceLost` / `ErrClosed` | 相同；关闭后 Render 经 Execute 返回 `ErrClosed` | 一致 |

## 4. 二进制体积（规格 6.6：增量 ≤ 1 MB）

方法：在 darwin 上用 `GOOS=windows GOARCH=<arch> go build -trimpath -ldflags='-s -w'` 构建 `examples/ui-window` 三次。uihost 目前只接了 Metal：在 Windows 上 `metal.NewPresenter` 是返回 `ErrUnsupported` 的桩，GPU 渲染器整个被链接器裁掉，d3d11 根本不会被链接。所以：
- **d3d11 版**用 `-overlay` 把 `uihost/gpu.go` 换成只改一行的副本（`newPresenter = d3d11.NewPresenter`，签名相同），用来近似 min-ab 将来的胶水；
- **无 GPU 版**沿用 M2 的存根（`.superpowers/sdd/2026-11-13-gpu-m2/task-10-shots/gpu_stub_m2.go.txt`）。

用 `go list -deps` 确认：d3d11 版的依赖里有 `ui/gpu`、`ui/gpu/atlas`、`ui/gpu/gen`、`ui/gpu/shaders`、`ui/gpu/d3d11`、`ui/gpu/metal`（桩）、`ui/shader/sl`；无 GPU 版的依赖里一个 `ui/gpu*`/`ui/shader*` 都没有。临时文件都在 `/tmp/t7-size/`，仓库里没有新增文件。

| 构建（windows） | amd64（字节） | arm64（字节） |
|---|---|---|
| 无 GPU（存根） | 11,721,728 | 10,526,208 |
| 当前（只有 Metal 桩，GPU 被裁掉） | 11,738,112 | 10,539,520 |
| d3d11 胶水 | 12,110,336 | 10,849,280 |
| **GPU 增量（d3d11 − 无 GPU）** | **388,608（379.5 KiB）** | **323,072（315.5 KiB）** |

结论：**达标**（≤ 1 MB）。作为对照，M2 在 macOS 上的增量是 305,024 字节。

交叉核对：一个最小 main 调用 `d3d11.NewPresenter` 并画一帧，另一个最小 main 只用 `gpu.NewRenderer` 加一个空设备，两者都按同样的参数构建：amd64 差 189,952 字节（185.5 KiB），arm64 差 173,568 字节。这就是 d3d11 包本身（含 gen 与 COM 绑定）在 `ui/gpu` 之上的增量。

辅助数据：对未剥离符号的 d3d11 版（amd64）运行 `go tool nm -size`，各包符号大小合计：

| 包 | 符号字节数 | 符号数 |
|---|---|---|
| `ui/gpu` | 100,184 | 75 |
| `ui/gpu/d3d11` | 75,617 | 136（其中 text 74,112） |
| `ui/gpu/atlas` | 9,792 | 12 |
| `ui/gpu/gen` | 3,056 | 3（`QuadInstanceAttrs` 3,008、`QuadTextures` 32、`QuadHLSL` 16 字节的字符串头） |
| `ui/gpu/metal`（桩） | 208 | 4 |
| `ui/gpu/shaders`、`ui/shader/sl` | 0 | 0 |

`ui/gpu/shaders` 和 `ui/shader/sl` 虽然在依赖里，但只提供类型和常量，没有符号被链接进来（`nm` 中没有以这两个包路径开头的符号），贡献可以忽略。`gen.QuadHLSL` 的正文是 17,038 字节的字符串（`quad.hlsl`），计在匿名的 `go:string.*` 数据里，不在上表中；`quad.metal` 不会被链接进 Windows 二进制。剩下的增量来自 pclntab、类型元数据和字符串。

## 5. Windows 实机待补清单

前提：Windows 10/11 x64（另找一台 arm64 设备跑同一套），安装 Go 1.27，在仓库根目录用 PowerShell 执行。硬件 GPU 和 WARP 各跑一遍。

| # | 命令 | 期望结果 |
|---|---|---|
| 1 | `go test ./ui/gpu/d3d11/ -v` | 全部 PASS，没有 SKIP。重点：`TestCompileQuad`（fxc 第一次运行，SM5 和 SM4 都要编译出以 `DXBC` 开头的字节码）、`TestD3DConformance`（各组子测试都 PASS：几何 ≤ 2、文字/图片缩放 ≤ 4、超阈值像素 ≤ 0.5%；包括 images/ImageCache 的阈值 2、opacity 系列、LayerUploadOrder、LayerPoolReuse、Partial/*，以及 strict verify 的 VerifyFractionalScroll）、`TestD3DGallery`、全部 `TestPresenter*`（Basic、Copy 逐字节相等、Resize、ZeroSize、DeviceRemoved、FailuresCounted、FailsBetweenFrames（Render 成功与 present 失败交替，第 3 次判丢失）、DirtyHint/discard 与 /sequential、OccludedNoHint、Close、BadHWND）。例外：`TestPresenterDirtyHint/sequential` 用的是隐藏窗口，上一帧可能被判为遮挡，这时只打一行日志 `the full frame was occluded (hidden window): no hint expected`，不做断言；如果 Renderer 把带提示的帧整帧画了，还会以 `the Renderer drew the hinted frame in full` SKIP。这两种情况都属正常，测试者要看 `-v` 日志记下是哪一种。FLIP_SEQUENTIAL 下 DirtyRects 是否真的被接受，由第 5 项的调试层输出确认 |
| 2 | `$env:TANGO_D3D_WARP=1; go test ./ui/gpu/d3d11/ -v; Remove-Item Env:TANGO_D3D_WARP` | 同上，全部 PASS；`TestWARPEnv` 确认适配器为 WARP（VendorId 0x1414，DeviceId 0x8c） |
| 3 | `go test ./ui/gpu/d3d11/ -run TestNewDeviceFallsBackToWARP -v` | PASS：模拟硬件失败后，设备落在 WARP 上 |
| 4 | `go test ./ui/gpu/d3d11/ -bench . -run '^$'` | `BenchmarkD3DGalleryFrame/scale1`、`/scale2` 输出 submit-ms 和 exec-ms。参考 Metal（640×300 gallery，submit 约 0.07 ms）：同一数量级即可，目标是 ≤ 2 ms。记录每帧 `Flush` 的开销和 NewDevice（fxc 编译）的耗时 |
| 5 | 先安装"图形工具"可选功能，再执行 `$env:TANGO_D3D_DEBUG=1; go test ./ui/gpu/d3d11/ -v -count=1` | PASS，调试层输出里没有任何 ERROR/WARNING（重点：NO_OVERWRITE/DISCARD 时序、"still bound on input" 一类 SRV/RTV 冲突、CopySubresourceRegion、ResizeBuffers 的 outstanding references、FLIP_SEQUENTIAL 下的 DirtyRects）；X4000 之类的编译警告每个入口点只打印一次。没装图形工具时，`TestNewDeviceDebugEnv` 也应 PASS（回退为普通设备，并记一次日志） |
| 6 | 用同样的命令在 arm64 Windows 上跑第 1、2 项 | 全部 PASS（检验 syscall ABI 和结构体布局） |
| 7 | 缺少编译器（建议在可回滚快照的虚拟机里做；System32 里的文件归 TrustedInstaller 所有，要先取得所有权再改名）。在管理员 PowerShell 中：<br>`$f = "$env:SystemRoot\System32\d3dcompiler_47.dll"`<br>`takeown /f $f`<br>`icacls $f /grant "${env:USERNAME}:F"`<br>`Rename-Item $f d3dcompiler_47.dll.bak`<br>`go test ./ui/gpu/d3d11/ -run 'TestD3DZeroSize\|TestPresenterBasic' -v`（阅读源文件时，竖线前的反斜杠只是 Markdown 表格的转义，实际输入时不要带）<br>（接入胶水后再加跑第 9 项的 `-renderer gpu`）<br>恢复：<br>`Rename-Item "$f.bak" d3dcompiler_47.dll`<br>`icacls $f /setowner "NT SERVICE\TrustedInstaller"`<br>`icacls $f /remove "$env:USERNAME"` | 两个测试都是 SKIP，不是 FAIL，也不 panic。SKIP 信息形如 `no Direct3D 11 device: d3d11: no Direct3D 11 device: … d3dcompiler_47.dll …`（`NewDevice` 编译着色器时 `D3DCompile` 找不到，错误包装 `ErrNoDevice`）。第 9 项的窗口打印 `tango: GPU unavailable, using CPU: …`，并在 CPU 上正常显示。恢复后，第 1 项重新全部 PASS |
| 8 | 真实的设备移除：例如 `dxcap -forcetdr`，或在设备管理器里禁用显卡驱动，期间运行第 9 项 | present 返回 `ErrDeviceLost`，窗口切到 CPU（兄弟子窗口显示），日志 `tango: uihost: surface N: GPU lost, using CPU`，不崩溃 |
| 9 | **min-ab 接入 `CanvasHWND` 和 uihost 胶水之后**，按下面的步骤手工检查（`examples/ui-window` 页面顶部是主题 Segmented 和三张统计卡片，下面有「表单」「表格」「其他」三个标签页，整页包在 Scroll 里）：<br>a. 在两个终端分别运行 `go run ./examples/ui-window -renderer gpu` 和 `go run ./examples/ui-window -renderer cpu`，两个窗口并排。三个标签页各用 Win+Shift+S 截一张图（「表单」页不勾选协议，让「提交」按钮保持禁用态）。<br>b. 拖动 GPU 窗口的右下角，在约 800×600 与 1400×950 之间来回拖 10 秒；双击标题栏最大化再还原；Win+↓ 最小化后从任务栏恢复；按 Alt+Enter。<br>c. 鼠标移出窗口、不碰键盘，等 10 秒。在这 10 秒前后各执行一次 `(Get-Process ui-window).TotalProcessorTime`。<br>d. 在「表单」页的「姓名」输入框里连续打字（可以用拼音输入法），在「简介」TextArea 里打几行；拖动「等级」滑块；点开「城市」Select 并输入筛选。把窗口高度缩到约 500 像素，用滚轮上下滚动整页。切到「表格」页，点「年龄」列头排序，勾选几行。<br>e. 把窗口拖到 DPI 不同的另一块显示器；或在"设置 → 显示 → 缩放"里把 100% 改成 150% 再改回来。<br>f. 关闭顺序（T6 清单第 8 项），两种都要试：<br>　(1) 正常关闭：点标题栏 ×，另做一次 Alt+F4，走胶水的常规关闭顺序；<br>　(2) 渲染器先关：先按第 8 项让 GPU 回退（回退时会释放交换链和设备，窗口仍然存在），再点 × 关窗口 | 日志 `ui-window: renderer: gpu`（或 `cpu`）。<br>a：每对截图一致。逐项比对：输入框和卡片的边框与圆角；Segmented、Tabs、Switch、Checkbox；「表格」页每行的 Progress 进度条；「其他」页的 Alert、Rate 星形、四色 Tag；禁用态「提交」按钮的半透明效果；文字。<br>b：没有拉伸，内容左上对齐。露出的区域是页面背景色（呈现器把交换链背景色设为 `Frame.Clear`，已从 sRGB 转成 DXGI 要求的线性值。颜色和页面背景对不上时，记下两者的值）。没有旧画面或闪黑；最小化和恢复不报错；Alt+Enter 不进入全屏。<br>c：两次的值相同，即静止时不出帧。<br>d：光标、候选窗、选中区、下拉层、滑块、滚动后的内容都没有残影或漏画；开发构建里没有 `gpu: verify:` 日志。<br>e：缩放正确切换，文字清晰。<br>f：两种顺序都不崩溃，进程正常退出，没有调试层报错 |
| 10 | 在第 9 项基础上设 `$env:TANGO_RENDERER="cpu"` 运行 | 走 CPU，不创建交换链 |
| 11 | 输入延迟与 CPU 占用（T6 清单第 6 项；帧延迟 1 加 vsync）：在第 9 项的两个窗口里分别做以下操作，并用任务管理器看 CPU 占用：<br>· 在「表单」页来回拖「等级」滑块；<br>· 窗口缩矮后拖动整页滚动条；<br>· 在「姓名」输入框快速连续打字；<br>· 在「其他」页连续点击「通知」按钮约 10 秒，让通知不断弹出和收起（ui-window 里没有持续转动的加载动画，用它代替）。<br>可选：用 PresentMon 2.x 量化，`PresentMon.exe --process_name ui-window.exe --output_file pm.csv`（参数以所装版本为准） | GPU 窗口的跟手程度不比 CPU 窗口差，拖动时内容紧跟鼠标。动画期间 GPU 窗口的 CPU 占用不高于 CPU 窗口。PresentMon 中帧间隔约等于一个刷新周期，排队不超过 1 帧，显示延迟约 1–2 个刷新周期 |
| 12 | 10_x 特性级别的设备（旧显卡，或用调试工具强制 10_0） | quad.hlsl 能用 vs_4_0/ps_4_0 编译；纹理上限为 8192；建管线失败时自动改用 WARP |
| 13 | （min-ab 平台侧接入后，2026-11-13 补）CPU 回退兄弟窗口 `TangoCanvasCPU` 盖住交换链 HWND 期间：触发一帧 `ErrTargetTooLarge`（把窗口拉到超过 16384 设备像素，或用测试钩子），并在 `TANGO_D3D_DEBUG=1` 下观察 | 兄弟窗口期间 `Present1`、`ResizeBuffers` 不报错；`DXGI_STATUS_OCCLUDED` / `DXGI_STATUS_CLIPPED` 都是成功码（`hrError` 对非负 HRESULT 返回 nil），按成功处理、不计失败 |
| 14 | 同上：DWM 能否让兄弟 GDI 窗口真正盖在 flip-model 交换链之上 | 超限那一帧显示 CPU 画面（兄弟窗口）而不是上一帧 GPU 画面；回到正常尺寸后兄弟窗口隐藏、GPU 画面恢复。若盖不住：记录现象，交给 min-ab 改为其他回退方式（例如回退帧期间临时用 GDI 画到独立顶层分层窗口） |

## 6. 已知风险

1. **整套实现都没有在 Windows 上运行过。** vtable 槽位、结构体偏移和 HRESULT 都对照过 Windows SDK 头文件（mingw-w64 副本）和 learn.microsoft.com，并有 darwin 单测（`TestABISlots`、`TestABIStructSizes`），但调用约定和 arm64 的 syscall 只做过推理。
2. **fxc 从未运行。** dxc 通过不等于 fxc 通过；SM4（vs_4_0/ps_4_0）下的编译也没有验证。
3. **精度**：没有 `IEEE_STRICTNESS`，fxc 可能合并成 mad 或重排浮点运算。依赖 `floor` 边界的函数（`strokeDash`、`Dither4x4`、`RampU`、层复合的 scale8）可能在个别像素上多差 1。阈值 2 大概率能吸收；如果实机一致性超阈值，加上这个标志（Ruling 中已写明后备方案）。
4. **首帧延迟**：`ps_main` 把所有辅助函数内联，OL3 下 fxc 可能要几百毫秒到 1–2 秒，这会推迟 `NewDevice` 和窗口的第一帧 GPU 画面。需要实机记录；如果太慢，可以考虑降优化级别、后台编译，或做字节码缓存。
5. **图片缓存阈值 2**：判断依据是 point 采样器在纹理中心取样是精确的。如果实机超阈值，可以在计划约束内放宽到 4（T1 minor）。
6. **SRV/RTV 冲突只靠推理排除**（T5），需要第 5 节第 5 项的调试层确认。
7. **`d3dcompiler_47.dll` 的可得性**：Windows 8.1 起系统自带，但精简系统（如 Server Core、定制镜像）是否都有没有核实。缺失时回退 CPU，不会崩溃。DLL 的加载加固（T3 minor）：`d3dcompiler_47.dll` 本身按 `GetSystemDirectoryW` 的绝对路径加载，但它依赖的 DLL 走默认搜索顺序，没有用 `LOAD_LIBRARY_SEARCH_SYSTEM32`；`GetSystemDirectoryW` 失败时退回写死的 `C:\Windows\System32`。
8. **只用离屏设备时没有帧队列节流**（T4 minor）：只用 Device、不用呈现器时（例如测试、基准），帧可能在驱动里排队；呈现器有帧延迟 1，不受影响。基准每 16 帧读回一次来规避。
9. **平台侧尚未实现**：Windows 窗口能否走 GPU，取决于 min-ab 实现 `CanvasHWND`（兄弟子窗口回退、WM_PAINT 不画 CPU 帧、WM_SIZE 后同步出帧）和按 GOOS 拆分的胶水。在此之前，Windows 窗口一直走 CPU。
10. **交换链回退链实际上走不到**：Go 1.27 要求 Windows 10 及以上，FLIP_DISCARD + SCALING_NONE 应该总能创建成功；FLIP_SEQUENTIAL 和 STRETCH 两级回退只为特殊驱动和测试保留（`swapChainModes`）。
11. 生成器的小遗留（T2 minor）：
    - 每个生成的 `_layout.go` 都声明一个 `TextureDesc`，同一个包里生成两个着色器会冲突（目前只有一个）；
    - `Generate` 假定被调用的函数排在调用者前面（`ir.Module.Funcs` 按被调用者在前排序，现已写进 `hlsl.go` 的包文档，与 `msl.go` 一致）；
    - `pow` 负底数（X3571）和 `frac` 的边界问题，目前着色器里都没有用到。
12. 测试的小遗留（T1 minor）：`Partial` 组为了判断 Retainer 会多打开一台设备。（`TestMetalConformanceSkip` 在没有 Metal 设备时已改为 SKIP。）
13. 体积测量用的是近似胶水（只把 `newPresenter` 换成 d3d11 的一行改动）。正式胶水只多一个类型断言，以及按 GOOS 拆分后少链接 metal 桩，误差在几 KB 量级。

## 7. 偏差与裁定

台账（`.superpowers/sdd/2026-11-13-gpu-m3/progress.md`）中的 Ruling：

1. 流程与 M2 相同：用影子仓库快照，不提交 git，与 editor-core-m2 计划并行（各自串行）。
2. M3 Task 1 不修改已有的 `ui/gpu/*_test.go`（只新增文件，以及修改 metal 测试）；editor-core-m2 T9（把测试的 cpuDevice 移进 cpuexec）等 M3 T1 完成后再做，避免并发编辑 `ui/gpu` 的测试。
3. 一致性套件放在 `ui/gpu/internal/gpuconf`，不放在计划写的 `gputest`：`gputest` 被 `ui/gpu` 的包内测试导入，放进去会形成导入环。计划中 T4/T5 的写法已改为 `gpuconf.Conformance`。
4. `D3DCompile` 只用 `OPTIMIZATION_LEVEL3`，不加 `IEEE_STRICTNESS`，与 Metal 默认的 fast-math 对应。如果实机一致性出现与 floor 相关的超阈值，再加上这个标志。
5. FLIP_DISCARD（Win10+ 的常规路径）不传脏矩形提示；不为了拿到提示改用 FLIP_SEQUENTIAL。DISCARD 是推荐的 flip 模型，提示只是可选项，代价是合成器按整窗更新。

与 min-ab（原 min-a6）商定的事项（台账 carry → T6/T7）：
- `ResizeBuffers` 在 present 开头调用（主线程，WM_SIZE 之后同步出一帧）；
- 分层或附着的 canvas 走 CPU；
- CPU 回退用交换链 HWND 之上的兄弟子 HWND（`Present(img)` 时显示，`Present(nil)` 时隐藏），交换链 HWND 上永远不画 GDI。

已写进 canvas-surface.md。

各任务对计划的偏离（均已经过审阅）：
- T1：错误文案带 `gpu:` 前缀；套件覆盖面比 brief 列出的大（ScaleChange、ManyUploads、RecreatedTargetHasNoFrame、LayerUploadOrder、局部帧全组）；新增 `gpuconf.Skip` 选项；`frame.go` 只补了文档注释。
- T2：
  - brief 给的跨界结构体示例在 MSL 和 HLSL 中偏移相同，所以另加了一个只有 HLSL 会拒绝的用例（`[2]float32`）；
  - uniform 中的标量数组、未补齐的嵌套结构体和 bool 现在都被拒绝（规则更严，有 3 个测试着色器为此调整）；
  - `TextureDesc` 声明在生成的 layout 文件里；
  - dxc 检查加了 `-HV 2018`。
- T3：设备测试单独放在 `api_windows_test.go`，并多加了几个测试；`DXGI_ERROR_DRIVER_INTERNAL_ERROR` 也算设备丢失；`device_other.go` 提前提供 `NewPresenter` 桩。
- T4：纹理上限和着色器 profile 随特性级别（10_x：8192、SM4），brief 只写了 16384/SM5；上传校验额外拒绝负 Stride。
- T5：图片校验失败算无效帧、不计故障（与 Metal 不同）；Task 4 评审的 5 条遗留全部处理（失败后查询 removed reason、实例上限检查前移并包装 `ErrTargetTooLarge`、10_x 回退 WARP、着色器头注释、ReadPixels 差异写进报告）。
- T6：
  - 帧延迟用 `SetMaximumFrameLatency(1)`，没有用 waitable swap chain；
  - SM 版本常量放在手写的 `ui/gpu/gen/profiles.go`，由 `TestProfilesMatchGenerator` 与 hlsl 包保持一致，所以 d3d11 不依赖 `ui/shader/hlsl`；
  - 新增 `SetBackgroundColor`（背景色跟随 `Frame.Clear`）；
  - 遮挡时把 `presentedSeq` 清零。
- T7：
  - 体积按第 4 节的方法测量（ui-window 的 d3d11 胶水版用 overlay 近似，另做了最小 main 交叉核对），没有按 brief 的字面直接构建 `examples/ui-window`，因为当前的 ui-window 在 Windows 上不链接 d3d11；
  - 契约交接对象是 min-ab（原 min-a6）。

延后处理的 minor 汇总（台账 "minor (deferred)"）：
- T1：图片缓存阈值 2 可能需要 4；Partial 组多开一台设备；`TestMetalConformanceSkip` 在没有 Metal 的 Mac 上会失败而不是跳过。
- T2：`TextureDesc` 冲突风险；`pow` 负底数（X3571）和 `frac` 边界；没有 `[loop]` 属性（X3511 风险）；`Generate` 假定被调用者在前。
- T3：依赖 DLL 的搜索顺序（`LOAD_LIBRARY_SEARCH_SYSTEM32` 加固）；`GetSystemDirectoryW` 失败时退回写死的 `C:\Windows\System32`。
- T4：只用离屏设备时没有帧队列节流。
- T5：只有最后一行不完整时，图片也整图打包。

## 8. 构建与测试

2026-11-13 本任务运行（darwin/arm64，Go 1.27）：

- darwin：`go build ./... && go vet ./...` 通过。`go test ./ui/shader/... ./ui/gpu/... ./uihost/` 全部 ok；另用 `-count=1 -timeout 40m` 不带缓存重跑一次，也全部 ok（ui/gpu 123 s、d3d11 4.0 s、metal 8.6 s、uihost 4.4 s，总计 2 分 4 秒）。
- `GOOS=windows GOARCH=amd64`：`go build ./...`、`go vet ./ui/...`、`go test -c -o /dev/null ./ui/gpu/d3d11/` 通过。
- `GOOS=windows GOARCH=arm64`：同上，通过。
- `GOOS=linux go build ./...`：通过。
- 本次检查时 `ui` 包没有被其他会话改坏，全模块检查一次通过。T2、T4、T5 期间曾因其他会话编辑 `ui`（`shad2a_defaults.go`、`nav_menu.go`、`nav_notify.go`）而暂时失败，等对方修好后重跑通过。
- 本任务只改了文档：canvas-surface.md 新增一节、gpu-renderer.md 第 7 节加一条，并新建本报告。没有改任何代码。
