# location：获取设备位置

`location` 读取设备的地理位置，可以取一次（`Current`），也可以持续跟踪（`Watch`）。iOS 用 Core Location，Android 用系统的 `LocationManager`，**不依赖** Google Play 服务。

如果用户从未被询问过，两个函数都会先请求「使用期间」定位权限。

```go
import "github.com/tangoui/tango/sys/location"
```

## 用法

```go
go func() { // Android 上必须在 goroutine 里调用 Current
	loc, err := location.Current(app, location.Options{Accuracy: location.High})
	switch {
	case errors.Is(err, location.ErrPermissionDenied):
		// 用户拒绝
	case errors.Is(err, location.ErrNotAvailable):
		// 定位服务已关闭
	case err == nil:
		log.Printf("%.5f, %.5f ±%.0fm", loc.Latitude, loc.Longitude, loc.Accuracy)
	}
}()
```

持续跟踪：

```go
stop, err := location.Watch(app, location.Options{DistanceFilter: 50}, func(l location.Location, err error) {
	if err != nil {
		return // 权限被撤销或服务关闭，本次跟踪已结束
	}
	updateMap(l)
})
defer stop()
```

## API 摘要

| 名字 | 说明 |
|---|---|
| `Current(a, Options) (Location, error)` | 取一次位置 |
| `Watch(a, Options, fn) (stop func(), error)` | 每次有新位置就在主线程调用 `fn`，直到 `stop` |
| `Options` | `Accuracy`；`DistanceFilter`（米，`Watch` 只报告移动超过该距离的更新，0 表示全部） |
| `Balanced`（默认，约 100 m）`High`（最佳，GPS）`Low`（公里级） | 精度档位，以电量换精度 |
| `Location` | `Latitude`、`Longitude`、`Altitude`、`Accuracy`、`AltitudeAccuracy`、`Speed`、`Heading`、`Time` |
| 错误 | `ErrPermissionDenied`、`ErrNotAvailable`、`ErrNotConfigured`、`ErrUnsupported` |

完整签名见 [API 参考](/reference/api/sys-location)。

## 平台支持

| macOS | Windows | Linux | iOS | Android |
|---|---|---|---|---|
| — | — | — | ✓ `CLLocationManager` | ✓ `LocationManager` |

> 移动端能力仅在模拟器上验证，属实验性，尚未上真机。

## 在页面里调用

目前没有 jsapi 对应，需要时用 `bind.Register` 暴露自己的方法：

```go
type Geo struct{ app *tango.App }

func (g Geo) Here() (location.Location, error) { return location.Current(g.app, location.Options{}) }

bind.Register(r, "Geo", Geo{app})
```

页面里 `await tango.call("Geo.Here")`。

## 注意

- **权限声明**：在 `tan.json` 的 `permissions` 里加 `"location"`。iOS 生成 `NSLocationWhenInUseUsageDescription`，缺少时返回 `ErrNotConfigured`；Android 生成 `ACCESS_FINE_LOCATION`、`ACCESS_COARSE_LOCATION`。本包只请求「使用期间」权限；后台定位需另外声明 `location-always` 并用 [permission](/guide/system/permission) 请求。见 [tan.json](/reference/tan-json)。
- 多个 `Watch` 共享一个定位会话，采用其中要求最高的精度。
- `Altitude` 只在 `AltitudeAccuracy > 0` 时有效；`Speed`、`Heading` 为负表示未知。
- 桌面返回 `ErrUnsupported`；应用退出后返回 `tango.ErrNotRunning`。
