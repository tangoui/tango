---
title: VirtualSelect 虚拟化选择器
---
# VirtualSelect 虚拟化选择器

只构建下拉列表中可见行的选择器，适合数万到数十万条选项，也支持单选和多选。

## 何时使用

- 选项数量很大，需要保持打开、滚动和键盘操作流畅时。
- 需要搜索、远程查询、分组或自定义选项行，同时希望使用虚拟列表时。
- 选项很少且只需要普通下拉选择时，用 [Select 选择器](/components/select)。
- 需要输入自由文本标签而不是从选项选择时，用 [InputTag 标签输入](/components/inputtag)。

## 在线体验

<GalleryPreview component="VirtualSelect" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import "github.com/tangoui/tango/ui"

var city string

var cities = []ui.Option{
    {Label: "北京", Value: "bj"},
    {Label: "上海", Value: "sh"},
    {Label: "广州", Value: "gz"},
}

func page(c *ui.Context) {
    ui.VirtualSelect(c, &city, cities).
        Placeholder("请选择城市").Filterable().AllowClear().Width(240)
}
```

单选绑定 `*string`，`Option.Label` 是显示文本，`Option.Value` 是保存值。`Filterable()` 开启按标签过滤，`AllowClear()` 允许清空。

## 常见用法

### 多选与数量限制

把第一个参数设为 `nil`，再调用 `Multiple` 绑定 `*[]string`。`MaxCount` 限制选中数量，`MaxTagCount` 只折叠显示的标签，不改变实际值。

```go
import "github.com/tangoui/tango/ui"

var selectedCities = []string{"bj", "sh"}

func page(c *ui.Context) {
    ui.VirtualSelect(c, nil, ui.Opts("北京", "上海", "广州", "深圳")).
        Multiple(&selectedCities).MaxCount(3).MaxTagCount(2).
        Placeholder("最多选择 3 个城市").Filterable()
}
```

### 分组与自定义选项行

`Groups` 接收 `[]ui.VirtualOptionGroup`，组标题不可选择。`OptionRender` 只替换行内容，选中标记、键盘高亮和禁用状态仍由组件绘制。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var groupValue string

var languageGroups = []ui.VirtualOptionGroup{
    {Label: "编译型", Options: ui.Opts("Go", "Rust")},
    {Label: "脚本型", Options: ui.Opts("JavaScript", "Python")},
}

func page(c *ui.Context) {
    ui.VirtualSelect(c, &groupValue, nil).Groups(languageGroups).
        OptionRender(func(o ui.Option, s ui.VirtualOptionState) {
            ui.Text(c, o.Label).Layout(layout.Grow(1))
            ui.Text(c, o.Value).Tertiary()
        }).Placeholder("选择语言")
}
```

### 远程搜索与防抖

`Remote` 在用户输入停止后调用查询函数，默认防抖时间为 300 毫秒；应用应在回调中更新选项数据，并在请求期间用 `Loading(true)` 显示进度。远程模式会关闭本地标签过滤。

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
)

var (
    remoteCity   string
    remoteCities []ui.Option
    remoteQuery  string
)

func page(c *ui.Context) {
    ui.VirtualSelect(c, &remoteCity, remoteCities).
        Remote(func(q string) {
            remoteQuery = q
            // 请求完成后替换 remoteCities，设置 Loading(false)，并调用 c.Invalidate()。
        }).Debounce(500 * time.Millisecond).Loading(remoteQuery != "").
        Placeholder("远程搜索城市").Width(260)
}
```

### 创建新条目

`AllowCreate` 会把没有精确匹配的输入作为新选项；用 `Created()` 读取本帧创建的值，再由应用决定是否持久化。

```go
import "github.com/tangoui/tango/ui"

var (
    tags    []string
    newTag  string
)

func page(c *ui.Context) {
    w := ui.VirtualSelect(c, nil, ui.Opts("Go", "Rust", "Zig")).
        Multiple(&tags).AllowCreate().Placeholder("输入后按 Enter")
    if value, ok := w.Created(); ok {
        newTag = value
    }
    ui.Text(c, "本帧新建："+newTag)
}
```

### 自定义虚拟列表尺寸

`ItemHeight` 设置固定选项行高，自定义行内容需要放得下。`ListHeight` 设置下拉视口的最大高度，默认 256，Element Plus 主题下为 274，单位为逻辑像素。`DataVersion` 用于原地修改选项切片后使缓存失效。

```go
import "github.com/tangoui/tango/ui"

var (
    manyOptions []ui.Option
    optionsVersion int
    chosen string
)

func page(c *ui.Context) {
    ui.VirtualSelect(c, &chosen, manyOptions).ItemHeight(36).ListHeight(320).
        DataVersion(optionsVersion).Placeholder("大量选项")
}
```

## 主题与样式

- VirtualSelect 的触发器、标签、下拉面板和选项行沿用 `theme.Components.Select`（`SelectToken`）；三套风格只改变令牌和外观，不改变虚拟化行为。
- 可关注 `OptionHeight`、`OptionPadding`、`OptionActiveBg`、`OptionSelectedBg`、`PopupBg`、`PopupRadius` 和 `PopupShadow` 等令牌。
- 选择器默认值使用 `ConfigProvider.Defaults("Select", ui.Props{...})`，支持 `size`、`allowClear`、`showSearch`、`placeholder`、`maxTagCount`、`listHeight` 和 `debounce`；组件上的显式方法优先。

## 注意事项

- **状态**：选中的值由 `*string` 或 `*[]string` 保存；下拉打开状态、查询词和滚动位置由组件按调用位置保存。循环渲染时用 `ui.Key` 保持每个控件身份稳定。
- **大数据量**：VirtualSelect 只构建可见行，但选项切片仍由应用持有；更新切片内容后递增 `DataVersion`，不要在每帧反复生成十万条数据。
- **远程查询**：远程回调可能延迟返回；请求完成后更新选项、关闭 loading 并调用 `c.Invalidate()`。需要避免旧请求覆盖新查询结果。
- **无障碍**：用 `A11yLabel` 命名选择器；打开后可用方向键移动、Enter 选择、Escape 关闭，分组标题不会进入选择顺序。
- **截图与测试**：`Open()` 和 `IsOpen()` 是强制展开与读取状态的 API，适合 gallery、截图和测试，不建议让生产界面永久强制展开。

## 示例代码

<!--@include: ./_gen/virtualselect-examples.md-->

## API

<!--@include: ./_gen/virtualselect-api.md-->

## 相关链接

- [源码 ui/fill_virtual_select.go](https://github.com/tangoui/tango/blob/main/ui/fill_virtual_select.go)
- [ui Go API](/reference/api/ui#VirtualSelect)
- [Select 选择器](/components/select)、[MultiSelect 多选](/components/multiselect)、[InputTag 标签输入](/components/inputtag)
