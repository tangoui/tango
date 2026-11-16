---
title: Input.Search 搜索框
---
# Input.Search 搜索框

带有搜索触发按钮的单行输入框，用于提交关键字查询或筛选条件。

## 何时使用

- 需要用户输入关键字，并通过 Enter 或按钮明确提交查询时。
- 只需要编辑文本、不需要搜索触发器时，用 [Input 输入框](/components/input)。
- 需要从固定选项中选择时，用 [Select 选择器](/components/select) 或 [AutoComplete 自动完成](/components/autocomplete)。
- 需要把搜索框放进窗口标题栏或工具栏时，可直接复用同一个 `Search` 控件。

## 在线体验

<GalleryPreview component="Search" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
    "github.com/tangoui/tango/ui"
)

var query string

func page(c *ui.Context) {
    search := ui.Search(c, &query).Placeholder("请输入关键字").AllowClear()
    if search.Searched() {
        ui.Text(c, "正在查询："+query)
    }
}
```

`Search` 绑定 `*string`。`Searched()` 只在本帧由 Enter 或搜索按钮触发时返回 `true`；输入内容本身的变化用 `Changed()` 读取。

## 常见用法

### 文字搜索按钮与加载状态

`EnterButton` 把默认的图标按钮替换成主按钮，`Loading(true)` 显示加载状态并阻止重复触发。异步请求结束后，把页面状态设回 `false` 并让页面重新构建。

```go
import "github.com/tangoui/tango/ui"

var (
    searchText string
    searching  bool
)

func page(c *ui.Context) {
    w := ui.Search(c, &searchText).EnterButton("搜索").Loading(searching).Width(320)
    if w.Searched() && !searching {
        searching = true
        // 在异步请求完成后设置 searching = false，并调用 c.Invalidate()。
    }
}
```

### 监听编辑与提交

用 `Changed()` 更新本地的筛选提示，用 `Searched()` 执行代价较高的查询。两者可以在同一帧分别判断。

```go
import "github.com/tangoui/tango/ui"

var (
    filterText string
    lastQuery  string
)

func page(c *ui.Context) {
    w := ui.Search(c, &filterText).Placeholder("过滤列表")
    if w.Changed() {
        ui.Text(c, "当前输入："+filterText)
    }
    if w.Searched() {
        lastQuery = filterText
    }
    ui.Text(c, "已提交："+lastQuery)
}
```

### 状态、尺寸与宽度

`Status("error")` 或 `Status("warning")` 用于校验反馈；`Disabled(true)` 禁止编辑和提交，`Size` 与 `Width` 调整控件尺寸。

```go
import "github.com/tangoui/tango/ui"

var errorQuery string

func page(c *ui.Context) {
    ui.Search(c, &errorQuery).Status("error").Size(ui.Large).Width(360).A11yLabel("错误状态的搜索")
    ui.Search(c, &errorQuery).Disabled(true).Size(ui.Small).A11yLabel("禁用的搜索")
}
```

### 在工具栏中使用

`Layout` 接受 TanGo 的布局选项；需要把控件放进标题栏时，也可以直接在标题栏的 `Center` 或工具栏回调中创建它。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var toolbarQuery string

func page(c *ui.Context) {
    ui.Row(c, func() {
        ui.Text(c, "文档").Strong()
        ui.Search(c, &toolbarQuery).Placeholder("搜索文档").Size(ui.Small).
            Layout(layout.W(280))
    }, layout.Gap(12))
}
```

## 主题与样式

- 搜索框的输入区域沿用 `Input` 的输入框令牌；默认图标按钮使用 `Button` 的按钮令牌，文字按钮由 `EnterButton` 创建，同一页面切换 Ant Design、Element Plus 或 shadcn 时会随主题变化。
- 相关令牌是 `theme.Components.Input` 与 `theme.Components.Button`；搜索控件自身没有单独的 `SearchToken`。
- `ConfigProvider.Defaults("Input", ui.Props{...})` 可为搜索框提供输入框级默认值；搜索专属的默认键包括 `placeholder`、`allowClear` 和 `enterButton`，组件级名称是 `"Search"`。

## 注意事项

- **状态**：查询字符串放在页面函数外，或放进 `ui.State`；不要在每帧把它重新赋成初始值。加载标志和上次提交的查询也应由应用保存。
- **事件时机**：`Changed()` 是编辑事件，`Searched()` 是提交事件；`Loading(true)` 时不会报告搜索触发。
- **无障碍**：有可见表单标签时优先使用 `FormItem`；独立使用时用 `A11yLabel` 命名搜索框。Tab 聚焦后可用 Enter 提交。
- **输入焦点**：需要从外部聚焦内部编辑器时，读取 `InputID()` 后传给 `c.SetFocus`；不要依赖每帧新建的临时 ID。
- **RTL**：搜索按钮和输入区域跟随当前 `ConfigProvider` 的方向设置，避免手工交换前后边距。

## 示例代码

<!--@include: ./_gen/search-examples.md-->

## API

<!--@include: ./_gen/search-api.md-->

## 相关链接

- [源码 ui/entry_inputx.go](https://github.com/tangoui/tango/blob/main/ui/entry_inputx.go)
- [ui Go API](/reference/api/ui#Search)
- [Input 输入框](/components/input)、[AutoComplete 自动完成](/components/autocomplete)、[Form 表单](/components/form)
