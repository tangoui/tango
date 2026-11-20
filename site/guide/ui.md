# 自绘 UI（实验性）

除了 WebView，TanGo 还可以用纯 Go 画界面：`github.com/tangoui/tango/ui` 是一个即时模式 API + 保留式场景的 UI 库，自带一套组件（按钮、表单、表格、日期选择、树、弹层……），`uihost` 把它放进原生窗口。不需要 HTML、JS，也不需要 WebView 进程。

::: warning 实验性
- 桌面自绘窗口已在 macOS 实机验证。Windows 和 Linux 有 CI 冒烟覆盖。iOS 在模拟器和一台 iPhone 12 Pro 上测过，Android 在模拟器上测过。 [平台支持](/guide/platforms)
- Apple Silicon macOS 默认用原生 Core Animation 图层。在默认的 `ui.ContentGeneral` 窗口里，非修饰键按下、文字提交或输入法组合会触发单向切换到 Metal。`ui.RunnerOptions{Content: ui.ContentData}` 适合列表、表格和仪表盘，用键盘浏览时保留原生图层。两种窗口在连续六帧绘制耗时超过 8 ms 或连续六帧全窗口软件回退时也会切换到 Metal。
- Intel Mac、Windows、Linux、iOS 和 Android 的完整自绘窗口或页面默认走 GPU（Metal、Direct3D 11、OpenGL 或 OpenGL ES），设备不可用时回退到 CPU。macOS 文字引擎默认只编译 CoreText，添加 `-tags tango_gotext` 可恢复 go-text 路径。
:::

## 打开一个自绘窗口

```go
package main

import (
    "fmt"

    "github.com/tangoui/tango"
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
    "github.com/tangoui/tango/uihost"
)

var name string
var clicks int

func page(c *ui.Context) {
    ui.Column(c, func() {
        ui.Title(c, "你好，TanGo", 3)
        ui.Input(c, &name)
        if ui.Button(c, "点我").Clicked() {
            clicks++
        }
        ui.Text(c, fmt.Sprintf("点了 %d 次", clicks))
    }, layout.Gap(12))
}

func main() {
    app := tango.New(tango.Options{Name: "UI Demo", ID: "com.example.uidemo"})
    app.OnReady(func() {
        uihost.Open(app, uihost.Options{Surface: tango.SurfaceOptions{
            Title: "UI Demo", Width: 800, Height: 600, Resizable: true,
        }}, page, ui.RunnerOptions{})
    })
    app.Run()
}
```

每一帧调用一次页面函数，创建组件节点、读取上一帧的交互（`Clicked()`、`Changed()`）并更新 Go 变量。没有事件、动画或输入时不出帧。页面函数和 `Runner` 回调默认在应用主线程上运行。见下文[后台数据与线程](#后台数据与线程)。

更完整的例子：`examples/ui-window`（表单、表格、对话框、主题切换、中文输入法）、`examples/ui-gallery`（全部组件）。

## 组件库

组件以 Ant Design 6.7 和 Ant Design Pro v6 为设计基线实现，Element Plus 主题对照 Element Plus 2.14.8，shadcn 主题对照 shadcn/ui v4（见 [设计基线](https://github.com/tangoui/tango/blob/main/docs/design-baseline.md)）。每个组件只有一个公开入口，按通用、布局、导航、数据录入、数据展示、反馈分组。每个组件的文档和在线示例见 [组件](/components/)，静态截图见 [组件截图目录](/components/screenshots)。

编写新组件的约定（文件命名、节点与保留状态、布局、主题令牌、绘制、焦点与输入法、浮层、测试）在仓库的 [ui/AUTHORING.md](https://github.com/tangoui/tango/blob/main/ui/AUTHORING.md)。

### 大数据量组件

- `AutoComplete`、`Mentions`、`Command`、`Cascader`、`CascaderPanel` 用 `Virtual(bool)` 显式开关虚拟化，原地修改选项时递增 `DataVersion(int)` 让缓存重算。
- `List`、`ProList`、`Conversations` 可用 `Height(float32)` 加 `Virtual()` 限制渲染范围。`List` 可设固定 `ItemHeight(float32)` 或用 `EstimatedItemHeight(float32)` 估算未测量条目高度。`RowKey(func(int) any)` 为高度缓存提供稳定身份，`ScrollToKey(key, align, offset)` 定位条目，`Range()` 返回上次构建的半开区间 `[first, end)`。
- `ProTable`、`EditableProTable`、`CellEditorTable`、`RowEditorTable` 用 `ScrollY(float32)` 加 `Virtual()` 虚拟化表体，`ProTable` 可设 `RowHeight(float32)`。`Tree` 的 `DataVersion(int)` 配合 `Height` 启用缓存，原地改节点结构、筛选结果或勾选规则后应更新版本。

## 主题

`ui/theme` 提供三套设计令牌：`theme.AntDesign()`（默认）、`theme.ElementPlus()` 和 `theme.Shadcn()`。三者都接受通用选项 `theme.Dark()`（深色）、`theme.WithPrimary(...)`（主色）和 `theme.Compact()`（紧凑尺寸）：

```go
t := theme.AntDesign(theme.Dark(), theme.WithPrimary(theme.Hex("#13c2c2")))
r, _, _ := uihost.Open(app, opts, page, ui.RunnerOptions{Theme: t}) // 显式主题：固定，不跟随系统
```

语义色由一个主色按各自设计系统的算法派生。

`theme.Shadcn()` 另外接受 `theme.ShadcnOptions`，可以和通用选项混用：

```go
theme.Shadcn()                                                  // 浅色 neutral 灰阶，圆角 10 px
theme.Shadcn(theme.Dark())                                      // 深色
theme.Shadcn(theme.ShadcnOptions{Base: "zinc", Radius: 8})      // 换基础灰阶和圆角
theme.Shadcn(theme.ShadcnOptions{Color: "blue"}, theme.Dark())  // 叠加 shadcn/ui 的颜色主题
```

| 字段 | 含义 |
|---|---|
| `Base` | 基础灰阶：`neutral`（默认）、`stone`、`zinc`、`mauve`、`olive`、`mist`、`taupe`；v4 已移除的 `slate`、`gray` 保留 v3 的颜色 |
| `Radius` | 对应 shadcn 的 `--radius`，单位 px，默认 10；卡片用 Radius+4，对话框用 Radius，按钮和输入框用 Radius−2 |
| `Color` | 叠加 shadcn/ui v4 的颜色主题（`blue`、`amber` 等，见 `theme.ShadcnColors`），影响主色、次要色、图表色和侧边栏主色 |
| `Primary` | 用品牌色替换默认的近黑（深色模式下近白）主色，焦点环随之变化；同时设置时优先于 `Color` |
| `Dark` / `Compact` | 与 `theme.Dark()` / `theme.Compact()` 相同 |

shadcn 主题的颜色与 shadcn/ui 公布的 CSS 变量一致，`t.Shadcn` 保留这些原始值，`t.CSSVars()` 按同名变量导出。

### 跟随系统深浅色与语言

`uihost.Open` 默认（`Options` 零值、不传 `RunnerOptions.Theme`）让窗口跟随系统：按系统深浅色生成主题，用户切换外观时自动换成对应主题；组件内置文案（确定、取消、分页……）跟随 `i18n.Locale()`，语言变化时重绘。

```go
r, _, err := uihost.Open(app, uihost.Options{
	Surface:          tango.SurfaceOptions{Title: "Demo", Width: 960, Height: 640},
	Theme:            uihost.ThemeSystem, // 默认；ThemeLight / ThemeDark 固定深浅；ThemeFixed 用 RunnerOptions.Theme
	AccentFromSystem: true,               // 系统强调色作为主色
	Flavor: func(dark bool, accent *theme.Color) *theme.Theme { // 可选，默认 Ant Design
		opts := []theme.Option{}
		if dark {
			opts = append(opts, theme.Dark())
		}
		if accent != nil {
			opts = append(opts, theme.WithPrimary(*accent))
		}
		return theme.ElementPlus(opts...)
	},
	Customize: func(t *theme.Theme) { t.Radius = 8 }, // 可选，每次生成后改令牌
	Locale:    uihost.LocaleSystem,                    // 默认；LocaleFixed 不改 ui 的默认语言
}, page, ui.RunnerOptions{})
```

| 模式 | 行为 |
|---|---|
| `ThemeSystem`（默认） | 跟随系统深浅色（开了 `AccentFromSystem` 也跟随强调色） |
| `ThemeLight` / `ThemeDark` | 固定浅色 / 深色，仍使用 `Flavor`、`Customize`、`AccentFromSystem` |
| `ThemeFixed` | 完全使用 `RunnerOptions.Theme`，不跟随；`Theme` 为零值但传了 `RunnerOptions.Theme` 时也按此处理 |
| `LocaleSystem`（默认） | 打开窗口和 `i18n.Locale()` 变化时调用 `ui.SetDefaultLocale`，没有对应语言包时用英文 |
| `LocaleFixed` | 不改 ui 的默认语言 |

运行中手动换主题时，先关掉跟随，否则下一次系统切换会被视为冲突：

```go
app.Post(func() { // FollowSystemTheme 在主线程调用
	uihost.FollowSystemTheme(r, false)
	r.Post(func() { r.SetTheme(theme.ElementPlus()) }) // SetTheme 在窗口的 UI 线程调用
})
// 回到跟随系统（立即应用当前外观）
app.Post(func() { uihost.FollowSystemTheme(r, true) })
```

忘了关也不会被覆盖：系统切换时 uihost 发现窗口的主题已不是自己上次设置的（应用调用了 `SetTheme`，或带主题设置的 `ConfigProvider` 替换了主题），就停止跟随该窗口。同理，应用自己调用了 `ui.SetDefaultLocale` 后不再跟随语言。`uihost.FollowingSystemTheme(r)` 查询当前状态。同一应用的所有窗口共享一份系统订阅，窗口全部关闭后取消。`uihost.Attach` 的叠层同样默认跟随，需要定制时用 `uihost.AttachWith(s, layer, opts, page, ropts)`。`tan dev` 热加载下行为一致。

### 与 WebView 页面共享令牌

`sys/theme/uivars` 把自绘 UI 的设计令牌（`(*theme.Theme).CSSVars()` 的全部输出）以 `:root { --tango-…: …; }` 注入每个 WebView 页面，并随系统深浅色更新。传入窗口的 `ThemeFor`，页面和自绘窗口就用同一套令牌：

```go
import "github.com/tangoui/tango/sys/theme/uivars"

opts := uihost.Options{AccentFromSystem: true}
_ = uivars.SyncPage(app, opts.ThemeFor) // 在创建 Surface 之前调用；nil 用默认 Ant Design
app.OnReady(func() { uihost.Open(app, opts, page, ui.RunnerOptions{}) })
```

```css
.card {
  background: var(--tango-color-bg-container);
  color: var(--tango-color-text);
  border-radius: var(--tango-radius);
}
```

变量写在 `<style id="tango-ui-vars">` 里并放在文档最前，页面自己的样式可以覆盖。生成函数依赖应用自身状态（例如手动换了主题）时，状态变化后调用 `uivars.Refresh(app)`。页面脚本可用 `@tangoui/runtime` 的 `Theme.vars()` / `Theme.onVarsChange(fn)` 读取，详见 [theme](/guide/system/theme#与自绘-ui-共享令牌)。

## 与 WebView 同窗

桌面和移动端都可用 `uihost.Attach` 把自绘层放在 WebView 上方或下方。移动端叠放层由 CPU 渲染。

```go
r, att, err := uihost.Attach(webSurface, uihost.Layer{
    Bounds:      tango.Rect{},  // 零值：铺满并随窗口缩放
    Above:       true,          // 在 WebView 上方；false 在下方（页面需透明背景）
    PassThrough: true,          // 透明像素处的鼠标事件交给下面的 WebView
}, overlayPage, ui.RunnerOptions{})
att.SetLayer(...)  // 移动 / 调整层
att.Detach()
```

适合在 Web 界面上叠一个高频刷新的自绘部件（图表、编辑器、画布），或反过来在自绘界面里嵌一块 Web 内容。示例：`examples/webview-overlay`。

## 后台数据与线程

自绘界面和后端在同一个进程里：直接调用 Go 函数、读 Go 结构体就行，不需要序列化，也不需要 IPC。要注意的只有线程。

**线程规则**

- 界面状态只在 UI 线程上读写。`uihost.Open`、`uihost.Attach` 和 `tan dev` 的 UI 线程是应用主线程。macOS 的独立 UI 线程目前只用于内部验证。
- 耗时或会阻塞的操作（文件、网络、子进程、`ipc` 的流）放到 goroutine 里做，不要在页面函数里做，否则整个窗口卡住。
- 在后台取得结果后，用 `runner.Post(func() { rows = res })` 交回 UI 线程更新状态。`Post` 可以从任意 goroutine 调用，在 UI 线程按顺序执行任务，之后请求一帧。`App.Post` 将任务投递到主线程，适用于 `uihost.Open`、`uihost.FollowSystemTheme` 等需要主线程的调用。`Invalidate` 可以从任意 goroutine 调用。
- 快捷键、菜单、托盘、`App.OnMessage` 和 `Surface.OnClose` 等主线程回调也可以用 `Runner.Post` 更新页面状态。开发构建（`-tags tango_dev`）的线程检查只作用于实际绑定了独立 UI 线程的 `Runner`。分阶段安排见 [迁移说明](https://github.com/tangoui/tango/blob/main/docs/release.md#迁移说明)。

**流式数据**用 Go 原生的手段：channel、`io.Reader`/`io.Writer`、回调。阻塞读写本身就是背压：goroutine 读得慢，生产者就会被挡住。比如一个终端面板：goroutine 读 shell 的输出，交回 UI 线程追加到缓冲区并 `Invalidate`；按键在事件处理里写给 shell。写本地管道一般很快，但它也可能阻塞，写入可能等待的目标（比如下面的 `ipc` 流，要等信用）应当经 channel 交给一个专门写的 goroutine。

**高频数据要合并。**每来一块数据就 `Post` 一次，数据比帧来得快时任务会在 UI 线程上越堆越多。`examples/duplex/panel.go` 的做法：goroutine 把数据追加到加锁的缓冲区，只在"还没有待处理的刷新"时才唤醒 UI 线程一次；页面函数一次取走全部数据，并清掉标记。

```go
// push buffers item. It may be called on any goroutine and never blocks
// on the UI thread.
func (c *coalescer) push(item string) {
	c.mu.Lock()
	c.items = append(c.items, item)
	first := !c.pending
	c.pending = true
	c.mu.Unlock()
	if first {
		c.wake() // outside the lock: wake may even call drain
	}
}

// drain takes every buffered item and clears the pending flag, so the
// next push wakes the UI thread again. Call it on the UI thread.
func (c *coalescer) drain() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	items := c.items
	c.items, c.pending = nil, false
	return items
}
```

唤醒函数让 UI 线程出一帧，页面函数在那一帧里 `drain`，状态（`p.recent`）只在 UI 线程上改：

```go
p.echoes = newCoalescer(func() { app.Post(runner.Invalidate) })
```

```go
func (p *echoPanel) page(c *ui.Context) {
	p.recent = append(p.recent, p.echoes.drain()...)
	if n := len(p.recent); n > recentEchoes {
		p.recent = p.recent[n-recentEchoes:]
	}
	// ……画出 p.recent
}
```

**复用 WebView 的服务。**已经用 `Handle`、`HandleStream`、`HandleDuplex` 注册给页面的服务，Go 界面用 `r.Call(ctx, method, args...)` 和 `r.Open(ctx, method, args...)` 在进程内直接调用，语义与页面完全一样，但不经过 JSON 消息和 base64（见 [IPC：进程内调用](/guide/concepts/ipc#进程内调用)）。混合界面里页面和 Go 面板因此共用同一个服务实例。这些调用都会阻塞，按上面的线程规则放进 goroutine。`examples/duplex -panel` 的 Go 面板就这样接页面也在用的 `Echo` 服务：

```go
// run sends {"t": time} on an Echo stream every second and pushes each
// echo to the panel. Open, Write and Receive block, so this runs on its
// own goroutine, never on the UI thread.
func (p *echoPanel) run(ctx context.Context, r *ipc.Router) {
	s, err := r.Open(ctx, "Echo")
	if err != nil {
		p.echoes.push("Open: " + err.Error())
		return
	}
	defer s.Close()

	go func() {
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-tick.C:
				if s.Write(map[string]string{"t": now.Format("15:04:05")}) != nil {
					return
				}
			}
		}
	}()

	for {
		var m struct {
			T string `json:"t"`
		}
		if err := s.ReceiveJSON(&m); err != nil {
			p.echoes.push("closed: " + err.Error())
			return
		}
		p.echoes.push("← " + m.T)
	}
}
```

`r.Open` 打开的流不属于任何页面，页面关闭时 `Drop` 不会结束它：用完要 `Close`（上面的 `defer s.Close()`）或取消 `ctx`（示例在窗口关闭时取消），否则服务端处理函数的 goroutine 不会退出。服务端想把 shell 这类字节流接到流上，用 `s.ReadWriter()` 配合 `io.Copy`，见 `examples/duplex/term.go`。以上代码都摘自 `examples/duplex`，在那里能编译、有测试。

## 用 Driver 测试

`ui.NewDriver` 无窗口地运行页面，时间是模拟的、默认关闭动画，结果确定：

```go
func TestCounter(t *testing.T) {
    d := ui.NewDriver(page, ui.DriverOptions{})
    d.Frame()
    if err := d.ClickLabel("点我"); err != nil {
        t.Fatal(err)
    }
    d.Frame()
    if _, ok := d.FindText("点了 1 次"); !ok {
        t.Fatal("计数没有更新")
    }
}
```

还有 `Click`、`Drag`、`Move`、`Type`、`Compose`（输入法组合）、`Key`、`Scroll`、`Advance` + `Settle`（推进模拟时间）。组件库自己的黄金图测试在 `ui/golden_test.go`（`go test ./ui -run Golden -update` 生成，与字体列表绑定，字体不同的机器自动跳过）。

## 热加载

`tan dev` 检测到应用使用 `uihost` 时，默认对自绘窗口做热加载：

```
tan dev
 ├─ 宿主进程（开发期间常驻）：窗口、输入、光标、输入法、剪贴板
 └─ 工作进程（你的应用，每次编译后替换）：ui.Runner + 页面代码，渲染帧
```

改完 Go 代码保存，窗口不关闭，新进程首帧就绪后直接替换画面；编译失败时保留旧进程，并在窗口角落提示错误。需要跨重载保留的状态用：

```go
uihost.Persist(&state) // 可 gob 编码的值；类型变了就丢弃旧状态并提示
```

WebView 窗口仍按重启处理（前端用 Vite HMR）。`tan dev -hot=false` 或 `TANGO_HOTRELOAD=0` 关闭热加载。宿主代码只在 `-tags tango_dev` 构建里（`tan dev` 自动加），默认构建与 `tan build` 不包含。示例：`examples/ui-hotreload`。

自动化测试可通过 `automation.Launch` 或 `tan mcp` 启动独立的开发构建，用语义树查找节点、执行动作或截图。自绘热加载窗口由宿主进程绘制，控制通道无法读取它的语义树。 [整机自动化](https://github.com/tangoui/tango/blob/main/docs/automation.md)

## 相关

- [组件文档](/components/)
- [多语言](/guide/i18n)：组件内置文案（日期选择、分页等）的语言
- API：[ui](/reference/api/ui)、[uihost](/reference/api/uihost)、[ui/theme](/reference/api/ui-theme)
