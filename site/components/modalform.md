---
title: ModalForm 弹窗表单
---
# ModalForm 弹窗表单

把 ProForm 放进 Modal 或 Drawer，用确认按钮完成校验和提交，保存成功后自动关闭。

## 何时使用

- 新建、编辑规则或配置项等字段较少、需要聚焦完成的表单。
- 需要校验、提交中状态和保存成功后自动关闭，而不想自己拼接 Modal 与 ProForm 时。
- 内容较长或需要从右侧展开时，用同一 API 的 `DrawerForm`；完整页面表单用 [ProForm 高级表单](/components/proform)。
- 只需要一句确认而没有字段时，用 [Confirm 确认框](/components/confirm)。

## 在线体验

<GalleryPreview component="ModalForm" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

var open bool
var fields = []ui.ProField{
    {Key: "name", Label: "规则名称", Required: true, Placeholder: "请输入名称"},
    {Key: "owner", Label: "负责人", Required: true},
}

func page(c *ui.Context) {
    if ui.Button(c, "新建规则").Primary().Clicked() {
        open = true
    }
    ui.ModalForm(c, &open, "新建规则", fields).
        OnFinish(func(v ui.ProValues, done func(error)) { done(nil) }).
        Show()
}
```

点击 Modal 的确认按钮后，内置 ProForm 先校验字段，再调用 `OnFinish`；`done(nil)` 后对话框关闭。

## 常见用法

### 绑定表单值与异步保存

`Values` 让页面外可以读取和预填 `ProValues`。保存请求完成后调用 `done`；返回错误会保留弹窗并显示表单错误。

```go
import "github.com/tangoui/tango/ui"

var (
    editOpen bool
    editVals = ui.ProValues{"name": "TradeCode 99"}
    editFields = []ui.ProField{{Key: "name", Label: "规则名称", Required: true}}
)

func page(c *ui.Context) {
    if ui.Button(c, "编辑").Clicked() {
        editOpen = true
    }
    form := ui.ModalForm(c, &editOpen, "编辑规则", editFields).
        Values(&editVals).
        OnFinish(func(v ui.ProValues, done func(error)) {
            go func() {
                // 在真实应用中完成保存后回调 done(err)。
                done(nil)
            }()
        }).Show()
    if form.Finished() {
        ui.Message(c, "success", "保存成功")
    }
}
```

### DrawerForm 抽屉表单

`DrawerForm` 使用相同的字段、校验和提交回调，只把容器换成右侧抽屉；`Width` 设置抽屉宽度。

```go
import "github.com/tangoui/tango/ui"

var drawerOpen bool
var drawerFields = []ui.ProField{
    {Key: "title", Label: "标题", Required: true},
    {Key: "description", Label: "说明", ValueType: ui.ValueTextArea},
}

func page(c *ui.Context) {
    if ui.Button(c, "打开抽屉").Primary().Clicked() {
        drawerOpen = true
    }
    ui.DrawerForm(c, &drawerOpen, "编辑内容", drawerFields).
        Width(520).
        OnFinish(func(v ui.ProValues, done func(error)) { done(nil) }).
        Show()
}
```

### 多列和保留值

`Columns` 把字段排成栅格列。关闭后默认保留字段值，符合 Pro Components v3 的默认行为。`DestroyOnHidden(true)` 在关闭时把值恢复到首次打开时的初始快照，显式绑定的 `Values` 也会被重置。

```go
import "github.com/tangoui/tango/ui"

var openAdvanced bool
var advancedFields = []ui.ProField{
    {Key: "name", Label: "名称", Required: true},
    {Key: "code", Label: "编码", Required: true},
    {Key: "owner", Label: "负责人"},
    {Key: "status", Label: "状态"},
}

func page(c *ui.Context) {
    if ui.Button(c, "高级配置").Clicked() {
        openAdvanced = true
    }
    ui.ModalForm(c, &openAdvanced, "高级配置", advancedFields).
        Columns(2).Show()
}
```

### 只读详情

`Readonly(true)` 把字段显示为只读值；Modal 的确认按钮只负责关闭，DrawerForm 会移除确认按钮，适合复用编辑字段作为详情视图。

```go
import "github.com/tangoui/tango/ui"

var detailOpen bool
var detailFields = []ui.ProField{
    {Key: "name", Label: "名称"},
    {Key: "amount", Label: "金额", ValueType: ui.ValueMoney},
}
var detailValues = ui.ProValues{"name": "TradeCode 99", "amount": 12800.0}

func page(c *ui.Context) {
    if ui.Button(c, "查看详情").Clicked() {
        detailOpen = true
    }
    ui.ModalForm(c, &detailOpen, "规则详情", detailFields).
        Values(&detailValues).Readonly(true).Show()
}
```

## 主题与样式

- ModalForm 使用 Modal、ProForm 和字段控件的令牌；DrawerForm 额外使用 Drawer 的宽度、页脚和滑入动画令牌。
- `Width` 控制 Modal 宽度或 Drawer 尺寸，两者默认均为 800px。`Columns` 控制表单栅格列数，默认 1 列。
- `ConfigProvider.Defaults("ModalForm", ...)` 和 `Defaults("DrawerForm", ...)` 支持 `width`、`columns`；链式方法优先。

## 注意事项

- `open` 和 `ProValues` 必须放在稳定状态中；ModalForm 会根据标题生成内部状态 key，多个同标题表单要用不同标题或外层 `Key` 区分。
- 调用 `KeepValues()` 可恢复默认的保留行为。
- `OnFinish` 的 `done` 可以异步调用；`done(nil)` 才会关闭，错误应传给 `done` 以保留弹窗并显示错误。
- `Finished` 应在 `Show()` 后读取，且只报告最近一次成功提交；不要在 `OnFinish` goroutine 中直接操作 UI。
- 弹窗标题、字段标签和校验错误会进入无障碍结构；只读模式仍应提供明确的关闭操作。

## 示例代码

<!--@include: ./_gen/modalform-examples.md-->

## API

<!--@include: ./_gen/modalform-api.md-->

## 相关链接

- [源码 ui/form_pro_modal.go](https://github.com/tangoui/tango/blob/main/ui/form_pro_modal.go)
- [ui Go API](/reference/api/ui)
- [ProForm 高级表单](/components/proform)、[Modal 对话框](/components/modal)、[Drawer 抽屉](/components/drawer)、[StepsForm 分步表单](/components/stepsform)
