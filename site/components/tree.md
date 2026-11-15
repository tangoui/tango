---
title: Tree 树形控件
---
# Tree 树形控件

多层级的结构列表：展开收起、选择、勾选、搜索、拖拽和异步加载子节点。

## 何时使用

- 文件夹、组织架构、地区、分类目录等天然有层级的数据。
- 需要在层级中选择或勾选节点（例如分配权限）时；勾选父节点会联动子节点。
- 在表单里从树中选一个值、收起成下拉框，用 `TreeSelect` 树选择；层级数据同时有多列字段时，用 [Table 表格](/components/table) 的 `Tree` 树形数据；只是两三层的导航菜单，用 `Menu` 导航菜单。

## 在线体验

示例包含可勾选（含半选）、连接线与图标、搜索高亮与拖拽、异步加载，以及 1000 个一级节点（每个 5 个子节点）的虚拟滚动。

<GalleryPreview component="Tree" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
    "github.com/tangoui/tango/ui"
)

var orgNodes = []ui.TreeNode{
    {Key: "hq", Title: "总部", Children: []ui.TreeNode{
        {Key: "rd", Title: "研发中心", Children: []ui.TreeNode{
            {Key: "fe", Title: "前端组"},
            {Key: "be", Title: "后端组"},
        }},
        {Key: "ops", Title: "运营中心", Children: []ui.TreeNode{
            {Key: "growth", Title: "增长组"},
        }},
    }},
}

var (
    orgExpanded = []string{"hq", "rd"}
    orgSelected []string
)

func page(c *ui.Context) {
    t := ui.Tree(c, orgNodes).Expanded(&orgExpanded).Selected(&orgSelected).Show()
    if t.SelectChanged() && len(orgSelected) > 0 {
        ui.Text(c, "选中："+orgSelected[0])
    }
}
```

`ui.Tree(c, nodes)` 接收一棵 `TreeNode` 树，链式设置后调用 `Show()`。每个节点的 `Key` 在整棵树中必须唯一，展开、选择、勾选都按 `Key` 记录。`Expanded`、`Selected` 把对应的键列表绑定到应用变量；不绑定时树自己保存这些状态。`SelectChanged()`、`ExpandChanged()`、`CheckChanged()` 报告本帧的变化。

## 常见用法

以下片段沿用基本用法里的 `orgNodes`，省略了 `import`。

### 可勾选

`Checkable(&checked)` 显示复选框并绑定勾选的键：勾选父节点会勾选全部子节点，部分子节点勾选时父节点显示半选，`HalfChecked()` 在 `Show` 之后返回半选的键。`CheckStrictly()` 取消父子联动，每个节点独立勾选。节点的 `Disabled` 禁用整个节点，`DisableCheckbox` 只禁用复选框。

```go
var checked = []string{"fe"}

func checkTree(c *ui.Context) {
    t := ui.Tree(c, orgNodes).Checkable(&checked).Show()
    if t.CheckChanged() {
        ui.Text(c, "半选："+fmt.Sprint(t.HalfChecked())).Tertiary()
    }
}
```

### 多选、连接线与图标

`Multiple()` 允许同时选中多个节点；`ShowLine()` 显示层级连接线；`ShowIcon()` 在标题前显示图标，节点 `Icon` 为 nil 时使用文件夹 / 文件图标；`BlockNode()` 让节点标题占满整行，便于点击。

```go
var (
    multiSel  []string
    lineOpen  = []string{"hq", "rd", "ops"}
)

func lineTree(c *ui.Context) {
    ui.Tree(c, orgNodes).Expanded(&lineOpen).Selected(&multiSel).
        Multiple().ShowLine().ShowIcon().BlockNode().Show()
}
```

### 搜索与过滤

`Search(keyword)` 高亮包含关键字的标题，关键字变化时自动展开所有匹配节点的父节点；再加 `Filter()` 会隐藏既不匹配、也不包含匹配子节点的节点（对应 Element Plus 的 `filter-node-method`）。

```go
var (
    keyword    string
    searchOpen []string
)

func searchTree(c *ui.Context) {
    ui.Input(c, &keyword).Placeholder("搜索部门")
    ui.Tree(c, orgNodes).Expanded(&searchOpen).Search(keyword).Filter().Show()
}
```

### 拖拽排序

`Draggable()` 允许拖动节点。树不会自己改数据：在 `Dropped()` 报告完成一次拖放后，用 `ui.TreeMove` 得到移动后的新树并写回。位置 `pos` 为 `ui.TreeDropBefore`、`ui.TreeDropInside`（成为第一个子节点）或 `ui.TreeDropAfter`。

```go
func dragTree(c *ui.Context) {
    t := ui.Tree(c, orgNodes).Draggable().BlockNode().Show()
    if drag, drop, pos, ok := t.Dropped(); ok {
        orgNodes = ui.TreeMove(orgNodes, drag, drop, pos)
    }
}
```

### 异步加载

`LoadData(fn)` 开启懒加载：展开一个没有 `Children`、也没有标记 `IsLeaf` 的节点时，树每帧调用 `fn(key)`，返回 true 表示仍在加载（显示转圈），返回 false 表示结束。在 `fn` 里发起一次请求，数据到达后把子节点挂到对应节点上。

```go
var (
    lazyNodes = []ui.TreeNode{
        {Key: "cn", Title: "中国"},
        {Key: "other", Title: "其他", IsLeaf: true},
    }
    pending = map[string]bool{}
    arrived = map[string][]ui.TreeNode{} // 后台请求写入，UI 线程读取
)

// requestChildren 在 goroutine 里请求子节点，结果经 app.Post 写入 arrived[key]。
func requestChildren(key string) {}

func lazyTree(c *ui.Context) {
    ui.Tree(c, lazyNodes).LoadData(func(key string) bool {
        if kids, ok := arrived[key]; ok {
            for i := range lazyNodes {
                if lazyNodes[i].Key == key {
                    lazyNodes[i].Children = kids
                }
            }
            return false
        }
        if !pending[key] {
            pending[key] = true
            requestChildren(key)
        }
        return true
    }).Show()
}
```

### 大数据量：虚拟滚动

`Height(px)` 把树变成固定高度的滚动视口，只构建视口内的行，开销与视口大小而不是节点总数成正比；`Built()` 返回本帧实际构建的行数。

```go
var bigOpen []string

func bigTree(c *ui.Context, nodes []ui.TreeNode) {
    ui.Tree(c, nodes).Expanded(&bigOpen).Height(320).A11yLabel("部门树").Show()
}
```

## 主题与样式

| 风格 | 差异 |
| --- | --- |
| Ant Design | 行高 24、行间距 4、每级缩进 24；选中节点浅色底、小圆角；搜索命中为橙红色文字。 |
| Element Plus | 对照 `el-tree`：行高 26 无间距、缩进 18；选中行主色浅底且为直角；默认点击标题即展开（`ExpandOnClick`）。 |
| shadcn | 侧边栏文件树样式：行高 28、缩进 16；选中项加粗（500）；搜索命中用浅色底标记；连接线为每级一条左边线。 |

组件令牌在 `theme.Components.Tree`（`theme.TreeToken`）：`TitleHeight`、`NodeGap`、`IndentSize`、`SwitcherSize`、`NodeBorderRadius`、`NodeHoverBg`、`NodeSelectedBg`、`NodeSelectedColor`、`LineColor`、`SearchHighlightColor`、`SearchHighlightBg`、`DropIndicatorColor` 等。`ExpandOnClick(on)` 可以在任何风格下单独开关「点击标题展开」。

`ConfigProvider.Defaults("Tree", ...)` 支持的键：`"showLine"`、`"showIcon"`、`"blockNode"`、`"checkable"`、`"multiple"`、`"draggable"`（bool），`"height"`（虚拟滚动视口高度，px），`"virtual"`（false 时 `height` 只是普通滚动区域）。

```go
func treeDefaults(c *ui.Context) {
    ui.ConfigProvider(c).Defaults("Tree", ui.Props{"showLine": true, "blockNode": true}).Body(func() {
        ui.Tree(c, orgNodes).Show()
    })
}
```

## 注意事项

- **Key 唯一**：节点 `Key` 在整棵树里必须唯一，同一棵树不要出现两个相同的键。
- **状态放在哪**：需要在别处读取或预设展开、选择、勾选时，用 `Expanded`、`Selected`、`Checkable` 绑定应用变量；不绑定时状态由树按自身的组件 ID 保存。条件分支或循环里渲染多棵树时，用 `ui.Key(c, key, fn)` 区分。
- **数据不可变**：拖拽、懒加载等改变结构的操作都由应用改 `nodes`；`ui.TreeMove` 返回新切片，不修改原树。
- **性能**：节点多于几百个时设置 `Height` 开启虚拟滚动；需要过滤高亮、可变高度等更多能力的超大树，可用 `VirtualTree`。
- **键盘与无障碍**：整棵树是一个 Tab 停靠点。↑ / ↓ 移动焦点行，Home / End 跳到首行 / 末行，→ 展开（或进入第一个子节点），← 收起（或回到父节点），Enter 选择，空格在可勾选时勾选、否则选择。用 `A11yLabel` 为树命名（放在 `FormItem` 里时自动使用表单项标签）。
- **RTL**：缩进、展开箭头和连接线在 RTL 布局中自动镜像。

## 示例代码

<!--@include: ./_gen/tree-examples.md-->

## API

<!--@include: ./_gen/tree-api.md-->

## 相关链接

- 源码：[ui/data_tree.go](https://github.com/tangoui/tango/blob/main/ui/data_tree.go)
- [ui Go API 参考](/reference/api/ui)
- [Table 表格](/components/table)（树形数据）、[List 列表](/components/list)
- [组件总览](/components/)、[组件截图目录](/components/screenshots)
