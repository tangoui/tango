---
title: Transfer 穿梭框
---
# Transfer 穿梭框

在两个列表之间移动条目，常用于授权和成员分配。

## 何时使用

- 从较大的候选集中选择多个对象，并明确展示已选结果。
- 选择项较少时使用 [Checkbox 多选框](/components/checkbox)。

## 在线体验

<GalleryPreview component="Transfer" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

var selectedUsers = []string{"u2"}

func page(c *ui.Context) {
    items := []ui.TransferItem{{Key: "u1", Title: "张三"}, {Key: "u2", Title: "李四"}}
    ui.Transfer(c, items, &selectedUsers)
}
```

右侧列表绑定 `target` 中的 key，左侧为其余条目。

## 常见用法

### 搜索和标题

```go
func search(c *ui.Context, items []ui.TransferItem, target *[]string) {
    ui.Transfer(c, items, target).Titles("候选人", "已选").Searchable().ListSize(240, 280)
}
```

### 默认勾选和操作文案

```go
func defaults(c *ui.Context, items []ui.TransferItem, target *[]string) {
    ui.Transfer(c, items, target).DefaultChecked("u1").Operations("加入", "移除")
}
```

### 状态和禁用

```go
func disabled(c *ui.Context, items []ui.TransferItem, target *[]string) {
    ui.Transfer(c, items, target).Status("error").Disabled(false)
}
```

## 主题与样式

Transfer 令牌控制列表尺寸、头部、选中项、操作按钮和错误状态。条目的 `Disabled` 可禁止移动。

## 注意事项

目标 key 列表是业务状态，应持久保存；`Changed` 表示本帧发生移动。大量条目建议开启搜索并控制列表高度，保持键盘焦点可在列表间移动。

## 示例代码

<!--@include: ./_gen/transfer-examples.md-->

## API

<!--@include: ./_gen/transfer-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/entry_transfer.go)
- [Checkbox 多选框](/components/checkbox)
