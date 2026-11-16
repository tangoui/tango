---
title: RouteContext / GridContent 路由上下文与内容容器
---
# RouteContext / GridContent 路由上下文与内容容器

读取外层 `ProLayout` 的路由状态，并用 `GridContent` 按 Fluid 或 Fixed 规则包住页面内容。

## 何时使用

- 页面内容需要知道当前外壳布局、选中菜单、窄屏或内容宽度时，用 `RouteContext`。
- ProLayout 的内容区需要统一使用流式宽度或居中定宽时，用 `GridContent`。
- 只需要页面标题、面包屑和操作区时，用 [PageHeader 页头](/components/pageheader) 或 [PageContainer 页容器](/components/pagecontainer)。
- 不在 `ProLayout` 内时，`RouteContext` 返回空的 `RouteInfo`；不要把它当作 URL 路由器使用。

## 在线体验

<GalleryPreview component="GridContent" />

## 基本用法

`RouteContext` 必须在 `ProLayout` 的内容回调中读取。菜单选中项和折叠状态由页面外部变量持有，`RouteInfo` 只描述当前帧的外壳状态。

```go
import (
    "fmt"

    "github.com/tangoui/tango/ui"
)

var routeSelected = "home"
var routeCollapsed bool

func routePage(c *ui.Context) {
    ui.ProLayout(c, ui.ProLayoutConfig{
        Title:        "TanGo Pro",
        Menu:         []ui.MenuItem{{Key: "home", Label: "首页"}, {Key: "settings", Label: "设置"}},
        Selected:     &routeSelected,
        Collapsed:    &routeCollapsed,
        Layout:       "top",
        ContentWidth: "Fixed",
    }, func() {
        rc := ui.RouteContext(c)
        ui.Text(c, fmt.Sprintf("布局=%s，选中=%s，定宽=%v", rc.Layout, rc.Selected, rc.ContentWidth == "Fixed"))
    })
}
```

## 常见用法

### 根据窄屏状态调整页面

`RouteInfo.IsMobile` 来自外层布局的断点判断，适合在同一页面中切换信息密度或操作区排列，而不是重新创建一套路由。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var mobileSelected = "orders"

func mobileAwarePage(c *ui.Context) {
    ui.ProLayout(c, ui.ProLayoutConfig{
        Title:      "TanGo Pro",
        Menu:       []ui.MenuItem{{Key: "orders", Label: "订单"}},
        Selected:   &mobileSelected,
        Breakpoint: ui.MD,
    }, func() {
        rc := ui.RouteContext(c)
        if rc.IsMobile {
            ui.Column(c, func() { ui.Text(c, "移动端操作"); ui.Button(c, "更多") }, layout.Gap(8))
        } else {
            ui.Row(c, func() { ui.Text(c, "桌面端操作"); ui.Button(c, "导出") }, layout.Gap(8))
        }
    })
}
```

### 让内容跟随外层宽度

不传 `ContentWidth` 时，`GridContent` 会读取外层 `ProLayout` 的配置；直接设置 `ContentWidth("Fluid")` 会让内容填满可用区域。

```go
import "github.com/tangoui/tango/ui"

func fluidPage(c *ui.Context) {
    ui.GridContent(c).ContentWidth("Fluid").Body(func() {
        ui.ProCard(c, "流式内容").Body(func() {
            ui.Text(c, "内容区随父容器伸缩")
        })
    })
}
```

### 居中定宽内容

`Fixed()` 内容默认最大宽度为 `ui.GridContentFixedWidth`，也可以用 `MaxWidth` 覆盖；`Body` 返回内部内容盒，适合继续设置布局。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var routeNote string

func fixedPage(c *ui.Context) {
    content := ui.GridContent(c).ContentWidth("Fixed").MaxWidth(720).Body(func() {
        ui.Text(c, "居中的编辑区")
        ui.TextArea(c, &routeNote).Rows(4).Layout(layout.Grow(1))
    })
    content.Style(layout.Pad(16))
}
```

## 主题与样式

- Ant Design：`Fixed` 内容默认以 `1200px` 为最大宽度并居中，`Fluid` 填满内容区。
- Element Plus：同样保留 Fluid / Fixed 语义，页面间距由外层布局和主题令牌决定。
- shadcn：`GridContent` 只负责内容宽度，不额外绘制卡片背景；需要分组背景时在 `Body` 中使用 `ProCard` 或普通布局容器。
- `ConfigProvider.Defaults("GridContent", ui.Props{"contentWidth": "Fixed"})` 可设置默认宽度；`ProLayout` 也支持 `contentWidth`、`layout` 等默认键。

## 注意事项

- **状态**：`Selected`、`Collapsed` 必须指向包级变量或持久页面状态；`RouteContext` 本身不保存业务状态。
- **作用域**：`RouteContext` 读取最近的 `ProLayout`；在外层布局之外调用时 `InLayout` 为 `false`，其余字段为零值。
- **宽度**：`GridContent` 的 `Fixed` 是居中约束，不是横向滚动；超宽表格应在表格自身设置横向滚动。
- **无障碍**：移动端只改变布局，不应隐藏唯一的键盘操作；菜单仍由 `ProLayout` 提供导航语义。
- **RTL**：`GridContent` 的居中和最大宽度不变，内容内部的行方向由当前主题 RTL 设置处理。

## 示例代码

<!--@include: ./_gen/routecontext-examples.md-->

## API

<!--@include: ./_gen/routecontext-api.md-->

## 相关链接

- [源码 pro4_route.go](https://github.com/tangoui/tango/blob/main/ui/pro4_route.go)
- [源码 prolayout.go](https://github.com/tangoui/tango/blob/main/ui/prolayout.go)
- [ui Go API](/reference/api/ui)
- [ProLayout 高级布局](/components/prolayout)、[PageContainer 页容器](/components/pagecontainer)
