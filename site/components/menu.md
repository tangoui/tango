---
title: Menu 导航菜单
---
# Menu 导航菜单

为页面和功能提供导航的菜单列表。支持侧栏内嵌（inline）、垂直弹出（vertical）、顶部水平（horizontal）三种模式，以及多级子菜单、分组、折叠图标模式和暗色侧栏。

## 何时使用

- 应用的一级导航：放在 Sider 里用内嵌模式，或放在 Header 里用水平模式。后台类应用可以直接用 [ProLayout](/components/prolayout)，它已经把 Menu、折叠、面包屑组装好了。
- 侧栏空间紧张时，用折叠图标模式（`Collapsed(true)`）或垂直弹出模式，子菜单向右弹出。
- 只是在同一页里切换几块内容，用 [Tabs 标签页](/components/tabs) 或 Segmented；点按钮弹出一组操作，用 Dropdown 下拉菜单（它与 Menu 共用 `ui.MenuItem`）；桌面应用顶部的“文件 / 编辑 / 视图”菜单栏用 Menubar。

## 在线体验

下面是官方 gallery 中 Menu 的全部场景：水平菜单、内嵌手风琴与暗色、垂直弹出与折叠图标模式、分组与水平溢出折叠。可切换 Ant Design、Element Plus、shadcn 风格和深浅色，并直接用键盘操作。

<GalleryPreview component="Menu" />

## 基本用法

菜单数据是一棵 `[]ui.MenuItem`，选中项绑定到一个 `*string`（菜单项的 `Key`）。点击叶子项时 Menu 会把它的 Key 写回这个变量，页面再根据它决定渲染什么。下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
    "github.com/tangoui/tango/ui/layout"
)

var navItems = []ui.MenuItem{
    {Key: "dashboard", Label: "仪表盘", Icon: icon.Dashboard, Children: []ui.MenuItem{
        {Key: "analysis", Label: "分析页"},
        {Key: "monitor", Label: "监控页"},
    }},
    {Key: "form", Label: "表单页", Icon: icon.Form},
    {Key: "list", Label: "列表页", Icon: icon.Table},
}

var selected = "analysis"

func page(c *ui.Context) {
    ui.Row(c, func() {
        ui.Menu(c, navItems, &selected).Width(220).Show()
        ui.Column(c, func() {
            ui.Text(c, "当前页面："+selected)
        }, layout.Pad(24), layout.Grow(1))
    }, layout.Grow(1))
}
```

`ui.Menu(...)` 只是开始构建，链式设置选项后必须调用 `Show()` 才会真正画出来。默认是内嵌模式，宽度填满父容器，`Width` 可以固定展开宽度。首次构建时，包含当前选中项的顶层子菜单会自动展开。

## 常见用法

### 水平菜单

`Mode(ui.MenuHorizontal)` 把菜单变成顶部导航条，子菜单以下拉方式展开。条目放不下时，末尾的条目会自动折叠进“…”子菜单，窗口变宽后再展开回来，所以不需要自己计算宽度。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

var topItems = []ui.MenuItem{
    {Key: "home", Label: "首页", Icon: icon.Home},
    {Key: "product", Label: "产品中心", Children: []ui.MenuItem{
        {Key: "cloud", Label: "云服务"},
        {Key: "sdk", Label: "开发工具包"},
    }},
    {Key: "docs", Label: "文档"},
    {Key: "about", Label: "关于我们", Disabled: true},
}

var topSel = "home"

func page(c *ui.Context) {
    ui.Menu(c, topItems, &topSel).Mode(ui.MenuHorizontal).Show()
}
```

水平菜单高度跟随设计语言：Ant Design 46px，Element Plus 60px；浅色模式下底部有一条分割线。

### 内嵌、垂直弹出与折叠

三种侧栏形态的区别只在子菜单怎么展开：

| 形态 | 写法 | 子菜单 |
| --- | --- | --- |
| 内嵌 | 默认（`ui.MenuInline`） | 在原地展开，向下推开后面的条目 |
| 垂直弹出 | `Mode(ui.MenuVertical)` | 悬停时从右侧弹出浮层 |
| 折叠图标 | `Collapsed(true)` | 只显示图标；悬停弹出子菜单，叶子项显示文字提示 |

折叠状态通常放在页面变量里，由一个按钮切换。折叠宽度为 Ant Design 80px、Element Plus 取主题的 `SiderCollapsed`，宽度变化带过渡动画。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
    "github.com/tangoui/tango/ui/layout"
)

var sideItems = []ui.MenuItem{
    {Key: "mail", Label: "邮件", Icon: icon.Mail},
    {Key: "apps", Label: "应用", Icon: icon.Appstore, Children: []ui.MenuItem{
        {Key: "app1", Label: "应用一"},
        {Key: "app2", Label: "应用二"},
    }},
    {Key: "setting", Label: "设置", Icon: icon.Setting},
}

var (
    sideSel   = "mail"
    collapsed bool
)

func page(c *ui.Context) {
    ic := icon.MenuFold
    if collapsed {
        ic = icon.MenuUnfold
    }
    if ui.Button(c, "").Icon(ic).A11yLabel("折叠菜单").Clicked() {
        collapsed = !collapsed
    }
    ui.Row(c, func() {
        ui.Menu(c, sideItems, &sideSel).Width(220).Collapsed(collapsed).Show()
        ui.Menu(c, sideItems, &sideSel).Mode(ui.MenuVertical).Width(200).Show()
    }, layout.Gap(40), layout.H(320))
}
```

两个菜单绑定同一个 `sideSel`，在任意一个里选中，另一个也会同步高亮，这就是“选中状态由调用方持有”的好处。

### 子菜单的展开状态

展开哪些子菜单属于 Menu 内部保留的状态（按菜单的位置标识保存），不需要调用方维护：

- `DefaultOpen(keys...)`：首次构建时展开指定子菜单，之后由用户操作决定。
- 首次构建时，包含当前选中项的顶层子菜单会自动展开。
- `Accordion()`：手风琴模式，同一时间只保留一个内嵌子菜单展开（Element Plus 的 `unique-opened`）。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

var accItems = []ui.MenuItem{
    {Key: "nav1", Label: "导航一", Icon: icon.Mail, Children: []ui.MenuItem{
        {Key: "1-1", Label: "选项 1"},
        {Key: "1-2", Label: "选项 2"},
    }},
    {Key: "nav2", Label: "导航二", Icon: icon.Appstore, Children: []ui.MenuItem{
        {Key: "2-1", Label: "选项 3"},
        {Key: "2-sub", Label: "三级菜单", Children: []ui.MenuItem{
            {Key: "2-sub-1", Label: "选项 4"},
        }},
    }},
    {Key: "nav3", Label: "导航三", Icon: icon.Setting, Children: []ui.MenuItem{
        {Key: "3-1", Label: "选项 5"},
    }},
}

var accSel = "1-1"

func page(c *ui.Context) {
    ui.Menu(c, accItems, &accSel).Width(256).Accordion().DefaultOpen("nav2").Show()
}
```

子菜单可以任意嵌套；内嵌模式下深层条目逐级缩进，弹出模式下深层子菜单继续向右弹出。

### 分组、分割线与危险项

`MenuItem` 上的几个标记决定条目的形态：

- `Group: true`：分组。`Label` 是不可点击的组标题，`Children` 与标题处于同一级（Ant 的 `type: "group"`）；在弹出子菜单里以带标题的分段呈现，键盘导航会跳过标题。
- `Divider: true`：渲染一条分割线代替条目。
- `Disabled: true`：禁用，不能点击也不能被键盘选中。
- `Danger: true`：危险项，文字用错误色。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

var groupItems = []ui.MenuItem{
    {Group: true, Label: "常用功能", Children: []ui.MenuItem{
        {Key: "work", Label: "工作台", Icon: icon.Dashboard},
        {Key: "data", Label: "数据分析", Icon: icon.Table},
    }},
    {Group: true, Label: "系统管理", Children: []ui.MenuItem{
        {Key: "users", Label: "用户管理", Icon: icon.User},
        {Key: "audit", Label: "审计日志", Disabled: true},
    }},
    {Divider: true},
    {Key: "logout", Label: "退出登录", Danger: true},
}

var groupSel = "work"

func page(c *ui.Context) {
    ui.Menu(c, groupItems, &groupSel).Width(240).Show()
}
```

### 暗色侧栏

`Dark()` 使用暗色侧栏配色（Ant 的 `theme="dark"`、Element 的暗色 el-menu）。Menu 本身只画条目背景，外层容器建议用主题的 `SiderBg` 铺底，让菜单下方的空白也是同一颜色。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
    "github.com/tangoui/tango/ui/layout"
)

var darkItems = []ui.MenuItem{
    {Key: "home", Label: "首页", Icon: icon.Home},
    {Key: "files", Label: "文件", Icon: icon.Folder, Children: []ui.MenuItem{
        {Key: "recent", Label: "最近打开"},
        {Key: "shared", Label: "共享给我"},
    }},
}

var darkSel = "recent"

func page(c *ui.Context) {
    ui.Column(c, func() {
        ui.Menu(c, darkItems, &darkSel).Dark().Width(240).Show()
    }, layout.H(300)).Bg(c.Theme.SiderBg)
}
```

shadcn 风格下 `Dark()` 被有意忽略：shadcn 的侧栏只有一种外观，由 `--sidebar-*` 变量随整体深浅色变化；要暗色侧栏请让整个主题进入暗色。

### 与路由配合

Tango 的自绘界面没有内置 URL 路由，“路由”就是一个保存当前页面 Key 的变量。常见做法是把菜单 Key 映射到页面函数：

- 选中变量即当前路由，`ui.Menu` 绑定它；程序内跳转（例如“返回列表”按钮）直接给这个变量赋值，菜单高亮会随之变化。
- `Clicked()` 返回本帧被点击的叶子 Key（没有则为空字符串），适合做“点击时才触发”的副作用，例如记录历史、打日志、关闭移动端抽屉。
- `ui.MenuPath(items, key)` 返回从根到该条目的路径（父级子菜单在前），可直接生成面包屑或页面标题。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
    "github.com/tangoui/tango/ui/layout"
)

var routes = []ui.MenuItem{
    {Key: "dashboard", Label: "仪表盘", Icon: icon.Dashboard, Children: []ui.MenuItem{
        {Key: "analysis", Label: "分析页"},
        {Key: "monitor", Label: "监控页"},
    }},
    {Key: "list", Label: "列表页", Icon: icon.Table},
}

var (
    route   = "analysis"
    history []string
)

var pages = map[string]func(c *ui.Context){
    "analysis": func(c *ui.Context) { ui.Text(c, "分析页内容") },
    "monitor":  func(c *ui.Context) { ui.Text(c, "监控页内容") },
    "list": func(c *ui.Context) {
        if ui.Button(c, "查看分析").Clicked() {
            route = "analysis" // 程序内跳转：改变量即可
        }
    },
}

func page(c *ui.Context) {
    ui.Row(c, func() {
        m := ui.Menu(c, routes, &route).Width(220).Show()
        if k := m.Clicked(); k != "" {
            history = append(history, k)
        }
        ui.Column(c, func() {
            var crumbs []string
            for _, it := range ui.MenuPath(routes, route) {
                crumbs = append(crumbs, it.Label)
            }
            ui.Breadcrumb(c, crumbs...)
            if render, ok := pages[route]; ok {
                render(c)
            }
        }, layout.Pad(24), layout.Gap(16), layout.Grow(1))
    }, layout.Grow(1))
}
```

如果应用确实有 URL（例如 WebView 混合应用或深链接），在启动或收到深链接时把路径解析成菜单 Key 写入 `route`，再由菜单的点击把 Key 转成路径回传即可。

## 主题与样式

| 方面 | Ant Design | Element Plus | shadcn |
| --- | --- | --- | --- |
| 内嵌条目 | 圆角（`RadiusLG`）、左右各留 4px 的选中块 | 通栏选中，无圆角、无留白 | Sidebar 风格：圆角条目、左右留 8px、16px 图标 |
| 水平菜单 | 高 46px，选中项底部 2px 下划线 | 高 60px，通栏下划线，子菜单项带下拉箭头 | NavigationMenu 风格：悬停 / 选中显示圆角底色，无下划线 |
| 暗色 | `Dark()` 生效 | `Dark()` 生效 | 忽略 `Dark()`，随主题深浅色 |
| 折叠宽度 | 80px | 主题 `SiderCollapsed` | 80px |

- 组件令牌：`theme.Components.Menu`（`theme.MenuToken`），包括条目高度 `ItemHeight`、圆角 `ItemBorderRadius`、亮色与暗色两套条目颜色（`ItemSelectedBg`、`DarkItemSelectedBg`……）以及弹出层 `PopupBg`、`PopupRadius` 等。用 `ui.ConfigProvider(c).Components(...)` 局部覆盖，零值字段沿用主题。
- `ConfigProvider.Defaults("Menu", ...)` 键：`"mode"`（`ui.MenuMode`）、`"theme"`（`"dark"` / `"light"`）、`"accordion"`、`"inlineCollapsed"`（bool）。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/theme"
)

var cfgItems = []ui.MenuItem{{Key: "a", Label: "条目 A"}, {Key: "b", Label: "条目 B"}}

var cfgSel = "a"

func page(c *ui.Context) {
    ui.ConfigProvider(c).
        Defaults("Menu", ui.Props{"theme": "dark", "accordion": true}).
        Components(theme.Components{Menu: theme.MenuToken{ItemHeight: 36}}).
        Body(func() {
            ui.Menu(c, cfgItems, &cfgSel).Width(200).Show() // 继承暗色、手风琴和 36px 行高
        })
}
```

## 注意事项

- **状态放在哪**：选中项由调用方的 `*string` 持有，放在包级变量或页面状态结构里，不要在页面函数里用局部变量（每帧都会被重置）。展开的子菜单、键盘焦点、弹出层由 Menu 内部按自身标识保留；同一位置的菜单如果会被条件分支替换，或同一列表里动态插入了兄弟控件，用 `ui.Key(c, "side-menu", func() { ... })` 包住，保持标识稳定。长时间不构建的菜单，其内部状态会被回收，再次出现时按 `DefaultOpen` 和选中项重新初始化。
- **Key 唯一**：所有层级的 `Key` 必须全局唯一，选中、展开、`MenuPath` 都按 Key 查找。分组（`Group`）和分割线不需要 Key。
- **Clicked 只报叶子**：点击带子菜单的条目只是展开/收起，不会改变选中值，`Clicked()` 也不会返回它。
- **键盘**：整个菜单只占一个 Tab 停靠点（落在聚焦项，否则选中项）。↑/↓ 在可见条目间移动；内嵌模式 → 展开子菜单（再按进入）、← 收起或回到父级；垂直与折叠模式 → 或回车打开弹出子菜单并进入；水平模式 ←/→ 沿菜单条移动，↓ 或回车打开下拉；回车/空格选中叶子，Esc 关闭弹出子菜单。
- **无障碍**：内嵌/垂直菜单暴露为 menu 角色，水平菜单为 menubar；子菜单的展开状态、禁用状态和 `Shortcut` 文本都会报告给读屏软件。折叠模式只显示图标，`Label` 仍是条目的无障碍名称，所以不要留空。
- **RTL**：在从右到左的作用域里，分割线、缩进与弹出方向自动镜像。

## 示例代码

<!--@include: ./_gen/menu-examples.md-->

## API

<!--@include: ./_gen/menu-api.md-->

## 相关链接

- 源码：[ui/nav_menu.go](https://github.com/tangoui/tango/blob/main/ui/nav_menu.go)、[ui/nav.go](https://github.com/tangoui/tango/blob/main/ui/nav.go)（`MenuItem`、`MenuPath`）
- [ui Go API](/reference/api/ui)
- 相关组件：[Layout 布局](/components/layout)、[ProLayout 高级布局](/components/prolayout)、[Breadcrumb 面包屑](/components/breadcrumb)、[Tabs 标签页](/components/tabs)
