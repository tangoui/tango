---
title: StatusBar 状态栏
---
# StatusBar 状态栏

工作台窗口底部的状态栏（对标 VS Code / JetBrains）：左右两区放置条目，每个条目可以带图标、文字、计数徽标和进度条，也可以着状态色并附带提示。

## 何时使用

- IDE、编辑器、数据工具这类桌面工作台需要常驻的状态信息：分支、光标位置、编码、同步进度、问题数量。
- 条目需要可点击（切换分支、打开问题面板），或只读展示。
- 页面内的提示或操作结果用 [Alert 警告提示](/components/alert)；短暂反馈用 [Message 全局提示](/components/message)。

## 在线体验

<GalleryPreview component="StatusBar" />

## 基本用法

`ui.StatusBar(c)` 返回状态栏，用 `Left` / `Right` 分别构建两侧；每一侧里用 `ui.StatusBarItem` 添加条目。可点击的条目用 `Clicked()` 读取点击。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

var branchClicks int

func page(c *ui.Context) {
    ui.StatusBar(c).
        Left(func() {
            if ui.StatusBarItem(c, "main").Icon(icon.Branch).Tooltip("切换分支").Clicked() {
                branchClicks++
            }
        }).
        Right(func() {
            ui.StatusBarItem(c, "行 12，列 5")
            ui.StatusBarItem(c, "UTF-8").Static()
        })
}
```

状态栏横向撑满父列的宽度，高度由主题决定。`Left` / `Right` 是逻辑方向，在 RTL 下两侧互换；左侧放不下时会被裁剪，右侧始终完整显示。

## 常见用法

### 图标、文字、徽标与进度

一个条目按“图标、文字、进度条、徽标”的顺序排列，每一项都是可选的。`Loading(true)` 让图标旋转（没有图标时使用加载图标），表示任务进行中；`Badge(n)` 显示计数（0 隐藏，超过 99 显示 `99+`），`BadgeText` 显示任意短文本；`Progress(p)` 显示 0 到 1 的确定进度条，负数隐藏它，没有确定进度的任务用 `Loading`。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

func statusItems(c *ui.Context) {
    ui.StatusBar(c).Left(func() {
        ui.StatusBarItem(c, "").Icon(icon.Sync).Loading(true).Tooltip("正在同步")
        ui.StatusBarItem(c, "错误").Icon(icon.CloseCircle).Badge(2).Tooltip("问题")
        ui.StatusBarItem(c, "索引中").Progress(0.42).Static()
    }).Right(func() {
        ui.StatusBarItem(c, "").Icon(icon.Bell).BadgeText("新").Tooltip("通知")
    })
}
```

### 可点击与只读条目

条目默认是按钮：悬停高亮、可以用 Tab 聚焦，点击或回车 / 空格触发 `Clicked()`。`Static()` 把它变成只读展示：没有悬停效果、不进入 Tab 顺序、`Clicked()` 永远为 false（有 `Tooltip` 时悬停仍显示提示）。`Disabled(true)` 置灰并停止响应。

```go
import "github.com/tangoui/tango/ui"

var showProblems bool

func statusClicks(c *ui.Context) {
    ui.StatusBar(c).Right(func() {
        if ui.StatusBarItem(c, "问题").Badge(3).Clicked() {
            showProblems = !showProblems
        }
        ui.StatusBarItem(c, "UTF-8").Static()
        ui.StatusBarItem(c, "格式化").Disabled(true)
    })
}
```

### 状态色

整条状态栏着色用 `StatusBar.Status`，取值 `"primary"`、`"success"`、`"warning"`、`"error"`、`"info"`，空串是默认的中性色，条目文字自动换成对应的反色。典型场景是调试中（primary）或没有打开文件夹。单个条目也可以用 `StatusBarItem.Status` 着色，用来标记警告、错误或远程连接。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

var debugging = true

func statusColors(c *ui.Context) {
    bar := ui.StatusBar(c)
    if debugging {
        bar.Status("primary")
    }
    bar.Left(func() {
        ui.StatusBarItem(c, "远程：dev-box").Icon(icon.Monitor).Status("success")
        ui.StatusBarItem(c, "2 个警告").Icon(icon.Warning).Status("warning")
    })
}
```

### 提示与无障碍名称

`Tooltip` 在指针停留或获得键盘焦点时显示在条目上方，同时作为读屏软件的描述；只有图标的条目（文字为空）用 `Tooltip` 或 `A11yLabel` 命名。整条状态栏是一个工具栏，名称默认来自 `Locale.StatusBar.Label`，可以用 `A11yLabel` 替换。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

func statusNames(c *ui.Context) {
    ui.StatusBar(c).A11yLabel("编辑器状态").Right(func() {
        ui.StatusBarItem(c, "").Icon(icon.Setting).A11yLabel("打开设置")
        ui.StatusBarItem(c, "").Icon(icon.Bell).Tooltip("通知")
    })
}
```

### 放在窗口底部

状态栏是普通的列内元素，放在页面最外层列的最后，内容区用 `Grow(1)` 占满剩余高度即可。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func workbench(c *ui.Context) {
    ui.Column(c, func() {
        ui.Column(c, func() {
            ui.Text(c, "编辑区")
        }, layout.Grow(1))
        ui.StatusBar(c).Right(func() {
            ui.StatusBarItem(c, "UTF-8").Static()
        })
    }, layout.Fill())
}
```

## 主题与样式

外观由 `theme.Components.StatusBar`（`theme.StatusBarToken`）决定：状态栏的 `Height`、`Bg`、上边线 `BorderColor`、`PaddingInline`、`Gap`；条目的 `ItemPaddingInline`、`ItemGap`、`ItemRadius`、`ItemFontSize`、`ItemColor`、悬停 / 按下底色，`IconSize`；徽标的 `BadgeHeight`、`BadgeBg`、`BadgeColor`；进度条的 `ProgressWidth`、`ProgressHeight`、`ProgressTrack`、`ProgressColor`。用 `ui.ConfigProvider(c).Components(theme.Components{StatusBar: ...})` 局部覆盖，零值字段沿用主题。

- **Ant Design**：布局底色的紧凑条，条目方角，悬停时出现浅色底。
- **Element Plus**：浅填充的扁平条，悬停文字变主色。
- **shadcn**：圆角条目、条目之间留出间隙，悬停使用 accent 底色。

StatusBar 没有 `ConfigProvider.Defaults` 键。整条着状态色后，条目的悬停、徽标和进度条都会改用与文字同色的半透明样式，保证对比度。

## 注意事项

- **状态放在哪**：计数、进度、分支名等数据放在应用的包级变量或状态里，每帧把它们传给条目，状态栏本身不保存业务数据。
- **点击读取**：`Clicked()` 要在 `Left` / `Right` 回调里、条目构建出来之后读取，通常直接写在 `if` 条件里。
- **条目也能单独用**：在状态栏之外调用 `StatusBarItem` 会用同样的外观画在状态栏高度的盒子里，但没有条栏背景与边线。
- **宽度不足**：左侧裁剪，右侧保持完整，所以重要的信息（编码、光标位置）放右侧，次要的放左侧。
- **无障碍**：条目是按钮（`Static` 时是文本），徽标和进度会进入描述 / 数值，`Loading` 标记为忙碌；纯图标条目必须提供名称。

## 示例代码

<!--@include: ./_gen/statusbar-examples.md-->

## API

<!--@include: ./_gen/statusbar-api.md-->

## 相关链接

- [源码：statusbar.go](https://github.com/tangoui/tango/blob/main/ui/statusbar.go)
- [ui Go API](/reference/api/ui)
- [TitleBar 标题栏](/components/titlebar)
- [Dock 停靠布局](/components/dock)
