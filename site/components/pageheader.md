---
title: PageHeader 页头
---
# PageHeader 页头

在页面顶部组合返回、面包屑、标题、副标题、标签、操作区、描述内容和底部标签页。

## 何时使用

- 详情页、编辑页或工作流页面需要一组统一的标题和页面级操作时使用。
- 页面只需要标题和面包屑时，可直接使用 `PageHeader` 的基础链式 API。
- 需要完整 Pro 页面外壳、内容区滚动和底部操作栏时，使用 [PageContainer 页容器](/components/pagecontainer)。
- 只需要单独的路径导航时，使用 [Breadcrumb 面包屑](/components/breadcrumb)。

## 在线体验

<GalleryPreview component="PageHeader" />

## 基本用法

`PageHeader` 是即时构建器，链式设置后调用 `Show()`；返回按钮和标题都在当前帧构建。

```go
import "github.com/tangoui/tango/ui"

func headerPage(c *ui.Context) {
    ui.PageHeader(c, "订单详情").
        SubTitle("单号 2026-1112-0001").
        Back().
        Show()
}
```

## 常见用法

### 监听返回操作

`BackClicked()` 报告本帧返回按钮是否被点击；应用可以在回调中切换页面状态或调用自己的导航函数。

```go
import "github.com/tangoui/tango/ui"

var headerPageName = "detail"

func backHeaderPage(c *ui.Context) {
    header := ui.PageHeader(c, "详情").Back().Show()
    if header.BackClicked() {
        headerPageName = "list"
    }
    ui.Text(c, "当前页面："+headerPageName)
}
```

### 面包屑与标签

`Breadcrumb` 的参数按从前到后的顺序显示；`Tags` 在标题旁构建任意标签组件。

```go
import "github.com/tangoui/tango/ui"

func breadcrumbHeaderPage(c *ui.Context) {
    ui.PageHeader(c, "订单详情").
        Breadcrumb("首页", "订单管理", "订单详情").
        Tags(func() { ui.Tag(c, "进行中").Color("processing") }).
        Show()
}
```

### 操作区与描述内容

`Extra` 适合放页面级按钮，`Content` 适合放创建人、日期等描述信息；两个回调都在页头内部构建。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func fullHeaderPage(c *ui.Context) {
    ui.PageHeader(c, "规则详情").
        Extra(func() {
            ui.Button(c, "导出")
            ui.Button(c, "编辑").Primary()
        }).
        Content(func() {
            ui.Row(c, func() {
                ui.Text(c, "负责人：Tango")
                ui.Text(c, "更新时间：2026-11-15")
            }, layout.Gap(32))
        }).
        Bordered().Show()
}
```

### 底部标签页和面包屑点击

`Footer` 可放 `Tabs` 等底部导航；`CrumbClicked()` 返回本帧被点击的面包屑索引，没有点击时为 `-1`。

```go
import "github.com/tangoui/tango/ui"

var headerTab = "detail"

func tabsHeaderPage(c *ui.Context) {
    header := ui.PageHeader(c, "订单").
        Breadcrumb("首页", "订单").
        Footer(func() {
            ui.Tabs(c, &headerTab, []ui.TabItem{
                {Key: "detail", Label: "详情"},
                {Key: "log", Label: "操作日志"},
            }).Show()
        }).Show()
    if index := header.CrumbClicked(); index == 0 {
        headerTab = "detail"
    }
}
```

## 主题与样式

- Ant Design：默认是 ghost 页头，标题、操作区和下方内容沿用主题间距；`Bordered()` 对应非 ghost 的容器背景和底部边框。
- Element Plus：页头默认使用更紧凑的内边距；`BackText` 可覆盖返回文案。
- shadcn：颜色和边框来自当前主题，`Bordered()` 适合放在卡片或页面内容边界上。
- `ConfigProvider.Defaults("PageHeader", ui.Props{"ghost": false})` 可设置默认容器样式；需要精确对齐外层时可用 `Padding(geom.Insets{...})`。

## 注意事项

- **状态**：当前 tab、业务返回目标和操作结果放在页面外部状态；`BackClicked`、`CrumbClicked` 只报告当前帧事件。
- **标题层级**：PageHeader 的标题带有一级标题语义；页面内不要再无条件创建另一个同级主标题。
- **操作区**：主操作放在 `Extra` 的右侧并使用 `Primary()`；危险操作应明确使用 `Danger()`。
- **无障碍**：返回按钮有按钮语义；标签页使用 `Tabs` 自带键盘导航，纯图标操作仍要设置 `A11yLabel`。
- **RTL**：返回图标和内联排列会随方向镜像，面包屑索引仍按传入顺序返回。

## 示例代码

<!--@include: ./_gen/pageheader-examples.md-->

## API

<!--@include: ./_gen/pageheader-api.md-->

## 相关链接

- [源码 nav_pageheader.go](https://github.com/tangoui/tango/blob/main/ui/nav_pageheader.go)
- [ui Go API](/reference/api/ui)
- [Breadcrumb 面包屑](/components/breadcrumb)、[Tabs 标签页](/components/tabs)、[PageContainer 页容器](/components/pagecontainer)
