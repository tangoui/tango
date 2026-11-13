# GPU 渲染（ui/gpu）与 Go 着色器编译器（ui/shader）

状态：已批准（2026-11-12，用户授权由设计者自审后开工）。

## 1. 意图

Tango 自绘 `ui/` 目前只有 CPU 光栅化（`paint.CPU` → `image.RGBA` → `Canvas.Present`）。代码编辑器（`docs/specs/code-editor.md`）要接替 MinCode 的 Monaco，大文件滚动与输入延迟需要 GPU；用户明确要求 GPU 渲染是必需的。GPU 渲染是整个 `ui/` 的渲染层，不是编辑器私有，因此单独立项，排在编辑器性能原型之前（编辑器中与渲染无关的部分可并行开发）。

用户选择的方案（见第 4 节）：**各平台原生图形接口 + 自研编译器，着色器用 Go 语法写一次，编译成各平台着色器语言**。

## 2. 目标 / 非目标 / 反目标

**目标**

- 实现 `paint.Renderer`，把 `paint.List` 的全部操作（FillRect、StrokeRect（含虚线）、ShadowRect、FillPath、LinearGradient、Text、Image、PushClip（含圆角）、PushOpacity、Pop）在 GPU 上画出，视觉结果与 `paint.CPU` 一致（第 6.6 节的比对口径）。
- 桌面三平台：macOS Metal、Windows Direct3D 11、Linux OpenGL 3.3 core（经 GTK3 `GtkGLArea`）。
- 着色器用 Go 语法写一次，编译为 MSL、HLSL（SM 5.0）、GLSL 330；架构上预留 GLSL ES 300（Android/鸿蒙）与 iOS Metal（与 macOS 共用 MSL），本项目不实现移动端后端。
- 桌面无 cgo：Metal/Cocoa 经 purego + objc 运行时，D3D11/DXGI 经 `syscall` + COM，GL/GTK 经 purego。
- 局部重绘：只重画变化区域（脏矩形），配合交换链的局部呈现能力。
- CPU 渲染保留：用于无头 `Driver` 测试、黄金图比对基准、GPU 不可用时自动回退。

**非目标**

- 移动端 GPU 后端（iOS、Android、鸿蒙）——随移动端项目做，本项目只保证着色器编译器能输出 GLSL ES 并通过离线编译检查。
- Vulkan、Direct3D 12、WebGPU。
- 通用 3D、自定义用户着色器 API（着色器编译器先作为 Tango 内部工具；是否对外开放以后再定）。
- GPU 路径细分曲面（FillPath 先走 CPU 光栅化蒙版 + 缓存，见 6.3）。

**反目标**

- GPU 与 CPU 渲染结果肉眼可见不同（文字粗细、圆角、阴影）。
- 为 GPU 引入外部原生库或 cgo、显著增大应用体积（目标：增量 ≤ 1 MB）。
- 照搬 mygo 的 `internal/gpu`、`internal/shader`、`internal/shaderir` 设计或代码。mygo 同样走"Go 语法着色器 → IR → 多后端"路线，我们只共享这一思路，前端、IR、后端结构按本文自行设计（见 6.4 的差异点）。

## 3. 约束

- Go 1.27；模块 `github.com/tangoui/tango`；新依赖只允许纯 Go。本设计不新增外部依赖（purego 已在用）。
- 上屏现状：`docs/specs/canvas-surface.md` 的 A 步（macOS，`internal/platform/darwin/canvas*.go`，layer-backed NSView + `layer.contents`）已有实现；B 步（Windows `StretchDIBits`、Linux GTK3 `GtkDrawingArea` + cairo）、C、D 步未完成或未实机验证。canvas-surface 明确把 GPU 列为"不做"，由本项目接手。
- `ui.RunnerOptions.Renderer`（`paint.Renderer`）与 `Window.Present(nil)` 已为 GPU 预留：GPU 渲染器自己负责呈现，`Present` 收到 nil。
- 只有 macOS 能实机验证；Windows/Linux 交叉编译 + vet，实机验收待补（同 canvas-surface B 步）。

## 4. 方案选择

| 方案 | 要点 | 取舍 |
|---|---|---|
| A 手写着色器 | 每个平台手写一套（约 8 个着色器 × 3 种语言） | 前期最小；但改一处要改 3–4 份，加平台要全部重写，各平台效果易出现细微差异 |
| **B 自研编译器（用户选定）** | Go 语法写一次，编译到 MSL / HLSL / GLSL（以后 GLSL ES） | 前期多约 1 万行；写一次、各平台一致、新平台只加一个输出后端；编译器自身需充分测试 |
| B′ naga 离线翻译 | WGSL 写一次，开发期用 naga 生成各语言 | 不用写编译器，但依赖外部 Rust 工具链，生成代码可读性一般 |
| C wgpu-native | WebGPU 统一接口 | 每平台多带 5–10 MB 原生库、引入外部 C 库，排除 |

用户在 A、B′ 与 B 之间选择了 B。附带收益（设计者补充）：Go 语法的着色器同时是合法的 Go 代码，可以**在 CPU 上直接执行**做参考实现与单元测试，这是选择 Go 语法而不是自定义语言的主要理由。

## 5. 分层

```
ui/paint            List（已有）、CPU 渲染（已有）、Renderer 接口（已有）
ui/gpu              设备无关的 GPU 渲染器：List → 批次（实例数据）→ 后端命令
  ├─ gpu/atlas      字形图集与图片纹理管理（LRU、分页、按需扩容）
  ├─ gpu/metal      macOS（purego + objc；CAMetalLayer）
  ├─ gpu/d3d11      Windows（syscall + COM；DXGI flip-model 交换链）
  └─ gpu/gl         Linux（purego；GtkGLArea 提供上下文）
ui/shader           着色器源码（Go 语法）与编译器
  ├─ shader/front   解析 + 类型检查（go/parser、go/types）+ 合法性检查
  ├─ shader/ir      自有 IR 与优化
  ├─ shader/msl     输出 MSL
  ├─ shader/hlsl    输出 HLSL
  ├─ shader/glsl    输出 GLSL 330 / GLSL ES 300
  └─ cmd/tanshader  生成器：编译 ui/gpu 的着色器，生成各语言源码（go:generate），结果提交到仓库
internal/platform   Canvas 契约扩展：GPU 呈现目标（第 6.5 节）
```

包名与拆分为建议，实现者可调整；`ui/shader` 不依赖 `ui/gpu`，可独立测试。

## 6. 已确定的设计

### 6.1 渲染模型

- 所有矩形类操作（FillRect、StrokeRect、ShadowRect、LinearGradient、Image、带圆角裁剪）用**实例化四边形 + 有向距离场（SDF）**在片元着色器里计算覆盖率，抗锯齿与 CPU 一致；阴影用解析近似（高斯模糊的圆角矩形闭式近似），与 CPU 版本按 6.6 比对。
- 虚线描边：片元着色器内按弧长计算虚线图案。
- 文字：字形蒙版一律取自 `paint.CPU.GlyphMask`（与 `paint.CPU` 共用子像素定位规则 `GlyphPlacement`），放入字形图集；GPU 按实例绘制。蒙版有两种来源：字形源实现了 `paint.GlyphRasterizer`（系统文字引擎：macOS CoreText、Windows DirectWrite、Linux FreeType）时，由平台引擎直接给出最终的 R8 覆盖率蒙版（引擎自带 gamma/对比度，不再套 `TextGamma`）；否则由 `GlyphSource.GlyphPath` 轮廓在 Go 里光栅化（纯 Go 引擎，套 `TextGamma`）。两种蒙版都带 1px 透明边（`GlyphMask` 在引擎蒙版贴边时补上），图集只认 `GlyphMask` 的结果、按 (Src, ID, 尺寸, 子像素, 倾斜) 作键，不关心来源；切换引擎会换成新的字形源，不会复用旧键。彩色字形（emoji）走 RGBA 图集页。
- FillPath：CPU 光栅化成覆盖率蒙版，按（路径、变换、尺寸）缓存进图集，GPU 当纹理绘制。
- 裁剪：矩形裁剪用剪刀测试；圆角裁剪与嵌套裁剪在着色器里用 SDF 计算（实例携带裁剪参数），不用模板缓冲。
- 不透明度组（PushOpacity）：组内容先画到离屏纹理再合成；常见的单个操作组优化为直接乘 alpha。
- 颜色：全部按预乘 alpha 在 sRGB 空间混合，与 CPU 渲染一致（不做线性空间混合，否则会和 CPU 结果不同）。
- 批处理：按（着色器、纹理、裁剪）合并连续操作，一帧目标 ≤ 20 次绘制调用（典型界面）。

### 6.2 局部重绘

- 脏区域来源：只用组件给出的 `Damage` / `ScrollRegion` 提示（见 6.8）；本帧没有任何提示时整帧重画，**不做**本帧与上帧 `paint.List` 的比对兜底。脏矩形合并到 ≤ 8 个，面积超过窗口一半时直接整帧重画。
  - 2026-11-13（M2）修订：原文为"本帧没有任何提示时，渲染器比较本帧与上帧的 `paint.List`（按操作序列与包围盒）兜底算出脏矩形"。M2 实现按审阅裁定（台账 Ruling 12）去掉了这一兜底，默认整帧更安全；这是经过审阅的偏差，M3 可重新评估是否需要兜底。
- 保留上一帧：交换链本身不保证保留内容（Metal drawable、DXGI `FLIP_DISCARD` 都不保留），所以**所有后端都渲染到一张常驻的离屏渲染目标**，只重画脏区域，再把整张离屏纹理拷贝（GPU blit）到本帧的 drawable / 后备缓冲后呈现。D3D11 的 `Present1` 脏矩形、GL 的 `EGL_KHR_partial_update` 只作为给系统合成器的提示，可选。
- 滚动优化（编辑器）：列表提供"区域平移"提示（`ScrollRegion`），渲染器在离屏纹理上平移已有像素，只画新露出部分。该提示是 `paint.List` 的新增可选操作。

### 6.3 资源

- 图集：每页 2048×2048（单通道蒙版页 + RGBA 页），满了加页，最多页数可配置，LRU 淘汰；DPI 变化时整体失效。
- 图片纹理：`paint.Image` 的 `image.Image` 以指针身份缓存，提供显式失效接口；大图单独纹理。
- 设备丢失（D3D11 `DEVICE_REMOVED`、GL 上下文丢失）：重建设备与全部资源，丢失期间回退 CPU 渲染一帧。

### 6.4 Go 着色器编译器

- **源码形式**：`ui/gpu/shaders/*.go`，普通 Go 包（无构建标签）：运行时只引用其中的数据类型（如 `Instance`），着色器函数只被生成器、CPU 执行器与测试调用（未被调用的函数在链接时被裁掉）；着色器包导入一个小的 `ui/shader/sl` 包，提供向量/矩阵类型（`Vec2/3/4`、`Mat2/3/4`）与内建函数（`Mix`、`Clamp`、`Smoothstep`、`Length`、`Dot`、`Sample`……）。`sl` 包的这些类型与函数有纯 Go 实现，因此着色器可以在 CPU 上直接执行（参考实现与测试）。
- **入口约定**：顶点函数与片元函数为带标记注释的普通函数；输入输出用结构体，字段标签标明语义（位置、插值方式）；资源（纹理、采样器、uniform 块、实例缓冲）用包级变量 + 结构体标签声明绑定位置。
- **主机可见布局**：uniform 结构体和实例属性结构体由主机按 Go 内存布局直接填缓冲区，所以 Go 布局必须就是 GPU 读的布局。前端检查：uniform 结构体（含嵌套结构体、数组）每个字段的 Go 偏移和结构体大小必须与 MSL 布局一致（float2 按 8 字节、float3/float4 按 16 字节对齐），不一致时报带位置的错误并给出两边偏移，要求调整字段顺序或加显式填充字段；`sl.Vec3`（Go 12 字节、MSL float3 16 字节）和 Go 的 `int`/`uint`（8 字节）在 uniform 与实例属性里一律拒绝，实例属性也不允许 `bool`。生成器 `tanshader` 在 `.metal` 旁边生成 `<name>_layout.go`：实例结构体的步长和按 location 排列的属性（偏移、标量类型、分量数），供设备建顶点描述符；以及实例结构体与每个 uniform 结构体大小、字段偏移的编译期断言（结构体改了却没重新 `go generate` 时编译失败）。
- **支持的子集**：标量/向量/矩阵、结构体、定长数组、`if`/`for`（有界）/`switch`、函数调用（内联或输出为函数）、常量。不支持：指针、切片、map、接口、闭包、goroutine、递归、反射——前端给出带位置的错误信息。
- **前端**：`go/parser` + `go/types`（类型检查交给标准库），再做着色器合法性检查。
- **IR**：自有的结构化 IR（保留 if/loop 结构，便于输出可读的高级着色器语言），不做 SSA 级别的激进优化；做常量折叠、死代码删除、内联。
- **后端**：MSL（Metal 2.1）、HLSL（SM 5.0，`vs_5_0`/`ps_5_0`）、GLSL 330 core；GLSL ES 300 作为同一 GLSL 后端的方言开关。各后端负责坐标系差异（Y 轴、深度范围、纹理原点）与绑定模型映射。
- **生成与运行时编译**：`go generate` 调用 `cmd/tanshader` 生成各语言源码并提交到仓库；运行时 Metal 用 `newLibraryWithSource`，GL 用 `glShaderSource`，D3D11 预编译为 DXBC 字节码嵌入（编译器 `D3DCompile` 在 Windows 生成期调用，或运行时加载系统自带 `d3dcompiler_47.dll` 编译——二选一见第 7 节）。正常构建不需要运行编译器。
- **与 mygo 的差异（刻意）**：前端直接复用 `go/types` 而不自写类型系统；IR 为结构化 IR；着色器可在 CPU 执行作为测试基准；生成物提交仓库、运行时零编译器依赖。

### 6.5 平台呈现（Canvas 契约扩展）

在 `internal/platform/canvas.go` 增加可选接口，后端实现后核心优先使用 GPU：

- macOS：canvas view 的 layer 换成 `CAMetalLayer`（`displaySyncEnabled`、`framebufferOnly`、像素格式 BGRA8Unorm），帧调度沿用现有 `updateLayer`/display link。
- Windows：canvas 的 HWND 上创建 D3D11 设备 + DXGI flip-model 交换链（`DXGI_SWAP_EFFECT_FLIP_DISCARD`，支持 `Present1` 脏矩形）；B 步的 `StretchDIBits` 作为 CPU 回退。
- Linux：canvas 控件从 `GtkDrawingArea` 换成 `GtkGLArea`（GL 3.3 core），在其 `render` 信号里渲染；X11 与 Wayland 都由 GTK 处理。B 步 cairo 路径作为 CPU 回退。
- 契约草案：`GPUCanvas` 可选接口暴露原生句柄（`CAMetalLayer*`、`HWND`、GL 上下文回调）与呈现时机；具体形状留给实现者，但须写回 `docs/specs/canvas-surface.md` 并保持 CPU 路径不变。
- 回退策略：创建 GPU 设备失败、驱动黑名单、环境变量 `TANGO_RENDERER=cpu` 时使用 CPU 渲染。Linux（里程碑 4 终审裁定，2026-11-13）：OpenGL 呈现器的 `GL_RENDERER` 不区分大小写地含 `llvmpipe`、`softpipe` 或 `swrast`（Mesa 软件光栅化：无 3D 的虚拟机、`LIBGL_ALWAYS_SOFTWARE`）时同样视为没有 GPU，关闭呈现器、撤销 GtkGLArea 切换（`CanvasGL.ReleaseGLArea`）、记一次 "GPU unavailable … software GL renderer <名字>" 日志，窗口走 CPU；`TANGO_RENDERER=gpu` 时照用软件 GL。详见 [canvas-surface.md](canvas-surface.md) Linux `CanvasGL` 一节"uihost 的选择"。

### 6.6 正确性与验收

- **黄金图比对**：复用 `ui` 现有黄金测试（`golden_test.go`、gallery），对每个页面同时出 CPU 与 GPU 截图（GPU 截图从离屏渲染目标读回），按像素比对：允许每通道差 ≤ 2、差异像素 ≤ 0.5%；文字区域另设阈值。
- **编译器测试**：每个着色器在 CPU 上执行的结果作为基准；生成的 MSL/HLSL/GLSL 通过各自的离线编译器做语法检查（macOS 上 `xcrun metal`；GLSL 用 glslang 参考编译器，若可得；HLSL 在 Windows CI 上 `fxc`/`dxc`）——工具不可得的平台跳过并标记。
- **性能**（参考机：Apple Silicon Mac，窗口 1920×1080 @2x）：
  - ui gallery 典型页面整帧渲染 GPU 时间 ≤ 2 ms，CPU 端提交 ≤ 2 ms。
  - 编辑器全屏文本（约 60 行 × 200 列，带高亮）整帧 ≤ 4 ms；单行编辑局部重绘 ≤ 1 ms。
  - 静止时零帧（不出帧、不占 GPU）。
- **体积**：GPU 渲染与着色器生成物带来的二进制增量 ≤ 1 MB。

### 6.7 里程碑

1. **编译器骨架 + macOS Metal**：`sl` 包、前端、IR、MSL 后端；Metal 后端画 FillRect/圆角/裁剪/文字（图集）；canvas 接 `CAMetalLayer`；gallery 跑通、黄金比对通过。此时编辑器性能原型可以开始。
2. **全部操作 + 局部重绘**：阴影、渐变、虚线、图片、FillPath 蒙版、不透明度组、脏矩形与滚动平移。
3. **Windows D3D11 + HLSL 后端**（交叉编译 + vet；实机验收待补）。
4. **Linux GL + GLSL 后端**（同上），GLSL ES 方言通过离线编译检查。
   - **状态（2026-11-13）**：完成。GLSL 330 / GLSL ES 300 后端（`ui/shader/glsl`）、`ui/gpu/gl` 设备（与 Metal 同一帧契约，一致性套件全组通过）与 `GtkGLArea` 呈现器、Linux `CanvasGL`、uihost 接入。只在容器（Mesa llvmpipe、Xvfb、X11/GLX）里实测过；真实 GPU、Wayland、HiDPI 硬件的实机验收待补。见 [gpu-renderer-m4-report.md](gpu-renderer-m4-report.md)。
5. **回退与稳健性**：设备丢失、CPU 回退、DPI 变化、多窗口共享设备、内存与图集上限。

### 6.8 边界决定（审阅后补充，设计者的决定）

- **Windows / Linux canvas 宿主（canvas-surface B 步）归本项目**：现在只有 darwin 与 fake 实现了 `CanvasCreator`。里程碑 3、4 各自先补齐该平台的 canvas 宿主（窗口、指针/键盘/滚轮输入、IME：Windows IMM32、Linux `GtkIMContext`；光标；DPI），同时实现 CPU 回退呈现（`StretchDIBits` / cairo），再接 GPU。若届时 B 步已由其他人完成则直接复用。
- **CPU 渲染器可以改，黄金图可以重录**：以本文为每种图元定义的算法为准（圆角/矩形覆盖率用 SDF 公式；阴影用同一闭式近似；图片缩放 GPU 在着色器里实现双三次（Catmull-Rom）以对齐 CPU；混合按预乘 alpha、8 位输出前四舍五入）。`paint.CPU` 中与之不一致的部分改成相同公式（可直接复用着色器的 Go 参考实现），改完一次性重录 `ui/testdata/*.png`。比对阈值按类别：几何与填充 ≤ 2/通道；文字与图片缩放 ≤ 4/通道；差异像素 ≤ 0.5%。
- **局部重绘与区域平移**：`paint.List` 新增两项可选信息，由本项目定义（编辑器是第一个使用者）：`Damage`（组件声明本帧变化区域，Runner 汇总）与 `ScrollRegion{Rect, Delta}`（区域平移）。渲染器**信任**这些提示；开发构建与测试里提供校验模式（整帧重画并比对，发现不一致即报错），发布构建不校验。平移区域内不得跨越裁剪边界与不透明度组（违反时退化为重画该区域）。CPU 路径同样支持（局部光栅化 + `Present` 增加可选脏矩形参数，未实现的宿主仍可整帧拷贝），`Driver` 默认整帧。
- **`tan dev` 热重载维持 CPU 渲染**：worker 进程继续用 CPU 渲染、经共享内存送像素；GPU 只在直接运行（`go run` / 发布构建）时启用。性能验收不在 dev 模式下进行。
- **GPU 路径只支持不透明 canvas**：不支持透明背景与 D 步（WebView 同窗）的按像素命中穿透；这些场景自动回退 CPU。目标是摆脱 WebView，D 步不是本项目的使用场景。
- **性能指标口径**：6.6 的帧时间按**热缓存稳态帧**计；"CPU 端提交"指从拿到 `paint.List` 到提交命令，不含 build 与布局。另设冷启动指标：编辑器全屏首帧（字形图集为空）≤ 50 ms，字形光栅化按核数并行；字号或 DPI 变化后的首帧同此标准。

## 7. 留给实现者

- `sl` 包的具体 API 命名与入口函数的标记方式（注释指令或命名约定）。
- D3D11 着色器字节码的获得方式：生成期在 Windows 上预编译嵌入，或运行时用系统 `d3dcompiler_47.dll` 编译（倾向前者，但生成期需要 Windows 环境；若不可得则后者）。
  - **已决定（2026-11-13，里程碑 3）**：运行时加载系统 `d3dcompiler_47.dll`，用 `D3DCompile` 编译嵌入的 `quad.hlsl`（`gen.QuadHLSL`）。原因是没有 Windows 生成环境。DLL 缺失时，错误包装 `d3d11.ErrNoDevice`，调用方回退 CPU；编译失败时，返回带编译器日志的错误。见 [gpu-renderer-m3-report.md](gpu-renderer-m3-report.md)。
- 批处理的排序与合并策略、实例数据布局、缓冲区环形分配。
- 脏矩形算法的合并启发式、阈值。
- GLSL 离线校验工具的选择。
  - **已决定（2026-11-13，里程碑 4）**：用 Khronos 参考编译器 glslang（`glslangValidator`，Debian 包 `glslang-tools`）。它在 Linux 测试容器里按需安装（`apt-get install -y glslang-tools`），在 PATH 上查找 `glslangValidator`，或由环境变量 `TANGO_GLSLANG` 指定路径；`ui/shader/glsl` 的 `TestGlslangValidates` 对每个测试模块的 `.vert`/`.frag` 分别按 GLSL 330 与 GLSL ES 300 校验，没有 glslang 时跳过。仓库不带二进制，macOS 上默认跳过。见 [gpu-renderer-m4-report.md](gpu-renderer-m4-report.md) 第 2 节。
- `GPUCanvas` 契约的具体方法签名（须保持 CPU 路径兼容并更新 canvas-surface 文档）。
