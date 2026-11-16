# macOS Native Lite Host

## 目标

为 Tango 增加一个独立的 macOS 原生轻量宿主，用于小型工具窗口（设置、更新器、提示、简单表单）。它不导入 `tango.App`、WebView、`ui.Runner`、Flex、a11y 或 Tango GPU renderer，直接通过 Go + purego 调 AppKit、CoreGraphics、CoreText。

第一阶段的验收目标：

- 计数器/短文本窗口可以创建、显示、关闭、重绘和接收鼠标/键盘事件；
- CoreText 绘制 Latin、CJK 短文本；
- 默认空闲 physical footprint < 31 MiB，目标接近 mygo/NSGo；
- 启动到首帧可测量；
- 无 WebKit/WebView 子进程；
- `CGO_ENABLED=0`，不改变 Tango 现有桌面/WebView/自绘路径。

## 非目标

- 第一阶段不迁移现有 `ui` 组件；
- 不实现通用 Flex、a11y、插件、WebView 嵌入或 GPU 渲染；
- 不修改 `uihost.Open` 的默认行为；
- 不在 Linux/Windows/iOS/Android 复用 macOS 实现；
- 不把 CoreText 路径设为现有 Tango 默认文字引擎。

## 方案

考虑过三种方案：

1. 在现有 `uihost.Open` 上继续减组件：已实测仍约 44 MiB，无法达到目标；放弃。
2. 新建独立 macOS 原生宿主包：不加载 Tango App/corehook/Canvas/Runner，直接持有 NSApplication、NSWindow、NSView 和 CoreText；选择此方案。
3. 另起辅助进程：会增加 IPC 和进程固定开销，不作为基础方案。

## 模块边界

新增 `macoslite/` 包，文件使用 `//go:build darwin`；非 macOS 提供同包的 `ErrUnsupported` stub，保证跨平台引用可编译。macOS 包只依赖：

- 标准库；
- `github.com/ebitengine/purego`；
- Tango 的最小几何/输入类型只有在不引入根包时才允许；第一阶段优先使用 `macoslite` 自己的几何和事件类型。

禁止导入：

- `github.com/tangoui/tango` 根包；
- `uihost`、`ui`、`ui/paint`、`ui/text`、`internal/corehook`、`internal/platform`；
- WebKit、Metal、GPU presenter。

## API 草案

```go
package macoslite

type App struct

type Options struct {
    Name  string
    Width, Height float64
    Title string
}

type Frame struct {
    Width, Height float64
    Scale float64
}

type View interface {
    Draw(*Frame, *Canvas)
    Mouse(MouseEvent)
    Key(KeyEvent)
    Close()
}

func New(opts Options, view View) (*App, error)
func (a *App) Run() error       // 必须在主 goroutine
func (a *App) Stop()
func (a *App) Invalidate()
```

`Canvas` 第一阶段只提供：背景填充、矩形、CoreText 文本、颜色和基本圆角矩形。文本 API 返回 glyph-run 结果内部缓存，不暴露 CoreText 对象。

## 原生生命周期

1. `New` 加载 AppKit/CoreGraphics/CoreText 动态库并注册 selector/function；不创建 WebKit；
2. `Run` 创建 autorelease pool、NSApplication、delegate、NSWindow、LiteView；
3. `LiteView drawRect:` 从 Go 回调 `View.Draw`，使用 CGBitmapContext/CoreText 绘制；
4. 输入事件转成 `MouseEvent`/`KeyEvent`；
5. `Stop` 关闭窗口并释放 CF/ObjC 资源；
6. 所有 Go 回调在主线程，native handle 用 owner + `runtime.AddCleanup`，cleanup 参数不得反向引用 owner。

## 内存策略

- 不创建 Tango UI 树、Flex memo、paint list、glyph atlas 或 GPU drawable；
- CoreText font 按 family/weight/size 缓存，使用 LRU；
- 窗口关闭清理所有 CF/ObjC handles；
- `App.TrimCaches` 释放字体和 CoreText layout cache，但保留当前 bitmap 直到下一帧；
- 基准脚本测主进程 physical footprint、首帧和二进制大小；与 NSGo/mygo 只比较同一计数器页面。

## 测试与验收

- purego symbol loading fake/contract tests；
- native handle release tests（计数式 fake release）；
- CoreText Latin/CJK screenshot/像素或 glyph-run 对照；
- counter e2e：窗口可见、按钮点击改变数字、键盘 Escape 关闭；
- `CGO_ENABLED=0 go test ./...`（macOS）；
- 真实构建：`CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' ./examples/macoslite-counter`；
- `/tmp/fwbench` 同口径 footprint 基准，目标 <31 MiB。

## 第一阶段实测（2026-11-16）

`scripts/macoslite-memory.sh --rounds 3`，Apple Silicon Retina，屏幕解锁：

- 空闲 physical footprint 两次三轮中位数 **26.3 MiB / 27.2 MiB**（单轮 25.7–29.1），低于 31 MiB 目标；同机此前测得 mygo 31.2 MiB、NSGo 26.3 MiB、Tango 完整自绘 CPU 约 49 MiB；
- 启动到首帧 175–240 ms；二进制 2.2 MiB；
- 不加载 WebKit、Metal；依赖只有 `macoslite` 与 purego。

实现与草案的差异：窗口 view 走 `wantsUpdateLayer`/`updateLayer`，帧以 CGImage 作为 layer contents（与 Tango 自绘 CPU 路径一致），不走 `drawRect:`；用户关窗同样结束 `Run`（单窗口宿主）；文字用 CTLine 直接画进帧的 RGBA 像素。

已知限制：单窗口；无布局、控件、IME、无障碍、文本测量 API；字体固定为系统 UI 字体。

## 后续扩展

第一阶段达标后，再增加 `TextField`、Button、Stack/Column 等少量原生控件，并评估是否用 `macoslite` 承载 Tango 设置/更新器窗口；每次扩展都必须重新测空闲和首帧内存。
