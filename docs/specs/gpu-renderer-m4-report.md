# GPU 渲染器里程碑 4 验收记录（Linux OpenGL 3.3）

状态：2026-11-13 完成了 GLSL 后端（GLSL 330 与 GLSL ES 300 两种方言）、`ui/gpu/gl` 设备（整帧、图片、色带、图层池、局部帧）、`GtkGLArea` 上的呈现器、Linux canvas 的 `platform.CanvasGL`，以及 uihost 接入（`uihost/gpu_linux.go`）。Linux 窗口现在默认走 OpenGL，`TANGO_RENDERER=cpu`、GL 不可用，或 `GL_RENDERER` 是软件光栅化（llvmpipe/softpipe/swrast，终审裁定，见下）时留在 CPU。

**实测范围**：只在 podman 容器 `localhost/tango-linuxtest:bookworm` 里测过（aarch64，GTK 3.24.38，Mesa 22.3.6 llvmpipe，`LIBGL_ALWAYS_SOFTWARE=1`，Xvfb，GDK 走 X11/GLX，没有窗口管理器和合成器）。另外，设备在 darwin 上通过 CGL（"4.1 Metal - 90.5"，Apple M4）跑过同一套一致性测试，但这个上下文只用于本机测试，uihost 在 darwin 上仍然只用 Metal。真实 GPU 驱动、Wayland、HiDPI 硬件和 GPU 重置都没有测过，规格 6.7 第 4 条允许"实机验收待补"，清单见第 7 节。

**审查重点 1 补充：软件 GL（2026-11-13 终审，控制者裁定）**。没有可用 GL 3.3 的机器仍按原设计回退（`UseGLArea` 报错 → 纯 CPU，`TestUseGLAreaUnavailable`）。此外，GL 能起来、但呈现器的 `GL_RENDERER`（`gl.NewPresenter` 返回的 renderer 的 `GLRenderer()` 方法）不区分大小写地含 `llvmpipe`、`softpipe` 或 `swrast` 时，uihost 也视为没有 GPU：关闭呈现器，经已有的 "GPU unavailable" 日志记一次原因 `software GL renderer <名字>`，`ReleaseGLArea` 撤销切换，窗口留在纯 cairo 路径；`TANGO_RENDERER=gpu` 时照用软件 GL。所以不开 3D 的虚拟机（Mesa 退到 llvmpipe）现在预期留在 CPU。测试：`uihost` `TestLinuxSoftwareGL`（llvmpipe/softpipe/swrast → CPU；llvmpipe + `TANGO_RENDERER=gpu` → GPU；硬件名字、空名字 → GPU）。本报告里所有容器内的 GPU 端到端运行（第 4 节）都带 `TANGO_RENDERER=gpu`（`ui-window -renderer gpu` 会设置它；探针直接设），裁定后仍然有效；容器的 llvmpipe 不带它会留在 CPU。

对应规格：[gpu-renderer.md](gpu-renderer.md) 6.5、6.6、6.7 第 4 条、6.8，以及第 7 节"GLSL 离线校验工具"（本里程碑已决定）。计划：`docs/plans/2026-11-13-gpu-m4.md`。M3 记录：[gpu-renderer-m3-report.md](gpu-renderer-m3-report.md)。平台契约：[canvas-surface.md](canvas-surface.md) 的"GPU 呈现"一节，Linux `CanvasGL` 小节。各任务报告与台账在 `.superpowers/sdd/2026-11-13-gpu-m4/`（`task-{1..7}-report.md`、`progress.md`），端到端截图与探针脚本在同目录的 `task-7-shots/`。

## 1. 已完成项

| 任务 | 内容 | 主要文件 |
|---|---|---|
| 1 | GLSL 后端 `ui/shader/glsl`：`Generate(m, GLSL330\|ES300)` 从同一份 IR 生成顶点和片元两份源码。实例属性用 `layout(location=N) in`，插值字段命名为 `v_<field>`（flat 字段和整型字段加 `flat`），片元输出 `o_color`，uniform 写成 `layout(std140) uniform <Name>_ub`，纹理写成 `uniform sampler2D`。顶点输出 `gl_Position = vec4(p.x, -p.y, 2.0*p.z - p.w, p.w)`，即 y 翻转、深度从 0..1 换到 -1..1。前端新增 std140 布局检查：std140 布局必须与 Go 布局一致，出错时报告字段名和两边的偏移。tanshader 额外生成 `quad.vert`/`quad.frag`，`gen.QuadGLSLVertex`/`QuadGLSLFragment` 用 embed 嵌入 | `ui/shader/glsl/`、`ui/shader/front/layout.go`、`ui/gpu/gen/` |
| 2 | GL 绑定层（purego；Linux 经 libepoxy 取函数指针，同时兼容 GLX 和 EGL；darwin 用 OpenGL.framework）。函数表缺任何函数就一个都不绑定。无头上下文：Linux 走 EGL（优先 surfaceless，不可用时退到默认显示加 pbuffer），darwin 走 CGL。`glThread` 是锁定 OS 线程的专用 goroutine，能处理 fn 里的 panic 和 Goexit。`compileProgram` 失败时，错误里带驱动的 info log。共享设备工具从 d3d11 搬到 `ui/gpu/internal/devkit`（LayerPool、CheckPartial、ScratchPlan 等），d3d11 的错误文本逐字不变 | `ui/gpu/gl/{funcs,thread,headless,context_*,proc_*,program}.go`、`ui/gpu/internal/devkit/` |
| 3 | GL 设备的整帧部分：常驻 `GL_RGBA8` 目标加 FBO（行 0 = 顶行），mask/color/ramp 纹理（ramp 为 `GL_RGBA16`），实例缓冲每帧孤立重分配，UBO 和 sampler 对象。每帧读 `glGetError` 计故障，连续 3 次或遇到 OOM、context reset 判丢失。上限为 `min(GL_MAX_TEXTURE_SIZE, 16384)`。`ReadPixels` 用 `glReadPixels`。所有 GL 对象都挂在 Device 上，没有包级 GL 状态（`TestGLNewDeviceAfterLost`） | `ui/gpu/gl/{device,pipeline,textures}.go` |
| 4 | 图片、图层池（`devkit.LayerPool[renderTex]`，每个池纹理有自己的 FBO）、局部帧（`gpu.Retainer`；Copies 用两次 `glBlitFramebuffer` 经 scratch 中转；清除四边形关闭混合绘制）。`TestGLConformance` 不再带 skip 表；唯一例外是只在 llvmpipe 上跳过一个子测试（见第 5 节） | `ui/gpu/gl/{layers,partial}.go` |
| 5 | Linux `platform.CanvasGL.UseGLArea()`。canvas 创建时就把 drawing area 放进 GtkOverlay，切换时再加一个 GtkGLArea 作为 overlay 子控件（GL 3.3 core，关闭 auto-render、alpha、深度和模板）。输入方面，overlay 和 GLArea 自己的事件窗口都设为 pass-through。GL 路径下的帧在 tick 和 size-allocate 里同步运行。`Present(nil)` 表示 GPU 已经呈现，`Present(img)` 会隐藏 GLArea，回到 cairo 显示。平台无关的状态逻辑放在 `canvas_gl_plan.go`；fake 后端实现了 `EnableCanvasGL` | `internal/platform/canvas.go`、`internal/platform/linux/canvas_gl_*.go`、`internal/platform/fake/canvas_gl.go` |
| 6 | 呈现器 `gl.NewPresenter(glarea)`，形状与 metal、d3d11 相同。GTK 符号由它自己 dlopen，不 import 平台包。设备在调用者线程上运行：先 `gtk_gl_area_make_current`，再复位与 GTK 共享的上下文状态。在 `render` 信号里，用一次 `glBlitFramebuffer` 把离屏目标拷进 GLArea 的帧缓冲，y 翻转一次。尺寸不一致时先清成 `Frame.Clear`，再左上对齐。uihost 新增 `glAreaTarget`；`gpu_linux.go` 的构建标签是 `linux && !android` | `ui/gpu/gl/presenter_linux.go`、`presenter_other.go`、`uihost/gpu_linux.go`、`uihost/gpu.go` |
| 7 | 端到端截图比对、光标与拖放检查（修了一个拖放缺陷，见第 4.3 节）、体积测量、规格更新、本报告 | `internal/platform/linux/canvas_drop_linux.go`、文档 |

规格第 7 节"GLSL 离线校验工具"已决定，选用 glslang，gpu-renderer.md 已加注日期，详见第 2 节。

## 2. GLSL 离线检查

- **glslang（Khronos 参考编译器，`glslangValidator`，Debian 包 `glslang-tools`，Glslang 11:12.0.0）：通过。** 容器里按需安装。Task 7 在本次运行时发现容器的 HTTP 出口被 URL 过滤，`apt-get` 拿到的是一个 HTML 拦截页，所以先把源改成 `https://deb.debian.org` 再安装。运行 `TANGO_GLSLANG=glslangValidator go test -run TestGlslang -v ./ui/shader/glsl/`，6 个模块（提交的 quad 着色器 `shaders`、`ok`、`scoping`、`names`、`exprs`、手工构造的 `select`）× 2 种方言（GLSL330、ES300）共 12 个子测试全部 PASS，每个子测试都分别校验 `.vert` 和 `.frag`。Task 1 和 Task 7 各跑了一次，结果相同。
- **GLSL ES 300**：上面 12 个子测试里有 6 个是 ES300，全部通过，满足规格 6.7 第 4 条"GLSL ES 方言通过离线编译检查"。ES 方言另外输出 `precision highp float/int/sampler2D`。
- 没有 glslang 时测试会跳过，macOS 本机就是跳过。生成器还会做一些 glslang 不管的检查，单测覆盖了这些：名字合法化（GLSL 关键字和保留字、`gl_` 前缀、连续的 `__`）、bool 插值字段报错、`sl_erf` 只在用到它的阶段输出。
- **驱动实际编译**：运行时 `compileProgram` 编译的是 GLSL 330，在 Mesa 22.3.6 llvmpipe（容器）和 Apple 的 GL 4.1 over Metal（darwin CGL）上都编译、链接成功。sampler 和 uniform block 的名字都能取到（`TestCompileQuad`、`TestQuadBindings`）。

## 3. 与 Metal 设备的语义对照

以 `ui/gpu/metal/metal_darwin.go` 的 Device 设计注释为准。GL 的设计注释写在 `ui/gpu/gl/device.go`，段落与 Metal 一一对应，另外多一段 Threads。"一致"指可见行为相同，实现方式可以不同。D3D11 一列沿用 M3 报告第 3 节的结论，方便三方对照。

| # | 条目（Metal 设计注释） | Metal | GL | 结论（括号内为 D3D11） |
|---|---|---|---|---|
| 1 | 渲染目标 | 常驻离屏 `RGBA8Unorm`，预乘 alpha，`Frame.Size` 变化时重建 | 常驻 `GL_RGBA8` 纹理加 FBO，预乘 alpha，尺寸变化时重建并把 `hasFrame` 置 false。目标按"行 0 = 顶行"存放，scissor、viewport、readback 和拷贝都直接用帧契约的左上坐标 | 一致（同 D3D11） |
| 2 | 同步 | 每帧一个 command buffer，下一次调用前等它完成 | 命令按顺序执行；每帧 `glFlush`，不等待；ReadPixels 由 `glReadPixels` 同步；资源重命名由驱动负责 | 可见行为一致；帧与帧之间没有显式等待（同 D3D11） |
| 3 | 上传 | staging 缓冲加 blit，批次之间切 pass | 按命令顺序 `glTexSubImage2D`：先 `Frame.Uploads`，然后每个批次先上传再绘制。`UNPACK_ROW_LENGTH` = Stride/bpp，Stride 不是整倍数时逐行上传；上传后把 unpack 状态恢复成默认值 | 一致 |
| 4 | 纹理重建 | 内容未定义，由 Renderer 重新上传 | 相同 | 一致 |
| 5 | 实例 | 无上限 | 每帧先 `glBufferData(nil)` 孤立，再按批次 `glBufferSubData`；容量 `max(n, 2·cap, 64 KiB)`，无上限 | 与 Metal 一致（D3D11 有 128 MiB 上限，属差异）。超大帧表现为 `GL_OUT_OF_MEMORY`，判丢失，而不是 `ErrTargetTooLarge` |
| 6 | 图片 | 每个 ImageUpload 新建 private 纹理（同 ID 替换）；ImageFrees 先释放；没有图片的批次绑 1×1 占位纹理 | 用像素直接 `glTexImage2D` 建 `GL_RGBA8` 纹理（同 ID 替换；Stride 合适时设 ROW_LENGTH，否则先打包成紧凑行）；ImageFrees 先 `glDeleteTextures`；占位纹理是 1×1 透明黑 | 一致。差异与 D3D11 相同：占位纹理已初始化；负 Stride 和校验失败算无效帧、不计故障；分配失败表现为帧末的 OOM，判丢失 |
| 7 | 图层池 | assignLayers：取至少等于、至多两倍于所需尺寸的最小空闲纹理；128 MiB；空闲 120 帧释放；纹理更大时只用左上角并缩放 UV | `devkit.LayerPool[renderTex]`，规则与常量相同；每个池纹理有专属 FBO，创建时检查完整性，不完整返回 allocError；`devkit.RescaleComposites` 先复制再缩放 | 一致 |
| 8 | 图层的危险跟踪 | 硬件 hazard tracking | 每次切换 FBO（`bindTarget`）前，把纹理单元 3、4 重置为占位纹理，保证池纹理不会同时被采样又被当作渲染目标（GL 反馈环属未定义行为）；顺序由 GL 命令流保证 | 一致（`TestGLLayerTargetSwitches`；D3D11 同法，但只有推理） |
| 9 | 清屏 | 目标的第一个 pass 清成 `Frame.Clear`，图层清成透明，之后 load | 整帧：关掉 scissor 后 `glClear` 成 `devkit.ClearColor(Frame.Clear)`；图层第一次绑定时清成透明；局部帧不清主目标 | 可见结果一致 |
| 10 | 局部帧 | Retainer；目标被重建或之后有失败时返回 `ErrNotRetained`；Copies 经 scratch；清除四边形不混合；画进目标的批次对每个脏矩形各画一次 | 相同：两次 `glBlitFramebuffer`（`GL_NEAREST`，关 scissor，坐标不翻转）；清除四边形在 `glDisable(GL_BLEND)` 下画；剪刀 = 批次剪刀 ∩ 脏矩形；scratch 尺寸用 `devkit.ScratchPlan` 决定，空闲 120 帧释放 | 一致 |
| 11 | 故障 | 等待时得到 pendingErr；连续 3 次，或 GPU 不可用，判丢失 | 帧末同步读 `glGetError`，一直读到 `GL_NO_ERROR`。`GL_OUT_OF_MEMORY` 或 `glGetGraphicsResetStatus` ≠ 0 时立即判丢失；其他 GL 错误或 FBO 不完整计一次故障，连续 3 次判丢失。reset 查询只在 GL ≥ 4.5 或带 KHR/ARB_robustness 时加载 | 一致。差异：没有 pendingErr（同 D3D11）；OOM 直接判丢失 |
| 12 | 尺寸超限 | 16384，预先拒绝，不计故障 | `min(GL_MAX_TEXTURE_SIZE, 16384)`，预先拒绝，返回包装 `gpu.ErrTargetTooLarge` 的错误，不计故障（`TestGLTargetTooLarge` 用测试钩子把上限压到 256）。无效帧也不计故障 | 一致；上限随驱动（D3D11 随特性级别） |
| 13 | ReadPixels | 只看 `hasFrame`，丢失后仍返回旧像素 | 第一帧之前、或失败帧之后报错；**丢失后返回 `ErrDeviceLost`**；读回时 OOM 判丢失，其他错误只报错、不计故障 | **差异**（与 D3D11 相同） |
| 14 | 零尺寸帧 | 返回 nil，不提交 | 不进 GL 线程，不建目标，返回 nil（`TestGLZeroSize`） | 一致 |
| 15 | 着色器 | 运行时编译 MSL | 运行时编译 GLSL 330（`gen.QuadGLSLVertex/Fragment`），失败时错误带 info log，第一行写明失败的阶段（`TestCompileErrorIncludesLog`） | 机制对应 |
| 16 | 设备选择 | 系统默认 Metal 设备 | 无头设备（`gl.NewDevice`）：EGL surfaceless 显示，不可用时退到默认显示加 1×1 pbuffer，要求 3.3 core，否则报 `ErrNoDevice`；darwin 用 CGL 3.2 core 配置（拿到 4.1）。呈现器的设备用 GtkGLArea 的上下文，由 GTK 选择驱动（X11 下走 GLX，Wayland 下走 EGL） | GL 特有 |
| 17 | 串行化 | 互斥锁 | 互斥锁；所有 GL 调用都在上下文所在的线程上（无头设备走专用的锁定线程，呈现器走 GTK 主线程） | 一致 |
| 18 | 关闭 | `ErrClosed` | `gl.ErrClosed`；幂等；在上下文线程上删除 GL 对象；`NewDevice` 建的设备连上下文和线程一起关闭；线程已经因 Goexit 结束时跳过删除 | 一致 |
| 19 | 失败帧与保留 | gpuFailed（command buffer 失败）清 `hasFrame`，编码前失败（syncFailed）不清 | GL 错误要到帧末才知道，此时帧可能已经画了一部分，所以走 `failed`，清 `hasFrame`，`Retained` 变为 false（对应 gpuFailed）。图层或 scratch 的 FBO 不完整在绘制之前就发现，走 `allocFailed`，计一次故障但**不清** `hasFrame`（对应 syncFailed），其中 OOM 判丢失 | **与 Metal 一致**（D3D11 的失败都发生在绘制之前，从不清 `hasFrame`，属差异） |

呈现器（对照 `presenter_darwin.go` 和 M3 报告的 P 系列）：

| # | 条目 | Metal | D3D11 | GL | 结论 |
|---|---|---|---|---|---|
| P1 | 目标 | `CAMetalLayer*`，设置 drawableSize | HWND 上的交换链，`ResizeBuffers` | `GtkGLArea*` 的帧缓冲，不需要手动 resize：GTK 在 size-allocate 后重建缓冲；render 时缓冲尺寸 = 分配尺寸 × `gtk_widget_get_scale_factor` | 机制对应 |
| P2 | 错误报告 | 延迟到下一次 wait | 同步 | make_current 失败或 `gtk_gl_area_get_error` 非空时立即 `ErrDeviceLost`；blit 的 GL 错误由下一次 present 返回 | 差异（与 D3D11 相近） |
| P3 | 背压 | `nextDrawable` 阻塞，maxInflight | 帧延迟 1 加 vsync | 不阻塞：`gtk_gl_area_queue_render` 会合并请求，GTK 每次绘制最多 render 一次，排队由 GTK 帧时钟和驱动的 SwapBuffers 节流 | **差异**（真机待测，见第 7 节第 8 项） |
| P4 | 拿不到 drawable、被遮挡 | 返回 nil | OCCLUDED 视为成功 | GLArea 隐藏（CPU 回退期间）或未映射时，queue_render 没有效果，render 不发生；GLArea 未 realize 时判丢失 | 对应 |
| P5 | 故障计数 | 与帧共用计数 | present 有独立计数 | render 回调有自己的连续失败计数，连续 3 次或 OOM、reset 判丢失，成功一次清零（同 D3D11） | 与 D3D11 相同 |
| P6 | 脏矩形提示 | 无 | 只在 FLIP_SEQUENTIAL 上传 | 无；GL 路径下 `PresentDamage` 一律整帧 | 与 Metal 相同 |
| P7 | 拷贝 | RGBA 目标拷到 BGRA drawable，全屏三角形逐像素拷 | 全屏三角形，后备缓冲更大时重复最后一行/列 | `glBlitFramebuffer`，`GL_NEAREST`，y 翻转一次，没有拷贝着色器；缓冲更大时先清成 `Frame.Clear`，再左上对齐 blit（**不重复边缘行列**），更小时裁剪 | 差异：resize 过程中露出的区域是 Clear 色而不是拉伸的边缘 |
| P8 | 调整大小时露出的区域 | — | 交换链背景色 = `Frame.Clear` | 清成上一成功帧的 `Frame.Clear`（第一帧之前为不透明黑） | 与 D3D11 对应 |
| P9 | 尺寸 0 | 直接返回 | nil，不提交 | nil，不排队（`TestPresenterZeroSize`） | 一致 |
| P10 | 丢失与关闭 | `ErrDeviceLost` / `ErrClosed` | 相同 | 相同。关闭：断开 render 信号 → 若 GLArea 仍以设备的 context realize（窗口还在：回退 CPU、软件 GL 被拒），make_current 后同步删除 GL 对象 → 释放对 GLArea 的引用。**关窗时不是这条路径**（终审实测更正）：GTK 3.24 的 `gtk_widget_dispose` 在发出 "destroy" 之前就递归 unrealize 了顶层，`SurfaceClosed` 到达时 GLArea 已 unrealize、context 已销毁，`Close` 不发任何 GL 调用、跳过删除，对象随 context（及其与顶层 paint context 的共享组，同一次 destroy 销毁）释放——`TestPresenterSurfaceClose`（真实 Linux canvas）实测 `realized=false`、context 已销毁、未删除；GLArea 先被单独销毁也不会崩溃（`TestPresenterAreaDestroyed`） | 一致（关窗时由 GTK 释放，不是呈现器删除） |
| P11 | 共享上下文（GL 特有） | — | — | GLArea 的上下文 GTK 也在用。设备工作前和 blit 前复位：PIXEL_PACK/UNPACK_BUFFER、pack/unpack 的 SKIP_* 和 ROW_LENGTH、ALIGNMENT、RASTERIZER_DISCARD、SAMPLE_ALPHA_TO_COVERAGE、PolygonMode；blit 前另外关闭 scissor 和 FRAMEBUFFER_SRGB，打开全部颜色通道；之后把 READ 和 DRAW 都绑回 GTK 的帧缓冲（`TestPresenterSharedState`，去掉复位做过变异测试，测试会失败） | GL 特有 |
| P12 | 线程 | 主线程，内部 LockOSThread | 主线程 | GTK 主线程。呈现器的设备在调用者线程上执行，没有专用 GL 线程 | 对应 |

## 4. 端到端

### 4.1 GPU 与 CPU 截图逐像素比对

方法：在容器里先 `go build ./examples/ui-window`，然后在 `xvfb-run -a -s "-screen 0 2200x1700x24"` 下分别用 `-renderer gpu` 和 `-renderer cpu` 启动（这个参数会设置 `TANGO_RENDERER`）。等 6 秒后执行 `xwd -root`，按 `xwininfo` 给出的窗口几何用 ImageMagick `convert -crop` 裁出窗口区域，再用一个只依赖标准库的比对程序（`task-7-shots/cmp/main.go`，不在模块里）逐像素比较 RGB 通道。`GDK_SCALE=1` 和 `GDK_SCALE=2` 各跑一次。截图工具（x11-apps、x11-utils、imagemagick）在容器里用 https 源安装成功，所以没有用"GLArea 帧缓冲读回"这个后备方案。脚本见 `task-7-shots/run.sh`。

| GDK_SCALE | 窗口（设备像素） | 完全相同 | 差 >0 | 差 >1 | 差 >2 | 差 >4 | 最大差 |
|---|---|---|---|---|---|---|---|
| 1 | 1000×760 | 99.364% | 4,834（0.636%） | 91（0.012%） | **0** | **0** | 2 |
| 2 | 2000×1520 | 99.488% | 15,572（0.512%） | 21（0.0007%） | **0** | **0** | 2 |

结论：**通过**。没有任何像素超过几何阈值 2，文字和图片阈值 4 更不会超；超阈值像素是 0%，规格上限是 0.5%。截图内容是 ui-window 的「表单」页：主题 Segmented、三张统计卡片、Tabs、Input、Select、TextArea、Slider、Switch、Checkbox、禁用和普通按钮。容器里没有 CJK 字体，所以中文显示为方框，GPU 和 CPU 两边一样。截图保存在 `task-7-shots/out/{gpu,cpu}{1,2}.png`，差异图在 `diff{1,2}.png`。

日志：GPU 运行打印 `ui-window: renderer: gpu`，CPU 运行打印 `ui-window: renderer: cpu`；四次运行都**没有** "GPU unavailable"。日志里唯一的另一行是 WebKit 的 "Overriding existing handler for signal 10"，与本里程碑无关。`uihost.Renderer` 返回 "gpu" 说明 `gl.NewPresenter` 成功、设备没有丢失。GLArea 确实在显示 GPU 帧，这一点由 Task 6 的端到端检查确认：读回 GLArea 帧缓冲中心像素，等于主题背景色，scale 1 和 2 都对。（Task 6 的检查程序在软件 GL 裁定之前运行，没有设 `TANGO_RENDERER`；裁定后同样的检查要带 `TANGO_RENDERER=gpu`，否则容器的 llvmpipe 会让窗口留在 CPU——终审修复时在容器里用 ui-window 验证过这两种情况，见 final-fix-report。）

### 4.2 光标形状

Task 5 的评审指出，GLArea 上方的光标没有测过。Task 7 写了一个临时探针程序（`internal/e2et7tmp`，跑完已删除，源码存档在 `task-7-shots/e2et7tmp_main.go.txt`），窗口里有 Input、Button 和 DropZone，打开 AcceptFiles。用 Python ctypes 调 `XWarpPointer` 把指针依次移到各控件上，再用 XFixes 的 `XFixesGetCursorImage` 读当前光标（`task-7-shots/probe.py`）：

| 位置 | GPU 窗口 | CPU 窗口 |
|---|---|---|
| Input | `text`，24×24，热点 (11,12) | 相同（像素哈希相同） |
| Button | `pointer`，24×24，热点 (8,5) | 相同 |
| DropZone、窗口角落 | 默认箭头 16×16 | 相同 |

`GDK_SCALE=2` 下结果也一样。结论：**通过**。光标仍归 drawing area 的窗口管，GLArea 的 pass-through 生效。

### 4.3 拖入文件：发现并修复一个缺陷

**现象**：同一个探针窗口，在 `TANGO_RENDERER=gpu` 下合成 XDND 拖动（`XdndEnter`/`XdndPosition`，类型 text/uri-list），GTK 回 `XdndStatus`，accept 为 0，DropZone 不高亮，日志里没有 over。CPU 窗口则 accept 为 1，有高亮。

**原因**：GTK 3 找拖放目标时，从指针下的控件开始往上找。pass-through 只影响事件投递，不影响这个查找。GLArea 盖在 drawing area 上之后，找到的是 GLArea，往上依次是 overlay、box、窗口；原来的拖放目标注册在 drawing area 上，而它是 GLArea 的兄弟，永远不会被问到。

**修复**（`internal/platform/linux/canvas_drop_linux.go`，只改了这一个产品文件）：`enableFileDrop` 改为把拖放目标（`gtk_drag_dest_set`、`gtk_drag_dest_add_uri_targets` 以及 drag-motion、drag-leave、drag-data-received 三个信号）注册在 `c.glOverlay` 上；没有 overlay 时（GTK < 3.18）仍注册在 drawing area 上。overlay 的分配和 drawing area 相同，所以拖放坐标不变。CPU 路径同样走 overlay：从 drawing area 往上找也会找到它，smokecanvas 34 项检查全部通过。

**回归测试**：`internal/platform/linux/canvas_gl_linux_test.go` 新增 `TestGLAreaFileDropTarget`：建一个 AcceptFiles 的 canvas，调用 UseGLArea，然后从 GLArea 往上找第一个带拖放目标列表的控件，断言它是 canvas 的 overlay。把修复撤掉时测试失败（`drag destination 0x15b00520, want the canvas overlay 0x15b9e130`）。用临时测试打印类型确认过：修复前从 GLArea 往上依次是 GtkGLArea、GtkOverlay、GtkBox、GtkWindow，第一个带拖放目标列表的是**顶层 GtkWindow**，它有自己的目标列表（不是 canvas 的拖放代码注册的）。GTK 停在它这里，而它没有连接 canvas 的 drag-motion 处理，所以 XdndStatus 是 accept=0，也没有高亮。恢复修复后测试通过，`GDK_SCALE=1` 和 `2` 各跑 3 次。

**修复后的探针结果**（`task-7-shots/out/probe-after-fix.txt`），GPU 和 CPU 窗口、`GDK_SCALE` 1 和 2 四种组合结果相同：

- 在 DropZone 上和 Input 上，`XdndStatus` 都是 accept=1、`XdndActionCopy`；
- 日志依次出现 `dropzone over=true` / `over=false`（移到 Input 上时）/ `true`，`XdndLeave` 之后为 false；
- 真实落下：`XdndDrop` 之后，GTK 请求选择内容，探针回 `file:///tmp/t7-dropped.txt`，收到 `XdndFinished` accepted=1，页面日志 `t7: dropped [/tmp/t7-dropped.txt]`；
- 拖动悬停时的截图（`{gpu,cpu}{1,2}-dragover.png`，DropZone 显示蓝色高亮和"release to drop"）GPU 与 CPU 逐像素比对，最大差 2，差 >2 的像素为 0。

## 5. 构建与测试

2026-11-13 本任务运行。所有 go 命令都设了 `GOCACHE=/tmp/gocache-m4`。容器命令的写法按全局约束固定，并多挂了一个 go-build 缓存目录。

- **darwin**：`go build ./... && go vet ./...` 通过；`go test -count=1 ./ui/shader/... ./ui/gpu/... ./uihost/...` 全部 ok（其中 ui/gpu 138 s，gl 7.4 s，metal 8.6 s，shaders 18 s，uihost 2.7 s）。
- **GOOS=windows**：`GOARCH=amd64` 和 `arm64` 下 `go build ./...` 都通过。
- **GOOS=linux**（在 darwin 上交叉编译）：`go build ./... && go vet ./ui/... ./uihost/...` 通过；`GOARCH=amd64 go build ./...` 通过。
- **gofmt**：`gofmt -l ui internal uihost` 无输出。
- **容器**：
  - `go vet ./ui/... ./uihost/... ./internal/platform/linux/...` 通过。
  - `xvfb-run -a … go test -count=1 ./ui/gpu/gl/ ./internal/platform/linux/... ./uihost/...`：`ui/gpu/gl` ok，`internal/platform/linux` ok；uihost 只有下面列出的 3 个既有失败。加上 `-skip 'TestAttachSurfaceCloseClosesRunner|TestAttachClosedSurface|TestSharedSubscription'` 后 uihost ok。
  - `ui/gpu/gl -v`：220 个 PASS。SKIP 只有两项：`Partial/VerifyFractionalScroll/1`（llvmpipe 专属，见下）和 `TestPresenterScale2Child`（它是子进程入口，由 `TestPresenterScale2` 带 `GDK_SCALE=2` 再启动一次，在子进程里 PASS）。
  - `TANGO_GLSLANG=glslangValidator go test ./ui/shader/... ./ui/gpu/...`：shader 各包 ok，`TestGlslangValidates` 12 个子测试都 PASS；ui/gpu 的子包都 ok；`ui/gpu` 本身只有 `TestRendererAtlasOverflow` 失败（既有问题，见下）。
  - CPU 路径回归：`go run ./internal/platform/linux/internal/smokecanvas -smoke` → `PASS: all 34 checks (+2 optional)`（包含本任务的拖放改动）。

**已知的既有容器失败**：不装 M4 的代码也同样失败，本里程碑没有修，留给相应负责人：

| 包 / 测试 | 现象 | 证据 |
|---|---|---|
| `ui/gpu` `TestRendererAtlasOverflow`（go、system 两个子测试） | `frame 1: evictions 0, glyph instances 2, patches 0; want 2, 2, 0`。推测 Linux 上的字体或字形尺寸让图集没有溢出 | darwin 通过；在主 checkout（不含 M4）上的容器里同样失败（台账 Task 3 记录）；`ui/gpu` 不 import `ui/gpu/gl` |
| `uihost` `TestAttachSurfaceCloseClosesRunner` | `Detached false after the surface closed` | 把 Task 6 的 uihost 改动全部还原后同样失败（Task 6 报告） |
| `uihost` `TestAttachClosedSurface` | `Attach to a closed surface succeeded` | 同上 |
| `uihost` `TestSharedSubscription` | `openWith` 里 `*Surface` 为 nil，panic，导致包内其余测试不再运行 | 同上 |

**只在 llvmpipe 上跳过的子测试**：`TestGLConformance/Partial/VerifyFractionalScroll/1`，依据是设备返回的 `GL_RENDERER` 含 "llvmpipe"，不看 GOOS。测量依据：在 llvmpipe 上分别整帧渲染 off=6.3 和 off=12.3，按 6 行错位逐像素比较，像素 (280,51) 为 210 和 208，差 2；而 Renderer 的 `verifyCopyTolerance` 是 1。darwin 上同样的比较最大差为 1。blit 本身逐位精确；差异来自 llvmpipe 在半覆盖的渐变边缘行上，抗锯齿舍入与 Dither4x4 相位叠加。`verifyCopyTolerance` 和 d3d11 都没有改。**真实 GPU 上这个子测试必须运行并通过**（第 7 节第 1 项）。

## 6. 二进制体积（规格 6.6：增量 ≤ 1 MB）

方法：在 darwin 上用 `GOOS=linux GOARCH=<arch> go build -trimpath -ldflags='-s -w'` 构建 `examples/ui-window` 两次：

- **GL 版**：当前代码，uihost 在 Linux 上链接 `gl.NewPresenter`；
- **无 GPU 版**：用 `-overlay` 把 `uihost/gpu_linux.go` 换成 `gpu_other.go` 的桩，只把构建标签改成 `linux`。

临时文件放在 `/tmp/t7m4-size/`，仓库里没有新增文件。`go list -deps` 显示 GL 版多了 `ui/gpu/gl`、`ui/gpu/gen`、`ui/gpu/internal/devkit`。无 GPU 版的依赖里仍然有 `ui/gpu`、`ui/gpu/atlas`、`ui/gpu/shaders`、`ui/shader/sl`，因为 uihost 引用了 `gpu` 包的错误值，但链接器把它们几乎全部裁掉了：未剥离符号时，这些包的符号只剩 5 个，共 151 字节。

| 构建（linux） | amd64（字节） | arm64（字节） |
|---|---|---|
| 无 GPU（桩） | 10,985,735 | 10,092,807 |
| GL 版 | 11,383,047 | 10,354,951 |
| **GPU 增量（GL − 无 GPU）** | **397,312（388 KiB）** | **262,144（256 KiB）** |

结论：**达标**（≤ 1 MB）。作为对照，M3 的 d3d11 增量是 amd64 388,608 字节、arm64 323,072 字节，M2 在 macOS 上是 305,024 字节。

辅助数据：对未剥离符号的 GL 版（amd64）运行 `go tool nm -size`，各包符号大小合计：

| 包 | 符号字节数 | 符号数 |
|---|---|---|
| `ui/gpu` | 99,131 | 75 |
| `ui/gpu/gl` | 62,101 | 149（其中 text 59,741；arm64 下合计 60,800） |
| `ui/gpu/internal/devkit` | 9,414 | 16 |
| `ui/gpu/atlas` | 9,035 | 12 |
| `ui/gpu/gen` | 416 | 4（`QuadInstanceAttrs` 360、`QuadTextures` 24，`QuadGLSLVertex`/`QuadGLSLFragment` 各 16 字节的字符串头） |

GLSL 生成物的正文是两段字符串：`quad.vert` 3,109 字节、`quad.frag` 15,443 字节，共 18,552 字节，计在匿名的 `go:string.*` 数据里，不在上表中。GL 版二进制里能找到 `#version 330 core`；`quad.metal`（`metal_stdlib`）和 `quad.hlsl`（`vs_main`）都没有被链接进 Linux 二进制。剩下的增量来自 pclntab、类型元数据和字符串。

## 7. 实机待补清单

前提：一台装了真实 GPU 的 Linux 桌面，x86_64 和 aarch64 各一台更好，GTK 3.24、Go 1.27，在仓库根目录执行。下面多数项要在四种环境里各跑一遍：**X11 + Mesa 硬件驱动**（Intel 或 AMD）、**Wayland 会话**（GNOME 或 KDE，GDK 走 EGL）、**NVIDIA 专有驱动**（X11 和 Wayland 各一次）、**HiDPI 屏**（整数缩放 2，以及可能的话分数缩放 1.25/1.5）。不要设置 `LIBGL_ALWAYS_SOFTWARE`。

| # | 命令 | 期望结果 |
|---|---|---|
| 1 | `go test -count=1 -v ./ui/gpu/gl/` | 全部 PASS。`GL_RENDERER` 打印真实 GPU 的名字，不含 "llvmpipe"。**`TestGLConformance/Partial/VerifyFractionalScroll/1` 必须运行并 PASS**（llvmpipe 上被跳过的那一项）。重点关注：`TestGLConformance` 各组（几何 ≤ 2、文字和图片缩放 ≤ 4、超阈值像素 ≤ 0.5%）、`TestGLGallery`、`TestPresenter*`（Basic、Resize、Scale2、SharedState、AreaDestroyed、SurfaceClose、ContextChanged、GLRenderer；SurfaceClose 子进程输出的 `CLOSE-ORDER …` 一行要记录：期望 `realizedAtSurfaceClosed=false … closeDeletedWithContext=false`，Wayland/EGL 下若不同也记下）。`-v` 日志里 `glGetGraphicsResetStatus available: …` 一行要记录下来 |
| 2 | `go test -count=1 ./internal/platform/linux/ ./uihost/...` | linux 包 PASS，包括 `TestUseGLArea`、`TestGLAreaInputPassThrough`（XTest，只能在 X11 下跑，Wayland 下会以 "XTest needs X11" SKIP，改用第 9 项手工检查）、`TestGLAreaFileDropTarget`。uihost 只允许出现第 5 节那 3 个已知失败；如果在真机上不失败，也记下来 |
| 3 | `TANGO_GLSLANG=glslangValidator go test -run TestGlslang -v ./ui/shader/glsl/`（先 `apt install glslang-tools`） | 12 个子测试都 PASS |
| 4 | `go test -bench BenchmarkGLGalleryFrame -run '^$' ./ui/gpu/gl/` | 输出 submit-ms 和 exec-ms。参考：darwin CGL 约 0.2 ms/帧，llvmpipe 约 1.0 ms/帧（含软件光栅化）。目标 ≤ 2 ms |
| 5 | 并排运行 `go run ./examples/ui-window -renderer gpu` 和 `-renderer cpu`，两个窗口尺寸相同（默认 1000×760）。三个标签页各截一张只含窗口内容的图（X11：`import -window <id> gpu.png`；Wayland：`gnome-screenshot -w -f gpu.png` 或 `grim -g "$(slurp)" gpu.png`，两张图要裁成同样大小）。用 ImageMagick 比对：<br>`n=$(identify -format '%[fx:w*h]' gpu.png)`<br>`compare -metric AE -fuzz 0.8% gpu.png cpu.png null:; echo " / $n"`（几何阈值）<br>`compare -metric AE -fuzz 1.6% gpu.png cpu.png null:; echo " / $n"`（文字/图片阈值）<br>（`compare` 把计数打印在 stderr，退出码 1 只表示"有差异"。`-fuzz 0.8%` 正好放过每通道差 ≤ 2、拦下差 3；`1.6%` 放过差 ≤ 4、拦下差 5。这是 Task 7 在 ImageMagick 6.9.11 上用单像素图标定的，单通道和三通道同时偏移结果相同） | 日志 `ui-window: renderer: gpu`，没有 "GPU unavailable"；**判定只看 `-fuzz 1.6%` 那条**：它的 AE ≤ 像素总数的 0.5%（1000×760 时为 3,800）即通过。fuzz 越大 AE 只会越小，所以 `-fuzz 0.8%` 的计数只作参考；它超过 0.5% 时，用 `compare -fuzz 0.8% gpu.png cpu.png diff.png` 生成差异图，确认多出来的像素只落在文字和图片上（几何与填充仍须 ≤ 2）。容器里对 Task 7 的截图，两条命令的 AE 都是 0。另外记下 `compare -version` 的输出：fuzz 标定是在 ImageMagick 6.9.11 上做的（按通道计的 AE），如果用的是 ImageMagick 7，先拿一个已知偏移（例如单通道 +3，`-fuzz 0.8%` 应得 AE 1）复核，再信任这些数字。逐项看边框、圆角、阴影、Progress、Rate 星形、Tag、禁用按钮的半透明、文字 |
| 6 | 在第 5 项的 GPU 窗口里：拖动右下角在约 800×600 与 1400×950 之间来回 10 秒；最大化再还原；最小化后恢复；移到另一块缩放不同的显示器（或在设置里改缩放比例） | 没有拉伸，内容左上对齐；resize 时露出的区域是页面背景色（`Frame.Clear`），允许短暂出现背景色带，不允许出现未初始化的垃圾或闪黑；缩放切换后文字清晰，不模糊，也不是只占窗口四分之一（`GDK_SCALE=2`：缓冲 = 逻辑尺寸 × 2） |
| 7 | 静止时零帧：鼠标移出窗口，不碰键盘，等 10 秒，前后各看一次 `ps -o time= -p $(pgrep ui-window)` | 两次值相同（GTK 帧时钟停止，不出帧） |
| 8 | 背压与输入延迟：在 GPU 窗口里来回拖「等级」滑块、拖动整页滚动条、快速打字、连续点「通知」按钮约 10 秒，同时用 `top` 看 CPU 占用（可选：`MESA_DEBUG=1` 或 `__GL_SYNC_TO_VBLANK=1`，后者是 NVIDIA 的变量） | 跟手程度不比 CPU 窗口差；CPU 占用不高于 CPU 窗口；不会因为 queue_render 合并而丢掉最后一帧（停止拖动后画面是最终状态）。**虚拟机（不开 3D，llvmpipe）**：不设 `TANGO_RENDERER` 时窗口留在 CPU（见第 11 项），不用比；想比较 CPU 占用只在 `TANGO_RENDERER=gpu`（`-renderer gpu`）下做，与 `-renderer cpu` 对照，结果只作记录（预期 llvmpipe 更高，这正是裁定的理由） |
| 9 | 输入、光标、IME：在「姓名」输入框里用 fcitx5 或 ibus 拼音输入；悬停在输入框、按钮、表格列头上 | 候选窗位置正确；光标随控件变化（文本 I 形、手形），与 CPU 窗口相同（容器里已用 XFixes 比对过，真机上目测） |
| 10 | 拖入文件：仓库里没有带 AcceptFiles 的窗口示例（ui-gallery 只离线渲染 PNG，不开窗口），先把本节末尾"附：拖放检查程序"存为 `/tmp/dropcheck/main.go`，在仓库根目录执行 `go run /tmp/dropcheck/main.go -renderer gpu`（再用 `-renderer cpu` 跑一次）。从 Nautilus 或 Dolphin 拖一个文件到虚线框里，X11 和 Wayland 各一次 | 悬停时虚线框变蓝、文字变成 "release to drop"，日志 `dropzone over=true`；移出窗口后 `over=false`；松开后日志 `dropped [<路径>]`。GPU 与 CPU 窗口表现相同。Wayland 下拖放走 wl_data_device 而不是 XDND，容器里没有覆盖到 |
| 11 | GL 不可用时：`GDK_GL=disable go run ./examples/ui-window -renderer gpu`，以及在不支持 3.3 的环境里运行（`ssh -X` 转发）；软件 GL：虚拟机不开 3D（或 `LIBGL_ALWAYS_SOFTWARE=1`），`go run ./examples/ui-window` 不带 `-renderer`，再带 `-renderer gpu` 各一次 | GL 不可用：打印一次 `tango: WARNING: TANGO_RENDERER=gpu but GPU unavailable, using CPU: …`，窗口用 CPU 正常显示，不崩溃。软件 GL 不带 `-renderer`：打印一次 `tango: GPU unavailable, using CPU: software GL renderer llvmpipe (…) (TANGO_RENDERER=gpu uses it)`，日志 `ui-window: renderer: cpu`，GTK Inspector 里 overlay 下没有 GtkGLArea；带 `-renderer gpu`：没有该日志，`renderer: gpu` |
| 12 | GPU 重置：在运行第 5 项时触发重置（Intel：`echo 1 \| sudo tee /sys/kernel/debug/dri/0/i915_wedged`；AMD：`sudo cat /sys/kernel/debug/dri/0/amdgpu_gpu_recover`；NVIDIA 没有通用办法，可以跳过）（竖线前的反斜杠只是 Markdown 表格转义） | 日志 `tango: uihost: surface N: GPU lost, using CPU: …`，窗口切到 CPU 继续显示，不崩溃。GtkGLArea 的上下文通常不是 robust 上下文，重置可能表现为 GL 错误连续 3 次或 make_current 失败，两种都算通过；记录实际是哪一种 |
| 13 | 关闭顺序：正常关窗（标题栏 ×、Alt+F4）；先让 GPU 回退（第 12 项）再关窗 | 都不崩溃，进程正常退出，没有 GTK/GL 警告 |
| 14 | `GTK_DEBUG=interactive TANGO_RENDERER=cpu go run ./examples/ui-window -renderer cpu`，在弹出的 GTK Inspector 的 Objects 页展开窗口的控件树；再用 `-renderer gpu` 对照一次 | 日志 `ui-window: renderer: cpu`；CPU 运行时 GtkOverlay 下只有一个 GtkDrawingArea，没有 GtkGLArea；GPU 运行时 GtkOverlay 下多一个 GtkGLArea |
| 15 | 有合成器的桌面（GNOME/KDE）上，GTK 用 GL 合成 GLArea（不经过 `glReadPixels` 回读） | 画面正立、颜色正确（预乘 alpha、无 alpha 通道的 GLArea 不透出桌面） |

附：拖放检查程序（第 10 项；在仓库根目录用 `go run` 运行，模块内的包都能直接 import）：

```go
// Command dropcheck: a window with a DropZone that accepts dropped files.
package main

import (
	"flag"
	"log"
	"os"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/uihost"
)

var lastOver bool

func page(c *ui.Context) {
	ui.Column(c, func() {
		z := ui.DropZone(c, func(over bool) {
			if over != lastOver {
				lastOver = over
				log.Printf("dropzone over=%v", over)
			}
			msg := "drop here"
			if over {
				msg = "release to drop"
			}
			ui.Column(c, func() { ui.Text(c, msg) }, layout.Pad(24), layout.MinH(200), layout.Center())
		})
		if p := z.Dropped(); len(p) > 0 {
			log.Printf("dropped %v", p)
		}
	}, layout.Pad(20))
}

func main() {
	renderer := flag.String("renderer", "", `"cpu" or "gpu"`)
	flag.Parse()
	if *renderer != "" {
		os.Setenv("TANGO_RENDERER", *renderer)
	}
	quit := true
	app := tango.New(tango.Options{Name: "dropcheck", ID: "dev.tango.dropcheck", QuitOnLastClose: &quit})
	app.OnReady(func() {
		r, s, err := uihost.Open(app, uihost.Options{Surface: tango.SurfaceOptions{
			Title: "dropcheck", Width: 600, Height: 420, AcceptFiles: true,
		}}, page, ui.RunnerOptions{})
		if err != nil {
			log.Fatal(err)
		}
		uihost.AcceptDrops(r, s)
		log.Printf("renderer: %s", uihost.Renderer(r))
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
```

## 8. 已知风险

1. **只在 llvmpipe/Xvfb/GLX 上运行过。** 硬件驱动的着色精度、`glBlitFramebuffer` 在不同驱动上的行为、NVIDIA 专有驱动下 GLX 与 libepoxy 的组合、Wayland 下 GDK 的 EGL 上下文都没有验证。
2. **背压完全依赖 GTK**：呈现器不阻塞，帧节奏由 GTK 帧时钟和驱动的 SwapBuffers 决定（P3）。长帧或快速 resize 时的表现要等真机确认（第 7 节第 6、8 项）。
3. **尺寸不一致时不重复边缘行列**（P7）：快速 resize 时可能短暂看到一条 Clear 色的带子，而不是像 Metal/D3D11 那样拉伸边缘。这是 brief 规定的行为。
4. **GPU 重置检测较弱**：无头 EGL 上下文不是 robust 上下文，`glGetGraphicsResetStatus` 实际上一直返回 0；GtkGLArea 也不保证 robust。重置多半表现为 GL 错误或 make_current 失败，然后走计数丢失。
5. **实例数量没有上限**（第 5 条）：极端大的帧会变成 `GL_OUT_OF_MEMORY` 并判丢失，回退 CPU，而不会像 D3D11 那样只把这一帧交给 CPU 画。
6. **线程**：GL 调用只在上下文所在的线程上进行。如果同一进程里某个线程留下了 GLX current（违反"呈现器只在主线程调用"的约定），之后再建的无头 EGL 设备会在 `eglMakeCurrent` 时得到 `EGL_BAD_ACCESS`（libglvnd）。这只影响 `gl.NewDevice`（测试和读回），返回错误，不会崩溃（Task 6 实测）。
7. **构建标签与移动端**（交给 min-95）：
   - `ui/gpu/gl` 的设备代码标签是 `linux || darwin`，**也包括 android**。purego 在 android 上必须开 cgo（`dlfcn_android.go` 依赖 `cgo.Dlopen`），所以 `GOOS=android` 不开 cgo 时 vet 不过。Android 移植时，要么开 cgo，要么把标签改成 `(linux && !android) || darwin`。`presenter_linux.go` 按文件名也会在 android 上编译（运行时 dlopen GTK 失败，只会报错）；uihost 的 `gpu_linux.go` 已经是 `linux && !android`，Android 一律走 CPU。
   - **ios** 满足 `darwin` 标签，会编译 CGL 上下文代码。iOS 没有 OpenGL.framework，按代码推断，运行时加载会失败，并返回包装 `ErrNoDevice` 的错误，而不是 `ErrUnsupported`。这一点是推断，没有在 iOS 上构建或运行验证过。
   - **`GL_RGBA16`（ramp 纹理）不在 GLES 3.0 核心里**，需要 `EXT_texture_norm16`。复用这个设备做 Android（GLES 3.0）时，要换成 `RGBA16F` 或两张 RGBA8，或者检查这个扩展。
   - Android 的 context loss：设备的全部 GL 对象都挂在 Device 上，丢失后在新上下文上 `newDevice(run, f)` 重建即可（`TestGLNewDeviceAfterLost`、`TestGLNewDeviceSharedRun`），M4 没有加 ResetDevice API。
8. **Ramp 上传假设小端**（amd64/arm64），代码注释里写明了。
9. **d3d11 的测试 bug**（交给 M3 Windows 实机清单的负责人，min-ab）：`ui/gpu/d3d11` 的 `TestD3DScrollOverlap` 在 Renderer 那一半很可能会在真实 Windows 上失败。verify 模式在局部帧之后还会跑一次整帧，`gpuconf.LastFrame` 记下的是这次整帧，Copies 为空，所以断言 "copies [], want one" 一定失败。GL 版的同名测试（`TestGLScrollOverlap`）改用 `partialCopies` 包装，只收集局部帧的 Copies。d3d11 按规定没改；实机跑 M3 清单第 1 项时如果遇到这个失败，属于测试问题，不是设备问题。
10. **共享上下文**：GTK 可能留下的状态已经按清单复位（P11），但 GTK 以外的 GL 用户（例如同一窗口里的其他 GLArea）没有考虑。呈现器在 `NewPresenter` 时记下 GLArea 的 GdkGLContext（GObject 弱引用，context 销毁时得到通知）；之后每次 make_current 都比对：GLArea 被 unrealize（context 已销毁）或 unrealize 后又 realize（换了新 context）时返回错误，即判设备丢失，uihost 回退 CPU；render 回调里遇到不是这个 context 时先清成黑色再判丢失，`Close` 也不会在新 context 里删名字（`TestPresenterContextChanged`）。
11. **拖放**：第 4.3 节的修复只在容器里用合成的 XDND 验证过。Wayland 下拖放走另一套协议，GTK 找目标控件的逻辑相同，但没有测过（第 7 节第 10 项）。

## 9. 偏差与裁定

台账（`.superpowers/sdd/2026-11-13-gpu-m4/progress.md`）中的 Ruling：

1. 不提交 git（tango 的提交只由 min-ab 做）；进度点用影子树快照（`snap`/`pkg` 脚本）。
2. 实现用 opus；Task 1、2 的评审用 sonnet，Task 3–6 的评审用 opus（涉及并发和 GL 状态）。
3. Task 2：GL 常量不导出（约 100 个 `gl` 前缀的常量，公开 API 只有 `ErrUnsupported`/`ErrNoDevice`/`ErrClosed`）。
4. Task 3：ramp 的 `GL_UNPACK_ROW_LENGTH` = Stride/8（计划写的 Stride/4 是计划本身的错误）；新增 `BlendFunc`。
5. Task 3：应 min-95 的请求，不加 ResetDevice API；改为要求 GL 对象全部挂在 Device 上，并加 `TestGLNewDeviceAfterLost`。
6. Task 4：`Partial/VerifyFractionalScroll/1` 只在 llvmpipe 上跳过（第 5 节），`verifyCopyTolerance` 不改。
7. Task 5 归本项目做（min-ab 同意），按 min-ab 的五条要求实现：plan 文件、`CanvasGL` 文档照 `CanvasHWND`、`gpu_linux.go`、fake、canvas-surface.md 写明实测范围。
8. Task 6：`NewPresenter` 的桩放在新文件 `presenter_other.go`（`!linux`），`device_other.go` 不动；`gpu_linux.go` 的标签用 `linux && !android`，`gpu_other.go` 用 `!darwin && !windows && !(linux && !android)`。
9. Task 7：截图工具在容器里装上了（https 源），没有用帧缓冲读回的后备方案；拖放缺陷改动很小，就在本任务里修了，并补了回归测试（第 4.3 节）。
10. 终审（控制者裁定）：`GL_RENDERER` 含 llvmpipe/softpipe/swrast 时 uihost 视为没有 GPU，留在 CPU，`TANGO_RENDERER=gpu` 例外（见开头"审查重点 1 补充"）；为此 `platform.CanvasGL` 追加 `ReleaseGLArea()`，呈现器起不来或被拒时撤销切换、回到纯 cairo 路径。修复记录：`.superpowers/sdd/2026-11-13-gpu-m4/final-fix-report.md`。

延后处理的 minor（台账 "minor (deferred)"）：
- T1：只有 GLSL 后端会报 bool 插值字段错误；整型 Select 的测试较弱；以 `_` 结尾的名字加 `_ub` 的命名；IR 遍历和作用域代码有了第 3 份副本；std140 错误位置没有测试；tanshader 文档没有提 GL。
- T2：bindLib 报告缺失名字的顺序不固定（遍历 map）；`headless.closed` 不是并发安全的，close 后再 do 会 panic，且 close 与 alive 判断之间有竞态（Device 已用 ErrClosed 挡住）；d3d11 的 layerPool 内嵌 `devkit.LayerPool`，提升出了不带前缀的 `Assign`；`ShaderSource` 的 `**byte` 指向含 Go 指针的 Go 内存（可以考虑 `runtime.Pinner`）。
- T3：`checkUploads` 和 FBO 不完整的提前返回会吞掉分配产生的 GL 错误（OOM 不会立刻判丢失）；测试钩子没有同步（注释要求测试不能并行）。
- T4：没有拷贝时，`copyRegions` 仍会绑定默认 FBO；无效的层帧在 `applyImages` 改动图片状态之后才被拒绝（与 D3D11 相同）；`allocFailed` 重复了 `failed` 的计数和判丢失逻辑。
- T5：resize 引起的 CPU 回退期间，`Present(img)` 可能在 overlay 的 size_allocate 中隐藏 GLArea，未测；每次 Present 切换显隐都会让 overlay 重新布局；XTest 不可用时穿透测试会静默跳过，且测试结束后指针留在窗口上；`t.Fatalf` 在非主线程读 `c.w/c.h`；切换失败后留下一个已 realize 的隐藏顶层；fake 复用了以 metal 命名的类型。（Task 5 的"拖放高亮与光标未测"已在本任务关闭，见第 4.2、4.3 节。）
- T6：设备已关闭或已丢失时，render 回调返回 TRUE 但不清屏（回退 CPU 之前会显示旧缓冲）；恢复 READ 帧缓冲时假设 GTK 进入回调时 READ 和 DRAW 绑的是同一个。（"`p.size` 只写不读"与"不记住 GdkGLContext"已在终审修复中解决，见第 8 节第 10 项。）

## 10. 改动文件（Task 7）

- 修改：`internal/platform/linux/canvas_drop_linux.go`（拖放目标改到 overlay）、`internal/platform/linux/canvas_gl_linux_test.go`（`TestGLAreaFileDropTarget`、`newGLTestCanvasEx`，测试用的 FFI 新增 `gtk_widget_get_parent`、`gtk_drag_dest_get_target_list`）、`docs/specs/gpu-renderer.md`（里程碑 4 状态；第 7 节 GLSL 校验工具的决定）、`docs/specs/canvas-surface.md`（Linux CanvasGL 一节新增"拖入文件"一条，"未测"补上真实文件管理器拖入）。
- 新增：本报告。
- 临时程序 `internal/e2et7tmp/` 已删除；截图、探针脚本和比对程序都在 `.superpowers/sdd/2026-11-13-gpu-m4/task-7-shots/`（目录名以点开头，Go 工具链会忽略）。
