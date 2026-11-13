# media：从相册选取照片视频、拍照

`media` 让用户从系统相册选取照片或视频（`Pick`），或打开系统相机拍一张照片（`TakePhoto`）。返回的是复制到应用缓存目录里的文件路径，可以直接 `os.Open` 读取。

桌面上没有这两个能力，选文件请用 [dialog](/guide/system/dialog)。

```go
import "github.com/tangoui/tango/sys/media"
```

## 用法

```go
go func() { // Android 上必须在 goroutine 里调用
	paths, err := media.Pick(app, media.PickOptions{Types: media.Images | media.Videos, Limit: 5})
	if errors.Is(err, media.ErrCanceled) {
		return
	}
	for _, p := range paths {
		upload(p)
		os.Remove(p) // 用完自己删除
	}
}()
```

拍照：

```go
path, err := media.TakePhoto(app) // JPEG
if errors.Is(err, media.ErrNotAvailable) {
	// 没有相机（例如 iOS 模拟器）
}
```

## API 摘要

| 名字 | 说明 |
|---|---|
| `Pick(a, PickOptions) ([]string, error)` | 打开系统相册选择器，按选择顺序返回路径 |
| `TakePhoto(a) (string, error)` | 打开系统相机，返回 JPEG 路径 |
| `PickOptions` | `Types`（`Images`、`Videos` 或二者，0 表示仅图片）、`Limit`（最多选几个，0 或 1 表示一个） |
| 错误 | `ErrCanceled`、`ErrNotAvailable`、`ErrPermissionDenied`、`ErrNotConfigured`、`ErrUnsupported` |

完整签名见 [API 参考](/reference/api/sys-media)。

## 平台支持

| | macOS | Windows | Linux | iOS | Android |
|---|---|---|---|---|---|
| `Pick` | — | — | — | ✓ `PHPickerViewController` | ✓ Photo Picker（33+）/ `GET_CONTENT` |
| `TakePhoto` | — | — | — | ✓ `UIImagePickerController` | ✓ `ACTION_IMAGE_CAPTURE` |

> 移动端能力仅在模拟器上验证，属实验性，尚未上真机。

## 在页面里调用

目前没有 jsapi 对应，需要时用 `bind.Register` 暴露自己的方法：

```go
type Photos struct{ app *tango.App }

func (p Photos) PickOne() ([]string, error) { return media.Pick(p.app, media.PickOptions{}) }

bind.Register(r, "Photos", Photos{app})
```

页面里 `await tango.call("Photos.PickOne")`。路径是本地文件，要在页面里显示，需要另外通过自己的方法或资源处理器把内容交给页面（见 [资源](/guide/concepts/assets)）。

## 注意

- **权限**：`Pick` 使用系统选择器，在独立进程中运行，**不需要**相册权限。`TakePhoto` 在 iOS 上需要 `NSCameraUsageDescription`（`tan.json` 的 `permissions` 加 `"camera"`），缺少时返回 `ErrNotConfigured`；Android 上拍照走系统相机，不声明 `CAMERA` 也能用，但一旦声明，Tango 会先请求授权。见 [tan.json](/reference/tan-json)。
- **文件位置**：iOS 为 `Library/Caches/tango-media`，Android 为 `cache/tango-media`。用完请删除。
- 用户取消返回 `ErrCanceled`；桌面返回 `ErrUnsupported`；应用退出后返回 `tango.ErrNotRunning`。
