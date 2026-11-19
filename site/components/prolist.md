---
title: ProList 高级列表
---
# ProList 高级列表

用 metas 描述列表项的标题、描述、头像、内容和操作，支持选择、展开、分页和卡片网格。

## 何时使用

- 项目、应用、文章等需要统一列表项结构的页面。
- 需要远程分页、批量选择或卡片网格时使用；复杂多列数据使用 [ProTable 高级表格](/components/protable)。

## 在线体验

<GalleryPreview component="ProList" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

type project struct{ Key, Name, Desc string }

func page(c *ui.Context) {
    rows := []project{{"p1", "TanGo", "UI 框架"}, {"p2", "MinCode", "IDE"}}
    ui.ProList(c, rows, ui.ProListMetas[project]{
        Title: func(r project) string { return r.Name },
        Description: func(r project) string { return r.Desc },
    }).HeaderTitle("项目列表").RowKey(func(r project) string { return r.Key }).Show()
}
```

## 常见用法

### 选择、展开和分页

```go
var selectedProjects []string

func selectable(c *ui.Context, rows []project, metas ui.ProListMetas[project]) {
    ui.ProList(c, rows, metas).RowKey(func(r project) string { return r.Key }).
        Selection(&selectedProjects).Pagination(5).Expandable(func(c *ui.Context, r project) {
            ui.Text(c, r.Desc)
        }).Show()
}
```

### 卡片网格

```go
func cards(c *ui.Context, rows []project, metas ui.ProListMetas[project]) {
    ui.ProList(c, rows, metas).Grid(3).Bordered().Show()
}
```

### 远程分页

```go
func remote(c *ui.Context, metas ui.ProListMetas[project]) {
    ui.ProList(c, nil, metas).Request(func(page, size int) ([]project, int, error) {
        return []project{}, 0, nil
    }).Pagination(10).Show()
}
```

### 长列表：虚拟滚动

`Height(px)` 固定条目区高度，再加 `Virtual()` 只构建视口内的条目（同 [List](/components/list) 的虚拟形态）；工具栏、选择提示和分页器留在区外。条目高度（含展开后的高度）滚到时实测、按 `RowKey` 缓存，未测到的按 `EstimatedItemHeight(px)` 估算。选择和展开状态按键保存，与条目是否被构建无关。条目内的控件滚出视口时会失去键盘焦点。

```go
func longProList(c *ui.Context, rows []project, metas ui.ProListMetas[project]) {
    ui.ProList(c, rows, metas).RowKey(func(r project) string { return r.Key }).
        Height(480).Virtual().Show()
}
```

## 主题与样式

ProList 复用 List、Card 和 Pro 工具栏令牌；`Bordered`、`Split`、`Grid` 控制列表和卡片形态。

## 注意事项

`RowKey` 用于保存选择和展开状态，应返回稳定唯一值。远程请求在页码变化时触发，`Reload` 可手动刷新；大量本地数据应启用分页、远程请求或 `Height(px).Virtual()`。

## 示例代码

<!--@include: ./_gen/prolist-examples.md-->

## API

<!--@include: ./_gen/prolist-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/pro3_list.go)
- [List 列表](/components/list)
