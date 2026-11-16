---
title: Watermark 水印
---
# Watermark 水印

在内容上平铺文字或图片水印，用于标识来源、环境或敏感内容；水印层不会拦截内容的鼠标事件。

## 何时使用

- 在内部资料、后台页面或预览区域标识“内部资料”“仅供预览”等信息。
- 在导出前的页面预览中叠加用户、租户或环境标识。
- 需要覆盖整个内容区域时使用 `Watermark`；只给一段文字加背景色时用 [Typography 文本](/components/text)；只显示一次图像时用 [Image 图片](/components/image)。

## 在线体验

<GalleryPreview component="Watermark" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func page(c *ui.Context) {
    ui.Watermark(c, "TanGo UI").Body(func() {
        ui.Column(c, func() {
            ui.Text(c, "这段内容带有水印").Strong()
            ui.Text(c, "水印绘制在内容上方，但不会阻止下方控件接收点击。").Secondary()
            ui.Button(c, "仍可点击").Primary()
        }, layout.Gap(12), layout.W(420), layout.H(180))
    })
}
```

`Watermark` 默认使用 -22° 旋转、100×100 的间距和主题感知的半透明文字；`Body` 中的内容决定水印覆盖区域。

## 常见用法

### 多行文字与自定义样式

构造函数可以接收多行文字；用 `FontSize`、`Weight`、`Color` 和 `Rotate` 调整水印的可读性与强调程度。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
    "github.com/tangoui/tango/ui/text"
    "github.com/tangoui/tango/ui/theme"
)

func page(c *ui.Context) {
    ui.Watermark(c, "TanGo", "内部资料 请勿外传").
        FontSize(14).
        Weight(text.Semibold).
        Color(theme.Fade(c.Theme.Error.Base, 0.2)).
        Rotate(-30).
        Body(func() {
            ui.Text(c, "内部文档预览").Layout(layout.W(360), layout.H(160))
        })
}
```

### 调整间距与首个水印位置

`Gap` 控制横向和纵向的平铺间距，`Offset` 控制左上角第一个水印的偏移；`Grid` 可关闭交错行布局。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func page(c *ui.Context) {
    ui.Watermark(c, "STAGING").
        Gap(72, 64).
        Offset(24, 20).
        Grid().
        Body(func() {
            ui.Text(c, "测试环境数据").Layout(layout.W(440), layout.H(180))
        })
}
```

### 使用图片水印

调用 `Image` 后使用图片代替文字；参数是图片对象和绘制尺寸，默认图片不旋转并以 20% 不透明度绘制，可用 `ImageOpacity` 覆盖。

```go
import (
    "image"

    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func page(c *ui.Context) {
    logo := image.NewRGBA(image.Rect(0, 0, 48, 48))
    ui.Watermark(c).Image(logo, 48, 48).ImageOpacity(0.14).Gap(96, 96).Body(func() {
        ui.Text(c, "带图片水印的预览").Layout(layout.W(420), layout.H(180))
    })
}
```

### 让水印容器参与布局

`Layout` 将布局选项应用到水印容器本身，适合在行内并排放置多个受保护区域。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func page(c *ui.Context) {
    ui.Row(c, func() {
        ui.Watermark(c, "左侧区域").Layout(layout.Grow(1)).Body(func() {
            ui.Text(c, "左侧").Layout(layout.H(140))
        })
        ui.Watermark(c, "右侧区域").Layout(layout.Grow(1)).Body(func() {
            ui.Text(c, "右侧").Layout(layout.H(140))
        })
    }, layout.Gap(16), layout.W(640))
}
```

## 主题与样式

默认颜色是当前主题正文色的半透明版本，随 Ant Design、Element Plus 和 shadcn 主题变化。显式调用 `Color` 会覆盖主题默认值；建议继续使用透明颜色，避免水印遮挡正文。`Weight` 使用 `ui/text` 包的字体权重，布局选项使用 `ui/layout` 包。

## 注意事项

- **覆盖范围**：水印只覆盖 `Body` 返回的容器区域；内容较长时应让外层容器提供足够高度。
- **交互**：水印绘制在子内容之上，但不会接收指针输入，按钮、输入框等仍可操作。
- **图片尺寸**：`Image` 只保存传入的 `image.Image` 引用并按指定尺寸绘制；需要解码图片时在页面外完成加载。
- **即时模式**：`Watermark` 每帧按相同的结构和顺序调用；需要变化的文字、颜色或图片放在页面状态中，不要依赖局部变量跨帧保存状态。
- **性能**：水印会在区域内平铺，过小的 `Gap` 会增加绘制次数；大面积内容优先使用适中的间距和图片尺寸。

## 示例代码

<!--@include: ./_gen/watermark-examples.md-->

## API

<!--@include: ./_gen/watermark-api.md-->

## 相关链接

- [源码 ui/data_watermark.go](https://github.com/tangoui/tango/blob/main/ui/data_watermark.go)
- [Go API 参考](/reference/api/ui)
- [组件总览](/components/)
