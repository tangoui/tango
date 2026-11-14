---
title: Cascader 级联选择
---
# Cascader 级联选择

从一组有层级关系的选项中逐级选择，例如省 / 市 / 区、商品类目。下拉菜单按层级分列展开，选中后显示完整路径。

## 何时使用

- 数据是一棵树、且需要沿着一条路径逐级选择时（地区、组织、类目）。
- 选项层级较深，平铺到一个 Select 中会难以查找时。
- 需要从树中选择任意节点、并更关心节点本身而不是路径时，用 [TreeSelect 树选择](/components/treeselect)；没有层级关系时用 [Select 选择器](/components/select)。

## 在线体验

<GalleryPreview component="Cascader" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
	"strings"

	"github.com/tangoui/tango/ui"
)

var regions = []ui.CascaderOption{
	{Label: "浙江", Value: "zj", Children: []ui.CascaderOption{
		{Label: "杭州", Value: "hz", Children: []ui.CascaderOption{{Label: "西湖区", Value: "xh"}, {Label: "滨江区", Value: "bj"}}},
		{Label: "宁波", Value: "nb", Children: []ui.CascaderOption{{Label: "海曙区", Value: "hs"}}},
		{Label: "温州", Value: "wz", Disabled: true},
	}},
	{Label: "江苏", Value: "js", Children: []ui.CascaderOption{
		{Label: "南京", Value: "nj", Children: []ui.CascaderOption{{Label: "玄武区", Value: "xw"}}},
	}},
}

var area []string

func page(c *ui.Context) {
	ui.Cascader(c, &area, regions).Placeholder("请选择地区").AllowClear()
	ui.Text(c, "值："+strings.Join(area, " / "))
}
```

选项用 `[]ui.CascaderOption` 描述（`Label` 显示、`Value` 存储、`Children` 下一级、`Disabled` 禁用）。值绑定到 `*[]string`，是从根到叶子的 `Value` 路径，例如 `["zj", "hz", "xh"]`；输入框默认显示各级标签用 “ / ” 连接。

## 常见用法

### 选择任意一级与悬停展开

默认只有选到叶子才算完成选择。`ChangeOnSelect()` 允许点击任意一级就选中（例如只选到“浙江 / 杭州”）；`ExpandOnHover()` 让鼠标悬停时展开下一级，点击只用于选中。

```go
import (
	"github.com/tangoui/tango/ui"
)

var (
	tree = []ui.CascaderOption{
		{Label: "电子产品", Value: "e", Children: []ui.CascaderOption{
			{Label: "手机", Value: "phone"}, {Label: "电脑", Value: "pc"},
		}},
	}
	category []string
)

func page(c *ui.Context) {
	ui.Cascader(c, &category, tree).ChangeOnSelect().ExpandOnHover().Placeholder("请选择类目")
}
```

### 自定义显示内容

`DisplayRender` 自定义输入框里显示的文本（单选模式），函数拿到各级标签和对应的选项。下面只显示最后一级：

```go
import (
	"github.com/tangoui/tango/ui"
)

var (
	opts = []ui.CascaderOption{{Label: "浙江", Value: "zj", Children: []ui.CascaderOption{{Label: "杭州", Value: "hz"}}}}
	city []string
)

func page(c *ui.Context) {
	ui.Cascader(c, &city, opts).DisplayRender(func(labels []string, _ []ui.CascaderOption) string {
		return labels[len(labels)-1]
	})
}
```

### 搜索

`Searchable()` 允许输入关键字，默认按完整标签路径（如“浙江 / 杭州 / 西湖区”）不区分大小写匹配，结果以路径列表显示。`FilterMethod` 可替换匹配规则，函数收到输入内容和候选路径。

```go
import (
	"strings"

	"github.com/tangoui/tango/ui"
)

var (
	opts = []ui.CascaderOption{{Label: "浙江", Value: "zj", Children: []ui.CascaderOption{{Label: "杭州", Value: "hz"}}}}
	pick []string
)

func page(c *ui.Context) {
	ui.Cascader(c, &pick, opts).Searchable().Placeholder("搜索地区")
	ui.Cascader(c, &pick, opts).Searchable().FilterMethod(func(q string, path []ui.CascaderOption) bool {
		last := path[len(path)-1]
		return strings.HasPrefix(last.Value, q) // 按末级的值前缀匹配
	})
}
```

### 多选与勾选策略

`Multiple(&paths)` 切换为多选：每个选项前出现复选框，父子联动，选中项以标签显示，`MaxTagCount` 折叠多余标签。此时第二个参数可以传 `nil`。

`ShowCheckedStrategy` 决定显示哪些标签：`ui.ShowParent`（默认，父节点全选时只显示父节点）、`ui.ShowChild`（显示每个选中的叶子）、`ui.ShowAll`（父子都显示）。默认绑定值始终保存选中的叶子路径，`CheckedPaths()` 返回按策略折叠后的路径；`StrategyValue(true)` 让绑定值本身也按策略保存（与 antd 的 `value` 行为一致）。

```go
import (
	"github.com/tangoui/tango/ui"
)

var (
	opts = []ui.CascaderOption{
		{Label: "浙江", Value: "zj", Children: []ui.CascaderOption{{Label: "杭州", Value: "hz"}, {Label: "宁波", Value: "nb"}}},
		{Label: "江苏", Value: "js", Children: []ui.CascaderOption{{Label: "南京", Value: "nj"}}},
	}
	checked [][]string
)

func page(c *ui.Context) {
	w := ui.Cascader(c, nil, opts).Multiple(&checked).MaxTagCount(3).ShowCheckedStrategy(ui.ShowParent)
	if w.Changed() {
		_ = w.CheckedPaths() // 例如 [["zj"], ["js", "nj"]]
	}
}
```

### 动态加载

`LoadData` 开启懒加载：展开一个没有 `Children` 且不是 `IsLeaf` 的选项时调用函数，参数是该选项的值路径；在任意 goroutine 中调用 `done(子选项)` 即可，期间箭头位置显示加载图标；`done(nil)` 表示它是叶子。

```go
import (
	"github.com/tangoui/tango/ui"
)

var (
	roots = []ui.CascaderOption{{Label: "浙江", Value: "zj"}, {Label: "江苏", Value: "js"}}
	lazy  []string
)

func fetchChildren(path []string) []ui.CascaderOption {
	if len(path) >= 2 {
		return nil // 第三级没有子节点
	}
	return []ui.CascaderOption{{Label: path[len(path)-1] + "-1", Value: path[len(path)-1] + "1", IsLeaf: len(path) == 1}}
}

func page(c *ui.Context) {
	ui.Cascader(c, &lazy, roots).LoadData(func(path []string, done func([]ui.CascaderOption)) {
		go func() { done(fetchChildren(path)) }()
	})
}
```

### 内嵌面板

`CascaderPanel` 是不带输入框的级联面板，直接嵌在页面中，支持 `ChangeOnSelect`、`Multiple`、`LoadData`、`Height` 等同类设置。

```go
import (
	"strings"

	"github.com/tangoui/tango/ui"
)

var (
	opts  = []ui.CascaderOption{{Label: "浙江", Value: "zj", Children: []ui.CascaderOption{{Label: "杭州", Value: "hz"}}}}
	inner []string
)

func page(c *ui.Context) {
	ui.CascaderPanel(c, &inner, opts).ChangeOnSelect().A11yLabel("地区")
	ui.Text(c, "已选："+strings.Join(inner, " / "))
}
```

## 主题与样式

- 输入框外观与 Select 一致，支持 `Variant(ui.FieldFilled)` 等外观、`Size`、`Status("error" | "warning")`。
- 组件令牌：`theme.Components.Cascader`（`CascaderToken`），包括 `OptionHeight`、`ColumnWidth`、`DropdownHeight`、`OptionHoverBg`、`OptionSelectedBg`、`ColumnDividerColor` 等；三套风格的选中背景、列分隔线不同，切换风格无需改代码。
- `ConfigProvider.Defaults("Cascader", ui.Props{...})` 支持 `size`、`status`、`allowClear`、`showSearch`、`placeholder`、`expandTrigger`（`"hover"`）、`changeOnSelect`、`maxTagCount`、`showCheckedStrategy`、`strategyValue`。

## 注意事项

- **状态**：值就是你绑定的路径切片；菜单展开状态、搜索词由组件按调用位置保存。选项树在代码中变化后，不再存在的值会以原始值显示，必要时自行清空。
- **选项数据**：同一层级中 `Value` 必须唯一。选项树很大时把它放在包级变量或缓存中，避免每帧重新构造。
- **表单校验**：放进 [Form](/components/form) 时用 `ui.RulesOf` 绑定路径切片，规则用 `ui.RequiredItems[string]`（参数为提示文字）。
- **无障碍与键盘**：Enter / 空格打开，↑ / ↓ 在当前列移动，→ 展开下一级、← 返回上一级，Home / End 跳到首尾，Enter 选中；在表单项里自动以表单项标签命名，否则使用 `A11yLabel` 或占位符。
- **截图与测试**：`Open()`、`Expand(path...)` 用于强制展开菜单，只在截图和测试中使用。

## 示例代码

<!--@include: ./_gen/cascader-examples.md-->

## API

<!--@include: ./_gen/cascader-api.md-->

## 相关链接

- [源码 ui/entry_cascader.go](https://github.com/tangoui/tango/blob/main/ui/entry_cascader.go)、[ui/p2b_cascader.go](https://github.com/tangoui/tango/blob/main/ui/p2b_cascader.go)
- [ui Go API](/reference/api/ui)
- [TreeSelect 树选择](/components/treeselect)、[Select 选择器](/components/select)、[Form 表单](/components/form)
