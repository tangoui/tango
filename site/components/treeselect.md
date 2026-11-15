---
title: TreeSelect 树选择
---
# TreeSelect 树选择

在下拉框中以树形结构展示选项并进行选择，支持单选、多选、可勾选（父子联动）、搜索和动态加载。

## 何时使用

- 选项是一棵树（部门、目录、地区），并且需要选中树中的某个节点、而不必沿路径逐级选择时。
- 需要勾选多个节点并保持父子联动（选中父节点即选中全部子节点）时。
- 需要沿路径逐级选择、值为完整路径时，用 [Cascader 级联选择](/components/cascader)；树需要常驻页面展示时，用 [Tree 树形控件](/components/tree)。

## 在线体验

<GalleryPreview component="TreeSelect" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
	"github.com/tangoui/tango/ui"
)

var depts = []ui.TreeSelectNode{
	{Label: "总部", Value: "hq", Children: []ui.TreeSelectNode{
		{Label: "研发部", Value: "rd", Children: []ui.TreeSelectNode{
			{Label: "前端组", Value: "fe"}, {Label: "后端组", Value: "be"},
		}},
		{Label: "市场部", Value: "mkt"},
		{Label: "法务部（停用）", Value: "legal", Disabled: true},
	}},
}

var dept string

func page(c *ui.Context) {
	ui.TreeSelect(c, &dept, depts).Placeholder("请选择部门").AllowClear().Width(260)
	ui.Text(c, "部门："+dept)
}
```

节点用 `[]ui.TreeSelectNode` 描述（`Label`、`Value`、`Children`、`Disabled`），**整棵树中的 `Value` 必须唯一**。单选时值绑定到 `*string`，任何一级节点都可以选中。

## 常见用法

### 多选

`Multiple(&vals)` 把选择绑定到 `*[]string`，每次点击切换一个节点，选中项以标签显示；`MaxTagCount` 折叠多余标签。多选时第二个参数可以传 `nil`。

```go
import (
	"github.com/tangoui/tango/ui"
)

var (
	nodes = []ui.TreeSelectNode{{Label: "浙江", Value: "zj", Children: []ui.TreeSelectNode{
		{Label: "杭州", Value: "hz"}, {Label: "宁波", Value: "nb"}, {Label: "温州", Value: "wz"},
	}}}
	cities []string
)

func page(c *ui.Context) {
	ui.TreeSelect(c, nil, nodes).Multiple(&cities).MaxTagCount(2).Placeholder("请选择城市").Width(300)
}
```

### 可勾选与勾选策略

`Checkable()` 在节点前显示复选框，父子联动（隐含多选）。`ShowCheckedStrategy` 决定以哪些节点显示标签：`ui.ShowChild`（默认，只显示选中的叶子）、`ui.ShowParent`（父节点全选时只显示父节点）、`ui.ShowAll`（父子都显示）。默认绑定值保存选中的叶子，`Checked()` 返回按策略折叠后的值；`StrategyValue(true)` 让绑定值本身也按策略保存（与 antd 的 `value` 行为一致）。

`CheckStrictly()` 取消父子联动：每个节点单独勾选，值就是勾选的节点本身，此时勾选策略不再生效。

```go
import (
	"strings"

	"github.com/tangoui/tango/ui"
)

var (
	nodes = []ui.TreeSelectNode{{Label: "浙江", Value: "zj", Children: []ui.TreeSelectNode{
		{Label: "杭州", Value: "hz"}, {Label: "宁波", Value: "nb"},
	}}}
	scope, strict []string
)

func page(c *ui.Context) {
	w := ui.TreeSelect(c, nil, nodes).Multiple(&scope).Checkable().
		ShowCheckedStrategy(ui.ShowParent).DefaultExpandAll().Width(320)
	ui.Text(c, "标签："+strings.Join(w.Checked(), ", "))

	ui.TreeSelect(c, nil, nodes).Multiple(&strict).CheckStrictly().Width(320)
}
```

### 搜索

`Searchable()` 允许输入关键字过滤节点，默认按标签不区分大小写匹配，命中节点的祖先会保留并展开。`FilterTreeNode` 可替换匹配规则。

```go
import (
	"strings"

	"github.com/tangoui/tango/ui"
)

var (
	nodes = []ui.TreeSelectNode{{Label: "研发部", Value: "rd", Children: []ui.TreeSelectNode{{Label: "前端组", Value: "fe"}}}}
	team  string
)

func page(c *ui.Context) {
	ui.TreeSelect(c, &team, nodes).Searchable().Placeholder("搜索部门")
	ui.TreeSelect(c, &team, nodes).FilterTreeNode(func(q string, n ui.TreeSelectNode) bool {
		return strings.HasPrefix(n.Value, q) // 按值前缀匹配
	})
}
```

### 展开控制

`DefaultExpandAll()` 首次打开时展开全部节点；`ExpandedKeys(&keys)` 把展开的节点绑定到 `*[]string`（受控），用户展开 / 收起后会写回，`ExpandChanged()` 报告本帧是否变化。

```go
import (
	"github.com/tangoui/tango/ui"
)

var (
	nodes    = []ui.TreeSelectNode{{Label: "浙江", Value: "zj", Children: []ui.TreeSelectNode{{Label: "杭州", Value: "hz"}}}}
	expanded = []string{"zj"}
	sel      string
)

func page(c *ui.Context) {
	ui.TreeSelect(c, &sel, nodes).ExpandedKeys(&expanded)
	if ui.Button(c, "全部收起").Clicked() {
		expanded = nil
	}
}
```

### 动态加载

`LoadData` 与 [Tree](/components/tree) 的约定相同：展开一个没有 `Children` 且不是 `IsLeaf` 的节点时，每帧调用一次函数，直到它返回 `false`（`true` 表示仍在加载，期间显示加载图标）；应用在此期间把子节点追加到自己的节点切片里。

```go
import (
	"sync"

	"github.com/tangoui/tango/ui"
)

var (
	lazyNodes = []ui.TreeSelectNode{{Label: "浙江", Value: "zj"}, {Label: "江苏", Value: "js"}}
	picked    string

	mu      sync.Mutex
	loading = map[string]bool{}
	fetched = map[string][]ui.TreeSelectNode{}
)

func fetchChildren(v string) []ui.TreeSelectNode {
	return []ui.TreeSelectNode{{Label: v + " 下级", Value: v + "-1", IsLeaf: true}}
}

func attach(ns []ui.TreeSelectNode, v string, kids []ui.TreeSelectNode) {
	for i := range ns {
		if ns[i].Value == v {
			ns[i].Children = kids
			return
		}
		attach(ns[i].Children, v, kids)
	}
}

func page(c *ui.Context) {
	ui.TreeSelect(c, &picked, lazyNodes).LoadData(func(v string) bool {
		mu.Lock()
		defer mu.Unlock()
		if kids, ok := fetched[v]; ok {
			attach(lazyNodes, v, kids)
			delete(fetched, v)
			return false // 加载完成
		}
		if !loading[v] {
			loading[v] = true
			go func() {
				kids := fetchChildren(v) // 例如请求服务器
				mu.Lock()
				fetched[v] = kids
				mu.Unlock()
			}()
		}
		return true // 仍在加载
	})
}
```

### 大数据量

可见节点超过 100 行时下拉列表自动启用虚拟滚动，上万个节点也能流畅展开；`Virtual(true | false)` 可强制开启或关闭，`ListHeight` 设置下拉最大高度（默认 256）。

## 主题与样式

- 输入框外观与 Select 一致，支持 `Variant(ui.FieldFilled)` 等外观、`Size`、`Status("error" | "warning")`。
- 下拉面板中的树节点使用 `theme.Components.Tree`（`TreeToken`）的令牌，与 Tree 组件保持一致；三套风格切换无需改代码。
- `ConfigProvider.Defaults("TreeSelect", ui.Props{...})` 支持 `size`、`status`、`allowClear`、`showSearch`、`placeholder`、`treeDefaultExpandAll`、`maxTagCount`、`virtual`、`listHeight`、`showCheckedStrategy`、`strategyValue`。

## 注意事项

- **状态**：选中值和（使用 `ExpandedKeys` 时的）展开状态都在你的变量中；未受控时展开状态、搜索词由组件按调用位置保存。
- **节点数据**：`Value` 在整棵树中必须唯一；大树请放在包级变量或缓存中，避免每帧重新构造。
- **表单校验**：单选用 `f.Item("部门").Rules(&dept, ui.Required("请选择部门"))`；多选用 `ui.RulesOf` 配合 `ui.RequiredItems[string]` 规则。
- **无障碍与键盘**：Enter / 空格打开，↑ / ↓ 移动，→ 展开、← 收起，Home / End 跳到首尾，Enter / 空格选中或勾选，Backspace 删除最后一个标签；在表单项里自动以表单项标签命名。
- **截图与测试**：`Open()` 强制展开下拉，只在截图和测试中使用。

## 示例代码

<!--@include: ./_gen/treeselect-examples.md-->

## API

<!--@include: ./_gen/treeselect-api.md-->

## 相关链接

- [源码 ui/entry_treeselect.go](https://github.com/tangoui/tango/blob/main/ui/entry_treeselect.go)、[ui/p2b_treeselect.go](https://github.com/tangoui/tango/blob/main/ui/p2b_treeselect.go)
- [ui Go API](/reference/api/ui)
- [Cascader 级联选择](/components/cascader)、[Tree 树形控件](/components/tree)、[Select 选择器](/components/select)、[Form 表单](/components/form)
