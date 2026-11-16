---
title: Layout 布局
---
# Layout 布局

搭建页面的整体框架：`Layout` 是容器，里面放 `Header`（顶栏）、`Sider`（侧栏）、`Content`（内容区）、`Footer`（页脚），也可以再嵌套 `Layout`。

## 何时使用

- 应用的外框：上中下结构、左侧导航 + 右侧内容，或顶栏下面再分左右两栏。
- 侧栏需要折叠、并在窄窗口自动收起时，用 `CollapsibleSider`。
- 标准的后台管理外壳（Logo、菜单、折叠按钮、面包屑、用户信息、页脚）直接用 [ProLayout 高级布局](/components/prolayout)，不必自己拼装。
- 只是排列几块内容、做栅格，用 Row / Column、Flex、Grid 或 Space，不需要 Layout。

## 在线体验

下面是官方 gallery 中的布局场景：各设计语言的默认样式、常见的三种页面结构、可折叠侧栏与断点，以及 ProLayout 的顶部 / 混合布局。

<GalleryPreview component="Layout" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func page(c *ui.Context) {
    ui.Layout(c, func() {
        ui.Header(c, func() {
            ui.Text(c, "Header").Color(c.Theme.TextOn(c.Theme.SiderBg, c.Theme.Text))
        })
        ui.Content(c, func() {
            ui.Text(c, "Content")
        }, layout.Pad(24))
        ui.Footer(c, func() {
            ui.Text(c, "TanGo ©2026")
        })
    })
}
```

排列方向由内容决定：`Layout` 的直接子节点里有 `Sider` 时横向排列，否则纵向排列。`Layout` 自身占满父容器的剩余空间，其中的 `Content` 和嵌套的 `Layout` 也会自动伸展填满剩余空间，所以 Header、Footer 只占自身高度，Content 撑满中间。

## 常见用法

### 顶栏 + 侧栏 + 内容

在外层纵向 Layout 里嵌套一个含 `Sider` 的 Layout，就得到“上 - 左右 - 下”的经典结构。`Sider(c, width, collapsed, fn)` 是固定宽度的侧栏，`collapsed` 为 true 时收到主题的折叠宽度（带动画）。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var sel = "home"

var items = []ui.MenuItem{{Key: "home", Label: "首页"}, {Key: "docs", Label: "文档"}}

func page(c *ui.Context) {
    ui.Layout(c, func() {
        ui.Header(c, func() { ui.Text(c, "TanGo") })
        ui.Layout(c, func() {
            ui.Sider(c, 200, false, func() {
                ui.Menu(c, items, &sel).Dark().Show()
            })
            ui.Content(c, func() {
                ui.Text(c, "当前："+sel)
            }, layout.Pad(24))
        })
        ui.Footer(c, func() { ui.Text(c, "Footer") })
    })
}
```

把 Sider 放在外层、Header / Content / Footer 放进内层 Layout，就是“侧栏通高”的另一种结构（见在线体验中的第三个示例）。

### 可折叠侧栏

`CollapsibleSider(c, width, &collapsed)` 把折叠状态放在调用方的 `bool` 里，底部自带 48px 的折叠触发条，点击切换。侧栏里的 Menu 用 `Collapsed(collapsed)` 跟随，变成图标模式。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
    "github.com/tangoui/tango/ui/layout"
)

var (
    collapsed bool
    sel       = "user"
)

var items = []ui.MenuItem{
    {Key: "user", Label: "用户", Icon: icon.User},
    {Key: "video", Label: "视频", Icon: icon.Folder},
    {Key: "upload", Label: "上传", Icon: icon.File},
}

func page(c *ui.Context) {
    ui.Layout(c, func() {
        ui.CollapsibleSider(c, 200, &collapsed).Body(func() {
            ui.Menu(c, items, &sel).Dark().Collapsed(collapsed).Show()
        })
        ui.Content(c, func() { ui.Text(c, "Content") }, layout.Pad(24))
    })
}
```

- `Light()`：亮色侧栏（Ant `theme="light"`），右侧带分割线。Element Plus 和 shadcn 风格下侧栏本来就是亮色。
- `CollapsedWidth(px)`：折叠后的宽度；设为 0 时侧栏完全隐藏，此时不显示底部触发条，需要自己提供展开按钮。
- `ReverseArrow()`：侧栏位于内容右侧（或 RTL 下位于左侧）时翻转触发条箭头。
- `Changed()` 报告本帧是否由触发条或断点改变了折叠状态。

### 响应式折叠

`Breakpoint(bp)` 让侧栏在窗口宽度小于断点时自动折叠，宽度回到断点以上时自动展开（Ant 的 `breakpoint`）；`Broken()` 报告当前是否处于断点以下。断点依次为 `ui.XS`、`ui.SM`、`ui.MD`、`ui.LG`、`ui.XL`、`ui.XXL`。

如果想把折叠按钮放在顶栏，用 `NoTrigger()` 隐藏底部触发条，再在 Header 里放按钮切换同一个变量：

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
    "github.com/tangoui/tango/ui/layout"
)

var (
    collapsed bool
    sel       = "home"
)

var items = []ui.MenuItem{
    {Key: "home", Label: "首页", Icon: icon.Home},
    {Key: "setting", Label: "设置", Icon: icon.Setting},
}

func page(c *ui.Context) {
    ui.Layout(c, func() {
        sider := ui.CollapsibleSider(c, 220, &collapsed).Breakpoint(ui.LG).NoTrigger()
        sider.Body(func() {
            ui.Menu(c, items, &sel).Dark().Collapsed(collapsed).Show()
        })
        ui.Layout(c, func() {
            ui.Header(c, func() {
                ic := icon.MenuFold
                if collapsed {
                    ic = icon.MenuUnfold
                }
                if ui.Button(c, "").Text().Icon(ic).A11yLabel("切换侧栏").Clicked() {
                    collapsed = !collapsed
                }
            }, layout.Pad(0))
            ui.Content(c, func() {
                if sider.Broken() {
                    ui.Text(c, "窄窗口：侧栏已自动折叠")
                }
            }, layout.Pad(24))
        })
    })
}
```

除了侧栏，页面其他部分也可以按窗口宽度调整：`ui.ScreenBreakpoint(c)` 返回当前窗口所在的断点，例如 `ui.ScreenBreakpoint(c) < ui.MD` 时把两栏改成单栏。

### 自定义区域样式

`Layout`、`Header`、`Content`、`Footer` 都返回通用容器 `*ui.Box`，`Sider` 返回嵌入了 `*ui.Box` 的 `*ui.SiderBoxW`，所以可以继续链式调用 `Bg`、`Border` 等方法；最后一个参数是 `layout.Option`，会覆盖默认的高度、内边距等布局属性。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func page(c *ui.Context) {
    th := c.Theme
    ui.Layout(c, func() {
        ui.Header(c, func() { ui.Text(c, "浅色顶栏") }, layout.H(56)).Bg(th.BgContainer)
        ui.Layout(c, func() {
            ui.Sider(c, 240, false, func() { ui.Text(c, "Sider") }, layout.Pad(16)).Bg(th.BgContainer)
            ui.Content(c, func() {
                ui.Column(c, func() { ui.Text(c, "卡片式内容区") }, layout.Pad(24), layout.Grow(1)).
                    Bg(th.BgContainer).Radius(th.RadiusLG)
            }, layout.Pad(24))
        })
    }, layout.H(400))
}
```

## 主题与样式

| 区域 | Ant Design | Element Plus | shadcn |
| --- | --- | --- | --- |
| Layout 背景 | `BgLayout` 灰底 | 无背景 | `bg-background` |
| Header | 64px 深色条，左右内边距 50px | 60px，无背景，左右 20px | 站点顶栏：主题 `HeaderHeight`，底部分割线 |
| Sider | 深色（`SiderBg`），可 `Light()` | 无背景，`CollapsibleSider` 为亮色 | Sidebar 外观，结束边有分割线 |
| Content | 无内边距 | 默认 20px 内边距 | 无内边距 |
| Footer | 居中，上下 24px、左右 50px | 60px 高，左右 20px | 居中 |

- 组件令牌：`theme.Components.Layout`（`theme.LayoutToken`）：`HeaderBg`、`HeaderHeight`、`HeaderPaddingInline`、`FooterBg`、`FooterPadding`、`SiderBg`、`LightSiderBg`、`SiderWidth`、`SiderCollapsedWidth`、`TriggerBg`、`TriggerHeight` 等，用 `ui.ConfigProvider(c).Components(theme.Components{Layout: ...})` 覆盖。
- `ConfigProvider.Defaults("Sider", ...)` 键：`"breakpoint"`（`ui.Breakpoint`）、`"collapsedWidth"`（数值）、`"theme"`（`"light"` / `"dark"`）。

## 注意事项

- **状态放在哪**：折叠状态是调用方的 `bool`（`CollapsibleSider` 取它的指针），放在包级变量或页面状态结构里，菜单再用同一个变量设置 `Collapsed`。断点检测的“上次是否处于断点以下”由侧栏内部保留，只在跨越断点时改写折叠状态，所以用户在宽窗口里手动折叠后不会被断点改回去。
- **方向判断只看直接子节点**：只有 `Layout` 的直接子节点是 `Sider` 时才横排；如果 Sider 外面又包了一层 Row / Column，Layout 会按纵向排列。
- **填满父容器**：`Layout` 会伸展占满父容器的剩余空间；嵌在可滚动页面或卡片里时，给外层一个确定的高度（例如 `layout.H(400)`），否则高度由内容决定。
- **无障碍**：`Sider` 暴露为导航（navigation）地标；折叠触发条是带展开状态的按钮，可用 Tab 聚焦、回车切换。自己放在 Header 里的图标按钮记得加 `A11yLabel`。
- **RTL**：在从右到左的作用域里，Sider 位于右侧，分割线与触发条箭头自动镜像。

## 示例代码

<!--@include: ./_gen/layout-examples.md-->

## API

<!--@include: ./_gen/layout-api.md-->

## 相关链接

- 源码：[ui/prolayout.go](https://github.com/tangoui/tango/blob/main/ui/prolayout.go)（`Layout`、`Header`、`Sider`、`CollapsibleSider`、`Content`、`Footer`）
- [ui Go API](/reference/api/ui)
- 相关组件：[Menu 导航菜单](/components/menu)、[ProLayout 高级布局](/components/prolayout)
