---
title: Loading 加载中
---
# Loading 加载中

在区域或整个窗口上显示带遮罩的加载指示器，阻止加载期间的底层交互。

## 何时使用

- 数据请求、保存或计算期间暂时阻止一个内容区域的操作。
- 页面初始化或全屏切换期间需要覆盖整个窗口时使用 `Fullscreen`。
- 只需要一个不阻止交互的旋转图标时用 [Spin 加载指示器](/components/spin)；短暂反馈用 [Message 全局提示](/components/message)。

## 在线体验

<GalleryPreview component="Loading" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import "github.com/tangoui/tango/ui"

var busy bool

func page(c *ui.Context) {
    ui.Switch(c, &busy).A11yLabel("加载中")
    ui.Loading(c, busy).Tip("正在加载").Body(func() {
        ui.Text(c, "需要等待的数据区域")
    })
}
```

`Loading(c, on)` 的 `on` 为 `true` 时显示遮罩；`Body` 定义被覆盖的区域，加载结束时将状态设为 `false`。

## 常见用法

### 区域加载与尺寸

用 `Tip` 增加说明文字，`Size` 调整指示器大小；`Body` 中的内容在加载时会被遮罩挡住并降低可见度。

```go
import "github.com/tangoui/tango/ui"

var tableBusy bool

func page(c *ui.Context) {
    if ui.Button(c, "刷新").Loading(tableBusy).Clicked() {
        tableBusy = true
    }
    ui.Loading(c, tableBusy).
        Tip("正在刷新列表").
        Size(ui.Large).
        Body(func() {
            ui.Column(c, func() {
                ui.Text(c, "第一行数据")
                ui.Text(c, "第二行数据")
            })
        })
}
```

### 全屏加载

`Fullscreen` 把加载层放到整个窗口，适合路由切换、首次启动或全局初始化。加载完成后把状态设回 `false`，遮罩会以动画移除。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

var switching bool

func page(c *ui.Context) {
    if ui.Button(c, "切换工作区").Icon(icon.Loading).Clicked() {
        switching = true
    }
    ui.Loading(c, switching).Tip("正在切换工作区").Fullscreen()
}
```

### 自定义遮罩颜色

`Background` 覆盖默认遮罩颜色，可根据当前页面的背景使用半透明颜色。颜色应保留透明度，避免完全遮挡加载上下文。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/theme"
)

var previewBusy = true

func page(c *ui.Context) {
    ui.Loading(c, previewBusy).
        Tip("生成预览").
        Background(theme.RGBA(255, 255, 255, 0.78)).
        Body(func() {
            ui.Text(c, "预览内容")
        })
}
```

### 异步任务完成后关闭

后台任务可以通过 channel 将结果交回页面状态；页面下一帧用状态控制 `Loading`。不要在后台 goroutine 里直接修改 UI 组件。

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
)

var (
    loadingDone = make(chan bool, 1)
    saving      bool
)

func page(c *ui.Context) {
    if ui.Button(c, "保存").Clicked() && !saving {
        saving = true
        go func() {
            time.Sleep(300 * time.Millisecond) // 模拟保存
            loadingDone <- true
        }()
    }
    select {
    case <-loadingDone:
        saving = false
    default:
    }
    ui.Loading(c, saving).Tip("保存中").Body(func() {
        ui.Text(c, "编辑内容")
    })
}
```

## 主题与样式

Loading 使用当前主题的背景、遮罩和 Spin 令牌。Ant Design 风格的全屏加载使用窗口遮罩；Element Plus 的区域加载遵循其 `v-loading` 的浅色遮罩；shadcn 使用主题的 muted 背景和指示器颜色。`ConfigProvider.Defaults("Spin", ...)` 的 `size` 和 `tip` 可提供默认值，显式调用 `Size`、`Tip` 会覆盖默认值。

## 注意事项

- **状态来源**：`on` 应来自页面级状态或任务状态；即时模式中每帧重新调用 `Loading`，不要把状态藏在局部组件变量里。
- **区域边界**：`Body` 返回的容器决定遮罩范围；需要覆盖卡片、表格等内容时，把它们放在同一个 `Body` 中。
- **交互阻止**：加载遮罩会阻止下层点击和键盘交互；如果仍需允许取消操作，应把取消按钮放在遮罩外或使用 `Spin`。
- **异步线程**：后台任务不要直接修改页面字段；用 channel 交回页面函数处理结果。`Loading` 自身不启动或取消任务，只反映 `on` 值。
- **全屏调用**：`Fullscreen` 只绘制全屏层，区域内容应使用 `Body`；同一作用域中避免同时创建多个全屏加载层。

## 示例代码

<!--@include: ./_gen/loading-examples.md-->

## API

<!--@include: ./_gen/loading-api.md-->

## 相关链接

- [源码 ui/feedback.go](https://github.com/tangoui/tango/blob/main/ui/feedback.go)
- [Go API 参考](/reference/api/ui)
- [Spin 加载指示器](/components/spin)、[Skeleton 骨架屏](/components/skeleton)
- [组件总览](/components/)
