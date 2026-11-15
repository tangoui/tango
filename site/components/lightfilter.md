---
title: LightFilter 轻量筛选
---
# LightFilter 轻量筛选

把查询条件压缩成“标签：值”的筛选胶囊：点击胶囊在浮层中编辑，确认后才写回值，并可直接清除单个条件。

## 何时使用

- 列表工具栏空间有限，但需要让用户快速查看和修改已生效的筛选条件时。
- 条件数量较少，适合把每个字段显示成一个独立筛选胶囊时。
- 需要把多余字段收进“更多筛选”浮层，同时保留已选条件概览时。
- 需要始终展开的多字段查询网格时，用 [QueryFilter 查询表单](/components/queryfilter)；只有一个普通输入框时用 [Search 搜索框](/components/search)。

## 在线体验

<GalleryPreview component="LightFilter" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见[打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
package main

import "github.com/tangoui/tango/ui"

var (
    lightValues = ui.ProValues{"status": "running"}
    lightFields = []ui.ProField{
        {Key: "status", Label: "状态", ValueType: ui.ValueSelect,
            ValueEnum: []ui.ProEnum{{Value: "running", Label: "运行中"}, {Value: "done", Label: "已完成"}}},
        {Key: "owner", Label: "负责人"},
        {Key: "created", Label: "创建日期", ValueType: ui.ValueDate},
    }
)

func page(c *ui.Context) {
    lf := ui.LightFilter(c, lightFields).Values(&lightValues).Show()
    if lf.Changed() {
        reloadList(lf.Params())
    }
}

func reloadList(params map[string]string) {}

func main() {}
```

LightFilter 使用和 `ProForm`、`QueryFilter` 相同的 `ProField` 定义。点击胶囊后，在浮层中编辑并确认，`Changed()` 在确认或清除的当前帧返回 `true`；`Params()` 返回非空值的文本参数。

## 常见用法

### 绑定筛选值并显示请求参数

把 `ProValues` 放在页面状态中，页面外的列表请求可以直接读取它。没有绑定 `Values` 时，组件会在自己的状态中保存筛选值。

```go
package main

import "github.com/tangoui/tango/ui"

var (
    values = ui.ProValues{"owner": "alice", "amount": 1000.0}
    fields = []ui.ProField{
        {Key: "owner", Label: "负责人"},
        {Key: "amount", Label: "金额", ValueType: ui.ValueMoney},
    }
)

func page(c *ui.Context) {
    lf := ui.LightFilter(c, fields).Values(&values).Bordered().Show()
    if lf.Changed() {
        fetch(lf.Params())
    }
}

func fetch(params map[string]string) {}

func main() {}
```

### 折叠到“更多筛选”

`Collapse(n)` 保留前 `n` 个字段为胶囊，其余字段进入一个统一浮层。浮层中的值确认后才生效，“更多筛选”按钮会显示当前已设置的数量。

```go
package main

import "github.com/tangoui/tango/ui"

var moreValues = ui.ProValues{
    "status": "running",
    "amount": 300.0,
}

var moreFields = []ui.ProField{
    {Key: "status", Label: "状态", ValueType: ui.ValueSelect,
        ValueEnum: []ui.ProEnum{{Value: "running", Label: "运行中"}, {Value: "paused", Label: "已暂停"}}},
    {Key: "owner", Label: "负责人"},
    {Key: "amount", Label: "金额", ValueType: ui.ValueMoney},
    {Key: "created", Label: "创建日期", ValueType: ui.ValueDate},
}

func page(c *ui.Context) {
    ui.LightFilter(c, moreFields).Values(&moreValues).Collapse(2).Show()
}

func main() {}
```

### 控制尺寸和边框

`Size` 改变胶囊内控件的高度，`Bordered` 为每个胶囊绘制边框。尺寸可以继承外层 `ConfigProvider.Size`，只有需要覆盖局部工具栏时才在实例上设置。

```go
package main

import "github.com/tangoui/tango/ui"

var filterValues ui.ProValues
var filterFields = []ui.ProField{
    {Key: "type", Label: "类型", ValueType: ui.ValueSelect,
        ValueEnum: []ui.ProEnum{{Value: "bug", Label: "缺陷"}, {Value: "task", Label: "任务"}}},
    {Key: "keyword", Label: "关键词"},
}

func page(c *ui.Context) {
    ui.LightFilter(c, filterFields).Values(&filterValues).
        Size(ui.Small).Bordered().Show()
}

func main() {}
```

### 根据状态隐藏条件

和 QueryFilter 一样，`Hide`、`Disable` 和 `Options` 都接收当前的 `ProValues`。被隐藏的字段不会生成胶囊，也不会进入“更多筛选”浮层。

```go
package main

import "github.com/tangoui/tango/ui"

var values ui.ProValues
var fields = []ui.ProField{
    {Key: "scope", Label: "范围", ValueType: ui.ValueSelect,
        ValueEnum: []ui.ProEnum{{Value: "all", Label: "全部"}, {Value: "mine", Label: "我的"}}},
    {Key: "team", Label: "团队", Hide: func(v ui.ProValues) bool { return v.String("scope") != "mine" }},
    {Key: "archived", Label: "包含归档", ValueType: ui.ValueSwitch},
}

func page(c *ui.Context) {
    ui.LightFilter(c, fields).Values(&values).Show()
}

func main() {}
```

## 主题与样式

LightFilter 继承 `ProField` 控件、颜色和尺寸令牌；`Bordered()` 只改变胶囊边框，不改变浮层内控件。可通过 `ConfigProvider.Defaults("LightFilter", ui.Props{...})` 设置 `size`、`bordered` 和 `collapse` 默认值。shadcn 主题会使用更接近数据表格 faceted filter 的虚线按钮与值徽标，Ant Design 和 Element Plus 保留各自的胶囊边框风格。

## 注意事项

- `Values` 必须绑定到稳定的页面状态；不要每帧新建 `ProValues`，否则浮层草稿和确认后的筛选会被覆盖。
- LightFilter 的值在确认前保存在浮层草稿中，取消或关闭浮层不会触发 `Changed()`；清除单个胶囊会触发一次变更。
- `Collapse(n)` 的 `n <= 0` 表示所有字段都显示为胶囊；字段较多时应设置正数，避免工具栏换行过长。
- 组件根节点是工具栏语义；键盘聚焦胶囊后可打开浮层并在控件、清除和确认之间操作。自定义 `Render` 控件要提供可访问名称。

## 示例代码

<!--@include: ./_gen/lightfilter-examples.md-->

## API

<!--@include: ./_gen/lightfilter-api.md-->

## 相关链接

- [LightFilter gallery 示例](https://github.com/tangoui/tango/blob/main/examples/ui-gallery/page_pro3_filter.go)
- [QueryFilter 查询表单](/components/queryfilter)
- [ProForm 高级表单](/components/proform)
- [源码](https://github.com/tangoui/tango/blob/main/ui/pro3_lightfilter.go)
- [ui Go API](/reference/api/ui)
