---
title: Command 命令面板
---
# Command 命令面板

提供可搜索、可分组、支持键盘上下选择的命令面板，适合应用内操作和快速跳转。

## 何时使用

- 命令数量较多，需要按标签、值或关键词快速过滤时使用。
- 用户熟悉快捷键并希望用键盘完成选择时使用。
- 只有少量固定选项的表单选择，使用 [Select 选择器](/components/select)；需要层级导航时使用 [Menu 导航菜单](/components/menu)。
- 命令需要在模态层中打开时，先用 [Modal 对话框](/components/modal) 管理打开状态，再在内容中放置 Command。

## 在线体验

<GalleryPreview component="Command" />

## 基本用法

`Command` 接收 `[]ui.CommandItem`，默认在 `Label`、`Value` 和 `Keywords` 中做不区分大小写的匹配，并按匹配程度排序。按 Enter 或点击条目后，用 `Selected()` 读取本帧选中的项目。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var commandPicked string

func commandPage(c *ui.Context) {
    items := []ui.CommandItem{
        {Value: "new", Label: "新建文件"},
        {Value: "open", Label: "打开项目"},
        {Value: "settings", Label: "设置"},
    }
    palette := ui.Command(c, items).Layout(layout.W(360))
    if item, ok := palette.Selected(); ok {
        commandPicked = item.Value
    }
    ui.Text(c, "已选择："+commandPicked)
}
```

## 常见用法

### 分组、图标和快捷键

相同 `Group` 的条目会归在一起。没有查询时按组首次出现的顺序排列，搜索时按各组排名最高的条目排列。`Icon` 放置前置图标，`Shortcut` 是右侧的快捷键提示；执行逻辑由 `Selected()` 处理。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/icon"
)

func groupedCommandPage(c *ui.Context) {
    ui.Command(c, []ui.CommandItem{
        {Value: "calendar", Label: "日历", Group: "建议", Icon: icon.Calendar},
        {Value: "profile", Label: "个人资料", Group: "设置", Icon: icon.User, Shortcut: "⌘P"},
        {Value: "settings", Label: "设置", Group: "设置", Icon: icon.Setting, Shortcut: "⌘S"},
    }).Shadow()
}
```

### 关键词搜索和禁用项

`Keywords` 用于补充搜索词；`Disabled` 项仍可展示但不会被键盘或点击选中。

```go
import "github.com/tangoui/tango/ui"

func searchableCommandPage(c *ui.Context) {
    ui.Command(c, []ui.CommandItem{
        {Value: "docs", Label: "打开文档", Keywords: []string{"帮助", "指南"}},
        {Value: "cache", Label: "清理缓存", Disabled: true, Keywords: []string{"维护"}},
    }).Placeholder("搜索命令").Empty("没有匹配的命令")
}
```

### 限制面板高度

`MaxHeight` 只限制结果列表的高度；结果超过高度时由面板内部的 Scroll 处理，面板宽度和位置仍可用 `Layout` 设置。

```go
import (
    "fmt"

    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func tallCommandPage(c *ui.Context) {
    items := make([]ui.CommandItem, 40)
    for i := range items {
        items[i] = ui.CommandItem{Value: fmt.Sprintf("item-%d", i), Label: fmt.Sprintf("命令 %d", i+1)}
    }
    ui.Command(c, items).MaxHeight(220).Layout(layout.W(420))
}
```

### 读取查询文本

`Query()` 返回当前搜索框内容，适合在面板旁显示调试信息或将选择结果映射到应用状态。

```go
import "github.com/tangoui/tango/ui"

func queryCommandPage(c *ui.Context) {
    palette := ui.Command(c, []ui.CommandItem{{Value: "build", Label: "构建项目"}})
    ui.Text(c, "查询："+palette.Query()).Tertiary()
}
```

## 主题与样式

- Ant Design：面板使用悬浮容器、分隔线和主题的选中背景。
- Element Plus：仍保留分组、禁用和键盘选择行为，颜色来自 Element 主题令牌。
- shadcn：Command 是 shadcn 风格的搜索框、列表项和选中背景；`Shadow()` 只增加浮层阴影。
- `ConfigProvider.Defaults("Command", ui.Props{"placeholder": "搜索", "emptyText": "无结果"})` 可设置默认提示文案。还可设置 `loop`（默认 `false`）、`shouldFilter`（默认 `true`）和 `vimBindings`（默认 `true`）。

## 注意事项

- **状态**：Command 内部保留查询和活动行；业务选中值要在 `Selected()` 成功后写入页面外部状态。
- **键盘**。输入框聚焦后使用 ↑ / ↓ 移动、Home / End 跳到首尾、Enter 选择。默认也支持 Ctrl+N/J 向下和 Ctrl+P/K 向上，移动时跳过禁用项。
- **性能**：大量条目应提供简洁的 `Label`、稳定的 `Value` 和必要的关键词，避免在每帧生成昂贵的字符串或图标对象。
- **大列表**。显示的条目多于 100 项时自动虚拟化，只构建可见行和活动行，键盘移动时列表跟随活动行。无障碍树只暴露已构建的条目。用 `Virtual(false)` 构建所有行，或用 `Virtual(true)` 强制开启虚拟化。
- **原地更新条目**。虚拟化且未设置 `Filter` 时会复用匹配结果、顺序、布局和宽度。原地修改同一切片且长度不变时，给 `DataVersion(version)` 传入新版本，或替换切片，下一帧会重新计算。
- **无障碍**：组件提供 combobox、listbox 和 option 语义；`Shortcut` 只是可读提示，真正的全局快捷键需要单独注册。
- **RTL**：列表和快捷键提示会按主题方向布局，`CommandItem.Value` 不应依赖视觉顺序。

## 示例代码

<!--@include: ./_gen/command-examples.md-->

## API

<!--@include: ./_gen/command-api.md-->

## 相关链接

- [源码 shad_command.go](https://github.com/tangoui/tango/blob/main/ui/shad_command.go)
- [ui Go API](/reference/api/ui)
- [Toggle 切换按钮](/components/toggle)、[Kbd 键帽](/components/kbd)、[Select 选择器](/components/select)
