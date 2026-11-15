---
title: Breadcrumb 面包屑
---
# Breadcrumb 面包屑

显示当前页面在系统层级结构中的位置，并能向上返回。

## 何时使用

- 系统有两层以上的层级结构，需要告诉用户“我在哪里”，并能一步回到上级页面。
- 需要在同级页面之间快速切换时，给某一级加下拉菜单。
- 后台外壳用 [ProLayout](/components/prolayout) 时，打开 `AutoBreadcrumb` 即可按菜单路径自动生成面包屑，不必手写；页面级的“标题 + 面包屑 + 返回”组合用 PageContainer 或 PageHeader。
- 只有一层的扁平页面不需要面包屑；分步流程用 [Steps 步骤条](/components/steps)。

## 在线体验

下面是官方 gallery 中的面包屑场景：纯文本路径，以及带图标、链接、下拉菜单和自定义分隔符的结构化面包屑。

<GalleryPreview component="Breadcrumb" />

## 基本用法

`ui.Breadcrumb(c, labels...)` 用一组文本直接画出面包屑，返回值的 `Clicked()` 是本帧被点击项的序号（没有点击为 -1）。最后一项表示当前页面，显示为普通文本，前面各项显示为可点击的链接。下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import "github.com/tangoui/tango/ui"

var path = []string{"首页", "应用中心", "应用列表", "某应用"}

func page(c *ui.Context) {
    if i := ui.Breadcrumb(c, path...).Clicked(); i >= 0 {
        path = path[:i+1] // 回到被点击的那一级
    }
}
```

`ui.Breadcrumb` 内部已经调用了 `Show()`，所以不能再链式设置分隔符等选项；需要这些选项时用下面的 `BreadcrumbItems`。

## 常见用法

### 结构化项：图标、链接与点击回调

`ui.BreadcrumbItems(c, items)` 接收 `[]ui.BreadcrumbItem`，链式设置后调用 `Show()`。每一项可以有：

- `Icon`：显示在文字前的图标；`Label` 为空时只显示图标（常用于“首页”）。
- `Href`：自定义的跳转目标，点击后通过 `ClickedHref()` 读出，由应用自己完成跳转。
- `OnClick`：点击时直接执行的回调。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

var route = "/apps/42"

func page(c *ui.Context) {
    b := ui.BreadcrumbItems(c, []ui.BreadcrumbItem{
        {Icon: icon.Home, Href: "/"},
        {Icon: icon.User, Label: "应用中心", Href: "/apps"},
        {Label: "应用列表", OnClick: func() { route = "/apps" }},
        {Label: "某应用"},
    }).Show()
    if href := b.ClickedHref(); href != "" {
        route = href
    }
    ui.Text(c, "当前路由："+route)
}
```

`Href` 与 `OnClick` 可以任选其一：前者把“去哪里”交给调用方统一处理（例如写入路由变量），后者适合一次性的动作。

### 下拉菜单

给某一项设置 `Menu []ui.MenuItem`，该项旁边出现下拉箭头，点击展开菜单，常用于在同级页面之间切换。选中的菜单 Key 由 `MenuClicked()` 返回。菜单数据与 [Menu 导航菜单](/components/menu)、Dropdown 共用 `ui.MenuItem`，支持图标、禁用、分割线。

```go
import "github.com/tangoui/tango/ui"

var app = "应用列表"

func page(c *ui.Context) {
    b := ui.BreadcrumbItems(c, []ui.BreadcrumbItem{
        {Label: "首页"},
        {Label: "应用中心", Menu: []ui.MenuItem{
            {Key: "应用列表", Label: "应用列表"},
            {Key: "应用分组", Label: "应用分组"},
            {Key: "应用设置", Label: "应用设置"},
        }},
        {Label: app},
    }).Show()
    if k := b.MenuClicked(); k != "" {
        app = k
    }
}
```

### 自定义分隔符

`Separator(s)` 把默认的 `/` 换成任意文本（Ant 与 Element 的 `separator`），`SeparatorIcon(ic)` 用图标作分隔符（Element 的 `separator-icon`）。shadcn 风格默认就是右箭头分隔符。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

var items = []ui.BreadcrumbItem{{Label: "首页"}, {Label: "活动管理"}, {Label: "活动详情"}}

func page(c *ui.Context) {
    ui.BreadcrumbItems(c, items).Separator(">").Show()
    ui.BreadcrumbItems(c, items).SeparatorIcon(icon.Right).Show()
}
```

### 由菜单路径生成

面包屑通常与侧栏菜单的层级一致。`ui.MenuPath(items, key)` 返回从根到某个菜单项的路径（父级在前），转换成面包屑项即可；点击祖先项时把它的 Key 写回选中变量。在 ProLayout 里，同样的逻辑由 `AutoBreadcrumb: true` 完成。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

var menu = []ui.MenuItem{
    {Key: "list", Label: "列表页", Icon: icon.Table, Children: []ui.MenuItem{
        {Key: "table-list", Label: "查询表格"},
        {Key: "basic-list", Label: "标准列表"},
    }},
}

var selected = "table-list"

func page(c *ui.Context) {
    var items []ui.BreadcrumbItem
    for _, it := range ui.MenuPath(menu, selected) {
        items = append(items, ui.BreadcrumbItem{Label: it.Label, Href: it.Key})
    }
    if k := ui.BreadcrumbItems(c, items).Show().ClickedHref(); k != "" && k != "list" {
        selected = k // "list" 是子菜单，没有对应页面
    }
}
```

## 主题与样式

| 方面 | Ant Design | Element Plus | shadcn |
| --- | --- | --- | --- |
| 链接项 | 浅色文字（`TextTertiary`），悬停变正文色并带浅色背景 | 粗体正文色，悬停变品牌色，无背景 | 弱化文字色，悬停变前景色 |
| 当前项 | 正文色 | 次要文字色 | 前景色 |
| 默认分隔符 | `/` | `/` | 右箭头图标 |

- 组件令牌：`theme.Components.Breadcrumb`（`theme.BreadcrumbToken`）：`ItemColor`、`LastItemColor`、`LinkHoverColor`、`LinkHoverBg`、`LinkFontWeight`、`ItemGap`、`SeparatorColor`、`SeparatorMargin`、`IconFontSize` 等。
- `ConfigProvider.Defaults("Breadcrumb", ...)` 键：`"separator"`（string），统一修改作用域内所有面包屑的分隔符。

## 注意事项

- **面包屑本身无状态**：每帧按传入的项绘制，当前路径保存在调用方（路由变量、选中的菜单 Key）。`Clicked`、`ClickedHref`、`MenuClicked` 只在点击发生的那一帧有值，读到后立即更新自己的状态。
- **最后一项是当前页**：没有 `Menu` 的最后一项显示为普通文本、不可点击；前面的每一项都显示为链接，哪怕没有设置 `Href` 或 `OnClick`（此时用 `Clicked()` 的序号判断）。
- **无障碍**：整条面包屑是带“面包屑”名称的导航地标，链接项可用 Tab 聚焦、回车触发，当前项标记为 current；分隔符对读屏软件隐藏。链接的无障碍名称取自 `Label`，只放图标（`Label` 为空）的项没有可读的名称，需要照顾读屏用户时给它也写上文字（例如“首页”）。
- **RTL**：从右到左排列；方向性的图标分隔符（默认右箭头）会镜像，`/` 这样的文本分隔符不变。

## 示例代码

<!--@include: ./_gen/breadcrumb-examples.md-->

## API

<!--@include: ./_gen/breadcrumb-api.md-->

## 相关链接

- 源码：[ui/nav.go](https://github.com/tangoui/tango/blob/main/ui/nav.go)（`Breadcrumb`、`BreadcrumbItems`、`MenuPath`）
- [ui Go API](/reference/api/ui)
- 相关组件：[Menu 导航菜单](/components/menu)、[ProLayout 高级布局](/components/prolayout)、[Steps 步骤条](/components/steps)
