---
title: LoginForm 登录表单
---
# LoginForm 登录表单

提供账号密码和手机号验证码两种登录流程的 Pro 登录表单，包含标题、登录方式切换、记住登录、忘记密码、验证码倒计时和异步提交状态。

## 何时使用

- 管理后台、桌面应用或 Web 应用的登录入口需要统一账号 / 手机号体验时。
- 登录提交、发送验证码或忘记密码都需要异步回调和错误反馈时。
- 需要全页背景、侧栏宣传内容或卡片定位时，使用 `LoginFormPage`。
- 需要完全自定义字段、布局或多因素流程时，用 [Form 表单](/components/form) 自己组合更合适。

## 在线体验

<GalleryPreview component="LoginForm" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见[打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
package main

import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    login := ui.LoginForm(c).
        Title("Tango").
        SubTitle("登录工作台").
        OnFinish(func(values ui.ProValues, done func(error)) {
            authenticate(values)
            done(nil)
        }).
        Show()
    if login.Finished() {
        openDashboard()
    }
}

func authenticate(values ui.ProValues) {}
func openDashboard() {}

func main() {}
```

默认同时显示账号密码和手机号两个 Tab。`OnFinish` 收到的值包含 `loginType`（`ui.LoginAccount` 或 `ui.LoginMobile`）、`username`、`password`、`mobile`、`captcha` 和 `autoLogin`；异步保存完成后必须调用 `done`。

## 常见用法

### 只显示账号密码登录

`AccountOnly()` 隐藏 Tab 和手机号流程。`OnForgot` 非空时显示忘记密码链接，点击后通过句柄的 `ForgotClicked()` 或闭包执行跳转。

```go
package main

import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    form := ui.LoginForm(c).
        AccountOnly().
        Title("管理后台").
        OnForgot(func() { resetPassword() }).
        OnFinish(func(values ui.ProValues, done func(error)) {
            done(signIn(values))
        }).
        Show()
    if form.ForgotClicked() {
        showResetHelp()
    }
}

func resetPassword() {}
func showResetHelp() {}
func signIn(values ui.ProValues) error { return nil }

func main() {}
```

### 手机号验证码登录

`MobileOnly()` 只保留手机号和验证码字段。点击“获取验证码”前会按 `MobilePattern` 校验手机号；`OnSendCode` 可以把发送请求放入 goroutine，成功回调 `done(nil)` 后开始倒计时。

```go
package main

import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    ui.LoginForm(c).
        MobileOnly().
        Title("手机号登录").
        MobilePattern(`^1\d{10}$`).
        CodeInterval(60).
        OnSendCode(func(mobile string, done func(error)) {
            go func() { done(sendCode(mobile)) }()
        }).
        OnFinish(func(values ui.ProValues, done func(error)) {
            go func() { done(verifyCode(values)) }()
        }).
        Show()
}

func sendCode(mobile string) error { return nil }
func verifyCode(values ui.ProValues) error { return nil }

func main() {}
```

`Countdown()` 返回剩余秒数；组件会在倒计时开始和结束时播报状态，而不是每秒播报。需要更短的演示倒计时可以传入较小的 `CodeInterval`，生产环境应使用服务端实际过期时间。

### 自定义标题和其他登录方式

`Logo`、`Title`、`SubTitle` 负责头部，`Actions` 负责表单下方的其他登录方式。纯图标按钮必须用 `A11yLabel` 命名。

```go
package main

import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

func logo(c *ui.Context) {
    ui.Text(c, "T").Strong()
}

func otherMethods(c *ui.Context) {
    ui.Button(c, "").Icon(icon.Message).Circle().A11yLabel("短信登录")
    ui.Button(c, "").Icon(icon.Global).Circle().A11yLabel("第三方账号登录")
    ui.Button(c, "").Icon(icon.Mail).Circle().A11yLabel("邮箱登录")
}

func page(c *ui.Context) {
    ui.LoginForm(c).
        Logo(func() { logo(c) }).
        Title("Tango").
        SubTitle("一套代码运行在桌面、Web 和移动端").
        Actions(func() { otherMethods(c) }).
        Show()
}

func main() {}
```

### 全页登录页与侧栏

`LoginFormPage` 在背景上放置登录卡片；`Placement` 支持 `left`、`center`、`right`，`Side` 在卡片旁边或下方渲染说明内容。背景可以用 `Background` 或 `BackgroundImage` 设置。

```go
package main

import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    ui.LoginFormPage(c).
        Placement("center").
        Title("Tango").
        SubTitle("欢迎回来").
        Side(func() {
            ui.Text(c, "统一工作台").Strong()
            ui.Text(c, "在不同平台继续处理你的任务。")
        }).
        OnFinish(func(values ui.ProValues, done func(error)) { done(nil) }).
        Show()
}

func main() {}
```

`LoginFormPage` 仍然使用 LoginForm 的所有提交和校验回调；`Side` 只改变页面布局，不参与登录值。

### 读取提交状态和错误

`Submitting()` 适合在页面的其他区域禁用导航，`Err()` 用于记录最近一次发送验证码或登录失败，`Values()` 用于读取当前表单值。

```go
package main

import "github.com/tangoui/tango/ui"

var current *ui.LoginFormW

func page(c *ui.Context) {
    current = ui.LoginForm(c).OnFinish(func(v ui.ProValues, done func(error)) {
        done(nil)
    }).Show()
    if current.Submitting() {
        ui.Text(c, "正在验证……")
    }
    _ = current.Values().String("username")
    _ = current.Err()
}

func main() {}
```

## 主题与样式

LoginForm 使用表单、输入框、按钮和 Tabs 的主题令牌；`Width` 默认 328，shadcn 主题使用更宽的登录块。可通过 `ConfigProvider.Defaults("LoginForm", ui.Props{"title": "...", "subTitle": "...", "submitText": "登录", "width": 360})` 设置默认标题、按钮和宽度；`LoginFormPage` 另有 `placement` 默认值。三套主题会调整卡片、标签、输入框和按钮的视觉，不改变回调数据结构。

## 注意事项

- LoginForm 自己持有交互状态，但调用位置必须保持稳定；`OnFinish`、`OnSendCode` 的业务状态应放在页面状态或服务层，不要每帧重置。
- 所有异步回调都要调用 `done`，否则按钮会一直处于加载状态；错误应通过 `done(err)` 交回组件显示。
- 默认手机号规则是 11 位且以 1 开头；境外号码必须显式使用 `MobilePattern`。
- `Actions` 中的纯图标按钮必须设置 `A11yLabel`；验证码倒计时已有可访问播报，不要另加每秒 live region。
- 生产环境应在服务端限流验证码发送，并在 `OnFinish` 中再次校验登录类型、验证码和会话状态。

## 示例代码

<!--@include: ./_gen/loginform-examples.md-->

## API

<!--@include: ./_gen/loginform-api.md-->

## 相关链接

- [LoginForm gallery 示例](https://github.com/tangoui/tango/blob/main/examples/ui-gallery/page_pro3_login.go)
- [ProForm 高级表单](/components/proform)
- [Form 表单](/components/form)
- [源码](https://github.com/tangoui/tango/blob/main/ui/pro3_login.go)
- [ui Go API](/reference/api/ui)
