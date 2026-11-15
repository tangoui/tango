---
title: VirtualList 虚拟列表
---
# VirtualList 虚拟列表

固定行高的高性能滚动列表，只构建视口及少量缓冲区内的行，适合数万到数十万条数据。

## 何时使用

- 数据量很大、每行高度固定，并且只需要垂直滚动浏览时。
- 行内有复选框、按钮等带状态控件，需要随着数据索引稳定保存时。
- 行高可变或需要分组标题时用 [Listy 高性能列表](/components/listy)；需要列、排序和筛选时用 [Table 表格](/components/table)。

## 在线体验

示例展示十万条记录、按索引跳转、行内复选框以及本帧实际构建的行范围。

<GalleryPreview component="VirtualList" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
    "fmt"

    "github.com/tangoui/tango/ui"
)

var names = []string{"北京", "上海", "广州", "深圳"}

func page(c *ui.Context) {
    ui.VirtualList(c, len(names), 40, func(i int) {
        ui.Text(c, fmt.Sprintf("第 %d 行：%s", i+1, names[i]))
    }).Height(240).A11yLabel("城市列表")
}
```

`VirtualList(c, n, itemHeight, fn)` 声明 `n` 个固定高度的行，`fn(i)` 只在当前帧需要构建行 `i` 时调用。必须给视口设置高度（可以通过 `Height` 或 `Layout`），否则列表会跟随父布局的可用高度。

## 常见用法

以下片段沿用基本用法里的 `names` 数据，省略了 `import`。

### 行内控件与稳定状态

每一行都在自己的索引作用域中构建。应用状态仍应放在页面变量里；例如复选框的值用数据索引保存，而不要在渲染函数中重新初始化。

```go
var selected = map[int]bool{}

func selectableList(c *ui.Context) {
    ui.VirtualList(c, 100000, 40, func(i int) {
        checked := selected[i]
        if ui.Checkbox(c, "", &checked).
            A11yLabel(fmt.Sprintf("选择第 %d 行", i+1)).Changed() {
            selected[i] = checked
        }
        ui.Text(c, fmt.Sprintf("记录 #%06d", i))
    }).Height(320)
}
```

### 跳转、定位与范围

`ScrollToIndex` 把目标行放到顶部，`ScrollIntoView` 只做让目标行完全可见所需的最小滚动。`Range` 返回本帧构建的半开区间 `[first, end)`，可用于显示调试信息或配套加载逻辑。

```go
var jumpTo = -1

func jumpableList(c *ui.Context) {
    if ui.Button(c, "跳到第 50001 条").Clicked() {
        jumpTo = 50000
    }
    list := ui.VirtualList(c, 100000, 36, func(i int) {
        ui.Text(c, fmt.Sprintf("记录 %d", i+1))
    }).Height(280)
    if jumpTo >= 0 {
        list.ScrollToIndex(jumpTo)
        jumpTo = -1
    }
    first, end := list.Range()
    ui.Text(c, fmt.Sprintf("本帧构建 %d–%d 行", first+1, end)).Tertiary()
}
```

### 调整缓冲区与布局

默认在视口上下各额外构建 3 行。行内容较重、快速滚动时希望更少构建可以降低 `Overscan`；行内绘制较轻、希望滚动更平滑可以提高它。`Layout` 可同时设置宽度、最大高度等布局选项。

```go
func tunedList(c *ui.Context) {
    ui.VirtualList(c, len(names), 40, func(i int) {
        ui.Text(c, names[i])
    }).Height(260).Overscan(6).Layout(layout.W(360))
}
```

### 处理空列表与滚动偏移

`n` 为 0 时不会调用行回调；`Offset` 可读取当前滚动位置，适合在页面状态栏中展示位置或在外部导航中同步滚动。

```go
func listStatus(c *ui.Context) {
    list := ui.VirtualList(c, len(names), 40, func(i int) {
        ui.Text(c, names[i])
    }).Height(240)
    ui.Text(c, fmt.Sprintf("滚动位置 %.0f px", list.Offset())).Tertiary()
}
```

## 主题与样式

VirtualList 复用滚动容器、列表和行内容的主题令牌，不单独提供组件令牌或 `ConfigProvider.Defaults` 键。行背景、分隔线和内边距应在 `fn(i)` 中用 `Bg`、`Paint`、`Layout` 等方式表达，列表本身只负责虚拟化和滚动。

## 注意事项

- **固定行高**：`itemHeight` 是虚拟布局的依据；回调内不要构建会改变行高的内容。可变高度内容改用 `Listy`。
- **状态放在哪**：页码、选择集合、数据切片等放在页面变量或 `ui.State` 中，不要在 `fn` 里声明后每帧重置。目标索引变化后调用 `ScrollToIndex` 或 `ScrollIntoView`。
- **Key 与数据**：组件会按行索引建立身份作用域；若行内还有自己的动态集合，仍需用 `ui.Key(c, 稳定键, fn)` 区分数据项。
- **性能**：`Overscan` 越大，滚动时构建的行越多；行内尽量避免昂贵计算，并把大图等资源缓存到应用状态。
- **无障碍**：用 `A11yLabel` 给没有可见标题的列表命名。组件暴露完整数据集的行数、位置和集合大小；行内交互控件仍需自己的可访问名称。
- **RTL**：滚动方向、列表语义和布局会遵循当前 RTL 设置，行内容由应用布局自动镜像。

## 示例代码

<!--@include: ./_gen/virtuallist-examples.md-->

## API

<!--@include: ./_gen/virtuallist-api.md-->

## 相关链接

- 源码：[ui/more_virtual.go](https://github.com/tangoui/tango/blob/main/ui/more_virtual.go)
- [ui Go API 参考](/reference/api/ui)
- [Listy 高性能列表](/components/listy)、[InfiniteScroll 无限滚动](/components/infinitescroll)、[Table 表格](/components/table)
- [组件总览](/components/)、[组件截图目录](/components/screenshots)
