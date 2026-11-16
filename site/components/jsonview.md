---
title: JSONView 值树查看器
---
# JSONView 值树查看器

以可折叠的树展示一份 JSON 文档、一个 Go 值，或任何自定义的 `ValueSource`（例如调试器的变量、远程文档）。节点在展开时才加载，大数组每 100 项自动分组，只构建可见的行，几兆字节的文档也能立即打开。

## 何时使用

- 查看接口响应、配置文件、日志里的 JSON，或在调试界面里检查任意 Go 值/变量树，需要折叠、搜索、复制某个值或路径时使用。
- 数据量很大、层级很深时尤其合适：展开才读取，不会一次性构建整棵树。
- 要把对象的少量字段排成键值表用 [Descriptions 描述列表](/components/descriptions)；通用的层级数据（文件树、组织结构）用 [Tree 树形控件](/components/tree)；想原样展示 JSON 文本并保留格式用 [CodeBlock 代码块](/components/codeblock)。

## 在线体验

<GalleryPreview component="JSONView" />

## 基本用法

`ui.JSONView` 接收一份 JSON 文档（`[]byte`）。文档不合法时，查看器顶部会显示解析错误提示而不是树。

```go
import "github.com/tangoui/tango/ui"

var doc = []byte(`{
  "name": "tango",
  "version": "0.4.0",
  "private": false,
  "license": null,
  "keywords": ["gui", "go"],
  "author": {"name": "Tango Team", "url": "https://example.com"}
}`)

func page(c *ui.Context) {
    ui.JSONView(c, doc).Height(320).ExpandDepth(2)
}
```

`Height` 固定整个查看器（含工具栏）的高度，不设置时在父容器里伸展，因此父容器需要明确高度。`ExpandDepth(n)` 决定首次显示时展开的层数，默认 1，即只展开根节点、显示它的直接成员。值按类型着色，对象显示键数、数组显示项数。

**数据身份**：JSONView 用切片的首地址和长度识别“同一份数据”，同一份切片会保留展开状态、选中行和搜索；换成另一个切片才会重新加载。所以把文档放在包级变量或状态里长期持有，不要每帧 `[]byte(str)` 重新转换。

## 常见用法

### 查看 Go 值

`ui.JSONViewValue` 通过反射展开结构体（导出字段，名称取 `json` 标签）、map（键排序）、切片、数组、指针和接口；带 `String` 或 `Error` 方法且不是容器的值显示该文本。

```go
import "github.com/tangoui/tango/ui"

type Profile struct {
    Age    int     `json:"age"`
    Score  float64 `json:"score"`
    Active bool    `json:"active"`
}

type User struct {
    ID      int               `json:"id"`
    Name    string            `json:"name"`
    Roles   []string          `json:"roles"`
    Profile *Profile          `json:"profile"`
    Labels  map[string]string `json:"labels"`
}

var users = []User{
    {ID: 1, Name: "张三", Roles: []string{"admin"}, Profile: &Profile{Age: 31, Score: 98.5, Active: true}, Labels: map[string]string{"team": "core"}},
    {ID: 2, Name: "Alice"},
}

func goValuePage(c *ui.Context) {
    ui.JSONViewValue(c, &users).Height(260).ExpandDepth(3).NoToolbar().ShowTypes()
}
```

传指针、map 或切片时，JSONView 用它们的地址识别身份，展开状态跨帧保持；传不可寻址的普通值（结构体副本、数字）没有身份，每帧都重新读取，只保留已展开的节点。数据被修改后，用 `DataVersion(n)` 通知它重新加载（保留展开状态）：

```go
import "github.com/tangoui/tango/ui"

var (
    cfg        = map[string]any{"debug": false, "retries": 3}
    cfgVersion int
)

func reloadPage(c *ui.Context) {
    if ui.Button(c, "改配置").Clicked() {
        cfg["retries"] = 5
        cfgVersion++ // 内容变了，版本号 +1
    }
    ui.JSONViewValue(c, cfg).Height(200).DataVersion(cfgVersion)
}
```

`ShowTypes()` 在值后面显示类型标签；`NoToolbar()` 隐藏搜索与展开/折叠工具栏。

### 大数组分组与懒加载

对象或数组的成员超过 100 个时，展开后不是平铺所有成员，而是显示 `[0 … 99]`、`[100 … 199]` 这样的分组，分组本身也要点开才加载；超过一万项时先按每组一万项分，展开后再按 100 项一组细分。对 JSON 文档，每个容器的成员偏移按每 100 项建立一次检查点，所以展开大数组中间的一组不需要从头扫描。你不用做任何设置，这是自动的。

```go
import (
    "fmt"
    "strings"

    "github.com/tangoui/tango/ui"
)

func bigJSON() []byte {
    var items []string
    for i := 0; i < 5000; i++ {
        items = append(items, fmt.Sprintf(`{"id": %d, "ok": %t}`, i, i%3 != 0))
    }
    return []byte(`{"items": [` + strings.Join(items, ",") + `]}`)
}

var bigDoc = bigJSON()

func bigPage(c *ui.Context) {
    ui.JSONView(c, bigDoc).Height(360).ExpandDepth(2)
}
```

### 搜索

工具栏的搜索框在键和值中查找（Enter 或箭头在命中间跳转，命中的路径会自动展开）。把查询绑定到自己的变量，就可以从外部驱动搜索，或配合 `NoToolbar()` 自画搜索框：

```go
import (
    "fmt"

    "github.com/tangoui/tango/ui"
)

var (
    jsonQuery string
    searchDoc = []byte(`{"users": [{"name": "alice", "city": "Hangzhou"}, {"name": "bob", "city": "Shanghai"}]}`)
)

func searchPage(c *ui.Context) {
    ui.Input(c, &jsonQuery).Placeholder("搜索键或值")
    v := ui.JSONView(c, searchDoc).Height(240).ExpandDepth(1).NoToolbar().Query(&jsonQuery)
    ui.Text(c, fmt.Sprintf("命中 %d 处，共 %d 行", v.Matches(), v.Rows()))
}
```

搜索在后台逐帧推进，大文档中 `Matches()` 可能还在增长；一次搜索最多访问 20 万个节点。

### 复制值与路径

在行上点右键，菜单有“复制值”和“复制路径”；当前行聚焦时 `Cmd/Ctrl+C` 复制值，`Cmd/Ctrl+Shift+C` 复制路径。路径形如 `$.author.name`、`$.items[3].id`，键名不是合法标识符时写成 `$["a b"]` 的形式。字符串值复制时不带引号，对象和数组复制为其 JSON 文本（对 JSON 文档是原文，对 Go 值是序列化结果）。可以读取这一帧复制了什么，并读取当前行的路径：

```go
import (
    "fmt"

    "github.com/tangoui/tango/ui"
)

var lastCopied string

func copyPage(c *ui.Context, data []byte) {
    v := ui.JSONView(c, data).Height(240)
    if s := v.Copied(); s != "" {
        lastCopied = s
    }
    ui.Text(c, fmt.Sprintf("当前行：%s；最近复制：%s", v.Current(), lastCopied))
}
```

`Copied()` 只在复制发生的那一帧返回内容，所以要像上面这样保存下来。键盘操作：树是一个 Tab 停靠点，上下方向键、`PageUp` / `PageDown`、`Home` / `End` 移动当前行，右方向键展开或进入子节点，左方向键折叠或回到父节点，`Enter` / 空格切换展开。

### 自定义数据源：JSONViewSource

数据不在内存里、需要按需向别处取（调试器变量、远程接口、文件系统）时，实现 `ui.ValueSource` 两个方法：`Roots()` 返回顶层节点，`Children(parent, from, to)` 返回 `[from, to)` 范围内的子节点（`to` 为 -1 表示全部）。节点的 `Count` 为 -1 表示数量未知，`Ref` 是你自己的句柄，会原样传回 `Children`。数据还没到时返回 `ok=false`，视图会显示加载动画并在之后再次询问；数据到达后调用 `Context.Invalidator()` 的函数让界面立即重绘。

```go
import (
    "os"
    "path/filepath"
    "sync"

    "github.com/tangoui/tango/ui"
)

// dirSource 把目录树当作值树：目录在展开时才异步读取。
type dirSource struct {
    root  string
    wake  func()
    mu    sync.Mutex
    cache map[string][]ui.ValueNode
    busy  map[string]bool
}

func (s *dirSource) Roots() []ui.ValueNode {
    return []ui.ValueNode{{Name: s.root, Kind: ui.ValueObject, Count: -1, Ref: s.root}}
}

func (s *dirSource) Children(parent *ui.ValueNode, from, to int) ([]ui.ValueNode, bool) {
    dir, _ := parent.Ref.(string)
    s.mu.Lock()
    defer s.mu.Unlock()
    kids, ok := s.cache[dir]
    if !ok {
        if !s.busy[dir] {
            s.busy[dir] = true
            go s.load(dir)
        }
        return nil, false // 还在加载
    }
    if to < 0 || to > len(kids) {
        to = len(kids)
    }
    if from >= to {
        return nil, true
    }
    return kids[from:to], true
}

func (s *dirSource) load(dir string) {
    entries, _ := os.ReadDir(dir)
    kids := make([]ui.ValueNode, 0, len(entries))
    for _, e := range entries {
        n := ui.ValueNode{Name: e.Name(), Ref: filepath.Join(dir, e.Name())}
        if e.IsDir() {
            n.Kind, n.Count = ui.ValueObject, -1
        } else {
            n.Kind, n.Value = ui.ValueOther, "文件"
        }
        kids = append(kids, n)
    }
    s.mu.Lock()
    s.cache[dir] = kids
    delete(s.busy, dir)
    s.mu.Unlock()
    s.wake()
}

var dirs *dirSource

func sourcePage(c *ui.Context) {
    if dirs == nil {
        dirs = &dirSource{root: ".", wake: c.Invalidator(), cache: map[string][]ui.ValueNode{}, busy: map[string]bool{}}
    }
    ui.JSONViewSource(c, dirs).Height(320)
}
```

`JSONViewSource` 用数据源值本身（可比较的类型，通常是指针）作为身份：同一个指针保持展开状态，换一个新的才重新加载；每帧新建一个数据源会让树每帧重置。数据变了而数据源对象不变时，用 `DataVersion` 触发重新加载。数据源还可以实现 `ui.ValueCopier`（`CopyValue(*ValueNode) string`）：当节点显示的是缩短后的值时，让“复制值”复制完整内容。

如果只是想先校验再显示 JSON，可以自己调用 `ui.JSONSource(data)`，它返回 `(ValueSource, error)`，再交给 `JSONViewSource`；`ui.GoValueSource(v)` 对应 Go 值。

## 主题与样式

JSONView 的令牌是 `theme.Components.JSONView`（`theme.JSONViewToken`）：

- 行与缩进：`RowHeight`、`Indent`（每层缩进）、`ValueFontSize`、`Radius`、`RowHoverBg`、`RowSelectedBg`、`SwitcherColor`。
- 值的类型色：`KeyColor`、`StringColor`、`NumberColor`、`BoolColor`、`NullColor`、`OtherColor`（函数、指针、调试器的不透明值）。
- 辅助文字：`MetaColor`（项数、类型、分组范围）、`PunctColor`（冒号和括号）。
- 搜索高亮：`MatchBg`、`MatchCurrentBg`、`MatchCurrentBorder`。

三套风格（Ant Design / Element Plus / shadcn）各自派生这些颜色，并适配浅色、深色与高对比主题。覆盖方式：

```go
import "github.com/tangoui/tango/ui/theme"

func jsonTheme() *theme.Theme {
    return theme.AntDesign(theme.WithComponents(theme.Components{
        JSONView: theme.JSONViewToken{RowHeight: 24, Indent: 14},
    }))
}
```

文案（搜索占位、项数、键数、复制菜单、展开/折叠提示、解析错误）来自本地化表 `Locale().JSONView`（`ui.JSONViewLocale`）。JSONView 没有 `ConfigProvider.Defaults` 键。

## 注意事项

- **状态与数据身份**：展开状态、当前行、搜索结果按 JSONView 的节点 ID 和数据身份保存。`JSONView` 以切片首地址和长度为身份，`JSONViewValue` 以指针/map/切片的地址为身份，`JSONViewSource` 以数据源值为身份。每帧生成新数据（`[]byte(s)` 转换、新建数据源、传结构体副本）会导致每帧重新加载；长期持有数据，变化时用 `DataVersion`。
- **数据不可在使用中被修改**：`JSONSource` 对 `data` 做懒扫描，使用期间不要原地改写这块内存，要换内容就换一个新的切片。
- **性能**：只构建视口内的行，展开时才读取对应容器，所以大文档不需要特别处理。自定义数据源的 `Children` 在界面线程调用，耗时的读取放到 goroutine 里，用 `ok=false` 表示未就绪。
- **多个查看器**：同一位置渲染多个 JSONView，且列表会增删重排时，用 `ui.Key(c, 稳定的键, fn)` 包住，状态才会跟着数据走。
- **无障碍**：树作为可展开的树视图暴露给读屏，行带有展开/收起、忙碌状态；用 `A11yLabel` 指定名称。搜索框和工具栏按钮带本地化标签，整棵树可以只用键盘操作。

## 示例代码

<!--@include: ./_gen/jsonview-examples.md-->

## API

<!--@include: ./_gen/jsonview-api.md-->

## 相关链接

- [源码：jsonview.go](https://github.com/tangoui/tango/blob/main/ui/jsonview.go)、[jsonview_source.go](https://github.com/tangoui/tango/blob/main/ui/jsonview_source.go)
- [ui Go API](/reference/api/ui)
- [CodeBlock 代码块](/components/codeblock)
- [LogView 日志查看器](/components/logview)
- [Tree 树形控件](/components/tree)
- [Descriptions 描述列表](/components/descriptions)
