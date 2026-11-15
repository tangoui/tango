---
title: Timeline 时间轴
---
# Timeline 时间轴

按时间或流程顺序展示一组事件节点。

## 何时使用

- 操作记录、审批流、发布历史和物流轨迹。
- 需要事件之间的轴线关系时使用；简单步骤导航用 [Steps 步骤条](/components/steps)。

## 在线体验

<GalleryPreview component="Timeline" />

## 基本用法

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    ui.Timeline(c, []ui.TimelineItem{
        {Content: "创建项目", Time: "09:00", Color: "success"},
        {Content: "提交审核", Time: "10:30", Color: "processing"},
        {Content: "等待发布", Time: "进行中", Hollow: true},
    }).Show()
}
```

## 常见用法

### 交替布局

```go
func alternate(c *ui.Context, items []ui.TimelineItem) {
    ui.Timeline(c, items).Mode("alternate").Reverse().Show()
}
```

### 横向时间轴

```go
func horizontal(c *ui.Context) {
    ui.Timeline(c, []ui.TimelineItem{{Content: "开始"}, {Content: "完成"}}).Horizontal().Show()
}
```

### 自定义节点和待处理项

`Body` 或 `Dot` 可构建自定义内容，`Pending` 在末尾追加旋转中的节点。

```go
func pending(c *ui.Context) {
    ui.Timeline(c, []ui.TimelineItem{{Content: "已完成"}}).Pending("下一步处理中").Show()
}
```

## 主题与样式

`Mode` 支持 `left`、`right`、`alternate`、`start`、`end`；默认键为 `mode` 和 `reverse`。颜色状态和空心节点由 Timeline 令牌统一适配三套主题。

## 注意事项

事件数组顺序就是渲染顺序；实时数据到达时替换外部数组。时间文本要与内容形成可读的语义，不要只用颜色区分状态。

## 示例代码

<!--@include: ./_gen/timeline-examples.md-->

## API

<!--@include: ./_gen/timeline-api.md-->

## 相关链接

- [源码](https://github.com/tangoui/tango/blob/main/ui/data_timeline.go)
- [Steps 步骤条](/components/steps)
