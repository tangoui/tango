---
title: CheckCard 多选卡片
---
# CheckCard 多选卡片

以卡片形式呈现可选项，支持单卡切换、单选组和多选组。

## 何时使用

- 选项需要标题、描述、头像或图标，普通 Checkbox 的信息密度不够时。
- 选择产品方案、页面模板、工作区或功能模块等视觉化选项时。
- 只有简短文本选项时，用 [Radio 单选](/components/radio) 或 [Checkbox 多选](/components/checkbox) 更紧凑。
- 选项很多、需要搜索或虚拟滚动时，不应把所有项都渲染成卡片。

## 在线体验

<GalleryPreview component="CheckCard" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

var selected bool

func page(c *ui.Context) {
    card := ui.CheckCard(c, "启用智能摘要", &selected).
        Description("在保存后自动生成摘要").Avatar("智").Show()
    if card.Changed() {
        ui.Message(c, "info", "选项已切换")
    }
}
```

卡片通过 `*bool` 绑定选中状态；`Show()` 负责构建卡片并处理点击、Enter 和空格操作。

## 常见用法

### 单选组

`CheckCardGroup.Value` 绑定一个字符串；再次点击已选卡片会清空值。每个选项由 `CheckCardOption` 描述。

```go
import "github.com/tangoui/tango/ui"

var template string
var templates = []ui.CheckCardOption{
    {Value: "dashboard", Title: "仪表盘", Description: "监控和指标", Avatar: "仪"},
    {Value: "form", Title: "表单页", Description: "配置和录入", Avatar: "表"},
    {Value: "disabled", Title: "暂不可用", Disabled: true},
}

func page(c *ui.Context) {
    ui.CheckCardGroup(c, templates).Value(&template).Show()
}
```

### 多选组

`Values` 绑定 `[]string`，提交顺序按选项定义顺序保持；单个选项仍可通过 `Disabled` 禁用。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

var features []string
var featureOptions = []ui.CheckCardOption{
    {Value: "table", Title: "数据表格", Description: "查询与筛选", Icon: icon.Table},
    {Value: "chart", Title: "图表", Description: "趋势与分布", Icon: icon.Dashboard},
}

func page(c *ui.Context) {
    ui.CheckCardGroup(c, featureOptions).Values(&features).Show()
}
```

### 尺寸、宽度与状态

`Small`、默认尺寸和 `Large` 分别提供 212、320、440 像素的默认宽度；`Width` 可以覆盖默认值，`Loading` 显示骨架。

```go
import "github.com/tangoui/tango/ui"

var compact, normal, loading bool

func page(c *ui.Context) {
    ui.Row(c, func() {
        ui.CheckCard(c, "小卡片", &compact).Small().Show()
        ui.CheckCard(c, "标准卡片", &normal).Width(360).Show()
        ui.CheckCard(c, "加载中", &loading).Loading(true).Show()
    })
}
```

### 描述、头像与右上角内容

`Avatar` 或 `AvatarIcon` 提供识别信息，`Description` 放置辅助说明，`Extra` 在右上角构建操作或状态；不需要边框时调用 `Borderless()`。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

var plan bool

func page(c *ui.Context) {
    ui.CheckCard(c, "团队版", &plan).
        AvatarIcon(icon.Team).Description("适合多人协作").
        Extra(func() { ui.Icon(c, icon.More).Size(16) }).
        Borderless().Show()
}
```

## 主题与样式

- `CheckCard` 使用 Pro 组件卡片令牌；Ant Design、Element Plus、shadcn 会分别呈现选中边框/角标或右上角选择指示器。
- `Size`、`Small`、`Large` 控制卡片尺寸；`Borderless` 只去掉普通边框，选中状态仍保留主题强调。
- `ConfigProvider.Defaults("CheckCard", ...)` 支持 `size` 和 `bordered`；组的默认尺寸也读取这个配置。

## 注意事项

- 把绑定的 `bool`、字符串或字符串切片放在稳定状态中；卡片每帧重建不会丢失它们。
- `CheckCardGroup` 应只使用 `Value` 或 `Values` 其中一种；多选组的 `Values` 会按选项顺序重新生成切片。
- 大量选项会带来布局和可访问性负担；应考虑 Select、Radio 或分页列表。
- 卡片具有 checkbox/radio 语义并支持 Enter、空格；有描述时会作为辅助说明传给无障碍树。

## 示例代码

<!--@include: ./_gen/checkcard-examples.md-->

## API

<!--@include: ./_gen/checkcard-api.md-->

## 相关链接

- [源码 ui/pro2_checkcard.go](https://github.com/tangoui/tango/blob/main/ui/pro2_checkcard.go)
- [ui Go API](/reference/api/ui)
- [Radio 单选](/components/radio)、[Checkbox 多选](/components/checkbox)、[ProCard](/components/procard)
