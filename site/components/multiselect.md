---
title: Select 多选（标签折叠）
---
# Select 多选（标签折叠）

从一组选项中选择多个值，并在选择器中以可删除标签展示已选项。

## 何时使用

- 需要从有限的候选项中选择多个值，例如城市、成员或权限时。
- 选项很多但仍需要 Select 的多选交互时，可启用虚拟下拉；极大数据集也可以直接用 [VirtualSelect 虚拟化选择器](/components/virtualselect)。
- 只允许一个值时，用 [Select 选择器](/components/select)。
- 需要输入不受候选项限制的标签时，用 [InputTag 标签输入](/components/inputtag)。

## 在线体验

<GalleryPreview component="MultiSelect" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import "github.com/tangoui/tango/ui"

var cities = []string{"bj", "sh"}

var cityOptions = []ui.Option{
    {Label: "北京", Value: "bj"},
    {Label: "上海", Value: "sh"},
    {Label: "广州", Value: "gz"},
}

func page(c *ui.Context) {
    ui.MultiSelect(c, &cities, cityOptions).
        Placeholder("请选择城市").AllowClear().A11yLabel("城市")
}
```

`MultiSelect` 绑定 `*[]string`，保存的是 `Option.Value`，标签显示 `Option.Label`。点击已选标签的关闭按钮可以移除单项，`AllowClear` 可以一次清空。

## 常见用法

### 搜索与标签折叠

`Searchable` 允许在选择器中输入过滤，`MaxTagCount` 只控制可见标签数量，`MaxCount` 则限制实际选择数量。

```go
import "github.com/tangoui/tango/ui"

var members = []string{"alice", "bob"}

func page(c *ui.Context) {
    ui.MultiSelect(c, &members, ui.Opts("alice", "bob", "carol", "dave")).
        Searchable().MaxTagCount(1).MaxCount(3).
        Placeholder("选择成员").A11yLabel("成员")
}
```

### 分组与自定义选项

`Groups` 使用 `[]ui.OptionGroup` 显示分组，传给构造函数的普通选项在使用分组时会被忽略。`OptionRender` 可以自定义行内容，但选择标记和高亮仍由组件维护。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var role string

var roleGroups = []ui.OptionGroup{
    {Label: "研发", Options: ui.Opts("后端", "前端")},
    {Label: "产品", Options: ui.Opts("产品经理", "设计师")},
}

func page(c *ui.Context) {
    ui.MultiSelect(c, &[]string{}, nil).Groups(roleGroups).
        OptionRender(func(o ui.Option, s ui.OptionState) {
            ui.Text(c, o.Label).Layout(layout.Grow(1))
            if s.Selected {
                ui.Text(c, "已选").Tertiary()
            }
        }).A11yLabel("角色")
    _ = role
}
```

### 创建新值

`AllowCreate` 隐含开启搜索。用户输入未精确匹配的值并确认后，可以通过 `Created()` 读取本帧创建的值；绑定数组也会包含该值。

```go
import "github.com/tangoui/tango/ui"

var (
    labels  []string
    created string
)

func page(c *ui.Context) {
    w := ui.MultiSelect(c, &labels, ui.Opts("前端", "后端")).
        AllowCreate().Placeholder("输入标签并按 Enter")
    if value, ok := w.Created(); ok {
        created = value
    }
    ui.Text(c, "最近创建："+created)
}
```

### 远程搜索与加载

`Remote` 接收防抖后的查询字符串，应用应替换选项并配合 `Loading` 展示请求过程。`Debounce` 可覆盖默认的 300 毫秒延迟。

```go
import (
    "time"

    "github.com/tangoui/tango/ui"
)

var (
    remoteMembers []string
    remoteOptions []ui.Option
    remoteLoading bool
)

func page(c *ui.Context) {
    ui.MultiSelect(c, &remoteMembers, remoteOptions).
        Remote(func(q string) {
            // 根据 q 请求数据，通过通道把结果交回 UI 线程，再替换 remoteOptions。
            // 后台请求完成后可调用事先取得的 c.Invalidator() 返回值唤醒 UI。
        }).Debounce(500 * time.Millisecond).Loading(remoteLoading).
        Searchable().Placeholder("远程搜索成员")
}
```

### 虚拟列表与数据更新

当选项多于 100 项时默认会使用虚拟下拉；可用 `Virtual(true)` 强制开启，`Virtual(false)` 关闭。原地修改选项切片后递增 `DataVersion`，让缓存重新建立。

```go
import "github.com/tangoui/tango/ui"

var (
    allOptions []ui.Option
    dataVersion int
    selected []string
)

func page(c *ui.Context) {
    ui.MultiSelect(c, &selected, allOptions).Virtual(true).
        ListHeight(320).DataVersion(dataVersion).A11yLabel("大量选项")
}
```

## 主题与样式

- MultiSelect 与 Select、Cascader、TreeSelect 共用 `theme.Components.Select`（`SelectToken`）。标签相关令牌包括 `MultipleItemBg`、`MultipleItemColor`、`MultipleItemHeight`，下拉相关令牌包括 `OptionHeight`、`OptionSelectedBg` 和 `PopupBg`。
- `Variant` 接收 `ui.FieldOutlined`、`ui.FieldFilled`、`ui.FieldBorderless` 或 `ui.FieldUnderlined`，也可使用 `Size`、`Status`、`PrefixText` 和 `SuffixIcon`。
- `ConfigProvider.Defaults("Select", ui.Props{...})` 支持 `size`、`allowClear`、`showSearch`、`placeholder`、`maxTagCount`、`listHeight` 和 `debounce`；显式链式调用优先。

## 注意事项

- **状态**：把 `[]string` 放在页面函数外或 `ui.State` 中；组件会写入绑定切片，但不会替应用持久化远程选项。
- **性能**：`ListHeight(px)` 限制下拉列表的可见高度。自定义复杂行时避免每次构建都做昂贵计算。
- **键盘**：打开后方向键移动，Enter 切换选项，Escape 关闭；搜索模式下 Backspace 在搜索词为空时删除最后一个标签。
- **无障碍**：独立使用时设置 `A11yLabel`，表单中可由 `FormItem` 标签命名；禁用选项不会被选中。
- **事件**：`Changed()` 和 `Created()` 都表示当前帧事件，若要记录最终选择，应复制 `[]string` 或在应用状态中保存。

## 示例代码

<!--@include: ./_gen/multiselect-examples.md-->

## API

<!--@include: ./_gen/multiselect-api.md-->

## 相关链接

- [源码 ui/entry_multiselect.go](https://github.com/tangoui/tango/blob/main/ui/entry_multiselect.go)、[ui/p2b_select.go](https://github.com/tangoui/tango/blob/main/ui/p2b_select.go)
- [ui Go API](/reference/api/ui#MultiSelect)
- [Select 选择器](/components/select)、[VirtualSelect 虚拟化选择器](/components/virtualselect)、[InputTag 标签输入](/components/inputtag)
