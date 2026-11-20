---
title: QRCode 二维码
---
# QRCode 二维码

把文本编码为可扫描的二维码，支持纠错等级、颜色、尺寸、中心图标和过期状态。

## 何时使用

- 展示登录链接、邀请链接、下载地址或支付信息等可扫描内容。
- 需要在页面上明确展示加载中、已过期和已扫描状态时。
- 只是展示短文本时用 [Text 文本](/components/text)；需要条形码或其他编码格式时选择对应的专用组件。

## 在线体验

示例包含基础二维码、中心图标、过期/加载/已扫描状态，以及颜色、尺寸和纠错等级。

<GalleryPreview component="QRCode" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import "github.com/tangoui/tango/ui"

var inviteURL = "https://example.com/invite/abc"

func page(c *ui.Context) {
    ui.QRCode(c, inviteURL).Size(160).A11yLabel("邀请链接二维码")
}
```

`QRCode(c, value)` 默认创建 160 逻辑像素、带边框的二维码，纠错级别从 M 开始。默认会在二维码版本不变时尽量提高纠错级别，可用 `BoostLevel(false)` 固定为指定级别。二维码内容由 `value` 决定；页面需要重新生成不同内容时，直接传入新的值即可。

## 常见用法

以下片段省略了 `import`。

### 纠错等级与中心图标

`Level` 接受 `L`、`M`、`Q`、`H` 四级纠错。二维码加入中心图标或处于可能被遮挡的环境时建议使用 `H`，同时不要让图标盖住过多模块。

```go
var logo image.Image = image.NewRGBA(image.Rect(0, 0, 32, 32))

func logoCode(c *ui.Context) {
    ui.QRCode(c, inviteURL).Level("H").Icon(logo).IconSize(36)
}
```

### 自定义颜色、背景和尺寸

`Color` 修改模块颜色，`Bg` 修改二维码背景，`Bordered(false)` 去掉外框。自定义颜色时要保持前景和背景有足够对比度，避免影响扫描。

```go
func styledCode(c *ui.Context) {
    ui.Row(c, func() {
        ui.QRCode(c, inviteURL).Size(120).Color(c.Theme.Success.Base)
        ui.QRCode(c, inviteURL).Size(120).
            Color(theme.Hex("#1677ff")).Bg(theme.Hex("#f5f5f5")).Bordered(false)
    }, layout.Gap(16))
}
```

### 过期、加载与已扫描状态

`Status` 支持 `active`（默认）、`expired`、`loading` 和 `scanned`。过期状态显示刷新动作，`Refreshed()` 在用户点击刷新时返回 `true`，此时由应用获取新的内容并重新渲染。

```go
var qrStatus = "active"

func statusCode(c *ui.Context) {
    code := ui.QRCode(c, inviteURL).Status(qrStatus)
    if code.Refreshed() {
        inviteURL = "https://example.com/invite/new"
        qrStatus = "active"
    }
}
```

### 读取编码矩阵

`Matrix()` 返回编码后的 `*QRMatrix`，编码失败时为 `nil`。通常只需要把组件画出来；需要做诊断或自定义导出时才读取矩阵。

```go
func inspectCode(c *ui.Context) {
    code := ui.QRCode(c, inviteURL)
    if code.Matrix() == nil {
        ui.Text(c, "二维码内容无法编码").Color(c.Theme.Error.Base)
    }
}
```

## 主题与样式

Ant Design 使用白色二维码底板和边框；Element Plus 沿用同一组件并使用 Element 圆角令牌；shadcn 保持高对比度底板。深色主题默认仍使用黑色模块和白色底板，以保证常见扫码器的识别率。

`ConfigProvider.Defaults("QRCode", ...)` 支持 `"size"`（像素）、`"errorLevel"`（`L` / `M` / `Q` / `H`）和 `"bordered"`；实例上的 `Size`、`Level`、`Bordered` 优先。

## 注意事项

- **内容安全**：二维码只编码传入的字符串；不要把密码、访问令牌等敏感信息直接放入可被拍摄的二维码。
- **可扫描性**：优先保持深色模块、浅色背景和足够留白；使用中心图标时使用 H 级纠错并控制图标尺寸。
- **状态放在哪**：过期时间、当前 URL、刷新状态和请求状态由应用保存；不要在渲染函数中生成随机值，否则二维码会不断变化。
- **无障碍**：组件默认以图像语义使用编码值作为名称；对 URL 或复杂内容应使用 `A11yLabel` 提供用户可理解的名称。状态会暴露忙碌、过期和已扫描描述。
- **RTL**：二维码本身不镜像，周围布局和状态文字遵循 RTL；不要对二维码图像做水平翻转。

## 示例代码

<!--@include: ./_gen/qrcode-examples.md-->

## API

<!--@include: ./_gen/qrcode-api.md-->

## 相关链接

- 源码：[ui/data_qrcode.go](https://github.com/tangoui/tango/blob/main/ui/data_qrcode.go)
- [ui Go API 参考](/reference/api/ui)
- [Image 图片](/components/image)、[Result 结果](/components/result)
- [组件总览](/components/)、[组件截图目录](/components/screenshots)
