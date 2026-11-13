---
title: Select 选择器
---
# Select 选择器

从一组选项中选择一个值。选项使用 []ui.Option，ui.Opts 可以快速从字符串创建选项。

## 在线体验

展开菜单，选择一项；也可以输入搜索词或点击清空。 工具栏支持 Ant Design、Element Plus、shadcn 以及深浅色切换；重置会恢复初始状态。

<ComponentPreview component="select" title="Select 选择器" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
    "github.com/tangoui/tango/ui"
)

var language string

func page(c *ui.Context) {
    ui.Select(c, &language, ui.Opts("Go", "TypeScript", "Python")).
        Placeholder("Choose a language").
        Searchable().
        AllowClear()
}
```

## 常用参数与方法

| 参数 / 方法 | 说明 |
| --- | --- |
| `val *string` | 当前选项的 Key |
| `opts []ui.Option` | 可选项列表 |
| `Placeholder(string)` | 未选择时的占位文本 |
| `Searchable()` | 启用搜索 |
| `AllowClear()` | 允许清空当前选择 |
| `Disabled(bool)` | 禁用选择器 |
| `Changed() bool` | 当前帧是否发生选择变化 |

## 使用说明

自定义选项时使用 ui.Option 的 Key 和 Label 区分存储值与显示文字。键盘 Enter 展开，方向键移动，Enter 选择，Escape 关闭。

## 实现与更多示例

- [源码](https://github.com/tangoui/tango/blob/main/ui/select.go)
- [完整组件截图目录](/guide/components)
- [GUI 组件在线体验总览](/components/)
- [ui Go API](/reference/api/ui)
