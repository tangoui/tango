---
title: Icon 图标
---
# Icon 图标

用统一的矢量图标表达动作、状态、导航和文件类型，图标会随当前设计风格切换到对应的图标库。

## 何时使用

- 在按钮、菜单、状态提示、列表操作中提供视觉线索。
- 需要单独展示图标，或要控制图标尺寸、颜色、旋转和动画时使用。
- 只有图标、没有可见文字的交互控件必须同时给按钮或控件设置 `A11yLabel`；不要把纯图标当作没有标签的操作入口。
- 需要一段可读文字时使用 [Text 排版](/components/text)，不要用图标代替说明文字。

## 在线体验

gallery 中包含内置图标、状态图标、文件类型图标和三套图标库浏览器，可切换设计语言并悬停查看名称。

<GalleryPreview component="Icon" />

## 基本用法

`ui.Icon` 接收 `ui/icon` 包中的 `*icon.Icon`，默认使用 14px 和当前文本色。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

func iconPage(c *ui.Context) {
    ui.Icon(c, icon.Search)
    ui.Icon(c, icon.CheckFilled).Size(20).Color(c.Theme.Success.Base)
    ui.Icon(c, icon.Loading).Size(20).Spin()
}
```

图标是即时渲染树中的叶节点；链式方法应在同一帧、同一处构建时调用。

## 常见用法

### 尺寸与颜色

使用 `Size` 和 `Color` 匹配文本层级或状态色。颜色应优先取当前主题令牌，而不是写死 RGB 值。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

func iconColorPage(c *ui.Context) {
    ui.Row(c, func() {
        ui.Icon(c, icon.Info).Size(16).Color(c.Theme.Primary.Base)
        ui.Icon(c, icon.Warning).Size(20).Color(c.Theme.Warning.Base)
        ui.Icon(c, icon.CloseFilled).Size(24).Color(c.Theme.Error.Base)
    })
}
```

### 旋转与加载

`Rotate` 接收角度，`Spin` 使用主题动画让适合表示进行中的图标旋转。用户开启减少动画时，TanGo 会按全局动画策略处理。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

func iconMotionPage(c *ui.Context) {
    ui.Icon(c, icon.ArrowRight).Rotate(90)
    ui.Icon(c, icon.Loading).Spin().A11yLabel("正在加载")
}
```

### 按名称浏览图标

gallery 使用 `icon.Names` 和 `icon.ByName` 构建图标浏览器。名称来自已注册的图标集合，适合在开发工具或图标选择器中使用；业务代码更推荐直接引用具名变量。

```go
import (
    "strings"

    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

var iconQuery string

func iconSearchPage(c *ui.Context) {
    ui.Input(c, &iconQuery).Placeholder("搜索图标名").AllowClear()
    q := strings.ToLower(strings.TrimSpace(iconQuery))
    for _, name := range icon.Names() {
        if strings.Contains(strings.ToLower(name), q) {
            ui.Icon(c, icon.ByName(name)).Size(18).A11yLabel(name)
        }
    }
}
```

### 无障碍标签

图标单独出现时用 `A11yLabel` 提供可访问名称；图标已经包含在带文字的 `Button` 或 `FloatButton.Description` 中时，不必重复添加可见标签。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

func iconA11yPage(c *ui.Context) {
    ui.Button(c, "").Icon(icon.Setting).A11yLabel("打开设置")
    ui.Icon(c, icon.Info).A11yLabel("信息")
}
```

## 主题与样式

- Ant Design 主题使用 Ant Design 图标语义；Element Plus 主题使用 Element 图标；shadcn 主题使用 Lucide 图标。组件仍通过同一个 `*icon.Icon` API 绘制。
- `Color` 可接 `c.Theme.Primary.Base`、`c.Theme.TextSecondary` 等当前主题颜色；尺寸由 `Size(px)` 明确指定。
- gallery 的图标库场景还展示 `ui/icon/antd`、`ui/icon/element`、`ui/icon/lucide` 中的完整集合；这些库的具体变量属于图标数据包，不改变 `ui.Icon` 的调用方式。

## 注意事项

- **状态放在哪**：Icon 本身没有业务状态；是否显示、当前图标名称和旋转角度应放在页面函数外或 `ui.State` 中。
- **无障碍**：纯装饰图标不要承担唯一信息；纯图标按钮必须设置 `A11yLabel`，并确保外层控件可以通过 Tab 聚焦。
- **RTL**：方向性图标应使用 TanGo 的方向图标（例如 `icon.Left` / `icon.Right`），不要用普通图形手动翻转；组件会在 RTL 作用域中处理方向。
- **动画**：加载图标只在确实有进行中工作时使用 `Spin`；长时间持续动画应尊重减少动画设置。

## 示例代码

<!--@include: ./_gen/icon-examples.md-->

## API

<!--@include: ./_gen/icon-api.md-->

## 相关链接

- 源码：[ui/text.go](https://github.com/tangoui/tango/blob/main/ui/text.go)、[ui/icon/icon.go](https://github.com/tangoui/tango/blob/main/ui/icon/icon.go)
- [ui Go API](/reference/api/ui)
- 相关组件：[Button 按钮](/components/button)、[FloatButton 悬浮按钮](/components/floatbutton)、[Text 排版](/components/text)
