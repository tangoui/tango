---
title: ProFormGroup 表单分组
---
# ProFormGroup 表单分组

把相关表单项放进带标题、说明、额外操作和可选折叠状态的分组容器，并可按列数组织字段。

## 何时使用

- 一个页面有基本信息、联系信息、结算信息等清晰的字段区块时。
- 需要让用户折叠不常用字段，或在分组标题右侧放校验、帮助等操作时。
- 同一份字段由 `ProForm` 自动分组，或需要在自定义 `Children` 布局中手动控制分组时。
- 只有一两个字段、没有独立标题或折叠需求时，直接使用 ProForm 的 `Columns` 更简单。

## 在线体验

<GalleryPreview component="ProFormGroup" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见[打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
package main

import "github.com/tangoui/tango/ui"

var groupValues ui.ProValues

func page(c *ui.Context) {
    fields := []ui.ProField{
        {Key: "name", Label: "姓名", Group: "基本信息", Required: true},
        {Key: "phone", Label: "手机号", Group: "基本信息"},
        {Key: "city", Label: "城市", Group: "联系信息", ValueType: ui.ValueSelect,
            ValueEnum: []ui.ProEnum{{Value: "bj", Label: "北京"}, {Value: "sh", Label: "上海"}}},
        {Key: "note", Label: "备注", Group: "联系信息", ValueType: ui.ValueTextArea, Span: 24},
    }
    ui.ProForm(c, fields).Values(&groupValues).Columns(2).Show()
}

func main() {}
```

相邻且 `Group` 相同的字段会被 ProForm 自动包进一个 ProFormGroup；字段换组时开始新的分组。`Group` 不是数据键，不会写进提交值。

## 常见用法

### 手动创建带说明和额外操作的分组

`Children` 适合需要自己决定字段顺序或混入自定义控件的页面。`ProFormItems` 会在指定的 `FormW` 中渲染标准 ProField 项。

```go
package main

import "github.com/tangoui/tango/ui"

var (
    accountValues ui.ProValues
    accountFields = []ui.ProField{
        {Key: "bank", Label: "开户行", Required: true},
        {Key: "number", Label: "账号", Required: true},
        {Key: "holder", Label: "户名", Required: true},
    }
)

func page(c *ui.Context) {
    ui.ProForm(c, nil).Values(&accountValues).NoSubmitter().Children(func(f *ui.FormW) {
        ui.ProFormGroup(c, "结算账户").
            Description("用于每月结算打款").
            Columns(3).
            Extra(func() { ui.Button(c, "校验").Small() }).
            Body(func() { ui.ProFormItems(c, f, accountFields, accountValues) })
    }).Show()
}

func main() {}
```

`Description` 显示在标题下方，`Extra` 在标题右侧执行即时模式回调；按钮自己的点击状态仍由页面变量或闭包保存。

### 可折叠并绑定状态

`Collapsed(&folded)` 是受控模式：页面状态决定当前是否折叠，用户点击标题后组件会更新这个布尔值，并在当前帧让 `Toggled()` 返回 `true`。

```go
package main

import "github.com/tangoui/tango/ui"

var folded bool
var contactValues ui.ProValues
var contactFields = []ui.ProField{
    {Key: "email", Label: "邮箱", Required: true},
    {Key: "address", Label: "地址"},
}

func page(c *ui.Context) {
    var group *ui.ProFormGroupW
    ui.ProForm(c, nil).Values(&contactValues).NoSubmitter().Children(func(f *ui.FormW) {
        group = ui.ProFormGroup(c, "联系信息").
            Collapsed(&folded).
            Body(func() { ui.ProFormItems(c, f, contactFields, contactValues) })
    }).Show()
    if group != nil && group.Toggled() {
        auditFoldState(folded)
    }
}

func auditFoldState(open bool) {}

func main() {}
```

如果不需要外部控制，可用 `Collapsible()`；希望首次打开就折叠时用 `DefaultCollapsed()`。空标题 `ProFormGroup(c, "")` 仍可使用折叠行为，但会使用语言包中的展开 / 收起名称。

### 栅格与间距

`Columns(n)` 按可用宽度把组内项目排成 `n` 列，`Gutter` 设置列间距，`Size` 设置项目行之间的间距。字段的 `Span` 仍可覆盖自己的栅格宽度。

```go
package main

import "github.com/tangoui/tango/ui"

var values ui.ProValues
var fields = []ui.ProField{
    {Key: "first", Label: "名"},
    {Key: "last", Label: "姓"},
    {Key: "department", Label: "部门"},
    {Key: "description", Label: "说明", ValueType: ui.ValueTextArea, Span: 24},
}

func page(c *ui.Context) {
    ui.ProForm(c, nil).Values(&values).NoSubmitter().Children(func(f *ui.FormW) {
        ui.ProFormGroup(c, "人员资料").Columns(3).Gutter(16).Size(8).
            Body(func() { ui.ProFormItems(c, f, fields, values) })
    }).Show()
}

func main() {}
```

列数是组内布局意图，不是固定像素宽度；窗口变窄时项目会按可用宽度重新计算。长文本字段可以用 `Span: 24` 独占整行。

### 使用 Extra 放置即时操作

分组标题的 `Extra` 只负责渲染，不会自动提交表单。需要校验或保存时，保留 `ProFormW` 句柄并调用 `Submit`，或在 `Submitter` 中集中管理按钮。

```go
package main

import "github.com/tangoui/tango/ui"

var (
    values ui.ProValues
    form   *ui.ProFormW
)

func page(c *ui.Context) {
    fields := []ui.ProField{{Key: "title", Label: "标题", Required: true}}
    form = ui.ProForm(c, nil).Values(&values).NoSubmitter().Children(func(f *ui.FormW) {
        ui.ProFormGroup(c, "内容").Extra(func() {
            if ui.Button(c, "保存").Primary().Clicked() {
                form.Submit()
            }
        }).Body(func() { ui.ProFormItems(c, f, fields, values) })
    }).OnFinish(func(v ui.ProValues, done func(error)) { done(nil) }).Show()
}

func main() {}
```

## 主题与样式

ProFormGroup 使用表单和布局的主题令牌绘制标题、描述、焦点环和间距。`ConfigProvider.Defaults("ProFormGroup", ui.Props{...})` 支持 `collapsible`、`columns`、`gutter`、`size`；实例上的 `Columns`、`Gutter`、`Size` 等链式方法优先。三套主题会分别应用自己的标题字重、边框和折叠图标风格。

## 注意事项

- `Group` 只对**连续**字段生效；同名字段被其他组隔开时会生成两个分组。
- `Collapsed(&state)` 的布尔变量必须放在稳定页面状态中；每帧重新声明会让折叠状态无法保存。
- 手动 `Children` 中，`ProFormItems` 使用的 `ProValues` 应与外层 `Values` 相同，避免控件写入另一份状态。
- 折叠标题具有按钮语义和键盘操作；`Extra` 中的图标按钮要用 `A11yLabel` 命名，避免只有图标没有可访问名称。

## 示例代码

<!--@include: ./_gen/proformgroup-examples.md-->

## API

<!--@include: ./_gen/proformgroup-api.md-->

## 相关链接

- [ProFormGroup gallery 示例](https://github.com/tangoui/tango/blob/main/examples/ui-gallery/page_shad2d_pro4.go)
- [ProForm 高级表单](/components/proform)
- [Form 表单](/components/form)
- [源码](https://github.com/tangoui/tango/blob/main/ui/pro4_group.go)
- [ui Go API](/reference/api/ui)
