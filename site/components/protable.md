---
title: ProTable 高级表格
---
# ProTable 高级表格

面向「查询列表页」的高级表格：从列定义自动生成查询表单，自带工具栏（刷新、密度、列设置、全屏），通过一个异步的 `Request` 函数获取分页数据。对标 Ant Design Pro Components v3 的 ProTable。

## 何时使用

- 后台管理系统里典型的列表页：上方查询条件、中间工具栏和批量操作、下方远程分页表格。
- 数据来自服务端，需要统一处理分页、排序、筛选、加载状态、请求失败重试时。
- 需要按 `valueType` 自动格式化单元格（金额、日期、状态、标签、进度……），并让用户自己调整列的顺序、显隐和固定时。
- 数据都在本地、不需要查询表单和工具栏时，用更轻量的 [Table 表格](/components/table)；需要行内编辑多行数据时用 `EditableProTable`。

## 在线体验

切换场景可以看到完整的查询列表、拖拽排序表格，以及外部参数、轮询、列状态持久化与自定义批量操作栏。

<GalleryPreview component="ProTable" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
    "strings"
    "time"

    "github.com/tangoui/tango/ui"
)

type rule struct {
    Key     string
    Name    string
    Status  string // "running" | "online" | "error"
    Calls   int
    Updated time.Time
}

var allRules = []rule{
    {"r1", "TradeCode 99", "running", 120, time.Date(2026, 11, 1, 9, 0, 0, 0, time.Local)},
    {"r2", "TradeCode 98", "online", 860, time.Date(2026, 11, 3, 14, 30, 0, 0, time.Local)},
    {"r3", "TradeCode 97", "error", 42, time.Date(2026, 11, 8, 18, 5, 0, 0, time.Local)},
}

var statusEnum = []ui.ProEnum{
    {Value: "running", Label: "运行中", Status: "processing"},
    {Value: "online", Label: "已上线", Status: "success"},
    {Value: "error", Label: "异常", Status: "error"},
}

// queryRules 模拟服务端：按查询条件过滤并分页。
func queryRules(req ui.ProRequest) ([]rule, int) {
    var out []rule
    for _, r := range allRules {
        if v := req.Params["name"]; v != "" && !strings.Contains(r.Name, v) {
            continue
        }
        if v := req.Params["status"]; v != "" && r.Status != v {
            continue
        }
        out = append(out, r)
    }
    total := len(out)
    if req.PageSize > 0 {
        a := min(total, (req.Page-1)*req.PageSize)
        out = out[a:min(total, a+req.PageSize)]
    }
    return out, total
}

func page(c *ui.Context) {
    cols := []ui.ProColumn[rule]{
        {Key: "name", Title: "规则名称", Value: func(r rule) any { return r.Name }},
        {Key: "status", Title: "状态", Width: 120, ValueType: ui.ValueStatus, ValueEnum: statusEnum,
            Value: func(r rule) any { return r.Status }},
        {Key: "calls", Title: "调用次数", Width: 120, ValueType: ui.ValueDigit, HideInSearch: true,
            Value: func(r rule) any { return r.Calls }},
        {Key: "updated", Title: "更新时间", Width: 180, ValueType: ui.ValueDateTime, HideInSearch: true,
            Value: func(r rule) any { return r.Updated }},
    }
    ui.ProTable(c, cols).
        Title("规则列表").
        RowKey(func(r rule) string { return r.Key }).
        Request(func(req ui.ProRequest, done func(ui.ProResult[rule])) {
            rows, total := queryRules(req)
            done(ui.ProResult[rule]{Rows: rows, Total: total})
        }).
        Show()
}
```

`ui.ProTable(c, cols)` 是泛型函数，`T` 是一行数据的类型。每个 `ProColumn` 用 `Value(row)` 取值、`ValueType` 决定显示格式；没有设置 `HideInSearch` 的列会自动出现在上方的查询表单里。`Request` 是数据源：页码、排序、筛选或查询条件变化时被调用，拿到数据后调用 `done` 交回结果。

## 常见用法

以下片段沿用基本用法里的 `rule`、`allRules`、`statusEnum` 和 `queryRules`，省略了 `import`。

### request：远程数据

`Request(fn)` 在 UI 线程上调用 `fn(req, done)`，表格同时显示加载遮罩。`done` 可以在**任意 goroutine** 上调用，所以真实的网络请求直接放进 goroutine：结果到达后表格自动重绘；如果期间又发起了新请求，旧请求的结果会被丢弃。`ProResult.Err` 不为 nil 时表格显示错误提示和「重试」按钮。

`ProRequest` 包含：`Page`、`PageSize`（`NoPagination()` 时为 0，表示要全部数据）、`Sort`（`[]ui.SortState`）、`Filters`（列 Key → 选中的值）、`Params`（查询表单的非空值，日期为 `2006-01-02` 文本）以及 `ExtParams`（见下文「外部参数」）。

```go
// fetchRules 代表一次 HTTP 请求，会阻塞。
func fetchRules(req ui.ProRequest) ([]rule, int, error) {
    rows, total := queryRules(req)
    return rows, total, nil
}

func remoteTable(c *ui.Context, cols []ui.ProColumn[rule]) {
    ui.ProTable(c, cols).
        RowKey(func(r rule) string { return r.Key }).
        PageSize(20).
        Request(func(req ui.ProRequest, done func(ui.ProResult[rule])) {
            go func() {
                rows, total, err := fetchRules(req)
                done(ui.ProResult[rule]{Rows: rows, Total: total, Err: err})
            }()
        }).
        Show()
}
```

### columns 与 valueType

`ValueType` 让同一份列定义同时决定单元格的显示和查询表单里的控件：

| valueType | 单元格显示 | `Value` 返回 |
| --- | --- | --- |
| `ui.ValueText`（默认） | 文本 | 任意，按 `fmt` 格式化 |
| `ui.ValueDigit` / `ui.ValueMoney` / `ui.ValuePercent` | 千分位数字 / 金额（`Currency` 指定币种）/ 百分比，右对齐 | 数字 |
| `ui.ValueDate` / `ui.ValueDateTime` / `ui.ValueFromNow` | 日期 / 日期时间 / 相对时间 | `time.Time` |
| `ui.ValueStatus` | 状态徽标，文字和颜色取自 `ValueEnum` 的 `Label` / `Status` | 枚举值 |
| `ui.ValueTag` | 标签，颜色取自 `ValueEnum` 的 `Color` | 枚举值 |
| `ui.ValueProgress` | 进度条 | 0–100 的数字 |
| `ui.ValueLink` | 链接，点击调用 `OnClick(row)` | 文本 |
| `ui.ValueIndex` | 当前页内的序号 | 忽略 |
| `ui.ValueOption` | 操作列，只用 `Render`，不进入查询表单 | — |

`ui.ValueSelect`、`ui.ValueDateRange`、`ui.ValueDigitRange` 等与 ProForm 共用的类型也可以用，查询表单会生成对应的控件。`Render(row, i)` 完全自定义单元格（`i` 是当前页内的行号）；`Hidden` 让列默认隐藏、可在列设置里打开；`HideInTable` 只在查询表单里出现；`Sortable` 显示排序按钮，`Filterable` 用 `ValueEnum` 生成筛选下拉，两者都是**服务端**的：结果写进 `req.Sort` / `req.Filters`，由 `Request` 处理。

```go
var opened string

func ruleColumns(c *ui.Context) []ui.ProColumn[rule] {
    return []ui.ProColumn[rule]{
        {Key: "index", Title: "#", Width: 56, ValueType: ui.ValueIndex},
        {Key: "name", Title: "规则名称", ValueType: ui.ValueLink,
            Value:   func(r rule) any { return r.Name },
            OnClick: func(r rule) { opened = r.Key }},
        {Key: "status", Title: "状态", Width: 120, ValueType: ui.ValueStatus, ValueEnum: statusEnum,
            Filterable: true, Value: func(r rule) any { return r.Status }},
        {Key: "calls", Title: "调用次数", Width: 120, ValueType: ui.ValueDigit, Sortable: true,
            HideInSearch: true, Value: func(r rule) any { return r.Calls }},
        {Key: "updated", Title: "更新时间", Width: 180, ValueType: ui.ValueDateTime, Hidden: true,
            HideInSearch: true, Value: func(r rule) any { return r.Updated }},
        {Key: "op", Title: "操作", Width: 120, ValueType: ui.ValueOption, Render: func(r rule, i int) {
            if ui.Link(c, "配置").Clicked() {
                opened = r.Key
            }
            ui.Link(c, "订阅")
        }},
    }
}
```

### 查询表单

查询表单由所有可搜索的列生成（有 `Key`、没有 `HideInSearch`，且不是操作、序号等无法搜索的类型），用户点击「查询」后回到第 1 页并重新请求，字段值以列 `Key` 为键出现在 `req.Params`。`NoSearch()` 隐藏查询表单。

页面上其他控件（页签、分段器、侧边树）也想参与查询时，用 `Params(v)` 传入外部参数：`v` 变化时（按打印结果比较）自动回到第 1 页重新请求；`v` 原样出现在 `req.ExtParams`，如果是 `map[string]string` 或 `map[string]any` 还会合并进 `req.Params`（与表单同名时表单优先）。

```go
var statusTab = ""

func tabbedTable(c *ui.Context, cols []ui.ProColumn[rule]) {
    ui.Segmented(c, &statusTab, []ui.Option{
        {Label: "全部", Value: ""}, {Label: "运行中", Value: "running"}, {Label: "异常", Value: "error"},
    })
    ui.ProTable(c, cols).NoSearch().
        Params(map[string]string{"status": statusTab}).
        Request(func(req ui.ProRequest, done func(ui.ProResult[rule])) {
            rows, total := queryRules(req) // req.Params["status"] 来自 Segmented
            done(ui.ProResult[rule]{Rows: rows, Total: total})
        }).
        Show()
}
```

### 工具栏与批量操作

`Title` 设置工具栏标题，`ToolBar(fn)` 在右侧设置图标前放自己的按钮（如「新建」）；设置图标依次是刷新、密度、列设置、全屏，`NoOptions()` 全部隐藏。

`Selection(&keys)` 开启行选择（需要 `RowKey`，跨页保留）；有选中项时表格上方出现选择提示栏，`TableAlertRender` 替换提示文字，`TableAlertOptionRender` 替换右侧操作（默认是「清空」），两者都会收到选中的键和清空函数；`NoTableAlert()` 隐藏提示栏。

```go
var (
    selectedRules []string
    creating      bool
)

func batchTable(c *ui.Context, cols []ui.ProColumn[rule]) {
    var pt *ui.ProTableW[rule]
    pt = ui.ProTable(c, cols).
        Title("规则列表").
        RowKey(func(r rule) string { return r.Key }).
        Selection(&selectedRules).
        ToolBar(func() {
            if ui.Button(c, "新建").Primary().Icon(icon.Plus).Clicked() {
                creating = true
            }
        }).
        TableAlertOptionRender(func(keys []string, clear func()) {
            if ui.Button(c, "批量删除").Small().Danger().Clicked() {
                // 删除 keys 后清空选择并刷新
                clear()
                pt.Reload()
            }
        }).
        Request(func(req ui.ProRequest, done func(ui.ProResult[rule])) {
            rows, total := queryRules(req)
            done(ui.ProResult[rule]{Rows: rows, Total: total})
        })
    pt.Show()
}
```

`Reload()`、`Rows()`（当前页数据）、`Loading()`、`Table()`（内部的 `*ui.TableW`）在 `Show` 之后可用；像上面这样在回调里通过变量 `pt` 调用即可。

### 轮询

`Polling(d)` 在上一次结果到达后每隔 `d` 重新请求当前页；轮询请求不显示加载遮罩，数据静默刷新。`PausePolling()` / `ResumePolling()` 暂停和恢复（恢复后在下一个间隔到期时请求），`PollingPaused()` 读取状态。

```go
func pollingTable(c *ui.Context, cols []ui.ProColumn[rule]) {
    var pt *ui.ProTableW[rule]
    pt = ui.ProTable(c, cols).NoSearch().
        Polling(5 * time.Second).
        ToolBar(func() {
            label := "暂停轮询"
            if pt != nil && pt.PollingPaused() {
                label = "恢复轮询"
            }
            if ui.Button(c, label).Clicked() && pt != nil {
                if pt.PollingPaused() {
                    pt.ResumePolling()
                } else {
                    pt.PausePolling()
                }
            }
        }).
        Request(func(req ui.ProRequest, done func(ui.ProResult[rule])) {
            rows, total := queryRules(req)
            done(ui.ProResult[rule]{Rows: rows, Total: total})
        })
    pt.Show()
}
```

### 列状态

用户可以在列设置里调整列的顺序、显隐和固定（左 / 右 / 不固定），拖动表头边缘调整列宽。这些都记录在 `ui.TableColumnState`（`Order`、`Hidden`、`Widths`、`Fixed`）里：

- `ColumnsState(&state)` 受控绑定（对应 `columnsState.value`），应用可以读取或重置它。
- `OnColumnsStateChange(fn)` 在用户修改后回调（对应 `columnsState.onChange`）。
- `PersistColumnsState(key, store)` 首次显示时从 `store` 读取、之后每次修改写回（对应 `columnsState.persistenceKey`）。`store` 实现 `ui.ColumnsStateStore` 接口，存在哪里（内存、配置文件、数据库）由应用决定，表格本身不读写磁盘；用两个函数时可以直接用 `ui.ColumnsStateFuncs` 适配。

```go
var (
    colState ui.TableColumnState
    saved    = map[string]ui.TableColumnState{} // 换成配置文件或数据库
)

func persistentTable(c *ui.Context, cols []ui.ProColumn[rule]) {
    store := ui.ColumnsStateFuncs{
        Load: func(k string) (ui.TableColumnState, bool) { s, ok := saved[k]; return s, ok },
        Save: func(k string, s ui.TableColumnState) { saved[k] = s },
    }
    ui.ProTable(c, cols).
        ColumnsState(&colState).
        PersistColumnsState("rules-table", store).
        Request(func(req ui.ProRequest, done func(ui.ProResult[rule])) {
            rows, total := queryRules(req)
            done(ui.ProResult[rule]{Rows: rows, Total: total})
        }).
        Show()
}
```

### 展开行、拖拽排序与行样式

`Expandable(fn)` 为每行加详情面板，`fn` 收到该行数据；`DragSort(fn)` 加拖拽把手（即 Pro 的 DragSortTable），`fn(from, to)` 收到当前页内的行号，应用在服务端重排后 `Reload()`，或用 `MovePageRow(from, to)` 只在本页内移动；`RowClass(fn)` 按行返回 `ui.RowStyle`。

```go
func detailTable(c *ui.Context, cols []ui.ProColumn[rule]) {
    var pt *ui.ProTableW[rule]
    pt = ui.ProTable(c, cols).NoSearch().
        Expandable(func(r rule) {
            ui.Text(c, "规则 "+r.Name+" 的详细配置")
        }).
        DragSort(func(from, to int) { pt.MovePageRow(from, to) }).
        RowClass(func(r rule) ui.RowStyle {
            if r.Status == "error" {
                return ui.RowStyle{Status: "error"}
            }
            return ui.RowStyle{}
        }).
        Request(func(req ui.ProRequest, done func(ui.ProResult[rule])) {
            rows, total := queryRules(req)
            done(ui.ProResult[rule]{Rows: rows, Total: total})
        })
    pt.Show()
}
```

## 主题与样式

| 风格 | 差异 |
| --- | --- |
| Ant Design | 对照 Pro Components：查询表单和表格各放在一张白色卡片里，工具栏在表格卡片顶部。 |
| Element Plus | 结构同 Ant Design，表格、表单、分页器使用 Element Plus 的样式。 |
| shadcn | Data Table 布局：查询表单和工具栏直接放在页面背景上，表格外是一圈圆角边框，底部是弱化的选择数 / 总数文字和「Previous / Next」描边按钮。 |

ProTable 由 Table、QueryFilter、Pagination 等组件组合而成，样式随它们的组件令牌变化（`theme.Components.Table`、`theme.Components.Pagination`、`theme.Components.Form` 等），没有单独的令牌。

`ConfigProvider.Defaults("ProTable", ...)` 支持的键：`"search"`、`"bordered"`、`"options"`（bool），`"pagination"`（`false`、每页条数，或 `ui.Props{"pageSize": n}`），`"size"`（初始密度：`"small"` / `"middle"` / `"large"`），`"headerTitle"`（string），`"polling"`（`time.Duration` 或毫秒数）。

```go
func proDefaults(c *ui.Context, cols []ui.ProColumn[rule]) {
    ui.ConfigProvider(c).Defaults("ProTable", ui.Props{"size": "middle", "pagination": 20}).Body(func() {
        ui.ProTable(c, cols).Request(func(req ui.ProRequest, done func(ui.ProResult[rule])) {
            rows, total := queryRules(req)
            done(ui.ProResult[rule]{Rows: rows, Total: total})
        }).Show()
    })
}
```

## 注意事项

- **状态放在哪**：页码、每页条数、查询条件、排序、筛选、密度和当前页数据都由 ProTable 按自身的组件 ID 保存，列定义和 `Request` 闭包可以每帧重新构造。在条件分支或循环里渲染多个 ProTable 时，用 `ui.Key(c, key, fn)` 区分；需要从外部控制的状态用 `Selection`、`ColumnsState`、`Params` 绑定。
- **不要阻塞 UI 线程**：`Request` 在 UI 线程上调用，网络、数据库等阻塞操作放进 goroutine 再调用 `done`；`done` 自带重绘，无需再 `Invalidate`。
- **请求时机**：只有请求参数（页码、条数、排序、筛选、查询条件、`Params`）变化或调用 `Reload()` 时才会重新请求，每帧调用 `Show()` 不会重复请求。数据在后台变化后调用 `Reload()`，或用 `Polling` 定时刷新。
- **行键**：开启 `Selection` 时一定要设置 `RowKey`，选择按键跨页保留。
- **键盘与无障碍**：表格部分的键盘操作与 [Table](/components/table#注意事项) 相同；内部表格的无障碍名称自动取 `Title`，密度菜单、列设置面板和拖拽把手都带有角色与名称。

## 示例代码

<!--@include: ./_gen/protable-examples.md-->

## API

<!--@include: ./_gen/protable-api.md-->

## 相关链接

- 源码：[ui/table_pro.go](https://github.com/tangoui/tango/blob/main/ui/table_pro.go)
- [ui Go API 参考](/reference/api/ui)
- [Table 表格](/components/table)、[Pagination 分页](/components/pagination)
- [组件总览](/components/)、[组件截图目录](/components/screenshots)
