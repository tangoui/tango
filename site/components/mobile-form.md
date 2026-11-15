---
title: Mobile Form 移动端表单
---
# Mobile Form 移动端表单

以移动端分组和标签布局组织 Field、Stepper、开关及自定义输入，并显示帮助与错误信息。

## 何时使用

- 移动端需要标签、输入控件、帮助或错误提示时使用 `Form`、`FormItem` 和 `Field`。
- 只有一两个简单输入时可以直接使用 `Field`，不必为了包装而创建 Form。
- 多步骤流程或桌面后台表单应使用对应的 Form Pro / Steps 组件。
- Form 只负责布局和呈现，不替业务执行校验、提交或网络请求。

## 在线体验

真实 gallery 展示左右标签、顶部标签、错误提示、Stepper 和提交按钮。

<GalleryPreview component="mobile.Form" />

## 基本用法

Field 通过 `*string` 绑定值；FormItem 的 Body 负责放置具体控件。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/mobile"
)

var name string

func page(c *ui.Context) {
    mobile.Form(c).Header("账户").Body(func() {
        mobile.FormItem(c, "姓名").Required().Body(func() {
            mobile.Field(c, &name).Placeholder("请输入姓名")
        })
    })
    _ = ui.ID(0)
}
```

## 常见用法

### 左侧标签和必填

默认标签在左侧，`Required` 用于表达必填语义；实际校验仍由业务代码完成。

```go
var phone string

func horizontal(c *ui.Context) {
    mobile.Form(c).Header("基本资料").Body(func() {
        mobile.FormItem(c, "手机号").Required().Body(func() {
            mobile.Field(c, &phone).Placeholder("请输入手机号").MaxLength(11)
        })
    })
}
```

### 垂直标签和帮助文字

`Vertical` 将标签放到输入上方，`Help` 适合说明格式或长度限制。

```go
var note string

func vertical(c *ui.Context) {
    mobile.Form(c).Vertical().Body(func() {
        mobile.FormItem(c, "备注").Help("最多 100 字").Body(func() {
            mobile.Field(c, &note).Placeholder("选填").MaxLength(100)
        })
    })
}
```

### 错误和提交

错误文本由页面状态决定；按钮点击只触发提交动作，不会自动发送请求。

```go
var email, errorText string

func submit(c *ui.Context) {
    mobile.Form(c).Body(func() {
        mobile.FormItem(c, "邮箱").Error(errorText).Body(func() {
            mobile.Field(c, &email).Placeholder("name@example.com")
        })
    })
    if mobile.Button(c, "提交").Primary().Block().Clicked() {
        if email == "" {
            errorText = "请输入邮箱"
        }
    }
}
```

### 数字输入和密码

Stepper 绑定 `*float64`，Field 可用 `Password` 切换密码输入。

```go
var quantity float64
var password string

func controls(c *ui.Context) {
    mobile.Form(c).Body(func() {
        mobile.FormItem(c, "数量").Body(func() {
            mobile.Stepper(c, &quantity).Min(1).Max(10).Step(1)
        })
        mobile.FormItem(c, "密码").Body(func() {
            mobile.Field(c, &password).Password().Placeholder("请输入密码")
        })
    })
}
```

### 带箭头的选择项

FormItem 可以用 `Arrow` 表示会打开 Picker 或下一页，并读取 `Clicked`。

```go
var pickerOpen bool

func choice(c *ui.Context) {
    item := mobile.FormItem(c, "生日").Arrow().Body(func() { ui.Text(c, "1990-06-01") })
    if item.Clicked() {
        pickerOpen = true
    }
    _ = pickerOpen
}
```

## 注意事项

输入字符串、数字、错误文本和弹层开关放在页面闭包或其他稳定状态中，不能在每帧重置。`Changed`、`Submitted` 和 `Clicked` 是当前帧事件；异步校验应在 UI 外完成，再把结果写回状态。密码字段不要把值写入日志。给没有可见标签的自定义控件补充无障碍标签，并在 RTL 下让布局自动镜像。

## 示例代码

<!--@include: ./_gen/mobile-form-examples.md-->

## API

<!--@include: ./_gen/mobile-form-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/mobile/form.go)
- [Button 按钮](/components/mobile-button)
- [PickerView 选择器视图](/components/mobile-pickerview)
- [ui mobile Go API](/reference/api/ui-mobile)
