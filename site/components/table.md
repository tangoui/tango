---
title: Table 表格
---
# Table 表格

展示行列数据。表格本身不持有数据：你告诉它有多少行、每列怎么取值，它负责分页、排序、筛选、选择、展开、固定列、虚拟滚动和单元格编辑。

## 何时使用

- 需要展示大量结构化数据，并对数据做排序、筛选、分页、批量勾选等操作时。
- 需要逐行对比字段、在行内放操作按钮（编辑、删除、查看）时。
- 典型的「查询表单 + 工具栏 + 远程分页表格」后台列表页，直接用 [ProTable 高级表格](/components/protable)，它在 Table 之上补齐了请求、查询表单、工具栏和列设置。
- 只是一组键值对详情，用 [Descriptions 描述列表](/components/descriptions)；没有统一列结构的条目列表，用 [List 列表](/components/list)；纯层级结构的导航或勾选，用 [Tree 树形控件](/components/tree)。

## 在线体验

切换场景可以看到基础、选择、排序筛选、固定列、树形、合并单元格、可编辑、10 万行虚拟滚动等示例；工具栏支持 Ant Design、Element Plus、shadcn 三套风格与深浅色切换。

<GalleryPreview component="Table" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
    "fmt"

    "github.com/tangoui/tango/ui"
)

type user struct {
    ID     int
    Name   string
    Age    int
    City   string
    Status string // "online" | "offline"
}

var users = []user{
    {1, "胡彦斌", 32, "杭州", "online"},
    {2, "吴彦祖", 42, "上海", "offline"},
    {3, "李大嘴", 28, "北京", "online"},
    {4, "张小泉", 51, "杭州", "online"},
}

var viewing string

func page(c *ui.Context) {
    cols := []ui.TableColumn{
        {Key: "name", Title: "姓名", Width: 140, Text: func(i int) string { return users[i].Name }},
        {Key: "age", Title: "年龄", Width: 80, Align: ui.AlignRight,
            Text: func(i int) string { return fmt.Sprint(users[i].Age) }},
        {Key: "city", Title: "城市", Text: func(i int) string { return users[i].City }},
        {Key: "status", Title: "状态", Width: 120, Render: func(i int) {
            if users[i].Status == "online" {
                ui.Badge(c).Status("success", "在线")
            } else {
                ui.Badge(c).Status("default", "离线")
            }
        }},
        {Key: "op", Title: "操作", Width: 100, Render: func(i int) {
            if ui.Link(c, "查看").Clicked() {
                viewing = users[i].Name
            }
        }},
    }
    ui.Table(c, cols, len(users)).
        RowKey(func(i int) string { return fmt.Sprint(users[i].ID) }).
        Show()
    if viewing != "" {
        ui.Text(c, "正在查看："+viewing)
    }
}
```

`ui.Table(c, cols, rows)` 只接收列定义和行数，链式设置修饰方法后调用 `Show()` 构建。每列的 `Text` 或 `Render` 回调收到的 `i` 是**数据下标**（0..rows-1），无论表格如何排序、筛选、分页，回调里的 `i` 始终指向原始数据，直接 `users[i]` 取值即可。`RowKey` 给每行一个稳定的键，选择、展开、编辑都按它记忆。

## 常见用法

以下片段沿用基本用法里的 `user` 类型和 `users` 数据，省略了 `import`。

### 数据源与列定义

`TableColumn` 只有 `Key`、`Title` 和 `Text` / `Render` 二选一是必需的：

- **取值**：`Text` 返回纯文本，开销最小；`Render` 在单元格里构建任意组件（标签、徽标、链接、进度条、按钮），两者同时设置时 `Render` 生效。
- **宽度**：`Width` 固定宽度；为 0 时按 `Grow`（默认 1）分配剩余空间，且不小于 `MinWidth`（默认 80）。
- **对齐**：`Align` 设置单元格与表头的对齐，数字列常用 `ui.AlignRight`；它是逻辑方向，RTL 下会镜像。
- **长文本**：`Ellipsis` 单行省略并在悬停时提示全文；`Wrap` 自动换行，行高按最高的单元格测量（两者同时设置时 `Wrap` 生效）。
- **外观**：`Size(ui.Middle)` / `Size(ui.Small)` 切换密度（默认 Large），`Bordered()` 显示全部边框，`Striped()` 斑马纹，`EmptyText` 自定义空状态文案，`Loading(true)` 显示加载遮罩。

```go
func styledTable(c *ui.Context) {
    cols := []ui.TableColumn{
        {Key: "name", Title: "姓名", Width: 120, Text: func(i int) string { return users[i].Name }},
        {Key: "city", Title: "城市", Width: 100, Render: func(i int) {
            ui.Tag(c, users[i].City).Color("processing")
        }},
        {Key: "note", Title: "备注", Ellipsis: true, Text: func(i int) string {
            return users[i].Name + " 的备注很长很长，超出列宽时省略，悬停显示全文"
        }},
    }
    ui.Table(c, cols, len(users)).Size(ui.Middle).Bordered().Striped().
        EmptyText("暂无用户").Show()
}
```

### 分页

本地数据用 `Pagination(&page, &pageSize)`：`rows` 传全部数据，表格只显示当前页，`page` 从 1 开始。`PageSizes` 设置每页条数选项，`PageChanged()` 报告本帧翻页或改变每页条数。

服务端数据用 `ServerPagination(&page, &pageSize, total)`：`rows` 只传当前页的行数，`total` 是总条数；检测到 `PageChanged()` 后去后台拉取新一页。请求放在 goroutine 里，结果写回 UI 线程后重绘（见 [后台数据与线程](/guide/ui#后台数据与线程)）。

```go
var curPage, curSize = 1, 10

func localPaged(c *ui.Context, cols []ui.TableColumn) {
    ui.Table(c, cols, len(users)).Pagination(&curPage, &curSize).PageSizes(10, 20, 50).Show()
}

var (
    srvPage, srvSize = 1, 10
    srvTotal         int
    srvRows          []user
    srvLoading       bool
)

// fetchPage 在 goroutine 里请求第 page 页，结果经 Runner.Post 写回 srvRows / srvTotal（在窗口的 UI 线程上），
// 并把 srvLoading 置回 false。
func fetchPage(page, size int) { srvLoading = true }

func serverPaged(c *ui.Context) {
    cols := []ui.TableColumn{
        {Key: "name", Title: "姓名", Text: func(i int) string { return srvRows[i].Name }},
        {Key: "city", Title: "城市", Text: func(i int) string { return srvRows[i].City }},
    }
    t := ui.Table(c, cols, len(srvRows)).
        ServerPagination(&srvPage, &srvSize, srvTotal).
        Loading(srvLoading).
        Show()
    if t.PageChanged() {
        fetchPage(srvPage, srvSize)
    }
}
```

### 排序

列设置 `Compare` 后表格在本地排序：比较函数收到两个数据下标，返回负数、0 或正数。点击表头在「升序 → 降序 → 取消」之间循环。

- 不绑定状态时表格自己记住排序，`DefaultSort` 指定初始顺序。
- `Sort(&state)` 受控绑定单列排序，`SortState{Column, Order}` 里 `Order` 为 `ui.SortAsc` / `ui.SortDesc` / `ui.SortNone`。
- 多列排序：给参与的列设置 `SortMultiple`（值越大优先级越高），用 `MultiSort(&[]ui.SortState)` 绑定。
- 服务端排序：只设 `Sortable: true` 不设 `Compare`，表格只显示排序按钮并更新状态，读到 `SortChanged()` 后按 `state` 重新请求数据。

```go
var sortState ui.SortState

func sortedTable(c *ui.Context) {
    cols := []ui.TableColumn{
        {Key: "name", Title: "姓名", Text: func(i int) string { return users[i].Name }},
        {Key: "age", Title: "年龄", Width: 100, Align: ui.AlignRight,
            Compare: func(a, b int) int { return users[a].Age - users[b].Age },
            Text:    func(i int) string { return fmt.Sprint(users[i].Age) }},
    }
    t := ui.Table(c, cols, len(users)).Sort(&sortState).Show()
    if t.SortChanged() {
        ui.Text(c, fmt.Sprintf("按 %s 排序，方向 %d", sortState.Column, sortState.Order)).Tertiary()
    }
}
```

`DisplayedRows()` 在 `Show` 之后返回当前页排序、筛选后的数据下标顺序，可用来导出「所见即所得」的数据。

### 筛选

列的 `Filters` 列出下拉中的候选值，`OnFilter(row, value)` 判断某行是否匹配某个值（多个值之间是「或」）。`FilterSearch` 在候选值上方加搜索框，`FilterSingle` 改为单选。筛选状态可以用 `Filters(&map[string][]string)` 受控绑定（列 Key → 选中的值），`FilterChanged()` 报告确认或重置。

需要完全自定义的筛选面板时设置 `FilterPanel`：回调收到 `*ui.TableFilterPanel`，在 `p.Context()` 上构建任意组件，把结果写进 `*p.Values`，再调用 `p.Confirm()` 应用或 `p.Reset()` 清除。

```go
var filters = map[string][]string{}

func filteredTable(c *ui.Context) {
    cols := []ui.TableColumn{
        {Key: "name", Title: "姓名", Text: func(i int) string { return users[i].Name }},
        {Key: "city", Title: "城市", Width: 140,
            Filters:      []ui.Option{{Label: "杭州", Value: "杭州"}, {Label: "上海", Value: "上海"}, {Label: "北京", Value: "北京"}},
            FilterSearch: true,
            OnFilter:     func(i int, v string) bool { return users[i].City == v },
            Text:         func(i int) string { return users[i].City }},
        {Key: "status", Title: "状态", Width: 140, FilterSingle: true,
            Filters:  []ui.Option{{Label: "在线", Value: "online"}, {Label: "离线", Value: "offline"}},
            OnFilter: func(i int, v string) bool { return users[i].Status == v },
            Text:     func(i int) string { return users[i].Status }},
    }
    ui.Table(c, cols, len(users)).Filters(&filters).Show()
}
```

### 行选择

`Selection(&[]int)` 在首列加复选框，按数据下标绑定选择；配合 `RowKey` 时更推荐 `SelectedKeys(&[]string)` 按行键绑定，翻页、排序、数据刷新后选择不会错位，分页时跨页保留。

- `SingleSelect()` 改为单选（单选框）；`SelectDisabled(fn)` 禁用部分行；`HideSelectAll()` 隐藏表头全选框。
- `Selections()` 在表头复选框旁加下拉菜单：不传参数时提供「全选所有 / 选择当页 / 反选当页 / 清空」，也可传入 `ui.TableSelection` 自定义项。
- `SelectionChanged()` 报告本帧的选择变化；`RowClicked()` / `RowDoubleClicked()` 返回被点击的数据行（没有时为 -1），`HighlightCurrentRow()` 让点击的行保持高亮。

```go
var selected []string

func selectableTable(c *ui.Context, cols []ui.TableColumn) {
    ui.Text(c, fmt.Sprintf("已选择 %d 项", len(selected)))
    t := ui.Table(c, cols, len(users)).
        RowKey(func(i int) string { return fmt.Sprint(users[i].ID) }).
        SelectedKeys(&selected).
        SelectDisabled(func(i int) bool { return users[i].Status == "offline" }).
        Selections().
        Show()
    if r := t.RowDoubleClicked(); r >= 0 {
        viewing = users[r].Name
    }
}
```

### 展开行与树形数据

`Expandable(render)` 增加展开列，`render(i)` 构建第 i 行的详情面板；`RowExpandable(fn)` 限制哪些行可展开，`ExpandedKeys(&[]string)` 受控绑定展开的行键。

层级数据用 `Tree(roots, children)`：`roots` 是顶层行的下标，`children(i)` 返回第 i 行的子行下标，表格在第一列画缩进和展开按钮；`TreeOpenKeys` 绑定展开状态，`Indent` 设置每级缩进。子节点需要按需加载时加 `TreeLazy(has, load)`：`has(i)` 表示该行有尚未加载的子节点，展开时调用一次 `load(i)`，加载期间显示转圈。树形表格配合 `Selection` 时父子勾选联动，`CheckStrictly()` 取消联动。

```go
var expanded []string

func expandableTable(c *ui.Context, cols []ui.TableColumn) {
    ui.Table(c, cols, len(users)).
        RowKey(func(i int) string { return fmt.Sprint(users[i].ID) }).
        Expandable(func(i int) {
            ui.Descriptions(c, []ui.DescItem{
                {Label: "姓名", Value: users[i].Name},
                {Label: "城市", Value: users[i].City},
                {Label: "年龄", Value: fmt.Sprint(users[i].Age)},
            })
        }).
        RowExpandable(func(i int) bool { return users[i].Status == "online" }).
        ExpandedKeys(&expanded).
        Show()
}

type file struct {
    Name string
    Kids []int
}

var files = []file{
    {"src", []int{1, 2}},
    {"main.go", nil},
    {"table.go", nil},
    {"README.md", nil},
}

var openDirs = []string{"0"}

func treeTable(c *ui.Context) {
    cols := []ui.TableColumn{{Key: "name", Title: "名称", Text: func(i int) string { return files[i].Name }}}
    ui.Table(c, cols, len(files)).
        Tree([]int{0, 3}, func(i int) []int { return files[i].Kids }).
        TreeOpenKeys(&openDirs).
        Show()
}
```

### 固定列、固定表头与表头分组

列数较多时用 `ScrollX(totalWidth)` 设置内容宽度开启横向滚动，并把关键列 `Fixed: ui.FixedLeft` / `ui.FixedRight` 固定在两侧，滚动时固定列边缘出现阴影。`ScrollY(maxHeight)` 限制表体高度：表头保持不动，只滚动表体。`Resizable()` 允许拖动表头边缘调整列宽。

- 表格放在外层滚动容器里时，`Sticky(offsetTop)` 让表头吸附在容器顶部，`StickySummary(offsetBottom)` 让总结栏吸附在底部；`StickyContainer(id)` 指定参照的滚动容器。
- 多级表头：列的 `Children` 填子列，父列只需要 `Key` 和 `Title`。

```go
func wideTable(c *ui.Context) {
    cols := []ui.TableColumn{
        {Key: "name", Title: "姓名", Width: 120, Fixed: ui.FixedLeft, Text: func(i int) string { return users[i].Name }},
        {Key: "info", Title: "基本信息", Children: []ui.TableColumn{
            {Key: "age", Title: "年龄", Width: 200, Text: func(i int) string { return fmt.Sprint(users[i].Age) }},
            {Key: "city", Title: "城市", Width: 300, Text: func(i int) string { return users[i].City }},
        }},
        {Key: "status", Title: "状态", Width: 400, Text: func(i int) string { return users[i].Status }},
        {Key: "op", Title: "操作", Width: 100, Fixed: ui.FixedRight, Render: func(i int) { ui.Link(c, "编辑") }},
    }
    ui.Table(c, cols, len(users)).ScrollX(1200).ScrollY(240).Resizable().Bordered().Show()
}
```

### 虚拟滚动

设置 `ScrollY` 后表格只构建视口内可见的行；再加 `Virtual()`（对应 Element Plus 的 Table V2）连可见范围外的列也不构建，适合 10 万行 × 几十列的数据。固定列、选择、键盘导航照常工作；`Wrap` 列的可变行高在虚拟滚动下同样可用：可见行测量后缓存，未测量的行按均值估算。

```go
func bigTable(c *ui.Context) {
    cols := []ui.TableColumn{
        {Key: "id", Title: "编号", Width: 100, Fixed: ui.FixedLeft, Text: func(i int) string { return fmt.Sprint("#", i+1) }},
        {Key: "v", Title: "数值", Width: 120, Align: ui.AlignRight, Text: func(i int) string { return fmt.Sprint(i * 37 % 1000) }},
        {Key: "note", Title: "说明", Text: func(i int) string { return fmt.Sprint("第 ", i+1, " 行") }},
    }
    ui.Table(c, cols, 100000).ScrollY(440).Virtual().Size(ui.Small).Show()
}
```

### 可编辑单元格与可编辑行

给列设置 `Editor` 后单元格可以编辑：点击进入编辑，回车或失焦提交，Esc 取消。`TableEditor.Kind` 选择编辑控件：`"text"`（默认）、`"number"`、`"select"`（配合 `Options`）、`"date"`；`Set(row, v)` 写回数据，`Validate(v)` 返回非空字符串时阻止提交并显示错误提示。

整行编辑用 `EditableRows(&[]string)` 绑定处于编辑状态的行键，此时该行所有带 `Editor` 的列同时显示输入框；在操作列里调用表格的 `Edit(i)`、`Save(i)`（校验失败返回 false）、`Cancel(i)`，用 `IsEditing(i)` 切换按钮。`Edited()` 报告本帧提交了编辑。

`DragSort(fn)` 增加拖拽把手列，松手时回调 `fn(from, to)` 由应用重排数据；聚焦表格后 Alt+↑ / Alt+↓ 也能移动当前行。

```go
var editing []string

func editableTable(c *ui.Context) {
    var tbl *ui.TableW
    cols := []ui.TableColumn{
        {Key: "name", Title: "姓名", Width: 160, Text: func(i int) string { return users[i].Name },
            Editor: &ui.TableEditor{
                Set: func(i int, v string) { users[i].Name = v },
                Validate: func(v string) string {
                    if v == "" {
                        return "此项为必填项"
                    }
                    return ""
                },
            }},
        {Key: "city", Title: "城市", Width: 160, Text: func(i int) string { return users[i].City },
            Editor: &ui.TableEditor{Kind: "select", Options: ui.Opts("杭州", "上海", "北京"),
                Set: func(i int, v string) { users[i].City = v }}},
        {Key: "op", Title: "操作", Render: func(i int) {
            if tbl.IsEditing(i) {
                if ui.Link(c, "保存").Clicked() {
                    tbl.Save(i)
                }
                if ui.Link(c, "取消").Clicked() {
                    tbl.Cancel(i)
                }
                return
            }
            if ui.Link(c, "编辑").Clicked() {
                tbl.Edit(i)
            }
        }},
    }
    tbl = ui.Table(c, cols, len(users)).
        RowKey(func(i int) string { return fmt.Sprint(users[i].ID) }).
        EditableRows(&editing).
        Bordered()
    tbl.Show()
}
```

注意 `tbl` 要在 `Show()` 之前赋值：操作列的 `Render` 在 `Show()` 期间运行，需要通过它调用 `Edit` / `Save` / `Cancel`。

### 合并单元格、总结栏与行样式

- `Span(fn)` 合并表体单元格：`fn` 收到 `ui.TableSpan{Row, Pos, Col, Key}`，返回 `(rowSpan, colSpan)`；返回 0 表示该单元格被相邻单元格覆盖。
- `Summary(fn)` 增加总结（合计）行，`fn(colKey, rows)` 收到当前显示的数据行，返回该列的文字；多次调用可增加多行，`SummaryFixed()` 让它在表体滚动时固定在底部。
- `RowClass(fn)` 按行返回 `ui.RowStyle`：`Status`（`"success"`、`"warning"`、`"error"` 等状态底色）、`Bg`、`Text`、`Bold`。

```go
func summaryTable(c *ui.Context, cols []ui.TableColumn) {
    ui.Table(c, cols, len(users)).
        Summary(func(col string, rows []int) string {
            switch col {
            case "name":
                return "合计"
            case "age":
                sum := 0
                for _, r := range rows {
                    sum += users[r].Age
                }
                return fmt.Sprint(sum)
            }
            return ""
        }).
        RowClass(func(i int) ui.RowStyle {
            if users[i].Status == "offline" {
                return ui.RowStyle{Status: "warning"}
            }
            return ui.RowStyle{}
        }).
        Show()
}
```

记录少、字段多、需要横向对比时，`Transposed()` 把每列变成一行、每条记录变成一列，`TransposedHeader` 设置记录列的标题。

## 主题与样式

三套风格共用同一套 API，外观按各自设计规范绘制：

| 风格 | 差异 |
| --- | --- |
| Ant Design | 默认 Large 密度，Middle / Small 依次收紧；表头上角圆角；选中行用主色浅底，排序中的列表头与表体有浅色底。 |
| Element Plus | 默认密度相当于 Middle，`Size(ui.Large)` 才用最宽松的内边距；表头无圆角；单元格文字为次要文字色；选中行不着色，`HighlightCurrentRow` 对应 `highlight-current-row`。 |
| shadcn | Data Table 样式：只有一档密度（`Small` 仅收紧上下内边距），表头标题是带排序图标的幽灵按钮，选中行为 muted 底色。 |

组件令牌在 `theme.Components.Table`（`theme.TableToken`）：`HeaderBg`、`HeaderColor`、`RowHoverBg`、`RowSelectedBg`、`RowStripedBg`、`BorderColor`、`FooterBg`、`CellFontSize`，以及三档密度的 `CellPaddingBlock` / `CellPaddingInline`（`MD`、`SM` 后缀对应 Middle 和 Small）等。用 `ConfigProvider` 局部覆盖：

```go
func themedTable(c *ui.Context, cols []ui.TableColumn) {
    ui.ConfigProvider(c).
        Components(theme.Components{Table: theme.TableToken{HeaderBg: theme.Hex("#e6f4ff")}}).
        Defaults("Table", ui.Props{"size": "middle", "bordered": true}).
        Body(func() {
            ui.Table(c, cols, len(users)).Show()
        })
}
```

`ConfigProvider.Defaults("Table", ...)` 支持的键：`"size"`（`"small"` / `"middle"` / `"large"` 或 `ui.ControlSize`）、`"bordered"`、`"striped"`（也接受 Element 的 `"stripe"`）。

## 注意事项

- **状态放在哪**：数据和受控绑定的变量（`page`、`selected`、`sortState`……）放在包级或页面闭包里，每帧都是同一个变量。没有绑定的排序、筛选、展开等由表格按自身的组件 ID 保存；在循环或条件分支里渲染多张表格时，用 `ui.Key(c, key, fn)` 包住每张表格，避免插入或删除兄弟组件后状态串位。
- **行键**：选择、展开、编辑都依赖 `RowKey`；没有设置时用数据下标作键，数据增删后会错位。
- **原地修改数据**：表格会缓存本地排序与筛选的结果。行数不变但数据内容变了（例如编辑后某列值改变）时，调用 `Revision(v)` 并递增 `v`，让表格重新计算。
- **性能**：数据量大时设置 `ScrollY`，列也多时再加 `Virtual()`；纯文本列用 `Text` 而不是 `Render`，后者每个单元格都会构建组件。
- **无障碍与键盘**：用 `A11yLabel` 为表格命名。表格是一个 Tab 停靠点：方向键移动当前单元格，Home / End 跳到首列 / 末列，PageUp / PageDown 翻屏，空格切换当前行的选择，Enter 编辑或展开，Ctrl/Cmd+C 复制单元格；可排序表头可聚焦，Enter 或空格排序；列宽拖拽手柄也能用方向键调整。
- **RTL**：`FixedLeft` / `FixedRight` 和 `AlignLeft` / `AlignRight` 都是逻辑方向，在 RTL 布局中自动镜像；需要物理方向时用 `AlignPhysicalLeft` / `AlignPhysicalRight`。

## 示例代码

<!--@include: ./_gen/table-examples.md-->

## API

<!--@include: ./_gen/table-api.md-->

## 相关链接

- 源码：[ui/table.go](https://github.com/tangoui/tango/blob/main/ui/table.go)（`table_*.go` 为分拆的实现）
- [ui Go API 参考](/reference/api/ui)
- [ProTable 高级表格](/components/protable)、[Pagination 分页](/components/pagination)、[Descriptions 描述列表](/components/descriptions)
- [组件总览](/components/)、[组件截图目录](/components/screenshots)
