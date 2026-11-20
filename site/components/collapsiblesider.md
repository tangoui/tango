---
title: CollapsibleSider 可折叠侧栏
---
# CollapsibleSider 可折叠侧栏

带折叠状态、底部触发器和响应式断点的布局侧栏，适合导航菜单和后台应用外壳。

## 何时使用

- 左右两栏页面中需要固定宽度的导航侧栏，并允许用户收起内容时使用。
- 窗口变窄时希望侧栏自动收起、恢复宽度后自动展开时使用 `Breakpoint`。
- 只需要静态侧栏时使用 [Layout 布局](/components/layout) 的 `Sider`；需要 Logo、菜单、面包屑等完整后台外壳时使用 [ProLayout 高级布局](/components/prolayout)。
- 侧栏放在内容末端或 RTL 页面中，需要翻转触发箭头时使用 `ReverseArrow`。

## 在线体验

gallery 的 `SiderTrigger` 场景展示底部折叠触发器、`Breakpoint` 响应式收起和 `Light` 亮色侧栏；catalog 将该真实场景映射为 `CollapsibleSider`。

<GalleryPreview component="CollapsibleSider" />

## 基本用法

折叠状态由调用方持有的 bool 指针提供；侧栏中的菜单也使用同一个值设置 `Collapsed`。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
    "github.com/tangoui/tango/ui/layout"
)

var (
    siderCollapsed bool
    siderSelected  = "home"
)

var siderItems = []ui.MenuItem{
    {Key: "home", Label: "首页", Icon: icon.Home},
    {Key: "settings", Label: "设置", Icon: icon.Setting},
}

func collapsibleSiderPage(c *ui.Context) {
    ui.Layout(c, func() {
        ui.CollapsibleSider(c, 200, &siderCollapsed).Body(func() {
            ui.Menu(c, siderItems, &siderSelected).Dark().Collapsed(siderCollapsed).Show()
        })
        ui.Content(c, func() {
            ui.Text(c, "当前页面："+siderSelected)
        }, layout.Pad(24))
    })
}
```

`Body` 返回普通 `*ui.Box`。侧栏收起时宽度和底部触发条高度都由主题令牌决定。

## 常见用法

### 断点自动折叠

`Breakpoint` 接收 `ui.XS` 到 `ui.XXL` 的断点。窗口低于断点时自动折叠，恢复到断点以上时自动展开；`Broken` 可用于提示或调整内容布局。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var responsiveCollapsed bool

func siderBreakpointPage(c *ui.Context) {
    sider := ui.CollapsibleSider(c, 220, &responsiveCollapsed).Breakpoint(ui.MD)
    sider.Body(func() { ui.Text(c, "导航") })
    ui.Content(c, func() {
        if sider.Broken() {
            ui.Text(c, "窄窗口：侧栏已自动折叠").Secondary()
        } else {
            ui.Text(c, "宽窗口：侧栏可展开")
        }
    }, layout.Pad(24))
}
```

`Changed` 报告本帧是否因触发器或断点改变了折叠值，适合在需要持久化布局偏好的场景中使用。

### 顶栏自定义触发器

调用 `NoTrigger` 隐藏底部触发条，再在 Header 中切换同一个 bool。这样可以把折叠入口放在页面顶部。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
    "github.com/tangoui/tango/ui/layout"
)

var topCollapsed bool

func siderHeaderTriggerPage(c *ui.Context) {
    sider := ui.CollapsibleSider(c, 220, &topCollapsed).Breakpoint(ui.LG).NoTrigger()
    sider.Body(func() { ui.Text(c, "菜单") })
    ui.Layout(c, func() {
        ui.Header(c, func() {
            glyph := icon.MenuFold
            if topCollapsed {
                glyph = icon.MenuUnfold
            }
            if ui.Button(c, "").Text().Icon(glyph).A11yLabel("切换侧栏").Clicked() {
                topCollapsed = !topCollapsed
            }
        }, layout.Pad(0))
        ui.Content(c, func() { ui.Text(c, "页面内容") }, layout.Pad(24))
    })
}
```

### 亮色侧栏和折叠宽度

`Light` 使用亮色侧栏令牌；`CollapsedWidth` 覆盖收起后的宽度，设为 0 时侧栏完全隐藏，也不会显示内置触发条。

```go
import "github.com/tangoui/tango/ui"

var lightCollapsed bool

func siderAppearancePage(c *ui.Context) {
    ui.CollapsibleSider(c, 240, &lightCollapsed).
        Light().CollapsedWidth(56).ReverseArrow().Body(func() {
            ui.Text(c, "亮色导航")
        })
}
```

### 与 Layout 或 ProLayout 组合

`CollapsibleSider` 必须作为 `Layout` 的直接子节点才能触发横向布局；完整后台外壳可让 `ProLayoutConfig` 管理同一类侧栏和折叠状态。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

var proCollapsed bool
var proSelected = "dashboard"
var proMenu = []ui.MenuItem{{Key: "dashboard", Label: "仪表盘", Icon: icon.Dashboard}}

func siderProLayoutPage(c *ui.Context) {
    ui.ProLayout(c, ui.ProLayoutConfig{
        Title: "TanGo Admin", Menu: proMenu, Selected: &proSelected, Collapsed: &proCollapsed,
    }, func() {
        ui.Text(c, "页面内容")
    })
}
```

## 主题与样式

- Ant Design 默认是深色侧栏，`Light()` 对应 `theme="light"`；Element Plus 和 shadcn 风格通常使用亮色或页面背景色侧栏。
- 相关令牌位于 `theme.Components.Layout`（`theme.LayoutToken`），包括 `SiderWidth`、`SiderCollapsedWidth`、`SiderBg`、`LightSiderBg`、`TriggerHeight`、`TriggerBg` 和触发器文字色。
- Ant 主题的 `SiderCollapsedWidth` 默认是 64px，与 ProLayout 共用，沿用 Ant Design Pro v6 的折叠宽度。antd 6.7 的独立 `Layout.Sider` 默认是 80px。 [设计基线](https://github.com/tangoui/tango/blob/main/docs/design-baseline.md)
- `ConfigProvider.Defaults("Sider", ...)` 支持 `breakpoint`（`ui.Breakpoint`）、`collapsedWidth`（数值）和 `theme`（`"light"` / `"dark"`）。

## 注意事项

- **状态放在哪**：`collapsed` 必须由调用方持有，放在包级变量或页面状态结构里；菜单也应读取同一个 bool。不要在页面函数中每帧声明并重置它。
- **断点语义**。启用 `Breakpoint` 时，组件在首次构建和窗口跨越断点时同步折叠状态。在两次跨越断点之间，用户可以手动展开或折叠。
- **布局方向**：`Layout` 只有在直接子节点中看到侧栏时才横向排列；外面再包 Row 或 Column 可能改变这一判断。
- **无障碍**：侧栏暴露导航地标；触发条是带展开状态的按钮，可用 Tab 聚焦并用 Enter 或空格切换。自定义图标触发器必须提供 `A11yLabel`。
- **RTL**：侧栏和触发箭头会镜像；需要位于内容末端时用 `ReverseArrow` 校正箭头方向。

## 示例代码

<!--@include: ./_gen/collapsiblesider-examples.md-->

## API

<!--@include: ./_gen/collapsiblesider-api.md-->

## 相关链接

- 源码：[ui/prolayout.go](https://github.com/tangoui/tango/blob/main/ui/prolayout.go)
- [ui Go API](/reference/api/ui)
- 相关组件：[Layout 布局](/components/layout)、[Menu 导航菜单](/components/menu)、[ProLayout 高级布局](/components/prolayout)
