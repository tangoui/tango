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
| `val *string` | 当前选中项的 Value |
| `opts []ui.Option` | 可选项列表 |
| `Placeholder(string)` | 未选择时的占位文本 |
| `Searchable()` | 启用搜索 |
| `AllowClear()` | 允许清空当前选择 |
| `Disabled(bool)` | 禁用选择器 |
| `Changed() bool` | 当前帧是否发生选择变化 |
| `Virtual(bool)` | 控制普通选项列表是否启用虚拟滚动。省略时超过 100 项自动启用 |
| `ListHeight(float32)` | 设置虚拟下拉面板的最大高度 |
| `DataVersion(int)` | 在原地修改虚拟列表数据后使选项缓存失效 |

## 使用说明

自定义选项时使用 ui.Option 的 Value 和 Label 区分存储值与显示文字。键盘 Enter 展开，方向键移动，Enter 选择，Escape 关闭。

虚拟下拉只构建视口附近的选项行，按切片身份、长度和版本缓存选项。原地修改选项的 `Label`、`Value` 或 `Disabled` 等字段后，递增 `DataVersion`，或传入新切片。普通选项列表可用 `Virtual(true)` 强制启用、用 `Virtual(false)` 关闭。分组、远程搜索和自定义选项行等功能会使用虚拟下拉。虚拟下拉的最大高度默认是 256，Element Plus 主题下为 274，单位为逻辑像素。

## 更多场景

官方 gallery 里这个组件的全部示例（基础、进阶与组合场景），可切换场景、主题和深浅色。

<GalleryPreview component="Select" />

## 实现与更多示例

- [源码](https://github.com/tangoui/tango/blob/main/ui/select.go)
- [组件截图目录](/components/screenshots)
- [组件总览](/components/)
- [ui Go API](/reference/api/ui)

## 全部示例代码

<!--@include: ./_gen/select-examples.md-->

## 完整 API

<!--@include: ./_gen/select-api.md-->
