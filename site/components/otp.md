---
title: Input.OTP 验证码输入
---
# Input.OTP 验证码输入

把一次性密码或短验证码拆成多个可独立聚焦的输入格，绑定为一个字符串。

## 何时使用

- 登录、支付或二次验证中输入固定长度的一次性验证码时。
- 需要编辑普通文本或不固定长度的内容时，用 [Input 输入框](/components/input)。
- 需要输入密码而不是短验证码时，用密码类型的输入框，并配合表单校验。

## 在线体验

<GalleryPreview component="OTP" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import "github.com/tangoui/tango/ui"

var code string

func page(c *ui.Context) {
    otp := ui.OTP(c, &code, 6).Numeric().A11yLabel("短信验证码")
    if otp.Completed() {
        ui.Text(c, "验证码已输入："+code)
    }
}
```

第三个参数是输入格数量，`Numeric()` 限制为数字。值由 `*string` 持有，填满所有格子时 `Completed()` 在当前帧返回 `true`。

## 常见用法

### 掩码输入

`Mask` 只改变输入格的显示，不会改变绑定值；例如 `Mask("•")` 适合需要隐藏验证码的界面。

```go
import "github.com/tangoui/tango/ui"

var secretCode string

func page(c *ui.Context) {
    ui.OTP(c, &secretCode, 6).Numeric().Mask("•").A11yLabel("隐藏验证码")
}
```

### 监听完成与修改

`Changed()` 表示当前帧编辑过任意一格，`Completed()` 表示这次编辑后所有格子都已填满。提交逻辑应放在 `Completed()` 分支中，避免每次输入都提交。

```go
import "github.com/tangoui/tango/ui"

var (
    verifyCode string
    verified   bool
)

func page(c *ui.Context) {
    otp := ui.OTP(c, &verifyCode, 4).Numeric()
    if otp.Changed() && otp.Completed() {
        verified = verifyCode != ""
    }
    if verified {
        ui.Text(c, "验证码格式完整")
    }
}
```

### 自动聚焦第一个空格

`Focus()` 会聚焦第一个空格；全部填满时会聚焦最后一格。只在首次需要聚焦时调用，避免每帧强制抢占用户当前焦点。

```go
import "github.com/tangoui/tango/ui"

var loginCode string

func page(c *ui.Context) {
    otp := ui.OTP(c, &loginCode, 6).Numeric()
    if loginCode == "" && c.Focused() == 0 {
        otp.Focus()
    }
}
```

### 尺寸、间距与状态

`Size` 设置输入格尺寸，`Gap` 设置格子之间的像素间距，`Status("error")` 显示校验错误，`Disabled(true)` 禁止编辑。

```go
import "github.com/tangoui/tango/ui"

var inputCode string

func page(c *ui.Context) {
    ui.OTP(c, &inputCode, 4).Size(ui.Small).Gap(4)
    ui.OTP(c, &inputCode, 4).Size(ui.Large).Status("error").A11yLabel("错误验证码")
    ui.OTP(c, &inputCode, 4).Disabled(true).A11yLabel("禁用验证码")
}
```

## 主题与样式

- 三套风格使用相同的输入格交互；shadcn 风格把格子连接为一个 InputOTP 组，Ant Design 与 Element Plus 风格默认保留格间距。
- 输入格沿用输入控件的全局尺寸、字体、边框和焦点令牌；OTP 没有独立的公开 `OTPToken`。
- `ConfigProvider.Defaults("OTP", ui.Props{"mask": "•"})` 可设置掩码等 OTP 专属默认值；长度始终由 `OTP(c, value, n)` 的 `n` 决定。

## 注意事项

- **状态**：验证码字符串放在页面函数外或 `ui.State` 中；不要把 `code` 声明在页面函数内部并每帧重置。
- **输入规则**：`Numeric()` 只接受 ASCII 数字；不调用它时接受非空格字符。粘贴内容会按格子顺序填入。
- **键盘操作**：每格都可聚焦，方向键和退格可在格子之间移动；Tab 顺序由组件维护。
- **无障碍**：用 `A11yLabel` 命名整组，每格会按“第 n 个字符 / 共 n 个”暴露；错误状态会传给辅助技术。
- **安全性**：`Mask` 只负责视觉隐藏，不是加密；提交和日志中不要记录验证码明文。

## 示例代码

<!--@include: ./_gen/otp-examples.md-->

## API

<!--@include: ./_gen/otp-api.md-->

## 相关链接

- [源码 ui/entry_otp.go](https://github.com/tangoui/tango/blob/main/ui/entry_otp.go)
- [ui Go API](/reference/api/ui#OTP)
- [Input 输入框](/components/input)、[Form 表单](/components/form)
