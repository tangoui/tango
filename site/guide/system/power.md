# power：电源状态、睡眠唤醒与阻止休眠

`power` 定义了三类电源相关能力：读取电源状态（交流电还是电池、电量、是否充电、省电模式）、在系统睡眠和唤醒时通知应用、在播放视频或执行长任务时阻止系统空闲休眠。

::: info 平台支持
macOS（IOKit 电源信息、IOPMAssertion）、Windows（GetSystemPowerStatus、PowerCreateRequest，退回 SetThreadExecutionState）、Linux（UPower、systemd-logind、power-profiles-daemon）已实现；iOS、Android 暂不支持，调用返回 `ErrUnsupported`。Windows 和 Linux 还没有在实机上验证（实验性）。
:::

```go
import "github.com/tangoui/tango/sys/power"
```

## 用法

在不支持的平台上调用会返回 `ErrUnsupported`，建议写成可降级的样子：

```go
power.OnSuspend(app, pauseSync) // 可以在 Run 之前调用
power.OnResume(app, resumeSync)

st, err := power.Get(app)
if err == nil && st.Source == power.Battery && st.Battery < 0.2 {
	reduceWork()
}

blk, err := power.KeepDisplayOn(app, "正在播放视频")
if err == nil {
	defer blk.Release()
}
```

## API 摘要

| 名字 | 说明 |
|---|---|
| `Get(a) (Status, error)` | 当前电源状态 |
| `Status` | `Source`（`Unknown`/`AC`/`Battery`）、`HasBattery`、`Battery`（`[0,1]`，未知或无电池为 -1）、`Charging`、`LowPower` |
| `OnChange(a, fn)` | 电源、电量或省电模式变化时调用 |
| `OnSuspend(a, fn)` / `OnResume(a, fn)` | 系统即将睡眠 / 已唤醒时调用 |
| `KeepAwake(a, reason) (*Block, error)` | 阻止系统空闲休眠（屏幕仍可能关闭），用于长任务 |
| `KeepDisplayOn(a, reason) (*Block, error)` | 保持屏幕常亮且系统不休眠，用于视频、演示 |
| `(*Block).Release()` / `Active()` | 结束阻止（重复调用无害）/ 是否仍有效 |

完整签名见 [API 参考](/reference/api/sys-power)。

## 平台支持

| macOS | Windows | Linux | iOS | Android |
|---|---|---|---|---|
| 未实现 | 未实现 | 未实现 | 未实现 | 未实现 |

> 当前所有平台都返回 `ErrUnsupported`，回调不会触发。

## 在页面里调用

目前没有 jsapi 对应，需要时用 `bind.Register` 暴露自己的方法：

```go
type Power struct{ app *tango.App }

func (p Power) Status() (power.Status, error) { return power.Get(p.app) }

bind.Register(r, "Power", Power{app})
```

## 注意

- 设计上：函数在应用运行后可在任意 goroutine 调用，回调在主线程运行；`On*` 可以在 `Run` 之前注册；应用退出后 `Get` 返回 `tango.ErrNotRunning`。
- 设计上：用户主动的睡眠（合盖、选「睡眠」）不会被阻止，`Block` 只阻止空闲休眠；进程退出时系统自动释放。
- `OnSuspend` 触发后系统不会等太久，保存状态要快。
