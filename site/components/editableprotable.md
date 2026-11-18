---
title: EditableProTable 可编辑表格
---
# EditableProTable 可编辑表格

在 `ProTable` 的列定义上增加行内编辑、新增、校验、异步保存、删除和拖拽排序能力，并把成功结果写回应用拥有的 `[]T`。

## 何时使用

- 列表中的少量字段需要直接编辑、保存或取消，而不想打开独立表单时。
- 新增行需要先经过必填、规则或异步查重，再加入受控数据时。
- 需要同时展示格式化只读列和少量可编辑列时。
- 复杂跨步骤编辑、字段很多或需要强制聚焦时，用 [ProForm 高级表单](/components/proform) 或 [ModalForm 弹窗表单](/components/modalform) 更合适。

## 在线体验

<GalleryPreview component="EditableProTable" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见[打开自绘窗口](/guide/ui#打开一个自绘窗口)。每个可编辑列必须同时提供 `Editor` 和 `Set`，否则该列只读。

```go
package main

import "github.com/tangoui/tango/ui"

type person struct {
    ID   string
    Name string
    Age  float64
}

var people = []person{{ID: "p1", Name: "Alice", Age: 30}}

var personColumns = []ui.ProColumn[person]{
    {Key: "name", Title: "姓名", Value: func(r person) any { return r.Name },
        Editor: &ui.ProField{Required: true},
        Set: func(r *person, v any) { r.Name, _ = v.(string) }},
    {Key: "age", Title: "年龄", ValueType: ui.ValueDigit,
        Value: func(r person) any { return r.Age },
        Editor: &ui.ProField{Min: 0, Max: 150},
        Set: func(r *person, v any) { r.Age, _ = v.(float64) }},
    {Key: "op", Title: "操作", ValueType: ui.ValueOption},
}

func page(c *ui.Context) {
    ui.EditableProTable(c, personColumns, &people).
        Title("人员").
        RowKey(func(r person) string { return r.ID }).
        Show()
}

func main() {}
```

点击行操作中的编辑后，`Editor` 根据列的 `ValueType` 生成控件；保存前运行字段规则，成功后通过 `Set` 把草稿写回 `*rows`。默认一次只编辑一行。

## 常见用法

### 新增行与异步保存

`NewRow` 显示“添加一行数据”，新行先处于编辑状态，保存通过校验后才加入 `*rows`。`OnSave` 收到的 `index` 对已有行是原索引，对新行是 `-1`。

```go
package main

import "github.com/tangoui/tango/ui"

type item struct{ ID, Title string }

var (
    items = []item{{ID: "i1", Title: "第一项"}}
    nextID = 1
    itemColumns = []ui.ProColumn[item]{
        {Key: "title", Title: "标题", Value: func(r item) any { return r.Title },
            Editor: &ui.ProField{Required: true},
            Set: func(r *item, v any) { r.Title, _ = v.(string) }},
    }
)

func page(c *ui.Context) {
    ui.EditableProTable(c, itemColumns, &items).
        RowKey(func(r item) string { return r.ID }).
        NewRow(func() item {
            nextID++
            return item{ID: "new" + string(rune('0'+nextID))}
        }).
        OnSave(func(row item, index int, done func(error)) {
            go done(saveItem(row, index))
        }).
        Show()
}

func saveItem(row item, index int) error { return nil }

func main() {}
```

`OnSave` 返回错误时，行保持编辑状态并显示错误；返回 `nil` 时才写回数据。实际项目中应使用稳定且唯一的 `RowKey`，不要依赖默认行索引。

### 受控编辑、多行编辑和限制行

`EditableKeys` 绑定正在编辑的行 key，页面可以预先打开某行或在外部控制编辑集合。默认只允许一行，`Multiple()` 允许多行；`RowEditable` 可以按业务状态限制编辑。

```go
package main

import "github.com/tangoui/tango/ui"

type task struct{ ID, Name, State string }

var (
    tasks = []task{{ID: "t1", Name: "设计", State: "open"}, {ID: "t2", Name: "发布", State: "done"}}
    editing = []string{"t1"}
    taskColumns = []ui.ProColumn[task]{
        {Key: "name", Title: "任务", Value: func(r task) any { return r.Name },
            Editor: &ui.ProField{Required: true}, Set: func(r *task, v any) { r.Name, _ = v.(string) }},
        {Key: "state", Title: "状态", ValueType: ui.ValueSelect,
            ValueEnum: []ui.ProEnum{{Value: "open", Label: "进行中"}, {Value: "done", Label: "完成"}},
            Value: func(r task) any { return r.State },
            Editor: &ui.ProField{}, Set: func(r *task, v any) { r.State, _ = v.(string) }},
    }
)

func page(c *ui.Context) {
    table := ui.EditableProTable(c, taskColumns, &tasks).
        RowKey(func(r task) string { return r.ID }).
        EditableKeys(&editing).
        Multiple().
        RowEditable(func(r task) bool { return r.State != "done" }).
        Show()
    if table.Changed() {
        syncEditingRows(editing)
    }
}

func syncEditingRows(keys []string) {}

func main() {}
```

受控 key 必须与 `RowKey` 返回值一致；删除或重排数据后同步更新 `editing`，避免保留不存在的 key。

### 字段规则与异步校验

`Editor` 可以配置 `Required`、`Rules`、`Validate` 和 `AsyncValidate`。同步规则先执行，全部通过后才运行异步校验；校验期间 `Validating(key)` 为 `true`。

```go
package main

import "github.com/tangoui/tango/ui"

type account struct{ ID, Name string }

var accounts = []account{{ID: "a1", Name: "TanGo"}}
var accountColumns = []ui.ProColumn[account]{
    {Key: "name", Title: "名称", Value: func(r account) any { return r.Name },
        Editor: &ui.ProField{
            Required: true,
            Rules: []ui.Rule{ui.MaxLen(20, "最多 20 个字符")},
            AsyncValidate: func(v any, done func(string)) {
                go func() {
                    if v == "已占用" {
                        done("名称已存在")
                        return
                    }
                    done("")
                }()
            },
        },
        Set: func(r *account, v any) { r.Name, _ = v.(string) }},
}

func page(c *ui.Context) {
    table := ui.EditableProTable(c, accountColumns, &accounts).
        RowKey(func(r account) string { return r.ID }).
        Show()
    if table.Validating("a1") {
        ui.Text(c, "正在检查名称……").Tertiary()
    }
}

func main() {}
```

`Errors(key)` 返回该行按列 key 保存的错误消息；异步回调可以从 goroutine 调用 `done`，组件会在 UI 失效后重新检查结果。

### 新增、分页和拖拽排序

`Pagination(n)` 在本地按每页 `n` 行显示，`AddAtTop()` 让草稿行和添加按钮位于表格上方，`DragSort` 在 `*rows` 重排后回调原始索引。

```go
package main

import "github.com/tangoui/tango/ui"

type row struct{ ID, Label string }

var rows = []row{{ID: "1", Label: "一"}, {ID: "2", Label: "二"}, {ID: "3", Label: "三"}}
var columns = []ui.ProColumn[row]{
    {Key: "label", Title: "名称", Value: func(r row) any { return r.Label },
        Editor: &ui.ProField{Required: true}, Set: func(r *row, v any) { r.Label, _ = v.(string) }},
}

func page(c *ui.Context) {
    ui.EditableProTable(c, columns, &rows).
        RowKey(func(r row) string { return r.ID }).
        NewRow(func() row { return row{ID: "new"} }).
        AddAtTop().
        Pagination(10).
        DragSort(func(from, to int) { recordMove(from, to) }).
        Bordered().
        Show()
}

func recordMove(from, to int) {}

func main() {}
```

新行在保存前不会参与已保存数据的拖拽排序；如果需要服务端排序，把移动回调交给保存接口，并在回调中同步应用状态。

### 自定义删除、保存错误和工具栏

`OnDelete` 在确认删除后调用；如果不设置，组件会直接从 `*rows` 删除。`ToolBar` 可以放批量操作或状态信息，`NoDelete()` 隐藏删除动作。

```go
package main

import "github.com/tangoui/tango/ui"

type record struct{ ID, Name string }

var records = []record{{ID: "r1", Name: "记录"}}
var recordColumns = []ui.ProColumn[record]{
    {Key: "name", Title: "名称", Value: func(r record) any { return r.Name },
        Editor: &ui.ProField{}, Set: func(r *record, v any) { r.Name, _ = v.(string) }},
}

func page(c *ui.Context) {
    table := ui.EditableProTable(c, recordColumns, &records).
        Title("记录").
        ToolBar(func() { ui.Button(c, "导出").Small() }).
        RowKey(func(r record) string { return r.ID }).
        OnDelete(func(row record, index int) { archive(row) }).
        NoDelete().
        Show()
    if table.Saved() || table.Deleted() {
        refreshSummary()
    }
}

func archive(row record) {}
func refreshSummary() {}

func main() {}
```

`NoDelete()` 会隐藏删除动作，因此上述示例只演示回调签名；要启用删除时移除它。远程保存使用 `OnSave`，返回非空错误会让当前行保持编辑状态。

### 大量数据：虚拟滚动

`ScrollY(px)` 固定表体高度，只构建视口内的行；`Virtual()` 表明意图（Element Plus 主题下用 Table V2 的 50px 行高），行高默认跟随 `Size`。`CellEditorTable` 和 `RowEditorTable` 有同样的两个方法。

正在编辑的行（或格所在的行）总是构建，即使滚出视口，草稿和焦点也不会丢。开始编辑某行（`Edit`、`AddRow`）、焦点移进正在编辑的行（例如 Tab 到它的下一个编辑器），以及校验失败时，表格会把这一行滚进视口。工具栏和添加按钮留在滚动区外。不设 `ScrollY` 时与原来一样，每一行都构建。

```go
func bigTable(c *ui.Context, cols []ui.ProColumn[row], rows *[]row) {
    ui.RowEditorTable(c, cols, rows).RowKey(func(r row) string { return r.ID }).
        ScrollY(480).Virtual().Show()
}
```

## 主题与样式

EditableProTable 的单元格、编辑控件和操作链接复用 `ProTable`、`Table`、`ProField` 的主题令牌。可通过 `ConfigProvider.Defaults("EditableProTable", ui.Props{...})` 设置 `size`、`bordered`、`pagination`、`maxLength` 默认值；`Bordered`、`Size`、`Pagination` 等实例方法优先。三套主题会调整表格边框、编辑态控件和操作按钮，不改变 `Set` 或校验流程。

## 注意事项

- `rows` 是应用拥有的切片，只有校验和 `OnSave` 成功后才会写回；不要在 `OnSave` 之前直接修改原行来模拟草稿。
- `RowKey` 在新增、删除和拖拽排序时必须稳定且唯一；默认索引只适合不会重排的静态数据。
- `OnSave`、`OnDelete` 和异步校验完成后都要调用回调；异步保存未完成时 `Validating(key)` 为 `true`，不要重复提交同一行。
- 大数据量应使用 `Pagination`、上层的服务端分页或 `ScrollY(px).Virtual()`；编辑器列应只放真正需要修改的字段。虚拟滚动时视口外的行不构建，它们不在无障碍树里（表格的行数仍报告全部行）。
- 操作列和编辑控件已有键盘语义；自定义 `Render` 或 `ToolBar` 中的图标按钮要设置 `A11yLabel`。

## 示例代码

<!--@include: ./_gen/editableprotable-examples.md-->

## API

<!--@include: ./_gen/editableprotable-api.md-->

## 相关链接

- [EditableProTable gallery 示例](https://github.com/tangoui/tango/blob/main/examples/ui-gallery/page_p2e_table.go)
- [ProTable 高级表格](/components/protable)
- [Table 表格](/components/table)
- [ProForm 高级表单](/components/proform)
- [源码](https://github.com/tangoui/tango/blob/main/ui/p2e_editable.go)
- [ui Go API](/reference/api/ui)
