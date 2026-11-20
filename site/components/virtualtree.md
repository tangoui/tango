---
title: VirtualTree 虚拟化树
---
# VirtualTree 虚拟化树

面向超大层级数据的虚拟化树，只构建视口内的可见节点，同时支持展开、勾选、过滤、键盘导航和懒加载。

## 何时使用

- 节点数达到数千、数万甚至更多，需要在固定高度视口中高效浏览时。
- 需要树形勾选、半选联动、当前节点、过滤高亮或按键定位时。
- 数据量较小或需要拖拽排序、连接线等完整普通树能力时用 [Tree 树形控件](/components/tree)；表单内选择层级值时用 [TreeSelect 树选择](/components/treeselect)。

## 在线体验

示例包含十万节点、可勾选半选联动、键盘导航、过滤匹配路径、懒加载和自定义节点内容。

<GalleryPreview component="VirtualTree" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。节点的 `Key` 在整棵树中必须唯一。

```go
import (
    "github.com/tangoui/tango/ui"
)

var nodes = []ui.TreeNode{
    {Key: "docs", Title: "文档", Children: []ui.TreeNode{
        {Key: "guide", Title: "指南"},
        {Key: "api", Title: "API"},
    }},
    {Key: "examples", Title: "示例", IsLeaf: true},
}

var expanded = []string{"docs"}

func page(c *ui.Context) {
    ui.VirtualTree(c, nodes).Expanded(&expanded).Height(240).Show()
}
```

`VirtualTree(c, nodes)` 会把展开后的可见节点展平，并只构建视口附近的行。用 `Expanded`、`Current`、`Checkable` 把树状态绑定到应用变量；不绑定时对应状态留在组件自己的状态中。

## 常见用法

以下片段沿用基本用法里的 `nodes`，省略了 `import`。

### 勾选与半选

`Checkable(&checked)` 显示复选框并绑定完整勾选键。默认父子联动，部分子节点选中时父节点出现在 `HalfChecked()` 中；`CheckStrictly` 让每个节点独立。

```go
var checked = []string{"guide"}

func checkableTree(c *ui.Context) {
    tree := ui.VirtualTree(c, nodes).Expanded(&expanded).
        Checkable(&checked).Height(240).Show()
    if tree.CheckChanged() {
        ui.Text(c, fmt.Sprintf("半选：%v", tree.HalfChecked())).Tertiary()
    }
}
```

### 搜索过滤与自定义节点

`Filter` 默认大小写不敏感地匹配标题，同时保留匹配节点的祖先并展开路径。`FilterMethod` 可以改为匹配业务字段，`NodeRender` 则完全接管行内容。

```go
var query string

func searchableTree(c *ui.Context) {
    ui.Input(c, &query).Placeholder("过滤节点")
    ui.VirtualTree(c, nodes).Filter(query).FilterMethod(func(q string, n *ui.TreeNode) bool {
        return strings.Contains(strings.ToLower(n.Title), strings.ToLower(q))
    }).NodeRender(func(n *ui.TreeNode, depth int) {
        ui.Text(c, fmt.Sprintf("%*s%s", depth*2, "", n.Title))
    }).Height(260).Show()
}
```

### 异步懒加载

展开没有子节点且没有 `IsLeaf` 标记的节点时，组件会持续调用 `LoadData` 回调；返回 `true` 表示仍在加载，返回 `false` 表示本次加载完成。数据到达后把子节点写回原树。

```go
var lazyNodes = []ui.TreeNode{{Key: "remote", Title: "远程目录"}}
var loadingTicks = map[string]int{}

func lazyTree(c *ui.Context) {
    ui.VirtualTree(c, lazyNodes).LoadData(func(key string) bool {
        loadingTicks[key]++
        if loadingTicks[key] < 3 {
            return true
        }
        for i := range lazyNodes {
            if lazyNodes[i].Key == key && len(lazyNodes[i].Children) == 0 {
                lazyNodes[i].Children = []ui.TreeNode{
                    {Key: key + "-a", Title: "子节点 A", IsLeaf: true},
                    {Key: key + "-b", Title: "子节点 B", IsLeaf: true},
                }
            }
        }
        return false
    }).Height(220).Show()
}
```

### 当前节点、点击和按键定位

`Current` 绑定键盘当前节点；`NodeClicked` 返回本次 `Show` 中被点击的节点键。`ScrollToKey` 会展开目标的祖先并把目标滚动到视口内，适合从外部搜索结果跳转。

```go
var current = "guide"
var targetKey = "api"

func navigableTree(c *ui.Context) {
    tree := ui.VirtualTree(c, nodes).Current(&current).Height(240)
    if ui.Button(c, "定位 API").Clicked() {
        tree.ScrollToKey(targetKey)
    }
    tree.Show()
    if key, ok := tree.NodeClicked(); ok {
        ui.Text(c, "点击："+key).Tertiary()
    }
}
```

### 原地更新节点

组件按顶层切片的身份、长度和数据版本缓存索引。原地修改键、子节点或影响过滤的字段后，递增 `DataVersion` 以重建索引和过滤结果。

```go
var treeVersion int

func updatedTree(c *ui.Context) {
    ui.VirtualTree(c, nodes).DataVersion(treeVersion).Height(240).Show()
}
```

## 主题与样式

VirtualTree 复用 Tree 的节点令牌（行高、缩进、选中和过滤高亮），三套主题分别对齐 Ant Design、Element Plus 和 shadcn 的树视觉。`ConfigProvider.Defaults("VirtualTree", ...)` 支持 `"checkStrictly"`、`"expandOnClickNode"` 和 `"checkOnClickNode"`；实例方法 `CheckStrictly`、`ExpandOnClick`、`CheckOnClick` 可覆盖默认值。

## 注意事项

- **Key 唯一**：`TreeNode.Key` 必须在整棵树中唯一；展开、勾选、当前节点和懒加载都以它作为身份。
- **状态放在哪**：需要被页面其他区域读取的状态绑定到 `Expanded`、`Checkable`、`Current` 的切片或字符串；筛选关键字放在页面变量里。
- **回调时机**：`CheckChanged`、`ExpandChanged` 和 `CurrentChanged` 要在 `Show` 返回后读取；`Range` 返回本帧构建行的半开区间。
- **性能**：`Height` 决定虚拟视口；节点总数可以很大，但展开后的可见索引、过滤结果和行回调仍应避免重复做昂贵工作。
- **键盘与无障碍**：整棵树是一个 Tab 停靠点。↑/↓、PageUp/PageDown、Home/End 移动，←/→ 收起或展开，Enter 设置当前节点，空格在可勾选时切换。需要无障碍名称时应给外层区域提供清晰标题。
- **RTL**：缩进、展开方向和节点布局遵循当前 RTL 设置。

## 示例代码

<!--@include: ./_gen/virtualtree-examples.md-->

## API

<!--@include: ./_gen/virtualtree-api.md-->

## 相关链接

- 源码：[ui/fill_virtual_tree.go](https://github.com/tangoui/tango/blob/main/ui/fill_virtual_tree.go)
- [ui Go API 参考](/reference/api/ui)
- [Tree 树形控件](/components/tree)、[TreeSelect 树选择](/components/treeselect)、[VirtualList 虚拟列表](/components/virtuallist)
- [组件总览](/components/)、[组件截图目录](/components/screenshots)
