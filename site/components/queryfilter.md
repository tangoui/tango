---
title: QueryFilter 查询表单
---
# QueryFilter 查询表单

用于列表页顶部的可折叠查询表单：按字段定义生成控件，收集查询条件，并在点击查询或重置后通知页面重新加载数据。

## 何时使用

- 列表页有多个查询条件，需要按可用宽度自动排成 1–4 列时。
- 查询条件超过一行，需要默认收起并由用户展开更多字段时。
- 需要与 `ProTable` 共用 `ProField` 定义、值类型和枚举时。
- 只有一两个输入项且不需要折叠时，用 [Form 表单](/components/form) 的 `Inline()` 更直接；需要胶囊式筛选时用 [LightFilter 轻量筛选](/components/lightfilter)。

## 在线体验

<GalleryPreview component="QueryFilter" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见[打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
package main

import "github.com/tangoui/tango/ui"

var (
    queryFields = []ui.ProField{
        {Key: "name", Label: "规则名称"},
        {Key: "status", Label: "状态", ValueType: ui.ValueSelect,
            ValueEnum: []ui.ProEnum{{Value: "running", Label: "运行中"}, {Value: "done", Label: "已完成"}}},
        {Key: "created", Label: "创建日期", ValueType: ui.ValueDate},
    }
    queryValues ui.ProValues
)

func page(c *ui.Context) {
    q := ui.QueryFilter(c, queryFields).Values(&queryValues).Show()
    if q.Searched() {
        reloadList(q.Params())
    }
}

func reloadList(params map[string]string) {}

func main() {}
```

`Values(&queryValues)` 把值放在页面自己的 `ProValues` 中；点击查询或重置后，`Searched()` 在当前帧返回 `true`，`Params()` 返回非空值的文本表示（日期格式为 `2006-01-02`）。不绑定 `Values` 时，QueryFilter 会自己保存值。

## 常见用法

### 查询多个值并发起请求

查询条件通常在 `Searched()` 分支中转换成请求参数。日期、数字和选择值已经由 QueryFilter 按字段类型管理，不需要自己读取控件。

```go
package main

import "github.com/tangoui/tango/ui"

var (
    filterValues = ui.ProValues{"status": "running"}
    filterFields = []ui.ProField{
        {Key: "keyword", Label: "关键词"},
        {Key: "status", Label: "状态", ValueType: ui.ValueSelect,
            ValueEnum: []ui.ProEnum{{Value: "running", Label: "运行中"}, {Value: "paused", Label: "已暂停"}}},
        {Key: "amount", Label: "金额", ValueType: ui.ValueMoney},
    }
)

func page(c *ui.Context) {
    q := ui.QueryFilter(c, filterFields).Values(&filterValues).Show()
    if q.Searched() {
        requestRules(q.Params())
    }
}

func requestRules(params map[string]string) {}

func main() {}
```

### 默认展开、标签宽度与按钮文案

`DefaultExpanded()` 适合字段较少但仍希望保留折叠能力的页面。`LabelWidth` 只影响水平查询项的标签区域；`Loading(true)` 可在请求未完成时让查询按钮显示加载状态并避免重复点击。

```go
package main

import "github.com/tangoui/tango/ui"

var (
    searching bool
    expandedFields = []ui.ProField{
        {Key: "owner", Label: "负责人"},
        {Key: "department", Label: "部门"},
        {Key: "updated", Label: "更新时间", ValueType: ui.ValueDateRange},
    }
)

func page(c *ui.Context) {
    q := ui.QueryFilter(c, expandedFields).
        LabelWidth(96).
        DefaultExpanded().
        SearchText("执行查询").
        ResetText("清空条件").
        Loading(searching).
        Show()
    if q.Searched() {
        searching = true
        startSearch(q.Params())
    }
}

func startSearch(params map[string]string) { searching = false }

func main() {}
```

### 按条件隐藏或禁用字段

`Hide` 和 `Disable` 接收当前全部值。隐藏字段不会参与布局，也不会参与查询表单的显示；禁用字段仍可保留已有值，但不能在当前条件下编辑。

```go
package main

import "github.com/tangoui/tango/ui"

var query ui.ProValues

var dependentFields = []ui.ProField{
    {Key: "kind", Label: "对象类型", ValueType: ui.ValueSelect,
        ValueEnum: []ui.ProEnum{{Value: "project", Label: "项目"}, {Value: "team", Label: "团队"}}},
    {Key: "project", Label: "项目", ValueType: ui.ValueSelect,
        ValueEnum: []ui.ProEnum{{Value: "tango", Label: "Tango"}},
        Hide: func(v ui.ProValues) bool { return v.String("kind") != "project" }},
    {Key: "archived", Label: "包含归档", ValueType: ui.ValueSwitch,
        Disable: func(v ui.ProValues) bool { return v.String("kind") == "team" }},
}

func page(c *ui.Context) {
    ui.QueryFilter(c, dependentFields).Values(&query).Show()
}

func main() {}
```

### 在 ProTable 中复用字段定义

`ProTable` 会用列定义自动创建查询表单；需要独立控制折叠、按钮或外部查询状态时，也可以先用 QueryFilter，再把 `Params()` 传给自己的列表请求。不要同时让两个查询表单绑定同一份值并各自提交。

```go
package main

import "github.com/tangoui/tango/ui"

type rule struct{ ID, Name string }

var rules = []rule{{ID: "r1", Name: "Tango"}}
var tableQuery ui.ProValues

func page(c *ui.Context) {
    fields := []ui.ProField{{Key: "name", Label: "规则名称"}}
    q := ui.QueryFilter(c, fields).Values(&tableQuery).Show()
    if q.Searched() {
        rows, total := findRules(q.Params())
        _ = rows
        _ = total
    }
}

func findRules(params map[string]string) ([]rule, int) { return rules, len(rules) }

func main() {}
```

## 主题与样式

QueryFilter 复用 `ProField` 控件的主题和尺寸。可在局部 `ConfigProvider` 中设置 `labelWidth`、`defaultCollapsed`、`searchText`、`resetText`；显式的链式方法优先。Ant Design 主题使用标签冒号和链接式展开，Element Plus 使用对应的表单边框，shadcn 主题使用更紧凑的查询工具栏间距。

## 注意事项

- 即时模式下，`queryValues` 应放在页面函数外或页面状态结构中，不能每帧重新声明，否则输入和重置状态会丢失；组件的 `State` Key 由页面结构和调用位置保持。
- QueryFilter 的 `Params()` 只返回非空值的字符串，适合拼接请求参数；需要保留类型时直接读取绑定的 `ProValues`。
- 字段很多时优先让它默认折叠；大数据量的结果集应交给 `ProTable` 的分页或服务端查询，不要在查询表单中预加载数据。
- 控件和展开按钮支持键盘操作；自定义字段渲染时仍应提供 `A11yLabel`。ConfigProvider 的 `Direction` 会影响布局方向。

## 示例代码

<!--@include: ./_gen/queryfilter-examples.md-->

## API

<!--@include: ./_gen/queryfilter-api.md-->

## 相关链接

- [QueryFilter gallery 示例](https://github.com/tangoui/tango/blob/main/examples/ui-gallery/page_form_pro_steps.go)
- [ProTable 高级表格](/components/protable)
- [LightFilter 轻量筛选](/components/lightfilter)
- [源码](https://github.com/tangoui/tango/blob/main/ui/form_pro_query.go)
- [ui Go API](/reference/api/ui)
