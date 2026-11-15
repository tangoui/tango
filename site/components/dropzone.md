---
title: DropZone 拖入文件高亮
---
# DropZone 拖入文件高亮

为页面中的任意区域提供文件拖入高亮和一次性落盘路径读取，不负责上传本身。

## 何时使用

- 需要让自定义内容在文件拖入时高亮，并由业务自己处理路径时。
- 需要把拖放命中范围和 `Upload`、编辑器或图片处理区分开时。
- 需要完整上传队列、进度和文件状态时，直接用 [Upload 上传](/components/upload)；`Upload().Dragger()` 已经内置拖拽交互。
- 只处理普通点击选择文件时，不必额外包一层 DropZone。

## 在线体验

<GalleryPreview component="DropZone" />

## 基本用法

```go
import (
    "path/filepath"

    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var droppedFiles []string

func page(c *ui.Context) {
    zone := ui.DropZone(c, func(over bool) {
        label := "把文件拖到这里"
        if over {
            label = "松开即可添加"
        }
        ui.Text(c, label)
    }).A11yLabel("附件区").Layout(layout.Pad(24), layout.MinH(120), layout.Center())
    if paths := zone.Dropped(); paths != nil {
        droppedFiles = paths
    }
    for _, path := range droppedFiles {
        ui.Text(c, filepath.Base(path))
    }
}
```

`body` 的 `over` 表示本帧区域是否是文件拖放目标；`Dropped()` 在收到落盘后返回路径，并在读取一次后清空。

## 常见用法

### 自定义悬停内容

根据 `over` 改变提示、边框或内部控件；默认的半透明背景和虚线边框会自动显示。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

func page(c *ui.Context) {
    ui.DropZone(c, func(over bool) {
        if over {
            ui.Text(c, "松开即可添加").Strong()
            return
        }
        ui.Text(c, "拖入图片或文档")
    }).Layout(layout.Grow(1), layout.MinH(160), layout.Center())
}
```

### 关闭默认高亮

当区域内部已有自己的拖拽视觉（例如 `Upload().Dragger()` 或业务自绘边框）时，用 `Highlight(false)` 只保留命中和路径能力。

```go
import "github.com/tangoui/tango/ui"

var uploadFiles []ui.UploadFile

func page(c *ui.Context) {
    ui.DropZone(c, func(bool) {
        ui.Upload(c, &uploadFiles).Dragger()
    }).Highlight(false)
}
```

### 与 Upload 并列

DropZone 负责自定义区域，Upload 负责文件列表、选择和上传状态；两者可以并列呈现，让用户明确不同的处理路径。

```go
import (
    "github.com/tangoui/tango/ui"
    "github.com/tangoui/tango/ui/layout"
)

var files []ui.UploadFile
var lastPaths []string

func page(c *ui.Context) {
    ui.Row(c, func() {
        zone := ui.DropZone(c, func(over bool) {
            text := "把文件拖到这里"
            if over {
                text = "松开即可添加"
            }
            ui.Text(c, text)
        }).Layout(layout.Grow(1), layout.MinH(120), layout.Center())
        if paths := zone.Dropped(); paths != nil {
            lastPaths = paths
        }
        ui.Upload(c, &files).Dragger()
    }, layout.Gap(24))
}
```

### 嵌套区域

嵌套区域中最后构建、且位于拖放位置上的区域优先；外层可以处理未被内层命中的文件。

```go
import "github.com/tangoui/tango/ui"

func page(c *ui.Context) {
    ui.DropZone(c, func(outerOver bool) {
        ui.Text(c, "外层区域")
        ui.DropZone(c, func(innerOver bool) {
            if innerOver {
                ui.Text(c, "内层区域命中")
            }
        })
        if outerOver {
            ui.Text(c, "外层高亮")
        }
    })
}
```

## 主题与样式

- 默认高亮使用当前主题的容器背景、主色和大圆角；`Highlight(false)` 可把视觉完全交给内部内容。
- `Layout` 接受普通布局选项；`A11yLabel` 设置区域的无障碍名称。
- DropZone 没有上传进度或文件列表令牌，相关样式由 `Upload` 和业务内容控制。

## 注意事项

- 系统宿主必须通过 Runner 的文件拖放回调提供悬停和落盘事件；浏览器或没有文件拖放桥接的宿主不会凭空产生路径。
- `Dropped()` 返回的切片只消费一次；应在读取后复制或保存到稳定状态，不要只放在当前帧的局部变量里。
- DropZone 只做命中和反馈，路径的安全检查、大小限制、上传和错误处理仍由业务或 Upload 完成。
- 为可操作区域设置 `A11yLabel`；悬停高亮不是键盘操作的替代，仍应提供点击选择或 Upload 入口。

## 示例代码

<!--@include: ./_gen/dropzone-examples.md-->

## API

<!--@include: ./_gen/dropzone-api.md-->

## 相关链接

- [源码 ui/p2e_drop.go](https://github.com/tangoui/tango/blob/main/ui/p2e_drop.go)
- [ui Go API](/reference/api/ui)
- [Upload 上传](/components/upload)、[Form 表单](/components/form)
