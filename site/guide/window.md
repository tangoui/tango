# 窗口与外观

桌面上每个 [Surface](/guide/concepts/app-surface) 是一个原生窗口。本页覆盖创建参数、几何与状态、外观、拖放和 WebView 的常用控制。窗口方法都可以从任意 goroutine 调用；页面侧对应的是 `@tangoui/runtime` 的 `Window` 模块。

::: tip 平台状态
下面的功能三个桌面后端都已实现，但只在 macOS 上实机验证过；Windows、Linux 尚未实机验证。平台忽略自己做不到的选项。
:::

## 创建参数

```go
s, err := app.NewSurface(tango.SurfaceOptions{
    Title:  "Editor",
    Width:  1100, Height: 720,          // 内容区大小，逻辑像素；0 用后端默认
    MinWidth: 640, MinHeight: 400,
    MaxWidth: 0, MaxHeight: 0,          // 0 不限
    Resizable: true,
    Hidden:    true,                     // 先不显示，准备好再 Show()
    Centered:  true,                     // 或用 X, Y 指定外框左上角
    State:     tango.StateNormal,        // 初始状态：Normal / Maximized / Fullscreen / Minimized
    Kind:      tango.KindWindow,         // KindPanel、KindModal、KindSheet（后两者要 Parent）
    Parent:    nil,
    Appearance: tango.Appearance{TitleBar: tango.TitleBarHidden},
    AcceptFiles: true,                   // 拖入的文件交给 OnFilesDropped，而不是让页面打开
    URL:   "/", Debug: true,
})
```

`KindPanel` 是浮动工具面板（不激活应用）；`KindSheet` 在 macOS 上是附着在父窗口上的表单，其他平台按 `KindModal` 处理。

## 几何与状态

```go
s.SetSize(800, 600)
w, h := s.Size()
s.SetSizeLimits(640, 400, 0, 0)
s.SetBounds(tango.Rect{X: 100, Y: 100, Width: 800, Height: 600})
s.Center()
s.Maximize(); s.Minimize(); s.Fullscreen(); s.Restore()
st := s.State()

s.OnResized(func(w, h int) { ... })
s.OnMoved(func(x, y int) { ... })
s.OnStateChanged(func(st tango.WindowState) { ... })
s.OnFocused(func() { ... }); s.OnBlurred(func() { ... })
```

**坐标口径**：`Rect` 的 X/Y 是窗口外框（含标题栏和边框）左上角，原点是主屏左上角、Y 向下，单位是逻辑像素；Width/Height 是内容区大小。`screen.All` 返回的显示器坐标、托盘图标的 `Bounds` 用同一坐标系。

### 记住窗口位置

```go
s, _ := app.NewSurface(tango.SurfaceOptions{Hidden: true, Resizable: true})
windowstate.Track(s, "main", nil) // 默认存到 <UserConfigDir>/<App.ID>/windows.json
s.Show()
```

在显示前调用，避免窗口跳动；保存的位置已不在任何显示器上时（拔了显示器）会改为居中。见 [windowstate](/guide/system/windowstate)。

## 外观

外观用一个 `Appearance` 结构体整体设置，零值就是普通窗口：

```go
s.SetAppearance(tango.Appearance{
    TitleBar:    tango.TitleBarHidden,   // Default / Hidden（内容延伸到标题栏下，保留系统按钮）/ None（无边框）
    Transparent: false,                  // 窗口与 WebView 背景透明
    Background:  0x1e1e1eff,             // 0xRRGGBBAA，非透明时的背景色
    Material:    tango.MaterialBlur,     // macOS 毛玻璃 / Windows Acrylic；MaterialMica：Windows 11 Mica
    AlwaysOnTop: true,
    Opacity:     0.95,
    NoMinimize:  true,                   // 还有 NoClose、NoMaximize、NoShadow（无边框时）
})
ap := s.Appearance()
```

### 无边框窗口的拖动区域

`TitleBarNone` / `TitleBarHidden` 时，在页面里给可拖动的元素加 CSS：

```css
.titlebar { --tango-drag: drag; }      /* 也识别 app-region: drag */
.titlebar button { --tango-drag: no-drag; }
```

核心注入的脚本会在这些元素上按下鼠标时请后端开始原生移动；后端支持时，无边框窗口的边缘也能缩放。

## 文件拖放

- **拖入**：`AcceptFiles: true` 后，`s.OnFilesDropped(func(paths []string, x, y int))` 收到绝对路径；页面里是 `Window.onFilesDropped`。
- **拖出**：`s.DragFiles(paths, iconPNG, done)` 从窗口把文件拖到 Finder / 资源管理器。必须在鼠标按下期间调用（页面的 `mousedown` / `dragstart` 转给 Go），页面里记得 `event.preventDefault()`。

## 页面控制

| 方法 | 说明 |
|---|---|
| `Navigate(url)` / `Reload()` | 导航（相对路径按资源 origin 解析）/ 重新加载 |
| `Eval(js)` / `Send(msg)` | 执行脚本 / 发原始消息 |
| `SetZoom(f)` / `Zoom()` | 页面缩放 |
| `Print()` / `PrintToPDF(opts)` | 打印 / 导出 PDF |
| `Screenshot()` | 页面截图（PNG） |
| `Find(text, opts)` / `FindNext()` / `StopFind()` | 页内查找 |
| `OpenDevTools()` | 打开检查器（需要 `Debug`） |
| `SetUserAgent` / `SetExtraHeaders` | 自定义 UA 与请求头 |
| `Cookies` / `SetCookie` / `DeleteCookie` / `ClearBrowsingData` | Cookie 与网站数据 |
| `OnLoaded` / `OnNavigating` / `OnLoadFailed` | 加载事件 |

下载、HTTP 认证、新窗口请求、权限请求和导航策略由处理函数决定，见 [安全模型](/guide/concepts/security#_4-webview-策略) 与 [API 参考](/reference/api/tango)。

## 关闭

```go
s.OnClose(func() bool { return !dirty })  // 用户点关闭；任一返回 false 就不关
s.OnClosed(func() { r.Drop(s) })          // 每个 Surface 恰好一次，在原生窗口销毁后
s.Close()                                  // 代码关闭：不询问 OnClose
```

## 在页面里控制窗口

```js
import { Window } from "@tangoui/runtime";
await Window.setTitle("未命名");
await Window.setSize(900, 600);
const off = Window.onResize(({ width, height }) => layout(width, height));
```

`Window` 能力默认开启，只作用于调用方自己的窗口。完整列表见 [前端运行时 API](/reference/js-api)。
