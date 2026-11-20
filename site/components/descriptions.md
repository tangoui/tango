---
title: Descriptions 描述列表
---
# Descriptions 描述列表

成组展示多个只读字段，每个字段是「标签：内容」，按列排成网格，常见于详情页。

## 何时使用

- 展示一条记录的详情，例如订单信息、用户资料、资源配置。
- 字段较多、需要按栅格分列对齐时；也可以加边框做成表格式的详情。
- 需要多条记录逐行对比时，用 [Table 表格](/components/table)（记录少字段多时用它的 `Transposed()` 转置）；字段需要编辑时用表单组件；需要按 Pro 规范的 `valueType` 自动格式化、行内编辑的详情，用 `ProDescriptions`。

## 在线体验

切换场景可以看到基础用法、带边框、尺寸、垂直布局，以及随窗口断点变化的列数。

<GalleryPreview component="Descriptions" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
    "github.com/tangoui/tango/ui"
)

var profile = []ui.DescItem{
    {Label: "用户名", Value: "曲丽丽"},
    {Label: "手机号", Value: "1810000000"},
    {Label: "居住地", Value: "杭州市西湖区"},
    {Label: "备注", Value: "无"},
    {Label: "联系地址", Value: "浙江省杭州市西湖区万塘路 18 号", Span: 2},
}

func page(c *ui.Context) {
    ui.Descriptions(c, profile).Title("用户信息")
}
```

`ui.Descriptions(c, items)` 默认无边框，列数随窗口宽度变化，`XS` 为 1 列、`SM` 为 2 列、`MD` 起为 3 列。每个 `DescItem` 是一对 `Label` / `Value`，`Span` 让一项占多列，最后一行的末项会填满剩余列。内容在链式配置完成后构建。

## 常见用法

以下片段省略了 `import`。

### 带边框与标题区操作

`Bordered()` 画出表格式的边框，标签列带底色；`Title` 设置标题，`Extra` 在标题行右侧放操作按钮。

```go
var order = []ui.DescItem{
    {Label: "产品", Value: "云数据库"},
    {Label: "计费模式", Value: "预付费"},
    {Label: "自动续费", Value: "是"},
    {Label: "下单时间", Value: "2018-04-24 18:00:00"},
    {Label: "使用时间", Value: "2019-04-24 18:00:00", Span: 2},
}

var editOpen bool

func borderedDesc(c *ui.Context) {
    ui.Descriptions(c, order).Title("订单详情").Bordered().Extra(func() {
        if ui.Button(c, "编辑").Primary().Clicked() {
            editOpen = true
        }
    })
}
```

### 自定义内容

`DescItem.Render` 替代 `Value`，在内容区构建任意组件，例如状态徽标、标签或链接。

```go
func customDesc(c *ui.Context) {
    ui.Descriptions(c, []ui.DescItem{
        {Label: "实例", Value: "db-prod-01"},
        {Label: "状态", Render: func() { ui.Badge(c).Status("processing", "运行中") }},
        {Label: "标签", Render: func() {
            ui.Tag(c, "生产").Color("error")
            ui.Tag(c, "华东 1").Color("processing")
        }},
    }).Title("实例信息").Bordered()
}
```

### 尺寸、列数与垂直布局

`Size(ui.Middle)` / `Size(ui.Small)` 收紧间距（Ant Design 和 shadcn 默认 Large，Element Plus 默认 Middle）；`Column(n)` 设置列数；`Vertical()` 把标签放在内容上方；`NoColon()` 去掉无边框模式下标签后的冒号；`LabelWidth(px)` 设置横向布局的标签宽度。

```go
func compactDesc(c *ui.Context) {
    ui.Descriptions(c, profile).Title("紧凑 · 4 列").Size(ui.Small).Column(4)
    ui.Descriptions(c, profile[:4]).Title("垂直布局").Bordered().Vertical()
    ui.Descriptions(c, profile).NoColon().LabelWidth(96)
}
```

### 响应式列数

`ColumnAt(bp, n)` 从断点 `bp` 起使用 `n` 列，窗口宽度命中的最大断点生效，`Column(n)` 可设置兜底列数。`Columns(map[ui.Breakpoint]int)` 一次设置多个断点。单项可通过 `DescItem.Spans` 按断点设置跨度，或用 `Filled: true` 占满当前行的剩余列并结束该行。

```go
func responsiveDesc(c *ui.Context) {
    ui.Descriptions(c, profile).Title("响应式列数").Bordered().
        ColumnAt(ui.XS, 1).ColumnAt(ui.SM, 2).ColumnAt(ui.LG, 3)
}
```

## 主题与样式

| 风格 | 差异 |
| --- | --- |
| Ant Design | 标签为三级文字色；带边框时标签列为浅灰底（`FillAlter`），外框为大圆角。 |
| Element Plus | 对照 `el-descriptions`：标签底色更深（`FillTertiary`）、标签为次要文字色，外框直角，单元格内边距更紧凑，标签与内容之间留 16px。 |
| shadcn | 沿用 Ant Design 的结构与间距，颜色取 shadcn 主题的中性色。 |

组件令牌在 `theme.Components.Descriptions`（`theme.DescriptionsToken`）：`LabelBg`、`LabelColor`、`ContentColor`、`TitleColor`、`TitleFontSize`、`BorderColor`、`Radius`、`ItemPaddingBottom`，以及带边框时三档尺寸的 `CellPaddingBlock` / `CellPaddingInline`（`MD`、`SM` 后缀对应 Middle 和 Small）。单个列表的标签底色也可以用 `LabelBg(color)` 直接覆盖。

`ConfigProvider.Defaults("Descriptions", ...)` 支持的键：`"column"`（int 或 `map[ui.Breakpoint]int`）、`"bordered"`、`"size"`、`"layout"`（`"vertical"`）、`"colon"`（bool）。

```go
func descDefaults(c *ui.Context) {
    ui.ConfigProvider(c).
        Defaults("Descriptions", ui.Props{"bordered": true, "column": 2}).
        Body(func() {
            ui.Descriptions(c, profile).Title("全局默认带边框、2 列")
        })
}
```

## 注意事项

- **状态**：描述列表是纯展示组件，不保存交互状态；`items` 可以每帧重新构造，也可以放在包级变量里复用。
- **列数与 Span**：一项的 `Span` 超过当前行剩余列数时整项换到下一行（`Span` 大于列数时按列数算）；列表最后一项会把 `Span` 补满剩余列；中间某行没占满时，该行各项按 `Span` 比例拉伸占满整行，列线就不再与其他行对齐，所以最好让每行的 `Span` 之和等于列数。
- **长内容**：内容会在单元格内换行；很长的文本（如配置说明）建议单独占一整行（`Span` 等于列数）。
- **RTL**：在 RTL 布局中标签与内容的顺序、对齐自动镜像。

## 示例代码

<!--@include: ./_gen/descriptions-examples.md-->

## API

<!--@include: ./_gen/descriptions-api.md-->

## 相关链接

- 源码：[ui/data_card.go](https://github.com/tangoui/tango/blob/main/ui/data_card.go)
- [ui Go API 参考](/reference/api/ui)
- [Card 卡片](/components/card)、[Table 表格](/components/table)、[List 列表](/components/list)
- [组件总览](/components/)、[组件截图目录](/components/screenshots)
