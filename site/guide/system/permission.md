# permission：查询和请求移动端运行时权限

`permission` 用同一套 API 在 iOS 和 Android 上查询、请求运行时权限：相机、相册、定位（使用期间 / 始终）、通知、麦克风。

多数能力包（[location](/guide/system/location)、[media](/guide/system/media)、[push](/guide/system/push)）会在需要时自己请求权限；当你想先解释再弹窗、或者在设置页显示权限状态时，用这个包。

```go
import "github.com/tangoui/tango/sys/permission"
```

## 用法

```go
go func() { // Android 上 Request 必须在 goroutine 里调用
	st, err := permission.Check(app, permission.Camera)
	if err != nil {
		return
	}
	if st == permission.NotDetermined {
		st, err = permission.Request(app, permission.Camera)
	}
	if st == permission.Denied {
		// 不会再弹窗：引导用户去系统设置
	}
}()
```

## API 摘要

| 名字 | 说明 |
|---|---|
| `Check(a, Kind) (Status, error)` | 当前状态，不弹窗 |
| `Request(a, Kind) (Status, error)` | 从未询问过时弹窗，返回结果状态；用户已做过决定则不再弹 |
| `Kind` 常量 | `Camera`、`Photos`、`Location`（使用期间）、`LocationAlways`（含后台）、`Notifications`、`Microphone` |
| `All` | 全部 `Kind` 的列表 |
| `Status` 常量 | `Granted`、`Denied`（拒绝、受策略限制或 Android「不再询问」）、`Limited`（只选了部分照片、近似位置、临时通知授权）、`NotDetermined`（从未询问） |
| `ErrNotConfigured` / `ErrUnsupported` | 未声明权限 / 桌面或不认识的权限 |

`Kind` 的字符串值与 `tan.json` 的 `permissions` 名字一致。完整签名见 [API 参考](/reference/api/sys-permission)。

## 平台支持

| macOS | Windows | Linux | iOS | Android |
|---|---|---|---|---|
| — | — | — | ✓ AVCaptureDevice / PHPhotoLibrary / CLLocationManager / UNUserNotificationCenter | ✓ `checkSelfPermission` / `requestPermissions` |

> 移动端能力仅在模拟器上验证，属实验性，尚未上真机。

## 在页面里调用

目前没有 jsapi 对应，需要时用 `bind.Register` 暴露自己的方法：

```go
type Perms struct{ app *tango.App }

func (p Perms) Camera() (permission.Status, error) { return permission.Request(p.app, permission.Camera) }

bind.Register(r, "Perms", Perms{app})
```

页面里 `await tango.call("Perms.Camera")` 得到 `"granted"`、`"denied"`、`"limited"` 或 `"notDetermined"`。

## 注意

- **必须先声明**：在 `tan.json` 的 `permissions` 列出要用的权限，`tan build` 据此生成 iOS 用途说明和 Android 清单权限：

  | 名字 | iOS Info.plist | Android |
  |---|---|---|
  | `camera` | `NSCameraUsageDescription` | `CAMERA` |
  | `photos` | `NSPhotoLibraryUsageDescription` 等 | `READ_MEDIA_IMAGES`、`READ_MEDIA_VIDEO` 等 |
  | `location` | `NSLocationWhenInUseUsageDescription` | `ACCESS_FINE_LOCATION`、`ACCESS_COARSE_LOCATION` |
  | `location-always` | 另加 `NSLocationAlwaysAndWhenInUseUsageDescription` | 另加 `ACCESS_BACKGROUND_LOCATION` |
  | `microphone` | `NSMicrophoneUsageDescription` | `RECORD_AUDIO` |
  | `notifications` | 无键（运行时询问） | `POST_NOTIFICATIONS`（33+） |

  iOS 上缺用途说明时 `Request` 返回 `ErrNotConfigured`（错误文本写出缺的键名），而不是让系统终止应用；Android 清单未声明时同样返回 `ErrNotConfigured`。详见 [tan.json](/reference/tan-json) 与 [移动端](/guide/mobile)。
- Android 30+ 的后台定位只能在系统设置里授予。
- 桌面返回 `ErrUnsupported`；应用退出后返回 `tango.ErrNotRunning`。
