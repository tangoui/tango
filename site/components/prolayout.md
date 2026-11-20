---
title: ProLayout 高级布局
---
# ProLayout 高级布局

一行配置搭出 Ant Design Pro 风格的后台外壳：Logo 与标题、侧栏或顶部菜单、折叠按钮、面包屑、用户信息、页面标题、内容区和页脚。对标 Ant Design Pro v6（Pro Components v3）的 ProLayout，配置项放在 `ui.ProLayoutConfig` 结构体里。

## 何时使用

- 中后台应用的整体外壳，需要 side（侧栏）、top（顶部导航）或 mix（混合）三种标准布局之一。
- 菜单、面包屑、页头页脚都按 Pro 的约定组织，想少写外壳代码、只关心页面内容。
- 外壳需要高度定制、不像后台（例如编辑器、聊天应用），用 [Layout 布局](/components/layout) + [Menu 导航菜单](/components/menu) 自己组装。
- 只需要页面内部的标题、面包屑、标签页和操作区，用 PageContainer 或 PageHeader。

## 在线体验

下面是官方 gallery 中的 ProLayout 场景：顶部导航、混合布局、自动面包屑与渲染钩子，以及 shadcn 风格的仪表盘外壳。可以切换设计语言与深浅色，点击菜单和折叠按钮试试。

<GalleryPreview component="ProLayout" />

## 基本用法

`ui.ProLayout(c, cfg, content)`：`cfg` 描述外壳，`content` 是内容区的页面函数。菜单选中项绑定到 `Selected`，折叠状态绑定到 `Collapsed`，两者都是调用方持有的变量。下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

var menu = []ui.MenuItem{
    {Key: "dashboard", Label: "仪表盘", Icon: icon.Dashboard, Children: []ui.MenuItem{
        {Key: "analysis", Label: "分析页"},
        {Key: "monitor", Label: "监控页"},
    }},
    {Key: "list", Label: "列表页", Icon: icon.Table, Children: []ui.MenuItem{
        {Key: "table-list", Label: "查询表格"},
    }},
    {Key: "account", Label: "个人中心", Icon: icon.User},
}

var (
    selected  = "analysis"
    collapsed bool
)

func page(c *ui.Context) {
    ui.ProLayout(c, ui.ProLayoutConfig{
        Title:     "TanGo Admin",
        Menu:      menu,
        Selected:  &selected,
        Collapsed: &collapsed,
        UserName:  "曲丽丽",
        Footer:    "TanGo UI ©2026",
    }, func() {
        ui.Card(c, "当前页面").Body(func() { ui.Text(c, selected) })
    })
}
```

默认是 `side` 布局。亮色侧栏里放 Logo 与菜单，顶栏左侧是折叠按钮和面包屑，右侧是操作区与用户。侧栏默认固定，顶栏随内容滚动，内容末尾是页脚。`ProLayout` 应放在窗口页面的最外层，让它占满整个窗口。

## 常见用法

### side / top / mix 三种布局

`Layout` 字段选择布局，对应 Pro 的 `layout` 属性：

| 值 | 结构 | 菜单 |
| --- | --- | --- |
| `"side"`（默认） | 侧栏（Logo + 菜单）在左，顶栏只含折叠按钮、面包屑和用户 | 侧栏内嵌菜单 |
| `"top"` | 只有顶栏：Logo、水平菜单、用户 | 水平菜单，放不下的条目折叠进“…” |
| `"mix"` | 通栏顶栏（Logo、操作、用户）+ 下方侧栏 | 侧栏菜单，侧栏底部是折叠触发条 |

side 和 mix 布局可以用 `Breakpoint` 在窄窗口自动折叠侧栏（需要同时给出 `Collapsed`），`SiderWidth` 覆盖侧栏展开宽度，`NavTheme: "dark"` 切换到深色侧栏，`LightSider: true` 强制使用亮色侧栏。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

var menu = []ui.MenuItem{
    {Key: "home", Label: "首页", Icon: icon.Home},
    {Key: "form", Label: "表单页", Icon: icon.Form},
    {Key: "setting", Label: "系统设置", Icon: icon.Setting},
}

var (
    layoutMode = "mix" // "side"、"top" 或 "mix"
    selected   = "home"
    collapsed  bool
)

func page(c *ui.Context) {
    ui.ProLayout(c, ui.ProLayoutConfig{
        Title:      "TanGo Pro",
        Menu:       menu,
        Selected:   &selected,
        Collapsed:  &collapsed,
        Layout:     layoutMode,
        Breakpoint: ui.MD, // 窗口窄于 MD 时自动折叠侧栏
        SiderWidth: 208,
        LightSider: true,
        UserName:   "曲丽丽",
    }, func() {
        ui.Text(c, "布局："+layoutMode)
    })
}
```

### 菜单数据与页面切换

`Menu` 就是 [Menu 导航菜单](/components/menu) 的 `[]ui.MenuItem`，支持多级子菜单、分组（`Group`）、图标、禁用。ProLayout 没有 URL 路由，`Selected` 就是“当前路由”：用菜单 Key 查页面函数来渲染内容，程序内跳转直接给 `Selected` 指向的变量赋值。

`MenuItemRender` 在建菜单前逐项改写数据（先子后父）：改文案、换图标、按权限禁用、加快捷键提示。它返回的是数据而不是界面，因为条目由菜单自己绘制。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

var menu = []ui.MenuItem{
    {Key: "workplace", Label: "工作台", Icon: icon.Dashboard},
    {Key: "admin", Label: "管理页", Icon: icon.Lock, Children: []ui.MenuItem{
        {Key: "users", Label: "用户管理"},
        {Key: "roles", Label: "角色权限"},
    }},
}

var (
    selected = "workplace"
    isAdmin  = false
)

var pages = map[string]func(c *ui.Context){
    "workplace": func(c *ui.Context) {
        if ui.Button(c, "去用户管理").Clicked() {
            selected = "users"
        }
    },
    "users": func(c *ui.Context) { ui.Text(c, "用户列表") },
    "roles": func(c *ui.Context) { ui.Text(c, "角色列表") },
}

func page(c *ui.Context) {
    ui.ProLayout(c, ui.ProLayoutConfig{
        Title:    "TanGo Admin",
        Menu:     menu,
        Selected: &selected,
        MenuItemRender: func(it ui.MenuItem) ui.MenuItem {
            if it.Key == "roles" && !isAdmin {
                it.Disabled = true // 没有权限：保留入口但禁用
            }
            return it
        },
    }, func() {
        if render, ok := pages[selected]; ok {
            render(c)
        }
    })
}
```

### 面包屑

面包屑有三个来源，按优先级：

1. `Breadcrumb []string`：静态面包屑。
2. `AutoBreadcrumb: true`（且 `Breadcrumb` 为空）：按 `Selected` 在菜单里的路径自动生成（父级子菜单在前），点击路径上的祖先菜单项会选中它。
3. `BreadcrumbRender`：拿到上面两种方式得到的 `[]ui.BreadcrumbItem` 再加工，例如在最前面加“首页”；返回空切片则隐藏面包屑。

side 布局的面包屑在顶栏里，top 和 mix 布局显示在内容区顶部。页面自己需要同一条路径时（例如传给 PageContainer），用 `ui.MenuPath(menu, key)` 计算。

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
    {Key: "home", Label: "首页", Icon: icon.Home},
}

var selected = "table-list"

func page(c *ui.Context) {
    ui.ProLayout(c, ui.ProLayoutConfig{
        Title:          "TanGo Admin",
        Menu:           menu,
        Selected:       &selected,
        AutoBreadcrumb: true, // 列表页 / 查询表格
        BreadcrumbRender: func(items []ui.BreadcrumbItem) []ui.BreadcrumbItem {
            home := ui.BreadcrumbItem{Icon: icon.Home, OnClick: func() { selected = "home" }}
            return append([]ui.BreadcrumbItem{home}, items...)
        },
    }, func() {
        ui.Text(c, "面包屑由菜单路径自动生成")
    })
}
```

### 页头、菜单与页脚的渲染钩子

ProLayout 提供与 Pro 同名思路的渲染钩子；其中带 `def` 参数的钩子拿到“默认部分”的构建函数，可以包一层，也可以完全替换：

| 字段 | 位置 |
| --- | --- |
| `MenuHeaderRender(def, collapsed)` | Logo 与标题区域（side 布局在侧栏顶部，top / mix 在顶栏开头） |
| `MenuFooterRender(collapsed)` | 侧栏菜单下方（side 布局） |
| `HeaderRender(def)` | 整个顶栏 |
| `HeaderContentRender()` | 顶栏中部：side 布局替换面包屑，top 布局替换水平菜单，mix 布局跟在 Logo 后面 |
| `Actions()` | 顶栏右侧、头像左边的操作区 |
| `FooterRender()` | 替换内容区末尾的默认页脚 |
| `FooterLinks` + `Footer` | 默认页脚（DefaultFooter）：一行链接 + 一行版权 |

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
    "github.com/tangoui/tango/ui/layout"
)

var menu = []ui.MenuItem{
    {Key: "home", Label: "首页", Icon: icon.Home},
    {Key: "setting", Label: "设置", Icon: icon.Setting},
}

var (
    selected  = "home"
    collapsed bool
    keyword   string
)

func page(c *ui.Context) {
    ui.ProLayout(c, ui.ProLayoutConfig{
        Title:     "TanGo Admin",
        Logo:      icon.Appstore,
        Menu:      menu,
        Selected:  &selected,
        Collapsed: &collapsed,
        UserName:  "曲丽丽",
        Actions: func() {
            ui.Button(c, "").Text().Icon(icon.Question).A11yLabel("帮助")
        },
        HeaderContentRender: func() {
            ui.Input(c, &keyword).Placeholder("搜索菜单").Layout(layout.W(220))
        },
        MenuFooterRender: func(collapsed bool) {
            if !collapsed {
                ui.Row(c, func() { ui.Text(c, "版本 0.3.1").Secondary() }, layout.Pad(16))
            }
        },
        FooterLinks: []ui.FooterLink{
            {Key: "docs", Title: "文档", OnClick: func() { selected = "home" }},
            {Key: "help", Title: "帮助"},
        },
        Footer: "© 2026 TanGo UI",
    }, func() {
        ui.Text(c, "搜索："+keyword)
    })
}
```

### 内容区：PageContainer、页面标题与内容宽度

- 内容区默认有 24px 内边距；直接在内容区放 `ui.PageContainer` 时，内容区会自动去掉内边距，让页头背景横跨整个内容区。也可以用 `NoPadding: true` 手动去掉。
- `PageTitle` 在内容顶部显示一个标题。
- `top` 布局中，`ContentWidth: "Fixed"` 把内容限制在居中的 1152px 内（Pro 的 `contentWidth`）。页面内部可以用 `ui.GridContent(c)` 跟随这一设置。
- `side` 和 `top` 布局中，顶栏默认随内容滚动，`FixedHeader: true` 固定顶栏。`NoFixedHeader: true` 优先于 `FixedHeader`。`side` 和 `mix` 布局中，`NoFixSiderbar: true` 让侧栏和内容一起滚动。
- 内容里调用 `ui.RouteContext(c)` 可以读到外壳的状态：当前布局、是否折叠、是否窄屏（`IsMobile`）、面包屑、选中项等，对应 Pro 的 RouteContext。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

var menu = []ui.MenuItem{{Key: "rules", Label: "规则管理", Icon: icon.Table}}

var (
    selected = "rules"
    tab      = "all"
)

func page(c *ui.Context) {
    ui.ProLayout(c, ui.ProLayoutConfig{
        Title: "TanGo Admin", Menu: menu, Selected: &selected,
        ContentWidth: "Fixed",
    }, func() {
        rc := ui.RouteContext(c)
        ui.PageContainer(c, "规则管理").
            Breadcrumb("首页", "规则管理").
            Tabs(&tab, []ui.TabItem{{Key: "all", Label: "全部"}, {Key: "on", Label: "运行中"}}).
            Extra(func() { ui.Button(c, "新建").Primary() }).
            Body(func() {
                if rc.IsMobile {
                    ui.Text(c, "窄屏布局")
                } else {
                    ui.Text(c, "当前布局："+rc.Layout)
                }
            })
    })
}
```

### 与 SettingDrawer 配合

`ui.SettingDrawer` 是 Pro 的设置抽屉：右下角浮动按钮打开抽屉，编辑整体风格（亮色菜单 / 暗色菜单 / 暗黑模式）、主题色、布局、内容宽度、固定顶栏与侧栏、色弱与紧凑模式。三件套的分工：

- `ui.ProSettings` 保存设置，`ui.DefaultProSettings()` 给出 Pro 的默认值；
- `ui.ProSettingsProvider(c, &settings, fn)` 把暗黑模式、主题色、紧凑、色弱转成主题作用于整个窗口（必须放在根上）；
- `settings.Apply(&cfg)` 把布局、侧栏主题、内容宽度、固定顶栏 / 侧栏写进 `ProLayoutConfig`。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

var menu = []ui.MenuItem{
    {Key: "home", Label: "首页", Icon: icon.Home},
    {Key: "form", Label: "表单页", Icon: icon.Form},
}

var (
    settings  = ui.DefaultProSettings()
    selected  = "home"
    collapsed bool
)

func page(c *ui.Context) {
    ui.ProSettingsProvider(c, &settings, func() {
        cfg := ui.ProLayoutConfig{Title: "TanGo Admin", Menu: menu, Selected: &selected, Collapsed: &collapsed}
        settings.Apply(&cfg)
        ui.ProLayout(c, cfg, func() {
            ui.Text(c, "布局："+settings.Layout+"，菜单主题："+settings.NavTheme)
        })
        ui.SettingDrawer(c, &settings).Show()
    })
}
```

不想显示浮动按钮时用 `NoTrigger()`，再用 `Open(&open)` 绑定自己的开关（例如放在顶栏 `Actions` 里的设置按钮）；`Colors` 替换预设主题色，`Changed()` 报告本帧是否有设置变化（可用来持久化设置）。与 Pro 的差异：自绘界面没有 CSS 滤镜，色弱模式改用主题的高对比度变体。

## 主题与样式

- Ant Design：默认使用 Pro v3 的亮色侧栏。顶栏高度和背景来自主题的 `ProHeaderHeight` / `ProHeaderBg`。
- Element Plus：同样的结构，菜单、顶栏、侧栏使用 Element 的配色和尺寸。
- shadcn：side 布局换成 shadcn 仪表盘外观（Sidebar + 站点顶栏）；侧栏只有一种随深浅色变化的外观，`LightSider` 与深色侧栏不适用。
- 外壳使用 `theme.Components.Layout`（`ProHeaderBg`、`ProHeaderHeight`、`SiderBg`、`LightSiderBg`、`ProSiderWidth`、`ProMixSiderWidth`、`SiderCollapsedWidth`、触发条颜色等），菜单使用 `theme.Components.Menu`。
- `ConfigProvider.Defaults("ProLayout", ...)` 键：`"layout"`、`"title"`、`"footer"`、`"navTheme"`（`"light"` / `"dark"`）、`"contentWidth"`（`"Fluid"` / `"Fixed"`）为字符串，`"noPadding"`、`"fixedHeader"`、`"fixSiderbar"` 为 bool，`"breakpoint"` 为 `ui.Breakpoint`。展开宽度可通过 `ProLayoutConfig.SiderWidth` 设置。`SettingDrawer` 另有 `"hideColors"`、`"hideLayout"`、`"hideOther"`（bool）隐藏对应分区。

## 注意事项

- **状态放在哪**：`Selected`、`Collapsed` 指向调用方的变量（包级变量或页面状态结构），不要指向页面函数里的局部变量。不给 `Collapsed` 时外壳不能折叠，`Breakpoint` 也不会生效。`ProSettings` 同理要放在包级变量里。
- **配置每帧传入**：`ProLayoutConfig` 是普通结构体，每帧构建一次，按条件改字段（例如切换 `Layout`）下一帧立即生效，不需要“刷新”。
- **菜单 Key 唯一**：选中、自动面包屑、`MenuPath` 都按 Key 查找。自动面包屑项的 `Href` 是对应菜单项的 Key。
- **放在根上**：ProLayout 占满整个窗口；`ProSettingsProvider` 的主题作用于整个窗口，要在页面最外层调用。
- **无障碍**：侧栏是导航地标，折叠按钮和触发条带展开状态；`Actions` 里的纯图标按钮请加 `A11yLabel`。菜单的键盘操作见 [Menu 导航菜单](/components/menu#注意事项)。

## 示例代码

<!--@include: ./_gen/prolayout-examples.md-->

## API

<!--@include: ./_gen/prolayout-api.md-->

## 相关链接

- 源码：[ui/prolayout.go](https://github.com/tangoui/tango/blob/main/ui/prolayout.go)、[ui/pro4_setting.go](https://github.com/tangoui/tango/blob/main/ui/pro4_setting.go)（SettingDrawer）、[ui/pro4_route.go](https://github.com/tangoui/tango/blob/main/ui/pro4_route.go)（RouteContext、GridContent）
- [ui Go API](/reference/api/ui)
- 相关组件：[Layout 布局](/components/layout)、[Menu 导航菜单](/components/menu)、[Breadcrumb 面包屑](/components/breadcrumb)
