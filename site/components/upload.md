---
title: Upload 上传
---
# Upload 上传

选择或拖入文件，并把文件上传到服务器或交给应用自行处理。支持按钮、拖拽区和照片墙三种触发方式，以及文本列表、图片列表两种文件列表。

## 何时使用

- 需要上传一个或多个文件（附件、头像、图片集）时。
- 需要显示上传进度、失败重试，或限制文件类型、大小和数量时。
- 只想让某块区域在文件拖入时高亮、自己处理拖入的文件时，用 [DropZone 拖入文件高亮](/components/catalog/dropzone)。

## 在线体验

<GalleryPreview component="Upload" />

## 基本用法

下面代码放在应用的页面文件中；完整的 `main` 和窗口初始化见 [打开自绘窗口](/guide/ui#打开一个自绘窗口)。

```go
import (
	"github.com/tangoui/tango/ui"
)

var files []ui.UploadFile

func page(c *ui.Context) {
	ui.Upload(c, &files).
		Action("https://example.com/api/upload").
		Accept(".pdf,.docx").
		MaxSize(10 << 20).
		Tip("支持 pdf / docx，单个文件不超过 10MB").
		Width(360)
}
```

文件列表绑定到 `*[]ui.UploadFile`。`Action(url)` 让组件把每个新加入的文件以 `multipart/form-data` POST 到该地址（字段名 `file`），列表里实时显示进度；服务器返回状态码 ≥ 400 或网络错误时，文件标记为失败并显示重试按钮。`Headers`、`Data`、`FieldName`、`Method`、`Retries`、`Client` 可进一步调整请求。

点击按钮打开的文件选择框由 `Picker` 提供，桌面应用通常在入口处用 `ConfigProvider` 统一配置为系统对话框（见下文“接入系统文件对话框”）。

## 常见用法

### 自定义上传请求

`CustomRequest` 替换内置的 HTTP 请求（对应 antd `customRequest`），适合走自己的 SDK、对象存储或需要签名的接口。函数在独立的 goroutine 中运行，通过 `progress(0..100)` 汇报进度，返回响应内容或错误；文件被移除时 `ctx` 会被取消。失败同样显示重试按钮，`Retries` 也生效。

```go
import (
	"context"
	"os"

	"github.com/tangoui/tango/ui"
)

var attachments []ui.UploadFile

func uploadToStorage(ctx context.Context, name string, data []byte) error { return nil }

func page(c *ui.Context) {
	ui.Upload(c, &attachments).Multiple().MaxCount(5).
		CustomRequest(func(ctx context.Context, f ui.UploadFile, progress func(float64)) ([]byte, error) {
			data, err := os.ReadFile(f.Path)
			if err != nil {
				return nil, err
			}
			progress(50)
			if err := uploadToStorage(ctx, f.Name, data); err != nil {
				return nil, err
			}
			progress(100)
			return nil, nil
		})
}
```

没有设置 `Action` 和 `CustomRequest` 时，Upload 只负责收集文件：新文件直接以 `"done"` 状态加入列表，并通过 `Added()` 交给应用；应用自己上传时，直接修改绑定切片里元素的 `Status`、`Percent`、`Err` 即可更新界面。

### 手动上传

`AutoUpload(false)` 让新加入的文件先以 `"ready"` 状态停在列表里（对应 Element Plus `auto-upload="false"`），之后调用 `Submit()` 一次性上传全部待上传文件，返回开始上传的数量。

```go
import (
	"fmt"

	"github.com/tangoui/tango/ui"
)

var pending []ui.UploadFile

func page(c *ui.Context) {
	up := ui.Upload(c, &pending).AutoUpload(false).ButtonText("选择文件").Width(360).
		Action("https://example.com/api/upload")
	if ui.Button(c, "上传到服务器").Primary().Clicked() {
		n := up.Submit()
		ui.Message(c, "info", fmt.Sprintf("开始上传 %d 个文件", n))
	}
}
```

### 图片列表与照片墙

`PictureList()`（即 `ListType(ui.UploadListPicture)`）以带缩略图的行展示文件；`PictureCard()` 以缩略图卡片展示，最后一张是 “+” 上传卡片，达到 `MaxCount` 后上传卡片隐藏。缩略图取自 `UploadFile.Thumb`。照片墙上点击预览图标后，`Previewed()` 返回该卡片的下标，可用来打开大图预览。

```go
import (
	"github.com/tangoui/tango/ui"
)

var (
	shots   []ui.UploadFile
	gallery []ui.UploadFile
	preview = -1
)

func page(c *ui.Context) {
	ui.Upload(c, &shots).PictureList().Accept("image/*").Width(360)

	up := ui.Upload(c, &gallery).PictureCard().Accept("image/*").MaxCount(8)
	if i := up.Previewed(); i >= 0 {
		preview = i
	}
	if i := up.Removed(); i >= 0 {
		preview = -1
	}
}
```

### 拖拽上传

`Dragger()` 把触发器换成大面积的虚线拖拽区，`DragText(标题, 提示)` 设置其中文字。拖拽区同样可以点击选择文件；`Directory()` 支持选择或拖入整个文件夹。

```go
import (
	"errors"
	"strings"

	"github.com/tangoui/tango/ui"
)

var dropped []ui.UploadFile

func page(c *ui.Context) {
	ui.Upload(c, &dropped).Dragger().Multiple().
		Accept("image/*").MaxSize(1 << 20).
		BeforeUpload(func(f ui.UploadFile) error {
			if strings.HasPrefix(f.Name, "tmp") {
				return errors.New(f.Name + " 是临时文件，已忽略")
			}
			return nil
		}).
		DragText("将图片拖到此处，或点击上传", "支持 png / jpg，单个不超过 1MB").
		Width(520)
}
```

窗口要把系统的拖放事件交给界面：创建窗口时打开 `SurfaceOptions.AcceptFiles`，再调用 `uihost.AcceptDrops(runner, surface)`；拖拽区会在文件拖到它上方时高亮，松开后按 `Accept`、`MaxSize`、`BeforeUpload` 检查并加入列表。

### 文件校验

检查按顺序进行：`Accept`（扩展名或 MIME 类型，逗号分隔，如 `".png,.jpg"`、`"image/*"`）→ `MaxSize`（字节）→ `BeforeUpload`（返回非 nil 错误则拒绝，错误文本作为提示）。被拒绝的文件不会加入列表，原因显示在触发器下方，也可以通过 `Rejections()` 读取。`MaxCount` 限制文件总数。

### 接入系统文件对话框

`Picker` 决定点击触发器时如何选择文件。桌面应用使用 `uihost.FilePicker(app)` 打开系统对话框（`Accept` 会转换成对话框的文件过滤器），一般在页面外层用 `ConfigProvider` 设置一次即可：

```go
import (
	"github.com/tangoui/tango"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/uihost"
)

var (
	app  *tango.App // 应用启动时创建
	docs []ui.UploadFile
)

func page(c *ui.Context) {
	ui.ConfigProvider(c).Defaults("Upload", ui.Props{"picker": uihost.FilePicker(app)}).Body(func() {
		ui.Upload(c, &docs).Multiple()
	})
}
```

## 主题与样式

- Ant Design：按钮触发 + 文本列表，照片墙卡片 102px、大圆角，拖拽区高 152px、浅灰背景虚线框，图标为主色。
- Element Plus：列表文字用次要色、悬停变主色，完成的文件显示成功标记，照片墙卡片 148px、圆角 6px，拖拽区高 180px。
- shadcn：列表行带边框和圆角，操作按钮为幽灵样式，拖拽区透明、悬停时变为浅灰背景。
- 组件令牌：`theme.Components.Upload`（`UploadToken`），包括 `ItemHeight`、`ItemHoverBg`、`ErrorColor`、`ProgressColor`、`PictureCardSize`、`DraggerHeight`、`DraggerBg`、`DraggerDragBorderColor` 等。
- `ConfigProvider.Defaults("Upload", ui.Props{...})` 支持 `accept`、`action`、`maxSize`、`maxCount`、`multiple`、`directory`、`disabled`、`autoUpload`、`listType`（`"text"`、`"picture"`、`"picture-card"`、`"picture-circle"`）、`headers`、`picker`。

## 注意事项

- **状态**：文件列表就是你绑定的切片；上传中的请求由组件管理，并按 `UploadFile.UID` 把进度写回对应元素。可以安全地在代码里删除或重排元素，但不要修改正在上传文件的 `UID`。
- **回调线程**：`CustomRequest`、`Picker` 都在独立 goroutine 中运行，不要在其中直接读写界面状态；进度通过 `progress` 回调汇报即可。
- **本帧事件**：`Added()`、`Removed()`、`Previewed()`、`Picked()` 只在发生的那一帧有值，需要在同一帧读取。
- **表单校验**：放进 [Form](/components/form) 时用 `ui.RulesOf` 绑定文件切片，规则用 `ui.RequiredItems[ui.UploadFile]`（参数为提示文字）。
- **无障碍**：触发器是可聚焦的按钮，Tab 聚焦后 Enter / 空格打开文件选择；文件列表以列表语义暴露，列表项中的预览、删除按钮可用键盘操作。

## 示例代码

<!--@include: ./_gen/upload-examples.md-->

## API

<!--@include: ./_gen/upload-api.md-->

## 相关链接

- [源码 ui/entry_upload.go](https://github.com/tangoui/tango/blob/main/ui/entry_upload.go)、[ui/p2d_upload.go](https://github.com/tangoui/tango/blob/main/ui/p2d_upload.go)、[uihost/upload.go](https://github.com/tangoui/tango/blob/main/uihost/upload.go)
- [ui Go API](/reference/api/ui)、[uihost Go API](/reference/api/uihost)
- [DropZone 拖入文件高亮](/components/catalog/dropzone)、[Form 表单](/components/form)、[ProForm 高级表单](/components/proform)（`upload` 值类型）
