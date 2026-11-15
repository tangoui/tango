---
title: StepsForm 分步表单
---
# StepsForm 分步表单

把多组 `ProField` 组织成带步骤导航的表单，当前步骤校验通过后才能继续，最后一步统一提交。

## 何时使用

- 注册、转账、发布或配置向导等需要分组逐步填写的流程。
- 每一步字段较少，且用户需要看到整体进度和上一步/下一步操作时。
- 字段不多且无需分步时，用 [ProForm 高级表单](/components/proform)；独立的步骤导航用 [Steps 步骤条](/components/steps)。
- 需要在弹窗中填写时，用 [ModalForm 弹窗表单](/components/modalform)。

## 在线体验

<GalleryPreview component="StepsForm" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

var values ui.ProValues
var steps = []ui.StepsFormStep{
    {Title: "基本信息", Fields: []ui.ProField{
        {Key: "name", Label: "名称", Required: true},
        {Key: "owner", Label: "负责人", Required: true},
    }},
    {Title: "完成", Fields: []ui.ProField{
        {Key: "notify", Label: "发送通知", ValueType: ui.ValueSwitch, Initial: true},
    }},
}

func page(c *ui.Context) {
    ui.StepsForm(c, steps).Values(&values).
        OnFinish(func(v ui.ProValues, done func(error)) { done(nil) }).
        Show()
}
```

每一步使用 `ProField` 描述字段；下一步会先校验当前字段，最后一步的 `OnFinish` 收到全部步骤的 `ProValues`。

## 常见用法

### 字段类型与校验

步骤可以混合选择、文本、金额和密码等 Pro 字段；`Required`、`Rules` 和 `Min` 等规则与 ProForm 相同。

```go
import "github.com/tangoui/tango/ui"

var transferSteps = []ui.StepsFormStep{
    {Title: "付款信息", Description: "账户与金额", Fields: []ui.ProField{
        {Key: "from", Label: "付款账户", ValueType: ui.ValueSelect, Required: true,
            ValueEnum: []ui.ProEnum{{Value: "a", Label: "主账户"}, {Value: "b", Label: "备用账户"}}},
        {Key: "amount", Label: "金额", ValueType: ui.ValueMoney, Required: true, Min: 0.01},
    }},
    {Title: "验证", Description: "输入支付密码", Fields: []ui.ProField{
        {Key: "password", Label: "支付密码", ValueType: ui.ValuePassword, Required: true,
            Rules: []ui.Rule{ui.MinLen(6, "至少 6 位")}},
    }},
}

func page(c *ui.Context) {
    ui.StepsForm(c, transferSteps).Show()
}
```

### 绑定值与异步提交

`Values` 把所有步骤绑定到自己的 `ProValues`；`OnFinish` 中可以启动保存请求，完成后调用 `done`。调用 `done(nil)` 才会结束提交并进入完成状态。

```go
import "github.com/tangoui/tango/ui"

var formValues ui.ProValues
var profileSteps = []ui.StepsFormStep{
    {Title: "资料", Fields: []ui.ProField{{Key: "name", Label: "姓名", Required: true}}},
    {Title: "确认", Fields: []ui.ProField{{Key: "remark", Label: "备注", ValueType: ui.ValueTextArea}}},
}

func page(c *ui.Context) {
    form := ui.StepsForm(c, profileSteps).Values(&formValues).
        OnFinish(func(v ui.ProValues, done func(error)) {
            go func() {
                // 在真实应用中调用服务，完成后回调 done。
                done(nil)
            }()
        }).Show()
    if form.Finished() {
        ui.Message(c, "success", "提交成功")
    }
}
```

### 在步骤中追加自定义内容

`StepsFormStep.Children` 接收当前步骤的 `*FormW`，可在字段之后追加协议、动态列表等普通 Form 内容。

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    agreed := false
    consentSteps := []ui.StepsFormStep{
        {Title: "协议", Fields: []ui.ProField{{Key: "name", Label: "姓名", Required: true}},
            Children: func(f *ui.FormW) {
                f.Item("协议").NoLabel().Body(func() {
                    ui.Checkbox(c, "我已阅读并同意服务协议", &agreed)
                })
            }},
    }
    ui.StepsForm(c, consentSteps).Show()
}
```

### 宽度与当前步骤

`Width` 控制表单内容宽度；`Current` 可读取当前 0 基步骤，`SetCurrent` 可在业务引导中跳转且不触发校验。

```go
import "github.com/tangoui/tango/ui"

var oneStep = []ui.StepsFormStep{{Title: "配置", Fields: []ui.ProField{{Key: "name", Label: "名称"}}}}

func page(c *ui.Context) {
    form := ui.StepsForm(c, oneStep).Width(720).Show()
    ui.Text(c, "当前步骤："+itoa(form.Current()))
}

func itoa(n int) string {
    if n == 0 {
        return "0"
    }
    return "1"
}
```

## 主题与样式

- 顶部步骤条使用 Steps 的令牌，表单内容使用 ProForm/Form 和字段控件的令牌；三套主题会同步切换。
- 默认宽度为 600，可通过 `Width` 或 `ConfigProvider.Defaults("StepsForm", ...)` 的 `width` 调整。
- 每个步骤的 `Columns` 控制当前步骤表单的栅格列数；不设置时使用单列。

## 注意事项

- `steps` 与绑定的 `ProValues` 应放在稳定状态中；`Show()` 每帧构建当前步骤，但状态由组件按位置保存。
- `OnFinish` 只在最后一步校验通过后调用；无异步保存时也要调用 `done(nil)`，否则提交会一直处于加载状态。
- `done` 可以在 goroutine 中调用，但不要在异步 goroutine 里直接操作 UI 或修改绑定值。
- `Current`、`Finished` 等状态应在 `Show()` 后读取；`Current` 是从 0 开始的索引。
- 步骤标题和字段标签会进入无障碍结构；不要只用颜色表达当前步骤或校验错误。

## 示例代码

<!--@include: ./_gen/stepsform-examples.md-->

## API

<!--@include: ./_gen/stepsform-api.md-->

## 相关链接

- [源码 ui/form_pro_steps.go](https://github.com/tangoui/tango/blob/main/ui/form_pro_steps.go)
- [ui Go API](/reference/api/ui)
- [ProForm 高级表单](/components/proform)、[Steps 步骤条](/components/steps)、[ModalForm 弹窗表单](/components/modalform)
