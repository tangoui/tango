# 自绘 UI（实验性）

除了 WebView，Tango 还可以用纯 Go 画界面：`github.com/tangoui/tango/ui` 是一个即时模式 API + 保留式场景的 UI 库，自带一套组件（按钮、表单、表格、日期选择、树、弹层……），`uihost` 把它放进原生窗口。不需要 HTML、JS，也不需要 WebView 进程。

::: warning 实验性
- 自绘窗口目前只在 **macOS** 上实机验证；Windows（GDI 位图）和 Linux（GTK/cairo）的 canvas 上屏已实现但未实机验证；移动端还在推进中。
- 默认用 CPU 渲染；GPU 渲染（`ui/gpu`，含 Go 写的着色器编译器 `ui/shader`）正在开发。
- WebView 与自绘在同一窗口叠放（`uihost.Attach`）目前只有 macOS 实现。
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
        ui.Title(c, "你好，Tango", 3)
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

页面函数每帧调用一次：组件函数创建节点、读取上一帧的交互（`Clicked()`、`Changed()`），直接改你的 Go 变量。没有事件、动画或输入时不出帧。UI 线程就是应用主线程，页面函数和 Runner 的回调都在主线程上运行。

更完整的例子：`examples/ui-window`（表单、表格、对话框、主题切换、中文输入法）、`examples/ui-gallery`（全部组件）。

## 组件库

组件按 Ant Design / Element Plus 的设计规范实现，每个组件只有一个公开入口，按通用、布局、导航、数据录入、数据展示、反馈分组。完整列表和截图见 [组件目录](/guide/components)。

编写新组件的约定（文件命名、节点与保留状态、布局、主题令牌、绘制、焦点与输入法、浮层、测试）在仓库的 [ui/AUTHORING.md](https://github.com/tangoui/tango/blob/main/ui/AUTHORING.md)。

## 主题

`ui/theme` 提供两套设计令牌：`theme.AntDesign()`（默认）和 `theme.ElementPlus()`，都接受选项：

```go
t := theme.AntDesign(theme.Dark(), theme.WithPrimary(theme.Hex("#13c2c2")))
r, _, _ := uihost.Open(app, opts, page, ui.RunnerOptions{Theme: t}) // 显式主题：固定，不跟随系统
```

语义色由一个主色按各自设计系统的算法派生，`theme.Compact()` 是紧凑尺寸。

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
app.Post(func() { // 在帧外调用
	uihost.FollowSystemTheme(r, false)
	r.SetTheme(theme.ElementPlus())
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

## 与 WebView 同窗（macOS）

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

- 界面状态只在 UI 线程上读写。页面函数运行在 UI 线程（也就是应用主线程）上。
- 耗时或会阻塞的操作（文件、网络、子进程、`ipc` 的流）放到 goroutine 里做，不要在页面函数里做，否则整个窗口卡住。
- 结果用 `app.Post` 交回 UI 线程，在那里写进状态，再调用 `Runner.Invalidate()` 请求重绘。`Invalidate` 在任何 goroutine 上调用都安全，真正的帧仍在 UI 线程上运行。

**流式数据**用 Go 原生的手段：channel、`io.Reader`/`io.Writer`、回调。阻塞读写本身就是背压：goroutine 读得慢，生产者就会被挡住。比如一个终端面板：goroutine 读 shell 的输出，交回 UI 线程追加到缓冲区并 `Invalidate`；按键在事件处理里写给 shell。写本地管道一般很快，但它也可能阻塞，写入可能等待的目标（比如下面的 `ipc` 流，要等信用）应当经 channel 交给一个专门写的 goroutine。

**高频数据要合并。**每来一块数据就 `app.Post` 一次，数据比帧来得快时任务会在 UI 线程上越堆越多。`examples/duplex/panel.go` 的做法：goroutine 把数据追加到加锁的缓冲区，只在"还没有待处理的刷新"时才唤醒 UI 线程一次；页面函数一次取走全部数据，并清掉标记。

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

## 相关

- [组件目录](/guide/components)
- [多语言](/guide/i18n)：组件内置文案（日期选择、分页等）的语言
- API：[ui](/reference/api/ui)、[uihost](/reference/api/uihost)、[ui/theme](/reference/api/ui-theme)
